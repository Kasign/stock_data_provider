# Quant / Research 公开 DTO 字段级契约

- 所属治理任务：GOV-004
- 状态：字段级设计与项目统一决议输入；当前各契约状态以 `doc/整体任务规划/跨模块契约目录与冻结状态.md` 为准。本文件不是已实现代码，也不是冻结 Schema。
- 规划归属：仅维护于 feat/design_doc；模块实现分别归未来 feat/quant、feat/research。
- 目标：跨模块只传稳定 Application DTO 和不透明引用，不暴露 Domain、Repository、Adapter 类型。

## 1. 共同序列化规则

| 项目 | 规则 |
|---|---|
| JSON 命名 | snake_case；发布后不随 Go 字段重命名自动变化 |
| ID / Ref | 不透明非空字符串；消费方不解析、不拼接、不推导内部 ID |
| 时间 | RFC3339 UTC；交易日期单独使用 YYYY-MM-DD（Asia/Shanghai） |
| system_as_of | 系统知识截止时间，不与交易日、published_at、valid_at 混用 |
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
| request_id | 是 | string | 本次 Quant Application 调用的 Request ID；由 Researchflow 当前 Worker/Adapter 生成或传播，不是 Run 长期关联 ID |
| correlation_id | 是 | string | 关联所属 Run 的跨模块工作链 ID；从 Researchflow Run Execution Context 原样继承 |
| security_id | 条件必需 | string | 单证券分析必需；市场分析可缺省 |
| scope | 是 | stock/market/industry | 分析范围 |
| system_as_of | 是 | timestamp | 系统知识截止时间 |
| trade_date | 否 | date | 目标交易日，不替代 as_of |
| period | 是 | string/enum | 分析周期，允许值由接口版本约束 |
| input_snapshot_refs | 是 | string[] | Data 快照引用，保留来源与 PIT 元数据 |
| adjustment_basis | 条件必需 | enum | 涉及价格序列时必需；unknown 必须显式表达 |
| parameters | 是 | typed object | 用例专属参数结构，不允许任意 map |
| quality_policy | 是 | strict/allow_degraded | 不允许 invalid 输入 |
| idempotency_key | 是 | string | 同一请求重试的幂等键 |

Market/Industry 范围必须有明确证券全集或分类版本引用。allow_degraded 只允许按用例规则处理 degraded，不得放行 invalid 或把 unknown 当有效数值。

QuantAnalysisRequest 的幂等作用域为 `(authenticated_caller_ref, use_case, idempotency_key)`。请求指纹覆盖 security_id、scope、system_as_of、trade_date、period、input_snapshot_refs、adjustment_basis、parameters 和 quality_policy；不覆盖 request_id、correlation_id 和接收时间。相同键/相同指纹返回原 QuantResultRef；相同键/不同指纹返回 idempotency_conflict。Quant 首批计算为无外部副作用的纯计算，不允许把幂等键当作结果 ID。

### 2.2 QuantResultRef

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| schema_version | 是 | string | DTO 版本 |
| result_id | 是 | string | 不透明结果 ID |
| result_type | 是 | indicator/factor/trend/market/industry/stock/strategy/backtest/risk/evaluation | 结果类型 |
| scope | 是 | stock/market/industry/portfolio | 结果范围 |
| security_id | 条件必需 | string | 单证券结果引用 |
| system_as_of | 是 | timestamp | 计算采用的系统知识截止时间 |
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
| value | 是 | string/number/null（由 metric_code 对应 Schema 固定） | 无有效数值时必须显式 null，由 metric_status 解释；精确数值与统计数值采用不同且固定的 JSON 类型 |
| unit | 是 | string/enum | CNY、CNY/share、shares、percentage_point、ratio、day 等 |
| period | 是 | string | 统计周期 |
| system_as_of | 是 | timestamp | 知识截止时间 |
| quality_status | 是 | valid/degraded/invalid/unknown | 质量 |
| metric_status | 是 | available/insufficient_data/insufficient_sample/not_applicable/unknown | 数值可用性 |
| calculation_version | 是 | string | 公式/实现版本 |
| sample_count | 条件必需 | integer/null | 统计/概率类指标必需 |
| lower_bound | 是 | string/number/null（与该 metric_code 的 value 类型一致） | 无区间时显式 null；有区间时与 upper_bound 同时非 null |
| upper_bound | 是 | string/number/null（与该 metric_code 的 value 类型一致） | 无区间时显式 null；有区间时与 lower_bound 同时非 null |

