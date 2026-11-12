package storage

import (
	"context"
	"encoding/json"
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

// TestMariaDBClaimOutboxConcurrentWorkersNeverShareOneMessage verifies that
// concurrent workers cannot acquire the same unexpired Outbox lease.
func TestMariaDBClaimOutboxConcurrentWorkersNeverShareOneMessage(t *testing.T) {
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
	repository, err := NewMySQLRunRepository(db, CapacityLimits{MaxQueuedRuns: 1000, MaxWorkers: 64})
	if err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	now := time.Now().UTC()
	runRef := "outbox-claim-race-run-" + suffix
	stageRef := "outbox-claim-race-stage-" + suffix
	templateFingerprint := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	record := application.CreateRunRecord{
		Scope: application.IdempotencyScope{
			CallerRef: "researchflow-outbox-claim-race", Operation: "CreateRun", Key: "claim-race-" + suffix,
		},
		Fingerprint: "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		Run: domain.Run{
			Ref: runRef, SchemaVersion: "run-v1", Status: domain.RunCreated, Version: 1,
			SystemAsOf: now.Add(-time.Minute), DeadlineAt: now.Add(5 * time.Minute),
			TemplateRef: "outbox-claim-template", TemplateVersion: "v1",
			TemplateFingerprint: templateFingerprint, InputSnapshotRefs: []string{},
			CorrelationID: "outbox-claim-correlation-" + suffix,
		},
		Stages: []domain.Stage{{
			Ref: stageRef, RunRef: runRef, Code: "context", Status: domain.StagePending,
			Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
			DiagnosticRefs: []string{}, ReasonCodes: []string{},
		}},
		ResolvedTemplate: template.ResolvedTemplate{
			Ref: "outbox-claim-template", Version: "v1", Fingerprint: templateFingerprint,
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
	finalization := RunFinalization{
		RunRef: runRef, ExpectedVersion: 2,
		ResultSummaryRef: "outbox-claim-summary-" + suffix, SummarySchemaVersion: "summary-v1",
		OutboxRef: "outbox-claim-event-" + suffix, TerminalStatus: domain.RunSucceeded,
		Summary: domain.ResultSummary{
			StageOutcomes:  map[string]domain.StageStatus{"context": domain.StageSucceeded},
			ExcludedStages: []domain.ExcludedStage{}, MissingStageCodes: []string{},
			DegradedStageCodes: []string{}, BlockedStageCodes: []string{},
			ReasonCodes: []string{}, Coverage: 1, QualityStatus: "valid",
			RiskGateDecisions: map[string]string{}, ActionabilityStatus: "not_evaluated",
		},
	}
	if updated, err := repository.FinalizeRun(ctx, finalization); err != nil || !updated {
		t.Fatalf("FinalizeRun updated=%v err=%v", updated, err)
	}

	start := make(chan struct{})
	type claimResult struct {
		worker   string
		messages []application.OutboxMessage
		err      error
	}
	results := make(chan claimResult, 2)
	var wait sync.WaitGroup
	for _, worker := range []string{"outbox-worker-a", "outbox-worker-b"} {
		worker := worker
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			messages, err := repository.ClaimOutbox(ctx, worker, runRef, 1, time.Minute)
			results <- claimResult{worker: worker, messages: messages, err: err}
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	var winner claimResult
	emptyClaims := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("%s ClaimOutbox: %v", result.worker, result.err)
		}
		switch len(result.messages) {
		case 0:
			emptyClaims++
		case 1:
			if winner.worker != "" {
				t.Fatalf("same Outbox event was claimed by both %s and %s", winner.worker, result.worker)
			}
			winner = result
		default:
			t.Fatalf("%s claimed %d messages with limit=1", result.worker, len(result.messages))
		}
	}
	if winner.worker == "" || emptyClaims != 1 {
		t.Fatalf("concurrent claims: winner=%q empty=%d, want one winner and one empty claim", winner.worker, emptyClaims)
	}
	message := winner.messages[0]
	if message.Ref != finalization.OutboxRef || message.AttemptCount != 1 || message.LeaseGeneration != 1 {
		t.Fatalf("claimed message metadata = ref:%q attempts:%d generation:%d",
			message.Ref, message.AttemptCount, message.LeaseGeneration)
	}
	loser := "outbox-worker-a"
	if winner.worker == loser {
		loser = "outbox-worker-b"
	}
	if acknowledged, err := repository.AcknowledgeOutbox(ctx, message.Ref, loser, message.LeaseGeneration); err != nil || acknowledged {
		t.Fatalf("non-owner acknowledgement = (%v, %v), want (false, nil)", acknowledged, err)
	}
	if acknowledged, err := repository.AcknowledgeOutbox(ctx, message.Ref, winner.worker, message.LeaseGeneration); err != nil || !acknowledged {
		t.Fatalf("owner acknowledgement = (%v, %v), want (true, nil)", acknowledged, err)
	}
}
