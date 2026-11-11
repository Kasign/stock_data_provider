package diagnostics

import (
	"context"
	"sync"
	"testing"
	"time"

	platformdiagnostics "example.com/stock-analysis-flow/internal/platform/diagnostics"
	researchclaim "example.com/stock-analysis-flow/internal/research/claim"
	researchcontext "example.com/stock-analysis-flow/internal/research/context"
	researchevent "example.com/stock-analysis-flow/internal/research/event"
)

func validContextSnapshot() researchcontext.Context {
	cutoff := time.Date(2026, 11, 6, 8, 0, 0, 0, time.UTC)
	return researchcontext.Context{
		SchemaVersion: "1.0.0", ContextID: "ctx-diagnostic", ContextVersion: 1,
		ResearchQuestion: "诊断测试", Scope: researchcontext.ScopeStock,
		SubjectRefs: []string{"600000.SH"}, SystemAsOf: cutoff,
		SectionRefs: []researchcontext.SectionRef{{
			SectionID: "profile", SectionVersion: "v1", SourceOwner: researchcontext.SourceOwnerData,
			SourceRefs: []string{"data:fixture:1"}, EvidenceRefs: []string{},
			QualityStatus: researchcontext.QualityValid, Required: true, Included: true,
			AssembledAt: cutoff, SystemAsOf: cutoff,
		}},
		QualitySummary: researchcontext.QualitySummary{Status: researchcontext.QualityValid, ReasonCodes: []string{}},
		IncludedSections: []string{"profile"}, OmittedSections: []researchcontext.OmittedSection{},
		BuilderVersion: "diagnostics-test-v1", CreatedAt: cutoff,
	}
}

func runCheck(t *testing.T, check platformdiagnostics.Check) platformdiagnostics.Result {
	t.Helper()
	report, err := (platformdiagnostics.Runner{}).Run(context.Background(), []platformdiagnostics.Check{check})
	if err != nil {
		t.Fatalf("Runner.Run() error = %v", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("result count = %d, want 1", len(report.Results))
	}
	return report.Results[0]
}

func TestContextCheckMapsQualityStates(t *testing.T) {
	tests := []struct {
		name string
		status researchcontext.QualityStatus
		want platformdiagnostics.Status
		code string
	}{
		{"valid", researchcontext.QualityValid, platformdiagnostics.StatusHealthy, ""},
		{"degraded", researchcontext.QualityDegraded, platformdiagnostics.StatusDegraded, "research.context.degraded"},
		{"unknown", researchcontext.QualityUnknown, platformdiagnostics.StatusUnavailable, "research.context.quality_unknown"},
		{"invalid", researchcontext.QualityInvalid, platformdiagnostics.StatusFailed, "research.context.quality_invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := validContextSnapshot()
			value.QualitySummary.Status = test.status
			if test.status != researchcontext.QualityValid {
				value.QualitySummary.ReasonCodes = []string{"fixture_reason"}
			}
			check, err := NewContextCheck("research.context.snapshot", "Context 快照", true, value)
			if err != nil {
				t.Fatalf("NewContextCheck() error = %v", err)
			}
			result := runCheck(t, check)
			if result.Observation.Status != test.want {
				t.Fatalf("status = %q, want %q", result.Observation.Status, test.want)
			}
			if result.Observation.ReasonCode != test.code {
				t.Fatalf("reason_code = %q, want %q", result.Observation.ReasonCode, test.code)
			}
		})
	}
}

func TestContextCheckDoesNotExposeValidationDetails(t *testing.T) {
	value := validContextSnapshot()
	value.ResearchQuestion = "secret user content"
	check, err := NewContextCheck("research.context.snapshot", "Context 快照", true, value)
	if err != nil {
		t.Fatalf("NewContextCheck() error = %v", err)
	}
	result := runCheck(t, check)
	if result.Observation.Status != platformdiagnostics.StatusFailed {
		t.Fatalf("status = %q, want failed", result.Observation.Status)
	}
	if result.Observation.SafeMessage == "" || result.Observation.SafeMessage == value.ResearchQuestion {
		t.Fatalf("unsafe diagnostic message: %q", result.Observation.SafeMessage)
	}
	if _, exists := result.Observation.Attributes["research_question"]; exists {
		t.Fatal("diagnostic attributes must not expose research content")
	}
}

