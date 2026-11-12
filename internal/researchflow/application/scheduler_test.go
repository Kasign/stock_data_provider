package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/state"
)

type dependencyValidatorFunc func(context.Context, domain.Stage, domain.Stage) (bool, error)

func (f dependencyValidatorFunc) ValidateDependency(ctx context.Context, upstream, downstream domain.Stage) (bool, error) {
	return f(ctx, upstream, downstream)
}

func schedulerFixture() (domain.Run, time.Time) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	return domain.Run{Ref: "run-1", Status: domain.RunRunning, Version: 2, DeadlineAt: now.Add(time.Hour)}, now
}

func TestBuildSchedulePlanPromotesOnlyValidatedDependenciesAndRespectsConcurrency(t *testing.T) {
	run, now := schedulerFixture()
	output := "snapshot:upstream"
	stages := []domain.Stage{
		{Ref: "stage-a", RunRef: run.Ref, Code: "market", Status: domain.StageSucceeded, Version: 2, OutputRef: &output},
		{Ref: "stage-b", RunRef: run.Ref, Code: "industry", Status: domain.StagePending, Version: 1, Dependencies: []string{"stage-a"}},
		{Ref: "stage-c", RunRef: run.Ref, Code: "stock", Status: domain.StagePending, Version: 1},
		{Ref: "stage-d", RunRef: run.Ref, Code: "already-running", Status: domain.StageRunning},
	}
	validated := 0
	plan, err := BuildSchedulePlan(context.Background(), run, stages,
		map[string]int{"stage-b": 10, "stage-c": 1}, 2, now,
		dependencyValidatorFunc(func(_ context.Context, upstream, downstream domain.Stage) (bool, error) {
			validated++
			if upstream.Ref != "stage-a" || downstream.Ref != "stage-b" {
				t.Fatalf("unexpected dependency validation: %s -> %s", upstream.Ref, downstream.Ref)
			}
			return true, nil
		}))
	if err != nil {
		t.Fatal(err)
	}
	if validated != 1 || plan.ActiveCount != 1 || plan.AvailableSlots != 1 {
		t.Fatalf("unexpected validation/capacity: validated=%d plan=%#v", validated, plan)
	}
	if len(plan.PromoteToReady) != 2 || len(plan.Dispatch) != 1 || plan.Dispatch[0].Ref != "stage-c" {
		t.Fatalf("unexpected deterministic plan: %#v", plan)
	}
	if plan.PromoteToReady[0].Status != domain.StageReady || plan.PromoteToReady[1].Status != domain.StageReady {
		t.Fatalf("pending stages were not promoted: %#v", plan.PromoteToReady)
	}
}

func TestBuildSchedulePlanBlocksDependencyWithoutUsableOutput(t *testing.T) {
	run, now := schedulerFixture()
	stages := []domain.Stage{
		{Ref: "stage-a", RunRef: run.Ref, Code: "market", Status: domain.StageFailed},
		{Ref: "stage-b", RunRef: run.Ref, Code: "industry", Status: domain.StagePending, Dependencies: []string{"stage-a"}},
	}
	plan, err := BuildSchedulePlan(context.Background(), run, stages, nil, 2, now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.MarkBlocked) != 1 || plan.MarkBlocked[0].Status != domain.StageBlocked ||
		len(plan.MarkBlocked[0].ReasonCodes) != 1 || plan.MarkBlocked[0].ReasonCodes[0] != "dependency_not_satisfied" {
		t.Fatalf("dependency failure was not blocked: %#v", plan)
	}
}

func TestBuildSchedulePlanWaitsForNonTerminalDependency(t *testing.T) {
	run, now := schedulerFixture()
	stages := []domain.Stage{
		{Ref: "stage-a", RunRef: run.Ref, Code: "market", Status: domain.StageRunning},
		{Ref: "stage-b", RunRef: run.Ref, Code: "industry", Status: domain.StagePending, Dependencies: []string{"stage-a"}},
	}
	plan, err := BuildSchedulePlan(context.Background(), run, stages, nil, 2, now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.WaitingRefs) != 1 || plan.WaitingRefs[0] != "stage-b" || len(plan.PromoteToReady) != 0 {
		t.Fatalf("non-terminal dependency was not held: %#v", plan)
	}
}

