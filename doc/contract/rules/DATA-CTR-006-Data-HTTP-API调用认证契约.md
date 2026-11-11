# DATA-CTR-006 Data HTTP API 调用认证契约

- 契约编号：`DATA-CTR-006`
- 契约版本：`1.0.0`
- 状态：`pending_evidence`（认证语义已收敛；OpenAPI 与 Handler 尚未符合，不能提交冻结）
- 生产方：Data HTTP Adapter；Root Server 负责默认同进程入口认证和 Remote Adapter 凭据注入
- 消费者：Root Server Remote Data Adapter（仅远程兼容模式）；运维/受控服务端 HTTP 调用方
- 权威依据：本文件、PLAT-CTR-003 1.0.0、DEC-PLAT-002-017、DEC-PLAT-002-038、DEC-PLAT-002-041
- 代码核对基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`

## 1. 部署模式与认证边界

Data 的默认部署/调用模式为 Root Server 与 Data 同进程的模块化单体：

- Root Server 是唯一外部 HTTP 入口；在路由层完成认证和路由授权，再将可信 Principal/Scope Context 注入 Data Application Port。
- Data InProcessHandler/Application Port 不执行第二套 HTTP API Key 或 Bearer 认证；不得从客户端 Header、Body 或 Query 构造 Principal。
- 同进程调用继承 Root Server 已验证的可信 Principal Context。Data 仍负责自身资源授权、AuthorizationRef、Snapshot Manifest 和读取时复核，不能因入口已认证而跳过 DATA-CTR-008。
- 当前默认模式不配置、不读取、不注入 Data API Key，不创建 Data Service Base URL，不产生 Server→Data 内部 HTTP 请求。

Data HTTP API 仅保留为远程兼容/独立部署模式：

- 只有该模式启用 Data HTTP Adapter 时，才需要 Data 专用 API Key。
- API Key 只认证受控的 Server Remote Data Adapter/服务端调用方，不是用户身份，不是资源授权凭证。
- WebUI、浏览器、用户请求和 LLM Tool 永远不得持有、传递或获取 Data API Key。
- Data API Key、Owner Bearer、Owner Session Cookie、Platform Service Bearer、Provider Token 必须彼此独立，不得复用同一 Secret 或相互替代。

## 2. 远程模式的唯一 HTTP 认证方案

远程兼容模式下，业务 HTTP API 唯一接受：

- Header：`X-API-Key`
- 凭据来源：Root Server Remote Data Adapter 从受控 Server Config/Secret 装载，并在发起 Data HTTP 请求时固定注入。
- Data HTTP Adapter 在受保护业务路由入口校验该 Key；不得信任客户端传入的同名 Header，Server Adapter 必须覆盖而不是透传用户 Header。

明确禁止：

- `Authorization: Bearer` 作为 Data API Key 的别名或备用认证方式。
- 同一 Key 同时接受 X-API-Key 与 Bearer 两种 Header。
- 将 Platform Service Bearer Secret 直接当作 Data API Key；两者的身份、受众、Scope 和轮换语义不同。
- 从 URL Query、Cookie、请求 Body 或任意用户自定义 Header 读取 Data API Key。
- 将认证 Key 映射为 `AuthorizationRef.subject_ref` 或 `SnapshotManifest.caller_ref`。

OpenAPI 约束：

- `components.securitySchemes` 只为 Data API Key 声明 `ApiKey`（`type: apiKey`、`in: header`、`name: X-API-Key`）；本 API 不声明或引用 `BearerAuth`。
- 顶层 `security` 只包含一个 requirement object：`ApiKey: []`。不得将 ApiKey 与 BearerAuth 放在两个 requirement object 中形成 OR 语义。
- `/health/live` 与 `/health/ready` 显式声明 `security: []`；其他业务 Operation 继承顶层 ApiKey 要求。若未来出现例外，必须在对应 Operation 显式声明并更新本契约。
- OpenAPI Security 表达 HTTP Adapter 的认证方式，不表达同进程 Application Port 的认证方式。

## 3. 凭据生命周期与 Secret 保护

- Data API Key 必须由密码学安全随机源生成，建议至少 256 bit 随机熵；不同环境不得复用。
- Server 仅在远程兼容模式装载 Data 专用 Secret；默认同进程模式不得要求该配置。
- Secret 通过类型化 Secret 传递；只有 Remote Data Adapter 构造认证请求时允许显式读取明文并设置 Header。
- Data HTTP Adapter 只在认证边界读取/比较 Key；比较必须使用常量时间路径。
- 不在日志、Fault、诊断 attributes、Request ID、响应体、URL、持久化业务记录或错误消息中记录 Key、Header 原文或完整摘要。
- 首期不承诺双 Key 并行、无停机轮换或动态撤销。若远程模式启用，凭据轮换必须使用维护窗口/受控切换：先让所有 Data 实例装载同一新 Key，再切换 Remote Adapter；确认旧 Key 在所有实例失效后恢复流量。不得将单实例验证等同全实例撤销。
- 若未来需要无停机双凭据轮换、Key ID 或即时撤销，必须升级本契约并明确多实例过渡语义；不得只靠部署脚本暗中扩展。

## 4. 认证、授权与失败语义

- 缺失、格式非法或不匹配的 X-API-Key：HTTP 401，稳定错误类别为 unauthenticated。
- Key 认证成功只证明受控 Remote Adapter 凭据有效；不自动授予所有 Data 资源访问权限。
- 可信调用主体和 Scope 必须来自 Server 已认证的 Principal Context。凭据字符串不得作为 subject_ref/caller_ref；资源 Owner 仍按 DATA-CTR-008 执行 purpose、Scope、AuthorizationRef、PolicySnapshot 和读取时复核。
- 已认证主体但资源/Scope 授权拒绝：HTTP 403，归属 DATA-CTR-008 的授权语义，不得伪装成 401 或 Provider 故障。
- 授权策略/快照依赖不可用：按 DATA-CTR-008 fail closed，使用其定义的 503 语义；不得降级为无授权读取。
- 认证失败、授权拒绝、Data 内部错误、上游 Provider 错误必须保持可区分；401/403 不得计入 Provider 可用性失败。
- Remote Adapter 必须保留可信逻辑调用主体。跨 HTTP 的可信主体传递/绑定机制不在本契约中发明，须由 DATA-CTR-008 明确；普通客户端 Header/Body/Query 自报主体一律不可信。

## 5. 健康接口

- `GET /health/live` 与 `GET /health/ready` 不要求 Data API Key，OpenAPI 对二者显式使用 `security: []`。
- 健康响应只提供最小存活/就绪结论及必要的标准错误，不暴露 Provider 凭证、Data API Key、连接串、详细依赖拓扑或内部配置。
- 健康接口无认证不意味着业务 API 无认证；部署网络边界是纵深防御，不替代业务 API 的 X-API-Key 校验。

## 6. 与其他契约的职责分界

- PLAT-CTR-003 定义 Principal/AuthMethod、Bearer、Owner Session、Service Scope 和 Secret 原语；本契约不扩展 Platform 身份枚举或 Service Scope。
- SRV-CTR-001 定义 Root Server 外部路由的 AuthMethod 矩阵、Session/Cookie/CSRF 和入口装配。
- DATA-CTR-008 定义 Data 资源授权、可信 caller/subject、purpose_code、AuthorizationRef、Snapshot Manifest 和读取时复核。
- 本契约只定义 Data 远程 HTTP Adapter 的传输凭据与默认同进程模式的认证边界，不定义 Provider Token，也不把 Data HTTP API 变成默认外部入口。

## 7. 当前基线差异与冻结门禁

在固定代码基线 `b6dfc85ba214fe16268c8cba780641a8bfe15086` 上发现：

1. `internal/marketdata/httpapi/openapi.yaml` 同时声明 ApiKey 和 BearerAuth，顶层两个 Security Requirement Object 表示 OR；它没有逐业务 Operation 显式覆盖为 ApiKey。
2. `internal/marketdata/httpapi/server.go` 的 `Server.auth` 在 X-API-Key 缺失时回退读取 `Authorization: Bearer`，并将两种 Header 的值与同一 `s.apiKey` 比较。
3. `internal/marketdata/httpapi/platform_contract_test.go` 明确将 Bearer Header 作为成功认证用例。
4. 历史 `doc/api/contracts/数据层API清单.md` 仍记载浏览器保存并发送 X-API-Key，且引用旧服务路径；该文件是历史接入说明，不是当前安全契约权威来源。

因此，虽然认证语义已由本文件与 DEC-PLAT-002-041 收敛，当前 OpenAPI/Handler/Test 仍共同表达了不符合目标的 Bearer 兼容行为。由于机器公开定义尚未对齐，本 CTR 继续为 `pending_evidence`；后续由 Data 责任范围同步 OpenAPI 和 Handler 契约定义后，再重新核验。本轮不修改 Data 代码、OpenAPI、测试或模块专项文档。

## 8. 版本与兼容

- 契约版本：1.0.0。
- 首版明确远程兼容模式唯一使用 X-API-Key；同进程默认模式不使用 Data API Key。
- 增加新的认证方式、改变凭据来源、改变认证 Header、放宽健康接口或改变凭据轮换/授权边界均属于破坏性语义变化，必须按 GOV-CTR-004 升级版本并记录项目级决议。
