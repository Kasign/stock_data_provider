package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var ErrLeaseRecoveryServiceConfig = errors.New("notification: lease recovery service is not configured")

type LeaseRecoveryService struct {
	Scanner     ExpiredDeliveryLeaseScanner
	Attempts    DeliveryAttemptRepository
	Repository  DeliveryLeaseRecoveryRepository
	RetryPolicy RetryPolicy
	RandomUnit  func() float64
}

type LeaseRecoveryReport struct {
	Scanned    int
	Recovered  int
	Conflicted int
	Failed     int
}

// RecoverExpiredOnce runs one bounded recovery pass. The caller supplies the
// UTC clock and entropy source so the pass is deterministic in tests and can
// be driven by the eventual Server worker without owning a timer here.
func (s *LeaseRecoveryService) RecoverExpiredOnce(
	ctx context.Context,
	now time.Time,
	limit int,
) (LeaseRecoveryReport, error) {
	var report LeaseRecoveryReport
	if s == nil || s.Scanner == nil || s.Attempts == nil || s.Repository == nil ||
		ctx == nil || !isRecoveryUTC(now) || limit < 1 || limit > 1000 {
		return report, ErrLeaseRecoveryServiceConfig
	}
	if err := s.RetryPolicy.Validate(); err != nil {
		return report, err
	}
	claims, err := s.Scanner.ListExpiredLeases(ctx, limit)
	if err != nil {
		return report, fmt.Errorf("notification: scan expired delivery leases: %w", err)
	}
	if len(claims) > limit {
		return report, fmt.Errorf("%w: scanner exceeded requested limit", ErrLeaseRecoveryHistory)
	}
	report.Scanned = len(claims)
	var failures []error
	for _, claim := range claims {
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			break
		}
		if err := s.recoverClaim(ctx, claim, now); err != nil {
			if errors.Is(err, ErrDeliveryLeaseConflict) || errors.Is(err, ErrDeliveryVersionConflict) ||
				errors.Is(err, ErrLeaseRecoveryNotApplicable) {
				report.Conflicted++
				continue
			}
			report.Failed++
			failures = append(failures, fmt.Errorf("delivery %s: %w", claim.Delivery.DeliveryID, err))
			continue
		}
		report.Recovered++
	}
	return report, errors.Join(failures...)
}

func (s *LeaseRecoveryService) recoverClaim(ctx context.Context, claim DeliveryClaim, now time.Time) error {
	if err := claim.Delivery.Validate(); err != nil {
		return err
	}
	if claim.Delivery.Status != domain.DeliverySending ||
		claim.Lease.WorkerID == "" || claim.Lease.FencingToken < 1 ||
		!isRecoveryUTC(claim.Lease.ExpiresAt) || now.Before(claim.Lease.ExpiresAt) {
		return ErrLeaseRecoveryNotApplicable
	}
	attempts, err := s.Attempts.ListByDelivery(ctx, claim.Delivery.DeliveryID)
	if err != nil {
		return err
	}
	assessment, err := AssessExpiredDeliveryLease(claim.Delivery, claim.Lease, attempts, now)
	if err != nil {
		return err
	}

	change := domain.DeliveryTransition{At: now, ReasonCode: assessment.ReasonCode, EvidenceRef: assessment.EvidenceRef}
	if assessment.AttemptID != "" {
		for i := range attempts {
			if attempts[i].AttemptID == assessment.AttemptID && attempts[i].ErrorCode != nil {
				change.ErrorCode = *attempts[i].ErrorCode
				break
			}
		}
	}
	switch assessment.Action {
	case LeaseRecoverySafeToRetry:
		nextAttemptAt := now.Add(time.Microsecond)
		if !nextAttemptAt.Before(claim.Delivery.DeadlineAt) {
			change.To = domain.DeliveryFailed
			change.ReasonCode = RetryReasonDeadlineExceeded
		} else {
			change.To = domain.DeliveryRetryWait
			change.NextAttemptAt = &nextAttemptAt
		}
	case LeaseRecoveryApplyRetryPolicy:
		if s.RandomUnit == nil {
			return ErrLeaseRecoveryServiceConfig
		}
		unit := s.RandomUnit()
		decision, err := s.RetryPolicy.ScheduleRetry(claim.Delivery, attempts, now, unit)
		if err != nil {
			return err
		}
		if decision.Retry {
			change.To = domain.DeliveryRetryWait
			change.ReasonCode = "retry_scheduled_after_recovery"
			change.NextAttemptAt = cloneRetryTime(decision.NextAttemptAt)
		} else {
			change.To = domain.DeliveryFailed
			change.ReasonCode = decision.ReasonCode
		}
	case LeaseRecoveryFinalizeSent:
		change.To = domain.DeliverySent
		change.ProviderMessageRef = assessment.ProviderMessageRef
	case LeaseRecoveryFinalizeFailed:
		change.To = domain.DeliveryFailed
	case LeaseRecoveryMarkUnknown:
		change.To = domain.DeliveryUnknown
	default:
		return ErrLeaseRecoveryNotApplicable
	}
	next, event, err := claim.Delivery.Transition(change)
	if err != nil {
		return err
	}
	return s.Repository.RecoverExpiredLease(
		ctx, claim.Delivery.Version, claim.Lease.FencingToken, next, event, now,
	)
}
