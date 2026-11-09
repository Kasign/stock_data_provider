# Factor 质量策略执行入口

## 目标

将 CalculationContext 中已声明的质量策略真正应用到 Factor QuantMetric 生成环节，避免调用方只校验上下文却绕过 strict / allow_degraded 的语义。

## 规则

- `NewQuantMetricWithContext` 先校验共享 CalculationContext，再使用上下文中的 `SystemAsOf` 生成 QuantMetric。
- `strict` 策略拒绝 `degraded` 质量的可用指标，返回 `ErrQualityPolicyRejected`。
- `allow_degraded` 策略允许 degraded 指标，但必须原样保留 `quality_status=degraded`，不将其提升为 valid。
- `insufficient_data` 仍以显式空值和不可用状态表达；即使质量为 unknown，也不得伪装成可用指标。
- `invalid` / `unknown` 质量不得生成 available 指标，沿用 `NewQuantMetric` 的既有约束。

## 边界

该入口只执行当前契约已存在的质量策略枚举，不推断停牌、缺失交易日、行情新鲜度或 Provider 质量。上述事实必须由 Data Adapter 提供可审计的质量结论。
