# News 数据模型与自动采集闭环收敛实施计划

更新时间：2026-11-26  
执行分支：`repair_all`  
任务编号：`NEWS-POST-08`  
状态：`IN_PROGRESS`

本文是本任务唯一实施方案。当前数据规则以 [新闻数据模型与质量契约](./新闻数据模型与质量契约.md) 为准。旧版 News 数据模型、身份、转载关系和证券/行业关系规则不再适用；冲突文档已直接替换或删除，不保留兼容要求。

## 1. 目标

一次性收敛新闻主体、全局 URL 去重、来源身份、Observation、内容版本、统一入库事务和 Feed 自动采集生产闭环。

最终结果：

1. `news_items` 保存当前完整新闻内容。
2. 完全相同的规范 URL 全局只对应一个新闻主体。
3. 来源原生文章 ID 只在来源范围内唯一；多个来源身份可以指向同一主体。
4. RSS/Atom 和被研究流程选中的 Provider 候选共用一个正式入库用例。
5. 主体、来源身份、Observation 和必要版本在一个 MariaDB 事务中完成。
6. 内容未变化不增加版本；内容变化才追加不可变历史版本。
7. Server 真实装配 Feed 采集路径，执行机制复用 Researchflow。
8. 删除不再需要的关系/身份历史表、双阶段写入路径和相关文档。

不做旧数据迁移、旧逻辑兼容、双写或兼容视图。用户在最终 Schema 验收后手动清理旧 News 数据；本任务不远程清库。

## 2. 当前代码事实与边界

- News V1 Migration 当前定义最终 10 张表；Schema Validator 运行时检查精确表集合、必需字段、索引和全部外键定义，Migration 测试同步核对外键契约。
- news_items 保存当前新闻主体和当前可持久化内容；news_item_versions 保存不可变历史内容版本。
- news_identifiers 只保存 source_id 范围内的来源原生文章 ID；规范 URL 通过 news_items.canonical_url_hash 全局唯一匹配。
- 内容指纹只反映来源侧新闻内容和元数据，不包含规范 URL、来源原生 ID、Provider 摘要或采集时间。
- Feed 摘要按 ContentAbstract/PersistentExcerpt 策略进入正式内容；未识别类型的 Body 不升级为全文，ProviderSummary 不冒充 SourceSummary。
- Provider SearchCandidates 默认输出 Researchflow StageOutput，不自动创建正式 News；被研究流程选中后才进入统一正式入库。
- 主体、来源身份、Observation 和内容版本由同一个正式入库事务提交；不再有双阶段 ResearchUpdate。
- News 不负责转载关系、事件聚合、证券/行业关系或身份历史。
- Research 通过 News Evidence Query 使用 NewsID + VersionID；PIT 继续依赖 Version、Observation、Availability Evidence/Decision。
- Server 当前已有 Provider Search Stage 和 Research Evidence Query 装配，但完整 Feed SourceCatalog/Transport/Collect/Ingest 生产组合仍待 P0-4。
- StaticSourceCatalog、Platform Endpoint Transport、CollectSource、Source Policy Resolver、Policy Version Publisher 已存在，继续复用。
- StageOutput 生命周期属于 Researchflow，不由 News Cleaner 管理。

P0-1 全量依赖审计已完成；本计划后续只对具体字段、SQL 和调用点做定点核对，不重复审计。

## 3. 最终 Schema

### 3.1 保留表（10 张）

1. `news_items`
2. `news_identifiers`
3. `news_observations`
4. `news_item_versions`
5. `news_authorization_snapshots`
6. `news_evidence_snapshots`
7. `news_policy_versions`
8. `news_current_policy_versions`
9. `news_version_availability_evidence`
10. `news_version_availability_decisions`

### 3.2 删除表（4 张）

- `news_relation_assertions`
- `news_relation_evidence`
- `news_item_relations`
- `news_identifier_status_history`

同步删除这些表专属的 Domain、Application、Repository、DTO、测试和注册代码。不得只删 DDL 而留下失效业务入口。

### 3.3 核心字段

`news_items`：

