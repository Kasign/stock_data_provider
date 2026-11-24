package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
	"example.com/stock-analysis-flow/internal/marketdata/repository"
)

// openMigratedMarketDataTestStore applies the same immutable Data baseline
// through Platform's Migration Runner; tests must not call a Store-owned Migrate.
func openMigratedMarketDataTestStore(ctx context.Context, dsn string) (*repository.Store, func() error, error) {
	if ctx == nil || dsn == "" {
		return nil, nil, fmt.Errorf("MarketData test context and DSN are required")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("open MariaDB: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(3)
	db.SetConnMaxLifetime(5 * time.Minute)
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = db.PingContext(pingCtx)
	cancel()
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("ping MariaDB: %w", err)
	}
	if err := platformstorage.ApplyMigrations(ctx, db, repository.MigrationNamespace, repository.MigrationDefinitions()); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("apply Data baseline: %w", err)
	}
	store, err := repository.NewWithBorrowedDB(db)
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("create MarketData repository: %w", err)
	}
	return store, db.Close, nil
}
