# Go 数据服务迁移状态与验收边界

> 更新时间：2026-10-23  
> 当前分支：`feat/full-go-data-providers-api-switch`  
> 当前定位：分阶段迁移；不是全 Python 去依赖的最终版本。

完整设计见 [Go 行情数据服务迁移方案](./GO_MARKET_DATA_MIGRATION_PLAN.md)，数据源扩展顺序与真实验收门槛见 [数据源扩展计划](./DATA_SOURCE_EXPANSION_PLAN.md)，真实 NAS 取数、故障回退与 Eastmoney 网络故障证据见 [数据源真实环境验收与故障排查记录](./DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md)，Review 与警示见 [架构 Review 记录与工程规范](./ARCHITECTURE_REVIEW_AND_ENGINEERING_RULES.md)，数据域明细见 [Python Provider 到 Go 迁移矩阵](./PYTHON_PROVIDER_GO_MIGRATION_MATRIX.md)。

## 1. 当前已实现

### 主 API 与服务边界

- 根目录 `cmd/analysis-api` 通过 `internal/adapters/marketdata` 调用原生 Go market-data API。
- 实时行情使用 `GET /api/v1/quotes/{symbol}`；日 K 使用 `GET /api/v1/bars/{symbol}/daily`。这两类数据不再静默回退到旧 Python API。
- Go 服务在不同 Go module 之间通过 HTTP JSON 契约连接，未跨 module 导入对方的 `internal` 包。
- `internal/adapters/pythonapi` 保留给旧 smoke test、对照和兼容测试，但不是根主 API 的行情 fallback。
- `go-core` 仍通过 Python bridge 获取尚未迁移的市场概况、新闻、LLM 或其他辅助能力，不能直接删除 bridge。
- `services/market-data` 已支持以 `MARKET_DATA_DB_HOST/PORT/USER/PASSWORD/NAME/CHARSET/TIMEZONE` 拆分配置并在 Go 代码中生成 MySQL DSN；根 `.env.example` 不再要求手工填写 `MARKET_DATA_MYSQL_DSN`。旧 DSN 仅在全部拆分字段都未配置时临时兼容；分析 API 继续独立使用 `DB_*` / `REDIS_*`。
- `MARKET_DATA_ADJUSTMENT` 作为行情分析与行情服务共用的默认复权口径；仅当需要为行情服务单独覆盖时，才配置 `MARKET_DATA_DEFAULT_ADJUSTMENT`。

### 原生行情 Provider

| 能力 | Provider | 状态 |
|---|---|---|
| 实时行情 | 东方财富、腾讯、新浪、Yahoo Finance（可选）、网易财经（候选） | Yahoo 新增 Chart 1m 实时快照，缺成交额固定 degraded、默认关闭；网易报价在 GitHub Runner 上出现 DNS 解析失败；当前配置仍将网易作为启用的后置回退，生产稳定性未验收 |
| 日线 K 线 | 东方财富、BaoStock、腾讯、新浪（仅 none）、Yahoo Finance（可选）、网易财经 CSV（可选） | BaoStock 已进入默认日 K 回退链；新浪只支持不复权且缺成交额；Yahoo 已有真实返回、缺成交额固定 degraded；网易财经 CSV 只支持不复权、仅在成交额/成交量与价格区间一致时保留金额；当前配置启用为后置回退，上游稳定性仍未验收 |
| 筹码分布 | 妙想 MX_API | 已新增可选 SupplementProvider、HTTP 路由和缓存；默认关闭 |
| 个股主力资金流 | 妙想 MX_API | 已新增可选 SupplementProvider、HTTP 路由和缓存；5/10 日合计必须有完整观测 |

MX 相关接口：

- `GET /api/v1/stocks/{symbol}/chip-distribution`
- `GET /api/v1/stocks/{symbol}/capital-flow`

启用条件是显式设置 `MARKET_DATA_MX_ENABLED=true` 并配置 `MARKET_DATA_MX_API_KEY`（兼容原变量 `MX_APIKEY`）。筹码和资金流尚不能证明精确 publication/available_at，返回值必须保留 degraded 质量状态；不得用于严格的历史时点分析。