金额和价格必须采用明确 decimal 精度策略；若统计指标使用 float64，必须声明误差容忍、舍入策略和固定样本。value 为 null 时必须说明原因，禁止用 0 代替缺失。

**QuantMetric 组合约束：**
- `metric_status=available` 时 value 必须非 null；`insufficient_data/insufficient_sample/not_applicable/unknown` 时 value 必须为 null。
- `quality_status=invalid/unknown` 时 metric_status 不得为 available；degraded 是否允许 available 必须由该 metric 的 quality_policy 明确声明。
- lower_bound/upper_bound 必须同时为 null 或同时非 null；非 null 时类型与 value 一致且 lower_bound <= value <= upper_bound。value 为 null 时区间必须均为 null。
- sample_count 为非负整数；Registry 声明 sample_count 是否必需及其统计口径，不得用输入行数冒充有效样本数。

### 2.4 TrendResult

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| trend_code | 是 | string | 趋势类型编码 |
| direction | 是 | bullish/bearish/sideways/unknown | 方向 |
| phase | 是 | emerging/accelerating/mature/weakening/reversing/unknown | 趋势阶段 |
| strength | 是 | number/null | 归一化强度；非 null 时范围固定为 [0,1]，并由 algorithm_version 说明归一化方法 |
| period | 是 | string | 周期 |
| evidence_metrics | 是 | QuantMetric[] | 支撑指标 |
| conflict_refs | 是 | string[] | 周期冲突/反向结构引用 |
| quality_status | 是 | enum | 质量 |
| system_as_of | 是 | timestamp | 截止时间 |
| algorithm_version | 是 | string | 算法版本 |

direction 与 phase 必须分开；unknown 不得用 sideways 代替。跨周期冲突必须显式表达。

### 2.5 RiskDecision

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| decision_id | 是 | string | 决策 ID |
| status | 是 | pass/reject/unknown | 硬风险结论 |
| rule_set_version | 是 | string | 规则集版本 |
| evaluated_at | 是 | timestamp | 评估时间 |
| system_as_of | 是 | timestamp | 输入知识截止时间 |
| reasons | 是 | RiskReason[] | 命中、拒绝或未知原因 |
| input_refs | 是 | string[] | 输入结果/快照引用 |

RiskReason 至少包含 code、severity（block/warn/info）、status（triggered/not_triggered/unknown）、safe message 和 evidence_refs。数据缺失、规则无法评估或输入质量不足时，RiskDecision 必须为 unknown 或 reject，绝不能 pass。硬门禁不提供可由评分或 LLM 调整的 override 字段。


### 2.7 QuantResult Envelope 与强类型 Payload

Quant Application 返回的 QuantResult 使用共同 Envelope，不允许 `map[string]any` 或无约束 JSON payload。

| 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| schema_version | 是 | string | Envelope Schema 版本 |
| result_id | 是 | string | 不透明结果 ID |
| result_type | 是 | indicator/factor/trend/market/industry/stock/strategy/backtest/risk/evaluation | 必须命中已注册结果类型 |
| scope | 是 | stock/market/industry/portfolio | 结果范围 |
| security_id | 条件必需 | string | 单证券结果必需 |
| system_as_of | 是 | RFC3339Nano UTC | 计算知识截止时间 |
| trade_date | 否 | YYYY-MM-DD | 对应交易日 |
| period | 是 | string | 结果周期 |
| algorithm_version | 是 | string | 算法语义版本 |
| parameter_fingerprint | 是 | string | 规范化参数指纹 |
| input_snapshot_refs | 是 | array<string> | 输入快照引用；无输入仅限模板白名单纯函数 |
| quality_status | 是 | valid/degraded/invalid/unknown | 结果质量 |
| result_status | 是 | completed/insufficient_data/insufficient_sample/rejected/failed | 计算状态 |
| payload | 是 | discriminated union | 按 result_type + schema_version 命中唯一强类型子 Schema |

