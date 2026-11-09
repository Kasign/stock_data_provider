package application

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

var (
	ErrWorkerLeaseLost = errors.New("Researchflow worker lease lost")
	ErrWorkerDeadline  = errors.New("Researchflow run deadline exceeded")
	ErrWorkerCancelled = errors.New("Researchflow run cancelled")
)

type StageExecutionSnapshot struct {
	Run               domain.Run
	Stage             domain.Stage
	Parameters        []byte
	InputSnapshotRefs []string
	RetryPolicy       template.RetryPolicy
}

type WorkerResultCommit struct {
	RunRef        string
	StageRef      string
	AttemptRef    string
	WorkerID      string
	FencingToken  uint64
	AttemptStatus domain.AttemptStatus
	Stage         domain.Stage
	FaultRef      *string
	OutputRef     *string
}

// WorkerStore is the Application boundary. Its persistence adapter must map
// CommitWorkerResult to the atomic storage CommitAttemptResult transaction.
type ExecutionFault struct {
	FaultRef   string
	ReasonCode string
}

// ExecutionFaultNormalizer maps provider/adapter errors to a persisted Fault
// reference and a stable reason code. Retryability remains controlled only by
// the immutable template RetryPolicy allowlist.
type ExecutionFaultNormalizer interface {
	NormalizeExecutionError(context.Context, error) (ExecutionFault, error)
}

type WorkerStore interface {
	ClaimAttempt(context.Context, string, string, string, string, time.Duration, int, int) (domain.Attempt, bool, error)
	LoadStageExecutionSnapshot(context.Context, string, string) (StageExecutionSnapshot, error)
	RenewAttemptLease(context.Context, string, string, string, string, uint64, time.Duration) (time.Time, error)
	CommitWorkerResult(context.Context, WorkerResultCommit) error
	ConfirmAttemptCancellation(context.Context, string, string, string, string, uint64, uint64) (bool, error)
}

type WorkerConfig struct {
	WorkerID          string
	LeaseDuration     time.Duration
	HeartbeatInterval time.Duration
	MaxParallel       int
}

type WorkerExecution struct {
	Attempt   domain.Attempt
	Replayed  bool
	Committed bool
}

type Worker struct {
	store         WorkerStore
	executor      StageExecutor
	clock         DatabaseClock
	config        WorkerConfig
	now           func() time.Time
	cancellations *workerCancellationRegistry
	faultNormalizer ExecutionFaultNormalizer
}

func NewWorker(store WorkerStore, executor StageExecutor, clock DatabaseClock, config WorkerConfig) (*Worker, error) {
	if store == nil || executor == nil || clock == nil || strings.TrimSpace(config.WorkerID) == "" ||
		len(config.WorkerID) > 128 || config.LeaseDuration <= 0 ||
		config.HeartbeatInterval <= 0 || config.HeartbeatInterval >= config.LeaseDuration ||
		config.MaxParallel <= 0 {
		return nil, errors.New("Researchflow worker dependencies or configuration are invalid")
	}
	return &Worker{store: store, executor: executor, clock: clock, config: config, now: time.Now, cancellations: newWorkerCancellationRegistry()}, nil
}

// NewWorkerWithFaultNormalizer enables controlled Provider error normalization.
// Without this port, Provider errors remain fail-closed and are not committed.
func NewWorkerWithFaultNormalizer(store WorkerStore, executor StageExecutor, clock DatabaseClock, config WorkerConfig, normalizer ExecutionFaultNormalizer) (*Worker, error) {
	if normalizer == nil {
		return nil, errors.New("Researchflow execution fault normalizer is required")
	}
	worker, err := NewWorker(store, executor, clock, config)
	if err != nil {
		return nil, err
	}
	worker.faultNormalizer = normalizer
	return worker, nil
}

// CancelRun signals all locally active Attempts for a durably-cancelling Run.
func (w *Worker) CancelRun(runRef string) int {
	if w == nil || w.cancellations == nil || strings.TrimSpace(runRef) == "" {
		return 0
	}
	return w.cancellations.cancelRun(runRef)
}

