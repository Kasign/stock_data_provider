package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

func TestMariaDBFinalizeRunAtomicallyPersistsSummaryAndOutbox(t *testing.T) {
	if os.Getenv("PLATFORM_STORAGE_INTEGRATION") != "1" {
		t.Skip("set PLATFORM_STORAGE_INTEGRATION=1 to run Researchflow MariaDB integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true",
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
	runRef := "finalize-run-" + suffix
	stageRef := "finalize-stage-" + suffix
	fingerprint := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	templateFingerprint := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	record := application.CreateRunRecord{
		Scope: application.IdempotencyScope{
			CallerRef: "researchflow-finalize-test", Operation: "CreateRun", Key: "finalize-" + suffix,
		},
		Fingerprint: fingerprint,
		Run: domain.Run{
			Ref: runRef, SchemaVersion: "run-v1", Status: domain.RunCreated, Version: 1,
			SystemAsOf: now.Add(-time.Minute), DeadlineAt: now.Add(5 * time.Minute),
			TemplateRef: "finalize-template", TemplateVersion: "v1",
			TemplateFingerprint: templateFingerprint, InputSnapshotRefs: []string{},
			CorrelationID: "finalize-correlation-" + suffix,
		},
		Stages: []domain.Stage{{
			Ref: stageRef, RunRef: runRef, Code: "context", Status: domain.StagePending,
			Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
			DiagnosticRefs: []string{}, ReasonCodes: []string{},
		}},
		ResolvedTemplate: template.ResolvedTemplate{
			Ref: "finalize-template", Version: "v1", Fingerprint: templateFingerprint,
			Stages: []template.ResolvedStage{{Definition: template.StageDefinition{Code: "context"}}},
		},
		Parameters: json.RawMessage(`{}`),
	}
	if _, err := repository.CreateRunAtomic(ctx, record); err != nil {
		t.Fatalf("CreateRunAtomic: %v", err)
	}
	startedAt := formatResearchflowTimeValue(now)
	finishedAt := formatResearchflowTimeValue(now.Add(time.Second))
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_runs
		SET status = ?, version = version + 1, started_at = ? WHERE run_ref = ?`,
		string(domain.RunRunning), startedAt, runRef); err != nil {
		t.Fatalf("start fixture Run: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_stages
		SET status = ?, version = version + 1, started_at = ?, finished_at = ?
		WHERE stage_ref = ?`,
		string(domain.StageSucceeded), startedAt, finishedAt, stageRef); err != nil {
		t.Fatalf("finish fixture Stage: %v", err)
	}

	input := RunFinalization{
		RunRef: runRef, ExpectedVersion: 2,
		ResultSummaryRef: "summary-" + suffix, SummarySchemaVersion: "summary-v1",
		OutboxRef: "outbox-" + suffix, TerminalStatus: domain.RunSucceeded,
		Summary: domain.ResultSummary{
			StageOutcomes: map[string]domain.StageStatus{"context": domain.StageSucceeded},
			ExcludedStages: []domain.ExcludedStage{}, MissingStageCodes: []string{},
			DegradedStageCodes: []string{}, BlockedStageCodes: []string{},
			ReasonCodes: []string{}, Coverage: 1, QualityStatus: "valid",
			RiskGateDecisions: map[string]string{}, ActionabilityStatus: "not_evaluated",
		},
	}
	updated, err := repository.FinalizeRun(ctx, input)
	if err != nil || !updated {
		t.Fatalf("FinalizeRun updated=%v err=%v", updated, err)
	}

	var runStatus, summaryRef string
	var runVersion uint64
	if err := db.QueryRowContext(ctx, `SELECT status, version, result_summary_ref
		FROM researchflow_runs WHERE run_ref = ?`, runRef).Scan(&runStatus, &runVersion, &summaryRef); err != nil {
		t.Fatalf("read finalized Run: %v", err)
	}
	if domain.RunStatus(runStatus) != domain.RunSucceeded || runVersion != 3 || summaryRef != input.ResultSummaryRef {
		t.Fatalf("finalized Run state=%s version=%d summary=%s", runStatus, runVersion, summaryRef)
	}
	var summaryCount, outboxCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM researchflow_result_summaries WHERE run_ref = ?`, runRef).Scan(&summaryCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM researchflow_outbox WHERE run_ref = ? AND event_type = ?`, runRef, runFinalizedEventType).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if summaryCount != 1 || outboxCount != 1 {
		t.Fatalf("summary count=%d outbox count=%d, want one each", summaryCount, outboxCount)
	}
	if _, err := repository.FinalizeRun(ctx, input); !errors.Is(err, ErrRunAlreadyFinalized) {
		t.Fatalf("duplicate FinalizeRun error=%v, want ErrRunAlreadyFinalized", err)
	}
}

func TestFinalizeRunRejectsIncompleteSummaryBeforeTransaction(t *testing.T) {
	repository := &MySQLRunRepository{db: &sql.DB{}}
	_, err := repository.FinalizeRun(context.Background(), RunFinalization{
		RunRef: "run-1", ExpectedVersion: 1, ResultSummaryRef: "summary-1",
		SummarySchemaVersion: "v1", OutboxRef: "outbox-1",
		TerminalStatus: domain.RunSucceeded,
	})
	if err == nil {
		t.Fatal("expected incomplete summary to be rejected")
	}
	if errors.Is(err, sql.ErrConnDone) {
		t.Fatalf("incomplete summary reached the database: %v", err)
	}
}
