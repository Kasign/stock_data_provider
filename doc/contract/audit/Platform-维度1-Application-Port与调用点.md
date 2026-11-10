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


## 8. 第三批：News 模块 Platform 消费者核验

### 8.1 News Application Port 与 Provider 出站适配

固定基线证据：

- `internal/news/app/ports.go`（blob `da1d571deba9dc038db4270c72ec5c7a025ce369`）
- `internal/news/adapters/provider/platform_doer.go`（blob `2f103b96ca184938dd9052b4b86c77812f0470c4`）
- `internal/news/adapters/provider/platform_set.go`（blob `2c5e25c211a2f76f3aaa11ce4bc9e492545c7e27`）
- `internal/news/adapters/provider/http.go`（blob `9bb7762f9f17d04ffc53a691bc955993a179e215`）
- 对应测试：`platform_doer_test.go`（blob `2c5e493093ce5340d9057c9af9a70cda8c56d7d9`）、`platform_set_test.go`（blob `f9a3e255ef25559efeb141594dfb6110d0e04149`）

已确认：

1. News Application 定义窄 `HTTPDoer` Port；注释明确生产环境由组合根注入绑定 Platform DestinationPolicy 的 Doer。
2. Adapter 的 `NewPlatformHTTPDoer` 使用 Platform `httpclient.Client.Bind` 创建 Provider 专属 `BoundDoer`，并为 endpoint 固定单 Host/Port、关闭 Redirect、限制 HTTP scheme 由配置决定。
3. `NewPlatformProviderSet` 为 Bocha、Tavily、Brave、SearXNG 分别构造绑定的 Doer，Provider 不持有 Platform Client 生命周期。
4. Provider Adapter 将 Platform Fault 映射为 News 自己的 `ProviderError`；仅透传经过 `Fault.Validate` 的 Code/Retryable，不透传 Cause，防止底层敏感错误越过领域边界。
5. 测试覆盖 DestinationPolicy 精确约束、跨 Host 请求拒绝、非法 Fault 降级为通用错误、Cause 脱敏，以及 Provider Set 中每个 Provider 确实持有 Platform BoundDoer。

判断：News 的 Platform HTTP Client / Fault 适配代码及单测质量符合“领域 Port 由 News 拥有、Platform 实现由组合根注入”的契约方向。

### 8.2 News Diagnostics 对 Platform Runner/Fault 的使用

固定基线证据：

- `internal/news/diagnostics/checks.go`（blob `7c11feb178f9f6833d2cdb9af2dd3ba30eeec3e2`）
- `internal/news/diagnostics/business_quality.go`（blob `fc5725e57abb3c260205f851ef50b03c7d91ce49`）
- `internal/news/diagnostics/checks_test.go`（blob `fa827622e38f86cce75866e2b9b52977b85f7a4d`）

已确认：

- News Diagnostics 返回 Platform `diagnostics.Check`，使用 Platform Fault 作为探针执行失败的内部错误分类。
- 测试直接调用 `platformdiagnostics.Runner{}` 执行 News Check，覆盖 Provider 未配置时 skipped、Parser 本地自检、Retention backlog degraded、无效 backlog unavailable 和错误脱敏。
- News 自有业务质量仍作为 Attributes 独立输出，不把领域质量状态错误折叠为 Platform 健康状态。

### 8.3 Server 生产装配缺口

与 News Adapter/Diagnostics 的局部实现相对照，固定基线 `internal/server/bootstrap/bootstrap.go`：

- 只 import `internal/news/adapters/storage` 的 Migration 包；未 import News Application、Provider Adapter 或 News Diagnostics。
- `newMigrationRegistry()` 注册 News Storage Migration，但这不等于 News Application 已装配。
- Server Diagnostics 通过 `NewServiceWithMigration(storageResources, migrationRegistry)` 创建，调用时没有传入 News Module Checks。
- Bootstrap 未创建 Platform `httpclient.Client`，也未调用 `NewPlatformProviderSet`。
- 当前 Server 路由只装配旧 Analysis API 和 Health/Readiness；没有 News Application Handler/内部服务构造点。

因此，当前基线证明 News 的 Platform Adapter 和 Diagnostics Check 已实现并有单测，但未证明它们被 Server 生产组合根使用。不能将 Adapter 单测通过等同于线上接入完成。

### 8.4 新增待核项

**AUD-PLAT-PORT-003：News Platform Adapter / Diagnostics 未接入 Server 生产组合根（P1 候选）**

证据范围仅限已检查的 News Provider Adapter、News Diagnostics 与 Server Bootstrap：

- Provider Set 工厂和 BoundDoer 适配存在，但在已检查的 Server Bootstrap 中没有构造调用。
- News Check 工厂存在，但 Server Diagnostics 构造时未注入模块 Check。
- Server 仅注册 News Migration。

处理建议：
1. 由 Server/News 责任方确定 News 是通过 Server 内部 Application Port 注入，还是由内部任务/服务直接调用；不要求暴露独立外部 News API。
2. 明确 Platform HTTP Client 生命周期归 Root Server，Provider-specific BoundDoer 由 News Adapter 构造。
3. 将 News Diagnostics Checks 注入 Server Diagnostics 聚合，或明确由独立内部诊断路径承载。
4. 补充至少一个组合根测试，证明真实构造/注入关系，不以 Adapter 单元测试替代集成证据。

### 8.5 本批结论

- News 的 HTTPDoer Port、Platform BoundDoer 适配、Platform Fault 映射和 Platform Diagnostics Check 均有实现及单测。
- News 的 Application Port 归属正确，Platform 仅提供技术原语。
- 固定基线 Server Bootstrap 尚无证据将 News Provider Set、News Application 和 News Diagnostics 接入生产运行时。
- 本批未完成 News Request ID 消费者核验；不能推断 News 业务流程已传播 Request ID。

**News 消费者核验：implementation partial / production wiring not evidenced。Platform 维度 1 总状态仍为 partial。**

## 9. 第四批：Research 模块 Platform 消费者核验

### 9.1 Application Port 与 Fault

固定基线证据：
- `internal/research/application/application.go`（blob `38998f41d16a2fafc2e9e8c33c9fe43f07a140c3`）
- `internal/research/application/authorization_error.go`（blob `3424b89db515e56b285253405c745c7002ce3861`）
- `internal/research/application/application_test.go`（blob `5cbabd0f2a1bbe3328f4370b0bcdbfdeb9ffd09b`）

已确认：
1. Research 在 `internal/research/application` 定义自己的 Application Port（`Port`、`EventPort`、`ClaimPort`），对外 DTO 使用 Application 自有字段，不直接暴露 Domain Entity 或 Repository 实现。
2. Application 内部 Repository/Authorizer 接口仍使用 Research Domain 类型，这是模块内部 Application→Domain/Adapter 的依赖，不等于跨模块 Port 暴露 Domain 对象。
3. Research 将验证、Not Found、Conflict、超时、存储完整性错误映射为 Platform `fault.Fault`；Evidence 授权错误进一步区分 forbidden、unavailable、timeout、internal。
4. Application 单测覆盖基本创建/读取、授权撤销和错误路径；本批未逐项核对全部 Fault Code 与项目级 Fault 分类矩阵。

判断：Research Application Port 的所有权和 DTO 边界符合模块自治方向；Platform Fault 被作为公共错误原语使用。

### 9.2 Diagnostics

固定基线证据：
- `internal/research/application/diagnostics.go`（blob `40589aebe8903dc3e16c6db99649465414829b9e`）
- `internal/research/diagnostics/checks.go`（blob `750be512ba3b9396022866f9ea7ed6d243e8e973`）
- `internal/research/application/diagnostics_test.go`（blob `b8bc0c4c6f18dc6216fc51a2330e6b2ce2b8b602`）
- `internal/research/diagnostics/checks_test.go`（blob `85eb29893307e3c28065b867a13c440aced36955`）

已确认：
- Application 的 `DiagnosticsPort` 构造 Platform `diagnostics.Check`；由调用方执行 Platform Runner，Research 不重复实现调度器。
- Research Diagnostics Adapter 将 Context/Event/Claim/Evidence 的领域快照校验映射为 Platform Observation。
- 诊断构造基于不可变版本；Evidence Check 通过 Application 的授权读取路径复核调用者权限，并避免把证据正文写入 Observation。
- 测试覆盖质量状态映射、Run cutoff 精确匹配、快照不可变性、敏感内容不进入 SafeMessage/Attributes，以及 Application 从指定版本构造 Check。

判断：Research Diagnostics 的职责划分和 Platform Runner 集成方式符合公共诊断契约。

### 9.3 Correlation ID 与 Request ID 的边界

固定基线证据：
- `internal/research/application/correlation.go`（blob `5117e19bb3856a5eb18d659bff7cc900533617f5`）
- `internal/research/application/correlation_test.go`（blob `f264067b31d2045d95c90d909087c29325403123`）

Research 定义独立的 Correlation ID Context Key，并明确与 Platform Request ID Context 区分；只允许可信父工作流上下文注入，缺失时由 Research ID Generator 生成新的 correlation ID。单测覆盖继承可信父 ID 和拒绝不合法值。

未确认 Server 是否把可信 Platform Request ID/父工作流标识映射为 Research Correlation ID，也未确认跨模块消息的 originating_request_id 与 correlation_id 的双字段关联。因此本批只确认 Research 内部边界设计，不宣称端到端 Request ID 传播完成。

### 9.4 Server 生产装配与存储所有权

固定基线证据：
- `internal/server/bootstrap/bootstrap.go`（blob `92df61265f173251ac7811f99a7ab9f41ac470ed`）
- `internal/server/diagnostics/service.go`（blob `190068e7fc170d6f81d8a79868086fa2870a65e7`）
- `internal/research/storage/migrations.go`（blob `597db1f16d503f9290f521a0002417227bd0fde6`）
- `internal/research/storage/repository.go`（blob `22e33c13951aeda92a56d602899d143a963b647d`）

已确认：
- Research Storage 提供 Platform `storage.Migration` 列表，导入包不会自行执行 DDL。
- Server Migration Registry 已注册 Research Migration。
- Research Repository 通过注入的 `*sql.DB` 执行存储操作；Repository 本身不创建/关闭数据库连接池。
- Server Bootstrap 未 import `research/application`、未构造 Research Application Service/Repository/Authorizer，也未把 Research Diagnostics Checks 传给 Server Diagnostics。
- Server 当前已具备 Migration 装配，但不代表 Research Application、Repository 或诊断检查已经进入生产运行时。

### 9.5 新增待核项

**AUD-PLAT-PORT-004：Research Application / Diagnostics 未接入 Server 生产组合根（P1 候选）**

当前固定基线中，Research Application、Diagnostics Adapter、Platform Fault 映射均已实现并有单测；Server 只注册 Research Migration；没有 Research Application 构造、Repository 注入、Authorizer 注入或模块 Diagnostics 注入的生产调用点证据。建议由 Server/Research 责任方确定内部服务装配和生命周期归属，并补充组合根测试；不要把 Migration 注册视为模块已接入。

**AUD-PLAT-PORT-005：Research Correlation ID 与 Platform Request ID 的端到端关联待证（P2 候选）**

Research 有意区分两个标识，这是合理的语义边界；但当前已核文件未证明可信 Server 上下文如何建立 Correlation ID，也未证明 Outbox/Worker 消费链保留 originating_request_id 与 correlation_id 的关联。需在 Server 与异步消费者核验时补证，暂不定性为实现缺陷。

### 9.6 本批结论

Research Application Port、Platform Fault 和 Diagnostics Adapter 的模块内实现方向正确；Research Migration 已由 Server 注册。固定基线仍缺 Application/Repository/Authorizer/Diagnostics 的 Server 生产装配证据。Request ID 与 Correlation ID 的跨模块传播尚待核验。

**Research 消费者核验：module-level implementation evidenced / production wiring not evidenced。Platform 维度 1 总状态仍为 partial。**


## 10. 第五批：Notification 模块消费者核验（初审）

### 10.1 Application Port 与模块边界

固定基线证据：

- `internal/notification/application/ports.go`（blob `2cda82ab5da00aab4c8898f9ac0e384fb9c32c66`）
- `internal/notification/application/channel_ports.go`（blob `111c614906e21b436d8e9d125bee186b453042f0`）
- `internal/notification/application/receive_event.go`（blob `b6cd60c1c7437cf4e0073eefc68514ee244c0dc8`）
- `internal/notification/application/delivery_execution_service.go`（blob `ef4d79b0213fc17e097c3ef8dcf0caf769751758`）
- `internal/notification/application/lease_recovery_service.go`（blob `7dc75bebedcdd60d986ac4ee71e20e038a8d28f2`）

已确认：

1. Notification Application 自有 SubscriptionRepository、DeliveryPlanRepository、DeliveryRepository、DeliveryAttemptRepository、DeliveryLeaseRecoveryRepository、TemplateRenderer、ChannelAdapter、ChannelResolver、RecipientResolver、InboxRepository 等 Port。
2. Port 使用 Notification Domain 类型；ChannelAdapter 注释要求具体 Adapter 在构造时绑定 Platform Secret 与策略约束 HTTP Doer，且不向 Application 暴露明文 Secret/任意 URL。
3. `EventReceiver.Receive` 在持久化前验证完整 Event Envelope；InboxRepository 契约要求事件和 Fanout 工作项在同一事务写入。
4. `DeliveryExecutionService` 只负责单条 Delivery 的 Preflight、渲染、Attempt 和渠道调用；注释明确队列扫描和 Claim 由上层 Worker 负责。
5. `LeaseRecoveryService.RecoverExpiredOnce` 是有界、调用方驱动的单次恢复过程，本身不持有 Timer 或后台 Goroutine 生命周期。

判断：Notification 已有较完整的模块内 Application Port 和投递执行领域服务；但接口定义和单测不构成生产 Worker 已运行的证据。

### 10.2 Repository、Migration 与事务边界

固定基线证据：

- `internal/notification/storage/migrations.go`（blob `ac5a90d65c9ccf0920b5aee5cfb6234ffcaa56c3`）
- `internal/notification/storage/mysql_inbox_repository.go`（blob `16ecbd5be10078cb871e337e90b40b3e75de12b9`）
- `internal/notification/storage/mysql_expired_delivery_lease_scanner.go`（blob `62fd035f9e73b12a417f3ad8cefda77681cb69fa`）

已确认：

- Notification Storage 提供 Platform `storage.Migration`；Server Migration Registry 已注册 Notification Migration。
- Inbox Repository 必须注入 `InboxFanoutWriter`；新事件写入 Inbox 和 Fanout 工作项复用同一 SQL 事务，避免接收成功但 Fanout 丢失。
- Inbox 使用 `(source_module,event_id)` 主键和 `(source_module,idempotency_key)` 唯一约束；持久化 `correlation_id`。
- Migration 中 Delivery State Event 表含可空 `request_id` 字段；Inbox 表持久化 `correlation_id`。这只能证明数据结构预留，不能证明请求上下文到事件/状态事件的端到端映射已接线。
- 过期租约扫描采用有界批次，按到期时间排序，并在恢复写入前重新检查数据库时钟；恢复服务使用完整 Attempt 历史和 fencing token 进行收敛。

### 10.3 Fault、Diagnostics 与 Request ID

- 本批抽查的 Application Port/Service 以 Notification 自有错误值和领域错误作为内部错误表达；尚未发现其到 Platform `fault.Fault` 的统一映射证据。本结论仅限已检查文件，尚未完成 Application 全目录错误映射扫描。
- 固定基线 `internal/notification` 顶层目录仅见 application、domain、schema、storage、template；未发现独立 diagnostics 包。Server Diagnostics 构造调用未传入 Notification Checks。
- Storage Schema 同时存在 `request_id` 与 `correlation_id` 字段，但尚未核实二者写入调用点、可信来源和异步消费者传播。

### 10.4 Server 生产装配与 Worker 生命周期

固定基线证据：

- `internal/server/bootstrap/bootstrap.go`（blob `92df61265f173251ac7811f99a7ab9f41ac470ed`）
- `internal/server/diagnostics/service.go`（blob `190068e7fc170d6f81d8a79868086fa2870a65e7`）
- `internal/server/runtime/lifecycle.go`（blob `fcfe753c345af505a4f25e94050f19049d9ad75b`）

已确认：

