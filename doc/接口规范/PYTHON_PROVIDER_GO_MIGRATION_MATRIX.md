# Python Provider → Go 迁移矩阵

- 适用分支：`feat/full-go-data-providers-api-switch`
- 静态检查基线：`4c603172448102058f95e6bc625494436a8402c9`
- 更新日期：2026-10-30
- 检查范围：当前仓库中的 Go Provider 接口、Provider 实现、启动注册、`providers.json`、Python bridge 适配代码。
- 证据限制：当前仓库没有完整包含原始 Python `DataFetcherManager`、Efinance、AkShare 的全部实现；本矩阵不能证明旧系统每个函数的来源和字段契约。

## 1. 状态定义

| 状态 | 含义 |
|---|---|
| 已实现且启用 | Go 有对应接口实现，且默认配置启用 |
| 已实现但关闭 | Go 有对应实现，但默认配置或独立开关关闭 |
| 已实现待运行验收 | 静态代码可见，尚未用目标环境真实请求验证 |
| 部分覆盖 | 仅覆盖数据域的一部分字段、场景或接口 |
| 未实现/未确认 | 当前仓库没有足够代码证据，不得按已实现处理 |
| 暂缓 | 不属于当前国内 A 股迁移范围 |

注意：配置启用不等于上游可用；实现存在不等于字段等价；接口返回成功不等于数据质量合格。

## 2. Go Provider 能力矩阵

以下“代码能力”根据当前分支的 Go 方法实现和启动注册静态核对；“默认状态”根据 `services/market-data/configs/providers.json` 与独立配置开关核对。

| Provider | 代码已实现的主要能力 | 默认状态/配置情况 | 迁移判断 |
|---|---|---|---|
| 东方财富 eastmoney | 实时行情、日 K、证券基础资料、搜索、板块排行、指数行情、市场宽度、涨跌停池、个股所属板块、龙虎榜、公告、筹码、个股资金流、人气榜 | 启用；上述主要能力开关启用 | 国内主数据源；仍需逐数据域真实样本和分页覆盖验收 |
| 腾讯 tencent | 实时行情、日 K、证券搜索 | 启用；quote/daily_bars/stock_search 启用 | 行情与搜索备用源；日 K 金额字段存在明确缺失语义，不能估算补齐 |
| 新浪 sina | 实时行情、日 K | 启用；quote/daily_bars 启用 | 行情备用源；成交额字段需保持 unknown/null 的既有约束 |
| 网易 netease | 实时行情、日 K | 启用；quote/daily_bars 启用 | 行情备用源；单位推断分支必须保留 degraded 语义 |
| Pytdx pytdx | 实时行情、日 K | 启用；quote/daily_bars 启用 | 行情备用源；金额与成交量尺度校验不可绕过 |
| BaoStock baostock | 日 K、交易日历、财务指标 | 启用；daily_bars/trading_calendar/fundamentals 启用 | 低频数据源；不支持实时行情，不能配置为 quote Provider |
| TickFlow tickflow | 实时行情、日 K、指数行情、市场宽度、行业/概念排行（含方向） | 启用；仅 daily_bars 默认启用，quote/index_quotes/market_breadth/sector_rankings 默认关闭 | **代码能力与配置不一致**；需先评估真实接口可用性、权限和覆盖，再决定启用 |
| 同花顺 hithink | 实时行情、日 K、搜索、指数行情、交易日历、人气榜、涨跌停池、龙虎榜、指数目录/成分/日线、估值、财务 | 整体关闭，能力开关关闭 | **代码能力与配置不一致**；优先补充能力/权限核实，不应重复开发已有方法 |
| Tushare tushare | 日 K、证券基础资料、交易日历、财务、估值、涨跌停池、龙虎榜、公告、指数目录/成分/历史成分/日线、历史交易状态 | 整体关闭，能力开关关闭 | **代码能力与配置不一致**；依赖 Token/积分权限，需按接口逐项核验可用权限 |
| 雪球 xueqiu | 人气榜 | 整体关闭，hot_rankings 关闭 | 可选人气榜来源；需验证登录态/Cookie、限流与数据覆盖 |
| Yahoo Finance yahoo | 实时行情、日 K | 整体关闭 | 海外能力，按当前国内 A 股范围暂缓 |
| 妙想 miaoxiang | 个股筹码分布、个股资金流 | 由独立 `MXEnabled`、API Key 配置控制，不在 providers.json 的普通 Provider 列表内 | 补充数据源；不得误记为普通 Provider 优先级链中的已启用源 |

