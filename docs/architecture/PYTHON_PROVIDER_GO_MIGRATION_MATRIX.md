# Python 数据源到 Go 的迁移矩阵

> 本文按上游 `ZhuLinsen/daily_stock_analysis/data_provider/base.py` 的 `DataFetcherManager._init_default_fetchers`、核心数据方法以及 `src/market_analyzer.py` 中的市场概览调用整理。该原始 Python 目录未完整复制到当前仓库，因此每个未迁移项目都必须以该上游源码为准逐项对照；本表不是“已迁移”的声明。

本矩阵与 [数据源扩展计划](./DATA_SOURCE_EXPANSION_PLAN.md) 配套维护；该计划优先按能力与真实验收状态安排新源，不将联网测试通过等同于生产接入完成。

## 1. 数据源注册清单

| 原 Python 数据源 | 原注册条件 / 默认位置 | 主要市场或用途 | 当前 Go 对应 | 状态 |
|---|---|---|---|---|
| EfinanceFetcher | Python wrapper；日线/行情/基本资料/所属板块多经由东方财富 | A 股行情、日线、实时指数、涨跌统计、行业/概念排行、个股基本信息、所属板块、增强数据 | Go 原生 profile 已映射动态 PE/PB、行业、市值、板块编号、ROE、净利率、净利润和毛利率；enhanced_data 可由独立 API 组合 | profile 扩展值没有独立上游时间戳，质量始终 degraded；任意地区指数列表与 Python 字段组合不保证 1:1 等价；未统一测试 |
| AkshareFetcher | Python 多端点聚合库；A 股日线回退东方财富 → 新浪 → 腾讯；筹码分布调用 stock_cyq_em 本地计算；人气榜/飙升榜使用东方财富 rank API | A 股行情、日线、指数历史、筹码分布、市场统计、行业/概念、涨停池、人气榜/飙升榜 | Go 原生 Eastmoney/Sina/Tencent 覆盖行情与日线，THS/Tushare 覆盖不同代码空间下的指数目录/成分/历史日 K；Eastmoney Go 原生 CYQ 算法、市场统计/板块/涨停池和人气榜/飙升榜均已接入，Xueqiu 为可选的人气榜/关注榜备用源 | 人气榜/飙升榜已接入独立路由，因来源时间缺失保持 degraded；个股基本资料扩展字段仍未完整迁移；筹码输出降级且仅为最新快照；未统一测试 |
| PytdxFetcher | 国内行情/日 K 末位回退源，默认开启 | A 股实时行情、日线 K 线；支持通达信除权事件辅助前/后复权 | Go 原生 `github.com/injoyai/tdx` v0.0.86 Provider（QuoteProvider + DailyBarProvider） | 已接入代码与路由，未统一测试；连接/行情服务器可用性、复权契约待集中验收 |
| BaostockFetcher | 原 Python 默认 P3；Go 按历史 K 线优先候选 | A 股历史日线优先；财务、复权因子、除权除息、基础资料/行业/指数成分按 API 能力逐项扩展 | Go 原生 TCP 客户端与 DailyBarProvider 已实现；NAS Docker 容器已真实登录/查询，并覆盖多个证券及前复权、后复权、不复权样本；GitHub Runner #100 又返回 8 根样本日 K | 日 K 已有目标容器首轮实测，可作为受限回退源；仍需持续观察稳定性和更多证券/日期范围，不能外推为全覆盖或 SLA；当前 `providers.json` 与服务 `.env.example` 默认开启财务指标和交易日历；部署时仍可分别通过 `MARKET_DATA_BAOSTOCK_FUNDAMENTALS_ENABLED`、`MARKET_DATA_BAOSTOCK_TRADING_CALENDAR_ENABLED` 关闭 |
| YfinanceFetcher | 默认启用；默认 P4；海外股票与指数的专用路径 | 美股 / 海外行情与历史数据、部分基本面数据 | 无 | 未迁移 |
| TencentFetcher | 默认启用；默认 P5 | A 股行情与日线兜底 | 原生 Go Tencent Provider | 已迁移部分行情能力，口径仍需逐项对照 |
| TushareFetcher | 仅配置 `TUSHARE_TOKEN` / `MARKET_DATA_TUSHARE_TOKEN` 且显式开启后注册 | A 股原始日线、证券基础资料、交易日历、季度财务指标、批量估值、涨跌停池、龙虎榜与公告；权限依接口而异 | Go 原生 HTTP `Tushare` Provider：DailyBar、StockBasic、TradingCalendar、Fundamentals、Valuation、LimitPool、DragonTiger、Announcement、IndexCatalog（默认关闭） | 接口与独立能力路由已接入；日线只支持 `none`；估值保留报告日期但无精确快照时间；`pe` 单独保留为 `pe_static`，不冒充 MRQ PE；PCF 未覆盖；财务返回报告期的最新修订版本并保留公告日期，但尚不支持历史 `as_of` 版本重建；事件金额按 provider_defined 保留；公告依赖单独开通权限；指数成分每次按月查询最新可用快照并保留大结果降级提示；未统一测试，不宣称生产可用 |
| TickFlowFetcher | 免费 API 可免 Key 读取历史日 K；实时行情需 `TICKFLOW_API_KEY` 且显式开启 quote 能力 | 国内 SH/SZ/BJ 实时行情、日线 K 线 | Go 原生 REST `TickFlow` Provider（免费日 K 默认启用，行情默认关闭） | 已接入 API client 和手→股单位标准化；暂未统一测试，免费源频率限制与 Key 权限需后续验收 |
| MiaoxiangFetcher（妙想 / MX_API） | 仅配置 `MX_APIKEY` 后注册；默认优先级 6 | 筹码分布及个股资金流等第三方补充数据 | Go `MiaoXiang`（独立 SupplementProvider，显式 opt-in） | 已迁移首批筹码/资金流字段；尚缺真实 MX Key 联调与精确 publication availability |
| LongbridgeFetcher | 仅配置 Longbridge OAuth / 旧凭据后注册 | 美股 / 港股行情及日线兜底 | 无 | 可选源，未迁移 |
| FutuFetcher | 检测到 Futu OpenD 端点配置后注册 | 港股等 Futu 支持市场 | 无 | 可选源，未迁移 |
| FinnhubFetcher | 仅配置 `FINNHUB_API_KEY` 后注册 | 以美股/海外股票为主的补充行情和历史数据 | 无 | 可选源，未迁移 |
| AlphaVantageFetcher | 仅配置 `ALPHAVANTAGE_API_KEY` 后注册 | 以美股/海外股票为主的补充行情和历史数据 | 无 | 可选源，未迁移 |
| 同花顺 / THS Financial-API | 通过 `MARKET_DATA_HITHINK_API_KEY` 或 `HITHINK_FINANCE_API_KEY` 配置；默认关闭，按能力显式开启 | A 股名称/代码搜索、行情快照、历史日 K、指数/板块目录、当前成分股、指数历史日 K、指数快照、最近一年交易日历、财务指标与三张财务报表、三类涨跌停池、龙虎榜、批量估值、人气榜/飙升榜 | Go 原生 REST `Hithink` Provider（StockSearchProvider + QuoteProvider + DailyBarProvider + IndexQuoteProvider + TradingCalendarProvider + FundamentalsProvider + ValuationProvider + IndexCatalogProvider + LimitPoolProvider + DragonTigerProvider + HotRankingProvider） | 按能力独立开关；指数目录默认关闭；三类涨跌停池复用统一端点，金额单位未知时保持 `provider_defined`；人气榜/飙升榜通过可选 `MARKET_DATA_HITHINK_HOT_RANKINGS_ENABLED` 接入现有源优先级链，数据延迟时保持 degraded；历史成分与完整板块历史行情仍不支持；财务指标分区与三张报表分区分别校验；服务层允许六个标准指标分区加最多三张报表，未知/重复分区仍拒绝；未统一测试 |
| Akshare / YFinance fundamental adapters | 由基本面上下文聚合逻辑调用，不完全等同于行情 Fetcher 列表 | 财务与基本面指标 | 无 | 未迁移 |

