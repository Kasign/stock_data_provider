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
| CTR-005 | 旧 Analysis CreateTask 请求约束历史上由 CTR-047 承接 | 随 DEC-018 旧 API 退役，不再有活动关闭条件；历史别名不计活动项 |
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
| CTR-043～044 | Dragon Tiger | CTR-043 公共分页计数语义已明确、契约可冻结；Hithink Adapter 实现偏差转 Data 内部整改；CTR-044 Provider 日期能力边界待裁决 |
| CTR-045 | Stock Search 质量状态消费 | P2；WebUI 空结果和质量状态提示待对齐 |
| CTR-046～047 | 旧 Analysis Run/Task HTTP 契约 | DEC-018 已决定整组旧 HTTP API 退役；CTR-047 按本矩阵第40节撤销活动状态，CTR-046/006/007/008 等相邻旧 HTTP 契约按同一边界归类，不再补齐旧 OpenAPI 或分页协议 |
| CTR-048～049 | Provider Probe History | P2；total 语义与存储错误 HTTP 映射待裁决/补证 |

这些结论只记录冻结影响，不表示责任模块已修复。不得再次对上述范围执行相同的初审或冻结影响分析；后续只检查裁决结果、责任模块新提交和关闭证据。

## 5. 尚未完成的 WebUI 维度 2 工作

| 工作批次 | 涉及编号/范围 | 当前状态 | 完成条件 |
|---|---|---|---|
| W2-A 旧 Analysis Task/TaskEvent HTTP 契约 | CTR-006、007、008 | 已由 DEC-018 退役决议 superseded，不再作为活动公共契约冻结项 | 旧 Task 时间 Schema、StageEvent Schema、Task Events cursor/分页均不再补齐旧 HTTP 契约。若旧事件表/队列仍被保留，其 DDL、持久化和并发正确性归 Server/Platform 内部工程质量或数据保留工作，不属于跨模块公开契约冻结 |
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

## 7. 本轮契约定义冻结门槛（范围更正）

本矩阵服务于 PLAT-002 的跨模块公开契约冻结。唯一冻结门禁以 `doc/contract/PLAT-002-项目级契约冻结登记簿.md` 第5节为准：

- 生产方、直接消费者、Owner、调用方向和公开边界明确。
- 权威定义位置、版本、字段/命令/事件语义、错误/状态/授权/幂等规则及兼容边界完整。
- 存在可核对的机器 Schema，或纯 InProcess Port 的等价权威强类型定义。
- 不存在尚未裁决且会改变公开契约的语义取舍。

模块内部实现、单元/集成测试、Provider 专项测试、CI、部署、Worker/Repository 生命周期和真实依赖可用性不属于本轮契约定义冻结门禁。实现偏差可单独记录，但不得阻止正确公开契约冻结，也不得借此扩张 PLAT-002 任务。

本矩阵中旧的“实现/测试/CI 关闭条件”仅保留历史追溯用途；与上述门禁冲突时，以项目级契约冻结登记簿第5节为准。

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

仍开放且不得从矩阵移除的事项包括：CTR-010 124字段逐项适用条件/无值原因证据；CTR-006～009、011～040、042～049各自未完成的裁决、实现、测试或产品范围决议；getMarketSentiment产品范围待确认。CTR-041保持withdrawn，CTR-003/005只作历史别名。

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
| CreateTask的AnalysisRequest Schema与RunRequest字段/约束不完整 | CTR-047（历史审计项） | 已被 DEC-018 旧 API 退役决议覆盖；不再是活动公共契约差异 | 不补齐旧 CreateTask Schema；新 API 只按 Researchflow CreateRunCommand 独立契约定义 |
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
- Analysis OpenAPI没有统一Error Schema/错误响应JSON Schema。CTR-046/047 是旧 Analysis HTTP 契约历史审计项；DEC-018 已决定退役，不再据此补齐旧 API Schema。
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
| CTR-047 | 旧 Analysis RunRequest 与 CreateTask HTTP DTO 字段差异 | 历史事实保留；旧 API 已由 DEC-018 决定退役 | 不再为旧请求定义新的 required/default/枚举或同步/异步 ID 规则；新 API 按 Researchflow 独立命令契约验收 |
| CTR-006 | Task.started_at/finished_at为*time.Time + omitempty，nil时字段省略；OpenAPI nullable=true允许显式null | 保持开放。裁决省略或null；DTO、OpenAPI和queued/running/terminal Golden一致 |
| CTR-007 | StageEvent固定字段run_id/stage/status/at；可选duration_ms/message/attributes；status为started/succeeded/failed/skipped；attributes为map[string]string；OpenAPI event仍为无约束object | 保持开放。展开StageEvent Schema、required/enum/date-time/类型；补Golden与Schema/Handler Contract Test |
| CTR-008 | 本轮只确认固定基线未定位旧Analysis taskqueue表的独立DDL；不对数据库现状作不存在判断 | 不改变既有状态和关闭条件；由原责任方补迁移/DDL、唯一单调ID和真实DB分页证据 |

补充：RunResult完整嵌套结构不在本批展开，归CTR-046关闭工作；Task.status的五个输出值与RunStatus六个领域状态不同，不能共用枚举。未新增CTR。



## 25. Server维度2第四切片：Run/Task决策与关闭依赖归并

