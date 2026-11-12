# News 代码与文档 Review 报告

- Review 日期：2026-11-10
- 分支：`feat/research-news-ingestion`
- 初始 Review 基线：`259229715042069e6a3dc89a4b537213317be827`
- 本轮修复基线：`a9fabb47c152a2b359924ccf1daafadab28f9740`
- 修复提交：`c567f991e28883d7c2551be7614590c32972d304`
- 范围：`internal/news/**`、`doc/news/**`
- 初始 Review 为只读审查并修正文档状态；后续按报告修复了 News 内部授权快照读取完整性和 Provider Catalog 错误分类，没有改变跨模块契约或生产开关。

## 一、需要处理的代码缺口

### [P1] 授权快照读取缺少载荷上限和数据库信封字段一致性校验

位置：`internal/news/adapters/storage/authorization_snapshot.go:112-151`

状态：已在提交 `c567f991` 修复读取侧缺口。

- Get 在 hash/JSON 解码前拒绝空载荷及超过 64 KiB 的 payload。
- Get 同时读取数据库信封字段，并与解码后的 Binding 逐项核对 subject/resource/decision/purpose/content level/decision fingerprint 及 decided_at/expires_at；时间比较按 MariaDB DATETIME(6) 精度归一。
- 最新授权仍按信封字段排序，再通过 Get 读取同一 authorization_ref；Get 会拒绝信封与 payload 不一致的行，因此异常元数据不能被当作有效授权返回。
- MariaDB 集成测试新增 decision 元数据篡改和超限 payload 拒绝断言；实际运行仍待 MariaDB Job。

剩余注意：最新授权选择与读取不是单条 SQL/同一事务快照；若未来要求并发写入下严格线性化，需要再将选择与 payload 读取合并到 Repository 的一致性边界。

### [P1] Evidence 内容读取与当前授权复核不在同一一致性边界

位置：
- `internal/news/app/get_evidence.go:54-99`
- `internal/news/app/authorization_read.go:55-88`

现状：`GetEvidence.Execute` 先调用 PIT Repository 读取完整 Version（包含标题和 content JSON），之后才读取最新授权快照并调用当前策略 Verifier/Resolver。授权检查和内容读取不是同一事务或版本化一致性边界。

风险：授权读取前内容已从数据库加载；策略可能在授权复核与返回之间变化。当前实现只能作为受限的预检流程，不能证明消除了 TOCTOU。

建议：由策略 Owner 与 Server/Repository 共同定义可执行的一致性机制，例如可验证的策略版本快照/租约或事务边界；将当前策略复核与精确 Version 内容读取绑定。无法证明版本一致时拒绝返回内容。补充检查期间撤销、过期和策略版本变化的并发测试。

### [P1] 严格 PIT 尚未执行 available_at 证据门禁

位置：
- `internal/news/app/get_evidence.go:146-151`
- `internal/news/adapters/storage/evidence_query.go:20-111`
- `internal/news/app/list_evidence.go:34-87`

现状：DTO 的 `AvailableAt` 保持 nil，但 Get/List 没有查询可验证的来源公开可用时间证据，也没有在缺失或晚于 `system_as_of` 时排除记录。List Repository 当前主要按系统观察时间和 published_at 过滤。

风险：API 契约要求严格 PIT 对 `available_at` 缺证据或晚于截止时间的记录排除；“返回 null”并不等价于“严格 PIT 排除”。当前 Get/List 不能标记 NEWS-CTR-001 完整验收。

建议：先定义并持久化有来源、有出处、可追溯的 available_at 证据，再在 Get/List 的 PIT SQL 中统一执行 `available_at <= system_as_of`；无证据记录必须排除。不得从 published_at、fetched_at 或 observed_at 推导。

### [P2] Provider Ingest 把 SourceCatalog 依赖错误映射成 Endpoint 不存在

位置：`internal/news/app/provider_ingest.go:80-84`

状态：已在提交 `c567f991` 修复。

- 仅当 `errors.Is(err, ErrSourceEndpointNotFound)` 时返回 not-found。
- 其他 Catalog 错误使用 `errors.Join(ErrSourceCatalogUnavailable, err)` 保留稳定分类与底层 cause。
- 增加 not-found、wrapped not-found、数据库超时三类测试，并断言失败路径不触碰 Repository。

## 二、实现边界与已核实事项

- Evidence Snapshot Repository 已在读取端检查非空载荷、8 MiB 上限、payload hash、item_count、caller、filters fingerprint、page size、system_as_of、sort version、created/expires 时间与数据库信封一致性。
- Observation backlog 使用单条 `COUNT(*) + MIN(retention_until)` 聚合查询；清理查询与 backlog 使用相同到期条件：`retention_until <= now AND payload_purged_at IS NULL`。
- Observation 清理在事务内锁定有界批次，校验 RowsAffected 与领取数量一致；失败回滚整批。集成测试代码已有触发器注入回滚和恢复重试用例，但本轮 CI 未实际运行 MariaDB Job。
- Evidence List 对尚未实现的 CanonicalSymbols、QualityStatuses、BusinessTime 过滤采取拒绝策略，而非静默忽略；这是能力未完成，不是静默扩大结果集。
- News Provider/Source Platform Doer 分属不同 package，构造独立 DestinationPolicy；未发现跨用例共用裸 http.Client 的生产 fallback。
- 本轮未发现 News 目录 gofmt 差异；这不代表其他模块或全仓门禁通过。

## 三、文档问题

