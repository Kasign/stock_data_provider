package domain

import (
	"errors"
	"time"
)

// InboxRecord is the immutable accepted-event record. Repository adapters must persist
// this record and its pending fan-out work item atomically.
type InboxRecord struct {
	Event      EventEnvelope
	ReceivedAt time.Time
}

var ErrInboxIdempotencyConflict = errors.New("notification: inbox idempotency conflict")

// ValidateInboxReplay is called after a repository lookup by either Inbox unique key.
// A replay is accepted only when both identity keys and the immutable content digest match.
func ValidateInboxReplay(existing, incoming EventEnvelope) error {
	if existing.SourceModule != incoming.SourceModule ||
		existing.EventID != incoming.EventID ||
		existing.IdempotencyKey != incoming.IdempotencyKey ||
		existing.PublishedAt != incoming.PublishedAt ||
		existing.PayloadFingerprint != incoming.PayloadFingerprint {
		return ErrInboxIdempotencyConflict
	}
	return nil
}
