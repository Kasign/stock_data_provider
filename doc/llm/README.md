# LLM 模块

- 分支：`feat/llm`
- 代码范围：`internal/llm/**`
- 文档范围：`doc/llm/**`
- 任务前缀：LLM
- 当前代码状态：Provider Port、DTO、Responses JSON Mapper/Parser、非流式 Adapter、SSE Reader/Stream Adapter、Draft 2020-12 Schema 编译与 Responses strict 子集门禁均已实现；本轮 INT-A 静态接入核对未发现 LLM 模块侧需要新增的接口或 Adapter。
- 当前模块分支代码核对基线：`501fc08da4782d907199fc03bdf0b181a1cf2346`；其后文档提交不改变该代码基线。另：Integration CI Run `43561072301` 对代码 SHA `b0aabaa2edf89c44cd2aeb79b1a57210a8017d1b` 执行根模块 `go test ./...` 与 `go vet ./...` success，因 `internal/llm` 属根模块，已覆盖该 SHA 的 LLM 普通测试与 Vet；本次 Workflow 未运行 LLM 专项 Race，也未执行真实 Gateway E2E。该结果不改变 LLM 模块分支代码验收基线。
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

1. **Server 组合根接线（blocked，非本分支范围）**：创建固定目标 BoundDoer；将 `BoundDoer.Do` 注入 `HTTPDoer`；包装 `BoundDoer.OpenStream` 返回值为 `HTTPStreamResponse`；解析并注入 SecretRef、ProviderConfig、CapabilityState 和 Clock；再将 `llm.Provider` 注入 Researchflow。
2. **真实 Gateway 联调（not_run）**：执行 Generate、SSE、取消、超限、错误响应和 Body 关闭 E2E；未验证能力继续保持 unknown。
3. **当前 SHA 自动化验证**：Integration Run `43561072301` 已对 SHA `b0aabaa2e` 覆盖 LLM 普通 `go test ./...` 与 `go vet ./...`；LLM 专项 `go test -race ./internal/llm/...` 仍无当前 SHA 证据。普通测试/Vet 通过不代表真实 Gateway 能力通过。

INT-A 的 LLM 模块侧接入核对及 Server 精确接线要求见 [02-任务拆分与验收](02-任务拆分与验收.md) 的“2026-11-13 跨模块联调：LLM 接入面核对（INT-A）”。

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

## Integration 接入状态（2026-11-14）

LLM 生产 Provider 装配当前为 `blocked`：`ProviderConfig` / `CapabilityState` / `ResponsesAdapter` 代码已存在，但 CapabilityState 必须绑定真实 Gateway 版本、验证时间、有效期和 evidence_ref；现有 Gateway 盘点明确没有真实请求、没有读取 API Key，也没有可核验的运行版本/能力证据。Server Bootstrap 尚未注入 BoundDoer、Secret、ProviderConfig、CapabilityState 和 Researchflow Provider。Integration Run `43561072301` 已覆盖根模块 LLM 普通测试与 Vet，未覆盖 LLM 专项 Race 或 Gateway E2E。没有真实证据前保持能力 `unknown`，不创建生产 Provider。