1. `README.md` 的早期 NEWS-CTR-001 段落仍写“Query Facade/PIT/Snapshot 未实现”，与当前 `GetEvidence`、`ListEvidence`、PIT Repository、Snapshot Page 实现不一致。
2. `新闻API契约.md` 第 11.5 节仍写 Facade、PIT Repository、Snapshot 未实现；该段应标为历史进度，当前能力与阻断项应以本报告和任务总览为准。
3. `新闻模块开发任务与验收清单.md` 的 Observation retention 旧段仍写“事务回滚专项用例未补齐”，与后续集成测试代码及本文件第六十六节状态相矛盾。
4. README 和任务清单存在多个按日期累积的阶段记录；历史记录保留可以接受，但必须显式声明其为历史快照，不能把旧的未完成状态当作当前状态。

## 四、CI 与验证证据

- Review 基线：`259229715042069e6a3dc89a4b537213317be827`
- 对应 Workflow Run：`42622085639`
- `Test go-core`：success
- 根模块 `Check gofmt`：failure；日志中的 13 处差异属于 Market Data、Notification、Quant，News 目录无差异。
- `News MariaDB Repository and Migration integration`：skipped，未执行数据库集成。
- 本轮未在本地执行 Go 编译、单测或 MariaDB 测试；不据此宣称运行验收通过。

## 五、处理优先级

1. P1：授权快照信封一致性与读取载荷上限。
2. P1：available_at 证据建模与严格 PIT 过滤。
3. P1：授权复核与精确内容读取的一致性边界；该项需要 Platform/Server/策略 Owner 协作。
4. P2：Provider Ingest 错误分类。
5. 真实 MariaDB 11.4 验收 V8、授权快照异常数据、清理并发与回滚。
6. Server 组合根接入、Migration 注册、Diagnostics Runner 和周期清理仍由 Server/集成分支负责。


## 六、修复后验证状态（2026-11-10）

- 修复提交：`c567f991e28883d7c2551be7614590c32972d304`。
- 修复代码提交 `abdb630f4fc409ced5d5341504ea7ef77e420c99` 对应 Run `42740420934` 已完成：`Test go-core` success；根模块 `Test .` failure，失败点为 Notification、Quant 等其他模块的 gofmt 差异，日志未列出 News 文件；News MariaDB 集成 Job 为 skipped。
- 该 Run 最初还捕获到授权快照文件多余右花括号的语法错误，已在 `abdb630f` 修复；后续同一 Run 的 gofmt 检查没有再报告 News 文件，但由于其他模块格式差异，根模块测试未继续执行。
- 最新文档提交 `e13d1543c7299cc887c30e4446811d4921743728` 尚未有独立 CI 结果；未在本地执行 Go 测试或 MariaDB 集成。
- 本轮修复涉及的 MariaDB 断言尚无真实数据库运行证据。
- 当前仍未解决：available_at 可信证据建模/严格 PIT 门禁；授权复核与精确内容读取 TOCTOU；最新授权选择与 payload 读取的事务一致性；真实 MariaDB 11.4 验收及 Server 组合根接线。


## 七、available_at 来源审计补充（2026-11-10）

检查了当前 Candidate、FeedEntry、NewsItemVersion 与 PIT Repository 的时间字段：

- Candidate 当前保留 `PublishedAt/PublishedAtRaw`、`FetchedAt`、`NormalizedAt`、`ObservedAt`；FeedEntry 额外保留 Atom `UpdatedAt/UpdatedAtRaw`。
- NewsItemVersion 只持久化 `PublishedAt` 与系统观察时间；Observation 持久化 `ObservedAt`，Candidate JSON 保存采集侧时间字段。
- 当前未发现带来源证明、来源字段路径/原始值、可信获取方式和可追溯出处的 `available_at` 证据字段或独立来源事件。
- Feed 的 Atom `updated` 是内容更新声明，不等于首次公开可用时间；`published_at`、`fetched_at`、`normalized_at`、`observed_at` 均不得替代。

结论：本分支当前没有足够证据安全实现 available_at 门禁。已在《新闻数据模型与质量契约》和《新闻API契约》中冻结首期准入规则：只接受经来源语义审核、Source/Endpoint 授权、字段 allowlist、精确时区解析并绑定具体内容版本的“发布来源明确声明首次公开可获取时间”证据；必须保留字段路径、原始值、证据 URL、策略版本、采集时间和解析器版本。普通 datePublished/published_date、RSS pubDate、Atom published/updated、dateModified、HTTP Date/Last-Modified、Provider 索引时间及系统采集时间均拒绝作为 available_at。首次成功抓取时间只证明当时已可获取，不证明首次公开时间，首期不回填。证据缺失、冲突或语义不明保持 NULL，严格 PIT 排除。当前接入来源仍无符合条件的字段，因此本次只冻结规则，不新增无真实来源支撑的字段/表，也不改 Get/List SQL；严格 PIT 仍未完成。


## 八、最新授权读取一致性边界修复（2026-11-10）

