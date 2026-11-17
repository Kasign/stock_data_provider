package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/state"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

type retrySnapshotReaderStub struct {
	snapshot StageExecutionSnapshot
	err      error
	calls    int
}

func (s *retrySnapshotReaderStub) LoadStageExecutionSnapshot(context.Context, string, string) (StageExecutionSnapshot, error) {
	s.calls++
	return s.snapshot, s.err
}

type retryDatabaseClockStub struct {
	now   time.Time
	err   error
	calls int
}

func (s *retryDatabaseClockStub) NowUTC(context.Context) (time.Time, error) {
	s.calls++
	return s.now, s.err
}

type retryStageExecutorStub struct {
	result                       WorkerExecution
	err                          error
	runRef, stageRef, attemptRef string
	calls                        int
}

func (s *retryStageExecutorStub) Execute(_ context.Context, runRef, stageRef, attemptRef string) (WorkerExecution, error) {
	s.calls++
	s.runRef, s.stageRef, s.attemptRef = runRef, stageRef, attemptRef
	return s.result, s.err
}

func retryStageSnapshot() StageExecutionSnapshot {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	retryAt := now.Add(-time.Minute)
	return StageExecutionSnapshot{
		Run: domain.Run{
			Ref: "run-1", Status: domain.RunRunning, Version: 7,
			DeadlineAt: now.Add(time.Hour),
		},
		Stage: domain.Stage{
			Ref: "stage-1", RunRef: "run-1", Status: domain.StageRetryWait,
			Version: 4, AttemptNo: 1, RetryAt: &retryAt,
			ReasonCodes: []string{"provider_unavailable"},
		},
		RetryPolicy: template.RetryPolicy{
			Ref: "retry-v1", MaxAttempts: 3,
			RetryableReasonCodes: []string{"provider_unavailable"},
		},
	}
}

func TestRetryStageDispatchesEligibleRetryWithStableAttemptReference(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	reader := &retrySnapshotReaderStub{snapshot: retryStageSnapshot()}
	clock := &retryDatabaseClockStub{now: now}
	executor := &retryStageExecutorStub{result: WorkerExecution{Committed: true}}
	service, err := NewRetryStageService(reader, clock, executor)
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.RetryStage(context.Background(), "run-1", "stage-1", "attempt-2")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Committed || executor.calls != 1 || executor.runRef != "run-1" ||
		executor.stageRef != "stage-1" || executor.attemptRef != "attempt-2" {
		t.Fatalf("retry dispatch did not preserve supplied references: result=%#v executor=%#v", got, executor)
	}
}

func TestRetryStageRejectsIneligibleSnapshotsBeforeDispatch(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		change func(*StageExecutionSnapshot)
		want   error
	}{
		{name: "run cancelling", change: func(s *StageExecutionSnapshot) { s.Run.Status = domain.RunCancelling }, want: ErrRetryStageUnavailable},
		{name: "deadline termination", change: func(s *StageExecutionSnapshot) { s.Run.TerminationReason = "deadline_exceeded" }, want: ErrRetryStageUnavailable},
		{name: "stage not retry wait", change: func(s *StageExecutionSnapshot) { s.Stage.Status = domain.StageFailed }, want: ErrRetryStageUnavailable},
		{name: "budget exhausted", change: func(s *StageExecutionSnapshot) { s.Stage.AttemptNo = 3 }, want: ErrRetryStageBudgetExhausted},
		{name: "reason not allowlisted", change: func(s *StageExecutionSnapshot) { s.Stage.ReasonCodes = []string{"schema_invalid"} }, want: ErrRetryStageNotRetryable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := retryStageSnapshot()
			tc.change(&snapshot)
			reader := &retrySnapshotReaderStub{snapshot: snapshot}
			executor := &retryStageExecutorStub{}
			service, err := NewRetryStageService(reader, &retryDatabaseClockStub{now: now}, executor)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.RetryStage(context.Background(), "run-1", "stage-1", "attempt-2"); !errors.Is(err, tc.want) {
				t.Fatalf("RetryStage error=%v, want %v", err, tc.want)
			}
			if executor.calls != 0 {
				t.Fatal("ineligible retry reached Worker")
			}
		})
	}
}

func TestRetryStageRejectsNotDueAndExpiredDeadline(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		change func(*StageExecutionSnapshot)
		want   error
	}{
		{name: "retry time future", change: func(s *StageExecutionSnapshot) { future := now.Add(time.Minute); s.Stage.RetryAt = &future }, want: ErrRetryStageNotDue},
		{name: "run deadline reached", change: func(s *StageExecutionSnapshot) { s.Run.DeadlineAt = now }, want: state.ErrDeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := retryStageSnapshot()
			tc.change(&snapshot)
			executor := &retryStageExecutorStub{}
			service, err := NewRetryStageService(&retrySnapshotReaderStub{snapshot: snapshot}, &retryDatabaseClockStub{now: now}, executor)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.RetryStage(context.Background(), "run-1", "stage-1", "attempt-2"); !errors.Is(err, tc.want) {
				t.Fatalf("RetryStage error=%v, want %v", err, tc.want)
			}
			if executor.calls != 0 {
				t.Fatal("not-due or expired retry reached Worker")
			}
		})
	}
}

func TestRetryStagePropagatesClockAndWorkerErrors(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	clockErr := errors.New("database clock unavailable")
	service, err := NewRetryStageService(
		&retrySnapshotReaderStub{snapshot: retryStageSnapshot()},
		&retryDatabaseClockStub{now: now, err: clockErr},
		&retryStageExecutorStub{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RetryStage(context.Background(), "run-1", "stage-1", "attempt-2"); !errors.Is(err, clockErr) {
		t.Fatalf("RetryStage error=%v, want clock error", err)
	}

	workerErr := errors.New("claim conflict")
	executor := &retryStageExecutorStub{err: workerErr}
	service, err = NewRetryStageService(
		&retrySnapshotReaderStub{snapshot: retryStageSnapshot()},
		&retryDatabaseClockStub{now: now},
		executor,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RetryStage(context.Background(), "run-1", "stage-1", "attempt-2"); !errors.Is(err, workerErr) {
		t.Fatalf("RetryStage error=%v, want Worker error", err)
	}
}
