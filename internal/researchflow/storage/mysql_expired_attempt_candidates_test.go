package storage

import (
	"context"
	"encoding/json"
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

func TestMariaDBExpiredAttemptCandidatesAndReclaim(t *testing.T) {
	if os.Getenv("PLATFORM_STORAGE_INTEGRATION") != "1" {
		t.Skip("set PLATFORM_STORAGE_INTEGRATION=1 to run Researchflow MariaDB integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true",
		os.Getenv("PLATFORM_MARIADB_USER"), os.Getenv("PLATFORM_MARIADB_PASSWORD"),
		os.Getenv("PLATFORM_MARIADB_ADDR"), os.Getenv("PLATFORM_MARIADB_DATABASE"))
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
	templateFingerprint := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	runRefs := []string{"zzzz-expired-attempt-" + suffix + "-a", "zzzz-expired-attempt-" + suffix + "-b"}
	stageRefs := []string{"zzzz-expired-stage-" + suffix + "-a", "zzzz-expired-stage-" + suffix + "-b"}
	attemptRefs := make([]string, 2)
	for index := range runRefs {
		record := application.CreateRunRecord{
			Scope: application.IdempotencyScope{
				CallerRef: "researchflow-expired-attempt-" + suffix,
				Operation: "CreateRun",
				Key: fmt.Sprintf("expired-attempt-%d-%s", index, suffix),
			},
			Fingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Run: domain.Run{
				Ref: runRefs[index], SchemaVersion: "v1", Status: domain.RunCreated, Version: 1,
				SystemAsOf: now.Add(-time.Minute), DeadlineAt: now.Add(time.Hour),
				TemplateRef: "expired-attempt-template", TemplateVersion: "v1",
				TemplateFingerprint: templateFingerprint, InputSnapshotRefs: []string{},
				CorrelationID: "expired-attempt-correlation-" + suffix,
			},
			Stages: []domain.Stage{{
				Ref: stageRefs[index], RunRef: runRefs[index], Code: "context", Status: domain.StagePending,
				Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry-v1",
				DiagnosticRefs: []string{}, ReasonCodes: []string{},
			}},
			ResolvedTemplate: template.ResolvedTemplate{
				Ref: "expired-attempt-template", Version: "v1", Fingerprint: templateFingerprint,
				Stages: []template.ResolvedStage{{Definition: template.StageDefinition{Code: "context"}}},
			},
			Parameters: json.RawMessage("{}"),
		}
		if _, err := repository.CreateRunAtomic(ctx, record); err != nil {
			t.Fatalf("create expired Attempt fixture: %v", err)
		}
		ready := record.Stages[0]
		ready.Status = domain.StageReady
		ready.Version++
		if updated, err := repository.CompareAndSwapStage(ctx, record.Stages[0].Version, ready); err != nil || !updated {
			t.Fatalf("prepare Ready Stage updated=%v err=%v", updated, err)
		}
		attempt, replayed, err := repository.ClaimAttempt(ctx, runRefs[index], stageRefs[index],
			"zzzz-expired-attempt-id-"+fmt.Sprintf("%d", index)+"-"+suffix,
			"expired-attempt-worker-"+fmt.Sprintf("%d", index), time.Minute, 3, 4)
		if err != nil || replayed {
			t.Fatalf("claim initial Attempt replayed=%v err=%v", replayed, err)
		}
		attemptRefs[index] = attempt.Ref
		if _, err := db.ExecContext(ctx, "UPDATE researchflow_attempts SET lease_expires_at = ? WHERE attempt_ref = ?",
			formatResearchflowTimeValue(now.Add(-time.Minute)), attempt.Ref); err != nil {
			t.Fatalf("expire Attempt Lease: %v", err)
		}
	}

	first, err := repository.ListExpiredAttemptCandidates(ctx, "", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].RunRef != runRefs[0] || first[0].StageRef != stageRefs[0] ||
		first[0].AttemptRef != attemptRefs[0] {
		t.Fatalf("first expired Attempt page=%#v", first)
	}
	second, err := repository.ListExpiredAttemptCandidates(ctx, first[0].RunRef, first[0].StageRef, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || second[0].RunRef != runRefs[1] || second[0].StageRef != stageRefs[1] {
		t.Fatalf("second expired Attempt page=%#v", second)
	}

	reclaimed, replayed, err := repository.ClaimAttempt(ctx, first[0].RunRef, first[0].StageRef,
		"zzzz-expired-attempt-reclaimed-"+suffix, "expired-attempt-recovery-worker", time.Minute, 3, 4)
	if err != nil || replayed || reclaimed.Number != 2 || reclaimed.FencingToken <= 1 {
		t.Fatalf("reclaim expired Attempt=%#v replayed=%v err=%v", reclaimed, replayed, err)
	}
	previous, err := repository.GetAttempt(ctx, attemptRefs[0])
	if err != nil || previous.Status != domain.AttemptTimedOut || previous.LeaseOwner != "" ||
		previous.LeaseExpiresAt != nil || previous.FencingToken >= reclaimed.FencingToken {
		t.Fatalf("previous Attempt was not fenced and closed: attempt=%#v err=%v", previous, err)
	}
}
