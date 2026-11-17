package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	newsapp "example.com/stock-analysis-flow/internal/news/app"
	newsdomain "example.com/stock-analysis-flow/internal/news/domain"
	newsstage "example.com/stock-analysis-flow/internal/news/stage"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	researchflowparameterschema "example.com/stock-analysis-flow/internal/researchflow/parameterschema"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	researchflowstorage "example.com/stock-analysis-flow/internal/researchflow/storage"
	researchflowtemplate "example.com/stock-analysis-flow/internal/researchflow/template"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
)

type newsSearchIntegrationResourcePolicyResolver struct{}

func (newsSearchIntegrationResourcePolicyResolver) Resolve(_ context.Context, ref string) (resourcepolicy.Snapshot, error) {
	registry, err := resourcepolicy.NewRegistry()
	if err != nil {
		return resourcepolicy.Snapshot{}, err
	}
	return registry.Resolve(ref, []string{"integration_fixture"})
}

const (
	newsSearchIntegrationTemplateRef = "integration.news.search_candidates"
	newsSearchIntegrationSchemaRef   = "integration.news.search_candidates.parameters"
)

type newsSearchIntegrationClock struct {
	db *sql.DB
}

func (c newsSearchIntegrationClock) NowUTC(ctx context.Context) (time.Time, error) {
	var now time.Time
	if err := c.db.QueryRowContext(ctx, "SELECT UTC_TIMESTAMP(6)").Scan(&now); err != nil {
		return time.Time{}, err
	}
	return now.UTC(), nil
}

type newsSearchIntegrationIDs struct {
	next atomic.Uint64
}

func (g *newsSearchIntegrationIDs) NewRef(ctx context.Context, kind string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return fmt.Sprintf("integration:%s:%d", kind, g.next.Add(1)), nil
}

type newsSearchIntegrationIdentity struct{}

func (newsSearchIntegrationIdentity) AuthenticatedCallerRef(context.Context) (string, error) {
	return "service:news-search-integration-test", nil
}
func (newsSearchIntegrationIdentity) OriginatingRequestID(context.Context) *string { return nil }
func (newsSearchIntegrationIdentity) CorrelationID(context.Context) string {
	return "correlation:news-search-integration-test"
}

type newsSearchIntegrationParameterRegistry struct{}

func (newsSearchIntegrationParameterRegistry) Get(_ context.Context, ref string) (researchflowparameterschema.Definition, error) {
	if ref != newsSearchIntegrationSchemaRef {
		return researchflowparameterschema.Definition{}, researchflowparameterschema.ErrSchemaNotFound
	}
	document := json.RawMessage("{\"$schema\":\"https://json-schema.org/draft/2020-12/schema\",\"type\":\"object\",\"additionalProperties\":false,\"required\":[\"query\",\"limit\"],\"properties\":{\"query\":{\"type\":\"string\",\"minLength\":1},\"query_dimension\":{\"type\":\"string\"},\"limit\":{\"type\":\"integer\",\"minimum\":1,\"maximum\":100}}}")
	return researchflowparameterschema.Definition{Ref: ref, Version: "1.0.0", Document: document}, nil
}

type newsSearchIntegrationApplicability struct{}

func (newsSearchIntegrationApplicability) Evaluate(context.Context, researchflowtemplate.ApplicabilityRule, json.RawMessage) (bool, string, error) {
	return true, "", nil
}

type newsSearchIntegrationProvider struct {
	calls atomic.Int32
}

func (*newsSearchIntegrationProvider) Code() string { return "integration_fixture" }
func (p *newsSearchIntegrationProvider) Search(_ context.Context, _ newsdomain.SearchRequest) ([]newsdomain.Candidate, error) {
	if p.calls.Add(1) == 1 {
		return nil, &newsapp.ProviderError{Code: newsapp.ProviderErrorRateLimited}
	}
	return []newsdomain.Candidate{{
		Title:         "Deterministic News Stage integration candidate",
		RawURL:        "https://publisher.example.test/integration-candidate",
		PublisherName: "Integration Fixture Publisher",
	}}, nil
}

