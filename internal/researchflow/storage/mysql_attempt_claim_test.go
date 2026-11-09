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

func TestClaimAttemptValidatesArgumentsWithoutDatabase(t *testing.T) {
	repository := &MySQLRunRepository{}
	_, _, err := repository.ClaimAttempt(context.Background(), "run", "stage", "attempt", "worker", time.Minute, 3)
	if err == nil {
		t.Fatal("expected missing database to be rejected")
	}
}

func TestMariaDBClaimAttemptLeaseAndFencing(t *testing.T) {
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
	templateFingerprint := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	runRef, stageRef := "claim-run-"+suffix, "claim-stage-"+suffix
	record := application.CreateRunRecord{
		Scope: application.IdempotencyScope{
			CallerRef: "researchflow-claim-test", Operation: "CreateRun", Key: "claim-"+suffix,
		},
		Fingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Run: domain.Run{
			Ref: runRef, SchemaVersion: "v1", Status: domain.RunCreated, Version: 1,
			SystemAsOf: now.Add(-time.Minute), DeadlineAt: now.Add(10 * time.Minute),
			TemplateRef: "claim-template", TemplateVersion: "v1",
			TemplateFingerprint: templateFingerprint, InputSnapshotRefs: []string{"snapshot-"+suffix},
			CorrelationID: "claim-correlation-"+suffix,
		},
		Stages: []domain.Stage{{
			Ref: stageRef, RunRef: runRef, Code: "context", Status: domain.StageReady,
			Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
			DiagnosticRefs: []string{}, ReasonCodes: []string{},
		}},
		ResolvedTemplate: template.ResolvedTemplate{
			Ref: "claim-template", Version: "v1", Fingerprint: templateFingerprint,
			Stages: []template.ResolvedStage{{Definition: template.StageDefinition{Code: "context"}}},
		},
		Parameters: json.RawMessage(`{}`),
	}
	if _, err := repository.CreateRunAtomic(ctx, record); err != nil {
		t.Fatalf("create run fixture: %v", err)
	}

	first, replayed, err := repository.ClaimAttempt(ctx, runRef, stageRef, "claim-attempt-1-"+suffix, "worker-a", time.Minute, 3)
	if err != nil || replayed {
		t.Fatalf("first claim replayed=%v err=%v", replayed, err)
	}
	if first.Number != 1 || first.FencingToken != 1 || first.Status != domain.AttemptRunning {
		t.Fatalf("first claim returned unexpected attempt: %#v", first)
	}

	// A running Attempt and a non-running Stage indicate inconsistent state;
	// Claim must fail closed instead of creating another Attempt.
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_stages SET status = ? WHERE stage_ref = ?`, string(domain.StageSucceeded), stageRef); err != nil {
		t.Fatalf("make stage state inconsistent: %v", err)
	}
	if _, _, err := repository.ClaimAttempt(ctx, runRef, stageRef, "invalid-claim-"+suffix, "worker-a", time.Minute, 3); !errors.Is(err, ErrClaimStageUnavailable) {
		t.Fatalf("claim with inconsistent stage state error=%v, want ErrClaimStageUnavailable", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_stages SET status = ? WHERE stage_ref = ?`, string(domain.StageRunning), stageRef); err != nil {
		t.Fatalf("restore stage state: %v", err)
	}

	replay, replayed, err := repository.ClaimAttempt(ctx, runRef, stageRef, "ignored-attempt-"+suffix, "worker-a", time.Minute, 3)
	if err != nil || !replayed || replay.Ref != first.Ref || replay.FencingToken != first.FencingToken {
		t.Fatalf("same-worker claim replay=%v attempt=%#v err=%v", replayed, replay, err)
	}
	if _, _, err := repository.ClaimAttempt(ctx, runRef, stageRef, "claim-attempt-other-"+suffix, "worker-b", time.Minute, 3); !errors.Is(err, ErrAttemptLeaseHeld) {
		t.Fatalf("other-worker claim error=%v, want ErrAttemptLeaseHeld", err)
	}

	expired := formatResearchflowTimeValue(now.Add(-time.Minute))
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_attempts SET lease_expires_at = ? WHERE attempt_ref = ?`, expired, first.Ref); err != nil {
		t.Fatalf("expire first lease: %v", err)
	}
	second, replayed, err := repository.ClaimAttempt(ctx, runRef, stageRef, "claim-attempt-2-"+suffix, "worker-b", time.Minute, 3)
	if err != nil || replayed {
		t.Fatalf("reclaim replayed=%v err=%v", replayed, err)
	}
	if second.Number != 2 || second.FencingToken != 2 || second.Ref == first.Ref {
		t.Fatalf("reclaim did not advance attempt/fencing token: first=%#v second=%#v", first, second)
	}
	loadedFirst, err := repository.GetAttempt(ctx, first.Ref)
	if err != nil {
		t.Fatalf("load expired attempt: %v", err)
	}
	if loadedFirst.Status != domain.AttemptTimedOut {
		t.Fatalf("expired attempt status=%q, want timed_out", loadedFirst.Status)
	}

	stage, err := repository.GetStage(ctx, stageRef)
	if err != nil {
		t.Fatalf("load stage before result commit: %v", err)
	}
	finished := time.Now().UTC()
	nextStage := stage
	nextStage.Version++
	nextStage.Status = domain.StageSucceeded
	nextStage.OutputRef = stringRef("result-" + suffix)
	nextStage.FinishedAt = &finished
	staleCommit := AttemptResultCommit{
		RunRef: runRef, StageRef: stageRef, AttemptRef: first.Ref,
		WorkerID: "worker-a", FencingToken: first.FencingToken,
		AttemptStatus: domain.AttemptSucceeded, Stage: nextStage,
	}
	if err := repository.CommitAttemptResult(ctx, staleCommit); !errors.Is(err, ErrAttemptLeaseHeld) {
		t.Fatalf("stale worker result error=%v, want ErrAttemptLeaseHeld", err)
	}

	currentCommit := AttemptResultCommit{
		RunRef: runRef, StageRef: stageRef, AttemptRef: second.Ref,
		WorkerID: "worker-b", FencingToken: second.FencingToken,
		AttemptStatus: domain.AttemptSucceeded, Stage: nextStage,
	}
	if err := repository.CommitAttemptResult(ctx, currentCommit); err != nil {
		t.Fatalf("current worker result commit: %v", err)
	}
	loadedSecond, err := repository.GetAttempt(ctx, second.Ref)
	if err != nil {
		t.Fatalf("load committed attempt: %v", err)
	}
	if loadedSecond.Status != domain.AttemptSucceeded || loadedSecond.OutputRef == nil ||
		*loadedSecond.OutputRef != "result-"+suffix {
		t.Fatalf("committed attempt has unexpected result: %#v", loadedSecond)
	}
	loadedStage, err := repository.GetStage(ctx, stageRef)
	if err != nil {
		t.Fatalf("load committed stage: %v", err)
	}
	if loadedStage.Status != domain.StageSucceeded || loadedStage.Version != nextStage.Version {
		t.Fatalf("committed stage has unexpected state: %#v", loadedStage)
	}
}

func stringRef(value string) *string { return &value }

