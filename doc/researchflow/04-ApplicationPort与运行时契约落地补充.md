# Researchflow Application Port 与运行时契约落地补充

- 适用任务：FLOW-001～003；后续 FLOW-004 Executor
- 契约范围：FLOW-CTR-001～003、GOV-CTR-002/003/005、PLAT-CTR-001～005、INT-CTR-001
- 状态：按已接受决议实现的字段/行为基线；Schema、Golden、Contract Test 和真实执行证据仍 pending_evidence
- 权威关系：03-CONTRACT-006与007最终决议.md 为状态/竞态权威；项目级契约目录为编号、公共字段和状态权威

## 1. 模块与跨模块边界

Researchflow 是唯一跨业务模块研究流程编排者，只调用公开 Application Port，不 import 其他模块的 Domain、Repository、SQL Model、HTTP DTO 或 Adapter。

- Quant：读取确定性数值分析与风险决策。
- Research：读取/创建研究 Context、Evidence、Event、Claim、Hypothesis 等领域对象。
- LLM：调用模型 Generate/Stream；Tool Call 只是待校验意图。
- Data/News：通过各自公开查询契约获取快照/证据，不绕过 Adapter。
- Notification：不是研究 DAG 中的同步 Stage Provider。Run 终态通知由 Researchflow 在终态 CAS 成功后写本地 Outbox，按 INT-CTR-001 投递；不得在 Run 尚未终态时同步调用 Notification 发送，也不依赖 Notification 内部包。

Platform 提供公共执行/技术原语；Researchflow 拥有 Run/Stage/Attempt/Template 状态机和编排策略。根 HTTP、DI、Worker 启动和根 Migration 注册不属于模块分支。

## 2. CreateRun 命令与身份

CreateRunCommand 必须显式包含 schema_version、idempotency_key、template_ref、template_version、system_as_of、deadline_at、input_snapshot_refs、parameters、originating_request_id、correlation_id。

- authenticated_caller_ref 从可信认证 Context 获取，不作为请求体字段，不接受客户端自报主体。
- idempotency_key 唯一作用域为 (authenticated_caller_ref, CreateRun)；相同键/相同指纹返回原 Run，不同指纹返回 idempotency_conflict。
- 请求指纹覆盖调用主体、template_ref/version、system_as_of、deadline_at、规范化 input_snapshot_refs 和 parameters；不覆盖 request_id、originating_request_id、correlation_id 和接收时间。
- `input_snapshot_refs` 是不透明 Ref 集合：必须为非 null 数组；Ref 必须非空、首尾无空白且不重复。规范化只做字典序排序，不静默 Trim 或改写 Ref；空数组仍由模板 `allow_empty_inputs` 决定是否允许。指纹计算本身也执行上述结构校验，避免幂等记录查询先于完整命令校验时接收非法 Ref。
- `RequestFingerprint` 在幂等查询之前还必须拒绝零值或非 UTC 的 `system_as_of/deadline_at`，不能先转 UTC 后再比较，否则同一瞬间的偏移时区输入可能绕过命令格式校验并命中已有幂等记录。
- system_as_of 必须为 RFC3339Nano UTC 且不晚于可信服务端当前时间；deadline_at 必须晚于数据库 UTC 当前时间。二者独立。
- originating_request_id 从可信 Request Context 注入；纯内部触发显式 null。correlation_id 从可信父链路继承，否则由 Researchflow 生成高熵 ID；不得信任客户端自报 Header。
- input_snapshot_refs 必须为非 null 数组；空数组仅允许模板注册时声明 allow_empty_inputs=true 的纯函数流程。

## 3. Workflow Template 与参数快照

Template 通过 template_ref + template_version 精确读取，不允许隐式 latest。模板必须版本化并固定 stage_code、依赖 DAG、required/optional、优先级、适用性规则、传播策略、每个 Stage 的 retry_policy、requires_hard_risk_gate、typed parameters Schema、Provider Output Registry 引用和资源预算。

Template fingerprint 必须覆盖会改变执行或结果语义的全部字段，包括 DAG、参数 Schema 版本、适用性规则版本、传播策略、重试策略、风险门禁声明、输出 Registry 版本和资源策略。不得只对显示名称或 stage_code 列表计算指纹。

## 4. 状态机执行不变量

状态枚举、传播矩阵、Hard Risk actionability、取消/重试/deadline/CAS/fencing 以 03 决议为准，不得另建状态语义。

1. Run 终态只能在所有 Stage 终态且无 running Attempt 后提交；结果摘要与终态在同一事务/CAS 固化。
2. blocked 是运行期依赖/前置条件失败，不创建 Attempt；skipped 只表示 CreateRun 前按版本化规则判定不适用/裁剪，并有 reason_code。required Stage 不适用时仅进入 resolved_template_snapshot.excluded_stages、不创建 Stage；optional Stage 不适用时创建初始终态为 skipped 的 Stage（Version=1、FinishedAt=CreateRun 数据库 UTC 时间、AttemptNo=0、无 Attempt），其 reason_code 保留在 Stage.reason_codes。skipped 不是运行期迁移；可执行 Stage 依赖不适用 Stage 时 CreateRun fail-closed。
3. retry_wait 不参与 Run 归并；RetryStage 仅由 Scheduler 推进，不是人工重跑入口。
4. max_attempts 包含首次执行；只对白名单 reason_code 重试；degraded、业务 reject/unknown、权限失败、Schema 错误不可自动重试。
5. Run deadline 是所有 Stage/Attempt 的共同绝对截止；数据库 UTC 时钟与 Run.version CAS 是并发裁决依据。
6. 取消/超时请求不代表 Worker 已停止；必须收到停止确认或先 fencing 撤销旧写入权，才可收敛终态。
7. 外部副作用使用稳定下游幂等键；结果 ambiguous 且下游无幂等/查询能力时不得自动重发。

## 5. Stage Provider Result 与错误边界

Stage Provider 只返回由 output_type + schema_version 注册的强类型 payload。run_ref、stage_ref、attempt_ref、stage_code 和 template_version 由 Orchestrator 注入，Provider 不得覆盖。

- 领域业务 reject/unknown 是合法业务 payload，不等于执行错误。
- Provider 技术错误使用 PLAT-CTR-001 Fault 语义；公开投影不得泄露 cause、SQL、SDK 原始信息或 Secret。
- 输出 Schema 不匹配属于执行契约错误，不得存成 succeeded。
- quality_summary 是领域质量维度，不映射为 Stage status；Stage status 表示执行/传播结果。
- diagnostic_refs 只存不透明引用，不嵌入原始日志或敏感正文。

## 6. 下游上下文继承

每次 Stage/Attempt 调用继承 Run 的 system_as_of、deadline_at、input_snapshot_refs、template_ref/version/fingerprint 和 correlation_id。每个实际执行使用独立 request_id；重试不能刷新 system_as_of、替换输入快照或变更 Template 版本。

调用 Data/News/Quant/Research/LLM 时，时间/快照由 Orchestrator 注入；AuthorizationRef 只是审计引用，不是 bearer token；LLM Tool Call 回到 Researchflow 后按 Registry、Schema、Scope、预算和幂等策略重新验证。Gateway/上游 Request ID 不得覆盖调用方 ID。

