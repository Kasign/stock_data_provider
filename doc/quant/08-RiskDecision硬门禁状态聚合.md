# RiskDecision 聚合器非法枚举 fail-closed

## 修正内容

RiskDecision DTO 的 Validate 会拒绝未知 severity/status，但聚合器也可能被上游直接调用，不能假定输入一定经过 DTO Validate。

- 未知 severity：聚合结果至少为 unknown，不得被忽略后误判 pass。
- 未知 reason status：无论属于 block 还是 warn/info，都不能产生 pass。
- 已明确触发的 block 仍优先 reject；非法的其他规则不能抹掉已经成立的拒绝证据。
- 原有空规则集、warn/info-only、block unknown 和全部 block not_triggered 语义保持不变。

该修正只加固聚合边界，不代替 DTO Validate，也不代表必需规则完整性已由 Application 保证。
