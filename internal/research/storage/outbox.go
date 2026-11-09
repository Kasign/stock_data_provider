package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"encoding/json"
	"fmt"
	"strings"
	"time"


	"github.com/gowebpki/jcs"

	researchevent "example.com/stock-analysis-flow/internal/research/event"
	researchapp "example.com/stock-analysis-flow/internal/research/application"
)

const (
	researchLifecycleEventType = "research.event.lifecycle_changed"
	researchLifecycleSchemaVersion = "1.0.0"
)

type researchLifecyclePayload struct {
	SourceEventRef string `json:"source_event_ref"`
	ResearchEventRef string `json:"research_event_ref"`
	EventVersion int `json:"event_version"`
	PreviousState *researchevent.EventStatus `json:"previous_state"`
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
		"INSERT INTO research_notification_outbox (event_id, event_type, schema_version, source_module, occurred_at, published_at, subject_ref, idempotency_key, correlation_id, payload_fingerprint, payload, created_at, available_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		envelope.EventID, envelope.EventType, envelope.SchemaVersion, envelope.SourceModule,
		envelope.OccurredAt, envelope.PublishedAt, envelope.SubjectRef, envelope.IdempotencyKey,
		envelope.CorrelationID, envelope.PayloadFingerprint, string(payload), publishedAt.UTC(), formatPIT(publishedAt), publishedAt.UTC())
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
	return buildResearchLifecycleEnvelopeValue(&previous, next, reasonCode, correlationID, transition.ChangedAt, publishedAt)
}

func buildResearchLifecycleCreatedEnvelope(next researchevent.ResearchEvent, reasonCode *string,
	correlationID string, changedAt time.Time) (researchLifecycleEnvelope, []byte, error) {
	if err := next.Validate(); err != nil { return researchLifecycleEnvelope{}, nil, err }
	if next.EventVersion != 1 { return researchLifecycleEnvelope{}, nil, fmt.Errorf("created lifecycle Event must be version 1") }
	return buildResearchLifecycleEnvelopeValue(nil, next, reasonCode, correlationID, changedAt, changedAt)
}

func buildResearchLifecycleEnvelopeValue(previous *researchevent.ResearchEvent, next researchevent.ResearchEvent,
	reasonCode *string, correlationID string, changedAt, publishedAt time.Time) (researchLifecycleEnvelope, []byte, error) {
	if !validCorrelationID(correlationID) { return researchLifecycleEnvelope{}, nil, fmt.Errorf("correlation_id must contain 1..128 safe ASCII characters") }
	if changedAt.IsZero() || changedAt.Location() != time.UTC { return researchLifecycleEnvelope{}, nil, fmt.Errorf("changed_at must be a non-zero UTC time") }
	if publishedAt.IsZero() || publishedAt.Location() != time.UTC { return researchLifecycleEnvelope{}, nil, fmt.Errorf("published_at must be a non-zero UTC time") }
	if reasonCode != nil && (*reasonCode == "" || *reasonCode != strings.TrimSpace(*reasonCode)) {
		return researchLifecycleEnvelope{}, nil, fmt.Errorf("reason_code must be null or a trimmed non-empty value")
	}
	researchEventRef := "research-event:" + next.EventID
	sourceEventRef := fmt.Sprintf("research-event-version:%s:%d", next.EventID, next.EventVersion)
	var previousState *researchevent.EventStatus
	if previous != nil { previousState = eventStatusPointer(previous.Status) }
	payloadValue := researchLifecyclePayload{
		SourceEventRef: sourceEventRef, ResearchEventRef: researchEventRef,
		EventVersion: next.EventVersion, PreviousState: previousState, CurrentState: next.Status,
		ReasonCode: reasonCode,
		SystemAsOf: next.SystemAsOf.UTC().Format(time.RFC3339Nano),
		ChangedAt: changedAt.UTC().Format(time.RFC3339Nano),
	}
	payload, err := json.Marshal(payloadValue)
	if err != nil { return researchLifecycleEnvelope{}, nil, fmt.Errorf("encode lifecycle payload: %w", err) }
	occurredAt := changedAt.UTC().Format(time.RFC3339Nano)
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

func insertResearchLifecycleCreatedOutbox(ctx context.Context, tx *sql.Tx, next researchevent.ResearchEvent,
	reasonCode *string, correlationID string, publishedAt time.Time) error {
	envelope, payload, err := buildResearchLifecycleCreatedEnvelope(next, reasonCode, correlationID, publishedAt)
	if err != nil { return err }
	_, err = tx.ExecContext(ctx,
		"INSERT INTO research_notification_outbox (event_id, event_type, schema_version, source_module, occurred_at, published_at, subject_ref, idempotency_key, correlation_id, payload_fingerprint, payload, created_at, available_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		envelope.EventID, envelope.EventType, envelope.SchemaVersion, envelope.SourceModule,
		envelope.OccurredAt, envelope.PublishedAt, envelope.SubjectRef, envelope.IdempotencyKey,
		envelope.CorrelationID, envelope.PayloadFingerprint, string(payload), publishedAt.UTC(), formatPIT(publishedAt), publishedAt.UTC())
	if err != nil { return fmt.Errorf("insert Research lifecycle-created Outbox record: %w", err) }
	return nil
}

func eventStatusPointer(value researchevent.EventStatus) *researchevent.EventStatus { return &value }

func validCorrelationID(value string) bool {
	if value == "" || len(value) > 128 { return false }
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == ':') { return false }
	}
	return true
}

