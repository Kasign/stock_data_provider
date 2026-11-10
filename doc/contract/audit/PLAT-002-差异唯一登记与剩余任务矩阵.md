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



## 25. Server维度2第四切片：Run/Task决策与关闭依赖归并

固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`。本节只增量更新既有CTR-046/047/006/007，不新建编号。

| CTR | 唯一责任边界 | 决策/事实状态 | 下一交付 |
|---|---|---|---|
| CTR-046 | Server负责Root HTTP路由、版本和适配；Researchflow负责自身Run/Stage/Attempt领域DTO、Schema/Golden | 项目架构已明确Server为唯一Composition Root、Researchflow为唯一跨模块流程编排者；但旧Analysis同步Run/Task/Events是否保留、是否迁移到Researchflow API尚未裁决 | 项目负责人确认API兼容/迁移边界；随后Server补最终路由/OpenAPI，Researchflow只补自身DTO契约，双方做真实组合测试 |
| CTR-047 | 旧Analysis请求由旧Analysis/Server负责；Researchflow CreateRunCommand不与其混用 | Researchflow命令字段和可信Context语义已有accepted决议；旧Analysis RunRequest仍存在run_id、TaskType校验、symbols上限和同步/异步差异待明确项 | 裁决同步Run客户端run_id、Task入口生成/覆盖规则、未知TaskType是否拒绝、symbols上限是否强制；再对齐Handler、Schema、Contract Test |
| CTR-006 | 旧Analysis Task DTO与对应OpenAPI | 当前Go行为已明确：started_at/finished_at nil时省略；nullable=true与真实输出不符 | 默认保持现有行为，Schema移除nullable并允许字段缺失；补queued/running/terminal Golden和Schema校验 |
| CTR-007 | 旧Analysis Task Events生产方/HTTP适配方 | StageEvent是旧Analysis进度事件，不是Researchflow Stage DTO；字段事实见Server维度2第9节 | 先确定旧Events端点保留范围，再为保留端点定义独立StageEvent Schema/Golden；若替换，定义迁移和兼容策略 |

### 本轮确认的权威边界

- Researchflow状态机设计决议FLOW-CTR-001/002为accepted；正式公开DTO在`internal/researchflow/publicdto`，Schema在`doc/researchflow/schema/`，Golden在`doc/researchflow/golden/`，测试在`internal/researchflow/publicdto/dto_test.go`。
- Researchflow现有Schema/Golden资产不代表已通过测试：既有决议记录指出定向Go Test/Race/Vet缺少执行证据。
- 旧Analysis的`internal/analysis/domain.RunRequest/RunResult/StageEvent`和`internal/platform/taskqueue.Task/Event`属于另一套旧HTTP契约。不能将Researchflow的Run/Stage/Attempt Schema直接复用为旧Analysis的Run/Task/StageEvent Schema。
- CTR-008仍独立负责旧Task Events的DDL、cursor持久化和真实数据库证据，本轮不重复核查。

### 关闭依赖顺序

1. CTR-046：项目负责人确认旧Analysis API保留/迁移边界。
2. CTR-047：据此确定同步/异步请求行为和运行时校验规则。
3. CTR-006：按当前nil省略行为修正Schema并补三态Golden，可与第2项并行。
4. CTR-007：确认Events端点保留后定义旧StageEvent契约；不得套用Researchflow Stage。
5. CTR-046：最后补最终Run响应Schema、Golden、Root路由和真实Handler/Adapter Contract Test。
6. 分别执行旧Analysis与Researchflow定向测试，记录SHA/CI证据。

本轮状态：complete_with_open_items；四项CTR均开放，无新增CTR、无CTR关闭。下一步仍为Server维度2，不进入其他维度。



## 26. Server维度2第五切片：项目级决策输入收敛

本节为项目负责人决策提案，非已批准决议。固定代码基线仍为`b6dfc85ba214fe16268c8cba780641a8bfe15086`。

| 决策ID | CTR | 推荐默认方案 | 确认前状态 | 确认后的交付 |
|---|---|---|---|---|
| D-SRV-API-01 | CTR-046 | 旧Analysis Run/Task/Events路径兼容保留；Researchflow API独立版本化；旧路径不得静默切换底层语义 | pending_decision | 明确兼容窗口/迁移触发条件；Server按最终边界补OpenAPI、路由和组合测试 |
| D-SRV-REQ-01 | CTR-047 | 未知非空TaskType拒绝；symbols最多100在Handler落实；异步Task run_id服务端生成并建议拒绝非空客户端值；同步Run ID先核Runner证据 | pending_decision | 请求Schema、Handler校验、错误码和Golden一致 |
| D-SRV-EVT-01 | CTR-007 | 旧Analysis Events兼容保留并定义独立StageEvent；Researchflow事件流另立契约 | pending_decision | 补旧事件Schema/Golden/cursor测试；新事件流不复用旧DTO |
| D-SRV-TIME-01 | CTR-006 | 保持Task时间字段nil时省略；OpenAPI移除nullable；补queued/running/terminal Golden | ready_for_implementation | Schema/Golden/Handler Contract Test通过后关闭CTR-006 |

### 决策提案的依据与约束

- 旧Analysis请求不能直接映射Researchflow CreateRunCommand：缺少template_ref/version、system_as_of、deadline_at、input_snapshot_refs、idempotency_key等必需语义；不得将as_of直接当作system_as_of。
- Researchflow是项目级唯一跨业务流程编排者；旧Analysis兼容保留不等于批准其长期独立承担新的跨模块编排。未来迁移必须有显式输入映射、身份/幂等策略和消费者迁移计划。
- 当前OpenAPI已声明symbols.maxItems=100，Handler尚无同等限制；建议兑现已发布Schema承诺。
- 空task_type当前有默认值；非空未知值当前未在HTTP解码层拒绝。建议明确区分空值默认和未知值错误。
- 同步Run Handler没有覆盖run_id，但Runner是否保留/重写需核实现有实现，不能只依据Handler下结论。
- CTR-006的nil省略由Go DTO标签直接确定，可不等待API边界裁决，按实际行为修正Schema。
- CTR-008不属于本轮；不重复核查DDL/cursor。

本轮状态：决策输入已形成，尚待项目负责人接受/修改；CTR-046/047/007保持pending_decision，CTR-006为ready_for_implementation但尚未修复/验收。无新增CTR、无CTR关闭。



## 27. PLAT-002 有限执行清单（收敛版，2026-11-10）

本节是后续执行的唯一批次入口。第1～26节保留历史审计证据，不再把其中每一节的“下一步”文字当作当前任务；如与本节冲突，以本节和总进度文档最新状态为准。固定代码基线仍为`b6dfc85ba214fe16268c8cba780641a8bfe15086`。

### 27.1 收敛结果与计数规则

- 不重新审计既有CTR的代码事实；本节仅基于已登记证据做任务编排。
- CTR编号覆盖至049；CTR-003/005为历史别名，CTR-041为withdrawn，CTR-010是124字段聚合项；CTR-038与CTR-048为不同接口问题。不得按编号区间推算活动数量。
- WebUI维度2第一轮完整性Review已完成（complete_with_open_items），不再重复其全量DTO/API Client初审。
- Server维度2已有五个审查切片；停止继续拆分“审计切片”，直接进入差异关闭批次。
- Platform维度2的七项适用性审查已形成证据；不再重复字段盘点。PLAT-CTR-001～007仍须按各自既有消费者/集成证据关闭。
- 当前没有证据支持把WebUI维度2的全部P2差异都作为项目整体冻结阻断；按具体端点、消费者和风险分别验收或延期。
- 审计维护分支只维护本清单和证据索引；业务修复必须由责任模块分支执行，之后按新提交SHA做增量验收。

### 27.2 有限执行批次

| 批次 | 范围 | 主要编号/事项 | 退出条件 |
|---|---|---|---|
| B0 安全与P1阻断 | 先处理明确P1和安全边界，不等待全部P2 | CTR-043（Dragon Tiger已登记P1实现冲突）；AUD-WEBUI-CTR-001/002；AUD-PLAT-PORT-001/008/009/014及相关P1候选 | 每项有责任模块、修复或批准延期、不绕过安全门禁的测试证据；P1未处理项不得被标记冻结 |
| B1 项目决策一次收敛 | 把需要项目负责人决策的事项集中成一次决议，不逐CTR开轮 | CTR-046/047/007；CTR-038/039；CTR-042/044；CTR-048/049；CTR-010字段适用性；产品范围CTR-013/023/030/033/036 | 决议写入权威`doc/contract/`位置；接受、拒绝或延期均明确影响范围/触发条件。无需决策的事实项移出本批 |
| B2 契约/Schema修正 | 按API/DTO批量修正，不按字段逐项开轮 | CTR-006/007/009/011/015/019/020/028/029/035/037；CTR-010只接收责任方提供的逐字段适用性映射 | 生产DTO、OpenAPI/Schema、Golden及Contract Test一致；不适用JSON Schema的内部Go契约提供等效机器证据 |
| B3 业务语义与消费者修复 | 按同一数据集/消费者一次性修复，保留已有来源证据 | CTR-012/014/016/017/018/021/022/024/025/026/027/031/032/034/040/043/045；以及已裁决的产品范围项 | 生产方语义有来源/算法/质量证据；消费者按真实语义处理；不完整、不可比、非PIT或不可用状态不会被展示为确定结论 |
| B4 集成验收与冻结 | 不再做新的全量审计；对B0～B3的修复提交做一次跨模块验收 | CTR-004/008/038/039/048/049等剩余测试、持久化、cursor、total/最新记录证据；PLAT/SRV/GOV/INT公共契约消费者集成证据 | 定向测试、Golden/Schema、真实DB/Root组合测试和精确SHA CI证据齐备；按契约编号逐项标记frozen或deferred |

说明：批次表是工作排序，不改变既有CTR的主责模块。某项如已由责任模块修复或已获正式延期决议，应直接在原编号下更新状态，不重复执行或另建批次问题。

### 27.3 当前最先执行的具体工作

**第一批只做Server维度2差异关闭，不再增加审查切片：**

1. CTR-006：责任模块在实际契约/实现分支修正Task时间字段Schema，补queued/running/terminal Golden及Handler Contract Test。当前审计基线已证明Go nil省略，保持现有运行时行为，不改DTO。
2. CTR-047：只补同步Runner对run_id的真实使用/持久化/查询证据；该证据到位后，将同步/异步run_id、未知TaskType和symbols上限与CTR-046/007的API决议集中裁决，不重复字段盘点。
3. CTR-046/007：项目负责人一次确认旧Analysis API与Events兼容保留边界；决议后Server按保留端点批量补OpenAPI/Schema/Golden/Root Contract Test。Researchflow DTO不替代旧Analysis DTO。
4. CTR-008：只由Researchflow/Server责任方补真实DDL、唯一单调ID及数据库分页证据；不重新审查Handler cursor算法。
5. Server维度2统一执行定向测试和CI；通过项逐项关闭，未通过项只保留原编号和明确阻断原因。

**本批执行限制：** 当前工作分支`feat/design_doc`只允许维护审计文档；不在该分支修改业务代码/OpenAPI/Golden/测试。实际修复由责任分支完成后，再以新SHA做增量验收。不得为CTR-006等事项擅自切换分支或改变固定审计基线。

### 27.4 后续执行纪律与退出条件

- 每轮必须关闭至少一个既有问题，或取得一个明确缺失的决定/证据；否则不创建新审计切片。
- 同一API/DTO的相关问题必须合并为一个修复批次，统一跑测试。
- 只有状态变化才更新总表；纯重复叙述不再追加历史章节。
- P0/P1未解决或未批准延期，不进入项目整体冻结；P2按受影响契约局部冻结或明确deferred，不无限阻塞其他无关契约。
- 十模块六维覆盖、跨模块关系、责任模块修复和三轮Review仍是PLAT-002总门槛；但不要求所有P2同一时间修完才允许对已满足条件的契约分批冻结。
- 本节不改变任何CTR状态，不宣称任何契约已修复或冻结。



## 28. CTR-047 同步 Runner run_id 增量证据（2026-11-10）

仅补此前明确缺失的Runner行为，不重复RunRequest字段盘点。

- 固定基线`internal/analysis/app/runner.go`已证明：合法非空客户端run_id被保留；空ID由Runner生成；非法ID返回Rejected/invalid_run_id。
- `internal/adapters/storage/mysqlredis/store.go`已证明：Begin使用INSERT，`analysis_runs.run_id`为主键；Finish/GetResult均按同一run_id写入/读取；重复ID不会覆盖已有Run，数据库会拒绝第二次INSERT。
- 当前Runner/Handler测试未覆盖显式ID贯穿、重复ID冲突及冲突HTTP状态；未执行真实MariaDB集成测试。Handler尚未发现将主键冲突稳定映射为409的专用分支。
- CTR-047同步Run推荐契约已收敛为：客户端合法run_id允许保留；省略时服务端生成；重复ID拒绝且应稳定映射409/run_id_conflict。异步Task Queue另按其服务端生成ID规则，不与同步入口混同。
- 状态调整：同步Run ID的实现事实已补齐，决策输入可用（ready_for_decision）；重复冲突HTTP映射、Runner/Store/Handler测试及真实DB证据仍为pending_evidence。CTR-047整体仍开放。
- 本轮没有新增CTR、修改业务代码或执行测试/CI。详细证据见`Server-维度2-DTO-Schema-Golden.md`第12节。


## 28. CTR-006 一次性修复与验收工单（2026-11-10）

本节将CTR-006从“修复准备”收敛为可直接执行的有限工单；不重新审计Task DTO，不改变固定代码审计基线。

### 28.1 固定基线事实

- 唯一契约文件：`doc/api/contracts/分析任务.openapi.yaml`。
- `components.schemas.Task.properties.started_at`与`finished_at`均为`type: string`、`format: date-time`，当前额外声明`nullable: true`。
- Go DTO既有行为为指针字段+ `omitempty`：nil时字段缺失，非nil时输出RFC3339时间值。
- 本项只对齐Schema与既有序列化行为；不改Task DTO、不改变字段名、不改变Task状态枚举，也不扩大到RunResult或StageEvent。

### 28.2 唯一修复要求

1. 删除`started_at`和`finished_at`的`nullable: true`。
2. 两字段继续保持非required属性；不得加入Task.required。
3. 保留`type: string`和`format: date-time`。
4. 不增加显式null输出；不改Go DTO序列化行为。
5. 只修改上述契约文件及CTR-006所需Golden/测试，不顺手重排其他Schema。

### 28.3 最小Golden矩阵

Golden以Task对象为核心，固定必需字段使用稳定值；只验证本CTR相关时间字段的存在性和值：

| 场景 | started_at | finished_at | 验收重点 |
|---|---|---|---|
| queued | 缺失 | 缺失 | 两字段均不出现，不能输出null |
| running | 有效RFC3339时间 | 缺失 | started_at为字符串时间，finished_at不出现 |
| terminal | 有效RFC3339时间 | 有效RFC3339时间 | 两字段均为字符串时间 |

Golden应由真实DTO序列化生成或与真实序列化逐字节/结构比较，不得手工构造后仅自我校验。

### 28.4 验收条件与关闭门槛

- OpenAPI解析/规范校验通过。
- Schema测试验证：started_at/finished_at允许缺失、拒绝null、非字符串和非法date-time。
- DTO Golden覆盖queued/running/terminal三态。
- Handler Contract Test至少验证queued响应和terminal响应与Schema一致；running状态如Handler可稳定构造，也纳入同一测试组。
- 运行责任模块定向Go测试及相关CI，记录责任分支、修复提交SHA、测试命令和结果。
- 以上证据齐全后才关闭CTR-006；仅修改Schema或Golden文件不构成关闭。

### 28.5 执行边界与当前状态

- 审计维护分支仍为`feat/design_doc`，只维护审计文档；本工单不授权在该分支修改业务契约文件。
- 固定审计基线仍为`b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- 本轮确认了契约文件和精确修复要求，但没有检查/修改责任模块分支，也没有业务实现提交可供验收；责任分支尚未在本工单中指定。
- CTR-006仍为`ready_for_implementation`，未关闭、未冻结。
- 后续不得再次讨论字段语义；取得责任分支修复SHA后，直接按28.4验收。


