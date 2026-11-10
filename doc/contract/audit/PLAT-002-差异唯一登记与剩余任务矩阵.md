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
| W2-C 其余已审数据集差异收敛 | CTR-011～037 | 本轮集中归类完成；均为 P2，保留活动项 | 既有证据已按契约/schema、消费者实现、数据语义/来源证据、产品范围、测试补证归类；不重复初审。各项冻结影响与关闭条件见第11节 |
| W2-D CTR-010 语义裁决准备 | CTR-010 | 项目级缺失/null/零值规则已在 GOV-CTR-001 标记 accepted；124 字段的逐字段适用条件仍待领域证据映射 | 已明确统一规则及逐字段裁决模板；不得仅凭 omitempty 推导删 nullable。下一步由字段所属契约责任方提供适用条件证据，不在审计分支实施修复 |
| W2-E 第一轮完整性 Review 收尾 | WebUI 维度 2 全部活动 CTR | complete_with_open_items | 数据集覆盖、API 消费映射、编号唯一、责任模块、关闭条件和遗留证据均已落档；活动CTR仍开放，不代表修复、CI通过或契约冻结。详见第13节 |

注：CTR-011～037 是收敛工作范围，不代表每项均阻断冻结。已在独立切片中确认的事实不得重做；如发现编号映射或状态冲突，只登记差异，不重新跑全链路。

## 6. 十模块 PLAT-002 项目级剩余主线

| 阶段 | 当前状态 | 剩余工作 |
|---|---|---|
| A 基线与范围 | complete（固定 SHA、十模块路径、文件规模已建立） | 只维护并行变更观察，不重新盘点基线 |
| B 十模块六维核验 | in_progress | Platform/Server/Data/News/Research/Researchflow/Quant/LLM/Notification/WebUI 的六维证据尚未齐备；按模块与维度分批完成 |
| C 跨模块关系 | not_started | 真实 Port 调用图、授权链、依赖方向、Root Composition 与生命周期图 |
| D 差异/裁决/补证唯一登记 | in_progress | 本矩阵建立后，继续把已有 CTR 与十模块其他差异纳入唯一登记；不得新建平行清单 |
| E 责任模块修复/补证 | pending_audit | 按正式责任模块处理；审计分支不改业务实现 |
| F 第一轮完整性 Review | in_progress（WebUI 维度2已 complete_with_open_items；其他模块/维度待覆盖） | W2-A～W2-E已完成；继续对其余模块/维度执行第一轮完整性Review |
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


## 11. W2-C CTR-011～037 既有差异集中归类（2026-11-10）

### 11.1 分类口径与总体判断

本节只汇总既有独立审计切片，不重新读取固定基线业务代码，不新增CTR。分类是主要责任/关闭路径；同一CTR可同时需要契约、实现和测试证据，但不得拆号。

- **契约/schema**：公共描述、required、枚举、参数或单位契约本身不准确/不完整。
- **消费者实现**：服务端已提供可用语义，但WebUI未正确消费，或分页/展示逻辑未维持请求语义。
- **数据语义/来源证据**：跨Provider等价性、交易日完整性、时间新鲜度、历史PIT等证据不足。
- **产品范围待决**：能力是否需要在WebUI暴露、当前页面是否需要消费，须由产品范围决定，不以“没有页面”直接推定接口错误。
- **测试补证**：已有行为或Schema缺少能防止回归的测试；仅在影响契约可信度/关闭门槛时保留，不单独扩张问题数量。

本批CTR均为P2，没有证据将其升级为P0/P1。它们不构成“全部阻断公共契约冻结”的同质集合：Schema错误阻断对应端点/DTO的准确冻结；产品范围和页面消费者缺口不自动阻断服务端DTO冻结；数据语义缺证则相关指标不得被消费者作强语义解释。WebUI维度2第一轮完整性Review可在责任、证据和关闭条件明确后结束，不以所有P2已修复为前提。

### 11.2 唯一归类表

