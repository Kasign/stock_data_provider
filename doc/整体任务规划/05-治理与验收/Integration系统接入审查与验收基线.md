# Integration 系统接入审查与整体验收基线

- 验收分支：`integration`
- 本文最近一次核对的 integration 代码验收 SHA：`dde7d4f8fb071912ffa34bb9c50d46507d1a388b`；其后仅有文档同步提交（包括 `d1fa84d`），不改变本次代码验收结论。
- 当前 CI：Integration Run `43370552552`，对应精确 HEAD `dde7d4f8fb071912ffa34bb9c50d46507d1a388b`，结论 `success`；gofmt、root/go-core vet/test、News/Research MariaDB 集成测试、Compose、WebUI 语法/单测和 Docker 构建全部通过。上一轮 Run `43369529083` 对代码 SHA `d664b9f` 同样 success。
- 历史审查基线 SHA：`63f49065a5ef3379d95948a6c2c6e504a555a63f`（Run #54），仅作为历史参考
- 历史参考：Run #23（`43178908392`）仅对应旧 SHA `fdc19b8591223dabccb533389d472713bfecc415`，结论 `success`
- 本文性质：Integration 当前代码树的源码接入审查、真实依赖 CI 证据与后续验收基线
- 范围：只记录根组合入口、模块装配、路由、Migration、Worker、事件连接和诊断接入；不替代各模块内部功能验收
- 限制：Run `43369529083` 对代码 SHA `d664b9f` 的全量检查通过，CI 已启用 `NEWS_MARIADB_TEST_DSN` 与 `RESEARCH_TEST_MYSQL_DSN`，执行 News/Research 模块级 MariaDB 集成测试。CI 通过不代表跨模块 Importer 持久化、真实外部 Provider、LLM Gateway、目标部署网络和生产凭据已验收。Researchflow Outbox → Notification Inbox 的 MariaDB/Redis 集成测试也继续执行。

## 1. 当前结论

**当前 Integration 代码 SHA 的通用 CI 已通过；Researchflow → Notification 的 MariaDB 接收/重试/幂等及 Relay 生命周期测试已有通过证据，News 与 Research 模块级 MariaDB 集成测试也已纳入 CI 并通过。News→Research 跨模块 Importer 持久化、Researchflow 生产 StageExecutor、Quant/LLM Provider 和 Notification 真实渠道仍未完成，因此系统整体尚未达到端到端业务联调条件。**

当前 `cmd/analysis-api/main.go` 已委托 `internal/server/bootstrap.Run`。Bootstrap 仍以旧 Analysis Runner、旧 Task Queue 和旧 HTTP API 为主业务链；同时已在配置 Key Ring 时构造 News Evidence Query Facade、Research Service、Importer 和 Authorizer，但尚无 Stage/HTTP 消费端调用该组合。

News、Research、Researchflow、Notification 的 Migration 已进入根 Registry，不代表相应业务执行链均已接入。News→Research 组合构造已完成但无消费者；Research 与 Researchflow Outbox → Notification Inbox Relay 已接入；News Parser/Provider 配置诊断已接入。Researchflow Scheduler/Worker/生产 StageExecutor、Quant 生产 Facade、LLM 可信 Provider 装载、Notification Fanout/Delivery Worker 仍未形成完整运行链。

因此当前阶段应判定为：

- 当前代码 SHA CI：`pass`（Run `43369529083`，`d664b9f`）；News/Research MariaDB 模块级集成测试已启用并通过。
- Root Bootstrap 基础设施：部分完成
- 新业务模块运行时装配：部分完成；仍以旧 Analysis Runner/Queue/API 为主
- 新业务 API / Worker：Researchflow 业务 API 与 Worker 未接入
- Research / Researchflow → Notification Outbox/Inbox：两个 Server Relay 均已接入；Researchflow MariaDB 测试覆盖首次接收失败、`notification_receive_unavailable`/next_attempt_at 持久化、到期重试成功、Outbox 确认和 Inbox/FanoutWorkItem 幂等；Relay Start/Stop 组件生命周期测试通过；真实渠道投递仍未接入
- MariaDB/Redis：CI 已启动 MariaDB/Redis 服务；Researchflow Relay 测试实际连接 MariaDB 并通过；不代表生产部署环境验收
- 外部 Provider、LLM Gateway、目标部署网络：未取得当前 SHA 的真实运行验收
- 端到端业务验收：尚未具备条件

