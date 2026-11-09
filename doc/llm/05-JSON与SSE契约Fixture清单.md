# LLM-002 JSON 与 SSE 契约 Fixture 清单

- 归属任务：LLM-002-F
- 对应协议：OpenAI Responses
- 日期：2026-11-05
- 状态：Fixture 设计基线、首批 JSON/SSE Golden 文件及 Mapper/Parser/HTTP Adapter/纯 SSE Parser 单测代码已存在；Golden/单测执行证据仍缺失，对应 LLM-CTR-001～003 当前为 pending_evidence；不代表真实模型能力证明。

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
| request_full_options.expected.json | temperature/top_p/reasoning_effort + strict JSON Schema + function tool + tool_choice=auto | 一次固定完整 wire 请求，校验采样、reasoning、text.format、schema、tool parameters/strict 和工具选择映射 |
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
data: {"type":"response.function_call_arguments.delta","sequence_number":3,"item_id":"fc_item_001","output_index":0,"call_id":"call_001","delta":"{\"symbol\":"}

event: response.function_call_arguments.delta
data: {"type":"response.function_call_arguments.delta","sequence_number":4,"item_id":"fc_item_001","output_index":0,"call_id":"call_001","delta":"\"600000.SH\"}"}

event: response.function_call_arguments.done
data: {"type":"response.function_call_arguments.done","sequence_number":5,"item_id":"fc_item_001","output_index":0,"call_id":"call_001","arguments":"{\"symbol\":\"600000.SH\"}"}

event: response.output_item.done
data: {"type":"response.output_item.done","sequence_number":6,"output_index":0,"item":{"id":"fc_item_001","type":"function_call","status":"completed","call_id":"call_001","name":"lookup_quote","arguments":"{\"symbol\":\"600000.SH\"}"}}

