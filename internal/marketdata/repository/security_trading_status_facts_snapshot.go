package repository

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/snapshotport"
	"github.com/gowebpki/jcs"
)

var (
	ErrSecurityTradingStatusFactsSnapshotNotFound = errors.New("market-data trading-status facts snapshot not found")
	ErrSecurityTradingStatusFactsSnapshotConflict = errors.New("market-data trading-status facts snapshot content conflict")
)

var _ snapshotport.SecurityTradingStatusFactsSnapshotRepository = (*Store)(nil)

// SaveSecurityTradingStatusFactsSnapshot 与 close-series 共用同一张不可变快照表与同一套主键，
// 通过 payload_kind 分流。重复写入保持幂等；已存在记录不会被覆盖，内容不一致时返回冲突。
func (s *Store) SaveSecurityTradingStatusFactsSnapshot(ctx context.Context, snapshot app.SecurityTradingStatusFactsSnapshot) error {
	if s == nil || s.db == nil {
		return errors.New("MariaDB trading-status facts snapshot storage disabled")
	}
	if ctx == nil {
		return errors.New("trading-status facts snapshot context is required")
	}
	manifestJSON, payloadJSON, err := canonicalSecurityTradingStatusFactsSnapshotJSON(snapshot)
	if err != nil {
		return err
	}
	manifest := snapshot.Manifest
	_, err = s.db.ExecContext(ctx, `INSERT INTO market_data_daily_bars_close_series_snapshots
(snapshot_ref, payload_kind, manifest_fingerprint, payload_fingerprint, manifest_schema_version, payload_schema_version, manifest_json, payload_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, UTC_TIMESTAMP(6))
ON DUPLICATE KEY UPDATE snapshot_ref=VALUES(snapshot_ref)`,
		manifest.SnapshotRef, app.SecurityTradingStatusFactsPayloadKind,
		manifest.ManifestFingerprint, snapshot.PayloadFingerprint,
		manifest.SchemaVersion, snapshot.Payload.SchemaVersion, string(manifestJSON), string(payloadJSON))
	if err != nil {
		return fmt.Errorf("persist immutable trading-status facts snapshot: %w", err)
	}

	stored, err := s.GetSecurityTradingStatusFactsSnapshot(ctx, manifest.SnapshotRef)
	if err != nil {
		return fmt.Errorf("verify persisted trading-status facts snapshot: %w", err)
	}
	storedManifestJSON, storedPayloadJSON, err := canonicalSecurityTradingStatusFactsSnapshotJSON(stored)
	if err != nil {
		return fmt.Errorf("verify stored trading-status facts snapshot: %w", err)
	}
	if stored.Manifest.ManifestFingerprint != manifest.ManifestFingerprint ||
		stored.PayloadFingerprint != snapshot.PayloadFingerprint ||
		!bytes.Equal(storedManifestJSON, manifestJSON) ||
		!bytes.Equal(storedPayloadJSON, payloadJSON) {
		return fmt.Errorf("%w: %s", ErrSecurityTradingStatusFactsSnapshotConflict, manifest.SnapshotRef)
	}
	return nil
}