- `news_id`：主键。
- `canonical_url`：规范 URL，可空。
- `canonical_url_hash`：规范 URL 的 SHA-256；非空 URL 必须有值并建立全局唯一索引。
- `title`、`publisher_name`、`publisher_domain`、`author`、`published_at`：当前新闻元数据。
- `content_json`：当前允许持久化的来源内容层；摘要按策略保存，正文必须先识别类型并通过对应策略。
- `content_fingerprint`：只覆盖来源侧内容和元数据，不覆盖 URL/来源身份/Provider 摘要/采集时间。
- `created_at`、`updated_at`：记录创建/更新时间。

删除 `origin_source_id`。来源身份由 `news_identifiers` 和 `news_observations` 表达。

`news_identifiers`：

- `source_id`、`identifier_hash`、`identifier_value`、`news_id`、`created_at`。
- 唯一键为 `(source_id, identifier_hash)`；Hash 命中后必须比较原始 ID。
- 删除 `identifier_type`、`identifier_status`；不保存 URL 身份和历史身份状态。

`news_observations`：保留来源/Provider、策略版本、候选元数据、观察时间、Retention、身份处理结果、news_id 和稳定原因码；不长期复制完整正文。

`news_item_versions`：保留版本号、指纹、标题、发布者、作者、发布时间、content_json、observed_at、observation_id，供 Research/PIT 历史回溯。

其余 6 张授权、Evidence Snapshot、策略版本和 Availability Evidence/Decision 表按现有消费者保留，不因本次简化删除。

## 4. 统一正式入库规则

- RSS/Atom 采集结果直接进入正式入库。
- Provider SearchCandidates 默认留在 Researchflow StageOutput；只有研究流程明确选中/确认后才进入正式 News。
- URL 存在时先按规范 URL 全局查找；命中则复用 news_id。
- URL 未命中时按 source_id + source_native_id 查找；命中则复用主体。
- URL 与来源原生 ID 分别命中不同主体时返回 identity_conflict，不自动合并。
- URL 和来源原生 ID 都缺失时拒绝创建正式主体。
- 同一 URL 被不同来源发现时，每个来源保留自己的来源原生身份和 Observation。
- 新主体写入当前内容并创建初始版本。
- 已有主体内容指纹不变时不新增版本；URL 身份补全只更新主体身份字段，不创建内容版本；来源内容或内容元数据变化时更新主体并追加版本。
- 主体、身份、Observation、版本在同一事务中提交或回滚。

### 4.1 固定事务锁顺序

1. 按固定策略类型顺序锁定并读取 Source/Endpoint/Provider 当前策略指针。
2. URL 存在时，按 canonical_url_hash 获取跨实例 URL 命名锁。
3. 获取来源原生身份的 source_id 命名锁。
4. 全局解析 URL 与来源原生 ID；命中不同主体则冲突。
5. 锁定或创建 news_items。
6. 锁定最新版本并比较指纹。
7. 写入来源身份和 Observation。
8. 提交事务后释放命名锁。

唯一索引是最终并发防线，进程内 Mutex 不能替代跨实例协调。所有正式写入路径遵守同一锁顺序。URL Hash 命中必须比较原文。

## 5. 阶段执行顺序

### P0-1：全量依赖审计

状态：DONE

已完成现有表、读写者、Repository、Research/PIT 消费者、关系职责、Server 装配和 Diagnostics/Schema 测试依赖核查。

### P0-2：最终数据模型与 Migration

状态：DONE

1. 逐字段对齐 Domain、Repository、PIT SQL 和 Research Evidence DTO。
2. 修改单一 V1 Migration，直接创建最终 10 张表。
3. 同步修改 Schema Validator、Schema 测试、Migration 测试和 Server Migration Registry 相关清单；运行时校验必须覆盖外键名称、所属表、引用表及有序列。
4. 删除 4 张表及其专属索引、外键、Repository 和失效用例。
5. 通过空库 Migration 和真实 MariaDB Schema 验证。

完成门槛：Migration 从空库直接创建最终 Schema；Validator 与真实 MariaDB 表/索引/外键一致。无需兼容旧 V1。已在 Go test suite Run `45595639304` 的 News MariaDB Job 中使用 MariaDB 11.4 实际通过物理 Schema 基线和 ValidateSchema 校验。

### P0-3：统一正式入库与全局 URL 去重

状态：DONE

