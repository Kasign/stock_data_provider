# Go 数据服务迁移状态与验收边界

> 更新时间：2026-10-22  
> 当前分支：`feat/full-go-data-providers-api-switch`  
> 当前定位：分阶段迁移；不是全 Python 去依赖的最终版本。

完整设计见 [Go 行情数据服务迁移方案](./GO_MARKET_DATA_MIGRATION_PLAN.md)，Review 与警示见 [架构 Review 记录与工程规范](./ARCHITECTURE_REVIEW_AND_ENGINEERING_RULES.md)，数据域明细见 [Python Provider 到 Go 迁移矩阵](./PYTHON_PROVIDER_GO_MIGRATION_MATRIX.md)。

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
| 实时行情 | 东方财富、腾讯、新浪 | Go API 主链路；新浪仅报价 |
| 日线 K 线 | 东方财富、腾讯 | Go API 主链路；新浪不参与 K 线 |
| 筹码分布 | 妙想 MX_API | 已新增可选 SupplementProvider、HTTP 路由和缓存；默认关闭 |
| 个股主力资金流 | 妙想 MX_API | 已新增可选 SupplementProvider、HTTP 路由和缓存；5/10 日合计必须有完整观测 |

MX 相关接口：

- `GET /api/v1/stocks/{symbol}/chip-distribution`
- `GET /api/v1/stocks/{symbol}/capital-flow`

启用条件是显式设置 `MARKET_DATA_MX_ENABLED=true` 并配置 `MARKET_DATA_MX_API_KEY`（兼容原变量 `MX_APIKEY`）。筹码和资金流尚不能证明精确 publication/available_at，返回值必须保留 degraded 质量状态；不得用于严格的历史时点分析。

### 日志与任务事件

- 根分析 API 增加 JSON slog、X-Request-ID 传播、HTTP 状态码/耗时/响应字节记录。
- 根 API 记录安全的任务输入/输出摘要；Runner 每个 stage 记录开始、完成/失败和耗时日志。
- market-data HTTP 层记录 request ID、path、method、状态码、耗时与响应字节数；MX 请求记录数据集、标的、状态和耗时，不记录 key 或查询正文。
- MariaDB stage event 已有持久化与查询接口；异步任务 API 已提供 `task_id`、状态查询和阶段事件轮询，SSE 与前端实时任务 UI 尚未实现。详见 [日志与任务步骤进度方案](./LOGGING_AND_TASK_PROGRESS_PLAN.md)。

## 2. 明确尚未完成

- 原 Python Provider 的完整替换：Efinance、AkShare、Pytdx、Baostock、Tushare、TickFlow、YFinance、Longbridge、Futu、Finnhub、Alpha Vantage 等仍需逐一对照上游源码、权限、优先级、单位和失败语义。
- 全市场指数/广度、行业/概念板块、涨停池、财务、完整资金流口径、龙虎榜、新闻公告等 Go 原生数据域未全部实现。
- `go-core` 的 market overview/新闻/LLM/部分持久化及通知仍有 Python bridge 依赖。
- 同花顺接入尚未实现，已列入计划；参考仓库 [Kasign/Financial-API](https://github.com/Kasign/Financial-API)，必须先核验真实 API 权限、速率限制和字段口径。
- 已实现 MariaDB `analysis_tasks`、有界 Go Worker、`POST /api/v1/analysis/tasks` 的 202 + task_id、任务状态查询和 `after_sequence` 阶段事件轮询；SSE、取消、自动重试与前端状态展示仍未完成。重启时遗留的 running 任务会显式标记为 `worker_interrupted`，不自动重跑。
- 尚未在目标服务器验证真实供应商连接、MariaDB/Redis 权限和 Docker Compose 完整部署；CI 绿色不能替代真实网络验收。

## 3. 构建与自动化测试状态

GitHub Actions 运行 [39583673094（run #84）](https://github.com/Kasign/stock_data_provider/actions/runs/39583673094) 已在根 module、`go-core` 和 `services/market-data` 三个模块中分别通过 `go test ./...`、`go test -race ./...` 和 `go vet ./...`。该运行验证了最新 Go 源码，包括 MX 配置与接口、请求 ID/JSON 日志和中文注释调整。

异步任务模块加入后，GitHub Actions 运行 [39586848833（run #120）](https://github.com/Kasign/stock_data_provider/actions/runs/39586848833) 再次在三个 Go module 中通过 `go test ./...`、`go test -race ./...` 和 `go vet ./...`，覆盖了任务队列、HTTP 状态/事件轮询及 panic 恢复代码。配置拆分与 `gofmt` 检查调整后，运行 [39711711489（run #140）](https://github.com/Kasign/stock_data_provider/actions/runs/39711711489) 在三个 Go module 中通过检查；随后复权默认值共享逻辑的回归测试也在 [39712383338（run #147）](https://github.com/Kasign/stock_data_provider/actions/runs/39712383338) 通过。两个运行均覆盖格式检查、依赖下载、测试、竞态检测和 `go vet`。CI 不执行真实 MariaDB SQL migration 或供应商端到端测试。

提交历史随后会整理为两个功能性提交（代码/配置/测试与文档/Review 分开）；整理只改变 commit history，不改变已测试的文件内容。**CI 通过不代表真实 MX Key/上游网络、东方财富/腾讯/新浪生产数据、目标服务器 MariaDB/Redis 或 Docker Compose 已完成端到端验收。**

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

## 4. 下一步顺序

1. 在真实环境做 MX/API 与 MariaDB/Redis smoke test，并记录实际权限、字段单位与质量状态。
2. 按迁移矩阵逐项增加市场广度、板块和财务等 Go 数据域；每个数据域先冻结契约再实现。
3. 在目标环境验收异步任务表/Worker/202 + task_id/状态与事件轮询，随后再实现 SSE 和前端状态展示。
4. 按 [架构 Review 记录与工程规范](./ARCHITECTURE_REVIEW_AND_ENGINEERING_RULES.md) 推进日志字段统一、THS Provider 与剩余数据域迁移。
