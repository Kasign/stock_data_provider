# WebUI 维度 2：Limit Pool 涨停池、跌停池与炸板池

**审计代码基线：** `origin/integration` @ `b6dfc85ba214fe16268c8cba780641a8bfe15086`  
**维护分支：** `feat/design_doc`  
**接口：** `GET /api/v1/market/limit-pools`  
**DTO：** `LimitPoolItem`、`LimitPoolSnapshot`

## 1. 生产方、协议与装配

- Port：`provider.LimitPoolProvider.FetchLimitPool(ctx, poolType, tradeDate, limit)`，独立于 Quote/DailyBar 回退链。
- 实现：Eastmoney、Hithink。
- Bootstrap 根据 ProviderPriority 和 `ProviderSettings.LimitPools` 装配；默认配置 Eastmoney 启用，Hithink disabled。实现两家 Provider 不代表默认启用两路回退。
- DataService 负责日期/type/limit 规范化、缓存、DTO 校验、unknown 回退、截断降级和最终候选选择。
- Handler：`Server.limitPool`；路由 `GET /api/v1/market/limit-pools`；OpenAPI 有对应 operation。
- WebUI：`getMarketLimitPool` → `market-overview.js:load("pool")` → `renderPool`。页面提供 up/down/broken、日期及 10/20/30/50/100 条数选择。

## 2. DTO 与字段语义

### LimitPoolSnapshot

- `type`：up=涨停池、down=跌停池、broken=炸板池。
- `trade_date`：请求交易日；不被 Provider 的 qdate 覆盖。
- `provider_report_date`：Provider 源端报告日期；不可验证时省略。
- `total`：源端报告池总量；`provider_returned`：Provider 收到的原始行数；`excluded_non_a_share_count`：被排除的非普通 A 股记录数。
- `total_is_lower_bound`：源端总量只能视为下界时使用；本基线两个 Provider 当前均未显式置 true。
- `has_data=false` 必须同时 total=0、items 为空、quality=unknown；表示未知/不可用，不代表已确认零事件。
- `fetched_at` 是服务端采集时间，不是源端快照时间。来源/版本/采集时间为必填。

### LimitPoolItem

- `symbol` 为规范证券代码，`code` 为六位代码；`change_percent` 是百分点值，10 表示 10%，必须存在。
- `last_price` 为人民币元/股（可选）；触板时间为源端本地时间字符串，不包含日期和时区。
- `consecutive_boards` 为连板数；`broken_count`、`open_times` 为开板/炸板相关计数，具体仅在 Provider 有字段时输出。
- `turnover_ratio_percent` 为百分数值；`turnover`、`seal_money`、`max_seal_money` 为 Provider 金额字段，必须结合 `provider_amount_unit` 解释，不能默认人民币元。
- `provider_amount_unit=provider_defined` 明确表示金额量纲未能归一化为 CNY。
- `source`、`source_version`、`fetched_at`、`quality_status` 为条目级元数据；条目质量不能高于其父快照。

## 3. Provider 实际映射与数据质量

### Eastmoney

- 使用 `getTopicZTPool`、`getTopicDTPool`、`getTopicZBPool` 三个接口。
- 请求日期采用 YYYYMMDD；对未来中国市场日期拒绝请求。
- 按源端 `tc` 校验分页覆盖、页间总数和 qdate 一致性；对 tc>0 但 pool 为空的语义不完整响应重试一次，不把它当成空池。
- 支持跨页读取到调用方 limit；记录 Provider 原始返回数和源端总数，源端总数大于返回条数时明确诊断。
- 校验代码、名称、市场标识、涨跌幅、重复证券；解析 fbt/lbt 触板时间、lbc 连板数、zbc 炸板次数、hybk 行业。
- 当前实现不填充 last_price、turnover、seal_money、max_seal_money、turnover_ratio_percent 等金额/行情扩展字段；不应将这些字段理解为 Eastmoney 当前已提供。
- Eastmoney 不提供可验证的独立快照时间，快照和条目保持 degraded；源端 qdate 用于 ProviderReportDate。

### Hithink

- 三类池分别调用 `limit-up-pool`、`limit-down-pool`、`limit-break-pool`。
- 当前仅请求第一页，size 为调用方 limit，读取 pagination.total/page/size 并校验页码与计数；没有翻页拉取全量。
- 从响应 timestamp 派生 ProviderReportDate；缺失、无效或未来时间会影响质量。请求日期与源端报告日期不一致时 Service 拒绝该候选。
- 解析涨跌幅、价格、触板时间、连板/开板字段；涨停池可映射 seal_money/max_seal_money，炸板池可映射 turnover/open_times。
- 金额字段未有明确单位证明，条目写入 `provider_amount_unit=provider_defined` 并降级，不可跨 Provider 做 CNY 金额比较。
- 重复证券被排除并写入 validation Reporter 诊断；缺失触板时间、开板次数、成交额等会保留字段缺失和原因。
- 空列表不构成已确认零事件，返回 unknown；当前 Service 会继续尝试后续 Provider。

### 默认可用性

- Eastmoney 默认启用并排在 ProviderPriority 前列。
- Hithink 默认 disabled，且需要独立 API Key。
- 因此当前默认业务路径实际只有 Eastmoney 一路；不能把 Hithink 的实现能力计为默认可用的冗余保障。

## 4. Service 校验、回退与缓存

