# Repair All 跨模块修复计划与进度

- 仓库：`Kasign/stock_data_provider`
- 工作分支：`repair_all`
- 当前工作分支 HEAD：`1ce641ff13a07e228784ef0c4041eded82e45621`（2026-11-19 文档更新后）
- 本轮生产代码基线：`9ce0d0b536e2d9b37f0fb5ff4cc2ab8ae59c6ba4`；该 SHA 的完整 Integration CI 尚未取得成功证据
- 更新日期：2026-11-19

## 1. 分支用途与执行原则

`repair_all` 是当前统一的跨模块 Review、修复和集成验收工作分支。用户授权在该分支直接修改现有模块代码和文档，以避免在多个模块分支间反复切换。

- 可以直接修复任意现有模块中的真实缺陷；修改位置由问题归属和架构职责决定。
- 不复制模块、不创建平行实现、不增加新的业务模块，不绕过现有 Port、Adapter、Repository、Platform 或 Server 装配边界。
- 多模块问题应同时检查生产者、消费者、契约、调用链、持久化和启动装配。
- 不为通过测试降低契约要求，不用 Mock/Fixture 代替真实生产验收。
- 复用已有公共能力；公共能力缺失时在其既有归属模块扩展。
- 每轮开始重新核对 `repair_all` HEAD 和 Actions；所有写入使用 expected SHA，防止覆盖并发更新。
- 变更按完整功能聚合提交，使用中文提交信息；模块专项文档和本文件同步记录。
- 历史模块分支文档中的“只允许修改模块目录”是旧分支协作约束，不限制用户已明确授权的 repair_all 聚合修复；但模块职责边界仍然有效。

## 2. 修复顺序

| 顺序 | 工作项 | 涉及模块 | 状态 |
|---|---|---|---|
| P0 | Data Migration 定义、Platform 只读校验、Server Root 统一注册与启动路径 | Data / Platform / Server | 代码与完整 CI 验收通过；生产启停演练待完成 |
| P0 | News Search Candidates Validator 与冻结 Schema 一致性 | News / Researchflow | 已修复，完整 Integration CI 已通过 |
| P1 | Data、News 到 Research 的数据/时间/Evidence 契约和真实消费边界 | Data / News / Research / Server | Review 完成；DATA-CTR-007 deferred、NEWS available_at 与真实消费者仍阻塞 |
| P1 | Quant Application Port、结果 Envelope、Researchflow 消费接口 | Quant / Researchflow / Server | Review 完成；QNT-014/QNT-013 与生产 Stage/Root 装配未闭环 |
| P1 | Research Context/Evidence 与 Researchflow Run/Stage/Result 语义及唯一持久化 Owner | Research / Researchflow / Server | Review 完成；Context 授权、PIT Mode 与生产 Stage 消费仍阻塞 |
| P2 | LLM Gateway 生产 Provider、Secret、Capability Evidence 与真实调用 | LLM / Platform / Server / Researchflow | 待系统 Review |
| P2 | Notification 事件、Recipient、Fanout/Delivery、Worker 和恢复链 | Notification / Platform / Server | Review 完成；Relay/Inbox/Fanout 机制通过，Server Delivery Worker/Recipient Directory 阻塞 |
| P3 | Diagnostics 注册、身份/审计传播、Worker 恢复、WebUI Root API 对接 | Platform / Server / 各业务模块 / WebUI | Root/Data Owner Session 单入口代码已实现；旧 Data 服务/Key 资产已清理；精确 SHA CI 与生产环境验收待完成 |
| P3 | 完整启动、迁移、跨模块端到端和故障恢复验收 | 全模块 | 待实施 |

## 3. 本轮已完成的修复

### RA-NEWS-001：Search Candidates Payload 唯一性校验

- 基线 SHA：`37d37f3107befbd42c98891917efc608c354330b`
- 提交 SHA：`a1d56c1f7034d2c77ecfee9bc6daf574237f9847`
- 修改：
  - `internal/news/stage/search_candidates_validation.go`
  - `internal/news/stage/search_candidates_test.go`
