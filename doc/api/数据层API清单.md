# 数据层 API 接口清单与 WebUI 接入映射

更新日期：2026-11-03
适用分支：feat/full-go-data-providers-api-switch（已包含 feat/webui-market-data-console 的 WebUI 合并内容）
核对依据：当前目标分支 services/market-data/openapi.yaml、services/market-data/internal/httpapi/server.go、web/src/api.js 及页面模块。服务端路由与 OpenAPI 已逐项核对。字段类型、完整响应 Schema 和状态码以 OpenAPI 为准。

说明：接口已注册/已封装不代表 Provider 已启用或真实数据已验收。Provider 代码能力、默认开关和运行验收状态见 [Python Provider → Go 迁移矩阵](./接口规范/PYTHON_PROVIDER_GO_MIGRATION_MATRIX.md) 与 [数据源真实环境验收记录](./接口规范/DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md)。

## 1. 接口边界

- 行情数据服务：浏览器默认代理前缀 /market-api。
- 分析任务服务：浏览器默认代理前缀 /analysis-api，与行情服务分离。
- 行情接口除健康检查外均使用 X-API-Key；该 Key 是行情服务内部访问凭据，保存在当前浏览器连接设置中。
- 分析任务请求不复用行情 API Key。管理员登录态、服务密钥和行情服务 API Key 不应混用。
- “前端封装”表示 web/src/api.js 存在对应请求函数，不代表当前部署的 Provider 数据一定可用。

## 2. 行情数据服务接口

以下路径均相对于 /market-api。接口方法以表格为准；健康检查为公开 GET，行情数据接口需要 API Key，主动探测使用 POST。

