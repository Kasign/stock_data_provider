# 新闻 API 契约

> 版本：设计草案 v0.2  
> 状态：当前不建设新闻独立对外 HTTP API；本文及 OpenAPI 仅保留为未来评估草案，不进入本阶段实现

## 1. 基本原则

- 当前新闻能力作为 `stock-analysis-server:8092` 进程内的业务模块，不新增新闻专属 HTTP Handler、`/api/v1/news*` 路由或独立对外 API。
- 新闻 Application 由同进程分析流程和内部调用方直接调用，不经过内部 HTTP，不使用新闻专属 Token。
- 本文后续路由、DTO、分页及错误语义只作为未来需要 HTTP 暴露时的设计输入，不代表当前应实现或注册。
- 新闻专项契约以本目录 `openapi.yaml` 为机器可读草案。
- API 只返回系统实际掌握的数据，不伪造发布时间、正文或关联结果。
- 数据结果与质量、来源和时间元信息一起返回。
- 查询接口不得隐式触发无限制外部抓取。

## 2. 未来 HTTP 接口候选（当前不实施、不注册）

| 方法 | 路径 | 目的 |
|---|---|---|
| GET | `/api/v1/news` | 按条件查询已入库新闻 |
| POST | `/api/v1/news/search` | 搜索候选新闻；默认仅返回候选，`persist` 默认 `false`，不隐式入库 |
| GET | `/api/v1/news/{id}` | 新闻详情、内容和版本信息 |
| GET | `/api/v1/news/sources` | 来源目录和状态 |

以下路径仅供未来评估，不是当前 API 交付范围。当前不注册任何新闻专属 HTTP 路由；若未来既有 Server API 需要输出新闻数据，应优先扩展该既有 API，而不是直接启用本表路径。来源管理、主动采集和外部 URL 测试继续作为内部运维能力。

## 3. 查询语义

需要明确支持范围：
- 起止时间及其字段语义。
- canonical symbol、市场/交易所；行业分类体系、行业代码/名称快照（当前不假设统一 security_id/industry_id）。
- 来源及来源类别。
- 内容完整度、质量状态。
- 分页、排序和最大 page size。
- 发布时间可信度与数据新鲜度。
- 是否包含摘要、正文、版本和关联证据。

分页必须有稳定排序键，避免同一时间戳导致翻页重复或遗漏。

## 4. 响应信息

新闻记录应尽可能返回：
- 新闻 ID、标题、摘要、规范 URL。
- 来源标识和来源类别。
- published_at、available_at、fetched_at、system_observed_at。
- 时间可信度和缺失状态。
- 内容类型/完整度。
- 证券/行业关联及关联证据状态。
- 质量状态和降级原因。
- 内容版本标识。

具体字段名和通用响应 envelope 以 N1 对现有代码规范核实后为准。

## 5. 错误语义

区分参数错误、未找到、未授权、Provider 部分失败、限流、内部错误和数据质量降级。部分 Provider 失败时，需明确是返回部分结果还是整体失败，不得静默吞掉错误。

## 6. 安全与调用边界

- 当前新闻模块不新增 HTTP Handler、新闻专属路由或独立对外 API；不提供任意 URL 抓取 HTTP 接口。
- 新闻 Application 仅由同进程内部调用。内部调用仍须遵守来源白名单、Provider/来源授权策略、配额、任务策略、SSRF 防护和审计要求。
- 来源 CRUD、主动采集、批量刷新、外部 URL 测试和授权策略修改属于内部运维能力，不通过新增公网接口开放。
- 如果未来既有 Server API 需要对外输出新闻数据，必须由该 API 复用 Server 自身认证、授权、限流和日志边界；不得绕过 Server，也不因此创建新闻独立 API。
- 当前根 Server 静态盘点未发现认证中间件，这是主 Server 整体安全现状，不构成本新闻模块新增认证中间件或独立 Token 的理由。

---

## 7. N1 对现有 API 的核对补充（2026-11-05）