- 提交：`1d2fbd2756d10afb2c8caf1758b154c573862016`，后续测试夹具唯一化提交：`35a64e43474673ad29690323a03b3c35677f215d`。
- `GetLatestAuthorizationSnapshotForEvidence` 不再先查 authorization_ref、再发起第二条 SQL 读取 payload；现在一条 SELECT 同时完成最新记录排序选择、信封字段与 payload/hash 读取。
- 同一条 SQL 语句在 MariaDB/InnoDB 下使用同一语句读取视图，避免并发追加导致“选择的授权行”和“随后读取的授权行”跨语句漂移。
- 共享读取函数统一执行载荷上限、SHA-256、Binding 结构和数据库信封一致性校验。
- MariaDB 集成测试增加“旧 allow + 新 deny，最新查询必须返回 deny”的断言；测试引用使用唯一后缀，避免持久测试库重复运行冲突。
- 这只解决授权快照选择/载荷读取之间的跨语句不一致；不解决 Evidence 内容读取与当前授权策略复核的 TOCTOU。后者仍需授权策略 Owner/Server 定义内容查询的共同一致性边界。
- 本提交尚未取得 CI/MariaDB 执行结果，不标记运行验收通过。


### 授权快照信封字段单元覆盖补充（2026-11-10）

- 新增 `TestAuthorizationSnapshotEnvelopeMatchesEveryIndexedField`，逐项覆盖 authorization_ref、subject_ref、resource_ref、decision、purpose_code、content_level、decided_at、expires_at、decision_fingerprint 的不匹配拒绝行为。
- 同时验证 DATETIME(6) 精度下亚微秒差异的归一语义。
- Provider Catalog 错误分类测试与授权信封测试的 gofmt 对齐已修复；最新提交 `91291c0b00e1d925a596f61ff33ec1d98c76e63b` 的 CI 仍在运行。


## 九、Evidence Application 边界测试补充（2026-11-10）

- 提交：`cc423098087b39e45716430b84a232e343914045`。
- `TestGetEvidenceStopsAfterContextCancellationAtDependencyBoundaries` 新增“当前授权复核返回后 Context 已取消”场景，要求 Execute 返回 context.Canceled，不能继续构造/返回 Evidence DTO。
- 新增 `TestGetEvidenceSuppressesCandidateAtExactRetentionDeadline`，验证 `RetentionUntil == now` 时按 `now >= retention_until` 抑制 Candidate 采集元数据，同时保留独立授权的版本摘要读取行为。
- 以上为 Application 单测覆盖；未实际运行测试前不标记通过，也不替代 MariaDB 集成验收。


## 十、授权复核 Context 取消边界（2026-11-10）

- 提交：`9edb6f7321939784c3be865d56544109b954f680`。
- Review 发现 `EvaluateNewsEvidenceReadAuthorization.Execute` 仅在调用外部依赖前检查 Context，未在 Verifier 和策略 Resolver 成功返回后检查。若依赖忽略取消并返回成功，内部 UseCase 会错误地产生授权成功结果（尽管 GetEvidence 外层仍有取消检查）。
- 已在两个依赖返回后增加 `ctx.Err()` 检查，并新增 Verifier/Resolver 取消测试。
- 该修复强化取消传播，不解决策略撤销与正文读取的 TOCTOU；两者仍须独立处理。


## 十一、采集策略依赖 Context 取消边界（2026-11-10）

- 提交：`e898104b9b548057330978c3fbc287e6c9d79a57`。
- Provider Ingest 原先在 SourceCatalog 和 ProviderOutputPolicyResolver 成功返回后没有检查 Context；Source Ingest 在 SourceCatalog 成功返回后也未检查。依赖若忽略取消，可能继续执行策略评估并触发写入。
- 已在每个依赖返回后检查 Context，并新增三条取消边界测试，断言取消后不写入 Observation/Identity。
- 该修复只保证取消传播，不代表数据库事务可撤销已经提交的写入；多条 Observation 的批次原子性仍由独立持久化契约决定。


## 十二、News 测试文件 gofmt 门禁修正（2026-11-10）

- CI Run `42743383781` 的根模块 gofmt 检查指出 News 四个测试文件存在格式差异；同时还列出 Market Data、Quant、Notification 的既有差异。
- 已仅修正 News 四个文件，提交 `0076c667d080aaf1b3889f54799d203693129eb9`。其他模块格式问题留给其负责分支处理。
- 该轮 CI 在 gofmt 步骤终止，不能据此判断 Go 测试结果。


## 十三、Feed 采集 Context 取消传播（2026-11-10）

- 提交：`a42c384137e2b22a68daaf126ff5cea22270128d`。
- Review 发现 CollectSource 在 SourceCatalog、SourceTransport 返回后未检查 Context；如果依赖忽略取消并返回成功，流程仍会继续进行策略评估、读取响应和解析。
- 已在入口和依赖返回边界增加取消检查；传输返回后若已取消则关闭响应 Body。读取响应体后也检查 Context。
- 新增两项依赖返回取消测试。未改变 SourceCollectionError 的既有业务分类；Context 取消直接向上传播。


## 十四、Feed 取消测试结构修正（2026-11-10）

- Run `42743661308` 在根模块 gofmt 前置步骤发现 `collect_source_test.go` 新增测试末尾多余右花括号，导致解析失败。
- 已修正测试函数结构，提交 `ac8410ee00b0afb0a4cb6238dd8b03b420e75382`。此前该轮并未执行 Go 测试。


## 十五、Provider Ingest 测试格式门禁修正（2026-11-10）

- Run `42743865832` 的根模块 gofmt 检查仍失败；News 自身唯一列出的文件为 `internal/news/app/provider_ingest_test.go`，其两处复合字面量字段缩进与 gofmt 输出不一致。
- 已仅修正该 News 文件，提交 `c4982d374813e70a30282e4e82027558a6d3384f`。其他模块格式差异未越界修改。
- 根模块测试和 News MariaDB 集成仍未执行。


