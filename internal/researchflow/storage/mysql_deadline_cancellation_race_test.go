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

	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
	"example.com/stock-analysis-flow/internal/researchflow/application"
	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/template"
	"github.com/go-sql-driver/mysql"
)

func deadlineRaceDB(t *testing.T) (context.Context, context.CancelFunc, *sql.DB, *MySQLRunRepository) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true", os.Getenv("PLATFORM_MARIADB_USER"), os.Getenv("PLATFORM_MARIADB_PASSWORD"), os.Getenv("PLATFORM_MARIADB_ADDR"), os.Getenv("PLATFORM_MARIADB_DATABASE"))
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	db, err := platformstorage.OpenSQL(ctx, connector, platformstorage.DefaultSQLConfig())
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if err = platformstorage.ApplyMigrations(ctx, db, MigrationNamespace, Migrations()); err != nil {
		db.Close()
		cancel()
		t.Fatal(err)
	}
	repo, err := NewMySQLRunRepository(db)
	if err != nil {
		db.Close()
		cancel()
		t.Fatal(err)
	}
	return ctx, cancel, db, repo
}

func deadlineRaceFixture(t *testing.T, ctx context.Context, repo *MySQLRunRepository, suffix, prefix string, codes ...string) (string, []string) {
	t.Helper()
	now := time.Now().UTC()
	stages := make([]domain.Stage, 0, len(codes))
	resolved := make([]template.ResolvedStage, 0, len(codes))
	refs := make([]string, 0, len(codes))
	runRef := prefix + "-" + suffix
	for _, code := range codes {
		ref := prefix + "-" + code + "-" + suffix
		refs = append(refs, ref)
		stages = append(stages, domain.Stage{Ref: ref, RunRef: runRef, Code: code, Status: domain.StagePending, Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1", DiagnosticRefs: []string{}, ReasonCodes: []string{}})
		resolved = append(resolved, template.ResolvedStage{Definition: template.StageDefinition{Code: code}})
	}
	fp := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tfp := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	rec := application.CreateRunRecord{Scope: application.IdempotencyScope{CallerRef: prefix, Operation: "CreateRun", Key: suffix}, Fingerprint: fp, Run: domain.Run{Ref: runRef, SchemaVersion: "v1", Status: domain.RunCreated, Version: 1, SystemAsOf: now.Add(-time.Minute), DeadlineAt: now.Add(5 * time.Minute), TemplateRef: prefix + "-template", TemplateVersion: "v1", TemplateFingerprint: tfp, InputSnapshotRefs: []string{}, CorrelationID: prefix + "-" + suffix}, Stages: stages, ResolvedTemplate: template.ResolvedTemplate{Ref: prefix + "-template", Version: "v1", Fingerprint: tfp, Stages: resolved}, Parameters: json.RawMessage("{}")}
	if _, err := repo.CreateRunAtomic(ctx, rec); err != nil {
		t.Fatal(err)
	}
	for _, ref := range refs {
		stage, err := repo.GetStage(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		stage.Status = domain.StageReady
		stage.Version++
		if updated, err := repo.CompareAndSwapStage(ctx, stage.Version-1, stage); err != nil || !updated {
			t.Fatalf("prepare Ready Stage %q updated=%v err=%v", ref, updated, err)
		}
	}
	return runRef, refs
}

func TestMariaDBExpireRunDeadlineFencesRunningAttempt(t *testing.T) {
	if os.Getenv("PLATFORM_STORAGE_INTEGRATION") != "1" {
		t.Skip("set PLATFORM_STORAGE_INTEGRATION=1")
	}
	ctx, cancel, db, repo := deadlineRaceDB(t)
	defer cancel()
	defer db.Close()
	suffix := fmt.Sprint(time.Now().UnixNano())
	run, stages := deadlineRaceFixture(t, ctx, repo, suffix, "deadline-fence", "active", "pending")
	attempt, replayed, err := repo.ClaimAttempt(ctx, run, stages[0], "deadline-attempt-"+suffix, "deadline-worker", time.Minute, 2, 8)
	if err != nil || replayed {
		t.Fatalf("ClaimAttempt replayed=%v err=%v", replayed, err)
	}
	if _, err = db.ExecContext(ctx, "UPDATE researchflow_runs SET deadline_at=? WHERE run_ref=?", formatResearchflowTimeValue(time.Now().UTC().Add(-time.Minute)), run); err != nil {
		t.Fatal(err)
	}
	runSnapshot, err := repo.GetRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	expectedRunVersion := runSnapshot.Run.Version
	updated, version, err := repo.ExpireRunDeadline(ctx, run, expectedRunVersion)
	if err != nil || !updated || version != expectedRunVersion+1 {
		t.Fatalf("ExpireRunDeadline=%v/%d/%v expected version %d", updated, version, err, expectedRunVersion+1)
	}
	got, err := repo.GetAttempt(ctx, attempt.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.AttemptTimedOut || got.FencingToken != attempt.FencingToken+1 || got.LeaseOwner != "" || got.LeaseExpiresAt != nil {
		t.Fatalf("Attempt not fenced: %#v", got)
	}
	for i, ref := range stages {
		stage, e := repo.GetStage(ctx, ref)
		if e != nil {
			t.Fatal(e)
		}
		want := uint64(3)
		if i == 0 {
			want = 4
		}
		if stage.Status != domain.StageTimedOut || stage.Version != want {
			t.Fatalf("Stage %s=%s/v%d want timed_out/v%d", ref, stage.Status, stage.Version, want)
		}
	}
	if err := repo.CommitAttemptResult(ctx, AttemptResultCommit{RunRef: run, StageRef: stages[0], AttemptRef: attempt.Ref, WorkerID: "deadline-worker", FencingToken: attempt.FencingToken, AttemptStatus: domain.AttemptSucceeded, Stage: domain.Stage{Ref: stages[0], RunRef: run, Code: "active", Status: domain.StageSucceeded, Version: 3, AttemptNo: attempt.Number, OutputRef: stringRef("late-result-" + suffix)}, OutputRef: stringRef("late-result-" + suffix)}); !errors.Is(err, ErrClaimRunUnavailable) && !errors.Is(err, ErrClaimStageUnavailable) && !errors.Is(err, ErrAttemptLeaseHeld) {
		t.Fatalf("stale result err=%v", err)
	}
}

func TestMariaDBExpiredDeadlineRejectsUserCancellation(t *testing.T) {
	if os.Getenv("PLATFORM_STORAGE_INTEGRATION") != "1" {
		t.Skip("set PLATFORM_STORAGE_INTEGRATION=1")
	}
	ctx, cancel, db, repo := deadlineRaceDB(t)
	defer cancel()
	defer db.Close()

	suffix := fmt.Sprint(time.Now().UnixNano())
	runRef, _ := deadlineRaceFixture(t, ctx, repo, suffix, "deadline-cancel", "stage")
	if _, err := db.ExecContext(ctx, "UPDATE researchflow_runs SET deadline_at=? WHERE run_ref=?",
		formatResearchflowTimeValue(time.Now().UTC().Add(-time.Minute)), runRef); err != nil {
		t.Fatal(err)
	}

	changed, err := repo.BeginRunCancellation(ctx, runRef, 1)
	if changed || !errors.Is(err, ErrClaimRunUnavailable) ||
		!errors.Is(err, application.ErrRunCancellationUnavailable) {
		t.Fatalf("expired-deadline cancellation changed=%v err=%v; want rejected cancellation", changed, err)
	}
	updated, version, err := repo.ExpireRunDeadline(ctx, runRef, 1)
	if err != nil || !updated || version != 2 {
		t.Fatalf("ExpireRunDeadline updated=%v version=%d err=%v; want true/2/nil", updated, version, err)
	}
	loaded, err := repo.GetRun(ctx, runRef)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Run.Status != domain.RunCreated || loaded.Run.TerminationReason != "deadline_exceeded" ||
		loaded.Run.Version != 2 {
		t.Fatalf("expired Run state=%#v; want created/deadline_exceeded/v2", loaded.Run)
	}
}

func TestMariaDBExpiredDeadlineYieldsToCompletedStages(t *testing.T) {
	if os.Getenv("PLATFORM_STORAGE_INTEGRATION") != "1" {
		t.Skip("set PLATFORM_STORAGE_INTEGRATION=1")
	}
	ctx, cancel, db, repo := deadlineRaceDB(t)
	defer cancel()
	defer db.Close()

	suffix := fmt.Sprint(time.Now().UnixNano())
	runRef, stages := deadlineRaceFixture(t, ctx, repo, suffix, "deadline-completed", "completed")
	now := time.Now().UTC()
	started := formatResearchflowTimeValue(now.Add(-2 * time.Minute))
	finished := formatResearchflowTimeValue(now.Add(-time.Minute))
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_runs
		SET status = ?, version = version + 1, started_at = ?, deadline_at = ?
		WHERE run_ref = ?`, string(domain.RunRunning), started,
		formatResearchflowTimeValue(now.Add(-time.Minute)), runRef); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE researchflow_stages
		SET status = ?, version = version + 1, started_at = ?, finished_at = ?
		WHERE stage_ref = ?`, string(domain.StageSucceeded), started, finished, stages[0]); err != nil {
		t.Fatal(err)
	}

	updated, version, err := repo.ExpireRunDeadline(ctx, runRef, 2)
	if err != nil || updated || version != 2 {
		t.Fatalf("ExpireRunDeadline updated=%v version=%d err=%v; want false/2/nil", updated, version, err)
	}
	var reason sql.NullString
	if err := db.QueryRowContext(ctx, "SELECT termination_reason FROM researchflow_runs WHERE run_ref = ?", runRef).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason.Valid && reason.String != "" {
		t.Fatalf("completed Run acquired termination reason %q", reason.String)
	}

	finalized, err := repo.FinalizeRun(ctx, RunFinalization{
		RunRef: runRef, ExpectedVersion: 2,
		ResultSummaryRef:     "deadline-completed-summary-" + suffix,
		SummarySchemaVersion: "summary-v1", OutboxRef: "deadline-completed-outbox-" + suffix,
		TerminalStatus: domain.RunSucceeded,
		Summary: domain.ResultSummary{
			StageOutcomes:  map[string]domain.StageStatus{"completed": domain.StageSucceeded},
			ExcludedStages: []domain.ExcludedStage{}, MissingStageCodes: []string{},
			DegradedStageCodes: []string{}, BlockedStageCodes: []string{}, ReasonCodes: []string{},
			RiskGateDecisions: map[string]string{}, Coverage: 1, QualityStatus: "valid",
			ActionabilityStatus: "not_evaluated",
		},
	})
	if err != nil || !finalized {
		t.Fatalf("normal FinalizeRun finalized=%v err=%v; completed work must retain normal aggregation", finalized, err)
	}
	loaded, err := repo.GetRun(ctx, runRef)
	if err != nil || loaded.Run.Status != domain.RunSucceeded || loaded.Run.TerminationReason != "" {
		t.Fatalf("completed Run final state=%#v err=%v", loaded.Run, err)
	}
}

func TestMariaDBDeadlineExpirationWinsAgainstStaleSchedulerStageCAS(t *testing.T) {
	if os.Getenv("PLATFORM_STORAGE_INTEGRATION") != "1" {
		t.Skip("set PLATFORM_STORAGE_INTEGRATION=1")
	}
	ctx, cancel, db, repo := deadlineRaceDB(t)
	defer cancel()
	defer db.Close()

	suffix := fmt.Sprint(time.Now().UnixNano())
	runRef, stageRefs := deadlineRaceFixture(t, ctx, repo, suffix, "deadline-scheduler-race", "pending")
	stageRef := stageRefs[0]
	if _, err := db.ExecContext(ctx, "UPDATE researchflow_stages SET status = ? WHERE stage_ref = ?",
		string(domain.StagePending), stageRef); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE researchflow_runs SET deadline_at = ? WHERE run_ref = ?",
		formatResearchflowTimeValue(time.Now().UTC().Add(-time.Minute)), runRef); err != nil {
		t.Fatal(err)
	}

	staleStage, err := repo.GetStage(ctx, stageRef)
	if err != nil {
		t.Fatal(err)
	}
	scheduledStage := staleStage
	scheduledStage.Status = domain.StageReady
	scheduledStage.Version++

	start := make(chan struct{})
	type expirationOutcome struct {
		updated bool
		err     error
	}
	expiration := make(chan expirationOutcome, 1)
	type casOutcome struct {
		updated bool
		err     error
	}
	cas := make(chan casOutcome, 1)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		updated, _, expireErr := repo.ExpireRunDeadline(ctx, runRef, 1)
		expiration <- expirationOutcome{updated: updated, err: expireErr}
	}()
	go func() {
		defer wg.Done()
		<-start
		updated, casErr := repo.CompareAndSwapStage(ctx, staleStage.Version, scheduledStage)
		cas <- casOutcome{updated: updated, err: casErr}
	}()
	close(start)
	wg.Wait()
	close(expiration)
	close(cas)

	expireResult := <-expiration
	if expireResult.err != nil || !expireResult.updated {
		t.Fatalf("deadline expiration updated=%v err=%v; deadline must win", expireResult.updated, expireResult.err)
	}
	casResult := <-cas
	if casResult.err != nil {
		t.Fatalf("stale scheduler Stage CAS returned error: %v", casResult.err)
	}

	loadedRun, err := repo.GetRun(ctx, runRef)
	if err != nil {
		t.Fatal(err)
	}
	if loadedRun.Run.TerminationReason != "deadline_exceeded" || loadedRun.Run.Version != 2 {
		t.Fatalf("Run termination=%q version=%d; want deadline_exceeded/v2",
			loadedRun.Run.TerminationReason, loadedRun.Run.Version)
	}
	loadedStage, err := repo.GetStage(ctx, stageRef)
	if err != nil {
		t.Fatal(err)
	}
	wantVersion := uint64(3)
	if casResult.updated {
		wantVersion = 4
	}
	if loadedStage.Status != domain.StageTimedOut || loadedStage.Version != wantVersion {
		t.Fatalf("Stage status=%s version=%d; want timed_out/v%d (scheduler CAS applied=%v)",
			loadedStage.Status, loadedStage.Version, wantVersion, casResult.updated)
	}
}

func TestMariaDBCancellationConfirmationAndLeaseFencingHaveSingleWinner(t *testing.T) {
	if os.Getenv("PLATFORM_STORAGE_INTEGRATION") != "1" {
		t.Skip("set PLATFORM_STORAGE_INTEGRATION=1")
	}
	ctx, cancel, db, repo := deadlineRaceDB(t)
	defer cancel()
	defer db.Close()

	suffix := fmt.Sprint(time.Now().UnixNano())
	runRef, stageRefs := deadlineRaceFixture(t, ctx, repo, suffix, "cancel-attempt-race", "running")
	stageRef := stageRefs[0]
	attempt, replayed, err := repo.ClaimAttempt(ctx, runRef, stageRef,
		"cancel-attempt-"+suffix, "cancel-worker", time.Minute, 2, 8)
	if err != nil || replayed {
		t.Fatalf("ClaimAttempt replayed=%v err=%v", replayed, err)
	}
	runSnapshot, err := repo.GetRun(ctx, runRef)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := repo.BeginRunCancellation(ctx, runRef, runSnapshot.Run.Version)
	if err != nil || !changed {
		t.Fatalf("BeginRunCancellation changed=%v err=%v", changed, err)
	}
	stage, err := repo.GetStage(ctx, stageRef)
	if err != nil {
		t.Fatal(err)
	}

	// 将租约放在竞争启动点之后的短窗口：先取得 Run 锁的一方
	// 根据同一数据库时间快照决定是协作确认还是过期回收。
	expiresAt := time.Now().UTC().Add(500 * time.Millisecond)
	if _, err := db.ExecContext(ctx, "UPDATE researchflow_attempts SET lease_expires_at = ? WHERE attempt_ref = ?",
		formatResearchflowTimeValue(expiresAt), attempt.Ref); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	type outcome struct {
		confirmed bool
		fenced    bool
		err       error
	}
	results := make(chan outcome, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		confirmed, confirmErr := repo.ConfirmAttemptCancellation(ctx, runRef, stageRef,
			attempt.Ref, "cancel-worker", attempt.FencingToken, stage.Version)
		results <- outcome{confirmed: confirmed, err: confirmErr}
	}()
	go func() {
		defer wg.Done()
		<-start
		fenced, fenceErr := repo.FenceExpiredCancellationAttempt(ctx, runRef, stageRef,
			attempt.Ref, stage.Version)
		results <- outcome{fenced: fenced, err: fenceErr}
	}()
	close(start)
	wg.Wait()
	close(results)

	confirmedWins, fencedWins := 0, 0
	for result := range results {
		if result.confirmed && result.fenced {
			t.Fatalf("one transaction reported two outcomes: %#v", result)
		}
		if result.confirmed {
			if result.err != nil {
				t.Fatalf("successful cancellation confirmation returned error: %v", result.err)
			}
			confirmedWins++
			continue
		}
		if result.fenced {
			if result.err != nil {
				t.Fatalf("successful lease fencing returned error: %v", result.err)
			}
			fencedWins++
			continue
		}
		if result.err == nil {
			t.Fatalf("losing cancellation transaction returned neither success nor error: %#v", result)
		}
		if !errors.Is(result.err, ErrClaimStageUnavailable) && !errors.Is(result.err, ErrAttemptLeaseHeld) {
			t.Fatalf("unexpected losing cancellation transaction error: %v", result.err)
		}
	}
	if confirmedWins+fencedWins != 1 {
		t.Fatalf("cancellation confirmation winners=%d lease fencing winners=%d; want exactly one",
			confirmedWins, fencedWins)
	}

	loadedAttempt, err := repo.GetAttempt(ctx, attempt.Ref)
	if err != nil {
		t.Fatal(err)
	}
	loadedStage, err := repo.GetStage(ctx, stageRef)
	if err != nil {
		t.Fatal(err)
	}
	if loadedStage.Status != domain.StageCancelled || loadedStage.Version != stage.Version+1 ||
		loadedStage.FinishedAt == nil {
		t.Fatalf("cancelled Stage state=%#v; want cancelled/version+1/finished", loadedStage)
	}
	if loadedAttempt.LeaseOwner != "" || loadedAttempt.LeaseExpiresAt != nil || loadedAttempt.FinishedAt == nil {
		t.Fatalf("Attempt lease was not released: %#v", loadedAttempt)
	}
	if confirmedWins == 1 && loadedAttempt.Status != domain.AttemptCancelled {
		t.Fatalf("cooperative confirmation winner left Attempt status %s", loadedAttempt.Status)
	}
	if fencedWins == 1 && (loadedAttempt.Status != domain.AttemptTimedOut ||
		loadedAttempt.FencingToken != attempt.FencingToken+1) {
		t.Fatalf("lease fencing winner left Attempt state=%#v", loadedAttempt)
	}
	loadedRun, err := repo.GetRun(ctx, runRef)
	if err != nil {
		t.Fatal(err)
	}
	if loadedRun.Run.Status != domain.RunCancelling || loadedRun.Run.TerminationReason != "user_cancelled" {
		t.Fatalf("Run cancellation state changed during Attempt race: %#v", loadedRun.Run)
	}
}
