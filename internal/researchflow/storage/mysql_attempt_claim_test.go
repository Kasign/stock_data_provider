package storage

import (
	"context"
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

func TestClaimAttemptValidatesArgumentsWithoutDatabase(t *testing.T) {
	repository := &MySQLRunRepository{}
	_, _, err := repository.ClaimAttempt(context.Background(), "run", "stage", "attempt", "worker", time.Minute, 3, 8)
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
	runRef, stageRef, secondStageRef := "claim-run-"+suffix, "claim-stage-"+suffix, "claim-stage-second-"+suffix
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
		Stages: []domain.Stage{
			{
				Ref: stageRef, RunRef: runRef, Code: "context", Status: domain.StageReady,
				Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
				DiagnosticRefs: []string{}, ReasonCodes: []string{},
			},
			{
				Ref: secondStageRef, RunRef: runRef, Code: "quant", Status: domain.StageReady,
				Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
				DiagnosticRefs: []string{}, ReasonCodes: []string{},
			},
			{
				Ref: "claim-stage-third-" + suffix, RunRef: runRef, Code: "research", Status: domain.StageReady,
				Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
				DiagnosticRefs: []string{}, ReasonCodes: []string{},
			},
			{
				Ref: "claim-stage-fourth-" + suffix, RunRef: runRef, Code: "report", Status: domain.StageReady,
				Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
				DiagnosticRefs: []string{}, ReasonCodes: []string{},
			},
		},
		ResolvedTemplate: template.ResolvedTemplate{
			Ref: "claim-template", Version: "v1", Fingerprint: templateFingerprint,
			Stages: []template.ResolvedStage{{Definition: template.StageDefinition{Code: "context"}}, {Definition: template.StageDefinition{Code: "quant"}}, {Definition: template.StageDefinition{Code: "research"}}, {Definition: template.StageDefinition{Code: "report"}}},
		},
		Parameters: json.RawMessage(`{}`),
	}
	if _, err := repository.CreateRunAtomic(ctx, record); err != nil {
		t.Fatalf("create run fixture: %v", err)
	}

	first, replayed, err := repository.ClaimAttempt(ctx, runRef, stageRef, "claim-attempt-1-"+suffix, "worker-a", time.Minute, 3, 8)
	if err != nil || replayed {
		t.Fatalf("first claim replayed=%v err=%v", replayed, err)
	}
	if first.Number != 1 || first.FencingToken != 1 || first.Status != domain.AttemptRunning {
		t.Fatalf("first claim returned unexpected attempt: %#v", first)
	}
	if _, _, err := repository.ClaimAttempt(ctx, runRef, secondStageRef,
		"claim-attempt-over-limit-"+suffix, "worker-limit", time.Minute, 3, 1); !errors.Is(err, ErrRunConcurrencyLimit) {
		t.Fatalf("claim above Run parallel limit error=%v, want ErrRunConcurrencyLimit", err)
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
	if _, _, err := repository.ClaimAttempt(ctx, runRef, stageRef, "invalid-claim-"+suffix, "worker-a", time.Minute, 3, 8); !errors.Is(err, ErrClaimStageUnavailable) {
		t.Fatalf("claim with inconsistent stage state error=%v, want ErrClaimStageUnavailable", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_stages SET status = ? WHERE stage_ref = ?`, string(domain.StageRunning), stageRef); err != nil {
		t.Fatalf("restore stage state: %v", err)
	}

	replay, replayed, err := repository.ClaimAttempt(ctx, runRef, stageRef, "ignored-attempt-"+suffix, "worker-a", time.Minute, 3, 8)
	if err != nil || !replayed || replay.Ref != first.Ref || replay.FencingToken != first.FencingToken {
		t.Fatalf("same-worker claim replay=%v attempt=%#v err=%v", replayed, replay, err)
	}
	if _, _, err := repository.ClaimAttempt(ctx, runRef, stageRef, "claim-attempt-other-"+suffix, "worker-b", time.Minute, 3, 8); !errors.Is(err, ErrAttemptLeaseHeld) {
		t.Fatalf("other-worker claim error=%v, want ErrAttemptLeaseHeld", err)
	}

	expired := formatResearchflowTimeValue(now.Add(-time.Minute))
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_attempts SET lease_expires_at = ? WHERE attempt_ref = ?`, expired, first.Ref); err != nil {
		t.Fatalf("expire first lease: %v", err)
	}
	if _, err := repository.RenewAttemptLease(ctx, runRef, stageRef, first.Ref, "worker-a", first.FencingToken, time.Minute); !errors.Is(err, ErrAttemptLeaseHeld) {
		t.Fatalf("expired lease renewal error=%v, want ErrAttemptLeaseHeld", err)
	}
	second, replayed, err := repository.ClaimAttempt(ctx, runRef, stageRef, "claim-attempt-2-"+suffix, "worker-b", time.Minute, 3, 8)
	if err != nil || replayed {
		t.Fatalf("reclaim replayed=%v err=%v", replayed, err)
	}
	if second.Number != 2 || second.FencingToken != 3 || second.Ref == first.Ref {
		t.Fatalf("reclaim did not advance attempt/fencing token: first=%#v second=%#v", first, second)
	}
	loadedFirst, err := repository.GetAttempt(ctx, first.Ref)
	if err != nil {
		t.Fatalf("load expired attempt: %v", err)
	}
	if loadedFirst.Status != domain.AttemptTimedOut || loadedFirst.LeaseOwner != "" || loadedFirst.LeaseExpiresAt != nil || loadedFirst.FencingToken != 2 {
		t.Fatalf("expired attempt was not fully fenced and released: %#v", loadedFirst)
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
	if _, _, err := repository.ClaimAttempt(ctx, runRef, stageRef, "invalid-retry-history-"+suffix, "worker-c", time.Minute, 3, 8); !errors.Is(err, ErrClaimStageUnavailable) {
		t.Fatalf("retry after successful attempt error=%v, want ErrClaimStageUnavailable", err)
	}

	if _, err := db.ExecContext(ctx, `UPDATE researchflow_attempts SET status = ? WHERE attempt_ref = ?`,
		string(domain.AttemptFailed), second.Ref); err != nil {
		t.Fatalf("prepare failed retry attempt: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_stages SET retry_at = NULL WHERE stage_ref = ?`, stageRef); err != nil {
		t.Fatalf("clear retry_at: %v", err)
	}
	if _, _, err := repository.ClaimAttempt(ctx, runRef, stageRef, "invalid-retry-time-"+suffix, "worker-c", time.Minute, 3, 8); !errors.Is(err, ErrClaimStageUnavailable) {
		t.Fatalf("retry without retry_at error=%v, want ErrClaimStageUnavailable", err)
	}

	if _, err := db.ExecContext(ctx, `UPDATE researchflow_stages SET retry_at = ? WHERE stage_ref = ?`, retryAt, stageRef); err != nil {
		t.Fatalf("restore retry_at: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_runs SET status = ? WHERE run_ref = ?`, string(domain.RunQueued), runRef); err != nil {
		t.Fatalf("move run out of running state: %v", err)
	}
	if _, _, err := repository.ClaimAttempt(ctx, runRef, stageRef, "invalid-retry-run-state-"+suffix, "worker-c", time.Minute, 3, 8); !errors.Is(err, ErrClaimStageUnavailable) {
		t.Fatalf("retry while run is queued error=%v, want ErrClaimStageUnavailable", err)
	}

	// An expired lease on another still-running Stage must not consume a
	// parallel slot or block a ready Stage from being claimed.
	thirdStageRef := "claim-stage-third-" + suffix
	expiredOtherAttemptRef := "claim-expired-other-attempt-" + suffix
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_runs SET status = ? WHERE run_ref = ?`,
		string(domain.RunRunning), runRef); err != nil {
		t.Fatalf("restore running Run for expired-peer lease test: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_stages SET status = ?, attempt_no = 1 WHERE stage_ref = ?`,
		string(domain.StageRunning), secondStageRef); err != nil {
		t.Fatalf("mark peer Stage running: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO researchflow_attempts (
		attempt_ref, run_ref, stage_ref, attempt_no, status, lease_owner,
		lease_expires_at, fencing_token, started_at, finished_at, fault_ref, output_ref
	) VALUES (?, ?, ?, 1, ?, ?, ?, 1, ?, NULL, NULL, NULL)`,
		expiredOtherAttemptRef, runRef, secondStageRef, string(domain.AttemptRunning),
		"worker-expired", formatResearchflowTimeValue(time.Now().UTC().Add(-time.Minute)),
		formatResearchflowTimeValue(time.Now().UTC().Add(-2*time.Minute))); err != nil {
		t.Fatalf("insert expired peer Attempt: %v", err)
	}
	third, replayed, err := repository.ClaimAttempt(ctx, runRef, thirdStageRef,
		"claim-third-after-peer-expiry-"+suffix, "worker-third", time.Minute, 3, 1)
	if err != nil || replayed {
		t.Fatalf("claim with expired peer lease replayed=%v err=%v", replayed, err)
	}
	if third.Number != 1 || third.FencingToken != 1 || third.Status != domain.AttemptRunning {
		t.Fatalf("claim after peer lease expiry returned unexpected Attempt: %#v", third)
	}

	fourthStageRef := "claim-stage-fourth-" + suffix
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_attempts SET lease_expires_at = ? WHERE attempt_ref = ?`,
		formatResearchflowTimeValue(time.Now().UTC().Add(-time.Minute)), third.Ref); err != nil {
		t.Fatalf("expire third Stage lease before concurrent budget test: %v", err)
	}
	type claimResult struct {
		attempt domain.Attempt
		replayed bool
		err error
	}
	start := make(chan struct{})
	results := make(chan claimResult, 2)
	var wg sync.WaitGroup
	claims := []struct{ stageRef, attemptRef, worker string }{
		{secondStageRef, "claim-concurrent-second-" + suffix, "worker-concurrent-a"},
		{fourthStageRef, "claim-concurrent-fourth-" + suffix, "worker-concurrent-b"},
	}
	for _, item := range claims {
		item := item
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			attempt, wasReplay, claimErr := repository.ClaimAttempt(
				ctx, runRef, item.stageRef, item.attemptRef, item.worker, time.Minute, 3, 1,
			)
			results <- claimResult{attempt: attempt, replayed: wasReplay, err: claimErr}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	successes, limited := 0, 0
	for result := range results {
		switch {
		case result.err == nil && !result.replayed:
			successes++
		case errors.Is(result.err, ErrRunConcurrencyLimit):
			limited++
		default:
			t.Fatalf("unexpected concurrent ClaimAttempt result: replayed=%v err=%v", result.replayed, result.err)
		}
	}
	if successes != 1 || limited != 1 {
		t.Fatalf("maxParallel=1 concurrent claims: successes=%d limited=%d, want 1 each", successes, limited)
	}
}

func stringRef(value string) *string { return &value }
