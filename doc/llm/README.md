# LLM 模块

- 分支：`feat/llm`
- 代码范围：`internal/llm/**`
- 文档范围：`doc/llm/**`
- 任务前缀：LLM
- 当前代码状态：Provider Port、DTO、Responses JSON Mapper/Parser、非流式 Adapter、SSE Reader/Stream Adapter、Draft 2020-12 Schema 编译与 Responses strict 子集门禁均已实现；当前没有已确认的 LLM 核心业务代码缺口。
- 当前模块验证状态：当前 HEAD `f36721efcea2385b83eb1833f62122fde473e307` 尚无对应 CI 运行。最近一次相关 Run `42989240185` 对应旧 SHA `18e4505f1ce2cecffa84218e95eca5f8a09e6d67`；根模块 gofmt 因 Notification 的 13 个文件失败，失败清单不含 `internal/llm/**`，根模块测试被跳过；`go-core` 测试/race/vet 通过，但不覆盖 LLM；Docker analysis-api 构建失败。LLM 包级 go test、race、vet 仍无成功执行证据。
- PR #45 已于 2026-11-09 合并到 `integration`，合并提交：`2eeea1c44a3268fc98bbd4c99dc1bf5e464f8814`。该合并提交不等于当前 `feat/llm` HEAD 已有独立 CI 通过证据。
- 当前 `integration` 已包含 Platform `BoundDoer.OpenStream` / `StreamResponse` 代码；LLM 已实现窄接口和 Stream Adapter。剩余接线责任是 Server 组合根将 Platform 流式响应包装为 LLM 的 `HTTPStreamResponse`，不得在 LLM 复制 HTTP Client 或目的地址策略。
- LLM 不创建 HTTP Client、不实现 SSRF/目标地址策略、不自行重试；网络传输和目的地址安全由 Platform 提供，组合根包装由 Server 负责。

## 职责边界

只负责 Go 侧模型服务协议适配，复用现有独立模型 Gateway；不重建推理 Gateway，不管理研究状态机、工具权限或业务结论。

- Provider/Model Adapter、协议转换、deadline/取消处理、限流与错误归一化。
- JSON Schema 本地编译、上游 strict 子集约束与结构化输出校验。
- Responses JSON/SSE 解析、流式消费、资源上限和 Body 生命周期。
- Usage/延迟元数据、能力证据门禁和错误脱敏。
- Tool Registry、白名单、权限和预算由 Researchflow 管理；LLM 不执行模型工具调用。
- Adapter 单次调用；只返回稳定错误类型、`retryable` 与 `retry_after_seconds` 提示，不决定重试。

## 当前剩余验收

1. 在全仓 gofmt 阻断解除后，实际执行并记录 LLM 包级 `go test`、`go test -race`、`go vet`。
2. 由 Server 组合根接入 Platform `BoundDoer.OpenStream`，完成跨模块编译和流式传输接线。
3. 对真实本地 Gateway 执行 Generate、SSE、多文本分段、Function Call、取消、超限和 Body 关闭 E2E。
4. 按 Provider/Model/Gateway 版本记录能力证据；未验证能力保持 unknown，不能仅凭路由可达启用。

## 文档索引

- [01-模型适配与工具边界设计](01-模型适配与工具边界设计.md)
- [02-任务拆分与验收](02-任务拆分与验收.md)
- [03-现有模型Gateway协议盘点](03-现有模型Gateway协议盘点.md)
- [04-ProviderPort与DTO字段级契约](04-ProviderPort与DTO字段级契约.md)
- [05-JSON与SSE契约Fixture清单](05-JSON与SSE契约Fixture清单.md)
- [06-Provider与Responses协议实现边界补充](06-Provider与Responses协议实现边界补充.md)
- [机器契约 Schema](schema/)

## 代码结构

```text
internal/llm/
  llm.go
  responses_*.go
  schema*.go
  testdata/
```

详细任务、历史实现记录和当前验收证据以 [02-任务拆分与验收](02-任务拆分与验收.md) 为准。
