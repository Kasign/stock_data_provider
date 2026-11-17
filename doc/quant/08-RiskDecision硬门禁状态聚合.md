# RiskDecision 聚合器与硬门禁完整性

## 聚合规则

- RuleSet 必须非空，且至少包含一条 `block` 规则。
- 所有 `block` 规则必须 `required=true`；warn/info 规则可以 required 或 optional。
- 任一 block triggered => reject，优先于其他 unknown。
- 没有 block triggered，但至少一个 block unknown => unknown。
- 至少一条 block 规则且全部 block not_triggered => pass；warn/info 不改变聚合结论。
- 没有 block 规则、未知 severity 或非法 status 不得 pass。

## 执行完整性

- 缺失 required 规则按其声明 severity 生成 unknown Reason；缺失 block 门禁必须明确生成 block + unknown。
- RuleSet 外的 Reason、重复 code、severity 不匹配均拒绝。
- Reasons 按 RuleSet 声明顺序输出。
- DTO Validate 再次校验 Reason.code 唯一、Reasons 非空、status 与聚合结果一致，以及时间边界。

该机制只定义执行完整性与聚合，不定义具体风险阈值或投资规则，也不允许评分/LLM 覆盖硬门禁。
