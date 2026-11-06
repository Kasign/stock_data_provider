# Market Data 模块化单体迁移计划与进度

- 维护分支：`feat/full-go-data-providers-api-switch`
- 迁移决策：采用 Root Server 同进程模块化单体
- 方案依据：`doc/整体任务规划/Market Data并入Root Server模块化单体调整方案.md`
- 总体规划：`doc/整体任务规划/整体任务规划总览.md`
- 跨模块准入：`doc/整体任务规划/跨模块接入执行说明.md`、`doc/整体任务规划/跨模块接入准入清单.md`
- 本记录建立时的代码基线：`d8595f408337f62be3697aac661ba1570fdd3667`
- 最近一次 M0 审计基线：`f2b9e5fc637ea59bff87f5449823e98f746c8428`（文档目录整理后）
- 当前实施状态：`M0_audit_in_progress`（文档结构已整理，开始逐项核对；尚未执行代码迁移）

> 本文记录 Data 从独立 Go Module/服务迁入 Root Server 根 Go Module、同进程运行的实施计划和进度。若本文与上述架构决策文档冲突，以架构决策文档为准；公共平台及 DATA 契约以冻结契约文档为准。本文不重新定义契约。

## 1. 目标与边界

### 1.1 目标

将 `services/market-data` 从独立 Go Module 和独立默认部署单元迁入根 Go Module，由 Root Server 作为唯一组合根，在同一进程中装配 Analysis 与 Market Data。

迁移后：

- Market Data 保留独立领域边界、数据事实所有权、Provider、标准化、质量、PIT、Repository、表结构和 Migration 所有权。
- Server 负责进程入口、配置聚合、依赖装配、共享基础设施生命周期、路由、认证上下文和优雅关闭。
- Analysis 等消费者只依赖 Data 的公开 Application Port，不依赖 Provider、Repository、内部 Model 或 SQL。
- Data HTTP Adapter 可保留，供兼容 API 或未来远程部署使用；Server 内部默认走进程内 Port，不通过 HTTP 自调用。
- WebUI 继续使用兼容的 `/market-api/` 路径，但由 Root Server 提供路由。
- 不因迁移改变数据语义、API DTO/Schema、数据库结构或历史数据。

### 1.2 非目标

本次迁移不包含：

- 重写 Provider、行情标准化、单位/精度、复权、交易日历、质量和 PIT 规则。
- 数据库表重命名、历史数据搬迁、数据清洗或重新计算。
- 将 Data Repository、SQL 或领域规则移交给 Server/Platform。
- 把 News、Quant、Researchflow 等其他模块一并重构。
- 删除远程 HTTP Adapter 或历史 go-migration profile（除非后续单独决策）。
- 在迁移阶段提前实施尚未冻结或仍有边界争议的公共契约改造。

### 1.3 关键约束

1. 只在当前分支实施；所有分支更新使用最新 HEAD 和 `expected_sha`。
2. 先完成迁移，再按冻结契约接入公共平台能力；不得把迁移与大规模契约重构混成一个提交批次。
3. 迁移前后保持 Provider 行为、API 语义、数据库 Schema、PIT 和质量状态一致。
4. 不允许消费者绕过 Application Port 访问 Data 内部包或数据库。
5. 不允许同一业务调用隐式地在 InProcess 与 Remote HTTP 两条路径间 fallback。
6. 共享资源必须明确 borrowed/owned 语义，禁止重复创建连接池或重复关闭共享资源。
7. Migration 由组合根显式注册和执行；不得在连接池 Open 中隐式执行。
8. 不把 CI 合约测试通过等同于真实 Provider 数据验收通过。

## 2. 目标结构

建议目标结构（具体包名可根据现有代码做最小调整，不为目录形式而机械重构）：

```text
cmd/
  server/
    main.go

internal/
  server/
    bootstrap/
    transport/
    config/

  marketdata/
    app/             # 对外 Application Port / UseCase
    domain/          # 领域规则与模型
    provider/        # Provider Adapter
    repository/      # Data Repository Adapter
    cache/           # Data Cache Adapter
    normalize/       # 标准化与质量规则
    httpapi/         # 兼容 HTTP Adapter
    config/
    bootstrap/       # 模块构造、Worker 生命周期

  platform/
    ...              # 公共基础设施
```

第一阶段允许先将现有代码整体迁入根 Module 下的临时兼容目录，保持 package 名和相对结构，优先完成 Module 归并与编译；后续再以小批次提取 Application Port 和 Bootstrap。不得将“迁移”扩大成无关的领域分层重写。

## 3. 阶段计划与验收门槛

### M0：迁移前代码审计与清单冻结

状态：`pending`

工作项：

