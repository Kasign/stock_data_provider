# Platform 维度 1：Application Port 与真实调用点

## 1. 核验范围

- 代码基线：`origin/integration`，`b6dfc85ba214fe16268c8cba780641a8bfe15086`
- 审计文档分支：`feat/design_doc`
- 本批范围：Platform 对外技术能力的接口形态、依赖方向，以及已确认的 Platform 内部反向依赖。
- 本批不宣称已完成全仓消费者调用点扫描；未检查的消费者不得视为不存在。

## 2. 契约判断

《公共平台最小契约规范》明确 Platform 只提供公共技术原语，不承载领域业务语义。依赖方向为：

```
cmd / transport / bootstrap
   -> platform/*
   -> application -> domain / ports
adapters -> ports
```

因此，Platform 不需要也不应该为 News、Research、Researchflow、Quant、LLM、Notification 等领域定义统一 Application Port。领域 Application Port 应由领域模块定义，Platform 仅提供可被组合和注入的技术能力。

## 3. 已核验的实现证据

| 能力 | 代码证据 | 已确认的接口形态 | 判断 |
|---|---|---|---|
| HTTP Client | `internal/platform/httpclient/client.go`（blob `5e62857b6f85fe0f8e88fd5e5e07dde06a2c4508`） | `Resolver` 标准 Go interface；`Config`、`DestinationPolicy` 配置结构；`New(Config)` 构造 `*Client`；`Client.Do` 接受请求和显式 DestinationPolicy | 技术能力通过标准 Go 类型和构造函数注入，不是领域 Application Port |
| HTTP Client 适配 | `internal/platform/httpclient/bound_doer.go`（blob `ae140df0b83330e86a7965e883d3e659a626b9fb`） | `Client.Bind(policy)` 返回窄接口适配器 `BoundDoer`，提供 `Do(*http.Request)` 和 `OpenStream` | 固定目标策略，避免消费者在单次调用时漏传或替换策略 |
| Diagnostics | `internal/platform/diagnostics/check.go`（blob `33f263b258a931e3d43a5b9d25ea3bdc83dddda2`） | `CheckFunc` 回调和 `Check` 描述独立诊断项；领域方提供执行函数 | 领域诊断逻辑由调用方提供，Platform 负责执行约束 |
| Diagnostics Runner | `internal/platform/diagnostics/runner.go`（blob `b93f6ef40f525f86f29e29eb9250e36d2d890cdf`） | `Runner.Run(ctx, checks)` | 执行器是通用技术原语，不替代领域诊断服务 |
| Request ID | `internal/platform/observability/request_id.go`（blob `b9e2d9ddc298ba23bead6ed1fcc9d6ec58ef707b`） | HTTP Middleware、Context 读写及出站传播函数 | 作为边界中间件/Context 原语使用，不是领域 Port |

## 4. 已确认的依赖边界问题

### AUD-PLAT-PORT-001：Platform Task Queue 反向依赖旧 Analysis Domain

**待裁决 · P1 候选**

证据：

- `internal/platform/taskqueue/queue.go`（blob `d438d934c6267ee1d7a14d482d37741cf3d1c795`）直接 import `example.com/stock-analysis-flow/internal/analysis/domain`。
- `Runner` 接口签名使用 `domain.RunRequest` 和 `domain.RunResult`。
- `Event` 暴露 `domain.StageEvent`。
- `Queue.Create` 直接接收 `domain.RunRequest`。
- 对应测试 `internal/platform/taskqueue/queue_test.go`（blob `6ff46bc8410cd72f1adb0077b21dc257f0a5583e`）同样直接依赖旧 Analysis Domain。

判断：

该实现不是领域无关的通用 Task Port，而是旧 Analysis Worker/Queue 的领域实现放置在 Platform 路径下，与公共契约“Platform 不依赖领域包”的方向相反。契约第 1、11 节明确禁止 Platform import 领域模块，并要求领域拥有自己的长任务语义。

处理建议：