## 7. 首批 Port 和验收矩阵

首批用例至少覆盖 CreateRun、GetRun、CancelRun、RetryStage、GetWorkflowTemplate、Template 版本读取、Provider Output Registry、Run/Stage/Attempt Repository CAS 和 Run terminal Outbox。

必须测试：请求幂等重放/冲突、模板 fingerprint、DAG 环/空 DAG、required/optional applicability、传播矩阵、Hard Risk pass/reject/unknown/未执行、deadline 边界、Cancel/claim 双向竞态、Retry/deadline 竞态、租约接管/fencing、迟到输出拒绝、外部副作用 ambiguous outcome、终态 Outbox 原子提交/回滚。

Fake Repository/Fake Clock/Fake Provider 只证明模块内逻辑；MariaDB/Redis、进程崩溃恢复、真实模型/工具和跨模块 L2/L3 证据单独记录。


## 8. 首批领域实现记录（2026-11-06）

### 已新增代码

- `internal/researchflow/domain/types.go`：Run/Stage/Attempt 状态、命令、读取实体和结果摘要类型。
- `internal/researchflow/domain/command.go`：CreateRun 基础字段校验、时间边界、输入快照校验、参数对象校验、caller-scoped 请求指纹和幂等冲突判定。
- `internal/researchflow/template/template.go`：模板必填字段、重试白名单、退避策略、传播矩阵、Hard Risk Registry、Applicability 接口、DAG 环检测、拓扑排序和 fingerprint。
- `internal/researchflow/state/machine.go`：Run/Stage/Attempt 合法迁移、有限重试、retry_wait 推进、取消、deadline、Run 归并及 actionability 判定。
- 对应 `*_test.go`：命令、模板、状态迁移和重试等单元测试。

### 字段来源与语义

| 字段/组 | 来源 | 语义与约束 |
|---|---|---|
| system_as_of / deadline_at | CreateRunCommand 显式调用参数 | 两者独立；前者不晚于可信服务端当前 UTC，后者晚于数据库 UTC 当前时间 |
| input_snapshot_refs | 调用方提供的已存在快照引用 | 非 null 数组；空数组必须由模板 allow_empty_inputs 授权 |
| originating_request_id | 可信 Request Context | 可空；内部触发为 null，不接受伪造客户端值 |
| correlation_id | 可信父链路或 Researchflow 生成器 | 必填；当前领域包只校验非空，注入/生成由后续 Application 层实现 |
| template_ref/version/fingerprint | 精确版本化 Template | 禁止隐式 latest；fingerprint 覆盖执行语义字段 |
| retry_policy | Template 的 Stage 定义 | max_attempts 包含首次执行；reason code 精确白名单；退避单位毫秒 |
| risk_decision | 已注册 Hard Risk Stage 的领域输出 | pass/reject/unknown 是业务决策；不得与 Stage 执行状态混为一谈 |

### 当前实现限制

- 参数目前校验为 JSON object，并通过 Template.ParameterSchemaRef 固定 Schema 引用；实际 JSON Schema Registry 校验仍未接入。
- Output Registry 目前只固定版本引用；尚未验证 Provider 的 output_type/schema_version。
- Applicability 和 Hard Risk Registry 由调用方注入；没有真实业务规则或注册数据，因此不能标记真实模板验收通过。
- 状态迁移、重试、取消和 deadline 函数尚未绑定数据库 UTC 时钟、事务、Run.version CAS、租约或 fencing。deadline 下的在途 Attempt 必须先停止确认/撤销写入权，再将 Stage/Attempt 收敛。
- 测试文件已新增，但当前没有测试运行结果；不报告通过。
- 尚未实现 Run 终态 Outbox、Repository、Migration、Application Facade 和跨模块 Adapter。


## 9. P0 Review 修正（2026-11-06）

- 硬风险门禁执行层失败与业务决策 reject/unknown 必须分开投影：前者 `actionability_status=not_evaluated`，后者 `blocked`；只有明确 pass 且 Run=succeeded 才 actionable；任何 Run=failed 均保持 not_evaluated。
- required Stage 的 skipped 不允许被 `no_impact` 吸收；optional Stage 才可 no-impact。
- ResultSummary.reason_codes 汇总各 Stage 的 reason_codes，排序去重，避免诊断信息在 Run 聚合时丢失。
- 对应单元测试已增加，等待执行证据。


## 10. 状态机契约对齐补充（2026-11-06）

本轮按 FLOW-CTR-001/002 最终决议补齐：
- optional skipped 且传播为 no_impact 时，coverage 按已完成计；required Stage 不允许 no_impact。
- CancelRun / ApplyDeadline 使用复制后的 Stage 切片，输入非法时不对调用方切片产生部分修改。
- FinalizeCancellation 在所有 Stage 终结且无 running Attempt 后固化 ResultSummary，再提交 Run=cancelled；取消结果 actionability 为 not_evaluated。
- 硬风险 Stage 成功返回的 RiskDecision 必须为 pass/reject/unknown，缺失或非法值拒绝聚合。


## 11. 项目级契约复核后的明确缺口（2026-11-06）

- 项目级 Review 已要求普通命令 Request Fingerprint 使用 RFC 8785 JCS + SHA-256。当前 `domain.RequestFingerprint` 仍使用 `json.Marshal`，不满足该要求；必须完成公共规范化实现和 Golden 后才能验收幂等契约。
- `CreateRunCommand.Parameters` 当前只检查 JSON object，缺少按不可变 template_version 绑定的 typed Parameter Schema Validator。
- 当前 Domain 类型的 JSON Tags 不构成公开 DTO。公开 Run/Stage/Attempt DTO 的字段、nullable、引用、状态和版本约束必须由独立 Schema/Golden 明确。
- 取消终态现在补充固化 ResultSummary；但数据库事务、Run.version CAS、固定锁顺序、Worker claim、容量原子预留、lease/fencing 和崩溃恢复仍未实现，不能将领域纯函数描述为并发安全。


## 12. Run 终态与 Retry/Worker Claim 边界复核（2026-11-06，按最终契约修订）

- `TransitionRun` 对外拒绝直接进入任何终态；只有 AggregateRun、FinalizeCancellation 等完成 Stage/Attempt 校验并固化 ResultSummary 的收敛入口可调用包内迁移函数提交终态。
- 2026-11-09 复核发现此前将 Scheduler Retry 与 Worker Claim 拆成两个可分别提交的事务，与 FLOW-CTR-001/002 最终决议冲突。最终契约明确：`retry_wait -> ready` 与下一 Attempt 创建必须同事务提交；Run.version CAS 必须同时协调 Retry、Cancel、Deadline 和 Worker Claim。
- 因 Attempt 只有 running/终态，没有 queued 状态，Researchflow 领域决策函数现提供 `ClaimRetryStage`：在一次原子用例中校验 retry_at、Run 状态、deadline 和 max_attempts，逻辑推进 `retry_wait -> ready -> running` 并构造下一 running Attempt。Repository 必须在同一事务内分配 attempt_no/fencing token、CAS 更新 Stage、插入 Attempt；不得单独提交中间 ready 状态。中间 ready 仅是逻辑状态，不单独持久化；一次 RetryStage Repository 更新只递增一次 Stage.version。
- 首次执行仍使用 `ClaimReadyStage`；其 Repository 写入同样必须与 Run.version CAS、Stage.version CAS 和 Attempt 唯一键在一个事务中完成。
- 纯函数不提供数据库并发保证；任何 claim/retry 决策都不能替代持久化 CAS、锁顺序、lease 和 fencing。
- 取消终态 ResultSummary 将 cancelled Stage 计入 MissingStageCodes；actionability 仍固定 not_evaluated。


