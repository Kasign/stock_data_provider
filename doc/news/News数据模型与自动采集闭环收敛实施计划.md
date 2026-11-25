# News 数据模型与自动采集闭环收敛实施计划

更新时间：2026-11-25  
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

## 2. 已确认的代码依赖事实

- 当前 News V1 是单个完整基线，创建 14 张表；Schema Validator 和 Migration 测试显式校验表、索引、外键清单。
- 当前 `news_items` 只有主体 ID、首次来源 ID 和创建时间；当前内容存放在 `news_item_versions`。
- 当前 `news_identifiers` 将 normalized-url 与来源原生 ID 一起按来源范围匹配，无法实现跨来源 URL 全局去重。
- Feed Parser 已能生成 ProviderSummary、SourceSummary、Body 和附件元数据；当前 Feed Ingest 未将完整 ContentLayers 写入正式新闻版本。
- Provider SearchCandidates Stage 当前输出 Researchflow StageOutput，不自动创建正式 News。
- 当前身份写入和 ResearchUpdate 版本写入分属不同事务；`ApplyResearchUpdate` 还混有证券/行业关系断言职责。
- Research 通过 News Evidence Query 使用 NewsID + VersionID；PIT 依赖 Version、Observation、Availability Evidence/Decision。
- Server 当前已有 Provider Search Stage 和 Research Evidence Query 装配，但没有完整 Feed SourceCatalog/Transport/Collect/Ingest 生产组合。
- StaticSourceCatalog、Platform Endpoint Transport、CollectSource、Source Policy Resolver、Policy Version Publisher 已存在，应优先复用。
- StageOutput 生命周期属于 Researchflow，不由 News Cleaner 管理。

以上是本任务已完成的 P0-1 审计结论，不再重复审计；实现时只对具体字段/SQL/调用点做定点核对。

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
- `content_json`：当前完整内容层。
- `content_fingerprint`：当前内容指纹。
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
- 已有主体内容指纹不变时不新增版本；内容变化时更新主体并追加版本。
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

<badge>DONE</badge>

已完成现有表、读写者、Repository、Research/PIT 消费者、关系职责、Server 装配和 Diagnostics/Schema 测试依赖核查。

### P0-2：最终数据模型与 Migration

<badge color="warning">IN_PROGRESS</badge>

1. 逐字段对齐 Domain、Repository、PIT SQL 和 Research Evidence DTO。
2. 修改单一 V1 Migration，直接创建最终 10 张表。
3. 同步修改 Schema Validator、Schema 测试、Migration 测试和 Server Migration Registry 相关清单。
4. 删除 4 张表及其专属索引、外键、Repository 和失效用例。
5. 通过空库 Migration 和真实 MariaDB Schema 验证。

完成门槛：Migration 从空库直接创建最终 Schema；Validator 与真实 MariaDB 表/索引/外键一致。无需兼容旧 V1。

### P0-3：统一正式入库与全局 URL 去重

1. 建立唯一正式入库 Application 用例。
2. RSS/Atom 和被选中的 Provider 候选复用该用例。
3. 合并主体、身份、Observation、版本写入事务。
4. 移除独立的双阶段版本更新入口。
5. 增加真实 MariaDB 测试：跨来源相同 URL、重复采集、内容变化、身份冲突、事务回滚、并发写入。

完成门槛：跨来源相同 URL 并发只产生一个主体；主体/身份/Observation/版本要么全部提交，要么全部回滚。

### P0-4：Feed 生产自动采集闭环

1. 复用 StaticSourceCatalog 和逐 Endpoint Platform Transport。
2. 确定最小可信 Source/Endpoint 启动配置及策略版本初始化。
3. Server 组合根装配 SourceCatalog、Transport、CollectSource 和统一入库用例。
4. 采集触发与执行复用 Researchflow Run/Stage/Worker，不新增 News Scheduler/Queue/Worker。
5. RSS/Atom 结果直接正式入库；Provider 候选仍须经研究流程选中。
6. 注册真实依赖对应的 News Diagnostics。
7. 补启动失败、Context 取消、网络超时、重试和真实 MariaDB 端到端测试。

完成门槛：Server 存在真实 Feed 生产调用路径，不以构造器存在或单元测试代替生产装配。

### P1-1：职责清理

1. 移除 News 证券/行业关系断言职责。
2. 删除转载关系 Domain/Application/Repository/表。
3. 删除身份状态历史能力。
4. 删除双阶段写入 API、Port、DTO 和测试。
5. 确保 Observation Retention 不处理 Researchflow StageOutput。

### P1-2：全量回归与验收

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

继续任务时先核对 `repair_all` 最新 HEAD，再直接从当前未完成阶段继续。P0-1 已关闭，不得重复审计。当前从 P0-2 逐字段对齐和 Migration 重建开始。只操作 `repair_all`，按功能组提交，中文文档/注释/提交信息。
