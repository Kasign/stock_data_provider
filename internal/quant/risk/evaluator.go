package risk

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrInvalidRuleSet = errors.New("invalid risk rule set")
	ErrRuleCoverage   = errors.New("risk rule evaluation coverage mismatch")
	ErrRuleEvaluation = errors.New("risk rule evaluation failed")
)

// RuleEvaluator 执行单条规则。业务规则实现由上层注入，Quant 不内置未经确认的风险阈值。
type RuleEvaluator func(context.Context) (Reason, error)

// EvaluateRuleSet 按 RuleSet 声明顺序执行可用规则并构造决策。
// 缺少必需规则或必需规则执行失败时，生成 block + unknown；不会生成 pass。
// 可选规则缺失时跳过，执行失败时保留 unknown 诊断，但不改变硬门禁语义。
// 规则实现必须返回与声明 code/severity 一致的结构化结论。
func EvaluateRuleSet(
	ctx context.Context,
	set RuleSet,
	input DecisionInput,
	evaluators map[string]RuleEvaluator,
) (Decision, error) {
	if ctx == nil {
		return Decision{}, fmt.Errorf("%w: nil context", ErrRuleEvaluation)
	}
	if err := set.Validate(); err != nil {
		return Decision{}, err
	}
	if input.RuleSetVersion != set.Version {
		return Decision{}, fmt.Errorf("%w: decision version does not match rule set", ErrInvalidRuleSet)
	}

	known := make(map[string]RuleDefinition, len(set.Rules))
	for _, rule := range set.Rules {
		known[rule.Code] = rule
	}
	for code, evaluator := range evaluators {
		if !canonicalIdentity(code) || evaluator == nil {
			return Decision{}, fmt.Errorf("%w: invalid evaluator registration", ErrRuleCoverage)
		}
		if _, exists := known[code]; !exists {
			return Decision{}, fmt.Errorf("%w: evaluator for unknown rule %q", ErrRuleCoverage, code)
		}
	}

	reasons := make([]Reason, 0, len(set.Rules))
	for _, rule := range set.Rules {
		if err := ctx.Err(); err != nil {
			return Decision{}, err
		}
		evaluator, exists := evaluators[rule.Code]
		if !exists {
			if rule.Required {
				reasons = append(reasons, unknownRuleReason(rule))
			}
			continue
		}

		reason, err := evaluator(ctx)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Decision{}, ctxErr
		}
		if err != nil {
			reason = unknownRuleReason(rule)
			reason.SafeMessage = "风险规则执行失败，无法确认结论"
			reasons = append(reasons, reason)
			continue
		}
		if reason.Code != rule.Code || reason.Severity != rule.Severity ||
			!validReasonStatus(reason.Status) || reason.SafeMessage == "" || reason.EvidenceRefs == nil {
			return Decision{}, fmt.Errorf("%w: invalid result for rule %q", ErrRuleCoverage, rule.Code)
		}
		reasons = append(reasons, reason)
	}

	input.Reasons = reasons
	decision, err := NewDecision(input)
	if err != nil {
		return Decision{}, err
	}
	return decision, nil
}

func unknownRuleReason(rule RuleDefinition) Reason {
	return Reason{
		Code: rule.Code, Severity: rule.Severity, Status: ReasonUnknown,
		SafeMessage: "风险规则未产生可用评估结论", EvidenceRefs: []string{},
	}
}
