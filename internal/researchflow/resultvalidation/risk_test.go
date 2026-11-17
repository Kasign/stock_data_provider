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
		InputRefs: []string{"qres-1"},
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
