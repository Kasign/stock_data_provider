package risk

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func ptrV2(value string) *string { return &value }

func validDecisionV2() Decision {
	return Decision{
		DecisionID: "risk-v2", Status: DecisionReject, RuleSetVersion: "risk-v2-rules",
		EvaluatedAt: "2026-11-09T01:00:00Z", SystemAsOf: "2026-11-09T00:59:00Z",
		Reasons: []Reason{{
			Code: "HARD_GATE", Severity: SeverityBlock, Status: ReasonTriggered,
			SafeMessage: "硬门禁已触发", EvidenceRefs: []string{},
			RuleRef: ptrV2("rule:hard_gate:v2"), ObservedValue: json.RawMessage(`12.5`),
			ExpectedRange: ptrV2("0-10"), RemediationCode: ptrV2("REDUCE_POSITION"),
		}},
		InputRefs: []string{"quant_result:sha256:" + strings.Repeat("a", 64)},
	}
}

// TestDecisionV2AcceptsExplanationFields 证明 2.0.0 的四个解释字段是权威
// RiskDecision 的一部分，而不是 Researchflow 另立的平行结构。
func TestDecisionV2AcceptsExplanationFields(t *testing.T) {
	if err := validDecisionV2().Validate(); err != nil {
		t.Fatalf("decision with explanation fields should validate: %v", err)
	}
}

// TestDecisionV2RejectsNonCanonicalExplanationFields 证明解释字段与既有字段
// 遵守同样的 canonical 约束（有效 UTF-8、非空、无首尾空白）。
func TestDecisionV2RejectsNonCanonicalExplanationFields(t *testing.T) {
	cases := map[string]func(*Reason){
		"padded rule ref":       func(r *Reason) { r.RuleRef = ptrV2(" rule:hard_gate:v2 ") },
		"empty rule ref":        func(r *Reason) { r.RuleRef = ptrV2(" ") },
		"padded expected range": func(r *Reason) { r.ExpectedRange = ptrV2(" 0-10 ") },
		"empty remediation":     func(r *Reason) { r.RemediationCode = ptrV2(" ") },
		"non scalar observed":   func(r *Reason) { r.ObservedValue = json.RawMessage(`{"value":12.5}`) },
		"invalid observed json": func(r *Reason) { r.ObservedValue = json.RawMessage(`{`) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			value := validDecisionV2()
			mutate(&value.Reasons[0])
			if err := value.Validate(); !errors.Is(err, ErrInvalidDecision) {
				t.Fatalf("non-canonical explanation field must fail closed, got %v", err)
			}
		})
	}
}

// TestDecisionV2ExplanationFieldsAreDeepCopied 证明 NewDecision 的 cloneReasons
// 深拷贝新增字段，避免调用方修改已构造的权威决策。
func TestDecisionV2ExplanationFieldsAreDeepCopied(t *testing.T) {
	original := validDecisionV2()
	decision, err := NewDecision(DecisionInput{
		DecisionID:     original.DecisionID,
		RuleSetVersion: original.RuleSetVersion, EvaluatedAt: original.EvaluatedAt,
		SystemAsOf: original.SystemAsOf, Reasons: original.Reasons, InputRefs: original.InputRefs,
	})
	if err != nil {
		t.Fatalf("build decision: %v", err)
	}
	*original.Reasons[0].RuleRef = "mutated"
	*original.Reasons[0].ExpectedRange = "mutated"
	*original.Reasons[0].RemediationCode = "mutated"
	original.Reasons[0].ObservedValue = json.RawMessage(`999`)
	if *decision.Reasons[0].RuleRef != "rule:hard_gate:v2" ||
		*decision.Reasons[0].ExpectedRange != "0-10" ||
		*decision.Reasons[0].RemediationCode != "REDUCE_POSITION" ||
		string(decision.Reasons[0].ObservedValue) != `12.5` {
		t.Fatalf("NewDecision did not isolate explanation fields: %+v", decision.Reasons[0])
	}
}

// TestDecisionV2AggregateStatusUnaffected 证明解释字段不改变状态聚合语义：
// 触发的 block 仍是 reject，全部 not_triggered 才是 pass。
func TestDecisionV2AggregateStatusUnaffected(t *testing.T) {
	base := validDecisionV2()
	base.Reasons[0].Status = ReasonNotTriggered
	if got := AggregateStatus(base.Reasons); got != DecisionPass {
		t.Fatalf("all not_triggered block should be pass, got %s", got)
	}
	base.Reasons[0].Status = ReasonUnknown
	if got := AggregateStatus(base.Reasons); got != DecisionUnknown {
		t.Fatalf("unknown block should be unknown, got %s", got)
	}
}
