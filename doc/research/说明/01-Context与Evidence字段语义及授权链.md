# Research Context / Evidence 字段语义与授权链补充

- 适用任务：RSC-001～006
- 契约范围：RSC-CTR-001～003、DATA-CTR-007、NEWS-CTR-001～003、GOV-CTR-002/006
- 状态（2026-11-18 校准）：Research 本地 DTO/Schema/Golden、领域校验、Repository、News Adapter 和 Outbox 已有实现；News→Research A 组授权/PIT/fail-closed 验收已通过，B 组真实 Evidence 成功写入为 shelved；消费者 Contract Test、Context Owner Authorizer 生产装配和真实数据验收仍按依赖推进。
- 权威关系：项目级契约目录及生产方 DTO 是公共语义权威；本文件约束 Research 的消费、留存和派生行为

## 1. 模块职责和依赖边界

Research 拥有 Context、Evidence、Event、Claim、Hypothesis、Scenario 的领域语义和版本。Data 拥有行情/财务等原始事实，News 拥有新闻来源、原文、Observation 和 Evidence Query，Quant 拥有数值计算。Research 不复制上游 Repository，不依赖 Researchflow，不重算 Quant 指标，也不自行签发 Data/News/Quant 的授权。

跨模块依赖仅通过 Research 自己的 Adapter 消费公开 Application DTO。上游 API、Schema 或权限未达到证据门槛时，Research Domain 可用合成 Fixture 开发，但不得把 Fixture 登记为真实来源接入通过。

## 2. Context 字段语义

ResearchContext 至少包括 schema_version、context_id、context_version、research_question、scope、subject_refs、system_as_of、section_refs、quality_summary、included_sections、omitted_sections、builder_version、created_at。

Context 的 system_as_of 必须与所属 Researchflow Run 完全一致；不得在装配或重试时刷新。section_refs 是不可变分区/证据集合引用，不等于授权，也不代表内容当前可读取。Context 不复制来源全文；读取内容时仍由资源 Owner 执行当前授权复核。

Research 定义分区结构、质量和语义校验；Researchflow 决定本次 Run 选取哪些分区、顺序、Token Budget 和裁剪策略。裁剪必须保留来源引用、时间、反证、unknown 和风险限制。required 分区缺失时 Context 不得标 valid；每个 omitted section 必须有稳定 reason_code。

**Context 受控读取：** 按 DEC-PLAT-002-024，Context 元数据本身属于受控 Research 资源。Application 的 ContextRef 只定位不可变版本，不构成读取许可。GetContext 必须单独接收 Server/可信入口注入的 caller_ref 与受控 purpose，并由 Research Owner ContextAuthorizer 对精确版本的 scope、subject_refs 和当前可见范围进行复核；未装配授权器、拒绝、unknown 或依赖不可用均不得返回 Context。此门禁与 section_refs 中各 Data/News/Quant 来源内容的授权是两层独立检查：Context 可见不代表其来源正文可读，来源 Owner 仍须在读取内容时重新授权。当前冻结决议尚未定义 Context 专用 resource_ref 拼接语法和策略注册表结构，不在 Research 中擅自增加。

## 3. Evidence 字段和认识论约束

Evidence DTO 的规范字段如下。RSC-CTR-001 的 Schema `$id` 标识 1.0.0 版本，JSON payload 不包含 schema_version。旧文档中的 source_type、observed_at、as_of、content_hash 仅为早期概念草稿，不作为跨模块 JSON 字段：

