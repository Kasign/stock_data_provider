package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
	_ "github.com/go-sql-driver/mysql"
)

type Store struct {
	db     *sql.DB
	ownsDB bool
}

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
	return &Store{db: db, ownsDB: true}, nil
}

// NewWithBorrowedDB 创建复用调用方连接池的 Store；Store 不拥有该连接池。
func NewWithBorrowedDB(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("borrowed MariaDB DB is required")
	}
	return &Store{db: db}, nil
}

// Configured reports whether this Store wraps a database connection pool.
func (s *Store) Configured() bool { return s != nil && s.db != nil }

func (s *Store) Close() error {
	if s == nil || s.db == nil || !s.ownsDB {
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

// ValidateCompatibilitySchema verifies the physical schema invariants introduced
// by Data migrations V4-V6. Migration history checksums cannot detect manual schema drift.
func (s *Store) ValidateCompatibilitySchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("schema validation context is required")
	}
	for _, column := range []string{"volume_ratio", "turnover_rate"} {
		var columnType, dataType, nullable string
		var precision, scale sql.NullInt64
		err := s.db.QueryRowContext(ctx, `SELECT column_type, data_type, is_nullable, numeric_precision, numeric_scale
FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = 'market_data_quotes' AND column_name = ?`, column).
			Scan(&columnType, &dataType, &nullable, &precision, &scale)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("market_data_quotes.%s is missing", column)
		}
		if err != nil {
			return fmt.Errorf("inspect market_data_quotes.%s: %w", column, err)
		}
		if strings.ToLower(columnType) != "decimal(16,8)" || strings.ToLower(dataType) != "decimal" ||
			nullable != "YES" || !precision.Valid || precision.Int64 != 16 ||
			!scale.Valid || scale.Int64 != 8 {
			return fmt.Errorf("market_data_quotes.%s must be nullable signed DECIMAL(16,8)", column)
		}
	}
	for _, index := range []struct {
		table, name, legacy, columns string
	}{
		{"market_data_daily_bar_versions", "idx_market_bar_revision_identity_asof", "uq_market_bar_revision_content", "symbol,trade_date,period,adjustment_basis,source,recorded_at,id"},
		{"market_data_fundamental_versions", "idx_market_fundamental_identity_asof", "uq_market_fundamental_revision_content", "symbol,report_year,report_quarter,source,recorded_at,id"},
	} {
		var legacyCount int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`, index.table, index.legacy).Scan(&legacyCount); err != nil {
			return fmt.Errorf("inspect legacy index %s.%s: %w", index.table, index.legacy, err)
		}
		if legacyCount != 0 {
			return fmt.Errorf("legacy index %s.%s must be removed", index.table, index.legacy)
		}
		rows, err := s.db.QueryContext(ctx, `SELECT non_unique, column_name, sub_part, index_type, collation FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ? ORDER BY seq_in_index`, index.table, index.name)
		if err != nil {
			return fmt.Errorf("inspect index %s.%s: %w", index.table, index.name, err)
		}
		var columns []string
		nonUnique := -1
		for rows.Next() {
			var currentNonUnique int
			var column, indexType string
			var subPart sql.NullInt64
			var collation sql.NullString
			if err := rows.Scan(&currentNonUnique, &column, &subPart, &indexType, &collation); err != nil {
				_ = rows.Close()
				return fmt.Errorf("read index %s.%s: %w", index.table, index.name, err)
			}
			if subPart.Valid || strings.ToUpper(indexType) != "BTREE" || !collation.Valid || collation.String != "A" {
				_ = rows.Close()
				return fmt.Errorf("index %s.%s must use full ascending BTREE columns", index.table, index.name)
			}
			if nonUnique != -1 && nonUnique != currentNonUnique {
				_ = rows.Close()
				return fmt.Errorf("index %s.%s has inconsistent uniqueness metadata", index.table, index.name)
			}
			nonUnique = currentNonUnique
			columns = append(columns, column)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read index %s.%s: %w", index.table, index.name, err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close index metadata for %s.%s: %w", index.table, index.name, err)
		}
		if nonUnique != 1 || strings.Join(columns, ",") != index.columns {
			return fmt.Errorf("index %s.%s must be non-unique with columns %s", index.table, index.name, index.columns)
		}
	}
	if err := s.validateTradingCalendarSnapshotSchema(ctx); err != nil {
		return err
	}
	if err := s.validateDailyBarsCloseSeriesSnapshotSchema(ctx); err != nil {
		return err
	}
	if err := s.validateDataAuthorizationPolicySchema(ctx); err != nil {
		return err
	}
	return nil
}

// MigrationDefinitions returns the immutable Data schema history used by Root Server.
// Versions 1-6 are published definitions and must not be changed.
func MigrationDefinitions() []platformstorage.Migration {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS market_data_provider_probe_runs (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
 run_id VARCHAR(64) NOT NULL,
 started_at DATETIME(6) NOT NULL,
 finished_at DATETIME(6) NOT NULL,
 status VARCHAR(32) NOT NULL,
 metadata JSON NOT NULL,
 PRIMARY KEY (id),
 UNIQUE KEY uk_provider_probe_run_id (run_id),
 KEY idx_provider_probe_started_at (started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS market_data_sentiment_snapshots (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
 market VARCHAR(32) NOT NULL,
 trade_date DATE NOT NULL,
 fetched_at DATETIME(6) NOT NULL,
 source VARCHAR(128) NOT NULL,
 quality_status VARCHAR(16) NOT NULL,
 metadata JSON NOT NULL,
 PRIMARY KEY (id),
 KEY idx_market_sentiment_day (market, trade_date, id),
 KEY idx_market_sentiment_fetched (fetched_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
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
		`CREATE TABLE IF NOT EXISTS market_data_daily_bar_versions (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
 symbol VARCHAR(16) NOT NULL,
 trade_date DATE NOT NULL,
 period VARCHAR(8) NOT NULL DEFAULT '1d',
 adjustment_basis VARCHAR(24) NOT NULL,
 source VARCHAR(64) NOT NULL,
 revision_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 recorded_at DATETIME(6) NOT NULL,
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
 PRIMARY KEY (id),
 KEY idx_market_bar_revision_identity_asof (symbol, trade_date, period, adjustment_basis, source, recorded_at, id),
 KEY idx_market_bar_revision_asof (symbol, trade_date, adjustment_basis, recorded_at),
 KEY idx_market_bar_revision_observed (recorded_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS market_data_fundamental_versions (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
 symbol VARCHAR(16) NOT NULL,
 report_year SMALLINT NOT NULL,
 report_quarter TINYINT NOT NULL,
 source VARCHAR(64) NOT NULL,
 revision_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 recorded_at DATETIME(6) NOT NULL,
 fetched_at DATETIME(6) NOT NULL,
 quality_status VARCHAR(16) NOT NULL,
 metadata JSON NOT NULL,
 PRIMARY KEY (id),
 KEY idx_market_fundamental_asof (symbol, report_year, report_quarter, recorded_at),
 KEY idx_market_fundamental_observed (recorded_at),
 KEY idx_market_fundamental_identity_asof (symbol, report_year, report_quarter, source, recorded_at, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS market_data_security_trading_status_versions (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
 symbol VARCHAR(16) NOT NULL,
 trade_date DATE NOT NULL,
 source VARCHAR(64) NOT NULL,
 revision_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 recorded_at DATETIME(6) NOT NULL,
 fetched_at DATETIME(6) NOT NULL,
 quality_status VARCHAR(16) NOT NULL,
 metadata JSON NOT NULL,
 PRIMARY KEY (id),
 KEY idx_market_security_status_identity_asof (symbol, trade_date, source, recorded_at, id),
 KEY idx_market_security_status_date_asof (symbol, trade_date, recorded_at),
 KEY idx_market_security_status_observed (recorded_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS market_data_adjustment_factor_versions (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
 symbol VARCHAR(16) NOT NULL,
 trade_date DATE NOT NULL,
 source VARCHAR(64) NOT NULL,
 factor_basis VARCHAR(16) NOT NULL,
 revision_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 recorded_at DATETIME(6) NOT NULL,
 fetched_at DATETIME(6) NOT NULL,
 available_at DATETIME(6) NULL,
 quality_status VARCHAR(16) NOT NULL,
 metadata JSON NOT NULL,
 PRIMARY KEY (id),
 KEY idx_market_adjustment_factor_asof (symbol, trade_date, source, factor_basis, recorded_at, id),
 KEY idx_market_adjustment_factor_observed (recorded_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS market_data_corporate_action_versions (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
 symbol VARCHAR(16) NOT NULL,
 provider_event_id VARCHAR(128) NOT NULL,
 source VARCHAR(64) NOT NULL,
 ex_date DATE NULL,
 announcement_date DATE NULL,
 revision_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 recorded_at DATETIME(6) NOT NULL,
 fetched_at DATETIME(6) NOT NULL,
 available_at DATETIME(6) NULL,
 quality_status VARCHAR(16) NOT NULL,
 metadata JSON NOT NULL,
 PRIMARY KEY (id),
 KEY idx_market_corporate_action_asof (symbol, ex_date, source, recorded_at, id),
 KEY idx_market_corporate_action_event_asof (symbol, provider_event_id, source, recorded_at, id),
 KEY idx_market_corporate_action_announcement (symbol, announcement_date),
 KEY idx_market_corporate_action_observed (recorded_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	}
	return []platformstorage.Migration{
		{Version: 1, Description: "market data base schema", Statements: stmts},
		{
			Version: 2, Description: "adjustment factor source-version identity",
			Statements: []string{
				`ALTER TABLE market_data_adjustment_factor_versions ADD COLUMN source_version VARCHAR(128) NOT NULL DEFAULT '' AFTER source`,
				`UPDATE market_data_adjustment_factor_versions
SET source_version=COALESCE(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(metadata,'$.source_version')),''),'')
WHERE source_version=''`,
				`CREATE INDEX idx_market_adjustment_factor_version_asof
ON market_data_adjustment_factor_versions (symbol, trade_date, source, source_version, factor_basis, recorded_at, id)`,
			},
		},
		{
			Version: 3, Description: "valuation snapshot revision history",
			Statements: []string{
				"CREATE TABLE IF NOT EXISTS market_data_valuation_versions (id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, request_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, source VARCHAR(64) NOT NULL, revision_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, recorded_at DATETIME(6) NOT NULL, fetched_at DATETIME(6) NOT NULL, quality_status VARCHAR(16) NOT NULL, metadata JSON NOT NULL, PRIMARY KEY (id), KEY idx_market_valuation_asof (request_fingerprint, recorded_at), KEY idx_market_valuation_identity_asof (request_fingerprint, source, recorded_at, id), KEY idx_market_valuation_observed (recorded_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci",
			},
		},
		{
			Version: 4, Description: "legacy market-data index and column compatibility",
			Operations: []platformstorage.MigrationOperation{
				{Kind: platformstorage.MigrationOperationEnsureNonUniqueIndex, Table: "market_data_daily_bar_versions", LegacyName: "uq_market_bar_revision_content", Name: "idx_market_bar_revision_identity_asof", Columns: "symbol, trade_date, period, adjustment_basis, source, recorded_at, id"},
				{Kind: platformstorage.MigrationOperationEnsureNonUniqueIndex, Table: "market_data_fundamental_versions", LegacyName: "uq_market_fundamental_revision_content", Name: "idx_market_fundamental_identity_asof", Columns: "symbol, report_year, report_quarter, source, recorded_at, id"},
				{Kind: platformstorage.MigrationOperationEnsureColumn, Table: "market_data_quotes", Name: "volume_ratio", Definition: "DECIMAL(16,8) NULL"},
				{Kind: platformstorage.MigrationOperationEnsureColumn, Table: "market_data_quotes", Name: "turnover_rate", Definition: "DECIMAL(16,8) NULL"},
			},
		},
		{
			Version: 5, Description: "immutable trading-calendar content snapshots",
			Statements: []string{
				`CREATE TABLE IF NOT EXISTS market_data_trading_calendar_snapshots (
 calendar_ref VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 content_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 market VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 start_date DATE NOT NULL,
 end_date DATE NOT NULL,
 source VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 adapter_version VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 schema_version VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 quality_status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 fetched_at DATETIME(6) NOT NULL,
 observed_at DATETIME(6) NOT NULL,
 payload_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
 PRIMARY KEY (calendar_ref),
 KEY idx_market_calendar_identity_observed (market, start_date, end_date, observed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			},
		},
		{
					Version: 6, Description: "immutable daily-bars close-series content snapshots",
			Statements: []string{
				`CREATE TABLE IF NOT EXISTS market_data_daily_bars_close_series_snapshots (
 snapshot_ref VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 manifest_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 payload_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 manifest_schema_version VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 payload_schema_version VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 manifest_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
 payload_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 PRIMARY KEY (snapshot_ref)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			},
		},
		{
			Version: 7, Description: "immutable Data authorization policy versions and current pointers",
			Statements: []string{
				`CREATE TABLE IF NOT EXISTS market_data_authorization_policy_versions (
 policy_type VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 policy_ref VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 policy_version VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 payload_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
 payload_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 created_at DATETIME(6) NOT NULL,
 PRIMARY KEY (policy_type, policy_ref, policy_version),
 KEY idx_market_data_policy_versions_created (policy_type, policy_ref, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE TABLE IF NOT EXISTS market_data_current_authorization_policy_versions (
 policy_type VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 policy_ref VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 policy_version VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 PRIMARY KEY (policy_type, policy_ref),
 KEY idx_market_data_current_policy_version (policy_type, policy_ref, policy_version),
 CONSTRAINT fk_market_data_current_policy_version
 FOREIGN KEY (policy_type, policy_ref, policy_version)
 REFERENCES market_data_authorization_policy_versions (policy_type, policy_ref, policy_version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			},
		},；Root Server 应注册 MigrationDefinitions 到统一 Registry。
func (s *Store) Migrate(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	return platformstorage.ApplyMigrations(ctx, s.db, "market_data", MigrationDefinitions())
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
	versionStmt := `INSERT INTO market_data_daily_bar_versions (symbol,trade_date,period,adjustment_basis,source,revision_hash,recorded_at,open_price,high_price,low_price,close_price,volume_shares,amount_cny,change_percent,fetched_at,normalized_at,quality_status,metadata) VALUES (?,?,?,?,?,?,UTC_TIMESTAMP(6),?,?,?,?,?,?,?,?,?,?,?)`
	for _, bar := range bars {
		metadata, e := marshalDailyBarMetadata(bar)
		if e != nil {
			return e
		}
		revisionHash, e := dailyBarRevisionHash(bar)
		if e != nil {
			return fmt.Errorf("fingerprint daily bar %s %s: %w", bar.Symbol, bar.TradeDate, e)
		}
		var latestHash string
		lookupErr := tx.QueryRowContext(ctx, `SELECT revision_hash FROM market_data_daily_bar_versions
WHERE symbol=? AND trade_date=? AND period=? AND adjustment_basis=? AND source=?
ORDER BY recorded_at DESC, id DESC LIMIT 1 FOR UPDATE`,
			bar.Symbol, bar.TradeDate, bar.Period, string(bar.AdjustmentBasis), bar.Source).Scan(&latestHash)
		if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
			return fmt.Errorf("read latest daily bar revision %s %s: %w", bar.Symbol, bar.TradeDate, lookupErr)
		}
		if shouldRecordRevision(latestHash, lookupErr == nil, revisionHash) {
			_, e = tx.ExecContext(ctx, versionStmt, bar.Symbol, bar.TradeDate, bar.Period, string(bar.AdjustmentBasis), bar.Source, revisionHash, bar.Open, bar.High, bar.Low, bar.Close, bar.Volume, bar.Amount, bar.ChangePercent, bar.FetchedAt.UTC().Format("2006-01-02 15:04:05.999999"), bar.NormalizedAt.UTC().Format("2006-01-02 15:04:05.999999"), string(bar.QualityStatus), metadata)
			if e != nil {
				return fmt.Errorf("record daily bar revision %s %s: %w", bar.Symbol, bar.TradeDate, e)
			}
		}
		_, e = tx.ExecContext(ctx, stmt, bar.Symbol, bar.TradeDate, bar.Period, string(bar.AdjustmentBasis), bar.Source, bar.Open, bar.High, bar.Low, bar.Close, bar.Volume, bar.Amount, bar.ChangePercent, bar.FetchedAt.UTC().Format("2006-01-02 15:04:05.999999"), bar.NormalizedAt.UTC().Format("2006-01-02 15:04:05.999999"), string(bar.QualityStatus), metadata)
		if e != nil {
			return fmt.Errorf("save daily bar %s %s: %w", bar.Symbol, bar.TradeDate, e)
		}
	}
	return tx.Commit()
}

func shouldRecordRevision(previousHash string, hasPrevious bool, nextHash string) bool {
	return !hasPrevious || previousHash != nextHash
}

func dailyBarRevisionHash(bar model.DailyBar) (string, error) {
	// 获取/归一化时间变化本身不是行情修订；只对内容、来源版本、单位和质量变化生成新版本。
	fingerprint := bar
	fingerprint.FetchedAt = time.Time{}
	fingerprint.NormalizedAt = time.Time{}
	fingerprint.SystemObservedAt = nil
	payload, err := marshalDailyBarMetadata(fingerprint)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// ListBarsAsOf returns the latest recorded revision per source and date that was
// already observed by the service at asOf. It deliberately returns different
// providers independently; provider-priority resolution remains a service concern.
func (s *Store) ListBarsAsOf(ctx context.Context, symbol, start, end string, adjust model.AdjustmentBasis, asOf time.Time, limit int) ([]model.DailyBar, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("MariaDB store disabled")
	}
	if asOf.IsZero() {
		return nil, errors.New("daily-bar as-of timestamp must not be zero")
	}
	if limit < 0 {
		return nil, errors.New("daily-bar as-of limit must not be negative")
	}
	asOfText := asOf.UTC().Format("2006-01-02 15:04:05.999999")
	query := `SELECT v.metadata, DATE_FORMAT(v.recorded_at, '%Y-%m-%d %H:%i:%s.%f')
FROM market_data_daily_bar_versions AS v
WHERE v.symbol=? AND v.adjustment_basis=?
  AND (?='' OR v.trade_date>=?) AND (?='' OR v.trade_date<=?)
  AND v.recorded_at<=?
  AND NOT EXISTS (
    SELECT 1 FROM market_data_daily_bar_versions AS newer
    WHERE newer.symbol=v.symbol AND newer.trade_date=v.trade_date
      AND newer.period=v.period AND newer.adjustment_basis=v.adjustment_basis
      AND newer.source=v.source AND newer.recorded_at<=?
      AND (newer.recorded_at>v.recorded_at OR (newer.recorded_at=v.recorded_at AND newer.id>v.id))
  )
ORDER BY v.trade_date DESC, v.recorded_at DESC, v.source ASC`
	args := []any{symbol, string(adjust), start, start, end, end, asOfText, asOfText}
	if limit > 0 {
		// PIT 查询需要比较多个来源的同日修订后再择取一致的来源序列。
		// 为多个 Provider 预留行数，避免 LIMIT 先截断低优先级来源而误判覆盖不足。
		if limit > int(^uint(0)>>1)/16 {
			return nil, errors.New("daily-bar as-of limit overflows query row limit")
		}
		query += ` LIMIT ?`
		args = append(args, limit*16)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query daily-bar revisions as of %s: %w", asOfText, err)
	}
	defer rows.Close()
	out := []model.DailyBar{}
	for rows.Next() {
		var metadata []byte
		var recordedAtText string
		if err := rows.Scan(&metadata, &recordedAtText); err != nil {
			return nil, err
		}
		var bar model.DailyBar
		if err := unmarshalDailyBarMetadata(metadata, &bar); err != nil {
			// Keep other source/date versions usable if one historic metadata blob is damaged.
			continue
		}
		recordedAt, err := time.ParseInLocation("2006-01-02 15:04:05.000000", recordedAtText, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("parse daily-bar record timestamp: %w", err)
		}
		bar.SystemObservedAt = &recordedAt
		out = append(out, bar)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func fundamentalsRevisionHash(snapshot model.FundamentalsSnapshot) (string, error) {
	fingerprint := snapshot
	fingerprint.NormalizeInstantsUTC()
	fingerprint.FetchedAt = time.Time{}
	fingerprint.SystemObservedAt = nil
	payload, err := json.Marshal(fingerprint)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// SaveFundamentals 追加保存财务快照的不同内容版本，不覆盖此前已观察到的修订。
func (s *Store) SaveFundamentals(ctx context.Context, snapshot *model.FundamentalsSnapshot) error {
	if s == nil || s.db == nil {
		return nil
	}
	if snapshot == nil {
		return errors.New("fundamentals snapshot is nil")
	}
	if snapshot.Symbol == "" || snapshot.Source == "" || snapshot.Year < 2000 ||
		snapshot.Quarter < 1 || snapshot.Quarter > 4 || snapshot.FetchedAt.IsZero() {
		return errors.New("fundamentals snapshot identity, period, source or fetched_at is invalid")
	}
	stored := *snapshot
	stored.NormalizeInstantsUTC()
	stored.SystemObservedAt = nil
	metadata, err := json.Marshal(&stored)
	if err != nil {
		return err
	}
	revisionHash, err := fundamentalsRevisionHash(stored)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin fundamentals revision transaction: %w", err)
	}
	defer tx.Rollback()

	var latestHash string
	lookupErr := tx.QueryRowContext(ctx, `SELECT revision_hash FROM market_data_fundamental_versions
WHERE symbol=? AND report_year=? AND report_quarter=? AND source=?
ORDER BY recorded_at DESC, id DESC LIMIT 1 FOR UPDATE`,
		stored.Symbol, stored.Year, stored.Quarter, stored.Source).Scan(&latestHash)
	if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
		return fmt.Errorf("read latest fundamentals revision %s %dQ%d: %w", stored.Symbol, stored.Year, stored.Quarter, lookupErr)
	}
	if shouldRecordRevision(latestHash, lookupErr == nil, revisionHash) {
		_, err = tx.ExecContext(ctx, `INSERT INTO market_data_fundamental_versions
(symbol,report_year,report_quarter,source,revision_hash,recorded_at,fetched_at,quality_status,metadata)
VALUES (?,?,?,?,?,UTC_TIMESTAMP(6),?,?,?)`,
			stored.Symbol, stored.Year, stored.Quarter, stored.Source, revisionHash,
			stored.FetchedAt.UTC().Format("2006-01-02 15:04:05.999999"), string(stored.QualityStatus), metadata)
		if err != nil {
			return fmt.Errorf("record fundamentals revision %s %dQ%d: %w", stored.Symbol, stored.Year, stored.Quarter, err)
		}
	}
	return tx.Commit()
}

// ListFundamentalsAsOf 读取截至系统观察时点、每个来源当时最新的报告版本。
func valuationRevisionHash(snapshot model.ValuationSnapshot) (string, error) {
	fingerprint := snapshot
	fingerprint.FetchedAt = time.Time{}
	fingerprint.SystemObservedAt = nil
	fingerprint.Items = append([]model.ValuationItem(nil), snapshot.Items...)
	for index := range fingerprint.Items {
		fingerprint.Items[index].FetchedAt = time.Time{}
	}
	payload, err := json.Marshal(fingerprint)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func valuationRequestFingerprint(symbols []string) (string, error) {
	if len(symbols) == 0 || len(symbols) > 100 {
		return "", errors.New("valuation request fingerprint requires 1..100 symbols")
	}
	seen := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		if symbol == "" {
			return "", errors.New("valuation request fingerprint contains an empty symbol")
		}
		if _, exists := seen[symbol]; exists {
			return "", errors.New("valuation request fingerprint contains duplicate symbols")
		}
		seen[symbol] = struct{}{}
	}
	payload, err := json.Marshal(symbols)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// SaveValuations 追加保存批量估值快照的内容修订；重复抓取相同内容不制造新版本。
func (s *Store) SaveValuations(ctx context.Context, snapshot *model.ValuationSnapshot) error {
	if s == nil || s.db == nil {
		return nil
	}
	if snapshot == nil {
		return errors.New("valuation snapshot is nil")
	}
	if strings.TrimSpace(snapshot.Source) == "" || strings.TrimSpace(snapshot.SourceVersion) == "" ||
		snapshot.FetchedAt.IsZero() || snapshot.Total != len(snapshot.Items) {
		return errors.New("valuation snapshot source, source_version, fetched_at or total is invalid")
	}
	fingerprint, err := valuationRequestFingerprint(snapshot.RequestedSymbols)
	if err != nil {
		return err
	}
	stored := *snapshot
	stored.SystemObservedAt = nil
	metadata, err := json.Marshal(&stored)
	if err != nil {
		return err
	}
	revisionHash, err := valuationRevisionHash(stored)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin valuation revision transaction: %w", err)
	}
	defer tx.Rollback()

	var latestHash string
	lookupErr := tx.QueryRowContext(ctx, "SELECT revision_hash FROM market_data_valuation_versions WHERE request_fingerprint=? AND source=? ORDER BY recorded_at DESC, id DESC LIMIT 1 FOR UPDATE",
		fingerprint, stored.Source).Scan(&latestHash)
	if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
		return fmt.Errorf("read latest valuation revision for %s: %w", stored.Source, lookupErr)
	}
	if shouldRecordRevision(latestHash, lookupErr == nil, revisionHash) {
		_, err = tx.ExecContext(ctx, "INSERT INTO market_data_valuation_versions (request_fingerprint,source,revision_hash,recorded_at,fetched_at,quality_status,metadata) VALUES (?,?,?,UTC_TIMESTAMP(6),?,?,?)",
			fingerprint, stored.Source, revisionHash,
			stored.FetchedAt.UTC().Format("2006-01-02 15:04:05.999999"), string(stored.QualityStatus), metadata)
		if err != nil {
			return fmt.Errorf("record valuation revision for %s: %w", stored.Source, err)
		}
	}
	return tx.Commit()
}

// ListValuationsAsOf 返回系统观察截止时点前、每个 Provider 当时最新的同一请求快照。
func (s *Store) ListValuationsAsOf(ctx context.Context, symbols []string, asOf time.Time) ([]model.ValuationSnapshot, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("MariaDB valuation revision storage disabled")
	}
	if asOf.IsZero() {
		return nil, errors.New("valuation as_of timestamp must not be zero")
	}
	fingerprint, err := valuationRequestFingerprint(symbols)
	if err != nil {
		return nil, err
	}
	asOfText := asOf.UTC().Format("2006-01-02 15:04:05.999999")
	rows, err := s.db.QueryContext(ctx, "SELECT v.metadata, DATE_FORMAT(v.recorded_at, '%Y-%m-%d %H:%i:%s.%f') FROM market_data_valuation_versions AS v WHERE v.request_fingerprint=? AND v.recorded_at<=? AND NOT EXISTS (SELECT 1 FROM market_data_valuation_versions AS newer WHERE newer.request_fingerprint=v.request_fingerprint AND newer.source=v.source AND newer.recorded_at<=? AND (newer.recorded_at>v.recorded_at OR (newer.recorded_at=v.recorded_at AND newer.id>v.id))) ORDER BY v.recorded_at DESC, v.source ASC",
		fingerprint, asOfText, asOfText)
	if err != nil {
		return nil, fmt.Errorf("query valuation revisions as of %s: %w", asOfText, err)
	}
	defer rows.Close()
	out := []model.ValuationSnapshot{}
	for rows.Next() {
		var metadata []byte
		var recordedAtText string
		if err := rows.Scan(&metadata, &recordedAtText); err != nil {
			return nil, err
		}
		var snapshot model.ValuationSnapshot
		if err := json.Unmarshal(metadata, &snapshot); err != nil {
			return nil, fmt.Errorf("decode valuation revision metadata: %w", err)
		}
		recordedAt, err := time.ParseInLocation("2006-01-02 15:04:05.000000", recordedAtText, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("parse valuation record timestamp: %w", err)
		}
		snapshot.SystemObservedAt = &recordedAt
		out = append(out, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) ListFundamentalsAsOf(ctx context.Context, symbol string, year, quarter int, asOf time.Time) ([]model.FundamentalsSnapshot, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("MariaDB fundamentals revision storage disabled")
	}
	if asOf.IsZero() {
		return nil, errors.New("fundamentals as_of timestamp must not be zero")
	}
	if year < 2000 || quarter < 1 || quarter > 4 {
		return nil, errors.New("fundamentals report period is invalid")
	}
	asOfText := asOf.UTC().Format("2006-01-02 15:04:05.999999")
	rows, err := s.db.QueryContext(ctx, `SELECT v.metadata, DATE_FORMAT(v.recorded_at, '%Y-%m-%d %H:%i:%s.%f')
FROM market_data_fundamental_versions AS v
WHERE v.symbol=? AND v.report_year=? AND v.report_quarter=? AND v.recorded_at<=?
  AND NOT EXISTS (
    SELECT 1 FROM market_data_fundamental_versions AS newer
    WHERE newer.symbol=v.symbol AND newer.report_year=v.report_year AND newer.report_quarter=v.report_quarter
      AND newer.source=v.source AND newer.recorded_at<=?
      AND (newer.recorded_at>v.recorded_at OR (newer.recorded_at=v.recorded_at AND newer.id>v.id))
  )
ORDER BY v.recorded_at DESC, v.source ASC`, symbol, year, quarter, asOfText, asOfText)
	if err != nil {
		return nil, fmt.Errorf("query fundamentals revisions as of %s: %w", asOfText, err)
	}
	defer rows.Close()
	out := []model.FundamentalsSnapshot{}
	for rows.Next() {
		var metadata []byte
		var recordedAtText string
		if err := rows.Scan(&metadata, &recordedAtText); err != nil {
			return nil, err
		}
		var snapshot model.FundamentalsSnapshot
		if err := json.Unmarshal(metadata, &snapshot); err != nil {
			return nil, fmt.Errorf("decode fundamentals revision metadata: %w", err)
		}
		snapshot.NormalizeInstantsUTC()
		recordedAt, err := time.ParseInLocation("2006-01-02 15:04:05.000000", recordedAtText, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("parse fundamentals record timestamp: %w", err)
		}
		snapshot.SystemObservedAt = &recordedAt
		out = append(out, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
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
		if err := unmarshalDailyBarMetadata(meta, &b); err != nil {
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
}		{
			Version: 7, Description: "immutable Data authorization policy versions and current pointers",
			Statements: []string{
				`CREATE TABLE IF NOT EXISTS market_data_authorization_policy_versions (
 policy_type VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 policy_ref VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 policy_version VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 payload_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
 payload_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 created_at DATETIME(6) NOT NULL,
 PRIMARY KEY (policy_type, policy_ref, policy_version),
 KEY idx_market_data_policy_versions_created (policy_type, policy_ref, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE TABLE IF NOT EXISTS market_data_current_authorization_policy_versions (
 policy_type VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 policy_ref VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 policy_version VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 PRIMARY KEY (policy_type, policy_ref),
 KEY idx_market_data_current_policy_version (policy_type, policy_ref, policy_version),
 CONSTRAINT fk_market_data_current_policy_version
 FOREIGN KEY (policy_type, policy_ref, policy_version)
 REFERENCES market_data_authorization_policy_versions (policy_type, policy_ref, policy_version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			},
		},
	}
}

// Migrate 保留给独立 Data 命令使用t (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
	_ "github.com/go-sql-driver/mysql"
)

type Store struct {
	db     *sql.DB
	ownsDB bool
}

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
	return &Store{db: db, ownsDB: true}, nil
}

// NewWithBorrowedDB 创建复用调用方连接池的 Store；Store 不拥有该连接池。
func NewWithBorrowedDB(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("borrowed MariaDB DB is required")
	}
	return &Store{db: db}, nil
}

// Configured reports whether this Store wraps a database connection pool.
func (s *Store) Configured() bool { return s != nil && s.db != nil }

func (s *Store) Close() error {
	if s == nil || s.db == nil || !s.ownsDB {
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

// ValidateCompatibilitySchema verifies the physical schema invariants introduced
// by Data migrations V4-V6. Migration history checksums cannot detect manual schema drift.
func (s *Store) ValidateCompatibilitySchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("schema validation context is required")
	}
	for _, column := range []string{"volume_ratio", "turnover_rate"} {
		var columnType, dataType, nullable string
		var precision, scale sql.NullInt64
		err := s.db.QueryRowContext(ctx, `SELECT column_type, data_type, is_nullable, numeric_precision, numeric_scale
FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = 'market_data_quotes' AND column_name = ?`, column).
			Scan(&columnType, &dataType, &nullable, &precision, &scale)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("market_data_quotes.%s is missing", column)
		}
		if err != nil {
			return fmt.Errorf("inspect market_data_quotes.%s: %w", column, err)
		}
		if strings.ToLower(columnType) != "decimal(16,8)" || strings.ToLower(dataType) != "decimal" ||
			nullable != "YES" || !precision.Valid || precision.Int64 != 16 ||
			!scale.Valid || scale.Int64 != 8 {
			return fmt.Errorf("market_data_quotes.%s must be nullable signed DECIMAL(16,8)", column)
		}
	}
	for _, index := range []struct {
		table, name, legacy, columns string
	}{
		{"market_data_daily_bar_versions", "idx_market_bar_revision_identity_asof", "uq_market_bar_revision_content", "symbol,trade_date,period,adjustment_basis,source,recorded_at,id"},
		{"market_data_fundamental_versions", "idx_market_fundamental_identity_asof", "uq_market_fundamental_revision_content", "symbol,report_year,report_quarter,source,recorded_at,id"},
	} {
		var legacyCount int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`, index.table, index.legacy).Scan(&legacyCount); err != nil {
			return fmt.Errorf("inspect legacy index %s.%s: %w", index.table, index.legacy, err)
		}
		if legacyCount != 0 {
			return fmt.Errorf("legacy index %s.%s must be removed", index.table, index.legacy)
		}
		rows, err := s.db.QueryContext(ctx, `SELECT non_unique, column_name, sub_part, index_type, collation FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ? ORDER BY seq_in_index`, index.table, index.name)
		if err != nil {
			return fmt.Errorf("inspect index %s.%s: %w", index.table, index.name, err)
		}
		var columns []string
		nonUnique := -1
		for rows.Next() {
			var currentNonUnique int
			var column, indexType string
			var subPart sql.NullInt64
			var collation sql.NullString
			if err := rows.Scan(&currentNonUnique, &column, &subPart, &indexType, &collation); err != nil {
				_ = rows.Close()
				return fmt.Errorf("read index %s.%s: %w", index.table, index.name, err)
			}
			if subPart.Valid || strings.ToUpper(indexType) != "BTREE" || !collation.Valid || collation.String != "A" {
				_ = rows.Close()
				return fmt.Errorf("index %s.%s must use full ascending BTREE columns", index.table, index.name)
			}
			if nonUnique != -1 && nonUnique != currentNonUnique {
				_ = rows.Close()
				return fmt.Errorf("index %s.%s has inconsistent uniqueness metadata", index.table, index.name)
			}
			nonUnique = currentNonUnique
			columns = append(columns, column)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read index %s.%s: %w", index.table, index.name, err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close index metadata for %s.%s: %w", index.table, index.name, err)
		}
		if nonUnique != 1 || strings.Join(columns, ",") != index.columns {
			return fmt.Errorf("index %s.%s must be non-unique with columns %s", index.table, index.name, index.columns)
		}
	}
	if err := s.validateTradingCalendarSnapshotSchema(ctx); err != nil {
		return err
	}
	if err := s.validateDailyBarsCloseSeriesSnapshotSchema(ctx); err != nil {
		return err
	}
	if err := s.validateDataAuthorizationPolicySchema(ctx); err != nil {
		return err
	}
	return nil
}

// MigrationDefinitions returns the immutable Data schema history used by Root Server.
// Versions 1-6 are published definitions and must not be changed.
func MigrationDefinitions() []platformstorage.Migration {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS market_data_provider_probe_runs (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
 run_id VARCHAR(64) NOT NULL,
 started_at DATETIME(6) NOT NULL,
 finished_at DATETIME(6) NOT NULL,
 status VARCHAR(32) NOT NULL,
 metadata JSON NOT NULL,
 PRIMARY KEY (id),
 UNIQUE KEY uk_provider_probe_run_id (run_id),
 KEY idx_provider_probe_started_at (started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS market_data_sentiment_snapshots (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
 market VARCHAR(32) NOT NULL,
 trade_date DATE NOT NULL,
 fetched_at DATETIME(6) NOT NULL,
 source VARCHAR(128) NOT NULL,
 quality_status VARCHAR(16) NOT NULL,
 metadata JSON NOT NULL,
 PRIMARY KEY (id),
 KEY idx_market_sentiment_day (market, trade_date, id),
 KEY idx_market_sentiment_fetched (fetched_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
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
		`CREATE TABLE IF NOT EXISTS market_data_daily_bar_versions (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
 symbol VARCHAR(16) NOT NULL,
 trade_date DATE NOT NULL,
 period VARCHAR(8) NOT NULL DEFAULT '1d',
 adjustment_basis VARCHAR(24) NOT NULL,
 source VARCHAR(64) NOT NULL,
 revision_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 recorded_at DATETIME(6) NOT NULL,
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
 PRIMARY KEY (id),
 KEY idx_market_bar_revision_identity_asof (symbol, trade_date, period, adjustment_basis, source, recorded_at, id),
 KEY idx_market_bar_revision_asof (symbol, trade_date, adjustment_basis, recorded_at),
 KEY idx_market_bar_revision_observed (recorded_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS market_data_fundamental_versions (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
 symbol VARCHAR(16) NOT NULL,
 report_year SMALLINT NOT NULL,
 report_quarter TINYINT NOT NULL,
 source VARCHAR(64) NOT NULL,
 revision_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 recorded_at DATETIME(6) NOT NULL,
 fetched_at DATETIME(6) NOT NULL,
 quality_status VARCHAR(16) NOT NULL,
 metadata JSON NOT NULL,
 PRIMARY KEY (id),
 KEY idx_market_fundamental_asof (symbol, report_year, report_quarter, recorded_at),
 KEY idx_market_fundamental_observed (recorded_at),
 KEY idx_market_fundamental_identity_asof (symbol, report_year, report_quarter, source, recorded_at, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS market_data_security_trading_status_versions (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
 symbol VARCHAR(16) NOT NULL,
 trade_date DATE NOT NULL,
 source VARCHAR(64) NOT NULL,
 revision_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 recorded_at DATETIME(6) NOT NULL,
 fetched_at DATETIME(6) NOT NULL,
 quality_status VARCHAR(16) NOT NULL,
 metadata JSON NOT NULL,
 PRIMARY KEY (id),
 KEY idx_market_security_status_identity_asof (symbol, trade_date, source, recorded_at, id),
 KEY idx_market_security_status_date_asof (symbol, trade_date, recorded_at),
 KEY idx_market_security_status_observed (recorded_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS market_data_adjustment_factor_versions (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
 symbol VARCHAR(16) NOT NULL,
 trade_date DATE NOT NULL,
 source VARCHAR(64) NOT NULL,
 factor_basis VARCHAR(16) NOT NULL,
 revision_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 recorded_at DATETIME(6) NOT NULL,
 fetched_at DATETIME(6) NOT NULL,
 available_at DATETIME(6) NULL,
 quality_status VARCHAR(16) NOT NULL,
 metadata JSON NOT NULL,
 PRIMARY KEY (id),
 KEY idx_market_adjustment_factor_asof (symbol, trade_date, source, factor_basis, recorded_at, id),
 KEY idx_market_adjustment_factor_observed (recorded_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS market_data_corporate_action_versions (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
 symbol VARCHAR(16) NOT NULL,
 provider_event_id VARCHAR(128) NOT NULL,
 source VARCHAR(64) NOT NULL,
 ex_date DATE NULL,
 announcement_date DATE NULL,
 revision_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 recorded_at DATETIME(6) NOT NULL,
 fetched_at DATETIME(6) NOT NULL,
 available_at DATETIME(6) NULL,
 quality_status VARCHAR(16) NOT NULL,
 metadata JSON NOT NULL,
 PRIMARY KEY (id),
 KEY idx_market_corporate_action_asof (symbol, ex_date, source, recorded_at, id),
 KEY idx_market_corporate_action_event_asof (symbol, provider_event_id, source, recorded_at, id),
 KEY idx_market_corporate_action_announcement (symbol, announcement_date),
 KEY idx_market_corporate_action_observed (recorded_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	}
	return []platformstorage.Migration{
		{Version: 1, Description: "market data base schema", Statements: stmts},
		{
			Version: 2, Description: "adjustment factor source-version identity",
			Statements: []string{
				`ALTER TABLE market_data_adjustment_factor_versions ADD COLUMN source_version VARCHAR(128) NOT NULL DEFAULT '' AFTER source`,
				`UPDATE market_data_adjustment_factor_versions
SET source_version=COALESCE(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(metadata,'$.source_version')),''),'')
WHERE source_version=''`,
				`CREATE INDEX idx_market_adjustment_factor_version_asof
ON market_data_adjustment_factor_versions (symbol, trade_date, source, source_version, factor_basis, recorded_at, id)`,
			},
		},
		{
			Version: 3, Description: "valuation snapshot revision history",
			Statements: []string{
				"CREATE TABLE IF NOT EXISTS market_data_valuation_versions (id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, request_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, source VARCHAR(64) NOT NULL, revision_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, recorded_at DATETIME(6) NOT NULL, fetched_at DATETIME(6) NOT NULL, quality_status VARCHAR(16) NOT NULL, metadata JSON NOT NULL, PRIMARY KEY (id), KEY idx_market_valuation_asof (request_fingerprint, recorded_at), KEY idx_market_valuation_identity_asof (request_fingerprint, source, recorded_at, id), KEY idx_market_valuation_observed (recorded_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci",
			},
		},
		{
			Version: 4, Description: "legacy market-data index and column compatibility",
			Operations: []platformstorage.MigrationOperation{
				{Kind: platformstorage.MigrationOperationEnsureNonUniqueIndex, Table: "market_data_daily_bar_versions", LegacyName: "uq_market_bar_revision_content", Name: "idx_market_bar_revision_identity_asof", Columns: "symbol, trade_date, period, adjustment_basis, source, recorded_at, id"},
				{Kind: platformstorage.MigrationOperationEnsureNonUniqueIndex, Table: "market_data_fundamental_versions", LegacyName: "uq_market_fundamental_revision_content", Name: "idx_market_fundamental_identity_asof", Columns: "symbol, report_year, report_quarter, source, recorded_at, id"},
				{Kind: platformstorage.MigrationOperationEnsureColumn, Table: "market_data_quotes", Name: "volume_ratio", Definition: "DECIMAL(16,8) NULL"},
				{Kind: platformstorage.MigrationOperationEnsureColumn, Table: "market_data_quotes", Name: "turnover_rate", Definition: "DECIMAL(16,8) NULL"},
			},
		},
		{
			Version: 5, Description: "immutable trading-calendar content snapshots",
			Statements: []string{
				`CREATE TABLE IF NOT EXISTS market_data_trading_calendar_snapshots (
 calendar_ref VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 content_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 market VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 start_date DATE NOT NULL,
 end_date DATE NOT NULL,
 source VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 adapter_version VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 schema_version VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 quality_status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 fetched_at DATETIME(6) NOT NULL,
 observed_at DATETIME(6) NOT NULL,
 payload_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
 PRIMARY KEY (calendar_ref),
 KEY idx_market_calendar_identity_observed (market, start_date, end_date, observed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			},
		},
		{
			Version: 6, Description: "immutable daily-bars close-series content snapshots",
			Statements: []string{
				`CREATE TABLE IF NOT EXISTS market_data_daily_bars_close_series_snapshots (
 snapshot_ref VARCHAR(96) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 manifest_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 payload_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 manifest_schema_version VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 payload_schema_version VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 manifest_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
 payload_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 PRIMARY KEY (snapshot_ref)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			},
		},
		{
			Version: 7, Description: "immutable Data authorization policy versions and current pointers",
			Statements: []string{
				`CREATE TABLE IF NOT EXISTS market_data_authorization_policy_versions (
 policy_type VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 policy_ref VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 policy_version VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 payload_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
 payload_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 created_at DATETIME(6) NOT NULL,
 PRIMARY KEY (policy_type, policy_ref, policy_version),
 KEY idx_market_data_policy_versions_created (policy_type, policy_ref, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE TABLE IF NOT EXISTS market_data_current_authorization_policy_versions (
 policy_type VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 policy_ref VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 policy_version VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 PRIMARY KEY (policy_type, policy_ref),
 KEY idx_market_data_current_policy_version (policy_type, policy_ref, policy_version),
 CONSTRAINT fk_market_data_current_policy_version
 FOREIGN KEY (policy_type, policy_ref, policy_version)
 REFERENCES market_data_authorization_policy_versions (policy_type, policy_ref, policy_version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			},
		},；Root Server 应注册 MigrationDefinitions 到统一 Registry。
func (s *Store) Migrate(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	return platformstorage.ApplyMigrations(ctx, s.db, "market_data", MigrationDefinitions())
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
	versionStmt := `INSERT INTO market_data_daily_bar_versions (symbol,trade_date,period,adjustment_basis,source,revision_hash,recorded_at,open_price,high_price,low_price,close_price,volume_shares,amount_cny,change_percent,fetched_at,normalized_at,quality_status,metadata) VALUES (?,?,?,?,?,?,UTC_TIMESTAMP(6),?,?,?,?,?,?,?,?,?,?,?)`
	for _, bar := range bars {
		metadata, e := marshalDailyBarMetadata(bar)
		if e != nil {
			return e
		}
		revisionHash, e := dailyBarRevisionHash(bar)
		if e != nil {
			return fmt.Errorf("fingerprint daily bar %s %s: %w", bar.Symbol, bar.TradeDate, e)
		}
		var latestHash string
		lookupErr := tx.QueryRowContext(ctx, `SELECT revision_hash FROM market_data_daily_bar_versions
WHERE symbol=? AND trade_date=? AND period=? AND adjustment_basis=? AND source=?
ORDER BY recorded_at DESC, id DESC LIMIT 1 FOR UPDATE`,
			bar.Symbol, bar.TradeDate, bar.Period, string(bar.AdjustmentBasis), bar.Source).Scan(&latestHash)
		if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
			return fmt.Errorf("read latest daily bar revision %s %s: %w", bar.Symbol, bar.TradeDate, lookupErr)
		}
		if shouldRecordRevision(latestHash, lookupErr == nil, revisionHash) {
			_, e = tx.ExecContext(ctx, versionStmt, bar.Symbol, bar.TradeDate, bar.Period, string(bar.AdjustmentBasis), bar.Source, revisionHash, bar.Open, bar.High, bar.Low, bar.Close, bar.Volume, bar.Amount, bar.ChangePercent, bar.FetchedAt.UTC().Format("2006-01-02 15:04:05.999999"), bar.NormalizedAt.UTC().Format("2006-01-02 15:04:05.999999"), string(bar.QualityStatus), metadata)
			if e != nil {
				return fmt.Errorf("record daily bar revision %s %s: %w", bar.Symbol, bar.TradeDate, e)
			}
		}
		_, e = tx.ExecContext(ctx, stmt, bar.Symbol, bar.TradeDate, bar.Period, string(bar.AdjustmentBasis), bar.Source, bar.Open, bar.High, bar.Low, bar.Close, bar.Volume, bar.Amount, bar.ChangePercent, bar.FetchedAt.UTC().Format("2006-01-02 15:04:05.999999"), bar.NormalizedAt.UTC().Format("2006-01-02 15:04:05.999999"), string(bar.QualityStatus), metadata)
		if e != nil {
			return fmt.Errorf("save daily bar %s %s: %w", bar.Symbol, bar.TradeDate, e)
		}
	}
	return tx.Commit()
}

func shouldRecordRevision(previousHash string, hasPrevious bool, nextHash string) bool {
	return !hasPrevious || previousHash != nextHash
}

func dailyBarRevisionHash(bar model.DailyBar) (string, error) {
	// 获取/归一化时间变化本身不是行情修订；只对内容、来源版本、单位和质量变化生成新版本。
	fingerprint := bar
	fingerprint.FetchedAt = time.Time{}
	fingerprint.NormalizedAt = time.Time{}
	fingerprint.SystemObservedAt = nil
	payload, err := marshalDailyBarMetadata(fingerprint)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// ListBarsAsOf returns the latest recorded revision per source and date that was
// already observed by the service at asOf. It deliberately returns different
// providers independently; provider-priority resolution remains a service concern.
func (s *Store) ListBarsAsOf(ctx context.Context, symbol, start, end string, adjust model.AdjustmentBasis, asOf time.Time, limit int) ([]model.DailyBar, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("MariaDB store disabled")
	}
	if asOf.IsZero() {
		return nil, errors.New("daily-bar as-of timestamp must not be zero")
	}
	if limit < 0 {
		return nil, errors.New("daily-bar as-of limit must not be negative")
	}
	asOfText := asOf.UTC().Format("2006-01-02 15:04:05.999999")
	query := `SELECT v.metadata, DATE_FORMAT(v.recorded_at, '%Y-%m-%d %H:%i:%s.%f')
FROM market_data_daily_bar_versions AS v
WHERE v.symbol=? AND v.adjustment_basis=?
  AND (?='' OR v.trade_date>=?) AND (?='' OR v.trade_date<=?)
  AND v.recorded_at<=?
  AND NOT EXISTS (
    SELECT 1 FROM market_data_daily_bar_versions AS newer
    WHERE newer.symbol=v.symbol AND newer.trade_date=v.trade_date
      AND newer.period=v.period AND newer.adjustment_basis=v.adjustment_basis
      AND newer.source=v.source AND newer.recorded_at<=?
      AND (newer.recorded_at>v.recorded_at OR (newer.recorded_at=v.recorded_at AND newer.id>v.id))
  )
ORDER BY v.trade_date DESC, v.recorded_at DESC, v.source ASC`
	args := []any{symbol, string(adjust), start, start, end, end, asOfText, asOfText}
	if limit > 0 {
		// PIT 查询需要比较多个来源的同日修订后再择取一致的来源序列。
		// 为多个 Provider 预留行数，避免 LIMIT 先截断低优先级来源而误判覆盖不足。
		if limit > int(^uint(0)>>1)/16 {
			return nil, errors.New("daily-bar as-of limit overflows query row limit")
		}
		query += ` LIMIT ?`
		args = append(args, limit*16)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query daily-bar revisions as of %s: %w", asOfText, err)
	}
	defer rows.Close()
	out := []model.DailyBar{}
	for rows.Next() {
		var metadata []byte
		var recordedAtText string
		if err := rows.Scan(&metadata, &recordedAtText); err != nil {
			return nil, err
		}
		var bar model.DailyBar
		if err := unmarshalDailyBarMetadata(metadata, &bar); err != nil {
			// Keep other source/date versions usable if one historic metadata blob is damaged.
			continue
		}
		recordedAt, err := time.ParseInLocation("2006-01-02 15:04:05.000000", recordedAtText, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("parse daily-bar record timestamp: %w", err)
		}
		bar.SystemObservedAt = &recordedAt
		out = append(out, bar)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func fundamentalsRevisionHash(snapshot model.FundamentalsSnapshot) (string, error) {
	fingerprint := snapshot
	fingerprint.NormalizeInstantsUTC()
	fingerprint.FetchedAt = time.Time{}
	fingerprint.SystemObservedAt = nil
	payload, err := json.Marshal(fingerprint)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// SaveFundamentals 追加保存财务快照的不同内容版本，不覆盖此前已观察到的修订。
func (s *Store) SaveFundamentals(ctx context.Context, snapshot *model.FundamentalsSnapshot) error {
	if s == nil || s.db == nil {
		return nil
	}
	if snapshot == nil {
		return errors.New("fundamentals snapshot is nil")
	}
	if snapshot.Symbol == "" || snapshot.Source == "" || snapshot.Year < 2000 ||
		snapshot.Quarter < 1 || snapshot.Quarter > 4 || snapshot.FetchedAt.IsZero() {
		return errors.New("fundamentals snapshot identity, period, source or fetched_at is invalid")
	}
	stored := *snapshot
	stored.NormalizeInstantsUTC()
	stored.SystemObservedAt = nil
	metadata, err := json.Marshal(&stored)
	if err != nil {
		return err
	}
	revisionHash, err := fundamentalsRevisionHash(stored)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin fundamentals revision transaction: %w", err)
	}
	defer tx.Rollback()

	var latestHash string
	lookupErr := tx.QueryRowContext(ctx, `SELECT revision_hash FROM market_data_fundamental_versions
WHERE symbol=? AND report_year=? AND report_quarter=? AND source=?
ORDER BY recorded_at DESC, id DESC LIMIT 1 FOR UPDATE`,
		stored.Symbol, stored.Year, stored.Quarter, stored.Source).Scan(&latestHash)
	if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
		return fmt.Errorf("read latest fundamentals revision %s %dQ%d: %w", stored.Symbol, stored.Year, stored.Quarter, lookupErr)
	}
	if shouldRecordRevision(latestHash, lookupErr == nil, revisionHash) {
		_, err = tx.ExecContext(ctx, `INSERT INTO market_data_fundamental_versions
(symbol,report_year,report_quarter,source,revision_hash,recorded_at,fetched_at,quality_status,metadata)
VALUES (?,?,?,?,?,UTC_TIMESTAMP(6),?,?,?)`,
			stored.Symbol, stored.Year, stored.Quarter, stored.Source, revisionHash,
			stored.FetchedAt.UTC().Format("2006-01-02 15:04:05.999999"), string(stored.QualityStatus), metadata)
		if err != nil {
			return fmt.Errorf("record fundamentals revision %s %dQ%d: %w", stored.Symbol, stored.Year, stored.Quarter, err)
		}
	}
	return tx.Commit()
}

// ListFundamentalsAsOf 读取截至系统观察时点、每个来源当时最新的报告版本。
func valuationRevisionHash(snapshot model.ValuationSnapshot) (string, error) {
	fingerprint := snapshot
	fingerprint.FetchedAt = time.Time{}
	fingerprint.SystemObservedAt = nil
	fingerprint.Items = append([]model.ValuationItem(nil), snapshot.Items...)
	for index := range fingerprint.Items {
		fingerprint.Items[index].FetchedAt = time.Time{}
	}
	payload, err := json.Marshal(fingerprint)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func valuationRequestFingerprint(symbols []string) (string, error) {
	if len(symbols) == 0 || len(symbols) > 100 {
		return "", errors.New("valuation request fingerprint requires 1..100 symbols")
	}
	seen := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		if symbol == "" {
			return "", errors.New("valuation request fingerprint contains an empty symbol")
		}
		if _, exists := seen[symbol]; exists {
			return "", errors.New("valuation request fingerprint contains duplicate symbols")
		}
		seen[symbol] = struct{}{}
	}
	payload, err := json.Marshal(symbols)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// SaveValuations 追加保存批量估值快照的内容修订；重复抓取相同内容不制造新版本。
func (s *Store) SaveValuations(ctx context.Context, snapshot *model.ValuationSnapshot) error {
	if s == nil || s.db == nil {
		return nil
	}
	if snapshot == nil {
		return errors.New("valuation snapshot is nil")
	}
	if strings.TrimSpace(snapshot.Source) == "" || strings.TrimSpace(snapshot.SourceVersion) == "" ||
		snapshot.FetchedAt.IsZero() || snapshot.Total != len(snapshot.Items) {
		return errors.New("valuation snapshot source, source_version, fetched_at or total is invalid")
	}
	fingerprint, err := valuationRequestFingerprint(snapshot.RequestedSymbols)
	if err != nil {
		return err
	}
	stored := *snapshot
	stored.SystemObservedAt = nil
	metadata, err := json.Marshal(&stored)
	if err != nil {
		return err
	}
	revisionHash, err := valuationRevisionHash(stored)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin valuation revision transaction: %w", err)
	}
	defer tx.Rollback()

	var latestHash string
	lookupErr := tx.QueryRowContext(ctx, "SELECT revision_hash FROM market_data_valuation_versions WHERE request_fingerprint=? AND source=? ORDER BY recorded_at DESC, id DESC LIMIT 1 FOR UPDATE",
		fingerprint, stored.Source).Scan(&latestHash)
	if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
		return fmt.Errorf("read latest valuation revision for %s: %w", stored.Source, lookupErr)
	}
	if shouldRecordRevision(latestHash, lookupErr == nil, revisionHash) {
		_, err = tx.ExecContext(ctx, "INSERT INTO market_data_valuation_versions (request_fingerprint,source,revision_hash,recorded_at,fetched_at,quality_status,metadata) VALUES (?,?,?,UTC_TIMESTAMP(6),?,?,?)",
			fingerprint, stored.Source, revisionHash,
			stored.FetchedAt.UTC().Format("2006-01-02 15:04:05.999999"), string(stored.QualityStatus), metadata)
		if err != nil {
			return fmt.Errorf("record valuation revision for %s: %w", stored.Source, err)
		}
	}
	return tx.Commit()
}

// ListValuationsAsOf 返回系统观察截止时点前、每个 Provider 当时最新的同一请求快照。
func (s *Store) ListValuationsAsOf(ctx context.Context, symbols []string, asOf time.Time) ([]model.ValuationSnapshot, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("MariaDB valuation revision storage disabled")
	}
	if asOf.IsZero() {
		return nil, errors.New("valuation as_of timestamp must not be zero")
	}
	fingerprint, err := valuationRequestFingerprint(symbols)
	if err != nil {
		return nil, err
	}
	asOfText := asOf.UTC().Format("2006-01-02 15:04:05.999999")
	rows, err := s.db.QueryContext(ctx, "SELECT v.metadata, DATE_FORMAT(v.recorded_at, '%Y-%m-%d %H:%i:%s.%f') FROM market_data_valuation_versions AS v WHERE v.request_fingerprint=? AND v.recorded_at<=? AND NOT EXISTS (SELECT 1 FROM market_data_valuation_versions AS newer WHERE newer.request_fingerprint=v.request_fingerprint AND newer.source=v.source AND newer.recorded_at<=? AND (newer.recorded_at>v.recorded_at OR (newer.recorded_at=v.recorded_at AND newer.id>v.id))) ORDER BY v.recorded_at DESC, v.source ASC",
		fingerprint, asOfText, asOfText)
	if err != nil {
		return nil, fmt.Errorf("query valuation revisions as of %s: %w", asOfText, err)
	}
	defer rows.Close()
	out := []model.ValuationSnapshot{}
	for rows.Next() {
		var metadata []byte
		var recordedAtText string
		if err := rows.Scan(&metadata, &recordedAtText); err != nil {
			return nil, err
		}
		var snapshot model.ValuationSnapshot
		if err := json.Unmarshal(metadata, &snapshot); err != nil {
			return nil, fmt.Errorf("decode valuation revision metadata: %w", err)
		}
		recordedAt, err := time.ParseInLocation("2006-01-02 15:04:05.000000", recordedAtText, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("parse valuation record timestamp: %w", err)
		}
		snapshot.SystemObservedAt = &recordedAt
		out = append(out, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) ListFundamentalsAsOf(ctx context.Context, symbol string, year, quarter int, asOf time.Time) ([]model.FundamentalsSnapshot, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("MariaDB fundamentals revision storage disabled")
	}
	if asOf.IsZero() {
		return nil, errors.New("fundamentals as_of timestamp must not be zero")
	}
	if year < 2000 || quarter < 1 || quarter > 4 {
		return nil, errors.New("fundamentals report period is invalid")
	}
	asOfText := asOf.UTC().Format("2006-01-02 15:04:05.999999")
	rows, err := s.db.QueryContext(ctx, `SELECT v.metadata, DATE_FORMAT(v.recorded_at, '%Y-%m-%d %H:%i:%s.%f')
FROM market_data_fundamental_versions AS v
WHERE v.symbol=? AND v.report_year=? AND v.report_quarter=? AND v.recorded_at<=?
  AND NOT EXISTS (
    SELECT 1 FROM market_data_fundamental_versions AS newer
    WHERE newer.symbol=v.symbol AND newer.report_year=v.report_year AND newer.report_quarter=v.report_quarter
      AND newer.source=v.source AND newer.recorded_at<=?
      AND (newer.recorded_at>v.recorded_at OR (newer.recorded_at=v.recorded_at AND newer.id>v.id))
  )
ORDER BY v.recorded_at DESC, v.source ASC`, symbol, year, quarter, asOfText, asOfText)
	if err != nil {
		return nil, fmt.Errorf("query fundamentals revisions as of %s: %w", asOfText, err)
	}
	defer rows.Close()
	out := []model.FundamentalsSnapshot{}
	for rows.Next() {
		var metadata []byte
		var recordedAtText string
		if err := rows.Scan(&metadata, &recordedAtText); err != nil {
			return nil, err
		}
		var snapshot model.FundamentalsSnapshot
		if err := json.Unmarshal(metadata, &snapshot); err != nil {
			return nil, fmt.Errorf("decode fundamentals revision metadata: %w", err)
		}
		snapshot.NormalizeInstantsUTC()
		recordedAt, err := time.ParseInLocation("2006-01-02 15:04:05.000000", recordedAtText, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("parse fundamentals record timestamp: %w", err)
		}
		snapshot.SystemObservedAt = &recordedAt
		out = append(out, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
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
		if err := unmarshalDailyBarMetadata(meta, &b); err != nil {
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