| 方法 | 路径 | 功能 | 参数摘要 | WebUI 封装 | API Key |
|---|---|---|---|---|---|
| GET | /health/live | 行情服务存活检查 | 无额外参数；详见 OpenAPI | marketLiveness | 否 |
| GET | /health/ready | 行情服务就绪检查 | 无额外参数；详见 OpenAPI | marketHealth | 否 |
| GET | /api/v1/market/quotes | 批量获取指定股票实时行情（最多 50 只） | symbols：逗号分隔，必填，最多 50 个且不得重复 | getMarketQuotes（已封装；自选股页按需调用） | 是 |
| GET | /api/v1/market/stocks | 全市场 A 股分页行情概览，可选返回 MA5/MA20 | page、page_size（1–100）、include_ma、adjustment | getMarketStocks | 是 |
| GET | /api/v1/quotes/{symbol} | 单只股票实时行情 | 路径参数 symbol | getQuote | 是 |
| GET | /api/v1/bars/{symbol}/daily | 单只股票日 K（支持日期范围、复权及 as_of 历史观察时点） | 路径参数 symbol；start、end、limit（1–2000）、adjustment、as_of | getDailyBars | 是 |
| GET | /api/v1/bundles/{symbol} | 单只股票行情与日 K 统一快照 | 路径参数 symbol；lookback_days（20–2000）、adjustment、as_of | getMarketBundle | 是 |
| GET | /api/v1/data-sources/diagnostics | Provider 业务请求熔断诊断（进程内真实请求观测） | 无额外参数；详见 OpenAPI | getDataSourceDiagnostics | 是 |
| POST | /api/v1/data-sources/probe | 主动探测已启用 Provider 的各项能力 | scope（all/core）、checks（connectivity/contract/quality/freshness/cross_provider/storage）、capabilities、markets（最多 1 个）、symbols（最多 1 个标准代码）、timeout_seconds（1–30，默认 12）；请求体可选 | runDataSourceProbe | 是 |
| GET | /api/v1/data-sources/probe/history | 最近已持久化的主动探测批次 | limit（1–100，默认 20）；依赖 MariaDB，存储不可用返回 503 | getDataSourceProbeHistory | 是 |
| GET | /api/v1/data-sources | Provider 注册信息与静态能力声明 | 无额外参数；详见 OpenAPI | getDataSources | 是 |
| GET | /api/v1/stocks/search | A 股证券名称/代码搜索 | q 必填（1–64 字符）；limit（1–50，默认 10） | searchStocks | 是 |
| GET | /api/v1/stocks/{symbol}/announcements | 单股公告分页 | 路径参数 symbol；start_date、end_date、page、page_size | getStockAnnouncements | 是 |
| GET | /api/v1/stocks/{symbol}/profile | 证券档案、上市信息及动态行情衍生字段 | 无额外参数；详见 OpenAPI | getStockProfile | 是 |
| GET | /api/v1/stocks/{symbol}/price-limit-rule | 指定交易日涨跌幅规则 | 路径参数 symbol；date 必填 | getStockPriceLimitRule | 是 |
| GET | /api/v1/stocks/{symbol}/trading-status | 指定交易日历史交易状态及证据 | 路径参数 symbol；date 必填；as_of 可选 | getStockTradingStatus | 是 |
| GET | /api/v1/stocks/{symbol}/fundamentals | 季度财务指标与三大报表 | 路径参数 symbol；year、quarter 必填；as_of 可选 | getStockFundamentals | 是 |
| GET | /api/v1/stocks/{symbol}/chip-distribution | 单股筹码分布快照 | 无额外参数；详见 OpenAPI | getChipDistribution | 是 |
| GET | /api/v1/stocks/{symbol}/capital-flow | 单股资金流摘要 | 无额外参数；详见 OpenAPI | getCapitalFlow | 是 |
| GET | /api/v1/stocks/{symbol}/boards | 单股所属行业/概念板块 | 无额外参数；详见 OpenAPI | getStockBoards | 是 |
| GET | /api/v1/market/valuations | 最多 100 只股票估值快照 | symbols 必填，最多 100 个代码 | getMarketValuations | 是 |
| GET | /api/v1/market/sectors | 行业/概念板块排行 | type（industry/concept）、direction（up/down）、sort_by（change_percent/amount/change_5d）、limit（1–100）；返回 amount/amount_unit、change_percent_5d、leading_stock 等字段 | getMarketSectors | 是 |
| GET | /api/v1/market/capital-flow | 市场级每日资金流历史序列（Tushare moneyflow_mkt_dc） | start、end 必填；闭区间，最长 2000 个自然日；需启用并配置有权限的 Tushare Token | getMarketCapitalFlow | 是 |
| GET | /api/v1/market/index-catalog | 国内指数/板块目录 | tag（cn_concept/region/tszs/industry/tushare） | getIndexCatalog | 是 |
| GET | /api/v1/market/indices/{symbol}/constituents | 指定指数成分及权重快照 | 路径参数 symbol；source、as_of 可选 | getIndexConstituents | 是 |
| GET | /api/v1/market/indices/{symbol}/daily | 指数/板块历史日线 | 路径参数 symbol；start、end 必填；source 可选 | getIndexDailyBars | 是 |
| GET | /api/v1/market/indices | 核心市场指数行情 | 无额外参数；详见 OpenAPI | getMarketIndices | 是 |
| GET | /api/v1/market/trading-calendar | A 股交易日历 | start、end 必填（闭区间） | getTradingCalendar | 是 |
| GET | /api/v1/market/session | A 股交易时段状态 | at 可选，RFC3339 时间 | getMarketSession | 是 |
| GET | /api/v1/market/breadth | 全市场涨跌分布/市场宽度 | 无额外参数；详见 OpenAPI | getMarketBreadth | 是 |
| GET | /api/v1/market/sentiment | 当前市场情绪快照（聚合市场宽度、涨跌停/炸板等） | 无额外参数；依数据源覆盖情况返回质量状态 | getMarketSentiment | 是 |
| GET | /api/v1/market/sentiment/history | 已持久化的市场情绪历史快照 | start、end、limit；依赖 MariaDB | getMarketSentimentHistory | 是 |
| GET | /api/v1/market/hot-stocks | 个股人气/飙升等榜单 | type、limit（1–100） | getMarketHotStocks | 是 |
| GET | /api/v1/market/dragon-tiger | 龙虎榜盘后分页 | date、page、page_size | getMarketDragonTiger | 是 |
| GET | /api/v1/market/limit-pools | 涨停、跌停、炸板池 | type（up/down/broken）、date、limit（1–100） | getMarketLimitPool | 是 |