1. 建立唯一正式入库 Application 用例。
2. RSS/Atom 和被选中的 Provider 候选复用该用例。
3. 合并主体、身份、Observation、版本写入事务。
4. 移除独立的双阶段版本更新入口。
5. 增加真实 MariaDB 测试：跨来源相同 URL、重复采集、内容变化、身份冲突、事务回滚、并发写入；必须调用正式 Application 用例，不得在测试中复制身份解析/版本写入流程。
6. 此项已实施：移除 `repositoryIngestHarness`，通过真实 `StaticSourceCatalog`、`CollectSource.Execute` 和 `IngestCollectedSource.Execute` 驱动 MariaDB；仅用固定 RSS 响应替代外部网络。

完成门槛：由正式 Application 用例驱动 MariaDB；跨来源相同 URL 并发只产生一个主体；主体/身份/Observation/版本要么全部提交，要么全部回滚。

当前 MariaDB Application 测试覆盖：
- 同一来源并发采集与身份复用；
- 不同来源并发采集相同规范 URL，全局只创建一个主体并保留两条来源身份；
- 同一来源原生 ID 后补充规范 URL，主体身份更新但内容版本数不增加；
- 同一来源原生 ID 对应不同 URL 时返回身份冲突；
- 同一最终幂等键相同载荷重放；最终幂等键载荷冲突由 Application 单元测试 `TestIngestCandidateRejectsIdempotencyKeyPayloadReuse` 覆盖。`IdempotencyPrefix` 是批次前缀，不是最终幂等键；内容变化会生成新的最终幂等键，不应错误地期待冲突；
- 并发内容变化追加版本；
- RSS description 经真实 Parser 和授权层级过滤后，同时持久化到 `news_items.content_json` 与最新 `news_item_versions.content_json`，且两者内容一致；
- 通过制造版本主键冲突，验证正式 Application 在主体、身份、Observation 已写入后失败时整笔事务回滚。

真实 MariaDB 11.4 验收已通过：Go test suite Run `45595639304` 的 News MariaDB Job `139140906585` 执行 `TestNewsMariaDBApplicationIntegration`、物理 Schema、Retention、Availability PIT、并发决策和策略版本相关用例，全部 PASS。该测试由正式 `StaticSourceCatalog`、`CollectSource.Execute`、`IngestCollectedSource.Execute` 和真实 MySQL Repository 驱动，不复制业务算法。

完整验证也已通过：Go test suite Run `45595639304`（根模块 go test、Race、Vet、News MariaDB 和其他模块测试全部成功）；Integration CI Run `45595639470`（News/Research/Researchflow 跨模块、Worker Bootstrap MariaDB/Redis、Compose、WebUI 检查全部成功）。验收代码 SHA：`c44a262db5544961b35cf910c22ecbac823f735c`。P0-3 达到完成门槛，标记 DONE。

### P0-4：Feed 生产自动采集闭环

状态：IMPLEMENTED，待本轮最终验收

#### 生产配置与启动装配

- News Config 使用可选环境变量 NEWS_FEED_SOURCES_JSON 读取 1..32 个只读可信 Feed Source/Endpoint 注册。
- 每个注册必须明确 Source ID、Endpoint ID、URL、Publisher、访问方式、URL Scheme/Host/Port Allowlist、响应大小上限、Source Policy 版本、当前版本前置条件和授权证据。
- 配置不允许未知字段、重复 Source/Endpoint、冲突 Source Policy、未授权的内容/存储路径或 Allowlist 外 URL。
- 配置缺失时不启用 Feed；配置存在但无效时 Server 启动失败。
- Server 组合根复用共享 MariaDB Repository 和 Platform HTTP Client，装配 StaticSourceCatalog、Platform Endpoint Transport、CollectSource、IngestCollectedSource。
- Source Policy 通过现有 News Policy Version 表和条件指针更新接口发布。相同当前版本/相同载荷幂等成功；显式 expected-current 前进成功；旧配置回退失败。

#### Researchflow 执行路径