- type 只允许 up/down/broken；日期规范为上海时区当天或过去日期；limit 1..100，默认 100。
- 校验请求/响应 type 和日期一致、source/version/fetched_at 存在、采集时间不在未来、报告日期格式及与请求日期一致。
- has_data=false 只能是 unknown 且 total=0/items 为空；有数据时 total 不得小于 items 数，items 不超过 limit 且必须非空。
- 每个 item 校验 symbol/code 一致、证券身份唯一、必填 name/change_percent、source/version/fetched_at 与父快照一致、时间戳合理、数值有限、价格正数、金额/换手指标非负、计数字段非负。
- 父快照 valid 要求可验证 ProviderReportDate，且不能包含 degraded 条目。
- 空池或 unknown 质量不阻断后续 Provider；回退耗尽时保留条目数和字段覆盖较好的 unknown 候选，并附加耗尽原因；unknown 不缓存。
- 有数据的缓存 TTL 为 1 分钟；命中缓存仍会重新执行 DTO 校验和截断降级检查。
- 当 total 大于返回 items 时 Service 降级并增加 `provider_limit_pool_response_truncated`，防止 UI 将截断列表当完整池。

## 5. HTTP 与 WebUI 消费

- Handler 默认 type=up、limit=100；type 仅 up/down/broken；limit 1..100；显式 date 必须 YYYY-MM-DD，Service 再拒绝未来日期。
- OpenAPI 与路由路径、参数枚举和边界基本一致。
- WebUI 默认 limit=30；日期为空时不传 date，由服务端按中国时区当天处理。用户控件只允许 10/20/30/50/100。
- 页面展示证券名称/代码、涨跌幅、连板数、首次/末次触板时间、行业；支持跳转行情和深度研究。
- 页面显示池类型、交易日期、源端 total、请求上限、当前返回条数、来源、版本、Provider 报告日期、采集时间和质量原因。
- 空列表且 has_data=false 时明确提示“服务端未确认”，不将其显示为零只。
- 页面没有展示 seal_money/max_seal_money/turnover/turnover_ratio_percent/open_times/broken_count/provider_amount_unit 等扩展字段；当前默认 Eastmoney 本身也未填充大部分扩展字段。是否纳入 UI 需要产品确认，不自动登记为缺陷。
- `web/tests/api.test.mjs` 未直接调用 getMarketLimitPool；`web/tests/market-overview.test.mjs` 没有 Limit Pool 专项 DOM 测试。Provider 测试覆盖 Eastmoney 日期拒绝、部分覆盖、分页、Hithink 重复证券和字段诊断；Service 测试覆盖 unknown 回退、回退耗尽及截断原因。尚缺 API Client 参数/鉴权/AbortSignal 专项测试、Handler 参数边界专项测试和完整 renderPool DOM 回归。
- OpenAPI 的 51 模型反射契约测试将 LimitPoolItem/LimitPoolSnapshot 纳入字段集合和 required 检查；另有 JSON 序列化语义断言。

## 6. 已登记差异

### AUD-WEBUI-CTR-041（撤销）：LimitPoolItem 的 Go JSON、OpenAPI 与 JSON Golden 实际一致

- 第一轮审计将 `SealMoney`、`MaxSealMoney` 误读为非 `omitempty` 指针，基于该错误前提登记了三方语义冲突。
- 固定基线 `internal/marketdata/model/model.go` 实际定义为 `json:"seal_money,omitempty"` 和 `json:"max_seal_money,omitempty"`。
- `internal/marketdata/httpapi/openapi.yaml` 的 `LimitPoolItem.required` 不包含这两个字段，属性也没有声明 nullable。
- `TestDataModelJSONSerializationMatchesOpenAPIRequiredAndNullableSemantics` 明确将这两个字段列入 omitted。
- 三方语义一致：字段可选，nil 时省略；本项不是已证实契约缺陷，撤销 CTR-041，不作为 P1 门禁，不需要等待 CI 才能确认这一静态事实。
- 本轮没有运行固定基线 CI；这不影响 Go tag、Schema 和现有 Golden 对这两个字段的静态一致性结论。

### AUD-WEBUI-CTR-010 影响范围补充

- LimitPoolItem 的 `last_price`、`open_times`、`turnover_ratio_percent`、`turnover`、`consecutive_boards`、`broken_count` 是 Go omitempty 可选字段，OpenAPI 对应数字属性声明 nullable=true；nil 时 Go 省略而不是输出 null。
- LimitPoolSnapshot.provider_report_date 是 omitempty 字符串，OpenAPI 声明 nullable=true，同样属于省略/null 不一致。
- `seal_money`、`max_seal_money` 不纳入 CTR-010：它们是非 omitempty 指针，问题是 required/nullable 和 Golden 三方矛盾，单独由 CTR-041 跟踪。

### AUD-WEBUI-CTR-042（P2）：Limit Pool Provider 金额字段缺少可比较单位证据

- Hithink 将 seal_money/max_seal_money/turnover 原值写入 DTO，显式标记 provider_amount_unit=provider_defined，并添加 provider_amount_unit_not_explicitly_documented。
- Eastmoney 当前不提供这些金额扩展字段。
- OpenAPI 仅说明 provider_defined 不代表 CNY，没有给出各 Provider 的单位映射或换算规则。
- 影响：下游无法安全地对不同 Provider 的封板金额/成交额做比较、聚合或排序。
- 关闭条件：取得供应商字段单位的权威定义并建立逐 Provider 单位映射/测试；无法证实时继续保持 provider_defined，不得换算或宣称 CNY。

## 7. 本切片结论

Limit Pool 已有统一 Port、两种 Provider、Service 校验/回退/缓存、HTTP/OpenAPI 和市场概览消费。默认仅 Eastmoney 启用；两家 Provider 均不会把空响应解释为确认零事件；截断会降级。已确认的主要契约问题为 LimitPoolItem 三方 JSON/Schema/Golden 矛盾（CTR-041），以及 Hithink 金额单位未证实（CTR-042）；CTR-010 增加七个字段影响范围。WebUI 维度 2 继续 partial。
