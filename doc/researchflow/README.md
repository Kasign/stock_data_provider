# Researchflow 模块

- 分支：`feat/researchflow`
- 代码：`internal/researchflow/**`
- 任务前缀：FLOW
- 当前状态：领域状态机、MySQL 核心事务、Worker、Scheduler、固定数量 WorkerPool、Deadline/Attempt/取消恢复与 Run Finalization 均已实现；`integration` 已将正式 News Search Template、Owner 内部用例和 Worker-only Runtime 接入 Server Lifecycle。当前 Runtime 周期扫描、并发执行与优雅关闭已具备生产装配代码；真实 MariaDB 已覆盖正常成功、Provider Retry、Deadline 和无在途 Attempt 的用户取消终态路径。进程崩溃恢复、在途取消 Fencing、多进程容量与 Outbox Relay 实际投递仍需补齐。生产 StageExecutor 当前只覆盖 News Search Provider。

Researchflow 是研究应用编排层，通过 Data、News、Quant、Research、LLM 的公开 Application Port 编排研究流程。Notification 不作为同步 DAG Stage；Run 终态 CAS 成功后按 INT-CTR-001 写本地 Outbox。它拥有 Run/Stage/Attempt 状态，不拥有其他模块内部领域规则。Platform Task Runner 只提供执行基础设施，不替代 Researchflow 业务状态机。

## 设计与契约

- [研究流程与状态机设计](01-研究流程与状态机设计.md)
- [FLOW-CTR-001/002 最终决议（历史别名 CONTRACT-006/007）](03-CONTRACT-006与007最终决议.md)
- [Application Port 与运行时契约补充](04-ApplicationPort与运行时契约落地补充.md)
- [FLOW-005 Context Assembly](05-FLOW-005上下文装配实施说明.md)
- [FLOW-006 Result Validation](06-FLOW-006结果综合与校验实施说明.md)
- [任务拆分与验收](02-任务拆分与验收.md)

## Stage Provider Envelope 与 Output Registry（2026-11-16）

已新增独立 Envelope DTO/Builder、版本化 Output Registry 通用框架、Schema/Golden。Provider 不能覆盖 Orchestrator 身份和完成时间；Registry 精确匹配版本及输出 discriminator，Payload/QualitySummary 使用注册的强类型 Validator。截至 2026-11-16，当时尚未注册业务 Stage；当前已在 integration 注册并装配正式 News Search Stage，生产 Provider 能力仍限于 News Search。实施与验收状态见 [Stage Provider Envelope 与 Output Registry 实施记录](07-Stage-Provider-Envelope与Output-Registry实施记录.md)。

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

截至该历史记录 SHA，当时尚未注册业务 Stage Entry。当前 integration 已注册 News Search Stage，并由 Server Worker Runtime 执行；Data、Quant、Research、LLM 等其他 Stage Adapter 仍未完成，不得创建占位 Entry。实现设计与验收范围见 [Stage Output 不可变资源与 Worker 原子提交设计](08-Stage-Output不可变资源与Worker原子提交设计.md)。

## 当前明确缺口

