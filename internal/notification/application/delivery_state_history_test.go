package application

import (
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

func TestValidateDeliveryStateHistory(t *testing.T) {
	delivery := validDeliveryForRepository()
	at := delivery.CreatedAt.Add(time.Minute)
	sending := domain.DeliveryStateEvent{
		DeliveryID: delivery.DeliveryID, Version: 2,
		From: domain.DeliveryQueued, To: domain.DeliverySending, ChangedAt: at,
	}
	snapshot := delivery
	snapshot.Status = domain.DeliverySending
	snapshot.Version = 2
	snapshot.UpdatedAt = at

	tests := []struct {
		name    string
		record  domain.DeliveryRecord
		events  []domain.DeliveryStateEvent
		wantErr bool
	}{
		{name: "initial version has no event", record: delivery},
		{name: "complete chain", record: snapshot, events: []domain.DeliveryStateEvent{sending}},
		{name: "event reason mismatch", record: func() domain.DeliveryRecord {
			value := snapshot
			reason := "snapshot_reason"
				value.ReasonCode = &reason
			return value
		}(), events: []domain.DeliveryStateEvent{func() domain.DeliveryStateEvent {
			event := sending
			reason := "event_reason"
			event.ReasonCode = &reason
			return event
		}()}, wantErr: true},

		{name: "missing event", record: snapshot, wantErr: true},
		{name: "version gap", record: snapshot, events: []domain.DeliveryStateEvent{{DeliveryID: delivery.DeliveryID, Version: 3, From: domain.DeliveryQueued, To: domain.DeliverySending, ChangedAt: at}}, wantErr: true},
		{name: "broken status chain", record: snapshot, events: []domain.DeliveryStateEvent{{DeliveryID: delivery.DeliveryID, Version: 2, From: domain.DeliveryBlocked, To: domain.DeliverySending, ChangedAt: at}}, wantErr: true},
		{name: "illegal transition", record: snapshot, events: []domain.DeliveryStateEvent{{DeliveryID: delivery.DeliveryID, Version: 2, From: domain.DeliveryQueued, To: domain.DeliverySent, ChangedAt: at}}, wantErr: true},
		{name: "snapshot mismatch", record: delivery, events: []domain.DeliveryStateEvent{sending}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ValidateDeliveryStateHistory(test.record, test.events)
			if test.wantErr {
				if !errors.Is(err, ErrDeliveryStateHistory) {
					t.Fatalf("expected ErrDeliveryStateHistory, got %v", err)
				}
				return
			}
			if err != nil || got != DeliveryStateHistoryConsistent {
				t.Fatalf("got=%q err=%v", got, err)
			}
		})
	}
}

func TestValidateDeliveryStateHistoryRejectsTimeRegression(t *testing.T) {
	delivery := validDeliveryForRepository()
	firstAt := delivery.CreatedAt.Add(time.Minute)
	secondAt := firstAt.Add(-time.Second)
	record := delivery
	record.Status = domain.DeliverySent
	record.Version = 3
	record.UpdatedAt = firstAt
	events := []domain.DeliveryStateEvent{
		{DeliveryID: delivery.DeliveryID, Version: 2, From: domain.DeliveryQueued, To: domain.DeliverySending, ChangedAt: firstAt},
		{DeliveryID: delivery.DeliveryID, Version: 3, From: domain.DeliverySending, To: domain.DeliverySent, ChangedAt: secondAt},
	}
	if _, err := ValidateDeliveryStateHistory(record, events); !errors.Is(err, ErrDeliveryStateHistory) {
		t.Fatalf("expected time regression rejection, got %v", err)
	}
}
