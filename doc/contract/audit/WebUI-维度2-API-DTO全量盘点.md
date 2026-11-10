# WebUI 维度 2：API Client、服务端路由与页面消费者全量盘点

**审计代码基线：** `origin/integration` @ `b6dfc85ba214fe16268c8cba780641a8bfe15086`  
**文档维护分支：** `feat/design_doc`  
**盘点范围：** `web/src/api.js`、WebUI 主要页面模块、Market Data HTTP 路由/OpenAPI、Analysis Task OpenAPI、`web/tests/api.test.mjs`。  
**本记录性质：** 接口入口和消费关系盘点，不代替各 DTO 数据集的字段语义审计。

## 1. 盘点结果

- `web/src/api.js` 共导出 44 个函数，其中 3 个为本地设置读写（`loadSettings`、`saveSettings`、`clearSettings`），其余 41 个为远端请求/健康检查入口。
- 41 个远端入口中，40 个在 WebUI 主要页面模块中找到精确标识符引用；`getMarketSentiment` 未发现页面模块消费者，是当前唯一已确认的孤儿 Client 函数。它是否为预留接口需确认产品范围，不自动登记为 P2。
- Market Data HTTP Adapter 在 `internal/marketdata/httpapi/server.go` 注册业务路由；OpenAPI 位于 `internal/marketdata/httpapi/openapi.yaml`。Analysis Task API 的路由由 `internal/server/bootstrap/router.go` 注册，任务 OpenAPI 位于 `doc/api/contracts/分析任务.openapi.yaml`。
- 已完成的数据集切片仍按既有第 1–36 节计算，不重复审计。盘点发现的未审或测试缺口进入后续清单，不把“有 Client 函数”误判为“页面完整消费”。

## 2. API Client 到页面消费者映射

| API Client 函数 | 页面/模块消费者 | 当前盘点结论 |
|---|---|---|
| marketLiveness / marketHealth | data-center.js / app.js | 健康检查入口；marketLiveness 有消费者 |
| analysisLiveness / analysisHealth | data-center.js / app.js | 健康检查入口；analysisLiveness 有消费者 |
| getDataSources | app.js / data-center.js | 已消费 |
| getMarketQuotes | app.js | 已消费 |
| getDataSourceDiagnostics | data-center.js | 已消费 |
| runDataSourceProbe | data-center.js | 已消费 |
| getDataSourceProbeHistory | data-center.js | 已消费 |
| getMarketValuations | stock-research.js | 已消费；第 27 节已审 |
| getMarketHotStocks | market-overview.js | 已消费；待对照第 1–36 节核对对应数据集覆盖 |
| getMarketIndices | market-overview.js | 已消费；第 33 节已审 |
| getMarketStocks | market-stocks.js | 已消费；已有全市场分页切片 |
| getMarketSentiment | 未发现消费者 | 孤儿 Client；需确认是否预留或应删除/接入，不自动登记 P2 |\n| getMarketSentimentHistory | market-overview.js | 已消费；历史 DTO 需确认是否独立覆盖 |
| getMarketCapitalFlow | market-overview.js | 已消费；第 30 节已审 |
| getMarketSession | existing-api-tools.js | 有工具模块消费者，但尚未证明进入主要页面交互；第 31 节已记录未展示缺口 |
| getIndexCatalog / getIndexConstituents / getIndexDailyBars | existing-api-tools.js | 有工具模块消费者；第 34–36 节已审，页面闭环差异已登记 |
| getStockPriceLimitRule / getStockTradingStatus | existing-api-tools.js | 有工具模块消费者；第 23 节已审相关 DTO |
| getMarketBreadth / getMarketSectors / getMarketLimitPool | market-overview.js | 已消费；MarketBreadth 第 32 节已审，板块第 24 节已审；LimitPool 对应切片需在完整性复核中定位 |
| getMarketDragonTiger / getTradingCalendar | market-activity.js | 已消费；交易日历第 31 节已审，龙虎榜对应切片需在完整性复核中定位 |
| getStockProfile / getStockBoards / getStockFundamentals / getStockAnnouncements / getChipDistribution / getCapitalFlow | stock-research.js | 已消费；分别对应既有证券资料、板块、财务、公告、筹码和个股资金流切片 |
| searchStocks | app.js | 已消费；证券搜索 DTO 需确认是否纳入第 23 节或另有独立切片 |
| getMarketBundle / getQuote / getDailyBars | app.js | 已消费；属于个股行情/日线基础数据，已有既有审计切片 |
| createAnalysisTask / getAnalysisRun / getAnalysisTask / getAnalysisTaskEvents | app.js | 已消费；Analysis Task 已审，Analysis Run 结果 DTO/Schema 需单独确认 |
| getChipDistribution / getCapitalFlow | stock-research.js | 已消费；第 28、29 节已审 |

