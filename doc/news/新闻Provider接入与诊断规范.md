# 新闻 Provider 接入与诊断规范

> 版本：设计草案 v0.1  
> 状态：Provider 与新闻业务质量契约草案；通用诊断延期至公共服务阶段

## 1. Provider 分类

必须区分“发现新闻的搜索/聚合 Provider”和“提供原始信息披露的来源”。

- 搜索/聚合 Provider：例如 Bocha、Tavily、Brave、显式配置的 SearXNG。
- 官方披露来源：监管机构、交易所、法定信息披露平台、上市公司公告。
- 媒体/行业来源：提供新闻报道或行业信息的来源。

Provider 的接入成功不代表新闻内容权威，也不代表正文完整。

## 2. 复用原则

Provider Adapter 直接实现于根模块 `internal/news/adapters/provider/`。`go-core/internal/adapters/newssearch/` 仅作为协议和历史行为参考，不作为新模块依赖；只迁移必要的请求/响应协议处理，并按新领域契约重构。不得复制旧 Searcher 的缓存、聚合、Python fallback 或旧 DTO 结构。

## 3. 统一适配结果

适配层应尽可能提供：
- provider/source 标识。
- 来源原始 ID（若有）。
- 标题、摘要、规范 URL。
- 来源声明的发布时间及原始时间文本。
- 作者/发布机构（若有）。
- 原始响应证据或可审计引用。
- Provider 请求时间、响应时间和耗时。
- 结果完整度和字段缺失信息。
- 错误分类、限流信息和可重试建议。

原始响应应按合规策略保存或记录受控证据引用；不能无条件持久化所有第三方响应正文。

## 4. 错误与状态

以下状态可用于新闻业务调用的临时结果表达，不代表新闻模块当前建设诊断服务或持久化诊断历史。通用诊断状态契约由公共服务统一定义：
- available：探测成功且能力可用。
- degraded：部分能力可用或质量不足。
- unavailable：确认不可用。
- skipped：因未配置、无凭证或策略跳过。
- unknown：没有足够证据判断。

跳过不等于失败，也不能标记为可用。认证失败、配额限制、限流、超时、网络错误、响应格式错误、空结果和内容解析失败应分别记录。

未来公共诊断服务可统一承载对象、阶段、耗时、错误、计数、质量和 Freshness 等诊断报告字段；新闻模块当前只返回业务调用所需的结构化错误和质量原因，不生成诊断报告或保存诊断历史。密钥、Token、Cookie、Authorization Header 和未获准正文始终不得进入日志或结果。

## 4.1 新闻诊断分层与复用边界

根仓库 `services/market-data` 已有 Provider Probe、Validator/Reporter、Freshness Evaluator、Probe History 和存储检查。它们面向行情 Provider 和行情质量，不应由新闻模块直接依赖其内部 model/service/repository 包，也不能把行情 Probe 的结果直接解释为新闻诊断。

新闻模块当前只实现自身业务规则：Provider 输出权利与 Source/Endpoint 来源授权交集、离线 URL allowlist、候选字段完整度、时间可信度、身份匹配质量、内容完整度和证券关联证据。通用超时/限流/脱敏/诊断框架及执行证据持久化由公共服务统一提供，新闻模块不通过根 Store 自行保存诊断记录。

### 诊断层次

| 层次 | 检查对象 | 示例 |
|---|---|---|
| 配置诊断 | Provider/来源配置 | 是否启用、凭证是否配置、策略是否批准、URL 是否满足安全约束 |
| 连通性诊断 | Provider/endpoint | DNS/TLS/HTTP、超时、响应码、限流 |
| 契约诊断 | Provider 响应 | JSON/RSS/Atom 格式、必需字段、分页/响应结构 |
| 采集诊断 | fetch | 状态码、响应大小、重定向、内容类型、耗时 |
| 解析诊断 | parse/normalize | 编码、HTML/Feed 解析、时间解析、字段完整度 |
| 身份诊断 | deduplicate | 原始 ID/URL 命中、Hash 碰撞核验、身份冲突、重复率 |
| 持久化诊断 | persist | 新建/更新/跳过/冲突数量、事务失败、幂等结果 |
| 关联诊断 | relation | 证券/行业命中、证据类型、未匹配和冲突数量 |
| 质量/新鲜度 | 标准化结果 | 时间可信度、内容完整度、来源权威性、采集延迟、Freshness |