- [ ] 获取迁移执行时最新 HEAD、分支状态和工作区/远端变更基线。
- [ ] 盘点根 Module、Data Module 的 Go 版本、依赖、replace、生成代码和工具链。
- [ ] 盘点 Data 全部 package、命令入口、测试、testdata、配置文件、脚本和 Docker 构建上下文。
- [ ] 盘点 Root Server 到 Data 的所有调用点，标记请求/响应 DTO、认证和超时行为。
- [ ] 盘点 Data HTTP 路由、中间件、OpenAPI、WebUI/Nginx 路由依赖。
- [ ] 盘点 MariaDB Pool、Redis Client、Migration、Probe 持久化和 Worker 的创建/关闭路径。
- [ ] 盘点 Compose、Dockerfile、CI workflow、部署脚本及 go-migration profile。
- [ ] 为每项迁移对象记录源路径、目标路径、依赖、风险、测试和回滚方式。
- [ ] 标记迁移阻断项；没有明确处理策略的阻断项不得进入对应阶段。

验收：

- [ ] 有完整迁移对象清单，覆盖生产代码、测试、配置、构建和部署文件。
- [ ] 所有跨 Module import 与内部包边界有明确处理方式。
- [ ] 资源所有权和生命周期有明确图示/清单。
- [ ] 迁移前基线测试和 CI 状态可追溯。

### M1：Go Module 归并

状态：`pending`

工作项：

- [ ] 将 Data 源码和配套测试迁入根 Module 的 Market Data 领域目录。
- [ ] 统一 Go 版本和依赖版本；以依赖兼容和 CI 结果确定最终版本，不盲目降级。
- [ ] 合并根 `go.mod/go.sum`，清理嵌套 Module 依赖声明。
- [ ] 调整 Docker Builder、CI、开发脚本和相关文档的 Module 路径。
- [ ] 暂时保留 Data 原有 HTTP API 和行为；不在此阶段改变认证与业务语义。
- [ ] 保留独立 Data 命令的兼容构建能力，直到 M2/M3 完成。

验收：

- [ ] 根 Module 可构建全部 Market Data 包和测试。
- [ ] 不再依赖嵌套 Data Module 才能编译。
- [ ] Provider、标准化、质量、PIT、Repository 测试通过。
- [ ] OpenAPI/HTTP Contract 测试通过。
- [ ] 迁移前后关键 DTO 和数据库 Schema 无非预期差异。

### M2：Data Application Port 与 Bootstrap 提取

状态：`pending`

工作项：

- [ ] 从 Data main 中提取模块构造装配，main 不再独占 Provider/Repository/Worker 装配逻辑。
- [ ] 提供强类型 Application Port，供 Server/Analysis 进程内调用。
- [ ] HTTP Handler 只依赖 Application Port，不将 HTTP DTO 作为领域共享类型。
- [ ] 明确模块返回的 Application、HTTP Handler、Diagnostics、Migration Runner 和生命周期句柄。
- [ ] Provider、Repository、Cache、Worker 的构造与 Start/Stop 有明确所有权。
- [ ] 独立兼容入口（如保留）与 Root Server 使用同一套 Module Bootstrap。
- [ ] 初始化失败时逆序释放已创建资源；Start/Stop 支持取消、超时和重复调用保护。

验收：

- [ ] 不启动 Data HTTP Server 也能通过 Application Port 完成核心用例。
- [ ] HTTP Adapter 与 InProcess Adapter 使用同一个 Application 实例。
- [ ] Application DTO 保留 nullable、精度、单位、来源、时间、复权、质量和 PIT 信息。
- [ ] 生命周期和初始化失败路径有自动化测试。

### M3：Root Server 同进程装配

状态：`pending`

工作项：

- [ ] Root Server Bootstrap 创建并持有共享基础设施。
- [ ] 构造 Market Data Module 并注入公开 Application Port。
- [ ] 将 Analysis 当前 Remote HTTP Client 替换为 InProcess Adapter。
- [ ] Data HTTP Handler 挂入 Root Server Router，兼容既有 `/market-api/` 路径。
- [ ] Root Server 统一入口 Request Context、认证和路由中间件。
- [ ] Data 领域授权仍由 Data 按契约执行；不因同进程取消 Scope、purpose 或审计。
- [ ] 禁止同一调用路径隐式 fallback 到 Remote HTTP。
- [ ] 明确 readiness 对 Data 模块初始化失败的反映规则。

验收：

- [ ] Analysis 只依赖 Data Application Port。
- [ ] 同进程调用结果与迁移前 Remote Adapter 的业务语义一致。
- [ ] Request Context 可贯穿入口、Application、Repository 和诊断记录。
- [ ] 外部 HTTP 路由仍执行正确的入口认证/授权。
- [ ] 不存在 Data 对 Server 的反向依赖。

### M4：共享资源与生命周期统一

状态：`pending`

工作项：

- [ ] MariaDB：Root Server 创建共享 Pool，Data Repository 明确接收 borrowed `*sql.DB`；Data 仍拥有 SQL 和 Repository。
- [ ] Repository 支持 borrowed/owned 资源语义；只有独立兼容入口关闭自有 Pool。
- [ ] Migration：Root Server 显式注册各模块 Runner，按 namespace 和依赖顺序执行；禁止 Store.Open 隐式迁移。
- [ ] Redis：Data Cache 适配共享 go-redis Client；如 Redis DB/配置不同，由组合根创建独立 Client 并明确拥有者。
- [ ] 保留 Data Cache key namespace、TTL、禁用缓存和降级语义。
- [ ] Provider HTTP：使用冻结的 Platform 受控 HTTP Client；若契约接入尚未具备条件，不自行复制安全实现，记录阻断并等待准入。
- [ ] Config：区分领域配置与远程 HTTP Adapter 认证配置；进程内模式不要求 Data API Key。
- [ ] Worker：Data 拥有执行规则，Root Server 统一 Start/Stop 和 drain。
- [ ] Logger/Diagnostics/Request Context 由组合根装配并按契约注入。