## 13. RFC 8785 Request/Template Fingerprint（2026-11-06）

- RequestFingerprint 与 WorkflowTemplate.Fingerprint 均先构造完整 JSON 对象，再由 RFC 8785 JCS 生成 canonical UTF-8 bytes，最后执行 SHA-256。
- CreateRun parameters 以 RawMessage 纳入待规范化 JSON；避免先经 `UseNumber`/map 解码导致 duplicate-key 被静默覆盖。JCS canonicalizer 必须拒绝 duplicate key。
- input_snapshot_refs、Template.Stages、Stage.Dependencies、RetryableReasonCodes 按契约集合语义排序；真正有序的参数数组保留原顺序。
- 时间先显式转 UTC，再进入 JSON 对象；tracing 字段仍不参与 RequestFingerprint。
- 依赖：`github.com/gowebpki/jcs@v1.0.2`（模块 Go 1.21）。
- 当前提交只完成 JCS 接入和等价输入/重复键测试代码；Golden 固定摘要、RFC 8785 全量测试执行记录仍待补充。


## 14. Typed Parameter Schema Registry 与解析门禁（2026-11-06）

- Template.ParameterSchemaRef 是不可变 Registry 引用，不允许 latest/浮动别名；Registry 返回的 ref 必须与请求完全一致，并同时返回非空 version、canonical document fingerprint。
- Template.Resolve 必须先执行参数 Schema 校验，再运行 applicability rule。Schema 不存在、版本/指纹缺失、引用错配、参数字段缺失/类型不符/额外字段均 fail-closed；不得进入 DAG 解析或 Run 创建。
- JSON Schema Draft 2020-12；根类型必须为 object，根对象必须关闭额外属性。MVP 禁止远程 $ref/任意 URL 加载，仅允许文档内局部引用，避免运行时联网和 Schema 供应链漂移。
- Schema 内容使用 RFC 8785 JCS 后计算 SHA-256。ResolvedTemplate 保留 Schema Ref、Registry Version、Fingerprint，Application 持久化到 resolved template snapshot；Registry 对相同 Ref 的版本/内容变更必须拒绝。
- 实现依赖 github.com/santhosh-tekuri/jsonschema/v5 v5.3.1（Apache-2.0，Go 1.19），固定 JSON Schema Draft 2020-12。


- ResolvedTemplate 只接受 64 位十六进制 SHA-256 Schema 指纹，并保留底层校验错误链，便于 Application 层按参数错误/Schema 缺失/Schema 不可变性冲突分类。


- 参数 Schema Registry 的 not_found 与暂时不可用错误必须区分；调用方不得把 Registry 连接/超时故障伪装为模板配置不存在。
- JSON Schema format assertion 显式开启；参数重复键、非对象、空 body 均在 schema 校验前拒绝。


### Resolved Template 指纹绑定修正（2026-11-06）

- 发现原 `Resolve` 结果虽然保存 ParameterSchema 的 version/fingerprint，但 Run 使用的 `ResolvedTemplate.Fingerprint` 仍仅为静态 Template 指纹，导致同一 Template Ref/Version 下 Schema Registry 版本或内容变化未反映到 Run 指纹。
- 现将 resolved fingerprint 定义为 RFC 8785 JCS + SHA-256，输入包含静态 Template fingerprint、ParameterSchemaRef、Registry version 和 canonical Schema digest。这样同一模板引用下 Schema 版本/内容变化会产生不同执行指纹。
- 修正模板测试夹具：此前使用非十六进制占位 fingerprint，与 Resolve 的 64 位 SHA-256 校验冲突；统一替换为合法固定 64 位十六进制值。

## 15. CreateRun Application Port 首轮

- `RequestIdentity` 是可信认证/请求上下文适配边界；caller_ref 不从命令体读取。originating_request_id/correlation_id 由可信上下文注入，correlation 缺失时由高熵 ID Generator 生成。
- TemplateRegistry 必须按 `(template_ref, template_version)` 精确读取；返回值 ref/version 不匹配立即拒绝。
- CreateRun 先计算 caller-scoped JCS 指纹并查询幂等记录，再解析模板。这样相同请求重放不依赖模板/Schema Registry 当前在线；并发首次请求仍由 Repository 原子写入门禁裁决。
- Run/Stage、完整 ResolvedTemplate（包括 ParameterSchema version/digest 与 applicability 排除记录）、原始参数快照及幂等键/指纹必须在 Repository 同一事务提交。
- Repository `CreateRunAtomic` 必须保证唯一键 `(caller_ref, operation, idempotency_key)`；若并发竞争中已存在记录，必须在事务内比较 fingerprint，相同返回原 Run，不同返回 idempotency_conflict。必须在写入前以数据库 UTC 时钟复核 deadline。
- 当前 Fake Repository 只证明 Application 编排，不证明 SQL/Redis/事务/CAS；持久化实现与 MariaDB 集成留在 FLOW-007/根集成阶段。


- Review 发现 ResolvedTemplate 首版快照虽保存了 Stage DAG 和指纹，但未显式保留 `requires_hard_risk_gate`、`allow_empty_inputs`、`output_registry_version`、`resource_policy_ref`，无法仅凭快照完整还原执行配置。现已将四项纳入 ResolvedTemplate 并由 Resolve 固定，CreateRun Repository Port 持久化该完整快照；增加回归测试。


### 2026-11-06 公共版本与 Fencing 字段对齐

- 对齐项目契约目录：已持久化 Run/Stage 的初始 version 从 1 开始，公开 Schema 的最小值同步改为 1。
- Stage 公开读取模型不包含 attempt_no；该字段只属于 Attempt。Domain 内部 Stage.AttemptNo 继续作为重试计数，不泄露为跨模块字段。
- Worker Claim 必须由 Repository/Executor 在同一事务门禁下分配并注入正数 fencing_token；纯领域 Claim 函数拒绝 0，生成的 Attempt 回显该 token。Attempt Schema 对 fencing_token 的最小值同步改为 1。
- 以上只统一字段语义和纯函数入参，不代表数据库 CAS、单调 token 分配或并发 fencing 已实现。


- CreateRun 持久化前对输入快照引用按集合语义排序，并将已通过 typed Schema 校验的 parameters 保存为 RFC 8785 JCS canonical bytes；避免同一指纹语义请求因输入顺序/JSON 排版产生不同的运行快照。


### 2026-11-06 适用性排除证据进入终态摘要

