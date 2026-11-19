package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

const (
	tradingCalendarSnapshotSchemaVersion = "1.0.0"
	tradingCalendarSnapshotRefPrefix     = "data_calendar:sha256:"
)

var ErrTradingCalendarSnapshotNotFound = errors.New("trading-calendar snapshot not found")

type tradingCalendarSnapshotPayload struct {
	AdapterVersion string                     `json:"adapter_version"`
	EndDate        string                     `json:"end_date"`
	Items          []model.TradingCalendarDay `json:"items"`
	Market         string                     `json:"market"`
	QualityReasons []string                   `json:"quality_reasons"`
	QualityStatus  model.QualityStatus        `json:"quality_status"`
	SchemaVersion  string                     `json:"schema_version"`
	Source         string                     `json:"source"`
	StartDate      string                     `json:"start_date"`
	TotalDays      int                        `json:"total_days"`
	TradingDays    int                        `json:"trading_days"`
}

// SaveTradingCalendarSnapshot stores one immutable normalized calendar content version.
// Fetch/observation timestamps are metadata, not part of the calendar content identity.
func (s *Store) SaveTradingCalendarSnapshot(ctx context.Context, snapshot *model.TradingCalendarSnapshot, observedAt time.Time) (string, time.Time, error) {
	if s == nil || s.db == nil {
		return "", time.Time{}, nil
	}
	if ctx == nil {
		return "", time.Time{}, errors.New("trading-calendar snapshot context is required")
	}
	if snapshot == nil {
		return "", time.Time{}, errors.New("trading-calendar snapshot is nil")
	}
	if observedAt.IsZero() {
		return "", time.Time{}, errors.New("trading-calendar system observation time is required")
	}
	payload, canonical, fingerprint, err := buildTradingCalendarSnapshotPayload(snapshot)
	if err != nil {
		return "", time.Time{}, err
	}
	calendarRef := tradingCalendarSnapshotRefPrefix + fingerprint
	observedAt = observedAt.UTC().Truncate(time.Microsecond)
	fetchedAt := snapshot.FetchedAt.UTC().Truncate(time.Microsecond)

	_, err = s.db.ExecContext(ctx, `INSERT INTO market_data_trading_calendar_snapshots
(calendar_ref, content_fingerprint, market, start_date, end_date, source, adapter_version, schema_version, quality_status, fetched_at, observed_at, payload_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE calendar_ref=VALUES(calendar_ref)`,
		calendarRef, fingerprint, payload.Market, payload.StartDate, payload.EndDate,
		payload.Source, payload.AdapterVersion, payload.SchemaVersion, string(payload.QualityStatus),
		fetchedAt.Format("2006-01-02 15:04:05.999999"), observedAt.Format("2006-01-02 15:04:05.999999"), canonical)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("persist immutable trading-calendar snapshot: %w", err)
	}

	var storedFingerprint, storedPayload string
	var storedFetchedAt, storedObservedAt time.Time
	err = s.db.QueryRowContext(ctx, `SELECT content_fingerprint, payload_json, fetched_at, observed_at
FROM market_data_trading_calendar_snapshots WHERE calendar_ref=?`, calendarRef).
		Scan(&storedFingerprint, &storedPayload, &storedFetchedAt, &storedObservedAt)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("verify immutable trading-calendar snapshot: %w", err)
	}
	if storedFingerprint != fingerprint || !bytes.Equal([]byte(storedPayload), canonical) {
		return "", time.Time{}, fmt.Errorf("immutable trading-calendar snapshot %s conflicts with stored content", calendarRef)
	}
	snapshot.CalendarRef = calendarRef
	observed := storedObservedAt.UTC()
	snapshot.SystemObservedAt = &observed
	return calendarRef, observed, nil
}