## 3. 分析任务服务接口

以下路径相对于 /analysis-api，由分析服务提供：

| 方法 | 路径 | 功能 | WebUI 调用 |
|---|---|---|---|
| GET | /healthz | 存活检查 | analysisLiveness |
| GET | /readyz | 就绪与依赖状态 | analysisHealth |
| POST | /api/v1/analysis/tasks | 创建分析任务；可附加 Idempotency-Key | createAnalysisTask |
| GET | /api/v1/analysis/tasks/{task_id} | 查询任务状态 | getAnalysisTask |
| GET | /api/v1/analysis/tasks/{task_id}/events | 按 after_sequence 续读阶段事件，前端每页最多 100 条 | getAnalysisTaskEvents |
| GET | /api/v1/analysis/runs/{run_id} | 查询分析运行结果 | getAnalysisRun |\n| GET | /api/v1/analysis/tasks | 服务端任务历史分页（当前未提供） | 暂无；需服务端补充分页、状态/类型/股票/日期过滤与总数语义 |

<<<<<<< HEAD:doc/数据层API清单.md
分析任务请求/响应的完整字段契约以 doc/api/分析任务.openapi.yaml 和分析服务当前实现为准。当前没有服务端研究报告列表/详情/归档 API；WebUI 直接使用已校验的 Run 结果在浏览器本地归档，不新增服务端接口。当前 OpenAPI 仅定义创建、单任务状态、事件与 Run 结果查询，没有服务端任务列表分页接口；WebUI 的“最近分析”只保留本地最多 10 条任务引用，不应称为服务端历史。
=======
分析任务请求/响应的完整字段契约以 分析任务.openapi.yaml 和分析服务当前实现为准。
>>>>>>> feat/full-go-data-providers-api-switch:doc/api/数据层API清单.md

## 4. 页面与接口对应关系

| 页面/模块 | 当前调用 | 当前边界 |
|---|---|---|
| 市场总览 | 核心指数、市场宽度、板块排行、涨跌停池、人气榜、市场情绪历史、市场级资金流历史 | 市场情绪历史与市场级资金流历史均已封装并提供页面入口；资金流依赖有权限的 Tushare Provider，需真实部署验收；全球市场联动暂缓 |
| 市场活动 | 龙虎榜、交易日历 | 已接入；空榜/未知质量不能解释为确定的零条 |
| 个股搜索与行情 | 股票搜索、单股行情、日 K、Bundle、数据源清单 | 已接入；行情质量与源端时间必须展示 |
| 个股研究 | 档案、估值、财务、所属板块、公告、筹码分布、个股资金流 | 已封装/按需加载；具体 Provider 能力和数据质量依回包判断 |
| 自选股 | 浏览器本地清单；`getMarketQuotes` 批量刷新最多 50 只；研究列表复用已加载行情 | 已支持真实批量行情刷新和行情字段排序；没有跨设备服务端自选持久化 |
| 数据中心 | WebUI 页面检查、行情 live/ready、分析 healthz/readyz、Provider 静态能力清单 | 不直连数据库；能力声明不是实时健康 |
| Provider 运行诊断与主动探测 | /api/v1/data-sources/diagnostics、/probe、/probe/history | 被动熔断诊断、主动能力探测和持久化历史分开展示；探测历史依赖 MariaDB |
| 选股、告警、通知、问股 | 当前为前端入口/状态能力 | 不能把 UI 状态当作服务端任务、规则或投递记录；后端闭环仍需补齐 |

