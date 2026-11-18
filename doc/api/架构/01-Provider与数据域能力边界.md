# Provider 与数据域能力边界

更新时间：2026-11-18  
代码基线：`feat/data @ 90871455b282d6124d3c25fb9189fbd274894e67`

## 1. 事实来源与判定层级

本文件是代码静态能力盘点，不是 Provider 真实数据验收报告。能力状态必须区分：

- **接口存在**：Provider 实现对应 Go Capability Interface。
- **配置启用**：`configs/marketdata/providers.json` 将 Provider 与能力设为 enabled。
- **运行装配**：Bootstrap 通过类型断言并将能力注册到 Service。
- **真实验收**：有明确代码 SHA、Provider、证券/日期/字段、原始响应或脱敏 Artifact、判定结果的真实请求证据。

上述四层不可互相替代。

## 2. 默认配置启用情况

以下矩阵来自 `configs/marketdata/providers.json`，表示仓库默认配置，不代表线上环境实际配置，也不代表真实数据通过。

| Provider | 默认启用 | 默认配置能力 |
|---|---:|---|
| Eastmoney | 是 | Quote、DailyBars、StockBasic、StockSearch、SectorRankings、IndexQuotes、MarketBreadth、LimitPools、StockBoards、DragonTiger、Announcements、ChipDistribution、CapitalFlow、HotRankings |
| BaoStock | 是 | DailyBars、TradingCalendar、Fundamentals |
| Tencent | 是 | Quote、DailyBars、StockSearch |
| Sina | 是 | Quote、DailyBars |
| NetEase | 是 | Quote、DailyBars |
| TickFlow | 是 | DailyBars |
| Pytdx | 是 | Quote、DailyBars |
| Hithink | 否 | 配置默认关闭 |
| Tushare | 否 | 配置默认关闭 |
| Yahoo | 否 | 配置默认关闭 |
| Xueqiu | 否 | 配置默认关闭 |

MiaoXiang 是由 `MXEnabled` 单独控制的补充 Provider，不在上述 ProviderPriority 主列表中；当前补充接口覆盖筹码分布和个股资金流。其启用状态必须读取实际运行配置，不能从主 Provider 配置推断。

## 2.1 已核对的具体实现入口

本轮逐个读取了以下 Provider 主要实现文件，并核对其公开方法。这里仅列本次已确认的实现，不将文件未覆盖的方法推定为不存在。

| Provider | 已确认的公开实现 | 默认配置 | 说明 |
|---|---|---|---|
| Eastmoney | Quote、DailyBars、StockBasic、StockBoards、HotRankings、ChipDistribution、CapitalFlow、StockSearch、ListStockUniverse、SectorRankings（含 Metric/Direction）、IndexQuotes、MarketBreadth、LimitPool；另有龙虎榜和公告独立文件 | 启用 | 默认能力较广；ListStockUniverse 是 Provider 方法，但当前 Bootstrap 未将 StockUniverseProvider 作为独立能力集合；其他已启用能力仍须以 SourceStatus 和真实证据确认 |
| Hithink | StockSearch、IndexQuotes、TradingCalendar、HotRankings、LimitPool、DragonTiger、IndexCatalog/Constituents/DailyBars、Valuations、Quote、DailyBars、Fundamentals | 关闭 | 代码实现存在不代表已配置或已获得真实接口权限 |
| BaoStock | DailyBars、TradingCalendar、Fundamentals；另有 AdjustmentFactorProvider 实现 | 启用 | 真实字段证据只按 Runtime Log 指定样本 |
| Tencent | Quote、DailyBars、StockSearch | 启用 | 搜索能力由独立实现文件提供 |
| Sina | Quote、DailyBars | 启用 | 当前默认只启用这两项 |
| NetEase | Quote、DailyBars | 启用 | 当前默认只启用这两项 |
| TickFlow | Quote、IndexQuotes、MarketBreadth、SectorRankings（含 Direction）、DailyBars | 启用 | 默认仅启用 DailyBars；其他已实现能力默认关闭 |
| Pytdx | Quote、DailyBars | 启用 | TCP Provider；出站连接由 Root 注入 |
| Yahoo | Quote、DailyBars | 关闭 | 海外市场能力不属于当前国内 A 股优先范围 |
| Tushare | 当前主文件确认 DailyBars；复权因子方法需结合接口完整实现核对 | 关闭 | 真实财务/估值权限曾返回 40203，不得记为数据通过 |
| Xueqiu | HotRankings | 关闭 | 真实在线验收仍需独立证据 |
| MiaoXiang | ChipDistribution、CapitalFlow | 独立开关 `MXEnabled` | 作为 SupplementProvider 注入，不进入 Quote/DailyBars 主回退链 |

