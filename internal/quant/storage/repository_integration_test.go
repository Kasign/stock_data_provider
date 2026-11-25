package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"

	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
	"example.com/stock-analysis-flow/internal/quant/manifest"
	"example.com/stock-analysis-flow/internal/quant/result"
)

func openQuantIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("QUANT_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("QUANT_TEST_MYSQL_DSN is not set; skipping MariaDB integration test")
	}
	config, err := mysqlParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse QUANT_TEST_MYSQL_DSN: %v", err)
	}
	if !strings.HasSuffix(strings.ToLower(config), "_test") {
		t.Fatalf("QUANT_TEST_MYSQL_DSN database %q is unsafe; dedicated database name must end with _test", config)
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping MariaDB: %v", err)
	}
	if err := platformstorage.ApplyMigrations(ctx, db, MigrationNamespace, Migrations()); err != nil {
		t.Fatalf("apply Quant migrations: %v", err)
	}
	return db
}

// mysqlParseDSN returns the database name after parsing with the official driver.
func mysqlParseDSN(dsn string) (string, error) {
	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		return "", err
	}
	return config.DBName, nil
}

func quantStorageRecord(t *testing.T, payload string) result.Record {
	t.Helper()
	raw := json.RawMessage(payload)
	value, err := manifest.BuildWithPayload(manifest.Input{
		ResultType: "trend", Scope: "stock", SecurityID: "600000.SH",
		Period: "1d", QualityStatus: "valid", ResultStatus: "completed",
		SchemaVersion: "1.0.0", PayloadSchemaVersion: "1.0.0",
		AlgorithmVersion:     "trend.test.v1",
		ParameterFingerprint: strings.Repeat("a", 64),
		SystemAsOf:           time.Date(2026, 11, 20, 2, 0, 0, 0, time.UTC),
		InputSnapshotRefs:    []string{"data_snapshot:sha256:" + strings.Repeat("b", 64)},
	}, raw)
	if err != nil {
		t.Fatalf("build Quant manifest: %v", err)
	}
	return result.Record{Manifest: value, Payload: raw}
}

func TestMySQLRepositorySaveIsImmutableAndIdempotent(t *testing.T) {
	db := openQuantIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repository, err := NewMySQLRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	record := quantStorageRecord(t, `{"direction":"up","strength":0.7}`)
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM quant_results WHERE result_id = ?", record.Manifest.ResultID)
	})
	if err := repository.Save(ctx, record); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	var firstCreatedAt time.Time
	if err := db.QueryRowContext(ctx, "SELECT created_at FROM quant_results WHERE result_id = ?", record.Manifest.ResultID).Scan(&firstCreatedAt); err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(ctx, record); err != nil {
		t.Fatalf("idempotent Save: %v", err)
	}
	equivalent := quantStorageRecord(t, `{ "strength": 0.7, "direction": "up" }`)
	if equivalent.Manifest.ResultID != record.Manifest.ResultID {
		t.Fatal("canonical-equivalent payloads produced different result IDs")
	}
	if err := repository.Save(ctx, equivalent); err != nil {
		t.Fatalf("canonical-equivalent Save: %v", err)
	}
	var afterCreatedAt time.Time
	if err := db.QueryRowContext(ctx, "SELECT created_at FROM quant_results WHERE result_id = ?", record.Manifest.ResultID).Scan(&afterCreatedAt); err != nil {
		t.Fatal(err)
	}
	if !afterCreatedAt.Equal(firstCreatedAt) {
		t.Fatalf("idempotent replay changed created_at: before=%s after=%s", firstCreatedAt, afterCreatedAt)
	}
}

func TestMySQLRepositoryGetRecordVerifiesImmutableStoredPayload(t *testing.T) {
	db := openQuantIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repository, err := NewMySQLRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	record := quantStorageRecord(t, `{"direction":"up","strength":0.7}`)
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM quant_results WHERE result_id = ?", record.Manifest.ResultID)
	})
	if err := repository.Save(ctx, record); err != nil {
		t.Fatal(err)
	}
	got, err := repository.GetRecord(ctx, record.Manifest.ResultID)
	if err != nil {
		t.Fatalf("GetRecord() error = %v", err)
	}
	if got.Manifest.ResultID != record.Manifest.ResultID || string(got.Payload) != string(record.Payload) {
		t.Fatalf("GetRecord() returned a different immutable record: %+v", got.Manifest)
	}
	if _, err := db.ExecContext(ctx, "UPDATE quant_results SET payload_json = ? WHERE result_id = ?",
		[]byte(`{"direction":"down","strength":0.7}`), record.Manifest.ResultID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.GetRecord(ctx, record.Manifest.ResultID); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("GetRecord() after stored payload tampering = %v, want ErrCorruptRecord", err)
	}
}

func TestMySQLRepositoryConcurrentIdempotentSave(t *testing.T) {
	db := openQuantIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repository, err := NewMySQLRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	record := quantStorageRecord(t, `{"direction":"up","strength":0.7}`)
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM quant_results WHERE result_id = ?", record.Manifest.ResultID)
	})
	const writers = 16
	var wait sync.WaitGroup
	errorsFound := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errorsFound <- repository.Save(ctx, record)
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Errorf("concurrent Save: %v", err)
		}
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM quant_results WHERE result_id = ?", record.Manifest.ResultID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("stored result count = %d, want 1", count)
	}
}

func TestMySQLRepositoryRejectsCorruptExistingRow(t *testing.T) {
	db := openQuantIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repository, err := NewMySQLRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	record := quantStorageRecord(t, `{"direction":"up","strength":0.7}`)
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM quant_results WHERE result_id = ?", record.Manifest.ResultID)
	})
	if err := repository.Save(ctx, record); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE quant_results SET payload_json = ? WHERE result_id = ?", []byte(`{"direction":"down","strength":0.7}`), record.Manifest.ResultID); err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(ctx, record); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("Save after stored payload corruption = %v, want ErrCorruptRecord", err)
	}
}

func TestMySQLRepositoryHonorsCancellation(t *testing.T) {
	db := openQuantIntegrationDB(t)
	repository, err := NewMySQLRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	record := quantStorageRecord(t, `{"direction":"up","strength":0.7}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := repository.Save(ctx, record); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save with cancelled context = %v, want context.Canceled", err)
	}
}


func TestMySQLSchemaValidatorAcceptsCompleteQuantBaseline(t *testing.T) {
	db := openQuantIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := ValidateSchema(ctx, db); err != nil {
		t.Fatalf("ValidateSchema() error = %v", err)
	}
}
