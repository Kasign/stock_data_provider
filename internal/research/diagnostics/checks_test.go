package diagnostics

import (
	"context"
	"testing"
	"time"

	platformdiagnostics "example.com/stock-analysis-flow/internal/platform/diagnostics"
	researchcontext "example.com/stock-analysis-flow/internal/research/context"
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
