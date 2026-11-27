package result

import (
	"encoding/json"
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/quant/risk"
)

func decisionWithExplanation() risk.Decision {
	ruleRef := "rule:hard_gate:v2"
	observed := json.RawMessage(`12.5`)
	expected := "0-10"
	remediation := "REDUCE_POSITION"
	return risk.Decision{
		DecisionID: "risk-v2", Status: risk.DecisionReject, RuleSetVersion: "risk-v2-rules",
		EvaluatedAt: "2026-11-09T01:00:00Z", SystemAsOf: "2026-11-09T00:59:00Z",
		Reasons: []risk.Reason{{
			Code: "HARD_GATE", Severity: risk.SeverityBlock, Status: risk.ReasonTriggered,
			SafeMessage: "硬门禁已触发", EvidenceRefs: []string{},
			RuleRef: &ruleRef, ObservedValue: observed, ExpectedRange: &expected, RemediationCode: &remediation,
		}},
		InputRefs: []string{},
	}
}

// TestRiskPayloadV2CarriesExplanationFields 证明 2.0.0 Payload 保留解释字段，
// 且其 schema version 与 Envelope 绑定值一致。
func TestRiskPayloadV2CarriesExplanationFields(t *testing.T) {
	payload, err := NewRiskPayloadV2(decisionWithExplanation())
	if err != nil {
		t.Fatalf("build risk payload v2: %v", err)
	}
	if payload.payloadSchemaVersion() != "2.0.0" {
		t.Fatalf("payload schema version = %q, want 2.0.0", payload.payloadSchemaVersion())
	}
	raw, err := payload.marshalPayload()
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	reasons, ok := decoded["reasons"].([]any)
	if !ok || len(reasons) == 0 {
		t.Fatalf("no reasons in payload: %s", raw)
	}
	first := reasons[0].(map[string]any)
	for _, field := range []string{"rule_ref", "observed_value", "expected_range", "remediation_code"} {
		if _, exists := first[field]; !exists {
			t.Fatalf("2.0.0 payload dropped %s: %s", field, raw)
		}
	}
}

// TestDecodePayloadRiskV2RoundTrip 证明 2.0.0 解码严格保留解释字段。
func TestDecodePayloadRiskV2RoundTrip(t *testing.T) {
	payload, err := NewRiskPayloadV2(decisionWithExplanation())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := payload.marshalPayload()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodePayload("risk", "2.0.0", raw)
	if err != nil {
		t.Fatalf("decode risk/2.0.0: %v", err)
	}
	got, ok := decoded.(RiskPayload)
	if !ok || got.payloadSchemaVersion() != "2.0.0" {
		t.Fatalf("decoded payload = %T/%q", decoded, payloadSchemaVersionOf(decoded))
	}
	decision := got.Decision()
	if decision.Reasons[0].RuleRef == nil || *decision.Reasons[0].RuleRef != "rule:hard_gate:v2" {
		t.Fatalf("rule_ref not preserved: %+v", decision.Reasons[0])
	}
	if string(decision.Reasons[0].ObservedValue) != `12.5` {
		t.Fatalf("observed_value not preserved: %s", decision.Reasons[0].ObservedValue)
	}
}

// TestDecodePayloadRiskV1RejectsExplanationFields 证明 1.0.0 冻结语义不变：
// 携带解释字段的 payload 不能用 1.0.0 声明，避免绕过契约版本边界。
func TestDecodePayloadRiskV1RejectsExplanationFields(t *testing.T) {
	payload, err := NewRiskPayloadV2(decisionWithExplanation())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := payload.marshalPayload()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodePayload("risk", "1.0.0", raw); err == nil {
		t.Fatal("risk/1.0.0 must reject payload with 2.0.0 explanation fields")
	}
}

// TestDecodePayloadRiskV2RejectsNonCanonical 证明 2.0.0 仍 fail-closed。
func TestDecodePayloadRiskV2RejectsNonCanonical(t *testing.T) {
	raw := json.RawMessage(`{"decision_id":"risk-v2","status":"reject","rule_set_version":"r","evaluated_at":"2026-11-09T01:00:00Z","system_as_of":"2026-11-09T00:59:00Z","reasons":[{"code":"HARD_GATE","severity":"block","status":"triggered","safe_message":"x","evidence_refs":[],"rule_ref":" "}],"input_refs":[]}`)
	if _, err := DecodePayload("risk", "2.0.0", raw); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("non-canonical rule_ref must fail closed, got %v", err)
	}
}

func payloadSchemaVersionOf(payload Payload) string {
	if payload == nil {
		return "<nil>"
	}
	return payload.payloadSchemaVersion()
}
