package state

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

var (
	ErrInvalidTransition    = errors.New("invalid researchflow state transition")
	ErrDeadlineExceeded     = errors.New("run deadline exceeded")
	ErrRetryBudgetExhausted = errors.New("stage retry budget exhausted")
	ErrNotRetryable         = errors.New("attempt reason code is not retryable")
	ErrAlreadyTerminal      = errors.New("run is already terminal")
)

// TransitionRun exposes only non-terminal Run transitions. Terminal transitions must
// go through aggregation/finalization so the ResultSummary invariant cannot be bypassed.
func TransitionRun(run domain.Run, next domain.RunStatus, now time.Time) (domain.Run, error) {
	if run.Status.Terminal() {
		return run, ErrAlreadyTerminal
	}
	if next.Terminal() {
		return run, fmt.Errorf("%w: terminal Run transitions require a finalizer", ErrInvalidTransition)
	}
	return transitionRun(run, next, now)
}

func transitionRun(run domain.Run, next domain.RunStatus, now time.Time) (domain.Run, error) {
	if run.Status.Terminal() {
		return run, ErrAlreadyTerminal
	}
	allowed := false
	switch run.Status {
	case domain.RunCreated:
		allowed = next == domain.RunQueued || next == domain.RunCancelled
	case domain.RunQueued:
		allowed = next == domain.RunRunning || next == domain.RunCancelling || next == domain.RunCancelled
	case domain.RunRunning:
		allowed = next == domain.RunCancelling || next == domain.RunSucceeded || next == domain.RunPartial || next == domain.RunFailed
	case domain.RunCancelling:
		allowed = next == domain.RunCancelled || next == domain.RunPartial || next == domain.RunFailed
	}
	if !allowed {
		return run, fmt.Errorf("%w: run %s -> %s", ErrInvalidTransition, run.Status, next)
	}
	run.Status = next
	run.Version++
	if next == domain.RunRunning && run.StartedAt == nil {
		v := now.UTC()
		run.StartedAt = &v
	}
	if next.Terminal() {
		v := now.UTC()
		run.FinishedAt = &v
	}
	return run, nil
}

func TransitionStage(stage domain.Stage, next domain.StageStatus, now time.Time) (domain.Stage, error) {
	allowed := false
	switch stage.Status {
	case domain.StagePending:
		allowed = next == domain.StageReady || next == domain.StageBlocked || next == domain.StageCancelled || next == domain.StageTimedOut
	case domain.StageReady:
		allowed = next == domain.StageRunning || next == domain.StageBlocked || next == domain.StageCancelled || next == domain.StageTimedOut
	case domain.StageRunning:
		allowed = next == domain.StageRetryWait || next == domain.StageSucceeded || next == domain.StageDegraded || next == domain.StageFailed || next == domain.StageTimedOut || next == domain.StageCancelled
	case domain.StageRetryWait:
		allowed = next == domain.StageReady || next == domain.StageFailed || next == domain.StageTimedOut || next == domain.StageCancelled
	}
	if !allowed {
		return stage, fmt.Errorf("%w: stage %s -> %s", ErrInvalidTransition, stage.Status, next)
	}
	stage.Status = next
	stage.Version++
	if next == domain.StageRunning && stage.StartedAt == nil {
		v := now.UTC()
		stage.StartedAt = &v
	}
	if next.Terminal() {
		v := now.UTC()
		stage.FinishedAt = &v
	}
	return stage, nil
}

func TransitionAttempt(attempt domain.Attempt, next domain.AttemptStatus, now time.Time) (domain.Attempt, error) {
	if attempt.Status != domain.AttemptRunning || !next.Terminal() {
		return attempt, fmt.Errorf("%w: attempt %s -> %s", ErrInvalidTransition, attempt.Status, next)
	}
	attempt.Status = next
	v := now.UTC()
	attempt.FinishedAt = &v
	return attempt, nil
}

