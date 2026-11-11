# LLM 模块

- 分支：`feat/llm`
- 代码：`internal/llm/**`
- 任务前缀：LLM
- 状态：LLM-002 DTO/Provider Port、Responses JSON Mapper/Parser、非流式 HTTP Adapter、Clock/Deadline 注入、SSE Reader Parser/Stream Adapter、公共 DTO Draft 2020-12 Schema 已实现；整体仍待 LLM 包级构建验证及真实 Gateway E2E
- 最近代码提交：以 `feat/llm` 当前 HEAD 为准
- LLM-001：静态协议盘点完成；在线真实端点验收未执行
- LLM-002：DTO/Provider Port、Responses JSON 请求映射/响应解析、非流式 HTTP Adapter 及对应单测已实现，待 gofmt/go test；Platform Fault 映射、Draft 2020-12 Schema 编译及 DTO Schema Contract Test 均待构建验证；JSON/SSE Golden 已扩展覆盖拒绝、未知项、Usage 边界、终态和序号异常；新增完整 Request Mapper Golden，覆盖 sampling、reasoning、strict schema、function tool 与 tool_choice；新增 Adapter 能力门禁测试，验证可选能力证据缺失时不会派发请求；已补充 SSE callback error（普通事件中断、终态回调拒绝）、CRLF/multiline data、事件数量、单事件累计字节、总输入字节上限、终态后事件拒绝回归测试，以及 Structured Output 非法 JSON/空输出/incomplete 边界测试（待执行）；完整矩阵仍待执行；Platform A1 已提供 `BoundDoer.OpenStream`，LLM A2 接入代码已实现但待包级验证和真实 E2E；local_validation_only、Usage 诊断码、Capability verification_method 与 capability_ttl_ms 已纳入 Mapper/DTO

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
- 文本分段只在终态/错误收尾时统一排序聚合，避免每个事件都排序造成高事件数场景下的重复 CPU 开销。
- Function Call 参数也按终态约束：收到 `response.function_call_arguments.done` 或对应 `response.output_item.done` 后，不再接受同一 call_id 的后续参数 delta；违规返回 `llm.sse.function_arguments_after_done`。
- 新增 `TestParseResponsesSSERejectsFunctionArgumentDeltaAfterDone`，覆盖 arguments.done 后追加 delta 的协议异常。

- 本轮代码提交：`722901c`、`cbf314b`；当前仍无该最新 HEAD 的独立包级测试证据。
- 最近一次已完成 CI（提交 `b0e64f5`）中，根模块因全仓 gofmt 检查失败而跳过 go test/race/vet；失败文件不属于 LLM。go-core 测试/race/vet 通过，但不覆盖根模块 LLM 包。Market-data 单测和 market-data Docker 镜像构建另有失败；News/Market-data MariaDB 集成通过。


## LLM Go 格式检查与全仓验证状态（2026-11-09）

- 已依据 PR #45 最新 CI 的 `gofmt -l ./cmd ./internal` 输出，将 `internal/llm/**` 中 10 个未格式化 Go 文件统一格式化，提交：`c244c8d2fbd322b1caaabd72cda1becd2169d1e4`。
- 格式化后最新 CI 的根模块 gofmt 清单已不再包含任何 `internal/llm/**` 文件；但全仓检查仍被 News、Notification、Quant、Research、Researchflow 模块的未格式化文件阻断。
- 因根模块 job 在 gofmt 步骤失败，依赖下载、`go test ./...`、race、vet 均未执行；当前没有 LLM 包级测试通过证据。
- LLM 分支不修改上述模块；待模块负责人修复后再跑根模块完整验证。


## Responses Strict Schema 子集门禁（2026-11-09）