## 3. 当前默认启用能力

| 数据域 | 默认启用的主要来源 | 已发现的限制 |
|---|---|---|
| 实时行情 | 东方财富、腾讯、新浪、网易、Pytdx | TickFlow 虽有实现但默认 quote 关闭；不同来源字段和单位契约不同 |
| 日 K | 东方财富、BaoStock、腾讯、新浪、网易、TickFlow、Pytdx | Tushare/同花顺已实现但关闭；来源复权算法不可默认视为等价 |
| 证券搜索 | 东方财富、腾讯 | 同花顺搜索实现存在但当前关闭 |
| 指数行情 | 东方财富 | TickFlow、同花顺实现存在但当前关闭 |
| 市场宽度 | 东方财富 | TickFlow 实现存在但当前关闭；不能据此推断历史情绪快照已实现 |
| 板块排行 | 东方财富 | TickFlow 实现存在但当前关闭；资金流入和 5 日涨幅排序需单独核对 |
| 财务 | BaoStock | Tushare、同花顺实现存在但关闭；PIT/公告可用时间仍需对账 |
| 估值 | 无默认启用的专用估值 Provider 配置 | Tushare、同花顺代码实现存在但关闭 |
| 交易日历 | BaoStock | Tushare、同花顺实现存在但关闭 |
| 历史交易状态 | 无默认启用来源 | Tushare 已实现但关闭 |
| 龙虎榜 | 东方财富 | Tushare、同花顺实现存在但关闭 |
| 公告 | 东方财富 | Tushare 实现存在但关闭 |
| 个股资金流 | 东方财富；妙想需独立启用 | 多源口径、报告日期和覆盖范围仍需核验 |
| 筹码分布 | 东方财富；妙想需独立启用 | 多数为当前快照/估算结果，不等同严格历史 PIT |
| 人气榜 | 东方财富 | 同花顺、雪球实现存在但关闭 |

## 4. Python bridge 当前可见调用清单

检查文件：`python_bridge/capabilities/market_data.py`。

| Python bridge 能力 | 可见旧调用 | Go 对应情况 | 当前结论 |
|---|---|---|---|
| 个股日 K | `DataFetcherManager.get_daily_data` | 多个 Go Provider 已实现 | 不能仅凭方法名判定字段等价；旧 bridge 将单位和复权标为 unknown |
| 个股实时行情 | `get_realtime_quote` | 多个 Go Provider 已实现 | 需同代码、同时间样本对比 |
| 股票名称/基础身份 | `get_stock_name` | 东方财富、Tushare 等有基础资料能力 | 需核对退市、改名、上市日期和代码空间 |
| 筹码分布 | `get_chip_distribution` | 东方财富、妙想有实现 | 需区分计算口径、更新时间和历史可用性 |
| 财务上下文 | `get_fundamental_context` | BaoStock、Tushare、同花顺有实现 | 需按字段、报告期、公告时间和修订版本对账 |
| 所属板块 | `get_belong_boards` | 东方财富有个股板块能力 | 需核对行业/概念分类体系及更新时间 |
| 市场概览 | `MarketAnalyzer.get_market_overview` | Go 有市场宽度、指数和板块等独立能力 | 不能视为完整等价；需先定义市场概览指标口径 |
| 个股辅助快照 | 筹码、财务、板块组合 | Go 有独立数据接口 | 旧桥历史 as_of 会跳过无 PIT 能力的当前辅助快照 |