// GetTradingCalendarSnapshot resolves an exact immutable calendar reference and
// verifies both the stored payload digest and the reference before returning it.
func (s *Store) GetTradingCalendarSnapshot(ctx context.Context, calendarRef string) (*model.TradingCalendarSnapshot, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("MariaDB trading-calendar snapshot storage disabled")
	}
	if ctx == nil {
		return nil, errors.New("trading-calendar snapshot context is required")
	}
	fingerprint, err := parseTradingCalendarSnapshotRef(calendarRef)
	if err != nil {
		return nil, err
	}
	var storedFingerprint, canonical string
	var fetchedAt, observedAt time.Time
	err = s.db.QueryRowContext(ctx, `SELECT content_fingerprint, payload_json, fetched_at, observed_at
FROM market_data_trading_calendar_snapshots WHERE calendar_ref=?`, calendarRef).
		Scan(&storedFingerprint, &canonical, &fetchedAt, &observedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrTradingCalendarSnapshotNotFound, calendarRef)
	}
	if err != nil {
		return nil, fmt.Errorf("read immutable trading-calendar snapshot: %w", err)
	}
	if storedFingerprint != fingerprint {
		return nil, fmt.Errorf("trading-calendar snapshot %s fingerprint column mismatch", calendarRef)
	}
	if fetchedAt.IsZero() || observedAt.IsZero() {
		return nil, fmt.Errorf("trading-calendar snapshot %s has missing observation metadata", calendarRef)
	}
	var payload tradingCalendarSnapshotPayload
	if err := json.Unmarshal([]byte(canonical), &payload); err != nil {
		return nil, fmt.Errorf("decode immutable trading-calendar snapshot %s: %w", calendarRef, err)
	}
	if payload.SchemaVersion != tradingCalendarSnapshotSchemaVersion {
		return nil, fmt.Errorf("unsupported trading-calendar snapshot schema version %q", payload.SchemaVersion)
	}
	snapshot := &model.TradingCalendarSnapshot{
		Market: payload.Market, StartDate: payload.StartDate, EndDate: payload.EndDate,
		TotalDays: payload.TotalDays, TradingDays: payload.TradingDays,
		Items: payload.Items, Source: payload.Source, SourceVersion: payload.AdapterVersion,
		FetchedAt: fetchedAt.UTC(), QualityStatus: payload.QualityStatus,
		QualityReasons: append([]string(nil), payload.QualityReasons...),
		CalendarRef:    calendarRef,
	}
	if err := validateTradingCalendarSnapshotPayload(snapshot); err != nil {
		return nil, fmt.Errorf("stored trading-calendar snapshot %s is invalid: %w", calendarRef, err)
	}
	_, rebuilt, rebuiltFingerprint, err := buildTradingCalendarSnapshotPayload(snapshot)
	if err != nil {
		return nil, err
	}
	if rebuiltFingerprint != fingerprint || !bytes.Equal(rebuilt, []byte(canonical)) {
		return nil, fmt.Errorf("trading-calendar snapshot %s content verification failed", calendarRef)
	}
	observed := observedAt.UTC()
	snapshot.SystemObservedAt = &observed
	return snapshot, nil
}

// GetTradingCalendarSnapshotAsOf 按精确市场和日期区间读取 asOf 时点已观察到的最新不可变日历版本。
// 只读取 Repository，不调用 Provider，也不使用更新的当前日历替代。
func (s *Store) GetTradingCalendarSnapshotAsOf(ctx context.Context, market, startDate, endDate string, asOf time.Time) (*model.TradingCalendarSnapshot, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("MariaDB trading-calendar snapshot storage disabled")
	}
	if ctx == nil {
		return nil, errors.New("trading-calendar snapshot context is required")
	}
	if strings.TrimSpace(market) == "" || strings.TrimSpace(startDate) == "" || strings.TrimSpace(endDate) == "" {
		return nil, errors.New("trading-calendar market and exact date range are required")
	}
	if asOf.IsZero() {
		return nil, errors.New("trading-calendar as_of timestamp is required")
	}
	asOfText := asOf.UTC().Format("2006-01-02 15:04:05.999999")
	var calendarRef string
	err := s.db.QueryRowContext(ctx, `SELECT calendar_ref
FROM market_data_trading_calendar_snapshots
WHERE market=? AND start_date=? AND end_date=? AND observed_at<=?
ORDER BY observed_at DESC, calendar_ref ASC
LIMIT 1`, market, startDate, endDate, asOfText).Scan(&calendarRef)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: no %s calendar for %s..%s was observed by %s", ErrTradingCalendarSnapshotNotFound, market, startDate, endDate, asOfText)
	}
	if err != nil {
		return nil, fmt.Errorf("select trading-calendar snapshot as of %s: %w", asOfText, err)
	}
	snapshot, err := s.GetTradingCalendarSnapshot(ctx, calendarRef)
	if err != nil {
		return nil, err
	}
	if snapshot.SystemObservedAt == nil || snapshot.SystemObservedAt.After(asOf.UTC()) || snapshot.FetchedAt.After(asOf.UTC()) {
		return nil, fmt.Errorf("trading-calendar snapshot %s is newer than requested as_of", calendarRef)
	}
	return snapshot, nil
}

