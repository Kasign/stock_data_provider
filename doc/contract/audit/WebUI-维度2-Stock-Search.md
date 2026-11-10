# WebUI 维度 2：Stock Search 证券名称与代码搜索

**审计代码基线：** `origin/integration` @ `b6dfc85ba214fe16268c8cba780641a8bfe15086`  
**维护分支：** `feat/design_doc`  
**接口：** `GET /api/v1/stocks/search`  
**DTO：** `StockSearchItem`、`StockSearchSnapshot`

## 1. 生产方、协议与装配

- Port：`provider.StockSearchProvider.SearchStocks(ctx, query, limit)`。
- 两个实现：Tencent Smartbox、Eastmoney A 股证券目录。
- Bootstrap 使用独立搜索优先级：Tencent → Eastmoney → 配置中的其他 ProviderPriority；按 ProviderSettings.Enabled 和 StockSearch 双重筛选后注入 DataService。
- 默认配置中 Tencent 和 Eastmoney 均启用 StockSearch；Hithink 默认 disabled，且当前没有 StockSearch 实现。
- 该排序是有意设计：腾讯按关键词查询，避免正常自动补全冷启动时先扫描全市场；Eastmoney 全量目录作为回退。
- Service 统一限制 query 1..64 Unicode 字符、limit 1..50，验证响应身份/来源/版本/采集时间/质量/条目证券身份，按 unknown/stale 语义决定回退。
- Handler：`GET /api/v1/stocks/search`；API Key 由路由认证中间件负责。
- WebUI：`app.js:scheduleStockSearch/loadStockSearch`，输入防抖 260ms、AbortController 取消旧请求、generation 防止过期响应覆盖新查询；API Client 在 `api.js:searchStocks`。

## 2. DTO 与字段语义

### StockSearchItem

- `symbol`：规范证券代码，格式为六位 code + .SH/.SZ/.BJ。
- `code`：六位证券代码。
- `name`：证券名称。
- `exchange`：SH/SZ/BJ。
- 四个字段均为必填，OpenAPI required 与 Go JSON tag 一致。

### StockSearchSnapshot

- `query`：服务端规范化（首尾去空白）后的原始用户查询。
- `total`：当前 Provider 对本次查询报告/保留的匹配候选数，不是全市场上市证券数，也不保证代表完整匹配集。
- `items`：本次返回候选，最多为请求 limit。
- `source`、`source_version`：Provider 和适配器版本。
- `fetched_at`：服务获取目录/搜索结果的时间，不是行情时间。
- `quality_status` / `quality_reasons`：候选目录的可信度及原因。
- source_version、quality_reasons 为 Go omitempty 可选字段；OpenAPI 未将它们列为 required，字段语义与 JSON 输出一致。
- OpenAPI 对 total 的说明是“Provider 报告/保留的匹配候选数”，与两个 Provider 当前行为一致；不能将其解释为完整证券目录总数。

## 3. Provider 实际行为

### Tencent Smartbox

- 请求 `https://smartbox.gtimg.cn/s3/`，参数固定为 v=2、t=all、c=1、q=规范化搜索词。
- 对非 UTF-8 响应按 GBK 解码；解析 v_hint 的 market~code~name 记录。
- 支持中文名称、六位代码和带 .SH/.SZ/.BJ 后缀代码；带交易所后缀时仅将后缀从上游查询词中剥离。
- 依据交易所和代码前缀过滤 ETF、指数、可转债、海外证券等非普通 A 股；统一规范化 symbol；去重。
- 对返回候选再执行名称/代码包含匹配；完全相等优先，其次前缀，再其次包含；保留上游顺序作为同级排序。
- Tencent Smartbox 不返回可核验的匹配总数，snapshot.total 是本次解析并匹配到的候选数，质量固定 degraded，原因 `provider_search_total_not_reported`。
- 无匹配 A 股候选时返回错误，促使 Service 尝试 Eastmoney；不能把 Smartbox 空候选直接当成已确认证券不存在。
- 不持久化查询结果；每次请求均访问 Smartbox。

### Eastmoney

- 从 Eastmoney A 股证券目录接口读取字段 f12/f13/f14，分页拉取完整目录；每页大小和最大页数受常量限制，目录总量超过安全上限直接失败。
- 多页之间检查源端 total 是否变化；若变化，认为无法证明分页来自同一完整目录并拒绝。
- 检查每页行数不超过 total 推导的预期行数；中途空页会降级并标记目录不完整。
- 规范化 symbol，按证券代码与市场字段识别 A 股，过滤 ETF、指数、B 股及市场字段矛盾行；过滤记录导致覆盖不完整时标记 degraded。
- 目录缓存 TTL 为 6 小时；使用互斥锁和 loading channel 合并并发刷新。刷新失败时在 retry cooldown 内可使用已有缓存，并标记 stale；没有可用缓存时返回错误。
- 查询匹配规则支持 name/code/symbol 的完全匹配、前缀匹配和包含匹配；同级按名称、symbol 排序，结果稳定。
- 返回的 total 是当前缓存目录中所有匹配项数量，items 按 limit 截断。
- 目录没有源端快照令牌；服务端 fetched_at 只表示本次获取时间，不应解释为源端目录发布时间。

## 4. Service 校验、回退与缓存

