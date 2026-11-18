# Data M3/M4 Root Server 装配核验

更新时间：2026-11-18  
Server 分支：feat/server  
代码核对基线：677798233ebd5d7aec525191d82bf350a38cbad2  
范围：记录 Server 对 Data 公开能力的装配现状和未闭环前置；不复制 Data 领域实现。

## 1. 当前已装配能力

Root 已使用最新根模块 `internal/marketdata/bootstrap.Module`，不再通过旧 Market Data HTTP API 调用自身。

| 能力 | 当前实现 | 仍需验收 |
|---|---|---|
| 配置 | `marketdataconfig.LoadForRoot()`；不要求 Data API Key 或独立 DB/Redis 参数 | 配置文件路径、真实环境 Provider 凭据和启用矩阵 |
| Store/Cache | Data Repository 使用 Root borrowed DB；Data Cache 使用 Root Redis Client Adapter | MariaDB/Redis 真实 API/Worker 启动 |
| HTTP Egress | 复用 Platform HTTP Client BoundDoer；按启用 Provider 和能力构建出口；Data RequireEgressDependencies=true | 每类启用配置的 BoundDoer 目标策略和真实连接 |
| TCP Egress | BaoStock/Pytdx 由 Server 构造 allowlist DialContext；限制 TCP、Host/Port，解析后拒绝非公网 IP，并向解析 IP 拨号 | 目标环境 DNS/网络和真实 Provider 连通性 |
| Application | Server AnalysisCollector 适配 Data Application Port，旧 Analysis Runner 直接同进程调用 Data | 数据映射、时点、质量状态和真实行情验证 |
| HTTP | Data InProcessHandler 挂载到 Root 明确的 `/api/v1/quotes/`、`bars/`、`bundles/`、`data-sources/`、`market/`、`stocks/` 路径前缀，由 Platform Router 统一 Owner Bearer 认证 | DATA-CTR-006 请求体限制、Request ID、Fault、路由集成测试 |
| Diagnostics | Data Diagnostics.Ready 注册为 Required Check；输出 Provider 数量摘要，不展开任意 ProviderDiagnostics | Data Ready 目前主要检查 DB/Redis Ping；不等同 Schema/checksum 校验 |
| Lifecycle | Server-owned Component 调用 Data Start/Stop；退出时先停消费者/模块，再 Close Data 自有资源，最后关 Root 共享资源 | 真实 API/Worker、失败回滚、SIGTERM、并发关闭 |
| Legacy | 删除 `internal/adapters/marketdata/**` 旧 HTTP Client/Collector/Config/DTO 文件 | 全仓 CI 确认无遗留引用 |

旧 Analysis Runner/HTTP API 仍保留，当前只是把其 DataCollector 依赖改为同进程 Data Application Adapter。SRV-013 旧 Analysis API/Worker 退役仍 deferred，不能与旧 Market Data Adapter 删除混为一谈。

## 2. 共享资源与 Provider 出站

### 2.1 共享资源

- Root 创建并持有 Platform MariaDB、Redis 和 HTTP Client。
- Data Repository 使用 borrowed DB；Data Cache 使用 Root Redis Adapter。
- Data Module 不拥有或关闭 Root 注入的 DB/Redis/Cache。
- Data Module 仅关闭自身创建的 BaoStock Client 等模块自有资源。
- 禁止 Data Module 再创建第二套 DB/Redis/HTTP Client。

### 2.2 HTTP Provider

Root 使用 Platform `BindHTTPDestination` 创建 BoundDoer。装配按 Provider 是否启用、是否声明对应业务能力决定，避免 disabled Provider 的空 URL 阻断整个 Root 启动。Eastmoney 按配置端点 origin 建立映射；其他 Provider 按 quote/daily-bars/search 等能力绑定对应出口。

Data `RequireEgressDependencies` 保持 true，缺少实际启用能力所需的出口时 Module 构造必须失败，不得回退到默认 HTTP Client。

待验收矩阵至少包括：
- Provider disabled 且 URL 为空；
- Provider enabled 但没有业务能力；
- Quote-only；
- DailyBars-only；
- Quote + DailyBars；
- TickFlow 有/无 API Key 的授权/公共日线出口；
- Eastmoney 多 Origin；
- 可选 Provider 配置缺失时的 skipped/degraded 语义。

### 2.3 BaoStock/Pytdx TCP

Platform 当前没有可直接复用的公共 TCP BoundDialer，因此 Server 在 Data 装配边界提供专属 DialContext，不扩展 Platform 公共 API。

当前实现：
- 只允许 tcp/tcp4/tcp6；
- 目标必须命中配置 Host/Port allowlist；
- 每次拨号解析 DNS，拒绝私网、回环、链路本地、组播、未指定及其他非公网 IP；
- 对已验证的 IP 地址直接拨号，避免验证后再按域名二次解析；
- 缺少注入时由 Data `RequireEgressDependencies` fail-closed。

真实网络连通性、DNS 变更和目标环境策略仍未验收。

## 3. Migration：仍未闭环

### 3.1 当前实际调用

Root 当前在 `MIGRATION_ENABLED=true` 时调用 Data Module 公开的 `MigrationRunner.Migrate(ctx)`，随后执行 Server Registry 中其他模块 Migration。这是对 Data 当前公开 API 的临时接入，不是最终统一注册方案。

### 3.2 仍存在的竞态

Data Store.Migrate 先调用 Platform `ApplyMigrations` 执行 `market_data` namespace 版本 1～3。Platform namespace GET_LOCK 只覆盖 ApplyMigrations 本身；返回后 Data 仍执行：
- `ensureNonUniqueHistoryIndex`
- `ensureColumn`

