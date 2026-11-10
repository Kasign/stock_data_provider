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


## 2026-11-06 当前 HTTP 适配 API 修正

本规范历史段落中的 `BoundDoer` / `Client.Bind()` 描述已过时。当前 Platform HTTP Client API 为 `Client.Do(ctx, request, DestinationPolicy)`。News Provider 使用 `provider.NewPlatformHTTPDoer`，Source 使用 `source.NewPlatformHTTPDoer`；每个 endpoint 独立绑定精确 Host/Port 策略。响应体大小、总 Deadline、TLS、DNS/IP 校验和拨号仍完全由 Platform Client 执行。News 不创建裸 HTTP Client，不接管共享 Client 生命周期。


## 2026-11-06 接入状态更新

Provider 适配器已落地 `provider.NewPlatformHTTPDoer`；Source 适配器已落地 `source.NewPlatformHTTPDoer`。二者均固定单个 endpoint 的 DestinationPolicy，拒绝重定向和平台自动重试；Platform Client 负责 DNS/IP、TLS、响应大小和 Deadline。Source endpoint 保留 query 参数，Provider endpoint 配置仍拒绝 query，避免将凭证放入 URL。生产组合根按每个 Provider/Source endpoint 独立注入。


## 17. 多供应商统一协议层专项复核与实施方案（2026-11-10）

本节是当前多供应商协议层的实施依据，结合当前分支代码复核结果修正早期“从零建设统一 Provider 协议”的表述。目标是演进现有协议，不重复建设平行模型。

### 17.1 当前代码事实

| 范围 | 现有实现 | 当前缺口 |
|---|---|---|
| 搜索 Provider 端口 | `app.Provider`：`Code + Search` | 没有显式描述能力/版本；接口仅覆盖搜索 |
| Provider 集合 | `ProviderSet`、`StaticProviderSet` | 没有独立 Registry；没有重复 Code 的集中校验 |
| 搜索请求 | `domain.SearchRequest`：Query、QueryDimension、Limit | 请求上下文较少；暂不应添加没有调用场景的通用参数 |
| 搜索结果 | `domain.SearchResult`、`ProviderOutcome` | 已保留聚合状态和 Provider 级结果；不是空白，需要核对字段缺口后增量演进 |
| 标准候选 | `domain.Candidate`、`NormalizeCandidate` | 已保留 Provider 身份、发布者身份、原始时间、观察时间及字段缺失信息 |
| 错误 | `app.ProviderError` | 错误码仍由 Adapter 分散产生，稳定分类与映射规则需要收敛 |
| 供应商适配 | Bocha、Tavily、Brave、SearXNG | 当前均实现 Search；没有证据表明它们支持统一 Fetch/Discover |
| 配置装配 | News Config + `NewPlatformProviderSet` | 配置字段和装配代码按供应商显式维护；不是动态注册中心 |
| 网络传输 | `app.HTTPDoer` + Platform BoundDoer 适配 | 生产逐 endpoint 绑定仍由 Server 组合根完成 |
| Feed 来源采集 | `CollectSource`、`SourceCatalog`、`SourceTransport`、RSS/Atom Parser | 属于已登记发布来源采集，不是搜索 Provider 的另一种实现 |

当前四个搜索 Provider 的实现位于 `internal/news/adapters/provider/providers.go`；搜索执行和结果聚合位于 `internal/news/app/search.go`；公共端口位于 `internal/news/app/ports.go`；标准候选及搜索结果模型位于 `internal/news/domain/types.go`。

### 17.2 架构修订决定

