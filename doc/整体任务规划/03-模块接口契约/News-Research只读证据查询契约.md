# News → Research 只读证据查询契约

- 所属治理任务：GOV-004
- 状态：目标契约设计，尚未实现、尚未冻结。
- 规划归属：`feat/design_doc`；News 业务实现归 `feat/research-news-ingestion`，Research 消费侧归未来 `feat/research`。
- 设计原则：News 是来源、采集、身份、版本、授权和内容的权威所有者；Research 只接收被授权的稳定 DTO，不访问 News Repository、SQL 或私有 Domain。

## 1. 当前实现核查与边界

本轮通过 `feat/research-news-ingestion` 分支读取以下文件：
- `internal/news/app/ports.go`
- `internal/news/app/search.go`
- `internal/news/app/ingest.go`
- `internal/news/app/research_update.go`
- `internal/news/domain/observation.go`
- `internal/news/domain/version_record.go`
- `internal/news/domain/policy.go`
- `internal/news/domain/identity.go`
- `internal/news/domain/relation.go`

分支相对既有核查点 `ea697fe437d44afffbd60874ad144adc4313e469` ahead 1 个提交，变化在新闻关系文档、领域类型和测试；本轮读取的 Application 端口仍未提供 Research 只读查询。

当前已存在：
- SearchCandidates：聚合 Provider 搜索结果，不持久化、不缓存。
- IngestCandidate：通过 IdentityRepository 事务端口完成身份解析和 Observation 幂等追加。
- ApplyResearchUpdate：通过 ResearchUpdateRepository 事务端口追加内容版本和关系断言。
- NewsItemVersion：包含版本号、指纹、标题、发布者、published_at、内容层、observed_at、observation_id。
- Observation：包含来源、Provider、候选、observed_at、身份状态、NewsID。
- SourcePolicy / EndpointPolicy / ProviderOutputPolicy：分开表达来源、入口和 Provider 输出权限；PolicyDecision 目前只包含 Allowed、ReasonCode、MaxRetention。
- RelationAssertion / RelationEvidence：追加式关系断言和证据定位。

当前缺失：
- ResearchEvidenceQuery 只读 Facade、请求/响应 DTO 和查询用例；
- 可按 system_as_of 过滤的持久化查询；
- SecurityIdentity / IndustryClassification / Relation 的知识时间查询闭环；
- 不可变授权决策快照或可追溯的策略版本引用；
- 稳定分页快照；
- 有明确来源证据的 available_at；
- 独立 fetched_at、normalized_at；
- 稳定 content_ref / 授权内容读取端口；
- 真实 Repository / Migration 持久化 Adapter。

以上缺失不得在其他模块文档中描述为已实现。

## 2. 模块边界

News Application 对外暴露只读 Facade：

- Research 只依赖 News Application 的公开接口和 DTO。
- News Facade 内部调用 News 自己的 Repository/Query Adapter、身份解析器、授权策略和内容裁剪器。
- Research 不得 import `internal/news/domain`、`internal/news/adapters/**`，不得直接访问 News 表或自行拼接 SQL。
- Researchflow 负责将 Run 的 `system_as_of`、调用用途和所需内容级别传入 News Facade。
- News 不负责创建 Research Evidence、Event、Claim、Hypothesis 或 Scenario；Research 把 News DTO 转换为自己的 Evidence Domain。
- 此 Facade 是同一根 Go Module 内的 Application 调用，不新增 News HTTP 服务、端口或服务间 Token。
- Facade 不是通用搜索/抓取 API，不接受任意 URL 作为抓取目标。

## 3. 最小公开接口

建议接口：

```go
type ResearchEvidenceQuery interface {
    GetEvidence(ctx context.Context, request GetEvidenceRequest) (ResearchEvidenceRecord, error)
    ListEvidence(ctx context.Context, request ListEvidenceRequest) (ResearchEvidencePage, error)
}
```

接口应由 News Application 提供具体实现，Repository 仅是内部依赖。接口和 DTO 放在 `internal/news/app` 的公开 Application 契约中，不新建跨领域万能 common 包。

### 3.1 GetEvidenceRequest

建议字段：

