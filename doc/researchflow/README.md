# Researchflow 模块

- 分支：`feat/researchflow`
- 代码：`internal/researchflow/**`
- 任务前缀：FLOW
- 当前状态：领域状态机、MySQL 核心生命周期事务、Application Worker 单次执行协调器、MySQL WorkerStore Adapter、Scheduler 单次 Run 运行协调器均已有代码；全局容量事务守卫（CreateRun 排队容量、ClaimAttempt 全局 Worker Lease）与固定数量 Worker Pool 已实现，并通过代码 SHA `6692cf1dfdb67509dca3dd564600d3085e935509` 的 Researchflow 模块门禁和 MariaDB 普通/Race 集成；Server 生命周期与周期调度接入、生产 StageExecutor Adapter 仍未闭环。WorkerStore 与 Scheduler 已有模块级及 MariaDB 验收证据，详见任务进度文档。

Researchflow 是研究应用编排层，通过 Data、News、Quant、Research、LLM 的公开 Application Port 编排研究流程。Notification 不作为同步 DAG Stage；Run 终态 CAS 成功后按 INT-CTR-001 写本地 Outbox。它拥有 Run/Stage/Attempt 状态，不拥有其他模块内部领域规则。Platform Task Runner 只提供执行基础设施，不替代 Researchflow 业务状态机。

## 设计与契约

- [研究流程与状态机设计](01-研究流程与状态机设计.md)
- [FLOW-CTR-001/002 最终决议（历史别名 CONTRACT-006/007）](03-CONTRACT-006与007最终决议.md)
- [Application Port 与运行时契约补充](04-ApplicationPort与运行时契约落地补充.md)
- [FLOW-005 Context Assembly](05-FLOW-005上下文装配实施说明.md)
- [FLOW-006 Result Validation](06-FLOW-006结果综合与校验实施说明.md)
- [任务拆分与验收](02-任务拆分与验收.md)

## Stage Provider Envelope 与 Output Registry（2026-11-16）

已新增独立 Envelope DTO/Builder、版本化 Output Registry 通用框架、Schema/Golden。Provider 不能覆盖 Orchestrator 身份和完成时间；Registry 精确匹配版本及输出 discriminator，Payload/QualitySummary 使用注册的强类型 Validator。当前不注册虚构业务 Stage，生产 Provider 仍未解锁。实施与验收状态见 [Stage Provider Envelope 与 Output Registry 实施记录](07-Stage-Provider-Envelope与Output-Registry实施记录.md)。

## 当前代码能力

- `domain`、`template`、`state`：Run/Stage/Attempt 模型、模板/DAG 与状态迁移。
- `application`：CreateRun、Scheduler 单次 Run 运行协调（快照读取、计划计算、Stage CAS、冲突重载重算、并发 Stage 派发）、SchedulerCandidateScanner（可调度 Run keyset 分页）、ExpiredAttemptScanner（过期 running Attempt 复合游标分页）、固定数量 Worker Pool（无缓冲背压、优雅 Shutdown）、Worker 单次执行协调（Lease 心跳检测跨进程取消并确认）、取消、Deadline、Retry、Outbox 与 Run Finalization 用例。
- `storage`：MySQL Run/Stage/Attempt 核心事务、租约/fencing、Deadline、Finalize、Outbox 持久化及全局容量守卫（排队 Run 与有效 Worker Lease）。 Run 终态 Outbox 已按冻结 `researchflow.run.terminal` / Schema `1.0.0` 生成完整通知 Payload。
- `publicdto`、`parameterschema`：公开 DTO、参数 Schema 校验。
- `contextassembly`、`toolpolicy`、`resultvalidation`：上下文装配、工具授权、Quant Envelope 与风险校验。
- `diagnostics`：Platform Diagnostics 适配入口。

## Stage Provider Envelope / Output Registry 与 Worker 原子提交（2026-11-16）

FLOW-CTR-004 的通用 Envelope / Registry 已接入 Worker 成功/降级提交链路：

- Worker 从持久化 ResolvedTemplate 读取 OutputRegistryVersion；Provider 仅返回 output_type、schema_version、payload、quality_summary、diagnostic_refs。
- Worker 注入 Run/Stage/Attempt/StageCode/TemplateVersion/CompletedAt，Registry 精确校验输出组合和两份 JSON Schema；未注入 Registry、版本不匹配、未知输出组合或 Validator 失败均不能提交成功。
- Provider 自报 output_ref / quality_summary_ref 的成功结果被拒绝。Storage 在原有 Run → Stage → Attempt 锁和 Lease/Fencing 事务中生成不透明引用，原子写入 Payload、QualitySummary 两条不可变资源，再更新 Attempt / Stage。
- Migration v6 新增 `researchflow_stage_output_resources`；Storage 提供按 resource_ref 精确读取能力。
- MariaDB 集成测试覆盖模板 Registry 版本快照、成功提交、两份资源读取及元数据；Worker 单测覆盖无 Registry 和 Provider 引用越权拒绝。

