package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

// RunFinalizationRouterStore loads only the persisted Run routing fields.
// Each selected finalizer reloads its own authoritative snapshot before commit.
type RunFinalizationRouterStore interface {
	LoadRunFinalizationSnapshot(context.Context, string) (DeadlineFinalizationSnapshot, error)
}

type NormalRunFinalizer interface {
	Finalize(context.Context, string) (RunFinalizationResult, error)
}

type ExpiredRunFinalizer interface {
	Finalize(context.Context, string) (DeadlineFinalizationResult, error)
}

type CancelledRunFinalizer interface {
	Finalize(context.Context, string) (CancellationFinalizationResult, error)
}

type RunFinalizationPath string

const (
	RunFinalizationPathNormal	RunFinalizationPath = "normal"
	RunFinalizationPathDeadline	RunFinalizationPath = "deadline"
	RunFinalizationPathCancellation	RunFinalizationPath = "cancellation"
)

type CoordinatedRunFinalizationResult struct {
	Run       domain.Run
	Path      RunFinalizationPath
	Finalized bool
}

// RunFinalizationCoordinator routes by persisted status and termination reason.
// It does not aggregate state or persist results; those remain finalizer duties.
type RunFinalizationCoordinator struct {
	store        RunFinalizationRouterStore
	normal       NormalRunFinalizer
	deadline     ExpiredRunFinalizer
	cancellation CancelledRunFinalizer
}

func NewRunFinalizationCoordinator(store RunFinalizationRouterStore, normal NormalRunFinalizer, deadline ExpiredRunFinalizer, cancellation CancelledRunFinalizer) (*RunFinalizationCoordinator, error) {
	if store == nil || normal == nil || deadline == nil || cancellation == nil {
		return nil, errors.New("Researchflow run finalization coordinator dependencies are required")
	}
	return &RunFinalizationCoordinator{store: store, normal: normal, deadline: deadline, cancellation: cancellation}, nil
}

func (c *RunFinalizationCoordinator) Finalize(ctx context.Context, runRef string) (CoordinatedRunFinalizationResult, error) {
	if c == nil || c.store == nil || c.normal == nil || c.deadline == nil || c.cancellation == nil {
		return CoordinatedRunFinalizationResult{}, errors.New("Researchflow run finalization coordinator is not configured")
	}
	if strings.TrimSpace(runRef) == "" {
		return CoordinatedRunFinalizationResult{}, errors.New("Researchflow Run reference is required")
	}
	snapshot, err := c.store.LoadRunFinalizationSnapshot(ctx, runRef)
	if err != nil {
		return CoordinatedRunFinalizationResult{}, fmt.Errorf("load Researchflow Run finalization routing state: %w", err)
	}
	if snapshot.Run.Ref != runRef || snapshot.Run.Version == 0 {
		return CoordinatedRunFinalizationResult{}, errors.New("Researchflow Run routing snapshot has an invalid reference or version")
	}

	switch {
	case snapshot.Run.Status == domain.RunRunning && snapshot.Run.TerminationReason == "":
		result, err := c.normal.Finalize(ctx, runRef)
		if err != nil {
			return CoordinatedRunFinalizationResult{}, err
		}
		return CoordinatedRunFinalizationResult{Run: result.Run, Path: RunFinalizationPathNormal, Finalized: result.Finalized}, nil

	case (snapshot.Run.Status == domain.RunCreated || snapshot.Run.Status == domain.RunQueued || snapshot.Run.Status == domain.RunRunning) &&
		snapshot.Run.TerminationReason == "deadline_exceeded":
		result, err := c.deadline.Finalize(ctx, runRef)
		if err != nil {
			return CoordinatedRunFinalizationResult{}, err
		}
		return CoordinatedRunFinalizationResult{Run: result.Run, Path: RunFinalizationPathDeadline, Finalized: result.Finalized}, nil

	case snapshot.Run.Status == domain.RunCancelling && snapshot.Run.TerminationReason == "user_cancelled":
		result, err := c.cancellation.Finalize(ctx, runRef)
		if err != nil {
			return CoordinatedRunFinalizationResult{}, err
		}
		return CoordinatedRunFinalizationResult{Run: result.Run, Path: RunFinalizationPathCancellation, Finalized: result.Finalized}, nil

	default:
		return CoordinatedRunFinalizationResult{}, fmt.Errorf("Researchflow Run state %q with termination reason %q has no eligible finalization path",
			snapshot.Run.Status, snapshot.Run.TerminationReason)
	}
}
