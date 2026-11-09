# LLM 模块

- 分支：`feat/llm`
- 代码范围：`internal/llm/**`
- 文档范围：`doc/llm/**`
- 任务前缀：LLM
- 当前代码状态：Provider Port、DTO、Responses JSON Mapper/Parser、非流式 Adapter、SSE Reader/Stream Adapter、Schema 编译与 strict 子集门禁均已实现；当前没有已确认的 LLM 代码缺口需要通过猜测性功能补齐。
- 当前验收状态：LLM 自身 gofmt 已通过最近一次相关 CI 的文件清单核对；LLM 包级 `go test`、`go test -race`、`go vet` 尚无成功执行证据；真实 Gateway E2E 尚未完成。
- PR #45（修正 LLM SSE 终态文本一致性）已于 2026-11-09 合并到 `integration`，合并提交：`2eeea1c44a3268fc98bbd4c99dc1bf5e464f8814`。
- 分支当前 HEAD：`20a2232579977fe5605f22f699b3a443afed953e`；该提交仅更新本目录验收文档，尚无对应 CI 运行记录。
- LLM 不创建 HTTP Client、不实现 SSRF/目标地址策略、不自行重试；网络传输和目的地址安全由 Platform 提供，组合根包装由 Server 负责。

## 职责边界

只负责 Go 侧模型服务协议适配，复用现有独立模型 Gateway；不重建推理 Gateway，不管理研究状态机、工具权限或业务结论。

- Provider/Model Adapter、协议转换、超时与错误分类。
- JSON Schema 本地编译、上游 strict 子集约束与结构化输出校验。
- Responses JSON/SSE 解析、流式消费、资源上限和 Body 生命周期。
- Usage/延迟元数据、能力证据门禁和错误脱敏。
- Tool Registry、白名单、权限和预算由 Researchflow 管理；LLM 不执行模型工具调用。

## 当前阻塞与依赖

1. 根模块最近一次相关 CI（Run `42567071019`）在全仓 gofmt 门禁失败，失败文件位于 News、Notification、Quant、Research、Researchflow 等其他模块；根模块依赖下载、LLM 包级测试、race、vet 均未执行。本分支不越权修改其他模块。
2. Platform 分支已实现并专项验收 `BoundDoer.OpenStream`（PLAT-CTR-005 A1），但当前 `integration` 的 `internal/platform/httpclient` 仍未包含该流式 API；需由 Platform/Server 后续集成完成组合根包装，不能在 LLM 分支复制 Platform 能力。
3. 本地 Gateway 的 Generate、SSE、取消、响应超限及模型能力证据 E2E 尚未验收。Capability 默认 unknown；不能仅凭 Gateway 路由可用就标记模型能力 supported。

## 下一步验收顺序

1. 等全仓 gofmt 阻断由对应模块修复后，执行根模块 `go test ./...`、`go test -race ./...`、`go vet ./...`，单独记录 LLM 包结果。
2. 由 Server 组合根适配 Platform `BoundDoer.OpenStream`，完成跨模块编译与流式传输接线。
3. 对真实本地 Gateway 执行 Generate、SSE、多文本分段、Function Call、取消、超限和 Body 关闭 E2E。
4. 按 Provider/Model/Gateway 版本记录能力证据；未验证能力保持 unknown/unsupported，不放宽 fail-closed 门禁。

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

详细任务与历史证据以 [02-任务拆分与验收](02-任务拆分与验收.md) 为准。
