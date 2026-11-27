package resultvalidation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func validPersistableComposedResult(t *testing.T) ComposedResearchResult {
	t.Helper()
	input := validCompositionInput()
	input.QuantResults = []QuantResultEnvelope{validQuantResult()}
	risk := validRiskDecision()
	input.RiskDecision = &risk
	registry, err := NewQuantPayloadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	composed, err := ComposeResult(input, registry)
	if err != nil {
		t.Fatal(err)
	}
	return composed
}

func assertRunResultRejected(t *testing.T, composed ComposedResearchResult, runRef string) {
	t.Helper()
	if _, err := RunResultFromComposed(composed, runRef); !errors.Is(err, ErrRunResultNotPersistable) {
		t.Fatalf("RunResult projection error = %v, want ErrRunResultNotPersistable", err)
	}
}

func TestRunResultFromComposedProjectsPersistableResult(t *testing.T) {
	composed := validPersistableComposedResult(t)
	payload, err := RunResultFromComposed(composed, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if payload.SchemaVersion != RunResultPayloadSchemaVersion || payload.RunRef != "run-1" {
		t.Fatalf("unexpected RunResult identity: %+v", payload)
	}
	if payload.SystemAsOf != "2026-11-09T08:00:00Z" {
		t.Fatalf("system_as_of = %v, want 2026-11-09T08:00:00Z", payload.SystemAsOf)
	}
	if payload.Sources.ContextID != "ctx-1" || payload.Sources.ContextVersion != 1 {
		t.Fatalf("context source binding missing: %+v", payload.Sources)
	}
	if len(payload.Sources.QuantResultRefs) != 1 ||
		payload.Sources.QuantResultRefs[0] != composed.QuantResults[0].Result.ResultID {
		t.Fatalf("quant result refs = %v", payload.Sources.QuantResultRefs)
	}
	if len(payload.Sources.QuantSnapshotRefs) != 1 || payload.Sources.QuantSnapshotRefs[0] != "snapshot-1" {
		t.Fatalf("quant snapshot refs = %v", payload.Sources.QuantSnapshotRefs)
	}
	if len(payload.Sources.EvidenceRefs) != 1 || payload.Sources.EvidenceRefs[0] != "ev-1" {
		t.Fatalf("evidence refs = %v", payload.Sources.EvidenceRefs)
	}
	if len(payload.Sources.ClaimRefs) != 1 || payload.Sources.ClaimRefs[0] != "clm-1" {
		t.Fatalf("claim refs = %v", payload.Sources.ClaimRefs)
	}
	if payload.Explanation.Text != composed.Explanation.Text ||
		payload.Explanation.Authority != LLMExplanationAuthority ||
		payload.Explanation.ResponseID != "resp-1" {
		t.Fatalf("LLM explanation was dropped or promoted: %+v", payload.Explanation)
	}
	if payload.RiskDecision == nil || payload.RiskDecision.DecisionID != composed.RiskDecision.DecisionID {
		t.Fatalf("risk projection was dropped: %+v", payload.RiskDecision)
	}
	if len(payload.Limitations) != 1 || payload.Limitations[0] != "仅覆盖已授权的输入快照" {
		t.Fatalf("limitations = %v", payload.Limitations)
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var topLevel map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &topLevel); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"quant_results", "evidence", "claims", "context"} {
		if _, exists := topLevel[forbidden]; exists {
			t.Fatalf("RunResult payload copied source resource %q", forbidden)
		}
	}
}

func TestRunResultFromComposedRejectsRunRefMismatchAndClockMismatch(t *testing.T) {
	composed := validPersistableComposedResult(t)
	assertRunResultRejected(t, composed, "run-2")

	clockMismatch := composed
	clockMismatch.SystemAsOf = composed.SystemAsOf.Add(1)
	assertRunResultRejected(t, clockMismatch, "run-1")
}

func TestRunResultFromComposedRejectsMissingExplanation(t *testing.T) {
	composed := validPersistableComposedResult(t)
	composed.Explanation = nil
	assertRunResultRejected(t, composed, "run-1")
}

func TestRunResultFromComposedRejectsNonPersistableExplanation(t *testing.T) {
	for name, mutate := range map[string]func(*LLMExplanation){
		"empty text":        func(value *LLMExplanation) { value.Text = "  " },
		"wrong authority":   func(value *LLMExplanation) { value.Authority = "authoritative" },
		"missing provider":  func(value *LLMExplanation) { value.ProviderID = "" },
		"missing model":     func(value *LLMExplanation) { value.ModelRequested = "" },
		"missing response":  func(value *LLMExplanation) { value.ResponseID = "" },
		"untrimmed version": func(value *LLMExplanation) { value.ModelVersion = " v1 " },
	} {
		t.Run(name, func(t *testing.T) {
			composed := validPersistableComposedResult(t)
			explanation := *composed.Explanation
			mutate(&explanation)
			composed.Explanation = &explanation
			assertRunResultRejected(t, composed, "run-1")
		})
	}
}

func TestRunResultFromComposedRejectsEmptySourceSets(t *testing.T) {
	composed := validPersistableComposedResult(t)
	composed.QuantResults = nil
	assertRunResultRejected(t, composed, "run-1")

	composed = validPersistableComposedResult(t)
	composed.QuantResults[0].Usable = false
	assertRunResultRejected(t, composed, "run-1")

	composed = validPersistableComposedResult(t)
	composed.Evidence = nil
	assertRunResultRejected(t, composed, "run-1")

	composed = validPersistableComposedResult(t)
	composed.Claims = nil
	assertRunResultRejected(t, composed, "run-1")
}

func TestRunResultFromComposedRejectsMissingContextAndRisk(t *testing.T) {
	composed := validPersistableComposedResult(t)
	composed.Context.ContextID = ""
	assertRunResultRejected(t, composed, "run-1")

	composed = validPersistableComposedResult(t)
	composed.RiskDecision = nil
	assertRunResultRejected(t, composed, "run-1")
}

func TestRunResultFromComposedRejectsInvalidSchemaVersion(t *testing.T) {
	composed := validPersistableComposedResult(t)
	if _, err := RunResultFromComposed(composed, "run-1"); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(RunResultPayloadSchemaVersion) == "" {
		t.Fatal("schema version constant must be non-empty")
	}
}
