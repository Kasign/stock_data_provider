# WebUI 维度 2：Hot Rankings 人气榜与飙升榜

**审计代码基线：** `origin/integration` @ `b6dfc85ba214fe16268c8cba780641a8bfe15086`  
**维护分支：** `feat/design_doc`  
**对应入口：** `GET /api/v1/market/hot-stocks`

## 1. 生产方与调用链

- DTO：`internal/marketdata/model/model.go` 中的 `HotRankingItem`、`HotRankingSnapshot`。
- Provider Port：`provider.HotRankingProvider.FetchHotStocks`。
- 实现：`internal/marketdata/provider/eastmoney.go`、`hithink.go`、`xueqiu.go`。
- 配置装配：`internal/marketdata/config/config.go`、`internal/marketdata/bootstrap/module.go`。
- Service：`internal/marketdata/service/hot_rankings.go`。
- Handler：`internal/marketdata/httpapi/hot_rankings.go`。
- Schema：`internal/marketdata/httpapi/openapi.yaml` 的 `HotRankingItem`、`HotRankingSnapshot`。
- WebUI：`web/src/api.js:getMarketHotStocks`、`web/src/market-overview.js:renderHotStocks/bindMarketOverview`。

当前存在统一 `HotRankingProvider` 协议，Eastmoney、同花顺、雪球均实现该 Port；Service 统一验证、回退和缓存。无需再为榜单 Provider 建立第二套协议层。

## 2. 类型、字段与单位语义

### HotRankingSnapshot

- `type`：`popular`、`surge`、`followers`。
- `total`：本次响应中归一化保留的条目数，不是全市场证券数量，也不是未经限制的完整榜单总数。
- `provider_returned`：Provider 原始返回行数，发生调用方 limit 截断或过滤前统计。
- `excluded_rows`：归一化明确排除的行数，例如重复证券。
- `total_is_lower_bound`：源端未提供权威总数时为 true。
- `provider_time`：Provider 数据准备时间；不能当成精确行情成交时间。
- `fetched_at`：服务端采集时间。
- 成功榜单仍可能是 delayed ranking，不能作为实时交易信号。Eastmoney 没有可信独立榜单快照时间；同花顺和雪球可解析响应级时间，但延迟榜单质量仍保持 degraded。

### HotRankingItem

- `rank`：源端排名位置；Service 要求从 1 开始连续递增。
- `symbol`、`code`：标准 A 股证券代码及六位代码，Service 校验二者一致。
- `latest_price`、`change`：人民币元/股。
- `change_percent`：百分点，10 表示 10%，不是 0.1 比率。
- `rank_change`：源端排名变化，符号沿用上游定义；不同 Provider 的含义需要谨慎比较。
- `heat`：Provider 原生热度值，量纲由 Provider 定义，不能跨源直接比较。
- `rank_trend`：Provider 原生趋势标签。
- 可选数值缺失时保持 nil，不补零。

## 3. Provider 实际能力与默认配置

- Eastmoney 实现 popular/surge，榜单行再通过指数行情接口批量补充价格、涨跌额和涨跌幅；榜单本身没有可信快照时间，成功结果保持 degraded。
- 同花顺实现 popular/surge；读取响应级 timestamp，但榜单明确按延迟数据处理，保持 degraded。缺失名称会记录字段诊断；重复证券会被排除并降级。
- 雪球实现 popular/surge/followers；surge 是全市场涨跌幅排序代理，不等价于 Eastmoney 飙升榜；单次请求最多 50 条，调用方请求更大 limit 时会截断并诊断。
- 默认 Provider Priority 中 Eastmoney 在前，Hithink 随后，Xueqiu 在末尾；默认配置只启用 Eastmoney 的 HotRankings，Hithink 和 Xueqiu 均 disabled。实现多个 Provider 不等于默认启用多路回退。
- OpenAPI 对 fallback 优先级的描述与默认顺序基本一致；是否实际回退由 providers 配置决定。

## 4. Service 校验、回退与缓存

