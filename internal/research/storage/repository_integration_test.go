package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
	researchclaim "example.com/stock-analysis-flow/internal/research/claim"
	researchevidence "example.com/stock-analysis-flow/internal/research/evidence"
	researchcontext "example.com/stock-analysis-flow/internal/research/context"
	researchevent "example.com/stock-analysis-flow/internal/research/event"
)

func openResearchIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("RESEARCH_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("RESEARCH_TEST_MYSQL_DSN is not set; skipping MariaDB integration test")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil { t.Fatalf("open MariaDB: %v", err) }
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil { t.Fatalf("ping MariaDB: %v", err) }
	if err := platformstorage.ApplyMigrations(ctx, db, MigrationNamespace, Migrations()); err != nil {
		t.Fatalf("apply Research migrations: %v", err)
	}
	for _, column := range []struct{ table, name string }{
		{"research_event_versions", "event_version"},
		{"research_claim_versions", "claim_version"},
	} {
		var dataType string
		if err := db.QueryRowContext(ctx, "SELECT DATA_TYPE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?", column.table, column.name).Scan(&dataType); err != nil {
			t.Fatalf("read %s.%s column type: %v", column.table, column.name, err)
		}
		if dataType != "bigint" {
			t.Fatalf("%s.%s data type = %q, want bigint", column.table, column.name, dataType)
		}
	}
	return db
}

func TestResearchEventLifecycleOutboxFailureRollsBackEventVersion(t *testing.T) {
	db := openResearchIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	now := time.Date(2026, 11, 9, 2, 0, 0, 123456789, time.UTC)
	eventID := fmt.Sprintf("event-outbox-rollback-%d", time.Now().UnixNano())
	repo, err := NewEventRepository(db, func() time.Time { return now })
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_notification_outbox WHERE subject_ref=?", "research-event:"+eventID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_event_versions WHERE event_id=?", eventID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_aggregate_heads WHERE aggregate_type='event' AND aggregate_id=?", eventID)
	})
	previous := researchevent.ResearchEvent{
		EventID: eventID, EventVersion: 1, EventType: "integration_fixture",
		Title: "Outbox rollback fixture", Status: researchevent.StatusDetected,
		SubjectRefs: []string{"security:600000"}, EvidenceRefs: []string{},
		SystemObservedAt: now, SystemAsOf: now,
		ImpactHypothesisRefs: []string{}, InvalidationConditions: []string{},
		Uncertainty: []string{}, SourceTimes: []researchevent.EventSourceTime{},
	}
	if err := repo.Append(ctx, previous); err != nil { t.Fatalf("append Event v1: %v", err) }
	next := previous
	next.EventVersion = 2
	next.Status = researchevent.StatusConsolidating
	next.SystemAsOf = now.Add(time.Minute)
	idempotencyKey := "research-event-state:research-event:" + eventID + ":2"
	if _, err := db.ExecContext(ctx,
		"INSERT INTO research_notification_outbox (event_id, event_type, schema_version, source_module, occurred_at, published_at, subject_ref, idempotency_key, correlation_id, payload_fingerprint, payload, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"nte_seed_"+eventID, researchLifecycleEventType, researchLifecycleSchemaVersion, "research",
		formatPIT(now), formatPIT(now), "research-event:"+eventID, idempotencyKey, "req_seed",
		strings.Repeat("a", 64), "{}", now); err != nil {
		t.Fatalf("seed Outbox idempotency conflict: %v", err)
	}
	err = repo.AppendWithLifecycle(ctx, next, nil, "req_test")
	if err == nil { t.Fatal("duplicate Outbox idempotency key must abort the Event append transaction") }
	if _, err := repo.GetVersion(ctx, eventID, 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Event v2 after Outbox rollback = %v, want ErrNotFound", err)
	}
	var latest int64
	if err := db.QueryRowContext(ctx, "SELECT latest_version FROM research_aggregate_heads WHERE aggregate_type='event' AND aggregate_id=?", eventID).Scan(&latest); err != nil {
		t.Fatalf("read aggregate head after rollback: %v", err)
	}
	if latest != 1 { t.Fatalf("aggregate latest_version = %d after Outbox rollback, want 1", latest) }
}


