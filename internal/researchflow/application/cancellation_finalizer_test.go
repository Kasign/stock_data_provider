package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

type cancellationFinalizerStoreFake struct {
	snapshot  DeadlineFinalizationSnapshot
	record    CancellationFinalizationRecord
	finalized bool
	err       error
}

func (f *cancellationFinalizerStoreFake) LoadDeadlineFinalizationSnapshot(context.Context, string) (DeadlineFinalizationSnapshot, error) {
	return f.snapshot, nil
}

func (f *cancellationFinalizerStoreFake) FinalizeCancellationRun(_ context.Context, record CancellationFinalizationRecord) (bool, error) {
	f.record = record
	return f.finalized, f.err
}

func validCancellationFinalizationSnapshot() DeadlineFinalizationSnapshot {
	return DeadlineFinalizationSnapshot{
		Run: domain.Run{Ref: "run-cancel", Status: domain.RunCancelling, Version: 8, TerminationReason: "user_cancelled"},
		Stages: []domain.Stage{
			{Ref: "stage-1", RunRef: "run-cancel", Code: "required", Required: true, Status: domain.StageCancelled, ReasonCodes: []string{"user_cancelled"}},
		},
		Attempts: []domain.Attempt{
			{Ref: "attempt-1", StageRef: "stage-1", Number: 1, Status: domain.AttemptCancelled},
		},
		PropagationPolicies: map[string]template.PropagationPolicy{"required": {}},
	}
}

func TestCancellationFinalizerAggregatesAndPersists(t *testing.T) {
	store := &cancellationFinalizerStoreFake{snapshot: validCancellationFinalizationSnapshot(), finalized: true}
	clock := runFinalizerClockFake{now: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)}
	ids := &runFinalizerIDsFake{}
	finalizer, err := NewCancellationFinalizer(store, clock, ids, CancellationFinalizerConfig{SummarySchemaVersion: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := finalizer.Finalize(context.Background(), "run-cancel")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Finalized || got.Run.Status != domain.RunCancelled || got.Run.ResultSummary == nil || got.Run.ResultSummaryRef == nil {
		t.Fatalf("unexpected result: %+v", got)
	}
	if store.record.ExpectedVersion != 8 || store.record.TerminalStatus != domain.RunCancelled ||
		store.record.TerminationReason != "user_cancelled" || store.record.ResultSummaryRef == "" || store.record.OutboxRef == "" {
		t.Fatalf("unexpected persistence record: %+v", store.record)
	}
}

func TestCancellationFinalizerRejectsOtherTerminationPathsAndRunningAttempts(t *testing.T) {
	store := &cancellationFinalizerStoreFake{snapshot: validCancellationFinalizationSnapshot()}
	finalizer, err := NewCancellationFinalizer(store, runFinalizerClockFake{now: time.Now().UTC()}, &runFinalizerIDsFake{}, CancellationFinalizerConfig{SummarySchemaVersion: "1"})
	if err != nil {
		t.Fatal(err)
	}
	store.snapshot.Run.TerminationReason = "deadline_exceeded"
	if _, err := finalizer.Finalize(context.Background(), "run-cancel"); err == nil {
		t.Fatal("expected deadline path rejection")
	}
	store.snapshot = validCancellationFinalizationSnapshot()
	store.snapshot.Attempts[0].Status = domain.AttemptRunning
	if _, err := finalizer.Finalize(context.Background(), "run-cancel"); err == nil {
		t.Fatal("expected running Attempt rejection")
	}
}

func TestCancellationFinalizerPropagatesStorageError(t *testing.T) {
	store := &cancellationFinalizerStoreFake{snapshot: validCancellationFinalizationSnapshot(), err: errors.New("db down")}
	finalizer, err := NewCancellationFinalizer(store, runFinalizerClockFake{now: time.Now().UTC()}, &runFinalizerIDsFake{}, CancellationFinalizerConfig{SummarySchemaVersion: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := finalizer.Finalize(context.Background(), "run-cancel"); err == nil {
		t.Fatal("expected storage error")
	}
}
