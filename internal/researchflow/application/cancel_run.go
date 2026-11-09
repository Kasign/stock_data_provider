package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

var (
	ErrRunAccessDenied            = errors.New("Researchflow Run access denied")
	ErrRunVersionConflict         = errors.New("Researchflow Run changed during cancellation")
	ErrRunCancellationDenied      = errors.New("Researchflow Run cannot be cancelled")
	ErrRunAlreadyTerminal         = errors.New("Researchflow run is already terminal")
	ErrRunCancellationUnavailable = errors.New("Researchflow run is unavailable for cancellation")
)

// RunAccessAuthorizer must verify ownership or an explicit read/write grant
// against trusted caller identity and persisted Run ownership.
type RunAccessAuthorizer interface {
	AuthorizeRunCancellation(context.Context, string, string) error
}

// RunCancellationStore is the durable cancellation linearization port.
type RunCancellationStore interface {
	BeginRunCancellation(context.Context, string, uint64) (bool, error)
}

type CancelRunResult struct {
	Run      domain.Run
	Stages   []domain.Stage
	Started  bool
	Replayed bool
}

// CancelRunService persists cancellation intent. It does not claim that active
// workers have stopped; worker confirmation/fencing and finalization follow.
type CancelRunService struct {
	identity   RequestIdentity
	authorizer RunAccessAuthorizer
	reader     RunReader
	cancellers RunCancellationStore
}

func NewCancelRunService(
	identity RequestIdentity,
	authorizer RunAccessAuthorizer,
	reader RunReader,
	cancellers RunCancellationStore,
) (*CancelRunService, error) {
	if identity == nil || authorizer == nil || reader == nil || cancellers == nil {
		return nil, errors.New("Researchflow CancelRun dependencies are incomplete")
	}
	return &CancelRunService{
		identity: identity, authorizer: authorizer, reader: reader, cancellers: cancellers,
	}, nil
}

func (s *CancelRunService) CancelRun(ctx context.Context, runRef string) (CancelRunResult, error) {
	if s == nil || s.identity == nil || s.authorizer == nil || s.reader == nil || s.cancellers == nil {
		return CancelRunResult{}, errors.New("Researchflow CancelRun service is not configured")
	}
	if strings.TrimSpace(runRef) == "" || strings.TrimSpace(runRef) != runRef || len(runRef) > 255 {
		return CancelRunResult{}, fmt.Errorf("%w: Run reference is invalid", domain.ErrInvalidCommand)
	}
	callerRef, err := s.identity.AuthenticatedCallerRef(ctx)
	if err != nil || strings.TrimSpace(callerRef) == "" {
		return CancelRunResult{}, fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}
	if err := s.authorizer.AuthorizeRunCancellation(ctx, callerRef, runRef); err != nil {
		if errors.Is(err, ErrRunAccessDenied) {
			return CancelRunResult{}, ErrRunAccessDenied
		}
		return CancelRunResult{}, fmt.Errorf("authorize Researchflow Run cancellation: %w", err)
	}

	current, err := s.reader.GetRun(ctx, runRef)
	if err != nil {
		return CancelRunResult{}, fmt.Errorf("load Researchflow Run before cancellation: %w", err)
	}
	if current.Run.Ref != runRef || current.Run.Version == 0 {
		return CancelRunResult{}, errors.New("Researchflow Run reader returned an inconsistent cancellation snapshot")
	}
	if current.Run.Status == domain.RunCancelling && current.Run.TerminationReason == "user_cancelled" {
		return cancelResult(current, false, true), nil
	}
	if current.Run.Status.Terminal() || current.Run.TerminationReason == "deadline_exceeded" {
		return CancelRunResult{}, ErrRunCancellationDenied
	}
	switch current.Run.Status {
	case domain.RunCreated, domain.RunQueued, domain.RunRunning:
	default:
		return CancelRunResult{}, ErrRunCancellationDenied
	}

	changed, err := s.cancellers.BeginRunCancellation(ctx, runRef, current.Run.Version)
	if err != nil {
		if errors.Is(err, ErrRunAlreadyTerminal) || errors.Is(err, ErrRunCancellationUnavailable) {
			return CancelRunResult{}, ErrRunCancellationDenied
		}
		return CancelRunResult{}, fmt.Errorf("persist Researchflow Run cancellation: %w", err)
	}
	latest, err := s.reader.GetRun(ctx, runRef)
	if err != nil {
		return CancelRunResult{}, fmt.Errorf("reload Researchflow Run after cancellation: %w", err)
	}
	if latest.Run.Ref != runRef || latest.Run.Version == 0 {
		return CancelRunResult{}, errors.New("Researchflow Run reader returned an inconsistent post-cancellation snapshot")
	}
	if latest.Run.Status != domain.RunCancelling || latest.Run.TerminationReason != "user_cancelled" {
		return CancelRunResult{}, ErrRunVersionConflict
	}
	return cancelResult(latest, changed, !changed), nil
}

func cancelResult(result CreateRunResult, started, replayed bool) CancelRunResult {
	return CancelRunResult{
		Run:      result.Run,
		Stages:   append([]domain.Stage(nil), result.Stages...),
		Started:  started,
		Replayed: replayed,
	}
}
