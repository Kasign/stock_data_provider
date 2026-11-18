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

## 3. Bootstrap 装配规则

`internal/marketdata/bootstrap/module.go` 当前创建 10 个主 Provider，并按 ProviderPriority 与 ProviderSettings 过滤。配置启用某项能力时，Bootstrap 必须验证对应 Provider 实现该 Capability Interface，否则启动失败。

当前装配的能力域包括：

- Quote、DailyBars；
- StockBasic、StockSearch；
- SectorRankings、IndexQuotes、MarketBreadth；
- LimitPools、DragonTiger、Announcements、StockBoards；
- TradingCalendar、SecurityTradingStatus；
- Fundamentals、Valuations；
- IndexCatalog；
- ChipDistribution、CapitalFlow、MarketCapitalFlow、HotRankings；
- 自动识别 PriceLimitEventProvider。

`httpapi.BuildSourceStatuses` 根据 Bootstrap 实际注册的 Provider 列表构造 SourceStatus，不从静态配置直接推断运行能力。该状态只说明装配能力，不是数据质量或真实可用性证明。

## 4. 需单独核验的接口能力

Provider 接口还定义了部分可选/扩展能力，例如历史指数成分、指标排序和方向性行业排名。不能因为接口类型存在就视为已注册或已有真实 Provider 支持。需逐一追踪具体实现、装配路径、调用场景和证据后再纳入有效能力矩阵。

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
