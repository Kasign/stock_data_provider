package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	researchclaim "example.com/stock-analysis-flow/internal/research/claim"
	researchcontext "example.com/stock-analysis-flow/internal/research/context"
	researchevent "example.com/stock-analysis-flow/internal/research/event"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
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
	return db
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
	cancelledContextID := "ctx-cancel-it-" + suffix
	orphanContextID := "ctx-orphan-it-" + suffix

	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_context_versions WHERE context_id IN (?, ?)", contextID, orphanContextID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_event_versions WHERE event_id=?", eventID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_claim_versions WHERE claim_id=?", claimID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_aggregate_heads WHERE aggregate_id IN (?, ?, ?, ?, ?)", contextID, eventID, claimID, cancelledContextID, orphanContextID)
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

	// Cancellation must not leave a partially-created aggregate.
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
}
