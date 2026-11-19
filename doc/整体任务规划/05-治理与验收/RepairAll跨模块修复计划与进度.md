# Repair All 跨模块修复计划与进度

- 仓库：`Kasign/stock_data_provider`
- 工作分支：`repair_all`
- 当前代码验收基线：`5334043ea8e1abc2c4006c371a3f95b123098946`（2026-11-19 最新 HEAD）
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
| P1 | Research Context/Evidence 与 Researchflow Run/Stage/Result 语义及唯一持久化 Owner | Research / Researchflow / Server | 待系统 Review |
| P2 | LLM Gateway 生产 Provider、Secret、Capability Evidence 与真实调用 | LLM / Platform / Server / Researchflow | 待系统 Review |
| P2 | Notification 事件、Recipient、Fanout/Delivery、Worker 和恢复链 | Notification / Platform / Server | 待系统 Review |
| P3 | Diagnostics 注册、身份/审计传播、Worker 恢复、WebUI Root API 对接 | Platform / Server / 各业务模块 / WebUI | 待系统 Review |
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

### RA-CROSS-REVIEW-005：Research / Researchflow Context、Evidence 与 Run/Stage 生产语义 Review

- 状态：PLANNED（前置任务 RA-CROSS-REVIEW-003/004 已完成审查；本任务尚未开始代码修改）。
- 目标：核对 Research Context/Evidence/Event 与 Researchflow Run/Stage/Attempt/ResultSummary 的时间、身份、授权、引用、持久化和消费边界，确认唯一数据 Owner 与真实调用链。
- 范围：Research、Researchflow、Server；仅在公共时间/身份/诊断原语确有归属问题时涉及 Platform。
- 首要核对：RSC-CTR-001/002、FLOW-CTR-001/003/004、GOV-CTR-002/006、SRV-CTR-001；Research/Researchflow 当前任务文档及 Server Root 生产装配。
- 重点检查：Run.system_as_of 是否贯穿 Context/Evidence/Stage/LLM；ResearchContext 创建/读取授权是否由 Owner 当前复核；Stage 输入输出引用是否与 ResultSummary/不可变 StageOutput 资源一致；Event/Claim 版本血缘是否真实可解析；Research/Researchflow 的持久化 Owner 是否唯一；Worker 重试/恢复是否保留身份、时间与引用一致性。
- 验收标准：每条链路记录 PASS / DEGRADED / FAIL / NOT_COMPARABLE / DEFERRED，附代码路径、契约、测试和精确 SHA；无真实调用或来源证据不得标记生产 PASS。
- 修复门禁：先核对上下游调用链，再在责任模块修复；不复制 Repository、授权、时钟、Worker、Diagnostics 或 Stage Provider；不新增无调用场景接口。
- 完成条件：真实缺陷修复并通过针对性测试；跨模块契约、调用方、实现方、Server 装配与测试一致；对应模块卡点和总规划同步；变更后精确 SHA 完整 CI 验收。

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