## 十六、Evidence Snapshot 保存 Context 取消边界（2026-11-10）

- Review 发现 `SaveEvidenceSnapshot.Execute` 校验 Snapshot 后直接调用 Repository，未在写入前阻止已取消请求，也未在 Repository 成功返回后观察取消。
- 已增加前后 Context 检查，并覆盖两类取消边界。
- 成功写入后才观察到取消时不执行补偿删除：Repository 可能已提交，取消只表示调用方无法确认本次操作结果，不代表持久化回滚。后续应通过 SnapshotRef 幂等读取确认。
- 对应代码提交：`4e9998e12315480a9f8e6a769a42bf58a7768af9`、`42f460f40c69ace10ba8e21f5603e8884e1ac7ca`。


## 十七、Feed 解析/归一化取消边界复核（2026-11-10）

- 复核发现 Feed 采集此前仅在响应体读取后检查 Context；解析和候选归一化阶段缺少取消检查，可能在调用方已取消后继续 CPU 工作并返回结果。
- 已补齐解析后、候选循环、归一化时钟回调后、结果封印前和最终返回前检查，并增加回归测试。
- 该修复不改变持久化、授权或 Feed 解析语义。
- 对应提交：`a7988ad1fb7aa9ef7635365e8215cee017c15b76`。


## 十八、Evidence Snapshot 幂等重放信封一致性修复（2026-11-10）

- 修复前：`CreateEvidenceSnapshot` 在 INSERT 失败后只读取既有行的 `payload_hash`；哈希相同即返回成功，没有验证数据库信封字段。
- 风险：既有行的 caller、分页参数、PIT 时间或过期时间等索引字段被篡改时，创建重放可能错误报告成功，而后续 Get 会拒绝该行。
- 修复：幂等重放同时读取并核对 caller_ref、filters_fingerprint、page_size、system_as_of、sort_version、created_at、expires_at、item_count；payload hash 不同返回冲突，hash 相同但信封不一致返回无效快照。
- 回归：MariaDB 集成用例篡改 caller_ref，断言重放返回 `ErrInvalidEvidenceSnapshot`；恢复字段后确认相同快照可再次幂等成功。
- 当前限制：尚未取得本次提交的 CI 结果；News MariaDB 集成此前为 skipped，该新增断言仍需真实 MariaDB 11.4 执行。


## 十九、Evidence Snapshot 写入端过期门禁修复（2026-11-10）

- Review 发现 `EvidenceSnapshot.ValidateAt` 只检查 CreatedAt/SystemAsOf 不晚于当前时间，没有检查 ExpiresAt；已过期快照仍可通过 Save/Application 和 Repository 写入前校验。
- 已将 `now >= ExpiresAt` 纳入无效快照条件，与 Get 的过期边界保持一致；达到过期时刻即拒绝保存/幂等重放。
- 新增边界单测覆盖过期前最后一纳秒、ExpiresAt 相等和过期后一纳秒。
- 当前未取得本次提交 CI 结果；MariaDB 11.4 集成仍需独立执行。


### Snapshot 幂等重放载荷哈希复核补充（2026-11-10）

- 二次复核发现只比较数据库 payload_hash 与本次待写 payload 的哈希，仍不能识别 payload_json 已被篡改但 payload_hash 未同步修改的记录。
- 重放查询现读取既有 payload_json，先检查非空及 8 MiB 上限，再重新计算 SHA-256；存储哈希与实际载荷不一致时返回 ErrInvalidEvidenceSnapshot。
- MariaDB 集成测试新增只篡改 payload_json、保留 payload_hash 的场景；恢复原始载荷后再次确认幂等创建成功。
- 仍待真实 MariaDB 11.4 执行；未取得运行结果前不标记通过。


## 二十、Evidence Snapshot 清理重复执行边界（2026-11-10）

- Repository 使用单条有界 DELETE，条件为 `expires_at <= now`，因此到期边界与读取端不可用边界一致；单条 SQL 的删除操作具备语句级原子性。
- 原集成场景只断言首次删除数大于零，未验证目标行确实消失及重复清理不重复计数。
- 新增 MariaDB 集成断言：首次清理后目标 Snapshot 行数为零；同一时间边界再次清理返回零且无错误。
- 该场景仍待 MariaDB 11.4 实际执行；并发读取与清理的时序仍由数据库语句级隔离/锁行为决定，当前不宣称提供跨请求原子快照租约。


## 二十一、Evidence Snapshot 并发清理批次审查（2026-11-10）

- Repository 使用单条有界 DELETE 语句完成删除；并发 Worker 的实际锁等待、批次领取和受影响行数必须由 MariaDB 验收确认。
- 原集成测试覆盖首次清理、目标行消失和重复清理，但没有覆盖多个 Worker 并发执行。
- 新增 4 条到期 Snapshot、两个批次上限为 2 的并发清理 Worker；要求总删除数恰好为 4、每个 Worker 不超过上限、所有 Snapshot 行消失且重复清理为 0。
- 该测试仍待 MariaDB 11.4 实际运行；当前不宣称并发清理已通过数据库验收。


## 二十二、Evidence Snapshot 清理后读取边界（2026-11-10）

- GetEvidenceSnapshot 通过 caller_ref 与 snapshot_ref 定位记录；记录已被清理时，sql.ErrNoRows 映射为 ErrEvidenceSnapshotUnavailable。
- 新增集成断言：先完成到期删除并确认行数为零，再通过相同 SnapshotRef 读取，必须返回 ErrEvidenceSnapshotUnavailable。
- 此用例验证删除后的确定性边界，不等同于并发 SELECT/DELETE 竞态或跨请求原子租约；后者仍需 MariaDB 实测。


