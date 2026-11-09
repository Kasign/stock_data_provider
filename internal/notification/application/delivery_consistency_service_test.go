package application

import (
	"context"
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

type consistencySnapshotReaderFake struct {
	snapshot DeliveryConsistencySnapshot
	found    bool
	err      error
}

func (f consistencySnapshotReaderFake) ReadDeliveryConsistencySnapshot(context.Context, string) (DeliveryConsistencySnapshot, bool, error) {
	return f.snapshot, f.found, f.err
}

func TestDeliveryConsistencyService(t *testing.T) {
	delivery := validDeliveryForRepository()
	service, err := NewDeliveryConsistencyService(consistencySnapshotReaderFake{
		snapshot: DeliveryConsistencySnapshot{Delivery: delivery, Attempts: []domain.DeliveryAttempt{}, StateEvents: []domain.DeliveryStateEvent{}},
		found: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, found, err := service.Assess(context.Background(), delivery.DeliveryID)
	if err != nil || !found || result.Status != DeliveryConsistencyPass {
		t.Fatalf("result=%+v found=%v err=%v", result, found, err)
	}

	readErr := errors.New("database unavailable")
	service, err = NewDeliveryConsistencyService(consistencySnapshotReaderFake{err: readErr})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Assess(context.Background(), delivery.DeliveryID); !errors.Is(err, readErr) {
		t.Fatalf("read error must be preserved, got %v", err)
	}

	service, err = NewDeliveryConsistencyService(consistencySnapshotReaderFake{})
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := service.Assess(context.Background(), delivery.DeliveryID); err != nil || found {
		t.Fatalf("missing delivery must be distinct from read failure: found=%v err=%v", found, err)
	}
}

func TestDeliveryConsistencyServiceRejectsInvalidQueries(t *testing.T) {
	service, err := NewDeliveryConsistencyService(consistencySnapshotReaderFake{found: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		ctx context.Context
		id string
	}{
		{name: "nil context", ctx: nil, id: "delivery-1"},
		{name: "empty id", ctx: context.Background(), id: ""},
		{name: "blank id", ctx: context.Background(), id: "  "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, found, err := service.Assess(tc.ctx, tc.id); !errors.Is(err, ErrInvalidDeliveryConsistencyQuery) || found {
				t.Fatalf("found=%v err=%v, want invalid query", found, err)
			}
		})
	}
	if _, _, err := (*DeliveryConsistencyService)(nil).Assess(context.Background(), "delivery-1"); !errors.Is(err, ErrDeliveryConsistencyReaderUnavailable) {
		t.Fatalf("nil service error=%v, want reader unavailable", err)
	}
}
