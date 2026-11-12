package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
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
// persisted Run/Stage snapshot. It plans state transitions separately from dispatch;
// ClaimAttempt remains the cross-process authority and enforces concurrency limits
// transactionally before creating an Attempt.
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
	switch run.Status {
	case domain.RunCreated, domain.RunQueued, domain.RunRunning:
		if run.TerminationReason != "" {
			return SchedulePlan{}, fmt.Errorf("Researchflow Run is not schedulable: status=%s", run.Status)
		}
	default:
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
					continue
				}
				if dependency.Status != domain.StageSucceeded && dependency.Status != domain.StageDegraded {
					dependencyBlocked = true
					continue
				}
				if dependency.OutputRef == nil || strings.TrimSpace(*dependency.OutputRef) == "" {
					dependencyBlocked = true
					continue
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
				}
			}
			if !allDependenciesTerminal && !dependencyBlocked {
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
			if stage.RetryAt == nil {
				return SchedulePlan{}, fmt.Errorf("Researchflow retry_wait Stage %q has no retry_at", stage.Ref)
			}
			if stage.RetryAt.After(now) {
				plan.WaitingRefs = append(plan.WaitingRefs, stage.Ref)
				continue
			}
			// Keep the persisted state as retry_wait. ClaimAttempt validates the
			// retry history, DB deadline and retry_at under the Run row lock,
			// then atomically creates the next Attempt and moves Stage to running.
			ready = append(ready, stage)
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

// SchedulePlanStore persists scheduler-owned Stage transitions using optimistic
// version checks. Each Stage is an independent CAS; after a conflict the caller
// must reload the Run snapshot and rebuild the whole plan before dispatching.
type SchedulePlanStore interface {
	CompareAndSwapStage(context.Context, uint64, domain.Stage) (bool, error)
}

var ErrSchedulePlanConflict = errors.New("Researchflow schedule plan snapshot is stale")

type SchedulePlanPersistResult struct {
	Promoted int
	Blocked  int
}

// PersistSchedulePlan applies only Pending -> Ready and Pending -> Blocked
// transitions. Dispatch is advisory: ClaimAttempt remains the authority that
// atomically reserves an Attempt and enforces cross-worker concurrency.
func PersistSchedulePlan(ctx context.Context, store SchedulePlanStore, plan SchedulePlan) (SchedulePlanPersistResult, error) {
	if store == nil {
		return SchedulePlanPersistResult{}, errors.New("Researchflow schedule plan store is required")
	}
	result := SchedulePlanPersistResult{}
	transitions := make([]domain.Stage, 0, len(plan.PromoteToReady)+len(plan.MarkBlocked))
	transitions = append(transitions, plan.PromoteToReady...)
	transitions = append(transitions, plan.MarkBlocked...)
	sort.Slice(transitions, func(i, j int) bool { return transitions[i].Ref < transitions[j].Ref })
	seen := make(map[string]struct{}, len(transitions))
	for _, stage := range transitions {
		if strings.TrimSpace(stage.Ref) == "" || stage.Version == 0 {
			return result, errors.New("Researchflow schedule plan contains an invalid Stage transition")
		}
		if _, exists := seen[stage.Ref]; exists {
			return result, fmt.Errorf("Researchflow schedule plan contains duplicate Stage transition %q", stage.Ref)
		}
		seen[stage.Ref] = struct{}{}
		if stage.Status != domain.StageReady && stage.Status != domain.StageBlocked {
			return result, fmt.Errorf("Researchflow schedule plan contains unsupported transition target %q", stage.Status)
		}
		applied, err := store.CompareAndSwapStage(ctx, stage.Version-1, stage)
		if err != nil {
			return result, fmt.Errorf("persist Researchflow Stage %q schedule transition: %w", stage.Ref, err)
		}
		if !applied {
			return result, fmt.Errorf("%w: stage=%s", ErrSchedulePlanConflict, stage.Ref)
		}
		if stage.Status == domain.StageReady {
			result.Promoted++
		} else {
			result.Blocked++
		}
	}
	return result, nil
}

// StageExecutionDispatcher delegates a planned Stage to the existing Worker.
// Worker.Execute performs the authoritative ClaimAttempt transaction; this
// interface does not introduce a second queue or a second concurrency gate.
type StageExecutionDispatcher interface {
	Execute(context.Context, string, string, string) (WorkerExecution, error)
}

var ErrScheduleReplanExhausted = errors.New("Researchflow scheduler exhausted snapshot replan attempts")

type SchedulerConfig struct {
	MaxParallel int
	MaxReplans  int
}

type ScheduledStageExecution struct {
	StageRef   string
	AttemptRef string
	Execution  WorkerExecution
}

type ScheduleRunResult struct {
	Plan        SchedulePlan
	Persisted   SchedulePlanPersistResult
	Executions  []ScheduledStageExecution
	ReplanCount int
}

// Scheduler coordinates one scheduling pass for a persisted Run. It is
// intentionally invoked by the Server lifecycle; periodic scanning and Worker
// Pool ownership remain outside this Application service.
type Scheduler struct {
	reader     RunReader
	store      SchedulePlanStore
	clock      DatabaseClock
	ids        IDGenerator
	validator  DependencyOutputValidator
	dispatcher StageExecutionDispatcher
	config     SchedulerConfig
}

func NewScheduler(
	reader RunReader,
	store SchedulePlanStore,
	clock DatabaseClock,
	ids IDGenerator,
	validator DependencyOutputValidator,
	dispatcher StageExecutionDispatcher,
	config SchedulerConfig,
) (*Scheduler, error) {
	if reader == nil || store == nil || clock == nil || ids == nil || dispatcher == nil ||
		config.MaxParallel <= 0 || config.MaxReplans <= 0 {
		return nil, errors.New("Researchflow scheduler dependencies or configuration are invalid")
	}
	return &Scheduler{
		reader: reader, store: store, clock: clock, ids: ids,
		validator: validator, dispatcher: dispatcher, config: config,
	}, nil
}

// ScheduleRun reloads and recomputes the complete plan after any Stage CAS
// conflict. Only a successfully persisted plan may be dispatched. ClaimAttempt
// remains the final cross-process concurrency, deadline, lease and fencing gate.
func (s *Scheduler) ScheduleRun(ctx context.Context, runRef string) (ScheduleRunResult, error) {
	if s == nil || s.reader == nil || s.store == nil || s.clock == nil ||
		s.ids == nil || s.dispatcher == nil {
		return ScheduleRunResult{}, errors.New("Researchflow scheduler is not configured")
	}
	if strings.TrimSpace(runRef) == "" || runRef != strings.TrimSpace(runRef) || len(runRef) > 255 {
		return ScheduleRunResult{}, errors.New("Researchflow scheduler Run reference is invalid")
	}

	result := ScheduleRunResult{Executions: []ScheduledStageExecution{}}
	for attempt := 0; attempt < s.config.MaxReplans; attempt++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		snapshot, err := s.reader.GetRun(ctx, runRef)
		if err != nil {
			return result, fmt.Errorf("load Researchflow scheduler snapshot: %w", err)
		}
		if snapshot.Run.Ref != runRef {
			return result, errors.New("Researchflow scheduler reader returned another Run")
		}
		now, err := s.clock.NowUTC(ctx)
		if err != nil {
			return result, fmt.Errorf("read Researchflow scheduler database clock: %w", err)
		}
		if now.IsZero() {
			return result, errors.New("Researchflow scheduler database clock returned a zero timestamp")
		}
		if _, offset := now.Zone(); offset != 0 {
			return result, errors.New("Researchflow scheduler database clock must return UTC")
		}

		plan, err := BuildSchedulePlan(ctx, snapshot.Run, snapshot.Stages, nil,
			s.config.MaxParallel, now, s.validator)
		if err != nil {
			return result, fmt.Errorf("build Researchflow schedule plan: %w", err)
		}
		persisted, err := PersistSchedulePlan(ctx, s.store, plan)
		if errors.Is(err, ErrSchedulePlanConflict) {
			result.ReplanCount++
			continue
		}
		if err != nil {
			return result, err
		}

		result.Plan = plan
		result.Persisted = persisted
		attemptRefs := make([]string, len(plan.Dispatch))
		for index, stage := range plan.Dispatch {
			attemptRef, idErr := s.ids.NewRef(ctx, "attempt")
			if idErr != nil {
				return result, fmt.Errorf("generate Researchflow Attempt reference for Stage %q: %w", stage.Ref, idErr)
			}
			if strings.TrimSpace(attemptRef) == "" || attemptRef != strings.TrimSpace(attemptRef) {
				return result, fmt.Errorf("Researchflow ID generator returned an invalid Attempt reference for Stage %q", stage.Ref)
			}
			attemptRefs[index] = attemptRef
		}

		// 单次 Run 的 Dispatch 集合已受 MaxParallel 限制；并发提交给固定 Worker Pool，
		// 让同一 Run 的独立 Stage 可以并行执行，而不增加第二套持久化队列。
		result.Executions = make([]ScheduledStageExecution, len(plan.Dispatch))
		dispatchErrors := make([]error, len(plan.Dispatch))
		var dispatchWait sync.WaitGroup
		for index, stage := range plan.Dispatch {
			index, stage, attemptRef := index, stage, attemptRefs[index]
			dispatchWait.Add(1)
			go func() {
				defer dispatchWait.Done()
				execution, dispatchErr := s.dispatcher.Execute(ctx, runRef, stage.Ref, attemptRef)
				result.Executions[index] = ScheduledStageExecution{
					StageRef: stage.Ref, AttemptRef: attemptRef, Execution: execution,
				}
				dispatchErrors[index] = dispatchErr
			}()
		}
		dispatchWait.Wait()
		for index, dispatchErr := range dispatchErrors {
			if dispatchErr != nil {
				return result, fmt.Errorf("dispatch Researchflow Stage %q: %w", plan.Dispatch[index].Ref, dispatchErr)
			}
		}
		return result, nil
	}
	return result, fmt.Errorf("%w: run=%s max_replans=%d", ErrScheduleReplanExhausted, runRef, s.config.MaxReplans)
}