- `integration` 已装配 Researchflow Worker-only Runtime：正式 Template 发布、Scheduler、WorkerPool、Deadline、过期 Attempt、取消恢复、Run Finalization 和终态 Outbox Relay 均已注册到 Server Lifecycle；Outbox Relay 先启动、Runtime 后启动，关闭时按逆序先停 Runtime 再停 Relay。
- Runtime 的调度和过期 Attempt 恢复任务已按本地 Worker 并发上限做有界异步派发；扫描循环不等待 Stage 执行完成，重复候选通过进程内 in-flight key 去重，Stop 会先取消扫描/执行上下文并等待任务退出，再关闭 WorkerPool。跨进程容量仍由 MariaDB ClaimAttempt 事务兜底。
- 最新代码 SHA `0623b3f65876be56658d2443686cca6b281644bf` 的 Integration CI Run [44086825786](https://github.com/Kasign/stock_data_provider/actions/runs/44086825786) 全量 success。真实 MariaDB Runtime 集成测试已覆盖正常成功、RateLimited 后一次 Retry 成功、Deadline 到期失败、用户取消后 Cancelled Finalization；上述终态均验证 ResultSummary 与唯一 terminal Outbox。进程崩溃恢复、在途取消 Fencing、多进程容量和 Outbox Relay 实际投递仍待验收。
- `StageExecutor` 是注入端口，Data/News/Quant/Research/LLM 的生产 Adapter 尚未在本分支闭环。
- Worker Pool 是模块内有界执行协程池，不是持久化队列；周期扫描与池生命周期由 Server 驱动。跨 Worker 的 Run 并发和全局 Worker 上限由 ClaimAttempt 数据库事务兜底。
- Go 测试、Race、Vet、真实 MariaDB、进程崩溃恢复和跨模块端到端验收必须按实际 SHA 记录，不以测试文件存在代替通过证据。

后续实施顺序以任务文档最新状态盘点为准；不要依据早期历史记录重复建设已经落地的 Repository、Worker 或状态机能力。


## Integration 接入状态（2026-11-13）

Server 在 integration 已接入 Researchflow → Notification Outbox Relay。Run 终态事件使用 `researchflow.run.terminal` / Schema `1.0.0`，Server 校验 Outbox Payload Fingerprint 并构造 Notification Event Envelope；只有 Notification EventReceiver 接收成功后才确认 Outbox。当前 Run #54（`43359166036`，SHA `63f49065a5ef3379d95948a6c2c6e504a555a63f`）的 MariaDB 接收、失败重试、重复处理和 Relay 组件启停测试通过。

该接入只覆盖终态事件送入 Notification Inbox/FanoutWorkItem；不代表 Researchflow Run Scheduler/Worker 已由 Server 启动，也不代表生产 StageExecutor、Data/News/Quant/LLM Adapter 或 Notification 渠道投递已完成。

## Integration 接入状态（2026-11-14，第 4 项只读盘点）

截至 2026-11-14 的历史盘点，Integration 侧 Researchflow Stage 接入曾判定 **`blocked`（非 `fail`）**：FLOW-CTR-004 的 Stage Provider Result 只有字段表、无权威 Output Registry 与 Envelope Schema；`StageExecutor` 在 `internal/researchflow/application/executor.go:47-49` 只有接口、生产实现为零；`WorkflowTemplate.OutputRegistryVersion` 只是字符串字段；Quant 侧 `DecodePayload` 仅注册 `trend/1.0.0` / `risk/1.0.0`。本轮未改动本模块任何代码或进度表，也未新增 Stage Code / 输出 Schema / 空壳 Executor。

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

Server 可信 Owner Session RequestIdentity Adapter 和内部 CreateRun/GetRun/CancelRun 组合已在 Owner Session + Researchflow Capacity 启用路径接入；仍无实际业务调用方和正式 Workflow Template。SRV-CTR-001 继续 deferred。


## Owner Session Researchflow 身份适配

Server 已提供 `ownerSessionResearchflowIdentity`：只接受可信 Principal 中的 Owner Session（`Kind=owner`、`Subject=owner`、`AuthMethod=owner_session`），拒绝 Owner Bearer、Service Bearer 和匿名请求。Request ID 从 Observability Context 传播；CorrelationID 由 CreateRun IDGenerator 生成。该适配器尚未接入生产 CreateRun/GetRun/CancelRun 组合。代码 SHA `1c82c16bbbf67084d85a0579f5e0f2d5796358c1` 的 Integration CI Run [44016866061](https://github.com/Kasign/stock_data_provider/actions/runs/44016866061) 全量 success。


## 2026-11-17 Server Owner 内部 Run 组合

Server 已在 Owner Session 启用且 Researchflow Capacity 显式配置时，装配进程内 CreateRun/GetRun/CancelRun Application 服务。三项服务共用 MySQLRunRepository 与持久化创建者授权；CreateRun 的 RequestIdentity 只接受可信 Owner Session Principal。ResourcePolicy Resolver 从同一 News Stage composition 实际成功装配的 Provider 实例生成 inventory，不读取客户端字段或自行创建 HTTP Client。

新增 Researchflow MySQL DatabaseClock，使用 MariaDB UTC 时间；MariaDB 集成测试验证了真实数据库时间读取。Server IDGenerator 使用 crypto/rand 的 128-bit 熵。当前 News Search 组合没有 hard-risk Stage，因此不伪造 HardRiskGateRegistry；未来声明 hard-risk Stage 会 fail-closed。

代码 SHA `fc2f7793fe22c6760f8756d9f277b4b579012a51` 的 Integration CI Run [44024703289](https://github.com/Kasign/stock_data_provider/actions/runs/44024703289) 全量 success。

**边界：** 当前只有进程内服务组合，没有实际业务调用方、正式 Workflow Template 或 Worker Runtime；未新增 HTTP 路由，SRV-CTR-001 继续 deferred。WebUI W2 尚未接通。首期输入责任和 CreateRun 时间/幂等实现已在后续 W2-B 变更中冻结与验收。

## 2026-11-17 CreateRun 首期时间与幂等输入约束

- 首期只支持 current mode。CreateRun Command 不接收客户端 `system_as_of`、`deadline_at`、`originating_request_id` 或 `correlation_id`；后两者由 Server Observability/ID 设施注入。
- CreateRun 指纹覆盖可信 Caller、操作、请求 Schema 版本、精确 Template Ref/Version、规范化快照引用集合和 JCS 参数；不覆盖 Server 生成的 PIT/Deadline 时间或追踪 ID。
- 必须先按 caller-scoped idempotency key 查询；只有未命中才读取 MariaDB UTC 时钟并生成 `system_as_of` 与 `deadline_at`。重放返回原 Run；并发同键请求由 MySQL Repository 原子唯一键和指纹比较收敛。
- Server Run Deadline 必须由显式配置提供，不设隐式默认值；它是整个 Run 的绝对上限。Attempt Timeout 仍只约束单次 Attempt，首期不另增 Stage 总超时；Retry 不得重置 Run Deadline。
- `CreateRunAtomic` 在事务写入前及提交前继续用数据库 UTC 时钟复核绝对 Deadline，不能只依赖 Application 层校验。
- WebUI 输入契约与公开 API 状态以 [Web整体规划 W2-B](../web/Web整体规划.md) 和 [Web API 边界](../web/API_CONTRACT.md) 为准；SRV-CTR-001 未重新进入前不增加公开 HTTP API。

## 2026-11-17 W2-B CreateRun 与 Server Deadline 落地

- WebUI 首期只提交请求 Schema 版本、幂等键、精确模板 ref/version、输入快照引用和模板参数；不提交 `system_as_of`、`deadline_at`、CallerRef 或追踪 ID。SRV-CTR-001 仍 deferred，没有新增 HTTP 路由。
- Server 新增显式配置 `RESEARCHFLOW_RUN_DEADLINE`，使用 Go duration 字符串；Researchflow Capacity 启用时必须提供正数配置，不设默认值。该时长注入 CreateRun Application。
- CreateRun 指纹只覆盖请求意图，加入请求 Schema 版本，排除 Server PIT/Deadline 时间及追踪 ID。先查幂等记录；重放不读取数据库时钟、不分配新 ID、不解析当前模板。
- 新 Run 在模板、参数和 ResourcePolicy 解析完成后读取一次 MariaDB UTC 时间，生成 `system_as_of` 和 `deadline_at = db_now + RunDeadline`；Repository 继续在事务写入前和提交前复核 Deadline。
- 已补充幂等重放不读取时钟的单测，以及真实 MariaDB CreateRun 的 PIT/Deadline 间隔持久化断言。
- Integration CI Run [44063735491](https://github.com/Kasign/stock_data_provider/actions/runs/44063735491) 对 SHA `e00e6e5c99e5d128b8087e2038d613a8400f23d1` 全量 success。

**未完成：** 正式 Workflow Template 的 required/allow_empty_inputs、RetryPolicy、Propagation、Applicability 和 template_ref/version 尚未全部由用户定稿；不得把当前集成测试夹具直接提升为正式模板。下一步先冻结该发布契约，再发布 `news.search_candidates`。

## 2026-11-17 正式 News Search Template 发布切片

- Server Bootstrap 在 Owner Session + Researchflow Capacity 显式启用时，构建并预检不可变模板 `news.search_candidates@1.0.0`，再通过 MySQL Template Registry 幂等发布；同版本不同内容由 Registry 拒绝并阻止启动。
- 模板固定参数 Schema Ref、Output Registry Version、ResourcePolicy Ref、Applicability Rule Ref/Version；Stage 必需，`AllowEmptyInputs=true`、`RequiresHardRiskGate=false`。
- RetryPolicy：`MaxAttempts=2`（含首次执行）、Attempt Timeout 30s、Full Jitter 100ms～1s，只允许 `news_provider_retryable_failure` 重试。
- Propagation：Degraded=Accept；Skipped/Failed/TimedOut/Blocked=Fail。Degraded 状态不会被伪装为 Succeeded。
- 启动预检实际执行参数 Schema 校验、Applicability 精确版本评估、Output Registry 精确 Stage/OutputType/SchemaVersion 与 Schema Ref 匹配，并执行 Template.Validate。
- 无 Provider inventory 时模板仍可发布，但 ResourcePolicy Resolve/CreateRun 继续 fail-closed；不创建无 Provider 的可执行 Run。
- 本切片没有开放 SRV-CTR-001 HTTP 路由，也没有声称 Worker Runtime 已接入。Server Scheduler/WorkerPool/Deadline scanner/Finalization 的生产生命周期装配仍是下一阶段；不得将模板发布等同于生产执行链闭环。

## 2026-11-17 终态与取消恢复发现能力

- 新增 `RunFinalizationCandidateScanner`：通过 RunRef Keyset 分页发现正常完成、Deadline 终止和用户取消的 Run；只有所有 Stage 均为终态且不存在运行中 Attempt 才返回候选。
- 新增 MariaDB `ListFinalizableRunRefs`，只负责无锁候选发现；Finalization Coordinator 仍重新加载不可变 Run/Stage/Attempt 快照，并由原子 Finalize 事务决定是否提交。
- 新增 `CancellationRecoveryScanner`：按 (RunRef, StageRef) 分页发现 `RunCancelling` 下 Lease 已过期的 Attempt，并调用既有 `FenceExpiredCancellationAttempt` 做事务内复核和 Fencing。
- 以上扫描器目前已具备 Application / Storage 实现和测试，但尚未装配进 Server 周期 Runtime；不能据此认定崩溃恢复已在生产进程中运行。
- 最新验证：Integration CI 对提交 `4996a1d8550269bdf4b5198a279483c2e9a9f03a` 全量成功，包含根模块测试、Worker Bootstrap MariaDB/Redis 生命周期、go-core、Compose 与 WebUI 检查。


## 2026-11-17 Server 共享资源关闭保护

- 复核发现 WorkerPool 的 Shutdown 超时路径会取消池上下文后返回；这不等于所有在途 Worker goroutine 已退出。
- Bootstrap 的共享资源关闭策略已调整为：只有所有 Lifecycle 消费者均成功停止，才关闭共享 MariaDB/Redis/HTTP 资源；任一组件停止失败或超时，保留共享资源并向上返回错误，避免在途消费者访问已关闭的连接池。
- 该保护是异常关闭时的安全兜底，不代表消费者已成功停止；进程收到该错误后应退出。资源最终由进程退出回收。
- 本次变更只修复资源关闭顺序，不代表 Researchflow Worker Runtime 已装配。Scheduler、WorkerPool、Deadline、ExpiredAttempt、Cancellation Recovery 与 Finalization 的周期生命周期仍待接入。

## 2026-11-17 进程本地 Worker 并发配置

- 新增 Server 配置 `RESEARCHFLOW_LOCAL_WORKERS`，仅在 `SERVER_ROLE=worker|all` 且 Researchflow Capacity 已启用时必填，范围 1～32。
- `RESEARCHFLOW_LOCAL_WORKERS` 只控制当前进程 WorkerPool 的并发；`RESEARCHFLOW_MAX_WORKERS` 仍是 MariaDB 原子 Claim/Capacity 使用的跨进程全局并发上限。两者不能互相替代，也不能将全局值直接作为每个进程的本地池大小。
- API 角色不创建 WorkerPool，因此拒绝配置 `RESEARCHFLOW_LOCAL_WORKERS`；未启用 Researchflow Capacity 时也拒绝单独配置该项。
- 本次只完成配置契约和验证，Worker Runtime 尚未装配；后续装配必须使用 `Process.ResearchflowLocalWorkers` 创建本地 WorkerPool。


## 2026-11-17 Server Researchflow Runtime 接入与并发审查

- Worker-only 角色不依赖 Owner Session；Owner Session 只负责可信身份和 CreateRun/GetRun/CancelRun 内部用例，公共执行组合独立装配。
- Server Lifecycle 注册顺序为 Notification Outbox Relay → Researchflow Runtime；逆序关闭确保 Runtime Finalizer 不会向已停止的 Relay 写入终态 Outbox。
- Deadline、取消恢复和 Finalization 保留在周期扫描线程中；调度 Run 与过期 Attempt 恢复通过有界异步任务执行，避免单个 Stage 执行时间阻塞其他恢复扫描。
- 异步任务受 `RESEARCHFLOW_LOCAL_WORKERS` 限制，按 Run/Stage 候选键去重；Runtime Stop 取消工作上下文并等待所有异步任务退出，随后才关闭 WorkerPool。
- 仍需通过真实 MariaDB 场景验证：正常 Run 完成、Provider retry、Deadline、用户取消、进程中断后的 Attempt 恢复、跨进程 Claim 容量竞争，以及 Outbox 投递/重试。公开 Researchflow HTTP API（SRV-CTR-001）仍 deferred。


## 2026-11-17 正常 Run 生产 Runtime MariaDB 端到端验收

- 将既有 News Search MariaDB 集成测试从直接调用 Scheduler/WorkerPool，升级为调用 Server 生产组合的 Researchflow Runtime。
- 测试使用真实 MariaDB DatabaseClock、MySQLRunRepository、正式 Worker/Stage Output Registry、Scheduler Candidate Scanner、WorkerPool 和 Run Finalization Coordinator；Provider 使用确定性集成 Fixture，不访问外部服务，也不把 Fixture 当作真实市场数据。
- 验收通过：Run 从 Created/Queued 经周期扫描和调度进入 Running；Worker Claim 并执行 News Search Stage；Stage Payload/QualitySummary 与 Attempt 状态持久化；Finalization 生成完整 ResultSummary；Run 终态与唯一 terminal Outbox 在同一 Storage 事务中提交。
- 该测试发现并修复状态聚合的空切片克隆问题：非 nil 空 ExcludedStages 经 append(nil, empty...) 变成 nil，导致 Storage 拒绝完整性校验。修复后 normal aggregation 与 cancellation aggregation 都保持非 nil 空数组。
- 精确代码 SHA `ef5e471b09511d8de017e8577b4c1ec29ee4c48f` 的 Integration CI Run [44084307957](https://github.com/Kasign/stock_data_provider/actions/runs/44084307957) 全量 success。
- 尚未覆盖：Retry 重新执行、Deadline 扫描与终止、用户取消和 Cancellation Recovery、进程中断后的 Expired Attempt Recovery、多 Worker 进程争抢全局 Claim 容量，以及 Outbox Relay 实际发送/重试。


## 2026-11-17 Researchflow Runtime 四类终态路径验收

- 精确代码 SHA `0623b3f65876be56658d2443686cca6b281644bf` 的 Integration CI Run [44086825786](https://github.com/Kasign/stock_data_provider/actions/runs/44086825786) 全量 success。
- 同一 MariaDB Runtime 集成用例通过生产周期扫描器验证四类路径：
  1. 正常成功：Scheduler → Worker → Stage Output → Run Finalization。
  2. Retry：首次 Provider RateLimited，Attempt 1 失败并保留输出证据；按精确 reason code 重试一次，Attempt 2 成功。
  3. Deadline：创建时固化短 Deadline，等数据库 UTC 到期后启动 Runtime；Deadline Scanner 先终止 Stage，Deadline Finalizer 将 Run 置为 Failed。
  4. 用户取消：Run 尚未开始执行时提交 CancelRun，Runtime 通过 Finalization Candidate Scanner 和 Cancellation Finalizer 将 Run 置为 Cancelled。
- 四类路径都验证 ResultSummary 引用及 terminal Outbox 原子写入。Provider 使用确定性 Fixture，测试不访问外部服务。
- 尚未验收：在途 Attempt 用户取消后的 Worker Context 停止确认与 Lease Fencing；进程崩溃后 ExpiredAttemptScanner 重新 Claim；多 Worker 进程的数据库全局 Capacity 竞争；Outbox Relay 对终态事件的实际投递、失败重试和幂等重复处理。
