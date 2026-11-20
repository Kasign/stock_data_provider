# Repair All 跨模块修复计划与进度

- 仓库：`Kasign/stock_data_provider`
- 工作分支：`repair_all`
- 当前工作分支 HEAD：以 GitHub `repair_all` 分支最新引用为准（避免在本文件提交后保留自引用 SHA）
- 本轮 RA009 生产代码验收 SHA：`1e916220a8784ae840132ce0cec3b3654c6b9655`；完整 Integration CI [Run 44482650366](https://github.com/Kasign/stock_data_provider/actions/runs/44482650366) 全项 success
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
| P2 | LLM Gateway 生产 Provider、Secret、Capability Evidence 与真实调用 | LLM / Platform / Server / Researchflow | Review 完成；模块协议/传输代码 PASS，Server 生产装配、Researchflow 消费者和真实 Gateway 证据 BLOCKED |
| P2 | Notification 事件、Recipient、Fanout/Delivery、Worker 和恢复链 | Notification / Platform / Server | Review 完成；Relay/Inbox/Fanout 机制通过，Server Delivery Worker/Recipient Directory 阻塞 |
| P3 | Diagnostics 注册、身份/审计传播、Worker 恢复、WebUI Root API 对接 | Platform / Server / 各业务模块 / WebUI | Data Owner Session 单入口、旧资产清理及精确 SHA CI 已通过；M6 真实生产验收仍待外部证据 |
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

## 5. RA-CROSS-REVIEW-008 历史审查任务与 RA009 当前结论

### RA-CROSS-REVIEW-008：WebUI ↔ Server 身份认证、API 契约与凭据暴露 Review（已完成，历史阻塞由 RA009 决策解除）

- 状态：REVIEW_DONE；其发现的 Owner Session 与 Data 认证不匹配问题已由用户决策和 RA009 实现解决。
- 目标：核对 WebUI 实际请求与 Server 已发布路由/认证矩阵，检查 Owner Session/Owner Bearer 的边界、CSRF、Request ID、API Key/Secret 是否进入浏览器持久化、Data Provider Key 是否由 Server 侧注入。
- 范围：WebUI、Server、Platform、Data；只审查当前真实 UI 调用和已冻结/已发布 API，不新增无业务场景接口。
- 首要核对：SRV-CTR-002、SRV-CTR-001（若已冻结）、PLAT-CTR-001/002/003、Data OpenAPI/Root Adapter 契约、WebUI 当前任务验收清单与 API 接口协作清单。
- 重点检查：WebUI 是否只用 HttpOnly Session Cookie；是否错误把 Owner Bearer/API Key 存 localStorage/sessionStorage/URL/日志；非安全请求是否携带 CSRF；UI 调用路径/方法/DTO 是否与 Server Router/OpenAPI 一致；Data Provider Key 是否只由 Server Data Adapter 注入；CORS/Origin/Host/Request ID 是否符合契约；未实现 API 是否被 UI 假调用或用 Mock 冒充真实。
- 诊断判定：按每条 UI 请求记录 PASS / FAIL / NOT_COMPARABLE / DEFERRED，附 UI 调用位置、Server 路由、契约、测试和精确 SHA。
- 修复门禁：UI 问题修 WebUI；路由/认证/请求上下文问题修 Server；公共安全原语修 Platform；Provider 凭据边界修 Data/Server。多模块改动同步 DTO、路由、UI 调用和测试。
- 原审查完成条件：浏览器无持久化高权限凭据；UI 请求均有真实 Server 端契约与认证策略；真实 Data Provider 调用不暴露 Key；针对性 UI/HTTP 测试及完整 CI 精确 SHA 验收。RA009 已完成代码层条件；M6 真实 Provider 和目标环境验收仍单独阻塞。

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


### RA-CROSS-REVIEW-008 首轮发现（2026-11-19，历史快照；已被 RA009 决策和实现取代）

- WebUI 当前行情请求默认走 /market-api，使用 X-API-Key；api.js 从 localStorage 读取并持久化 Data API Key。该 Key 是独立 Data HTTP 服务访问凭据，不是第三方 Provider Key，也不是 Owner Bearer。
- 当前 Compose 仍部署 stock-analysis-data（8091）和 stock-analysis-server（8092）两个服务；Nginx /market-api 指向独立 Data 容器，/analysis-api 指向 Root Server。此为 M5 迁移前兼容部署。
- Root Server 已在同进程构造 Market Data Module，并挂载 InProcessHandler；Root Data 路由由 Platform Router 的 OwnerAccess（Owner Bearer）保护，不接受 X-API-Key，也不会自动接受 Owner Session。WebUI Owner Session Cookie 当前经 /analysis-api/api 路径使用，不覆盖 /market-api。
- 因此，不能只把 Nginx /market-api upstream 改为 Root Server：当前浏览器发出的 X-API-Key 会被 Root Router 忽略并返回未认证；也不能向浏览器下发 Owner Bearer 或用 Nginx 静态 Owner Bearer 伪装用户身份。
- M5 目标“浏览器不持有 Data API Key、Root Server 单入口”与当前 Root 路由 Owner Bearer 策略之间存在认证契约缺口。需要明确 Data 业务路由是否允许 owner_session（含 POST 探测的 CSRF/资源授权），或由 Server/Platform 采用受控 Service Principal；本轮不擅自选择。
- 文档迁移状态发现：MarketData 迁移计划顶部及第 3 节仍写 M3/M4 Root 装配未完成，与当前 Root 代码和 Server 模块进度冲突；本轮将校准为 M3/M4 代码级完成、M5 部署切换待决策，历史快照保留并标记过期。


## 23. RA-CROSS-REVIEW-008 WebUI / Server / Data 认证与部署审查结论（2026-11-19，历史快照；不再作为当前部署状态）

- 审查基线：repair_all HEAD f8af53bf3a57bf94a2e0f1f3bf5e35cca629680f；本轮未修改生产代码。
- WebUI 当前行情请求通过 /market-api 访问独立 Data HTTP 服务，使用 X-API-Key；该 Data API Key 从 localStorage 读取并持久化。它不是第三方 Provider Key，也不是 Owner Bearer。
- Compose 仍部署 stock-analysis-data（8091）、stock-analysis-server（8092）和 WebUI；Nginx 的 /market-api 指向独立 Data 容器，/analysis-api 指向 Root Server。M5 尚未完成。
- Root Server 已具备同进程 Data Module/Application/InProcessHandler/Root Router；Data 路由使用 Owner Bearer，InProcessHandler 不识别 X-API-Key。Owner Session 仅挂载 SRV-CTR-002 六个 Session 路由，不自动授权 Data 业务路由；Cookie Path 当前只覆盖 /analysis-api/api。
- 直接将 /market-api upstream 切换至 Root Server 会发生认证不匹配；静态注入 Owner Bearer 或让浏览器持有 Owner Bearer 均违反 SRV-CTR-002。不得仅修改 Nginx 或删除 API Key。
- 文档已校准：MarketData M3/M4 代码级 Root 装配完成；M5 Compose/Nginx/WebUI 切换仍待完成，且受 Data 路由认证策略和 Cookie/API Prefix 约束。旧迁移文档中的 M3/M4 未完成状态已标记为过期历史快照。
- 当前唯一需要用户决策的事项：Data 浏览器业务路由采用显式 Owner Session（POST probe 必须 CSRF，保留 Owner Bearer 给运维调用），还是采用受控 Service Principal 由可信反向代理注入。推荐前者，并统一 Root API Prefix/Cookie Path；但在确认前不修改认证契约。
- 本轮未修改 WebUI、Server、Data 生产代码；没有执行新的代码测试。最近完整 Integration CI Run 44456052937 对代码 SHA 5334043ea8e1abc2c4006c371a3f95b123098946 成功；本轮提交仅为文档校准，不能称为当前 HEAD 的 CI 验收。


## RA009：Root Server Data Owner Session 与单入口迁移（2026-11-19）

<escape>状态：CODE_CI_PASS_PENDING_PRODUCTION；Owner Session 方案已实现，旧实现已清理，精确 SHA CI 通过，生产验收待外部证据。</escape>

- 用户明确批准 Owner Session 方案，并明确旧实现不需要保留。
- 唯一入口为 Root Server；WebUI 使用 HttpOnly Owner Session Cookie。Data 路由显式允许 Owner Session，保留 Owner Bearer 受控运维调用；Session 不继承 admin/service scope。
- 所有非安全方法由现有 SessionAuthenticator 校验 Origin/Host 与 CSRF；特别覆盖 Provider Probe 写操作。Data 领域授权仍由 Data 负责。
- 统一 API Prefix/Cookie Path 为 `/api`，取消独立 `/market-api` 和 `/analysis-api` upstream。
- 已删除浏览器 Data API Key 运行逻辑、Data HTTP Adapter 的 X-API-Key 认证、独立 Data 服务入口、独立容器/镜像资产、旧 Nginx 双 upstream、旧 Compose migration Profile 和旧 Data Key 配置设计文档；不保留双轨或回滚部署。
- 旧资产清理与新入口切换已在同一代码批次完成；不保留旧服务、旧 Key、双 upstream 或回滚部署资产。目标环境真实部署和数据验收仍未完成。

RA009 代码关闭条件已满足：Server 路由认证矩阵、CSRF、WebUI 请求和配置、Compose/Nginx 单入口、独立 Data 运行资产清理均已通过完整 Integration CI。精确代码 SHA `1e916220a8784ae840132ce0cec3b3654c6b9655` 对应 [Run 44482650366](https://github.com/Kasign/stock_data_provider/actions/runs/44482650366) 全项 success。M6 生产验收仍 BLOCKED_EXTERNAL_EVIDENCE：目标环境 HTTPS/Owner Session 浏览器验收、真实 MariaDB/Redis Migration 演练、备份恢复、真实 Provider 请求与数据质量证据尚未提供。详细清单见 Data 迁移计划与 Server 卡点文档。


### RA009 独立遗留边界：旧 Analysis Task/Run 浏览器认证

- Root Data 入口统一不等于旧 Analysis Task/Run 已完成 Owner Session 接入。
- 旧 Analysis 路由继续 Owner Bearer-only，WebUI 当前使用同源 Session Cookie，不会自动具备该权限；这是既有 SRV-CTR-001 deferred 边界，不是 Data 路由回归。
- 后续由 Server/Researchflow 按新 API 契约完成 Application、HTTP/OpenAPI、Root DI、Owner Session/资源授权与端到端联调；不得通过身份映射或静态 Bearer 绕过权限边界。


### RA009 代码 CI 通过后的路由补充（2026-11-19）

- 在统一入口复核中发现 Data Source 列表的精确路径 `GET /api/v1/data-sources` 未被原来的尾斜杠子树模式覆盖；这会使该路由落入 Root 的其他路由分发，而不是 Data Owner Session/Bearer 分发器。
- 已在 Server 统一注册精确路径与子树路径，并新增未认证拒绝、Owner Bearer 通过测试；Owner Session 与 Bearer 共用同一组注册模式。
- 修复代码 SHA `1e916220a8784ae840132ce0cec3b3654c6b9655` 的完整 Integration CI [Run 44482650366](https://github.com/Kasign/stock_data_provider/actions/runs/44482650366) 全项 success。
- 该问题属于实际路由覆盖缺陷，已修复；没有新增认证身份或放宽策略。


## 24. RA-CROSS-REVIEW-009 LLM Gateway 生产调用链审查（2026-11-19）

- 前置：RA-CROSS-REVIEW-003～008 已完成；RA009 Data 单入口迁移代码与 CI 验收完成，M6 生产证据继续独立阻塞。
- 当前状态：REVIEW_DONE_WITH_BLOCKERS。
- 审查目标：确认 LLM 模块是否具备真实可调用的 Application Port、Provider/Gateway Adapter、服务端 Secret 装配、受控 Platform Egress、调用超时/取消/错误归一化、能力与证据记录，以及 Researchflow/Server 的实际生产消费者。
- 审查范围：`internal/llm/**`、`internal/platform/**` 中已存在的相关公共能力、`internal/researchflow/**` 的 LLM 调用方、`internal/server/**` 的组合根与 Worker 生命周期，以及对应 `doc/llm/**`、`doc/researchflow/**`、`doc/server/**`。
- 执行原则：先沿真实调用链定位缺口；不复制 Gateway、不把用户本地 LLM 环境或独立 `Kasign/llm-gateway` 假定为已装配依赖；不新增没有实际消费者的接口/Worker；缺少外部 Secret 或 Endpoint 时判为 SKIPPED / NOT_COMPARABLE，不伪造真实调用成功。
- 验收：区分代码机制 PASS、生产装配 PASS、真实模型调用 PASS、NOT_COMPARABLE 和 DEFERRED；任何修复均同步调用方、实现方、契约、测试和模块卡点，并执行完整 Integration CI。


### RA-CROSS-REVIEW-009 审查结论（2026-11-19）

- 审查基线：repair_all 文档/代码树 `0b21a3e61172ad751bc17bf17ed0c271ae25b5ec`；生产代码自上一个已验收代码 SHA `1e916220a8784ae840132ce0cec3b3654c6b9655` 后未变化。该代码 SHA 的完整 Integration CI [Run 44482650366](https://github.com/Kasign/stock_data_provider/actions/runs/44482650366) 全项 success，包含根模块、go-core、News/Research/Researchflow、MariaDB/Redis Worker 集成、WebUI 测试和镜像构建。
- LLM 模块：`llm.Provider`、Responses Generate/Stream Adapter、JSON/SSE Parser、Schema 校验、Usage/错误归一化、CapabilityState 有效期与版本门禁均已实现。Adapter 使用显式 `config.Secret`、deadline、请求/响应字节限制、无透明重试；Stream Body 关闭和 callback 背压有本地测试。
- Platform：已有 Secret、BoundDoer/OpenStream、Fault、Clock；LLM 没有自建 HTTP Client/SSRF/DestinationPolicy。当前没有发现应在 LLM/Platform 内立即修复的真实代码缺陷。
- Server：`internal/server/llmtransport/transport.go` 提供固定 HTTPS host/port、禁止 HTTP/redirect/transport retry 的 BoundDoer 薄适配，并有 `NewResponsesAdapter` 辅助构造；但 Server Bootstrap/配置中没有调用它，没有 LLM Provider Config、SecretRef 解析、CapabilityState 生产注入或消费者关闭顺序接线。SRV-010 仍 BLOCKED。
- Researchflow：`toolpolicy/llm_adapter.go` 仅复用 LLM Schema Validator、把 ModelToolCall 转成待授权 Intent；`contextassembly/model_input.go` 仅序列化授权上下文并统计 Token，不调用模型。通用 StageExecutor/Output Registry 当前只在 Server 装配 News Search Stage，没有 LLM Stage/Provider 消费者。
- 判定：LLM 协议适配与本地安全门禁 PASS（代码机制）；Server transport wrapper PASS（代码机制）；Server 生产 Provider 装配 NOT_COMPARABLE；Researchflow 实际消费者 NOT_COMPARABLE；Gateway/Model 真实调用 NOT_RUN；Capability Evidence NOT_COMPARABLE。没有生产消费者与真实业务场景前，不新增占位 Stage/Worker/HTTP API。
- 后续解除条件：Server 先完成 SRV-010 生产配置/SecretRef/BoundDoer/Capability Evidence/生命周期接线；Researchflow 明确第一个真实模型调用 Stage 的输入、输出、预算、重试和工具授权；再执行 Generate/Structured Output/SSE/Usage/取消/超时/超限/429/5xx 的真实上游验收。
- 本轮没有生产代码修改；LLM 卡点已同步至 `doc/整体任务规划/LLM模块卡点.md`、Server 卡点和 LLM 当前任务记录。


## 25. RA-CROSS-REVIEW-010 跨模块 Diagnostics 结果契约一致性审查（2026-11-19）

- 前置：RA-CROSS-REVIEW-009 已完成；LLM 生产调用链因 Server/Researchflow/Gateway 外部前置继续 BLOCKED。
- 当前状态：PLANNED。
- 目标：核对 Platform Diagnostics 公共模型、Server 聚合/Readiness、Data/News/Research/Researchflow/Quant/Notification 的 Check 结果是否遵守统一状态语义，重点检查 reason_code、evidence_ref、dependency、retryable、required/optional、时间戳和 skipped/not_comparable 边界。
- 范围：现有 Diagnostics 生产实现、Schema/Contract/Golden 测试、Root Runner 聚合和模块诊断文档；不新增平行诊断系统，不为无消费者模块创建空 Check。
- 约束：先以 `doc/字段校验与诊断体系实施方案.md` 和 Platform 冻结契约为准；仅修复可复现的代码/契约不一致，不通过降低状态门槛或补造 evidence_ref 使测试通过。
- 验收：逐模块标记 PASS/DEGRADED/FAIL/SKIPPED/NOT_COMPARABLE，记录具体 Check、状态字段、证据来源、聚合行为和精确 SHA；真实数据/Provider 缺失时保持 NOT_COMPARABLE。


## 26. RA-CROSS-REVIEW-010 Diagnostics 结果契约一致性审查（2026-11-19）

- 审查基线：`repair_all` HEAD `4a31461b625369a22e1d7a263f5bbdb9f1c0f025`；对比上一生产代码验收 SHA `1e916220a8784ae840132ce0cec3b3654c6b9655`，其后 5 个提交均为文档变更，未发现生产代码差异。
- 验收证据：生产代码精确 SHA `1e916220a8784ae840132ce0cec3b3654c6b9655` 的 Integration CI Run [44482650366](https://github.com/Kasign/stock_data_provider/actions/runs/44482650366) 全项 success。本次为文档审查/状态校准，不将该 CI 冒充为本次文档 HEAD 的新 CI。
- Platform：Observation 五态、reason_code 格式、UTC/纳秒、属性长度、Runner 并发/超时、Required/Optional 聚合和 Fault 脱敏实现与 PLAT-CTR-004 当前机器契约一致；Runner 明确要求 Check 遵守 Context 取消，不具备强杀忽略 Context 的 Go 函数能力，这是契约边界而非缺陷。
- Server：Root 只聚合 Shared MariaDB/Redis、Schema、Data 模块 Ready 和已接入的 News 检查；Readiness 仅在所有 Required Check 为 healthy 时返回 ready。Optional Provider 未配置为 skipped，不被误报为失败；Required skipped 会被 Runner 转为 failed。
- Data：Root 通过 `marketDataReadinessChecks` 调用 Data 自有 `Diagnostics().Ready(ctx)`，没有复制 Provider 诊断规则。该检查只证明模块依赖 Ready，不等于所有 Provider 的实时质量/覆盖率验收。
- News：Parser、ProviderConfig、Storage、Observation Retention 检查工厂存在；Root 已注册 Parser、ProviderConfig、Storage、Retention（Retention 明确为 Optional）。Source、BusinessQuality、Provider Output Policy 等检查工厂虽存在，但未全部进入当前 Root 检查集；需在真实依赖/授权 Resolver 装配完成后逐项注册，不能用占位 Resolver 或伪造来源证据。
- Research：Context/Event/Claim/Evidence 检查按具体版本引用和 Run 截止时间构造，Evidence 读取经过当前授权复核；它们不是可无条件加入 Root Readiness 的全局检查。当前无适合的全局资源引用消费者，判为 NOT_COMPARABLE。
- Researchflow：StageAssessment 是具体 Stage 执行结果的诊断投影，不是进程级依赖 Check；not_comparable 保留 assessment_status 并映射 degraded，避免伪装 healthy。当前没有 Quant/LLM 等生产 Stage 消费者，不创建占位检查。
- Notification：Lease Recovery 诊断映射校验计数守恒、未知投递 ID 唯一性和时间；它依赖实际 Recovery Worker 批次报告。当前 Server Delivery/Fanout Worker 尚未装配，故生产注册 NOT_COMPARABLE，不得生成空批次冒充健康。
- Quant：当前没有可接入 Root 的生产计算/结果服务链路；无消费者诊断检查不应提前注册，按 RA-CROSS-REVIEW-004 卡点继续阻塞。
- reason_code/evidence_ref/dependency/retryable：Observation 对非 healthy 强制 reason_code；EvidenceRef 是领域授权引用，不允许 Platform 推断或合成。Fault 的 Dependency 为可选逻辑标识，Retryable 只允许用于 unavailable/timeout 且由具体错误可重试性决定；不应从 Check ID 或 Status 自动推断。当前审查样本未发现可复现的契约违反。
- 文档发现并修正：PLAT-002 登记簿将 PLAT-CTR-004 标为 `frozen`，但契约正文仍写 `freeze-ready（待项目负责人确认冻结）`。这是权威状态冲突，已将契约正文状态对齐为 `frozen`；不改变契约字段、版本或代码。
- 结论：REVIEW_DONE_WITH_INTEGRATION_BLOCKERS。当前可执行的 Platform/Server/Data/News Diagnostics 机制未发现需要立即改动的生产代码缺陷；其余模块检查必须等待真实生产消费者/Worker/授权证据，不能为追求“全模块都有 Check”而新增空实现。
- 后续：转入 RA-CROSS-REVIEW-011，审查完整启动/迁移/Worker 生命周期与跨模块端到端故障恢复；并持续保留 M6 生产环境证据门禁。若 News Source/Policy 检查的真实 Resolver 已完成装配，再单独补齐 Root 注册与针对性验收。


## 27. RA-CROSS-REVIEW-011：Server Lifecycle Shutdown 预算缺陷修复（2026-11-19）

- 基线：`repair_all` HEAD `d58cb66f06bd348d408cb76f96e7a48e2210a2b5`。
- 缺陷确认：`internal/server/runtime.Lifecycle.Stop(ctx)` 在并发启动期间使用无条件等待 `<-startDone`，没有监听 Shutdown Context。若组件 Start 忽略取消或无法及时退出，Root Bootstrap 的 shutdownTimeout 会被突破，`stopConsumersThenCloseResources` 无法及时返回。
- 影响边界：Root 已有保护逻辑，消费者停止失败/超时会保留 MariaDB/Redis/HTTP 共享资源，不会冒险关闭仍被在途组件使用的连接池；但无条件等待导致该保护逻辑无法按时触发，进程退出也被拖延。
- 修复：Lifecycle 等待启动阶段时同时监听 `startDone` 与 `ctx.Done()`。Shutdown 超时则记录并返回 Context 错误、完成本次 Stop 等待结果；不调用仍在 Start 中的组件 Stop。Root 收到错误后按既有策略保留共享资源并退出进程。
- 回归测试：新增忽略 Start Context 的阻塞组件，验证 Stop 在预算内返回 DeadlineExceeded、不会与 Start 并发调用 Stop、后续 Start 退出后仍保留原始关闭超时结果。
- 架构边界：不强杀 Go goroutine、不提前关闭共享资源、不新增 Lifecycle 平行实现、不改变正常逆序停止和启动失败回滚行为。
- 验收状态：代码已修改，等待精确 SHA Integration CI；当前不可将测试视为已执行。


### RA-CROSS-REVIEW-011 CI 首轮结果与修正（2026-11-19）

- 首轮 Integration CI Run [44488570373](https://github.com/Kasign/stock_data_provider/actions/runs/44488570373) 对提交 `6a6fcbec7f88fb9a4832eeb10c7df3118c5044c4`：根模块测试、News/Research/Researchflow 跨模块逐用例门禁、Worker Bootstrap MariaDB/Redis 集成、go-core Vet/测试、Compose、WebUI 语法/单测/镜像构建均通过；最终汇总因新增 Go 测试文件多一个空行导致 gofmt 检查失败。
- 未修改测试标准。已按 gofmt 修正空行，并清理 Worker/API Bootstrap 集成测试中已废弃且不再被 Data 配置读取的 `MARKET_DATA_BASE_URL`、`MARKET_DATA_API_KEY` 测试环境变量；这些是测试夹具残留，不是运行时配置或生产凭据问题。
- 修正后提交需重新执行完整 Integration CI；在新 Run 结束前仍不标记本轮验收通过。


## 28. 阶段二：跨模块卡点解阻总表（2026-11-19）

### 28.1 当前基线与已关闭事项

- 当前基线：`repair_all` HEAD `9a0680463f1ddea8f137e9009a6306f26e331ffd`。
- RA-CROSS-REVIEW-011 Lifecycle Shutdown 预算缺陷已修复；完整 Integration CI [Run 44489065474](https://github.com/Kasign/stock_data_provider/actions/runs/44489065474) 对精确 SHA `9a0680463f1ddea8f137e9009a6306f26e331ffd` 全项 success。该项关闭，不再列为待办。
- RA009 Data Root 单入口迁移代码验收已于 SHA `1e916220a8784ae840132ce0cec3b3654c6b9655` / Run 44482650366 通过。M6 真实部署验收仍单独开放。
- RA-CROSS-REVIEW-010 Diagnostics 审查已完成；无真实消费者的 Check/Worker 不作为待开发事项，不重复审查。
- 下表只列当前仍开放的跨模块解阻项。模块卡点文档保留具体证据、字段和恢复条件；本表负责唯一责任归属和推进顺序。

### 28.2 统一卡点登记

| 顺序 | 卡点/工作包 | 主责 Owner | 协作方 | 当前真实阻塞 | 下一项可执行交付 | 关闭条件 |
|---|---|---|---|---|---|---|
| 0 | 卡点治理与状态同步 | RepairAll | 全模块 | 历史文档存在过期状态，卡点分散 | 统一登记编号、主责、依赖、交付和验收；同步模块文档 | 无过期开放项、无重复责任、每项有明确解除条件 |
| 1 | DATA-CTR-007 Snapshot/Evidence | Data | Research、Quant、Server | 当前 DailyBarsSnapshot 不具备内容寻址 SnapshotRef、Manifest/Payload 绑定和 AuthorizationRef | Data 提交 Snapshot 契约方案及兼容/迁移边界；同步 Research Evidence 映射 | 冻结契约、实现、Golden/Contract Test 和可验证样本齐备 |
| 1 | DATA-003～DATA-007 外部数据证据 | Data | Provider/来源方、Server/运维 | PIT、估值公式/源端时间、Freshness、Go/Python 配对、Provider 请求证据不齐 | 逐字段列出已知/未知证据；先完成公开可查资料和现有样本核验，再登记真正外部缺口 | 每个子项具备指定证券/日期/Provider 的原始证据和独立复算/对账 |
| 1 | NEWS-BLK-01/03/05 | News（配置契约）/Server（装配） | Researchflow、Platform | Source Catalog 权威来源、Feed 生产组合和真实 Diagnostics 注册未闭环 | 先确认现有配置/Resolver/Root 装配是否可复用；输出唯一配置 Owner 和接线清单 | 权威 Source 配置、真实 Resolver/HTTP 出口、Root 装配和集成测试通过 |
| 1 | NEWS-BLK-02/04/07 | 实际业务来源 Owner | News、Researchflow、Server | Feed 无真实调用方；来源许可/网络证据和合格 available_at 来源缺失 | 从现有业务流程确认是否需要 Feed；公开核验来源时间语义与许可；无真实场景的项保持 deferred | 真实调用方、授权来源、available_at 原始证据和持久化版本链通过验收 |
| 1 | NEWS-BLK-06 Cleaner 周期触发 | Server/运维 | News、业务 Owner | 周期触发 Owner、频率和运行策略未定 | 明确运行责任、周期、并发、超时、重叠和停机策略 | 真实周期运行记录、失败重试和优雅停止证据 |
| 2 | Research Context 授权与 Run PIT | Research（授权策略） | Server、Researchflow | ContextAuthorizer 未有真实生产策略注入；没有真实 Context Stage 消费者 | Research 定义授权主体/资源策略；Researchflow 明确首个 Context Stage 输入输出；Server 实现真实注入 | 可信 CallerRef、授权撤销 fail-closed、创建前/读取后 SystemAsOf 一致性测试通过 |
| 2 | Evidence PIT Mode | Researchflow（模板/结果契约） | Research、项目契约 Owner | Template/ResultComposition 缺 PIT Mode；不能猜默认严格/非严格语义 | 提交字段、默认值、风险声明、Fingerprint 绑定的契约决议草案 | 契约冻结、Validator 传播、严格/非严格 Golden 和回归测试通过 |
| 2 | RSC-004 Data 主数据关联 | Data | Research | SecurityRef/IndustryRef 历史版本与知识时间来源未冻结，DATA-CTR-007 未解锁 | Data 提供正式主数据引用/版本契约；Research 不复制模型 | 权威引用契约和历史时点样本可验证 |
| 3 | QNT-013 生产结果存储/授权/审计 | Server（生产 Adapter） | Quant、Platform | 当前只有 Save Port/MemoryRepository，生产 SQL、授权 Reader、审计 Adapter 未闭环 | Quant 固定公开 Port；Server 设计并实现 Repository、Principal/Authorization、Audit 生产注入 | Root Migration、事务/幂等、授权撤销、审计失败语义集成通过 |
| 3 | QNT-014 计算 Facade | Quant | Data、Researchflow | 尚无正式计算 Application Facade，且输入精度/单位依赖 DATA-CTR-005/007 | 先完成不依赖未知输入的正式 Registry/DTO/Golden；输入依赖保持显式阻塞 | 正式计算 Facade、误差预算、Golden 和 Application 测试齐备 |
| 3 | QNT-015 Snapshot Adapter | Quant | Data、Research | DATA-CTR-007 未解锁 | 等 Data Snapshot 契约冻结后实现消费 Adapter | Snapshot 授权、内容完整性和 PIT 质量门禁通过 |
| 3 | FLOW-005 Context Assembly | Researchflow | Research、Server、LLM | 缺真实 Context Stage、Owner Authorizer、Run PIT 绑定和生产 Tokenizer 证据 | 明确首个真实 Stage；按已冻结接口接入，不创建空 Stage | 同一 Run 的 Context 创建/读取/ModelRequest 时间一致且授权通过 |
| 3 | FLOW-006 Result Composer / RF-BLOCK-007/008 | Researchflow | Quant、Research、Server | 无生产 Composer Facade/唯一结果持久化 Owner；RiskDecision 契约未统一 | 确认结果资源唯一 Owner，冻结 Quant→Composer 字段映射与状态/Reason 规则 | 真实 Quant/Research 输入、风险 fail-closed、持久化及 ResultRef 授权读取通过 |
| 3 | Researchflow Outbox 永久错误策略 | Researchflow | Notification、契约 Owner | 永久 Envelope 错误目前统一 Retry，Reject/DeadLetter 策略未决 | 形成保留现有状态模型前提下的告警/人工处置决策；若需改状态先走契约变更 | 永久/临时错误分类和可恢复处置策略经契约测试验证 |
| 4 | SRV-010 / LLM-012 生产调用 | Server（装配） | LLM、Researchflow、部署 | 无 SecretRef、ProviderConfig、CapabilityState 生产装配及真实消费者/Gateway 证据 | Server 确认受控 Secret/目标策略；Researchflow 提供首个真实场景；LLM 提供能力验收矩阵 | 真实 Gateway/Model 版本绑定，调用/取消/超时/错误场景留存证据 |
| 4 | NTF-BLOCK-01/02/03 事件/Filter/模板契约 | 各业务事件生产方 | Notification | 缺真实事件 Schema、Filter 语义和模板变量映射 | 先选定首个真实业务事件并由生产方提供 Schema/Golden/模板 | 正式契约冻结且 Notification Contract Test 通过 |
| 4 | NTF-BLOCK-04/05/06/07 生产投递装配 | Server | Notification、Platform | RecipientDirectory、Fanout/Delivery Worker、依赖组合和根 Migration 未形成生产闭环 | 以首个真实事件和接收人完成最小完整组合，不先造通用空壳 | Root 装配、Worker 生命周期、真实接收人、Migration 和投递集成通过 |
| 4 | NTF-BLOCK-08/09 恢复与 Secret 运维 | Server/部署运维 | Notification | 多实例故障恢复及生产 Webhook Secret 生命周期未验收 | 制定故障注入矩阵和 Secret 注入/轮换/撤销演练 | 租约恢复、Unknown 对账、停止及 Secret 轮换证据通过 |
| 5 | M6 生产部署验收 | Server/部署运维 | Data、Platform、WebUI | 目标 HTTPS/Owner Session、MariaDB/Redis Migration 启停、备份恢复和 Provider 真实数据证据缺失 | 形成部署演练脚本、环境配置清单和证据归档路径 | 目标环境真实运行、备份恢复、浏览器与 Provider 验收完成 |

### 28.3 执行约束与当前批次

1. 当前先执行阶段 0 状态治理，并立即启动阶段 1 的 DATA-CTR-007 契约梳理；不等待所有外部证据齐备才开展可独立完成的工作。
2. DATA-003～DATA-007、NEWS-BLK-04/07、LLM Gateway 和 M6 中真正依赖供应商/目标环境的证据，先完成公开资料、仓库资产和既有记录核查；确认无法从现有来源获得后，才保留为外部交付项。
3. 任何跨模块卡点都必须先明确契约 Owner，再改代码；不得通过默认 Allow、假 Provider、空 Worker、Mock Stage 或虚构来源关闭。
4. 已完成的机制审查不重复执行。新增工作必须产出契约决议、代码差异、可复现测试或外部证据之一。
5. 每个功能批次按完整上下游提交；所有代码和文档只在 `repair_all` 修改，记录精确 SHA 与完整 CI 结果。
6. 卡点状态采用 OPEN / IN_PROGRESS / BLOCKED_EXTERNAL / DEFERRED / CLOSED；未执行的真实验收不得标 PASS。

### 28.4 阶段 0 状态校准

- RA011 的 Lifecycle 修复和完整 CI 已通过；关闭 RA011 的 Lifecycle Shutdown 预算缺陷，不再列为未验收代码任务。
- RA009 的代码与 CI 已通过；Data/Server 文档中早期“等待最新 CI”的文字属于历史快照，以 RA009 最终验收记录为准。M6 目标环境证据仍开放。
- RA-CROSS-REVIEW-010 已完成，不重复执行 Diagnostics 全面审查；只在真实 Resolver/Worker/消费者接入后补充相应注册和测试。
- Data、News、Research、Researchflow、Quant、LLM、Notification 的业务卡点仍开放，具体以本节统一登记和各模块卡点文档为准。


### 28.5 阶段 1 首个工作包：DATA-CTR-005 / DATA-CTR-007（已启动）

- 已核对项目级冻结登记簿：DATA-CTR-005/007 均为 deferred；DEC-PLAT-002-020 已接受 Data Snapshot、Manifest、交易日历来源/版本和 Research Evidence 映射的责任决议；DEC-PLAT-002-047 已明确 Quant Factor 需要 interval=1d 与不可变交易日历版本。
- 已核对真实代码：Data Model/Application Port 的 DailyBar 使用 float64；Tushare/BaoStock 在 Provider 入口 ParseFloat；Hithink 日线 JSON 解码为 float64；MariaDB 当前 OHLC DECIMAL(20,8)、Amount DECIMAL(24,2)、ChangePercent DECIMAL(16,8)。因此不能从现有 float64 反向恢复精度，也不能把 DB scale 作为源端精度契约。
- 当前 DailyBarsSnapshot 只是查询结果 DTO；未发现 Data-owned 不可变 Payload Repository、SnapshotRef 内容寻址和读取时 AuthorizationRef 复核实现。DATA-CTR-008 Manifest Schema/Golden 只描述元数据，不包含行情 Payload。
- 已新增 Data 解阻方案 `doc/api/依赖/02-DATA-CTR-005与007解阻方案.md`，明确 D1-D7：字段证据盘点、DailyBars v2、交易日历/复权完整性、Snapshot 持久化、授权/Server 装配、Research Evidence 映射、Quant 消费验收。
- 当前只进入 D1 证据盘点；不擅自修改冻结登记簿，不以文档方案代替实现验收。下一步从现有 Provider 代码、历史 Live Probe Artifact 和交易日历 Repository 继续收集证据，再决定可直接实施的代码项。


### 28.6 D1 证据盘点补充

- 已确认 DailyBars 的 SourceVersion（Tushare/BaoStock/Hithink）为适配器/接口版本常量；通用校验只要求非空且非 `unknown`，不能视为上游数据修订版本。
- 初始审计时 TradingCalendar 只有 Provider 查询与 Redis 12 小时缓存；后续 Migration V5 已补齐不可变 CalendarRef Repository、内容寻址和 PIT 读取，见 28.9 与 28.17。SourceVersion 仍是 Provider/Adapter 版本，与 CalendarRef 内容身份分离。
- 后续实现必须分离 adapter_version、normalization_version、provider_data_version 和 calendar content ref；缺少源端版本/可靠日历证据时显式缺失或降级，不伪造版本。


### 28.7 D1 Provider 字段公开证据结果

- Tushare 官方文档已核实 daily 的成交量为手、成交额为千元，日线通常 15:00～16:00 入库且停牌期间不提供；没有字段精度/scale、舍入规则或逐行数据版本。
- BaoStock 官方文档已核实成交量为股、成交额为人民币元及停牌日 OHLC/零成交量额语义；没有字段精度/scale 或逐行数据版本。
- Hithink 尚无可核验官方字段单位/精度文档；现有代码将 turnover 映射 CNY，不能替代供应商证据。
- 公开资料和代码确认 Provider 原始数字 token/字符串可在解析入口保留；不需要、也不允许从 float64 或 DECIMAL 固定 scale 反向推断原始精度。


### 28.8 历史 Provider 精度证据去重

- 继承 BaoStock amount 四位小数精度与 Artifact `13410429933` 的 18 个交易日真实样本离线对账（Data Runtime Acceptance Log 第 28 节）；不重复请求该样本。
- 继承 Hithink backward 真实 Run `43214735212` 小数股数异常证据；继续拒绝非整数 shares，不放宽标准。
- Go/Python 旧源码基线已固定，但双侧同输入原始输出仍缺；DATA-006 保持开放。


### 28.9 Data D2 交易日历不可变内容版本（代码已提交，CI 待验）

- Data Migration V5 新增 `market_data_trading_calendar_snapshots`，Repository 实现 SHA-256 内容寻址、不可覆盖写入和精确读取完整性校验；Data Service 在 Provider 获取与 Redis Cache 命中路径持久化。
- `ValidateCompatibilitySchema` 已检查 V5 物理表、主键和非唯一查询索引，避免仅凭 Migration checksum 判定结构完整。
- 新增单元和 MariaDB 集成测试，覆盖相同内容幂等、内容/来源/质量变化、并发写入、精确读取、篡改拒绝和 Service Cache 命中。
- 最终精确 SHA `4aa468e4de6fc51a16068d43ab2229c5b0181ab9` 对应 Integration CI Run `44493758123` 全项 success。首轮 SHA `dc8079bac92f19c2928219433f29480fb9d7d3ba` 的 Run `44492921617` 因 gofmt/旧 Migration 数量断言失败，已修正并通过最终验收。
- 该子项不替代 DATA-CTR-005/007：DailyBars Decimal v2、Manifest 1.1.0、Payload fingerprint、AuthorizationRef 和 Research/Quant 消费仍是后续工作。


### 28.10 DailyBars Provider D1 全量范围盘点

- 当前默认部署配置启用 Eastmoney、BaoStock、Tencent、Sina、Netease、TickFlow、Pytdx 七个 DailyBars Provider；Hithink、Tushare、Yahoo 为可配置来源。
- 默认七个 Provider 均无 DailyBar.ProviderTime；SourceVersion 是 Adapter/API 版本常量；v1 RawVolume/RawAmount 仍为 float64，但默认七个 Provider 已额外提供 ExactValues sidecar。
- Tencent/Sina 没有已验证 Amount 字段；Netease/Tencent 有单位判断逻辑；Pytdx ExactValues 已通过 TDX protocol Price.Int64() 保留定点值，不能再将其列为精度全丢失。


### 28.11 Pytdx 协议精度路径已核实（2026-11-19）

- `github.com/injoyai/tdx v0.0.86` 的 `protocol.Price` 为 int64 定点值、单位厘；`pytdx.go` 的 v1 float64 兼容字段仍使用 `Float64()`，但 `pytdxDailyBarExactValues` 使用 `Int64()` 生成精确 OHLC/Amount 十进制值，Volume 保留整数词法值。
- `DailyBarExactValues` 的 OHLC 是请求复权后的最终值；RawVolume/RawAmount 是单位归一化前的来源值。QFQ/HFQ 由协议库调整后的 Kline 生成 ExactValues，因此该 sidecar 与返回的复权口径一致。
- Repository sidecar 已持久化、参与 Revision Hash，current/PIT 读回测试已存在。Amount 的 CNY 业务单位不因复用 Price 类型自动成立，仍需独立核验证据。

### 28.12 Data 精确值 Application Port 映射缺口（2026-11-19）

- 已确认默认 7 个 Provider 的 ExactValues 均在 Provider 层生成，Normalize 按值保留，Repository metadata 持久化并可由 current/PIT 读取恢复。
- 原 `app.DailyBar` / `application.mapDailyBar` 未包含 ExactValues，导致精确值跨 Data Application Port 丢失。已补同构 DTO、深拷贝映射和 Reader current/PIT 回归测试；精确 SHA `a2bbfee6fd02f6c54242349419de7273b8fa4a76` 的 Integration CI Run `44501617234` 全项 success。
- 该修复不修改旧 HTTP JSON，不改变现有 float64 字段或模块职责。
- D1 剩余证据：sidecar canonicalization 丢弃 lexical scale；ChangePercent 没有精确值字段；Tushare/Yahoo QFQ/HFQ 未生成精确调整值；Hithink/Pytdx Amount 单位证据不足；ProviderTime/逐行数据版本仍缺。D2 先完成字段级 DTO/Schema/Golden 设计审查，不擅自把 DATA-CTR-005 从 deferred 改为 frozen。


### 28.13 D1 精确值边界与下一阶段决议（2026-11-19）

- 10 个 Provider 的部分或全部请求路径已有 ExactValues sidecar；Tushare/Yahoo QFQ/HFQ 仍缺精确调整结果，不能将其 v1 float64 调整值当作 Decimal v2。
- ExactValues canonicalizer 会移除小数尾随零，当前保留的是精确数值而非原始 token scale；ChangePercent 尚无精确 sidecar 字段。
- 精确值已穿过 Provider → Normalize → Repository revision metadata → current/PIT → Application Port；Application Port 丢失缺陷由 SHA `a2bbfee6fd02f6c54242349419de7273b8fa4a76` / Run `44501617234` 全项 CI 关闭。
- 下一阶段进入 D2 设计审查：逐字段明确 canonical value、source scale evidence、单位、缺失、ChangePercent 来源、复权精确计算边界、interval/calendar ref 和 version 语义；DATA-CTR-005/007 继续 deferred，直到满足项目级重新进入条件。


### 28.14 D2 DailyBars v2 字段级设计审查启动（2026-11-19）

- 已根据 DEC-PLAT-002-020、DEC-PLAT-002-047 和 GOV-CTR-001/002 整理 v2 Snapshot Header / Bar Record 草案；明确 OHLC/Amount/ChangePercent 的 Decimal 语义、原始来源值、precision evidence、interval、复权、CalendarRef、ProviderTime 和版本字段。
- 设计审查识别到真实接线缺口：CalendarRef 已在 Data 内部独立持久化，但 DailyBars 质量评估使用的同一 CalendarRef 尚未随 DailyBars Application Snapshot 返回；不得通过第二次独立日历查询补造同一性。
- 当前 v2 仍有 ChangePercent 精确值、lexical scale evidence、Tushare/Yahoo QFQ/HFQ 精确调整、ProviderTime/逐行数据版本及 Amount 单位证据等缺口。
- 草案不等于冻结；不修改项目级登记簿，DATA-CTR-005/007 与 QNT-CTR-003 继续 deferred。


### 28.16 PIT 日历修复首轮 CI 与格式修正（2026-11-19）

- SHA dc866b3595fb49594446b15a9b8084c754c70a19 的根模块测试、跨模块测试、Worker MariaDB/Redis、go-core、Compose、WebUI 测试和镜像构建全部通过。
- 首轮汇总唯一失败是 trading_calendar_integration_test.go 新增测试前多一个空行导致 gofmt 门禁失败；已修正，不调整门禁或测试标准。
- 新 SHA 完整 Integration CI 待执行。


### 28.17 PIT 交易日历修复完整验收（2026-11-19）

- 精确 SHA 08684e8a9b50641b4d32c38ab6ac9a3c7dd808e0 的 Integration CI Run 44504593484 全项 success。
- gofmt、根模块 vet/test、跨模块测试、Worker MariaDB/Redis、go-core vet/test、Compose、WebUI 语法/单测/镜像构建均通过。
- DailyBarsAsOf 历史日历选择修复通过完整 CI；dc866b 首轮唯一 gofmt 格式问题已修正，没有修改门禁。
- D2 下一步不是直接改旧 v1，而是完成 CalendarRef Snapshot 传播设计、ChangePercent/scale/amount 证据审查，再确定版本化 DTO 实现范围。


### 28.18 PIT 日历 Service 入口集成测试补充（2026-11-19）

- 增加 Service + MariaDB 集成测试，验证 TradingCalendarAsOf 只返回截止时点前已观察的 CalendarRef，不访问实时 Provider。
- 用例覆盖“asOf 可见旧版本 + 数据库存在未来新版本 + 实时 Provider 返回未来版本”的组合，防止未来版本通过 Service 入口泄漏。


### 28.19 PIT 日历集成测试数据隔离（2026-11-19）

- Service as-of 集成测试使用独立历史日期区间，避免与其他测试的相同日历内容/观察时点产生共享内容地址，消除测试顺序耦合。
- 生产实现未变；新 SHA 重新执行完整 CI。


### 28.20 D2 CalendarRef 与 ChangePercent 字段审查补充（2026-11-19）

- 代码复核确认 coverage helper 当前只返回 bool/warnings，实际使用的 CalendarRef 被丢弃；正常读取与 PIT 读取必须分别从同一次 `TradingCalendar` / `TradingCalendarAsOf` 结果中捕获引用，不得二次查询。
- D2 方案明确以内部 `DailyBarCoverageEvidence` 传递 calendar_ref、日历质量、交易日分母、coverage 阈值与 warnings；v1 方法保留兼容，v2 使用独立版本化 Application Snapshot Header。
- ChangePercent 证据已按 Provider 分层：Tushare/BaoStock/Netease 可从原始 token/string 保留来源值；TickFlow 部分记录有来源值、缺失时现有 float64 派生；Pytdx/Yahoo 当前是 float64 派生；其余默认 Provider 未建立可核验来源链。没有冻结 Decimal 派生舍入规则前，derived 值在 v2 必须 null。
- 当前仍开放 lexical scale evidence、Tushare/Yahoo QFQ/HFQ 精确调整、Amount 单位、ProviderTime/provider_data_version 证据；未修改项目契约登记簿，DATA-CTR-005/007 与 QNT-CTR-003 继续 deferred。


### 28.21 Pytdx Amount 单位证据边界补充（2026-11-19）

- 当前仓库使用的 `github.com/injoyai/tdx v0.0.86` protocol 文档将 `Kline.Amount` 定义为 `Price` 类型并注释“成交额”，但未明确该字段业务单位；现有 Adapter 将其按 CNY 传入 ExactValues helper 是实现映射，不是来源证明。
- TDX 官方 `get_market_data` 文档将 Amount 单位标为万元；公式系统 AMOUNT/AMO 指标说明为元。两者是不同接口/语义，不能直接互相覆盖，也不能把较新版本依赖文档倒推到当前 v0.0.86 标准协议。
- v2 的 `amount_cny` 在与当前 protocol Kline.Amount 对应的单位证据明确前保持 null/degraded。该项仍是 D2 的真实证据卡点，不以当前代码常量作为验收依据。
- 证据来源见 `doc/api/依赖/02-DATA-CTR-005与007解阻方案.md` 第 2.4.10 节。


### 28.22 DailyBars v2 CalendarRef 同次评估传播实现验收（2026-11-19）

- Data Service 新增 `DailyBarCoverageEvidence`，将覆盖率计算实际使用的 CalendarRef、Calendar Quality、ExpectedTradingDays、MinimumRequiredBars、CoverageBasis、CoverageSufficient 与 warnings 作为一个结果返回。
- 当前视图由同一次 `TradingCalendar` 返回 CalendarRef；PIT 由同一次 `TradingCalendarAsOf` 返回历史 CalendarRef。缓存、Provider、MariaDB fallback 和 PIT 路径均传递实际使用的引用，禁止二次查询补造。
- v1 `DailyBars` / `DailyBarsAsOf` 签名和旧 Application Snapshot 保持兼容；独立 Application `DailyBarsV2Snapshot` / `DailyBarsV2Reader` 暴露 `interval=1d`、nullable CalendarRef、coverage evidence 和 SystemObservedAtCutoff。
- 无明确 start/end 或不可持久化日历 ref 时，v2 显式降级；v1 质量行为不因新增 ref 约束改变。
- Application current/PIT、无 ref/无界查询测试及 MariaDB 当前/PIT CalendarRef 同源集成测试均通过。
- 精确 SHA `1c23678b5a80c07e5f31b3e5008d1e031cdd8048` 对应 Integration CI Run [44509890582](https://github.com/Kasign/stock_data_provider/actions/runs/44509890582) 全项 success。
- CalendarRef Application 传播项 CLOSED；HTTP v2 Schema/Handler/Golden 和消费者契约测试仍 OPEN。Decimal lexical scale、ChangePercent、Tushare/Yahoo QFQ/HFQ、Amount 单位、ProviderTime/provider_data_version 及 DATA-CTR-007 Manifest/Payload/AuthorizationRef 仍未解除。


### 28.23 DailyBars v2 无界查询门禁与 DTO 阶段边界（2026-11-19）

- Application v2 新增无界查询回归测试：缺少 start/end 时，即使内部 Service 结果带有 CalendarRef，也必须在 v2 Header 清空 ref，标记 `unbounded_range_estimate` 并将 Snapshot/Bars 降级。
- 精确 SHA `6d00369b6b9c66261eb47625e6bec7a2d80217e1` 对应 Integration CI Run [44510702240](https://github.com/Kasign/stock_data_provider/actions/runs/44510702240) 全项 success；gofmt、根模块 vet/test、跨模块测试、Worker MariaDB/Redis、go-core vet/test、Compose、WebUI 语法/单测/镜像构建全部通过。
- 当前 Application `DailyBarsV2Snapshot` 仅关闭 CalendarRef / coverage Header 传播，不代表最终 DailyBarV2 Decimal body 已完成；Bars 仍暂时复用旧 Application DTO 的 float64 字段。
- 后续先完成 Decimal canonical value、原始 token/scale evidence、ChangePercent 来源、Amount 单位、复权精确值和 nullable/version 语义，再建立最终 HTTP v2 Schema/Handler/Golden 和 Research/Quant Consumer Contract Test。DATA-CTR-005/007 与 QNT-CTR-003 继续 deferred。


### 28.24 DailyBars v2 CalendarRef 质量 fail-closed 验收（2026-11-19）

- v2 Reader 在同一次 coverage evidence 表明日历质量不可靠时，即使存在 CalendarRef 也不向 Header 暴露引用；使用 `degraded_exchange_calendar` 并降级 Snapshot/Bars。
- 仅因历史 Provider 缺少 publication timestamp 的 timestamp-only degradation 不视为内容冲突；若日期覆盖和其他质量事实可靠，仍保留实际 CalendarRef。
- Application 测试覆盖 PIT 有效日历 ref 保留、冲突/降级日历 ref 拒绝、无持久化 ref 降级及无界查询降级。
- 精确 SHA `7c5f6e3c5c5b4bb146d46e60d1c723bd8fbce9c2` 对应 Integration CI Run [44511526743](https://github.com/Kasign/stock_data_provider/actions/runs/44511526743) 全项 success。
- 该工作只关闭 CalendarRef / coverage Header 的同一性和质量门禁；最终 Decimal DailyBarV2 Body、HTTP v2 Schema/Handler/Golden、Research/Quant Consumer Contract Test 仍 OPEN。DATA-CTR-005/007、QNT-CTR-003 继续 deferred。


### 28.25 日线精度与 ChangePercent 来源证据实现（2026-11-20）

- DailyBar ExactValues 新增原始 token、小数位、协议固定 scale 等 evidence；canonical Decimal 与 lexical evidence 分开。observed_token 不代表 Provider 合同精度。
- ChangePercent 来源证据接入 Tushare/BaoStock/Netease；派生值不冒充 provider token。Tushare amount 记录 thousand_CNY→CNY 的 1000 因子；Pytdx/Hithink 未核实单位时不生成精确 amount_cny。
- Application Port 深拷贝新增证据字段；Provider/Application 回归测试已覆盖。
- 精确 SHA `baaa2a57051e064f4f91a54851443379636bb685` 的 Integration CI Run [44658725187](https://github.com/Kasign/stock_data_provider/actions/runs/44658725187) 全项 success。
- 最终 DailyBarV2 Body、HTTP v2、Tushare/Yahoo QFQ/HFQ 精确调整、ProviderTime/provider_data_version 和 Research/Quant 消费者契约仍 OPEN；DATA-CTR-005/007、QNT-CTR-003 继续 deferred。


### 28.26 Quant QNT-013 审计回调取消边界修复（2026-11-20）

- 当前 repair_all HEAD：`0a10cb08daa7dcedc6707349ab96d576472bf7a0`。
- 最近提交修复 `GetResult.Execute` 的取消传播缺口：Auditor 回调执行期间若请求 Context 被取消，Application 必须返回取消错误，不得继续释放已构造的结果 Envelope。
- 新增回归测试验证审计事件已记录、请求取消后 Envelope 为空；未改变授权策略、审计事件字段或跨模块端口。
- Integration CI Run [44661192496](https://github.com/Kasign/stock_data_provider/actions/runs/44661192496) 对精确 SHA 全项 success：格式、根模块 vet/test、跨模块测试、Worker MariaDB/Redis、go-core、Compose、WebUI 测试与镜像构建全部通过。
- 结论：Quant Application 内部该取消传播子项 CLOSED；QNT-013 的生产 Storage/事务/Migration、授权 Reader 和审计持久化仍是 Server/Platform 装配卡点。不得将模块内回归通过等同于生产结果链路验收通过。


### 28.27 模块卡点索引完整性修正（2026-11-20）

- 复核发现整体任务规划 README 的模块卡点快照只链接 Research，遗漏 Data、LLM、News、Notification、Platform、Quant、Researchflow 和 Server 的现存卡点文档；阅读顺序编号也存在重复。
- 已补齐九个模块卡点链接，并修正阅读顺序编号；没有新增卡点文档或改变任何模块责任/契约状态。
- 已回读 README 确认链接路径与仓库现存文件名一致。


### 28.28 Quant / Server QNT-013 生产结果链路复核（2026-11-20）

- 复核基线：repair_all HEAD `487edf052b44721d5b8ba3c5429e5d200ec24772`；Quant 取消传播代码基线 `0a10cb08daa7dcedc6707349ab96d576472bf7a0` 的 Integration CI Run [44661192496](https://github.com/Kasign/stock_data_provider/actions/runs/44661192496) 全项 success。
- 检查范围：`internal/quant/application`、`internal/quant/result`、`internal/server/bootstrap`、`internal/server/migration`、`internal/researchflow` Quant Validator、Researchflow Composition/Runtime。
- 事实：Quant 只有 Save Port/MemoryRepository、GetResult 授权读取用例；无生产 SQL Repository、Quant Migration、PrincipalResolver/AuthorizedResultReader/AccessAuditor Adapter。Server Migration Registry 无 Quant namespace，Bootstrap 无 Quant 生产注入；Researchflow Worker 仅注册 News Search Stage。
- 事实：Researchflow Quant Validator 只验证已取得的 Envelope，不承担 Quant 授权读取或计算；不得把 Validator 当 Stage Provider。
- 授权阻断：用途 `quant_analysis`、级别 `full` 已在 Quant Application 固定，但生产策略 Owner、可信 subject_ref 映射、策略撤销线性化边界、审计保留/失败处置无可执行权威依据。禁止默认 Allow、伪造授权快照或把 Owner Session 等同 Quant 资源授权。
- 责任划分：Quant 定义并拥有结果持久化模块接口/Schema 和 Application Port；Server 组合根负责注入、生命周期及 Root Migration Registry 协调；Platform 提供公共 Storage/事务基础原语；授权策略 Owner 与 GOV-CTR-006 语义必须由正式决策明确，不能由 Server 擅自推断。
- 判定：QNT-013 模块内取消传播子项 CLOSED；QNT-013 生产结果链路 NOT_COMPARABLE/OPEN。QNT-014 计算 Facade、Researchflow Quant Stage 继续等待正式 Registry/Result DTO 与明确真实消费场景。
- 本轮未发现可安全直接实现的生产代码缺陷；不创建空 Migration namespace、不把 SQL 表结构塞入 Server、不将 MemoryRepository 用作生产实现。卡点已同步至 Quant 当前任务、Quant 模块卡点和 Server 模块卡点。
- 下一步：先由 Quant/项目契约 Owner 明确策略 Owner、撤销一致性和审计语义；同时可独立审查 Quant 结果持久化 Schema/幂等事务设计与 Platform Storage 接入边界。具备依据后再实现生产 Adapter、Root DI 和 MariaDB 并发/撤销竞态/审计失败集成测试。


### 28.29 QNT-013 结果持久化结构审查（2026-11-20）

- 复核 Quant Manifest/Record/Save Port、Platform Storage/Migration Runner、Server Migration Registry、Research SQL Repository/Migration。
- 确认 Quant Record 的 Manifest/Payload 已有 JCS/SHA-256 内容寻址与写入前验证；生产存储应保留完整 Manifest JSON 与原始 Payload JSON，不重塑业务 Payload。
- 提出最小单表逻辑模型（result_id 主键、manifest_fingerprint、result_fingerprint、manifest_json、payload_json、created_at），明确这是实现前建议而非冻结 Schema；没有真实查询用例不增加二级索引。
- Save 原子语义以数据库唯一键为最终仲裁：同内容幂等成功、不覆盖首次 created_at；异内容返回 ErrResultConflict；禁止 REPLACE、覆盖 UPSERT 和无锁先查后插。
- Quant 拥有 SQL Adapter/Migration，Server 只注册 namespace/Root 注入，Platform 复用现有连接池与 Migration Runner。授权读取、审计持久化独立于 Save Repository，授权一致性和审计失败语义未决前不实现默认 Allow。
- 本轮只完成设计审查和文档同步，没有生产代码变更；QNT-013 生产闭环保持 OPEN，QNT-014/Researchflow Quant Stage 不提前开放。


### 28.30 Quant 复用 Platform GOV-CTR-006 Fingerprint Verifier（2026-11-20）

- 复核发现 `internal/platform/authorization.VerifyFingerprint` 已是 GOV-CTR-006 唯一公共 JCS/SHA-256 指纹校验原语，但 Quant `application/get_result.go` 又实现了一套本地 fingerprint_input 组装和 JCS/SHA-256 计算，形成重复实现。
- Quant 已改为保留领域字段校验（policy_owner、decision、purpose/content、subject/resource、expires_at、PolicySnapshot tuple 顺序等），将序列化后的 AuthorizationRef/PolicySnapshot 交给 Platform `VerifyFingerprint`；不复制或修改 Platform 算法。
- GetResult 测试 Fixture 已改用 Platform `Fingerprint` 生成指纹，冻结 GOV-CTR-006 Quant Golden 测试继续作为跨实现一致性证据。
- 权威契约核对修正此前卡点表述：GOV-CTR-006 已明确 policy_owner=quant、MVP subject_ref=Researchflow service principal、resource_ref=result_id、purpose=quant_analysis、content_level=full。待实现的是 Quant Owner 策略规则/存储/快照签发、当前策略复核与撤销竞态，不是 Owner/Principal 映射尚未定义。
- 精确代码 SHA `b3530b0cce353462c30e5695143cbe0bbf6f7a54` 的 Integration CI Run [44662914467](https://github.com/Kasign/stock_data_provider/actions/runs/44662914467) 已触发，当前排队中；最终以该 SHA 的完整 CI 结果为准。后续文档修订不改变代码验收 SHA。


### 28.31 Quant 结果 MariaDB 持久化首版（2026-11-20）

- 新增 Quant 自有 `internal/quant/storage/repository.go`、`migrations.go`、`repository_integration_test.go`；不复制 Research/News Repository，只复用 Platform Storage/Migration 原语。
- 首版 `quant_results` 表仅保留内容寻址身份、Manifest/Payload 指纹、完整 Manifest JSON、原始 Payload JSON 和首次创建时间，不增加无查询用例支撑的二级索引。
- Save 写入前验证 Record；依赖数据库主键进行原子并发仲裁；重复路径重新验证存储 Manifest/Payload，拒绝未知 Manifest 字段/尾随 JSON；同内容幂等且不覆盖首次 created_at，异指纹返回冲突，存储损坏返回 ErrCorruptRecord。
- Server Migration Registry 注册 Quant namespace；Bootstrap 单测同步更新。
- Integration CI 增加 QUANT_TEST_MYSQL_DSN 和逐项 PASS 门禁，覆盖普通写入/重放/规范等价 JSON、created_at 稳定、16 并发写入、存储损坏拒绝和 Context 取消。
- 结果写入层已实现，但授权读取、Owner PolicySnapshot 签发/当前策略复核/撤销线性化、AccessAuditor 和 Root 依赖注入仍未完成；QNT-013 保持 OPEN。
- 代码验收 SHA：`7f1162ba2b6472c4653b108232862f7e9d5af1cd`；Integration CI #551 [44664065668](https://github.com/Kasign/stock_data_provider/actions/runs/44664065668) 对该精确 SHA 全项 success。Quant MariaDB 专项四项顶层用例逐项 PASS，根模块测试、vet、跨模块测试、Worker DB/Redis、go-core、Compose 与 WebUI 检查均通过。QNT-013 仍因生产授权读取/审计 Adapter 和 Root 注入未完成而保持 OPEN。


## 29. 模块卡点拆分与独立分支修复（2026-11-20）

- 本轮以 repair_all 基线 `fb0266bbb34fd5484ddbc4c3228383f04ee00cc9` 复核九份模块卡点、模块文档目录和远端分支；所有模块分支均已存在，不需要新建。
- 新增统一执行计划：`doc/整体任务规划/05-治理与验收/模块卡点拆分与分支修复执行计划.md`。其中记录各模块观察 HEAD、任务包编号、主责/协作边界、具体交付、依赖顺序和合并门禁。
- 任务归属：Data 负责 DailyBars v2 与 DATA-CTR-007 证据/契约；Quant 负责 QNT-013 Policy Owner/授权读取/审计；Researchflow 负责 FLOW-005/006、RiskDecision 与 Outbox 决策；Server 负责 Root 装配和生命周期；News、Research、LLM、Notification、Platform、WebUI 仅在真实输入或可复现缺陷出现时推进，不制造空壳任务。
- Platform 当前没有确认的本地 P0/P1/P2 代码任务；Research、LLM、Notification、WebUI 当前也没有可以无前置条件独立闭环的新业务代码任务。保持 NO_LOCAL_TASK / WAIT_DEPENDENCY 不等于模块完成生产验收。
- 执行顺序：先模块分支基线差异复核，再生产者接口/实现，后消费者适配，再 Server Root 装配，最后 repair_all 跨模块集成。共享 Bootstrap、Migration Registry、go.mod/go.sum 和项目契约登记簿必须有唯一修改 Owner。
- 本轮只修改治理文档，未修改生产代码、未切换分支、未对其他模块分支写入；模块分支修复从后续任务批次开始，不能把本次规划提交视为模块分支已同步。


## 30. 模块分支独立修复启动与首批基线同步（2026-11-20）

### 已完成的分支工作

- Platform：将 repair_all 已有的 Storage MigrationOperation、ValidateAppliedMigrations 公共能力同步到 feat/platform。精确代码 SHA b7c8bc6cb014cc7fba0393195dbb0a79eb9677a2，Platform Go checks Run 44667242918 的 Platform packages/Storage integration 均 success。随后更新 Platform 文档。
- Platform → Data 依赖同步：PR #91 已合并，合并提交 753d6af6a1a3aa2bd7cef2addc672e0510c73ee2；Data 分支获得 Platform 公共 Migration 能力，不复制第二套实现。
- Data：将 repair_all 中 Data 自有目录 internal/marketdata/**、doc/api/** 选择性同步到 feat/data，共 65 个文件变更；两分支这些路径的 Blob SHA 已逐项核对一致。同步提交 5b98ae1abbf0e04fa4c275b15f78087b0481c0f1；删除已迁入根 Go Module 的 cmd/market-data 和 services/market-data 旧独立服务文件。文档随后更新到 feat/data。
- Data 精确 SHA 5b98ae1abbf0e04fa4c275b15f78087b0481c0f1：根模块 Test . success，Market-data MariaDB migration/PIT persistence success；Data quality contract regression Run 44667706278 success。Live Provider Probe Run 44667706203 虽 workflow success，但 11 个真实 Provider 专项全部 skipped，不能解除 DATA-003～DATA-007 外部证据阻塞。
- Data Go Test Suite Run 44667706336 最终 failure 的两个 Job 均为过时共享 Workflow：一次性迁移 Job 仍引用已删除 cmd/market-data；Market Data 模块测试的 gofmt/test 路径也仍包含该目录。根模块测试本身通过。该问题登记为 CI-01，由 repair_all 集成/CI 治理负责，不归 Data 模块自行修改共享 Workflow。
- Quant：将 Quant 自有 MySQLRepository、quant namespace Migration v1、MySQLAccessAuditor、quant_access_audit Migration v2 和 MariaDB 集成测试同步到 feat/quant；Quant CI 增加 MariaDB 服务和显式未跳过测试步骤。精确代码 SHA 160541a316e3c9c621451d0c82b6cd5dfbd1327e 的 Quant module validation Run 44666849911 全项 success；Go Test Suite Run 44666849903 同 SHA success。

### 当前模块状态

- Platform：公共 Storage 能力已回到模块分支，当前没有新的本地 P0/P1/P2 功能任务。
- Data：模块代码/文档基线已同步；DATA-003～DATA-007 仍等待真实 Provider、业务时效规则或 Go/Python 双侧样本；DATA-008 继续 deferred。
- Quant：结果不可变写入和 AccessAuditor 首版已在 feat/quant 通过 CI；QNT-013 仍缺 Policy Owner 生产规则/版本签发/撤销一致性、AuthorizedResultReader 和 Server Root 注入。GOV-CTR-006 Golden 不能当生产 Allow 规则；具体缺失输入已写入 Quant 模块依赖文档。
- Researchflow、Server、News、Research、LLM、Notification、WebUI 尚未开始本轮模块代码回迁/修复，按模块任务拆分计划继续。

### 新增跨模块卡点

CI-01：.github/workflows/go-test.yml 保留已过时的 Market Data 一次性迁移 Job 和 cmd/market-data 路径。唯一主责为 repair_all 集成/CI 治理；删除一次性迁移步骤、更新模块测试路径并在精确 SHA 上重跑 Go Test Suite。不得在 Data 分支绕过或局部伪造通过。


## 31. RepairAll 单分支 CI 与文档路径修正（2026-11-20）

- 当前唯一工作分支为 `repair_all`；本节及项目总览中的单分支强制规则优先于第 29、30 节的历史模块分支拆分/同步计划。第 29、30 节仅保留历史审计，不作为后续执行指令；本轮不切换分支、不向其他分支写入。
- 复核 `.github/workflows/go-test.yml`：当前工作流已不存在历史 CI-01 所述的 `cmd/market-data` 一次性迁移 Job 和过时测试目录；Market Data 专项测试已指向 `internal/marketdata/**`。历史 CI-01 原描述不再代表当前工作流状态。
- 发现当前工作流 `push.branches` 未包含 `repair_all`，直接提交无法自动触发 Go Test Suite。已在本轮修复，将 `repair_all` 加入触发分支；未修改测试命令、断言或门禁。
- 精确提交 SHA：`f6386875d9f06c280d4b1e2f87d991998a5fc0ae`；Go Test Suite Run [44694709037](https://github.com/Kasign/stock_data_provider/actions/runs/44694709037) 已由该 SHA 的 push 触发，当前记录状态为 `in_progress`，最终结果待完成后回填。
- 同时修正仓库根 README 的 Data OpenAPI 路径：旧路径 `services/market-data/openapi.yaml` 已迁移，当前权威文件为 `internal/marketdata/httpapi/openapi.yaml`。
- 本组仅修改 CI 触发配置和 README/本计划文档；没有修改业务代码、模块边界或 API 契约。


### 31.1 RepairAll 专项数据库 Job 触发条件补齐（2026-11-20）

- 首轮 Go Test Suite Run [44694709037](https://github.com/Kasign/stock_data_provider/actions/runs/44694709037) 使用 SHA `f6386875d9f06c280d4b1e2f87d991998a5fc0ae`，确认根模块、go-core、Research、Researchflow 等 Job 已运行；但 Data 与 News 的 MariaDB Job 在 `repair_all` push 上被条件表达式跳过。
- 该跳过不是测试通过。已将 Market Data 专项测试、Market Data MariaDB 集成、News MariaDB 集成的 push 条件纳入 `repair_all`，保持各原有测试命令和断言不变。
- 修正提交 SHA：`cec698dc14187d2907f826ed114cb04d3ef64f93`；新 Go Test Suite Run [44694882303](https://github.com/Kasign/stock_data_provider/actions/runs/44694882303) 已由精确 SHA push 触发，当前状态 `pending`，等待全部 Job 完成。
- 验收要求：新 Run 中 Market Data module tests、Market Data MariaDB integration、News MariaDB integration 必须实际执行并通过；其余必需 Job 通过；任何 skipped 项按真实条件解释，不记为 PASS。
### 31.2 真实 Provider Probe 接入 repair_all（2026-11-20）

- 纠正此前“Data 等待真实 Provider 外部证据”的笼统描述：仓库已配置 Tushare、Hithink、TickFlow、MX 等 Secret 引用，真实 Probe 测试也已存在；实际遗漏是 Live Provider Workflow 的 push 分支只有 feat/data，repair_all 的常规 Go Test Suite 不会执行这些外部请求。
- 修改 .github/workflows/live-provider-smoke.yml：增加 repair_all push 触发；当 repair_all 提交包含 [include_public] 时，将 Live Probe 设为严格门禁，不再通过 job-level continue-on-error 隐藏失败；其他既有分支/PR 行为保持不变。
- 触发提交 SHA：402a7b106c936d26b046136bb6339c000104f6a0。提交信息包含 public、MX、Hithink、Tushare、TickFlow、历史日线、复权、停牌等专项 marker。
- Live Provider Run [44697044524](https://github.com/Kasign/stock_data_provider/actions/runs/44697044524)，Job [136156175610](https://github.com/Kasign/stock_data_provider/actions/runs/44697044524/job/136156175610)。截至记录时公共 Provider Probe 仍在执行，不能提前写 PASS；后续专项步骤与脱敏报告待 Run 完成后逐项读取。
- Secret 只由 GitHub Actions 注入进测试进程；日志和报告应继续只记录是否配置、请求元数据、脱敏响应摘要和能力结果，不得输出 Token/API Key。
- 验收时必须区分 passed、permission_required、skipped、failed。Tushare 40203 属接口权限不足，不等同于 Token 无效或网络失败；skipped 不能算通过。仅有真实请求成功也不能自动解除 DATA-CTR-005/007，仍需逐项核验 Decimal 原始精度、Amount/Volume 单位、ProviderTime/version、复权基准和快照内容定位。

### 31.3 Quant 当前 Run 结果访问范围决议（2026-11-20）

- 用户已明确选择：Researchflow 服务主体只能读取当前 Researchflow Run 自己创建或明确关联的 Quant 结果，不允许凭 service principal 全局读取。
- 已将该决议写入 Quant 跨模块依赖文档。关联事实归 Researchflow；优先复用现有不可变 Stage Output Resource 的 run_ref/stage_ref/attempt_ref 与内容，不新增重复关联表。仍需核对当前 Quant Stage Output Schema/Registry 是否已经显式保存目标 Quant result_id。
- Quant Policy Owner 负责最终授权判断；Researchflow 持有 Run/Stage/Attempt 与 Stage Output 关联事实；Server 负责组合根注入只读关联查询端口。Quant 不直接查询 Researchflow 私有 SQL。
- `GetResult.Execute(ctx, runRef, resultID)` 已要求非空 RunRef，并将其传入内部 `ReadRequest.RunRef`；缺失/空白 RunRef 在 Reader 和审计调用前 fail-closed，单元测试已覆盖。可信来源应是 Researchflow Worker 从 WorkerStore 加载并校验的 `StageExecutionRequest.Run.Ref`。但 AuthorizedResultReader 的持久化 Run→QuantResult 关联校验尚未实现，Server 尚无 Quant Stage Provider，QNT-013 仍未完成。
- 文档提交：Quant 依赖决议 32fc6a4d7a88c9f425b269ead6ebcdb36e60b2aa；Data 真实 Probe 门禁记录 24972eab8c46e2512333e91f3588b0325e78a783。均只写入 repair_all。
### 31.4 真实 Provider 证据与 Data/Quant 修复（2026-11-20）

- Live Provider strict Run [44699296891](https://github.com/Kasign/stock_data_provider/actions/runs/44699296891) 已完成，结论 failure；失败是真实外部能力/数据证据：MX API Key 业务码 114；Hithink backward adjusted Volume 原始小数无法安全转成 int64 shares；Hithink 2025 年报披露日原始值 2026-10-31 与官方 2026-04-17 冲突。该失败必须保留，不得改成 skipped/pass。
- Hithink none/forward 日线 23 条真实样本通过结构校验，仍因 Amount 缺失/单位未验证、forward reference date 未验证保持 degraded。Tushare Token 已注入但多个 API 返回 40203 permission_required；TickFlow Key 已注入，quote/index_quotes 通过，Universe 能力 403 permission_required。
- 历史 Provider Probe 对 `600036.SH`、`002982.SZ` 均执行 15 项能力并持久化 MariaDB：4 returned、9 degraded、2 not_supported、0 failed。Eastmoney/BaoStock 未复权跨源字段 45/54 匹配，9 个 Amount 因精度/单位证据不足 not_comparable。BaoStock 官方停牌窗口、Pytdx 停牌日线及公司行动一致性专项复跑通过。
- Data 代码/测试修复：`live_probe_test.go` 正确区分原始成交额保留与 normalized Amount；Hithink 调整方法断言按 Provider 实际方法名验证；TickFlow 使用 Universe ID 聚合，排除并计数不属于当前 A 股全集的陈旧退市成分。TickFlow 线上 Universe 读取仍受账号权限限制，当前仅单测覆盖修复逻辑。
- Quant 代码：`ReadRequest.RunRef` 与 `GetResult.Execute(ctx, runRef, resultID)` 已落地；空白 RunRef fail-closed。Researchflow 当前仅装配 News Search Stage，没有 Quant Stage Provider，也没有已确认的 Quant result_id Stage Output 关联 Schema；下一步先完成 Quant Facade/结果发布和 Researchflow 强类型输出，再实现关联读取适配器与 AuthorizedResultReader。
- 当前代码 SHA `fbfb525ca80092c36874a093b03cf8efd2958cbf`：Go Test Suite Run [44700201934](https://github.com/Kasign/stock_data_provider/actions/runs/44700201934) success；Full Integration Checks Run [44700201950](https://github.com/Kasign/stock_data_provider/actions/runs/44700201950) success。真实 Provider strict Run 失败不影响常规代码 CI 的通过结论，但其未通过的能力仍保持阻塞。

### 31.5 DailyBars V2 日历证据门禁与真实应用层复验（2026-11-20）

- 发现 V2 Reader 对 `CoverageBasis=weekday_estimate` 的处理不够严格：即使没有交易所日历快照，也可能将工作日估算的覆盖结果用于有效日线。现已要求 `exchange_calendar`、非空持久化 CalendarRef、显式日期范围及非降级日历质量同时成立；否则 Snapshot 与 Bar 降级。
- 新增 weekday-estimate 回归测试；真实 BaoStock Application Port 测试同时校验 `data_calendar:sha256:` 持久化引用及 Decimal 证据。
- Live Provider Run [44702141543](https://github.com/Kasign/stock_data_provider/actions/runs/44702141543)，Job `136172823901`，精确测试 SHA `77bff81d21a5e22d29177a36506192e31d77ba20`，结论 success。该轮公共 Provider 与 DailyBars V2 应用层专项通过；Hithink/MX/Tushare/TickFlow 专项未启用，不记为通过。
- 真实样本仍显示 Sina/Tencent 日线 Amount 缺失、Tencent Volume 原始单位为 hands、Eastmoney/BaoStock 前复权参考日期缺证据；均保持 degraded/not_comparable，不通过补值或放宽门槛处理。
- 本节记录后续将以全量 marker 重新执行 MX、Hithink、Tushare、TickFlow 和历史专项，以验证当前代码版本下的实际 Key/权限状态。

### 31.6 全量真实 Provider 复验、Data 证据修正与 Quant Run 审计（2026-11-20）

- SHA `d4806fa18f3e5178507694c02aa708992cc196f9` 的 Go Test Suite 与 Full Integration Checks 均成功；Live Provider Run [44703026046](https://github.com/Kasign/stock_data_provider/actions/runs/44703026046) 严格失败，详细证据已记录在 `doc/api/provider-migration/DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md`。
- 真实 Key 状态已区分：MX Header 已带 Key 但业务码 114 表示 Key 无效；Tushare 多接口 40203 表示接口未授权；TickFlow 行情/指数可用但 Universe 权限不足；Hithink 多能力可用；BaoStock 存在间歇 TCP 超时；Pytdx 历史及停牌样本通过。
- Data V2 同时验证日历引用和 `CalendarQualityStatus`，无引用或日历状态 degraded 均 fail-closed；新增 warning 缺失场景回归测试。
- Hithink 龙虎榜源端 `stock_count` 是唯一证券数，`stock_items` 是事件行，重复证券可能因多个上榜原因重复出现。现按 thscode 去重后再分页，保留原始行数和重复诊断；新增唯一证券数不一致时仍拒绝的回归测试。
- Quant 结果读取继续要求可信 Researchflow RunRef；新增 Quant access audit Migration v3，将 RunRef 持久化到每条读取审计事件。历史审计无法反向推导 RunRef，迁移后旧记录保持 NULL；新审计写入缺失 RunRef 时拒绝。
- Quant 的 AuthorizedResultReader 仍需在未来 Quant Stage Provider 接入时，按 RunRef + ResultID 查询 Researchflow 持久化的明确关联。当前尚无该 Stage Provider/关联资源契约，不得用主体级全局放行替代。
- 新提交 `d1286b1` 触发 Hithink 与 DailyBars V2 定向真实复验；结果待 CI/Live Probe 完成后补记。
