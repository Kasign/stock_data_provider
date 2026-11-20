# Provider 与数据域能力边界

更新日期：2026-11-18  
代码核对基线：`feat/data @ 90871455b282d6124d3c25fb9189fbd274894e67`  
核对范围：`internal/marketdata/provider/**`、`internal/marketdata/service/**`、`internal/marketdata/bootstrap/module.go`、`internal/marketdata/config/**`、`configs/marketdata/providers.json`

本文只描述代码实现、默认配置、Bootstrap 注册和已有真实证据。部署环境可能通过环境变量覆盖配置；没有运行环境配置快照时，不推断线上实际启用状态。

## 1. 判定层次

| 层次 | 含义 | 证据 |
|---|---|---|
| 接口 | Provider 实现某个细粒度 Capability Interface | `internal/marketdata/provider/provider.go` |
| 实现 | 具体 Provider 有对应方法及来源字段映射 | `internal/marketdata/provider/*.go` |
| 默认配置 | `providers.json` 中 ProviderPriority、enabled 和能力开关 | `configs/marketdata/providers.json` |
| 运行注册 | Bootstrap 按配置筛选并注入 Service；动态能力按既有类型断言发现 | `internal/marketdata/bootstrap/module.go`、对应 Service |
| 真实验收 | 指定代码 SHA、Run/Artifact、样本和字段质量证据 | [Provider Runtime Acceptance Log](../provider-migration/DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md) |

以上五层不能互相替代。配置启用不证明凭据/权限有效；接口注册不证明上游可用；请求成功不等于字段质量通过。

## 2. 主 Provider 与默认配置

Bootstrap 的 `available` 映射定义 11 个 Provider Adapter 候选；默认 `ProviderPriority` 只列出其中 10 个，Yahoo 虽被构造但不在默认优先级列表。默认配置实际启用 7 个主 Provider。MiaoXiang 不属于主列表，使用独立 `MARKET_DATA_MX_ENABLED`，默认 false。

| Provider | 代码已实现能力 | 默认优先级 | 默认启用 | 默认注册能力/说明 |
|---|---|---:|---:|---|
| Eastmoney | Quote、DailyBars、StockBasic、StockSearch、StockUniverse、SectorRankings（Metric/Direction）、IndexQuotes、MarketBreadth、StockBoards、LimitPools、DragonTiger、Announcements、ChipDistribution、CapitalFlow、HotRankings | 是 | 是 | 上述 `providers.json` 明确开启的能力均注册；StockUniverse 通过已注册 StockSearchProvider 的动态类型断言发现 |
| BaoStock | DailyBars、TradingCalendar、Fundamentals、AdjustmentFactors | 是 | 是 | DailyBars、TradingCalendar、Fundamentals 显式注册；AdjustmentFactorProvider 从已启用主 Provider 列表动态发现 |
| Tencent | Quote、DailyBars、StockSearch | 是 | 是 | 三项能力均显式注册；StockSearch 使用独立优先级 |
| Sina | Quote、DailyBars | 是 | 是 | Quote、DailyBars 显式注册 |
| NetEase | Quote、DailyBars | 是 | 是 | Quote、DailyBars 显式注册 |
| TickFlow | Quote、DailyBars、IndexQuotes、MarketBreadth、SectorRankings（含 Direction） | 是 | 是 | 默认仅 DailyBars 开启；其余能力虽有实现但默认关闭 |
| Pytdx | Quote、DailyBars | 是 | 是 | Quote、DailyBars 显式注册；TCP 出站连接由 Root 注入 |
| Hithink | Quote、DailyBars、StockSearch、IndexQuotes、TradingCalendar、HotRankings、LimitPools、DragonTiger、IndexCatalog（目录/当前成分/指数日线）、Valuations、Fundamentals | 是 | 否 | 代码已实现但默认关闭；不能计入默认运行能力 |
| Tushare | DailyBars、StockBasic、TradingCalendar、SecurityTradingStatus、Fundamentals、Valuations、LimitPools、DragonTiger、Announcements、IndexCatalog（含历史成分）、AdjustmentFactors、CorporateActions、MarketCapitalFlow | 是 | 否 | 代码已实现但默认关闭；权限/积分阻塞按具体接口记录 |
| Xueqiu | HotRankings | 是 | 否 | 代码已实现但默认关闭；Cookie/登录态与限流另行验收 |
| Yahoo Finance | Quote、DailyBars | 否 | 否 | Adapter 已构造，但默认 ProviderPriority 不含 yahoo；仅启用开关不足以进入主 Provider 列表 |

