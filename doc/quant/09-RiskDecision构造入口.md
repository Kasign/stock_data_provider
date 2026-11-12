# RiskDecision 构造入口

## 实现

`risk.NewDecision` 从调用方提供的决策身份、规则集版本、评估时间、system_as_of、已执行规则结论和输入引用构造 RiskDecision。

- 不接受调用方传入 status；状态统一由 `AggregateStatus` 计算。
- 构造后执行 DTO `Validate`，校验必需字段、Reason 唯一性、聚合状态和时间先后。
- `evaluated_at` 必须为规范 UTC RFC3339Nano，且不早于 `system_as_of`。
- 深拷贝 Reasons、EvidenceRefs 和 InputRefs，避免调用方复用底层切片后篡改已构造的决策。
- `reasons` 必须至少一项；`input_refs` 必须显式存在但允许空数组。

## 边界

NewDecision 不执行风险规则，也无法单独判断调用方是否漏报了某条必需规则。正式执行应优先使用 `BuildDecisionForRuleSet` / `EvaluateRuleSet`，由版本化 RuleSet 保证规则覆盖率、severity 和 Reasons 顺序；QNT-010 Application 负责加载可信规则集、固定版本和证据授权。
