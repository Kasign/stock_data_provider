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
