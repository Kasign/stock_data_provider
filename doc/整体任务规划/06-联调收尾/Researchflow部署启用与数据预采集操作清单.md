# Researchflow 部署启用与数据预采集操作清单（逐步执行版）

> 适用环境：部署机 `192.168.3.101` 上的 Root Server 单入口（docker-compose，web:8093 → server:8092）。
> 责任边界：本文所有步骤由**用户在部署机执行**；代码侧旧 Analysis 删除与 WebUI Researchflow 迁移已完成并通过 CI，无需再改代码。
> 生成日期：2026-11-28。命令与期望响应均来自部署态实测 + 源码取证；无法代做的部分已明确标注。
> 关键提醒：当前日期 2026-11-28 是**周六**，预采集与 Run 的交易日必须使用最近已收盘交易日 `2026-11-27`（周五），不要用周末日期。

## 0. 先确认四个地址/路径

| 名称 | 值 | 用途 |
|---|---|---|
| 浏览器访问 | `http://127.0.0.1:8093`（本机 SSH 转发）或 `http://192.168.3.101:8093` | 打开 WebUI、登录 Owner Session |
| Server 直连 | `http://127.0.0.1:8092`（**部署机上**执行 curl） | 验证路由是否注册，不经过浏览器 |
| `<DEPLOY_DIR>` | `docker-compose.yml` 所在目录 | 执行 `docker compose` 与编辑 `.env` |
| `<SRC_DIR>` | 仓库源码目录（含 `Makefile`） | 执行预采集命令 |

后文所有 `<DEPLOY_DIR>` / `<SRC_DIR>` 请替换为实际绝对路径。若不确定，在部署机执行：

```bash
ls -la ~ /opt /srv 2>/dev/null
find / -maxdepth 4 -name docker-compose.yml 2>/dev/null | head
```

## 1. 诊断基线（2026-11-28 实测，用于对照）

| 端点 | 实测响应 | 含义 |
|---|---|---|
| `/healthz` | 200 | nginx + Root Server 存活 |
| `/api/readyz` | `401 {"code":"server.session_unauthenticated"}` | Owner Session 已启用（走 session 分支） |
| `/api/v1/analysis/tasks/probe` | `401 WWW-Authenticate: Bearer` | 旧链已删除；部署态旧镜像仍可能残留此响应 |
| `/api/v1/researchflow/runs` | **404** | Researchflow 路由未注册 → 容量未配置 |

404 的代码根因（`internal/server/bootstrap/bootstrap.go`、`router.go`）：Researchflow 路由只在 `ownerSession.researchflow != nil` 时注册，该字段依赖 `ResearchflowCapacity.Configured == true`，后者要求 `RESEARCHFLOW_MAX_QUEUED_RUNS` 与 `RESEARCHFLOW_MAX_WORKERS` **同时显式配置**（`internal/server/config/researchflow.go`）。仓库 `.env.example` 中这三项默认均为注释状态。

## 2. 确认部署镜像包含新代码

**必须在部署机执行**。若镜像早于本轮提交，即使 `.env` 配好也仍是 404（旧二进制没有 Researchflow 路由）。

```bash
cd <SRC_DIR>
git rev-parse --abbrev-ref HEAD          # 期望 repair_all
git pull origin repair_all
git rev-parse HEAD                      # 期望 b1e8ede82 或更新
```

期望输出示例：

```text
repair_all
b1e8ede82...
```

然后重建并启动 server：

```bash
cd <DEPLOY_DIR>
docker compose build stock-analysis-server
docker compose up -d --force-recreate stock-analysis-server
docker compose ps                        # 确认 server 状态为 Up
```

若部署机没有源码目录、只消费已构建镜像，跳过 `git pull`/`build`，改为拉取包含 `b1e8ede82` 的镜像 tag 后 `docker compose up -d --force-recreate`。

## 3. 配置 `.env`（解 404 的核心步骤）

### 3.1 先生成两个独立凭据

在部署机任意目录执行**两次**，分别记下两个输出：

```bash
# 第一次：平台运维 Bearer Token（启动必需，管理员/probe 用）
openssl rand -hex 32
# 第二次：Quant 服务 Token（Worker 读数据用）
openssl rand -hex 32
```

