package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/state"
)

// DependencyOutputValidator validates the upstream output against the downstream
// stage's input contract. It also owns any schema/quality compatibility rules.
type DependencyOutputValidator interface {
	ValidateDependency(context.Context, domain.Stage, domain.Stage) (bool, error)
}

type SchedulePlan struct {
	PromoteToReady []domain.Stage
	MarkBlocked    []domain.Stage
	Dispatch       []domain.Stage
	WaitingRefs    []string
	ActiveCount    int
	AvailableSlots int
}

// BuildSchedulePlan computes a deterministic, bounded dispatch plan from a
// persisted Run/Stage snapshot. It does not persist transitions or reserve slots;
// ClaimAttempt remains the cross-process authority and must enforce concurrency
// limits transactionally before creating an Attempt.
func BuildSchedulePlan(
	ctx context.Context,
	run domain.Run,
	stages []domain.Stage,
	priorities map[string]int,
	maxParallel int,
	now time.Time,
	validator DependencyOutputValidator,
) (SchedulePlan, error) {
	if maxParallel < 1 {
		return SchedulePlan{}, errors.New("Researchflow scheduler max_parallel must be positive")
	}
	if run.Status != domain.RunRunning || run.TerminationReason != "" {
		return SchedulePlan{}, fmt.Errorf("Researchflow Run is not schedulable: status=%s", run.Status)
	}
	now = now.UTC()
	if run.DeadlineAt.IsZero() || !now.Before(run.DeadlineAt.UTC()) {
		return SchedulePlan{}, state.ErrDeadlineExceeded
	}
	byRef := make(map[string]domain.Stage, len(stages))
	seenCodes := make(map[string]struct{}, len(stages))
	for _, stage := range stages {
		if strings.TrimSpace(stage.Ref) == "" || strings.TrimSpace(stage.Code) == "" || stage.RunRef != run.Ref {
			return SchedulePlan{}, errors.New("Researchflow scheduler received an invalid or cross-run Stage")
		}
		if _, exists := byRef[stage.Ref]; exists {
			return SchedulePlan{}, fmt.Errorf("Researchflow scheduler found duplicate Stage ref %q", stage.Ref)
		}
		if _, exists := seenCodes[stage.Code]; exists {
			return SchedulePlan{}, fmt.Errorf("Researchflow scheduler found duplicate Stage code %q", stage.Code)
		}
		byRef[stage.Ref] = stage
		seenCodes[stage.Code] = struct{}{}
	}
	plan := SchedulePlan{}
	ready := make([]domain.Stage, 0, len(stages))
	for _, stage := range stages {
		switch stage.Status {
		case domain.StageRunning:
			plan.ActiveCount++
		case domain.StageReady:
			ready = append(ready, stage)
		case domain.StagePending:
			allDependenciesTerminal := true
			dependencyBlocked := false
			for _, dependencyRef := range stage.Dependencies {
				dependency, exists := byRef[dependencyRef]
				if !exists {
					return SchedulePlan{}, fmt.Errorf("Researchflow Stage %q references missing dependency %q", stage.Ref, dependencyRef)
				}
				if !dependency.Status.Terminal() {
					allDependenciesTerminal = false
					break
				}
				if dependency.Status != domain.StageSucceeded && dependency.Status != domain.StageDegraded {
					dependencyBlocked = true
					break
				}
				if dependency.OutputRef == nil || strings.TrimSpace(*dependency.OutputRef) == "" {
					dependencyBlocked = true
					break
				}
				if validator == nil {
					return SchedulePlan{}, errors.New("Researchflow dependency output validator is required")
				}
				valid, err := validator.ValidateDependency(ctx, dependency, stage)
				if err != nil {
					return SchedulePlan{}, fmt.Errorf("validate dependency %q for Stage %q: %w", dependency.Ref, stage.Ref, err)
				}
				if !valid {
					dependencyBlocked = true
					break
				}
			}
			if !allDependenciesTerminal {
				plan.WaitingRefs = append(plan.WaitingRefs, stage.Ref)
				continue
			}
			nextStatus := domain.StageReady
			if dependencyBlocked {
				nextStatus = domain.StageBlocked
			}
			updated, err := state.TransitionStage(stage, nextStatus, now)
			if err != nil {
				return SchedulePlan{}, fmt.Errorf("plan Stage %q transition: %w", stage.Ref, err)
			}
			if dependencyBlocked {
				updated.ReasonCodes = appendUniqueReason(updated.ReasonCodes, "dependency_not_satisfied")
				plan.MarkBlocked = append(plan.MarkBlocked, updated)
			} else {
				plan.PromoteToReady = append(plan.PromoteToReady, updated)
				ready = append(ready, updated)
			}
		case domain.StageRetryWait:
			plan.WaitingRefs = append(plan.WaitingRefs, stage.Ref)
		}
	}
	less := func(a, b domain.Stage) bool {
		pa, pb := priorities[a.Ref], priorities[b.Ref]
		if pa != pb {
			return pa < pb
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Ref < b.Ref
	}
	sort.Slice(ready, func(i, j int) bool { return less(ready[i], ready[j]) })
	sort.Strings(plan.WaitingRefs)
	plan.AvailableSlots = maxParallel - plan.ActiveCount
	if plan.AvailableSlots < 0 {
		plan.AvailableSlots = 0
	}
	dispatchCount := len(ready)
	if dispatchCount > plan.AvailableSlots {
		dispatchCount = plan.AvailableSlots
	}
	plan.Dispatch = append([]domain.Stage(nil), ready[:dispatchCount]...)
	return plan, nil
}

func appendUniqueReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}