## 2. 源码核查事实

### 2.1 进程入口与 Bootstrap

| 核查项 | 当前事实 | 结论 |
|---|---|---|
| 唯一进程入口 | `cmd/analysis-api/main.go` 调用 `bootstrap.Run(ctx)` | 已收敛到 Root Bootstrap |
| 进程配置 | Bootstrap 调用 `serverconfig.LoadProcess` | 已有 Server 级配置快照 |
| 共享资源 | 调用 `OpenLegacyAnalysisResources`，取得共享 DB/Redis 资源及旧 Store | 已有资源容器，但仍通过 Legacy Analysis 兼容入口打开 |
| 业务 Runner | 调用 `service.NewRunnerWithMarketDataStores` | 仍是旧 Analysis 业务 Runner |
| 任务队列 | 调用 `taskqueue.New`，注册 `legacyAnalysisWorker` | 仍是旧 Analysis Task Queue/Worker |
| HTTP API | 调用 `httpapi.NewWithTasks` | 仍是旧 Analysis API |
| 生命周期 | 注册旧 Worker 和 HTTP Server 到 `runtime.Lifecycle` | 基础启停能力已接入，但不是新模块 Worker 集合 |

### 2.2 当前 HTTP 路由

`internal/server/bootstrap/router.go` 当前显式注册：

- `GET /healthz`、`HEAD /healthz`
- `GET /readyz`、`HEAD /readyz`
- `GET /diagnostics`、`HEAD /diagnostics`
- `GET /`
- `POST /api/v1/analysis/runs`
- `GET /api/v1/analysis/runs/{run_id}`
- `GET /api/v1/analysis/runs/{run_id}/events`
- `POST /api/v1/analysis/tasks`
- `GET /api/v1/analysis/tasks/{task_id}`
- `GET /api/v1/analysis/tasks/{task_id}/events`

以上业务路由均指向旧 Analysis Handler。当前没有看到 Researchflow Run 创建/查询等新业务路由，也没有 Data、News、Research、Quant、LLM、Notification 的业务 Handler 注册。

News 不计划提供独立外部 API；这不影响其作为 Server 内部模块被 Research 等模块通过公开 Application Port 调用。

### 2.3 Migration 注册

当前 `newMigrationRegistry()` 注册四个 namespace：

| Namespace | 来源 | 当前状态 |
|---|---|---|
| `news` | `internal/news/adapters/storage.Migrations()` | 已注册 |
| `research` | `internal/research/storage.Migrations()` | 已注册 |
| `researchflow` | `internal/researchflow/storage.Migrations()` | 已注册 |
| `notification` | `internal/notification/storage.Migrations()` | 已注册 |
| `quant` | 当前未注册 | 未发现模块 Migration 注册入口 |
| `llm` | 当前未注册 | 未发现模块 Migration 注册入口 |
| Data | 当前未注册 | Data 模块化单体迁移仍有独立门禁 |

Registry 已具备 namespace 校验、确定性排序、执行报告和 Schema 预检。真实 MariaDB 下的迁移成功、部分失败恢复、重复执行和多实例并发行为仍需运行验收。

### 2.4 模块接入矩阵

