# A 股分析系统 Go API（可启动测试版）

本仓库 `Kasign/stock_data_provider` 将原生 Go 行情服务与 Go 分析主流程放在同一仓库内，但以独立 Go module、配置和容器隔离运行；旧 Python 能力仅保留在过渡 bridge 中，不混入 Go 源码目录。根 module、`go-core`、`services/market-data` 分别运行自己的 Go 命令。

本子项目提供可启动的 Go HTTP API、原生 Go market-data HTTP 适配器、MariaDB 持久化、Redis 缓存，以及一条保守的基础分析流程。主 API 的实时行情和日 K 从 Go market-data 服务获取，不再回退到旧 Python API。

> 当前版本可做端到端工程与数据链路测试，不等于成熟的交易系统：趋势策略只是可解释的基线；Go market-data 已提供行级来源和可声明的复权口径，但还没有精确的 publication available_at、完整 ST/停牌/涨跌停规则，也未迁移全市场广度、板块、筹码、财务、资金流、龙虎榜、新闻公告等所有数据域，所以历史回放会被质量门禁阻断，交易计划会被硬风险门禁阻断。系统不会真实下单，也不会伪造成交。

## 目录说明

- `cmd/analysis-api`：正式 Go HTTP API 入口，启动时连接 MariaDB/Redis，并装配原生 Go market-data 采集器。
- `cmd/storage-check`：只检查 DB/Redis 连接，并按配置执行迁移。
- `cmd/pythonapi-smoke`：旧 Python API 的可选诊断工具，不参与主 API 的数据采集。
- `cmd/demo`：纯内存假数据演示，不连接 DB/Redis，不能当真实结果。
- `internal/adapters/marketdata`：主 API 使用的原生 Go 行情 API 客户端和快照采集器。
- `internal/adapters/pythonapi`：保留的旧适配器，仅用于迁移对照和现存测试；主 API 不使用它作为 fallback。
- `internal/adapters/storage/mysqlredis`：MariaDB 运行/事件/快照/结果持久化，Redis 结果与快照缓存。
- `internal/analysis`：主流程 Runner、领域对象、模块注册与分析引擎。
- `internal/service`：当前 MVP 的真实依赖装配、基础标准化/数据门禁、市场与个股趋势分析。
- `internal/platform/httpapi`：同步与异步 HTTP API、请求参数校验与测试。\n- `internal/platform/taskqueue`：MariaDB 持久化任务队列、有界 Worker、状态查询和事件增量读取。

## 快速启动

### 1. 修改 `.env`

最少确认这些参数真实可用：

```dotenv
HTTP_ADDR=:8092
MARKET_DATA_BASE_URL=http://127.0.0.1:8091
MARKET_DATA_API_KEY=替换成不少于 32 字符的随机密钥
MARKET_DATA_ADJUSTMENT=forward_adjusted
# market-data 服务独立数据库配置；程序内部生成 MySQL DSN
MARKET_DATA_DB_HOST=Mariadb
MARKET_DATA_DB_PORT=3306
MARKET_DATA_DB_USER=stock_analysis_server
MARKET_DATA_DB_PASSWORD=替换为真实数据库密码
MARKET_DATA_DB_NAME=stock_analysis_server
MARKET_DATA_DB_CHARSET=utf8mb4
MARKET_DATA_DB_TIMEZONE=UTC
MARKET_DATA_REDIS_ADDR=Redis:6379
MARKET_DATA_REDIS_PASSWORD=真实 Redis 密码
DB_HOST=Mariadb
DB_PORT=3306
DB_USER=stock_analysis_server
DB_PASSWORD=替换为真实数据库密码
DB_NAME=stock_analysis_server
DB_AUTO_MIGRATE=true
REDIS_ADDR=Redis:6379
REDIS_PASSWORD=替换为真实 Redis 密码
REDIS_DB=0
```

`DB_AUTO_MIGRATE=true` 用于首次测试自动创建本项目表；迁移成功后，可以按你的发布流程改为 `false`。不要将真实 `.env` 提交到 Git。若 Go API 在 Docker 内运行，请使用 Docker 网络中的服务名，而不是把 `127.0.0.1` 当成其他容器。

### 2. 准备 Go 依赖并检查存储

Go 1.22+，在本目录执行：

```bash
go mod download
go test ./...
go vet ./...
go test -race ./...
go run ./cmd/storage-check
```

`storage-check` 成功表示 MariaDB/Redis 可以连接；如果 `DB_AUTO_MIGRATE=true`，同时会创建 `analysis_runs`、`analysis_stage_events`、`analysis_snapshots`、`analysis_tasks` 表。

