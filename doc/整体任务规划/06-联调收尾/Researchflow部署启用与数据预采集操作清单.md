# Researchflow 部署启用与数据预采集操作清单

> 适用环境：部署机 `192.168.3.101` 上的 Root Server 单入口（docker-compose，web:8093 → server:8092）。
> 责任边界：本文解决**服务端**（新路由 404 + 缺数据），由用户在部署机执行；**前端主流程**（旧 API 401）由代码改造组交付。两者都完成后，页面才有数据。
> 生成日期：2026-11-28。所有结论均来自部署态实测 + 源码取证，未臆测配置值。

## 0. 诊断基线（2026-11-28 实测）

| 端点 | 实测响应 | 含义 |
|---|---|---|
| `/healthz` | 200 | nginx + Root Server 存活 |
| `/api/readyz` | `401 {"code":"server.session_unauthenticated"}` | Owner Session 已启用（走 session 分支） |
| `/api/v1/analysis/tasks/probe` | `401 WWW-Authenticate: Bearer` | 旧链是 Bearer-only，浏览器 Session 无权访问 → 页面无数据的直接原因 |
| `/api/v1/researchflow/runs` | **404** | Researchflow 路由未注册 → 新服务在部署态没起来 |

404 的代码根因（`internal/server/bootstrap/bootstrap.go:166-167`、`router.go:111-118`）：
Researchflow 路由只在 `ownerSession.researchflow != nil` 时注册，而该字段依赖 `ResearchflowCapacity.Configured == true`，后者要求 `RESEARCHFLOW_MAX_QUEUED_RUNS` 与 `RESEARCHFLOW_MAX_WORKERS` **同时显式配置**（`internal/server/config/researchflow.go:24-43`）。`.env.example:386-392` 中这三项均为注释状态。

## 1. 前置：确认部署镜像版本

在部署机执行，确认 server 镜像包含本轮 `repair_all` 代码（Researchflow 路由注册代码在当前 HEAD）：

```bash
cd <部署目录>
docker images --format '{{.Repository}}:{{.Tag}} {{.ID}} {{.CreatedAt}}' | grep stock-analysis
git -C <源码目录> rev-parse HEAD        # 期望与仓库 repair_all HEAD 一致
```

若镜像早于本轮提交，须先 `docker compose build stock-analysis-server && docker compose up -d` 重建，否则即使配好 env 也仍是 404（旧二进制没有路由代码）。

## 2. 启用 Researchflow 路由（解 404）

编辑部署机 `.env`，去掉以下注释并填入显式值（禁止留默认/部分配置，否则 Server 启动 fail-closed）：

```dotenv
# 数据库全局容量，两项必须同时配置且为正整数
RESEARCHFLOW_MAX_QUEUED_RUNS=100
RESEARCHFLOW_MAX_WORKERS=8
# Run 绝对 Deadline，启用 Capacity 后必填
RESEARCHFLOW_RUN_DEADLINE=45m
# 仅 Worker/All 角色生效，控制本进程 WorkerPool 并发（1-32）
RESEARCHFLOW_LOCAL_WORKERS=2

# Worker 领域服务凭据（Quant SMA20 Stage 以 quant 身份读数据，仅 data:read）
# 生成独立随机值：openssl rand -hex 32
PLATFORM_SERVICE_CREDENTIALS=[{"name":"quant","token_env":"QUANT_SERVICE_TOKEN","scopes":["data:read"]}]
QUANT_SERVICE_TOKEN=<openssl rand -hex 32 生成的值>
```

约束：
- `SERVER_ROLE` 未设置时默认 `all`（`internal/server/config/config.go:22`），会同进程启动 Worker；若拆分为 api/worker 两个实例，api 角色**不启动** Worker（`bootstrap.go:292-310`）。
- `QUANT_SERVICE_TOKEN` 不得复用 `PLATFORM_ADMIN_TOKEN`。
- 若配置了 `NEWS_FEED_SOURCES_JSON`，也必须同时配置 Capacity（否则启动报错）。

重启并验证：

```bash
docker compose up -d --force-recreate stock-analysis-server
# 验证标准：从 404 变为 401（说明路由已注册，只差浏览器身份）
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8092/api/v1/researchflow/runs
```

返回 `401`（而非 404）即表示路由已注册成功。

