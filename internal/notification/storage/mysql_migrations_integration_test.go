package storage

import (
	"context"
	"os"
	"testing"
	"time"

	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

func TestNotificationMariaDBMigrationOrderAndIdempotencyIntegration(t *testing.T) {
	dsn := os.Getenv("NOTIFICATION_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NOTIFICATION_MARIADB_TEST_DSN to run Notification MariaDB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := openNotificationIntegrationDB(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	migrations := Migrations()
	if len(migrations) != 6 {
		t.Fatalf("expected six immutable Notification migrations, got %d", len(migrations))
	}
	for index, migration := range migrations {
		expected := uint64(index + 1)
		if migration.Version != expected {
			t.Fatalf("migration order is not contiguous: index=%d version=%d expected=%d", index, migration.Version, expected)
		}
	}
	if err := platformstorage.ApplyMigrations(ctx, db, "notification", migrations); err != nil {
		t.Fatalf("first migration application failed: %v", err)
	}
	if _, err := platformstorage.ApplyMigrations(ctx, db, "notification", migrations); err != nil {
		t.Fatalf("second migration application must be idempotent: %v", err)
	}

	rows, err := db.QueryContext(ctx, "SELECT version, status, checksum FROM platform_schema_migrations WHERE namespace = ? ORDER BY version", "notification")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var version uint64
		var status, checksum string
		if err := rows.Scan(&version, &status, &checksum); err != nil {
			t.Fatal(err)
		}
		count++
		if version != uint64(count) || status != "applied" || len(checksum) != 64 {
			t.Fatalf("unexpected migration history row: version=%d status=%s checksum=%s", version, status, checksum)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != len(migrations) {
		t.Fatalf("expected %d applied migrations, got %d", len(migrations), count)
	}
}
