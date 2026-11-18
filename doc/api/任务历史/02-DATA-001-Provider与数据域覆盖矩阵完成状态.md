# DATA-001：Provider 与数据域覆盖矩阵及证据缺口核验完成状态

完成日期：2026-11-18  
维护分支：`feat/data`  
任务启动分支 HEAD：`616aef993d60a81e1a763149c4b12657bcffd1a3`  
代码核对基线：`90871455b282d6124d3c25fb9189fbd274894e67`  
任务归档前分支 HEAD：`9c238d49c9da0da82e54486bb0ff7d326e39bbb4`  
任务状态：DONE

## 1. 任务目标

核对所有 MarketData Provider 的接口、实现、默认配置、Bootstrap 注册/动态发现、实际 Service 消费者和已有真实数据证据。对没有真实证据的 Provider × 能力明确标记未执行，不将接口存在、配置启用或 CI 通过当作真实数据通过。

## 2. 完成结果

### 2.1 Provider 清单与默认配置

- Bootstrap 的 `available` 映射构造 11 个 Provider Adapter 候选。
- 默认 `ProviderPriority` 列出 10 个（不含 Yahoo）；其中默认启用 7 个：Eastmoney、BaoStock、Tencent、Sina、NetEase、TickFlow、Pytdx。
- Hithink、Tushare、Xueqiu 默认关闭；Yahoo 虽构造但不在默认优先级列表。
- MiaoXiang 为独立补充 Provider，`MARKET_DATA_MX_ENABLED` 默认 false。
- 区分了默认仓库配置与部署环境实际配置；没有运行环境配置快照时不宣称线上实际启用。

### 2.2 能力实现与注册路径

已核对 11 个主 Provider 候选、MiaoXiang 及其独立实现文件，覆盖 Quote、DailyBars、StockBasic、StockSearch、StockUniverse、SectorRankings、IndexQuotes、MarketBreadth、StockBoards、LimitPools、DragonTiger、Announcements、TradingCalendar、SecurityTradingStatus、Fundamentals、Valuations、IndexCatalog、HistoricalIndexConstituents、ChipDistribution、CapitalFlow、MarketCapitalFlow、HotRankings、AdjustmentFactors、CorporateActions、PriceLimitEvents。

重点核实：

- StockUniverse 由已注册的 StockSearchProvider 列表动态断言；`DataService.StockUniverse` 被 `MarketStocks` 实际消费，不是孤立接口。
- AdjustmentFactors/CorporateActions 通过已启用 NamedProvider 动态发现。
- HistoricalIndexConstituents 在 IndexCatalog Provider 列表中按需类型断言。
- Metric/Directional SectorRanking 复用 SectorRankingProvider 列表按需断言。
- SecurityTradingStatus 通过显式配置和能力切片注册；Tushare 是当前唯一专用实现，默认关闭。
- PriceLimitEventProvider 没有任何具体 Provider 实现，但 HTTP `price_limit_rule` 已有真实消费者；缺少独立事件证据时返回 unknown 并失败关闭。
- SourceStatus 只覆盖部分注册能力，不代表所有扩展能力或真实数据质量。

### 2.3 真实证据映射

已将 Runtime Acceptance Log 的已有真实请求按 Provider/能力映射。主要结论：