| CTR | 主题 | 主分类 | 冻结影响判断 | 收敛/关闭条件 |
|---|---|---|---|---|
| 011 | PriceLimitRuleSnapshot required/枚举不完整 | 契约/schema + Golden补证 | 对应DTO Schema不能按现状准确冻结 | OpenAPI required/枚举与Go字段语义一致；纳入模型契约登记并补完整JSON Golden |
| 012 | StockBoards未消费total_is_lower_bound | 消费者实现 | 不阻断服务端DTO冻结；页面总数语义不可按确定总数展示 | WebUI消费下界标记，截断时明确呈现下界/不完整覆盖；补页面回归 |
| 013 | Fundamentals未暴露as_of PIT查询 | 产品范围待决 | 不阻断当前DTO冻结；历史时点功能范围未决 | 决定是否将PIT查询纳入WebUI；若纳入，再定义输入、交互及请求测试；若不纳入，记录明确范围决议 |
| 014 | Announcements跨页日期窗口/集合稳定性不足 | 消费者实现 + 分页契约 | 可能混合不同日期窗口的数据，相关分页行为未完整冻结 | 后续页复用首屏解析出的日期边界；定义稳定集合/去重策略并补跨自然日分页测试 |
| 015 | Valuation扩展指标nullable语义不一致 | 契约/schema | 对应字段Schema不能按当前Go省略行为准确冻结 | 按统一缺失/null决议修正OpenAPI或实现；补nil/非nil序列化覆盖。字段级语义纳入CTR-010统一裁决时不得重复计数 |
| 016 | Chip Distribution跨Provider average_cost/集中度等价性不足、provider_date描述不准 | 数据语义/来源证据 + 契约描述 | 相关指标不得宣称跨Provider可比；日期描述需准确 | 提供算法/字段来源与口径证据；无法证明等价则标记Provider限定；修正provider_date语义说明 |
| 017 | Eastmoney筹码算法除权窗口可能混合基准 | 数据语义/算法正确性 + 测试补证 | 相关算法输出在除权窗口缺少正确性证明 | 明确复权基准与除权窗口处理；补边界测试和参考Golden，验证前不宣称算法正确 |
| 018 | 个股资金流5/10日汇总未证明交易日窗口完整 | 数据语义/覆盖完整性 | 汇总值不能解释为已验证连续N交易日 | 按交易日历核验窗口完整性；缺日时降级/拒绝形成完整窗口结论；补缺日测试 |
| 019 | 个股资金流金额字段nullable不一致 | 契约/schema | 对应字段Schema不能按当前Go省略行为准确冻结 | 与CTR-010缺失/null统一决议保持一致，逐字段核对Go/OpenAPI/Golden；不得重复登记字段级CTR |
| 020 | 市场资金流14个omitempty字段nullable不一致 | 契约/schema | 对应字段Schema不能按当前Go省略行为准确冻结 | 与CTR-010统一决议保持一致，完成字段级Schema/序列化对照，不另建字段编号 |
| 021 | 市场资金流net_amount与四类金额恒等关系未获来源定义支持 | 数据语义/来源证据 | 消费者不得自行重算或断言恒等关系 | 获取权威字段定义或Provider证据；未证明前保留独立字段、禁止推导 |
| 022 | 市场资金流日期区间未按交易日历证明无缺日 | 数据语义/覆盖完整性 | 结果不能被解释为完整交易日区间 | 对预期交易日集合做覆盖核验；明确缺日处理和质量状态，补交易日历边界测试 |
| 023 | MarketSession有Client但页面只展示交易日历 | 产品范围待决 | 不阻断MarketSession DTO冻结；页面消费价值/范围未决 | 决定市场活动页是否展示Session阶段；纳入则补页面请求/DOM测试，不纳入则记录产品范围 |
| 024 | 集合竞价阶段粒度不足以判定订单行为 | 产品/契约边界 | 当前无下单功能，不构成现有页面或DTO冻结阻断；必须防止消费者误用 | 明确仅描述市场阶段、不代表可下单/订单状态；不扩展订单语义 |
| 025 | MarketBreadth总体分母覆盖停牌/未返回证券证据不足 | 数据语义/来源证据 | 总体比例/数量不得被解释为已证明全市场完整覆盖 | 明确分母定义与证券全集来源；证明覆盖或将结果标记为估算/部分覆盖并保持质量降级 |
| 026 | MarketBreadth情绪辅助池缺少完整日期证据仍参与计数 | 数据语义/实现边界 | 无法证明同日数据时不得混入当日情绪统计 | 辅助池计数前校验日期与快照归属；证据缺失时排除或降级并解释原因 |
| 027 | MarketIndices多指数快照缺统一时间跨度/最大陈旧度 | 数据语义/质量约束 | 不得将时间差异较大的指数集合宣称为同一时点快照 | 定义最大年龄和跨指数时间跨度阈值；超限时降级/排除并补边界测试 |
| 028 | Index Catalog Tushare标签与实际Provider能力不一致 | 契约/能力映射 | 能力声明不准确，目录标签不能按当前语义冻结 | 使标签、能力注册和Provider实现一致；不支持则移除或明确标记不可用 |
| 029 | Index Catalog缺稳定排序契约与回归测试 | 契约/schema + 测试补证 | 顺序未定义时消费者不得依赖当前偶然顺序 | 定义稳定排序键/次级键并补确定性测试 |
| 030 | Index Catalog Client无页面消费者及请求/DOM测试 | 产品范围待决 + 测试补证 | 不阻断服务端DTO冻结；是否需要目录页面待定 | 决定是否纳入WebUI页面；纳入则补请求和DOM测试，否则记录预留接口决议 |
| 031 | Index Constituents历史权重缺最大陈旧度策略/专项诊断 | 数据语义/质量约束 | 过期权重不能无提示地作为历史成分依据 | 定义最大陈旧度、超限质量状态和诊断reason；补陈旧/缺失边界测试 |
| 032 | Index Constituents历史名称使用当前证券目录，缺PIT证据 | 数据语义/PIT证据 | 历史名称不能保证时点正确，不得声称历史名称还原 | 提供历史名称PIT来源或明确返回当前名称；Schema/页面需标识其语义 |
| 033 | Index Constituents Client无页面消费及DOM测试 | 产品范围待决 + 测试补证 | 不阻断服务端DTO冻结；页面是否需要指数成分展示待定 | 产品决议是否建设页面；纳入则补请求/DOM测试，否则记录接口预留范围 |
| 034 | Index Daily Bars未按交易日历核对预期交易日 | 数据语义/覆盖完整性 | 不能证明请求区间无漏日 | 以交易日历生成预期交易日集合并核对；缺日时明确质量状态，补停牌/节假日/缺日测试 |
| 035 | Index Daily Bars跨Provider成交量/额单位说明不完整 | 契约/单位语义 | 未明确单位的跨Provider值不可安全比较/换算 | OpenAPI完整说明各Provider单位与归一化规则；未知单位保持不可比较，补单位Golden/测试 |
| 036 | Index Daily Bars Client无页面消费者及DOM测试 | 产品范围待决 + 测试补证 | 不阻断服务端DTO冻结；是否需要指数历史行情页面待定 | 产品决议是否建设页面；纳入则补请求/DOM测试，否则记录预留接口范围 |
| 037 | HotRankingItem.change_percent描述错误引用has_data约束 | 契约/schema | OpenAPI字段描述错误，相关Schema不能准确冻结 | 修正change_percent语义描述，补Schema断言；其omitempty nullable字段仍归CTR-010聚合项 |

### 11.3 去重与冻结门槛