| 模块 | 当前可见实现 | Bootstrap 实际接入 | 判断 |
|---|---|---|---|
| Platform | Config、Fault、Security、HTTP Client、Storage、Diagnostics、Task Queue 等基础包 | DB/Redis 资源、Security Authenticator、Diagnostics 基础服务和共享 HTTP Client 资源已使用；未见各模块生产 BoundDoer/完整 Check 集合注入 | 基础设施部分接入 |
| Data | 独立行情服务及迁移中的 Go Data 能力 | 仍由旧 Market Data Adapter 配置和 Runner 使用；未看到新 Data Application Port 装配 | 迁移门禁未通过，不作为新模块接入完成 |
| News | Application、Provider/Source Adapter、Storage、Evidence 查询能力和 Migration | Parser 自检、Provider 配置诊断已接入；当 Key Ring 配置时 Bootstrap 构造 PIT/授权/分页 Evidence Query Facade。Provider 采集运行时和生产 BoundDoer 未装配，Evidence Query 暂无业务消费者 | Evidence Query 依赖图已构造；采集和实际消费链未接入 |
| Research | Application Service、Port、Storage、Outbox 和 Migration | Bootstrap 在 Key Ring 配置时构造 Context/Evidence Repository、Research Service、News Importer/Authorizer；尚未向 Researchflow 注入公开 Port | News→Research 依赖已构造，尚无消费者和跨模块 MariaDB 验收 |
| Researchflow | Run/Stage/Attempt 状态、CreateRun、Scheduler 单次 Run 协调、MySQL WorkerStore、Worker 单次执行、固定数量 Worker Pool、Context Assembly、Result Validation | Bootstrap 未装配 Scheduler 周期驱动、Worker Pool 生命周期和生产 StageExecutor Adapter；未提供 Researchflow 业务路由 | 模块核心执行设施已有实现，Server/生产 Adapter 接入未完成 |
| Quant | Indicator/Factor/Registry/Risk/Result 等能力及查询用例 | Bootstrap 未构造 Quant Application Facade 或结果存储服务 | 未接入 |
| LLM | Provider Port、Responses JSON/SSE 协议适配与校验 | Bootstrap 未创建具体 Provider；未注入 ProviderConfig、CapabilityState 和受控 BoundDoer | 未接入 |
| Notification | EventReceiver、Inbox、Subscription、Delivery/Attempt、FanoutWorkItem、FanoutDispatcher、Migration | Bootstrap 已创建 EventReceiver，并通过 Research Outbox Relay 投递至 Notification Inbox/FanoutWorkItem；FanoutPlanFactory、生产渠道/身份目录、Fanout/Delivery Worker 仍未装配 | Research → Inbox 已接入；Inbox → 渠道投递未接入 |

说明：模块代码存在、模块单测通过、模块 Migration 已注册，均不能单独作为 Server 生产接入证据。Researchflow 的 WorkerStore/Scheduler/Worker Pool，以及 Notification 的 Inbox/FanoutDispatcher/Delivery 执行设施，属于模块内部已经实现的能力；缺口主要在生产 Adapter、组合根装配和生命周期驱动。接入必须以真实构造、依赖注入、调用链、生命周期和运行证据为准。

## 3. 已发现的文档同步问题

`doc/server/README.md` 的历史实施记录（SRV-007、SRV-12.7、SRV-12.23）仍描述只注册 News、Research、Notification 三个 Migration namespace；当前 Integration 的 `newMigrationRegistry()` 已额外注册 Researchflow。

这属于 Server 专项文档相对当前 Integration 代码的状态滞后。本文记录当前代码事实，不直接改写 Server 专项任务历史记录；后续由 Server 文档维护者按当前 Server 分支基线同步。

## 4. 当前阻断项

| 编号 | 阻断项 | 影响 | 责任归属建议 |
|---|---|---|---|
| INT-001 | 新模块公开构造入口和生产 Facade 未齐备 | Server 无法真实实例化并注入模块 | 对应模块分支 |
| INT-002 | Researchflow Scheduler 周期驱动、Worker Pool 生命周期、生产 StageExecutor Adapter 和业务入口未装配 | 模块已有 WorkerStore/Scheduler/Worker 执行设施，但无法由 Server 驱动完整生产流程 | Researchflow + Server |
| INT-003 | Quant 缺少完整可注入 Application/结果持久化边界 | Researchflow 无法消费生产 Quant 结果 | Quant |
| INT-004 | LLM 缺少 Provider 生产配置和 BoundDoer 注入 | 无法进行真实模型调用 | LLM + Server |
| INT-005 | Notification 生产 FanoutPlanFactory、渠道/身份目录和 Server Worker 装配未完成 | 模块内 Inbox/FanoutDispatcher/Delivery 执行设施已存在，但无法形成真实事件到渠道的投递链路 | Notification + Server |
| INT-006 | News→Research 依赖图已构造但尚无 Stage/HTTP 消费者；Quant/LLM 等公开 Port 仍未形成完整生产注入图 | 只能证明构造成功，不能证明跨模块业务调用 | 对应模块 + Server |
| INT-007 | Outbox → Inbox 已完成 Research 与 Researchflow 两个生产方接入；Notification 真实渠道投递仍未完成 | 事件可进入 Inbox/FanoutWorkItem，但尚不能送达外部渠道 | Notification + Server |
| INT-008 | Data 模块化单体迁移未满足门禁 | 新 Data Application Port 无法注入 Root Server | Data |
| INT-009 | 尚无更多可注册的进程级领域 Diagnostics Check；Research/Researchflow 的现有检查属于 Run-scoped 诊断 | 不得把需要 Run/Stage 上下文的检查伪装成进程级 Readiness；全局检查维持真实资源/Schema/News Provider 语义 | 对应模块 + Server；仅在出现真实进程级依赖检查时扩展 |
| INT-010 | 真实 MariaDB/Redis、Provider、LLM Gateway 和部署网络验收未执行 | 无法证明运行时和生产环境可用 | Server + Platform + 各模块 + 部署 |

