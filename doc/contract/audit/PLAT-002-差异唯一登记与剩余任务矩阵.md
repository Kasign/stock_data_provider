# PLAT-002 差异唯一登记与剩余任务矩阵

**用途：** 本文件是审计工作区的唯一执行索引，用于防止 CTR 重复审查、重复计数和任务范围膨胀；不取代 `doc/contract/` 的正式契约与决策登记。
**固定代码基线：** `origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`
**维护分支：** `feat/design_doc`
**本轮复核日期：** 2026-11-10

## 1. 收敛规则

1. 每个问题只保留一个活动 CTR；历史编号只作为别名/追溯，不重复计数。
2. 已完成的代码证据初审不再重读；只有新证据、冲突、责任变化或关闭条件变化才增量复核。
3. 测试缺口不自动升级为新 CTR；只有影响行为可信度、契约一致性或冻结门禁时才保留为差异。
4. 同一 API/DTO 的相关问题合并为一个工作批次执行，不再逐 CTR 单独开轮。
5. “已审查”不等于“已修复”；“已修复”不等于“CI 通过”；“CI 通过”不等于“契约已裁决/冻结”。
6. 审计分支只维护证据与任务索引，不修改业务代码、OpenAPI 或测试。

## 2. 编号去重口径

| 编号/主题 | 当前处理 | 计数规则 |
|---|---|---|
| CTR-003 | Run 路径/响应契约问题由 CTR-046 承接 | 历史别名，不单独计数 |
| CTR-005 | CreateTask 请求 DTO、TaskType/required 约束由 CTR-047 承接 | 历史别名；其剩余关闭条件并入 CTR-047 |
| CTR-010 | Go omitempty 与 OpenAPI nullable 的跨 DTO 字段差异 | 一个聚合 CTR，字段清单为其影响范围，不按 DTO 或字段新增编号 |
| CTR-038 / CTR-048 | 分别属于 Market Sentiment History 与 Provider Probe History | 不合并；相同 total 主题可共享规范，不共享缺陷编号 |
| CTR-041 | 固定基线 DTO、OpenAPI、Golden 三方一致，原登记撤销 | withdrawn，不计活动项 |
| CTR-001 / CTR-002 | WebUI 维度 1 的认证/安全接入问题 | 不纳入维度 2 统计或执行队列 |

当前登记编号覆盖 CTR-001 至 CTR-049。按以上口径，CTR-003/005 是历史承接编号，CTR-041 已撤销；CTR-010 是聚合差异。具体活动状态以各差异明细及下表为准，不用编号区间推断关闭状态。

## 3. WebUI 维度 2 已完成工作（禁止重复初审）

以下数据集已有独立切片及证据，不再重新执行全链路初审：
- Analysis Run / Analysis Task / Run Events
- 全市场分页股票、日 K、Bundle
- 证券资料、交易状态、涨跌幅规则
- StockBoards、Fundamentals、Announcements、Valuations、Chip Distribution
- 个股与市场级 Capital Flow
- Market Session、Market Breadth、Market Indices
- Index Catalog、Index Constituents、Index Daily Bars
- Hot Rankings、Market Sentiment History
- Limit Pool、Dragon Tiger、Stock Search、Provider Probe History

API Client 全量盘点也已完成：44 个导出函数，41 个远端入口，40 个在主要页面模块找到引用；`getMarketSentiment` 是唯一已确认未找到页面消费者的 Client，保持产品范围待确认，不自动新增 CTR。

## 4. 已完成的冻结影响/证据链 Review（不得重做）

| 范围 | 已完成内容 | 当前结论 |
|---|---|---|
| CTR-004 | 全市场分页 API Client 参数证据链 | P2 测试补证；不单独阻断服务端 OpenAPI/DTO 冻结 |
| CTR-038～040 | Market Sentiment History | CTR-038/039 语义待裁决；CTR-040 WebUI 实现未对齐 |
| CTR-042 | Limit Pool 金额单位 | P2；provider_defined 不可比较/换算边界待裁决 |
| CTR-043～044 | Dragon Tiger | CTR-043 P1 实现冲突；CTR-044 Provider 日期能力边界待裁决 |
| CTR-045 | Stock Search 质量状态消费 | P2；WebUI 空结果和质量状态提示待对齐 |
| CTR-046～047 | Analysis Run/Task | P2；Run 读取/事件 OpenAPI 缺失、CreateTask HTTP DTO 语义待裁决 |
| CTR-048～049 | Provider Probe History | P2；total 语义与存储错误 HTTP 映射待裁决/补证 |

