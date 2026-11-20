# Server 模块卡点

当前执行任务：跨模块生产装配卡点治理（Quant、LLM、Notification、Research Context/Stage）及 M6 目标环境验收。
状态：RA009/RA011 代码与 CI 已完成；生产链路按各模块真实消费者、策略和外部依赖证据分别验收。
- SRV-001 已完成：API/Worker/All 角色真实启动、Readiness、SIGTERM 和 Lifecycle 回滚/关闭顺序均有精确 SHA 验收证据。
- SRV-003 已完成：Run 44314707349 对真实 API Root + Data InProcessHandler 的认证、Validation Fault、Request ID 和请求体限制验收通过。
- SRV-004 已完成当前 Root 已注册检查集的 Health/Readiness/Diagnostics 验收；Data 最终 checksum/Schema 仍归 SRV-009。
- SRV-015 已转 BLOCKED：当前可安全装配的 News 服务已完成；其他业务模块等待真实消费者、生产配置、持久化、Gateway 或渠道依赖。

更新时间：2026-11-19（追加跨模块最新基线复核）
维护分支：feat/server
代码验收基线：98bed9ac29260e5b4c48f4c80ee59c4da9df8682
当前主线：SRV-009 Data/MarketData M3/M4 Root 生产装配

## 1. Root Readiness 503 根因已定位并修复（当前装配范围验收通过）

状态：RESOLVED（最终 Migration 收口仍见第 2 节）

根因证据：
- 失败 CI Run：44298185039，精确 SHA `f88af64ef5d91c357dec4f48214d52e2b81d8077`。
- 受保护 `/diagnostics` 报告中的唯一失败 Required Check：`server.storage.schema`。
- Data Module 自己执行 Migration 创建了 `market_data` namespace；Root Registry 只登记静态 Migration namespace，遂将 Data 的合法 namespace 判为未注册。
- 这不是 MariaDB/Redis Ping 失败、Data Module Ready 失败或 HTTP Listener 未启动。

