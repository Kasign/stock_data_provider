# N1-A 新闻基础设施架构评审报告

> 评审版本：2026-11-05  
> 当前状态：内部架构收敛完成，待用户评审确认  
> 适用分支：`feat/research-news-ingestion`  
> 本报告是 N1-A 设计评审材料，不代表业务代码、数据库或外部 Provider 已完成接入。

## 1. 评审结论

建议按本文架构进入 N2，但必须先由用户确认。本阶段不进入业务代码开发，不提前接入 DB、Redis 或通用诊断服务。

核心结论：

1. 新闻基础设施作为根 Go Module 内的模块化单体能力，落在 `internal/news/`，随 `cmd/analysis-api` 和 `stock-analysis-server:8092` 统一构建部署。
2. Provider Adapter 直接归属根模块。取消独立 `shared/newsprovider` Module，不兼容未上线的旧 go-core/Python 新闻服务。
3. 新闻模块只提供同进程 Application 能力；当前不新增新闻专属 HTTP Handler、路由、Token、服务、端口、数据库或 Redis。
4. 新闻业务质量、授权策略、身份、版本和证券关联由新闻 Domain/Application 负责；通用诊断、DB/Redis 和执行历史由未来公共服务统一提供。
5. 搜索 Provider 身份与实际发布来源身份必须分离；Provider 结果授权与发布者内容授权必须分别判定。
6. 先保留可验证事实和证据，不因发布时间缺失、发布者未知、身份冲突或授权未核实而伪造补齐。

## 2. 已确认的范围和边界

### 2.1 本专项负责

- 搜索候选接入及 Provider provenance。
- 已登记来源/入口的受控采集。
- 新闻元数据标准化、身份识别、去重和版本识别。
- 正文/附件提取载荷的授权判断和业务状态表达。
- 新闻与证券、行业的基础关联及关联证据。
- 内部 Application 用例和可测试的业务质量规则。

### 2.2 本专项不负责

- 多新闻聚合为事件、事件生命周期、影响方向/时效和价格验证。
- LLM 新闻解读、研究结论或买卖信号。
- 独立新闻微服务、独立部署、新闻专属公网 API。
- 新闻专属 DB/Redis 客户端、连接池、缓存、诊断框架和诊断历史。
- Python Bridge、旧 NewsSearcher、旧 DTO、Agent/Bot 调用方兼容、灰度或双写。
- 未经单独批准的历史 Python 数据导入。

事件研究映射到 RESEARCH-004/MIG-006；新闻基础设施主要映射 DATA-016/MIG-005。事件研究不应塞入新闻采集模块。

## 3. 目标架构与依赖方向

```text
cmd/analysis-api
  └── service wiring
       └── internal/news
            ├── domain/
            ├── app/
            │    ├── usecases
            │    └── ports
            └── adapters/
                 ├── provider/       # Bocha / Tavily / Brave / SearXNG
                 ├── announcement/   # 官方公告/披露来源，独立于搜索 Provider
                 ├── source/         # 已批准的 RSS/Atom/发布来源
                 ├── parser/
                 └── security/       # URL、DNS、重定向和响应安全策略
```

依赖约束：

- Domain 不依赖 HTTP、SDK、SQL、Redis 或外部 Provider。
- Application 编排业务用例，只依赖 Domain 和业务端口。
- Adapter 实现 Provider、Fetcher、Parser 等端口。
- Repository 端口按实际业务操作定义；当前不实现 DB/Redis Adapter。
- 不机械地按 11 个候选实体创建 11 个 Repository。
- 现有 `go-core/internal/adapters/newssearch` 只作协议和历史行为参考；迁移必要 HTTP 协议处理时按新契约重构，不复制旧 Searcher 的聚合、缓存、过滤排序、fallback 和旧 DTO。
- 不调整 go-core Docker 构建上下文，不创建跨 Go Module `replace`。

## 4. 领域模型职责

当前逻辑模型是 11 个新闻业务实体；2 个执行记录实体仅为未来候选，不代表当前必须落库。

| 实体 | 职责 |
|---|---|
| `news_sources` | 可识别的发布者/来源身份 |
| `news_source_endpoints` | 来源下的 RSS/API/网页等入口 |
| `news_source_policy_versions` | 来源/入口授权策略的不可变版本 |
| `news_provider_policy_versions` | 搜索 Provider 输出用途和留存策略版本 |
| `news_items` | 同一发布来源内可识别的新闻主体 |
| `news_item_identifiers` | 来源原始 ID、规范 URL 等身份凭据和历史别名 |
| `news_observations` | 每次搜索发现/来源采集的业务证据 |
| `news_item_versions` | 新闻标题、摘要、URL、时间等权威元数据版本 |
| `news_item_contents` | 绑定具体版本的正文/附件提取载荷 |
| `news_relations` | 新闻与证券/行业的追加式关系断言 |
| `news_relation_evidence` | 关系断言的证据和判定依据 |

`news_ingestion_runs`、`news_ingestion_run_steps` 暂不冻结；待公共服务执行记录契约确定后，判断复用公共记录还是保留必要的新闻业务执行实体。当前不建设 `news_schema_migrations`，不冻结物理 DDL。