func CancelRun(run domain.Run, stages []domain.Stage, policies map[string]template.PropagationPolicy, hardRiskStageCodes map[string]bool, now time.Time) (domain.Run, []domain.Stage, error) {
	if run.Status.Terminal() {
		return run, stages, ErrAlreadyTerminal
	}
	updatedStages := append([]domain.Stage(nil), stages...)
	if run.Status == domain.RunCreated || run.Status == domain.RunQueued {
		// Validate the whole input before changing any Stage; callers may reuse the slice on error.
		for _, stage := range updatedStages {
			if stage.Status == domain.StageRunning {
				return run, stages, errors.New("queued or created run cannot contain a running stage")
			}
		}
		updatedRun, err := transitionRun(run, domain.RunCancelled, now)
		if err != nil {
			return run, stages, err
		}
		updatedRun.TerminationReason = "user_cancelled"
		for i := range updatedStages {
			if !updatedStages[i].Status.Terminal() {
				updatedStages[i], err = TransitionStage(updatedStages[i], domain.StageCancelled, now)
				if err != nil {
					return run, stages, err
				}
			}
		}
		summary, summaryErr := buildTerminalSummary(updatedStages, run.ExcludedStages, policies, hardRiskStageCodes)
		if summaryErr != nil {
			return run, stages, summaryErr
		}
		updatedRun.ResultSummary = ptrSummary(summary)
		return updatedRun, updatedStages, nil
	}
	if run.Status == domain.RunRunning {
		updatedRun, err := TransitionRun(run, domain.RunCancelling, now)
		if err != nil {
			return run, stages, err
		}
		updatedRun.TerminationReason = "user_cancelled"
		for i := range updatedStages {
			switch updatedStages[i].Status {
			case domain.StagePending, domain.StageReady, domain.StageRetryWait:
				updatedStages[i], err = TransitionStage(updatedStages[i], domain.StageCancelled, now)
				if err != nil {
					return run, stages, err
				}
			}
		}
		return updatedRun, updatedStages, nil
	}
	return run, stages, nil
}

// ApplyDeadline moves unfinished work into cancellation at the Run deadline. If every Stage is already terminal,
// it leaves the Run running so the caller can use AggregateRun rather than deadline finalization.
func ApplyDeadline(run domain.Run, stages []domain.Stage, dbNow time.Time) (domain.Run, []domain.Stage, error) {
	if run.Status.Terminal() || dbNow.UTC().Before(run.DeadlineAt.UTC()) {
		return run, stages, nil
	}
	if run.Status != domain.RunQueued && run.Status != domain.RunRunning {
		return run, stages, nil
	}
	// A deadline worker may run after all work has completed but before the
	// normal aggregation transaction. Preserve the running Run so its success
	// can be finalized instead of forcing an invalid cancelling -> succeeded path.
	if run.Status == domain.RunRunning && len(stages) > 0 {
		allStagesTerminal := true
		for _, stage := range stages {
			if !stage.Status.Terminal() {
				allStagesTerminal = false
				break
			}
		}
		if allStagesTerminal {
			return run, stages, nil
		}
	}
	updatedStages := append([]domain.Stage(nil), stages...)
	for i := range updatedStages {
		updatedStages[i].ReasonCodes = append([]string(nil), stages[i].ReasonCodes...)
	}
	updatedRun, err := TransitionRun(run, domain.RunCancelling, dbNow)
	if err != nil {
		return run, stages, err
	}
	updatedRun.TerminationReason = "run_deadline_exceeded"
	for i := range updatedStages {
		switch updatedStages[i].Status {
		case domain.StagePending, domain.StageReady, domain.StageRetryWait:
			updatedStages[i], err = TransitionStage(updatedStages[i], domain.StageTimedOut, dbNow)
			if err != nil {
				return run, stages, err
			}
			updatedStages[i].ReasonCodes = append(updatedStages[i].ReasonCodes, "run_deadline_exceeded")
		}
	}
	return updatedRun, updatedStages, nil
}

