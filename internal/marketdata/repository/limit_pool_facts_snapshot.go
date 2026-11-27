package repository

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gowebpki/jcs"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/snapshotport"
)

// ErrLimitPoolFactsSnapshotConflict 表示同一定位引用的内容不一致或不可规范化。
var ErrLimitPoolFactsSnapshotConflict = errors.New("limit-pool facts snapshot conflict")

// SaveLimitPoolFactsSnapshot 写入不可变涨跌停事实快照，并读回校验指纹与规范字节。
// 它与其它数据类共用同一张快照表，由 payload_kind 分流。
func (s *Store) SaveLimitPoolFactsSnapshot(ctx context.Context, snapshot app.LimitPoolFactsSnapshot) error {
	if s == nil || s.db == nil {
		return errors.New("MariaDB limit-pool facts snapshot storage disabled")
	}
	if ctx == nil {
		return errors.New("limit-pool facts snapshot context is required")
	}
	manifestJSON, payloadJSON, err := canonicalLimitPoolFactsSnapshotJSON(snapshot)
	if err != nil {
		return err
	}
	manifest := snapshot.Manifest
	_, err = s.db.ExecContext(ctx, `INSERT INTO market_data_daily_bars_close_series_snapshots
(snapshot_ref, payload_kind, manifest_fingerprint, payload_fingerprint, manifest_schema_version, payload_schema_version, manifest_json, payload_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, UTC_TIMESTAMP(6))
ON DUPLICATE KEY UPDATE snapshot_ref=VALUES(snapshot_ref)`,
		manifest.SnapshotRef, app.LimitPoolFactsPayloadKind,
		manifest.ManifestFingerprint, snapshot.PayloadFingerprint,
		manifest.SchemaVersion, snapshot.Payload.SchemaVersion, string(manifestJSON), string(payloadJSON))
	if err != nil {
		return fmt.Errorf("persist immutable limit-pool facts snapshot: %w", err)
	}
	stored, err := s.GetLimitPoolFactsSnapshot(ctx, manifest.SnapshotRef)
	if err != nil {
		return fmt.Errorf("verify persisted limit-pool facts snapshot: %w", err)
	}
	storedManifestJSON, storedPayloadJSON, err := canonicalLimitPoolFactsSnapshotJSON(stored)
	if err != nil {
		return fmt.Errorf("verify stored limit-pool facts snapshot: %w", err)
	}
	if stored.Manifest.ManifestFingerprint != manifest.ManifestFingerprint ||
		stored.PayloadFingerprint != snapshot.PayloadFingerprint ||
		!bytes.Equal(storedManifestJSON, manifestJSON) ||
		!bytes.Equal(storedPayloadJSON, payloadJSON) {
		return fmt.Errorf("%w: %s", ErrLimitPoolFactsSnapshotConflict, manifest.SnapshotRef)
	}
	return nil
}

// GetLimitPoolFactsSnapshot 只按精确 SnapshotRef 与 limit_pool kind 读取，
// 读回后重新校验 Manifest、Payload 与组合指纹。
func (s *Store) GetLimitPoolFactsSnapshot(ctx context.Context, snapshotRef string) (app.LimitPoolFactsSnapshot, error) {
	if s == nil || s.db == nil {
		return app.LimitPoolFactsSnapshot{}, errors.New("MariaDB limit-pool facts snapshot storage disabled")
	}
	if ctx == nil {
		return app.LimitPoolFactsSnapshot{}, errors.New("limit-pool facts snapshot context is required")
	}
	if _, err := parseDataSnapshotRef(snapshotRef); err != nil {
		return app.LimitPoolFactsSnapshot{}, err
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
		snapshotRef, app.LimitPoolFactsPayloadKind).Scan(
		&manifestFingerprint, &payloadFingerprint, &manifestVersion, &payloadVersion, &manifestJSON, &payloadJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return app.LimitPoolFactsSnapshot{}, fmt.Errorf("%w: %s", snapshotport.ErrLimitPoolFactsSnapshotNotFound, snapshotRef)
	}
	if err != nil {
		return app.LimitPoolFactsSnapshot{}, fmt.Errorf("read immutable limit-pool facts snapshot: %w", err)
	}
	var snapshot app.LimitPoolFactsSnapshot
	if err := json.Unmarshal(manifestJSON, &snapshot.Manifest); err != nil {
		return app.LimitPoolFactsSnapshot{}, fmt.Errorf("decode stored limit-pool facts manifest: %w", err)
	}
	if err := json.Unmarshal(payloadJSON, &snapshot.Payload); err != nil {
		return app.LimitPoolFactsSnapshot{}, fmt.Errorf("decode stored limit-pool facts payload: %w", err)
	}
	snapshot.PayloadFingerprint = payloadFingerprint
	if snapshot.Manifest.SnapshotRef != snapshotRef ||
		snapshot.Manifest.ManifestFingerprint != manifestFingerprint ||
		snapshot.Manifest.SchemaVersion != manifestVersion ||
		snapshot.Payload.SchemaVersion != payloadVersion {
		return app.LimitPoolFactsSnapshot{}, errors.New("stored limit-pool facts snapshot metadata does not match its JSON content")
	}
	if err := snapshot.Verify(); err != nil {
		return app.LimitPoolFactsSnapshot{}, fmt.Errorf("stored limit-pool facts snapshot verification failed: %w", err)
	}
	canonicalManifest, canonicalPayload, err := canonicalLimitPoolFactsSnapshotJSON(snapshot)
	if err != nil {
		return app.LimitPoolFactsSnapshot{}, err
	}
	if !bytes.Equal(canonicalManifest, manifestJSON) || !bytes.Equal(canonicalPayload, payloadJSON) {
		return app.LimitPoolFactsSnapshot{}, errors.New("stored limit-pool facts snapshot JSON is not canonical")
	}
	return snapshot, nil
}

