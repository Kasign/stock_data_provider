# LLM 首批 Provider Port 与 DTO 字段级契约

- 所属任务：LLM-002
- 前置调查：LLM-001 静态协议盘点
- 首批协议：OpenAI Responses API
- 日期：2026-11-05
- 状态：契约草案，待 LLM/Platform Owner Review；不代表实现、真实端点验收或接口冻结。

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
| request_id | string | 是 | LLM 内部调用 ID，不承载 Secret |
| purpose_code | string | 是 | 稳定调用用途代码，不写自由文本 Prompt |
| model | string | 是 | 明确模型 ID，不依赖任意模型路由 |
| instructions | string | 是 | 序列化到 Responses instructions；允许空字符串 |
| input_text | string | 是 | 首批只支持单轮文本，序列化为 Responses input string |
| max_output_tokens | integer | 是 | 正整数，受调用预算限制 |
| temperature | *number | 否 | 仅 capability=supported 时发送 |
| top_p | *number | 否 | 仅 capability=supported 时发送 |
| reasoning_effort | enum | 否 | low/medium/high；未知能力时不得静默发送 |
| response_schema | JSONSchemaSpec? | 否 | 发送前做本地语法、大小和深度校验 |
| tools | ToolDefinition[] | 否 | 只接受调用方授权后的定义；不执行 |
| tool_choice | enum? | 否 | 首批限制 none/auto；required/指定工具延后 |
| deadline_at | UTC timestamp | 是 | 绝对截止时间，不得晚于调用方 Context deadline |
| correlation_id | string | 否 | 跨模块关联 ID，不承载领域对象 |
| trace_id | string | 否 | 受控追踪标识 |

首批不接收任意 map、任意 JSON 或完整 Responses input item 数组。多轮对话、多模态、background response、previous_response_id、conversation_id 等 deferred；出现时返回 unsupported_capability，不得静默丢弃。

### 4.1 JSONSchemaSpec

| 字段 | 类型 | 必需 | 约束 |
|---|---|---:|---|
| name | string | 是 | 稳定 Schema 名称 |
| schema_json | json.RawMessage | 是 | 合法 JSON Schema，限制字节数和深度 |
| strict | bool | 是 | 显式指定，不依赖上游默认值 |

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
| gateway_request_id | string? | 否 | 从响应头读取，缺失允许 |
| incomplete_reason | string? | 否 | Responses incomplete_details.reason |
| error | ModelErrorInfo? | 否 | 稳定错误码和安全摘要 |
| started_at / finished_at | UTC timestamp | 是 | 调用时间边界 |
| duration_ms | integer | 是 | 非负 |

OutputItem 首批类型：message、function_call、refusal、unknown。ContentPart 首批支持 output_text/refusal；未知 item 保留类型和安全元数据，不丢弃，也不伪装成空文本。

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
- Generate(ctx, ModelRequest) -> ModelResponse
- Stream(ctx, ModelRequest, onEvent func(ModelEvent) error) -> error

Stream 使用同步回调提供背压；回调返回错误或 Context 取消时停止读取上游。

ModelEvent 字段：type、sequence_number?、response_id?、output_index?、content_index?、item_id?、call_id?、delta?、usage?、error?、received_at。

首批识别 response.created、response.output_text.delta、response.output_text.done、response.completed、response.incomplete、response.failed、error。其他事件作为 UnknownEvent 保留类型和顺序，不执行副作用。上列是 Adapter parser 契约，不代表当前本地模型已产生所有事件。

流结束规则：
- 必须收到明确 terminal event（completed/incomplete/failed/error）才算终止；只有 completed 是成功，incomplete/failed/error 都必须按非成功结果返回。
- EOF 前没有 terminal event，返回 stream_interrupted；不得把连接关闭当成功。
- sequence_number 不得倒退；缺号/重号策略由 Fixture 固定并经 Owner Review。
- 首字节、首个文本 delta、首个 reasoning delta 分别记录；Responses TTFT 不能用首字节时间冒充。
- 收到首个事件后禁止自动透明重试，避免重复计费和重复输出。

## 7. Error Contract 与重试

ModelErrorKind：invalid_request、unauthorized、rate_limited、overloaded、timeout、cancelled、upstream_unavailable、upstream_error、invalid_response、incomplete_response、schema_validation_failed、unsupported_capability、stream_interrupted。

ModelErrorInfo 仅暴露 kind、stable_code、retryable、http_status?、retry_after?、request_id 和安全摘要；底层 Cause 仅内部日志，不序列化给调用方。

- 一次调用只能由一个层负责业务重试；首批建议由 Researchflow Run/Stage 策略决定，LLM Adapter 默认不自动重试。
- 429/503 可返回 retry_after 提示，是否重试由调用方 deadline/预算决定。
- timeout/cancelled 不自动重试。
- 请求可能已被上游接收但响应丢失时，不得假定未执行；仅确认未派发/连接建立前失败时才可考虑重试。
- Stream 已发出任意事件后禁止透明重试。

## 8. 安全与资源

- API Key 只通过 SecretRef 读取；不得进入 ModelRequest、DTO、错误摘要或日志。
- 不接受任意 Base URL、任意 Header 或请求方指定 Provider endpoint。
- 输入/Schema/Tool 数量和字节数设上限；非流式响应限制最大字节数；SSE 单事件/单行设上限。
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

## 10. Owner 未决项

1. 首批 input_text 单轮输入是否满足研究流程；建议先固定单轮文本，复杂 input item/multimodal 延后。
2. ToolDefinition 是否进入首批 ModelRequest；建议保留受控结构，但 capability=unknown 时禁止发送，工具执行永远归 Researchflow。
3. reasoning_effort 是否纳入首批参数；当前配置样本有 high，但 Qwen/llama.cpp 对该字段的实际行为仍需验证。
4. JSON Schema validator 的具体库与 Draft 版本；先冻结语义和 Fixture，不在此阶段锁定实现库。
5. Usage.total_tokens 缺失时是否允许派生；建议允许但必须 source=derived。
6. SSE sequence_number 缺号/重号策略；建议重号视为协议异常，缺号记录诊断并继续，最终由 Owner Review 确认。
7. Gateway request_id 与 LLM request_id 的映射；建议保留双字段，不覆盖内部 correlation_id。

**当前结论：LLM-002 的最小 Port/DTO 已细化到可评审和编写 Fixture 的粒度；仍待 LLM/Platform Owner Review。LLM-001 在线 E2E、LLM-004/005 真实模型能力仍未通过。**