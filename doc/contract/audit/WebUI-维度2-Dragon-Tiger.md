# WebUI 维度 2：Dragon Tiger 龙虎榜

**审计代码基线：** `origin/integration` @ `b6dfc85ba214fe16268c8cba780641a8bfe15086`  
**维护分支：** `feat/design_doc`  
**接口：** `GET /api/v1/market/dragon-tiger`  
**DTO：** `DragonTigerItem`、`DragonTigerSnapshot`

## 1. 生产方、协议与装配

- Port：`provider.DragonTigerProvider.FetchDragonTiger(ctx, tradeDate, page, pageSize)`。
- 实现：Eastmoney、Hithink；两者均通过同一接口注入 DataService。
- 默认 ProviderPriority 包含 Eastmoney 在 Hithink 之前；Eastmoney 默认 Enabled=true、DragonTiger=true；Hithink 默认 Enabled=false、DragonTiger=false。默认路径只有 Eastmoney，不构成双源冗余。
- Bootstrap 仅从已启用 Provider 中收集 DragonTigerProvider，并通过 `SetDragonTigerProviders` 注入 DataService；source status 也使用同一能力列表。
- Service 提供日期规范化、分页边界、响应校验、unknown 候选保留、Provider 回退和缓存。
- Handler 注册 `GET /api/v1/market/dragon-tiger`；API Key 认证由路由中间件处理。
- WebUI `getMarketDragonTiger` → `market-activity.js:loadDragon` → `renderDragon`，在市场活动页面提供日期筛选和分页加载。

## 2. DTO 与字段语义

### DragonTigerSnapshot

- `trade_date`：请求/结果交易日，采用上海市场日历日期。
- `provider_report_date`：源端报告日期；当前两个 Provider 均用报告交易日填充，不代表精确采集/发布时刻。
- `page`、`page_size`：请求分页；`total`、`pages`：Provider 声明的记录总数/页数。
- `source_returned`：本页 Provider 原始记录数（A 股过滤前）；`returned`：本页最终接受的 A 股条目数；`excluded_non_a_share_count`：本页明确排除的非 A 股记录数。三者应满足 source_returned = returned + excluded_non_a_share_count。
- `total_is_lower_bound`：源端可能有上限、总量只能视为下界时使用；当前两个 Provider 均未设置。
- `has_data=false` 不是已确认零上榜，必须保持 unknown；`has_more` 表示是否还有 Provider 页。
- `fetched_at` 是服务端采集时间，不是源端快照时间。

### DragonTigerItem

- `symbol`、`code`、`name`、`trade_date` 为证券身份和日期。
- `change_percent`、`deal_amount_ratio` 使用百分点；1.25 表示 1.25%。
- Eastmoney 的 `close_price_cny`、`*_amount_cny` 字段按人民币元/股或人民币元映射；净买入允许负数，买入/卖出/成交额不得为负。
- Hithink 金额保存在 `provider_buy_value`、`provider_sell_value`、`provider_net_value`、`provider_net_rate`、`provider_org_net_value`、`provider_hot_money_net_value` 等原始 Provider 字段，`provider_amount_unit=provider_defined`；不伪装成 CNY 字段。
- `reason` / `explanation` 是上榜原因和说明；`hot_rank`、`range_days`、`concepts` 为 Hithink 扩展数据。
- 条目和快照都有来源、适配器版本、服务端采集时间及质量状态。

## 3. Provider 实际映射

### Eastmoney

- 调用 Data Center `RPT_DAILYBILLBOARD_DETAILSNEW`，按 TRADE_DATE、pageNumber、pageSize 查询；按净买入金额降序。
- 校验请求日期不晚于上海时区今日，page 1..1000，page_size 1..500；校验返回 count、pages、data 数量、行交易日和证券身份。
- 可转债（SECURITY_TYPE_CODE=060）及 B 股记录被排除并计数；非 A 股排除不会污染当前页 A 股条目。
- 将 CLOSE_PRICE、CHANGE_RATE、BILLBOARD_NET_AMT、BILLBOARD_BUY_AMT、BILLBOARD_SELL_AMT、BILLBOARD_DEAL_AMT、DEAL_AMOUNT_RATIO 映射为明确 CNY/百分点字段；保留 EXPLAIN、EXPLANATION、SECURITY_TYPE_CODE。
- Provider 不提供可验证的精确快照时间，所有有数据的快照/条目保持 degraded，并生成 provider_time 诊断。
- code=9201 且“返回数据为空”以及 count=0 的空报告均保持 unknown，不解释为零上榜。
- Eastmoney 的分页记录数是本页 data 长度，与 Service 的 source_returned 定义一致。

