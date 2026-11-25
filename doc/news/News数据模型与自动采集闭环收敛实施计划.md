# News 数据模型与自动采集闭环收敛实施计划

更新时间：2026-11-25  
仓库：`Kasign/stock_data_provider`  
执行分支：`repair_all`  
计划基线：`8e8a926ff92e0136acfe9b0859ab6f64528ddf76`  
任务编号：`NEWS-POST-08`  
状态：`IN_PROGRESS`

## 1. 目标与范围

一次性收敛 News 新闻主体、URL 去重、来源身份、内容版本、正式入库事务和自动采集生产闭环。以个人研究系统的实际需要为准，不建设商业新闻平台能力，不为假设性场景增加抽象层。

允许清理 News 旧表和旧数据；不做旧数据迁移、旧逻辑兼容或双写。数据库清理由用户在最终 Schema 确认后手动执行，本任务不远程删除数据库数据。

最终要求：

1. `news_items` 保存当前可直接读取的完整新闻，不再是只有 ID 的空壳。
2. 完全相同的规范 URL 全局只对应一条新闻，不受来源 ID 不同影响。
3. 不同 URL 默认是不同文章，不按标题相似、事件相同自动合并。
4. 来源文章 ID 只在来源范围内识别身份；多个来源身份可以指向同一新闻主体。
5. 普通采集只走一个正式入库用例，主体、来源身份、Observation 和必要版本在一个 MariaDB 事务中完成。
6. 同一内容重复采集不重复创建新闻或版本；内容实质变化才追加历史版本。
7. RSS/Atom 有真实生产装配和调用路径；Provider 搜索候选只有被研究流程选中后才正式入库。
8. News 不负责新闻与证券/行业的关联。
9. 不增加复杂转载识别、重定向追踪、标题相似度合并、News 专属 Scheduler/Queue/Worker/Orchestrator。
10. Research/PIT 仍能按版本读取历史内容和可用时间证据。

## 2. 最终领域规则

### 2.1 新闻主体与来源身份

- 一条新闻表示一篇独立发布的文章，不表示一个新闻事件。
- 同一篇文章从多个来源入口被发现，只保留一个新闻主体，保留多个来源身份和采集记录。
- 同一新闻事件被不同媒体分别撰写，属于不同新闻。
- 不同网站以不同 URL 转载同一内容，默认作为不同文章保存；暂不自动合并或自动建立转载关系。
- 完全相同的规范 URL 是全局去重依据，不允许被 `source_id` 限定。
- 不同 URL 默认不合并；不做重定向链追踪、跟踪参数语义分析或跨站转载推断。
- 来源文章 ID 作为来源内身份，唯一范围为 `source_id + source_native_id`。
- URL 缺失时，允许凭可信来源文章 ID 入库；URL 和来源文章 ID 都缺失时不创建正式新闻主体。

### 2.2 URL

- URL 是新闻文章网络地址。
- 复用现有 Source URL Policy 对输入 URL 做安全校验和既有规范化，不另造 URL 归一化体系。
- 去重只比较最终保存的规范 URL 完全相同，不额外追踪重定向。
- 全局唯一约束放在 `news_items.canonical_url`；允许 NULL，以支持来源只提供文章 ID 的情况。
- `news_identifiers` 不承担全局 URL 唯一职责，只保存来源原生文章 ID。
- Hash 命中后仍需比较原始值，不能把 Hash 相等直接当作 URL/ID 原文相等。
- URL 与来源原生 ID 分别命中不同主体时，返回明确身份冲突，不自动合并两个既有主体。

### 2.3 当前内容与版本