// GetSecurityTradingStatusFactsSnapshot 只按精确 SnapshotRef 与 facts kind 读取，
// 并重新校验 Manifest、Payload 和组合指纹。
func (s *Store) GetSecurityTradingStatusFactsSnapshot(ctx context.Context, snapshotRef string) (app.SecurityTradingStatusFactsSnapshot, error) {
	if s == nil || s.db == nil {
		return app.SecurityTradingStatusFactsSnapshot{}, errors.New("MariaDB trading-status facts snapshot storage disabled")
	}
	if ctx == nil {
		return app.SecurityTradingStatusFactsSnapshot{}, errors.New("trading-status facts snapshot context is required")
	}
	if _, err := parseDataSnapshotRef(snapshotRef); err != nil {
		return app.SecurityTradingStatusFactsSnapshot{}, err
	}

	var (
		manifestFingerprint string
		payloadFingerprint  string
		manifestVersion     string
		payloadVersion      string
		manifestJSON        []byte
		payloadJSON         []byte
	)
	err := s.db.QueryRowContext(ctx, `SELECT manifest_fingerprint, payload_fingerprint,
manifest_schema_version, payload_schema_version, manifest_json, payload_json
FROM market_data_daily_bars_close_series_snapshots WHERE snapshot_ref=? AND payload_kind=?`,
		snapshotRef, app.SecurityTradingStatusFactsPayloadKind).Scan(
		&manifestFingerprint, &payloadFingerprint, &manifestVersion, &payloadVersion, &manifestJSON, &payloadJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return app.SecurityTradingStatusFactsSnapshot{}, fmt.Errorf("%w: %s", ErrSecurityTradingStatusFactsSnapshotNotFound, snapshotRef)
	}
	if err != nil {
		return app.SecurityTradingStatusFactsSnapshot{}, fmt.Errorf("read immutable trading-status facts snapshot: %w", err)
	}

	var snapshot app.SecurityTradingStatusFactsSnapshot
	if err := json.Unmarshal(manifestJSON, &snapshot.Manifest); err != nil {
		return app.SecurityTradingStatusFactsSnapshot{}, fmt.Errorf("decode stored trading-status facts manifest: %w", err)
	}
	if err := json.Unmarshal(payloadJSON, &snapshot.Payload); err != nil {
		return app.SecurityTradingStatusFactsSnapshot{}, fmt.Errorf("decode stored trading-status facts payload: %w", err)
	}
	snapshot.PayloadFingerprint = payloadFingerprint
	if snapshot.Manifest.SnapshotRef != snapshotRef ||
		snapshot.Manifest.ManifestFingerprint != manifestFingerprint ||
		snapshot.Manifest.SchemaVersion != manifestVersion ||
		snapshot.Payload.SchemaVersion != payloadVersion {
		return app.SecurityTradingStatusFactsSnapshot{}, errors.New("stored trading-status facts snapshot metadata does not match its JSON content")
	}
	if err := snapshot.Verify(); err != nil {
		return app.SecurityTradingStatusFactsSnapshot{}, fmt.Errorf("stored trading-status facts snapshot verification failed: %w", err)
	}
	canonicalManifest, canonicalPayload, err := canonicalSecurityTradingStatusFactsSnapshotJSON(snapshot)
	if err != nil {
		return app.SecurityTradingStatusFactsSnapshot{}, err
	}
	if !bytes.Equal(canonicalManifest, manifestJSON) || !bytes.Equal(canonicalPayload, payloadJSON) {
		return app.SecurityTradingStatusFactsSnapshot{}, errors.New("stored trading-status facts snapshot JSON is not canonical")
	}
	return snapshot, nil
}

func canonicalSecurityTradingStatusFactsSnapshotJSON(snapshot app.SecurityTradingStatusFactsSnapshot) ([]byte, []byte, error) {
	if err := snapshot.Verify(); err != nil {
		return nil, nil, fmt.Errorf("trading-status facts snapshot is invalid: %w", err)
	}
	manifest, err := json.Marshal(snapshot.Manifest)
	if err != nil {
		return nil, nil, fmt.Errorf("encode trading-status facts manifest: %w", err)
	}
	canonicalManifest, err := jcs.Transform(manifest)
	if err != nil {
		return nil, nil, fmt.Errorf("canonicalize trading-status facts manifest: %w", err)
	}
	payload, err := json.Marshal(snapshot.Payload)
	if err != nil {
		return nil, nil, fmt.Errorf("encode trading-status facts payload: %w", err)
	}
	canonicalPayload, err := jcs.Transform(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("canonicalize trading-status facts payload: %w", err)
	}
	return canonicalManifest, canonicalPayload, nil
}
