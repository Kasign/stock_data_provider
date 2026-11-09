package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

type workerStoreFake struct {
	attempt     domain.Attempt
	snapshot    StageExecutionSnapshot
	claimReplay bool
	renewErr    error
	commit      WorkerResultCommit
	commits     int
	cancelled   int
	cancelErr   error
	cancelOK    bool
}

func (f *workerStoreFake) ClaimAttempt(context.Context, string, string, string, string, time.Duration, int, int) (domain.Attempt, bool, error) {
	return f.attempt, f.claimReplay, nil
}
func (f *workerStoreFake) LoadStageExecutionSnapshot(context.Context, string, string) (StageExecutionSnapshot, error) {
	return f.snapshot, nil
}
func (f *workerStoreFake) RenewAttemptLease(context.Context, string, string, string, string, uint64, time.Duration) (time.Time, error) {
	return time.Now().Add(time.Minute), f.renewErr
}
func (f *workerStoreFake) ConfirmAttemptCancellation(context.Context, string, string, string, string, uint64, uint64) (bool, error) { f.cancelled++; return f.cancelOK, f.cancelErr }
func (f *workerStoreFake) CommitWorkerResult(_ context.Context, c WorkerResultCommit) error {
	f.commit = c
	f.commits++
	return nil
}

type workerExecutorFake struct {
	result StageExecutionResult
	err    error
}

type blockingWorkerExecutor struct { started chan struct{}; once sync.Once }
func (e *blockingWorkerExecutor) ExecuteStage(ctx context.Context, _ StageExecutionRequest) (StageExecutionResult, error) { e.once.Do(func(){close(e.started)}); <-ctx.Done(); return StageExecutionResult{}, ctx.Err() }

type timeoutWorkerExecutor struct{}

func (timeoutWorkerExecutor) ExecuteStage(ctx context.Context, _ StageExecutionRequest) (StageExecutionResult, error) {
	<-ctx.Done()
	return StageExecutionResult{}, ctx.Err()
}

func (f workerExecutorFake) ExecuteStage(context.Context, StageExecutionRequest) (StageExecutionResult, error) {
	return f.result, f.err
}

func newWorkerFixture() (*workerStoreFake, StageExecutionResult) {
	deadline := time.Now().Add(time.Hour)
	run := domain.Run{Ref: "run-1", Status: domain.RunRunning, DeadlineAt: deadline, SystemAsOf: time.Now().UTC(), CorrelationID: "corr-1"}
	stage := domain.Stage{Ref: "stage-1", RunRef: "run-1", Status: domain.StageRunning, Version: 3, AttemptNo: 1}
	attempt := domain.Attempt{Ref: "attempt-1", StageRef: stage.Ref, Number: 1, Status: domain.AttemptRunning, LeaseOwner: "worker-1", FencingToken: 7, StartedAt: time.Now().UTC()}
	output := "snapshot://result"
	store := &workerStoreFake{
		attempt: attempt,
		snapshot: StageExecutionSnapshot{
			Run: run, Stage: stage, Parameters: []byte(`{"symbol":"AAA"}`),
			RetryPolicy: template.RetryPolicy{
				Ref: "retry-v1", MaxAttempts: 3,
				RetryableReasonCodes: []string{"provider_unavailable"},
				Backoff:              template.BackoffPolicy{Algorithm: "exponential_full_jitter_v1", BaseDelayMS: 1000, MaxDelayMS: 5000},
				AttemptTimeoutMS:     60000,
			},
		},
	}
	return store, StageExecutionResult{AttemptStatus: domain.AttemptSucceeded, StageStatus: domain.StageSucceeded, OutputRef: &output}
}

type workerClockFake struct{}

func (workerClockFake) NowUTC(context.Context) (time.Time, error) { return time.Now().UTC(), nil }

func newWorker(t *testing.T, store *workerStoreFake, result StageExecutionResult) *Worker {
	t.Helper()
	w, err := NewWorker(store, workerExecutorFake{result: result}, workerClockFake{}, WorkerConfig{WorkerID: "worker-1", LeaseDuration: time.Minute, HeartbeatInterval: time.Second, MaxParallel: 2})
	if err != nil {
		t.Fatal(err)
	}
	return w
}
func TestWorkerExecuteClaimsExecutesAndCommits(t *testing.T) {
	store, result := newWorkerFixture()
	got, err := newWorker(t, store, result).Execute(context.Background(), "run-1", "stage-1", "attempt-new")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Committed || store.commits != 1 {
		t.Fatalf("expected committed result: %+v commits=%d", got, store.commits)
	}
	if store.commit.Stage.Version != 4 || store.commit.Stage.Status != domain.StageSucceeded || store.commit.FencingToken != 7 {
		t.Fatalf("unexpected commit: %+v", store.commit)
	}
}
func TestWorkerDoesNotExecuteReplayedClaim(t *testing.T) {
	store, result := newWorkerFixture()
	store.claimReplay = true
	got, err := newWorker(t, store, result).Execute(context.Background(), "run-1", "stage-1", "attempt-new")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Replayed || store.commits != 0 {
		t.Fatalf("replay must not execute or commit: %+v", got)
	}
}
func TestWorkerRejectsInvalidSnapshot(t *testing.T) {
	store, result := newWorkerFixture()
	store.snapshot.Stage.AttemptNo = 9
	_, err := newWorker(t, store, result).Execute(context.Background(), "run-1", "stage-1", "attempt-new")
	if err == nil {
		t.Fatal("expected inconsistent snapshot to fail")
	}
}
func TestWorkerDoesNotCommitExecutorError(t *testing.T) {
	store, _ := newWorkerFixture()
	w, err := NewWorker(store, workerExecutorFake{err: errors.New("provider unavailable")}, workerClockFake{}, WorkerConfig{WorkerID: "worker-1", LeaseDuration: time.Minute, HeartbeatInterval: time.Second, MaxParallel: 2})
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Execute(context.Background(), "run-1", "stage-1", "attempt-new")
	if err == nil || store.commits != 0 {
		t.Fatal("executor error must not be committed as success")
	}
}