原 Python 管理器会按注册的 Fetcher 优先级排序，并针对市场类型、数据能力、可用性和失败状态采用不同的路由 / fallback 规则。因此，简单地把这张表映射成一个全局 Provider 顺序并不足以复现原行为。

## 2. 按数据能力的当前覆盖

| 数据能力 / 原管理器入口 | 当前 Go 实现 | 当前缺口 |
|---|---|---|
| 个股实时行情 `get_realtime_quote` | Eastmoney、Tencent、Sina；Yahoo 为显式关闭的可选源 | GitHub Runner #100 中新浪、腾讯、东方财富均返回报价；腾讯和 Yahoo 因字段/成交额契约限制为 degraded，Yahoo 默认关闭；仍需逐项对照原 Fetcher 的字段补齐与市场路由 |
| 日线历史 `get_daily_data` | Eastmoney、Tencent、Sina、BaoStock；Yahoo 默认关闭，网易当前配置启用为后置回退但目标网络未验收 | GitHub Runner #100 对 Eastmoney、Tencent（none/qfq）、Sina（none）、BaoStock（qfq）和 Yahoo（qfq）各返回 8 根样本 K 线；新浪/腾讯/Yahoo 缺可核验成交额时保持 null/unknown/degraded；网易探测本次因 HTTP 502 被跳过；仍未迁移 Efinance / AkShare / Pytdx / Tushare / TickFlow / Longbridge 等来源、指数专用链和市场路由 |
| 股票名称/代码搜索 | 独立 `StockSearchProvider` 链，腾讯 Smartbox 优先、东方财富 CLIST 兜底；分别由 `MARKET_DATA_TENCENT_STOCK_SEARCH_ENABLED` 与 `MARKET_DATA_EASTMONEY_STOCK_SEARCH_ENABLED` 控制，不必关闭各自的行情或日 K 能力 | 两个国内 Provider 均保留独立开关，搜索优先级继续为腾讯 → 东方财富；`/api/v1/stocks/search` 已实现，现补齐 OpenAPI 响应模型与参数/错误契约。测试源码已覆盖认证、参数校验和规范化结果；本轮未执行测试或真实上游请求 |
| 指数实时报价 | 新增独立 `IndexQuoteProvider`、东方财富 `ulist.np/get` 适配器与 `GET /api/v1/market/indices`，仅覆盖 7 个白名单核心指数 | 当前 `configs/providers.json` 与服务模板默认启用；历史 Runner #100 返回 7 条，但接口经 302 跳转后未提供可解析行情时间，结果为 `degraded`。默认启用不等于已完成目标部署容器验收 |
| 指数目录 / 成分 / 日线 | Hithink 专用指数目录、当前成分和历史日线；Tushare index_basic、index_weight、index_daily 通过 tag=tushare 使用独立代码空间 | Hithink 使用 .TI/.SH/.SZ，Tushare 使用 .SH/.SZ/.CSI；当前成分按最近可用权重快照，日线成交量保留手、成交额从千元换算成人民币元；能力默认关闭并待统一验收 | 成分快照新增 provider_returned/excluded_count；缺名称被排除时显式计数并降级；两源均不提供可重建的历史成分生效序列 |
| 市场涨跌统计 get_market_stats | Go MarketBreadthProvider 通过东方财富 getTopicZDFenBu 汇总涨跌分布、通过 getTopicZTPool/getTopicDTPool 按同一报告日补充涨跌停总数，并以 CLIST 的 f6 分页汇总全市场成交额；API 为 GET /api/v1/market/breadth | 当前配置默认启用，结果缓存 5 分钟；成交额只在分页无缺失/重复、每条报价日期与分布报告日一致、成交额字段有效且证券数量与涨跌分布覆盖数完全一致时输出。多页采集非原子快照、证券全集未由独立主数据核验，质量仍为 `degraded`；代码与契约已更新，待用户统一回归及生产源验收 |
| 行业板块涨跌榜 `get_sector_rankings` | 新增独立 `SectorRankingProvider`、东方财富 CLIST 适配器及 `GET /api/v1/market/sectors?type=industry` | 当前配置默认启用；既往 Runner 返回 10 条，但上游缺少可核验快照时间，结果固定为 `degraded`。目标部署环境验收待完成 |
| 概念 / 题材涨跌榜 `get_concept_rankings` | 复用板块 ranking 契约、使用独立概念过滤条件；API 为 `GET /api/v1/market/sectors?type=concept` | 当前配置默认启用；既往 Runner 返回 10 条，但上游缺少可核验快照时间，结果固定为 `degraded`。目标部署环境验收待完成 |
| 涨停池 get_limit_up_pool、跌停池、炸板池 | 新增 LimitPoolProvider、东方财富 push2ex 三类池端点及 GET /api/v1/market/limit-pools?type=up|down|broken | 当前配置默认启用。既往 Runner #100 的报告总数为涨停 82、跌停 6、炸板 12，但涨停池本次只返回 30 条样本；源端没有可核验快照时间，结果为 `degraded`。NAS/Docker 与跨交易日语义验证未完成 |
| 个股所属板块 `get_belong_boards` | 新增独立 `StockBoardsProvider`、东方财富 `slist/get` 适配器与 `GET /api/v1/stocks/{symbol}/boards` | 当前配置默认启用；既往 Runner #100 对 600036.SH 返回 26 个板块项，但源端没有可核验快照时间，且板块类型语义仍有限，结果为 `degraded`；NAS/Docker 验收待完成 |
| 交易日历 | 新增独立 `TradingCalendarProvider`，通过 BaoStock TCP `query_trade_dates` 和可选 Tushare Pro `trade_cal`，统一使用 `GET /api/v1/market/trading-calendar?start=YYYY-MM-DD&end=YYYY-MM-DD` | BaoStock 已有首轮真实样本；Tushare Go 实现已加入代码但默认关闭，需配置有效 Token 与接口权限；两者均须满足连续自然日覆盖契约，待统一验收 |
| 筹码分布 `get_chip_distribution` | 东方财富原生 CYQ 算法（基于最新未复权日 K 与换手率计算），可选 MX API 作为补充源；独立 API 与缓存 | 当前东方财富能力默认启用；无独立筹码快照时间，结果保持 `degraded`，仅代表最新可计算快照，不得用于伪造历史 `as_of`；精确 publication/available_at 仍缺失 |
| 人气榜与飙升榜 | 东方财富原生 `HotRankingProvider`，API `GET /api/v1/market/hot-stocks?type=popular&limit=10`（`type=surge` 查询飙升榜）；端点分别由 `MARKET_DATA_EASTMONEY_HOT_RANKINGS_URL`、`MARKET_DATA_EASTMONEY_HOT_SURGE_URL` 配置 | 两个变量默认沿用当前公开端点，启动时执行 HTTP(S) URL 形态校验；源端缺少可信快照时间，结果继续标记 `degraded`；配置可达地址不等于完成生产可用性验收 |
| 基本面 `get_fundamental_context` | BaoStock 与可选 Tushare Pro `fina_indicator` 均提供 Go 原生 `FundamentalsProvider`，复用 `GET /api/v1/stocks/{symbol}/fundamentals?year=YYYY&quarter=N` | 按季度输出六组指标并保留报告期/公告日期；`as_of` 会被 HTTP 层明确拒绝（400），防止误以为支持历史时点查询；Provider 当前只返回指定报告期在请求时可获得的版本，尚不能重建当时已发布的历史修订；Tushare 权限与两源字段等价仍待统一验收 |
| 个股资金流 `get_capital_flow_context` | 可选 Go MX API 路由；字段按人民币元规范化，5/10 日累计要求完整观测 | 自然语言查询返回的指标需要验证字段标签/单位；缺值不补零，精确 available_at 未实现 |
| 龙虎榜 `get_dragon_tiger_context` | 东方财富原生 `DragonTigerProvider`、API `GET /api/v1/market/dragon-tiger?date=YYYY-MM-DD&page=1&page_size=100`；同花顺/Tushare 是可选能力 | 当前配置默认启用东方财富路由，但源端只有交易日、没有精确快照时间；空列表应为 `unknown` 而非确认零条，金额缺失保留 `null`。Runner 与 NAS/Docker 的交易日/分页覆盖仍待统一验收 |
| 股票名称 / 基础资料 `get_stock_name` / profile | 新增独立 `StockBasicProvider` 和 `GET /api/v1/stocks/{symbol}/profile`，接入东方财富 F10 发行信息（简称、上市日期、上市状态） | 单证券基础身份信息已在 GitHub Runner HTTP 200 实测通过；不等于批量名称或完整公司基本面，目标部署容器仍待验收 |
| 批量估值 | 可选 Tushare daily_basic Go Provider 与 GET /api/v1/market/valuations?symbols=... | 按最近已完成交易日返回 PE/PB/PS；不支持的 PCF 保持 null，并返回 provider_report_date；缺少精确更新时间时 quality 为 degraded；默认关闭，需 Token/积分权限，待统一验收 |
| 股票名称 / 代码搜索 | 独立 `StockSearchProvider`，东方财富 CLIST 证券目录与 6 小时缓存，API `GET /api/v1/stocks/search?q=贵州茅台&limit=10` | 实测已证实目录源 HTTP 200 并报告 5,925 条证券，但首轮 `pz=5000`、次轮 `pz=1000` 都遇到服务端分页上限低于请求值，代码因此只扫描首页并漏掉“贵州茅台”。当前调整为每页 100 条、最多 100 页，并新增后续页匹配回归测试；待 Go CI 与真实 Runner 再验收 |
| 新闻 / 公告 | 东方财富公告 API + Go Core 原生 Bocha、Tavily、Brave News 和显式 SearXNG；多源并发、日期门禁、域名加权、反营销、URL/标题去重与个股相关性排序 | 已补源适配器及本地 fixture 测试源码；未运行测试/构建/CI/真实搜索请求。仍保留 Python bridge fallback；更完整的多维情报查询与生产覆盖率需统一验收 |

