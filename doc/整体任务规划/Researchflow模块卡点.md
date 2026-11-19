# Researchflow 模块卡点

更新时间：2026-11-19  
对应分支：`feat/researchflow`  
最近一次已通过 Researchflow 专项验收 SHA：`555a16726bfa5e48968b7f23b08e1ba6de7706d9`  
最近一次代码变更 SHA：`c7849cf5ad44dfa3a702844c14eb657f864d7512`（ComposeResult 嵌套输入 UTF-8 校验；Researchflow 专项 CI 通过）  
维护范围：记录 Researchflow 推进所依赖的跨模块决策、契约同步和验收阻塞；不替代模块任务状态文档。

## 1. 当前卡点

### RF-BLOCK-001：项目级时间规则修订尚未同步到共同集成基线

- **现状**：项目级契约修订已提交在 `feat/design_doc`，HEAD 为 `bc0d11a3bb067d097195b345a32dbc93bef7525b`。GOV-CTR-002 和 GOV-CTR-004 均升级至 1.1.0，决策记录新增 DEC-PLAT-002-054。
- **原因**：项目级权威契约必须由契约 Owner/Integration 同步，Researchflow 模块分支不能自行修改 `doc/contract/**`。
- **影响**：本模块已按确认的规则方向实现 UTC 输出和数据库时钟；最终统一验收仍需共同基线包含新规则版本。
- **处理责任**：项目契约 Owner / Integration。
- **解除条件**：将 `feat/design_doc` 修订纳入共同集成基线，并确认 GOV-CTR-002 1.1.0、GOV-CTR-004 1.1.0、PLAT-002、README 和变更记录一致。
- **当前状态**：OPEN；不阻断本模块继续开发。

### RF-BLOCK-002：Run Schema 1.0.0 对非终态 Summary 引用的接受范围较宽

- **现状**：Run Schema 1.0.0 要求终态 Run 必须有非空 `result_summary_ref`，但 Schema 未禁止非终态携带非空引用。
- **运行时证据**：Storage `loadResultSummary` 明确校验终态必须有 ref、非终态必须为 null；CreateRun 禁止初始 Summary；Run CAS 禁止写入 Summary/FinishedAt；终态仅由 Finalizer 原子提交。
- **处理结论**：生产路径已 fail-closed。Schema 1.0.0 保留较宽消费者接受范围，不为形式上的 Schema 收紧单独发布 MAJOR；未来如消费者明确要求 Schema 级拒绝，再评估 2.0.0。
- **当前状态**：RESOLVED（生产路径不变量）；Schema 的较宽接受范围是当前兼容策略。

### RF-BLOCK-003：FLOW-CTR-004 Envelope 登记状态与模块资产不一致

- **现状**：Researchflow 分支已有 Stage Provider Result Envelope Schema/Golden、精确 Registry 和 Worker 提交前校验；模块规划记录项目登记簿仍写 Envelope Schema 未登记。
- **原因**：项目级登记簿与模块分支资产的同步责任不在 Researchflow 模块分支。
- **影响**：不能仅凭模块文件存在就宣告项目级登记和跨模块消费者验收全部完成。
- **处理责任**：项目契约 Owner / Integration。
- **解除条件**：核对权威 Schema/Golden 路径、版本、Registry 约束和登记簿记录一致。
- **当前状态**：OPEN；不阻断当前时间规则实现。

### RF-BLOCK-004：Outbox RetryAt 使用进程时钟

- **原问题**：OutboxDispatcher 原先使用 `time.Now()` 计算下一次重试时间，并将其持久化到 MariaDB；该时间由多个 Server/Worker 实例共同用于调度。
- **修复**：`MySQLDatabaseClock` 通过 Platform `time.Service` 包装 MariaDB UTC Source；`MySQLRunRepository` 实现 `DatabaseClock`；Dispatcher 在安排 RetryAt 前读取数据库 UTC 时钟。
- **失败处理**：缺少 DatabaseClock 能力或读取失败时 fail-closed，不写入 retry 时间。
- **测试与验收**：新增固定数据库时钟和时钟不可用时不提交 retry 的测试；Researchflow 模块 CI 已通过。
- **当前状态**：RESOLVED。

### RF-BLOCK-005：FLOW-005 Context Assembly 尚未形成生产链路

