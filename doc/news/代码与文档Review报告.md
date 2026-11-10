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

结论：本分支当前没有足够证据安全实现 available_at 门禁。下一步需要先在 NEWS-CTR-001/数据模型中明确可接受的来源证据类型、原始证据保存、解析/可信等级和回溯规则，再新增持久化字段及 Get/List SQL 过滤；在此之前继续保持 available_at=null，并明确严格 PIT 尚未完成。


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