- `news_items` 保存当前标题、摘要/正文内容层、发布者、作者、发布时间、规范 URL、内容指纹及创建/更新时间。
- 当前新闻查询不需要先关联版本表才能取得标题和内容。
- `news_item_versions` 作为历史快照保留，供 PIT/Research 历史回溯；不是当前新闻内容的唯一存储入口。
- 首次入库创建初始版本；指纹与当前版本相同不增加版本；内容实质变化时更新主体并追加版本。
- 采集时间、观察时间、Provider 变化本身不构成内容版本变化。
- 正文和摘要允许为空；有效元数据不能因正文为空而丢弃。
- 时间统一按 UTC 存储，保留真实来源精度，不伪造时分秒。
- 不新增独立正文清理系统。通过单条新闻去重、Observation 候选 TTL 和现有响应大小限制控制数据量；不先增加一组没有真实调用方的保留配置。

### 2.4 Observation 与采集入口

- Observation 表示一次发现/采集记录，不长期保存完整新闻正文副本。
- Observation 只保留身份、来源、采集时间、策略决策、必要质量/诊断字段和短期候选载荷。
- 候选载荷复用现有 Observation Retention 清理；必要追溯元数据保留。
- RSS/Atom 采集成功后直接走正式入库。
- Provider SearchCandidates 结果继续保留在 Researchflow StageOutput；只有研究流程选中/确认的候选才进入正式 News 入库。
- 不开发人工导入功能。

## 3. 数据模型调整范围

| 表 | 目标职责 | 处理原则 |
|---|---|---|
| `news_items` | 新闻主体及当前完整内容 | 扩展为当前新闻内容唯一读取入口；规范 URL 全局唯一 |
| `news_identifiers` | 来源原生文章 ID | 来源内唯一；多个来源身份可指向同一新闻 |
| `news_observations` | 发现/采集记录 | 保留必要诊断与幂等信息；不长期复制完整正文 |
| `news_item_versions` | 历史内容快照 | 保留 PIT/Research 版本语义；内容变化才追加 |

不新增 NewsContent、NewsDiscovery 等平行表。

关系和附属表处理：

- `news_relation_assertions` / `news_relation_evidence`：当前表达新闻与证券/行业的关系断言，不属于 News 采集职责。移除 News 内写入用例、Repository、表和 DTO；Research 自己负责关系分析。
- `news_item_relations`：当前表达新闻与新闻之间的转载关系。新方案不做自动转载识别；若全仓审计确认没有生产消费者，删除 Domain/Application/Repository/表。
- `news_identifier_status_history`：历史身份状态查询当前没有生产装配；本任务不保留历史身份兼容，审计确认后删除。
- Authorization Snapshot、Evidence Snapshot、Policy Version、Availability Evidence/Decision 等 PIT/授权表不因本次简化而批量删除，按真实生产调用者保留。

## 4. 实施阶段与步骤

### P0-1：全量依赖审计（当前阶段）

1. 核对 Schema、Migration、索引、外键完整清单。
2. 全仓核对每张 News 表的 SQL 读写者、Repository Port、Domain/Application 用例和生产装配点。
3. 核对 `ApplyResearchUpdate`、`IngestCollectedSource`、`IngestProviderCandidate`、`CollectSource` 的所有调用者。
4. 核对 Research、Researchflow、Quant、Server 对 News 当前内容、Version ID、Observation ID、Evidence Ref 的依赖。
5. 核对 PIT Evidence/Decision/Snapshot 是否引用 `news_item_versions`，明确历史版本最小保留字段。
6. 核对新闻-证券/行业关系与新闻-新闻关系的生产消费者。
7. 核对 SourceCatalog、SourceTransport、SourcePolicy Resolver、Server Lifecycle 和可信来源配置现状。
8. 输出表级依赖矩阵、调用链、待删除清单和最终字段来源清单。

完成门槛：所有 News 表和生产入口均有明确保留/删除决定；无基于文档猜测的调用关系。

### P0-2：最终数据模型与 Migration 重建

1. 根据 P0-1 结果定稿字段、唯一索引、外键、空值规则和内容指纹语义。
2. `news_items` 增加当前新闻内容字段及全局规范 URL 唯一约束。
3. `news_identifiers` 收敛为来源原生身份，唯一键不承担跨来源 URL 去重。
4. 保留必要 Observation、Version 和 PIT 表，删除确认无消费者的关系/历史表。
5. 同步更新 Migration、Schema Validator、Schema 测试和 Server Migration Registry 清单。
6. 不增加旧版 Schema 检测/升级兼容逻辑；最终使用空 News 库建表。

