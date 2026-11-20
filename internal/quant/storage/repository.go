// Package storage persists immutable Quant result records.
package storage

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"example.com/stock-analysis-flow/internal/quant/result"
)

var (
	ErrInvalidRepository = errors.New("quant storage repository is invalid")
	ErrCorruptRecord     = errors.New("quant stored result record is corrupt")
	ErrStorageUnavailable = errors.New("quant result storage is unavailable")
)

const resultTable = "quant_results"

// MySQLRepository implements the Quant result write port.
// The database connection lifecycle is owned by Server/Platform.
type MySQLRepository struct {
	db *sql.DB
}

func NewMySQLRepository(db *sql.DB) (*MySQLRepository, error) {
	if db == nil {
		return nil, ErrInvalidRepository
	}
	return &MySQLRepository{db: db}, nil
}

var _ result.Repository = (*MySQLRepository)(nil)

// Save persists one immutable result. The primary key arbitrates concurrent
// writers; a duplicate is accepted only when the stored record is intact and
// both content-addressed fingerprints match. Existing bytes are never updated.
func (r *MySQLRepository) Save(ctx context.Context, record result.Record) error {
	if r == nil || r.db == nil || ctx == nil {
		return ErrInvalidRepository
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot, err := record.ValidatedSnapshot()
	if err != nil {
		return err
	}
	manifestJSON, err := json.Marshal(snapshot.Manifest)
	if err != nil {
		return fmt.Errorf("%w: encode manifest: %v", ErrInvalidRepository, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	_, insertErr := r.db.ExecContext(ctx, `INSERT INTO quant_results
		(result_id, manifest_fingerprint, result_fingerprint, manifest_json, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?, UTC_TIMESTAMP(6))`,
		snapshot.Manifest.ResultID,
		snapshot.Manifest.ManifestFingerprint,
		snapshot.Manifest.ResultFingerprint,
		manifestJSON,
		[]byte(snapshot.Payload),
	)
	if insertErr == nil {
		return nil
	}

	// A failed INSERT is not automatically an idempotent replay. Read the row
	// selected by the unique result_id and verify every persisted identity layer.
	var storedManifestFingerprint, storedResultFingerprint string
	var storedManifestJSON, storedPayload []byte
	queryErr := r.db.QueryRowContext(ctx, `SELECT manifest_fingerprint, result_fingerprint,
		manifest_json, payload_json FROM quant_results WHERE result_id = ?`,
		snapshot.Manifest.ResultID,
	).Scan(&storedManifestFingerprint, &storedResultFingerprint, &storedManifestJSON, &storedPayload)
	if errors.Is(queryErr, sql.ErrNoRows) {
		return errors.Join(ErrStorageUnavailable, insertErr)
	}
	if queryErr != nil {
		return errors.Join(ErrStorageUnavailable, insertErr, queryErr)
	}

	var storedManifest struct {
		ManifestFingerprint string `json:"manifest_fingerprint"`
		ResultFingerprint   string `json:"result_fingerprint"`
		ResultID            string `json:"result_id"`
	}
	if err := json.Unmarshal(storedManifestJSON, &storedManifest); err != nil ||
		storedManifest.ResultID != snapshot.Manifest.ResultID ||
		storedManifest.ManifestFingerprint != storedManifestFingerprint ||
		storedManifest.ResultFingerprint != storedResultFingerprint {
		return ErrCorruptRecord
	}
	var storedRecord result.Record
	if err := json.Unmarshal(storedManifestJSON, &storedRecord.Manifest); err != nil {
		return ErrCorruptRecord
	}
	storedRecord.Payload = append(json.RawMessage(nil), storedPayload...)
	if err := storedRecord.Validate(); err != nil {
		return errors.Join(ErrCorruptRecord, err)
	}
	if storedManifestFingerprint != snapshot.Manifest.ManifestFingerprint ||
		storedResultFingerprint != snapshot.Manifest.ResultFingerprint ||
		!bytes.Equal(storedRecord.Payload, snapshot.Payload) {
		// Canonical-equivalent JSON may differ bytewise. The fingerprints are
		// authoritative after both records independently pass Manifest.VerifyPayload.
		if storedManifestFingerprint == snapshot.Manifest.ManifestFingerprint &&
			storedResultFingerprint == snapshot.Manifest.ResultFingerprint {
			return nil
		}
		return result.ErrResultConflict
	}
	return nil
}
