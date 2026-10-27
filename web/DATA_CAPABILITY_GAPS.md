# 数据能力缺口与服务巡检设计

更新时间：2026-10-27
状态：数据层需求登记。除“当前已有”外，其他均为规划，不代表 API 已实现。
配套文档：[产品排版计划](./UI_LAYOUT_PLAN.md) · [当前 API 契约](./API_CONTRACT.md) · [实施任务清单](./PRODUCT_BACKLOG.md)

## 1. 当前已核实接口与边界

### 1.1 服务与状态

| 服务 | 已存在能力 | 已知限制 |
|---|---|---|
| WebUI/Nginx（`stock-analysis-web`） | 页面可加载；Compose healthcheck 请求容器内 `/` | 浏览器没有独立的服务诊断面板接口；页面能打开只代表静态页面已加载 |
| Go 行情服务（`stock-analysis-data`） | `GET /health/live`、`GET /health/ready` | ready 只给整体就绪状态，不能分辨每个 Provider 的实时健康 |
| Go 分析服务（`stock-analysis-server`） | `GET /healthz`、`GET /readyz` | `/readyz` 可返回 MariaDB/Redis 依赖状态；服务不可达时无法通过该响应单独确认依赖故障 |
| MariaDB | 分析服务 ready 响应中有 `mariadb` 状态 | WebUI 不直连数据库；暂无独立给浏览器的诊断 API |
| Redis | 分析服务 ready 响应中有 `redis` 状态 | 同上；不能把分析服务故障直接等价于 Redis 故障 |
| 上游 Provider | `GET /api/v1/data-sources` 返回配置/能力清单 | 配置声明不是实时探测，不保证某项数据当前能成功拉取 |

