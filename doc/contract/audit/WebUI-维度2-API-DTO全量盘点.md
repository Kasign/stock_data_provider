# WebUI 维度 2：API Client、服务端路由与页面消费者全量盘点

**审计代码基线：** `origin/integration` @ `b6dfc85ba214fe16268c8cba780641a8bfe15086`  
**文档维护分支：** `feat/design_doc`  
**盘点范围：** `web/src/api.js`、WebUI 主要页面模块、Market Data HTTP 路由/OpenAPI、Analysis Task OpenAPI、`web/tests/api.test.mjs`。  
**本记录性质：** 接口入口和消费关系盘点，不代替各 DTO 数据集的字段语义审计。

## 1. 盘点结果

- `web/src/api.js` 共导出 44 个函数，其中 3 个为本地设置读写（`loadSettings`、`saveSettings`、`clearSettings`），其余 41 个为远端请求/健康检查入口。
- 41 个远端入口均在 WebUI 主要页面模块中找到直接引用；本轮没有发现“定义了 API Client 函数但在主要页面模块中完全无引用”的孤儿函数。
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
| getMarketSentiment / getMarketSentimentHistory | market-overview.js | 已消费；情绪主快照已在市场广度切片关联核对，历史 DTO 需确认是否独立覆盖 |
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

这些入口应在后续逐项核对对应浏览器测试是否覆盖真实请求参数、错误/空响应和取消行为；不得仅因缺少单元测试就自动登记为契约缺陷。

## 5. 待核数据集与收口分类

| 数据集/入口 | 已有审计状态 | 后续动作 |
|---|---|---|
| Hot Rankings | 本次盘点确认页面消费，未在第 1–36 节索引中确认独立字段语义切片 | 待核 DTO、OpenAPI、Provider/Service、页面字段 |
| Market Sentiment History | 主快照已随 Market Breadth 核对；历史响应需确认独立 DTO/Schema/页面消费 | 待核历史记录字段、日期/窗口、空值语义 |
| Limit Pool | 页面消费已确认；尚未从当前审计目录索引确认独立切片 | 待核池类型、日期、排序/截断、来源和页面消费 |
| Dragon Tiger | 页面消费已确认；尚未从当前审计目录索引确认独立切片 | 待核分页、金额单位、日期/席位语义及页面消费 |
| Stock Search | app.js 消费已确认；既有证券资料切片是否覆盖搜索 DTO 尚待定位 | 核对搜索候选 total、证券代码/交易所语义及 UI |
| Analysis Run | Client 和路由存在；任务 DTO 已审，但 Run 结果结构不等同 Task | 核对 Run DTO、事件结构、Schema/Golden 与结果页消费 |
| Provider Probe History | Client/页面/请求测试已接通；已有 Probe 切片覆盖情况待索引确认 | 核对历史记录 DTO、分页/截断和诊断字段 |
| Market Session / Index Catalog / Index Constituents / Index Daily Bars | 数据集字段审计已完成 | 只在最终完整性 Review 中复核，不重复展开 |

### 分类约束

- “工具模块存在引用”与“默认页面已消费”分开记录。
- 产品范围内应有但未接入的能力，才考虑登记 P2；明确预留能力不自动登记。
- 没有页面消费者但 Client 存在时，先确认产品范围，不自动判定缺陷。
- 测试覆盖缺口与 DTO/字段语义错误分开登记。
- 本盘点没有修改业务代码，也没有合并其他分支。

## 6. 本轮结论

WebUI API Client 的入口清单已完成第一轮盘点：44 个导出函数中 3 个为本地设置函数，其余远端入口均在主要页面模块找到引用。当前剩余工作不是继续无边界扫描 Client，而是按第 5 节明确列出的数据集补齐 DTO/Schema/Golden 和消费证据，然后做维度 2 完整性 Review。

当前不能据此宣布维度 2 完成：页面调用点存在不代表字段语义、OpenAPI required/nullable、错误映射和 Golden 测试已经全部核验。