| 字段 | 必需/nullable | 语义 |
|---|---|---|
| evidence_id | 必填 | Research 内不透明 ID；契约版本由 JSON Schema `$id` 标识，payload 不增加 schema_version |
| evidence_type | 必填 | fact/reported_fact/inference/hypothesis/unknown |
| source_owner | 必填 | data/news/quant/research |
| source_ref | 必填 | 上游资源或来源对象引用 |
| source_version | 条件必填 | 上游资源可版本化时必须提供；News Evidence 固定引用不可变 VersionID，因此 `source_owner=news` 时必填 |
| source_locator | 可选 | 内容定位符；不得放未授权正文 |
| derived_from_refs | 必填数组 | 直接上游 Evidence 引用；fact/reported_fact 可为空，inference/hypothesis 必须非空 |
| claim_ref | nullable | 关联 Claim |
| subject_refs | 必填数组 | 证券/行业/Event 主体引用 |
| published_at / available_at | 必填 nullable | 来源声明时间/有证据的外部可获得时间；无证据显式 null |
| system_observed_at | 必填 | 本系统观察该来源/版本的时间 |
| valid_time_start / valid_time_end | 必填 nullable | 必须同时 null 或同时存在且 start<=end |
| system_as_of | 必填 | 本 Evidence 纳入研究时的知识截止时间 |
| stance | 必填 | supports/contradicts/neutral |
| confidence | 必填 nullable | [0,1]；无可信统计估计时 null，不得使用 LLM 自评分 |
| confidence_method / confidence_method_version | 条件必填 | confidence 非 null 时必填 |
| quality_status | 必填 | valid/degraded/invalid/unknown |
| content_level | 必填 | metadata/summary/snippet/full，必须与授权快照一致 |
| authorization_ref | 必填 | 资源 Owner 签发的 AuthorizationRef，或 Research 对自身派生内容签发的引用 |
| limitations | 必填数组 | 使用限制 |
| content_fingerprint | 必填 | 除 evidence_id、authorization_ref、content_fingerprint 外的规范化语义字段按 RFC 8785 JCS + SHA-256 计算 |

- fact 必须有可验证来源和时间证据；reported_fact 只表达来源声称，不暗示独立核验。
- inference 必须保留直接上游 Evidence 引用；hypothesis 必须有验证方法、预期观察和失效条件。
- unknown 表达证据不足；不得用空字符串、0、false 或默认 valid 代替。
- Evidence 的 system_as_of 不得晚于所属 Run；Research Diagnostics Application 的 `BuildEvidenceCheck(..., runAsOf, ...)` 必须接收 Researchflow CreateRun 的非零 UTC cutoff，并传入 `ValidateForStrictPIT(runAsOf)`。该校验同时约束 Evidence.system_as_of 与 available_at；不得使用 Evidence 自身 system_as_of 代替 Run cutoff。语义变化必须追加新 Evidence ID/版本，不得原地覆盖。
- stance 是相对 Claim 的立场；无 claim_ref 时不得把无关证据误作支持。

## 4. AuthorizationRef 与派生来源链

AuthorizationRef 遵循 GOV-CTR-006，是审计快照引用，不是 bearer token。

- source_owner=data/news/quant：authorization_ref 必须由对应来源 Owner 签发；subject_ref 来自可信调用身份；resource_ref 精确对应 snapshot_ref、Evidence 版本或 Quant result_id。
- source_owner=research：Research 对自己的 Evidence 资源签发引用；不得覆盖上游 denied、conditional、unknown、expired 或 revoked。
- derived_from_refs 保存直接上游 Evidence 引用；沿引用链保留各来源 Owner 的 authorization_ref。混合来源中任一来源当前不可披露时，重新计算可安全披露范围；无法剥离则拒绝整个派生内容。
- 每次读取受限内容都向资源 Owner 复核当前策略、Scope、有效期和撤销状态；授权服务不可用或结果不完整时 fail closed。
- Research 本地 `AuthorizationRef.Validate(snapshot)` 只验证 GOV-CTR-006 快照结构、policy_version/snapshot_ref、策略 tuple 完整性/排序和 decision_fingerprint；`ValidateEvidenceAuthorizationBinding` 只将签发快照与 Adapter 提供的可信 subject/resource/purpose 及 Evidence.content_level 绑定。两者都不能证明授权当前有效，禁止单独作为正文读取许可。
- content_level 不得高于任何上游来源当前允许级别；Research 摘要/推断不会自动获得更高披露等级。
- content_fingerprint 用于内容一致性审计，不是签名或授权凭证。

