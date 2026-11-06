package domain

import "errors"

var ErrInboxIdempotencyConflict = errors.New("notification: inbox idempotency conflict")

// ValidateInboxReplay is called after a repository lookup by either Inbox unique key.
// A replay is accepted only when both identity keys and the immutable content digest match.
func ValidateInboxReplay(existing, incoming EventEnvelope) error {
	if existing.SourceModule != incoming.SourceModule ||
		existing.EventID != incoming.EventID ||
		existing.IdempotencyKey != incoming.IdempotencyKey ||
		existing.PayloadFingerprint != incoming.PayloadFingerprint {
		return ErrInboxIdempotencyConflict
	}
	return nil
}
