# FLOW-CTR-004 Stage Provider Result Envelope / Output Registry 影响分析（提案）

- 文档状态：只读影响分析提案；未批准契约变更，不修改冻结契约、Schema、Golden 或 Go 代码。
- 核对日期：2026-11-16
- 仓库/分支：`Kasign/stock_data_provider` / `integration`
- 核对 HEAD：`17ca61fad64e87da8c557f80bdfe66cc71b0bfcd`
- 权威基线：`doc/contract/PLAT-002-项目级契约冻结登记簿.md` 中 FLOW-CTR-004 = `frozen`；冻结语义不得由实现盘点反向修改。

## 1. 结论摘要

FLOW-CTR-004 不是完全没有代码，而是**执行结果的基础状态与引用模型已经存在，Stage Provider Result 的机器契约、版本化 Output Registry 和生产校验/装配链仍未闭合**。

当前不可安全启用生产 Stage Provider。原因不是缺一个简单接口，而是以下链条尚未形成权威闭环：

`template_version + output_registry_version`
→ `stage_code` 对应的唯一输出类型/Schema
→ Provider 返回 Envelope
→ 强类型 Payload Validator
→ Quality Summary 语义校验
→ 可信 CallerRef / 下游 Adapter
→ Stage/Attempt 结果原子提交
→ ResultSummary 的质量与覆盖率可解释输出。

本提案仅记录差距、责任边界和实施顺序，不创建假 Schema、不猜测质量枚举、不填造 coverage 分母，也不把现有 Quant Validator 冒充 Stage Output Registry。

## 2. 冻结语义（不得在本提案中改写）

Researchflow 的权威字段定义见 `doc/researchflow/03-CONTRACT-006与007最终决议.md` 第 6.2 节及 `doc/researchflow/04-ApplicationPort与运行时契约落地补充.md` 第 5 节。Stage Provider Result 至少包括：

| 字段 | 责任方 | 冻结语义 |
|---|---|---|
| run_ref / stage_ref / attempt_ref | Orchestrator | 执行边界注入，Provider 不得输入或覆盖 |
| stage_code / template_version | Orchestrator | 必须与已解析模板快照一致 |
| output_type / schema_version | Domain Provider | 必须在 Provider Output Registry 注册 |
| payload | Domain Provider | 按 output_type + schema_version 对应 Schema 校验，不允许任意 map |
| quality_summary | Domain Provider | typed object；质量语义由领域生产方定义，不映射 Stage status |
| diagnostic_refs | Platform/Domain | 不透明引用 |
| completed_at | Orchestrator | 执行边界完成时间，RFC3339Nano UTC |

另外，FLOW-CTR-004 对 ResultSummary 还要求质量状态、coverage 和 Summary 资源可关联到具体结果。具体枚举、分母口径、资源身份/版本绑定如果尚未由权威决议完整定义，必须走正式契约澄清/变更，不得由 Integration 自行推断。

## 3. 当前代码与 Schema 事实

### 3.1 Stage 执行结果模型

`internal/researchflow/application/executor.go`：

- `StageExecutionRequest` 包含 Run、Stage、Attempt、参数、输入快照、system_as_of、deadline、correlation_id。
- 请求中没有可信 Principal / CallerRef；CreateRun 的 `RequestIdentity.AuthenticatedCallerRef` 在创建时使用，但当前 Stage 执行请求未携带该可信主体。
- `StageExecutionResult` 只有 Attempt/Stage 状态、output_ref、quality_summary_ref、diagnostic_refs、reason_codes、自由字符串 quality_status / risk_decision、fault_ref、retry_at。
- 缺少 output_type、schema_version、payload、typed quality_summary 和 completed_at；因此现有结果类型不是冻结的 Stage Provider Result Envelope。

`ValidateStageExecutionResult` 只检查状态组合、引用非空和诊断/原因字段非空，不负责 Envelope discriminator、Payload Schema、质量对象和版本匹配校验。

### 3.2 Template 与 Output Registry

`internal/researchflow/template/template.go` 中：

- `WorkflowTemplate.OutputRegistryVersion` 必填，并进入 ResolvedTemplate 快照。
- 该值目前只是不可变模板中的字符串标识。
- 仓库没有按 `(output_type, schema_version)` 注册 Stage Provider Payload Validator 的版本化 Output Registry 实例，也没有将 `stage_code + template_version + output_registry_version` 映射到完整注册项的生产加载/校验链。

因此“模板记录了 OutputRegistryVersion”不等于“对应版本的注册表存在、完整且已被运行时使用”。

### 3.3 现有 Quant Registry 不可替代 Stage Output Registry

`internal/researchflow/resultvalidation/quant.go` 的 `Registry`：

- discriminator 是 `result_type + schema_version`；
- 用于 Researchflow 消费 Quant Result Envelope；
- 未注册类型/版本 fail-closed；
- 它不是 Stage Provider Output Registry，不能直接用于所有领域 Stage 的输出类型、Stage Code 适用范围和质量摘要校验。

Quant 侧 `internal/quant/registry` 及 `doc/quant/schema/` 已有部分 QNT-CTR-004 Envelope、Indicator、Factor Metric、Trend、Risk Schema/测试；但这只证明局部 DTO/Schema 资产存在，不证明生产 Stage Provider 已存在。QNT-CTR-003 Factor 输入/结果完整定义及 DATA-CTR-005/007 行情精度、SnapshotRef 仍是 deferred 依赖。

### 3.4 ResultSummary 机器约束不足