### 日志与任务事件

- 根分析 API 增加 JSON slog、X-Request-ID 传播、HTTP 状态码/耗时/响应字节记录。
- 根 API 记录安全的任务输入/输出摘要；Runner 每个 stage 记录开始、完成/失败和耗时日志。
- market-data HTTP 层记录 request ID、path、method、状态码、耗时与响应字节数；Provider 失败新增结构化日志，区分超时、网络连接失败、连接关闭、HTTP 状态、数据契约等错误类别，并清理日志消息中的 URL 查询串；MX 请求记录数据集、标的、状态和耗时，不记录 key 或查询正文。
- MariaDB stage event 已有持久化与查询接口；异步任务 API 已提供 `task_id`、状态查询和阶段事件轮询，SSE 与前端实时任务 UI 尚未实现。详见 [日志与任务步骤进度方案](./LOGGING_AND_TASK_PROGRESS_PLAN.md)。

## 2. 明确尚未完成

- 原 Python Provider 的完整替换仍未完成：Efinance、AkShare、Pytdx、Tushare、TickFlow、Longbridge、Futu、Finnhub、Alpha Vantage 等仍需逐一对照上游源码、权限、优先级、单位和失败语义；Yahoo Finance Chart API 提供可选日 K 与实时快照适配器（均默认关闭、缺成交额固定 degraded）；网易财经 Quote/CSV 已有独立 Go adapter，但实时行情与历史 CSV 必须分别验收。网易历史 CSV 曾返回 502，实时 Quote 在 GitHub Runner 上 DNS 解析失败；但当前 `.env.example` 与 `configs/providers.json` 将二者默认启用为后置回退。目标网络稳定性仍未验收，统一验收前可分别设置 `MARKET_DATA_NETEASE_QUOTE_ENABLED=false` 与 `MARKET_DATA_NETEASE_DAILY_BARS_ENABLED=false` 关闭。
- 全市场指数/广度、行业/概念板块、涨停池、财务、完整资金流口径、龙虎榜、新闻公告等 Go 原生数据域未全部实现。
- `go-core` 的 market overview/新闻/LLM/部分持久化及通知仍有 Python bridge 依赖。
- 同花顺接入尚未实现，已列入计划；参考仓库 [Kasign/Financial-API](https://github.com/Kasign/Financial-API)，必须先核验真实 API 权限、速率限制和字段口径。
- 已实现 MariaDB `analysis_tasks`、有界 Go Worker、`POST /api/v1/analysis/tasks` 的 202 + task_id、任务状态查询和 `after_sequence` 阶段事件轮询；SSE、取消、自动重试与前端状态展示仍未完成。重启时遗留的 running 任务会显式标记为 `worker_interrupted`，不自动重跑。
- 已完成一部分真实 Provider 请求及 MariaDB/Redis 链路验证，但目标环境的完整生产回归仍未完成；CI 绿色不能替代真实网络和端到端验收。


### 数据源扩展评估（2026-10-22）

- BaoStock 原生 Go TCP 客户端与 DailyBarProvider 已实现，包含登录、CRC 校验、压缩帧、分页和单位/复权标记；当前默认启用作日 K 备用源，可通过 MARKET_DATA_BAOSTOCK_ENABLED=false 显式关闭。2026-10-22 的真实 NAS 测试与 2026-10-23 的 GitHub Runner 探测均取得有效日 K 样本；这不代表所有日期、证券或目标容器都已完成验收。完整证据见[真实环境验收记录](./DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md)。
- BaoStock 首期定位为历史日 K 备用源，后续按独立能力评估部分财务指标、复权因子、除权除息、股票基础信息、行业分类及指数成分；不把它当作实时行情、新闻公告、龙虎榜、市场资金流的全能来源。
- 接入采用 Go 原生 TCP 客户端，不在生产采集路径引入 Python 运行时。本轮本地 TCP 集成回归覆盖分页/空末页、上游错误码、断线后重新登录及登出；Provider 拒绝重复交易日和非法 OHLC。GitHub Runner 已连续多轮成功登录并查询，仍需在目标 NAS/容器继续扩大日期、证券及故障恢复样本。
- 官方文档指出其前复权算法可能与同花顺、通达信等不同。数据模型和验收计划必须记录复权算法与来源，不能把同名的前复权序列自动视为数值等价。
- Eastmoney 故障于 2026-10-22 进一步复现：容器可访问百度 HTTPS；两个 Eastmoney 域名根路径可建立 HTTPS 并返回 404，但真实历史 K 线接口在连接部分解析到的 IP 时出现 Network unreachable。根因未确定，不能断言运营商封锁或域名整体被封。详情见[真实环境验收记录](./DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md)。
- Tencent 已作为 BaoStock 故障回退源成功返回 600036.SH 日 K 样本，但成交额缺失被标记 degraded；仍需独立单源验收。未复权数据不得作为前复权兜底。

详细验收步骤、能力分工、候选数据源成本/权限条件见 [数据源扩展计划](./DATA_SOURCE_EXPANSION_PLAN.md)。

### BaoStock Go 实现阶段（2026-10-22）

- 已新增独立 internal/baostock 协议包与 provider.Baostock；协议包负责会话串行化、TCP 帧、CRC32、96 类型 zlib 响应、分页和上下文 deadline。
- Provider 只实现 DailyBarProvider。成交量标准单位为股、成交额标准单位为人民币元；成交额缺失时保持 null/unknown/degraded，不估算成交额。
- 新增 adjustment_method=baostock_pct_change，标明其复权算法，避免只靠 forward_adjusted 标签就认定与其他来源数值完全相等。
- Provider 已注册为默认日 K 备用源；目标容器需要继续完成真实登录、查询和逐日口径验收，如遇部署网络或复权问题可通过环境变量关闭。

## 3. 构建与自动化测试状态

BaoStock 第一轮代码与配置修复在 [CI 运行 39754783078](https://github.com/Kasign/stock_data_provider/actions/runs/39754783078) 全部通过，包括根模块、go-core、services/market-data、Compose 校验和 Docker 镜像构建。该结果只覆盖模拟服务与构建，不包含真实 BaoStock 登录查询或 NAS 网络验收。

GitHub Actions 运行 [39583673094（run #84）](https://github.com/Kasign/stock_data_provider/actions/runs/39583673094) 已在根 module、`go-core` 和 `services/market-data` 三个模块中分别通过 `go test ./...`、`go test -race ./...` 和 `go vet ./...`。该运行验证了最新 Go 源码，包括 MX 配置与接口、请求 ID/JSON 日志和中文注释调整。

异步任务模块加入后，GitHub Actions 运行 [39586848833（run #120）](https://github.com/Kasign/stock_data_provider/actions/runs/39586848833) 再次在三个 Go module 中通过 `go test ./...`、`go test -race ./...` 和 `go vet ./...`，覆盖了任务队列、HTTP 状态/事件轮询及 panic 恢复代码。配置拆分与 `gofmt` 检查调整后，运行 [39711711489（run #140）](https://github.com/Kasign/stock_data_provider/actions/runs/39711711489) 在三个 Go module 中通过检查；随后复权默认值共享逻辑的回归测试也在 [39712383338（run #147）](https://github.com/Kasign/stock_data_provider/actions/runs/39712383338) 通过。两个运行均覆盖格式检查、依赖下载、测试、竞态检测和 `go vet`。CI 不执行真实 MariaDB SQL migration 或供应商端到端测试。

后续变更按功能范围分组提交；已经创建的提交保持原样，不重写历史、不 force-push。**CI 通过不代表真实 MX Key/上游网络、东方财富/腾讯/新浪生产数据、目标服务器 MariaDB/Redis 或 Docker Compose 已完成端到端验收。**

目标验收命令：

```bash
# 分别在仓库根目录、go-core、services/market-data 执行
go test ./...
go test -race ./...
go vet ./...
```

此外还需要在真实部署环境测试：

1. Go API 访问 Go market-data（API Key、代码规范化、超时和错误状态）。
2. 东方财富、腾讯、新浪真实供应商接口的响应解析、单位、时间和限流。
3. MX 关闭时明确返回 Provider 不可用；启用有效 Key 后验证真实 payload schema、比例/金额单位与空值。
4. 已有 MariaDB/Redis 的读写、migration、cache freshness、健康检查和容器依赖顺序。
5. 从外部请求到 run_id、所有 stage events、最终结果/失败结果的完整关联。

### 行情 Provider 契约与最新自动化验收（2026-10-23）

- [`91eff930`](https://github.com/Kasign/stock_data_provider/commit/91eff930fcfee919f3707658f697dedba8156369)：新浪、腾讯、东方财富报价增加统一质量契约校验；腾讯与东方财富日 K 共用范围校验、去重、排序、坏行降级和 limit 截断；东方财富重定向只跟随 HTTPS 官方域名。
- [`f5725e1a`](https://github.com/Kasign/stock_data_provider/commit/f5725e1a606ee2f00aaa434dc63e5b28f76c9031)：将原腾讯前复权测试夹具最低价改为 43.00，修正收盘价 43.05 低于最低价 43.10 的错误样本。生产数据质量门禁没有放宽。
- [`3e94fdd3`](https://github.com/Kasign/stock_data_provider/commit/3e94fdd353320b8059382604a1afc0c738206520)：BaoStock 在显式起止日期下也执行 limit 上限，并按日期升序返回。最新真实探测中调用上限 8 实际返回 8 根。
- [Go test suite #206](https://github.com/Kasign/stock_data_provider/actions/runs/39885215590) 已完成并成功：根模块、`go-core`、`services/market-data` 的 gofmt、测试、race、vet，以及 Compose 校验和 Docker 镜像构建通过。
- [Live provider probe #15](https://github.com/Kasign/stock_data_provider/actions/runs/39885215595) 该轮探测 job 成功。新浪报价 valid；腾讯报价及两种日 K 返回但 quality degraded（供应商字段/成交额契约限制）；东方财富主报价和延迟报价返回但 quality degraded；东方财富前复权日 K 与 BaoStock 前复权日 K 各返回 8 根 valid。前一轮探测 #14 曾遇到东方财富两个报价入口 HTTP 502，说明端点有波动，不能宣称稳定。
- 报价与日 K 的真实探测是特定时间窗口的网络/数据证据，不取代 NAS 容器端验收。BaoStock 已纳入默认日 K 回退链，后续继续验证停牌、无数据日期和更长历史范围。

- [`619522a6`](https://github.com/Kasign/stock_data_provider/commit/619522a62a69e73b566336e9ec19584be00bb26e)：东方财富报价时间解析兼容 10 位 Unix 秒与 13 位毫秒，并拒绝明显不合理的数字时间值。基于真实 payload 的 `f86=1792719731` 样本，测试确认报价时间会被解析而非误报缺失。
- [Go test suite #208](https://github.com/Kasign/stock_data_provider/actions/runs/39885682985) 已完成并成功：根模块、`go-core`、`services/market-data` 的格式检查、测试、race、vet，以及 Compose 校验和 Docker 镜像构建全部通过。
- [Live provider probe #17](https://github.com/Kasign/stock_data_provider/actions/runs/39885682979) 的 Provider 探测 job 通过：Sina 报价 valid；Tencent 报价及两种日 K 有返回但质量仍 degraded；Eastmoney 主报价经 302→200 重定向后、延迟域报价均为 valid 且已带 quote_time；Eastmoney 日 K 与 BaoStock 日 K 各返回 8 根 valid 数据。

## 4. 下一步顺序

1. 用户恢复测试前的 `.env` 并重新创建 `market-data` 后，确认 BaoStock 正常路由恢复；首轮真实样本已通过，后续继续扩大交易日、停牌/无数据日期和更长历史范围的验收。
2. 独立测试 Tencent 日 K：只启用 Tencent，先验收不复权数据日期覆盖、OHLC、成交量/成交额单位和质量标记，再按真实响应测试复权能力；缺失成交额或未复权价格不得伪装成完整前复权数据。
3. 独立测试新浪报价与日 K：日 K 当前只支持 `adjustment=none`，且 amount unknown/degraded；目标 NAS/Docker 网络和服务实际路由仍需验收。
4. 持续跟踪 Eastmoney 历史行情接口 `Network unreachable`；根因未确定，按[真实环境验收记录](./DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md)保存证据并待后续排查。
5. 接着按[数据源扩展计划](./DATA_SOURCE_EXPANSION_PLAN.md)分域推进财务、基础资料、行业/指数、市场广度、板块、龙虎榜和新闻公告数据域；Tushare/THS 的权限、费用及速率先核实再接入。
6. 完成实际部署版本的 Provider 错误日志验证，以及 MariaDB/Redis 权限、缓存新鲜度、健康检查与 Docker Compose 端到端回归；之后继续实现 SSE 和前端任务进度展示。


### Tushare 国内数据域扩展（2026-10-26，待统一验收）

本轮在现有日 K Provider 基础上，新增 Go 原生 StockBasicProvider、TradingCalendarProvider、FundamentalsProvider 和 ValuationProvider。能力复用现有 Provider 注册、数据服务缓存与契约校验，不引入 Python 运行时依赖。

- 证券基础资料使用 stock_basic，输出名称、行业、市场、上市/退市日期与上市状态；不会从该端点臆造 PE/PB、市值或完整公司资料。
- 交易日历使用 trade_cal，要求上游覆盖请求区间内每个自然日；缺日/重复日会返回错误，交由既有回退链继续处理。
- 季度财务指标使用 fina_indicator，只接受匹配股票及报告期的数据，并保留 ann_date 作为公告日期；缺字段的指标组明确降级，不补零。
- 批量估值使用 daily_basic 的 PE/PB/PS 字段，另行保存 provider_report_date。精确快照时间和 PCF TTM 不可由该端点确认，因此保持 degraded，PCF 留空。
- 所有 Tushare 能力维持默认关闭。启用前须配置有效 Token、总开关和对应能力开关，并确认账户积分/接口权限。本轮按需求没有运行测试；代码接入不等同于上游验收或生产可用。


### Tushare 国内事件数据扩展（2026-10-26，待统一验收）

- 涨跌停池基于 limit_list_d，支持 up/down/broken 三类；供应商金额字段未强制解释为人民币元，按 provider_defined 输出，并限制单次请求与返回条数。
- 龙虎榜基于 top_list，按供应商原始分页语义切页并过滤非 A 股行；买卖额、净额等保留在 Provider 专用金额字段，避免在单位未确认时误标为人民币元。
- 个股公告基于 anns_d，输出公告日期、源端发布时间、标题和原始来源 URL；发布时间无法解析时降级，返回条数达到上游单次上限时明确标记潜在截断。
- 三项能力均默认关闭。Tushare 文档标明涨跌停列表与龙虎榜有积分门槛，公告接口需要单独申请权限；本轮未运行测试，也未进行真实接口请求。


### Tushare 国内指数目录与历史行情（2026-10-26，待统一验收）

新增可选 `IndexCatalogProvider`，提供官方指数目录、最近可用的指数成分权重快照和指数日线历史。接口使用 `/api/v1/market/index-catalog?tag=tushare`、`/api/v1/market/indices/{symbol}/constituents` 与 `/api/v1/market/indices/{symbol}/daily?start=YYYY-MM-DD&end=YYYY-MM-DD`；指数代码保留 `.SH/.SZ/.CSI` 后缀，不能与个股 K 线接口混用。

Tushare 指数目录通过 `MARKET_DATA_TUSHARE_ENABLED=true`、`MARKET_DATA_TUSHARE_INDEX_CATALOG_ENABLED=true` 和有效 Token 显式开启。指数成分取最近可用权重日期，可能滞后于当前交易日；日线成交量保留手、成交额按千元转换成人民币元。本轮没有运行测试和真实请求，这些能力仍待统一验收。


### 雪球人气榜回退接入（2026-10-26，待统一验收）

新增可选 Go 原生 `Xueqiu` Provider，仅承载热度/人气排名，不加入个股报价或日 K 回退链。配置 `MARKET_DATA_XUEQIU_ENABLED=true` 与 `MARKET_DATA_XUEQIU_HOT_RANKINGS_ENABLED=true` 后，它才会进入既有榜单 Provider 链；需要登录态时可配置 `MARKET_DATA_XUEQIU_COOKIE`。该 Cookie 是敏感会话信息，不写入日志或仓库。

雪球热门榜可补充人气排名；`surge` 使用全市场涨幅排序做代理，属于近似榜单，不宣称与东方财富 hot-up 数据一致，保持 degraded。本轮未运行测试或真实请求。


### 雪球关注榜能力补充（2026-10-26，待统一验收）

`GET /api/v1/market/hot-stocks?type=followers` 使用现有榜单 Provider 路由，雪球通过 `type=12` 返回关注榜。东方财富和同花顺未实现该榜单类型时会返回能力不支持，由路由继续尝试下一个 Provider。需要启用 Xueqiu 后备；测试和真实上游响应留到统一验收。


### 指数成分报告日、权重与 OpenAPI 契约补齐（2026-10-26）

Tushare `index_weight` 适配现在将选择的最新权重报告日与逐证券权重写入响应（`provider_report_date`、`weight_percent`），避免丢失可用于解释成分快照时点的来源信息。指数目录、成分和历史日线的 OpenAPI schema 已补齐；热度榜 schema 同步加入 `followers` 类型。以上仅为代码/契约补齐，未运行测试或 CI。


### 龙虎榜原始成交金额字段保留（2026-10-26，待统一验收）

Tushare `top_list.l_amount` 现在独立映射为 `provider_deal_value`，与买入、卖出、净额等原始字段一并保留，避免单位尚未确认时错误写入人民币元字段。OpenAPI 同步描述供应商专用金额、比例及排名元数据。尚未运行测试；字段映射待统一验收。


### 静态代码 Review 第 1 轮（2026-10-26）

Review 范围：Tushare 新增数据域、指数契约与 Xueqiu 榜单接线；本轮不运行测试或触发 CI。

- **已修复：龙虎榜成交额字段漏请求。** `FetchDragonTiger` 读取 `l_amount`，但请求字段列表此前未包含该字段。通用列解析器会因此在有龙虎榜记录时返回“缺少字段”错误。现在将 `l_amount` 加入 `top_list` 请求字段列表。Tushare 文档将该字段定义为龙虎榜成交额。
- **已按可查参数调整、仍待统一验收：雪球 surge 请求契约。** 现改为 `type=sh_sz` 与 `order_by=percent`，作为全市场涨幅靠前的代理排序；仍需真实响应确认字段与排序语义。该榜单只能作为近似替代，不能宣称与东方财富飙升榜完全等价。
- **待统一验收：指数代码与上游映射。** 服务层支持 `.TI/.SH/.SZ/.CSI`，但 Tushare 实际返回哪些后缀及各接口接受的代码范围仍需用官方目录/真实响应确认；未确认前不宜从支持列表推断所有后缀都能查询成功。
- 本轮只做静态审阅与契约比对；未编译、未运行单元测试、未触发 GitHub Actions，也未请求真实上游。


### 市场涨跌统计字段补齐（2026-10-26，待统一验收）

- `GET /api/v1/market/breadth` 继续以东方财富分布桶提供上涨、下跌、平盘计数；另外查询涨停池、跌停池端点的供应商总数 `tc`，只有两个端点各自的 `qdate` 与分布接口 `qdate` 一致时才返回 `limit_up_count` / `limit_down_count`。
- 若池接口失败、空池无法确认或报告日期不匹配，对应字段返回 null，并增加质量原因；不把未知计数伪造成 0，也不把分页返回条数误当作全量总数。
- `total_amount_cny` 仍返回 null、`amount_coverage_count` 为 0。原 Python `get_market_stats` 会给出总成交额（单位为亿元），但目前分布端点没有成交额字段；在未接入且校验全市场成交额数据前，不将该字段标记为等价迁移完成。
- 本轮只更新代码、接口契约、文档与测试用例；未执行编译、测试、真实请求或 CI。


### Tushare 指数成分按月查询（2026-10-26，待统一验收）

Review 发现旧实现将最近四个月的 `index_weight` 记录放在单次请求中，再从响应中提取最新日期。官方接口说明该数据按月提供，并建议按当月第一天至最后一天查询；跨月合并请求会增加触及上游返回限制、最新快照不完整且难以识别的风险。

现已改为从当前月份开始，最多逐月向前查询四个月；每次只请求一个自然月，并选取该月最新报告日期的记录，找到第一个有成分的月份后停止。单月返回量达到保守阈值时增加降级原因；仍需统一验收真实接口的返回完整度。此次没有运行测试、编译或 CI。


### Tushare 估值字段语义 Review（2026-10-26，待统一验收）

静态核对 Tushare `daily_basic` 官方字段说明后，发现 `pe` 定义为市值/净利润，`pe_ttm` 定义为市值/近 12 个月净利润；官方字段没有提供与公共模型 `pe_mrq` 完全等价的“最近季度”PE。此前把 `pe` 映射为 `pe_mrq` 存在语义误标风险。现新增可空 `pe_static`，将原始 `pe` 保留在该字段，并让 `pe_mrq` 保持 null；质量原因明确记录该源不提供 MRQ PE。

本轮仅静态核对和补代码/测试用例，没有请求真实上游、编译或运行测试。


### 财务指标的历史时点边界（2026-10-26，待规划增强）

`FetchFundamentals` 对指定报告期会选择供应商当前返回的最新公告版本，并暴露该版本的 `publication_date`。这能帮助消费者防止使用晚于策略参考时点的数据，但不能重建同一报告期此前发布的旧版本；API 也尚未提供 `as_of` 参数。历史回测应在研究流水线中依据公告日期做 point-in-time 门禁，不能将当前最新修订值直接回灌到早期时点。未来若要支持完整时点重建，需要确认供应商可返回历史修订记录，再设计贯穿 Provider、Service 和 API 的 as-of 契约。


### 指数成分与日线的来源显式选择（2026-10-26）

静态 Review 发现，指数目录支持 `tag=tushare`，但成分/日线原先只按代码走全局 Provider 优先级。启用同花顺与 Tushare 时，重叠的 `.SH/.SZ` 代码可能被另一个 Provider 优先处理，返回错误来源或错误代码语义。现为 `/constituents` 和 `/daily` 增加可选 `source=hithink|tushare`，建议调用方沿用目录响应中的 `source` 值；指定来源时只请求该 Provider，不跨来源回退。未指定时继续自动回退。缓存键升级并包含来源，避免同代码跨来源复用缓存。


## 4. 2026-10-26 静态 Review 补充：Provider 优先级校验

- 发现配置校验遗漏：内置 `provider_priority` 已包含 `xueqiu`，但优先级列表的白名单校验没有包含该名称。由于配置加载会先校验整个优先级列表，即使雪球功能默认关闭，也会在 Provider 装配之前报 `unknown provider "xueqiu" in priority list`，导致 market-data 服务启动失败。
- 已将 Provider 名称支持判断集中到 `isSupportedProviderName`，让供应商配置对象校验和优先级列表校验共用同一份允许名单，避免两处名单继续漂移。雪球仍保持默认关闭；允许它出现在优先级列表不代表会启用其能力。
- 已新增 `TestLoadAcceptsXueqiuInProviderPriority` 回归测试源码，覆盖“优先级含雪球、雪球能力关闭时，配置仍可加载”的场景。
- 本次仅做代码与配置静态审查，没有运行 Go 测试、构建、GitHub Actions 或实时 Provider 请求。该修复尚需用户后续统一回归确认。


## 5. 2026-10-26 静态 Review 补充：Provider 能力开关校验

- 原有布尔环境变量校验需要手工维护完整名单。部分能力开关（例如 `MARKET_DATA_BAOSTOCK_FUNDAMENTALS_ENABLED`、`MARKET_DATA_BAOSTOCK_TRADING_CALENDAR_ENABLED`）未被覆盖，填入 `sometimes` 等非法值时，`boolEnv` 会静默保留 JSON/代码默认值，导致部署人员以为配置已生效。
- 现在按支持的 Provider 名称与能力后缀统一校验所有 Provider 能力布尔环境变量；原有全局布尔项校验仍保留。未来添加能力开关时，即使未单独补入静态校验清单，也会被该规则覆盖。
- 已新增 `TestLoadRejectsInvalidBooleanForProviderCapability` 回归测试源码，覆盖 BaoStock 财务指标开关的非法值。测试尚未执行，留待统一回归。
- 本次修改只改配置校验、回归测试源码和文档，没有运行测试、构建、CI 或真实数据请求。


## 6. 2026-10-26 静态 Review 补充：部分日 K 回退择优

- 当所有 Provider 都无法达到最低 K 线覆盖阈值时，服务会尝试保留质量降级的最佳部分序列。旧逻辑只按返回条数选择；若两家 Provider 返回相同条数，优先级更高但最新交易日更早的那组会一直胜出。
- 现在先选经过验证的较长序列；条数相同时再比较最新交易日，优先保留更新的那组；条数和最新交易日都相同时维持现有 Provider 优先级，避免不必要的切换。
- 已新增 `TestPreferPartialBarsChoosesNewestSeriesWhenCountsTie` 回归测试源码，覆盖相同条数的更新/过时序列、较长序列及相同候选保留规则。测试尚未执行。


## 7. 2026-10-26 静态 Review 补充：网易报价端点配置校验

- 配置中已支持 `MARKET_DATA_NETEASE_QUOTE_URL`，但启动时的上游 URL 校验表只覆盖了网易历史 CSV 地址，遗漏了实时报价地址。错误的 `file://` 等协议因此不会在配置加载阶段被拒绝。
- 已把网易报价 URL 纳入统一 `validateEndpointURL` 检查，并新增 `TestLoadRejectsMalformedNeteaseQuoteEndpointURL` 回归测试源码。验证只检查配置形态，不代表目标上游当前可达。
- 本次测试源码没有执行；没有运行 CI、构建或实时 Provider 请求。


## 8. 2026-10-26 静态 Review 补充：默认 Provider 链的超时预算

- 默认配置启用的个股报价 Provider 共 5 个、日 K Provider 共 7 个；按照每次请求 12 秒、每个 Provider 重试 1 次以及 Bundle 先取日 K 再取报价的执行方式，配置加载时计算的顺序预算约为 291.2 秒。原代码默认值和 `.env.example` 的 `MARKET_DATA_REQUEST_TIMEOUT=3m` 低于该预算，导致默认配置会因超时预算不足而拒绝启动。
- 已将 market-data 的默认请求超时和 `.env.example` 统一调整为 `5m`，可覆盖上述静态预算并留有少量额外余量；若增加默认 Provider 数量、提高单源超时或重试次数，仍需重新评估预算。实际环境若显式设置了更短的 `MARKET_DATA_REQUEST_TIMEOUT`，启动校验仍会拒绝不足的配置。
- 已新增 `TestLoadDefaultTimeoutCoversFullDefaultProviderChain` 回归测试源码，覆盖完整启用链的默认预算。测试尚未执行。
- 注意：该预算是 market-data 服务端的保守上限计算，不代表每次正常行情请求都会耗时五分钟。分析 API 的 `ANALYSIS_TIMEOUT` 是独立总任务上限；代码默认与 `.env.example` 已同步到 `5m`，`HTTP_WRITE_TIMEOUT` 示例为 `5m10s`。多标的或多轮数据采集可能超过五分钟，部署方应根据工作负载调整，不能把此默认值视为任何任务规模的时长保证。