func TestWorkerAppliesRetryPolicyInsteadOfProviderRetryAt(t *testing.T) {
	store, _ := newWorkerFixture()
	store.snapshot.RetryPolicy = template.RetryPolicy{
		Ref: "retry-v1", MaxAttempts: 3,
		RetryableReasonCodes: []string{"provider_unavailable"},
		Backoff:              template.BackoffPolicy{Algorithm: "exponential_full_jitter_v1", BaseDelayMS: 1000, MaxDelayMS: 5000},
		AttemptTimeoutMS:     60000,
	}
	store.snapshot.Stage.AttemptNo = 1
	store.attempt.Number = 1
	result := StageExecutionResult{
		AttemptStatus: domain.AttemptFailed,
		StageStatus:   domain.StageRetryWait,
		ReasonCodes:   []string{"provider_unavailable"},
	}
	got, err := newWorker(t, store, result).Execute(context.Background(), "run-1", "stage-1", "attempt-new")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Committed || store.commit.Stage.Status != domain.StageRetryWait || store.commit.Stage.RetryAt == nil {
		t.Fatalf("expected policy-controlled retry_wait commit: %+v", store.commit)
	}
}

func TestWorkerDisablesRetryForNonAllowlistedReason(t *testing.T) {
	store, _ := newWorkerFixture()
	store.snapshot.RetryPolicy = template.RetryPolicy{
		Ref: "retry-v1", MaxAttempts: 3,
		RetryableReasonCodes: []string{"provider_unavailable"},
		Backoff:              template.BackoffPolicy{Algorithm: "exponential_full_jitter_v1", BaseDelayMS: 1000, MaxDelayMS: 5000},
	}
	result := StageExecutionResult{
		AttemptStatus: domain.AttemptFailed,
		StageStatus:   domain.StageRetryWait,
		ReasonCodes:   []string{"schema_invalid"},
	}
	got, err := newWorker(t, store, result).Execute(context.Background(), "run-1", "stage-1", "attempt-new")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Committed || store.commit.Stage.Status != domain.StageFailed || store.commit.Stage.RetryAt != nil {
		t.Fatalf("non-allowlisted result must become failed: %+v", store.commit)
	}
}

func TestWorkerConvertsAttemptDeadlineIntoPolicyControlledTimeout(t *testing.T) {
	store, _ := newWorkerFixture()
	store.snapshot.RetryPolicy.AttemptTimeoutMS = 1
	store.snapshot.RetryPolicy.RetryableReasonCodes = []string{"attempt_timeout"}
	worker, err := NewWorker(store, timeoutWorkerExecutor{}, workerClockFake{}, WorkerConfig{
		WorkerID: "worker-1", LeaseDuration: time.Minute, HeartbeatInterval: time.Second, MaxParallel: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := worker.Execute(context.Background(), "run-1", "stage-1", "attempt-new")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Committed || store.commit.AttemptStatus != domain.AttemptTimedOut ||
		store.commit.Stage.Status != domain.StageRetryWait || store.commit.Stage.RetryAt == nil {
		t.Fatalf("attempt timeout should follow the retry policy: %+v", store.commit)
	}
}


func TestWorkerCancellationStopsProviderAndConfirmsAttempt(t *testing.T) {
	store, _ := newWorkerFixture()
	store.cancelOK = true
	executor := &blockingWorkerExecutor{started: make(chan struct{})}
	worker, err := NewWorker(store, executor, workerClockFake{}, WorkerConfig{WorkerID:"worker-1", LeaseDuration:time.Minute, HeartbeatInterval:time.Second, MaxParallel:2})
	if err != nil { t.Fatal(err) }
	done := make(chan error, 1)
	go func(){ _, err := worker.Execute(context.Background(), "run-1", "stage-1", "attempt-new"); done <- err }()
	<-executor.started
	if n := worker.CancelRun("run-1"); n != 1 { t.Fatalf("expected one active attempt cancelled, got %d", n) }
	if err := <-done; err != nil { t.Fatal(err) }
	if store.cancelled != 1 || store.commits != 0 { t.Fatalf("expected cancellation confirmation only: cancelled=%d commits=%d", store.cancelled, store.commits) }
}

func TestWorkerCancellationConfirmationFailureDoesNotCommit(t *testing.T) {
	store, _ := newWorkerFixture()
	store.cancelErr = errors.New("fencing conflict")
	executor := &blockingWorkerExecutor{started: make(chan struct{})}
	worker, err := NewWorker(store, executor, workerClockFake{}, WorkerConfig{WorkerID:"worker-1", LeaseDuration:time.Minute, HeartbeatInterval:time.Second, MaxParallel:2})
	if err != nil { t.Fatal(err) }
	done := make(chan error, 1)
	go func(){ _, err := worker.Execute(context.Background(), "run-1", "stage-1", "attempt-new"); done <- err }()
	<-executor.started
	worker.CancelRun("run-1")
	if err := <-done; err == nil { t.Fatal("expected cancellation fencing failure") }
	if store.commits != 0 { t.Fatal("must not commit after cancellation") }
}
