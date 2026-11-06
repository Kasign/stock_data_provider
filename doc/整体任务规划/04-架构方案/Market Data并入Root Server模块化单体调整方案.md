# Market Data 并入 Root Server 的模块化单体调整方案

- 维护分支：`feat/design_doc`
- 设计基线：`7344170dc53124361c9cf49efb0d66821d3605c1`
- 决策状态：`accepted`（采用同进程模块化单体方向）
- 实施状态：`not_started`（截至 2026-11-06 Data 分支已合入架构决议，但代码仍位于独立 Module；M0 仅完成初步只读审计）
- 范围：Root Server、Market Data、WebUI 部署接入、Platform 基础设施装配
- 非目标：结构迁移阶段不改变行情业务语义，不删除既有 Data API DTO/Schema，不拆分 Provider 子模块；DATA-CTR-005～008 的正式契约实现集中放在结构迁移后的 MDM-008

> 本方案落实项目总规划中“Server 是唯一组合根、业务模块优先同进程公开 Port、确有部署隔离需求再拆服务”的原则。合并的是运行时和 Go Module，不是 Data 的领域 Owner、Repository 或业务规则。

---

## 1. 决策摘要

采用方案 B：**Market Data 与 Root Server 合并为同一个 Go Module、同一个进程、同一个部署单元；Data 保留独立领域模块和可替换的 HTTP Adapter。**

目标运行结构：

```text
Browser / WebUI
      |
      | same-origin HTTPS
      v
Root Server :8092
  |-- Server Bootstrap / Router / Security / Request Context
  |-- Analysis Application
  |-- Market Data Module
  |    |-- Provider Adapters
  |    |-- Data Application / Use Cases
  |    |-- Domain Model / Normalization / Quality / PIT
  |    |-- Repository / Cache Adapter
  |    |-- HTTP Adapter (兼容与外部 API 边界)
  |-- Research / News / Quant / Researchflow / LLM / Notification
  |
  +-- shared Platform infrastructure
       |-- MariaDB connection pool
       |-- Redis connection pool (按模块可选)
       |-- controlled outbound HTTP client
       |-- config / secret / diagnostics / request context
```

部署目标：

```text
docker-compose
  |-- stock-analysis-server  (Root Server + Market Data)
  |-- stock-analysis-web
  +-- external MariaDB / Redis / 1panel-network
```

不再默认部署 `stock-analysis-data` 容器。保留独立 Data HTTP Adapter 的能力，作为兼容接口、诊断入口或未来拆分时的传输层，不再作为 Server 内部调用的必经路径。

### 1.1 决策理由

1. 当前部署是单机 Compose，Server 与 Data 没有独立扩缩容或多租户隔离需求。
2. Server 已是唯一组合根；News 等业务模块已按同进程 Application Port 规划。
3. Server 内部调用 Data 目前经 HTTP、Data API Key、Base URL 和 JSON DTO 往返，增加不必要的网络、配置和认证维护成本。
4. Data 的 Provider、质量、PIT、Repository 和采集生命周期仍有清晰领域边界，适合保留为模块而不是独立服务。
5. 同一进程可以共享基础设施资源并统一生命周期，未来拆分时只需替换模块 Adapter，不必重写领域逻辑。

### 1.2 不选择其他方案

| 方案 | 结论 | 原因 |
|---|---|---|
| Data 继续独立部署 | 暂不采用 | 对当前单机运行没有足够收益，持续承担内部 HTTP、Secret、健康检查和部署编排成本 |
| 一个容器两个进程 | 不采用 | 仍保留 HTTP、认证和双进程监管，复杂度没有实质下降 |
| Data 代码直接揉入 Server Application | 禁止 | 会破坏领域 Owner、依赖方向和未来拆分能力 |
| 同一进程模块化单体 | 采用 | 统一部署与生命周期，同时保留模块边界和未来拆分能力 |

---

## 2. 当前代码事实与审计发现

以下结论基于设计分支当前可读取的文件，不等于已完成所有依赖的静态分析或运行时测试。

| 位置 | 当前事实 | 对合并的影响 |
|---|---|---|
| 根 `go.mod` | Go 1.22；模块路径 `example.com/stock-analysis-flow` | 需要统一模块路径和 Go 版本 |
| `services/market-data/go.mod` | 独立 Module，Go 1.23.0；包含 TDX、x/text 等依赖 | 不能直接 import Data 的 `internal` 包；合并后需要处理依赖版本 |
| `cmd/analysis-api/main.go` | Bootstrap 创建 MySQL/Redis Store、Market Data HTTP Client、Runner、Task Queue 和 HTTP API | 是当前 Root Server 进程入口；应演进为唯一组合根 |
| `internal/adapters/marketdata/client.go` | Root Server 通过标准库 `net/http`、Base URL、JSON DTO 和 `X-API-Key` 调用 Data；没有使用现有 `internal/platform/httpclient` | 迁移后替换为 Data Application Port；不能将其与 Provider 出站 HTTP 混为一谈 |
| `services/market-data/cmd/market-data/main.go` | 集中加载 Data Config、连接 MariaDB/Redis、构造 Provider 能力集合、DataService、HTTP Handler 并监听端口 | Data 的构造装配逻辑应提取为可复用 Module Bootstrap，不再由独立 main 独占 |
| `services/market-data/internal/httpapi/server.go` | HTTP Server 持有 API Key 校验、Request ID、超时、访问日志、安全 Header 与多条 API 路由 | Handler 与业务 Application 需要解耦；不能直接把原 Handler 当作不受保护的内部 Port |
| `services/market-data/internal/repository/mysql.go` | Data 自有 MariaDB Repository、表结构和迁移逻辑 | Data 继续拥有 Repository 和 Migration；连接池由组合根统一提供 |
| `services/market-data/internal/cache` | Data 自有 Cache 接口及 Redis Adapter | Cache 仍归 Data Adapter；Redis 连接生命周期应由 Server 统一装配或由 Data Adapter 明确 Close |
| `docker-compose.yml` | 独立 Data、Server、Web 三个服务；Server depends_on Data healthy | 移除 Data 服务后调整健康依赖和配置 |
| `web/default.conf.template` | `/market-api/` 代理 Data:8091，`/analysis-api/` 代理 Server:8092 | 两个 API 前缀改为同一 Root Server 上游，保留 URL 兼容 |
| `doc/整体任务规划/跨模块契约目录与冻结状态.md` | Server 是唯一组合根；News 已明确采用根 Module 同进程 Application Port | Data 应统一遵循该部署原则；逻辑依赖 DAG 不因同进程而放宽 |

### 2.1 M0 初步审计结果（只读核对）

本轮继续核对了 Root Server、Data、WebUI 与旧迁移部署文件，得到以下已确认事实：

