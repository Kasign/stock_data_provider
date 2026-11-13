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

## 当前代码能力

- `domain`、`template`、`state`：Run/Stage/Attempt 模型、模板/DAG 与状态迁移。
- `application`：CreateRun、Scheduler 单次 Run 运行协调（快照读取、计划计算、Stage CAS、冲突重载重算、并发 Stage 派发）、SchedulerCandidateScanner（可调度 Run keyset 分页）、ExpiredAttemptScanner（过期 running Attempt 复合游标分页）、固定数量 Worker Pool（无缓冲背压、优雅 Shutdown）、Worker 单次执行协调（Lease 心跳检测跨进程取消并确认）、取消、Deadline、Retry、Outbox 与 Run Finalization 用例。
- `storage`：MySQL Run/Stage/Attempt 核心事务、租约/fencing、Deadline、Finalize、Outbox 持久化及全局容量守卫（排队 Run 与有效 Worker Lease）。 Run 终态 Outbox 已按冻结 `researchflow.run.terminal` / Schema `1.0.0` 生成完整通知 Payload。
- `publicdto`、`parameterschema`：公开 DTO、参数 Schema 校验。
- `contextassembly`、`toolpolicy`、`resultvalidation`：上下文装配、工具授权、Quant Envelope 与风险校验。
- `diagnostics`：Platform Diagnostics 适配入口。

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