当前原生 Go API 提供 `GET /api/v1/quotes/{symbol}`、`GET /api/v1/bars/{symbol}/daily`、`GET /api/v1/bundles/{symbol}`、`GET /api/v1/data-sources`、`GET /api/v1/stocks/{symbol}/profile`、`GET /api/v1/stocks/{symbol}/boards`、`GET /api/v1/market/sectors?type=industry|concept`、`GET /api/v1/market/indices`、`GET /api/v1/market/breadth`、`GET /api/v1/market/limit-pools?type=up|down|broken`、`GET /api/v1/market/trading-calendar?start=YYYY-MM-DD&end=YYYY-MM-DD`，并新增可选的 `GET /api/v1/stocks/{symbol}/chip-distribution` 与 `GET /api/v1/stocks/{symbol}/capital-flow`。这些新增能力保持独立路由/开关；当前代码已将部分东方财富国内能力（核心指数、市场广度、行业/概念榜、涨跌停池、所属板块、龙虎榜、公告、筹码分布和人气榜）配置为默认启用，但目标 NAS/Docker 端到端验收未完成，默认启用不代表生产可用；海外与需凭据的可选能力仍按独立开关管理。

## 3. Go Provider 设计要求

每一个新源应实现与能力相匹配的接口，而不是强迫所有源都实现同一组方法：