同一 result_type/schema_version 的 payload 结构不可原地改变。Researchflow 只校验 discriminator、Schema 和版本并保存结果引用；Research 不解析未知 payload，也不自行计算或重解释指标。未知 result_type/schema_version 必须返回 unsupported_result_type/unsupported_schema，不回退旧版本。


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
| system_as_of | 是 | timestamp | 系统知识截止时间 |
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
| published_at | 是 | timestamp/null | 来源声明发布时间；无来源证据显式 null |
| available_at | 是 | timestamp/null | 有证据支持的外部可获得时间；无来源证据显式 null |
| system_observed_at | 是 | timestamp | 本系统观察时间 |
| valid_time_start/end | 是 | timestamp/null | 业务有效区间；必须同时为 null 或同时提供且 start<=end |
| system_as_of | 是 | timestamp | 纳入研究时的知识截止时间 |
| stance | 是 | supports/contradicts/neutral | 对 Claim 的立场 |
| confidence | 是 | number/null | 仅允许 [0,1]；无可信置信估计时 null；非 null 时 confidence_method 与 confidence_method_version 必填，不能使用 LLM 自评分冒充统计概率 |
| confidence_method | 条件必需 | string | confidence 非 null 时必填，说明估计方法 |
| confidence_method_version | 条件必需 | string | confidence 非 null 时必填，固定方法版本 |
| quality_status | 是 | valid/degraded/invalid/unknown | 证据质量 |
| authorization_ref | 是 | string | 授权决策/安全状态引用 |
| limitations | 是 | string[] | 使用限制 |
| content_fingerprint | 是 | string | 规范化内容指纹，不替代来源版本；同一版本内容变化必须生成新 Evidence 版本 |

fact 必须有可验证来源与时间；reported_fact 只表示来源声称；inference 必须附推导依据；hypothesis 不得伪装成已证实事实；unknown 表达证据不足。valid_time_start/end 必须同时为 null 或同时提供且 start<=end。支持与反证都必须可追溯。Research 不复制 News 未授权正文，历史读取仍需重新检查当前授权。Evidence 的 system_as_of 不得晚于所属 Run。

### 3.3 ResearchEvent

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| event_id | 是 | string | 不透明 ID |
| event_version | 是 | integer | 追加式版本 |
| event_type | 是 | string/enum | 事件分类 |
| title | 是 | string | 规范化标题，不替代来源标题 |
| status | 是 | detected/consolidating/active/materializing/fading/invalidated/closed | 生命周期 |
| subject_refs | 是 | string[] | 主体引用 |
| evidence_refs | 是 | string[] | 支持/反证引用 |
| occurred_at | 是 | timestamp/null | 规范化事件发生时间；无可靠时间时显式 null |
| published_at | 是 | timestamp/null | 单一规范化来源发布时间；多来源原始值以 source_times[] 为准，无统一值时 null |
| available_at | 是 | timestamp/null | 单一规范化外部可获得时间；多来源原始值以 source_times[] 为准，无统一值时 null |
| system_observed_at | 是 | timestamp | 系统观察时间 |
| system_as_of | 是 | timestamp | 当前版本知识截止时间 |
| impact_hypothesis_refs | 是 | string[] | 影响假设引用 |
| invalidation_conditions | 是 | string[] | 失效条件 |
| uncertainty | 是 | string[] | 不确定因素 |
| supersedes_event_version | 否 | integer | 被修正版本 |
| source_times | 是 | EventSourceTime[] | 来源级时间证据；无来源时间证据使用空数组，不压缩或覆盖冲突值 |

合并、拆分、主体修正和状态变化均追加新版本并保留证据；禁止覆盖历史版本或把 Article 直接当 Event。多来源时间冲突需保留来源级时间，不得压成一个无来源的时间值。

### 3.4 Claim / Hypothesis

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| claim_id | 是 | string | 不透明 ID |
| claim_version | 是 | integer | 追加式版本 |
| statement | 是 | string | 可验证陈述 |
| epistemic_type | 是 | reported_fact/inference/hypothesis | 陈述性质 |
| scope_refs | 是 | string[] | 适用主体/范围 |
| system_as_of | 是 | timestamp | 知识截止时间 |
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
| system_as_of | 是 | timestamp | 基准知识时点 |
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

最小字段：status（valid/degraded/invalid/unknown）、missing_required_count、missing_optional_count、conflict_count、stale_count、reason_codes。该汇总只描述 Research 自身 Context/Evidence 完整性，不覆盖 Data Probe、News 业务质量或 Platform Diagnostics。计数字段为非负整数；reason_codes 去重并按字典序排序。

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

## 6. 历史 Review 决策（语义已确认）

