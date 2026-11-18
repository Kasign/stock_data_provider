package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	newsdomain "example.com/stock-analysis-flow/internal/news/domain"
	newsstage "example.com/stock-analysis-flow/internal/news/stage"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	platformhttpclient "example.com/stock-analysis-flow/internal/platform/httpclient"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	researchflowstorage "example.com/stock-analysis-flow/internal/researchflow/storage"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
	serverresources "example.com/stock-analysis-flow/internal/server/resources"
)

// TestNewsResearchflowTavilyMariaDBLive 验证真实 Tavily 请求经过 Server 生产组合、
// Researchflow Worker Runtime 和 MariaDB 持久化。未显式启用时不访问外部网络。
func TestNewsResearchflowTavilyMariaDBLive(t *testing.T) {
	if os.Getenv("NEWS_RESEARCHFLOW_LIVE_TEST") != "1" {
		t.Skip("NEWS_RESEARCHFLOW_LIVE_TEST 未启用")
	}

	endpoint := strings.TrimSpace(os.Getenv(newsconfig.TavilyEndpointEnv))
	apiKey := strings.TrimSpace(os.Getenv(newsconfig.TavilyAPIKeyEnv))
	if endpoint == "" || apiKey == "" {
		t.Fatal("真实 Researchflow 验收要求完整配置 Tavily endpoint 和 API key")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()

	db := openNewsResearchMariaDB(t, ctx)
	if err := platformstorage.ApplyMigrations(ctx, db, researchflowstorage.MigrationNamespace, researchflowstorage.Migrations()); err != nil {
		t.Fatalf("应用 Researchflow Migration 失败: %v", err)
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
		t.Fatalf("加载隔离的 Tavily News 配置失败: %v", err)
	}

	httpClient, err := platformhttpclient.New(platformhttpclient.Config{})
	if err != nil {
		t.Fatalf("创建 Platform HTTP Client 失败: %v", err)
	}
	defer httpClient.CloseIdleConnections()

	shared := &serverresources.Resources{DB: db, HTTPClient: httpClient}
	capacity := serverconfig.ResearchflowCapacity{
		Configured: true, MaxQueuedRuns: 5, MaxWorkers: 1,
	}
	owner, err := newResearchflowOwnerComposition(shared, newsCfg, capacity, 90*time.Second)
	if err != nil {
		t.Fatalf("创建 Server 生产 Researchflow Owner 组合失败: %v", err)
	}
	providerCodes, err := owner.newsStage.providerCodes()
	if err != nil {
		t.Fatalf("读取生产 Provider inventory 失败: %v", err)
	}
	if len(providerCodes) != 1 || providerCodes[0] != "tavily" {
		t.Fatalf("本验收必须只使用真实 Tavily Provider，实际 Provider inventory=%v", providerCodes)
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
	query := strings.TrimSpace(os.Getenv("NEWS_LIVE_QUERY"))
	if query == "" {
		query = "A股上市公司公告"
	}
	created, err := owner.createRun.CreateRun(principalCtx, researchflowdomain.CreateRunCommand{
		SchemaVersion:  "1.0.0",
		IdempotencyKey: fmt.Sprintf("tavily-live-%d", time.Now().UTC().UnixNano()),
		TemplateRef:    newsSearchWorkflowTemplateRef, TemplateVersion: newsSearchWorkflowTemplateVersion,
		InputSnapshotRefs: []string{},
		Parameters:        json.RawMessage(fmt.Sprintf("{\"query\":%q,\"limit\":3}", query)),
	})
	if err != nil {
		t.Fatalf("通过内部 Owner Session CreateRun 创建真实 Tavily Run 失败: %v", err)
	}

	runtimeComponent, err := newResearchflowRuntimeComponent(owner.execution, capacity, 1)
	if err != nil {
		t.Fatalf("创建 Server Researchflow Runtime 失败: %v", err)
	}
	if err := runtimeComponent.Start(ctx); err != nil {
		t.Fatalf("启动 Server Researchflow Runtime 失败: %v", err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer stopCancel()
		if err := runtimeComponent.Stop(stopCtx); err != nil {
			t.Errorf("停止 Server Researchflow Runtime 失败: %v", err)
		}
	}()

	startedAt := time.Now()
	var loaded researchflowapplication.CreateRunResult
	for {
		loaded, err = owner.execution.runRepository.GetRun(ctx, created.Run.Ref)
		if err != nil {
			t.Fatalf("读取真实 Tavily Researchflow Run 失败: %v", err)
		}
		if loaded.Run.Status.Terminal() {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("真实 Tavily Researchflow Run 未在时限内终止: %v", ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
	if loaded.Run.Status != researchflowdomain.RunSucceeded || loaded.Run.ResultSummaryRef == nil ||
		loaded.Run.ResultSummary == nil || len(loaded.Stages) != 1 {
		t.Fatalf("真实 Tavily Run 未成功完成或 ResultSummary 不完整: status=%s summary_ref_present=%t summary_present=%t stages=%d",
			loaded.Run.Status, loaded.Run.ResultSummaryRef != nil, loaded.Run.ResultSummary != nil, len(loaded.Stages))
	}

	stage := loaded.Stages[0]
	if stage.Code != newsstage.SearchCandidatesStageCode || stage.Status != researchflowdomain.StageSucceeded ||
		stage.AttemptNo != 1 || stage.OutputRef == nil || stage.QualitySummaryRef == nil ||
		len(stage.DiagnosticRefs) != 0 || len(stage.ReasonCodes) != 0 {
		t.Fatalf("真实 Tavily Stage 状态、Attempt 或诊断引用不符合预期: code=%s status=%s attempts=%d output=%t quality=%t diagnostics=%d reasons=%v",
			stage.Code, stage.Status, stage.AttemptNo, stage.OutputRef != nil, stage.QualitySummaryRef != nil,
			len(stage.DiagnosticRefs), stage.ReasonCodes)
	}

	payloadResource, err := owner.execution.runRepository.GetStageOutputResource(ctx, *stage.OutputRef)
	if err != nil {
		t.Fatalf("读取真实 Tavily Stage Payload 失败: %v", err)
	}
	qualityResource, err := owner.execution.runRepository.GetStageOutputResource(ctx, *stage.QualitySummaryRef)
	if err != nil {
		t.Fatalf("读取真实 Tavily Stage QualitySummary 失败: %v", err)
	}
	if payloadResource.Kind != "payload" || qualityResource.Kind != "quality_summary" ||
		payloadResource.RunRef != created.Run.Ref || qualityResource.RunRef != created.Run.Ref ||
		payloadResource.StageRef != stage.Ref || qualityResource.StageRef != stage.Ref ||
		payloadResource.AttemptRef != qualityResource.AttemptRef {
		t.Fatalf("真实 Tavily Stage Output 资源关联不一致: payload_kind=%s quality_kind=%s", payloadResource.Kind, qualityResource.Kind)
	}

	var payload newsstage.SearchCandidatesOutput
	if err := json.Unmarshal(payloadResource.Content, &payload); err != nil {
		t.Fatalf("解析持久化真实 Tavily Payload 失败: %v", err)
	}
	if payload.Status != newsdomain.SearchSuccess || len(payload.Candidates) == 0 ||
		len(payload.Outcomes) != 1 || payload.Outcomes[0].ProviderCode != "tavily" ||
		payload.Outcomes[0].Status != newsdomain.ProviderSuccess ||
		payload.Outcomes[0].CandidateCount != len(payload.Candidates) {
		t.Fatalf("持久化 Provider Outcome 不符合真实 Tavily 成功结果: status=%s candidates=%d outcomes=%d",
			payload.Status, len(payload.Candidates), len(payload.Outcomes))
	}
	for _, candidate := range payload.Candidates {
		if candidate.ProviderCode != "tavily" {
			t.Fatalf("持久化候选包含非 Tavily Provider: %q", candidate.ProviderCode)
		}
		// Candidate 契约没有 available_at；published_at 仅是 Provider 发布时间，不能升级为 PIT 可用时间。
	}

	var quality newsstage.SearchCandidatesQualitySummary
	if err := json.Unmarshal(qualityResource.Content, &quality); err != nil {
		t.Fatalf("解析持久化真实 Tavily QualitySummary 失败: %v", err)
	}
	if quality.ProviderCount != 1 || quality.ProviderSuccessCount != 1 ||
		quality.ProviderFailedCount != 0 || quality.ProviderSkippedCount != 0 ||
		quality.CandidateCount != len(payload.Candidates) {
		t.Fatalf("真实 Tavily QualitySummary 与 Payload 不一致: providers=%d success=%d failed=%d skipped=%d candidates=%d",
			quality.ProviderCount, quality.ProviderSuccessCount, quality.ProviderFailedCount,
			quality.ProviderSkippedCount, quality.CandidateCount)
	}

	var attemptCount, outboxCount, summaryCount int
	var attemptStatus string
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*), MAX(status) FROM researchflow_attempts WHERE run_ref = ? AND stage_ref = ?",
		created.Run.Ref, stage.Ref).Scan(&attemptCount, &attemptStatus); err != nil {
		t.Fatalf("查询真实 Tavily Attempt 持久化状态失败: %v", err)
	}
	if attemptCount != 1 || attemptStatus != string(researchflowdomain.AttemptSucceeded) {
		t.Fatalf("真实 Tavily Attempt 持久化状态不正确: count=%d status=%s", attemptCount, attemptStatus)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM researchflow_outbox WHERE run_ref = ? AND event_type = 'researchflow.run.terminal'",
		created.Run.Ref).Scan(&outboxCount); err != nil {
		t.Fatalf("查询真实 Tavily 终态 Outbox 失败: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM researchflow_result_summaries WHERE result_summary_ref = ? AND run_ref = ?",
		*loaded.Run.ResultSummaryRef, created.Run.Ref).Scan(&summaryCount); err != nil {
		t.Fatalf("查询真实 Tavily ResultSummary 持久化失败: %v", err)
	}
	if outboxCount != 1 || summaryCount != 1 {
		t.Fatalf("真实 Tavily Run Finalization 持久化不完整: outbox=%d result_summary=%d", outboxCount, summaryCount)
	}

	t.Logf("真实 Tavily Researchflow 生产链路通过: run_status=%s stage_status=%s attempts=%d candidates=%d elapsed=%s result_summary=true outbox=%d diagnostics=%d",
		loaded.Run.Status, stage.Status, attemptCount, len(payload.Candidates), time.Since(startedAt),
		outboxCount, len(stage.DiagnosticRefs))
}