### 4.1 AuthorizationRef 本地校验边界

- `AuthorizationRef` 与 `AuthorizationPolicySnapshot` 字段严格对应 GOV-CTR-006 1.0.0 Schema；`policy_version` 必须等于 `snapshot_ref`。
- `policies` 必须非空、tuple 唯一且按 `(policy_type, policy_ref, policy_version)` 字典序排列；指纹必须按完整策略集合复算。
- `allowed` 的 conditions 必须为空；`conditional` 必须有条件，且不能用于读取；`denied/unknown` 一律不可读取。
- subject_ref 只能与认证上下文提供的可信主体比较；resource_ref 必须与 Owner 返回的精确资源引用比较；用途和 content_level 必须匹配本次调用。
- 当前 Validator 不签发授权、不查询策略快照存储、不复核撤销/过期，不提供并发读取一致性保证。上述能力由各资源 Owner 实现并通过 GOV-CTR-006 L2 Contract Test 验收。

## 5. 时间与 PIT

绝对时间使用 RFC3339Nano UTC；交易日使用 YYYY-MM-DD 并按 Asia/Shanghai 交易日历解释。字段语义独立：published_at 是来源声明发布时间；available_at 是有证据的外部可获得时间；system_observed_at 是本系统观察时间；valid_time_start/end 是业务有效区间；system_as_of 是研究知识截止时间。

严格 PIT 必须满足来源可用时间和系统观察时间边界。available_at 无证据或晚于 system_as_of 时不得进入有效历史事实集合；不能以 published_at、fetched_at 或新闻主体首次观察时间替代版本 Observation 时间。Data 首批严格 PIT 输入仅按 DATA-CTR-007 接入 DailyBars v2；Data v1 float64 或无 AuthorizationRef 的结果不得作为精确 Evidence。

## 6. Event 版本与通知映射

ResearchEvent 是跨来源现实事件，不是 News Article 别名。创建、合并、拆分、主体修正或生命周期迁移均追加版本；source_times 保留来源级时间冲突。

Research Event 与 Claim 作为 Run 诊断快照时，其 system_as_of 必须不晚于 Researchflow CreateRun 的 runAsOf；Context 的 system_as_of 则必须与 Run cutoff 完全一致。四类诊断统一通过 Application Build*Check 显式接收该参数，不扩展领域 DTO。
- Event 若提供 available_at，必须满足 available_at <= system_as_of；Research Event 的 Domain 校验与 Evidence 严格 PIT 规则保持一致。由于 Event.system_as_of 还必须 <= Run cutoff，该约束可传递保证 available_at 不晚于 Run cutoff。

当前 `internal/research/event/` 已实现 Event/SourceTime 领域校验、版本追加校验和 MVP 生命周期转换；Event DTO 含 `time_normalization_rule_version`，仅接受已注册的 V1 规则版本。Validator 要求非 null 的 occurred_at/published_at/available_at 有匹配 SourceTime；同类来源时间一致时规范化值必须等于共识值，来源冲突时规范化值必须为 null。该实现证明规则版本字段与确定性共识校验已存在，不证明真实来源时间精度、来源声明主体或多来源合并/拆分业务样本已经验收。

`internal/research/storage/` 已实现 Event SQL Repository，版本追加与生命周期 Outbox 在同一事务提交，并支持 Outbox 领取、租约、重试、确认和拒绝。Research Event Domain 不直接查询 News Evidence Repository；News→Research Evidence Query/Importer 是独立 Adapter 链路。

生命周期：detected/consolidating/active/materializing/fading/invalidated/closed。只有状态真正迁移并持久化后才按 INT-CTR-001 发布 lifecycle_changed；Evidence 追加、来源补录或描述修订但状态未变，不发布。事件通过 Research 本地事务 Outbox 发布，不直接调用 Notification 内部接口。