func newOutboxEventID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil { return "", fmt.Errorf("generate Notification event ID: %w", err) }
	return "nte_" + hex.EncodeToString(value[:]), nil
}



type OutboxRepository struct { base repositoryBase }

func NewOutboxRepository(db *sql.DB, clock Clock) (*OutboxRepository, error) {
	base, err := newRepositoryBase(db, clock)
	if err != nil { return nil, err }
	return &OutboxRepository{base: base}, nil
}

func (r *OutboxRepository) ClaimOutbox(ctx context.Context, workerID string, limit int, leaseUntil time.Time) ([]researchapp.NotificationEventEnvelope, error) {
	if r == nil { return nil, ErrInvalidRepository }
	return r.base.ClaimOutbox(ctx, workerID, limit, leaseUntil)
}

func (r *OutboxRepository) MarkOutboxPublished(ctx context.Context, eventID, workerID, leaseToken string, deliveredAt time.Time) error {
	if r == nil { return ErrInvalidRepository }
	return r.base.MarkOutboxPublished(ctx, eventID, workerID, leaseToken, deliveredAt)
}

func (r *OutboxRepository) ScheduleOutboxRetry(ctx context.Context, eventID, workerID, leaseToken string, retryAt time.Time, reasonCode string, updatedAt time.Time) error {
	if r == nil { return ErrInvalidRepository }
	return r.base.ScheduleOutboxRetry(ctx, eventID, workerID, leaseToken, retryAt, reasonCode, updatedAt)
}

func (r *OutboxRepository) RejectOutbox(ctx context.Context, eventID, workerID, leaseToken, reasonCode string, rejectedAt time.Time) error {
	if r == nil { return ErrInvalidRepository }
	return r.base.RejectOutbox(ctx, eventID, workerID, leaseToken, reasonCode, rejectedAt)
}

var _ researchapp.OutboxDeliveryRepository = (*OutboxRepository)(nil)