统一部署的主分析 API 当前仅注册 runs/tasks 路由，尚无新闻接口。用户已确认新闻模块随根 `stock-analysis-server:8092` 部署，但当前没有新闻独立对外计划，因此不新增新闻 HTTP 路由。现有 Server API 如未来需要新闻能力，应由对应 API 自己复用主 Server 安全边界。market-data 与 go-core 的独立认证机制不迁移到新闻模块。

现有任务事件查询使用 after_sequence 和 limit，但这不是通用新闻列表分页规范。新闻列表应在 N1 最终评审时明确稳定排序键、游标/offset 方案和最大页大小。

候选 /api/v1/news、/search、/{id}、/sources、/diagnostics 仍为设计候选，不是现有路由，也未冻结。仍需冻结 /search 的 Provider 部分失败响应语义、调用权限和配额策略；搜索默认只返回候选，`persist=false`，如需持久化必须由调用方显式声明并受来源/Provider 存储策略约束。

---

## 8. N1 HTTP 安全边界补充（2026-11-05）

根 Go `internal/platform/httpapi.Server.Handler()` 当前挂载 request-id、请求日志和 panic recoverer，但未发现 API 认证中间件。由于当前不新增新闻 HTTP 路由，新闻模块不以 Server 认证中间件作为本阶段开发前置条件。来源 CRUD、外部 URL dry-run/fetch、手动采集、批量刷新和授权策略修改保持为内部运维能力；若将来通过既有 Server API 对外提供任何管理操作，必须先由该 API 落实管理员授权。


## 9. 公共服务延期决策补充（2026-11-05）

- 当前新闻模块不提供 `/api/v1/news/diagnostics` 或其他新闻专属诊断 HTTP 路由；新闻专属诊断路由不再作为未来候选；诊断能力归公共服务统一建设。
- 当前新闻业务只通过同进程 Application 接口提供能力，不因公共诊断需求新增新闻 HTTP API。
- DB/Redis 接入和持久化查询实现同样等待公共服务；本文查询字段仅作为业务查询契约输入，不代表当前已经有持久化查询实现。


## 10. 当前同进程 Application 契约（替代未来 HTTP API 草案作为实施入口）

当前只定义内部业务用例，不注册 HTTP 路由：

| 用例 | 输入重点 | 输出重点 | 当前是否实施 |
|---|---|---|---|
| SearchCandidates | query、canonical symbol、维度、时间窗口、limit | 带 provider_code/publisher_source 的候选、Provider 部分失败 | N2 |
| CollectSource | 已登记 source/endpoint ID、时间范围、采集选项 | 解析后的业务对象、授权和质量原因 | N3 |
| NormalizeCandidate | 候选原始字段、时间文本和来源上下文 | 标准字段、时间解析证据、缺失字段 | N3 |
| IdentifyCandidate | 来源身份、原始 ID/URL、内容指纹 | 身份候选、匹配/冲突原因 | N4 |
| ResolveSecurityRelations | 新闻候选、canonical symbol/行业线索 | 关联候选和证据 | N4 |
| QueryPersistedNews | 查询过滤、稳定排序/游标 | 持久化新闻页 | N6，等待公共 DB |

当前不定义对外 HTTP DTO，不要求所有用例立即实现。业务端口按用例最小化，禁止暴露 SQL、Redis、诊断 Run/Step 等基础设施类型。

## 11. NewsEvidenceQuery 跨模块字段级契约（当前实施入口）

本节是同进程 Application Port 契约，不是 HTTP API。跨模块唯一语义和冻结状态登记于 `doc/整体任务规划/跨模块契约目录与冻结状态.md` 的 NEWS-CTR-001~003；本文只维护 News 生产方的 DTO/授权实现字段，不复制其他模块内部结构。

### 11.1 请求与响应

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

### 11.2 AuthorizationRef

AuthorizationRef 是不可变授权审计快照，不是访问令牌。字段如下：