func TestResearchOutboxClaimRetryAndConfirmation(t *testing.T) {
	db := openResearchIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	now := time.Date(2026, 11, 9, 3, 0, 0, 123456789, time.UTC)
	eventID := fmt.Sprintf("event-outbox-delivery-%d", time.Now().UnixNano())
	repo, err := NewEventRepository(db, func() time.Time { return now })
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_notification_outbox WHERE subject_ref=?", "research-event:"+eventID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_event_versions WHERE event_id=?", eventID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_aggregate_heads WHERE aggregate_type='event' AND aggregate_id=?", eventID)
	})
	value := researchevent.ResearchEvent{
		EventID:eventID, EventVersion:1, EventType:"integration_fixture", Title:"Outbox delivery fixture",
		Status:researchevent.StatusDetected, SubjectRefs:[]string{}, EvidenceRefs:[]string{},
		SystemObservedAt:now, SystemAsOf:now, ImpactHypothesisRefs:[]string{},
		InvalidationConditions:[]string{}, Uncertainty:[]string{}, SourceTimes:[]researchevent.EventSourceTime{},
	}
	if err := repo.AppendWithLifecycle(ctx, value, nil, "corr_integration"); err != nil { t.Fatalf("append Event and initial Outbox: %v", err) }
	claimed, err := repo.base.ClaimOutbox(ctx, "worker_test", 10, now.Add(time.Minute))
	if err != nil { t.Fatalf("claim Outbox: %v", err) }
	if len(claimed) != 1 || claimed[0].IdempotencyKey != "research-event-state:research-event:"+eventID+":1" {
		t.Fatalf("unexpected claimed Outbox messages: %+v", claimed)
	}
	if err := repo.base.ScheduleOutboxRetry(ctx, claimed[0].EventID, "worker_test", claimed[0].LeaseToken, now.Add(2*time.Minute), "notification_unavailable", now); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}
	tooEarly, err := repo.base.ClaimOutbox(ctx, "worker_test", 10, now.Add(3*time.Minute))
	if err != nil { t.Fatalf("claim before retry time: %v", err) }
	if len(tooEarly) != 0 { t.Fatalf("retry became available too early: %+v", tooEarly) }
	now = now.Add(2*time.Minute)
	reclaimed, err := repo.base.ClaimOutbox(ctx, "worker_test", 10, now.Add(time.Minute))
	if err != nil { t.Fatalf("reclaim retry: %v", err) }
	if len(reclaimed) != 1 || reclaimed[0].EventID != claimed[0].EventID || reclaimed[0].LeaseToken == claimed[0].LeaseToken {
		t.Fatalf("retry must reuse the persisted Envelope identity: %+v", reclaimed)
	}
	if err := repo.base.MarkOutboxPublished(ctx, claimed[0].EventID, "worker_test", claimed[0].LeaseToken, now); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale lease token confirmation = %v, want ErrConflict", err)
	}
	if err := repo.base.MarkOutboxPublished(ctx, reclaimed[0].EventID, "worker_test", reclaimed[0].LeaseToken, now); err != nil {
		t.Fatalf("confirm Outbox delivery: %v", err)
	}
	if err := repo.base.MarkOutboxPublished(ctx, reclaimed[0].EventID, "worker_test", reclaimed[0].LeaseToken, now); err != nil {
		t.Fatalf("repeated confirmation must be idempotent: %v", err)
	}
	after, err := repo.base.ClaimOutbox(ctx, "worker_test", 10, now.Add(time.Minute))
	if err != nil { t.Fatalf("claim after confirmation: %v", err) }
	if len(after) != 0 { t.Fatalf("delivered Outbox message was claimed again: %+v", after) }
	var attempts int
	var status string
	if err := db.QueryRowContext(ctx, "SELECT attempt_count, delivery_status FROM research_notification_outbox WHERE event_id=?", claimed[0].EventID).Scan(&attempts,&status); err != nil {
		t.Fatalf("read Outbox delivery state: %v", err)
	}
	if attempts != 2 || status != "published" { t.Fatalf("Outbox attempts/status = %d/%s, want 2/published", attempts,status) }
}