### 二十三、N7-06 清理结果与 Context 取消语义（2026-11-10）

- Observation Retention Repository 在事务提交成功后返回批次计数；Snapshot TTL Repository 在 DELETE 成功且 RowsAffected 成功读取后返回删除计数。Cleaner 原先在 Repository 成功后再次检查 Context，可能丢弃已经确认的持久化结果。
- 已移除 Repository 成功后的二次 Context 检查，保留调用前取消门禁。成功结果现在优先于其后发生的 Context 取消；SQL/事务执行期间发生取消仍由 Repository 错误向上传递。
- Commit/网络错误可能具有提交结果不确定性。当前实现保留原始错误，不把它错误映射为确定回滚；调用方应通过 Observation backlog 或 SnapshotRef/行状态查询核实。
- 尚未覆盖 RowsAffected() 自身返回错误、Commit 连接中断和锁等待超时的可控驱动注入；也未运行 MariaDB 11.4。不得将该项标记为数据库运行验收通过。


### 二十四、N8-01 Evidence List 游标同时间戳边界（2026-11-10）

- 审查确认数据库查询排序与 Snapshot 比较器一致：`observed_at DESC, news_id ASC, version_id ASC`。
- Cursor 绑定 SnapshotRef 与最后一条记录的时间戳、NewsID、VersionID；分页在不可变 Snapshot 中定位精确排序位置，不使用 offset，也不重新执行 PIT 查询。
- 新增同一时间戳下多 News/Version 的跨页测试，覆盖次级排序键和连续分页无缺口/无重复。
- 尚未执行测试；MariaDB DATETIME(6) 实际排序及跨数据库精度边界仍待集成环境验收。


### 二十五、N8-02 Evidence 分页失败语义

- 整页授权语义已由 `ListEvidence.Execute` 实现：所有引用逐条授权解析；任一条失败即返回零值页面，不返回部分结果。已有第二条授权失败测试覆盖该行为。
- 修正了合法签名游标指向 Snapshot 中不存在的排序位置时的错误边界：由内部结构错误统一映射为 `ErrEvidenceSnapshotPageUnavailable`，避免将 Snapshot 内部校验错误泄露给跨模块调用方。
- 新增合法签名但位置缺失的 continuation 测试，检查无部分页面、无 PIT 重跑。
- 代码审查确认 Snapshot 读取错误原样传播，便于上层诊断；对外 Fault 映射仍由 Server/API 组合层负责。
- 尚未运行 Go 测试或 MariaDB 11.4。


### 二十六、N8-03 Evidence Snapshot 读取完整性

- 静态审查确认 GetEvidenceSnapshot 在 JSON 解码前执行载荷非空/8 MiB 上限检查和 SHA-256 校验；解码后执行 Snapshot.Validate，并将 payload 与数据库信封字段交叉核对。到期状态独立映射为 unavailable。
- 原 MariaDB 用例覆盖哈希字段篡改、信封元数据篡改、错误 caller、到期和删除后读取；但 payload_json 被篡改、载荷超限的直接 Get 路径缺少断言。
- 增加两项 MariaDB 集成回归：保留旧 hash 篡改 payload_json 后 Get 拒绝；写入超过 8 MiB 的合法 JSON 后 Get 在哈希/解码前拒绝。测试在各自断言后恢复原始 payload。
- 尚未取得 MariaDB 11.4 实际运行结果；不将新增集成用例记为通过。


### 二十七、N8-04 Observation Tombstone 读取语义

- Repository 的 PIT SELECT 在 SQL 层识别 payload_purged_at，避免将 candidate_json 载荷读回；扫描器再次将 Candidate 归零，形成双层防护。
- GetEvidence 对 Tombstone 执行时间线和空载荷校验；授权器仍可基于保留的 Observation 来源/策略字段与 News Item Version 做独立授权。DTO 只保留独立版本字段，Candidate 来源的 PublishedAtRaw/FetchedAt/NormalizedAt 不披露。
- 新增 Application 测试验证授权器看到的是完整一致 Tombstone、候选元数据不泄露，而独立版本内容在授权后仍可读取。
- 现有 MariaDB 集成已覆盖清理后 Tombstone 行保留、PIT 单条读取屏蔽和幂等重放拒绝；尚未覆盖 ListEvidenceVersionsAt 清理后读取。未执行 Go/MariaDB 测试，不标记运行验收通过。


### 二十八、N8-05 Identity 历史凭据一致性

- MariaDB Repository 通过 news_identifier_status_history 选取不晚于 system_as_of 的最新状态事件，并以 history_id 作为同一时间戳下的确定性次级排序；同时要求凭据 created_at <= system_as_of。查询不读取当前 identifier_status 来重建历史状态。
- Application 边界此前未校验 Repository 返回凭据是否属于请求的精确身份候选，亦未校验状态枚举；现已增加 SourceID/Type/Value/Hash 精确绑定、状态枚举和时间边界校验。
- 回归测试覆盖类型、原文、哈希和未知状态异常，以及合法 invalid 历史状态不参与匹配。
- Observation Tombstone 保留 Observation 行和身份历史外键；身份状态历史与 Candidate 载荷清理相互独立。
- 本轮未运行 Go 测试或 MariaDB 11.4；不标记运行验收通过。


