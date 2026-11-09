# QNT-010 Risk RuleSet 执行入口

## 目标

在不把具体投资阈值写死到 Quant Domain 的前提下，将版本化 RuleSet 与规则执行器连接起来。规则实现由上层注入；Quant 负责执行顺序、覆盖检查、结果结构校验和 fail-closed 决策构造。

## 接口

- `RuleEvaluator` 接收调用上下文并返回单条 `Reason`。
- `EvaluateRuleSet` 按 RuleSet 声明顺序执行，不依赖 map 遍历顺序。
- 规则执行器必须返回与 RuleDefinition 相同的 code/severity。
- 未注册的 evaluator、未知规则、规则身份/严重级别不匹配均拒绝构造决策。
- 缺少必需规则或必需规则执行失败时，生成 block + unknown；硬门禁不能因此变成 pass。
- 可选规则未注册时跳过；执行失败时保留 unknown 原因，但不改变硬门禁通过条件。
- context 取消会中止评估，不生成部分决策。
- 对外 safe_message 不包含底层 evaluator 的错误文本，避免泄漏内部实现信息。

## 边界

本入口不定义风险阈值、交易制度事实或规则集可信发布机制。具体规则、事实读取、可信规则集加载/审批/签名/发布/回滚，以及运行时授权仍由 Application/Server/Repository 接入。调用方不得将缺少输入或无法评估的必需规则从 RuleSet 删除以绕过硬门禁。

## 验收

测试覆盖声明顺序、必需规则缺失、执行错误 fail-closed、未知 evaluator、返回身份不匹配和 context 取消。该实现提供通用执行骨架，不代表已有真实风险规则或生产规则集。
