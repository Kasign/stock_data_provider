# News 代码与文档 Review 报告

- Review 日期：2026-11-10
- 分支：`feat/research-news-ingestion`
- Review 基线：`259229715042069e6a3dc89a4b537213317be827`
- 范围：`internal/news/**`、`doc/news/**`
- 本轮只读审查并修正文档状态；未修改业务代码，未改变跨模块契约或生产开关。

## 一、需要处理的代码缺口

### [P1] 授权快照读取缺少载荷上限和数据库信封字段一致性校验

位置：`internal/news/adapters/storage/authorization_snapshot.go:112-151`

现状：
- 写入端限制 JSON payload 最大 64 KiB，但 `GetAuthorizationSnapshot` 读取后没有复用该上限，直接计算 SHA-256 并 JSON 解码。
- 表中 `subject_ref/resource_ref/decision/purpose_code/content_level/decided_at/expires_at/decision_fingerprint` 等索引/元数据列未与 JSON Binding 对照。
- `GetLatestAuthorizationSnapshotForEvidence` 依赖这些数据库列筛选和排序，再按引用读取 payload。数据库元数据被错误修改时，payload hash 仍然有效，但“最新授权”选择可能偏离 Binding 本身。

风险：数据库记录异常或元数据不一致时，可能选中错误的历史授权快照；同时读取端缺少和写入端对称的资源上限。

建议：
1. 读取端先拒绝空载荷和超过 64 KiB 的 payload。
2. 读取时同时取回数据库信封字段，解码后逐项核对 Binding 的 subject/resource/decision/purpose/content level/decision fingerprint/时间字段。
3. 最新记录选择和 payload 读取应有明确的一致性语义；优先单条查询返回所需 payload 与信封字段，或在同一事务一致性快照中完成。
4. 增加元数据列被篡改、超限 payload、最新 deny/conditional 不得回退旧 allow 的 MariaDB 集成测试。

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

现状：`LookupEndpoint` 返回任意错误时，与身份不匹配、Endpoint 禁用一起映射为 `ErrSourceEndpointNotFound`。

风险：数据库不可用、超时或 Catalog 内部错误会被误报为资源不存在，调用方无法区分 not-found 与 dependency-unavailable，影响重试、诊断和排障。

建议：仅对 `errors.Is(err, ErrSourceEndpointNotFound)` 映射 not-found；其他错误保留 cause，并映射稳定的依赖不可用错误。增加 Catalog not-found、timeout、内部故障三类测试，确认均不触碰 Repository。

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