1. 不新建与现有 `app.Provider` 平行的万能 Provider 接口。
2. 当前搜索能力继续使用独立的 Search Provider 端口；未来确实出现新闻正文 Fetch、第三方推送等不同能力时，再按真实用例增加能力专属端口。
3. RSS/Atom 继续使用 Source Collection 端口。它代表经过登记、授权和 Endpoint 约束的发布来源，不并入搜索供应商 Registry。
4. 搜索结果优先演进现有 `SearchResult`、`ProviderOutcome`、`Candidate`；不因泛型形式更漂亮而新增通用 `ProviderResult[T]`。
5. Provider 输出策略、Source/Endpoint 授权交集、Observation 追踪和 Evidence/PIT 规则保持现有边界，不迁入 Adapter 或 Manager。
6. Platform 继续提供 HTTP Client、Fault、Request ID 和 Diagnostics 原语；News 只映射新闻供应商业务语义，不重建通用设施。

### 17.3 协议目标结构

- Search Provider Contract：稳定的 Provider 身份、搜索请求、标准候选、单 Provider 执行结果和稳定错误分类。
- Provider Catalog/Registry：启动时注册和校验身份唯一性；按需提供能力元数据。首期不做动态插件加载。
- Search Orchestrator：执行现有多 Provider 搜索、保留逐 Provider Outcome 和聚合状态；调度策略与协议定义分离。
- Source Collection Contract：继续负责 Source/Endpoint 登记、访问策略、RSS/Atom 采集和解析。
- Normalization：沿用 Candidate 标准化逻辑，明确 Provider 原始字段到 Candidate 字段的映射和时间可信度。
- Conformance Tests：所有 Search Provider 通过统一契约测试；Source Adapter 使用独立采集契约测试。

### 17.4 明确暂不建设的能力

首期不增加以下无当前用例的抽象或复杂度：

- 将 Search、Fetch、Discover、UpdateCheck、HistoricalSearch 合并成一个万能能力接口。
- 任意 `map[string]any` 请求参数或供应商自定义参数透传到领域层。
- 动态插件发现、热加载和运行期供应商注册。
- 自动评分、智能路由、主备切换、熔断和跨供应商自动合并。
- 跨请求缓存、混合供应商缓存或新的原始响应存储。
- 第二套通用 ProviderResult、通用 Fault 或通用 Diagnostics 模型。

以上能力只有在明确业务调用方、数据证据和验收标准后才能单独立项。

### 17.5 分阶段实施和验收

**P0：现状盘点与方案复核（本轮）**
- 核对所有搜索 Provider、配置、装配、调用和错误路径。
- 单独核对 RSS/Atom Source Collection 边界。
- 更新本规范、N1 Review 报告和任务验收清单。
- 验收：代码路径和缺口均有当前分支证据；不修改业务行为。

**P1：协议细化与冻结**
- 明确 Provider 身份唯一性、请求字段语义、Outcome 状态迁移和错误码映射表。
- 明确是否需要能力元数据；若当前只有 Search，则不引入无效能力抽象。
- 明确分页、部分结果、时间字段、Provider provenance 的现有语义及兼容要求。
- 验收：至少以两个不同响应特征的现有 Provider（例如 Bocha 与 Brave）逐字段走查协议；Search 与 RSS/Atom 两条路径边界明确。

**P2：最小协议基础实现**
- 只实现 P1 冻结且当前代码确实缺失的能力。
- 增加 Registry 注册校验（特别是空 ID、重复 ID、nil Provider），或证明现有装配已能保证这些约束。
- 统一错误码映射和 ProviderOutcome 构造，避免各 Adapter/调用处重复解释错误。
- 不改变现有供应商查询参数和业务授权行为。

**P3：单供应商试点**
- 选择一个响应结构/时间字段有代表性的 Provider，建议先选 Bocha 或 Brave。
- 通过新契约测试覆盖请求、响应解析、缺字段、空结果、错误、来源身份和时间语义。
- 与迁移前固定样本结果逐项比较。

**P4：其余搜索 Provider 迁移**
- 按供应商逐个接入同一 Contract Test。
- 清理重复错误映射和旧旁路。
- 逐个确认结果与授权门禁没有退化。

**P5：运行期调度优化（后置）**
- 仅基于实际调用量、配额、失败率和质量对比证据决定是否增加主备、熔断或路由。
- 所有切换保留原始 Provider Outcome；不把切换成功解释为数据完整或质量通过。

