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
)

const maximumEvidenceSnapshotBytes = 8 * 1024 * 1024

var _ app.EvidenceSnapshotRepository = (*MySQLRepository)(nil)

func (r *MySQLRepository) CreateEvidenceSnapshot(ctx context.Context, snapshot app.EvidenceSnapshot) error {
	if r == nil || r.db == nil || ctx == nil || snapshot.ValidateAt(time.Now().UTC()) != nil {
		return app.ErrInvalidEvidenceSnapshot
	}
	payload, err := json.Marshal(snapshot)
	if err != nil || !validEvidenceSnapshotPayloadSize(payload) {
		return app.ErrInvalidEvidenceSnapshot
	}
	sum := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(sum[:])
	_, insertErr := r.db.ExecContext(ctx, `INSERT INTO news_evidence_snapshots
		(snapshot_ref, caller_ref, filters_fingerprint, page_size, system_as_of,
		 sort_version, created_at, expires_at, item_count, payload_json, payload_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snapshot.SnapshotRef, snapshot.CallerRef, snapshot.FiltersFingerprint,
		snapshot.PageSize, snapshot.SystemAsOf.UTC(), snapshot.SortVersion,
		snapshot.CreatedAt.UTC(), snapshot.ExpiresAt.UTC(), len(snapshot.Items), payload, payloadHash,
	)
	if insertErr == nil {
		return nil
	}
	var existingPayload []byte
	var existingHash, existingCallerRef, existingFingerprint, existingSortVersion string
	var existingPageSize uint16
	var existingItemCount uint32
	var existingSystemAsOf, existingCreatedAt, existingExpiresAt time.Time
	queryErr := r.db.QueryRowContext(ctx, `SELECT payload_json, payload_hash, caller_ref, filters_fingerprint, page_size,
		system_as_of, sort_version, created_at, expires_at, item_count
		FROM news_evidence_snapshots WHERE snapshot_ref = ?`,
		snapshot.SnapshotRef,
	).Scan(&existingPayload, &existingHash, &existingCallerRef, &existingFingerprint, &existingPageSize,
		&existingSystemAsOf, &existingSortVersion, &existingCreatedAt, &existingExpiresAt, &existingItemCount)
	if queryErr == nil {
		if !validEvidenceSnapshotPayloadSize(existingPayload) {
			return app.ErrInvalidEvidenceSnapshot
		}
		existingSum := sha256.Sum256(existingPayload)
		existingActualHash := hex.EncodeToString(existingSum[:])
		if subtle.ConstantTimeCompare([]byte(existingHash), []byte(existingActualHash)) != 1 {
			return app.ErrInvalidEvidenceSnapshot
		}
		if subtle.ConstantTimeCompare([]byte(existingHash), []byte(payloadHash)) != 1 {
			return app.ErrEvidenceSnapshotConflict
		}
		// A matching payload hash alone is insufficient for an idempotent replay:
		// the denormalized lookup envelope must still describe that exact payload.
		if existingCallerRef != snapshot.CallerRef ||
			existingFingerprint != snapshot.FiltersFingerprint ||
			existingPageSize != uint16(snapshot.PageSize) ||
			existingSortVersion != snapshot.SortVersion ||
			existingItemCount != uint32(len(snapshot.Items)) ||
			!sameEvidenceSnapshotDBTime(existingSystemAsOf, snapshot.SystemAsOf) ||
			!sameEvidenceSnapshotDBTime(existingCreatedAt, snapshot.CreatedAt) ||
			!sameEvidenceSnapshotDBTime(existingExpiresAt, snapshot.ExpiresAt) {
			return app.ErrInvalidEvidenceSnapshot
		}
		return nil
	}
	if errors.Is(queryErr, sql.ErrNoRows) {
		return insertErr
	}
	return errors.Join(insertErr, queryErr)
}

func (r *MySQLRepository) GetEvidenceSnapshot(
	ctx context.Context,
	snapshotRef string,
	callerRef string,
	now time.Time,
) (app.EvidenceSnapshot, error) {
	if r == nil || r.db == nil || ctx == nil ||
		snapshotRef == "" || callerRef == "" || now.IsZero() {
		return app.EvidenceSnapshot{}, app.ErrInvalidEvidenceSnapshot
	}
	var payload []byte
	var storedHash string
	var itemCount uint32
	var storedFingerprint string
	var storedPageSize uint16
	var storedSystemAsOf, storedCreatedAt, storedExpiresAt time.Time
	var storedSortVersion string
	err := r.db.QueryRowContext(ctx, `SELECT payload_json, payload_hash, item_count,
		filters_fingerprint, page_size, system_as_of, sort_version, created_at, expires_at
		FROM news_evidence_snapshots WHERE snapshot_ref = ? AND caller_ref = ?`,
		snapshotRef, callerRef,
	).Scan(&payload, &storedHash, &itemCount, &storedFingerprint, &storedPageSize,
		&storedSystemAsOf, &storedSortVersion, &storedCreatedAt, &storedExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return app.EvidenceSnapshot{}, app.ErrEvidenceSnapshotUnavailable
	}
	if err != nil {
		return app.EvidenceSnapshot{}, err
	}
	if !validEvidenceSnapshotPayloadSize(payload) {
		return app.EvidenceSnapshot{}, app.ErrInvalidEvidenceSnapshot
	}
	sum := sha256.Sum256(payload)
	actualHash := hex.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(storedHash), []byte(actualHash)) != 1 {
		return app.EvidenceSnapshot{}, app.ErrInvalidEvidenceSnapshot
	}
	var snapshot app.EvidenceSnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil ||
		snapshot.Validate() != nil || snapshot.CreatedAt.After(now) || snapshot.SystemAsOf.After(now) ||
		uint32(len(snapshot.Items)) != itemCount ||
		snapshot.SnapshotRef != snapshotRef ||
		snapshot.CallerRef != callerRef ||
		snapshot.FiltersFingerprint != storedFingerprint ||
		snapshot.PageSize != int(storedPageSize) ||
		!sameEvidenceSnapshotDBTime(snapshot.SystemAsOf, storedSystemAsOf) ||
		snapshot.SortVersion != storedSortVersion ||
		!sameEvidenceSnapshotDBTime(snapshot.CreatedAt, storedCreatedAt) ||
		!sameEvidenceSnapshotDBTime(snapshot.ExpiresAt, storedExpiresAt) {
		return app.EvidenceSnapshot{}, app.ErrInvalidEvidenceSnapshot
	}
	if !now.Before(snapshot.ExpiresAt) {
		return app.EvidenceSnapshot{}, app.ErrEvidenceSnapshotUnavailable
	}
	return snapshot, nil
}

func (r *MySQLRepository) DeleteExpiredEvidenceSnapshots(
	ctx context.Context,
	now time.Time,
	limit int,
) (int64, error) {
	if r == nil || r.db == nil || ctx == nil || now.IsZero() ||
		limit < 1 || limit > 1000 {
		return 0, app.ErrInvalidEvidenceSnapshot
	}
	result, err := r.db.ExecContext(ctx, `DELETE FROM news_evidence_snapshots
		WHERE expires_at <= ? ORDER BY expires_at ASC LIMIT ?`, now.UTC(), limit)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// MariaDB DATETIME(6) stores microsecond precision; compare the payload and
// indexed metadata at the precision the schema can actually preserve.
func sameEvidenceSnapshotDBTime(payloadTime, storedTime time.Time) bool {
	return payloadTime.UTC().Truncate(time.Microsecond).Equal(storedTime.UTC().Truncate(time.Microsecond))
}

func validEvidenceSnapshotPayloadSize(payload []byte) bool {
	return len(payload) > 0 && len(payload) <= maximumEvidenceSnapshotBytes
}