两个值**不得相同**；`QUANT_SERVICE_TOKEN` 不得复用 `PLATFORM_ADMIN_TOKEN`。每次部署可重新生成。

### 3.2 编辑 `.env`

```bash
cd <DEPLOY_DIR>
cp .env .env.bak.$(date +%Y%m%d)          # 先备份
vi .env
```

在文件中**按 `.env.example` 原样填入**以下整块。注意：`PLATFORM_SERVICE_CREDENTIALS` 与 `PLATFORM_ADMIN_TOKEN` 都是**裸值、不加外层引号**（`deploy-go.sh` 会直接读取这两行）。如已有同名项，覆盖为这里的值：

```dotenv
# 平台运维 Bearer Token；deploy-go.sh 与受保护 readiness 检查必需；不加引号
PLATFORM_ADMIN_TOKEN=<粘贴 3.1 第一次生成的 64 位十六进制值>

# Researchflow 容量：两项必须同时显式配置且为正整数，否则 Server fail-closed
RESEARCHFLOW_MAX_QUEUED_RUNS=100
RESEARCHFLOW_MAX_WORKERS=8
RESEARCHFLOW_RUN_DEADLINE=45m
RESEARCHFLOW_LOCAL_WORKERS=2

# 服务凭据：裸 JSON 数组，不加外层引号；token_env 指向独立变量
PLATFORM_SERVICE_CREDENTIALS=[{"name":"quant","token_env":"QUANT_SERVICE_TOKEN","scopes":["data:read"]}]
QUANT_SERVICE_TOKEN=<粘贴 3.1 第二次生成的 64 位十六进制值>

# Owner Session：OWNER_SESSION_ENABLED 由 docker-compose.yml 硬编码为 true，
# 写进 .env 无效；以下四项由 compose 的 ${...:?} 强制要求，缺一项容器起不来
OWNER_SESSION_PUBLIC_ORIGIN=http://192.168.3.101:8093
OWNER_SESSION_ARGON2_MEMORY_KIB=65536
OWNER_SESSION_ARGON2_ITERATIONS=3
OWNER_SESSION_ARGON2_PARALLELISM=1
```

配置约束：

- `SERVER_ROLE` 未设置时默认 `all`（同进程启动 Worker）；若拆成 api/worker 两个实例，`api` 角色**不启动** Worker。
- `OWNER_SESSION_PUBLIC_ORIGIN` 必须与浏览器地址栏的协议+主机+端口完全一致；若你通过 `http://127.0.0.1:8093` 访问，就改成那个值，否则 CSRF/Origin 校验会拒绝。
- 若配置了 `NEWS_FEED_SOURCES_JSON`，也必须同时配置 Researchflow 容量，否则启动报错。

**HTTP 访问必须额外改 Cookie 配置**（当前部署通过 SSH 转发 `http://127.0.0.1:8093` 或 `http://192.168.3.101:8093`，不是 HTTPS）：`docker-compose.yml` 默认 `OWNER_SESSION_COOKIE_SECURE=true`、Cookie 名 `__Secure-owner_session`，浏览器在 HTTP 下会拒绝该 Cookie。在 `.env` 中追加：

```dotenv
OWNER_SESSION_COOKIE_SECURE=false
OWNER_SESSION_COOKIE_NAME=owner_session
```

若你通过 HTTPS 反向代理访问（浏览器地址栏是 `https://`），**不要**改这两项，保持默认。

### 3.3 重启并确认配置生效

```bash
cd <DEPLOY_DIR>
docker compose up -d --force-recreate stock-analysis-server
docker compose config | grep -E 'RESEARCHFLOW_|PLATFORM_SERVICE_CREDENTIALS|QUANT_SERVICE_TOKEN' | head -20
docker compose logs --tail=80 stock-analysis-server
```

`docker compose config` 输出中应能看到上面配置的值（token 会显示实际值，注意不要贴到公开位置）。启动日志中若有 `fail-closed` / `invalid config` 类错误，回到 3.2 检查字段名和值。

## 4. 验证 Researchflow 路由已注册

