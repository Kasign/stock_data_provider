# 新闻 Provider 接入与诊断规范

> 版本：设计草案 v0.1  
> 状态：待 N1 核实现有 Adapter 和诊断体系

## 1. Provider 分类

必须区分“发现新闻的搜索/聚合 Provider”和“提供原始信息披露的来源”。

- 搜索/聚合 Provider：例如 Bocha、Tavily、Brave、显式配置的 SearXNG。
- 官方披露来源：监管机构、交易所、法定信息披露平台、上市公司公告。
- 媒体/行业来源：提供新闻报道或行业信息的来源。

Provider 的接入成功不代表新闻内容权威，也不代表正文完整。

## 2. 复用原则

N1 必须先核实并优先复用现有 `go-core/internal/adapters/newssearch/` 能力，以及现有配置、请求封装、限流、超时、诊断和错误分类。

不复制已有 Provider，不为了统一接口而重写稳定实现。只有存在明确契约缺口时才扩展。

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

诊断状态应复用项目现有状态枚举和诊断服务。语义至少需要区分：
- available：探测成功且能力可用。
- degraded：部分能力可用或质量不足。
- unavailable：确认不可用。
- skipped：因未配置、无凭证或策略跳过。
- unknown：没有足够证据判断。

跳过不等于失败，也不能标记为可用。认证失败、配额限制、限流、超时、网络错误、响应格式错误、空结果和内容解析失败应分别记录。

诊断报告应包括对象类型（Provider/source/endpoint/run）、对象标识、能力/阶段、检查时间、耗时、执行状态、可用性状态、错误类别与错误码、脱敏摘要、输入/输出/跳过/失败计数、质量指标、Freshness 证据、授权策略版本引用和可重试建议。诊断报告不得包含 API Key、Token、Cookie、完整 Authorization Header 或未获准保存的正文。

## 4.1 新闻诊断分层与复用边界

根仓库 `services/market-data` 已有 Provider Probe、Validator/Reporter、Freshness Evaluator、Probe History 和存储检查。它们面向行情 Provider 和行情质量，不应由新闻模块直接依赖其内部 model/service/repository 包，也不能把行情 Probe 的结果直接解释为新闻诊断。

新闻模块复用的是成熟的诊断原则和底层约定：超时、错误分类、脱敏、状态语义、证据结构、质量校验和 Freshness 的“源端时间优先”原则。新闻模块在自己的 Domain/Application 中定义领域检查项，并通过既有根 Store 持久化运行证据。

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

### 被动诊断与主动 Probe

- 被动诊断：每次正常搜索/采集/写入都记录阶段耗时、计数、状态和错误，是默认方式。
- 主动 Probe：由内部受控任务触发，检查配置、连通性和响应契约；默认不执行全文抓取、不写入正式新闻、不触发高成本查询。
- 主动 Probe 未配置凭证、策略不允许或检查条件不满足时返回 skipped；只有获得明确失败证据才返回 failed/unavailable。
- 真实 Provider Probe 必须有单项超时、并发上限、频率限制和请求成本控制；CI 使用固定样本和 fake，不依赖真实 Key。
- 新闻诊断当前通过同进程 Application 暴露，不新增新闻 HTTP Handler、公网管理接口或独立诊断服务。

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

## 8. N1 现有能力复用核对（2026-11-05）

当前 Go 新闻搜索实现位于可选迁移路径的 go-core/internal/adapters/newssearch，包括 Bocha、Tavily、Brave News、显式 SearXNG、进程内缓存、查询计划、过滤排序和 Python fallback。新闻模块应通过适配器复用，不复制这些 Provider，也不在 N1 搬迁该包。

market-data 已有公告 Provider 和公告 HTTP 能力，Go Core 通过 StockAnnouncementSource 调用。该公告来源可作为官方披露候选适配器；分页当前受 Searcher 限制为第 1 页 100 条，需在新闻采集接入时评估完整性，不要误认为已实现全量采集。

market-data Probe 的 connectivity、contract、quality、freshness、cross_provider、storage 检查，以及 Validator/Reporter/Freshness/Probe History 可作为诊断架构参考。其数据结构和检查函数目前面向行情 Provider，不能直接拿来报告新闻抓取/解析/去重/持久化质量。新闻阶段应独立记录结果，并尽量复用底层日志、脱敏、超时、状态分类和观测方式。

现有 Searcher 只对有效发布时间候选排序，当前无原始正文采集和内容版本存储。新采集链路必须区分搜索候选返回、原文抓取、解析标准化、去重版本、持久化各阶段，不可把搜索 API 成功作为新闻入库成功。

---

## 9. N1 来源策略与安全边界补充（2026-11-05）

旧 Python 采集服务已有 HTTP(S) URL 校验、凭据 URL 拒绝、私网/保留 IP 拦截、DNS rebinding 防护、重定向校验、代理禁用、超时/响应体大小限制和错误脱敏。这些是网络安全措施，不代表来源授权合规。

当前没有发现逐来源 license/terms URL、robots 策略、允许抓取内容范围、全文保留期限、删除请求和政策复核时间等字段/执行机制。RSS/Atom/NewsNow 模板只能视为候选来源配置，不可自动视为允许全文抓取或长期保存。新模块应增加来源策略元数据和审核状态；在许可未确认前只保留必要元数据、短摘要和原始链接，并为删除/到期清理留接口。