## 5. 后续验收阶段与准入条件

### INT-A：模块构造与依赖装配审查

目标：形成真实模块依赖 DAG，不凭目录或接口名称推测可装配。

每个模块必须提供可验证证据：

- 公开构造入口及必需参数；
- 公开 Application Port / Facade；
- Repository 和共享资源所有权；
- Migration 注册项（如模块需要持久化）；
- Diagnostics Check；
- Worker/Handler（如模块提供）及关闭语义；
- 依赖其他模块的 Port 清单和注入方向。

通过标准：Server 可以按依赖顺序构造模块；不存在跨模块私有包依赖、空实现冒充生产实现或未满足的必需依赖。

### INT-B：Root Server 运行时装配

目标：把已通过 INT-A 的模块装配进 Bootstrap。

验收范围：

- Config/Secret 在 Bootstrap 集中解析并注入；
- Platform DB/Redis/HTTP Client 由 Server 创建并统一管理；
- 各模块使用固定 DestinationPolicy/BoundDoer；
- Migration namespace 与模块实际版本一致；
- API/Worker 运行角色选择正确；
- Readiness 聚合必需模块 Check；
- 关闭顺序满足消费者先于共享资源；
- 启动失败能逆序回滚。

通过标准：不依赖旧 Analysis Runner 执行新业务链路；所有必需模块均为真实构造实例。

### INT-C：跨模块业务链路

至少验证：

1. Researchflow 请求能够通过公开 Port 调用 Research、Quant、LLM；
2. Research/News 证据引用、AuthorizationRef 和派生关系完整保留；
3. Quant RiskDecision 的硬门禁不可被评分覆盖；
4. Researchflow 正确综合成功、降级、失败和跳过的 Stage；
5. 最终结果能够持久化、查询并通过 API 返回；
6. 业务事件能够按契约进入 Notification Inbox 并完成投递状态流转。

通过标准：真实请求、数据库事实、执行事件、最终结果之间可用 Request ID/Correlation ID/Run ID 串联；没有伪造数据或静默降级。

### INT-D：故障、幂等与恢复

至少覆盖：

- Data/News Provider 超时或不可用；
- LLM Gateway 超时、SSE 中断和无终态 EOF；
- MariaDB/Redis 断连及恢复；
- Worker 执行中进程终止；
- 重复创建 Run、重复投递事件；
- Notification 渠道超时后的 Unknown/Ambiguous 对账；
- Migration 部分失败和恢复；
- 优雅关闭期间存在在途 HTTP 请求和 Worker 任务。

通过标准：状态机、重试、幂等和恢复语义符合模块契约；失败不被伪装为成功；所有结果可诊断。

### INT-E：部署与最终验收

在目标部署环境完成：

- 容器/进程启动、健康探针、配置和 Secret 注入；
- 真实 MariaDB/Redis 连接池、迁移和故障恢复；
- 生产网络出口 SSRF/DNS/重定向安全验证；
- API/Worker 角色和副本策略验证；
- 备份、回滚和旧 Analysis 链路退役演练。