- query 去除首尾空白，不能为空，最多 64 个 Unicode 字符；limit 默认 10，最大 50。
- 校验 snapshot.Query 与请求 query 一致；source、可识别 source_version、fetched_at 必须存在。
- 校验 total 非负且不小于 items 数；items 数不得超过 limit。
- 仅接受 valid/degraded/stale/unknown 四种质量状态。
- 每个条目必须具备 name/code/exchange；symbol 必须是规范化值，且 code/exchange 必须与 symbol 两段一致；同一响应不允许重复 symbol。
- Provider 未填 source/query/fetched_at/quality_status 时，Service 会补入 Provider 名称、请求 query、服务端当前时间和 unknown，再进行校验。这里的 fetched_at 是服务端接收/获取结果时间，不是 Provider 源端时间。
- unknown 空结果不能证明证券不存在，继续回退；unknown 有匹配项先保留候选并继续回退。
- stale 空结果继续回退；stale 有匹配项保留为最后后备，优先覆盖条目更多、条数相同时采集时间更新的候选。
- valid 或 degraded 结果（包括 Tencent 的 degraded 非空结果）可直接返回。Tencent 默认优先因此正常有匹配候选时不会为了取得全量目录而强制调用 Eastmoney。
- 所有 Provider 耗尽后，优先返回 stale 匹配，再返回 unknown 匹配，再返回 unknown 空候选；若没有任何可保留快照则返回汇总错误。
- DataService 没有独立的查询结果缓存；Eastmoney Provider 内部缓存全量目录 6 小时，Tencent 每次实时请求。

## 5. HTTP、OpenAPI 与 WebUI 消费

- Handler 默认 limit=10；显式 limit 必须为整数 1..50；q 去空白后不能为空且最多 64 个 Unicode 字符。
- OpenAPI 路径、参数范围、DTO required 与 Handler/Go 模型一致；StockSearchItem/Snapshot 纳入 51 个模型反射契约登记。
- API Client `searchStocks` 对 query 去空白，使用 URLSearchParams 编码 q/limit，携带 Market API Key、AbortSignal 和 15 秒默认超时。
- API 请求测试已验证中文 query URL 编码、API Key、signal 透传；统一请求层另有超时、HTTP 错误矩阵和网络错误测试。Handler 测试覆盖成功响应、空 q 和 limit=51。
- 页面输入防抖 260ms，取消前一请求，generation 校验防止旧结果覆盖新输入；键盘活动项使用 aria-activedescendant，渲染用 textContent，不将 Provider 名称作为 HTML。
- 页面当前仅对 `quality_status === stale` 展示目录过期提示。对于 degraded/unknown 结果，页面均按普通结果显示“找到 N 条匹配结果”；对于 unknown 且 items 为空的回退耗尽响应，页面显示“没有匹配的股票，请检查名称或代码”。
- 因而服务端已经传出的质量状态和质量原因没有在 UI 完整消费；unknown 空结果尤其容易被用户误认为已确认不存在。
- 页面 view 组件有独立的 normalize/render/ARIA 单元测试；没有覆盖 app.js 中 loadStockSearch 对各质量状态的完整 DOM/交互集成测试。
- 页面显示条数使用当前 items.length，不使用 snapshot.total；当前 limit=10，因此这代表展示候选数，不是匹配总量。建议文案保持“展示 N 条候选”，不要改成“共 N 条”。

## 6. 已登记差异

### AUD-WEBUI-CTR-045（P2）：Stock Search WebUI 未消费 degraded/unknown 质量语义

- DataService 在回退耗尽时可返回 `quality_status=unknown` 且 `items=[]`，原因包含 `stock_search_fallback_exhausted_without_confirmed_match`；这代表当前 Provider 链无法确认匹配结果，不等价于已证明证券不存在。
- Tencent 正常返回非空候选时固定为 degraded，原因是 `provider_search_total_not_reported`；它的 total 只代表本次解析/匹配到的候选数。
- WebUI `loadStockSearch` 对空 items 不检查 quality_status，直接显示“没有匹配的股票”；对非空 items 仅传递 stale 标记，`renderStockSearchResults` 忽略 degraded/unknown 及 quality_reasons。
- 影响：目录覆盖不完整或搜索源无法确认时，用户无法区分“没有匹配项”和“搜索结果不完整/无法确认”；Tencent 的候选列表也被显示成与完整目录结果同等确定。
- 关闭条件：UI 至少区分 valid、degraded、stale、unknown；unknown 空结果应显示“当前搜索源无法确认，请稍后重试/检查代码”，而不是断言没有匹配证券；degraded 结果应说明候选覆盖或 Provider 限制，并可在可用时展示 quality_reasons。增加 app.js 搜索流程的质量状态 DOM 回归测试，覆盖四种状态及 unknown 空结果。

## 7. 本切片结论

Stock Search 已具备独立 Provider Port、Tencent 关键词搜索优先、Eastmoney 全量目录后备、统一 DTO 校验与质量驱动回退、HTTP/OpenAPI 和 WebUI 自动补全闭环。默认两家搜索 Provider 均启用；Tencent 优先，Eastmoney 只在 Tencent 失败/无匹配时承担回退。Provider/Service/API 基础测试较完整，主要已确认契约缺口集中在 WebUI 对质量状态的消费（CTR-045）。未发现 DTO required/nullable 不一致。WebUI 维度 2 继续 partial。
