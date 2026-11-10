package application

import (
	"context"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var ErrUnknownReconciliationPersistence = errors.New("notification: unknown reconciliation persistence failed")

// persistUnknownReconciliation 只接收包内受控编排生成的已验证证据。
func persistUnknownReconciliation(
	ctx context.Context,
	repository UnknownDeliveryReconciliationRepository,
	delivery domain.DeliveryRecord,
	assessment UnknownReconciliationAssessment,
	at time.Time,
	retryDecision *RetryDecision,
) error {
	if repository == nil || ctx == nil || ctx.Err() != nil || !isRecoveryUTC(at) {
		if ctx != nil && ctx.Err() != nil { return ctx.Err() }
		return ErrUnknownReconciliationPersistence
	}
	if err := delivery.Validate(); err != nil { return err }
	if delivery.Status != domain.DeliveryUnknown ||
		assessment.DeliveryID != delivery.DeliveryID ||
		!validReconciliationRef(assessment.AttemptID) ||
		assessment.FencingToken < 1 ||
		!validReconciliationRef(assessment.EvidenceRef) ||
		at.Before(delivery.UpdatedAt) {
		return ErrUnknownReconciliationNotApplicable
	}

	change := domain.DeliveryTransition{At: at, EvidenceRef: assessment.EvidenceRef, ReasonCode: assessment.ReasonCode}
	switch assessment.Action {
	case UnknownReconciliationRemainUnknown:
		return nil
	case UnknownReconciliationFinalizeSent:
		if !validReconciliationRef(assessment.ProviderMessageRef) || assessment.ErrorCode != "" || retryDecision != nil {
			return ErrUnknownReconciliationNotApplicable
		}
		change.To = domain.DeliverySent
		change.ProviderMessageRef = assessment.ProviderMessageRef
	case UnknownReconciliationFinalizeFailed:
		if !validReconciliationCode(assessment.ErrorCode) || assessment.ProviderMessageRef != "" || retryDecision != nil {
			return ErrUnknownReconciliationNotApplicable
		}
		change.To = domain.DeliveryFailed
		change.ErrorCode = assessment.ErrorCode
	case UnknownReconciliationApplyRetryPolicy:
		if retryDecision == nil || retryDecision.DeliveryID != delivery.DeliveryID ||
			retryDecision.DeliveryVersion != delivery.Version ||
			retryDecision.AttemptID != assessment.AttemptID ||
			retryDecision.FencingToken != assessment.FencingToken ||
			!retryDecision.EvaluatedAt.Equal(at) ||
			retryDecision.EvidenceRef != assessment.EvidenceRef ||
			!isRecoveryUTC(retryDecision.EvidenceObservedAt) ||
			retryDecision.EvidenceObservedAt.After(at) ||
			retryDecision.EvidenceRef == "" ||
			assessment.ErrorCode == "" || assessment.ProviderMessageRef != "" {
			return ErrRetryDecisionConflict
		}
		change.ErrorCode = assessment.ErrorCode
		if retryDecision.Retry {
			if retryDecision.ReasonCode != "" || retryDecision.NextAttemptAt == nil ||
				!isRecoveryUTC(*retryDecision.NextAttemptAt) ||
				!retryDecision.NextAttemptAt.After(at) ||
				!retryDecision.NextAttemptAt.Before(delivery.DeadlineAt) {
				return ErrRetryDecisionConflict
			}
			change.To = domain.DeliveryRetryWait
			change.ReasonCode = "retry_scheduled"
			change.NextAttemptAt = cloneRetryTime(retryDecision.NextAttemptAt)
		} else {
			if retryDecision.NextAttemptAt != nil ||
				(retryDecision.ReasonCode != RetryReasonBudgetExhausted && retryDecision.ReasonCode != RetryReasonDeadlineExceeded) {
				return ErrRetryDecisionConflict
			}
			change.To = domain.DeliveryFailed
			change.ReasonCode = retryDecision.ReasonCode
		}
	default:
		return ErrUnknownReconciliationNotApplicable
	}
	next, event, err := delivery.Transition(change)
	if err != nil { return err }
	if err := repository.ReconcileUnknown(ctx, delivery.Version, next, event); err != nil { return err }
	return nil
}

// reconcileUnknownDelivery 是包内状态收敛函数；生产入口必须由一致性快照服务调用。
func reconcileUnknownDelivery(
	ctx context.Context,
	repository UnknownDeliveryReconciliationRepository,
	policy RetryPolicy,
	delivery domain.DeliveryRecord,
	channel domain.ChannelConfigSnapshot,
	attempts []domain.DeliveryAttempt,
	verified verifiedUnknownDeliveryEvidence,
	now time.Time,
	randomUnit float64,
) (UnknownReconciliationAssessment, *RetryDecision, error) {
	if !verified.matches(delivery, channel, attempts) {
		return UnknownReconciliationAssessment{}, nil, ErrUnknownReconciliationNotApplicable
	}
	evidence := verified.evidence
	assessment, err := AssessUnknownDelivery(delivery, channel, attempts, evidence, now)
	if err != nil { return UnknownReconciliationAssessment{}, nil, err }
	var decision *RetryDecision
	if assessment.Action == UnknownReconciliationApplyRetryPolicy {
		computed, err := policy.ScheduleRetryAfterUnknownLookup(delivery, attempts, now, randomUnit, evidence.EvidenceRef, evidence.ObservedAt)
		if err != nil { return assessment, nil, err }
		decision = &computed
	}
	if err := persistUnknownReconciliation(ctx, repository, delivery, assessment, now, decision); err != nil {
		return assessment, decision, err
	}
	return assessment, decision, nil
}
