# Data M3/M4 Root Server 装配前置核验

更新时间：2026-11-18  
Server 分支：feat/server  
代码核对基线：a2ecf3a80bce9394aab79c786b9aaa53817866a3（后续提交仅修改 doc/server）  
范围：只记录 Server 消费 Data 公开能力的前置条件；不修改 Data/Platform 代码。

## 1. Data 已有公开能力

当前根 Go Module 中已存在 internal/marketdata/bootstrap.Module，公开入口包括：

- New(config, Dependencies)：构造 Data Module。
- Application()：返回 MarketDataReader Application Port。
- InProcessHandler()：返回由 Root Server 承担外层认证/传输策略的 Handler。
- Diagnostics()：返回 Ready(ctx) 与 ProviderDiagnostics()。
- MigrationRunner()：返回模块 MigrationRunner。
- Start/Stop/Close：管理 Data 自有后台任务和模块自有资源。

Dependencies 已提供共享 Store/Cache、逐 Provider HTTPDoer、BaoStock/Pytdx TCP DialContext 和 RequireEgressDependencies。Data Module 不拥有调用方注入的 Store/Cache，只关闭自身创建的 BaoStock Client。

结论：Server 不应再实现第二套 Data Bootstrap、Provider Registry、Application Port、HTTP Handler 或 Cache。

## 2. Root 当前缺口

当前 internal/server/bootstrap/bootstrap.go 仍加载旧 internal/adapters/marketdata 配置，并调用旧 service.NewRunnerWithMarketDataStores。当前 Root 未构造 Data Module，未注入 Data Application，未挂载 Data InProcessHandler，未把 Data Diagnostics 接入 Readiness，未把 Data Module 生命周期接入 Server Lifecycle。

当前 Server Migration Registry 已注册 News、Owner Credential、Research、Researchflow、Notification、Security Audit；没有 Data。

## 3. 共享资源与受控出站

### 3.1 共享资源

Server Resources 已通过 Platform 打开并持有 MariaDB、Redis、HTTP Client，且提供 BindHTTPDestination。

- Data Store 应借用 Server 持有的 SQL DB。
- Data Cache 应复用 Server 持有的 Redis Client Adapter。
- Data Provider HTTP 出口应复用 Platform HTTP Client.Bind() 生成的 BoundDoer。
- 不得在 Data Module 内重新创建/关闭共享 DB、Redis 或 HTTP Client。

### 3.2 Provider 出站

Data Module 的 RequireEgressDependencies 可要求所有实际启用的 Provider 能力具备注入的执行器。Root 必须基于配置后的真实 Provider endpoint 建立精确 DestinationPolicy，不能只检查配置字段非空，也不能对缺失执行器静默回退标准 HTTP Client。

BaoStock/Pytdx 使用 TCP DialContext，不属于 HTTP BoundDoer。当前 Platform 代码树未发现公共 TCP BoundDialer。先检查跨模块真实消费者：
- 若仅 Data 使用，评估由 Server 为 Data 提供专属受控 DialContext，不扩大 Platform 公共接口。
- 若有多个模块需要相同安全能力，再由 Platform Owner 评估公共化。
- 无论归属如何，未注入受控拨号时必须 fail-closed；不能把 net.Dialer 直连作为生产通过。

## 4. Migration 对接问题与复核结论

### 已核实的代码事实

- Data Module 的 `MigrationRunner` 仅公开 `Migrate(ctx) error`；Server Registry 的 `Register` 接收 `[]platformstorage.Migration`，二者不能直接注册。
- `Store.Migrate` 先通过 `platformstorage.ApplyMigrations` 执行 `market_data` namespace 的版本 1～3。
- Platform ApplyMigrations 在单个 MariaDB 连接上获取 namespace GET_LOCK；锁只覆盖 ApplyMigrations 调用本身。
- 返回后，Data 继续调用 `ensureNonUniqueHistoryIndex` 和 `ensureColumn`。这些方法先查 information_schema，再执行 DROP/CREATE INDEX 或 ADD COLUMN，全部在 Platform 锁外。
- 多个 API/Worker 实例可同时观察到索引/列缺失并执行 DDL；后执行者可能遇到索引/列已存在或已删除错误。现有 Platform 并发测试只覆盖 ApplyMigrations，不覆盖这些锁外 DDL。
- Platform migration checksum 根据 Statement 计算；已发布版本 1～3 不得修改 Statement。

### 推荐方案（明确结论）

1. **Data 定义迁移，Server 汇总注册，Platform 执行。** Data 在公开 Bootstrap API 提供完整、不可变的 `Migrations() []platformstorage.Migration`（或等价只读定义接口），Server 在构造 Data Module 前即可注册 `market_data` namespace。迁移 SQL 仍归 Data 所有。
2. **保留版本 1～3，兼容修复追加版本 4+。** 将当前锁外的索引修复和补列逻辑纳入新版本。必须同时兼容全新库和历史库，不能假设旧索引一定存在，也不能修改旧版本来改变 checksum。
3. **所有 Schema DDL 必须受同一 Platform namespace lock 保护。** 优先使用经目标 MariaDB/MySQL 版本验证的条件 DDL；如果静态 SQL 无法兼容目标版本，由 Platform Owner 评审受锁条件迁移能力。不得继续在 Runner 外执行 information_schema 检查后再 ALTER。
4. **补齐只读 checksum 校验。** Server Registry 当前 `ValidateApplied` 只读取 namespace/version/status，没有读取或比较 `platform_schema_migrations.checksum`。它的注释称 checksum 由 Platform Runner 校验，但 `MIGRATION_ENABLED=false` 时不会执行 ApplyMigrations，因此该模式确实漏检。建议 Platform 提供与 ApplyMigrations 共用 migration preparation/checksum 逻辑的只读 `ValidateAppliedMigrations` 公共方法，Server Registry 逐 namespace 调用；不要在 Server 重写 SHA-256 规则。
5. **保持失败恢复语义。** DDL 可能隐式提交；失败版本由 Platform 标记 failed 并要求人工恢复。不能自动把 failed 版本改成 applied，也不能在未确认部分 DDL 状态时盲目重跑。