// ScheduleRetry finalizes the current Attempt and moves its Stage to retry_wait.
// The caller persists both changes atomically and supplies jitter from an injected random source.
func ScheduleRetry(stage domain.Stage, attempt domain.Attempt, policy template.RetryPolicy, reasonCode string, attemptResult domain.AttemptStatus, now, runDeadline time.Time, jitter float64) (domain.Stage, domain.Attempt, error) {
	if attempt.Status != domain.AttemptRunning || stage.Status != domain.StageRunning || attempt.StageRef != stage.Ref || attempt.Number != stage.AttemptNo {
		return stage, attempt, ErrInvalidTransition
	}
	maxDurationMS := int64(^uint64(0)>>1) / int64(time.Millisecond)
	if policy.Backoff.Algorithm != "exponential_full_jitter_v1" || policy.Backoff.BaseDelayMS <= 0 || policy.Backoff.MaxDelayMS < policy.Backoff.BaseDelayMS || policy.Backoff.MaxDelayMS > maxDurationMS {
		return stage, attempt, errors.New("retry backoff policy is invalid or exceeds time.Duration range")
	}
	if now.UTC().Compare(runDeadline.UTC()) >= 0 {
		return stage, attempt, ErrDeadlineExceeded
	}
	if attempt.Number >= policy.MaxAttempts {
		return stage, attempt, ErrRetryBudgetExhausted
	}
	if !contains(policy.RetryableReasonCodes, reasonCode) {
		return stage, attempt, fmt.Errorf("%w: %s", ErrNotRetryable, reasonCode)
	}
	if attemptResult != domain.AttemptFailed && attemptResult != domain.AttemptTimedOut {
		return stage, attempt, errors.New("only failed or timed_out attempts may retry")
	}
	if jitter < 0 || jitter > 1 || math.IsNaN(jitter) {
		return stage, attempt, errors.New("jitter must be in [0,1]")
	}
	terminal, err := TransitionAttempt(attempt, attemptResult, now)
	if err != nil {
		return stage, attempt, err
	}
	capMS := float64(policy.Backoff.BaseDelayMS)
	for i := 1; i < attempt.Number && capMS < float64(policy.Backoff.MaxDelayMS); i++ {
		capMS *= 2
	}
	if capMS > float64(policy.Backoff.MaxDelayMS) {
		capMS = float64(policy.Backoff.MaxDelayMS)
	}
	delayNanos := jitter * capMS * float64(time.Millisecond)
	if delayNanos >= float64(int64(^uint64(0)>>1)) {
		return stage, attempt, errors.New("retry delay exceeds time.Duration range")
	}
	retryAt := now.UTC().Add(time.Duration(delayNanos))
	if !retryAt.Before(runDeadline.UTC()) {
		return stage, attempt, ErrDeadlineExceeded
	}
	updated, err := TransitionStage(stage, domain.StageRetryWait, now)
	if err != nil {
		return stage, attempt, err
	}
	updated.RetryAt = &retryAt
	updated.ReasonCodes = append([]string(nil), stage.ReasonCodes...)
	updated.ReasonCodes = append(updated.ReasonCodes, reasonCode)
	return updated, terminal, nil
}

// ClaimRetryStage atomically models Scheduler retry promotion and Worker claim.
// The persisted operation must commit retry_wait -> ready -> running together with
// insertion of the next Attempt; an intermediate ready Stage must not be committed.
func ClaimRetryStage(run domain.Run, stage domain.Stage, policy template.RetryPolicy, now time.Time, attemptRef string, fencingToken uint64) (domain.Stage, domain.Attempt, error) {
	if run.Status != domain.RunRunning || run.TerminationReason != "" {
		return stage, domain.Attempt{}, fmt.Errorf("%w: run does not permit retry", ErrInvalidTransition)
	}
	if !now.UTC().Before(run.DeadlineAt.UTC()) {
		return stage, domain.Attempt{}, ErrDeadlineExceeded
	}
	if stage.Status != domain.StageRetryWait || stage.RetryAt == nil || now.UTC().Before(*stage.RetryAt) {
		return stage, domain.Attempt{}, fmt.Errorf("%w: stage is not ready for retry", ErrInvalidTransition)
	}
	if policy.MaxAttempts < 1 || stage.AttemptNo < 1 || stage.AttemptNo >= policy.MaxAttempts {
		return stage, domain.Attempt{}, ErrRetryBudgetExhausted
	}
	ready, err := TransitionStage(stage, domain.StageReady, now)
	if err != nil {
		return stage, domain.Attempt{}, err
	}
	ready.RetryAt = nil
	claimed, attempt, err := ClaimReadyStage(run, ready, now, attemptRef, fencingToken)
	if err != nil {
		return stage, domain.Attempt{}, err
	}
	// ready 只是同一 RetryStage 事务中的逻辑中间态，不会单独持久化。
	// Stage.version 对应一次原子 Repository 更新，因此最终只递增一次。
	claimed.Version = stage.Version + 1
	return claimed, attempt, nil
}