func (r *repositoryBase) ClaimOutbox(ctx context.Context, workerID string, limit int, leaseUntil time.Time) ([]researchapp.NotificationEventEnvelope, error) {
	if r == nil || r.db == nil { return nil, ErrInvalidRepository }
	if ctx == nil || !validCorrelationID(workerID) || limit < 1 || limit > 100 { return nil, fmt.Errorf("invalid Outbox claim parameters") }
	now := r.now().UTC()
	if now.IsZero() || leaseUntil.IsZero() || leaseUntil.Location() != time.UTC || !leaseUntil.After(now) {
		return nil, fmt.Errorf("Outbox lease deadline must be a future UTC time")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil { return nil, fmt.Errorf("begin Research Outbox claim: %w", err) }
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx,
		"SELECT event_id, event_type, schema_version, source_module, occurred_at, published_at, subject_ref, idempotency_key, correlation_id, payload_fingerprint, payload FROM research_notification_outbox WHERE ((delivery_status IN ('pending','retry_wait') AND available_at <= ?) OR (delivery_status='leased' AND lease_until <= ?)) ORDER BY created_at, event_id LIMIT ? FOR UPDATE",
		formatPIT(now), formatPIT(now), limit)
	if err != nil { return nil, fmt.Errorf("select Research Outbox messages: %w", err) }
	values := make([]researchapp.NotificationEventEnvelope, 0, limit)
	for rows.Next() {
		var value researchapp.NotificationEventEnvelope
		if err := rows.Scan(&value.EventID, &value.EventType, &value.SchemaVersion, &value.SourceModule,
			&value.OccurredAt, &value.PublishedAt, &value.SubjectRef, &value.IdempotencyKey,
			&value.CorrelationID, &value.PayloadFingerprint, &value.Payload); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan Research Outbox message: %w", err)
		}
		if err := validateStoredOutboxEnvelope(value); err != nil {
			_ = rows.Close()
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil { _ = rows.Close(); return nil, fmt.Errorf("iterate Research Outbox messages: %w", err) }
	if err := rows.Close(); err != nil { return nil, fmt.Errorf("close Research Outbox rows: %w", err) }
	for index := range values {
		leaseToken, err := newOutboxLeaseToken()
		if err != nil { return nil, err }
		values[index].LeaseToken = leaseToken
		value := values[index]
		result, err := tx.ExecContext(ctx,
			"UPDATE research_notification_outbox SET delivery_status='leased', lease_owner=?, lease_token=?, lease_until=?, attempt_count=attempt_count+1, updated_at=? WHERE event_id=?",
			workerID, leaseToken, formatPIT(leaseUntil), now, value.EventID)
		if err != nil { return nil, fmt.Errorf("lease Research Outbox message: %w", err) }
		affected, err := result.RowsAffected()
		if err != nil { return nil, fmt.Errorf("verify Research Outbox lease: %w", err) }
		if affected != 1 { return nil, fmt.Errorf("%w: Research Outbox lease was not acquired", ErrConflict) }
	}
	if err := tx.Commit(); err != nil { return nil, fmt.Errorf("commit Research Outbox claim: %w", err) }
	return values, nil
}

func (r *repositoryBase) MarkOutboxPublished(ctx context.Context, eventID, workerID, leaseToken string, deliveredAt time.Time) error {
	if r == nil || r.db == nil { return ErrInvalidRepository }
	if ctx == nil || strings.TrimSpace(eventID) == "" || !validCorrelationID(workerID) || !validLeaseToken(leaseToken) ||
		deliveredAt.IsZero() || deliveredAt.Location() != time.UTC { return fmt.Errorf("invalid Research Outbox confirmation") }
	result, err := r.db.ExecContext(ctx,
		"UPDATE research_notification_outbox SET delivery_status='published', delivered_at=?, lease_owner=NULL, lease_token=NULL, lease_until=NULL, updated_at=? WHERE event_id=? AND delivery_status='leased' AND lease_owner=? AND lease_token=? AND lease_until>?",
		formatPIT(deliveredAt), deliveredAt, eventID, workerID, leaseToken, formatPIT(deliveredAt))
	if err != nil { return fmt.Errorf("confirm Research Outbox delivery: %w", err) }
	affected, err := result.RowsAffected()
	if err != nil { return fmt.Errorf("verify Research Outbox confirmation: %w", err) }
	if affected == 1 { return nil }
	var status string
	err = r.db.QueryRowContext(ctx, "SELECT delivery_status FROM research_notification_outbox WHERE event_id=?", eventID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) { return ErrNotFound }
	if err != nil { return fmt.Errorf("read Research Outbox confirmation state: %w", err) }
	if status == "published" { return nil }
	return fmt.Errorf("%w: Research Outbox message is not leased by this worker", ErrConflict)
}

func (r *repositoryBase) ScheduleOutboxRetry(ctx context.Context, eventID, workerID, leaseToken string, retryAt time.Time, reasonCode string, updatedAt time.Time) error {
	if r == nil || r.db == nil { return ErrInvalidRepository }
	if ctx == nil || strings.TrimSpace(eventID) == "" || !validCorrelationID(workerID) || !validLeaseToken(leaseToken) ||
		!validCorrelationID(reasonCode) || retryAt.IsZero() || retryAt.Location() != time.UTC ||
		updatedAt.IsZero() || updatedAt.Location() != time.UTC || !retryAt.After(updatedAt) {
		return fmt.Errorf("invalid Research Outbox retry parameters")
	}
	result, err := r.db.ExecContext(ctx,
		"UPDATE research_notification_outbox SET delivery_status='retry_wait', available_at=?, lease_owner=NULL, lease_token=NULL, lease_until=NULL, last_error_code=?, updated_at=? WHERE event_id=? AND delivery_status='leased' AND lease_owner=? AND lease_token=? AND lease_until>?",
		formatPIT(retryAt), reasonCode, updatedAt, eventID, workerID, leaseToken, formatPIT(updatedAt))
	if err != nil { return fmt.Errorf("schedule Research Outbox retry: %w", err) }
	affected, err := result.RowsAffected()
	if err != nil { return fmt.Errorf("verify Research Outbox retry: %w", err) }
	if affected != 1 { return fmt.Errorf("%w: Research Outbox message is not leased by this worker", ErrConflict) }
	return nil
}

func validateStoredOutboxEnvelope(value researchapp.NotificationEventEnvelope) error {
	if value.EventID == "" || value.EventType != researchLifecycleEventType ||
		value.SchemaVersion != researchLifecycleSchemaVersion || value.SourceModule != "research" ||
		value.SubjectRef == "" || value.IdempotencyKey == "" || !validCorrelationID(value.CorrelationID) ||
		len(value.PayloadFingerprint) != 64 || !json.Valid(value.Payload) {
		return fmt.Errorf("%w: stored Research Outbox envelope is invalid", ErrCorruptRecord)
	}
	fingerprintInput := researchLifecycleFingerprintInput{
		SourceModule: value.SourceModule, EventType: value.EventType, SchemaVersion: value.SchemaVersion,
		SubjectRef: value.SubjectRef, OccurredAt: value.OccurredAt, Payload: value.Payload,
	}
	encoded, err := json.Marshal(fingerprintInput)
	if err != nil { return fmt.Errorf("%w: encode stored Research Outbox fingerprint input: %v", ErrCorruptRecord, err) }
	canonical, err := jcs.Transform(encoded)
	if err != nil { return fmt.Errorf("%w: canonicalize stored Research Outbox fingerprint input: %v", ErrCorruptRecord, err) }
	sum := sha256.Sum256(canonical)
	if hex.EncodeToString(sum[:]) != value.PayloadFingerprint {
		return fmt.Errorf("%w: stored Research Outbox fingerprint mismatch", ErrCorruptRecord)
	}
	return nil
}


func (r *repositoryBase) RejectOutbox(ctx context.Context, eventID, workerID, leaseToken, reasonCode string, rejectedAt time.Time) error {
	if r == nil || r.db == nil { return ErrInvalidRepository }
	if ctx == nil || strings.TrimSpace(eventID) == "" || !validCorrelationID(workerID) || !validLeaseToken(leaseToken) ||
		!validCorrelationID(reasonCode) || rejectedAt.IsZero() || rejectedAt.Location() != time.UTC {
		return fmt.Errorf("invalid Research Outbox rejection parameters")
	}
	result, err := r.db.ExecContext(ctx,
		"UPDATE research_notification_outbox SET delivery_status='rejected', last_error_code=?, lease_owner=NULL, lease_token=NULL, lease_until=NULL, updated_at=? WHERE event_id=? AND delivery_status='leased' AND lease_owner=? AND lease_token=? AND lease_until>?",
		reasonCode, rejectedAt, eventID, workerID, leaseToken, formatPIT(rejectedAt))
	if err != nil { return fmt.Errorf("reject Research Outbox message: %w", err) }
	affected, err := result.RowsAffected()
	if err != nil { return fmt.Errorf("verify Research Outbox rejection: %w", err) }
	if affected != 1 { return fmt.Errorf("%w: Research Outbox message is not leased by this worker", ErrConflict) }
	return nil
}


func newOutboxLeaseToken() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil { return "", fmt.Errorf("generate Research Outbox lease token: %w", err) }
	return hex.EncodeToString(value[:]), nil
}

func validLeaseToken(value string) bool {
	if len(value) != 32 { return false }
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) { return false }
	}
	return true
}
