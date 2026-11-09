package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

// StageExecutionRequest is the immutable execution envelope assembled by the
// Researchflow Worker. Provider implementations must treat its references,
// timestamps and snapshots as authoritative and must not mutate them.
type StageExecutionRequest struct {
	Run               domain.Run
	Stage             domain.Stage
	Attempt           domain.Attempt
	Parameters        json.RawMessage
	InputSnapshotRefs []string
	SystemAsOf        time.Time
	DeadlineAt        time.Time
	CorrelationID     string
}

// StageExecutionResult is a provider outcome before it is committed by the
// Worker. Providers return opaque references; they never write Researchflow
// persistence or choose Run/Stage/Attempt identities.
type StageExecutionResult struct {
	AttemptStatus     domain.AttemptStatus
	StageStatus       domain.StageStatus
	OutputRef         *string
	QualitySummaryRef *string
	DiagnosticRefs    []string
	ReasonCodes       []string
	QualityStatus     string
	RiskDecision      string
	FaultRef          *string
	RetryAt           *time.Time
}

// StageExecutor is the module-owned execution port. Concrete Data/News/Quant/
// Research/LLM adapters are injected by the Server composition root.
type StageExecutor interface {
	ExecuteStage(context.Context, StageExecutionRequest) (StageExecutionResult, error)
}

var ErrInvalidStageExecutionResult = errors.New("invalid Researchflow stage execution result")

// ValidateStageExecutionResult rejects outcomes that cannot be committed by
// CommitAttemptResult. Retry policy and reason-code allowlisting are evaluated
// by the Application state transition before persistence.
func ValidateStageExecutionResult(result StageExecutionResult) error {
	switch result.StageStatus {
	case domain.StageSucceeded:
		if result.AttemptStatus != domain.AttemptSucceeded || !nonEmptyRef(result.OutputRef) || result.RetryAt != nil {
			return fmt.Errorf("%w: succeeded requires a succeeded Attempt, output_ref and no retry_at", ErrInvalidStageExecutionResult)
		}
	case domain.StageDegraded:
		if result.AttemptStatus != domain.AttemptDegraded || !nonEmptyRef(result.OutputRef) || result.RetryAt != nil {
			return fmt.Errorf("%w: degraded requires a degraded Attempt, output_ref and no retry_at", ErrInvalidStageExecutionResult)
		}
	case domain.StageRetryWait:
		if (result.AttemptStatus != domain.AttemptFailed && result.AttemptStatus != domain.AttemptTimedOut) ||
			result.RetryAt != nil || result.OutputRef != nil {
			return fmt.Errorf("%w: retry_wait requires a failed/timed_out Attempt, no provider retry_at and no output_ref", ErrInvalidStageExecutionResult)
		}
	case domain.StageFailed:
		if (result.AttemptStatus != domain.AttemptFailed && result.AttemptStatus != domain.AttemptTimedOut) ||
			result.RetryAt != nil {
			return fmt.Errorf("%w: failed requires a failed/timed_out Attempt and no retry_at", ErrInvalidStageExecutionResult)
		}
	case domain.StageTimedOut:
		if result.AttemptStatus != domain.AttemptTimedOut || result.RetryAt != nil {
			return fmt.Errorf("%w: timed_out requires a timed_out Attempt and no retry_at", ErrInvalidStageExecutionResult)
		}
	case domain.StageCancelled:
		if result.AttemptStatus != domain.AttemptCancelled || result.RetryAt != nil {
			return fmt.Errorf("%w: cancelled requires a cancelled Attempt and no retry_at", ErrInvalidStageExecutionResult)
		}
	default:
		return fmt.Errorf("%w: unsupported Stage status %q", ErrInvalidStageExecutionResult, result.StageStatus)
	}
	if result.OutputRef != nil && strings.TrimSpace(*result.OutputRef) == "" {
		return fmt.Errorf("%w: output_ref must be null or non-empty", ErrInvalidStageExecutionResult)
	}
	if result.QualitySummaryRef != nil && strings.TrimSpace(*result.QualitySummaryRef) == "" {
		return fmt.Errorf("%w: quality_summary_ref must be null or non-empty", ErrInvalidStageExecutionResult)
	}
	if result.FaultRef != nil && strings.TrimSpace(*result.FaultRef) == "" {
		return fmt.Errorf("%w: fault_ref must be null or non-empty", ErrInvalidStageExecutionResult)
	}
	if result.StageStatus == domain.StageSucceeded || result.StageStatus == domain.StageDegraded {
		if result.FaultRef != nil {
			return fmt.Errorf("%w: successful outcome cannot carry fault_ref", ErrInvalidStageExecutionResult)
		}
	}
	for _, ref := range result.DiagnosticRefs {
		if strings.TrimSpace(ref) == "" {
			return fmt.Errorf("%w: diagnostic_refs must contain only non-empty references", ErrInvalidStageExecutionResult)
		}
	}
	for _, code := range result.ReasonCodes {
		if strings.TrimSpace(code) == "" {
			return fmt.Errorf("%w: reason_codes must contain only non-empty codes", ErrInvalidStageExecutionResult)
		}
	}
	return nil
}

func nonEmptyRef(ref *string) bool {
	return ref != nil && strings.TrimSpace(*ref) != ""
}
