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
| SecurityTradingStatus | 无 |
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
| PriceLimitEvents | 无 |

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
| PriceLimitEvents | 当前未发现 Provider 实现 | Bootstrap 会扫描已启用 Provider 并收集 `PriceLimitEventProvider`，但当前扫描结果为空 | 未接入 |

**StockUniverse 不是孤立接口。** `internal/marketdata/service/market_stocks.go` 中 `DataService.StockUniverse` 遍历已注册的 StockSearchProvider，动态识别 StockUniverseProvider；`MarketStocks` 调用该方法获取全量证券目录，再做分页及行情/均线增强。因此无需新增独立配置项或重复注册机制。HTTP SourceStatus 目前只反映 StockSearch 能力，不单独暴露 StockUniverse 能力位；这不改变 Service 内部的真实消费路径。

**PriceLimitEventProvider 目前只有接口与 Service 装配槽位。** Provider 目录中未发现 `FetchPriceLimitEventEvidence` 的具体实现，Bootstrap 自动收集不会凭空产生可用 Provider；不得将其列为已接入或真实可用能力。

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

本表只汇总 Runtime Acceptance Log 已记录的样本，不把静态实现、普通测试或 CI 成功当作真实数据通过。

| Provider | 已有真实证据 | 当前判定 | 不可外推范围 |
|---|---|---|---|
| BaoStock | 复权因子与日 K 一致性、停牌/复牌及 ST 字段样本（Runtime Log 第 61、72 节） | 对应样本通过 | 不代表全证券、全日期状态覆盖或所有字段通过 |
| Pytdx | GBBQ 公司行为、复权参考日、停牌/临停样本（第 62、64、71 节） | 对应样本通过；不同复权模式结论分别处理 | 不代表与 BaoStock 原生算法完全等价 |
| Hithink | 600519.SH 五项估值复算、披露日期字段、日 K 分页及后复权成交量核验（第 66～69 节） | 五项估值样本通过；快照整体 degraded | 不代表全市场估值、所有财务字段或 Freshness 通过 |
| Eastmoney | 公共源日 K、复权请求与字段样本（第 41～57、70 节） | 按字段/复权模式判定；绝对参考日证据不足时 not_comparable | 不代表其他数据域已通过 |
| Tencent | 公共源日 K、复权请求与字段样本（第 41～57、70 节） | 按字段/复权模式判定；绝对参考日证据不足时 not_comparable | 不代表其他数据域已通过 |
| Sina | 公共源历史日 K 与跨源比较样本（第 41～57 节） | 按样本/口径判定；部分 adjusted 能力 not_supported | 不代表全部复权模式通过 |
| NetEase | 公共源历史日 K 与跨源比较样本（第 41～57 节） | 按样本/口径判定 | 不代表 Quote 或所有字段通过 |
| TickFlow | 日线成交量单位修复及线上复测（第 47～50 节） | 对应样本修复后通过 | 不外推到其他能力、证券或时间窗口 |
| Tushare | 财务/估值、复权因子/公司行动部分接口返回 40203 权限错误（第 60、65 节及相关记录） | 对应接口 permission_required | 不代表 Tushare 所有接口均失败；未获权限前不重复请求 |
| Xueqiu | 有在线探测入口，无对应真实请求验收记录 | 未执行/待凭据与证据 | 不得由接口存在推断通过 |
| MiaoXiang | 有在线探测入口，无对应真实请求验收记录 | 未执行/待凭据与证据 | 不得由接口存在推断筹码/资金流字段通过 |
| Yahoo | 海外市场能力暂缓 | deferred | 不属于当前国内 A 股优先验收范围 |

真实证据的原始记录、Run/Artifact、代码 SHA 和样本边界以 [DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md](../provider-migration/DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md) 为准。对没有逐能力真实证据的格子，状态保持未执行；缺少同口径或独立参考基准时标记不可比较。

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
