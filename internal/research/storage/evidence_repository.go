package storage

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	researchevidence "example.com/stock-analysis-flow/internal/research/evidence"
)

const evidenceTable = "research_evidence_records"

type EvidenceRepository struct { base repositoryBase }

func NewEvidenceRepository(db *sql.DB, clock Clock) (*EvidenceRepository, error) {
	base, err := newRepositoryBase(db, clock)
	if err != nil { return nil, err }
	return &EvidenceRepository{base: base}, nil
}

var _ researchevidence.Repository = (*EvidenceRepository)(nil)

// Append stores one immutable Evidence snapshot. The complete payload,
// including authorization_ref, defines idempotency under RSC-OPEN-007.
func (r *EvidenceRepository) Append(ctx context.Context, value researchevidence.Evidence) error {
	if r == nil { return ErrInvalidRepository }
	if ctx == nil { return errors.New("context is nil") }
	if err := value.Validate(); err != nil { return fmt.Errorf("validate Evidence: %w", err) }
	payload, digest, err := encodePayload(value)
	if err != nil { return err }
	now := r.base.now().UTC()
	if now.IsZero() { return errors.New("repository clock returned zero time") }

	var availableAt any
	if value.AvailableAt != nil { availableAt = formatPIT(*value.AvailableAt) }
	query := "INSERT INTO research_evidence_records " +
		"(evidence_id, source_owner, source_ref, source_version, available_at, system_observed_at, system_as_of, quality_status, content_level, authorization_ref, content_fingerprint, payload_sha256, payload, created_at) " +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) " +
		"ON DUPLICATE KEY UPDATE evidence_id=evidence_id"
	if _, err := r.base.db.ExecContext(ctx, query,
		value.EvidenceID, string(value.SourceOwner), value.SourceRef, nullableStringValue(value.SourceVersion),
		availableAt, formatPIT(value.SystemObservedAt), formatPIT(value.SystemAsOf),
		string(value.QualityStatus), string(value.ContentLevel), value.AuthorizationRef,
		value.ContentFingerprint, digest, string(payload), now,
	); err != nil {
		return fmt.Errorf("insert immutable Research Evidence: %w", err)
	}

	var storedDigest string
	var storedPayload []byte
	if err := r.base.db.QueryRowContext(ctx,
		"SELECT payload_sha256, payload FROM research_evidence_records WHERE evidence_id=?", value.EvidenceID,
	).Scan(&storedDigest, &storedPayload); err != nil {
		return fmt.Errorf("read Research Evidence after append: %w", err)
	}
	if err := verifyStoredPayload(storedPayload, storedDigest); err != nil { return err }
	if storedDigest != digest || !bytes.Equal(storedPayload, payload) {
		return fmt.Errorf("%w: Evidence %s already has different immutable content", ErrConflict, value.EvidenceID)
	}
	return nil
}

func (r *EvidenceRepository) Get(ctx context.Context, id string) (researchevidence.Evidence, error) {
	if r == nil { return researchevidence.Evidence{}, ErrInvalidRepository }
	if ctx == nil { return researchevidence.Evidence{}, errors.New("context is nil") }
	if id == "" { return researchevidence.Evidence{}, errors.New("evidence_id is required") }

	var value researchevidence.Evidence
	var storedDigest string
	var payload []byte
	if err := r.base.db.QueryRowContext(ctx,
		"SELECT payload_sha256, payload FROM research_evidence_records WHERE evidence_id=?", id,
	).Scan(&storedDigest, &payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) { return value, ErrNotFound }
		return value, fmt.Errorf("read Research Evidence: %w", err)
	}
	if err := verifyStoredPayload(payload, storedDigest); err != nil { return value, err }
	if err := json.Unmarshal(payload, &value); err != nil {
		return value, fmt.Errorf("%w: decode Evidence payload: %v", ErrCorruptRecord, err)
	}
	if err := value.Validate(); err != nil {
		return value, fmt.Errorf("%w: stored Evidence is invalid: %v", ErrCorruptRecord, err)
	}
	if value.EvidenceID != id {
		return value, fmt.Errorf("%w: stored Evidence identity mismatch", ErrCorruptRecord)
	}
	return value, nil
}

func nullableStringValue(value *string) any {
	if value == nil { return nil }
	return *value
}