这些结论只记录冻结影响，不表示责任模块已修复。不得再次对上述范围执行相同的初审或冻结影响分析；后续只检查裁决结果、责任模块新提交和关闭证据。

## 5. 尚未完成的 WebUI 维度 2 工作

| 工作批次 | 涉及编号/范围 | 当前状态 | 完成条件 |
|---|---|---|---|
| W2-A Task/TaskEvent DTO 与分页 | CTR-006、007、008 | 本轮统一冻结影响 Review 已完成；差异仍开放 | CTR-006 nullable/omitempty 不一致；CTR-007 StageEvent Schema 未展开；CTR-008 cursor 基于事件表 id，但 DDL/数据库持久化约束证据不足。待契约/实现/测试责任方处理后增量验收，不再重复初审 |
| W2-B DailyBar 查询窗口 | CTR-009 | 本轮冻结影响 Review 已完成；差异保留 P2 | WebUI 用户偏好显式传参；API Client 默认 120；服务端省略时使用环境可配置默认值（默认配置 120）；OpenAPI 未说明省略时配置化默认值。缺直接 Client URL/默认值回归测试 |
| W2-C 其余已审数据集差异收敛 | CTR-011～037（不含已在其他批次处理的 CTR-010 聚合字段） | 已有独立切片，尚未完成统一去重/阻断判断 | 只基于现有证据形成一次矩阵；按契约错误、实现偏差、产品范围、测试补证分类，不重新初审代码 |
| W2-D CTR-010 最终语义裁决准备 | CTR-010 | 124 个唯一字段、29 个 DTO、54 个响应 DTO 的静态统计已落档；目标语义未裁决 | 对照权威契约形成“缺失/null/零值”决策问题；不得直接要求 Data 改 DTO 或 OpenAPI，不在审计分支实施修复 |
| W2-E 第一轮完整性 Review 收尾 | WebUI 维度 2 全部活动 CTR | partial | 汇总 W2-A～W2-D，确认数据集覆盖、API 消费映射、编号唯一、责任与关闭条件完整；不要求所有 P2 已修复才算第一轮 Review 完成 |

注：CTR-011～037 是收敛工作范围，不代表每项均阻断冻结。已在独立切片中确认的事实不得重做；如发现编号映射或状态冲突，只登记差异，不重新跑全链路。

## 6. 十模块 PLAT-002 项目级剩余主线

| 阶段 | 当前状态 | 剩余工作 |
|---|---|---|
| A 基线与范围 | complete（固定 SHA、十模块路径、文件规模已建立） | 只维护并行变更观察，不重新盘点基线 |
| B 十模块六维核验 | in_progress | Platform/Server/Data/News/Research/Researchflow/Quant/LLM/Notification/WebUI 的六维证据尚未齐备；按模块与维度分批完成 |
| C 跨模块关系 | not_started | 真实 Port 调用图、授权链、依赖方向、Root Composition 与生命周期图 |
| D 差异/裁决/补证唯一登记 | in_progress | 本矩阵建立后，继续把已有 CTR 与十模块其他差异纳入唯一登记；不得新建平行清单 |
| E 责任模块修复/补证 | pending_audit | 按正式责任模块处理；审计分支不改业务实现 |
| F 第一轮完整性 Review | in_progress（仅 WebUI 维度 2 局部） | 先完成 W2-A～W2-E；其他模块/维度仍未完成第一轮 Review |
| G 第二轮正确性 Review | not_started | 仅在相关模块第一轮覆盖完成后执行 |
| H 第三轮冻结条件 Review | not_started | 仅在裁决、实现和测试证据齐备后执行 |

## 7. 全项目冻结门槛