- [x] 精确价格/金额采用 decimal string；允许误差的统计值采用 number 并由 Registry 声明 precision/tolerance；Data v1 number 不作为精确计算输入，见 DATA-CTR-005。
- [x] Quant 参数对象按 indicator_code/factor_code + schema_version 定义独立 typed Schema；首批参数不得使用任意 map，见本目录第 8.2/8.3 节。
- [x] input_snapshot_refs 必填；仅模板注册时显式声明 allow_empty_inputs=true 的纯函数场景允许空数组，见 FLOW-CTR-003。
- [x] Evidence 所有来源统一携带 authorization_ref；各 source_owner 签发自己的授权审计快照，公开/无限制访问也用 allow 决策记录，不设置绕过授权链的 not_required 空值。
- [x] ContextSectionRef、OmittedSection、ScenarioCondition、RiskReason 的字段和枚举见第 9.1–9.4 节；Schema/Golden 仍待实现。
- [x] Event 通过 source_times[] 保留来源级 occurred_at/published_at/available_at/system_observed_at，不以单一 occurred_at 覆盖冲突，见第 9.6 节。
- [x] QuantResult 采用共同元数据 Envelope + 按 result_type 区分的强类型 payload；禁止任意 payload map，具体 Registry/Schema 仍待实现。

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

本节历史 Review 决策项均已完成语义选择；剩余未勾选项属于 Schema/Golden/Contract Test 等实现验收证据，不再作为语义待决。本文保留 Quant / Research 公开 DTO 的字段级细节。当前已接受的公共语义和待补证据以项目级契约目录为准；本文件不构成 Schema 冻结，也不授权创建模块分支。


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
| assembled_at | 是 | timestamp | 本次 Context 分区装配时间 |
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
| purpose_code | 是 | enum | research_analysis/risk_review/user_display；受控用途代码 |
| content_level | 是 | metadata/summary/snippet/full | 内容级别 |
| subject_ref | 是 | string | 被授权调用主体 caller_ref |
| resource_ref | 是 | string | 本次授权决策覆盖的来源/内容/Endpoint 资源引用 |
| decided_at | 是 | timestamp | 决策时间 |
| expires_at | 是 | timestamp/null | 无固定到期时显式 null，不代表永久授权 |
| conditions | 是 | string[] | 限制条件，可为空 |
| decision_fingerprint | 是 | string | 决策内容指纹 |

该对象是某次访问决策的审计快照，不代表未来访问自动获准。subject_ref 对应被授权调用主体，resource_ref 对应目标资源。读取时必须由权威模块按当前策略重新授权；历史 Run 回放同时保留当时决策和当前访问决策，二者不能互相覆盖。

### 9.6 EventSourceTime

Event 允许多来源时间并存，不以单一 occurred_at 覆盖冲突来源。

| JSON 字段 | 必需 | 类型/约束 | 语义 |
|---|---|---|---|
| evidence_ref | 是 | string | 对应来源证据 |
| time_type | 是 | occurred_at/published_at/available_at/system_observed_at | 时间语义 |
| value | 是 | timestamp | 来源/系统时间值 |
| asserted_by | 是 | string | 来源或系统主体引用 |
| confidence | 否 | number/null | 仅允许 [0,1]；非 null 时 confidence_method 与 confidence_method_version 必填 |
| confidence_method | 条件必需 | string | confidence 非 null 时必填 |
| confidence_method_version | 条件必需 | string | confidence 非 null 时必填 |
| notes | 否 | string | 时间解释/冲突说明 |

Event 的 source_times 必填为非 null 数组；无来源时间时为空数组。Event 可保留规范化 occurred_at 作为研究归一结果，但必须同时提供 source_times[]；规范化值必须能追溯到所采用的证据与规则版本。


### 9.7 Evidence PIT、授权与内容指纹规则

Evidence 必须保留来源事实、系统观察和研究纳入时间三条独立时间线：

- `published_at`：来源声明的发布时间；无来源时间证据时 null。
- `available_at`：来源内容可被外部获取的时间；无可验证证据时为 null/unknown，不得由 published_at、fetched_at 或 system_observed_at 推导。
- `system_observed_at`：系统首次观察该来源事实/版本的时间，必填。
- `system_as_of`：本次研究允许使用的知识截止时间，必填，且不得早于 system_observed_at。
- `valid_time_start/end`：业务事实有效区间；与上述知识时间独立。

