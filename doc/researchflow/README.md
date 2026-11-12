# Researchflow 模块

- 分支：`feat/researchflow`
- 代码：`internal/researchflow/**`
- 任务前缀：FLOW
- 当前状态：领域状态机、MySQL 核心生命周期事务、Application Worker 单次执行协调器、MySQL WorkerStore Adapter、Scheduler 单次 Run 运行协调器均已有代码；全局容量事务守卫（CreateRun 排队容量、ClaimAttempt 全局 Worker Lease）已补代码，等待当前 SHA CI/MariaDB 验收；Worker Pool/常驻消费、周期扫描与 Server 生命周期接入、生产 StageExecutor Adapter、跨进程取消通知仍未闭环。WorkerStore 与 Scheduler 已有模块级及 MariaDB 验收证据，详见任务进度文档。

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
- `application`：CreateRun、Scheduler 单次 Run 运行协调（快照读取、计划计算、Stage CAS、冲突重载重算、Worker 派发）、Worker 单次执行协调、取消、Deadline、Retry、Outbox 与 Run Finalization 用例。
- `storage`：MySQL Run/Stage/Attempt 核心事务、租约/fencing、Deadline、Finalize、Outbox 持久化及全局容量守卫（排队 Run 与有效 Worker Lease）。
- `publicdto`、`parameterschema`：公开 DTO、参数 Schema 校验。
- `contextassembly`、`toolpolicy`、`resultvalidation`：上下文装配、工具授权、Quant Envelope 与风险校验。
- `diagnostics`：Platform Diagnostics 适配入口。

## 当前明确缺口

- Scheduler 已提供单次 Run 的运行协调服务；周期扫描、Server 生命周期启停与多 Run 调度入口尚未接入。当前派发直接调用 Worker.Execute，不引入第二套队列；跨进程并发仍由 ClaimAttempt 事务兜底。
- Worker 有单次 `Execute` 协调器；MySQLRunRepository 已实现 WorkerStore，执行快照读取与结果提交映射已落地。全局容量事务守卫代码已新增（Migration v4、必填容量配置、CreateRun 排队容量检查、ClaimAttempt 全局 Worker Lease 检查），但当前 SHA 尚未通过 CI/MariaDB 验收；有界 Worker Pool/常驻消费、queued Run 恢复、跨进程取消通知、优雅停机和生产 StageExecutor Adapter 仍未闭环。
- `StageExecutor` 是注入端口，Data/News/Quant/Research/LLM 的生产 Adapter 尚未在本分支闭环。
- 无独立 Worker Pool/队列消费循环；跨 Worker 的 Run 并发上限由 ClaimAttempt 数据库事务兜底。
- Go 测试、Race、Vet、真实 MariaDB、进程崩溃恢复和跨模块端到端验收必须按实际 SHA 记录，不以测试文件存在代替通过证据。

后续实施顺序以任务文档最新状态盘点为准；不要依据早期历史记录重复建设已经落地的 Repository、Worker 或状态机能力。