func TestBuildSchedulePlanRejectsInvalidRunAndMissingDependency(t *testing.T) {
	run, now := schedulerFixture()
	cancelling := run
	cancelling.Status = domain.RunCancelling
	if _, err := BuildSchedulePlan(context.Background(), cancelling, nil, nil, 1, now, nil); err == nil {
		t.Fatal("cancelling Run must not be scheduled")
	}
	stages := []domain.Stage{{Ref: "stage-a", RunRef: run.Ref, Code: "industry", Status: domain.StagePending, Dependencies: []string{"missing"}}}
	if _, err := BuildSchedulePlan(context.Background(), run, stages, nil, 1, now, nil); err == nil {
		t.Fatal("missing dependency reference must fail closed")
	}
	if _, err := BuildSchedulePlan(context.Background(), run, nil, nil, 0, now, nil); err == nil {
		t.Fatal("zero max_parallel must be rejected")
	}
}

func TestBuildSchedulePlanRequiresValidatorForSuccessfulDependency(t *testing.T) {
	run, now := schedulerFixture()
	output := "snapshot:upstream"
	stages := []domain.Stage{
		{Ref: "stage-a", RunRef: run.Ref, Code: "market", Status: domain.StageSucceeded, OutputRef: &output},
		{Ref: "stage-b", RunRef: run.Ref, Code: "industry", Status: domain.StagePending, Dependencies: []string{"stage-a"}},
	}
	if _, err := BuildSchedulePlan(context.Background(), run, stages, nil, 1, now, nil); err == nil {
		t.Fatal("successful dependency without validator must fail closed")
	}
	boom := errors.New("schema registry unavailable")
	_, err := BuildSchedulePlan(context.Background(), run, stages, nil, 1, now,
		dependencyValidatorFunc(func(context.Context, domain.Stage, domain.Stage) (bool, error) { return false, boom }))
	if !errors.Is(err, boom) {
		t.Fatalf("validator error=%v, want %v", err, boom)
	}
}

func TestBuildSchedulePlanRejectsExpiredRunDeadline(t *testing.T) {
	run, now := schedulerFixture()
	run.DeadlineAt = now
	if _, err := BuildSchedulePlan(context.Background(), run, nil, nil, 1, now, nil); !errors.Is(err, state.ErrDeadlineExceeded) {
		t.Fatal("expired Run deadline must be rejected")
	}
}

type schedulePlanStoreFake struct {
	applied    []domain.Stage
	conflictAt int
}

func (s *schedulePlanStoreFake) CompareAndSwapStage(_ context.Context, expected uint64, next domain.Stage) (bool, error) {
	if s.conflictAt > 0 && len(s.applied)+1 == s.conflictAt {
		return false, nil
	}
	if next.Version != expected+1 {
		return false, errors.New("invalid expected version")
	}
	s.applied = append(s.applied, next)
	return true, nil
}

func TestBuildSchedulePlanBlocksWhenAnyDependencyFailedEvenIfAnotherIsRunning(t *testing.T) {
	run, now := schedulerFixture()
	stages := []domain.Stage{
		{Ref: "stage-running", RunRef: run.Ref, Code: "running", Status: domain.StageRunning},
		{Ref: "stage-failed", RunRef: run.Ref, Code: "failed", Status: domain.StageFailed},
		{Ref: "stage-downstream", RunRef: run.Ref, Code: "downstream", Status: domain.StagePending,
			Dependencies: []string{"stage-running", "stage-failed"}},
	}
	plan, err := BuildSchedulePlan(context.Background(), run, stages, nil, 2, now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.MarkBlocked) != 1 || plan.MarkBlocked[0].Ref != "stage-downstream" ||
		len(plan.WaitingRefs) != 0 {
		t.Fatalf("failed dependency must dominate non-terminal dependency: %#v", plan)
	}
}