### 17.6 不可退化的验收门禁

- Provider Code 与 Publisher/Source 身份始终分离。
- 空结果、跳过、失败、部分成功不得互相混淆。
- PublishedAt、FetchedAt、NormalizedAt、ObservedAt 不得混用或推断。
- Provider 输出许可与 Source/Endpoint 内容许可继续独立评估并取交集。
- 未通过授权策略评估的 Provider Candidate 不得进入持久化。
- 搜索 Provider 结果不得绕过 Observation/Identity/Evidence 既有业务入口。
- Source/Endpoint 网络访问继续使用独立 BoundDoer 和可信 SourceCatalog。
- CI 使用固定样本/fake，不依赖真实 Provider Key 或消耗真实配额。
- 未执行的 Go 测试、MariaDB 集成或真实网络验收不得标记为通过。


### 17.7 Search Provider Contract v1（设计冻结）

本版本是 News 进程内 Search Provider 契约，不是对外 HTTP API，也不承诺跨语言/插件 ABI。

#### 请求语义

| 字段 | 规则 |
|---|---|
| Query | 必填；执行前 TrimSpace 后不得为空 |
| QueryDimension | 可选业务检索维度；仅用于结果归因，不改变 Provider 身份 |
| Limit | 必填；范围 1～100；Provider Adapter 可按供应商上限执行，但不得静默突破请求上限 |
| Context | 必须传播调用方 Context；取消/Deadline 不得伪装成单 Provider 普通失败 |
| 凭证/Endpoint | 只能来自受控配置和组合根绑定，不属于 SearchRequest |

首期不增加 StartAt/EndAt、AsOf、分页游标、任意 Filters 或通用 Options：现有搜索调用没有证明这些字段可被四个 Provider 一致执行。需要历史检索或时间过滤时，先明确调用方与供应商可实现的过滤语义，再扩展能力专属请求。

#### Provider 身份

- `Code()` 是稳定机器标识，必须非空、首尾无空白，并在一个 ProviderSet 中唯一。
- Code 不得由显示名称、发布者名称或文章域名推导。
- ProviderSet 中出现 nil Provider、空 Code 或重复 Code 时，整组注册无效；必须在任何上游网络请求发生前拒绝。
- 现有生产 Provider Code：`bocha`、`tavily`、`brave_news`、`searxng`。新增 Code 需固定测试样本和稳定标识，不复用已删除 Provider 的 Code 表示不同供应商。

首期不要求新增独立动态 Registry。对当前 StaticProviderSet，应用入口执行完整预检即可；只有出现运行期注册/动态启停用例时才拆分独立 Registry。

#### 候选数据语义

| 字段 | 契约 |
|---|---|
| ProviderCode | 搜索供应商身份；由 Orchestrator 依据已校验 Code 绑定，不信任响应自报值 |
| ProviderItemID | Provider 原始条目 ID；没有则为空，不得伪造 |
| SourceItemID | 原始发布来源条目 ID；与 ProviderItemID 分离 |
| PublisherName / PublisherDomain | 仅保留 Provider 明确返回或受信来源目录提供的值；不从 URL 推断 |
| PublishedAtRaw | 保留上游原始时间文本 |
| PublishedAt | 仅在时间格式和时区足以支持精确时点时填写；日粒度/无时区文本保留 Raw，不强转 |
| FetchedAt | 真实网络获取时间；未记录时保持 nil |
| NormalizedAt | 标准化处理时间；未记录时保持 nil |
| ObservedAt | 本次系统观察时间；不得替代 PublishedAt |
| QueryDimension | 归因到本次查询维度 |
| MissingFields | 标准化后按必需字段重新计算，不信任 Provider 自报列表 |

Provider 原始响应结构只存在于 Adapter 内。Candidate 是标准化候选，不是发布者原文或已授权可持久化内容。Observation 写入仍必须经过 Provider Output ∩ Source ∩ Endpoint 策略评估。