- 请求类型只允许 popular/surge/followers；limit 为 1–100，默认 10。
- Service 校验 snapshot 类型、total 与 items 数量、source/source_version/fetched_at、ProviderTime、质量状态、证券代码、排名唯一性/顺序/连续性、条目来源元数据及有限数值。
- 空榜必须是 unknown；unknown 或空结果不会阻断后续 Provider。回退耗尽后保留条目数/字段完整度较好的 unknown 候选，并附加耗尽原因。
- 缓存：valid 90 秒、degraded 30 秒、unknown 10 秒；多 Provider 场景下未耗尽回退的 unknown 候选不能提前阻断刷新。
- 未发现已证实的价格/涨跌幅公式或金额单位换算错误；跨 Provider 的热度量纲、rank_change 语义不应被视为等价。

## 5. WebUI 消费与测试

- 市场概览页面请求人气榜/飙升榜，默认 limit 20；支持调整返回数量。
- 页面显示排名、名称/代码、最新价、涨跌额、涨跌幅和排名变化；不显示 Provider 原生 heat/rank_trend。页面显示质量、来源版本、Provider 时间与采集时间，并明确榜单不是全市场证券完整清单。
- API Client 当前只接受 popular/surge；传入 followers 会被静默归一化为 popular。服务端与 OpenAPI 支持 followers，雪球 Provider 也支持 followers；当前 WebUI 控件只提供人气榜/飙升榜，是否需要 WebUI 暴露 followers 属于产品范围决策，不在本轮直接登记缺陷。
- `web/tests/market-sector-ranking.test.mjs` 通过源码断言覆盖榜单数量选择及部分页面结构；`web/tests/market-overview.test.mjs` 未直接测试 Hot Ranking 请求或 DOM 渲染。`web/tests/api.test.mjs` 未导入并直接调用 `getMarketHotStocks`。当前有 Provider 契约/重复行测试和 Service 回退测试；仍缺少该 API Client 的 URL/参数/鉴权/取消请求测试，以及榜单页面真实响应 DOM 回归测试。
- 当前未发现页面把榜单条目数误显示为全市场证券总数；UI 明确显示请求上限和实际返回数。

## 6. 已登记差异

### AUD-WEBUI-CTR-037（P2）：HotRankingItem.change_percent 的 OpenAPI 描述引用了错误数据集

- 位置：`internal/marketdata/httpapi/openapi.yaml` 的 `components.schemas.HotRankingItem.properties.change_percent.description`。
- 当前描述末尾称“successful LimitPoolSnapshot with has_data=true must return a non-null value”，这是 Limit Pool 的约束，不适用于 Hot Ranking。
- Go DTO 的 `HotRankingItem.ChangePercent` 是 `*float64` 且 `omitempty`；Hot Ranking Service 允许该字段缺失，不要求非空。
- 影响：OpenAPI 消费者可能错误地把 Hot Ranking 的 change_percent 当作必填有效值，或把 Limit Pool 约束套用到榜单。
- 关闭条件：改为 Hot Ranking 自身语义，明确百分点单位、缺失值允许省略；增加 Schema 描述/Golden 或契约断言，防止跨 Schema 复制错误。

### AUD-WEBUI-CTR-010 影响范围补充

- `HotRankingItem.latest_price`、`change`、`change_percent`、`rank_change`、`heat` 与 `HotRankingSnapshot.provider_time` 均为 Go omitempty 指针字段，nil 时 JSON 省略；OpenAPI 对应字段却声明 `nullable: true`。
- 归入既有 omitempty/nullable 一致性差异，不重复创建独立编号。

## 7. 本切片结论

Hot Ranking 已具备统一 Provider Port、三种 Provider 实现、Service 校验/回退/缓存、OpenAPI 和页面消费。当前已证实的契约问题为 CTR-037；CTR-010 增加六个字段。followers 是否进入 WebUI 属产品范围待决，API Client 与页面的请求测试覆盖不足但尚不能据此推断运行时错误。WebUI 维度 2 继续 partial。