### 三十、N8-06 Relation 历史链完整性

- 发现目标侧查询只返回直接触及目标 NewsID 的断言；后继确认断言更换 ToNewsID 后，旧目标查询看不到后继，新目标查询又看不到不再直接关联的父断言，双向历史均可能断链。
- Repository 使用递归 CTE 返回直接命中断言的完整 supersedes 链（祖先与后继），并严格限制所有节点不晚于 system_as_of。
- Application 校验直接关联记录、完整链闭包、稳定排序和追加式历史约束；新增目标变更与无关链节点拒绝测试。
- MariaDB 集成增加旧目标/新目标两侧读取完整链的回归代码；尚未运行 Go 测试或 MariaDB 11.4，递归 CTE 执行行为仍待验收。


#### N8-06 补充审查

- 空历史结果属于正常业务结果，已保留空切片语义；只有非空但不含直接命中断言的结果才判为 Repository 历史不一致。
- MariaDB 集成场景使用真实 News Item 外键和 ApplyNewsItemRelation 追加路径，避免测试直接写入关系表绕过业务约束。


## 七、MariaDB 11.4 实际执行结果（2026-11-10）

- News MariaDB Job 已从仅 PR/手动触发调整为：PR/手动仍可触发，且 `feat/research-news-ingestion` push 也执行；其他分支 push 不触发该 News 专项数据库 Job。
- 执行命令覆盖 `TestNewsMariaDBRepositoryIntegration` 与 `TestObservationRetentionPurgeIntegration`，不再只运行基础 Repository 测试。
- Run `42763611124`：News MariaDB Repository and Migration integration = success；MariaDB 版本为 11.4，实际执行 V1-V8 Migration、Repository、授权快照/PIT/关系历史和 Observation retention 清理用例。
- 同一 Run 的 `Test go-core` = success；根模块 `Test .` = failure，失败发生在根模块 gofmt 门禁，News 目录 gofmt 已由 News 专项 CI 通过，不能将根模块全仓验收标记为通过。
- News Provider/全量模块专项 Run `42763451242` = success，覆盖 News gofmt、全量单测、Race、Vet 和 Provider 契约测试。
- 本次集成样本修正均遵循现有契约：deny 的 expires_at 晚于 decided_at；PIT 样本完整写入三类策略版本；DATETIME(6) 比较按微秒精度；关系历史查询不使用未来 system_as_of。未放宽生产校验。
- 剩余阻断：available_at 证据建模/严格 PIT、授权复核与内容读取 TOCTOU、Server 生产装配、总 Migration 注册及真实生产来源/出口验收。CI 临时数据库通过不等于生产数据库已迁移。


## 八、V7→V8 既有数据升级回归（2026-11-10）

- 新增集成测试路径：在空库或最高已应用版本低于 V8 的测试库中，先执行 V1-V7，插入一条 V8 之前已存在的 Observation，再执行完整迁移集以应用 V8。
- 验证升级后既有记录的 `source_id`、`identity_status`、`candidate_json` 未改变，新增 `retention_until` 与 `payload_purged_at` 均保持 NULL。
- 对已处于 V8 的持久测试库不执行降级式 fixture，仍执行完整迁移集的版本/校验和一致性检查。
- Run `42764067365` 的 News MariaDB Repository and Migration integration Job = success，真实 MariaDB 11.4 执行通过。
- 同一提交的 News Provider/全量模块 Workflow Run `42764067354` = success；根模块 `Test .` 仍因其他模块 gofmt 差异失败。


## 九、available_at 证据准入规则冻结（2026-11-10）

- 首期只接受发布来源明确声明“首次公开可获取时间”的来源字段；普通发布时间、内容更新时间、HTTP 响应时间、搜索索引时间和系统采集时间均不具备该语义。
- 准入前必须完成来源语义审核和 Source/Endpoint allowlist，并保留来源/Endpoint/策略版本、字段路径、原始值、时区、证据 URL、采集时间、解析器版本及受控证据摘要/引用；证据绑定到具体 News Item Version。
- 日期粒度、时区缺失、语义不明、版本绑定不明、字段冲突、来源授权不足均不得生成 available_at；证据状态保持 unknown。
- 首次成功抓取仅证明在抓取时刻已可获取，不证明首次公开时刻。首期不以首次抓取时间回填 available_at；如未来需要利用该事实，必须另行定义区间/上界语义，不能复用 available_at。
- 已验证证据采用追加式修订，不允许后续采集静默覆盖；修订需保留旧证据与原因。
- 当前 Bocha/Tavily/Brave News/SearXNG 候选字段及 RSS/Atom/HTML 解析字段均无符合条件的来源可用时间声明，因此本轮不新增数据库字段/证据表，不修改 Get/List SQL；继续保持 available_at=NULL。
- NEWS-CTR-001 的严格 PIT 仍未完成：实现阶段必须先接入真实来源证据，再持久化 verified 证据，并在 Get/List 查询入口统一执行 `available_at <= system_as_of`；缺证据和晚于截止时间均排除。


## 十、N8-04 ListEvidence Tombstone 集成覆盖补充（2026-11-10）

