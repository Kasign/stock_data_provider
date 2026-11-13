# 新闻模块开发任务与验收清单

> **Integration 接入状态（2026-11-13）**：integration HEAD 23d07e7716aa6d7cd7f0fda4d3dbfa178c148a4e 的全量 CI Run [43362987745](https://github.com/Kasign/stock_data_provider/actions/runs/43362987745) success。公共 GOV-CTR-006 JCS Runtime Verifier 与 News Platform Adapter 已进入 integration，NEWS-CTR-002 AuthorizationRef/PolicySnapshot 的 schema_version 已对齐 Golden。News 的 MySQL Repository 已具备 PIT 查询、当前策略复核、授权快照和同事务正文读取能力；Server 尚未注入完整 Evidence Query Facade 或 Research Service。完整 ListEvidence 生产装配还缺 HMAC Cursor Key Ring 的可信配置（Active Key、轮换 Key 与 15 分钟重叠窗口），不得用默认密钥或跳过签名验证绕过。下一步以 [Integration 当前任务交接](../整体任务规划/05-治理与验收/Integration当前任务交接与继续执行.md) 为准。

> 当前验收状态（2026-11-12，News 分支 HEAD：`a8a3763e5dc662d856a438c52a0b1a28975def43`）：NEWS-CTR-002 显式 `risk_review` 用途映射修复及对应测试已通过当前 HEAD 的 CI。News Provider contract workflow Run `43219930444`：Provider contract tests、完整 News 模块测试均 success，包含 gofmt、单测、Race、Vet；Go test suite Run `43219930418`：News MariaDB Repository and Migration integration、根模块 `Test .`、Researchflow 相关测试及 `Test go-core` 均 success。Compose/Docker 与 Market Data 专项 jobs 按条件 skipped，不计为通过。两条 Run 的 `head_sha` 均与当前 HEAD 完全一致。PR [#88](https://github.com/Kasign/stock_data_provider/pull/88) 的历史合并记录不代表本轮再次合并；本轮未执行分支合并。

> 当前模块状态：News 自有领域代码质量门禁、Provider 固定响应契约和 MariaDB 11.4 持久化专项验收已有通过证据；公共 JCS Runtime Verifier 与 News 适配器已在 integration 实现，并以 GOV-CTR-006 四份 Golden 为验收向量；Server 生产装配尚未注入该 Verifier，受限内容披露仍保持关闭；News 内部的策略指针锁定、授权复核与精确 Version 正文读取事务机制已实现。NEWS-CTR-001 的 available_at 严格 PIT 继续等待合格来源证据，不得从 published_at/fetched_at/observed_at 推导。
> 历史记录说明：本文按日期累积的早期实施段落保留为过程记录；其中旧的未完成项、等待 CI 项和旧阻断状态不代表当前任务状态。当前状态以本段及文末“2026-11-12 当前任务边界与后续计划”为准。
> 状态：本分支只维护 News 领域实现和专项文档；公共平台原语来自共同基线，根组合根由 Server 实施分支负责。

## 一、阶段总览

| 阶段 | 目标 | 当前状态 | 前置依赖 |
|---|---|---|---|
| N1-A | 新闻业务架构与平台边界评审 | 已评审确认 | PLAT-002 项目级契约已进入 L1 冻结；News 消费者实现符合性和 Server 生产装配仍按各自证据验收 |
| N2 | Domain、Application Port 与 Provider 协议 Adapter | 已完成（根模块 gofmt/test/race/vet 与分析服务镜像构建通过） | 注入式 Transport Port、固定样本和 fake |
| N3 | 来源授权、安全抓取、解析与标准化 | 离线能力首批完成；生产触发、CollectionRunID、授权持久化编排、逐 Endpoint 装配和真实网络验收未完成 | 离线业务能力可先行；真实网络依赖 PLAT-002/008 和 SSRF 门禁 |
| N4 | 身份、去重、版本和证券关联 | Domain/Application、MariaDB Repository、身份/版本/关系历史首批已实现；生产采集编排和完整端到端运行未完成 | fake Repository/内存替身 |
| N5 | 公共平台基础能力接入 | Config/Secret、独立 BoundDoer、Fault/Request ID 适配已实现；Server 生产装配和部署 DNS/出口验收待完成 | 项目级 PLAT-CTR 已 L1 冻结；News 消费者 A2 与生产部署 A3 仍需独立证据 |
| N6 | 新闻持久化与 Migration | V1-V8、Repository、Retention 清理和 V7→V8 既有数据升级已通过 MariaDB 11.4 专项 CI；生产共享 DB 注入、总 Migration 注册待接入 | Server 共享 DB 注入与 Migration 总注册由集成分支负责 |
| N7 | 新闻领域诊断及可选执行记录 | Provider/Parser/Source/Storage Check 与单条 BusinessQualityCheck 已实现；Runner 注册、采集闭环诊断和批量质量/Freshness 扫描待接入 | PLAT-007；PLAT-010 不建设通用队列，News 异步化按真实触发需求 |
| N8 | 跨模块集成与验收 | News 模块单测/Race/Vet 与 MariaDB 专项验收已有通过证据；A2 生产接入仍未完成 | PLAT-013 领域接入；PLAT-014 故障演练 |
| H1 | 历史 Python 数据价值评估 | 可选，暂不实施 | 单独确认价值，不属于主路径 |

## 二、N1-A：新闻业务架构与公共平台边界

### A.1 新闻领域架构
- [x] 核对 Go 新闻搜索 Adapter、分析调用和时间截止处理。
- [x] 核对 Python Bridge、Agent、Bot、个股分析、大盘复盘调用。
- [x] 核对旧 Python 三类新闻/情报表及其语义差异。
- [x] 核对证券 canonical symbol 与行业标识现状。
- [x] 确认新闻属于根 Go Module 的 internal/news，随根 Server 同进程部署。
- [x] 确认不兼容未上线旧 Python/go-core 新闻调用链，不创建共享 Provider Module。
- [x] 确认 Provider Adapter 原生归属根模块，保留 provider provenance。
- [x] 完成新闻业务实体、时间、身份、版本、关联证据和 Application 契约设计。
- [x] 完成 Provider/官方来源公开条款核查，未确认授权统一保持 pending。
- [x] 完成 N1-A 架构评审报告。

### A.2 公共平台对齐
- [x] 只读核对 feat/platform-foundation 的架构、路线图、PLAT-001 盘点和 PLAT-002 契约评审稿。
- [x] 明确 Platform 提供横切基础能力，News 保留领域所有权。
- [x] 明确 News 保留 Repository、SQL、领域表和 Migration 内容；PLAT-009 是否抽取公共 Storage/Migration package，以最终契约和真实调用方为准。
- [x] 明确 Redis、Task、Metrics、Audit 均非新闻首期强制依赖；PLAT-004 动态 Service Key 暂缓，新闻同进程调用不依赖它。
- [x] 明确新闻同进程 Application 当前不需要 Service Key 或新闻专属认证路由。
- [x] 形成《新闻模块与公共平台接入矩阵》。
- [x] 向平台分支提出 HTTP/SSRF、Diagnostics、Fault 三项契约评审；平台已在 98f561e 接受并修订文档，后续转为实现与测试门禁。
- [x] 用户评审并确认 N1-A；进入 N2。

## 三、N2：Domain、Application 与 Provider 协议 Adapter

- [x] 建立独立新闻 Domain 模型，不复用分析消费 DTO 作为持久化实体。
- [x] 定义最小 Provider/Searcher Port 和 ProviderOutcome。
- [x] 将 Bocha、Tavily、Brave News、SearXNG 的首版协议处理迁入根模块 Adapter；不复用旧 Searcher 聚合、缓存和 fallback。
- [x] 统一候选字段、Provider provenance、发布者身份、原始时间文本和字段缺失语义；发布者域名不从结果 URL 猜测。
- [x] 明确区分 ProviderItemID（搜索结果 ID）与 SourceItemID（发布来源原始 ID）；只有 SourceItemID 可参与来源内身份凭据，Feed author 独立保留，不作为 PublisherName。
- [x] 保留发布时间未知但可待核实的候选；Brave page_age 不冒充发布时间。
- [x] 明确 Provider 部分失败、空结果、跳过和技术重试候选语义；未知错误默认不可重试，不自动触发重试。
- [x] Provider Adapter 只依赖注入式窄 Transport Port；不创建生产 HTTP Client、连接池、通用重试器或通用限流设施。
- [x] 补充固定响应样本和 fake Transport 测试代码，覆盖 Provider 鉴权头、字段映射、空时间、page_age、429 分类和响应结构。
- [x] 对 Bocha、Tavily、Brave、SearXNG 缺失结果字段/信封的响应按协议异常处理，避免把上游结构变更误判为空结果；已补充 Bocha、Tavily、SearXNG 回归测试。
- [x] Provider endpoint 拒绝 URL 凭证参数；SearXNG endpoint 使用 URL Path 规范化，保留已有 query 参数并避免将 `/search` 错拼到 query 中。
- [x] 执行根模块 gofmt、go test ./...、go test -race ./...、go vet ./... 和分析服务 Docker 镜像构建；提交 `8b6850e2254d4fbc89dfe0c57207c8312c5ec7aa` 对应 Job 全部通过。
- [x] PLAT-002 未冻结前不导入平台内部实现；PLAT-008 未可用前不做真实 Provider 网络验收。

### N2 第一批实现记录

- 代码提交：`764e315493eb534779d4a677248c6ea630ffccac`（建立初版 Domain/Application/Provider Adapter）。
- 本轮继续修正：ProviderOutcome 聚合规则、未知错误默认 retryable=false、Provider 业务错误分类、Brave page_age 语义、Bocha 成功码、Tavily Bearer 鉴权、SearXNG endpoint 规范化。
- 当前只完成协议适配与内存结果，不代表真实 Provider 可用、来源授权已批准或新闻数据可持久化。
- Transport 响应体大小、总 Deadline、连接/TLS、重定向和 DNS/IP 安全由公共平台 HTTP Client 承担；News 不另建通用网络设施。

## 四、N3：来源授权、安全抓取、解析与标准化

- [x] 实现 Provider 输出与 Source/Endpoint 授权策略分层评估；区分请求内处理、跨请求缓存、持久化用途和保留期限，并取各层许可交集。真实采集/留存仍受来源策略和平台 HTTP 安全门禁约束。
- [x] 实现离线 URL 规范化，仅允许 HTTP(S)，并校验精确主机和端口 allowlist；DNS/IP 特殊地址、DNS rebinding 和实际 Dial 校验仍由 PLAT-008 负责。
- [ ] 对初始 URL 及每次重定向重新校验 DNS/IP、私网/保留地址和授权范围，防止 DNS rebinding。
- [x] 实现 RSS 2.0 / Atom 1.0 离线 Feed 解析，提取 ID、标题、链接、摘要、正文片段、作者和发布时间；无时区/无法解析的时间保留原文并标记缺失，不伪造精确时间。
- [x] 实现离线 HTML 解析：提取 article/main/body 文本、标题/摘要/规范链接/发布时间元信息，剔除脚本、导航等非正文节点，并支持声明字符集转换。
- [x] 建立 SourceCatalog/SourceTransport/CollectSource 应用层在线 Feed 采集编排与 fake Transport 测试；不依赖平台实现，不开放生产网络。平台 BoundDoer 按 endpoint 注入及生产 DNS/出口验收仍未完成。
- [x] Feed 发布时间与更新时间分开保留；HTML 解析支持 Content-Type 字符集转换，发布时间仅在时区明确时解析。
- [x] Feed 附件元数据映射为独立 Domain ContentAttachmentMetadata；附件异常通过新闻业务质量原因码表达，不混入正文或直接判定 Feed parse failure。
- [x] 补充附件元信息与内容载荷类型的来源适配规则；附件保持独立载荷层，不推断为正文。
- [x] 增加统一候选标准化：文本空白、Provider provenance、采集时间、查询维度、明确时区发布时间和缺失字段顺序统一；重新分配缺失字段切片，避免改写调用方数据；不从文章 URL 推断发布者域名。
- [x] 定义独立的 Provider 搜索摘要、Source 摘要与正文载荷层；显式记录载荷来源、格式和可选内容分类，未知分类不推断为全文。
- [x] 将 RSS/Atom 与 HTML 解析结果映射到分层载荷；Feed 正文格式未知时显式标记 unknown，HTML 正文明确为 plain_text，不推断全文分类。
- [x] 新增 FeedEntry.ToCandidate 映射：Feed ID/GUID → SourceItemID；author → Author；发布者名称/域名必须由已登记 Source 配置显式提供；不从 Feed 作者或文章 URL 推断发布者身份。
- [x] 实现离线业务质量评估：元数据、内容、来源身份分别评估；区分 valid/degraded/invalid/unknown，并保留稳定 reason code。
- [x] 确保 Provider 搜索摘要不能替代来源摘要/正文；仅有发布者名称不能判定来源身份已解析。
- [x] 补充 RSS/Atom 内容类型识别、附件元信息及附件元数据质量规则；正文 ContentKind 仍不推断。
- [x] 完成 Feed Parser → Candidate → ContentLayers → BusinessQuality → ResearchUpdate 的跨层离线端到端验收（固定 RSS/Atom 样本，覆盖来源 ID/作者/发布者隔离、正文/摘要/附件分层、附件质量降级、Atom 外部正文引用仅保留元数据不访问网络、身份 Observation 和版本追加）。
- [ ] 真实网络请求使用公共 HTTP Client；Provider 与每个 Source endpoint 分别绑定独立 BoundDoer/DestinationPolicy。响应体上限、总 Deadline、连接/TLS 和通用 Retry Policy 不由 News 重复实现。
- [ ] Provider 配额、业务并发和来源访问策略由 News 提供给公共 Client/调用编排。
- [ ] News 尚未完成按 endpoint 的 BoundDoer 注入和来源授权验收前，不开放任意来源真实抓取；平台生产 DNS/出口 SSRF 验收仍未完成。
- [ ] 未获准留存的 Provider 输出/正文不持久化；授权不明的来源保持 skipped/restricted。
- [x] 固定样本覆盖 RSS/Atom、空 Feed、缺失字段、无时区时间、UTF-8 BOM、无效 XML、不支持格式、HTML 正文提取/编码转换、摘要正文分层映射、重复段落保留和业务质量状态边界；超大响应和重定向仍需结合平台 HTTP Client 验收。

## 五、N4：身份、去重、版本和证券关联

- [x] 实现来源原始 ID 与规范 URL 身份凭据；URL 先通过 HTTP(S)/Host/Port allowlist，再规范化。
- [x] 实现 SHA-256 Hash 命中后的原文核对、同来源多主体冲突检测和跨来源隔离。
- [x] 身份解析区分同来源重复命中与跨来源隔离；跨来源转载不自动合并。
- [x] 建立候选身份处理 Application 流程：同事务读取凭据、解析身份、创建主体/保存凭据并追加 Observation。
- [x] 无身份凭据候选只追加 Observation；身份冲突候选只追加冲突 Observation，禁止创建新闻主体。
- [x] 补充 Observation 幂等键、跨来源转载关系与内容版本联动；转载证据分别锚定双方版本，任一侧修订后标记 stale/unversioned，等待重新核验。
- [x] 实现纯 Domain 内容版本指纹：纳入来源标题、发布者元信息、发布时间、来源摘要和正文载荷；排除 Provider 搜索摘要、采集时间和查询上下文。
- [x] 固定测试覆盖采集时间变化不产生新指纹、Provider 摘要变化不产生新版本、来源摘要/正文修订产生新指纹及时间规范化。
- [x] 将版本指纹接入 Application/Repository 事务流程；指纹未变化复用当前版本，变化时追加单调递增版本。
- [x] 定义证券/行业关系断言：证券身份必须包含 market + symbol；行业身份包含分类体系及代码/名称快照。
- [x] 定义来源明确、标题匹配、正文匹配、规则、人工和模型六类关联方式及对应证据类型。
- [x] 模型推断仅允许 proposed；确认需形成独立人工确认断言，保留原始 model 方法与证据，不伪装为 source_explicit。
- [x] 定义追加式修正/撤销链和当前关系计算；校验父断言、同新闻归属、单一直接后继及重复/断裂链。
- [x] 将关系断言与证据接入 Application/Repository 同一事务；证据引用自动补齐当前内容版本或 Observation。
- [x] 补充证券代码变更、退市生命周期和行业分类版本管理纯领域规则。
- [x] 固定测试覆盖代码变更、独立简称变化、退市历史查询和跨市场同码。
- [x] 使用固定领域样本验证身份凭据构造、历史 URL 命中、无效凭据忽略、Hash 原文核对和多主体冲突；不实现 SQL、MariaDB 事务、Redis 或 Migration。

## 六、N5：公共平台基础能力接入

触发条件：Platform 原语已在当前基线；项目级 PLAT-CTR 语义已 L1 冻结，可按冻结定义完成 News 适配。L1 不代表消费者 A2 或生产 A3 通过；仍须按实际证据验收。

- [x] Config/Secret：News 定义独立配置结构，复用 Platform Env/Secret；真实仓库根 Go 测试已通过，Provider 凭证不存入新闻表或日志。
- [x] Error/Request ID：Provider 与 Source 适配器映射 Platform Fault，保留稳定 Code/Retryable，不透传 Cause；Request ID 由 Platform Client 从验证过的 Context 传播。Logger/Trace 结构化字段接入仍待验收。
- [x] GOV-CTR-006 Runtime Verifier：integration 的 `internal/platform/authorization` 使用 RFC 8785 JCS + SHA-256 重建并验证完整策略快照指纹；News 通过 `internal/news/adapters/platform` 实现窄 Port 适配，AuthorizationRef 与 PolicySnapshot 均保留 `schema_version=1.0.0`。四份项目 Golden 和 News 绑定 Golden 已纳入运行时测试；Server 注入与最新 SHA CI 通过后才可解除受限内容披露门禁。
- [x] HTTP Client 模块适配：Provider/Source 均已新增 Platform Client + DestinationPolicy 适配器；生产组合根注入和公网 DNS/出口验收仍待完成。
- [ ] Security：当前同进程 Application 不引入 Service Key；未来既有 HTTP API 暴露新闻操作时，由该 API 统一认证和 Scope 授权。
- [ ] 不导入平台未冻结的内部实现路径；公共契约变更通过版本和接入评审处理。

## 七、N6：新闻持久化与 Migration

触发条件：Platform Storage/Migration 代码已合并；News 可实现自身 Repository/Migration，但根共享连接注入和总注册由 Server Owner 完成。

- [x] News MySQLRepository 借用共享 `*sql.DB`，实现 Identity / ResearchUpdate / NewsItemRelation 三类 Repository Port；连接池生命周期仍由 Server 持有。
- [x] News V1 Migration DDL 已由 News 定义，使用 Platform Migration Runner 的 news namespace；Runner 注册/执行仍由 Server 集成负责。
- [ ] 根服务集成分支负责 Migration 注册入口和启动装配；News 分支不与其争抢全局入口。
- [ ] 不把新闻 Repository 或新闻表交给 Platform 实现；不创建 news_schema_migrations 私有账本。
- [ ] MariaDB 是新闻持久化首选；Redis 不作为首期必选依赖，只有出现跨进程协调、分布式限流/租约等明确需求后才评估。
- [x] MariaDB 11.4 集成已验证身份并发首次创建、Observation 同 key 同/异载荷、事务回滚、版本并发追加、跨新闻转载关系写入/读取；查询分页稳定性尚未进入该 Repository 范围。
- [x] 验证 News 空库初始化与重复执行；Platform Migration Runner 的通用失败状态/恢复保护已有独立集成覆盖。
- [ ] News schema 升级版本与失败 DDL 恢复的专属演进测试。
- [ ] 历史 SQLite 导入仅在单独批准后实施；默认不导入。

## 八、N7：公共诊断及可选 Task/Audit

- [x] 已提供 Provider 配置、Source Endpoint readiness、RSS/Atom Parser、Storage connection/schema 的 Platform Check 工厂；不自建 Runner、Registry、诊断 API 或诊断历史。
- [ ] 由 Server 组合根注册到共享 Runner；采集、身份与数据质量 Check 随在线采集闭环补齐。
- [ ] 新闻质量 valid/degraded/invalid/unknown、Freshness 和授权状态仍由 News 定义；平台 Status 只表达检查执行/能力状态。
- [x] 首批 Check Attributes 仅包含配置状态、Feed 格式/fixture 数量、Source access_method/策略版本状态、Schema 缺失表名；不携带正文、snippet、Token 或原始响应。
- [ ] PLAT-010 仅在需要定时采集、长耗时后台执行或进程崩溃恢复时接入；同步用例不强行异步化。
- [ ] 平台 Task 只保存执行生命周期元数据；Observation、内容版本和关系证据仍由 News 拥有。
- [ ] Audit 当前没有独立 PLAT 任务编号；只有未来出现来源策略管理等敏感管理操作时，再与平台分支评估最小审计事件接口，不预建新闻审计表。

## 九、N8：跨模块集成与验收

- [ ] 按 PLAT-013 领域接入任务完成跨模块契约、共同基线、依赖路径、DI 和 Migration 注册验收。
- [ ] 运行新闻单测、契约测试、平台适配测试和根服务集成测试。
- [ ] 对已获授权 Provider/Source 执行受控真实验收；未核实授权的能力保持 skipped/restricted。
- [ ] 输出分层验收报告：技术可用性、业务质量、授权状态和持久化状态分别呈现。
- [ ] 按 PLAT-014 计划执行必要故障演练；不等待全部 P2 条件任务才开始纯业务开发。

## 十、H1：历史 Python 数据价值评估（可选，当前不实施）

- [x] 不建设旧 Python DTO 转换、Bridge fallback、Agent/Bot 兼容或调用方灰度切换。
- [x] 历史 Python 旧表语义盘点保留为参考资料，不要求实现在线迁移。
- [ ] 只有确认历史数据具有明确业务价值后，才另立一次性导入任务。
- [x] 不做运行时双写，不引入旧库兼容 Repository，不以旧表结构约束新领域模型。
- [x] 不删除旧 Python 仓库或其服务。

## 十一、全程验收门槛

- 不依赖新闻模块自建 DB/Redis 连接池、通用 HTTP Client、通用诊断框架或诊断历史。
- 新闻 Repository、SQL、领域表和 Migration 内容由 News 拥有；DB 连接生命周期与 Migration Runner 优先复用根服务已有资源，是否抽取公共 package 以 PLAT-009 最终契约为准。
- Domain/Application 不依赖 SQL、Redis、HTTP 实现或诊断存储。
- 不新增新闻独立 HTTP Handler、路由、认证或部署入口。
- Provider Adapter 归 News 所有；只迁移必要协议处理，不复用旧 Searcher；通用 HTTP 传输策略归公共平台。
- 不伪造发布时间、来源、正文、授权或关联证据。
- 业务质量规则保留，通用诊断执行框架由公共平台提供。
- 不为旧 Python/go-core 链路新增兼容层；不擅自删除其他模块旧服务。
- 固定样本和 fake 测试可重复，不能把测试替身描述成真实服务验证。
- 文档、代码、任务状态一致；未验证事项明确标记。


## 十二、2026-11-05 最新公共平台规划对齐修订

只读核对 feat/platform-foundation 检查 HEAD ac06a544db2e7bf0b1841def745fecfe5f9331e8 后，修正以下执行依赖：

- PLAT-003 使用 owner Token + 每调用方独立环境 Secret；PLAT-004 动态 Service Key 为 P2 条件任务并暂缓。新闻同进程 Application 不需要 Service Key。
- 本条为旧 Platform 路线图快照，已由 2026-11-06 最新编号取代：PLAT-011 Metrics、PLAT-012 安全审计、PLAT-013 领域接入、PLAT-014 故障演练。
- 本条为平台 Storage 实现合并前的历史规划快照。当前 `internal/platform/storage` 已提供 SQL/Redis 生命周期与 namespace Migration Runner；News 实现自己的 Repository/SQL/Migration，Server 组合根负责共享资源注入和总注册。
- 真实网络请求仍以 PLAT-008 的安全传输契约为门禁；DNS/IP 校验必须绑定实际 Dial，不能只做一次独立 DNS 查询。
- 平台设计状态与实现状态分离，接入必须以代码、测试和集成证据验收。


## 十三、N2 验证记录与 N3 启动基线（2026-11-05）

- N2 根模块验证提交：`8b6850e2254d4fbc89dfe0c57207c8312c5ec7aa`。
- 根模块 gofmt、`go test ./...`、`go test -race ./...`、`go vet ./...` 均通过；分析服务 Docker 镜像构建通过。
- 全仓工作流仍有与新闻无关的失败：`services/market-data/internal/service/provider_probe_live_test.go` gofmt 不通过，导致该子模块测试和行情镜像构建未执行；本分支未修改该数据模块文件。
- MariaDB 行情集成 Job 通过，但这不构成新闻持久化验收。
- 已在本分支启用 push CI；独立镜像构建步骤不会因行情镜像失败而跳过分析服务镜像构建。
- 只读核对平台分支最新 HEAD `ac06a544db2e7bf0b1841def745fecfe5f9331e8`：PLAT-007 已有 Diagnostics 首批实现提交；PLAT-002 仍待评审，PLAT-008 尚未实现。News 继续只做离线 Domain/解析能力，不导入平台内部包、不执行真实来源抓取。

### N3 首批纯领域授权规则

- [x] N3 离线来源 URL 规则：仅允许 HTTP(S) 且必须同时命中 Scheme allowlist；精确主机匹配、不隐式放行子域名、默认端口规范化、非默认端口显式 allowlist、拒绝 userinfo 和其他协议；不执行 DNS 请求或实际网络连接。
- [x] N3 Provider 输出权利与 Source/Endpoint 许可分层评估：区分请求内处理、跨请求缓存和持久化；Brave/Bocha 等 Provider 可通过策略明确限制持久化；各层用途和保留期限按交集执行。

- [x] 首版 Provider 输出授权与 Source/Endpoint 授权分层评估；Provider 的请求内处理、跨请求缓存、持久化用途及各自保留上限独立判断，并与 Source/Endpoint 取最严格交集。
- [x] Source 是默认规则；Endpoint 仅能收紧，nil 表示继承，显式空集合表示禁止。
- [x] Pending/Revoked 默认拒绝；Approved/Restricted 仍必须逐项满足访问方式、用途、内容类型、存储方式和保留期限。
- [x] Endpoint 扩权没有独立书面授权实现前一律拒绝。
- [x] 策略评估只返回新闻业务 reason code，不创建平台 Diagnostics Observation/Runner/历史记录。
- [x] 校验内容类型与存储模式匹配，避免将全文载荷伪装为 metadata/excerpt 保存。


## 十四、N3 首批授权与 URL 规则验证记录（2026-11-05）

- 验证提交：7e55cf681f86c762aaa3d2e6a914495c162d2898（GitHub Actions run 41993324250）。
- 根模块 gofmt、go test ./...、go test -race ./...、go vet ./... 全部通过；分析服务 Docker 镜像构建通过。
- 固定测试覆盖 Source/Endpoint 权限交集、Endpoint 扩权拒绝、Pending/过期策略拒绝、Provider 请求内处理/缓存/持久化区分、Provider 内容类型和保留期限限制、内容类型与存储模式不匹配拒绝、URL 精确主机/端口/IPv6 规范化及非 HTTP(S) 协议拒绝。
- 全仓工作流仍因 services/market-data/internal/service/provider_probe_live_test.go gofmt 不通过而失败；该子模块测试与行情镜像构建被跳过/失败，本分支没有修改该数据模块文件。
- 当前没有真实 DNS、HTTP、重定向或 Provider 请求测试；PLAT-008 未实现，不能将离线 URL allowlist 等同于 SSRF 防护已完成。


## 十五、N4 候选身份 Application 流程（2026-11-05）

- [x] 新增 Observation 领域记录，保留来源、Provider、查询维度、标准化候选、采集时间、身份状态、主体引用和原因码。
- [x] 新增 IdentityRepository 事务端口：读取身份凭据、创建新闻主体、保存凭据和追加 Observation 必须在同一事务中完成。
- [x] 已匹配候选可为既有主体补充新的来源内身份别名；匹配到多个主体时返回冲突，不创建主体、不写入新身份凭据。
- [x] 无原始 ID 且无 URL 的候选只追加 unidentified Observation，不创建 news_item。
- [x] URL 不符合来源 allowlist 时保留拒绝 Observation，不创建主体。
- [x] 使用内存事务替身覆盖新建、命中、补充别名、无身份和多主体冲突路径；该替身不代表 MariaDB 并发/事务验收。
- [ ] 后续 Repository 实现必须保证事务隔离、身份唯一约束冲突处理和 Observation 幂等；当前未实现 DB/Redis。
- [x] Observation 使用上游稳定幂等键；相同键相同候选载荷返回既有记录，相同键不同载荷拒绝，并纳入同一身份事务。
- [x] 跨来源转载保留独立新闻主体，以 repost_of 关系连接；拒绝同来源伪转载、重复边和循环转载链。
- [ ] 版本指纹与追加式版本流程接入仍在 N4 后续批次。


## 十六、N4 内容版本指纹首批实现（2026-11-05）

- [x] 使用 SHA-256 和长度前缀字段编码生成稳定指纹，避免简单字符串拼接的字段边界歧义。
- [x] 纳入标题、发布者元信息、发布时间（优先规范化 UTC 时间；未知时间保留原始文本）、来源摘要及其来源标记、正文来源/格式/内容分类/文本。
- [x] 不纳入 Provider 搜索摘要、采集时间、Provider、查询维度和 Observation 上下文；纯重复采集不产生新版本。
- [x] 规范化标题/摘要空白和发布时间时区；正文仅统一 CRLF 换行并保留段落内容。
- [ ] 后续 Application/Repository 需比较当前版本指纹，仅在变化时追加版本；本批不实现持久化和历史版本查询。


## 十七、当前提交验证记录（2026-11-05）

- 当前代码基线：`b5796bab9035706cc6baf64d5104edaa96becef5`；后续若有新提交，以新 SHA 的独立 CI 结果为准。
- 该 SHA 对应 Workflow run `42002648064`：根模块 gofmt、`go test ./...`、`go test -race ./...`、`go vet ./...` 全部通过；分析 API Docker 镜像构建通过；MariaDB 行情集成通过。
- 同一工作流整体仍失败，原因是 `services/market-data/internal/service/provider_probe_live_test.go` gofmt 检查失败，导致行情子模块测试未执行、行情镜像构建失败。该文件不属于新闻模块，本分支未修改它。
- 上述根模块验证包含新闻身份 Application 和版本指纹测试；不代表新闻 Repository、MariaDB 持久化、并发唯一约束或真实来源抓取已验收。


## 十八、N4 证券/行业关系断言首批领域规则（2026-11-05）

- [x] Security subject 使用 Market + Symbol，名称只作为快照；不同市场相同代码不合并。
- [x] Industry subject 使用 ClassificationSystem + IndustryCode/IndustryName 快照，不假设已有统一 industry_id。
- [x] RelationMethod 与 RelationEvidenceType 必须匹配；证据只保存 Observation/Version 引用、字段路径、规则/模型版本和 hash，不复制正文。
- [x] 模型方法只能产生 proposed 断言；后续人工确认应创建 method=manual 的新断言并 supersede 原模型断言，不能把模型证据改写为来源明确。
- [x] 修正/撤销通过新断言 supersede 旧断言；当前关系沿链计算，末端非 confirmed 不作为当前关系。
- [x] 固定测试覆盖跨市场同码、方法/证据类型匹配、模型确认门禁、修正、撤销、缺失父断言、跨新闻引用和多后继冲突。
- [ ] Repository 层仍需事务唯一约束 `UNIQUE(supersedes_relation_id)`、并发冲突处理及关系链持久化；当前不接 DB/Redis。


## 十九、N4 Observation 幂等与跨来源转载关系（2026-11-05）

- [x] IngestCandidate 要求上游提供稳定 IdempotencyKey，覆盖一次采集操作的重试；不同采集轮次应使用不同 key。
- [x] Repository 事务端口要求相同幂等键与相同候选载荷返回既有 Observation；同键不同载荷必须拒绝，防止误吞不同数据。
- [x] 重试已创建主体时先解析到既有身份，不重复创建新闻主体；Observation 唯一性由幂等键保证。
- [x] 新增 NewsItemRelation repost_of，FromNewsID 表示转载项，ToNewsID 表示原始项；两端 SourceID 必须不同，且必须关联 Observation 或内容版本证据。
- [x] 固定测试覆盖同一采集操作重试、幂等键载荷冲突、跨来源转载、同来源拒绝、自环、重复边和循环转载链。
- [ ] 生产 Repository 仍需对 Observation 幂等键、转载边唯一性及关系图一致性实施事务约束；当前仍为 Domain + 内存替身。


## 二十、N4 版本追加与证券/行业关系 Application 流程（2026-11-05）

- [x] 新增 ResearchUpdate Application：在单一事务中读取最新内容版本、比较指纹、必要时追加版本，并处理关系断言及证据。
- [x] 同指纹重复采集复用既有版本，不受 Observation ID 和采集时间变化影响；来源标题/摘要/正文变化才追加下一版本。
- [x] 版本号从 1 开始按新闻主体单调递增；Repository 必须在事务内校验并发追加和唯一性。
- [x] 关系断言必须属于目标新闻主体；写入前校验 Domain 规则、证据类型和 supersedes 链，拒绝多后继/断链/跨新闻引用。
- [x] 内容匹配证据缺少版本引用时绑定本次有效内容版本；来源字段/人工/模型证据缺少引用时绑定当前 Observation。
- [x] 关系证据写入前在同一事务内查询并验证 SourceVersionID / ObservationID 的真实存在性及 NewsID 归属；拒绝缺失引用和跨新闻引用，并由事务回滚保护版本与关系写入。
- [x] 模型 proposed 断言不进入当前有效关系；人工确认通过新 manual 断言 supersede 模型断言。
- [x] 固定内存事务测试覆盖版本去重、版本追加、证券关联证据、模型提议/人工确认和事务失败回滚。
- [ ] 生产 Repository 尚未实现；数据库事务、唯一约束、并发写入及真实历史版本查询仍属于后续公共平台持久化阶段。


## 二十一、N4 证券身份生命周期首批领域规则（2026-11-05）

- [x] 新增外部证券目录身份快照：以 `IdentityNamespace + IdentityValue` 表示上游稳定的证券级身份键，不虚构本系统 security_id，也不使用发行人 ID 代替证券身份。
- [x] 证券代码快照包含 Market + Symbol、简称快照、代码状态、上市生命周期状态、有效起止时间、系统观察时间和来源证据。
- [x] 按业务有效时间解析证券身份；历史代码在有效期内仍可解析，代码变更通过旧代码历史区间和新代码有效区间表达。
- [x] 相同市场代码在有效时间出现多个身份记录时返回 ambiguous，不按简称或相似度猜测。
- [x] 跨市场相同代码独立解析，不自动合并；退市记录仍可用于历史时点解析。
- [ ] 当前没有权威证券主数据 Adapter/统一 security master；本批只定义纯 Domain 快照和解析规则，不生成或推断外部稳定身份键。
- [ ] 后续补充名称变更、代码变更、停复牌与退市的官方样本；停复牌不改变证券身份。
- [ ] 证券目录来源的接入、同步、持久化和跨进程并发约束待后续主数据/公共服务能力明确后再实施。


## 二十二、N4 行业分类版本管理（2026-11-05）

- [x] 行业分类身份由分类体系 + 分类版本 + 行业代码共同确定；相同代码在不同版本中不自动视为同一行业。
- [x] 新增纯 Domain 行业分类快照，记录行业名称、父级代码、业务有效起止时间、系统观察时间、来源证据和 active/discontinued 状态。
- [x] 按分类体系、版本、代码和业务有效时间解析；不跨版本自动合并，重叠有效记录返回 ambiguous。
- [x] 固定测试覆盖跨版本同码不同含义、名称及父级快照变化、非法有效期、缺失来源证据、缺失版本和跨版本不命中。
- [x] IndustryRelationSubject 增加 ClassificationVersion，关系断言必须明确分类版本。
- [ ] 分类体系升级映射不推断同名/同码等价；后续必须通过显式映射证据和独立关系断言表达。
- [ ] 本批仅实现纯 Domain，不包含分类目录 Adapter、官方数据同步、Repository、SQL、MariaDB、Redis 或 Migration。


## 二十三、N4 转载关系与内容版本联动（2026-11-05）

- [x] 转载关系证据分别绑定转载项和原始项的内容版本，避免单一版本引用无法说明证据来自哪一侧。
- [x] 任一侧内容版本变化，关系证据评估返回 stale；两侧版本均一致才返回 current。
- [x] 仅有 Observation 引用、没有双方版本锚点的关系标记为 unversioned，不伪装成已验证当前版本。
- [x] 版本评估只读，不自动改写历史证据引用；内容修订不会自动证明转载关系仍成立或已经失效。
- [ ] stale/unversioned 关系的重新核验、修订/撤销追加历史及并发唯一约束，需在转载关系 Repository/Application 事务能力中实现；本批不引入持久化。
- [ ] 生产 Repository 尚未实现；当前规则由纯 Domain 固定测试覆盖。


### ResearchUpdate 版本并发约束

- ResearchUpdate 必须在同一事务内先验证请求 Observation 存在且属于目标 NewsID，再读取最新版本、追加版本和关系断言。
- 生产 Repository 必须在读取 LatestNewsItemVersion 前取得按 NewsID 协调的跨进程/跨实例写锁，并持有至提交；不能仅依赖进程内 mutex。
- 版本表必须对 `(news_id, version_no)` 建立唯一约束。并发冲突后必须重跑完整事务回调，重新读取最新版本并重新计算指纹，不能只重试 AppendNewsItemVersion。
- 集成测试需覆盖同一 NewsID 并发追加不同内容、并发提交相同指纹、事务回滚后版本号连续性，以及不同 NewsID 可独立推进。


### IngestCandidate 身份并发与幂等约束

- 生产 IdentityRepository 必须在身份凭据查询前取得按 SourceID 协调的跨进程/跨实例写锁，并持有至主体、凭据和 Observation 同一事务提交；不能只依赖进程内 mutex。
- 身份凭据唯一键为 `(source_id, identifier_type, identifier_hash)`；Hash 命中后仍必须比较原文 Value，发现碰撞或多主体命中不得自动合并。
- `news_items.id` 与 `news_observations.id` 必须唯一；Observation 的 `idempotency_key` 按全局唯一处理。同一 key、相同候选载荷返回既有 Observation；同一 key、不同载荷返回冲突。
- 并发唯一键冲突必须回滚并重跑完整身份事务，重新查询凭据和幂等 Observation；不得仅重试主体创建或凭据插入。
- 集成测试覆盖相同身份并发首次创建（最多一个主体）、不同身份同来源并发、同幂等键并发重放、同幂等键不同载荷竞争，以及冲突回滚后不遗留孤立主体/凭据。
- 版本指纹必须包含规范化后的新闻 URL；ProviderItemID 属于来源内身份凭据，不纳入内容版本指纹。

### N3 Feed 正文载荷类型映射（离线能力）

- Atom `content` 的默认/显式 `text` 映射为 plain_text，`html`/ `text/html` 和 `xhtml` 映射为 html_fragment；未知 MIME 保留原始声明并标记 unknown。
- Atom `content src` 只保留外部引用元信息，不把空载荷伪装为正文，也不触发网络请求。
- Atom `xhtml` 只有在载荷是 XHTML namespace 下的单一 `div` 根元素时才标记为 HTML fragment；不符合结构时保留原始片段并标记 unknown。
- Atom 声明为 `text`、`html` 或扩展 MIME 却包含未转义 XML 子元素时，不按声明格式解释；保留原始片段并标记 unknown，避免把结构化 XML 错当普通文本或 HTML。
- RSS `content:encoded` 按 RSS 内容模块约定标记为 HTML fragment；RSS `description` 仍只作为来源摘要。
- Feed 内容格式识别不推断 ContentKind（全文/摘录）；只有来源明确提供的类型元数据才能进入后续判断。

### N3 Feed 内容类型与附件元信息（离线能力）

- `DetectFeedFormat` 只接受明确的 `application/rss+xml` 和 `application/atom+xml`，允许 MIME 参数；不根据 URL 后缀、响应正文或模糊的 `text/xml` / `application/xml` 猜测 Feed 类型。
- XML 根元素同时校验命名空间：RSS 2.0 根元素不接受意外 namespace；Atom 1.0 必须使用 `http://www.w3.org/2005/Atom`，不因本地元素名相同而误判格式。
- RSS 2.0 `enclosure` 与 Atom 1.0 `link rel="enclosure"` 提取 URL、原始 MIME、规范 MIME、原始长度、非负整数长度及 Atom 标题。
- 原始声明值必须保留；无法解析的 MIME 不产生规范 MIME，负数/溢出/非法长度不产生数值长度，但保留原始字符串供质量判断。 原始 URL、MIME 和长度文本不做覆盖式清洗；规范 URL 仅用于解析后的字段，规范 MIME/长度与原始声明分开保存。
- 即使附件 URL 为空，仍保留 Feed 中声明的附件元信息；通过 `ValidationIssues` 标记 `missing_url`、`invalid_media_type`、`invalid_length`。长度缺失属于可选元数据，不报错；0 是有效长度。正文与附件始终独立，不把附件提升为正文。
- 附件 URL 是不可信元数据；解析阶段不访问、不下载、不验证网络目标，也不代表允许抓取。实际抓取必须通过来源授权和平台 SSRF/HTTP Client 安全能力。
- 离线测试覆盖 RSS/Atom 附件、MIME 参数、非法长度、未知 Content-Type。在线 Feed 拉取和附件下载仍未开放。

## 2026-11-05 N3 Feed 指纹与安全边界补充

- [x] 内容版本指纹纳入 Atom 外部正文引用、原始 Content-Type 声明及附件元数据，并将算法标识升级为 `news-version-v2`。
- [x] 外部正文引用不创建正文载荷，不触发 Parser/Application 网络请求。
- [x] Feed XML 外部实体声明按非法 Feed 拒绝；不允许文件读取或外部网络实体展开。
- [x] Candidate 同时保留 RawURL 与 CanonicalURL；IngestCandidate 清空调用方预填规范值，仅在 Source URL Policy 允许时生成 CanonicalURL。ResearchUpdate 从事务内已验证的 Observation 读取 RawURL/CanonicalURL 后计算版本指纹，调用方不能伪造版本身份 URL。
- [ ] 在线 Feed 采集仍等待 News 按 endpoint 注入 BoundDoer、PLAT-002 冻结、来源授权与生产 DNS/出口验收；N3 尚未完成。


## N3 在线 Source Adapter 第一批应用层实现（2026-11-05）

本批次建立在线 Feed 采集用例，但不接入尚未冻结的 Platform HTTP Client：

- 新增 `SourceCatalog`：按 `source_id + endpoint_id` 查询已登记来源入口快照；采集请求不接受任意 URL。
- 新增 `SourceTransport`：调用时显式携带 source/endpoint 身份。生产组合根必须为每个 endpoint 注入独立 BoundDoer；当前接口和 fake 测试不代表已完成平台接入。
- 新增 `CollectSource`：先执行 Source/Endpoint 授权交集与初始 URL 精确 allowlist，再请求 RSS/Atom；校验 HTTP 状态、Content-Type、响应体上限和声明格式一致性。
- 采集结果保留 Candidate 与 ContentLayers；来源原始 ID、登记发布者信息、来源摘要和正文层分离；不下载附件、不访问 Atom 外部正文引用、不写 DB/Redis。
- 仅允许请求内 transient 处理；不允许通过该用例写入持久化内容或跨请求缓存。
- 错误使用稳定业务 Code，响应体和底层网络错误不进入错误文本。429、超时类传输错误和 5xx 标记为 retryable 候选，不自动重试；未知传输错误默认不可重试。
- DNS/IP 特殊地址拦截、DNS rebinding、代理、重定向逐跳校验及实际拨号绑定仍是 Platform HTTP Client 的职责；PLAT-002 未冻结、News 组合根未注入 BoundDoer、生产 DNS/出口验收未完成前，不得开放生产在线抓取。

当前验收仅覆盖 fake Transport 下的应用编排，不代表真实网络、SSRF 或授权文件真实性验收。


## N3 在线采集授权版本溯源（2026-11-05）

- SourceCatalog 返回的登记快照必须携带不可变的 SourcePolicyVersionID。
- EndpointPolicy 非 nil 时必须同时携带 EndpointPolicyVersionID；EndpointPolicy 为 nil（继承 Source 默认规则）时不允许提供 EndpointPolicyVersionID。
- CollectSource 在任何网络请求前校验版本引用结构；缺失或不一致时 fail closed。
- CollectSourceResult 返回 Source/Endpoint 策略版本引用及策略评估时间；新增 ObservationPolicyTrace 与 IngestCandidate.ExecuteWithPolicyTrace，将该信息写入 Observation，不复制授权全文或敏感凭据。
- Observation 的幂等载荷比较包含 Source/Endpoint 策略版本 ID；同一幂等键不能跨策略版本复用。策略评估时间保留首次记录，但不作为幂等载荷差异字段。
- 版本 ID 由 Catalog/策略注册表提供，本模块不生成、不推断，也不把 EvidenceRef 当作版本 ID。


## N3 Endpoint 独立传输绑定适配器（2026-11-05）

- 新增 `adapters/source.EndpointTransport`，按精确 `SourceID + EndpointID` 将请求分派到独立 HTTPDoer。
- 构造时复制绑定 map；运行期不读取调用方后续对原 map 的修改。
- 未绑定 Endpoint 直接失败，不存在默认 Client 或跨 Endpoint fallback；请求 Context 显式传递给绑定 Doer。
- 该适配器只负责确定性分派，不负责 DNS/IP、代理、重定向、实际拨号或策略授权。生产组合根必须注入平台 `httpclient.BoundDoer`；接口兼容和 fake 测试不构成 BoundDoer 生产接入验收。


## N3-01 Endpoint 独立传输绑定 Review（2026-11-06）

审查基线：`eda0ebafe5bc8192e9025dff95eefe5b4dedc940`。只读核对 News 当前分支及 `feat/platform-foundation` 的 HTTP Client 契约；未修改平台分支。

### News 适配器审查结论

- [x] `EndpointTransport` 按精确 `SourceID + EndpointID` 选择 Doer，不存在默认绑定或跨 Endpoint fallback。
- [x] 构造时复制绑定 map；调用方后续替换原 map 不影响已创建的适配器。
- [x] 请求 Context 传递到克隆后的 Request。
- [x] 空绑定、非法键、nil Doer、缺失 Endpoint 均有失败关闭测试。
- [x] `CollectSource` 只从 SourceCatalog 取得登记 URL；先执行 Source/Endpoint 授权交集与 News URL allowlist，再调用传输端口。
- [x] `CollectSource` 限制 RSS/Atom、请求内 transient 处理、响应大小和 Entry 数量；不下载附件、不访问 Atom 外部正文引用。
- [x] 适配器职责边界明确：不负责 DNS/IP、代理、重定向、实际拨号或授权决策。
- [x] 本轮未发现需要在 News 侧重复实现的 Endpoint 分派缺陷，未修改业务代码。

### 平台契约与生产门禁

- 平台分支当前 HEAD：`d989bbc8d78505efb625b0b6e450fe390244b478`。
- 平台已有 `httpclient.Client.Bind(DestinationPolicy)` 与 `httpclient.BoundDoer`，BoundDoer 满足 News 的 `Do(*http.Request)` 窄接口。
- 平台 HTTP Client 已实现精确 Host/Port 策略、DNS 结果校验与校验 IP 实际拨号绑定、禁用环境代理、重定向逐跳校验、响应体上限、超时和有界并发/幂等重试。
- 平台路线图仍将 PLAT-002 标记为待评审；PLAT-008 标记为“实现已测，领域接入待验收”。平台文档明确：平台单测和受控网络测试不等同于生产 DNS/出口验收。
- News 当前 `EndpointTransport` 的构造参数是通用 `app.HTTPDoer`，类型层面无法证明生产注入的一定是 BoundDoer。该保证必须由组合根装配与集成验收完成；不能通过把 fake 测试或接口兼容测试升级为生产验收来替代。
- PLAT-002 未冻结、平台实现未进入本分支可依赖基线、根服务组合根 Owner/接入点未确认前，不导入平台内部包、不复制平台 HTTP Client、不开放真实抓取。

### 判定

| 检查项 | 结论 | 说明 |
|---|---|---|
| News Endpoint 分派 | 通过 | 当前适配器实现及固定测试覆盖 |
| Source/Endpoint 授权前置 | 通过（代码级） | 固定测试覆盖授权失败不发网络请求 |
| 平台 BoundDoer 可用性 | 已有实现，待基线接入 | 仅在平台分支观察到 |
| News 生产组合根注入 | 未验收 | 当前分支没有实际生产装配证据 |
| DNS/IP 与实际拨号 | 不可比较/未验收 | 没有 News 真实请求证据 |
| 生产出口与来源授权 | 未验收 | 需要部署环境与真实授权证据 |

**N3-01 适配器代码审查完成；N3 在线采集整体仍未完成。** 后续转入 N3-02：审查并补齐 CollectSource 到 IngestCandidate 的幂等闭环；生产 BoundDoer 注入作为平台契约冻结后的独立集成门禁保留。


## 2026-11-06 N3-02 CollectSource → Observation 幂等闭环审查

### 已有能力与边界

- [x] CollectSource 从 SourceCatalog 取得已登记 Source/Endpoint 快照，返回 Candidate、ContentLayers 和授权版本 Trace；不写入 Observation 或内容版本。
- [x] IngestCandidate.ExecuteWithPolicyTrace 在身份事务中保存 Observation，并将 Source/Endpoint Policy Version ID 纳入同键载荷一致性判断。
- [x] ResearchUpdate.Execute 从事务内 Observation 读取 RawURL/CanonicalURL，验证 Observation 与 NewsID 归属后计算内容指纹并追加版本；内容层独立传入，不混入 Candidate。
- [x] feed_ingest_test.go 已有固定 RSS 样本的离线跨层测试，串联 Feed → Candidate/ContentLayers → Observation → NewsItemVersion，并检查附件质量降级。
- [x] 无 SourceItemID 且无 RawURL 时仍追加 unidentified Observation，保留发现证据，不创建 NewsItem。
- [x] 修正身份凭据构造：来源原生条目 ID 可独立作为来源内凭据；同条目 URL 不在 allowlist 时仅丢弃 URL 凭据，不再连带丢弃有效原生 ID。若没有原生 ID，非法 URL 仍 fail closed。新增固定 Domain 测试。

### 尚未形成的生产闭环

- [ ] 当前没有可确认的采集编排调用方把 CollectSourceResult 的每个 Entry 依次交给 IngestCandidate，并将同一 Entry 的 ContentLayers 与返回 ObservationID/NewsID 交给 ResearchUpdate。现有 feed_ingest_test.go 是测试内手工串联，不是生产组合根或调用方。
- [ ] IngestCandidate 的 IdempotencyKey 由上游提供；当前测试中的 feed-poll-1/item-1 只是固定样例。尚无真实采集触发器/Task 提供不可变 CollectionRunID，因此不新增自行生成批次 ID 的 helper，也不以时间戳、数组下标或标题拼接替代。
- [ ] 幂等键语义确定为：同一 CollectionRun、Source、Endpoint、稳定 Entry 身份的重试复用同一 key；后续独立 CollectionRun 必须生成新 key，以保留新的 Observation。CollectionRunID 的产生与持久化归未来真实触发器/调度调用方所有。
- [ ] Entry 稳定身份优先使用 Feed 原始 ID（RSS GUID/Atom ID）；缺失时可由经过来源 allowlist 规范化的 URL 提供身份凭据；两者均缺失时 Observation 保留为 unidentified，不用标题或数组位置伪造身份。
- [ ] PolicyTrace 必须直接使用本次 CollectSourceResult 的版本快照，不得在 Ingest 前重新查询 Catalog；当前 Trace 结构已支持透传，但尚无生产编排实现验证这一点。
- [ ] ContentLayers 仅在授权允许持久化且拥有有效 Observation/NewsID 后交给 ResearchUpdate；当前 CollectSource 强制 transient，不能据此将在线采集正文直接持久化。后续持久化采集必须重新满足 Source/Endpoint 的持久化用途和保留期限授权。
- [ ] 本分支没有生产 IdentityRepository/ResearchUpdateRepository；全局幂等键唯一约束、事务回滚、跨实例锁和并发重放仍待 N6 Repository 集成验收。

### 判定与下一步

- Domain/Application 组件能力已具备首批闭环测试，但生产采集到持久化的编排与批次身份契约未完成；不能标记 N3-02 整体通过。
- 不在缺少真实调用方时增加无调用的 orchestrator API，也不让当前 transient CollectSource 越权写入数据库。
- 下一步先明确真实采集触发入口及 CollectionRunID 所有权；入口确定后再实现薄编排，贯通 Entry → Observation → ContentVersion，并验证同批重试/跨批新 Observation/策略版本快照/内容授权。生产网络门禁和 N6 持久化验收仍独立保留。


## 2026-11-06 N3-03 公共 HTTP Client 基线与采集触发入口复核

### 平台分支最新只读结果

- 只读检查 `feat/platform-foundation` HEAD `1afcddc7cf08b0995ba872028f2ec4548ebb0d79`；没有对平台分支执行写操作。
- PLAT-008 已提供 `httpclient.Client.Bind(DestinationPolicy)` / `BoundDoer`；BoundDoer 固定策略并实现 News `HTTPDoer` 的 `Do(*http.Request)` 形态。平台路线图仍标记“实现已测、领域接入待验收”，平台专项 CI 不代表生产 DNS/出口验收。
- PLAT-002 在平台路线图仍标记“待评审”；PLAT-013 仍为领域接入设计/验收阶段。PLAT-010 已明确不抽取通用 Task Queue，现有队列仍直接服务 Analysis，不提供 News CollectionRun 调度契约。
- 当前 News 分支 HEAD `49ee81116ec929c9007e5627244bfd960f6b5359` 的文件树不包含 `internal/platform/httpclient` 或平台 Fault 包；因此本分支尚不能编译依赖 BoundDoer。不得在平台实现未进入共同基线前添加内部包 import、复制 HTTP Client 或用默认 http.Client 代替。

### News 组合根与触发入口核查

- `cmd/analysis-api/main.go` 当前只装配 Analysis Runner、分析任务队列和 HTTP API，没有 News Application、Source Catalog、Provider 或 Source Collector 的生产装配。
- 当前 News 目录仅有 Domain/Application/Provider/Source/Parser 组件和离线测试；没有生产采集调度器、CollectionRun 持久化记录或可确认的主动采集调用方。
- 平台现有 `taskqueue` 直接依赖 Analysis Runner/领域模型；PLAT-010 评审结论不支持把它当作通用新闻采集队列。不能借用 Analysis RunID 冒充 CollectionRunID。

### 判定与后续边界

- [x] PLAT-008 平台原语已存在；News 的窄 Doer 接口与 BoundDoer 形态兼容。
- [ ] PLAT-002 正式冻结及平台实现进入 News 共同基线。
- [ ] Server 组合根 Owner/装配入口明确，并为每个 Provider API endpoint 和每个 Source endpoint 分别绑定精确 DestinationPolicy。
- [ ] 生产采集触发器/调度入口与 CollectionRunID 生命周期明确；只有在该入口确定后，才实现同批重试稳定 key 与跨批新 Observation 的编排。
- [ ] 来源授权、内容持久化用途/保留期限、真实 Provider 账号配额、生产 DNS/出口网络验收。
- 本轮不增加无调用方的 orchestrator、Task、CollectionRun 或幂等键生成接口；不修改 `cmd/analysis-api/main.go`，因为平台依赖和组合根接入所有权尚未进入当前分支。
- 下一步优先等待/核对共同基线更新；进入当前分支后再完成 BoundDoer 注入与契约测试。若 Server 组合根仍由独立集成分支负责，应由该分支提供明确装配点，不由 News 专项擅自接管进程启动入口。


## 2026-11-06 授权披露门禁与采集返回裁剪 Review

- [x] 增加 `EvaluateNewsAuthorizationUse`：仅显式 allowed 且授权内容级别覆盖请求级别时允许；conditional/denied/unknown、未生效、过期和 Binding 错配均 fail closed。Conditional Conditions 尚未由 Owner 评估前不放行。
- [x] CollectSource 返回前按请求 ContentKind 裁剪 ContentLayers，避免 metadata 请求连带返回摘要/正文/附件元数据；未分类正文不提升为 full_text；attachment 仅返回附件元数据。
- [ ] 当前策略目录复核、GOV-CTR-006 JCS 指纹复算、撤销实时性、存储读取竞态保护仍未完成；本轮门禁不代表生产持久化内容读取已完成授权闭环。


## 二十五、2026-11-06 项目级总规划复核与接入计划

项目级规划基线：`feat/design_doc` @ `12823e069be9036ad6a05235ac4304581d6883a1`。详细阶段任务、依赖和退出条件见《[新闻模块公共平台契约接入实施计划](新闻模块公共平台契约接入实施计划.md)》。

- Platform 原语已合并；N5 不再等待平台代码迁入，改为 News Config/Secret、BoundDoer、Fault/Request ID 消费者接入。
- N6 改为 News 自有 Repository/SQL/Migration、授权快照、事务/并发和读取复核；Platform 只提供连接生命周期与 Migration Runner，Server 负责共享资源及总注册。
- NEWS-CTR-001 依赖真实 Repository，按项目契约实现 GetEvidence/ListEvidence、PIT、稳定排序、固定快照和 HMAC Cursor；不新增 News HTTP API。
- GOV-CTR-006 的 JCS Runtime Verifier 尚未在当前合并基线发现；等待公共唯一实现路径，不在 News 复制算法。
- N7 接入 Platform Diagnostics Runner，News 只提供领域 Check；不新增诊断 Runner、诊断历史库或 News 专属诊断路由。
- N3 在线生产编排依赖真实 CollectionRunID Owner 和持久化授权；未确认前不造 ID、不借用 Analysis RunID、不引入通用 Task Queue。
- N8 使用 PLAT-013；PLAT-014 是故障演练。PLAT-011 为按需 Metrics，PLAT-012 为安全审计。
- 当前根组合根、共享 DB/HTTP Client 生命周期、News Migration 总注册和统一诊断入口未装配；News 分支不修改 `cmd/**`、根 DI 或全局路由。


## 二十六、2026-11-06 N5-1 Provider Config loader 首批实现

- [x] 新增 `internal/news/config`，Config 字段私有，按启动时快照使用；环境变量只通过 Platform `config.Env` 读取。
- [x] Bocha/Tavily/Brave endpoint 与 API Key 成对校验；两者均未配置时 Provider 保持可选未配置状态。
- [x] SearXNG 只要求 endpoint，不引入 API Key。
- [x] API Key 复用 Platform `config.Secret`；News 不定义第二套 Secret，不在错误信息中输出凭证或 endpoint 原文。
- [x] endpoint 仅做本地 URL 结构校验：HTTP(S)、绝对地址、Host 必填，拒绝 userinfo/query/fragment/非法端口；不执行 DNS 或网络请求。实际 Scheme/Host/Port 出口约束仍由各自 BoundDoer DestinationPolicy 执行。
- [x] Config loader 不创建 Client、不实例化 Provider、不触发真实抓取。
- [x] 隔离 Go harness 中 `go test -count=1 ./internal/news/config` 通过；该 harness 对 Platform Env/Secret 使用同签名测试替身，只证明配置包逻辑测试通过。
- [ ] 在真实仓库执行 `go test ./internal/news/config` 和 `go test ./...`，确认实际 Platform 包编译与全仓回归；当前 GitHub API 未返回该分支 workflow run/status。
- [ ] N5-2：Provider endpoint 独立 BoundDoer 绑定描述与适配测试；Source/Endpoint 继续使用独立绑定，不共用 Provider 策略。
- [ ] N5-3：Request ID 与 News ProviderError → Platform Fault 的映射及测试。


### N5-2 进度补充（2026-11-06）

- [x] 纠正文档 API 假设：当前 Platform HTTP Client 使用 `Client.Do(ctx, request, DestinationPolicy)`，不存在 `Client.Bind()` / `BoundDoer`。
- [x] 新增 Provider 专属 `NewPlatformHTTPDoer`，每个 Provider endpoint 独立精确 Host/Port 策略。
- [x] Provider 默认禁止重定向、禁止平台自动重试（`Idempotent=false`）；HTTP 仅由显式 HTTP endpoint 开启。
- [x] 平台 Fault 映射为 ProviderError，保留安全 Code/Retryable，不暴露 Cause。
- [x] 新增策略构造与错误映射测试。
- [ ] 在真实仓库执行 `go test ./internal/news/adapters/provider`、`go test ./internal/news/config` 和 `go test ./...`。
- [ ] Source endpoint 的 Platform Client 策略适配仍需完成；不得与 Provider endpoint 共用 Doer。


### N6-1 进度补充（2026-11-06）

- [x] 新增 News V1 Migration 清单，覆盖 Item、Identifier、Observation、Version、Relation Assertion/Evidence、跨新闻转载关系。
- [x] 将身份唯一键、全局 Observation 幂等键、新闻版本号唯一键和主要领域外键下沉到 MariaDB。
- [x] 复用 `platform/storage.Migration`，不建立 News 私有 migration ledger，不触碰 Server 总注册。
- [x] 新增 Migration 定义和返回切片隔离测试。
- [ ] 实现 MySQL Repository：IdentityTransaction、ResearchUpdateTransaction。
- [ ] 执行 MariaDB 空库初始化、重复执行、事务回滚和并发写入集成测试。


### N6-1 Repository 进度补充（2026-11-06）

- [x] 新增 `MySQLRepository`，借用共享 `*sql.DB`，实现 Identity / ResearchUpdate / NewsItemRelation 三类 Repository Port。
- [x] SourceID 级 advisory lock 保护身份读取、解析、主体/凭据创建和 Observation 追加事务。
- [x] News Item 行锁保护版本号读取和追加；数据库唯一键作为并发最终约束。
- [x] 全局关系图 advisory lock 保护转载关系历史读取、校验和追加，避免跨实例旧快照竞争。
- [x] Observation 幂等重放按完整候选载荷核对；冲突不会静默覆盖。
- [x] 增加构造参数、锁名和 SQL NULL 映射单测。
- [ ] 真实 MariaDB 集成：身份并发首次创建、同 key 同/异载荷、版本并发追加、关系图并发、事务回滚、锁释放。


### N6 MariaDB 集成验收补充（2026-11-06）

- [x] 新增可选真实 MariaDB 集成测试，环境变量为 `NEWS_MARIADB_TEST_DSN`；未设置时普通 `go test ./...` 自动 Skip。
- [x] CI 新增 MariaDB 11.4 独立 News 集成 Job，覆盖迁移、身份并发、幂等、回滚、版本并发及转载关系。
- [x] News MariaDB 11.4 CI Job 已成功（run 42195956099）：Migration 首次/重复执行、身份并发、幂等、回滚、版本并发、转载关系读写通过。


### N5 Source Endpoint 适配进度补充（2026-11-06）

- [x] 新增 Source 专属 `NewPlatformHTTPDoer`，复用 Platform `Client.Do(ctx, request, DestinationPolicy)`。
- [x] Source endpoint 独立精确 Host/Port 策略；保留 Feed URL query，拒绝 userinfo/fragment。
- [x] Source 默认禁止重定向和 Platform 自动重试；HTTP 仅在 endpoint 显式使用 http scheme 时允许。
- [x] Platform Fault 的 Retryable 分类映射为 Source transport 的安全分类，不暴露 Cause。
- [x] `EndpointTransport` 继续按 SourceID + EndpointID 精确分派，未绑定时 fail-closed。
- [ ] 真实 DNS/IP 出口、Feed 重定向策略、来源授权与生产组合根注入验收。


### N5/N6 本轮状态

- [x] Provider Platform HTTP Doer 适配。
- [x] Source Platform HTTP Doer 适配；Feed query 保留、Host/Port 精确绑定。
- [x] Platform Fault 的 Provider/Source 安全映射和 Retryable 分类传递。
- [x] News MariaDB V1 Migration 与三组 Repository Port 实现。
- [x] News MariaDB 集成 CI 首次成功。
- [ ] Source/Provider 真实公网 endpoint 验收、Server 生产组合根注入、统一诊断入口仍待后续集成阶段。


### N6 MariaDB 真实验收结果

- [x] News MariaDB 11.4 CI 集成 Job `42195956099` 成功。
- [x] 并发身份创建只生成一个 News Item；同 key 同载荷返回既有 Observation；同 key 异载荷冲突。
- [x] 主体创建事务回滚；并发版本追加保持 VersionNo 单调且唯一。
- [x] 跨新闻转载关系证据归属、双方最新版本和历史读取通过。
- [ ] 多版本 Schema 升级/News 专属失败恢复场景后续补充。


### N7 首批 News Check 进度补充（2026-11-06）

- [x] Provider Config Checks：可选 Provider 未配置时返回 skipped，不触发网络请求、不暴露凭证。
- [x] Parser Self Checks：RSS/Atom 使用本地固定 fixture，检测解析器回归。
- [x] Source Endpoint Readiness：检查登记、启用、访问方式、策略版本引用及 URL allowlist，不访问外网。
- [x] Storage Checks：复用 MySQLRepository 的 Ping 与 information_schema 只读检查，覆盖 9 张 News 表。
- [x] Check ID、Required、Timeout、ReasonCode、SafeMessage 和 Attributes 遵守 Platform Diagnostics 契约。
- [ ] 将 News Checks 注册到 Server 的 Platform Runner；News 分支不修改根组合根。
- [ ] 在线 Provider/Source 连通性、采集成功率、身份去重率、解析完整度和数据新鲜度检查待采集闭环具备真实数据后实现。


### N7 新闻业务质量诊断补充（2026-11-06）

- [x] 新增 `BusinessQualityCheck`，复用 Domain 质量规则评估单条证据快照。
- [x] Platform Check Status 与 News BusinessQualityStatus 分离：评估执行成功返回 Platform healthy，业务质量通过 Attributes 单独呈现。
- [x] 输出仅包含质量维度状态、稳定 issue code 和数量；不输出 evidence ID、标题、URL、摘要、正文或凭证。
- [x] Check ID 基于 evidence ID 哈希，构造时冻结 Candidate/ContentLayers 输入快照。
- [x] 测试覆盖 invalid/unknown 业务质量、Platform 状态分离、敏感内容不泄露及空 evidence ID 拒绝。
- [ ] 持久化数据批量质量扫描、Freshness、采集成功率、身份去重率，待明确真实采集批次入口和查询端口后实施；不得伪造统计。


### NEWS-CTR-001 只读 Evidence Query DTO 首批实现（2026-11-06）

- [x] 新增 `GetEvidenceRequest` / `ListEvidenceRequest` / `ResearchEvidenceRecord` / `ResearchEvidencePage` 公开 Application DTO 与 `ResearchEvidenceQuery` Port。
- [x] 用途、内容级别、质量状态使用受控枚举；过滤字段封闭，不接受任意 URL、SQL、字段名或排序表达式。
- [x] 请求校验覆盖可信 caller_ref 非空、system_as_of UTC 且不晚于当前时间、filters 必填、数组数量上限/去重、质量枚举、业务时间独立范围和 page_size 1..100。
- [x] 响应 DTO 明确 available_at/published_at nullable，空 items/diagnostics 初始化为非 null 数组。
- [x] 新增 NEWS-CTR-001 Get/List/Response JSON Schema 与 Golden；Go 测试读取 Golden 并校验请求边界。
- [x] 后续已补齐 Get/List Facade、PIT Repository、固定 Snapshot Store、Cursor 与引用分页的首批实现。\n- [ ] available_at 可信证据过滤、授权快照信封一致性校验、授权与内容读取同一一致性边界仍未完成；NEWS-CTR-001 不得标记完整验收。


### NEWS-CTR-001 HMAC Cursor 首批实现（2026-11-06）

- [x] 新增 `EvidenceCursorCodec`，HMAC-SHA256 防篡改，base64url token。
- [x] Cursor 绑定 caller/purpose/content level/过滤指纹/page size/system_as_of/snapshot/sort version/最后排序键。
- [x] 固定 15 分钟 TTL，区分 cursor_invalid 与 cursor_expired。
- [x] 支持 active key + previous key ring；旧 key 可在 TTL 内验证，新 token 使用 active key。
- [x] Platform `config.Secret` 持有密钥；拒绝弱密钥、未知 Key ID 和超出密钥环上限。
- [x] 过滤数组排序规范化，nil/empty 语义统一；时间值规范化 UTC。
- [x] 后续已实现固定 Snapshot 持久化、过期删除 Repository、PIT 查询和 Facade 首批编排。\n- [ ] 严格 available_at 过滤和授权一致性边界仍未闭环。


### N3 采集过程时间证据补充（2026-11-06）

- [x] CollectSource 记录独立 fetched_at、normalized_at、observed_at。
- [x] Candidate 过程时间随 Observation candidate_json 持久化；无需重复 SQL 字段。
- [x] 同一幂等键重试忽略过程时间差异，并返回首次 Observation 的时间快照。
- [x] Version Fingerprint 排除 fetched_at/normalized_at，避免重复抓取制造伪内容版本。
- [x] 单测覆盖 UTC 归一化、采集阶段时间顺序、幂等重放和版本指纹稳定性。
- [ ] available_at 仅等待可验证的来源公开可用时间证据；不得从采集时间推算。


### NEWS-CTR-002 不可变授权快照存储（2026-11-06）

- [x] 新增 Application `AuthorizationSnapshotRepository` Port 和 `RecordAuthorizationSnapshot` 写入用例。
- [x] 新增 MariaDB Migration V2：不可变 `news_authorization_snapshots` 表。
- [x] Repository 实现 append-only：相同快照幂等，不同内容冲突；无更新/覆盖方法。
- [x] 读取校验原始 payload SHA-256、JSON 解码和 NEWS-CTR-002 binding 结构。
- [x] MariaDB 集成测试覆盖首次写入、相同快照重放、不同 payload 冲突、读取和 not-found。
- [ ] GOV-CTR-006 JCS 决策指纹验证仍待共享 Runtime Verifier；当前 payload_hash 只保护存储字节完整性。
- [ ] 当前策略版本复核、撤销/过期竞态和内容读取 TOCTOU 原子门禁尚未实现；受限内容仍不得通过该历史快照直接放行。


### NEWS-CTR-001 固定分页 Snapshot Store（2026-11-06）

- [x] 新增 Snapshot DTO/Repository Port 和 SaveEvidenceSnapshot 校验用例。
- [x] Snapshot 只保存不可变证据引用和排序键，不复制新闻内容。
- [x] 固定绑定 caller、filter fingerprint、page size、system_as_of、sort version 和 15 分钟 TTL。
- [x] 校验排序唯一性、知识时间上限、创建/过期时间关系和最大 5000 条结果；`PageAfter` 按稳定排序键续页，拒绝不属于当前快照的 Cursor Position。
- [x] 新增 MariaDB Migration V3 与 MySQL Repository：创建、幂等、冲突、Caller 隔离读取、过期拒绝和限量清理。
- [x] Storage integration CI 覆盖授权快照与分页 Snapshot 持久化。
- [ ] Query Facade 尚未接入 Snapshot Store；严格 PIT 查询、available_at 证据门禁、授权当前复核和内容裁剪仍未完成。


### NEWS-P0-01 前置存储完整性回归补充（2026-11-06）

- [x] News MariaDB 集成测试增加授权快照 payload_hash 篡改检测，篡改后读取必须返回 ErrInvalidAuthorizationSnapshot。
- [x] 增加分页 Evidence Snapshot payload_hash 篡改检测，篡改后读取必须返回 ErrInvalidEvidenceSnapshot。
- [x] 测试结束前恢复测试记录原始哈希，避免污染后续同库测试。
- [x] MariaDB 11.4 News 集成 Job `42207154603` 成功；授权快照与分页 Snapshot 篡改后读取均按预期拒绝。
- 该测试验证数据库载荷完整性哈希，不验证 GOV-CTR-006 JCS decision_fingerprint，两者不可互相替代。
- GOV-CTR-006 Runtime Verifier 尚未在当前分支 internal/platform/security 中发现；News 不复制实现，NEWS-CTR-002 的 JCS 验证仍保持阻塞。

- 同一 Workflow 的根模块 `Test .` Job 因 `internal/research/context/validate.go` 非法反斜杠及 `internal/research/evidence/authorization.go` NUL 字节导致 gofmt 失败；错误位于其他模块，本分支 News 任务不越界修改。


### NEWS-CTR-001 快照查询上下文绑定补充（2026-11-09）

- [x] EvidenceSnapshot 增加 purpose_code 与 requested_content_level，均来自创建快照时通过校验的 ListEvidenceRequest，不引入新外部数据源。
- [x] Validate 强制用途和内容级别为受控枚举；缺失或非法值判定为无效快照并拒绝持久化/读取。
- [x] MatchesRequest 比对 caller、purpose、content level、filters fingerprint、page_size 和 system_as_of；任一不同均不可复用该快照。
- [x] MariaDB 集成测试构造完整查询上下文；新字段保存在既有 payload_json 并由 payload_hash 完整性校验保护，无需新增 Migration 或重复建设存储能力。
- [ ] Query Facade 尚未调用 MatchesRequest；该方法须在 Facade 接入时于读取快照后、分页前调用。
- [ ] 严格 PIT Repository、available_at 来源证据门禁、当前授权复核与内容读取 TOCTOU 仍未完成。


### NEWS-CTR-001 单版本 GetEvidence PIT Repository（2026-11-09）

- [x] Repository Port 调用场景限定为 News Application GetEvidence Facade 内部读取单条主体/指定版本；不对 Research 暴露 Domain。
- [x] PIT 规则：版本自身 observed_at 与关联 Observation observed_at 均不得晚于 system_as_of；关联同时校验 news_id、observation_id。
- [x] 未指定 version_id 选择知识时点内最新可见版本；指定 version_id 也不能绕过 system_as_of。
- [x] 数据来源：SourceRef 来自 Observation.source_id，ProviderRef 来自 Observation.provider_code；版本/观察内容来自既有 V1 表，无新增字段、Migration 或外部数据源。
- [x] MariaDB 集成验收覆盖未来版本不可见及指定未来版本拒绝。
- [ ] ListEvidence PIT、关系/身份历史解析、available_at 来源证据、授权复核和 Facade 尚未完成。
- 判定：未命中或未来不可见统一返回 ErrEvidencePITNotFound（不可见/不存在不区分，避免泄露主体存在性）；SQL/连接故障作为存储故障传递，不可降级为空结果。


### NEWS-CTR-001 ListEvidence PIT Repository 首批实现（2026-11-09）

- [x] 新增 `ListEvidenceVersionsAt` Repository Port：按 `system_as_of` 选取每个 News Item 在知识时点内最新可见版本。
- [x] SQL 同时约束 Version 与关联 Observation 的 `observed_at`，并校验 `news_id`/`observation_id` 关联；通过 `NOT EXISTS` 排除知识时点内已有更新版本的记录。
- [x] 支持 `news_ids`、`source_refs`、`published_at_from/to` 的参数化过滤；固定排序为 `observed_at DESC, news_id ASC, version_id ASC`，最多读取快照上限加一条以便调用方识别超限。
- [x] Repository 集成测试覆盖时点前不可见、时点后可见、News ID 与 SourceRef 过滤。
- [ ] `canonical_symbols`、`quality_statuses`、`business_time_from/to` 尚无完整权威的双时态查询语义，本实现对这些非空过滤 fail closed，返回 `ErrInvalidEvidenceQuery`，不静默忽略。
- [ ] 当前仅完成 List PIT 存储原语；关系/身份历史解析、available_at 来源证据、AuthorizationRef 当前策略复核、固定 Snapshot 组装和 Query Facade 仍未完成。


## 二十四、ListEvidence PIT 首轮 CI 问题修正（2026-11-09）

- 最新 CI（run 42574499685）中的 News MariaDB 集成失败定位为测试请求将 `system_as_of` 设置为数据库观察时间之后 1 秒，违反 Application 契约对未来知识时点的拒绝规则；不是 SQL 查询或迁移失败。
- 测试改为使用当前 UTC 时点执行列表查询，保留“观察时点之前不可见”的历史断言；不放宽生产请求的未来时点校验。
- 本修正等待新 CI 确认；若后续仍失败，再按新的失败日志处理。


## 二十五、证据快照存储信封一致性（2026-11-09）

- `news_evidence_snapshots` 的 JSON payload 哈希只能证明 payload 本身未变化，不能证明用于检索的关系列（过滤指纹、页大小、知识时点、排序版本、创建/过期时间）未被独立修改。
- `GetEvidenceSnapshot` 现同时读取并核对上述持久化列与哈希校验后的 payload；任一不一致均返回 `ErrInvalidEvidenceSnapshot`，不继续分页。
- MariaDB 集成测试增加独立篡改 `page_size` 的拒绝断言，并在断言后恢复测试数据。
- 同一轮修正了 PIT 多版本测试后续关系断言引用旧版本的问题：关系写入要求引用当前最新版本，测试现引用修订后的版本。
- 本提交的 CI 尚待执行；全仓 Research 语法错误仍是其他模块问题，不在 News 分支处理。

- 追加处理数据库精度差异：MariaDB `DATETIME(6)` 仅保存微秒，payload 保留 Go 时间纳秒；元数据一致性比较统一按 UTC 微秒精度比较，避免合法快照因纳秒截断被误判损坏。集成测试改用未截断的 `time.Now()` 覆盖该场景。


### NEWS-CTR-001 关系断言知识时点历史读取（2026-11-09）

- [x] 新增 Application 查询请求与 Repository Port：按 News Item 和 UTC `system_as_of` 读取关系断言历史。
- [x] MariaDB 查询同时覆盖关系的发起方/接收方，并以 `observed_at <= system_as_of` 限定知识可见性。
- [x] 返回追加式历史中的 proposed、confirmed、retracted 及被替代断言；不把历史记录误算为当前关系图，也不在读取时重写历史。
- [x] 对每条读取记录执行 Domain 结构校验；数据库/扫描错误向上传递，不降级为空历史。
- [x] MariaDB 集成测试覆盖时点前后可见性、发起方/接收方查询、proposed/retracted 追加链保留及 observed_at + relation_id 稳定排序。
- [ ] NEWS-CTR-001 List Facade 仍需在授权门禁和共享 JCS Runtime Verifier 可用后串联 Snapshot/Cursor；不在 News 内复制 GOV-CTR-006 算法。

## 二十六、NEWS-CTR-001 Facade 前置缺口核对（2026-11-09）

- [x] 关系断言知识时点读取已实现；MariaDB 11.4 集成 Job `129027648625` 在提交 `5e62d375e15bc00e600816f1debf174df12550d2` 通过，覆盖原确认断言、proposed 替代提议、retracted 撤销及历史排序。
- [x] Snapshot Store、Snapshot 上下文绑定、HMAC Cursor Codec 和 List PIT SQL 均已有独立实现；当前缺的是安全的 Application Facade 编排，不重复建设这些原语。
- [x] 身份历史 PIT 已由 V4 `news_identifier_status_history`、`ListIdentityCredentialsAt` 和 `ResolveIdentityAt` 实现；新增状态必须在同一事务追加历史事件。
- [x] NEWS-CTR-002 已冻结 `news_evidence_version:<news_id>:v<version_no>` 资源引用，并在写入授权快照时绑定实际 Version / Observation；Source、Endpoint、Provider 策略引用及版本均需与 Observation 追踪一致。
- [ ] GOV-CTR-006 JCS Runtime Verifier 在当前 News 分支的公共 Platform 基线中仍未发现。现有授权逻辑只校验绑定结构、决策和时间窗口，不足以证明 decision_fingerprint；不得把历史 allowed 快照直接当作当前授权门禁。
- [ ] `available_at` 尚无可追溯的来源公开可用时间字段/证据。不得用 published_at、fetched_at 或 observed_at 推算。
- [ ] 因上述身份 PIT、资源授权绑定、JCS 验证和 available_at 证据缺口，暂不实现会返回 ResearchEvidenceRecord 内容的 List/Get Facade。先完成可验证的数据与安全前置条件，再一次性串联 PIT → Snapshot → Cursor → 当前授权复核 → 内容裁剪；任何失败均 fail closed。


## 二十七、NEWS-CTR-001 PIT 快照组装用例

- [x] 新增 `CreateEvidenceSnapshotForQuery` Application 用例：仅处理无 Cursor 的首屏请求，先执行严格 PIT 查询，再固化快照。
- [x] Snapshot 仅保留 NewsID、VersionID、ObservationID 和知识时间排序键，不复制标题、摘要、正文或 Candidate 内容。
- [x] 创建前验证 Version/Observation 的主体与引用绑定、双方观察时间均不晚于 system_as_of，以及 Repository 返回顺序满足固定排序。
- [x] 结果超过 5000 条时返回明确的 `ErrEvidenceSnapshotTooLarge`，不截断、不保存部分快照。
- [x] Cursor 请求在该用例入口 fail-closed，避免翻页时重新执行 PIT 查询；后续读取必须从既有 Snapshot Store 继续。
- [x] Snapshot 读取、MatchesRequest、PageAfter 与 HMAC Cursor Codec 已由 `ListEvidenceSnapshotPage` 同一 Application 用例串联（见第二十八节）。
- [ ] 授权快照资源绑定、当前授权策略复核、GOV-CTR-006 JCS Runtime Verifier、available_at 证据及身份历史 PIT 仍是受限内容 Facade 的前置条件；本用例不返回内容，因此不绕过授权门禁。
## 二十八、NEWS-CTR-001 Snapshot + Cursor Application 分页编排（2026-11-09）

- [x] 新增 reference-only ListEvidenceSnapshotPage 用例，首屏执行一次严格 PIT 并创建不可变 Snapshot。
- [x] 后续 Cursor 请求先验证 HMAC 与 caller/query 上下文，再按 caller 隔离读取 Snapshot；不重新执行 PIT、不重复写入 Snapshot。
- [x] 分页使用 Snapshot 的稳定排序键和固定 page_size；下一页 Cursor 绑定 snapshot_ref、最后排序键、过滤指纹、用途、内容级别和 system_as_of。
- [x] 最终页不返回 next_cursor；空页/无后续页遵循 Snapshot PageAfter 语义。
- [x] 此用例只返回 NewsID、VersionID、ObservationID 与知识时间，不返回新闻内容，不替代当前授权门禁。
- [ ] 执行本地/CI Go 测试及 MariaDB 真实存储分页验收；当前运行环境未提供本地仓库执行能力。
- [ ] 与授权门禁、身份 PIT、available_at 证据和 GOV-CTR-006 JCS Verifier 闭环后，才实现 ResearchEvidenceRecord 内容 Facade。

## 二十九、NEWS-CTR-001 身份凭据知识时点历史（2026-11-09）

- [x] 新增 V4 `news_identifier_status_history` 追加式状态历史表，并建立按来源/凭据/观察时间读取的索引。
- [x] V4 对既有凭据按 `news_identifiers.created_at` 与当时保存的 `identifier_status` 建立初始历史事件；当前应用写入路径只有 INSERT，没有状态 UPDATE，因此该回填不把已知的历史状态变更压平成当前状态。
- [x] 新增凭据时，在同一身份事务内写入 `created_at` 与初始状态事件，时间来自 IngestCandidate 的 `observed_at`，不使用数据库执行时间推算。
- [x] 新增 `ListIdentityCredentialsAt` Repository 查询，只返回 `created_at <= system_as_of` 且存在可见状态事件的凭据；按 observed_at、history_id 选择时点内最后状态。
- [x] 新增 `ResolveIdentityAt` Application 用例，先按 system_as_of 重建凭据集合，再复用 Domain 同来源身份解析规则。
- [ ] 未来若引入身份凭据状态变更入口，必须在同一事务追加状态事件；禁止只更新 `news_identifiers.identifier_status`。当前尚无状态变更业务用例。
- [x] V4 MariaDB 迁移与 Repository 集成由 News MariaDB CI 覆盖并通过（run `42586250167`）。
- [ ] IdentityAt Application 单测尚未获得独立 CI 结果；根模块测试在 gofmt 阶段被 Research 文件错误阻断。

## 三十、NEWS-CTR-002 授权资源与不可变证据绑定（2026-11-09）

- [x] 按项目 Golden 冻结资源引用格式：`news_evidence_version:<news_id>:v<version_no>`；与实际 VersionID 分开，引用稳定指向新闻主体的不可变版本序号。
- [x] 新增 `ValidateNewsAuthorizationBindingForEvidence`，同时核对 Version/Observation 的主体归属、ObservationID、SourceRef、resource_ref 和来源/Endpoint 策略版本。
- [x] `RecordAuthorizationSnapshot` 与 MariaDB append 入口必须同时接收实际 Observation、Version 并完成资源绑定校验；不能只凭一份结构合法的 Binding 入库。
- [x] 单测覆盖资源版本错配、新闻主体错配、Observation 错配、来源错配及 Source/Endpoint 策略版本错配。
- [ ] Observation 当前尚未保存 Provider Output Policy Version ID；带 ProviderRef 的授权资源尚不能证明该策略版本与具体采集 Observation 的绑定。需先补齐真实策略评估追踪，再开放该类内容授权。
- [ ] GOV-CTR-006 JCS Runtime Verifier 在 integration / Platform 公共基线仍未发现；当前仍只校验 fingerprint 格式，不能据此宣称决策指纹已验证。
- [ ] 当前策略撤销复核、授权检查与内容读取 TOCTOU 原子门禁仍未实现；受限内容保持 fail-closed。

## 三十一、NEWS-CTR-002 Provider Output Policy 版本追踪（2026-11-09）

- [x] 新增 Observation.ProviderOutputPolicyVersionID 与 IngestCandidate.PolicyTrace 字段，纳入 Observation 持久化、读取及幂等载荷比较。
- [x] V5 Migration 为 news_observations 增加 provider_output_policy_version_id；旧 Observation 保留空值，不伪造历史策略版本。
- [x] 授权资源绑定要求 ProviderRef 与 Observation 中的 Provider Output Policy Version 一致；ProviderRef 为空时也拒绝 Observation 携带未绑定的 Provider 策略版本。
- [ ] 当前 SearchCandidates 尚未接入 ProviderOutputPolicy 评估和可信策略版本解析；只有组合根提供已评估的 PolicyTrace 才能写入该版本。没有真实评估的记录不能被授权。
- [ ] Provider Output Policy 评估与采集入口的完整链路仍待开发；GOV-CTR-006 JCS Runtime Verifier 仍等待 Platform 公共实现。



## 三十二、NEWS-CTR-002 Endpoint / Provider Policy 引用闭环（2026-11-09）

- [x] Observation 增加 EndpointID 与 ProviderOutputPolicyRef；V6 Migration 持久化两项引用，读取、幂等载荷比较与 PolicyTrace 同步更新。
- [x] 授权资源绑定不仅比较策略版本，还必须将 EndpointRef / ProviderRef 分别与 Observation 中实际采集入口和 Provider Policy Ref 精确匹配。
- [x] Golden、Domain、Application 与 MariaDB 集成样本覆盖引用错配和持久化往返。
- [ ] 这些 PolicyTrace 字段仍由可信组合根在完成真实策略评估后注入；News 不接受 Web/API 传入的自声明策略引用。
- [ ] SearchCandidates 的 ProviderOutputPolicy 评估和版本解析仍待接入；JCS 运行时校验与当前策略撤销门禁仍受公共能力阻塞。

## 三十三、Provider 策略追踪集成回归修正（2026-11-09）

- [x] MariaDB 授权绑定集成样本补齐 `EndpointID` 与 `ProviderOutputPolicyRef`，与 NEWS-CTR-002 Golden 中的 endpoint/provider 引用一致。
- [x] 修正后 News MariaDB Repository / Migration integration Job 在 run `42586250167` 成功。
- [ ] 当前 CI 另外存在 Research 目录非法字符/NUL 导致根模块 gofmt 失败，以及 Market-data 的交易日测试失败；均不属于 News 代码范围，本分支不越界修复。

## 三十四、PIT Snapshot + Cursor Facade 测试构造修正（2026-11-09）

- [x] 修正分页失败路径测试对 `NewListEvidenceSnapshotPage` 的过期三参数调用；Repository 已由 Factory 持有，Facade 构造只接收 Factory 与 Cursor Codec。
- [x] 最新可用 CI 的 News MariaDB Repository / Migration integration Job 通过；
- [ ] News app 单测仍需等待根模块 gofmt 阻塞解除后获得完整测试结果。

## 三十五、Endpoint Policy 引用进入 Observation 的条件修正（2026-11-09）

- [x] `CollectSourceResult.ObservationPolicyTrace()` 仅在 Endpoint Policy 存在已评估版本时写入 EndpointID / EndpointPolicyVersionID；普通采集入口 ID 不冒充授权策略引用。
- [x] 单测分别覆盖未配置 Endpoint Policy 时不产生 EndpointRef，以及存在 Endpoint Policy 时准确传递 endpoint_id 和版本。
- [ ] 等待当前分支最新提交 `095ad43` 的 CI 验证。

## 三十六、Provider 搜索结果持久化前置门禁（2026-11-09）

- [x] `IngestCandidate.Execute` 拒绝 `ProviderCode` 非空的搜索候选，避免绕过 Provider Output Policy 追踪直接写入 Observation。
- [x] `ExecuteWithPolicyTrace` 对 Provider 候选要求 Provider Output Policy Ref 与 Version 成对存在；缺失或只提供其中一项均 fail closed。
- [x] 新增测试覆盖无策略追踪和不完整策略引用时不产生持久化。
- [ ] 该门禁只验证可信组合根传入的评估结果引用完整，不把调用方字符串视为策略评估本身；下一步仍需由 Server 注入真实 Policy Resolver/Evaluator，并基于实际账号协议、实例配置和策略版本评估。
- [ ] Brave 搜索输出仍不得持久化；Bocha 权利确认前不得持久化；Tavily Provider 层许可仍须叠加 Source/Endpoint 权限；SearXNG 必须有实例与上游策略证据。
- [ ] `SearchCandidates` 维持 transient discovery，不新增混合 Provider 缓存；Provider 结果在没有真实策略注册表时不得进入持久化路径。


## 三十七、Provider Policy 未接入前的强制 fail-closed（2026-11-09）

- [x] 收紧 `ExecuteWithPolicyTrace`：当前仅接受 Source/Endpoint Policy Trace；任何 Provider 候选均拒绝持久化。
- [x] 同时拒绝非 Provider 候选携带自声明 Provider Policy Ref/Version，避免通过完整字符串伪造已评估状态。
- [x] 增加完整伪造 Provider Policy Trace 的回归测试；确认失败时不创建 NewsItem、不追加 Observation。
- [ ] Provider Policy Resolver/Catalog 与真实评估入口尚未接入，因此当前 Provider 搜索结果只能 transient 使用，不得进入 Observation。
- [ ] 后续由 Server 组合根注入可信策略解析器；News 应在 Application 内执行 `EvaluateProviderOutputPolicy`，并校验来源/入口权限交集后才允许持久化。不得仅依赖调用方传入的 ref/version 字符串。


## 三十八、关系历史查询 Application 门禁（2026-11-09）

- [x] 新增 `ListNewsItemRelationHistory` Application 用例，统一校验 NewsID、UTC 知识时点及未来时间边界。
- [x] 对 Repository 返回结果再次校验关系结构、查询主体归属和 `observed_at <= system_as_of`，防止 Adapter 返回越界或无关记录。
- [x] 校验结果按 `observed_at ASC, relation_id ASC` 稳定排序；相同观察时刻按关系 ID 确定顺序。
- [x] Repository 错误原样向上传递，不降级为空历史。
- [x] 新增单测覆盖知识时点等值可见、同时间排序、未来记录拒绝、无关主体拒绝、乱序拒绝、存储错误透传和无效请求不触达 Repository。
- [ ] 本次新增 Application 测试等待当前提交 CI 验证；MariaDB 关系历史存储验收此前已通过。


## 三十九、News Schema 诊断数量一致性（2026-11-09）

- [x] 修正存储诊断中的必需表数量，和 News V1-V6 迁移维护的 10 张表一致。
- [x] 增加完整 Schema 健康状态断言，避免诊断报告长期保留过期数量。
- [x] 保留缺表时 degraded 状态和缺失表数量检查。
- [ ] 等待本次提交 CI 验证。

- [x] 扩展来源入口诊断回归测试，覆盖注册身份不匹配、禁用入口、不支持访问方式、缺失来源策略版本和 URL 不在允许列表等 fail-closed 分支。


## 四十、Provider Output Policy 诊断闭环状态（2026-11-09）

- [x] 新增必需诊断项 `news.provider.output_policy`，明确报告可信 Policy Catalog / Evaluator 尚未接入。
- [x] 诊断状态为 `degraded`，并输出 `provider_persistence=blocked`；不因 Provider 凭证已配置而误报为可持久化。
- [x] 该检查不访问上游、不读取或输出凭证、不授予任何持久化权限。
- [x] 增加正常与取消上下文测试。
- [ ] Server 注入可信 Policy Catalog / Evaluator 后，需将静态阻塞诊断替换为真实策略覆盖度与版本解析检查，并保留未知策略 fail-closed。

- [x] Snapshot + Cursor 增加存储快照上下文被独立篡改后的拒绝测试：即使快照结构与哈希外层存储可读，purpose 与签发 Cursor 不一致时仍拒绝分页，且不重跑 PIT。


## 四十一、Snapshot + Cursor Facade 异常边界回归（2026-11-09）

- [x] 覆盖 PIT 查询失败、Snapshot 创建失败、Snapshot 读取失败，验证底层存储错误原样透传，不伪装为空结果或普通无数据。
- [x] 覆盖 Cursor 的 caller、purpose、content level、filters、page_size、system_as_of 任一上下文不匹配时拒绝续页。
- [x] 验证上下文不匹配在访问 Snapshot Store / PIT Repository 前被拒绝，避免跨调用方读取及无效请求触发存储访问。
- [ ] 仍需由当前分支 CI 完成 go-core 与 News MariaDB 集成验收；内容 Facade 仍受当前授权复核、JCS Verifier、Provider Policy 与 available_at 证据门禁约束。


- [x] 关系历史查询回归补齐追加式状态保留：同一知识时点返回 proposed、confirmed、retracted 断言及 supersedes 引用，不把历史查询误当作当前关系图。


## 四十二、Evidence PIT 回读完整 Provider / Endpoint 策略追踪（2026-11-09）

- [x] 修正 Evidence PIT SELECT 与扫描映射，完整回读 Observation 的 endpoint_id、provider_output_policy_ref、provider_output_policy_version_id。
- [x] 保留既有 Source / Endpoint Policy Version 与 policy_evaluated_at 字段，避免跨模块 Research 读取时丢失采集时策略追踪。
- [x] MariaDB 集成样本写入独立策略追踪值，并断言 PIT 查询完整回读；该字段回读不等于授权评估通过，也不解除 Provider 持久化门禁。
- [ ] 等待当前提交的 News MariaDB 集成 CI；Provider Output Policy 真实评估仍由 Server 注入可信 Resolver / Catalog 后完成。


## 四十三、关系断言链数据库唯一性约束（2026-11-09）

- [x] 新增 V7 Migration，为证券/行业关系断言与跨新闻转载关系的 supersedes_relation_id 建立唯一索引。
- [x] 利用 MySQL/MariaDB UNIQUE 对 NULL 的允许行为保留多个链首断言，同时阻止同一父断言被多个后继同时替代。
- [x] Migration 单测覆盖两类关系表的唯一约束定义。
- [ ] 需在 MariaDB 集成环境执行 V7，确认升级路径与现有数据兼容；若历史数据存在多后继，Migration 应失败并暴露冲突，不自动删除或选择其中一条。


## 四十四、公共平台 BoundDoer 正式接入（2026-11-09）

- [x] Provider 传输适配器改为通过 Client.Bind(DestinationPolicy) 创建不可变 httpclient.BoundDoer，不再在每次请求时直接调用 Client.Do 传入可变策略。
- [x] Source 传输适配器同样改为持有独立 BoundDoer；Provider 与 Source 不共享绑定实例，且每个 endpoint 必须按自己的 Host/Port/Dependency 构造。
- [x] 保留 Provider 禁止重定向、Source 默认禁止重定向、精确 Host/Port、DNS/IP 与实际 Dial 校验、平台超时/响应体限制及 Request ID 传播。
- [x] 配置继续使用 Platform config.Env / config.Secret；News 领域配置只负责自身字段校验，不自行读取环境变量。
- [x] News Diagnostics 继续只定义 platform/diagnostics.Check 工厂，由 Server 注册共享 Runner；不在 News 重建 Runner。
- [x] News Migration 继续返回 platform/storage.Migration，由 Server 统一注册到 news namespace；News 不自建账本或 Runner。
- [ ] Server 组合根仍需创建共享 Platform HTTP Client、逐 Provider/Source endpoint 创建 BoundDoer、注册 Source EndpointTransport，并装配 News Diagnostics 与 Migration。
- [ ] GOV-CTR-006 JCS Runtime Verifier 在当前公共平台代码树中仍不存在；授权指纹校验和内容授权保持 fail-closed，不以 fingerprint 格式检查代替 JCS 验证。
- [ ] DNS/IP、实际 Dial、重定向与出口策略仍需部署环境 A3 验收；本地 BoundDoer 适配不代表生产出口验收完成。


## 四十五、News 公共平台组合根装配辅助入口

- [x] 新增 provider.NewPlatformProviderSet：按 News Config 中已配置的 Provider 构造 ProviderSet，每个 Provider 使用独立 Platform BoundDoer 和独立 Dependency 限额标识；未配置的可选 Provider 不创建实例。
- [x] Provider Secret 只在创建具体 Provider Adapter 时通过 Secret.Value 显式读取；配置、诊断和错误不输出凭证。
- [x] 新增 source.NewPlatformEndpointTransport：基于可信 SourceCatalog 启动快照，为每个启用且支持 RSS/授权 Feed 的 Source/Endpoint 创建独立 BoundDoer。
- [x] Source 绑定拒绝重复身份、非法 URL 和空的可支持 Endpoint 集合；禁用或不支持访问方式的入口不获得网络传输器。
- [ ] Server 组合根需提供共享 Platform Client 配置（超时、响应上限、并发与 Resolver），装配 ProviderSet / EndpointTransport，并管理 Client CloseIdleConnections 生命周期。
- [ ] SourceCatalog 若支持运行期变更，需在变更后重新生成受控绑定快照；不得把任意 URL 或客户端传入的 endpoint 身份直接转换为 BoundDoer。


## 四十六、Platform Fault 边界校验接入

- [x] Provider / Source 适配器映射 Platform Fault 前调用 Fault.Validate()；非法 Class、Code、SafeMessage、Dependency 或 Retryable 组合统一降级为安全的不可重试传输错误。
- [x] 增加非法 Fault 元数据回归测试，防止错误码污染 ProviderOutcome 或将不合法的 retryable 标志传入领域层。
- [x] 保留有效 Fault 的稳定 Code 与 Retryable 分类；Cause 不跨越 News Adapter 边界。


## 四十七、Provider 候选持久化接入可信策略评估

- [x] 新增 `ProviderOutputPolicyResolver` 应用端口，要求组合根从可信策略目录解析 Provider 当前策略的 Ref、不可变 Version 与策略内容；不接受候选或外部请求自声明策略引用。
- [x] 新增 `IngestProviderCandidate` 用例，在持久化前重新读取 SourceCatalog 与 Provider Policy Resolver，执行 Provider ∩ Source ∩ Endpoint 的用途、内容类型、存储方式、缓存与留存期限交集评估。
- [x] 策略解析缺失、身份不匹配、策略版本缺失、来源版本不完整或策略拒绝时 fail-closed；拒绝发生在 Identity Repository 事务之前，不产生 NewsItem、Identity Credential 或 Observation。
- [x] 通过策略评估后，自动写入 Source / Endpoint / Provider Policy Ref + Version 与 PolicyEvaluatedAt，不允许调用方构造 ObservationPolicyTrace 绕过 Provider 策略评估。
- [x] 增加允许路径、Provider 拒绝、Resolver 不可用、来源策略版本缺失及 Provider 身份错配测试。
- [ ] Server 组合根仍需注入真实、可信、可撤销的 ProviderOutputPolicyResolver；在接入前现有 Provider 候选仍不得通过旧 `IngestCandidate` 路径持久化。
- [ ] Resolver 必须以不可变策略版本为依据，并在策略更新/撤销后使后续请求读取最新版本；授权快照与 JCS 运行时校验另按 NEWS-CTR-002 / GOV-CTR-006 门禁处理。


## 四十八、当前授权策略版本与撤销复核（2026-11-09）

- [x] 新增 `domain.CurrentAuthorizationPolicySet`，表达可信策略 Owner 在本次读取时返回的完整当前策略版本、状态、生效/过期时间及针对请求用途/内容级别的评估结果。
- [x] 新增 `EvaluateNewsAuthorizationAgainstCurrentPolicies`，将授权快照中的 Source / Endpoint / Provider Output 策略 tuple 与当前策略逐项比对；缺失、重复、版本变化、撤销、待审核、未知状态、未生效、过期或当前范围不允许均拒绝。
- [x] 新增 `NewsAuthorizationFingerprintVerifier` 与 `CurrentNewsAuthorizationPolicyResolver` 应用端口；News 只依赖端口，不实现 JCS，也不创建策略目录。
- [x] 新增 `EvaluateNewsEvidenceReadAuthorization` 授权预检用例：校验 Binding 与精确 Observation/Version、caller/purpose/content level 一致；先验证 GOV-CTR-006 指纹，再读取当前策略并执行 fail-closed 判定。
- [x] 增加版本变化、策略撤销、当前用途/内容级别不再允许、策略缺失、Verifier/Resolver 不可用及调用绑定错配测试。
- [ ] Server/策略 Owner 注入真实 Resolver；Resolver 必须读取权威当前版本，不能仅检查历史 Snapshot 或复用长期缓存。
- [ ] GOV-CTR-006 唯一 JCS Runtime Verifier 尚未进入当前共同代码基线；适配器接入和项目 Golden 验收完成前，内容授权保持 fail-closed。
- [ ] 本用例是披露前预检，不提供内容、不消除检查后撤销的竞态；最终 NEWS-CTR-001 内容查询必须把当前策略复核与精确 Version 内容读取放入同一授权一致性边界，并补充撤销/过期竞态测试。


## 四十九、Provider 策略诊断状态细分（2026-11-09）

- [x] 新增 `ProviderOutputPolicyResolverCheck`，由 Server 组合根传入可信 Resolver 和 Provider 标识执行只读解析检查。
- [x] 区分 Resolver 未配置、目录查询不可用、策略绑定无效、策略版本已解析但具体请求尚未评估四种状态；不把目录可用或版本存在报告为策略评估通过。
- [x] 所有状态均明确 `provider_persistence=blocked`；该 Check 不发 Provider 网络请求、不触发配额消耗、不授权候选持久化。
- [x] 增加状态分类、绑定身份错配和敏感错误不外泄测试。
- [ ] Server 组合根注册 resolver-aware Check；旧无 Resolver Check 仅作为 fail-closed fallback，不作为生产接入完成证据。


- Review 新增 Source 候选低层 Ingest 入口门禁：当前 `IngestCandidate.Execute` 不验证 Source Policy Version，`ExecuteWithPolicyTrace` 不能信任调用方自报版本；生产编排完成前不得直接将其作为跨模块持久化入口，需先增加可信 SourceCatalog 绑定用例。


## 五十、Evidence Snapshot 写入完成与请求取消竞态（2026-11-09）

- [x] Snapshot Repository 写入成功后再次检查 Context；若请求在数据库写入期间被取消，不向调用方返回可继续使用的 Snapshot。
- [x] 增加“写入成功后立即取消”的 Application 回归测试，确认请求返回 context.Canceled，同时明确不可变 Snapshot 可能已经落库。
- [x] 已落库但未返回给调用方的 Snapshot 不执行补偿删除；它不含新闻正文或摘要，依靠既有 TTL 与过期清理回收，避免取消路径误删可能已被并发重试使用的快照。
- [ ] 仍需在 MariaDB 环境覆盖真实数据库写入与客户端取消的竞态；Application 测试仅验证调用边界，不代表数据库事务回滚。


## Observation 清理批次计数语义（2026-11-09）

- `Scanned` 表示本次清理事务通过 `FOR UPDATE SKIP LOCKED` 成功领取并锁定的到期 Observation 数，不代表全库到期 backlog，也不包含被其他事务锁定而跳过的记录。
- `Purged` 表示本批实际更新并清除候选载荷的记录数。全局剩余到期量和最早到期时间必须通过只读 `ObservationRetentionBacklog` 查询，不能用 `Scanned - Purged` 推算。
- 对当前 MySQL Repository，已锁定记录在同一事务内必须全部更新；若 `RowsAffected` 与领取数量不一致，整批返回错误并回滚，不把异常部分更新报告为成功结果。
- 本语义修正尚未通过 MariaDB 11.4 实际运行验证；并发锁跳过、回滚和结果计数仍属于待执行集成验收。

- 补充诊断边界：backlog 查询返回负数、正数却无最早到期时间、零数量却带最早时间时，诊断报告 unavailable，不再把不一致结果误报为 healthy/degraded。


### Evidence Snapshot 过期错误语义修正（2026-11-09）

- [x] `GetEvidenceSnapshot` 先校验 Snapshot 结构、未来创建/知识时点及数据库信封一致性，再单独判断 TTL；已过期但内容完整的快照统一返回 `ErrEvidenceSnapshotUnavailable`，不再误报为 `ErrInvalidEvidenceSnapshot`。
- [x] 既有 MariaDB 集成断言已覆盖 `now == expires_at` 的过期边界；本次未执行该测试，等待 News MariaDB Integration CI 提供运行证据。


### Evidence Snapshot TTL 清理 Application 入口补齐（2026-11-09）

- [x] 新增有界 EvidenceSnapshotRetentionCleaner，通过窄 Repository Port 执行到期 Snapshot 清理；限制批量 1～1000，统一 UTC 时间，拒绝负数或超过批量上限的 Repository 计数。
- [x] 清理用例传播调用前/执行期间的 Context 取消，不创建 News 内部调度器；周期调度仍归 Server 组合根。
- [ ] Server 尚未注册该 Cleaner 的周期任务；本分支仅提供 Application 入口。
- [ ] 本次未执行 Go 编译或单元测试，等待 CI。


### Evidence Snapshot 读取载荷大小上限（2026-11-09）

- [x] Create 与 Get 共用 8 MiB payload 大小校验；读取时在 SHA-256 和 JSON 反序列化前拒绝空载荷及超限载荷，避免异常数据库记录触发无界内存/解析开销。
- [x] 增加大小边界单测（空、nil、正常、上限、超上限）。
- [ ] 本次未执行 Go 测试；等待 CI。


## 2026-11-09 最新 CI 格式门禁复核

- Run `42621069210` 的根模块 `Check gofmt` 失败中，News 范围仍有 3 处格式问题，已在当前分支修复：Evidence Snapshot 存储文件多余空行、存储测试多余空行、Snapshot retention 测试嵌套缩进。
- 同一 Run 的其余 gofmt 差异涉及 Notification、Quant 等其他模块，本分支不越权修改；需由对应模块分支修复后再取得根模块全绿结果。
- 本次修复后仍需以新 CI 的 gofmt Job 结果确认，不提前标记通过。


## 2026-11-10 修复进度

- [x] 授权快照读取端增加 64 KiB 上限及数据库信封字段与 JSON Binding 一致性校验。
- [x] Provider Ingest 区分 SourceCatalog not-found 与依赖不可用，并保留底层错误原因。
- [ ] Run `42740420934` 中 News 未再触发 gofmt 错误，但根模块测试被其他模块格式差异阻断；News MariaDB 集成 Job skipped，仍需独立运行验收。
- [ ] 当前剩余主要阻断：available_at 可信证据门禁、授权复核与内容读取 TOCTOU、Server 组合根注入及 MariaDB 真实验收。详见《代码与文档Review报告》。


### available_at 审计结论（2026-11-10）

现有 Feed/Observation 时间字段不能证明新闻首次公开可用时间；Atom updated 也不是首次公开时间。当前保持 `available_at=null`，不从 published_at/fetched_at/normalized_at/observed_at 推断。严格 PIT 门禁需等待来源证据模型先行冻结。


### 最新授权快照一致性修复（2026-11-10）

最新授权排序选择与载荷读取现已合并为单条 SQL 语句，统一校验 payload/hash/数据库信封；并增加“旧 allow + 最新 deny”集成断言。该修改尚待 CI 与 MariaDB 11.4 实际运行。它不代表 Evidence 内容读取与当前授权策略复核的 TOCTOU 已解决。


### 2026-11-10 Evidence 读取边界测试补充

- GetEvidence 新增当前授权器返回后 Context 取消的回归用例，确保取消后不返回内容 DTO。
- 新增 RetentionUntil 与当前时间相等的边界用例，明确到期边界为 `now >= retention_until`；到期只抑制 Candidate 采集元数据。
- 对应提交：`cc423098087b39e45716430b84a232e343914045`；CI Run `42742190179` 待完成。


### 2026-11-10 授权复核取消传播补充

- 修复授权指纹 Verifier 与当前策略 Resolver 返回后未立即检查 Context 的缺口。即使依赖忽略取消并返回成功，UseCase 也会终止，不再将结果作为有效授权复核。
- 新增两个取消边界单测，验证 Verifier 取消后不调用策略 Resolver。
- 对应提交：`9edb6f7321939784c3be865d56544109b954f680`。


### 2026-11-10 采集策略依赖取消传播补充

- Provider Ingest 在 SourceCatalog、ProviderOutputPolicyResolver 返回后检查 Context；Source Ingest 在 SourceCatalog 返回后检查 Context。即使依赖忽略取消并返回成功，也不会继续进入授权策略评估或持久化。
- 新增三个取消边界测试，并检查取消后没有 Observation/Identity 持久化。
- 对应提交：`e898104b9b548057330978c3fbc287e6c9d79a57`。


### 2026-11-10 News 测试格式门禁修正

- 根据 CI gofmt diff 单独修正 `authorization_read_test.go`、`get_evidence_test.go`、`provider_ingest_test.go`、`source_ingest_test.go`。
- 未处理其他模块的 gofmt 差异，避免越界修改。
- 对应提交：`0076c667d080aaf1b3889f54799d203693129eb9`。


### 2026-11-10 Feed 采集取消传播补充

- CollectSource 增加入口、SourceCatalog 返回后、SourceTransport 返回后及响应体读取后的 Context 检查。传输依赖返回响应但请求已取消时，主动关闭响应 Body 并终止采集。
- 新增 SourceCatalog 和 SourceTransport 两个取消边界测试。
- 对应提交：`a42c384137e2b22a68daaf126ff5cea22270128d`。


### 2026-11-10 Feed 取消测试结构修正

- CI 在解析新增取消边界测试时发现多余右花括号，已修正。
- 对应提交：`ac8410ee00b0afb0a4cb6238dd8b03b420e75382`。


### 2026-11-10 Provider Ingest 测试格式修正

- CI 再次发现 `internal/news/app/provider_ingest_test.go` 两处复合字面量字段缩进不符合 gofmt，已修正。
- Market Data、Notification、Quant 的格式差异没有在 News 分支处理。
- 对应提交：`c4982d374813e70a30282e4e82027558a6d3384f`。


### 2026-11-10 Evidence Snapshot 保存取消传播

- `SaveEvidenceSnapshot.Execute` 增加 Repository 调用前和成功返回后的 Context 检查。
- 新增已取消时不调用 Repository、Repository 返回后发生取消的测试。
- 若 Repository 已提交后 Context 才取消，UseCase 返回取消错误但不执行补偿删除；持久化结果需通过幂等重试/读取确认，不将 Context 取消误认为事务回滚。
- 对应提交：`4e9998e12315480a9f8e6a769a42bf58a7768af9`、`42f460f40c69ace10ba8e21f5603e8884e1ac7ca`。


### 2026-11-10 Feed 解析与归一化取消边界

- `CollectSource.Execute` 原有取消检查覆盖了 Catalog、Transport 和响应体读取，但 Feed 解析后的归一化阶段仍可能在请求取消后继续处理候选并生成结果。
- 现已在解析返回后、每条候选处理前、归一化时间采集后、结果封印前及返回前检查 Context。
- 新增归一化阶段取消测试，确认取消不会返回采集结果。
- 对应提交：`a7988ad1fb7aa9ef7635365e8215cee017c15b76`。


## 2026-11-12 当前任务边界与后续计划

### 本轮已确认的验收证据

- News 专项 Run `43210204658`：`Test full News module` 与 `Test News Provider contracts` 均 success，步骤包含 News gofmt、全量单测、Race、Vet 及 Provider 契约测试。
- 根模块 CI Run `43210204807`：News MariaDB Repository and Migration integration success；根模块 `Test go-core` 与 `Test .` 均 success。Market Data MariaDB、Compose/Docker 和 Market Data 迁移/测试 Job 为 skipped，不计为通过。
- News Diagnostics 已补齐 Observation retention、Source Endpoint Catalog、Storage Ping/Schema Check 的依赖调用前后 Context 取消边界；Storage Ping 在依赖返回取消错误时优先传播 Context 取消，Schema Check 在已取消时不触发探测。上述代码已包含在当前 News 专项验证范围内。
- 最近的 `eac3269` 是 Observation Retention 事务选项参数修正；本轮没有发现需要继续重复实现的同类事务参数工作。

### 当前仍未关闭的事项及归属

| 事项 | 当前判定 | 责任边界 / 下一步 |
|---|---|---|
| GOV-CTR-006 JCS Runtime Verifier | 阻断 | 由公共能力 Owner 提供唯一实现；News 只接入并验证 Golden，不复制 JCS 算法 |
| 授权复核与精确 Version 内容读取 TOCTOU | News 内部事务机制已实现 | `evidence_authorization_read.go` 在同一 InnoDB 事务锁定策略指针、复读 Observation/Version、执行授权回调并读取正文；MariaDB 集成覆盖读取与撤销并发。剩余是公共 JCS Verifier 的真实注入和 Server 生产装配，不再重复建设事务机制 |
| available_at 严格 PIT | 阻断但暂无可实施真实数据 | 当前已核实来源没有符合准入规则的首次公开可获取时间字段；继续保持 NULL，不新增空壳字段/表，不修改 Get/List SQL。只有取得合格来源证据后才实现证据持久化和 PIT 过滤 |
| Provider/Source 生产装配、Migration 总注册、Diagnostics Runner、Cleaner 周期调度 | 非本分支任务 | 由 Server 组合根装配；News 提供的构造入口、Migration、Check 和 Cleaner 已存在 |
| `risk_review` 当前策略映射 | 当前 HEAD CI 已通过 | 显式 `risk_review` 映射到 `PurposeInternalBusiness`；反向通用映射仍拒绝，避免从 `internal_business` 自动推断 purpose_code。Source/Endpoint/Provider 策略仍独立决定是否许可 |
| 真实 Provider/Feed 网络、配额及来源授权验收 | 等待真实环境条件 | 需要可用账号凭证、实际套餐条款、逐来源授权及部署 DNS/出口证据；未满足时保持 skipped/restricted |
| News 自有代码质量与 MariaDB 专项验证 | 已通过当前 CI | 不重复进行无差异回归；后续代码变更再按变更范围触发验证 |

### 下一步执行顺序

1. 不新增 News 领域功能，不为 WebUI 增加 API，也不复制 Platform/Server 能力。
2. 跟进公共 GOV-CTR-006 Runtime Verifier 的可依赖实现；确认真实 package/API 和 Golden 后，由 Server 组合根注入并完成 News 消费者接入；News 已有事务型授权读取机制，不再重复实现。
4. `risk_review` 映射缺口已按冻结契约修复；当前 HEAD 的 News 专项测试、Race、Vet 和根模块相关 CI 已通过，不重复讨论用途语义。
5. 等待具备来源证据的真实数据后，再单独实现 available_at 证据模型与 Get/List 严格 PIT；不以模拟样本或推断时间验收。
6. 有真实 Provider 凭证和授权材料后，执行受控 Live Probe；否则不把固定响应契约测试描述为在线验收。

本分支当前没有可在不依赖外部 Owner、真实授权或真实来源证据的情况下继续完成的高价值生产闭环代码任务；当前优先事项是取得 integration Merge Commit `85c78f1` 的 CI 验收结果。后续若没有新的 News 专属缺陷或合格外部证据，保持代码冻结，只处理明确的 News 范围缺陷和必要文档同步。


## 十七、2026-11-12 NEWS-CTR-002 risk_review 显式用途映射修复

- 代码提交：`e76eb3a88ecffd7fe97e2ddf8cb55c28e04eee08`；测试提交：`6188c22efd24993bf2717b8b382661cb3854b3eb`。
- 按冻结契约将显式 `risk_review` 映射为领域策略用途 `internal_business`；未改变契约、Domain Purpose 枚举或策略数据结构。
- `research_analysis`、`risk_review`、`user_display` 分别对应 `internal_personal_research`、`internal_business`、`public_display`。反向映射仍拒绝 `PurposeInternalBusiness`，因为它没有唯一的 purpose_code。
- SourcePolicy、EndpointPolicy、ProviderOutputPolicy 继续按请求显式 purpose_code 检查策略许可；映射不等于授权通过。
- 新增测试覆盖用途映射、SourcePolicy 用途匹配、EndpointPolicy 用途匹配，以及不允许跨用途推断。
- 当前 HEAD 的 CI 已覆盖上述代码和测试：News 专项 gofmt、全量单测、Race、Vet、Provider 契约测试均通过；MariaDB Repository/Migration 集成也通过。详见本节后续“当前 HEAD CI 验收结果”。


### 十七（续）、risk_review 修复的当前 HEAD CI 验收结果（2026-11-12）

- 验收分支：`feat/research-news-ingestion`；验收 HEAD：`a8a3763e5dc662d856a438c52a0b1a28975def43`。
- News Provider contract workflow Run [43219930444](https://github.com/Kasign/stock_data_provider/actions/runs/43219930444)：`Test News Provider contracts`、`Test full News module` 均 success。覆盖 News gofmt、`go test ./internal/news/...`、News Race Detector、News Vet，以及 Provider contract tests、Race、Vet。
- Go test suite Run [43219930418](https://github.com/Kasign/stock_data_provider/actions/runs/43219930418)：News MariaDB Repository and Migration integration、根模块 `Test .`、Researchflow MariaDB lifecycle integration、Researchflow format/tests/race/vet、WorkerStore tests/race/vet、`Test go-core` 均 success。Compose/Docker 和 Market Data 专项 jobs 为 skipped，不计入通过范围。
- 两条 Run 的 `head_sha` 均精确匹配验收 HEAD；因此 `risk_review` 映射及新增存储层测试已被当前 HEAD 的 News 全量测试、Race、Vet 覆盖。
- 该结果不代表真实 Provider/Feed 网络、来源授权、部署 DNS/出口、Server 生产装配或 available_at 严格 PIT 已验收。