注：表格按主要消费者文件归组；同一函数可能由应用层进一步传递给状态/视图模块。工具模块 `existing-api-tools.js` 中存在函数引用，不等于该能力已进入默认页面工作流。

## 3. 服务端路由与 OpenAPI 入口

### Market Data

`internal/marketdata/httpapi/server.go` 的路由注册覆盖：
- 健康：`/health/live`、`/health/ready`
- 行情：单股 Quote、批量 Quotes、Market Stocks、Daily Bars、Bundle
- Provider：Data Sources、Diagnostics、Probe、Probe History
- 市场：Sectors、Indices、Valuations、Index Catalog、Index Constituents、Index Daily Bars、Market Capital Flow、Breadth、Sentiment、Sentiment History、Hot Stocks、Limit Pools、Dragon Tiger、Trading Calendar、Session
- 个股：Search、Trading Status、Price Limit Rule、Profile、Boards、Announcements、Fundamentals、Chip Distribution、Capital Flow

上述业务路由均能在 `internal/marketdata/httpapi/openapi.yaml` 找到对应 path。Market Data 的 OpenAPI 共有 35 个 path，包含 2 个健康路由。

### Server / Analysis

- `internal/server/bootstrap/router.go` 注册 `/healthz`、`/readyz`、`/diagnostics`，以及 Analysis Runs、Analysis Tasks、Task Events。
- `doc/api/contracts/分析任务.openapi.yaml` 只描述 Analysis Tasks 和 Task Events 三个 path；没有描述 WebUI Client 实际调用的 `GET /api/v1/analysis/runs/{run_id}`，也没有描述 Runs Events 路由。
- WebUI 目前没有 `analysisLiveness`、`/diagnostics` 的专用契约测试证据；健康检查的实际路由/访问策略应在维度 3 与 Server 维度交叉核验。

## 4. API Client 请求测试盘点

在 `web/tests/api.test.mjs` 的 import/直接调用范围内，已看到请求测试覆盖：
- Analysis Task 创建、状态/事件读取、Analysis Run 读取及幂等键/鉴权隔离；
- Quote、Daily Bars、Bundle、批量 Quotes；
- Provider Sources、Diagnostics、Probe、Probe History；
- Market Stocks、Sectors、Market Capital Flow、Valuations；
- 个股 Profile、Boards、Fundamentals、Announcements；
- Chip Distribution、个股 Capital Flow；
- Index Catalog、Constituents、Index Daily Bars；
- Market Session、Trading Calendar、Price Limit Rule、Trading Status；
- 搜索与健康检查的部分路径。

