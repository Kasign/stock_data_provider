package resultvalidation

import (
	"errors"
	"testing"
	"time"
)

func validRiskDecision() RiskDecision {
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	return RiskDecision{
		DecisionID: "risk-1", Status: "pass", RuleSetVersion: "risk-v1",
		EvaluatedAt: now, SystemAsOf: now,
		Reasons: []RiskReason{{Code: "max_drawdown", Severity: "block", Status: "not_triggered",
			SafeMessage: "未触发阻断条件", RuleRef: "rule-max-drawdown-v1", EvidenceRefs: []string{"ev-1"}}},
		InputRefs: []string{validQuantResult().ResultID},
	}
}

func TestValidateRiskDecisionRejectsPassWithTriggeredOrUnknownBlock(t *testing.T) {
	base := validRiskDecision()
	for _, status := range []string{"triggered", "unknown"} {
		value := base
		value.Reasons = append([]RiskReason(nil), base.Reasons...)
		value.Reasons[0].Status = status
		if err := ValidateRiskDecision(value, base.SystemAsOf); !errors.Is(err, ErrRiskPassContradictsBlock) {
			t.Fatalf("block status %s should prevent pass, got %v", status, err)
		}
	}
}

func TestValidateRiskDecisionPreservesRejectAndUnknown(t *testing.T) {
	base := validRiskDecision()
	base.Status = "reject"
	base.Reasons[0].Status = "triggered"
	if err := ValidateRiskDecision(base, base.SystemAsOf); err != nil {
		t.Fatalf("reject should be preserved: %v", err)
	}
	base.Status = "unknown"
	base.Reasons[0].Status = "unknown"
	if err := ValidateRiskDecision(base, base.SystemAsOf); err != nil {
		t.Fatalf("unknown should be preserved: %v", err)
	}
}

func TestValidateRiskDecisionRequiresSameRunSystemAsOf(t *testing.T) {
	base := validRiskDecision()
	if err := ValidateRiskDecision(base, base.SystemAsOf.Add(time.Second)); !errors.Is(err, ErrInvalidRiskDecision) {
		t.Fatalf("as-of mismatch should fail, got %v", err)
	}
}

func TestValidateRiskDecisionRejectsDuplicateReasonCodes(t *testing.T) {
	base := validRiskDecision()
	duplicate := base.Reasons[0]
	duplicate.SafeMessage = "重复规则代码"
	base.Reasons = append(base.Reasons, duplicate)
	if err := ValidateRiskDecision(base, base.SystemAsOf); !errors.Is(err, ErrInvalidRiskDecision) {
		t.Fatalf("duplicate reason codes must fail closed, got %v", err)
	}
}

func TestValidateRiskDecisionAcceptsDistinctMultipleReasons(t *testing.T) {
	base := validRiskDecision()
	base.Reasons = append(base.Reasons, RiskReason{
		Code: "turnover_spike", Severity: "warn", Status: "not_triggered",
		SafeMessage: "换手率未达到提示阈值", RuleRef: "rule-turnover-v1",
		EvidenceRefs: []string{"ev-2"},
	})
	if err := ValidateRiskDecision(base, base.SystemAsOf); err != nil {
		t.Fatalf("distinct reason codes should be accepted: %v", err)
	}
}


func TestValidateRiskDecisionRejectsNonCanonicalText(t *testing.T) {
	cases := map[string]func(*RiskDecision){
		"decision id padded": func(d *RiskDecision) { d.DecisionID = " risk-1 " },
		"rule set version padded": func(d *RiskDecision) { d.RuleSetVersion = " risk-v1 " },
		"input ref padded": func(d *RiskDecision) { d.InputRefs[0] = " " + d.InputRefs[0] },
		"reason code padded": func(d *RiskDecision) { d.Reasons[0].Code = " max_drawdown " },
		"rule ref padded": func(d *RiskDecision) { d.Reasons[0].RuleRef = " rule-max-drawdown-v1 " },
		"safe message padded": func(d *RiskDecision) { d.Reasons[0].SafeMessage = " 未触发阻断条件 " },
		"evidence ref padded": func(d *RiskDecision) { d.Reasons[0].EvidenceRefs[0] = " ev-1 " },
		"invalid UTF-8 decision id": func(d *RiskDecision) { d.DecisionID = string([]byte{0xff}) },
		"invalid UTF-8 reason code": func(d *RiskDecision) { d.Reasons[0].Code = string([]byte{0xff}) },
		"invalid UTF-8 input ref": func(d *RiskDecision) { d.InputRefs[0] = string([]byte{0xff}) },
		"invalid UTF-8 evidence ref": func(d *RiskDecision) { d.Reasons[0].EvidenceRefs[0] = string([]byte{0xff}) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			value := validRiskDecision()
			mutate(&value)
			if err := ValidateRiskDecision(value, value.SystemAsOf); !errors.Is(err, ErrInvalidRiskDecision) {
				t.Fatalf("non-canonical risk text must fail closed, got %v", err)
			}
		})
	}
}

func TestValidateRiskDecisionRejectsEmptyReasons(t *testing.T) {
	base := validRiskDecision()
	base.Reasons = []RiskReason{}
	if err := ValidateRiskDecision(base, base.SystemAsOf); !errors.Is(err, ErrInvalidRiskDecision) {
		t.Fatalf("empty risk reasons must fail closed, got %v", err)
	}
}

func TestValidateRiskDecisionRejectsEvaluationBeforeSystemAsOf(t *testing.T) {
	base := validRiskDecision()
	base.EvaluatedAt = base.SystemAsOf.Add(-time.Second)
	if err := ValidateRiskDecision(base, base.SystemAsOf); !errors.Is(err, ErrInvalidRiskDecision) {
		t.Fatalf("evaluation before the PIT must fail, got %v", err)
	}
}
func TestValidateRiskDecisionAcceptsOnlyScalarObservedValues(t *testing.T) {
	base := validRiskDecision()
	for _, raw := range []string{`"10.25"`, "10.25", "null"} {
		value := base
		value.Reasons = append([]RiskReason(nil), base.Reasons...)
		value.Reasons[0].ObservedValue = []byte(raw)
		if err := ValidateRiskDecision(value, base.SystemAsOf); err != nil {
			t.Fatalf("scalar observed value %s should be accepted: %v", raw, err)
		}
	}
	for _, raw := range []string{"true", `{"value":10}`, "[10]", "10 20"} {
		value := base
		value.Reasons = append([]RiskReason(nil), base.Reasons...)
		value.Reasons[0].ObservedValue = []byte(raw)
		if err := ValidateRiskDecision(value, base.SystemAsOf); !errors.Is(err, ErrInvalidRiskDecision) {
			t.Fatalf("non-scalar observed value %s should be rejected: %v", raw, err)
		}
	}
}
