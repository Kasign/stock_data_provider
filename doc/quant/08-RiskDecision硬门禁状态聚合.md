# RiskDecision 硬门禁状态聚合

## 实现

`risk.AggregateStatus` 根据规则执行器已经给出的 Reason 聚合 Decision.status：

- 没有规则，或没有任何 block 级规则：`unknown`，不能空规则集通过。
- 任一 block 规则 `triggered`：`reject`。
- 没有触发的 block 规则，但至少一条 block 规则为 `unknown`：`unknown`。
- 至少存在一条 block 规则，且全部明确 `not_triggered`：`pass`。
- warn/info 规则不能覆盖或降级 block 规则结论。

当同时存在 triggered 与 unknown 的 block 规则时，reject 优先，因为已存在明确拒绝证据。

## 边界

该函数只聚合已执行规则的结果，不定义具体风险规则、不推断缺失规则，也不接收评分或 LLM 覆盖参数。调用方必须确保所有必需硬门禁都被执行；缺失或不可评估的硬门禁必须以 block + unknown 显式传入。具体规则集、版本化配置和输入事实仍由 QNT-010 Application 负责。