| 检查项 | 代码证据 | 结论 |
|---|---|---|
| Root Server 数据库资源 | `internal/adapters/storage/mysqlredis/store.go` 的 `Open` 创建并 Ping `*sql.DB`，Store 暴露 `DB`；`DBAutoMigrate=true` 时在 Open 内部执行 Analysis Migration，随后执行 CheckSchema | Data Repository 可接收同一个 `*sql.DB`，但必须明确借用资源；Root Migration 也要从 Open 副作用中拆出，由 Server Bootstrap 显式编排，不能在连接创建时隐式执行 |
| Data 数据库资源 | `services/market-data/internal/repository/mysql.go` 的 `OpenMySQL` 自行 `sql.Open`，连接池默认 MaxOpen=10/MaxIdle=3/ConnMaxLifetime=5m；Data main 根据 `MARKET_DATA_DB_AUTO_MIGRATE` 执行 Data Migration | 当前确实存在两套 DB Pool 和两个 Migration 入口；合并时要保留 Data Migration 内容，但由 Server 统一执行 |
| Root/Data 有效 DSN 对照 | `.env.example` 两侧 DB Host/Port/Name/User、字符集和 UTC 时区配置意图相同，但 Root/Data 使用独立 Password 环境变量；Redis 地址/DB 示例相同，密码变量也独立。Data Config 还支持独立 Redis Username/TLS 等连接参数 | 示例文件不能证明生产凭据相同。部署前核对有效 DSN/凭据；只有完全一致才共享同一 DB Pool/Redis Client，否则由 Root Bootstrap 拥有多个资源句柄并分别注入 borrowed handle/client |
| Root Redis | Root Store 使用 `github.com/redis/go-redis/v9` 创建有连接池的 Client，并由 Store.Close 关闭 | 可作为统一 Redis 资源的基础 |
| Data Redis | `services/market-data/internal/cache/redis.go` 是自定义 RESP Client，每次命令重新 Dial、AUTH/SELECT、执行命令后关闭连接 | 不应原样保留为同进程默认实现；建议提供基于共享 go-redis Client 的 Data Cache Adapter，按 Data key namespace/TTL 规则隔离 |
| Data 配置 | `services/market-data/internal/config/config.go` 当前无条件校验 `MARKET_DATA_API_KEY`；Redis 地址可为空，允许禁用缓存；MySQL 可按 AllowNoStorage 配置 | 同进程模式必须将 Data API Key 从必填配置中移除；远程 Adapter 模式才要求 Key。Redis 可选语义不能被 Root Server 的 Redis 强依赖无意覆盖 |
| Data HTTP Handler | `services/market-data/internal/httpapi/server.go` 的 Handler 内部包含 API Key 校验和独立 Request ID/超时/访问日志中间件 | 不能原样挂载后再绕过认证；应拆分路由 Handler 与传输中间件，由 Root Server 统一注入认证/Request Context |
| WebUI 凭据 | `web/src/api.js` 将 `apiKey` 纳入设置对象，读取/写入 localStorage，并在请求时写入 `X-API-Key` | 浏览器暴露问题已由当前代码直接证实，M5 必须移除；迁移后需清理旧设置字段并轮换曾暴露的有效 Key |
| Nginx | `web/default.conf.template` 的 `/market-api/` 当前代理到 Data 服务，`/analysis-api/` 代理 Server | 两个前缀可保留，但上游统一改为 Root Server |
| go-core | `docker/docker-compose.go-migration.yml` 将 Python Bridge、go-core、Data 组成单独的 `go-migration` profile，并明确不与根 Compose 叠加；根 Compose 当前运行的是根模块 `cmd/analysis-api` | go-core 是仓库保留的旧迁移部署路径，不是根 Compose 的当前组合入口；本次不把 go-core 合并进 Root Server。仍需部署现场确认没有继续使用旧 profile |

**M0 扩展分支复核（2026-11-06）：** 本轮只读审计以 Data 分支 `feat/full-go-data-providers-api-switch` HEAD `d34eb937eaae713220456c8e8ba7c62f1227b7c8` 为准。根 `go.mod` 仍是 `example.com/stock-analysis-flow` / Go 1.22；Data `go.mod` 仍是独立 Module / Go 1.23.0；根 Dockerfile Builder 为 Go 1.22，Data Dockerfile Builder 为 Go 1.23，CI Go setup 为 1.23.x。Data 归并尚未开始。Data 分支从代码基线 `4ca1da2` 到当前 HEAD `d34eb93` 的比较仅显示迁移跟踪文档有变化，没有生产 Go/配置/部署代码变化；因此本段代码证据仍对应 `4ca1da2` 的实现状态，当前 HEAD 主要推进了审计记录。

本轮新增确认：

| 审计项 | 当前代码事实 | 迁移影响 / 必须保留的行为 |
|---|---|---|
| Module 依赖 | 根模块使用 `github.com/go-sql-driver/mysql v1.9.3`；Data 模块使用 v1.8.1，并引入 `github.com/injoyai/tdx`、`golang.org/x/text` 等依赖 | 合并时统一 Go 版本与依赖图，不能简单复制 Data go.mod；按根模块 MVS 结果复核依赖兼容性 |
| 仓库内调用方 | Root 的 `internal/adapters/marketdata/client.go` 和旧 `go-core/internal/adapters/marketdatahttp/client.go` 均使用远程 HTTP + Data API Key；WebUI `web/src/api.js` 仍保存 apiKey 到 localStorage 并写 `X-API-Key` | Root 调用改为 Application Port；go-core 属旧 `go-migration` Profile，不并入 Root，需确认是否保留；WebUI Key 必须删除 |
| Provider 出站网络 | Eastmoney、Hithink、Tencent、Sina、Yahoo、Netease、Tushare、TickFlow、Xueqiu、MiaoXiang 使用各自 `net/http` Client；BaoStock 与 PyTDX 使用原生 TCP/第三方 TDX Dial | Platform HTTP Client 只适用于 HTTP Provider，不适用于 BaoStock/PyTDX TCP。逐 Provider 登记 Host、Redirect、Cookie/Header、Timeout、Retry、响应上限；禁止机械替换 |
| 出站安全边界 | Data `validateEndpointURL` 主要校验 URL 结构和 http(s) scheme；Provider 仍直接持有 `http.Client`。根 `internal/platform/httpclient` 已提供目的地址策略、精确 Host/Port、DNS 与响应体约束，但 Data Provider 尚未使用 | Provider HTTP Client 接入 Platform 是需单独评估的技术子项；先完成 Host/Redirect 策略映射。TCP Provider 另行评估目标地址约束 |
| Data Config 资源 | `config.Load` 默认从进程工作目录读取 `configs/providers.json`；当前 Data Dockerfile 将其复制到 `/app/configs` | 并入根镜像后原文件不会自动出现在根工作目录；应采用 `go:embed` 或 Bootstrap 显式注入路径 |
| Data HTTP 路由 | Handler 注册 33 条受 API Key 保护的业务路由；`/health/live`、`/health/ready` 不走该 Key；外层含 Security Header、Request ID、Access Log、Timeout Context | 保留全部 v1 路由语义；拆出业务 Handler，由 Root 统一外部认证/授权/Request ID/错误映射 |
| DB Migration | Go `Store.Migrate` 与 `migrations/001_create_market_data.sql` 的 CREATE TABLE 名称集合已核对为 9/9 一致；Go 运行时还会把日线版本表、财务版本表的历史唯一索引改为非唯一索引，并补齐 Quote 的 `volume_ratio`/`turnover_rate` 两列 | M1/M4 必须保留 Go 运行时升级逻辑；当前只完成表名集合对照，字段定义、索引列顺序/唯一性、默认值、字符集/排序规则的完整 DDL diff 仍未完成，不能以 9/9 表名匹配宣称 Schema 完全一致 |
| Worker / 资源关闭 | Data main 启动 Quote Retention Goroutine（启动立即执行，之后每 24 小时执行）；关闭 BaoStock Client 和自有 Store；Worker 当前没有显式 Wait/Drain 句柄 | Module 必须提供受管理的 Start/Stop/Wait；共享 DB/Redis 不得由 Data Close；先停止 Worker，再关闭共享资源 |
| DB / Redis | Data Repository 自建 `*sql.DB`（MaxOpen=10/MaxIdle=3/ConnMaxLifetime=5m）；Redis 每条命令新建 TCP 连接并 AUTH/SELECT；Root Store 持有 DB 与 go-redis Client | Data Repository 增加借用 DB 构造；Cache 接入共享 go-redis Client；明确 borrowed/owned 生命周期并保留缓存禁用/降级语义 |
| Compose / Nginx / 部署 | 根 Compose 当前运行 Data、Server、Web 三容器；Nginx 两个 API 前缀分别代理 Data:8091 与 Server:8092；部署脚本固定构建、启动、健康检查三个容器 | 默认栈迁移为双容器；两个 API 前缀统一到 Root Server；部署脚本、健康检查和 .env 同步调整 |
| 旧迁移 Profile | `docker/docker-compose.go-migration.yml` 仍构建 `../services/market-data/Dockerfile`；go-core 仍通过远程 Data HTTP Client 调用它 | 删除旧目录/Dockerfile 会使 Profile 失效。MDM-001 必须确认现场使用情况；需保留回滚则 M1 保留兼容入口和可构建镜像，确认退役才可删除 |
| CI | `go-test.yml` matrix 包含根、go-core、services/market-data 三个 Module；Data MariaDB 集成测试、Docker Build、Parity 和 Live Probe Workflow 均硬编码旧路径 | M1 后 Data 测试迁到根 Module；更新 matrix、cache path、working-directory、触发路径、Docker Build；go-core 保持独立 Module，直至旧 Profile 退役 |

