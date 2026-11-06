# LLM 首批 Provider Port 与 DTO 字段级契约

- 所属任务：LLM-002
- 前置调查：LLM-001 静态协议盘点
- 首批协议：OpenAI Responses API
- 日期：2026-11-05
- 状态：LLM 字段级契约输入；跨模块编号、语义状态和阻断范围以 `doc/整体任务规划/跨模块契约目录与冻结状态.md` 为准；不代表实现、真实端点验收或接口冻结。

## 1. 目标与边界

- 首批只实现 Go 侧 Responses Provider Port/Adapter，复用现有 Gateway；不建设第二个推理 Gateway，不做复杂多 Provider 路由。
- LLM 不依赖 Researchflow、Quant、Research 的领域类型；OpenAI JSON DTO、SSE wire event 和 HTTP Header 仅存在 Adapter/Protocol 层。
- 调用方通过 LLM 自己的 DTO 表达模型请求；不得直接传入任意 HTTP Header、URL 或 Gateway SDK 类型。
- Tool Call 只作为待校验意图，不在 LLM 模块执行；权限、白名单、预算和执行均归 Researchflow。
- Structured Output 必须本地 Schema 校验；Gateway text.format 透传不等于模型能力通过。
- Usage 缺失保留 unknown/null；不得补 0。

## 2. ProviderConfig 与协议

| 字段 | 类型 | 必需 | 约束 |
|---|---|---:|---|
| provider_id | opaque string | 是 | 稳定配置 ID |
| protocol | enum | 是 | 首批固定 openai_responses |
| base_url | URL | 是 | HTTPS Gateway 根地址，规范化后以 /v1 结尾；请求方不可覆盖 |
| default_model | string | 是 | Gateway 可识别的模型 ID |
| api_key_secret_ref | opaque string | 是 | Secret 引用，不存明文 Key |
| request_timeout | duration | 是 | 正数总时限，并受调用方 deadline 约束 |
| max_request_bytes | integer | 是 | LLM 自身更小的请求上限 |
| max_response_bytes | integer | 是 | 非流式响应上限 |
| stream_enabled | bool | 是 | 当前部署本地上游 E2E 通过后才可设为 true |

- 首批 endpoint 固定为 POST {base_url}/responses；base_url 已含 /v1，Adapter 不得重复拼接版本段。
- Header 固定 Content-Type: application/json；认证使用 Authorization: Bearer，从 SecretRef 读取调用方 Gateway Key。
- 不透传任意调用方 Header；仅允许受控 Request/Correlation ID 和经审核的协议 Header。
- 不把 Gateway 管理会话、X-Admin-Key 或上游 Provider Key 作为 LLM Adapter 凭据。
- /v1/models 可用于模型清单，但不能据此推断 Structured Output、Tool Calling 或 Streaming 一定可用。

## 3. CapabilityState

使用 supported / unsupported / unknown 三态，默认 unknown。

首批能力键：responses_json、responses_stream、structured_output_json_schema、function_tool_calls、reasoning_effort、usage_details。

能力状态必须绑定 provider_id、model_id、Gateway 版本和验证时间。Gateway 升级或模型更换后，历史结果不能自动沿用为 supported。路由存在只证明协议路径可达，不证明上游模型完整支持该能力。

## 4. ModelRequest

