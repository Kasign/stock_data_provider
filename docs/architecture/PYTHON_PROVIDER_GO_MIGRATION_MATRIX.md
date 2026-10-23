# Python 数据源到 Go 的迁移矩阵

> 本文按上游 `ZhuLinsen/daily_stock_analysis/data_provider/base.py` 的 `DataFetcherManager._init_default_fetchers`、核心数据方法以及 `src/market_analyzer.py` 中的市场概览调用整理。该原始 Python 目录未完整复制到当前仓库，因此每个未迁移项目都必须以该上游源码为准逐项对照；本表不是“已迁移”的声明。

本矩阵与 [数据源扩展计划](./DATA_SOURCE_EXPANSION_PLAN.md) 配套维护；该计划优先按能力与真实验收状态安排新源，不将联网测试通过等同于生产接入完成。

## 1. 数据源注册清单

| 原 Python 数据源 | 原注册条件 / 默认位置 | 主要市场或用途 | 当前 Go 对应 | 状态 |
|---|---|---|---|---|
| EfinanceFetcher | 默认启用；默认优先级 P0 | A 股行情、日线及部分市场统计/排行榜能力 | 无 | 未迁移 |
| AkshareFetcher | 默认启用；默认 P1 | A 股行情、日线、筹码/财务/板块等按方法覆盖能力 | 无 | 未迁移 |
| PytdxFetcher | 默认启用；默认 P2 | A 股实时行情、历史行情 | 无 | 未迁移 |
| BaostockFetcher | 原 Python 默认 P3；Go 按历史 K 线优先候选 | A 股历史日线优先；财务、复权因子、除权除息、基础资料/行业/指数成分按 API 能力逐项扩展 | Go 原生 TCP 客户端与 DailyBarProvider 已实现；NAS Docker 容器已真实登录/查询，并覆盖多个证券及前复权、后复权、不复权样本；GitHub Runner #100 又返回 8 根样本日 K | 日 K 已有目标容器首轮实测，可作为受限回退源；仍需持续观察稳定性和更多证券/日期范围，不能外推为全覆盖或 SLA；财务指标、交易日历独立开关仍默认关闭 |
| YfinanceFetcher | 默认启用；默认 P4；海外股票与指数的专用路径 | 美股 / 海外行情与历史数据、部分基本面数据 | 无 | 未迁移 |
| TencentFetcher | 默认启用；默认 P5 | A 股行情与日线兜底 | 原生 Go Tencent Provider | 已迁移部分行情能力，口径仍需逐项对照 |
| TushareFetcher | 仅配置 `TUSHARE_TOKEN` 后注册；初始化成功后可提升到高优先级 | A 股行情、市场统计、财务及其他 Token 授权数据 | 无 | 可选源，未迁移 |
| TickFlowFetcher | 仅配置 `TICKFLOW_API_KEY` 后注册；优先级可配置，默认 2 | 批量日线/行情以及基于 universe quote 计算的市场涨跌统计 | 无 | 可选源，未迁移 |
| MiaoxiangFetcher（妙想 / MX_API） | 仅配置 `MX_APIKEY` 后注册；默认优先级 6 | 筹码分布及个股资金流等第三方补充数据 | Go `MiaoXiang`（独立 SupplementProvider，显式 opt-in） | 已迁移首批筹码/资金流字段；尚缺真实 MX Key 联调与精确 publication availability |
| LongbridgeFetcher | 仅配置 Longbridge OAuth / 旧凭据后注册 | 美股 / 港股行情及日线兜底 | 无 | 可选源，未迁移 |
| FutuFetcher | 检测到 Futu OpenD 端点配置后注册 | 港股等 Futu 支持市场 | 无 | 可选源，未迁移 |
| FinnhubFetcher | 仅配置 `FINNHUB_API_KEY` 后注册 | 以美股/海外股票为主的补充行情和历史数据 | 无 | 可选源，未迁移 |
| AlphaVantageFetcher | 仅配置 `ALPHAVANTAGE_API_KEY` 后注册 | 以美股/海外股票为主的补充行情和历史数据 | 无 | 可选源，未迁移 |
| 同花顺 / THS Financial-API | 拟单独配置 API Key 后注册；当前未接入 | 最新行情、日线、指数/板块、财务、交易日历、特色涨跌停/龙虎榜等，必须按官方端点和账户权限逐项验证 | 参考 `Kasign/Financial-API`，尚未实现 | 计划中，不可作为当前可用数据源 |
| Akshare / YFinance fundamental adapters | 由基本面上下文聚合逻辑调用，不完全等同于行情 Fetcher 列表 | 财务与基本面指标 | 无 | 未迁移 |