// Execute claims and runs one Stage. The store remains the concurrency,
// deadline, lease and fencing authority; this method only coordinates ports.
func (w *Worker) Execute(ctx context.Context, runRef, stageRef, attemptRef string) (WorkerExecution, error) {
	if strings.TrimSpace(runRef) == "" || strings.TrimSpace(stageRef) == "" || strings.TrimSpace(attemptRef) == "" {
		return WorkerExecution{}, errors.New("Researchflow worker references are required")
	}
	policySnapshot, err := w.store.LoadStageExecutionSnapshot(ctx, runRef, stageRef)
	if err != nil {
		return WorkerExecution{}, fmt.Errorf("load Researchflow retry policy snapshot: %w", err)
	}
	if policySnapshot.Run.Ref != runRef || policySnapshot.Stage.Ref != stageRef ||
		policySnapshot.Stage.RunRef != runRef || policySnapshot.RetryPolicy.MaxAttempts <= 0 {
		return WorkerExecution{}, errors.New("Researchflow retry policy snapshot is inconsistent")
	}
	attempt, replayed, err := w.store.ClaimAttempt(ctx, runRef, stageRef, attemptRef, w.config.WorkerID,
		w.config.LeaseDuration, policySnapshot.RetryPolicy.MaxAttempts, w.config.MaxParallel)
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
		snapshot.Run.DeadlineAt.IsZero() || snapshot.RetryPolicy.AttemptTimeoutMS <= 0 ||
		!reflect.DeepEqual(snapshot.RetryPolicy, policySnapshot.RetryPolicy) {
		return WorkerExecution{Attempt: attempt}, errors.New("Researchflow execution snapshot is inconsistent with claimed Attempt")
	}
	if !w.now().Before(snapshot.Run.DeadlineAt) {
		return WorkerExecution{Attempt: attempt}, ErrWorkerDeadline
	}

	runCtx, cancelRun := context.WithDeadline(ctx, snapshot.Run.DeadlineAt)
	defer cancelRun()
	if attempt.StartedAt.IsZero() {
		return WorkerExecution{Attempt: attempt}, errors.New("Researchflow Attempt start time is missing")
	}
	attemptTimeout := time.Duration(snapshot.RetryPolicy.AttemptTimeoutMS) * time.Millisecond
	if attemptTimeout <= 0 || int64(attemptTimeout/time.Millisecond) != snapshot.RetryPolicy.AttemptTimeoutMS {
		return WorkerExecution{Attempt: attempt}, errors.New("Researchflow Attempt timeout is outside supported duration")
	}
	attemptDeadline := attempt.StartedAt.Add(attemptTimeout)
	if attemptDeadline.After(snapshot.Run.DeadlineAt) {
		attemptDeadline = snapshot.Run.DeadlineAt
	}
	baseExecCtx, cancelExecutionCause := context.WithCancelCause(runCtx)
	defer cancelExecutionCause(nil)
	execCtx, cancelAttemptDeadline := context.WithDeadline(baseExecCtx, attemptDeadline)
	defer cancelAttemptDeadline()
	unregisterCancellation := w.cancellations.register(runRef, attempt.Ref, cancelExecutionCause)
	defer unregisterCancellation()
	stopHeartbeat := make(chan struct{})
	heartbeatDone := make(chan struct{})
	heartbeatErr := make(chan error, 1)
	go w.heartbeat(runCtx, cancelRun, snapshot, attempt, stopHeartbeat, heartbeatDone, heartbeatErr)

	request := StageExecutionRequest{
		Run: snapshot.Run, Stage: snapshot.Stage, Attempt: attempt,
		Parameters:        append([]byte(nil), snapshot.Parameters...),
		InputSnapshotRefs: append([]string(nil), snapshot.InputSnapshotRefs...),
		SystemAsOf:        snapshot.Run.SystemAsOf, DeadlineAt: snapshot.Run.DeadlineAt,
		AttemptDeadlineAt: attemptDeadline,
		CorrelationID:     snapshot.Run.CorrelationID,
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
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) || !w.now().Before(snapshot.Run.DeadlineAt) {
		return WorkerExecution{Attempt: attempt}, ErrWorkerDeadline
	}
	if errors.Is(context.Cause(execCtx), ErrWorkerCancelled) {
		confirmed, confirmErr := w.store.ConfirmAttemptCancellation(ctx, runRef, stageRef, attempt.Ref, w.config.WorkerID, attempt.FencingToken, snapshot.Stage.Version)
		if confirmErr != nil {
			return WorkerExecution{Attempt: attempt}, fmt.Errorf("confirm Researchflow Attempt cancellation: %w", confirmErr)
		}
		if !confirmed {
			return WorkerExecution{Attempt: attempt}, ErrWorkerLeaseLost
		}
		return WorkerExecution{Attempt: attempt}, nil
	}
	attemptTimedOut := errors.Is(execCtx.Err(), context.DeadlineExceeded) && runCtx.Err() == nil
	if attemptTimedOut {
		result = StageExecutionResult{
			AttemptStatus: domain.AttemptTimedOut,
			StageStatus:   domain.StageRetryWait,
			ReasonCodes:   []string{"attempt_timeout"},
		}
	} else if executeErr != nil {
		if execCtx.Err() != nil {
			return WorkerExecution{Attempt: attempt}, fmt.Errorf("Researchflow Stage execution context ended: %w", execCtx.Err())
		}
		if w.faultNormalizer == nil {
			return WorkerExecution{Attempt: attempt}, fmt.Errorf("execute Researchflow Stage: %w", executeErr)
		}
		fault, normalizeErr := w.faultNormalizer.NormalizeExecutionError(ctx, executeErr)
		if normalizeErr != nil {
			return WorkerExecution{Attempt: attempt}, fmt.Errorf("normalize Researchflow execution fault: %w", normalizeErr)
		}
		if strings.TrimSpace(fault.FaultRef) == "" || strings.TrimSpace(fault.ReasonCode) == "" {
			return WorkerExecution{Attempt: attempt}, errors.New("Researchflow execution fault normalizer returned incomplete evidence")
		}
		faultRef := fault.FaultRef
		result = StageExecutionResult{
			AttemptStatus: domain.AttemptFailed,
			StageStatus:   domain.StageRetryWait,
			FaultRef:      &faultRef,
			ReasonCodes:   []string{fault.ReasonCode},
		}
	}
	if err := ValidateStageExecutionResult(result); err != nil {
		return WorkerExecution{Attempt: attempt}, err
	}
	if result.StageStatus == domain.StageRetryWait {
		retryNow, retryErr := w.clock.NowUTC(ctx)
		if retryErr != nil {
			return WorkerExecution{Attempt: attempt}, fmt.Errorf("read database UTC time for retry policy: %w", retryErr)
		}
		retryAt, retryAllowed, retryErr := CalculateRetryAt(snapshot.RetryPolicy, attempt.Number, result.ReasonCodes, retryNow, CryptoRetryJitter{})
		if retryErr != nil {
			return WorkerExecution{Attempt: attempt}, retryErr
		}
		if retryAllowed {
			result.RetryAt = &retryAt
		} else {
			if result.AttemptStatus == domain.AttemptTimedOut {
				result.StageStatus = domain.StageTimedOut
			} else {
				result.StageStatus = domain.StageFailed
			}
			result.RetryAt = nil
		}
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
				select {
				case failures <- ErrWorkerDeadline:
				default:
				}
			}
			return
		case <-ticker.C:
			if _, err := w.store.RenewAttemptLease(ctx, snapshot.Run.Ref, snapshot.Stage.Ref,
				attempt.Ref, w.config.WorkerID, attempt.FencingToken, w.config.LeaseDuration); err != nil {
				select {
				case failures <- err:
				default:
				}
				cancel()
				return
			}
		}
	}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
