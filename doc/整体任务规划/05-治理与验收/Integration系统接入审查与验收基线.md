# Integration 系统接入审查与整体验收基线

- 验收分支：`integration`
- 审查基线 SHA：`fdc19b8591223dabccb533389d472713bfecc415`
- 对应 CI：Run #23（`43178908392`），结论 `success`
- 本文性质：Integration 当前代码树的静态接入审查记录与后续验收基线
- 范围：只记录根组合入口、模块装配、路由、Migration、Worker、事件连接和诊断接入；不替代各模块内部功能验收
- 限制：本轮通过 GitHub 源码 API 进行静态检查，未连接实际 MariaDB/Redis、外部 Provider、LLM Gateway 或部署环境；因此不构成运行时/生产验收

## 1. 当前结论

**Integration CI 已通过，但系统尚未达到端到端联调条件。**

当前 `cmd/analysis-api/main.go` 已委托 `internal/server/bootstrap.Run`，Server 已拥有共享资源、进程配置、HTTP/Worker 生命周期、Health/Readiness 和 Migration Registry 等基础设施；但 Bootstrap 实际创建的业务执行链仍是旧 Analysis Runner、旧 Analysis Task Queue 和旧 Analysis HTTP API。

News、Research、Researchflow、Notification 的 Migration 已进入根 Registry，不代表相应 Application Service、业务 Adapter、Worker、HTTP Handler 或事件链路已经接入。Quant 和 LLM 也尚未在 Bootstrap 中实例化。

因此当前阶段应判定为：

- CI 基线：通过
- Root Bootstrap 基础设施：部分完成
- 新业务模块运行时装配：未完成
- 新业务 API / Worker：未完成或未接入
- Outbox → Inbox 事件交付：未接入
- 真实数据库/Redis/Provider/LLM 运行验收：未执行
- 端到端业务验收：未具备条件

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
| News | Application、Provider/Source Adapter、Storage、Evidence 查询能力和 Migration | 当前仅注册 Migration；未实例化 News Application/Provider Adapter，也未注入生产 BoundDoer | 部分具备，未接入业务运行时 |
| Research | Application Service、Port、Storage、Outbox 和 Migration | 当前仅注册 Migration；未构造 Research Service、未向 Researchflow 注入公开 Port | 部分具备，未接入业务运行时 |
| Researchflow | Run/Stage/Attempt 状态、CreateRun、Scheduler 单次 Run 协调、MySQL WorkerStore、Worker 单次执行、固定数量 Worker Pool、Context Assembly、Result Validation | Bootstrap 未装配 Scheduler 周期驱动、Worker Pool 生命周期和生产 StageExecutor Adapter；未提供 Researchflow 业务路由 | 模块核心执行设施已有实现，Server/生产 Adapter 接入未完成 |
| Quant | Indicator/Factor/Registry/Risk/Result 等能力及查询用例 | Bootstrap 未构造 Quant Application Facade 或结果存储服务 | 未接入 |
| LLM | Provider Port、Responses JSON/SSE 协议适配与校验 | Bootstrap 未创建具体 Provider；未注入 ProviderConfig、CapabilityState 和受控 BoundDoer | 未接入 |
| Notification | EventReceiver、Inbox、Subscription、Delivery/Attempt、FanoutWorkItem、FanoutDispatcher、Migration | Bootstrap 未创建 EventReceiver/FanoutPlanFactory/Dispatcher/Delivery Worker；生产渠道与身份目录也未装配 | 模块内核心执行设施部分具备，生产 Factory/Server Worker 接入未完成 |

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
| INT-006 | Research/News/Quant 等模块间公开 Port 尚未全部形成生产注入图 | 无法确认真实调用链及依赖方向 | 对应模块 + Server |
| INT-007 | Outbox → Inbox 连接未装配 | Research/Researchflow 事件无法驱动 Notification | Server；前置为生产方与消费者 Port 就绪 |
| INT-008 | Data 模块化单体迁移未满足门禁 | 新 Data Application Port 无法注入 Root Server | Data |
| INT-009 | 模块 Diagnostics Check 未注册 | Readiness 只能覆盖基础资源，不能代表业务模块健康 | 对应模块 + Server |
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

本轮只完成静态接入审查，没有修改业务代码，也没有执行部署或真实依赖测试。

**下一步先执行 INT-A：逐模块核对公开构造入口、Application Port、Repository、Migration、Diagnostics 和 Worker，形成可执行依赖 DAG。** 对缺少模块能力的事项只登记归属和阻断，不在 Integration 越界代替模块实现；只有模块公开契约真实可用后，才进入 INT-B 的 Server 装配验收。

验收状态必须区分：

- `pass`：有当前 SHA 的可复核成功证据；
- `degraded`：功能按契约降级，且降级原因和影响明确；
- `fail`：违反契约或核心链路失败；
- `not_comparable`：缺少真实依赖、数据或环境，当前无法比较/验证；
- `blocked`：存在明确前置条件未满足，尚不能开始该项验收。