默认配置来自仓库文件，不代表线上运行时配置。默认启用主 Provider：Eastmoney、BaoStock、Tencent、Sina、NetEase、TickFlow、Pytdx。默认关闭：Hithink、Tushare、Xueqiu、Yahoo（Yahoo 还不在默认优先级中）。

### 2.1 默认启用能力矩阵

| 数据域 | 默认启用 Provider |
|---|---|
| Quote | Eastmoney、Tencent、Sina、NetEase、Pytdx |
| DailyBars | Eastmoney、BaoStock、Tencent、Sina、NetEase、TickFlow、Pytdx |
| StockBasic | Eastmoney |
| StockSearch | Tencent、Eastmoney（搜索优先级为 Tencent → Eastmoney） |
| StockUniverse | Eastmoney（通过 StockSearchProvider 列表动态发现） |
| SectorRankings | Eastmoney |
| IndexQuotes | Eastmoney |
| MarketBreadth | Eastmoney |
| StockBoards | Eastmoney |
| LimitPools | Eastmoney |
| DragonTiger | Eastmoney |
| Announcements | Eastmoney |
| TradingCalendar | BaoStock |
| SecurityTradingStatus | 无（Tushare 有实现，但默认关闭） |
| Fundamentals | BaoStock |
| Valuations | 无 |
| IndexCatalog | 无 |
| ChipDistribution | Eastmoney |
| CapitalFlow | Eastmoney |
| MarketCapitalFlow | 无 |
| HotRankings | Eastmoney |
| AdjustmentFactors | BaoStock（动态发现，不是独立配置开关） |
| CorporateActions | 无 |
| HistoricalIndexConstituents | 无 |
| PriceLimitEvents | 无（当前无 Provider 实现） |

MiaoXiang 的 ChipDistribution/CapitalFlow 由独立 `MARKET_DATA_MX_ENABLED` 控制，默认关闭，不纳入主 Provider 默认矩阵。

## 3. 扩展能力的真实注册路径

这些能力没有各自独立的 ProviderSettings 开关，不能仅凭配置文件中的字段判断是否接入。

| 能力 | 实现 Provider | 注册/消费路径 | 默认运行状态 |
|---|---|---|---|
| StockUniverse | Eastmoney | `StockSearchProvider` 列表中动态断言 `StockUniverseProvider`；`DataService.StockUniverse` 被 `MarketStocks` 用例调用 | Eastmoney 默认 StockSearch 开启，因此已接入 |
| AdjustmentFactors | BaoStock、Tushare | DataService 遍历已启用的 NamedProvider 并动态断言 `AdjustmentFactorProvider` | BaoStock 默认启用；Tushare 默认关闭 |
| CorporateActions | Tushare | DataService 遍历已启用的 NamedProvider 并动态断言 `CorporateActionProvider` | 无默认来源 |
| HistoricalIndexConstituents | Tushare | IndexCatalog Provider 列表中动态断言 `HistoricalIndexConstituentProvider` | 无默认来源 |
| MetricSectorRankings | Eastmoney | 复用已注册的 `SectorRankingProvider` 列表，调用时按需断言 | Eastmoney 默认启用 |
| DirectionalSectorRankings | Eastmoney、TickFlow | 复用已注册的 `SectorRankingProvider` 列表，调用时按需断言 | Eastmoney 默认启用；TickFlow 的 SectorRankings 默认关闭 |
| SecurityTradingStatus | Tushare | 显式 `security_trading_status` 开关；同时要求该 Provider 的 `stock_basic` 和 `trading_calendar` 开关开启 | 无默认来源；Tushare 默认关闭 |
| MarketCapitalFlow | Tushare | 显式 `market_capital_flow` 开关和独立能力切片 | 无默认来源；Tushare 默认关闭 |
| PriceLimitEvents | 当前没有 Provider 实现 | Bootstrap 会扫描已启用 Provider 并收集 `PriceLimitEventProvider`，但当前扫描结果为空；HTTP `price_limit_rule` 已有真实消费者，缺少事件源时返回 unknown 并失败关闭 | 能力缺口 |