- Review 发现 ResolvedTemplate 已记录 excluded_stages，但状态机聚合/取消摘要此前始终初始化为空，导致 CreateRun 前的适用性裁剪证据没有进入 Run 终态 ResultSummary。
- Domain Run 增加仅供内部归并使用的 excluded-stage 快照字段（不进入公开 Run DTO）；CreateRun 从 ResolvedTemplate 注入。正常聚合、排队取消和运行中取消最终摘要均保留该列表。
- 新增成功聚合与取消终态的排除证据回归测试。真实持久化时仍须从不可变 ResolvedTemplate Snapshot 恢复该字段。


- Template Resolve 输出的 Stage 依赖引用现按集合语义排序，并同步写回 ResolvedStage.Definition，确保相同 Template fingerprint 对应稳定的 ResolvedTemplate Snapshot/Run Stage 依赖序列。


### 2026-11-06 Run / Stage / ResultSummary 公开引用字段对齐

- Run 公开 DTO 改为 `result_summary_ref`，不再内嵌 ResultSummary；ResultSummary 作为独立公开资源，单独定义 `result-summary-1.0.0.schema.json` 与 Golden。
- Stage 公开 DTO 按项目字段级契约暴露 `quality_summary_ref`、`diagnostic_refs`，移除 Domain 内部的 `quality_status` 与 `risk_decision`；质量状态与风险门禁决策留在 ResultSummary 领域汇总。
- Run Schema 对终态状态增加条件约束：succeeded/partial/failed/cancelled 必须有非空 result_summary_ref；非终态允许显式 null。
- Domain 保留 ResultSummary 内容用于状态归并，ResultSummaryRef 由持久化终态提交时分配；Repository 必须与 Run 终态和 ResultSummary 在同一事务内写入。该持久化能力尚未实现。


- RunRepository / 终态 Repository 的后续实现必须在终态事务内分配 result_summary_ref，并原子保存 ResultSummary 资源和 Run 终态；公开 Schema 已通过条件约束拒绝缺少该引用的终态 Run。


## 16. FLOW-007 Repository 持久化模型与事务边界

本节定义 Researchflow 自有 Repository/Schema 的逻辑模型与原子边界；不包含可直接执行的 Migration SQL，也不代表 MariaDB 已验收。Platform Storage 仅提供连接、事务可用的 SQL DB 资源和 Migration Runner；业务表、SQL、Repository、事务内容由 Researchflow 自有。

### 16.1 逻辑表与字段映射

统一约束：目标数据库基线采用 Platform PLAT-009 已完成真实容器集成的 MariaDB 11.4；表使用 InnoDB；所有绝对时间以 UTC 存储，精度至少微秒（DATETIME(6)），并以 UTC_TIMESTAMP(6) 作为事务内裁决时钟；引用字段使用不透明字符串，不从引用内容推导业务含义。JSON 文档由模块按对应 Schema 校验后写入，读取时仍校验版本与结构。实际字段长度、索引长度须以 MariaDB 11.4 验证后冻结。Researchflow Migration 使用独立 namespace `researchflow`，版本号严格递增且已发布语句不可修改；通过 Platform `storage.ApplyMigrations` 显式执行，不在模块构造或 OpenSQL 时自动执行。

| 逻辑表 | 主键/唯一约束 | 持久化内容 | 字段来源与语义 |
|---|---|---|---|
| `researchflow_runs` | `run_ref` 主键 | schema_version、status、version、system_as_of、deadline_at、template_ref/version/fingerprint、input_snapshot_refs、result_summary_ref、originating_request_id、correlation_id、termination_reason、created_at/started_at/finished_at | Run 来自 CreateRun 与状态机；version 是单调 CAS 版本；result_summary_ref 仅在终态提交时分配 |
| `researchflow_resolved_templates` | `run_ref` 主键/外键 | schema_version、template fingerprint、parameter schema ref/version/fingerprint、完整 ResolvedTemplate JSON、canonical parameters JSON、created_at | 来源为 CreateRun 的不可变解析快照；必须包含 DAG、required/optional、依赖、适用性排除、传播/重试策略、Hard Risk 声明、allow_empty_inputs、Output Registry、Resource Policy |
| `researchflow_stages` | `stage_ref` 主键；唯一 `(run_ref, stage_code)` | status、version、required、dependency_stage_refs、retry_policy_ref、attempt_no、retry_at、output_ref、quality_summary_ref、diagnostic_refs、reason_codes、时间字段、fencing_token_high_water | Stage 来源为 ResolvedTemplate；attempt_no 是已分配尝试序号；fencing_token_high_water 是该 Stage 历史已发放 token 的单调高水位，不进入公开 Stage DTO |
| `researchflow_attempts` | `attempt_ref` 主键；唯一 `(stage_ref, attempt_no)`；唯一 `(stage_ref, fencing_token)` | run_ref、stage_ref、attempt_no、status、lease_owner、lease_expires_at、fencing_token、started_at/finished_at、fault_ref、output_ref | 每次实际执行追加一条 Attempt；已终结 Attempt 不覆盖；fencing_token 由 Repository 在事务内分配并回显 |
| `researchflow_result_summaries` | `summary_ref` 主键；唯一 `run_ref` | schema_version、summary JSON、created_at | 来源为状态机最终归并；一条 Run 仅允许一个不可变终态摘要；按独立 ResultSummary Schema 校验 |
| `researchflow_idempotency` | 唯一 `(caller_ref, operation, idempotency_key)` | request_fingerprint、run_ref、created_at | caller_ref 来自可信认证 Context；operation 固定 CreateRun；fingerprint 为 JCS+SHA-256；重放相同指纹返回原 Run，不同指纹冲突 |
| `researchflow_outbox` | `event_id` 主键；唯一业务事件幂等键 | aggregate_ref、event_type、schema_version、payload、occurred_at、publish 状态/尝试信息 | 来源为 Run 终态提交；仅在终态 CAS 成功的同一事务写入。事件字段和版本必须遵循 INT-CTR-001；不在本阶段自创事件 Schema |

建议索引（除主键/唯一约束外）：Run 按 `(status, deadline_at, created_at, run_ref)` 支持恢复/截止扫描；Stage 按 `(run_ref, status, retry_at, priority)` 支持依赖内调度；Attempt 按 `(status, lease_expires_at)` 支持租约扫描；Outbox 按 `(publish_status, occurred_at, event_id)` 支持发布扫描。索引最终以真实查询计划和目标数据量验证，不为未实现查询预建索引。

### 16.2 必须原子提交的用例