验收：

- [ ] 每个 Pool/Client/Worker 只有一个明确 owner。
- [ ] Migration 仅由组合根执行一次，失败阻止 readiness。
- [ ] Data Repository 不会关闭 borrowed Pool。
- [ ] Redis 共享或独立 Client 的配置语义可验证。
- [ ] Shutdown 顺序、超时和资源释放有集成测试。

### M5：部署与 WebUI 路由切换

状态：`pending`

工作项：

- [ ] Compose 默认只部署 Root Server 与 WebUI，不再依赖独立 Data 容器。
- [ ] Nginx 的 `/market-api/` 与 `/analysis-api/` 代理到同一 Root Server。
- [ ] WebUI 删除浏览器持有的 Data API Key、localStorage 持久化和请求头注入。
- [ ] 清理旧凭据设置；对曾暴露在浏览器中的有效 Key 制定轮换步骤。
- [ ] Provider Token 仍只由 Data Config/Provider Adapter 持有，不进入 WebUI。
- [ ] Data 配置、Worker、探测和 readiness 迁入 Root Server 部署配置。
- [ ] 保留远程 Data Adapter/历史 profile 作为显式兼容或回滚路径，直到部署验收通过。

验收：

- [ ] 全新部署不需要独立 Data 容器。
- [ ] WebUI 既有行情功能通过兼容路由正常工作。
- [ ] 浏览器存储、请求和构建产物不再包含 Data API Key。
- [ ] Server readiness、日志、诊断和优雅关闭覆盖 Market Data。
- [ ] 部署回滚步骤可执行且不破坏数据库数据。

### M6：迁移验收与稳定性观察

状态：`pending`

工作项：

- [ ] 对照迁移前基线检查 API、DTO、Provider、质量、PIT 和数据库行为。
- [ ] 验证 MariaDB/Redis 断连、恢复、超时与关闭。
- [ ] 验证 Worker 启停、取消、重复启动保护和 drain。
- [ ] 验证 Provider 超时、限流、不可用和降级不导致 Root Server 无故退出。
- [ ] 验证 Request ID、Fault、日志和诊断关联。
- [ ] 验证旧 Data HTTP Adapter 的显式兼容路径（若保留）。
- [ ] 检查重复 Pool、Provider、Worker、Migration 和双路调用。
- [ ] 形成迁移验收结论和剩余风险清单。

验收：

- [ ] 当前 HEAD 的根模块测试、race、vet、集成测试、WebUI 检查和镜像构建通过。
- [ ] 关键真实 Provider 验收单独记录；未执行/失败/跳过均不得标记通过。
- [ ] 数据库 Schema 和历史数据无非预期变化。
- [ ] 迁移回滚路径经过验证。

## 4. 迁移完成后的公共平台契约接入

迁移完成后，按冻结契约及准入清单实施，不在本迁移文档重新定义字段或语义。建议接入顺序如下，最终以契约依赖 DAG 和准入状态为准：

1. PLAT-CTR-005：受控 HTTP Client，覆盖 Provider 出站网络安全边界。
2. PLAT-CTR-006：Storage/连接池/Migration 生命周期接入。
3. PLAT-CTR-007：Config/Secret 统一装配与启动校验。
4. PLAT-CTR-002：Request Context、Request ID 和后台任务关联。
5. PLAT-CTR-001：Fault 统一映射与安全错误输出。
6. PLAT-CTR-003：Root 入口认证、Principal/Scope 注入和路由授权。
7. PLAT-CTR-004：将既有 Data Probe/Reporter/Freshness/Coverage 等诊断通过 Adapter 接入平台 Runner，不重建 Data 领域诊断。
8. DATA-CTR：按契约登记簿及准入清单逐项完成 DTO、单位精度、质量、PIT、认证和授权接入。

若某项契约在迁移期间已冻结且属于迁移正确性前置条件，可在对应阶段做最小必要适配；必须在进度记录中说明原因、范围和契约版本，不得借机扩展。

## 5. 风险与回滚策略

| 风险 | 控制措施 | 回滚原则 |
|---|---|---|
| Go 版本/依赖冲突 | 先做依赖图审计，分批调整，保留锁定版本记录 | 回退 Module 归并提交，不改业务数据 |
| 内部包越界 | 只暴露 Application Port，静态检查依赖方向 | 恢复 Remote Adapter 调用 |
| DTO 语义丢失 | 迁移前后 golden/contract 对照，nullable 与单位显式测试 | 回退 Adapter，不转换/重写历史数据 |
| DB Pool 双重所有权 | borrowed/owned 明确区分，Close 测试 | 恢复独立 Pool 但不同时启用两套生产写入 |
| Migration 重复执行 | namespace、版本表和显式 Runner | 先停止启动，不手工删除版本记录或表 |
| Redis 行为变化 | key、TTL、DB index、禁用语义对照 | 恢复旧 Cache Adapter |
| 认证边界变化 | Root 入口统一认证，Data Scope/审计继续校验 | 关闭新路由，不开放匿名访问 |
| 部署切换失败 | 保留显式旧 Data 服务部署配置和兼容 Adapter | 回滚 Compose/Nginx，不回滚数据库内容 |

