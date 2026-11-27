# Repair All 跨模块修复计划与进度

- 仓库：`Kasign/stock_data_provider`
- 工作分支：`repair_all`
- 当前工作分支 HEAD：以 GitHub `repair_all` 分支最新引用为准（避免在本文件提交后保留自引用 SHA）
- 本轮 RA009 生产代码验收 SHA：`1e916220a8784ae840132ce0cec3b3654c6b9655`；完整 Integration CI [Run 44482650366](https://github.com/Kasign/stock_data_provider/actions/runs/44482650366) 全项 success（历史基线，不代表当前 HEAD）
- 更新日期：2026-11-26
- 章节引用口径：历史段落中的“第 N 节”是旧编号引用，当前状态以本文标题与日期记录为准；跨模块顺序以第 2 节“修复顺序”为准。

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
| P1 | Data、News 到 Research 的数据/时间/Evidence 契约和真实消费边界 | Data / News / Research / Server | DATA-CTR-007 deferred；News 来源策略已按个人研究用途校准，人工导入能力与真实 PIT 证据链继续推进 |
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
- 状态更新（2026-11-26）：本节为 2026-11-19 历史基线（HEAD `b52c8afde`），其中"Envelope 仅注册 trend/risk、Factor Payload 未注册"已不再是当前现状。Indicator/Factor 已先后作为强类型 Payload 注册进 Envelope 白名单；Factor 经 DEC-058 授权后落库并返回 result_id（第⑤组）。当前权威状态以 `旧Analysis退役执行清单.md` 与 PLAT-002 登记簿为准，旧结论仅作历史证据保留。

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
- Evidence PIT（2026-11-19 首轮历史结论，后续已完成修正）：当时 ResultComposition.validateResearchEvidence 尚未拒绝 available_at=null，且对是否引入非严格 PIT Mode 暂缓决策。2026-11-25 已确认当前不存在非严格 PIT 业务场景，因此不增加 PIT Mode；ResultComposition 对所有进入最终研究结果的 Evidence 一律执行 strict PIT，要求 available_at 非空且不晚于 Evidence.system_as_of。实现及回归测试见本文件第 20 节/2026-11-25 记录。
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
| 1 | DATA-CTR-007 Snapshot/Evidence | Data | Research、Quant、Server | 历史卡点：通用 DailyBarsSnapshot 不具备内容寻址 SnapshotRef、Manifest/Payload 绑定和 AuthorizationRef；窄化 Close Series Snapshot 已实现双指纹/内容寻址/授权读取，剩余缺口是通用消费者契约与 Research Evidence 映射（见本文 2026-11-23 Snapshot V6 段落与 PLAT-002 登记簿第 44/78 行） | Data 提交 Snapshot 契约方案及兼容/迁移边界；同步 Research Evidence 映射 | 冻结契约、实现、Golden/Contract Test 和可验证样本齐备 |
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
- 当时 DailyBarsSnapshot 只是查询结果 DTO；未发现 Data-owned 不可变 Payload Repository、SnapshotRef 内容寻址和读取时 AuthorizationRef 复核实现。DATA-CTR-008 Manifest Schema/Golden 只描述元数据，不包含行情 Payload。（注：这是 2026-11-19 的历史审计结论；此后已实现 Close Series Payload/Manifest 双指纹、内容寻址 SnapshotRef、Migration V6 不可变 Repository 与授权读取，见本文"2026-11-23 Snapshot V6 Repository 实施"段落及 PLAT-002 登记簿第 44 行，非当前状态。）
- 已新增 Data 解阻方案 `doc/api/依赖/02-DATA-CTR-005与007解阻方案.md`，明确 D1-D7：字段证据盘点、DailyBars v2、交易日历/复权完整性、Snapshot 持久化、授权/Server 装配、Research Evidence 映射、Quant 消费验收。
- 当前只进入 D1 证据盘点；不擅自修改冻结登记簿，不以文档方案代替实现验收。下一步从现有 Provider 代码、历史 Live Probe Artifact 和交易日历 Repository 继续收集证据，再决定可直接实施的代码项。


### 28.6 D1 证据盘点补充

- 已确认 DailyBars 的 SourceVersion（Tushare/BaoStock/Hithink）为适配器/接口版本常量；通用校验只要求非空且非 `unknown`，不能视为上游数据修订版本。
- 初始审计时 TradingCalendar 只有 Provider 查询与 Redis 12 小时缓存；后续 Migration V5 已补齐不可变 CalendarRef Repository、内容寻址和 PIT 读取，见 28.9 与 28.17。SourceVersion 仍是 Provider/Adapter 版本，与 CalendarRef 内容身份分离。
- 后续实现必须分离 provider_interface_version、normalization_version、provider_data_version 和 calendar content ref；缺少源端版本/可靠日历证据时显式缺失或降级，不伪造版本。


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

### 31.7 DailyBars V2 真实应用层复验与 Live Provider 门禁修正（2026-11-20）

- BaoStock 真实 Application Port 专项在 Live Run [44710110784](https://github.com/Kasign/stock_data_provider/actions/runs/44710110784) PASS：17 根日线，OHLC/成交量/成交额 Decimal Evidence 到达 Application Port。交易日历因非交易日推断保持 degraded，V2 返回无可信 CalendarRef 的 degraded Snapshot/Bar；已修正此前测试错误要求可信日历引用的断言。
- Hithink 真实专项仍 FAIL：2025 年报报告期对应的 Provider `report_date_ms` 返回 2026-10-31，与官方披露日 2026-04-17 不一致；后复权 DailyBars 的 Volume 返回小数，不能安全归一化为整数 shares。none/forward 单窗口数据正常返回 23 行，backward 不生成有效 Bar。以上保留为真实数据/契约问题，不放宽校验、不舍入、不回退混合口径。
- 修正 Live Provider workflow：repair_all 上所有启用的定向探针均为严格门禁（包含此前单独 continue-on-error 的 BaoStock 停牌探针）；Job failure 会使 Live Provider Run failure，避免探针步骤失败但整个 Run 显示 success。
- Go Test Suite [44709526616](https://github.com/Kasign/stock_data_provider/actions/runs/44709526616) 与 Full Integration Checks [44709526576](https://github.com/Kasign/stock_data_provider/actions/runs/44709526576) 在代码 SHA `f61d7d53ec1713d5afe74e69e522d8d3d6484759` 全项 success；后续提交只修改 Live Provider workflow 门禁与文档。
- Quant 读取范围决议保持：`service:researchflow` 只能读取当前 Run 自己创建或明确关联的 Quant 结果。可信 RunRef 已进入 GetResult 内部请求与 Access Audit；但 Run→QuantResult 持久化关联、AuthorizedResultReader 和 Server Quant Stage Provider 仍未完成。没有强类型 Quant Stage 输出 Schema 前，不实现按主体全局放行或猜测 JSON 字段关联。
- 下一步：保留 Hithink 两项 Provider 缺陷为阻塞证据；Data 继续围绕 DailyBars 精度/复权基准/成交额单位与 PIT 日历证据；Quant 仅在 QNT-014 Facade 和 Researchflow 强类型输出关联契约具备后继续授权读取闭环。

### 31.8 Hithink 后复权成交量诊断细化与全量代码验收（2026-11-20）

- Hithink 后复权 Volume 原始返回小数股数，与 Data 当前整数 shares 契约不兼容。新增 `volume_not_integer_shares` QualityReason 与 `non_integer_share_volume` FieldIssue，明确区别于字段缺失；保留原始值证据，不舍入、不拼接 none 口径成交量。单测通过。
- Go Test Suite [44710655574](https://github.com/Kasign/stock_data_provider/actions/runs/44710655574) 对精确 SHA `e48f74d20911adbedb1ffe4bcbb043563df07931` 全部 success，覆盖 Root/Market Data race+vet、各模块测试、MariaDB 集成、Compose 和 Docker builds。
- Live Provider strict Run [44710655555](https://github.com/Kasign/stock_data_provider/actions/runs/44710655555) failure：BaoStock V2 Decimal Application Port PASS；Hithink backward adjustment 仍无法满足整数 shares 契约。此失败作为真实 Provider 阻塞保留。
- 当前 Quant 决议不变：只允许读取当前 Researchflow Run 自己创建或明确关联的 Quant 结果；RunRef 已传入内部 GetResult 请求并持久化到 Access Audit，但没有 Quant Stage Provider/强类型输出关联 Schema，AuthorizedResultReader 尚未完成。不得为关闭卡点而做主体级全局授权。



## 32. RepairAll 两日主线收口计划（2026-11-23）

### 32.1 执行目标与范围

- 唯一工作分支：`repair_all`；计划基线 SHA：`3597a9fead7a5bb7c0bd208209590aa438b2c6b8`。
- 目标窗口：2026-11-23 至 2026-11-24。优先关闭当前主线中可由现有代码、冻结契约和真实测试证据解决的缺陷；外部 Provider 权限、供应商语义、目标生产环境和缺失的项目级契约决策不得伪装成代码完成。
- 不切换分支、不向其他分支写入、不创建新分支、不绕过 integration 合并规则；本阶段只在 repair_all 内按授权修复，若需跨分支集成，必须单独经过 integration。
- 不重复实现已存在的 Data Root Migration/Readiness、Quant Result 不可变写入、Quant Access Audit、Researchflow Stage Output Resource、Platform Fingerprint 等能力。
- Hithink 后复权 Volume/公司行动仿射因子仅作为 Data 数据质量专项证据，不再单独扩展为主线；不舍入小数股数、不放宽 DailyBar shares 契约。

### 32.2 当前精确 SHA 验收基线

在代码 SHA `3597a9fead7a5bb7c0bd208209590aa438b2c6b8` 上复核：
- Integration CI Run `44723943926`：success，Full integration checks 完成。
- Live provider probe Run `44723943852`：success，Probe real public provider responses Job 完成。该 Job 成功只代表本次实际启用的探针通过；没有启用或被跳过的专项仍不得记为真实 Provider PASS。
- Go Test Suite Run `44723943844`：success；根模块 Test、Market Data MariaDB、Research/Researchflow/News/Notification MariaDB、go-core、Compose/Docker 等实际 Job 均完成并成功。
- 上述为当前代码 SHA 的 CI 基线，不代表 Data 全部外部数据证据、Quant 授权生产闭环、Researchflow Quant Stage 或目标生产环境验收完成。

### 32.3 主线任务包与顺序

| 顺序 | 任务包 | 唯一主责 | 本阶段交付 | 关闭条件 |
|---|---|---|---|---|
| P0-1 | Q-01 / QNT-013：Quant Policy Owner 与授权读取 | Quant 领域实现；Server 负责 Root 注入；Researchflow 提供 Run 关联只读事实 | 核对 GOV-CTR-006 和当前 GetResult/SQL Repository/Audit；补齐最小策略规则与不可变版本快照、签发/撤销、当前策略复核、RunRef+ResultID 关联读取和并发一致性测试；明确授权失败/审计失败行为 | 同 Run 明确关联可读；跨 Run、无关联、伪造 RunRef、撤销竞态、Owner 不可用、审计失败均 fail-closed；MariaDB 并发集成通过；Root 注入完成。若缺少已冻结的策略治理授权或真实关联 Port，则只关闭可独立验收子项并明确阻塞 |
| P0-2 | Data D2：DailyBars v2 与 DATA-CTR-005/007 解阻 | Data | 对照当前代码核查 Decimal 原始值/scale 证据、ChangePercent 来源、Amount/Volume 单位、ProviderTime/source data version、Adjustment basis/reference date、CalendarRef、Manifest/Payload/AuthorizationRef；补齐有来源证据的 DTO/Schema/Golden/Validator/HTTP 与 Application Consumer Contract | 每字段有真实来源、单位、时间/版本语义；未知字段保持 null/degraded；Schema/Golden/Validator/实现一致；消费者契约测试通过。项目登记簿的 deferred 状态只能按正式重新进入/冻结流程变更 |
| P0-3 | FLOW-005：Research Context 生产上下文一致性 | Researchflow + Research；Server 装配 | 核对 Context Port、CallerRef 授权、Run.SystemAsOf、Deadline、PIT/available_at 和 Context Snapshot；只修真实调用链上的时间/授权/证据错配 | 同一 Run 的可信 CallerRef、SystemAsOf、Deadline 贯穿 Research 读取和 LLM 输入；来源授权重新校验；无真实消费者时保持 NOT_COMPARABLE，不创建空 Stage |
| P0-4 | FLOW-006：Quant/Research 结果综合与风险语义 | Researchflow + Quant | 明确 Composer 的 RiskDecision 唯一来源；优先复用 Quant 已注册 risk Payload，避免第二份无来源绑定的 RiskDecision；核对 Result 唯一持久化 Owner、PIT 与来源授权传播 | Quant RiskPayload 到 Composer 的精确映射、状态/Reason/时间规则有契约测试；block/unknown/缺失/撤销均 fail-closed；结果资源唯一 Owner 与授权读取明确 |
| P0-5 | Server Root 生产装配 | Server | 仅在提供方公开 Port、Schema、授权和生产依赖满足后接线 Quant/Research/LLM；复核已有 Data Root 单入口、Migration/Readiness，不重复开发 | API/Worker 启动角色、Root DI、Migration、生命周期和真实 MariaDB/Redis 集成通过；没有真实消费者/Secret/Gateway/Recipient 时保持 BLOCKED |
| P1 | M6 目标环境验收 | Server/部署运维 | HTTPS Owner Session 浏览器验收、Migration enabled/disabled 演练、备份恢复、真实 Provider 出站矩阵 | 目标环境原始证据、运行日志和恢复结果归档；无法由 GitHub CI 代替 |

### 32.4 两日执行节奏

**第一天（11-23）：先解除生产结果链路与数据契约阻塞**
1. 对 QNT-013 做端到端代码审计：Policy Owner 是否缺实现、Quant Migration/Repository/Audit 当前版本、Researchflow Stage Output 关联事实、Server Root 注入现状；形成单一缺口清单。
2. 在 GOV-CTR-006 已冻结语义和用户已确认的 Run-scoped 规则范围内实施 Q-01；任何策略发布权限、审计保留或撤销事务边界若缺权威依据，先明确决策点，不写默认 Allow。
3. 对 Data DailyBars v2 做字段级实现审查，核对现有 ExactValues、CalendarRef、Snapshot/Manifest 能力与 DTO/HTTP Schema 的实际差异；只实现有源证据和契约依据的字段，不把 float64 格式化成 Decimal。
4. 当天结束前更新 Quant/Data 当前任务和卡点记录，执行针对性单测、Race、MariaDB 集成及全量 Go Test/Integration CI。

**第二天（11-24）：完成消费者链路、组合装配与收口**
1. 基于第一天确认的正式 Port/Schema，推进 FLOW-005 的 Run 时间与 Research 授权上下文一致性。
2. 推进 FLOW-006 的 Quant RiskDecision 来源统一、ResultRef/持久化 Owner 与 PIT/授权验证；只接入具备正式 Facade 和明确 Stage 场景的能力。
3. 将可生产装配的能力接入 Root Server；补齐 API/Worker 角色、依赖注入、Migration、生命周期和 MariaDB/Redis 组合测试。
4. 复核 Data/Quant/Researchflow/Server 四个模块的任务状态、卡点、文档索引和精确 SHA；执行完整 Go Test Suite、Integration CI、适用的 Live Provider Probe。
5. 输出主线验收表：每项标记 PASS / DEGRADED / FAIL / SKIPPED / NOT_COMPARABLE / BLOCKED_EXTERNAL，并列明精确 SHA、Actions Run、真实输入及未覆盖范围。

### 32.5 严格范围与退出条件

- 本轮不启动 QNT-003 Factor 生产发布、Trend/Market/Industry/Stock/Strategy/Backtest、LLM/Notification 空壳接入；这些任务受 Data 输入、算法决议、真实调用者或外部依赖约束。
- 不把 CI success 等同于 Live Provider 全部通过；不把 Provider workflow 的 skipped 等同于 pass；不把合成 Golden 等同于真实行情验证。
- 不修改项目级冻结契约以绕开实现缺口；需要变更时先记录差异、影响模块和重新进入条件。
- 每个代码逻辑组完成后审查 diff、更新模块当前任务/卡点、运行针对性测试和完整 CI，再按完整逻辑组提交并核对 SHA。
- 两日结束时，所有可在当前约束下安全完成的 P0 代码项应有验收证据；外部条件未满足的项必须有唯一 Owner、可执行解锁条件和明确下一步，不得用“基本完成”掩盖阻塞。


## 33. 2026-11-23 首轮主线代码审计与状态校准

### 33.1 本轮基线与验收状态

- 本轮开始前确认 `repair_all` HEAD：`6139bcbe7b40bc82564783ffb4af41710be32b09`；最新提交为两日收口计划文档提交。计划第 32 节记录的 `3597a9f...` 是计划起点，不是本轮实际 HEAD。
- `integration` HEAD：`5c89614bb87785b61b06d272880b5c0c8a7afe2f`；本轮未修改。
- 精确 SHA `6139bcbe7b40bc82564783ffb4af41710be32b09` 的 Actions workflow runs、check-runs 均为 0；commit status 为 pending 且无 status 项。此前 SHA `3597a9f...` 的 CI 不替代当前 HEAD 验收。
- 本轮完成代码/契约只读审计及对应文档校准，未修改生产代码；因此没有将任何生产代码项标记为本轮 PASS，也没有把文档审计冒充完整 CI。

### 33.2 Q-01 / QNT-013 审计结论

- Quant 结果 MySQLRepository、AccessAuditor、Migration v1-v3 已在 repair_all 代码中存在；Server Root 已注册 Quant Migration namespace。
- Policy Owner 的生产策略存储/签发/撤销、当前策略线性化读取及 AuthorizedResultReader 仍缺失；Server 尚无完整依赖注入。
- Researchflow StageOutputResource 虽有 Run/Stage/Attempt 归属，但没有 Quant Stage Provider 和显式 Quant result_id 输出关联契约。故当前 Run-scoped 授权读取没有可供生产 Reader 验证的关联事实。
- Q-01 维持 BLOCKED。不得默认 Allow、不得由 Quant 查询 Researchflow 私有 SQL、不得凭 Golden 示例或 service principal 身份推断资源关联。具体证据和重新进入条件见 Quant 当前任务/卡点记录。

### 33.3 Data D2 审计结论

- DailyBars v2 目前只完成 Application Header/CalendarRef/coverage evidence；Bars 仍复用 float64 DailyBar DTO，尚无最终 Decimal Body 和 HTTP v2 Schema/Handler/Golden。
- ExactValues 与部分 ChangePercent 来源证据已存在；Tushare/Yahoo 复权精度、部分 Amount 单位、逐条 ProviderTime/provider_data_version 仍缺证据。
- CalendarRef 同次评估和不可变交易日历已完成，不重复开发。DATA-CTR-005/007 保持 deferred，不用 float64 格式化或虚构单位/时间/版本推进。
- 具体字段矩阵和下一步见 Data 当前任务与解阻方案。

### 33.4 下一执行切片

1. **先执行第一批 Q-01 / QNT-013**：审计并实现 Quant Policy Owner 的最小授权规则、不可变策略版本、签发/撤销和当前策略复核；完成 RunRef + result_id 的明确关联读取与 fail-closed 门禁。没有真实 Researchflow 关联事实时，不得以主体级全局放行替代。
2. **第二批执行 Data D2**：按 Provider × none/forward/backward × OHLC/Volume/Amount/ChangePercent 建立字段证据矩阵，区分 source token、protocol scale、unit conversion、adjustment evidence 和 unknown；完成有证据字段的 DTO、Schema、Golden、Validator、HTTP 与 Application Reader。DATA-CTR-005/007 只能按正式重新进入条件变更状态。
3. **第三批执行 FLOW-005 / FLOW-006**：完成 Research Context 的 PIT/身份传播，统一 Quant RiskDecision 来源和结果综合规则；缺少真实消费者或冻结决策时保留阻塞。
4. **第四批执行 Server Root 装配**：只接入已经具备正式 Port、Schema、授权和生产依赖的能力，完成 Root DI、Migration、Worker 生命周期及跨模块 MariaDB 验收。
5. **最后完整验收**：Go Test、Race、Vet、Integration CI 和适用的 Live Provider Probe；每项结果绑定精确 SHA，外部权限、真实披露时点、未冻结决策和目标生产演练不得伪装成代码完成。


## 34. 2026-11-23 D2 Pytdx 复权精度证据补充

本次对 `github.com/injoyai/tdx@v0.0.86` 做了精确版本源码复核：Pytdx QFQ/HFQ 的仿射因子计算使用 float64，输出由 `roundHalfUpYuan` 按分舍入，再写入 Price 的厘定点表示。当前 ExactValues 从调整后的 Price.Int64() 记录 ProtocolScale=3；它是协议存储 scale，不是算法输出精度或原始 Provider token scale。DailyBarV2 必须区分这三者，并记录明确的算法版本/舍入语义。当前未改代码，等待字段契约定义后补针对性测试。


## 35. 2026-11-23 D2 复权锚点语义审查

- 当前服务对 forward 缺失 reference date 会降级；backward 缺失不会由通用门禁降级。Tushare 的 HFQ 累积因子例外有实现和单测依据，但不能据此推断 BaoStock/Hithink/TickFlow 的 backward API 也具有相同锚点语义。
- D2 设计已修正为方法级 reference semantics：日期锚定需明确日期；Provider 累积锚点可空但必须有端点契约/真实样本依据；未知锚点保持 degraded/not_comparable。当前不改 v1 质量门禁、不改变测试标准。
- 下一步核验缺日期的各 Provider 后复权端点；证据不足时保留降级，不补造参考日期。


## 36. 2026-11-23 D2 Provider 锚点证据更新

- Tushare 官方文档确认 QFQ 使用 end_date 最新因子归一化，HFQ 直接使用当日因子；BaoStock 官方文档确认 HFQ 以上市首日为固定锚点、QFQ 因子以最近除权日后交易日为锚点。二者的无显式 HFQ reference_date 有来源依据。
- Hithink/TickFlow/Eastmoney 目前没有足以确认具体端点锚点公式的资料；Hithink 的顶层 timestamp 是数据就绪时间，不得写成每根 Bar 的 ProviderTime。保持 unknown，不猜公式、不新增平行因子实现。
- 下一步集中处理 Hithink/TickFlow/Eastmoney 的真实样本/因子对照；如不能取得可复核证据，则保留 degraded/not_comparable。


## 37. 2026-11-23 按五批主线顺序启动 Q-01

- 执行顺序以第 32.3 节五批任务包为准：第一批 Q-01 / QNT-013；第二批 Data D2；第三批 FLOW-005 / FLOW-006；第四批 Server；最后完整验收。第 33.4 节旧的“Data D2 优先、Q-01 保持 BLOCKED”描述已按此顺序校准，不再作为当前执行入口。
- 本轮开始前 GitHub `repair_all` HEAD：`e8d1c5464bd78dc83d6a9f51df41ea2d86a7fbba`；前一 HEAD `0beb795c55cdd1a4a28f234672223228dca62193` 的后续提交为 `test(data): 修正后复权质量测试断言对象`。本节后续代码和 CI 证据必须使用新的精确 SHA。
- Q-01 现有能力：`GetResult` 已要求可信 Principal、非空 RunRef、result_id，并调用 `AuthorizedResultReader`；AuthorizationRef/PolicySnapshot 结构校验和 Platform GOV-CTR-006 指纹校验已存在；Quant 结果不可变 MySQL Repository、AccessAuditor、Migration v1-v3 已存在。
- Q-01 真实缺口：当前 `internal/quant/**` 没有生产 Policy Owner 规则/策略版本存储与签发/撤销实现，也没有生产 `AuthorizedResultReader`；Server Bootstrap 尚未完整装配 Quant Repository、PrincipalResolver、Reader、Auditor 和可信 UTC Clock。
- Run 范围关联缺口：Researchflow 已持久化不可变 Stage Output 及 Run/Stage/Attempt 归属，但当前没有 Quant Stage Provider，也没有已注册的强类型 Quant result_id 输出契约；因此尚无可验证的 Run→QuantResult 关联事实。关联事实归 Researchflow，Quant 只通过公开只读端口消费，Server 负责组合注入；不得由 Quant 读取 Researchflow 私有 SQL，也不得按 service principal 全局放行。
- 本批实施门禁：先实现可由 Quant 自身职责完成且有冻结契约依据的 Policy Owner/策略版本/撤销一致性；对缺失的 Run 关联生产者契约和发布权限治理，不虚构字段、策略或调用者。能独立验收的子项分别关闭；未具备真实关联事实前，生产读取链路保持 BLOCKED。
- 当前阶段：`IN_PROGRESS`；本次只校准执行入口和记录基线，尚未将 Q-01 任何生产子项标记为 PASS。

## 38. 2026-11-23 Data D2 已完成子项记录（不改变 Q-01 优先级）

- 后复权无 reference date 的质量判断已从“所有 backward 都放行”收紧为方法级证据规则；仅 Tushare/BaoStock 的固定锚点方法放行，其他未知方法降级。实时返回和缓存读取测试均覆盖。
- Go Test Suite 与 Integration CI 在代码 SHA `e8d1c5464bd78dc83d6a9f51df41ea2d86a7fbba` 最终通过；MariaDB job 首次容器初始化失败后单 job rerun 成功。Tagged Live Provider Probe 部分通过，Tushare 权限与 Eastmoney 网络仍阻塞。
- Hithink live raw probe 证明当前样本可用公司行为仿射公式重建 OHLC，但 Provider adjusted volume 为小数，旧 DailyBar.Volume int64 导致 v1 Normalizer 丢弃。v2 建议 volume_shares 继续表示实际未复权整数成交量，复权只改 OHLC。
- 当前 Decimal 重建仍被 AdjustmentFactor/CorporateAction float64 源字段和 float64 仿射算法阻塞；Hithink 公司行为接口尚未接入生产 Provider，adjustment reference anchor 也需通过不同 end date 对照确认。
- 以上仅记录已完成的 Data 子项；执行顺序仍以第 37 节为准，下一主线先完成 Q-01，不在本轮继续扩展 Data Provider。


## 39. 2026-11-23 QNT-013 授权证明与审计证据子项验收

- 本节 Quant 代码 SHA：`d72ac3e933f57f86147fad0b5f05c2d9fbd5999a`；当前 repair_all HEAD 以 GitHub 分支引用为准。
- GetResult 新增领域策略身份校验：PolicySnapshot 必须包含 `quant_result_access / quant-result-access`；策略版本不硬编码，其他有效策略仍可并存。负向测试使用正确 Fingerprint 证明错误策略身份仍被拒绝。
- Quant AccessAudit Migration v4 持久化 content_level、完整 AuthorizationRef/PolicySnapshot JSON、PolicySnapshotRef、DecisionFingerprint；不存结果 Payload。
- Go Test Suite [45102027281](https://github.com/Kasign/stock_data_provider/actions/runs/45102027281) success；Integration CI [45102027283](https://github.com/Kasign/stock_data_provider/actions/runs/45102027283) success（含 Quant MariaDB audit/result 持久化测试）；Quant module validation [45102582473](https://github.com/Kasign/stock_data_provider/actions/runs/45102582473) success（gofmt、单测、MariaDB no-skip、Manifest Fuzz、Race、Vet）。
- 已将 Quant 专项 CI 的 push 分支加入 repair_all，保证后续聚合分支变更不会漏掉 Quant 专项验收。
- Platform OwnerAccess 可复用 Owner Bearer + platform:admin 管理认证；Owner Session 不具备该权限。将其绑定为 Quant Policy 发布/撤销权限仍需明确治理映射，不把通用认证能力等同于已批准的领域权限。
- Q-01 仍 BLOCKED：Policy Owner 规则/不可变版本/current pointer/签发撤销、生产 AuthorizedResultReader、Researchflow 强类型 Run→QuantResult 输出关联、Server Root 完整注入均未完成。用户已确认的“仅读取当前 Run 自己创建或明确关联的结果”约束继续生效；不得默认 Allow、跨 Run 读取或由 Quant 访问 Researchflow 私有 SQL。


## 40. 2026-11-23 Q-01 策略管理提交复核与下一步

### 40.1 本轮精确基线与 CI

- 代码复核基线：`repair_all` SHA `c27cba069506187eb072dac1616babdf5d7aa282`。
- Quant 策略管理实现提交：`15f882fa6d13ac1fac040170b49646c30b13200e`、`dd828baf4dba2e7d697ca260b941cd9dccb404a7`；Cookie 配置修正：`6c0c12fc4e8b8b4f3cb19e06e569a922cdc71730`；后续两次提交仅修正格式。
- 精确 SHA `c27cba069506187eb072dac1616babdf5d7aa282` 的 Go Test Suite [45104096706](https://github.com/Kasign/stock_data_provider/actions/runs/45104096706)、Integration CI [45104096674](https://github.com/Kasign/stock_data_provider/actions/runs/45104096674) 均 success；Quant module validation [45103489316](https://github.com/Kasign/stock_data_provider/actions/runs/45103489316) success。
- 本轮没有切换分支、合并、rebase 或重写历史。

### 40.2 已完成的策略管理子项

- Quant 新增策略不可变版本表、current pointer 和 Owner 管理审计表；发布/撤销在 MariaDB 事务中推进版本，行锁与 expected_version 阻止并发旧版本覆盖。
- Server 将管理路由绑定 Owner Session；使用真实 Cookie 配置和 Session Authenticator，覆盖 Owner 身份、Origin/CSRF、Bearer 不得替代 Owner Session 等测试。
- 以上只完成 Policy Owner 管理/版本生命周期子项。当前快照是固定策略身份及版本号，Enabled 是策略总开关，不构成对任意 subject/resource 的 Allow。

### 40.3 Q-01 仍未完成的硬门槛

- Quant 生产 `AuthorizedResultReader` 尚不存在，`GetResult` 仍只有 Application Port 和单测替身；Server 未装配完整 PrincipalResolver/Reader/Auditor/UTC Clock 读取组合。
- Researchflow 当前生产 Stage Output Registry 只有 News Search Entry；没有 Quant Stage Provider，也没有已注册的强类型 `quant_result_id` 输出关联。通用 StageOutput 的 resource_ref/run_ref 不足以推导 Quant result_id 关联。
- 读取必须同时验证 GOV-CTR-006 的可信 Researchflow principal、精确 result_id、purpose=quant_analysis、content_level=full、当前策略状态，以及当前 Run 自己创建或明确关联该结果。策略撤销与授权读取必须共享可证明的一致性边界；否则 fail-closed。
- 不允许 Quant 直接查询 Researchflow 私有 SQL，不允许按主体全局放行，不允许通过 JSON 字段猜测、通用 resource_ref 或 Golden 示例补造关联事实。

### 40.4 下一执行切片与顺序

1. 继续 Q-01：先在 Researchflow 既有 StageOutput/ResultValidation 职责内核对并定义强类型 Quant Result 关联只读 Port；只有存在正式生产写入契约后，Quant 才实现 Reader，Server 再组合注入。
2. 增加 MariaDB 集成与并发验收：同 Run 关联允许、跨 Run 拒绝、无关联拒绝、策略撤销与读取竞态拒绝旧授权、审计失败不释放结果。
3. 完成 QNT-013/Q-01 并更新 Quant、Researchflow、Server 与总计划精确 SHA 证据后，才进入第二批 Data D2；不因策略管理子项通过提前切换主线。


## 41. 2026-11-23 最新 SHA 全量 CI 与 Q-01 关联边界复核

- 代码验收基线：`9a84615ae0dc611b35681eadd6cfde70dd867703`。
- Quant 专项验收 [45105490677](https://github.com/Kasign/stock_data_provider/actions/runs/45105490677)、Go test suite [45105490653](https://github.com/Kasign/stock_data_provider/actions/runs/45105490653)、Integration CI [45105490843](https://github.com/Kasign/stock_data_provider/actions/runs/45105490843) 均 completed / success。
- 策略管理审计集成测试已验证发布、撤销的 actor/action/前后版本记录；gofmt 首次失败已修正，最新 SHA 全部通过。
- 沿 Worker → StageExecutionResult → StageOutputCommit → CommitAttemptResult → StageOutputResource 核查：Stage Output 资源在 Attempt/Stage 结果事务中持久化，且携带 Run/Stage/Attempt 元数据；但没有 Quant Result ID 的强类型字段或独立关联记录。
- 当前生产 Researchflow composition 只装配 News Search Stage，没有 Quant Stage Provider；因此不能以通用 output_ref、resource_ref 或 JSON 猜测构造 Run→QuantResult 授权关联。
- 下一步 Q-01 必须先明确 Quant Stage 的强类型输出契约：Provider 成功返回的 Quant Result ID 必须与已验证的 Quant Result Envelope 一致，并在 Researchflow 完成 Attempt/Stage 的同一事务中写入不可变 Run→QuantResult 关联；之后 Quant Reader 才能在策略 head 行锁定的一致性事务中校验该关联、读取不可变结果并签发 GOV-CTR-006 授权证据。
- Q-01 未完成前不进入 Data D2。


## 42. 2026-11-23 QNT-013 / Q-01 结果生产入口基础能力

- 本轮只推进第一批 QNT-013 / Q-01，没有进入 Data D2。
- 新增 Quant 强类型结果记录构造与 Application CreateResult，用现有不可变 Quant Repository 持久化，不增加平行存储。
- 对 Trend/Risk Payload 与 Manifest 的关键元数据一致性增加 fail-closed 校验，并覆盖无效 discriminator、元数据不一致、取消、存储失败测试。
- 代码验收 SHA：`c1f1b294a0d5e41756d058bf4910464e8eefaffd`。Quant 专项验收 [45107613322](https://github.com/Kasign/stock_data_provider/actions/runs/45107613322)、Go test suite [45107613321](https://github.com/Kasign/stock_data_provider/actions/runs/45107613321)、Integration CI [45107613334](https://github.com/Kasign/stock_data_provider/actions/runs/45107613334) 均 success。
- 代码提交：`4efce013`（BuildRecord）、`2c36b51d`（CreateResult）、`50a1ad2c`（测试）、`978b5b88`（格式修正）、`9ee20f0f`（载荷与 Manifest 元数据一致性）、`c1f1b294`（一致性测试）。
- 该基础用例尚无真实 Trend/Risk 生产算法调用；Researchflow 尚无 Quant Stage Provider/Registry Entry，故 Run→QuantResult 关联和生产授权读取仍未完成。
- 后续顺序：先核实 Quant 现有领域算法是否具备可调用的正式生产入口；只在有真实算法调用的前提下接入 CreateResult 与 Quant Stage Provider；Researchflow 用精确 Stage Output Registry 校验并在现有 Attempt 事务中持久化 result_id；随后接通关联只读 Port、Quant AuthorizedResultReader、策略撤销一致性和 Server 装配。所有步骤仍受同 Run/跨 Run/无关联/撤销竞态/审计失败测试约束。


## 43. 2026-11-23 Q-01 / QNT-014 依赖顺序校准

- 最新复核基线：`repair_all` HEAD `689c0b83c8508872236564283552815ca5c66945`；最近生产代码 SHA `5b8aba15a3148ca36c818ffc2b05ee609f5a62ae` 的 Quant module validation（Run `45115231452`）、Go Test Suite（Run `45115231420`）、Integration CI（Run `45115231437`）均 success。HEAD 是文档提交，无独立 Actions Run。
- 对照最新代码修正历史状态：Quant 策略版本/current pointer、Owner Session 发布撤销管理、策略 Head 共享读取锁均已实现并有 MariaDB/专项测试；此前“Policy Owner 管理/撤销一致性尚未实现”的历史描述已过期。它们是 QNT-013 已完成子项，不得重复建设。
- Q-01 仍 BLOCKED 的原因不是策略开关/版本存储缺失，而是完整生产数据链不存在：没有真实 Quant 结果生产 Facade；Researchflow 没有 Quant Stage Provider/强类型 result_id 输出；没有生产 AuthorizedResultReader；Server 没有完整读取依赖装配。
- 补充复用边界：DATA-CTR-008 的 Snapshot Manifest 1.0.0 Schema 与 Golden 已存在，必须复用，不得另建第二套 Manifest。当前缺口是 Data 生产链尚未把 DailyBars v2 Payload 内容指纹、snapshot_ref、可信 caller/purpose 与 GOV-CTR-006 AuthorizationRef 绑定并提供可验证的读取/撤销复核；该契约实现证据仍需按项目登记簿完成。
- 依赖顺序存在真实循环：Quant Stage 只有在真实计算 Facade 能创建不可变结果后才能产出 result_id；Run→result_id 关联建立后 Reader 才能授权读取；但 QNT-014 Facade 又需要 DATA-CTR-005/007 的精确输入、Snapshot 身份和授权证据。因此“Q-01 完整关闭后才开始 Data D2”在当前代码事实下不可执行。
- 执行顺序据此校准为：①只推进首个生产指标必需的 Data D2 最小契约切片（Decimal close、来源/scale 证据、交易日/interval/复权语义、CalendarRef、Snapshot Manifest/Payload fingerprint、AuthorizationRef；无证据字段继续 null/degraded；不扩展为全 Provider/Amount/Factor 的完整 D2）；②Quant 基于该可信输入完成首个正式 Registry Entry、Payload、Metric 构造和生产 Facade；③Researchflow 注册真实 Quant Stage/Output Schema/Provider，并在现有 Attempt/Stage 原子事务中持久化精确 result_id 关联；④Quant 在策略 Head 锁内通过 Researchflow 公开只读 Port 校验 Run-result 关联、读取不可变结果并写 AccessAudit；⑤Server 完成可信依赖注入及同 Run/跨 Run/无关联/伪造 RunRef/撤销竞态/审计失败 MariaDB 验收。
- 本次顺序调整不代表 DATA-CTR-005/007 已冻结或已解锁；项目登记簿仍按 deferred 处理。只允许先完成契约证据盘点/设计与正式重新进入审查，未取得契约 Owner 的冻结/重新进入依据前，不发布对外 DTO/HTTP 契约或伪称生产可用。
- FLOW-005/FLOW-006、LLM/Notification 空壳接入、广泛 Provider 精度/Amount/HFQ 扩展均不在本切片内；待真实调用链形成后按主线推进。
- 本次只校准任务状态与依赖顺序，不修改生产代码，不改变测试标准，不创建分支，不切换分支。


## 2026-11-23 装配优先执行口径校准（以 HEAD ca79ef0 为基线）

本轮按用户确认的执行原则收敛范围：**Server 装配不再等待所有上游模块业务能力完善；先接入当前已有且具备生产条件的能力，再以真实业务流程暴露具体缺口。** 数据质量、算法覆盖和领域规则缺陷由对应模块独立负责，不作为 Server 初始化的笼统前置条件。

### 当前代码事实（已核对 repair_all）

| 能力 | 当前状态 | 结论 |
|---|---|---|
| Root 共享 MariaDB/Redis、Platform HTTP Client、Migration Registry | 已有 | 不重做；按现有 Root 资源所有权继续复用 |
| Data Module 构造、Root Handler、Readiness、Lifecycle | 已有 | 不重做；真实 Provider 数据质量另由 Data 验收 |
| News Search Stage、Researchflow Run/Template/Worker Runtime | 已有 | 当前正式执行 Stage 只有 News Search；不把它误报为完整研究链路 |
| Research News Evidence Query/Importer 组合 | 已有，按 Cursor Key Ring 配置启用 | 尚无真实 Researchflow 消费 Stage；不创建占位消费者 |
| Research/Researchflow Outbox 到 Notification Inbox Relay | 已有 | 不等于 Notification Delivery；后者不是首条研究报告链路的阻塞项 |
| Quant Result Repository、审计和策略存储基础 | 模块实现已有部分，Root 仅装配策略管理入口 | 生产 AuthorizedResultReader、真实计算 Facade、Quant Stage Provider/关联契约未形成；不能由 Server 伪造 |
| LLM Transport Adapter | 已有薄适配器 | 缺真实 Gateway 配置/Secret/Capability 与实际消费者；不创建空 Provider |
| Research Context | Repository/Service 组合部分存在 | Context Owner Authorizer 和真实消费者未形成；不注入默认 Allow |
| Researchflow 新 HTTP API | Application Create/Get/Cancel 用例及 Owner composition 存在 | Root Router 未挂载 Researchflow Run HTTP Handler；SRV-CTR-001 仍 deferred，不能沿用已退役旧 Analysis DTO 或自行猜测公开契约 |

### 本轮执行顺序

1. **Server 已有能力装配核验**：逐项确认构造、配置、注入、Migration、Readiness、Lifecycle、HTTP 注册及 Worker 注册；已通过的项目不重复建设。
2. **Server 真正缺失的装配补齐**：仅对已存在且契约明确的生产 Adapter/Use Case 补 Root 注入和测试；缺少领域实现或正式契约的项目登记为上游/契约阻断，不由 Server 造假补齐。
3. **Researchflow 对外 HTTP 契约**：基于现有 CreateRun/GetRun/CancelRun Application、WebUI 当前页面场景和 Owner Session 认证边界，先形成 SRV-CTR-001 最小提案；契约未确认前不挂载猜测性路由。
4. **最小真实流程**：先以现有 News Search Workflow 验证 WebUI→Root→Researchflow Run→Worker→Stage Output→Run 查询；再逐项接入 Quant/Research/LLM，不能把 News Search 的成功冒充完整 A 股研究报告成功。
5. **定点完善**：数据质量、算法规则、PIT/证据、通知投递等只在真实消费者需要时进入当前主线；非首条闭环必需能力后置。

### 当前判断

当前 Server 并非“完全没有生产装配”。Root 的基础设施、Data、News Search、Researchflow Runtime、部分 Research/Notification Relay 已有真实组合代码。当前最明确的 Server/API 缺口是 **Researchflow CreateRun/GetRun/CancelRun 尚未形成 Root 对外 HTTP 接口**；而 Quant 真实计算/授权读取、Research Context 授权消费、LLM 真实调用分别仍缺领域 Owner 交付，不能算作单纯 Server 装配遗漏。

本轮文档校准不代表代码验收完成。后续代码变更必须附精确 SHA 与针对性测试/Integration CI 证据。


### SRV-CTR-001 当前进度

- 已完成现有 Root 装配代码复核，并将“上游业务完善”与“Server 组合装配”分开登记。
- 已形成最小 API 草案：`doc/server/contracts/SRV-CTR-001-Researchflow最小HTTPAPI提案.md`，只覆盖 CreateRun/GetRun/CancelRun，复用现有 Application/Owner Session，不新增平行 ID 或业务状态。
- 当前仍为 DRAFT，尚未修改生产 Handler/Router。原因是项目级登记中的 SRV-CTR-001 仍为 deferred；必须先确认正式公开接口范围，避免沿用已退役旧 Analysis DTO 或猜测接口。
- 下一步：确认提案后实现正式 OpenAPI/Schema、Handler、Root Router、CSRF/资源授权测试和 WebUI 调用；再执行真实 MariaDB/Redis Worker 集成验收。


## 40. 2026-11-23 SRV-CTR-001 Researchflow HTTP API 冻结与实现

- 用户确认按三个接口和 Owner Session 认证方式执行：POST `/api/v1/researchflow/runs`、GET `/api/v1/researchflow/runs/{run_ref}`、POST `/api/v1/researchflow/runs/{run_ref}/cancel`。
- 正式契约 `doc/server/contracts/SRV-CTR-001-researchflow-http-api.md` 已冻结为 1.0.0；项目级登记簿 `doc/contract/PLAT-002-项目级契约冻结登记簿.md` 已将 SRV-CTR-001 从 `deferred` 更新为 `frozen`，登记簿版本 0.2.63-frozen。
- OpenAPI、CreateRun Request Schema、Run Aggregate Response Schema 和 Golden 已发布；旧提案文件已标记为历史归档并指向正式契约。
- Root 已接入 Owner Session HTTP Handler、三个路由、显式 Run/Stage/ResultSummary DTO 投影和 WebUI API 封装。复用现有 CreateRun/GetRun/CancelRun Application、Owner Session Identity Adapter、SessionAuthenticator 和 Researchflow Run 资源级授权；没有重写业务逻辑、创建平行认证体系或新增 ID 体系。
- 新增测试覆盖 Owner Bearer 拒绝、Session 与 Bearer 混合凭据拒绝，以及响应 Schema 字段投影和内部字段隔离。
- 精确代码 SHA `ab6225a5d936375ebacf6bd57c609502d8a7f75e`：Go Test Suite [Run 45132439066](https://github.com/Kasign/stock_data_provider/actions/runs/45132439066) 与 Integration CI [Run 45132439148](https://github.com/Kasign/stock_data_provider/actions/runs/45132439148) 全项 success。覆盖 gofmt、root Go tests/race/vet、Researchflow/Research、MariaDB/Redis Worker Bootstrap、WebUI syntax/unit/build。
- **当前状态：契约 FROZEN；代码 CODE_CI_PASS；目标环境 L3 验收 PENDING。** 剩余工作仅为真实 HTTPS 浏览器 Owner Session 登录/CSRF 恢复后执行 CreateRun、GET 轮询、CancelRun，并核验真实 MariaDB 持久化、Worker 执行和最终状态回读。不能把 CI 成功描述为生产端到端验收完成。


## 2026-11-23 Indicator Payload 进入 QuantResult 正式类型联合

- 在既有 QNT-CTR-004 1.0.0 冻结语义内补齐 L2 实现，不新增公开 DTO 或新版本契约。
- IndicatorPayload 原有构造器、指标映射和字段一致性校验保留；现已加入 Quant Result Payload 正式联合，支持 Envelope 解码、不可变 Record 构造与 Manifest 绑定。
- Envelope 与 Record 均校验所有 Metric 的 period、system_as_of、calculation_version、quality_status、metric_status 与 Manifest 一致；ResultStatus 由 MetricStatus 按固定映射校验，禁止元数据漂移。
- Envelope Schema 的 result_type 增加 indicator 分支，并复用现有 IndicatorResultPayload 1.0.0 结构定义；没有复制第二份参数或指标语义。
- 新增 SMA Indicator Payload 从 Record 构造、Envelope 编解码到 Manifest 校验的测试，以及 Manifest 上下文漂移拒绝测试和 Envelope Schema 正向测试。
- 本项只打通 Indicator 结果载荷的生产/持久化/回读基础，不代表真实 SMA/EMA 生产计算 Facade、Registry 活动版本、Data 快照输入或 Researchflow Stage 已完成。


## 2026-11-23 Data DailyBars Snapshot 存储与 OHLC Decimal 输入审计

- 审计基线：repair_all `2dc294d48dc57ba186997ea8452c0202a08eaad8`；本轮只读审计，未修改生产代码或冻结契约。
- 现有 `market_data_daily_bar_versions.metadata` 已保存 `DailyBarExactValues` sidecar；`dailyBarRevisionHash` 基于完整 `dailyBarMetadata` 计算，包含 exact_values、来源和质量等字段，仅排除 fetched_at、normalized_at、system_observed_at。现有单 Bar 版本历史不需要复制或另建一套。
- `market_data_daily_bars` 是可更新当前视图；`ListBarsAsOf` 负责读取单 Bar 历史修订并由 Service 选择来源一致的序列。两者都不能仅凭 SnapshotRef 精确寻址一次完整查询结果，也不保存该次查询的覆盖率证据与 Manifest 绑定。
- Migration V5 的 `market_data_trading_calendar_snapshots` 已有内容寻址、不可覆盖写入、读取时重算摘要及篡改拒绝实现。日线 Snapshot 若进入实现阶段，应复用同类 Repository 模式，只新增“整次查询结果”的不可变记录，不重复逐 Bar 版本数据。
- 默认 Provider 优先级首位为 Eastmoney；其日线 OHLC ExactValues 从原始 CSV 字段字符串构造，CloseEvidence 保存 observed token 和 source fraction digits。对首个 SMA 输入可以限定 `adjustment_basis=none`，直接使用精确 Close 字符串；不得从 float64 反向格式化。
- BaoStock 原始字符串、Tushare JSON RawMessage、Netease CSV 字段、TickFlow JSON RawMessage 也有对应精确值路径；Pytdx 使用协议 Price.Int64 与 ProtocolScale，属于协议定点表示证据，不应混称原始小数位。Hithink 当前默认关闭，且其 Amount 单位未知与首个 OHLC 输入无关。
- 当前 DailyBars Provider 的 `SourceVersion` 是 Adapter/API 版本常量，不是逐条 Provider 数据版本；Eastmoney 未暴露逐 Bar ProviderTime/provider_data_version。现有 D2 解阻方案明确：Research Evidence 的 `source_version` 只填可证明的 Provider 数据版本，缺失时显式缺失；Adapter 常量和 FetchedAt 均不得替代。
- DATA-CTR-008 Manifest 1.0.0 的 `sources[].source_version` 却是必填非空字符串，旧 Golden 又以时间戳填充该字段。这与当前 Provider 缺少逐条数据版本的事实存在输入兼容缺口；不能用 Adapter 常量或 FetchedAt 填平，也不能擅改冻结的 1.0.0。需要在契约治理中明确 Manifest 来源版本语义/兼容路径后，才可签发这类来源的正式 Manifest。
- Snapshot 身份必须绑定 Manifest 与 Payload：分别计算 `manifest_fingerprint` 和 `payload_fingerprint`，再对二者的规范化组合计算 `snapshot_ref`。不得把 SnapshotRef 简化为 PayloadFingerprint，也不得把旧 Golden 中两个值相等当作 Payload 已绑定的证据。
- 当前决策：无需新增逐 Bar 版本存储；未来 Snapshot Repository 只保存整次规范化 Payload、Manifest 和内容指纹。首个 SMA 可限定精确 Close、`adjustment_basis=none` 和同次 CalendarRef，但在 Manifest 来源版本缺口解决前，不得将它接成正式 Quant 生产输入。DATA-CTR-005/007 仍 deferred，不据此宣称跨模块契约已冻结。


## 2026-11-23 Data Snapshot 审计结论固化与下一任务

- 已把 Data 日线版本、精确 Close 来源、交易日历 V5、Provider SourceVersion 语义、Manifest 冲突和 SnapshotRef 指纹边界同步到 Data 依赖方案、Data 当前任务入口及 Data 卡点文档，避免后续每轮重复代码审计。
- 审计事实：单 Bar ExactValues 已在 revision metadata 持久化，revision hash 覆盖；V5 Calendar Snapshot 可复用；Eastmoney 原始 CSV Close 可作为首个未复权 SMA 输入；当前 SourceVersion 是 Adapter/API/协议版本而非 Provider 数据修订版本。
- 真实契约问题：DEC-PLAT-002-020 要求 Provider 数据版本缺失时显式缺失；DATA-CTR-008 1.0.0 Schema 却要求 sources.source_version 为非空字符串。冻结 1.0.0 不得原地改写，也不得用 Adapter/FetchedAt 伪造。
- SnapshotRef 必须绑定独立 ManifestFingerprint 与 PayloadFingerprint；实际组合公式、Payload Schema、来源版本表达及 GOV-CTR-006 resource_ref 关联由 DATA-CTR-007 新版契约/Golden 定义。
- 当前下一任务：做 DATA-CTR-008 版本演进与 DATA-CTR-007 新版契约的定点设计/影响分析，先确认字段版本语义和消费者兼容策略，再进入 Data Snapshot Repository、Owner 授权及 Quant SMA 生产链路。DATA-CTR-005/007 未达到重新进入条件前仍按登记簿状态管理。


## 2026-11-23 Data Snapshot 来源版本契约裁决与下一阶段

- Data 审计基线已固化到 Data 依赖方案、当前任务入口和卡点文档；单 Bar ExactValues/Revision Hash、V5 Calendar Snapshot、Eastmoney 原始 Close 输入边界不再重复审计。
- DEC-PLAT-002-055 已裁决 Provider 数据版本缺失表达：DATA-CTR-008 1.0.0 不改；2.0.0 显式 schema_version，source_version required+nullable，provider_interface_version 独立可选，不允许伪造 Provider 数据版本。
- DATA-CTR-007 负责定义 Payload Schema、PayloadFingerprint 和同时绑定 Manifest/Payload 的 SnapshotRef；GOV-CTR-006 resource_ref 绑定最终 SnapshotRef。
- 下一阶段：发布 DATA-CTR-008 2.0.0 与 DATA-CTR-007 Schema/Golden，完成 Research/Quant 消费者版本选择与兼容性测试；完成前不进入正式 Snapshot Repository/Quant SMA 生产读取。


## 2026-11-23 Data Snapshot Schema 草案进度

- 已新增 DATA-CTR-008 2.0.0 Manifest Schema 与 DATA-CTR-007 Close Series Payload 1.0.0 Schema 草案，均通过 JSON 语法解析。
- Payload 首批范围：单证券、1d、未复权、精确 Close、有效同次 CalendarRef 和 CoverageEvidence；逐 Bar 保留当前 Data normalization_version（normalize.Version 当前为 4）。
- 历史 Live Provider Artifact 14611734848 不包含逐 Bar 原始 token/ExactValues/CalendarRef/CoverageEvidence，因此不能构造真实 Golden；本轮没有重新发起 Provider 请求。
- 下一步先检查 MariaDB revision metadata 能否恢复同一批次完整 Golden 输入；若不能，才安排受控真实读取。Golden、Validator、Research/Quant 消费者适配未完成前不实现 Snapshot Repository 的正式写入入口。


## 2026-11-23 Data Snapshot Golden 的 PIT 读取路径确认

- DailyBarsAsOfWithCoverageEvidence 已存在：从 MariaDB ListBarsAsOf 读取历史日线版本，none 复权路径不访问外部 Provider；随后按同一 as_of 读取 Calendar Snapshot，计算并返回 CalendarRef/CoverageEvidence。
- Application Reader.GetDailyBarsV2 在显式 SystemObservedAtCutoff 下使用上述路径，并保留 ExactValues。
- 因此真实 Golden 可从既有 MariaDB 历史修订通过 PIT V2 Reader 生成，无需重新请求 Provider。当前 GitHub 历史 Artifact 仅有聚合统计，不足以构建 Golden；助手当前也没有用户运行中 MariaDB 的直接连接。
- 下一步应先通过现有 Server/应用入口对已有历史数据执行一次受控 PIT Snapshot 读取，保存真实输出作为 Golden 输入；若数据不完整再记录具体缺项。


### 2026-11-20 历史 Workflow 日志原始样本核对（不触发新请求）

- Live Provider Run 44699296891 / Job 136163491198 的日志确实包含 Eastmoney DailyBars 样本，但只有 forward_adjusted 与 backward_adjusted 两组；没有 none 未复权样本。
- 日志中的 sample_bars 是 float64 投影，虽然 HTTP body_sample 含部分原始 CSV 行，但响应采样被截断，且这些请求为复权口径；不具备本次首批 none SMA Payload 所需的完整 ExactValues + 原始 Close token + 同次 CalendarRef/CoverageEvidence。
- 因此该日志可证明 Eastmoney 对应复权请求曾真实返回，但不能作为本次 Snapshot Golden。不要从这些浮点样本反推 Decimal，也不要把复权样本改写为 none。


## 2026-11-23 Data Close Series Snapshot Builder 实现进度

- 新增 internal/marketdata/app/daily_bars_close_series.go：BuildDailyBarsCloseSeriesInput 将同一次 DailyBarsV2Reader PIT 结果映射为 DATA-CTR-007 首批 Payload，并同时产出 Manifest sources 输入和 SystemAsOf。
- 首批实现明确只支持 Eastmoney 未复权 1d：必须有显式日期范围、正数 Limit、显式 SystemObservedAtCutoff、同次 CalendarRef、valid Exchange Calendar、coverage_sufficient=true；不允许用 float64 回填 ExactValues。
- 每条 Bar 必须有精确 Close、observed-token Evidence、SourceFractionDigits、NormalizationVersion、FetchedAt/NormalizedAt/SystemObservedAt；Close 与原始 SourceToken 用精确有理数比较，SourceFractionDigits 必须与 token 小数位一致。
- 当前 Eastmoney SourceVersion 已核实为 eastmoney-stock-api-v1，映射到 provider_interface_version；Manifest source_version 明确为 nil，不伪造 Provider 数据版本。normalization_version 从 Data DTO 读取，当前 normalize.Version 为 4。
- 新增 daily_bars_close_series_test.go，覆盖正确映射、JSON 字段名、缺 CalendarRef、日历降级、覆盖不足、float-only、来源版本不匹配、token 不一致、非 PIT、重复交易日和空诊断字段拒绝。测试代码已写入，但最新 Go Test Suite 在 gofmt 门禁处提前失败，未执行该测试；本轮最后的格式修正提交使用 skip-ci，故最终 HEAD 尚无通过 CI 的证据，不能宣称测试通过。
- 本轮 push 触发的 Live provider probe workflow 中，实际 Provider 探测步骤全部 skipped（没有包含 [include_public] 标记，也未手动 dispatch），未重新请求 Provider。
- 当时尚未实现 Manifest/Payload JCS 指纹、SnapshotRef、Migration V6、不可变 Snapshot Repository、Data Owner 授权签发/撤销复核；Builder 当时只是输入构建层，不对外开放 Snapshot 读取。（注：这些能力已在同日后续"Snapshot 双指纹构建与校验""Snapshot V6 Repository 实施"及 Data Owner 策略持久化段落记录为已实现，属历史过程记录。）

### 下一步

1. 先通过真实 Data PIT V2 Reader 获取一份既有 MariaDB 历史日线的完整输出（不访问 Provider），确认 Builder 能从真实 Eastmoney ExactValues/CalendarRef/CoverageEvidence 生成 Payload。
2. 使用项目 JCS 库分别计算 PayloadFingerprint、ManifestFingerprint，再按 DEC-PLAT-002-055 对两者规范化组合计算 SnapshotRef；补 Golden 和契约校验。
3. 再新增 Migration V6 和不可变 Snapshot Repository，随后接 Data Owner 授权和 Quant Consumer。


## 2026-11-23 Snapshot 双指纹构建与校验实现

- 新增 internal/marketdata/app/daily_bars_close_series_manifest.go。
- PayloadFingerprint：对 Payload JSON 执行 RFC 8785 JCS 后 SHA-256。
- ManifestFingerprint：对 DATA-CTR-008 2.0.0 Manifest 指纹输入（不含 snapshot_ref / manifest_fingerprint）执行 JCS + SHA-256。
- SnapshotRef：对规范化对象 {manifest_fingerprint, payload_fingerprint} 执行 JCS + SHA-256，并加 data_snapshot:sha256: 前缀；不将两份指纹简单拼接，也不将 ManifestFingerprint 单独作为 SnapshotRef。
- Build/Verify 同时检查 schema 版本、purpose、caller_ref、来源绑定、PIT 时间边界、Calendar/coverage、Exact Close Evidence 和三份指纹；返回值对 Payload/来源指针做深拷贝，避免调用方修改原输入后改变已构建 Snapshot。
- 新增 Manifest 指纹测试：Manifest 改变时 PayloadFingerprint 保持不变但 SnapshotRef 改变；Payload 改变时 ManifestFingerprint 保持不变但 SnapshotRef 改变；同时覆盖篡改、PIT as_of、调用主体和用途拒绝。
- 当前仍未实现 Migration V6、不可变 Snapshot Repository、Data Owner 授权签发/撤销复核、Server 注入和 Quant 消费者；真实 MariaDB PIT Payload Golden 也未生成。因此此阶段只是 Snapshot 计算/校验层，不宣称生产闭环完成。


## 2026-11-23 Snapshot 生产服务实施进度

- 已从 `repair_all` 最新基线启动 Data Snapshot 生产服务修复。首个实际缺陷为 Close Series Payload 质量准入过宽：现已要求首批 SMA Snapshot 的聚合质量和每条 Bar 质量必须为 `valid`，并同步 DATA-CTR-007 Schema 与拒绝测试。
- 当前仅完成质量门禁子项，未宣称 Snapshot 生产服务完成。后续按真实依赖顺序处理 Payload/Manifest Golden 与 Validator、Migration V6 不可变 Repository、Data Owner 授权和 Server 装配，再接 Quant。
- 本轮 push 自动触发 Live Provider Probe 工作流；提交未包含 `[include_public]`、`[include_daily_bars_v2]` 等探测标记，真实 Provider 请求步骤按工作流条件跳过，且未手动 dispatch。真实历史数据 Golden 仍需从现有 MariaDB PIT Reader 物化，不能使用合成样本冒充。


## 2026-11-23 Snapshot Schema / Builder 一致性门禁

- 已增加 DATA-CTR-007 与 DATA-CTR-008 2.0.0 Schema 编译及 Builder 输出一致性测试，并覆盖非 valid 质量状态的 Schema 拒绝。
- 当前仍缺真实历史 Payload/Manifest Golden 与 DATA-CTR-007/008 消费者 Contract Test；Schema 文件存在不等于契约已冻结。Repository V6 写入与跨模块生产消费仍以契约治理状态为准。


## 2026-11-23 Snapshot V6 Repository 实施

- 已实现 Data 内部 Snapshot Repository Port、Migration V6 和 MariaDB 不可变 Repository。保存/读取以 SnapshotRef 精确定位，保存幂等且不覆盖，读取重新 Verify Manifest/Payload 双指纹及组合引用；数据库未配置时 fail closed。
- V6 仅增加查询级 Snapshot 表，不复制 `market_data_daily_bar_versions` 的逐 Bar 历史数据。物理 Schema 校验纳入 Data Store Compatibility Check。
- 测试覆盖 V6 Migration 定义、MariaDB 幂等/并发/篡改拒绝和存储未配置。下一步为 Snapshot Application 创建/读取用例，并接入 Data Owner 授权；跨模块公开消费仍需完成真实数据 Golden 与契约测试。


## 2026-11-23 Data Owner 策略持久化阶段

- Data Owner 已开始独立实现：新增 Data Access Policy 类型与最小策略评估规则，要求显式 subject/purpose/scope allow-list，且策略有效期/状态必须通过校验。
- Data Migration V7 增加不可变策略版本表与当前策略指针表，版本保存与指针切换事务化；不写默认允许策略，未发布策略时保持拒绝。
- 下一步完成策略读取原子性、Data AuthorizationRef/PolicySnapshot 签发与不可变授权记录，再接 Snapshot Application。


## 2026-11-23 Data Owner 策略发布并发返回语义修复

- 最新基线：`repair_all` SHA `766bb0292d2daba2d70876b632803db69a554b7b`；Go Test Suite [45172688942](https://github.com/Kasign/stock_data_provider/actions/runs/45172688942) 与 Integration CI [45172688886](https://github.com/Kasign/stock_data_provider/actions/runs/45172688886) 均 completed / success。
- Review 发现 DataAuthorizationPolicyOwner.Publish 在 CAS 发布成功后重新读取 current pointer 作为返回值。若另一发布者在其后立即推进指针，本次发布已成功却会因当前版本不再等于本次版本而错误返回 unavailable。
- 修复为直接返回本次已通过 DecodePolicyVersion 校验并成功持久化的不可变 PolicyVersion/Policy；不再进行发布后的 current pointer 查询。Current() 仍负责读取真正的当前版本。
- 增加并发回归测试，模拟 CAS 成功后 current pointer 被下一发布者推进，验证本次 Publish 仍返回本次版本，且不二次读取 current pointer。
- 本修复只调整发布返回语义，不改变 CAS、策略授权、版本不可复用或默认拒绝规则。
- 最终代码 SHA：`cdcd5f905de1103e49e1b79686f00882e2ea405d`；Go Test Suite [45174381526](https://github.com/Kasign/stock_data_provider/actions/runs/45174381526) 与 Integration CI [45174381536](https://github.com/Kasign/stock_data_provider/actions/runs/45174381536) 均 completed / success，覆盖 gofmt、单测、Race、Vet、Data MariaDB、模块集成与 Docker 构建。
- 同 SHA 的 Live Provider Probe 工作流虽 completed / success，但所有真实 Provider 探测步骤均 skipped，不计为真实行情源验证。
- 下一步：冻结 Data Owner 策略管理的最小 Server HTTP 契约前，继续核对 Data 策略对象的管理动作边界和现有 Owner Session 安全约束；不直接复制 Quant 策略路由，也不提前暴露未经契约定义的管理 API。


## 2026-11-23 Data Owner 策略管理接口前置语义补齐

- 代码基线：`repair_all` SHA `50b6eba3e3c2f85cad593a9680f6424b536a4054`。
- 新增 `app.ErrDataAuthorizationPolicyNotPublished`，MariaDB Repository 将当前策略指针查询的 `sql.ErrNoRows` 映射为该明确状态；Data Owner Application 将其映射为 `ErrDataPolicyNotPublished`。数据库连接/查询故障仍保持 unavailable，不会被伪装成“尚未发布”。
- Application 单测覆盖未发布状态映射；MariaDB 集成测试覆盖尚无策略版本时的查询结果，之后继续验证首次发布、CAS 冲突、撤销和历史版本不可重激活。
- 精确 SHA 的 Go Test Suite [45176291646](https://github.com/Kasign/stock_data_provider/actions/runs/45176291646) 与 Integration CI [45176291723](https://github.com/Kasign/stock_data_provider/actions/runs/45176291723) 均 completed / success；gofmt、常规测试、Race、Vet、Data MariaDB、模块集成和 Docker 构建通过。
- Live Provider Probe Run [45176291862](https://github.com/Kasign/stock_data_provider/actions/runs/45176291862) 的真实 Provider 探测步骤均 skipped，不作为真实数据验证。
- Data Owner 策略管理 HTTP 提案已补齐未发布状态语义，但仍是 DRAFT；公开路由必须等待契约确认和冻结。


## 2026-11-23 Data Owner 策略管理方案复审与领域审计补齐

- 当前代码基线：repair_all @ 9615e8ab70df106856e74551353b48f52e331316。
- API 评审结论：保留现有 /api/v1/owner/** 管理控制面命名空间；owner 表示 Owner 管理入口，不表示 Data 资源归属，也不授予 Owner Data 读取权限。
- 发布操作从 PUT 当前资源改为 POST /api/v1/owner/data/daily-bars-access-policy/versions：语义为创建不可变版本并 CAS 推进 current pointer，避免把非幂等的版本发布伪装为 PUT 替换。
- Data Policy 首版状态收敛为 approved / revoked。删除当前没有独立授权行为的 restricted 与没有审批状态机支撑的 pending；Evaluator 对 approved 按明确 allow-list 判定，对 revoked 一律拒绝。
- 现有 Server SecurityAuditEvent 不包含策略前后版本和 Payload Hash，不足以作为领域发布审计。新增 Data Migration V9 market_data_authorization_policy_publications，记录前后版本、Payload Hash、可信 actor_ref、Platform Request ID 和发生时间；与策略版本插入、current pointer CAS 同一事务提交。
- Data Owner Application 现在要求有效 Platform Request ID；缺失时 fail closed。策略审计写入失败时发布事务回滚，不留下已生效但不可追溯的策略。
- 接口文档已改为评审后 DRAFT，建议编号 SRV-CTR-003；项目级登记簿维护分支仍为 feat/design_doc，未在 repair_all 擅自改动登记簿或宣称契约已冻结。
- 本轮 CI 首次发现 Go 格式问题和集成测试结构问题，已根据精确 job log 修正；后续 SHA 的 Go Test Suite 与 Integration CI 尚待最终结果。


## 2026-11-24 统一数据库基线集成测试修正

- 基线：repair_all `f1f05a0ede014c197b0a1201377acc4921e719f5`。
- 最新 Integration CI 的两个根模块失败均来自测试仍断言旧契约：Worker Bootstrap 测试继续模拟 `MIGRATION_ENABLED=false` / `DB_AUTO_MIGRATE=false`；Data Migration 测试仍要求 V1-V9 九个历史版本并预先构造旧版 Schema。
- 当前生产实现已经由 Data 模块提供一个完整 Schema 基线（version 1），由 Server Registry 统一执行；启动后另行执行 Migration History 和 Data/Analysis 最终物理 Schema 校验。
- Worker 集成测试改为验证已初始化数据库的重复启动幂等，以及人工删除必需列后 Root Server 在最终 Schema 校验阶段 fail-closed；同时确认启动失败没有自动改写损坏结构。测试不再设置或断言已废弃的 Migration 开关，也不再依赖旧的 `market_data.schema_drift` 文本标签。
- Data Migration 集成测试改为从真正空库开始，由两个 Root Registry 并发执行同一完整基线，验证 Migration History 只有一个已应用的 version 1，并检查当前 Data 表、关键列和非唯一时态索引均真实存在。
- 本项仅修正测试契约，不改变生产 Migration 定义、模块职责或历史兼容行为。待精确 SHA 的根模块测试与完整 Integration CI 通过后，再记录阶段验收结论。



## 2026-11-24 统一数据库基线 CI 验收通过与模块 Schema 清单阶段启动

- 验收代码基线：`repair_all` SHA `657da77140a20d6ed6bcd138f6f008ce1de408c9`。
- Go Test Suite [Run 45343484485](https://github.com/Kasign/stock_data_provider/actions/runs/45343484485) 与 Integration CI [Run 45343484473](https://github.com/Kasign/stock_data_provider/actions/runs/45343484473) 均 completed / success，且两者 head_sha 与当前分支 HEAD 一致。
- Go Test Suite 覆盖根模块、Market Data、Research、Researchflow、Notification MariaDB 集成，另有 Data Migration/并发初始化、Docker Compose 校验和镜像构建；Integration CI 全项通过。
- 此轮 Data 基线迁移测试清理问题已修复：管理连接的关闭纳入测试 Cleanup，使隔离库 DROP 在连接关闭前执行。没有修改生产 Migration SQL、Schema 或运行规则。
- 当前统一初始化阶段的 CI 门禁通过；这只证明当前代码及 CI 环境下的测试通过，不等于所有模块的最终物理 Schema 均已具备独立完整校验，也不等于真实部署环境验收完成。

### 下一阶段：模块 Schema 清单与最终物理结构校验审计

- Root Migration Registry 当前注册 9 个 namespace：Market Data、Analysis、News、Owner Credential、Research、Researchflow、Quant、Notification、Server Security。
- 已抽查 Analysis、News、Research、Researchflow、Quant、Notification 的 Migration 定义，当前均以单个 version 1 完整基线创建各自模块表；各模块 SQL 仍由模块存储层持有。
- Root `bootstrapDatabase` 当前对所有已注册 namespace 执行 Migration 和 History/Checksum 验证，但显式最终物理 Schema Validator 目前只调用 Market Data 与 Analysis。
- 下一步逐个审计 9 个 namespace：基线定义、实际 Repository 所需表/列/索引、Migration History 覆盖范围、是否已有最终 Schema Validator，以及缺失校验是否会导致启动后才暴露错误。只补有明确必要性的 Validator，不为追求数量给每个模块机械新增重复校验。
- 此阶段先完成清单和差异结论，再按真实风险补齐；不改变业务表结构、不引入兼容迁移、不把 Migration History 成功等同于物理 Schema 完整。


### 2026-11-24 Schema 审计第一轮结论（基于当前代码）

本轮检查了 Root Migration Registry、各模块 Migration 定义、现有物理 Schema 检查及 MariaDB 集成测试。结论如下：

| Namespace | 基线表规模 | 启动时物理 Schema 检查 | 现有集成测试检查重点 |
|---|---:|---|---|
| Market Data | 17 张表 | 有专用 Validator | 特殊列类型、索引、日历/收盘序列/授权结构 |
| Analysis | 4 张表 | 有 Validator，但目前只检查 4 张表是否存在 | Migration 单元测试检查基线 SQL |
| News | 14 张表 | 无 | MariaDB 集成测试检查 observations 的 5 个关键列；Repository 场景覆盖较多 |
| Owner Credential（Platform） | 1 张表 | 无 | Credential Store 集成测试实际应用 Migration 并验证存取流程 |
| Research | 6 张表 | 无 | 集成测试检查 event/claim 两个版本列的数据类型，并覆盖 Repository 流程 |
| Researchflow | 9 张表 | 无 | 有基线 SQL 单元测试及大量存储行为测试；未发现启动级物理 Schema Validator |
| Quant | 5 张表 | 无 | Repository 集成测试应用 Migration 后覆盖存取行为；未发现完整物理 Schema 清单检查 |
| Notification | 10 张表（另有 1 条种子数据语句） | 无 | 集成测试检查 Migration History、revision 种子值和一个冗余索引 |
| Server Security Audit | 1 张表 | 无 | 当前主要是 Migration 定义单元测试；Writer 本身有写入实现 |

审计确认的实际缺口：

1. Migration History 的 checksum 只证明已登记 Migration 定义没有变化，不能证明物理表、列、索引仍然存在。
2. 当前只有 Market Data 和 Analysis 在启动阶段执行最终 Schema Validator；Analysis Validator 也只检查表存在，不检查列和索引。
3. 其他模块的集成测试分别检查部分关键结构或通过 Repository 行为间接覆盖，但这些测试不等价于每次进程启动时检查当前数据库结构。
4. 因此目前不能声称 9 个 namespace 的物理 Schema 都已完成启动前验证。

处理原则：

- 不把所有 Migration SQL 再复制一份到 Validator 中，也不为每个表重复维护完整 DDL。
- 下一步按 Repository 实际 SQL 访问路径识别每个模块的启动关键结构：至少确认核心表存在，并为曾出现过结构漂移、或缺失后会导致核心读写立即失败的关键列/索引补充精确检查。
- 先评估能否在 Platform Storage 提供一个小型、只读的 Schema 元数据检查原语，模块只声明自己的关键结构；不做运行时自动修复，不在 Validator 中执行 DDL。
- 第一批优先 News、Notification、Researchflow、Quant；随后补 Owner Credential 与 Server Security Audit。Analysis 现有检查也要评估是否需要补关键列。Market Data 保留已有专用校验，不重写。
- Validator 只用于启动前 fail-fast；Migration 仍然是 Schema 的唯一写入来源。每个新增检查都必须有对应的缺失/漂移测试，避免只测正常建库。

## 26. RA-WEB-API-001 旧 Analysis API → Researchflow 迁移盘点（2026-11-24）

- 核对基线：`repair_all` SHA `8be3b57306fad9df39a923945f3df6f11790f683`。
- 盘点文档：[WebUI旧Analysis到Researchflow迁移盘点](./WebUI旧Analysis到Researchflow迁移盘点.md)。
- 当前状态：**调用链盘点完成；业务模板、完整结果读取和 WebUI 切换均未完成**。
- 已确认：`web/src/api.js` 已有 SRV-CTR-001 helper，但 `web/src/app.js` 仍通过旧 Task/Run/Event API 完成创建、轮询和结果读取。
- 已确认：Root Server 仍装配旧 `internal/analysis` Runner、MariaDB `platform/taskqueue`、`legacyAnalysisWorker` 和 `platform/httpapi.NewWithTasks`；旧路由仍按 Owner Bearer 注册。
- 已确认：Researchflow 生产 Template/Executor/Output Registry 目前只有 News Search；不能把股票分析请求映射成 News Search。
- 新发现的关键阻塞：旧页面结果视图依赖完整 RunResult（Quality、MarketState、Factors、Signals、Risk、Plans、Execution、Events 等）；SRV-CTR-001 首期只返回 Run/Stage/ResultSummary 聚合，不提供 Stage Output 内容。完整报告/结果资源的唯一 Owner、授权读取 Port 和 WebUI 正式读取契约尚未确定。
- 旧任务类型审计：当前 UI 开放盘后扫描/自选股扫描，持仓复核禁用，历史回放未出现在当前表单；盘后扫描与自选股扫描目前共用同一 Runner 主流程，没有独立计算阶段分叉。需先确认产品语义，再决定模板数量。
- 认证边界：Researchflow 路由只允许 Owner Session；不为旧 Analysis API 添加 Session 兼容，不保留新旧双认证。
- 本轮只提交迁移盘点文档，没有修改生产代码或删除旧接口。后续依次完成业务语义确认、领域 Owner Facade/生产 Stage、结果读取契约、WebUI Run/Stage 页面迁移、旧调用方清零和旧路由退役。
- 当前 HEAD 和 Actions 以 GitHub 实时状态为准；本轮文档提交不冒充代码验收或完整 CI 通过。


## 2026-11-24 RA-02 SMA20 生产接入现状复核

- 复核起点：`repair_all` SHA `25f66ae30470a76a734afffe974247f0401fd4ba`；本记录只描述该代码基线的事实，不代表已完成生产接入。
- 代码已具备 Data Close Series Snapshot 创建/授权读取、Quant `CreateSMAService` 精确 SMA20 计算与持久化、Quant `SMA20Adapter` Stage 级输入输出封装。旧文档中“没有计算 Facade”的结论已过时，详见 `Quant模块卡点.md` 的 SMA20 状态校准。
- 仍未闭环的生产路径是：Server 可信 Worker Principal → Data Snapshot Creator（按当前 Data Policy 签发授权）→ Quant CreateSMAService（活动 Registry + MariaDB Repository + Clock）→ SMA20 Stage Adapter → Researchflow Executor/Output Registry/Template → Worker Attempt 原子提交。
- 当前 Server Composition/Runtime 仍仅装配 News Search。未核实并完成可信 Principal 与 Data 策略主体映射前，不注入 SMA20；未定义正式 Stage Payload/QualitySummary Schema、参数 Schema、Template 和结果关联契约前，不注册占位 Stage。
- 下一步按真实依赖推进：先核对 Data/Platform Service Principal 解析与 Data Policy 当前允许主体；再核对 Quant 生产 Registry/Repository/Clock 的实际装配条件及 Researchflow Executor 的多 Stage 分发机制；据此一次性完成 Server 组合、Stage 注册、MariaDB 端到端测试和模块文档。若身份/策略或 Run→result_id 关联契约仍缺失，记录为明确阻塞，不绕过授权或临时扩展公共契约。
- 当前 SHA 的 Combined Status 返回空状态集合，按 SHA 查询的 PR workflow runs 也未返回记录；因此本次不标记 CI 通过。后续必须对实际代码提交 SHA 核实 Go Test Suite、Integration CI 和相关 Quant/Data/Server 专项工作流。



## 2026-11-24 RA-02A Quant Data 消费的最小权限服务身份

- Platform Security 增加由显式 ServiceCredential 配置构造受限 Principal 的能力；Server 将服务凭据加载与 Owner HTTP Token 分离。
- 复核后修正权限范围：Data 访问身份属于 Quant，不是整个 Researchflow Worker。Server 只在 `quant.sma20` Stage 执行边界注入 `quant` Principal；News 等非 Data Stage 不继承 `data:read`。
- Quant 凭据至少具备 `data:read`。Data Policy 必须显式允许 `principal:service:quant`、用途 `quant_analysis` 和所需 Scope；无策略授权时 Snapshot 创建/读取仍拒绝。
- 当前 Quant Stage 尚未注册，因此 Quant 凭据可选；Quant Stage 加入生产 Template 时，Server 组合根必须校验凭据存在、Principal 有效且策略授权配置可用，再允许启动。
- CI 首轮暴露 Go 格式问题、SMA20 SampleCount 类型错误、Runtime 测试调用签名未更新和身份测试错误断言，均已根据真实失败日志修正。
- `eb6c3d5e647d16a7bd21584a6a5d83ca683698f0` 的 Go Test Suite 与 Integration CI 均通过：Go Test `45389404306`，Integration `45389404103`。通过范围包含 root tests/race/vet、模块专项测试、Researchflow/Market Data/Research/Notification MariaDB 集成、Compose 校验和 Docker build。
- 该结果只验收当前身份边界工作，不代表 SMA20 Stage 已生产注册或旧 Analysis 已退役。
- 本阶段只完成服务身份前置边界，不代表 SMA20 生产 Stage 已接入，也不代表旧 Analysis 可以删除。后续继续 Data Snapshot 适配器、Quant Reader/Facade 装配、Stage Executor 多路分发、Output Registry 与正式 Template。


## 2026-11-24 RA-02B Data → Quant SMA20 → Researchflow 首条生产 Stage 链

- 当前实现已从身份准备推进到首个 Stage 注册：Server 组合根按 Data Provider inventory 装配 Quant SMA20 Stage、Stage Executor Dispatcher、合并 Output Registry 和 quant.sma20@1.0.0 Template。
- Data Snapshot Adapter 使用 Run SystemAsOf 作为 PIT cutoff，并把 ResourcePolicy Provider allowlist 传入 Data；Data 查询/Repository 按 allowlist 过滤来源。Quant 只消费 Data Snapshot Creator/Authorized Reader，不访问 Data Repository。
- principal:service:quant 仅注入 quant.sma20 Stage；data:read 缺失时 Worker Runtime fail closed。Data Owner Policy 仍是独立授权事实，不因凭据存在而自动 Allow。
- 已有 Stage Executor 单测覆盖 Provider Policy、Data 授权拒绝、参数 Schema 和 Template 合同；Data Provider Filter 有对应单测/MariaDB测试。当前尚无完整 Worker→Data Snapshot→Quant Repository→Stage Output 的真实跨模块 MariaDB E2E。
- 本阶段状态：生产装配代码已接入，尚未完成完整验收。当前 SHA 的 Go Test Suite / Integration CI 尚无可确认的 completed/success 结果；不得用前序 SHA 的 CI 通过记录替代。
- 下一步优先完成 Run→Quant result_id 关联只读 Port 和 Quant AuthorizedResultReader，在同一可证明一致性边界内校验关联、当前策略版本并读取不可变结果；随后接 Research Context/Evidence、LLM、RiskDecision、完整报告结果和 WebUI。
- 旧 Analysis Runner/Task Queue/API、Python Bridge、Python API Client、go-core 继续保留到新模板和 WebUI 全部接通后，再按 RA-08/RA-09 一次性删除。


## RA-02B 后续：Worker 服务凭据与 Runtime 装配测试

- Quant Stage 已注册后，Worker/All 且实际装配 Eastmoney DailyBars Provider 时，必须显式提供 `quant` ServiceCredential 与 `data:read` Scope；不提供凭据时保持 fail closed。Data Owner Policy 的 Allow 仍是单独前置条件。
- Worker MariaDB/Redis Bootstrap 集成测试现在显式配置独立 Quant Token；News Search Runtime 集成测试现在通过生产所需的 Stage Executor 与 Output Registry Set 装配运行。
- `.env.example` 已记录该配置方式。此配置不意味着生产部署已自动获得 Data 授权。
- 本工作组只修复服务身份和测试组合完整性，不宣称 Quant 结果读取闭环或 RA-02 完成。下一步继续 Researchflow Run→Quant result_id 关联只读 Port 与 Quant AuthorizedResultReader。


## 2026-11-25 RA-02C：Quant 授权结果读取闭环基础

- Researchflow GetStageOutputForRunStage 已提供只读关联事实；Server 固定读取 quant.sma20 Stage，并要求 Payload/QualitySummary 两份实际内容通过 Quant Schema Validator，校验 Run/Stage/Attempt 与 result_id 的精确关系。
- Quant MySQLAuthorizedResultReader 在当前策略 Head 读取锁内验证策略启用状态/版本、Run 关联和不可变结果记录；随后生成 GOV-CTR-006 AuthorizationRef + PolicySnapshot 指纹。读取失败、撤销或关联缺失不释放结果。
- Server 组合根构造 Quant GetResult 与 AccessAuditor，并通过仅在显式读取时附加 Worker 身份的内部 Facade 提供给后续 Research Stage。
- 已新增 Reader 单测、关联适配器测试、双 Schema 内容校验测试和 MariaDB GetRecord/授权读取集成测试。当前 SHA 的完整 CI 仍需等待本提交结果。
- RA-02 尚未最终完成：Research Context/Evidence Stage 还未消费该 Facade；必须继续完成 Research Stage 接入和真实 Worker→Data→Quant→StageOutput→GetResult→AccessAudit 跨模块 E2E。
- 当前没有删除旧 Analysis/Python Bridge/Python API Client；继续按 RA-08/RA-09 门禁，等新模板、结果消费和 WebUI 全面迁移完成后统一删除。



## 2026-11-25 RA-02C 进度复核与旧代码退役距离

- 最新核查基线：`repair_all` SHA `feb1d4f79ab72b7b6664d309a9b3dc886ae7fd17`。本阶段已完成 Quant SMA20 Stage 的 Server 条件装配、Data PIT Snapshot 消费、Quant 结果持久化、Researchflow Stage Output 关联读取、当前策略授权复核与 AccessAudit 组合装配。
- 当前 SHA 的 Go Test Suite Run `45532181367` 和 Integration CI Run `45532181223` 均 completed/success。该结果不代表真实数据准确性验收，也不代表全量股票研究流程完成。
- 新执行链目前有 News Search 与 Quant SMA20 Stage；Research Context/Evidence/Claim 尚未作为 Researchflow Stage 消费 Quant GetResult，LLM 尚无 Server 生产 Provider/Stage，RiskDecision 尚未接入完整流程，完整报告结果读取资源/API 尚未提供。
- WebUI 仍有明确旧调用：`web/src/api.js` 保留 `/api/v1/analysis/tasks`、旧 Run/Event API；`web/src/app.js` 仍以 Task ID、Event Cursor、analysisBase 和旧 RunResult 状态驱动任务页、历史与报告关联。虽然已有 Researchflow API helper，尚未替换主业务流。
- Root Server 仍装配旧 Analysis Runner/Task Queue/Worker/HTTP Handler 和旧路由；Python API Client、Python Bridge、go-core 也仍在仓库。它们不能一起直接删除，否则当前旧业务入口和部分 UI 会失效。
- 距离一次性删除旧代码，必须先完成：①完整股票研究 Template 与领域 Stage；②Research/LLM/Risk/结果持久化和授权读取闭环；③Researchflow 取消/轮询/历史/结果正文/导出 API；④WebUI 全面迁移并清零旧 API 调用；⑤旧 Analysis 业务语义迁移/明确废弃清单；⑥全仓配置、Migration、Docker、CI、文档及外部调用方核查。之后再在一个逻辑工作组删除旧入口及全部专属资产，不保留兼容链。
- 下一工作组继续 RA-03：Research Context/Evidence 生产 Stage，消费 Server 注入的 News Evidence Importer 与 Quant Authorized GetResult；不在 Researchflow 复制领域规则，不让 Stage 访问 Repository，不绕过来源授权和 PIT 校验。


## RA-03 启动前代码级依赖复核：News 持久化是 Research Evidence 的前置条件

- 复核基线：`repair_all` SHA `7ff603232cdc2a47ce5a9c5d8ff5914ce34a37d6`。
- 当前 `news.search_candidates` 只调用 `news/app.SearchCandidates` 并返回 Provider 候选；该用例明确不持久化、不缓存。Stage Payload 的候选字段是 Provider/Source Item ID、标题、URL 和时间，不包含已持久化的 NewsID/VersionID。
- Research News Importer 的输入却要求可信 Caller、Purpose、SystemAsOf、NewsID 和 VersionID；它通过 News Evidence Query 读取并重新校验当前授权。因此不能把 SearchCandidates 的 Provider 候选直接映射成 Research Evidence，也不能跳过 News 授权/持久化。
- News 已有 `IngestProviderCandidate` Application Use Case，但它要求可信 SourceCatalog、ProviderOutputPolicyResolver、Source/Endpoint/Provider 策略版本及事务内复核。当前 Server 的 `newResearchNewsComposition` 只装配 Evidence Query/Importer/Authorizer 与 Research Repository，没有装配 Provider Candidate Ingest；Researchflow Composition 也没有 Research Stage 消费者。
- 因此 RA-03 的正确先后顺序修正为：①在 Server 组合根装配可信 SourceCatalog 与 Provider Output Policy Catalog；②新增 News Owner 的持久化 Ingest Stage（消费 SearchCandidates，按策略写 Observation/News identity，输出持久化 NewsID/VersionID）；③Research Stage 通过 News Evidence Importer 重新读取和授权 Evidence，并消费 Quant AuthorizedResultReader；④持久化 Research Context/Evidence/Claim，完成同库 MariaDB Worker E2E。
- 不得把 Provider Summary 直接当来源正文，不得用 Search Candidate 的 provider_item_id 冒充 NewsID，不得在 Research 中复制 News Repository 或 Ingest 策略。没有合法 Source/Endpoint/Provider Policy 的候选必须 skipped/denied 并保留诊断，不能为了跑通流程写入。
- 现有 `researchNewsComposition` 在 Bootstrap 中晚于 Researchflow Execution Composition 创建，后续装配时需调整依赖构造顺序或采用明确的依赖结构，避免循环依赖；Server 仍是唯一组合根。


## 2026-11-25 RA-02C Quant 授权结果读取阶段验收

- 验收代码基线：`repair_all` SHA `b99eec279ae95f59a806d90d95baed81c2ff1601`。
- 已完成 Quant MySQL AuthorizedResultReader、Researchflow Run/Stage/Attempt → SMA20 result_id 精确关联校验、Payload 与 QualitySummary 双 Schema 内容校验、当前 ResultAccessPolicy Head 锁内授权复核、不可变结果读取和 AccessAudit 持久化。
- Server 已装配 Quant GetResult、Researchflow StageOutputReader、SMA20 ResultAssociationVerifier 和内部 Worker 读取 Facade；Worker 身份只在显式 Quant 结果读取时附加。
- Go Test Suite Run [45532181367](https://github.com/Kasign/stock_data_provider/actions/runs/45532181367)：Researchflow/Research/Market Data 模块格式、测试、Race、Vet，Research/Market Data/News/Notification/Researchflow MariaDB 集成、根模块 Test、go-core Test、Compose 校验和 Docker build 均 success。
- Integration CI Run [45532181223](https://github.com/Kasign/stock_data_provider/actions/runs/45532181223)：Full integration checks success。以上两个 Run 的 head SHA 均与本节验收 SHA 一致。
- **阶段结论：RA-02C 授权结果读取能力已接入并通过当前 SHA CI；RA-02 整条研究消费链仍未完成。** Research Stage 尚未实际调用该 Facade，也没有 Worker→Data Snapshot→Quant Result→StageOutput→Research GetResult→AccessAudit 的完整业务 E2E。
- RA-03 不得直接把 News Search Candidates 当作 Research Evidence：`internal/news/app/search.go` 明确 SearchCandidates 不持久化；Research Evidence Importer 依赖 News 已持久化的精确版本、available_at 和当前授权复核。Research 模块当前将真实 Evidence 导入标记为 SHELVED，原因是 available_at 来源证据及授权/精确版本读取一致性尚未满足。下一步先补齐 News 受控 Provider Candidate Ingest 的真实生产接入与 PIT 可得性证据，再接 Research Evidence/Context Stage；不伪造时间或把候选直接升格为 Evidence。
- 旧 Analysis、Python Bridge、Python API Client、go-core 仍保留；本阶段不提前删除。继续完成 Research/LLM/Risk/结果聚合与 WebUI 迁移后，按 RA-08/RA-09 一次性退役。


## RA-03 News → Research 证据链当前状态（2026-11-26，NEWS-POST-08 已完成）

### 已完成的自动采集生产切片

- 本轮文档校准前的 `repair_all` HEAD：`cda3f9dca9c71b9c0c7a88b619e5d9f9d907d8a8`；该提交仅更新 News 任务验收文档。News 最终生产代码验收 SHA：`dc5832faed4a0643b7e59cdedcef8eef8d0c5b5a`。本节随后更新了跨模块任务状态，不改变生产代码。
- `NEWS-POST-08` 已完成：Server 通过共享 Platform HTTP Client、News MySQL Repository、可信 StaticSourceCatalog、CollectSource、IngestCollectedSource 装配 `news.collect_feeds`；Researchflow 注册 Feed Template、Dispatcher、Output Registry 和 Source/Endpoint ResourcePolicy。
- Feed 正式调用链为 `news.collect_feeds` → `CollectSource.Execute` → `IngestCollectedSource.Execute` → `MySQLRepository`。StageOutput 仅保存来源标识、NewsID 和计数，不保存文章正文或 URL。
- Go Test Suite Run `45744051927` 与 Integration CI Run `45744051952` 均对最终代码 SHA `dc5832faed4a0643b7e59cdedcef8eef8d0c5b5a` 成功；覆盖 Feed MariaDB 和 Server/Researchflow 组合 MariaDB 测试。
- 本次完成的是“已创建 Researchflow Run 后自动执行 Feed 采集”；Researchflow 尚不会按周期自动创建 Run。代码/CI 通过不等于目标部署已配置真实 Feed URL 或完成真实外网来源验收。

### 当前代码事实与未完成边界

- `news.search_candidates` 只执行 `SearchCandidates` 并产出 Researchflow StageOutput，不自动持久化。当前 Server Dispatcher 注册 News Search、News Feed、Quant SMA20；没有 Provider Candidate 持久化 Stage，也没有 Research Stage。
- News 已有 `app.IngestProviderCandidate`，但它依赖可信 `SourceCatalog` 和 `ProviderOutputPolicyResolver`。当前 Server 的 Research News Composition 只装配 Evidence Query、Authorizer、Importer 和 Research Service；Researchflow Composition 未装配 Provider Candidate Ingest，也没有真实消费者。
- `IngestProviderCandidate` 通过共享 `candidateIngest.executeCore` 写入 Observation，并在内容指纹变化时追加 NewsItemVersion；当前调用传入空 `ContentLayers`，不能把 ProviderSummary 当作发布者正文或来源摘要。旧记录中“该用例不创建 NewsItemVersion”的说法与当前代码不符，已在本节纠正。
- Feed 采集与统一正式入库不自动生成 `AvailabilityEvidence` 或 verified `AvailabilityDecision`。News API 契约仍规定：RSS `pubDate`、Atom `published/updated`、普通发布时间、HTTP Date/Last-Modified、fetched_at 和 observed_at 均不能推导首次公开可获取时间。
- 当前没有可核验的来源级“首次公开可获取时间”字段语义、字段路径、版本绑定和原始证据。缺少这些证据时 `available_at` 必须为 null；News 严格 PIT 查询和 Research Importer 必须拒绝该记录。Researchflow ResultComposition 已对进入最终结果的 Evidence 执行 strict PIT，不增加非严格模式绕过。
- Storage 层已有 Availability Evidence/Decision 的持久化与一致性门禁；Application 层审核入口和生产调用链仍未形成完整闭环。不能将 Storage 方法存在等同于 verified Decision 的生产生成能力。

### RA-03 后续执行顺序

1. 先确定一个真实发布者 Source/Endpoint，并核实其访问方式、个人研究用途、内容层级、留存范围和可信配置；没有真实 URL/配置时，不把测试 Feed Fixture 当作生产来源。
2. 针对该来源查验官方字段语义。只有来源明确声明首次公开可获取时间、字段可绑定到精确内容版本且原始证据可审计时，才接入 AvailabilityEvidence 解析与记录；不得把 RSS/Atom 常规发布时间映射为 available_at。
3. 补齐 AvailabilityEvidence 记录、来源语义审核/Decision、版本绑定和当前策略复核的真实 Application→Storage 调用链，并以 MariaDB 验证 verified、revoked、过期、来源证据缺失和版本不匹配。
4. 只有当 News Evidence Query 能在指定 SystemAsOf 返回真实合格的 NewsID/VersionID、当前授权和 available_at 后，才接 Research Context/Evidence Stage；Stage 复用 Server 注入的 News Importer 与 Quant Authorized GetResult，Researchflow 不访问 News/Research/Quant 私有 Repository。
5. 最后完成同一 MariaDB 的 Worker→Feed/News Version→AvailabilityEvidence→News Evidence Query→Research Importer→Research Evidence/Context→Quant GetResult→AccessAudit 跨模块 E2E，再执行精确 SHA 的 Go Test Suite 与 Integration CI。

### 明确不做

- 不开发人工导入 UI/API/表或独立写入路径。
- 不把 Provider SearchCandidates 全量自动写入 News；在候选选择语义、可信发布者 Source/Endpoint 映射和 ProviderOutputPolicy 生产目录没有明确调用契约前，不创建猜测性 Stage。
- 不创建空壳 Research Stage，不降低 strict PIT，不用 ProviderSummary 冒充来源正文，不伪造 available_at。
- 不重复实现已完成的 Feed 收集、统一入库、News Schema、Researchflow StageOutput 通用存储和 Quant 授权读取能力。

## 2026-11-25 RA-04：独立 Go Core / Python Bridge 旧运行链退役

- 核查基线：`repair_all` SHA `79c1620469579ecb0fbdb9a75a637eb907a08394`；开始修改前分支已出现并保留并发文档提交 `ee5f4e7638308d6a8a4f9dfbb826b1bae2f30671`，本次以其为父提交重新应用变更。
- Root Server 组合根没有导入 Go Core/Python Bridge；根 Compose 与 `scripts/deploy-go.sh` 均不部署二者。旧 Go Core 仅由 `go-core/cmd/analysisd/main.go` 启动，并强制依赖 `PYTHON_BRIDGE_TOKEN`；其五类 Adapter 均经 Python Bridge 内部 API 调用。
- Python Bridge 的内部调用方只有该 Go Core 运行链；反向 `python_bridge/go_core_client.py` 在仓库内无业务调用方且默认关闭。Bridge Dockerfile 还引用当前仓库不存在的根 `requirements.txt`。
- 本组删除 Go Core、Python Bridge、专属内部 OpenAPI 契约及失效的专用迁移验证脚本；同步清理 Go Test Suite / Integration CI 中 go-core 专项矩阵、测试、vet 和汇总项。根模块及其他模块验收保持原标准。
- RA-04 首批提交时保留了 `internal/adapters/pythonapi/**` 与 `cmd/pythonapi-smoke/**`，因为当时尚未完成其调用方复核；随后 MIG-014 通过 Root Server 实际装配链确认 `NewRunnerWithStores` 无生产调用方，已将 Python API Adapter、Smoke 命令和死构造入口单独退役。Root Server 旧 Analysis API/Task Queue/Worker 和 WebUI 旧 API 调用仍保留。
- 文档补充：已从 `doc/api/contracts/数据层API清单.md` 移除已退役的 Python Bridge OpenAPI 权威契约引用，明确该内部契约不属于当前 API。Go Test Suite 与 Integration CI 的路径过滤增加 `doc/api/contracts/**`，使后续契约文档变更触发完整验收。
- 代码验收：Go Test Suite Run [45535407664](https://github.com/Kasign/stock_data_provider/actions/runs/45535407664) 与 Integration CI Run [45535407708](https://github.com/Kasign/stock_data_provider/actions/runs/45535407708) 均对精确 SHA `1d82626c1501858736b98cc1525dfb29cccd5f03` completed/success。文档修正后的 SHA `6856e9add4c6f65a53f0fdd89b7e3fa480443f6d` 的 Go Test Suite Run [45536015272](https://github.com/Kasign/stock_data_provider/actions/runs/45536015272) 与 Integration CI Run [45536015267](https://github.com/Kasign/stock_data_provider/actions/runs/45536015267) 也均 completed/success。
- 最终 CI 复核发现 Go Test Suite 原有旧 Go 模块路径扫描曾随 go-core 专项删除而一并移除；该扫描并非 go-core 模块构建检查，已恢复为独立 guard（排除自身工作流文件，避免自匹配），仅移除已退役模块的存在性断言。Integration CI 增加对 Go Test Suite 工作流文件变更的触发，以确保两套 CI 同步验收。
- Go Core/Python Bridge 退役最终代码/CI SHA `9a531802eb58c18dd34b91a1d44fbea71ee0aa05` 的 Go Test Suite Run [45536603141](https://github.com/Kasign/stock_data_provider/actions/runs/45536603141) 与 Integration CI Run [45536603138](https://github.com/Kasign/stock_data_provider/actions/runs/45536603138) 均 completed/success。
- MIG-014 首次删除 Python API Adapter 后，SHA `2daa657988834ec6f639cb326527bc6c87d6fe47` 的 Go Test Suite 在 `Check gofmt` 失败：`internal/service/wiring_test.go` 第 45、113 行出现语法解析错误；Integration CI 汇总随根模块检查失败。没有放宽测试或跳过检查，随后在 `b2db192acdc27a0e2108ba30798c381adc49ebef` 修复测试，改为注入 DataCollector 验证 Runner 行为。
- **MIG-014 最终完整验收通过：** Go Test Suite Run [45538475246](https://github.com/Kasign/stock_data_provider/actions/runs/45538475246) 与 Integration CI Run [45538475107](https://github.com/Kasign/stock_data_provider/actions/runs/45538475107) 均对精确 SHA `b2db192acdc27a0e2108ba30798c381adc49ebef` completed/success。根模块 gofmt/Test/Race/Vet、Market Data/Research/Researchflow/News/Notification 测试、MariaDB 专项、Compose/Docker 构建，以及 Integration CI 的 Quant MariaDB、跨模块测试、Worker Bootstrap MariaDB/Redis、WebUI 语法/单测/镜像构建全部通过。
- 当前结论：独立 Go Core/Python Bridge、Python API Adapter/Smoke 与未使用构造入口均已退役；旧 Analysis Runner/Task Queue/Worker/HTTP API 和 WebUI 旧调用链仍按 RA-08/RA-09 保留。


## 2026-11-25 RA-04 后续：MIG-014 Python API Client 独立旧链清理

- 开始基线：`repair_all` SHA `9a531802eb58c18dd34b91a1d44fbea71ee0aa05`。
- 当前 Root Server 的实际装配使用 Data 模块 Application Port：`newMarketDataModule` → `servermarketdata.NewAnalysisCollector` → `service.NewRunnerWithDataCollector`。因此生产请求链不再需要 Python API Client。
- `internal/service/wiring.go` 的 `NewRunnerWithStores` 是旧 Python API Client/Collector 的构造入口，但没有 Root Server 生产调用方；`cmd/pythonapi-smoke` 是独立旧 API 探测工具。
- 本工作组删除 Python API Adapter、Smoke 命令和死构造入口；Runner 的测试改为注入 DataCollector，继续验证质量门禁、候选信号、不可交易硬门禁和持久化，不再依赖旧 Python HTTP 服务。
- Root Server 旧 Analysis Runner/Task Queue/Worker/HTTP API 仍存在，但其行情数据已从 Python API 切换为 Data Application Port。本次不提前删除这些业务入口，也不修改 WebUI。
- 后续按 RA-03 完成 News 来源采集、内容版本与 AvailabilityEvidence 生产闭环，再接 Research Stage；完整研究 Template、结果消费/API、WebUI 迁移完成后，才按 RA-08/RA-09 统一退役旧 Analysis 链。
- **MIG-014 已验收关闭：** Go Test Suite Run [45538475246](https://github.com/Kasign/stock_data_provider/actions/runs/45538475246) 与 Integration CI Run [45538475107](https://github.com/Kasign/stock_data_provider/actions/runs/45538475107) 均对最终实现 SHA `b2db192acdc27a0e2108ba30798c381adc49ebef` completed/success；失败根因及修复记录见上方 RA-04 验收记录。


## 2026-11-25 RA-03 契约门禁 Review 校准与来源溯源修复

- Review 基线：`repair_all` SHA `ad71e731c4c4b7d328a1acbe5496f73e12b16704`。
- 修正上一轮 Review 的一个判断：News Query 返回 `available_at = null` 并不等于严格 PIT 放行漏洞。News DTO 是通用只读查询结果，允许来源可得时间未知；Research Importer 的 `mapNewsEvidence` 已拒绝 null、非 UTC、精度不符或晚于 `system_as_of` 的时间，Research Domain 的 `ValidateForStrictPIT` 再次拒绝缺失 available_at。故本轮不在 News 通用查询端强制非空，不增加未经跨模块冻结的宽松 PIT Mode。未来若确有非严格研究消费场景，应先定义显式 PIT Mode，并贯穿 Template、Run、Stage、Evidence 和 Fingerprint。
- 发现真实缺陷：`AppendAvailabilityDecision` 在选中 Evidence 时仅验证 parsed 候选、原始值、保留状态与时间顺序，没有要求可追溯来源凭据；Evidence 的 `evidence_ref/evidence_hash` 两者均空时仍可被标记 verified。查询 SQL 也没有独立过滤缺少溯源凭据的历史/异常记录。
- 修复范围：Storage 在同一版本行锁事务内读取选中 Evidence 的 `evidence_ref/evidence_hash`，至少要求一个非空且合法；Evidence Query 的 available_at 子查询同步要求非空来源引用或 64 位小写 SHA-256。Integration Test 覆盖缺失溯源时查询不可见、Storage 拒绝 verified，补齐来源引用后才可见/追加。
- 这项门禁只证明时间候选有可追溯的来源记录，不证明字段语义等同“首次公开可获取时间”。来源字段语义、许可范围、版本绑定、审核规则仍必须逐来源确认；不把 RSS pubDate、published/updated、fetched_at 或 observed_at 自动认定为 available_at。
- Application 层 AvailabilityDecision Review Use Case 当前仍没有真实生产调用方，也没有统一的来源字段语义审核规则及其 Owner。暂不新增空壳接口；待真实来源/调用场景确定后，由 News Application 提供审核入口，Storage 继续负责并发序列化、版本归属与事务完整性。



## 2026-11-25 RA-03 严格 PIT 查询层 fail-closed 修复

- 开始基线：`6fb64ae1322267ea41e898ef8fcc502f09d1c32f`。
- 按 `doc/news/新闻API契约.md` 第 11.3 节，ResearchEvidenceQuery 的当前契约是严格 PIT；`available_at` nullable 是 DTO 空值表达，不是查询放行规则。缺失/unknown 必须从 Get/List 两种查询结果中排除。
- 修复 News Storage Get/List 查询：在标量 available_at 投影后用 `HAVING available_at IS NOT NULL` 过滤，避免无可用时间记录进入单条结果或固定分页 Snapshot；Application `validEvidencePITRecord` 同时将 nil 作为严格 PIT 无效。
- 回归覆盖：单条查询和列表查询均覆盖未有 verified Decision、缺少 Evidence 溯源、unresolved 最新 Decision、异常 selected_evidence_id、Evidence payload purge；无合格时间时不返回记录。
- 保留上一组来源溯源门禁：verified Decision 必须引用可追溯 Evidence；但不把 evidence_ref/hash 存在等同于首次公开可获取语义已验证。没有来源官方字段语义/许可证据时，Research B 组继续 SHELVED，不构造 Research Stage 空壳。



### 严格 PIT 修复首轮 CI 反馈与测试契约校准（2026-11-25）

- 严格 PIT 实现 SHA：`8d61f54e542b912de814b80b45313267d9befa80`。
- Go Test Suite Run [45543207564](https://github.com/Kasign/stock_data_provider/actions/runs/45543207564) 的 News MariaDB Job 首轮失败，根因不是 SQL 语法，而是两条旧集成测试仍把缺少/已过期 `available_at` 的记录作为 PIT 可查询结果：`TestNewsMariaDBRepositoryIntegration` 未创建任何 AvailabilityDecision；`TestObservationRetentionPurgeIntegration` 的可用时间证据本身已过期并被清理，却仍断言 Tombstone 可进入严格 PIT。
- 修正测试语义：普通可查询版本集成用例显式创建绑定到精确 Version 的可追溯 verified Evidence/Decision；Availability Evidence 已过期并清理后，单条和列表 PIT 均必须返回不可见，不再把 Tombstone 存在误当成严格 PIT 合格。
- News MariaDB workflow 的测试筛选此前未运行 `TestNewsAvailabilityPITResolverMariaDBIntegration` 与 `TestAppendAvailabilityDecisionSerializesConcurrentWritersMariaDBIntegration`；现将两项纳入明确的 MariaDB Job，确保本轮新增查询门禁和上一组 Decision 写入门禁真正执行。
- 首轮 CI 中 Research/Researchflow 模块检查、Researchflow WorkerStore、Compose/Docker Job 已通过；根模块和其他 MariaDB Job 当时仍在运行，整轮不记 PASS。后续以修正后的精确 SHA 重新跑完整 CI。



- 第二轮 CI 的 News MariaDB Job 再次暴露一处旧测试假设：历史版本具备 available_at，不代表后续新版本自动继承该证据。已在版本修订集成用例中为新 Version 单独建立 Observation 绑定的 verified Evidence/Decision，继续验证新版本只有在自己的来源时间证据合格后才进入最新 PIT 列表；未把旧版本时间证据跨版本复用。



### Availability Evidence 选中证据完整性门禁（2026-11-25）

- `AppendAvailabilityDecision` 之前只检查 parsed 状态、候选时间、原始值、时间顺序和来源引用/哈希；没有复用 `domain.AvailabilityEvidence.Validate()`，导致通过直接 Repository 写入的 verified Decision 仍可能选择 source_precision=year、缺少时区、缺少字段路径/解析器版本等不合格记录。
- Storage 现在读取完整的选中 Evidence，并通过 Domain Validate 校验精度、时区、原始值、字段路径、解析器版本、UTC/秒精度、时间顺序、保留期限及引用/哈希格式；同时确认 Evidence 的 news_id/version_id/observation_id 与被锁定 Version 完全一致。
- PIT SQL 独立执行相同关键约束，避免直接 SQL/历史异常 Decision 绕过 Application/Storage：只允许 second/minute、明确时区、非空且规范的 field_path/parser_version、空 parse_reason_code、时间顺序正确、秒精度候选及格式合法的来源凭据。
- Integration Test 覆盖 Storage 拒绝 year 精度/空时区，PIT Get/List 对相同异常记录 fail closed；没有增加自动来源语义判断。字段是否代表“首次公开可获取时间”仍必须由具体来源审核规则证明。



### News→Research 跨模块严格 PIT 集成测试适配（2026-11-25）

- Integration CI 在 SHA `23840cad6cd3f49baa5922ded6eb90e0d567e0e8` 发现真实组合根集成测试仍假设 News Query 会返回 `available_at = null` 的 DTO，导致其后续授权/Importer 用例无法进入。严格 PIT 修复后，Query 应在 Storage 层直接返回 `ErrEvidencePITNotFound`，这是预期行为，不是回归。
- 跨模块 MariaDB fixture 不再通过 Query 读取不存在严格 PIT 资格的记录来创建授权快照；改为用真实 Observation/Version 写入授权快照，但不伪造 available_at Evidence/Decision。
- A 组现在验证：无 verified available_at 时，News Query 在授权前拒绝、Research Importer 传播 PIT 不可见错误、Research Evidence 不落库、List Evidence Snapshot 不包含该记录。
- B 组成功导入仍明确 Skip，继续等待真实来源授权、来源字段语义和 verified available_at 生产链；未制造测试时间值冒充生产来源证据。
- Fingerprint 与当前策略版本变更的授权拒绝已有 `internal/news/app/authorization_read_test.go` 专项覆盖；本真库 fixture 没有合格 available_at，不再让 PIT 之前置门禁掩盖后置授权测试。


### 2026-11-25：Research strict PIT 消费门禁与机器契约同步

- Researchflow `resultvalidation.validateResearchEvidence` 之前允许 `available_at=null` 的 Evidence 进入最终组合；这与 Research Domain 已有的 `ValidateForStrictPIT` 语义及项目级严格 PIT 要求不一致。
- 当前不存在已冻结、已实现的非严格 PIT Template Mode，因此不新增模式字段或默认降级；ResultComposition 对所有 Evidence 一律执行 strict PIT：available_at 必须存在、为 UTC，且不晚于该 Evidence 自身的 system_as_of。Evidence 的 system_as_of 可以早于 Run，但不得晚于 Run。
- 新增缺失 available_at、available_at 晚于 Evidence cutoff、非 UTC available_at 的回归用例；有效测试数据显式提供可证明的时间。
- RSC-CTR-001 Schema 增加 RFC3339Nano UTC Z pattern，以及 source_owner=news 时 source_version 必填约束；nullable available_at 仍保留在 Research 存储 DTO 层，严格结果消费由 ResultComposition 门禁执行。
- NEWS-CTR-001 Page Schema 将已存在于 DTO/Golden 的 relation_status 加入 required；NEWS-CTR-001/002/003 与 RSC-CTR-001 登记簿说明同步修正；旧 News-Research 设计文档明确降为历史参考。


## 2026-11-25 Quant 物理 Schema 启动校验补齐

- 根据 2026-11-24 Schema 审计，Quant 原先只有 Migration History/Checksum 校验和 Repository 行为集成测试，没有 Root 启动阶段的物理结构校验。
- 新增 Quant 自有只读 storage.ValidateSchema：检查 5 张核心表、Repository/授权读取所需列，以及审计查询、策略快照唯一性和策略管理审计索引；不复制 DDL、不执行自动修复、不改变 Migration。
- Root bootstrapDatabase 在 Migration History 校验后、Redis/Repository 装配前执行 Quant Schema 校验；缺表、缺列或关键索引漂移均 fail-closed，并返回具体表/列/索引诊断。
- 新增结构快照单测覆盖缺表、缺列、缺索引及索引列顺序漂移；新增 MariaDB 基线集成测试验证当前真实 Migration 结构符合 Validator。
- 本工作组只补齐 Quant 启动前物理结构校验，不代表 RA-02 Research 消费链完成。Research Stage 尚未调用 Quant GetResult；News available_at 真实证据链仍按外部阻塞处理。


## 2026-11-25 News Schema readiness probe 复核

- News 已有只读 MissingSchemaTables 探测，并由 news.storage.schema 诊断项调用；此前 Schema 审计记录的“无检查”应精确理解为“没有 Root 启动级完整物理 Schema Validator”，不能忽略现存 readiness probe。
- 复核发现该诊断把 required_tables 固定写成 10，而 News Migration 与 requiredNewsSchemaTables 实际均定义 14 张表，导致健康诊断报告与真实检查范围不一致。
- 本轮将被检查表数量改为由 News Storage Probe 从同一张 required table 清单返回，避免再维护第二份硬编码数字；保持现有 MissingSchemaTables 的只读语义和 degraded readiness 行为，不新增平行 Schema 检查。


## 2026-11-25 Researchflow 物理 Schema 启动校验补齐

- 复核发现 Root 已在 Migration History/Checksum 校验后执行 MarketData、Analysis、Quant 的只读物理 Schema 校验，但 Researchflow Worker 核心持久化结构没有对应校验。Researchflow Migration 已应用并不代表人工 DDL 漂移可被发现。
- 新增 Researchflow Storage 自有 `ValidateSchema`，检查 9 张核心表的完整必需列，以及 Run/Stage/Attempt 唯一约束、Claim/Lease 扫描索引、Outbox Claim 索引和 Stage Output 关联索引；不复制 Migration DDL、不执行自动修复。
- Root `bootstrapDatabase` 在 Redis、HTTP Client 和 Repository 装配前执行该校验；缺表、缺列、索引缺失、唯一性变化或索引列顺序漂移均 fail-closed。
- 新增结构快照单测覆盖缺表、缺列、缺索引、唯一性和索引列顺序漂移；新增真实 MariaDB Migration 基线集成测试。
- 该校验只确认 Researchflow 物理结构符合当前 Worker/Run/Stage/Attempt/Outbox 访问契约，不代表 News 来源证据链、Research Stage 消费链或真实 Provider 数据质量验收完成。


## 2026-11-25 Research 物理 Schema 启动校验补齐

- Root 的 News→Research 组合在启动时创建 Research Context/Evidence MySQL Repository，但 Migration History/Checksum 通过后没有验证 Research 六张表的物理列和关键索引；人工 DDL 漂移可能在实际请求到达后才暴露。
- 新增 Research Storage 自有只读 `ValidateSchema`，覆盖 Aggregate Head、Context Version、Evidence Record、Event Version、Claim Version、Notification Outbox 六张表及其全部必需列和 PIT/幂等/状态扫描索引。
- Root `bootstrapDatabase` 在 Redis、HTTP Client 和 Repository 装配前执行 Research Schema 校验；缺表、缺列或关键索引漂移均 fail-closed，不执行自动修复。
- 新增结构快照单测覆盖缺表、缺 PIT 字段、缺 PIT 索引、唯一性和索引列顺序漂移；新增 MariaDB Migration 基线集成测试。
- 此校验确认 Research 存储契约结构，不解除 News 来源授权、available_at 生产证据和 Research 实际消费链的外部/跨模块阻塞。


## 2026-11-25 News Root 物理 Schema 启动校验补齐

- News Migration V1 定义 14 张表；现有 `MissingSchemaTables` 只检查表是否存在，属于 readiness probe，不能发现列缺失或索引漂移。
- 新增 News Storage 只读 `ValidateSchema`，检查全部 14 张表的必需列和 Migration V1 声明的全部索引（含主键、唯一键、普通索引的唯一性与列顺序）。
- Root `bootstrapDatabase` 在 Migration History/Checksum 校验后、其他业务 Repository/Redis/HTTP Client 装配前调用 News 校验；失败即停止启动，不自动修复、不兼容旧 Schema。
- 新增 News Schema 单测和真实 MariaDB Migration 基线集成测试；Go Test Suite 的 News MariaDB Job 显式运行该测试。
- 本工作组只确认 News 当前物理 Schema 符合 V1，不代表 RA-03 真实来源授权、来源字段语义审核或 verified `available_at` 生产链已完成。


## 2026-11-25 RA-03 个人研究用途调整与人工来源导入工作组

- 用户已明确项目为完全个人使用、非商业行为，且没有书面商业授权。由此撤销“没有商业授权即所有来源整体 BLOCKED_EXTERNAL”的统一判断；不要求用户先取得商业合同才能继续项目。
- 来源准入按具体发布者、访问方式、内容类型、用途和留存期限拆分。个人人工浏览/下载、服务端自动抓取、批量采集、长期全文留存、对外分发不是同一种行为，必须分别判断。
- 对公开条款允许个人非商业浏览/临时下载的来源，优先推进人工取得/导入；对明确限制程序化访问的来源不做自动采集；条款不明确的来源不伪造 approved 策略，也不阻断其他来源和人工路径。
- 已核对真实代码：CollectSource 当前仅支持 RSS/Atom 与 licensed_feed；IngestCollectedSource 要求真实 Collection Seal；SourceCatalog/Platform Endpoint Transport/当前策略 Resolver 已有模块实现，但 Server 尚未装配完整生产链。Domain 有 AccessManual，但缺少完整人工来源导入 Application 用例。
- 下一工作组定为 NEWS-POST-07：实现受控人工来源导入，复用现有 Source Policy、Identity/Observation、NewsItemVersion、Retention 和 AvailabilityEvidence 机制；不通过直接 Repository 写入、伪造 CollectionResult 或调用方自报 available_at 绕过门禁。
- 人工取得时间只作为系统观察/取得证据，不等于首次公开可获取时间。NEWS-CTR-001 Strict PIT 与 Research Importer 的严格校验保持不变；来源字段语义未审核前 available_at 仍为 unknown，Research B 组继续 SHELVED。
- 本轮只校准来源策略和任务边界，未宣称 Source 生产链、人工导入或 News→Research B 组已完成。

## 2026-11-26 Quant 三因子生产化第⑥组 Stage Provider 装配

- 起始 HEAD `0b8e42ed0`（第⑤组，已推送 `origin/repair_all`）。本组只做 Factor Stage Provider 与 Server 装配，不改契约版本。
- 新增 `internal/quant/stage/factor.go`：`FactorAdapter` 在可信 Run `SystemAsOf` 下创建授权 Data Close Series Snapshot，调用 `CreateFactorService`，把已落库 Manifest/Metric 投影为 `FactorOutput`/`FactorQualitySummary` 并自校验。`factor_code` 只作 Stage 层封闭分派，**不透传**给 Facade（其 parameters 契约仍为严格 `{}`）；逐因子最小样本下限（20/6/21）在 Adapter 先校验，参数 Schema 的 `limit` 下限取三者最小（6）仅作粗筛。
- 新增 `internal/server/bootstrap/quant_factor_stage.go`：`quant.factor` 的 Output Registry、冻结 Workflow Template、Stage Executor（`available`→succeeded、`insufficient_data`→degraded、Data 授权/参数/执行错误按固定 reason code 失败），并接入 `researchflow_execution_composition.go`、`researchflow_stage_dispatcher.go`、`researchflow_runtime.go`。
- 新增 `doc/researchflow/schema/quant-factor-parameters-1.0.0.schema.json` 与 Golden（`factor_code` 三值 enum、`limit` 6–1000），并在 `researchflow_template_prerequisites.go` 注册 Schema 文档、`quant.factor.always` 适用性规则与复合 dispatch。
- Output Registry 版本用 `1.3.0`：`NewStageOutputRegistrySet` 以 version 为唯一键，`1.0.0`/`1.2.0` 属 News、`1.1.0` 属 SMA20；新增测试 `TestQuantFactorOutputRegistryVersionDoesNotCollide` 守住该约束。
- 复用而非复制：`quantDataSnapshotAdapter` 同时实现 `DailyBarsCloseSeriesSnapshotCreator` 与 `CloseSeriesSnapshotReader`；Provider inventory 判定抽为 `quantDailyBarsProviderCodes`（SMA/Factor 共用），`NewStageOutputRegistrySet` 前合并去重；`QuantPurposeCode`/`QuantProviderCode` 作为通用别名，`SMA20PurposeCode`/`SMA20ProviderCode` 保留兼容。
- 验证：`go test ./internal/quant/... ./internal/server/...` 全包 PASS；覆盖三因子 Adapter 正例、insufficient 显式 null、非法请求不触达 Data、参数 Schema 文档一致性与 Golden、Executor 状态映射、Template 冻结断言、Dispatcher 路由与 Registry 版本唯一性。
- 第⑥组结束时未完成（Factor Run↔Result 关联已由第⑦组解决）：端到端真实质量证据与 Quant→Researchflow Contract Test 未做。QNT-CTR-003 仍 `deferred`。CI（`d574976ef`）：Go Test Suite [https://github.com/Kasign/stock_data_provider/actions/runs/45773733316](https://github.com/Kasign/stock_data_provider/actions/runs/45773733316)、Integration CI [https://github.com/Kasign/stock_data_provider/actions/runs/45773733461](https://github.com/Kasign/stock_data_provider/actions/runs/45773733461)、Quant module validation [https://github.com/Kasign/stock_data_provider/actions/runs/45773733036](https://github.com/Kasign/stock_data_provider/actions/runs/45773733036)、Required checks [https://github.com/Kasign/stock_data_provider/actions/runs/45773732918](https://github.com/Kasign/stock_data_provider/actions/runs/45773732918) 全部 success。

## 2026-11-26 Quant 三因子生产化第⑦组 Factor Run↔Result 关联

- 起始 HEAD `d574976ef`（第⑥组，已推送 `origin/repair_all`）。本组只做 Quant 结果关联校验与授权读取装配，不改契约版本。
- 问题证据：`internal/server/bootstrap/quant_sma20_result_association.go` 把 `StageCode` 硬编码为 `quant.sma20`，而 Researchflow 只提供按 `(runRef, stageCode, outputType, schemaVersion)` 精确读取、**没有 List 能力**（`stage_output_reader.go:34-58`），因此 Factor 已落库的 `result_id` 无法通过 `GetResult` 的关联校验。
- 方案：把校验泛化为已支持 Quant Stage 的封闭规格表（`internal/server/bootstrap/quant_result_association.go`，含 `quant.sma20` 与 `quant.factor`），并注入 Run Reader 做**前置枚举**而不是"按 stage 逐个试读"。这样能精确区分三种情况：①Run 无该 Stage 或未成功产出 → 无关联；②已成功但缺输出/质量引用、同码 Stage 重复、Run 引用不一致 → 完整性失败；③已成功但读取失败 → 直接上抛，**不降级为"无关联"**（原实现把 `ErrStageOutputUnavailable` 一律映射为 `NotFound`，会把存储故障伪装成"结果不存在"）。
- 装配修正：`researchflow_execution_composition.go` 原先只在 `quantStage != nil` 时创建授权读取链，仅装配 `quant.factor` 时会缺失；改为按实际存在的 Quant Stage 选择 Repository，任一存在即创建 `quantResultReader`。
- 新增测试 `internal/server/bootstrap/quant_result_association_test.go`（文件由 `quant_sma20_result_association_test.go` 重命名泛化）：SMA20/Factor 命中并断言 schema ref 与 Validator 契约、result_id 不匹配、Run 无 Quant Stage、Stage 未成功产出时不读取输出、成功后缺输出/质量引用、同码重复、Run 引用不一致、Run 无 Stage、读取失败与 GetRun 失败不被降级、未登记 Stage 被拒绝。
- 验证：`go test ./internal/server/bootstrap/ ./internal/quant/... ./internal/server/...` 全包 PASS。
- 未完成：端到端真实质量证据与 Quant→Researchflow Contract Test（下一组）。QNT-CTR-003 仍 `deferred`、QNT-CTR-004/005 仍 `frozen`。CI（`2022bf6a1`）：Go Test Suite [https://github.com/Kasign/stock_data_provider/actions/runs/45776105595](https://github.com/Kasign/stock_data_provider/actions/runs/45776105595)、Integration CI [https://github.com/Kasign/stock_data_provider/actions/runs/45776105620](https://github.com/Kasign/stock_data_provider/actions/runs/45776105620)、Quant module validation [https://github.com/Kasign/stock_data_provider/actions/runs/45776105619](https://github.com/Kasign/stock_data_provider/actions/runs/45776105619)、Required checks [https://github.com/Kasign/stock_data_provider/actions/runs/45776105613](https://github.com/Kasign/stock_data_provider/actions/runs/45776105613) 全部 success。

## 2026-11-26 Quant 三因子生产化第⑧组 端到端质量证据与跨模块契约

- 起始 HEAD `2022bf6a1`（第⑦组，已推送 `origin/repair_all`）。本组只补证据与契约测试，不改生产行为、不改契约版本。
- 问题证据：第①～⑦组的 Factor 断言都建立在 Quant 边界手写的 Snapshot/QualitySummary 之上，`factor_test.go` 的 `ExpectedTradingDays`/`MinimumRequiredBars` 由测试自行填写，因此"QualitySummary 是否真的来自 Data 覆盖率证据"从未被证伪；Quant 与 Researchflow 两侧的 Output Registry、关联规格表、文档化 Schema 也各自维护，缺少跨侧一致性断言。
- 端到端质量证据（`internal/server/bootstrap/quant_factor_end_to_end_test.go`）：只把 MariaDB Store 换成内存 Store，保留真实 Data 代码路径——`DailyBarsCloseSeriesSnapshotCreator`（真实 manifest/payload 双指纹、内容寻址 SnapshotRef）、`DataAuthorizationOwner`（真实策略判定与授权签发）、`AuthorizedDailyBarsCloseSeriesSnapshotReader`（真实授权复核），经生产 `quantDataSnapshotAdapter` → `FactorAdapter`（真实 `quant_analysis` 用途与 PIT/覆盖率门禁）→ `CreateFactorService` → 结果 Repository。断言：QualitySummary 的 `expected_trading_days`/`observed_bars`/`minimum_required_bars`/`coverage_sufficient`/`quality_status` 必须逐项等于**已持久化** Data Snapshot payload 的 coverage evidence（fixture 刻意让 `minimum_required_bars=18` 与 `observed_bars=20` 不同，改成 `observed_bars` 冒充即失败）；Factor 值 `0.013469673769` 由独立十进制计算得出，不复用被测算法；`result_id` 必须等于真实落库 Manifest 的 `result_id`，并可被 `quantResultAssociationVerifier` 按 Run/Stage 读到；insufficient 路径（区间只有 6 个交易日、Coverage 6/6 充足但算法窗口不足）显式 `value=null` 且仍以 `result_status=insufficient_data` 落库并持有 result_id。
- 跨模块契约测试（`internal/server/bootstrap/quant_researchflow_contract_test.go`）：①Output Registry Entry 与 `quantStageAssociationSpecs` 的 SchemaRef/Validator 必须一致（任一漂移即失败）；②`stageoutput.Registry.Validate` 必须接受 Quant Stage 真实产出的完整 Envelope；③同一 payload 在运行时 Validator 与文档化 Draft 2020-12 Schema 的判定必须一致（数值型 `value` 被两侧同时拒绝）；④模板 `OnDegraded=accept` 必须与 Executor 的 `insufficient_data`→degraded 映射一致，且 degraded 输出仍能被 Registry 接受；⑤`*marketdatabootstrap.Module` 必须实现 `quantDataSnapshotPort`（编译期断言）。
- 复用改造：把 SMA20/Factor 的 Output Registry 构建抽为 `newQuantSMA20OutputRegistry`/`newQuantFactorOutputRegistry`，组合根与契约测试共用同一构造函数，测试不再复制 Entry（原第⑥组测试里的重复 Entry 已删除）。
- 文档/注释纠正：`create_factor.go` 原注释称 insufficient 样本"不落库"，与实现及本组证据不符，已改为按 `result_status=insufficient_data` 落库。
- 验证：`go test ./internal/quant/... ./internal/server/... ./internal/researchflow/...` 全包 PASS；`go test -race ./internal/server/bootstrap/ ./internal/quant/stage/ ./internal/quant/application/` PASS；两条新测试均通过变异验证（改 `MinimumRequiredBars` 映射、改关联规格表 SchemaRef 后对应测试失败，恢复后通过）。
- 阶段结论：`旧Analysis退役执行清单.md` 的 1.2 三因子生产化在 Quant + Researchflow Stage 范围内闭环。QNT-CTR-003 仍 `deferred`、QNT-CTR-004/005 仍 `frozen`（本组未改 Schema/契约状态）；剩余生产门禁属阶段 2.1 完整股票研究 Template 与阶段 2.3 报告读取 DTO。CI（`8b3792e4d`）：Go Test Suite [https://github.com/Kasign/stock_data_provider/actions/runs/45778184833](https://github.com/Kasign/stock_data_provider/actions/runs/45778184833)、Integration CI [https://github.com/Kasign/stock_data_provider/actions/runs/45778184744](https://github.com/Kasign/stock_data_provider/actions/runs/45778184744)、Quant module validation [https://github.com/Kasign/stock_data_provider/actions/runs/45778184877](https://github.com/Kasign/stock_data_provider/actions/runs/45778184877)、Required checks [https://github.com/Kasign/stock_data_provider/actions/runs/45778184733](https://github.com/Kasign/stock_data_provider/actions/runs/45778184733) 全部 success。

## 2026-11-26 执行清单 1.3 trend-following 差距核实（书面结论，不补算法）

- 起始 HEAD `1201092fc`（第⑧组与契约口径修正已推送）。本组只做代码核实与结论，不新增代码、Schema、Golden、Registry Entry 或 Stage。
- 核实结论：旧 Runner 的 trend 逻辑是 `internal/service/wiring.go:258-305` 的 `trendFactor`（三因子：`trend.ma20_distance_pct`/`return_5d_pct`/`return_20d_pct`，percent、float64、最小样本 20/6/21）加 `wiring.go:307-351` 的 `trendStrategy`（`ma>0 && r5>0 && r20>0` 三正合取，只输出 `long_candidate`，`valid_until=as_of+24h`，entry/exit/invalidation 为中文自然语言串）。它没有 direction 四值、phase、strength、多周期冲突；`StrategyEligible` 缺失时静默 `continue`，导致"不可评估"与"无信号"不可区分；`AsOf` 取最后一根 bar 的 trade_date，无独立 `system_as_of`。
- 现有 `internal/quant/trend` 只有 `Result` DTO 与结构校验；`trend/1.0.0` Envelope 已注册可持久化；但没有算法、Registry Entry、Output/Quality Schema、参数 Schema、Template 与关联规格表条目。
- 不补算法的四条理由：①旧逻辑不覆盖 `TrendResult` 的 direction/phase/strength/conflict 字段，映射必然新增语义决策；②DEC-PLAT-002-049 禁止方向/阶段/强度互推且不定义阈值，`doc/quant/05` 第 50–57 行的 7 项语义至今未决；③单位 percent→ratio、时间 trade_date→system_as_of/trade_date 分离，旧中文规则串不可继承；④产物层次不同（趋势状态 ≠ 交易候选，QNT-008 Signal 仍 NOT_STARTED，塞进 Payload 等于私增 frozen 字段）。
- 交付：`doc/quant/07-trend-following策略差距与契约结论.md`——代码级事实、10 维差距矩阵、5 项决议请求（D1 周期集合 5/20 或 5/20/60，选后者须先补 `momentum_60d` Decimal Entry；D2 direction 纯符号；D3 phase v1 恒 `unknown`；D4 strength 恒 `null`；D5 先只做 TrendResult），以及决议通过后的最小可交付范围。
- 状态：1.3 退出条件"有生产策略 Stage 与契约"未满足，记为"书面结论完成、阻塞于 D1–D5"，不得记为已完成。契约状态未变（QNT-CTR-003 `deferred`、004/005 `frozen`），QNT-004 仍 BLOCKED_BY_DECISION、QNT-008 仍 NOT_STARTED。1.4 不依赖本结论，可并行。CI（`e14d6c0f6`）：Go Test Suite [https://github.com/Kasign/stock_data_provider/actions/runs/45779700146](https://github.com/Kasign/stock_data_provider/actions/runs/45779700146)、Integration CI [https://github.com/Kasign/stock_data_provider/actions/runs/45779700227](https://github.com/Kasign/stock_data_provider/actions/runs/45779700227)、Quant module validation [https://github.com/Kasign/stock_data_provider/actions/runs/45779700188](https://github.com/Kasign/stock_data_provider/actions/runs/45779700188)、Required checks [https://github.com/Kasign/stock_data_provider/actions/runs/45779700203](https://github.com/Kasign/stock_data_provider/actions/runs/45779700203) 全部 success；同次推送的前一个提交 `1201092fc` 未单独触发运行，不把 head 结论冒充该提交的独立 CI。

## 2026-11-26 CI 证据回填（第⑤～⑧组与 1.3）

- 问题：第⑤～⑧组与本轮 1.3 的进度记录一律写 `CI 状态 not_run`，但实际这些提交早已推送并触发 GitHub Actions；`not_run` 是"未等待中途 CI"的操作口径被误写成事实，与仓库可查的 Run 结论矛盾。
- 核实结果（`gh run list`）：`0b8e42ed0`（⑤）、`d574976ef`（⑥）、`2022bf6a1`（⑦）、`8b3792e4d`（⑧）、`e14d6c0f6`（1.3）的 Go Test Suite、Integration CI、Quant module validation、Required checks 均为 `completed/success`；`1201092fc` 与 `e14d6c0f6` 同次推送，仅 head commit 触发运行，因此 `1201092fc` 无独立 CI 结论，不把 head 结论冒充为其独立验收。
- 口径：已把各节 `CI 状态 not_run` 替换为对应 SHA 的 Run 链接；仍保留"本地测试不冒充 CI"与"文档 SHA 无独立 Run 时不冒用历史 CI"两条约束（AGENTS.md 第 14 条与第三节）。
- 未覆盖：`4cbd118ac`（第④组）未单独触发运行，其验收仍只有本地证据；本轮回填不改变任何契约状态、代码或测试。

## 2026-11-26 执行清单 1.3 落地 + 1.5 输入质量门禁（D1–D5 按推荐项通过）

- 起始 HEAD `cd126e877`（CI 证据回填已推送）。决议：D1=5/20/60、D2=纯符号、D3=phase 恒 `unknown`、D4=strength 恒 `null`、D5=只产出 TrendResult，均按 `doc/quant/07` 推荐项通过。
- 1.3 交付（`quant.trend` 成为真实生产 Stage，契约状态不变）：
  - 计算：`momentum_60d` 精确算法（`decimal_factor.go`）+ Registry Entry（61 根）。Entry 是趋势输入最小样本与覆盖率的唯一来源，Facade 断言 Entry 而不在代码里重复声明 scale/容差。
  - 语义：`internal/quant/trend/assess.go` 只做符号判定与质量分类（`valid`/`degraded`/`unknown`），拒绝空值、非 ratio、样本不足等 8 类非规范证据；不实现 phase/strength 推导，也不 import `registry`（registry 测试反向引用 trend 会形成测试期循环依赖，改用包内常量 + `TestTrendEvidenceCodesMatchRegistry` 收敛一致性）。
  - 用例：`CreateTrendService` 一次授权 Snapshot 读取产出四个证据 Metric（顺序即契约），`ValidateMetric` 对齐 `momentum_60d` Entry，经 `result.NewTrendPayload` + `resultPersister` 落库；Manifest `quality_status` 取判定值以满足 QNT-CTR-005，输入质量另以 `InputQualityStatus`/`result_quality_status` 表达。
  - Stage：Adapter 要求 `limit>=61` 且 `parameters` 严格 `{}`；Output/Quality Schema 1.0.0 + 强类型 DTO + Validator（复算方向、校验 phase/strength/conflict_refs/证据顺序、`unknown ⇔ insufficient_data`）；Output Registry `1.4.0`、冻结 Template、参数 Schema（无 `window` 字段）、`quant.trend.always`、Dispatcher 路由、Runtime 前置校验、关联规格表条目。
  - 生产缺口修正：Quant service Principal 注入从“仅 `quant.sma20`”泛化为 `isQuantStageCode`，否则 Factor/Trend 在生产 Worker 会被 Data 拒绝（既有单测直调 Adapter 掩盖了该缺口）。
  - 测试：trend 包纯符号/质量边界/非规范证据，Facade 三类结果，5 类 Stage Golden（bullish/bearish/sideways/degraded/unknown），文档 Schema 与运行时同判定，跨模块契约（Entry ↔ 关联规格表 ↔ 真实 Envelope），Registry 版本唯一性。
- 1.5 交付（Data 质量事实 → Quant 适用性的正式映射）：
  - `internal/quant/application/quality_applicability.go`：7 个封闭原因码 + 固定优先级（PIT → 快照质量 → 日历质量 → Data 覆盖率 → 样本下限 → Registry 覆盖率 → 逐 bar 质量），表外原因不猜测。
  - `validateAuthorizedCloseSeriesSnapshot` 拆成“结构完整性”与“质量事实→适用性”两段；历史回放缺 PIT 证据继续硬阻断，不退化为“尽力计算”。
  - `InputNotApplicableError` unwrap 到 `ErrCloseSeriesSnapshotQuality`；三个 Facade 统一经 `classifyCloseSeriesValidationError`，把原因码保留在错误链上。
  - Stage 层 `quant_input_reason.go` 把原因投影为 `quant_{sma20|factor|trend}_input_*`，不再退化为 `*_execution_failed`；表外原因回退为 `*_input_not_applicable`。
  - **错误分类语义变化（已同步改单测，不放宽门禁）**：此前只有“覆盖率低于 Registry 策略”落在 `Err*SnapshotQuality`，其余质量类失败（快照质量/日历/覆盖率标记/逐 bar 质量/PIT）都落在 `ErrInvalid*Snapshot`。1.5 起“输入存在但不可用”统一走前者并携带原因码，Snapshot 自身不自洽仍走后者且不带原因码。两类都继续 fail-closed。
- 验证：`go build ./...`、`go vet`、`go test ./internal/...` 全通过；`go test -race ./internal/quant/application/ ./internal/server/bootstrap/` PASS。变异验证三处：删 `CoverageSufficient` 分支（14 处断言失败）、删 PIT 映射分支（映射与 Factor 执行器测试失败）、去掉原因码透传（分类与 Facade 测试失败）；恢复后全绿。
- 状态：1.3、1.5 记为已完成（实现层面）；QNT-CTR-003 仍 `deferred`、004/005 仍 `frozen`，QNT-008 仍 NOT_STARTED。CI 状态以推送后 GitHub Actions 结论为准，本节不预填。

## 2026-11-26 执行清单 1.6 market breadth 指标决策（书面结论，待 M1）

- 起始 HEAD `2bd11ca31`（1.3 落地与 1.5 门禁映射已推送）。本组只做核实与结论，不新增代码、Schema、Golden、Registry Entry、Stage 或 Data 契约。
- 代码级事实：`internal/service/wiring.go:210-255` 的 `marketAnalyzer` 用 `MarketSnapshot.Bars`（由 `universeResolver` 复制 `req.Symbols` 而来）算 `watchlist_above_ma20_pct`，以 65/35 阈值映射 `watchlist_trend_positive`/`watchlist_trend_weak`/`watchlist_mixed`，算法版本 `mvp-watchlist-market-v1`；缺指数日 K 时自身写入“不能视为完整市场广度分析”。分母随请求变化 → 跨 Run 不可比，且不含停牌/ST/退市/新股规则。
- 消费者边界：新链零消费者（`quant` 无 MarketStage/MarketState DTO，2.1 Template 不含 Market Stage，QNT-005 NOT_STARTED）；旧链消费者是 `analysis/app/runner.go`（仅校验 regime 非空）与旧报告页（展示“市场状态”），随阶段 4 删除。
- Data 侧能力边界：`MarketBreadthSnapshot` 已是严格的 `cn_a_share` 全市场口径（四类家数精确对账、`requireFullCoverage`、部分覆盖必须 degraded、最旧成分报价时间做新鲜度），并有 `GET /api/v1/market/breadth`；但 Quant 唯一合规入口（授权快照）只有单证券 `daily_bars_close_series`，广度事实暂不可被 Quant 追溯消费。
- 交付：`doc/quant/08-market-breadth指标决策与正名.md`——代码级事实、差距矩阵、选项 A（正名为 `cross_section_participation_ma20`，A1/A2 两条实现路径及其代价）、选项 B（书面废弃，推荐）、命名规范、复活条件、QNT-005 前置依赖精确化。
- 状态：1.6 记为“书面结论完成，待 M1 决议”，不记为已完成；QNT-005 前置项已在其任务表中精确化，契约状态全部不变。CI 状态以推送后结论为准，本节不预填。

### M1 决议与 1.6 收尾（2026-11-26）

- 用户决议：按推荐项执行**选项 B 书面废弃**。1.6 由“书面结论完成、待 M1”改为已完成。
- 执行边界：不迁移、不正名、不注册为 Quant 指标，不新建“自选股广度”替代物，不为其补测试；`internal/service/wiring.go` 的 `marketAnalyzer` 仅加退役注释（指向 `doc/quant/08`，说明分母来自调用方 symbols、65/35 阈值无出处、新链无消费者、随阶段 4 删除），行为零改动。
- 命名规范与复活条件继续生效：`breadth`/`广度` 只指 Data 的 `MarketBreadthSnapshot`；横截面比例必须 `cross_section_*`/`*_participation` 并显式写分母；复活需“真实调用方 + 横截面授权快照 + 正名与独立算法版本”三条同时满足。
- QNT-005 前置项维持精确化状态（广度授权快照 payload、交易状态/排除事实、指数授权快照），不以 HTTP 广度或自选股横截面替代。契约状态全部不变。
- 1.6 无代码行为变更，因此不新增单测；验证仍以全量 `go build ./...`/`go test ./...` 为准。

## 2026-11-26 执行清单 1.4 tradability 硬门禁切片（部分完成）

- 起始 HEAD `d2ef6a93c`（1.6 决议收尾）。本组只落地“事实缺失即阻断”的等价门禁，不发明业务规则、不建 Facade/Stage、不改契约。
- 交付 `internal/quant/risk/tradability.go`：规则集 `qnt-risk-tradability/1.0.0`、规则 `tradability_unverified`（block + required）、四类封闭事实（风险警示/停牌/涨跌停/可成交量）、证据引用强制校验、表外事实 fail-closed。
- 不可覆盖性用结构保证而非约定：输入结构不含 Status/RuleSetVersion/评分/模型建议；规则集版本由 `EvaluateTradabilityGate` 固定注入；`Decision.Status` 只能由 `AggregateStatus` 推导；白名单测试阻止未来新增覆盖通道。
- 与旧链等价：旧链四类事实不可得即 `Allowed=false`；新门禁未验证即 `reject`；旧链缺判定在规划阶段拒绝；新门禁必需规则缺失/执行失败为 `block+unknown`（比旧链更严）。
- 阻塞登记（1.4 剩余部分）：Signal 契约（QNT-008 NOT_STARTED）、Data 可交易性事实（缺涨跌停/可成交量，且无授权快照 payload）、权威规则目录。文档：`doc/quant/09-risk-tradability硬门禁切片与前置.md`。
- 验证：`go build ./...`、`go vet`、`go test ./internal/quant/...` 通过；变异验证两处（未验证改 not_triggered、去掉证据强制）均使测试失败。CI 以推送后结论为准，本节不预填。

## 2026-11-26 1.4 三条前置：Signal 契约 + 规则目录 + Data 事实契约层

- 起始 HEAD `d2ef6a93c`（1.6 收尾）。用户明确要求"三条前置先补上，别卡住"，并在三个口径问题上全部按推荐项通过：最小信号 / 只承载真实拥有的事实 / 只登记有依据的规则。
- 交付一（QNT-CTR-006 Signal 1.0.0，DEC-059）：`internal/quant/signal` 强类型 DTO + Schema + Golden + 一致性测试。最小信号不含价格（避免复权口径歧义）与自然语言交易规则；`direction`/`status` 独立枚举、禁止互推与自动投射。
- 交付二（规则目录）：`internal/quant/risk/catalog.go` + 测试。目录条目携带依据来源与发布状态，pending 占位表达"缺权威阈值"；发布集合引用未发布规则即失败，不做"跳过缺失规则继续评估"。
- 交付三（DATA-CTR-007 扩展，DEC-060）：`security_trading_status_facts` payload 1.0.0 + 同族 Manifest 1.0.0 + Schema/Golden + 测试。五项真实事实逐项 available/unavailable；不含涨跌停/可成交量；JCS 双指纹与组合 SnapshotRef；本轮只交付契约层，存储与授权分流待接入。
- 契约统计变化：正式 CTR 44→45（新增 QNT-CTR-006 `deferred`）；QNT-CTR-003/004/005 与 DATA-CTR-007/008 状态均未改变。
- 验证：`go build ./...`、`go test ./internal/...` 全绿；变异验证两处（tradability 条目改未发布、去掉事实 key 顺序校验）均使测试失败。
- 1.4 仍为部分完成：唯一阻塞是 Data 交易事实的物理读取链路（存储列 + 授权读取分流 + Creator + MariaDB 集成测试）。

## 2026-11-26 1.4 前置② 物理读取链路（Data 存储/授权分流 + Quant 消费端，DEC-061）

- 影响分析先行，用户三项决议全部按推荐项通过：共享表 + `payload_kind` 列、独立 `policy_ref`、门禁事实集维持四项。
- Data 侧：迁移 V2 只 `ADD COLUMN`（不改表名，避免 DDL 非事务留下 `status=failed`）；快照写入显式给 kind、读取带 kind 条件；授权策略引用转闭集，`Issue`/`AuthorizeRead`/Owner 发布/HTTP 四处校验数据类一致且不回落日线；新增事实快照 Repository/Port/Creator/Authorized Reader（接口分开，不扩既有签名）。
- Quant 侧：`tradability_facts.go` 组装门禁输入，`price_limit`/`tradable_volume` 恒缺失；`server/bootstrap` 适配器转换前 `Verify()`，校验 `SnapshotRef`/`PurposeCode`/`PayloadKind`。
- 结论诚实登记：链路打通后决策仍恒 `reject`（缺涨跌停与可成交量事实），验收标准是"可追溯的拒绝"。端到端测试名 `ProducesTraceableRejection`。
- 验证：`gofmt`/`go vet`/`go test ./internal/...` 全绿，`-race` 覆盖 `internal/marketdata/...`、`internal/quant/...`、`internal/server/bootstrap`；变异验证六处均有效失败（含 HTTP 回落日线策略、发布传空 policyRef）。MariaDB 集成测试需 DSN，未在真实库执行；CI 结论以推送后为准，本节不预填。
- 1.4 仍未完成：缺的不是链路而是事实（涨跌停制度表、可成交量模型），须另行立项。

## 2026-11-26 1.4 Facade/Stage 装配与拒绝路径验收（DEC-062）

- 影响分析先行（`doc/quant/11-Risk门禁Stage装配与拒绝路径影响分析.md`），用户四项决议全部按推荐项通过：① 拒绝表达在决策里（Stage `succeeded` + `decision_status=reject` + Manifest `result_status=rejected`）；② 本轮不声明 `hard_risk` 门禁；③ 参数不要求 Signal 引用；④ `evaluated_at` 取 Run 的可信 `system_as_of`。
- Facade（`internal/quant/application/create_risk.go`）：授权事实快照 → `AssessTradabilityFacts` → `EvaluateTradabilityDecision` → `NewRiskPayload` → 既有 `resultPersister.persist`；`trade_date > system_as_of` 直接拒绝；`quality_status` 由事实快照质量决定，不把 `degraded` 升级为 `valid`。
- Stage：`internal/quant/stage/risk_output.go`（Stage Code/OutputType/SchemaVersion + 严格 Validator）、`risk.go`（Adapter：参数校验 → 创建事实快照 `purpose=risk_review` → 调 Facade → 双 Validator 自检）、`internal/server/bootstrap/quant_risk_stage.go`（Registry Set **1.5.0**、冻结模板、Executor、错误映射）。决策为 `unknown` 时不产出 Stage Output（`quant_risk_decision_unknown`），避免把规则执行失败当正常结论。
- 契约（DEC-062，QNT-CTR-003 additive L2 补充）：`QNT-CTR-003-risk-stage-output-1.0.0`、`QNT-CTR-003-risk-quality-summary-1.0.0`、`doc/researchflow/schema/quant-risk-parameters-1.0.0.schema.json` + Golden。`reasons` 恰 1 条（`tradability_unverified`/`block`）、`input_refs` 恰 2 条且首条为 snapshot、事实清单封闭去重、`status`↔`result_status`↔`reason.status` 由 Schema 硬绑；`evaluated_at == system_as_of` 与 reject 时清单非空由运行时 Validator 承担，并在 Schema description 中声明该分工。QNT-CTR-003 仍 `deferred`、004/005 仍 `frozen`，不触发 L1 冻结审查。
- 四处接线（缺一处即"注册成功但生产不可用/读不回"）：`isQuantStageCode` 服务身份注入、Dispatcher 按 Stage Code 分派、`quantStageAssociationSpecs` 结果授权读取白名单、`researchflow_template_prerequisites` 参数 Schema 与 `quant.risk.always` 适用性规则。历史教训：这类遗漏直接调 Adapter 的单测不会暴露，已各配专门测试。
- 验收（"装配 + 拒绝路径"优先）：新增 23 个测试全 PASS（Facade 6、输出契约 4 含 14 个篡改变异子用例、Stage 组合 9、端到端 4）。断言 `decision_status=reject`、`missing_facts=[price_limit,tradable_volume]`、`evidence_refs` 精确等于 `[facts_snapshot_ref, authorization_ref]`、`evaluated_at == system_as_of`、`result_id` 可解到已落库记录且 `record.Validate()` 通过。
- 变异验证 6 处全部有效失败并已恢复。其中"关联规格表漏登记 `quant.risk`"**原契约测试未捕获**（按表遍历会随表变短），已补 `TestQuantRiskStageIsRegisteredForResultAssociation` 显式断言后捕获——属补强要求而非调低标准。
- 验证：`gofmt`/`go vet`/`go test ./internal/...` 全绿；`-race` 覆盖 `internal/quant/...` 与 `internal/server/bootstrap` 无告警。真实 MariaDB 集成未在本地执行（无 DSN）；CI 结论见下节（首推失败于无关断言、修复后全绿），本节不预填。
- 1.4 仍为**部分完成**：链路与装配已通，但 `AssessTradabilityFacts` 无条件补两项缺失 → `Verified` 恒 false → 决策恒 `reject`，**门禁不能放行**。剩余阻塞为涨跌停制度表与可成交量模型（另行立项）。RF-BLOCK-008（Researchflow Composer 的 `RiskDecision` 投影）仍 OPEN，需双 Owner 决议。

### 2026-11-26 首次推送的 CI 失败与修复（Data 迁移历史断言）

- 推送 `8d3a9f40e..c4fd7380b` 后 Integration CI 失败（Run [45800949890](https://github.com/Kasign/stock_data_provider/actions/runs/45800949890)）：`internal/server/migration` 的 `TestMarketDataBaselineMigrationAndConcurrentRootRegistration` 断言"Data 迁移定义恰好一条 V1"，而 DEC-061 已按模块自身策略（"Fresh databases apply one V1. Future published schema changes start at V2"）新增 V2 `ADD COLUMN payload_kind`。
- **未在本地发现的原因**：该测试以 `SERVER_RELAY_TEST_MYSQL_DSN` 门控，本地无 DSN 时输出 SKIP 并计为通过；同批修改里 `internal/marketdata/repository/mysql_versions_test.go` 与 `internal/marketdata/bootstrap/module_test.go` 已改为期望 2 条迁移，唯独这份 DSN 门控测试未同步。这是"跳过即绿"的典型盲区，不是 CI 环境问题。
- 同一 Run 的其余 4 项工作流（Required checks 45800949902、Go test suite 45800949922、Quant module validation 45800949941、Live provider probe 45800949994）均 success，Integration CI 内除 `TEST_ROOT` 外 12 项检查亦全 success——失败仅由该断言引起，不含其他回归。
- 修复（`cb11af410`，`test(data): 修正 Data 迁移历史断言以覆盖 V2 增量`）：
  1. 去掉写死的版本数量，期望版本集合由 `MigrationDefinitions()` 推导，再与 `platform_schema_migrations` 逐条对齐（每个版本恰好一次且 `status=applied`），避免每加一条增量都要同步改这里；
  2. 迁移列表形状继续由无 DSN 的 `mysql_versions_test.go` 与 `bootstrap/module_test.go` 守着，此处不重复；
  3. 列存在性断言补 `market_data_daily_bars_close_series_snapshots.payload_kind`，让 V2 的实际效果被覆盖——此前 V2 只有"定义条数"层面的断言，缺"列真的建出来了"的证据。
- 修复后 CI（`cb11af410`）：Integration CI [45802338022](https://github.com/Kasign/stock_data_provider/actions/runs/45802338022)（`Full integration checks` 全 13 项含 `TEST_ROOT`、`Summarize all checks` 均 success）、Go test suite [45802337955](https://github.com/Kasign/stock_data_provider/actions/runs/45802337955)、Required checks [45802337985](https://github.com/Kasign/stock_data_provider/actions/runs/45802337985) 均 success。
- 该 SHA 只有上述 3 个工作流触发；`Quant module validation` 与 `Live provider probe` 因路径过滤未触发，**不以前一次运行的结论冒充**。
- 代码组结论：`2e4b13583` 的生产代码与 `cb11af410` 一致（两者仅差该测试文件），由 `cb11af410` 的 CI 覆盖；`c4fd7380b` 自身的 CI 失败记录保留，不改写为通过。

### 2026-11-26 L3 补验：真实 MariaDB/Redis 全链路（阶段 2.1 遗留项）

- 背景：阶段 2.1（`doc/quant/13` 第 11.6 节）诚实登记"完整 MariaDB/Redis 闭环未执行"。本轮补上服务与编排部分，不把 L2 内存 Store 证据冒充 L3。
- 本机启动真实服务：Docker Desktop + `mariadb:11` / `redis:7-alpine`（CI 为 11.4 / 7.4），端口 3306 / 6379。先以真实 DSN 跑通 Platform Storage/Security 集成（5/5 PASS）、`TestWorkerBootstrapMariaDBRedisIntegration`（PASS）与组合根真实 MariaDB 装配（PASS）。
- 新增 `internal/server/bootstrap/quant_research_mariadb_redis_integration_test.go::TestQuantResearchMariaDBRedisL3`：隔离库 + 完整迁移 + 生产组合根（带 Data Module）→ 发布 `quant.research@1.0.0` → Owner 创建带 per-Stage 信封的 Run → 真实 Worker OS 进程领取执行 → 终态从 MariaDB 回读。四 Stage 均 `AttemptNo >= 1`。
- 真实装配约束（测试配置缺失，非产品缺陷）：Worker 注册 Quant Stage 强制要求 `data:read` Scope 服务凭据，必须注入 `PLATFORM_SERVICE_CREDENTIALS` + `QUANT_SERVICE_TOKEN`；Data Module 不能把 TradingCalendar/SecurityTradingStatus 能力挂在 eastmoney 上。两处均按现有安全门禁补齐配置，未放宽校验。
- CI 门禁：`integration-ci.yml` `checks` job 注入 `QUANT_RESEARCH_MARIADB_REDIS_L3_TEST=1` 与 MariaDB/Redis service；新增精确步骤要求 `--- PASS`，SKIP 或 no-tests 一律失败，避免"跳过即绿"。
- **诚实边界**：隔离库无行情 PIT 种子数据，Data 的 `GetDailyBarsV2` fail-closed，四 Stage 均产出 `quant_*_execution_failed`、Run 终态 `failed`。这是正确行为；本轮只证明 MariaDB/Redis + 编排闭环，不证明"有数据 → 四 Stage 成功"。行情种子数据链路需专项立项，不伪造快照、不降低门禁。
- 本地全量 `go test -p 1 ./...`（真实 DSN）出现一处 News 并发摄取失败：`TestNewsMariaDBApplicationIntegration` 报 `Error 1020 Record has changed since last read`。归因：本机镜像为 mariadb:11，CI 为 11.4；该用例（跨来源并发规范 URL 摄取）从未在本地 DSN 模式验证过，且本轮未修改任何 News 代码。按规则 12 不修改无关模块消除失败；CI 结论以推送后为准，本节不预填 PASS。

- CI（`2e21208d6`）：Integration CI 45820373465 / Required checks 45820373630 / Go test suite 45820373809 / Quant module validation 45820373642 全部 success；Integration CI 中新增的 `Run quant.research MariaDB/Redis L3 integration test` 步骤结论 success，证明 L3 门禁在 CI 真实 MariaDB 11.4 / Redis 7.4 下通过而非 SKIP。

## 2026-11-26 行情种子数据专项：真实 Data Module 下四 Stage 结果闭环

- 影响分析先行：`doc/quant/14-行情种子数据专项影响分析.md`。用户决议按推荐项执行：方案 A（真实种子 + 真实 Data Module）、D1/D2 按推荐；合成但合法种子 + risk 保留 reject。
- 新增 `internal/server/bootstrap/quant_research_l3_seed_test.go`：通过生产 Repository API 向 L3 隔离库写入 PIT bar revisions、两张精确区间交易日历、授权策略与 trading-status 事实；所有 bar 带规范化 Volume/Amount 与精确 Close 证据，日历/bar/limit 相互自洽。
- L3 测试 `TestQuantResearchMariaDBRedisL3` 从"仅四 Stage terminal"收紧为：Run `succeeded`；`quant.factor`/`quant.trend` `succeeded`、`quant.sma20` `degraded [quant_sma20_insufficient_data]`，四 Stage 均有 OutputRef；读取 risk payload 断言 `status=reject`、`result_status=rejected`、`missing_facts` 含 `price_limit` 与 `tradable_volume`。
- 逐层失败归因与修复（均未降低门禁）：① 种子 SourceVersion 改为 Close Series 白名单常量；② PIT 重建提示不再被当作质量降级（同步修正 normalize/reader 测试）；③ bar 补齐 Volume/Amount；④ 日历按精确 start/end 分别写入；⑤ 因 sma20/factor 共用日历且 factor limit=6、`minimum_coverage=1`，10 月窗口只保留 6 个交易日，sma20 合法降级；⑥ risk 事实补齐历史名称与上市交易日序号；⑦ 修复真实 MariaDB 首次暴露的授权决策 JSON 字段顺序缺陷（Application 签发结构与 Repository 规范化字节校验不一致）。
- 本地针对性验收：`go test -p 1 ./internal/marketdata/normalize/ ./internal/marketdata/application/` PASS；`go test -p 1 ./internal/marketdata/application/ ./internal/marketdata/repository/ -run 'DataAuthorization|DataAllowed|Snapshot' -count=1` PASS；`go test -p 1 ./internal/server/bootstrap/ -run '^TestQuantResearchMariaDBRedisL3$' -count=1 -v -timeout 150s` PASS。
- CI 结论（代码 SHA `cdcc31c55`）：该 SHA 15 个 check run 全部 success——Integration CI [45831923632](https://github.com/Kasign/stock_data_provider/actions/runs/45831923632)（`Full integration checks` 全 13 项含 `Run quant.research MariaDB/Redis L3 integration test` 均 success）、Go test suite [45831923678](https://github.com/Kasign/stock_data_provider/actions/runs/45831923678)、Required checks [45831923615](https://github.com/Kasign/stock_data_provider/actions/runs/45831923615)、Quant module validation [45831923750](https://github.com/Kasign/stock_data_provider/actions/runs/45831923750)。CI 的 L3 步骤要求 `--- PASS`，SKIP/no-tests 即失败，故为真实执行而非跳过即绿。
- 边界：三处变异验证（无日历/无策略/未来 SystemObservedAt）未执行，状态 `not_run`，不得记为已验收；本节的 CI 结论只归属代码 SHA `cdcc31c55`，文档回填提交本身不预填 CI。阶段 2.1 的数据准备边界已关闭；2.2/2.3/2.4、阶段 3/4/5 仍按依赖顺序推进。

## 2026-11-26 阶段 2.2 影响分析（事实纠偏；决策已确认）

- 影响分析先行：`doc/quant/15-阶段2.2结果关联与授权读取影响分析.md`。用户确认 D1–D6 全部按推荐项：D1 范围/消费锚点=A（领域层+组合根+真实 Worker 可信边界，不新增 HTTP）、D2 完整报告 Owner=A（本轮不新建）、D3 关联持久化=A（保持运行时重算）、D4 Evidence=A（同构窄端口，不改 News 内核）、D5 2.4 上下文=A（只语义冻结）、D6 契约/SRV-CTR-001=A（一律不动）。
- **事实纠偏（本组施工依据，原 2.2 行口径过期）**：① `internal/researchflow/resultvalidation/quant.go` 是 FLOW-006 消费侧 Envelope 校验器，全仓零 Go import、只被自身 `_test.go` 触达，属孤儿包，**不是** 2.2 的关联/授权机制；② 真正的关联校验器 `internal/server/bootstrap/quant_result_association.go` **已生产装配**于 `researchflow_execution_composition.go:286`，四 Stage 封闭规格表、双 Validator、`result_id` 精确比对与 fail-closed 分流齐备；③ 真正"零"的是 `researchflowQuantResultReader.Read`（`researchflow_result_identity.go:41-49`）与 `quantResultReader` 字段——组合根构造完对象图后无任何 Stage/Worker/HTTP 读取，L3 集成测试亦零引用。故 2.2 精确含义是「生产授权读取行为未闭环」，不是「代码未写」，施工按"接通+验收既有能力"而非新建机制（规则 15）。
- 其他确定事实：`researchflow_execution_composition.go:259-269` 的 Repository 选取漏了 `quantRiskStage`；关联靠读时从 Stage Output payload 的 `result_id` 重算，无 `(run_ref,result_id)` 表/冻结契约；Research Evidence 侧关联完全空白（`GetEvidenceRequest` 无 RunRef、`RelationStatus=not_provided`、两套策略锁表）；完整报告资源唯一 Owner/ResultRef 未决（RF-BLOCK-007 OPEN）；SRV-CTR-001 仅三路由且不开放 Stage Output 内容读取。
- 施工分组：A 口径回填 → B 必要上下游修正（Risk 门控、规格表 Trend/Risk 覆盖断言、真实两-Run 跨读负例）→ C 生产消费者接线 + L3 端到端贯通与四类 fail-closed → D Evidence 侧同构关联。每组先补能暴露问题的测试、保留失败证据，再改实现。
- **如实保留**：报告正文的 LLM Gateway 生产消费者缺失与 News `available_at` 来源证据未闭环、1.4 恒 `reject`、RF-BLOCK-005/007/008 OPEN、行情种子专项三处变异 `not_run`；本组不生成完整报告正文、不新建 `RunResult` 唯一 Owner 资源、不新增 HTTP 端点、不改任何契约状态、不迁移 WebUI、不删旧链、不放宽任何门禁。
- 状态（影响分析阶段记录）：实施中；完整 CI 结论 `not_run`，不得用本地 PASS 冒充。实施与验收结果见下一节。

## 2026-11-26 阶段 2.2 实施完成与验收（CI 已按 `edd955bf7` 回填）

- 分支基线：`repair_all`，起始 HEAD = `origin/repair_all` = `577f2a40063607ba7e9aff42914afcd96bdf6362`；本组按 D1–D6 推荐项 A→D 连续施工，未切分支/合并/rebase/强推。
- **B 必要修正**：`researchflow_execution_composition.go` 补 `quantRiskStage.repository` 选取，避免只含 risk 的 Run 无授权读取链；`stageOutputReader` 上提为无条件构造，供 Quant Result 与 News Evidence 复用；`quant_result_association_test.go` 规格表覆盖断言补 Trend/Risk 与两组 schema ref 漂移断言。
- **C 生产消费者接线**：新增 `readQuantResultForRun` 作为「Run 终态后结果回读校验」唯一领域入口，只委托 `researchflowQuantResultReader.Read`（唯一注入 Worker 可信身份的进程内边界），HTTP 输入无法构造该标记；不新增 HTTP 端点、不改 SRV-CTR-001。既有 L3 真库用例内新增：引导态策略未发布拒绝、可信 Owner `Publish` 启用既有门禁、同 Run 四 Stage `result_id` 全可读且逐条 `allowed/result_released` 审计、停 Worker 后创建第二个真实 Run 的跨 Run 拒绝、无关联拒绝（allowed 审计 0 行）、四种伪造 RunRef 拒绝、活动读护栏内 revoke 不得线性化（裸读 `current_version` 未变 + `quant_policy_admin_audit` revoke 0 行）且撤销后读取拒绝。
- **D Evidence 侧同构关联**：新增 `internal/news/app/evidence_association.go`（窄端口 `RunEvidenceAssociationVerifier` 与两个错误）、`internal/server/bootstrap/news_evidence_association.go`（只登记 `news.collect_feeds` 的封闭规格表、双生产 Validator、success-only `news_ids` 提取、`newsEvidenceAssociationVerifier`、`researchflowNewsEvidenceReader`）、`news_evidence_association_test.go`；组合根装配 `newsEvidenceReader` 与 `readNewsEvidenceForRun`；L3 真库负例覆盖无 News Evidence Stage 的 Run 与伪造 News RunRef。不改 News 授权读取内核、不共用 Quant 的身份模型/策略锁表/授权主体、不伪造 `RelationStatus=provided`。
- **变异验证 8 处**全部 `FAILED_AS_EXPECTED` 后恢复：C 组（关联放行 / 撤销策略门禁移除 / Worker 身份注入移除 / guard 行锁移除，以 L3 为验真器）、D 组（关联放行 / 完整性门禁移除 / reader 先验关联移除 / 非 success source 过滤移除，以单测为验真器）。C4 初次因变异脚本 `replace(...,1)` 误改到 `SetResultAccessPolicyEnabled` 的锁出现两次 `UNEXPECTED_PASS`，按规则 12 先归因为脚本定位错误，改精确 anchor 后失效；**未**修改断言、放宽门禁或改无关模块来制造失败。
- **最终本地验收（提交前）**：`go test -p 1 ./internal/server/bootstrap/ ./internal/news/app/ -run 'TestNewsEvidence|TestNewsFeedAssociation|TestResearchflowNewsEvidence|TestQuantResultAssociation|TestQuantStageAssociation' -count=1` PASS；`QUANT_RESEARCH_MARIADB_REDIS_L3_TEST=1 ... go test -p 1 ./internal/server/bootstrap/ -run '^TestQuantResearchMariaDBRedisL3$' -count=1 -v -timeout 180s` → `--- PASS (7.07s)`（真实执行非 SKIP）；跨包回归 `./internal/server/bootstrap/ ./internal/quant/... ./internal/news/app/ ./internal/researchflow/...` 共 29 个包全绿；`git diff --check` PASS、`gofmt -l` 无输出、四个变异生产文件无残留 diff。
- **如实保留的边界（不夸大）**：Evidence 真库正例未做——需要真实 News Feed Run + 完整 PIT/授权 fixture，而 `research_news_mariadb_integration_test.go:552` 的 `successful_import_with_verified_available_at` 因 News 未提供可验证 `available_at` 仍显式 `t.Skip`；不得伪造时间证据或放宽严格 PIT。News 撤销/当前策略门禁属 News 内核既有能力（`current_policy_version_change_rejected` 真库用例 + `news_policy_versions FOR UPDATE`），按 D4-A 不改内核、不重复实现。故第 7 节第 6 条只按「关联语义单测 + 真库负例 / 授权语义由 News 内核负责」分层覆盖，不声称「Evidence 四类语义已全真库等价验收」。
- **完整 CI 结论（代码 SHA `edd955bf7a511d3616a00402d0c5e1d63f9a03c2`）**：推送后按该精确 SHA 查询，14 个 check run 全部 `completed / success`——[Integration CI 45848500318](https://github.com/Kasign/stock_data_provider/actions/runs/45848500318)（`Full integration checks` 成功）、[Required checks 45848500136](https://github.com/Kasign/stock_data_provider/actions/runs/45848500136)（`required-checks` 聚合成功）、[Go test suite 45848500266](https://github.com/Kasign/stock_data_provider/actions/runs/45848500266)（11 项全成功）、[Quant module validation 45848500209](https://github.com/Kasign/stock_data_provider/actions/runs/45848500209)。Integration CI 的 `Run quant.research MariaDB/Redis L3 integration test` 步骤日志含 `--- PASS: TestQuantResearchMariaDBRedisL3 (6.90s)`，步骤脚本要求精确 `--- PASS`，SKIP/no-tests 即 `::error::` 失败，故为真实执行。本结论只归属该代码 SHA，纯文档回填提交不预填 CI。
- **状态**：本组范围已完成并验收；CI 全绿不关闭 Evidence 真库正例这一 OPEN 边界（见上条）。
- 其余 OPEN 不变：LLM Gateway 报告正文消费者缺失、1.4 `price_limit`/`tradable_volume` 恒 `reject`、RF-BLOCK-005/007/008、行情种子专项三处变异 `not_run`、2.4、阶段 3/4/5。依赖顺序下一步为 2.4 FLOW-005/006 上下文一致性与风险语义。


## 2026-11-27 阶段 2.3 实施完成与验收（Server 结果读取 DTO / HTTP 契约）

- **基线与分支**：`repair_all`，起始 HEAD `a8bb3ce3f8898999d5af51a438c4289b4bffc992`（= `origin/repair_all`）；用户确认 `doc/quant/16` 的 D1–D6 全部按推荐项。
- **事实纠偏**：2.3 的原清单措辞"报告读取 DTO"易误导为完整报告。核实结论：完整报告资源唯一 Owner / ResultRef 未决议（RF-BLOCK-007 OPEN），LLM Gateway 与 News `available_at` 外部阻塞仍在；2.2 已接通的 `readQuantResultForRun` / `readNewsEvidenceForRun` 只有测试消费者。故 2.3 精确含义 = "Server 结果读取 DTO + 生产授权读取消费端"，不新建完整报告资源、不伪造正文。
- **E2 领域读侧**：`internal/server/bootstrap/researchflow_result_read.go` 新增 `quantResultReadUseCase` / `researchflowRunReadPort` 窄端口、`researchflowRunResultReadService`（GetRun 资源级授权先行）、`readQuantResultsForStages`（只从已授权 Stage Output 发现 result_id，跨 Run/缺引用/读失败/解析失败 fail-closed）、`projectQuantResultForRead`（受限公开投影，risk 不暴露 RiskDecision payload，未登记 result_type 拒绝）。组合根新增两个窄端口字段，复用 2.2 已装配对象（规则 15）。
- **E3 契约（additive）**：DEC-PLAT-002-064；新增 `result-read-response-1.1.0.schema.json`、OpenAPI 新路径、SRV-CTR-001 第 9.2 节；SRV-CTR-001 1.0.0 与 QNT-CTR-004/005 等 frozen 文件字节不变。
- **E4 HTTP 边界**：新增 `GET /api/v1/researchflow/runs/{run_ref}/results`；复用 `authenticate`（仅 Owner Session），结果读取链未装配时不注册路由（404 `researchflow.result_read_unavailable`）；响应 `resource_kind=authorized_domain_results` 显式不冒充完整报告。
- **E5 验收**：5 处变异 `FAILED_AS_EXPECTED`（跨 Run 检查 / result_type 白名单 / risk payload / resource_kind 冒充报告 / 真库 L3 risk payload）；1 处 `UNEXPECTED_PASS` 已按规则 12 归因（L3 层去跨 Run 检查被 Stage Output Reader 的第二道 runRef 校验挡住，E2 单测层该变异已有效失败），未改断言或门禁。`go test ./internal/...` 全包通过；真库 L3 `--- PASS: TestQuantResearchMariaDBRedisL3 (7.53s)` 含新增 2.3 断言（4 种结果类型受限投影 + risk 无 payload + 跨 Run fail-closed）；`git diff --check` PASS、`gofmt -l` 无输出、`go vet ./internal/...` 无告警；文档 Schema 经真实 handler 响应校验通过。
- **如实保留**：UI 页面仍调用旧 API（D4 本轮不改前端，页面切换属 3.1/3.2）；完整报告正文与 RF-BLOCK-007/008 仍 OPEN；阶段 2.3 完整 CI 已按代码 SHA `a000b44d1` 取得：四工作流全 success（Integration CI 45926549542 含 `--- PASS: TestQuantResearchMariaDBRedisL3 (7.12s)`、Go test suite 45926549570、Required checks 45926549669、Quant module validation 45926549532）。详见 `doc/quant/16` 第 12 节。

## 2026-11-27 四项根因根治 R2：tradable_volume 真实盘口闭环

- **范围与边界**：用户明确排除妥协方案——不采用成交量 × 参与率假设，不维持 `tradable_volume` 恒缺失。`tradable_volume` 唯一定义为 Eastmoney push2 真实买盘五档挂单量之和（股），盘口是日内时刻事实，建立独立 `order_book` payload、Manifest、数据类策略与授权快照，不并入单日 `security_trading_status_facts`。
- **已完成子组**：① `OrderBook` model + `order_book` facts payload；② Eastmoney 五档 Provider、配置能力开关、受控出站 origin、DataService fallback；③ 授权快照链（Schema/Golden、双指纹、Repository 按 `payload_kind` 分流、Creator/Authorized Reader、Owner 策略路由）；④ Quant 消费与契约：新增 `TradabilityEvidence` 显式绑定交易状态快照与盘口快照，`missing_facts` 从 `[price_limit, tradable_volume]` 收敛为 `[price_limit]`，四段证据引用 `[交易快照, 交易授权, 盘口快照, 盘口授权]` 贯穿 Decision/Manifest/Stage Output。
- **契约演进**：DEC-PLAT-002-066；DATA-CTR-007 `order_book` 1.0.0；QNT-CTR-003 新增 Stage Output 2.0.0（MAJOR，证据四元化）与 Quality Summary 1.0.1（PATCH 描述修正），旧 1.0.0 两份保持冻结；Risk 单 Stage Registry Set 1.5.0 → 1.7.0，合并 Set 1.6.0 → 1.8.0。QNT-CTR-003 / DATA-CTR-007 状态仍 `deferred`，不升级 L1。
- **组合根与真库**：新增 Server `quantOrderBookFactsAdapter`，Risk Stage 同时创建两类授权快照；L3 种子发布 `order-book-facts-access` 策略并启用 Eastmoney quote/order-book 能力，风险断言改为只缺 `price_limit` 且四段证据齐备；新增 MariaDB 盘口快照共享存储、`payload_kind` 跨类隔离与篡改检测测试（本地无 DSN 时 SKIP，CI 由 Integration CI 执行）。
- **变异验证**：三类生产语义变异均 `FAILED_AS_EXPECTED` 后恢复——把 `tradable_volume` 再写成缺失；把四段证据缩回交易状态两段；关闭两份快照的 symbol/system_as_of 绑定校验；另验证 Stage Validator 的证据基数回归会导致合法输出被拒。未删除断言、未放宽门禁。
- **本地验收**：`go test ./internal/... -count=1` 全包通过；`go vet ./internal/...` 通过；`git diff --check` 通过。三个已提交子组 SHA 的 CI 已按精确 SHA 查询：R2 子组一 `0c0418544`、子组二 `feda09168`、子组三 `20d6533d4` 四工作流全 success（Live provider probe / Required checks / Go test suite / Integration CI）。当前子组四提交 SHA 待生成，CI `not_run`。
- **如实保留**：`price_limit` 仍无合格事实来源，RiskDecision 仍 `reject`，不得表述为硬门禁已放行；R3 News P1、R4 完整报告唯一 Owner 仍 OPEN；当前子组四 CI 未取得前不得标记完整验收。

### 2026-11-27 R2 CI 失败归因与精确盘口快照复用修复

- **CI 事实**：子组四提交 `43053390f90d88201d2504fca711e8299c3af81c` 的 Required checks `45935976408`、Go test suite `45935976422`、Live provider probe `45935976512`、Quant module validation `45935976557` 均 success；Integration CI `45935976418` failure。所有前置步骤 success，失败步骤为汇总器，因为 `TEST_ROOT=failure`；根模块测试日志定位到 `TestQuantResearchMariaDBRedisL3` 的 `quant.risk` Stage：`status=failed`、`reasons=[quant_risk_execution_failed]`。
- **根因归因（代码问题，非环境问题）**：底层错误为 `Eastmoney order-book bids: invalid price field f19`。CI 与本地复现均发生在午间休市时段，Eastmoney 单证券详情只返回 `f57/f86`，没有五档深度。Provider 按契约 fail-closed 是正确行为；缺陷在于 L3 真库测试隐含依赖实时盘口开放时段，而不是盘口事实门禁放宽。
- **修复语义**：`snapshotport.OrderBookFactsSnapshotRepository` 新增按精确 `symbol + purpose + system_as_of` 查找已持久化盘口快照的窄方法；MariaDB 实现只用精确相等，不做“最新一条”或时间范围近似匹配。Creator 在读取实时 Provider 前精确复用已有真实盘口事实并重新签发授权；绑定不一致即 fail-closed。L3 在 CreateRun 得到精确 `system_as_of` 后写入合法盘口快照，因此不再依赖运行时刻是否处于交易时段。没有采用参与率、推断值、恒 reject 或放宽断言。
- **TDD 与变异验证**：修复前 `TestOrderBookFactsSnapshotCreatorReusesExactSystemAsOfSnapshot` 因调用实时 Provider 失败；修复后通过。变异一关闭 Creator 绑定校验，`RejectsReusedSnapshotWithDifferentSystemAsOf` failed；变异二移除 Repository 的 `system_as_of` 过滤，真库 `TestMariaDBOrderBookFactsSnapshotExactSystemAsOfReuse` failed；均恢复后复验通过。
- **本地验收**：`go test ./internal/... -count=1` 全包通过；`go vet ./internal/...` 通过；`gofmt -l` 无输出；`git diff --check` 通过；真库 `TestMariaDBOrderBookFactsSnapshotExactSystemAsOfReuse` PASS；本地 MariaDB/Redis `TestQuantResearchMariaDBRedisL3` PASS，四个 Stage 均终态且 `quant.risk=succeeded`。
- **CI 状态（回填）**：修复提交 `1423d29c9460906fc5caec969fb065f2a3f18eb9` 的 Required checks `45937832282`、Go test suite `45937832293`、Live provider probe `45937832230`、Integration CI `45937832229` 均 success；Integration CI 明确包含 `--- PASS: TestQuantResearchMariaDBRedisL3 (7.02s)`。Quant module validation 因本轮未改 `internal/quant` 未触发，按工作流路径规则不适用。R2 代码与 CI 验收完成；`price_limit` 仍缺失、RiskDecision 仍 reject 的语义边界不变。

## 2026-11-27 四项根因根治 R3-T2：来源无关 AvailabilityDecision 审核用例

- **基线与边界**：`repair_all`，T1 文档提交 `0806e572caf62281ab22d1789d94ed64e48cd53e`（DEC-PLAT-002-067）已推送；本组只做来源无关审核用例与决策链读取，不接入 Provider、不生成非空 `available_at`、不改 NEWS-CTR-001 1.0.0 Schema/Golden。
- **应用语义**：新增 `internal/news/app/availability_decision_review.go`，以 `AvailabilityReviewRule` 承接未来来源准入规则（版本必须绑定请求中的 `ReviewRuleVersion`）。唯一可形成 `verified` 的路径是规则判定 eligible、证据 `parsed` 且具备秒/分钟精度、来源时区、证据 URL/hash 与候选时间；多条 eligible 无唯一 preferred → `conflict`，无 eligible → `unresolved`，规则判定撤销 → `revoked`。决策保留完整 `considered_evidence_ids` 与 `supersedes` 链，不覆盖历史决策。
- **仓储语义**：`MySQLRepository` 新增 `LatestAvailabilityDecisionForVersion`，只读取链头；并发正确性仍由 `AppendAvailabilityDecision` 的版本行锁与序号 CAS 保证。真库用例新增链头断言与未知版本 `ErrAvailabilityDecisionNotFound` 断言。
- **TDD 与变异**：新增用例先因 API 未定义构建失败；实现后 `verified/conflict/unresolved/revoked`、规则版本不匹配、foreign/purged evidence、append 失败均按预期通过。变异“把 parsed 直接判为 verified”使 `TestReviewAvailabilityDecisionTreatsParsedWithoutRuleEligibilityAsUnresolved` 有效失败并恢复。
- **本地验收**：`go test ./internal/news/app -run TestReviewAvailabilityDecision -count=1` PASS；`go test ./internal/news/... -count=1` PASS；`go vet ./internal/news/...` PASS；`go test ./... -count=1` PASS；决策链真库 `TestAppendAvailabilityDecisionSerializesConcurrentWritersMariaDBIntegration` PASS。
- **本地基线边界**：本地 MariaDB 11.8.9 上 `TestNewsMariaDBApplicationIntegration` 的并发 URL 去重置现 `Error 1020`；在未修改基线同样失败，归因环境与既有基线，不修改无关模块或放宽断言。CI 目标为 MariaDB 11.4。
- **CI 状态（回填）**：代码提交 `003e4463f3de70c406e255309c057d54a8128fc5` 按精确 SHA 查询的 13 个检查全部 `completed/success`：Integration CI `45940026599`、required-checks `45940026508`、Go test suite `45940026587`（含 News MariaDB、Root Go module、Research/Researchflow/Notification/Market-data 等全部 Job）。真实来源未准入，NEWS-BLK-07/RSC-002 仍 OPEN，R3 未完整验收。

## 2026-11-27 四项根因根治 R3-T3：来源准入与证据产出框架

- **基线与边界**：本组在 `003e4463f` 之后继续，仍只做来源无关框架；不注册任何真实来源、不写非空 `available_at`、不改 NEWS-CTR-001 1.0.0 Schema/Golden。静态目录默认为空且 fail-closed。
- **资格记录**：新增 `AvailabilitySourceQualification`，强制官方首次公开语义引用、Endpoint 授权、逐版本绑定、声明时区/精度、脱敏原始响应 SHA-256、生效/撤销时间与审核规则版本；记录不保存原始载荷或样本值。
- **产出边界**：`QualifyAvailabilityEvidence` 从资格目录查找精确 Source/Endpoint/Provider 策略版本绑定，只有字段路径、时区、精度与 provenance 全部合法才输出 `AvailabilityTimeCandidate`，供后续 `RecordAvailabilityEvidence` 使用；它不审核、不写入、不产生 `available_at`。
- **审核绑定扩展**：Review Use Case 支持可选 `AvailabilityEvidenceBindingResolver`；`SourceBoundAvailabilityReviewRule` 必须拿到 Evidence→Observation 的精确来源策略绑定，绑定不一致一律 `ineligible`，资格撤销输出 `revoked`，`parsed` 单独仍保持不可信。
- **仓储与目录**：`MySQLRepository.ResolveAvailabilityEvidenceSourceBinding` 通过三方身份一致连接读取绑定；`StaticAvailabilityQualificationCatalog` 为不可变快照，按绑定选择最新 `effective_at`，构造期拒绝重复 ref、非法精度和歧义生效时间。
- **TDD 与变异**：新增测试先因 API 未定义构建失败；变异“忽略来源绑定不一致”使 `TestSourceQualifiedAvailabilityReviewRuleRequiresExactSourceBinding` 有效失败并恢复。`go test ./internal/news/...`（无 DSN）、`go vet ./internal/news/...`、`go test ./... -count=1`、`git diff --check` PASS；决策链与来源绑定真库用例 PASS。本地 MariaDB 11.8.9 的既有并发 URL 去重 `Error 1020` 已在未修改基线复现，不归因本组。
- **CI 状态（回填）**：代码提交 `57dc96804ba2086802e7eedb4f4892b0dfed8d2c` 的 13 个检查全部 `completed/success`：Integration CI `45940754064`、required-checks `45940754025`、Go test suite `45940754043`（News MariaDB、Root Go module 等全部 Job）。T4 契约版本与 B 组严格门禁、T5 真实来源仍待推进；T4 的最终 CI 通过依赖 T5。

## 2026-11-27 四项根因根治 R3-T5：真实来源数据自采结果

- **用户要求**：项目负责人要求“这些数据你自己去获取一下”，本轮直接联网获取公开官方/准官方材料，不再把可公开查询的数据停在口头等待。
- **已取得材料**：上证信息《公告数据服务说明书》1.0、深圳证券信息《关于证券信息的声明》、SSE `queryCompanyBulletin.do` 非空样本、SZSE `annList` 非空样本、CNINFO `topSearch`/`hisAnnouncement/query` 非空样本、东方财富 `api/security/ann` 聚合样本；全部记录 URL、UTC 采集时间、字节数与 SHA-256，详见 `doc/contract/NEWS-CTR-001-available_at-来源证据解锁影响分析.md` 第 1.17 节。
- **字段结论**：SSE `ADDDATE` 为秒级但无官方字段语义；SZSE `publishTime` 和 CNINFO `announcementTime` 实际为日级；东方财富时间字段属于第三方聚合链路；上证信息正式 API 仅产品级提到“发布时间”，字段级技术文档另发且未公开。全部不能映射为 `available_at`。
- **授权结论（按个人项目口径修订）**：项目负责人确认本项目为个人非公开项目，没有外部授权；外部合同/凭据不再是 T5 门禁，`EndpointAuthorizationRef` 使用个人用途与采集方式声明。公开材料仍不能证明字段是“首次公开可获取时间”，也不足以形成逐版本事实链。
- **状态边界**：T5 保持阻塞但不是“未尝试”，状态改为 `blocked_source_semantics`；T4 来源无关部分继续完成，B 组可用显式测试夹具验证真实跨模块成功链路，但生产成功路径仍依赖 T5 的官方字段语义与可审计样本。

## 2026-11-27 四项根因根治 R3-T4a：NEWS-CTR-001 2.0.0 Schema/Golden

- **基线与边界**：T1/T2/T3 已完成；本组只发布来源无关的 2.0.0 Page Schema/Golden 与秒级精度约束，1.0.0 的 4 个 Schema / 3 个 Golden 字节不变。不注册真实来源、不生成非空 `available_at`、不解除 Research B 组成功路径。
- **机器契约**：新增 `doc/news/schema/NEWS-CTR-001-research-evidence-page-2.0.0.schema.json`，`available_at` 从通用 `nullableDateTime` 收紧为独立 `nullableSecondDateTime`：非 null 必须是 UTC `YYYY-MM-DDTHH:MM:SSZ`，null 表示当前版本没有通过来源准入与审核的证据；其他时间字段保持原精度定义。
- **Golden 与测试**：新增 `doc/news/golden/NEWS-CTR-001-research-evidence-page-2.0.0.golden.json`。契约测试先因文件缺失失败，实现后 2.0.0 Golden 同时通过 JSON Schema 与 Go DTO 解析，且非空 `available_at.Nanosecond()==0`；带小数秒的时间被 Schema 拒绝。
- **变异验证**：把 2.0.0 `available_at` 正则放宽为允许小数秒后，`TestNewsEvidenceQueryContractV2RejectsFractionalAvailableAt` 有效失败；恢复后复验通过。
- **登记状态**：`契约版本与变更记录.md` 推进至 `0.2.67-frozen`，PLAT-002 NEWS-CTR-001 行登记 1.0.0 冻结不变与 2.0.0 Schema/Golden；NEWS-CTR-001 状态仍 `frozen`。T4b B 组严格门禁与 T5 真实来源仍待完成。
- **CI 状态（回填）**：代码提交 `a0d766a4beb7eca697a6e3bf460bebbb5e12f7aa` 的 13 个检查全部 `completed/success`：Integration CI `45943432850`、required-checks `45943432879`、Go test suite `45943432868`（Root Go module、News MariaDB 等全部 Job）。T4a 验收完成；B 组成功路径仍保持 fail-closed，依赖 T4b/T5。

## 2026-11-27 四项根因根治 R3-T4b：B 组严格门禁与测试夹具

- **口径修订**：项目负责人确认本项目为个人非公开项目、没有外部授权；外部合同/凭据从 T5 门禁移除，官方首次公开可获取字段语义与可审计样本仍是事实语义硬门禁。生产来源未准入，不写入生产 `available_at`。
- **TDD 证据**：先移除 B 组 `t.Skip` 且未提供 verified 决策夹具，真库测试失败于 `B 组授权读取失败: news.evidence.pit_not_found`；这证明严格 PIT 在缺少已审核决策时仍 fail-closed。
- **测试夹具**：`research_news_mariadb_integration_test.go` 新增显式测试专用 verified Evidence/Decision 夹具，只验证真实 News Facade → Research Importer → `research_evidence_records` 成功链路；不注册来源、不映射 Provider 字段、不把夹具当作生产证据。
- **断言范围**：B 组成功落库列、payload 留痕键、返回值与 News 输出的 `available_at`/AuthorizationRef 严格一致；列表子测试由“无 verified 时为空”改为“verified 后返回 1 条且 `available_at` 为 UTC 整秒并不晚于 system_as_of”。
- **变异验证**：把测试夹具 Decision 状态从 `verified` 改为 `unresolved` 后，B 组再次以 `news.evidence.pit_not_found` 失败；恢复 `verified` 后完整 `TestNewsResearchMariaDBIntegration` PASS。
- **本地验收**：`SERVER_RELAY_TEST_MYSQL_DSN=... go test ./internal/server/bootstrap -run TestNewsResearchMariaDBIntegration -count=1 -v` PASS；`go test ./... -count=1`、`go vet ./internal/news/...`、`git diff --check` PASS。
- **CI 首次失败与修复**：代码提交 `1fab00e4db45b76622a02c2ec6b3b21edb268655` 的 required-checks `45945031267`、Go test suite `45945031324` success；Integration CI `45945031292` failure，根因是工作流精确子测试清单仍写旧名 `list_evidence_page_excludes_missing_available_at`，并非测试失败。已同步为 `list_evidence_page_includes_verified_available_at`，并把 B 组“接受 SKIP”分支改为 SKIP 即失败。
- **CI 状态（回填）**：工作流修复提交 `b22b023cc42fb64c379aaa1ca47dacc891b0f6fe` 的 13 个检查全部 `completed/success`：required-checks `45945574254`、Go test suite `45945574315`、Integration CI `45945574250`（News/Research/Researchflow 跨模块步骤通过）。T4b 验收完成；生产 B 组仍不因测试夹具解锁，R3 完整验收仍依赖 T5 字段语义准入。

## 2026-11-27 四项根因根治 R4-R1：RunResult 可持久化投影

- **治理前置**：DEC-PLAT-002-068 确定 Researchflow 为完整 `RunResult` 唯一 Owner；Run 仅以 additive optional `result_ref` 引用；`ResultSummaryRef` 与 Quant `result_ref` 不得冒充完整报告；旧 Schema/Golden 字节不动。
- **投影边界**：新增 `RunResultFromComposed`，只接受可信 `run_ref`、UTC 时点、Context/Evidence/Claim/Quant/风险结论与非权威 LLM 解释齐全的组合结果；输出只包含来源身份引用和权威风险投影，不复制 Quant/Research 原始资源本体。
- **TDD 与变异**：新增测试先因 API 未定义构建失败；实现后 6 组投影用例通过。变异“放宽 LLM Authority 校验”使 `wrong_authority` 用例有效失败并恢复。
- **状态边界**：本组只完成来源无关的纯函数投影；不建表、不接 HTTP、不生成生产完整报告。生产成功路径仍依赖 R3 真实来源语义与 LLM Provider 接线。
- **CI 状态（回填）**：代码提交 `82e3437d9f41a1c33910f01fdc9007189f6b6635` 的 13 个 check run 全部 `completed/success`：required-checks `45946902003`、Go test suite workflow run `45946901979`（含 Researchflow module format/tests/race/vet、WorkerStore、MariaDB lifecycle、Root Go module 等）、Integration CI `45946902015`（Full integration checks）。R4-R1 验收完成。

## 2026-11-27 四项根因根治 R4-R2：run-result 1.0.0 与 Run 1.1.0 契约

- **契约边界**：经 DEC-PLAT-002-068 授权，新增 `doc/researchflow/schema/run-result-1.0.0.schema.json` 与 `doc/researchflow/golden/run-result-1.0.0.golden.json`；Run 以 additive MINOR 发布 `doc/researchflow/schema/run-1.1.0.schema.json` 与三个 1.1.0 Golden。既有 `run-1.0.0.schema.json` 和全部 1.0.0 Run/Stage/Attempt/ResultSummary Golden 字节不变。
- **机器语义**：RunResult 只保存 Run 绑定、UTC `system_as_of`、Context/Quant/Evidence/Claim 身份引用、非权威 LLM 解释和风险投影；`additionalProperties:false` 禁止复制来源正文，解释 `authority` 固定 `non_authoritative`。Run 1.1.0 的 optional `result_ref` 仅允许报告型终态引用完整报告；created/queued/running/cancelling/cancelled 及 user_cancelled/deadline_exceeded/run_deadline_exceeded 终态一律禁止非空 `result_ref`。
- **TDD 与变异**：先新增引用新 Schema/Golden 的契约测试，因文件缺失失败；实现后 Golden、DTO 投影 Schema 校验、冻结 1.0.0 拒绝 `result_ref`、1.1.0 允许缺省 `result_ref` 与非报告终态拒绝均通过。变异“放宽 Run 1.1.0 终止原因门禁”使 `TestRunV11SchemaRejectsResultRefOnNonReportTerminals` 有效失败；变异“允许 `authoritative` 解释”使 `TestRunResultSchemaRejectsAuthoritativeExplanation` 有效失败；恢复后复验通过。
- **登记状态**：`契约版本与变更记录.md` 推进至 `0.2.68-frozen`；PLAT-002 将 FLOW-CTR-005 从 `planned` 转为 `frozen`（仅契约定义），FLOW-CTR-001 登记 Run 1.1.0 additive。
- **状态边界**：本组只发布机器契约与 Golden；不建表、不接 HTTP、不启用生产完整报告。后续 R4-R3 才做物理表与 Finalizer 原子写入，生产成功路径仍依赖 R3 真实来源语义与 LLM Provider 注入；CI 待代码提交后按精确 SHA 回填。