**StockUniverse 不是孤立接口。** `internal/marketdata/service/market_stocks.go` 中 `DataService.StockUniverse` 遍历已注册的 StockSearchProvider，动态识别 StockUniverseProvider；`MarketStocks` 调用该方法获取全量证券目录，再做分页及行情/均线增强。因此无需新增独立配置项或重复注册机制。HTTP SourceStatus 目前只反映 StockSearch 能力，不单独暴露 StockUniverse 能力位；这不改变 Service 内部的真实消费路径。

**PriceLimitEventProvider 是已存在调用场景但缺少数据源实现的能力缺口。** Provider 目录中未发现 `FetchPriceLimitEventEvidence` 的具体实现，Bootstrap 自动收集结果为空；但 `internal/marketdata/httpapi/price_limit_rule.go` 会调用 `DataService.PriceLimitEventEvidence`，缺少独立事件证据时接口将状态置为 unknown、清空可用涨跌幅并保留质量原因，属于有意失败关闭。不能将其当作“无消费者的孤立接口”，也不能用普通涨跌停池代替特殊事件证据。

**SecurityTradingStatus 的默认覆盖同样存在缺口。** 当前仅 Tushare 实现专用 `SecurityTradingStatusProvider`，但 Tushare 默认关闭；BaoStock 日 K 中 `tradestatus/isST` 的指定样本不等于完整的按日证券状态 Provider。HTTP `price_limit_rule` 需要先获取该状态，再获取独立特殊事件证据；默认配置下两项来源都不齐全，接口必须返回 unknown，而不是推断正常状态。

## 4. Provider 实现文件核对索引

| Provider | 主要实现文件 |
|---|---|
| Eastmoney | `eastmoney.go`、`eastmoney_capital_flow.go`、`eastmoney_announcements.go`、`eastmoney_dragon_tiger.go` |
| Hithink | `hithink.go` |
| BaoStock | `baostock.go`、`baostock_calendar.go`、`baostock_fundamentals.go`、`baostock_adjustment_factors.go` |
| Tencent | `tencent.go`、`tencent_search.go` |
| Sina / NetEase / TickFlow / Pytdx / Yahoo / Xueqiu / MiaoXiang | 各自同名 Provider 文件 |
| Tushare | `tushare.go`、`tushare_capabilities.go`、`tushare_adjustment_factors.go`、`tushare_corporate_actions.go`、`tushare_security_trading_status.go`、`tushare_market_capital_flow.go` |

## 5. 真实数据证据矩阵

下表按 Provider 汇总账本中已经存在的真实请求结论。状态只适用于所列能力、代码基线和样本；未列出的已实现能力一律为**未执行**，不从同一 Provider 的其他能力推断。具体原始记录、Run/Artifact 和代码 SHA 以 [DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md](../provider-migration/DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md) 为准。

**重要：** 本节表格中的“通过/降级/失败/不可比较/权限阻塞”是历史 Run 对应样本的判定，不是 `feat/data @ 90871455b282d6124d3c25fb9189fbd274894e67` 当前代码基线的整体验收状态。当前代码基线尚未针对所有 Provider × 能力重新执行真实请求；没有完成差异审查和必要复验的格子，在当前基线仍按未执行处理。

**历史证据与当前代码基线的关系：** Runtime Acceptance Log 中多项真实请求发生在旧分支名 `feat/full-go-data-providers-api-switch` 或旧目录 `services/market-data/internal/provider/**`。当前核对基线为 `feat/data @ 90871455b282d6124d3c25fb9189fbd274894e67`。对 TickFlow、BaoStock、Hithink、Eastmoney、Tencent 的日 K 实现文件进行历史版本对照时，当前文件 Blob 与历史版本不完全相同（部分路径还发生了迁移）。因此这些真实请求仍是有效的历史样本事实，但**不能自动升级为当前代码 SHA 的实时验收通过**；需要逐 Provider/能力核对差异，必要时再做一次最小化真实复验。此规则同样适用于旧 SHA 的 CI 结果。