### 业务结果与通用诊断（后者延期）

- 新闻业务调用可返回阶段性业务结果、部分成功信息和错误原因，供调用方决策；不持久化为诊断历史。
- 当前不实现主动 Probe、被动诊断采集、诊断报告和诊断查询。
- Provider 的业务请求仍需有合理超时、并发和成本约束；统一客户端/限流能力待公共服务提供后接入。
- CI 使用固定样本和 fake，不依赖真实 Provider Key 或真实配额。
- 公共服务就绪后，再统一定义 Probe、诊断状态和执行证据契约。

### 诊断状态不能互相覆盖

- 检查执行状态：passed/degraded/failed/skipped/unknown。
- Provider/来源可用性：available/degraded/unavailable/skipped/unknown。
- 数据质量：valid/degraded/invalid/unknown。
- Freshness：fresh/stale/unknown/not_applicable。
- 授权策略：approved/restricted/pending/revoked。

例如，Provider 连通性 passed，但搜索返回零条有效候选，连通性仍是 passed，搜索质量可为 degraded；授权 pending 时，即使正文抓取成功，持久化仍必须 skipped/restricted，不能算整体可用。

## 5. 可靠性与安全

- 设置连接/读取超时、并发限制、重试上限和退避策略。
- 对 429 和服务端错误遵循限流与退避。
- 不对不可重试的认证/参数错误盲目重试。
- 响应大小、正文长度和重定向次数设置上限。
- 禁止从用户输入直接抓取任意 URL。
- 日志脱敏，不记录凭证。
- 记录来源授权、采集频率和内容保留限制。

## 6. 测试策略

- 单元测试使用固定响应样本。
- 契约测试覆盖字段缺失、异常时间、重复结果和错误映射。
- 默认 CI 不依赖真实 Provider 凭证，不消耗真实配额。
- Live Probe 应明确标记为可选/受控测试。
- 未配置 Provider 应报告 skipped，而不是 failed 或 available。
- 真实环境验收需记录时间、配置状态（不记录密钥）、请求能力、结果数量、质量和错误摘要。

## 7. N1 核查清单

- 当前 Provider 列表和注册方式。
- 配置与密钥读取路径。
- 统一 HTTP 客户端、超时、重试和限流。
- 现有 Probe/Validator/Reporter/Freshness 能力。
- 错误状态枚举与 API 返回约定。
- 是否已有官方公告适配器及其授权/数据边界。

---

## 8. N1 现有能力盘点（历史参考，2026-11-05）

当前 Go 新闻搜索实现位于可选迁移路径的 go-core/internal/adapters/newssearch，包括 Bocha、Tavily、Brave News、显式 SearXNG、进程内缓存、查询计划、过滤排序和 Python fallback。该实现仅作协议与历史行为参考；新模块迁移必要的 HTTP 协议处理并按根模块新契约重构，不复用旧 Searcher，也不复制其聚合、缓存、过滤排序和 fallback。

market-data 已有公告 Provider 和公告 HTTP 能力，Go Core 通过 StockAnnouncementSource 调用。该公告来源可作为官方披露候选适配器；分页当前受 Searcher 限制为第 1 页 100 条，需在新闻采集接入时评估完整性，不要误认为已实现全量采集。

market-data Probe 的 connectivity、contract、quality、freshness、cross_provider、storage 检查，以及 Validator/Reporter/Freshness/Probe History 可作为诊断架构参考。其数据结构和检查函数目前面向行情 Provider，不能直接拿来报告新闻抓取/解析/去重/持久化质量。新闻阶段应独立记录结果，并尽量复用底层日志、脱敏、超时、状态分类和观测方式。

现有 Searcher 只对有效发布时间候选排序，当前无原始正文采集和内容版本存储。新采集链路必须区分搜索候选返回、原文抓取、解析标准化、去重版本、持久化各阶段，不可把搜索 API 成功作为新闻入库成功。

---

## 9. N1 来源策略与安全边界补充（2026-11-05）