event: response.completed
data: {"type":"response.completed","sequence_number":7,"response":{"id":"resp_tool_001","status":"completed","model":"fixture-model","output":[{"id":"fc_item_001","type":"function_call","status":"completed","call_id":"call_001","name":"lookup_quote","arguments":"{\"symbol\":\"600000.SH\"}"}]}}
```

预期：按 item_id/call_id 聚合参数增量；arguments.done、output_item.done 与 delta 聚合结果必须一致；生成待授权 ModelToolCall，不执行工具。若最终 arguments 与 delta 聚合不一致，返回 invalid_response。

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
- `stream_large_event.test`：单行/单事件/累计字节超限，返回 resource_limit，并释放资源。

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

## 8. 已接受语义与实现证据待办

sequence_number 严格连续策略、Draft 2020-12 本地 Schema dialect、Request ID 四层区分已在 04 文档第 10 节接受，不再等待个人 Owner 确认。

本轮明确以下解析规则：
1. SSE error event 携带合法上游错误对象时映射为 upstream_error；event/data 结构畸形或字段矛盾映射 invalid_response。
2. UnknownEvent 仅保留 type、sequence_number（若有）、response_id、output_index、content_index、item_id、call_id、status，以及 item/content_part 的 type/id 等 allowlist 元数据；不得保留未知正文、Prompt 或任意原始字段。
3. terminal event 后出现额外完整事件按 invalid_response；不得追加到已完成响应。
4. 以上规则必须进入 JSON/SSE Golden 与 Contract Test；本清单不代表测试代码已实现或执行通过。

**本清单是 LLM-002-F 测试设计基线；独立 JSON/SSE fixture 文件尚未齐备。Mapper/Parser/HTTP Adapter/纯 SSE Parser 单测代码已存在但尚未执行，不代表真实模型验收通过。**


## 9. Responses JSON Parser 当前实现状态（2026-11-06）

已新增非流式 Responses JSON Request Mapper / Response Parser 的首批单测，覆盖请求字段映射、应用层元数据不进入 wire body、tool_choice 默认 none、completed/incomplete/failed、message/refusal/function_call/unknown item、Usage total 优先与安全派生、Schema 输出二次校验、畸形 envelope 和未知类型安全保留。

注意：
- 以上是已提交的测试代码，不是独立 Golden Fixture 文件；本轮尚未执行 gofmt/go test。
- 未知 OutputItem/ContentPart 仅保留受限的 wire type 元数据，不保留未知正文；annotations 暂不回传，避免任意上游元数据进入 DTO。
- 非流式 JSON Parser 不代表 HTTP Adapter、SSE Parser 或真实 Gateway E2E 已完成。


## 10. HTTP Adapter 测试覆盖（2026-11-06）

首批 fake-doer 单测覆盖固定 Responses endpoint、Bearer caller key 注入、X-Request-ID 与 Request-Id 分离、HTTP 429 与 Retry-After、传输错误脱敏、缺失 Secret/Doer 拒绝，以及 request deadline、Context deadline 和 Provider timeout 取最早值。fake doer 只用于测试；真实网络必须经 Platform BoundDoer。测试代码尚未运行，不能作为通过证据。


## 11. Clock / Deadline 确定性测试（2026-11-06）

- 使用固定/可推进 Clock，覆盖 ModelRequest deadline 校验与 CapabilityState 有效期边界。
- HTTP Adapter 覆盖 ModelRequest deadline、Context deadline、Provider timeout 三者分别成为最早边界；deadline 已过时必须在 doer 调用前终止。
- 通过可推进 Clock 验证 started_at / finished_at / duration_ms，不依赖测试机实际耗时。
- 本轮新增测试代码尚未执行，仍为 pending_evidence。


## 12. 纯 SSE Parser 首批测试覆盖（2026-11-06）

- 完整文本流：created → text delta → completed；校验事件顺序、response_id、model、Usage。
- Terminal：incomplete/failed 作为合法 ModelResponse 状态；error event 映射 upstream_error，不暴露原始 message。
- Sequence：连续序号、全缺失、混合缺失、缺号、重复、倒退；成功结果记录 sequence_number_status=available/unavailable。
- Function Call：arguments delta 聚合，并与 arguments.done、output_item.done、terminal response 最终参数比对。
- UnknownEvent：仅保留 type、sequence、response/index/item/call/status 和 item/content_part 安全类型标识，不保留未知正文。
- EOF/终态：无 terminal 返回部分 Response + stream_interrupted；terminal 后额外事件拒绝；callback error 原样返回。
- Resource：总字节、单行、单事件、事件数量和函数参数累计大小均有上限。
- 测试代码尚未执行；没有独立 SSE Golden 文件，也没有 Platform 网络流式接入或真实 Gateway E2E 证据。


## 13. 已落库的首批 Golden Fixture

### JSON

- `internal/llm/testdata/json/response_completed_text.json` + `.expected.json`
- `internal/llm/testdata/json/response_completed_function_call.json` + `.expected.json`
- `internal/llm/testdata/json/response_incomplete_max_output_tokens.json` + `.expected.json`
- `internal/llm/testdata/json/response_usage_inconsistent_total.json` + `.expected.json`

### SSE

- `internal/llm/testdata/sse/stream_completed_text.sse` + `.expected.json`
- `internal/llm/testdata/sse/stream_function_call.sse` + `.expected.json`
- `internal/llm/testdata/sse/stream_incomplete.sse` + `.expected.json`
- 错误/边界原始样例：sequence_gap、EOF_without_terminal、unknown_event、terminal_then_extra_event、error_event。

`responses_golden_test.go` 将原始 wire 与 expected DTO 比较；错误/边界 fixture 断言错误类型和安全行为。此集合只是第一批，Usage missing/partial/derived/overflow、mixed/duplicate/decreasing sequence、callback error、CRLF/multiline data、资源限制等仍需补充独立 fixture。部分边界目前由内联单测覆盖，尚未全部拆成独立 Fixture。


## 14. LLM 公共 DTO Draft 2020-12 Schema

已新增模块机器契约 Schema，版本 1.0.0：

- `schema/provider-config-1.0.0.schema.json`
- `schema/capability-state-1.0.0.schema.json`
- `schema/model-request-1.0.0.schema.json`
- `schema/model-response-1.0.0.schema.json`
- `schema/model-event-1.0.0.schema.json`
- `schema/model-error-info-1.0.0.schema.json`

新增 `dto_schema_test.go`：验证 ProviderConfig、CapabilityState、ModelRequest、ModelResponse、ModelEvent、ModelErrorInfo 的实际 JSON 序列化结果；并使用现有 JSON/SSE Golden 解析结果执行 Schema 校验，覆盖必填字段与公开错误枚举拒绝。CapabilityState 的 verification_method 与 ProviderConfig.capability_ttl_ms 同时纳入机器契约。

以上为 Schema 与 Contract Test 代码交付，尚未执行 gofmt/go test；不代表 LLM-CTR-001～003 已冻结。


## 15. Golden 覆盖扩展（2026-11-06）

新增 JSON 原始 Fixture：refusal、unknown output item、failed、Usage missing/partial/derived/overflow、missing status、completed missing response id；新增 request_minimal_text.expected.json 固定默认请求映射。

新增 SSE 原始 Fixture：failed terminal、sequence duplicate/decreasing/mixed-missing/all-missing。`responses_golden_test.go` 增加对应 Contract Golden 断言，覆盖 refusal 文本保留、未知正文脱敏、合法 failed 状态、Usage null/derived/overflow、非法 envelope、序号异常和 all-missing 标记。

以上 Fixture/测试代码尚未执行；其余完整 SSE 网络读取、Context cancel/backpressure 和本地 Gateway E2E 仍待 Platform streaming transport 与真实部署验收。

## 16. SSE Callback 中断回归覆盖

- 新增 `TestParseResponsesSSECallbackErrorReturnsPartialResponse`，使用 EOF 前已有文本增量的 Fixture，在消费者收到文本增量时返回 sentinel error。
- 新增 `TestParseResponsesSSECallbackErrorAtTerminalDoesNotComplete`，在合法 completed terminal 回调时返回 sentinel error，验证消费者拒绝终态事件后不能向上层暴露 completed。
- 两项测试均断言 Parser 原样返回 callback error、保留已接收的部分文本、状态保持 incomplete、不生成 ModelErrorInfo；普通事件中断另检查结束时间和耗时元数据。
- 此测试只覆盖纯 Parser 的同步回调中断语义，不代表网络读取取消、Body 关闭或 Platform backpressure 已实现。
- 测试代码已提交，尚未执行 gofmt/go test。


## 17. 新增内联边界测试（2026-11-09）

- Structured Output：非法 JSON 与空输出均拒绝并返回 schema_validation_failed；incomplete 响应不生成 structured_json，保留 incomplete_reason。
- SSE：新增 CRLF + 注释行 + 多条 data 拼接解析测试；新增超过 MaxSSEEvents 的 resource_limit 测试。
- 上述测试是纯解析器/DTO 层验证，不代表网络读取取消、HTTP Body Close、背压或 Platform 流式传输验收完成。
- 新增测试代码尚未执行 gofmt/go test，不能标记通过。


## 18. SSE 资源限制分类回归（2026-11-09）

- 单事件大小按所有 data 字段值及字段间拼接换行累计，超过 1 MiB 返回 `resource_limit`。
- 即使每一条 data 行均小于 64 KiB，多行拼接后超过单事件上限也必须返回 `resource_limit`。
- 整个 SSE 输入超过 8 MiB 时，在扫描前返回 `resource_limit`，稳定码为 `llm.sse.total_too_large`。
- 对应测试：`TestParseResponsesSSEClassifiesMultilineEventLimitAsResourceLimit`、`TestParseResponsesSSERejectsTotalPayloadAboveLimit`。
- 测试代码尚未执行；此处只定义验收预期，不构成通过证据。


## 19. SSE 终态后事件拒绝（2026-11-09）

- 对应测试：`TestParseResponsesSSERejectsEventAfterTerminal`。
- `response.completed`、`response.incomplete` 或 `response.failed` 终态后若仍出现携带 data 的事件，必须拒绝整个流。
- 当前覆盖 completed 后追加普通事件，预期错误为 `ErrorInvalidResponse / llm.sse.event_after_terminal`。
- 测试代码尚未执行，不构成通过证据。


## 20. SSE 终态文本一致性

- 输入先发送一个或多个 `response.output_text.delta`（或 `response.output_text.done`），终态 `response.output` 必须还原出完全相同的文本。
- 若终态文本与已累计文本不一致，返回 `ErrorInvalidResponse / llm.sse.terminal_output_text_mismatch`；不得以终态结果覆盖已交付给消费者的文本。
- 对应测试：`TestParseResponsesSSERejectsTerminalOutputTextMismatch`。
- 此规则属于本地 Parser 一致性测试，仍待执行。


## 21. 多文本分段 SSE 一致性回归

- 文本事件按 `output_index + content_index` 分段累计，多个分段可交错到达；最终 ModelResponse.output_text 按索引顺序聚合。
- 每个分段的 delta 与 done 独立比对，不能把一个分段的累计文本与另一个分段的 done 文本比较。
- done 后再次收到同一分段 delta 必须拒绝；负 output_index/content_index 必须拒绝。
- 对应测试：`TestParseResponsesSSEAggregatesMultipleTextPartsByIndex`、`TestParseResponsesSSERejectsTextDeltaAfterDone`。
- 测试代码尚未执行，不构成通过证据。

## 22. Function Call 参数终态一致性

- 每个 call_id 的 arguments delta 独立累计；arguments.done 必须与此前累计值一致。
- 收到 arguments.done 或对应 output_item.done 后，该 call_id 的参数流进入终态；后续 delta 必须拒绝。
- 终态 Response 中的 ToolCall.arguments_json 必须与已累计参数一致。
- 对应测试：`TestParseResponsesSSEFunctionCallAggregationAndConsistency`、`TestParseResponsesSSERejectsFunctionArgumentDeltaAfterDone`。
- 测试代码尚未执行，不构成通过证据。


## 23. Responses Strict Schema 子集回归

- 接受标准：根 object、所有对象属性 required、additionalProperties=false；允许嵌套 anyOf、nullable 类型联合、$defs 与本地 $ref。
- 拒绝标准：根 anyOf、allOf/其他未知关键字、缺失 required 属性、additionalProperties=true、未知 string format、远程 $ref。
- 上游 strict Response Schema 与 strict Function Tool 参数 Schema 都必须执行门禁；local_validation_only 仅跳过上游子集检查。
- 限制覆盖：对象深度、属性总数、字符串元数据总长度、enum 数量与大 enum 字符数。
- 对应测试：`TestValidateResponsesStrictSchemaSubsetAcceptsSupportedSchema`、`TestValidateResponsesStrictSchemaSubsetRejectsUnsupportedFeatures`、`TestMapResponsesRequestRejectsUnsupportedStrictSchemaButAllowsLocalValidation`、`TestMapResponsesRequestRejectsUnsupportedStrictToolSchema`。

## 24. Usage 明细范围诊断

- 缓存 Token 不能为负，且不能超过 input_tokens。
- 推理 Token 不能为负，且不能超过 output_tokens。
- 越界时保留上游原始值并追加 Usage.Diagnostics，不把异常明细当作可信正常值。
- 对应测试：`TestParseResponsesUsageDetailsAreRangeChecked`。

## 25. 高级 Schema 约束能力证据

- 发送上游的 Response Schema / Function Tool 参数，只要包含 `pattern`、`format`、数字边界、`multipleOf`、`minItems/maxItems`，必须要求 `structured_output_advanced_constraints=supported`。
- 该能力绑定 Provider、Model、Protocol、Gateway 版本和有效期；unknown/unsupported 一律不派发。
- strict 与 non-strict 都执行高级约束能力门禁；`local_validation_only` 只对 Response Schema 跳过上游能力要求。
- 对应测试：`TestValidateCapabilitiesRequiresAdvancedSchemaConstraintEvidence`。
