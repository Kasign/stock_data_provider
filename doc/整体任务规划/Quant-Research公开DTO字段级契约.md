# Quant / Research 公开 DTO 字段级契约

- 所属治理任务：GOV-004
- 状态：字段级设计草案，待 Quant / Research / Researchflow Review；不是已实现代码，也不是最终冻结 Schema。
- 规划归属：仅维护于 feat/design_doc；模块实现分别归未来 feat/quant、feat/research。
- 目标：跨模块只传稳定 Application DTO 和不透明引用，不暴露 Domain、Repository、Adapter 类型。

## 1. 共同序列化规则

| 项目 | 规则 |
|---|---|
| JSON 命名 | snake_case；发布后不随 Go 字段重命名自动变化 |
| ID / Ref | 不透明非空字符串；消费方不解析、不拼接、不推导内部 ID |
| 时间 | RFC3339 UTC；交易日期单独使用 YYYY-MM-DD（Asia/Shanghai） |
| as_of | 系统知识截止时间，不与交易日、published_at、valid_at 混用 |
| 单位 | 金额 CNY、价格 CNY/股、数量 shares；百分比明确百分点或比例 |
| 缺失 | 区分 required、optional、nullable；unknown 不转 0 / false / 空字符串 |
| 状态 | 稳定枚举；消费方对未知枚举采取 fail-safe |
| 版本 | DTO schema_version、算法/规则版本、数据快照版本分别管理 |
| 扩展 | 核心字段不使用任意 map[string]any；扩展字段必须有命名空间和版本约束 |
| 错误 | 使用 Platform Fault 语义，不暴露 SQL、Provider、SDK 原始错误或 Secret |

公开 DTO 必须显式 JSON Tag。指针字段的 nullable/optional 行为必须由 JSON golden 测试证明；不得用 omitempty 掩盖必需数据缺失。

## 2. Quant Application DTO

### 2.1 QuantAnalysisRequest

由 Researchflow 调用 Quant Application。Quant 不接受 Research Domain 对象，也不直接读取 Research Repository。

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| schema_version | 是 | string | 请求 Schema 版本 |
| request_id | 是 | string | 跨模块关联 ID |
| security_id | 条件必需 | string | 单证券分析必需；市场分析可缺省 |
| scope | 是 | stock/market/industry | 分析范围 |
| as_of | 是 | timestamp | 系统知识截止时间 |
| trade_date | 否 | date | 目标交易日，不替代 as_of |
| period | 是 | string/enum | 分析周期，允许值由接口版本约束 |
| input_snapshot_refs | 是 | string[] | Data 快照引用，保留来源与 PIT 元数据 |
| adjustment_basis | 条件必需 | enum | 涉及价格序列时必需；unknown 必须显式表达 |
| parameters | 是 | typed object | 用例专属参数结构，不允许任意 map |
| quality_policy | 是 | strict/allow_degraded | 不允许 invalid 输入 |
| idempotency_key | 是 | string | 同一请求重试的幂等键 |

Market/Industry 范围必须有明确证券全集或分类版本引用。allow_degraded 只允许按用例规则处理 degraded，不得放行 invalid 或把 unknown 当有效数值。

### 2.2 QuantResultRef

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| schema_version | 是 | string | DTO 版本 |
| result_id | 是 | string | 不透明结果 ID |
| result_type | 是 | indicator/factor/trend/market/industry/stock/strategy/backtest/risk/evaluation | 结果类型 |
| scope | 是 | stock/market/industry/portfolio | 结果范围 |
| security_id | 条件必需 | string | 单证券结果引用 |
| as_of | 是 | timestamp | 计算采用的系统知识截止时间 |
| trade_date | 否 | date | 对应交易日 |
| period | 是 | string | 结果周期 |
| algorithm_version | 是 | string | 算法实现版本 |
| parameter_fingerprint | 是 | string | 规范化参数摘要/哈希 |
| input_snapshot_refs | 是 | string[] | 输入快照引用 |
| quality_status | 是 | valid/degraded/invalid/unknown | 结果质量 |
| result_status | 是 | completed/insufficient_data/insufficient_sample/rejected/failed | 计算状态 |
| created_at | 是 | timestamp | 创建时间 |