**Provider 默认 Host / Redirect 策略补充核对：**

| Provider | 当前默认目标 Host | 当前 Redirect 行为 |
|---|---|---|
| Eastmoney | `push2.eastmoney.com`、`push2his.eastmoney.com`、`datacenter.eastmoney.com`、`push2ex.eastmoney.com`、`datacenter-web.eastmoney.com`、`np-anotice-stock.eastmoney.com`、`emappdata.eastmoney.com` | 自定义策略：仅允许 HTTPS 下东方财富域名内跳转，最多 3 次 |
| Hithink | `fuyao.aicubes.cn` | Go 默认 Redirect 策略 |
| Tencent | `web.sqt.gtimg.cn`、`web.ifzq.gtimg.cn`、`smartbox.gtimg.cn` | 禁止 Redirect |
| Sina | `hq.sinajs.cn`、`money.finance.sina.com.cn` | 禁止 Redirect |
| Yahoo | `query1.finance.yahoo.com` | 禁止 Redirect |
| Netease | `quotes.money.163.com`、`api.money.126.net` | 禁止 Redirect |
| Tushare | `api.tushare.pro` | Go 默认 Redirect 策略 |
| TickFlow | `api.tickflow.org`、`free-api.tickflow.org` | Go 默认 Redirect 策略 |
| Xueqiu | `stock.xueqiu.com` | Go 默认 Redirect 策略 |
| MiaoXiang | `mkapi2.dfcfs.com` | 禁止 Redirect |
| BaoStock | `public-api.baostock.com:10030` | 原生 TCP，不适用 HTTP Redirect |
| PyTDX | 由 `MARKET_DATA_PYTDX_HOSTS` 配置 | 原生 TCP/第三方 TDX Dial，不适用 HTTP Redirect |

上述 Host 是代码默认值，不是运行时最终 Host 的完整保证：Data Config 允许通过 `MARKET_DATA_*_URL` 覆盖多个 Provider Endpoint，PyTDX Host 也由环境变量注入。当前多数 Provider 的 `http.Client` 未显式设置 Transport，会沿用 Go 默认 Transport 的环境代理行为；Platform HTTP Client 当前显式 `Proxy: nil`。接入 Platform 前必须决定代理策略、精确 Host Allowlist、Redirect 跨域限制和各 Provider 响应体预算，不能仅把 Client 替换掉而忽略网络行为差异。

**Data 分支迁移跟踪文档补充证据（当前 Data HEAD：`d34eb937eaae713220456c8e8ba7c62f1227b7c8`）：** Data 分支新增 `doc/api/migration/MarketData模块化单体迁移计划与进度.md`，其 M0 仍为 `in_progress`，与本方案采用同一架构决策。该文档补充了以下迁移准入发现：

1. Data OpenAPI 与 HTTP Handler 的 Method+Path 双向核对为 35/35 匹配；这只证明路由路径/方法完整，不代表全部 Request/Response Schema、required/nullability、错误响应和鉴权 Header 已完成字段级核验。
2. Data HTTP API 覆盖 35 个 Operation；DataService 还聚合证券主数据、搜索、市场统计、交易日历/状态、财务、估值、公告、筹码、资金流、诊断等能力。Application Port 应按领域拆分用例接口；Analysis InProcess Adapter 只实现 Analysis 所需的 Quote/DailyBars/IndexBars，不应暴露一个包含所有 Provider/Repository 的巨大接口；HTTP Adapter 仍需覆盖完整 Data 能力面。
3. Root Remote Adapter 存在 DTO 信息损失：Quote 的 nullable `current_price` 被映射为非 nullable float64，Data 的 int64 volume 被映射为 float64；DailyBar 的均线、复权因子来源/版本/观察时间、标准化时间、原始单位/字段、fallback、Request ID 等字段没有完整进入 Root DTO。
4. Data DailyBars API 支持 `as_of`，可按系统观察时点重建历史修订版本；当前 Root `GetHistory` 只传 `limit` 和 `adjustment`，Analysis Collector 仅过滤未来交易日期，没有传递 PIT cutoff。**过滤交易日期不等于按 as_of 重建版本**。M2/M3 必须把明确的 PIT 查询语义纳入 Application Port 和 Analysis Adapter；若 Analysis RunRequest.AsOf 与 Data revision cutoff 不是同一语义，必须定义映射，不得默默混用。
5. Data 迁移跟踪文档已完成 Provider 请求模式矩阵初稿，但逐调用点 Query/Body/Header/Retry/Redirect/响应大小限制及跨域敏感信息转发测试仍未全部完成。
6. Data 分支 M0 第十四至十六轮确认 OpenAPI 与实际 HTTP 行为有多项契约描述错误：35 个 Operation 中 Handler 有 33 条业务路由执行 API Key 认证、2 条 health 路由公开；OpenAPI 仅 11 条业务 Operation 显式声明 ApiKey，另 22 条漏声明；Diagnostics 缺少 401。组件虽定义 `Error` Schema，但 35 个 Operation 的错误响应均未引用它；Server 实际输出 `error.class/code/message/retryable/request_id` 结构。现有鉴权 Contract Test 只检查 7 条路由，不能发现 22 条漏声明。公共 `writeDomainErr` 可产生 408/502/503/504，部分 Provider Operation 未声明实际可达的 504；参数默认值、边界、动态上限也尚未全部与 Handler 核对。该问题是现有 v1 HTTP 契约描述错误，不是实际 Handler 放开了认证。应在 MDM-001 阶段修正 security/401/Error Schema/实际可达错误状态码，并扩展逐 Operation Contract Test，作为 MDM-002 的前置门禁；这不等于提前实施 MDM-008 的新 Principal/Scope 契约。

**MDM-001 当前结论：** 仓库内主要代码路径、HTTP 调用方、Provider 传输类型、资源入口、Worker、路由、Compose、部署脚本和主要 CI Workflow 已完成静态盘点；Data 分支已完成 35 个 Operation 的路径/方法、参数名称、Body 存在性和状态码清单初稿，并识别 DTO/PIT 风险。当前明确阻断包括：22 条受保护业务 Operation 漏声明 ApiKey、Diagnostics 缺 401、Error Schema 未被任何 Operation 错误响应引用、现有 Contract Test 只检查 7 条路由，以及部分 Provider Handler 可达的 408/504 未被 OpenAPI 声明。必须在 MDM-001 修正并通过完整逐 Operation Contract Test。部署现场是否仍使用 `go-migration` Profile、Provider 可配置 Host 的最终 Allowlist、SQL 文件与 Go Migration 的完整字段/索引 DDL 差异、参数默认值/边界及成功响应 DTO Schema 对照、PIT 时间语义映射仍需确认；SQL/Go Migration 的表名集合已确认 9/9 一致。因此 MDM-001 暂不关闭，MDM-002 不得提前开始删除旧入口/构建文件。