- `internal/researchflow/domain/types.go` 和 `internal/researchflow/publicdto/dto.go` 的 `QualityStatus` 均为自由字符串。
- `doc/researchflow/schema/result-summary-1.0.0.schema.json` 只要求 `quality_status` 为非空 string，没有枚举约束。
- `coverage` 仅约束为 0..1 数字，没有定义可审计的 numerator / denominator、纳入/排除 Stage 规则或计算版本。
- 当前 ResultSummary Schema 没有独立的 Summary resource identity/version 内容绑定字段；Run 只通过 result_summary_ref 指向 Summary 资源。

这些缺口涉及冻结契约字段的机器化程度和权威口径。尤其质量枚举、coverage 分母及资源身份/版本绑定不得由实现者自行发明；须由契约 Owner 给出定点决议，必要时走版本升级。

### 3.5 Server / 生产 Provider

- `internal/server/bootstrap/bootstrap.go` 当前没有装配 Researchflow Scheduler、WorkerPool 和生产 StageExecutor。
- `internal/researchflow/application/executor.go` 只有 StageExecutor Port，没有生产实现。
- Data、News、Research、Quant、LLM 的已有 Application Facade/Adapter 是模块自身能力，不自动构成 Stage Provider；必须由各模块提供符合 Stage 输入/输出契约的生产 Application Port，再由 Server 组合。
- Notification 不属于同步 DAG Stage Provider；其 Outbox/Inbox 链路不应纳入 Stage Output Registry 的伪造注册项。

## 4. 缺口责任归属

| 缺口 | 责任方 | Integration 可做什么 |
|---|---|---|
| Stage Provider Result Envelope Schema / Go DTO / Golden | Researchflow + 契约 Owner | 形成提案、核对冻结字段、实施正式批准的机器化定义 |
| Output Registry 版本、注册项、Validator 装载与不可变性 | Researchflow | 明确 Registry Port/实现和精确注册语义；不得预填无 Provider 的虚假注册项 |
| quality_status 枚举、coverage 分子/分母、Summary 身份/版本绑定 | 契约 Owner + Researchflow | 先出字段口径提案；未决前不写死枚举/分母 |
| 每个 Stage 的真实 output_type/schema_version/payload/quality_summary | 对应领域模块 | 提供真实 Facade、领域 Schema/Golden、质量口径与生产 Provider |
| trusted CallerRef / Principal 运行时传播 | Server + Researchflow + Platform | 使用可信认证 Context 注入；不从普通 Header/DTO 取值 |
| StageExecutor 组合与生命周期 | Server | 在前述 Provider/Registry/Principal 门禁齐备后装配，不创建空壳 |
| 跨模块 Contract Test / MariaDB E2E | Integration | 最后执行真实 CreateRun→Schedule→Worker→StageExecutor→CommitAttemptResult |

## 5. 推荐实施顺序与进入门禁

1. **契约定点澄清**：由契约 Owner 对质量状态枚举、coverage 分母、Summary 资源身份/版本绑定逐项给出权威定义；若变更冻结语义，先完成 DEC、消费者影响确认和 Schema 版本升级。
2. **Envelope 机器定义**：按已冻结字段创建 Stage Provider Result Envelope Schema/Golden/Go DTO；拒绝额外字段、重复 JSON key、未知 discriminator 和不匹配的 Schema 版本。
3. **Output Registry**：建立不可变版本化 Registry，注册项至少绑定 stage_code、output_type、schema_version、payload_schema_ref、validator、quality_summary_schema_ref/validator；Registry 版本和注册内容需有稳定 fingerprint。以上注册项必须来自真实领域 Provider，不得占位。
4. **执行边界校验**：Worker 在提交前校验 Orchestrator 注入的 run/stage/attempt/stage_code/template_version/completed_at 与当前执行快照一致；Provider 不得覆盖身份字段；按精确 output_type/schema_version 验证 payload 和 typed quality_summary。
5. **CallerRef 传播**：从可信 Server Principal 生成/传递 caller_ref，验证与 Run 创建主体及下游 AuthorizationRef.subject_ref 一致；不得把 API Key、普通 Header 或 Provider 自报字段当作主体。
6. **领域 Provider 接入**：按实际可用依赖逐个接入 Data/News/Quant/Research/LLM；每个 Stage 都必须有真实用例和可读取的输出，不以 Mock/空实现证明生产能力。Quant 的 Factor Stage 受 DATA-CTR-005/007 与 QNT-CTR-003 阻断，不得先注册。
7. **ResultSummary 机器校验**：将经正式确认的 quality_status 枚举、coverage 计算口径和 Summary 身份/版本绑定落到 Schema/Golden/Validator；保留 Stage outcomes、excluded/missing stages 与 reason_codes 的可追溯关系。
8. **跨模块验收**：使用真实 MariaDB 执行 CreateRun→Schedule→Worker→StageExecutor→CommitAttemptResult；覆盖未注册类型/版本、Schema 不匹配、质量摘要无效、身份字段篡改、CallerRef 错配、ResultSummary 质量/coverage 错误和结果原子提交/回滚。取得精确 SHA CI success 后才将 Stage 生产接入状态改为 pass。

## 6. 当前结论与下一项任务

- 当前 FLOW-CTR-004 仍为 `frozen`；本提案不修改登记簿状态。
- 当前 Researchflow Stage 生产执行保持 `blocked`，不是 fail；现有状态机、Worker、Scheduler、MariaDB 模块验收不重复执行。
- 目前最先可执行的下一项不是 Server 接线，而是**契约定点澄清与 Envelope/Registry 机器定义**。但其中质量枚举、coverage 分母和 Summary 身份/版本绑定需先得到正式口径；在决议前不写入 Schema/Golden。
- Integration 本轮完成的是影响分析，不代表生产 Stage 进度已解锁。若要继续实施，应先授权/确认本提案第 5 节第 1 项的契约定点决议；否则保持 blocked，不重复只读盘点。
