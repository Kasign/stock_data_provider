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


### 2026-11-06：M0 第十三轮补充（认证重定向风险与 OpenAPI 操作清单）

#### 1. 认证信息跨域重定向风险（需按高优先级处理）

本轮复核 Tushare、Hithink、TickFlow、Xueqiu 等 Provider 的 HTTP Client 初始化方式：
- Eastmoney 有专用 redirect policy；Tencent/Sina/NetEase/Yahoo/MiaoXiang 显式禁止重定向。
- Tushare、Hithink、TickFlow、Xueqiu 当前使用默认 Go HTTP Client redirect policy。
- Tushare Token 放在 POST JSON Body；Hithink 的 X-api-key、TickFlow 的 x-api-key 是自定义 Header；Xueqiu 使用 Cookie Header。
- Go 默认重定向逻辑对 Authorization/Cookie 等敏感 Header 有跨域处理，但自定义 Key Header 不应假设自动受到同样保护；对 307/308，POST Body 可能被重放。由此，当前不能仅凭默认 Client 就认定认证信息不会跨域泄漏。
- 本轮未执行真实恶意重定向服务测试，因此记录为代码路径风险，而不是已复现的数据泄漏事件。

M4 必须增加可自动化的回归测试：
1. 起两个不同 Host 的本地 HTTP Test Server，源站返回 307/308 到目标站。
2. 分别验证 Tushare Body Token、Hithink/TickFlow 自定义 Key Header、Xueqiu Cookie 不会到达未授权目标 Host。
3. 覆盖同 Host、显式 allowlist Host、非 allowlist Host、HTTP→HTTPS、HTTPS→HTTP。
4. 所有 Provider 默认禁止跨 Host；需要跨 Host 的 Provider 通过显式目标集合授权，不以 Go 默认行为代替安全策略。
5. 认证 Header 与 Endpoint Host 必须绑定；对 POST Body 携带凭据的 Provider，跨 Host redirect 必须直接拒绝，不能只删除 Header。

#### 2. 本轮补齐的调用模式细节

- Tushare 日 K：POST / JSON，api_name=daily，params 为 ts_code/start_date/end_date，fields 为 ts_code,trade_date,open,high,low,close,pct_chg,vol,amount；Token 在 Body。统一请求限速间隔约 700ms。其他能力复用 fetchCapabilityTable，api_name 与 params/fields 随业务变化；资金流市场接口为 moneyflow_mkt_dc。
- Hithink：GET，X-api-key Header；公共 get() 将 Query 编码到 URL，覆盖搜索 q/asset_type/limit、指数 thscodes、日 K thscode/interval/start/end/adjust/offset、财务 thscode/period/limit 等；请求之间至少间隔约 150ms。
- TickFlow：GET /v1/quotes（symbols 或 universes）、GET /v1/universes、GET /v1/universes/CN_Equity_A、POST /v1/universes/batch（JSON ids）、GET /v1/klines；授权请求 x-api-key Header，免费日 K 使用独立 free Base URL；免费调用间隔至少 1 秒。
- Tencent：GET，Quote 将证券代码拼入 URL Path/Query 形式，日 K 使用 param Query；搜索走 Smartbox endpoint，设置 v=2/t=all/c=1/q=关键词；User-Agent 与 Referer 固定；禁止重定向。
- Eastmoney：主要 GET 参数通过 url.Values 组装；核心字段参数在各能力中显式定义，例如 Quote 使用 secid/fields/fltt/invt/时间戳，个股资金流使用 fields1/fields2，板块/指数榜单使用 fields、pn/pz、po、fid 等分页/排序字段；龙虎榜、公告使用 datacenter/announcement 对应 Query。热榜接口为 POST，需在逐调用点附表中继续列出完整 JSON Body。
- 以上为请求结构摘要；尚未将每个 Provider 每个方法的完整 Query/Body 字段枚举为机器可读清单，也未逐一验证响应大小上限。

#### 3. OpenAPI 操作清单复核

本轮再次解析 services/market-data/openapi.yaml 的顶层 Path，确认共有 35 个路径操作（含 health、probe、diagnostics、quotes/bars/bundle、stocks、market 等）。本次仅确认 Operation 路径清单数量和名称，没有完成每个 Operation 的 method、requestBody、parameters、responses、security 与 Schema 引用的逐项核对；也未完成 Server Handler 的实际 JSON 字段与 OpenAPI Schema 的 golden 对照。

下一轮应生成逐 Operation 核验矩阵，至少包含：
- Method + Path；
- Handler/UseCase；
- Path/Query/Header 参数；
- Request DTO 与 OpenAPI required/nullable；
- Response DTO/字段；
- 成功响应状态码；
- 400/401/403/404/409/422/429/5xx 错误响应；
- ApiKey/BearerAuth 是否与实际中间件一致；
- 兼容性要求及对应测试。

#### 4. 本轮 CI 状态（HEAD 1804a094）

- WebUI checks：success。
- Platform Go checks：success。
- Live Provider probe：success。
- Go-Python data quality parity：success。
- Go test suite：仍在运行；Root Test、go-core Test、MariaDB migration/PIT persistence 已 success；services/market-data Test 与 Compose/Docker build 当时 in_progress。
- 因此当前 CI 不能标记为全绿。

**M0 状态继续保持 in_progress。** 本轮未改动 Provider 请求行为或 OpenAPI；发现的认证跨域风险必须进入 M4 安全验收项。


### 2026-11-06：M0 第十四轮补充（OpenAPI 鉴权契约与实际路由差异）

#### 1. 发现明确的 OpenAPI 鉴权契约缺口

按 OpenAPI 3.0.3 Path/Operation 逐项提取 35 个 Operation，并与 httpapi/server.go 的 Go 1.22+ ServeMux 注册对照：
- Server 共注册 35 条 method+path 路由，与 OpenAPI 35 个 Operation 数量一致。
- Server 中只有 GET /health/live、GET /health/ready 不经过 s.auth；其余 33 条均经过 API Key 认证中间件。
- OpenAPI 中只有 11 条业务 Operation 显式声明 security: ApiKey；22 条业务 Operation 未声明 security，且文件顶层也没有全局 security。
- OpenAPI 未声明 security 的 22 条路径为：
  GET /api/v1/quotes/{symbol}
  GET /api/v1/bars/{symbol}/daily
  GET /api/v1/bundles/{symbol}
  GET /api/v1/data-sources/probe/history
  GET /api/v1/data-sources
  GET /api/v1/stocks/search
  GET /api/v1/stocks/{symbol}/announcements
  GET /api/v1/stocks/{symbol}/profile
  GET /api/v1/stocks/{symbol}/fundamentals
  GET /api/v1/stocks/{symbol}/chip-distribution
  GET /api/v1/stocks/{symbol}/capital-flow
  GET /api/v1/stocks/{symbol}/boards
  GET /api/v1/market/valuations
  GET /api/v1/market/sectors
  GET /api/v1/market/trading-calendar
  GET /api/v1/market/stocks
  GET /api/v1/market/sentiment
  GET /api/v1/market/sentiment/history
  GET /api/v1/market/breadth
  GET /api/v1/market/hot-stocks
  GET /api/v1/market/dragon-tiger
  GET /api/v1/market/limit-pools
- 其中部分 Operation 声明了 401 响应，但未声明 security；这仍会让 OpenAPI 客户端/文档工具把接口误判为无需认证。GET /api/v1/data-sources/diagnostics 已声明 ApiKey，但响应列表未声明 401，属于另一处错误响应契约缺口。
- 当前不是 Server 鉴权实现缺失，而是 OpenAPI 对实际认证行为描述不完整；不能为兼容文档而擅自放开 Server 路由。

#### 2. 迁移前修复要求

在 M0 准入前应确定 OpenAPI 的统一表达方式：
- 推荐在 OpenAPI 顶层声明全局 security: [{ApiKey: []}]，再仅对 health/live 与 health/ready 显式设置 security: []。
- 若存在真正公开的业务接口，必须由产品/安全边界确认后，Server 和 OpenAPI 同步调整；当前源码没有发现此类公开业务路由。
- 为所有 33 条受保护路由统一声明 401 响应及 Error Schema；对 diagnostics 等缺少 401 的操作补齐。
- 增加 contract test：逐 Operation 对照实际 mux 注册、认证中间件与 OpenAPI security；新增路由必须同步生成/维护契约测试。
- 该修复属于 OpenAPI 契约准确性修复，不涉及业务 Provider 行为；仍应在当前分支先评审，不要在本轮审计中顺手修改 API 文档或 Server。

#### 3. 其他操作级核验进度

- method+path：35/35 已确认一致。
- auth/security：发现 22 条漏声明，11 条显式 ApiKey，2 条 health 明确公开。
- requestBody、参数 required/type/format、response Schema、错误响应完整性尚未逐项核完。
- 当前 OpenAPI 采用部分 Operation 内联 Schema、部分 components 引用；后续需核对 Handler JSON 编码字段与 Schema，不可只比较 DTO 名称。

**M0 状态继续保持 in_progress。** 此发现列为 OpenAPI 契约阻断项，M1 前应完成契约修复并通过 contract test；本轮只更新迁移审计文档，没有修改 OpenAPI 或鉴权代码。


### 2026-11-06：M0 第十五轮补充（OpenAPI Operation 级响应与契约测试审计）

#### 1. 35 个 Operation 的逐项结构盘点

本轮直接读取当前 HEAD 的 `services/market-data/openapi.yaml`，按 Path/Method 解析 Operation，并与 `internal/httpapi/server.go` 的 ServeMux 路由逐项对照。当前共 35 个 Operation：2 个公开 Health、33 个受 API Key 保护的业务路由；Path + Method 数量一致。

下表记录每个 Operation 的参数名称、Request Body、响应状态码及鉴权声明。参数类型、required、format、默认值与 Handler 实际校验规则仍需下一轮逐项做语义比对，本表不将“存在参数定义”等同于“参数契约已核验”。

| Method + Path | 参数 | Body | 响应状态码 | OpenAPI 鉴权 |
|---|---|---|---|---|
| GET /health/live | — | — | 200 | 公开 |
| GET /health/ready | — | — | 200, 503 | 公开 |
| GET /api/v1/market/quotes | symbols | — | 200, 400, 401 | ApiKey |
| GET /api/v1/quotes/{symbol} | symbol | — | 200, 400, 401, 502, 504 | 缺失 |
| GET /api/v1/bars/{symbol}/daily | symbol, start, end, limit, adjustment, as_of | — | 200, 400, 401, 404, 502, 503, 504 | 缺失 |
| GET /api/v1/bundles/{symbol} | symbol, lookback_days, adjustment, as_of | — | 200, 400, 401, 502, 504 | 缺失 |
| POST /api/v1/data-sources/probe | — | ProviderProbeRequest | 200, 400, 401, 500 | ApiKey |
| GET /api/v1/data-sources/probe/history | limit | — | 200, 400, 401, 503 | 缺失 |
| GET /api/v1/data-sources/diagnostics | — | — | 200 | ApiKey |
| GET /api/v1/data-sources | — | — | 200, 401 | 缺失 |
| GET /api/v1/stocks/search | q, limit | — | 200, 400, 401, 502, 503 | 缺失 |
| GET /api/v1/stocks/{symbol}/announcements | symbol, start_date, end_date, page, page_size | — | 200, 400, 401, 502 | 缺失 |
| GET /api/v1/stocks/{symbol}/profile | symbol | — | 200, 400, 401, 502, 503 | 缺失 |
| GET /api/v1/stocks/{symbol}/price-limit-rule | symbol, date | — | 200, 400, 401 | ApiKey |
| GET /api/v1/stocks/{symbol}/trading-status | symbol, date, as_of | — | 200, 400, 401, 404, 502, 503 | ApiKey |
| GET /api/v1/stocks/{symbol}/fundamentals | symbol, year, quarter, as_of | — | 200, 400, 401, 404, 502, 503 | 缺失 |
| GET /api/v1/stocks/{symbol}/chip-distribution | symbol | — | 200, 400, 401, 502, 503 | 缺失 |
| GET /api/v1/stocks/{symbol}/capital-flow | symbol | — | 200, 400, 401, 502, 503 | 缺失 |
| GET /api/v1/stocks/{symbol}/boards | symbol | — | 200, 400, 401, 502, 503 | 缺失 |
| GET /api/v1/market/valuations | symbols | — | 200, 400, 401, 502, 503 | 缺失 |
| GET /api/v1/market/sectors | type, limit, direction, sort_by | — | 200, 400, 401, 502, 503 | 缺失 |
| GET /api/v1/market/index-catalog | tag | — | 200, 400, 401, 502, 503 | ApiKey |
| GET /api/v1/market/indices/{symbol}/constituents | symbol, source, as_of | — | 200, 400, 401, 502, 503 | ApiKey |
| GET /api/v1/market/indices/{symbol}/daily | symbol, start, end, source | — | 200, 400, 401, 502, 503 | ApiKey |
| GET /api/v1/market/indices | — | — | 200, 401, 502, 503 | ApiKey |
| GET /api/v1/market/trading-calendar | start, end | — | 200, 400, 401, 502, 503 | 缺失 |
| GET /api/v1/market/session | at | — | 200, 400, 401, 502, 503 | ApiKey |
| GET /api/v1/market/stocks | page, page_size, include_ma, adjustment | — | 200, 400, 401, 502, 503 | 缺失 |
| GET /api/v1/market/capital-flow | start, end | — | 200, 400, 401, 502, 503 | ApiKey |
| GET /api/v1/market/sentiment | date | — | 200, 400, 401, 502 | 缺失 |
| GET /api/v1/market/sentiment/history | start, end, limit | — | 200, 400, 401, 503 | 缺失 |
| GET /api/v1/market/breadth | — | — | 200, 401, 502, 503 | 缺失 |
| GET /api/v1/market/hot-stocks | type, limit | — | 200, 400, 401, 502, 503 | 缺失 |
| GET /api/v1/market/dragon-tiger | date, page, page_size | — | 200, 400, 401, 502 | 缺失 |
| GET /api/v1/market/limit-pools | type, date, limit | — | 200, 400, 401, 502, 503 | 缺失 |

#### 2. 本轮确认的契约问题

1. **鉴权声明仍为 11/33。** 33 条受保护路由中仅 11 条显式声明 ApiKey；22 条未声明。两条 Health 明确声明 `security: []`，与 Server 一致。OpenAPI 顶层没有 global security。
2. **诊断接口漏写 401。** `GET /api/v1/data-sources/diagnostics` 声明了 ApiKey，但只列出 200；Server 实际经过 `s.auth`，无效/缺失 API Key 会返回 401。
3. **Error Schema 未被 Operation 响应引用。** OpenAPI 定义了 `components.schemas.Error`，但本轮检索 35 个 Operation 的响应定义，没有发现任何响应的 `content.application/json.schema` 引用该 Error Schema。多数错误响应只有 description，没有可供客户端生成器使用的错误 Body Schema。Server 的 `writeErr`/错误处理实际输出统一的 `error.class/code/message/retryable/request_id` 结构，因此文档没有表达真实错误体。
4. **现有鉴权 contract test 覆盖不完整。** `TestOpenAPIDataRoutesDeclareValidAPIKeySecurity` 只硬编码检查 7 条 ApiKey 路由，没有枚举完整 33 条受保护路由，也没有对照 ServeMux 注册与 OpenAPI Operation。当前测试即使 22 条业务 Operation 漏声明 security，仍可能通过。
5. **Operation 错误状态码不应简单统一成固定全集。** 各 Handler 的业务错误不同，需以实际错误映射为准；但每条受保护路由都必须描述 401，每个已声明错误状态码都应引用统一 Error Schema。Health ready 的 503 也应描述其实际 JSON 错误结构，Health live 无需添加错误响应。

#### 3. 建议的契约修复与测试方案

- OpenAPI 顶层设置 `security: [{ApiKey: []}]`；Health live/ready 保留显式 `security: []`。
- 保留当前 Server 的 33 条 API Key 保护行为，不为匹配不完整文档而放开接口。
- 给 diagnostics 补 401；给所有受保护业务 Operation 补齐 401 + Error Schema。
- 为已声明的 400/404/409/422/429/500/502/503/504 等错误响应补充 `application/json` 和 Error Schema；按 Handler 实际可能返回的状态码核验，避免只补文档、不符合运行行为。
- contract test 应从 OpenAPI YAML 读取全部 Path/Method，并与 ServeMux 路由清单做双向集合比较；再验证除 Health 外全部 Operation 有 ApiKey，Health 为 `security: []`，所有受保护 Operation 声明 401，所有错误响应引用 Error Schema。
- 测试应覆盖未配置 API Key、错误 API Key、正确 API Key三类 HTTP 行为；避免仅对字符串片段做静态断言。

#### 4. 当前审计结论与下一步