| 字段 | 必需 | 语义 |
|---|---|---|
| news_id | 是 | News 内部不透明主体 ID |
| version_id | 否 | 指定不可变版本；指定后仍需检查该版本在 system_as_of 是否可见 |
| system_as_of | 条件必需 | 未指定 version_id 时必需；历史查询统一知识截止时间 |
| purpose | 是 | 受控用途枚举，如 internal_personal_research / internal_business |
| requested_content_level | 是 | metadata / title / snippet / abstract / full_text 中的受控级别 |
| caller_ref | 是 | 内部可信调用方标识，由组合根注入，不接受用户任意伪造 |
| include_relations | 否 | 是否返回同一知识时点可见的主体关系引用 |

规则：
- 未指定 version_id 时，必须给出 system_as_of；News 选择在该时点系统已知、且通过授权过滤的最新可用版本。
- 指定 version_id 时，必须验证该版本的 observed_at 不晚于 system_as_of；若调用方未提供 system_as_of，可仅用于明确版本的当前授权查询，不可用于历史回测或声称 PIT。
- 不得以 `published_at` 替代 `system_as_of`。
- 不得将当前最新版本回填到历史时点。
- 若主体存在身份冲突、版本冲突或知识时间不可确定，返回明确状态/错误，不得猜测。

### 3.2 ListEvidenceRequest

建议字段：

| 字段 | 必需 | 语义 |
|---|---|---|
| system_as_of | 是 | 整个分页会话固定的系统知识截止时间 |
| published_from / published_to | 否 | 来源声明发布时间筛选，不是系统知识时间 |
| observed_from / observed_to | 否 | 系统观察时间筛选；不得与发布时间混淆 |
| source_refs | 否 | 已知 News Source ID 列表 |
| subject_refs | 否 | 已解析的 Security / Industry 不透明引用 |
| identity_statuses | 否 | resolved / ambiguous / unresolved 等明确状态过滤 |
| quality_statuses | 否 | News 业务质量状态过滤 |
| purpose | 是 | 受控调用用途 |
| requested_content_level | 是 | 内容级别 |
| caller_ref | 是 | 内部可信调用方标识 |
| page_size | 是 | 有硬上限的分页大小 |
| cursor | 否 | News 生成的不透明 continuation token |

限制：
- 不接受任意 URL、任意 SQL 条件、任意字段名或任意排序表达式。
- 查询时间窗必须有限制；page_size 必须有固定上限。
- 过滤条件的组合和排序由 News 定义并版本化，不能由调用方注入数据库排序。
- subject_refs 必须是 News 已知的稳定身份/关系引用，不接受只有证券代码、没有市场/交易所上下文的模糊标识。
- 不允许调用方要求绕过授权或访问已撤销内容。

## 4. ResearchEvidenceRecord：公开返回 DTO

建议返回结构（字段名为目标契约，不代表当前代码已有）：

```go
type ResearchEvidenceRecord struct {
    NewsID                  string
    ObservationID           string
    VersionID               string
    VersionNo               int
    Fingerprint             string

    SourceRef               string
    ProviderRef             string
    IdentityStatus          string
    BusinessQualityStatus   string

    Title                   *string
    PublisherName           *string
    PublisherDomain         *string
    PublishedAt             *time.Time
    PublishedAtRaw          *string
    AvailableAt             *time.Time

    ObservedAt              time.Time
    FetchedAt               *time.Time
    NormalizedAt            *time.Time

    ContentLevel            string
    ContentRef              *string
    ContentExcerpt          *string

    AuthorizationRef        string
    AuthorizationStatus     string
    SystemAsOf              time.Time

    RelationRefs            []string
}
```

