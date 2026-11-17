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

type newsSearchIntegrationProvider struct{}

func (newsSearchIntegrationProvider) Code() string { return "integration_fixture" }
func (newsSearchIntegrationProvider) Search(context.Context, newsdomain.SearchRequest) ([]newsdomain.Candidate, error) {
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

	composition, err := newNewsSearchStageCompositionWithProviders(newsapp.StaticProviderSet{
		Providers: []newsapp.Provider{newsSearchIntegrationProvider{}},
	})
	if err != nil {
		t.Fatalf("创建真实 News Stage 适配组合失败: %v", err)
	}
	worker, err := researchflowapplication.NewWorkerWithStageOutputRegistry(
		repository, composition.executor, clock,
		researchflowapplication.WorkerConfig{
			WorkerID:      "integration-news-search-worker",
			LeaseDuration: 30 * time.Second, HeartbeatInterval: 5 * time.Second,
			MaxParallel: capacity.MaxWorkers,
		},
		composition.registry,
	)
	if err != nil {
		t.Fatalf("创建 Researchflow Worker 失败: %v", err)
	}
	pool, err := researchflowapplication.NewWorkerPool(worker, researchflowapplication.WorkerPoolConfig{Workers: 1})
	if err != nil {
		t.Fatalf("创建 Researchflow WorkerPool 失败: %v", err)
	}
	if err := pool.Start(ctx); err != nil {
		t.Fatalf("启动 Researchflow WorkerPool 失败: %v", err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		if err := pool.Shutdown(stopCtx); err != nil {
			t.Errorf("停止 Researchflow WorkerPool 失败: %v", err)
		}
	}()

	scheduler, err := researchflowapplication.NewScheduler(
		repository, repository, clock, ids, nil, pool,
		researchflowapplication.SchedulerConfig{MaxParallel: 1, MaxReplans: 3},
	)
	if err != nil {
		t.Fatalf("创建 Researchflow Scheduler 失败: %v", err)
	}
	scheduled, err := scheduler.ScheduleRun(ctx, created.Run.Ref)
	if err != nil {
		t.Fatalf("调度 News Search Stage 失败: %v", err)
	}
	if len(scheduled.Executions) != 1 || !scheduled.Executions[0].Execution.Committed {
		t.Fatalf("News Search Stage 未完成持久化提交: %+v", scheduled)
	}

	loaded, err := repository.GetRun(ctx, created.Run.Ref)
	if err != nil {
		t.Fatalf("读取 Researchflow Run 失败: %v", err)
	}
	if len(loaded.Stages) != 1 {
		t.Fatalf("Run Stage 数量=%d，期望 1", len(loaded.Stages))
	}
	stage := loaded.Stages[0]
	if stage.Status != researchflowdomain.StageSucceeded || stage.OutputRef == nil || stage.QualitySummaryRef == nil {
		t.Fatalf("News Search Stage 状态或输出引用不完整: %+v", stage)
	}
	attempt, err := repository.GetAttempt(ctx, scheduled.Executions[0].AttemptRef)
	if err != nil {
		t.Fatalf("读取 Researchflow Attempt 失败: %v", err)
	}
	if attempt.Status != researchflowdomain.AttemptSucceeded || attempt.OutputRef == nil || *attempt.OutputRef != *stage.OutputRef {
		t.Fatalf("Attempt 与 Stage 输出引用不一致: %+v / %+v", attempt, stage)
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
