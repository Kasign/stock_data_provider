package risk

import (
	"context"
	"errors"
	"testing"
)

func TestEvaluateRuleSetExecutesInDeclarationOrderAndBuildsDecision(t *testing.T) {
	set := validRuleSet()
	var order []string
	evaluators := map[string]RuleEvaluator{
		"INPUT_QUALITY": func(context.Context) (Reason, error) {
			order = append(order, "INPUT_QUALITY")
			return Reason{Code: "INPUT_QUALITY", Severity: SeverityBlock, Status: ReasonNotTriggered, SafeMessage: "输入质量合格", EvidenceRefs: []string{"snapshot:1"}}, nil
		},
		"TRADING_STATUS": func(context.Context) (Reason, error) {
			order = append(order, "TRADING_STATUS")
			return Reason{Code: "TRADING_STATUS", Severity: SeverityBlock, Status: ReasonNotTriggered, SafeMessage: "交易状态正常", EvidenceRefs: []string{"snapshot:2"}}, nil
		},
	}
	decision, err := EvaluateRuleSet(context.Background(), set, decisionInput(set.Version, nil), evaluators)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != DecisionPass {
		t.Fatalf("status = %q, want pass", decision.Status)
	}
	if len(order) != 2 || order[0] != "INPUT_QUALITY" || order[1] != "TRADING_STATUS" {
		t.Fatalf("evaluation order = %v", order)
	}
}

func TestEvaluateRuleSetUsesImmutableExecutionSnapshot(t *testing.T) {
	set := validRuleSet()
	evaluators := map[string]RuleEvaluator{}
	evaluators["INPUT_QUALITY"] = func(context.Context) (Reason, error) {
		set.Rules[1].Code = "MUTATED_RULE"
		delete(evaluators, "TRADING_STATUS")
		return Reason{Code: "INPUT_QUALITY", Severity: SeverityBlock, Status: ReasonNotTriggered, SafeMessage: "输入质量合格", EvidenceRefs: []string{}}, nil
	}
	evaluators["TRADING_STATUS"] = func(context.Context) (Reason, error) {
		return Reason{Code: "TRADING_STATUS", Severity: SeverityBlock, Status: ReasonNotTriggered, SafeMessage: "交易状态正常", EvidenceRefs: []string{}}, nil
	}
	decision, err := EvaluateRuleSet(context.Background(), set, decisionInput("risk-2026.11.1", nil), evaluators)
	if err != nil {
		t.Fatalf("EvaluateRuleSet() error = %v", err)
	}
	if decision.Status != DecisionPass || len(decision.Reasons) != 2 || decision.Reasons[1].Code != "TRADING_STATUS" {
		t.Fatalf("evaluation did not use its entry snapshot: %+v", decision)
	}
}

func TestEvaluateRuleSetMissingRequiredEvaluatorFailsClosed(t *testing.T) {
	set := validRuleSet()
	evaluators := map[string]RuleEvaluator{
		"INPUT_QUALITY": func(context.Context) (Reason, error) {
			return Reason{Code: "INPUT_QUALITY", Severity: SeverityBlock, Status: ReasonNotTriggered, SafeMessage: "输入质量合格", EvidenceRefs: []string{}}, nil
		},
	}
	decision, err := EvaluateRuleSet(context.Background(), set, decisionInput(set.Version, nil), evaluators)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != DecisionUnknown {
		t.Fatalf("status = %q, want unknown", decision.Status)
	}
	if len(decision.Reasons) != 2 || decision.Reasons[1].Code != "TRADING_STATUS" || decision.Reasons[1].Status != ReasonUnknown {
		t.Fatalf("missing required rule was not represented: %+v", decision.Reasons)
	}
}

func TestEvaluateRuleSetRequiredEvaluatorErrorFailsClosed(t *testing.T) {
	set := validRuleSet()
	evaluators := map[string]RuleEvaluator{
		"INPUT_QUALITY": func(context.Context) (Reason, error) {
			return Reason{}, errors.New("sensitive implementation detail")
		},
		"TRADING_STATUS": func(context.Context) (Reason, error) {
			return Reason{Code: "TRADING_STATUS", Severity: SeverityBlock, Status: ReasonNotTriggered, SafeMessage: "交易状态正常", EvidenceRefs: []string{}}, nil
		},
	}
	decision, err := EvaluateRuleSet(context.Background(), set, decisionInput(set.Version, nil), evaluators)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != DecisionUnknown || decision.Reasons[0].Status != ReasonUnknown {
		t.Fatalf("rule error must fail closed: %+v", decision)
	}
	if decision.Reasons[0].SafeMessage == "sensitive implementation detail" {
		t.Fatal("internal evaluator error leaked into public safe message")
	}
}

func TestEvaluateRuleSetEvaluatorPanicFailsClosed(t *testing.T) {
	set := validRuleSet()
	evaluators := map[string]RuleEvaluator{
		"INPUT_QUALITY": func(context.Context) (Reason, error) {
			panic("sensitive panic detail")
		},
		"TRADING_STATUS": func(context.Context) (Reason, error) {
			return Reason{Code: "TRADING_STATUS", Severity: SeverityBlock, Status: ReasonNotTriggered, SafeMessage: "交易状态正常", EvidenceRefs: []string{}}, nil
		},
	}
	decision, err := EvaluateRuleSet(context.Background(), set, decisionInput(set.Version, nil), evaluators)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != DecisionUnknown || decision.Reasons[0].Status != ReasonUnknown {
		t.Fatalf("required evaluator panic must fail closed: %+v", decision)
	}
	if decision.Reasons[0].SafeMessage != "风险规则执行失败，无法确认结论" {
		t.Fatalf("unexpected panic diagnostic: %q", decision.Reasons[0].SafeMessage)
	}
}

