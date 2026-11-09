package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/state"
)

var (
	ErrRetryStageUnavailable = errors.New("Researchflow Stage is not eligible for retry")
	ErrRetryStageNotDue      = errors.New("Researchflow Stage retry time has not arrived")
	ErrRetryStageNotRetryable = errors.New("Researchflow Stage failure reason is not retryable")
	ErrRetryStageBudgetExhausted = errors.New("Researchflow Stage retry budget is exhausted")
)

// RetryStageSnapshotReader returns the persisted Run, Stage and immutable retry policy.
type RetryStageSnapshotReader interface {
	LoadStageExecutionSnapshot(context.Context, string, string) (StageExecutionSnapshot, error)
}

// RetryStageExecutor delegates the actual claim and execution to Worker.
// The Worker/Storage transaction remains authoritative for concurrency, deadline,
// attempt numbering, lease ownership and fencing.
type RetryStageExecutor interface {
	Execute(context.Context, string, string, string) (WorkerExecution, error)
}

type RetryStageService struct {
	reader   RetryStageSnapshotReader
	clock    DatabaseClock
	executor RetryStageExecutor
}

func NewRetryStageService(reader RetryStageSnapshotReader, clock DatabaseClock, executor RetryStageExecutor) (*RetryStageService, error) {
	if reader == nil || clock == nil || executor == nil {
		return nil, errors.New("Researchflow RetryStage dependencies are incomplete")
	}
	return &RetryStageService{reader: reader, clock: clock, executor: executor}, nil
}

// RetryStage is an internal Scheduler dispatch use case, not a user-requested
// manual rerun. attemptRef must be stable across redelivery of the same dispatch.
func (s *RetryStageService) RetryStage(ctx context.Context, runRef, stageRef, attemptRef string) (WorkerExecution, error) {
	if s == nil || s.reader == nil || s.clock == nil || s.executor == nil {
		return WorkerExecution{}, errors.New("Researchflow RetryStage service is not configured")
	}
	if !validRetryReference(runRef) || !validRetryReference(stageRef) || !validRetryReference(attemptRef) {
		return WorkerExecution{}, fmt.Errorf("%w: Run, Stage and Attempt references are required", domain.ErrInvalidCommand)
	}
	snapshot, err := s.reader.LoadStageExecutionSnapshot(ctx, runRef, stageRef)
	if err != nil {
		return WorkerExecution{}, fmt.Errorf("load Researchflow retry snapshot: %w", err)
	}
	if snapshot.Run.Ref != runRef || snapshot.Stage.Ref != stageRef || snapshot.Stage.RunRef != runRef ||
		snapshot.Run.Version == 0 || snapshot.Stage.Version == 0 {
		return WorkerExecution{}, errors.New("Researchflow retry snapshot identity or version is inconsistent")
	}
	if snapshot.Run.Status != domain.RunRunning || snapshot.Run.TerminationReason != "" {
		return WorkerExecution{}, ErrRetryStageUnavailable
	}
	if snapshot.Stage.Status != domain.StageRetryWait || snapshot.Stage.RetryAt == nil || snapshot.Stage.AttemptNo < 1 {
		return WorkerExecution{}, ErrRetryStageUnavailable
	}
	if snapshot.RetryPolicy.MaxAttempts < 1 || snapshot.Stage.AttemptNo >= snapshot.RetryPolicy.MaxAttempts {
		return WorkerExecution{}, ErrRetryStageBudgetExhausted
	}
	if !hasRetryableReason(snapshot.Stage.ReasonCodes, snapshot.RetryPolicy.RetryableReasonCodes) {
		return WorkerExecution{}, ErrRetryStageNotRetryable
	}
	now, err := s.clock.NowUTC(ctx)
	if err != nil {
		return WorkerExecution{}, fmt.Errorf("read database UTC time for Researchflow retry: %w", err)
	}
	now = now.UTC()
	if snapshot.Run.DeadlineAt.IsZero() || !now.Before(snapshot.Run.DeadlineAt.UTC()) {
		return WorkerExecution{}, state.ErrDeadlineExceeded
	}
	if now.Before(snapshot.Stage.RetryAt.UTC()) {
		return WorkerExecution{}, ErrRetryStageNotDue
	}
	return s.executor.Execute(ctx, runRef, stageRef, attemptRef)
}

func validRetryReference(value string) bool {
	return strings.TrimSpace(value) != "" && strings.TrimSpace(value) == value && len(value) <= 255
}
