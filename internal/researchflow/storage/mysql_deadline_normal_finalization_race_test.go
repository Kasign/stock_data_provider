package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
	"example.com/stock-analysis-flow/internal/researchflow/application"
	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

// TestMariaDBDeadlineFinalizationWinsAgainstStaleNormalFinalizer verifies that
// a normal finalizer holding a pre-deadline snapshot cannot override the
// persisted deadline termination reason.
func TestMariaDBDeadlineFinalizationWinsAgainstStaleNormalFinalizer(t *testing.T) {
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
	runRef := "deadline-normal-race-run-" + suffix
	stageRef := "deadline-normal-race-stage-" + suffix
	templateFingerprint := "abababababababababababababababababababababababababababababababab"
	record := application.CreateRunRecord{
		Scope: application.IdempotencyScope{
			CallerRef: "researchflow-deadline-normal-race", Operation: "CreateRun", Key: "deadline-normal-race-" + suffix,
		},
		Fingerprint: "cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd",
		Run: domain.Run{
			Ref: runRef, SchemaVersion: "run-v1", Status: domain.RunCreated, Version: 1,
			SystemAsOf: now.Add(-time.Minute), DeadlineAt: now.Add(5 * time.Minute),
			TemplateRef: "deadline-normal-race-template", TemplateVersion: "v1",
			TemplateFingerprint: templateFingerprint, InputSnapshotRefs: []string{},
			CorrelationID: "deadline-normal-race-" + suffix,
		},
		Stages: []domain.Stage{{
			Ref: stageRef, RunRef: runRef, Code: "context", Status: domain.StagePending,
			Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
			DiagnosticRefs: []string{}, ReasonCodes: []string{},
		}},
		ResolvedTemplate: template.ResolvedTemplate{
			Ref: "deadline-normal-race-template", Version: "v1", Fingerprint: templateFingerprint,
			Stages: []template.ResolvedStage{{Definition: template.StageDefinition{Code: "context"}}},
		},
		Parameters: json.RawMessage("{}"),
	}
	if _, err := repository.CreateRunAtomic(ctx, record); err != nil {
		t.Fatalf("CreateRunAtomic: %v", err)
	}

	// Persist the deadline winner's linearization point and terminalize the
	// Stage as ExpireRunDeadline does before the application aggregation step.
	if _, err := db.ExecContext(ctx, "UPDATE researchflow_runs SET status=?, version=version+1, deadline_at=?, termination_reason=? WHERE run_ref=?",
		string(domain.RunRunning), formatResearchflowTimeValue(now.Add(-time.Minute)), "deadline_exceeded", runRef); err != nil {
		t.Fatalf("prepare deadline-owned Run: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE researchflow_stages SET status=?, version=version+1 WHERE stage_ref=?",
		string(domain.StageTimedOut), stageRef); err != nil {
		t.Fatalf("prepare deadline Stage: %v", err)
	}

	summary := domain.ResultSummary{
		StageOutcomes:  map[string]domain.StageStatus{"context": domain.StageTimedOut},
		ExcludedStages: []domain.ExcludedStage{}, MissingStageCodes: []string{"context"},
		DegradedStageCodes: []string{}, BlockedStageCodes: []string{}, ReasonCodes: []string{"deadline_exceeded"},
		RiskGateDecisions: map[string]string{}, Coverage: 0, QualityStatus: "incomplete",
		ActionabilityStatus: "not_evaluated",
	}
	deadlineInput := application.DeadlineFinalizationRecord{
		RunRef: runRef, ExpectedVersion: 2, ResultSummaryRef: "deadline-race-summary-" + suffix,
		SummarySchemaVersion: "summary-v1", OutboxRef: "deadline-race-outbox-" + suffix,
		TerminalStatus: domain.RunFailed, TerminationReason: "deadline_exceeded", Summary: summary,
	}
	normalInput := RunFinalization{
		RunRef: runRef, ExpectedVersion: 2, ResultSummaryRef: "normal-race-summary-" + suffix,
		SummarySchemaVersion: "summary-v1", OutboxRef: "normal-race-outbox-" + suffix,
		TerminalStatus: domain.RunSucceeded,
		Summary: domain.ResultSummary{
			StageOutcomes:  map[string]domain.StageStatus{"context": domain.StageSucceeded},
			ExcludedStages: []domain.ExcludedStage{}, MissingStageCodes: []string{},
			DegradedStageCodes: []string{}, BlockedStageCodes: []string{}, ReasonCodes: []string{},
			RiskGateDecisions: map[string]string{}, Coverage: 1, QualityStatus: "valid",
			ActionabilityStatus: "not_evaluated",
		},
	}

	start := make(chan struct{})
	type outcome struct {
		path    string
		updated bool
		err     error
	}
	results := make(chan outcome, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		updated, err := repository.FinalizeDeadlineRun(ctx, deadlineInput)
		results <- outcome{path: "deadline", updated: updated, err: err}
	}()
	go func() {
		defer wait.Done()
		<-start
		updated, err := repository.FinalizeRun(ctx, normalInput)
		results <- outcome{path: "normal", updated: updated, err: err}
	}()
	close(start)
	wait.Wait()
	close(results)

	deadlineWins := 0
	normalRejected := 0
	for result := range results {
		if result.path == "deadline" {
			if result.err != nil || !result.updated {
				t.Fatalf("deadline finalization updated=%v err=%v; deadline path must win", result.updated, result.err)
			}
			deadlineWins++
			continue
		}
		if result.updated {
			t.Fatal("stale normal finalizer unexpectedly committed")
		}
		if result.err == nil || (!errors.Is(result.err, ErrRunAlreadyFinalized) &&
			!strings.Contains(result.err.Error(), "termination reason")) {
			t.Fatalf("stale normal finalizer error=%v; want finalized or deadline-reason rejection", result.err)
		}
		normalRejected++
	}
	if deadlineWins != 1 || normalRejected != 1 {
		t.Fatalf("deadline wins=%d normal rejected=%d, want 1/1", deadlineWins, normalRejected)
	}

	var status, reason, summaryRef string
	var version uint64
	if err := db.QueryRowContext(ctx, "SELECT status, version, termination_reason, result_summary_ref FROM researchflow_runs WHERE run_ref=?",
		runRef).Scan(&status, &version, &reason, &summaryRef); err != nil {
		t.Fatal(err)
	}
	if domain.RunStatus(status) != domain.RunFailed || version != 3 || reason != "deadline_exceeded" ||
		summaryRef != deadlineInput.ResultSummaryRef {
		t.Fatalf("persisted Run status=%s version=%d reason=%s summary=%s", status, version, reason, summaryRef)
	}
	var summaries, events int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM researchflow_result_summaries WHERE run_ref=?", runRef).Scan(&summaries); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM researchflow_outbox WHERE run_ref=? AND event_type=?",
		runRef, runFinalizedEventType).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if summaries != 1 || events != 1 {
		t.Fatalf("persisted summaries=%d events=%d, want 1/1", summaries, events)
	}
}
