# Research Context / Evidence 字段语义与授权链补充

- 适用任务：RSC-001～003
- 契约范围：RSC-CTR-001～003、DATA-CTR-007、NEWS-CTR-001～003、GOV-CTR-002/006
- 状态：模块实现输入基线；Schema/Golden/Contract Test 仍按项目契约目录逐项验收
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
- Evidence 的 system_as_of 不得晚于所属 Run；语义变化必须追加新版本。
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

当前 `internal/research/event/` 已实现 Event/SourceTime 领域结构、字段和时间校验、SourceTime 到 evidence_refs 的引用约束、版本追加校验及 MVP 生命周期转换。它不查询 Evidence Repository、不执行多来源事实归并、不持久化版本，也不发布 Outbox。非 null 的 occurred_at/published_at/available_at 至少要求存在相同 time_type 的 SourceTime；这只是来源类型存在性检查，不等于已验证规范化规则版本。

生命周期：detected/consolidating/active/materializing/fading/invalidated/closed。只有状态真正迁移并持久化后才按 INT-CTR-001 发布 lifecycle_changed；Evidence 追加、来源补录或描述修订但状态未变，不发布。事件通过 Research 本地事务 Outbox 发布，不直接调用 Notification 内部接口。

## 6.1 RSC-004 Data 实体引用依赖复核

2026-11-06 只读核对 Data 分支 `feat/full-go-data-providers-api-switch`，HEAD 为 `3a82139deb6e343fb651abb07512e909206f1b65`。代码树未发现 SecurityRef、IndustryRef、SecurityMaster、IndustryClassification 或独立 GetSecurity/ListIndustries 查询端口；旧 `internal/analysis/app/ports.go` 仅有泛化 UniverseResolver，不能视为 Data 所有的稳定实体契约。故 RSC-004 暂不开发 Adapter，也不在 Research 中复制 Security/Industry 模型。待 Data 提供稳定引用、历史版本/知识时间语义和公开 Application Port 后再继续。

## 6.2 Claim / Hypothesis 领域实现边界

当前 `internal/research/claim/` 以单一 Claim DTO 表达 reported_fact、inference 和 hypothesis 三类认识论状态。Hypothesis 必须提供 validation_method、expected_observation 和非空 invalidation_conditions；支持与反证引用不能重叠。当前只验证本地结构、UTC 知识截止时间和版本号连续性，不查询 Evidence/Scope 资源，不持久化版本，也不推断支持度或概率。状态值按项目契约枚举校验；状态迁移和跨版本替代关系尚未接入。

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

## 8. 跨模块边界决议与待决项

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

### RSC-OPEN-002：ResearchEvent.changed_at 与 INT-CTR-001 时间映射缺字段

项目级 ResearchEvent DTO（Quant-Research 公开 DTO 字段级契约第 3.3 节）包含 occurred_at、published_at、available_at、system_observed_at、system_as_of，但没有 changed_at。与此同时，INT-CTR-001 的 research.event.lifecycle_changed Payload 要求 changed_at，且 Envelope.occurred_at 映射为 ResearchEvent.changed_at。

这是生产 DTO 与通知事件映射的真实字段缺口，不能擅自把 occurred_at 或 system_observed_at 当作 changed_at：
- 需要项目级决策：在 ResearchEvent 公开 DTO 增加明确的状态迁移提交时间 changed_at，或在 INT-CTR-001 中明确由 Research Outbox 生成并持久化的 transition_committed_at 作为事件时间。
- 该时间必须在状态迁移事务中固定；Outbox 重试不得刷新；同一 event_version 的时间不得变化。
- 决议完成前，Research 可以继续实现 Event 版本和状态迁移，但不得伪造 lifecycle_changed 的 changed_at/occurred_at，也不得将该事件生产接入标记为通过。

该问题只阻断 ResearchEvent lifecycle_changed 的通知生产映射，不阻断 Research Event Domain、版本追加和状态机独立测试。
