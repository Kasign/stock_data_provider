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

// TestMariaDBExpireRunDeadlineAtomicallyTimesOutPendingStages verifies the
// deadline marker, Stage closure and version changes commit as one transaction.
func TestMariaDBExpireRunDeadlineAtomicallyTimesOutPendingStages(t *testing.T) {
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
	now := time.Now().UTC()
	runRef := "deadline-expire-run-" + suffix
	stageRef := "deadline-expire-stage-" + suffix
	templateFingerprint := "dededededededededededededededededededededededededededededededede"
	record := application.CreateRunRecord{
		Scope: application.IdempotencyScope{
			CallerRef: "researchflow-deadline-expire", Operation: "CreateRun", Key: "deadline-expire-" + suffix,
		},
		Fingerprint: "efefefefefefefefefefefefefefefefefefefefefefefefefefefefefefefef",
		Run: domain.Run{
			Ref: runRef, SchemaVersion: "run-v1", Status: domain.RunCreated, Version: 1,
			SystemAsOf: now.Add(-time.Minute), DeadlineAt: now.Add(5 * time.Minute),
			TemplateRef: "deadline-expire-template", TemplateVersion: "v1",
			TemplateFingerprint: templateFingerprint, InputSnapshotRefs: []string{},
			CorrelationID: "deadline-expire-" + suffix,
		},
		Stages: []domain.Stage{{
			Ref: stageRef, RunRef: runRef, Code: "context", Status: domain.StagePending,
			Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
			DiagnosticRefs: []string{}, ReasonCodes: []string{},
		}},
		ResolvedTemplate: template.ResolvedTemplate{
			Ref: "deadline-expire-template", Version: "v1", Fingerprint: templateFingerprint,
			Stages: []template.ResolvedStage{{Definition: template.StageDefinition{Code: "context"}}},
		},
		Parameters: json.RawMessage("{}"),
	}
	if _, err := repository.CreateRunAtomic(ctx, record); err != nil {
		t.Fatalf("CreateRunAtomic: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE researchflow_runs SET deadline_at=? WHERE run_ref=?",
		formatResearchflowTimeValue(now.Add(-time.Minute)), runRef); err != nil {
		t.Fatalf("expire fixture deadline: %v", err)
	}

	updated, version, err := repository.ExpireRunDeadline(ctx, runRef, 1)
	if err != nil || !updated || version != 2 {
		t.Fatalf("ExpireRunDeadline updated=%v version=%d err=%v; want true/2/nil", updated, version, err)
	}

	var runStatus, terminationReason string
	var runVersion uint64
	if err := db.QueryRowContext(ctx, "SELECT status, version, termination_reason FROM researchflow_runs WHERE run_ref=?",
		runRef).Scan(&runStatus, &runVersion, &terminationReason); err != nil {
		t.Fatal(err)
	}
	if domain.RunStatus(runStatus) != domain.RunCreated || runVersion != 2 || terminationReason != "deadline_exceeded" {
		t.Fatalf("Run status=%s version=%d termination_reason=%s", runStatus, runVersion, terminationReason)
	}

	var stageStatus, reasons string
	var stageVersion uint64
	if err := db.QueryRowContext(ctx, "SELECT status, version, reason_codes FROM researchflow_stages WHERE stage_ref=?",
		stageRef).Scan(&stageStatus, &stageVersion, &reasons); err != nil {
		t.Fatal(err)
	}
	var reasonCodes []string
	if err := json.Unmarshal([]byte(reasons), &reasonCodes); err != nil {
		t.Fatal(err)
	}
	if domain.StageStatus(stageStatus) != domain.StageTimedOut || stageVersion != 2 ||
		len(reasonCodes) != 1 || reasonCodes[0] != "run_deadline_exceeded" {
		t.Fatalf("Stage status=%s version=%d reason_codes=%v", stageStatus, stageVersion, reasonCodes)
	}

	updated, version, err = repository.ExpireRunDeadline(ctx, runRef, 2)
	if err != nil || updated || version != 2 {
		t.Fatalf("repeated ExpireRunDeadline updated=%v version=%d err=%v; want false/2/nil", updated, version, err)
	}
}