1. Bootstrap 只导入 Notification Storage Migration，并在 Migration Registry 注册 Notification；未导入 Notification Application 或 Storage Repository 构造。
2. Bootstrap 只注册旧 Analysis Worker；没有 Notification Worker 的构造、Lifecycle.Register、Start 或 Stop 调用点。
3. Server Diagnostics 使用 `NewServiceWithMigration(storageResources, migrationRegistry)`，没有传入 Notification Module Checks。
4. Notification 的 DeliveryExecutionService 明确把扫描/Claim 留给上层 Worker；LeaseRecoveryService 只提供单次恢复入口。固定基线未找到负责驱动这些入口的 Server 运行组件。
5. Inbox Repository 需要事务内 FanoutWriter，但本批 Server Bootstrap 没有注入该依赖的构造点。

### 10.5 新增待核项

**AUD-PLAT-PORT-006：Notification Application、Fanout 与 Delivery Worker 未接入 Server 生产组合根（P1 候选）**

证据边界：已检查 Notification Application Port/执行服务、Inbox Repository、Server Bootstrap、Diagnostics Service 与 Lifecycle。模块内有接收、持久化、单条投递执行和租约恢复能力，但固定基线 Server 只注册 Migration；没有 Notification Application/Repository/FanoutWriter/Worker 构造和 Lifecycle 接线。需由 Notification/Server 责任方明确 Fanout 持久化实现归属、Delivery Worker 的调度与 Claim 入口、Lease Recovery 周期驱动、共享资源所有权及停止顺序，并补组合根测试。当前不能认定为已运行的生产投递链。

**AUD-PLAT-PORT-007：Notification Request ID 与 Correlation ID 端到端映射待证（P2 候选）**

Migration 中 Inbox 持久化 `correlation_id`，Delivery State Event 持久化 `request_id`；但本批未发现从可信 Platform Request ID 到事件上下文、Fanout 工作项、Delivery 状态事件及 Worker 的端到端映射证据。需在 Server/Outbox/Fanout 消费链核验时补证，暂不定性为运行时缺陷。

### 10.6 本批结论

Notification 模块内 Application Port、Inbox 幂等、Fanout 事务注入、Delivery Attempt/租约/fencing token 和租约恢复逻辑已有实现证据；Server 只注册 Migration，尚无 Application、Fanout、Delivery Worker 和 Diagnostics 的生产装配证据。Request ID 与 Correlation ID 的端到端传播待核。Fault 映射和 Application 全目录消费者扫描未完成。

**Notification 维度 1 状态：partial；模块内实现 evidenced / production wiring not evidenced。Platform 维度 1 总状态仍为 partial。**


## 11. 第六批：Quant 模块消费者核验（初审）

### 11.1 模块性质与 Application Port 边界

固定基线证据：

- `doc/quant/01-量化引擎总体设计.md`（blob `ce55ace71676c34e65f7b41166542eab266c8a50`）
- `doc/quant/02-任务拆分与验收.md`（blob `fb9f9226204a766ca27d4b39378dd3671a7cfe0a`）
- `internal/quant` 固定基线目录树：domain、factor、indicator、manifest、metric、registry、result、risk、trend；未见 application、adapters、storage、diagnostics 子目录。

设计文档明确 Quant 是确定性数值引擎，不调用 LLM、不编排研究任务、不连接 Provider/HTTP/SQL/Redis；目标分层按实际能力逐步建立，不为目录齐全创建空包。QNT-001～003 主要是纯计算，QNT-010 风险执行骨架与 QNT-013 Repository Port/MemoryRepository 仍处于部分完成状态；生产 Storage、Migration 和真实事实接入明确列为后续集成验收。

已确认：

1. Quant 当前提供领域计算/契约组件，不是已完成的独立业务 Application 服务。
2. 当前未定义 Quant Application Port，也未看到跨模块调用所需的服务级请求/响应组合接口。此项需结合 QNT-008～010 及 Researchflow 消费契约后续复核，不能仅凭缺少 application 目录认定架构违约。
3. Quant 结果 Repository Port 当前只有 `Save`；没有 `Get`/Query 读取能力。文档明确在 QuantResultRef/Envelope 关系及读取时授权复核流程冻结前，不提供可能绕过授权的读取端口。
4. Quant 纯计算包的错误值主要是包内领域错误；本批未发现 Platform Fault 适配层。由于当前尚无 Application/Transport Adapter，不能要求 Domain 直接依赖 Platform Fault；待 Application/Server 组合层形成后再核验错误映射。

### 11.2 Platform 原语消费情况

已抽查 Quant 的 Manifest、Registry、Metric、Risk、Result 关键生产实现：

- 未发现这些纯计算/契约包直接 import Platform HTTP Client、Diagnostics、Request ID 或 Storage。
- 该边界与 Quant 总体设计中的“纯数值引擎、不连接 HTTP/SQL/Redis”一致。
- Quant Registry 是领域指标元数据 Registry，不应替换或扩展为 Platform 通用 Registry。
- 当前没有 Quant Diagnostics Check 实现；也没有证据表明 Quant 应在纯计算层自行运行 Platform Diagnostics。待 Application/Server 接入时，应由 Quant 提供领域检查，再由 Server 组合 Platform Runner。

### 11.3 结果持久化与授权边界

固定基线证据：

- `internal/quant/result/repository.go`（blob `0fb18c3ba8fb17af87b81e195596bbc556c6eb51`）
- `internal/quant/result/memory_repository.go`（blob `622adc454d1c9aefa68b4ca2666b2285030deae9`）
- `internal/quant/result/ref.go`（blob `bb05015e56cf419b871b92c53e5943ae3a0dd6a0`）
- `internal/quant/manifest/manifest.go`（blob `a3315a5d6fdd822136094ec5565771d23dc27fd1`）
- `doc/quant/11-RepositoryPort与不可变结果记录.md`（blob `c5567fa78e98d4bf94abdbcc0844eba7a92b193a`）

已确认：

1. Manifest 使用 RFC 8785 JCS + SHA-256 形成内容寻址身份；结果 payload 指纹与 Manifest 分离验证。
2. Repository Port 只提供 Save，要求同一 result_id 相同内容幂等、不同内容冲突，并要求实现以事务/原子操作保证并发正确性。
3. MemoryRepository 明确仅用于测试和本地组合测试，不替代 MariaDB/Platform Storage，也不具备进程重启后的持久性。
4. 当前未实现 Quant SQL/Platform Storage Adapter，也未注册 Quant Migration；这与 QNT-013 文档将真实 Storage、事务和根 Migration 列为后续集成验收一致。
5. BuildRef 从已验证 Manifest 派生结果身份字段，但注释明确不签发或验证 AuthorizationRef 的授权事实；AuthorizationRef 仅作为必填引用文本传入。Golden 测试验证了示例引用的结构与绑定，不等同于生产授权签发、签名验证、主体/资源/用途/时效复核。
6. 因此结果读取和授权必须留在未来 Application/Server 边界；不得把 BuildRef 或 Schema/Golden 通过当成授权已生效。

### 11.4 Risk 执行与硬门禁边界

固定基线证据：

- `internal/quant/risk/evaluator.go`（blob `c86b2d5c27f5d59ad1c51be034b2d36f9bc0cbbd`）
- `internal/quant/risk/decision_builder.go`（blob `21eee22703901a7116f7fff2bd8f95da7bbe4229`）
- `internal/quant/risk/aggregate.go`（blob `a48a3a542cf2f30a16290f686ccb540c87777c9c`）
- `doc/quant/12-RiskRuleSet执行入口.md`（blob `32ade0d3bb58724e85a2823f7cf539931a123b30`）

已确认：

- `EvaluateRuleSet` 按 RuleSet 声明顺序执行，并复制输入规则集/evaluator 注册表。
- 必需规则缺失或执行失败时生成 block + unknown；未知 evaluator、规则身份/严重级别不匹配拒绝构造决策；Context 取消不生成部分决策。
- 规则执行器错误文本和 panic 内容不会进入公开 SafeMessage。
- `AggregateStatus` 以 block 级硬门禁为基础；触发 block 返回 reject，block 结论 unknown 或缺少 block 规则返回 unknown，只有 block 规则明确未触发才 pass。
- Quant 只提供通用执行骨架；具体风险规则、事实读取、可信规则集加载/审批/签名/发布/回滚和运行时授权明确留给 Application/Server/Repository 接入。当前无证据证明生产规则集已接入。

### 11.5 Server 生产组合根

固定基线证据：

- `internal/server/bootstrap/bootstrap.go`（blob `92df61265f173251ac7811f99a7ab9f41ac470ed`）
- `internal/server/bootstrap/bootstrap_test.go`（blob `7cc244a2d6e62b5a177d08efd3484195598b5cf4`）
- `internal/server/diagnostics/service.go`（blob `190068e7fc170d6f81d8a79868086fa2870a65e7`）

已确认：

1. Server Bootstrap 未 import Quant；未构造 Quant Registry、计算服务、Risk RuleSet 执行器或 Result Repository。
2. Migration Registry 当前只注册 News、Research、Notification；Bootstrap 测试断言的 namespace 也只有这三个模块，没有 Quant。
3. Server Diagnostics 当前只接入共享 Storage/Migration 检查；没有 Quant Module Checks。
4. Server Lifecycle 当前只注册旧 Analysis Worker；Quant 当前也没有独立 Worker/后台任务生命周期。
5. 因 Quant 尚未形成生产 Application/Storage Adapter，这些缺失符合当前模块阶段状态；但不能据此将 Quant 认定为已可供 Researchflow/Server 生产调用。

### 11.6 新增待核项

**AUD-PLAT-PORT-008：Quant 生产 Application/Storage/Diagnostics 接入尚未形成（P1 候选，阶段性阻断）**

证据边界：Quant 领域计算、Registry、Risk 执行骨架、Manifest/Result Save Port 已有模块内实现；但固定基线没有 Application/Adapter/Storage/Diagnostics 目录，Server 未构造 Quant 服务、未注册 Migration、未接入 Diagnostics。Quant 任务规划明确 QNT-010 的具体规则/可信发布/事实接入及 QNT-013 的生产 Storage/事务/根 Migration 尚未完成。此项应作为后续生产集成门禁，而非认定当前纯计算 Domain 违反 Platform 依赖方向。进入 Researchflow/Server 生产调用前，需明确 Application Port、输入事实授权、Result Save/Read 授权边界、Platform Fault 映射、Diagnostics、Storage Migration 和组合根测试。

**AUD-PLAT-PORT-009：Quant AuthorizationRef 生产签发与验证责任待证（P1 候选，安全边界）**

`BuildRef` 仅检查 AuthorizationRef 非空文本并将其写入 Ref；代码注释明确不签发或验证授权事实。Golden 只验证固定样本的 Schema/指纹/主体/资源字段，不构成生产授权证明。后续必须由明确的可信授权责任方签发并验证 AuthorizationRef，至少覆盖主体、资源、用途、策略版本、决策时间、有效期/撤销语义；读取端口必须在授权复核完成后才能开放。当前未发现读取接口，因此暂不认定存在绕过授权的可利用读取路径。

### 11.7 本批结论

Quant 的纯计算与结果契约边界基本符合其模块设计；Risk 执行骨架有 fail-closed 实现证据。Quant 当前不是已装配的生产 Application 模块，尚无 Platform Fault/Diagnostics Adapter、生产 Storage/Migration、Server 组合根或 Worker 接入证据。AuthorizationRef 只作为引用文本，不证明授权已签发/验证。

**Quant 维度 1 状态：partial；domain/contract implementation evidenced / production application wiring not yet implemented by current stage. Platform 维度 1 总状态仍为 partial。**


## 12. 第七批：LLM 模块消费者核验（初审）

### 12.1 LLM Domain/Adapter 与 Platform HTTP Port

固定基线证据：

- `internal/llm/llm.go`（blob `cb7e0bb9a8b79a31d0d3a9bd213d03423354c87a`）
- `internal/llm/responses_adapter.go`（blob `dc398278474dcdc63efd2c6e695c344bb176fc2e`）
- `internal/llm/responses_stream.go`（blob `6923f9d0a00ed31f82def9c970ef493e3855e1ff`）
- `internal/server/llmtransport/transport.go`（blob `c5e1cffcd2724f50e806f549e805e1188757ce9a`）
- `internal/server/resources/storage.go`（blob `beebb1858f93dcc03e6a71b9706f406c5fde0481`）

已确认：

1. `internal/llm` 定义窄 `HTTPDoer` 和 `HTTPStreamDoer` Port；ResponsesAdapter 不创建 HTTP Client，不持有可变 DestinationPolicy。
2. `internal/server/llmtransport.Transport` 将 Root Server 共享 Platform Client 绑定为一个不可变 BoundDoer，同时适配普通 HTTP 和 SSE Stream；不拥有/关闭共享 Client。
3. `validateProviderPolicy` 要求 Gateway HTTPS、单一精确 Host、端口精确匹配、禁止 Redirect、禁止 HTTP、禁止传输层自动重试；非标准端口必须显式 allowlist。
4. `server/resources.Resources.BindHTTPDestination` 从 Server 所有的共享 Platform Client 创建 BoundDoer；资源关闭后拒绝新绑定。
5. `llmtransport.NewResponsesAdapter` 将同一个 Transport 注入 JSON 与 SSE 路径，避免两个出口策略分叉。
6. LLM Adapter 通过传入的 `context.Context` 建立带 Deadline 的 HTTP Request；Platform HTTP Client 在出站前调用 `observability.PropagateRequestID`，因此只有调用上下文中存在可信 Platform Request ID 时才会覆盖/传播出站 `X-Request-ID`。

判断：LLM 的底层出站 Port 与 Server 侧 Platform BoundDoer Adapter 已实现，符合 Platform Client 由 Root Server 共享持有、模块使用受限绑定的方向。

### 12.2 Secret、Provider 配置与能力证据

固定基线证据：

- `internal/platform/config/env.go`（blob `c1d5b0dfc7e11c5e0a4a22d2a8c1e4dca7d0a7bb`）
- `internal/llm/llm.go`（同上）
- `internal/llm/responses_adapter_test.go`（blob `6994bbbeb758fdbdba7feaa43e850b0247be6ae1`）
- `internal/server/llmtransport/transport_test.go`（blob `526822771f5e1b558c2ddbe13b91466d830f05fb`）

已确认：

1. `ProviderConfig` 只保存 `APIKeySecretRef`，不保存凭据明文；配置校验要求 SecretRef 非空。
2. ResponsesAdapter 构造时必须显式传入 `platform/config.Secret`；未配置 Secret 或未注入 Platform-bound Doer 时拒绝创建。
3. Platform Secret 的 String/GoString/Format/LogValue/JSON/Text 序列化均脱敏；只有显式 `Value()` 可取明文，Adapter 仅在构造出站 Authorization Header 时读取。
4. ProviderConfig 限制 HTTPS Gateway URL、固定 `/v1` 路径、拒绝 URL UserInfo/Query/Fragment；Provider/Model 需匹配 allowlist。
5. CapabilityState 把 Provider、Model、Protocol、Gateway Version、验证时间、有效期、验证方法、EvidenceRef 和能力状态绑定；未知、过期、未来验证或超过 Provider TTL 的证据不能作为 supported；流式开关要求当前 SSE 能力证据。
6. 单测覆盖缺失 Secret/Doer、凭据 Header 注入、错误信息脱敏、能力过期、模型 allowlist、Gateway Policy Host/Port/Redirect/HTTP/Retry 约束。

边界：本批看到的是 Secret 引用字段与 Secret 值的独立注入契约；未发现生产配置解析/SecretRef 解析器把具体 ProviderConfig.APIKeySecretRef 绑定到对应 `platform/config.Secret` 的调用点。必须由 Server 组合根负责该绑定，不能仅凭构造器接受一个 Secret 就认定引用解析和 Secret 归属已完成。

### 12.3 Platform Fault 映射与错误脱敏

固定基线证据：

- `internal/llm/platform_fault.go`（blob `cde2dcdbafa54d2211999cc8b1e94ca82830a885`）
- `internal/llm/llm_test.go`（blob `8c325127b47f4ab271588e35541184530c829d6f`）
- `internal/llm/responses_adapter_test.go`（blob `6994bbbeb758fdbdba7feaa43e850b0247be6ae1`）

已确认：

