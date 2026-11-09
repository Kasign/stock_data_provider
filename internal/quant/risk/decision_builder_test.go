package risk

import (
	"errors"
	"testing"
)

func TestNewDecisionDerivesStatusAndCopiesInputs(t *testing.T) {
	input := DecisionInput{
		DecisionID: "risk-decision-002", RuleSetVersion: "1.0.0",
		EvaluatedAt: "2026-11-09T01:00:00Z", SystemAsOf: "2026-11-09T00:59:00Z",
		Reasons: []Reason{{
			Code: "PRICE_LIMIT", Severity: SeverityBlock, Status: ReasonNotTriggered,
			SafeMessage: "未触发价格限制", EvidenceRefs: []string{"evidence:price:001"},
		}},
		InputRefs: []string{"quant_result:002"},
	}

	decision, err := NewDecision(input)
	if err != nil {
		t.Fatalf("NewDecision() error = %v", err)
	}
	if decision.Status != DecisionPass {
		t.Fatalf("status = %q, want pass", decision.Status)
	}

	input.Reasons[0].EvidenceRefs[0] = "mutated"
	input.InputRefs[0] = "mutated"
	if decision.Reasons[0].EvidenceRefs[0] != "evidence:price:001" ||
		decision.InputRefs[0] != "quant_result:002" {
		t.Fatal("decision retained caller-owned slice references")
	}
}

func TestNewDecisionFailsClosedForUnknownHardGate(t *testing.T) {
	input := DecisionInput{
		DecisionID: "risk-decision-003", RuleSetVersion: "1.0.0",
		EvaluatedAt: "2026-11-09T01:00:00Z", SystemAsOf: "2026-11-09T00:59:00Z",
		Reasons: []Reason{{
			Code: "INPUT_QUALITY", Severity: SeverityBlock, Status: ReasonUnknown,
			SafeMessage: "输入质量未知", EvidenceRefs: []string{},
		}},
		InputRefs: []string{"quant_result:003"},
	}
	decision, err := NewDecision(input)
	if err != nil {
		t.Fatalf("NewDecision() error = %v", err)
	}
	if decision.Status != DecisionUnknown {
		t.Fatalf("status = %q, want unknown", decision.Status)
	}
}

func TestNewDecisionRejectsInvalidDTO(t *testing.T) {
	input := DecisionInput{
		DecisionID: "risk-decision-004", RuleSetVersion: "1.0.0",
		EvaluatedAt: "2026-11-09T01:00:00Z", SystemAsOf: "2026-11-09T00:59:00Z",
		Reasons: []Reason{{
			Code: "BAD", Severity: "critical", Status: ReasonNotTriggered,
			SafeMessage: "invalid severity", EvidenceRefs: []string{},
		}},
		InputRefs: []string{"quant_result:004"},
	}
	if _, err := NewDecision(input); !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("error = %v, want ErrInvalidDecision", err)
	}
}

func TestNewDecisionPreservesPresentEmptyCollections(t *testing.T) {
	input := DecisionInput{
		DecisionID: "risk-decision-005", RuleSetVersion: "1.0.0",
		EvaluatedAt: "2026-11-09T01:00:00Z", SystemAsOf: "2026-11-09T00:59:00Z",
		Reasons: []Reason{}, InputRefs: []string{},
	}
	decision, err := NewDecision(input)
	if err != nil {
		t.Fatalf("NewDecision() error = %v", err)
	}
	if decision.Reasons == nil || decision.InputRefs == nil || decision.Status != DecisionUnknown {
		t.Fatalf("unexpected empty decision: %+v", decision)
	}
}