- 已完成：35 个 Operation 的 method/path、参数名称、Request Body 存在性、响应状态码、ApiKey 声明情况初步盘点；35/35 与 Server 路由数量一致。
- 已确认阻断：22 条 security 漏声明；diagnostics 缺少 401；Error Schema 没有被 Operation 错误响应引用；鉴权 contract test 仅覆盖 7 条受保护路由。
- 尚未完成：逐参数 required/type/format/default 与 Handler 校验逻辑核对；所有成功 Response Schema 与 Handler JSON 的字段/nullable 对照；每个 Handler 的真实错误状态码与 Error Body golden 对照。
- 本轮没有修改 OpenAPI、Server、Handler 或 contract test，仅补充迁移审计文档。

**M0 状态保持 in_progress，不满足 M1 准入条件。**


### 2026-11-06：M0 第十六轮补充（参数校验与错误状态码映射）

#### 1. 参数定义与 Handler 校验的交叉核验

本轮抽查了关键参数组，并对照 OpenAPI 参数 required/type/range 与 Handler 实际校验：

- `GET /api/v1/market/quotes`：`symbols` 在 OpenAPI 标为 required；Handler 拒绝缺失、空值、重复证券、非法代码及超过 50 项，基本一致。
- `GET /api/v1/stocks/search`：`q` required，长度 1..64；Handler 对空值及超过 64 个 Unicode 字符拒绝。可选 `limit` 默认 10、范围 1..50，与 OpenAPI 默认值/边界一致。
- `GET /api/v1/market/valuations`：`symbols` required；Handler 限制最多 100 个原始 token，拒绝空 token、非法代码和非 A 股代码，与文档描述基本一致。
- `GET /api/v1/stocks/{symbol}/fundamentals`：`year`、`quarter` required，year 最小 2007，quarter 1..4；Handler 还将 year 上界限制为当前年份，OpenAPI 未显式描述动态上界。
- `GET /api/v1/stocks/{symbol}/price-limit-rule` 与 `GET /api/v1/stocks/{symbol}/trading-status`：`date` required、`as_of` optional；Handler 对 date 使用 YYYY-MM-DD 并拒绝未来日期，对 as_of 要求单个 RFC3339 时间戳且不能晚于当前时间，文档总体一致。
- `GET /api/v1/market/capital-flow`、`GET /api/v1/market/trading-calendar`、`GET /api/v1/market/sentiment/history`：start/end 均 required；Handler 对范围、日期格式和最大跨度执行校验，OpenAPI 的 required 声明一致。
- `GET /api/v1/market/session`：`at` optional；Handler 未提供时使用当前服务时间，提供时要求单个 RFC3339 时间戳，与文档描述一致。
- `GET /api/v1/market/stocks`：page/page_size/include_ma/adjustment 均 optional；Handler 分别校验整数范围、布尔值和复权枚举，默认值由 Handler 决定。后续仍需核对 OpenAPI schema 的 default 是否与 Handler 默认值完全一致。
- `GET /api/v1/market/limit-pools`、`GET /api/v1/market/dragon-tiger`、`GET /api/v1/stocks/{symbol}/announcements`：Handler 存在分页/条数默认值及上限；需把 OpenAPI 的 minimum/maximum/default 与 Handler 逐字段对照，当前不将参数名称清单视为完成核验。

本轮抽查暂未发现上述必填参数在 OpenAPI 中被错误标成 optional 的明确案例；但 fundamentals year 的动态上界、分页默认值/范围仍需逐 Operation 完成机器化核验。

#### 2. 错误状态码与实际公共错误映射不完整

Server 的 `writeDomainErr` 统一映射：
- Provider 无效/异常响应：502；
- 无可用 Provider：503；
- `context.DeadlineExceeded`：504；
- `context.Canceled`：408；
- 参数/日期/复权校验：400。

因此，调用 `writeDomainErr` 的 Operation 不仅可能返回文档中常见的 502/503，也可能返回 504；请求取消时还会尝试返回 408。当前 OpenAPI 只有 Quote、Daily Bars、Bundle 等少数 Operation 声明了 504。比如 stock search、profile、fundamentals、chip-distribution、capital-flow、boards、valuations、sectors、index catalog、index constituents/daily、indices、trading-calendar、session、breadth、hot-stocks、dragon-tiger、limit-pools 等 Operation 的响应列表没有 504，但对应 Handler 的 Provider 错误路径使用公共 `writeDomainErr` 映射，超时会产生 504。

这意味着 OpenAPI 的错误状态码并非仅缺统一 Schema，还存在实际可达状态码漏声明。需要在后续逐 Handler 矩阵中标注是否使用 `writeDomainErr`、是否可能返回 408/504，以及是否存在专属 404/503/500 分支。不要机械地给不访问 Provider 的历史存储接口添加 504。

#### 3. Error Body 结构已与 Server 实现确认

Server 的 `writeErrWithRetryable` 始终输出 JSON：
- 顶层 `error`；
- `error.class`、`error.code`、`error.message`、`error.retryable`；
- `error.request_id` 在非空时输出（`omitempty`）。

`error.class` 由 HTTP 状态映射为 validation / unauthenticated / forbidden / not_found / conflict / timeout / unavailable / internal。OpenAPI 的 `components.schemas.Error` 已定义上述结构，但当前 Operation 响应未引用它。该问题可通过共享响应组件或各响应引用同一 Error Schema 解决，不需要改变 Server 错误 Body。

#### 4. 下一步

1. 为 35 个 Operation 建立参数级矩阵：name、in、required、type、format、enum、minimum/maximum、default、Handler 默认值和实际校验。
2. 对每个 Handler 标注错误产生路径：直接 writeErr、writeDomainErr、专属业务错误；据此补齐真实可达状态码，重点是 504。
3. 将所有错误响应统一引用 Error Schema；diagnostics 增加 401。
4. 扩展 contract test，避免只测 7 条静态路径；以完整 Operation 与路由集合为基准验证。
5. 继续做成功响应 JSON 与 OpenAPI Schema 的实际字段/nullable golden 对照。

**M0 状态保持 in_progress。** 本轮只更新迁移审计文档，没有修改 OpenAPI 或 Handler 行为。


### 2026-11-06：M0 第十七轮补充（OpenAPI JSON Golden 覆盖范围）

#### 1. 当前 OpenAPI contract test 实际覆盖边界

检查 `services/market-data/internal/httpapi/openapi_contract_test.go`，当前只有 5 个测试：
- `TestOpenAPIDataRoutesDeclareValidAPIKeySecurity`
- `TestProviderProbeValidationSchemaIsDocumented`
- `TestSectorRankingFiveDayChangeSchemaIsDocumented`
- `TestOpenAPIDataModelFieldsMatchKnownJSONContract`
- `TestDataModelJSONSerializationMatchesOpenAPIRequiredAndNullableSemantics`

其中字段/JSON 序列化测试目前只对少数代表性结构做手工断言：Quote、DailyBar、Bundle、HotRankingItem、MarketSentimentSnapshot、LimitPoolItem、SourceStatus、StockAnnouncementSnapshot。它们能够防止已列举字段回归，但不构成全部 Response Schema 与 Go JSON 的自动对账。

当前测试没有完成以下自动化覆盖：
1. 遍历所有 OpenAPI components.schemas 与 Go Model JSON tag 的双向字段比较。
2. 对所有 Schema required 字段验证 JSON 必须存在，对 nullable 字段验证 null/非 null 行为。
3. 对每个 Operation 的 200 Response Schema 验证 Handler 实际输出。
4. 对匿名 map 包装响应做 JSON Golden 验证。
5. 对错误响应验证统一 Error Body 与 OpenAPI Error Schema 一致。

#### 2. Handler 输出形态需要单独纳入 Golden

当前 Handler 并非全部直接返回单个 Model：
- Daily Bars 返回外层集合对象；PIT 分支额外输出 point_in_time=true 与 as_of。
- Data Sources 返回 sources 与 normalization_version。
- Diagnostics 返回 items、scope、observed_from 的匿名对象。
- Batch Quotes 返回 items、total、succeeded、failed、fetched_at、quality_status 的组合对象。
- Health live/ready 返回状态对象。
- 其余大量 Handler 直接序列化 Model Snapshot 或单个 Model。

因此仅验证 Model struct 无法覆盖 Operation 的外层包装字段、PIT 条件字段、逐项错误字段和聚合统计字段。建议为每个 Operation 建立最小有效/降级/错误三类 JSON fixture；对条件字段分别验证存在与省略规则。Provider 行为本身不需要在 M0 变更，测试可以使用现有 fake Provider 与固定时间。

#### 3. M0 验收建议新增硬性测试门槛

- **Schema/Model 对账**：对具有 Go Model 对应关系的 components schema，自动核对 JSON tag、required、nullable、type/format；允许差异必须通过显式例外清单解释。
- **Operation Golden**：每条 Operation 至少有一个成功响应 fixture，重点覆盖匿名包装、PIT、nullable、optional/omitempty。
- **Error Golden**：400、401、404、408、429（若该路由可达）、500、502、503、504 等实际错误分支按路由能力覆盖；统一验证 Error Body。
- **路由/鉴权对账**：与第十五轮所述测试合并，确保 35 个 Operation 与 mux 路由双向一致。
- **不将文档静态字符串测试视为运行时契约测试**：测试需要解析 YAML 结构，并实际 Marshal/Serve HTTP 响应。

**M0 状态保持 in_progress。** 当前契约测试是局部回归保护，不足以证明全部 Handler JSON 与 OpenAPI 完全一致。本轮只记录审计结论，没有修改测试或业务代码。


### 2026-11-06：M0 第十八轮补充（Provider 出站请求行为矩阵）

#### 1. 核查范围与证据

本轮按当前分支 services/market-data/internal/provider/ 下实际 Provider 实现核查 HTTP Client 构造、请求方法、凭据位置、超时、限速、重定向和响应体读取边界。覆盖 Eastmoney、Tushare、Hithink、TickFlow、Xueqiu、MiaoXiang、Tencent、Sina、NetEase、Yahoo Finance、BaoStock、Pytdx。

本轮为源码静态核查，不代表已对所有上游接口执行真实网络测试。Endpoint override、代理、跨 Host 重定向和恶意响应场景仍需自动化隔离测试。

#### 2. Provider 请求行为矩阵

| Provider | 传输/方法 | 请求参数与凭据 | Client 超时/节流 | Redirect | 响应体边界 |
|---|---|---|---|---|---|
| Eastmoney | HTTP GET 为主；热榜等路径存在 POST JSON | Query 参数；UT 为公开前端参数，不是部署私密凭据；固定 UA/Referer/Accept | 默认 12s；多个请求另设 6s/8s attempt timeout；部分 GET 最多 3 次重试，退避 700ms、1400ms | 自定义 eastmoneyRedirectPolicy，需单独验证 Host/协议规则 | 成功响应 maxProviderPayloadBytes；HTTP 错误最多读取 4096 bytes |
| Tushare | HTTP POST JSON | api_name、params、fields 在 JSON Body；Token 同样在 Body | 默认 12s；请求间隔约 700ms | 未设置 CheckRedirect，使用 Go 默认策略 | 成功响应 maxProviderPayloadBytes；错误响应最多读取 4096 bytes |
| Hithink | HTTP GET | Query；X-api-key Header | 默认 15s；请求间隔至少 150ms | 未设置 CheckRedirect，使用 Go 默认策略 | 成功响应 maxProviderPayloadBytes；错误响应最多读取 4096 bytes |
| TickFlow | HTTP GET + POST JSON | Query 或 JSON Body；授权请求按调用参数附加 x-api-key Header；免费 K 线使用独立 free Base URL | 默认 20s；免费请求间隔至少 1s | 未设置 CheckRedirect，使用 Go 默认策略 | 成功响应 maxProviderPayloadBytes；错误 Body 最多读取 2048 bytes，并对已配置 API Key 做文本脱敏 |
| Xueqiu | HTTP GET | Query；Cookie Header；Cookie 仅驻留内存，代码注释要求不进入日志/错误 | 默认 10s；未发现 Provider 级统一节流 | 未设置 CheckRedirect，使用 Go 默认策略 | 成功响应 maxProviderPayloadBytes；错误响应最多读取 4096 bytes |
| MiaoXiang | HTTP POST JSON | toolQuery JSON Body；apikey Header | 默认 10s；存在跨实例共享的包级请求间隔控制，精确间隔值仍需补录 | 显式返回 http.ErrUseLastResponse，不自动跟随 | 成功响应上限 2 MiB；错误响应最多读取 4096 bytes |
| Tencent | HTTP GET | URL Query/param；固定 UA/Referer；无用户 API Key | 默认 12s；未发现统一 Provider 级节流 | 显式禁止自动跟随 | 成功响应 maxProviderPayloadBytes；错误响应最多读取 4096 bytes |
| Sina | HTTP GET | Query/Path；固定 UA/Referer/Accept | 默认 12s；未发现统一 Provider 级节流 | 显式禁止自动跟随 | Quote/Kline 使用 maxSinaResponseBytes 上限 |
| NetEase | HTTP GET | Query；固定 UA/Referer/Accept | 默认 12s；未发现统一 Provider 级节流 | 显式禁止自动跟随 | Quote 使用 maxNeteaseQuoteBytes；历史 K 线使用 maxProviderPayloadBytes |
| Yahoo Finance | HTTP GET | Query；固定 UA/Referer/Accept | 默认 12s；未发现统一 Provider 级节流 | 显式禁止自动跟随 | 成功响应 maxProviderPayloadBytes；错误响应最多读取 4096 bytes |
| BaoStock | 原生 TCP/协议客户端 | 证券代码、日期范围、复权标记通过协议方法传递；不是 HTTP API | Provider Adapter 不持有 HTTP Client；连接/超时由 BaoStock Client 实现负责 | 不适用 | 需核对协议层帧长度/响应行数限制 |
| Pytdx | 原生 TDX TCP 协议 | 行情代码、分页 offset/count、复权数据通过协议方法传递；不是 HTTP API | 默认 12s；单次 TCP Dial 上限 1.5s；轮转最多尝试 6 个 Host；请求间隔至少 150ms；日线每页 800 条，最多扫描 40 页 | 不适用 | 当前可见限制包括 40 页/每页 800 条；仍需核对底层帧/分配上限 |

说明：
- 默认超时是 Provider 构造函数在 timeout 未配置或非正数时使用的值；Context deadline 仍可提前取消。
- “未发现统一 Provider 级节流”不表示上游没有频控，也不表示可以无限并发。
- Endpoint override 能改变目标地址；不同 Provider 的 URL 解析、Scheme、Host、端口和允许目标约束不统一。平台受控 HTTP Client 接入时必须以配置来源和可信目标策略为边界，不可只替换底层 Client 就默认安全。

#### 3. 当前可以确认的安全与资源风险

1. **凭据跨域重定向保护不一致。** Tushare、Hithink、TickFlow、Xueqiu 使用默认 Go redirect 行为；自定义 API Key Header、Cookie、Body Token 不应假设均受到跨 Host 保护。Eastmoney 有自定义策略，仍需通过 Host 测试证明；MiaoXiang、Tencent、Sina、NetEase、Yahoo 明确禁止自动重定向。
2. **跨 Host POST Body 凭据风险。** Tushare Token 在 JSON Body；TickFlow POST 可携带自定义 API Key Header。对于 307/308，必须验证是否重放原始 Body/Header。对 Body 携带凭据的请求，跨 Host 应拒绝，而不是只删除敏感 Header。
3. **节流策略不是统一资源。** Tushare、Hithink、TickFlow、Pytdx、MiaoXiang 有各自节流机制；部分使用实例字段，MiaoXiang 使用包级状态。迁移到同进程后要评估多实例/多组合根是否造成共享限速状态差异，不能未经验证统一成一个全局限速器。
4. **请求体与响应体上限不统一。** HTTP Provider 多数有响应体上限，但常量不同；MiaoXiang 单独为 2 MiB，Sina/NetEase 使用专用上限。错误响应通常只读取 2–4 KiB。TCP Provider 不受这些 HTTP 限制，需要独立核查协议解码限制。
5. **Endpoint 可配置性扩大出站边界。** 多数 Provider 支持部分或全部 Endpoint override，但并非所有构造函数都在创建时校验绝对 URL、Scheme、UserInfo、Query、Fragment 或 Host allowlist。必须在配置加载/受控 Client 层统一验证，保留 Provider 的路径和 Query 语义。
6. **重试行为不一致。** 本轮明确观察到 Eastmoney 对部分 429/5xx/网络超时等错误最多重试 3 次，并按 700ms、1400ms 退避；不能泛化到其他 Provider。平台 HTTP Client 接入不应暗中改变重试次数、幂等性或节流节奏。

#### 4. M4 受控 HTTP Client 接入的具体验收矩阵