Python bridge 还包含 search、LLM、notification、persistence 等非行情能力。本矩阵只盘点行情相关调用，不将这些能力误归为行情 Provider 迁移缺口。

## 5. 明确的迁移缺口

### P0：字段级等价与证据不足

- [ ] 获取并固定原始 Python Provider 上游版本/提交，确认 DataFetcherManager、Efinance、AkShare 的实际调用链。
- [ ] 建立同证券、同交易日、同复权口径的 Python/Go 脱敏样本。
- [ ] 对比日 K OHLC、成交量、成交额、涨跌幅、null/zero、日期覆盖和复权算法。
- [ ] 对比实时行情源端时间、采集时间、价格、成交量、成交额和质量状态。
- [ ] 对比财务报告期、公告时间、修订版本和可用时间。
- [ ] 对比筹码、板块、资金流、龙虎榜、公告和人气榜字段及分页覆盖。

### P1：代码已实现但默认关闭，需先验证再启用

- [ ] TickFlow：quote、index_quotes、market_breadth、sector_rankings。
- [ ] 同花顺：quote、daily_bars、index_quotes、trading_calendar、hot_rankings、limit_pools、dragon_tiger、fundamentals、valuations、index_catalog 等。
- [ ] Tushare：stock_basic、trading_calendar、security_trading_status、fundamentals、valuations、limit_pools、dragon_tiger、announcements、index_catalog 等。
- [ ] 雪球：hot_rankings。
- [ ] 妙想：独立开关下的筹码和资金流真实请求、权限及质量验证。

以上项目先做权限、限流、字段和质量验证，再决定是否打开默认配置。不要在未验证前批量打开所有开关。

### P1：仍需新增或扩展的数据能力

- [ ] 全市场批量股票概览（已有指定股票批量 quote，不等于全市场扫描）。
- [ ] 市场情绪历史快照（当前市场宽度快照不等于历史序列）。
- [ ] 市场级资金流及历史序列（个股资金流不等于市场资金流）。
- [ ] 板块多维排行：今日涨幅、成交额、近 5 日涨幅、资金净流入，需分别定义完整候选集和排序口径。
- [ ] Provider 主动探测及逐 Provider、逐 capability 健康检查。
- [ ] 诊断历史持久化。
- [ ] 严格 PIT：财报修订、指数成分历史生效区间、公司行动/复权因子和 publication/available_at。

## 6. 对账验收规则

每个数据域使用同一证券集合、同一交易日/观察时点、同一复权口径。样本至少覆盖沪深主板、创业板、科创板、北交所、停牌、非交易日、复权事件、缺失字段和上游错误。

逐字段记录：
- Python 原始字段、Go 标准字段、转换规则和单位；
- 数值差异、精度、null/zero 差异；
- 交易日覆盖、重复/乱序/越界记录；
- 来源、质量状态、质量原因、fallback；
- fetched_at、provider_time、report_date、announcement_time、available_at；
- 分页总数、返回数、去重数、覆盖率及是否 lower-bound。

判定只能使用：`一致`、`可解释差异`、`Go 更严格`、`缺少证据`、`Go 缺失能力`、`旧实现缺失能力`。未获得旧 Python 原始实现或真实样本时，必须写“缺少证据”，不得推断为等价。

## 7. 维护约束

- Go 已有 shares/CNY 单位统一、复权语义、null/zero 区分、质量门禁和诊断元数据必须保留。
- 未验证的单位、复权、来源、时间和覆盖率必须保持 unknown/null/degraded。
- Python 兼容仅在边界转换，不反向污染 Go 内部标准模型。
- 删除旧 Python 适配器前，必须完成调用方迁移、固定样本对账和目标环境回归。
- 本矩阵是静态盘点基线；真实运行验收记录维护在 `DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md`。


## 8. 原 Python Provider 上游基线已固定（2026-10-30）

本轮已定位并固定原项目源码，不再把“旧 Python 实现不在当前 Go 仓库”当作无法继续静态对照的理由。