固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`。本节只增量更新既有CTR-046/047/006/007，不新建编号。

| CTR | 唯一责任边界 | 决策/事实状态 | 下一交付 |
|---|---|---|---|
| CTR-046 | Server负责Root HTTP路由、版本和适配；Researchflow负责自身Run/Stage/Attempt领域DTO、Schema/Golden | 项目架构已明确Server为唯一Composition Root、Researchflow为唯一跨模块流程编排者；但旧Analysis同步Run/Task/Events是否保留、是否迁移到Researchflow API尚未裁决 | 项目负责人确认API兼容/迁移边界；随后Server补最终路由/OpenAPI，Researchflow只补自身DTO契约，双方做真实组合测试 |
| CTR-047 | 旧 Analysis CreateTask HTTP 请求契约（已退役范围） | DEC-018 已决定旧 API 退役；DEC-019 的旧接口规则不迁移至 Researchflow | 撤销旧 CreateTask 的活动契约待办；新 API 只按 FLOW-CTR-001 等 Researchflow 契约定义并验收 |
| CTR-006 | 旧Analysis Task DTO与对应OpenAPI | 当前Go行为已明确：started_at/finished_at nil时省略；nullable=true与真实输出不符 | 默认保持现有行为，Schema移除nullable并允许字段缺失；补queued/running/terminal Golden和Schema校验 |
| CTR-007 | 旧Analysis Task Events生产方/HTTP适配方 | StageEvent是旧Analysis进度事件，不是Researchflow Stage DTO；字段事实见Server维度2第9节 | 先确定旧Events端点保留范围，再为保留端点定义独立StageEvent Schema/Golden；若替换，定义迁移和兼容策略 |

### 本轮确认的权威边界

- Researchflow状态机设计决议FLOW-CTR-001/002为accepted；正式公开DTO在`internal/researchflow/publicdto`，Schema在`doc/researchflow/schema/`，Golden在`doc/researchflow/golden/`，测试在`internal/researchflow/publicdto/dto_test.go`。
- Researchflow现有Schema/Golden资产不代表已通过测试：既有决议记录指出定向Go Test/Race/Vet缺少执行证据。
- 旧Analysis的`internal/analysis/domain.RunRequest/RunResult/StageEvent`和`internal/platform/taskqueue.Task/Event`属于另一套旧HTTP契约。不能将Researchflow的Run/Stage/Attempt Schema直接复用为旧Analysis的Run/Task/StageEvent Schema。
- CTR-008仍独立负责旧Task Events的DDL、cursor持久化和真实数据库证据，本轮不重复核查。

### 关闭依赖顺序

1. CTR-046：项目负责人确认旧Analysis API保留/迁移边界。
2. CTR-047：旧 CreateTask HTTP 契约已由 DEC-018 退役，不再执行同步/异步旧请求行为的冻结工作；Researchflow 新 API 独立按 FLOW-CTR-001 验收。
3. CTR-006：按当前nil省略行为修正Schema并补三态Golden，可与第2项并行。
4. CTR-007：确认Events端点保留后定义旧StageEvent契约；不得套用Researchflow Stage。
5. CTR-046：最后补最终Run响应Schema、Golden、Root路由和真实Handler/Adapter Contract Test。
6. 分别执行旧Analysis与Researchflow定向测试，记录SHA/CI证据。

本轮状态：complete_with_open_items；四项CTR均开放，无新增CTR、无CTR关闭。下一步仍为Server维度2，不进入其他维度。



## 26. Server维度2第五切片：项目级决策输入收敛

本节为项目负责人决策提案，非已批准决议。固定代码基线仍为`b6dfc85ba214fe16268c8cba780641a8bfe15086`。

| 决策ID | CTR | 推荐默认方案 | 确认前状态 | 确认后的交付 |
|---|---|---|---|---|
| D-SRV-API-01 | CTR-046 | 旧 Analysis API 不保留兼容期；WebUI 直接切换 Researchflow 新 API。当前缺口是新 API 的实际路由、公开 DTO/OpenAPI、认证/错误映射和旧专属路由退役尚未完成。 | `pending_evidence` | 新 Researchflow API 路由与 OpenAPI 和 Handler/WebUI 一致；跨端 Contract Test 通过；验收后删除旧 Analysis 路由、Handler、专属 DTO/事件适配及专属测试。 |
| D-SRV-REQ-01 | CTR-047 | 历史上针对旧 Analysis CreateTask/Run 的请求规则；旧 API 已按 DEC-018 退役 | `superseded_by_DEC-018` | 不再落实旧 Handler/OpenAPI；Researchflow 新 API 只按其独立 CreateRunCommand、caller-scoped idempotency_key 和 fingerprint 契约验收。 |
| D-SRV-EVT-01 | CTR-007 | 旧 Analysis Event 不设兼容期；新 Researchflow Task/Run/Event 使用独立版本化 DTO。当前缺口是新事件 Schema/游标/查询路由实现与旧专属事件适配清理。 | `pending_evidence` | 新 Event Schema/Golden、Handler/WebUI 映射和游标测试通过；新 API 验收后删除旧专属事件路由/DTO/适配。 |
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
| B0 安全与P1阻断 | 先处理明确P1和安全边界，不等待全部P2 | AUD-WEBUI-CTR-001/002；AUD-PLAT-PORT-001/008/009/014及相关P1候选 | 每项有责任模块、修复或批准延期、不绕过安全门禁的测试证据；P1未处理项不得被标记冻结 |
| B1 项目决策一次收敛 | 把需要项目负责人决策的事项集中成一次决议，不逐CTR开轮 | CTR-046/047/007；CTR-038/039；CTR-042/044；CTR-048/049；CTR-010字段适用性；产品范围CTR-013/023/030/033/036 | 决议写入权威`doc/contract/`位置；接受、拒绝或延期均明确影响范围/触发条件。无需决策的事实项移出本批 |
| B2 契约/Schema修正 | 按API/DTO批量修正，不按字段逐项开轮 | CTR-006/007/009/011/015/019/020/028/029/035/037；CTR-010只接收责任方提供的逐字段适用性映射 | 生产DTO、OpenAPI/Schema、Golden及Contract Test一致；不适用JSON Schema的内部Go契约提供等效机器证据 |
| B3 业务语义与消费者修复 | 按同一数据集/消费者一次性修复，保留已有来源证据 | CTR-012/014/016/017/018/021/022/024/025/026/027/031/032/034/040/045；以及已裁决的产品范围项 | 生产方语义有来源/算法/质量证据；消费者按真实语义处理；不完整、不可比、非PIT或不可用状态不会被展示为确定结论 |
| B4 集成验收与冻结 | 不再做新的全量审计；对B0～B3的修复提交做一次跨模块验收 | CTR-004/008/038/039/048/049等剩余测试、持久化、cursor、total/最新记录证据；PLAT/SRV/GOV/INT公共契约消费者集成证据 | 定向测试、Golden/Schema、真实DB/Root组合测试和精确SHA CI证据齐备；按契约编号逐项标记frozen或deferred |

说明：批次表是工作排序，不改变既有CTR的主责模块。某项如已由责任模块修复或已获正式延期决议，应直接在原编号下更新状态，不重复执行或另建批次问题。

### 27.3 当前最先执行的具体工作

**第一批只做Server维度2差异关闭，不再增加审查切片：**

1. CTR-006：责任模块在实际契约/实现分支修正Task时间字段Schema，补queued/running/terminal Golden及Handler Contract Test。当前审计基线已证明Go nil省略，保持现有运行时行为，不改DTO。
2. CTR-047：历史同步 Runner run_id 证据已记录；因旧 HTTP API 已按 DEC-018 退役，不再为该旧请求入口补 HTTP 409、Schema 或 Contract Test 证据。
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
- 历史状态（已被后续 DEC-018 覆盖）：当时同步Run ID事实为 ready_for_decision，重复冲突HTTP映射和测试证据仍缺。该证据缺口不再构成旧 HTTP API 的活动契约门禁；底层存储/运行组件若被新模块继续使用，其内部正确性另由对应模块工程验收负责。
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
| Data→Quant | DailyBars Snapshot 缺少 Quant 所需的不可变输入引用/Manifest 连接；交易日历版本及完整性证据未映射 | `pending_evidence` | 依赖快照、PIT、连续交易日窗口的 Quant 输入契约不能冻结 | 统一决策：Data 提供日历版本和完整性证据，区分非交易日、停牌、缺失/无效行情；Quant 负责连续性校验；补真实签发与消费证据 |
| Data→Quant | Data v1 number/float64 与 Quant 精度要求不一致；v2 Decimal 精度、scale/舍入和版本识别未形成真实接入证据 | `pending_evidence` | 精确计算输入契约不能冻结 | 按既有 v2 Decimal 规则补字段级精度/舍入定义、原始精度保真、Golden 和真实 Adapter 证据；禁止 float64 转字符串伪造精度 |
| Data→Quant | Data Quality 含 `stale`，Quant QualityPolicy 无对应明确映射 | `pending_evidence` | stale 输入处理策略不能冻结 | Data 提供 stale 原因/时间证据；Quant 按指标输入策略拒绝或保留 degraded，不得提升为 valid |
| Data→Quant | 权威 OpenAPI 路径/版本与历史契约文档引用冲突 | `pending_evidence` | 相关契约引用无法作为唯一权威来源冻结 | 统一权威路径、版本和所有消费者引用；以固定基线文件为证，不扩大 API 版本设计 |
| Data→Research | Data 日线事实本身的 `available_at` 是否公开、来源证据及缺失表达尚待核实 | `pending_evidence` | 严格 PIT 可用性不能从 trade_date/provider_time/fetched_at 推断 | 核对 DATA-CTR-005/007 完整定义；若确实缺失，由 Data 明确提供有证据的 available_at，无证据时 null |
| Data→Research | Snapshot Manifest 的 snapshot_ref/manifest_fingerprint/sources 与 Evidence source_ref/source_version/source_locator/authorization_ref 映射未明确 | `pending_evidence` | Evidence 来源定位及授权绑定不能冻结 | 明确 Evidence 直接引用不可变 Snapshot；Manifest 保留多 Provider 来源版本；source_locator 定位证券/日期/记录；Schema 版本与 Provider 数据版本分离；authorization_ref.resource_ref 绑定 Snapshot |
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
| NEWS-RSC-B | 来源资源语义已由 DEC-PLAT-002-021 裁决；当前缺口是 News Item/Version/Observation/Source/Provider/分页 Snapshot 在生产 DTO 与 Research Evidence 中的实际映射证据。 | `pending_evidence` | 补齐字段映射、Schema/Golden 和消费者测试；内部 version_id 不冒充 Provider source_version，分页 snapshot_ref 不冒充单条资源版本。 |
| NEWS-RSC-C | News 授权读取返回 `authorization_ref` 字符串及 authorization_status；授权快照/PolicySnapshot保存在 News 内部 Binding，未见面向 Research 的完整 GOV-CTR-006 AuthorizationRef + PolicySnapshot 公开映射或读取 Port | `pending_evidence` | 按 GOV-CTR-006 实现 News Owner 签发/公开传递 AuthorizationRef 与 PolicySnapshot；Research 按可信 caller/purpose 消费，News 每次读取复核当前授权；补双方 Contract Test 和撤销/过期证据 |
| NEWS-RSC-D | 质量与 stale 使用原则已由 DEC-PLAT-002-021 裁决；当前缺口是 News 来源质量/新鲜度事实到 Research Evidence 的实际映射及策略执行证据。 | `pending_evidence` | News 保留质量/新鲜度事实；Research 对 stale 明确拒绝或按显式策略降为 degraded；Schema/Golden/消费者测试覆盖。 |
| NEWS-RSC-E | NEWS-CTR-001 查询契约允许 canonical_symbols、quality_statuses、business_time_from/to；固定基线 MySQL `ListEvidenceVersionsAt` 对上述任一非空条件直接返回 ErrInvalidEvidenceQuery | `has_difference` | 对外部 Application Port 来说属于契约声明与真实 Repository 能力不一致。由 News 责任方统一明确首批支持范围；若暂不支持，收窄权威契约/Schema，不能静默保留为“支持”字段 |
| NEWS-RSC-F | 关系状态语义已由 DEC-PLAT-002-021 裁决；当前缺口是首期关系能力状态在 News/Research DTO、Schema 和查询结果中的一致表达。 | `pending_evidence` | 首期未提供关系时显式表达 not_provided；confirmed_none 只用于有证据确认无关系；Schema/Golden/消费者测试通过。 |
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
| DATA-FLOW-B | Run 级 `input_snapshot_refs` 会传递给所有 Stage/Attempt；已审文档未明确每个 `stage_code` 可读取哪些 Snapshot、是否需要 Stage 级引用子集，以及如何将 Template 资源策略绑定到具体 Snapshot | `deferred`（条件项） | 在出现真实 Data 消费 Stage 前，统一决定是否由 Template 声明 Stage 输入引用/资源范围，或由 Adapter 按注册策略过滤。目标是最小权限，不允许 Stage 因继承 Run 全量 Ref 而默认获得所有数据访问权；不阻断纯编排 Domain |
| DATA-FLOW-C | Data Manifest 的 purpose_code、caller_ref 和 AuthorizationRef 均有生产方规则，但尚无真实 Researchflow Data Stage Adapter 证明可信 Principal/Scope、业务 purpose_code 与 Snapshot/AuthorizationRef 的一致映射 | `pending_evidence` | Adapter 必须使用可信调用主体，验证 purpose_code 与调用 Scope、Manifest 和授权引用一致；不得从请求体自报 caller_ref，也不得把 AuthorizationRef 当 bearer token |
| DATA-FLOW-D | Researchflow 专项 README/任务拆分明确当前没有生产 Data Facade Adapter；DATA-CTR-005 v2 DTO/Provider 精度链路仍 pending_evidence；FLOW-CTR-003/004 的下游字段映射与 Provider Adapter Contract Test 也未完成 | `pending_evidence` | 仅在真实 Data 消费 Stage 启用前，补 v2 DTO/Schema/Golden、Stage 输入映射、输出 Envelope、授权/PIT 校验和双方 Contract Test；不要求先为编排层增加通用行情接口 |

### 本批结论

- **职责归属与依赖方向：`freeze-ready`**。Researchflow 不应依赖 Data 内部 Model/Repository，也不应复制 Data 行情 DTO。
- **Run 级执行上下文语义：`accepted`**。唯一 `system_as_of`、不透明输入快照引用、重试不变更输入的原则已明确。
- **Stage 级最小权限映射：`deferred`（条件项）**。仅在首个真实 Data 消费 Stage 进入实现/接入前裁决；当前不阻断纯编排 Domain。
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
| NEWS-FLOW-A | 快照生命周期和 Run 输入准备方式已由 DEC-PLAT-002-022 裁决；当前缺口是分页结果到 Run 级持久化输入 Snapshot 的真实物化、保留和授权复核实现。 | `pending_evidence` | Run 创建前受控准备并物化实际需要内容；生命周期覆盖 Run deadline/重试/恢复；重试复用原 Snapshot。 |
| NEWS-FLOW-B | Run 创建前两阶段输入准备已由 DEC-PLAT-002-022 裁决；当前缺口是查询、物化、固定 input_snapshot_refs、创建 Run 的实际顺序和原子/失败恢复证据。 | `pending_evidence` | Contract Test 证明先准备/物化再创建 Run；Run 固定 refs，重试不重新查询或替换输入。 |
| NEWS-FLOW-C | 可信 caller_ref 注入及 purpose/content level/filters 固定方式已由 DEC-PLAT-002-022 裁决；当前缺口是 Server/Researchflow Adapter 的真实主体映射和参数指纹证据。 | `pending_evidence` | caller_ref 由 Server 可信上下文注入；查询条件由版本化 Template/Run 参数固定；News 授权主体匹配并通过消费者测试。 |
| NEWS-FLOW-D | Cursor 持有边界已由 DEC-PLAT-002-022 裁决；Cursor 由 News Adapter 持有，不进入 Run.input_snapshot_refs。当前缺口是 Adapter 分页续页和物化完整输入的实现证据。 | `pending_evidence` | Cursor 仅在同一查询 Snapshot 内由 News Adapter 使用；Run 固定物化 refs；重试复用物化输入。 |
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
| QNT-FLOW-A | QuantResultRef 与 Envelope 的资源所有权/读取边界已由 DEC-PLAT-002-023 裁决；当前缺口是 Quant Facade 按 Ref 返回必要 Envelope 元数据并执行当前授权复核的实现。 | `pending_evidence` | Quant Facade、Envelope Schema/Golden、授权复核和 Researchflow 消费者 Contract Test 通过。 |
| QNT-FLOW-B | QNT-CTR-004 已有部分 Factor QuantMetric、TrendResult、RiskDecision 的 DTO/Schema/Golden，但各 result_type/schema_version 的强类型 Payload 子 Schema 不齐；FLOW-006 只提供 Validator Registry 接口，没有生产 Payload Schema 注册 | pending_evidence | Quant 按实际首批可消费结果提供精确 Payload Schema、Golden 和强类型序列化证据；Researchflow 注册对应 Validator 并做消费者负例。不得用任意 map、宽泛 Schema 或 Notification Risk Event Schema 替代 |
| QNT-FLOW-C | QNT-CTR-005 的 Manifest/AuthorizationRef 结构与部分内容寻址测试已存在，但真实 Facade 读取、可信 Principal/Scope、当前授权复核、结果归属、撤销/竞态仍未完成 | pending_evidence | Quant Owner Facade 每次 GetResult 验证可信 Researchflow service principal、purpose/content scope、resource_ref=result_id 和当前策略；denied/unknown/expired/revoked 不返回 payload。补撤销与读取竞态、越权主体、结果归属和脱敏 Fault 测试 |
| QNT-FLOW-D | Manifest 已绑定 system_as_of 与 input_snapshot_refs，FLOW-006 要求 Envelope.system_as_of 与 Run 一致并校验输入快照引用，但固定基线无真实 Adapter/联合测试证明 Manifest、Envelope、Run 三者逐项一致 | pending_evidence | Adapter 校验 Envelope.system_as_of == Run.system_as_of，Envelope.input_snapshot_refs 与 Quant Manifest 完全一致，参数指纹、算法/Schema 版本与 Manifest 一致；错配拒绝，不能只检查字段非空 |
| QNT-FLOW-E | Stage.output_ref 投影规则已由 DEC-PLAT-002-023 裁决；当前缺口是 Quant ResultRef 与 Researchflow Stage 输出关联信息的实际映射。 | `pending_evidence` | Stage.output_ref 只保存 Quant 资源引用和编排关联信息，不复制完整 Payload；双方 Schema/Golden/Contract Test 一致。 |
| QNT-FLOW-F | Quant 模块 CI 只证明模块内代码；Researchflow 文档明确生产 Facade 接入、结果持久化和跨模块调用链未完成，固定基线未见 Quant Consumer Adapter、Server 真实装配和双方 Contract Test | pending_evidence | A/E 决策后补真实 Adapter、Server DI 装配、ResultRef 保存/读取、未知版本拒绝、授权拒绝、PIT/输入错配及重试复用同一结果引用等 L2 测试；模块 CI/Fixture 不替代联合验收 |

### 本批结论

- 职责与依赖方向：freeze-ready。Quant 拥有结果和授权语义，Researchflow 拥有流程/Stage 执行语义；无需改变模块依赖或创建通用 Provider 抽象。
- Manifest 内容寻址与授权资源绑定原则：accepted。Manifest 绑定结果类型、版本、system_as_of、输入快照和结果指纹；AuthorizationRef 绑定 result_id；读取时由 Quant Owner 复核当前授权。
- Ref→Envelope→Stage output_ref 的读取/投影关系：pending_decision。Quant 文档明确 Ref 与 Envelope 关系/GetResult 边界未定，而 FLOW-006 已按完整 Envelope 消费。
- 强类型 Payload Schema 与消费者 Validator：pending_evidence。部分 DTO 已有 Schema/Golden，不代表所有实际 result_type/schema_version 均可消费。
- 真实授权读取、PIT/Manifest 对齐、Adapter/Server 装配及联合测试：pending_evidence。当前没有足够证据宣称 Quant→Researchflow 达到 L2 或 frozen。
- 沿用 QNT-CTR-001～005、FLOW-CTR-003/004、GOV-CTR-002/006，不新增 CTR。Data→Quant 已登记问题不重复登记。

本批只更新 PLAT-002 审计记录；未修改业务代码、Schema、Golden、测试或执行 CI。后续统一处理 A/E 的有限契约决策；B/C/D/F 等待责任模块实现证据后增量核验，不重复本轮算法或 DTO 审查。


## 30. Research→Researchflow 研究对象消费边界首轮核验（2026-11-10）

- 固定代码审计基线：origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086；维护分支：feat/design_doc。
- 范围：RSC-CTR-001/002 与 FLOW-CTR-003/004 的组合边界；核对 Context 创建/读取、Evidence/Claim 精确引用、授权、Run 时间传播、Stage 输出和重试语义。
- 不重复 Data→Research 已审查的 available_at 来源和 Snapshot→Evidence 映射；不审查 News Owner 内部查询实现，不修改 Research/Researchflow/Server 业务代码，不新增 CTR。

### 已确认的契约边界

1. Research 拥有 Context/Evidence/Claim/Event 等研究语义及版本；Researchflow 拥有 Run/Stage/Attempt、分区选择、预算裁剪和流程编排。Research 不依赖 Researchflow；跨模块只能通过 Research Application DTO/Port。
2. Researchflow 的 FLOW-005 已提供纯映射函数 ToResearchCreateContextCommand：将 Researchflow 的候选分区、Plan、预算结果与 Research 提供的 section version/quality/assembled_at、Context QualitySummary 映射为 Research CreateContextCommand。该函数不创建 Context、不获取正文、不做授权；Server/Worker 负责调用 Research Port。
3. Context.system_as_of 必须与 Run.system_as_of 完全一致；Context 的 section_refs 保留全部已评估分区，included/omitted 仅表达本次模型上下文选择。Research 负责分区语义/质量，Researchflow 负责选择和裁剪。
4. Evidence.system_as_of 可早于 Run，但不得晚于 Run；严格 PIT 使用 Run cutoff 校验。Evidence 是 Research 内不可变资源，evidence_id 为精确身份；其 source_owner 为 data/news/quant/research 时，授权快照由对应 Owner 签发，读取受限内容必须由 Owner 当前复核。
5. Research Application GetEvidence 接受 evidence_id、可信 caller_ref、purpose_code，并调用 EvidenceAuthorizer 后才返回 DTO；AuthorizationRef 本身不是 bearer token。Researchflow ComposeResult 已要求 Evidence 派生引用和 Claim 支持/反证引用在本次输入集合内闭环。
6. Claim 由 Research 持有版本，ClaimRef 使用 claim_id + claim_version；Researchflow 不应自行拼接或推断 Claim 版本，也不直接依赖 Research Domain/Repository。
7. Researchflow ComposeResult 只组合 ResearchContext/EvidenceRecord/Claim DTO，不重写研究事实；最终 Stage 结果仍须遵守 FLOW-CTR-004 的 Orchestrator Envelope、领域 typed payload、质量/诊断 sidecar 三层边界。

### 差异登记（审计工作项，非新增 CTR）

| 编号 | 固定基线事实 | 状态 | 冻结影响 / 后续统一处理 |
|---|---|---|---|
| RSC-FLOW-A | Research Stage 写操作幂等范围已由 DEC-PLAT-002-024 裁决；当前缺口是被允许的写操作实现稳定幂等键、请求指纹和响应丢失对账。 | `pending_evidence` | 仅对允许 Stage 调用且有业务副作用的写操作实现跨 Attempt 稳定幂等；同指纹返回原 Ref、异指纹冲突、响应丢失可对账。 |
| RSC-FLOW-B | 按 Run 固定 Ref 精确读取及失败策略已由 DEC-PLAT-002-024 裁决；当前缺口是 Researchflow Adapter 对固定 Context/Evidence/Claim refs 的加载、顺序和失败语义实现。 | `pending_evidence` | 按精确 Ref/版本读取；缺失、版本冲突、授权拒绝默认 fail-closed；仅版本化 Template 可声明降级。 |
| RSC-FLOW-C | Context 元数据受控访问原则已由 DEC-PLAT-002-024 裁决；当前缺口是可信 caller/purpose、可见范围和当前授权检查在 Research Port/Adapter 中的实现证据。 | `pending_evidence` | Context 读取具备可信调用身份/purpose 和明确可见范围；ContextRef 不替代 Evidence 授权；跨模块授权测试通过。 |
| RSC-FLOW-D | Claim Application Create/Get/Update 目前只做领域结构/版本校验和 Repository 读写；ClaimRef 不含 caller/purpose，Claim DTO 没有 AuthorizationRef，CreateClaim 不校验 supporting/contradicting EvidenceRefs 与 ScopeRefs 的真实存在/版本，Claim Get 也不执行关联 Evidence Owner 当前授权。现有 RSC-CTR-002/RSC-005 已登记 Evidence/Scope 真实引用解析未完成 | pending_evidence | Researchflow 不得把 Claim DTO 中的 refs 当作已授权内容或已解析事实。消费时必须按精确 Evidence 引用调用 GetEvidence 并由来源 Owner 复核，再由 Composer 校验支持/反证闭环；Claim 自身的读取范围和引用完整性证据由 Research Owner 补齐。沿用 RSC-CTR-002/RSC-005，不重复建立模块内缺口 |
| RSC-FLOW-E | 仅对明确允许 Stage 调用且产生业务副作用的写操作要求幂等，已由 DEC-PLAT-002-024 裁决；当前缺口是该范围内各写操作的逐项实现/测试映射。 | `pending_evidence` | 登记允许 Stage 调用的写操作清单，并逐项验证幂等键、规范化指纹、重放/冲突及提交后响应丢失对账。 |
| RSC-FLOW-F | 固定基线只有 Context DTO 投影和纯组合函数；FLOW-005 明确生产 Owner Adapter、Research Facade、Context 持久化调用和真实 Tokenizer 尚未接入。FLOW-006 明确生产 Research Evidence/Claim Facade Adapter、当前授权复核和引用闭环尚未接入；未见 Research Consumer Adapter、Server 装配和双方联合 Contract Test | pending_evidence | 在 A/B/C 决策后实现真实 Research Facade Adapter 与 Server/Worker 装配。L2 至少覆盖 Context Create/Get 版本一致、Run.system_as_of 固定、Evidence Owner 授权拒绝/撤销、Claim 精确版本、引用闭环、部分失败策略、Stage 输出引用持久化和 Attempt 重试不重复写入。Fixture/纯函数测试不替代真实跨模块证据 |

### 本批结论

- **职责与依赖方向：freeze-ready**。Research 拥有研究对象语义，Researchflow 拥有编排/选择/预算语义；无需调整模块 DAG 或复制 Research Domain。
- **Context 字段映射和 Run 时间传播：accepted**。已有纯 DTO 投影；Context.system_as_of 必须等于 Run，Evidence.system_as_of 允许早于 Run 但不得晚于 Run。投影函数不承担授权和持久化。
- **Context 元数据访问边界、Context/Claim 写入幂等、按固定 Ref 集合加载策略：pending_decision**。这些直接影响 Stage 重试和结果可追溯性，需在正式消费契约冻结前明确。
- **Claim 引用完整性、Owner 授权复核和真实生产消费链路：pending_evidence**。Researchflow 组合器的引用闭环校验不等于 Research Owner 已验证底层 Evidence/Scope 存在或仍可访问。
- 沿用 RSC-CTR-001/002、FLOW-CTR-003/004、GOV-CTR-003/005/006，不新增 CTR。Data→Research 的 available_at、Snapshot 来源映射及 News→Research 的授权来源缺口不重复登记。

本批只更新 PLAT-002 审计记录；未修改业务代码、Schema、Golden、测试或执行 CI。后续统一处理 A/B/C/E 的有限契约决策；D/F 等待 Research Owner 和 Server/Researchflow 生产接入证据后增量核验，不重做本轮已确认的职责、时间和 DTO 投影审查。


## 31. Researchflow→LLM 模型调用与工具往返边界首轮核验（2026-11-10）

- 固定审计基线：origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086；维护分支：feat/design_doc。
- 范围：LLM-CTR-001～003 与 FLOW-002/004/005/008 的组合边界；核对 ModelRequest 映射、Run/Stage 上下文传播、Prompt/Template 版本、Tool Call 往返、重试与流式终止、错误投影和生产装配证据。
- 不重复审查 LLM 内部 Responses JSON/SSE Parser、JSON Schema 编译器和 Tool Policy 领域规则；不修改 LLM/Researchflow/Server 业务代码，不新增 CTR。

### 已确认的契约边界

1. Researchflow 是模型调用编排方，LLM 是单次模型协议适配方。LLM Generate/Stream 不决定业务重试，不执行工具，不授权工具，不解释业务结论；工具白名单、Scope、参数 Schema、预算和执行归 Researchflow。
2. Researchflow 每次 Stage/Attempt 调用必须继承 Run.system_as_of、deadline_at、input_snapshot_refs、template_ref/version/fingerprint 和 correlation_id；每次实际执行使用独立 request_id。LLM ModelRequest/Response 携带并原样回显 request_id、correlation_id、system_as_of；deadline 取 ModelRequest、Context 和 Provider timeout 的最早值。
3. LLM ModelRequest 固定首批单轮 instructions + input_text；不接受任意 Responses input item、多轮、previous_response_id、conversation_id 或多模态。ToolDefinition 是 Registry 筛选后的只读声明，LLM 只解析 function_call 意图。
4. Researchflow ToolPolicy 已有 Registry、Scope/Schema/预算校验、Intent 转换和 AuthorizedCall；模型可见工具列表不等于授权，响应必须再次 Authorize。AuthorizedCall 带调用 ID、参数快照、幂等键和受 Run Deadline/预算裁剪的执行时限；实际 Executor 仍须绑定 Run/Stage/Attempt 并二次校验幂等。
5. LLM 错误只提供 retryable/Retry-After 技术提示，是否重试由 Researchflow 的不可变 Template RetryPolicy 白名单、max_attempts、deadline 和预算决定。请求可能已被上游接收但响应丢失时不得假定未执行；Stream 收到首个事件后禁止透明重试，未收到 terminal event 的 EOF 不能视为成功。
6. ModelResponse 只有 completed/incomplete/failed 三态；只有 completed 响应可以转为工具意图。Stage 输出 Envelope 的执行关联字段由 Orchestrator 注入，Provider 不得覆盖 Run/Stage/Attempt 身份。

### 差异登记（审计工作项，非新增 CTR）

| 编号 | 固定基线事实 | 状态 | 冻结影响 / 后续统一处理 |
|---|---|---|---|
| LLM-FLOW-A | ResolvedTemplate、模型配置来源及 ModelRequest 组装责任已由 DEC-PLAT-002-025 裁决；当前缺口是 Researchflow Adapter/Server 配置注入和 Prompt/Template/Schema fingerprint 的生产映射。 | `pending_evidence` | Run 固定版本化 ResolvedTemplate；Server 受控注入 Provider/Model；每 Attempt 独立 request_id；Schema/Golden/Adapter 测试通过。 |
| LLM-FLOW-B | LLM 首批协议明确单轮 instructions + input_text，并将多轮/previous_response_id/任意 input item deferred；同时允许 ModelResponse 返回 function_call 意图。Researchflow ToolPolicy 能授权并生成 AuthorizedCall，但当前没有已定义的 ToolResult → 下一次 ModelRequest 输入映射/Responses continuation 形式 | deferred | 明确 MVP 工具调用语义：若支持完整工具循环，必须定义受限 ToolResult DTO、call_id 关联、工具结果 schema/大小限制、模型续轮请求及多轮状态/重试语义；若维持单轮契约，则明确 Tool Call 仅为 Stage 终端建议/待执行意图，不允许执行后再声称模型已消费工具结果。不得把工具结果拼入自由文本伪装为已支持的 continuation |
| LLM-FLOW-C | LLM stable error 到 Researchflow Retry ReasonCode 的分类原则已由 DEC-PLAT-002-025 裁决；当前缺口是具体错误码映射、RetryPolicy 执行及 ambiguous outcome 防重放测试。 | `pending_evidence` | 显式错误码映射；retryable 不单独授权重试；ambiguous/incomplete/schema/auth/parameter 错误不自动重放；限流遵循 Retry-After 和 Run 预算。 |
| LLM-FLOW-D | ToolPolicy AuthorizedCall 的幂等键和 deadline 已生成，但对象本身不包含 Run/Stage/Attempt/Worker/fencing 身份；Policy 注释要求实际执行器二次绑定身份并做幂等控制。固定基线没有 Tool Executor Port/持久化执行记录、工具结果回写和 ambiguous outcome 对账的生产证据 | pending_evidence | 真实 Executor 必须从可信 StageExecutionRequest 注入 Run/Stage/Attempt/fencing，派生稳定下游 idempotency_key 与 request fingerprint；持久化调用意图/执行结果或可查询对账记录；重试不得重复产生非幂等外部副作用。Scope/Schema 重新校验和 deadline 由 Researchflow 执行层落实 |
| LLM-FLOW-E | LLM-CTR-001～003 仍为 pending_evidence；有 ModelRequest/Response、Schema 和 ToolPolicy 单测，但文档明确真实 Gateway Generate/SSE/模型能力 E2E 未完成；Capability 默认 unknown，需绑定 Provider/Model/Protocol/Gateway 版本和有效期 | pending_evidence | 为实际首批 Provider/Model/Gateway 版本提供真实 Generate/Stream、Tool Calling、Structured Output、Usage、取消/超限证据；unknown/过期能力必须拒绝对应请求。不能以路由可达、Mock 或单模块 Fixture 标记 supported |
| LLM-FLOW-F | Researchflow FLOW-005 记录真实 LLM Adapter 未接入；Worker 只有通用 StageExecutor Port，LLM Adapter、Server DI/Secret 注入、Platform BoundDoer/OpenStream 包装、调用结果到 Stage typed output 的完整链路未形成固定基线证据 | pending_evidence | A/B/C 决策后补 Researchflow LLM Stage Provider Adapter、Server 组合根装配、Platform HTTP 流式接口包装、错误/取消映射和 Stage Output Registry 注册；联合测试覆盖上下文原样传播、工具拒绝/往返策略、重试、SSE terminal/断流和 fencing 迟到结果拒绝 |

### 本批结论

- **职责与依赖方向：freeze-ready**。Researchflow 编排模型调用和工具，LLM 只负责模型协议适配；不需要 LLM 反向依赖 Researchflow，也不需要把工具执行迁入 LLM。
- **基础调用上下文语义：accepted**。system_as_of、deadline_at、correlation_id、request_id 的职责已明确；每次 Attempt 独立 request_id，Run 的知识截止/关联 ID 在重试中保持不变。
- **ModelRequest 配置/Prompt 版本映射、Tool Call 是否支持完整往返、LLM ambiguous outcome 的重试分类：pending_decision**。这三项影响执行正确性，不能仅靠补测试解决。
- **Tool Executor 生产幂等、真实模型能力、Adapter/Server 装配和联合 Contract Test：pending_evidence**。模块内 ToolPolicy 与 LLM Parser 已有实现不代表跨模块链路完成。
- 沿用 LLM-CTR-001～003、FLOW-CTR-002/003/004、GOV-CTR-003/004/005，不新增 CTR。LLM 内部 Schema/Parser 审查和 Researchflow Tool Policy 规则不重复登记。

本批只更新 PLAT-002 审计记录；未修改业务代码、Schema、Golden、测试或执行 CI。后续统一处理 A/B/C 的有限契约决策；D/E/F 等待责任模块生产接入和实测证据后增量核验，不重复本轮协议 Parser/工具权限设计审查。


## 32. 业务事件生产方→Notification 首批事件交互核验（2026-11-10）

- 固定审计基线：origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086；维护分支：feat/design_doc。
- 核验范围：INT-CTR-001、NTF-CTR-001～003、GOV-CTR-003/004；只检查 Researchflow/Quant/Research 事件生产边界、Outbox 到 Notification Envelope 的映射、Inbox 原子接收和 Server Relay 证据。不审查 Notification 内部 Subscription/Renderer/Channel 状态机，不扩展首批事件范围。

### 已确认的统一契约

1. 首批仅三类业务事件：`researchflow.run.terminal`、`quant.risk_decision.changed`、`research.event.lifecycle_changed`，均为 schema_version `1.0.0`。不发布 Run 运行进度、普通指标、Evidence/Claim/Hypothesis 变化。
2. 业务事实生产方拥有事件触发条件、业务时间、稳定发布意图、event_id、correlation_id 和本地 Outbox；Notification 负责校验 Envelope/Payload、Inbox 双键幂等和异步 fan-out。生产方不直接调用渠道、不写 Notification Delivery，也不依赖 Notification Domain/Repository。
3. 生产方在业务事实与 Outbox 同一事务提交；跨模块交付采用 at-least-once。Outbox 重试复用同一 event_id/idempotency_key/occurred_at/published_at/correlation_id/payload/fingerprint；Notification Inbox 以 (source_module,event_id) 和 (source_module,idempotency_key) 双唯一键去重。相同键不同内容 fail closed。
4. Envelope `event_id` 与领域对象 ID 分属不同身份空间；`occurred_at` 来自事实发生/状态迁移时间，`published_at` 是首次写入 Outbox 的时间；`correlation_id` 来自可信父链路；`payload_fingerprint` 按 INT/NTF 规定的字段集合执行 RFC 8785 JCS + SHA-256。
5. Notification `PublishEvent` 成功仅表示 Inbox 接收/幂等吸收，不代表订阅匹配、渠道调用或用户已读；不承诺外部 exactly-once。渠道结果 ambiguous 时由 Notification 自己进入 unknown 并对账。

### 生产方与消费者代码核验

- **Research：部分对齐。** `internal/research/storage/event_repository.go` 的 `AppendWithLifecycle` 使用版本追加事务 Hook，将 Event 新版本与 Outbox 同事务提交；首次创建和真实状态迁移均写 Outbox，普通版本修订不生成 lifecycle_changed。`internal/research/storage/outbox.go` 已构造完整首批 Payload/Envelope 字段、独立随机 event_id、稳定版本级 idempotency_key、occurred_at=changed_at、published_at、correlation_id，并按 JCS + SHA-256 计算 payload_fingerprint；Outbox 有领取、租约、重试、确认/拒绝机制。研究模块文档记录 Schema/Golden/Contract Test 与专用 MariaDB 验收通过。缺口是 Server→Notification Relay 尚未接线，且未见当前固定基线的真实 PublishEvent 端到端调用证据。
- **Researchflow：存在明确实现偏差。** `internal/researchflow/storage/mysql_run_finalize.go` 在 Run 终态、ResultSummary 和 Outbox 同事务提交，原子性方向正确；但代码常量是 `researchflow.run.finalized`，与 INT-CTR-001 的 `researchflow.run.terminal` 不同；Payload 使用 `run_status` 而不是 `status`，且未提供 `source_event_ref`、`system_as_of`、`template_ref`、`template_version`，`termination_reason` 空值时被省略而不是显式 null；Outbox 记录没有独立 `event_id`、`occurred_at`、`published_at`、`subject_ref` 等完整 Envelope 字段。其 fingerprint 是对原始 eventPayload 直接 SHA-256，不是规定的 Envelope 业务字段 JCS fingerprint。`OutboxMessage` 也未携带构造 Notification Envelope 所需的完整字段。当前实现不能直接投递为 INT-CTR-001 事件。
- **Quant：生产 Outbox 缺失。** `internal/quant/risk/decision_builder.go` 仅构造领域 RiskDecision；`internal/quant/result/repository.go` 只有结果写入 Port，要求原子唯一性，但固定基线未见 Quant Storage/Migration/风险决策事务 Outbox 实现。Quant 文档已声明风险事件应在风险事实本地事务写 Outbox，但这是目标契约/任务要求，不是当前实现证据。首次决策与实质变化的比较、风险事实持久化和 Outbox 原子提交均未形成可核验链路。
- **Notification：接收核心已建立，Fanout 生产写入未闭合。** `EventReceiver` 在持久化前验证完整 Envelope/Payload；`MySQLInboxRepository` 双唯一键处理重放，并要求 `InboxFanoutWriter` 在同一事务写 Fanout 工作项。固定基线只有 Writer 接口/扩展点，README 明确 Fanout 表/生产 Writer、WorkItem claim/ack/retry、活跃 Subscription 分页和完整 Dispatcher/Worker 尚未完成；不能将 Inbox Repository 存在等同于完整消费链路。
- **Server：Relay/装配缺证据。** Research Outbox 与 Researchflow Outbox 各自提供领取/确认/重试接口，但未见 Server 将它们映射为 Notification `EventReceiver.Receive`/公开接收 Port 的统一 Relay、DI 装配、共享 DB/事务资源配置和故障恢复集成证据。Quant Outbox 生产端也尚不存在。

### 差异登记（审计工作项，非新增 CTR）

| 编号 | 固定基线事实 | 状态 | 冻结影响 / 后续统一处理 |
|---|---|---|---|
| BUS-EVT-A | Researchflow 终态事务 Outbox 的事件类型、Payload 字段/空值、Envelope 身份字段和 fingerprint 算法与 INT-CTR-001/NTF-CTR-001 不一致；OutboxMessage 缺完整 Envelope 字段 | pending_evidence | 既有 INT-CTR-001 决议已明确，无需新增语义决策。Researchflow Owner 按 `researchflow.run.terminal`、既定 Payload、独立 event_id、occurred_at/published_at/subject_ref、JCS fingerprint 修正 Outbox 和 Publisher DTO；保持 Run+ResultSummary+Outbox 同事务。补终态四状态、显式 null、重放相同 Envelope、指纹 Golden 和 Notification Validator 联合测试。未修正前禁止将该 Outbox 接入 Notification |
| BUS-EVT-B | Quant 只有 RiskDecision 领域构造与结果 Repository Port，未见风险事实持久化和 Outbox 原子提交实现；事件触发规则虽已在 INT-CTR-001/Quant 文档确定，但没有真实持久化比较链 | pending_evidence | Quant Owner 实现首次决策/实质变化识别、稳定 subject_ref、事件 Payload/Envelope 与风险事实同事务 Outbox；warning/info-only 变化不触发，多主体按主体拆分，重复提交保持同一发布意图。补数据库事务、并发决策、状态/硬阻断原因集合变化和 Outbox 回滚测试。不得由 Server 或 Notification 推导风险变化 |
| BUS-EVT-C | Research Event 版本与 Outbox 已同事务，Envelope/Payload/JCS/稳定幂等键与 INT-CTR-001 基本对齐；缺少 Server Relay 的实际接收调用与跨模块 L2 证据 | pending_evidence | Research Owner 保持现有领域 Outbox；Server 提供受控 Relay，将原始完整 Envelope 传给 Notification 接收 Port，成功后确认 Outbox，失败按原 Envelope 重试；覆盖接收成功但确认丢失、重复 Publish、Inbox 幂等和 Outbox lease/fencing。不得重建 event_id 或刷新 published_at |
| BUS-EVT-D | Notification EventReceiver/MySQLInboxRepository 已校验 Envelope、双键幂等，并要求事务内 FanoutWriter；固定基线尚无生产 FanoutWriter/工作项表和完整 Worker 消费链路 | pending_evidence | Notification Owner 完成与既有 INT-CTR-001 兼容的 Fanout 工作项写入及 claim/ack/retry，确保 Inbox 首次接收与 Fanout 同事务；重复事件不重复 Fanout，唯一键冲突/异指纹 fail closed。仅阻断真实自动通知链路，不阻断 Notification Domain/Repository 独立开发 |
| BUS-EVT-E | 固定基线未见 Server 同时装配 Research/Researchflow Relay 与 Notification Receiver 的端到端链路；Quant 生产端亦缺失 | pending_evidence | Server 负责装配各生产方 Outbox Publisher、Notification EventReceiver 和 Worker 生命周期；不得承载事件业务规则或重写 Payload。联合测试覆盖三生产方字段映射、Outbox→Relay→Inbox、进程崩溃/确认丢失/重复投递、correlation_id 和错误边界；真实 MariaDB/根集成 CI 证据单独记录 |

### 本批结论

- **事件业务职责与依赖方向：freeze-ready。** Research/Quant/Researchflow 拥有各自事实和事件，Notification 只消费版本化 Envelope；Server 只装配 Relay，不拥有事件语义。
- **首批事件语义与可靠交付规则：accepted。** 三类事件、触发条件、Payload、Envelope、Outbox/Inbox 幂等、业务时间和安全字段边界已由 INT-CTR-001 决议，不新增业务事件类型。
- **Research 生产端：实现基本对齐，Server Relay pending_evidence。** Research 事件不需要重做 Outbox。
- **Researchflow 生产端：pending_evidence，且有对已接受契约的实现偏差（BUS-EVT-A）。** 这是责任模块实现修复，不是 PLAT-002 改契约。
- **Quant 生产端：pending_evidence。** 尚无风险事实到 Outbox 的持久化事务实现。
- **Notification Fanout 与 Server Relay：pending_evidence。** Inbox 接收入口不代表完整通知链路已通。
- 沿用 INT-CTR-001、NTF-CTR-001～003、QNT-CTR-005、RSC-CTR-003、FLOW-CTR-001/003、GOV-CTR-003/004，不新增 CTR。Research 已有 Outbox 证据不重复登记为新缺口；不审查通知渠道/Renderer 内部规则。

本批只更新 PLAT-002 审计文档；未修改任何业务模块代码、Schema、Golden、测试或执行 CI。后续按责任模块修复/补证据后增量核验，不在 PLAT-002 分支代修。

## 33. WebUI→Server 对外 HTTP API 契约边界首轮核验（2026-11-10）

- 固定审计基线：origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086；维护分支：feat/design_doc。
- 核验范围：WebUI `web/src/api.js` 与 `doc/web/API_CONTRACT.md` 实际请求；Root Server `internal/server/bootstrap` 路由、认证、中间件；Market Data Adapter 路由；`doc/api/contracts/分析任务.openapi.yaml`；Web Nginx 代理配置及现有前后端测试。仅审 HTTP 契约、公开 DTO/Schema、错误/认证和真实路由装配证据。

### 已确认的边界与现状

1. WebUI 通过同源 Nginx 前缀分别调用 `/market-api` 与 `/analysis-api`；Nginx 将两者转发到独立 Market Data（8091）和 Analysis/Root Server（8092）服务，且转发 X-Request-ID、代理超时。固定基线 Root Server 并未装配 Market Data HTTP Adapter；`internal/marketdata/httpapi.Server.InProcessHandler()` 只是可供可信 Root Server 挂载的无认证业务路由，不能视为已挂载。
2. Market Data Adapter 当前 `Handler()` 自带 X-API-Key 认证；WebUI `loadSettings/saveSettings` 将 apiKey 保存到 localStorage，并由 `request()` 对行情请求添加 `X-API-Key`。这属于迁移期兼容模式，文档已明确不是目标安全模型；API_CONTRACT 也明确目标是 Root Server 注入 Data Adapter、浏览器不持有 Key。不能把当前浏览器存 Key 视为可冻结的目标认证契约。
3. Analysis WebUI 使用 `POST /api/v1/analysis/tasks`、`GET /api/v1/analysis/tasks/{task_id}`、`GET /api/v1/analysis/tasks/{task_id}/events` 和 `GET /api/v1/analysis/runs/{run_id}`。Root Server Router 已注册这些路由（Run 查询及事件读取亦已注册），但全部使用 `OwnerAccess`；WebUI 请求不发送 Owner Bearer，只使用 same-origin credentials。因此当前浏览器 Analysis 请求按设计会得到 401，并非可用的浏览器登录会话接口。
4. Root Server 安全边界当前明确：`/healthz` 匿名；`/readyz`、`/diagnostics` 和全部 Analysis 业务路由 Owner Bearer；Service Scope 不可访问 Owner 路由。Root 中间件重生成 Request ID，不信任调用方 X-Request-ID；公开 Fault Envelope 含 class/code/message/retryable，Request ID 通过响应头回传。WebUI 能从结构化错误和 X-Request-ID header 解析状态、code 和 requestId。
5. 分析 OpenAPI `doc/api/contracts/分析任务.openapi.yaml` 版本 1.0.0 仅定义 Tasks 创建/查询/事件三类路径，缺少 WebUI 已使用且 Root 已注册的 `/api/v1/analysis/runs/{run_id}`；TaskEvent.event 仅为 opaque object；AnalysisRequest 字段未列 required/enum/严格格式边界；未定义 Bearer securityScheme、401/403 及统一 Error Envelope/Request ID header。OpenAPI 不能完整代表当前 HTTP 边界。
6. WebUI `api.test.mjs` 通过 mock fetch 验证请求路径、Idempotency-Key、Market-only X-API-Key、错误码和 Request ID 解析；Root `router_test.go` 验证 Owner/Service/匿名路由权限；Root middleware 测试验证 Request ID 重生成、请求大小、过载和 Fault 安全输出。两侧各有单测，但固定基线未见 OpenAPI 驱动的跨端 Contract Test，也没有浏览器登录会话→Server 用户身份→资源授权的端到端证据。

### 差异登记（审计工作项，非新增 CTR）

| 编号 | 固定基线事实 | 状态 | 冻结影响 / 后续统一处理 |
|---|---|---|---|
| WEB-SRV-A | Market Data 仍为独立 8091 Adapter，WebUI 浏览器保存 apiKey 到 localStorage 并直接发送 X-API-Key；Root Server 未装配 Market Data Adapter。目标文档要求 Server 注入 Key/可信调用上下文，浏览器不得持有 Provider/API Key | pending_evidence | 沿用 MDM-006/PLAT Security 决议；Server/Platform/Data 完成 Data Adapter 迁移、服务端密钥注入、统一认证和部署路由后，WebUI 移除旧 localStorage Key 读写/请求头注入并轮换已暴露凭据。迁移完成前此接口仅为受限兼容路径，不得作为目标认证契约冻结 |
| WEB-SRV-B | Analysis 路由全部 OwnerAccess，WebUI 不携带 Owner Bearer，当前浏览器请求预期 401；Web Session/Cookie、CSRF、登录限流和身份分离语义已由 DEC-PLAT-002-017 决议；当前缺 Server/WebUI A2 接入证据 | pending_evidence | 按 DEC-PLAT-002-017 实现 Server A2 登录/退出、Session Middleware、Cookie/CSRF/Origin、Credential Version、路由认证矩阵与资源授权上下文；WebUI 实现 Cookie/CSRF 请求和登录退出交互，补浏览器 Session E2E。不得把 Owner Bearer 下发浏览器 |
| WEB-SRV-C | Analysis 旧 API 不保留兼容期；新 Researchflow API 是唯一 WebUI 目标，OpenAPI 必须覆盖新路由、请求/响应、认证、错误和 Run/Task/Event Schema。当前缺口是新契约与 Handler/WebUI 的一致性证据。 | `pending_evidence` | 新 API OpenAPI 与 Handler/WebUI/跨端 Contract Test 一致；新 API 验收后删除旧 Analysis 专属路由和适配。 |
| WEB-SRV-D | Root Server 当前只装配 Legacy Analysis API/Worker 与 Platform Health/Router；Researchflow Application Facade/任务查询、模块 DTO 到 HTTP DTO 的正式 Adapter 尚未作为目标模块链路接入；Market Data 也未挂载 Root Server | pending_evidence | Server 作为唯一组合根，分别装配 Researchflow Facade/HTTP Handler 和 Data Facade/Adapter；Handler 只做校验、映射和 HTTP 语义，不拥有任务/行情领域规则。需要明确迁移期间旧 Analysis API 的兼容/退役策略，禁止将 Legacy Task DTO 直接冻结为最终 Researchflow 公共模型 |
| WEB-SRV-E | WebUI 请求封装测试和 Root Router/Middleware 单测分离存在；没有同一 OpenAPI Schema 驱动的 WebUI→Handler 契约验证，也没有 Session/资源授权、代理部署前缀、Request ID 和错误 Envelope 的端到端验收 | pending_evidence | 增加以权威 OpenAPI/JSON Schema 为基准的 Handler Contract Test，并由 WebUI API 测试校验请求/响应边界；集成测试覆盖 Nginx 前缀剥离、Cookie/CSRF、401/403/404/409/413/429/503、错误码与 Request ID、任务幂等和事件游标。不要只靠两端各自 Mock 单测宣布联调通过 |
| WEB-SRV-F | Market Data 与 Root Server 的错误 Envelope 细节不同：Market Data error 可在 body `error.request_id` 中携带 ID，Root Server publicError 不含该字段而依赖 `X-Request-ID` header；WebUI 当前兼容两者。Platform 公共契约已规定最终 Request ID 通过响应头返回，Body 字段仅作为旧接口兼容，不再构成项目级语义待决 | pending_evidence | 按 doc/platform/08 第5节以响应 X-Request-ID 为权威，保留旧 Body request_id 的兼容读取；统一各 Transport 的 Fault→HTTP 映射和 Golden/Contract Test，不向浏览器暴露内部 dependency/cause |

### 本批结论

- **浏览器与 Server 的职责边界：freeze-ready。** WebUI 只消费公开 HTTP DTO；Server 负责认证、授权、路由、DTO 映射和错误投影；领域语义归 Data/Researchflow 等生产模块；Platform 提供认证、Fault、Request ID 等技术能力。
- **当前路由与安全状态：accepted。** Market Data 仍独立部署并由 API Key 保护；Analysis 由 Root Server Owner Bearer 保护，WebUI 不携带 Owner Token，因此当前浏览器 Analysis 请求预期 401。这是已记录的迁移状态，不代表目标浏览器访问链路已完成。
- **浏览器认证模型、完整 Analysis OpenAPI、错误 Request ID 规范：pending_decision。** 需要明确的公共 API 决议，不是单纯补测试。
- **Data 迁移/密钥移除、模块 Facade 装配、跨端 Contract Test 与部署验收：pending_evidence。** OpenAPI 声明、Handler 注册、WebUI 封装、Root 装配、代理可达和真实认证成功必须分别验收。
- 不新增 CTR；沿用 MDM-006、PLAT Security/Fault/Observability 契约、FLOW-CTR-001/003/004、GOV-CTR-001/002/003。此处不重复行情字段/Provider 语义或 WebUI 视觉审查。

本批只更新 PLAT-002 审计记录；未修改 WebUI/Server/Data 业务代码、OpenAPI、测试或执行 CI。

## 34. Server 组合根与 Platform 横向技术契约接入核验（2026-11-10）

- 固定审计基线：origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086；维护分支：feat/design_doc。
- 核验范围：`internal/server/bootstrap`、`resources`、`runtime`、`migration`、`diagnostics`、`health`、`cmd/analysis-api`，Platform Config/Storage/Security/HTTP Client/Fault/Observability/Diagnostics 公共原语，以及独立 `cmd/market-data` 资源入口。只核对组合根装配和技术契约复用，不审模块内部领域实现。

### 已有实际接入与正确边界

1. Root `cmd/analysis-api/main.go` 已唯一委托 `internal/server/bootstrap.Run`，未新增第二个 Root Server 入口。Bootstrap 集中读取进程配置，创建共享 SQL/Redis/Platform HTTP Client，执行 Migration 预检/迁移，构造 Legacy Analysis Worker/HTTP Handler，并将组件注册到 Runtime Lifecycle。
2. Server Resources 使用 Platform `storage.OpenSQL/OpenRedis` 创建共享连接资源，按 HTTP Client→Redis→DB 顺序关闭；初始化失败按逆序清理。`stopConsumersThenCloseResources` 保证 Lifecycle 消费者停止后再关闭共享资源。
3. Runtime Lifecycle 按注册顺序启动、逆序停止；启动失败自动回滚，Stop 等待 Start 完成并聚合停止错误。当前注册顺序是 Legacy Worker 后 HTTP Server，因此关闭时先 HTTP Server 再 Worker，最后 Bootstrap 关闭共享资源。对应单测覆盖顺序、失败回滚、并发 Stop/Start 和错误保留。
4. Server 通过 Platform Security `Authenticator/Router/RoutePolicy` 注册显式路由策略；通过 Platform Observability Middleware 生成 Request ID；通过 Platform Fault 映射安全错误；通过 Platform Diagnostics Runner 执行并聚合 Server 自有 Storage/Schema Check。Server 自己实现 HTTP 入口、路由组合、公开错误 DTO 和业务依赖 Check，未复制 Platform 的通用 Runner/Auth/HTTP Client 核心。
5. Server Migration Registry 只收集模块提供的 Migration 定义，按 namespace 排序后调用 Platform `ApplyMigrations`；SQL 内容仍归模块。`MIGRATION_ENABLED=false` 时执行只读已应用版本预检。此职责方向符合 Server/Platform/模块分层。
6. Platform HTTP Client 具备受控 DestinationPolicy/BoundDoer/流式能力；Server Resources 提供固定策略绑定入口并在关闭后拒绝新绑定。此为可用装配能力，不等同于已存在 Provider/LLM 生产消费者。

### 生产装配与复用缺口

1. **Root 组合根尚处迁移态。** `bootstrap.Run` 当前实际创建 Legacy Analysis Runner/Queue/API；未见 Data、Quant、Research、Researchflow、LLM、Notification 的完整公开 Facade/Adapter/Worker 接入。`newMigrationRegistry` 的真实注册集合只有 `news`、`research`、`notification` 三个 namespace；测试明确断言该集合。当前不能把目录、模块 Port 或迁移规划视为 Server 已装配。
2. **Data 仍是独立进程和独立资源所有者。** `cmd/market-data` 自行加载配置、`repository.OpenMySQL` 直接 `sql.Open` 并设置连接池；Redis 通过 `marketdata/cache.Redis` 自定义 RESP 客户端逐命令建连。Repository 的 Migration 调用虽复用 Platform `ApplyMigrations`，连接池创建/生命周期及 Redis 客户端未复用 Root Server 的 Platform Storage 资源。该状态与此前 WebUI→Server 记录的独立 Data 服务一致，是迁移未完成，不是 Server 再造一套 Platform。
3. **Platform HTTP Client 尚缺生产消费者证据。** Root Resources 创建共享 Client，提供 `BindHTTPDestination`；本轮固定基线中 Bootstrap 未绑定具体上游，也未把 BoundDoer 注入 Data/News/LLM 等生产构造入口。Resources 单测只证明绑定能力，不证明模块 Provider/模型调用已走 Platform 受控链路。
4. **Web Session 原语尚未由 Server 接入。** Server 当前只构造 Bearer `Authenticator` 并使用 Owner RoutePolicy；没有 Cookie Session/CSRF/登录/退出/限流/审计流程接线。Platform Session 等 L1 原语存在，但 Server A2 未完成。该差异已由 WEB-SRV-B 登记，本节不重复建立差异项。
5. **配置装配仍带有 Legacy 耦合。** Bootstrap 通过旧 `mysqlredis` Adapter 解析共享 DB/Redis 配置，并加载旧 Market Data Config 后构造 Legacy Analysis Runner；配置仍以当前兼容链为主，尚未证明各目标模块 Config/Secret Schema 均通过公开装配入口集中注入。该问题随模块正式接入统一核验，不另行要求 Server 接管领域配置定义。
6. **Diagnostics 仅覆盖当前已装配资源。** Root Storage/Schema Check 使用 Platform Diagnostics Runner；尚未注册各目标模块的真实 Check，Readiness 不能被解释为十模块全部依赖健康。必须在模块接入时由 Owner 提供真实 Check，再由 Server 声明 Required/Optional。

### 差异登记（审计工作项，非新增 CTR）

| 编号 | 固定基线事实 | 状态 | 冻结影响 / 后续统一处理 |
|---|---|---|---|
| SRV-PLAT-A | Root Bootstrap 仍以 Legacy Analysis 为主要服务；未见十模块公开 Facade/Adapter/Worker 的完整装配；Migration Registry 只注册 News/Research/Notification | pending_evidence | Server Owner 按模块公开构造入口逐项装配，不直接访问模块私有 Repository/Domain；Migration/Worker/Diagnostics 仅注册已交付能力。以模块装配清单、DI 图、启动/关闭证据和根集成测试证明，不创建空 Registry 或占位 Adapter。Data/Researchflow 等未就绪项保持明确阻断 |
| SRV-PLAT-B | Market Data 独立入口自行创建 SQL 连接池和自定义 Redis RESP 客户端；未统一复用 Root Server Platform Storage 资源 | pending_evidence | 沿用 Data 迁移和 Server 唯一宿主决议；迁移到 Root 后由 Server 创建共享 Platform SQL/Redis 资源并注入 Data Store/Cache，模块 Store 不拥有借用资源。迁移前保留独立进程兼容边界，不在两个进程间伪造共享连接池 |
| SRV-PLAT-C | Root 已创建 Platform HTTP Client 并提供 BoundDoer 绑定方法，但固定基线没有实际业务上游绑定和模块生产注入链路 | pending_evidence | Server 按每个真实上游配置精确 DestinationPolicy/Dependency，创建固定 BoundDoer 并注入模块公开构造入口；消费者停止后关闭 Client。补 SSRF/重定向/超时/响应限制/流关闭和真实 Provider E2E 证据；不得将 Client 可构造或单测通过标记为生产接入完成 |
| SRV-PLAT-D | Root Diagnostics 已使用 Platform Runner，但只注册当前 MariaDB/Redis/Schema 检查；未接入各模块真实诊断 Check | pending_evidence | 模块 Owner 提供具有稳定 ID、Required/Optional、reason_code、safe_message、evidence_ref 的真实 Check；Server 负责注册与聚合，并明确 Readiness 门禁。不能将当前 Root Readiness 宣称为全模块健康 |
| SRV-PLAT-E | Root 当前配置和构造仍依赖 Legacy Analysis/Data Adapter 配置入口；目标模块 Config/Secret 集中装配证据不足 | pending_evidence | Server 只集中加载进程级配置和模块公开配置结构，按模块注入最小必要 Secret；不复制领域配置 Schema，不允许业务模块任意散读环境变量。随 SRV-PLAT-A 的正式模块接入核验 |

### 本批结论

- **Server/Platform/领域模块职责边界：freeze-ready。** Server 是组合根和唯一目标 HTTP 宿主；Platform 提供横向技术原语；领域模块拥有业务语义、Repository、Migration 内容、公开 Port 和 Worker。无需重做公共 Platform 契约或引入新的万能模块接口。
- **Server 生命周期和共享资源管理基础：accepted。** Root Resources、Lifecycle、Migration Registry、Platform Router/Fault/Observability/Diagnostics 的基础接入及单测存在；启动/关闭顺序与资源所有权方向正确。
- **十模块生产组合装配：pending_evidence。** 当前实际仍是 Legacy Analysis 主链，Migration/Worker/Diagnostics/HTTP Facade 仅有部分模块接入，不能以设计文档或单模块测试替代根装配证据。
- **Platform HTTP Client 的生产消费、Data 统一 Storage、模块 Diagnostics/Config 注入：pending_evidence。** 不需要新增 Platform 技术原语；按现有契约由 Server 和责任模块完成接线。
- **Web Session A2 接入**沿用 WEB-SRV-B，不重复登记。沿用既有 PLAT-CTR-005、PLAT-015～019、GOV-CTR-003/004/005 与各模块 Migration/Adapter 契约，不新增 CTR。

本批只更新 PLAT-002 审计文档；未修改 Server/Platform/业务模块代码、未执行 CI。后续只在责任模块完成正式接入后核验 SRV-PLAT-A～E 的证据，不重复审查已通过的 Platform 包级原语和 Lifecycle 单测。

## 35. PLAT-002 全量差异矩阵收敛与最终冻结评估（2026-11-10）

### 35.1 评估基线与统计口径

- 固定代码基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- 维护分支：`feat/design_doc`。
- 本轮核对权威登记簿、唯一差异矩阵、十模块总进度，以及已完成的跨模块边界审查（第25～34节）。
- 正式 CTR 统计以 `doc/contract/PLAT-002-项目级契约冻结登记簿.md` 第3节为准；审计工作项统计以本矩阵现有表格为准。两者不可相加，也不得把审计工作项当作新 CTR。
- 正式登记簿仍有 43 个 CTR；本轮复核时 0 项达到 L2 `frozen`。单项当前状态以登记簿第3节为准，不因项目级决议批准而自动变化。
- 69 行、56 个唯一编号工作项是本轮决策前的历史审计规模快照，不是当前待决数量。DEC-PLAT-002-018～026 已覆盖原 22 个含 `pending_decision` 的工作项；当前非条件性项目级待决队列为 0。相关工作项已在本矩阵当前状态表中转为 `pending_evidence`；DATA-FLOW-B、LLM-FLOW-B 继续按条件触发规则保持 `deferred`。历史审查章节保留当时状态，不能当作当前状态读取。
- W2-C 的 CTR-011～037 是聚合范围，CTR-010 是字段级聚合差异；历史别名 CTR-003/005 不重复计数；CTR-041 已 withdrawn。以上去重规则继续有效。

### 35.2 双层冻结结论（必须区分）

**L1 Design Baseline：已冻结。**
- 项目级 L1 设计基线 1.0.0 已有明确决议，锚点为 `6d2966e35aab06ed30791701e4ca6fc036a07702`。
- 冻结范围是已经明确的模块职责、依赖方向、公开边界、身份/时间/数值/授权/错误/状态/幂等/版本兼容原则和首批跨模块调用范围。
- 该冻结不覆盖明确保留的条件性未决子范围，也不代表代码、Schema、Golden、消费者测试或真实环境已经验收。

**L2 Contract Evidence：尚未有正式 CTR 冻结。**
- 43 个正式 CTR 均未达到 L2 `frozen`。
- Schema/Golden 文件存在、单模块测试通过、局部 Adapter 存在或文档写明 accepted，均不能代替生产方和全部直接消费者的契约证据。
- 因此当前只能宣布“L1 设计基线冻结；L2 契约验收未完成”，不能宣布“PLAT-002 全部契约冻结完成”。

### 35.3 可以维持 L1 冻结的稳定范围

以下职责和语义边界已有审查证据，可继续作为 L1 冻结基线，不要求为补实现证据而重新设计：

1. **模块职责与依赖方向**：Platform 只提供技术原语；Server 是唯一组合根；Data/News/Quant/Research/Researchflow/LLM/Notification 各自拥有领域事实与语义；WebUI 只消费公开 HTTP 契约。
2. **数据与研究边界**：Data 拥有行情事实、Snapshot Manifest 和 Data AuthorizationRef；Research 拥有 Evidence/Context/Claim 等研究语义；Researchflow 负责跨模块编排，不访问其他模块 Repository/SQL。
3. **计算与编排边界**：Quant 拥有指标、因子、风险决策及结果资源；Researchflow 负责 Stage 组合，不重算 Quant 指标、不覆盖 RiskDecision。
4. **模型与工具边界**：LLM 负责模型协议适配；Researchflow 负责工具策略、授权和执行编排；LLM 不执行工具。
5. **事件边界**：业务事实生产模块产生版本化事件并在本地事务写 Outbox；Notification 负责接收、Inbox 幂等和投递编排；Server 负责 Relay 装配，不推导业务事件语义。
6. **HTTP 与基础设施边界**：WebUI 不持有 Provider/API Key；Server 负责入口认证、授权、DTO 映射和 HTTP 错误投影；Platform 提供 Fault、Security、Request ID、Diagnostics、HTTP Client、Storage 等横向原语。
7. **生命周期边界**：Server 管理共享资源和组件生命周期；模块拥有自身 Repository、Migration 内容、Worker/Facade Port；Platform 提供通用连接与 Migration Runner。

以上只冻结稳定职责/语义，不把未决字段映射、未启用能力、未完成 Adapter 或未验收部署纳入冻结范围。

### 35.4 当前待决事项与条件性延期

- **当前非条件性项目级 `pending_decision`：0 项。** 原待决事项已由 DEC-PLAT-002-017、018～025 及 GOV-CTR-006 覆盖；正式语义以决策记录为准。
- **条件性延期：**
  - `DATA-FLOW-B`：仅在首个真实 Data 消费 Stage 进入实现/接入前，决定 Stage 级引用子集和最小权限策略；不阻断纯编排 Domain。
  - `LLM-FLOW-B`：仅在项目决定启用完整 Tool Call continuation 时，补充 ToolResult 回传、上下文和版本化契约；不阻断首期单轮文本调用。
- 其余原 `pending_decision` 工作项已转为 `pending_evidence`：决策方向已定，剩余是责任模块实现、Schema/Golden、Adapter、授权复核和双方测试。不得再把这些事项要求项目负责人重复裁决。
- 如果后续实现暴露出决议未覆盖的新业务取舍，必须先登记具体差异和影响，再单独提交裁决；不得仅因实现困难而重新打开已批准决议。

### 35.5 L2 pending_evidence 的主要阻断簇

以下是当前最主要的实现/证据门禁，统一由责任模块处理，PLAT-002 只做增量验收：

1. **WebUI 维度2 DTO/OpenAPI**：CTR-006/007/008/009/010 及 CTR-011～049 中既有 P2 差异，按对应端点和字段局部冻结；W2-C 已完成归类，不重做初审。P2 不自动阻断无关端点或其他模块。
2. **Data 生产与消费证据**：v2 Decimal Schema/原始精度链路、Snapshot/Manifest/AuthorizationRef、交易日历与质量映射、Data→Quant/Research/Researchflow Adapter 和联合测试。
3. **News 生产与消费证据**：available_at 来源证据、授权快照映射、查询 filters 实际支持范围、Snapshot/Cursor 生命周期、Research/Researchflow Consumer Adapter 和 PIT/授权联合测试。NEWS-RSC-E/G 的实现/文档冲突先由 News Owner 修正，不能将声明能力当成真实能力。
4. **Quant→Researchflow**：强类型 Payload 子 Schema/Validator、Quant Application Facade、结果当前授权读取、Manifest/Run 上下文一致性、真实 Adapter 和联合测试。
5. **Research→Researchflow**：真实 Facade Adapter、Claim 引用闭环、Owner 授权复核、Context/写操作幂等和跨模块测试。
6. **Researchflow→LLM**：Tool Executor 持久化幂等/fencing、真实 Provider/Model/Gateway 能力、Stage Adapter、Server Secret/BoundDoer 注入、SSE/取消/错误映射联合测试。
7. **事件生产与 Notification**：Researchflow Outbox 按既定 INT-CTR-001 对齐；Quant 风险事实 Outbox 缺失；Research Relay、Notification Fanout Writer/Worker、Server Relay 装配及崩溃/重复投递测试。
8. **WebUI→Server 与 Server/Platform**：Data API Key 退出浏览器并迁移至 Server；目标模块 Facade/Worker/Migration/Diagnostics/Config 装配；Platform BoundDoer 生产注入；OpenAPI 驱动跨端测试和 Session E2E。
9. **Platform 基础契约证据**：Fault/Request Context/Diagnostics/Storage/HTTP Client/Config/Security 的包级原语已部分实现，但公共 Schema、跨模块映射、真实消费者接入和必要 Golden/Contract Test 仍需按 PLAT-CTR-001～007 补齐。

### 35.6 责任归属与不得越权的处理方式

- PLAT-002：维护唯一登记、L1 冻结范围、决策追踪、证据索引和冻结评估；不代替责任模块修改 DTO/Schema/Golden/测试/业务实现。
- Platform：维护 Fault、Security、Request Context、Diagnostics、HTTP Client、Storage、Config 等技术原语及自身机器契约。
- Server：负责 Root Composition、DI、Session/路由安全、资源生命周期、Migration/Worker 注册、HTTP Adapter/错误映射和模块 Diagnostics 汇总。
- Data/News/Quant/Research/Researchflow/LLM/Notification：分别负责本领域公开 DTO/Port、Schema/Golden、生产行为、Repository/Outbox/Worker 和领域授权；消费者模块负责自己的映射与消费测试。
- WebUI：负责按权威 OpenAPI 消费请求/响应、错误和认证流程，不反向定义领域模型。
- 责任模块修复后，PLAT-002 只按新提交 SHA、对应 Schema/Golden/Contract Test 和消费者映射增量核验；不重新跑已完成的初审。

### 35.7 当前冻结评估与退出条件

| 评估层级 | 当前结论 | 退出条件 |
|---|---|---|
| L1 设计基线 | `frozen`（锚点 6d2966e...） | 仅通过正式契约变更流程修改 |
| 单项 L2 契约 | 当前 0/43 `frozen` | 逐项满足登记簿第5节适用门禁 |
| 跨模块 L2 关系 | 部分边界职责/语义已 freeze-ready；调用 Adapter/授权/联合测试未齐 | 生产方与消费者双向证据、授权链、Schema/Golden/Contract Test |
| Server A2 装配 | `pending_evidence` | 十模块真实 Facade/Worker/Migration/资源/Session/Diagnostics 接线及 Root 集成测试 |
| L3 部署/真实 Provider/模型/通知 | `deferred / not established`，不得混入 L2 | 按部署与真实依赖单独验收，不作为无关 L2 契约的前置条件 |

PLAT-002 的项目级决策收敛已完成；**PLAT-002 项目整体不宣告完成**。当前主线是责任模块实现/补证、按新 SHA 增量验收，以及后续第二/第三轮 Review。全量 import 图/完整消费者追踪、十模块六维覆盖及冻结门禁 Review 尚未完成，不能将其标记为 complete。

### 35.8 后续唯一执行主线

1. 项目级非条件性决策队列已清零；DEC-PLAT-002-018～026 是当前执行依据，不再重复提交 B1～B8 裁决。
2. 各责任模块按第35.5节和正式决议实施/补证，按真实依赖并行；先处理新 Researchflow API/WebUI 切换与 Server 请求规则，再推进 Data/News/Quant/Research/LLM 的对应 Adapter 和事件链路。
3. 每获得一组责任模块新提交，PLAT-002 只按新 SHA 增量核验相关工作项；满足 L2 门禁的 CTR 单项冻结，不满足的保持 `pending_evidence`。
4. 十模块六维证据和全仓依赖图补齐后，执行第二轮正确性 Review；责任模块补证齐备后执行第三轮冻结门禁 Review。
5. `DATA-FLOW-B`、`LLM-FLOW-B` 仅在各自触发条件出现时重新进入决策；没有新提交/决议/证据时，不重复读取相同代码或增加审计章节。

## 36. 项目级待决事项分批提案与状态去重（2026-11-10）

本节保留项目决策批次的历史提案、映射与最终裁决记录。36.3～36.5 是裁决前快照；当前有效结果以 36.6 和正式决策记录 DEC-PLAT-002-018～026 为准，不得将历史提案重新作为待批准事项。不得创建新的 CTR。

### 36.1 统计口径纠正

第35.1节此前按带显式大写编号的工作项统计，遗漏了以 Data→Quant、Data→Research 等边界名称登记的表格行。本轮重新按矩阵全部状态表格行统计：

- 截至第35节（不含第36节提案映射表），有状态表格行：69 行。
- 带唯一显式审计编号的工作项：56 项，编号无重复。
- 其余 13 行为边界/契约聚合表格行，其中 Data→Quant、Data→Research 各有两个不同问题，不能因显示名称相同而合并。
- 裁决前历史快照：截至第35节，含 pending_decision 的行项目共22项：21项纯待决（NEWS-FLOW-D 随 A/B 合并裁决，不独立成项），1项同时包含待决与证据缺口。该数字仅描述决策前状态，不代表当前队列。
- 该统计包含历史切片表格和当前差异表格，不等于 69 个独立契约；正式 CTR 仍以登记簿第3节为准。

### 36.2 已有决议覆盖项：移出项目负责人待决队列

| 工作项 | 已有权威依据 | 当前处理 |
|---|---|---|
| WEB-SRV-B | DEC-PLAT-002-017 已接受 Owner 密码登录、HttpOnly Web Session Cookie、Bearer/Service 身份分离、CSRF/Origin、路由显式认证方式和资源 Owner 授权边界 | 不再要求项目负责人重复决策；改为 pending_evidence，由 Server/WebUI 补 A2 接入与端到端证据 |
| WEB-SRV-F | doc/platform/08 第5节规定响应返回最终 X-Request-ID；PLAT-CTR-002/Server Root 已按响应 Header 传播 | 不再作为语义待决；Header 为权威通道，旧 Market Data Body request_id 仅兼容读取；剩余为旧/新 Transport 一致性补证，改为 pending_evidence |
| NEWS-RSC-C | GOV-CTR-006 已定义公共 AuthorizationRef 审计快照结构和资源授权 Owner 原则 | 授权语义不重开决策；News 需补公开读取映射、PolicySnapshot 传递和当前授权复核证据，改为 pending_evidence |
| Data→Research available_at | 现有原则已明确无来源证据时必须为 null，不得从 trade_date/provider_time/fetched_at 推断 | 这是 Data 字段/来源事实核验，不是项目级二选一；改为 pending_evidence，确认 DATA-CTR-005/007 实际字段及来源 |
| DATA-FLOW-B | 只有真实 Data 消费 Stage 启用时才需要决定 Stage 级最小资源范围 | 标记 deferred，触发条件为首个 Data Stage 进入实现/接入；不阻断纯编排 Domain |
| LLM-FLOW-B | LLM 首批明确单轮文本，多轮/previous_response_id 等已 deferred；ToolResult continuation 尚无首期产品范围承诺 | 标记 deferred，触发条件为项目决定启用完整模型工具往返；首期不把已执行工具结果伪装为模型已消费 |

### 36.3 项目负责人决策批次

| 批次 | 决策主题 | 对应现有工作项/契约 | 建议裁决方向 | 不包含的工作 |
|---|---|---|---|---|
| DEC-B1 | 旧 Analysis API 直接退役 | D-SRV-API-01、D-SRV-EVT-01、WEB-SRV-C；CTR-046/007 | 当前无正在服役的旧 Analysis 服务；WebUI 直接切换 Researchflow 新 API，不保留旧 API 兼容期。新 API 独立使用版本化 DTO/事件语义；OpenAPI 覆盖实际路由、认证、错误和 Run/Task/Event Schema。新 API 接入验收后删除旧路由、Handler 和专属适配代码 | 不自动删除仍被新模块使用的通用能力、历史数据或 Migration |
| DEC-B2 | 请求校验与 Run/Task ID（历史提案） | D-SRV-REQ-01；CTR-047 | 原提案曾针对旧 Analysis API 定义 TaskType、symbols 和旧 Run/Task ID 规则；已由 DEC-018 退役决议限定为历史语义，不再要求落实 | Researchflow CreateRunCommand 使用独立的 caller-scoped idempotency_key/fingerprint，不继承旧 run_id/TaskType 规则 |
| DEC-B3 | Data Snapshot 事实映射 | Data→Quant 两项、Data→Research Manifest 映射；DATA-CTR-005/007/008、QNT-CTR-001/002、RSC-CTR-001 | Data 提供行情/交易日历/完整性证据，证据缺失显式 unknown；Quant 按具体指标策略校验连续性；stale 不得升级 valid，容忍时显式 degraded；Research 引用不可变 Snapshot 并保留来源/授权链 | 不改变既有 Decimal v2 决策；available_at 继续单独补证 |
| DEC-B4 | News Evidence 来源/质量/关系语义 | NEWS-RSC-B/D/F；NEWS-CTR-001/002/003、RSC-CTR-001 | 区分 News Item/Version、Observation、Source、Provider、分页 Snapshot；质量和 stale 策略显式；RelationRefs 区分 not_provided 与 confirmed_none，不以空数组表达无关系结论 | 不重开 GOV-CTR-006 授权语义 |
| DEC-B5 | News 输入进入 Researchflow Run | NEWS-FLOW-A/B/C/D；NEWS-CTR-001/002、FLOW-CTR-003/004、GOV-CTR-002/006 | Run 创建前两阶段准备输入：固定身份/查询条件查询 News，仅物化该 Run 实际需要的持久化不可变 Snapshot，再创建 Run 固定 refs；Cursor 仅由 News Adapter 持有，重试复用原 Snapshot | 不建设无必要的通用快照平台；无 News 输入不创建空 Snapshot |
| DEC-B6 | Quant ResultRef 与 Stage 输出投影 | QNT-FLOW-A/E；QNT-CTR-001～005、FLOW-CTR-004 | Quant 拥有完整 Result Envelope/Payload/Manifest/Schema/授权；Researchflow 通过 Facade 获取必要元数据并复核授权，Stage.output_ref 仅保存资源引用和编排关联信息，不复制完整 Payload | 仅在下游用例确实需要时通过 Quant Facade 读取结果内容 |
| DEC-B7 | Research Stage 引用读取、幂等和 Context 访问 | RSC-FLOW-A/B/C/E；RSC-CTR-001/002、FLOW-CTR-002/003/004、GOV-CTR-003/005/006 | 固定 Ref 精确读取、默认 fail-closed；Context 元数据受控；仅对明确允许 Stage 调用且产生业务副作用的写操作要求稳定幂等键、规范化指纹、重放对账，幂等键跨 Attempt 稳定 | 不给所有 Research 命令无差别增加幂等字段 |
| DEC-B8 | Researchflow→LLM 配置固定与重试分类 | LLM-FLOW-A/C；LLM-CTR-001/003、FLOW-CTR-002/003/004、GOV-CTR-003/004/005 | Run 固定版本化 ResolvedTemplate、Prompt/Schema fingerprint 和模型策略；Provider/Model 由 Server 受控注入；Attempt 独立 request_id；stable error 显式映射 Retry ReasonCode；ambiguous outcome 不盲目重放 | 不包含已 deferred 的完整 Tool Call continuation；retryable=true 不单独授权重试 |

### 36.4 批次优先级与依赖

1. DEC-B1/B2：旧 Analysis API 兼容和请求规则，直接影响 Server 维度2的 CTR-046/047/007 关闭；已有部分事实，优先一次确认。
2. DEC-B3/B4：Data 与 News 事实/证据映射，决定 Research/Quant 输入能否正确解释；应在对应 Adapter 开发前确认。
3. DEC-B5：依赖 News Evidence 资源模型，随后再确定 Researchflow Run 输入准备方式。
4. DEC-B6/B7：Quant/Research 的 Application Facade 与 Researchflow Provider Adapter 实现前确认。
5. DEC-B8：LLM Stage Adapter/RetryPolicy 实现前确认。

本批不要求项目负责人逐个批准 8 份独立文档；可在一次项目决策评审中逐项接受/修改/拒绝/延期。每项决策必须有明确结果；不同意推荐方案时记录替代方案和影响，不允许静默跳过。

### 36.5 本轮状态调整边界

- 将已有决议覆盖的 WEB-SRV-B、WEB-SRV-F、NEWS-RSC-C 移出语义待决，转为实现/跨端证据待补。
- 将 Data→Research available_at 的事实核验转为 pending_evidence。
- 将 DATA-FLOW-B、LLM-FLOW-B 作为条件性扩展转为 deferred。
- NEWS-FLOW-D 仅随 NEWS-FLOW-A/B 一并裁决，不作为独立决策批次。
- 本条为裁决前的历史状态说明，已被第36.6节最终裁决覆盖；DEC-B1～B8 不再是待批准提案。


### 36.6 项目负责人最终裁决（2026-11-10）

项目负责人已确认 B1～B8 的 Review 修订方向，正式决议见项目级决策记录 DEC-PLAT-002-018～025；汇总状态见 DEC-PLAT-002-026。原 DEC-B1～B8 仅保留为审计映射，不再是待批准提案。

| 提案映射 | 正式决议 | 最终裁决要点 | 后续责任边界 |
|---|---|---|---|
| B1 | DEC-PLAT-002-018 | 当前无服役旧 Analysis 服务；WebUI 直接切换新 Researchflow API；不保留兼容期；新 API 验收后删除旧路由和专属适配代码 | Server、WebUI、Researchflow |
| B2 | DEC-PLAT-002-019（旧 API 规则，已由 DEC-018 限定退役适用范围） | 不作为新 Researchflow API 的请求约束；新 API 按 FLOW-CTR-001 独立契约 | Server、Researchflow |
| B3 | DEC-PLAT-002-020 | Data 提供行情/日历/完整性事实；Quant 决定指标连续性策略；stale 不得升级 valid；Research 引用不可变 Snapshot | Data、Quant、Research |
| B4 | DEC-PLAT-002-021 | News 来源/版本/Observation 分离；质量显式；关系未提供不等于无关系 | News、Research |
| B5 | DEC-PLAT-002-022 | Run 创建前两阶段准备输入；只物化 Run 实际需要内容；Cursor 不进入 Run refs；重试复用快照 | News、Researchflow、Server |
| B6 | DEC-PLAT-002-023 | Quant 拥有结果 Envelope；Researchflow 仅保存 ResultRef/关联信息，不复制完整 Payload | Quant、Researchflow、Server |
| B7 | DEC-PLAT-002-024 | 固定 Ref 精确读取、默认 fail-closed；仅 Stage 可调用且有业务副作用的写操作要求幂等 | Research、Researchflow |
| B8 | DEC-PLAT-002-025 | Run 固定 ResolvedTemplate/模型策略；Attempt 独立 request_id；错误分类控制重试，ambiguous outcome 不盲目重放 | LLM、Researchflow、Server |

本轮只完成项目级决议登记，不修改责任模块代码，不运行 CI，不将任何 CTR 自动标记为 frozen。下一阶段按正式决议派发责任模块工作，并以新代码 SHA 做增量验收；不重新执行已经完成的全量初审。


## 37. CTR-006 责任分支增量核验（2026-11-10）

- 核验目标：只确认既有 CTR-006 工单是否已有责任分支修复证据；不重新审计 Task DTO，不改变固定代码基线。
- Server 责任分支 `feat/research-analysis-flow-native-go` 当前 HEAD 为 `a7efeff9e8802e69cc58a0e7d17f3f37a8eee873`（提交说明为更新 readiness 验收文档）。与固定审计基线 `b6dfc85ba214fe16268c8cba780641a8bfe15086` 比较，Server 分支是该基线的祖先，基线领先 1677 个提交；不能把旧分支 HEAD 当作当前基线后的修复证据。
- 固定基线中的权威 OpenAPI `doc/api/contracts/分析任务.openapi.yaml`（Blob `aa8f153f25c46649dcdf187d5fef8f376113577b`）仍为 `started_at`、`finished_at` 声明 `nullable: true`，与既有 Go 指针字段 `omitempty` 的“缺失而非显式 null”行为不一致；既有 queued/running/terminal Golden 和 Handler Contract Test 关闭条件尚未得到新提交证据。
- 固定基线 `b6dfc85...` 的 GitHub Check Runs 为 0，Commit Status 为空；本次没有可引用的 CI 成功证据。未执行测试，也未修改责任模块代码。
- 结论：CTR-006 仍为 `pending_evidence`（既有工单状态不变）；本轮没有可接受的修复 SHA，不得关闭或冻结。下一步只需等待 Server 责任分支产生针对 CTR-006 的修复提交，再按既定 28.4 验收条件增量核验；不重复字段语义讨论。


## 38. CTR-043 Dragon Tiger 契约语义与实现符合性边界更正（2026-11-10）

- **更正依据：** 回到既有 CTR-043 原始登记及总进度第31节、第32节、第33节、第34节的历史核验记录。CTR-043 原始问题是 Hithink 龙虎榜分页计数与公共契约不一致，不是 Provider 金额单位或 WebUI 金额展示问题。上一版第38节将审查对象错误改写为金额展示问题，现以本节整体取代。
- **公共契约语义：** `source_returned` 表示本次请求页从 Provider 收到的原始记录数；`returned` 表示规范化后返回的记录数；`excluded_non_a_share_count` 表示明确排除的非 A 股记录数。三者须满足 `source_returned = returned + excluded_non_a_share_count`。分页请求身份由 trade_date/page/page_size 确定。该语义已有 Data DTO、Service 校验和 OpenAPI 定义，不需要通过 Provider 内部实现来重新决定。
- **已知实现偏差（仅作为模块内部符合性记录）：** 固定审计基线 `b6dfc85ba214fe16268c8cba780641a8bfe15086` 的 Hithink Dragon Tiger Provider 一次读取完整 `stock_items` 后本地分页，却将整份列表数量写入 `source_returned`；当总量大于 page_size 时，与 Service 对当前页原始记录数的校验冲突。重复 symbol 被静默去重但未计入排除数，也可能破坏上述计数恒等式。这是 Data 内部 Adapter 对已定义公共契约的实现不符合，不是公共契约语义缺失。
- **契约冻结判断：** CTR-043 的公共契约语义明确，生产方与消费方无需理解 Hithink 的内部分页策略、原始字段或 Provider 测试结构。就 PLAT-002 的契约冻结目标而言，本项应标记为 `freeze-ready`（契约语义可冻结）；Hithink Adapter 的修复、Provider 专项测试和运行时验证属于 Data 模块内部实现质量工作，不作为本项契约冻结门禁。
- **边界补充：** Data 对外仍须按公共契约返回正确的分页计数和守恒关系。若真实服务违反已冻结契约，应作为 Data 模块实现缺陷跟踪和修复；不得因此反复修改或扩大公共契约，也不得要求 WebUI 了解 Provider 的分页方式。
- **金额字段问题不属于原始 CTR-043，但已另行形成项目级契约决议：** DEC-PLAT-002-027 已明确 Provider 原始金额及 `provider_amount_unit` 不属于跨模块公开 DTO，Data 对外只提供规范化 CNY 金额字段。该公开字段边界需要在 Data OpenAPI/DTO 契约中落实，详见本矩阵第39节；不要求 WebUI 消费 Provider 原始金额，也不将 Provider 内部测试列为 PLAT-002 跨模块契约门禁。
- **执行边界：** 本轮只修正 PLAT-002 审计文档和状态分类，不修改 Data/WebUI 责任模块代码，不执行测试或 CI。原实现偏差保留为 Data 模块内部待修事项，不阻断契约冻结。


## 39. DEC-PLAT-002-027：Dragon Tiger 公开 DTO 的 Provider 字段边界落实

- 项目级决议已明确：Provider 原始金额和 Provider 专属单位字段属于 Data 内部适配细节；跨模块公开 DTO 只输出规范化人民币金额。Data 负责 Provider 原始值识别、单位验证、换算与质量判断。
- 固定基线 b6dfc85ba214fe16268c8cba780641a8bfe15086 的 internal/marketdata/model/dragon_tiger.go 通过 JSON tag 暴露 provider_buy_value、provider_sell_value、provider_deal_value、provider_net_value、provider_net_rate、provider_org_net_value、provider_hot_money_net_value、provider_amount_unit；internal/marketdata/httpapi/openapi.yaml 的 DragonTigerItem Schema 也公开声明这些字段。它们不是只存在于 Provider 内部的私有字段。
- 当前公开契约边界与 DEC-PLAT-002-027 不一致。Data 应调整公开 DTO/OpenAPI/Schema/Golden：移除上述 Provider 原始字段；保留标准化 *_cny 字段和明确的缺失语义。无法可靠换算的金额不得填入 CNY 字段，应省略/为空并通过 quality_status/quality_reasons 表达 degraded 或 unknown。
- WebUI 只消费公开 CNY 字段，不识别同花顺、不处理 provider_defined、不负责换算。Data 内部可继续保留任意 Provider 原始字段、模型或诊断信息，但不得通过跨模块公开 JSON DTO 泄漏。
- 状态：项目级语义决议已 accepted；Data 公开 DTO/OpenAPI 落实证据尚缺，故该字段边界在 L2 仍为 pending_evidence。这是既有 DEC-027 的落实项，不新建 CTR，不与原始 CTR-043 的分页计数问题混为一项。
- 本分支只登记契约边界与证据缺口，不修改 Data 代码、OpenAPI、Golden 或测试；后续由 Data 责任分支落实后，以新 SHA 增量核验。


## 40. CTR-047 退役归类：旧 Analysis CreateTask 请求不再作为活动冻结契约（2026-11-11）

- **唯一依据：** DEC-PLAT-002-018 已由项目负责人接受，明确旧 Analysis API、旧 Task/Run HTTP 路由及旧 StageEvent HTTP 契约不保留兼容期；WebUI 直接切换 Researchflow 新 API，新 API 验收后删除旧路由及专属适配代码。
- **CTR-047 原始对象：** 旧 Analysis 的 CreateTask/AnalysisRequest HTTP 请求 Schema 与领域 RunRequest 的字段、默认值、TaskType、as_of/as_of_mode、parameters、同步/异步 run_id 行为差异。它不是 Researchflow CreateRunCommand 的公共契约。
- **状态修正：** CTR-047 标记为 `withdrawn_by_scope_decision`（因公开 API 退役而撤销活动冻结任务），不再保留 P2 活动差异，不要求补齐旧 CreateTask OpenAPI、Handler 校验或旧请求 Contract Test。历史字段核验与同步 Runner run_id 证据仅用于解释旧实现，不作为新 API 的约束。
- **DEC-PLAT-002-019 适用范围：** 该决议原本约束旧 Analysis CreateTask/同步 Run HTTP 请求。其历史裁决不再形成旧 API 活动门禁；TaskType 四值、symbols 上限100、异步 Task ID 服务端生成、同步显式 run_id 幂等/409 规则均不得直接套用到 Researchflow 新 API。
- **新 API 的独立契约：** Researchflow 使用独立的 CreateRunCommand 和版本化 DTO；调用主体从可信认证 Context 获取，CreateRun 幂等键作用域为 `(authenticated_caller_ref, CreateRun)`，使用 `idempotency_key` 与规范化请求 fingerprint；相同键和指纹返回既有 Run，不同指纹冲突。Run 引用由 Researchflow 生成。参数约束由具体版本化 Template/Parameter Schema 定义，不沿用旧 Analysis 的 `symbols` 通用上限。
- **责任边界：** 旧 CreateTask API 不再由 Server/WebUI/Researchflow 继续维护或冻结。Server/WebUI/Researchflow 只需按 DEC-018 完成新 API 接入验收、OpenAPI/DTO/认证/错误/事件对齐；Researchflow 新 API 的证据仍归 FLOW-CTR-001～004 等既有正式契约，不新增 CTR。
- **固定基线的新 API 现状（跨模块缺口）：** `internal/researchflow/application/create_run.go` 已有 CreateRun Application，`internal/researchflow/publicdto/dto.go` 已有 Run/Stage 公开 DTO；但 `internal/server/bootstrap/router.go` 仅注册旧 `/api/v1/analysis/**` 路由，`bootstrap.go` 仍装配旧 Runner/TaskQueue/Platform HTTP API，没有 Researchflow HTTP Handler/路由装配。WebUI `web/src/api.js` 仍调用 `/analysis-api/api/v1/analysis/tasks`、Run 与 Task Events；`doc/web/API_CONTRACT.md` 仍记录旧路径。由此确认 DEC-018 在固定基线尚未完成接入验收。
- **冻结边界：** 该缺口不恢复 CTR-047，也不要求补齐旧 API。下一批只核对新 API 的 Server HTTP 映射是否已有权威路径、请求/响应 DTO、认证/错误/事件 Schema 与 WebUI 消费契约；模块级 CreateRunCommand 继续由 FLOW-CTR-001 定义，HTTP 宿主与映射归 SRV-CTR-001。不得把 Researchflow Application DTO 直接等同于 HTTP DTO，也不得把旧 Analysis Request 字段迁移过去。
- **不代表：** 本状态不表示旧 API 代码已删除，也不表示新 API 已接入验收。旧代码退役属于 DEC-018 的迁移执行工作；新 API 的契约证据按其正式契约单独验收。
