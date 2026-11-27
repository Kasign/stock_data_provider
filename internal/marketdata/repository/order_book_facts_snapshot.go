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
	ErrOrderBookFactsSnapshotNotFound = errors.New("market-data order-book facts snapshot not found")
	ErrOrderBookFactsSnapshotConflict = errors.New("market-data order-book facts snapshot content conflict")
)

var _ snapshotport.OrderBookFactsSnapshotRepository = (*Store)(nil)

// SaveOrderBookFactsSnapshot 与前两类快照共用不可变存储表，通过 payload_kind 分流。
// 重复写入必须幂等；已存在记录内容不一致时必须冲突，不能覆盖。
func (s *Store) SaveOrderBookFactsSnapshot(ctx context.Context, snapshot app.OrderBookFactsSnapshot) error {
	if s == nil || s.db == nil {
		return errors.New("MariaDB order-book facts snapshot storage disabled")
	}
	if ctx == nil {
		return errors.New("order-book facts snapshot context is required")
	}
	manifestJSON, payloadJSON, err := canonicalOrderBookFactsSnapshotJSON(snapshot)
	if err != nil {
		return err
	}
	manifest := snapshot.Manifest
	_, err = s.db.ExecContext(ctx, `INSERT INTO market_data_daily_bars_close_series_snapshots
(snapshot_ref, payload_kind, manifest_fingerprint, payload_fingerprint, manifest_schema_version, payload_schema_version, manifest_json, payload_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, UTC_TIMESTAMP(6))
ON DUPLICATE KEY UPDATE snapshot_ref=VALUES(snapshot_ref)`,
		manifest.SnapshotRef, app.OrderBookFactsPayloadKind,
		manifest.ManifestFingerprint, snapshot.PayloadFingerprint,
		manifest.SchemaVersion, snapshot.Payload.SchemaVersion, string(manifestJSON), string(payloadJSON))
	if err != nil {
		return fmt.Errorf("persist immutable order-book facts snapshot: %w", err)
	}
	stored, err := s.GetOrderBookFactsSnapshot(ctx, manifest.SnapshotRef)
	if err != nil {
		return fmt.Errorf("verify persisted order-book facts snapshot: %w", err)
	}
	storedManifestJSON, storedPayloadJSON, err := canonicalOrderBookFactsSnapshotJSON(stored)
	if err != nil {
		return fmt.Errorf("verify stored order-book facts snapshot: %w", err)
	}
	if stored.Manifest.ManifestFingerprint != manifest.ManifestFingerprint ||
		stored.PayloadFingerprint != snapshot.PayloadFingerprint ||
		!bytes.Equal(storedManifestJSON, manifestJSON) ||
		!bytes.Equal(storedPayloadJSON, payloadJSON) {
		return fmt.Errorf("%w: %s", ErrOrderBookFactsSnapshotConflict, manifest.SnapshotRef)
	}
	return nil
}

// GetOrderBookFactsSnapshot 只按精确 SnapshotRef 与 order_book kind 读取，
// 读回后重新校验 Manifest、Payload 与组合指纹。
func (s *Store) GetOrderBookFactsSnapshot(ctx context.Context, snapshotRef string) (app.OrderBookFactsSnapshot, error) {
	if s == nil || s.db == nil {
		return app.OrderBookFactsSnapshot{}, errors.New("MariaDB order-book facts snapshot storage disabled")
	}
	if ctx == nil {
		return app.OrderBookFactsSnapshot{}, errors.New("order-book facts snapshot context is required")
	}
	if _, err := parseDataSnapshotRef(snapshotRef); err != nil {
		return app.OrderBookFactsSnapshot{}, err
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
		snapshotRef, app.OrderBookFactsPayloadKind).Scan(
		&manifestFingerprint, &payloadFingerprint, &manifestVersion, &payloadVersion, &manifestJSON, &payloadJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return app.OrderBookFactsSnapshot{}, fmt.Errorf("%w: %s", ErrOrderBookFactsSnapshotNotFound, snapshotRef)
	}
	if err != nil {
		return app.OrderBookFactsSnapshot{}, fmt.Errorf("read immutable order-book facts snapshot: %w", err)
	}
	var snapshot app.OrderBookFactsSnapshot
	if err := json.Unmarshal(manifestJSON, &snapshot.Manifest); err != nil {
		return app.OrderBookFactsSnapshot{}, fmt.Errorf("decode stored order-book facts manifest: %w", err)
	}
	if err := json.Unmarshal(payloadJSON, &snapshot.Payload); err != nil {
		return app.OrderBookFactsSnapshot{}, fmt.Errorf("decode stored order-book facts payload: %w", err)
	}
	snapshot.PayloadFingerprint = payloadFingerprint
	if snapshot.Manifest.SnapshotRef != snapshotRef ||
		snapshot.Manifest.ManifestFingerprint != manifestFingerprint ||
		snapshot.Manifest.SchemaVersion != manifestVersion ||
		snapshot.Payload.SchemaVersion != payloadVersion {
		return app.OrderBookFactsSnapshot{}, errors.New("stored order-book facts snapshot metadata does not match its JSON content")
	}
	if err := snapshot.Verify(); err != nil {
		return app.OrderBookFactsSnapshot{}, fmt.Errorf("stored order-book facts snapshot verification failed: %w", err)
	}
	canonicalManifest, canonicalPayload, err := canonicalOrderBookFactsSnapshotJSON(snapshot)
	if err != nil {
		return app.OrderBookFactsSnapshot{}, err
	}
	if !bytes.Equal(canonicalManifest, manifestJSON) || !bytes.Equal(canonicalPayload, payloadJSON) {
		return app.OrderBookFactsSnapshot{}, errors.New("stored order-book facts snapshot JSON is not canonical")
	}
	return snapshot, nil
}

func canonicalOrderBookFactsSnapshotJSON(snapshot app.OrderBookFactsSnapshot) ([]byte, []byte, error) {
	if err := snapshot.Verify(); err != nil {
		return nil, nil, fmt.Errorf("order-book facts snapshot is invalid: %w", err)
	}
	manifest, err := json.Marshal(snapshot.Manifest)
	if err != nil {
		return nil, nil, fmt.Errorf("encode order-book facts manifest: %w", err)
	}
	canonicalManifest, err := jcs.Transform(manifest)
	if err != nil {
		return nil, nil, fmt.Errorf("canonicalize order-book facts manifest: %w", err)
	}
	payload, err := json.Marshal(snapshot.Payload)
	if err != nil {
		return nil, nil, fmt.Errorf("encode order-book facts payload: %w", err)
	}
	canonicalPayload, err := jcs.Transform(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("canonicalize order-book facts payload: %w", err)
	}
	return canonicalManifest, canonicalPayload, nil
}
