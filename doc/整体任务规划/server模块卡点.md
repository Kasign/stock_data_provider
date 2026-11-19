# Server 模块卡点

当前执行任务：SRV-009 Data/MarketData M3/M4 Root 生产装配最终收口。
状态：BLOCKED（等待 Data/Platform Owner 前置；不在 Server 重复实现公共 Migration 能力）。
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


## 2. 跨模块前置：Data Migration 定义与并发安全

状态：BLOCKED（等待 Data / Platform Owner 前置）

已核实问题：

跨分支复核基线（2026-11-19，只读检查，不修改其他分支）：
- Data 分支 `feat/data` HEAD：`9f6adaf280a94bc60f993554473fd0adb70b07da`。其公开 `internal/marketdata/bootstrap.MigrationRunner` 仍只有 `Migrate(ctx)`，没有可供 Root Registry 注册的静态 Migration 定义。
- 同一 Data SHA 的 `internal/marketdata/repository/mysql.go` 中，`Store.Migrate` 调用 Platform `ApplyMigrations` 后仍执行 `ensureNonUniqueHistoryIndex` 和 `ensureColumn`；兼容 DDL 仍在 namespace 锁/版本/checksum 保护范围之外。
- Platform 分支 `feat/platform` HEAD：`308b0bfdf3eba6a83ac16f1eae1aec66518ebbcd`。公开 `internal/platform/storage/storage.go` 有 `ApplyMigrations`、checksum preparation 和 namespace advisory lock，但没有只读 `ValidateAppliedMigrations` API。
- Data 分支旧迁移文档仍称 Root 未构造 Data Module/未挂载 Handler；这与当前 Server 分支实际代码及 Run 44314707349 不一致。该描述是 Data 文档未同步 Root 进展，不应据此重做 Server 已完成的 Bootstrap、HTTP Handler、Readiness 或 Lifecycle。

- Data Store.Migrate 在 Platform ApplyMigrations 的 namespace 锁保护范围之外，仍执行兼容性 `ensureNonUniqueHistoryIndex`、`ensureColumn` DDL；多实例并发启动存在竞态。
- Root 当前直接调用 Data Module 的 MigrationRunner，再运行 Server Registry；Data 尚未以完整静态 Migration 定义注册进统一 Registry。
- `MIGRATION_ENABLED=false` 时当前只读校验不覆盖 Data checksum/物理 Schema 漂移；Data Diagnostics.Ready 的 DB/Redis Ping 不能替代 Schema 校验。

期望前置：
- Data 暴露完整、不可变的 Migration 定义，保留已发布版本 checksum，并将兼容 DDL 纳入后续版本。
- Platform 提供与 Apply 共用 checksum preparation 的只读 ValidateAppliedMigrations 能力。
- Server 仅负责注册、调度和真实启动验收，不复制 SQL、checksum 算法或 Migration Runner。

解除条件：
- Data/Platform 对应改动合入当前集成基线；Server 接入统一 Registry；验证迁移开启/关闭、多实例并发启动、checksum 不一致和 Schema 漂移行为。

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

## 6. WebUI 尚未切换到 Root Data Handler

状态：BLOCKED（依赖 Web/Data/部署任务，不作为 Server Root 装配已完成的证据）

当前代码事实：
- `web/src/api.js` 行情基路径仍为 `/market-api`，行情请求仍发送 `X-API-Key`。
- `web/default.conf.template` 的 `/market-api/` 仍代理到 `MARKET_DATA_SERVICE_NAME:8091`。
- Root Data Handler 挂载在 `/api/v1/...`，由 Owner Bearer 保护；不接受旧的 Data API Key，也不自动把 Owner Session Cookie 转成 Owner Bearer。

结论：
- 本轮删除的只是 Server 内部 `internal/adapters/marketdata/**` 旧 HTTP Adapter。
- 不删除 `cmd/market-data`、`services/market-data`、Nginx `/market-api/` 代理和独立 Data 部署资产。
- 在 WebUI API 基路径、Owner Session/CSRF、Root 路由认证和 Nginx upstream 完成切换并通过真实浏览器/部署验收前，独立 Data Service 必须保留。


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
