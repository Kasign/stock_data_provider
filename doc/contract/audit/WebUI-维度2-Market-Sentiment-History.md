# WebUI 维度 2：Market Sentiment History 市场情绪历史

**审计代码基线：** `origin/integration` @ `b6dfc85ba214fe16268c8cba780641a8bfe15086`  
**维护分支：** `feat/design_doc`  
**接口：** `GET /api/v1/market/sentiment/history`  
**对应 DTO：** `MarketSentimentHistory`、`MarketSentimentSnapshot`

## 1. 数据链路与职责

- 当前情绪快照由 `DataService.MarketSentiment` 构建：基础数据来自 MarketBreadth，再按同一 `trade_date` 读取 up/down/broken 三类 LimitPool。
- 任一辅助池不可用、无数据或报告日期不匹配时，对应计数保持 nil，并添加质量原因；不会补零。
- 当前快照仅允许当前中国市场日期采集；历史日期必须走只读 History API。
- 配置 Store 后，当前快照通过 `Store.SaveMarketSentiment` 追加写入 `market_data_sentiment_snapshots`；未配置存储时，历史查询返回 503。
- History API 只读取持久化记录，不从当前行情或其他数据集合成历史日期；没有记录时返回空 items。
- 存储表在 SQL migration 与 `Store.Migrate` 中均有定义：主键自增 id，按 market/trade_date/id 建索引，并另有 fetched_at 索引。

## 2. DTO 与 OpenAPI

### MarketSentimentSnapshot

- 必填：market、trade_date、total_securities、covered_securities、advancing_count、declining_count、unchanged_count、unclassified_count、limit_up_count、limit_down_count、broken_board_count、source、fetched_at、quality_status。
- 三个 LimitPool 计数字段为非 omitempty 指针，辅助数据不可用时 JSON 显式输出 null；OpenAPI required 且 nullable，语义一致。
- provider_report_date、source_version、三个 lower-bound 标志、pool_sources、pool_source_versions、quality_reasons 在 Go 中 omitempty，OpenAPI 不将其列为 required；没有发现本 DTO 的 omitempty/nullable 错配。
- 所有计数均为证券数量；limit_pool total 若为下界，使用对应 *_is_lower_bound 标志表达，UI 显示“≥N”。未知值显示“—”。
- `source` 可能由 breadth 来源及成功附加的各 LimitPool 来源组成；`pool_sources` 和 `pool_source_versions` 记录每类池的来源和版本。source_version 是 omitempty，且当前快照构造没有为组合数据生成统一 source_version，因此不可假设一定存在。

### MarketSentimentHistory

- 必填字段：market、start_date、end_date、items、total。
- items 是 MarketSentimentSnapshot 数组；Repository 初始化为空 slice，因此无记录时 JSON 返回空数组而非 null。
- start_date/end_date 是请求闭区间；数据按 trade_date 降序，每个交易日最多返回一条快照。
- total 当前由 Service 设置为 `len(items)`，即本次返回条数，不是日期范围内所有可用记录的总数。OpenAPI 仅声明 integer，没有说明该语义，也没有 has_more/cursor。
- OpenAPI 51 模型反射测试已将两个 DTO 纳入字段集合和 required 对照；另有专项断言要求 MarketSentimentSnapshot 的三个 LimitPool 计数字段为 required。未发现已证实的字段集合或 required 不一致。

## 3. 日期、范围与限制

- Handler 要求 start/end 均存在，严格 YYYY-MM-DD，end 不早于 start，闭区间最多 366 个自然日。
- limit 默认 366，范围 1..500；非法值返回 400。
- Service 重复校验日期格式、顺序及范围，并限制 limit 1..500。
- WebUI API Client 默认 limit=120，调用时显式传 120；Client 对传入 limit 做 1..500 截断归一化。
- WebUI 日期范围校验不设 366 天上限，故用户可提交超过服务端上限的范围，最终收到服务端 400；UI 没有在请求前提示该接口 366 日限制。
- 当前页面显示“已读取 N/total 条”，但由于 total 就是 len(items)，该比例恒为 N/N，不能表示日期范围内历史记录总量或是否被 limit 截断。

## 4. Repository 最新记录选择与持久化

- SaveMarketSentiment 对每次快照执行 INSERT，同一交易日允许多次保存，不做 upsert。
- ListMarketSentiment 对每个 trade_date 选择 `MAX(id)`，即最后插入的记录；代码注释称“最近一次采集”。
- id 是插入顺序，不是 fetched_at。并发请求可能先取得较早 fetched_at、后完成 INSERT；因此最大 id 不保证 fetched_at 最大。该差异在并发或写入延迟时可能返回非最新采集快照。
- 表中有 fetched_at 索引，但查询没有使用它来选最新记录。
- 本轮未发现专门覆盖 SaveMarketSentiment/ListMarketSentiment 的 Repository 单元或 MariaDB 集成测试；现有 mysql_integration_test 聚焦其他 PIT 数据集。
- 没有验证过历史记录保留期限/清理策略；当前迁移未见 TTL 或清理机制。此项属于运维策略待确认，不直接登记契约缺陷。

