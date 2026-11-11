package application

import (
	"context"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var ErrDeliveryCompletionUnavailable = errors.New("notification: delivery completion unavailable")

// DeliveryCompletionService closes the application-level loop after a channel
// call. Attempt completion is persisted first; Delivery state is then CAS-updated.
// If the second write fails, lease recovery can derive the outcome from the
// immutable Attempt history without repeating the channel call.
type DeliveryCompletionService struct {
	deliveries DeliveryRepository
	attempts   DeliveryAttemptRepository
	retry      RetryPolicy
}

func NewDeliveryCompletionService(
	deliveries DeliveryRepository,
	attempts DeliveryAttemptRepository,
	retry RetryPolicy,
) (*DeliveryCompletionService, error) {
	if deliveries == nil || attempts == nil {
		return nil, ErrDeliveryCompletionUnavailable
	}
	if err := retry.Validate(); err != nil {
		return nil, err
	}
	return &DeliveryCompletionService{deliveries: deliveries, attempts: attempts, retry: retry}, nil
}

// Complete records the channel result and converges Delivery state. randomUnit
// is used only for retryable rejections and must be sampled by the caller.
func (s *DeliveryCompletionService) Complete(
	ctx context.Context,
	claim DeliveryClaim,
	attempt domain.DeliveryAttempt,
	channel domain.ChannelConfigSnapshot,
	result domain.ChannelSendResult,
	finishedAt time.Time,
	randomUnit float64,
	evidenceRef string,
) error {
	if s == nil || s.deliveries == nil || s.attempts == nil {
		return ErrDeliveryCompletionUnavailable
	}
	if ctx == nil || ctx.Err() != nil {
		if ctx == nil {
			return ErrDeliveryCompletionUnavailable
		}
		return ctx.Err()
	}
	delivery := claim.Delivery
	if err := delivery.Validate(); err != nil {
		return err
	}
	if delivery.Status != domain.DeliverySending ||
		channel.ChannelRef != delivery.ChannelRef ||
		channel.ConfigVersion != delivery.ChannelConfigVersion ||
		claim.Lease.WorkerID != attempt.WorkerID ||
		claim.Lease.FencingToken != attempt.FencingToken ||
		!claim.Lease.ExpiresAt.Equal(attempt.LeaseExpiresAt) ||
		attempt.DeliveryID != delivery.DeliveryID ||
		int64(attempt.AttemptNo) != delivery.AttemptCount+1 ||
		attempt.AttemptNo < 1 ||
		attempt.FinishedAt != nil {
		return ErrRetryDecisionConflict
	}
	if !isRecoveryUTC(finishedAt) || !finishedAt.Before(claim.Lease.ExpiresAt) {
		return ErrRetryDecisionConflict
	}

	completed, err := CompleteAttemptFromChannelResult(
		attempt, channel, result, finishedAt, evidenceRef,
	)
	if err != nil {
		return err
	}
	if err := s.attempts.CompleteWithLease(
		ctx, delivery.Version, claim.Lease.FencingToken, completed,
	); err != nil {
		return err
	}

	// StartWithLease 已在独立事务中推进 attempt_count，但不改变 Delivery.version。
	// Claim 快照因此已过时；完成状态迁移前必须重新读取当前投影，禁止用旧计数覆盖数据库。
	current, found, err := s.deliveries.Get(ctx, delivery.DeliveryID)
	if err != nil {
		return err
	}
	if !found || current.DeliveryID != delivery.DeliveryID || current.Version != delivery.Version ||
		current.Status != domain.DeliverySending ||
		current.AttemptCount != int64(attempt.AttemptNo) ||
		current.MaxAttempts != delivery.MaxAttempts ||
		!current.CreatedAt.Equal(delivery.CreatedAt) ||
		!current.DeadlineAt.Equal(delivery.DeadlineAt) {
		return ErrRetryDecisionConflict
	}
	delivery = current
	claim.Delivery = current

	change := domain.DeliveryTransition{At: finishedAt}
	switch result.Outcome {
	case domain.ChannelOutcomeAccepted:
		change.To = domain.DeliverySent
		change.ProviderMessageRef = result.ProviderMessageRef
	case domain.ChannelOutcomeAmbiguous:
		change.To = domain.DeliveryUnknown
		change.ReasonCode = "channel_outcome_ambiguous"
		change.EvidenceRef = attempt.AttemptID
	case domain.ChannelOutcomeNotSent:
		change.To = domain.DeliverySkipped
		change.ReasonCode = "channel_request_not_sent"
		change.RequestNotSent = true
		change.EvidenceRef = evidenceRef
	case domain.ChannelOutcomeRejected:
		if !completed.Retryable {
			change.To = domain.DeliveryFailed
			change.ReasonCode = "channel_rejected"
			change.ErrorCode = result.ErrorCode
			break
		}
		history, err := s.attempts.ListByDelivery(ctx, delivery.DeliveryID)
		if err != nil {
			return err
		}
		decision, err := s.retry.ScheduleRetry(delivery, history, finishedAt, randomUnit)
		if err != nil {
			return err
		}
		return ApplyRetryDecision(ctx, s.deliveries, claim, completed, decision, finishedAt)
	default:
		return domain.ErrInvalidChannelSendResult
	}

	next, event, err := delivery.Transition(change)
	if err != nil {
		return err
	}
	return s.deliveries.UpdateWithLease(
		ctx, delivery.Version, claim.Lease.FencingToken, next, event, finishedAt,
	)
}
