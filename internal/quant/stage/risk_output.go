package stage

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"

	"example.com/stock-analysis-flow/internal/quant/risk"
)

// Risk Stage Output 的强类型 DTO 与运行时 Validator。
//
// 这份 Schema 属于 QNT-CTR-003（deferred）的 L2 机器契约，只约束可交易性硬门禁切片。
// 它刻意只允许一条 Reason：规则集 qnt-risk-tradability/1.0.0 只有一条 block 规则；
// 将来扩充规则必须发新的 Stage Output Schema 版本，不能靠"多塞几条理由"静默扩展。
// 2.0.0 将输入证据从 2 项扩为 4 项（交易状态 + 盘口）；3.0.0 再接入 price_limit，
// 扩为 6 项（再加涨跌停池快照与授权）。旧 1.0.0/2.0.0 均保留冻结。
// Quality Summary 1.0.1 只修正描述口径，字段结构不变。
const (
	RiskStageCode            = "quant.risk"
	RiskOutputType           = "quant.risk_result"
	RiskSchemaVersion        = "3.0.0"
	RiskQualitySchemaVersion = "1.0.1"

	// RiskPurposeCode 是风险门禁读取交易事实的用途，必须与 Data 侧允许用途一致。
	RiskPurposeCode = "risk_review"
)

var ErrInvalidRiskStageOutput = errors.New("invalid Quant Risk stage output")

var (
	riskStageResultIDPattern = regexp.MustCompile(`^quant_result:sha256:[0-9a-f]{64}$`)
	riskStageSnapshotPattern = regexp.MustCompile(`^data_snapshot:sha256:[0-9a-f]{64}$`)
)

// riskMissingFactOrder 是门禁缺项的封闭取值，与 risk.TradabilityFact 一一对应。
var riskMissingFactOrder = map[string]struct{}{
	string(risk.TradabilityFactRiskWarning):    {},
	string(risk.TradabilityFactSuspension):     {},
	string(risk.TradabilityFactPriceLimit):     {},
	string(risk.TradabilityFactTradableVolume): {},
}

// riskBlockingFactOrder 是"已核验且不可交易"的事实键封闭取值（Data 侧 key）。
var riskBlockingFactOrder = map[string]struct{}{
	"risk_warning_status": {},
	"suspension_status":   {},
	"delisting_status":    {},
	"is_trading_day":      {},
	"price_limit":         {},
}

// RiskOutput 是风险门禁的 Researchflow Stage payload。
// 内嵌 risk.Decision 使 payload 与 QNT-CTR-004 RiskDecision 字段语义完全一致，
// 额外只补 Stage 层必需的结果身份与机器可读的缺项清单。
type RiskOutput struct {
	SchemaVersion string   `json:"schema_version"`
	ResultID      string   `json:"result_id"`
	ResultStatus  string   `json:"result_status"`
	MissingFacts  []string `json:"missing_facts"`
	BlockingFacts []string `json:"blocking_facts"`
	risk.Decision
}

// RiskQualitySummary 是风险 Stage 的 quality_summary。
//
// quality_status 是输入事实的质量，decision_status 是结论；两者分开表达，
// 避免"输入降级"被读成"结论不确定"，也避免"结论拒绝"被读成"输入有问题"。
type RiskQualitySummary struct {
	SchemaVersion  string   `json:"schema_version"`
	RuleSetVersion string   `json:"rule_set_version"`
	QualityStatus  string   `json:"quality_status"`
	DecisionStatus string   `json:"decision_status"`
	ResultStatus   string   `json:"result_status"`
	MissingFacts   []string `json:"missing_facts"`
	BlockingFacts  []string `json:"blocking_facts"`
}