语义要求：
- `NewsID`、`ObservationID`、`VersionID` 是不透明字符串；不暴露数据库主键类型和内部聚合。
- `VersionNo` 与 `Fingerprint` 用于版本识别和内容一致性验证，不替代 VersionID。
- `SourceRef`、`ProviderRef` 是不透明引用；不能把 Provider 输出权利等同于 Publisher 来源授权。
- `IdentityStatus` 和 `BusinessQualityStatus` 是不同维度，不合并成单一健康状态。
- `PublishedAt` 是来源声明的发布时间；`AvailableAt` 只有可验证来源证据时才返回；`ObservedAt` 是系统观察时间。
- `FetchedAt`、`NormalizedAt` 当前模型尚未分别提供，未来若新增，必须按实际采集/标准化边界写入，不能从 Observation.ObservedAt 推导。
- `ContentLevel` 必须与本次授权结果一致。没有获准的摘要/正文时，相关字段缺省，不得用搜索摘要冒充发布者正文。
- `ContentRef` 只有在 News 已建立稳定、授权可控的内容引用/读取机制后才可提供；不能先返回一个无法授权读取或无法稳定定位的伪引用。
- `AuthorizationRef` 必须指向不可变授权决策快照，而不是单一 bool 或当前策略配置 ID。
- `SystemAsOf` 记录本次查询采用的系统知识截止时间。

### 4.1 DTO 的内容分层

建议内容返回分为：
- metadata：NewsID、Source/Provider 引用、身份/质量、时间和授权引用；
- title：metadata + 授权标题；
- snippet：title + 获准的搜索片段；
- abstract：title + 获准摘要；
- full_text：只有 Source、Endpoint、Provider Output Policy 对当前 purpose、内容类型、存储/使用方式全部授权时才允许。

内部个人研究不自动意味着全文复制权；模型输入、持久化、展示和再分发是不同用途。Research 默认只持久化引用、指纹、必要短摘要和授权依据，不复制未获授权全文。

## 5. 授权决策必须可追溯

当前 PolicyDecision 只有 Allowed、ReasonCode、MaxRetention，SourcePolicy/EndpointPolicy/ProviderOutputPolicy 也没有独立不可变策略版本引用。仅把这些结构体在查询时重新计算一次，不足以证明历史 Run 当时实际获得了什么授权。

建议 News 增加不可变授权决策记录：

```go
type AuthorizationDecisionRecord struct {
    ID                    string
    SourcePolicyRef       string
    EndpointPolicyRef     string
    ProviderPolicyRef     string

    Purpose               string
    ContentLevel          string
    StorageMode           string
    Decision              string
    ReasonCode            string
    MaxRetention          time.Duration

    EvaluatedAt           time.Time
    PolicySnapshotHash    string
}
```

要求：
- Source / Endpoint / Provider 权限分别评估，再取交集；任何一方未配置、pending、revoked、unknown 或不满足用途/内容/留存条件时，不能默认批准。
- 记录必须引用当时的不可变策略快照或版本。若只保存策略 ID，策略记录必须不可变；修改策略应追加新版本。
- 记录 Decision、ReasonCode、Purpose、ContentLevel、StorageMode、MaxRetention、EvaluatedAt 和策略快照指纹。
- 查询时重新检查当前是否允许继续访问/展示内容；历史授权记录用于审计和解释，不代表当前撤销后仍可继续读取全文。
- Research DTO 只返回 AuthorizationRef 和安全状态，不暴露完整策略内部规则、敏感许可材料或 Provider 凭据。
- 授权被撤销后，Research 历史事实引用可以保留必要元数据和审计引用，但内容读取必须重新授权；不得因为历史 Run 曾获准而绕过当前访问控制。

## 6. 双时间轴与 PIT 查询

必须区分业务有效/来源时间与系统知识时间。

### 6.1 时间字段语义

| 字段 | 时间轴 | 含义 | 不能替代 |
|---|---|---|---|
| published_at | 来源/业务时间 | 来源声明的发布时间 | available_at、observed_at |
| available_at | 来源可获得时间 | 有证据证明外部可获得的时间 | published_at、fetched_at |
| valid_from / valid_to | 业务有效时间 | 证券/行业/关系在现实业务中的有效区间 | system_as_of |
| observed_at | 系统知识时间 | 本系统首次/本次观察到记录的时间 | available_at、fetched_at |
| fetched_at | 采集执行时间 | 实际抓取/下载完成时间 | observed_at |
| normalized_at | 标准化时间 | 解析/规范化完成时间 | fetched_at |
| system_as_of | 查询知识截止时间 | 本次 Run 允许看到的系统知识上限 | 交易日或发布时间 |