completed 不代表 valid；消费方必须同时检查 result_status 和 quality_status。invalid/unknown 不可作为有效数值事实。

### 2.3 QuantMetric

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| metric_code | 是 | string | 稳定指标编码 |
| value | 条件必需 | number/null | 无有效数值时为 null，由 metric_status 解释 |
| unit | 是 | string/enum | CNY、CNY/share、shares、percentage_point、ratio、day 等 |
| period | 是 | string | 统计周期 |
| as_of | 是 | timestamp | 知识截止时间 |
| quality_status | 是 | valid/degraded/invalid/unknown | 质量 |
| metric_status | 是 | available/insufficient_data/insufficient_sample/not_applicable/unknown | 数值可用性 |
| calculation_version | 是 | string | 公式/实现版本 |
| sample_count | 条件必需 | integer/null | 统计/概率类指标必需 |
| lower_bound | 否 | number/null | 区间下界，与 upper_bound 成对 |
| upper_bound | 否 | number/null | 区间上界，与 lower_bound 成对 |

金额和价格必须采用明确 decimal 精度策略；若统计指标使用 float64，必须声明误差容忍、舍入策略和固定样本。value 为 null 时必须说明原因，禁止用 0 代替缺失。

### 2.4 TrendResult

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| trend_code | 是 | string | 趋势类型编码 |
| direction | 是 | bullish/bearish/sideways/unknown | 方向 |
| phase | 是 | emerging/accelerating/mature/weakening/reversing/unknown | 趋势阶段 |
| strength | 是 | number/null | 归一化强度，需定义范围 |
| period | 是 | string | 周期 |
| evidence_metrics | 是 | QuantMetric[] | 支撑指标 |
| conflict_refs | 是 | string[] | 周期冲突/反向结构引用 |
| quality_status | 是 | enum | 质量 |
| as_of | 是 | timestamp | 截止时间 |
| algorithm_version | 是 | string | 算法版本 |

direction 与 phase 必须分开；unknown 不得用 sideways 代替。跨周期冲突必须显式表达。

### 2.5 RiskDecision

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| decision_id | 是 | string | 决策 ID |
| status | 是 | pass/reject/unknown | 硬风险结论 |
| rule_set_version | 是 | string | 规则集版本 |
| evaluated_at | 是 | timestamp | 评估时间 |
| as_of | 是 | timestamp | 输入知识截止时间 |
| reasons | 是 | RiskReason[] | 命中、拒绝或未知原因 |
| input_refs | 是 | string[] | 输入结果/快照引用 |

RiskReason 至少包含 code、severity（block/warn/info）、status（triggered/not_triggered/unknown）、safe message 和 evidence_refs。数据缺失、规则无法评估或输入质量不足时，RiskDecision 必须为 unknown 或 reject，绝不能 pass。硬门禁不提供可由评分或 LLM 调整的 override 字段。

### 2.6 Quant Facade 边界

首期可按用例暴露 AnalyzeStock、AnalyzeMarket、EvaluateRisk、GetResult 等 Application 用例。具体签名由 QNT-001 细化；不得暴露 Repository、SQL Model、Data HTTP DTO 或内部计算结构。结果读取必须验证调用方权限和引用归属。

## 3. Research Application DTO

### 3.1 ResearchContext

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| schema_version | 是 | string | DTO 版本 |
| context_id | 是 | string | 不透明 ID |
| context_version | 是 | integer | 不可变版本 |
| research_question | 是 | string | 研究问题 |
| scope | 是 | market/industry/stock/event | 研究范围 |
| subject_refs | 是 | string[] | 证券/行业/Event 引用 |
| as_of | 是 | timestamp | 系统知识截止时间 |
| section_refs | 是 | ContextSectionRef[] | 分区引用及版本 |
| quality_summary | 是 | QualitySummary | 完整性/质量概览 |
| included_sections | 是 | string[] | 已包含分区 |
| omitted_sections | 是 | OmittedSection[] | 裁剪/缺失分区及原因 |
| builder_version | 是 | string | 语义构建版本 |
| created_at | 是 | timestamp | 创建时间 |