| 字段 | 类型 | 必需 | 约束 |
|---|---|---:|---|
| request_id | string | 是 | 由调用方生成的本次 LLM 调用 Request ID；Response 原样回显，不承载 Secret |
| purpose_code | string | 是 | 稳定调用用途代码，不写自由文本 Prompt |
| model | string | 是 | 明确模型 ID，不依赖任意模型路由 |
| instructions | string | 是 | 序列化到 Responses instructions；允许空字符串 |
| input_text | string | 是 | 首批只支持非空单轮文本，序列化为 Responses input string |
| max_output_tokens | integer | 是 | 正整数，受调用预算限制 |
| temperature | *number | 否 | 仅 capability=supported 时发送 |
| top_p | *number | 否 | 仅 capability=supported 时发送 |
| reasoning_effort | enum | 否 | low/medium/high；默认不发送，仅 capability=supported 时允许 |
| response_schema | JSONSchemaSpec? | 否 | 发送前做本地语法、大小和深度校验 |
| tools | ToolDefinition[] | 否 | 只接受调用方授权后的定义；不执行 |
| tool_choice | enum? | 否 | 默认 none；仅 tools 非空且 function_tool_calls=supported 时允许 auto；required/指定工具延后 |
| deadline_at | UTC timestamp | 是 | 绝对截止时间，不得晚于调用方 Context deadline |
| correlation_id | string | 否 | 跨模块关联 ID，不承载领域对象 |
| system_as_of | UTC timestamp | 是 | Researchflow Run 的知识截止值；仅用于调用审计/上下文追溯，不自动注入 Prompt 或转发为 Responses wire 字段 |
| trace_id | string | 否 | 受控追踪标识 |

首批不接收任意 map、任意 JSON 或完整 Responses input item 数组。多轮对话、多模态、background response、previous_response_id、conversation_id 等 deferred；出现时返回 unsupported_capability，不得静默丢弃。

### 4.1 JSONSchemaSpec

| 字段 | 类型 | 必需 | 约束 |
|---|---|---:|---|
| name | string | 是 | 稳定 Schema 名称 |
| schema_json | json.RawMessage | 是 | 合法 JSON Schema Draft 2020-12，限制字节数和深度 |
| strict | bool | 是 | 显式指定，不依赖上游默认值；上游 strict 子集另行检查 |

Adapter 序列化到 Responses text.format 的 type=json_schema/name/strict/schema。Schema 编译失败属于本地 invalid_request；响应后必须再次校验 JSON。

### 4.2 ToolDefinition

| 字段 | 类型 | 必需 | 约束 |
|---|---|---:|---|
| name | string | 是 | 注册表稳定名称，不允许任意命令/路径 |
| description | string | 否 | 长度受限 |
| parameters_json | json.RawMessage | 是 | 参数 JSON Schema |
| strict | bool | 是 | 明确约束参数格式 |

ToolDefinition 是 Researchflow Registry 筛选后的只读声明。LLM 只序列化和解析，不判断权限、不执行、不回填工具结果。能力 unknown 时不得发送。

### 4.3 参数约束

- Responses 使用 max_output_tokens；不得把 Chat Completions max_tokens 当成同义字段静默映射。
- temperature/top_p/reasoning_effort 都是可选字段；仅在当前 model capability=supported 时允许发送。
- 不硬编码某个 Qwen 模型的默认采样参数；由 ProviderConfig/调用方显式传入。
- Gateway 本地推理参数面板可能合并默认值；实际参数以 Gateway 最终发出的请求审计为准。

## 5. ModelResponse

| 字段 | 类型 | 必需 | 约束 |
|---|---|---:|---|
| request_id | string | 是 | 回显 ModelRequest.request_id |
| system_as_of | UTC timestamp | 是 | 原样回显 ModelRequest.system_as_of，不由 LLM Adapter 刷新 |
| provider_id | string | 是 | 实际选用的 Provider 配置 ID |
| response_id | string | 条件必需 | 上游 Responses id；缺失需标记契约异常 |
| status | enum | 是 | completed / incomplete / failed |
| output_items | OutputItem[] | 是 | 保留 item 类型和顺序；空数组必须有状态/错误解释 |
| output_text | string | 否 | 按 output item 顺序聚合 message/output_text part 的便捷字段 |
| structured_json | json.RawMessage? | 否 | 仅本地 Schema 校验成功后提供 |
| tool_calls | ModelToolCall[] | 否 | 仅解析，不执行 |
| usage | Usage? | 否 | 缺失为 nil，不伪造 0 |
| model_requested | string | 是 | 请求模型 ID |
| model_actual | string? | 否 | 上游响应 model 或 Gateway 实际模型 |
| model_version | string? | 否 | 上游明确返回才设置 |
| gateway_request_id | string? | 否 | Gateway 生成的 X-Request-ID，缺失允许 |
| upstream_request_id | string? | 否 | Gateway 透传/回传的上游 Request-Id；与 gateway_request_id 分开保存 |
| incomplete_reason | string? | 否 | Responses incomplete_details.reason |
| error | ModelErrorInfo? | 否 | 稳定错误码和安全摘要 |
| started_at / finished_at | UTC timestamp | 是 | 调用时间边界 |
| duration_ms | integer | 是 | 非负 |

