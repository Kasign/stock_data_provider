# 现有模型 Gateway 协议盘点

- 调查对象：Kasign/llm-gateway
- 分支与版本：dev @ 3cd5aacced882143628c23ee00e06243616d1a5f
- 源码观察日期：2026-11-05
- 调查方式：GitHub 源码、OpenAPI、配置契约和 Mock/opt-in 测试静态核验
- 状态：静态协议盘点完成；当前部署的本地 llama.cpp Responses 端到端调用尚未执行，不代表生产模型链路验收通过。

## 1. 结论摘要

当前调用侧主协议确定为 **OpenAI Responses API**，不是 Chat Completions 的字段翻译层：

- Method：POST
- Path：/v1/responses
- Content-Type：application/json
- Base URL：部署配置中的 Gateway HTTPS 根地址，末尾包含 /v1；真实主机名与密钥不写入仓库。
- 客户端 wire protocol：Responses（wire_api=responses 是客户端/SDK 的协议选择，不是 Gateway 服务端配置项）。
- 认证：调用方 Gateway API Key，优先使用 Authorization: Bearer；Gateway 同时接受 X-API-Key。
- 非流式：Responses JSON response envelope。
- 流式：同一路径，stream=true 或 Accept: text/event-stream，返回 SSE。
- 模型：使用 Gateway 可见的模型 ID；Gateway 会根据活动本地模型、Provider model mapping 或 Agent 默认模型解析实际上游模型，并覆盖出站 model 字段。

用户已有部署配置样本为 HTTPS Gateway + /v1 + wire_api=responses。部署主机名、调用方 Key 和上游 Key 均只保留在本地部署配置/Secret，不进入此文档或源码。

**关键边界：** Gateway 路由能够接收并转发 Responses 请求，不等于当前本地 llama.cpp/Qwen 模型对所有 Responses 字段、结构化输出和工具调用都具备生产兼容性。当前源码中本地 Provider 的 Capabilities 显式声明 ChatCompletions/Streaming，没有独立 Responses capability 位；Responses 是按路径转发并由上游实际能力决定。

## 2. Endpoint 与认证

### 2.1 请求入口