- 原因：冻结 Schema 对 `candidates[].missing_fields` 声明 `uniqueItems: true`，但 Go Validator 只检查枚举，未检查重复值。
- 结果：重复的 missing field 现在被拒绝，并有回归测试。
- 治理：未改 Schema、未复制 Validator、未更改 Researchflow 注册方式。
- 验收：已由完整 Integration CI Run [44450277491](https://github.com/Kasign/stock_data_provider/actions/runs/44450277491) 在精确 SHA `5b9ebe6a2b8103b4c63a48c3c0f0ddb004d583ec` 覆盖通过；后续聚合基线持续包含该修复。

## 4. P0 Migration 三方统一 Review 结论（已实施，保留历史问题记录）

P0 原始问题及实施约束作为审计记录保留；当前状态以第 7、9～13 节的精确 SHA 和 CI 证据为准，不再将本节历史问题描述为当前未修复问题。

- Data 已导出 V1-V4 静态 Migration definitions；V1-V3 保持原始定义，V4 纳入兼容索引/补列操作。
- Platform 已提供统一 MigrationOperation、namespace lock、checksum 和只读 ValidateAppliedMigrations。
- Server Root 已统一注册 Data definitions；enabled 由 Root Registry 执行，disabled 由只读校验，移除 external namespace 过渡路径。
- Migration checksum 与物理 Schema 漂移分开判定；Data Ready 负责物理结构校验。
- 代码层 MariaDB/Redis 集成与 Root Bootstrap 验收已通过；生产环境 Migration enabled/disabled 部署演练仍未执行，不能标记生产验收完成。

## 5. 当前执行任务与边界

### RA-CROSS-REVIEW-008：WebUI ↔ Server 身份认证、API 契约与凭据暴露 Review

- 状态：REVIEW_DONE_WITH_BLOCKERS（已完成 WebUI 请求、Nginx、Compose、Root Server Data 路由和 Owner Session 契约审查；M5 代码/部署切换等待 Data 业务路由认证策略确认）。
- 目标：核对 WebUI 实际请求与 Server 已发布路由/认证矩阵，检查 Owner Session/Owner Bearer 的边界、CSRF、Request ID、API Key/Secret 是否进入浏览器持久化、Data Provider Key 是否由 Server 侧注入。
- 范围：WebUI、Server、Platform、Data；只审查当前真实 UI 调用和已冻结/已发布 API，不新增无业务场景接口。
- 首要核对：SRV-CTR-002、SRV-CTR-001（若已冻结）、PLAT-CTR-001/002/003、Data OpenAPI/Root Adapter 契约、WebUI 当前任务验收清单与 API 接口协作清单。
- 重点检查：WebUI 是否只用 HttpOnly Session Cookie；是否错误把 Owner Bearer/API Key 存 localStorage/sessionStorage/URL/日志；非安全请求是否携带 CSRF；UI 调用路径/方法/DTO 是否与 Server Router/OpenAPI 一致；Data Provider Key 是否只由 Server Data Adapter 注入；CORS/Origin/Host/Request ID 是否符合契约；未实现 API 是否被 UI 假调用或用 Mock 冒充真实。
- 诊断判定：按每条 UI 请求记录 PASS / FAIL / NOT_COMPARABLE / DEFERRED，附 UI 调用位置、Server 路由、契约、测试和精确 SHA。
- 修复门禁：UI 问题修 WebUI；路由/认证/请求上下文问题修 Server；公共安全原语修 Platform；Provider 凭据边界修 Data/Server。多模块改动同步 DTO、路由、UI 调用和测试。
- 完成条件：浏览器无持久化高权限凭据；UI 请求均有真实 Server 端契约与认证策略；真实 Data Provider 调用不暴露 Key；针对性 UI/HTTP 测试及完整 CI 精确 SHA 验收。

## 14. RA-CROSS-REVIEW-003 基线与验收证据（2026-11-19）

- 本轮开始时 repair_all HEAD：b3a5b03e126a5754128367f578ef9b14b4d0755a，提交为本轮总体计划校准文档。
- 上一代码基线 5334043ea8e1abc2c4006c371a3f95b123098946 的完整 Integration CI Run [44456052937](https://github.com/Kasign/stock_data_provider/actions/runs/44456052937) 精确 SHA success；该 CI 早于本轮文档提交，不将其冒充为本轮 HEAD 的 CI。
- Data/News→Research 审查结论如第 5 节；News 授权原子读取机制有现有实现，不重复建设；Data→Research 受 DATA-CTR-007 deferred 阻断；Server 组合装配存在但没有真实消费者。
- 本轮未修改生产代码；当前完成的是契约/代码/装配 Review 和卡点同步。生产端到端验收仍未通过。
- 本文件为 repair_all 聚合任务状态权威；模块职责与模块内部任务状态仍以各自模块总规划和唯一当前任务文档为准。


## 15. RA-CROSS-REVIEW-004 结果生产与消费审查结论（2026-11-19）

- 审查代码基线：repair_all HEAD b52c8afde2f2e32fca498a78987d34f96fba4335；本轮未修改生产代码。
- Quant 计算入口：internal/quant/application/ 仅有 GetResult 授权读取用例，没有 Indicator/Factor/Trend/Risk 计算 Application Facade；QNT-014 未完成。
- Quant 结果持久化：internal/quant/result/ 仅有 Repository Save Port 与 MemoryRepository；GetResult 依赖 AuthorizedResultReader、PrincipalResolver、AccessAuditor Port，repair_all 代码树未发现生产 SQL Repository、生产授权 Reader 或审计持久化 Adapter。QNT-013 生产装配待完成。
- Quant Envelope：当前 DecodePayload/BuildEnvelope 仅注册 trend/1.0.0 与 risk/1.0.0。IndicatorPayload 类型虽存在，但没有进入受支持联合类型；Factor Payload 未注册。该 fail-closed 行为与未完成的 Indicator 正式 Registry Entry/Factor deferred 状态一致，不是应放宽的 Bug。
- Researchflow：resultvalidation/quant.go 只负责对已授权取得的 Quant Envelope 做强类型校验/映射，NewQuantPayloadRegistry 当前仅支持 trend/risk；FromQuantEnvelope 明确不负责授权。它不是可执行 Quant Stage Provider。
- Server：Researchflow Runtime 当前将唯一 StageExecutor/StageOutputRegistry 注入 Worker；researchflowComposition 仅包含 News Search Stage，Stage Output Registry 仅注册 news.search_candidates。bootstrap.go 无 Quant 装配调用。不存在 Quant Stage 的生产接线。
- 判定：
  - Quant 已有 Trend/Risk Envelope 类型与读取逻辑：代码机制 PASS；生产存储/授权/审计 NOT_COMPARABLE。
  - Indicator：DEFERRED（正式 Registry/Envelope 注册前保持拒绝）。
  - Factor：DEFERRED（QNT-CTR-003、DATA-CTR-005/007 未解锁）。
  - Researchflow Quant Validator：代码机制 PASS；无 Quant Stage 的生产调用，NOT_COMPARABLE。
  - Server Quant 组合装配：NOT_COMPARABLE（未实现；依赖 Quant Facade/生产 Adapter 和明确 Stage 场景）。
- 本轮未发现可在现有契约和模块职责内安全修复的局部代码缺陷。不得把纯函数、Validator、MemoryRepository 或类型声明拼成伪生产链路。
- 卡点已同步至 Quant、Researchflow、Server 项目级卡点和模块当前任务记录；模块分支未被单独推送。

## 16. RA-CROSS-REVIEW-005 登记（2026-11-19）

- 前置审查 RA-CROSS-REVIEW-003/004 均已完成；生产消费证据缺口继续保持 DEFERRED/NOT_COMPARABLE。
- 本任务聚焦 Research 与 Researchflow 的 Context/Evidence/Run/Stage/Result 生产语义和唯一持久化 Owner，防止跨模块重复存储或时间/身份语义漂移。
- 当前状态 IN_PROGRESS；本轮开始已核对 repair_all HEAD 8afbba2a0a519f932063861085bbdc89f7e76fa8 和最新 Actions。


### RA-CROSS-REVIEW-005 首轮核对（2026-11-19）

- Research Context Domain/Repository/Application 已存在；GetContext 要求注入 ContextAuthorizer，未配置时明确 fail-closed。
- Server 当前 News→Research 组合调用 NewService 时未传 ContextAuthorizer；这意味着未来调用 GetContext 会返回 research_context_authorizer_not_configured。当前没有真实 Context 消费者，因此不能通过默认 Allow 或临时 Authorizer 绕过；需要确认 Research Context 的授权策略 Owner、可信 Principal 注入和实际调用场景后才能装配。
- Researchflow contextassembly 只做规划与 DTO 投影；ToResearchCreateContextCommand 接收 ResearchContextSnapshot，不接收 Run，无法独立证明 snapshot.SystemAsOf 等于 Run.SystemAsOf。其实施文档明确要求 Server/Worker 在 CreateContext 前校验，读取后再次校验；当前未发现生产 Stage/Worker 调用该 Adapter。
- Researchflow Run 在 CreateRun 时由 DatabaseClock 生成唯一 SystemAsOf；Worker StageExecutionRequest 携带 Run/SystemAsOf/DeadlineAt。当前缺少 Context Stage 将这些字段绑定到 Research Adapter 的真实调用链。
- 本轮暂未发现可在不新增无调用场景接口、不绕过 Owner 授权的前提下直接修复的局部代码缺陷；继续审查 Evidence/Claim/Event 引用和唯一持久化 Owner。


### RA-CROSS-REVIEW-005 Evidence PIT 首轮结论（2026-11-19）

- Research Evidence Domain 已有 ValidateForStrictPIT(runAsOf)，明确要求 available_at 非空且不晚于 Evidence.system_as_of，并要求 Evidence.system_as_of 不晚于 Run.system_as_of。
- Researchflow ResultComposition 的 validateResearchEvidence 当前只检查 Evidence.system_as_of 不晚于 Run、system_observed_at 不晚于 Evidence.system_as_of 和授权引用非空；不检查 available_at 是否满足严格 PIT。
- 项目级字段契约明确：available_at=null/unknown 时不得作为严格 PIT 可得事实；只有 Researchflow Template 显式声明非严格 PIT 模式及其风险时，才可使用仅满足“系统当时已观察”的 Evidence。
- 当前 WorkflowTemplate 结构未定义显式 PIT Mode 字段；ResultCompositionInput/ValidationPolicy 也没有 PIT Mode。当前唯一生产模板是 News Search，不消费 Research Evidence，且没有生产 ResultComposition 调用者，因此这是未来 Research 消费链路的契约/实现阻塞，不是可在当前调用链中直接补一行校验的局部缺陷。
- 不擅自增加 PIT Mode 枚举、默认值或模板字段；需要先由契约 Owner 明确严格模式默认值、非严格模式名称/风险字段及其参与 Template Fingerprint 的规则。未明确前，任何使用 Evidence 的研究结果都不得宣称严格 PIT 生产验收通过。


## 17. RA-CROSS-REVIEW-005 Context / Evidence / Run / Stage 审查结论（2026-11-19）

- 审查代码基线：repair_all SHA 19ce60f35a47a08bf63b60e2f9f9ba2faf5a6155；本轮未修改生产代码。
- Research Storage：Context、Evidence、Event、Claim 均有 Research-owned Repository 与 Migration；Server Root 的 Migration Registry 已注册 researchstorage.Migrations()。Event/Claim/Context 使用版本化 append；Event Lifecycle 与 Notification Outbox 在同一事务；Evidence 使用不可变 Payload/Fingerprint，重复 ID 必须完全相同。
- Researchflow Run：CreateRun 由 DatabaseClock 生成唯一 system_as_of/deadline_at；Worker StageExecutionRequest 携带 Run、AuthenticatedCallerRef、SystemAsOf、DeadlineAt、InputSnapshotRefs，时间传播基础能力存在。
- Context：Research GetContext 要求真实 ContextAuthorizer，缺失时 fail-closed；当前 Server News→Research composition 未注入 ContextAuthorizer。由于没有真实 Context 消费者，不能注入恒 Allow 占位授权器。
- Context PIT：contextassembly DTO 投影不接收 Run，无法独立校验 snapshot.SystemAsOf；FLOW-005 文档明确要求 Server/Worker 在创建前和读取后校验与 Run.SystemAsOf 完全一致。当前没有生产 Context Stage 消费者。
- Evidence PIT：Research Evidence ValidateForStrictPIT 已要求 available_at 非空且不晚于 Evidence.system_as_of。Researchflow ResultComposition.validateResearchEvidence 当前没有执行该检查；项目级契约要求只有 Template 显式声明非严格 PIT 模式及风险时才可使用 available_at=null 的 Evidence。当前 WorkflowTemplate/ResultCompositionInput 没有 PIT Mode，且无生产 ResultComposition 调用者。需契约 Owner 明确字段、默认值、风险声明和 Fingerprint 规则后再实现，当前不猜测。
- Event/Claim：Domain 具备时间、版本递增、状态迁移和引用验证；MySQL Repository 以事务校验并追加版本。当前无真实 Researchflow Stage 生产消费者将 Event/Claim 纳入最终结果。
- 判定：Research Persistence/版本机制代码 PASS；Researchflow Run 时钟传播代码 PASS；Context 授权生产装配 NOT_COMPARABLE；Context Run PIT 绑定 NOT_COMPARABLE；Evidence 严格 PIT 消费 DEFERRED；Event/Claim 生产消费 NOT_COMPARABLE。
- 本轮未发现可在当前冻结契约和无真实调用者条件下安全修复的局部代码缺陷。所有阻塞已同步 Research、Researchflow、Server 卡点及当前任务记录。

## 18. RA-CROSS-REVIEW-006 登记与状态（2026-11-19）

- 前置 RA-CROSS-REVIEW-005 已完成；剩余 Context 授权、PIT Mode、生产 Stage 消费问题分别保留在 Research/Researchflow/Server 卡点，不在 Notification Review 中重复处理。
- 本任务聚焦 Research 与 Researchflow 两类 Outbox 到 Notification Inbox、Fanout、Delivery 和 Worker 恢复的完整链路。
- 状态：REVIEW_DONE_WITH_BLOCKERS；审查结论见第 19 节。

## 19. RA-CROSS-REVIEW-006 Outbox→Inbox→Fanout→Delivery 审查结论（2026-11-19）

- 审查基线：repair_all HEAD 69f4dfaa6c8677ddfdf719a8492c6c9514b8b9f8；本轮未修改生产代码。
- Research：Event 状态版本与 Lifecycle Outbox 在同一 Research Storage 事务内提交；Outbox Envelope 使用 JCS Fingerprint、稳定 IdempotencyKey、CorrelationID；Relay 对永久 Schema/身份/指纹错误执行 Reject，对临时 Notification 接收失败执行 Retry。
- Researchflow：Run Finalize 在一个 MariaDB 事务中提交 ResultSummary、Run 终态和 terminal Outbox；Server Publisher 对 Envelope/Payload/时间/终态字段做严格校验。OutboxDispatcher 对所有 Publish 错误统一 Retry，只记录 OutboxErrorCoder.Code；Researchflow Outbox Store 没有 Reject/DeadLetter 端口，当前契约明确没有最大尝试次数/死信状态。因此 notification_event_invalid 也会进入重试。该策略需后续明确永久错误处置/告警/死信决议，不在本轮擅自增加状态或丢弃事件。
- Server：Worker/All 角色装配 Research Relay；Researchflow Capacity 配置后装配 Researchflow Relay；两者共用 Notification EventReceiver，API 角色不启动 Relay。Inbox Schema Validation、双唯一键幂等和 Inbox+FanoutWorkItem 同事务写入均已存在。
- Notification：Fanout Dispatcher、Delivery Execution、Retry/Lease Recovery、WeCom Channel Adapter 与 MySQL Repository 均存在；WeCom 真实渠道连通性已有独立 Live Smoke 证据。但 Server 当前没有 RecipientDirectory、Fanout Worker、Delivery Worker、Channel Registry/Secret/Template Renderer 生产组合装配；没有 Server 运行时 Delivery 完整闭环。
- 诊断判定：
  - Research Event→Outbox：代码/事务机制 PASS。
  - Researchflow Run Finalize→Outbox：代码/事务机制 PASS。
  - 两类 Relay→Notification Inbox：代码与已有集成测试机制 PASS；本轮未执行新的测试。
  - Inbox→Fanout WorkItem：同事务机制 PASS。
  - Fanout→Delivery→真实 Channel：NOT_COMPARABLE（Server Worker/RecipientDirectory/模板/渠道生产组合未装配）。
  - Researchflow 永久 Publish 错误处理：DEFERRED（Outbox 无 Reject/DeadLetter，需契约 Owner 决策）。
- 修复结论：未发现可在当前模块边界和冻结策略内安全直接修复的生产代码缺陷；NTF-BLOCK-04/05/06/08/09 继续阻塞完整投递生产验收。
- 文档已同步 Notification、Researchflow、Server 卡点和模块当前任务记录；本轮未改动模块分支。

## 20. RA-CROSS-REVIEW-007 登记与状态（2026-11-19）

- 本任务聚焦 Server Root HTTP、身份认证、公共 Fault/Diagnostics、角色隔离与生命周期。
- 状态：REVIEW_DONE；审查结论见第 21 节。

## 21. RA-CROSS-REVIEW-007 Server Root HTTP / 身份 / Diagnostics 审查结论（2026-11-19）

- 审查基线：repair_all SHA 0ae392c44b00f8002a31e4b8346522ac851b99a0；本轮未修改生产代码。
- 路由矩阵与 SRV-CTR-002 一致：/healthz 匿名；/readyz、/diagnostics 使用 Owner Bearer；/api/v1/owner/* 六个 Session 端点独立使用 Session Cookie + Origin/Host/CSRF；旧 Data/Analysis 业务路由继续 Owner Bearer。Owner Session 不继承 platform:admin，也没有被隐式放宽到业务路由。
- Platform Router 要求每条路由显式 RoutePolicy，拒绝 root catch-all；未登记路径保持 404。当前无 Research/Quant/Notification 业务 HTTP 路由，不因 WebUI 预期而提前增加 API。
- Request ID Middleware 在入口重新生成可信 ID，不信任外部 X-Request-ID；请求体大小/并发上限、标准 Fault 安全投影、Cache-Control/no-store 均已接入。
- Diagnostics/Readiness：Server 聚合 Shared Storage/Migration、Data、News 检查；Research 的 Context/Event/Claim/Evidence Diagnostics 是按具体引用构造的资源检查，不是通用依赖检查；Researchflow StageAssessment 是执行结果诊断投影；Notification Lease Recovery Diagnostics 依赖尚未装配的 Worker。当前未发现应无条件加入 Root readiness 的缺失模块检查。
- Lifecycle：组件按注册顺序启动、逆序停止；HTTP 在消费者之前停止，消费者成功停止后才关闭共享 DB/Redis；Shutdown 失败时保留资源并向进程入口返回错误。当前注册组件的 Start 路径为快速启动或遵守进程 Context；未来新增阻塞式 Start 必须遵守 Context，避免 Lifecycle 等待启动结束时超过 Shutdown budget。
- 判定：当前已实现路由/身份边界 PASS；未登记业务 API NOT_COMPARABLE/DEFERRED（等待 SRV-CTR-001 真实用例）；Notification Worker/Delivery 仍按 RA-CROSS-REVIEW-006 卡点。
- 本轮未发现可复现且应在当前 Server/Platform 职责内立即修复的生产代码缺陷。Owner Session 路由认证行为与冻结候选契约一致。
- 上一完整 Integration CI Run [44456052937](https://github.com/Kasign/stock_data_provider/actions/runs/44456052937) 在代码 SHA 5334043ea8e1abc2c4006c371a3f95b123098946 成功；其后本轮为文档审查提交，没有代码变更，不将该 CI 误标为本轮 HEAD 验收。

## 22. RA-CROSS-REVIEW-008 登记（2026-11-19）

- 前置 RA-CROSS-REVIEW-007 已完成 Server Root 路由、身份认证、Diagnostics、Fault、生命周期审查。
- 新任务聚焦 WebUI 实际请求与 Server 已实现路由/Owner Session/Data 凭据边界；必须核对当前 WebUI 源码，不推断或模拟 UI 调用。
- 当前状态 PLANNED；开始执行前重新核对 repair_all 最新 HEAD、Actions 和总览计划。


### RA-CROSS-REVIEW-008 首轮发现（2026-11-19）

- WebUI 当前行情请求默认走 /market-api，使用 X-API-Key；api.js 从 localStorage 读取并持久化 Data API Key。该 Key 是独立 Data HTTP 服务访问凭据，不是第三方 Provider Key，也不是 Owner Bearer。
- 当前 Compose 仍部署 stock-analysis-data（8091）和 stock-analysis-server（8092）两个服务；Nginx /market-api 指向独立 Data 容器，/analysis-api 指向 Root Server。此为 M5 迁移前兼容部署。
- Root Server 已在同进程构造 Market Data Module，并挂载 InProcessHandler；Root Data 路由由 Platform Router 的 OwnerAccess（Owner Bearer）保护，不接受 X-API-Key，也不会自动接受 Owner Session。WebUI Owner Session Cookie 当前经 /analysis-api/api 路径使用，不覆盖 /market-api。
- 因此，不能只把 Nginx /market-api upstream 改为 Root Server：当前浏览器发出的 X-API-Key 会被 Root Router 忽略并返回未认证；也不能向浏览器下发 Owner Bearer 或用 Nginx 静态 Owner Bearer 伪装用户身份。
- M5 目标“浏览器不持有 Data API Key、Root Server 单入口”与当前 Root 路由 Owner Bearer 策略之间存在认证契约缺口。需要明确 Data 业务路由是否允许 owner_session（含 POST 探测的 CSRF/资源授权），或由 Server/Platform 采用受控 Service Principal；本轮不擅自选择。
- 文档迁移状态发现：MarketData 迁移计划顶部及第 3 节仍写 M3/M4 Root 装配未完成，与当前 Root 代码和 Server 模块进度冲突；本轮将校准为 M3/M4 代码级完成、M5 部署切换待决策，历史快照保留并标记过期。


## 23. RA-CROSS-REVIEW-008 WebUI / Server / Data 认证与部署审查结论（2026-11-19）

- 审查基线：repair_all HEAD f8af53bf3a57bf94a2e0f1f3bf5e35cca629680f；本轮未修改生产代码。
- WebUI 当前行情请求通过 /market-api 访问独立 Data HTTP 服务，使用 X-API-Key；该 Data API Key 从 localStorage 读取并持久化。它不是第三方 Provider Key，也不是 Owner Bearer。
- Compose 仍部署 stock-analysis-data（8091）、stock-analysis-server（8092）和 WebUI；Nginx 的 /market-api 指向独立 Data 容器，/analysis-api 指向 Root Server。M5 尚未完成。
- Root Server 已具备同进程 Data Module/Application/InProcessHandler/Root Router；Data 路由使用 Owner Bearer，InProcessHandler 不识别 X-API-Key。Owner Session 仅挂载 SRV-CTR-002 六个 Session 路由，不自动授权 Data 业务路由；Cookie Path 当前只覆盖 /analysis-api/api。
- 直接将 /market-api upstream 切换至 Root Server 会发生认证不匹配；静态注入 Owner Bearer 或让浏览器持有 Owner Bearer 均违反 SRV-CTR-002。不得仅修改 Nginx 或删除 API Key。
- 文档已校准：MarketData M3/M4 代码级 Root 装配完成；M5 Compose/Nginx/WebUI 切换仍待完成，且受 Data 路由认证策略和 Cookie/API Prefix 约束。旧迁移文档中的 M3/M4 未完成状态已标记为过期历史快照。
- 当前唯一需要用户决策的事项：Data 浏览器业务路由采用显式 Owner Session（POST probe 必须 CSRF，保留 Owner Bearer 给运维调用），还是采用受控 Service Principal 由可信反向代理注入。推荐前者，并统一 Root API Prefix/Cookie Path；但在确认前不修改认证契约。
- 本轮未修改 WebUI、Server、Data 生产代码；没有执行新的代码测试。最近完整 Integration CI Run 44456052937 对代码 SHA 5334043ea8e1abc2c4006c371a3f95b123098946 成功；本轮提交仅为文档校准，不能称为当前 HEAD 的 CI 验收。


## RA009：Root Server Data Owner Session 与单入口迁移（2026-11-19）

<escape>状态：CODE_IMPLEMENTED_PENDING_CI_AND_PRODUCTION；Owner Session 方案已实现，旧实现已清理，CI 与生产验收待完成。</escape>

- 用户明确批准 Owner Session 方案，并明确旧实现不需要保留。
- 唯一入口为 Root Server；WebUI 使用 HttpOnly Owner Session Cookie。Data 路由显式允许 Owner Session，保留 Owner Bearer 受控运维调用；Session 不继承 admin/service scope。
- 所有非安全方法由现有 SessionAuthenticator 校验 Origin/Host 与 CSRF；特别覆盖 Provider Probe 写操作。Data 领域授权仍由 Data 负责。
- 统一 API Prefix/Cookie Path 为 `/api`，取消独立 `/market-api` 和 `/analysis-api` upstream。
- 已删除浏览器 Data API Key 运行逻辑、Data HTTP Adapter 的 X-API-Key 认证、独立 Data 服务入口、独立容器/镜像资产、旧 Nginx 双 upstream、旧 Compose migration Profile 和旧 Data Key 配置设计文档；不保留双轨或回滚部署。
- 旧资产清理与新入口切换同批实施；先确保新入口代码和测试通过，再完成单 Root 部署切换及真实验收，最后删除所有旧资产。

RA009 关闭条件：Server 路由认证矩阵、CSRF、WebUI 请求和配置、Compose/Nginx 单入口、独立 Data 运行资产清理、部署验收、真实 Provider/数据证据及最新精确 SHA CI 全部完成。详细分步清单见 Data 迁移计划与 Server 卡点文档。
