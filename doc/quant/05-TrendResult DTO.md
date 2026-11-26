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
- TrendResult 与通用 QuantMetric 的自由文本字段及 JSON 标量原文必须是有效 UTF-8，避免 Go JSON 解码/序列化替换非法字节。
- 非法时间、空引用、非法质量状态拒绝；自由文本字段必须是有效 UTF-8 且无首尾空白，避免身份/引用出现多种等价表示。
- Golden 固定 DTO 的 JSON 结构，不代表某个真实趋势算法已完成验收。

## DEC-PLAT-002-049 跨层状态约束补充（2026-11-11）

- TrendResult.direction 与 phase 独立；strength 可空，非空必须有限且位于 [0,1]。
- evidence_metrics/conflict_refs 必须显式提供数组，允许空数组。
- 每个 evidence Metric 必须通过自身 Validate，且 `metric.system_as_of` 必须与 `TrendResult.system_as_of` 完全一致。
- TrendResult 的 period、system_as_of、algorithm_version、quality_status 与 Envelope 对应字段一致的约束，需在完整 Envelope DTO/Validator 落地后完成；当前仓库尚无可供绑定的完整 Envelope 类型，不能提前宣称跨层一致性验收通过。


## 2026-11-12 多周期 Trend 实现前置核验

本轮核对范围：`internal/quant/trend/**`、TrendResult Schema/Golden、项目级 Quant-Research DTO 契约及 Quant 总体设计。结论是 DTO 层已完成，但多周期 Trend 算法不能据现有契约直接编码。

### 已有实现事实
- `internal/quant/trend/trend.go` 只有 `Result` DTO 和 `Validate()`；没有周期输入模型、趋势推断函数、多周期聚合器或冲突生成器。
- Validator 只检查 direction/phase 枚举、strength 有限且位于 [0,1]、必需数组、Metric 自身有效及 system_as_of 一致、文本规范性；注释明确趋势推断/强度归一化/质量策略由上层负责。
- `doc/quant/05-TrendResult DTO.md` 已声明本 DTO 文档不定义趋势推断、强度归一化和跨周期冲突识别。
- 项目级 DTO 契约只规定 direction、phase、strength、period、evidence_metrics、conflict_refs 等输出字段，以及 direction/phase 独立、unknown 不等于 sideways、跨周期冲突显式表达；没有定义算法公式和聚合优先级。
- Quant 总体设计提出短/中/长周期及价格结构、动量、波动、量价、反转观察，但属于能力方向，不足以形成可复算算法规范。

### 编码前必须明确的语义
1. 周期集合及映射：短/中/长分别对应哪些输入窗口/周期标识，周期按交易日还是自然日计算。
2. 输入边界：Trend 消费哪些已注册 Indicator/Factor metric_code，指标窗口、复权基准、交易日连续性和 system_as_of 如何对齐。
3. 单周期推断：direction、phase 的判定条件、优先级和边界；不能从字段枚举反推算法。
4. strength：归一化公式、输入范围、截断/舍入规则；不得把主观权重或任意指标加权伪装为既定规则。
5. 多周期聚合：周期权重是否存在、方向冲突如何判定、是否允许高周期覆盖低周期、冲突输出如何稳定排序及引用。
6. 不完整输入：缺少某周期、Metric unknown/invalid/degraded、窗口不足、周期不可比较时的结果状态和质量传播规则。
7. 算法版本：上述规则必须整体绑定明确 algorithm_version，并由 Golden 固定正常、边界、冲突和 unknown 场景。

### 本轮处理结论
- 不新增猜测性的 Trend 算法或合成指标权重。
- 不修改已冻结的 QNT-CTR-004 项目级契约。
- QNT-004 保持“部分完成”；后续先取得上述算法语义决议，再在本分支实现纯函数聚合与 Golden。Data 多周期 Adapter 和 Researchflow 消费端验收分别由其责任分支完成。

## 2026-11-26 旧 Runner 趋势逻辑核实补充

本轮按执行清单 1.3 逐行核实旧 Runner 的 trend 实现（`internal/service/wiring.go:258-351`：`trendFactor` 三因子 + `trendStrategy` 三正合取）与 `internal/quant/trend` 的差距，结论与决议请求见 `doc/quant/07-trend-following策略差距与契约结论.md`：

- 旧逻辑只输出单一 `long_candidate`，没有 direction 四值、phase、strength 与 conflict 表达，无法映射到本 DTO 而不新增语义决策；因此不能把旧 `trendStrategy` 当作趋势算法迁移源。
- 本轮仍不实现算法、不注册 `quant.trend` Stage、不改本 DTO 与其 Schema/Golden。phase 与 strength 在无决议时的合法表达分别为 `unknown` 与 `null`。
- 1.3 的退出条件（生产策略 Stage 与契约）未满足，阻塞于 07 文档第 6 节的 D1–D5 五项决议。

## 2026-11-26 D1–D5 决议通过后的实现状态

上一节“阻塞于 D1–D5”的结论已被决议取代：D1=5/20/60、D2=纯符号、D3=phase 恒 `unknown`、D4=strength 恒 `null`、D5=只产出 TrendResult，均按 `doc/quant/07` 推荐项通过。据此：

- `internal/quant/trend` 已实现方向判定与质量三分类（`valid`/`degraded`/`unknown`），`evidence_metrics` 固定为 MA20 距离 + 5d/20d/60d 四个 Metric（顺序即契约），`conflict_refs` 恒为空数组（单周期无跨周期冲突）。
- 本 DTO 未改字段、未改 Schema：phase/strength 仍按第 6 节约定以 `unknown`/`null` 表达，这不是“由 direction 推出”，而是契约内的显式取值（DEC-PLAT-002-049 继续生效）。
- `quant.trend` 已成为真实生产 Stage（Output/Quality Schema 1.0.0、Output Registry 1.4.0、冻结 Template、参数 Schema 无 `window`、Dispatcher 与关联规格表条目），蓝图落库、`result_id` 可被授权读取。
- 仍不变：QNT-CTR-003 `deferred`、QNT-004 的契约状态、QNT-008 Signal NOT_STARTED；相位与强度的判定规则仍需独立决议，才可推进 v2。
