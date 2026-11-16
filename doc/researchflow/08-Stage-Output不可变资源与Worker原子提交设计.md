# Stage Output 不可变资源与 Worker 原子提交设计

- 状态：Researchflow 内部实现设计，遵循冻结契约 FLOW-CTR-004；不改变公共契约字段语义。
- 日期：2026-11-16
- 范围：Provider Result Envelope 校验后的结果资源持久化、引用生成及 Attempt/Stage 原子提交。
- 非目标：注册业务 Stage、定义领域质量枚举、定义 ResultSummary Coverage、装配 Server 生产 StageExecutor。

## 1. 设计结论

Provider 只返回领域输出内容；Orchestrator 负责执行身份、完成时间、结果引用和持久化。Worker 在提交前使用模板快照中的 `OutputRegistryVersion` 精确选择 Registry，并完成 Envelope 校验。通过校验的 Payload 与 QualitySummary 必须和 Attempt、Stage 状态在同一个 MariaDB 事务中提交。

不再接受成功/降级结果携带 Provider 自报的 `output_ref` 或 `quality_summary_ref`。引用由 Researchflow Storage 在事务提交路径生成。

## 2. 持久化模型

新增表 `researchflow_stage_output_resources`，每个 Provider 输出对应两条不可变资源记录：

| 字段 | 语义 |
|---|---|
| resource_ref | Storage 生成的全局唯一不透明引用，分别供 Stage.output_ref / Stage.quality_summary_ref 使用 |
| run_ref / stage_ref / attempt_ref | 归属的 Run、Stage、Attempt |
| resource_kind | 固定为 `payload` 或 `quality_summary` |
| stage_code | 从已持久化 Stage 读取，不信任 Provider |
| template_version | 从 Run 快照读取 |
| registry_version / registry_fingerprint | 使用的精确 Registry 版本及指纹 |
| output_type / schema_version | Provider discriminator，经 Registry 精确匹配 |
| schema_ref | 对应 Payload 或 QualitySummary 的注册 Schema 引用 |
| content | 已通过对应 Validator 的 JSON object 原文 |
| created_at | 数据库 UTC 时间 |

约束：
- `resource_ref` 为主键；同一 Attempt 的 `resource_kind` 唯一，防止重复提交生成第二份结果。
- 资源行与 Attempt、Stage 使用 Run/Stage/Attempt 复合外键约束归属。
- 资源记录没有 UPDATE/DELETE 业务路径；修正结果必须创建新的 Attempt，不覆盖旧内容。
- 不为 payload 或 quality_summary 推导领域含义，不规范化或改写 JSON 内容。
- `Stage.output_ref` 指向 payload 资源；`Stage.quality_summary_ref` 指向 quality_summary 资源；Attempt.output_ref 与 Stage.output_ref 保持一致。

## 3. Worker 边界

1. LoadStageExecutionSnapshot 同时返回 Run.TemplateVersion、ResolvedTemplate.OutputRegistryVersion；两者必须来自持久化快照。
2. StageExecutor 的成功/降级结果只允许携带 ProviderResult：`output_type`、`schema_version`、`payload`、`quality_summary`、`diagnostic_refs`。不得携带结果引用、身份字段或 completed_at。
3. Worker 从 Run/Stage/Attempt 和已解析模板构造 ExecutionIdentity；以数据库时钟读取 completed_at。
4. Registry 版本必须与模板快照精确相等；按 `stage_code + output_type + schema_version` 查找唯一 Entry，并分别验证 Payload、QualitySummary。
5. 成功/降级结果缺少 ProviderResult、Registry 未注入、Registry 版本不符、Entry 未注册或任一 Validator 失败，均不得调用 CommitWorkerResult。
6. RetryWait/Failed/TimedOut/Cancelled 等无输出结果不要求 ProviderResult，维持现有状态机、RetryPolicy、取消流程。
7. Registry 不允许空注册表为生产成功 Stage 放行；当前没有业务 Entry，意味着当前没有可成功提交的生产 Stage，这与生产 Provider 尚未接入的状态一致。

## 4. 原子提交

扩展现有 `CommitAttemptResult`，保持既有锁顺序 Run -> Stage -> Attempt 和 Lease/Fencing 校验：

1. 开启事务并锁定 Run、Stage、Attempt。
2. 校验 Run deadline、Stage version/attempt_no、Attempt owner/fencing_token/lease。
3. 对成功/降级结果，要求携带已校验的 Envelope 与注册项元数据；失败类结果不得携带输出资源。
4. 在事务内生成两个不可猜测的资源引用，插入 payload 与 quality_summary 两条不可变资源记录。
5. 使用 Storage 生成的引用更新 Attempt.output_ref、Stage.output_ref、Stage.quality_summary_ref，并提交状态、诊断、原因及数据库完成时间。
6. 任一步骤失败均回滚资源行、Attempt 和 Stage 更新；过期 Worker 不会留下孤立资源或成功状态。

资源引用生成使用密码学随机字节，不使用时间戳、Run/Stage 编号或 Provider 内容作为引用。引用作为不透明值对外传递，不暴露内部表主键推导规则。

## 5. 读取边界

Storage 提供按 `resource_ref` 精确读取不可变资源的能力，返回资源类型、Schema/Registry 元数据和原始 JSON 内容。读取时验证引用存在且类型匹配；不提供按任意 SQL 条件读取或覆盖更新能力。

下游 Adapter 通过引用读取资源，并根据自身契约做领域映射；不直接依赖 Researchflow 数据表结构。

## 6. 失败与兼容策略

- 不能因 Registry 缺失而回退到旧的 Provider 自报引用路径。
- 不能在 Worker 外先写资源，再尝试提交 Attempt/Stage。
- 不能将校验失败伪装成 Provider 业务失败或消耗 RetryPolicy 次数；校验失败属于提交前契约拒绝，保留 Attempt 由现有 Lease 超时/回收机制处理，不产生成功 Attempt/Stage。
- 不改变 heartbeat、超时、取消确认、RetryPolicy、数据库 deadline 和 fencing 语义。
- 现有仅供测试的直接 Storage 提交路径必须同步迁移到带已校验结果资源的输入；不保留生产旁路。

## 7. 验收

- Migration 可重复应用且不改写历史 Migration。
- Worker 对未注入 Registry、Registry 版本不匹配、未知 Stage/output/schema 组合、Payload 无效、QualitySummary 无效、Provider 尝试传入引用/身份字段均拒绝提交。
- 成功/降级结果两类资源、Attempt 和 Stage 同事务提交；人为制造任一插入/更新失败时全部回滚。
- 过期 Lease、错误 Worker、旧 FencingToken 不得写入资源或覆盖 Stage/Attempt。
- 通过 resource_ref 可精确读取原始 Payload/QualitySummary；资源不可更新，重复 Attempt 资源插入失败。
- RetryWait、Failed、TimedOut、Cancelled 路径保持原行为，不生成输出资源。
- MariaDB 集成测试与根模块测试、gofmt、vet、全量 Integration CI 均通过后，才标记本阶段完成。

## 8. 当前不解锁事项

本设计只闭合 Researchflow 的通用执行与持久化边界，不代表任何业务 Stage 已注册或可运行。领域 Provider、真实 Payload/QualitySummary Schema、质量语义仍由各模块提供；ResultSummary 质量枚举、Coverage 分母和 Summary 身份/版本绑定仍按独立契约决议处理。
