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
