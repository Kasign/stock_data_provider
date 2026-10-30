# 数据层 API 接口清单与 WebUI 接入映射

更新日期：2026-10-30
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

以下路径均相对于 /market-api。当前 server.go 注册的行情接口均为 GET。

| 方法 | 路径 | 功能 | 参数摘要 | WebUI 封装 | API Key |
|---|---|---|---|---|---|
| GET | /health/live | 行情服务存活检查 | 无额外参数；详见 OpenAPI | marketLiveness | 否 |
| GET | /health/ready | 行情服务就绪检查 | 无额外参数；详见 OpenAPI | marketHealth | 否 |
| GET | /api/v1/market/quotes | 批量获取指定股票实时行情（最多 50 只） | symbols：逗号分隔，必填，最多 50 个且不得重复 | 未封装（服务端已实现） | 是 |
| GET | /api/v1/quotes/{symbol} | 单只股票实时行情 | 路径参数 symbol | getQuote | 是 |
| GET | /api/v1/bars/{symbol}/daily | 单只股票日 K（支持日期范围、复权及 as_of 历史观察时点） | 路径参数 symbol；start、end、limit（1–2000）、adjustment、as_of | getDailyBars | 是 |
| GET | /api/v1/bundles/{symbol} | 单只股票行情与日 K 统一快照 | 路径参数 symbol；lookback_days（20–2000）、adjustment、as_of | getMarketBundle | 是 |
| GET | /api/v1/data-sources/diagnostics | Provider 业务请求熔断诊断（进程内真实请求观测） | 无额外参数；详见 OpenAPI | 未接入 WebUI | 是 |
| POST | /api/v1/data-sources/probe | 主动探测已启用 Provider 的各项能力 | 无请求体；单项超时和并发受限 | 未接入 WebUI | 是 |
| GET | /api/v1/data-sources/probe/history | 主动探测历史批次 | limit（1–100，默认 20）；依赖 MariaDB | 未接入 WebUI | 是 |
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
| GET | /api/v1/market/sectors | 行业/概念板块实时排行 | type（industry/concept）、direction（up/down）、sort_by（change_percent/amount/change_5d）、limit（1–100） | getMarketSectors | 是 |
| GET | /api/v1/market/capital-flow | 市场级每日资金流历史序列（Tushare moneyflow_mkt_dc） | start、end 必填；闭区间，最长 2000 个自然日；需启用并配置有权限的 Tushare Token | 未接入 WebUI | 是 |
| GET | /api/v1/market/index-catalog | 国内指数/板块目录 | tag（cn_concept/region/tszs/industry/tushare） | getIndexCatalog | 是 |
| GET | /api/v1/market/indices/{symbol}/constituents | 指定指数成分及权重快照 | 路径参数 symbol；source、as_of 可选 | getIndexConstituents | 是 |
| GET | /api/v1/market/indices/{symbol}/daily | 指数/板块历史日线 | 路径参数 symbol；start、end 必填；source 可选 | getIndexDailyBars | 是 |
| GET | /api/v1/market/indices | 核心市场指数行情 | 无额外参数；详见 OpenAPI | getMarketIndices | 是 |
| GET | /api/v1/market/trading-calendar | A 股交易日历 | start、end 必填（闭区间） | getTradingCalendar | 是 |
| GET | /api/v1/market/session | A 股交易时段状态 | at 可选，RFC3339 时间 | getMarketSession | 是 |
| GET | /api/v1/market/breadth | 全市场涨跌分布/市场宽度 | 无额外参数；详见 OpenAPI | getMarketBreadth | 是 |
| GET | /api/v1/market/sentiment | 当前市场情绪快照（聚合市场宽度、涨跌停/炸板等） | 无额外参数；依数据源覆盖情况返回质量状态 | 未接入 WebUI | 是 |
| GET | /api/v1/market/sentiment/history | 已持久化的市场情绪历史快照 | start、end、limit；依赖 MariaDB | 未接入 WebUI | 是 |
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
| GET | /api/v1/analysis/runs/{run_id} | 查询分析运行结果 | getAnalysisRun |

分析任务请求/响应的完整字段契约以 doc/接口规范/分析任务.openapi.yaml 和分析服务当前实现为准。

## 4. 页面与接口对应关系

