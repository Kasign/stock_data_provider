# Market Data 模块化单体迁移与最终验收状态

更新时间：2026-11-19
维护分支：repair_all
代码基线（迁移实现审查基线）：9ce0d0b536e2d9b37f0fb5ff4cc2ab8ae59c6ba4；最新提交 SHA 以 RepairAll 总规划为准

## 1. 最终架构决策

**唯一生产 API 入口是 Root Server。Market Data 只作为 Root Server 进程内模块运行，不再提供独立 Data HTTP 服务。**

- Root Server 是唯一外部 HTTP 入口和组合根，负责 HTTP 路由、身份认证、CSRF、Request ID、请求限制、共享资源和生命周期。
- Market Data 保留领域所有权：Provider、标准化、质量、PIT、缓存语义、Repository、Schema/Migration 和 Application Port。
- Root Server 创建 Market Data Module，注入共享 MariaDB、Redis 和受控 HTTP/TCP 出站依赖；消费者通过 Application Port 使用 Data 能力。
- Data HTTP Adapter 只保留 Root 进程内路由适配器，由 Root Router 外层认证后调用；不提供独立 Handler、独立监听端口或 Data API Key 认证。
- WebUI 只使用同源 /api。Owner Session Cookie 为 HttpOnly、Secure、SameSite=Lax、Path=/api；CSRF Token 仅驻留浏览器运行时内存。
- Data 路由显式接受 Owner Session 和受控 Owner Bearer。Owner Session 不映射为 Owner Bearer，不继承 platform:admin 或 Service Scope。Data 领域授权仍按其自身契约执行。
- 所有 Session 非安全请求必须通过现有 SessionAuthenticator 的 Origin/Host 与原子 CSRF 校验；Provider Probe POST 同样适用。
- Provider API Key、Token、Cookie 是服务端 Provider 凭据，与已退役的浏览器 Data API Key 无关。

不保留独立 Data 服务、旧 Data API Key、X-API-Key 兼容、双 API upstream、旧 Data 容器/镜像、独立部署 Profile 或回滚分支。新架构和旧实现清理在同一迁移批次完成。

## 2. 代码与部署状态

| 阶段 | 当前状态 | 证据 / 边界 |
|---|---|---|
| M1 Go Module 归并 | DONE | Data 代码位于 internal/marketdata，根 Go Module 统一依赖 |
| M2 Application Port / Module | DONE | Data Module、Application Port、Provider 装配、生命周期和 Data InProcessHandler 已存在 |
| M3 Root Server 装配 | DONE | Server 创建 Module、注入共享资源、挂载 Data 路由并向 Analysis 注入 Application |
| M4 Platform / Migration / Readiness | DONE（代码级） | 共享 DB/Redis、受控出站、静态 Migration Definitions、Root Registry、Diagnostics/Readiness/Lifecycle 已装配 |
| M5 Owner Session 与单入口切换 | DONE_CODE_CI | WebUI 同源 /api；Root Data 路由 Owner Session/Owner Bearer；Compose 仅 Root Server + WebUI；Nginx 单 /api upstream；独立 Data 资产已删除 |
| M6 生产验收 | BLOCKED_EXTERNAL_EVIDENCE | 仍需目标环境真实 HTTPS、MariaDB/Redis、Migration 演练、Provider 数据与部署证据 |

### 本批明确删除的退役资产

- cmd/market-data/main.go
- services/market-data/ 独立 Dockerfile、.env、迁移副本和构建上下文
- docker/docker-compose.go-migration.yml 中旧 Data 服务 Profile
- scripts/migrate-market-data-module.sh
- Data HTTP Adapter 的独立 Handler、X-API-Key 校验和 MARKET_DATA_API_KEY 配置
- Module.MigrationRunner 独立服务兼容入口
- Data OpenAPI 中独立 Data API Key 安全方案及独立服务地址

