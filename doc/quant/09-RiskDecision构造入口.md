# RiskDecision 构造入口

## 实现

新增 `risk.NewDecision`，从调用方提供的决策身份、规则集版本、评估时间、system_as_of、已执行规则结论和输入引用构造 RiskDecision。

- 不接受调用方传入 status；状态统一由 `AggregateStatus` 计算。
- 构造后立即执行 DTO `Validate`，非法时间、空身份、未知枚举或缺失数组直接返回错误。
- 深拷贝 Reasons、EvidenceRefs 和 InputRefs，避免调用方复用底层切片后篡改已构造的决策。
- 空规则集仍构造为 `unknown`，不会默认通过。

## 边界

NewDecision 不执行风险规则，也无法判断必需规则是否全部提交。QNT-010 Application 必须负责规则集加载、版本固定、规则完整性和证据授权；该构造器只保证状态聚合和 DTO 结构一致。
