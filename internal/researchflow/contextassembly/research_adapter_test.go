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
			Status: "degraded", MissingRequiredCount: 0, MissingOptionalCount: 0,
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

func TestToResearchCreateContextCommandRejectsForgedMandatoryOmissionAndTokenCount(t *testing.T) {
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	candidates := []SectionCandidate{{ID: "required", SourceOwner: "data", Required: true, EstimatedTokens: 2, SourceRefs: []string{"snapshot-1"}}}
	metadata := map[string]SectionMetadata{"required": {Version: "1", QualityStatus: "valid", AssembledAt: now}}
	snapshot := ResearchContextSnapshot{
		ResearchQuestion: "分析", Scope: "stock", SubjectRefs: []string{"600000.SH"},
		SystemAsOf: now, BuilderVersion: "v1",
		QualitySummary: researchapp.ContextQualitySummary{Status: "degraded", MissingRequiredCount: 1, ReasonCodes: []string{"required_missing"}},
	}
	forged := Plan{
		IncludedSections: []string{}, UsedTokens: 0, BudgetTokens: 2,
		OmittedSections: []OmittedSection{{
			SectionID: "required", SourceOwner: "data", ReasonCode: OmissionBudgetExceeded,
			Required: true, SourceRefs: []string{"snapshot-1"}, SafeMessage: ContextBudgetMessage,
		}},
	}
	if _, err := ToResearchCreateContextCommand(snapshot, candidates, metadata, forged); !errors.Is(err, ErrInvalidResearchContextProjection) {
		t.Fatalf("mandatory omission should be rejected, got %v", err)
	}
	plan, err := BuildPlan(candidates, 2)
	if err != nil {
		t.Fatal(err)
	}
	plan.UsedTokens = 1
	if _, err := ToResearchCreateContextCommand(snapshot, candidates, metadata, plan); !errors.Is(err, ErrInvalidResearchContextProjection) {
		t.Fatalf("inconsistent token accounting should be rejected, got %v", err)
	}
}

func TestAssembleResearchContextReservesModelWindowBeforeSelectingSections(t *testing.T) {
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	candidates := []SectionCandidate{
		{ID: "required", SourceOwner: "data", Required: true, EstimatedTokens: 20, SourceRefs: []string{"snapshot-1"}},
		{ID: "optional", SourceOwner: "research", EstimatedTokens: 15},
	}
	snapshot := ResearchContextSnapshot{
		ResearchQuestion: "分析", Scope: "stock", SubjectRefs: []string{"600000.SH"},
		SystemAsOf: now, BuilderVersion: "v1",
		QualitySummary: researchapp.ContextQualitySummary{
			Status: "degraded", MissingOptionalCount: 0, ReasonCodes: []string{"budget_exceeded"},
		},
	}
	metadata := map[string]SectionMetadata{
		"required": {Version: "1", QualityStatus: "valid", AssembledAt: now},
		"optional": {Version: "1", QualityStatus: "valid", AssembledAt: now},
	}
	result, err := AssembleResearchContext(snapshot, candidates, metadata, BudgetInput{
		ModelContextTokens: 100, SystemTokens: 20, ToolSchemaTokens: 10,
		OutputReserve: 30, SafetyMargin: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.AvailableTokens != 30 || result.Plan.UsedTokens != 20 ||
		len(result.Command.IncludedSections) != 1 || result.Command.IncludedSections[0] != "required" ||
		len(result.Command.OmittedSections) != 1 || result.Command.OmittedSections[0].SectionID != "optional" {
		t.Fatalf("unexpected assembled result: %+v", result)
	}
}

func TestAssembleResearchContextFailsClosedWhenMandatorySectionsExceedReservedBudget(t *testing.T) {
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	candidates := []SectionCandidate{{ID: "required", SourceOwner: "data", Required: true, EstimatedTokens: 31, SourceRefs: []string{"snapshot-1"}}}
	snapshot := ResearchContextSnapshot{
		ResearchQuestion: "分析", Scope: "stock", SubjectRefs: []string{"600000.SH"},
		SystemAsOf: now, BuilderVersion: "v1",
		QualitySummary: researchapp.ContextQualitySummary{Status: "valid", ReasonCodes: []string{}},
	}
	metadata := map[string]SectionMetadata{"required": {Version: "1", QualityStatus: "valid", AssembledAt: now}}
	_, err := AssembleResearchContext(snapshot, candidates, metadata, BudgetInput{
		ModelContextTokens: 100, SystemTokens: 20, ToolSchemaTokens: 10,
		OutputReserve: 30, SafetyMargin: 10,
	})
	if !errors.Is(err, ErrMandatoryBudgetExceeded) {
		t.Fatalf("expected mandatory budget failure, got %v", err)
	}
}

func TestToResearchCreateContextCommandKeepsQualityCountsIndependentFromBudgetOmissions(t *testing.T) {
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
		QualitySummary: researchapp.ContextQualitySummary{
			Status: "degraded", MissingOptionalCount: 3, ConflictCount: 1,
			ReasonCodes: []string{"source_missing"},
		},
	}
	metadata := map[string]SectionMetadata{
		"required": {Version: "1", QualityStatus: "valid", AssembledAt: now},
		"optional": {Version: "1", QualityStatus: "valid", AssembledAt: now},
	}
	command, err := ToResearchCreateContextCommand(snapshot, candidates, metadata, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(command.OmittedSections) != 1 || command.OmittedSections[0].SectionID != "optional" {
		t.Fatalf("expected one budget omission, got %+v", command.OmittedSections)
	}
	if command.QualitySummary.MissingOptionalCount != 3 || command.QualitySummary.ConflictCount != 1 {
		t.Fatalf("Research quality facts were coupled to budget omissions: %+v", command.QualitySummary)
	}
}

func TestToResearchCreateContextCommandRejectsInvalidQualitySummary(t *testing.T) {
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	candidates := []SectionCandidate{{ID: "market", SourceOwner: "data", EstimatedTokens: 1, SourceRefs: []string{"snapshot-1"}}}
	plan, err := BuildPlan(candidates, 1)
	if err != nil {
		t.Fatal(err)
	}
	metadata := map[string]SectionMetadata{"market": {Version: "1", QualityStatus: "valid", AssembledAt: now}}
	for _, quality := range []researchapp.ContextQualitySummary{
		{Status: "pass", ReasonCodes: []string{}},
		{Status: "valid", MissingRequiredCount: -1, ReasonCodes: []string{}},
		{Status: "valid", ConflictCount: -1, ReasonCodes: []string{}},
	} {
		snapshot := ResearchContextSnapshot{
			ResearchQuestion: "分析", Scope: "stock", SubjectRefs: []string{"600000.SH"},
			SystemAsOf: now, BuilderVersion: "v1", QualitySummary: quality,
		}
		if _, err := ToResearchCreateContextCommand(snapshot, candidates, metadata, plan); !errors.Is(err, ErrInvalidResearchContextProjection) {
			t.Fatalf("quality %+v should be rejected, got %v", quality, err)
		}
	}
}