本轮进一步对照了历史/当前的 `FetchDailyBars` 实现：BaoStock、Hithink（含 `fetchDailySegment`）、Eastmoney、Tencent 的相关方法差异集中在统一时钟注入；对应历史样本仍可作为字段/单位结论的参考，但当前 SHA 的端到端结果仍未重新执行。TickFlow 当前日 K 方法新增了逐字段解析诊断、数组长度/日期/OHLC/数值校验和缺失字段报告；`HandsToShares` 归一化仍保留，历史 8 条样本可证明当时的手转股根因与修复，但不能替代当前代码对同一样本的复验。TickFlow Quote 的手转股逻辑保持，主要变化为统一时钟注入和索引行情原始结构定义。

| Provider | 已有真实证据及判定 | 尚未验收/适用边界 |
|---|---|---|
| Eastmoney | DailyBars：600036.SH 未复权 8 条样本 valid；qfq 因参考日期缺失 degraded，后续 qfq/hfq 参考日期仍不可比较（第 43、51、70 节）。StockSearch：历史一次 HTTP 502（第 22 节） | Quote、StockBasic、StockUniverse、SectorRankings、IndexQuotes、MarketBreadth、StockBoards、LimitPools、DragonTiger、Announcements、ChipDistribution、CapitalFlow、HotRankings 均无对应完整字段质量验收记录，未执行 |
| BaoStock | AdjustmentFactors 与自身 qfq/hfq 日 K 一致性样本通过（第 61 节）；停牌/复牌、tradestatus/isST 指定样本通过（第 72 节）；Fundamentals 曾返回股本字段样本（第 67 节）；TradingCalendar 实际返回但缺少源端快照时间，degraded（第 59 节） | 其他证券/日期状态覆盖、完整 Fundamentals 字段、普通 DailyBars 全面可用性仍未验收；历史连接拒绝记录仍保留，不用其他样本外推 |
| Tencent | DailyBars：none 样本 degraded（成交额缺失）；qfq/hfq 返回数据但参考日期缺失，not_comparable（第 43、51、70 节）；Quote 有限样本存在但不能据此认定全字段质量通过 | StockSearch 及其他未列能力未执行；历史成交额字段缺失不得推算 |
| Sina | DailyBars：none 样本 degraded（成交额缺失）；调整口径 not_supported（第 43、51、57 节）；Quote 有限样本存在但不代表全部字段通过 | StockSearch 不支持；Quote 完整字段质量和其他未列能力未执行 |
| NetEase | 历史 Quote/DailyBars 请求曾出现 DNS/HTTP 502（第 22 节）；该记录仅代表当时请求失败 | 尚无后续对应真实成功和质量复验；当前能力状态未确认，不将历史瞬时失败外推为永久失败 |
| TickFlow | Quote：600519.SH 单行样本 valid（第 47 节）；IndexQuotes：6 个核心指数 valid（第 41 节）；DailyBars：手转股修复后 8 条样本通过单位交叉校验，但缺 prev_close，整体 degraded（第 49～50 节） | MarketBreadth、SectorRankings 未执行；Quote/DailyBars 证据只适用于对应样本，不外推全市场 |
| Pytdx | DailyBars：none/qfq/hfq 各 9 条样本通过结构/质量断言（第 64 节）；GBBQ 与 BaoStock 公司行动事件 3/3 对账（第 62 节）；盘中临停/全天停牌指定窗口通过（第 71 节） | Pytdx 与 BaoStock 原生复权算法不可直接等同；跨 Provider adjusted OHLC 仍 not_comparable；SecurityTradingStatus Provider 本身未做端到端真实验收 |
| Yahoo | DailyBars：有限窗口 none/qfq/hfq 返回样本，但成交额缺失，degraded（第 41、43、51 节） | 海外市场能力暂缓；不属于当前国内 A 股优先验收范围 |
| Hithink | Valuations：600519.SH 五项指标独立复算通过，但快照整体 degraded（第 67 节）；Fundamentals：披露日期字段样本与官方年报日期不一致，PIT 保守延迟可见（第 68 节）；DailyBars：none/forward 返回 23 条，backward 因小数股数被拒绝（第 69 节） | IndexCatalog、TradingCalendar、LimitPools、DragonTiger 等虽有在线探测入口，但不能将入口/单能力结果外推为全部通过；其余能力未执行 |
| Tushare | DailyBars、Fundamentals、Valuations、adj_factor、dividend 等指定接口出现 40203，按对应接口标记 permission_required（第 60、65 节） | 未获得权限前不重复请求；不能将这些权限结果外推为全部 Tushare 能力失败；SecurityTradingStatus、IndexCatalog 等其他能力未执行 |
| Xueqiu | 无真实请求验收记录 | HotRankings 未执行；Cookie/登录态与限流状态未知 |
| MiaoXiang | 无真实请求验收记录 | ChipDistribution、CapitalFlow 未执行；`MARKET_DATA_MX_ENABLED` 默认 false |

