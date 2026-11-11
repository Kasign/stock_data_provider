# PLAT-CTR-002 Request Context 与身份关联契约

- 契约编号：PLAT-CTR-002
- 契约版本：1.0.0
- 状态：freeze-ready（待项目负责人确认冻结）
- 代码审计基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`
- 生产方：Platform（Request ID 与 Principal Context 原语）
- 直接消费者：Root Server、服务端 HTTP/出站 Adapter；Worker 只消费领域持久化的关联标识，不继承 HTTP Request Context
- 权威性：本文件定义同步请求的可信上下文与 Request ID 传播；身份类型/Scope 规则见 PLAT-CTR-003；持久化 ID 和幂等规则见 GOV-CTR-003。

## 1. 范围与边界

PLAT-CTR-002 定义一次同步 HTTP 请求在服务端调用树中的可信关联上下文，包含：

1. Request ID：标识一次入站 HTTP 请求及其同步派生调用。
2. Principal Context：由服务端认证/路由授权组件建立的可信调用主体。

二者是独立的 Context 值，不合并为可序列化的通用 RequestContext DTO。PLAT-CTR-002 不定义领域资源授权、业务身份快照、异步任务身份、事件身份或幂等键。

- Principal 的 Kind、Subject、Scopes、认证来源和 RoutePolicy 语义由 PLAT-CTR-003 定义。
- originating_request_id、correlation_id、event_id、idempotency_key 的生成方、作用域和持久化语义由 GOV-CTR-003 及领域 CTR 定义。
- HTTP Error Envelope 中可选的 body request_id 与响应头关系由 PLAT-CTR-001 定义。

## 2. Request ID 格式

- HTTP Header 名称固定为 `X-Request-ID`。
- 有效值长度为 1～128 个 ASCII 字符，只允许字母、数字、`-`、`_`、`.`、`:`。
- Request ID 是不透明关联标识；消费者不得从其内容推导身份、权限、时间、顺序或幂等性。
- 默认生成方式使用 16 字节密码学随机数并编码为 32 位小写十六进制。
- 随机源不可用时允许实现使用进程内安全回退生成器；消费者不得依赖生成值的具体格式，只依赖上述有效字符范围及单次请求关联语义。

## 3. 入站可信边界

- 新接入的 HTTP 边界必须使用 `MiddlewareWithOptions`。
- 零值 `MiddlewareOptions{}` 不信任客户端提供的 `X-Request-ID`，始终生成新的 Request ID。
- 只有在可信 Gateway 已覆盖外部 Header、且服务端网络入口无法绕过 Gateway 时，才允许显式设置 `TrustIncomingRequestID: true`。
- 开启信任时，只接受一个合法 Header 值；缺失、非法或重复值均生成新的 Request ID。
- 历史 `Middleware` 保留为 Deprecated 兼容入口，不得用于新服务或可被不可信网络直接访问的入口；只有满足可信 Gateway 前提的历史部署才可暂时使用。
- 最终 Request ID 必须写入响应 `X-Request-ID` Header，并存入该请求的 Context。
- Request ID 不承担认证、授权、访问控制或去重功能。

## 4. Context API 与 Principal

当前等价权威 Go API 为：

- `observability.RequestIDHeader`：固定 Header 常量。
- `observability.MiddlewareWithOptions(next, options)`：校验/生成 Request ID、写响应 Header、将值放入请求 Context。
- `observability.FromContext(ctx)`：读取当前 Context 中已验证的 Request ID；不存在时返回空字符串。
- `observability.WithRequestID(ctx, requestID)`：仅在输入值符合格式时建立带 Request ID 的子 Context；无效值不改变 Context。
- `observability.PropagateRequestID(ctx, request)`：只从可信 Context 覆盖出站 Header；Context 无 Request ID 时删除请求对象中已有的 `X-Request-ID`。
- `security.Principal{Kind, Subject, Scopes}`：进程内调用主体快照。
- `security.PrincipalFromContext(request)`：读取由 Platform Guard 写入的 Principal，并返回 Scope 切片副本。

Principal 只能由服务端认证/授权链写入 Context。客户端自报的 Principal、Subject、Scope 或身份 Header 不构成可信身份。Principal 的具体身份种类、凭证和 Scope allowlist 由 PLAT-CTR-003 管理。

`context.Context` 是同步调用树的运行时载体，不是持久化 DTO。下游同步调用必须沿用请求 Context 或其派生 Context，以保留 Request ID、Principal 和取消/Deadline 信号；不得通过新建 Background Context 静默丢失调用取消边界。

## 5. 出站与异步边界

- Platform HTTP Client 的缓冲和流式请求均从当前可信 Context 传播 Request ID。
- 出站传播必须覆盖调用方预设的 `X-Request-ID`；没有可信 Context 值时必须删除预设值，防止不可信 Header 泄漏。
- Request ID 传播不代表认证凭证传播。Authorization、Cookie、Service Secret 和 Provider Credential 不得由 Platform 自动转发；出站调用方必须按目标依赖显式注入自身凭证及 DestinationPolicy。
- 不得把入站 `context.Context`、Principal、Bearer Token 或 Session Token 持久化到任务、数据库、事件或队列。
- Worker/异步任务使用新的执行 Context；如需保留来源关联，按 GOV-CTR-003 和对应领域契约保存 originating_request_id、correlation_id 及必要的领域授权引用。不得将来源 Request ID 冒充 Worker 当前请求 ID，也不得把历史 Principal 当作当前授权结论。

## 6. Schema、Golden 与验收适用性

本契约由 HTTP Header 规则和进程内 Go Context API 构成，不存在独立跨网络 JSON DTO；因此不适用 JSON Schema/OpenAPI，也不应为 Principal Context 创建 JSON Golden。

可核对的等价机器定义与证据：

- Go API：`internal/platform/observability/request_id.go`、`internal/platform/security/security.go`。
- Request ID 行为 Golden：`doc/platform/golden/PLAT-CTR-002-request-id.golden.json`。
- 对应 Contract Test：`internal/platform/observability/request_id_contract_test.go`。
- 实际消费者与接线证据由 Server/领域实现符合性验收另行跟踪，不属于本轮定义冻结门禁。

## 7. 版本与兼容

本契约首版为 1.0.0。Header 名称/有效格式、入站信任默认值、Principal Context 的可信来源、出站覆盖/删除规则、同步与异步上下文边界发生不兼容变化时，按 GOV-CTR-004 升级版本。增加新的认证来源或 Scope 不得通过改变 PLAT-CTR-002 隐式完成，必须按 PLAT-CTR-003 评审。
