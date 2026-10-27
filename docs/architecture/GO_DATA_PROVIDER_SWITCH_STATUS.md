# Go 数据服务迁移状态与验收边界

> 更新时间：2026-10-27  
> 当前分支：`feat/full-go-data-providers-api-switch`  
> 当前定位：分阶段迁移；不是全 Python 去依赖的最终版本。

## 0. 国内 Provider 静态审查记录（2026-10-27）

### 审查与验证边界

本轮及后续改动以源码、配置和文档静态审查为主；没有手动执行测试、构建、CI 或真实 Provider 请求。此前普通推送曾自动触发仓库已有的公共 Provider 探测、Go 检查/构建及 WebUI 校验工作流，结果并非全部成功。为遵守统一验收安排，后续提交使用 `[skip ci]`；需要注意，这不会代替本地或部署环境的实际验证。

### 国内 Provider 配置接线

- **腾讯股票搜索**：`MARKET_DATA_TENCENT_STOCK_SEARCH_URL` 已接入配置读取、URL 校验和启动装配；旧构造函数继续兼容，空值回落 Smartbox 默认端点。腾讯搜索没有可核验的结果总数，因此仍保持 `degraded`，服务层可回退到东方财富。
- **URL 环境变量**：配置校验清单中的 27 个 HTTP(S) Provider URL 字段已与 `services/market-data/.env.example` 对照，未发现遗漏。新增 URL 必须同步配置字段、环境变量、URL 校验、根/服务模板、启动装配和文档。Pytdx 主机列表、BaoStock 主机/端口是 TCP 配置，不算 URL。
- **默认能力文档**：已校正文案使其与 `providers.json` 对齐。当前默认配置启用网易行情/日 K 作为后置回退、BaoStock 财务/交易日历；这不代表相关上游在部署网络中已完成稳定性验收。Yahoo 默认关闭。README 已改为描述“部分国内数据域已接入”，同时明确字段覆盖、权限、质量和时点语义尚不等价。

### 质量与时间契约修复

| 数据域 | 当前服务层约束 |
|---|---|
| 市场广度 | 分类家数必须对账；成交额覆盖不完整时总额为 null、质量不得为 valid；非原子成交额聚合或未暴露源快照时间的质量原因不能被忽略；报告日期必须规范且不在未来。 |
| 指数行情/指数目录 | 指数价格、金额单位及有限数值受校验；单项降级不能被父快照标成 valid；源时间不能为零或明显晚于当前时间。未知/空指数目录仅缓存 5 分钟。 |
| 指数成分/指数日 K | 成分快照校验报告日期、源时间、来源返回/排除数量；缺名称/重复去重不能被隐藏为完整有效结果。指数日 K 校验交易日期、OHLC、涨跌幅、成交量/成交额单位与空值一致性，禁止父级 valid 掩盖降级条目；服务层拒绝未来 end_date，Tushare 成交量转换拒绝 >= 2^63 的浮点边界。 |
| 行业/概念排行、涨跌停池 | 校验记录总数、排名/代码唯一性、必填值、非有限数值、计数/金额范围和单位，并传播子项质量状态。东方财富计数字段拒绝小数、负数及浮点安全整数范围外的值，不再静默截断。 |
| 个股板块、龙虎榜、热度榜、公告 | 父级快照不能高于任一条目的质量等级；日期、分页与空结果语义必须一致。未确认的空列表保持 unknown，而不是伪装成“确认没有数据”。 |
| 财务数据 | 报告期必须是请求季度的期末且不晚于中国市场当前日期；披露日期必须规范、不得在未来，且不得早于对应报告期末。 |
| 证券主数据 | Provider 缺少质量状态或返回 unknown 时降级并附加原因，不自动提升为 valid。 |
| 筹码分布、资金流 | 缓存命中与新 Provider 回包使用同一套字段校验；拒绝越界比例、非有限/反向成本区间，以及观测数量不足的 5/10 日合计。由于当前模型没有精确来源可用时间字段，即使 Provider 或旧 Redis 缓存标 valid，也会降级/拒绝，并记录 `publication_availability_timestamp_unverified`。 |
| Tushare 日 K | 结束日期先规范化为 `YYYY-MM-DD` 再做未来日期比较，避免输入带首尾空格时校验口径不一致。 |

### 缓存策略修复

- **个股板块**：此前虽定义 unknown 空响应 5 分钟 TTL，但只写入 `HasData=true`；现在所有通过校验的响应均缓存，确认有数据缓存 12 小时，unknown 空响应缓存 5 分钟。
- **财务快照**：抽取 `fundamentalsCacheTTL` 统一读写策略：有效且有数据 12 小时；有数据但 degraded/unknown 为 1 小时；无数据的 unknown 为 5 分钟。
- **指数目录**：valid 目录缓存 12 小时；degraded 缓存 1 小时；unknown/空目录缓存 5 分钟。指数成分和指数日 K 原本已使用较短的非 valid TTL，本轮补强了其时间、单位与父子质量门禁。
- **补充数据缓存**：筹码分布/资金流缓存会再次通过共享校验；旧版 Redis 中被标 valid、但无法证明来源可用时间的条目会被忽略并回源。

### 测试源码与待验收事项

本轮为以上约束补充或调整了测试源码，但**均未执行**。最新提交使用 `[skip ci]`，未触发新的 GitHub Actions。后续仍需由统一验收确认测试/构建结果，以及目标部署环境中的上游连通性、数据单位和真实字段完整性；静态校验不代表生产质量已验收。

完整设计见 [Go 行情数据服务迁移方案](./GO_MARKET_DATA_MIGRATION_PLAN.md)，数据源扩展顺序与真实验收门槛见 [数据源扩展计划](./DATA_SOURCE_EXPANSION_PLAN.md)，真实 NAS 取数、故障回退与 Eastmoney 网络故障证据见 [数据源真实环境验收与故障排查记录](./DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md)，Review 与警示见 [架构 Review 记录与工程规范](./ARCHITECTURE_REVIEW_AND_ENGINEERING_RULES.md)，数据域明细见 [Python Provider 到 Go 迁移矩阵](./PYTHON_PROVIDER_GO_MIGRATION_MATRIX.md)。

