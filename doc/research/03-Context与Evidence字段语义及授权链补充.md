# Research Context / Evidence 字段语义与授权链补充

- 适用任务：RSC-001～003
- 契约范围：RSC-CTR-001～003、DATA-CTR-007、NEWS-CTR-001～003、GOV-CTR-002/006
- 状态：Research 本地 DTO/Schema/Golden/Fixture 与专项存储验收已完成对应范围；消费方 Contract Test、生产 Owner Adapter、跨模块授权/TOCTOU 和真实数据验收仍按依赖项推进，不得将本地测试等同于生产接入通过
- 权威关系：项目级契约目录及生产方 DTO 是公共语义权威；本文件约束 Research 的消费、留存和派生行为

## 1. 模块职责和依赖边界

Research 拥有 Context、Evidence、Event、Claim、Hypothesis、Scenario 的领域语义和版本。Data 拥有行情/财务等原始事实，News 拥有新闻来源、原文、Observation 和 Evidence Query，Quant 拥有数值计算。Research 不复制上游 Repository，不依赖 Researchflow，不重算 Quant 指标，也不自行签发 Data/News/Quant 的授权。

跨模块依赖仅通过 Research 自己的 Adapter 消费公开 Application DTO。上游 API、Schema 或权限未达到证据门槛时，Research Domain 可用合成 Fixture 开发，但不得把 Fixture 登记为真实来源接入通过。

## 2. Context 字段语义

ResearchContext 至少包括 schema_version、context_id、context_version、research_question、scope、subject_refs、system_as_of、section_refs、quality_summary、included_sections、omitted_sections、builder_version、created_at。

Context 的 system_as_of 必须与所属 Researchflow Run 完全一致；不得在装配或重试时刷新。section_refs 是不可变分区/证据集合引用，不等于授权，也不代表内容当前可读取。Context 不复制来源全文；读取内容时仍由资源 Owner 执行当前授权复核。

Research 定义分区结构、质量和语义校验；Researchflow 决定本次 Run 选取哪些分区、顺序、Token Budget 和裁剪策略。裁剪必须保留来源引用、时间、反证、unknown 和风险限制。required 分区缺失时 Context 不得标 valid；每个 omitted section 必须有稳定 reason_code。

## 3. Evidence 字段和认识论约束

Evidence DTO 的规范字段如下。RSC-CTR-001 的 Schema `$id` 标识 1.0.0 版本，JSON payload 不包含 schema_version。旧文档中的 source_type、observed_at、as_of、content_hash 仅为早期概念草稿，不作为跨模块 JSON 字段：

| 字段 | 必需/nullable | 语义 |
|---|---|---|
| evidence_id | 必填 | Research 内不透明 ID；契约版本由 JSON Schema `$id` 标识，payload 不增加 schema_version |
| evidence_type | 必填 | fact/reported_fact/inference/hypothesis/unknown |
| source_owner | 必填 | data/news/quant/research |
| source_ref | 必填 | 上游资源或来源对象引用 |
| source_version | 条件必填 | 上游资源可版本化时必须提供 |
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

当前 `internal/research/event/` 已实现 Event/SourceTime 领域校验、版本追加校验和 MVP 生命周期转换；`internal/research/storage/` 已实现 Event SQL Repository，版本追加与生命周期 Outbox 在同一事务提交，并支持 Outbox 领取、租约、重试、确认和拒绝。Research 仍不查询 News Evidence Repository，也不执行多来源事实归并。非 null 的 occurred_at/published_at/available_at 至少要求存在相同 time_type 的 SourceTime；这只是来源类型存在性检查，不等于已验证规范化规则版本。

生命周期：detected/consolidating/active/materializing/fading/invalidated/closed。只有状态真正迁移并持久化后才按 INT-CTR-001 发布 lifecycle_changed；Evidence 追加、来源补录或描述修订但状态未变，不发布。事件通过 Research 本地事务 Outbox 发布，不直接调用 Notification 内部接口。

## 6.1 RSC-004 Data 实体引用依赖复核

2026-11-09 只读核对 Data 分支 `feat/full-go-data-providers-api-switch`，HEAD 为 `7b3aa2a099b29b841caf93b0890985ecbc002cf4`。Market Data 已迁入根 Go Module（`internal/marketdata` 存在，`services/market-data/go.mod` 已不存在）；代码树仍未发现 SecurityRef、IndustryRef、SecurityMaster、IndustryClassification 或独立 GetSecurity/ListIndustries 查询端口。当前 `SecurityTradingStatus` 是具体状态数据，不是稳定证券主数据身份；旧 `internal/analysis/app/ports.go` 泛化 UniverseResolver 也不能视为 Data 所有的稳定实体契约。故 RSC-004 暂不开发 Adapter，也不在 Research 中复制 Security/Industry 模型。待 Data 提供稳定引用、历史版本/知识时间语义和公开 Application Port 后再继续。

## 6.2 RSC-006 Scenario 契约准入检查

当前仍受本文件 `RSC-OPEN-006` 阻断：Scenario 主体字段及 QuantResultRef 元数据已明确，但 `ScenarioCondition.expected_value` 尚无按 condition_type、metric_ref、unit 定义的类型映射或 JSON Schema，ConditionGroup（all/any）也尚无与 `conditions[]` 对应的机器可验证联合结构。Research 不直接调用 Quant；Quant Evaluation 由 Researchflow 获取并完成当前授权复核后传入。契约补齐前不使用 `any`、任意 map 或 JSON RawMessage 代替强类型定义。