1. 不在本次审计文档分支直接修改业务实现。
2. 由旧 Analysis 迁移/Server 责任方确认该 Queue 的最终归属。
3. 若保留为领域实现，应迁出 Platform 命名空间并由对应领域拥有；若抽取通用能力，抽象边界必须不引用领域 DTO/事件/Repository。
4. 在迁移完成前，该项不得作为 Platform 依赖边界通过证据。

## 5. 消费者调用点核验状态

本批已确认 Platform API 的接口形态，但尚未完成所有消费者的生产代码扫描。以下事项保持待核：

- 根 Module 内各模块是否通过构造函数注入 Platform 能力，还是在业务层直接创建/持有技术客户端。
- Server Bootstrap 是否为 HTTP Client、Storage、Diagnostics、Request ID 提供唯一装配入口。
- Data 独立 Module 与迁入根 Module 的边界是否遵守契约；独立 Module 不得跨 Module import 根模块 `internal`。
- 是否存在消费者绕过 `BoundDoer`、直接调用底层 HTTP Client 或重复实现相同基础能力。
- Platform 其他包是否存在未发现的领域 import；本批仅以 taskqueue 的明确证据登记，不代表全量 import 扫描已经完成。

由于 GitHub 代码搜索未返回可用的基线消费者结果，本批不将其作为全仓无调用点的证据。后续需按固定 SHA 对 Server 和各领域消费者逐个追踪真实构造点、注入点和调用点。

## 6. 本批结论

- Platform 不需要定义统一领域 Application Port；现有技术能力采用标准 Go interface、配置结构、构造函数和回调注入，方向符合契约设计。
- `internal/platform/taskqueue` 对旧 `internal/analysis/domain` 的反向依赖是已证实的边界问题，登记为 AUD-PLAT-PORT-001，等待责任模块裁决。
- 消费者追踪、Server 装配入口、Data 独立 Module 边界和全量 Platform import 扫描尚未完成。

**维度状态：partial。不得标记为 verified。**


## 7. 第二批：Server 组合根与 Data Module 接入点

### 7.1 Server Bootstrap 的 Platform 消费证据

固定基线文件：

- `internal/server/bootstrap/bootstrap.go`（blob `92df61265f173251ac7811f99a7ab9f41ac470ed`）
- `internal/server/bootstrap/router.go`（blob `2597c998236a8e981773635eced7915a28d6b2ed`）
- `internal/server/diagnostics/service.go`（blob `190068e7fc170d6f81d8a79868086fa2870a65e7`）
- `internal/server/migration/registry.go`（blob `dbb81dec2f1ab18a647ecea16a9ac6b0da03b483`）
- `internal/server/runtime/lifecycle.go`（blob `fcfe753c345af505a4f25e94050f19049d9ad75b`）

已确认的真实调用：

| Platform 能力 | Server 调用点 | 方式与判断 |
|---|---|---|
| Config | `bootstrap.Run` 调用 `config.LoadDotEnvIfExists`、`config.NewEnv` | Server 组合根直接使用公共配置原语 |
| Security | `platformsecurity.NewAuthenticator`；`NewRouter`、`AnonymousAccess`、`OwnerAccess` | Server 装配认证器与路由策略，符合组合根职责 |
| Storage | `newMigrationRegistry` 注册模块提供的 `[]platformstorage.Migration`；Registry 调用 `platformstorage.ApplyMigrations` | SQL 定义由模块提供，Platform 执行底层迁移能力，Server 负责编排 |
| Diagnostics | `serverdiagnostics.NewServiceWithMigration` 使用 `platformdiagnostics.Runner{}` | Server 负责聚合 Storage/模块 Check，Platform 负责通用执行 |
| Task Queue | Bootstrap 调用 `taskqueue.New`，注册 `legacyAnalysisWorker` | 目前是旧 Analysis 专用能力，和 AUD-PLAT-PORT-001 的领域反向依赖问题相互印证 |
| Lifecycle | `runtime.NewLifecycle`、`Register`、`Start`、`Stop` | 属于 Server 自身的进程级组件编排，不是 Platform Port |

Server 的路由文件仅注册 Health/Readiness/Diagnostics 和旧 Analysis 路由；没有在该路由装配文件中发现新领域 Application Port 的接入。Bootstrap 中明确说明 Legacy Analysis 组件仍处于过渡边界。