## 25. Data 跨模块消费边界差异登记（2026-11-10）

固定代码基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`。本节只登记 Data→Quant、Data→Research 两条边界已完成审查发现；不代表责任模块已修复，也不改变固定审计基线。

| 边界 | 唯一问题主题 | 当前状态 | 冻结影响 | 后续统一处理条件 |
|---|---|---|---|---|
| Data→Quant | Data Application DTO 与内部 Model 字段映射高度同构，公共契约独立性证据不足；内部采集字段不得未经领域语义审查直接外露 | `pending_evidence` | Data 公共 DTO 边界暂不能按当前实现直接冻结 | Data 责任方明确公开字段的业务含义/必要性，剔除纯内部实现字段或证明其公开用途，并提供 DTO/Schema/Golden 一致性证据 |
| Data→Quant | DailyBars Snapshot 缺少 Quant 所需的不可变输入引用/Manifest 连接；交易日历版本及完整性证据未映射 | `pending_decision + pending_evidence` | 依赖快照、PIT、连续交易日窗口的 Quant 输入契约不能冻结 | 统一决策：Data 提供日历版本和完整性证据，区分非交易日、停牌、缺失/无效行情；Quant 负责连续性校验；补真实签发与消费证据 |
| Data→Quant | Data v1 number/float64 与 Quant 精度要求不一致；v2 Decimal 精度、scale/舍入和版本识别未形成真实接入证据 | `pending_evidence` | 精确计算输入契约不能冻结 | 按既有 v2 Decimal 规则补字段级精度/舍入定义、原始精度保真、Golden 和真实 Adapter 证据；禁止 float64 转字符串伪造精度 |
| Data→Quant | Data Quality 含 `stale`，Quant QualityPolicy 无对应明确映射 | `pending_decision` | stale 输入处理策略不能冻结 | Data 提供 stale 原因/时间证据；Quant 按指标输入策略拒绝或保留 degraded，不得提升为 valid |
| Data→Quant | 权威 OpenAPI 路径/版本与历史契约文档引用冲突 | `pending_evidence` | 相关契约引用无法作为唯一权威来源冻结 | 统一权威路径、版本和所有消费者引用；以固定基线文件为证，不扩大 API 版本设计 |
| Data→Research | Data 日线事实本身的 `available_at` 是否公开、来源证据及缺失表达尚待核实 | `pending_decision` | 严格 PIT 可用性不能从 trade_date/provider_time/fetched_at 推断 | 核对 DATA-CTR-005/007 完整定义；若确实缺失，由 Data 明确提供有证据的 available_at，无证据时 null |
| Data→Research | Snapshot Manifest 的 snapshot_ref/manifest_fingerprint/sources 与 Evidence source_ref/source_version/source_locator/authorization_ref 映射未明确 | `pending_decision` | Evidence 来源定位及授权绑定不能冻结 | 明确 Evidence 直接引用不可变 Snapshot；Manifest 保留多 Provider 来源版本；source_locator 定位证券/日期/记录；Schema 版本与 Provider 数据版本分离；authorization_ref.resource_ref 绑定 Snapshot |
| Data→Research | v2 DTO、Data Owner AuthorizationRef 签发/复核、Research Data Adapter 和联合 Contract Test 尚无完整真实接入证据 | `pending_evidence` | Data→Research 首批日线 Evidence 交互不能标记 frozen | 只补真实生产方签发、Research Adapter 映射、授权复核和双方联合 Contract Test；不重审已确认的 Research Domain/Storage/PIT 规则 |

### 本节执行边界

- 上述问题进入唯一矩阵，后续由 Data/Quant/Research 责任方统一处理；PLAT-002 不修改责任模块代码。
- 已确认的业务职责、依赖方向、首批单证券日线范围、PIT 时间字段区分、授权 Owner 原则不再重复审查。
- 本轮之后如无责任方新提交或决策，不重读上述相同文件；只核验新增证据和关闭条件。
- 下一审查批次转入 News→Research 边界，沿用同一固定代码基线，范围仅限新闻 Evidence 查询/来源版本/PIT/授权与 Research 消费映射。


## 26. News→Research 跨模块消费边界首轮核验（2026-11-10）

- 固定代码基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- 范围：News Evidence Query DTO/Facade、PIT Repository、Snapshot/Cursor、授权读取路径，以及 Research Evidence Domain/消费边界。
- 已确认：News 拥有新闻来源、Observation、内容版本、授权和只读 Evidence Query；Research 拥有 Evidence 认识论语义；Server 负责同进程 Facade 装配。News 不新增独立 HTTP API，Research 不访问 News Repository/SQL/Domain。依赖方向正确，不需要改架构。

| 编号（审计工作项，非新增CTR） | 固定基线事实 | 状态 | 冻结影响 / 后续统一处理 |
|---|---|---|---|
| NEWS-RSC-A | News DTO 声明 available_at，但 `researchEvidenceRecordFromPIT` 未赋值；字段始终为 null。MySQL PIT 查询也未选择来源可用时间字段。News 文档明确禁止推断 available_at；Research 严格 PIT 对 null 明确拒绝 | `pending_evidence` | News 当前可提供非严格PIT证据，但不能证明首批 Research Evidence 严格PIT可用。只有取得来源级可验证时间证据才能填充；否则保持 null 并排除严格PIT，不得用 published_at/fetched_at/system_observed_at 替代 |
| NEWS-RSC-B | News DTO 的 `source_ref` 是 Source ID；另有 news_id/version_id/observation_id/provider_ref，但没有明确的 Research `source_ref/source_version/source_locator` 映射；News page 的 snapshot_ref 是分页结果集快照，不等同于单条新闻内容版本或来源版本 | `pending_decision` | 冻结前明确 Evidence 直接资源引用、来源版本和定位符：区分 News Item、News Version、Observation、Source、Provider 及分页 Snapshot；不能把内部 version_id 自动当作 Provider source_version，也不能把分页 snapshot_ref 当作单条资源版本 |
| NEWS-RSC-C | News 授权读取返回 `authorization_ref` 字符串及 authorization_status；授权快照/PolicySnapshot保存在 News 内部 Binding，未见面向 Research 的完整 GOV-CTR-006 AuthorizationRef + PolicySnapshot 公开映射或读取 Port | `pending_decision + pending_evidence` | 明确 Research 如何取得并校验 Owner 签发的授权快照；引用必须绑定可信 caller、News 资源版本、purpose/content level。Research 不得自行签发或覆盖 News 授权结果 |
| NEWS-RSC-D | News DTO 有 stale 状态；Research Evidence QualityStatus 仅 valid/degraded/invalid/unknown。当前映射函数将 BusinessQualityStatus 与 QualityStatus 固定写为 unknown，未见 stale/业务质量到 Research 质量的语义映射 | `pending_decision` | News 保留来源质量/新鲜度事实；Research 明确 stale 的 Evidence 使用策略及质量映射，不得无条件降为 degraded/unknown 或提升为 valid |
| NEWS-RSC-E | NEWS-CTR-001 查询契约允许 canonical_symbols、quality_statuses、business_time_from/to；固定基线 MySQL `ListEvidenceVersionsAt` 对上述任一非空条件直接返回 ErrInvalidEvidenceQuery | `has_difference` | 对外部 Application Port 来说属于契约声明与真实 Repository 能力不一致。由 News 责任方统一明确首批支持范围；若暂不支持，收窄权威契约/Schema，不能静默保留为“支持”字段 |
| NEWS-RSC-F | ResearchEvidenceRecord.RelationRefs 当前固定为空数组；查询 SQL 未读取新闻关联记录。契约/DTO 是否承诺返回关联引用需与首批范围核对 | `pending_decision` | 若首批不提供关系，明确空数组代表“本查询不返回关系”而非“确认无关系”；若承诺提供，则补关系版本/PIT映射和来源证据 |
| NEWS-RSC-G | News Get/List Facade、Snapshot Page、授权复核代码已存在；但同一文档第11.5节仍记载 Facade、PIT Repository、授权当前复核和固定 Snapshot“尚未实现”，与固定基线实现冲突 | `has_difference` | 更新 News 专项文档的实现状态，以代码/测试证据为准；不得据旧文档把已实现能力重复列为未实现，也不得因此直接判定端到端验收完成 |
| NEWS-RSC-H | News 侧有生产方 DTO/Schema/Golden/局部测试；Research Evidence Domain/Schema/Golden 已有，但固定基线未发现 Research News Adapter 或 Research Application 对 News Query Port 的接入；未见双方联合 Contract Test/Server 真实装配证据 | `pending_evidence` | 由 Research Adapter 与 Server 组合根责任方补消费者映射、授权/PIT字段校验和真实联合测试；PLAT-002 只验收证据，不代替实现 |

### 本批补充判断

1. News Query 的读取路径已具备单条 PIT 查询、不可变分页引用、HMAC Cursor、授权快照当前策略复核和内容级别裁剪等基础实现；这是 A1 代码证据，不等于跨模块 A2 接入或 A3 部署验收。
2. `ListEvidenceVersionsAt` 先按系统观察时间选取版本，随后 Snapshot 保存引用；严格 PIT 的 available_at 约束尚未在该 Repository 查询中体现。即使后续逐条 Get 返回 null，Research 仍会拒绝严格 PIT，但这不能替代生产方在查询边界清晰表达严格PIT过滤策略。
3. 授权当前策略复核由 News Owner 执行，Research 本地 Validator 不能替代；当前跨模块可见 DTO 是否足以让 Research 建立完整授权链，需先裁决，不将内部 Binding 结构直接暴露为公共 DTO。
4. News→Research 首批只读 Evidence Query 是唯一审查范围；不扩展到新闻采集、Provider 搜索、独立 HTTP API、Research Event/Claim 生命周期。

本批未修改 News/Research 业务代码、Schema、Golden 或测试，未执行 CI；只登记固定基线审查差异。后续统一由 News/Research/Server 责任方处理，本审计分支不越权修复。


## 27. Data→Researchflow 跨模块输入边界首轮核验（2026-11-10）

- 固定代码审计基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- 审计维护分支：`feat/design_doc`。
- 本批范围：DATA-CTR-005/008、FLOW-CTR-003/004 中 Data 快照进入 Run、Stage Provider 和下游执行上下文的边界。
- 本批不创建新的 Data DTO，不修改 Data、Researchflow、Server 责任模块代码，不扩展 Data 首批 DailyBarsV2 范围。

### 已确认的契约边界

1. Data 是行情事实、不可变 Snapshot Manifest 和 Data AuthorizationRef 的 Owner；Researchflow 只负责 Run 输入快照组织、固定执行上下文和编排，不拥有行情语义。
2. Researchflow 的 `CreateRunCommand.input_snapshot_refs` 是必填、非 null 的不透明 Ref 集合；Run 保存该集合，Stage/Attempt 继承同一组引用。重试不得刷新 `system_as_of`、替换快照或改变 Template 版本。
3. Data Snapshot Manifest 已定义内容寻址，并绑定 caller、purpose_code、symbol、日期范围、adjustment_basis、system_as_of 和所选来源版本；Data AuthorizationRef 的 `resource_ref` 必须绑定该 `snapshot_ref`。
4. Data 的 `purpose_code` 是数据访问业务目的（quant_analysis/research_analysis/risk_review），不是 Researchflow 模块身份。Researchflow 不应自行签发或改写 Data AuthorizationRef。
5. 因此不存在独立的“Data→Researchflow 行情 DTO”需求。只有某个 Stage Provider 实际读取 DailyBars 时，才由该 Provider 的 Adapter 消费 DATA-CTR-005 v2；编排层只传播不透明快照引用和固定执行上下文。

### 差异登记（审计工作项，非新增 CTR）

| 编号 | 固定基线事实 | 状态 | 冻结影响 / 后续统一处理 |
|---|---|---|---|
| DATA-FLOW-A | FLOW-CTR-003 已规定 Run 的 `system_as_of`、`input_snapshot_refs`、Template 版本和 `correlation_id` 由 Stage/Attempt 继承；Data Manifest 也绑定自己的 `system_as_of`。但当前 Researchflow Data Stage Adapter 尚不存在，未见消费者验证 Manifest 的 `system_as_of` 与 Run 完全一致的映射/测试证据 | `pending_evidence` | 真实 Data 消费 Adapter 必须按不透明 snapshot_ref 解析/验证 Manifest，拒绝时间边界不匹配；不得只凭 Run 保存了 Ref 就宣称 PIT 输入已验收 |
| DATA-FLOW-B | Run 级 `input_snapshot_refs` 会传递给所有 Stage/Attempt；已审文档未明确每个 `stage_code` 可读取哪些 Snapshot、是否需要 Stage 级引用子集，以及如何将 Template 资源策略绑定到具体 Snapshot | `pending_decision`（条件项） | 在出现真实 Data 消费 Stage 前，统一决定是否由 Template 声明 Stage 输入引用/资源范围，或由 Adapter 按注册策略过滤。目标是最小权限，不允许 Stage 因继承 Run 全量 Ref 而默认获得所有数据访问权；不阻断纯编排 Domain |
| DATA-FLOW-C | Data Manifest 的 purpose_code、caller_ref 和 AuthorizationRef 均有生产方规则，但尚无真实 Researchflow Data Stage Adapter 证明可信 Principal/Scope、业务 purpose_code 与 Snapshot/AuthorizationRef 的一致映射 | `pending_evidence` | Adapter 必须使用可信调用主体，验证 purpose_code 与调用 Scope、Manifest 和授权引用一致；不得从请求体自报 caller_ref，也不得把 AuthorizationRef 当 bearer token |
| DATA-FLOW-D | Researchflow 专项 README/任务拆分明确当前没有生产 Data Facade Adapter；DATA-CTR-005 v2 DTO/Provider 精度链路仍 pending_evidence；FLOW-CTR-003/004 的下游字段映射与 Provider Adapter Contract Test 也未完成 | `pending_evidence` | 仅在真实 Data 消费 Stage 启用前，补 v2 DTO/Schema/Golden、Stage 输入映射、输出 Envelope、授权/PIT 校验和双方 Contract Test；不要求先为编排层增加通用行情接口 |

### 本批结论

- **职责归属与依赖方向：`freeze-ready`**。Researchflow 不应依赖 Data 内部 Model/Repository，也不应复制 Data 行情 DTO。
- **Run 级执行上下文语义：`accepted`**。唯一 `system_as_of`、不透明输入快照引用、重试不变更输入的原则已明确。
- **Stage 级最小权限映射：`pending_decision`（条件项）**。只在明确 Data 消费 Stage 的资源授权范围前需要裁决。
- **真实 Data 消费交互：`pending_evidence`**。当前无生产 Data Stage Adapter，不能宣称 Data→Researchflow 已完成 L2 或 frozen。
- 沿用 DATA-CTR-005、DATA-CTR-008、FLOW-CTR-003、FLOW-CTR-004，不新增 CTR；本批不把 Researchflow 对 Data 的逻辑依赖误判为已经存在的直接 DTO 调用。

本批只更新 PLAT-002 审计记录；未修改业务代码、Schema、Golden、测试或执行 CI。后续只在真实 Data Stage Adapter 或相关契约提交出现时增量核验，不重复本轮职责和上下文审查。


## 28. News→Researchflow 跨模块消费边界首轮核验（2026-11-10）

- 固定代码审计基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- 审计维护分支：`feat/design_doc`。
- 范围：NEWS-CTR-001/002/003 与 FLOW-CTR-003/004 的组合边界；只核对 News Evidence 查询结果、分页快照、PIT 和授权上下文如何成为 Run/Stage 固定输入。
- 不重复 News→Research 已登记的 available_at、Evidence 来源字段、授权 DTO 映射、质量状态和 Research Adapter 问题；本批只记录它们对 Researchflow 输入生命周期的额外影响。
- 不修改 News、Researchflow、Server 业务代码，不新增 CTR。

### 已确认的边界

1. News 的 `ListEvidence` 是同进程只读 Application Port；请求必须显式携带可信 `caller_ref`、`purpose_code`、`requested_content_level`、Run 的 `system_as_of`、受控 filters 和 page_size，不隐式触发搜索/抓取。
2. News 首屏返回的 `snapshot_ref` 固定分页结果集；后续页使用同一 `snapshot_ref` 和不透明 `cursor`。Cursor 是续页令牌，不是独立业务资源引用，也不能代替 Evidence 版本引用。
3. News 首期分页 Snapshot 和 Cursor TTL 均为 15 分钟；过期或快照不可用必须明确失败，禁止静默重建快照继续翻页。
4. Researchflow 的 `CreateRunCommand.input_snapshot_refs` 在 Run 创建时固定，纳入请求指纹，并由 Stage/Attempt/重试原样继承；`system_as_of` 同样固定。当前契约没有授权 Stage 在执行或重试时刷新输入快照。
5. News AuthorizationRef 的 `subject_ref` 必须等于可信 `caller_ref`，`resource_ref` 必须指向实际被授权的 News Evidence/版本资源；每次当前内容读取仍由 News Owner 复核授权，历史 allowed 不能直接作为后续访问许可。

### 差异登记（审计工作项，非新增 CTR）

| 编号 | 固定基线事实 | 状态 | 冻结影响 / 后续统一处理 |
|---|---|---|---|
| NEWS-FLOW-A | News 分页 `snapshot_ref` 是不可变结果集快照，但 Snapshot/Cursor TTL 只有 15 分钟；Researchflow Run/Stage/Attempt 可跨越该时长，且重试必须沿用原始输入。现有契约未定义将短期分页快照提升/固定为 Run 生命周期输入的机制 | `pending_decision` | 必须区分“查询分页快照”与“Run 输入快照”。推荐在 Run 创建前由受控准备步骤形成可持久引用的 Evidence 输入快照，或由 News 提供明确的 Run 级 Pin/Materialize 能力；其保留期至少覆盖 Run deadline、重试/恢复和规定的历史追溯期。不能把 15 分钟 Cursor/Snapshot 直接当作长期 Run 输入，也不能在重试时重新查询最新结果 |
| NEWS-FLOW-B | Researchflow CreateRun 要求创建时提供固定 `input_snapshot_refs`；News `ListEvidence` 的首屏查询却是在调用时才生成 `snapshot_ref`。现有契约没有定义该快照在 CreateRun 前取得并纳入请求指纹，还是由 Stage 首次执行时取得并登记为派生输入；后者会使 Run 初始指纹未覆盖真实输入，且与“Run 输入固定”原则冲突 | `pending_decision` | 明确唯一生命周期：优先采用 CreateRun 前准备并校验输入引用，再创建 Run；若产品要求 Stage 内动态发现，则必须定义不可变的 Stage 输入派生记录、原子持久化、重试复用和审计语义，不得静默追加/替换 Run.input_snapshot_refs |
| NEWS-FLOW-C | News 查询要求 `caller_ref/purpose_code/requested_content_level/filters/page_size`；Researchflow 具有可信 `authenticated_caller_ref`，但尚未明确该身份如何映射为 News caller_ref（原始用户主体还是受控服务主体/委托主体），也未明确 purpose/content level/filter 是否由版本化 Template/参数固定并纳入 Run 指纹 | `pending_decision` | 统一可信身份与委托规则：News AuthorizationRef.subject_ref 必须匹配实际传入 caller_ref；caller_ref 不得来自请求体自报。purpose_code、requested_content_level、filters 和分页策略必须由版本化 Template/参数或已固定输入快照承载，确保同一 Run 重试不改变用途、内容级别或查询范围 |
| NEWS-FLOW-D | News 查询分页 Cursor 绑定 caller、purpose、content level、filters、page_size、system_as_of、snapshot_ref 和排序版本；Researchflow 只定义不透明 `input_snapshot_refs`，未明确 Cursor 的持有者、保存位置及跨 Attempt 续页规则 | `pending_decision`（随 A/B 决策处理） | Cursor 只能由 News Adapter/Owner 持有并原样用于同一查询快照续页；不得放入 `input_snapshot_refs` 冒充资源引用，不得跨 Run/过滤条件/调用主体复用。若 Run 输入准备已物化为完整 Evidence 引用集合，则 Cursor 不进入 Run/Stage 公共契约 |
| NEWS-FLOW-E | Researchflow 运行时文档要求 Data/News 通过公开查询契约调用；模块 README 明确当前不包含 News 生产 Facade Adapter，News 专项文档也仍将内容查询 Facade 标为未闭环。固定基线未见 Researchflow News Consumer Adapter、真实 Server 装配及双方联合 Contract Test 证据 | `pending_evidence` | 在 A/B/C 生命周期和身份语义决议后，由责任模块实现 Adapter 和 Server 装配；验收必须覆盖固定 system_as_of、输入快照生命周期、分页完整性、授权拒绝/撤销、Attempt 重试复用和 Run 输入指纹，不以 Fake Provider 单测替代 L2 |
| NEWS-FLOW-F | News 首批严格 PIT 会排除 `available_at` 为 null/unknown 的记录；News→Research 已登记该字段映射/来源证据缺口。Researchflow 若将查询结果作为 Run 输入，不能把非严格 PIT 返回的结果或分页 Snapshot 标记成严格 PIT 已验收输入 | `pending_evidence` | 沿用 NEWS-RSC-A 的处理结论；Researchflow Adapter 必须保留 News 的 PIT 可用性结果，不得自行推断 available_at，也不得将不满足严格 PIT 的 Evidence 纳入要求严格 PIT 的 Template |

### 本批结论

- **模块职责与依赖方向：`freeze-ready`**。News 拥有 Evidence 查询、分页快照、授权和 PIT；Researchflow 拥有 Run 输入固定、Stage/Attempt 生命周期和编排。无需新增 News HTTP API，也不应由 Researchflow 复制 News 查询/授权逻辑。
- **PIT 时间传播原则：`accepted`**。News 查询必须原样接收 Run.system_as_of；Stage/Attempt/重试不得刷新时间边界。
- **News 查询快照到 Run 输入的生命周期：`pending_decision`**。15 分钟分页 Snapshot 与 Run 长生命周期输入不是同一概念，且首屏快照生成时点与 CreateRun 固定输入之间存在流程缺口。
- **可信主体、purpose/content level 与查询条件映射：`pending_decision`**。必须在 Run 创建/模板版本中固定，不能由 Stage 重试时临时变化。
- **真实 News 消费交互：`pending_evidence`**。固定基线尚无生产 News Adapter/Server 装配/联合 Contract Test，不能宣称 NEWS→Researchflow L2 或 frozen。
- 沿用 NEWS-CTR-001/002/003、FLOW-CTR-003/004、GOV-CTR-002/006；不新增 CTR。NEWS-RSC-A/B/C/D/H 已有问题不重复登记，只引用其既有结论。

本批只更新 PLAT-002 审计记录；未修改业务代码、Schema、Golden、测试或执行 CI。后续统一处理 A/B/C/D 的生命周期与身份决策，不在本审计分支代替责任模块实现。只有相关决议或 Adapter 提交出现后才增量复核，不重做本批初审。


## 29. Quant→Researchflow 结果消费边界首轮核验（2026-11-10）

- 固定代码审计基线：origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086；维护分支：feat/design_doc。
- 范围：QNT-CTR-001～005 与 FLOW-CTR-003/004 的组合边界；核对结果身份、Manifest、Envelope、PIT/输入引用、授权和 Researchflow 消费映射。
- 不审查 Quant 算法公式、Registry 全量条目或 Risk 规则内容；不重复 Data→Quant 已登记问题。不修改 Quant、Researchflow、Server 业务代码，不新增 CTR。

### 已确认的契约边界

1. Quant 是结果语义和结果资源 Owner；Researchflow 是主要编排消费者。Researchflow 不重算指标、不解释/转换单位、不覆盖 RiskDecision，也不依赖 Quant 内部 Domain/Repository/SQL。
2. Quant Result Manifest 是不可变内容寻址身份，绑定 result_type、schema_version、system_as_of、input_snapshot_refs 和 result_fingerprint；result_id 由 Manifest fingerprint 派生。AuthorizationRef.resource_ref 绑定 result_id，subject_ref 来自可信调用主体，policy_owner=quant。
3. Quant 每次结果读取必须由 Quant Owner 按当前策略重新授权；历史 allowed AuthorizationRef 是审计快照，不是 bearer token，也不能覆盖当前 denied/expired/unknown。
4. Researchflow 已接受以 result_type + schema_version 精确选择强类型 Payload Validator；未知 discriminator/schema 必须拒绝，不回退旧版本；Envelope 的 system_as_of、参数指纹、输入快照引用、AuthorizationRef 必须与 Run 上下文校验。
5. Stage 执行 Envelope 的执行关联字段由 Orchestrator 注入，Quant 只拥有 typed domain payload、output_type/schema_version 和领域质量摘要；Quant 不得伪造 run_ref/stage_ref/attempt_ref 或 Stage 执行状态。
6. Quant 结果引用与 Run 原始输入快照引用是不同身份空间：前者指向 Quant 计算结果资源，后者指向上游事实快照。不得把 Quant result_id 冒充 Data/News input_snapshot_refs。

### 差异登记（审计工作项，非新增 CTR）

| 编号 | 固定基线事实 | 状态 | 冻结影响 / 后续统一处理 |
|---|---|---|---|
| QNT-FLOW-A | Quant 文档明确 QuantResultRef 已有 DTO/Schema/Golden，但 Ref 与完整 QuantResult Envelope 的共同字段一致性、两者关系及 GetResult 返回边界仍待明确；Researchflow FLOW-006 却以完整 Quant Envelope 作为输入 | pending_decision | 明确唯一读取流程：Researchflow 持有 QuantResultRef，通过 Quant Application Facade 按 result_id 获取经当前授权复核的完整 Envelope；或明确 Ref 本身就是完整结果投影。必须说明字段权威、GetResult 返回类型和错误语义，不能由 Researchflow 自行拼装 Envelope |
| QNT-FLOW-B | QNT-CTR-004 已有部分 Factor QuantMetric、TrendResult、RiskDecision 的 DTO/Schema/Golden，但各 result_type/schema_version 的强类型 Payload 子 Schema 不齐；FLOW-006 只提供 Validator Registry 接口，没有生产 Payload Schema 注册 | pending_evidence | Quant 按实际首批可消费结果提供精确 Payload Schema、Golden 和强类型序列化证据；Researchflow 注册对应 Validator 并做消费者负例。不得用任意 map、宽泛 Schema 或 Notification Risk Event Schema 替代 |
| QNT-FLOW-C | QNT-CTR-005 的 Manifest/AuthorizationRef 结构与部分内容寻址测试已存在，但真实 Facade 读取、可信 Principal/Scope、当前授权复核、结果归属、撤销/竞态仍未完成 | pending_evidence | Quant Owner Facade 每次 GetResult 验证可信 Researchflow service principal、purpose/content scope、resource_ref=result_id 和当前策略；denied/unknown/expired/revoked 不返回 payload。补撤销与读取竞态、越权主体、结果归属和脱敏 Fault 测试 |
| QNT-FLOW-D | Manifest 已绑定 system_as_of 与 input_snapshot_refs，FLOW-006 要求 Envelope.system_as_of 与 Run 一致并校验输入快照引用，但固定基线无真实 Adapter/联合测试证明 Manifest、Envelope、Run 三者逐项一致 | pending_evidence | Adapter 校验 Envelope.system_as_of == Run.system_as_of，Envelope.input_snapshot_refs 与 Quant Manifest 完全一致，参数指纹、算法/Schema 版本与 Manifest 一致；错配拒绝，不能只检查字段非空 |
| QNT-FLOW-E | FLOW-CTR-004 要求 Orchestrator 注入执行关联字段；QuantResultRef 另含 result_id、algorithm_version、parameter_fingerprint、authorization_ref、input_snapshot_refs 等元数据，二者投影关系尚未形成共同 Schema/Golden | pending_decision | 明确 Quant Application Result Envelope 到 Researchflow Stage Provider Result Envelope 的投影：哪些字段留在 Quant 结果资源，哪些由 Orchestrator 注入，output_ref 指向哪个资源；避免双权威，也禁止 Provider 覆盖执行关联字段 |
| QNT-FLOW-F | Quant 模块 CI 只证明模块内代码；Researchflow 文档明确生产 Facade 接入、结果持久化和跨模块调用链未完成，固定基线未见 Quant Consumer Adapter、Server 真实装配和双方 Contract Test | pending_evidence | A/E 决策后补真实 Adapter、Server DI 装配、ResultRef 保存/读取、未知版本拒绝、授权拒绝、PIT/输入错配及重试复用同一结果引用等 L2 测试；模块 CI/Fixture 不替代联合验收 |

### 本批结论

- 职责与依赖方向：freeze-ready。Quant 拥有结果和授权语义，Researchflow 拥有流程/Stage 执行语义；无需改变模块依赖或创建通用 Provider 抽象。
- Manifest 内容寻址与授权资源绑定原则：accepted。Manifest 绑定结果类型、版本、system_as_of、输入快照和结果指纹；AuthorizationRef 绑定 result_id；读取时由 Quant Owner 复核当前授权。
- Ref→Envelope→Stage output_ref 的读取/投影关系：pending_decision。Quant 文档明确 Ref 与 Envelope 关系/GetResult 边界未定，而 FLOW-006 已按完整 Envelope 消费。
- 强类型 Payload Schema 与消费者 Validator：pending_evidence。部分 DTO 已有 Schema/Golden，不代表所有实际 result_type/schema_version 均可消费。
- 真实授权读取、PIT/Manifest 对齐、Adapter/Server 装配及联合测试：pending_evidence。当前没有足够证据宣称 Quant→Researchflow 达到 L2 或 frozen。
- 沿用 QNT-CTR-001～005、FLOW-CTR-003/004、GOV-CTR-002/006，不新增 CTR。Data→Quant 已登记问题不重复登记。

本批只更新 PLAT-002 审计记录；未修改业务代码、Schema、Golden、测试或执行 CI。后续统一处理 A/E 的有限契约决策；B/C/D/F 等待责任模块实现证据后增量核验，不重复本轮算法或 DTO 审查。
