package application

import (
	"context"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var ErrDeliveryPreflightCompletionUnavailable = errors.New("notification: delivery preflight completion unavailable")

// DeliveryPreflightCompletionService persists a pre-send decision and releases
// the claim lease atomically with the Delivery state transition.
type DeliveryPreflightCompletionService struct {
	deliveries DeliveryRepository
}

func NewDeliveryPreflightCompletionService(deliveries DeliveryRepository) (*DeliveryPreflightCompletionService, error) {
	if deliveries == nil {
		return nil, ErrDeliveryPreflightCompletionUnavailable
	}
	return &DeliveryPreflightCompletionService{deliveries: deliveries}, nil
}

// Apply leaves ready deliveries in sending state. skip and block decisions
// must carry an evidence reference for the preflight operation that prevented
// any channel request from being issued.
func (s *DeliveryPreflightCompletionService) Apply(
	ctx context.Context,
	claim DeliveryClaim,
	result DeliveryPreflightResult,
	at time.Time,
	evidenceRef string,
) error {
	if s == nil || s.deliveries == nil {
		return ErrDeliveryPreflightCompletionUnavailable
	}
	if ctx == nil {
		return ErrDeliveryPreflightCompletionUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	delivery := claim.Delivery
	if err := delivery.Validate(); err != nil {
		return err
	}
	if delivery.Status != domain.DeliverySending ||
		claim.Lease.WorkerID == "" || claim.Lease.FencingToken < 1 ||
		claim.Lease.ExpiresAt.IsZero() || !isRecoveryUTC(at) ||
		!at.Before(claim.Lease.ExpiresAt) {
		return ErrDeliveryLeaseConflict
	}
	if result.Action == DeliveryPreflightReady {
		return nil
	}
	if result.Action != DeliveryPreflightSkip && result.Action != DeliveryPreflightBlock {
		return ErrDeliveryPreflightNotApplicable
	}
	if result.ReasonCode == "" || evidenceRef == "" {
		return domain.ErrInvalidDeliveryChange
	}
	change := domain.DeliveryTransition{At: at, ReasonCode: result.ReasonCode, EvidenceRef: evidenceRef}
	switch result.Action {
	case DeliveryPreflightSkip:
		change.To = domain.DeliverySkipped
		change.RequestNotSent = true
	case DeliveryPreflightBlock:
		change.To = domain.DeliveryBlocked
	}
	next, event, err := delivery.Transition(change)
	if err != nil {
		return err
	}
	return s.deliveries.UpdateWithLease(ctx, delivery.Version, claim.Lease.FencingToken, next, event, at)
}