1. CTR-015、019、020只保留各自原始业务差异；它们涉及的Go omitempty/OpenAPI nullable字段语义统一由CTR-010集中裁决，不能在CTR-010之外再次统计为新增字段问题。
2. CTR-011、028、029、035、037主要属于公共Schema/能力/单位契约准确性；对应Schema或能力声明修正前，不对受影响端点宣称完整冻结。
3. CTR-012、014、016～018、021～022、025～027、031～032、034属于消费者行为或数据语义证据问题；在关闭前必须限制下游解释，不允许把不完整、不可比或非PIT结果包装成确定结论。
4. CTR-013、023、024、030、033、036属于产品范围/能力边界决策；不因页面尚未消费而直接判定服务端API错误，也不自动阻断服务端DTO冻结。必须留下纳入或不纳入的明确决议。
5. CTR-029及CTR-030/033/036的测试缺口不另建CTR；只作为相应契约或产品范围差异的关闭证据。
6. 本批全部保持P2。当前没有依据把任何一项升级P0/P1；也不能因均为P2就全部视为无冻结影响。
7. 本批集中归类完成后，不再对CTR-011～037做第二轮相同归类。后续只接收正式裁决、责任模块新提交或测试证据，做增量验收。

### 11.4 W2-C 完成与下一批

W2-C 的既有差异集中归类完成。下一批进入 W2-D：只针对CTR-010形成缺失/null/零值的权威语义裁决问题清单，明确需要治理层回答的决策，不修改DTO、OpenAPI、Golden或业务代码。随后执行W2-E第一轮完整性Review收尾；维度2结束后回到PLAT-002十模块六维核验主线。


## 12. W2-D CTR-010 缺失/null/零值语义裁决准备（2026-11-10）

### 12.1 权威规则核对结果

本批对照固定基线 doc/整体任务规划/02-契约治理/跨模块契约目录与冻结状态.md 的 GOV-CTR-001，以及 doc/contract/README.md、doc/contract/PLAT-002-项目级契约冻结登记簿.md 和既有 DEC 记录。

GOV-CTR-001 已标记为 accepted，其公共规则不是待重新发明的候选方案：

1. **字段缺失**：仅表示 optional 字段不适用于该 DTO 版本/请求；不得用缺失表达“查询过但未知”。
2. **显式 null**：表示字段存在但当前无可提供值；required + nullable 字段必须输出 null，不得使用 omitempty。
3. **零值**：是真实业务值，必须与缺失/null区分；不得因数值为0而省略（具体字段是否允许0仍由领域约束定义）。
4. **unavailable**：是能力/依赖状态，不是0或null的同义词；应由对应状态/质量语义表达。
5. **字符串**：不得用空字符串表达缺失/未知。
6. **数组**：已确认无结果时使用空数组；未执行/未知按状态表达。
7. nil slice、空数组、null、字段缺席、零值必须分别进入 Golden。

因此，项目级通用语义已有 accepted 决议；CTR-010未解决的重点不是再次投票选择 optional + non-nullable，而是**124个字段逐项适用性与无值原因的领域映射**。

### 12.2 对 CTR-010 的准确解释

固定基线清单仍为 124 个唯一字段、29 个 DTO、54 个响应 DTO。每个字段当前 Go JSON tag 带 omitempty，对应 OpenAPI 属性声明 nullable: true；nil 时 Go JSON 实际省略。该静态事实不能证明字段在“适用但当前无值”时也应省略。

按 GOV-CTR-001，字段必须先回答以下问题：

- 字段对当前 DTO 版本/请求是否适用？若不适用，才允许省略。
- 字段适用但当前无可提供值？应显式 null；若为 required + nullable，必须始终输出字段。
- 字段有真实数值且值为0？必须保留0，不得因 omitempty 省略。
- 无值实际代表 Provider/依赖/能力不可用，还是仅该记录没有该项数据？前者必须使用 unavailable/quality/diagnostic 状态，不能只靠null表达。
- 字段是否有条件约束（Provider能力、市场/证券类型、日期范围、DTO版本或请求参数）？必须由字段所属领域契约明确条件，不能靠字段名推断。

### 12.3 逐字段裁决模板

后续责任方对124个字段只需补齐以下信息，不重新盘点或新建CTR：

| 项目 | 必须回答的问题 |
|---|---|
| 适用条件 | 哪些 DTO 版本、请求参数、Provider/证券/市场场景下字段适用？ |
| 不适用表达 | 是否允许省略？触发省略的明确条件是什么？ |
| 适用但无值 | 必须输出 null，还是该字段本身不允许无值？理由/来源是什么？ |
| 合法零值 | 0 是否为有效业务值；若有效，如何保证序列化保留？ |
| 不可用状态 | 是否另有 quality_status、reason_code、provider capability 等状态表达？ |
| 生产方证据 | Go DTO、Adapter/Provider 赋值路径及权威字段定义 |
| 消费方影响 | WebUI/其他消费者是否区分 absent、null、0、unavailable |
| 测试证据 | 覆盖适用有值、适用无值、条件不适用、合法零值；适用时覆盖 unavailable 与空数组/null/缺失边界 |

### 12.4 当前可作出的结论与不可越权结论

- **可以确认**：GOV-CTR-001 的公共规则已 accepted；CTR-010 的 optional + non-nullable 不能作为全局修复方案直接执行。
- **可以确认**：如果字段适用但值未知/无可提供值，不能仅因 Go 当前 omitempty 就把缺失解释成未知；若该字段契约规定 required + nullable，则必须显式输出 null。
- **不能确认**：124个字段是否全部应改为 required + nullable，或其中哪些字段确实只在部分 DTO/请求条件下适用。现有统计只证明 Go tag/OpenAPI nullable 差异，不包含完整字段适用条件裁决。
- **不能确认**：是否需要统一修改 Go DTO。应先逐字段确认目标契约，再由对应生产方对齐实现；不得把审计准备任务变成 Data 分支改造任务。
- **不能宣称**：CTR-010 已关闭、124字段已完成语义裁决、契约已冻结或 Golden 已补齐。