- News-owned Stage Code：news.collect_feeds。
- Workflow Template：news.feed_collection@1.0.0。
- Stage Output Type：news.feed_collection@1.0.0。
- Feed-enabled News Output Registry：1.2.0；组合后 Search Workflow Template 使用新版本 1.2.0，避免相同 Registry Version 对应不同内容。
- Feed Stage 只接受 Run ResourcePolicy 中的 Source/Endpoint 注册快照；引用包含完整注册 SHA-256 指纹。配置变化后，旧 Run 不会静默执行新 URL 或新策略。
- 正式入库只走 CollectSource.Execute 和 IngestCollectedSource.Execute，底层仍是统一 MariaDB 事务；不复制入库算法。
- StageOutput 只保存 Source/Endpoint、结果状态、计数、News ID 和原因码；不保存文章正文、原始 URL 或响应内容。
- 正式入库幂等前缀稳定为 Run/Stage/Source/Endpoint，跨 Attempt 重试不改变幂等键。
- 全部失败来源均为可重试且无跳过来源时，Stage 进入 Researchflow RetryWait；部分成功但存在不可重试/跳过来源时保留 Degraded 结果。
- 使用现有 Researchflow Dispatcher、Output Registry、Workflow Template、Scheduler、Worker Pool 和生命周期；不新增 News Scheduler/Queue/Worker/Orchestrator。

#### Diagnostics 与测试

- Feed 已注册 Source/Endpoint 诊断和当前 Source Policy 诊断。
- 已增加 Config 严格解析、资源快照指纹、Stage 输出一致性、部分失败/重试/取消、Workflow Template 和组合注册测试。
- 新增真实 MariaDB 测试：Feed RSS → 正式 Parser/CollectSource → IngestCollectedSource → MySQLRepository；验证正文授权过滤、News 主体/版本/Observation 持久化和 StageOutput 不泄露文章内容。
- 新增 Server/Researchflow 组合 MariaDB 测试：验证 Workflow Template 发布、Dispatcher 注册、Run ResourcePolicy、Worker Runtime 构造、策略前进、同版本重复启动幂等和旧配置回退拒绝。
- Go Test Suite 工作流已加入 Feed Stage 与 Server/Researchflow 组合两项 MariaDB 测试的精确 PASS 门禁；尚待本轮统一 CI 执行。

#### 自动化边界

当前 Researchflow Runtime 只会自动调度已持久化的 Run，不会按周期自动创建 Run。Feed Stage 会在 news.feed_collection Run 创建后自动执行；当前不包含无人创建 Run 的周期轮询能力。若要实现持续定时采集，需要在 Researchflow 增加通用周期触发能力，并明确采集间隔、时区、错过周期补跑和并发去重规则；不应在 News 内另建 Scheduler。

完成门槛：Feed 配置和 Server/Researchflow 生产组合真实运行；策略当前指针不可回退；正式采集走统一事务；MariaDB、Go Test、Race、Vet 和跨模块 Integration CI 全部通过。

### P1-1：职责清理

状态：IN_PROGRESS（旧关系/身份历史/双阶段写入代码已删除，待残留检查）

1. 移除 News 证券/行业关系断言职责。
2. 删除转载关系 Domain/Application/Repository/表。
3. 删除身份状态历史能力。
4. 删除双阶段写入 API、Port、DTO 和测试。
5. 确保 Observation Retention 不处理 Researchflow StageOutput。

### P1-2：全量回归与验收

阶段性状态：P0-2/P0-3 对应的 News MariaDB、根模块 Go Test/Race/Vet 和 Integration CI 已全部通过；P0-4 完成后必须重新执行最终全量回归。

1. News 全量测试、Race Detector、Vet。
2. MariaDB Migration、Schema、唯一约束、事务、并发和回滚测试。
3. Research/PIT/Researchflow/Quant 消费者回归。
4. Server 启动装配和 Feed 端到端验收。
5. 更新当前状态和任务历史，记录精确 SHA 与 CI Run。
6. CI 未完成不得标记 DONE。

## 6. 不实施的能力

- 不兼容旧 News Schema、旧数据或旧写入路径。
- 不迁移旧数据、不双写、不建兼容视图。
- 不按标题/正文相似度或事件相似度合并。
- 不做跨站转载自动识别或复杂重定向追踪。
- 不建设 News 专属 Scheduler、Queue、Worker、Orchestrator。
- 不开发人工导入。
- 不由 News 负责证券/行业关系。
- 不新增没有真实调用方的管理 API、目录数据库或生命周期子系统。

## 7. 会话恢复

继续任务时先核对 `repair_all` 最新 HEAD，再直接从当前未完成阶段继续。P0-1 已关闭，不得重复审计。当前从 P0-4 Feed Server 生产装配开始。P0-2/P0-3 已完成，不得重复 Schema 审计或统一入库测试方案；只在 P0-4 新增真实生产装配与 Researchflow 调用路径。只操作 `repair_all`，按功能组提交，中文文档/注释/提交信息。