在**部署机**执行（不要在你的本地电脑）：

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8092/api/v1/researchflow/runs
```

判定：

- 返回 `401` → **成功**。路由已注册，只差浏览器身份，继续第 5 步。
- 返回 `404` → 回到第 2、3 步：确认镜像含新代码、`.env` 容量两项都配置且重启生效。
- 返回 `000` / 连接拒绝 → 确认 server 容器 Up、端口 8092 映射正确。

注意：当前契约**没有列表路由**（`GET /api/v1/researchflow/runs`），只有 `POST /api/v1/researchflow/runs`（创建）、`GET /api/v1/researchflow/runs/{run_ref}`（查询）、`POST /api/v1/researchflow/runs/{run_ref}/cancel`（取消）、`GET /api/v1/researchflow/runs/{run_ref}/results`（结果）。所以：

- 未认证 GET 返回 `401` 是**预期**：认证中间件先拦截，说明路由已注册。
- 带有效 Cookie GET 返回 `404`/`405` 也是**正常**：内部 mux 没有列表端点，不代表 Researchflow 未启用。登录态请改用第 5 步的 `GET /api/v1/owner/session` 判断。

## 5. 浏览器登录 Owner Session（首次需设置密码）

Owner Session **没有默认用户名/密码**，身份固定为 `owner`。首次部署流程：

1. 浏览器打开 `http://127.0.0.1:8093`（必须与你配置的 `OWNER_SESSION_PUBLIC_ORIGIN` 完全一致）。
2. 进入页面「设置 → 安全与备份」，首次访问会引导设置 owner 密码；对应端点 `POST /api/v1/owner/setup`，服务端用 Argon2id 存储（参数即第 3.2 节三项）。
3. 用刚设置的密码登录，对应端点 `POST /api/v1/owner/session`。成功后写入 HttpOnly Cookie（默认名 `__Secure-owner_session`，Path `/api`）；CSRF token 只驻留 JS 内存，写请求带 `X-CSRF-Token`。
4. 验证登录态：

```bash
# 部署机执行，把 Cookie 名/值换成浏览器实际值（HTTP 环境按第 3.2 节改名后为 owner_session）
curl -s -o /dev/null -w '%{http_code}\n' \
  -H 'Cookie: owner_session=<浏览器中的 Cookie 值>' \
  http://127.0.0.1:8092/api/v1/owner/session
```

返回 `200` 表示 Cookie 有效。若登录后仍 401，按此顺序排查：

- `OWNER_SESSION_PUBLIC_ORIGIN` 与浏览器地址栏是否**完全一致**（含协议、端口）。
- **HTTP 访问必须改 Cookie 配置**：见第 3.2 节末尾的 `OWNER_SESSION_COOKIE_SECURE` / `OWNER_SESSION_COOKIE_NAME`；默认的 `__Secure-owner_session` + Secure=true 在 HTTP 下会被浏览器拒绝，表现为“登录成功但一刷新就掉线”。
- 若设了 `OWNER_SESSION_RESET_REQUIRED=true`，会强制重置密码，按页面提示重设。

## 6. 预采集真实 PIT 事实数据（解“没数据”）

`quant.research@1.0.0` 四个 Stage（sma20 / factor / trend / risk）依赖真实 PIT 事实，生产环境**没有自动预采集**（涨跌停 Worker 默认关闭，盘口只有 CLI）。以下在**部署机、能连 MariaDB、加载同一份 `.env`** 的环境执行。

前置条件（缺一不可）：

- 第 2 步的 server 已启动并完成 Migration（采集器直接写 MariaDB，表不存在会报错）。
- `.env` 中 `DB_DSN` / `DB_NAME` 指向实际库（默认库名 `stock_analysis_server`）。
- **不需要 Redis，也不需要 Provider API Key**；采集器只用 `DB_DSN` + eastmoney provider 默认配置。

以标的 `600519.SH`、交易日 `2026-11-27`（最近已收盘交易日）为例：

```bash
cd <SRC_DIR>
# 涨跌停池事实（只落库，不签发授权）；-trade-date 必填且须为已收盘交易日
make limit-pool-collect ARGS="-symbol 600519.SH -trade-date 2026-11-27"
# 真实五档盘口事实（只落库）
make order-book-collect ARGS="-symbol 600519.SH"
```

其余依赖说明：

