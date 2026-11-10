package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
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
			StageOutcomes:  map[string]domain.StageStatus{"context": domain.StageSucceeded},
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

	// Worker A claims the event. Once its lease expires, Worker B reclaims it
	// with a higher generation; A must not acknowledge B's lease.
	claimedA, err := repository.ClaimOutbox(ctx, "worker-a", runRef, 10, 30*time.Second)
	if err != nil || len(claimedA) != 1 {
		t.Fatalf("Worker A ClaimOutbox got %d messages, err=%v", len(claimedA), err)
	}
	if claimedA[0].AttemptCount != 1 || claimedA[0].LeaseGeneration != 1 {
		t.Fatalf("Worker A claim metadata = attempt %d generation %d", claimedA[0].AttemptCount, claimedA[0].LeaseGeneration)
	}
	renewedUntil, renewed, err := repository.RenewOutboxLease(ctx, input.OutboxRef, "worker-a", claimedA[0].LeaseGeneration, 60*time.Second)
	if err != nil || !renewed || !renewedUntil.After(now.Add(30*time.Second)) {
		t.Fatalf("Worker A RenewOutboxLease = (%s, %v), err=%v", renewedUntil, renewed, err)
	}
	var persistedLease string
	if err := db.QueryRowContext(ctx, `SELECT lease_expires_at FROM researchflow_outbox WHERE outbox_ref = ?`, input.OutboxRef).Scan(&persistedLease); err != nil {
		t.Fatalf("read renewed Outbox lease: %v", err)
	}
	persistedExpiry, err := parseResearchflowTime(persistedLease)
	if err != nil || !persistedExpiry.Equal(renewedUntil) {
		t.Fatalf("persisted Outbox lease expiry = %s, want %s (err=%v)", persistedExpiry, renewedUntil, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_outbox SET lease_expires_at = ?
		WHERE outbox_ref = ?`, formatResearchflowTimeValue(now.Add(-time.Second)), input.OutboxRef); err != nil {
		t.Fatalf("expire Worker A lease: %v", err)
	}
	if _, renewed, err := repository.RenewOutboxLease(ctx, input.OutboxRef, "worker-a", claimedA[0].LeaseGeneration, time.Minute); err != nil || renewed {
		t.Fatalf("expired Worker A lease renewal = %v, err=%v; want false", renewed, err)
	}
	claimedB, err := repository.ClaimOutbox(ctx, "worker-b", runRef, 10, 30*time.Second)
	if err != nil || len(claimedB) != 1 {
		t.Fatalf("Worker B ClaimOutbox got %d messages, err=%v", len(claimedB), err)
	}
	if claimedB[0].LeaseGeneration != 2 || claimedB[0].AttemptCount != 2 {
		t.Fatalf("Worker B claim metadata = attempt %d generation %d", claimedB[0].AttemptCount, claimedB[0].LeaseGeneration)
	}
	if acknowledged, err := repository.AcknowledgeOutbox(ctx, input.OutboxRef, "worker-a", claimedA[0].LeaseGeneration); err != nil || acknowledged {
		t.Fatalf("stale Worker A acknowledgement = %v, err=%v; want false", acknowledged, err)
	}
	retryAt := time.Now().UTC().Add(2 * time.Second)
	if retried, err := repository.RetryOutbox(ctx, input.OutboxRef, "worker-b", claimedB[0].LeaseGeneration, retryAt, "temporary_failure"); err != nil || !retried {
		t.Fatalf("Worker B RetryOutbox = %v, err=%v", retried, err)
	}
	if claimed, err := repository.ClaimOutbox(ctx, "worker-c", runRef, 10, 30*time.Second); err != nil || len(claimed) != 0 {
		t.Fatalf("message claimed before next_attempt_at: count=%d err=%v", len(claimed), err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_outbox SET next_attempt_at = ?
		WHERE outbox_ref = ?`, formatResearchflowTimeValue(now.Add(-time.Second)), input.OutboxRef); err != nil {
		t.Fatalf("make Outbox retry due: %v", err)
	}
	claimedC, err := repository.ClaimOutbox(ctx, "worker-c", runRef, 10, 30*time.Second)
	if err != nil || len(claimedC) != 1 || claimedC[0].LeaseGeneration != 3 {
		t.Fatalf("Worker C retry claim = %+v, err=%v", claimedC, err)
	}
	if acknowledged, err := repository.AcknowledgeOutbox(ctx, input.OutboxRef, "worker-c", claimedC[0].LeaseGeneration); err != nil || !acknowledged {
		t.Fatalf("Worker C acknowledgement = %v, err=%v", acknowledged, err)
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

func TestMariaDBFinalizeCancellationRunAtomicallyPersistsSummaryAndOutbox(t *testing.T) {
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
	runRef := "finalize-cancel-run-" + suffix
	stageRef := "finalize-cancel-stage-" + suffix
	fingerprint := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	templateFingerprint := "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	record := application.CreateRunRecord{
		Scope: application.IdempotencyScope{
			CallerRef: "researchflow-finalize-cancel-test", Operation: "CreateRun", Key: "finalize-cancel-" + suffix,
		},
		Fingerprint: fingerprint,
		Run: domain.Run{
			Ref: runRef, SchemaVersion: "run-v1", Status: domain.RunCreated, Version: 1,
			SystemAsOf: now.Add(-time.Minute), DeadlineAt: now.Add(5 * time.Minute),
			TemplateRef: "finalize-cancel-template", TemplateVersion: "v1",
			TemplateFingerprint: templateFingerprint, InputSnapshotRefs: []string{},
			CorrelationID: "finalize-cancel-correlation-" + suffix,
		},
		Stages: []domain.Stage{{
			Ref: stageRef, RunRef: runRef, Code: "context", Status: domain.StagePending,
			Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
			DiagnosticRefs: []string{}, ReasonCodes: []string{},
		}},
		ResolvedTemplate: template.ResolvedTemplate{
			Ref: "finalize-cancel-template", Version: "v1", Fingerprint: templateFingerprint,
			Stages: []template.ResolvedStage{{Definition: template.StageDefinition{Code: "context"}}},
		},
		Parameters: json.RawMessage(`{}`),
	}
	if _, err := repository.CreateRunAtomic(ctx, record); err != nil {
		t.Fatalf("CreateRunAtomic: %v", err)
	}
	finishedAt := formatResearchflowTimeValue(now.Add(time.Second))
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_runs
		SET status = ?, version = version + 1, termination_reason = ?
		WHERE run_ref = ?`, string(domain.RunCancelling), "user_cancelled", runRef); err != nil {
		t.Fatalf("mark fixture Run cancelling: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_stages
		SET status = ?, version = version + 1, reason_codes = ?, finished_at = ?
		WHERE stage_ref = ?`, string(domain.StageCancelled), `["user_cancelled"]`, finishedAt, stageRef); err != nil {
		t.Fatalf("cancel fixture Stage: %v", err)
	}

	input := application.CancellationFinalizationRecord{
		RunRef: runRef, ExpectedVersion: 2,
		ResultSummaryRef: "summary-cancel-" + suffix, SummarySchemaVersion: "summary-v1",
		OutboxRef: "outbox-cancel-" + suffix, TerminalStatus: domain.RunCancelled,
		TerminationReason: "user_cancelled",
		Summary: domain.ResultSummary{
			StageOutcomes:     map[string]domain.StageStatus{"context": domain.StageCancelled},
			RiskGateDecisions: map[string]string{}, ExcludedStages: []domain.ExcludedStage{},
			MissingStageCodes: []string{"context"}, DegradedStageCodes: []string{},
			BlockedStageCodes: []string{}, ReasonCodes: []string{"user_cancelled"},
		},
	}
	updated, err := repository.FinalizeCancellationRun(ctx, input)
	if err != nil || !updated {
		t.Fatalf("FinalizeCancellationRun updated=%v err=%v", updated, err)
	}

	var runStatus, terminationReason, summaryRef string
	var runVersion uint64
	if err := db.QueryRowContext(ctx, `SELECT status, version, termination_reason, result_summary_ref
		FROM researchflow_runs WHERE run_ref = ?`, runRef).Scan(&runStatus, &runVersion, &terminationReason, &summaryRef); err != nil {
		t.Fatalf("read cancelled Run: %v", err)
	}
	if domain.RunStatus(runStatus) != domain.RunCancelled || runVersion != 3 ||
		terminationReason != "user_cancelled" || summaryRef != input.ResultSummaryRef {
		t.Fatalf("cancelled Run state=%s version=%d reason=%s summary=%s", runStatus, runVersion, terminationReason, summaryRef)
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
}


// TestMariaDBFinalizeRunRollsBackWhenOutboxInsertFails verifies that a
// failure after the ResultSummary insert cannot leave a partial terminal Run.
func TestMariaDBFinalizeRunRollsBackWhenOutboxInsertFails(t *testing.T) {
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
	runRef := "rollback-finalize-run-" + suffix
	stageRef := "rollback-finalize-stage-" + suffix
	now := time.Now().UTC()
	fingerprint := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	templateFingerprint := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	record := application.CreateRunRecord{
		Scope: application.IdempotencyScope{
			CallerRef: "researchflow-finalize-rollback-test", Operation: "CreateRun", Key: "rollback-" + suffix,
		},
		Fingerprint: fingerprint,
		Run: domain.Run{
			Ref: runRef, SchemaVersion: "run-v1", Status: domain.RunCreated, Version: 1,
			SystemAsOf: now.Add(-time.Minute), DeadlineAt: now.Add(5 * time.Minute),
			TemplateRef: "rollback-template", TemplateVersion: "v1",
			TemplateFingerprint: templateFingerprint, InputSnapshotRefs: []string{},
			CorrelationID: "rollback-correlation-" + suffix,
		},
		Stages: []domain.Stage{{
			Ref: stageRef, RunRef: runRef, Code: "context", Status: domain.StagePending,
			Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
			DiagnosticRefs: []string{}, ReasonCodes: []string{},
		}},
		ResolvedTemplate: template.ResolvedTemplate{
			Ref: "rollback-template", Version: "v1", Fingerprint: templateFingerprint,
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

	triggerName := "researchflow_fail_outbox_" + suffix
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER `+triggerName+` BEFORE INSERT ON researchflow_outbox
		FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'forced Researchflow Outbox insert failure'`); err != nil {
		t.Fatalf("create Outbox failure trigger: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "DROP TRIGGER IF EXISTS "+triggerName)
	}()

	input := RunFinalization{
		RunRef: runRef, ExpectedVersion: 2,
		ResultSummaryRef: "rollback-summary-" + suffix, SummarySchemaVersion: "summary-v1",
		OutboxRef: "rollback-outbox-" + suffix, TerminalStatus: domain.RunSucceeded,
		Summary: domain.ResultSummary{
			StageOutcomes: map[string]domain.StageStatus{"context": domain.StageSucceeded},
			ExcludedStages: []domain.ExcludedStage{}, MissingStageCodes: []string{},
			DegradedStageCodes: []string{}, BlockedStageCodes: []string{},
			ReasonCodes: []string{}, Coverage: 1, QualityStatus: "valid",
			RiskGateDecisions: map[string]string{}, ActionabilityStatus: "not_evaluated",
		},
	}
	if updated, err := repository.FinalizeRun(ctx, input); err == nil || updated {
		t.Fatalf("FinalizeRun updated=%v err=%v, want failed transaction", updated, err)
	}

	var status string
	var version uint64
	var summaryRef sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT status, version, result_summary_ref
		FROM researchflow_runs WHERE run_ref = ?`, runRef).Scan(&status, &version, &summaryRef); err != nil {
		t.Fatalf("read Run after rollback: %v", err)
	}
	if domain.RunStatus(status) != domain.RunRunning || version != 2 || summaryRef.Valid {
		t.Fatalf("Run changed despite Outbox failure: status=%s version=%d summary_ref=%v", status, version, summaryRef)
	}
	var summaryCount, outboxCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM researchflow_result_summaries WHERE run_ref = ?", runRef).Scan(&summaryCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM researchflow_outbox WHERE run_ref = ?", runRef).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if summaryCount != 0 || outboxCount != 0 {
		t.Fatalf("partial finalization persisted: summaries=%d outbox=%d, want 0/0", summaryCount, outboxCount)
	}
}


// TestMariaDBFinalizeRunConcurrentCAS ensures competing finalizers cannot
// persist duplicate summaries or finalized events for one Run version.
func TestMariaDBFinalizeRunConcurrentCAS(t *testing.T) {
	if os.Getenv("PLATFORM_STORAGE_INTEGRATION") != "1" {
		t.Skip("set PLATFORM_STORAGE_INTEGRATION=1 to run Researchflow MariaDB integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true",
		os.Getenv("PLATFORM_MARIADB_USER"), os.Getenv("PLATFORM_MARIADB_PASSWORD"),
		os.Getenv("PLATFORM_MARIADB_ADDR"), os.Getenv("PLATFORM_MARIADB_DATABASE"))
	config, err := mysql.ParseDSN(dsn)
	if err != nil { t.Fatal(err) }
	connector, err := mysql.NewConnector(config)
	if err != nil { t.Fatal(err) }
	db, err := platformstorage.OpenSQL(ctx, connector, platformstorage.DefaultSQLConfig())
	if err != nil { t.Fatalf("open MariaDB: %v", err) }
	defer db.Close()
	if err := platformstorage.ApplyMigrations(ctx, db, MigrationNamespace, Migrations()); err != nil {
		t.Fatalf("apply Researchflow migrations: %v", err)
	}
	repository, err := NewMySQLRunRepository(db)
	if err != nil { t.Fatal(err) }

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	now := time.Now().UTC()
	runRef, stageRef := "concurrent-finalize-run-"+suffix, "concurrent-finalize-stage-"+suffix
	templateFingerprint := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	record := application.CreateRunRecord{
		Scope: application.IdempotencyScope{CallerRef:"researchflow-finalize-race", Operation:"CreateRun", Key:"race-"+suffix},
		Fingerprint:"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		Run: domain.Run{Ref:runRef, SchemaVersion:"run-v1", Status:domain.RunCreated, Version:1,
			SystemAsOf:now.Add(-time.Minute), DeadlineAt:now.Add(5*time.Minute),
			TemplateRef:"concurrent-template", TemplateVersion:"v1", TemplateFingerprint:templateFingerprint,
			InputSnapshotRefs:[]string{}, CorrelationID:"concurrent-finalize-"+suffix},
		Stages:[]domain.Stage{{Ref:stageRef, RunRef:runRef, Code:"context", Status:domain.StagePending,
			Version:1, Required:true, Dependencies:[]string{}, RetryPolicyRef:"retry-v1",
			DiagnosticRefs:[]string{}, ReasonCodes:[]string{}}},
		ResolvedTemplate:template.ResolvedTemplate{Ref:"concurrent-template", Version:"v1",
			Fingerprint:templateFingerprint, Stages:[]template.ResolvedStage{{Definition:template.StageDefinition{Code:"context"}}}},
		Parameters:json.RawMessage(`{}`),
	}
	if _, err := repository.CreateRunAtomic(ctx, record); err != nil { t.Fatalf("CreateRunAtomic: %v", err) }
	started, finished := formatResearchflowTimeValue(now), formatResearchflowTimeValue(now.Add(time.Second))
	if _, err := db.ExecContext(ctx, "UPDATE researchflow_runs SET status=?, version=version+1, started_at=? WHERE run_ref=?",
		string(domain.RunRunning), started, runRef); err != nil { t.Fatal(err) }
	if _, err := db.ExecContext(ctx, "UPDATE researchflow_stages SET status=?, version=version+1, started_at=?, finished_at=? WHERE stage_ref=?",
		string(domain.StageSucceeded), started, finished, stageRef); err != nil { t.Fatal(err) }

	inputs := []RunFinalization{
		{RunRef:runRef, ExpectedVersion:2, ResultSummaryRef:"race-summary-a-"+suffix,
			SummarySchemaVersion:"summary-v1", OutboxRef:"race-outbox-a-"+suffix, TerminalStatus:domain.RunSucceeded,
			Summary:domain.ResultSummary{StageOutcomes:map[string]domain.StageStatus{"context":domain.StageSucceeded},
				ExcludedStages:[]domain.ExcludedStage{}, MissingStageCodes:[]string{}, DegradedStageCodes:[]string{},
				BlockedStageCodes:[]string{}, ReasonCodes:[]string{}, RiskGateDecisions:map[string]string{},
				Coverage:1, QualityStatus:"valid", ActionabilityStatus:"not_evaluated"}},
		{RunRef:runRef, ExpectedVersion:2, ResultSummaryRef:"race-summary-b-"+suffix,
			SummarySchemaVersion:"summary-v1", OutboxRef:"race-outbox-b-"+suffix, TerminalStatus:domain.RunSucceeded,
			Summary:domain.ResultSummary{StageOutcomes:map[string]domain.StageStatus{"context":domain.StageSucceeded},
				ExcludedStages:[]domain.ExcludedStage{}, MissingStageCodes:[]string{}, DegradedStageCodes:[]string{},
				BlockedStageCodes:[]string{}, ReasonCodes:[]string{}, RiskGateDecisions:map[string]string{},
				Coverage:1, QualityStatus:"valid", ActionabilityStatus:"not_evaluated"}},
	}
	start := make(chan struct{})
	type outcome struct { updated bool; err error }
	results := make(chan outcome, 2)
	var wait sync.WaitGroup
	for _, input := range inputs {
		input := input
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			updated, err := repository.FinalizeRun(ctx, input)
			results <- outcome{updated:updated, err:err}
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	successes, finalizedLosers := 0, 0
	for result := range results {
		switch {
		case result.err == nil && result.updated:
			successes++
		case errors.Is(result.err, ErrRunAlreadyFinalized):
			finalizedLosers++
		default:
			t.Fatalf("unexpected competing finalizer result updated=%v err=%v", result.updated, result.err)
		}
	}
	if successes != 1 || finalizedLosers != 1 {
		t.Fatalf("competing finalizers: success=%d already_finalized=%d, want 1/1", successes, finalizedLosers)
	}
	var runStatus, summaryRef string
	var version uint64
	if err := db.QueryRowContext(ctx, "SELECT status, version, result_summary_ref FROM researchflow_runs WHERE run_ref=?", runRef).
		Scan(&runStatus, &version, &summaryRef); err != nil { t.Fatal(err) }
	if domain.RunStatus(runStatus) != domain.RunSucceeded || version != 3 {
		t.Fatalf("final Run status=%s version=%d, want succeeded/3", runStatus, version)
	}
	var summaries, events int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM researchflow_result_summaries WHERE run_ref=?", runRef).Scan(&summaries); err != nil { t.Fatal(err) }
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM researchflow_outbox WHERE run_ref=? AND event_type=?", runRef, runFinalizedEventType).Scan(&events); err != nil { t.Fatal(err) }
	if summaries != 1 || events != 1 {
		t.Fatalf("persisted summaries=%d events=%d, want 1/1 (summary_ref=%s)", summaries, events, summaryRef)
	}
}
