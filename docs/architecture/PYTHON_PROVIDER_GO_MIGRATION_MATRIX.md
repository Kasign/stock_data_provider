# Python 数据源到 Go 的迁移矩阵

> 本文按上游 `ZhuLinsen/daily_stock_analysis/data_provider/base.py` 的 `DataFetcherManager._init_default_fetchers`、核心数据方法以及 `src/market_analyzer.py` 中的市场概览调用整理。该原始 Python 目录未完整复制到当前仓库，因此每个未迁移项目都必须以该上游源码为准逐项对照；本表不是“已迁移”的声明。

本矩阵与 [数据源扩展计划](./DATA_SOURCE_EXPANSION_PLAN.md) 配套维护；该计划优先按能力与真实验收状态安排新源，不将联网测试通过等同于生产接入完成。

## 1. 数据源注册清单

| 原 Python 数据源 | 原注册条件 / 默认位置 | 主要市场或用途 | 当前 Go 对应 | 状态 |
|---|---|---|---|---|
| EfinanceFetcher | 默认启用；默认优先级 P0 | A 股行情、日线及部分市场统计/排行榜能力 | 无 | 未迁移 |
| AkshareFetcher | 默认启用；默认 P1 | A 股行情、日线、筹码/财务/板块等按方法覆盖能力 | 无 | 未迁移 |
| PytdxFetcher | 默认启用；默认 P2 | A 股实时行情、历史行情 | 无 | 未迁移 |
| BaostockFetcher | 原 Python 默认 P3；Go 按历史 K 线优先候选 | A 股历史日线优先；财务、复权因子、除权除息、基础资料/行业/指数成分按 API 能力逐项扩展 | Go 原生 TCP 客户端与 DailyBarProvider 已实现，默认关闭；NAS 宿主机 DNS/TCP 通过，容器内真实登录/查询未验收 | 代码已实现，生产验收未完成；不可默认启用 |
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
| 个股实时行情 `get_realtime_quote` | Eastmoney、Tencent、Sina（仅报价） | Sina 不提供日 K；原 Fetcher 的字段补齐、市场路由和所有可选源尚未复刻 |
| 日线历史 `get_daily_data` | Eastmoney、Tencent | 尚未迁移 Efinance / AkShare / Pytdx / Baostock / Tushare / TickFlow / YFinance / Longbridge 等来源、指数专用链和市场路由 |
| 指数日线 / 指数实时报价 | 有少量底层 Provider 能处理的场景，但完整指数专用路由未迁移 | 原 Python 对裸指数代码、带交易所后缀的指数代码和各市场有独立识别/路由逻辑 |
| 市场涨跌统计 `get_market_stats` | 无 | 上涨、下跌、平盘、涨停、跌停、总成交额必须来自同一市场范围、同一数据时点；不得用自选股横截面伪装全市场统计 |
| 行业板块涨跌榜 `get_sector_rankings` | 无 | 须实现板块识别、排名口径和数据时点 |
| 概念 / 题材涨跌榜 `get_concept_rankings` | 无 | 须与行业板块分开建模 |
| 涨停池 `get_limit_up_pool` | 无 | 需要交易日、涨停判定规则、ST / 创业板 / 科创板 / 北交所差异及来源说明 |
| 个股所属板块 `get_belong_boards` | 无 | 需要个股-板块关系和类型元数据 |
| 筹码分布 `get_chip_distribution` | 可选 Go MX API 路由和缓存 | 精确 publication available_at 仍缺失；输出降级质量，历史 as_of 不得倒灌 |
| 基本面 `get_fundamental_context` | 无 | 必须区分报告期、披露日、财务指标单位、来源、缺失字段和适用市场 |
| 个股资金流 `get_capital_flow_context` | 可选 Go MX API 路由；字段按人民币元规范化，5/10 日累计要求完整观测 | 自然语言查询返回的指标需要验证字段标签/单位；缺值不补零，精确 available_at 未实现 |
| 龙虎榜 `get_dragon_tiger_context` | 无 | 必须保留交易日、上榜原因、营业部/机构口径及金额单位 |
| 股票名称 / 批量名称 `get_stock_name` / `batch_get_stock_names` | 仅有报价中的名称字段 | 缺少批量基础资料 Provider 与名称数据来源、更新时间管理 |
| 新闻 / 公告 | Go 分析服务仍通过 Python bridge 的新闻搜索 / 能力接口 | 尚未建立 Go 原生搜索、公告数据契约与来源/发布时间/抓取时间模型 |

当前原生 Go API 提供 `GET /api/v1/quotes/{symbol}`、`GET /api/v1/bars/{symbol}/daily`、`GET /api/v1/bundles/{symbol}`、`GET /api/v1/data-sources`，并新增可选的 `GET /api/v1/stocks/{symbol}/chip-distribution` 与 `GET /api/v1/stocks/{symbol}/capital-flow`。后两项只在 `MARKET_DATA_MX_ENABLED=true` 且 MX API Key 配置有效时启用。这不代表其他数据域已经切换到 Go。

## 3. Go Provider 设计要求

每一个新源应实现与能力相匹配的接口，而不是强迫所有源都实现同一组方法：

- `NamedProvider`：名称、版本和健康状态；
- `QuoteProvider`：实时行情；
- `DailyBarProvider`：历史 K 线；
- 后续新增明确独立的能力接口：`IndexProvider`、`MarketBreadthProvider`、`SectorRankingProvider`、`ChipDistributionProvider`、`FundamentalsProvider`、`CapitalFlowProvider`、`DragonTigerProvider`、`LimitUpPoolProvider`、`NewsProvider`。

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