每个使用 HTTP 的 Provider 至少验证：
- 请求保持：Method、Path、Query 编码、JSON Body、Content-Type、Accept、UA、Referer、Origin、自定义认证 Header 与 Cookie 不变。
- 超时与取消：Client timeout、每次请求 Context deadline、调用方取消；确认响应 Body 被关闭，取消后不再启动后续重试。
- 重定向：同 Host 301/302/303/307/308；跨 Host 301/302/303/307/308；HTTP→HTTPS；HTTPS→HTTP；allowlist 与非 allowlist Host。默认拒绝跨 Host，只有契约明确授权的目标可例外。
- 凭据隔离：分别覆盖 Tushare Body Token、Hithink/TickFlow Key Header、Xueqiu Cookie、MiaoXiang apikey；断言非授权 Host 收不到凭据。跨 Host 307/308 不得重放含凭据 Body。
- 响应大小：正常边界、恰好上限、超过上限、超大错误 Body、无 Content-Length 的流式响应；确认不会无界读取或将截断 JSON 当成完整成功结果。
- 重试/限速：429、500、502、503、504、连接重置、超时、Context cancel；确认只重试原本允许重试的错误，退避可取消，POST 不因重试产生非幂等副作用。
- TCP 单独验收：BaoStock/Pytdx 的目标 Host 配置、Dial timeout、连接轮转、协议响应帧/行数上限、取消和 Close；不将 HTTP Client 契约错误套用到 TCP 协议。

建议拆为通用 HTTP 安全测试 + Provider 专属请求 Golden。通用层验证受控 Client 的重定向、大小、超时和目标限制；Provider Golden 验证请求语义与凭据位置，避免重复实现安全逻辑，又不因统一 Client 改变上游协议。

#### 5. M0 尚未完成的 Provider 审计项

- [ ] 为 Eastmoney 所有 GET/POST 调用点生成完整 Query/Body 字段矩阵；目前已确认存在多组独立 Endpoint 与重试路径，不能用一个通用摘要代表所有能力。
- [ ] 补录 MiaoXiang 包级限速的精确间隔、缓存并发行为及缓存失效边界。
- [ ] 核对各 Provider Endpoint 配置项与真实部署配置的映射，区分可配置 URL、固定 URL、Token/API Key、Cookie 与公开参数。
- [ ] 核对所有 HTTP Provider 的响应上限常量、错误 Body 上限和 JSON 解码前分配边界；核对 BaoStock/Pytdx 协议库帧长度与解码限制。
- [ ] 检查所有 Provider 是否有并发上限、连接池复用、空闲连接回收和关闭能力；迁入同进程后避免多份 Client/连接池。
- [ ] 完成双 Host 重定向测试和 Endpoint 非法配置测试；本轮只有静态源码证据，不能将风险写成已复现漏洞或已通过安全验收。

**本轮结论：** Provider 请求语义已形成覆盖 12 个 Provider 的首版矩阵，但仍未达到逐调用点完整盘点；出站安全风险明确，需在 M4 受控 HTTP Client 接入时做行为等价和安全回归。M0 继续保持 in_progress，不满足 M1 准入。


### 2026-11-06：M0 第十九轮补充（Eastmoney 调用点与 MiaoXiang 缓存/限速）

#### 1. Eastmoney 调用点矩阵（源码核查）

本轮继续拆分 Eastmoney 的独立调用族，避免把多个接口共用 getJSON/getJSONWithRetry 误认为同一个请求契约。

| 能力/调用族 | 方法 | 主要 Query 字段 | Body / Header | Endpoint 来源 | 超时/重试 |
|---|---|---|---|---|---|
| 个股实时行情 | GET | secid、fields、fltt、invt、ut、时间戳 _ | UA、Referer、Accept | EASTMONEY_QUOTE_URL | Client 默认 12s；该调用走 getJSON，不额外覆盖 attempt timeout |
| 个股基础资料/市场画像 | GET | reportName、columns、quoteColumns、filter、pageNumber、pageSize、sortTypes、sortColumns、source、client、版本时间戳；画像补充 quote 请求另有 secid/fields/fltt/invt/ut | UA、Referer、Accept | STOCK_BASIC_URL；行情补充复用 QUOTE_URL | 基础资料 6s attempt；画像行情 8s attempt |
| 个股日 K / 筹码分布 | GET | secid、fields1、fields2、klt、fqt、beg、end、lmt、ut、时间戳；筹码分布固定日线窗口和 limit | UA、Referer、Accept | KLINE_URL | 日 K 走 getJSON；筹码分布 8s attempt |
| 热门/飙升榜原始榜单 | POST JSON | Body: appId、globalId、marketType、pageNo、pageSize | Content-Type/Accept/UA/Referer；无私密 API Key | HOT_RANKINGS_URL / HOT_SURGE_URL | 每次 8s Context；最多 3 次；本调用的重试与通用 GET 退避需分别验证 |
| 热榜行情补全 | GET | secids、fields、ut、fltt、invt | UA、Referer、Accept | INDEX_URL | 8s attempt |
| 证券目录/搜索/市场成交额 | GET | pn、pz、po、np、ut、fltt、invt、fid、fs、fields、时间戳；不同调用的 fs/fields/page size 不同 | UA、Referer、Accept | SECTOR_RANKING_URL（市场股票列表可独立覆盖） | 搜索/成交额 6s attempt；部分调用分页 |
| 板块排名/五日候选 | GET | pn、pz、po、np、ut、fltt、invt、fid、fs、fields、时间戳 | UA、Referer、Accept | SECTOR_RANKING_URL | 排名 6s；五日候选 8s；候选最多 20 页/2000 行 |
| 板块五日日 K | GET | secid、fields1、fields2、klt、fqt、beg、end、lmt、ut、时间戳 | UA、Referer、Accept | KLINE_URL | 8s attempt |
| 指数行情 | GET | secids、fields、fltt、invt、ut、时间戳 | UA、Referer、Accept | INDEX_URL | 走 getJSON；Client 默认 12s |
| 市场涨跌分布 | GET | dpt、ut、时间戳 | UA、Referer、Accept | MARKET_STOCKS_URL | 8s attempt |
| 涨跌停/炸板池 | GET | ut、dpt、Pageindex、pagesize、sort、date、时间戳 | UA、Referer、Accept | LIMIT_POOL_BASE_URL + 固定路径 getTopicZTPool/getTopicDTPool/getTopicZBPool | 6s attempt；DTPool 特定异常响应存在第二次请求路径 |
| 个股板块归属 | GET | forcect、spt、fields、pi、pz、po、fid、fid0、invt、secid、ut、时间戳 | UA、Referer、Accept | STOCK_BOARDS_URL | 6s attempt |
| 个股资金流 | GET | secid、fields1、fields2、klt、lmt、end、ut、时间戳 | UA、Referer、Accept | CAPITAL_FLOW_URL | 8s attempt |
| 龙虎榜 | GET | reportName、columns、filter(TRADE_DATE)、pageNumber、pageSize、sortColumns、sortTypes、source、client | UA、Referer、Accept | DRAGON_TIGER_URL | 8s attempt |
| 个股公告 | GET | sr、page_size、page_index、ann_type、client_source、f_node、s_node、begin_time、end_time、stock_list | UA、Referer、Accept | ANNOUNCEMENTS_URL | 8s attempt |

补充说明：
- Eastmoney 的 UT 是公开前端请求参数，不应按私密 Token 管理；但仍需按配置/代码版本跟踪其变化。
- Hot ranking POST 的 Body 包含 appId/globalId，但当前代码注释和配置语义未将其定义为部署密钥；不可与 Tushare Token 混为一谈。
- GET 参数由 url.Values 编码，getJSON 会覆盖 URL 原有 RawQuery；Endpoint 配置契约因此要求基础 URL 不携带 Query。
- 上表是按调用族归并的参数清单，不代表每个分页循环、Fallback URL、异常重试分支都已逐调用点验收。M0 仍需补充请求级 Golden 覆盖。

#### 2. Eastmoney Endpoint 配置映射与校验发现

当前 services/market-data/internal/config/config.go 对以下环境变量读取并调用 validateEndpointURL：
- Eastmoney：QUOTE、KLINE、STOCK_BASIC、SECTOR_RANKING、INDEX、MARKET_STOCKS、LIMIT_POOL_BASE、STOCK_BOARDS、DRAGON_TIGER、ANNOUNCEMENTS、HOT_RANKINGS、HOT_SURGE、CAPITAL_FLOW。
- Tencent：QUOTE、KLINE、NONE_KLINE、STOCK_SEARCH。
- Sina：QUOTE、KLINE。
- Yahoo：CHART。
- NetEase：HISTORY、QUOTE。
- Tushare：API_URL。
- Hithink：BASE_URL。
- Xueqiu：BASE_URL。
- TickFlow：API_URL、FREE_API_URL。
- MiaoXiang：API_URL。

validateEndpointURL 当前要求绝对 URL、Host 非空、Scheme 为 http/https，且不允许 UserInfo、Query、ForceQuery、Fragment；**但明确允许明文 HTTP，也没有对 Host/IP/DNS 目标做 allowlist 限制**。这意味着当前检查解决的是 URL 形态和凭据嵌入问题，不是 SSRF/出站目标授权。

另外，Eastmoney Provider 构造器存在 WithHotRankingURLs、WithCapitalFlowURL 等二次覆盖入口；这些方法主要做 TrimSpace/空值处理，没有独立调用 validateEndpointURL。必须确认配置构造链路确实先经过 config 校验，且后续不会从不可信动态配置绕过。

迁移到统一 HTTP Client 时建议拆成两层：
1. 配置层：保留现有 endpoint 字段与默认值；拒绝非 HTTPS（如需本地测试则通过显式测试开关，而不是生产默认放行）；拒绝 UserInfo/Query/Fragment。
2. Transport 层：校验解析后的目标 Host、端口、解析 IP 和重定向目标；DNS 解析后防止私网/回环/链路本地地址绕过；对重定向重新执行策略。是否允许自建代理或私有上游应通过显式可信目标配置表达，不应隐式放开全部内网地址。

#### 3. MiaoXiang 限速、并发准入与缓存边界

源码确认：
- 请求间隔目标为全局至少 300ms，状态由包级 miaoXiangRateMu / miaoXiangLastRequest 维护，而不是 Provider 实例字段。
- 同时存在包级 miaoXiangRequestLock.TryLock()；任一实例正在请求时，其他实例不会排队等待，而是立即返回 “request already in progress; supplemental query skipped”。
- 锁覆盖限速等待、HTTP 请求、响应读取、JSON 解码和结果提取全过程；不是只保护网络发送瞬间。
- LastRequest 在 client.Do 返回后更新（无论成功响应还是网络错误），因此网络失败也会占用下一次 300ms 节流窗口；在 Do 前取消则不会更新。
- 每实例独立缓存，TTL 为 5 分钟；缓存命中不经过全局请求锁，也不占用限速窗口。
- 缓存键由调用路径传入，当前 Chip Distribution / Capital Flow 分别构造 dataset + canonical symbol 键；成功解析出非空受支持表格后才写缓存，失败、HTTP 非 2xx、超限或解码失败不会写缓存。
- 缓存过期判断为 now - storedAt >= TTL，命中后删除过期项；当前未发现周期清理任务，因此长期不再访问的过期键会留在 map，直到同键再次读取或实例生命周期结束。
- 成功响应上限 2 MiB；错误响应最多丢弃读取 4096 bytes；请求通过 apikey Header 认证，POST JSON Body 只有 toolQuery。
- 构造器要求绝对 HTTP(S) URL，禁止 UserInfo、Query、Fragment；但仍允许 HTTP，且未做目标 Host allowlist。

迁移影响：
1. 同进程多实例时，包级全局 TryLock 会让一个实例的慢请求阻塞其他实例的补充数据请求准入；这不是“跨实例共享限速”这么简单，而是全局单飞 + 全局最小间隔。
2. 如果后续拆分成多个 Server 组合根/测试并行运行，包级锁和时间戳会跨实例共享，测试之间可能产生非确定性；应明确将其变为显式共享 RateLimiter 依赖，或明确约束单实例。
3. 如果未来改成排队限流器，会改变当前“繁忙时快速跳过”的行为，必须保留该行为或经产品/调用方确认后再改。
4. 缓存目前没有容量上限和全局清理策略；当前数据集/股票组合有限，但仍应增加容量/生命周期边界或证明实例短生命周期足够。
5. 多实例各自缓存不共享，可能造成重复上游请求；若迁移时合并缓存，需明确 key、数据类型不可变性和 TTL 语义，避免跨实例共享可变 slice。

#### 4. 本轮审计结论与待办状态

- [x] Eastmoney 主要调用族的 Method、Query/Body、Endpoint、attempt timeout 已形成矩阵。
- [x] MiaoXiang 300ms 全局间隔、全局 TryLock、每实例 5 分钟缓存和成功写缓存边界已确认。
- [x] 明确 Endpoint URL 校验并不等同于出站 Host/IP 授权。
- [ ] Eastmoney 所有调用点的逐字段 Request Golden、Fallback 与重试分支仍需测试覆盖核验。
- [ ] 完成 Provider 构造/配置实际调用链核对，确认所有环境变量都传入正确的 Provider 字段。
- [ ] 补齐 MiaoXiang 缓存容量/实例生命周期决策。
- [ ] 对 HTTP/TCP Provider 做连接池、并发上限、响应分配和 Close 生命周期审计。

**本轮结论：** Eastmoney 参数矩阵和 MiaoXiang 缓存/限速行为已由源码确认；Endpoint 当前只做 URL 形态校验，不提供目标主机授权。该问题应纳入 M4 统一出站安全边界，但不能在 M0 文档中误报为已存在可远程利用的 SSRF 漏洞。M0 继续 in_progress。


### 2026-11-06：M0 第二十轮补充（Provider 装配与连接资源生命周期）

审计基线：`71f969e8f48cebec4ff8bb44cfb963b1f902d3a0`。本轮只读核查配置、独立入口、Provider 构造器、HTTP/TCP Client 和 Cache 资源；未修改业务实现。

#### 1. 配置到 Provider 的装配链

已核对 `config.Load()`、`services/market-data/cmd/market-data/main.go` 的 Provider 构造和 DataService 注册：

- Eastmoney 的 13 组 Endpoint（含 Hot Rankings、Hot Surge、Capital Flow）均传入构造器/With 方法；UT 作为独立参数传入。
- Hithink BaseURL/API Key、Tencent 4 组 URL、Sina 2 组 URL、Yahoo Chart URL、NetEase Quote/History URL、Tushare API URL/Token、TickFlow API/Free API URL/API Key、Xueqiu BaseURL/Cookie 均从 Config 传入对应构造器。
- BaoStock Host/Port/UserID/Password/ConnectTimeout/RequestTimeout 组装为 `baostock.Config` 后创建单实例 Client。
- Pytdx Hosts 传入 `NewPytdx`；其 Timeout 使用 ProviderTimeout。
- MiaoXiang 不在常规 Provider map 中，而是在 `MXEnabled` 时单独创建 SupplementProvider；API Key、URL、Timeout 均来自 Config。
- Config.Load 对 HTTP Endpoint 进行 URL 形态校验，但 BaoStock Host/Port 和 Pytdx Hosts 是 TCP 目标，未经过同等的目标主机/端口策略校验。HTTP Endpoint 允许明文 HTTP 的既有发现仍然成立。

本轮确认的是源码默认值和 Config→构造器映射，不代表已核对生产 Compose/部署环境的最终覆盖值；部署变量映射仍需单独对照。

#### 2. HTTP Provider Client 与连接池现状

静态核查 10 个 HTTP Provider 源文件：

| Provider | Client 创建方式 | Redirect / Transport 特征 |
|---|---|---|
| Eastmoney | 构造器内创建独立 `*http.Client` | 自定义 `eastmoneyRedirectPolicy` |
| Hithink | 构造器内创建独立 `*http.Client` | 默认 Redirect；未显式注入 Transport |
| Tencent | 构造器内创建独立 `*http.Client` | 禁止自动 Redirect |
| Sina | 构造器内创建独立 `*http.Client` | 独立默认 Transport |
| Yahoo | 构造器内创建独立 `*http.Client` | 禁止自动 Redirect |
| NetEase | 构造器内创建独立 `*http.Client` | 默认 Transport |
| Tushare | 构造器内创建独立 `*http.Client` | 默认 Redirect |
| TickFlow | 构造器内创建独立 `*http.Client` | 默认 Redirect |
| Xueqiu | 构造器内创建独立 `*http.Client` | 默认 Redirect |
| MiaoXiang | 构造器内创建独立 `*http.Client` | 禁止自动 Redirect |

结论：
- 当前各 Provider 构造器分别创建 Client，没有共享 Transport/连接池的组合根注入点；未发现 Provider 对这些 Client 提供 Close 或统一 CloseIdleConnections。
- 这些 Client 使用 Go 默认 Transport（除 Client 级 Timeout/CheckRedirect 外未显式配置 Transport），因此连接复用由各自默认 Transport 独立管理；不同 Provider 之间不能共享连接池，也无法由 Root 统一控制空闲连接回收。
- 仓库已有 `internal/platform/httpclient.New`，具备受控 Dial、目标地址解析策略、并发限制、响应体上限和重试等能力；但本轮检查的 Data Provider 构造器尚未使用该平台 Client。M4 接入必须逐 Provider 对照请求语义、超时、重试和响应上限，不能简单替换 `http.Client`。
- 当前不宜在 M0 直接把 Client 改造成全局单例。目标应是由 Data Bootstrap 创建受控 Client/依赖，并按依赖目标与策略注入 Provider；共享 Transport 的边界、限流配额和关闭 owner 应显式定义。