完成门槛：Migration 从空库创建最终 Schema；Schema Validator 与真实 MariaDB 表/索引/外键一致。

### P0-3：统一正式入库与全局 URL 去重

1. 建立唯一正式入库 Application 用例，RSS/Atom 与被选中的 Provider 候选共用。
2. 事务内校验可信 Source/Endpoint 和当前策略。
3. 先按规范 URL 全局查找主体；命中则复用 news_id，并追加来源身份/Observation。
4. URL 未命中时，按来源原生文章 ID 查找；命中则复用主体。
5. 两种身份分别命中不同主体时返回 identity_conflict，不自动合并。
6. 新主体写入当前完整内容、来源身份、Observation 和初始版本。
7. 已有主体内容未变时不新增版本；内容变化时更新主体并追加历史版本。
8. 所有写入在一个 MariaDB 事务中完成。
9. 定义固定锁顺序和跨实例并发策略；唯一索引作为最终防线。
10. 增加真实 MariaDB 测试：相同 URL 跨来源、重复采集、内容变化、身份冲突、事务回滚、并发写入。

完成门槛：相同 URL 跨来源并发只产生一条主体；主体/身份/Observation/版本要么全部提交要么全部回滚。

### P0-4：生产自动采集闭环

1. 复用现有 `StaticSourceCatalog` 和 Source/Endpoint Policy，不先建设数据库目录管理系统。
2. 明确可信 Source/Endpoint 配置来源；优先使用现有配置结构，只有真实缺口才增加必要配置。
3. Server 组合根装配 SourceCatalog、逐 Endpoint Platform Transport、CollectSource 和统一入库用例。
4. 将 Feed 采集接入现有 Researchflow Run/Stage/Worker 执行体系；不另建 News Scheduler/Worker/Queue。
5. RSS/Atom 采集结果直接走正式入库事务。
6. Provider 搜索结果仍作为 StageOutput 候选；被研究流程选中后调用同一正式入库用例。若审计发现当前没有真实候选选择调用点，不虚构调用者或自动将全部搜索结果入库。
7. 补 Diagnostics 注册、启动失败处理、Context 取消、网络超时和重试测试。
8. 在真实配置/网络可用时完成 Feed→Parser→统一入库→MariaDB 查询端到端验收；没有配置的来源标记 skipped/not_comparable，不伪造通过。

完成门槛：Server 真正构造并调用 Feed 生产链路，不是仅有 Adapter/单元测试。

### P1-1：职责清理与生命周期

1. 移除 News 内新闻-证券/行业关联断言职责。
2. 删除确认无生产消费者的转载关系模型及存储。
3. 删除失效的双阶段写入 API、Port、DTO 和测试。
4. Observation 仅保留短期候选载荷，复用现有 Cleaner。
5. Researchflow StageOutput 生命周期由 Researchflow 自身管理，不由 News Cleaner 处理。
6. 不新增正文 TTL、独立归档系统或复杂内容生命周期工作流；通过去重和有界采集控制数据量。

### P1-2：全量回归、文档与验收

1. 更新 News Domain/Application/Storage/Server 单元和集成测试。
2. 执行 News 全量测试、Race Detector、Vet。
3. 执行 MariaDB Migration、Schema、事务、并发和回滚集成测试。
4. 执行 Research/Researchflow/Quant 对 News 的消费者回归。
5. 执行 Server 启动装配和 Feed 端到端测试。
6. 更新本计划、当前任务状态、模块总规划、数据模型契约和任务历史。
7. 记录精确提交 SHA、CI Run 和验收结果；CI 未完成不得标记 DONE。

## 5. 当前代码基线与已确认事实

计划基线：`8e8a926ff92e0136acfe9b0859ab6f64528ddf76`。