如果没有可验证的 available_at，只能将其视为 unknown；系统可以用 observed_at 表达“本系统不晚于何时已知”，但不能将 observed_at 写进 available_at。

### 6.2 历史查询顺序

对于 `system_as_of = T` 的历史研究查询，News Query Facade 必须先执行知识时间过滤，再做业务有效时间解析：

1. 过滤 Observation：`observed_at <= T`。
2. 过滤 NewsItemVersion：`observed_at <= T`，并确保关联 Observation 在 T 时可见。
3. 过滤 SecurityIdentityRecord / IndustryClassificationRecord：`observed_at <= T`。
4. 过滤 RelationAssertion：`observed_at <= T`，并仅在 T 时已知的断言集合上解析 supersedes 链。
5. 在过滤后的记录上，再按 published_at、valid_from/valid_to 等业务时间选择适用事实。
6. 在知识时间与业务时间均符合后，执行当前授权策略和历史授权记录检查。
7. 输出固定版本 ID、Observation ID、关系断言引用、SystemAsOf 和授权引用。

禁止：
- 先取当前最新关系，再用 valid_from/valid_to 伪装历史状态；
- 先取当前身份映射，再将证券代码历史变更视为当时已知；
- 用当前新闻版本覆盖历史 Run 当时可见的旧版本；
- 用发布时间早于 T 推导系统在 T 前已知；
- 用今天的抓取/采集结果填充过去的 system_as_of。

## 7. Security / Industry / Relation 历史解析

当前新增的 SecurityIdentityRecord / IndustryClassificationRecord 含 ObservedAt，但 Resolver 只按业务有效时间解析；RelationAssertion 含 ObservedAt，ResolveCurrentRelations 是当前状态解析，不是历史知识时点查询。

因此必须提供显式知识时间查询能力。推荐在 News Query Adapter 中统一完成，而不是让 Research 自己拼接：
- `ResolveSecurityIdentityAt(ctx, identity, validAt, systemAsOf)`
- `ResolveIndustryClassificationAt(ctx, classification, validAt, systemAsOf)`
- `ResolveRelationsAt(ctx, newsID, validAt, systemAsOf)`

若将参数收敛为 QueryContext，必须仍能独立表达 `valid_at` 和 `system_as_of`，不允许一个 as_of 同时兼任两者。

同一证券跨代码变化的身份链路必须基于稳定 IdentityNamespace + IdentityValue 或显式追加式映射维护；不能仅凭 Market + Symbol 当前值推断历史同一性。行业分类必须保留 ClassificationSystem + ClassificationVersion；跨版本映射需显式记录，不能按相同代码自动映射。

## 8. 稳定分页与快照一致性

ListEvidence 的分页必须保证整个分页会话读取同一个知识快照。

推荐方案：
- 第一次请求确定 `system_as_of`、固定过滤条件、排序规则版本和快照标识。
- Cursor 是不透明 token，包含或引用 snapshot_id、过滤条件摘要、system_as_of、排序版本、最后一条记录的稳定排序键和过期时间。
- Cursor 必须防篡改；不能直接信任客户端传回的 offset 或 SQL 条件。
- 后续页必须验证 cursor 与请求过滤条件一致；不一致返回明确 invalid_cursor。
- 排序必须有稳定唯一 tie-breaker，例如 observed_at + news_id + version_id + observation_id，具体顺序由 News 固定。
- 数据库实现可采用一致性事务快照、版本化水位线或物化查询结果；不能仅依赖 OFFSET，在并发插入时导致重复/漏项。
- 分页期间新增的版本/关系不进入当前分页快照；下一次新查询才可见。
- Snapshot/Cursor 有最大生命周期与资源上限，过期后明确要求重新查询，不静默从第一页续接。
- 结果必须返回 snapshot_id 或可审计的 snapshot_ref，Research Run 记录它以便复现。

## 9. 错误和状态语义

建议区分以下结果，不以空数组代表所有异常：