- **交易状态事实**：由 Risk Stage 读取时从 PIT 表构建；PIT 无数据时 Risk Stage 明确拒绝（fail-closed），不会降级成实时抓取。
- **日线收盘序列 / 交易日历**：SMA20/factor/trend Stage 经 `DailyBarsCloseSeriesSnapshot` 读取 PIT 版本表（`market_data_daily_bar_versions`）与日历快照，由 Data provider 读取路径回写。库中尚无 PIT 数据时三个 Stage 会 degrade/fail。
- **provider**：`configs/marketdata/providers.json` 已启用 eastmoney（quote/daily_bars/order_book/limit_pools 均为 true），确认部署机可访问其数据源。

**关键提醒**：L3 测试里的种子数据全部位于 `internal/server/bootstrap/quant_research_l3_seed_test.go`（`_test.go`），部署时不会执行。若 Stage 因缺数据 degrade/fail，这是独立的数据补录项，不是代码缺陷。

## 7. 端到端验收（按顺序，问题会逐层暴露）

1. **服务存活**：`curl -s http://127.0.0.1:8092/healthz` → 200。
2. **路由注册**：`curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8092/api/v1/researchflow/runs` → 401。
3. **浏览器登录**：打开 WebUI，确认页面无报错、无旧 Analysis 请求（浏览器 DevTools Network 中不应出现 `/api/v1/analysis/`）。
4. **创建 Run**：输入标的 `600519.SH`，`asOfDate` 选 `2026-11-27`（最近已收盘交易日），提交后观察 Run 状态到终态。
5. **读取结果**：状态为 `succeeded` / `partial` 后，结果区应显示授权领域结果，并明确标注“非完整研究报告”；`results[]` 不应为空。
6. **取消路径**：另建一个 Run，在排队/运行中取消，确认状态变为 cancelled。

判定：

- 第 2 步仍 404 → 回第 2、3 步（镜像版本或 `.env` 容量未生效）。
- 路由 401 但页面无数据 → 确认 WebUI 已部署含 Researchflow 迁移的代码（`b1e8ede82` 或更新）。
- 能创建 Run 但 `results[]` 为空 → 回第 6 步检查预采集与 PIT 数据。
- 周末/节假日跑 Run 被门禁拒绝 → 检查 `asOfDate` 是否为已收盘交易日。

## 8. 停服窗口清理旧数据结构（可选，需停机）

旧 Analysis 代码已删除，以下表/记录只保留历史数据，可按仓库规则 16 在停服窗口清理重建。清理前**先备份数据库**：

```bash
# 在部署机、能连 MariaDB 的环境执行；替换库名与凭据
mysqldump -u <user> -p <db> > analysis_backup_$(date +%Y%m%d).sql
```

然后在 MariaDB 的 `stock_analysis_server`（或你的 `DB_NAME`）库中执行：

```sql
DROP TABLE IF EXISTS analysis_runs;
DROP TABLE IF EXISTS analysis_stage_events;
DROP TABLE IF EXISTS analysis_snapshots;
DROP TABLE IF EXISTS analysis_tasks;
DELETE FROM platform_schema_migrations WHERE namespace = 'analysis';
```

清理后重启 server，确认：

- `curl -s http://127.0.0.1:8092/healthz` → `200`
- `curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8092/api/v1/researchflow/runs` → `401`（未认证即说明路由已注册）
- 登录态用 `GET /api/v1/owner/session` 返回 `200` 判断；**不要**用 `researchflow/runs` 的 200 判断，该路径没有列表端点。

## 9. 已知边界（非阻塞，但需知悉）

- **结果区是降级渲染**：`/results` 返回 `authorized_domain_results`（Quant 指标/因子/趋势 + risk 状态，risk 无 payload），不是完整研究报告。完整报告唯一 Owner（RF-BLOCK-007，需真实 LLM projector）仍 OPEN，生产 projector 注入 nil。按本轮用户决策，页面明确标注“非完整研究报告”即可。
- **交易日语义**：契约层未传 `asOfDate` 时取 UTC 当天；周末/节假日跑 Run，risk 的 `trade_date` 会落在非交易日并被 Stage 门禁拒绝。页面应传入最近已收盘交易日。
- **数据可重建**：按仓库规则 16，表和数据可手动清空重建，无需保留历史逻辑数据。
