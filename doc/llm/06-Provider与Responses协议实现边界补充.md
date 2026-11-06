# LLM Provider 与 Responses 协议实现边界补充

- 适用任务：LLM-002；补充 LLM-001 的可复现验收
- 契约范围：LLM-CTR-001～003、PLAT-CTR-001/005、GOV-CTR-002/003/005
- 状态：七项协议语义已接受；本文件补充实现边界，不代表 Schema/Golden/Contract Test 或真实模型能力已通过
- 权威关系：04-ProviderPort与DTO字段级契约.md 第 10 节为已接受协议决议；项目级契约目录为公共编号和状态权威

## 1. 依赖与协议隔离

LLM 只依赖 Platform 公共技术原语。首批协议固定 OpenAI Responses；wire DTO、HTTP Header 和错误体只存在 Adapter/Protocol，不得泄漏到 Application/Domain DTO。LLM 不依赖 Quant、Research、Researchflow 或 Notification 的领域类型。

Researchflow 决定调用时机、调用预算和业务重试；LLM 负责一次受控模型调用、协议解析、资源限制和安全错误投影。LLM 不执行 Tool，不根据模型输出调用业务模块。

## 2. ProviderConfig 与 Secret

ProviderConfig 是服务端受控配置，不是调用方请求参数。首批固定 provider_id、protocol=openai_responses、base_url、default_model、allowed_models、api_key_secret_ref、request_timeout_ms、max_request_bytes、max_response_bytes、stream_enabled 与 Capability 有效期策略。

base_url 必须是服务端配置的 HTTPS Gateway 根地址并规范化为 /v1 结尾；请求方不得覆盖 URL、Provider、Header 或 Secret。Adapter 固定 POST {base_url}/responses，不得重复拼接 /v1。API Key 只通过 SecretRef 在调用时读取，禁止写入 DTO、日志、错误、Fixture 或持久化请求快照。

request_timeout_ms、max_request_bytes、max_response_bytes 必须为正数并显式配置；max_response_bytes 首批不超过 8 MiB；配置序列化时间单位固定为 integer milliseconds，不得把 Go duration 纳秒整数直接暴露为 JSON。所有限制不得超过 PLAT-CTR-005 公共上限。HTTP 重定向、代理、DNS/SSRF、TLS 和连接资源策略交给 Platform HTTP Client。

## 3. ModelRequest / ModelResponse

ModelRequest 是 LLM 自己的应用 DTO，不允许传入任意 URL/Header/Gateway SDK 类型。MVP 单轮文本输入：
- request_id：调用方生成的单次调用 ID；Response 原样回显，不以 Gateway ID 替换。
- correlation_id：从 Researchflow Run Context 继承。
- provider_id/model：必须命中服务端 ProviderConfig；model 必须属于 allowed_models。
- purpose_code：必填稳定用途代码，不使用自由文本表达调用目的。
- instructions/input_text：typed 文本字段；instructions 可为空，input_text 必须非空。
- response_schema：可选 JSONSchemaSpec；发送前完成本地编译、dialect、大小和深度校验。
- local_validation_only：可选 bool；true 时不要求上游 structured output 能力，只保证本地输出校验，不代表模型受约束生成。
- max_output_tokens：正整数且不超过当前 Model Capability 已验证上限。
- temperature/top_p/reasoning_effort：只有对应 CapabilityState 当前有效且 supported 才允许发送，否则明确拒绝。
- tools/tool_choice：默认关闭；仅 function_tool_calls=supported 且 Tool 定义通过本地 Schema 校验时允许受控 auto。LLM 只解析，不批准或执行。
- deadline_at：必填绝对 UTC 截止时间，不得晚于调用方 Context deadline。
- system_as_of：必填 RFC3339Nano UTC，必须不晚于可信服务端当前时间；原样回显，仅用于审计/上下文追溯，不注入 Prompt 或 Responses wire 字段。
- trace_id：可选受控追踪标识。