OutputItem 首批类型：message、function_call、refusal、unknown。ContentPart 首批支持 output_text/refusal；未知 item 保留类型和安全元数据，不丢弃，也不伪装成空文本。

### 5.3 OutputItem / ContentPart / ModelToolCall

| 对象 | 字段 | 类型 | 约束 |
|---|---|---|---|
| OutputItem | type | enum | message / function_call / refusal / unknown |
| OutputItem | item_id | string? | 上游 item id，缺失允许 |
| OutputItem | status | string? | 原样保留上游状态；不映射成成功/失败的唯一依据 |
| OutputItem | content_parts | ContentPart[] | message 的有序内容片段 |
| OutputItem | tool_call | ModelToolCall? | 仅 function_call 类型可设置 |
| ContentPart | type | enum | output_text / refusal / unknown |
| ContentPart | text | string? | output_text 文本 |
| ContentPart | annotations | Annotation[]? | 仅保留已识别且安全的引用元数据 |
| ModelToolCall | call_id | string | 上游调用 ID；缺失时标记协议异常 |
| ModelToolCall | output_item_id | string? | 关联的 OutputItem ID |
| ModelToolCall | name | string | 工具名；不直接执行 |
| ModelToolCall | arguments_json | string | 原始参数 JSON 文本，解析前保留原值 |
| ModelToolCall | status | string? | 上游 function_call 状态 |

未知 OutputItem/ContentPart 必须保留 type 与有界安全元数据；不能把未知内容丢弃后仍报告完整成功。ToolCall arguments_json 需在 Researchflow 的 Tool Registry/Schema 下重新验证。

### 5.4 条件必需字段

- status=completed：response_id 应存在；output_items 结构必须可解析。
- status=incomplete：保留 incomplete_reason；若上游给出 incomplete_details.reason 则必须透传。
- status=failed：error 必须存在，且只包含稳定错误码与安全摘要。
- response_id、model_actual、gateway_request_id 在上游未提供时可空；不得从 request_id 伪造 response_id。

### 5.1 Usage

| 字段 | 类型 | 语义 |
|---|---|---|
| input_tokens | *int64 | 上游明确返回才设置 |
| output_tokens | *int64 | 上游明确返回才设置 |
| total_tokens | *int64 | 优先使用上游值；缺失时仅在 input/output 都明确时允许派生，并标记 source=derived |
| input_cached_tokens | *int64 | 可选明细 |
| output_reasoning_tokens | *int64 | 可选明细 |
| source | enum | upstream / derived / unknown |

Usage 是计量信息，不是输出质量证明；缺失、0 和 unknown 必须区分。

### 5.2 完成状态

- completed：收到 response.completed 且响应结构有效。
- incomplete：收到 response.incomplete，保留 incomplete_details.reason。
- failed：收到 response.failed、协议错误或本地解析/校验错误。
- 非流式响应缺少 status 或 output 结构异常时，不自动判为 completed。
- Tool Call 输出不能等同于文本回答完成；必须作为 output item 独立返回。

## 6. Stream Port 与事件

建议接口：
- Generate(ctx, ModelRequest) -> (ModelResponse, error)
- Stream(ctx, ModelRequest, onEvent func(ModelEvent) error) -> (ModelResponse, error)