关键职责约束：

- `news_items.source_id` 必须指向真实来源；来源未知时只保留 observation。
- `news_items` 不重复保存版本化标题、摘要、URL 和发布时间。
- `news_item_contents` 不重复保存 title/summary，只保存正文和独立内容载荷。
- Provider 候选证据与发布者事实不可混为一谈。
- 不建立无明确用途的 Provider 注册表、原始大响应表、去重日志表或 current-version 映射表。

## 5. 时间语义

| 字段 | 唯一含义 |
|---|---|
| `published_at` | 发布者声明的发布时间；保留原始文本、时区、解析状态和解析器版本 |
| `available_at` | 有证据证明内容首次对市场参与者可获得的时间；无证据为 NULL |
| `observed_at` | 每一次搜索/采集观察发生的时间 |
| `fetched_at` | 来源正文响应体实际获取完成时间；不能用搜索响应时间代替 |
| `normalized_at` | 某一元数据版本完成标准化的时间 |
| `first_observed_at` | 系统首次观察到新闻主体的时间，由最早 observation 聚合 |
| `last_observed_at` | 系统最近一次观察到新闻主体的时间 |

`system_observed_at` 不再作为独立字段：首次系统观察时间由 `news_items.first_observed_at` 承载，单次观察时间由 `news_observations.observed_at` 承载。禁止重复表达同一时间语义。

历史研究和回测遵循 point-in-time 原则。不得把搜索引擎更新时间、抓取时间或当前系统时间冒充原始发布时间或历史可用时间。

## 6. 身份、去重和版本规则

### 6.1 身份

- 搜索候选先形成 observation。
- 只有发布来源已识别且至少有来源原始 ID 或规范 URL，才允许创建 news_item。
- 身份唯一范围为同一 source；不同来源转载默认保留为不同新闻主体。
- 来源原始 ID 优先；规范 URL 是身份凭据/别名。
- 标题、摘要、发布时间相似度和内容指纹只作辅助匹配，不单独构成主体唯一身份。
- Hash 仅用于索引；命中后必须核对原始值。
- source-native-id 与 normalized-url 指向不同主体时返回 identity_conflict，不自动合并。
- URL 变化时旧 identifier 标记 historical，新增 active identifier，不覆盖历史凭据。
- identifier 状态为 active/historical/invalid，不维护冗余 is_primary。

### 6.2 版本

- `source_version` 是发布者外部修订标识；缺失则 NULL。
- `version_no` 是系统内部单调递增版本号。
- `version_fingerprint` 由标准化权威元数据和获准保存的内容确定，不包含抓取/观察/标准化时间。
- 相同 fingerprint 的重复抓取不创建新版本。
- 只有采集时间、Provider 或观察时间变化不创建新版本。
- 标题、摘要、规范 URL、语言、发布时间或获准正文实质变化时创建新版本。
- 外部 source_version 变化作为来源修订证据保存；内容 fingerprint 未变化时不单独制造内容版本。
- 当前版本按最大 version_no 获取；旧版本不可原地覆盖。

## 7. 来源授权与内容安全

Provider 与 publisher/source 是两个不同主体，策略不能互相替代。

最终允许范围是以下条件的交集：

1. Provider 输出策略：用途、请求内处理、缓存、持久化、展示/再分发、保留期限。
2. Source 级策略：发布者默认许可。
3. Endpoint 级策略：入口独立条款或访问限制。
4. 当前调用目的及内容类型。
5. 具体字段/正文载荷的保留期限和删除要求。

Source 策略默认作用于所有 endpoint；Endpoint 策略默认只能收紧。若要放宽，必须有独立书面授权证据、范围和有效期。来源策略未核实，endpoint 不能自动 approved。

已核查的工程边界：

- Brave Search API：不长期缓存/持久化 Search Results。
- Bocha：在存储权利书面确认前不持久化。
- Tavily：Provider 输出保存许可仍需叠加原始发布者内容授权。
- SEC/EDGAR：遵守官方接口要求、User-Agent 和速率限制。
- HKEX、SSE、SZSE、CNINFO：未取得足以覆盖系统自动采集/长期留存的许可前保持 pending/restricted。
- SearXNG 和 NewsNow 不会自动授予上游来源内容留存权。

实际账号套餐、补充协议和书面许可尚需核验；这是对应 Provider/来源真实接入及内容留存的门槛，不应被误报为已授权，也不阻断当前架构设计评审。

网络层仍须防 SSRF：限制协议、凭据 URL、私网/保留 IP、DNS rebinding、重定向、代理、超时、响应体大小和并发。网络安全校验不等于版权授权。

## 8. Application 用例契约

### SearchCandidates

- 输入：query、可选 canonical symbol、query dimension、时间窗口、结果上限、调用上下文。
- Provider 选择和凭据由内部配置控制，不允许任意 URL 或调用方绕过授权。
- 每个 Provider/query dimension 独立返回 success、empty、failed、skipped。
- 聚合结果区分 success、empty、partial、failed、skipped。
- Provider 请求失败与候选质量拒绝分开表达。
- 默认不抓取全文、不持久化、不跨请求缓存。