- BaoStock：复权因子与自身 qfq/hfq OHLC 一致性样本通过；停牌/复牌、tradestatus/isST 指定样本通过；交易日历因缺少源端快照时间降级。
- Pytdx：none/qfq/hfq 日 K 样本、GBBQ 公司行为事件对账、官方临停/全天停牌指定样本有通过证据；与 BaoStock 原生复权算法不能直接跨源匹配。
- Hithink：600519.SH 五项估值独立复算通过但快照 degraded；财报披露日期样本存在上游字段异常；日 K backward 因小数股数被拒绝。
- Eastmoney：未复权日 K 指定样本通过；复权绝对参考日缺少独立证据时保持不可比较。
- Tencent：未复权日 K 指定样本降级（成交额缺失）；复权参考日不可比较。
- Sina：未复权日 K 指定样本降级；adjusted 能力按记录为 not_supported。
- NetEase：历史请求有 DNS/HTTP 502 失败记录，未用历史瞬时失败外推永久不可用。
- TickFlow：单股 Quote 样本 valid；核心指数行情 6 条样本 valid；日 K 手转股修复样本通过单位交叉校验，但因 prev_close 缺失整体 degraded。
- Tushare：DailyBars、Fundamentals、Valuations、adj_factor/dividend 等指定接口出现 40203，按接口标记 permission_required；未把权限阻塞外推到全部能力。
- Yahoo：有限窗口日 K 返回样本但成交额缺失，degraded；海外市场能力暂缓。
- Xueqiu、MiaoXiang：没有对应真实请求验收证据，标记未执行。

未列出的已实现 Provider × 能力一律标记未执行；没有实现的能力标记不支持；缺少同口径/参考日期/覆盖证据时标记不可比较。

### 2.4 历史证据与当前代码基线

当前代码核对基线为 `90871455b282d6124d3c25fb9189fbd274894e67`。对主要历史真实 Run 的对应实现做了方法级差异检查：

- BaoStock、Hithink 日 K、Eastmoney、Tencent 日 K 的已核对方法差异集中在统一时钟注入。
- TickFlow 日 K 增加逐字段诊断、数组长度/日期/OHLC/数值校验；手转股逻辑仍保留。
- Pytdx 日 K 增加历史窗口截断、复权参考数据请求错误等诊断。
- Hithink Valuations/Fundamentals、BaoStock Fundamentals 的已核对方法差异集中在统一时钟注入；Tushare Fundamentals 对应方法与历史版本一致。
- 这些历史 Run 仍是有效的历史样本事实，但不是当前代码 SHA 的重新执行结果；没有将旧 SHA 的 PASS 自动继承为当前代码验收通过。

### 2.5 发现的明确能力缺口

- `SecurityTradingStatusProvider` 只有 Tushare 实现，且默认关闭；BaoStock 日 K 的 `tradestatus/isST` 样本不等于完整的按日证券状态 Provider。
- `PriceLimitEventProvider` 没有具体实现，但 `price_limit_rule` API 已调用该能力；默认配置下特殊事件证据缺失，接口按设计返回 unknown。
- 两项都没有在 DATA-001 中擅自实现；后续必须先确认独立数据源、字段语义、覆盖承诺、调用配额和真实消费者，再按任务计划准入。

## 3. 文档与代码变更范围

- 新建/更新唯一架构矩阵：`doc/api/架构/01-Provider与数据域能力边界.md`。
- 更新唯一当前任务状态和模块总规划。
- 删除过时重复文档：`doc/api/architecture/行情数据服务说明.md`，其有效内容已收敛到新的唯一架构矩阵。
- 代码范围 `internal/marketdata/**` 未修改；没有新增 API、DTO、字段、数据库表或 Provider。
- 未对任何 Provider 发起新的真实请求，未消耗上游配额。

## 4. 验收依据

- 当前分支矩阵覆盖 11 个 Provider Adapter 候选及 MiaoXiang。
- 接口、实现、默认配置、注册路径和消费者已逐项核对。
- 已有真实样本按 Provider/能力归档；未执行、不支持、权限阻塞、降级、失败、不可比较已明确区分。
- Runtime Acceptance Log 中历史 SHA、Run/Job、样本范围可追溯；当前代码基线未重新执行的格子没有标记当前通过。
- 本任务为静态能力与证据映射任务，不以 CI 作为真实 Provider 验收条件。任务归档前 HEAD 的 Commit Status 无检查记录，不宣称该 SHA CI 通过。

## 5. 后续任务

下一唯一执行任务为 DATA-002：DailyBar 质量与复权参考基准。任务边界和子项以 `doc/api/当前任务状态和进度.md` 为准。