### 12.5 W2-D 状态

W2-D 的“权威规则核对与裁决问题准备”完成；CTR-010 本身仍开放。下一步 W2-E 进行第一轮完整性 Review 收尾时，将该未决字段映射明确列为遗留证据项和责任模块输入，不再重复做本次规则核对。后续只在责任方提供逐字段适用条件/权威来源或新决议后增量更新。


## 13. W2-E 第一轮完整性 Review 收尾（2026-11-10）

### 13.1 覆盖与索引一致性

- WebUI维度2的数据集审计切片覆盖已完成；唯一矩阵第3节列出完整范围。此前API盘点第5节中仍把Hot Rankings、Market Sentiment History、Limit Pool、Dragon Tiger、Stock Search、Analysis Run、Provider Probe History列为待核，是索引未更新，不是实际审计缺失；已在API盘点第7节纠正状态。
- API Client盘点保持44个导出函数、41个远端入口、40个主要页面模块引用；唯一未发现主要页面消费者的是getMarketSentiment，继续作为产品范围待确认项，不新增CTR。
- Market Data路由/OpenAPI入口盘点已完成；Analysis Run GET/Events缺失OpenAPI由CTR-046承接。健康检查、诊断路由访问策略属于维度3/Server交叉核验，不混入维度2。
- 编号去重规则：CTR-003由046承接，CTR-005由047承接；CTR-041撤销；CTR-010为跨DTO聚合CTR；CTR-038与CTR-048是不同接口的独立差异；CTR-001/002属于维度1。没有新增编号。

### 13.2 活动差异的唯一主责模块

以下为基于已记录问题发生层确定的**技术主责模块**，用于后续派发和验收；协作模块不改变唯一主责。产品范围/项目级语义裁决仍由项目负责人确认，不能以技术模块归属替代决议。

| CTR | 技术主责模块 | 主要协作/决策方 |
|---|---|---|
| 004 | WebUI | — |
| 006、007 | Server | Researchflow（领域事件结构/状态来源） |
| 008 | Researchflow | Server（HTTP cursor契约） |
| 009 | Data | WebUI（Client参数测试） |
| 010 | Data | WebUI（消费者语义验证）；项目负责人裁决 |
| 011 | Data | Server（如经Root API转发时的Schema同步） |
| 012 | WebUI | Data（下界标记来源语义） |
| 013 | WebUI | Data（PIT能力已有部分服务端支持）；项目负责人确认产品范围 |
| 014 | WebUI | Data（分页集合/日期边界契约） |
| 015、016、017、018、019、020、021、022 | Data | WebUI（消费限制与展示边界） |
| 023 | WebUI | Data（MarketSession API能力）；项目负责人确认产品范围 |
| 024 | Data | WebUI（不得将阶段信息解释为订单行为） |
| 025、026、027、028、029 | Data | WebUI（质量与排序语义消费） |
| 030 | WebUI | Data（Index Catalog API）；项目负责人确认产品范围 |
| 031、032 | Data | WebUI（历史权重/名称语义提示） |
| 033 | WebUI | Data（Index Constituents API）；项目负责人确认产品范围 |
| 034、035 | Data | WebUI（缺日与单位语义消费） |
| 036 | WebUI | Data（Index Daily Bars API）；项目负责人确认产品范围 |
| 037 | Data | WebUI（字段消费） |
| 038、039 | Data | 项目负责人/契约责任方裁决total与最新记录选择语义 |
| 040 | WebUI | Data（Service防御性日期边界核验） |
| 042、043、044 | Data | WebUI（未知单位/质量与日期边界消费） |
| 045 | WebUI | Data（quality_status/quality_reasons公共语义） |
| 046、047 | Server | Researchflow（Run/Task领域语义与执行行为） |
| 048、049 | Data | WebUI（total语义消费）；项目负责人/契约责任方裁决 |

说明：CTR-006/007归Server是因为当前差异落在HTTP DTO/OpenAPI；CTR-008归Researchflow是因为cursor持久化与事件队列存储证据是核心关闭条件。CTR-046/047归Server是因为权威HTTP路由/OpenAPI/Handler请求响应契约是核心交付物。若后续正式责任分配与此不同，只允许根据责任模块明确认领/决议做增量更正，不重新审查代码。

### 13.3 第一轮完整性Review结论

**W2-E完成：WebUI维度2第一轮完整性Review为complete_with_open_items。** 完成范围仅包括：

1. 数据集审计覆盖与API Client/页面消费者映射有索引；
2. CTR编号唯一、别名承接、撤销项和聚合项口径明确；
3. 活动差异有唯一技术主责模块、必要协作方和关闭条件；
4. 未决产品范围、契约裁决、实现修复和测试补证作为遗留事项保留；
5. 过期API盘点状态已校正，后续不再以旧待核列表启动重复审查。

仍开放且不得从矩阵移除的事项包括：CTR-043 P1；CTR-010 124字段逐项适用条件/无值原因证据；CTR-006～009、011～040、042～049各自未完成的裁决、实现、测试或产品范围决议；getMarketSentiment产品范围待确认。CTR-041保持withdrawn，CTR-003/005只作历史别名。

本结论不表示WebUI维度2公共契约冻结，也不表示项目级阶段F完成。阶段F仍需覆盖其余模块和维度；阶段B十模块六维核验仍是当前项目主线。

### 13.4 下一阶段执行边界

从下一轮开始停止WebUI维度2全量完整性Review。回到阶段B十模块六维核验，按既定十模块清单与六个维度推进；先从唯一矩阵/总进度中确认当前未完成模块和维度，选择一个模块的一个未完成维度做证据核验。不得重新读取已完成WebUI维度2数据集，不检查Data新分支，不改变固定代码审计基线。

## 14. Platform 维度2：Diagnostics Observation DTO 首批切片（2026-11-10）