修复与验收：
- Registry 新增 `RegisterExternalNamespace`；Bootstrap 显式登记 `market_data`。
- 外部 namespace 只读校验要求 history 至少存在一条记录，所有版本非零且状态均为 `applied`；外部 namespace 不进入 Root Migration 执行清单，不复制 Data SQL。
- 新增注册、重复注册、缺失 history、failed status、外部 namespace 不参与执行等测试。
- Server CI Run [44305939060](https://github.com/Kasign/stock_data_provider/actions/runs/44305939060)，精确 SHA `c3602ca9369e116a3dece751d420e660e2614450`：gofmt、Server package tests、Race、真实 MariaDB/Redis、Vet、镜像构建、Notification Schema、真实容器 Readiness 和 SIGTERM 清洁退出全部 success。
- 容器日志确认 Data Migration 与其他静态 namespace Migration succeeded；smoke 输出 `Server container reached readiness and exited cleanly after SIGTERM`。

限制：
- 外部 namespace 只校验 history 存在和 applied 状态，不验证预期版本集合和 checksum；这只是过渡性校验。
- Data Store.Migrate 在 Platform ApplyMigrations 锁释放后仍执行兼容索引/补列 DDL，多实例竞态仍存在；最终 Migration 收口仍 BLOCKED。


## 2. Data Migration 与 Root 统一装配（代码级已完成）

状态：DONE_CODE_LEVEL；生产环境验收仍待完成。

- Data 以静态 Migration Definitions 注册进 Root Migration Registry；Root 统一执行 Migration enabled/disabled 策略。
- Platform 提供 namespace lock、checksum 和只读 ValidateAppliedMigrations；Data 物理 Schema 校验纳入 Data Readiness。
- Root 通过共享 MariaDB/Redis 资源构造 Data Module；Data 不创建/关闭共享连接池。
- Data InProcessHandler 已挂载到 Root Router；Analysis Collector 通过 Data Application Port 使用行情能力。
- Data Readiness 纳入 Root Diagnostics；Data Module 生命周期纳入 Root Lifecycle。
- 本节历史快照中关于 MigrationRunner 缺失、Root 未挂载 Data、Data 必须独立部署等描述均已被上述实现取代。
- 剩余验收：真实生产 DDL 对照、Migration enabled/disabled 部署演练、真实备份恢复和生产启动/关闭证据。

## 3. 尚未闭环的 Server 自身验收

- Data Handler 的 DATA-CTR-006 Root HTTP 入口行为已通过 Run 44314707349：Owner Bearer、无需 Data API Key、Request ID、Fault 投影、请求体限制和真实路由分发；未覆盖真实 Provider 上游连通性。
- API/Worker/All 角色 Root Bootstrap、Readiness、SIGTERM、Lifecycle 回滚和多实例独立启停已通过 Run 44312706873；该项不再作为未完成卡点。
- 启用 Provider 配置矩阵及 HTTP/TCP 受控出口注入；真实 Provider 连通性没有真实上游证据时标记 NOT_COMPARABLE。
- Data Module Start/Stop/Close 与 Root 共享资源的关闭顺序由现有 Lifecycle/Bootstrap 测试覆盖；后续只在出现新的可复现缺陷时补充，不重复创建生命周期实现。

## 4. 当前不应做的事

- 不重做已经接入并通过当前精确 SHA CI 的 Data Root 装配；后续只处理明确的剩余验收项。
- 不把过渡性外部 namespace 状态检查描述成最终 checksum/Schema 校验。
- 不在 Server 分支直接修改 Data/Platform 私有实现或复制公共能力。
- 不提前删除旧 Analysis API/Worker；SRV-013 仍按依赖保持 deferred。


## 5. 其他模块生产服务尚未全部装配

状态：PARTIAL（可安全装配项已推进，其余按真实依赖阻塞）

已接入：
- News Storage Connection/Schema Diagnostics。
- Worker/All 角色的 News Observation/Availability Evidence Retention Cleaner 和 Evidence Snapshot Cleaner。
- Root News Observation Retention Cleaner 的真实 MariaDB 批次/保留边界集成测试已通过，CI Run [44309516411](https://github.com/Kasign/stock_data_provider/actions/runs/44309516411)，精确 SHA `f5821a356e23691d9738f4b3808abf8e0e3494d8`；真实 RSS/Atom 来源采集仍因生产 Source/Endpoint 配置缺失保持阻塞。
- Researchflow News Search Stage/Runtime、Research/Researchflow Outbox 到 Notification Inbox 的接收 Relay 已存在，不重复开发。

尚未装配及原因：
- News RSS/Atom Source Collection：缺生产 Source/Endpoint Catalog 配置与受治理的 Endpoint BoundDoer，不可凭空生成来源或开放任意 URL。
- Research：News→Research 组合只在 Cursor Key Ring 配置时构造，当前无真实 Stage/HTTP 消费者；不能创建无调用场景的 Handler。
- Quant：只有确定性计算能力和 Memory Result Repository，缺生产持久化 Repository、Root Facade/Worker 及 Researchflow Stage 输入契约。
- LLM：传输适配器存在，但无已验证 Gateway 版本/Capability Evidence、SecretRef 和生产 Provider 配置。
- Notification Delivery：`feat/notification` 已提供 WeCom Webhook Adapter，Live Smoke Run 44295239517 成功且用户确认真实收信；但当前 `feat/server` 代码树尚无该 Adapter。仍缺共同集成基线、生产 Secret 注入、权威 Recipient Directory、Fanout/Delivery Worker 与可靠回执/Unknown Lookup 闭环。Server 不复制其他模块 Adapter。
- Researchflow：当前只装配 News Search Stage；其他 Stage 等各模块公开 Facade、Schema、授权和真实输入证据。

原则：已具备生产条件的服务直接装配；缺真实数据源、调用者、持久化或外部依赖的服务保持 BLOCKED，不创建空 Worker、恒 true Check、Fake Provider 或虚构接收人。

## 6. WebUI / Data 单 Root API 接入（代码已实现）

状态：CODE_IMPLEMENTED_PENDING_CI_AND_PRODUCTION。

- WebUI 的 Data 和 Analysis 请求统一使用同源 /api；运行时不再读取或发送 Data API Key / X-API-Key。
- WebUI 使用 HttpOnly Owner Session Cookie；CSRF Token 只驻留 JS 内存，Provider Probe POST 发送 X-CSRF-Token。
- Root Data 路由显式允许 Owner Session 或 Owner Bearer；同时提交两种凭据时拒绝；Session 请求由现有 SessionAuthenticator 校验 Origin/Host/CSRF。
- Compose 只部署 stock-analysis-server 与 stock-analysis-web；Nginx 只保留 /api/ 到 Root Server 的业务 upstream。
- 已删除 cmd/market-data、services/market-data 独立 Docker/配置/迁移资产、旧 go-migration Compose Profile 和旧 Data API Key 认证实现。
- 不保留独立 Data 服务、双 upstream、Remote Adapter 或旧 Key 回滚路径。
- 待完成：最新精确 SHA Integration CI、真实浏览器登录/刷新恢复/Probe/登出、目标 HTTPS Origin/Argon2 参数、真实 MariaDB/Redis 和 Provider 数据验收。

## 7. 跨模块前置最新基线复核（2026-11-19）

本轮仅只读检查其他分支，没有修改其他分支。Server 当前分支 HEAD 为 `33521adc5cff432f0efbcb6a6f090bb01e02ec10`。

### 7.1 SRV-009 直接依赖

| 责任模块 | 分支最新 HEAD | 本轮核对结果 | 对 SRV-009 的影响 |
|---|---|---|---|
| Data | `9f6adaf280a94bc60f993554473fd0adb70b07da` | 最新提交仍为文档汇总；`MigrationRunner` 仍只公开 `Migrate(ctx)`；`Store.Migrate` 仍在 Platform ApplyMigrations 锁范围外执行兼容索引/补列 DDL | 静态 Migration 定义和并发安全前置仍未满足 |
| Platform | `308b0bfdf3eba6a83ac16f1eae1aec66518ebbcd` | 最新提交仍为文档整理；`storage.ApplyMigrations` 有统一 checksum preparation 和 namespace advisory lock，但没有可供 Root 使用的只读 checksum 校验入口 | Platform 只读校验前置仍未满足 |

### 7.2 SRV-015 其他模块装配观察基线

以下分支仅作为其他模块装配阻塞的观察基线，不改变 SRV-009 当前主线，也不代表 Server 已完成这些模块的生产验收。

| 模块 | 分支最新 HEAD | Server 侧现有卡点 |
|---|---|---|
| News | `cb4206e8e59ec7bcb27660d33e4cc28b1cd2a06d` | RSS/Atom 生产 Source/Endpoint Catalog、授权策略和受控采集入口未满足 |
| Research | `3f3be141c616e9b3beee33a78363132c49effece` | 尚无明确生产 Stage/HTTP 消费者；真实授权与来源证据仍需按调用场景验收 |
| Researchflow | `e36d33ac334b5dbbebe39719ed26e09756da59dd` | 当前 Root 只装配 News Search Stage；其他 Stage 等提供方公开 Facade、Schema、授权和真实输入 |
| Quant | `404f449fb5d0ad544a4ac106687200fef1e785f9` | 缺生产持久化 Repository、可注入 Application Facade 和 Researchflow 输入契约 |
| LLM | `77ef70568a0e76ab3092b25503c36ee2316990a3` | 缺真实 Gateway 版本/Capability Evidence、SecretRef、生产目标策略和端到端调用证据 |
| Notification | `b2116b9b180d2d2e14d47d93c6c16631e6ccb9ab` | WeCom Adapter 尚未进入 Server 共同集成基线；生产 Secret、Recipient Directory、Fanout/Delivery Worker 与 Unknown 对账仍未闭环 |

### 7.3 本轮执行结论

- Data/Platform 的最新基线未解除 SRV-009 前置；Server 不应自行复制 Migration 定义、SQL、checksum 算法或锁。
- Server 已完成的 Data Module 构造、共享资源、受控 Egress、Readiness、Lifecycle 和 Root HTTP Handler 不重复开发。
- SRV-015 继续保持阻塞观察，不在 SRV-009 未解除时创建占位 Worker/Handler 或切换当前任务。
- 下一步触发条件：Data 和 Platform 对应能力进入共同集成基线后，再在 `feat/server` 注册 Data 静态 Migration、接入只读 checksum 校验并执行真实 MariaDB 并发/漂移验收。


## RepairAll 聚合分支收口进展（2026-11-19）

`repair_all` 已完成 SRV-009 中 Data Migration 的代码装配部分：

- Data 静态定义已加入 Root Registry，Root Migration enabled/disabled 路径统一；不再走外部 namespace 过渡机制。
- Platform 提供只读 checksum 校验，Root disabled 路径能验证 Data 预期版本和 checksum。
- 旧库 V1-V3 升级、兼容 DDL 和多实例并发 MariaDB 测试已新增；CI 等待 [44447674606](https://github.com/Kasign/stock_data_provider/actions/runs/44447674606)。
- SRV-009 的生产环境启停演练、物理 Schema drift Diagnostics 验收仍未关闭；Server 模块分支未被本轮修改。


## RepairAll Root Data 启动就绪校验补充（2026-11-19）

- Root Migration enabled 时执行统一 Registry 后，现也执行 `marketDataModule.Diagnostics().Ready`；disabled 时先 `ValidateApplied`，随后执行同一 Ready 检查。两条路径在启动 Worker/HTTP 前都会校验 Data 的 MariaDB、Redis 和 V4 物理 Schema。
- 此前 Ready 仅位于 `MIGRATION_ENABLED=false` 分支，导致 enabled 路径和 Worker 启动路径的检查边界不一致；现已移出分支统一执行。
- 物理 Schema 漂移由 Data `Store.ValidateCompatibilitySchema` 检查，不混同 Platform Migration checksum。MariaDB 集成测试验证缺失列会阻止 Ready。
- 最新代码 SHA `b866188cf2b784a2e9c2d9a11ff002962a3453bd` 待 CI；模块分支 `feat/server` 未同步。


## RepairAll SRV-009 Root Data 物理 Schema 就绪验收（2026-11-19）

- 当前聚合基线：`5d97dc89acfd0493b8b7f4c316db6a5d5dc609e9`；Integration CI [Run 44452547201](https://github.com/Kasign/stock_data_provider/actions/runs/44452547201) 精确 SHA 全项 success。
- Root Migration Registry 已同时注册 Server 测试 namespace 与 Data 静态 Migration definitions；集成测试在 Migration 执行后验证 Data Ready，再模拟缺列/缺索引，确认 Readiness fail-closed。
- Root 在 Migration enabled/disabled 路径之后统一执行 Data Ready；启动 Worker 或 HTTP 流量前检查 MariaDB、Redis 和 V4 物理 Schema。
- 代码级 SRV-009 Migration/Schema 门禁验收通过；生产环境 Migration enabled/disabled 演练、真实 Provider 出站矩阵仍未完成。其他模块生产装配卡点仍按本文件第 5/7 节的准入条件处理。
- 本次仅更新 repair_all 聚合基线，`feat/server` 分支未同步。


## RepairAll SRV-009 Checksum / 物理 Schema 分离验收（2026-11-19）

- 最新 repair_all SHA：`ff9a98457220ae307b9dc4a62ce274849cd60f12`；Integration CI [Run 44453457919](https://github.com/Kasign/stock_data_provider/actions/runs/44453457919) 精确 SHA 全项 success。
- MariaDB 集成测试确认：物理列/索引被删除时，Root Migration Registry checksum 预检不误报 Migration 定义被修改；Data Ready 以 `market_data.schema_drift` fail-closed。
- 该行为在 Root Registry + Data Repository + MariaDB 的真实组合中验收，不是 Mock。
- 生产数据库启停演练仍待执行；`feat/server` 分支未同步。


## RepairAll Root Data Ready 最终 MariaDB 验收（2026-11-19）

- 最新聚合代码 SHA：`d386e097db64e6913290441ccc7bce0602358348`；Integration CI [Run 44454928267](https://github.com/Kasign/stock_data_provider/actions/runs/44454928267) 全项 success。
- Root Worker/API Bootstrap 在真实 MariaDB/Redis 集成环境重新通过。Schema 校验不再误用 MariaDB `COLUMN_DEFAULT` NULL 元数据；列和索引检查保留精确物理结构约束。
- 启动失败测试现输出内部 Fault Cause，帮助定位 Schema 检查原因；该内部原因不暴露为 HTTP 公开字段。
- 生产环境 Migration enabled/disabled 演练和其他模块外部依赖仍未完成；`feat/server` 未同步。


## RepairAll SRV-009 Migration disabled 启动门禁（2026-11-19）

- 最新代码 SHA：`02415a319fbe45921be2be469592d84043676626`；Integration CI [Run 44455503870](https://github.com/Kasign/stock_data_provider/actions/runs/44455503870) 全项 success。
- 真实 API Bootstrap 集成测试覆盖 Migration disabled：Migration history/checksum 有效但 Data 物理列缺失时，Root 必须在 HTTP 启动前以 `market_data.schema_drift` 失败；数据库缺列保持原状，不自动执行 Migration DDL。
- 代码层 Migration enabled/disabled 两种路径均通过；生产部署演练仍待执行，`feat/server` 未同步。


## RepairAll News→Research 装配边界复核（2026-11-19）

- internal/server/bootstrap/research_news.go 已创建 News Evidence Query Facade、Research Service、Importer、Authorizer，并共用 Server 持有的 MariaDB DB；不是缺少装配代码。
- internal/server/bootstrap/bootstrap.go 仅在 Evidence Cursor Key Ring 已配置时构造该组合；当前无 Researchflow Stage、HTTP 或 Worker 业务用例调用该组合，启动日志明确为等待业务消费端接入。
- 判定：装配构造 PASS（代码存在）；生产消费链路 NOT_COMPARABLE（无真实消费者/调用记录）。不得为了接上线创建无场景 Handler 或绕过 Researchflow Stage Provider。
- 解锁条件：Researchflow/Server 明确实际消费场景、输入 DTO、可信 Principal/CallerRef 注入和执行上下文，再由组合根接线并做集成验收。
- 本结论来自 repair_all 审查基线 5334043ea8e1abc2c4006c371a3f95b123098946；不代表 feat/server 分支已同步。


## RepairAll Quant 生产消费链路复核（2026-11-19）

- Quant GetResult 只定义授权读取用例及 PrincipalResolver、AuthorizedResultReader、AccessAuditor 端口；当前未发现 Server 对应生产实现和 Root 注入。
- Quant 结果 Repository 当前只有 Save Port 和 MemoryRepository；不能将 MemoryRepository 作为生产持久化验收。
- Server bootstrap 当前没有 Quant 相关调用；Researchflow Worker 仅注入 News Search Stage Executor/Output Registry。当前没有 Quant Stage 生产消费者。
- 该缺口由 Quant QNT-013/QNT-014 的公开能力、生产 Adapter 和明确 Stage 场景共同决定；Server 不应提前创建空壳 Worker/Handler，也不复制 Quant 存储或授权规则。
- 本结论只针对 repair_all 审查基线 b52c8afde2f2e32fca498a78987d34f96fba4335，不代表 feat/server 分支同步。


## RepairAll RA-CROSS-REVIEW-005 Context Authorizer 装配复核（2026-11-19）

- 审查基线：repair_all SHA 8afbba2a0a519f932063861085bbdc89f7e76fa8。
- internal/server/bootstrap/research_news.go 创建 Research Application Service 时调用 NewService(contexts, evidence, authorizer, nil, nil)，未传可选 ContextAuthorizer。
- Research Service 的 GetContext 对 ContextAuthorizer 缺失采取 fail-closed；当前没有真实 Context 消费者，因此该组合尚不能用于 Context 读取，但 News Evidence Importer 仍可使用其 Evidence 能力。
- 不注入默认 Allow，不新增平行授权服务；先明确 Research Context 资源授权 Owner/策略和可信 CallerRef，再由 Server 装配真实 Authorizer。
- 该状态属于待接入消费者和授权策略的组合根卡点，不把无调用场景的 Handler/Stage 当成修复。


## RepairAll RA-CROSS-REVIEW-005 最终结论（2026-11-19）

- Server Root 已注册 Research Migration，Research Context/Evidence/Event/Claim 持久化归属明确。
- Server News→Research composition 创建 Research Service 时未注入 ContextAuthorizer；当前无 Context 生产消费者，因此暂不补默认实现。未来接入必须提供真实 Research Owner 授权策略和可信 CallerRef。
- 当前没有 Context Stage 调用 Researchflow Context Assembly，也没有 ResultComposition 的生产消费者；Run/SystemAsOf 绑定与 Evidence available_at 严格 PIT 尚无端到端验收。
- Evidence PIT Mode 需要项目契约 Owner 决策，Server 不自行定义或注入策略字段。


## RepairAll RA-CROSS-REVIEW-006 Notification 生产装配复核（2026-11-19）

- Server Bootstrap 已在 Worker/All 角色装配 Research→Notification Relay；Researchflow Capacity 配置后装配 Researchflow→Notification Relay；API 角色不启动这两个 Relay。
- 两条 Relay 共用 Notification EventReceiver；Inbox Schema Registry、MySQL Inbox 与 Fanout WorkItem Repository 已装配，Inbox+Fanout WorkItem 使用同一 DB 事务。
- Server 当前没有 Notification RecipientDirectory、Fanout Worker、Delivery Worker、Channel Registry/Secret、Template Renderer 的生产装配；模块服务和 Adapter 存在不等于 Server Delivery 闭环。
- NTF-BLOCK-04/05/06/08/09 继续 BLOCKED。不得在没有 RecipientDirectory/业务模板/Worker 配置的情况下创建空壳投递服务。


## RepairAll RA-CROSS-REVIEW-007 最终结论（2026-11-19）

- Server Router 对所有路由显式声明 Platform RoutePolicy，拒绝 root catch-all；未登记路径保持 404。
- 当前路由矩阵与 SRV-CTR-002 一致：healthz 匿名；readyz/diagnostics Owner Bearer；Owner Session 六个浏览器端点单独执行 Session Cookie、Origin/Host 和 CSRF；旧 Data/Analysis 业务路由不接受 Session，不把 Session 映射为 Owner Bearer。
- Request ID、Fault 安全错误投影、请求体/并发限制、Readiness/Diagnostics 和消费者逆序停止后关闭共享资源均已接入。
- 当前没有可复现的 Root HTTP/身份代码缺陷；未发布的 Research/Quant/Notification API 不提前创建。下一项转为 WebUI 实际调用与凭据暴露审查。


## RepairAll RA-CROSS-REVIEW-008 M5 部署/认证阻塞（2026-11-19，历史快照；已由 RA009 解除代码层阻塞）

- Root Server 的 Data InProcessHandler 已挂载，但路由策略仍为 Owner Bearer；WebUI 当前只持有 Owner Session Cookie，不持有 Owner Bearer。
- 迁移期 WebUI 仍调用独立 Data 服务并持久化 Data API Key。Compose/Nginx 尚未切换为单 Root Server。
- 单纯把 /market-api upstream 指向 Root Server 会因 X-API-Key 与 Owner Bearer 认证不匹配而失败；静态注入 Owner Bearer 会绕过 Owner Session/资源授权边界，禁止采用。
- M5 解锁前置：明确 Data 路由是否允许 Owner Session（必须覆盖探测等写请求的 CSRF 与领域权限），或采用受控 Service Principal；同时决定统一 API Prefix/Cookie Path。由 Server/Platform/Data/WebUI 共同更新契约和验收。
- 在认证策略冻结前，不删除独立 Data 服务、不删除浏览器 Key 兼容路径、不修改部署契约测试。


## RepairAll RA009：Data 入口统一 Owner Session（用户已确认，2026-11-19）

当前状态：旧独立 Data 运行资产已删除，Root 单入口与 Owner Session 路由已实现；等待最新精确 SHA CI 和目标环境验收。

状态：CODE_IMPLEMENTED_PENDING_CI_AND_PRODUCTION

最终决策：WebUI 通过 HttpOnly Owner Session Cookie 访问 Root Server Data 路由；Data 路由显式允许 Owner Session，同时保留 Owner Bearer 运维调用。非安全方法必须由现有 SessionAuthenticator 执行 Origin/Host 与 CSRF 原子校验；Data 领域资源授权不得跳过。

部署目标只有 Root Server 一个 API upstream，统一路径和 Cookie Path 为 `/api`。不保留独立 Data 服务、旧 `/market-api` / `/analysis-api` 双 upstream、浏览器 Data API Key、`X-API-Key` 兼容或回滚部署资产。切换与清理在同一迁移批次完成，避免先删除再导致生产不可用。

实现清单见《Market Data 模块化单体迁移与最终验收状态》。代码已实现并删除旧独立服务资产；未完成最新精确 SHA CI 和真实部署前，不得标记 M5/M6 生产验收完成。

本节覆盖本文件第 6 节及历史快照中“独立 Data Service 必须保留”“不删除旧代理/部署资产”等旧结论；这些内容仅作为历史审查记录，不再是当前目标架构。


## RA009 补充：旧 Analysis HTTP 认证边界（独立于 Data 迁移）

- Data 业务路由已通过显式 Owner Session/Owner Bearer 分发器接入 Root；该身份策略不自动扩展到旧 Analysis Task/Run 路由。
- 当前旧 Analysis 路由仍由 Platform Owner Bearer RoutePolicy 保护，而 WebUI 不持有 Owner Bearer。因此旧 WebUI Task/Run 请求仍无法凭 Owner Session 通过认证。
- 不得把 Owner Session 伪装成 Owner Bearer，也不得为完成 Data 迁移而放宽旧 Analysis 路由。
- 该问题属于 SRV-CTR-001 deferred / Researchflow 新 HTTP API 装配边界，单独作为 Server 与 Researchflow 联调卡点；在新 API 合同、Application、路由和端到端测试完成前，不宣称旧 Task/Run 浏览器链路可用。


## RepairAll RA009 最终代码验收（2026-11-19）

- 用户已确认采用 Owner Session + Root Server 单入口，并明确不保留旧独立 Data 服务、旧 Data API Key、双 upstream 或回滚部署。
- Server 已移除旧 Data 服务入口与 Data API Key 认证；Data 路由显式接受 Owner Session / Owner Bearer，非安全 Session 请求由现有 SessionAuthenticator 校验 Origin/Host/CSRF。
- 路由回归发现 `GET /api/v1/data-sources` 精确路径未覆盖于旧尾斜杠子树路由，已补精确路径注册，并新增 Owner Bearer / 未认证测试。没有更改认证标准。
- 精确代码 SHA `1e916220a8784ae840132ce0cec3b3654c6b9655` 的 Integration CI [Run 44482650366](https://github.com/Kasign/stock_data_provider/actions/runs/44482650366) 全项 success。
- M5 代码和 CI 验收完成；M6 仍需目标环境真实 HTTPS/Session/CSRF、MariaDB/Redis Migration 启停、备份恢复及 Provider 真实数据证据。此类外部证据不能由 CI 替代。
- 旧 Analysis Task/Run API 仍是独立的 SRV-CTR-001 deferred 边界，不属于 Data 路由迁移；不得把 Data 的 Owner Session 策略隐式扩展到旧 Analysis 路由。


## RepairAll RA-CROSS-REVIEW-009：LLM 生产装配边界（2026-11-19）

- `internal/server/llmtransport/transport.go` 已实现 Platform BoundDoer/OpenStream 薄包装与 ResponsesAdapter 构造辅助函数；固定 HTTPS host/port、禁止 redirect、禁止 transport retry 的策略检查已有单测。
- 当前 `internal/server/bootstrap/**` 与 `internal/server/config/**` 未发现 LLM Provider 装配、Gateway SecretRef 解析、CapabilityState 加载或 `llmtransport.NewResponsesAdapter` 调用点；SRV-010 仍 BLOCKED。
- Researchflow 当前 Server Runtime 只注入 News Search Stage Executor/Output Registry，没有实际 LLM Provider Stage 消费者；ToolPolicy Adapter 仅解析模型提出的工具调用意图，不执行模型调用。
- 不在 Server 侧创建空 Provider、占位配置、假 Capability Evidence 或空 Stage。解除条件是 Gateway/Model/SecretRef/网络策略证据和 Researchflow 明确的首个真实调用场景。
- 审查基线 `0b21a3e61172ad751bc17bf17ed0c271ae25b5ec`；当前生产代码沿用已验收 SHA `1e916220a8784ae840132ce0cec3b3654c6b9655`，Integration CI [Run 44482650366](https://github.com/Kasign/stock_data_provider/actions/runs/44482650366) 全项 success。


## RepairAll 阶段二卡点状态校准（2026-11-19）

- RA009 Root Server Data 单入口代码验收已完成：精确 SHA `1e916220a8784ae840132ce0cec3b3654c6b9655` 的 Integration CI Run [44482650366](https://github.com/Kasign/stock_data_provider/actions/runs/44482650366) 全项 success。此前“等待最新精确 SHA CI”的段落是实施过程记录，不再代表当前代码状态。
- RA011 Lifecycle Shutdown 预算修复已由精确 SHA `9a0680463f1ddea8f137e9009a6306f26e331ffd` 的 Integration CI Run [44489065474](https://github.com/Kasign/stock_data_provider/actions/runs/44489065474) 全项 success。
- Server 当前实质卡点转为其他模块生产组合装配（Quant、LLM、Notification、Research Context/Stage）及 M6 真实目标环境验收；不再将 RA009/RA011 代码验收列为开放任务。
- 统一责任与解锁顺序见 `doc/整体任务规划/05-治理与验收/RepairAll跨模块修复计划与进度.md` 第 28 节。


## RepairAll Quant 生产装配复核更新（2026-11-20）

- 复核基线：repair_all HEAD `487edf052b44721d5b8ba3c5429e5d200ec24772`。
- Server Bootstrap `newMigrationRegistry` 当前未注册 Quant namespace；Quant 也尚无模块 Migration 定义/生产结果 Repository。Server 不应自行定义 Quant 表结构或将结果存储业务逻辑写进组合根。
- Bootstrap 未构造 Quant PrincipalResolver、AuthorizedResultReader、AccessAuditor，也没有 Quant 计算 Facade 或 Researchflow Quant Stage 装配。Researchflow Worker 当前注入 News Search Stage Executor/Output Registry。
- Quant 的 GetResult Port 已固定 `quant_analysis/full`，但没有可验证的授权策略 Owner、策略撤销一致性边界、可信 subject_ref 映射和审计保留/失败处置定义。Server 不得创建默认 Allow 或用 Owner Session 代替领域资源授权。
- Server 当前可推进：确认共享 DB/Platform Storage 能力、Root 生命周期和 Migration Registry 的接入方式；按 GOV-CTR-006 现有语义由 Quant Owner 实现策略规则/快照/撤销门禁；Server 在 Quant 提供完整 Adapter/ Migration 后负责 Root 注入与 MariaDB Integration。禁止先写空壳实现。
- 该卡点与 RA009 Data 单入口迁移无关；不得回退已完成的 Data/Session 认证决策。


## RepairAll Quant Storage 设计审查后的 Server 责任边界（2026-11-20）

- Quant 结果持久化应由 Quant 模块提供 Repository SQL Adapter 与 Migration 定义；Server Migration Registry 只注册 Quant namespace，Root 只负责构造并注入依赖。Platform 继续提供公共 SQL pool/Migration Runner。
- Server 不定义 Quant 领域表、不实现 Quant Repository，不将结果存储逻辑放入 Bootstrap。
- AuthorizedResultReader 的策略判定/结果读取一致性和 AccessAuditor 持久化仍需按 GOV-CTR-006 正式语义装配；策略 Owner、撤销一致性和审计保留语义未明确前，不创建默认 Allow Adapter。
- 当前无 Quant Storage Adapter/Migration，因此 Server 不应先注册空 namespace。待 Quant 提供完整 Migration 后再注册并由 Integration 验证精确 namespace/version/checksum。