**结构迁移编号映射：** 与 Data 分支迁移跟踪文档统一采用 M0～M6 七个结构迁移阶段：MDM-001=M0 审计、MDM-002=M1 Module 归并、MDM-003=M2 Application Port/Bootstrap、MDM-004=M3 Root Server 装配、MDM-005=M4 共享资源与生命周期统一、MDM-006=M5 部署/WebUI 切换、MDM-007=M6 结构回归与稳定性验收。迁移验收后另执行 MDM-008 公共平台/数据契约接入；不得把 MDM-008 混入结构迁移验收。

**M0 已确认的主要改造点：**

1. Data Repository 增加接收共享 `*sql.DB` 的构造方式，并区分借用/自有资源；独立远程兼容入口可继续自有连接池。
2. Data Migration 从 Data main 的自动执行路径迁到 Server Bootstrap 的显式 Migration Registry；Analysis 与 Data 使用不同 namespace，顺序执行且只执行一次。
3. Data Cache 改为适配 Root Redis Client；如果 Data 与 Root 配置的 Redis DB 不同，由 Server 创建并拥有独立 Client，不能强行共用错误的 DB index。
4. Data Config 拆分“领域配置”和“远程 HTTP Adapter 认证配置”；进程内模式不要求 Data API Key。
5. Data HTTP Handler 拆分 Application 路由与 transport middleware，Root Server 负责入口认证、可信 Context、Request ID 和统一错误映射。
6. 旧 `go-migration` profile 暂时保留为历史回滚/迁移材料，待确认不再使用后另行删除，不把它混入本次 Data 同进程改造。

### 2.2 当前最重要的技术风险

- **Go Module 版本：** 根模块 Go 1.22，Data 模块 Go 1.23.0。建议统一到 Go 1.23.x（以 Data 依赖与 CI 实际兼容结果为准），并同步 Docker Builder、CI、开发环境说明；不要为了维持旧版本而降级 Data 依赖。
- **构造函数耦合：** Data 目前的大量 Provider capability 装配集中在独立 main 中。必须抽成可测试的 `marketdata.Module` 或等价 Bootstrap API，返回 Application Port、HTTP Handler、Diagnostics 和 Close 生命周期句柄。
- **内部包边界：** 合并后 Data 代码迁入根 Module 的 `internal/marketdata`，根 Module 内其他模块可通过明确的公开 Port/Facade 调用；禁止消费者依赖 Data 的 provider/repository/model 私有实现。
- **共享资源：** 根 Server 与 Data 当前各自打开数据库连接，并可能各自执行 Migration。合并后不能简单让两个 Store 各开一套连接池和重复执行迁移；应由 Server 提供共享连接池，Data 只接收连接句柄并执行自己命名空间下的迁移。
- **HTTP API 安全：** Data 原 HTTP Handler 通过 X-API-Key 保护业务接口。浏览器不得继续持有 Data API Key；同进程不等于可以绕过 Server 用户/Owner 认证。外部 HTTP 路由必须由 Root Server 统一认证、授权、限流和 Request ID 管理，再调用 Data Application Port。
- **数据库兼容：** Data 当前表结构、索引、版本表、历史 PIT 数据和 MariaDB Migration 必须原样保留；本次不做表重命名、数据搬迁或历史数据清洗。

---

## 3. 目标模块边界

### 3.1 Market Data 模块拥有的事实与能力

Data 是以下事实的唯一 Owner：

- Provider 配置、Provider 能力、Provider 调用与来源状态。
- 行情、日线、指数、证券基础信息、财务、估值、日历、行业、资金流等 Data 领域数据。
- 数据标准化、单位/精度、复权口径、质量校验、覆盖率和降级语义。
- Snapshot、来源版本、系统观察时间、PIT 查询和数据质量诊断。
- Data Repository、Data 表结构与 Migration 内容。
- Data 采集/保留 Worker 的业务规则与执行函数。

Server 不得直接操作 Data 表、SQL、Provider 或 Data Repository。

### 3.2 Root Server 拥有的职责

Root Server 是系统唯一的应用组合根和统一 HTTP 服务宿主，职责覆盖整个进程内已注册模块，而不只服务于 Market Data：

- 唯一进程入口、配置聚合、依赖注入、模块注册、进程信号和优雅关闭。
- 统一 HTTP Server/Router、API 版本前缀、路由注册与外部 API 暴露范围管理。
- 统一接入 Platform 的认证授权、限流、审计、Request ID、超时/取消、访问日志和 Fault/HTTP 状态映射。
- 注册各模块提供的 HTTP Handler/Adapter，并汇总实际开放接口的 OpenAPI 文档；不要求每个模块都必须暴露 HTTP API。
- 共享 MariaDB/Redis/HTTP Client 等基础设施的创建与生命周期；模块通过构造参数或公开 Port 获得所需依赖。
- 跨模块公开 Application Port 注入、后台 Worker 启停顺序、Migration Runner 编排和资源关闭。
- 对外 API 的协议适配与入口治理；业务用例和领域规则仍由各模块拥有。
- 不拥有行情领域对象、Provider 策略、数据质量规则或 Data SQL，也不接管 News、Quant、Research、Researchflow、LLM、Notification 的业务逻辑。

**边界说明：** Root Server 不是内部微服务反向代理，也不是业务 Facade 的替代品。模块间同进程调用直接走强类型 Application Port；HTTP Handler 仅用于外部协议边界或明确的远程兼容场景。Nginx 位于 Server 之前，只负责 TLS/静态资源/反向代理等边缘职责，不承担模块依赖注入或业务路由。

### 3.3 允许与禁止的依赖

允许：

```text
Server Bootstrap -> Market Data Module Constructor
Analysis Application -> Market Data Public Port
Researchflow -> Data Public Application Port
Data HTTP Adapter -> Data Application Port
Data Application -> Data Domain / Repository Port
Data Repository Adapter -> shared DB handle
```

禁止：

- Analysis/Research/Quant 直接 import `internal/marketdata/provider`、`repository`、`model` 等实现包。
- 业务模块直接查询 Data 表或复用 Data Repository。
- Data 反向依赖 Server、WebUI、Researchflow 或 Quant。
- Data 自己创建第二个 HTTP Server、第二套共享 DB Pool 或进程级 Logger。
- 同进程调用绕过 Application Port，直接访问其他模块内部对象。

---

## 4. 推荐代码结构

合并后建议按职责迁移，而不是机械地把整个 `services/market-data` 目录移动到任意位置：

```text
cmd/
  analysis-api/
    main.go                       # 沿用现有 Root Server 入口；本次不强制改名为 cmd/server

internal/
  server/
    bootstrap/                    # 唯一组合根
    transport/                    # Root HTTP Router、认证与 API 前缀
    config/                       # 进程级配置聚合
  marketdata/
    app/                           # 对外 UseCase / Application Port
    domain/                        # 领域模型与规则（从现有 model/service 分层）
    provider/                      # Provider Adapter
    repository/                    # Data Repository Adapter
    cache/                         # Data Cache Adapter
    normalize/                     # 标准化与质量校验
    httpapi/                       # Data HTTP Adapter（只依赖 app Port）
    config/                        # Data 专属配置 Schema/Loader
    bootstrap/                     # Provider/Repository/Worker 装配
  adapters/
    storage/                       # Platform 共享存储 Adapter（不承接 Data SQL）
  platform/
    ...                            # 现有公共技术原语
```