func TestResearchSQLRepositoriesAppendAndReadVersions(t *testing.T) {
	db := openResearchIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	now := time.Date(2026, 11, 6, 8, 0, 0, 123456789, time.UTC)
	clock := func() time.Time { return now }

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	contextID := "ctx-it-" + suffix
	eventID := "event-it-" + suffix
	claimID := "claim-it-" + suffix
	evidenceID := "evidence-it-" + suffix
	cancelledContextID := "ctx-cancel-it-" + suffix
	orphanContextID := "ctx-orphan-it-" + suffix
	lockedContextID := "ctx-lock-cancel-it-" + suffix

	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_context_versions WHERE context_id IN (?, ?, ?)", contextID, orphanContextID, lockedContextID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_event_versions WHERE event_id=?", eventID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_claim_versions WHERE claim_id=?", claimID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_evidence_records WHERE evidence_id=?", evidenceID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_aggregate_heads WHERE aggregate_id IN (?, ?, ?, ?, ?, ?)", contextID, eventID, claimID, cancelledContextID, orphanContextID, lockedContextID)
	})

	contextRepo, err := NewContextRepository(db, clock)
	if err != nil { t.Fatal(err) }
	eventRepo, err := NewEventRepository(db, clock)
	if err != nil { t.Fatal(err) }
	claimRepo, err := NewClaimRepository(db, clock)
	if err != nil { t.Fatal(err) }

	cutoff := now
	contextV1 := researchcontext.Context{
		SchemaVersion: "1.0.0", ContextID: contextID, ContextVersion: 1,
		ResearchQuestion: "integration test", Scope: researchcontext.ScopeStock,
		SubjectRefs: []string{"600000.SH"}, SystemAsOf: cutoff,
		SectionRefs: []researchcontext.SectionRef{{
			SectionID: "profile", SectionVersion: "v1", SourceOwner: researchcontext.SourceOwnerData,
			SourceRefs: []string{"data:fixture:1"}, EvidenceRefs: []string{},
			QualityStatus: researchcontext.QualityValid, Required: true, Included: true,
			AssembledAt: cutoff, SystemAsOf: cutoff,
		}},
		QualitySummary: researchcontext.QualitySummary{Status: researchcontext.QualityValid, ReasonCodes: []string{}},
		IncludedSections: []string{"profile"}, OmittedSections: []researchcontext.OmittedSection{},
		BuilderVersion: "integration-v1", CreatedAt: cutoff,
	}

	// Seed an orphan immutable row to force a duplicate-key failure after the
	// adapter has created/locked the aggregate head. The transaction must roll
	// back that newly-created head row.
	if _, err := db.ExecContext(ctx, "INSERT INTO research_context_versions "+
		"(context_id, context_version, system_as_of, payload_sha256, payload, created_at) VALUES (?, 1, ?, ?, ?, ?)",
		orphanContextID, formatPIT(cutoff), payloadDigest([]byte("{}")), "{}", cutoff); err != nil {
		t.Fatalf("seed orphan Context version: %v", err)
	}
	orphanContext := contextV1
	orphanContext.ContextID = orphanContextID
	if err := contextRepo.Append(ctx, orphanContext); err == nil {
		t.Fatal("append over orphan Context version must fail")
	}
	var orphanHeadCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM research_aggregate_heads "+
		"WHERE aggregate_type='context' AND aggregate_id=?", orphanContextID).Scan(&orphanHeadCount); err != nil {
		t.Fatalf("check rolled-back aggregate head: %v", err)
	}
	if orphanHeadCount != 0 {
		t.Fatalf("failed append left %d aggregate head rows, want 0", orphanHeadCount)
	}

	if err := contextRepo.Append(ctx, contextV1); err != nil { t.Fatalf("append Context v1: %v", err) }
	if err := contextRepo.Append(ctx, contextV1); err != nil { t.Fatalf("replay Context v1: %v", err) }
	conflictingContext := contextV1
	conflictingContext.ResearchQuestion = "different payload"
	if err := contextRepo.Append(ctx, conflictingContext); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting Context replay error = %v, want ErrConflict", err)
	}
	contextGap := contextV1
	contextGap.ContextVersion = 3
	if err := contextRepo.Append(ctx, contextGap); !errors.Is(err, ErrConflict) {
		t.Fatalf("Context version gap error = %v, want ErrConflict", err)
	}
	contextV2 := contextV1
	contextV2.ContextVersion = 2
	contextV2.CreatedAt = cutoff.Add(time.Minute)
	contextV2.SystemAsOf = cutoff.Add(time.Minute)
	contextV2.SectionRefs = append([]researchcontext.SectionRef(nil), contextV1.SectionRefs...)
	contextV2.SectionRefs[0].SystemAsOf = contextV2.SystemAsOf
	contextV2.SectionRefs[0].AssembledAt = contextV2.SystemAsOf
	if err := contextRepo.Append(ctx, contextV2); err != nil { t.Fatalf("append Context v2: %v", err) }
	if got, err := contextRepo.GetVersion(ctx, contextID, 1); err != nil || got.ContextVersion != 1 {
		t.Fatalf("get Context v1 = version %d, error %v", got.ContextVersion, err)
	}

	// Identical concurrent appends must converge to one immutable version.
	contextV3 := contextV2
	contextV3.ContextVersion = 3
	contextV3.CreatedAt = cutoff.Add(2 * time.Minute)
	contextV3.SystemAsOf = cutoff.Add(2 * time.Minute)
	contextV3.SectionRefs = append([]researchcontext.SectionRef(nil), contextV2.SectionRefs...)
	contextV3.SectionRefs[0].SystemAsOf = contextV3.SystemAsOf
	contextV3.SectionRefs[0].AssembledAt = contextV3.SystemAsOf
	var concurrent sync.WaitGroup
	replayErrors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		concurrent.Add(1)
		go func() {
			defer concurrent.Done()
			replayErrors <- contextRepo.Append(ctx, contextV3)
		}()
	}
	concurrent.Wait()
	close(replayErrors)
	for err := range replayErrors {
		if err != nil { t.Fatalf("concurrent identical Context append: %v", err) }
	}

	// Conflicting payloads for one version must have exactly one winner.
	contextV4A := contextV3
	contextV4A.ContextVersion = 4
	contextV4A.CreatedAt = cutoff.Add(3 * time.Minute)
	contextV4A.SystemAsOf = cutoff.Add(3 * time.Minute)
	contextV4A.SectionRefs = append([]researchcontext.SectionRef(nil), contextV3.SectionRefs...)
	contextV4A.SectionRefs[0].SystemAsOf = contextV4A.SystemAsOf
	contextV4A.SectionRefs[0].AssembledAt = contextV4A.SystemAsOf
	contextV4B := contextV4A
	contextV4B.ResearchQuestion = "concurrent competing payload"
	var outcomes sync.WaitGroup
	conflictErrors := make(chan error, 2)
	for _, candidate := range []researchcontext.Context{contextV4A, contextV4B} {
		candidate := candidate
		outcomes.Add(1)
		go func() {
			defer outcomes.Done()
			conflictErrors <- contextRepo.Append(ctx, candidate)
		}()
	}
	outcomes.Wait()
	close(conflictErrors)
	successes, conflicts := 0, 0
	for err := range conflictErrors {
		switch {
		case err == nil: successes++
		case errors.Is(err, ErrConflict): conflicts++
		default: t.Fatalf("unexpected concurrent Context append error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent conflicting append results: successes=%d conflicts=%d, want 1/1", successes, conflicts)
	}

	// A request canceled before Append starts must not create an aggregate.
	cancelledContext := contextV1
	cancelledContext.ContextID = cancelledContextID
	cancelCtx, cancelAppend := context.WithCancel(ctx)
	cancelAppend()
	if err := contextRepo.Append(cancelCtx, cancelledContext); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Context append error = %v, want context.Canceled", err)
	}
	if _, err := contextRepo.GetVersion(ctx, cancelledContextID, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancelled Context append GetVersion error = %v, want ErrNotFound", err)
	}
	var cancelledHeadCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM research_aggregate_heads WHERE aggregate_type='context' AND aggregate_id=?", cancelledContextID).Scan(&cancelledHeadCount); err != nil {
		t.Fatalf("check pre-canceled aggregate head: %v", err)
	}
	if cancelledHeadCount != 0 {
		t.Fatalf("pre-canceled append left %d aggregate head rows, want 0", cancelledHeadCount)
	}


	// Cancellation while waiting for the aggregate-head row lock must abort
	// the append transaction and leave the previous version/head unchanged.
	lockedContextV1 := contextV1
	lockedContextV1.ContextID = lockedContextID
	if err := contextRepo.Append(ctx, lockedContextV1); err != nil {
		t.Fatalf("append lock-cancel Context v1: %v", err)
	}
	blocker, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin aggregate-head lock transaction: %v", err)
	}
	if _, err := blocker.ExecContext(ctx, "UPDATE research_aggregate_heads SET updated_at=updated_at WHERE aggregate_type='context' AND aggregate_id=?", lockedContextID); err != nil {
		_ = blocker.Rollback()
		t.Fatalf("lock aggregate head: %v", err)
	}
	lockedContextV2 := lockedContextV1
	lockedContextV2.ContextVersion = 2
	lockedContextV2.CreatedAt = cutoff.Add(time.Minute)
	lockedContextV2.SystemAsOf = cutoff.Add(time.Minute)
	lockedContextV2.SectionRefs = append([]researchcontext.SectionRef(nil), lockedContextV1.SectionRefs...)
	lockedContextV2.SectionRefs[0].SystemAsOf = lockedContextV2.SystemAsOf
	lockedContextV2.SectionRefs[0].AssembledAt = lockedContextV2.SystemAsOf
	lockWaitCtx, cancelLockWait := context.WithCancel(ctx)
	appendResult := make(chan error, 1)
	appendResultReceived := false
	started := make(chan struct{})
	go func() {
		close(started)
		appendResult <- contextRepo.Append(lockWaitCtx, lockedContextV2)
	}()
	// 无论断言在哪个分支失败，都先取消请求并释放阻塞事务，再回收工作协程。
	defer func() {
		cancelLockWait()
		_ = blocker.Rollback()
		if !appendResultReceived {
			select {
			case <-appendResult:
			case <-time.After(5 * time.Second):
				t.Errorf("append goroutine did not exit after lock-wait test cleanup")
			}
		}
	}()
	<-started
	// Wait until the append SQL is visible in MariaDB's process list while the
	// blocker still owns the row lock; this avoids mistaking pre-cancellation
	// for cancellation during an actual database lock wait.
	waitDeadline := time.Now().Add(5 * time.Second)
	lockWaitObserved := false
	for time.Now().Before(waitDeadline) {
		var active int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.PROCESSLIST WHERE INFO LIKE 'INSERT IGNORE INTO research_aggregate_heads%' OR INFO LIKE 'SELECT latest_version FROM research_aggregate_heads%'").Scan(&active); err != nil {
			cancelLockWait()
			_ = blocker.Rollback()
			t.Fatalf("inspect MariaDB process list for blocked append: %v", err)
		}
		if active > 0 {
			lockWaitObserved = true
			break
		}
		select {
		case appendErr := <-appendResult:
			appendResultReceived = true
			cancelLockWait()
			_ = blocker.Rollback()
			t.Fatalf("append returned before lock wait was observed: %v", appendErr)
		case <-time.After(20 * time.Millisecond):
		}
	}
	if !lockWaitObserved {
		cancelLockWait()
		_ = blocker.Rollback()
		t.Fatal("MariaDB did not expose the append statement while the aggregate-head lock was held")
	}
	cancelLockWait()
	var appendErr error
	select {
	case appendErr = <-appendResult:
		appendResultReceived = true
	case <-time.After(5 * time.Second):
		_ = blocker.Rollback()
		t.Fatal("append did not return after lock-wait context cancellation")
	}
	if err := blocker.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("release aggregate-head lock: %v", err)
	}
	if !errors.Is(appendErr, context.Canceled) {
		t.Fatalf("lock-wait append error = %v, want context.Canceled", appendErr)
	}
	var lockedLatest int64
	if err := db.QueryRowContext(ctx, "SELECT latest_version FROM research_aggregate_heads WHERE aggregate_type='context' AND aggregate_id=?", lockedContextID).Scan(&lockedLatest); err != nil {
		t.Fatalf("read aggregate head after canceled lock wait: %v", err)
	}
	if lockedLatest != 1 {
		t.Fatalf("aggregate head after canceled lock wait = %d, want 1", lockedLatest)
	}
	var lockedV2Count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM research_context_versions WHERE context_id=? AND context_version=2", lockedContextID).Scan(&lockedV2Count); err != nil {
		t.Fatalf("read version after canceled lock wait: %v", err)
	}
	if lockedV2Count != 0 {
		t.Fatalf("canceled lock-wait append left %d version-2 rows, want 0", lockedV2Count)
	}

	eventV1 := researchevent.ResearchEvent{
		EventID: eventID, EventVersion: 1, EventType: "integration_fixture",
		Title: "Research storage integration", Status: researchevent.StatusDetected,
		SubjectRefs: []string{}, EvidenceRefs: []string{},
		SystemObservedAt: cutoff, SystemAsOf: cutoff,
		ImpactHypothesisRefs: []string{}, InvalidationConditions: []string{},
		Uncertainty: []string{}, SourceTimes: []researchevent.EventSourceTime{},
	}
	if err := eventRepo.Append(ctx, eventV1); err != nil { t.Fatalf("append Event v1: %v", err) }
	if err := eventRepo.Append(ctx, eventV1); err != nil { t.Fatalf("replay Event v1: %v", err) }
	eventGap := eventV1
	eventGap.EventVersion = 3
	if err := eventRepo.Append(ctx, eventGap); !errors.Is(err, ErrConflict) {
		t.Fatalf("Event version gap error = %v, want ErrConflict", err)
	}
	eventV2 := eventV1
	eventV2.EventVersion = 2
	eventV2.SystemAsOf = cutoff.Add(time.Minute)
	if err := eventRepo.Append(ctx, eventV2); err != nil { t.Fatalf("append Event v2: %v", err) }
	if got, err := eventRepo.GetVersion(ctx, eventID, 2); err != nil || got.EventVersion != 2 {
		t.Fatalf("get Event v2 = version %d, error %v", got.EventVersion, err)
	}

	claimV1 := researchclaim.Claim{
		ClaimID: claimID, ClaimVersion: 1, Statement: "integration fixture",
		EpistemicType: researchclaim.EpistemicReportedFact,
		ScopeRefs: []string{}, SystemAsOf: cutoff,
		SupportingEvidenceRefs: []string{}, ContradictingEvidenceRefs: []string{},
		RequiredDataRefs: []string{}, Status: researchclaim.StatusProposed,
		Limitations: []string{},
	}
	if err := claimRepo.Append(ctx, claimV1); err != nil { t.Fatalf("append Claim v1: %v", err) }
	if err := claimRepo.Append(ctx, claimV1); err != nil { t.Fatalf("replay Claim v1: %v", err) }
	claimGap := claimV1
	claimGap.ClaimVersion = 3
	if err := claimRepo.Append(ctx, claimGap); !errors.Is(err, ErrConflict) {
		t.Fatalf("Claim version gap error = %v, want ErrConflict", err)
	}
	claimV2 := claimV1
	claimV2.ClaimVersion = 2
	claimV2.SystemAsOf = cutoff.Add(time.Minute)
	if err := claimRepo.Append(ctx, claimV2); err != nil { t.Fatalf("append Claim v2: %v", err) }
	if got, err := claimRepo.GetVersion(ctx, claimID, 2); err != nil || got.ClaimVersion != 2 {
		t.Fatalf("get Claim v2 = version %d, error %v", got.ClaimVersion, err)
	}

	evidenceRepo, err := NewEvidenceRepository(db, clock)
	if err != nil { t.Fatal(err) }
	evidenceV1 := researchevidence.Evidence{
		EvidenceID: evidenceID, EvidenceType: researchevidence.TypeFact,
		SourceOwner: researchevidence.SourceData, SourceRef: "data:integration:fixture",
		DerivedFromRefs: []string{}, SubjectRefs: []string{},
		SystemObservedAt: cutoff, SystemAsOf: cutoff,
		Stance: researchevidence.StanceNeutral, QualityStatus: researchevidence.QualityValid,
		ContentLevel: researchevidence.ContentMetadata, AuthorizationRef: "auth-integration-v1",
		Limitations: []string{},
	}
	evidenceV1.ContentFingerprint, err = evidenceV1.ComputeContentFingerprint()
	if err != nil { t.Fatalf("compute Evidence fingerprint: %v", err) }
	if err := evidenceRepo.Append(ctx, evidenceV1); err != nil { t.Fatalf("append Evidence: %v", err) }
	if err := evidenceRepo.Append(ctx, evidenceV1); err != nil { t.Fatalf("replay Evidence: %v", err) }
	if got, err := evidenceRepo.Get(ctx, evidenceID); err != nil || got.AuthorizationRef != evidenceV1.AuthorizationRef {
		t.Fatalf("get Evidence authorization_ref = %q, error %v", got.AuthorizationRef, err)
	}
	evidenceRenewed := evidenceV1
	evidenceRenewed.AuthorizationRef = "auth-integration-v2"
	if err := evidenceRepo.Append(ctx, evidenceRenewed); !errors.Is(err, ErrConflict) {
		t.Fatalf("Evidence authorization snapshot replacement error = %v, want ErrConflict", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE research_evidence_records SET payload=CONCAT(payload, ' ') WHERE evidence_id=?", evidenceID); err != nil {
		t.Fatalf("tamper stored Evidence payload: %v", err)
	}
	if _, err := evidenceRepo.Get(ctx, evidenceID); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("tampered Evidence read error = %v, want ErrCorruptRecord", err)
	}

	if _, err := db.ExecContext(ctx, "UPDATE research_context_versions SET payload=CONCAT(payload, ' ') WHERE context_id=? AND context_version=2", contextID); err != nil {
		t.Fatalf("tamper stored Context payload: %v", err)
	}
	if _, err := contextRepo.GetVersion(ctx, contextID, 2); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("tampered Context read error = %v, want ErrCorruptRecord", err)
	}
	if err := contextRepo.Append(ctx, contextV2); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("tampered Context idempotent replay error = %v, want ErrCorruptRecord", err)
	}
	contextV3 := contextV2
	contextV3.ContextVersion = 3
	contextV3.SystemAsOf = contextV2.SystemAsOf.Add(time.Minute)
	contextV3.CreatedAt = contextV2.CreatedAt.Add(time.Minute)
	contextV3.SectionRefs = append([]researchcontext.SectionRef(nil), contextV2.SectionRefs...)
	contextV3.SectionRefs[0].SystemAsOf = contextV3.SystemAsOf
	contextV3.SectionRefs[0].AssembledAt = contextV3.SystemAsOf
	if err := contextRepo.Append(ctx, contextV3); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("append after tampered previous Context error = %v, want ErrCorruptRecord", err)
	}
}
