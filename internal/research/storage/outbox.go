package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gowebpki/jcs"

	researchevent "example.com/stock-analysis-flow/internal/research/event"
)

const (
	researchLifecycleEventType = "research.event.lifecycle_changed"
	researchLifecycleSchemaVersion = "1.0.0"
)

type researchLifecyclePayload struct {
	SourceEventRef string `json:"source_event_ref"`
	ResearchEventRef string `json:"research_event_ref"`
	EventVersion int `json:"event_version"`
	PreviousState researchevent.EventStatus `json:"previous_state"`
	CurrentState researchevent.EventStatus `json:"current_state"`
	ReasonCode *string `json:"reason_code"`
	SystemAsOf string `json:"system_as_of"`
	ChangedAt string `json:"changed_at"`
}

type researchLifecycleFingerprintInput struct {
	SourceModule string `json:"source_module"`
	EventType string `json:"event_type"`
	SchemaVersion string `json:"schema_version"`
	SubjectRef string `json:"subject_ref"`
	OccurredAt string `json:"occurred_at"`
	Payload json.RawMessage `json:"payload"`
}

type researchLifecycleEnvelope struct {
	EventID string `json:"event_id"`
	EventType string `json:"event_type"`
	SchemaVersion string `json:"schema_version"`
	SourceModule string `json:"source_module"`
	OccurredAt string `json:"occurred_at"`
	PublishedAt string `json:"published_at"`
	SubjectRef string `json:"subject_ref"`
	IdempotencyKey string `json:"idempotency_key"`
	CorrelationID string `json:"correlation_id"`
	PayloadFingerprint string `json:"payload_fingerprint"`
	Payload json.RawMessage `json:"payload"`
}

func insertResearchLifecycleOutbox(ctx context.Context, tx *sql.Tx, previous, next researchevent.ResearchEvent,
	transition researchevent.LifecycleTransition, reasonCode *string, correlationID string, publishedAt time.Time) error {
	envelope, payload, err := buildResearchLifecycleEnvelope(previous, next, transition, reasonCode, correlationID, publishedAt)
	if err != nil { return err }
	_, err = tx.ExecContext(ctx,
		"INSERT INTO research_notification_outbox (event_id, event_type, schema_version, source_module, occurred_at, published_at, subject_ref, idempotency_key, correlation_id, payload_fingerprint, payload, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		envelope.EventID, envelope.EventType, envelope.SchemaVersion, envelope.SourceModule,
		envelope.OccurredAt, envelope.PublishedAt, envelope.SubjectRef, envelope.IdempotencyKey,
		envelope.CorrelationID, envelope.PayloadFingerprint, string(payload), publishedAt.UTC())
	if err != nil { return fmt.Errorf("insert Research lifecycle Outbox record: %w", err) }
	return nil
}

func buildResearchLifecycleEnvelope(previous, next researchevent.ResearchEvent,
	transition researchevent.LifecycleTransition, reasonCode *string, correlationID string,
	publishedAt time.Time) (researchLifecycleEnvelope, []byte, error) {
	if err := transition.Validate(); err != nil { return researchLifecycleEnvelope{}, nil, err }
	if previous.EventID != next.EventID || transition.EventID != next.EventID ||
		transition.EventVersion != next.EventVersion || transition.FromStatus != previous.Status ||
		transition.ToStatus != next.Status {
		return researchLifecycleEnvelope{}, nil, fmt.Errorf("lifecycle transition identity does not match Event versions")
	}
	if correlationID == "" || len(correlationID) > 128 {
		return researchLifecycleEnvelope{}, nil, fmt.Errorf("correlation_id must contain 1..128 characters")
	}
	if publishedAt.IsZero() || publishedAt.Location() != time.UTC {
		return researchLifecycleEnvelope{}, nil, fmt.Errorf("published_at must be a non-zero UTC time")
	}
	if reasonCode != nil && (*reasonCode == "" || *reasonCode != trimASCII(*reasonCode)) {
		return researchLifecycleEnvelope{}, nil, fmt.Errorf("reason_code must be null or a trimmed non-empty value")
	}
	researchEventRef := "research-event:" + next.EventID
	sourceEventRef := fmt.Sprintf("research-event-version:%s:%d", next.EventID, next.EventVersion)
	payloadValue := researchLifecyclePayload{
		SourceEventRef: sourceEventRef, ResearchEventRef: researchEventRef,
		EventVersion: next.EventVersion, PreviousState: previous.Status,
		CurrentState: next.Status, ReasonCode: reasonCode,
		SystemAsOf: next.SystemAsOf.UTC().Format(time.RFC3339Nano),
		ChangedAt: transition.ChangedAt.UTC().Format(time.RFC3339Nano),
	}
	payload, err := json.Marshal(payloadValue)
	if err != nil { return researchLifecycleEnvelope{}, nil, fmt.Errorf("encode lifecycle payload: %w", err) }
	occurredAt := transition.ChangedAt.UTC().Format(time.RFC3339Nano)
	fingerprintInput := researchLifecycleFingerprintInput{
		SourceModule: "research", EventType: researchLifecycleEventType,
		SchemaVersion: researchLifecycleSchemaVersion, SubjectRef: researchEventRef,
		OccurredAt: occurredAt, Payload: payload,
	}
	fingerprintJSON, err := json.Marshal(fingerprintInput)
	if err != nil { return researchLifecycleEnvelope{}, nil, fmt.Errorf("encode fingerprint input: %w", err) }
	canonical, err := jcs.Transform(fingerprintJSON)
	if err != nil { return researchLifecycleEnvelope{}, nil, fmt.Errorf("canonicalize lifecycle fingerprint input: %w", err) }
	sum := sha256.Sum256(canonical)
	eventID, err := newOutboxEventID()
	if err != nil { return researchLifecycleEnvelope{}, nil, err }
	envelope := researchLifecycleEnvelope{
		EventID: eventID, EventType: researchLifecycleEventType,
		SchemaVersion: researchLifecycleSchemaVersion, SourceModule: "research",
		OccurredAt: occurredAt, PublishedAt: publishedAt.UTC().Format(time.RFC3339Nano),
		SubjectRef: researchEventRef,
		IdempotencyKey: fmt.Sprintf("research-event-state:%s:%d", researchEventRef, next.EventVersion),
		CorrelationID: correlationID, PayloadFingerprint: hex.EncodeToString(sum[:]),
		Payload: payload,
	}
	return envelope, payload, nil
}

func newOutboxEventID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil { return "", fmt.Errorf("generate Notification event ID: %w", err) }
	return "nte_" + hex.EncodeToString(value[:]), nil
}

func trimASCII(value string) string {
	start, end := 0, len(value)
	for start < end && (value[start] == ' ' || value[start] == '\t' || value[start] == '\n' || value[start] == '\r') { start++ }
	for end > start && (value[end-1] == ' ' || value[end-1] == '\t' || value[end-1] == '\n' || value[end-1] == '\r') { end-- }
	return value[start:end]
}
