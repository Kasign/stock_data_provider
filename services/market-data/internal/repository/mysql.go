package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	_ "github.com/go-sql-driver/mysql"
)

type Store struct{ db *sql.DB }

func OpenMySQL(ctx context.Context, dsn string) (*Store, error) {
	if dsn == "" {
		return nil, nil
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open MariaDB: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(3)
	db.SetConnMaxLifetime(5 * time.Minute)
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping MariaDB: %w", err)
	}
	return &Store{db: db}, nil
}
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.PingContext(ctx)
}
func (s *Store) Migrate(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS market_data_quotes (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
 symbol VARCHAR(16) NOT NULL,
 quote_time DATETIME(6) NULL,
 fetched_at DATETIME(6) NOT NULL,
 normalized_at DATETIME(6) NOT NULL,
 source VARCHAR(64) NOT NULL,
 current_price DECIMAL(20,8) NULL,
 change_value DECIMAL(20,8) NULL,
 change_percent DECIMAL(16,8) NULL,
 open_price DECIMAL(20,8) NULL,
 high_price DECIMAL(20,8) NULL,
 low_price DECIMAL(20,8) NULL,
 prev_close DECIMAL(20,8) NULL,
 volume_shares BIGINT NULL,
 amount_cny DECIMAL(24,2) NULL,
 volume_ratio DECIMAL(16,8) NULL,
 turnover_rate DECIMAL(16,8) NULL,
 quality_status VARCHAR(16) NOT NULL,
 metadata JSON NOT NULL,
 PRIMARY KEY (id), KEY idx_market_quote_symbol_time (symbol, quote_time), KEY idx_market_quote_fetched (fetched_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS market_data_daily_bars (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
 symbol VARCHAR(16) NOT NULL,
 trade_date DATE NOT NULL,
 period VARCHAR(8) NOT NULL DEFAULT '1d',
 adjustment_basis VARCHAR(24) NOT NULL,
 source VARCHAR(64) NOT NULL,
 open_price DECIMAL(20,8) NOT NULL,
 high_price DECIMAL(20,8) NOT NULL,
 low_price DECIMAL(20,8) NOT NULL,
 close_price DECIMAL(20,8) NOT NULL,
 volume_shares BIGINT NULL,
 amount_cny DECIMAL(24,2) NULL,
 change_percent DECIMAL(16,8) NULL,
 fetched_at DATETIME(6) NOT NULL,
 normalized_at DATETIME(6) NOT NULL,
 quality_status VARCHAR(16) NOT NULL,
 metadata JSON NOT NULL,
 PRIMARY KEY (id), UNIQUE KEY uq_market_bar_source_basis (symbol, trade_date, period, adjustment_basis, source), KEY idx_market_bar_lookup (symbol, trade_date, adjustment_basis)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("market-data schema migration: %w", err)
		}
	}
	// CREATE TABLE IF NOT EXISTS does not add new columns to an already existing
	// deployment. Use information_schema checks so the migration works on both
	// MySQL and MariaDB versions without relying on vendor-specific
	// `ADD COLUMN IF NOT EXISTS` syntax.
	for _, column := range []struct{ name, definition string }{
		{name: "volume_ratio", definition: "DECIMAL(16,8) NULL"},
		{name: "turnover_rate", definition: "DECIMAL(16,8) NULL"},
	} {
		if err := s.ensureColumn(ctx, "market_data_quotes", column.name, column.definition); err != nil {
			return fmt.Errorf("migrate market_data_quotes.%s: %w", column.name, err)
		}
	}
	return nil
}

func (s *Store) ensureColumn(ctx context.Context, table, column, definition string) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND COLUMN_NAME=?`, table, column).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	// table/column/definition are internal constants supplied by this package,
	// never user input.
	_, err := s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `%s` %s", table, column, definition))
	return err
}
func (s *Store) SaveQuote(ctx context.Context, q *model.Quote) error {
	if s == nil || s.db == nil {
		return nil
	}
	meta, err := json.Marshal(q)
	if err != nil {
		return err
	}
	var quoteTime any
	if q.QuoteTime != nil {
		quoteTime = q.QuoteTime.In(time.UTC).Format("2006-01-02 15:04:05.999999")
	}
	return exec(ctx, s.db, `INSERT INTO market_data_quotes (symbol,quote_time,fetched_at,normalized_at,source,current_price,change_value,change_percent,open_price,high_price,low_price,prev_close,volume_shares,amount_cny,volume_ratio,turnover_rate,quality_status,metadata) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, q.Symbol, quoteTime, q.FetchedAt.UTC().Format("2006-01-02 15:04:05.999999"), q.NormalizedAt.UTC().Format("2006-01-02 15:04:05.999999"), q.Source, q.CurrentPrice, q.Change, q.ChangePercent, q.Open, q.High, q.Low, q.PrevClose, q.Volume, q.Amount, q.VolumeRatio, q.TurnoverRate, string(q.QualityStatus), meta)
}