1. **CreateRun**：先在事务外执行幂等快速读取；未命中后开启事务，使用数据库 UTC 时钟复核 deadline，插入 Run、完整 ResolvedTemplate Snapshot 和全部 Stage，最后插入幂等记录以竞争 `(caller_ref, operation, idempotency_key)` 唯一键。幂等记录依赖 Run 引用，因此不能在 Run 尚未插入时先占位。并发相同键请求由唯一约束裁决：唯一键冲突时回滚本事务，再在新事务读取胜出记录并比较 fingerprint；相同 fingerprint 返回原 Run，不同 fingerprint 返回幂等冲突。不得先提交 Run 再补 Stage/快照/幂等记录。
2. **Worker Claim**：按 Run → Stage 的锁顺序读取并校验 Run=running、未取消、数据库时间早于 deadline、Stage=ready；同事务递增 Stage.version 与 fencing_token_high_water，分配 attempt_no，插入新 Attempt 并将 Stage 置 running。Run.version 必须参与同一命令门禁/CAS，防止与 Cancel/Deadline/Retry 竞争。
3. **Attempt 输出提交/完成**：校验 Run 仍允许执行、Attempt 仍 running、提交 token 等于 Stage 当前 fencing 高水位且 lease 未失效；同事务保存 Output 引用并推进 Attempt/Stage。旧 token、已取消 Run、已终结 Attempt 的迟到结果必须拒绝，不能覆盖既有输出。
4. **RetryStage（Promotion + Claim）**：按 Run → Stage 锁顺序确认 Run=running、未到 deadline、未取消、Stage=retry_wait、retry_at 已到且 max_attempts 预算仍可用；在同一事务/CAS 中逻辑推进 `retry_wait -> ready -> running`，递增 Stage.version，分配下一 attempt_no 和 fencing token，插入 running Attempt。不得把中间 ready 状态单独提交；不得在 Run 已 cancelling/terminal 时推进 Retry。
5. **Cancel/Deadline**：与 Worker Claim、Retry 使用同一 Run.version CAS 线性化；进入 cancelling 后禁止新 Claim/Retry。未启动 Stage 的收敛和在途 Attempt 停止确认/fencing 分开记录，不能仅因发出取消就假设 Worker 已停止。
6. **Run Finalize**：按 Run → Stage（stage_ref 升序）→ Attempt（attempt_ref 升序）固定锁顺序，验证所有 Stage 终态且不存在 running Attempt；在同一事务写入不可变 ResultSummary、分配 result_summary_ref、CAS 更新 Run 终态/version，并写入 Outbox。任一环节失败必须整体回滚，禁止出现终态 Run 无摘要或摘要无终态 Run。

任何用例不得跨事务持有数据库锁调用 Data/News/Quant/Research/LLM 或 Notification；外部调用在事务外执行，结果通过带 fencing token 的提交命令回写。

### 16.3 CAS、时钟、唯一性与冲突语义

- Run 更新使用 `WHERE run_ref=? AND version=? AND status IN (...)` 并在成功时 `version=version+1`；Stage 使用独立 version CAS。受 Run 状态约束的写命令必须先通过 Run CAS 门禁，不能只 CAS Stage。
- 所有 deadline/lease/retry 到期判定使用数据库 UTC 时钟（`UTC_TIMESTAMP(6)` 或等价数据库时钟表达式），应用进程时间仅用于非裁决性展示；不得将 Go Clock 读数当成并发裁决依据。
- Attempt 序号只在首次 Worker Claim 或 RetryStage 原子 Promotion+Claim 成功时由 Stage 行锁内递增，并受 `(stage_ref, attempt_no)` 唯一约束；RetryStage 与新 Attempt 必须同事务。fencing token 同样只在上述 Claim 事务中由 Stage 行内高水位递增并受唯一约束。禁止用进程内计数器、Redis INCR 或 MAX 查询替代同一数据库事务内的分配。
- 锁顺序固定为 Run → Stage（按 stage_ref 升序）→ Attempt（按 attempt_ref 升序）；批量更新也遵守相同排序，降低死锁概率。遇到死锁可按明确次数重试整个幂等事务，但必须重新读取并重新验证状态，不能盲目重放 SQL。
- CAS 未命中是并发冲突，不等于存储不可用；应用层重新读取后按当前状态返回 already_terminal、claim_lost、deadline_exceeded、retry_not_due 或 conflict 等稳定业务结果。数据库连接/超时故障映射 Platform Fault，不能伪装为 CAS 冲突。
- 结果摘要和 Outbox 都是 append-only/不可变事实；Outbox 发布状态可变，但原始事件 payload、event_id、schema_version 不可变。Notification 发布成功与否不回滚已完成的 Run。

### 16.4 明确延期与禁止事项

- 本阶段不新增 SQL 实现、Migration 文件、Server DI、Worker 启动、HTTP 路由，也不修改 `internal/platform/**`。Platform Storage 的连接和 Migration Runner 已存在，Researchflow 只消费其公开能力。
- 全局 Worker/队列容量原子预留属于 FLOW-004 Executor 资源预算实现；在资源策略和调度入口尚未落地前，不在本阶段臆造容量表。
- Lease 心跳、过期接管、进程崩溃恢复及真实外部副作用对账属于 FLOW-004；本节只固定持久化字段和 fencing 提交门禁。
- 表名为模块私有逻辑名，Migration namespace 固定为 `researchflow`；版本号严格递增，已发布 Migration 的 SQL/语句顺序不可原地修改。MariaDB DDL 可能隐式提交，失败后按 Platform Migration Runner 规则进入人工检查/前向修复，不假设事务回滚。
- 本轮 Review 已确认目标 MariaDB 版本为 11.4、Migration Runner 为 `internal/platform/storage.ApplyMigrations`；并修正 CreateRun 幂等唯一键占位顺序与 Retry Promotion 错误分配 Attempt 的冲突。
- 该设计当前状态为 `reviewed_design_pending_implementation`；尚未形成可执行 Migration 或 Repository 实现，真实 MariaDB 并发集成、死锁、唯一键竞争、CAS、事务回滚和崩溃恢复仍未执行，不得标记实现完成或 L3 通过。


### Attempt 终态校验 fail-closed 补充（2026-11-09）

- Run 聚合和用户取消终结此前只拒绝 `AttemptRunning`，空状态或未知状态会被误认为已停止。
- 新增统一 `validateAttemptsTerminal`，要求传入的每个 Attempt 状态均属于明确的终态枚举；running、空值和未知值一律拒绝。
- AggregateRun、FinalizeCancellation 共用校验；FinalizeDeadline 复用 AggregateRun，因此同样受保护。
- 新增回归测试覆盖空状态、queued、unknown 三种非终态输入，分别验证聚合和取消终结拒绝。
- 本修正仍需定向 Go Test/Race/Vet 执行证据；不将静态增加的测试描述为通过。


- 终态收敛还要求 Stage 集合完整且唯一：空 Stage 集合、空 stage_code、重复 stage_code 或非终态 Stage 均 fail-closed；Attempt 集合中的每一项也必须处于明确终态，未知/空状态不能视为已停止。AggregateRun、FinalizeCancellation 共用这些校验，FinalizeDeadline 通过 AggregateRun 复用。


## 9. Worker 与 StageExecutor 运行时契约（2026-11-09）

### 9.1 执行边界

- Application Worker 只依赖 `WorkerStore`、`DatabaseClock` 和 `StageExecutor` Port；不得直接 import Storage、SQL Model 或其他业务模块包。
- Worker 执行顺序：读取不可变 Stage/RetryPolicy Snapshot → ClaimAttempt → 幂等 Claim replay 直接返回 → 重新读取 running Stage Snapshot → 校验 RetryPolicy 未漂移 → StageExecutor → 校验结果 → CommitWorkerResult。
- WorkerStore Adapter 的 `CommitWorkerResult` 必须映射到 Storage `CommitAttemptResult`，不得绕过 Run/Stage/Attempt 锁顺序、lease expiry、owner、fencing token、Stage.version CAS。
- Provider 不得自行写 Researchflow DB，不得生成/覆盖 Run/Stage/Attempt 身份，不得决定 retry_at。