- Review 发现 Observation Retention 集成测试此前只验证单条 GetEvidenceVersionAt 在清理后不返回 Candidate；ListEvidenceVersionsAt 的清理后载荷屏蔽缺少真实 MariaDB 断言。
- 在 TestObservationRetentionPurgeIntegration 中增加列表 PIT 查询：要求保留合法 News Item Version 引用，同时返回空 Candidate 和非空 PayloadPurgedAt。
- 查询时点使用清理后的当前 UTC 时间，遵守 system_as_of <= now 校验。
- 代码提交：0452bbbfc42b56661403bd49273c6e9d7046a6ea。News 全量 CI Run 42768996986 success；MariaDB 11.4.13 集成 Run 42768996723 success。新增 ListEvidence Tombstone 断言已在真实数据库执行通过。
- 该测试只覆盖 Observation Tombstone 的列表读取语义，不解决授权预检与内容读取之间的 TOCTOU，也不改变 available_at 证据准入规则。


## 九、2026-11-11 冻结决议符合性修正

- 代码基线：`9ace32d2ce8fa0c6775815f3b8b5379220fa1610`；本轮修复提交：`35c2a52079dbb002ed9ff328e77c3d738b79f34d`、`78a041c38a4ba88e8aa266c43fb0617f2d082e78`。
- 按 DEC-PLAT-002-042 同步 Evidence DTO：新增必填 `relation_status`，当前未接入关系查询时固定为 `not_provided`，`relation_refs` 保持空数组；不得把未查询误报为已确认无关系。
- 按 GOV-CTR-002 / DEC-PLAT-002-042 为 NEWS-CTR-001 所有 Schema 的绝对时间字段增加 UTC `Z` pattern，并同步 ResearchEvidencePage Schema/Golden。
- 明确 `system_observed_at` 的权威来源为 `NewsItemVersion.ObservedAt`；Observation.ObservedAt 仍独立参与 PIT。
- 本轮未实现 available_at 推断：现有来源无满足准入条件的首次公开可用时间证据，严格 PIT 仍不能验收。
- 授权读取与内容查询尚未位于可证明的同一策略/存储一致性边界；公共 JCS Runtime Verifier 和可信当前策略 Resolver 仍需由公共平台/Server 组合根提供。News 不复制 JCS，也不声称 TOCTOU 已解决。
- 本轮未执行构建或测试。若需要构建，按分支规则先将本分支合并到 `origin/integration`，仅在 integration 执行，且不得直接修改 integration。


## 2026-11-12 News 模块自身代码验收复核

- 验收基线：`83358f8cbe1f69a635218438dc97015e33300581`；范围仅为 `internal/news/**`、`doc/news/**` 及 News 自身 CI 门禁。未审查或修改其他模块实现。
- News 全模块专项 Run `43143438840`（基线代码 `1ab32cd1f6e27c8f5e3727997ca5d6c7f3e3dcc2`）通过：News gofmt、`go test ./internal/news/...`、`go test -race ./internal/news/...`、`go vet ./internal/news/...`；Provider 契约专项的测试、Race、Vet 也通过。其后提交只修改工作流和文档，没有改变 `internal/news/**` 代码。
- MariaDB 11.4 Run `43144140642`（当前代码树）成功执行两个独立集成测试：`TestNewsMariaDBRepositoryIntegration` 与 `TestObservationRetentionPurgeIntegration` 均 PASS。此前 CI 命令只运行第一个测试，现已修正为同时执行两个测试。
- 根模块 Go CI 仍失败，但失败位置在 Quant gofmt 和 Researchflow 未定义符号（`validSourceOwner`、`evidenceGraphHasCycle`）；不属于 News，未越界处理。

### 当前验收结论

**News 自身基础代码质量门禁通过，但 NEWS-CTR-001/002 的业务能力完整性尚未通过，不应把整个 News 模块标记为全部完成或可生产披露。**

1. **P1 — 严格 PIT 的 available_at 门禁未完成。** 当前没有任何已接入来源提供满足冻结准入规则的“首次公开可获取时间”证据。代码正确地不以 published_at、fetched_at、observed_at 等字段代替；Get/List 仍不能宣称满足严格 available_at PIT。待出现合格真实来源后，再实现证据持久化和查询过滤。
2. **P1 — 授权复核与内容读取 TOCTOU 未解决。** 当前 `GetEvidenceVersionAt` 先读取完整 Version 内容，随后才读取授权快照并调用当前策略 Authorizer；两者没有共同的策略/存储一致性边界。Application 的 fail-closed 构造和授权检查不能证明消除竞态。安全闭环需要在跨模块验收时明确公共 Verifier、可信策略 Resolver 与内容读取事务/版本边界，完成前不得开放受限内容生产披露。
3. **P2 — 真实 Provider 网络与授权证据未验收。** 四家 Provider 的固定响应契约、错误分类、Race 和 Vet 已通过；但真实账号配额、输出许可、供应商网络稳定性和真实响应语义未验证。不得用固定响应测试替代 Live Probe。
4. **非本分支范围 — 生产装配。** Server 的 Provider/Source BoundDoer 注入、Migration 总注册、Diagnostics Runner 和周期清理注册，留待跨模块联调；本轮不修改 Server 或其他模块。

### 结论分层

- **已通过：** News 包级格式、单测、Race、Vet、Provider 固定响应契约，以及 MariaDB 11.4 Migration/Repository/Observation Retention 集成测试。
- **仍阻断业务完整验收：** available_at 严格 PIT、授权复核与内容读取 TOCTOU。
- **留待跨模块联调：** Server 生产装配及公共授权能力接线。
- **留待真实环境验收：** Provider 网络、配额和输出许可。

本次验收只证明当前 News 自身代码和上述 CI 场景通过，不代表全仓 CI 通过，也不代表 News 已可生产上线。