func TestEvaluateRuleSetOptionalEvaluatorPanicIsDiagnosticOnly(t *testing.T) {
	set := validRuleSet()
	evaluators := map[string]RuleEvaluator{
		"INPUT_QUALITY": func(context.Context) (Reason, error) {
			return Reason{Code: "INPUT_QUALITY", Severity: SeverityBlock, Status: ReasonNotTriggered, SafeMessage: "输入质量合格", EvidenceRefs: []string{}}, nil
		},
		"TRADING_STATUS": func(context.Context) (Reason, error) {
			return Reason{Code: "TRADING_STATUS", Severity: SeverityBlock, Status: ReasonNotTriggered, SafeMessage: "交易状态正常", EvidenceRefs: []string{}}, nil
		},
		"CONCENTRATION": func(context.Context) (Reason, error) {
			panic("optional panic detail")
		},
	}
	decision, err := EvaluateRuleSet(context.Background(), set, decisionInput(set.Version, nil), evaluators)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != DecisionPass || len(decision.Reasons) != 3 {
		t.Fatalf("optional evaluator panic must remain diagnostic-only: %+v", decision)
	}
	if decision.Reasons[2].Status != ReasonUnknown || decision.Reasons[2].SafeMessage != "风险规则执行失败，无法确认结论" {
		t.Fatalf("optional panic diagnostic is invalid: %+v", decision.Reasons[2])
	}
}

func TestEvaluateRuleSetRejectsUnknownEvaluatorAndMismatchedResult(t *testing.T) {
	set := validRuleSet()
	unknown := map[string]RuleEvaluator{"NOT_REGISTERED": func(context.Context) (Reason, error) { return Reason{}, nil }}
	if _, err := EvaluateRuleSet(context.Background(), set, decisionInput(set.Version, nil), unknown); !errors.Is(err, ErrRuleCoverage) {
		t.Fatalf("unknown evaluator error = %v, want ErrRuleCoverage", err)
	}

	mismatch := map[string]RuleEvaluator{
		"INPUT_QUALITY": func(context.Context) (Reason, error) {
			return Reason{Code: "OTHER", Severity: SeverityBlock, Status: ReasonNotTriggered, SafeMessage: "不匹配", EvidenceRefs: []string{}}, nil
		},
	}
	if _, err := EvaluateRuleSet(context.Background(), set, decisionInput(set.Version, nil), mismatch); !errors.Is(err, ErrRuleCoverage) {
		t.Fatalf("mismatched result error = %v, want ErrRuleCoverage", err)
	}
}

func TestEvaluateRuleSetHonorsCancellation(t *testing.T) {
	set := validRuleSet()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := EvaluateRuleSet(ctx, set, decisionInput(set.Version, nil), nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestEvaluateRuleSetOptionalEvaluatorErrorIsDiagnosticOnly(t *testing.T) {
	set := validRuleSet()
	evaluators := map[string]RuleEvaluator{
		"INPUT_QUALITY": func(context.Context) (Reason, error) {
			return Reason{Code: "INPUT_QUALITY", Severity: SeverityBlock, Status: ReasonNotTriggered, SafeMessage: "输入质量合格", EvidenceRefs: []string{}}, nil
		},
		"TRADING_STATUS": func(context.Context) (Reason, error) {
			return Reason{Code: "TRADING_STATUS", Severity: SeverityBlock, Status: ReasonNotTriggered, SafeMessage: "交易状态正常", EvidenceRefs: []string{}}, nil
		},
		"CONCENTRATION": func(context.Context) (Reason, error) {
			return Reason{}, errors.New("optional evaluator unavailable")
		},
	}
	decision, err := EvaluateRuleSet(context.Background(), set, decisionInput(set.Version, nil), evaluators)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != DecisionPass {
		t.Fatalf("optional evaluator failure changed hard-gate status: %q", decision.Status)
	}
	if len(decision.Reasons) != 3 {
		t.Fatalf("reasons = %d, want 3 including optional diagnostic", len(decision.Reasons))
	}
	optional := decision.Reasons[2]
	if optional.Code != "CONCENTRATION" || optional.Severity != SeverityWarn ||
		optional.Status != ReasonUnknown || optional.SafeMessage == "optional evaluator unavailable" {
		t.Fatalf("optional failure diagnostic is invalid or leaked internal error: %+v", optional)
	}
}

func TestEvaluateRuleSetStopsWhenContextIsCancelledDuringEvaluation(t *testing.T) {
	set := validRuleSet()
	ctx, cancel := context.WithCancel(context.Background())
	evaluators := map[string]RuleEvaluator{
		"INPUT_QUALITY": func(context.Context) (Reason, error) {
			cancel()
			return Reason{Code: "INPUT_QUALITY", Severity: SeverityBlock, Status: ReasonNotTriggered, SafeMessage: "输入质量合格", EvidenceRefs: []string{}}, nil
		},
	}
	if _, err := EvaluateRuleSet(ctx, set, decisionInput(set.Version, nil), evaluators); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