- 本地 Draft 2020-12 Schema 编译与 Responses strict 子集检查分离：本地验证仍使用完整 Draft 2020-12；只有即将发送到上游的 `strict: true` Response Schema / Function Tool 参数 Schema 才执行子集门禁。
- 根 Schema 必须是 object；对象属性必须全部列入 required，并设置 `additionalProperties: false`；根层不允许 anyOf；type 数组只接受一个基础类型与 null 组成的 nullable 联合。
- 支持类型、枚举、anyOf、$defs、本地 $ref、const、items，以及官方列明的字符串/数字/数组约束；未知关键字、allOf/not/条件组合等不支持能力直接拒绝，不做静默改写。
- 应用官方结构限制：对象最多 10 层、对象属性总数最多 5000、属性名/定义名/enum/const 字符总量最多 120000、enum 值最多 1000；超过 250 个字符串 enum 值时，该 enum 字符总量不超过 15000。
- `local_validation_only=true` 不发送上游 Schema，因此跳过 Responses strict 子集门禁，但仍执行本地完整 Schema 编译与响应校验。
- 来源：OpenAI Structured Outputs 文档 https://developers.openai.com/api/docs/guides/structured-outputs （Supported schemas / limitations）。
- 当前仅有静态契约测试，尚未完成本地 Gateway 对该子集的 E2E 验证；细分模型能力继续以 capability evidence 为准。

- 另补充结构预算边界测试及 Adapter 派发前拒绝测试；当前仍等待 CI 执行包级测试。

- 新增 Strict Schema 子集限制边界和 Adapter 派发前拒绝测试；测试文件已按 CI gofmt diff 格式化，等待最新 CI 确认 LLM 文件从全仓 gofmt 清单中移除。

- 高级 Structured Output 约束已拆为独立模型能力 `structured_output_advanced_constraints`；pattern/format/数字边界/数组长度约束默认 fail-closed，需对应模型的有效 capability evidence。

- Usage 明细增加 cached_tokens / reasoning_tokens 的负值与越界诊断，保留上游原值和来源，不伪造修正值。

- 最新 CI 的根模块 gofmt 清单已不再包含任何 `internal/llm/**` 文件；但根模块仍被 News、Notification、Quant、Research、Researchflow 文件阻断，根模块 go test/race/vet 未执行。

- 最新 CI：`42558267262`。根模块 gofmt 清单已不再包含 `internal/llm/**`；当前仍由 News、Notification、Quant、Research、Researchflow 文件阻断，根模块依赖下载、go test、race、vet 均跳过。该 CI 只能证明 LLM 文件格式检查通过，不能证明 LLM 构建/测试通过。

- 高级约束能力门禁覆盖 strict / non-strict 的所有上游 Schema；只有 `local_validation_only` 的 Response Schema 跳过上游能力要求，Function Tool 参数仍然执行门禁。
- 最新 CI：`42558442773` 的根模块 gofmt 清单不含 `internal/llm/**`，但其他模块格式问题仍使根模块测试提前跳过。


## LLM-005 流式接入最新状态（2026-11-09）

- Platform `feat/platform-foundation` 的 PLAT-CTR-005 A1 已通过专项 CI，提供 `BoundDoer.OpenStream`。
- LLM 已实现 `MapResponsesStreamRequestAt`、`ParseResponsesSSEReader`、`HTTPStreamDoer` 和 `ResponsesAdapter.Stream`；Stream 使用同步 callback、受控 Body、固定 SSE Accept、流式请求标志和已有能力证据门禁。
- 组合根需要将 Platform `*httpclient.StreamResponse` 包装为 LLM `HTTPStreamResponse`；当前 integration 基线尚未包含 Platform A1 新 API，不能直接宣称跨分支集成完成。
- 最新代码有 fake transport 回归测试，但 LLM 包级测试尚未执行；待根模块其他模块 gofmt 问题修复后补齐 go test/race/vet，并进行本地 Gateway E2E。


## 最近一次格式检查（2026-11-09）

- CI：`42560516644`。根模块 gofmt 清单已不包含 `internal/llm/**`；仍被其他模块未格式化文件阻断，根模块 go test/race/vet 未执行。
- LLM 新增 Stream Adapter/Reader Parser 与取消门禁回归测试尚无包级执行证据；不标记通过。