- 原仓库：`Kasign/daily_stock_analysis`
- 分支：`main`
- 固定提交：`7a21cd76952c7fe44a7fc5a6113a5b2b8a16bb32`
- 提交日期：2026-10-17
- 关键文件：
  - `data_provider/base.py`
  - `data_provider/efinance_fetcher.py`
  - `data_provider/akshare_fetcher.py`
  - `data_provider/tushare_fetcher.py`
  - `data_provider/tickflow_fetcher.py`

### 8.1 Python 实时行情调用链

`DataFetcherManager.get_realtime_quote` 对普通 A 股按以下路径尝试：
1. `EfinanceFetcher.get_realtime_quote`
2. `AkshareFetcher.get_realtime_quote(source="em")`
3. `AkshareFetcher.get_realtime_quote(source="sina")`
4. `AkshareFetcher.get_realtime_quote(source="tencent")`
5. 全部失败返回空结果。

指数走独立固定来源链，不与普通个股路径混为一谈。

### 8.2 Python 日 K 注册和回退链

`DataFetcherManager._init_default_fetchers` 默认创建并按优先级排序：
1. EfinanceFetcher（P0）
2. AkshareFetcher（P1）
3. PytdxFetcher（P2）
4. BaostockFetcher（P3）
5. YfinanceFetcher（P4）
6. TencentFetcher（P5）

配置 Tushare Token 时，额外创建 TushareFetcher，并由其初始化状态动态提升优先级；配置 TickFlow Key 时加入 TickFlowFetcher；妙想、长桥、富途等按各自凭据/端点可用性加入。日 K 管理器过滤市场与能力后逐源尝试，成功时返回 `(DataFrame, source_name)`。

### 8.3 Python 字段处理与 Go 的确定差异

| 字段/行为 | Python 固定源码 | Go 当前契约 | 判定 |
|---|---|---|---|
| Efinance 日 K 缺失 volume | `_normalize_data` 补为 0 | 保留 null/unknown/degraded 或拒绝不完整关键字段 | Go 更严格；不得为表面一致改成 0 |
| Efinance 日 K 缺失 amount | `_normalize_data` 补为 0 | 保留 null/unknown/degraded | Go 更严格 |
| Efinance OHLC 缺列 | 缺 open 时以 close 补 open/high/low | 严格 OHLC 检查，不能伪造完整价格区间 | Go 更严格 |
| AkShare 日 K | 中文列名映射后交给 BaseFetcher 清洗 | Go Provider 各自显式字段映射、单位与质量校验 | 需要同一原始样本比较 |
| Python BaseFetcher 清洗 | 数值化、剔除 close/volume 为空的行、日期升序 | Go 校验日期、OHLC、数量级、重复和来源质量 | 规则不完全相同，需记录可解释差异 |
| Python 技术指标 | MA5/MA10/MA20、volume_ratio（前五日均量比较） | 当前 Go DailyBar 核心字段含 MA5/MA20/MA120/MA250，不等同 Python 指标集合 | Go 缺 MA10/volume_ratio 对应字段，属于接口能力差异 |
| Python 实时行情 | Efinance 先尝试，再由 AkShare 分别尝试 EM/Sina/Tencent | Go 按配置的 Provider 能力优先级路由 | 路由顺序不是 1:1，需用来源链记录比较 |

### 8.4 证据边界

上述内容属于固定提交源码的静态调用链和字段处理对照，已经比之前的“缺少原始实现”更进一步，但仍不能证明两边实时数据值等价。当前 Go 仓库没有包含原 Python `data_provider` 包及其运行依赖；本轮未执行旧 Python Provider 的真实网络请求。

后续字段级对账必须使用同一证券、同一交易日、同一复权基准，分别从固定 Python 提交和 Go 服务导出 JSON，再比较价格、成交量、成交额、指标、缺失值、来源和时间字段。旧 Python 将缺失值补零的行为应标记为“Go 更严格”，不得将其作为 Go 回归目标。