Research 定义 Context 领域语义；Researchflow 负责单次 Run 的分区选择、排序、Token Budget、裁剪和模型上下文装配。裁剪不得静默丢失来源、关键反证和风险限制。

### 3.2 Evidence

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| evidence_id | 是 | string | 不透明 ID |
| evidence_type | 是 | fact/reported_fact/inference/hypothesis/unknown | 认识论类型 |
| source_owner | 是 | data/news/quant/research | 权威来源模块 |
| source_ref | 是 | string | 来源对象引用 |
| source_version | 条件必需 | string | 版本化来源必须提供 |
| source_locator | 否 | string | 来源定位符，不直接存未授权全文 |
| claim_ref | 否 | string | 关联 Claim |
| subject_refs | 是 | string[] | 证券/行业/Event 引用 |
| published_at | 否 | timestamp | 来源声明发布时间 |
| available_at | 否 | timestamp | 有证据支持的外部可获得时间 |
| observed_at | 是 | timestamp | 本系统观察时间 |
| valid_time_start/end | 否 | timestamp | 业务有效区间 |
| system_as_of | 是 | timestamp | 纳入研究时的知识截止时间 |
| stance | 是 | supports/contradicts/neutral | 对 Claim 的立场 |
| confidence | 否 | number/null | 置信表达及其方法来源 |
| quality_status | 是 | valid/degraded/invalid/unknown | 证据质量 |
| authorization_ref | 是 | string | 授权决策/安全状态引用 |
| limitations | 是 | string[] | 使用限制 |
| content_fingerprint | 否 | string | 内容指纹，不替代来源版本 |

fact 必须有可验证来源与时间；reported_fact 只表示来源声称；inference 必须附推导依据；hypothesis 不得伪装成已证实事实；unknown 表达证据不足。支持与反证都必须可追溯。Research 不复制 News 未授权正文，历史读取仍需重新检查当前授权。Evidence 的 system_as_of 不得晚于所属 Run。

### 3.3 Event

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| event_id | 是 | string | 不透明 ID |
| event_version | 是 | integer | 追加式版本 |
| event_type | 是 | string/enum | 事件分类 |
| title | 是 | string | 规范化标题，不替代来源标题 |
| status | 是 | detected/consolidating/active/materializing/fading/invalidated/closed | 生命周期 |
| subject_refs | 是 | string[] | 主体引用 |
| evidence_refs | 是 | string[] | 支持/反证引用 |
| occurred_at | 否 | timestamp | 事件发生时间 |
| published_at | 否 | timestamp | 来源发布时间 |
| available_at | 否 | timestamp | 有证据的外部可获得时间 |
| observed_at | 是 | timestamp | 系统观察时间 |
| system_as_of | 是 | timestamp | 当前版本知识截止时间 |
| impact_hypothesis_refs | 是 | string[] | 影响假设引用 |
| invalidation_conditions | 是 | string[] | 失效条件 |
| uncertainty | 是 | string[] | 不确定因素 |
| supersedes_event_version | 否 | integer | 被修正版本 |

合并、拆分、主体修正和状态变化均追加新版本并保留证据；禁止覆盖历史版本或把 Article 直接当 Event。多来源时间冲突需保留来源级时间，不得压成一个无来源的时间值。

### 3.4 Claim / Hypothesis

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| claim_id | 是 | string | 不透明 ID |
| claim_version | 是 | integer | 追加式版本 |
| statement | 是 | string | 可验证陈述 |
| epistemic_type | 是 | reported_fact/inference/hypothesis | 陈述性质 |
| scope_refs | 是 | string[] | 适用主体/范围 |
| as_of | 是 | timestamp | 知识截止时间 |
| supporting_evidence_refs | 是 | string[] | 支持证据，可为空数组 |
| contradicting_evidence_refs | 是 | string[] | 反证，可为空数组 |
| required_data_refs | 是 | string[] | 验证所需数据引用 |
| validation_method | 条件必需 | string | Hypothesis 必需 |
| expected_observation | 条件必需 | string | Hypothesis 必需 |
| invalidation_conditions | 条件必需 | string[] | Hypothesis 必需 |
| status | 是 | proposed/testing/supported/contradicted/invalidated/unknown | 研究状态 |
| limitations | 是 | string[] | 限制 |

