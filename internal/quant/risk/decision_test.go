package risk

import (
	"errors"
	"testing"
)

func sampleDecision() Decision {
	return Decision{
		DecisionID:     "risk-decision-001",
		Status:         DecisionUnknown,
		RuleSetVersion: "1.0.0",
		EvaluatedAt:    "2026-11-09T01:00:00Z",
		SystemAsOf:     "2026-11-09T00:59:00Z",
		Reasons: []Reason{{
			Code:         "INPUT_QUALITY_UNKNOWN",
			Severity:     SeverityBlock,
			Status:       ReasonUnknown,
			SafeMessage:  "输入质量无法确认",
			EvidenceRefs: []string{"evidence:quality:001"},
		}},
		InputRefs: []string{"quant_result:001"},
	}
}

func TestDecisionValidateAcceptsUnknownFailClosedRepresentation(t *testing.T) {
	if err := sampleDecision().Validate(); err != nil {
		t.Fatalf("valid decision rejected: %v", err)
	}
}

func TestDecisionValidateDoesNotInferRuleOutcome(t *testing.T) {
	for _, status := range []DecisionStatus{DecisionPass, DecisionReject, DecisionUnknown} {
		decision := sampleDecision()
		decision.Status = status
		if err := decision.Validate(); err != nil {
			t.Errorf("status %q rejected: %v", status, err)
		}
	}
}

func TestDecisionValidateRejectsInvalidFields(t *testing.T) {
	cases := map[string]func(*Decision){
		"missing decision id":       func(d *Decision) { d.DecisionID = "" },
		"missing rule version":      func(d *Decision) { d.RuleSetVersion = "" },
		"nil reasons":               func(d *Decision) { d.Reasons = nil },
		"nil input refs":            func(d *Decision) { d.InputRefs = nil },
		"empty input ref":           func(d *Decision) { d.InputRefs = []string{""} },
		"nil evidence refs":         func(d *Decision) { d.Reasons[0].EvidenceRefs = nil },
		"empty evidence ref":        func(d *Decision) { d.Reasons[0].EvidenceRefs = []string{" "} },
		"empty safe message":        func(d *Decision) { d.Reasons[0].SafeMessage = " " },
		"unknown decision status":   func(d *Decision) { d.Status = "sideways" },
		"unknown severity":          func(d *Decision) { d.Reasons[0].Severity = "critical" },
		"unknown reason status":     func(d *Decision) { d.Reasons[0].Status = "passed" },
		"non UTC time":              func(d *Decision) { d.EvaluatedAt = "2026-11-09T09:00:00+08:00" },
		"non canonical time":        func(d *Decision) { d.SystemAsOf = "2026-11-09T00:59:00.000Z" },
		"invalid calendar date":     func(d *Decision) { d.EvaluatedAt = "2026-02-30T01:00:00Z" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			decision := sampleDecision()
			mutate(&decision)
			if err := decision.Validate(); !errors.Is(err, ErrInvalidDecision) {
				t.Fatalf("got %v, want ErrInvalidDecision", err)
			}
		})
	}
}

func TestDecisionValidateAllowsPresentEmptyArrays(t *testing.T) {
	decision := sampleDecision()
	decision.Reasons = []Reason{}
	decision.InputRefs = []string{}
	if err := decision.Validate(); err != nil {
		t.Fatalf("empty but present arrays rejected: %v", err)
	}
}
