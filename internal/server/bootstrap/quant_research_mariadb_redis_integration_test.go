package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"

	marketdatabootstrap "example.com/stock-analysis-flow/internal/marketdata/bootstrap"
	marketdatacache "example.com/stock-analysis-flow/internal/marketdata/cache"
	marketdataconfig "example.com/stock-analysis-flow/internal/marketdata/config"
	marketdatarepository "example.com/stock-analysis-flow/internal/marketdata/repository"
	newsstorage "example.com/stock-analysis-flow/internal/news/adapters/storage"
	newsapp "example.com/stock-analysis-flow/internal/news/app"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	platformhttpclient "example.com/stock-analysis-flow/internal/platform/httpclient"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	quantresult "example.com/stock-analysis-flow/internal/quant/result"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
	quantstorage "example.com/stock-analysis-flow/internal/quant/storage"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
	serverresources "example.com/stock-analysis-flow/internal/server/resources"
)

const quantResearchL3TestEnv = "QUANT_RESEARCH_MARIADB_REDIS_L3_TEST"

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

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

	// Seed real PIT rows through production repository APIs so the real Worker's
	// Data Module can construct authorized Close Series / facts Snapshots from
	// MariaDB. Every gate (PIT timestamps, normalization version, exact-value
	// evidence, valid calendar, published policies) is satisfied by the fixtures
	// in quant_research_l3_seed_test.go; no snapshot is forged. See doc/quant/14.
	l3DataStore, err := marketdatarepository.NewWithBorrowedDB(db)
	if err != nil {
		t.Fatalf("create L3 seed Data store: %v", err)
	}
	seedQuantResearchL3(t, ctx, l3DataStore)
	// UpsertBars stamps revisions with UTC_TIMESTAMP(6) and the PIT reader rewrites
	// SystemObservedAt to that recorded_at. Give the clock a clear margin so the Run
	// system_as_of (used as the PIT cutoff) is strictly after the seeded revisions.
	time.Sleep(2 * time.Second)

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
	if loaded.Run.Status != researchflowdomain.RunSucceeded {
		t.Fatalf("L3 Run status=%s, want succeeded\n%s", loaded.Run.Status, child.output.String())
	}
	wantStageStatus := map[string]researchflowdomain.StageStatus{
		"quant.sma20":  researchflowdomain.StageDegraded,
		"quant.factor": researchflowdomain.StageSucceeded,
		"quant.trend":  researchflowdomain.StageSucceeded,
		"quant.risk":   researchflowdomain.StageSucceeded,
	}
	stagesByCode := make(map[string]researchflowdomain.Stage, len(loaded.Stages))
	for _, stage := range loaded.Stages {
		stagesByCode[stage.Code] = stage
		want, ok := wantStageStatus[stage.Code]
		if !ok {
			t.Fatalf("unexpected Stage %s in quant.research Run", stage.Code)
		}
		if stage.Status != want {
			t.Fatalf("Stage %s status=%s reasons=%v, want %s\n%s", stage.Code, stage.Status, stage.ReasonCodes, want, child.output.String())
		}
		if stage.OutputRef == nil {
			t.Fatalf("Stage %s has no persisted output; reasons=%v", stage.Code, stage.ReasonCodes)
		}
	}
	for code, want := range wantStageStatus {
		if _, ok := stagesByCode[code]; !ok {
			t.Fatalf("quant.research Run is missing Stage %s (want %s)", code, want)
		}
	}
	sma := stagesByCode["quant.sma20"]
	if len(sma.ReasonCodes) != 1 || sma.ReasonCodes[0] != "quant_sma20_insufficient_data" {
		t.Fatalf("sma20 reasons=%v, want [quant_sma20_insufficient_data]", sma.ReasonCodes)
	}

	riskStage := stagesByCode["quant.risk"]
	riskPayloadResource, err := owner.execution.runRepository.GetStageOutputResource(ctx, *riskStage.OutputRef)
	if err != nil {
		t.Fatalf("read risk Stage payload: %v", err)
	}
	var riskPayload quantstage.RiskOutput
	if err := json.Unmarshal(riskPayloadResource.Content, &riskPayload); err != nil {
		t.Fatalf("decode risk Stage payload: %v", err)
	}
	if riskPayload.Status != "reject" || riskPayload.ResultStatus != "rejected" {
		t.Fatalf("risk decision=%s result_status=%s, want reject/rejected", riskPayload.Status, riskPayload.ResultStatus)
	}
	if !containsString(riskPayload.MissingFacts, "price_limit") || !containsString(riskPayload.MissingFacts, "tradable_volume") {
		t.Fatalf("risk missing_facts=%v, want price_limit and tradable_volume", riskPayload.MissingFacts)
	}

	// ---- 阶段 2.2 C 组：生产授权读取链端到端贯通 ----
	// 组合根早已装配 quantResultReader（researchflow_execution_composition.go），但此前
	// 没有任何生产消费者。以下经唯一领域入口 readQuantResultForRun，在真实 MariaDB 上
	// 贯通 GetResult → Run/Stage 关联校验 → 当前策略 FOR UPDATE 护栏 → 不可变结果读取。
	// 不新增 HTTP 端点、不绕过 Worker 可信身份、不放宽任何门禁。
	if owner.execution.quantResultReader == nil {
		t.Fatal("生产组合根未装配 quantResultReader")
	}
	resultPolicyStore, err := quantstorage.NewMySQLResultAccessPolicyStore(db)
	if err != nil {
		t.Fatalf("create Quant result access policy store: %v", err)
	}
	resultPolicyManager, err := quantapplication.NewResultAccessPolicyManager(resultPolicyStore)
	if err != nil {
		t.Fatalf("create Quant result access policy manager: %v", err)
	}
	policyState, err := resultPolicyManager.Current(ctx)
	if err != nil {
		t.Fatalf("read current Quant result access policy: %v", err)
	}
	if policyState.Enabled || policyState.Version != 0 {
		t.Fatalf("L3 隔离库应为 fail-closed 引导态: %+v", policyState)
	}

	readResult := func(runRef, resultID string) (quantresult.Envelope, error) {
		return owner.execution.readQuantResultForRun(ctx, runRef, resultID)
	}
	assertDenied := func(label, runRef, resultID string) {
		t.Helper()
		envelope, readErr := readResult(runRef, resultID)
		if readErr == nil || envelope.ResultID != "" || envelope.AuthorizationRef != "" {
			t.Fatalf("%s: 期望 fail-closed，实际 envelope=%+v err=%v", label, envelope, readErr)
		}
	}

	// 从四份真实 Stage Output 提取 result_id，复用关联规格表既有的提取器。
	resultIDs := make(map[string]string, len(stagesByCode))
	for code, stage := range stagesByCode {
		resource, resourceErr := owner.execution.runRepository.GetStageOutputResource(ctx, *stage.OutputRef)
		if resourceErr != nil {
			t.Fatalf("read %s Stage Output: %v", code, resourceErr)
		}
		spec, supported := quantStageAssociationSpecFor(code)
		if !supported {
			t.Fatalf("Stage %s 未登记 Quant 结果关联规格表", code)
		}
		id, extractErr := spec.ResultID(json.RawMessage(resource.Content))
		if extractErr != nil || id == "" {
			t.Fatalf("extract %s result_id: %q %v", code, id, extractErr)
		}
		resultIDs[code] = id
	}
	if len(resultIDs) != 4 {
		t.Fatalf("expected 4 Quant result_ids, got %d", len(resultIDs))
	}

	// 修复前失败证据：策略未发布时读取必须在当前策略门禁处 fail-closed。
	probeID := resultIDs["quant.risk"]
	assertDenied("策略未发布", created.Run.Ref, probeID)

	// 由可信 Owner 用例启用既有门禁（不是放宽门禁）。
	policyState, err = resultPolicyManager.Publish(ctx, "owner", policyState.Version)
	if err != nil {
		t.Fatalf("publish Quant result access policy: %v", err)
	}
	if !policyState.Enabled || policyState.Snapshot == nil {
		t.Fatalf("published policy is not enabled with a snapshot: %+v", policyState)
	}

	// 1) 同 Run 关联可读：四 Stage 的 result_id 均返回完整 Envelope 与 allowed 审计。
	for code, id := range resultIDs {
		envelope, readErr := readResult(created.Run.Ref, id)
		if readErr != nil {
			t.Fatalf("同 Run 授权读取 %s(%s) 失败: %v", code, id, readErr)
		}
		if envelope.ResultID != id || envelope.AuthorizationRef == "" || envelope.Payload == nil {
			t.Fatalf("同 Run %s Envelope 不完整: %+v", code, envelope)
		}
		var decision, reason string
		if err := db.QueryRowContext(ctx,
			`SELECT decision, reason_code FROM quant_access_audit WHERE run_ref = ? AND resource_ref = ? ORDER BY audit_id DESC LIMIT 1`,
			created.Run.Ref, id).Scan(&decision, &reason); err != nil {
			t.Fatalf("read allowed audit for %s: %v", code, err)
		}
		if decision != "allowed" || reason != "result_released" {
			t.Fatalf("%s audit = %s/%s, want allowed/result_released", code, decision, reason)
		}
	}

	// 停掉 Worker，避免它认领下面创建的第二个 Run 并引入非确定性输出。
	child.stop(t)

	// 2) 跨 Run fail-closed：第二个真实持久化的 Run + Run A 的合法 result_id。
	createdB, err := owner.createRun.CreateRun(principalCtx, researchflowdomain.CreateRunCommand{
		SchemaVersion:     "1.0.0",
		IdempotencyKey:    fmt.Sprintf("quant-research-l3-B-%d", time.Now().UTC().UnixNano()),
		TemplateRef:       quantResearchWorkflowTemplateRef,
		TemplateVersion:   quantResearchWorkflowTemplateVersion,
		InputSnapshotRefs: []string{},
		Parameters:        parameters,
	})
	if err != nil {
		t.Fatalf("create second real Run: %v", err)
	}
	assertDenied("跨 Run", createdB.Run.Ref, probeID)

	// 3) 无关联 fail-closed：格式合法但不属于任何 Stage 的 result_id，且底层结果零读取。
	unrelatedID := "quant_result:sha256:" + strings.Repeat("0", 64)
	assertDenied("无关联", created.Run.Ref, unrelatedID)
	var released int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM quant_access_audit WHERE decision = 'allowed' AND resource_ref = ?`,
		unrelatedID).Scan(&released); err != nil {
		t.Fatalf("count released audit rows for unrelated result: %v", err)
	}
	if released != 0 {
		t.Fatalf("unrelated result was released %d times", released)
	}

	// 4) 伪造 RunRef fail-closed：空、空白、改写聚合 Ref、合法字符集但不存在。
	for _, forged := range []string{"", "  ", created.Run.Ref + "-tampered", "run:researchflow:forged-0001"} {
		assertDenied("伪造 RunRef="+forged, forged, probeID)
	}

	// 5) 撤销竞态 fail-closed：读护栏持有 policy head 行锁期间 revoke 无法插入；
	// 释放后策略已 Disabled，后续读取不得返回 Envelope。只证明阻塞，不断言调度时序。
	var guard quantapplication.ResultAccessPolicyReadGuard = resultPolicyStore
	locked := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseRead := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseRead()
	readDone := make(chan error, 1)
	go func() {
		readDone <- guard.WithCurrentResultAccessPolicy(ctx, func(state quantapplication.ResultAccessPolicyState) error {
			if !state.Enabled || state.Version != policyState.Version {
				return errors.New("policy drifted inside the active read guard")
			}
			close(locked)
			<-release
			return nil
		})
	}()
	select {
	case <-locked:
	case <-time.After(5 * time.Second):
		t.Fatal("read guard did not enter its callback")
	}
	revokeDone := make(chan error, 1)
	go func() {
		_, revokeErr := resultPolicyManager.Revoke(ctx, "owner", policyState.Version)
		revokeDone <- revokeErr
	}()
	select {
	case revokeErr := <-revokeDone:
		t.Fatalf("revoke linearized inside an active read guard: %v", revokeErr)
	case <-time.After(300 * time.Millisecond):
	}
	// 释放前用不加锁的裸读复核：护栏仍活跃时 revoke 不得提交新版本或审计行。
	// 只依赖 FOR UPDATE 的线性化语义，不依赖"revoke 跑得慢"这种时序假象——移除
	// guard 行锁的变异会在这里观测到已提交的 revoke，从而让测试失败。
	var headVersion uint64
	if err := db.QueryRowContext(ctx,
		`SELECT current_version FROM quant_result_access_policy_head WHERE policy_type = 'quant_result_access' AND policy_ref = 'quant-result-access'`,
	).Scan(&headVersion); err != nil {
		t.Fatalf("read policy head while read guard active: %v", err)
	}
	if headVersion != policyState.Version {
		t.Fatalf("revoke committed policy version %d while the read guard was active (want %d)", headVersion, policyState.Version)
	}
	var revokeAudits int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM quant_policy_admin_audit WHERE action = 'revoke'`,
	).Scan(&revokeAudits); err != nil {
		t.Fatalf("read policy admin audit while read guard active: %v", err)
	}
	if revokeAudits != 0 {
		t.Fatalf("revoke wrote %d admin audit rows while the read guard was active", revokeAudits)
	}
	releaseRead()
	if err := <-readDone; err != nil {
		t.Fatalf("read guard: %v", err)
	}
	if err := <-revokeDone; err != nil {
		t.Fatalf("revoke: %v", err)
	}
	assertDenied("撤销后读取", created.Run.Ref, probeID)

	// ---- 阶段 2.2 D 组：News Evidence 关联门禁在真实组合根 + 真库上 fail-closed ----
	// quant.research Run 不含 News Evidence Stage，因此任何 Evidence 读取都必须先被
	// Run↔Evidence 关联校验拒绝，绝不触达 News 的 PIT / 当前授权读取内核。
	// 同 Run 可读的正例由 news_evidence_association_test.go 用生产 Validator 自检的
	// feed payload 覆盖；真库正例需要真实 News Feed Run + Evidence PIT 授权 fixture，
	// 属 FLOW-006 前置，如实保留（doc/quant/15）。
	if owner.execution.newsEvidenceReader == nil {
		t.Fatal("生产组合根未装配 newsEvidenceReader")
	}
	newsRequest := newsapp.GetEvidenceRequest{NewsID: "news:sha256:" + strings.Repeat("3", 64)}
	if _, err := owner.execution.readNewsEvidenceForRun(ctx, created.Run.Ref, newsRequest); !errors.Is(err, newsapp.ErrEvidenceAssociationNotFound) {
		t.Fatalf("无 News Evidence Stage 的 Run 读取应判无关联: %v", err)
	}
	for _, forged := range []string{"", "  ", "run:researchflow:news-forged-0001"} {
		if _, err := owner.execution.readNewsEvidenceForRun(ctx, forged, newsRequest); err == nil {
			t.Fatalf("伪造 News RunRef %q 必须 fail-closed", forged)
		}
	}
}
