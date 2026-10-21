# A 股分析系统 Go API（可启动测试版）

将本目录整体放进现有 `daily_stock_analysis` 仓库的独立 `go/` 子目录中，例如 `daily_stock_analysis/go/`；不要把 Go 源码混进原 Python 目录。所有 `go` 命令和 `docker compose -f docker-compose.go.yml ...` 命令都在这个 Go 模块根目录执行。

本子项目提供可启动的 HTTP API、现有 Python 行情 API 适配器、MariaDB 持久化、Redis 缓存，以及一条保守的基础分析流程。

> 当前版本可做端到端工程与数据链路测试，不等于成熟的交易系统：趋势策略只是可解释的基线；上游数据缺少逐条 provider、available_at、复权口径，以及 ST/停牌/涨跌停等完整交易状态，所以历史回放会被质量门禁阻断，交易计划会被硬风险门禁阻断。系统不会真实下单，也不会伪造成交。

## 目录说明

- `cmd/analysis-api`：正式 Go HTTP API 入口，启动时连接 MariaDB/Redis，并装配真实 Python API 采集器。
- `cmd/storage-check`：只检查 DB/Redis 连接，并按配置执行迁移。
- `cmd/pythonapi-smoke`：单独探测当前 Python API。
- `cmd/demo`：纯内存假数据演示，不连接 DB/Redis，不能当真实结果。
- `internal/adapters/pythonapi`：调用 Python API 获取实时行情、日 K 和数据诊断。
- `internal/adapters/storage/mysqlredis`：MariaDB 运行/事件/快照/结果持久化，Redis 结果与快照缓存。
- `internal/analysis`：主流程 Runner、领域对象、模块注册与分析引擎。
- `internal/service`：当前 MVP 的真实依赖装配、基础标准化/数据门禁、市场与个股趋势分析。
- `internal/platform/httpapi`：HTTP API、请求参数校验与测试。

## 快速启动

### 1. 修改 `.env`

最少确认这些参数真实可用：

```dotenv
HTTP_ADDR=:8092
PYTHON_API_BASE_URL=http://127.0.0.1:8000
DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=stock_analysis
DB_PASSWORD=替换为真实数据库密码
DB_NAME=stock_analysis
DB_AUTO_MIGRATE=true
REDIS_ADDR=127.0.0.1:6379
REDIS_PASSWORD=
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

`storage-check` 成功表示 MariaDB/Redis 可以连接；如果 `DB_AUTO_MIGRATE=true`，同时会创建 `analysis_runs`、`analysis_stage_events`、`analysis_snapshots` 表。

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

### 5. 查询结果和审计事件

从 POST 返回值中复制 `run_id`：

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

脚本会检查 Go 存活、DB/Redis/表结构就绪状态，提交一次真实分析，随后读取持久化结果和审计事件。如果上游 Python API 数据不可用、质量门禁阻断或接口返回错误，脚本会输出错误并失败，不会把失败假报为通过。

### 6. 验证 MariaDB 持久化

在 MariaDB 中执行：

```sql
SELECT run_id, status, task_type, as_of, started_at, finished_at
FROM analysis_runs ORDER BY started_at DESC LIMIT 10;

SELECT run_id, stage, status, event_at, duration_ms
FROM analysis_stage_events ORDER BY id DESC LIMIT 30;

SELECT run_id, snapshot_type, snapshot_id, as_of, created_at
FROM analysis_snapshots ORDER BY created_at DESC LIMIT 20;
```

## API 清单

| 方法 | 路径 | 作用 |
|---|---|---|
| `GET` | `/healthz` | 进程存活检查 |
| `GET` | `/readyz` | MariaDB + Redis 就绪检查 |
| `POST` | `/api/v1/analysis/runs` | 同步执行分析并持久化结果 |
| `GET` | `/api/v1/analysis/runs/{run_id}` | 查询已保存结果（Redis 优先、MariaDB 兜底） |
| `GET` | `/api/v1/analysis/runs/{run_id}/events` | 查询阶段审计事件 |

当前 API 没有内置用户认证。请先仅在内网或可信 Docker 网络访问；需要对外发布时，必须配置反向代理鉴权、HTTPS 和来源限制，不要把 8092 端口直接暴露到公网。

## Docker 部署

提供 `Dockerfile` 和 `docker-compose.go.yml`。在 `1panel-network` 网络已存在的情况下，先设置 Compose 使用的 `GO_DB_HOST`、`GO_REDIS_ADDR`、`GO_PYTHON_API_BASE_URL`，然后：

```bash
docker compose -f docker-compose.go.yml up -d --build
docker compose -f docker-compose.go.yml logs -f stock-analysis-go
```

`GO_PYTHON_API_BASE_URL` 必须填写原 Python 服务在该 Docker 网络中的实际服务名和端口；默认值只是示例。DB/Redis 同样必须是在该网络中可解析的主机名。若使用主机进程启动（`go run`），这些 `GO_*` 覆盖变量不需要使用。

## 当前边界

- MariaDB 已用于运行记录、阶段审计事件和原始/标准化/组合快照持久化；Redis 已用于结果/快照/通用缓存和就绪检查。
- 模拟组合目前是空的、未配置的组合快照（资金为 0），没有模拟账户/持仓账本。
- 数据门禁要求至少一只股票有 21 根有效日 K。由于上游缺少 provider、available_at 和调整口径等字段，普通扫描会标记为 partial；历史回放会阻断。
- 市场分析目前使用自选股相对 20 日均线的横截面指标和可获得的指数日 K，不应将它理解为完整市场广度、行业轮动或资金流分析。
- 趋势基线查看收盘价相对 MA20 以及 5/20 个交易间隔收益，只生成研究候选；交易可行性和风险门禁在当前 MVP 中拒绝生成仓位计划。
- 尚未实现公司行动/复权、行业及财务因子、公告新闻融合、完整交易制度库、模拟成交账本、可靠 PIT 历史回测与收益验收。

中文细节见 `docs/数据库与Redis接入-中文.md`、`docs/Go-Python-API-adapter-中文.md`、`docs/本地测试与使用说明-中文.md`。