#### 3. TCP Provider 与资源所有权

**BaoStock**
- 独立入口只创建一个 `baostock.Client`，注入 `provider.NewBaostock`；入口 defer 调用 Close。
- Client 内部以容量为 1 的 gate 串行化 socket 请求，持有单个 TCP 连接和 Reader；断连/协议错误会丢弃连接，后续请求重新 Dial/Login。
- Dial 使用 ConnectTimeout，协议读写使用 RequestTimeout 与 Context deadline；Close 最多等待 4 秒获取 gate，执行 logout 后关闭 socket。
- 该 Client 是长生命周期、串行复用连接；Root Bootstrap 迁入时应保留单实例和串行语义，不能让每个 UseCase/请求重复创建，也不能让多个 owner 重复 Close。
- TCP Host/Port 来自配置，当前未发现目标地址 allowlist/私网地址策略。需纳入平台 TCP 出站目标治理或明确可信上游配置。

**Pytdx**
- Provider 不持有长连接；每次日线/复权查询通过 `dailyClient()` 创建 TDX Client，并在调用路径 defer Close。
- 每次 Dial 最多轮转 6 个 Host，单 Host Dial timeout 上限 1.5 秒；Client 请求 Timeout 使用 Provider Timeout。
- Provider 实例内部用 rateMu 同步 Host index 和 150ms 最小请求间隔；连接生命周期属于单次调用，不是可由 Root 关闭的常驻资源。
- 迁移时需保留按调用创建/关闭的语义；若未来改为连接池，必须另做并发、重连、取消和 Close 设计，不能当作无行为变化的优化。
- Pytdx Hosts 来自配置或库内默认列表，当前未发现对自定义 Host 做目标地址授权。

#### 4. Redis Cache 与 Worker 资源补充

- `cache.Cache` 接口只有 Get/Set/Ping，没有 Close；当前 Redis Adapter 每条命令都新建 TCP 连接、执行 AUTH/SELECT/命令后立即关闭，不维护长连接池。因此不存在一个可关闭的共享 Redis Pool，但会产生每次操作 Dial/AUTH/SELECT 开销。
- Root 若改为注入平台 Redis Client，应保留 key、TTL、错误降级和禁用语义；同时需明确 Client owner，避免 Data Cache 关闭 Root borrowed Client。
- Data 入口启动 Quote Retention Worker 后没有保存 Stop/Wait 句柄；Worker 生命周期仍未满足 Root 统一 Shutdown 的要求。该项需在 M2/M4 提取 Bootstrap 时解决。
- MariaDB Store 有明确 Close，当前独立入口 defer 关闭；迁入 Root 后应改为 borrowed Pool，不允许 Store 关闭 Root 持有的共享 DB。

#### 5. 本轮结论与 M0 状态

- [x] Config→Provider 构造器映射已核对（仅源码默认配置和装配链）。
- [x] HTTP Provider 独立 Client 创建、默认 Transport/Redirect 差异和缺少统一 Close 已核对。
- [x] BaoStock 长连接与串行 gate、Pytdx 单次调用连接生命周期已核对。
- [x] Redis 当前为逐命令短连接、Cache 接口无 Close 已核对。
- [ ] 生产部署环境变量与 Compose/Nginx 实际覆盖值尚未完成对照。
- [ ] HTTP 双 Host Redirect、凭据隔离、目标 IP/端口拒绝测试尚未执行。
- [ ] BaoStock/Pytdx TCP 目标地址限制、协议边界和取消行为仍需测试验证。
- [ ] Retention Worker Stop/Wait 生命周期尚未实现；当前仅记录为迁移阻断项。

**本轮结论：** 当前 Data Provider 的 HTTP Client 彼此独立，尚未接入已存在的 Platform HTTP Client；BaoStock 是单实例长连接且显式关闭，Pytdx 是按调用创建并关闭；Redis 当前不是连接池而是逐命令短连接。M0 已补齐主要配置装配与资源模型的静态证据，但安全行为测试、部署覆盖值和 Worker 生命周期仍未闭环，不能准入 M1。


### 2026-11-06：M0 第二十一轮（部署基线、Worker 生命周期、平台 HTTP 契约、TCP 协议）

审计基线：`02fd53f61bffdafae9df4a668e3191fcdfc49fe5`。本轮批量核对根 Compose、可选迁移 Compose、部署脚本、Data 入口 Worker、BaoStock 协议实现和 Platform HTTP Client 及其测试；未修改业务实现。

#### 1. 部署配置与真实迁移基线

**根目录 `docker-compose.yml` 当前仍是三容器部署：**
- `stock-analysis-data` 单独构建 `services/market-data`，监听 8091。
- `stock-analysis-server` 单独构建根 Go Server，通过 `MARKET_DATA_BASE_URL=http://stock-analysis-data:8091` 访问 Data，并依赖 Data healthy。
- `stock-analysis-web` 单独部署，分别配置 Market Data / Analysis 上游服务名。
- Data 与 Server 都读取根 `.env`；Compose 显式覆盖 Data HTTP Addr、API Key、Redis Addr/Password/DB、AutoMigrate，并将相同 API Key 注入 Server。Provider 其余配置由根 `.env` 提供。
- 根 Compose 依赖外部 `1panel-network`，Data 与 Server 仍然是独立容器和网络调用。

**可选 `docker/docker-compose.go-migration.yml`：**
- 是 Python Bridge + go-core + 独立 market-data 的历史迁移 Profile。
- Data 读取 `services/market-data/.env`，go-core 读取 `go-core/.env`；该文件明确要求不要与根 Compose 混用。
- 该文件不能作为方案 B 已部署的证据。

**`scripts/deploy-go.sh`：**
- 固定使用根 `docker-compose.yml`，先校验 Compose 配置，再构建并启动 Data、Server、WebUI 三个服务。
- 仍检查旧 `stock-analysis-go` / `stock-market-data` 容器并要求先清理；最终检查 8092 的 healthz/readyz。
- 因此方案 B 完成后必须同步修改根 Compose、部署脚本、健康检查、容器迁移说明和 Web Nginx upstream；只改 Go composition 不足以完成部署迁移。

**部署迁移差异清单：**
1. Data 不再独立构建/启动，Provider 与 Data Application 由 Root Server Composition Root 创建。
2. Server 内部调用改 InProcess，移除对 `MARKET_DATA_BASE_URL` 的内部 self-HTTP 依赖；保留 Data HTTP Adapter 仅供外部/兼容入口。
3. 根 Compose 移除 Data 独立服务、depends_on 和服务间 API Key/URL 注入；保留 WebUI 对外路径和兼容路由。
4. `scripts/deploy-go.sh` 的构建目标、容器状态、日志、健康检查和旧容器迁移逻辑一并调整。
5. 可选 go-migration Profile 应明确废弃或重写，避免继续启动第二套 Data。
6. 对比根 `.env` 与旧 `services/market-data/.env`，逐变量迁移 Provider、DB、Redis、API Key、TZ 和 AutoMigrate 配置；禁止覆盖用户真实密钥。

#### 2. Retention Worker 的实际退出竞态

入口在 Store 创建后调用 `runQuoteRetentionWorker(ctx, store, ...)`，该函数内部启动匿名 goroutine 并立即返回：
- 首次 purge 在 goroutine 内执行；后续每 24 小时 ticker 执行一次。
- 每次 purge 使用父 ctx 派生 30 秒 job context。
- Worker 只监听父 ctx 取消，没有返回 done channel、Stop 或 Wait API。
- 主流程收到 ctx.Done 后只等待 HTTP Server Shutdown，随后 run 返回，defer 顺序关闭 BaoStock、Store；没有等待 Worker goroutine 退出。

**结论：存在真实的资源关闭竞态。** 若退出时 Worker 正在调用 `Store.PurgeQuotesBefore`，主流程可能先关闭 Store。虽然派生 job context 会收到取消，但数据库调用不保证在 Store.Close 前立即退出。

M2/M4 必须将 Worker 改为可管理对象或显式 Start/Stop/Wait：
- Start 由 Bootstrap 持有；
- Stop 取消 Worker context；
- Wait 等待当前 purge 完成并返回退出错误；
- Root Shutdown 顺序应先停止接收新请求/停止后台 Worker，再等待 Worker 和 HTTP 请求退出，最后由 Root 关闭 DB；
- Worker 只借用 Store，不得自行 Close Store；
- 为 purge 增加可注入时钟/执行器测试，覆盖启动立即执行、周期触发、取消中断、正在执行时 Shutdown 等情况。

#### 3. Platform HTTP Client 契约边界

已确认 `internal/platform/httpclient.Client` 的可观察契约：
- 每次 Do 必须提供 DestinationPolicy，AllowedHosts 必填且 exact-match；不支持通配符。
- 默认只允许 HTTPS；HTTP 必须通过 AllowHTTP 显式允许，并受 AllowedPorts 限制。
- 禁止 UserInfo、自定义 Host Header、未授权 Host/Port；域名解析后拒绝任何非公网 IP（包含 DNS 混合公网/私网结果）。
- 自定义 Dial 使用已校验 IP，保留原始请求 Host；新建连接时重新解析，避免 DNS rebinding。
- 默认禁止 Redirect；显式允许时最多 3 次、每次目标仍须符合策略，拒绝 HTTPS 降级和循环 URL。
- 默认超时 15 秒、Dial 5 秒、Response Header 8 秒；并发默认 4，可按 dependency 单独配置。
- 默认响应体上限 2 MiB，最大配置上限 16 MiB；读取后关闭原始 Body，再以缓冲 Body 返回。
- 默认重试关闭；仅 policy.Idempotent=true 且请求 Body 可重放时启用，最多 2 次；只对传输错误、502/503/504 重试，间隔 100ms/200ms。
- 注入 Request ID；提供 CloseIdleConnections；不读取 HTTP_PROXY/HTTPS_PROXY 环境代理。

**与现有 Provider 的不兼容点：**
- Tushare 是 POST 且 Token 在 JSON Body；MiaoXiang/TickFlow 等也有 POST 请求，必须为请求体重放显式构造 GetBody，并审慎标记幂等；不能因 HTTP Method 为 POST 就默认重试。
- Eastmoney 存在 POST 热榜和自定义 Redirect 行为；Hithink/Tushare/TickFlow/Xueqiu 默认 Redirect；Tencent/Yahoo/MiaoXiang 禁止 Redirect。接入策略必须逐 Endpoint 定义，不能一刀切。
- 现有 Provider timeout 有 10/12/15/20 秒等差异；Platform Client 采用 Client 全局 Timeout，需通过依赖级实例/请求 context 设计保留原 timeout 语义，避免统一 15 秒导致行为漂移。
- 响应体上限各 Provider 当前不同（MiaoXiang 2 MiB 成功/4 KiB 错误、BaoStock 64 MiB 协议帧等）；Platform HTTP 默认 2 MiB 可能截断现有大响应，需要按 Provider 证据配置，且不可超过平台 16 MiB 上限。
- Platform Client 每次 Do 将响应完整读入内存后返回；不适合未经评估地承接大文件/流式下载接口。
- Provider 现有重试/限速有调用点差异；平台重试只能对确认幂等的请求开启，避免双层重试放大请求次数。

**建议接入方式：** M4 由 Data Bootstrap 创建 Platform Client 及按 Provider/Endpoint 定义的 DestinationPolicy，再通过依赖接口注入 Provider。请求构造、签名/凭据、解析、业务级重试/限速仍由 Provider 负责；Transport、DNS/目标校验、请求并发、响应体上限、基础观测由 Platform Client 负责。先做一个低风险 GET Provider 的适配试点并加 Request Golden，再批量迁移；不要直接替换全部 `*http.Client` 字段。

#### 4. BaoStock TCP 协议边界

协议实现静态确认：
- 请求帧 Body 上限 64 MiB，头部固定 21 字节，包含协议版本、消息类型和长度；请求尾部附 CRC32。
- 响应头解析后先校验声明长度不超过 64 MiB。
- 压缩响应先按声明长度读取压缩体，再校验 CRC Trailer 的数字格式、CDATA 终止符，zlib 解压后再次限制解压结果不超过 64 MiB。
- 非压缩响应按 CDATA 终止符读取，累计大小超过 64 MiB 即拒绝；随后校验 CRC32。
- 现有注释说明压缩 type-96 的 CRC 数值与 CRC32(header || compressedBody) 不匹配，因此目前只验证 Trailer 格式，依赖 zlib checksum 验证压缩内容；非压缩响应仍执行严格 CRC 校验。

**风险与迁移要求：**
- 64 MiB 是单帧硬上限，不是业务响应大小预算；Root 需评估高并发下的内存占用，保留压缩前/解压后双重限制。
- 压缩帧按 Header 声明长度分配内存，虽有上限，但最大仍可分配 64 MiB；建议后续以实测业务数据确定更精细上限，不能在本轮擅自降低。
- 非压缩响应不以声明长度作为边界，而以 CDATA 结束符为准；该行为是为兼容服务端 UTF-8 错误文本长度差异，回归测试需覆盖。
- BaoStock 通过 socket deadline + Context.AfterFunc 实现取消；连接读写失败后丢弃连接，后续重新登录。
- TCP Host/Port 仍需目标地址白名单/可信配置策略；Platform HTTP Client 不适用于 TCP Provider，需单独的 TCP Dial Policy。

#### 5. Pytdx TCP 取消与重连边界

- Context 覆盖 Host 轮转 Dial 和 150ms rate wait；单 Host Dial timeout 不超过 1.5 秒，最多尝试 6 个 Host。
- 建立 TDX Client 后只调用 `client.SetTimeout(p.timeout)`；后续 GetQuote/GetKlineDay/GetGbbq 是同步调用，没有将每次请求的 ctx 直接传入底层协议调用。
- 因此取消 Context 可阻止尚未开始的 Dial/限速等待，但不能保证中断已经开始的 TDX 网络请求；在途请求主要依赖 Client Timeout。
- 每次调用结束 defer Close；TDX Client 配置 WithRedial，底层可能重新 Dial，仍须确保重连使用同一组受控 Host 和超时。
- 配置 Host 当前是字符串列表，允许 host:port 或默认端口 7709；没有在 Config.Load 进行主机名/端口授权。

迁移要求：保留当前每次调用独立 Client 和 defer Close；若要实现强 Context 取消，需先确认底层 tdx/ios API 是否可注入可取消 Conn 或 deadline，不应在 M0 直接更换协议库。补充测试覆盖取消发生在 Dial 前、Dial 中、请求读写中、重连中，以及关闭失败。

#### 6. 本轮结论与状态调整

- [x] 根 Compose 三容器部署基线与可选旧迁移 Profile 已核对。
- [x] 部署脚本的构建、启动、健康检查和旧容器约束已核对。
- [x] Retention Worker 启动/退出和 Store 关闭顺序已核对，确认存在缺少 Wait 的关闭竞态。
- [x] Platform HTTP Client 的目标校验、Redirect、重试、并发、超时、响应体和 Close 契约已核对。
- [x] BaoStock 帧大小、压缩/非压缩读取、CRC 和取消方式已核对。
- [x] Pytdx Context 覆盖边界和同步请求 Timeout 行为已核对。
- [ ] 生产部署 .env 的真实变量值未读取（本轮只审计仓库模板/Compose，不读取用户服务器密钥）。
- [ ] Provider 级 HTTP 双 Host/凭据隔离/重定向和响应大小行为测试仍未执行。
- [ ] BaoStock/Pytdx TCP 自定义目标策略和取消行为的测试覆盖仍未完整核验。
- [ ] Worker Stop/Wait 代码改造与测试尚未实施，属于迁移实现阶段工作，不是文档审计可关闭项。

**M0 状态：** 静态审计范围已明显收敛，但仍有 Provider 请求级 Golden/安全行为核验与 OpenAPI 参数/错误契约等未闭环项。Worker Stop/Wait、平台 HTTP Client 接入、TCP 出站策略属于 M2/M4 的实现任务，不应错误地作为 M0 必须先完成的代码改造。M0 暂不准入 M1，下一轮应优先批量完成剩余 OpenAPI 参数/错误契约与 Provider Request Golden 覆盖矩阵，并把可后置的运行时改造明确移入对应 M 阶段。


### 2026-11-06：M0 第二十二轮（OpenAPI Operation / Handler 契约核对）

审计基线：`d418e42b6d51950db6e4f78108c20d5b923a117f`。本轮对 `services/market-data/openapi.yaml`、`internal/httpapi/server.go`、主要 Handler、DataService 错误分支和现有 OpenAPI contract tests 做逐 Operation 静态核对。只更新迁移审计文档，不改 OpenAPI/Handler/测试代码。