通过标准：核心业务链路可重复运行，故障可定位和恢复，旧链路退役不丢失任务或事实数据。

## 6. 本轮审查结论与下一步

历史轮次在 `integration` 完成 Researchflow → Notification 事件契约修正、Server Relay 接入和组件生命周期测试。Run #54（`43359166036`）对应历史 SHA `63f49065a5ef3379d95948a6c2c6e504a555a63f`，结论 `success`；gofmt、root/go-core vet/test、MariaDB/Redis 集成、WebUI 检查及 Docker 构建全部通过。当前代码验收以 Run `43369529083` / SHA `d664b9f` 为准。CI 使用 `SERVER_RELAY_TEST_MYSQL_DSN`，实际执行 `TestResearchflowOutboxToNotificationInboxMariaDB` 的迁移、Outbox 领取、EventReceiver 首次拒绝、`notification_receive_unavailable` 与 next_attempt_at 持久化、到期重试、Outbox 确认、Inbox/FanoutWorkItem 唯一记录和重复处理幂等检查；另有 Relay Start/Stop 组件生命周期单测。

**当前已通过的范围仅为 Researchflow Outbox → Notification EventReceiver → Inbox/FanoutWorkItem，包括接收失败重试。** 尚未验收真实渠道发送、Fanout/Delivery Worker 生产装配、外部渠道 Unknown 对账，以及完整 Researchflow 业务执行链。

下一步不重复 Relay 组件启停与 Outbox 重试测试。可继续完成 Server Worker 角色的 Bootstrap 级启动/关闭集成验证；其后只有在模块提供冻结 Stage 定义、Output Registry 和真实公开 Facade 后，才接入 News → Researchflow、Data → Quant、Research/Quant/LLM → Researchflow。Quant 风险决策 Outbox 生产方、Notification Recipient Directory/渠道 Provider/Delivery Worker、Web 新业务 API 仍是模块或组合根阻塞项，不用空实现绕过。

验收状态必须区分：

- `pass`：有当前 SHA 的可复核成功证据；
- `degraded`：功能按契约降级，且降级原因和影响明确；
- `fail`：违反契约或核心链路失败；
- `not_comparable`：缺少真实依赖、数据或环境，当前无法比较/验证；
- `blocked`：存在明确前置条件未满足，尚不能开始该项验收。


## 7. 2026-11-13 最新接入复核

本节更新当前 integration 的跨模块接入事实。完整可续接任务清单、文件入口和执行顺序见《[Integration 当前任务交接与继续执行](./Integration当前任务交接与继续执行.md)》。

- 最新精确 SHA：`dde7d4f8fb071912ffa34bb9c50d46507d1a388b`；Integration CI Run [43370552552](https://github.com/Kasign/stock_data_provider/actions/runs/43370552552) success。
- GOV-CTR-006 公共 JCS Runtime Verifier 已在 internal/platform/authorization 实现；News Adapter 已在 internal/news/adapters/platform 实现。四份 GOV Golden 和 News Binding Golden 已纳入测试。
- Server Bootstrap 已在 Key Ring 配置时构造 News Evidence Query Facade、EvidenceCursorCodec、Research Service、Importer 和 Authorizer；Key Ring 缺失时不生成默认密钥。当前没有 Stage/HTTP 消费端调用该组合，真实 MariaDB News→Research 持久化链路仍待验收。
- Research News Evidence Importer/Authorizer 已存在；News MySQL Repository、Research Storage 和 Research Application 的构造入口均可复用，不得重写。
- Researchflow Scheduler/WorkerPool 的生产生命周期与 StageExecutor Adapter 尚未完成；Quant 的生产计算 Port/Payload Registry 门禁未解除。
- 不重复 Outbox → Notification Inbox/FanoutWorkItem 的既有 MariaDB 验收；真实渠道投递仍未接入。
- 不把本轮 GOV Verifier Golden 测试通过描述为 News → Research 或 Researchflow 端到端通过。

- 下一步优先完成 News→Research 的 MariaDB 端到端联调和游标 Key 轮换验收；随后依据冻结 Stage/Template/Output Registry 契约推进 Researchflow，不重复已有 Outbox→Notification MariaDB 验收。