结构是目标边界，不要求一次性重命名所有包。第一阶段可以先保留原有 package 名称与相对目录，仅完成 Go Module 归并和构造装配提取；在可编译、可测试后，再按需要做领域分层重构，避免把“合并部署”扩大成无关重写。

**入口兼容约束：** 当前正式根入口为 `cmd/analysis-api/main.go`，根 Dockerfile/Compose 也围绕该入口构建。本次只要求它演进为 Root Server 唯一组合根，不要求新增或改名为 `cmd/server`；如未来确需统一命名，必须单独评估 Dockerfile、Compose、CI、部署脚本和运维命令的联动成本。

### 4.1 Data 对外 Port

Data 至少提供两类明确接口：

1. **业务 Application Port：** 供 Analysis、Researchflow、Research 等模块使用，返回强类型结果，不依赖 HTTP DTO。
2. **HTTP Handler Adapter：** 供 Root Server 注册为 /market-api/ 等兼容路由，或供受控远程调用使用；只负责请求解析、认证上下文映射、响应编码和错误映射，内部仍调用同一个 Application Port。

Root Server 统一拥有 HTTP 服务实例和路由注册权，但不因此接管 Data 的 HTTP Operation 语义或领域逻辑。Data Owner 维护 Data Handler、请求/响应 DTO、OpenAPI Operation 和对应 Contract Test；Server Owner 负责挂载、全局中间件、统一错误边界及 OpenAPI 汇总。其他模块若开放 HTTP API，遵循相同的职责分工。

不得把 Data HTTP DTO 当作所有同进程模块共享的 Domain 类型。

### 4.2 当前 Analysis 调用点迁移

当前 `internal/adapters/marketdata.Client` 是 HTTP 客户端，提供行情和日线读取能力。目标是新增/替换为进程内 Adapter：

```text
Analysis Runner
    |
    v
MarketDataPort
    |
    v
InProcessMarketDataAdapter
    |
    v
Market Data Application
```

迁移时保留当前调用方真正使用的行为：证券代码规范化、日线数量范围、复权口径、质量状态、来源版本、Provider 时间、抓取时间、单位、错误语义和 PIT 约束。不能因从 JSON 变为 Go 对象而丢失字段或把缺失值转换为零值。

原 HTTP Client 可在兼容期保留为 `RemoteMarketDataAdapter`，供独立部署回滚或未来拆分使用；待内部调用和部署验收完成后，再评估是否删除。不要同时让同一生产调用点在进程内与 HTTP 两种路径间隐式 fallback。

---

## 5. 认证、授权与 Request ID

### 5.1 Data API Key 的调整

同进程内部调用不需要 Server 给自己发送 HTTP，也不需要 Data API Key 作为模块间凭据。目标规则：

- 进程内调用通过 Server Bootstrap 注入的强类型 Application Port，不传递 `X-API-Key`。
- Data API Key 不再作为 Server→Data 内部调用的必要 Secret；完成切换后从 Server Config、Compose 和部署必填项中移除。
- Provider Token 仍只由 Data Config/Provider Adapter 持有，绝不暴露给 Server Application 或 WebUI。
- 如果保留独立 Data HTTP Adapter 供外部网络调用，则该 Adapter 必须有独立明确的认证策略；不能因为内部 Port 不需要 Key 就把原 HTTP 业务 API 改为匿名访问。
- 如果 Data HTTP API 仅通过 Root Server 暴露，Root Server 必须在路由层执行 owner/session/service 认证和对应 Scope 授权，再将可信 Principal 映射到 Data Application 请求上下文。不能让浏览器直接请求 Data Handler，也不能把客户端自报 Principal 当作可信身份。

因此，`DATA-CTR-006` 的“Server 通过 X-API-Key 调用独立 Data 服务”只适用于远程部署 Adapter。采用本方案后，需在契约登记簿明确其远程兼容范围与进程内调用规则，不应继续要求同进程内部调用携带 Data API Key。

### 5.2 DATA-CTR-008 与领域授权必须保留

取消 Data 服务间 API Key 不代表取消 Data 的数据授权：

- caller principal 必须由 Root Server 已验证的认证上下文或受信任模块身份产生。
- purpose_code、Scope、请求范围绑定、Snapshot Manifest 内容寻址和 AuthorizationRef 签发语义保持不变。
- `subject_ref` 不得由请求体、Query 或任意 Header 自报。
- Data 读取受限快照时仍需执行 Owner 当前授权复核；撤销、过期、unknown、依赖不可用和 TOCTOU 仍然 fail closed。
- Data API Key 不写入 AuthorizationRef、Manifest、fingerprint 或日志。
- 同进程调用必须保留审计主体、purpose、resource_ref、Request ID 和授权决策证据。

### 5.3 Request ID

- Root Server 在可信入口生成/校验 Request ID，并写入标准 Request Context。
- 进程内 Port 调用通过 context 原样传播同一个 Request ID，不重新生成无关联 ID。
- Data 的日志、诊断、错误和 Snapshot/审计记录按契约保留 Request ID / originating_request_id / correlation_id 的作用域。
- Request ID 不是身份或授权凭据。
- 未来重新拆分 Data 时，HTTP Adapter 从同一 Context 传播 Request ID，恢复跨进程关联。

---

## 6. 数据库、Redis 与 Migration

### 6.1 MariaDB

- MariaDB 仍是事实源；本方案不更换数据库、不迁移历史数据、不改变表结构。
- Root Bootstrap 统一拥有 MariaDB Pool（可按有效 DSN 创建一个或多个 Pool）；仅当 Root/Data 的 Host/Port/Database/User/Password/Charset/Timezone/连接参数完全一致时才共享同一个 `*sql.DB`，否则 Root 创建并持有独立 Data Pool，Data Repository 仍只接收 borrowed handle；Data 仍拥有所有 Data SQL、Repository 和 Migration 内容。Data Store 必须区分 borrowed/owned 资源，只有独立部署兼容入口可以关闭自有 DB Pool。
- 各模块使用独立 Migration namespace/version；Server 统一按明确顺序调用各模块 Migration Runner。
- Migration 只能由组合根执行一次；禁止 Server Store 和 Data Store 各自自动执行迁移。当前 Root Store.Open 会自动执行 Analysis Migration/CheckSchema，Data main 也会按配置执行 Data Migration；合并前必须把两侧 Migration 都改为显式注册，由 Server Bootstrap 按 namespace 顺序执行，随后分别 CheckSchema。
- Migration 失败必须阻止 readiness/启动完成；不能继续接受请求并把缺表问题留到首次调用。
- 连接池的 MaxOpen/MaxIdle/ConnMaxLifetime 应按总负载统一配置，模块不能各自重复创建池或覆盖全局参数。
- 需要验证 MariaDB Driver 版本、时区、ParseTime、字符集、连接参数和现有表兼容。

### 6.2 Redis

- Redis 是可选缓存/协调基础设施，不得仅因同进程就升级为所有模块的强依赖。
- Server Bootstrap 可创建共享 Redis Client/Pool；Data Cache Adapter 接收接口或客户端句柄。优先将当前每条命令重新 Dial 的自定义 RESP 实现替换为 go-redis Adapter；若 Data Redis DB index、用户名/密码、TLS 或连接参数与 Root 不同，由 Server 创建独立 Client 并统一 Close。
- Data 缓存 key 前缀、TTL、序列化和失效规则仍由 Data 拥有。
- Redis 不可用时的行为必须沿用当前 Data Config：明确 fail-fast、降级或禁用缓存，不得在合并时改变。
- Server 负责最终 Close；Data 负责停止使用资源，不重复关闭共享客户端。
- 若模块未来需要独立 Redis DB/连接参数，作为 Adapter 配置隔离，不等于必须独立部署。

### 6.3 Data 后台任务