#### 1. 路由与认证契约

- OpenAPI 当前共有 35 个 Operation：2 个公开 Health + 33 个业务 Operation。
- Server mux 当前注册 35 条路由；2 个 Health 公开，其余 33 个都包裹 `s.auth`，认证实现同时读取 `X-API-Key` 和 `Authorization: Bearer`，两者都与同一个 Data API Key 做常量时间比较。
- OpenAPI 根级 `security` 同时列出 `ApiKey` 与 `BearerAuth` 两个独立 Security Requirement Object。OpenAPI 语义是 OR，不是 AND。因此继承根级安全声明的业务 Operation 会被文档描述为同时支持 X-API-Key 或 Bearer Token；Server 实际也支持两种请求头形式，但 11 个 Operation 显式设置 operation-level ApiKey 后覆盖根级声明，造成 Operation 之间认证声明不一致。
- 其中 11 个 Operation（market/quotes、data-sources/probe、diagnostics、price-limit-rule、trading-status、index-catalog、indices/{symbol}/constituents、indices/{symbol}/daily、indices、session、capital-flow，按当前文件逐项显式声明）使用 operation-level ApiKey 覆盖根级配置；其余业务 Operation 继承根级 ApiKey OR BearerAuth 声明。
- `/health/live` 与 `/health/ready` 使用 `security: []`，与 Server 一致。
- `/api/v1/data-sources/diagnostics` 是唯一未声明 401 响应的业务 Operation；Server auth 对缺失/无效 API Key 会返回 401。其 security 本身已声明 ApiKey，因此这是响应文档缺失，不是认证绕过。
- 现有 `TestOpenAPIDataRoutesDeclareValidAPIKeySecurity` 只对固定的少数路径检查 operation-level ApiKey，没有验证根级 Bearer 声明，也没有要求每个受保护 Operation 都声明 401；所以未发现上述问题。

**契约修正建议（需进入 MDM-001 实施，不在本轮直接改）：**
1. 保留根级 `ApiKey OR BearerAuth`，移除业务 Operation 上不必要的 ApiKey 覆盖，使 33 个受保护 Operation 统一继承相同认证能力；Health 继续显式 `security: []`。
2. 给 diagnostics 补 401。
3. 将安全测试改为从 OpenAPI paths 自动枚举 33 个业务 Operation，与 Server 路由集合比对：每个受保护 Operation 必须要求 ApiKey 且声明 401，两个 Health 必须 security=[]。

#### 2. Error Schema 与实际错误体

- `components.schemas.Error` 已定义 `error.class/code/message/retryable/request_id?`，字段与 Server `errorBody` 的 JSON 结构基本一致；但整个 OpenAPI 文件没有任何 `$ref: '#/components/schemas/Error'`，35 个 Operation 均未在错误响应 content 中声明该 Schema。
- Handler 的 `writeErr` / `writeDomainErr` 都通过统一 `errorBody` 输出 JSON；因此当前客户端只能从描述文本推测错误结构，OpenAPI codegen 无法获得统一错误类型。
- `writeDomainErr` 当前映射：validation→400、无启用 Provider→503、deadline→504、canceled→408、其他 Provider 错误→502；认证 401 由 auth middleware 返回。Error.class 由 HTTP status 映射。
- OpenAPI 大部分业务 Operation 已声明 400/401/502/503/504 的子集，但没有统一错误响应组件；408（请求取消）和序列化失败 500 也未形成统一响应描述。
- 修正时建议定义可复用 `ApiErrorResponse`（或保留 Error 命名并明确它是 response schema），在 400/401/404/408/409/429/500/502/503/504 等确实可能出现的响应 content 中引用；不要给每个 Operation 粘贴重复 JSON Schema。

#### 3. 已确认的响应状态码缺口

以下缺口由 Handler 调用 `writeDomainErr`，且 DataService 明确返回 `no enabled ... provider` 时映射为 503；当前 OpenAPI 未声明对应 503：

| Operation | 代码证据 | OpenAPI 当前响应 | 缺失 |
|---|---|---|---|
| GET `/api/v1/quotes/{symbol}` | Quote 返回 `no enabled quote provider` | 200/400/401/502/504 | 503 |
| GET `/api/v1/bundles/{symbol}` | Bundle 下游 Quote/DailyBars 无 Provider 可用时走 domain error 映射 | 200/400/401/502/504 | 503 |
| GET `/api/v1/stocks/{symbol}/announcements` | StockAnnouncements 返回 `no enabled stock-announcement provider` | 200/400/401/502 | 503 |
| GET `/api/v1/market/sentiment` | MarketSentiment 委托 MarketBreadth；无 Provider 时返回 `no enabled market-breadth provider`，外层错误包装仍包含该语义 | 200/400/401/502 | 503 |
| GET `/api/v1/market/dragon-tiger` | DragonTiger 返回 `no enabled dragon-tiger provider` | 200/400/401/502 | 503 |

另外：
- diagnostics 缺少 401 已在认证节记录。
- 所有调用 `writeDomainErr` 的 Operation 在请求取消时可能返回 408；当前各 Operation 未声明 408。需要确认项目是否将 408 作为稳定对外行为，若保留则纳入共享错误响应；若决定取消请求只在内部中断，则需调整 Handler 映射，而不是文档假装不会发生。
- 502/503 的含义应区分“已配置 Provider 全部失败”和“没有可用 Provider”，不要统一描述成 Provider unavailable。

#### 4. 参数与运行时校验对照

已核对 33 个业务 Operation 的参数名和主要 Handler 读取点。未发现路由参数名与 Handler 参数名整体错位；已显式声明的参数名称基本一致。重点约束如下：

- `/api/v1/market/quotes`：symbols 必填；最多 50 个、不可重复、必须为合法 A 股代码。OpenAPI 描述写了最多 50，但 Schema 未表达数量/格式约束（因为它是逗号分隔字符串，需用描述或 pattern/maxLength 表达）。
- `/api/v1/market/valuations`：symbols 必填，最多 100 个原始 token、禁止空 token、仅六位 A 股代码（可带 SH/SZ/BJ 后缀）；Schema 当前只定义 string + example，约束主要依赖 description。
- `/api/v1/stocks/search`：q 必填，1–64 字符；limit 1–50，默认 10；OpenAPI 与 Handler 一致。
- `/api/v1/stocks/{symbol}/announcements`：start_date/end_date 可选，缺省为最近 7 个中国日历日；最多 366 天；page 1–1000，page_size 1–100，默认 1/20。参数 Schema 已表达分页边界，日期跨度只在 description。
- `/api/v1/market/sectors`：type、limit、direction、sort_by 的 enum/range/default 与 Handler 一致。
- `/api/v1/market/stocks`：page 1–10000、page_size 1–100、include_ma bool、adjustment enum 与 Handler 一致。
- `/api/v1/market/dragon-tiger`：page 1–1000、page_size 1–500；date 可选。当前 Schema 有分页范围，但缺少明确 date 采用 Asia/Shanghai 且未来日期拒绝的参数级说明（整体 description 仅说明默认当日）。
- `/api/v1/market/limit-pools`：type enum、limit 1–100、date 默认当日；Schema 已列 enum/range/default，但 required=false 未显式写出（OpenAPI 默认 optional，语义不冲突）。
- `/api/v1/market/capital-flow`：start/end 必填，最多 2000 日；描述写了范围上限，参数 Schema 仅 format=date。
- `/api/v1/market/sentiment/history`：start/end 必填、limit 1–500 默认 366；Handler 另限制日期跨度 366 日，Schema/description 未明确该跨度约束。
- `/api/v1/market/trading-calendar` 与 `/api/v1/market/indices/{symbol}/daily`：start/end 必填，Handler 有日期跨度上限；OpenAPI 参数只表达 date 格式，描述没有把具体上限写清楚。
- `/api/v1/market/indices/{symbol}/constituents`：as_of 是 YYYY-MM-DD 日期，不是 RFC3339 datetime；当前 Schema format=date 与 Handler 一致。
- `/api/v1/stocks/{symbol}/price-limit-rule` 与 `trading-status`：date 必填且唯一、不能晚于中国市场当日；trading-status 的 as_of 是 RFC3339 datetime。Schema 对 format 有表达，但重复参数和未来时间限制仍由 Handler 执行。
- Provider Probe Body Schema 已覆盖 Scope、Markets、Symbols、Checks、Capabilities、DailyBar adjustments/window、timeout；与 Handler 的字段集合基本一致。Handler 限制 JSON Body 最大 8 KiB、必须恰好一个 JSON object、拒绝未知字段；OpenAPI 当前未描述 8 KiB Body 上限。数组 maxItems/uniqueItems、timeout 1–30 已表达。

参数审计结论：目前主要是“运行时约束已存在、OpenAPI Schema 约束表达不足”，没有证据表明这些校验本身失效。MDM-001 应优先把稳定边界显式化；对依赖中国交易日/时区、重复 query key、跨字段约束的规则，用 description 明确说明，不能伪造 OpenAPI 无法精确表达的约束。

#### 5. 现有 Contract Test 覆盖缺口

当前 `openapi_contract_test.go` 包含 4 组测试：
1. YAML 可解析、ApiKey scheme 名称、固定路径 security、部分 Schema 关键字段。
2. ProviderProbe validation 字段存在。
3. SectorRanking.change_percent_5d 字段存在。
4. 部分 Data Model JSON 必填/省略/null 语义与 OpenAPI 的人工列举字段一致。

当前没有：
- OpenAPI 35 Operation 与 Server 35 路由的自动集合比对；
- 根级 security 与 operation-level security 的语义校验；
- 所有受保护 Operation 的 401 和统一错误 Schema 引用校验；
- 所有 Operation 参数名/required/type/enum/range 与 Handler 的契约矩阵；
- 每个 Operation 的 200 JSON Golden；
- 各 Operation 400/401/404/502/503/504 的错误 JSON Golden；
- Handler 实际返回状态码集合与 OpenAPI responses 的自动比对。

建议分层补测试：先做路由/认证/响应状态集合静态契约测试，再做参数边界测试，最后按高风险 DTO 和错误分支补 Handler Golden；避免一次性为 35 个 Operation 写重复的大型快照。

#### 6. 本轮结论与 M0 状态

- [x] 35 Operation / 35 Server 路由集合数量已复核。
- [x] 认证实际同时接受 X-API-Key 和 Authorization Bearer；发现 11 个 operation-level ApiKey 覆盖导致认证声明不一致。
- [x] 发现 diagnostics 缺少 401 文档响应。
- [x] Error Schema 与实际结构基本匹配，但完全未被 Operation 错误响应引用。
- [x] 发现 5 个 Operation 漏声明可能返回的 503。
- [x] 主要参数名及 Handler 读取点已核对；记录 Schema 未表达的边界约束。
- [x] 现有 Contract Test 覆盖范围和缺口已确认。
- [ ] 逐 Operation 完整响应状态码矩阵仍需把剩余 Handler 的具体分支补齐。
- [ ] OpenAPI 200 DTO 与 Go JSON 的全量字段/nullable/required 自动交叉核对仍未完成。
- [ ] Provider Request Golden 仍需单独核查 Provider HTTP 请求行为和测试覆盖。
- [ ] MDM-001 契约修正尚未执行；本轮没有获得修改 OpenAPI、Handler 或测试代码的授权。

**本轮结论：** M0 已找到明确且可定位的 OpenAPI 契约问题：11 个 operation-level ApiKey 覆盖与根级 OR 不一致、diagnostics 缺少 401、5 个 Operation 缺少 503、统一 Error Schema 未被引用，以及多项运行时参数约束没有进入 Schema/description。它们应作为 MDM-001 的契约修正输入。当前仍保持审计模式，不直接修改 API 契约或实现。


### 2026-11-06：M0 第二十三轮（Provider Request Golden 覆盖审计）

审计基线：`3076ab76262dee0ab2d08539ad4db38823d7c2c0`。本轮检查 Provider 测试文件清单，并抽查 12 个 Provider 的请求构造/HTTP mock 测试。这里的“Golden”指测试固定请求 Method、URL/Query、Header、Body、重定向与响应语义；仅验证解析结果或调用成功不算完整 Request Golden。未运行测试，也未修改 Provider 代码。

#### 1. 现有测试覆盖矩阵

| Provider | 已有请求级证据 | 当前判断 | 主要缺口 |
|---|---|---|---|
| Eastmoney | quote/Kline Query、UT、profile Referer/Query、sector ranking Query、index quote Query、stock-list 分页 Query、hot ranking Endpoint 配置；有外部 Redirect 拒绝与临时 502 重试测试 | 相对最完整，但仍非全调用族 Golden | Hot ranking POST Body、capital-flow、dragon-tiger、announcement、limit-pool、market breadth 等请求字段需按调用族补齐；Redirect 目前只覆盖部分策略，不是 301/302/303/307/308 全矩阵 |
| Tushare | 日线 POST 方法和 JSON Body、复权/单位解析；市场资金流、复权因子、证券状态等有专用 Provider 测试 | 业务响应覆盖较多，通用请求契约不足 | Token Body 字段、API Name/Fields/Params、所有 POST 调用族和 endpoint override 未形成统一请求 Golden；跨 Host 307/308 与凭据不外泄未覆盖 |
| Hithink | 日线 X-api-key Header、分页 offset；热榜/指数/财务等有独立业务测试 | 局部请求字段已验证 | 各 API endpoint、Query、分页终止条件、Header 与重定向组合缺少完整矩阵 |
| TickFlow | Quote X-API-Key、日线/指数/板块/市场宽度等有独立测试 | 能力级测试较多，通用 HTTP 请求 Golden 不完整 | GET/POST 请求分支、Free API 限速、API Key 隔离、重定向与响应体边界未统一覆盖 |
| MiaoXiang | POST、apikey Header、JSON Body、响应解析、缓存避免重复请求、无效 Key/Endpoint 测试 | 当前最明确的请求契约测试之一 | 全局 TryLock 并发跳过、300ms 限速边界、HTTP 失败不缓存、超大响应/错误体上限、取消和 Redirect 需补测试 |
| Xueqiu | 热榜 Query（size/type）、响应字段与排序校验 | Query 级局部覆盖 | Cookie Header 精确值、敏感 Header 重定向隔离、错误码、timeout/响应体边界未覆盖 |
| Tencent | Quote/Kline Query、调整接口参数、Referer/响应解析及日期边界测试 | 请求参数覆盖较好 | 禁止 Redirect、超时/取消、响应上限与 endpoint override 的显式断言不足 |
| Sina | Quote/Kline Query、Referer、历史 datalen、编码和响应解析测试 | 请求参数覆盖较好 | 禁止/允许 Redirect 策略、timeout/取消、响应上限和 endpoint override 需补 |
| NetEase | Kline Query、Quote callback、Referer、CSV/JSONP 解析 | 请求参数覆盖较好 | Redirect、timeout/取消、响应大小上限、endpoint override 需补 |
| Yahoo Finance | Chart Query（range/interval 等）、Ticker/调整口径及响应解析 | Query 级局部覆盖 | 禁止 Redirect、跨 Host 凭据隔离、timeout/取消、响应上限和 endpoint override 需补 |
| BaoStock | 协议帧、日线、交易日、复权因子、错误和数据规范化测试 | 主要是协议/数据语义测试 | TCP 请求帧字段 Golden、长度/CRC/压缩异常、断线重连、并发 gate、Close 等待和取消行为需形成独立协议矩阵 |
| Pytdx | 日线 Client 创建/关闭、复权因子失败、金额与交易所映射测试 | 主要是调用/数据语义测试 | Host 轮转顺序、Dial timeout、每次调用 Close、重连目标、150ms rate limit、请求中取消不生效边界需要 fake dialer/协议层测试 |

#### 2. 当前不能称为完整 Request Golden 的原因

1. Provider 测试主要围绕业务能力和数据质量组织，缺少按 Endpoint/调用族固定 Method、完整 Query、Header、Body 的可复用断言层。
2. Provider 的 HTTP Client 创建方式、Redirect policy、Timeout、响应体上限分散在构造器；测试没有统一验证这些传输行为。
3. 现有 Eastmoney Redirect 测试覆盖自定义策略的可信 HTTPS Host 与外部目标拒绝，但不能替代其他 Provider 的跨 Host 凭据隔离测试；也不能推导 Tushare Body Token 在 307/308 下安全。
4. 多数 httptest 只校验请求的一两个关键字段，不能证明未校验字段没有漂移；建议采用按调用族的 Query/Body Golden，而不是只断言某个参数非空。
5. TCP Provider 不应套用 HTTP Golden；BaoStock 应按协议帧字段/CRC/压缩体，Pytdx 应按 Host 轮转和调用生命周期分别建立 fake transport 测试。

#### 3. M0 剩余 Request Golden 的最小闭环

按风险和收益排序，先补以下集合，避免对全部 Provider 做无差别重写：

