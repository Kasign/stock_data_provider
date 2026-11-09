package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

type workerStoreFake struct {
	attempt     domain.Attempt
	snapshot    StageExecutionSnapshot
	claimReplay bool
	renewErr    error
	commit      WorkerResultCommit
	commits     int
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
func (f *workerStoreFake) CommitWorkerResult(_ context.Context, c WorkerResultCommit) error {
	f.commit = c
	f.commits++
	return nil
}
type workerExecutorFake struct {
	result StageExecutionResult
	err    error
}
func (f workerExecutorFake) ExecuteStage(context.Context, StageExecutionRequest) (StageExecutionResult, error) {
	return f.result, f.err
}

func newWorkerFixture() (*workerStoreFake, StageExecutionResult) {
	deadline := time.Now().Add(time.Hour)
	run := domain.Run{Ref: "run-1", Status: domain.RunRunning, DeadlineAt: deadline, SystemAsOf: time.Now().UTC(), CorrelationID: "corr-1"}
	stage := domain.Stage{Ref: "stage-1", RunRef: "run-1", Status: domain.StageRunning, Version: 3, AttemptNo: 1}
	attempt := domain.Attempt{Ref: "attempt-1", StageRef: stage.Ref, Number: 1, Status: domain.AttemptRunning, LeaseOwner: "worker-1", FencingToken: 7}
	output := "snapshot://result"
	store := &workerStoreFake{attempt: attempt, snapshot: StageExecutionSnapshot{Run: run, Stage: stage, Parameters: []byte(`{"symbol":"AAA"}`)}}
	return store, StageExecutionResult{AttemptStatus: domain.AttemptSucceeded, StageStatus: domain.StageSucceeded, OutputRef: &output}
}
func newWorker(t *testing.T, store *workerStoreFake, result StageExecutionResult) *Worker {
	t.Helper()
	w, err := NewWorker(store, workerExecutorFake{result: result}, WorkerConfig{WorkerID: "worker-1", LeaseDuration: time.Minute, HeartbeatInterval: time.Second, MaxAttempts: 3, MaxParallel: 2})
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
	w, err := NewWorker(store, workerExecutorFake{err: errors.New("provider unavailable")}, WorkerConfig{WorkerID: "worker-1", LeaseDuration: time.Minute, HeartbeatInterval: time.Second, MaxAttempts: 3, MaxParallel: 2})
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Execute(context.Background(), "run-1", "stage-1", "attempt-new")
	if err == nil || store.commits != 0 {
		t.Fatal("executor error must not be committed as success")
	}
}
