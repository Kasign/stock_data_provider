package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

func validISODate(value string, optional bool) bool {
	if value == "" {
		return optional
	}
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func validAdjustmentFactorBasis(value model.AdjustmentFactorBasis) bool {
	return value == model.AdjustmentFactorShared || value == model.AdjustmentFactorForward || value == model.AdjustmentFactorBackward
}

func validObservedQuality(value model.QualityStatus) bool {
	return value == model.QualityValid || value == model.QualityDegraded || value == model.QualityUnknown
}

func validateAdjustmentFactorRecord(value model.AdjustmentFactor) error {
	if strings.TrimSpace(value.Symbol) == "" || !validISODate(value.TradeDate, false) ||
		value.Factor <= 0 || math.IsNaN(value.Factor) || math.IsInf(value.Factor, 0) ||
		!validAdjustmentFactorBasis(model.NormalizeAdjustmentFactorBasis(value.Basis)) ||
		strings.TrimSpace(value.Source) == "" || strings.TrimSpace(value.SourceVersion) == "" ||
		value.FetchedAt.IsZero() || !validObservedQuality(value.QualityStatus) {
		return errors.New("identity, factor, source/version, fetched_at or quality is invalid")
	}
	if value.ProviderTime != nil && value.ProviderTime.IsZero() {
		return errors.New("provider_time must be a non-zero timestamp when supplied")
	}
	if value.AvailableAt != nil && value.AvailableAt.IsZero() {
		return errors.New("available_at must be a non-zero timestamp when supplied")
	}
	return nil
}

func canonicalRevisionTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	canonical := value.UTC()
	return &canonical
}

func adjustmentFactorHash(value model.AdjustmentFactor) (string, error) {
	value.Basis = model.NormalizeAdjustmentFactorBasis(value.Basis)
	value.SystemObservedAt = nil
	value.FetchedAt = time.Time{}
	value.ProviderTime = canonicalRevisionTime(value.ProviderTime)
	value.AvailableAt = canonicalRevisionTime(value.AvailableAt)
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// SaveAdjustmentFactors 追加保存复权因子观测版本；同内容重复采集不重复写入。
func (s *Store) SaveAdjustmentFactors(ctx context.Context, values []model.AdjustmentFactor) error {
	if s == nil || s.db == nil {
		return errors.New("MariaDB adjustment-factor storage disabled")
	}
	if len(values) == 0 {
		return nil
	}

	type revision struct {
		value model.AdjustmentFactor
		hash  string
	}
	revisions := make([]revision, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := validateAdjustmentFactorRecord(value); err != nil {
			return fmt.Errorf("invalid adjustment factor observation for %s on %s: %w", value.Symbol, value.TradeDate, err)
		}
		value.Basis = model.NormalizeAdjustmentFactorBasis(value.Basis)
		identity := value.Symbol + "|" + value.TradeDate + "|" + value.Source + "|" + value.SourceVersion + "|" + string(value.Basis)
		if _, exists := seen[identity]; exists {
			return fmt.Errorf("duplicate adjustment-factor revision identity: %s", identity)
		}
		seen[identity] = struct{}{}
		hash, err := adjustmentFactorHash(value)
		if err != nil {
			return err
		}
		revisions = append(revisions, revision{value: value, hash: hash})
	}
	// 所有批次按相同顺序加锁，降低并发刷新相同证券时的死锁概率。
	sort.Slice(revisions, func(i, j int) bool {
		left, right := revisions[i].value, revisions[j].value
		leftKey := left.Symbol + "|" + left.TradeDate + "|" + left.Source + "|" + left.SourceVersion + "|" + string(left.Basis)
		rightKey := right.Symbol + "|" + right.TradeDate + "|" + right.Source + "|" + right.SourceVersion + "|" + string(right.Basis)
		return leftKey < rightKey
	})

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin adjustment-factor batch revision: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	for _, item := range revisions {
		value := item.value
		var latest string
		lookupErr := tx.QueryRowContext(ctx, `SELECT revision_hash FROM market_data_adjustment_factor_versions
WHERE symbol=? AND trade_date=? AND source=? AND source_version=? AND factor_basis=? ORDER BY recorded_at DESC,id DESC LIMIT 1 FOR UPDATE`,
			value.Symbol, value.TradeDate, value.Source, value.SourceVersion, string(value.Basis)).Scan(&latest)
		if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
			return fmt.Errorf("lock latest adjustment-factor revision for %s on %s: %w", value.Symbol, value.TradeDate, lookupErr)
		}
		if lookupErr != nil || latest != item.hash {
			stored := value
			stored.SystemObservedAt = nil
			metadata, marshalErr := json.Marshal(stored)
			if marshalErr != nil {
				return marshalErr
			}
			_, insertErr := tx.ExecContext(ctx, `INSERT INTO market_data_adjustment_factor_versions
(symbol,trade_date,source,source_version,factor_basis,revision_hash,recorded_at,fetched_at,available_at,quality_status,metadata)
VALUES (?,?,?,?,?,?,?,?,?,?,?)`, value.Symbol, value.TradeDate, value.Source, value.SourceVersion, string(value.Basis), item.hash,
				time.Now().UTC(), value.FetchedAt.UTC(), nullableTime(value.AvailableAt), string(value.QualityStatus), metadata)
			if insertErr != nil {
				return fmt.Errorf("insert adjustment-factor revision for %s on %s: %w", value.Symbol, value.TradeDate, insertErr)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit adjustment-factor batch revision: %w", err)
	}
	committed = true
	return nil
}

// ListAdjustmentFactorsAsOf 返回各交易日、各来源版本及因子口径在系统观察时点之前的最新版本。
func (s *Store) ListAdjustmentFactorsAsOf(ctx context.Context, symbol, startDate, endDate string, asOf time.Time) ([]model.AdjustmentFactor, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("MariaDB adjustment-factor storage disabled")
	}
	if strings.TrimSpace(symbol) == "" || !validISODate(startDate, true) || !validISODate(endDate, true) || asOf.IsZero() || asOf.After(time.Now().UTC()) {
		return nil, errors.New("invalid adjustment-factor as_of query")
	}
	asOfUTC := asOf.UTC()
	rows, err := s.db.QueryContext(ctx, `SELECT v.metadata, DATE_FORMAT(v.recorded_at,'%Y-%m-%d %H:%i:%s.%f')
FROM market_data_adjustment_factor_versions v
WHERE v.symbol=? AND (?='' OR v.trade_date>=?) AND (?='' OR v.trade_date<=?)
AND v.recorded_at<=? AND v.fetched_at<=? AND v.available_at IS NOT NULL AND v.available_at<=?
AND NOT EXISTS (
 SELECT 1 FROM market_data_adjustment_factor_versions n
 WHERE n.symbol=v.symbol AND n.trade_date=v.trade_date AND n.source=v.source
 AND n.source_version=v.source_version AND n.factor_basis=v.factor_basis
 AND n.recorded_at<=? AND n.fetched_at<=? AND n.available_at IS NOT NULL AND n.available_at<=?
 AND (n.recorded_at>v.recorded_at OR (n.recorded_at=v.recorded_at AND n.id>v.id))
)
ORDER BY v.trade_date ASC,v.source ASC,v.source_version ASC,v.factor_basis ASC,v.recorded_at DESC`,
		symbol, startDate, startDate, endDate, endDate, asOfUTC, asOfUTC, asOfUTC, asOfUTC, asOfUTC, asOfUTC)
	if err != nil {
		return nil, fmt.Errorf("query adjustment-factor revisions: %w", err)
	}
	defer rows.Close()
	out := []model.AdjustmentFactor{}
	for rows.Next() {
		var raw []byte
		var observed string
		if err := rows.Scan(&raw, &observed); err != nil {
			return nil, err
		}
		var value model.AdjustmentFactor
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, fmt.Errorf("decode adjustment factor: %w", err)
		}
		at, err := time.ParseInLocation("2006-01-02 15:04:05.000000", observed, time.UTC)
		if err != nil {
			return nil, err
		}
		value.SystemObservedAt = &at
		out = append(out, value)
	}
	return out, rows.Err()
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}