### 5.1 Provider × 能力状态的缺省规则

为避免把“没有写入证据表”误解为“通过”，DATA-001 矩阵按以下规则解释：

1. 在第 2～4 节确认有具体实现、但第 5 节没有该 Provider × 能力的独立真实证据：状态为 **未执行**。
2. Provider 没有实现对应 Capability Interface：状态为 **不支持**，不应发起请求或虚构结果。
3. 已请求但权限/积分/凭据被拒绝：状态为 **权限阻塞**，不是数据质量失败，也不是通过。
4. 请求返回但字段、单位、覆盖或时间语义不满足质量门禁：状态为 **降级**；违反硬契约且不能安全使用时为 **失败**。
5. 缺少同口径、复权参考日、来源可用时间或独立基准：状态为 **不可比较**，不能通过放宽门禁改成 matched。
6. CI、Provider 接口单测、模拟 HTTP Fixture、MariaDB Repository/PIT 测试，只证明各自测试范围，不是上游真实数据证据。

## 6. 统一状态判定

| 状态 | 判定标准 |
|---|---|
| 通过 | 指定样本的字段、单位、时间、覆盖和质量契约均有真实证据支持 |
| 降级 | 数据可用但存在已明确的时间、字段、覆盖或来源限制；限制不会被伪装为有效 |
| 失败 | 真实请求返回错误数据，或违反字段、单位、时间、覆盖等硬契约 |
| 不可比较 | 缺少同口径、独立参考日期、来源可用时间或可比样本，不能作正确性结论 |
| 未执行 | 没有该 Provider × 能力的真实请求证据 |
| 权限阻塞 | 凭据、积分、Cookie、API Key 或账号权限不足；不等于数据质量失败 |

无凭据/无权限记为 skipped、not_configured 或 permission_required；不支持的能力记为 not_supported；不能把 skipped/not_configured 统计为失败或通过。

## 7. 依赖与职责边界

- Platform 提供 HTTP Client/BoundDoer、Fault、Storage、Diagnostics、Time 等公共技术能力；Data 不重复实现。
- Server 是组合根，负责共享 DB/Redis、配置与 Secret 注入、Migration 总注册、Root 路由和生命周期；Data 只暴露已有 Module、Application Port、Handler、Diagnostics 与 MigrationRunner。
- Research/Quant/Researchflow 通过 Data Application Port 消费数据，不直接依赖 Provider/Repository。
- Web 只消费已经批准并实现的 API，不反向决定 Provider 能力或数据字段。
- MDM 并入 Root Server 的剩余装配由 Server SRV-009 负责，不属于本任务的 Data Provider 能力开发。

## 8. 后续核验入口

- 任务唯一状态：[模块总任务规划和进度](../模块总任务规划和进度.md)
- 当前任务：[当前任务状态和进度](../当前任务状态和进度.md)
- 真实请求账本：[DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md](../provider-migration/DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md)
- 字段质量与诊断规则：[字段校验与诊断体系实施方案](../quality/字段校验与诊断体系实施方案.md)
- 阶段验收方案：[国内 A 股数据层阶段五统一验收计划](../acceptance/国内A股数据层阶段五统一验收计划.md)
