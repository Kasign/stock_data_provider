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
	budgetStageRef := "claim-stage-budget-" + suffix
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
				Ref: stageRef, RunRef: runRef, Code: "context", Status: domain.StagePending,
				Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
				DiagnosticRefs: []string{}, ReasonCodes: []string{},
			},
			{
				Ref: secondStageRef, RunRef: runRef, Code: "quant", Status: domain.StagePending,
				Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
				DiagnosticRefs: []string{}, ReasonCodes: []string{},
			},
			{
				Ref: "claim-stage-third-" + suffix, RunRef: runRef, Code: "research", Status: domain.StagePending,
				Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
				DiagnosticRefs: []string{}, ReasonCodes: []string{},
			},
			{
				Ref: "claim-stage-fourth-" + suffix, RunRef: runRef, Code: "report", Status: domain.StagePending,
				Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
				DiagnosticRefs: []string{}, ReasonCodes: []string{},
			},
			{
				Ref: budgetStageRef, RunRef: runRef, Code: "budget", Status: domain.StagePending,
				Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
				DiagnosticRefs: []string{}, ReasonCodes: []string{},
			},
		},
		ResolvedTemplate: template.ResolvedTemplate{
			Ref: "claim-template", Version: "v1", Fingerprint: templateFingerprint,
			Stages: []template.ResolvedStage{{Definition: template.StageDefinition{Code: "context"}}, {Definition: template.StageDefinition{Code: "quant"}}, {Definition: template.StageDefinition{Code: "research"}}, {Definition: template.StageDefinition{Code: "report"}}, {Definition: template.StageDefinition{Code: "budget"}}},
		},
		Parameters: json.RawMessage(`{}`),
	}
	if _, err := repository.CreateRunAtomic(ctx, record); err != nil {
		t.Fatalf("create run fixture: %v", err)
	}
	for _, stage := range record.Stages {
		ready := stage
		ready.Status = domain.StageReady
		ready.Version++
		if updated, err := repository.CompareAndSwapStage(ctx, stage.Version, ready); err != nil || !updated {
			t.Fatalf("prepare Ready Stage %q updated=%v err=%v", stage.Ref, updated, err)
		}
	}

	// Exhausting the budget through an expired final lease must atomically
	// close both Attempt and Stage instead of rolling back to a stuck running state.
	budgetAttempt, replayed, err := repository.ClaimAttempt(ctx, runRef, budgetStageRef,
		"claim-budget-attempt-"+suffix, "worker-budget", time.Minute, 1, 8)
	if err != nil || replayed {
		t.Fatalf("claim final-budget Attempt replayed=%v err=%v", replayed, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_attempts SET lease_expires_at = ?
		WHERE attempt_ref = ?`, formatResearchflowTimeValue(time.Now().UTC().Add(-time.Minute)), budgetAttempt.Ref); err != nil {
		t.Fatalf("expire final-budget Attempt lease: %v", err)
	}
	if _, _, err := repository.ClaimAttempt(ctx, runRef, budgetStageRef,
		"claim-budget-overrun-"+suffix, "worker-budget-reclaim", time.Minute, 1, 8);
		!errors.Is(err, ErrAttemptLimitReached) {
		t.Fatalf("claim after final Attempt lease expiry error=%v, want ErrAttemptLimitReached", err)
	}
	closedBudgetAttempt, err := repository.GetAttempt(ctx, budgetAttempt.Ref)
	if err != nil || closedBudgetAttempt.Status != domain.AttemptTimedOut ||
		closedBudgetAttempt.LeaseOwner != "" || closedBudgetAttempt.LeaseExpiresAt != nil ||
		closedBudgetAttempt.FinishedAt == nil {
		t.Fatalf("final-budget Attempt was not terminalized: attempt=%#v err=%v", closedBudgetAttempt, err)
	}
	closedBudgetStage, err := repository.GetStage(ctx, budgetStageRef)
	if err != nil || closedBudgetStage.Status != domain.StageTimedOut ||
		closedBudgetStage.FinishedAt == nil || closedBudgetStage.RetryAt != nil ||
		closedBudgetStage.AttemptNo != 1 {
		t.Fatalf("final-budget Stage was not closed consistently: stage=%#v err=%v", closedBudgetStage, err)
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
	// Storage must enforce retry scheduling metadata independently of Application.
	missingRetryAt := currentCommit
	missingRetryAt.AttemptStatus = domain.AttemptFailed
	missingRetryAt.Stage.Status = domain.StageRetryWait
	missingRetryAt.Stage.RetryAt = nil
	if err := repository.CommitAttemptResult(ctx, missingRetryAt); err == nil {
		t.Fatal("retry_wait result without retry_at must be rejected")
	}
	staleRetryAt := currentCommit
	staleRetryAt.Stage.RetryAt = &finished
	if err := repository.CommitAttemptResult(ctx, staleRetryAt); err == nil {
		t.Fatal("terminal Stage result with retry_at must be rejected")
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
	if loadedSecond.LeaseOwner != "" || loadedSecond.LeaseExpiresAt != nil {
		t.Fatalf("terminal Attempt retained lease ownership: %#v", loadedSecond)
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

	// A missing historical row must be rejected before the same-worker
	// idempotent replay path. The latest Attempt alone is not sufficient proof
	// that the Stage counter and its full history agree.
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_stages
		SET status = ?, version = version + 1 WHERE stage_ref = ?`,
		string(domain.StageRunning), stageRef); err != nil {
		t.Fatalf("prepare replay against incomplete history: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_attempts SET
		status = ?, lease_owner = ?, lease_expires_at = ?
		WHERE attempt_ref = ?`,
		string(domain.AttemptRunning), "worker-b",
		formatResearchflowTimeValue(time.Now().UTC().Add(time.Minute)), second.Ref); err != nil {
		t.Fatalf("prepare live latest Attempt for incomplete-history replay: %v", err)
	}
	// Terminal historical Attempts must not retain worker ownership or an
	// active lease; running Attempts must have a live-state shape and no finish time.
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_attempts SET
		status = ?, lease_owner = ?, lease_expires_at = ?, finished_at = NULL
		WHERE attempt_ref = ?`,
		string(domain.AttemptFailed), "stale-worker",
		formatResearchflowTimeValue(time.Now().UTC().Add(time.Minute)), first.Ref); err != nil {
		t.Fatalf("inject terminal Attempt with stale lease fields: %v", err)
	}
	if _, replayed, err := repository.ClaimAttempt(ctx, runRef, stageRef,
		"ignored-terminal-lease-replay-"+suffix, "worker-b", time.Minute, 3, 8);
		!errors.Is(err, ErrAttemptHistoryInconsistent) || replayed {
		t.Fatalf("claim with terminal Attempt lease residue replayed=%v err=%v, want ErrAttemptHistoryInconsistent", replayed, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_attempts SET
		status = ?, lease_owner = NULL, lease_expires_at = NULL,
		finished_at = COALESCE(finished_at, ?)
		WHERE attempt_ref = ?`,
		string(domain.AttemptTimedOut), formatResearchflowTimeValue(time.Now().UTC()), first.Ref); err != nil {
		t.Fatalf("restore historical Attempt lease state: %v", err)
	}

	// Fencing generations must increase with attempt_no. A lower token on a
	// later Attempt could otherwise let an older worker generation remain valid.
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_attempts
		SET fencing_token = 99 WHERE attempt_ref = ?`, first.Ref); err != nil {
		t.Fatalf("corrupt historical fencing order: %v", err)
	}
	if _, replayed, err := repository.ClaimAttempt(ctx, runRef, stageRef,
		"ignored-token-order-replay-"+suffix, "worker-b", time.Minute, 3, 8);
		!errors.Is(err, ErrAttemptHistoryInconsistent) || replayed {
		t.Fatalf("claim with non-monotonic fencing history replayed=%v err=%v, want ErrAttemptHistoryInconsistent", replayed, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_attempts
		SET fencing_token = 2 WHERE attempt_ref = ?`, first.Ref); err != nil {
		t.Fatalf("restore historical fencing token: %v", err)
	}
	// Stage.started_at must remain the first Attempt's start across retries.
	shiftedStageStart := formatResearchflowTimeValue(first.StartedAt.Add(time.Second))
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_stages SET started_at = ? WHERE stage_ref = ?`,
		shiftedStageStart, stageRef); err != nil {
		t.Fatalf("corrupt Stage first-start timestamp: %v", err)
	}
	if _, replayed, err := repository.ClaimAttempt(ctx, runRef, stageRef,
		"ignored-stage-start-replay-"+suffix, "worker-b", time.Minute, 3, 8);
		!errors.Is(err, ErrAttemptHistoryInconsistent) || replayed {
		t.Fatalf("claim with mismatched Stage.started_at replayed=%v err=%v, want ErrAttemptHistoryInconsistent", replayed, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_stages SET started_at = ? WHERE stage_ref = ?`,
		formatResearchflowTimeValue(first.StartedAt), stageRef); err != nil {
		t.Fatalf("restore Stage first-start timestamp: %v", err)
	}

	// A running Stage cannot retain retry_at or finished_at metadata.
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_stages
		SET finished_at = ?, retry_at = ? WHERE stage_ref = ?`,
		formatResearchflowTimeValue(time.Now().UTC()), formatResearchflowTimeValue(time.Now().UTC()), stageRef); err != nil {
		t.Fatalf("inject invalid running Stage timestamps: %v", err)
	}
	if _, replayed, err := repository.ClaimAttempt(ctx, runRef, stageRef,
		"ignored-running-stage-time-replay-"+suffix, "worker-b", time.Minute, 3, 8);
		!errors.Is(err, ErrAttemptHistoryInconsistent) || replayed {
		t.Fatalf("claim with invalid running Stage timestamps replayed=%v err=%v, want ErrAttemptHistoryInconsistent", replayed, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_stages
		SET finished_at = NULL, retry_at = NULL WHERE stage_ref = ?`, stageRef); err != nil {
		t.Fatalf("restore running Stage timestamps: %v", err)
	}

	// Attempt start/finish timestamps must form a valid interval.
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_attempts
		SET finished_at = ? WHERE attempt_ref = ?`,
		formatResearchflowTimeValue(second.StartedAt.Add(-time.Second)), second.Ref); err != nil {
		t.Fatalf("corrupt Attempt finish timestamp: %v", err)
	}
	if _, replayed, err := repository.ClaimAttempt(ctx, runRef, stageRef,
		"ignored-attempt-time-replay-"+suffix, "worker-b", time.Minute, 3, 8);
		!errors.Is(err, ErrAttemptHistoryInconsistent) || replayed {
		t.Fatalf("claim with inverted Attempt timestamps replayed=%v err=%v, want ErrAttemptHistoryInconsistent", replayed, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_attempts
		SET finished_at = started_at WHERE attempt_ref = ?`, second.Ref); err != nil {
		t.Fatalf("restore Attempt finish timestamp: %v", err)
	}

	if _, err := db.ExecContext(ctx, `DELETE FROM researchflow_attempts WHERE attempt_ref = ?`, first.Ref); err != nil {
		t.Fatalf("remove historical Attempt to simulate corruption: %v", err)
	}
	if _, replayed, err := repository.ClaimAttempt(ctx, runRef, stageRef,
		"ignored-corrupt-history-replay-"+suffix, "worker-b", time.Minute, 3, 8);
		!errors.Is(err, ErrAttemptHistoryInconsistent) || replayed {
		t.Fatalf("claim with missing historical Attempt replayed=%v err=%v, want ErrAttemptHistoryInconsistent", replayed, err)
	}
}

func stringRef(value string) *string { return &value }
