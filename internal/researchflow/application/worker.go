package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

var (
	ErrWorkerLeaseLost = errors.New("Researchflow worker lease lost")
	ErrWorkerDeadline = errors.New("Researchflow run deadline exceeded")
)

type StageExecutionSnapshot struct {
	Run       domain.Run
	Stage     domain.Stage
	Parameters []byte
	InputSnapshotRefs []string
}

type WorkerResultCommit struct {
	RunRef string
	StageRef string
	AttemptRef string
	WorkerID string
	FencingToken uint64
	AttemptStatus domain.AttemptStatus
	Stage domain.Stage
	FaultRef *string
	OutputRef *string
}

// WorkerStore is the Application boundary. Its persistence adapter must map
// CommitWorkerResult to the atomic storage CommitAttemptResult transaction.
type WorkerStore interface {
	ClaimAttempt(context.Context, string, string, string, string, time.Duration, int, int) (domain.Attempt, bool, error)
	LoadStageExecutionSnapshot(context.Context, string, string) (StageExecutionSnapshot, error)
	RenewAttemptLease(context.Context, string, string, string, string, uint64, time.Duration) (time.Time, error)
	CommitWorkerResult(context.Context, WorkerResultCommit) error
}

type WorkerConfig struct {
	WorkerID string
	LeaseDuration time.Duration
	HeartbeatInterval time.Duration
	MaxAttempts int
	MaxParallel int
}

type WorkerExecution struct {
	Attempt domain.Attempt
	Replayed bool
	Committed bool
}

type Worker struct {
	store WorkerStore
	executor StageExecutor
	config WorkerConfig
	now func() time.Time
}

func NewWorker(store WorkerStore, executor StageExecutor, config WorkerConfig) (*Worker, error) {
	if store == nil || executor == nil || strings.TrimSpace(config.WorkerID) == "" ||
		len(config.WorkerID) > 128 || config.LeaseDuration <= 0 ||
		config.HeartbeatInterval <= 0 || config.HeartbeatInterval >= config.LeaseDuration ||
		config.MaxAttempts <= 0 || config.MaxParallel <= 0 {
		return nil, errors.New("Researchflow worker dependencies or configuration are invalid")
	}
	return &Worker{store: store, executor: executor, config: config, now: time.Now}, nil
}

