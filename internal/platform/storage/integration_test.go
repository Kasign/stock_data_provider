package storage

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

	"github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"

	"example.com/stock-analysis-flow/internal/platform/fault"
)

func requireStorageIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("PLATFORM_STORAGE_INTEGRATION") != "1" {
		t.Skip("set PLATFORM_STORAGE_INTEGRATION=1 to run real MariaDB/Redis integration tests")
	}
}

func openIntegrationSQL(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s)/%s?parseTime=true",
		os.Getenv("PLATFORM_MARIADB_USER"),
		os.Getenv("PLATFORM_MARIADB_PASSWORD"),
		os.Getenv("PLATFORM_MARIADB_ADDR"),
		os.Getenv("PLATFORM_MARIADB_DATABASE"),
	)
	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	connector, err := mysql.NewConnector(config)
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenSQL(ctx, connector, DefaultSQLConfig())
	if err != nil {
		t.Fatalf("OpenSQL against MariaDB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func TestMariaDBStorageIntegration(t *testing.T) {
	requireStorageIntegration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db := openIntegrationSQL(t, ctx)

	if err := PingSQL(ctx, db); err != nil {
		t.Fatal(err)
	}
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	namespace := "platform_it_" + suffix
	table := "platform_it_records_" + suffix
	migrations := []Migration{{
		Version: 1,
		Description: "create integration record table",
		Statements: []string{fmt.Sprintf(
			"CREATE TABLE %s (id BIGINT PRIMARY KEY, value VARCHAR(32) NOT NULL) ENGINE=InnoDB",
			table,
		)},
	}}

	var wait sync.WaitGroup
	errorsByCall := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errorsByCall <- ApplyMigrations(ctx, db, namespace, migrations)
		}()
	}
	wait.Wait()
	close(errorsByCall)
	for err := range errorsByCall {
		if err != nil {
			t.Fatalf("concurrent ApplyMigrations failed: %v", err)
		}
	}

	if _, err := db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (id, value) VALUES (?, ?)", table), 1, "persisted"); err != nil {
		t.Fatal(err)
	}
	var value string
	if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT value FROM %s WHERE id = ?", table), 1).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "persisted" {
		t.Fatalf("MariaDB round trip = %q, want persisted", value)
	}

	var historyCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM platform_schema_migrations WHERE namespace = ? AND version = 1",
		namespace,
	).Scan(&historyCount); err != nil {
		t.Fatal(err)
	}
	if historyCount != 1 {
		t.Fatalf("migration history rows = %d, want 1", historyCount)
	}
	var lockFree sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT IS_FREE_LOCK(?)", migrationLockName(namespace)).Scan(&lockFree); err != nil {
		t.Fatal(err)
	}
	if !lockFree.Valid || lockFree.Int64 != 1 {
		t.Fatalf("migration advisory lock was not released: %+v", lockFree)
	}

	failedNamespace := namespace + "_failed"
	partialTable := "platform_it_partial_" + suffix
	failedMigration := []Migration{{
		Version: 1,
		Description: "verify partial DDL recovery",
		Statements: []string{
			fmt.Sprintf("CREATE TABLE %s (id BIGINT PRIMARY KEY) ENGINE=InnoDB", partialTable),
			"THIS IS NOT VALID SQL",
		},
	}}
	err := ApplyMigrations(ctx, db, failedNamespace, failedMigration)
	var applyFault *fault.Fault
	if !errors.As(err, &applyFault) || applyFault.Code != "platform.storage.migration_apply_failed" {
		t.Fatalf("expected migration apply fault, got %v", err)
	}
	var partialTableCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?",
		partialTable,
	).Scan(&partialTableCount); err != nil {
		t.Fatal(err)
	}
	if partialTableCount != 1 {
		t.Fatalf("first DDL statement should remain visible after MariaDB implicit commit; table count=%d", partialTableCount)
	}
	err = ApplyMigrations(ctx, db, failedNamespace, failedMigration)
	var recoveryFault *fault.Fault
	if !errors.As(err, &recoveryFault) || recoveryFault.Code != "platform.storage.migration_requires_recovery" {
		t.Fatalf("failed migration must require explicit recovery, got %v", err)
	}
}

func TestRedisStorageIntegration(t *testing.T) {
	requireStorageIntegration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client, err := OpenRedis(ctx, redis.Options{
		Addr: os.Getenv("PLATFORM_REDIS_ADDR"),
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("OpenRedis failed: %v", err)
	}
	defer client.Close()

	if err := PingRedis(ctx, client); err != nil {
		t.Fatal(err)
	}
	options := client.Options()
	if options.PoolSize != defaultRedisPoolSize ||
		options.MaxActiveConns != maxRedisPoolSize ||
		options.MaxIdleConns != defaultRedisPoolSize {
		t.Fatalf("unexpected normalized Redis pool settings: pool=%d active=%d idle=%d",
			options.PoolSize, options.MaxActiveConns, options.MaxIdleConns)
	}

	key := "platform:integration:" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := client.Set(ctx, key, "redis-round-trip", 5*time.Second).Err(); err != nil {
		t.Fatal(err)
	}
	defer client.Del(context.Background(), key)
	value, err := client.Get(ctx, key).Result()
	if err != nil {
		t.Fatal(err)
	}
	if value != "redis-round-trip" {
		t.Fatalf("Redis round trip = %q, want redis-round-trip", value)
	}
}
