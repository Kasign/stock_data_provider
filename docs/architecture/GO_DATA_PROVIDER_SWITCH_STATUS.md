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
| 实时行情 | 东方财富、腾讯、新浪、Yahoo Finance（可选）、网易财经（候选） | Yahoo 新增 Chart 1m 实时快照，缺成交额固定 degraded、默认关闭；网易报价 DNS 解析失败、应用默认禁用 |
| 日线 K 线 | 东方财富、BaoStock、腾讯、新浪（仅 none）、Yahoo Finance（可选）、网易财经 CSV（可选） | BaoStock 已进入默认日 K 回退链；新浪只支持不复权且缺成交额；Yahoo 已有真实返回、缺成交额固定 degraded；网易财经 CSV 只支持不复权、仅在成交额/成交量与价格区间一致时保留金额，默认关闭 |
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

- 原 Python Provider 的完整替换仍未完成：Efinance、AkShare、Pytdx、Tushare、TickFlow、Longbridge、Futu、Finnhub、Alpha Vantage 等仍需逐一对照上游源码、权限、优先级、单位和失败语义；Yahoo Finance Chart API 提供可选日 K 与实时快照适配器（均默认关闭、缺成交额固定 degraded）；网易财经 Quote/CSV 已有独立 Go adapter，但实时行情与历史 CSV 必须分别验收。网易历史 CSV 当前返回 502、实时 Quote 在 GitHub Runner 上 DNS 解析失败，二者仍默认禁用。
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