## 5. WebUI 消费与测试

- API Client：`getMarketSentimentHistory` 校验 start/end 日期及顺序，生成 start/end/limit 查询参数，并传递 API Key 和 AbortSignal。
- 页面：`market-overview.js` 的 `loadSentimentHistory` 读取用户选择的日期范围，固定请求 limit=120；对过期/取消请求使用 generation 与 AbortController 防止旧响应覆盖新状态。
- 页面展示交易日、上涨/下跌/平盘数量、涨停/跌停/炸板数量、质量状态、覆盖、源端日期、来源版本、采集时间和质量原因；缺失计数不显示为 0。
- 页面提供按快照交易日跳转 LimitPool/DragonTiger 的联动。
- `web/tests/market-overview.test.mjs` 覆盖日期校验、取消/过期响应判定、下界计数显示、来源时间证据格式；没有直接测试 getMarketSentimentHistory 的 URL 参数、API Key、AbortSignal，也没有测试历史表格真实 DOM 渲染。
- `web/tests/api.test.mjs` 未导入/调用 getMarketSentimentHistory。
- `internal/marketdata/httpapi/openapi_contract_test.go` 包含两个模型的字段/required 反射校验，但本轮没有发现 History Handler 的专用请求/错误响应测试；`server_test.go` 未覆盖 sentiment/history 路由。
- 当前 `getMarketSentiment` Client 没有 WebUI 页面消费者，而它对应的实时采集接口是历史记录的写入入口。已确认 WebUI 历史页只读历史，不触发实时采集。是否由外部定时任务/其他调用方负责每日采集，需由 Server/部署侧确认；不能据此直接判定数据缺陷。

## 6. 已登记差异

### AUD-WEBUI-CTR-038（P2）：历史接口 total 不能表达范围总量或截断状态

- 证据：`DataService.MarketSentimentHistory` 设置 `Total: len(items)`；Repository SQL 在 LIMIT 后返回结果；OpenAPI 未定义 total 是返回条数还是范围总数；WebUI 显示 `items.length/total`。
- 影响：当日期范围内记录数超过 limit 时，客户端无法判断是否还有未返回记录；当前页面 N/N 也会造成“已完整读取”的误解。
- 关闭条件（二选一并统一契约）：
  1. 将字段明确命名/描述为 returned_count，并增加 has_more/next_cursor 或 offset/page，支持可靠续读；或
  2. total 改为日期范围内符合条件的总记录数，并增加 has_more 或分页机制，确保客户端能判断截断。
- 测试需覆盖 0 条、少于 limit、等于 limit、超过 limit，以及重复交易日只取一条的行为。

### AUD-WEBUI-CTR-039（P2）：按自增 id 选择最新记录不等于按采集时间选择最新记录

- 证据：Repository 使用 `MAX(id)` 选每个交易日的快照；表同时保存 `fetched_at`，但未用于最新记录排序。
- 影响：并发采集或写入延迟时，后插入但采集时间更早的记录可能覆盖较新的观测。
- 关闭条件：明确“最新”的权威顺序；若语义为最新采集时间，则按 fetched_at DESC、id DESC 选取，并增加并发/乱序写入测试；若语义为最后写入，则调整注释、OpenAPI 和页面文案，不得称为最近采集。

### AUD-WEBUI-CTR-040（P2）：历史页面日期校验未执行服务端 366 自然日限制

- 证据：页面调用 `validateMarketDateRange(start,end)` 不传 maxCalendarDays；Handler 强制闭区间不超过 366 天。
- 影响：用户可在页面提交服务端必然拒绝的日期范围，体验为请求后 400，而不是本地即时校验。
- 关闭条件：WebUI 与 Handler 使用同一闭区间 366 自然日限制，并增加 366/367 日边界测试；OpenAPI 和页面提示保持一致。

## 7. 本切片结论

历史情绪 DTO 与 OpenAPI 的字段集合/required 基本一致，缺失辅助池计数保留 null，WebUI 也保留 unknown/下界语义。已确认问题集中在历史记录分页/total 语义、Repository 最新记录排序，以及 UI 未提前校验服务端日期范围。实时采集由当前快照接口触发，而历史页面只读；每日采集责任仍需 Server/部署侧补证。WebUI 维度 2 继续 partial。