### 3. 启动 Go API

```bash
go run ./cmd/analysis-api
```

默认监听 `:8092`。新终端检查：

```bash
curl -i http://127.0.0.1:8092/healthz
curl -i http://127.0.0.1:8092/readyz
```

- `/healthz` 只检查进程是否存活。
- `/readyz` 同时检查 MariaDB 和 Redis 是否可用。

### 4. 提交真实分析请求

最小请求示例（默认分析时点为服务端当前的上海时间）：

```bash
curl -sS -X POST 'http://127.0.0.1:8092/api/v1/analysis/runs' \
  -H 'Content-Type: application/json' \
  -d '{"symbols":["600519"]}'
```

可一次提供多只六位 A 股代码，但每次最多 100 只。也可以显式指定任务和分析时点：

```json
{
  "task_type": "post_market_scan",
  "as_of": "2026-10-20T15:00:00+08:00",
  "universe_id": "my-watchlist-v1",
  "symbols": ["600519", "000001", "300750"],
  "portfolio_id": "paper-unconfigured",
  "market_id": "CN-A",
  "strategy_ids": ["trend-following"],
  "strategy_config_version": "trend-following-mvp-v1",
  "risk_config_version": "unverified-tradability-block-v1",
  "trading_rules_version": "not-configured-v1",
  "data_policy_version": "baseline-quality-v1"
}
```

以下版本字段和默认值是为了固定每次运行的规则标识，并不代表已经建立了完整的交易规则库。当前只注册 `trend-following` 这一条基线策略。

### 5. 异步任务与步骤进度（推荐新调用方使用）

新接口先把任务写入 MariaDB，再返回 `202 Accepted` 与 `task_id`；有界 Worker 在后台执行分析。原同步接口仍保留。

```bash
# 创建任务：可选 Idempotency-Key，重试相同请求时避免重复创建
curl -i -X POST 'http://127.0.0.1:8092/api/v1/analysis/tasks' \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: watchlist-scan-20261021-001' \
  -d '{"symbols":["600519","000001"]}'

# 查询状态；状态为 queued/running/succeeded/partial/failed
curl -sS 'http://127.0.0.1:8092/api/v1/analysis/tasks/<task_id>'

# 首次查询阶段事件；下一次把 next_after_sequence 作为 after_sequence
curl -sS 'http://127.0.0.1:8092/api/v1/analysis/tasks/<task_id>/events?after_sequence=0&limit=100'
```

状态中包含 `run_id`、当前/最近阶段、已完成阶段数和失败码。事件的 `sequence` 来自 MariaDB 的持久化阶段事件 ID；调用方保存 `next_after_sequence` 即可增量续读。阶段事件只在 Runner 实际执行并持久化后出现，排队期间事件数组可以为空；不提供伪造百分比。SSE 和取消接口尚未实现。

若继续使用同步接口，复制返回的 `run_id` 查询：

```bash
curl -sS 'http://127.0.0.1:8092/api/v1/analysis/runs/<run_id>'
curl -sS 'http://127.0.0.1:8092/api/v1/analysis/runs/<run_id>/events'
```

结果首先从 Redis 读取；缓存未命中、缓存不可解析或 Redis 读取失败时，回退到 MariaDB。MariaDB 是最终事实源。分析结果和快照写入 MariaDB 成功后，Redis 缓存写入失败只记录警告，不会把已经持久化的结果改判为失败。

### 一键执行在线冒烟测试

Go API 启动后，在项目目录执行：

```bash
./scripts/full-smoke-test.sh
# 可指定自选股，例如多个代码以逗号分隔
SYMBOLS=600519,000001 ./scripts/full-smoke-test.sh
```

脚本会检查 Go 存活、DB/Redis/表结构就绪状态，提交一次真实分析，随后读取持久化结果和审计事件。如果 Go market-data API、底层数据源不可用，质量门禁阻断或接口返回错误，脚本会输出错误并失败，不会把失败假报为通过。

### 6. 验证 MariaDB 持久化

在 MariaDB 中执行：

```sql
SELECT run_id, status, task_type, as_of, started_at, finished_at
FROM analysis_runs ORDER BY started_at DESC LIMIT 10;

SELECT run_id, stage, status, event_at, duration_ms
FROM analysis_stage_events ORDER BY id DESC LIMIT 30;

SELECT run_id, snapshot_type, snapshot_id, as_of, created_at
FROM analysis_snapshots ORDER BY created_at DESC LIMIT 20;

SELECT task_id, run_id, status, task_type, failure_code, created_at, started_at, finished_at
FROM analysis_tasks ORDER BY created_at DESC LIMIT 20;
```