## 6.3 Claim / Hypothesis 领域实现边界

当前 `internal/research/claim/` 以 Claim DTO 表达 reported_fact、inference 和 hypothesis 三类认识论状态；Hypothesis 必须提供 validation_method、expected_observation 和非空 invalidation_conditions，支持与反证引用不能重叠。Claim Validator 已实现本地结构、UTC 知识截止时间、版本连续性和状态迁移门禁；`internal/research/storage/` 已实现 Claim SQL Repository 的事务内版本追加与精确版本读取。当前仍不查询 Evidence/Scope 资源、不自动推断支持度或概率；`supersedes_version` 契约缺口仍未解决。

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
- system_as_of、system_observed_at、available_at 使用固定 30 字符 UTC RFC3339Nano（9 位小数）字符串并以 ascii_bin 排序，保证 PIT 纳秒精度；Migration v2 将 v1 DATETIME(6) 数据扩展为 6 位微秒 + 3 位零纳秒。created_at/updated_at 仍使用 DATETIME(6)，只作审计时间。payload_sha256 只用于完整 DTO 序列化内容的幂等校验。
- Event lifecycle_changed 的 changed_at 已按项目级 INT-CTR-001 映射决议，在 Event 新版本和 Outbox 同一事务中固化；Outbox 重试复用原值。Server → Notification PublishEvent Relay 接线仍待组合根集成。
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

### RSC-OPEN-003：Event 规范化时间缺少规则版本字段

Quant-Research DTO 要求非 null 的 occurred_at/published_at/available_at 规范化值可追溯到来源 Evidence 和采用的规则版本，但 ResearchEvent DTO 目前没有 normalization_rule_version 或等价的规则引用字段。当前 Domain 只能检查相应 time_type 的 SourceTime 存在，不能证明归一规则版本。未完成项目级字段/元数据决议前，不将规范化时间完整性标记通过；多来源冲突时保留 source_times 并将规范化值置 null。

### RSC-OPEN-004：Event 合并/拆分的来源关系字段缺失

Quant-Research DTO 第 3.3 节的 ResearchEvent 字段表没有 `source_refs` 和 `derived_from_refs`，但第 9.9 节要求 Event 合并/拆分时记录这两类引用。现有 `supersedes_event_version` 只能表达同一 Event 内的版本替代，不能表达多个 Event 合并为一个或一个 Event 拆分为多个。

在项目级契约明确字段、方向、基数和历史查询语义前，Research 只实现单 Event 版本追加与状态转换，不新增本地私有字段，不实现跨 Event 合并/拆分写入。

### RSC-OPEN-005：Claim/Hypothesis 版本替代字段缺失

Quant-Research DTO 第 3.4 节定义 claim_version，但没有 `supersedes_version`；第 9.9 节又要求修正声明 supersedes_version。当前实现只能验证 claim_version 从 1 起、相邻版本号连续和 system_as_of 不回退，不能验证显式替代关系。项目级契约补齐前不新增私有字段，也不把版本追加验收标记为完整。

### RSC-DEC-002：Lifecycle changed_at 的事务时间语义

项目级决议已明确 changed_at 是 Event 状态迁移提交时间，必须随 Event 新版本和 Outbox 在同一领域事务内持久化；Outbox 重试复用持久化值。它不是 occurred_at 或 system_observed_at。当前 Research 已实现该事务和 Outbox 机制，专用 MariaDB 验收已通过；剩余是 Server → Notification Relay 接线。

### RSC-OPEN-002：Server → Notification Relay 接线待完成

INT-CTR-001 已接受 `lifecycle_changed` Payload 映射：`source_event_ref`、`research_event_ref`、`event_version`、`previous_state`、`current_state`、`reason_code`、`system_as_of`、`changed_at`。`changed_at` 是状态迁移提交时间，不得用 `occurred_at` 或 `system_observed_at` 代替。Research Migration v4/v5 提供事务 Outbox 和领取/租约/重试/确认/拒绝；CreateEvent 首次创建与 UpdateEvent 状态迁移均在同一事务写入 Outbox；Schema/Golden 自动 Contract Test 与专用 MariaDB 验收已通过。当前未完成的是 Server 组合根消费 Research Outbox 并调用 Notification `PublishEvent` 的 Relay 接线；这不是 DTO 映射或 Research Outbox 本身的缺口。



### RSC-OPEN-007：Evidence 授权快照续期与存储幂等边界

Evidence.content_fingerprint 排除 authorization_ref，而存储层 payload_sha256 覆盖完整 DTO。相同 evidence_id、相同内容但不同 AuthorizationRef 会导致内容指纹相同、完整 payload 摘要不同。当前已采用完整 DTO 不可变、冲突拒绝策略；不允许原地覆盖历史 AuthorizationRef，也不自动生成替代 evidence_id。若要持久化授权续期历史，须由项目契约定义独立授权记录或关联结构。该待决项不阻断当前 Evidence Append/Get；它仅阻断“同一 Evidence ID 下授权续期可追溯”的额外能力。