### Hithink

- 调用 `/api/a-share/special-data/dragon-tiger-list`，board_type=all；请求指定日期时将 date 传给 Provider。
- 要求响应包含有效 trade_date；显式请求日期必须与响应日期一致；响应日期不能晚于上海时区今日。
- Provider 当前响应结构包含整个 `stock_items` 数组，代码按 page/page_size 在内存中切片；pages 根据 stock_items 数量计算。
- Hithink 金额字段未获得公开字段表的明确货币单位证明，原值独立保留并标记 provider_defined；因此即使时间戳存在，数据质量仍保持 degraded。
- Hithink 额外返回热钱/机构净额、热度排名、区间天数和概念；共享 DTO 只映射其已明确表达的字段。
- 空列表保持 unknown。显式日期仅支持最近一年，早于该范围会报错；Eastmoney 没有同样的服务端一年限制。
- 当前基线没有 Hithink 龙虎榜 Provider 专项测试文件；其分页映射尚未被独立的真实响应契约测试覆盖。

## 4. Service 校验、回退与缓存

- 日期为空时规范为上海时区当天；显式日期必须是 YYYY-MM-DD 且不晚于今日。
- page 1..1000、page_size 1..500。
- 校验请求身份、来源/版本/采集时间、非未来采集时间、总数/页数、当前页原始数、接受数与排除数关系。
- total>0 时要求 pages 与 total/page_size 对应，source_returned 必须精确等于当前页应有原始记录数。
- 有数据时要求 report_date 与请求日期一致、has_more 与 pages 一致、证券代码规范化一致、条目来源版本/日期/采集时间有效、数值有限；买入/卖出/成交额非负，净买入可为负。
- 快照不得在包含 degraded 条目时标 valid。
- 空报表、当前页全是非 A 股或 unknown 质量候选会继续尝试后备 Provider；回退耗尽后保留较有价值的 unknown 候选并附原因。
- 有数据缓存 TTL 5 分钟；unknown/空结果 TTL 30 秒；仅回退耗尽或单 Provider 配置时才复用 unknown 缓存。
- 当前无龙虎榜 MariaDB Repository/历史表；数据通过 Provider 实时查询并由缓存短暂复用。页面分页不是历史数据持久化能力。

## 5. HTTP、OpenAPI 与 WebUI

- Handler 默认 page=1、page_size=100；page 1..1000、page_size 1..500；显式 date 格式错误返回 400，未来日期由 Service 拒绝。
- OpenAPI 路径、查询参数和边界与 Handler 一致；DTO 进入 51 个模型反射字段/required 契约测试。
- WebUI Client 默认 page_size=50；传递 date/page/page_size 和 AbortSignal，携带 API Key。
- 页面显示交易日、页码、本页返回数、源端本页记录数、排除非 A 股数、来源、报告日期、版本、采集时间、质量和质量原因。
- 表格展示股票身份、涨跌幅、净买入、成交额、上榜原因和质量；金额格式明确标“元”，只读取 *_cny 字段。Hithink 未证实单位的原始金额不显示为人民币值，这一保守行为正确。
- 支持分页、取消旧请求、日期筛选失效提示，以及跳转行情/深度研究。
- API Client `getMarketDragonTiger` 未在 `web/tests/api.test.mjs` 中找到直接请求测试；`web/tests/market-activity.test.mjs` 仅覆盖日历范围和来源时间证据，没有龙虎榜渲染/分页 DOM 回归。
- Provider 有 Eastmoney 日期、分页、单位、空报告、日期错配、可转债排除等测试；Service 有日期、金额符号、条目身份/质量、unknown 回退与空页测试。未发现 Hithink 龙虎榜 Provider 专项测试、Handler 参数边界专用测试。
- OpenAPI JSON 序列化语义测试覆盖 Quote、DailyBar、Bundle、HotRanking、MarketSentiment、LimitPool、SourceStatus、StockAnnouncementSnapshot；未覆盖 DragonTigerItem/Snapshot 的 omitempty 实际序列化。

