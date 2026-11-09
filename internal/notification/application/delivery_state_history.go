package application

import (
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var ErrDeliveryStateHistory = errors.New("notification: invalid delivery state history")

// DeliveryStateHistoryConsistency describes a read-only comparison of immutable events and snapshot.
type DeliveryStateHistoryConsistency string

const (
	DeliveryStateHistoryConsistent DeliveryStateHistoryConsistency = "consistent"
)

// ValidateDeliveryStateHistory checks the complete event chain against the current Delivery snapshot.
// Version 1 is the creation snapshot (queued) and intentionally has no StateEvent;
// subsequent versions must have exactly one event each.
func ValidateDeliveryStateHistory(delivery domain.DeliveryRecord, events []domain.DeliveryStateEvent) (DeliveryStateHistoryConsistency, error) {
	if err := delivery.Validate(); err != nil {
		return "", errors.Join(ErrDeliveryStateHistory, err)
	}
	if int64(len(events)) != delivery.Version-1 {
		return "", ErrDeliveryStateHistory
	}

	previousStatus := domain.DeliveryQueued
	previousAt := delivery.CreatedAt
	for index, event := range events {
		expectedVersion := int64(index) + 2
		if event.DeliveryID != delivery.DeliveryID ||
			event.Version != expectedVersion ||
			event.From != previousStatus ||
			!validStateEventStatus(event.From) ||
			!validStateEventStatus(event.To) ||
			event.From == event.To ||
			event.ChangedAt.IsZero() ||
			event.ChangedAt.Location() != time.UTC ||
			event.ChangedAt.Before(previousAt) ||
			event.ChangedAt.Before(delivery.CreatedAt) {
			return "", ErrDeliveryStateHistory
		}
		previousStatus = event.To
		previousAt = event.ChangedAt
	}
	if len(events) == 0 {
		if delivery.Version != 1 || delivery.Status != domain.DeliveryQueued || !delivery.UpdatedAt.Equal(delivery.CreatedAt) {
			return "", ErrDeliveryStateHistory
		}
		return DeliveryStateHistoryConsistent, nil
	}
	last := events[len(events)-1]
	if last.To != delivery.Status || last.Version != delivery.Version ||
		!last.ChangedAt.Equal(delivery.UpdatedAt) {
		return "", ErrDeliveryStateHistory
	}
	return DeliveryStateHistoryConsistent, nil
}

func validStateEventStatus(status domain.DeliveryStatus) bool {
	switch status {
	case domain.DeliveryQueued, domain.DeliverySending, domain.DeliveryRetryWait,
		domain.DeliverySent, domain.DeliverySkipped, domain.DeliveryBlocked,
		domain.DeliveryFailed, domain.DeliveryUnknown:
		return true
	default:
		return false
	}
}
