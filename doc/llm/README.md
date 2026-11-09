# LLM 模块

- 分支：`feat/llm`
- 代码：`internal/llm/**`
- 任务前缀：LLM
- 状态：LLM-002 DTO/Provider Port、Responses JSON Mapper/Parser、非流式 HTTP Adapter、Clock/Deadline 注入、纯 SSE Parser、公共 DTO Draft 2020-12 Schema 已实现；整体仍待 LLM 包级构建验证
- 最近代码提交：`830f8d54f2965ac35fe21dda201c6de01230a6dd`
- LLM-001：静态协议盘点完成；在线真实端点验收未执行
- LLM-002：DTO/Provider Port、Responses JSON 请求映射/响应解析、非流式 HTTP Adapter 及对应单测已实现，待 gofmt/go test；Platform Fault 映射、Draft 2020-12 Schema 编译及 DTO Schema Contract Test 均待构建验证；JSON/SSE Golden 已扩展覆盖拒绝、未知项、Usage 边界、终态和序号异常；新增完整 Request Mapper Golden，覆盖 sampling、reasoning、strict schema、function tool 与 tool_choice；新增 Adapter 能力门禁测试，验证可选能力证据缺失时不会派发请求；已补充 SSE callback error（普通事件中断、终态回调拒绝）、CRLF/multiline data、事件数量、单事件累计字节、总输入字节上限、终态后事件拒绝回归测试，以及 Structured Output 非法 JSON/空输出/incomplete 边界测试（待执行）；完整矩阵仍待执行；Platform 受控流读取尚未提供；local_validation_only、Usage 诊断码、Capability verification_method 与 capability_ttl_ms 已纳入 Mapper/DTO

只负责 Go 侧模型服务协议适配，复用现有独立模型服务；不重建推理 Gateway，不管理研究状态机、工具权限或业务结论。

设计文档：[01-模型适配与工具边界设计](01-模型适配与工具边界设计.md)；协议调查：[03-现有模型Gateway协议盘点](03-现有模型Gateway协议盘点.md)；Port/DTO 契约：[04-ProviderPort与DTO字段级契约](04-ProviderPort与DTO字段级契约.md)；JSON/SSE Fixture 清单：[05-JSON与SSE契约Fixture清单](05-JSON与SSE契约Fixture清单.md)；Provider/Responses 实现边界补充：[06-Provider与Responses协议实现边界补充](06-Provider与Responses协议实现边界补充.md)；机器契约：[schema/](schema/)。

目标结构：
```text
internal/llm/
  adapter/ protocol/ output/ streaming/
```

职责：Provider/Model Adapter、协议转换、超时/限流/错误分类、结构化输出、流式响应、用量/延迟元数据和脱敏。Tool Registry/白名单/权限/预算由 Researchflow 管理；LLM 不执行模型工具调用。

详细任务见 [02-任务拆分与验收](02-任务拆分与验收.md)。


## 集成验证记录（2026-11-09）

- LLM 已通过 PR #44 合入 `integration`，Merge Commit：`018ad759e239e46bffd1cac6b320adf925cf579a`。
- PR #44 的 Go test suite 已触发但失败：根模块在全仓 gofmt 阶段被 `internal/research/context/validate.go` 的非法转义及 `internal/research/evidence/authorization.go` 的 NUL 字符阻断；因此根模块 `go test ./...`、`go test -race ./...`、`go vet ./...` 均未执行，不能据此判断 LLM 包测试通过或失败。
- 同一工作流中 `services/market-data` 另有 `TestProviderProbeDefaultDailyBarsExcludeCurrentIncompleteDay` 失败；`go-core` 测试通过。上述失败不属于 LLM 文件范围，本分支不越权修改。
- 新增 LLM-002-K：拒绝 SSE 已发送文本增量与终态输出文本不一致；代码提交为 `18b34850f3ae1ef1e05989722685e61759949f23`，待包级构建验证。
- Platform `httpclient.Client.Do` 仍会缓冲完整响应体，当前 integration 的 `internal/platform/httpclient` 仍只有缓冲式 `BoundDoer.Do`，未提供受控流式 Body API；LLM-005 网络 Stream 继续等待 Platform 流式能力，不自建 HTTP 网络客户端。


## 最新 LLM 静态 Review（2026-11-09）

- SSE 文本累计由单一全局字符串改为按 `output_index + content_index` 分段聚合，最终按索引顺序生成 ModelResponse.output_text；避免多个输出项交错到达时互相误判或覆盖。
- 每个文本分段独立校验 delta 与 done；done 后再收到同一分段 delta 返回 `llm.sse.output_text_after_done`；负索引返回 `llm.sse.invalid_output_text_index`。
- 新增多输出项交错事件聚合、文本 done 后 delta 拒绝、函数参数 done 后 delta 拒绝测试。
- Function Call 参数也按终态约束：收到 `response.function_call_arguments.done` 或对应 `response.output_item.done` 后，不再接受同一 call_id 的后续参数 delta；违规返回 `llm.sse.function_arguments_after_done`。
- 新增 `TestParseResponsesSSERejectsFunctionArgumentDeltaAfterDone`，覆盖 arguments.done 后追加 delta 的协议异常。

- 本轮代码提交：`722901c`、`cbf314b`；当前仍无该最新 HEAD 的独立包级测试证据。
- 最近一次已完成 CI（提交 `b0e64f5`）中，根模块因全仓 gofmt 检查失败而跳过 go test/race/vet；失败文件不属于 LLM。go-core 测试/race/vet 通过，但不覆盖根模块 LLM 包。Market-data 单测和 market-data Docker 镜像构建另有失败；News/Market-data MariaDB 集成通过。
