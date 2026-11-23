package repository

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"github.com/gowebpki/jcs"
)

const dailyBarsCloseSeriesSnapshotRefPrefix = "data_snapshot:sha256:"

var (
	ErrDailyBarsCloseSeriesSnapshotNotFound = errors.New("market-data close-series snapshot not found")
	ErrDailyBarsCloseSeriesSnapshotConflict = errors.New("market-data close-series snapshot content conflict")
)

var _ app.DailyBarsCloseSeriesSnapshotRepository = (*Store)(nil)

// SaveDailyBarsCloseSeriesSnapshot 以 SnapshotRef 为主键保存完整查询结果。
// 重复写入保持幂等；已存在记录不会被覆盖，内容不一致时返回冲突。
func (s *Store) SaveDailyBarsCloseSeriesSnapshot(ctx context.Context, snapshot app.DailyBarsCloseSeriesSnapshot) error {
	if s == nil || s.db == nil {
		return errors.New("MariaDB daily-bars close-series snapshot storage disabled")
	}
	if ctx == nil {
		return errors.New("daily-bars close-series snapshot context is required")
	}
	manifestJSON, payloadJSON, err := canonicalDailyBarsCloseSeriesSnapshotJSON(snapshot)
	if err != nil {
		return err
	}
	manifest := snapshot.Manifest
	_, err = s.db.ExecContext(ctx, `INSERT INTO market_data_daily_bars_close_series_snapshots
(snapshot_ref, manifest_fingerprint, payload_fingerprint, manifest_schema_version, payload_schema_version, manifest_json, payload_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, UTC_TIMESTAMP(6))
ON DUPLICATE KEY UPDATE snapshot_ref=VALUES(snapshot_ref)`,
		manifest.SnapshotRef, manifest.ManifestFingerprint, snapshot.PayloadFingerprint,
		manifest.SchemaVersion, snapshot.Payload.SchemaVersion, string(manifestJSON), string(payloadJSON))
	if err != nil {
		return fmt.Errorf("persist immutable daily-bars close-series snapshot: %w", err)
	}

	stored, err := s.GetDailyBarsCloseSeriesSnapshot(ctx, manifest.SnapshotRef)
	if err != nil {
		return fmt.Errorf("verify persisted daily-bars close-series snapshot: %w", err)
	}
	storedManifestJSON, storedPayloadJSON, err := canonicalDailyBarsCloseSeriesSnapshotJSON(stored)
	if err != nil {
		return fmt.Errorf("verify stored daily-bars close-series snapshot: %w", err)
	}
	if stored.Manifest.ManifestFingerprint != manifest.ManifestFingerprint ||
		stored.PayloadFingerprint != snapshot.PayloadFingerprint ||
		!bytes.Equal(storedManifestJSON, manifestJSON) ||
		!bytes.Equal(storedPayloadJSON, payloadJSON) {
		return fmt.Errorf("%w: %s", ErrDailyBarsCloseSeriesSnapshotConflict, manifest.SnapshotRef)
	}
	return nil
}