严格 PIT 规则：available_at 为 null/unknown 的 Evidence 不得作为“在该时点已公开可得”的事实参与严格 PIT 研究；如果调用场景只要求“系统当时已观察”，必须由 Researchflow Template 显式声明非严格 PIT 模式及其风险，不得由 Research 自动降级。

所有 Evidence 必须有 `authorization_ref`。授权快照由 `source_owner` 对应的权威模块签发：Data/News/Quant/Research 各自负责其拥有内容的访问策略。公开或无限制内容也必须签发 allow 快照；禁止 null、空字符串或 not_required 绕过授权审计。该快照不是访问令牌；读取敏感内容时仍由来源 Owner 重新执行当前授权检查。

`content_fingerprint` 必填，使用 RFC 8785 JSON Canonicalization Scheme 规范化受控 Evidence 内容后计算 SHA-256。指纹覆盖 source_owner、source_ref、source_version（若有）、source_locator（若有）、evidence_type、subject_refs、来源时间字段、stance 和可被当前授权引用的内容摘要；不覆盖本系统接收时间、数据库主键和可变展示字段。不得将未经授权的全文纳入 Research 指纹输入；只对当前可访问的规范化表示计算。内容变更必须生成新 Evidence 版本/指纹，不覆盖历史记录。

### 9.8 ResearchContext 与 QualitySummary 约束

- `context_version` 必须单调递增；已发布版本不可原地修改。重新装配、增删分区或改变证据集合必须产生新版本。
- `section_id` 在同一 Context 内唯一；`section_refs` 与 `included_sections` 必须一一对应，`omitted_sections` 不得与 included_sections 重叠。
- `included_sections` 和 `omitted_sections` 按 Template 定义的稳定优先级排序；同优先级按 section_id 升序，保证 fingerprint 稳定。
- `QualitySummary` 字段固定为 status、missing_required_count、missing_optional_count、conflict_count、stale_count、reason_codes；计数字段为非负 integer，reason_codes 去重并按字典序排序。
- 任一 required section 被 omitted，Context.status 不得为 valid；具体是否阻断 Run 由 Researchflow Template 的 Stage 传播策略决定。Research 不自行把质量状态改写为 Run/Stage 执行状态。
- `retryable` 仅表示来源/装配操作在技术层面可能重试，不替代 Researchflow 的重试预算、deadline 和幂等策略。

### 9.9 ResearchEvent / Claim / Hypothesis 版本与状态约束

