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

// GetLatestLimitPoolFactsSnapshotAtOrBefore 取同一证券、交易日、用途下，
// system_as_of 不晚于 asOf 的最新涨跌停事实快照。
//
// system_as_of 存在 manifest_json 里且是变长 RFC3339Nano 字符串，SQL 侧按字符串
// 比较会出错（"…:05.5Z" 与 "…:05.05Z"），因此这里先按身份收窄候选，再在 Go 侧解析
// 时间取最大值。候选规模由 (symbol, trade_date, purpose) 决定，是单证券单日量级。
func (s *Store) GetLatestLimitPoolFactsSnapshotAtOrBefore(
	ctx context.Context, symbol, tradeDate, purpose string, asOf time.Time,
) (app.LimitPoolFactsSnapshot, error) {
	if s == nil || s.db == nil {
		return app.LimitPoolFactsSnapshot{}, errors.New("MariaDB limit-pool facts snapshot storage disabled")
	}
	if ctx == nil {
		return app.LimitPoolFactsSnapshot{}, errors.New("limit-pool facts snapshot context is required")
	}
	if strings.TrimSpace(symbol) == "" || strings.TrimSpace(tradeDate) == "" || strings.TrimSpace(purpose) == "" ||
		asOf.IsZero() || asOf.Location() != time.UTC {
		return app.LimitPoolFactsSnapshot{}, errors.New("limit-pool facts reuse requires canonical symbol, trade_date, purpose and UTC as-of instant")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT snapshot_ref, JSON_UNQUOTE(JSON_EXTRACT(manifest_json, '$.system_as_of'))
FROM market_data_daily_bars_close_series_snapshots
WHERE payload_kind=?
  AND JSON_UNQUOTE(JSON_EXTRACT(manifest_json, '$.symbol'))=?
  AND JSON_UNQUOTE(JSON_EXTRACT(manifest_json, '$.trade_date'))=?
  AND JSON_UNQUOTE(JSON_EXTRACT(manifest_json, '$.purpose_code'))=?`,
		app.LimitPoolFactsPayloadKind, symbol, tradeDate, purpose,
	)
	if err != nil {
		return app.LimitPoolFactsSnapshot{}, fmt.Errorf("query reusable limit-pool facts snapshot: %w", err)
	}
	defer rows.Close()

	var bestRef string
	var bestAsOf time.Time
	for rows.Next() {
		var snapshotRef, rawAsOf string
		if err := rows.Scan(&snapshotRef, &rawAsOf); err != nil {
			return app.LimitPoolFactsSnapshot{}, fmt.Errorf("scan reusable limit-pool facts snapshot: %w", err)
		}
		candidate, parseErr := time.Parse(time.RFC3339Nano, rawAsOf)
		if parseErr != nil {
			return app.LimitPoolFactsSnapshot{}, fmt.Errorf("reusable limit-pool facts snapshot %s has a non-canonical system_as_of %q", snapshotRef, rawAsOf)
		}
		if candidate.After(asOf) {
			continue
		}
		if bestRef == "" || candidate.After(bestAsOf) {
			bestRef, bestAsOf = snapshotRef, candidate
		}
	}
	if err := rows.Err(); err != nil {
		return app.LimitPoolFactsSnapshot{}, fmt.Errorf("iterate reusable limit-pool facts snapshots: %w", err)
	}
	if bestRef == "" {
		return app.LimitPoolFactsSnapshot{}, fmt.Errorf("%w: %s/%s/%s/%s",
			snapshotport.ErrLimitPoolFactsSnapshotNotFound, symbol, tradeDate, purpose, asOf.Format(time.RFC3339Nano))
	}
	return s.GetLimitPoolFactsSnapshot(ctx, bestRef)
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