func TestPersistSchedulePlanUsesCASAndReportsPartialProgress(t *testing.T) {
	store := &schedulePlanStoreFake{conflictAt: 2}
	plan := SchedulePlan{
		PromoteToReady: []domain.Stage{
			{Ref: "stage-b", Version: 4, Status: domain.StageReady},
			{Ref: "stage-a", Version: 3, Status: domain.StageReady},
		},
		MarkBlocked: []domain.Stage{{Ref: "stage-c", Version: 7, Status: domain.StageBlocked}},
	}
	result, err := PersistSchedulePlan(context.Background(), store, plan)
	if !errors.Is(err, ErrSchedulePlanConflict) {
		t.Fatalf("persist error=%v, want schedule conflict", err)
	}
	if result.Promoted != 1 || result.Blocked != 0 || len(store.applied) != 1 ||
		store.applied[0].Ref != "stage-a" {
		t.Fatalf("unexpected partial CAS result=%#v applied=%#v", result, store.applied)
	}
}

func TestPersistSchedulePlanRejectsDuplicateAndInvalidTransitions(t *testing.T) {
	store := &schedulePlanStoreFake{}
	_, err := PersistSchedulePlan(context.Background(), store, SchedulePlan{
		PromoteToReady: []domain.Stage{{Ref: "same", Version: 2, Status: domain.StageReady}},
		MarkBlocked:    []domain.Stage{{Ref: "same", Version: 3, Status: domain.StageBlocked}},
	})
	if err == nil {
		t.Fatal("duplicate Stage transitions must be rejected")
	}
	_, err = PersistSchedulePlan(context.Background(), store, SchedulePlan{
		PromoteToReady: []domain.Stage{{Ref: "stage", Version: 1, Status: domain.StageRunning}},
	})
	if err == nil {
		t.Fatal("unsupported transition target must be rejected")
	}
}

func TestBuildSchedulePlanDispatchesDueRetryWaitWithoutPrematureStateChange(t *testing.T) {
	run, now := schedulerFixture()
	due := now
	future := now.Add(time.Minute)
	stages := []domain.Stage{
		{Ref: "stage-retry-due", RunRef: run.Ref, Code: "retry_due", Status: domain.StageRetryWait, Version: 4, AttemptNo: 1, RetryAt: &due},
		{Ref: "stage-retry-future", RunRef: run.Ref, Code: "retry_future", Status: domain.StageRetryWait, Version: 4, AttemptNo: 1, RetryAt: &future},
	}
	plan, err := BuildSchedulePlan(context.Background(), run, stages, nil, 1, now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Dispatch) != 1 || plan.Dispatch[0].Ref != "stage-retry-due" ||
		plan.Dispatch[0].Status != domain.StageRetryWait {
		t.Fatalf("due retry was not dispatched in retry_wait state: %#v", plan.Dispatch)
	}
	if len(plan.WaitingRefs) != 1 || plan.WaitingRefs[0] != "stage-retry-future" {
		t.Fatalf("future retry should remain waiting: %#v", plan.WaitingRefs)
	}
	if len(plan.PromoteToReady) != 0 || len(plan.MarkBlocked) != 0 {
		t.Fatalf("retry scheduling must not perform an intermediate state transition: %#v", plan)
	}
}

func TestBuildSchedulePlanRejectsRetryWaitWithoutRetryAt(t *testing.T) {
	run, now := schedulerFixture()
	stages := []domain.Stage{
		{Ref: "stage-retry-invalid", RunRef: run.Ref, Code: "retry_invalid", Status: domain.StageRetryWait, Version: 2, AttemptNo: 1},
	}
	if _, err := BuildSchedulePlan(context.Background(), run, stages, nil, 1, now, nil); err == nil {
		t.Fatal("retry_wait Stage without retry_at must fail closed")
	}
}

type schedulerRunReaderFake struct {
	results []CreateRunResult
	calls   int
}

func (f *schedulerRunReaderFake) GetRun(_ context.Context, runRef string) (CreateRunResult, error) {
	if len(f.results) == 0 {
		return CreateRunResult{}, errors.New("no scheduler snapshot")
	}
	index := f.calls
	if index >= len(f.results) {
		index = len(f.results) - 1
	}
	f.calls++
	result := f.results[index]
	if result.Run.Ref != runRef {
		return CreateRunResult{}, errors.New("unexpected Run reference")
	}
	return result, nil
}

type schedulerClockFake struct {
	now time.Time
}

func (f schedulerClockFake) NowUTC(context.Context) (time.Time, error) {
	return f.now, nil
}

type schedulerIDGeneratorFake struct {
	refs  []string
	calls int
}