| 字段 | JSON 类型 | 约束 |
|---|---|---|
| authorization_ref | string | 不透明稳定引用 |
| policy_owner | enum | data/news/quant/research；News 内容必须为 news |
| policy_version | string | 不可变 News Authorization Policy Registry 版本；decision_fingerprint 同时覆盖本次判定采用的 Provider Output Policy、Source Policy、Endpoint Policy 版本 |
| decision | enum | allowed/denied/conditional/unknown |
| purpose_code | enum | research_analysis/risk_review/user_display |
| content_level | enum | metadata/summary/snippet/full |
| subject_ref | string | 被授权调用主体的稳定引用，必须对应请求 caller_ref |
| resource_ref | string | 本次授权决策覆盖的新闻/来源/Endpoint 资源引用 |
| decided_at | RFC3339Nano UTC | 决策时间 |
| expires_at | RFC3339Nano UTC/null | null 表示未配置固定过期，不表示永久授权 |
| conditions | array<string> | 稳定 condition_code；无条件为空数组 |
| decision_fingerprint | string | 决策输入与策略版本指纹 |

News 为 News 来源内容签发 AuthorizationRef。公开或无限制内容也签发 allowed 快照，不允许 null/not_required 绕过授权审计。每次当前受限内容读取仍必须重新检查 Source/Endpoint/Provider 策略、用途、内容级别、撤销和过期状态；AuthorizationRef.subject_ref 对应 caller_ref，resource_ref 对应被读取资源。历史 allowed 不构成当前放行凭据。授权校验与内容读取必须使用同一策略版本快照；若读取期间策略版本发生变化，必须重新授权，否则丢弃内容并拒绝返回。当前决策 denied/unknown 或授权依赖不可用时，受限内容拒绝访问；conditional 仅在 News 验证全部 condition_code 均满足后允许，否则拒绝。

### 11.3 PIT、Cursor 与稳定排序

严格 PIT 先按 system_as_of 过滤系统已观察到的 Observation、Version、Identity、Classification、Relation，再按业务时间过滤/排序。available_at 必须满足 available_at <= system_as_of；available_at 晚于截止时间或缺证据（null/unknown）时，严格 PIT 排除；不得从 published_at/provider_time/fetched_at/system_observed_at 推导。

首期排序版本 `news-evidence-v1` 固定为：
`system_observed_at DESC, news_id ASC, version_id ASC`。

首屏创建不可变结果集快照，快照与 Cursor TTL 均为 15 分钟。Cursor 绑定 caller_ref、purpose_code、requested_content_level、规范化 filters 指纹、page_size、system_as_of、snapshot_ref、排序版本和稳定唯一键。Cursor 使用不透明 token，包含 token_version/key_id 并通过 HMAC-SHA256 防篡改；密钥轮换时旧密钥至少保留到其签发的全部 15 分钟 Cursor 过期。后续页必须读取同一 snapshot_ref。过期、签名错误、过滤指纹不一致和快照缺失分别返回稳定错误，不得静默重建快照。

### 11.4 错误码

| code | Fault class | retryable | 语义 |
|---|---|---:|---|
| news.evidence.invalid_query | validation | false | 请求结构、枚举、范围非法 |
| news.evidence.access_denied | forbidden | false | 当前调用主体无权访问所请求内容级别 |
| news.evidence.pit_unavailable | unavailable | false | 指定历史时点不可证明 |
| news.evidence.cursor_invalid | validation | false | Cursor 签名或查询指纹不匹配 |
| news.evidence.cursor_expired | conflict | false | Cursor 超过 15 分钟 TTL，调用方从首屏重查 |
| news.evidence.snapshot_unavailable | unavailable | false | 固定分页快照已不可用，不得自动创建新快照 |

底层 SQL/Redis/Platform 依赖故障使用 PLAT-CTR-001 Fault，不得映射成空 items 或业务质量 degraded。Provider 采集部分失败属于 News 采集用例，不属于该只读 Query Port 的部分成功语义。

