# RiskDecision 版本化规则清单与完整性门禁

## 目标

解决仅凭传入 Reasons 聚合、无法判断必需硬门禁是否漏执行的问题。规则清单由 QNT-010 Application 加载并传入；本模块不内置具体投资/交易规则。

## 规则清单

`RuleSet` 包含明确版本和有序规则定义。每条规则声明唯一 code、severity 和 required：

- required 规则必须是 block。
- 同一规则集内 code 唯一。
- 规则集版本非空，且至少包含一条规则。
- 规则清单顺序作为 RiskDecision Reasons 的稳定排序依据。
- Version 与 Code 必须是有效 UTF-8、非空且不含首尾空白；不擅自增加长度或字符集限制。

## 构造与完整性校验

`BuildDecisionForRuleSet` 在构造决策前：

1. 校验规则集，并要求 DecisionInput.RuleSetVersion 与规则集版本完全一致。
2. 拒绝规则集外的评估结果、重复结果和 severity 不匹配。
3. 对缺失的 required 规则自动生成 block + unknown，不允许缺失规则被忽略后得到 pass。
4. 缺失的 optional 规则不生成结果；其是否应为 required 由规则集版本定义。
5. 按规则清单顺序排列结果，再由 NewDecision 聚合状态并执行 DTO Validate。

## 边界

当前只提供版本化规则清单结构与结果覆盖率门禁，不定义具体风险规则、阈值、数据来源或策略。规则集如何存储、审批、签名、发布和回滚，以及如何从可信来源加载，仍由 Application/Server 和后续 Repository 设计负责。调用方不得通过临时构造规则集来绕过正式规则配置治理。
