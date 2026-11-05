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