ModelResponse 区分 response_id、request_id、correlation_id、system_as_of、gateway_request_id、upstream_request_id；request_id、correlation_id 和 system_as_of 必须分别原样回显 ModelRequest 对应值。状态为 completed/incomplete/failed；只有合法 completed 才是成功模型结果。Output item/content part 保留类型和顺序；refusal 不合并为普通文本。未知结构只保留有界安全元数据。

Usage 缺失字段保持 null/unknown，不补 0。上游 total_tokens 优先；仅 input_tokens/output_tokens 均明确且加法无溢出时才可派生，并标记 source=derived。上游 total 与分项不一致时保留上游值并产生诊断。

## 4. CapabilityState

能力键固定为 (provider_id, model_id, protocol, gateway_version, capability_key)，至少记录 status、verified_at、验证方式和 evidence_ref。状态为 supported/unsupported/unknown。只有当前版本证据有效的 supported 才可发送可选能力；未验证、证据过期或版本变化一律 unknown。responses_stream、structured_output_json_schema、function_tool_calls、reasoning_effort 分别验证，不得互相推断。stream_enabled=true 必须绑定当前版本真实 JSON/SSE E2E 证据。

## 5. Structured Output

本地校验 dialect 固定 Draft 2020-12，必须执行 format assertion、深度限制、请求/响应字节限制和 Schema 编译检查。上游 Responses strict 子集兼容性是独立 Capability。不支持的关键字不得静默删除；调用方显式选择 local_validation_only 时，只保证本地输出校验，不代表模型受约束生成；不满足本地 Schema 时返回 schema_validation_failed，不回退普通文本成功。

## 6. SSE 与流终态

- SSE 保留事件顺序、事件边界和有界解析结果；未知事件仅保留 type、sequence_number（若有）及安全 allowlist 元数据。
- sequence_number 全流提供时必须严格连续递增；全流缺失可继续但标记 unavailable；同流混合有无序号、缺号、重复或倒退均不得 completed。
- 必须收到 terminal event；只有 response.completed 是成功，response.incomplete/failed/error 是非成功终态。EOF 无 terminal 返回 stream_interrupted。
- terminal 后出现额外完整事件视为 invalid_response；不能追加到已完成响应。流读取、callback 和累计字节受调用 deadline/资源上限约束。
- 首个事件发出后禁止透明重试，避免重复计费和重复输出。

## 7. 错误与重试

合法上游 response.incomplete/response.failed 是 ModelResponse 状态，不自动转成 Go error；传输失败、HTTP 非成功、协议畸形、本地 Schema 校验失败、callback/context 错误使用 Platform Fault 并映射为 ModelErrorInfo。

LLM Adapter 默认不自动重试。Researchflow 是首批业务重试决策者；429/503 的 Retry-After 只是提示，必须服从 Run deadline、调用预算和幂等边界。请求可能已被上游接收但响应丢失时不得假定未执行；仅能证明请求未派发时才允许上层重新发起。Stream 已发出任意事件后不得透明重试。

## 8. Request ID / 诊断 / 安全

request_id 是调用方单次模型调用身份；correlation_id 是 Run 链路关联；gateway_request_id 是 Gateway X-Request-ID；upstream_request_id 是 Gateway 透传的上游 Request-Id。四者不可覆盖。Platform Fault 的 dependency/cause 不进入默认公开 safe_message。默认不持久化完整 Prompt、Response、Tool 参数和原始 SSE；Secret、Authorization Header、敏感正文不得进入日志或错误响应。

## 9. 最小自动化验收

ProviderConfig/SecretRef/URL/超时和字节上限；DTO required/nullable 和 ID；JSON request/response/Usage；Draft 2020-12 和 strict 子集；SSE terminal/EOF/sequence/callback/backpressure；错误映射；能力状态有效期；无透明重试；Secret 脱敏。

JSON/SSE Fixture 全通过只证明 Adapter 协议解析达到模块级要求，不代表本地 llama.cpp/Qwen 的真实能力 supported。真实 Gateway E2E、响应头 ID 映射和模型能力测试单独记录 L3。
