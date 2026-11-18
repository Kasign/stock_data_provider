# LLM → Server 生产装配依赖

## 1. 职责分工

LLM 提供 `Provider`、`ResponsesAdapter`、`ProviderConfig`、`CapabilityState` 和窄传输接口。Server 是组合根，负责将这些能力装配成可运行 Provider。

## 2. Server 必须提供

1. 真实 Gateway 的部署版本、实际模型 ID 和逐项能力验证结果。
2. 与 Provider/Model/Gateway 版本绑定的 CapabilityState：`verified_at`、`valid_until`、`verification_method`、`evidence_ref` 和能力状态。
3. 解析 `api_key_secret_ref` 并注入 `platform/config.Secret`；不得混用 Web Session、Admin Key、上游 Provider Key。
4. 通过共享 Platform HTTP Client 创建固定 Gateway DestinationPolicy/BoundDoer；HTTPS、Host/Port、禁止重定向、禁止传输层自动重试。
5. 将 BoundDoer 的 OpenStream 返回值薄包装为 LLM HTTPStreamResponse，保留原始错误和 Body 所有权。
6. 构造 ResponsesAdapter 并注入 Researchflow 消费入口。
7. 生命周期关闭时，先停止并等待 LLM 消费者，再关闭共享 Platform Client。

## 3. 当前代码与接入状态

- `internal/server/llmtransport/transport.go` 已提供 Platform BoundDoer 到 LLM HTTPDoer/HTTPStreamDoer 的薄适配，以及 `NewResponsesAdapter` 构造辅助函数。
- 在当前分支代码核对中，尚未找到该构造辅助函数被 Server Bootstrap 调用并注入 Researchflow 的生产装配调用点。
- `doc/server/模块总任务规划和进度.md` 将 SRV-010「LLM Provider 生产装配」列为 BLOCKED；`doc/server/当前任务状态和进度.md` 同样记录真实 Gateway 版本/能力快照和生产 Provider 装配未闭环。
- 因此，静态传输适配存在不等于生产 Provider 已接入；本模块不越权修改 `internal/server/**` 或 `doc/server/**`。

## 4. 阻塞解除条件

Server 提供真实 Gateway 版本与能力 Evidence、可用 SecretRef、固定 BoundDoer，并完成 Bootstrap/Researchflow 注入和关闭顺序后，LLM 配合执行真实 Generate/SSE/取消/超时/超限联调。

## 5. 验收状态

当前：BLOCKED（Server/部署环境责任）。

- PASS：Server 真实构造 Provider，固定目标策略生效，Secret 与能力证据正确注入，Researchflow 调用和生命周期可追踪。
- DEGRADED：仅部分模型/能力通过真实验证，未通过能力明确禁用。
- FAIL：凭据混用、目标可被请求覆盖、无证据启用能力、Body 未释放或消费者关闭顺序错误。
- NOT_COMPARABLE：只有静态接口或 Mock，未执行真实 Gateway 请求。
