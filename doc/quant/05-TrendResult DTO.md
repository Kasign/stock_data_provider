# TrendResult DTO

## 范围

实现 QNT-CTR-004 的 TrendResult 结构映射，不实现趋势推断算法、强度归一化算法或跨周期冲突识别。相关算法语义由具体 algorithm_version 和后续冻结的用例契约定义。

## 字段

- `trend_code`：趋势类型编码。
- `direction`：bullish、bearish、sideways、unknown。
- `phase`：emerging、accelerating、mature、weakening、reversing、unknown。
- `strength`：可空；非空必须为有限数且位于 [0,1]。
- `period`：趋势周期。
- `evidence_metrics`：通用 QuantMetric 数组，不复用仅覆盖首批 Factor 的 Factor 专用 DTO。
- `conflict_refs`：显式冲突引用数组。
- `quality_status`、`system_as_of`、`algorithm_version`：质量、知识截止时间和算法版本。

## 通用 QuantMetric 边界

通用指标 DTO 只验证公共结构：标识、单位、周期、时间、质量/可用状态、数值 JSON 标量、区间成对、不可用值显式 null。value/bounds 的具体 JSON 类型、精度、舍入、范围、样本口径及 degraded 放行策略必须由 metric_code 对应 Registry Schema/quality_policy 决定；DTO 不从指标编码猜测这些语义。

## 验收边界

- direction 与 phase 独立序列化；unknown 不得被转换为 sideways。
- strength 拒绝 NaN、Inf 和 [0,1] 外数值。
- 必需数组必须显式存在；允许空数组。
- 非法时间、空引用、非法质量状态拒绝。
- Golden 固定 DTO 的 JSON 结构，不代表某个真实趋势算法已完成验收。