- 审计记录：`Platform-维度2-DTO-Schema-Golden.md`。
- 固定基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`；只核 Platform `diagnostics.Observation` 一个 DTO 切片。
- 已核 Go DTO/JSON Tag/Validator、PLAT-CTR-004 JSON Schema、healthy/unavailable Golden、Go 契约测试、News/Research 模块级生产方及 Server Diagnostics/Health Handler 消费路径。
- 发现 PLAT2-D2-001～004：Go 字节长度与Schema字符长度不等价；空白字符串语义不等价；attributes总字节预算未在Schema表达；现有测试未证明Schema与Go对同一边界样例双重校验。以上均并入既有PLAT-CTR-004证据缺口，不新增CTR。
- PLAT-CTR-004保持 `pending_evidence`，不可因Go Golden测试存在而标记frozen。Server完整Diagnostics Report Schema/HTTP Golden转由Server维度2核验，不在Platform切片越界补建。
- 本切片状态 `partial`；Platform维度2仍为partial。本轮只完成Observation一个DTO，不代表Fault、Request ID、HTTP Client等其他公共结构完成核验。
- 下一切片：Platform维度2核对Fault内部对象与领域/Transport映射边界；Server完整HTTP Error Envelope留在Server维度2。
- 本轮仅新增/更新审计文档，未修改业务代码、Schema、Golden、测试，未执行CI；固定代码审计基线不变。
## 15. Platform 维度2：Fault 内部对象与跨模块映射核验（2026-11-10）

- 审计记录：`Platform-维度2-DTO-Schema-Golden.md` 第7节；固定代码基线 `b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- Fault所有字段均为 `json:"-"`，直接JSON序列化为 `{}`；这是内部对象边界的预期行为，不应为Fault本身创建公开Schema/Golden。
- Class、Code、SafeMessage、Dependency、Retryable、Cause/Unwrap的实现和本地测试基本覆盖；Platform Diagnostics Runner会Validate并归一化非法Fault。
- 已确认LLM `ModelErrorFromPlatformFault` 是跨模块直接消费者：映射Class/Code/SafeMessage/Retryable，排除Dependency/Cause；现有测试覆盖合法映射及敏感字段排除。
- PLAT2-D2-005：LLM Mapper投影前不调用Fault.Validate，缺少非法Class/Code、控制字符SafeMessage、非法Retryable组合的映射测试。作为证据缺口并入既有P002-D02 / PLAT-CTR-001，不新增CTR；由LLM Owner补信任边界说明及测试，必要时对非法Fault安全降级。
- Market Data与Platform HTTP当前使用独立HTTP Error DTO/映射路径，未发现直接消费Platform Fault的证据；不能用它们的错误Envelope测试替代Fault消费者测试。
- 本切片结论：Fault内部对象契约基本齐备；跨模块投影仍partial。P002-D02继续开放，PLAT-CTR-001保持pending_evidence。
- 下一切片：Platform维度2核对Request ID公共原语与直接Transport消费者；Server完整HTTP Error Envelope留在Server维度2。
- 本轮仅更新审计文档，未修改业务代码、Schema、Golden、测试，未执行CI。
## 16. Platform 维度2：Request ID 原语与传播边界核验（2026-11-10）

- 审计记录：`Platform-维度2-DTO-Schema-Golden.md` 第8节；固定代码基线 `b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- Request ID Header/格式/最大长度/生成/Context/出站传播规则由 `internal/platform/observability/request_id.go` 实现；Golden `PLAT-CTR-002-request-id.golden.json` 被 Contract Test读取，单测覆盖合法/非法/重复Header、默认不信任、显式可信入口、出站覆盖和无Context删除。
- Root Server Bootstrap在路由外层使用 `MiddlewareWithOptions` 零值安全策略，直接可达时重建客户端ID；旧Analysis Handler内部仍调用Deprecated `Middleware`，但当前组合根外层已先生成可信ID。独立暴露旧Handler仍受可信Gateway前提约束。
- 缓冲和流式Platform HTTP Client均从Context传播ID；Research Correlation ID使用独立Context Key，不从Request ID继承。GOV-CTR-003涉及originating_request_id/Worker/Event完整链路，不在本切片关闭。
- PLAT2-D2-006：缺少真实Root Server入口到出站Platform HTTP Client的端到端组合测试，属集成证据缺口，不是已证实实现错误。由Server/Integration Owner补证。
- P002-D03继续开放：Root Server当前入口接线符合安全默认，但Deprecated兼容入口的生产暴露路径和迁移/移除仍需部署证据。并入既有P002-D03，不新增CTR。
- 本切片结论：Platform Request ID原语与局部传播测试基本齐备，组合链路partial；PLAT-CTR-002不能仅凭包级测试标记frozen，GOV-CTR-003保持独立pending_evidence。
- 下一切片：Platform维度2核对HTTP Client配置、DestinationPolicy、缓冲/流式结果元数据及PLAT-CTR-005 Golden/Contract Test；不重审SSRF算法，不审Server LLM Transport业务映射。
- 本轮仅更新审计文档，未修改业务代码、Schema、Golden、测试，未执行CI。
## 17. Platform 维度2：HTTP Client配置与缓冲/流式结果契约核验（2026-11-10）

- 审计记录：`Platform-维度2-DTO-Schema-Golden.md` 第9节；固定代码基线 `b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- 已核Config默认值/上限、两套Dependency并发池、Map/策略切片复制、DestinationPolicy、BoundDoer、缓冲Do结果所有权、StreamResponse/StreamStats及News/LLM直接消费者。
- 代码实现覆盖缓冲/流式配置和生命周期；A1专项文档列出21项流式验收矩阵，相关单测覆盖增量读取、取消/Close、EOF、截断、Body/响应头上限、解压后字节、空闲超时、并发隔离和统计。
- PLAT2-D2-007：CTR测试未完整断言MaxResponseHeaderBytes与流式默认值/上限；StreamDependencyLimits缺少非法值、条目数和Map复制的专门配置测试。代码已有默认/校验实现，登记为回归测试证据缺口，归入PLAT-CTR-005，不新增CTR。
- PLAT2-D2-008：Server LLM Transport将StreamResponse映射为LLM窄响应时不暴露StreamStats，LLM领域层无法记录Platform传输终态/字节数/耗时。属条件性诊断需求确认项；由LLM/Server Owner确认A2是否要求，若不要求记录不适用理由，若要求则通过LLM自有安全DTO映射，不让领域层依赖Platform内部统计类型。
- PLAT-CTR-005内部Go API不经JSON序列化，故JSON Schema/Golden原则上不适用；以版本化Go API、Contract Test、消费者适配测试作为替代证据。但项目登记簿需明确该不适用理由和替代证据，不得虚构Golden或误报缺失JSON Golden为实现失败。
- News Provider/Source和LLM Transport均使用BoundDoer固定策略；News Fault Mapper验证Fault并剔除Cause；LLM Fault Mapper校验差异已在PLAT2-D2-005登记，本轮不重复。
- 本切片结论：Platform HTTP Client A1实现及局部测试基本齐备；配置回归覆盖、LLM统计可见性决策、A2真实消费者联调和A3生产网络/Secret/Runtime证据仍待补。PLAT-CTR-005保持pending_evidence。
- 本轮HTTP Client切片完成后，暂不转Server维度2。项目Platform契约目录中的PLAT-CTR-003（身份/授权）、PLAT-CTR-006（Storage/Migration）、PLAT-CTR-007（Typed Config/Secret）尚未完成维度2适用性核验。下一切片先核PLAT-CTR-003的身份/授权结构、Header/Context、序列化适用性和直接消费者；不重审权限算法/授权语义（维度4）。完成Platform维度2剩余契约适用性清点后，再转Server维度2。
- 本轮仅更新审计文档，未修改业务代码、Schema、Golden、测试，未执行CI。

