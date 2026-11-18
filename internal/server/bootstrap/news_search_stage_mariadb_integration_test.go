package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
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
	serverrelay "example.com/stock-analysis-flow/internal/server/relay"
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

type newsSearchIntegrationNoopDispatcher struct{}

func (newsSearchIntegrationNoopDispatcher) Execute(context.Context, string, string, string) (researchflowapplication.WorkerExecution, error) {
	return researchflowapplication.WorkerExecution{}, nil
}

type newsSearchIntegrationOutboxStore struct {
	repository *researchflowstorage.MySQLRunRepository
	runRef     string
}

func (s newsSearchIntegrationOutboxStore) ClaimOutbox(
	ctx context.Context, workerID, _ string, limit int, leaseDuration time.Duration,
) ([]researchflowapplication.OutboxMessage, error) {
	return s.repository.ClaimOutbox(ctx, workerID, s.runRef, limit, leaseDuration)
}

func (s newsSearchIntegrationOutboxStore) AcknowledgeOutbox(
	ctx context.Context, outboxRef, workerID string, leaseGeneration uint64,
) (bool, error) {
	return s.repository.AcknowledgeOutbox(ctx, outboxRef, workerID, leaseGeneration)
}

func (s newsSearchIntegrationOutboxStore) RetryOutbox(
	ctx context.Context, outboxRef, workerID string, leaseGeneration uint64,
	nextAttemptAt time.Time, errorCode string,
) (bool, error) {
	return s.repository.RetryOutbox(ctx, outboxRef, workerID, leaseGeneration, nextAttemptAt, errorCode)
}

type newsSearchIntegrationNotificationReceiver struct {
	failFirst atomic.Bool
	mu        sync.Mutex
	events    []map[string]any
}

func (r *newsSearchIntegrationNotificationReceiver) Receive(ctx context.Context, raw []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.failFirst.CompareAndSwap(true, false) {
		return errors.New("temporary Notification receiver failure")
	}
	var event map[string]any
	if err := json.Unmarshal(raw, &event); err != nil {
		return err
	}
	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()
	return nil
}

type newsSearchIntegrationProvider struct {
	calls               atomic.Int32
	recoveryCalls       atomic.Int32
	cancellationStarted chan struct{}
	cancellationOnce    sync.Once
}

