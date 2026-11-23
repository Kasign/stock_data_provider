# News 模块卡点

更新时间：2026-11-18  
维护分支：`feat/news`  
模块代码范围：`internal/news/**`  
模块文档范围：`doc/news/`  
本文件记录 News 继续开发时需要其他模块、真实环境或业务决策配合的事项；它不是 News 模块任务状态看板。News 当前任务和进度仍以 `doc/news/模块总任务规划和进度.md` 为准。

## 1. 当前结论

News-owned 的授权预检、Diagnostics Check 工厂和 Cleaner 一次性执行能力均已完成对应实现与模块内验证（其中 POST-02 的生产闭环仍受外部条件阻塞）。POST-03、POST-04 不再是 News 侧待开发任务；当前没有已确认、可独立推进的 News 生产功能任务。

后续在 `feat/news` 只处理审查或运行中能够复现、且归属 News 的缺陷；不为填充进度虚构功能。当前未闭环事项集中在可信来源注册、实际业务调用方、Server 生产装配、真实授权网络验收、共享 Diagnostics 注册、Cleaner 周期触发责任，以及合格 `available_at` 来源证据。

## 2. 卡点清单

| 编号 | 卡点 | 原因与当前证据 | 责任方/需要决策或提供的内容 | 对 News 的影响 | 解除条件 |
|---|---|---|---|---|---|
| NEWS-BLK-01 | Source/Endpoint 可信注册配置来源未确定 | News 目前有 `StaticSourceCatalog`，但没有已确认的生产 Source/Endpoint 清单权威来源；News Config 无 Source 注册项，Migration 无 Source/Endpoint 目录表。不能自行猜测应该使用配置文件、数据库管理表或其他服务 | Server + News + 实际业务调用方：明确谁维护 SourceID、EndpointID、固定 URL、Publisher 元数据、URL allowlist、启停状态及策略版本；明确配置变更和发布流程 | 阻塞 Feed 生产装配；不阻塞离线采集、授权、解析和诊断开发 | 形成唯一权威配置来源、字段契约、更新/回滚流程和可信数据样例 |
| NEWS-BLK-02 | Feed Collect→Ingest 的实际业务调用方未确认 | 当前 News Search Stage 是搜索候选，不是 Feed 采集；没有已确认的调用方负责选择来源、Purpose、ContentKind、StorageMode、Retention、批次和 IdempotencyPrefix | Researchflow 或其他实际业务模块 Owner：明确是否需要 Feed 采集、触发条件、执行上下文、失败重试和幂等 Prefix 规则；若没有真实场景则继续保持未接入 | 阻塞持续生产采集和端到端写入验收；News 单次用例可继续 | 有明确调用方、真实业务场景及调用契约；不得仅为了复用 News 能力而虚构调用方 |
| NEWS-BLK-03 | Server 尚未装配 Source Feed 生产组合 | News 已有 SourceCatalog/SourceTransport/CollectSource/IngestCollectedSource 和当前策略 Resolver；Server 当前已装配 Provider Search Stage 与 Evidence Query，但尚未装配 Source Feed 组合 | Server Owner：使用共享 Platform HTTP Client/每 Endpoint 独立 DestinationPolicy、News MySQLRepository Resolver 和可信 SourceCatalog，构造 CollectSource/IngestCollectedSource；管理生命周期但不替业务调用方决定触发策略 | 阻塞真实部署运行和生产网络验收；不阻塞 News 侧 Adapter/Port/测试 | Server 组合根提交装配代码，并在目标部署配置中验证依赖注入；由调用方驱动真实 Collect→Ingest |
| NEWS-BLK-04 | 真实 Source 授权与网络验收条件不足 | 没有当前可用于本轮验收的、许可明确的 Source/Endpoint 清单及对应真实部署出口证据；测试替身和离线 Feed Fixture 不能证明授权或真实网络能力 | 实际业务/来源 Owner 提供可核验许可依据、允许的访问方式/内容/存储范围、合法测试 URL；Server/运维提供真实 DNS、出口和部署环境 | 阻塞真实网络采集、授权闭环和 Observation 端到端验收；不能把 skipped/not_comparable 改成 PASS | 对指定 Source/Endpoint 有许可证据、配置快照、真实请求、解析结果和 Ingest 持久化证据 |
| NEWS-BLK-05 | News Diagnostics 的共享 Runner/Registry 生产注册由 Server 负责 | News 只拥有领域 Check 工厂；Server 拥有共享 Runner、Registry 和诊断入口。News 已补 `SourceCollectionPolicyCheck` 并完成 News 模块单测/契约测试；尚未由 Server 注册 | News：完成工厂、依赖接口、四态语义和安全输出；Server：在 Repository/Catalog 等真实依赖装配后注册对应 Check，避免 nil/fake 依赖 | 不阻塞 News Check 开发；阻塞共享 Diagnostics 入口观察到全部 News 检查 | News 提供工厂清单与依赖说明；Server 按真实装配逐项注册并完成集成测试 |
| NEWS-BLK-06 | Cleaner 周期触发方与频率未确认 | News 的一次性、有界 Cleaner、Application 边界测试及 MariaDB 到期/批次/重复/并发/回滚验收已完成；当前只缺周期触发 Owner、频率/重叠执行/停机语义决策 | 实际业务调用方或 Server/运维 Owner：确定触发责任、周期、批次、并发上限、超时、重试、关闭和告警策略。Server 只负责进程级装配/启停，不替业务决定周期 | 阻塞周期运行闭环；不阻塞 News 对 Cleaner 单次执行、边界、并发和取消行为的测试 | 有明确 Owner、运行频率和生命周期契约；真实环境有周期执行记录 |
| NEWS-BLK-07 | 合格 `available_at` 来源证据不足 | 已有 PIT/Authorization 机制，但尚无来源明确声明的首次公开可获取时间证据及内容版本绑定/再利用许可。published_at、RSS pubDate、HTTP Date、fetched_at、observed_at 均不能替代 | 业务/来源 Owner：提供官方字段语义、首次公开可获取时间证据、内容版本绑定方式及再利用许可；News 负责适配和证据保存 | POST-05/POST-06 保持 SHELVED；不得推导时间或写入伪造的 Research Evidence | 来源语义、授权、原始字段、证据 URL、内容版本和可审计样本均经审核通过 |

