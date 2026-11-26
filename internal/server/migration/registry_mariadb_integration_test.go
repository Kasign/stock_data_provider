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
		var readinessFault *platformfault.Fault
		if errors.As(err, &readinessFault) && readinessFault.Cause != nil {
			t.Fatalf("Data readiness rejected the freshly migrated schema: %v (internal cause: %v)", err, readinessFault.Cause)
		}
		t.Fatalf("Data readiness rejected the freshly migrated schema: %v", err)
	}
	if _, err := db.ExecContext(ctx, "ALTER TABLE market_data_quotes DROP COLUMN turnover_rate"); err != nil {
		t.Fatalf("simulate physical schema drift: %v", err)
	}
	if err := registry.ValidateApplied(ctx, db); err != nil {
		t.Fatalf("migration checksum validation should not treat physical column drift as a changed migration definition: %v", err)
	}
	requireMarketDataSchemaDrift(t, dataService.Ready(ctx), "missing V4 compatibility column")
	if _, err := db.ExecContext(ctx, "ALTER TABLE market_data_quotes ADD COLUMN turnover_rate DECIMAL(16,8) UNSIGNED NULL"); err != nil {
		t.Fatalf("simulate incompatible unsigned compatibility column: %v", err)
	}
	if err := registry.ValidateApplied(ctx, db); err != nil {
		t.Fatalf("migration checksum validation should not treat column type drift as a changed migration definition: %v", err)
	}
	requireMarketDataSchemaDrift(t, dataService.Ready(ctx), "unsigned V4 compatibility column")
	if _, err := db.ExecContext(ctx, "ALTER TABLE market_data_quotes MODIFY COLUMN turnover_rate DECIMAL(16,8) NULL"); err != nil {
		t.Fatalf("restore exact compatibility column definition: %v", err)
	}
	if err := dataService.Ready(ctx); err != nil {
		t.Fatalf("Data readiness rejected restored schema: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DROP INDEX idx_market_bar_revision_identity_asof ON market_data_daily_bar_versions"); err != nil {
		t.Fatalf("simulate physical index drift: %v", err)
	}
	if err := registry.ValidateApplied(ctx, db); err != nil {
		t.Fatalf("migration checksum validation should not treat physical index drift as a changed migration definition: %v", err)
	}
	requireMarketDataSchemaDrift(t, dataService.Ready(ctx), "missing V4 compatibility index")
	if _, err := db.ExecContext(ctx, "CREATE INDEX idx_market_bar_revision_identity_asof ON market_data_daily_bar_versions (symbol(3), trade_date, period, adjustment_basis, source, recorded_at, id)"); err != nil {
		t.Fatalf("simulate prefix compatibility index: %v", err)
	}
	if err := registry.ValidateApplied(ctx, db); err != nil {
		t.Fatalf("migration checksum validation should not treat index prefix drift as a changed migration definition: %v", err)
	}
	requireMarketDataSchemaDrift(t, dataService.Ready(ctx), "prefix V4 compatibility index")
	if _, err := db.ExecContext(ctx, "DROP INDEX idx_market_bar_revision_identity_asof ON market_data_daily_bar_versions"); err != nil {
		t.Fatalf("remove prefix compatibility index: %v", err)
	}
	if _, err := db.ExecContext(ctx, "CREATE INDEX idx_market_bar_revision_identity_asof ON market_data_daily_bar_versions (symbol, trade_date, period, adjustment_basis, source, recorded_at, id)"); err != nil {
		t.Fatalf("restore exact compatibility index: %v", err)
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

func requireMarketDataSchemaDrift(t *testing.T, err error, scenario string) {
	t.Helper()
	var readinessFault *platformfault.Fault
	if !errors.As(err, &readinessFault) || readinessFault.Code != "market_data.schema_drift" {
		t.Fatalf("Data readiness error for %s = %v, want market_data.schema_drift", scenario, err)
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
	t.Cleanup(func() {
		if err := adminDB.Close(); err != nil {
			t.Errorf("close MariaDB admin connection: %v", err)
		}
	})
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

// TestMarketDataBaselineMigrationAndConcurrentRootRegistration verifies that
// a completely empty database receives the complete Data schema baseline plus
// every pending additive migration exactly once, even when two Root registries
// start concurrently.
func TestMarketDataBaselineMigrationAndConcurrentRootRegistration(t *testing.T) {
	dsn := os.Getenv("SERVER_RELAY_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set SERVER_RELAY_TEST_MYSQL_DSN to run the Data baseline migration integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	adminDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open MariaDB admin connection: %v", err)
	}
	t.Cleanup(func() {
		if err := adminDB.Close(); err != nil {
			t.Errorf("close MariaDB admin connection: %v", err)
		}
	})
	if err := adminDB.PingContext(ctx); err != nil {
		t.Fatalf("ping MariaDB: %v", err)
	}
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	testDatabase := "market_data_baseline_it_" + suffix
	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", testDatabase)); err != nil {
		t.Fatalf("create isolated empty MariaDB database: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := adminDB.ExecContext(cleanupCtx, fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", testDatabase)); err != nil {
			t.Errorf("drop isolated MariaDB database: %v", err)
		}
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
		t.Fatalf("ping isolated empty MariaDB database: %v", err)
	}

	definitions := marketdatabootstrap.MigrationDefinitions()
	// 迁移列表本身的形状（V1 完整基线 + 后续增量）由 mysql_versions_test.go 与
	// bootstrap/module_test.go 以无需 DSN 的方式守着，这里不重复硬编码版本数量：
	// 之前此处写死"恰好一条 V1"，DEC-061 新增 V2 后只有带 DSN 的 CI 才发现漂移。
	// 本测试只要求非空（否则断言会退化），具体版本集合在后面与迁移历史对齐。
	if len(definitions) == 0 {
		t.Fatal("Data migration definitions are empty")
	}
	registries := []*Registry{NewRegistry(), NewRegistry()}
	for _, registry := range registries {
		if err := registry.Register(marketdatabootstrap.MigrationNamespace, definitions); err != nil {
			t.Fatalf("register complete Data baseline: %v", err)
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
			t.Fatalf("concurrent Root baseline migration failed: %v", runErr)
		}
	}
	if err := registries[0].ValidateApplied(ctx, db); err != nil {
		t.Fatalf("read-only Data baseline validation failed: %v", err)
	}

	// 检查基线一次性创建全部当前模块表，而不是只检查 Migration History。
	requiredTables := []string{
		"market_data_provider_probe_runs",
		"market_data_sentiment_snapshots",
		"market_data_quotes",
		"market_data_daily_bars",
		"market_data_daily_bar_versions",
		"market_data_fundamental_versions",
		"market_data_security_trading_status_versions",
		"market_data_adjustment_factor_versions",
		"market_data_corporate_action_versions",
		"market_data_trading_calendar_snapshots",
		"market_data_daily_bars_close_series_snapshots",
		"market_data_authorization_policy_versions",
		"market_data_current_authorization_policy_versions",
		"market_data_authorization_decisions",
		"market_data_authorization_revocations",
		"market_data_authorization_policy_publications",
		"market_data_valuation_versions",
	}
	for _, table := range requiredTables {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?", table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("baseline table %s missing: count=%d err=%v", table, count, err)
		}
	}
	for _, item := range []struct{ table, column string }{
		{"market_data_quotes", "volume_ratio"},
		{"market_data_quotes", "turnover_rate"},
		{"market_data_daily_bars_close_series_snapshots", "snapshot_ref"},
		{"market_data_daily_bars_close_series_snapshots", "payload_kind"},
		{"market_data_authorization_decisions", "authorization_ref"},
		{"market_data_authorization_revocations", "authorization_ref"},
		{"market_data_authorization_policy_publications", "request_id"},
	} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND COLUMN_NAME=?", item.table, item.column).Scan(&count); err != nil || count != 1 {
			t.Fatalf("baseline column %s.%s missing: count=%d err=%v", item.table, item.column, count, err)
		}
	}
	for _, index := range []string{"idx_market_bar_revision_identity_asof", "idx_market_fundamental_identity_asof", "idx_market_calendar_identity_observed"} {
		var nonUnique int
		if err := db.QueryRowContext(ctx, "SELECT MAX(NON_UNIQUE) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND INDEX_NAME=?", index).Scan(&nonUnique); err != nil || nonUnique != 1 {
			t.Fatalf("baseline lookup index %s must be present and non-unique: non_unique=%d err=%v", index, nonUnique, err)
		}
	}

	// 并发启动后历史必须与已注册定义逐条对应：任何迁移都不能重复执行，
	// 也不能停留在 running/failed。期望版本集合由 definitions 推导而非写死，
	// 否则每加一条增量都要同步改这里——V2 就是因为写死 "恰好一条 V1" 而漏改，
	// 只在带 DSN 的 CI 里才暴露。
	wantVersions := make(map[uint64]bool, len(definitions))
	for _, definition := range definitions {
		wantVersions[definition.Version] = true
	}
	historyRows, err := db.QueryContext(ctx, "SELECT version, status FROM platform_schema_migrations WHERE namespace=?", marketdatabootstrap.MigrationNamespace)
	if err != nil {
		t.Fatalf("read Data migration history: %v", err)
	}
	defer historyRows.Close()
	appliedVersions := make(map[uint64]string, len(definitions))
	for historyRows.Next() {
		var version uint64
		var status string
		if err := historyRows.Scan(&version, &status); err != nil {
			t.Fatalf("scan Data migration history row: %v", err)
		}
		appliedVersions[version] = status
	}
	if err := historyRows.Err(); err != nil {
		t.Fatalf("iterate Data migration history: %v", err)
	}
	if len(appliedVersions) != len(wantVersions) {
		t.Fatalf("Data migration history versions = %v, want exactly %v", appliedVersions, wantVersions)
	}
	for version := range wantVersions {
		status, ok := appliedVersions[version]
		if !ok {
			t.Fatalf("Data migration history is missing version %d: got %v", version, appliedVersions)
		}
		if status != "applied" {
			t.Fatalf("Data migration version %d status = %q, want applied", version, status)
		}
	}
}
