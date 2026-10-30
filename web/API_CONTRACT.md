# WebUI 真实 API 与配置边界

更新时间：2026-10-30  
适用分支：`feat/webui-market-data-console`

本文只描述当前 WebUI 实际调用与当前仓库可核对的接口注册状态。OpenAPI 声明不等于 Handler 已注册，更不等于 Provider 已配置或上游可用。

## 1. 服务边界

| 配置项 | 目标服务 | 用途 | 浏览器保存范围 |
|---|---|---|---|
| 行情 API 基础路径 | `stock-analysis-data` | 行情、证券资料、数据源清单与运行诊断 | 当前浏览器 |
| 行情服务 API Key | 行情服务 | 通过 `X-API-Key` 访问受保护的行情接口 | 当前浏览器；不回显 |
| 分析 API 基础路径 | `stock-analysis-server` | 创建分析任务、查询任务/Run/阶段事件 | 当前浏览器 |
| 分析服务认证 | 分析服务 | 当前 WebUI 不复用行情 API Key | 由分析服务自身决定 |

行情服务健康检查使用 `GET /health/live`、`GET /health/ready`。分析服务健康检查使用 `GET /healthz`、`GET /readyz`，属于独立服务路径，不应拿行情服务 Handler 代替分析服务契约。

## 2. 行情服务接口映射

当前 `web/src/api.js` 的行情接口封装与 `services/market-data/internal/httpapi/server.go` 的路由注册应逐项对照。核心已注册并由 WebUI 封装使用的接口包括：

| 能力 | 方法与路径 | WebUI 封装 | 说明 |
|---|---|---|---|
| Provider 静态清单 | `GET /api/v1/data-sources` | `getDataSources` | 配置/能力声明，不代表健康 |
| Provider 运行诊断 | `GET /api/v1/data-sources/diagnostics` | `getDataSourceDiagnostics` | 只读进程内真实业务请求统计，不主动探测 |
| 批量行情 | `GET /api/v1/market/quotes` | `getMarketQuotes` | 仅查询明确传入的证券列表 |
| 指数行情/目录/成分/日线 | `GET /api/v1/market/indices`、`index-catalog`、`indices/{symbol}/constituents`、`indices/{symbol}/daily` | 对应 `getMarketIndices`、`getIndexCatalog`、`getIndexConstituents`、`getIndexDailyBars` | 目录与行情快照不是同一资源 |
| 市场广度/板块 | `GET /api/v1/market/breadth`、`/market/sectors` | `getMarketBreadth`、`getMarketSectors` | 按实际服务端质量字段展示 |
| 全市场分页股票 | `GET /api/v1/market/stocks` | `getMarketStocks`、`bindMarketStocks` | 个股行情页按需加载；分页展示报价、成交额、换手率、量比、MA5/MA20、页内覆盖率和质量原因；MA/报价缺失保留未提供状态 |
| 市场情绪快照/历史 | `GET /api/v1/market/sentiment`、`/market/sentiment/history` | `getMarketSentiment`、`getMarketSentimentHistory` | 历史只读数据库已保存快照，不补造缺失交易日 |
| 人气榜/涨跌停/龙虎榜/交易日历/时段 | `GET /api/v1/market/hot-stocks`、`limit-pools`、`dragon-tiger`、`trading-calendar`、`session` | 对应同名 `getMarket*` / `getTradingCalendar` 封装 | 不将缺失数据补成零 |
| 个股行情/日 K/Bundle | `GET /api/v1/quotes/{symbol}`、`bars/{symbol}/daily`、`bundles/{symbol}` | `getQuote`、`getDailyBars`、`getMarketBundle` | 保留复权、来源与质量语义 |
| 个股资料/财务/公告/板块 | `GET /api/v1/stocks/{symbol}/profile`、`fundamentals`、`announcements`、`boards` | 对应 `getStock*` 封装 | 分别处理空结果、分页和报告期 |
| 个股筹码/资金流 | `GET /api/v1/stocks/{symbol}/chip-distribution`、`capital-flow` | `getChipDistribution`、`getCapitalFlow` | 个股资金流不是市场级资金流 |

### 已发现的契约差异