- `ModelErrorFromPlatformFault` 将 Platform Fault Class/Code 映射为 LLM 自有 ModelErrorInfo；Dependency/Cause 不进入公开 DTO。
- 映射覆盖 validation、unauthenticated、forbidden、unavailable、timeout、not_found、conflict、internal；支持 response_too_large、context canceled/deadline 的特殊分类。
- 非 Platform Fault 不被误映射；缺失 Code/SafeMessage 使用稳定安全回退值。
- AdapterError 对外只输出 ModelErrorInfo.SafeMessage，内部 Cause 保留 errors.Is/errors.As 链。
- 单测验证不泄露 Dependency、Cause 和 Secret。

判断：LLM Adapter 的 Platform Fault 消费及错误脱敏已有实现和单测证据；尚未核对所有下游模块如何把 ModelErrorInfo 再映射为各自 Application Fault/Retry Policy。

### 12.4 Request ID 与 Correlation ID 边界

固定基线证据：

- `internal/llm/llm.go`：ModelRequest 包含 RequestID、CorrelationID、PurposeCode、TraceID。
- `internal/llm/responses_adapter.go` / `responses_stream.go`：结果保留 RequestID、CorrelationID；出站 HTTP Request 使用调用方 Context。
- `internal/platform/observability/request_id.go`：Platform 只从可信 Context PropagateRequestID；无 ID 时删除调用方预设 Header。
- `internal/platform/httpclient/client.go`：实际出站时调用 `observability.PropagateRequestID`。

判断：

1. LLM DTO 中 RequestID 与 CorrelationID 是独立字段，语义上未混为一谈。
2. HTTP 出站 Request ID 来源是 Context，而不是 ModelRequest.RequestID 字段；Adapter 没有把 DTO 的 RequestID 自动写入可信 Context，也没有把 CorrelationID 写成出站 Request ID。
3. 若上游 Application/Worker 已正确构造可信 Context，Platform HTTP Client 可传播 Request ID；但固定基线尚无 LLM 生产调用方，未证明 DTO RequestID、Context Request ID、CorrelationID 在调用入口处一致/关联。
4. Gateway Request ID 与 Upstream Request ID 分别保存，测试证明两者未混淆。

**AUD-PLAT-PORT-011：LLM DTO RequestID 与可信 Context Request ID 的入口映射待证（P2 候选）**

当前出站传播原语正确，但 LLM ModelRequest.RequestID 只是 DTO 字段；真正出站 Header 依赖 Context。待 Server/调用模块 Application 装配时，必须明确 RequestID 的可信来源、Context 注入点以及 CorrelationID 的独立传播/持久化语义，补端到端测试。当前没有生产调用方，暂不定性为已发生 Request ID 丢失。

### 12.5 Diagnostics 与 Server 生产装配

固定基线证据：

- `internal/server/bootstrap/bootstrap.go`（blob `92df61265f173251ac7811f99a7ab9f41ac470ed`）
- `internal/server/diagnostics/service.go`（blob `190068e7fc170d6f81d8a79868086fa2870a65e7`）
- `internal/server/bootstrap/bootstrap_test.go`（blob `7cc244a2d6e62b5a177d08efd3484195598b5cf4`）

已确认：

1. LLM 目录没有 Application/Diagnostics/Storage/Worker 子包；现有实现是 LLM Domain/Responses Adapter 和 Server 侧 Transport Adapter。
2. Server Bootstrap 未 import `internal/llm` 或 `internal/server/llmtransport`；没有构造 ProviderConfig、解析 SecretRef、装载 CapabilityState、创建 LLM Adapter 的生产调用点。
3. Server 没有把 LLM Gateway 健康/能力证据 Check 注入 Diagnostics；当前 Diagnostics 只组合共享 Storage/Migration 检查。
4. Server HTTP Router 当前只暴露旧 Analysis、Health/Readiness/Diagnostics 路由，没有 LLM 对外路由；本批没有发现 LLM 对外独立 API 的需求或契约，不能据此要求新增公开路由。
5. LLM 当前是被调用的模型能力 Adapter；没有证据要求它自己拥有 Worker 或后台 Lifecycle。其关键生命周期应由未来 Application/调用方和 Root Server 管理共享 Platform Client。

**AUD-PLAT-PORT-010：LLM Gateway Adapter 尚未进入生产组合根（P1 候选）**

证据边界：LLM Responses Adapter、Server llmtransport、Platform Secret 和 BoundDoer 能力均已实现并有测试；但固定基线 Server Bootstrap 没有构造/调用 llmtransport，没有 Provider 配置与 SecretRef 解析装配，没有 CapabilityState 加载/刷新入口，也没有领域 Diagnostics Check 注入。此项属于生产接入缺口，不能把 Adapter 单测或 Transport 工厂测试当成真实 Gateway 端到端生产调用证据。责任方需明确 ProviderConfig/SecretRef/CapabilityState 的来源与刷新、调用方 Application Port、Gateway Check 的依赖/状态/证据语义，并补 Server 组合根测试和受控真实 Gateway 验收。

### 12.6 本批结论

LLM 的底层 JSON/SSE Adapter、Platform BoundDoer 接口、Secret 脱敏、Provider/Model allowlist、版本化能力证据和 Platform Fault 映射均有实现及单测证据；Server 侧 Transport Adapter 也已实现。固定基线没有 LLM Application/生产 Provider 配置装配、SecretRef 解析、CapabilityState 加载/刷新、Diagnostics 注入或生产调用方。RequestID DTO 到可信 Context 的入口映射待证。

**LLM 维度 1 状态：partial；adapter/transport implementation evidenced / production composition wiring not evidenced。Platform 维度 1 总状态仍为 partial。**


## 13. 第八批：Researchflow 模块消费者核验（初审）

### 13.1 Application Port 与跨模块依赖方向

固定基线证据：

- `internal/researchflow/application/create_run.go`（blob `4c126384d95a1738c4f1603ca7325a9219380adf`）
- `internal/researchflow/application/executor.go`（blob `1b8a6a3f6ac0df79a9ab72bca70b787788f05c9a`）
- `internal/researchflow/application/worker.go`（blob `c08ee149de33742fea5ca1b30384308cc72cdd1b`）
- `internal/researchflow/application/outbox_dispatcher.go`（blob `f8082139616d18cceccf082d2278e78152522f93`）
- `internal/researchflow/contextassembly/research_adapter.go`（blob `a3e8e263efbb32787edc1bb2187b10a5df5be2c8`）
- `internal/researchflow/toolpolicy/llm_adapter.go`（blob `d1513080bdf22ab6ef0926a9d2a09246fb7b7be8`）
- `internal/researchflow/resultvalidation/composer.go`（blob `c4fbf942ad99ed98f5d0b8eb4ddb18bd930c83a8`）

已确认：

1. Researchflow Application 自有 `RequestIdentity`、`DatabaseClock`、`IDGenerator`、`TemplateRegistry`、`RunRepository`、`StageExecutor`、`WorkerStore`、`ExecutionFaultNormalizer`、`OutboxStore`、`OutboxPublisher` 等窄 Port；Application 不直接持有具体 SQL、HTTP、Broker 或 Provider Client。
2. `StageExecutor` 是 Researchflow 模块所有的执行 Port，注释要求 Data/News/Quant/Research/LLM 适配器由 Server 组合根注入；Provider 只返回结果引用，不得直接写 Researchflow 持久化或决定 Run/Stage/Attempt 身份。
3. `contextassembly` 只依赖 Research Application DTO，将完成的上下文选择计划映射为 `CreateContextCommand`；明确不创建 Context、不授权 Evidence、不读取 Section 正文。
4. `toolpolicy` 依赖 LLM 的 Schema 校验器与 Model Tool Call/Response DTO；模型 Tool Call 只转换为待授权 Intent，必须逐个经过 Registry.Authorize，不代表工具获准或已执行。
5. `resultvalidation` 使用 Research Application 的 Context/Evidence/Claim DTO、LLM ModelResponse DTO；Quant 结果采用 Researchflow 自己定义的 QuantResultEnvelope，不依赖 Quant 内部 Domain/Repository 类型。
6. `ComposeResult` 要求调用方通过 Research/Quant 公共 Facade 取数并先完成当前 Owner 授权；LLM Explanation 明确标记 non_authoritative，事实、证据和 RiskDecision 分字段保留，不计算综合评分或可操作状态。
7. RiskDecision 校验拒绝“block 已触发/未知却返回 pass”；Quant 结果校验要求精确 result_type/schema_version 注册，不允许未知版本回退。
8. 当前可见跨模块直接依赖集中在 Research 与 LLM 的稳定 DTO/本地校验能力；未发现 Researchflow 直接依赖 Quant 内部实现或 Notification 内部实现。StageExecutor/OutboxPublisher 的具体适配器仍须由组合根提供。

判断：Application Port 归属和主要跨模块 DTO 边界基本符合“Researchflow 负责编排、领域模块负责事实和能力”的方向；但接口定义和纯函数 Adapter 不等于真实 Facade/Stage 执行器已经接入。

### 13.2 Worker、事务、Lease 与 Outbox 生命周期

固定基线证据：

- `internal/researchflow/application/worker.go`（同上）
- `internal/researchflow/application/deadline_scanner.go`（blob `0e27493c1f6d86c07f92f9330a8ca2ebd92445a1`）
- `internal/researchflow/application/outbox_dispatcher.go`（同上）
- `internal/researchflow/storage/mysql_run_repository.go`（blob `5ef98c916502824eedc431c02469e1d08678a0b5`）
- `internal/researchflow/storage/mysql_outbox.go`（blob `1f8fcc7f164a1b191366a8935d9b94b13005c919`）
- `internal/researchflow/storage/migrations.go`（blob `f6cf573974cae7e2d426f934ef57d30ec62699ed`）

已确认：

1. Worker 在执行前读取不可变 RetryPolicy Snapshot；通过 Store ClaimAttempt 获取跨进程并发权威，再校验 Run/Stage/Attempt 快照一致性。
2. Worker 使用 Run Deadline 与 Attempt Deadline 双重 Context；Heartbeat 续租，Lease/FencingToken 决定提交权；租约丢失、Run Deadline、取消分别处理；Provider RetryAt 不作为重试决策，重试由模板 RetryPolicy allowlist 控制。
3. Worker 通过 `WorkerStore.CommitWorkerResult` 提交；Port 注释要求 Storage Adapter 将其映射为原子 `CommitAttemptResult` 事务。
4. DeadlineScanner 只做有界候选扫描和版本栅栏过期；Run 汇总与最终化分开，避免扫描器自行越权完成最终事务。
5. OutboxDispatcher 提供 at-least-once 发布；稳定 IdempotencyKey、LeaseGeneration、并发上限、PublishTimeout、RetryPolicy 和条件 Ack/Retry 已纳入接口；Publisher 必须幂等。
6. MySQL Storage 实现 Run/Stage/Attempt/Idempotency/ResultSummary/Outbox 等表和事务；Migration 使用 Platform Storage Migration 类型。
7. Outbox 消息持久化 CorrelationID，但未持久化 OriginatingRequestID；StageExecutionRequest 只携带 CorrelationID。

### 13.3 Platform Diagnostics、Fault 与 Request ID

固定基线证据：

- `internal/researchflow/diagnostics/adapter.go`（blob `a21ba4591c1e99927b9dfb4febb761f901bf1939`）
- `internal/researchflow/diagnostics/adapter_test.go`（blob `e18aa77e1ef3ab6ccf94f9a1f4116b7731ee8be9`）
- `internal/researchflow/application/create_run.go`（同上）
- `internal/researchflow/application/worker.go`（同上）
- `internal/researchflow/storage/migrations.go`（同上）

已确认：

1. Researchflow Diagnostics Adapter 将 pass/degraded/fail/not_comparable 四态映射为 Platform Observation；not_comparable 映射 degraded，同时通过 assessment_status 属性保留原语义。
2. Adapter 生成 Platform `diagnostics.Check`，检查 ID 稳定；对 Stage 身份、ReasonCode、CheckedAt、Duration、Attributes 数量进行校验。
3. Application 的 `RequestIdentity` Port 分别读取 AuthenticatedCallerRef、OriginatingRequestID、CorrelationID；CreateRun 将 OriginatingRequestID 和 CorrelationID 写入 Run。
4. Run Storage 保存 originating_request_id 与 correlation_id；但 Worker 的 StageExecutionRequest 仅携带 CorrelationID，OutboxMessage/Outbox 表也仅携带 CorrelationID。未见 Worker 从可信 Platform Request ID 重建 Context 或向下游执行器传播 originating request ID 的明确机制。
5. Researchflow 没有直接依赖 Platform Fault 的实现；执行错误通过 `ExecutionFaultNormalizer` Port 转成 FaultRef/ReasonCode。若未注入 Normalizer，Worker fail-closed，不提交 Provider 错误结果。当前未找到将 Platform Fault 映射为 Researchflow FaultRef 的生产 Adapter。
6. 本批未发现 Researchflow Diagnostics Check 被 Server Diagnostics Service 注册；模块 Check 工厂存在不代表 Server 会执行它。

**AUD-PLAT-PORT-013：Researchflow originating Request ID 未证明贯穿 Worker 与 Outbox（P2 候选）**

Run 创建时已从 RequestIdentity 读取并持久化 OriginatingRequestID；但 StageExecutionRequest 与 OutboxMessage 只传 CorrelationID，固定基线未证明异步执行器/事件消费者能恢复 originating request 关联。需由 Server/Researchflow 明确 Request ID 与 Correlation ID 各自用途、异步 Context 重建和事件字段映射，并补端到端测试。当前不认定 CorrelationID 缺失或已发生请求追踪故障。

### 13.4 Server 生产组合根装配

固定基线证据：

- `internal/server/bootstrap/bootstrap.go`（blob `92df61265f173251ac7811f99a7ab9f41ac470ed`）
- `internal/server/diagnostics/service.go`（blob `190068e7fc170d6f81d8a79868086fa2870a65e7`）
- `internal/server/runtime/lifecycle.go`（blob `fcfe753c345af505a4f25e94050f19049d9ad75b`）

已确认：

1. Server Bootstrap 的 Migration Registry 当前只注册 News、Research、Notification；没有注册 `researchflowstorage.Migrations()`。
2. Bootstrap 没有构造 Researchflow RunRepository、TemplateRegistry、RequestIdentity、StageExecutor、Worker、DeadlineScanner、OutboxDispatcher 或 Run Finalization Coordinator。
3. Lifecycle 当前只注册旧 Analysis Worker；未注册 Researchflow Worker、Deadline Scanner、Outbox Dispatcher 或相关调度组件。
4. Server Diagnostics 没有注入 Researchflow Stage/Dependency/Quality/LLM/Result Checks。
5. 未找到由 Server 组合根把 Research Facade、LLM Adapter、Quant 结果读取/校验、Data/News 执行器和 Notification Outbox Publisher 装配为 StageExecutor 的生产调用点。
6. Lifecycle 已支持按注册顺序启动、逆序停止和失败回滚；但尚无 Researchflow 组件注册，因此不能把通用生命周期能力视为模块运行时已接入。

**AUD-PLAT-PORT-012：Researchflow Application、Storage 与异步运行组件尚未进入 Server 生产组合根（P1 候选）**

模块内已有 Application Port、MySQL Storage、Migration、Worker/Deadline/Outbox 执行骨架及 Diagnostics Adapter；固定基线 Server 未注册 Migration、未构造 Application/Worker/Publisher/StageExecutor、未接入 Diagnostics。生产执行链缺少各领域 Facade/Stage Adapter、FaultNormalizer、Notification Outbox Publisher 和组件调度/停止顺序。此项属于生产集成缺口，不等于 Researchflow 模块内状态机/存储实现未完成。进入联调前需完成 Root Server 装配、真实 Adapter、Migration Registry、Lifecycle、Diagnostics 和组合根测试。

### 13.5 本批结论

Researchflow Application Port 和主要跨模块 DTO 边界已具备；Run/Stage/Attempt 的持久化、租约栅栏、Deadline、取消、重试策略、Outbox at-least-once 和四态 Diagnostics 均有模块内实现证据。当前固定基线 Server 没有将这些能力装配成可运行的生产流程，尤其缺少真实 StageExecutor/OutboxPublisher、Migration 注册、Worker 调度和 Diagnostics 注册。Originating Request ID 从 Run 到异步 Stage/Outbox 的传播待证。

**Researchflow 维度 1 状态：partial；application/storage execution skeleton evidenced / production composition and cross-module execution wiring not evidenced。Platform 维度 1 总状态仍为 partial。**


## 14. 第九批：Server Root Composition 核验（维度 1 收尾批次）