Root 的 docker-compose.yml 只运行 stock-analysis-server 和 stock-analysis-web。Nginx 的业务 API 路由只有 /api/，直接转发 Root Server，不做 Cookie Path 改写。Root /healthz 仍作为匿名存活检查。

## 3. 认证与路由约束

| 路由 | 允许身份 | 说明 |
|---|---|---|
| /healthz | anonymous | Root Liveness |
| /readyz、/diagnostics | owner_bearer | 受控运维接口 |
| /api/readyz | owner_session 或 owner_bearer | WebUI Readiness 视图；Cookie Path 内 |
| /api/v1/owner/* | 按 SRV-CTR-002 | 登录、Session、CSRF 恢复、登出、轮换、密码修改 |
| /api/v1/quotes、bars、bundles、data-sources、market、stocks | owner_session 或 owner_bearer | Root Router 显式路由策略；Session 写请求执行 Origin/Host/CSRF |
| /api/v1/analysis/* | 现有 owner_bearer | 不因 Owner Session 启用而自动放宽 |

Owner Bearer 不下发浏览器。浏览器不保存 Data API Key、Owner Bearer 或 Provider Secret。不得由 Nginx 静态注入 Owner Bearer，也不得允许同时提交 Session Cookie 与 Authorization Header。

## 4. 自动化验收与生产证据

代码层验收已通过：精确代码 SHA `1e916220a8784ae840132ce0cec3b3654c6b9655` 的 Integration CI [Run 44482650366](https://github.com/Kasign/stock_data_provider/actions/runs/44482650366) 全项 success。此前 CI 暴露的 OpenAPI 操作矩阵常量、WebUI readiness 路径和 Owner Session 初始化时机断言已按实际契约修正；随后补齐 `/api/v1/data-sources` 精确根路由认证注册及 Owner Bearer/未认证回归。生产环境验收仍未执行，CI 不代表 M6 通过。

必须完成：

1. Root Data 路由测试：Owner Session、Owner Bearer、未认证、重复/冲突凭据、CSRF 缺失/错误、Origin/Host 不匹配。
2. WebUI 测试：登录、刷新后 CSRF 恢复、Data GET、Provider Probe POST、登出、轮换；localStorage 不再保存旧 Key，所有请求不发送 X-API-Key。
3. Data/OpenAPI 测试：仅保留 Data 业务操作，不再定义独立 Data HTTP API Key 或独立服务入口。
4. Compose 配置校验、Nginx 路由检查、Root Server 镜像和 WebUI 镜像构建。
5. 完整 Integration CI 已在精确代码 SHA `1e916220a8784ae840132ce0cec3b3654c6b9655` 全部通过（Run 44482650366）；后续代码变更仍须重新验收，不得通过降低门禁或将失败步骤改为 skipped 规避。
6. 目标环境真实 HTTPS Origin 与 Argon2id 参数基准测试；真实 Owner Session Cookie/CSRF 浏览器验收。
7. Root 共享 MariaDB/Redis、Migration enabled/disabled、Schema 漂移、启动失败回滚、SIGTERM 优雅关闭验收。
8. 真实 Provider 请求与数据质量验收。Provider 缺少凭据或权限时记为 skipped / not-comparable，不得伪造成功。
9. 生产脱敏 DDL 对照、真实数据库迁移和备份恢复演练。

## 5. 当前执行结论

- M1～M4 不再重复实施或重复审查。
- M5 的代码、部署配置、旧独立 Data 资产清理及精确 SHA Integration CI 均已通过；不保留旧服务或回滚路径。
- M6 需要真实部署环境和数据证据，不能由 Mock、CI 临时数据库、静态配置检查或 Provider 能力声明替代。
- Data 的 DATA-003～DATA-007 真实披露时点、估值字段、Freshness、Go/Python parity 和 Provider 专项证据仍按 Data 总规划管理，不因本次部署迁移而关闭。
- 不创建 Remote Adapter、双轨部署或旧服务回滚路径。