## 5. 数据层接口的重要约束

1. /api/v1/market/quotes?symbols=... 只查询调用方明确提供的最多 50 只股票，不是全市场扫描；逐项返回失败码，整体质量可降级。
2. WebUI 本地选股只对 `/api/v1/market/stocks` 最近一次成功加载的当前分页执行条件判断；不自动翻页、不宣称全市场扫描。涨跌幅、换手率、成交额取自该页 quote，MA5/MA20 取自该页均线字段；null 按未知处理。
2. /api/v1/data-sources 是 Provider 注册与能力声明；不能证明某个 Provider 当前可请求成功。
3. /api/v1/data-sources/diagnostics 来源于真实 quote/daily_bars 请求形成的进程内熔断器状态，不会主动探测全部 Provider，服务重启后历史计数会重置。
4. 日 K as_of 是系统观察时间点查询，只读取本地记录的历史版本；不是把当前 Provider 数据伪装成历史数据。
5. 指数目录、指数行情、指数成分、指数历史日线是不同数据域，不能互相替代。
6. 市场宽度、涨跌停池、龙虎榜等接口的空数据可能是 unknown/degraded；不得将未知转换为 0 或“无事件”。
7. 行业/概念排行服务端支持 type、direction、sort_by（change_percent/amount/change_5d）和 limit；WebUI 当前已封装并传递 sort_by/direction，页面可切换今日涨幅、成交额和近 5 日涨幅及排序方向。
8. 前端只负责展示服务端单位和质量字段，不猜测成交量/成交额单位，不把成交额当作资金净流入。
9. 行业/概念板块排行的 `SectorRanking.leading_stock` 当前仅为领涨股票名称，接口未提供领涨股证券代码；WebUI 不得将该名称作为代码或通过模糊名称反查证券身份。若要支持板块领涨股直达研究，数据层需新增并在 OpenAPI 明确声明标准 A 股证券代码字段（建议 `leading_stock_symbol`），再由 WebUI 按证券代码格式校验后开放入口。

## 6. 统一字段与错误语义

- 质量状态：valid、degraded、stale、invalid、unknown。
- 行情/日 K 标准单位：价格 CNY/股、成交量股、成交额 CNY；百分比数值以百分点表达，例如 1.25 表示 1.25%。
- 时间字段必须区分源端事件时间、服务端采集时间 fetched_at、标准化时间 normalized_at 和系统观察时间 system_observed_at。
- 错误响应统一包含 error.code、error.message，可包含 request_id；前端应保留 HTTP 状态和请求 ID，错误信息限长展示。
- 复权请求仅接受 none、forward_adjusted、backward_adjusted；unknown 是响应状态，不是请求参数。

## 7. 仍需数据层/产品评估的 API 需求

- 板块资金净流入排行（需核实源字段语义、单位、报告时间和完整候选板块集）；市场级资金流与市场情绪历史已有服务端 API 和 WebUI 接入，仍需真实权限及部署验收。
- 板块多维排行与资金流入排行（需核实接口是否提供相应排序字段）。
- 自选股批量概览（批量 quote 已由 WebUI `getMarketQuotes` 封装并用于自选股刷新；MA/资金等扩展字段仍需明确是否纳入批量接口，不能将单股研究字段当作批量 quote 已返回）。
- 选股规则执行、策略保存/扫描任务/候选结果/历史记录；当前 `/api/v1/market/stocks` 是分页行情，不包含筛选表达式和执行语义。
- 告警规则/评估/触发记录、通知渠道与投递结果。
- Provider 主动探测与逐能力诊断已有独立 API 和 WebUI 接入；仍需在真实部署环境验收 Provider 权限、数据质量对账及 MariaDB 探测历史持久化。
- 全球市场联动数据。

## 8. 权威接口定义

