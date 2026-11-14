# Integration 系统接入审查与整体验收基线

- 验收分支：`integration`
- 当前 integration HEAD：`c43fb88f72f33648513e528b9f634acaec71f14e`（docs-only，父提交 `b0aabaa2edf89c44cd2aeb79b1a57210a8017d1b`）。
- 最近代码验收 SHA：`b0aabaa2edf89c44cd2aeb79b1a57210a8017d1b`；Integration CI Run `43561072301` 对精确 SHA success，覆盖全仓 gofmt、root/go-core vet/test、Compose、WebUI 语法/单测和 Docker 构建。当前 HEAD 的文档提交不触发 CI（workflow paths 不包含 doc/**），不得把文档 SHA 标记为已执行代码 CI。
- 历史代码验收 SHA：`dde7d4f8fb071912ffa34bb9c50d46507d1a388b`（Run `43370552552`）；更早 Run `43369529083` 对 SHA `d664b9f`，均保留为历史证据。
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

- 当前代码 SHA CI：`pass`（Run `43561072301`，`b0aabaa2e`）；A 组 News→Research 跨模块 MariaDB 负向/授权/分页/旧 Key 轮换验收已通过；B 组成功写入仍因 available_at 来源证据缺失而 blocked。News/Research 模块级 MariaDB 集成测试不等同跨模块 Importer 成功写入。
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

## 8. 2026-11-14 A/B 分组与 available_at 阻断

- A 组跨模块 MariaDB 验收用例已落地于 `internal/server/bootstrap/research_news_mariadb_integration_test.go`，使用真实双 namespace Migration、真实组合根和冻结 NEWS-CTR-002 Golden。**状态已升级为 `pass`**：提交 `b0aabaa2edf89c44cd2aeb79b1a57210a8017d1b` 的 Integration CI Run [43561072301](https://github.com/Kasign/stock_data_provider/actions/runs/43561072301) success，head_sha 精确匹配，全部检查项通过。本机无 MariaDB 时按 DSN 守卫 `t.Skip` 只代表开发态；已用非空假 DSN 验证三个 DB 子测试进入真实连接阶段而非 Skip，CI job env 提供 `SERVER_RELAY_TEST_MYSQL_DSN` 与 mariadb:11.4 service，确认该 success 包含真库执行。公开仓库逐步日志下载需管理员权限（403），不声称逐用例日志已核对。无 DB 的 Golden 指纹/播种字段/策略载荷自检与游标旧 Key 轮换用例亦在本地通过。
- B 组（Research Evidence 成功持久化、AuthorizationRef 与派生关系留痕）状态为 `blocked`，不是 `fail`：News 侧 `researchEvidenceRecordFromPIT` 依据 GOV-CTR-002 不设置 `AvailableAt`，Research 侧 `mapNewsEvidence` 要求非 nil 否则 `ErrEvidenceNotPITSafe` 且不落库。前置条件为 News Owner 提供可验证的 `available_at` 来源证据；Integration 不得伪造、回填或修改 News 领域代码绕过。
- 完整用例清单、代码依据和后续顺序见《[Integration 当前任务交接与继续执行](./Integration当前任务交接与继续执行.md)》第 3 节第 3 项。A 组已 `pass`，不再重复；第 3 节第 4 项 Researchflow Stage 契约盘点已判定 `blocked`（见本文第 9 节与交接文档第 6 节），第 6 项 LLM / Notification 装配复核亦已完成并判定 `blocked`（见本文第 10 节与交接文档第 7 节）。**第 3 节 1～6 项至此全部处理完毕：1、2、3A `pass`，3B、4、5、6 `blocked`；当前无新增 Integration 开发任务**，收到新证据后只推进其中一条真实依赖链。当前阶段判定仍为 INT-B 进行中，系统整体尚未达到 INT-C 端到端业务联调条件。

## 9. 2026-11-14 第 4 项 Researchflow Stage 准入判定（`blocked`）

- 盘点基线：核对时 HEAD `c43fb88f7`（docs-only），最近代码 SHA `b0aabaa2e`，Run 43561072301 success；契约权威为 `doc/contract/PLAT-002-项目级契约冻结登记簿.md` 版本 `0.2.60-frozen`（`L1_contracts_frozen`，43 项 CTR：37 frozen / 4 deferred / 2 not_applicable，冻结审计基线 `b6dfc85b`，**不等于当前 HEAD**）。本轮纯只读盘点，未改动任何 `.go` 文件。
门禁判定（依据交接文档第 3 节第 4 项）：

| 门禁 | 判定 | 核心依据 |
|---|---|---|
| Stage 输入/输出 | 部分满足 | FLOW-CTR-004 登记簿第 58 行「只有字段表，无权威 Schema/Output Registry」；`stage-1.0.0.schema.json` 只有不透明 `output_ref` |
| Registry 版本 | 部分满足 | Template Registry 有精确版本/指纹校验，但 Output Registry 只有 `template.go:58` 字符串字段，无版本常量、无注册项、无可装载实例 |
| 可信 CallerRef | 契约层满足、运行时有 L2 缺口 | `service:researchflow` 与可信 Context 注入已冻结且多处 fail-closed；但远程可信 caller 传递机制、下游 Adapter 字段映射未形成单一权威契约，生产代码中该值零出现 |
| 生产 Provider | 未满足 | `application/executor.go:47-49` 的 `StageExecutor` 只有接口、生产实现为零；`resultvalidation.Registry` 生产注册表为空；Quant `DecodePayload` 仅 `trend/1.0.0` / `risk/1.0.0`；Bootstrap 无 Scheduler/Worker/WorkerPool 装配 |

- 结论：**不具备接入条件，记为 `blocked`（不是 `fail`）**。不创建空壳 StageExecutor、不虚构 Stage Code 或输出 Schema、不把 Quant 当生产 Provider、不改其他模块领域代码。阶段判定仍为 INT-B 进行中。
- `deferred` 影响范围：`DATA-CTR-005` / `DATA-CTR-007` / `QNT-CTR-003` 阻断 Quant 生产 Stage，`SRV-CTR-001` 阻断新 Researchflow HTTP API；`not_applicable` 两项不阻断。真正卡住第 4 项的是 FLOW-CTR-004 等 frozen 契约的 L2 实现缺口。
- 重新进入条件（四条，须同时满足，详见交接文档第 6.5 节）：冻结 `stage_code + template_version + output_registry_version` 及 Output Registry 注册项/Schema；各模块公开 Application Port 且 Quant 完成 Payload 注册与 DATA-CTR-005/007 复核；提供生产 `StageExecutor` 与可信 CallerRef 运行时绑定；补跨模块 Contract Test 与 MariaDB 端到端验收并取得精确 SHA 的 CI success。
- 不重复事项：`doc/server/README.md` 第 12 章 Server 侧门禁核查、Researchflow 模块级 gofmt/test/race/vet 与 MariaDB 验收（Run `43008530832` / `43008538854`）、Outbox→Notification 验收（Run `43359166036`）、第 3 项 A 组（Run `43561072301`）。

## 10. 2026-11-14 LLM / Notification 生产装配准入复核（均为 `blocked`）

- 盘点基线：当前 HEAD `c43fb88f7`（docs-only），最近代码 SHA `b0aabaa2e`，Run [43561072301](https://github.com/Kasign/stock_data_provider/actions/runs/43561072301) 对精确代码 SHA success；本轮无业务 Go 代码变更。
- **LLM**：ProviderConfig/CapabilityState/ResponsesAdapter 已有代码，但 Bootstrap 没有 Provider 生产装配；CapabilityState 必须绑定真实 Gateway 版本、验证时间、有效期和 evidence_ref。现有 Gateway 盘点明确没有真实调用、没有读取 API Key，也没有可核验运行版本/能力证据。Integration 根模块 `go test ./...` 与 `go vet ./...` 已覆盖并通过 LLM 普通测试/Vet；没有 LLM 专项 Race 或 Gateway E2E 证据。结论：保持能力 unknown，不构造 Provider。
- **Notification**：RecipientDirectory 仍是 Port，ChannelAdapter/Resolver 是内部 SPI；DeliveryExecutionService 只执行已领取任务，当前无真实 Recipient Directory、具体 Channel Adapter、Delivery Worker 和凭据。Bootstrap 只接入 EventReceiver/Inbox 与 Outbox Relay。Run `43359166036` 的既有证据仅覆盖 Outbox→Inbox/FanoutWorkItem，不代表真实渠道发送。结论：不创建 Fake 生产渠道或空壳 Worker。
- **后续条件**：LLM 需要真实 Gateway 版本、目标模型、SecretRef 与逐项 capability evidence；Notification 需要真实渠道协议/凭据、回执/幂等/查询语义和权威身份目录。条件未满足前，Server 组合根没有可安全装配的生产 Provider/Worker，Integration 不新增代码。
- **CI 边界**：`integration-ci.yml` 的 paths 不包含 `doc/**`，本轮文档同步提交不会触发 CI；代码验收仍以 SHA `b0aabaa2edf89c44cd2aeb79b1a57210a8017d1b` / Run `43561072301` 为准，不能把新文档 HEAD 记作 CI 通过。
- **工具链与证据级别**：本机已安装 `gh` 2.104.0，网页设备码登录需用户一次浏览器交互授权（非交互 shell 不能代替）；未认证时 job 逐步日志下载返回 403，授权后可用 `gh run view 43561072301 --log` 核对。A 组 `pass` 的证据组合、「逐用例日志待补齐」口径、`git fetch` 的 HTTPS 兜底方式与多会话协作注意事项，统一见交接文档第 8 节，不在本文重复。
- **协作事实**：第 6 项结论由远端 docs-only 提交 `a98823abe` 落地；本地曾落后一个提交，已按交接文档第 4 节规则核对证据后快进对齐，未重复盘点。下次新聊天务必先查远端 HEAD/CI，不得假定本文 SHA 为最新。