| 项目 | 约定 | 证据/边界 |
|---|---|---|
| Base URL | https://<gateway-host>:8443/v1 | 真实主机名只存在于部署配置 |
| Method / Path | POST /v1/responses | Server 将 /v1/* 统一交给 proxyV1/serveInference |
| Content-Type | application/json | Gateway 和本地/远程 Provider 均以 JSON 发送 |
| Stream | stream=true；或 Accept 包含 text/event-stream | core.parse 同时检查 JSON stream 与 Accept |
| Query | 原始 query 保留 | Forward 将 RawQuery 附加到上游路径 |
| Request ID | Gateway 生成 X-Request-ID；上游 Request-Id 等头按 Provider 路径转发/回传 | 不应将两个 ID 混为一项 |

### 2.2 调用方认证

- 推理接口支持 Authorization: Bearer <GatewayCallerKey> 或 X-API-Key。
- API Key 必须有效且启用；同时执行调用方 RPM、模型 ACL 和请求 Pipeline。
- X-Admin-Key 不是通用推理 Key；管理员 Web Session、Admin Key、调用方 Gateway Key 和上游 Provider Key 是四种不同凭据。
- Gateway 调用方凭据不会原样转发给本地 llama.cpp 或远程 Provider。
- 本地 llama.cpp 只有配置 LLAMA_UPSTREAM_API_KEY 时，Gateway 才向本地上游单独设置 Authorization: Bearer。
- 远程 Provider 的 API Key 由 Gateway 管理端配置，按 authorization / api-key / x-api-key Header 注入；不读取调用方传入的 Authorization 作为上游凭据。

## 3. 请求体字段与改写行为

### 3.1 Responses 请求

首批需要关注的字段：

| 字段 | Gateway 行为 | 当前结论 |
|---|---|---|
| model | 由 Gateway 模型解析结果覆盖为实际上游 ID | 客户端传入值不能作为实际模型选择的唯一依据 |
| instructions | 本地后端会与 input 中 system/developer 内容合并 | 本地路径会改写请求语义布局 |
| input | Responses 输入内容；本地路径移除其中 system/developer role 项，其内容并入 instructions | 其余 item 尽量保留；复杂 item 仍需真实模型验证 |
| max_output_tokens | 作为请求 JSON 字段转发 | 具体上限由本地模型/上游决定 |
| temperature / top_p 等 | 本地 llama.cpp/Ollama 可经 Gateway 推理参数面板合并；远程 Provider 不注入本地默认采样参数 | 参数是否支持由上游决定 |
| stream | 决定 JSON 或 SSE 转发 | 也可由 Accept: text/event-stream 推断 |
| tools | 本地 Responses sanitize 保留；Responses bridge 不改 path/body | 仅透传工具定义，不执行工具 |
| text.format | 当前未见 Gateway 对 Responses JSON Schema 做服务端校验或转换 | 本地模型是否支持需端到端验证 |
| metadata / 其他字段 | OpenAPI 推理 payload 为宽松 JSON，不能据此认定所有字段语义受 Gateway 保证 | 需按上游能力逐项验收 |

### 3.2 本地后端特有改写

当请求使用本地 llama.cpp/Ollama Provider 时：

1. sanitizeForLlama 针对 /v1/responses 解析 JSON。
2. 将 instructions 与 input 中 role=system/developer 的文本按顺序合并到 instructions。
3. 从 input 列表移除已合并的 system/developer 项。
4. 保留其他 input item 和 tools；JSON 无法解析时退回原始 body。
5. stream=true 且 stream_options.include_usage 未启用时，Gateway 注入 stream_options.include_usage=true。
6. 推理参数面板会按本地后端的参数策略合并请求；远程 Provider 不应用这组本地默认值。
7. Responses bridge 当前为 no-op：不把 /v1/responses 翻译成 /v1/chat/completions，也不重写工具字段。

因此，LLM Adapter 不应假设 Gateway 是字节级透明代理；对本地后端必须考虑 instructions/input 的规范化。stream_options.include_usage 是 Gateway 为本地流式请求注入的字段，其对当前 llama.cpp Responses 版本的兼容性仍需实测。

## 4. 响应、Usage 与结构化输出

### 4.1 非流式响应

Responses 成功响应以 OpenAI Responses envelope 为准，核心字段包括：

- id、object=response、model、status；
- output[]，其中 message 内容可包含 output_text；
- usage，通常包含 input_tokens、output_tokens、total_tokens；
- 失败或不完整时还可能有 error / incomplete_details 等字段。

Gateway 远程 OpenAI-compatible Provider 对成功 JSON 采用原始响应体透传；本地 Provider 通过本地 HTTP Client 转发上游 JSON。Gateway 没有为 Responses output 建立严格的全字段业务 DTO，也不替 LLM Adapter 做业务 Schema 校验。

### 4.2 Usage

当前 Gateway 统计解析器兼容：
- Chat Completions：usage.prompt_tokens / completion_tokens；
- Responses：usage.input_tokens / output_tokens；
- Responses SSE 完成事件：response.usage.input_tokens / output_tokens。

缺失 usage 应在 LLM Adapter 中保留为 unknown/null，不得填 0。流式请求时本地路径会尝试启用 include_usage，但不能保证每个上游都返回 usage。

### 4.3 Structured Output

- Gateway 对 Responses text.format / JSON Schema 没有观察到独立验证、修复或强制约束逻辑。
- 远程 Provider 主要透传请求字段；本地 sanitizer 只做 system/developer 合并和 stream usage 参数处理。
- 当前不能认定本地 llama.cpp + 实际 Qwen 模型对 Responses structured output 的 JSON Schema 语义、strict 模式和截断行为均兼容。
- LLM Adapter 必须在收到输出后自行解析并验证调用方指定的 Schema；模型声称 JSON 合法不能代替本地校验。非法、截断、缺字段必须返回结构化错误，不得静默回退成成功文本。

**结论：** Gateway 请求路径具备字段透传条件；结构化输出端到端能力仍为未验证，不应在 LLM-004 验收中标记通过。

## 5. Streaming / SSE

### 5.1 已证实行为

- stream=true 或 Accept: text/event-stream 会选择流式路径。
- Responses 与 Chat Completions 共用 /v1 入口，但仍按各自请求体和 SSE 事件语义处理。
- 本地 HTTP Provider 以 32 KiB 块读取上游并及时 Flush，SSE 原始字节透传；远程 OpenAI-compatible Provider 同样透传 SSE。
- 远程 Provider 的 Responses 路径会旁路解析 sequence_number；若响应已提交后发生上游读取错误，会追加 Responses 兼容的 event:error，并给出下一个 sequence_number。
- 非流式和流式远程 Provider Mock 测试覆盖 Responses JSON/SSE、query、OpenAI-Beta、模型覆盖、凭据隔离、Request-Id 和原始 SSE 字节。

### 5.2 已知限制与风险

- 本地 Provider 的 StreamProxy 在响应提交后遇到读取错误时返回错误并结束转发；没有发现它像远程 Responses Provider 一样追加 Responses event:error。此时 HTTP 状态已提交，客户端可能只观察到流中断。
- request_trace 的增量文本解析主要识别 Chat Completions choices[].delta；Responses response.output_text.delta 当前会作为通用 sse 事件记录，不形成统一 content delta/TTFT。
- Responses usage 可由独立 usage parser 识别；但不能因此认为 Responses 的全部事件类型已被结构化审计。
- 本地流式请求自动注入 stream_options.include_usage=true 的行为需要对当前 llama.cpp 版本验证。
- 远程 Provider 的 Responses error event 行为不等于当前本地 llama.cpp 上游也有相同错误封套。

**结论：** Gateway 的 SSE 传输链路具备实现基础；实际部署的本地 Responses 流式端到端仍待验证，LLM-005 不可标记完成。

## 6. Tool Call 边界

- Responses tools 定义在本地 sanitize 测试中保持不变；远程 Provider 测试也验证工具/请求体透传边界。
- Gateway 不执行模型返回的 function/tool call；它只负责转发请求与响应。
- 模型是否正确生成 function_call、参数是否符合 JSON Schema、是否遵循并行/拒绝语义，取决于实际上游和模型版本。
- LLM Adapter 只将工具调用解析为待校验的结构化输出；Researchflow 负责 Tool Registry、白名单、参数 Schema、权限、预算、执行和结果回填。
- 未经 Researchflow 授权的工具调用不得执行；不能将模型返回的工具名称或参数直接交给操作系统、HTTP 或数据库。

## 7. Timeout、取消、限流与错误

| 场景 | Gateway 可观察行为 | LLM Adapter 处理建议 |
|---|---|---|
| 调用方 Key 缺失/无效/过期 | 401 | 认证配置错误，不重试 |
| 调用方 RPM 超限 | 429 | 读取 Retry-After，按调用预算退避 |
| Redis 限流不可用 | 503 | 视为 Gateway 基础依赖故障，不盲目重试 |
| 请求体超限 | 常规推理入口 413，最大 64 MiB | 本地限制应远低于 Gateway 上限 |
| Provider 并发/队列满 | 503，可能为 provider_concurrency_limit / provider_queue_full | 有界退避；不在多个层重复重试 |
| Provider 队列等待超时 | 503 provider_queue_timeout | 记录排队耗时；按整体 deadline 判断是否重试 |
| 上游 HTTP 非 2xx | 保留/映射上游状态；错误正文按协议脱敏 | 按状态与稳定错误码分类，不解析任意敏感正文 |
| Gateway 整体 deadline | 未提交响应时 504 gateway_timeout；已提交流式响应后 HTTP 状态不能更改 | 将请求标记 timeout；不能把部分文本当完整成功 |
| 客户端断连/管理端取消 | Context 取消；未提交时可记录 499，已提交后保持原 HTTP 状态但审计标记 cancelled | 取消不可重试；及时释放资源 |
| 上游建连/读取失败 | 502/504 等稳定错误；远程 Provider 有 provider_upstream/provider_timeout 等映射 | 只对明确安全且幂等的请求有限重试 |
| Responses SSE 提交后失败 | 远程 Provider 追加 event:error；本地 Provider 当前主要结束流 | 必须识别异常终止，不能把 EOF/断流当 response.completed |

超时层：
- 本地 Gateway 整体请求时限由 llama.request_timeout_seconds 控制；当前契约记录正数限制整个请求，0 不增加 Gateway Client 整体时限，客户端断连/管理端取消仍有效。旧配置缺省兼容 3600 秒，实际部署值必须读取运行配置确认。
- 远程 Provider 可配置 request_timeout_seconds；0 不增加 Provider 级整体时限，仍受 Context、拨号/TLS 和 5 分钟响应头超时约束。
- 调用方、OpenResty proxy_read_timeout/proxy_send_timeout、Gateway 和上游时限必须分层协调；任一更短时限都可能先终止请求。
- 本地非流式上游响应体限制 32 MiB；远程 Provider 默认响应体限制 32 MiB，可配置但最高 512 MiB。常规入站推理请求体限制 64 MiB。

## 8. 当前测试证据与未覆盖范围

| 证据 | 已覆盖 | 不能据此宣称 |
|---|---|---|
| internal/provider/openai_compatible_test.go：Responses JSON/SSE | 远程 Provider Mock 的 JSON/SSE 透传、query、OpenAI-Beta、model 覆盖、调用方与 Provider Key 隔离、Request-Id、原始 SSE | 真实公网 Provider 或本地 llama.cpp 端到端 |
| internal/server/agent_responses_test.go | Agent 前缀路由、默认模型覆盖、双 Provider JSON/SSE 隔离 | 当前用户部署的全局本地模型路径 |
| internal/server/responses_passthrough_test.go | 本地 sanitize 保留 tools；Responses bridge 不重写路径/body | 本地 llama.cpp 真正接受并正确处理所有 Responses 请求 |
| internal/provider/openai_compatible_e2e_test.go | 存在 opt-in 的真实 Responses Provider 探测，需 LLM_GATEWAY_RESPONSES_E2E=1 和本地环境凭据 | 默认 CI 通过不代表 E2E 已执行；该探测也不是本地 llama.cpp 完整链路 |
| Gateway docs/api-config-db-contracts.md | CI #114 等历史 Mock/CI 证据及真实端点探测说明 | 真实本地模型、结构化输出、工具调用、长时流式和公网故障演练 |

本轮没有向部署 Gateway 发起真实推理请求，也没有读取或使用任何 API Key。静态代码审查与 Mock 测试证据不能代替线上模型验证。

## 9. LLM 模块 MVP 决策建议

1. 首批协议明确为 OpenAI Responses；不把 Chat Completions、Anthropic Messages、多模型路由同时塞入第一版抽象。
2. Model Adapter 显式配置 base_url、protocol、model、Secret 引用、请求 deadline 和 stream policy；不要从模型名称猜测协议能力。
3. ModelRequest 内部采用受控字段：request_id、model、instructions、input、max_output_tokens、sampling/reasoning parameters、response_schema、inert tool definitions、deadline 和 metadata。OpenAI JSON DTO 只存在 Adapter 内。
4. ModelResponse 统一输出 text/structured payload、response_id、actual_model、status、usage（nullable）、finish/incomplete/error details、request_id 和 timing。
5. Responses JSON output 需要按 output item 类型解析，不能只假设 output[0].content[0].text 永远存在。
6. Structured output 采用“请求 schema + Adapter 本地校验”双层约束；本地模型真实能力通过 E2E 决定是否启用，不做静态承诺。
7. Stream 使用 Responses SSE event parser；至少区分 response.created、response.output_text.delta、response.completed、response.incomplete、response.failed、error 和未知事件。结束事件缺失或流中断必须返回 incomplete/error。
8. 工具调用只解析为待审批 ToolCall，不在 LLM Adapter 内执行；Researchflow 负责完整授权和执行闭环。
9. Usage 缺失保持 nil/unknown；模型版本、Gateway request ID、总耗时、首字节和可用 TTFT 分开记录。
10. 超时/取消由调用方 deadline 驱动；仅对发送前失败或明确可安全重放的请求有限重试，流开始输出后禁止透明重试。

## 10. LLM-001 验收状态

- 静态盘点：完成。Endpoint、认证、协议路径、模型选择、请求改写、响应/Usage、SSE、工具边界、超时/错误、大小限制和测试证据均已记录。
- 在线真实请求：未执行（not_run）。本轮没有使用部署凭据，也没有可证明的当前部署运行日志。
- 本地 llama.cpp Responses JSON/SSE：待真实部署 E2E。
- Structured Output、工具调用：待当前模型版本 E2E。
- LLM-001 总体状态：**partial / 在线验收未通过**。静态结果足以启动 LLM-002 的 Port/Fixture 设计；不能据此宣称 LLM-005 或 LLM-004 端到端通过。

### 建议的下一轮在线验收用例

1. 非流式 Responses：最小 input、短 max_output_tokens，检查 response envelope、output_text 和 usage。
2. 流式 Responses：验证 event 顺序、response.completed、usage、断连后的停止行为。
3. instructions/input system/developer 合并：验证模型行为未因 Gateway sanitize 丢失或重复指令。
4. text.format JSON Schema：验证严格模式、非法输出、截断和 Schema 不匹配。
5. tools function call：验证模型只返回调用意图，Gateway 不执行工具。
6. 超时/取消：验证客户端断连和管理端停止后，上游请求确实结束并释放模型资源。
7. 错误矩阵：无效 Gateway Key、模型不可用、请求过大、上游 5xx、响应头超时和 SSE 中途断流。

**本文件只记录可复核的代码事实和待验收项，不将 Gateway 的 OpenAI-compatible 标签解释为完整 Responses/Structured Output/Tool Calling 兼容保证。**
