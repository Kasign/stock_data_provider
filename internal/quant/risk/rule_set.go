package risk

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidRuleSet = errors.New("invalid risk rule set")
	ErrRuleCoverage   = errors.New("risk rule coverage mismatch")
)

// RuleDefinition 声明一个版本化规则集中的规则身份和严重级别。
// Required 表示该规则必须在每次评估中产生结论；必需规则必须是 block。
type RuleDefinition struct {
	Code     string
	Severity Severity
	Required bool
}

// RuleSet 是一次风险评估使用的显式规则清单；Version 必须与 Decision.RuleSetVersion 一致。
type RuleSet struct {
	Version string
	Rules   []RuleDefinition
}

// Validate 校验规则集自身的身份、规则唯一性和必需硬门禁约束。
func (set RuleSet) Validate() error {
	if !nonBlank(set.Version) || len(set.Rules) == 0 {
		return ErrInvalidRuleSet
	}
	seen := make(map[string]struct{}, len(set.Rules))
	for _, rule := range set.Rules {
		if !nonBlank(rule.Code) || !validSeverity(rule.Severity) {
			return ErrInvalidRuleSet
		}
		if _, exists := seen[rule.Code]; exists {
			return ErrInvalidRuleSet
		}
		seen[rule.Code] = struct{}{}
		if rule.Required && rule.Severity != SeverityBlock {
			return ErrInvalidRuleSet
		}
	}
	return nil
}

// BuildDecisionForRuleSet 按固定规则清单构造 RiskDecision。
// 缺失的必需规则自动补为 block + unknown；未知规则、重复结论、严重级别不匹配均拒绝。
// 返回的 Reasons 按规则集声明顺序排列，保证同一输入产生稳定输出顺序。
func BuildDecisionForRuleSet(set RuleSet, input DecisionInput) (Decision, error) {
	if err := set.Validate(); err != nil {
		return Decision{}, err
	}
	if input.RuleSetVersion != set.Version {
		return Decision{}, fmt.Errorf("%w: decision version does not match rule set", ErrInvalidRuleSet)
	}

	provided := make(map[string]Reason, len(input.Reasons))
	for _, reason := range input.Reasons {
		if _, exists := provided[reason.Code]; exists {
			return Decision{}, fmt.Errorf("%w: duplicate rule result %q", ErrRuleCoverage, reason.Code)
		}
		provided[reason.Code] = reason
	}

	reasons := make([]Reason, 0, len(set.Rules))
	for _, rule := range set.Rules {
		reason, exists := provided[rule.Code]
		if !exists {
			if rule.Required {
				reasons = append(reasons, Reason{
					Code: rule.Code, Severity: SeverityBlock, Status: ReasonUnknown,
					SafeMessage: "必需风险规则未产生评估结论", EvidenceRefs: []string{},
				})
			}
			continue
		}
		if reason.Severity != rule.Severity {
			return Decision{}, fmt.Errorf("%w: severity mismatch for rule %q", ErrRuleCoverage, rule.Code)
		}
		reasons = append(reasons, reason)
		delete(provided, rule.Code)
	}
	if len(provided) != 0 {
		return Decision{}, fmt.Errorf("%w: result contains rule outside rule set", ErrRuleCoverage)
	}

	input.Reasons = reasons
	return NewDecision(input)
}
