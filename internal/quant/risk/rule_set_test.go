package risk

import (
	"errors"
	"testing"
)

func validRuleSet() RuleSet {
	return RuleSet{
		Version: "risk-2026.11.1",
		Rules: []RuleDefinition{
			{Code: "INPUT_QUALITY", Severity: SeverityBlock, Required: true},
			{Code: "TRADING_STATUS", Severity: SeverityBlock, Required: true},
			{Code: "CONCENTRATION", Severity: SeverityWarn},
			{Code: "INFO_NOTE", Severity: SeverityInfo},
		},
	}
}

func decisionInput(version string, reasons []Reason) DecisionInput {
	return DecisionInput{
		DecisionID: "risk-decision-ruleset-001", RuleSetVersion: version,
		EvaluatedAt: "2026-11-09T01:00:00Z", SystemAsOf: "2026-11-09T00:59:00Z",
		Reasons: reasons, InputRefs: []string{"quant_result:ruleset:001"},
	}
}

func TestBuildDecisionForRuleSetPassRequiresEveryRequiredRule(t *testing.T) {
	set := validRuleSet()
	input := decisionInput(set.Version, []Reason{
		{Code: "TRADING_STATUS", Severity: SeverityBlock, Status: ReasonNotTriggered, SafeMessage: "交易状态正常", EvidenceRefs: []string{}},
		{Code: "INPUT_QUALITY", Severity: SeverityBlock, Status: ReasonNotTriggered, SafeMessage: "输入质量合格", EvidenceRefs: []string{}},
	})
	decision, err := BuildDecisionForRuleSet(set, input)
	if err != nil {
		t.Fatalf("BuildDecisionForRuleSet() error = %v", err)
	}
	if decision.Status != DecisionPass {
		t.Fatalf("status = %q, want pass", decision.Status)
	}
	if len(decision.Reasons) != 2 || decision.Reasons[0].Code != "INPUT_QUALITY" ||
		decision.Reasons[1].Code != "TRADING_STATUS" {
		t.Fatalf("reasons are not in rule-set order: %+v", decision.Reasons)
	}
}

func TestBuildDecisionForRuleSetMissingRequiredRuleFailsClosed(t *testing.T) {
	set := validRuleSet()
	input := decisionInput(set.Version, []Reason{
		{Code: "INPUT_QUALITY", Severity: SeverityBlock, Status: ReasonNotTriggered, SafeMessage: "输入质量合格", EvidenceRefs: []string{}},
	})
	decision, err := BuildDecisionForRuleSet(set, input)
	if err != nil {
		t.Fatalf("BuildDecisionForRuleSet() error = %v", err)
	}
	if decision.Status != DecisionUnknown {
		t.Fatalf("status = %q, want unknown", decision.Status)
	}
	missing := decision.Reasons[1]
	if missing.Code != "TRADING_STATUS" || missing.Severity != SeverityBlock ||
		missing.Status != ReasonUnknown || len(missing.EvidenceRefs) != 0 {
		t.Fatalf("missing required rule not represented as block+unknown: %+v", missing)
	}
}

func TestBuildDecisionForRuleSetRejectsUnknownDuplicateAndMismatchedResults(t *testing.T) {
	set := validRuleSet()
	base := Reason{Code: "INPUT_QUALITY", Severity: SeverityBlock, Status: ReasonNotTriggered, SafeMessage: "输入质量合格", EvidenceRefs: []string{}}
	tests := []struct {
		name    string
		reasons []Reason
	}{
		{"unknown rule", []Reason{{Code: "UNREGISTERED", Severity: SeverityBlock, Status: ReasonNotTriggered, SafeMessage: "未知", EvidenceRefs: []string{}}}},
		{"duplicate result", []Reason{base, base}},
		{"severity mismatch", []Reason{{Code: "INPUT_QUALITY", Severity: SeverityWarn, Status: ReasonNotTriggered, SafeMessage: "输入质量合格", EvidenceRefs: []string{}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := BuildDecisionForRuleSet(set, decisionInput(set.Version, tt.reasons)); !errors.Is(err, ErrRuleCoverage) {
				t.Fatalf("error = %v, want ErrRuleCoverage", err)
			}
		})
	}
}

func TestBuildDecisionForRuleSetRejectsVersionMismatch(t *testing.T) {
	set := validRuleSet()
	if _, err := BuildDecisionForRuleSet(set, decisionInput("risk-2026.11.2", nil)); !errors.Is(err, ErrInvalidRuleSet) {
		t.Fatalf("error = %v, want ErrInvalidRuleSet", err)
	}
}

func TestRuleSetRejectsDuplicateAndNonBlockRequiredRules(t *testing.T) {
	tests := []RuleSet{
		{Version: "1.0.0", Rules: []RuleDefinition{{Code: "A", Severity: SeverityBlock}, {Code: "A", Severity: SeverityBlock}}},
		{Version: "1.0.0", Rules: []RuleDefinition{{Code: "A", Severity: SeverityWarn, Required: true}}},
		{Version: "1.0.0", Rules: []RuleDefinition{{Code: "A", Severity: "critical"}}},
		{Version: "1.0.0"},
	}
	for i, set := range tests {
		if err := set.Validate(); !errors.Is(err, ErrInvalidRuleSet) {
			t.Errorf("case %d: error = %v, want ErrInvalidRuleSet", i, err)
		}
	}
}

func TestBuildDecisionForRuleSetAllowsMissingOptionalRule(t *testing.T) {
	set := validRuleSet()
	input := decisionInput(set.Version, []Reason{
		{Code: "INPUT_QUALITY", Severity: SeverityBlock, Status: ReasonNotTriggered, SafeMessage: "输入质量合格", EvidenceRefs: []string{}},
		{Code: "TRADING_STATUS", Severity: SeverityBlock, Status: ReasonNotTriggered, SafeMessage: "交易状态正常", EvidenceRefs: []string{}},
	})
	decision, err := BuildDecisionForRuleSet(set, input)
	if err != nil {
		t.Fatalf("BuildDecisionForRuleSet() error = %v", err)
	}
	if decision.Status != DecisionPass || len(decision.Reasons) != 2 {
		t.Fatalf("optional rules should not block: %+v", decision)
	}
}

func TestRuleSetRejectsNonCanonicalIdentities(t *testing.T) {
	tests := []RuleSet{
		{Version: " risk-1", Rules: []RuleDefinition{{Code: "A", Severity: SeverityBlock}}},
		{Version: "risk-1 ", Rules: []RuleDefinition{{Code: "A", Severity: SeverityBlock}}},
		{Version: "risk-1", Rules: []RuleDefinition{{Code: " A", Severity: SeverityBlock}}},
		{Version: "risk-1", Rules: []RuleDefinition{{Code: "A ", Severity: SeverityBlock}}},
		{Version: "risk-1", Rules: []RuleDefinition{{Code: string([]byte{0xff}), Severity: SeverityBlock}}},
		{Version: string([]byte{0xff}), Rules: []RuleDefinition{{Code: "A", Severity: SeverityBlock}}},
	}
	for i, set := range tests {
		if err := set.Validate(); !errors.Is(err, ErrInvalidRuleSet) {
			t.Errorf("case %d: Validate() error = %v, want ErrInvalidRuleSet", i, err)
		}
	}
}
