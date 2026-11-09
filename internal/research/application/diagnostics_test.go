package application

import (
	"context"
	"testing"
	"time"

	platformdiagnostics "example.com/stock-analysis-flow/internal/platform/diagnostics"
	researchcontext "example.com/stock-analysis-flow/internal/research/context"
	researchevent "example.com/stock-analysis-flow/internal/research/event"
	researchclaim "example.com/stock-analysis-flow/internal/research/claim"
)

func TestDiagnosticsApplicationBuildsChecksFromExactVersions(t *testing.T) {
	now := time.Date(2026, 11, 9, 5, 0, 0, 0, time.UTC)
	contextRepo := &contextRepoFake{value: researchcontext.Context{
		SchemaVersion: researchcontext.SchemaVersion, ContextID: "ctx_diag", ContextVersion: 1,
		ResearchQuestion: "诊断快照", Scope: researchcontext.ScopeStock,
		SubjectRefs: []string{"security:600000"}, SystemAsOf: now,
		SectionRefs: []researchcontext.SectionRef{{
			SectionID: "profile", SectionVersion: "v1", SourceOwner: researchcontext.SourceOwnerData,
			SourceRefs: []string{"data:profile:1"}, EvidenceRefs: []string{},
			QualityStatus: researchcontext.QualityValid, Required: true, Included: true,
			AssembledAt: now, SystemAsOf: now,
		}},
		QualitySummary: researchcontext.QualitySummary{Status: researchcontext.QualityValid, ReasonCodes: []string{}},
		IncludedSections: []string{"profile"}, OmittedSections: []researchcontext.OmittedSection{},
		BuilderVersion: "test", CreatedAt: now,
	}}
	eventRepo := &eventRepoFake{value: researchevent.ResearchEvent{
		EventID: "evt_diag", EventVersion: 1, EventType: "earnings_guidance",
		Title: "业绩指引变化", Status: researchevent.StatusDetected,
		SubjectRefs: []string{"security:600000"}, EvidenceRefs: []string{"ev_source_1"},
		SystemObservedAt: now, SystemAsOf: now, ImpactHypothesisRefs: []string{},
		InvalidationConditions: []string{"后续公告撤回"}, Uncertainty: []string{},
		SourceTimes: []researchevent.EventSourceTime{{
			EvidenceRef: "ev_source_1", TimeType: researchevent.TimeSystemObservedAt,
			Value: now, AssertedBy: "news:observation",
		}},
	}}
	claimRepo := &claimRepoFake{value: researchclaim.Claim{
		ClaimID: "clm_diag", ClaimVersion: 1, Statement: "盈利改善判断",
		EpistemicType: researchclaim.EpistemicReportedFact, ScopeRefs: []string{"security:600000"},
		SystemAsOf: now, SupportingEvidenceRefs: []string{"ev_source_1"},
		ContradictingEvidenceRefs: []string{}, RequiredDataRefs: []string{},
		Status: researchclaim.StatusProposed, Limitations: []string{},
	}}
	service, err := NewServiceWithClaims(contextRepo, &evidenceRepoFake{}, claimRepo, &authorizerFake{}, nil, nil)
	if err != nil { t.Fatal(err) }
	service.events = eventRepo

	checks := []struct {
		name string
		build func() (platformdiagnostics.Check, error)
	}{
		{"context", func() (platformdiagnostics.Check, error) {
			return service.BuildContextCheck(context.Background(), ContextRef{ContextID: "ctx_diag", ContextVersion: 1}, true)
		}},
		{"event", func() (platformdiagnostics.Check, error) {
			return service.BuildEventCheck(context.Background(), EventRef{EventID: "evt_diag", EventVersion: 1}, true)
		}},
		{"claim", func() (platformdiagnostics.Check, error) {
			return service.BuildClaimCheck(context.Background(), ClaimRef{ClaimID: "clm_diag", ClaimVersion: 1}, true)
		}},
	}
	for _, item := range checks {
		t.Run(item.name, func(t *testing.T) {
			check, err := item.build()
			if err != nil { t.Fatalf("build diagnostic check: %v", err) }
			if !check.Required { t.Fatal("required flag was not preserved") }
			observation, err := check.Run(context.Background())
			if err != nil { t.Fatalf("run diagnostic check: %v", err) }
			if observation.Status != platformdiagnostics.StatusHealthy {
				t.Fatalf("status = %q, want healthy", observation.Status)
			}
		})
	}
}

func TestDiagnosticsApplicationRejectsInvalidReferences(t *testing.T) {
	service, err := NewServiceWithClaims(&contextRepoFake{}, &evidenceRepoFake{}, &claimRepoFake{}, &authorizerFake{}, nil, nil)
	if err != nil { t.Fatal(err) }
	if _, err := service.BuildContextCheck(context.Background(), ContextRef{ContextID: " ", ContextVersion: 1}, true); err == nil {
		t.Fatal("blank Context ID must be rejected")
	}
	if _, err := service.BuildClaimCheck(context.Background(), ClaimRef{ClaimID: "clm_1", ClaimVersion: 0}, true); err == nil {
		t.Fatal("non-positive Claim version must be rejected")
	}
}