**P0：认证信息与请求目标**
- Tushare：POST JSON 的 api_name、token、params、fields；断言 Token 不进入 URL/日志；对 301/302/303/307/308 的 Body/Method 行为建立测试。
- Hithink/TickFlow：X-api-key / X-API-Key Header 名称和值、Endpoint host、Query/Body 精确字段。
- Xueqiu：Cookie Header 的存在、精确作用域和跨 Host 不外泄。
- MiaoXiang：apikey Header、POST Body、禁止 Redirect、缓存/全局并发语义。

**P0：Eastmoney 关键调用族**
- 热榜 POST Body：appId/globalId/marketType/pageNo/pageSize。
- 资金流：secid/fields1/fields2/klt/lmt/end/ut。
- 龙虎榜：reportName/columns/filter/sort/page/source/client。
- 公告：sr/page_size/page_index/ann_type/client_source/f_node/s_node/begin_time/end_time/stock_list。
- 涨跌停池：日期、类型、分页、字段集及 provider report date 语义。
- 每个调用族固定 endpoint、method、完整 query/body、关键 Header；分页需覆盖第一页/中间页/终止页。

**P1：HTTP 传输行为**
- 同 Host 与跨 Host 301/302/303/307/308；HTTP→HTTPS 与 HTTPS→HTTP。
- 有凭据与无凭据两类请求；跨 Host 不允许携带 API Key、Cookie 或 Body Token。
- timeout、context cancel、429/5xx、重试次数/间隔、响应体大小边界。
- endpoint override 与默认 endpoint 一致性。

**P1：TCP**
- BaoStock：header/body length、CRC、压缩/非压缩、错误 CDATA、超限、断线后重新登录、并发请求串行和 Close。
- Pytdx：Host 轮转、超时、失败后重连、rate limit、Close、在途请求 Context 取消边界。

#### 4. 本轮判断

- Eastmoney、Tencent、Sina、NetEase 等已有不少请求参数断言，不能笼统评价为“没有请求测试”。
- Tushare、Hithink、TickFlow、MiaoXiang、Xueqiu 已覆盖部分关键凭据/请求形态，但跨 Host 和传输边界证据不足。
- BaoStock/Pytdx 当前测试重点是数据/协议结果，缺少完整连接生命周期和目标安全测试。
- 现有测试文件数量较多，但测试数量不等于请求契约覆盖率；应以 Endpoint/调用族矩阵为覆盖分母。

**M0 收敛建议：** Provider Request Golden 的全量补齐不应阻断 M1 的文档/模块边界设计。M0 可以把当前已证实的请求契约差异记录为迁移基线；涉及凭据泄露、重定向、目标地址的安全回归测试必须在 M4 平台出站 Client 接入前完成。MDM-001 OpenAPI 修正与 M1 Module 合并可并行推进，但当前仍需用户授权后才能修改 OpenAPI/Provider/测试代码。


### 2026-11-06：M0 第二十四轮（新总览与跨模块契约对齐复核）

审计基线：当前分支 HEAD `66a47655ce9d9b9a008729f7aebeb2f71cdfd294`。该 HEAD 是将 `feat/design_doc` 合入 Data 分支后的合并提交；本轮读取了 `doc/整体任务规划/README.md`、项目总览、契约目录与冻结状态、Data OpenAPI/Go 模型修正清单、Data 服务间认证设计、GOV-CTR-006 机器契约、Platform 集成清单和 Market Data 模块化单体方案。只更新本迁移跟踪文档，不修改规划分支文档、OpenAPI、Handler、Model 或测试代码。

#### 1. 最新总体决议与迁移边界

本轮确认项目总览与 Market Data 架构方案一致，方案 B 是当前唯一默认方向：

- Data 迁入根 Go Module 的 `internal/marketdata`，与 Root Server 同进程、同部署；Root Server 是唯一 Composition Root。
- Data 仍拥有 Provider、行情/财务/估值、Security Master、Calendar、Normalization、Quality、PIT、Repository、Schema 和 Migration；不能把 Data 领域代码揉进 Server Application。
- Root/Quant/Research/Researchflow 通过 Data Application Port 消费；内部调用不走 HTTP，不使用 Data API Key，不创建 Server→Data 自调用 HTTP Client。
- Data HTTP Adapter 保留给远程/独立部署兼容；远程模式继续按 X-API-Key 认证。浏览器不得继续持有 Data API Key。
- Data 迁移不改变 v1 API 行为、Provider 算法、数据单位/质量语义、PIT、历史数据和数据库表所有权。
- MDM-001=M0 审计，MDM-002=M1 Module 归并，MDM-003=M2 Port/Bootstrap，MDM-004=M3 Root 装配，MDM-005=M4 共享资源与生命周期，MDM-006=M5 部署/WebUI，MDM-007=M6 稳定性验收；MDM-008 在结构迁移后单独接入 DATA/Platform 公共契约。

**阶段门禁：** 当前 M0 仍未通过，不能开始 M1 的目录搬迁、删除嵌套 go.mod、独立 Dockerfile 或旧部署入口。M0 需要先形成完整迁移对象清单、修正 v1 OpenAPI 已确认的契约描述错误，并通过全量 Contract Test。Provider 请求 Golden 的全量扩展可作为迁移基线和 M4 安全验收输入，不应把所有 Provider 测试重写错误地变成 M1 前置条件。

#### 2. DATA-CTR-005～008 与 GOV-CTR-006 的新边界

项目契约目录当前将 DATA-CTR-005～008、GOV-CTR-006 标为 `pending_evidence`，语义已接受但未达到 frozen；本轮按契约明确区分结构迁移与正式契约接入：

- **DATA-CTR-005：** v1 Quote/DailyBar 继续保持现有 JSON number/float64 兼容形态，不得宣称精确 Decimal。跨模块精度敏感输入首批使用 v2 `GET /api/v2/bars/{symbol}/daily`，Decimal string、显式日期范围/limit/adjustment_basis/purpose_code/system_as_of，返回 PIT、质量、来源、不可变 snapshot_ref 和 authorization_ref。v2 Schema、Go Decimal Model、原始精度链路和 Golden 尚待实施，不能把 v1 DTO 直接冒充 v2。
- **DATA-CTR-006：** 进程内由 Root Server 对外入口认证并注入可信 Principal/Scope Context，Data Application Port 不使用 Data API Key；远程兼容 HTTP Adapter 才继续校验 X-API-Key。Data Key、Owner Token、Platform Service Secret、Provider Token 必须分离。
- **DATA-CTR-007：** Research 首批真实 Data Evidence 只消费单证券 DailyBarsV2，必须保留 PIT、snapshot_ref、authorization_ref、source/version、quality 和单位；Fundamentals、Valuations、TradingStatus、SecurityProfile 动态字段和市场当前快照不属于首批历史 Evidence。
- **DATA-CTR-008 + GOV-CTR-006：** 同进程和远程模式都必须保留授权审计。可信 principal、Scope、purpose_code、请求范围绑定、内容寻址 Snapshot Manifest、AuthorizationRef、读取时重新授权、策略版本竞态 fail-closed 都不能因移除 Data API Key 而省略。AuthorizationRef 不是 bearer token。
- **PLAT-CTR-005/006/007：** Provider 出站安全、Storage/Migration 生命周期、Typed Config/Secret 的公共边界已定义，但登记状态仍为 pending_evidence。M4 只能按实际已合入并验证的 Platform 实现接入，不能仅凭设计文档认定已验收。

本轮没有把 DATA-CTR-005～008 提前塞入 M0/M1，也没有将 MDM-008 与结构迁移合并。

#### 3. 最新 Data OpenAPI / Model 修正提交复核

当前分支 `services/market-data/openapi.yaml` blob 为 `f57886d414d7e78b932044460525b701a73041dc`，`openapi_contract_test.go` blob 为 `f9916e8417461f04e9a81efeb5d92391ea5e9f38`。与《Data OpenAPI 与 Go 模型契约修正清单》记录的 OpenAPI blob `9a15f7810a07c60887a574d82e1f84d56a8c9964` 已不是同一版本，必须以当前分支文件重新核对，不能直接沿用旧清单数字作为当前未修复项。

已确认两次近期修正：
- `0f39b434`（修正数据模型与 OpenAPI 字段契约）：修改 OpenAPI 和 `openapi_contract_test.go`，补入原清单列出的 15 个 Go 字段、调整部分 required、移除 9 个没有 Go 响应来源的 Schema 字段，并增加人工列举式契约断言。
- `3662db4b`（校正行情情绪字段 required 语义）：将 `MarketSentimentSnapshot.source_version` 从 required 移除，并在测试中增加 optional 断言。

这意味着旧清单中的“15 个字段缺失、9 个多余字段、部分 required 差异”已有实际修正，不能继续原样计为全部未完成。当前测试已对这批已知字段做局部保护，但仍不是 Go Model ↔ OpenAPI 的全量双向反射测试，也没有替代真实 Handler JSON Golden。

**本轮发现的剩余差异/文档冲突：**

1. `DragonTigerSnapshot.source_version`：旧字段清单要求 Go 始终输出时应为 required；当前 OpenAPI `DragonTigerSnapshot.required` 未包含 `source_version`，而新加的 `TestOpenAPIDataModelFieldsMatchKnownJSONContract` 的 required 检查列表也漏掉该字段。需复核当前 Go JSON Tag 与 Validator 后补入自动断言。
2. `LimitPoolItem.change_percent`：当前 OpenAPI 将其声明为 `nullable: true`，测试也明确要求 nullable；但《Data OpenAPI 与 Go 模型契约修正清单》第 11.14 节记录 DataService Validator 在 HasData=true 时拒绝 nil、HasData=false 时 Items 必须为空，因此当前可达成功响应不会输出该字段为 null。两份证据冲突，需由 Data Owner 基于当前 Handler/Validator 选择“Schema 非 nullable”或“运行时允许 null”，不能同时保留相反承诺。
3. 《Data OpenAPI 与 Go 模型契约修正清单》仍以旧 OpenAPI blob `9a15...` 和旧代码快照为基线；应把 `0f39b434`、`3662db4b` 的修复纳入清单，重算 Required/Nullable/字段差异，尤其不要再把已修复的字段重复登记为待修。
4. 新测试仍以人工列举的固定字段为主；没有从 35 个 Operation 自动构建 Operation→Handler→Response DTO→Schema 映射，也没有递归比较所有 Go JSON Tag、嵌套对象、required、nullable、enum、format 和真实 JSON。
5. 当前 v1 OpenAPI 仍只有 35 个 Operation；项目契约中定义的 v2 DailyBars 接口尚未出现在该 OpenAPI 文件中。这与 DATA-CTR-005 仍为 pending_evidence、计划在 MDM-008 实施一致，不是本轮 M0 的 v1 兼容接口缺失。

#### 4. 认证契约：纠正“22 条漏声明 ApiKey”的旧结论

本轮重新按 OpenAPI 继承语义解析当前 `openapi.yaml`，结果如下：

- 35 个 Operation；2 个 Health 显式 `security: []`。
- 11 个业务 Operation 显式声明 operation-level `ApiKey: []`。
- 其余 22 个业务 Operation 没有 operation-level security，继承 OpenAPI 根级 `security`。
- 根级 `security` 包含两个独立 Security Requirement Object：`ApiKey: []` 和 `BearerAuth: []`。OpenAPI 语义是 OR，因此这 22 个 Operation **并非没有认证声明，也不是公开路由**；它们被文档描述为支持 X-API-Key 或 Bearer 两种方式。
- Data 当前 Server `s.auth` 同时读取并校验 `X-API-Key` 与 `Authorization: Bearer`，两者使用同一个 Data API Key。故真实问题是 11 个 operation-level ApiKey 覆盖与其余 22 个继承根级 OR 的安全声明不一致，而不是 Bearer 虚假支持。
- 11 个显式 ApiKey Operation 已覆盖 X-API-Key；Health 显式公开。所有 33 个业务 Operation 的实际 Handler 仍执行 X-API-Key 认证。

**需要纠正的旧记录：** 本迁移文档早期 M0 记录及设计方案中“22 条受保护业务 Operation 漏声明 ApiKey”的表述不准确，应以本轮的 OpenAPI 继承语义结果替代。规划分支中的《Market Data并入Root Server模块化单体调整方案》《Data 服务间认证映射与 Secret 注入设计》和契约登记簿也有“业务 Operation 均 operation-level 声明 ApiKey”的过度概括；这些文档由 `feat/design_doc` 维护，本轮不越权修改，只将差异记录在 Data 审计文档中。

**建议的 v1 修正：**
1. 保留根级 ApiKey OR BearerAuth，移除 11 个业务 Operation 上重复且范围更窄的 operation-level ApiKey 覆盖；Health 保持匿名声明。
2. 保留两条 Health 的 `security: []`。
3. diagnostics 补 401。
4. Contract Test 解析 YAML 结构，验证 33 个业务 Operation 的有效 Security Requirement 只有 ApiKey，Health 无认证；同时与 Server mux 路由集合自动比对。不要再以“每个 Operation 必须写一份 operation-level security”作为必要条件。

#### 5. 错误响应契约复核

- 当前 `components.schemas.Error` 仍没有被任何 Operation 的错误响应引用；本轮按当前 OpenAPI 全文检查，35 个 Operation 的 Error Schema 引用数仍为 0。
- 当前 Server `errorBody` 始终输出 `class/code/message/retryable`，`request_id` 可省略；但 OpenAPI Error Schema 内层 required 仍只有 `code/message`。即使后续把 Error Schema 引用到响应，也仍需把实际始终输出的 `class/retryable` 纳入 required，或明确调整运行时编码行为。
- 新增的 `platform_contract_test.go` 已测试 Fault 的 HTTP class 映射、安全 message、retryable 和 Request ID，但 `TestOpenAPIErrorSchemaDocumentsPlatformFaultProjection` 只检查 Schema 文本包含关键字段，没有验证 Operation 错误响应是否引用 Schema，也没有验证 required 与实际 JSON 输出一致。
- diagnostics 仍是唯一缺少 401 响应声明的业务 Operation。
- 当前仍缺少 5 个已确认可返回“无启用 Provider”503 的 Operation 声明：`GET /api/v1/quotes/{symbol}`、`GET /api/v1/bundles/{symbol}`、`GET /api/v1/stocks/{symbol}/announcements`、`GET /api/v1/market/sentiment`、`GET /api/v1/market/dragon-tiger`。
- 408 请求取消和 500 序列化失败仍需按每个 Handler 的真实可达路径确定是否纳入对外稳定契约；不可直接把所有通用状态码无差别添加到全部 Operation。

#### 6. Platform Principal 与 Data 授权的代码边界

当前根 Module 已有 `internal/platform/security`：
- `Principal` 表达 IdentityKind、Subject、Scopes。
- `Authenticator` 使用 Bearer 凭据摘要和常量时间比较；`Guard` 按显式 RoutePolicy 执行 anonymous/owner/service 授权，并把可信 Principal 放入 Request Context。
- `PrincipalFromContext` 可读取可信主体；客户端自报 `X-Identity`/`X-Principal` 不参与身份建立。
- Platform Security 的 Bearer 认证属于 Root Server 新路由边界，不能据此推断旧 Data HTTP Adapter 已支持 Bearer，也不能把 Platform Service Secret 当 Data API Key。
- 本轮未发现 Data Application Port 已消费该 Principal/Scope Context 的集成证据；这属于 MDM-003/004 的装配与 MDM-008 的正式授权契约接入，不能仅因 Platform package 已存在就标记完成。

#### 7. M0 状态与后续执行顺序

本轮关闭/更新：
- [x] 重新读取最新总览、契约目录、Data 认证/DTO 契约、GOV-CTR-006 和模块化单体方案。
- [x] 确认方案 B、MDM-001～008 阶段映射及 Data 领域所有权边界。
- [x] 核对当前 OpenAPI blob 与近期 Schema 修正提交，避免沿用过期字段差异数字。
- [x] 纠正 OpenAPI 根级 security 继承导致的“22 条漏 ApiKey”误判。
- [x] 识别 DragonTigerSnapshot.source_version required 漏项、LimitPoolItem.change_percent nullable 冲突。
- [x] 识别 Error Schema 未引用、required 不完整、diagnostics 401 和 5 个 503 缺口。
- [ ] 全量 Go Model ↔ OpenAPI 双向字段/required/nullable/enum/format 自动差异报告。
- [ ] 35 Operation 的真实 Handler 成功/错误状态码与 Error Body Golden。
- [ ] 所有参数 required/type/default/range 与 Handler 校验规则的逐项复核。
- [ ] 真实部署是否使用旧 go-migration Profile 的现场确认。
- [ ] Data SQL 文件与 Go Migration 的完整列/索引/默认值/字符集 DDL diff。
- [ ] M0 全量验收记录与 MDM-001 契约修正门禁。

**当前结论：** 契约已明确 Data 同进程调用边界和 DATA-CTR-005～008 的后续接入语义；部分 v1 OpenAPI/Go 字段修正已在 Data 分支真实提交，但认证继承、错误 Schema 引用、部分 required/nullable 和错误状态码仍未闭环。M0 继续 `in_progress`，M1 不准入。本轮仍为只读审计，没有修改 API、业务实现、Provider、测试或规划分支文件。


