package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func securityTradingStatusRevisionHash(snapshot model.SecurityTradingStatusSnapshot) (string, error) {
	fingerprint := snapshot
	fingerprint.FetchedAt = time.Time{}
	fingerprint.SystemObservedAt = nil
	payload, err := json.Marshal(fingerprint)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// SaveSecurityTradingStatus appends a new content revision per source and security date.
// Repeated reads of the same content do not create new rows; a later correction does.
func (s *Store) SaveSecurityTradingStatus(ctx context.Context, snapshot *model.SecurityTradingStatusSnapshot) error {
	if s == nil || s.db == nil {
		return errors.New("MariaDB security-trading-status revision storage disabled")
	}
	if snapshot == nil {
		return errors.New("security-trading-status snapshot is nil")
	}
	if snapshot.Symbol == "" || snapshot.TradeDate == "" || snapshot.Source == "" ||
		snapshot.SourceVersion == "" || snapshot.FetchedAt.IsZero() {
		return errors.New("security-trading-status identity, source version or fetched_at is invalid")
	}
	if _, err := time.Parse("2006-01-02", snapshot.TradeDate); err != nil {
		return fmt.Errorf("security-trading-status trade_date is invalid: %w", err)
	}
	stored := *snapshot
	stored.SystemObservedAt = nil
	metadata, err := json.Marshal(&stored)
	if err != nil {
		return err
	}
	revisionHash, err := securityTradingStatusRevisionHash(stored)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin security-trading-status revision transaction: %w", err)
	}
	defer tx.Rollback()

	var latestHash string
	lookupErr := tx.QueryRowContext(ctx, `SELECT revision_hash FROM market_data_security_trading_status_versions
WHERE symbol=? AND trade_date=? AND source=?
ORDER BY recorded_at DESC, id DESC LIMIT 1 FOR UPDATE`,
		stored.Symbol, stored.TradeDate, stored.Source).Scan(&latestHash)
	if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
		return fmt.Errorf("read latest security-trading-status revision %s %s: %w", stored.Symbol, stored.TradeDate, lookupErr)
	}
	if shouldRecordRevision(latestHash, lookupErr == nil, revisionHash) {
		recordedAt := time.Now().UTC().Format("2006-01-02 15:04:05.999999")
		_, err = tx.ExecContext(ctx, `INSERT INTO market_data_security_trading_status_versions
(symbol,trade_date,source,revision_hash,recorded_at,fetched_at,quality_status,metadata)
VALUES (?,?,?,?,?,?,?,?)`,
			stored.Symbol, stored.TradeDate, stored.Source, revisionHash, recordedAt,
			stored.FetchedAt.UTC().Format("2006-01-02 15:04:05.999999"), string(stored.QualityStatus), metadata)
		if err != nil {
			return fmt.Errorf("record security-trading-status revision %s %s: %w", stored.Symbol, stored.TradeDate, err)
		}
	}
	return tx.Commit()
}

// ListSecurityTradingStatusAsOf returns each provider's latest recorded version for a security date
// up to the supplied system-observation timestamp. It never reads live Providers.
func (s *Store) ListSecurityTradingStatusAsOf(ctx context.Context, symbol, tradeDate string, asOf time.Time) ([]model.SecurityTradingStatusSnapshot, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("MariaDB security-trading-status revision storage disabled")
	}
	if asOf.IsZero() {
		return nil, errors.New("security-trading-status as_of timestamp must not be zero")
	}
	if _, err := time.Parse("2006-01-02", tradeDate); err != nil {
		return nil, fmt.Errorf("security-trading-status trade_date is invalid: %w", err)
	}
	asOfText := asOf.UTC().Format("2006-01-02 15:04:05.999999")
	rows, err := s.db.QueryContext(ctx, `SELECT v.metadata, DATE_FORMAT(v.recorded_at, '%Y-%m-%d %H:%i:%s.%f')
FROM market_data_security_trading_status_versions AS v
WHERE v.symbol=? AND v.trade_date=? AND v.recorded_at<=?
  AND NOT EXISTS (
    SELECT 1 FROM market_data_security_trading_status_versions AS newer
    WHERE newer.symbol=v.symbol AND newer.trade_date=v.trade_date AND newer.source=v.source
      AND newer.recorded_at<=?
      AND (newer.recorded_at>v.recorded_at OR (newer.recorded_at=v.recorded_at AND newer.id>v.id))
  )
ORDER BY v.recorded_at DESC, v.source ASC`, symbol, tradeDate, asOfText, asOfText)
	if err != nil {
		return nil, fmt.Errorf("query security-trading-status revisions as of %s: %w", asOfText, err)
	}
	defer rows.Close()
	out := []model.SecurityTradingStatusSnapshot{}
	for rows.Next() {
		var metadata []byte
		var recordedAtText string
		if err := rows.Scan(&metadata, &recordedAtText); err != nil {
			return nil, err
		}
		var snapshot model.SecurityTradingStatusSnapshot
		if err := json.Unmarshal(metadata, &snapshot); err != nil {
			continue
		}
		recordedAt, err := time.ParseInLocation("2006-01-02 15:04:05.000000", recordedAtText, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("parse security-trading-status record timestamp: %w", err)
		}
		snapshot.SystemObservedAt = &recordedAt
		out = append(out, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
