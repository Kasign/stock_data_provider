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

func TestMariaDBListSchedulableRunRefsUsesKeysetAndSkipsInactiveRuns(t *testing.T) {
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
	repository, err := NewMySQLRunRepository(db, CapacityLimits{MaxQueuedRuns: 1000, MaxWorkers: 64})
	if err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	prefix := "zzzz-researchflow-scheduler-" + suffix + "-"
	now := time.Now().UTC()
	fingerprint := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	templateFingerprint := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	refs := []string{prefix + "a", prefix + "b", prefix + "c", prefix + "d", prefix + "e", prefix + "f"}
	for index, runRef := range refs {
		stageRef := "scheduler-candidate-stage-" + fmt.Sprintf("%d", index) + "-" + suffix
		record := application.CreateRunRecord{
			Scope: application.IdempotencyScope{
				CallerRef: "researchflow-scheduler-candidate-" + suffix,
				Operation: "CreateRun",
				Key:       fmt.Sprintf("scheduler-candidate-%d-%s", index, suffix),
			},
			Fingerprint: fingerprint,
			Run: domain.Run{
				Ref: runRef, SchemaVersion: "v1", Status: domain.RunCreated, Version: 1,
				SystemAsOf: now.Add(-time.Minute), DeadlineAt: now.Add(time.Hour),
				TemplateRef: "scheduler-candidate-template", TemplateVersion: "v1",
				TemplateFingerprint: templateFingerprint, InputSnapshotRefs: []string{},
				CorrelationID: "scheduler-candidate-correlation-" + suffix,
			},
			Stages: []domain.Stage{{
				Ref: stageRef, RunRef: runRef, Code: "context", Status: domain.StagePending,
				Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
				DiagnosticRefs: []string{}, ReasonCodes: []string{},
			}},
			ResolvedTemplate: template.ResolvedTemplate{
				Ref: "scheduler-candidate-template", Version: "v1", Fingerprint: templateFingerprint,
				Stages: []template.ResolvedStage{{Definition: template.StageDefinition{Code: "context"}}},
			},
			Parameters: json.RawMessage("{}"),
		}
		if _, err := repository.CreateRunAtomic(ctx, record); err != nil {
			t.Fatalf("create candidate fixture %q: %v", runRef, err)
		}
	}

	if _, err := db.ExecContext(ctx, "UPDATE researchflow_runs SET status = ? WHERE run_ref = ?",
		string(domain.RunQueued), refs[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE researchflow_runs SET status = ? WHERE run_ref = ?",
		string(domain.RunRunning), refs[2]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE researchflow_stages SET status = ? WHERE run_ref = ?",
		string(domain.StageRetryWait), refs[2]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE researchflow_runs SET status = ?, termination_reason = ? WHERE run_ref = ?",
		string(domain.RunCancelling), "user_cancelled", refs[3]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE researchflow_runs SET deadline_at = ? WHERE run_ref = ?",
		formatResearchflowTimeValue(now.Add(-time.Minute)), refs[4]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE researchflow_stages SET status = ? WHERE run_ref = ?",
		string(domain.StageSucceeded), refs[5]); err != nil {
		t.Fatal(err)
	}

	first, err := repository.ListSchedulableRunRefs(ctx, prefix[:len(prefix)-1], 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || first[0] != refs[0] || first[1] != refs[1] {
		t.Fatalf("first schedulable page=%v, want %v", first, refs[:2])
	}
	second, err := repository.ListSchedulableRunRefs(ctx, first[len(first)-1], 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || second[0] != refs[2] {
		t.Fatalf("second schedulable page=%v, want only %q", second, refs[2])
	}
}