## 18. Platform 维度2：PLAT-CTR-003 身份、授权与 Secret 边界（2026-11-10）

- 审计记录：`Platform-维度2-DTO-Schema-Golden.md` 第10节；固定代码基线 `b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- 对照项目契约登记簿原始PLAT-CTR-003：其批准范围是anonymous、owner Bearer、service Bearer；不得将Owner Web Session静默并入该CTR。
- Platform `security.go` 已实现IdentityKind/Principal、Owner与Service凭证摘要、32～256字节URL-safe Token约束、SHA-256摘要和常量时间比较、固定Scope白名单、拒绝重复/未知Scope和Service platform:admin、不可变Authenticator快照、Context Principal切片隔离、显式RoutePolicy和默认拒绝。Secret格式化/序列化脱敏有测试。
- Server Bootstrap真实消费Platform Authenticator，但当前只调用 `NewAuthenticator(OwnerToken, nil)`，仅装配Owner Bearer；Router当前路由显式匿名/Owner策略，没有Service路由。不能将Platform包支持Service Scope视为Service生产链路已接入。
- Principal/RoutePolicy等均为进程内Go结构，无公开JSON传输边界；不适用JSON Schema/Golden，以Go Contract Test、Router测试和消费者Context测试作为机器证据。Secret不得进入Golden/DTO。
- Owner Web Session的Platform原语虽已存在，但Server Bootstrap没有Session接入。SessionRecord作为Redis内部JSON持久化记录无独立schema_version/Golden的证据，归入既有P002-D12～D14及Server A2扩展验收，不扩大PLAT-CTR-003原范围。
- 本轮未完成全仓Principal/Scope消费者import扫描；领域资源授权、AuthorizationRef签发和读取时复核留在维度4。
- 本切片结论：Platform Bearer A1实现基本符合；Server Service凭证装配、真实服务间消费者证据和L3 Secret部署证据未齐，PLAT-CTR-003继续pending_evidence。Web Session另按独立扩展准入验收，不新增CTR或把Session并入Bearer契约。
- 本轮只更新审计文档，未修改业务代码、Schema、Golden、测试，未执行CI。
- 下一切片：Platform维度2核对PLAT-CTR-006 Storage/Repository/Migration相关结构的序列化适用性、Migration机器契约及直接消费者；不重审连接/事务算法（维度5），不重复本轮Security审查。

## 19. Platform 维度2：PLAT-CTR-006 Storage/Repository/Migration（2026-11-10）

- 详见 `Platform-维度2-DTO-Schema-Golden.md` 第11节；固定基线 `b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- Platform `SQLConfig`、`Migration` 与Server Registry输入是进程内Go API，不适用公开JSON Schema/HTTP Golden；Server RunReport虽有JSON Tag，固定基线仅用于Bootstrap日志，未发现公开HTTP DTO边界。应以Go Contract Test、注册清单测试和消费者测试提供机器证据。
- Platform Migration约束：namespace 1～64字节小写ASCII；1～1000条；Version非零唯一；Description最多255 Unicode字符；Statement非空且最多1MiB；按Version排序，checksum由有序SQL语句计算，历史版本必须为有序前缀。
- PLAT2-D2-009：Server Registry注册时未提前校验Description 255字符/Statement 1MiB边界，Runner最终会拒绝；登记校验阶段与执行阶段约束需统一或明确权威层。
- PLAT2-D2-010：checksum仅覆盖Statements，不覆盖Description；需裁决Description是否属于已发布Migration不可变定义的一部分，并补相应兼容测试。
- PLAT2-D2-011：固定基线存在Researchflow `Migrations()`（namespace researchflow），但Server Bootstrap `newMigrationRegistry` 只注册News/Research/Notification，遗漏Researchflow。作为Root Server直接消费者接线缺口登记，责任落在Server组合根；维度5再核对启动生命周期和修复。
- A2真实MariaDB/Redis集成测试受 `PLATFORM_STORAGE_INTEGRATION=1` 门控，本轮未执行；真实连接池、锁和关闭顺序证据留维度5/L3。
- 本轮未改实现、Schema、Golden、测试，未执行CI。
- 下一切片：Platform维度2核对PLAT-CTR-007 Typed Config/Secret的字段、序列化脱敏、消费者映射和机器测试适用性；不重审本轮Migration执行算法。