### 9.2 Lease、Deadline 与 Attempt Timeout

- Lease heartbeat 在 Provider 执行期间持续续租；续租失败立即取消执行 Context，当前 Worker 不得再提交结果。
- Run deadline 是绝对截止；Attempt deadline 为 `min(Run.deadline_at, Attempt.started_at + RetryPolicy.attempt_timeout_ms)`。StageExecutionRequest 同时携带两者。
- Attempt 超时映射为 Attempt=timed_out、reason_code=`attempt_timeout`，再经模板 RetryPolicy 决定 retry_wait 或 Stage=timed_out；Run deadline 不得误记为 Attempt timeout。
- RetryAt 使用 DatabaseClock UTC；算法为 `exponential_full_jitter_v1`，重试必须命中精确 reason-code allowlist，max_attempts 包含首次执行。
- Provider 技术错误当前不自动映射为可重试结果；必须先由 Fault/ReasonCode 归一化层产出受控结果，未完成该层前 fail closed。

### 9.3 Snapshot 与依赖约束

- Claim 前读取的 RetryPolicy 用于传入 ClaimAttempt.max_attempts；Claim 后必须重新读取 Snapshot 并比较完整 RetryPolicy，策略缺失、错配或漂移拒绝执行。
- 参数、输入快照引用在交给 Provider 前复制；system_as_of、template fingerprint、correlation_id 和 deadline 继承 Run，不允许 Provider 改写。
- 当前尚未落地 Server Composition Root Adapter、Worker 生命周期启动、取消确认 Port、deadline 扫描器与 MariaDB Worker 集成测试；模块内 Fake Port 测试不等于部署验收。


### 9.4 Worker 协作取消 Port（2026-11-09）

- Worker.CancelRun(runRef) 只负责当前进程内活动 Attempt 的协作取消；不是取消持久化入口。调用方必须先执行 Storage BeginRunCancellation，再广播本地/跨进程取消信号。
- Worker 为每个活动 Attempt 注册 context.CancelCauseFunc；Run 取消信号触发后 Provider Context 被取消。Provider 返回后 Worker 通过 ConfirmAttemptCancellation Port 原子确认 Attempt/Stage 取消，参数包含 Worker、fencing token 与 Claim 后的 Stage version。
- 确认成功不再执行 CommitWorkerResult；确认失败、lease 失效或 fencing 冲突均 fail-closed。进程外 Worker 未收到广播或崩溃时，由 Storage FenceExpiredCancellationAttempt 回收。
- 真实 Adapter 必须映射 MySQLRunRepository.ConfirmAttemptCancellation，并保持 Run → Stage → Attempt 锁顺序和数据库 UTC lease 检查。当前仅 Fake Port 测试，未完成跨进程通知与 MariaDB 集成验收。


### 9.5 Provider Fault 归一化边界（2026-11-09）

- Worker 可通过 ExecutionFaultNormalizer Port 将 Provider error 映射为 ExecutionFault{FaultRef, ReasonCode}；FaultRef 必须指向已持久化 Fault 事实，ReasonCode 必须是稳定、可审计的领域原因码。
- Normalizer 不提供 retryable、RetryAt 或退避参数。Worker 只按 Run Snapshot 内不可变 RetryPolicy 的精确 ReasonCode allowlist、MaxAttempts 和指数 full jitter 计算是否重试。
- 未注入 Normalizer 时 Provider error 仍 fail-closed，不会被自动猜测为 provider_unavailable；Normalizer 返回空 FaultRef/ReasonCode 或自身错误时不得提交。
- 上下文取消和 deadline 不进入 Provider Fault 分类。真实 Platform Fault 创建/查询 Adapter 及 Server 注入仍待根组合层接入。


### 9.6 Run Deadline 到期收敛

- Deadline expiration 与用户取消是互斥路径：仅处理 Run=created/queued/running 且数据库 UTC 已达到 deadline_at 的 Run；Run=cancelling 不允许进入 deadline expiration，不得混用 `user_cancelled` 与 `deadline_exceeded`。
- Storage `ExpireRunDeadline` 锁顺序 Run → Stage（stage_ref 排序）→ Attempt（stage_ref/attempt_no 排序）；所有非终态 Stage 转 timed_out，追加 `run_deadline_exceeded`；所有 running Attempt 立即转 timed_out、清除 lease、fencing_token+1。该操作不等待 lease expiry。
- Run 保持 running 以便 Application `FinalizeDeadline` 使用 Domain 聚合规则计算 failed/partial 与 ResultSummary；Storage 只写 termination_reason=`deadline_exceeded` 并递增 Run.version。`FinalizeDeadline` 只接受 `running + deadline_exceeded`，随后使用新 version 调用既有 `FinalizeRun`，ResultSummary/Outbox/Run 终态仍原子提交。
- FinalizeRun 对 deadline_exceeded 增加终态限制，只允许 failed/partial，禁止 deadline 已过仍以 succeeded 终结。
- 当前仅实现 Storage 原语和终结门禁；Deadline Scanner、Application 聚合/Finalize 编排、Server 周期任务和 MariaDB 集成验收尚未完成。


### 9.7 Deadline Scanner Port

- Application `DeadlineExpirationStore` 只暴露有界候选查询与按 Run.version 执行过期收敛；Storage 的候选查询使用 DB UTC 和稳定顺序，返回非锁定快照，Expire 方法重新锁 Run 并校验 deadline/version。
- `DeadlineScanner` 按 BatchSize 顺序执行，记录扫描/成功/版本冲突数量；错误立即停止本批，避免吞掉存储故障。
- Scanner 不直接生成 ResultSummary 或终结 Run。完成 Stage/Attempt fencing 后，`DeadlineFinalizer` 读取完整执行快照，按既有领域规则计算 failed/partial，并调用 Finalization Store Port 写入 ResultSummary + Outbox + Run 终态。真实 Storage Snapshot/Finalize Adapter 和 Server 周期调度仍待完成。


### 9.8 Deadline 与用户取消互斥及幂等

- `deadline_exceeded` 一旦写入 Run.termination_reason，即成为终止路径的线性化标记；BeginRunCancellation 不得覆盖为 user_cancelled。
- `ExpireRunDeadline` 对相同 Run 重复调用必须不增加版本、不重复 fencing；候选查询排除已有 termination_reason 的记录，避免 Deadline Scanner 热循环。
- Run=cancelling 与 deadline expiration 互斥；谁先在 Run 行锁下成功建立终止原因，另一条路径必须拒绝。

### 9.9 Deadline Finalize 领域/持久化状态对齐（2026-11-09）

