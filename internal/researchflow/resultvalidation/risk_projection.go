package resultvalidation

import (
	"encoding/json"
	"fmt"
	"time"

	quantresult "example.com/stock-analysis-flow/internal/quant/result"
)

// RiskDecisionFromQuantEnvelope 把 Quant 权威 risk Payload 投影为 Researchflow
// 的组合视图。决策状态、聚合约束、canonical 文本与解释字段均以 Quant 权威
// Decision.Validate 为准；本函数不重新解释风险语义，只做类型投影与时间表示转换。
//
// 该投影是风险结论的唯一来源：Composer 不再接受第二份无来源绑定的 RiskDecision。
func RiskDecisionFromQuantEnvelope(envelope QuantResultEnvelope) (RiskDecision, error) {
	if envelope.ResultType != "risk" {
		return RiskDecision{}, fmt.Errorf("%w: result_type %q is not risk", ErrInvalidRiskDecision, envelope.ResultType)
	}
	payload, err := quantresult.DecodePayload(envelope.ResultType, envelope.PayloadSchemaVersion, json.RawMessage(envelope.Payload))
	if err != nil {
		return RiskDecision{}, fmt.Errorf("%w: decode Quant risk payload: %v", ErrInvalidRiskDecision, err)
	}
	riskPayload, ok := payload.(quantresult.RiskPayload)
	if !ok {
		return RiskDecision{}, fmt.Errorf("%w: Quant payload is not RiskPayload", ErrInvalidRiskDecision)
	}
	decision := riskPayload.Decision()
	// 权威 Validate 覆盖 status 聚合、时间先后、Reason Code 唯一与 canonical 文本。
	if err := decision.Validate(); err != nil {
		return RiskDecision{}, fmt.Errorf("%w: authoritative Quant decision: %v", ErrInvalidRiskDecision, err)
	}
	evaluatedAt, err := time.Parse(time.RFC3339Nano, decision.EvaluatedAt)
	if err != nil {
		return RiskDecision{}, fmt.Errorf("%w: evaluated_at: %v", ErrInvalidRiskDecision, err)
	}
	systemAsOf, err := time.Parse(time.RFC3339Nano, decision.SystemAsOf)
	if err != nil {
		return RiskDecision{}, fmt.Errorf("%w: system_as_of: %v", ErrInvalidRiskDecision, err)
	}
	view := RiskDecision{
		DecisionID:     decision.DecisionID,
		Status:         string(decision.Status),
		RuleSetVersion: decision.RuleSetVersion,
		EvaluatedAt:    evaluatedAt,
		SystemAsOf:     systemAsOf,
		Reasons:        make([]RiskReason, 0, len(decision.Reasons)),
		InputRefs:      append([]string{}, decision.InputRefs...),
	}
	for _, source := range decision.Reasons {
		reason := RiskReason{
			Code:            source.Code,
			Severity:        string(source.Severity),
			Status:          string(source.Status),
			SafeMessage:     source.SafeMessage,
			EvidenceRefs:    append([]string{}, source.EvidenceRefs...),
			RuleRef:         cloneStringPtr(source.RuleRef),
			ObservedValue:   append(json.RawMessage{}, source.ObservedValue...),
			ExpectedRange:   cloneStringPtr(source.ExpectedRange),
			RemediationCode: cloneStringPtr(source.RemediationCode),
		}
		view.Reasons = append(view.Reasons, reason)
	}
	return view, nil
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}
