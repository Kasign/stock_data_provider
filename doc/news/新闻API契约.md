# News 内部 API 契约

更新时间：2026-11-25  
适用分支：`repair_all`

本文只定义 News 模块的同进程 Application 契约和跨模块 Evidence Query 契约，不定义独立 HTTP API、OpenAPI 或外部 Token。

## 1. 模块边界

- News 随根 Server 进程部署，由 Server 组合根装配。
- Research、Researchflow 等内部调用方通过 Application Port 调用 News，不经过内部 HTTP。
- 不注册 `/api/v1/news*` 路由，不建设独立 News API 服务。
- Platform 提供通用 HTTP、Storage、Fault、Diagnostics 等基础能力；News 不重复实现。
- Researchflow 负责 Run/Stage/Worker 和 StageOutput 生命周期；News 不建设独立 Scheduler、Queue、Worker 或 Orchestrator。

## 2. 当前 Application 用例

| 用例 | 调用方/入口 | 职责 |
|---|---|---|
| `IngestCollectedSource` | Server Feed 采集装配 | 将 RSS/Atom 采集结果交给统一正式入库事务 |
| `IngestProviderCandidate` | Researchflow 选中候选后的内部调用 | 将明确选中的 Provider 候选交给统一正式入库事务 |
| `NewsEvidenceQuery.GetEvidence` | Research Adapter | 按 NewsID/VersionID 查询单条证据并执行当前授权复核 |
| `NewsEvidenceQuery.ListEvidence` | Research Adapter | 固定 Snapshot 分页读取证据并逐条复核当前授权 |

Provider SearchCandidates 默认只产生 Researchflow StageOutput，不直接创建正式 News。只有研究流程明确选中/确认的候选才调用 `IngestProviderCandidate`。

## 3. 正式入库和身份契约

- 一篇文章对应一个 news_id，不按事件聚合。
- 完全相同的规范 URL 全局唯一；URL Hash 命中后必须比较 URL 原文。
- 来源原生文章 ID 只在 source_id 范围内唯一。
- 同一 URL 被不同来源发现时复用 news_id，并分别保存来源身份和 Observation。
- URL 与来源原生 ID 分别命中不同主体时返回 `identity_conflict`，不自动合并。
- URL 和来源原生 ID 都缺失时不创建正式新闻。
- 主体、来源身份、Observation 和必要内容版本在一个 MariaDB 事务中提交。
- 内容指纹变化才追加不可变版本；版本与 Observation 绑定。
- 不存在独立的 Identity History、ResearchUpdate 双阶段写入、转载关系或证券/行业关系 Application API。

## 4. NewsEvidenceQuery 跨模块字段级契约

本节是同进程 Application Port 契约，不是 HTTP API。跨模块唯一语义和冻结状态登记于 `doc/整体任务规划/跨模块契约目录与冻结状态.md` 的 NEWS-CTR-001~003；本文只维护 News 生产方的 DTO/授权实现字段，不复制其他模块内部结构。

### 4.1 请求与响应

`GetEvidence` 按 news_id 查询已持久化证据；若 version_id 省略，必须根据请求 system_as_of 选择当时可见版本，不得默认取当前最新版本。`ListEvidence` 只查询已持久化记录，不隐式触发搜索、Provider 请求或抓取。

请求字段：
- caller_ref：必填不透明主体引用，由可信 Application 调用上下文注入，不接受不可信客户端自报身份。
- purpose_code：必填枚举 research_analysis/risk_review/user_display。
- requested_content_level：必填枚举 metadata/summary/snippet/full；授权不允许时拒绝，不静默降级。
- system_as_of：必填 RFC3339Nano UTC；必须不晚于服务端可信 UTC 当前时间，并原样使用 Researchflow Run 值。
- filters：必填 typed object，仅允许 news_ids（最多 100）、source_refs（最多 50）、canonical_symbols（最多 100）、published_at_from/to、quality_statuses（valid/degraded/stale/invalid/unknown）；数组元素唯一。
- business_time_from/to：可选 nullable RFC3339Nano UTC；同时存在时 from <= to。
- page_size：必填 integer，范围 1..100，无默认值。
- cursor：首屏省略，后续页必填；不透明、防篡改，不允许调用方拼装。

