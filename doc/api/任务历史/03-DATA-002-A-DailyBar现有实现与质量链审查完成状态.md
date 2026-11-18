# DATA-002-A：DailyBar 现有实现与质量链审查完成状态

完成日期：2026-11-18  
维护分支：`feat/data`  
任务启动代码基线：`90871455b282d6124d3c25fb9189fbd274894e67`  
本子项归档前 HEAD：`4446714966b05cdb66ed9912e7d8132db2a7e6f3`  
子项状态：DONE

## 1. 核对范围

- `internal/marketdata/service/data.go`：DailyBars、DailyBarsAsOf、覆盖率、缓存契约、调整元数据质量。
- `internal/marketdata/service/provider_probe.go`：DailyBars Provider Probe、交易上下文、跨 Provider 检查。
- `internal/marketdata/service/daily_bar_trading_context.go`：交易日历/证券状态缺口归因。
- `internal/marketdata/service/adjustment_rebuild.go`：复权因子重建和 PIT 可见性。
- `internal/marketdata/normalize/normalize.go`、`internal/marketdata/validation/daily_bar.go`：字段校验和标准化。
- `internal/marketdata/provider/cross_provider_daily_compare.go`：跨 Provider 比较契约。
- Provider 日 K 主实现与现有 Service/Provider 测试。

## 2. 已有能力确认

1. DailyBars 已有 Provider 回退、熔断/重试、缓存契约校验、来源/版本一致性、部分数据降级保留、MariaDB stale 兜底、日期范围和 limit 截断。
2. Normalize/Validation 已覆盖证券身份、日期、OHLC、成交量/成交额单位、金额/成交量一致性、重复日期、质量状态和结构化诊断。
3. DailyBarsAsOf 已从持久化原始日 K 与 PIT 可见复权因子重建；不回退实时 Provider adjusted K 线。
4. 复权重建已校验同证券、同 Provider、同 SourceVersion 因子，按 factor basis 选公式，保留 revision hash、SystemObservedAt、AvailableAt 和质量原因。
5. Provider Probe 已调用交易日历 + SecurityTradingStatus 上下文 helper；同轮 Probe 共享状态缓存并限制最多 10 次状态查询。该能力不等于普通 DailyBars 每次读取都会逐日查询证券状态。
6. CrossProviderDailyComparator 已由 Provider Probe 使用；输入质量必须 valid，调整口径、method、refDate 和日期集合必须满足一致性约束；金额缺失/精度未知只会得到 partial/not_comparable。
7. 80% lookback、65% 交易日覆盖、日历不可用降级、复权参考日期等均已有固定测试。阈值先保留，不在静态审查中随意调整。

## 3. 子项结论

- 现有能力充足，不需要重复建设 Normalize、Validator、Reporter、Probe、Freshness 或新的复权重建器。
- Provider Probe 的交易上下文 helper 有真实生产调用；普通 DailyBars 读取路径未调用该 helper。是否把逐日证券状态查询扩展到普通读取路径，必须先确认消费者和性能预算，不能无条件接入。
- DailyBarsAsOf 使用 PIT 专用重建；不得将当前证券状态或当前因子可用时间用于历史 as_of 查询。
- 没有发现必须新增 DailyBar 字段的证据；现有 AdjustmentMethod、AdjustmentRefDate、Factor Source/Version/Hash、SystemObservedAt、AvailableAt 已覆盖主要追溯维度。

## 4. 变更范围与验收

- 本子项只做代码、测试和文档静态核对。
- 未修改 `internal/marketdata/**`，未新增 API/字段/数据库结构。
- 未触发真实 Provider 请求或消耗上游配额。
- 未宣称当前代码 SHA 的真实 Provider 验收通过。

## 5. 后续

DATA-002-B：逐 Provider 核对 none/forward/backward 方法、AdjustmentRefDate、因子口径和跨源可比条件。当前唯一任务仍为 DATA-002。