### 14.1 Root Server 当前装配清单

固定基线证据：

- `internal/server/bootstrap/bootstrap.go`（blob `92df61265f173251ac7811f99a7ab9f41ac470ed`）
- `internal/server/bootstrap/router.go`（blob `2597c998236a8e981773635eced7915a28d6b2ed`）
- `internal/server/config/process.go`（blob `13ff754095b763f4afbfa9dda1bbecb74a0b9a91`）
- `internal/server/config/security.go`（blob `3119060466de2d99da96b2bdfc1374e68ad4e356`）
- `internal/server/bootstrap/bootstrap_test.go`（blob `7cc244a2d6e62b5a177d08efd3484195598b5cf4`）

已确认：

1. `bootstrap.Run` 解析进程配置后，通过 `OpenLegacyAnalysisResources` 打开共享 MariaDB、Redis 和 Platform HTTP Client；再构造旧 MarketData Adapter、旧 Analysis Runner、Platform TaskQueue。
2. Root Server 的 Migration Registry 明确只注册 `news`、`research`、`notification` 三个 namespace；对应单测固定断言这三个 namespace。未注册 Researchflow、Quant、LLM、Data 的模块 Migration。Data 当前另有独立 `cmd/market-data` 入口；Quant 当前阶段尚无生产 Storage Migration。
3. Root Server 只构造旧 Analysis `service.Runner` 与 `platform/taskqueue.Queue`；Lifecycle 只注册 `legacy-analysis-worker` 和可选 HTTP Server，没有注册 Researchflow 或 Notification Worker/Dispatcher，也没有其他新模块后台组件。
4. HTTP Router 只注册 `/healthz`、`/readyz`、`/diagnostics`、根页面以及旧 `/api/v1/analysis/**` 路由；没有 Data、Research、Researchflow、Quant、LLM、News 或 Notification 的模块 Application 路由。
5. Server Process Config 仍以 `ANALYSIS_TIMEOUT`、`ANALYSIS_WORKERS`、`ANALYSIS_MAX_QUEUED` 为主要业务运行参数；注释明确领域模块配置由各模块自己的配置入口解析。
6. Server `LoadSecurity` 只解析 Owner Token；代码注释明确模块 Service Scope 尚未接线前旧 Analysis 路由保持 Owner-only。Bootstrap 调用 `NewAuthenticator(ownerToken, nil)`，没有装载模块 Service Credentials。当前 Router 也没有模块 Service Route，因此这是明确的过渡状态，不据此单独判定越权缺陷。
7. Bootstrap 对 HTTP Server 角色执行完整的共享资源创建、Migration 校验、旧 Analysis API/Worker 装配；Worker 角色不创建 HTTP Handler。当前没有基于模块能力的独立服务装配矩阵。

### 14.2 Platform 公共能力的实际消费与生命周期

固定基线证据：

- `internal/server/resources/storage.go`（blob `beebb1858f93dcc03e6a71b9706f406c5fde0481`）
- `internal/server/bootstrap/http_middleware.go`（blob `0ba274509dd14e3da3e1d936b19181fb44126851`）
- `internal/server/bootstrap/http_middleware_test.go`（blob `5f999c41a4e7b6a574ee877abbabef307fdbafae`）
- `internal/server/diagnostics/storage_checks.go`（blob `de8bc86f2a2d1921ddc01929cff85a90cc31a3bc`）
- `internal/server/runtime/lifecycle.go`（blob `fcfe753c345af505a4f25e94050f19049d9ad75b`）
- `internal/server/runtime/lifecycle_test.go`（blob `97e67e036a0339033436a41a290a241167bf587d`）

已确认：

1. Server Resources 拥有共享 Platform SQL、Redis、HTTP Client；`BindHTTPDestination` 可基于共享 Client 创建固定策略 BoundDoer，并在 Resources 关闭后拒绝新绑定。
2. 但 Bootstrap 没有调用 `BindHTTPDestination`，也没有把 Resources 传给 News/LLM/Researchflow 或 Data 的生产 Adapter。共享 Client 的存在只证明基础资源已创建，不证明模块已消费。
3. Server HTTP Middleware 使用 Platform `MiddlewareWithOptions` 零值策略，默认重生成客户端传入的 `X-Request-ID`；单测确认不会接受 caller-controlled ID。Platform Client 出站传播只会使用可信 Context 中的 ID。
4. Diagnostics Service 支持接收模块 Check 分组，但 Bootstrap 调用 `NewServiceWithMigration(storageResources, migrationRegistry)` 时没有传入任何模块 Checks；Storage Checks 只检查 MariaDB、Redis 和 Migration Schema。
5. Lifecycle 本身具备顺序启动、逆序停止、启动失败回滚、并发停止等待和错误聚合测试。Bootstrap 的 defer 明确先 Stop Lifecycle Consumers，再 Close HTTP/Redis/DB 等共享资源，顺序符合资源所有权原则。
6. 当前生命周期机制本身已有证据，但因为新模块 Worker/Dispatcher 未注册，不能将通用 Lifecycle 测试视为新模块停止顺序已验收。

### 14.3 旧 Analysis 与 Platform 边界

固定基线证据：

- `internal/platform/taskqueue/queue.go`（blob `d438d934c6267ee1d7a14d482d37741cf3d1c795`）
- `internal/server/bootstrap/bootstrap.go`（同上）

已确认：

- Root Server 仍直接创建 `platform/taskqueue.Queue`，并以旧 Analysis `service.Runner` 作为执行器。
- TaskQueue 的 Runner、Create 参数和 Event 字段直接引用旧 `internal/analysis/domain` 类型；因此 Server 目前的核心异步工作流仍被旧 Analysis Domain DTO 锁定。
- 这与前文 **AUD-PLAT-PORT-001** 相互印证：问题不只是命名/目录遗留，而是 Platform TaskQueue 的公共 API 仍反向依赖领域类型。
- 责任方需在旧 Analysis 迁移时决定：将该队列迁出 Platform 领域无关命名空间，或抽取真正通用的任务 DTO/事件协议；不能让新模块继续依赖该旧接口。

### 14.4 Server 维度 1 汇总判断

Server 已具备 Root Composition 的基础技术能力：共享 Platform Resources、受控 HTTP Destination 绑定入口、Request ID Middleware、显式 Router/Auth Policy、Migration Registry、统一 Diagnostics Runner、Lifecycle 与有序关闭。

但固定基线的实际组合仍是 transitional legacy Analysis runtime：

- 生产 Application 装配仍以旧 Analysis 为中心；
- 新模块只在部分 Migration 层注册；
- 新模块 HTTP/Application/Worker/Outbox/Diagnostics/受控 HTTP Adapter 没有形成生产装配；
- Service Credential 配置与模块路由明确暂缓；
- Shared Platform HTTP Client 已创建但尚无新模块生产消费者；
- 旧 TaskQueue 仍处于 Platform 命名空间并依赖旧 Analysis Domain。

**AUD-PLAT-PORT-014：Root Server 尚未完成从旧 Analysis 到模块化 Application Composition 的迁移（P1 候选，项目级集成阻断）**

此项作为项目级组合根总缺口，汇总并关联 AUD-PLAT-PORT-001/003/004/006/008/010/012，不替代各模块具体责任项。证据为 Bootstrap 的实际构造调用、Migration Registry 单测、Router 路由清单、Diagnostics 构造调用及 Lifecycle 注册清单。进入模块化生产联调前，需明确 Server 作为唯一组合根的装配清单，按模块接入 Application/Adapter/Repository/Secret/Diagnostics/Migration/Worker/Outbox/Route，并以组合根测试证明资源所有权、启动依赖顺序、失败回滚和逆序停止。

**Server 维度 1 状态：partial；Root Server infrastructure evidenced / new-module production composition not evidenced。**

### 14.5 Platform 维度 1 阶段性结论（十模块尚未全部核验）

本批完成 Server Root Composition 的实际调用点核验，并对前述模块生产接线缺口进行汇总。当前已核验 Platform、Data、News、Research、Notification、Quant、LLM、Researchflow 的部分维度 1 消费者和 Server 组合根；尚未完成 WebUI 与所有模块的全部维度 1 消费者扫描。不能宣称维度 1 全量完成，也不能以当前 Bootstrap 的缺失直接推断各模块内部接口未实现。

下一步仍按既定十模块清单补齐未核消费者，并在全量完成后再进行跨模块调用图、授权链和依赖图核验。


## 15. 第十批：WebUI 对外契约消费核验（维度 1）

### 15.1 浏览器请求边界与部署代理

固定基线证据：

- `web/src/api.js`（blob `b8bcbee5908b43bfa1bc2ca744363190af38287f`）
- `web/default.conf.template`（blob `d13735e1da540ff24a5c59b7a5f0febf317c4161`）
- `web/Dockerfile`（blob `3a64acf49c62457b1c3635684c1fd10522a738d6`）
- `web/tests/deployment-contract.test.mjs`（blob `a12e45901e621ce6bc545f55330c6ab12d7b6e5e`）
- `web/tests/api.test.mjs`（blob `a26eca75b24a744232d370fdbe2411806be45136`）

已确认：

1. 浏览器统一使用相对路径 Fetch；默认行情前缀为 `/market-api`，分析前缀为 `/analysis-api`。未发现该 API Client 直接访问 Provider URL、数据库或 Redis 的实现。
2. Nginx 将 `/market-api/` 代理到独立 Market Data 服务的 8091 端口，将 `/analysis-api/` 代理到独立 Analysis Server 的 8092 端口；这证明当前部署仍是双后端兼容拓扑，不是所有模块经 Root Server 统一装配后的单一模块化 API 边界。
3. Nginx 对两个上游都覆盖传入 `X-Request-ID`，使用自身 `$request_id`，并将 `X-Real-IP`、`X-Forwarded-For`、`X-Forwarded-Proto` 传给上游。该做法避免浏览器自报 Request ID 直接成为可信 ID；与 Server 侧默认重新生成不可信 ID 的策略方向一致。
4. Nginx 配置设置同源 CSP（`connect-src 'self'`）、禁止对象/Frame、限制脚本和样式来源；静态资源通过 Nginx 同源提供。
5. Deployment Contract 测试只断言 Compose 服务名、Nginx 上游变量与代理配置；它不发起真实后端请求，也不验证 Root Server 认证、Session/CSRF 或模块路由是否可达。
6. API 单测通过 mock `fetch` 验证 URL、Header、错误处理和超时；它证明客户端请求构造行为，不证明生产服务端路由、权限或数据能力。

### 15.2 行情 API Key 的浏览器持有状态

固定基线证据：

- `web/src/api.js`（blob `b8bcbee5908b43bfa1bc2ca744363190af38287f`）
- `web/index.html`（blob `4b8a160f517846ab47a04ab72cc3479c464ec7d2`）
- `web/tests/api.test.mjs`（blob `a26eca75b24a744232d370fdbe2411806be45136`）
- `doc/web/API_CONTRACT.md`（blob `65c49fcb9bdb193881cc9e2e667a9d1b3060faaf`）
- `doc/contract/PLAT-002-项目级契约冻结登记簿.md`（blob `60829e9662125b4d6ff004c680924e48e1c7e89f`）

已确认：

1. `loadSettings/saveSettings` 将 `apiKey` 作为行情连接设置的一部分读写浏览器 `localStorage`；`request` 对行情请求添加 `X-API-Key`。
2. API Client 的分析任务请求不添加该行情 Key；单测明确断言行情 Key 只发送给 Market Data 路由，不发送给 Analysis 路由。
3. 页面设置说明该 Key 是内部 Market Data 服务访问密钥，不是第三方 Provider 凭据；页面也说明第三方 Provider Key 由服务端环境变量管理，WebUI 不读取或回显第三方 Provider Key。
4. WebUI API_CONTRACT 第 5 节明确：浏览器持有行情 API Key 是迁移期兼容状态，目标架构禁止浏览器持有；MDM-006 与 Root Server 登录认证切换完成后，应移除浏览器读写与请求头注入、清理 localStorage 旧字段并轮换已暴露凭据。
5. 项目级契约登记簿规定 WebUI 只消费服务端 API，不在前端复制领域质量判断、权限决策或服务端业务规则；目标由 Root Server 对外暴露 HTTP 契约。

**AUD-WEBUI-CTR-001：行情服务 API Key 仍由浏览器 localStorage 持有（P1 候选，迁移安全门禁）**

当前实现是已登记的迁移期兼容状态，不应误报为意外实现偏差；但它尚未达到项目级目标契约。浏览器脚本可读取 localStorage 中的 Key，因此这不是服务端托管凭据模型。关闭条件：Root Server 认证/会话方案完成；行情请求改为可信 Session/Cookie + CSRF 或正式裁决的等效机制；前端不再读取/写入/发送内部服务 Key；旧 Key 清理并轮换；补充浏览器与 Server 认证集成测试。不能用把 Owner Token 或服务端长期密钥放入浏览器的方式替代。

### 15.3 Analysis API 认证与 WebUI 当前不匹配

固定基线证据：

- `web/src/api.js`（blob `b8bcbee5908b43bfa1bc2ca744363190af38287f`）
- `web/tests/api.test.mjs`（blob `a26eca75b24a744232d370fdbe2411806be45136`）
- `web/index.html`（blob `4b8a160f517846ab47a04ab72cc3479c464ec7d2`）
- `internal/server/config/security.go`（blob `3119060466de2d99da96b2bdfc1374e68ad4e356`）
- `internal/platform/security/security.go`（blob `b9a21dd479684ef39c1261ee3587ebf79edcbbc9`）
- `doc/web/API_CONTRACT.md`（blob `65c49fcb9bdb193881cc9e2e667a9d1b3060faaf`）

已确认：

1. `createAnalysisTask/getAnalysisTask/getAnalysisTaskEvents/getAnalysisRun` 只向 `analysisBase` 发起同源请求，不注入 Authorization Bearer，也没有 Session/CSRF 专用处理。
2. Server `LoadSecurity` 要求 Owner Token；Root Server 当前 Owner Authenticator 使用 Owner Principal，旧 Analysis 路由处于 Owner-only 认证状态。
3. WebUI API_CONTRACT 明确写明：当前 Server Analysis 路由使用 Owner Bearer，而 WebUI 不携带该凭证，因此浏览器 Analysis 请求预期 401；禁止把 Owner Token 写入浏览器或匿名开放。
4. 当前 API 单测 mock 成功响应，验证创建任务/读取事件的请求构造；并不验证生产环境认证成功。不能把这些测试标记为端到端联调通过。

**AUD-WEBUI-CTR-002：Analysis API 尚无 WebUI 可用的 Session/CSRF 认证通道（P1，明确联调阻断）**

这是已被 WebUI 契约明确记录的待接入状态。关闭条件：Root Server 提供正式 Web Session/Cookie、CSRF、防暴力登录与资源授权；WebUI 按契约建立会话并携带 CSRF；服务端按用户/资源授权任务读写；浏览器自动化对真实 Server 验证成功、401/403、CSRF 拒绝、会话过期和跨用户隔离。禁止将 Owner Token 或内部服务 Key 放到前端作为临时修复。

### 15.4 WebUI 对项目级契约的消费判断

1. WebUI 请求封装以同源 HTTP API 为边界，未发现绕过后端直连 Provider/DB/Redis 的代码证据。
2. WebUI 通过数据侧 API 读取质量状态、来源、时间和错误，不在本批检查到的 API Client 层复制 Provider 选择/标准化逻辑；具体页面展示语义不属于本维度本批次的全量审查范围。
3. Request ID：浏览器不主动提交可信 Request ID；Nginx 生成并覆盖上游 Request ID。API Client 在失败时读取错误 envelope 的 request_id 或响应头 `X-Request-ID`，并限制长度；成功响应没有统一的 Request ID 暴露给调用方。本批不把成功请求 ID 未暴露单独登记为缺陷，后续如需前端成功请求诊断，应由项目级可观测性契约明确需求。
4. 当前双前缀、双后端代理和浏览器 Key 都是迁移期架构；目标契约为 Root Server 对外提供模块化服务 API。该差距与 **AUD-PLAT-PORT-014** Root Server 组合根迁移缺口相关，不重复创建同一架构项。
5. 本批没有执行生产部署的真实浏览器认证联调；没有证据证明 Data API Key 认证与 Analysis Owner Bearer 在目标部署中均能由浏览器完成。

**WebUI 维度 1 状态：partial；same-origin API client and reverse-proxy implementation evidenced / target Root Server session-auth integration not evidenced。**