// ClaimReadyStage models a Worker claim after scheduler promotion. The caller must
// persist the Stage and Attempt atomically under the Run.version/CAS gate and ensure
// attemptRef uniqueness; this pure function does not provide cross-process exclusion.
func ClaimReadyStage(run domain.Run, stage domain.Stage, now time.Time, attemptRef string, fencingToken uint64) (domain.Stage, domain.Attempt, error) {
	if run.Status != domain.RunRunning || run.TerminationReason != "" {
		return stage, domain.Attempt{}, fmt.Errorf("%w: run does not permit worker claim", ErrInvalidTransition)
	}
	if !now.UTC().Before(run.DeadlineAt.UTC()) {
		return stage, domain.Attempt{}, ErrDeadlineExceeded
	}
	if stage.Status != domain.StageReady || strings.TrimSpace(stage.Ref) == "" || stage.AttemptNo < 0 || strings.TrimSpace(attemptRef) == "" || fencingToken == 0 {
		return stage, domain.Attempt{}, fmt.Errorf("%w: stage is not claimable", ErrInvalidTransition)
	}
	updated, err := TransitionStage(stage, domain.StageRunning, now)
	if err != nil {
		return stage, domain.Attempt{}, err
	}
	updated.AttemptNo++
	attempt := domain.Attempt{
		Ref: attemptRef, StageRef: stage.Ref, Number: updated.AttemptNo,
		Status: domain.AttemptRunning, StartedAt: now.UTC(), FencingToken: fencingToken,
	}
	return updated, attempt, nil
}

func AggregateRun(run domain.Run, stages []domain.Stage, attempts []domain.Attempt, policies map[string]template.PropagationPolicy, hardRiskStageCodes map[string]bool, requiresHardRiskGate bool, now time.Time) (domain.Run, error) {
	if err := validateTerminalStages(stages); err != nil {
		return run, fmt.Errorf("cannot aggregate run: %w", err)
	}
	if err := validateAttemptsTerminal(attempts); err != nil {
		return run, fmt.Errorf("cannot aggregate run: %w", err)
	}
	summary := domain.ResultSummary{StageOutcomes: map[string]domain.StageStatus{}, RiskGateDecisions: map[string]string{}, ExcludedStages: cloneExcludedStages(run.ExcludedStages), MissingStageCodes: []string{}, DegradedStageCodes: []string{}, BlockedStageCodes: []string{}, ReasonCodes: []string{}, QualityStatus: "valid", ActionabilityStatus: "not_evaluated"}
	fail, partial, completed := false, false, 0
	gateCount, gatePassed, gateExecutionFailed := 0, true, false
	for _, s := range stages {
		summary.StageOutcomes[s.Code] = s.Status
		p, ok := policies[s.Code]
		if !ok {
			return run, fmt.Errorf("missing propagation policy for stage %s", s.Code)
		}
		switch s.Status {
		case domain.StageSucceeded:
			completed++
		case domain.StageDegraded:
			completed++
			summary.DegradedStageCodes = append(summary.DegradedStageCodes, s.Code)
			summary.QualityStatus = "degraded"
			if p.OnDegraded == domain.PropagateFail {
				fail = true
			} else if p.OnDegraded == domain.PropagatePartial {
				partial = true
			}
		case domain.StageSkipped:
			summary.MissingStageCodes = append(summary.MissingStageCodes, s.Code)
			if s.Required || p.OnSkipped == domain.PropagateFail {
				fail = true
			} else if p.OnSkipped == domain.PropagatePartial {
				partial = true
			} else if p.OnSkipped == domain.PropagateNoImpact {
				completed++
			}
		case domain.StageFailed:
			summary.MissingStageCodes = append(summary.MissingStageCodes, s.Code)
			if p.OnFailed == domain.PropagateFail {
				fail = true
			} else {
				partial = true
			}
		case domain.StageTimedOut:
			summary.MissingStageCodes = append(summary.MissingStageCodes, s.Code)
			if p.OnTimedOut == domain.PropagateFail {
				fail = true
			} else {
				partial = true
			}
		case domain.StageBlocked:
			summary.MissingStageCodes = append(summary.MissingStageCodes, s.Code)
			summary.BlockedStageCodes = append(summary.BlockedStageCodes, s.Code)
			if p.OnBlocked == domain.PropagateFail {
				fail = true
			} else {
				partial = true
			}
		case domain.StageCancelled:
			return run, errors.New("cancelled stages require cancellation finalization")
		}
		if hardRiskStageCodes[s.Code] {
			gateCount++
			summary.RiskGateDecisions[s.Code] = s.RiskDecision
			if s.Status != domain.StageSucceeded {
				gateExecutionFailed = true
			} else {
				switch s.RiskDecision {
				case "pass":
				case "reject", "unknown":
					gatePassed = false
				default:
					return run, fmt.Errorf("hard risk stage %s returned invalid risk decision %q", s.Code, s.RiskDecision)
				}
			}
		}
		for _, reason := range s.ReasonCodes {
			if reason != "" {
				summary.ReasonCodes = append(summary.ReasonCodes, reason)
			}
		}
	}
	sort.Strings(summary.MissingStageCodes)
	sort.Strings(summary.DegradedStageCodes)
	sort.Strings(summary.BlockedStageCodes)
	summary.ReasonCodes = uniqueSorted(summary.ReasonCodes)
	summary.Coverage = float64(completed) / float64(len(stages))
	if requiresHardRiskGate && gateCount == 0 {
		fail = true
	}
	finalStatus := domain.RunSucceeded
	if fail {
		finalStatus = domain.RunFailed
	} else if partial {
		finalStatus = domain.RunPartial
	}
	if requiresHardRiskGate {
		// A failed Run or failed gate execution is not an evaluated business decision.
		if !gateExecutionFailed && gateCount > 0 && finalStatus != domain.RunFailed {
			summary.ActionabilityStatus = "blocked"
			if gatePassed && finalStatus == domain.RunSucceeded {
				summary.ActionabilityStatus = "actionable"
			}
		}
	}
	if run.Status != domain.RunRunning && run.Status != domain.RunCancelling {
		return run, fmt.Errorf("%w: run cannot be aggregated from %s", ErrInvalidTransition, run.Status)
	}
	updated, err := transitionRun(run, finalStatus, now)
	if err != nil {
		return run, err
	}
	updated.ResultSummary = &summary
	return updated, nil
}

