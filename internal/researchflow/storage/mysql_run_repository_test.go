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
		Stages: []domain.Stage{{Ref: "stage-1", RunRef: "run-1", Code: "context"}},
	}
	if err := repository.validateRecord(record); err != nil {
		t.Fatalf("valid create-run record rejected: %v", err)
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

	stageRef := loadedRun.Stages[0].Ref
	loadedStage, err := repository.GetStage(ctx, stageRef)
	if err != nil {
		t.Fatalf("GetStage: %v", err)
	}
	if loadedStage.Ref != stageRef || loadedStage.RunRef != winningRunRef {
		t.Fatalf("GetStage returned unexpected stage: %#v", loadedStage)
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
	updated, err = repository.CompareAndSwapStage(ctx, nextStage.Version, terminalStage)
	if err != nil || !updated {
		t.Fatalf("ready -> succeeded Stage CAS updated=%v err=%v", updated, err)
	}
	resurrectedStage := terminalStage
	resurrectedStage.Version++
	resurrectedStage.Status = domain.StageRunning
	resurrectedStage.FinishedAt = nil
	if updated, err = repository.CompareAndSwapStage(ctx, terminalStage.Version, resurrectedStage); err == nil || updated {
		t.Fatalf("terminal -> running Stage CAS updated=%v err=%v; want rejected transition", updated, err)
	}
}