## 十二、News Diagnostics 依赖调用后的取消边界（2026-11-12）

- Review 发现 Observation retention、Source endpoint Catalog、Storage Ping/Schema Check 在外部依赖返回后未再次检查 Context。若依赖忽略取消并返回成功，诊断可能在请求已取消后继续报告 healthy/degraded。
- 已在上述依赖返回边界补充 Context 检查。
- 新增依赖执行期间主动取消 Context 的回归测试，覆盖 retention、source catalog、storage ping 和 schema 四条路径。
- 该修复只约束诊断执行生命周期，不改变业务质量状态、授权决策或平台 Runner 语义。
- 对应代码提交待 CI 验收；不把新增测试视为已运行通过。


### 十二、补充：Storage 探测的调用前取消门禁

- 对照取消传播 Review 记录与实际代码时，发现 Storage Ping/Schema 虽已在依赖返回后检查 Context，但调用前门禁尚未落入最终代码。
- 已补充调用前检查，确保 Context 已取消时不再触发数据库探测。
- 新增回归测试验证已取消 Context 下 Ping 和 MissingSchemaTables 均不会被调用，诊断状态为 unavailable。


## 十三、News Diagnostics Storage Context 取消边界补充（2026-11-12）

- 复核 StorageChecks 时发现 Schema Check 缺少数据库探测调用前的 Context 门禁；已补充 `ctx.Err()` 检查，已取消时不调用 `MissingSchemaTables`。
- 同时发现 Storage Ping 先处理依赖错误、后检查 Context；依赖在取消时返回 `context.Canceled` 会被错误归类为普通连接故障。现改为依赖返回后优先检查 Context，再处理 Ping 错误。
- 回归测试直接调用 Check.Run，断言已取消 Context 返回 `context.Canceled` 且 Schema 探测调用次数为零；Ping 依赖取消并返回取消错误时也验证取消错误优先级。
- 代码提交：`1ba83c9`、`dc8d102`、`34c9cdf`、`30e1582`。最新 SHA 的 News 专项和组合 CI 尚在执行，结果待记录。
- 本修复只收敛 News Diagnostics 的取消语义，不改变诊断业务状态、数据库 Schema、Migration 或平台 Runner 行为。


## 十四、2026-11-12 最新代码与 CI 状态复核

- 当前文档记录的代码基线：`eac3269ebaa625bd583f5ef3a73a139ff02c3881`；最新提交修正 Observation Retention 事务调用参数，未改变领域行为或公共服务边界。
- News 专项 Workflow Run [43210204658](https://github.com/Kasign/stock_data_provider/actions/runs/43210204658)：
  - `Test full News module`：success，包含 News gofmt、`go test ./internal/news/...`、Race、Vet。
  - `Test News Provider contracts`：success，包含 SearchCandidates/Provider Adapter 契约测试、Race 和 Vet。
- 根模块 Workflow Run [43210204807](https://github.com/Kasign/stock_data_provider/actions/runs/43210204807)：
  - News MariaDB Repository and Migration integration：success，实际执行 News Migration/Repository 集成。
  - 根模块 `Test go-core` 与 `Test .`：success。
  - Market Data MariaDB、Compose/Docker、Market Data 迁移及测试 Job 为 skipped；不能将其记为通过。
- 2026-11-12 Diagnostics Context 取消边界修复已纳入 News 专项 CI：Observation retention、Source Endpoint Catalog、Storage Ping、Schema Check 覆盖依赖调用前/后的取消传播；已取消时不触发探测，依赖返回取消错误时优先传播 Context 取消。当前没有证据表明需要再重复实现同类取消检查。
- 当前 CI 证明 News 包级格式、单测、Race、Vet、Provider 固定响应契约和 MariaDB 专项集成通过；不证明真实 Provider 网络/来源授权、生产 DNS/出口、Server 组合根装配或受限内容生产披露通过。

### 当前缺口重新分级

| 缺口 | 是否属于 News 可独立继续的代码任务 | 判定 |
|---|---|---|
| available_at 证据与严格 PIT | 否，当前无合格来源证据 | 继续 NULL；不得推导、伪造或新增无真实证据支撑的字段/表。获得符合冻结准入规则的来源后再实施 |
| 授权复核与内容读取 TOCTOU | 否，需跨模块一致性方案 | 等待公共 JCS Verifier、可信策略 Owner 与 Server/Repository 共同确定一致性边界；News 仅实现明确方案的适配 |
| Provider/Source BoundDoer、Migration、Diagnostics Runner、Cleaner 调度生产装配 | 否，归 Server 组合根 | News 已提供对应构造/领域入口，不修改 Server 文件 |
| 真实 Provider/Feed 网络与授权验收 | 当前不具备条件 | 等待真实凭证、账号适用条款、来源授权和部署网络证据；固定响应测试不替代 Live Probe |
| News 自有单测、Race、Vet、Provider 契约、MariaDB 集成 | 已有当前 CI 证据 | 不为形式重复回归；只有后续代码改动或新缺陷才重跑相应门禁 |

### 本轮结论

News 自身代码质量和 MariaDB 专项验证已取得当前 CI 通过证据。当前剩余事项均受公共能力、Server 组合根、策略 Owner 或真实来源证据约束；没有发现可通过在 News 内重复开发公共能力来安全关闭的缺口。后续按“有明确新证据再推进”的方式执行，不新增空壳接口、字段、表或 WebUI 专用 API。
