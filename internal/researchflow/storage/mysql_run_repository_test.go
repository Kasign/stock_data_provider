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

func TestResearchflowTimestampUsesFixedUTCPrecision(t *testing.T) {
	input := time.Date(2026, 11, 9, 1, 2, 3, 123456789, time.FixedZone("offset", 8*60*60))
	encoded, err := formatResearchflowTime(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != 30 || encoded != "2026-11-08T17:02:03.123456789Z" {
		t.Fatalf("formatted timestamp = %q, want fixed-width UTC timestamp", encoded)
	}
	decoded, err := parseResearchflowTime(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.Equal(input) || decoded.Location() != time.UTC {
		t.Fatalf("decoded timestamp = %s (%s), want same instant in UTC", decoded, decoded.Location())
	}
}

func TestCalculateResearchflowLeaseExpiryCapsAtRunDeadline(t *testing.T) {
	now := time.Date(2026, 11, 10, 1, 2, 3, 123456789, time.UTC)
	deadline := now.Add(1500 * time.Millisecond)
	expiresAt, err := calculateResearchflowLeaseExpiry(now, time.Duration(1<<63-1), deadline)
	if err != nil {
		t.Fatal(err)
	}
	if !expiresAt.Equal(deadline) {
		t.Fatalf("lease expiry = %s, want Run deadline %s", expiresAt, deadline)
	}
	if encoded := formatResearchflowTimeValue(expiresAt); len(encoded) != 30 || encoded != "2026-11-10T01:02:04.623456789Z" {
		t.Fatalf("persisted expiry = %q, want fixed-width nanosecond timestamp", encoded)
	}
}

func TestCalculateResearchflowLeaseExpiryRejectsInvalidBoundary(t *testing.T) {
	now := time.Date(2026, 11, 10, 1, 2, 3, 123456789, time.UTC)
	for _, duration := range []time.Duration{0, -time.Nanosecond} {
		if _, err := calculateResearchflowLeaseExpiry(now, duration, now.Add(time.Minute)); err == nil {
			t.Fatalf("duration %s was accepted", duration)
		}
	}
	if _, err := calculateResearchflowLeaseExpiry(now, time.Second, now); err == nil {
		t.Fatal("Run deadline equal to database time was accepted")
	}
}

func TestResearchflowTimestampRejectsZeroValue(t *testing.T) {
	if _, err := formatResearchflowTime(time.Time{}); err == nil {
		t.Fatal("expected zero timestamp to be rejected")
	}
}

func TestSortResearchflowStagesUsesResolvedTemplateOrder(t *testing.T) {
	stages := []domain.Stage{
		{Code: "risk"},
		{Code: "context"},
		{Code: "quant"},
	}
	sortResearchflowStages(stages, map[string]int{"context": 0, "quant": 1, "risk": 2})
	want := []string{"context", "quant", "risk"}
	for index := range want {
		if stages[index].Code != want[index] {
			t.Fatalf("stage order = %#v, want %#v", stages, want)
		}
	}
}

func TestMySQLRunRepositoryValidatesResolvedSnapshotIdentity(t *testing.T) {
	repository := &MySQLRunRepository{db: &sql.DB{}}
	record := application.CreateRunRecord{
		Scope:       application.IdempotencyScope{CallerRef: "caller-1", Operation: "CreateRun", Key: "key-1"},
		Fingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Run: domain.Run{
			Ref: "run-1", SchemaVersion: "v1", Status: domain.RunCreated, Version: 1,
			TemplateRef: "template-1", TemplateVersion: "v1",
			TemplateFingerprint: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			CorrelationID:       "corr-1",
		},
		Parameters: json.RawMessage(`{}`),
		ResolvedTemplate: template.ResolvedTemplate{
			Ref: "template-1", Version: "v1",
			Fingerprint: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
		Stages: []domain.Stage{{Ref: "stage-1", RunRef: "run-1", Code: "context", Status: domain.StagePending, Version: 1}},
	}
	if err := repository.validateRecord(record); err != nil {
		t.Fatalf("valid create-run record rejected: %v", err)
	}
	forgedRun := record
	forgedStartedAt := time.Now().UTC()
	forgedRun.Run.StartedAt = &forgedStartedAt
	if err := repository.validateRecord(forgedRun); err == nil {
		t.Fatal("CreateRun accepted caller-supplied Run StartedAt")
	}
	forgedStage := record
	forgedStage.Stages = append([]domain.Stage(nil), record.Stages...)
	forgedStage.Stages[0].AttemptNo = 1
	if err := repository.validateRecord(forgedStage); err == nil {
		t.Fatal("CreateRun accepted Stage AttemptNo owned by ClaimAttempt")
	}
	record.ResolvedTemplate.Version = "v2"
	if err := repository.validateRecord(record); err == nil {
		t.Fatal("expected mismatched resolved-template version to be rejected")
	}
}

func TestMariaDBCreateRunAtomicConcurrentIdempotency(t *testing.T) {
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
	makeRecord := func(runRef, stageRef string) application.CreateRunRecord {
		return application.CreateRunRecord{
			Scope: application.IdempotencyScope{
				CallerRef: "researchflow-integration-test", Operation: "CreateRun", Key: "create-" + suffix,
			},
			Fingerprint: fingerprint,
			Run: domain.Run{
				Ref: runRef, SchemaVersion: "v1", Status: domain.RunCreated, Version: 1,
				SystemAsOf: now.Add(-time.Minute), DeadlineAt: now.Add(5 * time.Minute),
				TemplateRef: "integration-template", TemplateVersion: "v1",
				TemplateFingerprint: templateFingerprint, InputSnapshotRefs: []string{"snapshot-" + suffix},
				CorrelationID: "correlation-" + suffix,
			},
			Stages: []domain.Stage{{
				Ref: stageRef, RunRef: runRef, Code: "context", Status: domain.StagePending,
				Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
				DiagnosticRefs: []string{}, ReasonCodes: []string{},
			}},
			ResolvedTemplate: template.ResolvedTemplate{
				Ref: "integration-template", Version: "v1", Fingerprint: templateFingerprint,
				Stages: []template.ResolvedStage{{
					Definition: template.StageDefinition{Code: "context"},
				}},
			},
			Parameters: json.RawMessage(`{}`),
		}
	}

	records := []application.CreateRunRecord{
		makeRecord("run-"+suffix+"-a", "stage-"+suffix+"-a"),
		makeRecord("run-"+suffix+"-b", "stage-"+suffix+"-b"),
	}
	type outcome struct {
		result application.CreateRunResult
		err    error
	}
	outcomes := make(chan outcome, len(records))
	var wait sync.WaitGroup
	for _, record := range records {
		record := record
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := repository.CreateRunAtomic(ctx, record)
			outcomes <- outcome{result: result, err: err}
		}()
	}
	wait.Wait()
	close(outcomes)

	var winningRunRef string
	replayed := 0
	for item := range outcomes {
		if item.err != nil {
			t.Fatalf("concurrent CreateRunAtomic failed: %v", item.err)
		}
		if item.result.Replayed {
			replayed++
		}
		if winningRunRef == "" {
			winningRunRef = item.result.Run.Ref
		} else if item.result.Run.Ref != winningRunRef {
			t.Fatalf("concurrent calls returned different runs: %q and %q", winningRunRef, item.result.Run.Ref)
		}
	}
	if replayed != 1 {
		t.Fatalf("replayed calls = %d, want exactly 1", replayed)
	}

	loadedRun, err := repository.GetRun(ctx, winningRunRef)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if loadedRun.Run.Ref != winningRunRef || len(loadedRun.Stages) != 1 {
		t.Fatalf("GetRun returned unexpected run/stages: run=%q stages=%d", loadedRun.Run.Ref, len(loadedRun.Stages))
	}
	nextRun := loadedRun.Run
	nextRun.Version++
	nextRun.Status = domain.RunQueued
	updated, err := repository.CompareAndSwapRun(ctx, loadedRun.Run.Version, nextRun)
	if err != nil || !updated {
		t.Fatalf("CompareAndSwapRun updated=%v err=%v", updated, err)
	}
	updated, err = repository.CompareAndSwapRun(ctx, loadedRun.Run.Version, nextRun)
	if err != nil || updated {
		t.Fatalf("stale CompareAndSwapRun updated=%v err=%v, want false without error", updated, err)
	}
	runningRun := nextRun
	runningRun.Version++
	runningRun.Status = domain.RunRunning
	forgedStartedAt := now.Add(-24 * time.Hour)
	runningRun.StartedAt = &forgedStartedAt
	if updated, err = repository.CompareAndSwapRun(ctx, nextRun.Version, runningRun); err == nil || updated {
		t.Fatalf("Run CAS accepted caller-supplied started_at; want rejection")
	}
	runningRun.StartedAt = nil
	updated, err = repository.CompareAndSwapRun(ctx, nextRun.Version, runningRun)
	if err != nil || !updated {
		t.Fatalf("queued -> running Run CAS updated=%v err=%v", updated, err)
	}
	persistedRunningRun, err := repository.GetRun(ctx, winningRunRef)
	if err != nil || persistedRunningRun.Run.StartedAt == nil {
		t.Fatalf("Run CAS did not assign database started_at: started_at=%v err=%v", persistedRunningRun.Run.StartedAt, err)
	}
	terminalRun := runningRun
	terminalRun.Version++
	terminalRun.Status = domain.RunSucceeded
	finished := now
	terminalRun.FinishedAt = &finished
	if updated, err = repository.CompareAndSwapRun(ctx, runningRun.Version, terminalRun); err == nil || updated {
		t.Fatalf("running -> succeeded Run CAS updated=%v err=%v; want finalizer-only rejection", updated, err)
	}
	terminationMutation := runningRun
	terminationMutation.Version++
	terminationMutation.Status = domain.RunCancelling
	terminationMutation.TerminationReason = "user_cancelled"
	if updated, err = repository.CompareAndSwapRun(ctx, runningRun.Version, terminationMutation); err == nil || updated {
		t.Fatalf("Run CAS termination_reason mutation updated=%v err=%v; want rejection", updated, err)
	}

	stageRef := loadedRun.Stages[0].Ref
	loadedStage, err := repository.GetStage(ctx, stageRef)
	if err != nil {
		t.Fatalf("GetStage: %v", err)
	}
	if loadedStage.Ref != stageRef || loadedStage.RunRef != winningRunRef {
		t.Fatalf("GetStage returned unexpected stage: %#v", loadedStage)
	}
	forgedStage := loadedStage
	forgedStage.Version++
	forgedStage.Status = domain.StageReady
	forgedStage.AttemptNo = 7
	retryAt := now.Add(time.Hour)
	forgedStage.RetryAt = &retryAt
	forgedOutput := "forged-output"
	forgedStage.OutputRef = &forgedOutput
	forgedStarted := now
	forgedStage.StartedAt = &forgedStarted
	if updated, err = repository.CompareAndSwapStage(ctx, loadedStage.Version, forgedStage); err == nil || updated {
		t.Fatalf("Stage CAS accepted scheduler transition with forged attempt/result/time fields")
	}
	nextStage := loadedStage
	nextStage.Version++
	nextStage.Status = domain.StageReady
	updated, err = repository.CompareAndSwapStage(ctx, loadedStage.Version, nextStage)
	if err != nil || !updated {
		t.Fatalf("CompareAndSwapStage updated=%v err=%v", updated, err)
	}
	updated, err = repository.CompareAndSwapStage(ctx, loadedStage.Version, nextStage)
	if err != nil || updated {
		t.Fatalf("stale CompareAndSwapStage updated=%v err=%v, want false without error", updated, err)
	}

	attemptRef := "attempt-" + suffix
	startedAt := formatResearchflowTimeValue(now)
	leaseExpiresAt := formatResearchflowTimeValue(now.Add(time.Minute))
	if _, err := db.ExecContext(ctx, `INSERT INTO researchflow_attempts (
		attempt_ref, run_ref, stage_ref, attempt_no, status, lease_owner,
		lease_expires_at, fencing_token, started_at, finished_at, fault_ref, output_ref
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		attemptRef, winningRunRef, stageRef, 1, string(domain.AttemptRunning),
		"researchflow-test-worker", leaseExpiresAt, 1, startedAt, nil, nil, nil,
	); err != nil {
		t.Fatalf("insert attempt fixture: %v", err)
	}
	loadedAttempt, err := repository.GetAttempt(ctx, attemptRef)
	if err != nil {
		t.Fatalf("GetAttempt: %v", err)
	}
	if loadedAttempt.Ref != attemptRef || loadedAttempt.StageRef != stageRef ||
		loadedAttempt.FencingToken != 1 || loadedAttempt.LeaseOwner != "researchflow-test-worker" {
		t.Fatalf("GetAttempt returned unexpected attempt: %#v", loadedAttempt)
	}

	conflict := makeRecord("run-"+suffix+"-conflict", "stage-"+suffix+"-conflict")
	conflict.Fingerprint = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	if _, err := repository.CreateRunAtomic(ctx, conflict); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("different request fingerprint error = %v, want ErrIdempotencyConflict", err)
	}

	// A direct Storage CAS must not bypass the state machine by resurrecting a terminal Stage.
	terminalStage := nextStage
	terminalStage.Version++
	terminalStage.Status = domain.StageSucceeded
	finishedAt := now
	terminalStage.FinishedAt = &finishedAt
	if updated, err = repository.CompareAndSwapStage(ctx, nextStage.Version, terminalStage); err == nil || updated {
		t.Fatalf("Stage CAS accepted ready -> succeeded; Worker result transaction must own completion")
	}
	persistedStage, err := repository.GetStage(ctx, stageRef)
	if err != nil {
		t.Fatalf("reload Stage after rejected completion CAS: %v", err)
	}
	if persistedStage.Status != domain.StageReady || persistedStage.AttemptNo != 0 ||
		persistedStage.RetryAt != nil || persistedStage.OutputRef != nil ||
		persistedStage.StartedAt != nil || persistedStage.FinishedAt != nil {
		t.Fatalf("scheduler CAS altered fields owned by Claim/Worker: %#v", persistedStage)
	}
}


func TestMariaDBWorkerStoreSnapshotClaimAndCommit(t *testing.T) {
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
	runRef, stageRef := "worker-run-"+suffix, "worker-stage-"+suffix
	record := application.CreateRunRecord{
		Scope: application.IdempotencyScope{
			CallerRef: "researchflow-worker-integration-test", Operation: "CreateRun", Key: "worker-create-"+suffix,
		},
		Fingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Run: domain.Run{
			Ref: runRef, SchemaVersion: "v1", Status: domain.RunCreated, Version: 1,
			SystemAsOf: now.Add(-time.Minute), DeadlineAt: now.Add(10 * time.Minute),
			TemplateRef: "worker-template", TemplateVersion: "v1",
			TemplateFingerprint: templateFingerprint,
			InputSnapshotRefs: []string{"input-snapshot-" + suffix},
			CorrelationID: "worker-correlation-" + suffix,
		},
		Stages: []domain.Stage{{
			Ref: stageRef, RunRef: runRef, Code: "quant", Status: domain.StagePending,
			Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
			DiagnosticRefs: []string{}, ReasonCodes: []string{},
		}},
		ResolvedTemplate: template.ResolvedTemplate{
			Ref: "worker-template", Version: "v1", Fingerprint: templateFingerprint,
			Stages: []template.ResolvedStage{{
				Definition: template.StageDefinition{
					Code: "quant",
					Retry: template.RetryPolicy{
						Ref: "retry-v1", MaxAttempts: 3, AttemptTimeoutMS: 60000,
						RetryableReasonCodes: []string{"provider_unavailable"},
						Backoff: template.BackoffPolicy{
							Algorithm: "exponential_full_jitter_v1", BaseDelayMS: 100, MaxDelayMS: 1000,
						},
					},
				},
			}},
		},
		Parameters: json.RawMessage(`{"symbol":"AAA"}`),
	}
	created, err := repository.CreateRunAtomic(ctx, record)
	if err != nil {
		t.Fatalf("CreateRunAtomic: %v", err)
	}
	queued := created.Run
	queued.Version++
	queued.Status = domain.RunQueued
	if updated, err := repository.CompareAndSwapRun(ctx, created.Run.Version, queued); err != nil || !updated {
		t.Fatalf("queue Run updated=%v err=%v", updated, err)
	}
	running := queued
	running.Version++
	running.Status = domain.RunRunning
	if updated, err := repository.CompareAndSwapRun(ctx, queued.Version, running); err != nil || !updated {
		t.Fatalf("start Run updated=%v err=%v", updated, err)
	}
	stage := created.Stages[0]
	stage.Version++
	stage.Status = domain.StageReady
	if updated, err := repository.CompareAndSwapStage(ctx, created.Stages[0].Version, stage); err != nil || !updated {
		t.Fatalf("ready Stage updated=%v err=%v", updated, err)
	}

	attemptRef := "worker-attempt-" + suffix
	attempt, replayed, err := repository.ClaimAttempt(
		ctx, runRef, stageRef, attemptRef, "worker-integration",
		time.Minute, 3, 2,
	)
	if err != nil || replayed {
		t.Fatalf("ClaimAttempt replayed=%v err=%v", replayed, err)
	}
	snapshot, err := repository.LoadStageExecutionSnapshot(ctx, runRef, stageRef)
	if err != nil {
		t.Fatalf("LoadStageExecutionSnapshot: %v", err)
	}
	if snapshot.Run.Ref != runRef || snapshot.Run.Status != domain.RunRunning ||
		snapshot.Stage.Ref != stageRef || snapshot.Stage.Status != domain.StageRunning ||
		snapshot.Stage.AttemptNo != attempt.Number ||
		string(snapshot.Parameters) != `{"symbol":"AAA"}` ||
		len(snapshot.InputSnapshotRefs) != 1 || snapshot.InputSnapshotRefs[0] != "input-snapshot-"+suffix ||
		snapshot.RetryPolicy.Ref != "retry-v1" || snapshot.RetryPolicy.MaxAttempts != 3 ||
		snapshot.RetryPolicy.AttemptTimeoutMS != 60000 {
		t.Fatalf("unexpected Worker execution snapshot: %+v", snapshot)
	}

	outputRef := "worker-output-" + suffix
	resultStage := snapshot.Stage
	resultStage.Version++
	resultStage.Status = domain.StageSucceeded
	resultStage.OutputRef = &outputRef
	resultStage.FinishedAt = nil // Storage assigns the authoritative database timestamp.
	if err := repository.CommitWorkerResult(ctx, application.WorkerResultCommit{
		RunRef: runRef, StageRef: stageRef, AttemptRef: attempt.Ref,
		WorkerID: "worker-integration", FencingToken: attempt.FencingToken,
		AttemptStatus: domain.AttemptSucceeded, Stage: resultStage,
		OutputRef: &outputRef,
	}); err != nil {
		t.Fatalf("CommitWorkerResult: %v", err)
	}
	persistedAttempt, err := repository.GetAttempt(ctx, attempt.Ref)
	if err != nil || persistedAttempt.Status != domain.AttemptSucceeded ||
		persistedAttempt.LeaseOwner != "" || persistedAttempt.LeaseExpiresAt != nil {
		t.Fatalf("persisted Attempt = %+v err=%v", persistedAttempt, err)
	}
	persistedStage, err := repository.GetStage(ctx, stageRef)
	if err != nil || persistedStage.Status != domain.StageSucceeded ||
		persistedStage.OutputRef == nil || *persistedStage.OutputRef != outputRef {
		t.Fatalf("persisted Stage = %+v err=%v", persistedStage, err)
	}
}