| 页面/模块 | 当前调用 | 当前边界 |
|---|---|---|
| 市场总览 | 核心指数、市场宽度、板块排行、涨跌停池、人气榜 | 市场级资金流和市场情绪历史 API 已有服务端实现，尚未接入 WebUI；全球市场联动暂缓 |
| 市场活动 | 龙虎榜、交易日历 | 已接入；空榜/未知质量不能解释为确定的零条 |
| 个股搜索与行情 | 股票搜索、单股行情、日 K、Bundle、数据源清单 | 已接入；行情质量与源端时间必须展示 |
| 个股研究 | 档案、估值、财务、所属板块、公告、筹码分布、个股资金流 | 已封装/按需加载；具体 Provider 能力和数据质量依回包判断 |
| 自选股 | 浏览器本地清单；研究列表只复用当前已加载行情 | 没有跨设备服务端自选持久化；批量行情接口已存在但尚未由 WebUI 封装 |
| 数据中心 | WebUI 页面检查、行情 live/ready、分析 healthz/readyz、Provider 静态能力清单 | 不直连数据库；能力声明不是实时健康 |
| Provider 运行诊断 | /api/v1/data-sources/diagnostics、/probe、/probe/history | 被动熔断诊断与主动能力探测分离；探测历史依赖 MariaDB；当前页面尚未调用 |
| 选股、告警、通知、问股 | 当前为前端入口/状态能力 | 不能把 UI 状态当作服务端任务、规则或投递记录；后端闭环仍需补齐 |

## 5. 数据层接口的重要约束

1. /api/v1/market/quotes?symbols=... 只查询调用方明确提供的最多 50 只股票，不是全市场扫描；逐项返回失败码，整体质量可降级。
2. /api/v1/data-sources 是 Provider 注册与能力声明；不能证明某个 Provider 当前可请求成功。
3. /api/v1/data-sources/diagnostics 来源于真实 quote/daily_bars 请求形成的进程内熔断器状态，不会主动探测全部 Provider，服务重启后历史计数会重置。
4. 日 K as_of 是系统观察时间点查询，只读取本地记录的历史版本；不是把当前 Provider 数据伪装成历史数据。
5. 指数目录、指数行情、指数成分、指数历史日线是不同数据域，不能互相替代。
6. 市场宽度、涨跌停池、龙虎榜等接口的空数据可能是 unknown/degraded；不得将未知转换为 0 或“无事件”。
7. 行业/概念排行服务端支持 type、direction、sort_by（change_percent/amount/change_5d）和 limit；WebUI 当前封装尚未暴露 sort_by/direction 时使用服务端默认值。
8. 前端只负责展示服务端单位和质量字段，不猜测成交量/成交额单位，不把成交额当作资金净流入。

## 6. 统一字段与错误语义

- 质量状态：valid、degraded、stale、invalid、unknown。
- 行情/日 K 标准单位：价格 CNY/股、成交量股、成交额 CNY；百分比数值以百分点表达，例如 1.25 表示 1.25%。
- 时间字段必须区分源端事件时间、服务端采集时间 fetched_at、标准化时间 normalized_at 和系统观察时间 system_observed_at。
- 错误响应统一包含 error.code、error.message，可包含 request_id；前端应保留 HTTP 状态和请求 ID，错误信息限长展示。
- 复权请求仅接受 none、forward_adjusted、backward_adjusted；unknown 是响应状态，不是请求参数。

## 7. 仍需数据层/产品评估的 API 需求

- 板块资金净流入排行（需核实源字段语义、单位、报告时间和完整候选板块集）；市场级资金流和市场情绪历史已有服务端 API，仍需真实权限及部署验收。
- 板块多维排行与资金流入排行（需核实接口是否提供相应排序字段）。
- 自选股批量概览（已有批量 quote 接口；MA、资金、换手等字段仍需定义是否扩展）。
- 选股规则执行、告警规则/评估/触发记录、通知渠道与投递结果。
- Provider 主动探测与逐能力健康检查已有独立 API；仍需在 MariaDB 环境验收探测历史，并补齐可视化接入。
- 全球市场联动数据。

## 8. 权威接口定义

- 行情服务 OpenAPI：services/market-data/openapi.yaml（代码内权威协议，不迁移到文档目录）。
- 分析任务 OpenAPI：doc/接口规范/分析任务.openapi.yaml。
- Python 桥接 OpenAPI：doc/接口规范/Python桥接.openapi.yaml。
- WebUI 实际请求实现：web/src/api.js。
- 行情服务路由注册：services/market-data/internal/httpapi/server.go。