Stream 使用同步回调提供背压，并返回最终聚合的 ModelResponse。Generate/Stream 的 Go error 表示传输、HTTP 非成功、协议解析/校验、本地 Schema 校验、回调或 Context 取消等 Adapter 执行错误；上游合法返回的 status=incomplete/failed 通过 ModelResponse.status 与 ModelResponse.error 表达，不重复包装成 Go error。Stream 收到 terminal event 后返回最终 Response；无 terminal event 的 EOF 返回部分 Response 与 stream_interrupted 类型错误。回调错误或 Context 取消必须停止读取上游，并返回已解析的部分 Response（若有）及对应 error。

ModelEvent 字段：type、sequence_number?、response_id?、output_index?、content_index?、item_id?、call_id?、item?、content_part?、delta?、arguments?、usage?、error?、received_at。

首批识别 response.created、response.output_item.added/done、response.content_part.added/done、response.output_text.delta/done、response.refusal.delta/done、response.function_call_arguments.delta/done、response.completed、response.incomplete、response.failed、error。文本与函数参数增量均使用 delta 字段，但由 type 区分；item/content_part 字段承载已解析的结构对象。其他事件作为 UnknownEvent 保留类型和顺序，不执行副作用。上列是 Adapter parser 契约，不代表当前本地模型已产生所有事件。

流结束规则：
- 必须收到明确 terminal event（completed/incomplete/failed/error）才算终止；只有 completed 是成功，incomplete/failed/error 都必须按非成功结果返回。
- EOF 前没有 terminal event，返回 stream_interrupted；不得把连接关闭当成功。
- sequence_number 若全流提供，必须严格连续递增；重复/倒退返回 invalid_response，出现缺号返回 stream_sequence_gap，不得将该流标记为 completed。若整条流均未提供 sequence_number，可继续解析并记录该 Provider 未提供序号；同一条流中有的事件提供、有的事件缺失，按协议异常处理。
- 首字节、首个文本 delta、首个 reasoning delta 分别记录；Responses TTFT 不能用首字节时间冒充。
- 收到首个事件后禁止自动透明重试，避免重复计费和重复输出。

## 7. Error Contract 与重试

ModelErrorKind：invalid_request、unauthorized、rate_limited、overloaded、timeout、cancelled、upstream_unavailable、upstream_error、invalid_response、incomplete_response、schema_validation_failed、unsupported_capability、stream_interrupted、stream_sequence_gap、resource_limit。

ModelErrorInfo 字段为 kind、stable_code、retryable、http_status?、retry_after?、safe_message；调用关联 ID 统一由 ModelResponse.request_id 承载，不在嵌套错误对象重复；底层 Cause 仅内部日志，不序列化给调用方。

- 一次调用只能由一个层负责业务重试；首批建议由 Researchflow Run/Stage 策略决定，LLM Adapter 默认不自动重试。
- 429/503 可返回 retry_after 提示，是否重试由调用方 deadline/预算决定。
- timeout/cancelled 不自动重试。
- 请求可能已被上游接收但响应丢失时，不得假定未执行；仅确认未派发/连接建立前失败时才可考虑重试。
- Stream 已发出任意事件后禁止透明重试。

## 8. 安全与资源

- API Key 只通过 SecretRef 读取；不得进入 ModelRequest、DTO、错误摘要或日志。
- 不接受任意 Base URL、任意 Header 或请求方指定 Provider endpoint。
- 输入/Schema/Tool 数量和字节数设上限；非流式响应限制最大字节数；SSE 单事件/单行设上限，并限制累计事件数/累计字节数。
- 默认不保存完整 Prompt/Response 和原始 SSE；只记录 request_id、provider/model、参数指纹、耗时、Usage、状态、稳定错误码和 Gateway request ID。
- 若开启内容诊断，必须显式开关、脱敏、大小上限和保留期限。
- deadline 贯穿 DNS/TLS/请求/读取/Stream callback；Context cancel 必须中止网络读取。