### CollectSource

- 输入必须是已登记 source_id + endpoint_id 和受控采集参数；不接受任意用户 URL。
- 执行前计算 Provider/Source/Endpoint/用途/保留期限的授权交集。
- 按 fetch、parse、normalize、identify 阶段返回业务结果和失败/跳过原因。
- 未授权时不得执行受限抓取或内容保存；缺少身份凭据时只形成 observation，不创建 news_item。

### 纯业务能力

- `NormalizeCandidate`：纯转换，不访问网络或存储。
- `IdentifyCandidate`：identified/unidentifiable/conflict，附身份命中证据。
- `AssessBusinessQuality`：valid/degraded/invalid/unknown，附字段级原因。
- `ResolveSecurityRelations`：输出关系断言及证据；标题/正文匹配、规则或模型推断默认 proposed。

调用级不可继续错误用 Go error 表达；部分成功、跳过和质量降级使用结构化业务结果表达。不得吞掉单 Provider 失败，也不得以 HTTP 状态码代替领域状态。

## 9. 质量与状态分层

| 维度 | 状态/语义 | 归属 |
|---|---|---|
| Provider 任务 | success / empty / failed / skipped | Provider/Application 业务结果 |
| 搜索聚合 | success / empty / partial / failed / skipped | Application 业务结果 |
| 新闻质量 | valid / degraded / invalid / unknown | 新闻 Domain |
| Freshness | fresh / stale / unknown / not_applicable | 新闻 Domain |
| 授权 | approved / restricted / pending / revoked | 新闻策略 |
| 新闻主体生命周期 | active / withdrawn | news_items |
| 关系断言 | proposed / confirmed / rejected / retracted | news_relations |
| 正文载荷 | available / restricted / expired / purged | news_item_contents |
| 通用执行诊断 | passed / failed / skipped 等 | 未来公共服务 |

搜索 HTTP 200 不代表候选有用；候选有效不代表正文可抓取；正文抓取成功不代表解析、身份、关联或保存授权通过。

## 10. 阶段计划和门槛

| 阶段 | 目标 | 约束 |
|---|---|---|
| N1-A | 业务架构和契约冻结 | 当前完成内部收敛，待用户评审 |
| N2 | Domain、Application ports、Provider Adapter | 固定响应样本/fake HTTP server；不依赖真实 Key，不做持久化 |
| N3 | 来源抓取、解析、标准化和时间处理 | 仅对已批准来源；未确认授权的来源保持 skipped/restricted |
| N4 | 身份、去重、版本、证券/行业关联 | fake Repository/内存测试替身；不声称生产存储 |
| N5 | 历史 Python 数据价值评估 | 可选；未经单独批准不导入 |
| N6 | 公共 DB/Redis/通用诊断接入 | 等公共服务契约和实现稳定 |
| N7 | 持久化、迁移和全链路验收 | 依赖 N6 |

N1-A 评审通过并获得用户确认后才进入 N2。当前不修改业务代码，不运行 Provider Live Probe，不执行数据库迁移。

## 11. 当前风险和处理

| 风险 | 处理 |
|---|---|
| Provider 输出留存权不清楚 | 按 Provider policy 分用途控制；未确认持久化权则禁止持久化 |
| 发布者授权不清楚 | Source/Endpoint 默认为 pending，不因 URL 可访问而放行 |
| 搜索 Provider provenance 丢失 | Adapter 首层保留 provider_code，不映射旧 NewsItem |
| 发布时间被误用 | 原始文本、时区、解析状态和 available_at 证据分开 |
| 转载被误合并 | 同 source 内身份识别；跨 source 保留独立主体 |
| 内容修订覆盖历史 | 追加版本和 fingerprint，不原地覆盖 |
| 证券/行业主数据尚未统一 | 使用 canonical symbol + market/exchange；行业保存分类快照 |
| 公共服务尚未就绪 | 暂不实现 DB/Redis/诊断，不自建替代方案 |
| 旧服务仍在仓库 | 新模块不依赖、不兼容、不擅自删除无关旧服务 |

## 12. 评审确认项

请重点确认以下架构决策：

1. 是否同意根 Go Module 原生实现、`internal/news/` 目录边界及单进程部署。
2. 是否同意不兼容未上线旧 Python/go-core 新闻链路、不创建共享 Provider Module。
3. 是否同意 11 个业务实体作为逻辑模型候选，执行记录和物理 DDL 延后公共服务阶段。
4. 是否同意 Provider 与 publisher 分离、Source/Endpoint 策略继承及授权交集规则。
5. 是否同意时间、身份、版本、关系断言和 Application 状态语义。
6. 是否同意 N2 先实现无 DB/Redis 的 Domain、Application 和 Provider Adapter，N3/N4 再逐步补齐采集与业务规则。

**确认后才进入 N2。** 如有不同意见，应在本报告对应章节调整后再冻结，不以先写代码的方式倒逼架构接受。
