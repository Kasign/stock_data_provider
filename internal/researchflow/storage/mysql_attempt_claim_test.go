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
			CallerRef: "researchflow-claim-test", Operation: "CreateRun", Key: "claim-" + suffix,
		},
		Fingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Run: domain.Run{
			Ref: runRef, SchemaVersion: "v1", Status: domain.RunCreated, Version: 1,
			SystemAsOf: now.Add(-time.Minute), DeadlineAt: now.Add(10 * time.Minute),
			TemplateRef: "claim-template", TemplateVersion: "v1",
			TemplateFingerprint: templateFingerprint, InputSnapshotRefs: []string{"snapshot-" + suffix},
			CorrelationID: "claim-correlation-" + suffix,
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
	originalExpiry := *first.LeaseExpiresAt
	renewedExpiry, err := repository.RenewAttemptLease(
		ctx, runRef, stageRef, first.Ref, "worker-a", first.FencingToken, 2*time.Minute,
	)
	if err != nil || !renewedExpiry.After(originalExpiry) {
		t.Fatalf("renew lease expiry=%s original=%s err=%v", renewedExpiry, originalExpiry, err)
	}
	first.LeaseExpiresAt = &renewedExpiry
	if loaded, err := repository.GetAttempt(ctx, first.Ref); err != nil ||
		loaded.LeaseExpiresAt == nil || !loaded.LeaseExpiresAt.Equal(renewedExpiry) {
		t.Fatalf("renewed lease was not persisted: attempt=%#v err=%v", loaded, err)
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
	if _, err := repository.RenewAttemptLease(ctx, runRef, stageRef, first.Ref, "worker-a", first.FencingToken, time.Minute); !errors.Is(err, ErrAttemptLeaseHeld) {
		t.Fatalf("expired lease renewal error=%v, want ErrAttemptLeaseHeld", err)
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
		OutputRef: nextStage.OutputRef,
	}
	// Attempt and Stage must reference the same immutable output artifact.
	mismatchedOutput := currentCommit
	mismatchedOutput.OutputRef = stringRef("different-result-" + suffix)
	if err := repository.CommitAttemptResult(ctx, mismatchedOutput); err == nil {
		t.Fatal("result commit must reject different Attempt and Stage output refs")
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

	// RetryWait must follow a failed/timed-out attempt, carry a due retry_at,
	// and belong to a still-running Run. Corrupt states must fail closed.
	retryAt := formatResearchflowTimeValue(time.Now().UTC().Add(-time.Second))
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_stages
		SET status = ?, retry_at = ?, version = version + 1 WHERE stage_ref = ?`,
		string(domain.StageRetryWait), retryAt, stageRef); err != nil {
		t.Fatalf("prepare retry with successful latest attempt: %v", err)
	}
	if _, _, err := repository.ClaimAttempt(ctx, runRef, stageRef, "invalid-retry-history-"+suffix, "worker-c", time.Minute, 3); !errors.Is(err, ErrClaimStageUnavailable) {
		t.Fatalf("retry after successful attempt error=%v, want ErrClaimStageUnavailable", err)
	}

	if _, err := db.ExecContext(ctx, `UPDATE researchflow_attempts SET status = ? WHERE attempt_ref = ?`,
		string(domain.AttemptFailed), second.Ref); err != nil {
		t.Fatalf("prepare failed retry attempt: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_stages SET retry_at = NULL WHERE stage_ref = ?`, stageRef); err != nil {
		t.Fatalf("clear retry_at: %v", err)
	}
	if _, _, err := repository.ClaimAttempt(ctx, runRef, stageRef, "invalid-retry-time-"+suffix, "worker-c", time.Minute, 3); !errors.Is(err, ErrClaimStageUnavailable) {
		t.Fatalf("retry without retry_at error=%v, want ErrClaimStageUnavailable", err)
	}

	if _, err := db.ExecContext(ctx, `UPDATE researchflow_stages SET retry_at = ? WHERE stage_ref = ?`, retryAt, stageRef); err != nil {
		t.Fatalf("restore retry_at: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_runs SET status = ? WHERE run_ref = ?`, string(domain.RunQueued), runRef); err != nil {
		t.Fatalf("move run out of running state: %v", err)
	}
	if _, _, err := repository.ClaimAttempt(ctx, runRef, stageRef, "invalid-retry-run-state-"+suffix, "worker-c", time.Minute, 3); !errors.Is(err, ErrClaimStageUnavailable) {
		t.Fatalf("retry while run is queued error=%v, want ErrClaimStageUnavailable", err)
	}
}

func stringRef(value string) *string { return &value }