## 9. 实现拆分与验收

| 子任务 | 内容 | 验收 |
|---|---|---|
| LLM-002-A | ProviderConfig / Protocol / CapabilityState | 配置校验、SecretRef 隔离、unknown 默认 |
| LLM-002-B | ModelRequest / ModelResponse DTO | required/optional/null、边界值和 JSON golden |
| LLM-002-C | Provider Port | Generate/Stream 签名、Context/deadline/callback 语义 |
| LLM-002-D | Responses JSON Adapter | request mapping、model、output item 解析 |
| LLM-002-E | Responses SSE Parser | event 顺序、usage、terminal、未知事件、断流 |
| LLM-002-F | Fixture/Contract Tests | JSON/SSE fixtures、错误映射、Schema 校验、凭据不入日志 |
| LLM-002-G | HTTP Adapter 接入 | Platform HTTP Client 单独验收；真实 Gateway 调用另列 Integration |

DTO、Port、Fixture 和 parser 单元测试不依赖 Researchflow/Platform 生产实现。真实 Gateway E2E 与当前本地模型能力验证是 L3，不反向阻断纯协议/领域设计。

## 10. 项目统一决议与待验证项

### 10.1 已接受的协议决议（历史 CONTRACT-008 候选）

1. **单轮文本输入：接受为 MVP。** ModelRequest 固定 instructions + input_text；input_text 非空。多轮消息、previous_response_id、conversation_id、图片/音频/视频和任意 input item 延后，不允许静默降级或丢弃。
2. **ToolDefinition：纳入 MVP 的受控可选字段。** 默认 tool_choice=none；仅当 tools 非空且 function_tool_calls=supported 时允许 auto。required、指定工具强制选择和并行工具策略延后。LLM 只解析 function_call，不执行、不批准。
3. **reasoning_effort：保留可选 typed 字段，默认不发送。** 只有 CapabilityState 对当前 Provider + Model + Gateway 版本明确为 supported 才能发送；当前本地 Qwen/llama.cpp 未验证，不得因为 Gateway 配置样本出现 high 就启用。
4. **JSON Schema：固定 Draft 2020-12 为本地 Schema 语义基线。** Adapter 本地完整校验输入 Schema 与模型输出；上游 Responses strict schema 子集兼容性单独判定，不支持的关键字不得静默移除。具体 Go validator 库可延后，但不得延后 dialect、格式校验和深度/字节限制。
5. **Usage.total_tokens：允许受控派生。** 上游值优先；缺失时仅当 input_tokens 与 output_tokens 都明确且加法无溢出时派生，source=derived。上游 total 与分项不一致时保留上游原值并产生诊断，不覆盖。
6. **SSE sequence_number：严格校验。** 若整条流均提供，必须连续递增；重复/倒退为 invalid_response，缺号为 stream_sequence_gap，均不得标记 completed。整条流都未提供可继续但标记序号能力 unavailable；同流混合有无序号按协议异常处理。
7. **Request ID：明确调用方/关联/Gateway/上游四层 ID。** ModelRequest.request_id 由调用方生成并由 ModelResponse 原样回显；correlation_id 用于跨模块关联；gateway_request_id 记录 Gateway 的 X-Request-ID，upstream_request_id 单独记录 Gateway 透传/回传的上游 Request-Id。四者不得相互覆盖或伪造。

### 10.2 Port 返回值语义

- Generate(ctx, request) 与 Stream(ctx, request, callback) 均返回 (ModelResponse, error)。Stream 在 terminal event 后返回最终聚合 Response。
- Go error 表示传输/HTTP 非成功、协议解析、Schema 校验、callback 或 Context 执行错误；若收到结构合法的 response.incomplete / response.failed，则以 Response.status 与 Response.error 表达，不把同一上游终态重复包装成 Go error。
- EOF 无 terminal event、sequence gap、重复/倒退序号均返回 typed error；可同时返回部分 Response 供诊断，但部分结果不得标记 completed。
- Tool Call 默认关闭；工具定义和工具调用能力都需显式 supported，调用结果交回 Researchflow。