Claim 与 Hypothesis 可共享存储基类，但公开 DTO 必须明确 epistemic_type；Hypothesis 的验证方法、预期观察和失效条件不可缺失。

### 3.5 Scenario

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| scenario_id | 是 | string | 不透明 ID |
| scenario_version | 是 | integer | 版本 |
| subject_refs | 是 | string[] | 适用证券/行业 |
| as_of | 是 | timestamp | 基准知识时点 |
| horizon | 是 | string | 预测周期 |
| conditions | 是 | ScenarioCondition[] | 触发/前提条件 |
| expected_path | 是 | string | 条件化路径描述 |
| evidence_refs | 是 | string[] | 支持与反证 |
| quant_evaluation_ref | 否 | QuantResultRef | Quant 数值评估引用 |
| probability_status | 是 | available/insufficient_sample/unknown/not_evaluated | 概率可用性 |
| risks | 是 | string[] | 风险因素 |
| invalidation_conditions | 是 | string[] | 失效条件 |
| limitations | 是 | string[] | 样本与方法限制 |

Scenario 语义由 Research 持有，概率和收益分布由 Quant 提供。没有可用 Quant 评估或样本不足时，概率必须为 unknown/insufficient_sample，不得由 LLM 生成数值概率。

### 3.6 QualitySummary

最小字段：status（valid/degraded/invalid/unknown）、missing_required_count、missing_optional_count、conflict_count、stale_count、reason_codes。该汇总只描述 Research 自身 Context/Evidence 完整性，不覆盖 Data Probe、News 业务质量或 Platform Diagnostics。

## 4. 模块调用关系和返回约束

| 调用方 | 被调用方 | 输入/输出 | 禁止 |
|---|---|---|---|
| Researchflow | Quant | QuantAnalysisRequest → QuantResult/QuantResultRef/RiskDecision | 访问 Quant Domain/Repository |
| Researchflow | Research | Application Request → Context/Evidence/Event/Claim/Scenario DTO | 直接操作 Research Repository |
| Researchflow | News | News Query Facade → ResearchEvidenceRecord/Page | 直接查询 News SQL/Domain |
| Research | Quant | 不允许 | Research → Quant import |
| Quant | Research | 不允许 | Quant ↔ Research 依赖 |
| LLM | Quant/Research | 不直接调用 | LLM 承担领域事实和计算 |
| Notification | Quant/Research | 不直接依赖 | Notification 计算告警条件 |

Researchflow 是唯一跨模块组合者，并负责传播同一 Run 的 system_as_of。各模块仍需独立校验输入、引用归属和质量状态，不信任调用方伪造的质量标记。

## 5. Schema 与版本策略

1. 每个 Application DTO 单独维护 Schema 版本，不能用模块版本代替 DTO 版本。
2. 增加可选字段可在同一兼容版本进行；改变 required、字段语义、单位、枚举含义或删除字段应提升 major。
3. 新增枚举值存在兼容风险；消费方对未知枚举必须 fail-safe，不得默认 pass。
4. 历史结果保存 schema_version、算法/规则版本和输入引用，不得静默用当前算法重算替代历史结果。
5. 示例 JSON、JSON Schema、Go JSON Tag 和 golden fixture 同步维护。
6. 跨模块契约变更由生产模块提出、消费模块 Review；禁止消费模块复制生产模块私有结构。
7. 时间字段需覆盖 UTC、Asia/Shanghai 交易日转换和缺失值行为。

## 6. 待 Review 决策

- [ ] 价格/金额最终采用 decimal string，还是 JSON number + 明确精度；禁止实现中混用。
- [ ] Quant 参数对象按每个 Use Case 定义独立结构并生成 JSON Schema。
- [ ] input_snapshot_refs 是否允许空数组，仅限纯函数场景并在用例级白名单化。
- [ ] Evidence 的 authorization_ref 对非 News 来源采用统一授权审计引用还是来源专属安全引用。
- [ ] ContextSectionRef、OmittedSection、ScenarioCondition、RiskReason 的完整字段和枚举。
- [ ] Event 发生时间与来源发布时间冲突时保留来源级多值表达。
- [ ] QuantResult 采用强类型子 DTO + 共同元数据，不使用任意 payload map。

