package application

import (
	"context"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var (
	ErrEventValidatorUnavailable = errors.New("notification: event validator unavailable")
	ErrInboxRepositoryUnavailable = errors.New("notification: inbox repository unavailable")
	ErrClockUnavailable = errors.New("notification: clock unavailable")
)

type EventValidator interface {
	ValidateEvent(raw []byte) (domain.EventEnvelope, error)
}

// InboxRepository.Accept must atomically check both unique identities:
// (source_module,event_id) and (source_module,idempotency_key). A new insert must
// persist the Inbox record and its fan-out work item in the same local transaction.
type InboxRepository interface {
	Accept(ctx context.Context, event domain.EventEnvelope, receivedAt time.Time) (record domain.InboxRecord, duplicate bool, err error)
}

type Clock interface {
	Now() time.Time
}

type ReceiveEventResult struct {
	EventID    string
	AcceptedAt time.Time
	Duplicate  bool
}

type EventReceiver struct {
	validator EventValidator
	inbox     InboxRepository
	clock     Clock
}

func NewEventReceiver(validator EventValidator, inbox InboxRepository, clock Clock) (*EventReceiver, error) {
	if validator == nil {
		return nil, ErrEventValidatorUnavailable
	}
	if inbox == nil {
		return nil, ErrInboxRepositoryUnavailable
	}
	if clock == nil {
		return nil, ErrClockUnavailable
	}
	return &EventReceiver{validator: validator, inbox: inbox, clock: clock}, nil
}

// Receive validates the complete Envelope and Payload before the Inbox repository
// can persist the event or create fan-out work.
func (r *EventReceiver) Receive(ctx context.Context, raw []byte) (ReceiveEventResult, error) {
	var result ReceiveEventResult
	if r == nil || r.validator == nil || r.inbox == nil || r.clock == nil {
		return result, errors.New("notification: event receiver is not initialized")
	}
	event, err := r.validator.ValidateEvent(raw)
	if err != nil {
		return result, err
	}
	receivedAt := r.clock.Now()
	if receivedAt.IsZero() || receivedAt.Location() != time.UTC {
		return result, errors.New("notification: receiver clock must return UTC time")
	}
	record, duplicate, err := r.inbox.Accept(ctx, event, receivedAt)
	if err != nil {
		return result, err
	}
	return ReceiveEventResult{
		EventID:    record.Event.EventID,
		AcceptedAt: record.ReceivedAt,
		Duplicate:  duplicate,
	}, nil
}