func FinalizeCancellation(run domain.Run, stages []domain.Stage, attempts []domain.Attempt, policies map[string]template.PropagationPolicy, hardRiskStageCodes map[string]bool, now time.Time) (domain.Run, error) {
	if run.Status != domain.RunCancelling || run.TerminationReason != "user_cancelled" {
		return run, fmt.Errorf("%w: run is not a user cancellation", ErrInvalidTransition)
	}
	if err := validateTerminalStages(stages); err != nil {
		return run, fmt.Errorf("cannot finalize cancellation: %w", err)
	}
	if err := validateAttemptsTerminal(attempts); err != nil {
		return run, fmt.Errorf("cannot finalize cancellation: %w", err)
	}
	summary, err := buildTerminalSummary(stages, run.ExcludedStages, policies, hardRiskStageCodes)
	if err != nil {
		return run, fmt.Errorf("cannot build cancellation summary: %w", err)
	}
	updated, err := transitionRun(run, domain.RunCancelled, now)
	if err != nil {
		return run, err
	}
	updated.ResultSummary = &summary
	return updated, nil
}

// validateTerminalStages ensures a complete, uniquely identified terminal Stage set.
func validateTerminalStages(stages []domain.Stage) error {
	if len(stages) == 0 {
		return errors.New("run has no stages")
	}
	seen := make(map[string]struct{}, len(stages))
	for _, stage := range stages {
		if strings.TrimSpace(stage.Code) == "" {
			return errors.New("stage code is required for terminal aggregation")
		}
		if _, exists := seen[stage.Code]; exists {
			return fmt.Errorf("duplicate stage code %q in terminal aggregation", stage.Code)
		}
		seen[stage.Code] = struct{}{}
		if !stage.Status.Terminal() {
			return fmt.Errorf("stage %s is not in a terminal state: %q", stage.Code, stage.Status)
		}
	}
	return nil
}

// validateAttemptsTerminal fails closed for running and unknown Attempt states.
func validateAttemptsTerminal(attempts []domain.Attempt) error {
	for _, attempt := range attempts {
		if !attempt.Status.Terminal() {
			return fmt.Errorf("attempt %q is not in a terminal state: %q", attempt.Ref, attempt.Status)
		}
	}
	return nil
}

func ptrSummary(summary domain.ResultSummary) *domain.ResultSummary { return &summary }