## API 清单

| 方法 | 路径 | 作用 |
|---|---|---|
| `GET` | `/healthz` | 进程存活检查 |
| `GET` | `/readyz` | MariaDB + Redis 就绪检查 |
| `POST` | `/api/v1/analysis/runs` | 兼容用：同步执行分析并持久化结果 |
| `POST` | `/api/v1/analysis/tasks` | 创建异步任务，返回 202 + task_id |
| `GET` | `/api/v1/analysis/tasks/{task_id}` | 查询异步任务状态与当前阶段 |
| `GET` | `/api/v1/analysis/tasks/{task_id}/events?after_sequence=N` | 增量查询已持久化阶段事件 |
| `GET` | `/api/v1/analysis/runs/{run_id}` | 查询已保存结果（Redis 优先、MariaDB 兜底） |
| `GET` | `/api/v1/analysis/runs/{run_id}/events` | 查询阶段审计事件 |

当前 API 没有内置用户认证。请先仅在内网或可信 Docker 网络访问；需要对外发布时，必须配置反向代理鉴权、HTTPS 和来源限制，不要把 8092 端口直接暴露到公网。

## Docker 部署

提供 `Dockerfile` 和 `docker-compose.go.yml`。Compose 会同时启动 `market-data` 与 `stock-analysis-go` 两个 Go 服务。先将 `.env.example` 复制为 `.env`，生成同一个不少于 32 字符的随机 `MARKET_DATA_API_KEY` 给两个服务使用，分别设置 `MARKET_DATA_DB_*` / `MARKET_DATA_REDIS_*` 与分析 API 的 `DB_*` / `REDIS_*` 连接字段，指向 `1panel-network` 上现有的 MariaDB 与 Redis，然后：

```bash
docker compose -f docker-compose.go.yml up -d --build
docker compose -f docker-compose.go.yml logs -f stock-analysis-go
```

`market-data` 服务复用现有 MariaDB/Redis，不需要再部署数据库容器。Compose 自动将分析 API 指向 `http://market-data:8091`；不要在 API 容器中填 `127.0.0.1:8091`。另外根 API 的 `DB_HOST`、`REDIS_ADDR` 也必须配置为容器网络可解析的现有服务名。若用宿主机进程运行 `go run ./cmd/analysis-api`，则将 `MARKET_DATA_BASE_URL` 改为 `http://127.0.0.1:8091`，并先单独启动 market-data 服务。

## 当前边界

- MariaDB 已用于异步任务、运行记录、阶段审计事件和原始/标准化/组合快照持久化；Redis 已用于结果/快照/通用缓存和就绪检查。
- 模拟组合目前是空的、未配置的组合快照（资金为 0），没有模拟账户/持仓账本。
- 数据门禁要求至少一只股票有 21 根有效日 K。主 Go API 可使用行级来源、质量信息和明确的复权口径；由于缺少严格的 publication available_at 等点时信息，普通扫描仍会标记为 partial，历史回放会阻断。
- 市场分析目前使用自选股相对 20 日均线的横截面指标和可获得的指数日 K，不应将它理解为完整市场广度、行业轮动或资金流分析。
- 趋势基线查看收盘价相对 MA20 以及 5/20 个交易间隔收益，只生成研究候选；交易可行性和风险门禁在当前 MVP 中拒绝生成仓位计划。
- 尚未实现公司行动/复权、行业及财务因子、公告新闻融合、完整交易制度库、模拟成交账本、可靠 PIT 历史回测与收益验收。

异步任务接口契约见 `docs/architecture/analysis-tasks.openapi.yaml`。完整 Python Provider 迁移缺口见 `docs/architecture/PYTHON_PROVIDER_GO_MIGRATION_MATRIX.md`。旧 Python 适配器说明见 `docs/Go-Python-API-adapter-中文.md`；其不再是主 API 的行情入口。

## 后续架构规划与工程规范

- 当前架构 Review 与后续修改约束：`docs/architecture/ARCHITECTURE_REVIEW_AND_ENGINEERING_RULES.md`
- 结构化日志和任务步骤状态：`docs/architecture/LOGGING_AND_TASK_PROGRESS_PLAN.md`
- Go 数据源切换进度：`docs/architecture/GO_DATA_PROVIDER_SWITCH_STATUS.md`
- 同花顺候选接入参考：`https://github.com/Kasign/Financial-API`。当前仅进入规划，尚未接入生产链路。