func TestContextCheckUsesImmutableSnapshot(t *testing.T) {
	value := validContextSnapshot()
	check, err := NewContextCheck("research.context.snapshot", "Context 快照", true, value)
	if err != nil {
		t.Fatalf("NewContextCheck() error = %v", err)
	}
	value.QualitySummary.Status = researchcontext.QualityInvalid
	value.ResearchQuestion = ""
	result := runCheck(t, check)
	if result.Observation.Status != platformdiagnostics.StatusHealthy {
		t.Fatalf("snapshot status = %q, want healthy", result.Observation.Status)
	}
}

func TestEventAndClaimChecksValidateSnapshots(t *testing.T) {
	cutoff := time.Date(2026, 11, 6, 8, 0, 0, 0, time.UTC)
	event := researchevent.ResearchEvent{
		EventID: "event-diagnostic", EventVersion: 1, EventType: "fixture",
		Title: "诊断事件", Status: researchevent.StatusDetected,
		SubjectRefs: []string{}, EvidenceRefs: []string{},
		SystemObservedAt: cutoff, SystemAsOf: cutoff,
		ImpactHypothesisRefs: []string{}, InvalidationConditions: []string{},
		Uncertainty: []string{}, SourceTimes: []researchevent.EventSourceTime{},
	}
	eventCheck, err := NewEventCheck("research.event.snapshot", "Event 快照", true, event)
	if err != nil {
		t.Fatalf("NewEventCheck() error = %v", err)
	}
	if got := runCheck(t, eventCheck).Observation.Status; got != platformdiagnostics.StatusHealthy {
		t.Fatalf("valid Event status = %q, want healthy", got)
	}
	event.Title = ""
	invalidEventCheck, err := NewEventCheck("research.event.invalid", "无效 Event", true, event)
	if err != nil {
		t.Fatalf("NewEventCheck(invalid) error = %v", err)
	}
	if got := runCheck(t, invalidEventCheck).Observation.Status; got != platformdiagnostics.StatusFailed {
		t.Fatalf("invalid Event status = %q, want failed", got)
	}

	claim := researchclaim.Claim{
		ClaimID: "claim-diagnostic", ClaimVersion: 1, Statement: "fixture claim",
		EpistemicType: researchclaim.EpistemicReportedFact,
		ScopeRefs: []string{}, SystemAsOf: cutoff,
		SupportingEvidenceRefs: []string{}, ContradictingEvidenceRefs: []string{},
		RequiredDataRefs: []string{}, Status: researchclaim.StatusProposed,
		Limitations: []string{},
	}
	claimCheck, err := NewClaimCheck("research.claim.snapshot", "Claim 快照", true, claim)
	if err != nil {
		t.Fatalf("NewClaimCheck() error = %v", err)
	}
	if got := runCheck(t, claimCheck).Observation.Status; got != platformdiagnostics.StatusHealthy {
		t.Fatalf("valid Claim status = %q, want healthy", got)
	}
	claim.Statement = ""
	invalidClaimCheck, err := NewClaimCheck("research.claim.invalid", "无效 Claim", true, claim)
	if err != nil {
		t.Fatalf("NewClaimCheck(invalid) error = %v", err)
	}
	if got := runCheck(t, invalidClaimCheck).Observation.Status; got != platformdiagnostics.StatusFailed {
		t.Fatalf("invalid Claim status = %q, want failed", got)
	}
}

func TestResearchCheckHonorsCanceledRunnerContext(t *testing.T) {
	check, err := NewContextCheck("research.context.cancel", "Context 快照", true, validContextSnapshot())
	if err != nil {
		t.Fatalf("NewContextCheck() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report, err := (platformdiagnostics.Runner{}).Run(ctx, []platformdiagnostics.Check{check})
	if err != nil {
		t.Fatalf("Runner.Run() error = %v", err)
	}
	if got := report.Results[0].Observation.Status; got != platformdiagnostics.StatusUnavailable {
		t.Fatalf("canceled check status = %q, want unavailable", got)
	}
}

func TestResearchChecksAreSafeForConcurrentRunnerExecution(t *testing.T) {
	contextCheck, err := NewContextCheck("research.context.concurrent", "Context 快照", true, validContextSnapshot())
	if err != nil {
		t.Fatalf("NewContextCheck() error = %v", err)
	}
	const workers = 24
	var wait sync.WaitGroup
	errors := make(chan string, workers)
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			observation, err := contextCheck.Run(context.Background())
			if err != nil {
				errors <- err.Error()
				return
			}
			if observation.Status != platformdiagnostics.StatusHealthy {
				errors <- string(observation.Status)
			}
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		t.Errorf("concurrent Context check failed: %s", err)
	}
}
