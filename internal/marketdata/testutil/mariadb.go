package testutil

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"example.com/stock-analysis-flow/internal/marketdata/repository"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

// OpenMariaDBStore 为集成测试创建空库 Schema，并返回借用该连接池的 Data Store。
// Migration 通过 Platform Runner 执行，不调用领域 Store 的独立迁移入口。
func OpenMariaDBStore(ctx context.Context, dsn string) (*repository.Store, *sql.DB, error) {
	if ctx == nil || dsn == "" {
		return nil, nil, errors.New("MariaDB integration test context and DSN are required")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("open MariaDB: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(3)
	db.SetConnMaxLifetime(5 * time.Minute)
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("ping MariaDB: %w", err)
	}
	if err := platformstorage.ApplyMigrations(ctx, db, repository.MigrationNamespace, repository.MigrationDefinitions()); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("apply MarketData baseline: %w", err)
	}
	store, err := repository.NewWithBorrowedDB(db)
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("create MarketData repository: %w", err)
	}
	return store, db, nil
}