旧 Python 采集服务已有 HTTP(S) URL 校验、凭据 URL 拒绝、私网/保留 IP 拦截、DNS rebinding 防护、重定向校验、代理禁用、超时/响应体大小限制和错误脱敏。这些是网络安全措施，不代表来源授权合规。

当前没有发现逐来源 license/terms URL、robots 策略、允许抓取内容范围、全文保留期限、删除请求和政策复核时间等字段/执行机制。RSS/Atom/NewsNow 模板只能视为候选来源配置，不可自动视为允许全文抓取或长期保存。新模块应增加来源策略元数据和审核状态；在许可未确认前只保留必要元数据、短摘要和原始链接，并为删除/到期清理留接口。


## 10. 来源授权矩阵

逐 Provider 和原始发布来源的条款核查见《新闻来源授权矩阵》。Provider 输出使用权与原始发布来源内容权利必须独立判断；未确认许可的来源默认 pending，不得自动抓取或持久化。

## 11. 公共服务延期决策补充（2026-11-05）

本规范中第 4、4.1 节原先描述的 Probe、诊断报告、被动诊断历史和诊断查询均为未来能力设计，不属于新闻模块当前开发范围。当前新闻模块不实现诊断服务、诊断存储、主动 Probe 或诊断 API。

新闻模块仍必须实现业务层错误与质量表达，包括来源授权不允许、候选字段缺失、发布时间未知、正文抓取/解析失败、身份冲突、重复/转载、证券未匹配等。它们是业务结果，不应伪装成通用诊断状态。未来由公共服务把新闻业务结果纳入统一诊断体系。


## 12. Provider provenance 与共享适配器历史方案（已废弃，2026-11-05）

当前 go-core 的 Searcher 会把多个 Provider 结果合并并映射为旧 NewsItem，最终 DTO 不保留搜索 Provider 身份；NewsItem.Source 是发布来源字段，不是 Provider。当前 10 分钟缓存也会缓存混合 Provider 输出。因此现有 Searcher 不能直接作为新闻持久化入口。新模块必须在 Provider Adapter 边界保留 provider_code，不得先映射旧 DTO 再补身份。

【已废弃的历史方案，不得实施】根服务与 go-core 是两个独立 Go module，根模块不能直接 import `go-core/internal/**`。当时因此建议抽取中立共享 Go library module，由根新闻模块和 go-core 双向适配。用户已明确不兼容未上线旧服务，该共享方案已取消；当前按第 13 节直接将必要协议处理重构到根模块。

（以下共享候选设计已废弃，仅保留历史背景，不再建设共享 Go Module。）

共享候选必须保留：
- `provider_code` 和可用时的 `provider_item_id`。
- 发布来源名称/域名（publisher source），与 provider_code 分开。
- 原始 URL、原始发布时间文本、标题、摘要和字段缺失情况。
- 当前请求的观察时间与查询维度。
- Provider 单项错误/部分失败信息，但不落入通用诊断历史。

Provider 输出策略必须在缓存和持久化之前执行。Brave 只允许必要的短暂运行期使用，不得长期缓存或写库；博查在存储权利书面确认前不持久化；Tavily 的 Provider 层许可仍需叠加发布者授权。共享层默认不缓存混合 Provider 结果。



## 13. 最终 Provider 实施决策（2026-11-05）

本节替代此前第 2、8、12 节中要求根模块与 go-core 共享 Provider Library、保留旧 Searcher 兼容和 Python fallback 的建议。

- 目标运行时只有根 Go Module；Provider Adapter 直接实现于 `internal/news/adapters/provider/`。
- 不创建独立共享 Go Module，不调整 go-core Docker build context，不引入本地 replace。
- 现有 go-core Searcher 仅供协议行为参考。按新领域契约迁移必要的 Provider 请求/响应解析，不复制旧聚合、旧 DTO、旧缓存、旧 fallback。
- 不保留旧 `NewsSearcher` 兼容层或 Python Bridge fallback；整体服务尚未上线，不为旧服务增加兼容成本。
- 搜索 Provider 和官方公告源采用不同端口；publisher 与 provider 身份分离。
- Provider 候选保留 provider_code、provider_item_id、publisher、原始 URL、原始发布时间文本、observed_at、query dimension 和字段缺失信息。
- Provider 执行结果按实际 Provider/query 任务返回 success、empty、failed、skipped 及错误类别；部分成功必须保留失败明细。
- 首期不跨请求缓存，不缓存混合 Provider 结果。Brave 不长期缓存/持久化；博查书面许可未确认前不持久化；Tavily 输出仍叠加发布者许可。
- 不为旧 go-core/Python 调用方设置兼容验收；删除旧新闻集成前先核实仓库内引用，不触碰无关模块。