### 15.5 本批审计范围与未覆盖项

本批核验 API Client 核心请求封装、Nginx 部署代理、API Client 单测、Deployment Contract 测试、WebUI API 契约和项目级契约登记簿。未逐一复核所有页面模块的字段映射与每条 API 的 OpenAPI/Handler/JSON Golden；这属于维度 2（DTO/Schema/Golden）及跨端 API-01 接口矩阵的后续任务。不得将本批结论扩大为 WebUI 全部 API 契约已审计。


## 16. 第十一批：WebUI 维度 2（Analysis API DTO/OpenAPI 映射初审）

### 16.1 核验范围与证据

固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`

- `web/src/api.js`（blob `b8bcbee5908b43bfa1bc2ca744363190af38287f`）
- `web/src/task-workflow.js`（blob `5bd115ed4600f180b6dfea0ab8343553c6ab63ff`）
- `web/tests/api.test.mjs`（blob `a26eca75b24a744232d370fdbe2411806be45136`）
- `doc/api/contracts/分析任务.openapi.yaml`（blob `aa8f153f25c46649dcdf187d5fef8f376113577b`）
- `internal/server/bootstrap/router.go`（blob `2597c998236a8e981773635eced7915a28d6b2ed`）

本批只审查 Analysis Run 查询路径在 WebUI、Root Server Router 与 OpenAPI 之间的映射，不代表已完成所有任务 DTO 或事件字段的逐字段核验。

### 16.2 已确认映射

1. WebUI `getAnalysisRun(settings, runId)` 调用 `GET /api/v1/analysis/runs/{run_id}`；路径参数使用 `encodeURIComponent`。
2. Root Server Router 注册相同的 `GET /api/v1/analysis/runs/{run_id}`，并应用 OwnerAccess Policy。
3. `web/tests/api.test.mjs` 覆盖 Run 查询路径、结构化错误 Code、HTTP Status、Request ID 提取，以及错误响应体长度限制。
4. WebUI 事件页校验器要求响应包含 `task_id` 和 `events`，并验证 `next_after_sequence` 单调推进；事件序号与分页游标分开处理。此处只证明前端本地防御性校验，不替代服务端 DTO/Schema 证据。
5. OpenAPI 当前定义任务创建、任务状态和阶段事件三类路径，没有定义 `GET /api/v1/analysis/runs/{run_id}`，也没有定义该接口的 Run 响应 Schema。Root Server Router 与 WebUI API Client 均已使用该路径，因此文档描述不完整。

### 16.3 差异登记

**AUD-WEBUI-CTR-003：Analysis Run 查询接口未纳入 OpenAPI（P2，契约文档缺口）**

- 生产路由证据：`internal/server/bootstrap/router.go` 注册 `GET /api/v1/analysis/runs/{run_id}`。
- 消费者证据：`web/src/api.js` 的 `getAnalysisRun` 调用该路径；`web/tests/api.test.mjs` 覆盖调用与错误映射。
- 契约证据：`doc/api/contracts/分析任务.openapi.yaml` 仅包含 tasks 的 POST/GET/events 路径，没有 Run 查询路径及响应 Schema。
- 影响：OpenAPI 无法完整生成或验证 WebUI 当前使用的 Analysis API 客户端契约；调用代码与运行时路由存在，但契约文档缺失。
- 责任方：旧 Analysis API / Server 契约维护方。
- 关闭条件：补充 Run 查询路径、响应 DTO 字段、状态码、错误响应与 Golden/契约测试；确认 WebUI 仅依赖已声明字段。
- 定性边界：这是 P2 文档/Schema 完整性问题，不据此认定运行时路由缺失或服务不可用。

### 16.4 本批未覆盖

- 未逐字段核对 `AnalysisRequest`、`Task`、`TaskEvent` 与 Go DTO/JSON 序列化结构。
- 未核对创建任务 payload 的所有页面构造点及历史兼容字段。
- 未核对 Market Data API 所有前端函数与 Handler 请求/响应 DTO 的逐字段映射。
- 未发现本批有可作为 Run 响应 Golden 的独立契约测试证据；需后续专门扫描。
- WebUI 维度 2 保持 partial，不得将此单条 Run 路径核验扩展为整个 API 面已完成。


## 17. 第十二批：WebUI 维度 2（全市场分页股票 DTO 映射）

### 17.1 固定基线与证据

- 代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`
- `web/src/api.js`（blob `b8bcbee5908b43bfa1bc2ca744363190af38287f`）
- `web/src/market-stocks.js`（blob `af32c747c93b3a414a6675baeed2f13a1e7670f4`）
- `web/tests/market-stocks-navigation.test.mjs`（blob `432633fb402de33801560b34942a716b6285471c`）
- `web/tests/api.test.mjs`（blob `a26eca75b24a744232d370fdbe2411806be45136`）
- `internal/marketdata/httpapi/market_stocks.go`（blob `c18ff1424f99d56905593c498f1aabc7ea871ad4`）
- `internal/marketdata/model/market_stocks.go`（blob `57d94b9030e9330dceb8597e7f6371fb4c997a0c`）
- `internal/marketdata/httpapi/openapi.yaml`（blob `cd8a6e3dfb8e3162ee789b11e883be9283c59f32`）
- `internal/marketdata/httpapi/openapi_contract_test.go`（blob `7f97f075a23bd698a7e3fc32e90509d1e999217b`）

### 17.2 请求参数映射

1. WebUI `getMarketStocks` 发送 `page`、`page_size`、`include_ma`、`adjustment`；默认值为 1、50、true、forward_adjusted。
2. WebUI 将 page 限制到 1–10000、page_size 限制到 1–100；复权值限制为 none/forward_adjusted/backward_adjusted，未知值回退 forward_adjusted。
3. Handler 使用相同的默认值和边界；对非法 page/page_size/include_ma/adjustment 返回 400，而不是静默扩大查询范围。
4. 页面自身将 page size 限制为 20/50/100，并始终请求 includeMA=true；用户选择的复权方式传给 API Client。
5. 该接口只返回当前页，不在浏览器自动遍历全市场；页面按服务端目录顺序展示，不对单页数据做伪全市场排序。

### 17.3 响应 DTO 与字段语义

1. Go `MarketStocksSnapshot` 与 OpenAPI `MarketStocksSnapshot` 字段集合及 required/nullable 语义由 `TestAllGoModelFieldsAndRequiredMatchOpenAPISchemas` 反射测试约束。
2. 聚合字段包括 page/page_size/total/total_is_lower_bound/returned、当前页 quote_succeeded/quote_failed/coverage_ratio、include_ma/ma_adjustment_basis、目录来源/版本/获取时间、行情获取时间、items 和 quality_status。
3. 单条记录包括 symbol/code/name/exchange、可空 Quote、MA5/MA20 指针、MA 复权口径、可选 MA Provider/ErrorCode、质量状态和质量原因。
4. `coverage_ratio` 明确是当前页覆盖率；`total_is_lower_bound` 表示目录总量仅为下界。页面按比例乘 100 展示百分比，并在总量为下界时显示“至少”。
5. Quote、MA 等缺失值按 null 处理并显示“—”，不补零；页面从单条记录或嵌套 Quote 读取质量状态/原因，未提供时回退 unknown/明确提示。
6. 页面在刷新期间保留上次成功快照并标记旧页；本次请求失败时清空列表，避免把旧数据当作新结果。连接配置变化时取消旧请求并使旧快照失效。
7. 页面筛选/导出复用当前页快照时检查 generation/busy 状态；浏览器测试覆盖刷新期间禁止继续使用旧快照。

### 17.4 契约测试判断与待补证

- Market Data OpenAPI 声明该接口参数和响应 Schema；Go OpenAPI Contract Test 检查路由清单、参数类型/默认值，并对 51 个 Go Model 进行字段集合、required 和 pointer-nullable 对照。
- 当前 WebUI `api.test.mjs` 对 `getMarketStocks` 的覆盖集中在超时/Abort 行为，没有直接断言四个查询参数的序列化结果、默认值和非法输入归一化；`market-stocks-navigation.test.mjs` 主要覆盖页面入口与状态交互，不等价于 API DTO Golden。
- 未发现本批接口有独立的 WebUI 请求/响应 Golden；页面读取响应的字段语义已有代码证据，但前端契约测试仍有缺口。

**AUD-WEBUI-CTR-004：全市场分页 API Client 缺少请求参数契约测试（P2，测试补证项）**

- 当前实现的请求参数与 Handler/OpenAPI 约束一致，未发现已证实的运行时参数错配。
- 缺口是前端测试没有固定 page/page_size/include_ma/adjustment 的 URL 序列化、默认值、边界归一化和非法复权值回退行为。
- 关闭条件：补充 API Client 单测，覆盖默认值、上下界、复权枚举、URL 编码与服务端 400 不被误吞；必要时增加一份最小响应 Fixture 验证字段空值/质量语义。
- 不要求在 WebUI 复制 Go 领域质量判断；只验证客户端传输和消费契约。

### 17.5 本批结论

全市场分页股票接口的请求参数、Go JSON DTO、OpenAPI Schema 和页面字段语义整体对齐；服务端 Go OpenAPI Contract Test 提供较强的模型字段/必填/null 约束。当前未发现字段语义错配。前端 API Client 请求参数断言不足，登记 AUD-WEBUI-CTR-004（P2）。WebUI 维度 2 仍为 partial。


## 18. 第十三批：WebUI 维度 2（Analysis Task 创建请求与 DTO Schema）

### 18.1 固定基线与证据

代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`

- `internal/analysis/domain/types.go`（blob `f13ceaa6cf4474413072db898d4c2043347ac410`）
- `internal/analysis/app/runner.go`（blob `3866ebafdf0a76d9887966b339b350c47c15f82f`）
- `doc/api/contracts/分析任务.openapi.yaml`（blob `aa8f153f25c46649dcdf187d5fef8f376113577b`）
- `web/src/api.js`（blob `b8bcbee5908b43bfa1bc2ca744363190af38287f`）
- `web/src/app.js`（blob `324348f063d89c4a3861ef2dd38232a9253fe0cd`）
- `web/src/task-workflow.js`（blob `5bd115ed4600f180b6dfea0ab8343553c6ab63ff`）

### 18.2 已确认的请求消费行为

1. WebUI 当前创建任务只发送 `{task_type, symbols}`；`createAnalysisTask` 将 payload 原样 JSON 提交至 `POST /api/v1/analysis/tasks`，并可选传递 `Idempotency-Key`。
2. WebUI 对创建响应只在 Task ID、Run ID、状态、标的集合等关键字段有效时更新对应状态；POST 响应不完整时保留请求上下文，后续通过 Task 状态接口补齐权威字段。
3. WebUI 的阶段事件消费者要求 `task_id`、`events`、`next_after_sequence` 结构有效；序号游标必须前进，事件按 Task ID 隔离。若事件包含 `run_id`，再校验其与当前 Run 一致；Domain StageEvent 的 `run_id` 并非必填，因此前端不将字段缺失误判为跨 Run 事件。
4. Domain `RunRequest` 的 JSON 字段包含 `task_type`、`as_of`、`as_of_mode`、`universe_id`、`symbols`、`portfolio_id`、`strategy_ids`、`market_id`、策略/风险/交易规则/数据策略版本以及 `parameters`；其中 `run_id` 为 omitempty。
5. Domain `TaskType` 明确定义 post_market_scan、watchlist_scan、position_review、historical_replay 四种值。Runner 对标的数量/格式及任务类型执行校验。

### 18.3 OpenAPI 差异

OpenAPI 的 `AnalysisRequest` 描述声称字段定义与 `domain.RunRequest` 一致，但当前 Schema：
- 漏掉 `as_of_mode`；
- 漏掉 `parameters`；
- `task_type` 未声明四种枚举值；
- 没有声明请求字段的 required 集合，无法表达 Runner 所需的关键字段约束。

WebUI 当前只发送 `task_type` 和 `symbols`，因此上述差异不证明现有 WebUI 创建请求会失败；但契约文档对完整 API 请求能力的描述不准确，且无法为未来显式时点/参数化请求生成可靠客户端。

**AUD-WEBUI-CTR-005：AnalysisRequest OpenAPI Schema 与 Domain JSON 契约不完整（P2）**

- 依据：OpenAPI 描述声称与 `domain.RunRequest` 一致，但字段集合缺少 `as_of_mode`、`parameters`，任务类型枚举和必填约束也未表达。
- 影响：契约生成、请求校验和未来调用方无法依据 OpenAPI 完整构造合法请求；当前 WebUI 最小请求 `task_type + symbols` 不受本差异直接影响。
- 责任方：旧 Analysis API / Server 契约维护方。
- 关闭条件：明确 HTTP 请求 DTO 与 Domain RunRequest 的映射边界；若确实使用同一 JSON DTO，补齐字段/枚举/required/边界；若 HTTP DTO 有意收窄，则修改 OpenAPI 描述并定义独立 Request Schema，增加序列化/契约测试。
- 不应简单把整个 Domain 类型无条件暴露为 HTTP DTO；应先明确 API 层允许调用方控制的字段，尤其是 as_of、配置版本与 parameters 的权限和默认值语义。

### 18.4 本批结论

WebUI 当前创建请求的最小字段与调用路径明确；事件分页消费者有结构和身份防御。AnalysisRequest OpenAPI 与其声称对应的 Domain JSON 字段不完整，新增 AUD-WEBUI-CTR-005（P2）。尚未完成 Task/TaskEvent 响应 DTO 的逐字段核验，也未检查服务端 JSON 解码是否严格拒绝未知字段；维度 2 继续保持 partial。


## 19. 第十四批：WebUI 维度 2（Task / TaskEvent 响应 DTO）

### 19.1 固定基线与证据

代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`

- `internal/platform/taskqueue/queue.go`（blob `d438d934c6267ee1d7a14d482d37741cf3d1c795`）
- `internal/platform/taskqueue/queue_test.go`（blob `6ff46bc8410cd72f1adb0077b21dc257f0a5583e`）
- `internal/analysis/domain/types.go`（blob `f13ceaa6cf4474413072db898d4c2043347ac410`）
- `doc/api/contracts/分析任务.openapi.yaml`（blob `aa8f153f25c46649dcdf187d5fef8f376113577b`）
- `web/src/task-workflow.js`（blob `5bd115ed4600f180b6dfea0ab8343553c6ab63ff`）
- `web/src/task-event-view.js`（blob `935050409e1d130cb9e901156c8075c1bdcb24d7`）

### 19.2 Task 状态和字段映射

1. Go `taskqueue.Task` JSON 字段：task_id、run_id、status、task_type、current_stage、completed_stages、created_at、started_at、finished_at、failure_code、error_message。
2. OpenAPI Task Schema 声明相同字段；required 为 task_id、run_id、status、task_type、completed_stages、created_at。Go 的 current_stage/failure_code/error_message 使用 `omitempty`，与非 required 对齐。
3. Go `StartedAt` / `FinishedAt` 是 `*time.Time` 且使用 `omitempty`；无值时序列化为字段缺失，而不是 JSON null。OpenAPI 对这两个字段设置 `nullable: true`，允许 null，和当前 Go 序列化器的输出形态不完全一致。
4. `taskStatusForRun` 将 completed / blocked_data_quality 映射 succeeded，partial 映射 partial，failed / rejected_request / 非终态 running / 未知状态映射 failed。单测覆盖这些分支；OpenAPI Task status 枚举 queued/running/succeeded/partial/failed 与 Worker 生成值一致。
5. `CreatedAt` 为非指针 time.Time 且 required；StartedAt/FinishedAt 为可选指针。时间值使用 Go time 标准 JSON 编码，OpenAPI 声明 date-time。

**AUD-WEBUI-CTR-006：Task 时间字段 nullable 与 omitempty 输出语义不一致（P2）**

- 证据：Go Task 的 StartedAt/FinishedAt 为指针并带 omitempty；OpenAPI 两字段声明 nullable=true。
- 影响：Schema 允许 null，但当前 Go DTO 在无值时省略字段；客户端若区分“缺失”和“显式 null”，会得到不同语义。
- 责任方：旧 Analysis API / Server 契约维护方。
- 关闭条件：二选一统一：若 API 语义为缺失，移除 nullable；若语义为显式 null，调整 DTO 序列化并增加 JSON Golden/契约测试。前端应对缺失与 null 均保持兼容，直到契约冻结。

