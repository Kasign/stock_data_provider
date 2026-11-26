package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"

	marketdatabootstrap "example.com/stock-analysis-flow/internal/marketdata/bootstrap"
	marketdatacache "example.com/stock-analysis-flow/internal/marketdata/cache"
	marketdataconfig "example.com/stock-analysis-flow/internal/marketdata/config"
	marketdatarepository "example.com/stock-analysis-flow/internal/marketdata/repository"
	newsstorage "example.com/stock-analysis-flow/internal/news/adapters/storage"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	platformhttpclient "example.com/stock-analysis-flow/internal/platform/httpclient"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
	serverresources "example.com/stock-analysis-flow/internal/server/resources"
)

const quantResearchL3TestEnv = "QUANT_RESEARCH_MARIADB_REDIS_L3_TEST"

// TestQuantResearchMariaDBRedisL3 verifies the production composition path for
// quant.research against real MariaDB and Redis: the template is published, an
// Owner creates a Run carrying the per-Stage parameter envelope, and the Run is
// persisted with four pending Stages. It is opt-in because it needs real
// services; CI wires the services through integration-ci.yml.
func TestQuantResearchMariaDBRedisL3(t *testing.T) {
	if os.Getenv(quantResearchL3TestEnv) != "1" {
		t.Skip("set QUANT_RESEARCH_MARIADB_REDIS_L3_TEST=1 with MariaDB and Redis configured")
	}
	adminDSN := os.Getenv("SERVER_RELAY_TEST_MYSQL_DSN")
	redisAddr := os.Getenv("PLATFORM_REDIS_ADDR")
	if adminDSN == "" || redisAddr == "" {
		t.Fatal("L3 requires SERVER_RELAY_TEST_MYSQL_DSN and PLATFORM_REDIS_ADDR")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	adminConfig, err := mysql.ParseDSN(adminDSN)
	if err != nil {
		t.Fatalf("parse MariaDB admin DSN: %v", err)
	}
	if adminConfig.Net != "tcp" {
		t.Fatalf("MariaDB admin DSN network=%q, want tcp", adminConfig.Net)
	}
	host, port, err := net.SplitHostPort(adminConfig.Addr)
	if err != nil {
		t.Fatalf("parse MariaDB address: %v", err)
	}
	admin, err := sql.Open("mysql", adminDSN)
	if err != nil {
		t.Fatalf("open MariaDB admin: %v", err)
	}
	if err := admin.PingContext(ctx); err != nil {
		_ = admin.Close()
		t.Fatalf("ping MariaDB admin: %v", err)
	}

	database := fmt.Sprintf("quant_research_l3_%d", time.Now().UTC().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+database+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		_ = admin.Close()
		t.Fatalf("create isolated database: %v", err)
	}
	databaseConfig := *adminConfig
	databaseConfig.DBName = database
	connector, err := mysql.NewConnector(&databaseConfig)
	if err != nil {
		_, _ = admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+database+"`")
		_ = admin.Close()
		t.Fatalf("create isolated connector: %v", err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(16)
	t.Cleanup(func() {
		_ = db.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = admin.ExecContext(cleanupCtx, "DROP DATABASE IF EXISTS `"+database+"`")
		_ = admin.Close()
	})
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping isolated MariaDB: %v", err)
	}

	migrationRegistry, err := newMigrationRegistry()
	if err != nil {
		t.Fatalf("create migration registry: %v", err)
	}
	report, err := migrationRegistry.RunWithReport(ctx, db)
	if err != nil {
		t.Fatalf("apply migrations: report=%+v error=%v", report, err)
	}

	redisClient, err := platformstorage.OpenRedis(ctx, redis.Options{
		Addr: redisAddr, Password: os.Getenv("REDIS_PASSWORD"), DB: 15,
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("open Redis: %v", err)
	}
	t.Cleanup(func() { _ = redisClient.Close() })

	httpClient, err := platformhttpclient.New(platformhttpclient.Config{})
	if err != nil {
		t.Fatalf("create Platform HTTP client: %v", err)
	}
	defer httpClient.CloseIdleConnections()

	shared := &serverresources.Resources{DB: db, Redis: redisClient, HTTPClient: httpClient}
	store, err := marketdatarepository.NewWithBorrowedDB(db)
	if err != nil {
		t.Fatalf("create Data borrowed store: %v", err)
	}
	cacheAdapter, err := marketdatacache.NewGoRedis(redisClient)
	if err != nil {
		t.Fatalf("create Data Redis cache: %v", err)
	}
	dataCfg := marketdataconfig.Config{
		ProviderPriority: []string{"eastmoney"},
		ProviderSettings: map[string]marketdataconfig.ProviderSettings{
			"eastmoney": {Enabled: true, DailyBars: true},
		},
	}
	dataModule, err := marketdatabootstrap.New(dataCfg, marketdatabootstrap.Dependencies{
		Store: store, Cache: cacheAdapter,
	})
	if err != nil {
		t.Fatalf("create Data Module: %v", err)
	}
	t.Cleanup(func() { _ = dataModule.Close(context.Background()) })

	newsRepository, err := newsstorage.NewMySQLRepository(db)
	if err != nil {
		t.Fatalf("create News Repository: %v", err)
	}
	capacity := serverconfig.ResearchflowCapacity{Configured: true, MaxQueuedRuns: 10, MaxWorkers: 1}
	owner, err := newResearchflowOwnerComposition(ctx, shared, newsRepository, newsconfig.Config{}, capacity, 90*time.Second, dataModule)
	if err != nil {
		t.Fatalf("create Researchflow Owner composition with Data Module: %v", err)
	}
	if err := owner.publishWorkflowTemplate(ctx); err != nil {
		t.Fatalf("publish workflow templates: %v", err)
	}

	// Assert the production composition published quant.research@1.0.0 with the
	// four Stage codes; otherwise CreateRun below would only prove a News Run.
	published, err := owner.execution.templateRegistry.Get(ctx, quantResearchWorkflowTemplateRef, quantResearchWorkflowTemplateVersion)
	if err != nil {
		t.Fatalf("read published quant.research template: %v", err)
	}
	if len(published.Stages) != 4 {
		t.Fatalf("quant.research Stage count = %d, want 4", len(published.Stages))
	}
	for _, code := range []string{"quant.sma20", "quant.factor", "quant.trend", "quant.risk"} {
		found := false
		for _, stage := range published.Stages {
			if stage.Code == code {
				found = true
			}
		}
		if !found {
			t.Fatalf("quant.research is missing Stage %s", code)
		}
	}

	parameters, err := os.ReadFile("../../../doc/researchflow/golden/quant-research-parameters-1.0.0.golden.json")
	if err != nil {
		t.Fatalf("read quant.research golden: %v", err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(parameters, &envelope); err != nil {
		t.Fatalf("decode quant.research golden: %v", err)
	}
	if _, ok := envelope["stages"]; !ok {
		t.Fatal("quant.research golden has no stages envelope")
	}

	principalCtx, err := platformsecurity.WithPrincipal(ctx, platformsecurity.Principal{
		Kind: platformsecurity.IdentityOwner, AuthMethod: platformsecurity.AuthMethodOwnerSession, Subject: "owner",
	})
	if err != nil {
		t.Fatalf("create Owner principal: %v", err)
	}
	created, err := owner.createRun.CreateRun(principalCtx, researchflowdomain.CreateRunCommand{
		SchemaVersion:  "1.0.0",
		IdempotencyKey: fmt.Sprintf("quant-research-l3-%d", time.Now().UTC().UnixNano()),
		TemplateRef:    quantResearchWorkflowTemplateRef, TemplateVersion: quantResearchWorkflowTemplateVersion,
		InputSnapshotRefs: []string{}, Parameters: parameters,
	})
	if err != nil {
		t.Fatalf("create quant.research Run: %v", err)
	}
	loaded, err := owner.execution.runRepository.GetRun(ctx, created.Run.Ref)
	if err != nil {
		t.Fatalf("read persisted quant.research Run: %v", err)
	}
	if loaded.Run.Status != researchflowdomain.RunCreated || len(loaded.Stages) != 4 {
		t.Fatalf("persisted Run shape: status=%s stages=%d", loaded.Run.Status, len(loaded.Stages))
	}
	for _, stage := range loaded.Stages {
		if stage.Status != researchflowdomain.StagePending {
			t.Fatalf("Stage %s status = %s, want pending", stage.Code, stage.Status)
		}
	}

	// Start a real Server Worker OS process against the same isolated MariaDB
	// and Redis. The Run must be claimed, executed and finalized by production
	// code; the parent only polls the persisted state.
	values := map[string]string{
		workerBootstrapHelperEnv:                "1",
		"SERVER_ROLE":                           "worker",
		"DB_HOST":                               host,
		"DB_PORT":                               port,
		"DB_USER":                               adminConfig.User,
		"DB_PASSWORD":                           adminConfig.Passwd,
		"DB_NAME":                               database,
		"DB_PARAMS":                             "charset=utf8mb4&parseTime=true&loc=UTC",
		"REDIS_ADDR":                            redisAddr,
		"REDIS_PASSWORD":                        os.Getenv("REDIS_PASSWORD"),
		"REDIS_DB":                              os.Getenv("REDIS_DB"),
		"RESEARCHFLOW_MAX_QUEUED_RUNS":          "10",
		"RESEARCHFLOW_MAX_WORKERS":              "1",
		"RESEARCHFLOW_RUN_DEADLINE":             "5m",
		"RESEARCHFLOW_LOCAL_WORKERS":            "1",
		"PLATFORM_SERVICE_CREDENTIALS":          `[{"name":"quant","token_env":"QUANT_SERVICE_TOKEN","scopes":["data:read"]}]`,
		"QUANT_SERVICE_TOKEN":                   "quant-worker-integration-token-0123456789abcdef",
		"NOTIFICATION_EVENT_MAX_ENVELOPE_BYTES": "65536",
		"NOTIFICATION_EVENT_MAX_PAYLOAD_BYTES":  "32768",
		"NOTIFICATION_EVENT_MAX_JSON_DEPTH":     "32",
		"HTTP_ADDR":                             "127.0.0.1:0",
		"HTTP_READ_TIMEOUT":                     "not-a-duration",
	}
	child := startWorkerBootstrapChild(t, values)
	defer child.killIfRunning()
	child.waitReady(t)

	deadline := time.Now().Add(45 * time.Second)
	for {
		select {
		case childErr := <-child.waitDone:
			child.finished = true
			t.Fatalf("Worker OS process exited during L3 acceptance: %v\n%s", childErr, child.output.String())
		default:
		}
		loaded, err = owner.execution.runRepository.GetRun(ctx, created.Run.Ref)
		if err != nil {
			t.Fatalf("poll persisted Run: %v", err)
		}
		if loaded.Run.Status.Terminal() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Run %s did not reach a terminal state; status=%s\n%s", created.Run.Ref, loaded.Run.Status, child.output.String())
		}
		time.Sleep(50 * time.Millisecond)
	}

	if len(loaded.Stages) != 4 {
		t.Fatalf("terminal Run has %d Stages, want 4", len(loaded.Stages))
	}
	for _, stage := range loaded.Stages {
		if !stage.Status.Terminal() {
			t.Fatalf("Stage %s did not reach a terminal state: %s", stage.Code, stage.Status)
		}
		if stage.AttemptNo < 1 {
			t.Fatalf("Stage %s was never dispatched by the real Worker", stage.Code)
		}
	}
	t.Logf("L3 terminal status=%s stages=%d", loaded.Run.Status, len(loaded.Stages))
	for _, stage := range loaded.Stages {
		t.Logf("stage=%s status=%s reasons=%v output=%v", stage.Code, stage.Status, stage.ReasonCodes, stage.OutputRef != nil)
	}
}