## 3. 确认 Owner Session 与登录

`docker-compose.yml:12-18` 已固定 `OWNER_SESSION_ENABLED=true`、Cookie Path `/api`。只需确认 `.env` 中以下项已填（缺失会导致启动失败，当前能起说明已满足）：

```dotenv
OWNER_SESSION_PUBLIC_ORIGIN=<浏览器访问的精确 Origin，如 http://192.168.3.101:8093>
OWNER_SESSION_ARGON2_MEMORY_KIB=65536
OWNER_SESSION_ARGON2_ITERATIONS=3
OWNER_SESSION_ARGON2_PARALLELISM=1
```

## 4. 数据预采集（解"没数据"）

`quant.research@1.0.0` 四个 Stage 依赖真实 PIT 事实，生产环境**没有自动预采集**（涨跌停 Worker 默认关闭，盘口只有 CLI）。以下在**部署机、能连 MariaDB/Redis、加载同一份 .env** 的环境执行。以标的 `600519.SH`、交易日 `2026-11-28` 为例（交易日须为已收盘交易日）：

```bash
cd <源码目录>
# 涨跌停池事实（只落库，不签发授权）；-trade-date 必填且须为已收盘交易日
make limit-pool-collect ARGS="-symbol 600519.SH -trade-date 2026-11-28"
# 真实五档盘口事实（只落库）
make order-book-collect ARGS="-symbol 600519.SH"
```

其余依赖说明：
- **交易状态事实**：由 `Module.CreateSecurityTradingStatusFactsSnapshot` 在 Risk Stage 读取时从 PIT 表构建；PIT 表由 provider 读取时回写。若 PIT 无数据，Risk Stage 会明确拒绝（fail-closed），不会降级成实时抓取。
- **日线收盘序列 / 交易日历**：SMA20/factor/trend Stage 经 `DailyBarsCloseSeriesSnapshot` 读取 PIT 版本表（`market_data_daily_bar_versions`）与日历快照。这些由 Data provider 读取路径回写；若库中尚无 PIT 数据，三个 Stage 会 degrade/fail。
- **provider**：`configs/marketdata/providers.json` 已启用 eastmoney（quote/daily_bars/order_book/limit_pools 均为 true）。确认部署机可访问其数据源。

**关键提醒**："真库 L3 跑通"的种子数据全部位于 `internal/server/bootstrap/quant_research_l3_seed_test.go`（`_test.go`），部署时不会执行。生产库的 PIT 数据需要按上表来源实际具备；若验证发现 Stage 因缺数据 degrade/fail，这是独立的数据补录项，不是代码缺陷。

## 5. 端到端验证（按此顺序，问题会逐层暴露）

```bash
# 1) 服务存活（无需身份）
curl -s http://127.0.0.1:8092/healthz
# 2) Researchflow 路由已注册：期望 401（不是 404）
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8092/api/v1/researchflow/runs
# 3) 浏览器页面登录 Owner Session 后，在页面创建单标的研究任务（前端改造完成后可用）
# 4) 观察 Run 状态到终态；succeeded/partial 后读取 results，确认 results[] 非空
```

判定：
- 第 2 步仍为 404 → 回到第 1、2 节（镜像版本或 env 容量未生效）。
- 路由 401 但页面无数据 → 属前端主流程仍在调旧 API（401），由代码改造组交付。
- 页面能创建 Run 但 results 为空 → 属第 4 节数据未就绪，检查预采集与 PIT 数据。

## 6. 已知边界（非阻塞，但需知悉）

- **结果区是降级渲染**：`/results` 返回 `authorized_domain_results`（Quant 指标/因子/趋势 + risk 状态，risk 无 payload），不是完整研究报告；完整报告唯一 Owner（RF-BLOCK-007，需真实 LLM projector）仍 OPEN，生产 projector 注入 nil。按本轮用户决策，页面明确标注"非完整报告"即可，不阻塞。
- **交易日语义**：契约层未传 `asOfDate` 时取 UTC 当天；周末/节假日跑 Run，risk 的 `trade_date` 会落在非交易日并被 Stage 门禁拒绝。前端改造应传入最近已收盘交易日。
- **数据可重建**：按仓库规则 16，表和数据可手动清空重建；无需保留历史逻辑数据。
