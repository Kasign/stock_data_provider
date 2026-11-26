package risk

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidTradabilityFacts 表示可交易性事实本身不合规（表外事实项、证据引用不合法等）。
// 这类错误必须向上传播为规则执行失败 → block + unknown，不能被当作"已核验"。
var ErrInvalidTradabilityFacts = errors.New("invalid tradability facts")

const (
	// TradabilityRuleSetVersion 是"可交易性硬门禁"这一规则集切片的版本。
	// 它只覆盖 unverified-tradability-block 一条硬门禁，不是完整业务风险规则目录：
	// 其余规则必须来自权威规则来源，量化引擎不得自行猜测阈值或规则。
	TradabilityRuleSetVersion = "qnt-risk-tradability/1.0.0"

	// TradabilityGateCode 是"可交易性未验证"硬门禁的规则编码。
	// 语义与旧 Analysis 链的 tradabilityChecker 等价：拿不到可信的
	// ST/停牌/涨跌停/可成交量事实时，不得放行。
	TradabilityGateCode = "tradability_unverified"
)

// TradabilityFact 是可交易性判定所需的单项事实。取值封闭，新增事实必须同步标签表，
// 否则评估会 fail-closed，避免"新事实被静默忽略后仍判为已验证"。
type TradabilityFact string

const (
	TradabilityFactRiskWarning    TradabilityFact = "risk_warning_status"
	TradabilityFactSuspension     TradabilityFact = "suspension_status"
	TradabilityFactPriceLimit     TradabilityFact = "price_limit"
	TradabilityFactTradableVolume TradabilityFact = "tradable_volume"
)

// TradabilityFacts 是本次判定可用的可交易性事实快照。
// Verified 表示四类事实均已核验通过；EvidenceRefs 必须指向可审计的证据
// （例如授权快照引用），否则视为未验证。
type TradabilityFacts struct {
	Verified     bool
	EvidenceRefs []string
	MissingFacts []TradabilityFact
}

// TradabilityGateInput 是评估可交易性门禁的输入。
// 刻意不包含 Status 与 RuleSetVersion：决策状态只能由规则结论聚合得出，
// 规则集版本由本包固定，调用方无法指定，因此不存在评分或 LLM 覆盖入口。
type TradabilityGateInput struct {
	DecisionID  string
	EvaluatedAt string
	SystemAsOf  string
	InputRefs   []string
	Facts       TradabilityFacts
}

// TradabilityRuleSet 返回可交易性硬门禁的规则集。
// 单条 block 规则且 Required=true，满足 RuleSet.Validate 的"所有 block 必须 required"。
func TradabilityRuleSet() RuleSet {
	return RuleSet{
		Version: TradabilityRuleSetVersion,
		Rules: []RuleDefinition{
			{Code: TradabilityGateCode, Severity: SeverityBlock, Required: true},
		},
	}
}

// EvaluateTradabilityGate 按固定规则集评估可交易性门禁并构造正式 RiskDecision。
// 事实未验证 → block + triggered → reject；事实声明已验证但缺证据或含表外事实 →
// 规则执行失败 → block + unknown → unknown。两条路径都不会产生 pass。
func EvaluateTradabilityGate(ctx context.Context, input TradabilityGateInput) (Decision, error) {
	return EvaluateRuleSet(ctx, TradabilityRuleSet(), DecisionInput{
		DecisionID:     input.DecisionID,
		RuleSetVersion: TradabilityRuleSetVersion,
		EvaluatedAt:    input.EvaluatedAt,
		SystemAsOf:     input.SystemAsOf,
		InputRefs:      input.InputRefs,
	}, map[string]RuleEvaluator{
		TradabilityGateCode: TradabilityGate(input.Facts),
	})
}

// TradabilityGate 返回执行可交易性门禁的规则评估器。
// 它只依据传入事实产生结论，不读取任何评分、模型输出或调用方声称的状态。
func TradabilityGate(facts TradabilityFacts) RuleEvaluator {
	verified := facts.Verified
	evidenceRefs := append([]string{}, facts.EvidenceRefs...)
	missingFacts := append([]TradabilityFact{}, facts.MissingFacts...)

	return func(ctx context.Context) (Reason, error) {
		if err := ctx.Err(); err != nil {
			return Reason{}, err
		}
		missingLabels, err := tradabilityFactLabels(missingFacts)
		if err != nil {
			return Reason{}, err
		}
		if verified {
			if err := validateTradabilityEvidence(evidenceRefs); err != nil {
				return Reason{}, err
			}
			return Reason{
				Code:         TradabilityGateCode,
				Severity:     SeverityBlock,
				Status:       ReasonNotTriggered,
				SafeMessage:  "可交易性事实已核验（ST/停牌/涨跌停/可成交量）",
				EvidenceRefs: append([]string{}, evidenceRefs...),
			}, nil
		}
		message := "未取得可信的 ST/停牌/涨跌停/可成交量事实，无法确认可交易性"
		if len(missingLabels) > 0 {
			message = "可交易性事实不完整，缺失：" + strings.Join(missingLabels, "、") + "；无法确认可交易性"
		}
		return Reason{
			Code:         TradabilityGateCode,
			Severity:     SeverityBlock,
			Status:       ReasonTriggered,
			SafeMessage:  message,
			EvidenceRefs: append([]string{}, evidenceRefs...),
		}, nil
	}
}

// validateTradabilityEvidence 要求"已验证"必须给出至少一条非空证据引用。
// 缺少证据时返回错误，由 EvaluateRuleSet 转为 block + unknown，而不是放行。
func validateTradabilityEvidence(refs []string) error {
	if len(refs) == 0 {
		return fmt.Errorf("%w: 声明已验证但未提供证据引用", ErrInvalidTradabilityFacts)
	}
	for _, ref := range refs {
		if !canonicalIdentity(ref) {
			return fmt.Errorf("%w: 证据引用必须是非空、无首尾空白的有效 UTF-8", ErrInvalidTradabilityFacts)
		}
	}
	return nil
}

// tradabilityFactLabels 把封闭枚举转为安全消息标签；表外事实直接报错，
// 避免未来新增事实后仍被静默忽略。
func tradabilityFactLabels(facts []TradabilityFact) ([]string, error) {
	labels := make([]string, 0, len(facts))
	seen := make(map[TradabilityFact]struct{}, len(facts))
	for _, fact := range facts {
		if _, exists := seen[fact]; exists {
			continue
		}
		seen[fact] = struct{}{}
		label, ok := tradabilityFactLabel(fact)
		if !ok {
			return nil, fmt.Errorf("%w: 未知可交易性事实 %q", ErrInvalidTradabilityFacts, string(fact))
		}
		labels = append(labels, label)
	}
	return labels, nil
}

func tradabilityFactLabel(fact TradabilityFact) (string, bool) {
	switch fact {
	case TradabilityFactRiskWarning:
		return "风险警示状态（ST）", true
	case TradabilityFactSuspension:
		return "停牌状态", true
	case TradabilityFactPriceLimit:
		return "涨跌停状态", true
	case TradabilityFactTradableVolume:
		return "可成交量", true
	default:
		return "", false
	}
}
