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
	queued := run
	queued.Status = domain.RunQueued
	if _, err := BuildSchedulePlan(context.Background(), queued, nil, nil, 1, now, nil); err == nil {
		t.Fatal("queued Run must not be scheduled")
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