## 6. 已登记差异

### AUD-WEBUI-CTR-043（P1）：Hithink 龙虎榜分页快照的 source_returned 与 Service 分页契约不一致

- Hithink `FetchDragonTiger` 将 `SourceReturned` 设置为整份 `data.StockItems` 的长度；随后仅将 `data.StockItems[startIndex:endIndex]` 放入当前页 Items。
- Service 的 `validateDragonTigerSnapshot` 明确要求 `SourceReturned == min(page_size, total-(page-1)*page_size)`，即当前页原始记录数。
- 因此当 Hithink 返回总记录数大于 page_size 时，任何页的 source_returned 都会大于当前页预期数，Service 会拒绝该候选。若 Eastmoney 不可用/unknown，Hithink 无法作为有效回退源。
- 同时 Hithink 在检测到当前页重复 symbol 时会跳过该行，但没有增加 `ExcludedNonAShareCount`（该字段语义也不适合计重复项），从而使 `source_returned = returned + excluded_non_a_share_count` 约束再次不成立。
- 关闭条件：明确 SourceReturned 是本页还是整份响应。按当前公共契约，应设置为当前页原始切片长度；重复记录应作为 Provider 数据矛盾拒绝该页，或扩展明确的 dropped/duplicate 计数契约，不能静默丢弃后让计数不守恒。增加 page 1/中间页/末页、大于 page_size、重复 symbol 的 Hithink httptest 和 Service 回退集成测试。

### AUD-WEBUI-CTR-044（P2）：Hithink 龙虎榜显式日期范围限制未体现在公共 API 契约

- DataService 和 Handler 接受任何不晚于今日的有效日期；OpenAPI 也只声明 date 为 YYYY-MM-DD，没有历史范围上限。
- Hithink Provider 对显式日期限制为最近一年；超过范围返回 Provider 错误。
- 当前默认 Hithink disabled，故默认路径不受影响；若启用作回退源，较早日期时会被静默跳过并可能只剩 Eastmoney。
- 关闭条件：将 Provider 支持范围作为 capability/诊断元数据公开，或至少在 Provider 能力文档和探测结果中说明；不建议把某个 Provider 的一年限制收窄成公共 API 的全局日期限制。

### AUD-WEBUI-CTR-010 影响范围补充

- DragonTigerItem 的 16 个 omitempty 指针字段：`change_percent`、`close_price_cny`、`net_buy_amount_cny`、`buy_amount_cny`、`sell_amount_cny`、`deal_amount_cny`、`deal_amount_ratio`、`provider_buy_value`、`provider_sell_value`、`provider_deal_value`、`provider_net_value`、`provider_net_rate`、`provider_org_net_value`、`provider_hot_money_net_value`、`hot_rank`、`range_days` 在 OpenAPI 中声明 nullable=true，但 Go nil 时因 omitempty 直接省略。
- DragonTigerItem.provider_amount_unit 与 DragonTigerSnapshot.provider_report_date 是 omitempty 字符串，OpenAPI 也声明 nullable=true，同样应明确选择“省略”还是“显式 null”。
- 当前反射契约测试只验证字段集合和 required；不会检测 omitempty 指针/字符串的 nullable 声明是否与 Go JSON 输出一致。DragonTiger DTO 也没有专门 JSON serialization Golden。

## 7. 本切片结论

龙虎榜已具备统一 Provider Port、Eastmoney/Hithink 实现、服务端质量校验/回退/缓存、HTTP/OpenAPI 和 WebUI 页面消费。默认只有 Eastmoney 启用。Eastmoney 分页与计数契约基本闭合，空报表和非 A 股空页保持 unknown。主要问题是 Hithink 当前分页适配器与 Service 的本页计数契约冲突（CTR-043），导致大于单页大小时回退不可用；另有 Provider 历史支持范围未公开（CTR-044）。WebUI 已具备基本消费，但 API Client、Hithink Provider 和页面 DOM 测试覆盖不足。WebUI 维度 2 继续 partial。