#### 单 Provider Outcome 与聚合状态

单 Provider 状态：

- `success`：请求成功且至少返回一个候选。
- `empty`：请求成功但返回零候选；不是故障。
- `skipped`：按配置/策略明确跳过；不是成功，也不是故障。
- `failed`：本次请求执行失败。

Outcome 至少保留现有字段：ProviderCode、QueryDimension、Status、CandidateCount、ErrorCode、Retryable、Duration、ObservedAt。Duration 只表示本次 Provider.Search 调用耗时；ObservedAt 表示该次调用完成/被判定的时间。CandidateCount 是 Adapter 返回并交由标准化的候选数，不代表最终持久化数量或质量合格数量。

聚合状态按以下规则解释：

| 条件 | SearchResult.Status |
|---|---|
| 所有 Provider 均 skipped，或没有可执行 Provider | skipped |
| 没有 success/empty，且至少一个 failed | failed |
| 至少一个 success/empty，同时存在 failed 或 skipped | partial |
| 全部执行成功且候选总数为 0 | empty |
| 至少一个候选且没有 failed/skipped | success |

`partial` 必须保留每个 Provider Outcome；不得只返回成功候选而丢弃失败来源。

#### 错误码与重试语义

- `ProviderError.Code` 是稳定机器可读分类，不得包含 Secret、URL 凭证、响应正文或任意上游错误文本。
- `Retryable` 仅表示技术层可重试候选；不授权 Adapter 或 Orchestrator 自动重试。实际重试预算归 Platform HTTP Client 与 Provider 配额策略共同约束。
- `Skipped` 只用于明确的配置/策略跳过；不能把 401、429、超时或未知错误标记为 skipped。
- 统一错误码最小集合：`provider_unauthorized`、`provider_rate_limited`、`provider_timeout`、`provider_transport_failed`、`provider_request_invalid`、`provider_response_invalid`、`provider_business_rejected`、`provider_upstream_unavailable`、`provider_unknown`。
- HTTP 429 映射为 `provider_rate_limited` 且 Retryable=true；认证拒绝映射为 `provider_unauthorized` 且 Retryable=false；超时映射为 `provider_timeout` 且 Retryable=true；可重试的 5xx 映射为 `provider_upstream_unavailable` 且 Retryable=true；响应解码/结构缺失映射为 `provider_response_invalid` 且默认 Retryable=false。
- Platform Fault 必须先 Validate，再映射到上述 News 错误分类；不得将平台 Fault 的任意 Code 直接当作稳定 News 公共错误码。底层细节只允许进入受控诊断 Reason/Evidence，不进入用户可见错误文本。

#### 分页与顺序

- 当前 SearchRequest 只有 Limit，没有 Page/Cursor；现有四个 Adapter 只证明单次搜索请求，不证明统一跨页能力。
- 首期协议明确为单次请求结果；Adapter 不得声称已完整遍历上游全部结果。
- ProviderSet 顺序决定当前执行顺序；首期保持确定性顺序，不并发改造，以免改变请求成本、限流行为和结果顺序。
- 若未来需要分页，必须定义供应商能力差异、游标生命周期、重复页防护、停止条件和部分失败语义后单独升级协议。

#### 契约测试要求

所有 Search Provider 必须覆盖：
- Code 非空、规范且在集合内唯一。
- 请求方法、鉴权位置、查询参数和 Limit 映射。
- 正常结果、空结果、缺失结果 Envelope。
- ProviderCode 归因和 Publisher 身份不推断。
- 发布时间原文保留及不可信时间不强转。
- HTTP 401/403、429、5xx、网络超时、非法 JSON、业务拒绝的错误映射。
- 不输出凭证或原始响应内容。
- Context 取消传播。

该测试集只验证进程内 Search Provider Contract；RSS/Atom 继续使用独立 Source Collection Contract Test。
