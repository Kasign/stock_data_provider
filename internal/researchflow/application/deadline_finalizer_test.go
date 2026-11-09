package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

type deadlineFinalizerStoreFake struct {
	snapshot DeadlineFinalizationSnapshot
	record   DeadlineFinalizationRecord
	finalized bool
	loadErr  error
	saveErr  error
}

func (f *deadlineFinalizerStoreFake) LoadDeadlineFinalizationSnapshot(context.Context, string) (DeadlineFinalizationSnapshot, error) {
	return f.snapshot, f.loadErr
}
func (f *deadlineFinalizerStoreFake) FinalizeDeadlineRun(_ context.Context, record DeadlineFinalizationRecord) (bool, error) {
	f.record = record
	return f.finalized, f.saveErr
}

type deadlineFinalizerClockFake struct { now time.Time; err error }
func (f deadlineFinalizerClockFake) NowUTC(context.Context) (time.Time, error) { return f.now, f.err }

type deadlineFinalizerIDsFake struct { next int; err error }
func (f *deadlineFinalizerIDsFake) NewRef(_ context.Context, kind string) (string, error) {
	if f.err != nil { return "", f.err }
	f.next++
	return kind + "-" + string(rune('0'+f.next)), nil
}

func validDeadlineFinalizationSnapshot() DeadlineFinalizationSnapshot {
	return DeadlineFinalizationSnapshot{
		Run: domain.Run{Ref: "run-1", Status: domain.RunRunning, Version: 7, TerminationReason: "deadline_exceeded"},
		Stages: []domain.Stage{
			{Ref: "stage-1", RunRef: "run-1", Code: "required", Required: true, Status: domain.StageTimedOut, ReasonCodes: []string{"run_deadline_exceeded"}},
		},
		PropagationPolicies: map[string]template.PropagationPolicy{
			"required": {OnTimedOut: domain.PropagateFail},
		},
	}
}

func TestDeadlineFinalizerAggregatesAndPersistsWithExpectedVersion(t *testing.T) {
	store := &deadlineFinalizerStoreFake{snapshot: validDeadlineFinalizationSnapshot(), finalized: true}
	ids := &deadlineFinalizerIDsFake{}
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	finalizer, err := NewDeadlineFinalizer(store, deadlineFinalizerClockFake{now: now}, ids, DeadlineFinalizerConfig{SummarySchemaVersion: "1.0.0"})
	if err != nil { t.Fatal(err) }
	got, err := finalizer.Finalize(context.Background(), "run-1")
	if err != nil { t.Fatal(err) }
	if !got.Finalized || got.Run.Status != domain.RunFailed || got.Run.ResultSummary == nil || got.Run.ResultSummaryRef == nil {
		t.Fatalf("unexpected deadline finalization result: %+v", got)
	}
	if store.record.ExpectedVersion != 7 || store.record.TerminalStatus != domain.RunFailed ||
		store.record.TerminationReason != "deadline_exceeded" || store.record.ResultSummaryRef == "" || store.record.OutboxRef == "" {
		t.Fatalf("unexpected persistence record: %+v", store.record)
	}
}

func TestDeadlineFinalizerRejectsWrongStateAndRunningAttempts(t *testing.T) {
	store := &deadlineFinalizerStoreFake{snapshot: validDeadlineFinalizationSnapshot()}
	finalizer, err := NewDeadlineFinalizer(store, deadlineFinalizerClockFake{now: time.Now().UTC()}, &deadlineFinalizerIDsFake{}, DeadlineFinalizerConfig{SummarySchemaVersion: "1.0.0"})
	if err != nil { t.Fatal(err) }
	store.snapshot.Run.TerminationReason = "user_cancelled"
	if _, err := finalizer.Finalize(context.Background(), "run-1"); err == nil {
		t.Fatal("expected user cancellation path to be rejected")
	}
	store.snapshot = validDeadlineFinalizationSnapshot()
	store.snapshot.Attempts = []domain.Attempt{{Ref: "attempt-1", Status: domain.AttemptRunning}}
	if _, err := finalizer.Finalize(context.Background(), "run-1"); err == nil {
		t.Fatal("expected running Attempt to block finalization")
	}
}

func TestDeadlineFinalizerPropagatesPersistenceAndClockErrors(t *testing.T) {
	store := &deadlineFinalizerStoreFake{snapshot: validDeadlineFinalizationSnapshot(), saveErr: errors.New("database unavailable")}
	finalizer, err := NewDeadlineFinalizer(store, deadlineFinalizerClockFake{now: time.Now().UTC()}, &deadlineFinalizerIDsFake{}, DeadlineFinalizerConfig{SummarySchemaVersion: "1.0.0"})
	if err != nil { t.Fatal(err) }
	if _, err := finalizer.Finalize(context.Background(), "run-1"); err == nil {
		t.Fatal("expected persistence error")
	}
	store.saveErr = nil
	clockErr := errors.New("clock unavailable")
	finalizer, err = NewDeadlineFinalizer(store, deadlineFinalizerClockFake{err: clockErr}, &deadlineFinalizerIDsFake{}, DeadlineFinalizerConfig{SummarySchemaVersion: "1.0.0"})
	if err != nil { t.Fatal(err) }
	if _, err := finalizer.Finalize(context.Background(), "run-1"); err == nil {
		t.Fatal("expected database clock error")
	}
}
