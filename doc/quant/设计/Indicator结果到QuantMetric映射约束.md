# Indicator 结果到 QuantMetric 的映射约束

更新时间：2026-11-18  
适用范围：`feat/quant`，QNT-001 / QNT-002  
依据：项目级《Quant-Research 公开 DTO 字段级契约》2.3、2.3.1、2.7.1；当前 `internal/quant/indicator` 纯函数实现；Quant Metric DTO 与 Registry Validator。

## 1. 目标与边界

本文定义 Indicator 纯计算结果进入公开 `IndicatorResultPayload` 前的字段映射边界。它不是生产构造器实现，也不批准任何尚未确定的 Registry Entry 元数据。

纯计算函数当前只返回数值、算法状态和 sample_count。它们不具备可信请求上下文、输入快照、数据质量判定或 Registry 版本选择能力。因此不得将 `indicator.Result`、`MACDResult`、`BandsResult` 直接 JSON 序列化为 `QuantMetric`。

## 2. 固定映射

| Indicator | 输出 metric_code（固定顺序） | 算法结果来源 | sample_count 来源 |
|---|---|---|---|
| SMA | `sma` | `Result.Value` | `Result.SampleCount` |
| EMA | `ema` | `Result.Value` | `Result.SampleCount` |
| Return | `return` | `Result.Value` | `Result.SampleCount` |
| Volatility | `volatility` | `Result.Value` | `Result.SampleCount` |
| RSI | `rsi` | `Result.Value` | `Result.SampleCount` |
| ATR | `atr` | `Result.Value` | `Result.SampleCount` |
| MACD | `macd_dif`、`macd_dea`、`macd_histogram` | `MACDResult.DIF/DEA/Histogram` | 同一个 `MACDResult.SampleCount` |
| Bollinger | `bollinger_middle`、`bollinger_upper`、`bollinger_lower` | `BandsResult.Middle/Upper/Lower` | 同一个 `BandsResult.SampleCount` |

单值 Indicator 必须输出恰好一个 Metric。MACD/Bollinger 必须分别按表中顺序输出三个 Metric；不允许合并成复合 value，也不允许拆成多个 Envelope 或只发布部分分量。

## 3. QuantMetric 字段来源矩阵

| QuantMetric 字段 | 唯一允许来源 | 禁止推断/默认 |
|---|---|---|
| metric_code | 本文固定映射 + 已注册 Indicator Entry | 不从 Go 类型名或调用方任意字符串推导 |
| value | 对应纯函数的数值结果；仅在 available 时写入 | 不可用状态必须为 null；不得以 0 代替 |
| unit | 对应 metric_code 的正式 Registry Entry | 不从公式名称、float64 或指标常识猜测 |
| period | 已验证的 QuantAnalysisRequest / 计算上下文 | 不把 Window 整数直接格式化成交易周期；Window 不包含 bar interval/calendar |
| system_as_of | 已验证请求中的 UTC 知识截止时间，且经 Server 提供的可信时钟边界校验 | 不在 Domain/Indicator 内调用当前系统时间 |
| quality_status | 输入 Snapshot 质量检查与该 Entry.quality_policy 的判定结果 | 不从 `StatusAvailable` 推断 valid；计算成功不等于输入质量有效 |
| metric_status | 纯函数 Status 到公开 Metric 状态的显式映射 | 不将 invalid input、未知状态降级成 available |
| calculation_version | Registry 实际选中的 algorithm_version | 不使用包版本、Git SHA 或随意常量替代 |
| sample_count | 纯函数返回的 SampleCount，并按 Entry.sample_count.counting_rule 校验口径 | 不以原始输入数组长度、Provider 行数替代；0 与 null 不混淆 |
| lower_bound / upper_bound | 当前 Indicator 纯函数没有计算区间，输出 null | 不把输入窗口、标准差或 synthetic Golden 值伪造成区间 |

`period`、`system_as_of`、`quality_status`、`calculation_version` 属于上下文/Registry 元数据，不属于 Indicator 纯函数输出。未来构造器必须显式接收已经校验的上下文与 Registry Entry，不得自行补默认值。

## 4. 状态映射约束

| Indicator 内部状态 | QuantMetric.metric_status | value | 约束 |
|---|---|---|---|
| `available` | `available` | 非 null | 还必须满足 Entry 的 value_type、precision、range 和 quality_policy |
| `insufficient_data` | `insufficient_data` | null | 保留真实 sample_count；不能用 0 替代未知 |
| 未来明确产生的 insufficient_sample | `insufficient_sample` | null | 只有算法/用例语义确实定义该状态时使用 |
| 未知或未映射状态 | 拒绝构造 | — | fail closed，不做字符串兜底 |

当前 Indicator 内部状态仅有 `available` 与 `insufficient_data`。不得仅因 Metric DTO 支持其他状态，就让 Indicator 构造器自行生成它们。

## 5. 复合 Indicator 一致性

MACD 三个 Metric 与 Bollinger 三个 Metric 必须来自同一次纯函数调用，并共享：

- metric_status；
- sample_count；
- period；
- system_as_of；
- quality_status；
- calculation_version。

若任一分量无法按相同 Entry 版本和上下文构造，则整组失败，不返回部分 metrics。数组顺序和 metric_code 必须严格符合项目 DTO 2.7.1 的 2.7.1 IndicatorResultPayload 映射。

## 6. 构造器实施前置条件

实现生产构造器前，必须具备：

1. 每个 metric_code 的正式 Registry Entry，且 Entry 的 value_type、unit、precision、range、quality_policy、sample_count 元数据均有权威来源。
2. 逐 metric_code 的 Parameters Schema 及算法版本选择规则。
3. 可信且已验证的调用上下文：period、system_as_of、quality_status、输入快照/质量证据。
4. 生产精度与舍入策略。价格派生指标等待 DATA-CTR-005 Decimal v2；统计指标也不能把 synthetic 测试阈值当生产 tolerance。
5. sample_count 的实际计数与 Entry.counting_rule 一致性测试。
6. Indicator Payload Schema/Golden/Validator 对固定映射、数组顺序、同组上下文一致性提供可执行验证。

以上任一条件未满足时，不创建“半成品”生产 Entry，也不实现接受任意字段并填充默认值的通用构造器。

## 7. 当前可独立推进与阻断

Quant 当前可独立完成：固定映射测试、状态映射测试、复合输出原子性测试，以及构造器输入上下文的强类型边界设计。

Quant 当前不能独立决定：生产 value_type/precision/tolerance/rounding/range/quality_policy；RSI unit；Volatility 年化因子与交易日历映射；Data Decimal v2 的原始输入精度和快照质量事实。这些由项目决策或 Data 契约解锁。

**结论：** 本文锁定字段来源和组装规则，不表示生产构造器或 Registry Entry 已实现。QNT-001/QNT-002 继续 IN_PROGRESS。