- **现状**：Research 模块 `feat/research` 已有 Context Domain、Schema/Golden、Application Port 和 Storage Repository；Researchflow 已有 `contextassembly` 纯规划、Research CreateContext DTO 投影、Tokenizer 接口、最终模型输入组合和 Tool Policy。LLM 模块 `feat/llm` 当前有 ModelRequest/Provider/Schema 能力，但未发现生产 Tokenizer 实现；Server/Worker 尚未将 Context Assembly 接入真实 Run Stage。
- **PIT 绑定缺口**：`ResearchContextSnapshot.SystemAsOf` 当前仅校验非零 UTC，未与 Run.SystemAsOf 比较。Research Context 的 SectionRefs 必须与 Context.SystemAsOf 一致，但调用方仍必须在 CreateContext 前和读取后验证 Context.SystemAsOf 与 Run.SystemAsOf 完全相同。
- **Deadline 绑定缺口**：构造 LLM ModelRequest 时必须直接继承同一 Run 的 SystemAsOf/DeadlineAt；LLM Validate 只能校验 UTC 和时间边界，不能替代 Run 关联一致性验证。
- **授权边界**：Researchflow contextassembly 不获取资源、不签发/复核授权；必须由各资源 Owner 的当前授权 Adapter 获取可进入上下文的正文，且仅把授权后的 included section 内容交给模型。
- **Researchflow 本模块已补齐**：Context Assembly 对 ResearchQuestion、Scope、BuilderVersion、SubjectRefs、QualitySummary.ReasonCodes、候选 ID/Owner/引用、Section Metadata.Version、Tokenizer ModelID/Version 和 SectionContent ID/Text 增加有效 UTF-8 校验，避免 JSON 序列化将无效字节静默替换。代码 SHA `e6e941c71c9960b6675ce846df1b621a16064f06` 的 Run [44311895491](https://github.com/Kasign/stock_data_provider/actions/runs/44311895491) 中 Researchflow format/tests/race/vet、WorkerStore unit/race/vet、MariaDB lifecycle integration 均通过。
- **本轮新增预算输入校验**：`MeasureBudgetReservations` 对 System Prompt 和 Tool Schema 增加 UTF-8 校验；无效字节在 Tokenizer 调用前 fail-closed，并增加两组测试。代码 SHA `cc31402165a35b23e8cce39e8d9e941d9653610f` 的 Run [44436587245](https://github.com/Kasign/stock_data_provider/actions/runs/44436587245) 中 Researchflow format/tests/race/vet、WorkerStore 和 MariaDB lifecycle integration 均通过；根模块 Test/Race/Vet 当时仍在运行。
- **Tool Policy 输入边界补强**：Registry Name/Description/Schema/Scopes、LLM Tool Call、Arguments JSON、IdempotencyKey 和 GrantedScopes 增加 UTF-8 校验；无效输入在 Schema Validator/工具暴露前拒绝。代码 SHA `ebf54dfae766e6c95d0470158f88fb570b1a1901` 的 Run [44437240026](https://github.com/Kasign/stock_data_provider/actions/runs/44437240026) 中 Researchflow format/tests/race/vet、WorkerStore 和 MariaDB lifecycle integration 均通过。
- **Run `44437240026` 最终状态复查（2026-11-19）**：根模块 Test/Race/Vet 均已完成并 success；Researchflow format/tests/race/vet、WorkerStore unit/race/vet、Researchflow MariaDB lifecycle integration 均 success。Market Data、News、Compose/Docker 等相关 Jobs 为 skipped，不计入对应领域的真实集成验收。
- **影响**：现有测试只能证明纯函数预算/映射/JSON 组合逻辑，不能证明生产数据可授权、PIT 正确、Tokenizer 预算准确或真实 LLM 调用成功。
- **处理责任**：Research Owner（Context Facade/授权语义）、LLM Owner（真实 Tokenizer）、Server Owner（组合根/Worker 接入）、Researchflow（适配和 Run Context 绑定）。
- **解除条件**：完成 Owner Adapter 与当前授权；Run/Context/LLM 的 SystemAsOf 和 Deadline 一致性测试；生产 Tokenizer 与模型版本绑定及准确性证据；Server/Worker 真实 Stage 注册和集成测试。
- **当前状态**：OPEN；阻断 FLOW-005 真实生产链路，不阻断 FLOW-001/002 其他工作。

### RF-BLOCK-006：FLOW-008 尚无真实诊断生产者和 Server 注册

- **现状**：Researchflow 的 `NewStageCheck` 只把已提供的 StageAssessment 投影为 Platform Diagnostics.Check；当前未发现生产代码调用该函数，Server bootstrap/composition 也未注册 Researchflow Diagnostics.Check。
- **边界**：该 Adapter 不会自动诊断 Data/News/Quant/Research/LLM，也不产生独立 DiagnosticRecord。它只负责四态映射与安全投影。
- **本轮修复**：CheckedAt 统一转 UTC；Attributes 使用 allowlist（当前仅 `source` 稳定代码），拒绝自由文本、未知 key 和保留字段；StageRef、StageCode、ReasonCode、SafeMessage、EvidenceRef 及 Attribute 键值增加有效 UTF-8 校验，拒绝无效字节，避免 JSON 序列化替换造成诊断内容失真。代码 SHA `384582d37f679d272de708b87b32a6b4c93cf649` 的 Run [44309745636](https://github.com/Kasign/stock_data_provider/actions/runs/44309745636) 中 Researchflow format/tests/race/vet、WorkerStore unit/race/vet、MariaDB lifecycle integration 均通过；Run 总结论 success。
- **影响**：目前只能证明 Adapter 结构和安全投影正确，不能宣称真实模块诊断已接入或运行。
- **处理责任**：各领域 Stage Provider 产生真实 Assessment；Researchflow 维护映射；Server 负责注册/聚合 Platform Checks。
- **解除条件**：至少一个真实 Stage Provider 提供有来源证据的 Assessment，Server 注册 Check 并通过聚合测试；后续按真实 Stage 逐项覆盖。
- **当前状态**：OPEN；只阻断 FLOW-008 端到端诊断验收，不阻断其他模块任务。

## 2. 本模块已继续推进的工作

- CreateRun 通过 DatabaseClock 生成 `system_as_of` 和 `deadline_at`；两者不属于调用方命令体。
- MySQLDatabaseClock 通过 Platform `time.Service` 获取数据库 UTC；Outbox RetryAt 使用同一数据库时钟。
- Run/Stage/Attempt DTO 时间统一规范化 UTC；ResultSummary 的 quality/coverage/actionability 语义已明确并补测试。
- Diagnostics Adapter 已统一 UTC CheckedAt，并限制属性白名单；当前尚无真实 Assessment 生产者和 Server 注册。
- Context Assembly 生产接入所需的 Research Owner 授权、Run PIT/Deadline 绑定、LLM Tokenizer 和 Server/Worker 接线已明确卡点。
- 代码验收 SHA `799384a87a07ea3c63acb1d5a895a3a097cde3f5` 的 Actions Run [44302171395](https://github.com/Kasign/stock_data_provider/actions/runs/44302171395) 为 success；Researchflow module format/tests/race/vet、WorkerStore 单测/Race/Vet、MariaDB lifecycle integration 均通过。该 Run 中 Market Data、News 专项集成 Job 为 skipped，不作为其真实集成验收证据。

## 3. 下一步解除顺序

1. FLOW-002：由 News Owner 修复 Payload Validator 的 uniqueItems 缺口；Researchflow 在集成基线复验。
2. FLOW-002：继续核对真实生产 Template、HardRiskGateRegistry 和 Stage Output Registry；不为缺失 Provider 注册占位项。
3. FLOW-006：明确 RF-BLOCK-008 中 Composer RiskDecision 的唯一 Owner、来源和聚合语义。
4. FLOW-006：确认 RunResult 唯一持久化 Owner，再接通 Quant/Research/LLM Owner Facade 与 Server/Worker。
5. FLOW-005：Research Owner Adapter、Run PIT/Deadline 绑定、LLM Tokenizer 和 Server/Worker Context Stage。
6. FLOW-008：由真实 Stage Provider 提供 Assessment，再由 Server 注册 Platform Diagnostics.Check。
7. 项目契约 Owner/Integration 核对 GOV-CTR-002/004 1.1.0 和 FLOW-CTR-004 Envelope 登记簿。
### RF-BLOCK-007：FLOW-006 Result Composer 尚无生产 Facade 与结果资源持久化

- **现状**：Researchflow 的 Result Composer/Validator 已是纯函数实现；Quant Envelope 的版本/字段错配已修复。当前通过 `NewQuantPayloadRegistry` 精确注册 Quant Envelope 1.0.0 的 trend/risk Payload，并通过 `FromQuantEnvelope` 映射 Quant Typed Envelope。
- **安全边界**：生产调用必须先通过 Quant Application `GetResult`，由 Quant Owner 完成当前授权、Manifest 绑定和读取审计；`FromQuantEnvelope` 本身只验证/映射，不授权。Research/LLM 同样必须使用各自 Owner Facade。
- **Run 绑定**：Composer 要求 Context/Quant/LLM/Risk 的 SystemAsOf 与 Run 完全一致；Server/Worker 必须将同一 Run 的 SystemAsOf/DeadlineAt 传给各 Adapter，不得重新取当前时间。
- **结果持久化缺口**：`ComposedResearchResult` 当前没有持久化 Store/ResultRef。Run 的 `ResultSummaryRef` 只指向执行摘要，不等于完整研究报告。
- **建议方案**：由 Researchflow 持有按 RunRef 唯一关联的 `RunResult` 子资源；只持久化组合输出、LLM 解释和来源引用，不复制 Research/Quant 原始资源；`ResultSummaryRef` 保持执行摘要语义，不增加 Run Schema 1.0.0 字段。该归属需项目架构确认后再实现。
- **影响**：纯函数组合测试通过不代表真实 Quant/Research/LLM 数据、授权、模型调用和 Run 完成后的结果可检索。
- **处理责任**：Quant Owner（GetResult/授权/Manifest）、Research Owner（Context/Evidence/Claim/授权）、LLM Owner（真实模型调用/Tokenizer）、Researchflow（组合与版本校验）、Server Owner（Worker/Adapter/持久化装配）；完整结果 Owner 需项目架构确认。
- **解除条件**：确定唯一结果资源 Owner；Server/Worker 通过各 Owner Facade 获取数据；绑定 Run PIT/Deadline；完成真实 Quant/Research/LLM 组合、持久化、ResultRef 读取和 Contract Test。
- **当前状态**：OPEN；阻断 FLOW-006 生产闭环，不阻断纯函数校验继续完善。

### RF-BLOCK-008：Composer RiskDecision 与 Quant RiskDecision 契约不一致

- **现状**：Quant 的权威结构是 `internal/quant/risk/decision.go`，对应 `doc/quant/schema/QNT-CTR-004-risk-decision-1.0.0.schema.json`；Researchflow 另有 `internal/researchflow/resultvalidation/risk.go` 的 RiskDecision/RiskReason。
- **字段差异**：Researchflow Reason 增加 `rule_ref`、`observed_value`、`expected_range`、`remediation_code`，Quant QNT-CTR-004 1.0.0 不包含这些字段；Quant 时间是规范 UTC 字符串，Researchflow 使用 `time.Time`。
- **语义差异**：Quant `Decision.Validate` 要求 EvaluatedAt 不早于 SystemAsOf、Reason Code 唯一、Decision.Status 与 `AggregateStatus(Reasons)` 一致；Quant AggregateStatus 要求至少一条 block 规则，所有 block 明确 not_triggered 才能 pass。Researchflow `ValidateRiskDecision` 已拒绝空 Reasons、`EvaluatedAt < SystemAsOf`，并阻止 block triggered/unknown 与 pass 冲突；本轮已按 Quant DTO 结构约束补齐 Reason Code 唯一性校验；本轮继续补齐 DecisionID、RuleSetVersion、Reason.Code/RuleRef/SafeMessage、InputRefs、EvidenceRefs 的有效 UTF-8、非空且无首尾空白约束，与 Quant `nonBlank` 规则一致；仍未复用 Quant 的完整聚合约束。
- **新增来源证据**：Quant `result.RiskPayload` 明确包装 `quant/risk.Decision`，Quant `result_type=risk` 的 Golden Payload 也是 QNT-CTR-004 RiskDecision；这意味着 Quant 风险决策已经作为 Quant ResultEnvelope 的一部分输出。
- **调用边界**：`ComposeResult` 当前同时接收 `QuantResults []QuantResultEnvelope` 和独立 `RiskDecision *RiskDecision`，但没有正式 Adapter 或来源绑定。若 Composer RiskDecision 意在表达 Quant 风险结论，应从已验证的 Quant `risk` Payload 投影，避免调用方另行传入一份可能冲突的决策；若它代表独立 HardRiskGate，则必须使用不同的领域身份/契约，不能继续把它描述为 Quant RiskDecision。
- **原因**：虽然 Quant 已有权威 RiskDecision Payload，但 Composer 独立参数包含 Quant DTO 不存在的 `rule_ref`、`observed_value`、`expected_range`、`remediation_code`，因此无法仅凭字段相似性判定它是 Quant 投影还是独立 HardRiskGate 结论。
- **影响**：不能将 Quant 的 risk Payload 直接映射到 Composer RiskDecision，也不能宣称两边硬风险语义已完成端到端一致性验收。
- **处理责任**：Quant Owner + Researchflow Owner；若属于独立 Gate 结果，由项目契约 Owner 确认其归属和公共语义。
- **本模块独立推进**：Researchflow 在 `ValidateRiskDecision` 中拒绝重复 Reason.Code，并校验关键文本字段与引用为有效 UTF-8、非空且无首尾空白；增加对应拒绝测试和多条不同 Reason.Code 接受测试。该结构约束与 Quant `Decision.Validate` 的要求一致，不代表已经完成 Quant DTO Adapter 或聚合语义对齐。
- **建议决策**：优先考虑将 Quant 风险决策作为 `QuantResults` 中 `result_type=risk` 的权威来源；Composer 不再接受第二份无来源绑定的 Quant RiskDecision。只有在确有独立 HardRiskGate 时，才单独定义 Gate 决策类型和 Owner。
- **解除条件**：由 Quant Owner + Researchflow Owner 确认上述来源选择；冻结字段映射、时间规则、Reason 引用规则和状态聚合边界；补充 Quant RiskPayload 到 Composer 的 Contract Test，证明 block triggered/unknown、缺少 block、状态与 Reason 不一致时均 fail-closed。
- **当前状态**：OPEN；阻断 FLOW-006 风险结论生产接入，不阻断 Quant trend/risk Payload 的独立强类型验证。Reason.Code 本地结构校验补齐后，剩余卡点仍需跨模块决议。

### RF-BLOCK-009：News Search Payload Validator 未执行 Schema uniqueItems 约束

- **现状**：News Payload Schema `doc/news/schema/NEWS-STAGE-search-candidates-payload-1.0.0.schema.json` 对 `candidate.missing_fields` 声明 `uniqueItems: true`。
- **代码事实（修复前）**：`ValidatePayloadJSON` 只检查枚举，没有检查重复值。`repair_all` 已在 `internal/news/stage/search_candidates_validation.go` 增加重复项检测，并在 `search_candidates_test.go` 增加拒绝重复值的回归测试。
- **影响**：Researchflow 的 `StageOutputRegistry` 复用 News 提供的 Validator，生产 Envelope/Payload 校验目前不能证明完全符合该 Schema。不能将当前 Validator 标记为完整 pass。
- **责任边界**：由 News Owner 修复 News 自己的 Payload Validator 和测试；Researchflow 不复制 `internal/news/**` 的领域规则。
- **解除条件**：已满足（repair_all 完整 CI 通过；Stage Output Registry 使用同一 News Validator）。
- **验收证据**：完整 Integration CI Run [44450277491](https://github.com/Kasign/stock_data_provider/actions/runs/44450277491)，HEAD `5b9ebe6a2b8103b4c63a48c3c0f0ddb004d583ec`，结论 success。
- **当前状态**：RESOLVED（仅限 repair_all 聚合基线）；`feat/news` 和 `feat/researchflow` 尚未同步，不得宣称模块分支已解除。

## 4. 状态定义

- `OPEN`：有明确责任人和解除条件，当前未解除。
- `IN_PROGRESS`：正在按解除条件处理。
- `RESOLVED`：有对应 SHA、测试或权威契约证据证明解除。
- 卡点不等于整个模块停止；只暂停依赖该卡点的具体变更。

## 5. 2026-11-19 最新分支复核与本模块独立修复

### 跨模块分支最新状态

| 分支 | 当前 HEAD | 对 Researchflow 的影响 |
|---|---|---|
| `feat/researchflow` | 代码验收 SHA `555a16726bfa5e48968b7f23b08e1ba6de7706d9`；当前分支 HEAD 为文档提交 `6a0df9ca6c24fb0006996ee0c8be82fbe5008670` | 本轮补齐 ComposeResult 嵌套输入 UTF-8 边界；专项 CI 已通过 |
| `feat/server` | `c8a246505eefda0661a15d593def56ddbb5b0711` | Worker 生产 Executor 仍只装配 News Search；未发现 Research/Quant/LLM Stage/Adapter 注册 |
| `feat/research` | `0d9dc0651a6e0bda3888d695f2ffec5545c248bd` | 本轮相对前次仅调整 Diagnostics 测试样本；Context/Evidence/Claim 生产消费仍需真实授权调用方 |
| `feat/quant` | `992a10d1837dad8e695f9e9d7b6f459ebdcc3eb9` | 本轮增加 GetResult 主体解析后的 Context 取消检查；RiskDecision 契约未变化，RF-BLOCK-008 仍未解除 |
| `feat/llm` | `77ef70568a0e76ab3092b25503c36ee2316990a3` | Provider Port 存在；生产装配/真实模型消费者和目标模型 Tokenizer 仍未闭环 |
| `feat/data` | `9f6adaf280a94bc60f993554473fd0adb70b07da` | 尚无可注册的 Researchflow Data Stage 完整契约 |
| `feat/news` | `cb4206e8e59ec7bcb27660d33e4cc28b1cd2a06d` | 现有 News Search 已装配；Payload Validator uniqueItems 缺口仍由 News Owner 负责 |

### 本模块独立修复

- `ComposeResult` 之前只对部分字段检查空白；多个嵌套字符串字段未验证 UTF-8，后续 `encoding/json` 深拷贝可能静默替换非法字节。
- 已在组合入口递归校验全部输入字符串和 `json.RawMessage` 原始字节，并在任何投影/深拷贝前 fail-closed。
- 新增测试覆盖 Run ID、Context、Evidence、Claim、LLM Response、Limitations、RiskDecision 和 Quant Payload。
- 代码 SHA：`c7849cf5ad44dfa3a702844c14eb657f864d7512`；格式修正 SHA：`555a16726bfa5e48968b7f23b08e1ba6de7706d9`。
- Run [44440261770](https://github.com/Kasign/stock_data_provider/actions/runs/44440261770)：Researchflow 专项 format/tests/race/vet、WorkerStore、MariaDB lifecycle integration 已通过；Run 最终 completed/success：根模块 Test/Race/Vet、Researchflow format/tests/race/vet、WorkerStore unit/race/vet、Researchflow MariaDB lifecycle integration 均 success；News/Market Data/Compose 相关 Jobs 为 skipped，不代表这些领域的真实集成通过。
- 本轮没有改动其他分支；没有新增占位 Stage/Adapter。Server 尚未装配 Research/Quant/LLM 的事实仍成立。


## RepairAll Quant→Researchflow→Server 生产装配审查（2026-11-19）

- 审查基线：repair_all SHA b52c8afde2f2e32fca498a78987d34f96fba4335。
- Server bootstrap.go 当前无 Quant 相关装配调用；researchflowComposition 仅保存 newsStage，researchflowRuntime 将 newsStage.executor 与 newsStage.registry 注入 Worker。
- news_search_stage.go 的 Stage Output Registry 仅注册 news.search_candidates；当前 StageExecutor 只处理 SearchCandidatesStageCode。
- Researchflow resultvalidation/quant.go 是结果校验/映射能力，不是 StageExecutor，也不负责授权；不能据此认定 Quant Stage 已装配。
- 判定：当前 Server/Researchflow 生产 Quant Stage 为 NOT_COMPARABLE（尚无生产 Stage 场景与提供方 Facade），不是 Root 装配漏接一段现成实现。
- 解锁条件：Quant 提供正式计算 Facade、完整输入/输出契约与生产结果持久化/授权端口；明确 Researchflow Stage 输入参数、资源授权和结果引用；随后 Server 才能装配并进行 Worker 真实集成验收。