## 6.1 RSC-004 Data 实体引用依赖复核

2026-11-09 只读核对 Data 分支 `feat/full-go-data-providers-api-switch`，HEAD 为 `7b3aa2a099b29b841caf93b0890985ecbc002cf4`。Market Data 已迁入根 Go Module（`internal/marketdata` 存在，`services/market-data/go.mod` 已不存在）；代码树仍未发现 SecurityRef、IndustryRef、SecurityMaster、IndustryClassification 或独立 GetSecurity/ListIndustries 查询端口。当前 `SecurityTradingStatus` 是具体状态数据，不是稳定证券主数据身份；旧 `internal/analysis/app/ports.go` 泛化 UniverseResolver 也不能视为 Data 所有的稳定实体契约。故 RSC-004 暂不开发 Adapter，也不在 Research 中复制 Security/Industry 模型。待 Data 提供稳定引用、历史版本/知识时间语义和公开 Application Port 后再继续。

## 6.2 RSC-006 Scenario 契约准入复核（2026-11-18）

此前关于缺少 expected_value Schema 和 ConditionGroup 联合结构的 `RSC-OPEN-006` 已过期：当前已存在 `doc/整体任务规划/schema/RSC-CTR-002-scenario-1.0.0.schema.json`、对应 Golden，以及 `internal/research/scenario/model.go` 的递归 Condition/ConditionGroup 校验。expected_value 在 Go DTO 中使用通用承载类型，但 Validator 只接受字符串、有限数值、布尔值或非空标量数组；不接受任意对象/嵌套表达式；exists/not_exists 必须省略 expected_value，in/not_in 必须使用非空数组。ConditionType 对 metric_ref/unit 的要求也已有校验。

剩余边界不是再造 Schema，而是 Quant Metric Registry 的具体指标类型/单位/版本映射、QuantEvaluationRef 的真实来源和授权复核，以及 Researchflow 是否存在真实消费场景。Research 不直接调用 Quant。未有真实消费者前，不新增 Scenario Application API、Repository、Migration 或独立 Worker。

## 6.3 Claim / Hypothesis 领域实现边界

当前 `internal/research/claim/` 以 Claim DTO 表达 reported_fact、inference 和 hypothesis 三类认识论状态；Hypothesis 必须提供 validation_method、expected_observation 和非空 invalidation_conditions，支持与反证引用不能重叠。Claim Validator 已实现本地结构、UTC 知识截止时间、版本连续性和状态迁移门禁；`internal/research/storage/` 已实现 Claim SQL Repository 的事务内版本追加与精确版本读取。当前仍不查询 Evidence/Scope 资源、不自动推断支持度或概率；`supersedes_version` 已按冻结 DTO 增加，并在追加新版本时校验其指向当前前一版本。

## 7. 验收门禁

| 范围 | 必须覆盖 |
|---|---|
| Context | Run system_as_of 一致、required section 缺失、omitted reason、版本不可变、来源完整 |
| Evidence | 五类 evidence_type、正反中立、required/nullable、时间区间配对、confidence 方法条件 |
| PIT | available_at unknown/晚于截止、system_observed_at 边界、历史版本选择、不回退当前版本 |
| Authorization | Owner 映射、derived_from_refs 链、撤销/过期/unknown/不可用、TOCTOU |
| Fingerprint | JCS/SHA-256 复算、语义变化摘要变化、授权引用不进入内容指纹输入 |
| Event | 版本追加、合并/拆分追溯、状态事件、发布幂等、Outbox 回滚 |
| Security | 未授权正文不进入 DTO/日志/Fixture；模型输出不得生成来源授权或统计置信概率 |

Fixture/Fake Adapter 只能证明 Research 自身模型和消费边界；真实 Data/News 授权、PIT 和分页能力须分别取得生产方证据后才能升级 A2。