## 6. 进度记录

### 2026-11-06：建立迁移跟踪记录

- 设计决策：已接受方案 B，同进程模块化单体。
- 代码迁移：未开始。
- 本记录创建时分支 HEAD：`d8595f408337f62be3697aac661ba1570fdd3667`。
- 当前仓库树仍同时存在根 `go.mod` 与 `services/market-data/go.mod`，并保留 `cmd/analysis-api`、`cmd/market-data` 两个入口；Data 仍位于独立 `services/market-data` 目录。
- 已确认存在 Root Server Remote Adapter：`internal/adapters/marketdata`。
- M0 仅完成文档/目录层面的初步定位，尚未完成逐文件依赖、资源生命周期和调用点审计；因此 M0 仍为 pending。
- 本次未修改业务代码、数据库、部署配置或契约。

### 进度状态表

| 阶段 | 状态 | 说明 |
|---|---|---|
| M0 迁移前审计 | pending | 已定位主要目录，需逐项核对依赖、调用点、资源和 CI |
| M1 Go Module 归并 | pending | 未开始 |
| M2 Application Port/Bootstrap | pending | 未开始 |
| M3 Root Server 装配 | pending | 未开始 |
| M4 共享资源统一 | pending | 未开始 |
| M5 部署/WebUI 切换 | pending | 未开始 |
| M6 迁移验收 | pending | 未开始 |
| 公共平台契约接入 | blocked_by_migration | 迁移完成后按冻结契约执行 |

### 变更记录模板

后续每轮更新按以下格式追加，避免只写“已完成”而没有证据：

- 日期与代码 SHA：
- 阶段/任务：
- 修改范围：
- 关键设计决定：
- 测试与 CI（必须注明对应 SHA/Run）：
- 未完成项/阻断：
- 回滚点：
- 下一步：

## 7. 开始实施前检查

进入 M0/M1 前必须重新读取当前分支 HEAD 和方案文档。本文创建时的 SHA 仅是记录基线，不是后续写入锁。任何提交都必须以当时最新 HEAD 创建 Tree/Commit，并使用 `expected_sha` 更新分支；发现并发提交时停止写入并重新审计。

### 2026-11-06：M0 第一轮代码与部署审计

审计基线：f2b9e5fc637ea59bff87f5449823e98f746c8428。本轮只读代码与配置，未修改业务实现。

#### 1. Go Module 与命令入口

| Module | 路径 | 声明版本 | 当前职责 | 本次迁移处理 |
|---|---|---:|---|---|
| Root | go.mod | Go 1.22 | 当前 Root Analysis API、Platform、Web/Server 组合基础 | 作为目标 Module；需评估提升到 Go 1.23 |
| Market Data | services/market-data/go.mod | Go 1.23.0 | Provider、标准化、质量、PIT、Repository、HTTP API | M1 归并至 Root Module |
| go-core | go-core/go.mod | Go 1.23 | 独立历史 Go Core 服务及其 Analysis/Transport | 本次明确不并入 Root；保留独立 Module 和历史 go-migration profile |

依赖冲突已确认：Root 使用 github.com/go-sql-driver/mysql v1.9.3，Market Data 使用 v1.8.1；Root 直接依赖 github.com/redis/go-redis/v9 v9.7.3，Market Data 当前没有使用该客户端，而是自定义 RESP Cache Client。Market Data 另有 github.com/injoyai/tdx v0.0.86、golang.org/x/text v0.28.0 等依赖。合并时必须以全仓调用和测试结果决定版本，不应机械保留较低版本。

命令入口：
- cmd/analysis-api/main.go：当前 Root Server 主入口，启动 Analysis Runner、任务队列、HTTP API；通过 internal/adapters/marketdata 访问 Data。
- services/market-data/cmd/market-data/main.go：当前 Data 独立服务入口，负责 Provider 注册、DataService、MariaDB/Redis、Retention Worker、Data HTTP Server 和 BaoStock Client。
- go-core/cmd/analysisd/main.go：独立历史 go-core 入口，不纳入本次迁移。
- 根 cmd/demo、cmd/pythonapi-smoke、cmd/storage-check 是辅助工具入口，需在 M1 后逐项确认是否继续保留根路径，不与 Data Server 入口混为一谈。

#### 2. Root → Data 调用链与 HTTP 边界