项目级冻结至少要求：
- 十模块六维核验均有生产方、消费者、Schema/Golden/Contract Test、错误诊断、授权、生命周期和依赖边界证据。
- 跨模块调用图、授权链、依赖图完成双向核对。
- 所有 P0 已解决；所有 P1 已修复并有测试证据，或有明确、经批准的延期决议。
- P2 有明确责任人、关闭条件和延期决议；不能把未裁决项默认为接受。
- 正式契约决议已写入 `doc/contract/` 权威位置；审计工作区不能代替决策。
- 第一轮完整性、第二轮正确性、第三轮冻结条件 Review 分别完成并有独立结论。
- 固定基线以外的并行分支变更，只有触及契约边界时才做增量复核；不得悄悄混入固定基线。

## 8. 执行纪律

- 每轮完成一个完整工作批次，不再按单个 CTR 拆轮。
- 每轮结束更新本矩阵和总进度，记录实际减少了哪些未完成项。
- 若没有新证据，不重复读取同一批代码；直接推进下一项尚未完成工作。
- 不新增 CTR，除非证明现有编号无法表达且与所有已有问题不重复。
- 固定审计基线不变；不检查 Data 新分支，不合并四个并行修改分支。


## 9. W2-A Task/TaskEvent 批次 Review 结果（2026-11-10）

本批一次性核对 CTR-006/007/008 的 Go DTO、HTTP Handler、OpenAPI、队列读取逻辑和现有 Handler 测试；不重复 Analysis Run/Task 的其他部分。

| 编号 | 固定基线事实 | 性质与冻结影响 | 关闭条件 |
|---|---|---|---|
| CTR-006 | `taskqueue.Task.StartedAt`、`FinishedAt` 为 `*time.Time` 且带 `omitempty`；OpenAPI Task Schema 将二者声明为非 required 且 `nullable: true` | Go nil 实际省略字段，OpenAPI允许显式 null；序列化语义不一致。P2，Task 响应 Schema 字段语义未冻结 | 在权威契约明确“省略”或“null”之一；DTO tag、OpenAPI、JSON Golden 三方一致，并覆盖 queued/running/terminal 状态 |
| CTR-007 | `domain.StageEvent` 字段为 run_id、stage、status、at、duration_ms、message、attributes；status 有 started/succeeded/failed/skipped 四值；时间为 time.Time；可选字段有 omitempty。OpenAPI TaskEvent.event 仅声明 `type: object` 并用描述指向领域结构体 | 事件载荷没有字段级 Schema、required、枚举和 date-time 约束。P2，Task Events 消费者契约不完整 | OpenAPI独立定义 StageEvent Schema，明确必需字段、状态枚举、date-time、可选属性 map 类型；Golden/Schema 测试与 Go JSON 一致 |
| CTR-008 | Handler 默认 after_sequence=0、limit=100，拒绝负 cursor、limit 非整数或不在 1..500；Queue 查询 `analysis_stage_events` 中同 run_id 且 `id > after`，按 id ASC、LIMIT 返回；响应 next_after_sequence 为最后一条返回事件 sequence，无事件时保持传入 cursor | API 增量读取算法清楚；但事件序号实际取数据库 `id`，当前已核文件未包含该表 DDL/索引/自增约束，不能仅凭查询语句证明持久化单调性、唯一性及跨重启稳定性。现有 HTTP 测试只用 fake service 验证 after=4 返回 sequence=7，未覆盖真实 DB 分页、重复读取、空页 cursor 保持和边界 limit。P2，分页契约与持久化证据不完整 | 补充权威 migration/DDL 证据，确认 id 唯一且单调分配、不会重用；明确 cursor 是不透明事件序号、exclusive after；补数据库集成测试覆盖多页、空页、边界、重启后续读和并发追加；OpenAPI补充默认值及 next cursor 语义 |

### 本批测试证据边界