实施时由根模块自己的构建、测试和部署流程负责，不为兼容旧嵌套模块改变仓库布局。

## 14. 公共平台 HTTP/Diagnostics 接入约束（2026-11-05）

- Provider/Source Adapter 只依赖注入的 Transport Port；不自行创建生产级通用 HTTP Client、连接池、统一重试器或通用诊断 Runner。
- Provider API 配额、业务并发、请求成本和 Provider-specific retryability 属于 News/Provider Adapter；连接、TLS、总 Deadline、响应体上限、通用重试预算和 Trace 传播由公共 HTTP Client 承担。
- Source URL 的许可、域名策略、DNS/IP 私网阻断、重定向目标逐跳复核和 DNS rebinding 防护属于新闻采集安全策略；必须与平台 HTTP Client 的底层拨号/重定向能力共同实现，不能只做初始 URL 字符串检查。
- 平台契约未明确 DNS/IP/Redirect 安全钩子前，不执行任意来源真实抓取；固定样本解析和 fake Transport 测试不受此限制。
- News 业务质量状态和 Platform Diagnostics 状态分开保留。News Check Adapter 只把领域结果作为受控摘要/Reason Code 映射给公共 Runner，不把新闻正文、Provider snippet、原始响应或 Secret 放入 Check Attributes。
- 平台 failed 表示 Check 执行器/实现自身异常；新闻内容 invalid 应作为领域质量结果呈现，不得伪装为 Check 内部失败。


## 15. N2 首批 Provider Adapter 实施状态（2026-11-05）

根模块 `internal/news/adapters/provider` 已建立 Bocha、Tavily、Brave News、SearXNG 的首版协议 Adapter，统一通过 Application 定义的窄 HTTPDoer 注入传输能力。

- Bocha：使用 Bearer 鉴权；仅接受响应业务码 200；保留原始 item ID、siteName、siteDomain（仅当 Provider 明确返回）及 datePublished 原文。
- Tavily：使用 Authorization Bearer Header；请求 news topic 和基础搜索深度；保留 published_date 原文。日期只有日粒度且无时区时，不强行转换为精确时间点。
- Brave News：使用 X-Subscription-Token；published_date 才作为发布时间候选；page_age 不作为发布时间。
- SearXNG：只支持显式配置实例；保留 Provider 身份和原始时间字段，不把 engine 名称当作发布者。
- 所有 Adapter 保留 provider_code；publisher_name/publisher_domain 与 Provider 身份分离。publisher_domain 不从结果 URL 推断。
- 未知时间原文为空时保持未知；可解析的 RFC3339 时间才填充 PublishedAt，其他格式保留原文供后续标准化阶段处理。
- HTTPDoer 由外部注入；公共平台 HTTP Client/BoundDoer 已合并，但 News 尚无生产逐 endpoint 注入、真实 Provider 网络和来源授权验收。不得自行创建生产 Client/连接池/通用重试器/限流器/跨请求缓存；响应大小、Deadline、TLS、重定向和 DNS/IP 安全由平台 BoundDoer 负责。

fake Transport 测试已在根模块 GitHub Actions 中执行通过，覆盖各 Provider 的基本字段/鉴权语义、429 Retryable 候选、响应结构错误路径等。提交 7e55cf681f86c762aaa3d2e6a914495c162d2898 对应的根模块 gofmt、go test ./...、go test -race ./...、go vet ./... 和分析服务镜像构建通过；未执行真实 Provider 网络请求。


## 9. N3 Provider / Source / Endpoint 授权评估实现