- Root 的 internal/adapters/marketdata/client.go 是独立 HTTP Client：读取 MARKET_DATA_BASE_URL、MARKET_DATA_API_KEY，默认地址为 http://127.0.0.1:8091，超时默认 135 秒，向 Data 发送 X-API-Key，并解码 Quote/DailyBar JSON DTO。
- internal/adapters/marketdata/collector.go 实现 Analysis 的 app.DataCollector，通过该 Client 拉取行情、日 K 和指数数据，再映射到 Analysis Domain Snapshot。该适配器是当前 Root 与 Data 的主要耦合点。
- Data 的 services/market-data/internal/httpapi/server.go 直接依赖 *service.DataService，自行注册 /api/v1/... 路由，并在 Handler 外包裹 security headers、request ID、access log、timeout context。迁入后应拆分路由注册与独立 HTTP Server Middleware，避免 Root Router 重复应用入口中间件。
- 当前 Data HTTP DTO 与内部 model 类型存在直接耦合；M2 应明确 Application DTO 边界，不能让 Analysis 直接引用 Data 的内部 Model。
- Root Remote Adapter 当前使用自建 net/http.Client，未使用 internal/platform/httpclient 的受控出站 Client。该差异在本次迁移中先记录；迁移后按冻结 Platform 契约统一 Provider 出站请求，不能将远程 Data Client 与 Provider Client 混为一个调用面。

#### 3. 资源生命周期与迁移影响

| 资源 | Root 当前实现 | Data 当前实现 | 迁移要求/风险 |
|---|---|---|---|
| MariaDB | internal/adapters/storage/mysqlredis.Store 创建并拥有 *sql.DB，配置连接池；Open 内可执行 Root Migration 和 Schema 检查；Close 关闭 Pool | repository.OpenMySQL 另建 *sql.DB，Store.Close 关闭；Data main 单独执行 Migrate | 目标由 Root Bootstrap 创建共享 Pool；Data Repository 接收 borrowed Pool；Data Migration 仍由 Data 提供 Runner、Root 显式执行。需核对两边 Schema、DSN、连接池参数和 Migration 幂等性 |
| Redis | Root 使用共享 go-redis/v9 Client，由 Root Store 创建/关闭 | internal/cache/redis.go 每条命令重新 Dial TCP、AUTH/SELECT、执行后关闭连接 | 需新增基于共享 go-redis Client 的 Data Cache Adapter；保留 DB index、username/password、key namespace、TTL 和禁用缓存语义 |
| Provider HTTP | Data Provider 各自构造请求客户端/传输配置（需继续逐 Provider 核验） | 由 Data Bootstrap 创建和注入 Provider | 迁移后逐 Provider 接入 Platform 受控 HTTP Client；需先盘点所有 Provider Host allowlist 和特殊协议（BaoStock TCP 等） |
| BaoStock | 无对应资源 | Data main 创建 Client，退出时 Close | 由 Data Module 持有，Root 统一调用 Stop/Close |
| Quote Retention Worker | Root Analysis Task Queue 通过 Start/Close 管理 | Data main 调用 runQuoteRetentionWorker 启动 ticker goroutine，ctx 取消后退出，但没有显式 Wait/Stop 完成确认 | 抽取 Worker 生命周期句柄，Start/Stop/Wait 可测试；Root 统一启动和 drain |
| Migration | Root Store.Open 内可触发 Root Migration | Data main 在打开 Repository 后显式调用 Store.Migrate | 禁止把 Data Migration 藏进共享 Pool Open；由组合根按模块顺序注册执行，避免启动时重复迁移 |

#### 4. 构建、部署与 CI

- 默认 docker-compose.yml 当前部署三个容器：stock-analysis-data、stock-analysis-server、stock-analysis-web；Server 依赖 Data healthy，并通过容器 DNS + API Key 访问 Data。目标方案应调整为 Root Server + WebUI，Data 不再是默认独立容器。
- 根 Dockerfile 当前使用 Go 1.22 构建 ./cmd/analysis-api；Data Dockerfile 使用 Go 1.23 构建 ./cmd/market-data。Module 归并后需统一 Builder 版本、依赖下载和单镜像构建上下文。
- docker/docker-compose.go-migration.yml 是历史 Python Bridge + Data + go-core profile；不得误删或误改成当前 Root Server 部署定义。是否保留按非目标约束处理，后续另行决策。
- .github/workflows/go-test.yml 当前矩阵分别测试 .、go-core、services/market-data，并独立运行 Data MariaDB/PIT 集成测试、Compose 校验及两个 Go 镜像构建。归并后应移除 Data 独立 Module 矩阵项，将 Data 测试纳入 Root Module，同时保留 go-core 独立矩阵项。
- .github/workflows/data-quality-parity.yml 曾硬编码旧 doc/api/接口规范/ 路径；本次目录整理后已发现路径失效，需同步修正为 doc/api/provider-migration/。该项属于文档移动引发的 CI 配置修复。
- 2026-11-06 当前 HEAD f2b9e5f 的 Actions：Platform Go checks success；Live Provider probe 外层 workflow success（不代表每项 Provider 数据验收通过）；Go test suite 尚有 Data Module Job in_progress，其 Root、go-core、MariaDB/PIT 和 Compose/Docker Job 已 success；Parity 因报告路径仍指向旧目录而失败；WebUI checks 的单元测试 Job 失败，需单独查看失败断言，不归因于 Data 迁移。

#### 5. 首轮迁移对象清单（目录/文件组级）