## 3. 卡点与当前可执行任务的边界

### 不需要等待外部模块的 News 任务

1. NEWS-POST-03 News-owned Diagnostics Check 工厂、状态判定、测试和依赖交接已完成；Server Registry 注册仍是外部卡点。
2. NEWS-POST-04 News-owned 一次性 Cleaner、批次/幂等/并发/失败恢复/Context 取消及 MariaDB 边界验收已完成；周期触发仍是外部卡点。
3. 当前没有需要为了等待其他模块而停下的 News 侧已知代码任务；后续只修复审查证实的缺陷，不虚构新功能。

### 必须等待或协作的工作

1. NEWS-BLK-01/02/03：可信来源清单、调用方和 Server 生产组合。
2. NEWS-BLK-04：合法来源和真实网络环境。
3. NEWS-BLK-05：共享 Diagnostics Runner/Registry 的 Server 注册。
4. NEWS-BLK-06：Cleaner 周期触发责任及频率。
5. NEWS-BLK-07：available_at 合格来源证据。

## 4. 执行原则

- 卡点不等于整个 News 模块暂停；只暂停依赖该卡点的验收环节。
- 不为绕过卡点创建假 Source、假调用方、假授权、假网络成功或虚构周期运行记录。
- 不在 News 分支修改 Server/Platform/其他模块来替代责任方工作。
- 每个卡点解除时更新本文件，并在对应模块任务文档记录精确 SHA 和真实验收证据。
- 当前任务始终以 News 模块总任务规划和进度文件为准；本文件不创建第二套任务看板。
