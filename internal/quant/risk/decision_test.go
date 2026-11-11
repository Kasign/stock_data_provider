package risk

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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

func TestDecisionValidateRequiresAggregatedStatus(t *testing.T) {
	decision := sampleDecision()
	if err := decision.Validate(); err != nil {
		t.Fatalf("aggregated unknown status rejected: %v", err)
	}
	for _, status := range []DecisionStatus{DecisionPass, DecisionReject} {
		decision := sampleDecision()
		decision.Status = status
		if err := decision.Validate(); !errors.Is(err, ErrInvalidDecision) {
			t.Errorf("status %q: got %v, want ErrInvalidDecision", status, err)
		}
	}
}

func TestDecisionValidateRejectsInvalidFields(t *testing.T) {
	cases := map[string]func(*Decision){
		"missing decision id":           func(d *Decision) { d.DecisionID = "" },
		"decision id padded":            func(d *Decision) { d.DecisionID = " risk-decision-001 " },
		"rule version padded":           func(d *Decision) { d.RuleSetVersion = " 1.0.0 " },
		"input ref padded":              func(d *Decision) { d.InputRefs = []string{" quant_result:001 "} },
		"reason code padded":            func(d *Decision) { d.Reasons[0].Code = " INPUT_QUALITY_UNKNOWN " },
		"safe message padded":           func(d *Decision) { d.Reasons[0].SafeMessage = " 输入质量无法确认 " },
		"evidence ref padded":           func(d *Decision) { d.Reasons[0].EvidenceRefs = []string{" evidence:quality:001 "} },
		"missing rule version":          func(d *Decision) { d.RuleSetVersion = "" },
		"nil reasons":                   func(d *Decision) { d.Reasons = nil },
		"empty reasons":                 func(d *Decision) { d.Reasons = []Reason{} },
		"duplicate reason code":         func(d *Decision) { d.Reasons = append(d.Reasons, d.Reasons[0]) },
		"evaluated before system as of": func(d *Decision) { d.EvaluatedAt = "2026-11-09T00:58:00Z" },
		"status disagrees with reasons": func(d *Decision) { d.Status = DecisionPass },
		"nil input refs":                func(d *Decision) { d.InputRefs = nil },
		"empty input ref":               func(d *Decision) { d.InputRefs = []string{""} },
		"nil evidence refs":             func(d *Decision) { d.Reasons[0].EvidenceRefs = nil },
		"empty evidence ref":            func(d *Decision) { d.Reasons[0].EvidenceRefs = []string{" "} },
		"empty safe message":            func(d *Decision) { d.Reasons[0].SafeMessage = " " },
		"unknown decision status":       func(d *Decision) { d.Status = "sideways" },
		"unknown severity":              func(d *Decision) { d.Reasons[0].Severity = "critical" },
		"unknown reason status":         func(d *Decision) { d.Reasons[0].Status = "passed" },
		"non UTC time":                  func(d *Decision) { d.EvaluatedAt = "2026-11-09T09:00:00+08:00" },
		"non canonical time":            func(d *Decision) { d.SystemAsOf = "2026-11-09T00:59:00.000Z" },
		"invalid calendar date":         func(d *Decision) { d.EvaluatedAt = "2026-02-30T01:00:00Z" },
		"invalid UTF-8 decision id":     func(d *Decision) { d.DecisionID = string([]byte{0xff}) },
		"invalid UTF-8 rule version":    func(d *Decision) { d.RuleSetVersion = string([]byte{0xff}) },
		"invalid UTF-8 input ref":       func(d *Decision) { d.InputRefs = []string{string([]byte{0xff})} },
		"invalid UTF-8 reason code":     func(d *Decision) { d.Reasons[0].Code = string([]byte{0xff}) },
		"invalid UTF-8 safe message":    func(d *Decision) { d.Reasons[0].SafeMessage = string([]byte{0xff}) },
		"invalid UTF-8 evidence ref":    func(d *Decision) { d.Reasons[0].EvidenceRefs = []string{string([]byte{0xff})} },
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

func TestDecisionValidateAllowsPresentEmptyInputRefs(t *testing.T) {
	decision := sampleDecision()
	decision.InputRefs = []string{}
	if err := decision.Validate(); err != nil {
		t.Fatalf("empty but present input_refs rejected: %v", err)
	}
}

func TestDecisionMatchesGoldenJSON(t *testing.T) {
	path := filepath.Join("..", "..", "..", "doc", "quant", "golden", "result", "risk-decision-1.0.0.golden.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read RiskDecision Golden: %v", err)
	}

	var decision Decision
	if err := json.Unmarshal(raw, &decision); err != nil {
		t.Fatalf("decode RiskDecision Golden: %v", err)
	}
	if err := decision.Validate(); err != nil {
		t.Fatalf("RiskDecision Golden failed DTO validation: %v", err)
	}

	encoded, err := json.Marshal(decision)
	if err != nil {
		t.Fatalf("marshal RiskDecision: %v", err)
	}
	var got, want any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("decode marshaled RiskDecision: %v", err)
	}
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("decode expected Golden: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RiskDecision JSON mismatch: got %s, want %s", encoded, raw)
	}
}
