# Server 模块卡点

更新时间：2026-11-18
维护分支：feat/server
代码验收基线：f5821a356e23691d9738f4b3808abf8e0e3494d8；当前文档 HEAD：f5821a356e23691d9738f4b3808abf8e0e3494d8
当前主线：SRV-015 其他业务模块 Root 装配盘点与可接入服务装配

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

- Data Handler 的 DATA-CTR-006 端到端行为：Owner 身份上下文、请求体限制、Request ID、Fault 投影和实际路由分发。
- API 与 Worker 两种角色的真实 Root Bootstrap、启动失败回滚和生命周期关闭。
- 启用 Provider 配置矩阵及 HTTP/TCP 受控出口注入；真实 Provider 连通性没有真实上游证据时标记 NOT_COMPARABLE。
- Data Module Start/Stop/Close 与 Root 共享资源的关闭顺序、幂等性和并发边界。

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
- Notification Delivery：虽有持久化 Repository 和领域用例，但缺权威 Recipient Directory、真实 Channel Adapter/凭据、投递 Worker 与回执/Unknown Lookup 闭环。
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