- `services/market-data/openapi.yaml` 声明了 `GET /api/v1/market/capital-flow`，但当前 `server.go` 没有注册对应 Handler，WebUI 也没有该接口封装。当前不能将市场级资金流标记为可用。
- 个股资金流 `GET /api/v1/stocks/{symbol}/capital-flow` 已注册 Handler，并有 `getCapitalFlow` 封装。不得用个股资金流冒充市场级资金流。
- `GET /api/v1/market/stocks`、`GET /api/v1/market/sentiment`、`GET /api/v1/market/sentiment/history` 已注册 Handler，且现已提供 WebUI API 封装；市场股票分页尚未接入具体展示列表，情绪历史已提供手动日期范围查询入口。
- `POST /api/v1/data-sources/probe` 是主动探测接口。数据中心的同花顺状态展示不得调用该接口；本页面仅读取 `GET /api/v1/data-sources` 与只读运行诊断快照。不得要求用户提供同花顺 API Key 来完成 UI 验收。

## 3. 分析任务服务边界

WebUI 使用独立的 `analysisBase` 调用：

- `POST /api/v1/analysis/tasks`：创建任务，可附带 `Idempotency-Key`。
- `GET /api/v1/analysis/tasks/{task_id}`：读取任务状态。
- `GET /api/v1/analysis/tasks/{task_id}/events`：按序列游标读取阶段事件。
- `GET /api/v1/analysis/runs/{run_id}`：读取分析结果。
- 健康检查：`GET /healthz`、`GET /readyz`。

这些路径属于分析服务，不属于行情服务 `server.go`。分析任务接口的字段契约以 `doc/接口规范/分析任务.openapi.yaml` 为准。

## 4. 设置中心能力状态

| 设置分组 | 当前能力 | 真实边界 |
|---|---|---|
| 显示偏好 | 已实现 | 主题、行情颜色、精度、单位、自选股排序/显示和日 K 默认参数保存在浏览器 |
| 连接与服务 | 已实现 | 保存行情/分析代理路径及行情服务 Key；连接检查只检查服务健康端点 |
| 浏览器本地数据 | 已实现 | 导入/导出工作区 JSON；不包含 API Key、数据库或服务器配置 |
| 基础设置 | 部分实现 | 仅日 K 默认条数与复权方式本地保存；其他未接入项禁用 |
| 数据源管理 | 只读 | 展示注册信息和能力声明；不支持前端启停、凭据修改或主动测试 |
| AI 模型与问股 | 待接入 | 无服务端模型配置、密钥托管和模型测试 API |
| 通知渠道 | 待接入 | 无渠道 CRUD、订阅管理、发送测试与投递记录 API；仅保留浏览器本地偏好草稿 |
| 任务调度 | 待接入 | 无周期调度 CRUD、状态和执行历史 API |
| 选股默认项 | 待接入 | 无策略 Schema、条件选股配置和扫描任务 API |
| 安全与备份 | 待接入 | 无管理员认证、服务密钥轮换、审计和服务器备份恢复 API |

## 5. 验收规则

1. “已注册”只表示 Handler 存在；“OpenAPI 已声明”只表示协议文档存在；“WebUI 已封装”只表示有前端请求函数。这三者必须分别核对。
2. 未配置 Provider Key、未观测到真实业务请求、服务端接口不可读，必须显示为不同状态；不得互相推断。
3. 同花顺诊断不发起真实上游请求，不消耗第三方额度，不以 API Key 测试作为交付条件。
4. 没有服务端 API 的设置项必须禁用并标记待接入；浏览器本地保存成功不能反馈为服务端配置成功。
5. 任何未在当前 Handler 注册的 OpenAPI 路径不得被页面描述为生产可用能力。


### 全市场分页行情页面

- 入口位于“个股行情”页，用户点击“加载全市场”后才请求 `GET /api/v1/market/stocks`；不会在页面打开时自动遍历全市场。
- 支持每页 20/50/100 条、前复权/后复权/不复权、上一页/下一页和当前页刷新。
- 页面展示证券名称/代码、最新价、涨跌幅、成交额、换手率、量比、MA5/MA20、单条质量状态。
- 汇总展示证券目录数量及是否下限、当前页行情成功/失败数与覆盖率、目录来源/版本/获取时间、行情获取时间和 MA 复权口径。
- `coverage_ratio` 是当前页覆盖率，不解释为全市场覆盖率；`total_is_lower_bound=true` 时总数显示为“至少”。
- MA 或行情字段为 null 时显示“—”，不补零；接口失败时清空当前表格并明确展示失败原因，不保留旧数据冒充新连接结果。
- 点击单条记录进入该股票行情详情；全市场分页只读取用户当前请求页，不自动连续翻页。


- 个股深度研究的“研究标的与快捷入口”列表复用自选股页已读取的批量行情缓存，不额外逐只请求；当前正在研究的股票优先使用本次单股行情快照。批量刷新期间或刷新失败时，保留的旧批量快照会明确标记；没有批量记录的股票仍显示“未加载”。