### 2026-11-06：M0 第二十五轮（Handler 状态码闭环与 Migration / Module 归并核查）

审计基线：`b85e1874da80022095ee9dea08e95b50622e1d08`。本轮读取当前 35 条 mux 路由、所有 HTTP Handler、统一错误映射、OpenAPI responses、Repository Migration、SQL DDL、Root/Nested go.mod。仅更新本迁移审计文档。

#### 1. 真实 HTTP 状态码规则（以 Handler 为准）

统一错误出口 `writeDomainErr` 的确定映射：
- 参数/领域输入错误：400 `VALIDATION_ERROR`。
- 无启用能力 Provider：503 `PROVIDER_UNAVAILABLE`。
- 上游超时：504 `PROVIDER_TIMEOUT`，retryable=true。
- 请求 Context 取消：408 `REQUEST_CANCELED`。
- 其他 Provider/上游错误：502 `PROVIDER_BAD_RESPONSE`。
- `writeJSON` 序列化失败兜底为 500 `INTERNAL_ERROR`；该分支是通用防御性兜底，不代表每个响应 DTO 当前都存在可触发的序列化错误。
- 所有 33 个业务路由在 auth middleware 层均可返回 401；两个 Health 路由不经过 auth。
- Health Ready 独立返回 503 `DEPENDENCY_UNAVAILABLE`。
- PIT 查询另有明确的 503（PIT storage disabled）和 404（指定 as_of 没有可用 revision）。
- 批量行情有意将单只股票 Quote 失败放入 item.error_code，整体仍返回 200，不应错误地给该 Operation 加 502/503。

#### 2. 35 Operation 状态码对照（OpenAPI 当前声明与 Handler 可达行为）

| Operation | OpenAPI 当前响应 | Handler 可达状态 / 说明 |
|---|---|---|
| GET /health/live | 200 | 200 |
| GET /health/ready | 200,503 | 200,503 |
| GET /api/v1/market/quotes | 200,400,401 | 200,400,401；单只 Quote 失败内嵌 item，不改变整体状态 |
| GET /api/v1/quotes/{symbol} | 200,400,401,502,504 | 200,400,401,408,502,503,504；缺 408/503 |
| GET /api/v1/bars/{symbol}/daily | 200,400,401,404,502,503,504 | 200,400,401,404,408,502,503,504；缺 408 |
| GET /api/v1/bundles/{symbol} | 200,400,401,502,504 | 200,400,401,408,502,503,504；缺 408/503 |
| POST /api/v1/data-sources/probe | 200,400,401,500 | 200,400,401；500 是 JSON 序列化兜底 |
| GET /api/v1/data-sources/probe/history | 200,400,401,503 | 200,400,401,408,502,503,504；缺 408/502/504 |
| GET /api/v1/data-sources/diagnostics | 200 | 200,401；缺 401 |
| GET /api/v1/data-sources | 200,401 | 200,401 |
| GET /api/v1/stocks/search | 200,400,401,502,503 | 200,400,401,408,502,503,504；缺 408/504 |
| GET /api/v1/stocks/{symbol}/announcements | 200,400,401,502 | 200,400,401,408,502,503,504；缺 408/503/504 |
| GET /api/v1/stocks/{symbol}/profile | 200,400,401,502,503 | 200,400,401,408,502,503,504；缺 408/504 |
| GET /api/v1/stocks/{symbol}/price-limit-rule | 200,400,401 | 200,400,401,408,502,503,504；缺 408/502/503/504 |
| GET /api/v1/stocks/{symbol}/trading-status | 200,400,401,404,502,503 | 200,400,401,404,408,502,503,504；缺 408/504 |
| GET /api/v1/stocks/{symbol}/fundamentals | 200,400,401,404,502,503 | 200,400,401,404,408,502,503,504；缺 408/504 |
| GET /api/v1/stocks/{symbol}/chip-distribution | 200,400,401,502,503 | 200,400,401,408,502,503,504；缺 408/504 |
| GET /api/v1/stocks/{symbol}/capital-flow | 200,400,401,502,503 | 200,400,401,408,502,503,504；缺 408/504 |
| GET /api/v1/stocks/{symbol}/boards | 200,400,401,502,503 | 200,400,401,408,502,503,504；缺 408/504 |
| GET /api/v1/market/valuations | 200,400,401,502,503 | 200,400,401,408,502,503,504；缺 408/504 |
| GET /api/v1/market/sectors | 200,400,401,502,503 | 200,400,401,408,502,503,504；缺 408/504 |
| GET /api/v1/market/index-catalog | 200,400,401,502,503 | 200,400,401,408,502,503,504；缺 408/504 |
| GET /api/v1/market/indices/{symbol}/constituents | 200,400,401,502,503 | 200,400,401,408,502,503,504；缺 408/504 |
| GET /api/v1/market/indices/{symbol}/daily | 200,400,401,502,503 | 200,400,401,408,502,503,504；缺 408/504 |
| GET /api/v1/market/indices | 200,401,502,503 | 200,401,408,502,503,504；缺 408/504 |
| GET /api/v1/market/trading-calendar | 200,400,401,502,503 | 200,400,401,408,502,503,504；缺 408/504 |
| GET /api/v1/market/session | 200,400,401,502,503 | 200,400,401,408,502,503,504；缺 408/504 |
| GET /api/v1/market/stocks | 200,400,401,502,503 | 200,400,401,408,502,503,504；缺 408/504 |
| GET /api/v1/market/capital-flow | 200,400,401,502,503 | 200,400,401,408,502,503,504；缺 408/504 |
| GET /api/v1/market/sentiment | 200,400,401,502 | 200,400,401,408,502,503,504；缺 408/503/504 |
| GET /api/v1/market/sentiment/history | 200,400,401,503 | 200,400,401,408,502,503,504；缺 408/502/504 |
| GET /api/v1/market/breadth | 200,401,502,503 | 200,401,408,502,503,504；缺 408/504 |
| GET /api/v1/market/hot-stocks | 200,400,401,502,503 | 200,400,401,408,502,503,504；缺 408/504 |
| GET /api/v1/market/dragon-tiger | 200,400,401,502 | 200,400,401,408,502,503,504；缺 408/503/504 |
| GET /api/v1/market/limit-pools | 200,400,401,502,503 | 200,400,401,408,502,503,504；缺 408/504 |

说明：
- “Handler 可达”是按统一错误映射和各 Handler 的调用路径做静态推导，不是运行时逐一注入故障所得；后续 Contract Test 需要把这些映射变成可执行断言。
- 上表将 domain error 分支按统一映射列出。对某个能力实际不会出现“无 Provider”或某类错误的，需要由 Service 实现和 Provider 组合再缩窄，不可仅因共用 writeDomainErr 就机械添加无意义响应。
- diagnostics 缺 401 是本轮确认的文档遗漏；此前的“只有 diagnostics 缺 401”结论仍成立。
- 之前只列出 5 个漏 503 的结论不完整：price-limit-rule 等共用 writeDomainErr 的 Operation 还需按实际 Service 能力核验；不应继续把 5 个视为完整差异集。

#### 3. Error Schema / Contract Test 需要落地的最小范围

- 先定义共享错误 Response 组件，将 Error Schema 引用到真实可达的错误状态码。
- Error JSON 的 `class/code/message/retryable` 在当前编码器中始终写出；`request_id` 由 middleware 提供时写出。Schema required 应反映这一点。
- 408 是否作为稳定公开响应保留，需要产品/API 决策；若保留，应为所有真实可达的 domain-error Operation 声明；若不保留，应调整 Handler 对 Context canceled 的 HTTP 映射，不能只删 OpenAPI 声明。
- 500 目前仅是通用 JSON 序列化兜底；建议共享错误组件覆盖 500，但不要把它误判为常规 Provider 故障。
- 503 必须按 Service 的真实能力空集分支确认。对于已明确 no enabled provider 的能力，Operation 必须声明 503。
- 增加基于 mux/OpenAPI 的路由集合、认证继承、响应状态码映射测试；再对 400/401/404/408/502/503/504 错误体做共享 Golden，而不是为每个接口重复快照。

#### 4. SQL DDL 与运行时 Migration 对照

当前 Data 目录只存在一份 SQL 文件 `migrations/001_create_market_data.sql`，而真正运行迁移的入口是 `internal/repository/mysql.go:Store.Migrate` 中的 Go 字符串 DDL，并非读取/执行该 SQL 文件。

已核对两侧基础表集合均为 9 张：
1. market_data_quotes
2. market_data_daily_bars
3. market_data_daily_bar_versions
4. market_data_fundamental_versions
5. market_data_security_trading_status_versions
6. market_data_sentiment_snapshots
7. market_data_provider_probe_runs
8. market_data_adjustment_factor_versions
9. market_data_corporate_action_versions

基础字段与核心索引设计总体同向，SQL 文件不是完整的历史迁移序列，也不是运行时唯一事实来源。运行时 Go Migration 额外承担旧库升级：
- `ensureNonUniqueHistoryIndex` 检查并删除旧的内容哈希唯一索引，再建立允许 A→B→A 修订序列的非唯一复合索引；覆盖 daily-bar 与 fundamentals revision 表。
- `ensureColumn` 为旧版 `market_data_quotes` 补充 `volume_ratio`、`turnover_rate`。
- SQL 初始 DDL 已包含这两列和目标非唯一索引，但不包含上述旧库索引替换逻辑。
- Migration 没有 schema_migrations/version 表，重复执行靠 CREATE IF NOT EXISTS + information_schema 幂等检查；DDL 多语句不是单一事务。
- Root Compose 目前 Data 容器独立启动并执行 AutoMigrate；合并后应由 Root Composition Root 在唯一 DB Pool 上执行一次 Data migration。必须明确失败时不启动依赖 Data 的业务路由，且不能让 Data Store 关闭 Root 所有的 DB Pool。

**迁移风险：** 不能把 `001_create_market_data.sql` 当作可重放全部历史数据库的版本化 migration。M1/M4 应先建立当前生产 Schema 基线与旧索引兼容测试，再决定采用现有幂等 Go Migration 迁入统一 Migration Registry，还是将其拆成有版本号的增量迁移。不得在未比对真实部署 Schema 前重建或重命名任何表/索引。

#### 5. Go Module 归并前置发现

当前 Root 与 Data 仍是两个 Go Module：
- Root `go.mod`: module `example.com/stock-analysis-flow`，Go 1.22；MySQL Driver v1.9.3，Redis v9.7.3。
- Data `services/market-data/go.mod`: module `github.com/Kasign/stock_data_provider/services/market-data`，Go 1.23.0；MySQL Driver v1.8.1，另有 `github.com/injoyai/tdx v0.0.86`、`golang.org/x/text v0.28.0` 等 Provider 依赖。
- Data 生产代码大量使用嵌套 Module 的完整 import path；归并到 Root 后必须整体替换为 Root module path 下的 `internal/marketdata/...`，不能保留旧 module import。
- Go 版本需从 1.22 升到至少 1.23.0（除非逐项验证并降级 Data 代码/依赖，不建议为迁移降低版本）。
- MySQL Driver 需要选定单一版本；Root 现用 v1.9.3，Data v1.8.1，优先验证 v1.9.3 与 Data Store/Integration Test 兼容。
- Data 依赖中的 SQLite/Xorm/leveldb 等大部分当前标记为 indirect，但是否仍被实际 package graph 引入需要通过 `go mod why` / `go list -deps` 验证；不能仅凭 indirect 就删除。
- Root 模块路径当前是 `example.com/stock-analysis-flow`，需要确认这是有意的稳定 import path 还是历史占位符；M1 不应未经决策就随意更换 module path。
- 根 Dockerfile 当前构建 Root Module；Data 独立 Dockerfile/Compose 构建嵌套 Module。M1 合并后应先让 Root 的 `go test ./...` 能覆盖迁移后的 Data 包，再改部署构建。

#### 6. 本轮结论与下一步

- [x] 35 Operation 的路由、认证入口和 OpenAPI response code 已重新对照。
- [x] 统一错误映射及 408/500/502/503/504 产生条件已核对。
- [x] 纠正“5 个 503 漏项就是全部差异”的过早结论，按各 Handler 的真实错误出口细化。
- [x] 9 张 Data 表在 SQL 初始 DDL 与 Go Runtime Migration 中均有定义。
- [x] 确认 Runtime Migration 额外负责旧索引替换与旧列补齐，SQL 文件不能替代迁移历史。
- [x] 发现 Root/Data Go 版本和 MySQL Driver 版本冲突，列出 M1 依赖归并要求。
- [ ] 在真实数据库 Schema（生产/测试脱敏快照）上执行完整 DDL 差异核验；当前只对仓库 DDL 做静态对照。
- [ ] M0 全量 Contract Test 与 v1 OpenAPI 修正尚未执行。
- [ ] M1 Go Module 归并仍未开始，当前仅完成依赖冲突清单。

**当前状态：** M0 仍 `in_progress`，M1 暂不准入。下一轮优先把 Model ↔ OpenAPI 全量差异做成可复核的字段矩阵，并将本轮状态码结论纳入 MDM-001；若用户授权实施契约修正，则可在当前分支一次性修复 OpenAPI 与 Contract Test，不触碰其他模块代码。


### 2026-11-06：M0 第二十六轮（Go Model ↔ OpenAPI 字段/Required/Nullable 静态交叉核对）

审计基线：`b1e8d7227ccabb4fb4b11ed7506e94b205e97060`。本轮读取 11 个 model 源文件与当前 OpenAPI components.schemas，按 Go JSON Tag 和 Schema properties/required/nullable 做静态提取比对；不是运行中的 Go reflection/YAML parser 测试，复杂嵌套/组合 Schema 仍需后续自动化测试确认。

#### 1. 字段覆盖

- Go Model 文件中提取到 57 个 struct。
- OpenAPI 中提取到 60 个 Schema。
- 51 个同名 Go Struct ↔ OpenAPI Schema 可以直接对照。
- 这 51 组的 JSON 字段名集合一致：未发现 Go JSON Tag 字段缺失于 OpenAPI，也未发现 OpenAPI properties 中有额外字段。
- 6 个 Go struct 当前没有同名 Schema：AdjustmentFactor、CorporateAction、StockUniverseSnapshot、MarketStockOverviewItem、ProviderProbeFieldComparison、PriceLimitRuleSnapshot；它们属于内部/嵌套/由 Handler 另行投影的模型，不能仅按名称判定缺失契约。
- 9 个 Schema 没有同名 Go struct：Error、QualityStatus、AdjustmentBasis、RequestAdjustmentBasis、DailyBarsResponse、ProviderProbeRequest、DataSourcesResponse，以及两个 operation/response 局部 schema（get/post）；需通过 Handler/Wrapper 关系确认，而非要求每个 Schema 都有同名 struct。

**字段名结论：** 当前已修正近期字段缺失/多余问题，51 组直接映射的字段名层面没有发现新差异；剩余主要集中在 Required、Nullable 和语义约束，而不是继续增加字段。

#### 2. Required 差异

51 组中发现 1 个明确差异：

- `DragonTigerSnapshot.source_version`：Go 字段为 `SourceVersion string json:"source_version"`，没有 `omitempty`，JSON 必然输出；OpenAPI `DragonTigerSnapshot.required` 未包含 `source_version`。应补 required，并在字段契约测试中覆盖。

其余 50 组按当前 JSON Tag 的 `omitempty` 与 OpenAPI required 列表静态比对，没有发现额外 Required 差异。该结论只覆盖直接同名 Struct/Schema；匿名嵌入、手工 map、Handler 投影、外层 Wrapper 仍需通过实际 JSON Golden 验证。

#### 3. Nullable 差异（重要）

在 51 个同名 Schema 中共提取到 150 个 `nullable: true` 字段：
- 30 个对应 Go 的非 omitempty 指针字段，nil 时会由 encoding/json 输出 `null`，与 nullable 语义一致。
- 120 个对应 Go 的 `omitempty` 指针字段或普通非指针字段；按当前 Go JSON 编码规则，nil/零空值会被省略，普通 string/int/bool 不会输出 null。OpenAPI 因而允许了当前 Go Response DTO 不会产生的 null 值，Schema 比实际响应更宽松。

明确的普通非指针 nullable 误差包括：
- `DragonTigerItem.provider_amount_unit`：Go string + omitempty，OpenAPI nullable。
- `DragonTigerSnapshot.provider_report_date`：Go string + omitempty，OpenAPI nullable。
- `StockBasic.industry`、`StockBasic.board_code`：Go string + omitempty，OpenAPI nullable。
- `LimitPoolSnapshot.provider_report_date`：Go string + omitempty，OpenAPI nullable。
- `ChipDistribution.date`、`ChipDistribution.provider_date`：Go string + omitempty，OpenAPI nullable。