| 源路径 | 目标路径建议 | 依赖/风险 | 验证与回滚 |
|---|---|---|---|
| services/market-data/internal/baostock/** | internal/marketdata/provider/baostock/** | BaoStock TCP Client 有独立连接生命周期 | 保留协议/超时/关闭测试；回滚恢复 Data Module |
| services/market-data/internal/provider/** | internal/marketdata/provider/** | 多 Provider、HTTP/TCP 协议、不同 Host 与重试边界 | Provider 全量单测、真实探测分开验收 |
| services/market-data/internal/normalize/** | internal/marketdata/normalize/** | 标准化、单位、复权和质量语义不可变 | 黄金样例、Parity、质量测试；回滚不改数据库数据 |
| services/market-data/internal/validation/**、freshness/** | internal/marketdata/quality/{validation,freshness}/** | 不能与 Platform Diagnostics 混淆领域规则 | 字段级测试、Freshness 测试 |
| services/market-data/internal/model/** | internal/marketdata/domain/model/**（需拆分 HTTP/Application DTO） | 当前 HTTP Handler 直接使用内部 Model | JSON Golden/OpenAPI 双向契约；回滚保留原 DTO |
| services/market-data/internal/service/** | internal/marketdata/app/service/** | DataService 当前是主要用例聚合，字段和构造器依赖较多 | 现有 Service/Repository 集成测试 |
| services/market-data/internal/repository/** | internal/marketdata/repository/** | 需支持 borrowed/owned DB，Migration 拆出 Runner | MariaDB/PIT/重复写入/关闭所有权测试 |
| services/market-data/internal/cache/** | internal/marketdata/cache/** | 自定义 RESP Client 改共享 Redis Adapter | Redis key/TTL/DB index/禁用语义测试 |
| services/market-data/internal/config/**、configs/** | internal/marketdata/config/**、configs/marketdata/** | Provider Secret、旧服务地址/API Key 与进程内模式配置需分离 | 配置兼容测试；回滚恢复旧 env/config |
| services/market-data/internal/httpapi/**、openapi.yaml | internal/marketdata/transport/http/**、internal/marketdata/openapi.yaml | 保留 HTTP DTO/路由；入口 Middleware 不能重复 | OpenAPI、Handler、路由兼容测试 |
| services/market-data/cmd/market-data/main.go | 逻辑提取到 internal/marketdata/bootstrap/**；Root 新入口由 cmd/server 组合 | 入口包含 Provider 注册、资源创建、Worker 和 HTTP Server，不能整文件照搬 | Bootstrap 单测、启动/失败释放/Shutdown 测试；旧入口暂留兼容 |
| internal/adapters/marketdata/** | 保留显式 Remote Adapter；新增 Analysis→Data InProcess Adapter | 当前 Client/Collector 是 HTTP DTO 到 Analysis Snapshot 的映射点 | 同样例 Remote/InProcess 结果一致；可回滚到 Remote Adapter |
| Dockerfile、services/market-data/Dockerfile、docker-compose.yml | 根 Dockerfile、Compose 统一部署 | Go 版本、COPY 上下文、默认服务依赖改变 | Compose config、单镜像构建、旧部署回滚 |
| .github/workflows/go-test.yml、Parity workflow | Root Module 测试矩阵与新文档路径 | 不能丢失 Data race/vet/MariaDB/PIT/Parity 覆盖 | 新旧 CI Job 覆盖矩阵对照 |

#### 6. 当前明确风险与未关闭项

1. Go 版本/依赖冲突（待决策）：Root Go 1.22 与 Data/go-core Go 1.23 不一致；MySQL Driver 1.9.3 与 1.8.1 不一致。进入 M1 前需确认部署 Go 版本、最低兼容性及 Data 全量测试结果。
2. Data Application Port 缺失（M2 工作项）：当前只存在 HTTP Handler 与 DataService；尚无对外稳定 Application Port。不可让 Analysis 直接 import Data internal 子包作为迁移捷径。
3. 双数据库 Pool（M4 工作项）：Root 和 Data 各自创建 MariaDB Pool；迁移后需明确一个共享 Pool owner、Data Repository borrowed semantics。
4. 双 Redis Client/协议差异（M4 工作项）：Root go-redis Pool 与 Data per-command RESP Client 的连接、认证、DB、TTL 行为不同；需要 adapter 和集成测试，不能简单替换类型。
5. Worker 停止确认缺失（M2/M4 工作项）：Retention Worker 目前依赖 context 退出但无 WaitGroup/Stop handle；Root Shutdown 必须等待其退出。
6. HTTP Middleware 重叠（M2/M3 工作项）：Data Handler 已内置 Request ID、Access Log、Timeout 和 Security Headers；挂载到 Root 前要拆分 route handler 与 server middleware。
7. Provider 出站安全适配（M4 后续）：Data Provider 目前尚未统一接入 Platform 受控 HTTP Client；需要逐个 Provider 主机白名单及协议清单。
8. go-core 的独立性（已确认非本次迁移对象）：go-core 继续作为独立 Go Module 和历史 profile，不得因“统一 Module”误将其合并。
9. Parity workflow 路径失效（本轮修复项）：目录移动后必须修正 workflow 的 paths 和报告校验路径。
10. WebUI 检查失败（待定位）：f2b9e5f 对应 WebUI unit tests 失败；需查看具体失败用例，不能假设与文档移动无关，也不能归因于 Data 迁移。

**M0 状态：进行中，未通过。** 已完成 Module/入口、主要调用链、主要资源所有权、默认 Compose 和关键 CI 的第一轮静态核对；尚未完成 WebUI/Nginx 路由、所有 Provider 出站 Host、Data 全量 package/config/testdata 清单、所有 Root 调用点及当前 HEAD CI 收敛复核。未完成前不进入 M1。

### 2026-11-06：M0 第二轮补充（目录规模与 WebUI 代理）

- Data Module 当前共 240 个文件：Provider 93、Service 64、HTTP API 27、Model 11、Repository 9、Validation 8、BaoStock 4、Freshness 4、Cache 3、Config 3，另含命令入口、配置、Migration SQL、OpenAPI、Docker 和 Module 文件。迁移对象不是单一服务文件，M1 必须以完整目录清单和测试资产为单位搬迁。
- WebUI 的 web/default.conf.template 中，/market-api/ 通过 proxy_pass 到 MARKET_DATA_SERVICE_NAME:8091/，Nginx 会剥离 /market-api/ 前缀；/analysis-api/ 同样剥离前缀并代理到 ANALYSIS_SERVICE_NAME:8092/。M5 切换时应将两个 location 指向 Root Server 同一个 upstream，并保留原外部路径及 URI 剥离语义；Root Router 内部注册 Data 与 Analysis 路由时需确认不存在 pattern 冲突。
- 当前 Data Handler 自己处理 X-API-Key；WebUI 通过同源 /market-api/ 访问独立 Data 容器。M5 需要改变的是浏览器凭据边界与代理目标，不是删除 Data 的领域授权语义。Root 入口认证和 Data Scope/授权应按冻结契约组合。
- Data Provider 主体至少可确认 Eastmoney、Tushare 等直接持有 *http.Client 并自行发请求；BaoStock 使用独立 TCP Client。所有 Provider 的 endpoint 环境覆盖、重定向、DNS、请求方法、Body、Host 清单尚未逐项完成，不能据此直接把全部 Provider 统一替换为 Platform HTTP Client。
- Root Store 的配置使用 DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME 与 REDIS_ADDR/REDIS_PASSWORD/REDIS_DB；Data 使用 MARKET_DATA_DB_HOST/PORT/USER/PASSWORD/NAME 等独立字段（旧 MARKET_DATA_MYSQL_DSN 仅兼容），以及 MARKET_DATA_REDIS_ADDR/USERNAME/PASSWORD/DB。迁移时不能只复用 Root Pool 而忽略 Data 独立配置语义；需核对部署实际值并明确统一账号、schema、Redis DB 和 Data key namespace。
- 已修复 .github/workflows/data-quality-parity.yml 中旧 doc/api/接口规范/ 路径，改为 doc/api/provider-migration/。修复提交为 dc137543；该提交之后的 Parity Run 尚需观察，不能引用修复前失败结果作为当前状态。

**M0 仍未通过。** 还需完成：确认 Data 配置的 DSN/Redis 环境变量及实际部署样例；逐 Provider 出站请求/Host/重定向/协议清单；Data 与 Root 所有 API 调用点和 DTO 字段映射清单；Root/Data Migration 完整表清单及版本治理；Retention Worker 生命周期测试设计；所有 Data 配置、Migration、testdata 和生成/工具文件的逐项迁移登记；当前 HEAD CI 复核。

### 2026-11-06：M0 第三轮补充（存储配置与迁移开关）

已核对 services/market-data/internal/config/config.go、根 .env.example、Data .env.example 与默认 docker-compose.yml：

- Data 数据库配置优先使用 MARKET_DATA_DB_HOST/PORT/USER/PASSWORD/NAME/CHARSET/TIMEZONE；MARKET_DATA_MYSQL_DSN 仅作为兼容旧配置。Redis 使用 MARKET_DATA_REDIS_ADDR/USERNAME/PASSWORD/DB，支持 username；Data 默认 MARKET_DATA_DB_AUTO_MIGRATE=true。
- Root 使用 DB_HOST/PORT/USER/PASSWORD/NAME、DB_PARAMS、REDIS_ADDR/PASSWORD/DB；Root Redis Client 当前未配置 Username。Root DB_AUTO_MIGRATE 在配置默认值为 false，但仓库 .env.example 显式设置 true。
- 根 .env.example 的 Root 与 Data 示例数据库名都为 stock_analysis_server，Redis DB 示例都为 0，但账号/密码配置项独立。不能据此认定生产环境密码相同；迁移前必须核对实际部署配置并决定统一 Secret/连接配置映射。
- 默认 Compose 将根 .env 同时注入 Data 与 Server；Data Redis 地址/密码/DB 和 AutoMigrate 另由 Compose 显式传入。Data DB Host/User/Password/Name 仍从同一个 .env 中的 MARKET_DATA_DB_* 读取。
- Data config.Load 当前即使只走 InProcess 也会校验 MARKET_DATA_API_KEY；M3/M5 应将领域配置与 HTTP Adapter 认证配置拆开，InProcess 模式不应因为不再自调用 HTTP 而被迫保留内部 API Key；若保留独立 Data HTTP Adapter，则该 Adapter 仍需按契约认证。
- Data Migration 当前由独立 repository.Store.Migrate 执行，默认自动迁移开启；Root mysqlredis.Open 可按 DB_AUTO_MIGRATE 执行 Analysis Migration，并随后检查 analysis_runs、analysis_stage_events、analysis_snapshots、analysis_tasks。两套 Migration 作用于同一示例数据库但拥有不同表，归并后应保留 Data/Analysis 各自 Migration Owner，由 Root 统一显式编排，避免重复执行或把 Data Migration 混入通用 Pool Open。

本轮不改变代码配置和数据库行为，只补充审计证据。M0 仍需完成逐 Provider 请求边界、调用点/DTO 全量清单、Schema/Migration 对照与当前 HEAD CI 收敛。

### 2026-11-06：M0 第四轮补充（生产调用链与 Provider 出站协议）

#### 1. Root 生产调用链已定位

cmd/analysis-api/main.go → internal/service.NewRunnerWithMarketDataStores → internal/adapters/marketdata.NewClient → NewCollector → analysis/app.DataCollector → Analysis Runner。该路径当前没有 Python fallback；Collector 通过远程 HTTP API 获取 Quote/DailyBar/Index 数据，再转换为 Analysis Domain RawSnapshot。M3 应替换的是 Collector 的数据访问 Adapter，而不是重写 Analysis Runner、Normalizer 或策略流程。

#### 2. Provider 出站协议首轮矩阵

| Provider | 默认 HTTP Host / 协议 | 关键边界 |
|---|---|---|
| Eastmoney | push2.eastmoney.com、push2his.eastmoney.com、push2ex.eastmoney.com、datacenter.eastmoney.com、datacenter-web.eastmoney.com、np-anotice-stock.eastmoney.com、emappdata.eastmoney.com | 多端点；部分请求自定义重定向策略；Endpoint 可通过配置覆盖，需按实际 URL Host 动态校验 |
| Tencent | web.sqt.gtimg.cn、web.ifzq.gtimg.cn、smartbox.gtimg.cn | Quote/Kline/Search 多 Host；请求 Header 与响应编码有差异 |
| Sina | hq.sinajs.cn、money.finance.sina.com.cn | Quote/Kline 两 Host；当前明确禁止重定向，响应存在 GBK/JSONP 处理 |
| NetEase | api.money.126.net、quotes.money.163.com | Quote JSONP + History CSV；当前明确禁止重定向，涉及 GBK/CSV 解码 |
| Tushare | api.tushare.pro | POST JSON、Token、请求节流；需保留 Body 重放/重试语义和脱敏 |
| Hithink | fuyao.aicubes.cn | X-api-key Header；默认关闭；REST 多能力 |
| TickFlow | api.tickflow.org、free-api.tickflow.org | 授权与免费端点分离；POST/GET 混用、速率限制、API Key Header |
| Yahoo | query1.finance.yahoo.com | 海外 Provider，默认关闭；不属于当前国内 A 股核心链 |
| Xueqiu | stock.xueqiu.com | Cookie 登录态，默认关闭；敏感凭据需脱敏 |
| MiaoXiang | mkapi2.dfcfs.com | 可选补充接口，默认关闭；Key 与响应字段有独立约束 |
| BaoStock | public-api.baostock.com:10030 TCP | 原生 TCP 会话，需保留连接/登录/关闭和请求超时，不适用 HTTP Client |
| Pytdx | 通达信 TCP Host 列表（配置或 github.com/injoyai/tdx 内置 Hosts），默认端口 7709 | 轮转尝试有限 Host，TCP 协议；不能套用 HTTP DestinationPolicy |

以上 Host 来自当前配置默认值与 Provider 实现的首轮核对；Endpoint 环境覆盖仍可能改变最终 Host。进入 M4 前应形成配置解析后的 Host allowlist 和 Provider × 请求能力清单，并为重定向目标单独登记；不得仅按 Provider 名称写死单 Host。

#### 3. Platform HTTP Client 适配限制

internal/platform/httpclient.Client 使用 DestinationPolicy 精确 Host/Port allowlist、DNS 公网地址校验、默认不读取系统代理、响应体大小限制、依赖级并发和可控重试。Data Provider 当前多数自建 *http.Client；其 redirect 策略不一致（例如 Sina/NetEase 明确禁止、Eastmoney 有专用策略，其他实现需逐项核对）。M4 不能直接把 *http.Client 全局替换为 Platform Client：需提供 Data Provider 的受控传输 Adapter，保留每个接口的方法、Header、Body、超时、重定向、速率限制、响应大小与错误分类语义。BaoStock/Pytdx 继续走独立 TCP Client，不纳入 HTTP Adapter。

**M0 进度判断：** Root→Data 主生产调用链和主要 Provider 协议类别已定位；仍未对 30 个 Provider 源文件逐个核对所有请求方法/重定向/Body/错误语义，也未完成全量 API DTO 字段映射、Data Migration SQL 与实际部署库的对照。M0 继续保持进行中。