原 Python 管理器会按注册的 Fetcher 优先级排序，并针对市场类型、数据能力、可用性和失败状态采用不同的路由 / fallback 规则。因此，简单地把这张表映射成一个全局 Provider 顺序并不足以复现原行为。

## 2. 按数据能力的当前覆盖

| 数据能力 / 原管理器入口 | 当前 Go 实现 | 当前缺口 |
|---|---|---|
| 个股实时行情 `get_realtime_quote` | Eastmoney、Tencent、Sina；Yahoo 为显式关闭的可选源 | GitHub Runner #100 中新浪、腾讯、东方财富均返回报价；腾讯和 Yahoo 因字段/成交额契约限制为 degraded，Yahoo 默认关闭；仍需逐项对照原 Fetcher 的字段补齐与市场路由 |
| 日线历史 `get_daily_data` | Eastmoney、Tencent、Sina、BaoStock；Yahoo、网易已实现但默认关闭 | GitHub Runner #100 对 Eastmoney、Tencent（none/qfq）、Sina（none）、BaoStock（qfq）和 Yahoo（qfq）各返回 8 根样本 K 线；新浪/腾讯/Yahoo 缺可核验成交额时保持 null/unknown/degraded；网易探测本次因 HTTP 502 被跳过；仍未迁移 Efinance / AkShare / Pytdx / Tushare / TickFlow / Longbridge 等来源、指数专用链和市场路由 |
| 指数实时报价 | 新增独立 `IndexQuoteProvider`、东方财富 `ulist.np/get` 适配器与 `GET /api/v1/market/indices`，仅覆盖 7 个白名单核心指数 | GitHub Runner #100 返回 7 条；接口发生 302 后跳转至 delay 域名并返回 200，源端未提供可解析行情时间，输出 degraded；默认关闭，目标部署容器仍待验收 |
| 指数日线 | 现有底层 Provider 可能支持部分指数 K 线，但完整指数专用路由未迁移 | 原 Python 对裸指数代码、带交易所后缀的指数代码和各市场有独立识别/路由逻辑，尚未完成逐项对照 |
| 市场涨跌统计 get_market_stats | 新增 MarketBreadthProvider、东方财富 getTopicZDFenBu 汇总接口及 GET /api/v1/market/breadth | GitHub Runner #100 返回 5,377 个计数记录（上涨 4,406、下跌 854、平盘 117），但证券全集未经证券主数据校验、成交额不可用、无源端快照时间，质量仍为 degraded；默认关闭。单次成功不消除间歇性 502 风险，不得将该统计宣称为已完整覆盖全 A 股 |
| 行业板块涨跌榜 `get_sector_rankings` | 新增独立 `SectorRankingProvider`、东方财富 CLIST 适配器及 `GET /api/v1/market/sectors?type=industry` | GitHub Runner 复验返回 10 条，Go 测试/镜像 CI 已通过；上游未提供可核验快照时间，结果固定 `degraded`；默认关闭，目标部署环境待验收 |
| 概念 / 题材涨跌榜 `get_concept_rankings` | 复用板块 ranking 契约、使用独立概念过滤条件；API 为 `GET /api/v1/market/sectors?type=concept` | GitHub Runner 复验返回 10 条，Go 测试/镜像 CI 已通过；上游未提供可核验快照时间，结果固定 `degraded`；默认关闭，目标部署环境待验收 |
| 涨停池 get_limit_up_pool、跌停池、炸板池 | 新增 LimitPoolProvider、东方财富 push2ex 三类池端点及 GET /api/v1/market/limit-pools?type=up|down|broken | GitHub Runner #100 三类端点均返回 200，报告总数分别为涨停 82、跌停 6、炸板 12（涨停池本次只返回 30 条样本项）；请求交易日为 2026-10-23，源端没有可核验快照时间，质量 degraded；默认关闭，NAS/Docker 与跨交易日语义验证未完成 |
| 个股所属板块 `get_belong_boards` | 新增独立 `StockBoardsProvider`、东方财富 `slist/get` 适配器与 `GET /api/v1/stocks/{symbol}/boards` | GitHub Runner #100 对 600036.SH 返回 26 个板块项；302 → delay 域名 200；源端没有可核验快照时间且板块类型语义仍有限，结果 degraded；默认关闭，NAS/Docker 验收待完成 |
| 交易日历 | 新增独立 `TradingCalendarProvider`，通过 BaoStock TCP `query_trade_dates` 和 `GET /api/v1/market/trading-calendar?start=YYYY-MM-DD&end=YYYY-MM-DD` | GitHub Runner #100 成功返回 2026-10-17 至 2026-10-23 的 7 个日历日；此前 Runner 有连接/读取超时记录，且源端时间元数据缺失；默认关闭，需多轮重试及 NAS/Docker 验收 |
| 筹码分布 `get_chip_distribution` | 可选 Go MX API 路由和缓存 | 精确 publication available_at 仍缺失；输出降级质量，历史 as_of 不得倒灌 |
| 基本面 `get_fundamental_context` | 新增 BaoStock 原生 TCP 季频指标 Provider 与 `GET /api/v1/stocks/{symbol}/fundamentals?year=YYYY&quarter=N`，覆盖盈利、营运、成长、偿债、现金流、杜邦六组指标 | 指标层首批迁移，不是完整财务报表或原 Python fundamental context 的替代；报告期与披露日期独立输出，保留原始值和单位，缺失不补零；默认关闭，真实 Runner 与 NAS/Docker 验收待完成 |
| 个股资金流 `get_capital_flow_context` | 可选 Go MX API 路由；字段按人民币元规范化，5/10 日累计要求完整观测 | 自然语言查询返回的指标需要验证字段标签/单位；缺值不补零，精确 available_at 未实现 |
| 龙虎榜 `get_dragon_tiger_context` | 无 | 必须保留交易日、上榜原因、营业部/机构口径及金额单位 |
| 股票名称 / 基础资料 `get_stock_name` / profile | 新增独立 `StockBasicProvider` 和 `GET /api/v1/stocks/{symbol}/profile`，接入东方财富 F10 发行信息（简称、上市日期、上市状态） | 单证券基础身份信息已在 GitHub Runner HTTP 200 实测通过；不等于批量名称或完整公司基本面，目标部署容器仍待验收 |
| 新闻 / 公告 | Go 分析服务仍通过 Python bridge 的新闻搜索 / 能力接口 | 尚未建立 Go 原生搜索、公告数据契约与来源/发布时间/抓取时间模型 |