func nullableDate(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func corporateActionHash(value model.CorporateAction) (string, error) {
	value.SystemObservedAt = nil
	value.FetchedAt = time.Time{}
	value.AvailableAt = canonicalRevisionTime(value.AvailableAt)
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// SaveCorporateActions 按事件标识追加保存公司行动修订，不覆盖此前已观察版本。
func (s *Store) SaveCorporateActions(ctx context.Context, values []model.CorporateAction) error {
	if s == nil || s.db == nil {
		return errors.New("MariaDB corporate-action storage disabled")
	}
	if len(values) == 0 {
		return nil
	}

	type revision struct {
		value model.CorporateAction
		hash  string
	}
	revisions := make([]revision, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value.Symbol) == "" || strings.TrimSpace(value.ProviderEventID) == "" ||
			strings.TrimSpace(value.EventType) == "" || strings.TrimSpace(value.Source) == "" ||
			!validISODate(value.ExDate, true) || !validISODate(value.AnnouncementDate, true) ||
			(value.ExDate == "" && value.AnnouncementDate == "") || !validISODate(value.ImplementationAnnouncementDate, true) ||
			!validISODate(value.RecordDate, true) || !validISODate(value.PayDate, true) ||
			value.FetchedAt.IsZero() || !validObservedQuality(value.QualityStatus) {
			return fmt.Errorf("invalid corporate-action event %s for %s", value.ProviderEventID, value.Symbol)
		}
		for _, number := range []*float64{value.CashDividendCNY, value.CashDividendBeforeTaxCNY, value.BonusShareRatio, value.TransferShareRatio, value.RightsIssueRatio, value.RightsIssuePriceCNY} {
			if number != nil && (math.IsNaN(*number) || math.IsInf(*number, 0) || *number < 0) {
				return fmt.Errorf("corporate-action event %s has invalid numeric terms", value.ProviderEventID)
			}
		}
		identity := value.Symbol + "|" + value.ProviderEventID + "|" + value.Source
		if _, exists := seen[identity]; exists {
			return fmt.Errorf("duplicate corporate-action revision identity: %s", identity)
		}
		seen[identity] = struct{}{}
		hash, err := corporateActionHash(value)
		if err != nil {
			return err
		}
		revisions = append(revisions, revision{value: value, hash: hash})
	}
	// 批次按相同事件身份顺序加锁，降低并发刷新时的死锁概率。
	sort.Slice(revisions, func(i, j int) bool {
		left, right := revisions[i].value, revisions[j].value
		leftKey := left.Symbol + "|" + left.ProviderEventID + "|" + left.Source
		rightKey := right.Symbol + "|" + right.ProviderEventID + "|" + right.Source
		return leftKey < rightKey
	})

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin corporate-action batch revision: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	for _, item := range revisions {
		value := item.value
		var latest string
		lookupErr := tx.QueryRowContext(ctx, `SELECT revision_hash FROM market_data_corporate_action_versions
WHERE symbol=? AND provider_event_id=? AND source=? ORDER BY recorded_at DESC,id DESC LIMIT 1 FOR UPDATE`,
			value.Symbol, value.ProviderEventID, value.Source).Scan(&latest)
		if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
			return fmt.Errorf("lock latest corporate-action revision for %s: %w", value.ProviderEventID, lookupErr)
		}
		if lookupErr != nil || latest != item.hash {
			stored := value
			stored.SystemObservedAt = nil
			metadata, marshalErr := json.Marshal(stored)
			if marshalErr != nil {
				return marshalErr
			}
			_, insertErr := tx.ExecContext(ctx, `INSERT INTO market_data_corporate_action_versions
(symbol,provider_event_id,source,ex_date,announcement_date,revision_hash,recorded_at,fetched_at,available_at,quality_status,metadata)
VALUES (?,?,?,?,?,?,?,?,?,?,?)`, value.Symbol, value.ProviderEventID, value.Source, nullableDate(value.ExDate), nullableDate(value.AnnouncementDate), item.hash,
				time.Now().UTC(), value.FetchedAt.UTC(), nullableTime(value.AvailableAt), string(value.QualityStatus), metadata)
			if insertErr != nil {
				return fmt.Errorf("insert corporate-action revision for %s: %w", value.ProviderEventID, insertErr)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit corporate-action batch revision: %w", err)
	}
	committed = true
	return nil
}

// ListCorporateActionsAsOf 仅返回 as_of 时系统已经观察到的各事件最新修订。
func (s *Store) ListCorporateActionsAsOf(ctx context.Context, symbol, startDate, endDate string, asOf time.Time) ([]model.CorporateAction, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("MariaDB corporate-action storage disabled")
	}
	if strings.TrimSpace(symbol) == "" || !validISODate(startDate, true) || !validISODate(endDate, true) || asOf.IsZero() || asOf.After(time.Now().UTC()) {
		return nil, errors.New("invalid corporate-action as_of query")
	}
	asOfUTC := asOf.UTC()
	rows, err := s.db.QueryContext(ctx, `SELECT v.metadata,DATE_FORMAT(v.recorded_at,'%Y-%m-%d %H:%i:%s.%f')
FROM market_data_corporate_action_versions v
WHERE v.symbol=? AND (?='' OR COALESCE(v.ex_date,v.announcement_date)>=?) AND (?='' OR COALESCE(v.ex_date,v.announcement_date)<=?)
AND v.recorded_at<=? AND v.fetched_at<=? AND v.available_at IS NOT NULL AND v.available_at<=?
AND NOT EXISTS (
 SELECT 1 FROM market_data_corporate_action_versions n
 WHERE n.symbol=v.symbol AND n.provider_event_id=v.provider_event_id AND n.source=v.source
 AND n.recorded_at<=? AND n.fetched_at<=? AND n.available_at IS NOT NULL AND n.available_at<=?
 AND (n.recorded_at>v.recorded_at OR (n.recorded_at=v.recorded_at AND n.id>v.id))
)
ORDER BY COALESCE(v.ex_date,v.announcement_date) ASC,v.source ASC,v.recorded_at DESC`,
		symbol, startDate, startDate, endDate, endDate, asOfUTC, asOfUTC, asOfUTC, asOfUTC, asOfUTC, asOfUTC)
	if err != nil {
		return nil, fmt.Errorf("query corporate-action revisions: %w", err)
	}
	defer rows.Close()
	out := []model.CorporateAction{}
	for rows.Next() {
		var raw []byte
		var observed string
		if err := rows.Scan(&raw, &observed); err != nil {
			return nil, err
		}
		var value model.CorporateAction
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, fmt.Errorf("decode corporate action: %w", err)
		}
		at, err := time.ParseInLocation("2006-01-02 15:04:05.000000", observed, time.UTC)
		if err != nil {
			return nil, err
		}
		value.SystemObservedAt = &at
		out = append(out, value)
	}
	return out, rows.Err()
}