## 7. 验收清单

- [ ] Quant/Research 跨模块 DTO 均有字段表、示例 JSON 和 schema_version。
- [ ] required/optional/nullable、单位、时间语义和枚举明确。
- [ ] Quant 结果保留输入快照、算法版本、质量、状态和 PIT。
- [ ] Risk unknown 不通过，硬门禁不可被评分或 LLM 覆盖。
- [ ] Evidence 区分事实/报道事实/推断/假设/未知，保留正反证据和授权引用。
- [ ] Event/Claim/Hypothesis/Scenario 追加式版本可回溯。
- [ ] Research 不依赖 Quant；跨模块组合仅由 Researchflow 负责。
- [ ] JSON golden、Schema 校验、未知枚举和空值测试纳入 CI。
- [ ] DTO 不暴露 Domain、Repository、SQL Model、Data HTTP DTO 或 News 私有类型。

## 8. 当前结论

本文将 Quant / Research 公开 DTO 从概念性字段推进到字段级草案，可进入模块 Owner Review，但价格精度、参数结构、授权引用和复合子对象仍有未决项。GOV-004 仍未通过；本文不构成 Schema 冻结，也不授权创建模块分支。


## 9. 复合 DTO 字段级定义

本节补齐第 6 节列出的复合对象。所有引用均为不透明 ID；数组顺序如影响语义必须由生产方保证并写入 Schema 说明。

### 9.1 ContextSectionRef

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| section_id | 是 | string | 稳定分区编码，如 market_overview、security_profile、quant_metrics、events、risks |
| section_version | 是 | string | 分区内容版本/指纹 |
| source_owner | 是 | data/quant/research/news | 权威来源模块 |
| source_refs | 是 | string[] | 来源对象引用，可为空仅限纯 Research 派生分区 |
| evidence_refs | 是 | string[] | 分区内证据引用 |
| quality_status | 是 | valid/degraded/invalid/unknown | 分区质量 |
| required | 是 | boolean | 本研究模板是否要求该分区 |
| included | 是 | boolean | 是否实际装入 Context |
| observed_at | 是 | timestamp | 本次装配观察时间 |
| system_as_of | 是 | timestamp | 分区知识截止时间 |

同一 Context 中 section_id 唯一；同一 section_id 的不同版本不能同时作为当前版本装入，除非显式标记为对比研究。

### 9.2 OmittedSection

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| section_id | 是 | string | 被省略分区编码 |
| source_owner | 是 | enum | 来源模块 |
| reason_code | 是 | not_requested/budget_exceeded/unavailable/unauthorized/quality_rejected/failed/duplicate | 省略原因 |
| required | 是 | boolean | 是否为模板必需 |
| source_refs | 是 | string[] | 已知来源引用，可为空 |
| safe_message | 是 | string | 可对调用方展示的安全摘要 |
| retryable | 是 | boolean | 是否允许重新装配时重试 |

required=true 的分区被省略时，Context 整体不能标记为 valid；必须由 Researchflow 按 Workflow 门禁决定阻断、降级或失败。

### 9.3 ScenarioCondition

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| condition_id | 是 | string | 条件稳定编码 |
| condition_type | 是 | price/volume/indicator/event/fundamental/market/industry/other | 条件类别 |
| description | 是 | string | 人类可读条件 |
| operator | 是 | gt/gte/lt/lte/eq/neq/in/not_in/crosses/exists/not_exists | 比较运算 |
| metric_ref | 条件必需 | string | Quant Metric 或外部事实引用 |
| expected_value | 条件必需 | typed scalar/object | 目标值；必须与 unit 一致 |
| unit | 条件必需 | string | 比较单位 |
| window | 否 | string | 条件观察窗口 |
| evaluation_status | 是 | satisfied/not_satisfied/unknown/not_evaluated | 当前验证状态 |
| evidence_refs | 是 | string[] | 支撑该条件的证据 |
| required | 是 | boolean | 是否为场景成立的必要条件 |

