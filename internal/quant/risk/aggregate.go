package risk

// AggregateStatus 根据已执行规则的结构化结论聚合风险状态。
// 至少需要一条 block 级规则；硬门禁触发优先 reject，其次 unknown，全部明确未触发才 pass。
// 未知严重级别或结论一律按 unknown 处理，避免非法枚举被误判为通过。
// warn/info 不得覆盖硬门禁结论，也不能单独构成 pass 的依据。
func AggregateStatus(reasons []Reason) DecisionStatus {
	hasBlock := false
	hasUnknownBlock := false
	hasTriggeredBlock := false
	hasMalformed := false

	for _, reason := range reasons {
		switch reason.Severity {
		case SeverityBlock:
			hasBlock = true
			switch reason.Status {
			case ReasonTriggered:
				hasTriggeredBlock = true
			case ReasonNotTriggered:
			case ReasonUnknown:
				hasUnknownBlock = true
			default:
				hasMalformed = true
			}
		case SeverityWarn, SeverityInfo:
			if !validReasonStatus(reason.Status) {
				hasMalformed = true
			}
		default:
			hasMalformed = true
		}
	}

	if hasTriggeredBlock {
		return DecisionReject
	}
	if !hasBlock || hasUnknownBlock || hasMalformed {
		return DecisionUnknown
	}
	return DecisionPass
}