| 位置 | 当前代码事实 | 影响 |
|---|---|---|
| Schema/Migration | 当前只有一个 V1 完整基线；`news_items` 只有 news_id、origin_source_id、created_at | 需要重建最终主体字段和唯一约束 |
| Identity | `news_identifiers` 当前按 source/type/hash 唯一；Domain 仅在同一 source 内匹配，包括 normalized-url | 必须将 URL 去重提升为全局主体规则 |
| Version | 标题、发布者、作者、发布时间、content_json、fingerprint 在 `news_item_versions` | 版本表保留为历史快照；主体增加当前内容 |
| Feed | CollectSource 已解析 ContentLayers；IngestCollectedSource 只写 Observation 元数据并清空 ProviderSummary | 正式入库缺少内容写入 |
| Provider | SearchCandidates Stage 只产生 Researchflow StageOutput；IngestProviderCandidate 是独立用例 | 不能把所有搜索候选自动写入正式库 |
| Update | ApplyResearchUpdate 在独立事务写版本，同时处理证券/行业关系断言 | 需要拆分职责并统一事务 |
| Storage | IdentityTransaction 与 ResearchUpdateTransaction 分离；LatestNewsItemVersion 锁定 news_items 和最新版本 | 需要统一事务与固定锁顺序 |
| PIT | Availability Evidence/Decision 以 version_id 绑定；Research Evidence Query 读取 Version + Observation | 必须保留版本和 Observation 的证据链 |
| Server | 已装配 Search Stage、Research News Evidence Query、Retention；没有装配 Feed Source 链路 | 需要完成生产组合 |
| Source | StaticSourceCatalog、Platform Endpoint Transport、CollectSource、策略 Resolver 已存在 | 复用现有能力，不重复造 Adapter |

## 6. 审计期间需要特别确认的事项

1. `news_items.origin_source_id` 在多来源同 URL 后是否应删除；来源集合应由来源身份和 Observation 表达。
2. 规范 URL 的现有生成规则、URL Policy 输出和身份解析调用链。
3. `news_item_versions.content_json` 的结构及 PIT 读取依赖；主体当前内容与历史快照复用同一结构。
4. 来源原生 ID 的字段约束；移除 normalized-url identifier 后的冲突规则。
5. 关系表和身份状态历史表的全仓生产消费者。
6. Researchflow 是否已有可承载 Feed Collection 的真实 Stage/触发场景；若无，增加最小 Stage 但复用现有 Run/Worker。
7. Source/Endpoint 启动配置及策略版本初始化方式。

## 7. 明确不做

- 不兼容旧 News Schema、旧数据或旧写入路径。
- 不做旧数据迁移、双写或兼容视图。
- 不自动合并不同 URL 的跨站转载。
- 不做标题相似度或事件相似度合并。
- 不追踪复杂重定向链，不分析跟踪参数语义。
- 不新增 NewsContent/NewsDiscovery 等表。
- 不新增 News 专属 Scheduler、Queue、Worker、Orchestrator。
- 不开发人工导入。
- 不由 News 负责证券/行业关联。
- 不顺手重构 Platform、Researchflow 或其他无关模块。

## 8. 后续会话恢复说明

后续会话必须先读取本文件、`当前任务状态和进度.md`、`模块总任务规划和进度.md`，然后核对 `repair_all` 当前 HEAD。不得从旧聊天摘要推断任务状态。

继续顺序：
1. 从 P0-1 尚未完成的表级依赖审计继续。
2. 每完成一组审计，在当前任务文档更新证据和下一步。
3. P0-1 完成后，更新本文件中的最终字段/删除清单。
4. Schema 决策未完成前不得提前写 Migration。
5. 每个功能组完成测试、更新文档后再提交；只操作 `repair_all`。

当前进度：P0-1 已完成核心 Schema/Repository/Research/PIT/关系表/Server Source 生产装配的定点核查；Diagnostics 和 Schema 测试清单收尾后进入 P0-2。