- ResearchEvent、Claim、Hypothesis、Scenario 均采用追加式版本；同一对象的版本号从 1 开始严格递增，版本不可覆盖或复用。
- 修正必须声明 supersedes_version；若对象被拆分/合并，必须记录 source_refs 与 derived_from_refs，禁止删除旧对象或重写历史引用。
- ResearchEvent 状态迁移由 Research 领域规则注册；MVP 允许 detected -> consolidating -> active -> materializing -> fading -> closed，任一非终态可转 invalidated；禁止从 closed/invalidated 回到 active，恢复必须创建新 Event 并引用旧对象。
- Claim/Hypothesis 状态迁移：proposed -> testing -> supported/contradicted/invalidated/unknown；supported/contradicted/invalidated 为终态。新证据改变结论时创建新版本，不回写历史版本状态。
- confidence 若存在必须在 [0,1]，同时记录 confidence_method/version；未定义方法时 confidence 必须 null，不允许把 LLM 自评分当成统计概率。
- Research 不自行生成 Quant probability/return/risk 数值；所有数值结论必须引用 QuantResult/Metric 的 code、schema_version、algorithm_version 和输入快照。


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
  "system_as_of": "2026-11-04T08:00:00Z",
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
  "system_as_of": "2026-11-04T08:00:00Z",
  "quality_status": "valid",
  "metric_status": "available",
  "calculation_version": "turnover.v1",
  "sample_count": 1,
  "lower_bound": null,
  "upper_bound": null
}
```

注意：示例使用 decimal string 表达精确金额/价格；该类型规则已纳入 GOV-CTR-001 与 QNT-CTR-001。具体字段精度和范围仍需 Registry/Schema 固定。

### 10.2 RiskDecision：未知不得放行

```json
{
  "decision_id": "risk_decision_example",
  "status": "unknown",
  "rule_set_version": "risk.rules.v1",
  "evaluated_at": "2026-11-04T08:01:00Z",
  "system_as_of": "2026-11-04T08:00:00Z",
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
  "system_observed_at": "2026-11-03T02:05:00Z",
  "system_as_of": "2026-11-04T08:00:00Z",
  "stance": "supports",
  "confidence": null,
  "quality_status": "degraded",
  "authorization_ref": "authorization_decision_example",
  "limitations": ["来源未提供可验证的首次公开时间"],
  "content_fingerprint": "sha256:example"
}
```

available_at=null 表示没有可验证的外部可获得时间，不得用 published_at 或 system_observed_at 自动填充。授权决策详情由 EvidenceAuthorizationRef 对应权威模块查询。

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

## 11. 精度策略基线（已由 GOV-CTR-001 接受）

建议将所有货币和价格金额采用 JSON string decimal：
- 金额：定点 decimal，单位字段明确；如 "125000000.00" CNY。
- 价格：定点 decimal；精度由 Data 源字段和证券市场规则约束，不统一强行四舍五入为两位。
- 比例/收益率：使用 number 仅限有明确范围和误差容忍的统计量；百分比字段明确 percentage_point 或 ratio。
- 计算内部可使用 decimal 或整数最小单位；不得通过 float64 计算货币后再声称精确。
- 所有 decimal string 使用规范格式：十进制、无千分位、无货币符号、无科学计数法；负数使用前导负号。
- JSON Schema 使用 string + pattern/format 约束，并在 Go 侧使用专用 Decimal 类型和 Marshal/Unmarshal 测试。

该建议优于 JSON number 的原因是避免跨语言 IEEE-754 精度损失，尤其金额、价格、成本和收益计算。但需要结合现有 Data OpenAPI 的 number 字段决定 Adapter 边界转换，不要求修改 Data 服务既有 API；Quant 内部和 Quant/Research/Researchflow 跨模块契约应统一。

## 12. Quant / Research 历史决议记录（2026-11-05，现由领域契约目录接管）

本节是供 Owner 确认的建议决议，不代表 Owner 已签字或 Schema 已冻结。

### 12.1 GOV-CTR-001 / QNT-CTR-001：数值 JSON 类型与精度（历史别名 CONTRACT-001）

建议采用按指标语义固定类型的策略，而不是全局强制所有数值均为 string 或 number：

| 数值类别 | JSON 表达 | 规则 |
|---|---|---|
| 价格、金额、单价、成本、费用 | decimal string | 十进制精确表达；Schema 固定小数位/最大精度和舍入模式 |
| 股数、样本数、计数 | integer | 明确整数范围；禁止浮点计数 |
| 比例、收益率、波动率、标准化分数 | number | 明确单位、计算精度、舍入和绝对/相对误差 |
| 无效或不可用数值 | null | 字段按 DTO required 规则输出，原因由 metric_status/quality_status 表达 |

具体约束：
1. 每个指标编码在 Metric Registry 声明 JSON 类型、unit、精度、舍入模式、允许范围和误差容忍。
2. QuantMetric 的 value/lower_bound/upper_bound 遵循同一指标编码的同一类型；不得同一指标有时返回 string、有时返回 number。
3. lower_bound 与 upper_bound 同时存在或同时为 null；无区间时显式 null。若指标不支持区间，应在专属 Schema 中省略这两个字段。
4. 金额/价格在 Go 内部使用明确十进制类型或经过验证的十进制运算边界；禁止通过 float64 来回转换后宣称精确。
5. JSON number 统计指标定义稳定舍入位数和 golden 容差；NaN、+Inf、-Inf 不允许序列化，必须转为明确的 unknown/failed 结果。
6. QuantMetric 通用字段的 value 类型由指标编码 Schema 约束；JSON Schema 应按指标类型拆分 oneOf 或使用指标专属 DTO，不允许宽泛接受任意 string/number。

**建议决议：采纳按指标语义固定 JSON 类型；金额/价格 decimal string，计数 integer，统计比例 number。** 该决议已由 GOV-CTR-001/QNT-CTR-001 登记，Schema/Golden 未齐前不标记 frozen。

### 12.2 QNT-CTR-002：Indicator 首批代码与参数 Schema（历史别名 CONTRACT-002）

首批只纳入可以由日线 OHLCV 稳定复算、定义成熟且能提供固定样本的指标：

| indicator_code | 首批参数 | 输入要求 | 输出/边界 |
|---|---|---|---|
| sma | window | close | window 个有效样本；不足返回 insufficient_data |
| ema | window、seed_method | close | seed_method 固定为版本化枚举；初始化算法不得隐式改变 |
| return | window、return_basis | close | return_basis 明确 simple/log；单位 ratio |
| volatility | window、return_basis、annualization_factor | close | 明确收益序列和年化因子；样本不足不得输出有效值 |
| rsi | window、smoothing_method | close | 平滑算法版本化；固定边界样本 |
| macd | fast_window、slow_window、signal_window | close | fast_window < slow_window；分别输出 DIF/DEA/histogram |
| atr | window、smoothing_method | high/low/close | 需要完整 OHLC；复权口径一致 |
| bollinger | window、multiplier、stddev_method | close | 输出 middle/upper/lower；标准差算法固定 |

首批规则：
- 参数必须进入 indicator-specific typed DTO；不接受任意 map。
- indicator_code + schema_version 唯一确定参数 Schema 和默认值；禁止调用方省略参数后依赖不透明默认值。
- 首批不纳入复杂形态识别、自动参数搜索、任意表达式和用户脚本。
- 输入序列必须包含交易日、质量、单位和复权语义；窗口按交易日有效样本计数，缺失日处理策略由指标定义明确。
- 每个指标至少提供正常、边界、缺失、异常、复权切换五类固定样本；结果包含 algorithm_version 和输入快照引用。

**决议：以上八项作为首批范围；如需缩减或扩张，按项目契约变更流程更新 Registry、Schema 和消费者映射。**

### 12.3 QNT-CTR-003：Factor 首批候选与横截面规则（历史别名 CONTRACT-003）

首批候选因子：

| factor_code | 候选定义 | 关键输入 |
|---|---|---|
| momentum_20d | 20 个有效交易日收益动量 | 复权收盘价 |
| momentum_60d | 60 个有效交易日收益动量 | 复权收盘价 |
| volatility_20d | 20 日收益波动 | 复权收盘价、收益口径 |
| turnover_mean_20d | 20 日平均换手率 | 换手率及来源/质量 |
| relative_strength_20d | 相对指定基准的 20 日收益差 | 个股与基准同口径收益 |
| volume_ratio_20d | 当日成交量相对 20 日基准量 | 成交量、停牌/异常成交处理 |

横截面契约：
- 必须固定 universe_ref、trade_date、as_of、证券主数据版本和行业分类版本（若中性化）。
- 标准化顺序固定为：样本资格过滤 → 缺失处理 → 异常值处理 → 中性化（如启用）→ 标准化；每一步输出样本数和排除原因。
- winsorize 必须指定分位数/阈值；zscore 必须指定母体标准差定义；rank 必须指定并列排名策略和输出区间。
- 缺失样本不能按 0 参与；minimum_coverage 不满足时结果为 insufficient_data/unknown，不输出可用于排序的有效因子值。
- Indicator 与 Factor 是并列子系统；Factor 可直接消费标准化数据，不要求依赖 Indicator Engine。

**决议：首批固定 momentum_20d、momentum_60d、volatility_20d；其余候选 deferred，见 QNT-CTR-003。**

### 12.4 NEWS-CTR-002 / RSC-CTR-001：Evidence 授权引用（历史别名 CONTRACT-004）

建议统一保留 authorization_ref 不透明字符串引用；授权决策快照本身由授权策略 Owner 管理，不由 Research 复制成自己的授权事实。

- policy_owner 必须是实际对该来源/内容拥有授权决策权的模块或策略主体。
- News/Data 内容读取由来源 Owner 重新执行当前授权检查；Research 保存当时的 decision_ref、policy_version、content_level、purpose、scope 和 fingerprint。
- 历史回放同时保留当时授权决策引用与本次读取的当前授权结果；当前 denied/expired 不得因历史 allowed 而放行。
- Research 派生内容由 Research 对自身派生记录负责；引用外部来源时仍保留原始来源授权链，不能用 Research 自身决策覆盖上游限制。
- 授权状态 unknown/缺少决策引用时，不允许读取受限正文；可在允许范围内保留 metadata 引用并标记限制。

**决议：采用统一不透明 AuthorizationRef + 来源责任模块当前复核；不建立跨模块万能授权服务，见 NEWS-CTR-002/RSC-CTR-001。**

### 12.5 GOV-CTR-002 / FLOW-CTR-003：system_as_of 传播（历史别名 CONTRACT-005）

建议将 ResearchRun.system_as_of 作为一次研究运行唯一的系统知识截止时间：

1. CreateRun 时确定并持久化 UTC 时间；重试 Stage 不得自动刷新。
2. QuantAnalysisRequest、ResearchContext、Evidence 写入/查询和 News Query 均接收同一 system_as_of。
3. 子对象的 system_as_of 不得晚于 Run.system_as_of；若来源数据在该时点不可见，必须排除或标记不可用。
4. system_observed_at 是事实被系统观察并持久化的实际时间，不能为了满足 PIT 而回填成 Run.system_as_of。
5. trade_date 是交易日，不是知识截止时间；published_at 是来源声明时间，不是系统已知时间。
6. 新建 Run 可选择新的 as_of；原 Run 的历史输入快照和结果不可被新时点覆盖。

**决议：Run 固定 system_as_of，所有下游模块只消费该值；禁止 Stage 各自取当前时间，见 GOV-CTR-002/FLOW-CTR-003。**

### 12.6 历史 Review 状态（由项目统一契约目录覆盖）

| 决策 | 建议 | 当前状态 |
|---|---|---|
| CONTRACT-001（历史别名） | 数值 JSON 类型与精度按 Registry 固定 | 项目决议已接受；QNT-CTR-001 待 Schema/Golden |
| CONTRACT-002（历史别名） | 八项 Indicator 候选及 typed 参数 Schema | 项目决议已接受；QNT-CTR-002 待 Schema/Golden |
| CONTRACT-003（历史别名） | 首批三个 Factor 与算法边界 | 项目决议已接受；QNT-CTR-003 待 Schema/Golden |
| CONTRACT-004（历史别名） | 不透明 AuthorizationRef 与来源 Owner 当前复核 | 项目决议已接受；NEWS-CTR-002/RSC-CTR-001 待 Schema/Golden |
| CONTRACT-005（历史别名） | Run 唯一固定 system_as_of | 项目决议已接受；FLOW-CTR-003 待 Adapter Golden |

**历史说明：本表最初是规划侧建议，不再要求虚构具体 Owner 签字。当前统一决议与状态以 `doc/整体任务规划/跨模块契约目录与冻结状态.md` 为准；项目决议已接受不等于 Schema/Golden/Contract Test 已通过。**

## 13. 五模块交叉契约映射补充（2026-11-06）

- Researchflow `ResearchRun.system_as_of` 是 Run 唯一系统知识截止值。QuantAnalysisRequest/QuantResultRef、ResearchContext/Claim/Scenario 的 `system_as_of` 必须沿用该值；Evidence 的 `system_as_of` 与其语义相同，且不得晚于所属 Run。
- `system_observed_at` 始终记录系统实际观察/持久化时间；不能用 system_as_of 回填。published_at/available_at/valid_time 保留各自来源时间和业务有效时间。Data API 查询参数 `as_of` 的映射由 Quant Data Adapter 负责，不改写 Data 自身时间语义。
- Evidence.authorization_ref 指向 EvidenceAuthorizationRef 审计快照，包含 policy_owner、policy_version、decision、purpose、content_level、subject_ref、decided_at、expires_at、conditions 和 decision_fingerprint。它不是永久访问令牌；读取受限内容仍需来源 Owner 当前授权复核。Evidence 不用 authorization_status 替代该引用。
- ResearchEvent 是多来源证据归并后的领域对象；NotificationEvent 是通知模块消费的版本化集成消息。两者 ID/版本空间独立，转换必须显式映射并生成独立 event_id。
- quality_status、Quant result_status、RiskDecision、Run/Stage/Attempt、ModelResponse.status、Delivery.status 分域管理；同名状态不可直接转换。Run partial 表示覆盖不完整；Run succeeded 仍可有 degraded 质量摘要；Risk unknown 不可 pass。
- request_id 标识一次调用；correlation_id 贯穿链路；event_id 标识稳定事件；idempotency_key 标识命令意图。幂等使用 Owner 定义作用域与 fingerprint；相同键同指纹返回原结果，不同指纹冲突。

**以上字段语义由项目级契约目录统一治理；历史 CONTRACT-001~005 仅作为追溯别名。字段名统一使用 system_as_of；Schema/Golden/Contract Test 未完成前不构成 frozen。**
