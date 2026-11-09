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
