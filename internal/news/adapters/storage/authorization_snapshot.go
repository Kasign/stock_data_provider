package storage

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

const maximumAuthorizationSnapshotBytes = 64 * 1024

var _ app.AuthorizationSnapshotRepository = (*MySQLRepository)(nil)

// AppendAuthorizationSnapshot stores a validated immutable NEWS authorization
// decision. The payload hash protects stored bytes; it is not a GOV-CTR-006 JCS
// decision fingerprint verifier.
func (r *MySQLRepository) AppendAuthorizationSnapshot(
	ctx context.Context,
	binding domain.NewsAuthorizationBinding,
	observation domain.Observation,
	version domain.NewsItemVersion,
) error {
	if r == nil || r.db == nil || ctx == nil ||
		domain.ValidateNewsAuthorizationBindingForEvidence(binding, observation, version) != nil {
		return app.ErrInvalidAuthorizationSnapshot
	}
	payload, err := json.Marshal(binding)
	if err != nil || len(payload) == 0 || len(payload) > maximumAuthorizationSnapshotBytes {
		return app.ErrInvalidAuthorizationSnapshot
	}
	sum := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(sum[:])
	ref := binding.AuthorizationRef
	decidedAt, err := time.Parse(time.RFC3339Nano, ref.DecidedAt)
	if err != nil {
		return app.ErrInvalidAuthorizationSnapshot
	}
	expiresAt, err := authorizationExpiry(ref.ExpiresAt)
	if err != nil {
		return app.ErrInvalidAuthorizationSnapshot
	}

	_, insertErr := r.db.ExecContext(ctx, `INSERT INTO news_authorization_snapshots
		(authorization_ref, subject_ref, resource_ref, decision, purpose_code,
		 content_level, decided_at, expires_at, decision_fingerprint, payload_json, payload_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ref.AuthorizationRef, ref.SubjectRef, ref.ResourceRef, string(ref.Decision),
		ref.PurposeCode, string(ref.ContentLevel), decidedAt.UTC(),
		expiresAt, ref.DecisionFingerprint, payload, payloadHash,
	)
	if insertErr == nil {
		return nil
	}

	var existingHash string
	queryErr := r.db.QueryRowContext(ctx,
		"SELECT payload_hash FROM news_authorization_snapshots WHERE authorization_ref = ?",
		ref.AuthorizationRef,
	).Scan(&existingHash)
	if queryErr == nil {
		if subtle.ConstantTimeCompare([]byte(existingHash), []byte(payloadHash)) == 1 {
			return nil
		}
		return app.ErrAuthorizationSnapshotConflict
	}
	if errors.Is(queryErr, sql.ErrNoRows) {
		return insertErr
	}
	return errors.Join(insertErr, queryErr)
}

// GetAuthorizationSnapshot verifies storage integrity and the immutable binding
// shape before returning it. Current policy revocation/expiry must be checked
// separately by the content access path.
func (r *MySQLRepository) GetAuthorizationSnapshot(
	ctx context.Context,
	authorizationRef string,
) (domain.NewsAuthorizationBinding, error) {
	if r == nil || r.db == nil || ctx == nil || authorizationRef == "" {
		return domain.NewsAuthorizationBinding{}, app.ErrInvalidAuthorizationSnapshot
	}
	var payload []byte
	var storedHash string
	err := r.db.QueryRowContext(ctx,
		"SELECT payload_json, payload_hash FROM news_authorization_snapshots WHERE authorization_ref = ?",
		authorizationRef,
	).Scan(&payload, &storedHash)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.NewsAuthorizationBinding{}, app.ErrAuthorizationSnapshotNotFound
	}
	if err != nil {
		return domain.NewsAuthorizationBinding{}, err
	}
	sum := sha256.Sum256(payload)
	actualHash := hex.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(storedHash), []byte(actualHash)) != 1 {
		return domain.NewsAuthorizationBinding{}, app.ErrInvalidAuthorizationSnapshot
	}
	var binding domain.NewsAuthorizationBinding
	if err := json.Unmarshal(payload, &binding); err != nil ||
		domain.ValidateNewsAuthorizationBinding(binding) != nil ||
		binding.AuthorizationRef.AuthorizationRef != authorizationRef {
		return domain.NewsAuthorizationBinding{}, app.ErrInvalidAuthorizationSnapshot
	}
	return binding, nil
}

func authorizationExpiry(value domain.NullableDateTime) (any, error) {
	if !value.Present {
		return nil, app.ErrInvalidAuthorizationSnapshot
	}
	if value.Value == nil {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, *value.Value)
	if err != nil {
		return nil, app.ErrInvalidAuthorizationSnapshot
	}
	return parsed.UTC(), nil
}
