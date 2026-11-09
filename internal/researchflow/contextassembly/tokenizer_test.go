package contextassembly

import (
	"errors"
	"testing"
)

type testTokenizer struct {
	modelID string
	version string
	count   func(string) (int, error)
}

func (t testTokenizer) ModelID() string { return t.modelID }
func (t testTokenizer) Version() string { return t.version }
func (t testTokenizer) CountTokens(value string) (int, error) {
	return t.count(value)
}

func TestEstimateSectionTokensUsesModelTokenizerAndPreservesInput(t *testing.T) {
	tokenizer := testTokenizer{
		modelID: "model-a", version: "tokenizer-v3",
		count: func(value string) (int, error) {
			if value == "market authorized text" {
				return 17, nil
			}
			return 9, nil
		},
	}
	candidates := []SectionCandidate{
		{ID: "market", SourceOwner: "data", EstimatedTokens: 999, SourceRefs: []string{"snapshot-1"}},
		{ID: "news", SourceOwner: "news", EstimatedTokens: 888, EvidenceRefs: []string{"ev-1"}},
	}
	contents := []SectionContent{
		{SectionID: "market", Text: "market authorized text"},
		{SectionID: "news", Text: "news authorized text"},
	}
	estimated, provenance, err := EstimateSectionTokens(tokenizer, "model-a", candidates, contents)
	if err != nil {
		t.Fatal(err)
	}
	if provenance.ModelID != "model-a" || provenance.Version != "tokenizer-v3" {
		t.Fatalf("unexpected tokenizer provenance: %+v", provenance)
	}
	if estimated[0].EstimatedTokens != 17 || estimated[1].EstimatedTokens != 9 {
		t.Fatalf("unexpected estimates: %+v", estimated)
	}
	if candidates[0].EstimatedTokens != 999 || candidates[1].EstimatedTokens != 888 {
		t.Fatal("input candidates were mutated")
	}
	estimated[0].SourceRefs[0] = "mutated"
	if candidates[0].SourceRefs[0] != "snapshot-1" {
		t.Fatal("candidate references were not copied")
	}
}

func TestEstimateSectionTokensRejectsUnavailableOrMismatchedTokenizer(t *testing.T) {
	candidate := []SectionCandidate{{ID: "market"}}
	content := []SectionContent{{SectionID: "market", Text: "authorized"}}
	available := testTokenizer{modelID: "model-a", version: "v1", count: func(string) (int, error) { return 1, nil }}
	for _, tc := range []struct {
		tokenizer Tokenizer
		model     string
		want      error
	}{
		{nil, "model-a", ErrTokenizerUnavailable},
		{available, "model-b", ErrTokenizerUnavailable},
		{testTokenizer{modelID: "model-a", version: "", count: available.count}, "model-a", ErrTokenizerUnavailable},
	} {
		if _, _, err := EstimateSectionTokens(tc.tokenizer, tc.model, candidate, content); !errors.Is(err, tc.want) {
			t.Fatalf("expected %v, got %v", tc.want, err)
		}
	}
}

func TestEstimateSectionTokensRejectsIncompleteAndInvalidContent(t *testing.T) {
	tokenizer := testTokenizer{modelID: "model-a", version: "v1", count: func(string) (int, error) { return 1, nil }}
	candidate := []SectionCandidate{{ID: "market"}}
	for _, contents := range [][]SectionContent{
		nil,
		{{SectionID: "market", Text: " "}},
		{{SectionID: "market", Text: "text"}, {SectionID: "market", Text: "duplicate"}},
		{{SectionID: "other", Text: "text"}},
	} {
		if _, _, err := EstimateSectionTokens(tokenizer, "model-a", candidate, contents); !errors.Is(err, ErrInvalidTokenEstimate) {
			t.Fatalf("expected invalid token estimate, got %v", err)
		}
	}
}

func TestEstimateSectionTokensRejectsInvalidCounterResult(t *testing.T) {
	for _, count := range []int{0, -1} {
		tokenizer := testTokenizer{modelID: "model-a", version: "v1", count: func(string) (int, error) { return count, nil }}
		_, _, err := EstimateSectionTokens(tokenizer, "model-a",
			[]SectionCandidate{{ID: "market"}},
			[]SectionContent{{SectionID: "market", Text: "authorized"}},
		)
		if !errors.Is(err, ErrInvalidTokenEstimate) {
			t.Fatalf("count %d should be rejected, got %v", count, err)
		}
	}
}

func TestEstimateSectionTokensPropagatesTokenizerFailure(t *testing.T) {
	want := errors.New("tokenizer failure")
	tokenizer := testTokenizer{modelID: "model-a", version: "v1", count: func(string) (int, error) { return 0, want }}
	_, _, err := EstimateSectionTokens(tokenizer, "model-a",
		[]SectionCandidate{{ID: "market"}},
		[]SectionContent{{SectionID: "market", Text: "authorized"}},
	)
	if !errors.Is(err, want) {
		t.Fatalf("expected tokenizer failure, got %v", err)
	}
}