func (f *schedulerIDGeneratorFake) NewRef(_ context.Context, _ string) (string, error) {
	if f.calls >= len(f.refs) {
		return "", errors.New("no Attempt reference configured")
	}
	value := f.refs[f.calls]
	f.calls++
	return value, nil
}

type schedulerDispatcherFake struct {
	calls []ScheduledStageExecution
	err   error
}

func (f *schedulerDispatcherFake) Execute(_ context.Context, runRef, stageRef, attemptRef string) (WorkerExecution, error) {
	f.calls = append(f.calls, ScheduledStageExecution{StageRef: stageRef, AttemptRef: attemptRef})
	if f.err != nil {
		return WorkerExecution{}, f.err
	}
	return WorkerExecution{Attempt: domain.Attempt{Ref: attemptRef, StageRef: stageRef}}, nil
}

type schedulerReplanStoreFake struct {
	calls          int
	conflictFirst  bool
	alwaysConflict bool
	applied        []domain.Stage
}

func (f *schedulerReplanStoreFake) CompareAndSwapStage(_ context.Context, expected uint64, next domain.Stage) (bool, error) {
	f.calls++
	if f.alwaysConflict || (f.conflictFirst && f.calls == 1) {
		return false, nil
	}
	if next.Version != expected+1 {
		return false, errors.New("invalid expected version")
	}
	f.applied = append(f.applied, next)
	return true, nil
}

func TestBuildSchedulePlanAllowsInitialRunStates(t *testing.T) {
	now := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	for _, status := range []domain.RunStatus{domain.RunCreated, domain.RunQueued, domain.RunRunning} {
		run := domain.Run{Ref: "run-initial", Status: status, Version: 1, DeadlineAt: now.Add(time.Hour)}
		stage := domain.Stage{Ref: "stage-initial", RunRef: run.Ref, Code: "market", Status: domain.StagePending, Version: 1}
		plan, err := BuildSchedulePlan(context.Background(), run, []domain.Stage{stage}, nil, 1, now, nil)
		if err != nil {
			t.Fatalf("status %q should be schedulable: %v", status, err)
		}
		if len(plan.PromoteToReady) != 1 || len(plan.Dispatch) != 1 || plan.Dispatch[0].Status != domain.StageReady {
			t.Fatalf("status %q did not produce initial dispatch: %#v", status, plan)
		}
	}
	run := domain.Run{Ref: "run-cancelling", Status: domain.RunCancelling, Version: 1, DeadlineAt: now.Add(time.Hour)}
	if _, err := BuildSchedulePlan(context.Background(), run, nil, nil, 1, now, nil); err == nil {
		t.Fatal("cancelling Run must not be schedulable")
	}
}

