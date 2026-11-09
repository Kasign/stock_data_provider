package storage

import (
	"encoding/json"
	"testing"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

func validResultSummaryPayload() []byte {
	summary := domain.ResultSummary{
		StageOutcomes:       map[string]domain.StageStatus{"context": domain.StageSucceeded},
		ExcludedStages:      []domain.ExcludedStage{},
		MissingStageCodes:   []string{},
		DegradedStageCodes:  []string{},
		BlockedStageCodes:   []string{},
		ReasonCodes:         []string{},
		Coverage:            1,
		QualityStatus:      "complete",
		RiskGateDecisions:   map[string]string{},
		ActionabilityStatus: "not_evaluated",
	}
	payload, _ := json.Marshal(summary)
	return payload
}

func TestDecodeResultSummaryAcceptsCompletePayload(t *testing.T) {
	got, err := decodeResultSummary(validResultSummaryPayload())
	if err != nil {
		t.Fatal(err)
	}
	if got.Coverage != 1 || got.StageOutcomes["context"] != domain.StageSucceeded {
		t.Fatalf("unexpected ResultSummary: %#v", got)
	}
}

func TestDecodeResultSummaryRejectsIncompletePayload(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{name: "empty", payload: []byte(" ")},
		{name: "invalid json", payload: []byte("{")},
		{name: "missing collections", payload: []byte(`{"coverage":1,"quality_status":"complete","actionability_status":"not_evaluated"}`)},
		{name: "coverage out of range", payload: []byte(`{"stage_outcomes":{},"excluded_stages":[],"missing_stage_codes":[],"degraded_stage_codes":[],"blocked_stage_codes":[],"reason_codes":[],"coverage":1.1,"quality_status":"complete","risk_gate_decisions":{},"actionability_status":"not_evaluated"}`)},
		{name: "missing projection status", payload: []byte(`{"stage_outcomes":{},"excluded_stages":[],"missing_stage_codes":[],"degraded_stage_codes":[],"blocked_stage_codes":[],"reason_codes":[],"coverage":1,"risk_gate_decisions":{},"actionability_status":"not_evaluated"}`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := decodeResultSummary(test.payload); err == nil {
				t.Fatal("expected malformed ResultSummary to be rejected")
			}
		})
	}
}
