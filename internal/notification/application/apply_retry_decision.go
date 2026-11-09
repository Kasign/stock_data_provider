package application

import (
	"context"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var ErrRetryDecisionConflict = errors.New("notification: retry decision conflicts with delivery state")

// ApplyRetryDecision persists a RetryPolicy result through the current Delivery lease.
// The Repository must atomically CAS the snapshot, append its state event, and release
// the lease when the target status is no longer sending.
func ApplyRetryDecision(
	ctx context.Context,
	repository DeliveryRepository,
	claim DeliveryClaim,
	attempt domain.DeliveryAttempt,
	decision RetryDecision,
	now time.Time,
) error {
	if repository == nil || !isRecoveryUTC(now) {
		return ErrRetryDecisionConflict
	}
	delivery := claim.Delivery
	if err := delivery.Validate(); err != nil {
		return err
	}
	if err := attempt.Validate(); err != nil {
		return err
	}
	if delivery.Status != domain.DeliverySending ||
		decision.DeliveryID != delivery.DeliveryID || decision.DeliveryVersion != delivery.Version ||
		decision.AttemptID != attempt.AttemptID || decision.FencingToken != attempt.FencingToken ||
		!decision.EvaluatedAt.Equal(now) ||
		claim.Lease.FencingToken < 1 ||
		!isRecoveryUTC(claim.Lease.ExpiresAt) ||
		!now.Before(claim.Lease.ExpiresAt) ||
		claim.Lease.WorkerID != attempt.WorkerID ||
		claim.Lease.FencingToken != attempt.FencingToken ||
		!claim.Lease.ExpiresAt.Equal(attempt.LeaseExpiresAt) ||
		attempt.DeliveryID != delivery.DeliveryID ||
		attempt.AttemptNo != delivery.AttemptCount ||
		attempt.FinishedAt == nil || attempt.Outcome == nil ||
		*attempt.Outcome != domain.DeliveryAttemptRejected ||
		!attempt.Retryable || now.Before(*attempt.FinishedAt) {
		return ErrRetryDecisionConflict
	}

	change := domain.DeliveryTransition{At: now}
	if attempt.ErrorCode != nil {
		change.ErrorCode = *attempt.ErrorCode
	}
	if decision.Retry {
		if decision.ReasonCode != "" || decision.NextAttemptAt == nil {
			return ErrRetryDecisionConflict
		}
		change.To = domain.DeliveryRetryWait
		change.ReasonCode = "retry_scheduled"
		change.NextAttemptAt = cloneRetryTime(decision.NextAttemptAt)
	} else {
		if decision.NextAttemptAt != nil ||
			(decision.ReasonCode != RetryReasonBudgetExhausted &&
				decision.ReasonCode != RetryReasonDeadlineExceeded) {
			return ErrRetryDecisionConflict
		}
		change.To = domain.DeliveryFailed
		change.ReasonCode = decision.ReasonCode
	}

	next, event, err := delivery.Transition(change)
	if err != nil {
		return err
	}
	return repository.UpdateWithLease(
		ctx,
		delivery.Version,
		claim.Lease.FencingToken,
		next,
		event,
		now,
	)
}