- 行情服务 OpenAPI：services/market-data/openapi.yaml（代码内权威协议，不迁移到文档目录）。
<<<<<<< HEAD:doc/数据层API清单.md
- 分析任务 OpenAPI：doc/api/分析任务.openapi.yaml。
- Python 桥接 OpenAPI：doc/api/Python桥接.openapi.yaml。
=======
- 分析任务 OpenAPI：doc/接口规范/分析任务.openapi.yaml。
- Python 桥接 OpenAPI：Python桥接.openapi.yaml。
>>>>>>> feat/full-go-data-providers-api-switch:doc/api/数据层API清单.md
- WebUI 实际请求实现：web/src/api.js。
- 行情服务路由注册：services/market-data/internal/httpapi/server.go。


### 板块排行字段补充说明（2026-11-03）

- WebUI 使用 `amount` + `amount_unit` 展示成交额；仅 `amount_unit=CNY` 时显示人民币单位，unknown 不推断。
- 五日排序由服务端 `sort_by=change_5d` 执行，返回值字段为 `change_percent_5d`。当前 Go 模型包含该 JSON 字段，但 `services/market-data/openapi.yaml` 的 `SectorRanking` 属性清单尚未声明它，属于 OpenAPI 文档缺口；本分支不修改服务端 OpenAPI，已记录待数据层补齐。
- `direction=up` 表示按所选指标从高到低，`direction=down` 表示从低到高。
- 板块资金净流入没有独立服务端接口，WebUI 继续显示未接入，不以成交额替代。


### 市场观察联动接口核对（2026-11-03）

- `GET /api/v1/market/sentiment/history` 返回已持久化的 `trade_date`，WebUI 用该日期作为联动查询条件。
- `GET /api/v1/market/limit-pools?type=up|down|broken&date=YYYY-MM-DD` 支持显式交易日期。
- `GET /api/v1/market/dragon-tiger?date=YYYY-MM-DD` 支持显式交易日期。
- 以上联动只保证请求日期相同，不保证不同数据源为同一时刻的原子快照，需分别核验各响应质量与源端时间。
- 板块排行的 `code` 为 BK 板块代码，当前 `leading_stock` 只有名称；尚无可靠的板块成分股查询映射，WebUI 不将 BK 代码传给指数成分接口。


### Provider 主动探测字段级响应补充（2026-11-03）

- `POST /api/v1/data-sources/probe` 的 `items[]` 实际由数据侧 `ProviderProbeItem` 返回，除基础状态/质量/时间字段外，还包含 `validation_total_count`、`validation_error_count`、`validation_warning_count`、`validation_info_count`、`validation_truncated`、`validation_issues` 和 `coverage_dimensions`。
- `validation_issues[]` 字段包括 `stage`、`field`、`provider_field`、`code`、`expected`、`actual_type`、`actual_preview`、`severity`、`message`，以及可选的行号、证券代码和交易日期。实际值摘要由服务端脱敏并限长；WebUI 只按文本展示，不解释或重写规则。
- `coverage_dimensions[]` 当前用于市场宽度，维度为 `quote_response`、`price_classification`、`quote_timestamp`、`amount`；ratio 是服务端按明确总体分母计算的覆盖率。它与 `coverage_ratio` 的总体有效证券覆盖率不是同一个指标。
- **OpenAPI 文档缺口**：当前 `ProviderProbeItem` Schema 尚未声明上述 validation 与 coverage_dimensions 字段，但 Go 响应模型已包含。WebUI 以当前 Go JSON 响应模型和实际回包字段为接入依据；数据侧需后续补齐 OpenAPI，避免客户端只依据 Schema 生成时遗漏字段。本 WebUI 分支不修改服务端 OpenAPI/Go 代码。
- 时间语义：`provider_time` 为源端时间，`fetched_at` 为服务端采集时间，`checked_at` 为诊断检查时间；任何字段缺失时均不得用其他时间字段补位。