### 19.3 TaskEvent 与分页游标

1. Go Event 是 `{sequence:int64,event:domain.StageEvent}`；StageEvent 字段为 run_id、stage、status、at、duration_ms、message、attributes。
2. StageStatus 当前枚举值为 started、succeeded、failed、skipped；可选字段 duration_ms/message/attributes 使用 omitempty。
3. OpenAPI TaskEvent 仅声明 sequence 和一个无属性约束的 object event，并用文字称其为原始 domain.StageEvent；没有表达 StageEvent 字段、status 枚举、时间格式及可选字段语义。
4. OpenAPI TaskEventsResponse 要求 task_id、events、next_after_sequence；WebUI 校验器检查 task_id 与当前任务一致，events 为数组，next_after_sequence 为安全整数且不回退；非空页必须推进游标，游标不能小于本页最大事件序号。
5. WebUI 对每条事件校验 sequence 为正安全整数、event 为对象；可选 run_id 存在时必须与当前 Run 匹配。渲染器消费 stage/name、message、status、at、duration_ms、error_code，并对缺失状态/说明使用明确回退文案。
6. 本轮已核对 Queue.Events 的 sequence 使用持久化事件行 ID、按升序读取并按 after 过滤；但尚未核对 HTTP Handler 计算 next_after_sequence 的完整实现及其边界测试，不能将前端游标校验视为服务端分页正确性的证明。
7. OpenAPI 将 event 留作任意 object，且缺少 StageEvent Golden/字段反射校验的证据。本轮没有发现前端把可选 run_id 当作必填，也没有发现已证实的字段错配。

**AUD-WEBUI-CTR-007：TaskEvent.event 缺少可验证的 StageEvent Schema（P2）**

- 证据：Go Event 明确嵌入 domain.StageEvent；OpenAPI event 只有 type: object 和描述，没有 properties/required/enum/format。
- 影响：OpenAPI 生成客户端只能把事件正文视为任意对象，无法验证 stage/status/at 等字段及其兼容性；WebUI 也无法通过 Schema 工具检查服务端变更。
- 责任方：旧 Analysis API / Server 契约维护方。
- 关闭条件：定义独立 HTTP StageEvent Schema（或明确引用等价组件），至少覆盖 run_id、stage、status、at、duration_ms、message、attributes 的 required/nullable/omitempty 和状态枚举；增加 Go JSON Golden 与 OpenAPI 一致性测试。
- 注意：当前前端渲染器还读取 error_code，但 Go domain.StageEvent 没有该字段；本批未找到 HTTP 层额外 DTO 注入 error_code 的证据。需在 Handler/历史事件 JSON 核验中确认是否为历史兼容字段，不在本轮直接判定运行时缺陷。

### 19.4 本批结论与边界

- Task 字段集合和状态枚举总体对齐；taskStatusForRun 有覆盖终态和异常状态的单测。
- Task 的可选时间字段存在 nullable/omitempty 语义差异，登记 AUD-WEBUI-CTR-006（P2）。
- TaskEvent 的外层序号/事件结构明确，但 OpenAPI 没有描述 StageEvent 的实际字段和枚举，登记 AUD-WEBUI-CTR-007（P2）。
- WebUI 事件分页客户端的游标防御已核；服务端 Handler 的 next_after_sequence 计算、Task/TaskEvent JSON Golden、未知字段解码策略仍待核。
- WebUI 维度 2 继续保持 partial；本批不代表全部 Task API 及历史事件兼容性已审完。


## 20. 第十五批：WebUI 维度 2（TaskEvent 服务端分页与事件字段）

### 20.1 固定基线与证据

代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`

- `internal/platform/httpapi/tasks.go`（blob `43eb8baccd820c63e23ffd6a2a501e78b2b880b6`）
- `internal/platform/httpapi/tasks_test.go`（blob `31374cbdb3e38fdfefc4b407c648668f70a1e5c5`）
- `internal/platform/taskqueue/queue.go`（blob `d438d934c6267ee1d7a14d482d37741cf3d1c795`）
- `web/src/task-event-view.js`（blob `935050409e1d130cb9e901156c8075c1bdcb24d7`）

### 20.2 服务端分页规则

- Handler 默认 after_sequence=0、limit=100；after_sequence 解析为 int64 且拒绝负值，limit 只接受 1～500。
- Handler 调用 taskService.Events 后，以本页最后一条事件的 Sequence 作为 next_after_sequence；空页时保持请求 after 值。
- Queue.Events 先确认 task 存在，再查询同一 run 的事件；SQL 条件为 id > after，按 id ASC 排序并 LIMIT。返回 Event.Sequence 使用该数据库行 id。
- 因而当前分页采用“排他游标”：客户端传入上一页最后一条 sequence，下一页只取更大的 sequence；空页不会使游标回退。
- Handler 单测 TestGetTaskEventsSupportsIncrementalSequence 仅覆盖 after=4 返回 sequence=7 且 next=7 的单页场景。未覆盖空页保持游标、连续两页、limit 边界、负数/非法游标、非法 limit、超过 int64 的游标及满页行为。
- 当前证据足以确认 Handler/Queue 的分页算法和单页推进规则；但没有定位到创建 analysis_stage_events 表的迁移定义，故不将数据库 id 的自增/不可变约束视为已核验。sequence 长期稳定性仍需补充 Migration/DDL 证据。

**AUD-WEBUI-CTR-008：TaskEvent 分页边界缺少服务端契约测试（P2）**

- 证据：HTTP Handler 只测试一组非连续序号的单页增量读取；分页算法包含空页游标保持、limit 和游标解析分支，但没有对应边界测试。
- 影响：后续修改分页参数解析、空页游标或连续读取行为时，缺少测试防止重复事件、漏事件或游标回退。
- 责任方：旧 Analysis API / Server 契约维护方。
- 关闭条件：补充空页保持游标、两页连续读取无重复/遗漏、limit=1/500、limit=0/501、负数/非法/溢出 after_sequence 等测试；并通过真实 DDL 或 Migration 测试证明事件 ID 的唯一、单调和不可变语义。

### 20.3 前端 error_code 字段核验

- WebUI `renderSelectedTaskEventView` 尝试显示 `event.error_code`，但当前 Go `domain.StageEvent` 仅有 run_id、stage、status、at、duration_ms、message、attributes，没有 error_code。
- 当前没有发现 HTTP Handler 对事件补充 error_code；Queue.Events 直接反序列化并返回 domain.StageEvent。
- JavaScript 对 undefined 字段会跳过显示，因此这不是 JSON 解码或页面运行时错误；但当前服务端 DTO 无法按该字段为前端提供错误代码。
- 暂不新增独立差异：待 StageEvent Schema 收敛时，由 API 责任方决定是否正式增加 error_code，或移除前端无数据来源的展示项。不得把 attributes 中可能存在的同名键推定为顶层字段。

### 20.4 本批结论与未完成项

- Handler 的排他游标、空页保持、Queue 的 ID 升序读取规则已核实；没有发现当前算法会主动回退游标。
- 新增 AUD-WEBUI-CTR-008（P2），性质为测试补证，不是已证实运行时分页错误。
- 数据库事件 ID 的 DDL 约束、Task/TaskEvent JSON Golden、未知字段解码策略仍待核。
- WebUI 维度 2 继续保持 partial。


## 21. 第十六批：WebUI 维度 2（日 K 响应消费与 Go/OpenAPI 空值语义）

### 21.1 固定基线与证据

代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`

- `internal/marketdata/model/model.go`（blob `6f04516bd11452a01adcb5acceab571fb1c28ed5`）
- `internal/marketdata/httpapi/server.go`（blob `7b54b54b3c6182ecab830c2c43f91c548dc60dba`）
- `internal/marketdata/httpapi/openapi.yaml`（blob `cd8a6e3dfb8e3162ee789b11e883be9283c59f32`）
- `internal/marketdata/httpapi/openapi_contract_test.go`（blob `7f97f075a23bd698a7e3fc32e90509d1e999217b`）
- `internal/marketdata/service/data.go`（blob `d2abb990d8d7ec2fcbf112a5f1beca49c8d40f7b`）
- `web/src/api.js`（blob `b8bcbee5908b43bfa1bc2ca744363190af38287f`）
- `web/src/app.js`（blob `324348f063d89c4a3861ef2dd38232a9253fe0cd`）
- `web/src/chart.js`（blob `15a9c606dc2919c5d1c398ec18887e1d5e230dc7`）
- `web/tests/chart.test.mjs`（blob `fca5a36c8e56e1a766e052819cb09cea6845d6f4`）
- `web/tests/api.test.mjs`（blob `a26eca75b24a744232d370fdbe2411806be45136`）

### 21.2 请求参数与 Handler 行为

1. WebUI `getDailyBars` 默认 `limit=120`、`adjustment=forward_adjusted`，始终序列化 limit/adjustment；start/end 非空时追加查询参数。
2. WebUI 自定义日期范围时调用日 K API；未指定日期范围时优先调用 Bundle，只有 Bundle 返回 404 才回退到 Quote + DailyBars 两个请求。
3. Handler 的 adjustment 缺省值来自 Server 配置，随后通过 Adjustment 规范化校验；合法请求口径为 none、forward_adjusted、backward_adjusted。
4. Handler 的 limit 必须是整数，范围为 1 至配置的 maxBars；省略时由 Service 使用 defaultBars。Service 默认 defaultBars 为 120，配置不一致时需结合实际部署配置判断，不把构造参数默认值等同于所有部署的固定值。
5. Handler 接受可选 RFC3339Nano 的 as_of；拒绝重复、空白、非法和未来时间。PIT 查询使用 DailyBarsAsOf，并在响应中增加 point_in_time=true、as_of 和逐条 system_observed_at；存储未启用和没有可用修订分别返回稳定错误码。
6. 普通日 K 与 PIT 日 K 共用 DailyBarsResponse 的主体字段；point_in_time/as_of 为条件输出字段，Schema 未要求所有响应都必须出现。

### 21.3 DailyBar Go JSON 与 OpenAPI

1. Go DailyBar 的 OHLC 是非指针 float64，OpenAPI 对应 number 且非 nullable；trade_date 为 string/date，period 为 1d。
2. MA5/MA20/MA120/MA250、volume、amount、change_percent 是非 omitempty 指针；OpenAPI 对应字段声明 nullable=true，Go 序列化会在无值时输出 null。
3. adjustment_basis、source、fetched_at、normalized_at、volume_unit、amount_unit、quality_status、normalization_version 等必需字段均在 Go JSON 与 OpenAPI 中定义。
4. is_partial、调整因子重建元数据、provider_timestamp、system_observed_at、原始量额、fallback 信息、request_id 等 Go omitempty 字段，在 OpenAPI 中作为可选字段定义。
5. DailyBarsResponse 要求 symbol、period、adjustment_basis、volume_unit、amount_unit、quality_status、warnings、bars；point_in_time/as_of 是可选字段，符合 Handler 普通查询与 PIT 查询的条件输出方式。
6. AdjustmentBasis 响应枚举包含 unknown；RequestAdjustmentBasis 不包含 unknown，且 Schema 明确 unknown 只是响应状态，不能作为请求参数。与 Handler 的请求校验方向一致。
7. Go Contract Test 的模型反射测试覆盖 Quote、DailyBar 等 51 个模型的字段集合、required 与非 omitempty pointer-nullable 语义；JSON 序列化测试进一步覆盖 Quote/DailyBar 的字段出现、omitempty 缺失和 null 输出。该证据比仅检查 OpenAPI YAML 存在更强。

### 21.4 WebUI 消费行为

1. `normalizeDailyBars` 只消费绘图所需的日期、OHLC、成交量、成交额、涨跌幅、复权口径、来源、质量状态/原因、部分 K 线标记和 fallback 来源。
2. 日期不符合 YYYY-MM-DD 的记录被排除；数据按 trade_date 升序排列。
3. 数值解析拒绝 null、undefined、空字符串和非有限数字；OHLC 非正值视为不可绘制；volume/amount 只接受非负值。缺失值保留 null，不补零。
4. K 线图允许成交量缺失，显示“成交量未报告”；OHLC 内部关系不一致时不自动修复、不绘制该条 K 线。
5. 涨跌幅按百分点数值消费，图表展示时附加百分号；与 OpenAPI 中 1.25 表示 1.25% 的语义一致。
6. 技术指标由前端使用有效收盘价计算 MA5/20/60/120/250、RSI 和 MACD；没有把 DailyBar 响应中的 ma5/ma20/ma120/ma250 当作前端指标计算输入。该行为是现有消费实现，不构成字段遗漏或 Schema 不匹配。
7. `chart.test.mjs` 覆盖升序、缺失值保留、非法 OHLC 拒绝、成交量缺失、指标样本数不足及均线覆盖层选择。API 测试覆盖日 K URL、limit/adjustment/start/end 序列化和超时行为。

### 21.5 差异判断与边界

- 本批未发现 DailyBar Go JSON 字段集合、required、nullable 与 OpenAPI 的已证实差异。
- 本批未发现 WebUI 对日 K 空值、成交量单位、成交额单位、涨跌幅或复权口径的已证实误读。
- 前端本地指标计算与服务端 DailyBar 内含均线并存；两者当前没有字段冲突，但后续应避免把前端计算指标误称为服务端返回值。
- 默认 limit=120 时，前端无法仅凭该批数据计算需要 250 个有效收盘样本的 MA250；用户可调整 limit。该点属于查询窗口/指标可用性，不是接口契约错配。本批不新增差异编号。
- 本批未完成所有行情接口（Bundle、证券详情、指数日 K 等）的逐字段核验，也未执行前端与真实服务端的联调；不据此宣称 WebUI 维度 2 完成。

### 21.6 本批结论

日 K 请求参数、Handler 分支、Go DailyBar JSON、OpenAPI Schema 与 WebUI 图表消费逻辑整体对齐。服务端已有模型反射和 JSON 序列化契约测试，前端已有日 K 图表行为测试。当前未发现需要新增 AUD-WEBUI-CTR 差异项；WebUI 维度 2 保持 partial，下一批转向 Bundle 响应 DTO 与其前端消费链。


## 22. 第十七批：WebUI 维度 2（Bundle 聚合响应 DTO 与快照元数据）

### 22.1 固定基线与证据

