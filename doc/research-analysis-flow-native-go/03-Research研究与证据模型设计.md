# Research 研究与证据模型设计

## 1. 目标与边界

Research 负责把跨领域数据转化为可追溯的研究事实、证据、事件、假设和情景。它不负责新闻采集、不拥有行情事实、不重复计算 Quant 指标，也不把模型生成内容当成原始事实。

核心原则：
- 原始事实、标准化事实、推断、假设、未知项必须区分。
- 每条事实/证据都必须能追溯来源、时间、版本和质量。
- 证据既包括支持证据，也包括反向证据。
- 新闻摘要、LLM 输出不能替代原始来源。
- 事件影响方向、持续时间和价格验证是研究结论，不是新闻字段的简单映射。

## 2. Context

Context 是一次研究任务在指定 as_of 和范围下允许使用的信息集合，不是把所有数据拼成一个大 JSON。

Context 由可引用的分区组成：
- MarketContext：市场环境、指数、宽度、量能和风险偏好；
- IndustryContext：行业/主题强度、成员版本和轮动；
- StockContext：证券状态、行情、趋势、量价、财务和估值；
- EventContext：相关事件、事件版本、来源和生命周期；
- EvidenceContext：支持/反向证据、可信度、时间和限制；
- StrategyContext：适用策略、历史样本和回测统计；
- RiskContext：风险门禁、预算和拒绝原因。

Context 元信息：
- context_id/version、research_run_id、as_of；
- input_snapshot_refs、source_refs、quality_summary；
- included_sections、omitted_sections 和 omission_reasons；
- token_budget（仅模型上下文使用）、truncation_policy；
- builder_version、created_at。

构建规则：
1. 按任务模板选择分区，不默认全量加载。
2. 按时间、相关性、可信度和证据完整性排序；保留来源引用。
3. 重要反向证据和质量限制不得因 token 裁剪被静默删除。
4. 超预算时优先压缩重复描述，不得删除来源、时间、事实/推断标记和风险门禁。
5. 不同 as_of、不同快照或不同权限范围不得混用同一 Context 版本。

## 3. Evidence

Evidence 表达支持或反驳某项 Claim 的可追溯证据。建议字段：
- evidence_id、evidence_type、claim_ref；
- source_type、source_ref、source_version、source_trust；
- published_at、available_at、observed_at、as_of；
- security_refs、industry_refs、event_ref；
- stance：supports / contradicts / neutral；
- epistemic_type：fact / inference / hypothesis / unknown；
- quality、confidence_basis、limitations；
- content_hash、created_at。

证据类型包括公告、新闻原文、公司披露、行情表现、财务数据、量化指标、政策文件和历史样本。source_trust 由来源策略决定，不由 LLM 自行赋值。

Evidence 不复制整篇文章或完整行情数据；存储来源引用、摘要、哈希和必要片段，原文由 News 或相应事实所有者保管。

## 4. Claim 与研究假设

Claim 是可被证据支持或反驳的陈述；Hypothesis 是带验证条件的研究假设。两者不能与 Evidence 混为一体。

Hypothesis 至少包含：
- hypothesis_id、statement、scope、created_at、as_of；
- supporting_evidence_refs、contradicting_evidence_refs；
- validation_method、required_data_refs；
- expected_observation、invalidation_condition；
- status：proposed / testing / supported / weakened / rejected / inconclusive；
- author_type：rule / analyst / llm_assisted；
- version 和 limitations。

假设必须能提出可验证和可证伪条件。不能只写“看好”“利好”而没有作用路径、时间范围和失效条件。

## 5. Event Research

News 与 Event 的职责分开：

| News（新闻分支） | Event Research（分析分支） |
|---|---|
| 来源授权、搜索、采集、原文保存 | 多篇来源归并为同一事件 |
| 标准化、去重、转载关系 | 事件分类、生命周期和版本 |
| 原始发布时间、采集时间、来源可信度 | 证券/行业/产业链实体关联 |
| 原始事实、原文引用和来源诊断 | 影响方向、路径、时效、不确定性 |
| 新闻搜索 API | 事件与价格表现关联验证 |

Event 模型建议包含：
- event_id/version、event_type、title/summary；
- occurred_at、published_at、available_at、observed_at；
- entity_links（证券、行业、主题、产业链）；
- source_evidence_refs、duplicate_cluster_refs；
- lifecycle_status、impact_hypothesis；
- expected_horizon、uncertainty、counter_evidence_refs；
- price_validation_refs、quality、limitations。

生命周期建议：detected → consolidating → active → materializing → fading → invalidated/closed。状态变化必须有事件和证据记录，不能覆盖旧版本。

事件影响研究：
1. 确认事实与事件发生/公开时间；
2. 识别影响对象和产业链路径；
3. 提出正向、负向或不确定影响假设；
4. 明确预期时效和失效条件；
5. 关联事件前后价格、成交和相对表现；
6. 与市场/行业背景和相似事件样本比较；
7. 记录事后验证结果，不能把价格上涨反推成新闻事实正确。

## 6. Scenario

Scenario 是条件化的未来路径，不是单一目标价。至少包含：
- scenario_id、基准时点、预测周期；
- 触发条件、支持/反向证据；
- 预期路径、风险和失效条件；
- 历史相似样本范围、样本数；
- return_distribution_ref、probability_calibration_ref；
- model/rule version、limitations。

概率必须有历史样本和校准证据；无足够样本时返回定性情景或 unknown，不输出伪精确数字。

## 7. LLM 使用约束

LLM 可用于：
- 对已提供证据进行归纳和解释；
- 生成待验证假设和情景；
- 选择允许使用的工具；
- 生成带引用的研究报告草稿。

LLM 不可用于：
- 伪造来源、时间、行情、财务字段或指标值；
- 自行将 inference 升级为 fact；
- 修改 Quant 结果、数据质量状态和硬风险门禁；
- 在没有样本/校准依据时生成概率；
- 绕过工具权限、数据访问范围和输出 Schema。

所有模型输出必须进行 JSON/Schema 校验、引用存在性校验、数值来源校验和安全过滤；校验失败应产生可诊断失败，不允许静默接受自由文本作为结构化事实。