权威代码：[行情服务 Handler](https://github.com/Kasign/stock_data_provider/blob/feat/full-go-data-providers-api-switch/services/market-data/internal/httpapi/server.go)、[分析服务 Handler](https://github.com/Kasign/stock_data_provider/blob/feat/full-go-data-providers-api-switch/internal/platform/httpapi/server.go)、[Compose](https://github.com/Kasign/stock_data_provider/blob/feat/full-go-data-providers-api-switch/docker-compose.yml)。

### 1.2 当前已有的 A 股数据域

| 数据域 | 现有 API/字段 | 能做什么 | 尚存边界 |
|---|---|---|---|
| 个股行情 | `GET /api/v1/quotes/{symbol}`；价格、涨跌、成交量/额、可选换手率/量比、源端时间、来源和质量 | 详情与研究列表的核心行情字段 | 换手率/量比可能缺失；不保证所有 Provider 都提供 |
| 日 K | `GET /api/v1/bars/{symbol}/daily`；OHLC、成交量/额、日期、复权口径、来源与质量 | K 线和基于历史收盘价的 MA 计算 | 全市场批量 K 线/均线没有统一接口 |
| Bundle | `GET /api/v1/bundles/{symbol}` | 单股报价、日 K、快照 ID、参考时间和质量 | 单证券接口，不是全市场扫描 |
| 指数 | `GET /api/v1/market/indices` | 现有核心指数 | 不是通用全球指数目录/报价 |
| 市场广度 | `GET /api/v1/market/breadth` | 涨跌分布、覆盖数、部分涨跌停统计和成交额聚合 | `total_amount_cny` 是成交额，不是净流入；有覆盖范围限制 |
| 行业/概念 | `GET /api/v1/market/sectors?type=industry/concept` | 现有排行、涨跌幅、部分成交额、领涨股 | 未核实通用排序参数、板块近五日表现或完整资金净流入榜 API |
| 涨跌停池 | `GET /api/v1/market/limit-pools` | 涨停、跌停、炸板及 Provider 实际提供的连板等字段 | 必须保留字段与统计口径差异 |
| 人气榜 | `GET /api/v1/market/hot-stocks` | 已有 popularity/surge 等真实排名类型 | 排名不能当投资建议；时间与质量必须保留 |
| 龙虎榜 | `GET /api/v1/market/dragon-tiger` | 盘后列表、分页和上榜信息 | 可能延迟发布；按交易日/源端报告日期解释 |
| 交易日历 | `GET /api/v1/market/trading-calendar` | Provider 明确返回的交易/休市日期 | 未提供日期不得用星期规则推断 |
| 公司资料/板块 | `GET /api/v1/stocks/{symbol}/profile`、`/boards` | 公司基础资料和当前板块 | unknown 空结果不等于没有板块 |
| 财务 | `GET /api/v1/stocks/{symbol}/fundamentals` | 指标、报告期、公告日期、质量 | 必须保留报告期和发布日期，禁止把当前数据倒灌历史 |
| 估值 | `GET /api/v1/market/valuations?symbols=...` | 有限数量股票的 PE/PB/PS/PCF 等字段 | 不是全市场分页股票概览 |
| 公告 | `GET /api/v1/stocks/{symbol}/announcements` | 股票公告、日期范围与分页 | 来源覆盖及公告发布时间可能不完整 |
| 筹码/个股资金流 | `GET /api/v1/stocks/{symbol}/chip-distribution`、`/capital-flow` | 个股筹码与资金流摘要 | Provider 可选或未启用；不能直接等价为全市场/板块资金流 |

路由存在不等于部署环境的真实 Provider 已通过验收。字段详情以当前 `services/market-data/openapi.yaml`、Go 模型和 Handler 为准。

## 2. 首页新增数据缺口

### 2.1 市场情绪与历史变化

**当前**：市场广度和涨跌停池可支撑部分即时情绪原子指标；未发现统一的情绪快照历史和历史查询接口。

**需要补齐**：
1. 定义涨跌比例、涨停/跌停、炸板、连板高度等原子指标的分子/分母、覆盖范围、交易日/采集时间和缺失语义。
2. 新增定时快照存储与按交易日/时间范围查询；快照保存 report date、fetched_at、source/version、coverage、quality/reasons 和算法版本。
3. 将“尚未采集、源不支持、确定无记录、采集失败、覆盖不完整”分开表达。
4. 历史图表只读取真实快照；正式积累数据前不画历史曲线，不冒充有数月历史。
5. 初期不创建不透明的 0–100 情绪分。后续若有综合分，公开组成指标和算法版本。

建议接口类型（草案，不是现有路由）：情绪最新快照及按交易日/时间范围读取历史序列；历史入库由定时采集负责，不应靠用户反复刷新。

### 2.2 市场级资金流

市场广度 `total_amount_cny` 和板块 `amount` 是成交额字段；个股资金流 API 也不等价于市场资金流。

需要调研国内 A 股数据源，并新增市场级资金流 Provider/model/API，明确净流入定义、币种/单位、来源时间、报告日期、覆盖范围、来源版本和质量。主力净流入、大单净流入等不同口径不可直接混加。趋势图需要历史快照及明确的起始采集日期；没有真实流向指标时只能显示“未接入”，不得从成交额推导。

### 2.3 板块多维排行

- 今日涨幅：核实现有榜单的实际排序和覆盖范围。
- 成交额排行：需要服务端明确排序、完整候选范围、总数和分页；不能只重排前 N 条。
- 近 5 日涨幅：需要稳定板块 ID、历史日线或快照、交易日窗口、是否包含盘中数据的计算口径。
- 资金净流入：需要板块级资金流 Provider/API 和统一统计定义。
- 每个排名需返回维度、方向、report date、source/version、fetched_at、coverage 与 quality。样本不完整要标注为样本排名，不得宣称全市场 Top 10。

排序维度建议采用后端审核过的枚举；具体 API 路由未确认前都应保持设计草案。

### 2.4 全球市场联动

当前指数和目录服务主要面向配置的国内指数、行业和概念；未核实通用全球指数实时快照与全球交易日历 API。

先调研网络可达性、延迟、历史深度、费用与授权；确定首批指数后，新增独立资产标识、交易所/市场、币种、时区、交易时段、本地交易日期、行情时间、来源与质量。跨市场比较须按可比时点对齐；海外数据调研不阻塞国内 A 股。

## 3. 个股研究列表与批量股票概览

现有 Quote 已包括 current_price、change、change_percent、prev_close、volume、amount、volume_ratio、turnover_rate（后两项可能缺失）与质量/来源/时间；DailyBar 可用于计算 MA5/20/60。
目前没有核实到覆盖全市场的分页证券清单 + 批量行情/日 K/均线统一接口。估值的有限批量接口不能替代它。前端逐股拉取大量报价/K 线/财务/资金流会导致延迟与上游限流。

首版个股研究列表应限定为搜索结果、自选和最近研究。若要全市场扫描/排序，后端需提供批量概览，明确股票范围/筛选分页、关键指标、MA 算法版本、复权口径、最后有效交易日、字段缺失与每标的质量。

## 4. 数据中心“立即检查”与服务诊断

### 4.1 状态必须分层

| 状态维度 | 解释 |
|---|---|
| liveness | 进程是否存活/能响应 |
| readiness | 必需依赖是否就绪 |
| dependency health | MariaDB、Redis 等依赖的实际状态 |
| provider capability | Provider 是否启用、声明支持哪些数据域 |
| provider probe | 真实受控探测是否成功、降级、失败、超时或跳过 |
| data quality | 最新回包是否 valid/degraded/stale/invalid/unknown |

`/api/v1/data-sources` 是能力/配置清单，不是上游实时健康检查。服务端 ready 也不能代表每个 Provider 和每个能力都正常。

### 4.2 立即检查流程

1. WebUI/Nginx 自身可用性；
2. 行情服务 `/health/live`、`/health/ready`；
3. 分析服务 `/healthz`、`/readyz`；
4. 展示分析服务真实返回的 MariaDB/Redis 子状态；
5. 读取 Provider capability；
6. 后端诊断 API 落地后，再执行逐 Provider/能力真实探测。

页面显示检查时间、耗时、状态、错误码/安全摘要和重试入口；顶部汇总 healthy/degraded/unhealthy/unknown。单项失败不应中断其他检查。没有独立证据时，MariaDB/Redis 显示 unknown，不从上层服务不可达推断为依赖故障。

### 4.3 后端诊断 API 设计要求（尚未实现）

需设计受保护的“触发一轮检查”和“读取最近结果”操作。每条结果至少包括 service/dependency/provider/capability 标识、status、checked_at、duration_ms、safe error code/message、last_success_at（有可靠记录时）、source/version 和 quality。
- 有最大并发、单项超时、总超时、冷却/限频。
- 对 Provider 的每项 capability 分别检查，不能以 quote 成功推断该 Provider 的财务/公告/资金流正常。
- 不允许浏览器传任意 URL 或直连 MariaDB/Redis，防止 SSRF 与凭据泄露。
- 诊断结果在持久化存储不可用时也应尽量返回当次真实检查结果，并标注无法持久化；最近快照的存储策略需要后端设计。
- 不输出密码、API Key、Cookie、完整带凭据 URL 或内部敏感配置。

## 5. 数据层任务优先级

| 优先级 | 任务 | 交付 |
|---|---|---|
| P0 | 复用现有健康接口 | 页面区分 WebUI、行情、分析服务 liveness/readiness，显示可读取的 DB/Redis 依赖子状态 |
| P1 | Provider/能力诊断 | 受保护探测 API、限流/超时、逐能力状态、最近检查和安全错误摘要 |
| P1 | 市场情绪快照 | 口径定义、定时采集、历史存储/查询 |
| P1 | 热点板块多维榜 | 完整候选/排序/分页、5 日历史和板块资金流 |
| P1 | 市场级资金流 | 数据源、单位/时点/覆盖与质量契约 |
| P1 | 股票批量概览 | 搜索/分页范围、行情与均线口径、逐标的质量 |
| P2 | 全球指数 | Provider 评估、资产清单、时区/交易时段/币种与质量 |
| P2 | 诊断历史 | 最近成功、故障持续时间、状态趋势和持久化降级策略 |

## 6. 验收原则

- “已有接口”“已接前端”“UI 占位”“测试 fixture”“生产 Provider 实际通过”必须分开记录。
- 每项市场统计要有单位、统计口径、时间、覆盖与 quality。
- 对服务诊断要能区分进程故障、依赖故障、Provider 故障和数据质量问题。
- 只有实现并审核 API 后才在 UI 启用真实交互；本文件中的 API 描述全是需求，不代表当前 Handler 已实现。