代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`

- `internal/marketdata/model/model.go`（blob `6f04516bd11452a01adcb5acceab571fb1c28ed5`）
- `internal/marketdata/httpapi/server.go`（blob `7b54b54b3c6182ecab830c2c43f91c548dc60dba`）
- `internal/marketdata/httpapi/openapi.yaml`（blob `cd8a6e3dfb8e3162ee789b11e883be9283c59f32`）
- `internal/marketdata/httpapi/openapi_contract_test.go`（blob `7f97f075a23bd698a7e3fc32e90509d1e999217b`）
- `internal/marketdata/service/data.go`（blob `d2abb990d8d7ec2fcbf112a5f1beca49c8d40f7b`）
- `web/src/api.js`（blob `b8bcbee5908b43bfa1bc2ca744363190af38287f`）
- `web/src/app.js`（blob `324348f063d89c4a3861ef2dd38232a9253fe0cd`）

### 22.2 Bundle 请求与服务端行为

1. WebUI 无自定义日期范围时优先调用 `GET /api/v1/bundles/{symbol}`，传入 lookback_days 和 adjustment；当前调用点显式构造 lookbackDays，范围为 20 至 2000。
2. Handler 对 lookback_days 执行整数和范围校验（20 至当前 Server 配置的 maxBars）；省略时传 0 给 Service，由 Service 使用 defaultBars。
3. OpenAPI 将 lookback_days 声明为 integer、minimum 20、maximum 2000；未声明 default，也未描述省略时由服务端 defaultBars 配置决定。
4. Service 当前默认构造配置下 defaultBars 为 120，但该值通过构造参数注入，不是 Bundle Handler 固定常量。WebUI 当前显式传入窗口，不依赖省略默认值。
5. adjustment 缺省使用 Server 配置默认值，并按 RequestAdjustmentBasis 校验；OpenAPI 使用同一请求枚举。
6. as_of 按带时区 RFC3339 时间解析。历史 cutoff 不会直接泄漏最新行情：仅在请求时间接近当前且 QuoteTime 可证明不晚于 cutoff 时保留 Quote；否则省略 Quote 并增加 warning/degraded 状态。
7. PIT Bundle 会按上海时区 cutoff 处理当日未收盘 K 线；复权历史可能使用当前公司行为因子时，Service 明确增加 warning 并降级，避免把非 PIT 复权结果误认为完全历史可复现。
8. Bundle 会额外请求 MA250 预热窗口，最终仍按 lookback 截断；Quote 与 DailyBars 复权口径不一致时增加警告并降级。来源混合、缺失日 K、均线历史不足等情况也会影响质量状态。

### 22.3 Bundle DTO 与 OpenAPI

1. Go Bundle 字段为 symbol、name、quote、daily_bars、sources、warnings、snapshot_id、analysis_reference_time、fetched_at、quality_status。
2. name、quote、warnings 使用 omitempty；其余字段要求输出。OpenAPI required 集合与 Go JSON 序列化语义一致。
3. quote 是可选对象，不是显式 nullable 字段；这与 Go `*Quote + omitempty` 的缺失语义一致。
4. daily_bars 是 DailyBar 数组；sources 是 string-valued map；warnings 是字符串数组；snapshot_id、analysis_reference_time、fetched_at、quality_status 均有 Schema。
5. Go 模型反射测试将 Bundle 纳入 51 个 Model 的字段集合和 required 对照；JSON 序列化测试明确检查 symbol/daily_bars/sources/snapshot_id 出现，name/quote/warnings 省略。反射测试同时检查剩余 required 字段，避免只依赖上述局部断言。

### 22.4 WebUI 消费行为

1. WebUI 对 Bundle 顶层 symbol 执行请求标的身份校验；若嵌套 Quote 存在，也独立校验 Quote.symbol。顶层不匹配时丢弃整个 Bundle，Quote 不匹配时仅剔除 Quote 并追加警告。
2. 页面使用 daily_bars 渲染日 K；从首条 bar 读取单位，从记录中读取 adjustment_basis；Bundle 顶层 quality_status 和 warnings 决定质量提示。
3. Quote 缺失时清空实时行情区域，并提示 Bundle 已返回多少根日 K 但没有实时行情，不把缺失 Quote 当作请求整体成功的完整行情。
4. 页面展示 snapshot_id、analysis_reference_time 和 sources；sources 以键值形式呈现，超过展示上限时提示可复制完整元数据。
5. Bundle 失败时，只有 HTTP 404 会触发旧服务兼容回退到 Quote + DailyBars；其他聚合错误不会重复调用所有 Provider，避免额外放大请求。
6. Bundle 当前不向页面直接暴露 fetched_at 的独立展示字段；这不影响 DTO 契约，页面展示的 analysis_reference_time 与 snapshot_id 用于识别分析快照及其参考时点。

### 22.5 差异判断

**AUD-WEBUI-CTR-009：Bundle lookback_days 省略默认值未在 OpenAPI 说明（P2，契约文档补充）**

- 证据：Handler 对省略 lookback_days 传 0，Service 再使用注入的 defaultBars；当前默认构造配置下为 120。OpenAPI 参数仅声明 integer/minimum/maximum，没有 default 或省略语义描述。
- 影响：不传 lookback_days 的 API 调用方无法仅依据 OpenAPI 判断实际返回窗口；不同部署配置还可能使用不同 defaultBars。当前 WebUI 调用点显式传入 lookbackDays，不受影响。
- 责任方：Market Data HTTP API 契约维护方。
- 关闭条件：在 OpenAPI description 中明确“省略时使用服务端配置的默认窗口”，并说明默认值由部署配置决定；若项目决定固定为 120，则将默认值固定到构造配置并声明 default: 120，同时增加 Handler 默认行为契约测试。不要在配置仍可变时虚构一个固定 default。

本批未发现 Bundle DTO 字段集合、required/omitempty、Quote 缺失语义或 WebUI 快照消费的已证实运行时错配。AUD-WEBUI-CTR-009 是调用方可见的默认行为文档缺口，不是当前 WebUI 调用错误。

### 22.6 本批结论

Bundle 聚合 DTO 与 Go/OpenAPI 字段语义整体一致；WebUI 对顶层/嵌套证券身份、Quote 缺失、质量降级、快照来源及旧服务 404 兼容回退均有明确处理。新增 AUD-WEBUI-CTR-009（P2），仅针对 lookback_days 省略默认值文档不完整。WebUI 维度 2 继续保持 partial。


## 23. 第十八批：WebUI 维度 2（证券资料、历史交易状态与涨跌幅规则 DTO）

### 23.1 固定基线与证据

代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`

- `internal/marketdata/model/model.go`（blob `6f04516bd11452a01adcb5acceab571fb1c28ed5`）
- `internal/marketdata/model/security_trading_status.go`（blob `41b8c25b5a6a228dbd05550b4a25beecc060c009`）
- `internal/marketdata/model/trading_rules.go`（blob `eca0041346bb74eb7d2913868f30e459c0ef7adb`）
- `internal/marketdata/httpapi/security_trading_status.go`（blob `841cb0d4ca4133facb034a75aea0f5ba7813a678`）
- `internal/marketdata/httpapi/price_limit_rule.go`（blob `31414d48b4ef6c60a7058162964aa5454f439b9a`）
- `internal/marketdata/httpapi/server.go`（blob `7b54b54b3c6182ecab830c2c43f91c548dc60dba`）
- `internal/marketdata/httpapi/openapi.yaml`（blob `cd8a6e3dfb8e3162ee789b11e883be9283c59f32`）
- `internal/marketdata/httpapi/openapi_contract_test.go`（blob `7f97f075a23bd698a7e3fc32e90509d1e999217b`）
- `internal/marketdata/httpapi/security_trading_status_test.go`（blob `88d31cef968e114a52c8e463d0fa18db7b17297f`）
- `internal/marketdata/httpapi/price_limit_rule_test.go`（blob `fbaa4062b407afe9e2079594833b6ca09b007759`）
- `web/src/api.js`（blob `b8bcbee5908b43bfa1bc2ca744363190af38287f`）
- `web/src/stock-research.js`（blob `49fe406d7dcf5e8fdec76f2f0bfcda6a18d4db85`）

### 23.2 路由与调用边界

1. Server 注册 `GET /api/v1/stocks/{symbol}/profile`、`/trading-status`、`/price-limit-rule`。
2. Profile Handler 直接返回 `StockBasic`；Trading Status Handler 直接返回 `SecurityTradingStatusSnapshot`；Price Limit Handler 直接返回 `PriceLimitRuleSnapshot`。三者均未发现独立 HTTP DTO 转换层。
3. WebUI API Client 导出 `getStockProfile`、`getStockTradingStatus`、`getStockPriceLimitRule`。已检查的 `stock-research.js` 实际使用 Profile API；状态/规则 UI 面板在标的切换时会清空，但本批未证明两个状态/规则请求函数已接入 UI 事件链，调用方接线列为待核，不据此声称已完成端到端联调。
4. Profile API 的 WebUI 消费逻辑读取证券代码、名称、证券类型/标签、上市状态/标签、上市/退市日期、标准化版本、质量状态与原因、来源元数据。响应证券代码会与请求标的进行一致性校验。
5. Trading Status Handler 要求唯一、非空 YYYY-MM-DD date，拒绝未来日期；可选 as_of 必须为单值、有效且非未来 RFC3339Nano。提供 as_of 时只读取本地 PIT 版本，不回退实时 Provider；缺少存储或版本分别返回 503/404。
6. Price Limit Handler 同样要求唯一、非空且非未来日期；它先取历史交易状态，再取特殊无涨跌幅事件证据。任一证据不可用时仍返回 200，但将 status/quality 降为 unknown 并清除 limit_percent；不会把证据缺失当作正常限幅或 no_limit。

### 23.3 StockBasic 与 SecurityTradingStatusSnapshot

1. StockBasic 的 Go required 字段与 OpenAPI required 集合一致；`ipo_date`、`out_date`、`source_version`、`quality_reasons` 以及估值、市值、股本、行业、板块和盈利类可选字段均使用 `omitempty`。
2. OpenAPI StockBasic 将多项可选字符串和数值属性声明为 `nullable: true`。Go 对应字段为 string 或 pointer，且均带 `omitempty`；nil/空字符串实际序列化为字段缺失，不会输出显式 JSON null。
3. SecurityTradingStatusSnapshot 的 required 集合与 Go 非 omitempty 字段一致。唯一明确声明 nullable 的 `system_observed_at` 是 `*time.Time + omitempty`，nil 时序列化为字段缺失而非 null。
4. SecurityTradingStatusSnapshot 的风险警示、退市和停复牌状态均有独立 unknown 值；unknown 不等于 normal。OpenAPI 已声明对应枚举，保留了这一安全语义。
5. 当前 51 模型反射测试检查字段集合、required，以及非 omitempty pointer 必须 nullable；但没有反向检查 omitempty 字段不应被声明 nullable。其 JSON 序列化样例未覆盖 StockBasic 和 SecurityTradingStatusSnapshot。因此这些 nullable 过宽问题没有被现有测试捕获。

### 23.4 PriceLimitRuleSnapshot 与 PriceLimitEventEvidence

1. Go `PriceLimitRuleSnapshot` 非 omitempty 字段包括 symbol、market、board、trade_date、risk_warning_status、listing_status_known、special_exception_status、status、rule_version、rule_source、rule_source_url、quality_status、quality_reasons；这些字段均会出现在实际 JSON。
2. OpenAPI Price Limit Rule 的内联响应 Schema required 目前只有 symbol、trade_date、status、quality_status、quality_reasons，漏列 market、board、risk_warning_status、listing_status_known、special_exception_status、rule_version、rule_source、rule_source_url 八个 Go 必输出字段。
3. Go `RiskWarningStatus` 枚举为 unknown/normal/risk_warning；`PriceLimitException` 枚举为 unknown/none/relisting_first_day/delisting_first_day/exchange_specified。Price Limit Rule 内联 Schema 对 risk_warning_status 和 special_exception_status 只声明 string，未表达枚举；status 已声明 unknown/limited/no_limit。
4. `special_exception_evidence` 和 `limit_percent` 是 Go omitempty 指针；OpenAPI 将二者声明 nullable=true。nil 时 Go 会省略字段，而非输出 null。
5. PriceLimitEventEvidence 已进入 51 模型契约登记表，其 required 集合与 Go 非 omitempty 字段一致；status 枚举也已声明。其 omitempty 字段在 OpenAPI 中作为可选字段，没有额外 nullable 声明。
6. PriceLimitRuleSnapshot 本身没有进入 51 模型契约登记表；其 OpenAPI 响应仍是路径内联对象，现有模型反射和 JSON 序列化测试都没有覆盖该 DTO。当前专用 HTTP 测试主要覆盖 unknown 规则和非法日期，未见完整响应 Golden。

### 23.5 差异登记

**AUD-WEBUI-CTR-010：omitempty 响应字段在 OpenAPI 中被过度声明为 nullable（P2）**

- 影响对象：StockBasic 多项可选字段、SecurityTradingStatusSnapshot.system_observed_at、PriceLimitRuleSnapshot.limit_percent、StockBoard.change_percent、FundamentalsSnapshot.system_observed_at。StockBoard.change_percent 已在第 24 节补充核验，FundamentalsSnapshot.system_observed_at 已在第 25 节补充核验；special_exception_evidence 因 OpenAPI 3.0.3 的 $ref/nullable 写法不满足同层 type 条件，仍列为待核，不计入已证实范围。
- 证据：上述 Go 字段均使用 omitempty；OpenAPI 对部分字段声明 nullable=true；Go encoding/json 的 nil/空值行为是省略字段。
- 影响：客户端按 Schema 可能接受服务端永远不会发出的显式 null；缺失与 null 的语义被混淆，特别是生成式客户端和严格响应校验器。
- 责任方：Market Data HTTP API 契约维护方。
- 关闭条件：对响应 DTO 明确区分“可省略”和“可为 null”；对 omitempty 字段移除 nullable，只有 Go 实际输出 null 的非 omitempty 指针才保留 nullable；增加反向 Schema 检查，禁止 omitempty 字段无依据地 nullable。

**AUD-WEBUI-CTR-011：PriceLimitRule 响应 Schema required 与枚举约束不完整（P2）**

- 证据：Go PriceLimitRuleSnapshot 有八个非 omitempty 字段未进入 OpenAPI required；risk_warning_status、special_exception_status 也未声明 Go 已有枚举。
- 影响：OpenAPI 客户端无法依赖这些字段必然存在，也无法在生成客户端/校验器中限制未知状态字符串；该接口的契约描述弱于实际 Go 输出与规则状态模型。
- 测试缺口：PriceLimitRuleSnapshot 不在 51 模型反射登记表，且缺少完整响应 JSON Golden。
- 责任方：Market Data HTTP API 契约维护方。
- 关闭条件：将 PriceLimitRuleSnapshot 提升为可复用 Schema 或纳入等效的专用模型契约测试；required 与 Go 非 omitempty 字段一致；补齐 RiskWarningStatus、PriceLimitException 枚举；增加 JSON Golden/字段与 required 一致性测试。

### 23.6 本批结论

Profile、Trading Status、Price Limit Rule 路由与基础 Handler 行为已核对。新增 AUD-WEBUI-CTR-010、AUD-WEBUI-CTR-011，均为 P2 契约精度/测试覆盖问题；本批未证明运行时业务结果错误。Trading Status 与 Price Limit Rule 的 WebUI 请求函数到实际 UI 事件链仍待核。WebUI 维度 2 继续保持 partial。


### 23.7 Review 复核补充

对本节差异重新核对 OpenAPI 版本与 WebUI 实际消费代码后，作如下收敛：

1. OpenAPI 版本为 3.0.3。CTR-010 中 `special_exception_evidence` 的 `nullable: true` 与 `$ref` 同处一个对象，但没有同层 `type`；不能直接把它认定为有效的 nullable 声明，已从 CTR-010 的已证实影响字段中移除，并记录为 Schema 写法待核。
2. CTR-010 由 StockBasic 多项 string/number 字段、SecurityTradingStatusSnapshot.system_observed_at、PriceLimitRuleSnapshot.limit_percent 和 StockBoard.change_percent 支撑；这些属性在 OpenAPI 中直接声明 type 并标 nullable，而 Go 对应字段均为 omitempty，差异成立。
3. `stock-research.js` 实际 import 并调用 `getStockProfile`；没有 import `getStockTradingStatus` / `getStockPriceLimitRule`。两个 API Client 函数存在，但本文件中的交易状态/涨跌幅区域只在标的切换时重置占位内容，没有实际请求和响应渲染。结论限定为“本页面未接线”，不外推为整个 WebUI 所有入口均未使用。
4. CTR-011 的八个漏列 required 字段和两个缺失枚举均由 Go 结构体与 OpenAPI 内联响应 Schema 直接对照确认，差异成立。
5. 两项均为契约精度/测试覆盖问题，不是已证实的线上数据错误；P2 级别维持。



### 24. WebUI 维度 2：个股所属板块 StockBoards

#### 24.1 核验范围与证据

- 固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- Go Model：`internal/marketdata/model/model.go`（blob `6f04516bd11452a01adcb5acceab571fb1c28ed5`）。
- Provider Port：`internal/marketdata/provider/provider.go`（blob `6635d2247379d1ffcb826f80e8e2176422b0cc0a`）。
- Eastmoney Provider：`internal/marketdata/provider/eastmoney.go`（blob `5f2a8c2d0fb883414f870f6e29af51943d12c204`）。
- Provider 边界测试：`internal/marketdata/provider/eastmoney_stock_boards_test.go`（blob `a11d3663d0d1eeba75082c1a4560510de27a4749`）。
- Service 校验、缓存与回退：`internal/marketdata/service/data.go`（blob `d2abb990d8d7ec2fcbf112a5f1beca49c8d40f7b`）；回退测试 `internal/marketdata/service/stock_boards_fallback_test.go`（blob `d95da9ce4fd9c8e17ee27e33c94969549d2d84e9`）。
- HTTP 路由与 Handler：`internal/marketdata/httpapi/server.go`（blob `7b54b54b3c6182ecab830c2c43f91c548dc60dba`）。
- OpenAPI：`internal/marketdata/httpapi/openapi.yaml`（blob `cd8a6e3dfb8e3162ee789b11e883be9283c59f32`）。
- OpenAPI 契约测试：`internal/marketdata/httpapi/openapi_contract_test.go`（blob `7f97f075a23bd698a7e3fc32e90509d1e999217b`）。
- WebUI API Client：`web/src/api.js`（blob `b8bcbee5908b43bfa1bc2ca744363190af38287f`）。
- WebUI 页面消费：`web/src/stock-research.js`（blob `49fe406d7dcf5e8fdec76f2f0bfcda6a18d4db85`）。