### 7.2 当前 Server 组合根覆盖范围

Bootstrap 的 Migration Registry 当前只注册：

- News：`newsstorage.Migrations()`
- Research：`researchstorage.Migrations()`
- Notification：`notificationstorage.Migrations()`

当前 Worker Lifecycle 只注册旧 Analysis Worker；HTTP API 只接入旧 Analysis Handler。不能将这些事实推断为其他模块没有自己的 Application Port，而是说明本固定基线的 Server 组合根尚未将全部领域模块装配为运行组件。

### 7.3 Data Module 的独立启动与注入边界

固定基线文件：

- `cmd/market-data/main.go`（blob `06fc460cbea38c1aad34b9e23c5124004a91f3fe`）
- `internal/marketdata/bootstrap/module.go`（blob `859df0ca2fc26f02dab2399fe684617da9f2fc89`）
- `internal/marketdata/bootstrap/egress.go`（blob `d32914b2693aba0163c521a3dc9c3223c55585e8`）

已确认：

1. Data 有独立 `cmd/market-data` 启动入口。入口自行打开 MariaDB、按配置创建 Redis Cache，再调用 `bootstrap.New`。
2. `marketdata/bootstrap.Dependencies` 已为各 Provider 预留 `provider.HTTPDoer` 和受控 TCP Dial 注入点，并有 `RequireEgressDependencies` 完整性校验。
3. `Module.New` 可返回窄化的 `Application()`、`Diagnostics()`、`MigrationRunner()`，并区分调用方拥有的 Store/Cache 与模块自身拥有的 BaoStock Client。
4. 当前 `cmd/market-data/main.go` 调用 `bootstrap.New` 时仅传入 Store、Cache、Logger，没有传入 HTTPDoer / Dialer，也没有启用 `RequireEgressDependencies`。
5. Data Module 的 Provider 注入机制具备承接受控出口的能力，但该独立启动入口没有证据表明实际使用 Platform HTTP Client 的 `BoundDoer` / `BoundDialer`。

判断：

- Data 的 Application Port 和基础设施依赖注入接口已经存在；这部分不应误判为“缺少 Port”。
- 但在本固定基线的独立启动方式下，Platform 受控出站能力与 Data Provider 的依赖注入尚未形成可证明的接线。需要单独审计 Data 的出站安全契约和部署边界。
- 这也解释了为何不能只看 Data 的 Dependencies 字段或 Egress 完整性校验，就认定 Platform HTTP Client 已接入。

### 7.4 本批新增待核项

**AUD-PLAT-PORT-002：Data 独立入口未证明接入 Platform 受控出站执行器（待核，P1 候选）**

证据边界：`cmd/market-data/main.go` 的 Dependencies 仅传 Store/Cache/Logger；Module 依赖中虽有 HTTPDoer/Dialer 字段，但当前入口未注入。当前入口也未开启 `RequireEgressDependencies`。

后续处理：
- 先核对 Data 部署是否仍由独立 `cmd/market-data` 承担，以及是否有其他生产启动入口负责注入；
- 核对所有 Provider 的默认 HTTP/TCP 实现，判断是否存在绕过受控出口的真实网络调用；
- 再由 Data 与 Platform 责任方确定是由 Root Server 注入，还是保留独立 Data 组合根但由其创建 Platform BoundDoer/BoundDialer；
- 未完成前不得标记 Data Platform 出站接入通过。

### 7.5 本批结论

- 已找到 Server 对 Config、Security、Storage、Diagnostics、Task Queue 的真实调用点；Server 是 Platform 技术能力的消费者和组合根。
- Server 当前 Bootstrap 仍以旧 Analysis 为中心，News/Research/Notification Migration 已登记，但其余领域的 HTTP/Application/Worker 接入不能从当前 Bootstrap 中得到证据。
- Data 独立入口和 Module 注入结构已核；其 HTTPDoer/Dialer 受控注入点存在，但入口没有注入这些依赖。
- Request ID Middleware、Platform HTTP Client 的 Server 装配、各领域模块消费者仍未完成全量追踪。

**维度状态仍为 partial。**