- Quote Retention Worker、Provider Probe/诊断及其他 Data 定时任务由 Data 模块提供 Start/Stop 生命周期。
- Server 统一启动 Worker，并在 HTTP Server Shutdown 前停止接收新任务、等待执行中任务 drain。
- 每个 Worker 必须响应 context 取消、设置超时、避免重复启动；启动失败需回滚已启动资源。
- 不得由 Data 模块自行安装第二套进程信号处理器。

---

## 7. HTTP 路由与 WebUI 迁移

### 7.1 路由目标

当前 WebUI 使用两个同源前缀：

- `/analysis-api/` → Server :8092
- `/market-api/` → Data :8091

目标保留前端可见路径，统一由 Nginx 转发至 Root Server :8092：

- `/analysis-api/` → Root Server Analysis Router
- `/market-api/` → Root Server 挂载的 Market Data HTTP Adapter

这能降低 WebUI 改动范围，并避免在同一轮同时重写所有前端 API URL。路由迁移必须保证 path strip/prefix、Query、方法、状态码、响应体和超时语义不变。

### 7.2 认证要求

- `/market-api/` 不再直连 Data 容器。
- 浏览器不得持有 Data API Key；删除 WebUI 的 Data Key 输入、localStorage 读写和 X-API-Key 请求头。
- Root Server 在 `/market-api/` 路由边界执行现有 WebUI 所需的认证与授权。若现有前端依赖 Owner Token，应由 Server 认证机制处理，不能把 Data Key 替代为 Owner Token。
- 不允许把客户端提交的 X-API-Key 透传给 Data Handler。
- Data Handler 若继续保留旧 Key middleware，只能用于独立远程部署 Adapter；Root Server 同进程挂载时必须使用明确的受信任内部装配模式，不能通过空 Key/默认匿名绕过安全。
- 认证/授权失败必须与 Provider/数据源故障区分；健康检查不得暴露 Secret。

### 7.3 Nginx 与 Compose

- Nginx 的 `MARKET_DATA_SERVICE_NAME` 改为 Root Server 服务名，或改为统一 `ANALYSIS_SERVICE_NAME`，避免两个服务变量指向同一目标造成误导。
- Compose 移除 `stock-analysis-data` 服务、Data 独立端口 expose、Server 的 Data depends_on 和 Web 对 Data 健康状态的依赖。
- Root Server 容器负责 Data 配置、Provider Token、DB/Redis 依赖和 Data Worker。
- Web 只依赖 Root Server healthy。
- Server readiness 应纳入必须的模块装配和必要依赖状态；Provider 单个上游不可用不应自动导致进程不健康，遵循 Data 质量/诊断契约。
- 对外只暴露 Web 端口；Server 继续仅在受控网络或 loopback 暴露，不把 Data 业务端口独立暴露。

---

## 8. 迁移阶段与实施任务

迁移按阶段进行，每阶段可独立 Review 和回滚；不允许一次性大搬迁后才第一次编译。

### M0：依赖审计与冻结清单

Owner：项目总规划 + Server + Data

交付：
- Data 全量 import 依赖与 Go 版本清单。
- OpenAPI 35 个 Operation 与 Handler 的 Method/Path/security/参数/Request Body/成功响应/错误状态码/Error Schema 逐项核验；修复 22 条 ApiKey 漏声明、Diagnostics 401、错误响应 Schema 引用和真实可达 408/504，再以全量 Contract Test 锁定实际行为。
- Provider 构造、DataService 构造、Repository/Cache 构造和所有后台 Worker 清单。
- 所有 Data HTTP 路由、WebUI API 使用点和 Nginx 转发规则清单。
- Data 表/索引/Migration namespace 与现有 DB pool 配置清单。
- DATA-CTR-006/008、PLAT-CTR-005/006/007、SRV-CTR-001 受影响点清单。
- go-core 入口与 Compose 正式入口关系的明确结论；不得只按目录名称推断是否仍是生产入口。

验收：每个迁移对象有当前路径、目标路径、Owner、兼容要求和测试证据；无未登记的 Data HTTP 调用点；OpenAPI 的 35 个 Operation 与实际 Handler 的认证、参数、错误状态码和 Error Body 一致，全量 Contract Test 通过，M1 不得带着已确认的契约描述错误进入归并。

### M1：Go Module 归并（不改变部署调用）

Owner：Data + Server

交付：
- 将 `services/market-data` 代码纳入根 Go Module，迁移到 `internal/marketdata`。
- 统一 Go 版本与依赖；更新 go.mod/go.sum、Docker Builder、CI。
- 删除嵌套 go.mod/go.sum 和独立 Data Docker Build（仅在确认无其他开发流程依赖后）。
- 暂时保留原 Data HTTP API 与远程 Server Adapter，使现有部署仍可运行。
- 所有 Data 单元测试、集成测试和 OpenAPI Contract Test 在根 Module 下可执行。

验收：根 Module 的 `go test ./...`、`go vet ./...`、Data 相关 race 测试通过；无 import internal 越界；Data API 行为未改变。

### M2：提取 Data Module Bootstrap 与 Application Port

Owner：Data

交付：
- 从 `cmd/market-data/main.go` 抽出可注入 Config、DB、Redis、Logger、HTTP Client、Clock 等依赖的 Module 构造函数。
- 返回 Data Application Port、HTTP Handler、Diagnostics、Start/Stop 或 Close 生命周期接口。
- 将 Provider 构造与能力注册从 main 中移入 Data Bootstrap；配置校验保持 Data Owner。
- 把 Data HTTP Handler 改为只依赖 Data Application Port，不直接拥有业务逻辑。
- 为内部调用定义强类型 Application DTO/Port；不要直接复用 HTTP JSON DTO。Application DTO 必须保留 Data Model 的 nullable、整数精度、单位、来源、Provider 时间、复权、质量、fallback、标准化和 PIT 元数据。按领域拆分 Application 用例接口；Analysis Adapter 只依赖 Quote/DailyBars/IndexBars 所需 Port，不能将 Data 的全部 HTTP Operation 做成一个巨大接口。
- 现有独立 Data main 暂时改为调用同一 Module Bootstrap，避免出现两套 Provider 装配逻辑。
- 将 Data DailyBars 的 `as_of` 修订查询语义纳入 Application Port；Analysis Adapter 必须传递明确的 PIT cutoff，不得只按交易日期过滤未来数据。

验收：Data 可以在单元测试中无 HTTP Server 启动；HTTP Handler 和 InProcess Adapter 使用同一 Data Application 实例；Start/Stop 可重复性与失败回滚有测试。

### M3：Root Server 同进程装配

Owner：Server + Data

交付：
- Server Bootstrap 创建共享基础设施并注入 Data Module。
- Analysis 的 MarketDataPort 改为 InProcess Adapter；不再使用 Base URL/API Key 做内部调用。
- Researchflow/Research 等模块如需 Data，统一依赖 Data Public Port；不得自行创建 HTTP Client 或绕过 Port。
- Data HTTP Adapter 由 Root Server Router 挂载到 `/market-api` 路由前缀，并由 Root Server 统一执行外部认证与授权。
- 先建立可信的进程内调用上下文承载既有调用身份；Principal/Scope/purpose_code 的最终字段与校验语义在 MDM-008 按 DATA-CTR-006/008 对齐，不在结构迁移阶段自行发明契约。
- Data 的 Migration 与 Worker 由 Server 统一编排资源生命周期；具体共享资源与 Migration Runner 由 M4 完成。

验收：同一 Data 请求不产生内部 HTTP 往返；不存在重复 Provider 实例、重复 DB Pool、重复 Migration、重复 Worker 或双重 Request ID。

### M4：共享资源与生命周期统一

Owner：Server + Data + Platform