#### 24.2 真实调用链

1. Server 注册 `GET /api/v1/stocks/{symbol}/boards`，Handler 调用 `s.securities.StockBoards(ctx, symbol)`，成功后透传 `StockBoardSnapshot` JSON，并按缓存命中设置缓存响应头；未发现独立 HTTP DTO 转换层。
2. Provider Port 为 `StockBoardsProvider.FetchStockBoards(ctx, symbol)`，返回统一的 `model.StockBoardSnapshot`。
3. 本基线可见的具体板块成员 Provider 实现为 Eastmoney `FetchStockBoards`，使用 slist 接口读取 f12 板块代码、f14 板块名称、f3 涨跌幅。代码限制为普通 A 股证券；响应 source_version 固定为 `eastmoney-slist-stock-boards-v1`。
4. DataService 对证券代码做标准化和请求/响应身份一致性校验；检查来源、版本、采集时间、数量关系、重复板块代码、条目质量及涨跌幅数值有效性。未知或空成员结果不会直接作为“无所属板块”，而会继续尝试后备 Provider；所有后备源耗尽后才保留 unknown 结果。
5. 已确认成员结果缓存 12 小时；未知/空结果缓存 5 分钟。Eastmoney 返回 1000 条时标记 `total_is_lower_bound=true` 和 degraded，避免声称已取得完整列表。
6. WebUI API Client 导出 `getStockBoards`，请求同一路径。个股研究页 import 并调用该函数；初次加载和按已请求面板刷新均会触发 boards 请求。通用 `fetchOne` 在渲染前校验响应 symbol 与当前请求证券一致。
7. `renderBoards` 实际读取 items、name、code、change_percent、source、source_version、fetched_at、quality_status、item/整体 quality_reasons；空列表且 `has_data=false` 时明确显示“数据源未确认”，不会解释为零板块。

#### 24.3 DTO 与 OpenAPI

1. Go `StockBoard` 的必输出字段为 code、name、source、fetched_at、quality_status；OpenAPI required 集合一致。source_version、quality_reasons 和 change_percent 均为 Go `omitempty` 字段，OpenAPI 也未将它们列入 required。
2. Go `StockBoardSnapshot` 的必输出字段为 symbol、total、provider_returned、excluded_rows、total_is_lower_bound、has_data、items、source、fetched_at、quality_status；OpenAPI required 集合与 Go 非 omitempty 字段一致。source_version、quality_reasons 均为可省略字段。
3. OpenAPI `StockBoard.change_percent` 声明为 `type: number, nullable: true`，但 Go 字段是 `*float64 + omitempty`。无涨跌幅时 encoding/json 会省略字段而不是输出 null。这是 CTR-010 同类的可省略/null 语义过宽问题，本批将 StockBoard.change_percent 纳入 CTR-010 影响范围。
4. StockBoard 的 code 描述为 Eastmoney board code（例如 BK0477）；响应同时提供 source/source_version。当前可见生产实现是 Eastmoney，未据模拟回退测试推断存在 HiThink 生产实现。
5. OpenAPI 契约测试已把 StockBoard、StockBoardSnapshot 纳入 Go Model 字段与 required 反射对照；路由/参数矩阵也登记了 boards 路径及 symbol path 参数。本批未发现字段集合或 required 集合错配。
6. 本批未找到 StockBoard/StockBoardSnapshot 的完整 JSON Golden 专项断言；现有反射测试覆盖字段与 required，但不等同于对真实序列化响应的完整黄金样例覆盖。

#### 24.4 WebUI 对覆盖下界的消费缺口

1. Eastmoney Provider 请求 `pz=1000`；当返回总数达到 1000 时，明确将 `total_is_lower_bound` 置为 true，并将质量标记为 degraded。对应 Provider 测试验证 1000 条、下界标志、degraded 状态和质量原因。
2. WebUI `renderBoards` 只用 `data.total` 显示“源端返回 N 条”，没有读取 `data.total_is_lower_bound`，也没有展示 `provider_returned` 或 `excluded_rows`。当源端命中 1000 条边界时，页面仍显示确定数量文案，未把“至少 N 条/列表可能截断”表达出来。
3. 页面会显示整体 quality_status 与 quality_reasons，但这不能替代对数量字段语义的直接表达；用户需要从质量原因自行推断 total 是下界，且现有文案“源端返回 1000 条”没有显式说明总量未知。
4. 因此该项是 WebUI 对已有 DTO 语义消费不完整，不是 Provider 没有提供下界信息，也不是已证实的错误数据值。

#### 24.5 差异登记

**AUD-WEBUI-CTR-012：个股板块页面未消费 total_is_lower_bound（P2）**

- 证据：Provider 在 1000 条源端边界设置 `total_is_lower_bound=true`；OpenAPI 将该字段列为 StockBoardSnapshot 必需字段；WebUI `renderBoards` 仅展示 `data.total`，未读取下界标志。
- 影响：源端达到 1000 条上限时，页面可能将“至少 1000 条”呈现为确定的“源端返回 1000 条”，掩盖成员列表覆盖不完整。
- 责任方：WebUI 个股研究页面维护方。
- 关闭条件：展示层根据 `total_is_lower_bound` 明确显示“至少 N 条”或“列表可能被源端上限截断”；增加下界为 true/false 的页面渲染测试，确保不会把下界显示为确定总数。

#### 24.6 本批结论

StockBoards 的 Provider Port、Go DTO、Service 校验/回退/缓存、HTTP 路由、OpenAPI 和 WebUI 实际调用链均已核验。字段集合和 required 集合未发现错配；新增 AUD-WEBUI-CTR-012（P2，WebUI 未消费下界语义），并将 StockBoard.change_percent 纳入既有 AUD-WEBUI-CTR-010。未证明运行时板块成员值错误。WebUI 维度 2 继续保持 partial。


### 25. WebUI 维度 2：Fundamentals 财务指标与 PIT 消费

#### 25.1 核验范围与证据

- 固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- Go DTO：`internal/marketdata/model/model.go`（blob `6f04516bd11452a01adcb5acceab571fb1c28ed5`）。
- DataService 校验、Provider 回退、缓存和 PIT：`internal/marketdata/service/fundamentals.go`（blob `0112474efd5b65c5cf9812991c02aa0c9ae5d9df`）。
- PIT 测试：`internal/marketdata/service/fundamentals_pit_test.go`（blob `47af2882a746b1327b35fae7b87b09f120cde30a`）；回退测试 `internal/marketdata/service/fundamentals_fallback_test.go`（blob `850a782871fbc9ee308ca37d5a37e18a9c3197b3`）。
- BaoStock Provider：`internal/marketdata/provider/baostock_fundamentals.go`（blob `4ec129e8157157234dc8d199ec6bab344437ffba`）；测试 `internal/marketdata/provider/fundamentals_test.go`（blob `8c706f4cd1cbca7711931c12f0664221547cc2b2`）。
- HiThink Provider：`internal/marketdata/provider/hithink.go`（blob `337c072c14f764f1f9cfff833f44ddcf5426e04b`）；测试 `internal/marketdata/provider/hithink_fundamentals_test.go`（blob `8d4e33139b9d8bcedf30497bdcfc6c77fd41ce6e`）。
- PIT 版本存储：`internal/marketdata/repository/mysql.go`（blob `3981fde146a6e56502348acb47d41ce87b082223`）；版本哈希测试 `internal/marketdata/repository/mysql_versions_test.go`（blob `464d278d131761119bf2ee9af3a6bfc405a5d8f5`）。
- HTTP Handler：`internal/marketdata/httpapi/fundamentals.go`（blob `d6062a41b50b75b8921e1e977e90fda00d2ded2c`）；测试 `internal/marketdata/httpapi/fundamentals_test.go`（blob `8719e68f1697ed7ed05a93cf300723ceae7c1afb`）。
- OpenAPI：`internal/marketdata/httpapi/openapi.yaml`（blob `cd8a6e3dfb8e3162ee789b11e883be9283c59f32`）；反射契约测试 `internal/marketdata/httpapi/openapi_contract_test.go`（blob `7f97f075a23bd698a7e3fc32e90509d1e999217b`）。
- WebUI API Client：`web/src/api.js`（blob `b8bcbee5908b43bfa1bc2ca744363190af38287f`）；页面消费 `web/src/stock-research.js`（blob `49fe406d7dcf5e8fdec76f2f0bfcda6a18d4db85`）。

#### 25.2 时间与数据语义

1. `FundamentalsSnapshot.year/quarter` 标识请求的报告期；每个 `FundamentalSection.report_date` 是对应财务报告期末；`publication_date` 是披露日期；`available_at` 是精确到时分秒的可用时间证据；`fetched_at` 是本服务收到 Provider 响应的时间；PIT 查询返回的 `system_observed_at` 是本系统记录该版本的时间。这些字段不是同一时间概念。
2. DataService 校验 section 的 report_date 必须等于所请求季度的期末，不能用 Provider 返回的其他期间填充目标季度；publication_date 必须是有效日期、不得早于报告期末或晚于中国市场当前日期。
3. available_at 若存在，必须是非零、非未来时间，且其中国时区日期必须不早于报告期末，并与 publication_date 同日（publication_date 存在时）。当前核验的 BaoStock 与 HiThink Provider 实现没有填充 available_at；现阶段主要依赖日期级 publication_date 证据。
4. BaoStock 使用 `statDate` 作为 report_date、`pubDate` 作为 publication_date。缺少/非法 pubDate 时保留有效指标但将 section 降级，并产生字段级诊断；指标缺失/非法时跳过该指标，不生成 0 值。
5. HiThink 指标端点不提供可确认披露日期和单位，因此指标分区标记 degraded，并使用 `provider_defined` 单位；缺少标准能力分区时补充显式 unknown 空分区。其财务报表端点按 `period_end_ms` 精确选择目标报告期，使用 `report_date_ms` 作为披露日期证据；缺失/非法指标被跳过并降级。
6. FundamentalMetric.value 是非指针数值且 required；这不意味着缺失值变为零。Provider 仅把有效数值追加到 metrics，缺失项不进入数组；真实数值 0 与缺失指标因此保持可区分。
7. DataService 要求六个标准指标分区存在，允许最多额外三张财务报表分区；检查重复分区/指标键、指标元数据、数值有限性、报告期与披露日期一致性。完整有效快照为 12 小时缓存，degraded 为 1 小时，unknown/无数据为 5 分钟；多 Provider 场景不会让未标记回退耗尽的旧 unknown 缓存阻断后续来源尝试。

#### 25.3 PIT 存储与查询

1. HTTP Handler 要求 year、quarter；可选 as_of 必须恰好一个有效、非未来 RFC3339 时间戳。提供 as_of 时只调用 FundamentalsAsOf，不执行实时 Provider 回退；PIT 存储未启用返回 503，未找到可用历史版本返回 404。
2. Service 将每个不同内容版本交给 Store 保存；MariaDB `market_data_fundamental_versions` 按 symbol、报告年/季度、source 保存 revision_hash、recorded_at、fetched_at、quality_status 和完整 metadata。revision hash 排除 fetched_at 与 system_observed_at，因此仅采集时间变化不会伪造内容修订；不同指标值会产生新版本。
3. PIT 查询按 as_of 时刻取每个来源当时最新的版本，再对 section 做可用性过滤；若有精确 available_at，允许同日时刻级闭区间判断；否则必须 publication_date 严格早于 as_of 所在中国自然日。同日但无精确时刻、缺少披露日期的 section 会被排除并标 unknown，整体质量降级。
4. PIT 测试覆盖未启用存储、披露日期早于/等于/缺失时的排除行为，以及精确 available_at 在 as_of 前后时刻的边界行为。MariaDB 版本哈希测试覆盖仅 fetched_at 变化不产生新版本、指标值变化产生新版本。
5. 该实现没有把报告期末、Provider 抓取时间或系统观察时间当作披露时间；也没有在 PIT 缺少本地版本时静默回退实时数据。当前 Provider 未提供 available_at，因此同日精确时间可用性仍受数据源披露精度限制。

#### 25.4 WebUI 消费链

1. WebUI API Client 的 `getStockFundamentals` 请求 `/api/v1/stocks/{symbol}/fundamentals`，当前只传 year、quarter；未暴露可选 as_of 参数。
2. 个股研究页面提供报告年/季度选择器和“读取财务指标”动作；报告期变更会取消旧请求、清空旧结果并要求重新加载。加载函数调用 getStockFundamentals，真实响应进入 renderFundamentals。
3. 渲染层展示每个分区的质量状态、报告期、披露日期、available_at、指标名称/数值/单位及分区质量原因；顶部展示报告期、system_observed_at 和快照质量原因。
4. 当 snapshot.has_data=false 或无 sections 时，页面明确说明“未确认有可用指标，不代表指标值为 0”；分区无指标时也展示未返回状态。没有发现前端把缺失指标补零的逻辑。
5. 页面没有 as_of 输入/查询动作，API Client 也未暴露该参数，因此当前 WebUI 只消费最新 Provider 结果，不能从页面发起系统历史时点 PIT 查询。服务端 PIT 能力存在，但前端尚未接入。

#### 25.5 DTO / OpenAPI / 测试

1. FundamentalMetric、FundamentalSection、FundamentalsSnapshot 均纳入现有 Go Model 字段与 required 反射对照；Go 非 omitempty 字段与 OpenAPI required 集合一致。report_date、publication_date、available_at 是可省略属性，OpenAPI 未错误声明 nullable。
2. FundamentalsSnapshot.system_observed_at 为 Go `*time.Time + omitempty`，nil 时 JSON 省略；OpenAPI 却直接声明 `type: string, format: date-time, nullable: true`。这属于 AUD-WEBUI-CTR-010 同类的省略/null 语义不一致。
3. OpenAPI 对 report_date、publication_date、available_at 分别描述报告期末、披露日期、精确市场可用时间；接口 description 也说明 as_of 读取本地版本、不请求实时 Provider、同日无时刻证据时保守排除。参数、错误状态与 Handler 行为基本对齐。
4. HTTP 专用测试覆盖必需报告期参数、非法季度、未来报告期、PIT 未启用、as_of 空值，以及正常响应中的 publication_date/source_field；未发现覆盖完整 FundamentalsSnapshot JSON 的专用 Golden。反射测试覆盖字段与 required，但当前没有反向约束 omitempty 字段不得 nullable 的通用检查。
5. Provider 测试覆盖 BaoStock 报告期/披露日期、比例与人民币单位、未来日期、缺失分类、字段级缺失/非法诊断；HiThink 测试覆盖缺失分区、重复分区/指标、重复报告期行和字段级缺失/非法诊断。

#### 25.6 差异登记

**AUD-WEBUI-CTR-013：Fundamentals PIT 查询能力未接入 WebUI（P2，待产品范围确认）**

- 证据：HTTP/OpenAPI 支持可选 as_of；WebUI `getStockFundamentals` 只构造 year、quarter 参数，个股研究页没有 as_of 控件或 PIT 查询动作。
- 影响：页面无法复现历史系统观察时点的财务数据，当前页面展示的是最新可用 Provider 结果；研究/回测场景不能通过该页面避免使用事后更新的财务版本。
- 责任方：WebUI 个股研究页面维护方；产品范围需确认是否要求页面提供 PIT 查询。
- 关闭条件：若纳入页面范围，API Client 增加可选 as_of，页面提供明确时区的历史时点输入并展示系统观察时间/排除原因，补充日期边界与请求参数测试；若不纳入，需在 WebUI 功能范围文档明确 PIT 仅由服务端/其他调用方使用。

#### 25.7 差异更新与本批结论

- AUD-WEBUI-CTR-010 影响范围增加 `FundamentalsSnapshot.system_observed_at`：Go omitempty 指针 nil 时省略，OpenAPI 声明 nullable=true。
- 新增 AUD-WEBUI-CTR-013（P2，待产品范围确认）：服务端 PIT 能力未接入当前 WebUI 个股财务查询链。
- 本批未发现报告期/披露日期概念混淆、缺失值补零或 Provider 将请求报告期伪造成来源报告期的证据；PIT 日期级过滤是保守的。
- Fundamentals DTO 字段与 required 集合未发现错配；未发现已证实的运行时财务数值错误。WebUI 维度 2 继续 partial。
