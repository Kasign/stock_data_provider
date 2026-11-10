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
