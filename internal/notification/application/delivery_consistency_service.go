package application

import (
	"context"
	"errors"
	"strings"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

// DeliveryConsistencySnapshot is a single database-consistent read set.
type DeliveryConsistencySnapshot struct {
	Delivery   domain.DeliveryRecord
	Attempts   []domain.DeliveryAttempt
	StateEvents  []domain.DeliveryStateEvent
}

// DeliveryConsistencySnapshotReader must read the Delivery and both complete histories
// from one consistent database snapshot; it must not combine independent repository reads.
type DeliveryConsistencySnapshotReader interface {
	ReadDeliveryConsistencySnapshot(ctx context.Context, deliveryID string) (snapshot DeliveryConsistencySnapshot, found bool, err error)
}

var (
	ErrDeliveryConsistencyReaderUnavailable = errors.New("notification: consistency snapshot reader unavailable")
	ErrInvalidDeliveryConsistencyQuery       = errors.New("notification: invalid delivery consistency query")
)

// DeliveryConsistencyService assesses one atomically-read persistence snapshot.
type DeliveryConsistencyService struct {
	reader DeliveryConsistencySnapshotReader
}

func NewDeliveryConsistencyService(reader DeliveryConsistencySnapshotReader) (*DeliveryConsistencyService, error) {
	if reader == nil {
		return nil, ErrDeliveryConsistencyReaderUnavailable
	}
	return &DeliveryConsistencyService{reader: reader}, nil
}

func (s *DeliveryConsistencyService) Assess(ctx context.Context, deliveryID string) (DeliveryConsistencyResult, bool, error) {
	if s == nil || s.reader == nil {
		return DeliveryConsistencyResult{}, false, ErrDeliveryConsistencyReaderUnavailable
	}
	if ctx == nil || strings.TrimSpace(deliveryID) == "" {
		return DeliveryConsistencyResult{}, false, ErrInvalidDeliveryConsistencyQuery
	}
	snapshot, found, err := s.reader.ReadDeliveryConsistencySnapshot(ctx, deliveryID)
	if err != nil || !found {
		return DeliveryConsistencyResult{}, found, err
	}
	result := AssessDeliveryConsistency(DeliveryConsistencyInput{
		Delivery: snapshot.Delivery,
		Attempts:               snapshot.Attempts,
		AttemptsComparable:     true,
		StateEvents:            snapshot.StateEvents,
		StateEventsComparable: true,
	})
	return result, true, nil
}