func TestSchedulerReplansAfterStageCASConflictAndDispatches(t *testing.T) {
	run, now := schedulerFixture()
	run.Status = domain.RunCreated
	pending := domain.Stage{Ref: "stage-a", RunRef: run.Ref, Code: "market", Status: domain.StagePending, Version: 1}
	ready := pending
	ready.Status = domain.StageReady
	ready.Version = 2
	reader := &schedulerRunReaderFake{results: []CreateRunResult{
		{Run: run, Stages: []domain.Stage{pending}},
		{Run: run, Stages: []domain.Stage{ready}},
	}}
	store := &schedulerReplanStoreFake{conflictFirst: true}
	ids := &schedulerIDGeneratorFake{refs: []string{"attempt-a"}}
	dispatcher := &schedulerDispatcherFake{}
	scheduler, err := NewScheduler(reader, store, schedulerClockFake{now: now}, ids, nil, dispatcher,
		SchedulerConfig{MaxParallel: 1, MaxReplans: 2})
	if err != nil {
		t.Fatal(err)
	}

	result, err := scheduler.ScheduleRun(context.Background(), run.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if reader.calls != 2 || store.calls != 1 || result.ReplanCount != 1 {
		t.Fatalf("scheduler did not reload after CAS conflict: reader=%d store=%d result=%#v", reader.calls, store.calls, result)
	}
	if len(result.Plan.Dispatch) != 1 || len(result.Executions) != 1 ||
		len(dispatcher.calls) != 1 || dispatcher.calls[0].AttemptRef != "attempt-a" ||
		len(store.applied) != 0 {
		t.Fatalf("scheduler did not dispatch the recomputed ready Stage: result=%#v calls=%#v", result, dispatcher.calls)
	}
}

func TestSchedulerStopsAfterBoundedCASConflicts(t *testing.T) {
	run, now := schedulerFixture()
	stage := domain.Stage{Ref: "stage-a", RunRef: run.Ref, Code: "market", Status: domain.StagePending, Version: 1}
	reader := &schedulerRunReaderFake{results: []CreateRunResult{{Run: run, Stages: []domain.Stage{stage}}}}
	store := &schedulerReplanStoreFake{alwaysConflict: true}
	dispatcher := &schedulerDispatcherFake{}
	scheduler, err := NewScheduler(reader, store, schedulerClockFake{now: now},
		&schedulerIDGeneratorFake{refs: []string{"unused"}}, nil, dispatcher,
		SchedulerConfig{MaxParallel: 1, MaxReplans: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scheduler.ScheduleRun(context.Background(), run.Ref); !errors.Is(err, ErrScheduleReplanExhausted) {
		t.Fatalf("scheduler should stop after bounded conflicts, got %v", err)
	}
	if reader.calls != 2 || store.calls != 2 || len(dispatcher.calls) != 0 {
		t.Fatalf("scheduler exceeded or bypassed replan boundary: reader=%d store=%d dispatch=%d", reader.calls, store.calls, len(dispatcher.calls))
	}
}


type schedulerConcurrentPoolDispatcher struct {
	started chan string
	release chan struct{}
}

func (d *schedulerConcurrentPoolDispatcher) Execute(ctx context.Context, runRef, stageRef, attemptRef string) (WorkerExecution, error) {
	d.started <- stageRef
	select {
	case <-d.release:
		return WorkerExecution{Attempt: domain.Attempt{Ref: attemptRef, StageRef: stageRef}}, nil
	case <-ctx.Done():
		return WorkerExecution{}, ctx.Err()
	}
}

func TestSchedulerDispatchesIndependentStagesConcurrentlyThroughWorkerPool(t *testing.T) {
	run, now := schedulerFixture()
	run.Status = domain.RunCreated
	stages := []domain.Stage{
		{Ref: "stage-b", RunRef: run.Ref, Code: "quant", Status: domain.StagePending, Version: 1},
		{Ref: "stage-a", RunRef: run.Ref, Code: "context", Status: domain.StagePending, Version: 1},
	}
	reader := &schedulerRunReaderFake{results: []CreateRunResult{{Run: run, Stages: stages}}}
	store := &schedulerReplanStoreFake{}
	ids := &schedulerIDGeneratorFake{refs: []string{"attempt-a", "attempt-b"}}
	executor := &schedulerConcurrentPoolDispatcher{
		started: make(chan string, 2),
		release: make(chan struct{}, 2),
	}
	pool, err := NewWorkerPool(executor, WorkerPoolConfig{Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := pool.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown Worker Pool: %v", err)
		}
	}()
	scheduler, err := NewScheduler(reader, store, schedulerClockFake{now: now}, ids, nil, pool,
		SchedulerConfig{MaxParallel: 2, MaxReplans: 1})
	if err != nil {
		t.Fatal(err)
	}

	type scheduleOutcome struct {
		result ScheduleRunResult
		err    error
	}
	done := make(chan scheduleOutcome, 1)
	go func() {
		result, scheduleErr := scheduler.ScheduleRun(context.Background(), run.Ref)
		done <- scheduleOutcome{result: result, err: scheduleErr}
	}()
	started := make([]string, 0, 2)
	for index := 0; index < 2; index++ {
		select {
		case stageRef := <-executor.started:
			started = append(started, stageRef)
		case <-time.After(time.Second):
			t.Fatalf("Scheduler did not dispatch both independent Stages concurrently; started=%v", started)
		}
	}
	executor.release <- struct{}{}
	executor.release <- struct{}{}
	outcome := <-done
	if outcome.err != nil {
		t.Fatal(outcome.err)
	}
	if len(outcome.result.Executions) != 2 ||
		outcome.result.Executions[0].StageRef != "stage-a" ||
		outcome.result.Executions[1].StageRef != "stage-b" {
		t.Fatalf("Scheduler did not preserve deterministic result order: %#v", outcome.result.Executions)
	}
}