### Root 调用顺序

- Bootstrap 创建共享 MariaDB 连接池。
- 静态 Migration Registry 获取各模块公开定义并注册（Data 定义无需先构造 Module）。
- `MIGRATION_ENABLED=true`：Registry 调用 Platform ApplyMigrations。
- `MIGRATION_ENABLED=false`：Registry 调用 Platform 只读版本、状态和 checksum 校验。
- Migration 成功后再构造 Data Module，注入 borrowed Store/Cache；Module 不拥有共享 DB/Redis。
- Data Module Ready 另行检查运行所需依赖和实际 Schema；migration history 显示 applied 不等于物理表结构绝无漂移。

**实施责任：Data Owner 提供公开定义并收敛锁外 DDL；Platform Owner 提供只读 checksum 校验；Server 只负责注册和调度。两个前置未完成前，不接入当前 `MigrationRunner.Migrate`，也不宣称 SRV-002/SRV-009 Migration 完成。**

## 5. Diagnostics 与 Readiness

Data Diagnostics.Ready(ctx) 是模块公开的就绪检查入口。Server 接入时必须将它适配为 Platform Diagnostics Check，并明确 Required/Optional。

- PASS：共享 DB/Redis 可用，Data Ready 成功，必需 Schema 已通过只读校验。
- DEGRADED：必需服务和 Schema 正常，但可选 Provider/能力被明确标记 skipped/degraded，输出稳定 reason_code 和证据。
- FAIL：Data Ready 失败、必需存储/Schema 不可用、必需 Provider 受控出口缺失；Required 检查不健康时 Readiness=false。
- NOT_COMPARABLE：Provider 未配置、没有真实样本或缺少可比较来源；不得把无数据解释为 Provider 健康，也不得将其标记为真实数据验收 PASS。

ProviderDiagnostics 的具体字段只有在确认字段来源、含义、敏感信息和稳定性后才可映射；不把任意 map[string]any 原样暴露到 Root Diagnostics。

## 6. HTTP 路由与认证

- Root 使用 Data InProcessHandler，不通过 localhost 或远程 HTTP 自调用。
- 路由必须遵循 DATA-CTR-006：同进程可信 Principal Context 与远程兼容 X-API-Key 模式严格区分；不得恢复已被禁止的 Bearer/API Key 混用。
- WebUI 不保存或发送 Data Provider API Key。
- 路由认证主体、Owner Session 是否可访问、错误投影、Request ID、超时和请求体上限必须有明确契约映射；没有证据的路由不注册。
- 只有在真实调用路径确认后才挂载 /market-api；不为 UI 预先增加接口或字段。

## 7. Lifecycle 与资源所有权

Data Module 自身 Start/Stop/Close 与 Server runtime.Component 的接口不完全相同。Root 需要一个 Server-owned 生命周期适配器：
- Start 调用 Data Module.Start。
- Stop 先停止 Data 后台任务，再关闭 Data 自有资源。
- Data Module 不得关闭 Server 注入的 DB/Redis/Cache。
- 只有全部模块消费者停止后，Server 才关闭共享 HTTP Client、Redis、DB。
- 启动失败时按逆序回滚；停止超时不得提前关闭仍被在途任务使用的共享资源。

## 8. SRV-009 完成门槛

- Data Module 使用唯一 Root 共享 DB/Redis/HTTP Client，无重复连接池。
- 每个实际启用 Provider 能力均有精确、受控的 HTTP/TCP 执行器；缺失依赖 fail-closed。
- Migration 的执行、只读验证、并发安全和失败恢复有真实 MariaDB 证据。
- Data Application/InProcessHandler 只在有真实消费者和正式认证映射时接入。
- Data Diagnostics.Ready 的 PASS/DEGRADED/FAIL/NOT_COMPARABLE 语义固定。
- Module Start/Stop/Close、Server Lifecycle 和共享资源关闭顺序有测试。
- API/Worker 独立角色、真实 MariaDB/Redis、SIGTERM 和并发启动均有精确 SHA 验收。
- 真实 Provider 数据验收单独记录；Mock/Fixture/CI 不能替代真实行情数据证据。


## 9. 项目总览 MDM 状态快照差异

项目总览《整体任务规划总览.md》中的 MDM 进度说明标注为 2026-11-06，仍写明 `services/market-data/go.mod` 存在、MDM-002 尚未开始。对本轮 `feat/server` HEAD 的 Git Tree 核对结果：
- 根目录只有一个 `go.mod`；当前 Tree 中不存在 `services/market-data/go.mod`。
- 根模块已存在 `internal/marketdata/**`。
- 但仍存在 `services/market-data/Dockerfile`、`services/market-data/migrations/**`、`cmd/market-data/**` 等旧部署/入口资产。

因此总览中“嵌套 go.mod 仍存在”的具体事实已经过时；但 MDM-002 是否整体完成仍不能据此判定，因为旧 Docker/迁移/命令入口尚在，且 MDM-002 还包含 Go 版本、依赖、CI 和 Docker Builder 统一。Server 分支不修改项目级权威总览；应由总览维护方基于当前 Data 分支/Integration 精确 SHA 更新 MDM 状态和残留清单。SRV-009 只处理 Root Server M3/M4 装配，不代替 MDM-002/006/007 验收。