- not_found：主体不存在或在知识截止时间不可见；对外是否区分需评估信息泄露风险。
- version_not_visible：指定版本晚于 system_as_of。
- identity_ambiguous：身份解析存在多个候选。
- authorization_denied：授权不允许本次用途/内容级别。
- authorization_unknown：授权记录或策略证据不足。
- content_unavailable：元数据可见，但所需内容没有被合法保存或不可读取。
- snapshot_expired：分页快照过期。
- invalid_cursor：Cursor 与过滤条件不匹配或签名无效。
- invalid_time_range：时间窗非法或超过最大范围。
- storage_unavailable：查询依赖的持久化存储不可用。
- internal_error：安全摘要错误；底层 SQL/Provider/Secret 不向 Research 暴露。

结果状态和错误必须保留 Request ID，并映射 Platform Fault；但 News 领域质量状态与 Platform Diagnostics 状态仍分开。

## 10. Repository / Migration 最小所有权

News 自己拥有：
- News Item / Identity Credential；
- Observation；
- News Item Version；
- Source / Endpoint / Provider Policy 及不可变策略快照；
- Authorization Decision；
- Security Identity / Industry Classification；
- Relation Assertion / Relation Evidence；
- 内容载荷元数据和授权内容存储引用；
- 查询快照/分页游标所需的短期状态（可用 Redis 辅助，但不得将 Redis 作为唯一事实存储）。

Platform 只提供 SQL/Redis 连接生命周期、Ping 和 Migration Runner。News 自己提供 Repository、SQL、Migration、事务和保留策略。Research 自己拥有 Evidence/Claim/Hypothesis/Scenario 表，不复用 News Repository。

在 Platform Storage 完成真实 MariaDB 集成和迁移故障演练之前，News N6 持久化实现不得标记为生产验收完成。

## 11. 分阶段实施建议

1. **N6 Storage 基础**：待 Platform Storage 真实 MariaDB 验收后，News 实现自有 Repository/Migration；先覆盖 Identity、Observation、Version、Relation。
2. **双时间查询**：Repository 增加 system_as_of 过滤，Security/Industry/Relation Resolver 明确 valid_at + system_as_of。
3. **授权版本化**：建立不可变策略快照和 AuthorizationDecisionRecord，确保查询返回可追溯 AuthorizationRef。
4. **只读 Facade**：实现 GetEvidence/ListEvidence DTO、用途/内容级别校验、内容裁剪和稳定分页。
5. **内容引用**：仅在许可允许时实现 content_ref 和受控读取；补 available_at/fetched_at/normalized_at 的来源与采集证据。
6. **Research 接入**：Research 只依赖 Facade，把 DTO 转换为自己的 Evidence；不跨模块访问 News 私有包。
7. **历史复现验收**：模拟版本更新、授权撤销、身份变更、关系 supersede 和分页并发，验证历史 Run 不受未来知识污染。

## 12. 验收清单

- [ ] Research 只依赖 News Application Facade 和公开 DTO。
- [ ] Facade 不接受任意 URL/SQL/动态排序作为抓取或查询指令。
- [ ] GetEvidence/ListEvidence 明确区分 news_id、version_id、system_as_of 和业务时间。
- [ ] 指定版本晚于 system_as_of 时拒绝返回。
- [ ] 不指定版本时，选择 system_as_of 时点已知的最新版本。
- [ ] Observation、Version、Identity、Classification、Relation 均执行知识时间过滤。
- [ ] 关系 supersedes 链在历史知识快照内解析，而非调用 ResolveCurrentRelations 代替。
- [ ] available_at 仅来自可验证证据；没有证据时为 unknown/缺省。
- [ ] fetched_at、normalized_at 不从 observed_at 推导。
- [ ] Source / Endpoint / Provider 权限取交集；授权决策有不可变 AuthorizationRef。
- [ ] 历史授权审计与当前内容访问授权分离；撤销后不得凭旧 Run 授权继续读取全文。
- [ ] 内容级别、用途、留存、展示和模型输入分别校验。
- [ ] ListEvidence 分页绑定同一快照和过滤条件，支持稳定排序、游标校验和过期处理。
- [ ] Repository/Migration 由 News 拥有，Platform 不承载领域 SQL。
- [ ] 覆盖授权拒绝/撤销、版本更新、身份冲突、历史时点、关系修正、分页并发和存储故障测试。
- [ ] 真实 MariaDB 集成和 Migration 故障演练通过后，才标记持久化生产验收完成。