当前原生 Go API 提供 `GET /api/v1/quotes/{symbol}`、`GET /api/v1/bars/{symbol}/daily`、`GET /api/v1/bundles/{symbol}`、`GET /api/v1/data-sources`、`GET /api/v1/stocks/{symbol}/profile`、`GET /api/v1/stocks/{symbol}/boards`、`GET /api/v1/market/sectors?type=industry|concept`、`GET /api/v1/market/indices`、`GET /api/v1/market/breadth`、`GET /api/v1/market/limit-pools?type=up|down|broken`、`GET /api/v1/market/trading-calendar?start=YYYY-MM-DD&end=YYYY-MM-DD`，并新增可选的 `GET /api/v1/stocks/{symbol}/chip-distribution` 与 `GET /api/v1/stocks/{symbol}/capital-flow`。这些新增能力保持独立路由/开关；未通过 NAS/Docker 验收的能力默认关闭。

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
- **日 K**：新浪不复权、腾讯不复权/前复权、东方财富前复权、BaoStock 前复权和 Yahoo 前复权均返回样本。新浪、腾讯、Yahoo 的成交额缺失保持 null；未核验的金额不能通过价格乘成交量补造。网易本轮探测因 HTTP 502 被跳过，当前 Provider 默认关闭。
- **东方财富行业/概念榜、核心指数、个股板块**：本轮均返回数据；多个 push2 端点出现 302 并跳转至 push2delay 后得到 200。榜单和指数因源端快照时间不可核验而降级；个股板块返回 26 项，同样降级。相关 502/302 与请求诊断仍统一追踪于 [Issue #6](https://github.com/Kasign/stock_data_provider/issues/6)。
- **市场广度**：返回 5,377 个计数记录，其中上涨 4,406、下跌 854、平盘 117；但证券全集未与证券主数据核对、源端快照时间缺失，且该端点不提供市场成交额，输出 degraded。不得仅因覆盖计数等于供应商报告总数就宣称完整全市场覆盖。
- **涨跌停/炸板池**：本轮返回涨停池、跌停池、炸板池；上游报告总数依次为 82、6、12，涨停池样本只返回 30 项。各项缺少源端快照时间；默认仍关闭。
- **股票基础资料**：600036.SH 返回简称、上市日期及上市状态，quality_status=valid。只证明该股票的基础身份字段可读，不代表全市场证券主数据完整。
- **BaoStock 交易日历**：返回 7 天窗口，说明本次 Runner 查询成功；此前仍有网络超时记录，且没有替代目标 NAS/Docker 验收。财务指标 Provider 未包含在本次 live probe 的报告结果中，不能将其记为线上探测通过。
- **妙想 MX_API**：筹码分布和资金流因没有显式开启 live tests / MX_APIKEY 而 skipped，不是通过，也不一定表示 Provider 失败。

所有未通过目标部署验收的新增能力继续保持独立开关默认关闭。此报告只构成一次 GitHub 托管 Runner 实测，不代表生产 SLA 或所有交易日、证券、分页的完整性证明。
