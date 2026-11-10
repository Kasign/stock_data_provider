package risk

// DecisionInput 是构造 RiskDecision 所需的调用方事实；Status 不允许由调用方指定。
type DecisionInput struct {
	DecisionID     string
	RuleSetVersion string
	EvaluatedAt    string
	SystemAsOf     string
	Reasons        []Reason
	InputRefs      []string
}

// NewDecision 根据已执行规则的结论构造 RiskDecision。
// 决策状态只能由 AggregateStatus 推导；输入切片会被复制，避免调用方后续修改已构造结果。
func NewDecision(input DecisionInput) (Decision, error) {
	reasons := cloneReasons(input.Reasons)
	refs := append([]string(nil), input.InputRefs...)
	if input.Reasons != nil && reasons == nil {
		reasons = []Reason{}
	}
	if input.InputRefs != nil && refs == nil {
		refs = []string{}
	}

	decision := Decision{
		DecisionID:     input.DecisionID,
		Status:         AggregateStatus(reasons),
		RuleSetVersion: input.RuleSetVersion,
		EvaluatedAt:    input.EvaluatedAt,
		SystemAsOf:     input.SystemAsOf,
		Reasons:        reasons,
		InputRefs:      refs,
	}
	if err := decision.Validate(); err != nil {
		return Decision{}, err
	}
	return decision, nil
}

func cloneReasons(source []Reason) []Reason {
	if source == nil {
		return nil
	}
	result := make([]Reason, len(source))
	for i, reason := range source {
		result[i] = reason
		if reason.EvidenceRefs != nil {
			result[i].EvidenceRefs = append([]string{}, reason.EvidenceRefs...)
		}
	}
	return result
}
