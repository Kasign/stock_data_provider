# Researchflow 模块卡点

更新时间：2026-11-18  
对应分支：`feat/researchflow`  
代码验收基线 SHA：`2df5ee14b5c39613a10f124da463ae3f939e13b3`  
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
- **测试与验收**：新增固定数据库时钟和时钟不可用时不提交 retry 的测试；代码验收 SHA `2df5ee14b5c39613a10f124da463ae3f939e13b3` 的 Actions Run 44293452576 中 Researchflow module gofmt/tests/race/vet Job 已 success。
- **当前状态**：RESOLVED。

## 2. 本模块已继续推进的工作

- `CreateRunService` 通过 `DatabaseClock.NowUTC` 生成 `system_as_of`，并用同一次数据库 UTC 时钟读取计算 `deadline_at`。
- `MySQLDatabaseClock` 通过 Platform `time.Service` 获取当前数据库 UTC 时间；`MySQLRunRepository` 复用同一时钟用于 Outbox RetryAt。
- Run/Stage/Attempt DTO 投影将必填时间和可空时间统一规范化为 UTC；新增 +08:00 输入的 Z 输出测试。
- Outbox RetryAt 不再依赖进程时钟；数据库时钟缺失/失败时不提交重试计划。
- `doc/researchflow/04-ApplicationPort与运行时契约落地补充.md` 已校正 CreateRun 命令字段、RequestFingerprint 输入和服务端时间生成方式。
- 代码验收 SHA `2df5ee14b5c39613a10f124da463ae3f939e13b3` 的 Researchflow module gofmt/tests/race/vet 已通过。

## 3. 下一步解除顺序

1. 项目契约 Owner/Integration 将 GOV-CTR-002/004 1.1.0 同步到共同集成基线。
2. 核对 FLOW-CTR-004 Envelope 项目登记簿状态与模块 Schema/Golden/Registry 资产。
3. 继续 FLOW-001 命令 DTO、执行上下文机器契约和 Fault 投影的定点 Review。
4. 继续 FLOW-002 真实生产 Template、HardRiskGateRegistry 和 Stage Output Registry 注册来源核对。
5. 不重复建设已验收的 Worker、Storage、Outbox；仅对实际缺口和真实生产 Stage 接入补验收。

## 4. 状态定义

- `OPEN`：有明确责任人和解除条件，当前未解除。
- `IN_PROGRESS`：正在按解除条件处理。
- `RESOLVED`：有对应 SHA、测试或权威契约证据证明解除。
- 卡点不等于整个模块停止；只暂停依赖该卡点的具体变更。
