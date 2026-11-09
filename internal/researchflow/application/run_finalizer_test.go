package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

type runFinalizerStoreFake struct {
	snapshot  DeadlineFinalizationSnapshot
	record    RunFinalizationRecord
	finalized bool
	err       error
}

func (f *runFinalizerStoreFake) LoadRunFinalizationSnapshot(context.Context, string) (DeadlineFinalizationSnapshot, error) {
	return f.snapshot, nil
}

func (f *runFinalizerStoreFake) FinalizeRunResult(_ context.Context, r RunFinalizationRecord) (bool, error) {
	f.record = r
	return f.finalized, f.err
}

type runFinalizerClockFake struct {
	now time.Time
	err error
}

func (f runFinalizerClockFake) NowUTC(context.Context) (time.Time, error) {
	return f.now, f.err
}

type runFinalizerIDsFake struct{ n int }

func (f *runFinalizerIDsFake) NewRef(_ context.Context, kind string) (string, error) {
	f.n++
	return kind + "-" + string(rune('0'+f.n)), nil
}

func validRunFinalizationSnapshot() DeadlineFinalizationSnapshot {
	return DeadlineFinalizationSnapshot{
		Run:                 domain.Run{Ref: "run-normal", Status: domain.RunRunning, Version: 4},
		Stages:              []domain.Stage{{Ref: "stage-1", RunRef: "run-normal", Code: "required", Required: true, Status: domain.StageSucceeded}},
		Attempts:            []domain.Attempt{{Ref: "attempt-1", StageRef: "stage-1", Number: 1, Status: domain.AttemptSucceeded}},
		PropagationPolicies: map[string]template.PropagationPolicy{"required": {}},
	}
}

func TestRunFinalizerAggregatesAndPersists(t *testing.T) {
	store := &runFinalizerStoreFake{snapshot: validRunFinalizationSnapshot(), finalized: true}
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	f, err := NewRunFinalizer(store, runFinalizerClockFake{now: now}, &runFinalizerIDsFake{}, RunFinalizerConfig{SummarySchemaVersion: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.Finalize(context.Background(), "run-normal")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Finalized || got.Run.Status != domain.RunSucceeded || got.Run.ResultSummary == nil || got.Run.ResultSummaryRef == nil {
		t.Fatalf("unexpected result: %+v", got)
	}
	if store.record.ExpectedVersion != 4 || store.record.TerminalStatus != domain.RunSucceeded || store.record.TerminationReason != "" {
		t.Fatalf("unexpected record: %+v", store.record)
	}
}

func TestRunFinalizerRejectsDeadlineAndIncompleteStages(t *testing.T) {
	store := &runFinalizerStoreFake{snapshot: validRunFinalizationSnapshot()}
	f, err := NewRunFinalizer(store, runFinalizerClockFake{now: time.Now().UTC()}, &runFinalizerIDsFake{}, RunFinalizerConfig{SummarySchemaVersion: "1"})
	if err != nil {
		t.Fatal(err)
	}
	store.snapshot.Run.TerminationReason = "deadline_exceeded"
	if _, err := f.Finalize(context.Background(), "run-normal"); err == nil {
		t.Fatal("expected deadline path rejection")
	}
	store.snapshot = validRunFinalizationSnapshot()
	store.snapshot.Stages[0].Status = domain.StageRunning
	if _, err := f.Finalize(context.Background(), "run-normal"); err == nil {
		t.Fatal("expected non-terminal Stage rejection")
	}
}

func TestRunFinalizerPropagatesStorageError(t *testing.T) {
	store := &runFinalizerStoreFake{snapshot: validRunFinalizationSnapshot(), err: errors.New("db down")}
	f, err := NewRunFinalizer(store, runFinalizerClockFake{now: time.Now().UTC()}, &runFinalizerIDsFake{}, RunFinalizerConfig{SummarySchemaVersion: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Finalize(context.Background(), "run-normal"); err == nil {
		t.Fatal("expected storage error")
	}
}
