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

状态：`in_progress`

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
| M0 迁移前审计 | in_progress | 已完成多轮静态审计，逐 Provider、DTO、Migration 与 CI 收敛仍未完成 |
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

### 2026-11-06：M0 第五轮补充（Application Port、DTO 与 Migration 资产）

- Analysis 当前已有 internal/analysis/app.DataCollector 接口，但这是 Analysis 消费数据的 Port，不是 Data 模块对外 Application Port。当前实现 internal/adapters/marketdata.Collector 通过 HTTP Client 实现该接口。M2 需要在 Data 侧新增独立 Application Port/UseCase，再由 Root 新建 InProcess Adapter 实现 Analysis 的 DataCollector；不能把现有 Analysis Port 误认为 Data Port 已存在。
- Data HTTP Handler 当前直接输出 internal/model 的 Quote、DailyBar、Bundle 等类型；Root HTTP Adapter 自己定义 QuoteResponse、HistoryBar、apiHistory DTO。两侧结构并不完全一致：Data Quote 的 current_price 为 nullable pointer、volume 为 int64，Root QuoteResponse 的 current_price 为非 nullable float64、volume 为 *float64；Data DailyBar OHLC 为非 nullable float64，并包含 MA、normalized_at、system_observed_at、raw unit、fallback、normalization version 等字段，Root HistoryBar 只保留部分字段。M2 Application DTO 必须以 Data 领域语义为基准，并由 InProcess Adapter 映射到 Analysis Domain；不要将现有 HTTP DTO 直接当作内部 Port。
- Analysis RawSnapshot/SourceNote 已承载 Source、FetchedAt、QualityStatus、Diagnostic、MissingFields 等信息；迁移前后应对照 RawSnapshot golden，确认 InProcess Adapter 不丢失 Provider 来源、版本、时间、质量和缺失字段语义。
- Data 的 HTTP Handler 注册 /api/v1/quotes/{symbol}、/api/v1/bars/{symbol}/daily、/api/v1/bundles/{symbol}、/api/v1/market/**、/api/v1/stocks/**、/api/v1/data-sources/** 等路由；Root Analysis API 当前使用 /api/v1/analysis/**。Nginx 对 /market-api/ 与 /analysis-api/ 均剥离前缀，因此 Root Router 可以注册对应路由组，但必须做最终 pattern 冲突测试，并保留外部 URI 行为。
- Data 当前存在 services/market-data/migrations/001_create_market_data.sql，同时 repository/mysql.go 内还有运行时硬编码 DDL、ensureColumn 和 ensureNonUniqueHistoryIndex；Root Migration 则位于 internal/adapters/storage/mysqlredis.Store.Migrate 的代码语句中。M0 必须核对 SQL 文件与运行时 DDL 是否一致，确认是否存在只在历史部署升级时执行的增量迁移，再设计统一 Migration Runner；不能简单把 SQL 文件移动后就认定迁移资产完整。

**M0 当前仍有未完成审计：** 需要检查 Data 全部 DTO 与 OpenAPI 双向差异，分析 RawSnapshot 映射细节，核对 SQL Migration 文件与运行时 DDL 的差异，盘点各 Provider 所有请求参数/重定向与认证方式，并查看 WebUI unit test 失败日志。当前仅形成了迁移方案和第一轮静态清单，不具备进入 M1 的准入条件。

### 2026-11-06：M0 第六轮补充（DDL 等价、DTO 映射与目录回归）

#### 1. Migration DDL 对账结果

已将 migrations/001_create_market_data.sql 与 repository/mysql.go 的 Migrate() 内 9 个 CREATE TABLE 语句逐表规范化比较：9/9 表完全一致，未发现列、索引或约束差异。两者表清单均为 market_data_quotes、market_data_daily_bars、market_data_daily_bar_versions、market_data_fundamental_versions、market_data_security_trading_status_versions、market_data_sentiment_snapshots、market_data_provider_probe_runs、market_data_adjustment_factor_versions、market_data_corporate_action_versions。

但运行时 Migrate() 还包含增量升级逻辑，SQL 文件没有表达：
- 检查并删除历史 market_data_daily_bar_versions.uq_market_bar_revision_content 唯一索引，确保 A→B→A 内容回归修订可以追加；再创建 idx_market_bar_revision_identity_asof 普通索引。
- 检查并删除历史 market_data_fundamental_versions.uq_market_fundamental_revision_content 唯一索引，再创建 idx_market_fundamental_identity_asof 普通索引。
- 检查 market_data_quotes.volume_ratio、turnover_rate 列是否存在，不存在则 ALTER TABLE 增加。

结论：DDL 当前不存在双份定义漂移，但 SQL 文件只是基础建表快照，不是完整的可执行升级历史。M2/M4 应把基础 Schema 与历史升级步骤拆成可版本化 Migration，并对已部署库做 migration version/dirty state 管理；迁移期间必须保留现有信息架构检查和索引纠正行为。禁止只执行 001 SQL 取代当前 Migrate()。

#### 2. Data HTTP DTO → Analysis RawSnapshot 映射

- Data HTTP API 直接 JSON 输出 internal/model Quote、DailyBar；Root adapter 再解码为 QuoteResponse、HistoryBar。Analysis app.DataCollector 是消费侧 Port，不是 Data Application Port。
- Quote：Data Quote 的 current_price 是 *float64，Root QuoteResponse 却是 float64；Root Client 会拒绝非正数/非有限价格，但若 Data 返回 null，JSON 解码到 float64 会变成 0，再以 missing/non-positive 错误处理。该行为虽不静默制造有效价格，但丢失了“原始 null”与“明确为 0”的差异。Quote 的 fetched_at、quote_time、source 和部分价格/成交量字段被映射；source_version、normalization_version、raw_volume/raw_amount 及单位原始字段、fallback 信息没有完整传入 RawQuote。
- DailyBar：Root HistoryBar 仅保留 trade_date、OHLC、volume、amount、change_percent、source/source_version、provider_timestamp、fetched_at、adjustment_basis、quality_status/reasons、volume_unit、amount_unit。Data DailyBar 另有 MA5/20/120/250、adjustment_method/reference_date/factor source/factor source version/factor system observed time、normalized_at、system_observed_at、raw volume/amount/unit/field、fallback、request ID、normalization version 等字段，当前 HistoryBar 未保留。
- Collector convertBars 额外执行 OHLC 正数/有限/区间关系、成交量/额非负、日期唯一、as_of 截止和当日 K 线可用时点校验；缺失 volume/amount 保持 nil。它会把 adjustment_basis 缺失映射成 unknown，并把 available_at 记为缺失，因为现有 Data API 没有精确披露供应商发布时间。
- Quote Collector 需要有效 quote_time，过滤未来时间/晚于显式 as_of 的报价；缺失可选字段会写入 SourceNote.MissingFields。RawSnapshot 当前不携带 Data 的 QualityReasons、FallbackFrom/Reason、NormalizationVer、SourceVersion 和 raw-unit 元数据，诊断只在 SourceNote 中保留部分摘要。

迁移设计约束：InProcess Adapter 应直接映射 Data Application DTO 到 Analysis RawSnapshot，不能复用 HTTP DTO 作为内部契约；先定义字段保留策略，明确 RawSnapshot 当前确实不承载的元数据是被有意降级还是需要扩展。对历史 Analysis 输入兼容性，必须使用 Remote 与 InProcess 同输入 golden 对比，不能把“结构字段更多”直接等同于“分析结果等价”。

#### 3. WebUI 目录移动回归修复

- WebUI checks 在 977bf59c 失败的唯一单测为 web/tests/ui-contract.test.mjs：读取 doc/api/分析任务.openapi.yaml 时 ENOENT。
- 已修复为 doc/api/contracts/分析任务.openapi.yaml，提交 dd36d129。该提交对应的 WebUI checks 已 success；修复仅调整测试文件引用路径，不改 WebUI 运行时代码或 API 行为。
- data-quality-parity workflow 中旧 doc/api/接口规范/ 引用也已在 dc137543 修正为 doc/api/provider-migration/。

#### 4. M0 进度更新

已完成：DDL 基础定义逐表一致性比对；运行时增量升级步骤盘点；Analysis Collector 主要字段映射与时间/质量过滤逻辑审计；目录移动导致的 WebUI 单测失败定位和修复。

仍未完成：逐 Provider 请求语义完整矩阵（含所有 endpoint override、重定向目标、超时、重试、响应大小）；完整 HTTP DTO/OpenAPI 字段级校验；Root/Data Migration 的 schema owner 和真实部署库版本治理；配置、测试、fixtures、生成文件逐项迁移清单；当前最新 HEAD 的全部 CI 完成复核。

**M0 状态继续保持 in_progress。** 本轮未改业务实现、Provider 算法、数据库结构或历史数据；尚不满足 M1 准入。

### 2026-11-06：M0 第七轮补充（CI 收敛与 Provider 源文件覆盖）

#### 1. Provider 源文件盘点覆盖

已对 services/market-data/internal/provider/ 下 30 个非测试 Go 源文件进行首轮逐文件扫描，覆盖 Eastmoney（含公告、资金流、龙虎榜、板块扩展）、Tencent（含搜索）、Sina、NetEase、Tushare（含复权因子、能力、公司行动、资金流、交易状态）、TickFlow、Hithink、MiaoXiang、Xueqiu、Yahoo、BaoStock、Pytdx，以及 Provider 聚合/诊断/跨源比较代码。

协议类别已确认：HTTP Provider 涵盖 GET 与 POST；Tushare、MiaoXiang、TickFlow 有 POST 请求；Eastmoney 混用 GET/POST；Sina/NetEase/Yahoo 对重定向有显式策略；Xueqiu 使用 Cookie；Hithink 使用 X-api-key；TickFlow 使用授权与免费 API 分离；BaoStock/Pytdx 是 TCP，不能接入 HTTP DestinationPolicy。

该扫描确认了主要 Host 与协议类型，但尚未将每个调用点的 query/body、headers、超时、retry/backoff、redirect target、response size、Endpoint override 逐条形成表格。因此本轮只视为 Provider 源文件覆盖完成，不代表 HTTP Client 迁移适配设计已完成。M4 前仍需完成请求级矩阵。

#### 2. 最新可用 CI 证据

- dd36d129（修复 WebUI 契约文档路径）对应 WebUI checks：success；Platform Go checks：success；Live Provider probe workflow：success。
- 同一提交的 Go test suite：Root Test . success；Market-data MariaDB migration/PIT persistence success；go-core tests success；services/market-data 测试 Job 后续 cancelled；Compose/Docker Job 仍 in_progress 后被新提交触发的并发策略中断，未获得完整 Go suite 结论。
- 同一提交的 Go-Python parity Job 后续 cancelled，不能视为成功或失败。
- 同一提交的 WebUI browser smoke 失败：320px viewport 检查在 Run Flow 弹窗测试期间记录到 12 次 GET / 的 net::ERR_ABORTED，被 browser-smoke.mjs:2062 判定为 unexpected non-API network failures。该失败发生在 WebUI unit checks 已通过之后，暂未证明与 API 文档目录移动有关；需要独立分析浏览器导航/重载与测试监听器，不在本轮无证据地放宽断言。
- f19b556 是仅文档更新的 HEAD；上述 CI 证据来自最近一次含代码变更的 dd36d129，不能冒充 f19b556 的 checks。

#### 3. 目录结构引用复核

- 已修复 web/tests/ui-contract.test.mjs 对旧 OpenAPI 路径的引用，当前改为 doc/api/contracts/分析任务.openapi.yaml。
- 已修复 data-quality-parity workflow 的旧 doc/api/接口规范/路径，改为 doc/api/provider-migration/。
- GitHub Code Search 对上述两个旧路径字符串未返回残留命中；本次不据此宣称所有 Markdown 相对链接全部有效，文档内部链接仍需完整巡检。

#### 4. M0 结论

本轮完成 Provider 非测试源文件首轮覆盖、Data Migration DDL 与运行时升级行为对账、Analysis DTO 映射损失记录，以及文档移动导致的 WebUI 单测失败修复。仍有三项实质性准入工作未完成：

1. 完成逐 Provider 请求调用点级矩阵与可配置 Endpoint 的最终 Host 约束；
2. 完成 OpenAPI/HTTP DTO 的字段级契约差异核验，以及 Data→Analysis 字段保留/降级清单；
3. 明确 Data/Analysis Migration 的版本治理和共享数据库真实部署配置，并获得最新完整 Go test、Parity、Compose/Docker 验收结果；另需定位 WebUI browser smoke 的 320px ERR_ABORTED。

**M0 状态：in_progress。** 当前不开始 M1。先关闭上述准入项，避免 Module 归并后才发现 Provider 请求行为、PIT 元数据或数据库升级路径不兼容。

### 2026-11-06：M0 第八轮补充（OpenAPI 路由完整性）

已将 services/market-data/openapi.yaml 的 Operation（HTTP Method + Path）与 services/market-data/internal/httpapi/server.go 的 mux 注册逐项比对：35/35 完全匹配，OpenAPI 未声明而 Server 暴露的路由 0 个，OpenAPI 声明但 Server 缺失的路由 0 个。该结果证明当前路径/方法层一致，不代表 Request/Response Schema、字段 required/nullability、错误响应、鉴权 Header 和示例值也完全一致；后者仍需独立字段级核验。


### 2026-11-06：M0 第九轮补充（Data Application 能力面与 Worker 生命周期）

- Data HTTP Server 当前注册 35 个 Method+Path Operation，OpenAPI 也为 35 个 Operation，已完成路径/方法双向核对。其 Handler 分布覆盖行情/日 K/Bundle、批量行情、Provider Probe/Diagnostics、市场宽度/情绪/资金流/板块/指数/涨跌停/龙虎榜/交易日历/交易时段，以及证券搜索、公告、Profile、交易状态、价格限制、财务、筹码分布、个股资金流、Boards。
- DataService 是一个聚合型用例对象，除 Quote/DailyBars/Bundle 外，还通过多个 setter 注入 StockBasic/Search、SectorRanking、IndexQuote、MarketBreadth、LimitPool、DragonTiger、StockBoards、TradingCalendar、SecurityTradingStatus、PriceLimitEvent、Fundamentals、Valuation、IndexCatalog、ChipDistribution、CapitalFlow、MarketCapitalFlow、HotRanking、Supplement 等 Provider capability。M2 Application Port 不能只覆盖当前 Analysis Collector 用到的 Quote/DailyBars，否则会导致 WebUI 的其余 Data API 在合并后无调用入口。
- 当前 Analysis app.DataCollector 仅消费 Quote、DailyBars、IndexBars；它不需要把 Data 的全部 35 个 HTTP Operation 暴露为 Analysis Port。建议拆分 Data Application API 为按领域聚合的用例接口（行情/证券主数据/市场统计/研究补充/诊断），Analysis InProcess Adapter 只实现 Analysis 所需的采集 Port；HTTP Adapter 继续调用完整 Data Application API。不要创建一个把全部 Provider 或 Repository 暴露给 Root 的巨大接口。
- Retention Worker 的实际实现为 runQuoteRetentionWorker：内部启动匿名 goroutine，首次立即执行清理，之后每 24 小时执行一次；使用 30 秒子 Context，但没有返回 Stop Handle、WaitGroup 或 done channel。Root main 当前只管理 Analysis Task Queue 的 Start/Close，并通过 HTTP Server Shutdown 等待服务停止；Data Worker 无法被 Root Shutdown 显式等待。M2 应将它改成具备 Start/Stop/Wait 的生命周期组件，并确保先停止 Worker，再关闭 Data Repository/共享 DB。
- Root Store.Close 当前关闭 Root DB 和 Redis；Data Store.Close 关闭自己创建的 DB。Data Redis Cache 没有 Close 接口，每条命令 Dial、AUTH/SELECT、执行并关闭连接。统一资源时，Redis Cache Adapter 应借用 Root Redis Client，不应让 Data Module 再关闭 Root Redis Client。



### 2026-11-06：M0 第十轮补充（Provider 请求语义矩阵初稿）

本轮基于当前 Provider 源码与 config.Config 的实际构造路径，补充请求级矩阵。此处记录的是迁移前行为基线，不是 M4 的目标安全策略；具体请求的 query 字段和 JSON Body 字段仍需在后续逐调用点补齐。

#### 1. Provider 请求矩阵（第一批）

| Provider/能力 | 协议与认证 | 默认超时/请求约束 | 重试与重定向 | 响应限制/备注 |
|---|---|---|---|---|
| Eastmoney 行情、日 K、证券目录、板块、指数、市场宽度、涨停池、龙虎榜、公告、个股资金流 | HTTP GET；行情榜单等个别能力另有 POST JSON；公开 Web 参数，无私有 Key；UT 为可覆盖的前端公共参数 | Provider 默认 12s；Quote/Kline 等调用点常使用 6s/8s attempt timeout；Market turnover 特殊路径 60s | GET 通用 retry 最多 3 次，仅对 429/部分 5xx、timeout、连接类错误重试，退避 700ms/1400ms；部分语义不完整响应另有一次补偿重试；仅 HTTPS 且目标 Host 为 *.eastmoney.com 的重定向可跟随，最多 3 跳 | 通用 JSON 通过 readBoundedBody(maxProviderPayloadBytes)；错误响应最多读取 4096 bytes；热榜 POST 同样有重试与 bounded body |
| Tencent 行情、日 K、证券搜索 | HTTP GET；无 Key；查询参数位于 URL Query | 默认 12s | 禁止重定向（返回 http.ErrUseLastResponse）；未发现统一请求重试 | maxProviderPayloadBytes；搜索结果需解码 GBK/解析 v_hint |
| Sina 行情、日 K | HTTP GET；无 Key；行情代码拼接在 quote URL，日 K 使用 Query | 默认 12s | 禁止重定向；未发现统一请求重试 | 单次响应上限 1 MiB；超限显式失败 |
| NetEase 行情、历史 CSV | HTTP GET；无 Key；证券代码拼接在行情 URL，历史请求使用完整配置 URL | 默认 12s | 禁止重定向；未发现统一请求重试 | 行情响应上限 1 MiB；历史 CSV 使用 maxProviderPayloadBytes；非 2xx 错误体最多读取 4096 bytes |
| Yahoo Finance | HTTP GET；无 Key；Ticker 拼入 Chart URL，Query 传入区间等参数 | 默认 12s | 禁止重定向；未发现统一请求重试 | maxProviderPayloadBytes；公开非 SLA 数据源，默认可选启用 |
| Tushare | HTTP POST JSON；Token 位于 JSON Body（不是 Header/Query） | 默认 12s | 默认 Go Client 重定向行为；未发现 Provider 内部重试 | 非 2xx 错误体最多读取 4096 bytes；响应读取需纳入统一 bounded body 核验 |
| Hithink | HTTP GET；X-api-key Header | 默认 15s | 默认 Go Client 重定向行为；未发现 Provider 内部重试 | 走统一 JSON 请求封装；需核对响应读取是否所有路径均有上限 |
| TickFlow 授权 API | HTTP GET/POST；x-api-key Header | 默认 20s；授权与免费 API 使用不同 Base URL | 默认 Go Client 重定向行为；未发现 Provider 内部重试 | 成功响应 maxProviderPayloadBytes；错误响应最多 2048 bytes，且会对 Key 脱敏 |
| TickFlow 免费日 K | HTTP GET；无 Key，走独立 free Base URL | 默认 20s；调用间隔至少 1s | 同上 | 同上；不能将免费 Base URL 与授权 Host 混为同一信任域 |
| MiaoXiang | HTTP POST JSON；apikey Header | 默认 10s；进程级全局互斥，非排队准入；两次请求至少间隔约 300ms | 禁止重定向；未发现重试 | 响应上限 2 MiB；错误响应最多读取 4096 bytes；日志不输出 Key |
| Xueqiu | HTTP GET；可选 Cookie Header，同时发送 Referer/Origin | 默认 10s | 默认 Go Client 重定向行为；未发现 Provider 内部重试 | maxProviderPayloadBytes；Cookie 仅保存在内存，错误和日志不得泄漏 |
| BaoStock | 自定义 TCP 协议 | Connect 默认 5s；Request 默认 15s；请求内部另有 4s Context 限制 | 非 HTTP redirect/retry；单连接请求串行，显式 Login/Logout | 不适用 HTTP DestinationPolicy；Host 默认 public-api.baostock.com，端口由客户端协议配置 |
| Pytdx | TDX TCP 协议 | Provider timeout 默认 12s；单次 Dial 最多 1.5s | 使用 TDX Client WithRedial()；轮换候选 Host | 不适用 HTTP DestinationPolicy；需独立审计 Host 列表来源、端口及重连次数 |

#### 2. Endpoint 覆盖与 Host 约束现状

internal/config/config.go 当前对配置化 HTTP Endpoint 调用 validateEndpointURL，校验项为：
- 必须是绝对 URL，Scheme 仅允许 http/https；
- 必须有 Host；
- 禁止 URL UserInfo、Query、ForceQuery 和 Fragment。

当前没有校验 Host 是否属于对应 Provider 的官方域名白名单，也没有统一的 endpoint override 授权/风险分级。Eastmoney 的 redirect policy 只约束跳转目标，不约束初始 endpoint；因此将自定义 Eastmoney URL 配置为非 Eastmoney Host 时，首跳仍可能访问该 Host。其他采用默认 Go Redirect Policy 的 Provider 也可能跟随跨 Host 跳转；若请求携带 Tushare Token Body、Hithink/TickFlow/MiaoXiang Key Header 或 Xueqiu Cookie，必须验证重定向时敏感信息不会被转发至非预期域名。

当前配置化 URL 环境变量包括：
- Eastmoney：MARKET_DATA_EASTMONEY_*_URL（Quote、Kline、Stock Basic、Sector Ranking、Index、Market Stocks、Limit Pool Base、Stock Boards、Dragon Tiger、Announcements、Hot Rankings、Hot Surge、Capital Flow）。
- Tencent：MARKET_DATA_TENCENT_*_URL（Quote、Kline、None Kline、Stock Search）。
- Sina：MARKET_DATA_SINA_*_URL；NetEase：MARKET_DATA_NETEASE_*_URL；Yahoo：MARKET_DATA_YAHOO_CHART_URL。
- Tushare：MARKET_DATA_TUSHARE_API_URL；Hithink：MARKET_DATA_HITHINK_BASE_URL；TickFlow：MARKET_DATA_TICKFLOW_API_URL 与 MARKET_DATA_TICKFLOW_FREE_API_URL；Xueqiu：MARKET_DATA_XUEQIU_BASE_URL；MiaoXiang：MARKET_DATA_MX_API_URL。
- BaoStock/Pytdx 为 TCP Host 配置，必须与 HTTP Endpoint 策略分开治理。

#### 3. M4 受控 HTTP Client 接入约束（审计结论，不改代码）

1. 先建立 Provider→默认 Host 集合、Endpoint override 模式（仅默认 Host / 显式允许自定义 Host / 禁止自定义 Host）和 Redirect Host 集合；不能仅以 URL Scheme 校验代替出站目标约束。
2. 认证信息必须绑定到 Provider 预期 Host；跨 Host 重定向默认禁止。若某 Provider 必须跳转，需显式列出目标域名，并测试 Authorization/Cookie/自定义 Key Header/Body Token 不会泄漏。
3. 保留每个调用点的 GET/POST、Query/Body、Header、超时、重试次数和退避、速率控制、响应上限及错误体截断行为；不能将 Eastmoney 重试泛化到所有 Provider。
4. Eastmoney 的 push2.eastmoney.com、push2his.eastmoney.com、push2ex.eastmoney.com、push2delay.eastmoney.com 等子域属于明确的重定向/服务域名集合；最终 allowlist 应基于实际配置和 live probe 记录核实，不应简单放行所有任意域名。
5. BaoStock/Pytdx 使用独立 TCP Adapter；公共平台若没有 TCP 目标控制契约，必须单独定义边界或记录为平台能力阻断，不能伪装成 HTTP Client。

#### 4. 本轮未完成项

- 尚未逐一列出每个 Eastmoney/Tencent/Hithink/TickFlow 业务调用的完整 Query 字段和 Body Schema；本表按请求模式归并，不替代逐调用点清单。
- 尚未完成所有 Provider 对 http.Client 默认 Redirect Policy 的敏感信息跨域转发测试。
- 尚未对 Hithink、Tushare、Tencent Search 等每个独立请求入口的响应大小限制逐条确认。
- 尚未将 Endpoint override 改造为 Host allowlist；本轮仅审计并记录，不提前修改 Provider 行为。

**M0 状态继续保持 in_progress。** 请求级矩阵已从“源码文件覆盖”推进到“主要 Provider 请求模式和安全边界初稿”，仍未达到逐调用点完整度，不能据此进入 M1。


### 2026-11-06：M0 第十一轮补充（OpenAPI Schema、HTTP DTO 与 PIT 调用链）

#### 1. Data Go Model 与 OpenAPI Schema（Quote/DailyBar）

对 services/market-data/internal/model/model.go 的 Quote、DailyBar 字段与 services/market-data/openapi.yaml 对应 Schema 做了首轮核对：
- Quote 的 nullable 数值字段、Volume int64、QuoteTime nullable、FetchedAt/NormalizedAt、Source/SourceVersion、原始单位字段、质量/回退/RequestID/NormalizationVersion 等字段均在 OpenAPI Schema 中有对应定义；当前未发现这两个结构的核心字段在 OpenAPI 中整体缺失。
- DailyBar 的 OHLC、MA5/20/120/250、nullable Volume/Amount/ChangePercent、Adjustment Basis/Method/Reference Date/Factor Source/Version/SystemObservedAt、ProviderTimestamp、FetchedAt/NormalizedAt、SystemObservedAt、RawVolume/RawAmount/Unit/Field、Quality/Fallback/RequestID/NormalizationVersion 均在 OpenAPI Schema 中有对应定义。
- OpenAPI Quote/DailyBar 将 nullable 字段标为 nullable，并对必填字段与 omitempty 可选字段做了区分；本轮静态核对未发现明显的 Go JSON tag 与 Schema 属性命名不一致。
- 此结果只覆盖 Quote、DailyBar 两个核心 DTO，不代表 35 个 Operation 的全部 Request/Response Schema、required/nullability、错误响应、认证 Header 已完成全量验证。

#### 2. Data HTTP DTO → Root Remote Adapter DTO 差异

Root 的 internal/adapters/marketdata/types.go 并不是 Data HTTP Model 的无损映射：
- Quote：Data Quote.current_price 是 *float64，Root QuoteResponse.CurrentPrice 是 float64；Data volume 是 *int64，Root 是 *float64。JSON null 会把 CurrentPrice 解码成 0，随后被 Client 作为 missing/non-positive 拒绝，虽然没有静默作为有效价格使用，但丢失了原始 null 语义；int64 volume 转 float64 也改变了数值类型及大整数精度边界。
- Quote：Root DTO 没有接收 normalized_at、quote_time_semantics、volume_ratio、turnover_rate、raw_volume/raw_amount/raw units/raw field、fallback_from/reason、request_id、normalization_version。source_version、quality_status/reasons 已接收。
- DailyBar：Root HistoryBar 的 OHLC 使用 *float64，而 Data DailyBar 为非 nullable float64；Volume 使用 *float64，而 Data 为 *int64。其余已接收 trade_date、OHLC、volume、amount、change_percent、source/source_version、provider_timestamp、fetched_at、adjustment_basis、quality_status/reasons、volume_unit、amount_unit。
- DailyBar 未被 Root HistoryBar 接收的字段包括 period/is_partial、MA5/20/120/250、adjustment_method/reference_date/factor source/factor source version/factor system observed time、normalized_at、system_observed_at、raw volume/amount/unit/field、fallback、request_id、normalization_version。
- DailyBarsResponse 的 point_in_time 与 as_of 也未被 Root apiHistory 接收；Root HistoryResult 同样没有这两个状态字段。

#### 3. 当前 Analysis Collector 的 PIT 风险（新增高优先级发现）

Data OpenAPI 对 /api/v1/bars/{symbol}/daily 明确区分：
- 不传 as_of：返回当前 Provider 选出的序列；
- 传 as_of：只从系统在该时间点之前记录的版本中重建历史序列，不调用实时 Provider，也不回退当前值表。

但当前 Root Client.GetHistory 只发送 limit 和 adjustment，没有发送 as_of；Root Analysis Collector 虽然按 RunRequest.AsOf 过滤未来日期/当日尚未完成的日 K，却无法保证已存在历史交易日使用的是 as_of 时刻已观察到的版本。即：过滤交易日期晚于 as_of，不等于 PIT 版本重建。

迁移 M2/M3 必须将明确的 PIT 语义纳入 Data Application Port 请求模型：历史分析请求必须携带 AsOf cutoff，并由 Data Application 层执行版本重建；不能只把当前日 K 列表交给 Analysis 后再做日期过滤。InProcess 与 Remote Adapter golden 对比时必须覆盖“同一交易日 A→B→A 修订、as_of 位于不同修订观察时间之间”的场景，并断言 point_in_time=true 和 system_observed_at 不晚于 cutoff。若当前 Analysis RunRequest 的 AsOf 语义不适合直接作为 Data revision cutoff，需要先明确两种时间语义的映射规则，禁止默默混用。

#### 4. 本轮结论与剩余项

- Quote/DailyBar 的 Data Go Model 与 OpenAPI Schema 核心字段首轮一致；主要 DTO 损失发生在 Root Remote Adapter，而不是 Data OpenAPI 定义。
- 当前 Remote Adapter 的 Quote 类型存在 nullable 与 int64→float64 类型降级；DailyBar 丢失 MA、复权因子、PIT、标准化、原始单位及 fallback 元数据。
- 发现当前 Analysis Collector 未传递 Data API 支持的 as_of 参数，存在历史修订版本无法按分析时点重建的风险；这是 M2/M3 必须解决的迁移准入项，不应作为普通字段丢失处理。
- 后续仍需覆盖全部 35 个 Operation 的 Schema、鉴权和错误响应；同时需检查 Data HTTP Handler 的 JSON 输出是否与 Model/OpenAPI 一致，不能只依据静态结构声明。

**M0 状态继续保持 in_progress。** 本轮未修改业务代码和 API 行为。


### 2026-11-06：M0 第十二轮补充（部署配置与 Migration/资源边界）

#### 1. 当前 Compose 部署形态

当前根 docker-compose.yml 仍部署三个容器：stock-analysis-data（独立构建 services/market-data）、stock-analysis-server（根 Dockerfile）、stock-analysis-web。Root Server 通过 MARKET_DATA_BASE_URL 指向 Data 容器，并共享 MARKET_DATA_API_KEY；Web 容器仍分别配置 Data 与 Analysis 服务名。该形态是迁移前 Remote HTTP 运行基线，不能误认为方案 B 已完成。

历史 docker/docker-compose.go-migration.yml 仍是独立 profile，使用 services/market-data/.env、go-core/.env 和根 .env 分别装配；其中 go-core 与 Data 仍通过 HTTP 通信。M5 才调整默认 Compose，不在 M0 删除该兼容 profile。

#### 2. MariaDB 配置与数据库边界

- Root Store 使用 DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME/DB_PARAMS；默认 DB_NAME 为 stock_analysis_server，DB_AUTO_MIGRATE 默认 false。
- Data 使用 MARKET_DATA_DB_HOST/PORT/USER/PASSWORD/NAME/CHARSET/TIMEZONE；支持旧 MARKET_DATA_MYSQL_DSN；MARKET_DATA_DB_AUTO_MIGRATE 默认 true。
- 根 .env.example 同时设置 Root 与 Data 两组变量，二者 DB_HOST 均为 Mariadb、DB_NAME 均为 stock_analysis_server，数据库用户同为 stock_analysis_server，但密码变量分别为 DB_PASSWORD 与 MARKET_DATA_DB_PASSWORD。也就是说，示例部署意图是同一 MariaDB Database 下由两组连接配置访问不同业务表，而不是两个独立数据库。
- services/market-data/.env.example 是历史独立服务配置，默认 MARKET_DATA_DB_NAME=stock_analysis、用户 appuser；不能将该文件默认值与根 Compose 的统一部署配置混为一谈。
- Root Migrate 创建 analysis_runs、analysis_stage_events、analysis_snapshots、analysis_tasks；Data Migrate 创建 market_data_* 表。当前已确认表名空间不冲突，但两侧迁移执行入口、自动迁移默认值、连接池和失败关闭路径相互独立。
- Root mysqlredis.Open 在构造 Store 时可按 DB_AUTO_MIGRATE 执行 Root Migrate；Data main 在 OpenMySQL 后按 MARKET_DATA_DB_AUTO_MIGRATE 执行 Data Migrate。两者均在当前独立进程内执行；合并进程后不能让两个 Store 各自创建/关闭 Pool，也不能继续由两个独立开关隐式触发 Migration。

M2/M4 的建议边界：
1. Root Server 只创建一份 MariaDB *sql.DB 并拥有关闭权；Data Repository 接收 borrowed *sql.DB。
2. Root 组合根显式注册 Root 与 Data 两个 Migration Runner，按明确顺序执行；Migration 的 SQL 和领域所有权仍分别归 Root、Data。
3. Migration Runner 需要各自版本表或统一版本表中的独立 namespace；版本号不能只用一个全局递增数字混淆两个模块。
4. DB_AUTO_MIGRATE 与 MARKET_DATA_DB_AUTO_MIGRATE 应在 M3/M4 迁移期间收敛为 Root 的统一迁移策略或明确兼容映射；禁止出现一个开关关闭但另一个仍在启动时偷偷迁移的状态。
5. 迁移前先确认生产环境是否确实使用同一 Database、同一 MariaDB 实例及相同字符集/时区；示例文件只能证明默认部署意图，不能证明用户实际生产配置。

#### 3. Redis 配置边界

Root 使用 REDIS_ADDR/REDIS_PASSWORD/REDIS_DB 和独立的连接超时配置；Data 使用 MARKET_DATA_REDIS_ADDR/USERNAME/PASSWORD/DB。根 .env.example 中两侧地址均为 Redis:6379，DB 均为 0，但密码变量分开。迁移后是否共享同一 go-redis Client，取决于两侧用户名、密码、DB、TLS/连接参数是否完全一致；不一致时由 Root 组合根创建多个 Client 并分别拥有。Data Cache 只能借用，不得关闭 Root 持有的 Client。

#### 4. 最新 CI 证据补充

- 5ea9155c（430a2001 之前的代码/文档基线）上的 Go Test Suite 全部 Job success：Root Test、services/market-data Test、go-core Test、MariaDB migration/PIT persistence、Compose/Docker image build。
- 同一 SHA 的 Go-Python data quality parity、Platform Go checks、WebUI checks、Live Provider probe 均 success。
- 430a2001 当前触发的 CI 已观察到：Root Test、go-core Test、MariaDB migration/PIT persistence success；Parity、Platform Go、WebUI checks、Live Provider probe success；services/market-data Test 与 Compose/Docker build 当时仍 in_progress，尚不能记录最终结论。
- 上述证据不覆盖独立的 WebUI browser smoke 320px ERR_ABORTED 问题；该问题仍需单独定位。5ea9155c 之后的纯文档提交也不能直接继承前一 SHA 的 CI 结论，必须以对应提交的实际 Checks 为准。

#### 5. M0 风险更新

新增需进入 M2/M4 准入清单的事项：
- Data HTTP Adapter 迁移后继续保留，但默认内部调用切换 InProcess；当前 Compose 仅作为 Remote 基线和回滚路径。
- 两套 DB/Redis 配置名称及自动 Migration 开关必须统一映射，且共享资源必须显式 borrowed/owned。
- Root/Data 目前同库不同业务表，示例配置如此；生产实际 DSN/DB 需在迁移部署前核实。
- Root/Data Migration 需分模块版本命名空间并由 Root 组合根显式执行，不能简单把两个 Migrate() 合并成一个无边界函数。

**M0 状态继续保持 in_progress。** 本轮仅补充部署配置与资源审计，没有改变 Compose、数据库配置或 Migration 行为。
