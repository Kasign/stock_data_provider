# LLM-002 JSON 与 SSE 契约 Fixture 清单

- 归属任务：LLM-002-F
- 对应协议：OpenAI Responses
- 日期：2026-11-05
- 状态：测试设计基线，待 LLM/Platform Owner Review；不是已实现测试或真实模型能力证明。

## 1. Fixture 原则

1. 每个样例同时保存原始 wire payload、预期内部 DTO、预期 typed error/终态和关键诊断。
2. JSON fixture 使用原始 Responses envelope；SSE fixture 保留 event 行、data 行、空行分隔和 EOF 位置，不把事件预先拼成数组。
3. 未知字段/事件不应导致 panic；已知结构缺字段、类型错误或终态矛盾必须返回明确错误。
4. Fixture 验证的是 Adapter parser/normalizer，不证明某个 Gateway 或模型真实支持相应能力。
5. Tool Call Fixture 只断言结构化解析，不触发任何工具执行。
6. 任何包含 Prompt、响应正文或工具参数的 Fixture 均使用合成数据，不放入真实用户内容、Secret 或生产日志。

## 2. JSON Request Mapping

| Fixture | 输入条件 | 预期检查 |
|---|---|---|
| request_minimal_text.json | model、instructions、input_text、max_output_tokens | Responses `instructions` / `input` / `max_output_tokens` 映射准确；不发送空 tools 或默认采样参数 |
| request_sampling_supported.json | temperature/top_p/reasoning_effort + 对应 capability=supported | 仅支持字段发送，值不被改写 |
| request_sampling_unknown_rejected.json | 可选采样字段存在但 capability=unknown | 返回 unsupported_capability；不静默丢弃或透传 |
| request_tool_auto_supported.json | tools 非空、tool_choice=auto、function_tool_calls=supported | tools/name/description/parameters/strict 映射准确 |
| request_tool_capability_unknown.json | tools 非空但 capability=unknown | 本地拒绝，不发送上游 |
| request_schema_draft2020.json | Draft 2020-12 Schema | 本地 Schema 编译通过；上游 strict 子集检查单独执行 |
| request_schema_unsupported_keyword.json | Schema 含上游 strict 子集不支持关键字 | 不删除关键字后继续；返回 unsupported_capability 或 invalid_request |

## 3. JSON Response Golden

### 3.1 Completed 文本

文件：`response_completed_text.json`

```json
{
  "id": "resp_fixture_001",
  "object": "response",
  "model": "fixture-model",
  "status": "completed",
  "output": [
    {
      "id": "msg_fixture_001",
      "type": "message",
      "status": "completed",
      "content": [{ "type": "output_text", "text": "Fixture answer" }]
    }
  ],
  "usage": { "input_tokens": 12, "output_tokens": 5, "total_tokens": 17 }
}
```

预期：ModelResponse.status=completed；response_id 有值；output_items 顺序保留；output_text 为 `Fixture answer`；Usage.source=upstream。

### 3.2 Function Call

文件：`response_completed_function_call.json`

```json
{
  "id": "resp_fixture_002",
  "object": "response",
  "model": "fixture-model",
  "status": "completed",
  "output": [{
    "id": "fc_item_001",
    "type": "function_call",
    "status": "completed",
    "call_id": "call_fixture_001",
    "name": "lookup_quote",
    "arguments": "{\"symbol\":\"600000.SH\"}"
  }]
}
```

预期：产生一个 ModelToolCall；arguments_json 原文保留；LLM Adapter 不执行工具；Researchflow 必须重新按 Registry/Schema 校验。

### 3.3 Refusal 与未知输出项

- `response_completed_refusal.json`：message content 包含 refusal part；保留 refusal 类型，不合并为普通 output_text。
- `response_unknown_output_item.json`：output 中包含未来未知 type；保留 type 与有界安全元数据；不得静默丢弃后仍声称输出完整。

### 3.4 Incomplete / Failed

- `response_incomplete_max_output_tokens.json`：status=incomplete，incomplete_details.reason=max_output_tokens；预期 status=incomplete、incomplete_reason 保留，不得判为 completed。
- `response_failed_upstream.json`：status=failed，error 含上游 code/message；预期 ModelResponse.status=failed，ModelResponse.error 有稳定错误映射；结构合法的上游终态不重复转成 Go error。
- `response_missing_status.json`：缺 status；预期 invalid_response。
- `response_completed_missing_id.json`：status=completed 但缺 response id；预期协议异常，不得构造伪 response_id。