- `NamedProvider`：名称、版本和健康状态；
- `QuoteProvider`：实时行情；
- `DailyBarProvider`：历史 K 线；
- 后续新增明确独立的能力接口：`IndexProvider`、`MarketBreadthProvider`、`SectorRankingProvider`、`TradingCalendarProvider`、`ChipDistributionProvider`、`FundamentalsProvider`、`CapitalFlowProvider`、`DragonTigerProvider`、`LimitUpPoolProvider`、`NewsProvider`。

不同能力应有独立数据模型、配置开关、超时 / 重试预算、熔断状态和测试；不能因一个供应商只支持报价就被排除，也不能把其日 K 能力默认为可用。

标准化结果至少应记录：`source`、`source_version`、`provider_timestamp`（源数据时点）、`fetched_at`、`normalized_at`、`quality_status`、`quality_reasons`、单位、复权口径及请求 ID。缺失值要保留为缺失，不得默认为零；跨源兜底需要输出 `fallback_from` 与 `fallback_reason`。

## 4. 迁移验收顺序

1. **冻结旧契约**：从上游 Python Fetcher 与 Manager 为每项能力提取请求参数、数据字段、异常规则、优先级、缓存和时间语义；为典型股票、指数、停牌 / 无数据、超时、权限失败准备脱敏样本。
2. **先补共享基础能力**：证券代码 / 市场识别、交易日与时区、单位归一化、复权基准、来源元数据、质量等级、Provider 健康与负缓存。
3. **按数据域实现**：报价与日 K → 指数 / 市场广度 → 板块 / 概念 / 涨停池 → 筹码 / 财务 / 资金流 / 龙虎榜 → 新闻与公告。
4. **差异对照**：以原 Python 与 Go 的同一请求、同一数据时点进行差异测试。不能把不同时间点的行情差异误报成标准化错误；也不能将两套结果不同简单判定为 Go 正确。
5. **切换调用方**：为旧 API 的每个路由建立 Go 兼容适配器，比对 HTTP method/path/query/body/response/error/status；没有覆盖的路由先保留明确的过渡桥接，并将使用量和失败原因记录到日志。
6. **删除 Python 运行时依赖**：只有当所有被使用的能力已切换、兼容测试通过、部署环境验收完成后，才能删除对应 Python provider 代码与 bridge 路由。LLM、通知等非数据源能力不应随行情 Provider 一起误删。

