package storage

import (
	"context"
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

func TestBeginRunCancellationValidatesRepositoryAndArguments(t *testing.T) {
	repository := &MySQLRunRepository{}
	if _, err := repository.BeginRunCancellation(context.Background(), "run-1", 1); err == nil {
		t.Fatal("expected missing database to be rejected")
	}
	repository = &MySQLRunRepository{db: nil}
	if _, err := repository.BeginRunCancellation(context.Background(), "", 1); err == nil {
		t.Fatal("expected missing database to be rejected before argument validation")
	}
}

func TestMariaDBBeginRunCancellationLinearizesWorkerOperations(t *testing.T) {
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
	fingerprint := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	templateFingerprint := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	runRef, runningStageRef, readyStageRef := "cancel-run-"+suffix, "cancel-running-"+suffix, "cancel-ready-"+suffix
	record := application.CreateRunRecord{
		Scope:       application.IdempotencyScope{CallerRef: "researchflow-cancel-test", Operation: "CreateRun", Key: "cancel-" + suffix},
		Fingerprint: fingerprint,
		Run: domain.Run{
			Ref: runRef, SchemaVersion: "v1", Status: domain.RunCreated, Version: 1,
			SystemAsOf: now.Add(-time.Minute), DeadlineAt: now.Add(10 * time.Minute),
			TemplateRef: "cancel-template", TemplateVersion: "v1",
			TemplateFingerprint: templateFingerprint, InputSnapshotRefs: []string{"snapshot-" + suffix},
			CorrelationID: "cancel-correlation-" + suffix,
		},
		Stages: []domain.Stage{
			{Ref: runningStageRef, RunRef: runRef, Code: "running", Status: domain.StageReady, Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1", DiagnosticRefs: []string{}, ReasonCodes: []string{}},
			{Ref: readyStageRef, RunRef: runRef, Code: "ready", Status: domain.StageReady, Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1", DiagnosticRefs: []string{}, ReasonCodes: []string{}},
		},
		ResolvedTemplate: template.ResolvedTemplate{
			Ref: "cancel-template", Version: "v1", Fingerprint: templateFingerprint,
			Stages: []template.ResolvedStage{
				{Definition: template.StageDefinition{Code: "running"}},
				{Definition: template.StageDefinition{Code: "ready"}},
			},
		},
		Parameters: json.RawMessage(`{}`),
	}
	if _, err := repository.CreateRunAtomic(ctx, record); err != nil {
		t.Fatalf("create cancellation fixture: %v", err)
	}
	attempt, replayed, err := repository.ClaimAttempt(ctx, runRef, runningStageRef, "cancel-attempt-"+suffix, "worker-a", time.Minute, 2)
	if err != nil || replayed {
		t.Fatalf("claim running stage replayed=%v err=%v", replayed, err)
	}
	changed, err := repository.BeginRunCancellation(ctx, runRef, 2)
	if err != nil || !changed {
		t.Fatalf("begin cancellation changed=%v err=%v", changed, err)
	}
	changed, err = repository.BeginRunCancellation(ctx, runRef, 2)
	if err != nil || changed {
		t.Fatalf("idempotent cancellation changed=%v err=%v", changed, err)
	}
	if _, _, err := repository.ClaimAttempt(ctx, runRef, readyStageRef, "late-claim-"+suffix, "worker-b", time.Minute, 2); !errors.Is(err, ErrClaimRunUnavailable) {
		t.Fatalf("claim after cancellation error=%v, want ErrClaimRunUnavailable", err)
	}
	runningStage, err := repository.GetStage(ctx, runningStageRef)
	if err != nil || runningStage.Status != domain.StageRunning {
		t.Fatalf("in-flight stage was falsely stopped: stage=%#v err=%v", runningStage, err)
	}
	lateStageResult := runningStage
	lateStageResult.Version++
	lateStageResult.Status = domain.StageSucceeded
	if err := repository.CommitAttemptResult(ctx, AttemptResultCommit{
		RunRef: runRef, StageRef: runningStageRef, AttemptRef: attempt.Ref,
		WorkerID: "worker-a", FencingToken: attempt.FencingToken,
		AttemptStatus: domain.AttemptSucceeded, Stage: lateStageResult,
	}); !errors.Is(err, ErrClaimRunUnavailable) {
		t.Fatalf("result commit after cancellation error=%v, want ErrClaimRunUnavailable", err)
	}
	readyStage, err := repository.GetStage(ctx, readyStageRef)
	if err != nil || readyStage.Status != domain.StageCancelled || readyStage.FinishedAt == nil {
		t.Fatalf("not-yet-started stage was not cancelled: stage=%#v err=%v", readyStage, err)
	}
	run, err := repository.GetRun(ctx, runRef)
	if err != nil || run.Run.Status != domain.RunCancelling || run.Run.Version != 3 || run.Run.TerminationReason != "user_cancelled" {
		t.Fatalf("run cancellation state mismatch: run=%#v err=%v", run.Run, err)
	}
	loadedAttempt, err := repository.GetAttempt(ctx, attempt.Ref)
	if err != nil || loadedAttempt.Status != domain.AttemptRunning {
		t.Fatalf("in-flight attempt was falsely stopped: attempt=%#v err=%v", loadedAttempt, err)
	}

	if _, err := repository.ConfirmAttemptCancellation(ctx, runRef, runningStageRef, attempt.Ref,
		"worker-a", attempt.FencingToken+1, runningStage.Version); !errors.Is(err, ErrAttemptLeaseHeld) {
		t.Fatalf("stale fencing token cancellation error=%v, want ErrAttemptLeaseHeld", err)
	}
	confirmed, err := repository.ConfirmAttemptCancellation(ctx, runRef, runningStageRef, attempt.Ref,
		"worker-a", attempt.FencingToken, runningStage.Version)
	if err != nil || !confirmed {
		t.Fatalf("worker cancellation confirmation confirmed=%v err=%v", confirmed, err)
	}
	loadedAttempt, err = repository.GetAttempt(ctx, attempt.Ref)
	if err != nil || loadedAttempt.Status != domain.AttemptCancelled || loadedAttempt.LeaseOwner != "" || loadedAttempt.LeaseExpiresAt != nil {
		t.Fatalf("Attempt cancellation was not persisted: attempt=%#v err=%v", loadedAttempt, err)
	}
	runningStage, err = repository.GetStage(ctx, runningStageRef)
	if err != nil || runningStage.Status != domain.StageCancelled || runningStage.FinishedAt == nil {
		t.Fatalf("Stage cancellation was not persisted: stage=%#v err=%v", runningStage, err)
	}
	finalized, err := repository.FinalizeRun(ctx, RunFinalization{
		RunRef: runRef, ExpectedVersion: 3,
		ResultSummaryRef: "cancel-summary-" + suffix, SummarySchemaVersion: "summary-v1",
		OutboxRef: "cancel-outbox-" + suffix, TerminalStatus: domain.RunCancelled,
		TerminationReason: "user_cancelled",
		Summary: domain.ResultSummary{
			StageOutcomes: map[string]domain.StageStatus{
				"running": domain.StageCancelled, "ready": domain.StageCancelled,
			},
			ExcludedStages: []domain.ExcludedStage{}, MissingStageCodes: []string{},
			DegradedStageCodes: []string{}, BlockedStageCodes: []string{},
			ReasonCodes: []string{"user_cancelled"}, Coverage: 0,
			QualityStatus: "incomplete", RiskGateDecisions: map[string]string{},
			ActionabilityStatus: "not_evaluated",
		},
	})
	if err != nil || !finalized {
		t.Fatalf("finalize cancelled Run finalized=%v err=%v", finalized, err)
	}
	run, err = repository.GetRun(ctx, runRef)
	if err != nil || run.Run.Status != domain.RunCancelled || run.Run.Version != 4 {
		t.Fatalf("cancelled Run final state mismatch: run=%#v err=%v", run.Run, err)
	}
}
