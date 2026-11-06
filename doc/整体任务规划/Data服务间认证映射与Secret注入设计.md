# Data 服务间认证映射与 Secret 注入设计

- 所属治理任务：GOV-004
- 状态：目标认证边界设计，待 Data / Server / Platform 对齐；不代表已实现统一服务身份系统。
- 规划归属：`feat/design_doc`
- 相关实现分支：Data `feat/full-go-data-providers-api-switch`、Platform `feat/platform-foundation`、Server 集成分支。
- 核查文件：`services/market-data/openapi.yaml`（本轮读取 blob SHA：`9a15f7810a07c60887a574d82e1f84d56a8c9964`）。

## 1. 当前 Data API 认证事实

当前 OpenAPI 声明：
- `ApiKey`：Header `X-API-Key`；
- `BearerAuth`：HTTP Bearer；
- 全局 Security 声明 ApiKey 与 BearerAuth 两种方案（OpenAPI OR 语义）；
- `/health/live`、`/health/ready` 显式 `security: []`；
- 业务 API Operation 显式声明 `ApiKey`。

因此，业务 Data API 的实际契约应按 Operation 级 `X-API-Key` 处理，不能仅因 OpenAPI 全局声明还出现 BearerAuth，就认为 Data API 已经接受 Platform Service Secret/Bearer Token。

Data API 的 `X-API-Key`、根 Server owner Token、Platform service Secret、各 Provider Token 是四种不同凭据：
- Data API Key：允许根 Server 的 Data Adapter 调用行情服务；
- Owner Token：保护根 Server 的管理/用户级接口；
- Platform Service Secret：平台内部服务身份凭据，若启用，应有自己的签发、校验和轮换语义；
- Provider Token：Data 服务访问上游供应商的凭据。

不得把这些凭据混用、复用同一值或在日志/错误/诊断中回显。

## 2. 首期推荐方案

**保持 Data 当前 `X-API-Key` API 契约；由根 Server 在服务端注入 Data 专用 Secret，Data Adapter 统一添加 Header。**

请求链路：

```text
Browser / WebUI
    │ 仅访问 Root Server API，不持有 Data Key
    ▼
Root Server Handler
    │ 业务授权 / Request ID / Fault
    ▼
Quant Data Adapter
    │ 从 Server Config 读取 Data 专用 Secret
    │ 设置 X-API-Key
    ▼
Market Data Service (独立 Go Module)
    │ 校验 X-API-Key
    ▼
Data Provider Adapter
    │ Provider Token 只在 Data 服务内部
    ▼
Upstream Providers
```

边界：
- WebUI/浏览器永远不拿到 Data API Key、Platform Service Secret 或 Provider Token。
- 根 Server 只在 Data Adapter 构造认证请求时读取 Data 专用 Secret；Handler、Domain、Use Case 不直接读取环境变量。
- Data Adapter 是根 Server 中唯一知道 Data API Key Header 的业务基础设施适配器。
- Data 服务独立管理自己的 API Key 校验、有效性和轮换，不 import 根模块 Platform internal。
- Data Provider Token 只由 Data 服务注入和使用，不透传到根 Server。
- 根 Server owner Token 不作为 Data API Key 发送；外部用户身份不能变成服务间凭据。

## 3. 为什么不直接把 Platform Service Secret 当作 Data X-API-Key

两者的安全语义不等价：
- Platform Service Secret 代表调用方服务身份；可能有调用方、受众、权限、有效期、轮换和撤销语义。
- Data X-API-Key 当前是 Data 服务自身的 API 凭据；OpenAPI 只定义 Header，不表达 issuer、audience、scope、expiry 或调用方身份。
- 把同一个字符串同时当两种凭据，会造成凭据边界混淆，也无法证明 Data 端真正校验了 Platform 服务身份。
- 让 Data 独立 Go Module import 根模块 Platform internal 会破坏 Module 边界。

因此首期不强行做凭据复用或统一身份改造。Data API Key 作为独立服务凭据，Platform Service Secret 若存在则继续用于其明确支持的服务身份场景。

## 4. Secret 配置与装配约束

建议根 Server Config 持有一个 Data 专用 Secret 字段，配置键名在 Server 配置文档/部署配置中统一冻结（例如采用 `MARKET_DATA_API_KEY` 这一类明确归属的名称；在实际确认前不要求代码照抄示例）。

约束：
1. 启动时读取并校验必需 Secret；Data Adapter 未配置凭据时，启动或 readiness 按 Data 能力是否必需明确失败/降级，不能每次请求才静默发现。
2. 生产 Secret 由部署环境或 Secret Manager 注入；dotenv 仅本地开发辅助。
3. Secret 作为类型化 Secret 传递，不使用普通 string 在业务层广泛传播。
4. 仅在构造 Data Adapter 的认证边界调用 Secret 明文读取方法。
5. 不将 Secret 写入日志、Fault.SafeMessage、诊断 attributes、Request ID、任务参数、数据库业务记录或 API 响应。
6. 禁止在 WebUI、浏览器 LocalStorage、URL Query、前端配置接口或 LLM Prompt 中出现该 Secret。
7. 禁止将 Secret 作为 Data Adapter 的普通可序列化配置字段暴露给 JSON。
8. 测试使用专用临时凭据或 Mock Transport，不读取生产 Secret。

## 5. Header 与网络边界

