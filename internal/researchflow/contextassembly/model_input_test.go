package contextassembly

import (
	"errors"
	"strings"
	"testing"
	"time"

	researchapp "example.com/stock-analysis-flow/internal/research/application"
)

func TestComposeModelInputSerializesOnlySelectedSectionsAndCountsExactPayload(t *testing.T) {
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
		QualitySummary: researchapp.ContextQualitySummary{Status: "valid", ReasonCodes: []string{}},
	}
	metadata := map[string]SectionMetadata{
		"market": {Version: "1.0.0", QualityStatus: "valid", AssembledAt: now},
		"news":   {Version: "2.0.0", QualityStatus: "degraded", AssembledAt: now},
	}
	tokenizer := testTokenizer{
		modelID: "model-a", version: "tokenizer-v1",
		count: func(value string) (int, error) {
			if !strings.Contains(value, "market authorized content") || strings.Contains(value, "news secret content") {
				t.Fatal("tokenizer did not receive the expected final serialized input")
			}
			return 37, nil
		},
	}
	input, err := ComposeModelInput(snapshot, candidates, metadata, plan,
		[]SectionContent{{SectionID: "market", Text: "market authorized content"}},
		tokenizer, "model-a", 40)
	if err != nil {
		t.Fatal(err)
	}
	if input.EstimatedTokens != 37 || input.Tokenizer.Version != "tokenizer-v1" {
		t.Fatalf("unexpected composed input metadata: %+v", input)
	}
	if !strings.Contains(input.Text, "ev-1") || strings.Contains(input.Text, "news-1") ||
		strings.Contains(input.Text, "news secret content") {
		t.Fatalf("model input leaked omitted section or lost provenance: %s", input.Text)
	}
}

func TestComposeModelInputFailsClosedWhenExactSerializationExceedsBudget(t *testing.T) {
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	candidates := []SectionCandidate{{ID: "market", SourceOwner: "data", Required: true, EstimatedTokens: 1, SourceRefs: []string{"snapshot-1"}}}
	plan, err := BuildPlan(candidates, 1)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := ResearchContextSnapshot{
		ResearchQuestion: "分析", Scope: "stock", SubjectRefs: []string{"600000.SH"},
		SystemAsOf: now, BuilderVersion: "v1",
		QualitySummary: researchapp.ContextQualitySummary{Status: "valid", ReasonCodes: []string{}},
	}
	metadata := map[string]SectionMetadata{"market": {Version: "1", QualityStatus: "valid", AssembledAt: now}}
	tokenizer := testTokenizer{modelID: "model-a", version: "v1", count: func(string) (int, error) { return 2, nil }}
	_, err = ComposeModelInput(snapshot, candidates, metadata, plan,
		[]SectionContent{{SectionID: "market", Text: "authorized"}},
		tokenizer, "model-a", 1)
	if !errors.Is(err, ErrModelInputOverBudget) {
		t.Fatalf("expected exact input budget rejection, got %v", err)
	}
}

func TestComposeModelInputRejectsMissingDuplicateAndExtraContent(t *testing.T) {
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	candidates := []SectionCandidate{{ID: "market", SourceOwner: "data", Required: true, EstimatedTokens: 1, SourceRefs: []string{"snapshot-1"}}}
	plan, err := BuildPlan(candidates, 1)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := ResearchContextSnapshot{
		ResearchQuestion: "分析", Scope: "stock", SubjectRefs: []string{"600000.SH"},
		SystemAsOf: now, BuilderVersion: "v1",
		QualitySummary: researchapp.ContextQualitySummary{Status: "valid", ReasonCodes: []string{}},
	}
	metadata := map[string]SectionMetadata{"market": {Version: "1", QualityStatus: "valid", AssembledAt: now}}
	tokenizer := testTokenizer{modelID: "model-a", version: "v1", count: func(string) (int, error) { return 1, nil }}
	for _, contents := range [][]SectionContent{
		nil,
		{{SectionID: "market", Text: "text"}, {SectionID: "market", Text: "duplicate"}},
		{{SectionID: "market", Text: "text"}, {SectionID: "extra", Text: "extra"}},
	} {
		if _, err := ComposeModelInput(snapshot, candidates, metadata, plan, contents, tokenizer, "model-a", 10); !errors.Is(err, ErrInvalidModelInput) {
			t.Fatalf("invalid contents should be rejected, got %v", err)
		}
	}
}

func TestComposeModelInputRejectsTokenizerMismatch(t *testing.T) {
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	candidates := []SectionCandidate{{ID: "market", SourceOwner: "data", Required: true, EstimatedTokens: 1, SourceRefs: []string{"snapshot-1"}}}
	plan, err := BuildPlan(candidates, 1)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := ResearchContextSnapshot{
		ResearchQuestion: "分析", Scope: "stock", SubjectRefs: []string{"600000.SH"},
		SystemAsOf: now, BuilderVersion: "v1",
		QualitySummary: researchapp.ContextQualitySummary{Status: "valid", ReasonCodes: []string{}},
	}
	metadata := map[string]SectionMetadata{"market": {Version: "1", QualityStatus: "valid", AssembledAt: now}}
	tokenizer := testTokenizer{modelID: "model-a", version: "v1", count: func(string) (int, error) { return 1, nil }}
	_, err = ComposeModelInput(snapshot, candidates, metadata, plan,
		[]SectionContent{{SectionID: "market", Text: "text"}}, tokenizer, "model-b", 10)
	if !errors.Is(err, ErrTokenizerUnavailable) {
		t.Fatalf("expected tokenizer mismatch rejection, got %v", err)
	}
}