func ValidateRiskStageOutputJSON(payload []byte) error {
	var output RiskOutput
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil || ensureJSONEOF(decoder) != nil {
		return ErrInvalidRiskStageOutput
	}
	if output.SchemaVersion != RiskSchemaVersion ||
		!riskStageResultIDPattern.MatchString(output.ResultID) ||
		output.RuleSetVersion != risk.TradabilityRuleSetVersion {
		return ErrInvalidRiskStageOutput
	}
	if err := output.Decision.Validate(); err != nil {
		return ErrInvalidRiskStageOutput
	}
	// evaluated_at 必须等于 system_as_of：决策是 PIT 时刻的判断，Schema 无法表达相等，
	// 由运行时校验承担；否则同一份事实会因评估时刻不同而产生不同 result_id。
	if output.EvaluatedAt != output.SystemAsOf {
		return ErrInvalidRiskStageOutput
	}
	if len(output.InputRefs) != 6 || !riskStageSnapshotPattern.MatchString(output.InputRefs[0]) ||
		!factorStageOpaqueRef(output.InputRefs[1]) || !riskStageSnapshotPattern.MatchString(output.InputRefs[2]) ||
		!factorStageOpaqueRef(output.InputRefs[3]) || !riskStageSnapshotPattern.MatchString(output.InputRefs[4]) ||
		!factorStageOpaqueRef(output.InputRefs[5]) {
		return ErrInvalidRiskStageOutput
	}
	if len(output.Reasons) != 1 {
		return ErrInvalidRiskStageOutput
	}
	reason := output.Reasons[0]
	if reason.Code != risk.TradabilityGateCode || reason.Severity != risk.SeverityBlock ||
		len(reason.EvidenceRefs) != len(output.InputRefs) {
		return ErrInvalidRiskStageOutput
	}
	for index := range output.InputRefs {
		if reason.EvidenceRefs[index] != output.InputRefs[index] {
			return ErrInvalidRiskStageOutput
		}
	}
	if err := validateRiskFactLists(output.MissingFacts, output.BlockingFacts); err != nil {
		return err
	}
	return validateRiskDecisionConsistency(output)
}

// validateRiskDecisionConsistency 固定 status / reason.status / result_status / 缺项清单的联合映射。
// reject 必须至少有一个缺项或阻断事实：Verified 为 false 的两种成因都会体现在这两个清单里，
// 两者同时为空却拒绝，说明适配层篡改了结论。
func validateRiskDecisionConsistency(output RiskOutput) error {
	reasonStatus := output.Reasons[0].Status
	switch output.Status {
	case risk.DecisionReject:
		if output.ResultStatus != "rejected" || reasonStatus != risk.ReasonTriggered ||
			len(output.MissingFacts)+len(output.BlockingFacts) == 0 {
			return ErrInvalidRiskStageOutput
		}
	case risk.DecisionPass:
		if output.ResultStatus != "completed" || reasonStatus != risk.ReasonNotTriggered ||
			len(output.MissingFacts) != 0 || len(output.BlockingFacts) != 0 {
			return ErrInvalidRiskStageOutput
		}
	case risk.DecisionUnknown:
		if output.ResultStatus != "failed" || reasonStatus != risk.ReasonUnknown {
			return ErrInvalidRiskStageOutput
		}
	default:
		return ErrInvalidRiskStageOutput
	}
	return nil
}

func validateRiskFactLists(missing, blocking []string) error {
	for _, value := range missing {
		if _, ok := riskMissingFactOrder[value]; !ok {
			return ErrInvalidRiskStageOutput
		}
	}
	for _, value := range blocking {
		if _, ok := riskBlockingFactOrder[value]; !ok {
			return ErrInvalidRiskStageOutput
		}
	}
	if hasDuplicateRiskFact(missing) || hasDuplicateRiskFact(blocking) {
		return ErrInvalidRiskStageOutput
	}
	return nil
}

func hasDuplicateRiskFact(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func ValidateRiskQualitySummaryJSON(payload []byte) error {
	var summary RiskQualitySummary
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&summary); err != nil || ensureJSONEOF(decoder) != nil {
		return ErrInvalidRiskStageOutput
	}
	if summary.SchemaVersion != RiskQualitySchemaVersion ||
		summary.RuleSetVersion != risk.TradabilityRuleSetVersion ||
		(summary.QualityStatus != "valid" && summary.QualityStatus != "degraded") {
		return ErrInvalidRiskStageOutput
	}
	if err := validateRiskFactLists(summary.MissingFacts, summary.BlockingFacts); err != nil {
		return err
	}
	switch summary.DecisionStatus {
	case string(risk.DecisionPass):
		if summary.ResultStatus != "completed" ||
			len(summary.MissingFacts) != 0 || len(summary.BlockingFacts) != 0 {
			return ErrInvalidRiskStageOutput
		}
	case string(risk.DecisionReject):
		if summary.ResultStatus != "rejected" ||
			len(summary.MissingFacts)+len(summary.BlockingFacts) == 0 {
			return ErrInvalidRiskStageOutput
		}
	case string(risk.DecisionUnknown):
		if summary.ResultStatus != "failed" {
			return ErrInvalidRiskStageOutput
		}
	default:
		return ErrInvalidRiskStageOutput
	}
	return nil
}

// riskFactNames 把门禁事实枚举转为 Stage payload 的字符串清单，保持输入顺序。
func riskFactNames(facts []risk.TradabilityFact) []string {
	names := make([]string, 0, len(facts))
	for _, fact := range facts {
		names = append(names, string(fact))
	}
	return names
}