这些兼容操作通过 information_schema 查询后再执行 DROP/CREATE INDEX 或 ADD COLUMN，位于锁外。多个 API/Worker 实例可能同时观察到相同缺失状态并执行 DDL，存在真实并发竞态。不能把这条 Migration 链路标记为多实例安全。

### 3.3 只读校验缺口

`MIGRATION_ENABLED=false` 时：
- Server Registry.ValidateApplied 只校验已注册 namespace/version/status，不比较 Data checksum；
- Data Module.Diagnostics.Ready 当前验证 DB/Redis 依赖 Ping，不验证 Data Schema 或 migration checksum。

因此当前启动日志只能说明已注册的 Server Migration 预检及 Data 依赖 Ping 通过，不能说明 Data Schema/checksum 已通过。

### 3.4 最终责任分工

1. **Data Owner**：公开完整、不可变的 `Migrations() []platformstorage.Migration` 或等价只读定义；保持已发布 v1～v3 Statement/checksum 不变；将兼容 DDL 纳入版本 4+，并覆盖全新库和历史库。
2. **Platform Owner**：提供与 ApplyMigrations 共用 checksum preparation 的只读 `ValidateAppliedMigrations` 能力；保留 failed 状态和人工恢复语义。
3. **Server Owner**：在 Registry 注册 Data namespace、调用 Platform Apply/Validate；不访问 Data 私有 SQL，不复制 Migration Runner/checksum 算法。

前置完成前保留当前 Data MigrationRunner 调用作为兼容路径，但 SRV-002/SRV-009 Migration 仍为 IN_PROGRESS。若要进入多实例生产部署，必须先完成上述 Data/Platform 修复和真实 MariaDB 并发启动验收。

## 4. HTTP、Diagnostics、Lifecycle 验收

### HTTP

- Root 使用 Data InProcessHandler，不经 localhost 或 HTTP 自调用。
- 当前路由由 Root Platform Router 统一 Owner Bearer 认证。
- Owner Session 的 `/api/v1/owner/` 由更具体的独立路由接管，不因 Data 前缀路由而混入 Data Handler。
- 当前单测覆盖匿名 Data 请求拒绝、Owner Data 请求到达 Handler；还需按 DATA-CTR-006 验证真实 Handler 的 Principal Context、请求体上限、Request ID、Fault 投影和错误语义。

### Diagnostics

- Data Diagnostics.Ready 作为 Required Check 接入 Platform Diagnostics Runner。
- Check 只在 Ready 成功时报告 healthy；失败由 Required Check 影响 Readiness。
- ProviderSummary 只用于数量摘要；不将 ProviderDiagnostics 的任意 map 原样暴露。
- 当前 Ready 的 DB/Redis Ping 不能替代 Schema 校验；Migration history applied 也不等于物理表结构无漂移。

### Lifecycle

- Data Module 通过 Server-owned Component 注册到 Root Lifecycle。
- 停止顺序：停止 Analysis/其他消费者和 Worker → Data Module.Stop → Data Module.Close 自有资源 → 关闭 Root 共享 HTTP Client/Redis/DB。
- 仍需真实 API/Worker 两种角色测试启动失败回滚、重复 Stop/Close、SIGTERM 和并发关闭。

## 5. WebUI 与独立 Data Service 迁移边界

当前仓库事实：
- `web/src/api.js` 的行情 API 默认基路径仍是 `/market-api`，请求仍携带 `X-API-Key`。
- `web/default.conf.template` 的 `/market-api/` 仍反向代理到 `MARKET_DATA_SERVICE_NAME:8091`。
- 本轮 Root Data Handler 暴露的是 `/api/v1/quotes/`、`bars/`、`bundles/`、`data-sources/`、`market/`、`stocks/`，并由 Owner Bearer 保护；它不接受旧的 Data API Key，也不会自动将 Owner Session Cookie 提升为 Owner Bearer。
- 因此 Root Data Module 已供 Server 内部 Analysis 使用，但当前 WebUI 仍走独立 Data Service；不能声称 WebUI 已切换到 Root Data Module。

当前 Server 变更只删除 `internal/adapters/marketdata/**` 旧 HTTP Client/Collector/Config/DTO，不删除 `cmd/market-data`、`services/market-data`、Nginx `/market-api/` 代理或独立 Data 部署资产。WebUI 的 API 基路径、Owner Session/CSRF 调用、Root 路由认证策略、Nginx upstream 和独立服务下线需要由 Web/Data/部署任务单独完成。没有真实浏览器与部署验收前，保留旧独立服务是正确的兼容边界。

## 6. SRV-009 完成门槛

- Data Module 使用唯一 Root 共享 DB/Redis/HTTP Client，无重复连接池。
- 所有实际启用 Provider 的 HTTP/TCP 出口均受控；disabled Provider 的空 URL 不阻断启动；缺少必需出口 fail-closed。
- Analysis 通过 Data Application Port 读取行情，数据时点、复权口径、质量状态映射通过测试。
- Data InProcessHandler 按 DATA-CTR-006 完成认证、Request ID、请求体限制和 Fault 端到端验收。
- Data Diagnostics.Ready 与 Schema/checksum 只读校验边界明确。
- Migration 执行/只读校验并发安全，失败恢复有真实 MariaDB 证据。
- API/Worker、真实 MariaDB/Redis、SIGTERM、启动失败回滚和并发启动均在精确 SHA CI 通过。
- CI/Fixture 不替代真实 Provider 行情数据验收。
