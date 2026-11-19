package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"

	marketdatabootstrap "example.com/stock-analysis-flow/internal/marketdata/bootstrap"
	marketdatacache "example.com/stock-analysis-flow/internal/marketdata/cache"
	marketdatarepository "example.com/stock-analysis-flow/internal/marketdata/repository"
	marketdataservice "example.com/stock-analysis-flow/internal/marketdata/service"
	platformfault "example.com/stock-analysis-flow/internal/platform/fault"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

// TestRegistryMariaDBIntegration 验证 Server Registry 与 Platform Runner 的真实组合行为。
func TestRegistryMariaDBIntegration(t *testing.T) {
	dsn := os.Getenv("SERVER_RELAY_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set SERVER_RELAY_TEST_MYSQL_DSN to run the Server migration integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adminDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open MariaDB admin connection: %v", err)
	}
	if err := adminDB.PingContext(ctx); err != nil {
		t.Fatalf("ping MariaDB: %v", err)
	}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	testDatabase := "server_it_" + suffix
	namespace := "server_it_" + suffix
	table := "server_it_records_" + suffix
	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE `%s`", testDatabase)); err != nil {
		t.Fatalf("create isolated MariaDB database: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = adminDB.ExecContext(cleanupCtx, fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", testDatabase))
	})

	config, err := mysqlDriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse MariaDB DSN: %v", err)
	}
	config.DBName = testDatabase
	connector, err := mysqlDriver.NewConnector(config)
	if err != nil {
		t.Fatalf("create isolated MariaDB connector: %v", err)
	}
	db := sql.OpenDB(connector)
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping isolated MariaDB database: %v", err)
	}

	registry := NewRegistry()
	if err := registry.Register(namespace, []platformstorage.Migration{{
		Version:     1,
		Description: "create Server registry integration table",
		Statements: []string{
			fmt.Sprintf("CREATE TABLE %s (id BIGINT PRIMARY KEY, value VARCHAR(32) NOT NULL) ENGINE=InnoDB", table),
		},
	}}); err != nil {
		t.Fatalf("register Server migration: %v", err)
	}
	if err := registry.Register(marketdatabootstrap.MigrationNamespace, marketdatabootstrap.MigrationDefinitions()); err != nil {
		t.Fatalf("register Data migrations: %v", err)
	}

	report, err := registry.RunWithReport(ctx, db)
	if err != nil {
		t.Fatalf("RunWithReport: %v", err)
	}
	if report.Status != "succeeded" || len(report.Namespaces) != 2 {
		t.Fatalf("unexpected migration report: %+v", report)
	}
	completedNamespaces := make(map[string]bool, len(report.Namespaces))
	for _, item := range report.Namespaces {
		if item.Status != "succeeded" {
			t.Fatalf("migration namespace did not succeed: %+v", item)
		}
		completedNamespaces[item.Namespace] = true
	}
	if !completedNamespaces[namespace] || !completedNamespaces[marketdatabootstrap.MigrationNamespace] {
		t.Fatalf("migration report omitted a registered namespace: %+v", report.Namespaces)
	}

	if err := registry.ValidateApplied(ctx, db); err != nil {
		t.Fatalf("ValidateApplied rejected the applied schema: %v", err)
	}
	dataStore, err := marketdatarepository.NewWithBorrowedDB(db)
	if err != nil {
		t.Fatalf("create Data Store for readiness check: %v", err)
	}
	dataService := marketdataservice.NewWithCapabilities(
		nil, nil, nil, marketdatacache.NewDisabled(), dataStore,
		time.Minute, time.Minute, 1, 100, marketdataservice.ResilienceConfig{}, nil,
	)
	if err := dataService.Ready(ctx); err != nil {
		t.Fatalf("Data readiness rejected the freshly migrated schema: %v", err)
	}
	if _, err := db.ExecContext(ctx, "ALTER TABLE market_data_quotes DROP COLUMN turnover_rate"); err != nil {
		t.Fatalf("simulate physical schema drift: %v", err)
	}
	if err := dataService.Ready(ctx); err == nil {
		t.Fatal("Data readiness accepted a missing V4 compatibility column")
	}
	if _, err := db.ExecContext(ctx, "ALTER TABLE market_data_quotes ADD COLUMN turnover_rate DECIMAL(16,8) NULL"); err != nil {
		t.Fatalf("restore compatibility column: %v", err)
	}
	if err := dataService.Ready(ctx); err != nil {
		t.Fatalf("Data readiness rejected restored schema: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DROP INDEX idx_market_bar_revision_identity_asof ON market_data_daily_bar_versions"); err != nil {
		t.Fatalf("simulate physical index drift: %v", err)
	}
	if err := dataService.Ready(ctx); err == nil {
		t.Fatal("Data readiness accepted a missing V4 compatibility index")
	}
	if _, err := db.ExecContext(ctx, "CREATE INDEX idx_market_bar_revision_identity_asof ON market_data_daily_bar_versions (symbol, trade_date, period, adjustment_basis, source, recorded_at, id)"); err != nil {
		t.Fatalf("restore compatibility index: %v", err)
	}
	if err := dataService.Ready(ctx); err != nil {
		t.Fatalf("Data readiness rejected restored compatibility index: %v", err)
	}

	if _, err := db.ExecContext(ctx,
		fmt.Sprintf("INSERT INTO %s (id, value) VALUES (?, ?)", table), 1, "server-registry"); err != nil {
		t.Fatalf("insert migrated table: %v", err)
	}
	var value string
	if err := db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT value FROM %s WHERE id = ?", table), 1).Scan(&value); err != nil {
		t.Fatalf("read migrated table: %v", err)
	}
	if value != "server-registry" {
		t.Fatalf("MariaDB round trip = %q, want server-registry", value)
	}
	if _, err := db.ExecContext(ctx, "UPDATE platform_schema_migrations SET checksum = REPEAT('0', 64) WHERE namespace = ? AND version = 1", namespace); err != nil {
		t.Fatalf("tamper migration checksum: %v", err)
	}
	if err := registry.ValidateApplied(ctx, db); err == nil {
		t.Fatal("ValidateApplied() accepted a changed migration checksum")
	}
}