## 5.1 数据源扩展说明（2026-10-22）

BaoStock 的详细 Go 接入、按能力分工和验收条件见 [数据源扩展计划](./DATA_SOURCE_EXPANSION_PLAN.md)。Go 原生客户端与 Provider 已加入并有模拟协议测试，但完成容器内真实登录/查询、字段单位/复权口径验证及逐日对账前，不能标记为生产可用；默认启用开关保持关闭。

当前东方财富在用户 NAS 的实时行情与历史 K 线调用均出现 Empty reply；腾讯中芯国际样本未返回 qfqday。因此后续日 K 路由应以“请求所需复权口径 + Provider 当前能力 + 数据质量”为准，不应仅按静态优先级选择任何返回了 K 线的源。

## 5. 本轮状态

本分支已增加 Sina 报价专用 Go Provider 和测试；GitHub Actions 的三个 Go 模块（根模块、`go-core`、`services/market-data`）的测试、竞态检测与 `go vet` 已在最近一次 CI 运行中通过。

根目录 `cmd/analysis-api` 已改为调用 `internal/adapters/marketdata`，实时行情和日 K 主采集链路不再依赖 Python API；旧 `internal/adapters/pythonapi` 包仍保留给 pythonapi-smoke、迁移对照和旧测试，当前主 API 不调用该包。尚未完成：逐一移植原 Python 的全部 Fetcher 与其他数据域；`go-core` 的市场概况、新闻、LLM、持久化与通知等能力仍有 Python bridge 依赖；尚未进行目标服务器上的真实供应商、MariaDB / Redis、Docker Compose 端到端验收。请勿将本阶段作为完全去 Python 化的最终发布。


## 国内季度财务指标第一阶段（真实 Provider 验收待完成）