func (*newsSearchIntegrationProvider) Code() string { return "integration_fixture" }
func (p *newsSearchIntegrationProvider) Search(ctx context.Context, request newsdomain.SearchRequest) ([]newsdomain.Candidate, error) {
	if request.Query == "permanent-failure" {
		return nil, &newsapp.ProviderError{Code: newsapp.ProviderErrorUnauthorized}
	}
	if request.Query == "inflight-cancel" {
		p.cancellationOnce.Do(func() { close(p.cancellationStarted) })
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if request.Query == "expired-attempt-recovery" {
		p.recoveryCalls.Add(1)
		return []newsdomain.Candidate{{
			Title:         "Deterministic recovered News Stage candidate",
			RawURL:        "https://publisher.example.test/recovered-candidate",
			PublisherName: "Integration Fixture Publisher",
		}}, nil
	}
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

	cancelCreated, err := createRun.CreateRun(ctx, researchflowdomain.CreateRunCommand{
		SchemaVersion: "1.0.0", IdempotencyKey: "news-search-stage-cancel-integration",
		TemplateRef: workflowTemplate.Ref, TemplateVersion: workflowTemplate.Version,
		InputSnapshotRefs: []string{},
		Parameters:        json.RawMessage("{\"query\":\"cancel\",\"query_dimension\":\"market\",\"limit\":10}"),
	})
	if err != nil {
		t.Fatalf("创建待取消 Researchflow Run 失败: %v", err)
	}
	cancelService, err := researchflowapplication.NewCancelRunService(
		newsSearchIntegrationIdentity{}, repository, repository, repository,
	)
	if err != nil {
		t.Fatalf("创建 Researchflow CancelRun Service 失败: %v", err)
	}
	cancelResult, err := cancelService.CancelRun(ctx, cancelCreated.Run.Ref)
	if err != nil {
		t.Fatalf("在 Runtime 启动前提交用户取消意图失败: %v", err)
	}
	if !cancelResult.Started || cancelResult.Run.Status != researchflowdomain.RunCancelling ||
		len(cancelResult.Stages) != 1 || cancelResult.Stages[0].Status != researchflowdomain.StageCancelled {
		t.Fatalf("用户取消意图未正确持久化: %+v", cancelResult)
	}

	inFlightCancelCreated, err := createRun.CreateRun(ctx, researchflowdomain.CreateRunCommand{
		SchemaVersion: "1.0.0", IdempotencyKey: "news-search-stage-inflight-cancel-integration",
		TemplateRef: workflowTemplate.Ref, TemplateVersion: workflowTemplate.Version,
		InputSnapshotRefs: []string{},
		Parameters:        json.RawMessage("{\"query\":\"inflight-cancel\",\"query_dimension\":\"market\",\"limit\":10}"),
	})
	if err != nil {
		t.Fatalf("创建在途取消 Researchflow Run 失败: %v", err)
	}

	recoveryCreated, err := createRun.CreateRun(ctx, researchflowdomain.CreateRunCommand{
		SchemaVersion: "1.0.0", IdempotencyKey: "news-search-stage-expired-attempt-recovery-integration",
		TemplateRef: workflowTemplate.Ref, TemplateVersion: workflowTemplate.Version,
		InputSnapshotRefs: []string{},
		Parameters:        json.RawMessage("{\"query\":\"expired-attempt-recovery\",\"query_dimension\":\"market\",\"limit\":10}"),
	})
	if err != nil {
		t.Fatalf("创建 Expired Attempt 恢复 Researchflow Run 失败: %v", err)
	}
	permanentFailureCreated, err := createRun.CreateRun(ctx, researchflowdomain.CreateRunCommand{
		SchemaVersion: "1.0.0", IdempotencyKey: "news-search-stage-permanent-failure-integration",
		TemplateRef: workflowTemplate.Ref, TemplateVersion: workflowTemplate.Version,
		InputSnapshotRefs: []string{},
		Parameters:        json.RawMessage("{\"query\":\"permanent-failure\",\"query_dimension\":\"market\",\"limit\":10}"),
	})
	if err != nil {
		t.Fatalf("创建不可重试 Provider 失败 Researchflow Run 失败: %v", err)
	}
	recoveryScheduler, err := researchflowapplication.NewScheduler(
		repository, repository, clock, ids, nil, newsSearchIntegrationNoopDispatcher{},
		researchflowapplication.SchedulerConfig{MaxParallel: 1, MaxReplans: 1},
	)
	if err != nil {
		t.Fatalf("创建恢复测试用 Scheduler 失败: %v", err)
	}
	if _, err := recoveryScheduler.ScheduleRun(ctx, recoveryCreated.Run.Ref); err != nil {
		t.Fatalf("将恢复测试 Stage 推进到 Ready 失败: %v", err)
	}
	recoveryStageRef := recoveryCreated.Stages[0].Ref
	crashedAttempt, replayed, err := repository.ClaimAttempt(
		ctx, recoveryCreated.Run.Ref, recoveryStageRef,
		"integration:attempt:crashed-worker", "integration:worker:crashed",
		30*time.Second, 2, 2,
	)
	if err != nil {
		t.Fatalf("模拟进程崩溃前 Claim Attempt 失败: %v", err)
	}
	if replayed || crashedAttempt.Status != researchflowdomain.AttemptRunning {
		t.Fatalf("恢复测试必须建立全新的 Running Attempt: replayed=%t attempt=%+v", replayed, crashedAttempt)
	}
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_attempts SET
		lease_expires_at = DATE_FORMAT(DATE_SUB(UTC_TIMESTAMP(6), INTERVAL 1 SECOND), '%Y-%m-%dT%H:%i:%s.%f000Z')
		WHERE attempt_ref = ? AND status = 'running'`, crashedAttempt.Ref); err != nil {
		t.Fatalf("模拟崩溃 Worker Lease 过期失败: %v", err)
	}

	fixtureProvider := &newsSearchIntegrationProvider{cancellationStarted: make(chan struct{})}
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
	select {
	case <-fixtureProvider.cancellationStarted:
	case <-ctx.Done():
		t.Fatalf("Runtime 未开始执行在途取消 Provider: %v", ctx.Err())
	}
	inFlightCancelResult, err := cancelService.CancelRun(ctx, inFlightCancelCreated.Run.Ref)
	if err != nil {
		t.Fatalf("取消正在执行 Stage 的 Researchflow Run 失败: %v", err)
	}
	if !inFlightCancelResult.Started || inFlightCancelResult.Run.Status != researchflowdomain.RunCancelling ||
		len(inFlightCancelResult.Stages) != 1 || inFlightCancelResult.Stages[0].Status != researchflowdomain.StageRunning {
		t.Fatalf("在途取消必须保留 Running Stage，等待 Worker 确认停止: %+v", inFlightCancelResult)
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
		cancelledRun, loadErr := repository.GetRun(ctx, cancelCreated.Run.Ref)
		if loadErr != nil {
			t.Fatalf("轮询取消 Researchflow Runtime Run 状态失败: %v", loadErr)
		}
		inFlightCancelledRun, loadErr := repository.GetRun(ctx, inFlightCancelCreated.Run.Ref)
		if loadErr != nil {
			t.Fatalf("轮询在途取消 Researchflow Runtime Run 状态失败: %v", loadErr)
		}
		recoveredRun, loadErr := repository.GetRun(ctx, recoveryCreated.Run.Ref)
		if loadErr != nil {
			t.Fatalf("轮询 Expired Attempt 恢复 Researchflow Run 状态失败: %v", loadErr)
		}
		permanentFailureRun, loadErr := repository.GetRun(ctx, permanentFailureCreated.Run.Ref)
		if loadErr != nil {
			t.Fatalf("轮询不可重试 Provider 失败 Researchflow Run 状态失败: %v", loadErr)
		}
		if normalRun.Run.Status.Terminal() && expiredRun.Run.Status.Terminal() &&
			cancelledRun.Run.Status.Terminal() && inFlightCancelledRun.Run.Status.Terminal() &&
			recoveredRun.Run.Status.Terminal() && permanentFailureRun.Run.Status.Terminal() {
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
	cancelledLoaded, err := repository.GetRun(ctx, cancelCreated.Run.Ref)
	if err != nil {
		t.Fatalf("读取 Cancellation Runtime 完成后的 Run 失败: %v", err)
	}
	if cancelledLoaded.Run.Status != researchflowdomain.RunCancelled ||
		cancelledLoaded.Run.TerminationReason != "user_cancelled" ||
		cancelledLoaded.Run.ResultSummaryRef == nil || len(cancelledLoaded.Stages) != 1 ||
		cancelledLoaded.Stages[0].Status != researchflowdomain.StageCancelled {
		t.Fatalf("用户取消 Run 未走 Cancellation Finalizer 正确终止: run=%+v stages=%+v", cancelledLoaded.Run, cancelledLoaded.Stages)
	}
	var cancellationOutboxCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM researchflow_outbox
		WHERE run_ref = ? AND event_type = 'researchflow.run.terminal'`, cancelCreated.Run.Ref).Scan(&cancellationOutboxCount); err != nil {
		t.Fatalf("读取 Cancellation 终态 Outbox 失败: %v", err)
	}
	if cancellationOutboxCount != 1 {
		t.Fatalf("Cancellation Finalization 应原子生成一条终态 Outbox，实际=%d", cancellationOutboxCount)
	}
	inFlightCancelledLoaded, err := repository.GetRun(ctx, inFlightCancelCreated.Run.Ref)
	if err != nil {
		t.Fatalf("读取在途取消 Runtime 完成后的 Run 失败: %v", err)
	}
	if inFlightCancelledLoaded.Run.Status != researchflowdomain.RunCancelled ||
		inFlightCancelledLoaded.Run.TerminationReason != "user_cancelled" ||
		inFlightCancelledLoaded.Run.ResultSummaryRef == nil || len(inFlightCancelledLoaded.Stages) != 1 ||
		inFlightCancelledLoaded.Stages[0].Status != researchflowdomain.StageCancelled {
		t.Fatalf("在途取消未由 Worker 确认停止并完成 Finalization: run=%+v stages=%+v", inFlightCancelledLoaded.Run, inFlightCancelledLoaded.Stages)
	}
	var inFlightAttemptRef string
	if err := db.QueryRowContext(ctx, `SELECT attempt_ref FROM researchflow_attempts
		WHERE run_ref = ? ORDER BY attempt_no DESC LIMIT 1`, inFlightCancelCreated.Run.Ref).Scan(&inFlightAttemptRef); err != nil {
		t.Fatalf("读取在途取消 Attempt 引用失败: %v", err)
	}
	inFlightAttempt, err := repository.GetAttempt(ctx, inFlightAttemptRef)
	if err != nil {
		t.Fatalf("读取在途取消 Attempt 失败: %v", err)
	}
	if inFlightAttempt.Status != researchflowdomain.AttemptCancelled {
		t.Fatalf("Worker 未将已确认停止的在途 Attempt 标记为 Cancelled: %+v", inFlightAttempt)
	}
	var inFlightOutboxCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM researchflow_outbox
		WHERE run_ref = ? AND event_type = 'researchflow.run.terminal'`, inFlightCancelCreated.Run.Ref).Scan(&inFlightOutboxCount); err != nil {
		t.Fatalf("读取在途取消终态 Outbox 失败: %v", err)
	}
	if inFlightOutboxCount != 1 {
		t.Fatalf("在途 Cancellation Finalization 应原子生成一条终态 Outbox，实际=%d", inFlightOutboxCount)
	}
	recoveredLoaded, err := repository.GetRun(ctx, recoveryCreated.Run.Ref)
	if err != nil {
		t.Fatalf("读取 Expired Attempt 恢复后的 Run 失败: %v", err)
	}
	if recoveredLoaded.Run.Status != researchflowdomain.RunSucceeded ||
		recoveredLoaded.Run.ResultSummaryRef == nil || len(recoveredLoaded.Stages) != 1 ||
		recoveredLoaded.Stages[0].Status != researchflowdomain.StageSucceeded ||
		recoveredLoaded.Stages[0].AttemptNo != 2 {
		t.Fatalf("Expired Attempt 未重新 Claim 并完成 Run: run=%+v stages=%+v", recoveredLoaded.Run, recoveredLoaded.Stages)
	}
	recoveredFirstAttempt, err := repository.GetAttempt(ctx, crashedAttempt.Ref)
	if err != nil {
		t.Fatalf("读取崩溃前 Attempt 失败: %v", err)
	}
	var recoveredAttemptRef string
	if err := db.QueryRowContext(ctx, `SELECT attempt_ref FROM researchflow_attempts
		WHERE run_ref = ? ORDER BY attempt_no DESC LIMIT 1`, recoveryCreated.Run.Ref).Scan(&recoveredAttemptRef); err != nil {
		t.Fatalf("读取恢复后 Attempt 引用失败: %v", err)
	}
	recoveredSecondAttempt, err := repository.GetAttempt(ctx, recoveredAttemptRef)
	if err != nil {
		t.Fatalf("读取恢复后 Attempt 失败: %v", err)
	}
	if recoveredFirstAttempt.Status != researchflowdomain.AttemptTimedOut ||
		recoveredSecondAttempt.Status != researchflowdomain.AttemptSucceeded ||
		recoveredSecondAttempt.FencingToken <= recoveredFirstAttempt.FencingToken ||
		recoveredSecondAttempt.OutputRef == nil ||
		recoveredLoaded.Stages[0].OutputRef == nil ||
		*recoveredSecondAttempt.OutputRef != *recoveredLoaded.Stages[0].OutputRef {
		t.Fatalf("Expired Attempt Fencing/重新执行结果不正确: first=%+v second=%+v stage=%+v",
			recoveredFirstAttempt, recoveredSecondAttempt, recoveredLoaded.Stages[0])
	}
	var recoveryOutboxCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM researchflow_outbox
		WHERE run_ref = ? AND event_type = 'researchflow.run.terminal'`, recoveryCreated.Run.Ref).Scan(&recoveryOutboxCount); err != nil {
		t.Fatalf("读取恢复 Run 终态 Outbox 失败: %v", err)
	}
	if recoveryOutboxCount != 1 {
		t.Fatalf("崩溃恢复后的 Run Finalization 应原子生成一条终态 Outbox，实际=%d", recoveryOutboxCount)
	}

	permanentFailureLoaded, err := repository.GetRun(ctx, permanentFailureCreated.Run.Ref)
	if err != nil {
		t.Fatalf("读取不可重试 Provider 失败后的 Run 失败: %v", err)
	}
	if permanentFailureLoaded.Run.Status != researchflowdomain.RunFailed ||
		permanentFailureLoaded.Run.ResultSummaryRef == nil || len(permanentFailureLoaded.Stages) != 1 {
		t.Fatalf("不可重试 Provider 失败未正确终结 Run: run=%+v stages=%+v",
			permanentFailureLoaded.Run, permanentFailureLoaded.Stages)
	}
	permanentFailureStage := permanentFailureLoaded.Stages[0]
	if permanentFailureStage.Status != researchflowdomain.StageFailed || permanentFailureStage.AttemptNo != 1 ||
		permanentFailureStage.OutputRef == nil || permanentFailureStage.QualitySummaryRef == nil ||
		len(permanentFailureStage.ReasonCodes) != 1 ||
		permanentFailureStage.ReasonCodes[0] != "news_all_provider_requests_failed" {
		t.Fatalf("不可重试 Provider 失败 Stage 缺少状态或持久化诊断输出: %+v", permanentFailureStage)
	}
	var permanentFailureAttemptRef string
	if err := db.QueryRowContext(ctx, `SELECT attempt_ref FROM researchflow_attempts
		WHERE run_ref = ? ORDER BY attempt_no DESC LIMIT 1`, permanentFailureCreated.Run.Ref).Scan(&permanentFailureAttemptRef); err != nil {
		t.Fatalf("读取不可重试 Provider 失败 Attempt 引用失败: %v", err)
	}
	permanentFailureAttempt, err := repository.GetAttempt(ctx, permanentFailureAttemptRef)
	if err != nil {
		t.Fatalf("读取不可重试 Provider 失败 Attempt 失败: %v", err)
	}
	if permanentFailureAttempt.Status != researchflowdomain.AttemptFailed ||
		permanentFailureAttempt.OutputRef == nil ||
		*permanentFailureAttempt.OutputRef != *permanentFailureStage.OutputRef {
		t.Fatalf("不可重试 Provider 失败 Attempt 未保留 Stage Payload 引用: %+v", permanentFailureAttempt)
	}
	permanentFailurePayloadResource, err := repository.GetStageOutputResource(ctx, *permanentFailureStage.OutputRef)
	if err != nil {
		t.Fatalf("读取不可重试 Provider 失败 Payload 资源失败: %v", err)
	}
	var permanentFailurePayload newsstage.SearchCandidatesOutput
	if err := json.Unmarshal(permanentFailurePayloadResource.Content, &permanentFailurePayload); err != nil {
		t.Fatalf("解析不可重试 Provider 失败 Payload 失败: %v", err)
	}
	if permanentFailurePayload.Status != newsdomain.SearchFailed ||
		permanentFailurePayload.FailureCode != "all_provider_requests_failed" ||
		len(permanentFailurePayload.Outcomes) != 1 ||
		permanentFailurePayload.Outcomes[0].Status != newsdomain.ProviderFailed ||
		permanentFailurePayload.Outcomes[0].ErrorCode != newsapp.ProviderErrorUnauthorized ||
		permanentFailurePayload.Outcomes[0].Retryable {
		t.Fatalf("不可重试 Provider 失败 Outcome 未完整持久化: %+v", permanentFailurePayload)
	}
	permanentFailureQualityResource, err := repository.GetStageOutputResource(ctx, *permanentFailureStage.QualitySummaryRef)
	if err != nil {
		t.Fatalf("读取不可重试 Provider 失败 QualitySummary 资源失败: %v", err)
	}
	var permanentFailureQuality newsstage.SearchCandidatesQualitySummary
	if err := json.Unmarshal(permanentFailureQualityResource.Content, &permanentFailureQuality); err != nil {
		t.Fatalf("解析不可重试 Provider 失败 QualitySummary 失败: %v", err)
	}
	if permanentFailureQuality.ProviderCount != 1 || permanentFailureQuality.ProviderSuccessCount != 0 ||
		permanentFailureQuality.ProviderFailedCount != 1 || permanentFailureQuality.ProviderSkippedCount != 0 ||
		permanentFailureQuality.CandidateCount != 0 {
		t.Fatalf("不可重试 Provider 失败 QualitySummary 与 Outcome 不一致: %+v", permanentFailureQuality)
	}
	var permanentFailureOutboxCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM researchflow_outbox
		WHERE run_ref = ? AND event_type = 'researchflow.run.terminal'`, permanentFailureCreated.Run.Ref).Scan(&permanentFailureOutboxCount); err != nil {
		t.Fatalf("读取不可重试 Provider 失败 Run 终态 Outbox 失败: %v", err)
	}
	if permanentFailureOutboxCount != 1 {
		t.Fatalf("不可重试 Provider 失败 Run Finalization 应生成一条终态 Outbox，实际=%d", permanentFailureOutboxCount)
	}

	// 使用真实 MariaDB Outbox Store 和 Server Relay，验证发布、暂时失败重试及确认提交。
	receiver := &newsSearchIntegrationNotificationReceiver{}
	receiver.failFirst.Store(true)
	runRefs := []string{
		created.Run.Ref, deadlineCreated.Run.Ref, cancelCreated.Run.Ref,
		inFlightCancelCreated.Run.Ref, recoveryCreated.Run.Ref, permanentFailureCreated.Run.Ref,
	}
	for index, runRef := range runRefs {
		relay, err := serverrelay.NewResearchflowNotificationRelay(
			newsSearchIntegrationOutboxStore{repository: repository, runRef: runRef},
			receiver,
			serverrelay.ResearchflowNotificationConfig{
				WorkerID:  "integration:notification-relay:" + runRef,
				BatchSize: 1, PollInterval: 10 * time.Millisecond,
				LeaseDuration: time.Second, PublishTimeout: 500 * time.Millisecond,
			},
			nil,
		)
		if err != nil {
			t.Fatalf("创建 Researchflow Notification Relay 失败: %v", err)
		}
		firstDispatch, err := relay.ProcessOnce(ctx)
		if err != nil {
			t.Fatalf("执行 Researchflow Outbox 首次投递失败: %v", err)
		}
		if firstDispatch.Claimed != 1 {
			t.Fatalf("Run %s 应恰好 Claim 一条终态 Outbox，实际=%+v", runRef, firstDispatch)
		}
		if index == 0 {
			if firstDispatch.Retried != 1 || firstDispatch.Published != 0 {
				t.Fatalf("首次 Notification 暂时失败应进入 Outbox Retry：%+v", firstDispatch)
			}
			retryDeadline := time.Now().Add(10 * time.Second)
			for {
				retryDispatch, retryErr := relay.ProcessOnce(ctx)
				if retryErr != nil {
					t.Fatalf("执行 Researchflow Outbox 重试失败: %v", retryErr)
				}
				if retryDispatch.Published == 1 {
					break
				}
				if time.Now().After(retryDeadline) {
					t.Fatalf("暂时失败的 Outbox 未在重试窗口内重新投递: %+v", retryDispatch)
				}
				select {
				case <-ctx.Done():
					t.Fatalf("等待 Outbox 重试成功失败: %v", ctx.Err())
				case <-time.After(50 * time.Millisecond):
				}
			}
		} else if firstDispatch.Published != 1 || firstDispatch.Retried != 0 {
			t.Fatalf("Run %s 的终态 Outbox 未成功投递: %+v", runRef, firstDispatch)
		}
	}

	receiver.mu.Lock()
	receivedEvents := append([]map[string]any(nil), receiver.events...)
	receiver.mu.Unlock()
	if len(receivedEvents) != len(runRefs) {
		t.Fatalf("Notification EventReceiver 应收到全部终态事件，实际=%d 期望=%d", len(receivedEvents), len(runRefs))
	}
	seenIdempotencyKeys := make(map[string]struct{}, len(receivedEvents))
	for _, event := range receivedEvents {
		if event["event_type"] != "researchflow.run.terminal" {
			t.Fatalf("收到非预期 Notification EventType: %+v", event)
		}
		key, ok := event["idempotency_key"].(string)
		if !ok || key == "" {
			t.Fatalf("Notification Envelope 缺少 IdempotencyKey: %+v", event)
		}
		if _, exists := seenIdempotencyKeys[key]; exists {
			t.Fatalf("Notification Envelope IdempotencyKey 重复: %s", key)
		}
		seenIdempotencyKeys[key] = struct{}{}
	}
	for index, runRef := range runRefs {
		var publishedAt sql.NullString
		var attemptCount uint32
		var leaseOwner, leaseExpires sql.NullString
		if err := db.QueryRowContext(ctx, `SELECT published_at, attempt_count, lease_owner, lease_expires_at
			FROM researchflow_outbox WHERE run_ref = ?`, runRef).Scan(
			&publishedAt, &attemptCount, &leaseOwner, &leaseExpires,
		); err != nil {
			t.Fatalf("读取已投递 Outbox 状态失败（Run %s）: %v", runRef, err)
		}
		expectedAttempts := uint32(1)
		if index == 0 {
			expectedAttempts = 2
		}
		if !publishedAt.Valid || attemptCount != expectedAttempts || leaseOwner.Valid || leaseExpires.Valid {
			t.Fatalf("Outbox 投递确认/重试状态不正确（Run %s）：published=%+v attempts=%d owner=%+v lease=%+v",
				runRef, publishedAt, attemptCount, leaseOwner, leaseExpires)
		}
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
	if fixtureProvider.calls.Load() != 2 || fixtureProvider.recoveryCalls.Load() != 1 {
		t.Fatalf("正常 Run 应调用两次（首次失败后重试），崩溃恢复 Run 应执行一次：normal=%d recovery=%d",
			fixtureProvider.calls.Load(), fixtureProvider.recoveryCalls.Load())
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
