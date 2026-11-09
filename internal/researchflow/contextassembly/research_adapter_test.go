package contextassembly

import (
	"errors"
	"testing"
	"time"

	researchapp "example.com/stock-analysis-flow/internal/research/application"
)

func TestToResearchCreateContextCommandMapsPlanWithoutInventingQuality(t *testing.T) {
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	candidates := []SectionCandidate{
		{ID: "market", SourceOwner: "data", Required: true, EstimatedTokens: 4, SourceRefs: []string{"snapshot-1"}, EvidenceRefs: []string{"ev-1"}},
		{ID: "news", SourceOwner: "news", EstimatedTokens: 8, SourceRefs: []string{"news-1"}, EvidenceRefs: []string{"ev-2"}},
	}
	plan, err := BuildPlan(candidates, 5)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := ResearchContextSnapshot{
		ResearchQuestion: "分析目标证券", Scope: "stock", SubjectRefs: []string{"600000.SH"},
		SystemAsOf: now, BuilderVersion: "context-builder-v1",
		QualitySummary: researchapp.ContextQualitySummary{
			Status: "degraded", MissingRequiredCount: 0, MissingOptionalCount: 1,
			ConflictCount: 2, StaleCount: 1, ReasonCodes: []string{"budget_exceeded", "stale_source"},
		},
	}
	metadata := map[string]SectionMetadata{
		"market": {Version: "1.0.0", QualityStatus: "valid", AssembledAt: now},
		"news":   {Version: "2.0.0", QualityStatus: "degraded", AssembledAt: now},
	}
	command, err := ToResearchCreateContextCommand(snapshot, candidates, metadata, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(command.SectionRefs) != 2 || !command.SectionRefs[0].Included || command.SectionRefs[1].Included {
		t.Fatalf("unexpected section refs: %+v", command.SectionRefs)
	}
	if command.SectionRefs[0].SystemAsOf != now || command.SectionRefs[1].SectionVersion != "2.0.0" {
		t.Fatalf("section metadata was not preserved: %+v", command.SectionRefs)
	}
	if len(command.OmittedSections) != 1 || command.OmittedSections[0].ReasonCode != OmissionBudgetExceeded {
		t.Fatalf("unexpected omissions: %+v", command.OmittedSections)
	}
	if command.QualitySummary.ConflictCount != 2 || command.QualitySummary.StaleCount != 1 {
		t.Fatalf("Research quality facts were changed: %+v", command.QualitySummary)
	}
}

func TestToResearchCreateContextCommandRejectsInconsistentPlan(t *testing.T) {
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	candidates := []SectionCandidate{{ID: "market", SourceOwner: "data", Required: true, EstimatedTokens: 2, SourceRefs: []string{"snapshot-1"}}}
	plan, err := BuildPlan(candidates, 2)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := ResearchContextSnapshot{
		ResearchQuestion: "分析", Scope: "stock", SubjectRefs: []string{"600000.SH"},
		SystemAsOf: now, BuilderVersion: "v1",
		QualitySummary: researchapp.ContextQualitySummary{Status: "valid", ReasonCodes: []string{}},
	}
	metadata := map[string]SectionMetadata{"market": {Version: "1", QualityStatus: "valid", AssembledAt: now}}
	plan.IncludedSections = append(plan.IncludedSections, "unknown")
	if _, err := ToResearchCreateContextCommand(snapshot, candidates, metadata, plan); !errors.Is(err, ErrInvalidResearchContextProjection) {
		t.Fatalf("expected invalid projection, got %v", err)
	}
}

func TestToResearchCreateContextCommandRequiresQualityCountsToMatchPlan(t *testing.T) {
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	candidates := []SectionCandidate{
		{ID: "required", SourceOwner: "data", Required: true, EstimatedTokens: 2, SourceRefs: []string{"snapshot-1"}},
		{ID: "optional", SourceOwner: "research", EstimatedTokens: 2},
	}
	plan, err := BuildPlan(candidates, 2)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := ResearchContextSnapshot{
		ResearchQuestion: "分析", Scope: "stock", SubjectRefs: []string{"600000.SH"},
		SystemAsOf: now, BuilderVersion: "v1",
		QualitySummary: researchapp.ContextQualitySummary{Status: "valid", ReasonCodes: []string{}},
	}
	metadata := map[string]SectionMetadata{
		"required": {Version: "1", QualityStatus: "valid", AssembledAt: now},
		"optional": {Version: "1", QualityStatus: "valid", AssembledAt: now},
	}
	if _, err := ToResearchCreateContextCommand(snapshot, candidates, metadata, plan); !errors.Is(err, ErrInvalidResearchContextProjection) {
		t.Fatalf("expected quality count mismatch, got %v", err)
	}
}

func TestToResearchCreateContextCommandRejectsNonUTCMetadata(t *testing.T) {
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	local := time.FixedZone("UTC+0", 0)
	candidates := []SectionCandidate{{ID: "market", SourceOwner: "data", EstimatedTokens: 1, SourceRefs: []string{"snapshot-1"}}}
	plan, err := BuildPlan(candidates, 1)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := ResearchContextSnapshot{
		ResearchQuestion: "分析", Scope: "stock", SubjectRefs: []string{"600000.SH"},
		SystemAsOf: now, BuilderVersion: "v1",
		QualitySummary: researchapp.ContextQualitySummary{Status: "valid", ReasonCodes: []string{}},
	}
	metadata := map[string]SectionMetadata{"market": {Version: "1", QualityStatus: "valid", AssembledAt: now.In(local)}}
	if _, err := ToResearchCreateContextCommand(snapshot, candidates, metadata, plan); !errors.Is(err, ErrInvalidResearchContextProjection) {
		t.Fatalf("expected non-UTC metadata rejection, got %v", err)
	}
}