### 3.5 Usage

- `response_usage_missing.json`：无 usage；预期 Usage=nil/unknown，不补 0。
- `response_usage_partial.json`：只有 input_tokens；预期 total_tokens 不派生。
- `response_usage_derived_total.json`：input_tokens/output_tokens 都存在、total_tokens 缺失；预期 total_tokens=input+output，source=derived。
- `response_usage_overflow.json`：分项相加溢出；预期不派生，记录 usage 诊断。
- `response_usage_inconsistent_total.json`：上游 total 与分项和不一致；保留上游 total，source=upstream，记录一致性诊断。

## 4. Structured Output Fixture

| Fixture | 场景 | 预期 |
|---|---|---|
| structured_valid_draft2020.json | 输出是合法 JSON 且符合 Schema | structured_json 提供，Schema 校验成功 |
| structured_invalid_json.json | 输出文本不是合法 JSON | schema_validation_failed 或 invalid_response，不能回退成成功文本 |
| structured_schema_mismatch.json | JSON 可解析但缺 required/类型不符 | schema_validation_failed |
| structured_truncated_json.json | 输出被截断 | 不提供 structured_json 成功结果；记录 incomplete/parse error |
| structured_unknown_format.json | 返回格式不符合请求格式 | 明确失败，不静默转换 |

Schema fixture 需要覆盖 required、additionalProperties、enum、数组项、嵌套对象、数值边界、format 校验、递归/深度上限和超大输入拒绝。上游是否接受 strict Schema 与本地 Schema 校验必须分别断言。

## 5. SSE Golden

### 5.1 完整文本流

文件：`stream_completed_text.sse`

```text
event: response.created
data: {"type":"response.created","sequence_number":1,"response":{"id":"resp_stream_001","object":"response","status":"in_progress","model":"fixture-model"}}

event: response.output_text.delta
data: {"type":"response.output_text.delta","sequence_number":2,"response_id":"resp_stream_001","output_index":0,"content_index":0,"delta":"Fixture "}

event: response.output_text.delta
data: {"type":"response.output_text.delta","sequence_number":3,"response_id":"resp_stream_001","output_index":0,"content_index":0,"delta":"stream"}

event: response.output_text.done
data: {"type":"response.output_text.done","sequence_number":4,"response_id":"resp_stream_001","output_index":0,"content_index":0,"text":"Fixture stream"}

event: response.completed
data: {"type":"response.completed","sequence_number":5,"response":{"id":"resp_stream_001","object":"response","status":"completed","model":"fixture-model","output":[],"usage":{"input_tokens":8,"output_tokens":2,"total_tokens":10}}}

```

预期：按到达顺序回调事件；Stream 返回最终 ModelResponse.status=completed；Usage.source=upstream；只有 terminal response.completed 后才可判成功。

### 5.2 Function Call 流

文件：`stream_function_call.sse`

```text
event: response.created
data: {"type":"response.created","sequence_number":1,"response":{"id":"resp_tool_001","status":"in_progress","model":"fixture-model"}}

event: response.output_item.added
data: {"type":"response.output_item.added","sequence_number":2,"output_index":0,"item":{"id":"fc_item_001","type":"function_call","status":"in_progress","call_id":"call_001","name":"lookup_quote","arguments":""}}

event: response.function_call_arguments.delta
data: {"type":"response.function_call_arguments.delta","sequence_number":3,"item_id":"fc_item_001","output_index":0,"call_id":"call_001","delta":"{\\\"symbol\\\":"}

event: response.function_call_arguments.delta
data: {"type":"response.function_call_arguments.delta","sequence_number":4,"item_id":"fc_item_001","output_index":0,"call_id":"call_001","delta":"\\\"600000.SH\\\"}"}

event: response.function_call_arguments.done
data: {"type":"response.function_call_arguments.done","sequence_number":5,"item_id":"fc_item_001","output_index":0,"call_id":"call_001","arguments":"{\\\"symbol\\\":\\\"600000.SH\\\"}"}

event: response.output_item.done
data: {"type":"response.output_item.done","sequence_number":6,"output_index":0,"item":{"id":"fc_item_001","type":"function_call","status":"completed","call_id":"call_001","name":"lookup_quote","arguments":"{\\\"symbol\\\":\\\"600000.SH\\\"}"}}

event: response.completed
data: {"type":"response.completed","sequence_number":7,"response":{"id":"resp_tool_001","status":"completed","model":"fixture-model","output":[]}}
```