本文件中没有看到以下函数被 `api.test.mjs` 导入并直接调用（不等于全仓库没有任何浏览器级覆盖）：
- `marketLiveness`、`analysisLiveness`
- `getMarketHotStocks`、`getMarketIndices`
- `getMarketSentiment`、`getMarketSentimentHistory`
- `getMarketBreadth`、`getMarketLimitPool`、`getMarketDragonTiger)

这些入口的直接 API Client 请求测试缺口已作为对应 CTR 或补证条件记录；没有新代码/测试证据时不再重复扫描。健康检查路由和访问策略属于维度 3 / Server 交叉核验，不扩展本维度 2。

## 5. 待核数据集与收口分类

| 数据集/入口 | 当前审计状态 | 后续动作 |
|---|---|---|
| Hot Rankings | 字段/Provider/页面消费切片已完成；CTR-037及CTR-010影响已登记 | 不重做初审；仅在责任方提交修复或新证据后增量验收 |
| Market Sentiment History | 历史DTO、Repository、Handler、页面及CTR-038～040冻结影响Review已完成 | CTR-038/039待语义裁决；CTR-040待WebUI实现对齐；不重复初审 |
| Limit Pool | DTO、Provider、Service、配置、Handler、OpenAPI、页面及CTR-042 Review已完成 | CTR-042单位证据/不可比较边界待裁决；不重复初审 |
| Dragon Tiger | DTO、Provider、Service、分页、金额单位、页面及CTR-043/044 Review已完成 | CTR-043保留P1，等待Data责任方修复/明确延期；CTR-044能力边界待裁决 |
| Stock Search | DTO、Provider、Service、质量状态及页面消费已审；CTR-045 Review已完成 | CTR-045等待WebUI质量状态消费和DOM回归；不重复初审 |
| Analysis Run | Run GET/Events、RunResult、Task请求DTO及消费者证据已审；CTR-046/047 Review已完成 | 等待Server责任方补齐权威OpenAPI和对应测试；不重复初审 |
| Provider Probe History | DTO、分页、Repository、Handler、WebUI和CTR-048/049 Review已完成 | 等待total语义裁决、存储错误映射和故障测试；不重复初审 |
| Market Session / Index Catalog / Index Constituents / Index Daily Bars | 字段审计及对应CTR-023/028～036归类已完成 | 仅保留产品范围决议、能力/单位证据和对应关闭条件；不重复展开 |

### 分类约束

- “工具模块存在引用”与“默认页面已消费”分开记录。
- 产品范围内应有但未接入的能力，才考虑登记 P2；明确预留能力不自动登记。
- 没有页面消费者但 Client 存在时，先确认产品范围，不自动判定缺陷。
- 测试覆盖缺口与 DTO/字段语义错误分开登记。
- 本盘点没有修改业务代码，也没有合并其他分支。

## 6. 本轮结论

WebUI API Client 的入口清单已完成第一轮盘点：44 个导出函数中 3 个为本地设置函数；其余 41 个远端入口有 40 个在主要页面模块找到精确引用，`getMarketSentiment` 是唯一已确认的孤儿 Client。当前剩余工作不是继续无边界扫描 Client，而是按第 5 节明确列出的数据集补齐 DTO/Schema/Golden 和消费证据，然后做维度 2 完整性 Review。

当前不能据此宣布维度 2 完成：页面调用点存在不代表字段语义、OpenAPI required/nullable、错误映射和 Golden 测试已经全部核验。


## 7. W2-E 第一轮完整性 Review 状态校正（2026-11-10）

本次校正以固定代码审计基线 `b6dfc85ba214fe16268c8cba780641a8bfe15086` 和现有数据集审计章节为准。此前第5节将若干已经完成切片的数据集仍列为“待核”，属于索引过期；现已更新为“已审、仅待增量验收”，不得据此重新开展初审。

- 数据集覆盖：第3节矩阵列出的 WebUI 维度2数据集均有独立审计切片；后续 API 盘点新增的 Hot Rankings、Market Sentiment History、Limit Pool、Dragon Tiger、Stock Search、Analysis Run、Provider Probe History 均已有专门章节或CTR冻结影响Review。
- API消费映射：44个导出函数、41个远端入口、40个主要页面模块引用的盘点结果保持不变。唯一未发现主要页面消费者的是 `getMarketSentiment`，继续列为产品范围待确认，不自动新增CTR。
- 服务端契约入口：Market Data 35个OpenAPI path（含2个健康路由）已与路由入口盘点；Analysis Run GET/Events缺失OpenAPI由CTR-046承接。健康检查/诊断访问策略归维度3，不混入本轮。
- 编号去重：CTR-003→046、CTR-005→047；CTR-041 withdrawn；CTR-010为124字段聚合项；CTR-038与CTR-048分别保留；CTR-001/002归维度1。无新增CTR。
- 冻结影响：CTR-043仍为P1；其余本维度活动差异按唯一矩阵保留各自P2/待裁决状态。第一轮完整性Review不等于修复、CI通过或契约冻结。

### 7.1 第一轮结论

**WebUI 维度2第一轮完整性Review完成（带开放差异与证据保留项）**。完成的含义仅为审查范围、数据集覆盖、API消费映射、编号去重、问题分类及遗留关闭条件已登记完整；不表示所有差异已关闭，也不表示该维度可整体冻结。

CTR-010逐字段适用性/无值原因证据、CTR-043 P1处置、其余活动CTR的契约裁决/实现修复/测试补证仍在项目主线待办中。后续只做责任方新提交、正式决议和测试证据的增量验收，不重复完整性Review。