交付：
- MariaDB：Root Server 统一拥有一个或多个 Pool；仅在 Root/Data 有效 DSN 与凭据完全一致时共享同一 Pool，否则由 Root 创建并持有独立 Data Pool。Data Repository 明确区分 borrowed/owned，进程内模式只接收 borrowed `*sql.DB`，独立兼容入口才拥有并关闭自有 Pool。
- Migration：将 Root Store.Open 与 Data main 中的隐式迁移副作用拆出；由 Root Bootstrap 显式注册 Analysis/Data Migration Runner，按 namespace 和依赖顺序执行。必须保留 Data Go Migration 中的索引唯一性修复、缺列补齐等兼容升级逻辑，不能只执行静态 SQL 文件。
- Redis：Data Cache Adapter 接收共享 go-redis Client；若 Data DB index/连接配置不同，由 Bootstrap 创建独立 Client 并统一关闭；保留 Data Cache 禁用、降级、key namespace 和 TTL 语义。
- Config：拆分 Data 领域配置与远程 HTTP Adapter 认证配置；同进程不要求 Data API Key；`configs/providers.json` 改为嵌入资源或由 Bootstrap 显式传入路径，避免根镜像工作目录变化后配置文件丢失。
- Provider HTTP：按 PLAT-CTR-005 使用受控 HTTP Client，逐 Provider 配置 Host Allowlist、Redirect、代理策略、响应体预算和并发限制；必须增加双 Host 本地测试覆盖 307/308、跨 Host、HTTP/HTTPS 降级，验证 Tushare Body Token、Hithink/TickFlow Key Header、Xueqiu Cookie 不会到达未授权 Host。BaoStock/PyTDX 原生 TCP 不得套用 HTTP Client，单独保留拨号/重连边界。
- Worker：Retention Worker 等后台任务提供 Start/Stop/Wait，支持 Context 取消、超时、重复启动保护和 drain；Data 不自行处理进程信号。
- 资源关闭：Data 不关闭 borrowed DB/Redis；Root 先停止 Worker，再关闭 HTTP Server、Module 自有资源和共享 Pool。

验收：
- 每个 DB Pool、Redis Client、Provider Client 和 Worker 只有一个明确 owner。
- Root/Data Migration 各执行一次；Migration 失败阻止 readiness。
- Data Repository 不会关闭 borrowed Pool；共享 Redis Client 不会被 Data 重复关闭。
- Worker 可取消、等待退出；关闭顺序与失败回滚有集成测试。
- Provider HTTP 的 Host/Redirect/代理/响应限制与迁移前请求语义有逐 Provider 对照；TCP Provider 有独立网络策略说明。

### M5：WebUI 与部署切换

Owner：Server + WebUI

交付：
- Nginx 两个 API 前缀统一转发 Root Server。
- WebUI 删除 Data API Key 配置、localStorage 读写及请求 Header。
- 清理旧 localStorage 中 Data Key 字段；若历史上存放过真实有效 Key，部署前轮换旧 Key。
- 若 Data 旧 Handler 在 MDM-008 前仍依赖 X-API-Key，短期凭据只允许由 Root Server 内部兼容 Adapter 注入，不得重新暴露给浏览器；MDM-008 完成可信 Context 后移除同进程路径上的 Key。
- Compose 删除独立 Data 服务及其健康依赖；Server 纳入 Data Provider 配置和 Data Worker。
- 更新 .env.example、部署说明、健康检查和回滚文档。

验收：浏览器 Network/Storage/Source Map/响应体中均无 Data API Key；`/market-api` 与 `/analysis-api` 均可用；Server 单容器重启后 Data 与 Analysis 同时恢复。

### M6：结构迁移稳定性与兼容回归

Owner：Server + Data + Platform

交付：
- 启动失败回滚、优雅关闭、Worker drain、DB/Redis 断连恢复测试。
- Data Repository/Migration 只执行一次的集成测试。
- Provider 超时/失败、Data 质量降级、缓存不可用、HTTP 请求取消等回归。
- 迁移前后 Data v1 HTTP/API 行为、Provider 结果、单位、质量、PIT、缓存和历史数据固定样本对账。
- 记录未来恢复独立部署的触发阈值与拆分步骤。
- 本阶段不提前宣称 DATA-CTR-005～008 已实现；正式契约改造留到 MDM-008。

验收：结构迁移不改变现有 v1 API 与业务行为；单位、质量、PIT、缓存和历史数据没有回退；启动/关闭和回滚路径可执行。正式跨模块契约验收在 MDM-008 单独完成。

### 迁移后按公共契约对齐（MDM-008）

Owner：Data + Server + Platform；Quant/Researchflow 作为消费者验收方。

- 按 DATA-CTR-005/007 更新精度 DTO、Application Port 映射与 PIT 输入语义；保留 v1 兼容边界。
- 按 DATA-CTR-006/008 将同进程调用改为 Root Server 可信 Principal/Scope Context，移除同进程 Data API Key；远程 Adapter 如继续保留则独立保留 Key 认证。
- 补齐 Snapshot Manifest、AuthorizationRef、purpose Scope、读取时当前授权复核、策略版本竞态和 Request ID 传播。
- 更新 OpenAPI、Schema、Golden、Validator、Contract Test 和 Data/Server/消费者文档；文档路径随 M1 迁移到 `internal/marketdata/httpapi/openapi.yaml`。
- 按真实 CI 与跨模块测试证据升级契约状态，不因目录迁移自动 frozen。

验收：DATA-CTR-005～008 与 GOV-CTR-006 的 Schema/Golden/运行时测试和消费者映射一致；未解决项按实际调用点阻断，不将远程兼容 Adapter 的 Key 轮换误当作同进程接入门禁。

---

## 9. 契约与文档同步要求

本方案获项目决策后，需要按以下方式同步，不允许各业务分支自行发明兼容语义：

| 契约/文档 | 调整内容 |
|---|---|
| SRV-CTR-001 | 补充 Data Module 注册、共享 DB/Redis、HTTP Router、Worker 生命周期、启动失败回滚和关闭顺序 |
| DATA-CTR-006 | 明确 X-API-Key 仅用于远程 Data HTTP Adapter；同进程内部 Port 不使用 Data API Key；保留外部 Adapter 的认证要求 |
| DATA-CTR-007 | Research 使用 Data Public Application Port 或其稳定 Remote Adapter；首批 DailyBarsV2 字段与 PIT 语义不变 |
| DATA-CTR-008 | 主体改由 Root Server 可信 Principal 注入；Manifest、Scope、purpose 和当前授权复核语义不变 |
| PLAT-CTR-005 | HTTP Client 只用于真实出站 Provider/外部依赖；Server→Data 内部调用不再经过 HTTP Client；Data Provider 网络安全仍必须遵守公共安全策略 |
| PLAT-CTR-006 | Server 统一创建共享连接池和 Migration Runner；Data 仍拥有自己的 Schema/Repository/Migration 内容 |
| PLAT-CTR-007 | Data Config 由 Server Bootstrap 装配，环境变量只在 Bootstrap 读取；模块 Config 保持独立类型 |
| 跨模块接入执行说明 | 从“Data 独立 Module 只能经 HTTP 接入 Platform”调整为“Data 与 Server 同一根 Module，通过公共 Port 接入；不 import 其他模块 internal” |
| Data 服务间认证映射与 Secret 注入设计 | 标注独立部署模式为兼容/未来模式；当前目标模式不需要 Server→Data API Key |
| 部署说明与 WebUI 验收清单 | 更新容器拓扑、Nginx 上游、API Key 清理、健康检查和回滚步骤 |

契约状态不能因文档变更自动升级为 frozen；仍需各 Owner 的 Schema/Golden/Contract Test 和 CI 证据。