### 10.3 尚待实现证据与 L3 验证

- 为七项已接受决议补齐 JSON Schema、JSON/SSE Golden、未知事件/序号异常/断流和错误映射 Contract Test。
- 选择并锁定 Draft 2020-12 validator 实现库与许可证审查；补齐格式校验、深度/大小限制和 golden fixtures。
- 以真实 Gateway 验证 X-Request-ID、上游 Request-Id 的响应头映射；当前文档只确认两者是不同 ID。
- 对当前本地 llama.cpp/Qwen 验证 Responses JSON/SSE、Tool Call、Structured Output、reasoning_effort 和断流行为；未验证能力继续为 unknown。

**当前结论：七项协议语义已接受；Schema、Golden、Contract Test 未齐，LLM-CTR-001/002/003 仍为 pending_evidence，不代表实现完成或 L2 Module Ready。**



### 10.4 Capability 有效期与请求资源边界

Capability 记录唯一键为 `(provider_id, model_id, protocol, gateway_version, capability_key)`，至少保存 status、verified_at、验证方式和 evidence_ref。状态有效期由 ProviderConfig 显式配置；超过有效期、Gateway 版本变化或模型 ID 变化后，状态一律视为 unknown，不能沿用历史 supported。

- unsupported 必须有可复现的失败证据；unknown 表示未验证、证据过期或运行环境发生变化。
- 可选能力只有 status=supported 且证据仍在有效期内才可使用；unknown/unsupported 不发送对应字段或能力。
- ProviderConfig 的 request_timeout、max_request_bytes、max_response_bytes 必须为正数并显式配置；不得依赖 HTTP Client 隐式默认值，且不得超过 PLAT-CTR-005 的全局安全上限。
- ModelRequest.max_output_tokens 必须大于 0，并不超过当前模型已验证的输出上限；instructions、input_text、Schema、Tool 数量及序列化后的总请求字节必须在发请求前校验。
- JSON Schema Draft 2020-12 是本地校验基线。上游 strict 子集不支持的关键字必须明确拒绝，或按调用方显式选择的 local_validation_only 模式处理；不得静默删除关键字后继续发送。
- 未知 SSE 事件保留 type、顺序和有界安全元数据；原始 Prompt、Response 和 SSE 默认不持久化。
- stream_enabled=true 必须绑定 provider/model/gateway 版本的真实 E2E 证据；仅路由可达、配置存在或模型清单可查询，不构成启用依据。


## 11. Platform Fault 映射与关联 ID 一致性

LLM Adapter 内部执行错误使用 Platform Fault；对调用方公开时投影为 ModelErrorInfo：

| Platform Fault | ModelErrorInfo |
|---|---|
| Fault Code | stable_code |
| Fault Class | kind（映射到 ModelErrorKind） |
| Retryable | retryable |
| Dependency | 仅在受控诊断元数据保留，不拼入 safe_message |
| Safe Message | safe_message |
| Cause | 仅内部日志，不序列化 |

ModelResponse.request_id 是该次模型调用的关联 ID；ModelErrorInfo 不重复携带 request_id。gateway_request_id 与 upstream_request_id 分别记录；correlation_id 继续用于跨模块链路关联。

ModelResponse.status=incomplete/failed 是协议响应结果，不自动等同于 Platform Fault。只有 Adapter 执行、传输、协议解析或本地校验失败才构造对应 Fault。上游 response.failed 的错误信息是安全归一化投影，不暴露原始响应正文。具体 Fault Go 类型与逐类映射表以 PLAT-CTR-001 为准；LLM 只维护 ModelErrorKind 投影，并以契约测试固定。