// GetDailyBarsCloseSeriesSnapshot 只按精确 SnapshotRef 读取，并重新校验 Manifest、Payload 和组合指纹。
func (s *Store) GetDailyBarsCloseSeriesSnapshot(ctx context.Context, snapshotRef string) (app.DailyBarsCloseSeriesSnapshot, error) {
	if s == nil || s.db == nil {
		return app.DailyBarsCloseSeriesSnapshot{}, errors.New("MariaDB daily-bars close-series snapshot storage disabled")
	}
	if ctx == nil {
		return app.DailyBarsCloseSeriesSnapshot{}, errors.New("daily-bars close-series snapshot context is required")
	}
	if _, err := parseDailyBarsCloseSeriesSnapshotRef(snapshotRef); err != nil {
		return app.DailyBarsCloseSeriesSnapshot{}, err
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
FROM market_data_daily_bars_close_series_snapshots WHERE snapshot_ref=?`, snapshotRef).Scan(
		&manifestFingerprint, &payloadFingerprint, &manifestVersion, &payloadVersion, &manifestJSON, &payloadJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return app.DailyBarsCloseSeriesSnapshot{}, fmt.Errorf("%w: %s", ErrDailyBarsCloseSeriesSnapshotNotFound, snapshotRef)
	}
	if err != nil {
		return app.DailyBarsCloseSeriesSnapshot{}, fmt.Errorf("read immutable daily-bars close-series snapshot: %w", err)
	}

	var snapshot app.DailyBarsCloseSeriesSnapshot
	if err := json.Unmarshal(manifestJSON, &snapshot.Manifest); err != nil {
		return app.DailyBarsCloseSeriesSnapshot{}, fmt.Errorf("decode stored daily-bars close-series manifest: %w", err)
	}
	if err := json.Unmarshal(payloadJSON, &snapshot.Payload); err != nil {
		return app.DailyBarsCloseSeriesSnapshot{}, fmt.Errorf("decode stored daily-bars close-series payload: %w", err)
	}
	snapshot.PayloadFingerprint = payloadFingerprint
	if snapshot.Manifest.SnapshotRef != snapshotRef ||
		snapshot.Manifest.ManifestFingerprint != manifestFingerprint ||
		snapshot.Manifest.SchemaVersion != manifestVersion ||
		snapshot.Payload.SchemaVersion != payloadVersion {
		return app.DailyBarsCloseSeriesSnapshot{}, errors.New("stored daily-bars close-series snapshot metadata does not match its JSON content")
	}
	if err := snapshot.Verify(); err != nil {
		return app.DailyBarsCloseSeriesSnapshot{}, fmt.Errorf("stored daily-bars close-series snapshot verification failed: %w", err)
	}
	canonicalManifest, canonicalPayload, err := canonicalDailyBarsCloseSeriesSnapshotJSON(snapshot)
	if err != nil {
		return app.DailyBarsCloseSeriesSnapshot{}, err
	}
	if !bytes.Equal(canonicalManifest, manifestJSON) || !bytes.Equal(canonicalPayload, payloadJSON) {
		return app.DailyBarsCloseSeriesSnapshot{}, errors.New("stored daily-bars close-series snapshot JSON is not canonical")
	}
	return snapshot, nil
}

func canonicalDailyBarsCloseSeriesSnapshotJSON(snapshot app.DailyBarsCloseSeriesSnapshot) ([]byte, []byte, error) {
	if err := snapshot.Verify(); err != nil {
		return nil, nil, fmt.Errorf("daily-bars close-series snapshot is invalid: %w", err)
	}
	manifest, err := json.Marshal(snapshot.Manifest)
	if err != nil {
		return nil, nil, fmt.Errorf("encode daily-bars close-series manifest: %w", err)
	}
	canonicalManifest, err := jcs.Transform(manifest)
	if err != nil {
		return nil, nil, fmt.Errorf("canonicalize daily-bars close-series manifest: %w", err)
	}
	payload, err := json.Marshal(snapshot.Payload)
	if err != nil {
		return nil, nil, fmt.Errorf("encode daily-bars close-series payload: %w", err)
	}
	canonicalPayload, err := jcs.Transform(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("canonicalize daily-bars close-series payload: %w", err)
	}
	return canonicalManifest, canonicalPayload, nil
}

func parseDailyBarsCloseSeriesSnapshotRef(value string) (string, error) {
	if !strings.HasPrefix(value, dailyBarsCloseSeriesSnapshotRefPrefix) {
		return "", errors.New("daily-bars close-series snapshot_ref is malformed")
	}
	digest := strings.TrimPrefix(value, dailyBarsCloseSeriesSnapshotRefPrefix)
	if len(digest) != 64 {
		return "", errors.New("daily-bars close-series snapshot_ref is malformed")
	}
	decoded, err := hex.DecodeString(digest)
	if err != nil || len(decoded) != 32 || strings.ToLower(digest) != digest {
		return "", errors.New("daily-bars close-series snapshot_ref is malformed")
	}
	return digest, nil
}

func (s *Store) validateDailyBarsCloseSeriesSnapshotSchema(ctx context.Context) error {
	table := "market_data_daily_bars_close_series_snapshots"
	columns := []struct {
		name, columnType, dataType string
	}{
		{"snapshot_ref", "varchar(96)", "varchar"},
		{"manifest_fingerprint", "char(64)", "char"},
		{"payload_fingerprint", "char(64)", "char"},
		{"manifest_schema_version", "varchar(16)", "varchar"},
		{"payload_schema_version", "varchar(16)", "varchar"},
		{"manifest_json", "longtext", "longtext"},
		{"payload_json", "longtext", "longtext"},
		{"created_at", "datetime(6)", "datetime"},
	}
	for _, column := range columns {
		var columnType, dataType, nullable string
		err := s.db.QueryRowContext(ctx, `SELECT column_type, data_type, is_nullable
FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`, table, column.name).
			Scan(&columnType, &dataType, &nullable)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%s.%s is missing", table, column.name)
		}
		if err != nil {
			return fmt.Errorf("inspect %s.%s: %w", table, column.name, err)
		}
		if strings.ToLower(columnType) != column.columnType || strings.ToLower(dataType) != column.dataType || nullable != "NO" {
			return fmt.Errorf("%s.%s must be NOT NULL %s", table, column.name, column.columnType)
		}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT column_name FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ? AND index_name = 'PRIMARY' ORDER BY seq_in_index`, table)
	if err != nil {
		return fmt.Errorf("inspect %s primary key: %w", table, err)
	}
	var primaryColumns []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read %s primary key: %w", table, err)
		}
		primaryColumns = append(primaryColumns, column)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("read %s primary key: %w", table, err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close %s primary key metadata: %w", table, err)
	}
	if strings.Join(primaryColumns, ",") != "snapshot_ref" {
		return fmt.Errorf("%s primary key must be snapshot_ref", table)
	}
	return nil
}
