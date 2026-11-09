package risk

// AggregateStatus 根据已执行规则的结构化结论聚合风险状态。
// 至少需要一条 block 级规则；硬门禁触发优先 reject，其次 unknown，全部明确未触发才 pass。
// warn/info 不得覆盖硬门禁结论，也不能单独构成 pass 的依据。
func AggregateStatus(reasons []Reason) DecisionStatus {
	hasBlock := false
	hasUnknownBlock := false
	hasTriggeredBlock := false

	for _, reason := range reasons {
		if reason.Severity != SeverityBlock {
			continue
		}
		hasBlock = true
		switch reason.Status {
		case ReasonTriggered:
			hasTriggeredBlock = true
		case ReasonUnknown:
			hasUnknownBlock = true
		}
	}

	if hasTriggeredBlock {
		return DecisionReject
	}
	if !hasBlock || hasUnknownBlock {
		return DecisionUnknown
	}
	return DecisionPass
}
