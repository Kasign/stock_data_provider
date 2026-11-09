package resultvalidation

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	llmapp "example.com/stock-analysis-flow/internal/llm"
	researchapp "example.com/stock-analysis-flow/internal/research/application"
)

func validCompositionInput() ResultCompositionInput {
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	return ResultCompositionInput{
		RunID: "run-1", SystemAsOf: now,
		Context: researchapp.ResearchContext{
			SchemaVersion: "1.0.0", ContextID: "ctx-1", ContextVersion: 1,
			ResearchQuestion: "分析目标证券", Scope: "stock", SubjectRefs: []string{"600000.SH"},
			SystemAsOf: now,
			SectionRefs: []researchapp.ContextSectionRef{{
				SectionID: "market", SectionVersion: "1", SourceOwner: "data",
				SourceRefs: []string{"snapshot-1"}, EvidenceRefs: []string{"ev-1"},
				QualityStatus: "valid", Required: true, Included: true, AssembledAt: now, SystemAsOf: now,
			}},
			QualitySummary:   researchapp.ContextQualitySummary{Status: "valid", ReasonCodes: []string{}},
			IncludedSections: []string{"market"}, OmittedSections: []researchapp.ContextOmittedSection{},
			BuilderVersion: "builder-v1", CreatedAt: now,
		},
		Evidence: []researchapp.EvidenceRecord{{
			EvidenceDraft: researchapp.EvidenceDraft{
				EvidenceType: "fact", SourceOwner: "data", SourceRef: "snapshot-1",
				SourceVersion: nil, SourceLocator: nil, DerivedFromRefs: []string{},
				SubjectRefs: []string{"600000.SH"}, PublishedAt: nil, AvailableAt: nil,
				SystemObservedAt: now, ValidTimeStart: nil, ValidTimeEnd: nil, SystemAsOf: now,
				Stance: "supports", QualityStatus: "valid", ContentLevel: "summary",
				AuthorizationRef: "auth-ev-1", Limitations: []string{},
			},
			EvidenceID:         "ev-1",
			ContentFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}},
		Claims: []researchapp.Claim{{
			ClaimID: "clm-1", ClaimVersion: 1, Statement: "成交量保持稳定",
			EpistemicType: "reported_fact", ScopeRefs: []string{"600000.SH"},
			SystemAsOf: now, SupportingEvidenceRefs: []string{"ev-1"},
			ContradictingEvidenceRefs: []string{}, RequiredDataRefs: []string{},
			InvalidationConditions: []string{}, Status: "proposed", Limitations: []string{},
		}},
		LLMResponse: &llmapp.ModelResponse{
			RequestID: "req-1", CorrelationID: "corr-1", SystemAsOf: now,
			ProviderID: "provider-a", ResponseID: "resp-1",
			Status: llmapp.ResponseCompleted, OutputItems: []llmapp.OutputItem{},
			OutputText:     "基于已提供证据，成交量变化需要结合价格结构观察。",
			ModelRequested: "model-a", ModelActual: "model-a", ModelVersion: "v1",
			StartedAt: now, FinishedAt: now, DurationMS: 1,
		},
		Limitations: []string{"仅覆盖已授权的输入快照"},
	}
}

func TestComposeResultKeepsFactsEvidenceExplanationAndRiskSeparate(t *testing.T) {
	input := validCompositionInput()
	input.QuantResults = []QuantResultEnvelope{validQuantResult()}
	registry := NewRegistry()
	if err := registry.Register("indicator", "1.0.0", PayloadValidatorFunc(func(payload json.RawMessage) error { return nil })); err != nil {
		t.Fatal(err)
	}
	risk := validRiskDecision()
	input.RiskDecision = &risk
	result, err := ComposeResult(input, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.QuantResults) != 1 || !result.QuantResults[0].Usable ||
		len(result.Evidence) != 1 || len(result.Claims) != 1 ||
		result.Explanation == nil || result.Explanation.Authority != LLMExplanationAuthority ||
		result.RiskDecision == nil || result.RiskDecision.Status != "pass" {
		t.Fatalf("unexpected composed result: %+v", result)
	}
	if result.QuantResults[0].Result.Payload[0] != '{' ||
		result.Explanation.Text == "" || result.RiskDecision.Status != input.RiskDecision.Status {
		t.Fatal("composition changed or dropped source facts")
	}
	input.Evidence[0].EvidenceDraft.SubjectRefs[0] = "mutated"
	input.Claims[0].Statement = "mutated"
	if result.Evidence[0].SubjectRefs[0] != "600000.SH" || result.Claims[0].Statement == "mutated" {
		t.Fatal("composed result did not isolate mutable source slices")
	}
}

