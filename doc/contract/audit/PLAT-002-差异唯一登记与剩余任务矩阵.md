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
| W2-A Task/TaskEvent DTO 与分页 | CTR-006、007、008 | 已有初审记录，尚未完成统一关闭条件 Review | 一次性核对 StartedAt/FinishedAt 缺失/null、StageEvent Schema、分页边界与事件 ID 持久化语义；给出契约裁决项、实现项、测试项及冻结影响 |
| W2-B DailyBar 查询窗口 | CTR-009 | 已有初审记录，尚未完成冻结影响收敛 | 明确 lookback_days 缺省值由谁拥有、WebUI 显式传参的边界及 OpenAPI 默认语义 |
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