## 8. Persistence 边界

- Context/Event/Claim 按 ID + 版本保存不可变快照，读取使用精确版本，不提供未经契约定义的列表/搜索接口。
- Evidence 当前契约没有 Research-owned version 字段，Repository 以 evidence_id 作为不可变键；相同 ID、相同完整 payload 才允许幂等重放，任何完整 payload 变化（包括 authorization_ref 变化）均冲突拒绝，不自动生成替代 ID。语义变化如何形成新 Evidence ID/版本仍待项目契约明确；content_fingerprint 不替代上游 source_version。
- Research 拥有领域表、SQL、Repository Port 和 Migration 定义；连接池、Ping/Close、Migration 执行与历史校验复用 `internal/platform/storage`。
- Research Migration namespace 为 `research`，由组合根显式注册和执行；Research 分支不修改服务启动入口或根 Migration 注册表。
- system_as_of、system_observed_at、available_at 使用固定 30 字符 UTC RFC3339Nano（9 位小数）字符串并以 ascii_bin 排序，保证 PIT 纳秒精度；初版 Migration v1 直接使用 CHAR(30) RFC3339Nano，不做历史精度回填。created_at/updated_at 仍使用 DATETIME(6)，只作审计时间。payload_sha256 只用于完整 DTO 序列化内容的幂等校验。
- Event lifecycle_changed 的 changed_at 已按项目级 INT-CTR-001 映射决议，在 Event 新版本和 Outbox 同一事务中固化；Outbox 重试复用原值。项目级 Integration 接入审查已记录 Research 与 Researchflow 两个 Outbox→Notification Relay 已进入 Root Server 组合接入；Research 不承担 Server Relay/Worker 生命周期开发，也不重复实现 Notification 接收端。该接入记录不代表真实外部通知渠道投递通过。
- Evidence SQL Adapter 的 Append/Get 已实现；RSC-OPEN-007 仅保留授权快照续期历史的独立留档语义待决，不再阻断 Evidence 基础不可变存储。

## 9. 跨模块边界决议与待决项

### RSC-DEC-001：Quant 来源引用不等于 Research 直接依赖 Quant

项目级 Quant-Research DTO 将 Evidence.source_owner 允许值定义为 data/news/quant/research，Scenario 也允许保存 quant_evaluation_ref；但同一 DTO 文档第 4 节明确 Research → Quant 不允许，Researchflow 是唯一跨模块组合者。模块边界按以下方式统一解释：

- Research 不新增 Quant Application Port，不直接调用 Quant，不访问 Quant Repository/Domain。
- Quant 结果如需进入 Research，由 Researchflow 通过公开 Quant Port 获取并完成调用主体、资源归属和当前 AuthorizationRef 复核，再把已授权的强类型 DTO 或不透明引用传入 Research。
- Research 保存 source_owner=quant 时，保留 Quant result_ref/source_version/authorization_ref；不得把历史 allowed 当作持续读取许可，也不得自行解引用 Quant 资源。
- 未收到可验证的授权结果 DTO 时，只能保存不透明引用和来源元数据，不得复制或重解释 Quant 结果内容。
- Scenario.quant_evaluation_ref 是结果引用，不表示 Research 可以直接请求 Quant。

因此，该项是边界解释，不再作为 Research → Quant 直接调用的开放阻断；Quant 内容读取仍由 Researchflow 调用 Quant Owner 负责。

### RSC-DEC-003：Event 时间规则版本字段与 V1 共识校验已实现

旧 `RSC-OPEN-003` 关于缺少规范化规则版本字段的描述已失效。当前 ResearchEvent DTO 已有 `time_normalization_rule_version`，Domain 仅接受 `TimeNormalizationRuleVersionV1=1.0.0`；Validator 校验来源时间类型、Evidence 引用、同类时间共识和冲突时规范化值必须为空。Application 负责透传该版本，不自动推断来源时间。

