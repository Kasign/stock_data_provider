package migration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"

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