## 13. 当前结论

当前 News 已有 Candidate、Observation、Version、Policy、Identity、Relation 和写入事务端口，但没有 Research 只读 Query Facade、持久化实现和完整 system_as_of 查询。该文档定义后续目标契约，不代表上述能力已经实现。GOV-004 在 News 方向仍未完成，不能冻结。


## 14. 最新 News 分支源码复核（2026-11-05）

本节基于 News 分支最新 HEAD `cefbbbb10ef418f8bca1d2ff3d6a1fef8ce203b6`（`test(news): 修复证据归属校验格式并覆盖跨新闻版本`，2026-11-05 10:26 UTC）复核。相较上一核查点 `ea697fe437d44afffbd60874ad144adc4313e469`，compare 为 ahead 4 / behind 0，变化涉及新闻任务文档、关系更新事务和测试、关系领域校验及证券身份测试；没有新增 Research 只读查询 Facade 的证据。

### 14.1 已核实的代码能力

- `Observation` 与 `NewsItemVersion` 均有 `ObservedAt`；版本同时关联 `ObservationID`。
- `SecurityIdentityRecord`、`IndustryClassificationRecord` 和 `RelationAssertion` 均保存 `ObservedAt`。
- `ResolveSecurityIdentity` 仅接收 market、symbol、业务 `asOf`，只按 `ValidFrom/ValidTo` 匹配；未接收 `system_as_of`，也不负责过滤未来才被系统观察到的记录。
- `ResolveIndustryClassification` 仅按分类体系、分类版本、行业代码和业务有效时间解析；同样没有知识时间过滤。
- `ResolveCurrentRelations` 对传入的断言集合计算当前 supersedes 链和当前已确认关系；没有历史知识时点参数。
- `ApplyResearchUpdate` 通过事务端口追加版本、关系和证据，并验证 Observation/Version 存在且归属同一 NewsID；它是写入用例，不是只读 Query Facade。
- 最新提交新增跨 News Item 的 SourceVersion 证据归属拒绝测试，并验证事务回滚；这是有效的写入完整性改进，但不覆盖 PIT 查询。

### 14.2 授权与查询能力差距

- `PolicyDecision` 当前只有 `Allowed`、`ReasonCode`、`MaxRetention`，没有不可变决策 ID、策略版本引用、策略快照哈希或评估时间。
- Source/Endpoint/Provider Policy 有有效期、证据引用及权限集合，但现有结构不能单独证明某次历史研究实际使用的策略版本和决策结果。
- 本轮未发现 `ResearchEvidenceQuery`、`GetEvidence`、`ListEvidence` 只读 Facade 或对应 DTO。
- 未发现将 Observation、Version、Identity、Classification、Relation 按统一 `system_as_of` 过滤的查询闭环。
- 未发现绑定固定 snapshot、过滤条件、排序版本和稳定唯一键的分页 Cursor 实现。
- News README 仍将身份识别/版本/关系后续阶段、N6 Repository/Migration 列为待完成；当前模型时间字段不等于持久化查询能力已经存在。

### 14.3 必须遵循的实现顺序

1. News Repository/Query Adapter 提供同一知识水位的 Observation、Version、Identity、Classification、Relation 读取能力。
2. 在查询层先按 `ObservedAt <= system_as_of` 过滤所有记录，再按 `valid_at` / `published_at` 解析业务有效事实；历史 supersedes 链只能由过滤后的断言集合计算。
3. 授权策略采用不可变版本或不可变快照；保存本次授权决策引用。历史授权用于审计，当前读取仍须重新检查撤销状态。
4. 建立固定查询快照和签名/防篡改 Cursor，保证并发写入期间分页集合不漂移。
5. 最后提供 News Application Facade 与 DTO，Research 只依赖该 Facade；内容读取再次执行用途、内容级别和当前授权校验。

**本轮结论：News → Research Facade、知识时间查询、授权决策快照、稳定分页和 Repository/Migration 均未实现。GOV-004 News 阻断保持打开；本轮只做只读源码审查和规划文档更新，没有修改 News 分支。**