func TestNewsSearchStageResearchflowMariaDBIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// 复用现有隔离 MariaDB 测试夹具，再追加 Researchflow 自有 Migration；
	// 测试不访问外部 Provider，也不将 fixture 作为真实市场数据。
	db := openNewsResearchMariaDB(t, ctx)
	if err := platformstorage.ApplyMigrations(ctx, db, researchflowstorage.MigrationNamespace, researchflowstorage.Migrations()); err != nil {
		t.Fatalf("应用 Researchflow Migration 失败: %v", err)
	}

	clock := newsSearchIntegrationClock{db: db}
	ids := &newsSearchIntegrationIDs{}
	capacity := researchflowstorage.CapacityLimits{MaxQueuedRuns: 10, MaxWorkers: 2}
	repository, err := researchflowstorage.NewMySQLRunRepository(db, capacity)
	if err != nil {
		t.Fatalf("创建 Researchflow MySQL Repository 失败: %v", err)
	}
	templateRegistry, err := researchflowstorage.NewMySQLTemplateRegistry(db, nil)
	if err != nil {
		t.Fatalf("创建 Researchflow Template Registry 失败: %v", err)
	}

	retry := researchflowtemplate.RetryPolicy{
		Ref: "news-search-retry-v1", MaxAttempts: 2,
		RetryableReasonCodes: []string{"news_provider_retryable_failure"},
		Backoff: researchflowtemplate.BackoffPolicy{
			Algorithm: "exponential_full_jitter_v1", BaseDelayMS: 100, MaxDelayMS: 1000,
		},
		AttemptTimeoutMS: 30000,
	}
	propagation := researchflowtemplate.PropagationPolicy{
		OnDegraded: researchflowdomain.PropagateAccept,
		OnSkipped:  researchflowdomain.PropagateFail,
		OnFailed:   researchflowdomain.PropagateFail,
		OnTimedOut: researchflowdomain.PropagateFail,
		OnBlocked:  researchflowdomain.PropagateFail,
	}
	workflowTemplate := researchflowtemplate.WorkflowTemplate{
		Ref: newsSearchIntegrationTemplateRef, Version: "1.0.0",
		AllowEmptyInputs: true, ParameterSchemaRef: newsSearchIntegrationSchemaRef,
		OutputRegistryVersion: newsSearchStageOutputRegistryVersion,
		ResourcePolicyRef:     resourcepolicy.NewsSearchConfiguredProvidersRef,
		Stages: []researchflowtemplate.StageDefinition{{
			Code: newsstage.SearchCandidatesStageCode, Required: true, Dependencies: []string{},
			Applicability: researchflowtemplate.ApplicabilityRule{
				RuleRef: "integration.news.search_candidates.always", RuleVersion: "1.0.0",
			},
			Retry: retry, Propagation: propagation,
		}},
	}
	if err := templateRegistry.Publish(ctx, workflowTemplate); err != nil {
		t.Fatalf("发布隔离测试 Workflow Template 失败: %v", err)
	}

	parameterValidator, err := researchflowparameterschema.NewValidator(newsSearchIntegrationParameterRegistry{})
	if err != nil {
		t.Fatalf("创建参数 Schema Validator 失败: %v", err)
	}
	createRun, err := researchflowapplication.NewCreateRunService(
		newsSearchIntegrationIdentity{}, clock, 2*time.Minute, ids, templateRegistry, parameterValidator,
		newsSearchIntegrationApplicability{}, nil, newsSearchIntegrationResourcePolicyResolver{}, repository,
	)
	if err != nil {
		t.Fatalf("创建 Researchflow CreateRun Service 失败: %v", err)
	}
	created, err := createRun.CreateRun(ctx, researchflowdomain.CreateRunCommand{
		SchemaVersion: "1.0.0", IdempotencyKey: "news-search-stage-integration",
		TemplateRef: workflowTemplate.Ref, TemplateVersion: workflowTemplate.Version,
		InputSnapshotRefs: []string{},
		Parameters:        json.RawMessage("{\"query\":\"market\",\"query_dimension\":\"market\",\"limit\":10}"),
	})
	if err != nil {
		t.Fatalf("创建 Researchflow Run 失败: %v", err)
	}
	if created.Run.SystemAsOf.IsZero() || created.Run.DeadlineAt.Sub(created.Run.SystemAsOf) != 2*time.Minute {
		t.Fatalf("Run PIT/Deadline must be server-generated from one database instant: system_as_of=%s deadline_at=%s", created.Run.SystemAsOf, created.Run.DeadlineAt)
	}
	deadlineCreateRun, err := researchflowapplication.NewCreateRunService(
		newsSearchIntegrationIdentity{}, clock, time.Second, ids, templateRegistry, parameterValidator,
		newsSearchIntegrationApplicability{}, nil, newsSearchIntegrationResourcePolicyResolver{}, repository,
	)
	if err != nil {
		t.Fatalf("创建 Deadline 集成 CreateRun Service 失败: %v", err)
	}
	deadlineCreated, err := deadlineCreateRun.CreateRun(ctx, researchflowdomain.CreateRunCommand{
		SchemaVersion: "1.0.0", IdempotencyKey: "news-search-stage-deadline-integration",
		TemplateRef: workflowTemplate.Ref, TemplateVersion: workflowTemplate.Version,
		InputSnapshotRefs: []string{},
		Parameters:        json.RawMessage("{\"query\":\"deadline\",\"query_dimension\":\"market\",\"limit\":10}"),
	})
	if err != nil {
		t.Fatalf("创建短 Deadline Researchflow Run 失败: %v", err)
	}
	for {
		now, clockErr := clock.NowUTC(ctx)
		if clockErr != nil {
			t.Fatalf("读取数据库 UTC 时间等待 Deadline 到期失败: %v", clockErr)
		}
		if !now.Before(deadlineCreated.Run.DeadlineAt) {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("等待测试 Run Deadline 到期失败: %v", ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}

	fixtureProvider := &newsSearchIntegrationProvider{}
	composition, err := newNewsSearchStageCompositionWithProviders(newsapp.StaticProviderSet{
		Providers: []newsapp.Provider{fixtureProvider},
	})
	if err != nil {
		t.Fatalf("创建真实 News Stage 适配组合失败: %v", err)
	}
	runtimeClock, err := researchflowstorage.NewMySQLDatabaseClock(db)
	if err != nil {
		t.Fatalf("创建 Researchflow MariaDB UTC Clock 失败: %v", err)
	}
	runtimeComponent, err := newResearchflowRuntimeComponent(
		&researchflowComposition{
			newsStage: composition, runRepository: repository,
			clock: runtimeClock, ids: ids,
		},
		serverconfig.ResearchflowCapacity{
			Configured: true, MaxQueuedRuns: capacity.MaxQueuedRuns, MaxWorkers: capacity.MaxWorkers,
		},
		1,
	)
	if err != nil {
		t.Fatalf("创建 Researchflow 生产 Runtime 失败: %v", err)
	}
	if err := runtimeComponent.Start(ctx); err != nil {
		t.Fatalf("启动 Researchflow 生产 Runtime 失败: %v", err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		if err := runtimeComponent.Stop(stopCtx); err != nil {
			t.Errorf("停止 Researchflow 生产 Runtime 失败: %v", err)
		}
	}()

	// 通过生产周期扫描器完成正常 Run 的候选发现、调度、执行和 Finalization，
	// 同时让已过期 Run 走 Deadline Scanner / Deadline Finalizer 路径。
	for {
		normalRun, loadErr := repository.GetRun(ctx, created.Run.Ref)
		if loadErr != nil {
			t.Fatalf("轮询正常 Researchflow Runtime Run 状态失败: %v", loadErr)
		}
		expiredRun, loadErr := repository.GetRun(ctx, deadlineCreated.Run.Ref)
		if loadErr != nil {
			t.Fatalf("轮询 Deadline Researchflow Runtime Run 状态失败: %v", loadErr)
		}
		if normalRun.Run.Status.Terminal() && expiredRun.Run.Status.Terminal() {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("Researchflow Runtime 未在截止时间内完成正常与 Deadline Run: %v", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}

	loaded, err := repository.GetRun(ctx, created.Run.Ref)
	if err != nil {
		t.Fatalf("读取 Researchflow Runtime 完成后的 Run 失败: %v", err)
	}
	deadlineLoaded, err := repository.GetRun(ctx, deadlineCreated.Run.Ref)
	if err != nil {
		t.Fatalf("读取 Deadline Runtime 完成后的 Run 失败: %v", err)
	}
	if deadlineLoaded.Run.Status != researchflowdomain.RunFailed ||
		deadlineLoaded.Run.TerminationReason != "deadline_exceeded" ||
		deadlineLoaded.Run.ResultSummaryRef == nil || len(deadlineLoaded.Stages) != 1 ||
		deadlineLoaded.Stages[0].Status != researchflowdomain.StageTimedOut {
		t.Fatalf("超期 Run 未走 Deadline Finalizer 正确终止: run=%+v stages=%+v", deadlineLoaded.Run, deadlineLoaded.Stages)
	}
	var deadlineOutboxCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM researchflow_outbox
		WHERE run_ref = ? AND event_type = 'researchflow.run.terminal'`, deadlineCreated.Run.Ref).Scan(&deadlineOutboxCount); err != nil {
		t.Fatalf("读取 Deadline 终态 Outbox 失败: %v", err)
	}
	if deadlineOutboxCount != 1 {
		t.Fatalf("Deadline Finalization 应原子生成一条终态 Outbox，实际=%d", deadlineOutboxCount)
	}
	if loaded.Run.Status != researchflowdomain.RunSucceeded || loaded.Run.ResultSummaryRef == nil {
		t.Fatalf("Researchflow Runtime 未完成正常 Run Finalization: %+v", loaded.Run)
	}
	if len(loaded.Stages) != 1 {
		t.Fatalf("Run Stage 数量=%d，期望 1", len(loaded.Stages))
	}
	stage := loaded.Stages[0]
	if stage.Status != researchflowdomain.StageSucceeded || stage.OutputRef == nil || stage.QualitySummaryRef == nil {
		t.Fatalf("News Search Stage 状态或输出引用不完整: %+v", stage)
	}
	rows, err := db.QueryContext(ctx, `SELECT attempt_ref, attempt_no FROM researchflow_attempts
		WHERE run_ref = ? AND stage_ref = ? ORDER BY attempt_no`, created.Run.Ref, stage.Ref)
	if err != nil {
		t.Fatalf("查询 Runtime Attempts 失败: %v", err)
	}
	type attemptIdentity struct {
		ref string
		no  int
	}
	attempts := make([]attemptIdentity, 0, 2)
	for rows.Next() {
		var item attemptIdentity
		if err := rows.Scan(&item.ref, &item.no); err != nil {
			_ = rows.Close()
			t.Fatalf("读取 Runtime Attempt 身份失败: %v", err)
		}
		attempts = append(attempts, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		t.Fatalf("遍历 Runtime Attempts 失败: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("关闭 Runtime Attempts 查询失败: %v", err)
	}
	if len(attempts) != 2 || attempts[0].no != 1 || attempts[1].no != 2 || stage.AttemptNo != 2 {
		t.Fatalf("RateLimited 首次执行应触发一次重试，attempts=%+v stage=%+v", attempts, stage)
	}
	firstAttempt, err := repository.GetAttempt(ctx, attempts[0].ref)
	if err != nil {
		t.Fatalf("读取首次 Researchflow Attempt 失败: %v", err)
	}
	secondAttempt, err := repository.GetAttempt(ctx, attempts[1].ref)
	if err != nil {
		t.Fatalf("读取重试 Researchflow Attempt 失败: %v", err)
	}
	if firstAttempt.Status != researchflowdomain.AttemptFailed || firstAttempt.OutputRef == nil {
		t.Fatalf("首次 RateLimited Attempt 必须保留失败 Provider 输出证据: %+v", firstAttempt)
	}
	firstOutput, err := repository.GetStageOutputResource(ctx, *firstAttempt.OutputRef)
	if err != nil {
		t.Fatalf("读取首次 RateLimited Attempt 输出证据失败: %v", err)
	}
	var firstPayload newsstage.SearchCandidatesOutput
	if err := json.Unmarshal(firstOutput.Content, &firstPayload); err != nil {
		t.Fatalf("解析首次 RateLimited Attempt 输出证据失败: %v", err)
	}
	if firstPayload.Status != newsdomain.SearchFailed || len(firstPayload.Outcomes) != 1 ||
		!firstPayload.Outcomes[0].Retryable || firstPayload.Outcomes[0].ErrorCode != newsapp.ProviderErrorRateLimited {
		t.Fatalf("首次 Attempt 未保留可重试限流证据: %+v", firstPayload)
	}
	if secondAttempt.Status != researchflowdomain.AttemptSucceeded || secondAttempt.OutputRef == nil ||
		*secondAttempt.OutputRef != *stage.OutputRef {
		t.Fatalf("重试 Attempt 与 Stage 输出引用不一致: %+v / %+v", secondAttempt, stage)
	}
	if fixtureProvider.calls.Load() != 2 {
		t.Fatalf("News Provider 应执行首次请求与一次重试，实际调用=%d", fixtureProvider.calls.Load())
	}
	var outboxCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM researchflow_outbox
		WHERE run_ref = ? AND event_type = 'researchflow.run.terminal'`, created.Run.Ref).Scan(&outboxCount); err != nil {
		t.Fatalf("读取 Run 终态 Outbox 失败: %v", err)
	}
	if outboxCount != 1 {
		t.Fatalf("正常 Run Finalization 应原子生成一条终态 Outbox，实际=%d", outboxCount)
	}

	payloadResource, err := repository.GetStageOutputResource(ctx, *stage.OutputRef)
	if err != nil {
		t.Fatalf("读取不可变 Stage Payload 资源失败: %v", err)
	}
	qualityResource, err := repository.GetStageOutputResource(ctx, *stage.QualitySummaryRef)
	if err != nil {
		t.Fatalf("读取不可变 Stage QualitySummary 资源失败: %v", err)
	}
	if payloadResource.Kind != "payload" || qualityResource.Kind != "quality_summary" ||
		payloadResource.StageCode != newsstage.SearchCandidatesStageCode ||
		payloadResource.RegistryVersion != newsSearchStageOutputRegistryVersion ||
		payloadResource.OutputType != newsstage.SearchCandidatesOutputType ||
		payloadResource.SchemaVersion != newsstage.SearchCandidatesSchemaVersion {
		t.Fatalf("Stage Output 资源元数据不符合冻结注册项: payload=%+v quality=%+v", payloadResource, qualityResource)
	}

	var payload newsstage.SearchCandidatesOutput
	if err := json.Unmarshal(payloadResource.Content, &payload); err != nil {
		t.Fatalf("解析持久化 News Payload 失败: %v", err)
	}
	if payload.Status != newsdomain.SearchSuccess || len(payload.Candidates) != 1 ||
		payload.Candidates[0].ProviderCode != "integration_fixture" {
		t.Fatalf("持久化 News Payload 内容不符合预期: %+v", payload)
	}
	var quality newsstage.SearchCandidatesQualitySummary
	if err := json.Unmarshal(qualityResource.Content, &quality); err != nil {
		t.Fatalf("解析持久化 News QualitySummary 失败: %v", err)
	}
	if quality.ProviderCount != 1 || quality.ProviderSuccessCount != 1 || quality.CandidateCount != 1 {
		t.Fatalf("持久化 News QualitySummary 不符合预期: %+v", quality)
	}
}
