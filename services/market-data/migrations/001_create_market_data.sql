-- Canonical storage contract: prices are CNY/share; volume is shares; amount is CNY.
-- adjustment_basis is part of the natural key because adjusted series must never mix.
CREATE TABLE IF NOT EXISTS market_data_quotes (
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
  PRIMARY KEY (id),
  KEY idx_market_quote_symbol_time (symbol, quote_time),
  KEY idx_market_quote_fetched (fetched_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS market_data_daily_bars (
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
  PRIMARY KEY (id),
  UNIQUE KEY uq_market_bar_source_basis (symbol, trade_date, period, adjustment_basis, source),
  KEY idx_market_bar_lookup (symbol, trade_date, adjustment_basis)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Append-only versions preserve provider revisions for point-in-time reconstruction.
-- The canonical market_data_daily_bars table remains the current convenience view.
CREATE TABLE IF NOT EXISTS market_data_daily_bar_versions (
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
  UNIQUE KEY uq_market_bar_revision_content (symbol, trade_date, period, adjustment_basis, source, revision_hash),
  KEY idx_market_bar_revision_asof (symbol, trade_date, adjustment_basis, recorded_at),
  KEY idx_market_bar_revision_observed (recorded_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
