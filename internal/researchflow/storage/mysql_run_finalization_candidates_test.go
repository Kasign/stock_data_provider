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

func TestMariaDBListFinalizableRunRefsSelectsOnlyFullyStoppedRuns(t *testing.T) {
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

	suffix := fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	prefix := "zzzz-researchflow-finalize-" + suffix + "-"
	now := time.Now().UTC()
	fingerprint := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	templateFingerprint := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	refs := []string{prefix + "normal", prefix + "deadline", prefix + "cancel", prefix + "active", prefix + "unrouted"}
	stageStatuses := []domain.StageStatus{
		domain.StageSucceeded, domain.StageTimedOut, domain.StageCancelled,
		domain.StageRunning, domain.StageSucceeded,
	}
	runStatuses := []domain.RunStatus{
		domain.RunRunning, domain.RunCreated, domain.RunCancelling,
		domain.RunRunning, domain.RunQueued,
	}
	reasons := []string{"", "deadline_exceeded", "user_cancelled", "", ""}
	for index, runRef := range refs {
		stageRef := fmt.Sprintf("finalize-candidate-stage-%d-%s", index, suffix)
		record := application.CreateRunRecord{
			Scope: application.IdempotencyScope{
				CallerRef: "researchflow-finalize-candidate-" + suffix,
				Operation: "CreateRun",
				Key:       fmt.Sprintf("finalize-candidate-%d-%s", index, suffix),
			},
			Fingerprint: fingerprint,
			Run: domain.Run{
				Ref: runRef, SchemaVersion: "v1", Status: domain.RunCreated, Version: 1,
				SystemAsOf: now.Add(-time.Minute), DeadlineAt: now.Add(time.Hour),
				TemplateRef: "finalize-candidate-template", TemplateVersion: "v1",
				TemplateFingerprint: templateFingerprint, InputSnapshotRefs: []string{},
				CorrelationID: "finalize-candidate-correlation-" + suffix,
			},
			Stages: []domain.Stage{{
				Ref: stageRef, RunRef: runRef, Code: "context", Status: domain.StagePending,
				Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
				DiagnosticRefs: []string{}, ReasonCodes: []string{},
			}},
			ResolvedTemplate: template.ResolvedTemplate{
				Ref: "finalize-candidate-template", Version: "v1", Fingerprint: templateFingerprint,
				Stages: []template.ResolvedStage{{Definition: template.StageDefinition{Code: "context"}}},
			},
			Parameters: json.RawMessage("{}"),
		}
		if _, err := repository.CreateRunAtomic(ctx, record); err != nil {
			t.Fatalf("create candidate fixture %q: %v", runRef, err)
		}
		if _, err := db.ExecContext(ctx, "UPDATE researchflow_runs SET status = ?, termination_reason = ? WHERE run_ref = ?",
			string(runStatuses[index]), nullableText(reasons[index]), runRef); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "UPDATE researchflow_stages SET status = ? WHERE run_ref = ?",
			string(stageStatuses[index]), runRef); err != nil {
			t.Fatal(err)
		}
	}

	first, err := repository.ListFinalizableRunRefs(ctx, prefix[:len(prefix)-1], 2)
	if err != nil {
		t.Fatal(err)
	}
	wantFirst := refs[:2]
	if len(first) != len(wantFirst) || first[0] != wantFirst[0] || first[1] != wantFirst[1] {
		t.Fatalf("first finalizable page=%v, want %v", first, wantFirst)
	}
	second, err := repository.ListFinalizableRunRefs(ctx, first[len(first)-1], 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || second[0] != refs[2] {
		t.Fatalf("second finalizable page=%v, want only %q", second, refs[2])
	}
}