- 新闻 Domain 已提供纯函数策略评估，不依赖数据库、Redis、平台 Diagnostics 或真实 HTTP。
- Provider 输出策略独立表达允许用途、允许内容类型、请求内处理、跨请求缓存、持久化模式及各自最大保留期限。
- Source 策略是发布者默认规则，必须明确访问方式、用途、内容类型、存储模式、最大保留期限和授权证据。
- Endpoint 策略通过 nil 继承 Source；非 nil 空集合代表明确禁止；已有 Endpoint 策略只能缩小 Source 权限或保留期限。当前未实现扩权授权流程，任何扩权声明均拒绝。
- 最终候选使用决策取 Provider 输出权利、Source 和 Endpoint 约束的交集；Provider 获准保存输出不代表原始发布者允许抓取/保存正文。
- Pending、Revoked、未知状态、缺少授权证据、过期策略均默认拒绝；Restricted 只允许显式列出的范围。
- EvaluateSourceURL 只执行离线 URL 结构、协议、精确主机和端口校验；不执行 DNS 解析、IP 特殊地址判断、重定向或实际拨号。DNS rebinding 和逐跳网络安全仍由 PLAT-008 负责。

Brave/Bocha/Tavily 等具体 Provider 的策略值必须按实际账号套餐、书面协议和本项目核验记录配置，不能仅凭 Provider 名称默认批准。


### 9.1 内容类型与存储模式约束

- persistent_metadata 仅允许 metadata/title；persistent_excerpt 仅允许 snippet/abstract；persistent_full_text 仅允许 full_text/attachment。
- none/transient 不代表持久化授权；跨请求缓存另由 Provider 的 AllowCrossRequestCache 和 MaxCacheRetention 独立控制。
- 即使 Provider 和 Source 分别允许某项内容及存储方式，内容类型与存储模式不匹配仍拒绝；不得通过将全文标记为 metadata 绕过全文留存限制。


## 2026-11-06 平台合并后 HTTP/Diagnostics 基线复核

- `internal/platform/httpclient.BoundDoer` 已进入当前共同分支，满足 News `app.HTTPDoer` 的窄接口；Provider/Source Adapter 无需自建 HTTP Client。生产装配仍需按每个固定 endpoint 单独 Bind。
- `internal/platform/diagnostics.Runner` 已有平台 L1 原语和测试证据，但 Runner 不提供历史存储或 HTTP 路由；News 领域 Check 和 Server 路由接入仍待完成。
- 当前 `provider/http.go` 通过注入的 HTTPDoer 读取响应体；生产响应大小上限依赖平台 BoundDoer，不能把 fake/raw HTTPDoer 作为生产实现。
- 平台 L1 CI 通过不代表 News 实际注入、真实 Provider 请求、生产 DNS/出口或来源授权验收通过。


## 2026-11-06 项目级契约与 News Diagnostics 接入计划

项目级统一诊断规划和 Platform 现有 Runner 已核对。News 后续接入按以下边界执行：

1. Platform Runner 负责 Check 的超时、并发、panic 安全处理和公共状态聚合；News 负责 Check 定义、领域 reason_code、证据引用和业务质量解释。
2. 首批 Check 计划覆盖配置/凭证是否已配置、Provider 配置与能力状态、Source/Endpoint 授权状态、Fetch/Parse/Normalize、Identity/Deduplicate、Persist 和 Relation。未配置的可选 Provider 必须 skipped/not_configured，不标记失败或可用。
3. Connectivity/contract/quality/freshness/authorization 分别表达；ProviderOutcome 的 empty/partial/failed/skipped 不得被 Platform Status 覆盖；quality=invalid 不得被映射为 Check failed。
4. Check 默认只读，不发起真实 Provider 请求、不消耗配额。主动 Probe 必须由受控运维调用明确触发，并满足来源授权与网络安全门禁。
5. Observation 的 attributes 仅允许白名单低敏计数/枚举；禁止正文、snippet、原始响应、URL Query、Token、Cookie、DSN 和模型上下文。
6. 不新增 News Diagnostics HTTP 路由、Runner、Registry 或通用历史库。统一诊断路由的认证和注册由 Server Owner 负责；News Ingestion Run/Stage 若有真实采集触发需求，其事实记录仍由 News 持有。
7. PLAT-CTR-004 仍为 `pending_evidence`；News Check Adapter 的 Schema/Golden/Runner Contract Test 和状态映射证据通过后，才申请对应调用点 A2。