func parseTradingCalendarSnapshotRef(calendarRef string) (string, error) {
	if !strings.HasPrefix(calendarRef, tradingCalendarSnapshotRefPrefix) {
		return "", fmt.Errorf("invalid trading-calendar snapshot reference %q", calendarRef)
	}
	fingerprint := strings.TrimPrefix(calendarRef, tradingCalendarSnapshotRefPrefix)
	if len(fingerprint) != 64 {
		return "", fmt.Errorf("invalid trading-calendar snapshot fingerprint length")
	}
	if _, err := hex.DecodeString(fingerprint); err != nil || strings.ToLower(fingerprint) != fingerprint {
		return "", fmt.Errorf("invalid trading-calendar snapshot fingerprint")
	}
	return fingerprint, nil
}

func buildTradingCalendarSnapshotPayload(snapshot *model.TradingCalendarSnapshot) (tradingCalendarSnapshotPayload, []byte, string, error) {
	if err := validateTradingCalendarSnapshotPayload(snapshot); err != nil {
		return tradingCalendarSnapshotPayload{}, nil, "", err
	}
	reasons := append([]string(nil), snapshot.QualityReasons...)
	sort.Strings(reasons)
	uniqueReasons := reasons[:0]
	for _, reason := range reasons {
		if reason == "" {
			return tradingCalendarSnapshotPayload{}, nil, "", errors.New("trading-calendar quality reason must not be empty")
		}
		if len(uniqueReasons) == 0 || uniqueReasons[len(uniqueReasons)-1] != reason {
			uniqueReasons = append(uniqueReasons, reason)
		}
	}
	items := append([]model.TradingCalendarDay(nil), snapshot.Items...)
	payload := tradingCalendarSnapshotPayload{
		AdapterVersion: strings.TrimSpace(snapshot.SourceVersion),
		EndDate:        snapshot.EndDate,
		Items:          items,
		Market:         snapshot.Market,
		QualityReasons: uniqueReasons,
		QualityStatus:  snapshot.QualityStatus,
		SchemaVersion:  tradingCalendarSnapshotSchemaVersion,
		Source:         strings.TrimSpace(snapshot.Source),
		StartDate:      snapshot.StartDate,
		TotalDays:      snapshot.TotalDays,
		TradingDays:    snapshot.TradingDays,
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return tradingCalendarSnapshotPayload{}, nil, "", fmt.Errorf("marshal canonical trading-calendar payload: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return payload, canonical, hex.EncodeToString(digest[:]), nil
}

func validateTradingCalendarSnapshotPayload(snapshot *model.TradingCalendarSnapshot) error {
	if snapshot == nil {
		return errors.New("trading-calendar snapshot is nil")
	}
	start, err := time.Parse("2006-01-02", snapshot.StartDate)
	if err != nil {
		return fmt.Errorf("invalid trading-calendar start date: %w", err)
	}
	end, err := time.Parse("2006-01-02", snapshot.EndDate)
	if err != nil {
		return fmt.Errorf("invalid trading-calendar end date: %w", err)
	}
	if snapshot.Market != "cn_a_share" || start.After(end) {
		return errors.New("trading-calendar market or date range is invalid")
	}
	totalDays := int(end.Sub(start)/(24*time.Hour)) + 1
	if snapshot.TotalDays != totalDays || len(snapshot.Items) != totalDays {
		return fmt.Errorf("trading-calendar date coverage mismatch: declared=%d items=%d expected=%d", snapshot.TotalDays, len(snapshot.Items), totalDays)
	}
	tradingDays := 0
	for index, item := range snapshot.Items {
		if item.Date != start.AddDate(0, 0, index).Format("2006-01-02") {
			return fmt.Errorf("trading-calendar item %d is missing, duplicate or unordered", index)
		}
		if item.IsTradingDay {
			tradingDays++
		}
	}
	if snapshot.TradingDays != tradingDays {
		return fmt.Errorf("trading-calendar trading-day count mismatch: declared=%d actual=%d", snapshot.TradingDays, tradingDays)
	}
	if strings.TrimSpace(snapshot.Source) == "" || strings.TrimSpace(snapshot.SourceVersion) == "" ||
		strings.EqualFold(strings.TrimSpace(snapshot.SourceVersion), "unknown") {
		return errors.New("trading-calendar source and adapter version are required")
	}
	if snapshot.QualityStatus != model.QualityValid && snapshot.QualityStatus != model.QualityDegraded {
		return fmt.Errorf("unsupported trading-calendar quality status %q", snapshot.QualityStatus)
	}
	return nil
}

// validateTradingCalendarSnapshotSchema verifies the immutable calendar table
// independently of migration checksums, including its primary and lookup indexes.
func (s *Store) validateTradingCalendarSnapshotSchema(ctx context.Context) error {
	table := "market_data_trading_calendar_snapshots"
	columns := []struct {
		name, columnType, dataType string
	}{
		{"calendar_ref", "varchar(96)", "varchar"},
		{"content_fingerprint", "char(64)", "char"},
		{"market", "varchar(32)", "varchar"},
		{"start_date", "date", "date"},
		{"end_date", "date", "date"},
		{"source", "varchar(64)", "varchar"},
		{"adapter_version", "varchar(128)", "varchar"},
		{"schema_version", "varchar(16)", "varchar"},
		{"quality_status", "varchar(16)", "varchar"},
		{"fetched_at", "datetime(6)", "datetime"},
		{"observed_at", "datetime(6)", "datetime"},
		{"payload_json", "longtext", "longtext"},
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
	primaryRows, err := s.db.QueryContext(ctx, `SELECT column_name FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ? AND index_name = 'PRIMARY' ORDER BY seq_in_index`, table)
	if err != nil {
		return fmt.Errorf("inspect %s primary key: %w", table, err)
	}
	var primaryColumns []string
	for primaryRows.Next() {
		var column string
		if err := primaryRows.Scan(&column); err != nil {
			_ = primaryRows.Close()
			return fmt.Errorf("read %s primary key: %w", table, err)
		}
		primaryColumns = append(primaryColumns, column)
	}
	if err := primaryRows.Err(); err != nil {
		_ = primaryRows.Close()
		return fmt.Errorf("read %s primary key: %w", table, err)
	}
	if err := primaryRows.Close(); err != nil {
		return fmt.Errorf("close %s primary key metadata: %w", table, err)
	}
	if strings.Join(primaryColumns, ",") != "calendar_ref" {
		return fmt.Errorf("%s primary key must be calendar_ref", table)
	}

	indexRows, err := s.db.QueryContext(ctx, `SELECT non_unique, column_name, index_type, collation, sub_part
FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ? AND index_name = 'idx_market_calendar_identity_observed'
ORDER BY seq_in_index`, table)
	if err != nil {
		return fmt.Errorf("inspect %s identity index: %w", table, err)
	}
	var indexColumns []string
	nonUnique := -1
	for indexRows.Next() {
		var currentNonUnique int
		var column, indexType string
		var collation sql.NullString
		var subPart sql.NullInt64
		if err := indexRows.Scan(&currentNonUnique, &column, &indexType, &collation, &subPart); err != nil {
			_ = indexRows.Close()
			return fmt.Errorf("read %s identity index: %w", table, err)
		}
		if nonUnique != -1 && nonUnique != currentNonUnique {
			_ = indexRows.Close()
			return fmt.Errorf("%s identity index has inconsistent uniqueness metadata", table)
		}
		if strings.ToUpper(indexType) != "BTREE" || !collation.Valid || collation.String != "A" || subPart.Valid {
			_ = indexRows.Close()
			return fmt.Errorf("%s identity index must use full ascending BTREE columns", table)
		}
		nonUnique = currentNonUnique
		indexColumns = append(indexColumns, column)
	}
	if err := indexRows.Err(); err != nil {
		_ = indexRows.Close()
		return fmt.Errorf("read %s identity index: %w", table, err)
	}
	if err := indexRows.Close(); err != nil {
		return fmt.Errorf("close %s identity index metadata: %w", table, err)
	}
	if nonUnique != 1 || strings.Join(indexColumns, ",") != "market,start_date,end_date,observed_at" {
		return fmt.Errorf("%s identity index must be non-unique on market,start_date,end_date,observed_at", table)
	}
	return nil
}