另外，Quote、DailyBar、Fundamentals、StockBoard、MarketCapitalFlow、SectorRanking、Index、DragonTiger、HotRanking、LimitPool、ChipDistribution、CapitalFlow、Valuation 等模型的大量 `*T,omitempty` 字段也声明为 nullable；这类字段在 nil 时实际省略，不会序列化为 null。若契约目标是“缺失值统一以 null 表达”，应调整 Go JSON Tag 去掉 omitempty，而不是只保留宽松的 Schema；若目标是保持现有响应字节兼容，则应去掉对应 Schema 的 nullable。

**不能机械把 120 个 nullable 全部删掉：** 应按字段逐类确认是否有历史 API 客户端依赖“null”语义。由于当前 Go 类型的 omitempty 已决定实际 wire output，推荐 v1 保持现有 JSON 行为，Schema 与其对齐；若将来需要显式 null，应作为单独兼容性变更评估。

#### 4. LimitPoolItem.change_percent 的条件约束冲突仍待最终裁决

当前 Go 字段是 `*float64 json:"change_percent"`（没有 omitempty），OpenAPI 将其列为 required + nullable。这一组合与 Go 编码器一致：字段必然出现，nil 时为 null。

但此前对 DataService Validator 的静态审计结论是：HasData=true 时拒绝 nil change_percent，HasData=false 时 Items 必须为空。若该 Validator 仍适用于每个 Item，则正常成功响应的 item.change_percent 实际不可能为 null，nullable 只是过宽；但如果保留 required + nullable，则必须存在明确的合法业务分支允许某个 Item 在成功响应中为 null。此项应以当前 Validator 单元测试和真实 JSON fixture 再确认，不能只凭 Go 类型决定。

#### 5. 本轮结果与剩余工作

- [x] 51 组同名 Go Struct/OpenAPI Schema 字段名集合静态对照。
- [x] 发现 DragonTigerSnapshot.source_version required 漏项。
- [x] 量化 150 个 nullable 中约 120 个与当前 Go omitempty/非指针输出行为不一致。
- [x] 识别 7 个明确普通字符串 nullable 过宽字段。
- [ ] 将 120 个 nullable 差异按客户端兼容性逐字段裁决。
- [ ] LimitPoolItem.change_percent 结合 Validator 测试与实际 Handler JSON fixture 复核。
- [ ] 将静态提取逻辑沉淀为可执行 Go Contract Test，避免每轮人工/脚本重复比对。
- [ ] 覆盖 6 个无同名 Schema 的模型及 9 个无同名 Go struct 的 Schema，建立实际 Handler DTO 映射。
- [ ] MDM-001 OpenAPI/Contract Test 修正尚未实施。

**当前结论：** 近期字段名修复基本覆盖；Required 仅发现 DragonTigerSnapshot.source_version 一处明确遗漏；Nullable 有较大范围的“Schema 允许 null、Go 实际省略”差异，需按 v1 兼容性策略收敛。M0 仍未通过，但剩余问题已从笼统全量字段差异收敛为可定位的 required/nullable、错误响应和可执行契约测试任务。


### 2026-11-06：M0 第二十七轮（LimitPool Nullable 条件语义复核与历史结论纠偏）

审计基线：`3a82139deb6e343fb651abb07512e909206f1b65`。本轮读取 `validateLimitPoolSnapshot`、LimitPool Handler、服务测试、OpenAPI Contract Test 与 LimitPool Schema，解决第二十六轮遗留问题。

#### 1. change_percent 的真实约束

当前 `LimitPoolItem.ChangePercent` 是 `*float64 json:"change_percent"`，无 `omitempty`；OpenAPI 将其设为 required + nullable。当前实现语义如下：

- 当 `snapshot.HasData == true`：Validator 遍历所有 Items，要求 Symbol、Code、Name 均非空且 `ChangePercent != nil`；否则整个 Provider Snapshot 被拒绝，进入 fallback。
- 当 `snapshot.HasData == false`：Validator 要求 Total=0、Items 为空、QualityStatus=unknown；不允许返回带缺失涨跌幅的 Item。
- 因此，合法 LimitPool API 响应中的每个 Item 都必须有非 null 的 change_percent；unknown/empty 快照没有 Item，不存在该字段的实例。
- 但 Go DTO 本身允许零值或内部构造的 Item 将该指针序列化为 null；现有 OpenAPI Contract Test 的 `TestDataModelJSONSerializationMatchesOpenAPIRequiredAndNullableSemantics` 明确验证了 DTO 级别 `change_percent: null` 的序列化能力。

**最终判断：** 这不是 Validator 与 OpenAPI 的简单矛盾，而是 DTO 级 JSON 表达能力与 Endpoint 级业务不变量不同。当前 OpenAPI 的 required + nullable 准确描述 DTO 层可序列化形态，但没有表达 Endpoint 对 HasData=true Items 的条件约束。建议保留当前 Go 类型和 v1 Schema，补充 `LimitPoolSnapshot` 的条件约束说明（has_data=true 时 items 非空且每项 change_percent 非 null；has_data=false 时 items 为空且 total=0、quality_status=unknown），并增加 API/Service Contract Test 覆盖条件分支。不要为了收紧 OpenAPI 而擅自把字段改成非指针或改变 JSON wire format。

#### 2. 测试覆盖缺口

当前 `data_test.go` 已覆盖 LimitPool 多类非法数据（source/version、total、symbol/code、非有限数值、负数计数、质量不一致、未来/错配 report date），但尚未发现专门测试以下两个分支：
- HasData=true 且 Item.ChangePercent=nil 必须被 Validator 拒绝。
- HasData=false 但 Items 非空必须被 Validator 拒绝。

这两条应补为明确回归测试；目前代码逻辑已经实施，缺的是直接锁定该契约的测试用例。本轮未修改测试代码。

#### 3. 对第二十六轮 nullable 统计的解释修正

第二十六轮统计的 120 个 nullable 差异，是“Go DTO 的零值/缺省对象是否可能序列化 null”与“OpenAPI Schema nullable”之间的静态对应，不代表 120 个字段都会在真实 Handler 成功响应中出现 null。部分字段可能被 Service Validator、Provider 质量门禁或 Handler 投影约束排除。

后续 nullable 治理应分三层，不得一刀切：
1. **DTO serialization contract**：Go JSON Tag 实际能否输出 null/省略。
2. **Endpoint business invariant**：成功响应在业务状态约束下是否允许 null。
3. **OpenAPI wire contract**：Schema 描述的是 DTO 结构可表达范围，还是进一步描述成功响应的业务子集；必要时用 description/oneOf/条件说明表达。

据此，第二十六轮“120 个 nullable 不一致”应降级为“120 个待分类字段”，不是 120 个已确认 OpenAPI 错误。普通非指针且 omitempty 字段（如 provider_amount_unit、provider_report_date）仍是优先复核对象；指针 + omitempty 字段必须结合 endpoint 约束判断，不能只看 Go Tag。

#### 4. 本轮结论

- [x] 确认 HasData=true 时 ChangePercent=nil 会被拒绝。
- [x] 确认 HasData=false 时必须 Total=0 且 Items 为空，QualityStatus=unknown。
- [x] 确认 OpenAPI required + nullable 与 DTO 级序列化行为一致。
- [x] 纠正第二十六轮对 120 个 nullable 字段的过度定性。
- [ ] 补充两个 Validator 条件分支的直接测试。
- [ ] 对其余 nullable 差异按 DTO / Endpoint / OpenAPI 三层重新分类。
- [ ] 为 LimitPoolSnapshot 增加条件业务约束描述与端到端响应断言。

**当前状态：** LimitPool change_percent 不再作为确定的 Schema 缺陷；保留现有 v1 wire contract，转为条件语义文档和测试补强任务。M0 仍未通过；当前主要工作应转向将静态审计结论变为自动化、可重复验证的 Contract Test，以及数据库真实 Schema 基线核对。


### 2026-11-06：M0 第二十八轮（现有 Contract Test 能力盘点与门禁缺口）

审计基线：3b1eb36acb96b389ad9c5927a9fb174fd14e6e02。本轮对照 openapi_contract_test.go、platform_contract_test.go、server_test.go、LimitPool Validator 测试，确定哪些审计结论已经被自动化锁定、哪些仍只是文档结论。

#### 1. 已存在的自动化覆盖

- TestOpenAPIDataRoutesDeclareValidAPIKeySecurity：验证 ApiKey Scheme 和 7 条选定路径的 operation-level ApiKey 声明；**不是**全量 35 Operation 的路由/认证对照，也没有验证 root-level security 是否仍含 Bearer OR。
- TestOpenAPIDataModelFieldsMatchKnownJSONContract：人工枚举一批字段存在、required/optional、nullable 和多余字段；不是全量 Model ↔ Schema 反射对照。
- TestDataModelJSONSerializationMatchesOpenAPIRequiredAndNullableSemantics：仅覆盖 Quote、DailyBar、Bundle、HotRankingItem、MarketSentimentSnapshot、LimitPoolItem、SourceStatus、StockAnnouncementSnapshot 等少量代表 DTO；不是所有 51 组 Struct/Schema 的 Golden。
- TestPlatformFaultHTTPProjectionMapsClassAndDefaultsToNonRetryable：覆盖 writeErr 的 400/401/403/404/409/502/503/504/500 class、code、message、retryable、request_id。
- TestDomainErrorProjectionDoesNotExposeInternalCause：覆盖 Provider 错误安全消息。
- TestDomainTimeoutIsMarkedAsRetryCandidateWithSafeMessage：覆盖 deadline → 504。
- server_test.go 有多个真实 Handler 的 200/400/401 测试，以及 LimitPool 200/400；未形成所有 Handler 的 408/502/503/504 注入矩阵。
- LimitPool Validator 当前实现已拒绝 HasData=true 且 ChangePercent=nil，但现有测试未发现专门的 nil ChangePercent 负例；HasData=false + 非空 Items 也未发现专门负例。

#### 2. 已明确的测试缺口

1. **Security**：解析 OpenAPI YAML，遍历全部 35 Operation 与 mux 注册路由；验证 Health security: []、33 个业务 Operation 统一接受 ApiKey OR BearerAuth，并用运行时测试锁定两种请求头形式。
2. **Response**：从 Handler/统一错误映射建立状态码矩阵；为每类稳定错误共享验证 Error JSON Golden，并校验 Operation response 引用 Error Schema。当前 Error Schema 仅通过文本包含断言，且内层 required 只有 code/message，与实际必输出 class/code/message/retryable 不一致。
3. **Model**：将 51 组同名 Struct/Schema 的 JSON Tag、properties、required、nullable 自动比对；对 nullable 再区分 DTO serialization 和 Endpoint business invariant，不能只按指针判断。
4. **Endpoint**：补充 LimitPool 两个条件分支负例；对有条件业务约束的 Schema 使用 description/组合结构表达，并通过 Handler Golden 验证。
5. **Parameter**：对 35 Operation 的 query/path/body 参数建立 Handler 校验表；将静态审计中的日期跨度、分页范围、批量数量、body 上限等关键约束转成可执行断言。

#### 3. 最小实施拆分（仍属于 MDM-001）

- CT-01：OpenAPI security 全量一致性测试；保留根级 ApiKey OR BearerAuth，移除 11 个不一致的 operation-level 覆盖；Health 明确匿名。
- CT-02：Error Schema required 与共享 Error Response 引用；逐 Operation 修正状态码声明（不机械给所有接口添加全部错误码）。
- CT-03：Go Model/OpenAPI Schema 全量字段与 required/nullable 对照测试；修正 DragonTigerSnapshot.source_version required。
- CT-04：LimitPool 条件约束与缺失分支测试；补齐其他确认过的 endpoint 约束说明。
- CT-05：35 Operation 参数约束表和高风险参数边界测试。
- CT-06：运行全部 Market Data 单元/集成测试，生成 M0 证据清单；真实数据库 DDL 对照仍需脱敏 Schema 快照。

**范围边界：** 本轮只审计测试现状，没有修改 OpenAPI、Handler、Model 或测试代码。上述 CT 项仍需用户明确授权实施 MDM-001 代码修正后执行；在授权前继续完成审计证据与迁移门禁整理。

#### 4. 本轮结论

- [x] 区分现有测试真实覆盖范围与此前审计推导范围。
- [x] 确认 Error Schema 测试只做文本断言，未校验 required / Operation 引用。
- [x] 确认 35 Operation 的状态码矩阵尚未转为自动化测试。
- [x] 确认 LimitPool 两个关键 Validator 分支缺少直接负例。
- [x] 将 MDM-001 代码修正拆成 CT-01～CT-06，便于一次按功能组实施。
- [ ] MDM-001 代码修正与全量测试尚未执行。
- [ ] 真实数据库 Schema 快照核验尚未执行。

**当前状态：** M0 仍未通过，M1 不准入。审计已足以形成一组边界清晰的 MDM-001 修正任务；下一步应优先取得脱敏数据库 Schema 证据，同时等待/确认是否授权当前分支实施 CT-01～CT-06。不得把“测试覆盖不足”误写为“现有运行时校验不存在”。
### 2026-11-06：CT-01～CT-06 实施进度（用户已授权）

实施基线：8540319ed9688bb33ec08eda2973274f2301eb56。仅修改 feat/full-go-data-providers-api-switch；未触碰其他分支或其他模块；PR #1 仍保持 Open/Draft。

#### CT-01：认证与路由一致性
- 经重新读取 Server.auth，Data HTTP Adapter 接受 X-API-Key 与 Authorization: Bearer 两种凭据格式，两者校验同一个 Data API Key。此前“只接受 X-API-Key”的审计结论错误。
- OpenAPI 保留根级 ApiKey OR BearerAuth；移除 11 个 Operation 的 operation-level ApiKey 覆盖，避免其安全能力与另外 22 个 Operation 不一致。
- 新增全量路由对照测试：35 个 OpenAPI Path/Method 与 Server.Handler 路由清单逐项一致；2 个 Health 接口匿名，33 个业务接口统一继承两种认证方式。
- 新增运行时认证测试，覆盖 X-API-Key、Bearer、缺失凭据、错误凭据。

#### CT-02：Error Response 契约
- Error.error.required 已修正为 class/code/message/retryable；request_id 仍为可选字段。
- 当前 OpenAPI 已声明的 119 个非成功响应均增加 application/json 内容并引用 components.schemas.Error。
- 新增 YAML 解析测试，逐 Operation 验证所有已声明的 4xx/5xx 响应都引用统一 Error Schema，并要求全部 33 个受保护业务 Operation 声明 401；已补齐 diagnostics 的 401。
- 尚需结合 Handler 状态码矩阵复核漏声明的状态码（特别是取消请求 408 的稳定可达性）；不能将 408 无差别添加到全部 Operation。

#### CT-03：Model ↔ Schema
- 新增 51 组 Go Model / OpenAPI Schema 自动对照注册，比较 JSON 字段集合和 required 集合；对无 omitempty 的指针字段校验 Schema nullable。
- DragonTigerSnapshot.source_version 已加入 OpenAPI required，与 Go json tag 对齐。
- 原有代表性 JSON 序列化测试继续保留。剩余 nullable 需按 DTO serialization / Endpoint business invariant 分类，不将所有指针与 Schema nullable 差异机械判为错误。

#### CT-04：LimitPool 条件语义
- 新增 HasData=true 且 ChangePercent=nil、HasData=false 且 Items 非空两个 Validator 负例；调整 Validator 分支顺序，使无数据快照先执行空态不变量检查，并由测试断言命中该分支，而非被 total/items 通用检查提前拦截。
- OpenAPI 已描述 has_data 条件约束：true 时 items 非空且每项 change_percent 非 null；false 时 total=0 且 items 为空。
- 保留 v1 DTO 的 pointer 与 nullable wire contract，不改变既有 JSON 结构。

#### CT-05：参数边界
- 新增 15 个高风险参数边界负例，覆盖 Market Stocks 分页/布尔/复权、Sector/LimitPool limit、DragonTiger 分页、Stock Search 长度/limit、Announcements 分页和 Bundle lookback 下界。
- 现有 DragonTiger、Announcements、Stock Search、日期、证券代码等 Handler 测试继续保留。
- 35 Operation 的完整参数矩阵仍需与 OpenAPI Schema 做最终逐项核验；尚未覆盖的日期跨度/Body 大小等约束继续列为剩余项。

#### CT-06：回归验收
- 已触发 Market Data Go test、race、vet、Docker build 和 MariaDB/PIT 集成工作流；最终状态以最新提交的 Actions 结果为准。
- 真实部署数据库 Schema 快照尚未提供，DDL 与生产现场的最终对照仍未完成；不得用 CI 临时 MariaDB 代替生产 Schema 证据。

当前准入判断：CT-01～CT-04 的代码与契约修正已提交；CT-05 已完成第一批边界测试但完整参数矩阵仍待核对；CT-06 等待最新 CI 结果与数据库 Schema 证据。M0 暂不宣告通过，MDM-002 / M1 仍不准入。