尚未完成的是来源方对时间字段的真实语义/精度证据，以及真实多来源事件样本。不得把本地共识校验通过扩大为真实来源时间验收通过。

### RSC-DEC-004：Event 派生血缘字段已冻结，真实合并/拆分样本待验收

旧 `RSC-OPEN-004` 关于缺少跨 Event 来源字段的描述已失效。冻结 RSC-CTR-002 使用 `derived_from_refs` 表达精确 Event 版本血缘；Research Repository 在版本追加事务内解析引用并递归拒绝缺失节点、损坏节点和循环图，不使用 `supersedes_event_version`。

当前不把字段/存储能力等同于多来源合并/拆分业务已完成。是否增加专门用例或 Application 行为，必须先有真实调用场景、来源证据和明确的版本关系语义；不得为了功能完整而预建接口。

### RSC-DEC-005：Claim/Hypothesis supersedes_version 已落地

旧 `RSC-OPEN-005` 关于版本替代字段缺失的描述已过期。当前 Application DTO 和 Claim Domain 已包含 `supersedes_version`；版本追加时校验其指向同一 Claim 的前一版本。该字段只表达语义结论被替代，不用于普通证据追加。剩余工作是 Scope 引用解析与真实研究样本，不再新增同义字段。

### RSC-DEC-002：Lifecycle changed_at 的事务时间语义

项目级决议已明确 changed_at 是 Event 状态迁移提交时间，必须随 Event 新版本和 Outbox 在同一领域事务内持久化；Outbox 重试复用持久化值。它不是 occurred_at 或 system_observed_at。当前 Research 已实现该事务和 Outbox 机制；Root Server 的 Research Relay 已有项目级组合接入记录。真实外部通知渠道投递仍由 Notification Owner 验收，不属于 Research 剩余开发。

### RSC-DEC-006：Research → Notification Relay 已进入 Root Server 组合接入

INT-CTR-001 的 `lifecycle_changed` Payload 映射及 Research 事务 Outbox 已实现；`changed_at` 是状态迁移提交时间，随 Event 新版本和 Outbox 同事务固化，不能用 `occurred_at` 或 `system_observed_at` 代替。项目级 Integration 接入审查记录已将 Research 与 Researchflow 两个 Outbox→Notification Relay 记为已接入；Research 分支不再承担 Server Relay/Worker 生命周期开发，也不重复实现 Notification 接收端。

该状态是组合根接入记录，不自动代表真实外部通知渠道投递通过。Research 后续只处理自身 Outbox 数据/契约缺陷；Relay 生命周期、MariaDB 组合验收和渠道投递分别以 Server/Integration/Notification 的权威记录为准。

### RSC-OPEN-007：Evidence 授权快照续期与存储幂等边界

Evidence.content_fingerprint 排除 authorization_ref，而存储层 payload_sha256 覆盖完整 DTO。相同 evidence_id、相同内容但不同 AuthorizationRef 会导致内容指纹相同、完整 payload 摘要不同。当前已采用完整 DTO 不可变、冲突拒绝策略；不允许原地覆盖历史 AuthorizationRef，也不自动生成替代 evidence_id。若要持久化授权续期历史，须由项目契约定义独立授权记录或关联结构。该待决项不阻断当前 Evidence Append/Get；它仅阻断“同一 Evidence ID 下授权续期可追溯”的额外能力。


### News Evidence 版本引用门禁

NEWS-CTR-001 的 `ResearchEvidenceRecord` 必须返回不可变 `VersionID`。因此 Research 消费 News 时，`source_owner=news` 的 Evidence 必须提供 `source_version`；缺失时在 Evidence Domain Validator 拒绝，不允许以 `source_ref` 或 `source_locator` 替代版本身份。该规则是 RSC-CTR-001“上游资源可版本化时 source_version 必填”的具体化，不新增字段或跨模块契约。