条件表达式不允许执行任意代码、SQL、脚本或外部 URL。跨多个条件的 AND/OR 组合应使用受限 ConditionGroup（operator=all/any、conditions[]），不允许隐式自然语言解析作为唯一执行依据。

### 9.4 RiskReason

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| code | 是 | string | 稳定机器码 |
| severity | 是 | block/warn/info | 严重级别 |
| status | 是 | triggered/not_triggered/unknown | 规则评估状态 |
| safe_message | 是 | string | 安全摘要，不含敏感内部细节 |
| rule_ref | 是 | string | 规则 ID/版本引用 |
| evidence_refs | 是 | string[] | 触发/无法评估的证据 |
| observed_value | 否 | string/number/null | 脱敏后的观察值 |
| expected_range | 否 | string | 规则阈值摘要 |
| remediation_code | 否 | string | 建议处置机器码 |

只要存在 severity=block 且 status=triggered，RiskDecision 不得 pass。任何必需阻断规则 status=unknown 时，RiskDecision 不得 pass。warn/info 不得被提升为硬门禁，除非规则集版本明确配置。

### 9.5 EvidenceAuthorizationRef

统一引用授权决策，不把授权结论简化为 Evidence 的永久布尔值。

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| authorization_ref | 是 | string | 不透明授权决策 ID |
| policy_owner | 是 | data/news/quant/research | 授权策略权威模块 |
| policy_version | 是 | string | 策略版本 |
| decision | 是 | allowed/denied/conditional/unknown | 当时决策 |
| purpose | 是 | string/enum | 使用目的 |
| content_level | 是 | metadata/summary/snippet/full | 内容级别 |
| subject_ref | 是 | string | 被授权来源对象 |
| decided_at | 是 | timestamp | 决策时间 |
| expires_at | 否 | timestamp | 授权到期时间 |
| conditions | 是 | string[] | 限制条件，可为空 |
| decision_fingerprint | 是 | string | 决策内容指纹 |

该对象是决策审计快照，不代表未来访问自动获准。读取时必须由权威模块按当前策略重新授权；历史 Run 回放同时保留当时决策和当前访问决策，二者不能互相覆盖。

### 9.6 EventSourceTime

Event 允许多来源时间并存，不以单一 occurred_at 覆盖冲突来源。

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| evidence_ref | 是 | string | 对应来源证据 |
| time_type | 是 | occurred_at/published_at/available_at/observed_at | 时间语义 |
| value | 是 | timestamp | 来源/系统时间值 |
| asserted_by | 是 | string | 来源或系统主体引用 |
| confidence | 否 | number/null | 时间可信程度及其评估方法 |
| notes | 否 | string | 时间解释/冲突说明 |

Event 可保留规范化 occurred_at 作为研究归一结果，但必须同时提供 source_times[]；规范化值必须能追溯到所采用的证据与规则版本。

## 10. 最小 JSON 示例与空值规则

以下示例用于固定字段语义，不代表最终 JSON Schema 已生成。示例时间均为 UTC；交易日字段另用 YYYY-MM-DD。

### 10.1 QuantMetric：无数据与有效值

无数据时显式 null，不能省略 value 或填 0：

```json
{
  "metric_code": "volatility_20d",
  "value": null,
  "unit": "ratio",
  "period": "20d",
  "as_of": "2026-11-04T08:00:00Z",
  "quality_status": "unknown",
  "metric_status": "insufficient_data",
  "calculation_version": "volatility.v1",
  "sample_count": 8,
  "lower_bound": null,
  "upper_bound": null
}
```

有效值示例：

```json
{
  "metric_code": "turnover_amount",
  "value": "125000000.00",
  "unit": "CNY",
  "period": "1d",
  "as_of": "2026-11-04T08:00:00Z",
  "quality_status": "valid",
  "metric_status": "available",
  "calculation_version": "turnover.v1",
  "sample_count": 1,
  "lower_bound": null,
  "upper_bound": null
}
```

