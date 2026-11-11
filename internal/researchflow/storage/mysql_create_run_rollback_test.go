package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
	"example.com/stock-analysis-flow/internal/researchflow/application"
	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

// TestMariaDBCreateRunAtomicRollsBackPartialWrites injects a Stage insert
// failure after the Run row has been inserted and verifies full rollback.
func TestMariaDBCreateRunAtomicRollsBackPartialWrites(t *testing.T) {
	if os.Getenv("PLATFORM_STORAGE_INTEGRATION") != "1" {
		t.Skip("set PLATFORM_STORAGE_INTEGRATION=1 to run Researchflow MariaDB integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true",
		os.Getenv("PLATFORM_MARIADB_USER"), os.Getenv("PLATFORM_MARIADB_PASSWORD"),
		os.Getenv("PLATFORM_MARIADB_ADDR"), os.Getenv("PLATFORM_MARIADB_DATABASE"))
	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	connector, err := mysql.NewConnector(config)
	if err != nil {
		t.Fatal(err)
	}
	db, err := platformstorage.OpenSQL(ctx, connector, platformstorage.DefaultSQLConfig())
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer db.Close()
	if err := platformstorage.ApplyMigrations(ctx, db, MigrationNamespace, Migrations()); err != nil {
		t.Fatalf("apply Researchflow migrations: %v", err)
	}
	repository, err := NewMySQLRunRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	runRef := "rollback-create-run-" + suffix
	stageRef := "rollback-create-stage-" + suffix
	triggerName := "researchflow_fail_stage_" + suffix
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER `+triggerName+` BEFORE INSERT ON researchflow_stages
		FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'forced Researchflow Stage insert failure'`); err != nil {
		t.Fatalf("create Stage failure trigger: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "DROP TRIGGER IF EXISTS "+triggerName)
	}()

	now := time.Now().UTC()
	fingerprint := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	templateFingerprint := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	record := application.CreateRunRecord{
		Scope: application.IdempotencyScope{
			CallerRef: "researchflow-create-rollback-test", Operation: "CreateRun", Key: "rollback-create-" + suffix,
		},
		Fingerprint: fingerprint,
		Run: domain.Run{
			Ref: runRef, SchemaVersion: "run-v1", Status: domain.RunCreated, Version: 1,
			SystemAsOf: now.Add(-time.Minute), DeadlineAt: now.Add(5 * time.Minute),
			TemplateRef: "rollback-create-template", TemplateVersion: "v1",
			TemplateFingerprint: templateFingerprint, InputSnapshotRefs: []string{},
			CorrelationID: "rollback-create-correlation-" + suffix,
		},
		Stages: []domain.Stage{{
			Ref: stageRef, RunRef: runRef, Code: "context", Status: domain.StagePending,
			Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
			DiagnosticRefs: []string{}, ReasonCodes: []string{},
		}},
		ResolvedTemplate: template.ResolvedTemplate{
			Ref: "rollback-create-template", Version: "v1", Fingerprint: templateFingerprint,
			Stages: []template.ResolvedStage{{Definition: template.StageDefinition{Code: "context"}}},
		},
		Parameters: json.RawMessage(`{}`),
	}
	if _, err := repository.CreateRunAtomic(ctx, record); err == nil {
		t.Fatal("CreateRunAtomic succeeded despite forced Stage insert failure")
	}

	for _, check := range []struct {
		name  string
		query string
		args  []any
	}{
		{name: "Run", query: "SELECT COUNT(*) FROM researchflow_runs WHERE run_ref = ?", args: []any{runRef}},
		{name: "Stage", query: "SELECT COUNT(*) FROM researchflow_stages WHERE stage_ref = ?", args: []any{stageRef}},
		{name: "idempotency", query: "SELECT COUNT(*) FROM researchflow_run_idempotency WHERE caller_ref = ? AND operation = ? AND idempotency_key = ?", args: []any{record.Scope.CallerRef, record.Scope.Operation, record.Scope.Key}},
	} {
		var count int
		if err := db.QueryRowContext(ctx, check.query, check.args...).Scan(&count); err != nil {
			t.Fatalf("count %s after rollback: %v", check.name, err)
		}
		if count != 0 {
			t.Errorf("%s rows after failed CreateRunAtomic = %d, want 0", check.name, count)
		}
	}
}
