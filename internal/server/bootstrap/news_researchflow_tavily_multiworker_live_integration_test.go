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
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"

	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	newsdomain "example.com/stock-analysis-flow/internal/news/domain"
	newsstage "example.com/stock-analysis-flow/internal/news/stage"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	platformhttpclient "example.com/stock-analysis-flow/internal/platform/httpclient"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
	serverresources "example.com/stock-analysis-flow/internal/server/resources"
)

// TestNewsResearchflowTavilyMultiWorkerMariaDBRedisLive 验证两个独立 Server Worker
// OS 进程在共享 MariaDB/Redis 下同时领取并执行真实 Tavily Run。
func TestNewsResearchflowTavilyMultiWorkerMariaDBRedisLive(t *testing.T) {
	if os.Getenv("NEWS_RESEARCHFLOW_MULTIWORKER_LIVE_TEST") != "1" {
		t.Skip("NEWS_RESEARCHFLOW_MULTIWORKER_LIVE_TEST 未启用")
	}

	endpoint := strings.TrimSpace(os.Getenv(newsconfig.TavilyEndpointEnv))
	apiKey := strings.TrimSpace(os.Getenv(newsconfig.TavilyAPIKeyEnv))
	adminDSN := strings.TrimSpace(os.Getenv("SERVER_RELAY_TEST_MYSQL_DSN"))
	redisAddr := strings.TrimSpace(os.Getenv("PLATFORM_REDIS_ADDR"))
	if endpoint == "" || apiKey == "" || adminDSN == "" || redisAddr == "" {
		t.Fatal("双 Worker 真实链路验收要求 Tavily、MariaDB admin DSN 和 Redis 地址均已配置")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()

	adminConfig, err := mysql.ParseDSN(adminDSN)
	if err != nil {
		t.Fatalf("解析 MariaDB admin DSN 失败: %v", err)
	}
	if adminConfig.Net != "tcp" {
		t.Fatalf("MariaDB admin DSN network=%q，要求 tcp", adminConfig.Net)
	}
	host, port, err := net.SplitHostPort(adminConfig.Addr)
	if err != nil {
		t.Fatalf("解析 MariaDB 地址失败: %v", err)
	}
	admin, err := sql.Open("mysql", adminDSN)
	if err != nil {
		t.Fatalf("打开 MariaDB admin 连接失败: %v", err)
	}
	if err := admin.PingContext(ctx); err != nil {
		_ = admin.Close()
		t.Fatalf("连接 MariaDB admin 失败: %v", err)
	}

	database := fmt.Sprintf("news_researchflow_multiworker_%d", time.Now().UTC().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+database+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		_ = admin.Close()
		t.Fatalf("创建隔离双 Worker 数据库失败: %v", err)
	}

	databaseConfig := *adminConfig
	databaseConfig.DBName = database
	connector, err := mysql.NewConnector(&databaseConfig)
	if err != nil {
		_, _ = admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+database+"`")
		_ = admin.Close()
		t.Fatalf("创建隔离 MariaDB connector 失败: %v", err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(16)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		_, _ = admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+database+"`")
		_ = admin.Close()
		t.Fatalf("连接隔离 MariaDB 数据库失败: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := admin.ExecContext(cleanupCtx, "DROP DATABASE IF EXISTS `"+database+"`"); err != nil {
			t.Errorf("删除隔离双 Worker 数据库失败: %v", err)
		}
		_ = admin.Close()
	})

	migrationRegistry, err := newMigrationRegistry()
	if err != nil {
		t.Fatalf("创建 Server 完整 Migration Registry 失败: %v", err)
	}
	if report, err := migrationRegistry.RunWithReport(ctx, db); err != nil {
		t.Fatalf("应用 Server 完整 Migration 失败: report=%+v error=%v", report, err)
	}

	newsCfg, err := newsconfig.Load(platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		switch key {
		case newsconfig.TavilyEndpointEnv:
			return endpoint, true
		case newsconfig.TavilyAPIKeyEnv:
			return apiKey, true
		default:
			return "", false
		}
	}))
	if err != nil {
		t.Fatalf("加载双 Worker Tavily 配置失败: %v", err)
	}
	httpClient, err := platformhttpclient.New(platformhttpclient.Config{})
	if err != nil {
		t.Fatalf("创建 Platform HTTP Client 失败: %v", err)
	}
	defer httpClient.CloseIdleConnections()

	shared := &serverresources.Resources{DB: db, HTTPClient: httpClient}
	capacity := serverconfig.ResearchflowCapacity{
		Configured: true, MaxQueuedRuns: 10, MaxWorkers: 2,
	}
	owner, err := newResearchflowOwnerComposition(shared, newsCfg, capacity, 90*time.Second)
	if err != nil {
		t.Fatalf("创建生产 Researchflow Owner 组合失败: %v", err)
	}
	providerCodes, err := owner.newsStage.providerCodes()
	if err != nil {
		t.Fatalf("读取双 Worker Provider inventory 失败: %v", err)
	}
	if len(providerCodes) != 1 || providerCodes[0] != "tavily" {
		t.Fatalf("双 Worker 验收必须只使用真实 Tavily Provider，实际 inventory=%v", providerCodes)
	}
	if err := owner.publishWorkflowTemplate(ctx); err != nil {
		t.Fatalf("发布正式 News Search Workflow Template 失败: %v", err)
	}

	principalCtx, err := platformsecurity.WithPrincipal(ctx, platformsecurity.Principal{
		Kind: platformsecurity.IdentityOwner, AuthMethod: platformsecurity.AuthMethodOwnerSession,
		Subject: "owner",
	})
	if err != nil {
		t.Fatalf("构造受信任 Owner Session Principal 失败: %v", err)
	}
	queries := []string{"A股上市公司公告 近期", "A股上市公司财报 近期"}
	runRefs := make([]string, 0, len(queries))
	for index, query := range queries {
		created, err := owner.createRun.CreateRun(principalCtx, researchflowdomain.CreateRunCommand{
			SchemaVersion:  "1.0.0",
			IdempotencyKey: fmt.Sprintf("tavily-multiworker-live-%d-%d", time.Now().UTC().UnixNano(), index),
			TemplateRef:    newsSearchWorkflowTemplateRef, TemplateVersion: newsSearchWorkflowTemplateVersion,
			InputSnapshotRefs: []string{},
			Parameters:        json.RawMessage(fmt.Sprintf("{\"query\":%q,\"limit\":3}", query)),
		})
		if err != nil {
			t.Fatalf("创建双 Worker 真实 Tavily Run %d 失败: %v", index+1, err)
		}
		runRefs = append(runRefs, created.Run.Ref)
	}

	values := map[string]string{
		workerBootstrapHelperEnv:                "1",
		"SERVER_ROLE":                           "worker",
		"DB_HOST":                               host,
		"DB_PORT":                               port,
		"DB_USER":                               adminConfig.User,
		"DB_PASSWORD":                           adminConfig.Passwd,
		"DB_NAME":                               database,
		"DB_AUTO_MIGRATE":                       "true",
		"DB_PARAMS":                             "charset=utf8mb4&parseTime=true&loc=UTC",
		"REDIS_ADDR":                            redisAddr,
		"REDIS_PASSWORD":                        os.Getenv("REDIS_PASSWORD"),
		"REDIS_DB":                              os.Getenv("REDIS_DB"),
		"MARKET_DATA_BASE_URL":                  "http://127.0.0.1:8091",
		"MARKET_DATA_API_KEY":                   "multiworker-bootstrap-integration-secret-0123456789",
		"RESEARCHFLOW_MAX_QUEUED_RUNS":          "10",
		"RESEARCHFLOW_MAX_WORKERS":              "2",
		"RESEARCHFLOW_RUN_DEADLINE":             "45m",
		"RESEARCHFLOW_LOCAL_WORKERS":            "1",
		"MIGRATION_ENABLED":                     "true",
		"NEWS_PROVIDER_TAVILY_ENDPOINT":         endpoint,
		"NEWS_PROVIDER_TAVILY_API_KEY":          apiKey,
		"NOTIFICATION_EVENT_MAX_ENVELOPE_BYTES": "65536",
		"NOTIFICATION_EVENT_MAX_PAYLOAD_BYTES":  "32768",
		"NOTIFICATION_EVENT_MAX_JSON_DEPTH":     "32",
		"HTTP_ADDR":                             "127.0.0.1:0",
		"HTTP_READ_TIMEOUT":                     "not-a-duration",
	}
	children := make([]*workerBootstrapChild, 0, 2)
	defer func() {
		for _, child := range children {
			child.killIfRunning()
		}
	}()
	for index := 1; index <= 2; index++ {
		childValues := make(map[string]string, len(values)+1)
		for key, value := range values {
			childValues[key] = value
		}
		childValues["RESEARCH_NOTIFICATION_RELAY_WORKER_ID"] = fmt.Sprintf("tavily-multiworker-%d", index)
		children = append(children, startWorkerBootstrapChild(t, childValues))
	}

	// 轮询两个 Run 的活动 Lease。必须同时观察到两个不同 Worker ID，
	// 才能证明两个 OS 进程均实际领取了真实 Provider 工作，而不只是成功启动。
	workerDeadline := time.Now().Add(60 * time.Second)
	observedWorkerIDs := make(map[string]struct{}, 2)
	for {
		allReady := true
		for index, child := range children {
			if !strings.Contains(child.output.String(), "股票分析 Worker 已启动") {
				allReady = false
			}
			select {
			case childErr := <-child.waitDone:
				child.finished = true
				t.Fatalf("Worker OS 进程 %d 在验收期间提前退出: %v\n%s", index+1, childErr, child.output.String())
			default:
			}
		}

		activeWorkerIDs := make(map[string]struct{}, 2)
		for _, runRef := range runRefs {
			var leaseOwner sql.NullString
			err := db.QueryRowContext(ctx, `SELECT lease_owner FROM researchflow_attempts
				WHERE run_ref = ? AND status = 'running' ORDER BY attempt_no DESC LIMIT 1`, runRef).Scan(&leaseOwner)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("读取活动 Researchflow Lease 失败: %v", err)
			}
			if err == nil && leaseOwner.Valid && strings.TrimSpace(leaseOwner.String) != "" {
				activeWorkerIDs[leaseOwner.String] = struct{}{}
			}
		}
		for workerID := range activeWorkerIDs {
			observedWorkerIDs[workerID] = struct{}{}
		}
		if allReady && len(activeWorkerIDs) >= 2 {
			break
		}
		if time.Now().After(workerDeadline) {
			t.Fatalf("未观察到两个不同 Worker 同时持有真实 Run Lease: ready=%t active_worker_ids=%d observed_worker_ids=%d",
				allReady, len(activeWorkerIDs), len(observedWorkerIDs))
		}
		select {
		case <-ctx.Done():
			t.Fatalf("等待双 Worker 同时领取真实 Run 超时: %v", ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}

	startedAt := time.Now()
	loadedRuns := make([]string, 0, len(runRefs))
	candidateCounts := make(map[string]int, len(runRefs))
	for {
		loadedRuns = loadedRuns[:0]
		allTerminal := true
		for _, runRef := range runRefs {
			loaded, err := owner.execution.runRepository.GetRun(ctx, runRef)
			if err != nil {
				t.Fatalf("读取双 Worker Researchflow Run 失败: %v", err)
			}
			if !loaded.Run.Status.Terminal() {
				allTerminal = false
				continue
			}
			if loaded.Run.Status != researchflowdomain.RunSucceeded || loaded.Run.ResultSummaryRef == nil ||
				loaded.Run.ResultSummary == nil || len(loaded.Stages) != 1 {
				t.Fatalf("双 Worker Run 未成功完成或 ResultSummary 不完整: status=%s summary_ref=%t summary=%t stages=%d",
					loaded.Run.Status, loaded.Run.ResultSummaryRef != nil, loaded.Run.ResultSummary != nil, len(loaded.Stages))
			}
			stage := loaded.Stages[0]
			if stage.Code != newsstage.SearchCandidatesStageCode || stage.Status != researchflowdomain.StageSucceeded ||
				stage.AttemptNo != 1 || stage.OutputRef == nil || stage.QualitySummaryRef == nil ||
				len(stage.DiagnosticRefs) != 0 || len(stage.ReasonCodes) != 0 {
				t.Fatalf("双 Worker Stage 终态不正确: status=%s attempts=%d output=%t quality=%t diagnostics=%d reasons=%v",
					stage.Status, stage.AttemptNo, stage.OutputRef != nil, stage.QualitySummaryRef != nil,
					len(stage.DiagnosticRefs), stage.ReasonCodes)
			}

			payloadResource, err := owner.execution.runRepository.GetStageOutputResource(ctx, *stage.OutputRef)
			if err != nil {
				t.Fatalf("读取双 Worker Tavily Payload 失败: %v", err)
			}
			qualityResource, err := owner.execution.runRepository.GetStageOutputResource(ctx, *stage.QualitySummaryRef)
			if err != nil {
				t.Fatalf("读取双 Worker Tavily QualitySummary 失败: %v", err)
			}
			var payload newsstage.SearchCandidatesOutput
			if err := json.Unmarshal(payloadResource.Content, &payload); err != nil {
				t.Fatalf("解析双 Worker Tavily Payload 失败: %v", err)
			}
			if payload.Status != newsdomain.SearchSuccess || len(payload.Candidates) == 0 ||
				len(payload.Outcomes) != 1 || payload.Outcomes[0].ProviderCode != "tavily" ||
				payload.Outcomes[0].Status != newsdomain.ProviderSuccess ||
				payload.Outcomes[0].CandidateCount != len(payload.Candidates) {
				t.Fatalf("双 Worker Tavily Payload 不符合预期: status=%s candidates=%d outcomes=%d",
					payload.Status, len(payload.Candidates), len(payload.Outcomes))
			}
			for _, candidate := range payload.Candidates {
				if candidate.ProviderCode != "tavily" {
					t.Fatalf("双 Worker Payload 出现非 Tavily 候选: %q", candidate.ProviderCode)
				}
			}
			var quality newsstage.SearchCandidatesQualitySummary
			if err := json.Unmarshal(qualityResource.Content, &quality); err != nil {
				t.Fatalf("解析双 Worker QualitySummary 失败: %v", err)
			}
			if quality.ProviderCount != 1 || quality.ProviderSuccessCount != 1 ||
				quality.ProviderFailedCount != 0 || quality.ProviderSkippedCount != 0 ||
				quality.CandidateCount != len(payload.Candidates) {
				t.Fatalf("双 Worker QualitySummary 不一致: providers=%d success=%d failed=%d skipped=%d candidates=%d",
					quality.ProviderCount, quality.ProviderSuccessCount, quality.ProviderFailedCount,
					quality.ProviderSkippedCount, quality.CandidateCount)
			}

			var attempts, outbox, summaries int
			var attemptStatus string
			if err := db.QueryRowContext(ctx, "SELECT COUNT(*), MAX(status) FROM researchflow_attempts WHERE run_ref = ?",
				runRef).Scan(&attempts, &attemptStatus); err != nil {
				t.Fatalf("读取双 Worker Attempt 记录失败: %v", err)
			}
			if attempts != 1 || attemptStatus != string(researchflowdomain.AttemptSucceeded) {
				t.Fatalf("双 Worker Attempt 记录不正确: attempts=%d status=%s", attempts, attemptStatus)
			}
			if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM researchflow_outbox WHERE run_ref = ? AND event_type = 'researchflow.run.terminal'",
				runRef).Scan(&outbox); err != nil {
				t.Fatalf("读取双 Worker 终态 Outbox 失败: %v", err)
			}
			if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM researchflow_result_summaries WHERE result_summary_ref = ? AND run_ref = ?",
				*loaded.Run.ResultSummaryRef, runRef).Scan(&summaries); err != nil {
				t.Fatalf("读取双 Worker ResultSummary 失败: %v", err)
			}
			if outbox != 1 || summaries != 1 {
				t.Fatalf("双 Worker Finalization 不完整: outbox=%d summaries=%d", outbox, summaries)
			}
			candidateCounts[runRef] = len(payload.Candidates)
			loadedRuns = append(loadedRuns, runRef)
		}
		if allTerminal {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("双 Worker 真实 Tavily Runs 未在时限内全部终止: %v", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}

	totalCandidates := 0
	for _, candidateCount := range candidateCounts {
		totalCandidates += candidateCount
	}
	for _, child := range children {
		child.stop(t)
	}
	t.Logf("双 Worker 真实 Tavily 生产链路通过: processes=2 concurrent_worker_ids=%d runs=%d candidates=%d elapsed=%s",
		len(observedWorkerIDs), len(loadedRuns), totalCandidates, time.Since(startedAt))
}