注意：示例使用 decimal string 表达金额，作为当前推荐方向；最终需在模块 Owner Review 中冻结。若采用该方案，所有金额/价格字段统一 decimal string，不能同一 DTO 中混用 number 和 string。

### 10.2 RiskDecision：未知不得放行

```json
{
  "decision_id": "risk_decision_example",
  "status": "unknown",
  "rule_set_version": "risk.rules.v1",
  "evaluated_at": "2026-11-04T08:01:00Z",
  "as_of": "2026-11-04T08:00:00Z",
  "input_refs": ["quant_result_example"],
  "reasons": [
    {
      "code": "TRADING_STATUS_UNAVAILABLE",
      "severity": "block",
      "status": "unknown",
      "safe_message": "无法确认证券当前交易状态",
      "rule_ref": "risk.trading_status.v1",
      "evidence_refs": [],
      "remediation_code": "REFRESH_SECURITY_STATUS"
    }
  ]
}
```

### 10.3 Evidence：来源知识时间和授权引用

```json
{
  "evidence_id": "evidence_example",
  "evidence_type": "reported_fact",
  "source_owner": "news",
  "source_ref": "news_object_example",
  "source_version": "version_3",
  "subject_refs": ["security_example"],
  "published_at": "2026-11-03T02:00:00Z",
  "available_at": null,
  "observed_at": "2026-11-03T02:05:00Z",
  "system_as_of": "2026-11-04T08:00:00Z",
  "stance": "supports",
  "confidence": null,
  "quality_status": "degraded",
  "authorization_ref": "authorization_decision_example",
  "limitations": ["来源未提供可验证的首次公开时间"],
  "content_fingerprint": "sha256:example"
}
```

available_at=null 表示没有可验证的外部可获得时间，不得用 published_at 或 observed_at 自动填充。授权决策详情由 EvidenceAuthorizationRef 对应权威模块查询。

### 10.4 ScenarioCondition：受限可执行条件

```json
{
  "condition_id": "close_above_ma20",
  "condition_type": "indicator",
  "description": "收盘价高于 MA20",
  "operator": "gt",
  "metric_ref": "metric_close_example",
  "expected_value": "20.50",
  "unit": "CNY/share",
  "window": "1d",
  "evaluation_status": "not_evaluated",
  "evidence_refs": [],
  "required": true
}
```

### 10.5 序列化空值规则

- required 且 nullable：字段必须存在，值可为 null。
- optional 且 non-nullable：无值时省略，不得输出 null。
- optional 且 nullable：是否省略或输出 null 必须由字段级 Schema 固定，禁止同一版本混用。
- required 且 non-nullable：字段必须存在且非 null。
- 空数组表示已评估但集合为空；null 表示未知/不可得（仅在字段允许 nullable 时）。
- 对于计数，0 表示确实统计为零；统计未执行或未知必须用状态字段表达，不能填 0 伪装成功。

## 11. 精度策略建议（待 Owner 冻结）

建议将所有货币和价格金额采用 JSON string decimal：
- 金额：定点 decimal，单位字段明确；如 "125000000.00" CNY。
- 价格：定点 decimal；精度由 Data 源字段和证券市场规则约束，不统一强行四舍五入为两位。
- 比例/收益率：使用 number 仅限有明确范围和误差容忍的统计量；百分比字段明确 percentage_point 或 ratio。
- 计算内部可使用 decimal 或整数最小单位；不得通过 float64 计算货币后再声称精确。
- 所有 decimal string 使用规范格式：十进制、无千分位、无货币符号、无科学计数法；负数使用前导负号。
- JSON Schema 使用 string + pattern/format 约束，并在 Go 侧使用专用 Decimal 类型和 Marshal/Unmarshal 测试。

该建议优于 JSON number 的原因是避免跨语言 IEEE-754 精度损失，尤其金额、价格、成本和收益计算。但需要结合现有 Data OpenAPI 的 number 字段决定 Adapter 边界转换，不要求修改 Data 服务既有 API；Quant 内部和 Quant/Research/Researchflow 跨模块契约应统一。