已新增 BaoStock 原生 TCP 的六组季度指标能力：`GET /api/v1/stocks/{symbol}/fundamentals?year=YYYY&quarter=N`。范围是盈利能力、营运能力、成长能力、偿债能力、现金流量与杜邦指标。此能力保持 `MARKET_DATA_BAOSTOCK_FUNDAMENTALS_ENABLED=false` 默认关闭；它不是完整财务报表 API。历史分析必须用 `publication_date` 排除分析参考时间之后才披露的数据。GitHub Runner 真实协议复验与目标 NAS/Docker 验收完成前，不标记为生产可用。东方财富公开接口问题统一记录在 `docs/architecture/EASTMONEY_DEFERRED_ISSUES.md` 与 [Issue #6](https://github.com/Kasign/stock_data_provider/issues/6)，本轮不分散处理。


## GitHub Runner 国内 Provider 实测记录（2026-10-23，Probe #100）

来源：[Live Provider Probe #100](https://github.com/Kasign/stock_data_provider/actions/runs/39941383744) 的 live-provider-report artifact（采样行情 600519.SH、日 K 600036.SH；报告请求区间 2026-09-18 至 2026-10-22）。本轮 artifact 的 K 线能力每次仅返回 8 根样本，不能据此证明完整区间覆盖率。

- **报价**：新浪、腾讯、东方财富均返回；Yahoo 也返回但成交额缺失。新浪返回股数与人民币成交额字段；腾讯按其字段契约保留质量降级原因。各源时间戳不同，不可将抓取时间作为行情源时间，也不可直接视为同一时点进行价差判断。
- **日 K**：新浪不复权、腾讯不复权/前复权、东方财富前复权、BaoStock 前复权和 Yahoo 前复权均返回样本。新浪、腾讯、Yahoo 的成交额缺失保持 null；未核验的金额不能通过价格乘成交量补造。网易历史 CSV 曾返回 HTTP 502，因此不能视为已验收；当前配置仍启用为后置回退，目标部署若不希望使用可设置 `MARKET_DATA_NETEASE_DAILY_BARS_ENABLED=false`。
- **东方财富行业/概念榜、核心指数、个股板块**：本轮均返回数据；多个 push2 端点出现 302 并跳转至 push2delay 后得到 200。榜单和指数因源端快照时间不可核验而降级；个股板块返回 26 项，同样降级。相关 502/302 与请求诊断仍统一追踪于 [Issue #6](https://github.com/Kasign/stock_data_provider/issues/6)。
- **市场广度**：返回 5,377 个计数记录，其中上涨 4,406、下跌 854、平盘 117；但证券全集未与证券主数据核对、源端快照时间缺失，且该端点不提供市场成交额，输出 degraded。不得仅因覆盖计数等于供应商报告总数就宣称完整全市场覆盖。
- **涨跌停/炸板池**：本轮返回涨停池、跌停池、炸板池；上游报告总数依次为 82、6、12，涨停池样本只返回 30 项。各项缺少源端快照时间；默认仍关闭。
- **股票基础资料**：600036.SH 返回简称、上市日期及上市状态，quality_status=valid。只证明该股票的基础身份字段可读，不代表全市场证券主数据完整。
- **BaoStock 交易日历**：返回 7 天窗口，说明本次 Runner 查询成功；此前仍有网络超时记录，且没有替代目标 NAS/Docker 验收。财务指标 Provider 未包含在本次 live probe 的报告结果中，不能将其记为线上探测通过。
- **妙想 MX_API**：筹码分布和资金流因没有显式开启 live tests / MX_APIKEY 而 skipped，不是通过，也不一定表示 Provider 失败。

所有未通过目标部署验收的新增能力继续保持独立开关默认关闭。此报告只构成一次 GitHub 托管 Runner 实测，不代表生产 SLA 或所有交易日、证券、分页的完整性证明。


- **龙虎榜历史记录（2026-10-23）**：当时首次完成 `DragonTigerProvider`、东方财富数据中心 `RPT_DAILYBILLBOARD_DETAILSNEW` 与 API 接入，历史配置默认关闭。当前分支已在 `configs/providers.json` 和服务环境模板中默认启用对应能力，但 Runner / NAS / Docker 端到端验收仍未完成；源端报告仅含交易日、无精确快照时间，空列表为 `unknown`，金额缺失保留 `null`。

## 下一批数据域优先级更新（2026-10-23）

1. 龙虎榜进入 Go 原生接入批次，但在 GitHub Actions 真实接口探测与 NAS/Docker 实测之前不生产启用。
2. 东方财富行业/概念榜在 Runner #101 再次遇到 HTTP 502。本批切换到统一有限重试（至多 3 次、带退避），并添加 HTTP 502 后恢复的 httptest 回归测试；连续 502 仍应记为真实源失败。
3. 后续补齐公告/新闻，先对照 Python 入口、发布时间/公告日期、去重 ID、分页覆盖与源链接，避免重复实现。海外源保持暂停。


## 龙虎榜首次真实端点响应与探测日期修正（2026-10-26）

Live Provider Probe #104 的内部探测步骤失败于龙虎榜：请求实际返回 HTTP 200，但 JSON 为 `{"result":null,"success":false,"message":"返回数据为空","code":9201}`。这次请求日是 2026-10-25（星期日），不是有效交易日；不能把它计为接口稳定性失败，也不能将该空响应解读为已确认零上榜。Provider 现在将明确的 9201 空报表保留为 `quality_status=unknown`，Runner 改为从 BaoStock 交易日历选择最近一个已结束的交易日后再测；新逻辑待下一轮真实 Runner 验收。


## 龙虎榜证券范围与分页质量计数（2026-10-26）

Live Provider Probe #107 请求 2026-10-23 的真实东方财富龙虎榜后，发现供应商报表的第 48 条为 `113711.SH`、`SECURITY_TYPE_CODE=060`。东方财富债券页面明确列出 113711 为常青转债（可转债），而龙虎榜页面会在“全部”视图中混合显示个股和可转债；因此不能将该记录强行标准化为 A 股普通股票（[东方财富常青转债页面](https://quote.eastmoney.com/bond/sh113711.html)，[东方财富龙虎榜说明页](https://datapc.eastmoney.com/emdatacenter/ranking/index)）。

现在 API 将 `total/pages` 解释为供应商原始报表计数，另行返回 `source_returned`（本页原始行数）、`returned`（过滤后符合 A 股股票契约的本页行数）、`excluded_non_a_share_count`（已识别并排除的非 A 股证券数）。三者必须满足 `source_returned = returned + excluded_non_a_share_count`。已识别可转债会加入 `provider_non_a_share_security_rows_excluded` 质量原因；若某页只有被排除的非 A 股证券，返回 unknown 空页及保留的 has_more 分页信息，不得把它说成当日全市场没有股票上榜。该修复等待当前 HEAD 的 Go CI 和新一轮真实 Runner 结果。


| 单只 A 股公告列表 | Go 原生 `AnnouncementProvider`、东方财富 `/api/security/ann` 及 `GET /api/v1/stocks/{symbol}/announcements` | GitHub Go CI #304 与真实 Provider Probe #114 通过。`600036.SH` 在 2026-10-19 至 2026-10-25 返回 `total_hits=2`、1 页、2 条记录；源端 `display_time` 的冒号毫秒格式已正确解析为 `published_at`，本轮样本质量为 valid。当前 `configs/providers.json` 与服务模板默认开启该能力；NAS/Docker 目标部署验收仍未完成 |

## 公告域和通用新闻搜索的迁移边界（2026-10-26）

- Go 原生公告 Provider 只覆盖东方财富单只股票公告列表，不代表通用新闻搜索已迁移。
- Go 原生新闻链现已支持 Bocha/Tavily/Brave News/显式 SearXNG 与股票公告源；已配置的远程源并发聚合，统一应用日期门禁、URL/标题去重、可信域名排序、营销过滤与股票相关性权重。原 Python bridge 仍作为无可用原生结果时的兼容 fallback，不再是默认唯一搜索路径。
- Go-core 已从最近 10 个完整交易日的原生龙虎榜分页结果派生 `is_on_list/recent_count/latest_date`，并将覆盖质量传入旧 analyzer context；语义边界与降级标记已实现，仍需在统一验收阶段用真实数据与旧 Python 输出对账。


## 公告发布时间毫秒格式的真实 Runner 修正（2026-10-26）

Live Provider Probe #112（[run 40258376973](https://github.com/Kasign/stock_data_provider/actions/runs/40258376973)）对东方财富公告端点完成 HTTP 200 请求，返回 600036.SH 在 2026-10-19 至 2026-10-25 区间的 2 条公告，源端 `total_hits=2`、第一页 `returned=2`。实际 `display_time` 值包含 `2026-10-23 17:20:42:715` 和 `2026-10-20 17:32:45:870`，其毫秒使用冒号分隔；旧解析器因此把真实源端发布时间误判为缺失。Provider 现在在严格识别“末尾三位数字”的情况下，将该冒号规范化为小数点再解析，不改变 API 返回的原始 `display_time`；纯日期仍然不会被伪造为发布时间。该修正待新的 Go CI 与真实 Runner 再次验收。


## 国内优先的 Go 原生新闻搜索第一阶段（2026-10-26）

- Go 核心新增可配置的国内优先搜索链：Bocha API 优先，自建 SearXNG 为后备；凭据沿用 `BOCHA_API_KEYS` 和 `SEARXNG_BASE_URLS`。公共 SearXNG 自动发现不启用，避免将随机公开实例当作可靠生产依赖。
- 原生搜索按 `NEWS_STRATEGY_PROFILE`（ultra_short/short/medium/long = 1/3/7/30 天）与 `NEWS_MAX_AGE_DAYS` 的较小值筛选。来源发布时间无法解析、超出时效、无标题/无有效 HTTP(S) URL 的记录不会作为有效新闻返回；URL 与标题去重后按个股直接相关程度排序。
- 如果未配置原生源，或者所有源都没有符合时效与数据契约的结果，继续使用原 Python Bridge 搜索。这样可以先切流验证，而不把新增代码等同于已经完全去 Python 化。
- 本阶段只提供国内个股/市场新闻的首批原生采集；没有完整移植 Python 的垃圾信息过滤、官方媒体白名单、复杂语言排序和多维情报检索策略。Go CI / Runner 结果通过前，不把该阶段标为生产验收完成。


## 原生新闻搜索真实探测开关（2026-10-26）

`.github/workflows/live-provider-smoke.yml` 增加了 Go 原生新闻搜索的独立探测步骤：将 GitHub Secrets `BOCHA_API_KEYS` 与/或 `SEARXNG_BASE_URLS` 注入 Runner，实际查询贵州茅台近期期新闻并上传 `live-news-report` artifact。没有配置凭据时报告状态为 `skipped`，不会误报为接口通过。该探测与 market-data 的行情/龙虎榜/公告探测分开；即使搜索请求通过，也只代表当前配置源的单个查询窗口，不代表新闻覆盖率或长期可用性。


### 同花顺三类涨跌停池补齐（2026-10-26）

- `LimitPoolProvider` 的同花顺实现已覆盖涨停、跌停和炸板三个端点，不再将 `down/broken` 当成“不支持”而直接回退。
- 三种池共用 `GET /api/v1/market/limit-pools` 入口，分别保留涨跌幅、最新价、触及时间、开板次数、换手率等适用字段；只请求第一页，避免在公共接口上执行大范围分页扫描。
- 金额字段继续按供应商定义单位输出，不能误认为人民币元；上游时间戳不足或字段缺失时保留 `degraded` 质量标记。代码已更新，测试和真实端点验收留待统一执行。


### Tushare 国内指数链补充（2026-10-26，待统一验收）

- 新增 Tushare `IndexCatalogProvider`：`index_basic` 返回官方指数目录，独立使用 `tag=tushare`，不把 Tushare 市场目录误标为同花顺概念/行业目录。
- 新增当前成分：`index_weight` 选择最近四个月内可用的最新权重日期，再批量查询 `stock_basic` 补足证券名称，避免为每个成分发出独立请求。它是最近一次权重快照，不代表实时或历史任意时点成分。
- 新增指数日线：`index_daily` 支持最长十年日期区间；成交量保留“手”单位，成交额按接口的千元人民币口径转成人民币元。
- 为区分不同指数代码体系，目录验证接受 Tushare 的 `.CSI` 后缀；个股证券与指数仍使用独立路由。


### 雪球热度榜备用源（2026-10-26，待统一验收）

新增独立 `Xueqiu` `HotRankingProvider`，作为东方财富和可选同花顺之后的最后回退源。人气榜使用雪球 `/v5/stock/hot_stock/list.json?type=10`；飙升榜使用雪球 `/v5/stock/screener/quote/list.json` 的全市场涨幅排序。这是可用性折中，不等价于东方财富的飙升榜，所以响应明确携带降级原因。雪球可能要求有效登录 Cookie；该变量默认留空、默认关闭，不记录 Cookie，也不在代码仓库保存真实值。


### 雪球关注榜 API 补齐（2026-10-26，待统一验收）

现有 `GET /api/v1/market/hot-stocks` 新增 `type=followers`。雪球 `hot_stock/list.json?type=12` 对应关注榜；东方财富和同花顺不支持此类型时由既有 Provider 链继续回退，最终由启用的 Xueqiu Provider 响应。该榜单只表示关注排名，不与人气或涨幅信号混用。


## Go 原生新闻搜索多源聚合第二阶段（2026-10-27）

- Bocha、Tavily News、Brave News 和显式配置的 SearXNG 实现统一 remoteProvider 契约；API Key 只从环境变量读取，Tavily/Brave 未配置 key 时不注册。
- 已配置搜索源与单股公告在个股搜索中并行请求，远程结果按注册顺序合并，公告源优先保留为去重后的规范链接。每个源失败会单独记录，不会直接丢弃其他源成功结果。
- Brave 使用中国地区与简体中文偏好；只使用源端 published_date 或 page_age，不使用 page_fetched 推导文章发布时间。Tavily 请求新闻 topic 并要求返回源端发布日期。
- 这批迁移 Provider 接口和聚合、质量门禁逻辑；不宣称与 Python 的多维 search_comprehensive_intel(max_searches=5) 完全等价。Python fallback 保留至后续核实调用覆盖率与真实部署表现。
- 回归测试使用本地 HTTP fixture，尚未执行测试、构建、CI 或真实上游请求。


## Go 原生新闻搜索多维查询第三阶段（2026-10-27）

- 新增 NEWS_MAX_SEARCHES 配置。Go-core 生产环境通过 NewFromEnv 默认最多执行 5 个查询维度；直接构造 Config 时默认只执行 1 个查询，上限固定为 5。
- 个股维度：通用消息、业绩/财报、监管/处罚、合同/业务、股东/持股；市场维度：通用市场新闻、宏观政策、指数/成交、IPO/并购、行业/产业政策。
- 查询维度在多源之间轮转分配，减少调用成本；所有任务并行运行，结果保留 dimension 字段。公告候选使用 announcement 维度，并在同标题去重时优先保留公告源链接。
- 新增五维查询与标签保留的本地 HTTP fixture 测试源码，未运行测试、构建、CI 或真实上游请求。
- 这仍不等于 Python 的全部搜索策略完全等价；Python bridge fallback 暂时保留至统一验收和调用覆盖率核实完成。


## 个股龙虎榜上下文摘要迁移（2026-10-27）

- Go-core 从最近 10 个完整交易日的 Go 原生龙虎榜分页 API 派生个股摘要；每日期最多读取 10 页、每页 500 条，日期请求最多 4 路并发。
- `is_on_list` 仅在最新参考交易日有完整非空榜单时给出 true/false；榜单空/unknown 或分页不完整时保留 null 或标记 `recent_count_is_lower_bound=true`，不把未知解释成未上榜。
- `recent_count`、`latest_date`、参考交易日、覆盖日数、来源、质量状态与原因经 StockBundle / AnalysisContext / legacy analyzer context 传递。
- 新增客户端本地 HTTP fixture 与上下文桥接测试源码；没有运行测试、构建、CI 或真实上游请求。


## 2026-10-27 — Go CI 阻塞已修复，龙虎榜上下文新增代码待验收

- Eastmoney struct tag 与之前列出的 gofmt 问题已修正。新闻多源测试显式配置 `MaxSearches=2`，确保两个远程 Provider 都被本地 fixture 验证；生产环境的查询上限逻辑不变。
- 提交 `d551057` 的 Go 全量工作流 [#40449084361](https://github.com/Kasign/stock_data_provider/actions/runs/40449084361) 通过，包含测试、race、vet、Compose 配置和两个 Go 镜像构建；[Live Provider Probe #40449084825](https://github.com/Kasign/stock_data_provider/actions/runs/40449084825) 同样通过。
- 后续 HEAD `24952dd` / `71993b1` 新增龙虎榜上下文摘要，相关 Go 客户端、domain、contextpack 与测试代码需要按最新 HEAD 重新验收。不能将上一提交的 CI 结果外推到这些新增代码。


## 个股 profile 消费链路迁移（2026-10-27）

- Go-core 新增结构化 StockProfile，消费现有 `/api/v1/stocks/{symbol}/profile`，保留证券身份、上市日期/状态、动态 PE/PB、行业、市值/流通市值、板块编号、ROE、净利率、净利润、毛利率与来源/质量字段。
- profile 与基本面、板块、筹码、资金流、龙虎榜并发获取；某个辅助域失败只产生日志 warning，不覆盖或丢弃行情与日 K。
- StockProfile 经 AnalysisContext 传给 legacy analyzer 的 `stock_profile_context`、`stock_profile`、`stock_info` 与 `enhanced_data`。由于来源接口只提供当前快照，显式历史 `as_of` 请求不会附加 profile。
- 新增 profile 客户端本地 HTTP fixture 与 context bridge 测试源码；未运行测试、构建、CI 或真实上游请求。