- `internal/platform/httpapi/tasks_test.go` 有 `TestGetTaskEventsSupportsIncrementalSequence`，仅证明 Handler 将 cursor 传给 Service，并按返回最后一条事件更新 next cursor；fake service 在内存中过滤，不验证数据库真实排序和持久化。
- `internal/platform/taskqueue/queue_test.go` 当前可见测试覆盖 ID 生成格式、队列关闭、worker 生命周期等；未发现真实数据库事件分页测试。
- `internal/platform/taskqueue/queue.go` 使用 `SELECT id,event_json ... WHERE run_id=? AND id>? ORDER BY id ASC LIMIT ?`，但本轮在固定基线仓库树中未找到 `analysis_stage_events` 的建表 DDL；不能把代码依赖的 id 属性当成已验证的数据库约束。
- OpenAPI 对 after_sequence 的 minimum/default 和 limit 的 minimum/maximum/default 已有表达；缺失点不是参数完全未定义，而是 cursor 语义、事件载荷 Schema、持久化约束和真实 DB 回归证据。
- 本批未执行 CI，也未修改业务代码、OpenAPI、DTO 或测试；只登记审计结论。


## 10. W2-B DailyBar Bundle lookback_days 批次 Review 结果（2026-11-10）

| 观察点 | 固定基线事实 | 判断 |
|---|---|---|
| WebUI 默认与偏好 | `web/index.html` 的个股行情日 K 选择默认 120；设置页允许 60/120/250，说明文字明确作为下一次新查询的默认条数。个股查询流程将当前 limit 夹在 20..2000 后作为 `lookbackDays` 显式传给 Bundle | WebUI 用户偏好是页面请求窗口的实际来源；请求不依赖服务端缺省值 |
| API Client | `getMarketBundle` 默认 `lookbackDays=120`，并始终序列化 `lookback_days`；但 `web/tests/api.test.mjs` 只用该函数验证 timeout/AbortSignal，没有直接断言默认 120、显式值及 URL 编码 | 默认值存在，但缺直接请求契约测试 |
| HTTP Handler | `server.bundle` 未收到参数时传 0 给 Application；收到参数时校验不低于 20 且不超过配置的 maxBars | Handler 不自行固定 120；省略值委托服务层 |
| Service/配置 | `DataService.Bundle` 对 lookback<=0 使用 `s.defaultBars`；配置 `MARKET_DATA_DEFAULT_DAILY_BARS` 默认 120，`MARKET_DATA_MAX_DAILY_BARS` 默认 1000（允许范围由配置校验约束） | 服务端默认值是部署可配置值，不是稳定公共常量 |
| OpenAPI | Bundle 的 lookback_days schema 有 minimum=20、maximum=2000，但没有 default，也没有说明省略时使用服务端配置 | 缺少省略行为描述；不应机械写死 default=120，因为部署可配置且当前默认 max 配置为1000 |
| 测试 | OpenAPI contract test 只断言 type/minimum/maximum；Handler bounds test覆盖低于20；API Client无默认/显式参数URL断言；未见Bundle handler省略参数时使用配置默认值的专项HTTP测试 | 参数边界有部分测试；缺配置缺省链路和Client URL回归 |

### CTR-009 结论

- **保留 P2**：不是已证实的线上请求窗口错误，而是省略参数的公共契约描述不完整、客户端默认值缺直接测试。
- WebUI 正常路径显式传入用户偏好，服务端环境默认值不会覆盖用户设置；API Client独立调用省略选项时默认发送120；非WebUI调用方省略HTTP参数时则使用部署配置值。
- 不能把 OpenAPI 直接补成固定 `default: 120`。应描述“省略时使用服务端 `MARKET_DATA_DEFAULT_DAILY_BARS` 配置，默认部署值120”，并明确该默认可配置；同时记录服务端最大值受 `MARKET_DATA_MAX_DAILY_BARS` 配置控制。
- 关闭条件：OpenAPI明确可配置默认语义和有效边界；API Client测试覆盖默认120及显式lookback值的URL参数；HTTP Handler/Service测试覆盖省略参数走配置默认值与显式值优先；配置默认/最大值的约束测试保持一致。
- 该问题不单独阻断整个 WebUI 维度 2 的公共 Schema 冻结，但 Bundle 查询窗口语义在上述契约说明补齐前不标记为完整。
- 本批没有修改业务代码、OpenAPI、DTO 或测试，没有执行 CI；固定基线不变。

### 下一批

进入 W2-C：仅基于现有审计切片，将 CTR-011～037（排除已单独处理的 CTR-010 聚合项及已由其他批次承接的编号）一次性归类为契约错误、实现偏差、产品范围待决、测试补证；不重新初审代码，不新增编号。