// TestRegistryMariaDBPartialDDLFailureRequiresRecovery verifies the real
// MariaDB behavior when an earlier DDL statement succeeds but a later
// statement in the same migration fails. Server must report the Platform
// recovery classification and must not retry or mark the schema ready.
func TestRegistryMariaDBPartialDDLFailureRequiresRecovery(t *testing.T) {
	dsn := os.Getenv("SERVER_RELAY_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set SERVER_RELAY_TEST_MYSQL_DSN to run the Server migration integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adminDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open MariaDB admin connection: %v", err)
	}
	defer adminDB.Close()
	if err := adminDB.PingContext(ctx); err != nil {
		t.Fatalf("ping MariaDB: %v", err)
	}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	testDatabase := "server_fail_it_" + suffix
	namespace := "server_fail_it_" + suffix
	table := "server_partial_ddl_" + suffix
	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE `%s`", testDatabase)); err != nil {
		t.Fatalf("create isolated MariaDB database: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = adminDB.ExecContext(cleanupCtx, fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", testDatabase))
	})

	config, err := mysqlDriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse MariaDB DSN: %v", err)
	}
	config.DBName = testDatabase
	connector, err := mysqlDriver.NewConnector(config)
	if err != nil {
		t.Fatalf("create isolated MariaDB connector: %v", err)
	}
	db := sql.OpenDB(connector)
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping isolated MariaDB database: %v", err)
	}

	registry := NewRegistry()
	if err := registry.Register(namespace, []platformstorage.Migration{{
		Version:     1,
		Description: "exercise partial DDL failure recovery",
		Statements: []string{
			fmt.Sprintf("CREATE TABLE %s (id BIGINT PRIMARY KEY) ENGINE=InnoDB", table),
			"THIS IS NOT VALID SQL",
		},
	}}); err != nil {
		t.Fatalf("register failing migration: %v", err)
	}

	report, runErr := registry.RunWithReport(ctx, db)
	if runErr == nil {
		t.Fatal("RunWithReport() error = nil, want statement failure")
	}
	if report.Status != "failed" || len(report.Namespaces) != 1 {
		t.Fatalf("unexpected failure report: %+v", report)
	}
	item := report.Namespaces[0]
	if item.Namespace != namespace || item.Status != "failed" ||
		item.ReasonCode != "platform.storage.migration_apply_failed" || !item.RecoveryRequired {
		t.Fatalf("unexpected partial DDL failure result: %+v", item)
	}

	var tableCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?",
		table).Scan(&tableCount); err != nil {
		t.Fatalf("inspect partial DDL table: %v", err)
	}
	if tableCount != 1 {
		t.Fatalf("first DDL table count = %d, want 1 after partial failure", tableCount)
	}

	var status string
	var failureStatement uint32
	if err := db.QueryRowContext(ctx,
		"SELECT status, failure_statement FROM platform_schema_migrations WHERE namespace = ? AND version = 1",
		namespace).Scan(&status, &failureStatement); err != nil {
		t.Fatalf("read failed migration history: %v", err)
	}
	if status != "failed" || failureStatement != 2 {
		t.Fatalf("migration history = (%q, %d), want (failed, 2)", status, failureStatement)
	}

	if err := registry.ValidateApplied(ctx, db); err == nil {
		t.Fatal("ValidateApplied() error = nil, want incomplete migration rejection")
	}

	retryReport, retryErr := registry.RunWithReport(ctx, db)
	if retryErr == nil {
		t.Fatal("retry RunWithReport() error = nil, want manual recovery gate")
	}
	if retryReport.Status != "failed" || len(retryReport.Namespaces) != 1 {
		t.Fatalf("unexpected retry report: %+v", retryReport)
	}
	retryItem := retryReport.Namespaces[0]
	if retryItem.ReasonCode != "platform.storage.migration_requires_recovery" || !retryItem.RecoveryRequired {
		t.Fatalf("retry result = %+v, want migration_requires_recovery", retryItem)
	}
	var platformErr *platformfault.Fault
	if !errors.As(retryErr, &platformErr) || platformErr.Code != "platform.storage.migration_requires_recovery" {
		t.Fatalf("retry error = %v, want Platform migration_requires_recovery Fault", retryErr)
	}
}

