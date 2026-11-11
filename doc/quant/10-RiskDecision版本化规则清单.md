# RiskDecision 版本化规则清单与完整性门禁

## 目标

解决仅凭传入 Reasons 聚合、无法判断必需硬门禁是否漏执行的问题。规则清单由 QNT-010 Application 加载并传入；本模块不内置具体投资/交易规则。

## 规则清单

`RuleSet` 包含明确版本和有序规则定义。每条规则声明唯一 code、severity 和 required：

- required 规则必须是 block；反向约束同样成立：所有 `block` 级硬门禁必须声明为 required，禁止 `block + optional`。否则缺少 evaluator 时会被静默跳过，存在漏执行后仍得到 pass 的风险。
- 同一规则集内 code 唯一。
- 规则集版本非空，且至少包含一条规则。
- 规则清单顺序作为 RiskDecision Reasons 的稳定排序依据。
- Version 与 Code 必须是有效 UTF-8、非空且不含首尾空白；不擅自增加长度或字符集限制。

## 执行快照与不可变性

`EvaluateRuleSet` 在入口复制 RuleSet.Rules 和 evaluator 注册表；本次执行只使用入口快照。
规则回调即使持有调用方原始 slice/map 的引用并修改它们，也不能改变当前执行中的规则身份、顺序或 evaluator 注册关系。
调用方仍不得在函数入口复制期间并发写入传入的 slice/map；跨 goroutine 共享配置应由上层使用不可变对象或同步保护。

## 构造与完整性校验

`BuildDecisionForRuleSet` 在构造决策前：

1. 校验规则集，并要求 DecisionInput.RuleSetVersion 与规则集版本完全一致。
2. 拒绝规则集外的评估结果、重复结果和 severity 不匹配。
3. 对缺失的 required 规则自动生成 block + unknown，不允许缺失规则被忽略后得到 pass。
4. 仅 `warn/info + optional` 允许缺失时跳过；`block` 规则必须 required，不能通过配置为 optional 绕过硬门禁。
5. 按规则清单顺序排列结果，再由 NewDecision 聚合状态并执行 DTO Validate。

## 边界

当前只提供版本化规则清单结构与结果覆盖率门禁，不定义具体风险规则、阈值、数据来源或策略。规则集如何存储、审批、签名、发布和回滚，以及如何从可信来源加载，仍由 Application/Server 和后续 Repository 设计负责。调用方不得通过临时构造规则集来绕过正式规则配置治理。
