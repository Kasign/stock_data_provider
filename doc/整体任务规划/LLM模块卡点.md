# LLM 模块卡点

更新时间：2026-11-18  
所属模块：LLM  
维护分支：`feat/llm`  
模块代码范围：`internal/llm/**`  
模块任务入口：`doc/llm/模块总任务规划和进度.md`  
当前任务：LLM-012「真实 Gateway 能力验收」

## 1. 卡点结论

<badge>BLOCKED</badge>

LLM 模块的 Provider Port、Responses JSON/SSE Adapter、Structured Output Schema 校验、Usage/错误归一化、Context Deadline/取消和资源边界等模块内实现已存在；当前代码 Review 和自动化验收已完成。

当前不能完成真实 Gateway 能力验收，原因不是缺少一套新的 LLM Adapter，而是生产运行时链路尚未闭环：

1. Server 尚未把真实 Gateway 配置、SecretRef、固定目标 BoundDoer 和 Capability Evidence 装配成生产 Provider。
2. Researchflow 尚未出现实际消费 `llm.Provider` 的生产 Stage 接线。
3. 缺少与具体 Gateway/模型版本绑定的真实请求、响应和能力证据。

因此，Generate、Structured Output、Responses SSE、Usage、取消/超时、请求/响应超限及 429/5xx/认证错误等真实上游能力均不能判为 PASS。当前状态应保持 NOT_RUN；在缺少版本与有效证据时，比较结论为 NOT_COMPARABLE。Mock、Fixture、静态 Adapter 测试和 Platform 层通过结果均不能替代真实 Gateway 验收。

## 2. 原因与影响

| 卡点 | 当前事实 | 直接影响 |
|---|---|---|
| Server 生产装配未完成 | `doc/server/模块总任务规划和进度.md` 中 SRV-010 为 BLOCKED；虽有 `internal/server/llmtransport/transport.go` 薄包装，但未形成 Bootstrap 到 Researchflow 的生产 Provider 注入闭环 | LLM Provider 没有受控、可复现的生产调用入口 |
| Gateway 能力证据缺失 | 未提供当前 Gateway/模型版本、有效 SecretRef、能力验证记录及 Evidence 引用 | 无法确认上游是否真实支持 JSON/Structured Output、SSE、Usage、取消、超时和资源限制 |
| Researchflow 消费者未接线 | Researchflow 当前生产 Stage 基线包含 News Search；其规划将 LLM Provider 列为依赖，当前任务仍等待真实 LLM 调用证据 | 即使 Server 构造 Provider，也不能单凭构造成功证明业务 Stage 已使用它 |
| 端到端验收尚未发生 | 当前 LLM CI 证明的是本地代码与测试通过，不包含本轮真实 Gateway 请求 | 不能宣称模型调用链路或生产能力已联调完成 |

## 3. 责任边界

| 责任方 | 必须完成的事项 | 不应转嫁的事项 |
|---|---|---|
| Server（SRV-010） | 从受控配置解析 SecretRef；创建共享 Platform HTTP Client 和固定目标 BoundDoer；包装流式响应并保持 Body 所有权；构造 LLM Provider；注入真实 CapabilityState、Clock；接入 Bootstrap/Lifecycle，并明确关闭顺序 | 不要求 LLM 模块自行读取环境变量、管理 Secret 或创建网络客户端 |
| LLM（LLM-012） | 在生产调用入口具备后，验证 Provider 对真实 Gateway 的协议行为；按能力矩阵记录版本、请求条件、响应证据和 PASS/DEGRADED/FAIL/NOT_COMPARABLE 结论；发现 LLM 自身缺陷时回到 `feat/llm` 修复 | 不负责 Server 组合根、Researchflow 业务重试或工具授权 |
| Researchflow | 在具备正式 Provider 和业务 Stage 契约后，创建真实消费者 Adapter/Stage，负责预算、工具权限、业务重试和流程状态 | 不复制 LLM 协议解析和 Provider Adapter |
| Platform | 提供既有 HTTP Client、BoundDoer、Fault、Secret、Clock 等公共原语 | 不因 LLM 单模块卡点重复建设 HTTP Client、SSRF 或 Fault |