// TestMarketDataMigrationUpgradeAndConcurrentRootRegistration exercises the
// published V1-V3 history, legacy compatibility DDL, and two Root instances.
func TestMarketDataMigrationUpgradeAndConcurrentRootRegistration(t *testing.T) {
	dsn := os.Getenv("SERVER_RELAY_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set SERVER_RELAY_TEST_MYSQL_DSN to run the Data migration integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open MariaDB admin connection: %v", err)
	}
	defer adminDB.Close()
	if err := adminDB.PingContext(ctx); err != nil {
		t.Fatalf("ping MariaDB: %v", err)
	}
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	testDatabase := "market_data_it_" + suffix
	databaseCreated := false
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if databaseCreated {
			_, _ = adminDB.ExecContext(cleanupCtx, fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", testDatabase))
		}
		_ = adminDB.Close()
	})
	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE `%s`", testDatabase)); err != nil {
		t.Fatalf("create isolated MariaDB database: %v", err)
	}
	databaseCreated = true
	config, err := mysqlDriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse MariaDB DSN: %v", err)
	}
	config.DBName = testDatabase
	connector, err := mysqlDriver.NewConnector(config)
	if err != nil {
		t.Fatalf("create isolated MariaDB connector: %v", err)
	}
	db := sql.OpenDB(connector)
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping isolated MariaDB database: %v", err)
	}
	definitions := marketdatabootstrap.MigrationDefinitions()
	if len(definitions) != 4 {
		t.Fatalf("Data migration definition count = %d, want 4", len(definitions))
	}
	if err := platformstorage.ApplyMigrations(ctx, db, marketdatabootstrap.MigrationNamespace, definitions[:3]); err != nil {
		t.Fatalf("apply published Data migrations V1-V3: %v", err)
	}
	// Recreate the deployed legacy state: two missing columns and old unique
	// history indexes that must be replaced by non-unique temporal indexes.
	for _, statement := range []string{
		"ALTER TABLE market_data_quotes DROP COLUMN volume_ratio",
		"ALTER TABLE market_data_quotes DROP COLUMN turnover_rate",
		"CREATE UNIQUE INDEX uq_market_bar_revision_content ON market_data_daily_bar_versions (symbol, trade_date, period, adjustment_basis, source, revision_hash)",
		"CREATE UNIQUE INDEX uq_market_fundamental_revision_content ON market_data_fundamental_versions (symbol, report_year, report_quarter, source, revision_hash)",
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("prepare legacy Data schema (%s): %v", statement, err)
		}
	}
	registries := []*Registry{NewRegistry(), NewRegistry()}
	for _, registry := range registries {
		if err := registry.Register(marketdatabootstrap.MigrationNamespace, definitions); err != nil {
			t.Fatalf("register complete Data migration history: %v", err)
		}
	}
	start := make(chan struct{})
	results := make(chan error, len(registries))
	var workers sync.WaitGroup
	for _, registry := range registries {
		workers.Add(1)
		go func(registry *Registry) {
			defer workers.Done()
			<-start
			_, runErr := registry.RunWithReport(ctx, db)
			results <- runErr
		}(registry)
	}
	close(start)
	workers.Wait()
	close(results)
	for runErr := range results {
		if runErr != nil {
			t.Fatalf("concurrent Root migration failed: %v", runErr)
		}
	}
	if err := registries[0].ValidateApplied(ctx, db); err != nil {
		t.Fatalf("read-only Data migration validation failed: %v", err)
	}
	for _, column := range []string{"volume_ratio", "turnover_rate"} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='market_data_quotes' AND COLUMN_NAME=?", column).Scan(&count); err != nil || count != 1 {
			t.Fatalf("compatibility column %s missing: count=%d err=%v", column, count, err)
		}
	}
	for _, index := range []string{"uq_market_bar_revision_content", "uq_market_fundamental_revision_content"} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND INDEX_NAME=?", index).Scan(&count); err != nil || count != 0 {
			t.Fatalf("legacy unique index %s remains: count=%d err=%v", index, count, err)
		}
	}
	for _, index := range []string{"idx_market_bar_revision_identity_asof", "idx_market_fundamental_identity_asof"} {
		var nonUnique int
		if err := db.QueryRowContext(ctx, "SELECT MAX(NON_UNIQUE) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND INDEX_NAME=?", index).Scan(&nonUnique); err != nil || nonUnique != 1 {
			t.Fatalf("temporal history index %s is not non-unique: non_unique=%d err=%v", index, nonUnique, err)
		}
	}
}