// Execute claims and runs one Stage. The store remains the concurrency,
// deadline, lease and fencing authority; this method only coordinates ports.
func (w *Worker) Execute(ctx context.Context, runRef, stageRef, attemptRef string) (WorkerExecution, error) {
	if strings.TrimSpace(runRef) == "" || strings.TrimSpace(stageRef) == "" || strings.TrimSpace(attemptRef) == "" {
		return WorkerExecution{}, errors.New("Researchflow worker references are required")
	}
	attempt, replayed, err := w.store.ClaimAttempt(ctx, runRef, stageRef, attemptRef, w.config.WorkerID,
		w.config.LeaseDuration, w.config.MaxAttempts, w.config.MaxParallel)
	if err != nil {
		return WorkerExecution{}, fmt.Errorf("claim Researchflow Stage: %w", err)
	}
	if replayed {
		return WorkerExecution{Attempt: attempt, Replayed: true}, nil
	}

	snapshot, err := w.store.LoadStageExecutionSnapshot(ctx, runRef, stageRef)
	if err != nil {
		return WorkerExecution{Attempt: attempt}, fmt.Errorf("load Researchflow execution snapshot: %w", err)
	}
	if snapshot.Run.Ref != runRef || snapshot.Stage.Ref != stageRef ||
		snapshot.Stage.RunRef != runRef || snapshot.Stage.Status != domain.StageRunning ||
		snapshot.Stage.AttemptNo != attempt.Number || snapshot.Run.Status != domain.RunRunning ||
		snapshot.Run.DeadlineAt.IsZero() {
		return WorkerExecution{Attempt: attempt}, errors.New("Researchflow execution snapshot is inconsistent with claimed Attempt")
	}
	if !w.now().Before(snapshot.Run.DeadlineAt) {
		return WorkerExecution{Attempt: attempt}, ErrWorkerDeadline
	}

	execCtx, cancel := context.WithDeadline(ctx, snapshot.Run.DeadlineAt)
	defer cancel()
	stopHeartbeat := make(chan struct{})
	heartbeatDone := make(chan struct{})
	heartbeatErr := make(chan error, 1)
	go w.heartbeat(execCtx, cancel, snapshot, attempt, stopHeartbeat, heartbeatDone, heartbeatErr)

	request := StageExecutionRequest{
		Run: snapshot.Run, Stage: snapshot.Stage, Attempt: attempt,
		Parameters: append([]byte(nil), snapshot.Parameters...),
		InputSnapshotRefs: append([]string(nil), snapshot.InputSnapshotRefs...),
		SystemAsOf: snapshot.Run.SystemAsOf, DeadlineAt: snapshot.Run.DeadlineAt,
		CorrelationID: snapshot.Run.CorrelationID,
	}
	result, executeErr := w.executor.ExecuteStage(execCtx, request)
	close(stopHeartbeat)
	<-heartbeatDone
	select {
	case heartbeatFailure := <-heartbeatErr:
		if errors.Is(heartbeatFailure, ErrWorkerDeadline) {
			return WorkerExecution{Attempt: attempt}, ErrWorkerDeadline
		}
		return WorkerExecution{Attempt: attempt}, fmt.Errorf("%w: %v", ErrWorkerLeaseLost, heartbeatFailure)
	default:
	}
	if errors.Is(execCtx.Err(), context.DeadlineExceeded) || !w.now().Before(snapshot.Run.DeadlineAt) {
		return WorkerExecution{Attempt: attempt}, ErrWorkerDeadline
	}
	if executeErr != nil {
		return WorkerExecution{Attempt: attempt}, fmt.Errorf("execute Researchflow Stage: %w", executeErr)
	}
	if err := ValidateStageExecutionResult(result); err != nil {
		return WorkerExecution{Attempt: attempt}, err
	}

	stage := snapshot.Stage
	if stage.Version == ^uint64(0) {
		return WorkerExecution{Attempt: attempt}, errors.New("Researchflow Stage version exhausted")
	}
	stage.Version++
	stage.Status = result.StageStatus
	stage.RetryAt = cloneTime(result.RetryAt)
	stage.OutputRef = cloneString(result.OutputRef)
	stage.QualitySummaryRef = cloneString(result.QualitySummaryRef)
	stage.DiagnosticRefs = append([]string(nil), result.DiagnosticRefs...)
	stage.ReasonCodes = append([]string(nil), result.ReasonCodes...)
	stage.QualityStatus = result.QualityStatus
	stage.RiskDecision = result.RiskDecision
	stage.FinishedAt = nil
	commit := WorkerResultCommit{
		RunRef: runRef, StageRef: stageRef, AttemptRef: attempt.Ref,
		WorkerID: w.config.WorkerID, FencingToken: attempt.FencingToken,
		AttemptStatus: result.AttemptStatus, Stage: stage,
		FaultRef: cloneString(result.FaultRef), OutputRef: cloneString(result.OutputRef),
	}
	if err := w.store.CommitWorkerResult(ctx, commit); err != nil {
		return WorkerExecution{Attempt: attempt}, fmt.Errorf("commit Researchflow Stage result: %w", err)
	}
	return WorkerExecution{Attempt: attempt, Committed: true}, nil
}

func (w *Worker) heartbeat(ctx context.Context, cancel context.CancelFunc, snapshot StageExecutionSnapshot, attempt domain.Attempt,
	stop <-chan struct{}, done chan<- struct{}, failures chan<- error) {
	defer close(done)
	ticker := time.NewTicker(w.config.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				select { case failures <- ErrWorkerDeadline: default: }
			}
			return
		case <-ticker.C:
			if _, err := w.store.RenewAttemptLease(ctx, snapshot.Run.Ref, snapshot.Stage.Ref,
				attempt.Ref, w.config.WorkerID, attempt.FencingToken, w.config.LeaseDuration); err != nil {
				select { case failures <- err: default: }
				cancel()
				return
			}
		}
	}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil { return nil }
	copy := *value
	return &copy
}