## 4. 卡点解除条件

以下条件必须同时满足，才将 LLM-012 从 BLOCKED 转为可执行：

- [ ] Server 完成 SRV-010：生产 Bootstrap 能构造并注入 LLM Provider，不只是存在 transport 辅助函数。
- [ ] Secret 通过受控 SecretRef 注入；凭据不进入仓库、普通配置输出或日志。
- [ ] Gateway 目标通过固定 DestinationPolicy/BoundDoer 约束；不得由调用参数任意指定目标 URL。
- [ ] 提供 Gateway 版本、模型 ID、验证时间、能力快照有效期和 Evidence 引用。
- [ ] Researchflow 提供实际消费者调用路径，或提供 Server 所有、受控且可重复执行的真实 Gateway 验收入口。
- [ ] 能在不泄露凭据的前提下执行成功、失败、取消、超时和资源限制场景，并保存可审计结果。

## 5. 恢复执行顺序

1. Server Owner 先完成 SRV-010 生产装配，并在 Server 当前任务文档中记录精确代码 SHA 与验收证据。
2. Researchflow Owner 明确第一个真实 LLM 消费场景及 Stage 输入/输出契约；没有业务场景时，不创建占位 Stage。
3. 提供 Gateway/模型版本和 Capability Evidence，确认测试环境、凭据与网络策略可用。
4. LLM Owner 恢复 LLM-012，按下表逐项真实验收。
5. 若失败属于 LLM Adapter/DTO/错误映射等模块内缺陷，在 `feat/llm` 修复并对精确 SHA 执行测试；若属于装配、凭据或出站策略，退回 Server/Platform 责任方。

## 6. LLM-012 真实验收矩阵

| 能力 | 当前状态 | 必需证据 |
|---|---|---|
| Responses Generate | NOT_RUN | 真实请求、完整响应、Gateway/模型版本 |
| Structured Output | NOT_RUN | 真实 Schema 请求、非法输出与截断边界 |
| Responses SSE | NOT_RUN | 真实事件序列、终态、EOF/断连行为 |
| Usage | NOT_RUN | 上游真实 Usage 字段及缺失/不一致行为 |
| 取消/超时 | NOT_RUN | 请求取消、Deadline、资源释放证据 |
| 请求/响应超限 | NOT_RUN | Platform 与 LLM 两层上限及 Fault 映射 |
| 429/5xx/认证错误 | NOT_RUN | 真实状态码、Retry-After 和稳定错误映射 |

诊断结论只能使用 PASS、DEGRADED、FAIL、NOT_COMPARABLE。缺少真实部署版本或证据时使用 NOT_COMPARABLE；没有执行过的项目记录为 NOT_RUN，不得将其描述为通过。

## 7. 当前模块可继续与不可继续的工作

**已完成、无需重复：**
- LLM-009 文档结构治理。
- LLM-010 主要实现/测试路径专项 Review；已修复流式 Body 超限 Fault 映射缺陷。
- LLM-011 精确代码基线自动化验收：Run 44285420286 对应 SHA `19356c7371d2267383e6665c9f220a7e6dc0bcd1`，根模块与 go-core 的 gofmt、go test、race、vet 通过。

**当前不应开展：**
- 不新增重复 Provider、Adapter、HTTP Client 或 Schema Parser。
- 不创建 Mock Gateway 来替代真实上游验收。
- 不提前实施 LLM-008 多模型路由/降级。
- 不修改冻结的 LLM Schema 1.0.0 或契约语义来绕过当前卡点。
- 不在 `feat/llm` 修改 Server、Researchflow 或 Platform 代码。

**下一步：** 等待 SRV-010 生产装配与真实 Gateway Evidence；期间只接受能直接降低 LLM-012 验收风险的模块内缺陷修复，不把等待包装成新功能开发。

## 8. 关联文档

- [LLM 模块总任务规划和进度](../llm/模块总任务规划和进度.md)
- [LLM 当前任务状态和进度](../llm/当前任务状态和进度.md)
- [Server 模块总任务规划和进度](../server/模块总任务规划和进度.md)
- [Researchflow 模块总任务规划和进度](../researchflow/模块总任务规划和进度.md)
