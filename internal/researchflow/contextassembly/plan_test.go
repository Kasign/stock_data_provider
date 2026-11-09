package contextassembly

import (
	"errors"
	"reflect"
	"testing"
)

func TestBuildPlanUsesStablePriorityAndSectionIDOrder(t *testing.T) {
	plan, err := BuildPlan([]SectionCandidate{
		{ID: "z", SourceOwner: "research", Priority: 2, EstimatedTokens: 4},
		{ID: "b", SourceOwner: "quant", Priority: 1, EstimatedTokens: 3},
		{ID: "a", SourceOwner: "news", Priority: 1, EstimatedTokens: 2},
	}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.IncludedSections, []string{"a", "b"}) {
		t.Fatalf("included sections = %v", plan.IncludedSections)
	}
	if len(plan.OmittedSections) != 1 || plan.OmittedSections[0].SectionID != "z" ||
		plan.OmittedSections[0].ReasonCode != OmissionBudgetExceeded {
		t.Fatalf("unexpected omitted sections: %+v", plan.OmittedSections)
	}
	if plan.UsedTokens != 5 || plan.BudgetTokens != 5 {
		t.Fatalf("unexpected token accounting: %+v", plan)
	}
}

func TestBuildPlanNeverDropsProtectedEvidence(t *testing.T) {
	plan, err := BuildPlan([]SectionCandidate{
		{ID: "optional", SourceOwner: "research", Priority: 1, EstimatedTokens: 3},
		{ID: "counterevidence", SourceOwner: "research", Priority: 9, EstimatedTokens: 4,
			EvidenceRefs: []string{"ev-support", "ev-counter"}, ProtectedEvidenceRefs: []string{"ev-counter"}},
	}, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.IncludedSections, []string{"counterevidence"}) {
		t.Fatalf("protected section was not retained: %v", plan.IncludedSections)
	}
	if len(plan.OmittedSections) != 1 || plan.OmittedSections[0].SectionID != "optional" {
		t.Fatalf("unexpected omissions: %+v", plan.OmittedSections)
	}
}

func TestBuildPlanFailsWhenMandatorySectionsExceedBudget(t *testing.T) {
	_, err := BuildPlan([]SectionCandidate{
		{ID: "required", SourceOwner: "quant", Required: true, EstimatedTokens: 4},
		{ID: "risk", SourceOwner: "research", EstimatedTokens: 3, EvidenceRefs: []string{"risk-evidence"}, ProtectedEvidenceRefs: []string{"risk-evidence"}},
	}, 6)
	if !errors.Is(err, ErrMandatoryBudgetExceeded) {
		t.Fatalf("expected mandatory budget error, got %v", err)
	}
}

func TestBuildPlanValidatesProtectedEvidenceMembership(t *testing.T) {
	_, err := BuildPlan([]SectionCandidate{{
		ID: "risks", SourceOwner: "research", EstimatedTokens: 1,
		EvidenceRefs: []string{"ev-1"}, ProtectedEvidenceRefs: []string{"ev-missing"},
	}}, 10)
	if !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("expected invalid plan error, got %v", err)
	}
}

func TestBuildPlanDoesNotMutateCandidateSlices(t *testing.T) {
	sourceRefs := []string{"source-2", "source-1"}
	evidenceRefs := []string{"ev-1"}
	protectedRefs := []string{"ev-1"}
	input := []SectionCandidate{{
		ID: "section", SourceOwner: "research", EstimatedTokens: 1,
		SourceRefs: sourceRefs, EvidenceRefs: evidenceRefs, ProtectedEvidenceRefs: protectedRefs,
	}}
	if _, err := BuildPlan(input, 10); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(input[0].SourceRefs, []string{"source-2", "source-1"}) ||
		!reflect.DeepEqual(input[0].EvidenceRefs, []string{"ev-1"}) ||
		!reflect.DeepEqual(input[0].ProtectedEvidenceRefs, []string{"ev-1"}) {
		t.Fatalf("input slices were mutated: %+v", input[0])
	}
}

func TestBuildPlanRejectsNonCanonicalIdentityAndReferences(t *testing.T) {
	tests := []SectionCandidate{
		{ID: " section ", SourceOwner: "research", EstimatedTokens: 1},
		{ID: "section", SourceOwner: " research", EstimatedTokens: 1},
		{ID: "section", SourceOwner: "research", EstimatedTokens: 1, SourceRefs: []string{" source-1"}},
		{ID: "section", SourceOwner: "research", EstimatedTokens: 1, EvidenceRefs: []string{"ev-1 "}},
	}
	for _, candidate := range tests {
		if _, err := BuildPlan([]SectionCandidate{candidate}, 10); !errors.Is(err, ErrInvalidPlan) {
			t.Fatalf("candidate %+v error = %v", candidate, err)
		}
	}
}