func buildTerminalSummary(stages []domain.Stage, excludedStages []domain.ExcludedStage, policies map[string]template.PropagationPolicy, hardRiskStageCodes map[string]bool) (domain.ResultSummary, error) {
	summary := domain.ResultSummary{
		StageOutcomes: map[string]domain.StageStatus{}, RiskGateDecisions: map[string]string{},
		ExcludedStages: cloneExcludedStages(excludedStages), MissingStageCodes: []string{},
		DegradedStageCodes: []string{}, BlockedStageCodes: []string{}, ReasonCodes: []string{},
		QualityStatus: "valid", ActionabilityStatus: "not_evaluated",
	}
	completed := 0
	for _, stage := range stages {
		summary.StageOutcomes[stage.Code] = stage.Status
		if hardRiskStageCodes[stage.Code] {
			if stage.RiskDecision != "" {
				switch stage.RiskDecision {
				case "pass", "reject", "unknown":
					summary.RiskGateDecisions[stage.Code] = stage.RiskDecision
				default:
					return domain.ResultSummary{}, fmt.Errorf("hard risk stage %s returned invalid risk decision %q", stage.Code, stage.RiskDecision)
				}
			} else if stage.Status == domain.StageSucceeded {
				return domain.ResultSummary{}, fmt.Errorf("hard risk stage %s succeeded without a risk decision", stage.Code)
			}
		}
		switch stage.Status {
		case domain.StageSucceeded:
			completed++
		case domain.StageDegraded:
			completed++
			summary.DegradedStageCodes = append(summary.DegradedStageCodes, stage.Code)
			summary.QualityStatus = "degraded"
		case domain.StageSkipped:
			summary.MissingStageCodes = append(summary.MissingStageCodes, stage.Code)
			policy, ok := policies[stage.Code]
			if !ok {
				return domain.ResultSummary{}, fmt.Errorf("missing propagation policy for skipped stage %s", stage.Code)
			}
			switch policy.OnSkipped {
			case domain.PropagateNoImpact:
				if stage.Required {
					return domain.ResultSummary{}, fmt.Errorf("required stage %s cannot use skipped=no_impact", stage.Code)
				}
				completed++
			case domain.PropagatePartial, domain.PropagateFail:
			default:
				return domain.ResultSummary{}, fmt.Errorf("invalid on_skipped policy for stage %s: %q", stage.Code, policy.OnSkipped)
			}
		case domain.StageFailed, domain.StageTimedOut, domain.StageCancelled:
			summary.MissingStageCodes = append(summary.MissingStageCodes, stage.Code)
		case domain.StageBlocked:
			summary.MissingStageCodes = append(summary.MissingStageCodes, stage.Code)
			summary.BlockedStageCodes = append(summary.BlockedStageCodes, stage.Code)
		}
		for _, reason := range stage.ReasonCodes {
			if reason != "" {
				summary.ReasonCodes = append(summary.ReasonCodes, reason)
			}
		}
	}
	sort.Strings(summary.MissingStageCodes)
	sort.Strings(summary.DegradedStageCodes)
	sort.Strings(summary.BlockedStageCodes)
	summary.ReasonCodes = uniqueSorted(summary.ReasonCodes)
	if len(stages) > 0 {
		summary.Coverage = float64(completed) / float64(len(stages))
	}
	return summary, nil
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func uniqueSorted(values []string) []string {
	sort.Strings(values)
	if len(values) < 2 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}

// FinalizeDeadline aggregates timed-out and completed stages after all in-flight attempts
// have stopped or been fenced. The template propagation matrix determines partial vs failed.
func FinalizeDeadline(run domain.Run, stages []domain.Stage, attempts []domain.Attempt, policies map[string]template.PropagationPolicy, hardRiskStageCodes map[string]bool, requiresHardRiskGate bool, now time.Time) (domain.Run, error) {
	if run.Status != domain.RunCancelling || run.TerminationReason != "run_deadline_exceeded" {
		return run, fmt.Errorf("%w: run is not cancelling due to deadline", ErrInvalidTransition)
	}
	return AggregateRun(run, stages, attempts, policies, hardRiskStageCodes, requiresHardRiskGate, now)
}

func cloneExcludedStages(values []domain.ExcludedStage) []domain.ExcludedStage {
	if values == nil {
		return []domain.ExcludedStage{}
	}
	return append([]domain.ExcludedStage(nil), values...)
}
