package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
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

	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_context_versions WHERE context_id=?", contextID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_event_versions WHERE event_id=?", eventID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_claim_versions WHERE claim_id=?", claimID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_aggregate_heads WHERE aggregate_id IN (?, ?, ?)", contextID, eventID, claimID)
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
