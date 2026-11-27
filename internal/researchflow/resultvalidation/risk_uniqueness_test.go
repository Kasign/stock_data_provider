package resultvalidation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	quantresult "example.com/stock-analysis-flow/internal/quant/result"
	"example.com/stock-analysis-flow/internal/quant/risk"
)

// riskEnvelopeV2 构造一个由 Quant 权威 Decision 承载的 risk Envelope，
// 携带 2.0.0 的解释字段，作为 Composer 的唯一风险来源。
func riskEnvelopeV2(t *testing.T, decision risk.Decision) QuantResultEnvelope {
	t.Helper()
	payload, err := quantresult.NewRiskPayloadV2(decision)
	if err != nil {
		t.Fatalf("build risk payload v2: %v", err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	return QuantResultEnvelope{
		SchemaVersion: quantresult.EnvelopeSchemaVersion, PayloadSchemaVersion: "2.0.0",
		ResultID: "quant_result:sha256:" + strings.Repeat("c", 64), ResultType: "risk",
		Scope: "stock", SecurityID: "600000.SH", SystemAsOf: now,
		TradeDate: "2026-11-09", Period: "20d", AlgorithmVersion: "risk-evaluator.v1",
		ParameterFingerprint: "sha256:risk-parameters", InputSnapshotRefs: []string{"snapshot-1"},
		QualityStatus: "valid", ResultStatus: "completed", AuthorizationRef: "auth-risk-1",
		Payload: raw,
	}
}

func authoritativeRiskDecision() risk.Decision {
	ruleRef := "rule:hard_gate:v2"
	observed := json.RawMessage(`12.5`)
	expected := "0-10"
	remediation := "REDUCE_POSITION"
	return risk.Decision{
		DecisionID: "risk-v2", Status: risk.DecisionReject, RuleSetVersion: "risk-v2-rules",
		EvaluatedAt: "2026-11-09T08:00:00Z", SystemAsOf: "2026-11-09T08:00:00Z",
		Reasons: []risk.Reason{{
			Code: "HARD_GATE", Severity: risk.SeverityBlock, Status: risk.ReasonTriggered,
			SafeMessage: "硬门禁已触发", EvidenceRefs: []string{"ev-1"},
			RuleRef: &ruleRef, ObservedValue: observed, ExpectedRange: &expected, RemediationCode: &remediation,
		}},
		InputRefs: []string{"quant_result:sha256:" + strings.Repeat("a", 64), "snapshot-1"},
	}
}

// TestRiskDecisionFromQuantEnvelopeProjectsAuthoritativeFields 证明投影视图
// 完整保留 Quant 权威 Decision 的解释字段，不再另立平行来源。
func TestRiskDecisionFromQuantEnvelopeProjectsAuthoritativeFields(t *testing.T) {
	envelope := riskEnvelopeV2(t, authoritativeRiskDecision())
	view, err := RiskDecisionFromQuantEnvelope(envelope)
	if err != nil {
		t.Fatalf("project risk decision: %v", err)
	}
	if view.DecisionID != "risk-v2" || view.Status != "reject" {
		t.Fatalf("decision identity misprojected: %+v", view)
	}
	if len(view.Reasons) != 1 {
		t.Fatalf("reasons = %d, want 1", len(view.Reasons))
	}
	reason := view.Reasons[0]
	if reason.RuleRef == nil || *reason.RuleRef != "rule:hard_gate:v2" ||
		reason.ExpectedRange == nil || *reason.ExpectedRange != "0-10" ||
		reason.RemediationCode == nil || *reason.RemediationCode != "REDUCE_POSITION" {
		t.Fatalf("explanation fields not projected: %+v", reason)
	}
	if string(reason.ObservedValue) != `12.5` {
		t.Fatalf("observed_value not projected: %s", reason.ObservedValue)
	}
}

// TestRiskDecisionFromQuantEnvelopeRejectsWrongType 证明非 risk Envelope
// 不能充当风险来源。
func TestRiskDecisionFromQuantEnvelopeRejectsWrongType(t *testing.T) {
	envelope := validQuantResult()
	if _, err := RiskDecisionFromQuantEnvelope(envelope); !errors.Is(err, ErrInvalidRiskDecision) {
		t.Fatalf("trend envelope must not project to risk decision, got %v", err)
	}
}

// TestComposeResultRejectsTwoRiskSources 证明同时存在 Quant risk Envelope 与
// 独立 RiskDecision 入参时 fail-closed，杜绝第二份无来源绑定决策。
func TestComposeResultRejectsTwoRiskSources(t *testing.T) {
	input := validCompositionInput()
	input.QuantResults = []QuantResultEnvelope{validQuantResult(), riskEnvelopeV2(t, authoritativeRiskDecision())}
	standalone := validRiskDecision()
	input.RiskDecision = &standalone
	registry, err := NewQuantPayloadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ComposeResult(input, registry); !errors.Is(err, ErrInvalidResultComposition) {
		t.Fatalf("two risk sources must fail closed, got %v", err)
	}
}

// TestComposeResultProjectsRiskFromEnvelope 证明当风险决策由 Quant Envelope
// 提供时，Composer 投影输出而非消费独立入参，且解释字段完整保留。
func TestComposeResultProjectsRiskFromEnvelope(t *testing.T) {
	input := validCompositionInput()
	input.QuantResults = []QuantResultEnvelope{validQuantResult(), riskEnvelopeV2(t, authoritativeRiskDecision())}
	input.Limitations = append(input.Limitations, "硬门禁已触发")
	registry, err := NewQuantPayloadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result, err := ComposeResult(input, registry)
	if err != nil {
		t.Fatalf("compose with authoritative risk: %v", err)
	}
	if result.RiskDecision == nil || result.RiskDecision.DecisionID != "risk-v2" {
		t.Fatalf("risk decision not projected from envelope: %+v", result.RiskDecision)
	}
	if result.RiskDecision.Reasons[0].RuleRef == nil || *result.RiskDecision.Reasons[0].RuleRef != "rule:hard_gate:v2" {
		t.Fatalf("explanation field dropped in composition: %+v", result.RiskDecision.Reasons[0])
	}
}

// TestComposeResultRejectsRiskPayloadSchemaMismatch 证明 1.0.0 risk payload
// 声明不能悄悄升级为 2.0.0；版本由 Envelope 严格绑定。
func TestComposeResultRejectsRiskPayloadSchemaMismatch(t *testing.T) {
	envelope := riskEnvelopeV2(t, authoritativeRiskDecision())
	envelope.PayloadSchemaVersion = "1.0.0"
	input := validCompositionInput()
	input.QuantResults = []QuantResultEnvelope{validQuantResult(), envelope}
	registry, err := NewQuantPayloadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ComposeResult(input, registry); !errors.Is(err, ErrInvalidResultComposition) {
		t.Fatalf("risk payload schema mismatch must fail closed, got %v", err)
	}
}

// riskDecisionWithStatus 构造指定 status/reason status 的权威决策，
// 用于四类 fail-closed 契约测试。
func riskDecisionWithStatus(decisionStatus risk.DecisionStatus, reasonStatus risk.ReasonStatus) risk.Decision {
	decision := authoritativeRiskDecision()
	decision.Status = decisionStatus
	decision.Reasons[0].Status = reasonStatus
	return decision
}

// TestComposeResultRiskFailClosedContract 覆盖 RF-BLOCK-008 解除条件要求的
// 四类 fail-closed：block triggered 被保留为 reject、block unknown 保留为
// unknown、缺少 block 由 Quant 聚合为 unknown、status 与 Reason 不一致拒绝。
func TestComposeResultRiskFailClosedContract(t *testing.T) {
	t.Run("block triggered preserved as reject", func(t *testing.T) {
		decision := riskDecisionWithStatus(risk.DecisionReject, risk.ReasonTriggered)
		if decision.Validate() != nil {
			t.Fatalf("reject with triggered block should validate")
		}
		envelope := riskEnvelopeV2(t, decision)
		view, err := RiskDecisionFromQuantEnvelope(envelope)
		if err != nil || view.Status != "reject" {
			t.Fatalf("reject must be preserved, view=%+v err=%v", view, err)
		}
	})

	t.Run("block unknown preserved as unknown", func(t *testing.T) {
		decision := riskDecisionWithStatus(risk.DecisionUnknown, risk.ReasonUnknown)
		if decision.Validate() != nil {
			t.Fatalf("unknown block should validate")
		}
		envelope := riskEnvelopeV2(t, decision)
		view, err := RiskDecisionFromQuantEnvelope(envelope)
		if err != nil || view.Status != "unknown" {
			t.Fatalf("unknown must be preserved, view=%+v err=%v", view, err)
		}
	})

	t.Run("missing block aggregates to unknown", func(t *testing.T) {
		decision := authoritativeRiskDecision()
		decision.Reasons[0].Severity = risk.SeverityWarn
		if got := risk.AggregateStatus(decision.Reasons); got != risk.DecisionUnknown {
			t.Fatalf("missing block must aggregate to unknown, got %s", got)
		}
		if decision.Validate() == nil {
			t.Fatal("decision claiming reject without a block must fail authoritative validation")
		}
		envelope := riskEnvelopeV2(t, authoritativeRiskDecision())
		// 直接注入非法 payload：status=reject 但无 block，权威 Validate 必须拒绝。
		envelope.Payload = json.RawMessage(`{"decision_id":"risk-v2","status":"reject","rule_set_version":"v2","evaluated_at":"2026-11-09T08:00:00Z","system_as_of":"2026-11-09T08:00:00Z","reasons":[{"code":"HARD_GATE","severity":"warn","status":"triggered","safe_message":"x","evidence_refs":["ev-1"]}],"input_refs":[]}`)
		if _, err := RiskDecisionFromQuantEnvelope(envelope); !errors.Is(err, ErrInvalidRiskDecision) {
			t.Fatalf("status/reason inconsistency must fail closed, got %v", err)
		}
	})

	t.Run("status inconsistent with reason is rejected", func(t *testing.T) {
		decision := riskDecisionWithStatus(risk.DecisionPass, risk.ReasonTriggered)
		if decision.Validate() == nil {
			t.Fatal("pass with triggered block must fail authoritative validation")
		}
		envelope := riskEnvelopeV2(t, authoritativeRiskDecision())
		envelope.Payload = json.RawMessage(`{"decision_id":"risk-v2","status":"pass","rule_set_version":"v2","evaluated_at":"2026-11-09T08:00:00Z","system_as_of":"2026-11-09T08:00:00Z","reasons":[{"code":"HARD_GATE","severity":"block","status":"triggered","safe_message":"x","evidence_refs":["ev-1"]}],"input_refs":[]}`)
		if _, err := RiskDecisionFromQuantEnvelope(envelope); !errors.Is(err, ErrInvalidRiskDecision) {
			t.Fatalf("pass/triggered contradiction must fail closed, got %v", err)
		}
	})
}