## 20. Platform 维度2 — PLAT-CTR-007 Typed Config/Secret（2026-11-10）

审计详情：`Platform-维度2-DTO-Schema-Golden.md` 第20节。固定代码基线不变。

| 差异 | 唯一归属 | 当前判断 | 关闭条件 |
|---|---|---|---|
| PLAT2-D2-012 | PLAT-CTR-007 | Market Data/Storage Adapter把Platform Secret.Value()转成普通公开Config字符串；存在类型级脱敏保护丢失风险，未证实实际日志泄漏 | 责任模块明确明文使用边界；保留Secret或限制窄化访问；Config格式化/JSON/slog脱敏测试或等效访问约束证据 |
| PLAT2-D2-013 | PLAT-CTR-007 | Python API与旧Market Data配置入口直接读取进程环境；调用方/活跃部署未确认，暂不判定为生产违规 | Server/迁移维度核实真实调用点、部署入口；活跃入口统一从Bootstrap注入Platform Env，非活跃入口标记兼容/退役 |
| PLAT2-D2-014 | PLAT-CTR-007 | Secret脱敏、配置快照和消费者默认/边界测试缺少完整消费者级证据 | 活跃消费者补Config输出脱敏、快照隔离、默认值/边界测试；非活跃入口提供不适用证据 |

本轮结论：
- Platform Env/Secret基础API及本体脱敏测试存在；Server Process和News Config使用Platform Env，News/Server保留Secret包装。
- Adapter Secret类型丢失和旧环境读取入口是消费者接入证据缺口；不新增CTR。
- Config均为进程内启动配置，不适用公开HTTP Schema/Golden；以Go Config Contract Test、Bootstrap注入测试和消费者安全测试替代。
- PLAT-CTR-007继续 `pending_evidence`；不代表Platform维度2整体冻结。
- 本轮仅更新审计文档，无业务代码、Schema、Golden、测试或CI变更。

### 下一步唯一任务

**Server — 维度2：DTO、Schema、Golden。** 从Root Server公开HTTP响应/请求DTO、真实Handler、OpenAPI、Golden和WebUI/内部消费者映射开始；不重复Platform维度2，不提前审查Server其余五个维度。


## 21. Server 维度2首轮切片：Root Health/Diagnostics 与旧 Analysis HTTP（2026-11-10）

审计详情：`Server-维度2-DTO-Schema-Golden.md`。固定代码基线仍为 `b6dfc85ba214fe16268c8cba780641a8bfe15086`。

| 观察/差异 | 唯一归属 | 当前判断 | 关闭条件 |
|---|---|---|---|
| SRV2-D2-001：Root /diagnostics复合Report、/readyz成功/失败响应没有Server专属Schema/Golden；Platform Observation Schema不能约束Server新增Readiness/Report包装 | SRV-CTR-001（Root HTTP契约）；嵌入Observation继续归PLAT-CTR-004 | P2证据缺口，不是已证实运行时字段错误；不新增CTR | 明确运维端点是否作为稳定契约；若保留，补Server Schema与healthz/readyz/diagnostics成功失败Golden/Handler Contract Test；若不做，登记不适用理由及替代测试 |
| Analysis Run OpenAPI覆盖不足 | CTR-046（GET Run/Events）+ SRV-CTR-001（Root API清单） | 既有差异；POST Run也尚未进入当前Analysis OpenAPI，随Root路由清单统一登记，不重复编号 | OpenAPI覆盖真实注册的Run路由、响应结构和错误状态；Go Handler与Schema/Gateway Contract Test一致 |
| CreateTask的AnalysisRequest Schema与RunRequest字段/约束不完整 | CTR-047 | 既有差异，不新增CTR | 对齐完整请求字段、required/default/枚举和未知字段拒绝语义 |
| StageEvent Schema仅为泛化object | CTR-007 | 既有差异，不新增CTR | 展开StageEvent字段、状态枚举、时间格式和可选字段语义 |
| Task started_at/finished_at的omitempty与nullable差异 | CTR-006 | 既有差异，不新增CTR | 明确省略或null并对齐Go/OpenAPI/Golden |
| Root错误Envelope存在publicError(class/code/message/retryable)与legacy error(code/message)两种形状 | PLAT-CTR-001（Server负责映射） | 安全消息已有收敛测试，但统一Envelope策略和全路由映射证据未完成；不新增CTR | 明确统一或按端点保留多种Envelope；同步状态码、OpenAPI、WebUI消费和Golden |

### 本批证据与边界

- Root Router实际注册Health/Readiness/Diagnostics、Root discovery、同步Run/Events、异步Task/Events共13条method-pattern；不把Market Data独立服务路由混入Server。
- Health Handler有fake checker测试，覆盖错误脱敏、Ready/Draining、依赖状态、GET/HEAD；缺完整Server Report Schema/Golden。
- 旧Analysis Handler有请求大小、Content-Type、未知字段、默认值、任务幂等、cursor和若干错误映射测试；测试以Fake依赖为主，未发现OpenAPI双向校验或稳定Golden。
- WebUI `web/src/api.js`消费Root /healthz、/readyz和error.message/code；Request ID读取X-Request-ID Header；未发现该文件消费/diagnostics。
- 本批只做固定SHA静态核验；无业务代码、OpenAPI、DTO、Golden、测试或CI改动。