预期：按 item_id/call_id 聚合参数增量；arguments.done 与 output_item.done 的最终参数一致；生成待授权 ModelToolCall，不执行工具。若最终 arguments 与 delta 聚合不一致，返回 invalid_response。

### 5.3 Terminal 事件

- `stream_incomplete.sse`：以 response.incomplete 结束，返回 status=incomplete 和 incomplete_reason。
- `stream_failed.sse`：以 response.failed 结束，返回 status=failed 和 ModelErrorInfo。
- `stream_error_event.sse`：以 error event 结束，返回 typed protocol/upstream error；不能将其解释成 completed。
- `stream_unknown_event.sse`：未知 event 保留事件类型、顺序和有界安全元数据；不触发副作用。

### 5.4 Sequence 与 EOF

| Fixture | 输入 | 预期 |
|---|---|---|
| stream_sequence_contiguous.sse | 10,11,12,13 | 正常解析 |
| stream_sequence_gap.sse | 10,11,13 | stream_sequence_gap；不可 completed |
| stream_sequence_duplicate.sse | 10,11,11 | invalid_response；不可 completed |
| stream_sequence_decreasing.sse | 10,12,11 | invalid_response；不可 completed |
| stream_sequence_all_missing.sse | 全部事件无 sequence_number | 允许解析，序号能力记录 unavailable |
| stream_sequence_mixed_missing.sse | 部分事件有序号、部分无 | invalid_response |
| stream_eof_without_terminal.sse | delta 后直接 EOF | 部分 Response + stream_interrupted；不可 completed |
| stream_terminal_then_extra_event.sse | terminal 后仍有事件 | 协议异常；不得追加为同一成功响应的有效输出 |

### 5.5 Backpressure / Cancel

- `stream_callback_error.test`：callback 返回 sentinel error；立即停止读取并返回部分 Response + 原 callback error。
- `stream_context_cancel.test`：Context cancel/deadline；关闭上游 body/连接，返回部分 Response（若有）和 cancelled/timeout typed error。
- `stream_slow_callback.test`：慢 callback 应形成自然背压，不创建无界事件缓冲队列。
- `stream_large_event.test`：单行/单事件/累计字节超限，返回 resource_limit/invalid_response，并释放资源。

## 6. 统一断言矩阵

每个 Fixture 至少断言：

1. 是否解析成功；若失败，稳定错误 kind/code。
2. ModelResponse.status、response_id、request_id、provider_id 和模型字段。
3. OutputItem/ContentPart 的类型、顺序、ID 和文本/参数。
4. Usage 各 nullable 字段与 source；缺失不得变 0。
5. SSE 事件顺序、sequence_number 诊断、terminal 状态和 EOF 语义。
6. 是否可标记 completed；只有合法 completed terminal/JSON status 才允许。
7. Secret、Authorization、完整敏感 Prompt 不进入日志、错误摘要或测试快照。
8. Tool Call 仅形成待授权 DTO，测试中不存在工具执行副作用。

## 7. 与能力验收的边界

- JSON/SSE Fixture 全通过只代表 LLM Adapter 的协议解析与 DTO 归一化达到模块级测试要求。
- `responses_stream`、`structured_output_json_schema`、`function_tool_calls`、`reasoning_effort` 的 CapabilityState 必须另由当前 Provider + Model + Gateway 版本的真实 E2E 证据设置。
- 远程 Provider Mock、Gateway 路由存在、字段透传、Fixture 全通过均不能单独把本地 llama.cpp/Qwen 能力设为 supported。
- 当前仍未对实际部署 Gateway 发起真实调用；LLM-001 partial，LLM-004/005 真实能力待验收。

## 8. Owner 待确认

1. 是否接受 sequence_number 缺号即失败的严格策略。
2. 是否接受 Draft 2020-12 作为本地 Schema dialect，并明确 format assertion 语义。
3. SSE `response.error` 与 `error` event 的稳定错误映射。
4. unknown OutputItem/ContentPart 的安全元数据 allowlist。
5. 对 terminal 后多余事件是立即失败还是只记录协议诊断；建议立即失败。

**本清单是 LLM-002-F 的测试设计基线，待 Owner Review；尚未生成实际 fixture 文件、测试代码或真实模型验收结果。**
