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
- system_as_of 必须为 RFC3339Nano UTC 且不晚于可信服务端当前时间；deadline_at 必须晚于数据库 UTC 当前时间。二者独立。
- originating_request_id 从可信 Request Context 注入；纯内部触发显式 null。correlation_id 从可信父链路继承，否则由 Researchflow 生成高熵 ID；不得信任客户端自报 Header。
- input_snapshot_refs 必须为非 null 数组；空数组仅允许模板注册时声明 allow_empty_inputs=true 的纯函数流程。

## 3. Workflow Template 与参数快照

Template 通过 template_ref + template_version 精确读取，不允许隐式 latest。模板必须版本化并固定 stage_code、依赖 DAG、required/optional、优先级、适用性规则、传播策略、每个 Stage 的 retry_policy、requires_hard_risk_gate、typed parameters Schema、Provider Output Registry 引用和资源预算。

Template fingerprint 必须覆盖会改变执行或结果语义的全部字段，包括 DAG、参数 Schema 版本、适用性规则版本、传播策略、重试策略、风险门禁声明、输出 Registry 版本和资源策略。不得只对显示名称或 stage_code 列表计算指纹。

## 4. 状态机执行不变量

状态枚举、传播矩阵、Hard Risk actionability、取消/重试/deadline/CAS/fencing 以 03 决议为准，不得另建状态语义。

1. Run 终态只能在所有 Stage 终态且无 running Attempt 后提交；结果摘要与终态在同一事务/CAS 固化。
2. blocked 是运行期依赖/前置条件失败，不创建 Attempt；skipped 只表示 CreateRun 前按版本化规则判定不适用/裁剪，并有 reason_code。
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