---

## 10. 成本、收益与风险

### 10.1 一次性改造成本

基于当前已看到的结构估算；需要 M0 全量依赖审计后再收敛。

| 工作 | 估算 | 说明 |
|---|---:|---|
| M0 依赖/调用/迁移审计 | 1～2 人日 | 已完成初步只读核对；仍需逐 Provider/路由/迁移全量盘点 |
| M1 Go Module 归并与依赖统一 | 1～2 人日 | import 路径、Go 版本、CI、Docker |
| M2 Data Bootstrap / Port 提取 | 2～3 人日 | Provider 装配、Config 双模式、HTTP middleware 拆分 |
| M3 Root Server 同进程装配 | 1～2 人日 | Data Module 注册、Analysis InProcess Adapter、Root Router 装配 |
| M4 共享资源与生命周期统一 | 2～3 人日 | borrowed DB、go-redis Adapter、显式 Migration、Worker drain、Provider HTTP Client 接入评估 |
| M5 WebUI/Nginx/Compose 迁移 | 1～2 人日 | Key 清理、API 路由与部署 |
| M6 结构回归与稳定性验收 | 1～2 人日 | 行为一致性、安全、PIT、生命周期 |
| MDM-008 契约对齐 | 1～2 人日 | DATA-CTR-005～008、GOV-CTR-006 的 DTO/身份/授权/Golden 对齐 |
| **合计** | **10～18 人日（待 M0 收敛）** | 不是承诺工期；M0～M6 结构迁移及 MDM-008 契约接入估算，包含 DB/Redis Adapter 与生命周期调整，不包含无关领域重构和新增业务功能 |

相比此前估算，本轮将共享资源/生命周期独立列为 M4，并纳入 Root DTO/PIT 调用语义修复、Provider 受控出站网络和旧 Profile 兼容决策，暂按 10～18 人日估算；M0 完成逐调用点和部署现场核验后再收敛。

### 10.2 长期收益

- 减少一个常驻容器、一套健康检查和一套服务间配置。
- Server 内部 Data 调用不再经过 TCP/JSON/API Key，减少网络跳数和重复序列化。
- 统一启动、日志、Request ID、诊断、资源池和 Worker 生命周期。
- 避免 Data 与 Server 对同一数据库各自维护连接池、迁移入口和部署依赖。
- 新模块默认采用同进程 Port，只有满足拆分条件才承担独立部署成本。

### 10.3 主要风险与控制

| 风险 | 控制 |
|---|---|
| Data 单个 Provider 阻塞影响 Server | Provider 超时、context 取消、并发限制、熔断和诊断隔离；不得把同步 Provider 调用放到启动关键路径 |
| Data 资源泄漏导致进程整体资源耗尽 | 统一连接池、并发预算、响应体限制、Worker 生命周期和资源关闭测试 |
| Data Handler 直接暴露绕过认证 | Root Server Router 统一认证；Data Handler 只作为内部 Adapter 挂载 |
| 一次性搬迁影响行情数据 | 不改 Schema/历史数据；先 Module 归并，再切换调用路径 |
| Go 依赖升级造成回归 | 固定版本、完整测试、依赖差异 Review；不做顺手升级 |
| 未来需要独立扩容 | 保留 Remote Adapter、HTTP DTO/Schema 和 Application Port；通过适配器替换恢复独立服务 |

---

## 11. 未来何时重新拆成独立服务

以下任一条件形成可量化、持续的实际需求时，再评估 Data 独立部署：

1. Data 需要独立扩容，CPU/内存/网络资源与 Server 明显不同。
2. Data Provider 任务的故障域或发布节奏需要与 Server 隔离。
3. 多个独立系统需要复用 Data API，且不能通过统一 Gateway/Server 入口解决。
4. Data 需要独立安全网络区、独立出站策略或独立凭据边界。
5. Data 需要独立部署窗口、版本节奏或 SLA。
6. 同进程资源竞争通过限流、Worker 隔离和资源预算仍无法解决。

重新拆分时：

- 保留 Data Application Port 作为领域边界。
- 将 InProcess Adapter 替换为 Remote HTTP Adapter。
- 使用现有版本化 DTO/OpenAPI、认证和 Request ID 契约。
- Data 重新获得独立 Bootstrap/进程入口，但不复制业务实现。
- 不改变 Data Repository Owner 和数据质量语义。

---

## 12. Review 清单与当前未决项

### 12.1 架构 Review

- [x] 目标运行时选择同进程模块化单体。
- [x] Server 保持唯一组合根。
- [x] Data 保持行情领域唯一 Owner。
- [x] 模块调用通过 Application Port，不访问对方 internal。
- [x] Data 的 HTTP API 保留为 Adapter，不作为内部调用必经路径。
- [x] DATA-CTR-008 授权语义不随 API Key 移除而取消。
- [x] MariaDB/Redis/Worker 生命周期归 Root Server 统一编排。
- [x] WebUI API 前缀可保留，Nginx 上游统一指向 Root Server。
- [x] 明确未来独立部署的触发条件。

### 12.2 实施前仍需核实

1. 根模块当前 `mysqlredis.Store` 与 Data `repository.Store` 的实际 DB/Redis 资源是否可抽象为共享句柄；确认是否存在相互依赖或不同 DSN/事务设置。
2. Data 所有 Migration 的执行顺序、namespace 和幂等性；当前 Data main 与 Server storage Open 是否都执行迁移。
3. Data 真实 HTTP API 是否还有除 WebUI、Analysis Adapter 以外的调用者。
4. 代码层面确认 `go-core` 仅在独立的 `go-migration` profile 中与 Python Bridge、Data 组合，根 Compose 使用 `cmd/analysis-api`；仍需部署现场确认未继续使用该旧 profile。
5. WebUI 当前 Data Key 输入与 localStorage 使用点、Owner Token/session 认证方式及 CORS/CSRF 行为。
6. Data HTTP API 是否必须继续允许独立部署调用；如保留，需定义独立模式的 Key 生命周期与网络 ACL。
7. 已确认 Data 当前 Redis Adapter 每条命令单独建立 TCP 连接；目标优先改为共享 go-redis Client Adapter。若 Redis DB index/连接参数不同，则由 Server Bootstrap 创建独立 Client 并统一 Close。

### 12.3 明确不在本轮做的事情

- 不在 `feat/design_doc` 修改业务 Go 代码。
- 不直接修改 `feat/full-go-data-providers-api-switch`、`feat/platform-foundation`、WebUI 或其他模块分支。
- 不删除 DATA-CTR-006、PLAT-CTR-005 或 Data HTTP API 的既有定义；先按本方案明确适用模式，再由项目总规划更新契约登记。
- 不因部署合并把所有业务模块强行放入同一个 package。
- 不创建 Portfolio、Alert、Report 等候选模块分支。
- 不以本次设计 Review 代替模块 CI、集成测试或真实部署验收。

---

## 13. 建议执行顺序

1. 在项目总规划 Review 本方案，并确认 DATA-CTR-006 远程/进程内双模式边界。
2. 先执行 M0，只读完成依赖、数据库、Redis、Worker、WebUI 路由和 go-core 入口审计。
3. M0 通过后，分配 M1/M2 给 Data Owner，M3 给 Server Owner；两者可并行，但必须以相同设计基线协作。
4. M1/M2 合并到集成基线并通过模块 CI 后，再进行 M3 同进程装配。
5. M3 通过后，WebUI/Server 执行 M4；最后完成 M5 部署验收。
6. 所有契约继续保持 `pending_evidence`，直到真实测试和 CI 证据齐全。

**最终决议：采用方案 B。先统一 Module 和进程，再逐步切换内部 Port 与部署；不进行无边界的大规模代码搬迁。**