// PurgeQuotesBefore removes old real-time audit snapshots in bounded batches.
// Daily bars are historical source data and are deliberately not affected.
func (s *Store) PurgeQuotesBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	const batchSize = 5000
	var total int64
	for {
		result, err := s.db.ExecContext(ctx, `DELETE FROM market_data_quotes WHERE fetched_at < ? ORDER BY fetched_at ASC LIMIT 5000`, cutoff.UTC().Format("2006-01-02 15:04:05.999999"))
		if err != nil {
			return total, fmt.Errorf("purge old quote snapshots: %w", err)
		}
		deleted, err := result.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("read quote purge row count: %w", err)
		}
		total += deleted
		if deleted < batchSize {
			return total, nil
		}
		if err := ctx.Err(); err != nil {
			return total, err
		}
	}
}

func (s *Store) UpsertBars(ctx context.Context, bars []model.DailyBar) error {
	if s == nil || s.db == nil {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt := `INSERT INTO market_data_daily_bars (symbol,trade_date,period,adjustment_basis,source,open_price,high_price,low_price,close_price,volume_shares,amount_cny,change_percent,fetched_at,normalized_at,quality_status,metadata) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE open_price=VALUES(open_price),high_price=VALUES(high_price),low_price=VALUES(low_price),close_price=VALUES(close_price),volume_shares=VALUES(volume_shares),amount_cny=VALUES(amount_cny),change_percent=VALUES(change_percent),fetched_at=VALUES(fetched_at),normalized_at=VALUES(normalized_at),quality_status=VALUES(quality_status),metadata=VALUES(metadata)`
	for _, bar := range bars {
		metadata, e := json.Marshal(bar)
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, stmt, bar.Symbol, bar.TradeDate, bar.Period, string(bar.AdjustmentBasis), bar.Source, bar.Open, bar.High, bar.Low, bar.Close, bar.Volume, bar.Amount, bar.ChangePercent, bar.FetchedAt.UTC().Format("2006-01-02 15:04:05.999999"), bar.NormalizedAt.UTC().Format("2006-01-02 15:04:05.999999"), string(bar.QualityStatus), metadata)
		if e != nil {
			return fmt.Errorf("save daily bar %s %s: %w", bar.Symbol, bar.TradeDate, e)
		}
	}
	return tx.Commit()
}
func (s *Store) ListBars(ctx context.Context, symbol, start, end string, adjust model.AdjustmentBasis, limit int) ([]model.DailyBar, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("MariaDB store disabled")
	}
	query := `SELECT metadata FROM market_data_daily_bars WHERE symbol=? AND adjustment_basis=? AND (?='' OR trade_date>=?) AND (?='' OR trade_date<=?) ORDER BY trade_date DESC, fetched_at DESC`
	args := []any{symbol, string(adjust), start, start, end, end}
	if limit > 0 {
		// Read a bounded surplus because each date may have one valid row per
		// configured provider. Do not deduplicate here: the newest row can be
		// malformed, and the service validator must be able to fall back to an
		// older valid row for that date. Current supported sources are <= 2.
		if limit > int(^uint(0)>>1)/4 {
			return nil, errors.New("daily-bar limit overflows query row limit")
		}
		query += ` LIMIT ?`
		args = append(args, limit*4)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.DailyBar{}
	for rows.Next() {
		var meta []byte
		if err := rows.Scan(&meta); err != nil {
			return nil, err
		}
		var b model.DailyBar
		if err := json.Unmarshal(meta, &b); err != nil {
			// A corrupt metadata row should not discard every other valid
			// historical row. The query already selects the canonical typed
			// columns; skip only the unreadable metadata payload and continue.
			continue
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Keep DB's newest-first order. ValidateAndSortBars validates each row,
	// keeps the newest valid row for each trade date, and returns ascending bars.
	return out, nil
}
func exec(ctx context.Context, db *sql.DB, query string, args ...any) error {
	_, err := db.ExecContext(ctx, query, args...)
	return err
}
