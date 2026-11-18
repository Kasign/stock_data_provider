# Researchflow 模块卡点

更新时间：2026-11-18  
对应分支：`feat/researchflow`  
代码验收基线 SHA：`9ec3ebdbdf79cf548f98c438019b5aff2e2dc929`  
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
- **影响**：现有测试只能证明纯函数预算/映射/JSON 组合逻辑，不能证明生产数据可授权、PIT 正确、Tokenizer 预算准确或真实 LLM 调用成功。
- **处理责任**：Research Owner（Context Facade/授权语义）、LLM Owner（真实 Tokenizer）、Server Owner（组合根/Worker 接入）、Researchflow（适配和 Run Context 绑定）。
- **解除条件**：完成 Owner Adapter 与当前授权；Run/Context/LLM 的 SystemAsOf 和 Deadline 一致性测试；生产 Tokenizer 与模型版本绑定及准确性证据；Server/Worker 真实 Stage 注册和集成测试。
- **当前状态**：OPEN；阻断 FLOW-005 真实生产链路，不阻断 FLOW-001/002 其他工作。

### RF-BLOCK-006：FLOW-008 尚无真实诊断生产者和 Server 注册

- **现状**：Researchflow 的 `NewStageCheck` 只把已提供的 StageAssessment 投影为 Platform Diagnostics.Check；当前未发现生产代码调用该函数，Server bootstrap/composition 也未注册 Researchflow Diagnostics.Check。
- **边界**：该 Adapter 不会自动诊断 Data/News/Quant/Research/LLM，也不产生独立 DiagnosticRecord。它只负责四态映射与安全投影。
- **本轮修复**：CheckedAt 统一转 UTC；Attributes 使用 allowlist（当前仅 `source` 稳定代码），拒绝自由文本、未知 key 和保留字段；相关模块测试通过。
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
- 代码验收 SHA `9ec3ebdbdf79cf548f98c438019b5aff2e2dc929` 的 Researchflow module gofmt/tests/race/vet 已通过。

## 3. 下一步解除顺序

1. 项目契约 Owner/Integration 将 GOV-CTR-002/004 1.1.0 同步到共同集成基线。
2. 核对 FLOW-CTR-004 Envelope 项目登记簿状态与模块 Schema/Golden/Registry 资产。
3. 推进 FLOW-005：Research Owner Adapter、Run PIT/Deadline 绑定、LLM Tokenizer 和 Server/Worker Context Stage。
4. 推进 FLOW-008：从真实 Stage Provider 接入 Assessment，并由 Server 注册 Platform Diagnostics.Check。
5. 继续 FLOW-001 Fault/Context 机器契约和 FLOW-002 其他领域真实 Stage 的定点验收。

## 4. 状态定义

- `OPEN`：有明确责任人和解除条件，当前未解除。
- `IN_PROGRESS`：正在按解除条件处理。
- `RESOLVED`：有对应 SHA、测试或权威契约证据证明解除。
- 卡点不等于整个模块停止；只暂停依赖该卡点的具体变更。
