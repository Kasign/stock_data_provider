package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	researchevent "example.com/stock-analysis-flow/internal/research/event"
)

const eventTable = "research_event_versions"

type EventRepository struct { base repositoryBase }

func NewEventRepository(db *sql.DB, clock Clock) (*EventRepository, error) {
	base, err := newRepositoryBase(db, clock)
	if err != nil { return nil, err }
	return &EventRepository{base: base}, nil
}

var _ researchevent.Repository = (*EventRepository)(nil)

func (r *EventRepository) Append(ctx context.Context, value researchevent.ResearchEvent) error {
	if r == nil { return ErrInvalidRepository }
	if err := value.Validate(); err != nil { return fmt.Errorf("validate Event: %w", err) }
	payload, digest, err := encodePayload(value)
	if err != nil { return err }
	now := r.base.now().UTC()
	insert := "INSERT INTO research_event_versions (event_id, event_version, status, system_as_of, payload_sha256, payload, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)"
	return r.base.appendVersion(ctx, "event", value.EventID, int64(value.EventVersion),
		eventTable, "event_id", "event_version", payload, now, insert,
		[]any{value.EventID, value.EventVersion, string(value.Status), formatPIT(value.SystemAsOf), digest, string(payload), now},
		func(previous []byte) error {
			var prior researchevent.ResearchEvent
			if err := json.Unmarshal(previous, &prior); err != nil { return fmt.Errorf("decode previous Event: %w", err) }
			return value.ValidateNextVersion(prior)
		})
}

func (r *EventRepository) GetVersion(ctx context.Context, id string, version int) (researchevent.ResearchEvent, error) {
	if r == nil { return researchevent.ResearchEvent{}, ErrInvalidRepository }
	var value researchevent.ResearchEvent
	payload, err := r.base.getVersion(ctx, eventTable, "event_id", "event_version", id, int64(version))
	if err != nil { return value, err }
	if err := json.Unmarshal(payload, &value); err != nil { return value, corruptStoredRecord("Event", "decode failed", err) }
	if err := value.Validate(); err != nil { return value, corruptStoredRecord("Event", "validation failed", err) }
	if value.EventID != id || value.EventVersion != version { return value, corruptStoredIdentity("Event") }
	return value, nil
}