响应字段：
- items：必填数组，无结果为空数组。
- next_cursor：无下一页为 null，不使用空字符串。
- snapshot_ref：必填不透明快照引用；首屏创建，后续页沿用。
- system_as_of：必填，回显请求值。
- diagnostics：必填数组，无诊断为空数组；仅放诊断引用，不放业务结论。

### 4.2 AuthorizationRef

AuthorizationRef 是不可变授权审计快照，不是访问令牌。字段如下：

| 字段 | JSON 类型 | 约束 |
|---|---|---|
| authorization_ref | string | 不透明稳定引用 |
| policy_owner | enum | data/news/quant/research；News 内容必须为 news |
| policy_version | string | 必须逐字等于 `PolicySnapshot.snapshot_ref`，表示本次决策采用的完整不可变策略组合快照；各 Provider Output/Source/Endpoint 策略的具体版本只记录在 `PolicySnapshot.policies` |
| decision | enum | allowed/denied/conditional/unknown |
| purpose_code | enum | quant_analysis/research_analysis/risk_review/user_display；NewsEvidenceQuery 首期只允许 research_analysis/risk_review/user_display |
| content_level | enum | metadata/summary/snippet/full |
| subject_ref | string | 被授权调用主体的稳定引用，必须对应请求 caller_ref |
| resource_ref | string | 唯一格式为 `news_evidence_version:{news_id}:v{version_no}`；绑定不可变 News Item Version。`version_id` 保持独立不透明 ID，不参与拼接 |
| decided_at | RFC3339Nano UTC | 决策时间 |
| expires_at | RFC3339Nano UTC/null | null 表示未配置固定过期，不表示永久授权 |
| conditions | array<string> | 稳定 condition_code；无条件为空数组 |
| decision_fingerprint | string | 决策输入与策略版本指纹 |

News 为 News 来源内容签发 AuthorizationRef。公开或无限制内容也签发 allowed 快照，不允许 null/not_required 绕过授权审计。每次当前受限内容读取仍必须重新检查 Source/Endpoint/Provider 策略、用途、内容级别、撤销和过期状态；AuthorizationRef.subject_ref 对应 caller_ref，resource_ref 必须严格等于 `news_evidence_version:{news_id}:v{version_no}`；不得使用 version_id 或可变 news_id 单独拼接。历史 allowed 不构成当前放行凭据。授权校验与内容读取必须使用同一策略版本快照；若读取期间策略版本发生变化，必须重新授权，否则丢弃内容并拒绝返回。当前决策 denied/unknown 或授权依赖不可用时，受限内容拒绝访问；conditional 仅在 News 验证全部 condition_code 均满足后允许，否则拒绝。

### 4.3 PIT、Cursor 与稳定排序

严格 PIT 先按 system_as_of 过滤系统已观察到的 Observation、Version、Identity、Classification、Relation，再按业务时间过滤/排序。available_at 必须满足 available_at <= system_as_of；available_at 晚于截止时间或缺证据（null/unknown）时，严格 PIT 排除；不得从 published_at/provider_time/fetched_at/system_observed_at 推导。

首期排序版本 `news-evidence-v1` 固定为：
`system_observed_at DESC, news_id ASC, version_id ASC`。

首屏创建不可变结果集快照，快照与 Cursor TTL 均为 15 分钟。Cursor 绑定 caller_ref、purpose_code、requested_content_level、规范化 filters 指纹、page_size、system_as_of、snapshot_ref、排序版本和稳定唯一键。Cursor 使用不透明 token，包含 token_version/key_id 并通过 HMAC-SHA256 防篡改；密钥轮换时旧密钥至少保留到其签发的全部 15 分钟 Cursor 过期。后续页必须读取同一 snapshot_ref。过期、签名错误、过滤指纹不一致和快照缺失分别返回稳定错误，不得静默重建快照。

### 4.4 错误码

| code | Fault class | retryable | 语义 |
|---|---|---:|---|
| news.evidence.invalid_query | validation | false | 请求结构、枚举、范围非法 |
| news.evidence.access_denied | forbidden | false | 当前调用主体无权访问所请求内容级别 |
| news.evidence.pit_unavailable | unavailable | false | 指定历史时点不可证明 |
| news.evidence.cursor_invalid | validation | false | Cursor 签名或查询指纹不匹配 |
| news.evidence.cursor_expired | conflict | false | Cursor 超过 15 分钟 TTL，调用方从首屏重查 |
| news.evidence.snapshot_unavailable | unavailable | false | 固定分页快照已不可用，不得自动创建新快照 |