func TestComposeResultRetainsNonUsableQuantResultsWithoutPromotingThem(t *testing.T) {
	input := validCompositionInput()
	quant := validQuantResult()
	quant.QualityStatus = "unknown"
	input.QuantResults = []QuantResultEnvelope{quant}
	registry := NewRegistry()
	if err := registry.Register("indicator", "1.0.0", PayloadValidatorFunc(func(json.RawMessage) error { return nil })); err != nil {
		t.Fatal(err)
	}
	result, err := ComposeResult(input, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.QuantResults) != 1 || result.QuantResults[0].Usable ||
		len(result.QuantResults[0].ReasonCodes) != 1 || result.QuantResults[0].ReasonCodes[0] != "quality_not_usable" {
		t.Fatalf("non-usable Quant result was dropped or promoted: %+v", result.QuantResults)
	}
}

func TestComposeResultRejectsMissingEvidenceReferencesAndAsOfMismatch(t *testing.T) {
	input := validCompositionInput()
	input.Claims[0].SupportingEvidenceRefs = []string{"missing-evidence"}
	if _, err := ComposeResult(input, nil); !errors.Is(err, ErrInvalidResultComposition) {
		t.Fatalf("missing evidence reference should fail, got %v", err)
	}
	input = validCompositionInput()
	input.Context.SystemAsOf = input.SystemAsOf.Add(time.Second)
	if _, err := ComposeResult(input, nil); !errors.Is(err, ErrInvalidResultComposition) {
		t.Fatalf("Context as-of mismatch should fail, got %v", err)
	}
}

func TestComposeResultRejectsIncompleteOrToolCallingLLMResponse(t *testing.T) {
	input := validCompositionInput()
	input.LLMResponse.Status = llmapp.ResponseIncomplete
	if _, err := ComposeResult(input, nil); !errors.Is(err, ErrInvalidLLMExplanation) {
		t.Fatalf("incomplete LLM response should fail, got %v", err)
	}
	input = validCompositionInput()
	input.LLMResponse.ToolCalls = []llmapp.ModelToolCall{{CallID: "call-1", Name: "tool", ArgumentsJSON: "{}"}}
	if _, err := ComposeResult(input, nil); !errors.Is(err, ErrInvalidLLMExplanation) {
		t.Fatalf("tool call response should not be treated as explanation, got %v", err)
	}
}

func TestComposeResultRejectsRiskPassContradiction(t *testing.T) {
	input := validCompositionInput()
	risk := validRiskDecision()
	risk.Reasons[0].Status = "triggered"
	input.RiskDecision = &risk
	if _, err := ComposeResult(input, nil); !errors.Is(err, ErrInvalidResultComposition) {
		t.Fatalf("blocking risk must prevent composition as pass, got %v", err)
	}
}

func TestComposeResultAllowsEvidenceKnowledgeCutoffEarlierThanRun(t *testing.T) {
	input := validCompositionInput()
	input.Evidence[0].EvidenceDraft.SystemAsOf = input.SystemAsOf.Add(-time.Hour)
	input.Evidence[0].EvidenceDraft.SystemObservedAt = input.SystemAsOf.Add(-2 * time.Hour)
	if _, err := ComposeResult(input, nil); err != nil {
		t.Fatalf("older Evidence system_as_of should remain valid: %v", err)
	}
}