- Data Adapter 对受控 Data Service Base URL 固定设置 `X-API-Key`。
- 认证 Header 不允许由用户请求参数、任意 Header 转发或 LLM Tool 参数覆盖。
- 目标 URL 固定来自 Server Config，并使用 Platform HTTP Client 的固定 DestinationPolicy；禁止任意 URL SSRF。
- Data 服务仅允许预期的内网/受控调用来源访问业务端口；网络 ACL 是额外防线，不替代 API Key 校验。
- 健康接口按当前 OpenAPI 明确为无认证接口，只返回最小存活/就绪信息，不包含 Provider Key、Data Key、连接串或详细配置。
- 代理/网关不得把外部客户端提供的 `X-API-Key` 原样透传到 Data；必须由 Data Adapter 覆盖为服务端 Secret。
- Request ID 可以跨 Server → Data 传播，但它不是认证信息，也不能作为授权依据。

## 6. Key 轮换与多环境

首期至少区分 dev/test/staging/prod 的不同 Key：
- 不跨环境复用同一 Key。
- 轮换应支持短暂双 Key 窗口或明确的协调切换；若 Data 当前只支持单 Key，则需要维护窗口并先部署新 Key，再切换 Server。
- 日志只记录认证失败的稳定 Code，不记录 Key 值、Header 原文或 Key 哈希全值。
- 认证失败与网络失败、Provider 失败分开统计；不能把 401/403 记为 Provider 不可用。
- Key 撤销后，Data API 应拒绝旧 Key；Server readiness/诊断应能区分认证配置错误与 Data 网络不可达。
- Secret 轮换、撤销、权限变化要有操作记录，但不能记录 Secret 明文。

## 7. 与 Platform Security 的未来衔接

首期保持 Data X-API-Key 不变。若后续需要统一服务身份，必须先设计跨 Go Module 的稳定协议，而不是共享 Go internal 包：
- 由可信服务身份系统签发短期凭据或受众明确的服务令牌；
- Token 至少表达 issuer、audience（Data Service）、subject/caller、scope、issued_at、expires_at 和 key/version ID；
- Data 服务在自己的独立 Module 中实现校验 Adapter；
- 根 Server 通过公开配置/HTTP 协议调用，不依赖 Platform 内部 Go 类型；
- 评估是否以 mTLS / Gateway 身份断言替代长期 API Key；
- 迁移需有双凭据窗口、撤销策略、审计和回滚方案。

在没有真实多服务身份治理需求前，不为“统一”而增加复杂度，也不提前更改 Data API 认证契约。

## 8. 验收清单

- [ ] Data 业务 API 实际只接受已声明的服务端凭据；OpenAPI Operation Security 与 Handler Middleware 一致。
- [ ] Root Server Data Adapter 服务端注入 Data 专用 Key。
- [ ] WebUI/浏览器无法获得 Data Key、Platform Service Secret 或 Provider Token。
- [ ] 用户请求 Header 无法覆盖 Data Adapter 认证 Header。
- [ ] Data Service Base URL 固定且受 DestinationPolicy 限制。
- [ ] 401/403、网络不可达、Provider 错误可区分。
- [ ] 健康接口无认证且只暴露最小信息。
- [ ] Secret 不出现在日志、Fault、诊断、任务参数、响应或 LLM 上下文。
- [ ] dev/test/staging/prod 使用不同 Key，轮换/撤销流程通过演练。
- [ ] Data 独立 Go Module 不 import 根模块 Platform internal。
- [ ] Platform Service Secret 不被误当作 Data X-API-Key；未来统一身份改造单独评审。



## DATA-CTR-008 与 Data API 凭据的边界

- Data API Key 只用于认证；通过认证后由服务端映射到稳定 caller principal 和 Scope。Key 字符串本身不得写入 AuthorizationRef、Snapshot Manifest 或 fingerprint input。
- caller principal 是服务端登记的稳定身份引用；同一 principal 的 Key 轮换不改变 subject_ref。Key 撤销、principal 禁用或 Scope 收窄必须在下一次读取生效。
- Data v2 请求的 purpose_code 必填且受 Scope 限制；客户端不得提交 subject_ref/caller_ref。认证失败返回 401；认证成功但 Scope 不足返回 403；授权依赖不可用返回 503。不得把这些状态归类为 Provider 故障。
- `schema/DATA-CTR-008-snapshot-manifest-1.0.0.schema.json` 定义授权引用绑定的最小 Snapshot Manifest。Manifest 只包含查询范围和来源版本元数据，不包含数据正文或任何 Secret。
- Snapshot Manifest 的 `manifest_fingerprint` 对除 `snapshot_ref`、`manifest_fingerprint` 外的字段执行 RFC 8785 JCS + SHA-256；`snapshot_ref` 必须为 `data_snapshot:sha256:<manifest_fingerprint>`。同一规范化 Manifest 必须得到同一引用，任何绑定字段变化必须得到不同引用。
- GOV-CTR-006 的 `decision_fingerprint` 仍严格遵循公共算法；通过 resource_ref 指向内容寻址 snapshot_ref，避免为 Data 单独扩展或改变公共算法。
- 授权检查与 Snapshot Manifest/数据读取必须处于同一可证明的策略版本边界；检查后策略版本变化时重新授权，无法证明一致性则丢弃结果并 fail closed。

## 9. 当前结论

首期最优边界是：**Root Server 服务端持有 Data 专用 API Key，Data Adapter 在内部 HTTP 请求中注入 `X-API-Key`；浏览器只调用 Root Server，不接触 Data Key。** Platform Service Secret、Owner Token 和 Provider Token 保持独立。

该方案不需要为兼容旧服务增加复杂适配，也不要求 Data 立刻切换认证协议。实际环境仍需完成 Key 轮换、网络 ACL、服务端 Header 注入和安全日志验收。