// GetLimitPoolFactsSnapshotForSystemAsOf 只按精确的证券、交易日、用途与评估截止时刻
// 复用已持久化的涨跌停事实；不使用时间范围或「最新一条」近似匹配。
func (s *Store) GetLimitPoolFactsSnapshotForSystemAsOf(
	ctx context.Context, symbol, tradeDate, purpose string, systemAsOf time.Time,
) (app.LimitPoolFactsSnapshot, error) {
	if s == nil || s.db == nil {
		return app.LimitPoolFactsSnapshot{}, errors.New("MariaDB limit-pool facts snapshot storage disabled")
	}
	if ctx == nil {
		return app.LimitPoolFactsSnapshot{}, errors.New("limit-pool facts snapshot context is required")
	}
	if strings.TrimSpace(symbol) == "" || strings.TrimSpace(tradeDate) == "" || strings.TrimSpace(purpose) == "" ||
		systemAsOf.IsZero() || systemAsOf.Location() != time.UTC {
		return app.LimitPoolFactsSnapshot{}, errors.New("limit-pool facts reuse requires canonical symbol, trade_date, purpose and UTC system_as_of")
	}
	var snapshotRef string
	err := s.db.QueryRowContext(ctx, `SELECT snapshot_ref
FROM market_data_daily_bars_close_series_snapshots
WHERE payload_kind=?
  AND JSON_UNQUOTE(JSON_EXTRACT(manifest_json, '$.symbol'))=?
  AND JSON_UNQUOTE(JSON_EXTRACT(manifest_json, '$.trade_date'))=?
  AND JSON_UNQUOTE(JSON_EXTRACT(manifest_json, '$.purpose_code'))=?
  AND JSON_UNQUOTE(JSON_EXTRACT(manifest_json, '$.system_as_of'))=?
ORDER BY created_at DESC LIMIT 1`,
		app.LimitPoolFactsPayloadKind, symbol, tradeDate, purpose, systemAsOf.Format(time.RFC3339Nano),
	).Scan(&snapshotRef)
	if errors.Is(err, sql.ErrNoRows) {
		return app.LimitPoolFactsSnapshot{}, fmt.Errorf("%w: %s/%s/%s/%s",
			snapshotport.ErrLimitPoolFactsSnapshotNotFound, symbol, tradeDate, purpose, systemAsOf.Format(time.RFC3339Nano))
	}
	if err != nil {
		return app.LimitPoolFactsSnapshot{}, fmt.Errorf("query reusable limit-pool facts snapshot: %w", err)
	}
	return s.GetLimitPoolFactsSnapshot(ctx, snapshotRef)
}

func canonicalLimitPoolFactsSnapshotJSON(snapshot app.LimitPoolFactsSnapshot) ([]byte, []byte, error) {
	if err := snapshot.Verify(); err != nil {
		return nil, nil, fmt.Errorf("limit-pool facts snapshot is invalid: %w", err)
	}
	manifest, err := json.Marshal(snapshot.Manifest)
	if err != nil {
		return nil, nil, fmt.Errorf("encode limit-pool facts manifest: %w", err)
	}
	canonicalManifest, err := jcs.Transform(manifest)
	if err != nil {
		return nil, nil, fmt.Errorf("canonicalize limit-pool facts manifest: %w", err)
	}
	payload, err := json.Marshal(snapshot.Payload)
	if err != nil {
		return nil, nil, fmt.Errorf("encode limit-pool facts payload: %w", err)
	}
	canonicalPayload, err := jcs.Transform(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("canonicalize limit-pool facts payload: %w", err)
	}
	return canonicalManifest, canonicalPayload, nil
}