实现文件核对基线为当前任务开始时的 `feat/data @ 90871455b282d6124d3c25fb9189fbd274894e67`。Bootstrap 的能力类型断言是运行装配的最终门禁；若配置启用但 Provider 未实现接口，模块构造应失败。

本轮已定位 BaoStock TradingCalendar/AdjustmentFactor、Tencent StockSearch、Eastmoney CapitalFlow 的独立实现文件。仍需继续核对其他可选 Capability Interface（尤其 CorporateAction、HistoricalIndexConstituent、SecurityTradingStatus、MarketCapitalFlow）是否有具体 Provider 实现、注册路径和真实消费者。

## 3. Bootstrap 装配规则

`internal/marketdata/bootstrap/module.go` 当前创建 11 个主 Provider，并按 ProviderPriority 与 ProviderSettings 过滤。配置启用某项能力时，Bootstrap 必须验证对应 Provider 实现该 Capability Interface，否则启动失败。

当前装配的能力域包括：

- Quote、DailyBars；
- StockBasic、StockSearch；
- SectorRankings、IndexQuotes、MarketBreadth；
- LimitPools、DragonTiger、Announcements、StockBoards；
- TradingCalendar、SecurityTradingStatus；
- Fundamentals、Valuations；
- IndexCatalog；
- ChipDistribution、CapitalFlow、MarketCapitalFlow、HotRankings；
- 自动识别 PriceLimitEventProvider。\n\nProvider 接口还定义 AdjustmentFactorProvider、CorporateActionProvider、StockUniverseProvider、MetricSectorRankingProvider、DirectionalSectorRankingProvider、HistoricalIndexConstituentProvider 等扩展能力；这些接口的存在不代表已由 Bootstrap 注册。必须继续追踪具体 Provider 的实现与实际调用路径。

`httpapi.BuildSourceStatuses` 根据 Bootstrap 实际注册的 Provider 列表构造 SourceStatus，不从静态配置直接推断运行能力。该状态只说明装配能力，不是数据质量或真实可用性证明。

## 4. 尚待核验的扩展能力

当前仍需逐一核验 CorporateActionProvider、HistoricalIndexConstituentProvider、SecurityTradingStatusProvider、MarketCapitalFlowProvider，以及 StockUniverseProvider 的独立注册路径。必须确认具体实现、Bootstrap 注册、真实消费者和真实数据证据后，才能纳入有效运行能力矩阵。

## 5. 真实验收入口

- 追加式 Provider 真实运行证据：`../provider-migration/DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md`
- 阶段五历史验收方案：`../acceptance/国内A股数据层阶段五统一验收计划.md`
- 字段校验与诊断规则：`../quality/字段校验与诊断体系实施方案.md`

当前已知限制：
- BaoStock 停牌/复牌字段存在明确样本证据，但不能外推为全市场完整覆盖。
- Eastmoney/Tencent 的复权绝对参考日缺少独立来源证据时必须保持不可比较。
- Tushare、Hithink、Yahoo、Xueqiu 默认关闭；无凭据或未启用的 Probe 应记为 skipped/not_configured，不记为失败，也不能记为通过。
- 真实数据验收以 Runtime Acceptance Log 对应的具体记录为准，不能用 Go 测试、配置启用或 CI 总体成功替代。

## 6. 后续盘点要求

对每个 Provider × 数据域至少补齐：
1. Go 接口和实现文件；
2. 默认配置与部署实际配置；
3. Bootstrap 注册路径；
4. 字段来源、单位、时间语义、复权口径及限制；
5. 真实请求证据及状态（通过/降级/失败/不可比较/未执行）；
6. 证据 SHA、Run/Artifact 和适用样本范围。

没有证据的格子保持未执行或不可比较，不允许按能力接口存在推定通过。