Server维度2当前仅完成首轮一个切片，状态为 `in_progress / partial`；不得将其视为整个Server维度2完成。下一切片应继续Server当前真实公开的模块HTTP Handler/Transport消费者映射，先盘点是否有Root已注册模块路由；若固定基线没有新增模块路由，则对Server公开错误DTO与Analysis API现有差异做增量补证，不重复本批Health/Analysis初审。


## 22. Server 维度2第二切片收尾：错误 DTO 与消费者交叉核验

- 固定代码基线仍为 b6dfc85ba214fe16268c8cba780641a8bfe15086；只读该SHA，不检查Data新分支。
- Root Router没有新增模块Handler，仍为13条method-pattern；本轮不扩大路由范围。
- Root错误生产路径已交叉核验：Bootstrap Middleware/Fault为class/code/message/retryable；Security Guard、Health Handler、旧Analysis Handler主要为code/message；Analysis Run执行失败另带partial_result；声明超限与Handler解码超限的错误码不同。
- WebUI只消费message/code/status/requestId，不消费class/retryable；Request ID实际Body优先、Header其次，既有测试已验证Body优先。此前审计文档中“Header优先”的说法已在Server审计记录中纠正。
- Analysis OpenAPI没有统一Error Schema/错误响应JSON Schema。CTR-046（Run路由OpenAPI缺失）、CTR-047（CreateTask请求Schema）均为既有项，本批不重复登记。
- 错误Envelope统一建议：Root稳定错误响应逐步统一为嵌套Error DTO，保留code/message并明确class/retryable；实现前须由Server/项目责任方裁决覆盖端点、枚举、HTTP状态、retryable规则、超限错误码、Request ID权威通道及OpenAPI范围。审计分支不擅自改实现。
- 唯一归属：Envelope归PLAT-CTR-001；Root路由/运维契约归SRV-CTR-001；Request ID归PLAT-CTR-002；CTR-046/047保持原编号。无新增CTR。
- 本批状态：第二切片complete_with_open_items；Server维度2整体仍为in_progress / partial，不等于契约冻结。
- 本批仅更新审计文档，无业务代码、OpenAPI、DTO、Golden、测试或CI变更。
- 下一步：继续Server维度2，但只推进**Run/Task DTO字段与既有OpenAPI Schema的差异关闭证据**，以CTR-046/047、CTR-006/007的现有登记为唯一入口；不重新初审错误Envelope、Health/Readiness或WebUI基础错误解析。


## 23. PLAT-CTR-002 的 Root/旧Analysis 嵌套 Middleware 增量发现

- 本轮Server错误DTO交叉核验发现：Root Middleware的MiddlewareWithOptions会生成新Request ID并写响应Header/Context，但不会改写原始Request Header；旧Analysis Server.Handler仍使用默认信任入站ID的兼容型 observability.Middleware。
- 两层组合后，旧Analysis内层Middleware仍可能从原始X-Request-ID读取调用方提交的合法ID，覆盖Context和响应Header。故“Root外层先重建ID”不足以证明Analysis路由已隔离不可信入站ID。
- 影响限于Request ID可信边界、日志关联和响应关联一致性；不等同于认证绕过。该发现归既有PLAT-CTR-002 / P002-D03，不新增CTR，也不在本批修改实现。
- 关闭条件：Server组合路径移除/隔离旧兼容Middleware，保证内层只使用Root已验证Context；补Root完整组合测试，验证伪造入站ID不能覆盖最终响应Header、Handler日志Context和后续传播ID。
- 该增量证据不改变本轮错误Envelope结论；PLAT-CTR-002保持开放，后续按原唯一登记增量验收。


## 24. Server 维度2第三切片：Run/Task DTO 字段语义增量核验（2026-11-10）

本批只增量推进CTR-046/047/006/007，不重复CTR-008分页算法初审。

| 编号 | 本轮确认的字段级事实 | 当前状态与关闭条件 |
|---|---|---|
| CTR-046 | POST Run、GET Run、GET Run Events缺少OpenAPI路径；RunResult没有响应Schema。同步Run与Task共用RunRequest，但两者run_id行为不同；RunResult是多层嵌套DTO | 保持开放。补齐真实Root路径/状态/请求响应Schema，明确同步/异步run_id差异；为RunResult及Events提供完整Schema、Golden和Handler/Schema一致性测试 |
| CTR-047 | RunRequest JSON字段为run_id、task_type、as_of、as_of_mode、universe_id、symbols、portfolio_id、strategy_ids、market_id、strategy_config_version、risk_config_version、trading_rules_version、data_policy_version、parameters。现有AnalysisRequest缺run_id/as_of_mode/parameters；没有required、TaskType enum。Handler补默认值并覆盖as_of_mode；历史回放要求显式as_of | 保持开放。权威请求契约须区分服务端派生字段、默认值和条件必填；明确同步Run与异步Task对run_id的差异；Schema与运行时校验/Contract Test对齐 |
| CTR-006 | Task.started_at/finished_at为*time.Time + omitempty，nil时字段省略；OpenAPI nullable=true允许显式null | 保持开放。裁决省略或null；DTO、OpenAPI和queued/running/terminal Golden一致 |
| CTR-007 | StageEvent固定字段run_id/stage/status/at；可选duration_ms/message/attributes；status为started/succeeded/failed/skipped；attributes为map[string]string；OpenAPI event仍为无约束object | 保持开放。展开StageEvent Schema、required/enum/date-time/类型；补Golden与Schema/Handler Contract Test |
| CTR-008 | 本轮只确认固定基线未定位旧Analysis taskqueue表的独立DDL；不对数据库现状作不存在判断 | 不改变既有状态和关闭条件；由原责任方补迁移/DDL、唯一单调ID和真实DB分页证据 |

补充：RunResult完整嵌套结构不在本批展开，归CTR-046关闭工作；Task.status的五个输出值与RunStatus六个领域状态不同，不能共用枚举。未新增CTR。