底层 SQL/Redis/Platform 依赖故障使用 PLAT-CTR-001 Fault，不得映射成空 items 或业务质量 degraded。Provider 采集部分失败属于 News 采集用例，不属于该只读 Query Port 的部分成功语义。



### 4.5 Go Application DTO 与 JSON Schema 实现状态

跨模块语义以 `doc/整体任务规划/跨模块契约目录与冻结状态.md` 的 NEWS-CTR-001 为准。本分支已新增：

- `app.GetEvidenceRequest`、`app.ListEvidenceRequest`、`app.ResearchEvidenceRecord`、`app.ResearchEvidencePage`。
- `app.ResearchEvidenceQuery` 同进程只读 Port。
- `doc/news/schema/NEWS-CTR-001-*.schema.json` 与 `doc/news/golden/NEWS-CTR-001-*.golden.json`。

DTO、请求边界校验、Schema/Golden、HMAC Cursor Codec、PIT Repository、单条 Evidence 查询、授权快照当前复核、固定 Snapshot、分页查询及 `EvidenceQueryFacade` 均已有实现。Server 已将该同进程 Query Port 装配给 Research Adapter；这不构成独立对外 HTTP API。NEWS-CTR-001 1.0.0 保持冻结；2.0.0 已发布 Research Evidence Page Schema/Golden，规定 `available_at` 非 null 时必须是 UTC 整秒，其他 1.0.0 Schema/Golden 不变。严格 PIT 仍受合格 `available_at` 来源证据约束，当前来源没有合格字段，相关记录的 `available_at` 为 null 并被严格 PIT 排除。因此应区分“Query 机制实现/2.0.0 机器契约发布”和“NEWS-CTR-001 全量生产验收”：前者已有代码与 Schema，后者仍受来源证据、B 组严格门禁与生产验收条件限制。


### 4.6 Cursor HMAC Claims

Cursor 外部格式为 `base64url(payload).base64url(signature)`，签名为 HMAC-SHA256。Claims JSON Schema 位于 `schema/NEWS-CTR-001-cursor-claims-1.0.0.schema.json`。

News Application 已实现 `EvidenceCursorCodec`：签发与验证均绑定 caller_ref、purpose_code、requested_content_level、规范化 filters 指纹、page_size、system_as_of、snapshot_ref、`news-evidence-v1` 排序版本和最后一条排序键；TTL 固定 15 分钟。密钥由 Platform `config.Secret` 注入，轮换期间必须同时保留 active 与尚未过期 Cursor 使用的 previous keys。

Cursor Codec 不保存分页结果快照。Snapshot Store 缺失、过期或不可用时仍必须明确失败，不得重新创建快照或退回 OFFSET。


### 4.7 采集时间字段语义

Evidence DTO 的 `fetched_at` 和 `normalized_at` 来自 Observation 中的 Candidate 过程时间。News 当前不推断 `available_at`：未取得来源明确、可验证的公开可用时间证据时返回 JSON `null`。`observed_at` 对应系统观察时间，三者不可互相替代。

#### available_at 证据校验与准入

首期仅接受发布来源明确声明“首次公开可获取时间”的字段，且字段语义、Source/Endpoint 授权、具体版本归属、原始字段路径/值、明确时区、证据 URL、采集时间和解析器版本均可审计。字段必须经来源级 allowlist 与语义审核后才能标记 verified。普通 `datePublished`、`published_date`、RSS `pubDate`、Atom `published/updated`、网页 `dateModified`、HTTP `Date/Last-Modified`、Provider 索引时间及本系统采集时间均不满足该准入条件；它们不得被映射或回填为 `available_at`。

首次成功抓取时间只证明内容在该时刻已可获取，不证明首次公开时间，首期不将其当作 `available_at`。证据缺失、时区不明、字段语义不明、版本绑定不明或证据冲突时，DTO 返回 `null`，严格 PIT 必须排除该记录。证据修订必须保留历史记录和修订原因，不得静默覆盖。当前已接入来源尚无符合准入规则的字段，故当前 `available_at` 均为 `null`；严格 PIT 门禁尚未完成，不能据此宣称 NEWS-CTR-001 通过。

