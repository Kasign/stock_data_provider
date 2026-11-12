package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"

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
	defer adminDB.Close()
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
		t.Fatalf("register migration: %v", err)
	}

	report, err := registry.RunWithReport(ctx, db)
	if err != nil {
		t.Fatalf("RunWithReport: %v", err)
	}
	if report.Status != "succeeded" || len(report.Namespaces) != 1 {
		t.Fatalf("unexpected migration report: %+v", report)
	}
	if item := report.Namespaces[0]; item.Namespace != namespace || item.Status != "succeeded" {
		t.Fatalf("unexpected namespace result: %+v", item)
	}

	if err := registry.ValidateApplied(ctx, db); err != nil {
		t.Fatalf("ValidateApplied rejected the applied schema: %v", err)
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
