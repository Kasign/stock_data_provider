# Market Data 并入 Root Server 模块化单体：最终架构决议

更新时间：2026-11-19
维护分支：repair_all

## 1. 决议

Market Data 与 Root Server 运行在同一 Go Module、同一进程中。Root Server 是唯一外部 HTTP 入口和唯一组合根。Market Data 是有独立领域边界的内部模块，不再作为独立 HTTP 服务部署。

生产部署只包含 Root Server 与 WebUI。旧独立 Data 服务、旧 Data API Key、独立 Dockerfile、独立 Compose Profile、Remote HTTP Adapter 和双 upstream 均退役，不保留回滚兼容实现。

## 2. 模块职责

### Market Data

- 拥有 Provider、数据模型、标准化、质量、PIT、Freshness、Repository、Schema/Migration 和 Data Application Port。
- 负责 Data 领域查询、探测和领域授权语义。
- 提供 Module 构造、InProcessHandler、Diagnostics、Provider Summary 和生命周期能力。
- 不创建独立 HTTP Listener，不持有独立 Data API Key，不创建或关闭 Root 共享 DB/Redis 连接池。

### Root Server

- 唯一外部 HTTP 入口与组合根。
- 构造 Market Data Module，注入共享 MariaDB、Redis、受控 HTTP/TCP 出站执行器。
- 统一注册 Data Migration Definitions、Readiness、Diagnostics 和 Lifecycle。
- 通过 Application Port 向其他内部模块提供 Data 能力。
- Data HTTP 路由显式接受 Owner Session 或 Owner Bearer；Session 非安全请求执行 Origin/Host/CSRF 校验。
- Owner Session 不继承 platform:admin 或 Service Scope；Data 领域授权仍由 Data 自身执行。

### WebUI / Nginx

- WebUI 只访问同源 /api。
- Owner Session 使用 HttpOnly、Secure、SameSite=Lax、Path=/api Cookie。
- CSRF Token 仅驻留 JS 运行时内存；所有非安全请求发送 X-CSRF-Token。
- 不持有 Data API Key、Owner Bearer 或 Provider Secret。
- Nginx /api/ 原样转发到 Root Server；不做 Cookie Path 改写，不设置第二个 Data upstream。

## 3. 路由认证矩阵

| 路由 | 身份 | 约束 |
|---|---|---|
| /healthz | anonymous | Root Liveness |
| /readyz、/diagnostics | owner_bearer | 运维入口 |
| /api/readyz | owner_session 或 owner_bearer | WebUI Readiness |
| /api/v1/owner/* | SRV-CTR-002 | Session 生命周期 |
| /api/v1/quotes、bars、bundles、data-sources、market、stocks | owner_session 或 owner_bearer | Root Data 路由；写请求 CSRF |
| /api/v1/analysis/* | 现有 owner_bearer | 不隐式扩展认证范围 |

不得接受浏览器 X-API-Key，不得让 Nginx 注入 Owner Bearer，不得同时接受 Session Cookie 和 Authorization Header。

## 4. 已落地的实现边界

- Data 位于 internal/marketdata；Server 在 internal/server/bootstrap 构造 Module。
- Data Application Port 被现有内部消费者使用；Data InProcessHandler 由 Root Router 挂载。
- Data Migration Definitions 由 Root Migration Registry 管理。
- Data Readiness 纳入 Root Diagnostics；Data Module Lifecycle 纳入 Root 启停顺序。
- Root Compose 仅有 stock-analysis-server 和 stock-analysis-web。
- WebUI 与 Nginx 使用单一 /api 入口。
- Data 独立服务入口、独立镜像/Compose 配置、Data API Key 认证与兼容迁移脚本已删除。

## 5. 明确不再采用的方案

- 独立 stock-analysis-data 容器或 8091 监听端口。
- 浏览器 localStorage 保存 Data API Key。
- X-API-Key 作为 Root 内部调用凭据。
- Root Server 通过 HTTP 自调用 Data。
- Remote/InProcess 双路 fallback。
- 独立 Data 服务 Profile、双 upstream、双轨部署或回滚服务。
- 由反向代理伪装 Owner Bearer。

Provider 自身需要的 Tushare、同花顺、TickFlow、雪球等凭据仍是服务端 Provider Secret，继续由 Root/Data 服务端配置管理，不属于已退役的 Data API Key。

## 6. 验收门槛

- Data/Server/WebUI 的路由、认证、CSRF、Request ID 和 DTO 测试通过。
- Compose 校验、Nginx 路由校验、Root Server 与 WebUI 镜像构建通过。
- 最新精确 SHA 的完整 Integration CI 全部通过。
- 目标环境真实 HTTPS Owner Session、Cookie、CSRF 和 Data API 浏览器验收通过。
- 真实 MariaDB/Redis、Migration enabled/disabled、Schema 漂移、备份恢复和 SIGTERM 关闭验收通过。
- Provider 真实请求、数据字段、时间、质量和 PIT 证据独立通过；未取得真实证据的能力保持 not-comparable / skipped。

当前代码阶段：M1～M4 完成；M5 代码和单入口配置已实现，等待最新精确 SHA 完整 CI；M6 生产环境与真实 Provider 数据证据待完成。