代码 SHA：`7392caaba59aa4d322d4b91e1d34a7294d2f6505`；Integration CI Run [43837079196](https://github.com/Kasign/stock_data_provider/actions/runs/43837079196) 全量 success（gofmt、root/go-core vet/test、MariaDB 跨模块与 Worker Bootstrap、Compose、WebUI 检查及构建）。

当前仍未注册任何业务 Stage Entry，也没有生产 StageExecutor Adapter 或 Server Worker 生产装配。通用执行/持久化边界完成，不代表 News、Research、Quant 或 LLM 已成为生产 Provider；不得创建占位 Entry。实现设计与验收范围见 [Stage Output 不可变资源与 Worker 原子提交设计](08-Stage-Output不可变资源与Worker原子提交设计.md)。

## 当前明确缺口

- Scheduler 已提供单次 Run 的运行协调服务；周期扫描、Server 生命周期启停与多 Run 调度入口尚未接入。当前派发直接调用 Worker.Execute，不引入第二套队列；跨进程并发仍由 ClaimAttempt 事务兜底。
- Worker 有单次 `Execute` 协调器；MySQLRunRepository 已实现 WorkerStore，执行快照读取与结果提交映射已落地。全局容量事务守卫和固定数量 Worker Pool 已通过模块 gofmt/test/race/vet 与 MariaDB 普通/Race 集成；queued Run 周期恢复由 Server 调用 SchedulerCandidateScanner 分页接入；进程崩溃后 running Stage 的过期 Attempt 由 ExpiredAttemptScanner 发现，Server 应调用 Worker.Execute 触发 ClaimAttempt 原子回收；Worker Lease 心跳已能检测跨进程 Run 取消并确认。ExpiredAttemptScanner 已通过当前 HEAD 的模块 gofmt/test/race/vet 与 MariaDB 普通/Race 集成验收。Server 已在 integration 接入终态 Outbox Relay，并验证 Relay 组件 Start/Stop；Researchflow Run Scheduler/Worker 的 Server 生命周期接线、生产 StageExecutor Adapter 仍未闭环。Worker Pool 已提供模块内优雅 Shutdown。
- `StageExecutor` 是注入端口，Data/News/Quant/Research/LLM 的生产 Adapter 尚未在本分支闭环。
- Worker Pool 是模块内有界执行协程池，不是持久化队列；周期扫描与池生命周期由 Server 驱动。跨 Worker 的 Run 并发和全局 Worker 上限由 ClaimAttempt 数据库事务兜底。
- Go 测试、Race、Vet、真实 MariaDB、进程崩溃恢复和跨模块端到端验收必须按实际 SHA 记录，不以测试文件存在代替通过证据。

后续实施顺序以任务文档最新状态盘点为准；不要依据早期历史记录重复建设已经落地的 Repository、Worker 或状态机能力。


## Integration 接入状态（2026-11-13）

Server 在 integration 已接入 Researchflow → Notification Outbox Relay。Run 终态事件使用 `researchflow.run.terminal` / Schema `1.0.0`，Server 校验 Outbox Payload Fingerprint 并构造 Notification Event Envelope；只有 Notification EventReceiver 接收成功后才确认 Outbox。当前 Run #54（`43359166036`，SHA `63f49065a5ef3379d95948a6c2c6e504a555a63f`）的 MariaDB 接收、失败重试、重复处理和 Relay 组件启停测试通过。

该接入只覆盖终态事件送入 Notification Inbox/FanoutWorkItem；不代表 Researchflow Run Scheduler/Worker 已由 Server 启动，也不代表生产 StageExecutor、Data/News/Quant/LLM Adapter 或 Notification 渠道投递已完成。

## Integration 接入状态（2026-11-14，第 4 项只读盘点）

Integration 侧已完成 Researchflow Stage 接入的契约与代码准入盘点，判定 **`blocked`（非 `fail`）**：FLOW-CTR-004 的 Stage Provider Result 只有字段表、无权威 Output Registry 与 Envelope Schema；`StageExecutor` 在 `internal/researchflow/application/executor.go:47-49` 只有接口、生产实现为零；`WorkflowTemplate.OutputRegistryVersion` 只是字符串字段；Quant 侧 `DecodePayload` 仅注册 `trend/1.0.0` / `risk/1.0.0`。本轮未改动本模块任何代码或进度表，也未新增 Stage Code / 输出 Schema / 空壳 Executor。

模块自身能力（状态机、Scheduler、WorkerPool、MariaDB 验收）保持本文前述记录，不重复验收。解锁所需的冻结契约、`stage_code + template_version + output_registry_version` 三元组、生产 Provider 与跨模块端到端验收条件，以及下一轮起点，统一以 [Integration 当前任务交接与继续执行](../整体任务规划/05-治理与验收/Integration当前任务交接与继续执行.md) 第 6 节为准。

## 2026-11-16 可信 CallerRef 执行期传播

Researchflow CreateRun 从可信 RequestIdentity 取得 authenticated caller，并将其写入唯一幂等记录。此前 Worker 的执行快照没有读取该身份，StageExecutionRequest 也未向生产 Provider 传递，导致下游模块无法获得创建 Run 时的可信调用主体。

本次在 Migration v5 为 `researchflow_run_idempotency.run_ref` 增加唯一约束；Worker 在同一 Repeatable Read 快照中从 `operation=CreateRun` 的幂等记录读取 caller_ref，并在 claim 前拒绝缺失身份。Worker 第二次读取执行快照时复核身份未变化，再由 Orchestrator 注入 StageExecutionRequest.AuthenticatedCallerRef。该字段仅供内部 Application Port 使用，不进入 Run/Stage/Attempt 公共 DTO，也不接受 Provider 或客户端覆盖。

本节只完成 Worker → StageExecutor 的可信 CallerRef 进程内传递，不代表 Server 生产身份装配或远程认证传输已完成。生产 RequestIdentity 必须来自 Server 已验证的 Principal：Owner 主动发起 Run 时，CallerRef 使用该 Principal 的可信 Subject（当前单 Owner Subject 为 `owner`），不能从请求 DTO/Header 自报；AuthMethod 由 Server 路由策略单独校验。`service:researchflow` 是 DEC-PLAT-002-050 中 Quant Result 读取授权的特定服务主体示例，不是所有 Researchflow CreateRun 的通用 CallerRef，不得据此把 Owner 发起的 Run 改记为服务主体。Provider 仍须按自身 Owner 授权规则校验，不得把 CallerRef 当作 AuthorizationRef 或 Bearer Token。MariaDB 集成用例已增加真实身份读取断言；新提交的 CI 验收结果待记录。


## 2026-11-17 News Search 参数准入第一切片

- Server Bootstrap 包已实现不可变参数 Schema Registry，引用为 `doc/researchflow/schema/news-search-parameters-1.0.0.schema.json`，版本 `1.0.0`；它目前由 `newNewsSearchStageCompositionWithProviders` 组合工厂持有。
- Schema 从 News Stage 已有输入形状与 `internal/news/app/search.go` 校验边界推导：`query` 必填且非空白，`limit` 必填且为 1～100 的整数，`query_dimension` 可选字符串，拒绝额外字段。
- Schema Registry 返回副本，未知 ref 使用明确的 `ErrSchemaNotFound`；Researchflow Validator 负责编译、实例校验和 fingerprint。
- Server Bootstrap 包已实现精确版本的 `news.search_candidates.always@1.0.0` Applicability Rule。该规则只适用于 News Search 工作流：该流程的 News 搜索 Stage 始终参与 DAG；未知 ref/version fail closed。改变适用条件必须发布新版本。
- **接线边界：** 当前 `bootstrap.Run` 尚未调用 `newNewsSearchStageComposition`，Researchflow 生产 Worker Runtime 也未装配；因此 Registry/Rule 的实现与定向测试已完成，但尚不能称为生产运行时已接入。
- 这只是正式 Template 的两个准入依赖，不代表已发布生产 Template、Resource Policy 已就绪、CreateRun 已装配或 Worker Runtime 已启动。


## ResourcePolicy 授权快照与 News Stage 执行约束（2026-11-17）

ResourcePolicy 已接入 CreateRun、Worker 与 News Stage 的应用执行链：

- CreateRun 在原子持久化前解析策略，并将策略定义指纹与 Run 级 Provider allowlist 快照写入现有 ResolvedTemplate JSON。
- Worker 从 Run 持久化快照恢复授权，Claim 前后深比较，并把独立 Provider Code allowlist 传入 StageExecutor。
- News Stage 只执行快照允许的 Provider；当前配置新增 Provider 不扩大旧 Run，移除 Provider 不静默替换，而是输出 not_configured Outcome。
- 失败及 retry_wait 阶段可持久化经 Output Registry 校验的 Provider 诊断输出。

代码 SHA `8db50e263f3383defd3d9b216d0c81d37ed3dd63` 的 Integration CI Run [44014768749](https://github.com/Kasign/stock_data_provider/actions/runs/44014768749) 全量 success。

**边界：** 这完成的是 ResourcePolicy 应用层授权执行链，不代表 Server 生产 Worker Runtime、生产 CreateRun 消费者或正式 Workflow Template 已接入。生产 Bootstrap、可信 Owner 内部消费组合和正式 Template 仍是后续主线；SRV-CTR-001 继续 deferred。


## Run 访问授权

Researchflow 的 GetRun 和 CancelRun 均要求通过持久化 Run 创建者关联授权。MySQLRunRepository 使用既有 `researchflow_run_idempotency` 中的 `caller_ref + operation=CreateRun + run_ref` 记录验证权限；GetRun 在读取 Run 前授权，避免未授权调用者枚举 RunRef。精确 SHA `1600c321792a5172f56b8ca7bc49adf67aec5f6a` 的 Integration CI Run [44015922257](https://github.com/Kasign/stock_data_provider/actions/runs/44015922257) 全量 success。

Server 可信 Owner Session RequestIdentity Adapter 和内部 CreateRun/GetRun/CancelRun 组合仍未接入；SRV-CTR-001 继续 deferred。