- `ExpireRunDeadline` 的持久化线性化结果为 Run=`running`、termination_reason=`deadline_exceeded`；Stage/Attempt 已在同一事务内超时并 fencing。
- `FinalizeDeadline` 只接受上述状态组合，调用 `AggregateRun` 计算 failed/partial 和 ResultSummary；拒绝旧的 `cancelling + run_deadline_exceeded` 组合及 `user_cancelled`。
- 后续 Application Finalizer 必须使用 Expire 返回的新 Run.version 调用 Storage `FinalizeRun`，由后者原子写入 ResultSummary、Outbox 与 Run 终态。该编排和真实 MariaDB 集成仍待完成。

### 9.10 Deadline Application Finalizer 首轮实现（2026-11-09）

- `DeadlineFinalizer` 读取 Run/Stage/Attempt 与 ResolvedTemplate 派生的传播策略/硬风险门禁快照，使用 DatabaseClock 调用 Domain `FinalizeDeadline`。
- Finalizer 生成 ResultSummary/Outbox 引用，并通过 `DeadlineFinalizationStore` 传递预期 Run.version、终态、termination_reason 和摘要；Store Adapter 必须映射 Storage `FinalizeRun`，确保摘要、Outbox、Run 终态同事务。
- Application 只依赖 Port，不直接依赖 SQL/Storage。当前 Fake Port 测试已补，未执行；MySQLRunRepository 已实现 `LoadDeadlineFinalizationSnapshot`，从持久化 ResolvedTemplate Snapshot、Stage 和 Attempt 历史恢复聚合输入；并实现 `FinalizeDeadlineRun` 到既有原子 `FinalizeRun` 的薄适配。MariaDB 集成验收和 Server 周期调度仍未完成。

### 9.11 Deadline Scanner 排除已完成 Stage 集合（2026-11-09）

- Review 发现 Run 已有全部终态 Stage、但尚未完成 Run 聚合时，Deadline Expire 仍会写入 `deadline_exceeded`，使本可正常 succeeded 的 Run 被错误禁止成功终结。
- Storage `ExpireRunDeadline` 现在在确认全部 Stage 终态且无运行中 Attempt 后返回未变更；候选 SQL 同步排除全部 Stage 已终态的 Run，避免扫描热循环。
- 这些 Run 必须由正常聚合/Finalize 路径处理；Deadline Expire 只负责仍有未完成 Stage 的 Run。

### 9.12 正常 Run 聚合与终态提交（2026-11-09）

- 新增 Application `RunFinalizer`：仅接受 `running` 且无终止原因的 Run；复用统一快照装配，调用 Domain `AggregateRun`，要求所有 Stage/Attempt 已终态。
- 聚合成功后生成 ResultSummary/Outbox 引用，通过 Storage Port 提交；仅当持久化 CAS 成功才向返回对象填充 ResultSummaryRef。
- MySQL `LoadRunFinalizationSnapshot` 复用不可变 ResolvedTemplate Snapshot、Stage、Attempt 装配；`FinalizeRunResult` 将正常结果映射到现有原子 `FinalizeRun` 事务。
- 正常路径只允许 succeeded/partial/failed，且不接受 termination_reason；deadline 与 user cancellation 仍走各自专用 finalizer，避免跨终止原因覆盖。
- 当前未完成：Server 聚合触发/周期扫描与生命周期接入；MariaDB 正常聚合并发、CAS 冲突、Outbox 原子性集成验收。


### 9.13 User Cancellation Application Finalizer

- `CancellationFinalizer` 只接受持久化状态 `Run=cancelling` 且 `termination_reason=user_cancelled`；deadline 和正常完成路径均拒绝。
- Application 使用 DatabaseClock 调用 Domain `FinalizeCancellation`；Stage 集合完整且唯一、Stage/Attempt 全部明确终态是终结前置条件。
- `CancellationFinalizationStore` 负责提交 ExpectedVersion、`cancelled`、`user_cancelled`、ResultSummary 与 Outbox 引用。MySQL Adapter 映射到现有 `FinalizeRun`，不得复制原子事务。
- FinalizeRun 对 cancelling + user_cancelled 仅接受 RunCancelled；ResultSummary、Outbox、Run 终态和 version 更新保持同一事务。
- Server 必须在 BeginRunCancellation 成功后广播 Worker.CancelRun；仅当活动 Attempt 均完成协作确认或被过期 lease fencing 后，才调用 CancellationFinalizer。当前尚无 Server 调度/DI 和 MariaDB 端到端验收。


## 9.14 统一终态协调入口

Application 的 `RunFinalizationCoordinator` 是三种终态聚合路径的路由入口，不替代各 Finalizer，也不持有 SQL/事务逻辑。

| 持久化 Run 状态 | termination_reason | 路由 |
|---|---|---|
| running | 空 | 正常 Run Finalizer |
| created / queued / running | deadline_exceeded | Deadline Finalizer |
| cancelling | user_cancelled | Cancellation Finalizer |
| 其他组合 | 任意 | fail-closed，返回状态错误 |

- Coordinator 首先读取持久化 Run 快照，只用于选择路径；选中的 Finalizer 必须重新读取完整 Stage/Attempt 快照并执行自己的状态/原因校验。
- 不允许从 `cancelling + user_cancelled` 路由到普通或 Deadline Finalizer；不允许 deadline reason 路由到正常 Finalizer；不允许无 termination reason 的 cancelling 被当作取消完成。
- 各 Finalizer 的 Storage Adapter 仍通过既有原子 FinalizeRun 事务写入 Run、ResultSummary、Outbox；Coordinator 不新增事务。
- Server 组合根负责注入 Coordinator，并由 Deadline Scanner、Stage 完成检查、取消收敛流程调用；不得在 Researchflow 模块中新增 Server DI 或 HTTP 接口。

## 10. GetRun Application 查询契约（2026-11-09）

- `RunReader.GetRun(ctx, runRef)` 是 Application 查询 Port；Server Composition Root 将 MySQL Repository 注入 `GetRunService`，HTTP Handler 只消费 `CreateRunResult` 中的公开领域投影，不直接调用 Storage。
- RunRef 必须非空、无首尾空白且不超过 255 字符；Storage 返回的 Run.Ref 必须与请求完全一致。
- Stage 集合必须非空；每个 Stage 必须属于请求 Run，具有非空 ref/code 和正 version；Stage ref 与 stage code 在同一 Run 内分别唯一。
- 终态 Run 必须同时具有 ResultSummaryRef 与 ResultSummary；非终态 Run 不允许提前挂载摘要。Storage 必须按 ResultSummaryRef 与 RunRef 双键条件读取摘要，不能仅凭摘要 Ref 读取其他 Run 的资源。
- ResultSummary payload 必须包含 StageOutcomes、RiskGateDecisions、ExcludedStages、Missing/Degraded/Blocked Stage Codes、ReasonCodes；Coverage 范围为 [0,1]，QualityStatus 和 ActionabilityStatus 必填。
- GetRun 只做读取和一致性门禁，不负责授权判定；调用入口必须先由 Server 认证/授权层确认调用者有权读取该 Run。当前 Application Facade 不接受请求体 caller_ref，也不自行推断资源所有权。
- 当前新增 Fake Reader/解码测试只证明 Application 与 payload 结构校验；MariaDB 回读、终态摘要关联和跨模块 DTO/Schema/Golden 仍待验收。

