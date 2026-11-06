package domain

import (
	"errors"
	"testing"
)

func TestValidateInboxReplay(t *testing.T) {
	base := EventEnvelope{
		SourceModule: "quant", EventID: "event-1",
		IdempotencyKey: "publish-1", PayloadFingerprint: "fingerprint-a",
	}
	if err := ValidateInboxReplay(base, base); err != nil {
		t.Fatalf("same event replay should be accepted: %v", err)
	}
	cases := []struct {
		name string
		edit func(*EventEnvelope)
	}{
		{name: "event id", edit: func(e *EventEnvelope) { e.EventID = "event-2" }},
		{name: "idempotency key", edit: func(e *EventEnvelope) { e.IdempotencyKey = "publish-2" }},
		{name: "fingerprint", edit: func(e *EventEnvelope) { e.PayloadFingerprint = "fingerprint-b" }},
		{name: "source module", edit: func(e *EventEnvelope) { e.SourceModule = "research" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			incoming := base
			tc.edit(&incoming)
			if err := ValidateInboxReplay(base, incoming); !errors.Is(err, ErrInboxIdempotencyConflict) {
				t.Fatalf("expected idempotency conflict, got %v", err)
			}
		})
	}
}

func TestValidateInboxReplayRequiresStablePublishedAt(t *testing.T) {
	existing := EventEnvelope{
		SourceModule: "quant", EventID: "event-1",
		IdempotencyKey: "publish-1", PublishedAt: "2026-11-06T09:00:00Z",
		PayloadFingerprint: "fingerprint-a",
	}
	incoming := existing
	incoming.PublishedAt = "2026-11-06T09:00:01Z"
	if !errors.Is(ValidateInboxReplay(existing, incoming), ErrInboxIdempotencyConflict) {
		t.Fatal("published_at change must conflict on event replay")
	}
}
