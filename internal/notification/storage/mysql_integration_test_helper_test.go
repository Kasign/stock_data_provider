package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"

	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

// openNotificationIntegrationDB 为每个测试创建独立 MariaDB Schema，避免全局 ClaimDue
// 在不同测试遗留的 Delivery/FanoutWorkItem 之间串读。测试 DSN 用户需具备 CREATE/DROP DATABASE 权限。
func openNotificationIntegrationDB(t *testing.T, ctx context.Context, dsn string) (*sql.DB, error) {
	if t == nil || ctx == nil || dsn == "" {
		return nil, errors.New("notification storage: integration test, context and DSN are required")
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	if config.DBName == "" {
		return nil, errors.New("notification storage: integration DSN must include a base database name")
	}

	sum := sha256.Sum256([]byte(t.Name() + fmt.Sprint(time.Now().UnixNano())))
	databaseName := "ntf_it_" + hex.EncodeToString(sum[:8])
	adminConfig := *config
	adminConfig.DBName = ""
	adminConnector, err := mysql.NewConnector(&adminConfig)
	if err != nil {
		return nil, err
	}
	adminDB, err := platformstorage.OpenSQL(ctx, adminConnector, platformstorage.DefaultSQLConfig())
	if err != nil {
		return nil, fmt.Errorf("notification storage: open MariaDB admin connection: %w", err)
	}
	if _, err := adminDB.ExecContext(ctx, "CREATE DATABASE `"+databaseName+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		_ = adminDB.Close()
		return nil, fmt.Errorf("notification storage: create isolated MariaDB schema: %w", err)
	}

	testConfig := *config
	testConfig.DBName = databaseName
	testConnector, err := mysql.NewConnector(&testConfig)
	if err != nil {
		_, _ = adminDB.ExecContext(ctx, "DROP DATABASE IF EXISTS `"+databaseName+"`")
		_ = adminDB.Close()
		return nil, err
	}
	db, err := platformstorage.OpenSQL(ctx, testConnector, platformstorage.DefaultSQLConfig())
	if err != nil {
		_, _ = adminDB.ExecContext(ctx, "DROP DATABASE IF EXISTS `"+databaseName+"`")
		_ = adminDB.Close()
		return nil, fmt.Errorf("notification storage: open isolated MariaDB schema: %w", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, dropErr := adminDB.ExecContext(cleanupCtx, "DROP DATABASE IF EXISTS `"+databaseName+"`"); dropErr != nil {
			t.Errorf("notification storage: drop isolated MariaDB schema %s: %v", databaseName, dropErr)
		}
		if closeErr := adminDB.Close(); closeErr != nil {
			t.Errorf("notification storage: close MariaDB admin connection: %v", closeErr)
		}
	})
	return db, nil
}