## 11. CancelRun Application 命令契约（2026-11-09）

- `CancelRunService.CancelRun(ctx, runRef)` 只接受可信 `RequestIdentity.AuthenticatedCallerRef`；必须先经 `RunAccessAuthorizer.AuthorizeRunCancellation(ctx, callerRef, runRef)` 校验持久化 Run 所有权/取消权限，再读取 Run 和调用取消 Storage。
- Researchflow Run 当前不在 Run 聚合内持久化 caller owner；因此 Application 不得自行假定 caller_ref 与 RunRef 的关系，也不得仅凭已认证身份允许取消。Server 必须注入真实授权 Adapter；未配置 Authorizer 时构造失败。
- 取消使用 Run 当前 Version 调用 `BeginRunCancellation`，由 Storage 在 Run 行锁内与 Claim/Commit 线性化。Application 不直接更新状态、不广播假取消成功、不绕过事务。
- `Started=true` 只表示本次请求赢得持久化 `created/queued/running -> cancelling` 迁移；不表示在途 Worker 已停止。Worker 必须确认停止，或由 lease fencing 回收；CancellationFinalizer 再原子写入 ResultSummary、Outbox 和 Run 终态。
- 已处于 `cancelling + user_cancelled` 时幂等返回；终态 Run 或 `deadline_exceeded` 终止权已生效时拒绝。CAS 未获胜后必须重读；只有持久化状态已是同一用户取消才按幂等重放，否则返回版本冲突。
- Server HTTP/DI、真实 RunAccessAuthorizer Adapter 和跨进程 Worker 协作不属于本分支 Application Facade 的实现范围。

## 12. RetryStage Application 派发契约（2026-11-09）

- `RetryStageService.RetryStage(ctx, runRef, stageRef, attemptRef)` 仅供 Scheduler 内部派发；它不是用户手动重跑入口。Stage 进入 retry_wait 的唯一依据仍是 Worker 完成失败/超时 Attempt 时按不可变 Template RetryPolicy 计算出的 retry_at 和精确 reason-code allowlist。
- 同一次 Scheduler 派发的重投递必须复用稳定 `attemptRef`；不得每次重试请求都生成新 Attempt Ref。Worker/Storage 的 ClaimAttempt 对同一 Worker 的有效 lease 和同一 Attempt 进行幂等回读。
- Application 读取 Run/Stage/RetryPolicy 快照并预检：Run 必须为 running 且无 termination_reason；Stage 必须为 retry_wait、有 retry_at 且已有至少一次 Attempt；AttemptNo 必须小于 max_attempts；Stage.reason_codes 至少一个命中 RetryPolicy.RetryableReasonCodes；数据库 UTC 当前时间必须早于 Run.deadline_at 且不早于 Stage.retry_at。
- Facade 预检只用于快速拒绝非法/未到期派发，不构成并发保证。预检后 Run 可能被取消、Deadline 到期或其他 Worker 抢先 Claim；Worker.Execute -> Storage ClaimAttempt 必须重新在数据库事务中校验 Run/Stage/Attempt 状态、retry_at、deadline、attempt budget、lease/fencing 和 max_parallel，并原子提交新 Attempt 与 Stage 状态。
- RetryStage 不单独将 retry_wait 持久化为 ready。领域 `ClaimRetryStage` 中 ready 仅为逻辑中间态；持久化操作直接以一次 Stage version 增量完成 retry_wait -> running，并插入下一 running Attempt。
- 不重新计算 retry backoff，不修改 system_as_of、deadline、template_ref/version/fingerprint、parameters 或 input_snapshot_refs；重试沿用原 Run 的不可变执行快照。
- RetryStage 返回 WorkerExecution 表示 Worker 执行/Claim 结果，不代表 Run 终态。终态聚合继续由 Run Finalizer 处理。


## 13. GetWorkflowTemplate Application 查询契约（2026-11-09）

- GetWorkflowTemplateService.GetWorkflowTemplate(ctx, ref, version) 只允许精确 Ref + Version 查询；不允许 latest、默认版本、别名回退或缺省版本。
- Application 复用 CreateRun 已使用的 TemplateRegistry.Get Port。Registry 错误原样保留错误链；返回 Ref/Version 与请求不完全一致时返回 ErrTemplateVersionMismatch。
- Application 使用 template.Fingerprint 对返回 Template 的执行语义字段重新计算指纹。Registry 若提供非空 Fingerprint，必须与计算值相等；缺省时由 Application 返回计算值。该校验用于发现 Registry 内容漂移，不替代注册阶段的 Template Validate。
- 返回 Template 的 Stages、Dependencies、RetryableReasonCodes 必须深拷贝，禁止将 Registry 内部可变切片直接暴露给调用方。
- 查询 Facade 不负责授权。Server 必须在调用前完成认证和按 Template 资源策略授权；公开响应 DTO 不得直接暴露内部 Template 结构或内部错误。
- Template Registry Adapter 的发布/写入路径必须执行 template.Validate（注入真实 HardRiskGateRegistry）、固化 Fingerprint，并拒绝同一 Ref/Version 的内容覆盖；该真实 Registry Adapter/持久化由模块集成阶段完成。


## 14. MySQL Template Registry 持久化契约（2026-11-09）

- v3 Migration 新增 `researchflow_workflow_templates`，主键 `(template_ref, version)`，保存 immutable definition、SHA-256 fingerprint 和数据库 UTC 创建时间。模块只声明 Migration；必须由 Server Composition Root 注册并执行。
- `MySQLTemplateRegistry.Get` 只按 Ref + Version 精确读取，不支持 latest/别名；缺失版本返回 `ErrWorkflowTemplateNotFound`，严格 JSON 解码（拒绝未知字段/尾随内容）、identity、fingerprint 或模板语义校验失败返回 `ErrWorkflowTemplateCorrupt`。
- `Publish` 是 insert-only 发布操作：必须先执行 `template.Validate` 并注入真实 HardRiskGateRegistry，再计算 fingerprint。相同 Ref/Version + 相同 fingerprint 幂等成功；同一身份不同 fingerprint 返回 `ErrWorkflowTemplateConflict`，禁止覆盖已发布版本。
- Version 最大长度为 64，与 SQL 列一致；Ref 最大长度为 255。发布和读取均拒绝空白或首尾空格身份。
- Template definition 的 Stage、Dependencies、RetryableReasonCodes 以稳定顺序持久化；读取返回独立切片。Fingerprint 不包含自身字段，防止递归哈希。
- Application `TemplateRegistry` Port 由该 Adapter 实现；`GetWorkflowTemplate` 和 `CreateRun` 可共用同一 Adapter。模板发布权限、发布者审计、管理 API、Server DI 和根 Migration 注册由 Server 组合层负责，本模块不擅自开放外部发布接口。
- MariaDB 集成测试覆盖精确版本、相同内容重放、同版本变更冲突、缺失版本和读取隔离；执行结果必须在启用 MariaDB 的 CI/环境中单独确认，静态 Migration 测试不能替代真实 DB 验收。
