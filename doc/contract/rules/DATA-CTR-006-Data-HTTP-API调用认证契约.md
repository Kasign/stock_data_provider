# DATA-CTR-006 Data HTTP API 调用认证契约（退役记录）

- 契约编号：`DATA-CTR-006`
- 原契约版本：`1.0.0`
- 状态：`retired`
- 退役决策：DEC-PLAT-002-054（2026-11-19，项目负责人明确批准）
- 替代契约：SRV-CTR-002 1.1.0
- 退役范围：独立 Data HTTP 服务认证、Data API Key、`X-API-Key`、Remote Data Adapter 凭据注入及其 OpenAPI 安全定义。
- 保留目的：仅记录历史契约退役事实与替代关系；本文不再定义可部署或可调用的兼容认证方式。

## 1. 最终部署边界

Market Data 已迁入 Root Server 同进程模块化单体。Root Server 是唯一外部 HTTP 入口；Data 通过 InProcessHandler/Application Port 提供能力，不再独立部署 Data HTTP 服务。

- 不创建、不配置、不读取、不注入 Data API Key。
- 浏览器、WebUI、用户请求和 LLM Tool 不得持有或发送 `X-API-Key`。
- 不保留 Remote Data Adapter、独立 Data Service Base URL、独立 Data HTTP API Key 认证模式。
- Data 业务 HTTP 路由的外部认证统一归 Server SRV-CTR-002：WebUI 使用 Owner Session Cookie；受控运维调用可使用 Owner Bearer。
- Server 将通过认证的真实 Principal Context 注入 Data Application；Data 领域授权仍由 DATA-CTR-008 等领域契约负责。

## 2. 认证职责迁移

原 DATA-CTR-006 所描述的传输层 API Key 认证不再适用。当前认证矩阵、Cookie、CSRF、Origin/Host、Owner Bearer 和 Owner Session 的定义以 `doc/server/contracts/SRV-CTR-002-owner-session-http-api.md` 1.1.0 为唯一权威来源。

Platform 身份原语继续由 PLAT-CTR-003 定义；Data 资源授权、AuthorizationRef、Snapshot Manifest 和读取时复核继续由 DATA-CTR-008 定义。不得将 Owner Session 直接等同于资源授权通过。

## 3. 清理要求

RA009 迁移批次完成时，必须删除：
- `MARKET_DATA_API_KEY`、`X-API-Key`、Remote Data Adapter Secret 和相关配置；
- 独立 Data HTTP 服务入口、容器、Dockerfile/Compose service 和健康检查；
- Nginx `/market-api/` upstream；
- WebUI 中 Data API Key 的本地存储、设置输入、校验、提示和请求 Header 注入；
- 只为独立 Data HTTP 服务认证保留的 OpenAPI security scheme、Handler 分支、测试和部署文档。

不保留旧 Key 兼容、双服务并行、独立 Data 服务回滚路径。历史 Migration、Data 领域数据、Provider 凭证和仍由 Root/Data Module 使用的 HTTP Handler 业务实现不属于本契约退役范围，不得误删。

## 4. 验收

退役验收要求在 RA009 完成时核对全仓：
- 不存在运行时 Data API Key 配置或 `X-API-Key` 发送/接收逻辑；
- 不存在独立 Data 服务部署/入口引用；
- 所有 Data 业务请求通过 Root Server 显式认证路由进入；
- WebUI Owner Session/CSRF、Server Owner Session/Owner Bearer 和 Data 领域授权测试通过；
- 真实 Root 部署、Data Readiness 和 Provider 数据验收通过。

在 RA009 尚未完成时，本文件只表示契约已退役，不得据此声称代码和部署清理已经完成。
