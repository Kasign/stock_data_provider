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

type deadlineFinalizerIntegrationIDs struct {
	prefix string
}

func (g deadlineFinalizerIntegrationIDs) NewRef(_ context.Context, kind string) (string, error) {
	return g.prefix + "-" + kind, nil
}

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
	repository, err := NewMySQLRunRepository(db, CapacityLimits{MaxQueuedRuns: 1000, MaxWorkers: 64})
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
			Stages: []template.ResolvedStage{{
				Definition: template.StageDefinition{
					Code:        "context",
					Propagation: template.PropagationPolicy{OnTimedOut: domain.PropagateFail},
				},
			}},
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

	databaseClock, err := NewMySQLDatabaseClock(db)
	if err != nil {
		t.Fatalf("create Researchflow database clock: %v", err)
	}
	finalizer, err := application.NewDeadlineFinalizer(
		repository, databaseClock, deadlineFinalizerIntegrationIDs{prefix: "deadline-finalize-" + suffix},
		application.DeadlineFinalizerConfig{SummarySchemaVersion: "1.0.0"},
	)
	if err != nil {
		t.Fatalf("create Researchflow deadline finalizer: %v", err)
	}
	finalized, err := finalizer.Finalize(ctx, runRef)
	if err != nil {
		t.Fatalf("finalize expired Researchflow Run: %v", err)
	}
	if !finalized.Finalized || finalized.Run.Status != domain.RunFailed ||
		finalized.Run.ResultSummary == nil || finalized.Run.ResultSummaryRef == nil {
		t.Fatalf("unexpected failed deadline finalization result: %+v", finalized)
	}

	var finalStatus, finalReason, persistedSummaryRef string
	var finalVersion uint64
	if err := db.QueryRowContext(ctx, `SELECT status, version, termination_reason, result_summary_ref
		FROM researchflow_runs WHERE run_ref = ?`, runRef).Scan(
		&finalStatus, &finalVersion, &finalReason, &persistedSummaryRef,
	); err != nil {
		t.Fatalf("read finalized Researchflow Run: %v", err)
	}
	if domain.RunStatus(finalStatus) != domain.RunFailed || finalVersion != 3 ||
		finalReason != "deadline_exceeded" || persistedSummaryRef == "" ||
		persistedSummaryRef != *finalized.Run.ResultSummaryRef {
		t.Fatalf("persisted failed Run status=%s version=%d reason=%s summary_ref=%s",
			finalStatus, finalVersion, finalReason, persistedSummaryRef)
	}

	var summaryCount, outboxCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM researchflow_result_summaries WHERE result_summary_ref = ?", persistedSummaryRef,
	).Scan(&summaryCount); err != nil {
		t.Fatalf("count persisted ResultSummary: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM researchflow_outbox WHERE run_ref = ? AND event_type = ?", runRef, "researchflow.run.terminal",
	).Scan(&outboxCount); err != nil {
		t.Fatalf("count terminal Outbox event: %v", err)
	}
	if summaryCount != 1 || outboxCount != 1 {
		t.Fatalf("failed finalization rows summary=%d outbox=%d; want exactly one each", summaryCount, outboxCount)
	}

	var eventPayload string
	if err := db.QueryRowContext(ctx,
		"SELECT payload FROM researchflow_outbox WHERE run_ref = ? AND event_type = ?", runRef, "researchflow.run.terminal",
	).Scan(&eventPayload); err != nil {
		t.Fatalf("read failed terminal Outbox payload: %v", err)
	}
	var terminalEvent struct {
		Status            domain.RunStatus `json:"status"`
		TerminationReason *string          `json:"termination_reason"`
		ResultSummaryRef  *string          `json:"result_summary_ref"`
	}
	if err := json.Unmarshal([]byte(eventPayload), &terminalEvent); err != nil {
		t.Fatalf("decode failed terminal Outbox payload: %v", err)
	}
	if terminalEvent.Status != domain.RunFailed || terminalEvent.TerminationReason == nil ||
		*terminalEvent.TerminationReason != "deadline_exceeded" || terminalEvent.ResultSummaryRef == nil ||
		*terminalEvent.ResultSummaryRef != persistedSummaryRef {
		t.Fatalf("failed terminal Outbox payload mismatch: %+v", terminalEvent)
	}
}
