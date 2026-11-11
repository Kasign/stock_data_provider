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
	if err != nil || len(payload) == 0 || len(payload) > maximumEvidenceSnapshotBytes {
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
	var existingHash string
	queryErr := r.db.QueryRowContext(ctx,
		"SELECT payload_hash FROM news_evidence_snapshots WHERE snapshot_ref = ?",
		snapshot.SnapshotRef,
	).Scan(&existingHash)
	if queryErr == nil {
		if subtle.ConstantTimeCompare([]byte(existingHash), []byte(payloadHash)) == 1 {
			return nil
		}
		return app.ErrEvidenceSnapshotConflict
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
	err := r.db.QueryRowContext(ctx,
		"SELECT payload_json, payload_hash, item_count FROM news_evidence_snapshots WHERE snapshot_ref = ? AND caller_ref = ?",
		snapshotRef, callerRef,
	).Scan(&payload, &storedHash, &itemCount)
	if errors.Is(err, sql.ErrNoRows) {
		return app.EvidenceSnapshot{}, app.ErrEvidenceSnapshotUnavailable
	}
	if err != nil {
		return app.EvidenceSnapshot{}, err
	}
	sum := sha256.Sum256(payload)
	actualHash := hex.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(storedHash), []byte(actualHash)) != 1 {
		return app.EvidenceSnapshot{}, app.ErrInvalidEvidenceSnapshot
	}
	var snapshot app.EvidenceSnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil ||
		snapshot.ValidateAt(now) != nil ||
		uint32(len(snapshot.Items)) != itemCount ||
		snapshot.SnapshotRef != snapshotRef ||
		snapshot.CallerRef != callerRef {
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
