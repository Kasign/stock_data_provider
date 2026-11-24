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
	db, err := openNotificationIntegrationDB(t, ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	migrations := Migrations()
	if len(migrations) != 1 || migrations[0].Version != 1 {
		t.Fatalf("expected one complete Notification V1 baseline, got %#v", migrations)
	}
	if err := platformstorage.ApplyMigrations(ctx, db, "notification", migrations); err != nil {
		t.Fatalf("first migration application failed: %v", err)
	}
	if err := platformstorage.ApplyMigrations(ctx, db, "notification", migrations); err != nil {
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
	if count != 1 {
		t.Fatalf("expected one applied baseline migration, got %d", count)
	}
	var currentRevision uint64
	if err := db.QueryRowContext(ctx, "SELECT current_revision FROM notification_subscription_revision WHERE singleton_id = 1").Scan(&currentRevision); err != nil {
		t.Fatalf("read Notification revision seed: %v", err)
	}
	if currentRevision != 0 {
		t.Fatalf("initial subscription revision = %d, want 0", currentRevision)
	}
	var redundantIndexCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics
		WHERE table_schema = DATABASE() AND table_name = 'notification_channel_config_versions'
		AND index_name = 'idx_notification_channel_config_kind'`).Scan(&redundantIndexCount); err != nil {
		t.Fatalf("inspect removed Notification index: %v", err)
	}
	if redundantIndexCount != 0 {
		t.Fatal("redundant Notification channel config index was recreated")
	}
}