**3.0.0 披露日保守边界（DEC-PLAT-002-073，v1 provisional）**：NEWS-CTR-001 3.0.0 在 2.0.0 基础上以 MAJOR 增加一种可审核的降级语义——当来源仅提供"披露日"编码（如巨潮 `announcementTime`，北京时间整点毫秒时间戳）时，经来源资格记录显式声明 `disclosure_boundary` 精度与非空 `DateBoundarySemanticsRef` 后，`available_at` 可取该披露日当天最后一秒（23:59:59 声明时区，换算 UTC 整秒）。该值恒不早于真实公开时刻，严格 PIT 不会前视；代价是精度从秒级降为天级。1.0.0/2.0.0 Schema/Golden 字节不变，3.0.0 为独立新文件。通用 `ParseAvailabilityTime` 仍拒绝纯日期/毫秒串，边界候选只能由专用解析器在资格闸门内产生。字段路径必须为不含 `[]` 的稳定标识，JSONPath 数组符号在采集期规范化。此语义仅为通道确立；生产来源准入、Evidence/Decision 落库与 B 组真库门禁仍未闭环，`available_at` 生产值在无合格来源前继续保持 `null`。


### 4.8 NEWS-CTR-002 授权快照持久化边界

最终 News V1 基线包含 `news_authorization_snapshots`，Application 提供 append-only `AuthorizationSnapshotRepository`。旧 V2/V3 Migration 版本不再存在；数据库必须按最终 V1 基线重建。相同 AuthorizationRef 的相同 binding 可幂等重放；相同引用绑定不同 payload 时返回冲突。读取时重新校验原始 JSON payload SHA-256 和 binding 结构。

这里的 `payload_hash` 是数据库存储完整性校验，不等于 `decision_fingerprint` 的 GOV-CTR-006 JCS 验证。当前分支尚未接入公共 JCS Runtime Verifier，不能把结构校验或 SHA-256 原文哈希描述为 JCS 验收。

该存储只证明 News 保存了某一历史授权决策快照，不证明当前策略仍允许访问。所有受限内容读取仍必须在实际读取边界重新核验当前 Source/Endpoint/Provider 策略、撤销、过期、用途与内容级别，并确保授权检查与内容读取不存在 TOCTOU 窗口。


### 4.9 固定分页 Snapshot Store

最终 News V1 基线包含 `news_evidence_snapshots`。Snapshot 只持久化 `news_id`、`version_id`、`observation_id` 和 `system_observed_at` 排序键，不复制新闻标题、摘要、正文或内容片段。快照固定绑定 caller、filters fingerprint、page size、system_as_of、排序版本及 15 分钟 TTL。

Repository 创建时校验排序键唯一且符合 `system_observed_at DESC, news_id ASC, version_id ASC`，并限制单个快照最多 5000 条；Application 的 `PageAfter` 使用精确排序键续页，不使用 OFFSET；相同快照引用的相同 payload 可幂等重放，不同 payload 返回冲突。读取时校验 payload_hash、item_count、caller 绑定和 TTL；过期快照必须失败，不得重新生成。

该 Store 已由 `CreateEvidenceSnapshotForQuery`、`EvidenceSnapshotPageReader` 和 `ListEvidence` 接入 Query Facade。首屏创建快照前由 PIT Repository 按 `system_as_of` 选择版本，Query 侧仍须执行严格 `available_at <= system_as_of` 过滤；分页阶段逐条调用 GetEvidence，在正文读取边界重新核验当前授权。Snapshot Store 本身只固定引用和排序键，不代表数据可见或内容已授权。


### 4.10 当前 Query 与 PIT 边界

- `EvidenceQueryFacade` 组合单条 GetEvidence 与固定快照 ListEvidence；Server 将同进程 Query Port 注入 Research Adapter，不注册独立 HTTP 路由。
- GetEvidence/ListEvidence 负责版本选择、稳定排序、Snapshot/Cursor 和当前授权复核。
- 严格 PIT 仍要求合格的 available_at 来源证据；缺少证据时保持 NULL 并排除严格 PIT，不从采集时间或普通发布时间推导。
- 当前实现状态以最新代码和 CI 为准；本节不保留历史 CI Run 作为当前验收证明。