## 1. 当前已实现

### 主 API 与服务边界

- 根目录 `cmd/analysis-api` 通过 `internal/adapters/marketdata` 调用原生 Go market-data API。
- 实时行情使用 `GET /api/v1/quotes/{symbol}`；日 K 使用 `GET /api/v1/bars/{symbol}/daily`。这两类数据不再静默回退到旧 Python API。
- Go 服务在不同 Go module 之间通过 HTTP JSON 契约连接，未跨 module 导入对方的 `internal` 包。
- `internal/adapters/pythonapi` 保留给旧 smoke test、对照和兼容测试，但不是根主 API 的行情 fallback。
- `go-core` 的新闻检索已增加 Go 原生 Bocha/SearXNG 与个股公告候选源，但仍保留 Python bridge 兼容回退；LLM、部分非中国市场概况及其他辅助能力仍未全部迁移，不能直接删除 bridge。
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

- 原 Python Provider 的完整替换与字段级等价尚未完成：Efinance、AkShare 等仍有 Python 聚合逻辑或本地算法需要逐函数对照；Pytdx、Tushare、TickFlow 已有部分 Go adapter，但权限、优先级、单位、日期覆盖和失败语义仍需确认；Longbridge、Futu、Finnhub、Alpha Vantage 等海外/账户型来源按需求暂缓。Yahoo Finance Chart API 提供可选日 K 与实时快照适配器（均默认关闭、缺成交额固定 degraded）；网易财经 Quote/CSV 已有独立 Go adapter，但实时行情与历史 CSV 必须分别验收。网易历史 CSV 曾返回 502，实时 Quote 在 GitHub Runner 上 DNS 解析失败；当前 `.env.example` 与 `configs/providers.json` 仍将二者默认启用为后置回退，生产稳定性未验收前可分别设置 `MARKET_DATA_NETEASE_QUOTE_ENABLED=false` 与 `MARKET_DATA_NETEASE_DAILY_BARS_ENABLED=false` 关闭。
- 多个国内数据域已有 Go 原生适配器，但覆盖与质量不等于完整迁移：核心指数快照、指数目录/当前成分/指数日 K（成分来源返回/排除条数已显式暴露）、市场广度、行业/概念排行、涨跌停池、财务/估值、龙虎榜、个股公告和新闻候选源已部分接入。仍缺严格时点财报与历史成分/公司行动数据、权威交易日历驱动的盘中和节假日判断、排行榜与涨跌停池的全量覆盖审计（池子 total 大于 items 时服务层会降级并标记截断原因）、资金流/筹码精确可用时间，以及旧 Python 接口字段级对账；详见迁移矩阵和数据源扩展计划。
- `go-core` 的新闻链路现有原生候选源并保留 bridge 回退；非中国市场概况、LLM、部分持久化及通知仍有 Python bridge 依赖。
- 同花顺 Go Provider 已按能力接入证券搜索、行情/日 K、指数目录/当前成分/指数历史 K、核心指数快照、滚动交易日历、财务指标与三张报表、批量估值、涨跌停池、龙虎榜、人气/飙升榜；各能力默认关闭或需显式开关且要求有效 API Key。历史成分、部分行业历史指标、字段级 1:1 Python 等价与账号权限/限流尚未确认。不得把“代码已接入”解读为生产权限和质量已验收；参考接口仓库 [Kasign/Financial-API](https://github.com/Kasign/Financial-API)。
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
日 K 响应也会拒绝空字段名和重复列名，避免供应商返回表头变化后发生列索引覆盖和字段错读；对应回归源码已补充，仍待统一执行。
StockBasic 只接受 A 股普通股票代码；未知或缺失的上游市场分类直接拒绝，不再默认归类为“主板股票”，避免将 ETF/基金等非目标证券误报为普通股票。该防护已补回归测试源码，尚待统一执行。

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


### 市场涨跌统计与全市场成交额（2026-10-27，待统一验收）

- `GET /api/v1/market/breadth` 以东方财富 `getTopicZDFenBu` 分布桶提供上涨、下跌、平盘计数；涨停池、跌停池的供应商总数 `tc` 只在各自 `qdate` 与分布报告日期一致时返回。
- 全市场成交额另由东方财富 CLIST 接口分页读取 `f6`（人民币元），通过配置的 `MARKET_DATA_EASTMONEY_SECTOR_RANKING_URL` 指向同一 CLIST 地址。仅当分页总数稳定、每页行数完整、证券标识有效且无重复、每行报价日期与分布报告日一致，并且成交额覆盖数与分布计数一致时，才填写 `total_amount_cny`；否则金额保持 null、覆盖数为 0，并附带质量原因。
- 成功聚合仍会标记 `market_amount_quotes_collected_non_atomically`：分批请求不是同一时刻的原子快照；此外，分布计数没有经独立证券主数据核验，供应商也未提供可验证的盘中快照时间，因此整体质量仍为 degraded。
- 广度结果缓存使用 5 分钟 TTL，降低分页访问造成的请求放大。新增/调整了本地 CLIST fixture 与质量契约断言。本轮未运行编译、测试、构建、CI 或真实 Provider 请求，后续按统一验收计划执行。


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


### 内置 Provider 默认值与 providers.json 一致性（2026-10-27）

静态对比发现，配置文件存在时与配置文件缺失时的行为有三处差异：BaoStock 的交易日历/财务指标能力在内置默认值中关闭但在 `configs/providers.json` 中开启；Yahoo 日 K 在内置默认值中开启但配置文件明确关闭；网易行情在内置默认值中关闭但配置文件及部署模板开启。现已把默认表抽为 `defaultProviderFile()`，统一以上三项策略，并增加默认值回归测试源码。这样在缺失配置文件的本地运行与常规部署配置下，国内源能力不会因加载路径不同而漂移。未运行测试、构建或 CI。

### 根目录环境变量模板覆盖范围补齐（2026-10-27）

根目录 `.env.example` 用于 Compose 部署，因此已补齐当前国内默认 Provider 的主要能力覆盖项：东方财富市场广度、板块、指数、涨跌停池、所属板块、龙虎榜、公告、基础资料和日 K/行情开关；腾讯行情与日 K 开关；新浪行情开关；以及 BaoStock 交易日历与财务指标开关。服务级模板补入东方财富基础资料开关，并移除重复的新浪日 K 开关。模板值与当前国内默认策略一致，部署时可直接按需改为 `false`。本次未运行测试、构建、CI 或真实 Provider 请求。

### SourceStatus OpenAPI 字段对齐（2026-10-27）

静态比对运行时模型与 OpenAPI 时发现，数据源状态响应已包含 `index_catalog_capable`，但 `SourceStatus` schema 遗漏了该字段。现已补齐契约字段，保持 Provider 能力状态的 API 文档与实际 JSON 一致；未运行测试或构建。

### 腾讯股票搜索端点可配置化（2026-10-27）

腾讯 Smartbox 搜索能力已经独立开关，但旧构造函数将搜索地址固定为代码常量。现新增 `MARKET_DATA_TENCENT_STOCK_SEARCH_URL` 配置并纳入 URL 形态校验；旧 `NewTencentConfigured` 保持兼容，新构造函数接收搜索地址，默认仍使用 Smartbox 地址。配置测试与本地 fixture 测试源码已补充，未运行测试、构建、CI 或真实上游请求。

### 全市场成交额分页聚合补齐（2026-10-27，待统一验收）

- `GET /api/v1/market/breadth` 新增东方财富 CLIST 全市场成交额汇总：按 A 股过滤条件分页读取 `f6`（人民币元），每页最多 100 条，最多 4 个并发请求，单次聚合预算为 60 秒。
- 每页要求返回完整条数且 `data.total` 在分页过程中保持一致；逐条校验证券代码/市场、非负成交额和 `f86` 报价日期；跨页证券重复、分页缺页、日期错位、全市场证券数与涨跌分布覆盖数不一致时均不输出总额。
- 全量成功且两个数据域的证券数一致后才写入 `total_amount_cny` / `amount_coverage_count`；多页抓取非原子快照、源端分布总体未由独立证券主数据核验，质量继续标记 `degraded`。
- 市场广度缓存从 90 秒调整为 5 分钟，避免每次缓存过期都重新扫描整个证券列表。新增分页聚合与报价日期门禁测试源码；按当前安排未执行测试、构建、CI 或真实上游请求，仍需后续统一回归。


### 东方财富人气榜与飙升榜端点配置化（2026-10-27）

- 静态审查发现，东方财富 Provider 的人气榜与飙升榜仍直接引用两个包级 URL 常量，配置层没有独立的端点覆盖入口；这与同一 Provider 的行情、日 K、板块、指数、龙虎榜和公告端点的配置方式不一致。
- 新增 `MARKET_DATA_EASTMONEY_HOT_RANKINGS_URL`（人气榜）与 `MARKET_DATA_EASTMONEY_HOT_SURGE_URL`（飙升榜），采用原公开端点作为默认值，纳入统一的绝对 HTTP(S) URL 形态校验，并在启动装配时传入 Provider。
- 旧的 `NewEastmoneyConfiguredWithAnnouncements` 等构造入口保留默认端点；新增的 `WithHotRankingURLs` 只覆盖非空配置值，因而不改变旧调用者行为。
- 已补充配置覆盖、非法协议拒绝及旧构造函数兼容的测试源码，并同步根目录与服务级环境模板、服务 README 和迁移矩阵。未执行测试、构建、CI 或真实上游请求；榜单没有可信的源端快照时间，响应仍应保持 `degraded`。


### 服务级环境模板请求超时同步（2026-10-27）

- 复核发现根目录 `.env.example` 和 Go 配置默认值已采用 `MARKET_DATA_REQUEST_TIMEOUT=5m`，但 `services/market-data/.env.example` 仍保留旧的 `3m`。该值低于默认 Provider 链的保守重试/回退预算，独立使用服务级模板可能触发启动阶段的超时预算校验失败。
- 已将服务级模板同步为 `5m`，并注释预算缘由。该修复只改环境示例与状态文档；未运行测试、构建、CI 或真实 Provider 请求。


### 东方财富国内补充能力默认状态与迁移矩阵对齐（2026-10-27）

- 当前 `defaultProviderFile()`、`services/market-data/configs/providers.json` 与服务级环境模板中，东方财富的指数报价、市场广度、行业/概念排名、涨跌停池、个股所属板块、龙虎榜、公告、筹码分布、人气榜/飙升榜均配置为默认启用，并可用各自独立能力开关关闭。
- 迁移矩阵先前部分条目仍写“默认关闭”或“未实现”，与当前代码/配置不符；现已按当前分支更新实现、开关默认值和验收限制。此前 Probe #100 报告内关于“当时默认关闭”的句子保留为历史记录，不再作为当前默认值的说明。
- 这些能力仍有真实部署网络、完整分页/交易日覆盖或源端快照时间方面的未验收项；默认启用仅描述配置行为，不得将 `degraded` 结果用于声称生产质量已达标。
- 本次仅修正文档，不运行测试、构建、CI 或真实 Provider 请求。


### 新浪报价时间语义与网易成交量单位质量修复（2026-10-27）

- 新浪报价解析了源端日期与时间并转换为 UTC，但标准模型此前遗漏 `quote_time_semantics`；现补为 `provider_snapshot_time`，并增加对应测试源码断言，避免下游将其与采集时间混淆。
- 网易历史日 K 的成交量单位校验会用成交金额与 OHLC 均价交叉验证“股/手”解释。此前若两种解释都不成立，虽然质量状态降级且成交额置空，仍把未转换的原始成交量写入标准 `volume`、强制标注为 `shares`。现将单位校验返回类型改为可空指针：明确为股时使用原值，明确推断为手时换算为股，单位无法判定时返回空 `volume` 和 `volume_unit=unknown`，由统一日 K 契约校验器剔除该行，不再伪造标准化单位。
- 已更新网易单位交叉校验测试源码并新增本地 HTTP fixture 测试，覆盖单位歧义时不得形成标准化股数；同步修正服务 README 中网易能力的默认状态描述，使其与 `providers.json` 及环境模板一致，并明确当前 DNS/502 稳定性仍未验收。未运行测试、构建、CI 或真实 Provider 请求。


### 腾讯行情时间解析的时区数据库回退（2026-10-27）

- 腾讯行情解析器已优先使用 IANA `Asia/Shanghai`，无法加载 zoneinfo 时回退到固定 UTC+8；本次将时区选择抽成可注入 helper，并新增“zoneinfo 不可用”与“本地行情时间转 UTC”测试源码。这样可在不改动系统时区数据库的情况下覆盖极简容器回退路径。
- 本次未运行测试、构建、CI 或真实 Provider 请求。


### 普通个股报价 OpenAPI 时间语义字段对齐（2026-10-27）

- 对照 `internal/model.Quote` 与 OpenAPI `Quote` schema 时发现，Go 模型已输出可选的 `quote_time_semantics`，但普通个股报价 schema 遗漏该字段；指数报价 schema 已包含同名字段。
- 已在 `services/market-data/openapi.yaml` 为个股 `Quote` 补入 `quote_time_semantics` 描述，明确这是供应商 `quote_time` 的语义，而不是采集时间。该字段保持可选，不改变已有 JSON 响应行为。
- 本次仅更新契约文档和状态记录；未运行测试、构建、CI 或真实 Provider 请求。


### 根 Compose 环境模板补齐国内上游 URL 覆盖项（2026-10-27）

- 静态检查 `docker-compose.yml` 后确认 `stock-analysis-data` 的 `env_file` 是仓库根目录 `.env`；`services/market-data/.env.example` 只是服务级手工部署模板，根 Compose 不会自动加载它。
- 根 `.env.example` 原先缺少多项虽已由 Go 配置读取、并通过启动 URL 形态校验的国内端点覆盖变量：东方财富行情、日 K、基础资料、板块、指数、市场广度、涨跌停池、所属板块、龙虎榜、公告；腾讯行情与日 K；新浪报价；以及雪球可选源配置。现已按 `config.go` 中的默认值补齐根模板，并补入东方财富公共 web token 的可选覆盖项。
- 保留原有默认 endpoint 与能力开关，不更改任何 Provider 的默认启停策略；这次只让 Compose 部署者更容易显式覆盖地址，避免误以为服务级模板会由根 Compose 自动加载。未运行测试、构建、CI 或真实 Provider 请求。


### 国内 Provider 静态 Review 补充（2026-10-27，本轮待统一验收）

- **Tushare StockBasic 证券范围**：格式合法的六位代码不等于 A 股普通股票。现在先按普通 A 股代码范围校验，ETF/基金及不支持的证券在发起上游请求前拒绝；未知或缺失的 `market` 不再默认映射为“主板股票”。
- **Tushare 日 K 响应字段表**：日 K 入口与其他 Tushare 能力采用一致的严格表头规则，拒绝空字段名和重复列名，避免重复列导致索引覆盖及字段错读。
- **估值快照时点**：服务层拒绝无效或晚于中国市场当前日期的 `provider_report_date`，拒绝零值或超过当前时间 60 秒的 `provider_timestamp`；缺少报告日期/精确时间的估值不能标记为 valid。
- 已新增对应回归测试源码。以上变更均只做了静态源码核对，**没有执行测试、构建、CI 或真实 Provider 请求**；仍需后续统一回归。


### Go 原生新闻检索质量策略增强（2026-10-27，本轮待统一验收）

- **可信来源排序**：按文章 URL 的实际 hostname 识别监管部门、交易所与信息披露网站，并为预置的可信财经媒体域名提供次级加分；不会仅相信搜索接口返回的 `siteName` 标签，也不会把可信域名当成文章真实的绝对保证。
- **垃圾/营销过滤**：默认过滤明确的荐股群、导流和开户链接用语；`GO_NEWS_BLOCKED_TERMS` 可以追加自定义过滤词。过滤词有意保持窄范围，避免把一般性市场情绪词误判成垃圾内容。
- **排序增强**：在股票代码/名称匹配基础上，增加监管、政策、指数、成交额与上市事件等市场主题词权重，标题命中高于摘要命中；原有新鲜度过滤、URL/标题去重和 Python 兼容 fallback 保持不变。
- **时间质量门禁**：拒绝晚于当前时间的发布时间，避免未来时间戳污染新闻排序与“最新消息”上下文。
- 配置示例已同步至根目录与 `go-core/.env.example`，技术说明同步至 `go-core/README.md`。本轮只新增/更新了回归测试源码，未执行测试、构建、CI 或真实新闻源请求；仍需统一验收。


### 中国市场概况转为 Go 原生聚合（2026-10-27，本轮待统一验收）

- `go-core/internal/adapters/marketdatahttp.Client` 新增原生市场概况聚合：并发请求指数快照、市场广度、行业排名和概念排名四个 Go API，减少 Python bridge 正常路径依赖与串行等待。
- 指数快照映射到统一 `MarketOverview.Indices`；上涨/下跌/平盘、涨跌停数量与人民币成交额映射到统一模型；行业/概念排名保留来源及质量提示。
- 对缺失成交额/涨跌停计数、部分市场覆盖、异常/过时的采集时间、非 valid 快照与接口失败保留明确 warnings。当前公开接口没有独立的板块跌幅榜，不伪造 `BottomSectors` / `BottomConcepts`，并输出对应限制提示。
- 原生路径只服务中国市场 live snapshot，拒绝把历史 `as_of` 伪装成实时数据。香港、美股、日股、韩股概况继续走 bridge。中国市场的原生组件不可用或结果无效时回退到 bridge，并在 warnings 标记；个股行情和日 K 仍不回退到 Python。
- 新增原生聚合和兼容回退回归测试源码。本轮未执行测试、构建、CI 或真实上游请求，仍待统一验收。


### Go-core 个股辅助数据并发迁移（2026-10-27，本轮待统一验收）

- `go-core/internal/adapters/marketdatahttp.Client` 在行情/日 K bundle 获取成功后，并发请求原生基本面、所属板块、筹码分布和资金流接口；辅助接口失败只增加 warning，不把可用行情 bundle 整体判失败。
- 基本面默认请求最近一个已结束季度，保留分区、公告日期、指标单位和来源质量信息；以不重复键兼容旧分析器，并使用分区前缀保留同名指标。
- 所属板块仅在 Provider 明确返回 `has_data=true` 时作为有效列表；筹码区间、资金流单位和质量信息保持原义，不将区间误作中位成本或将 null 变为零。
- 资金流新增到 Go `StockBundle`、`AnalysisContext` 及 Python 分析器桥接上下文。显式 `as_of` 请求不会附加仅支持当前快照的辅助数据；严格历史时点财报版本和辅助快照历史仍待实现。
- 已补对应客户端回归测试源码。本轮未执行测试、构建、CI 或真实上游请求，仍待统一验收。


- **报告期选择补充**：默认基本面查询改为以中国市场披露截止日判断最近可期待的报告期，避免三季报/年报披露窗口内普遍请求尚未披露的季度。对应回归测试源码已补，未执行测试、构建、CI 或真实上游请求。


### Go-core 个股公告并入原生新闻检索（2026-10-27，本轮待统一验收）

- `go-core/internal/adapters/marketdatahttp` 新增公告分页 DTO 与 `FetchStockAnnouncements`，校验请求日期/股票身份、分页计数、公告日期范围、源 URL、来源和质量状态；保持 `announcement_date`、`published_at`、`fetched_at` 的语义独立。
- 个股分析使用 market-data 的按股票公告接口，并发获取当前公告窗口，与 Bocha/SearXNG 候选按 URL/标题去重；公告候选按股票关联加权。即使未配置网页搜索服务，只要公告接口可用也可返回个股公告。
- 公告请求使用独立的短超时；单项不可用不会阻断核心分析和其他新闻来源。未提供源端精确发布时间的公告使用 `announcement_date` 作日期级过滤，不使用 `fetched_at` 伪造发布时间。
- 已增加客户端契约、公告独立来源以及与网页搜索合并去重的回归测试源码。
- 按约定，本轮未运行测试、格式化、构建、CI 或真实上游请求；待后续统一验收。


### Go 原生新闻多源聚合第二阶段（2026-10-27，本轮待统一验收）

- 将 Tavily News 与 Brave News 加入既有 Go 原生 Provider 接口；只有配置 API Key 才注册，不自动请求或启用任何公开 SearXNG 实例。
- Bocha、Tavily、Brave News、显式 SearXNG 均由统一调度器并发调用，失败来源单独记录，其他来源仍可提供结果；对所有候选统一执行发布时间门禁、可信域名加权、营销过滤、规范 URL/标题去重和个股相关性排序。
- Tavily 请求新闻 topic 并要求源端发布日期；Brave News 定向中国及简体中文检索。Brave 的 page_fetched 不视为发布时间；缺少可解析源端发布时间的结果会被质量过滤。
- API 凭据只从环境变量读取，日志不输出 key。新增 Tavily/Brave 本地 HTTP fixture、跨源聚合和去重测试源码；未运行测试、构建、CI 或真实供应商请求。
- 当前仍保留 Python bridge fallback 作为兼容边界；后续在统一验收与调用覆盖核实后，再评估是否可在部署配置中关闭新闻 fallback。


### Go-core 龙虎榜上下文摘要（2026-10-27，本轮待统一验收）

- `StockBundle.DragonTiger` 与 `AnalysisContext.DragonTiger` 新增个股龙虎榜摘要；从最新 10 个完整交易日读取现有 Go API 并派生 `is_on_list`、`recent_count`、`latest_date`。
- 摘要通过 `dragon_tiger_context` / `dragon_tiger` 传给兼容分析器。unknown/empty snapshots 不会强行转换成 `false`；覆盖不完整时保留 lower-bound 标记。
- 每日最多分页 10 页、每页 500 条，最多 4 个日期请求并发。未运行测试、构建、CI 或真实上游请求。


## 2026-10-27 — 构建阻塞修复与最新 HEAD 验证边界

- Eastmoney JSON struct tag 的非法字符问题已由 `50fecf2` 修复；此前 go-core 与 market-data 的格式问题已整理。多维新闻检索的两个文件由 `d692a0d` 格式化；双 Provider 聚合测试改为显式设置 `MaxSearches=2`，修正直接配置默认只执行一个维度与测试期望不一致的问题。
- `.github/workflows/go-test.yml` 现支持 `workflow_dispatch`，且格式检查输出脚本保留正确的单个 `\\n` 转义。
- 提交 `d551057` 对应的 [Go test suite #40449084361](https://github.com/Kasign/stock_data_provider/actions/runs/40449084361) 通过，根模块、go-core、market-data 的测试/race/vet、Compose 校验和两种 Go 镜像构建全部通过；同一 HEAD 的 [WebUI checks #40449084307](https://github.com/Kasign/stock_data_provider/actions/runs/40449084307) 与 [Live Provider Probe #40449084825](https://github.com/Kasign/stock_data_provider/actions/runs/40449084825) 也通过。
- 随后分支继续前进至 `24952dd` / `71993b1`，新增个股龙虎榜上下文摘要并补充相关交接/进度说明。因此，上述绿色 CI 仅覆盖 `d551057`，不能自动视为新增 `DragonTiger` 上下文代码已验证；最新 HEAD 应再跑 Go 全量工作流确认。


### Tushare 指数日 K 成交量溢出边界（2026-10-27）

- 静态审查发现 `float64(math.MaxInt64)` 会舍入为 `2^63`。原来仅拒绝 `rawVolume > float64(math.MaxInt64)` 会让精确的 `2^63` 通过检查，再转 `int64` 时溢出。
- 现改为在转换前拒绝 `rawVolume >= float64(math.MaxInt64)`；越界量保留为 `volume=null`、`volume_unit=unknown`，该条目标为 degraded 并记录稳定原因。
- 新增 `TestTushareIndexDailyBarsRejectsInt64VolumeOverflow` 测试源码。未执行测试、构建、CI 或真实 Provider 请求。


### 国内 Provider 浮点转整数边界复核（2026-10-27）

- 同花顺指数日 K、个股行情/日 K 的成交量和财报时间戳统一在 `int64` 转换前拒绝 `>= float64(math.MaxInt64)` 的边界；对时间戳同时要求整数毫秒值。
- TickFlow 整数解析增加正负 int64 范围检查，避免浮点输入越界后直接转换；腾讯报价成交量的预防性换算边界也改为包含等号的拒绝条件。
- 新增 `numeric_boundary_test.go`，覆盖同花顺、TickFlow 与腾讯的 `2^63` 边界。仅增加测试源码，未执行测试/构建/CI/真实 Provider 请求。


### 补充数据和财务快照的采集时间语义（2026-10-27）

- 修正筹码分布、个股资金流和财务快照对 Provider 回包的 `fetched_at` 处理：该字段表示本服务收到成功回包的 UTC 时间，由服务层统一盖章，不再直接信任上游同名值。避免未来时间污染响应语义，也避免写入缓存后因 `time.Since(fetched_at) < 0` 而持续缓存未命中。
- 筹码分布 `date` 与 `provider_date` 在完成严格 `YYYY-MM-DD` 格式校验后，还必须不晚于中国市场当前日期；未来日期会拒绝该 Provider 结果，不作为可用快照返回。
- 已补充对应回归测试源码；按协作约定，未运行测试、构建、CI 或真实 Provider 请求。


### 核心行情与日 K 时间字段统一由服务层盖章（2026-10-27）

- 实时行情此前只在 Provider 未填写 `fetched_at` / `normalized_at` 时补默认值；历史日 K 同样沿用 Provider 非零时间。静态审查确认，这会允许错误或未来时间进入 API、MariaDB 与 Redis，并使时间语义依赖具体 Provider 的实现方式。
- 现由行情服务在收到成功响应时统一写入 `fetched_at`，并在完成标准化和质量处理后写入 `normalized_at`。供应商时间仍放在单独的行情/日 K 源时间字段内，不复用服务时间字段。
- 已补对应回归测试源码；按约定未执行测试、构建、CI 或真实 Provider 请求。


### 缓存命中时校验服务时间字段（2026-10-27）

- 继 Provider 回包时间统一由服务层盖章后，补充缓存读取门禁：实时行情与日 K 的缓存条目若 `fetched_at` 或 `normalized_at` 缺失或超前当前时间容差，视为不可信缓存并触发上游刷新。
- 历史 K 线的交易日期虽可以很早，但这不等于服务采集时间可以缺失或位于未来；缓存时间检查独立于交易日期和行情源时间。
- 已添加缓存回归测试源码；未运行测试、构建、CI 或真实 Provider 请求。


### 日 K 日期字符串统一采用严格规范（2026-10-27）

- 修正请求入口和共享日 K 校验器：起止日期、Provider/缓存中的交易日期必须严格符合 `YYYY-MM-DD`，仅能被宽松解析但重新格式化后不相等的字符串不再接受。
- 该约束用于保证后续字典序范围筛选、日期排序与缓存键一致，不依赖单一 Provider 自行规范化数据。
- 已补充日期格式回归测试源码；按约定未执行测试、构建、CI 或真实 Provider 请求。


## 财务报告期查询与历史时点边界

- `GET /api/v1/stocks/{symbol}/fundamentals` 只接受 `year` 与 `quarter` 来定位报告期；当前不支持历史修订版本重建。若调用方传入 `as_of`（包括空值），HTTP 层明确返回 400 `VALIDATION_ERROR`，不静默忽略。
- Provider 当前返回请求时可获得的该报告期版本，并保留公告日期用于下游时点门禁。该行为不代表可回放某个历史日期当时已公开的财务数值；未来若建设完整 PIT 能力，需增加历史修订存储并确认源端能提供版本记录。


### 同花顺财务指标与三张报表服务契约

服务层财务校验允许六个标准指标分区外，再附带 `income_statement`、`balance_sheet`、`cash_flow_statement` 三个可选报表分区。先前的“必须正好六个分区”限制会拒绝同时返回指标与报表的同花顺快照；现允许六个标准分区与最多三个报表分区。若同花顺省略标准指标分区，适配器会补充 `unknown` 空分区并记录具体缺失原因，不补零；重复或未知指标能力会在适配器发起报表请求前被拒绝。相关修正尚未运行测试、构建、真实 Provider 请求或 CI。


### 国内证券搜索与未来财务报告期门禁（2026-10-27）

- 证券搜索路由和回归测试源码已存在，但 OpenAPI 曾遗漏 `GET /api/v1/stocks/search` 路径及响应模型；本轮补齐查询参数、错误状态、`StockSearchItem` / `StockSearchSnapshot` schema，并说明 `fetched_at` 不等于实时行情时间。
- 基本面接口现在会在读取缓存或访问 Provider 前拒绝尚未结束的年度/季度报告期，避免空数据分区绕过未来报告期校验。新增服务层与 HTTP 层测试源码覆盖该门禁。
- 本轮未运行测试、构建、CI 或真实 Provider 请求。


### 证券搜索缓存状态头语义（2026-10-27）

- 搜索服务没有独立缓存命中返回值，旧 HTTP handler 却把所有非 stale 响应都标为 `X-Market-Data-Cache: HIT`，可能将刚执行的上游查询误报为服务缓存命中。
- 已改为仅在快照明确 stale 时返回 `X-Market-Data-Cache: STALE`；普通响应不声明缓存命中，并补充回归测试源码。未运行测试、构建、CI 或真实 Provider 请求。


## 当前剩余任务与加速执行顺序（2026-10-27）

本阶段继续只做国内 A 股数据层；海外数据源、真实 Provider 请求、构建/测试/CI 均留到用户统一验收。每个功能批次合并独立能力相关改动、测试源码和文档，集中使用一个中文提交并附 `[skip ci]`，不为无关小改动拆分提交。

### P1：完成国内数据能力与字段语义差异

1. **按能力域核对 Efinance / AkShare 替代完整度**：以行情、日 K、证券搜索/基础资料、所属板块、指数/成分、市场广度、涨跌停池、热度榜、筹码、资金流和增强数据为清单，对照原 Python 实现和现有 Go 路由。Efinance / AkShare 多数是公开接口的组合包装，不再机械复制同名 Provider；确实缺失的返回字段、组合结果或筛选语义才新增代码。
2. **补齐 Pytdx / Tushare / TickFlow 已接入能力的契约差异**：逐项核对交易日期范围、复权支持、成交量/成交额单位、权限失败、分页截断和质量降级。能通过静态代码与 fixture 证明的问题直接修；账号权限、源端完整性和实时延迟只记录待验收，不以代码存在代表可用。
3. **处置公开国内源稳定性边界**：东方财富间歇性 302/502 已记录在 Issue #6；网易行情/日 K 有既往 DNS/502 失败记录。代码侧优先保证超时、错误分类、回退和质量状态明确，不新增无证据的生产可用性结论；真实可达性留到统一验收，不在每批开发中重复请求。

### P2：历史时点与数据可回放能力

1. **财务 Point-in-Time**：当前财务 API 返回请求时可取得的报告期版本，不还原历史修订。先确认上游是否提供历史版本/真实可用时间，再设计版本表、唯一键、缓存键和 `as_of` 查询；在历史数据不可获得前不伪造 PIT 支持。
2. **复权因子与指数成分历史**：现有前/后复权和最新成分快照并不保证可复原历史当时有效的数据。需按来源记录复权算法、因子版本/生效日和成分有效区间，再决定数据结构与回放接口，不能简单取消质量门禁。

### P3：统一验收（由用户安排）

- 一次性执行 Go tests/race/vet、OpenAPI/Compose 校验、容器构建、目标环境健康检查及国内源 smoke test。开发批次不重复启动这些流程。
- 复核配置默认值与根 `.env.example`、`configs/providers.json`、服务级模板一致；对不稳定/缺凭据的数据源显式显示 disabled、unknown 或 degraded，不将配置启用等同于上游可用。
- 合并前集中审查 PR 历史、文档状态、已知问题 Issue #6 和分支差异，再决定合并；在用户统一验收前保持 PR 草稿状态。

### 本轮已收口

- 同花顺指标缺失时显式补充 `unknown` 分区，不补零；未知/重复能力在发起三张财报请求前拒绝。
- 腾讯行情时区回退路径已有可注入测试源码，覆盖极简容器无 zoneinfo 情况。
- 财务报告期查询拒绝尚未结束的未来季度，避免空分区绕过未来报告期检查。
- 证券搜索 OpenAPI 契约已补齐；普通响应不再错误声明缓存命中。
- 以上改动均未运行测试、构建、CI 或真实 Provider 请求。


### TickFlow 核心指数报价补齐（2026-10-27）

- 原 Python TickFlowFetcher 可使用授权的 `quotes.get(symbols=[...])` 获取 6 个 A 股核心指数；Go 适配器此前仅实现个股 Quote 与日 K，未实现现有 `IndexQuoteProvider` 接口。
- 新增独立 `FetchIndexQuotes` 能力，限定 6 个核心指数，不把未知代码混入结果；字段保留来源报价时间，缺少时间/成交额/OHLC 时显式降级，不伪造源时间或补零。
- 默认关闭，新增 `MARKET_DATA_TICKFLOW_INDEX_QUOTES_ENABLED`。该能力需 TickFlow API Key，与免 Key 日 K 分开配置；配置缺 Key 时会在启动校验阶段失败。
- 已补接口夹具与配置门禁测试源码；按协作约定未运行测试、构建、CI 或真实 Provider 请求。市场广度与 SW1 行业榜已按官方 SDK 暴露的证券池和 universe 行情接口接入 Go 适配器，均默认关闭且需授权 Key；SW1 输出为成分股涨跌幅均值，并非源端行业指数行情。

### 国内 Provider 静态契约复核（2026-10-27）

- Tushare 日线的 `pct_chg` 不是 OHLC 主字段，但缺失时会影响涨跌幅完整性。当前适配器此前仅在字段存在但解析失败时降级，字段缺失或为 `null` 时可能保留 `valid`；本轮将两种情况统一标记 `degraded`，涨跌幅继续保留为空，不补零。
- TickFlow 报价时间按毫秒 Unix 时间戳处理。个股行情和核心指数行情统一拒绝早于 2000-01-01 或晚于当前时间 5 分钟以上的时间戳，且将其标记为缺失/无效；不设置固定“陈旧”阈值，避免将休市/盘后最后有效快照误判。
- 仍未完成的国内差异：TickFlow 市场广度已按官方 SDK 的 `GET /v1/universes/CN_Equity_A` 与 `GET /v1/quotes?universes=CN_Equity_A` 契约接入，先校验证券池成员，再要求返回唯一行情覆盖完整；默认关闭，用户 Key 的实时行情/证券池权限与目标环境仍待验收。申万一级行业榜已按 `CN_Equity_SW1_*` 证券池实现，输出为成分股涨跌幅均值，不等同于源端提供的直接行业指数收益；默认关闭，权限与源端覆盖待验收。Tushare / Pytdx / TickFlow 的字段单位、历史覆盖与回退语义仍需完成全量静态对照。历史财务修订、历史成分生效区间和复权因子时点重建依赖更完整的来源数据或存储设计，不能用最新值替代。
- 本轮只增加代码与测试源码，按协作约定未运行测试、构建、CI 或真实 Provider 请求。

### TickFlow 全市场 A 股涨跌统计接入（2026-10-27）

- 根据 TickFlow 官方 SDK 的接口契约，使用 `GET /v1/universes/CN_Equity_A` 读取证券池清单及 `symbol_count`，再调用 `GET /v1/quotes?universes=CN_Equity_A` 读取实时行情；两个请求均通过 API Key 鉴权。
- 统计前校验证券池数量、规范化后的唯一证券成员以及行情覆盖是否完整。缺证券、重复/未知代码、混合报告日期或没有有效来源日期时整体失败并让服务层回退，绝不把局部行情冒充全市场。
- 涨跌/平盘按最新价与昨收价计算；成交额仅在所有证券均有有效金额时汇总，金额覆盖不完整时保留覆盖数并将总额置空。涨跌停数量按原 Python A 股规则计算（北交所 30%、科创/创业板 20%、ST 5%、普通主板 10%），主板证券缺失名称、无法判定 ST 状态时不输出涨跌停总数。
- 新能力由 `MARKET_DATA_TICKFLOW_MARKET_BREADTH_ENABLED` 独立控制，默认关闭；启用时启动配置要求 TickFlow API Key。官方 SDK 契约不代表具体账号已开通对应接口权限。
- 新增隔离的 HTTP fixture 与配置门禁测试源码；按协作约定未运行测试、构建、CI 或真实 Provider 请求。

### Pytdx 保留交易所前缀，避免代码歧义（2026-10-27）

- 通达信 Go 客户端的 `GetQuote`、`GetKlineDay` 和 `GetGbbq` 支持带交易所前缀的代码；先前适配器却把 `600519.SH` 等标识裁剪为纯数字，让底层 `AddPrefix` 再根据代码形态猜测交易所。
- 本轮改为传入 `sh600519` / `sz000001` / `bj920000` 形式，并在实时行情响应中同时匹配代码和交易所；个股 Provider 入口还会拒绝指数代码，避免 `000001.SH` 被当成个股日 K 查询。
- 新增纯单元测试源码覆盖前缀映射、同代码错交易所与指数代码拒绝。按约定未运行测试、构建、CI 或真实 TDX 连接。

### 国内行情单位与证券匹配边界（2026-10-27）

- Pytdx 现在将来源返回的代码和交易所同时纳入实时报价匹配，拒绝同代码但不同交易所的响应；对应隔离单测覆盖沪深市场歧义和缺少 K 线数据。
- Pytdx 日 K 的价格/成交量可以独立保留；当来源成交额与成交量出现“一个为零、另一个为正”的矛盾时，现将归一化成交额置空并降级，而不让标准化层因此丢弃整根 K 线。原始成交额与单位仍保留用于排查。
- Tushare 日线 `vol` 的接口口径为“手”，只有整数手数才允许换算成股数。之前通用转换会对小数手数四舍五入，本轮改为拒绝小数成交量并触发上游回退，防止把格式异常数据悄悄改成看似有效的成交股数。
- 以上均为静态代码修正和测试源码补充；未执行测试、构建、CI 或真实 Provider 请求。

### 公共手单位成交量换算拒绝小数输入（2026-10-27）

- `normalize.HandsToShares` 现在统一要求来源“手”数为非负整数后再换算成股数；拒绝小数手数、非有限值和溢出值。
- 此门禁覆盖按“手”返回成交量的东方财富报价/日 K、腾讯按手口径分支、Pytdx 和 Tushare 等共用转换路径。TickFlow 的 A 股实时行情与日 K 原始成交量单位是“股”，必须使用 `RawSharesToInt64` 直接校验，不能进入手数换算路径。
- Tushare 保留 Provider 级错误上下文，具体原因由公共归一化错误返回；已扩展公共转换测试和 Tushare 数据夹具源码。未运行测试、构建、CI 或真实 Provider 请求。

### TickFlow 成交量单位修正（2026-10-27）

- 原项目同一 TickFlow 数据源的修复记录明确说明：A 股实时行情和日 K 的 `volume` 返回值已经是“股”，不是“手”；错误地乘以 100 会把成交量放大 100 倍。参考原项目修复记录：`https://github.com/ZhuLinsen/daily_stock_analysis/pull/2128`。
- Go 适配器现将 TickFlow 报价与日 K 的成交量按股数直接校验为非负整数，不再调用 `HandsToShares`；`raw_volume_unit` 和归一化 `volume_unit` 均明确为 `shares`。东方财富、腾讯等其他来源仍按各自单位契约转换。
- 将新增 Quote 与日 K HTTP 夹具，验证原始股数与标准化股数保持一致；未发起真实 TickFlow 请求，未运行测试、构建或 CI。

### 公共股单位成交量换算拒绝小数输入（2026-10-27）

- `normalize.RawSharesToInt64` 现在和 `HandsToShares` 一样只接受非负整数股数，不再对小数股数四舍五入。
- 该校验覆盖 TickFlow A 股实时行情/日 K，以及腾讯行情单位推断与科创板日 K 的原始股数路径。遇到小数股数会拒绝该 Provider 结果并进入已有回退，而不是悄悄修改成交量。
- 已补公共转换测试源码；未执行测试、构建、CI 或真实 Provider 请求。

### TickFlow 申万一级行业榜接入（2026-10-27）

- 基于官方 SDK 所暴露的 `GET /v1/universes`、`GET /v1/universes/CN_Equity_A`、`POST /v1/universes/batch` 和 `GET /v1/quotes?universes=CN_Equity_A` 组合实现。
- 只接受 `CN_Equity_SW1_*` 且名称以 `SW1` 开头的证券池；代码从 A 股全集白名单校验，行业详情缺失、重复成员、非 A 股代码、行情漏行或跨报告日时整体失败并回退。
- 行业涨跌幅沿用原 Python TickFlowFetcher 的口径：对该 SW1 证券池内有有效行情的成分股涨跌幅取算术平均。因为这不是源端直接提供的行业指数涨跌幅，且多请求采集并非原子快照，结果固定为 `degraded`。
- 新能力由 `MARKET_DATA_TICKFLOW_SECTOR_RANKINGS_ENABLED` 独立控制，默认关闭；启用需要 API Key。概念板块不由该能力提供，仍使用既有概念来源。
- 已补充 HTTP fixture 与配置门禁测试源码；本轮未运行测试、构建、CI 或真实 Provider 请求。

### Pytdx 成交额辅助声明位置修复（2026-10-27）

- 对最新 Pytdx 成交额质量降级代码进行静态复核时，发现辅助类型与函数声明被插入到 `pytdxDomesticSymbol` 函数体内部，导致 Go 源文件无法解析。
- 已将辅助声明移到文件级，并补齐成交额/成交量组合校验：当一个为零、另一个为正时保留原始金额并将归一化金额置空、标记 degraded；非法金额或非整数手数直接拒绝。
- 新增独立测试源码覆盖正常金额、两类零值矛盾和非法数字边界。未运行测试、构建或 CI。
