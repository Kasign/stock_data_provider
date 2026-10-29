# Go 数据服务迁移状态与验收边界

> 更新时间：2026-10-28  
> 当前分支：`feat/full-go-data-providers-api-switch`  
> 当前定位：分阶段迁移；不是全 Python 去依赖的最终版本。

## 0. 国内 Provider 静态审查记录（2026-10-27）

### 审查与验证边界

本轮及后续改动以源码、配置和文档静态审查为主；没有手动执行测试、构建、CI 或真实 Provider 请求。此前普通推送曾自动触发仓库已有的公共 Provider 探测、Go 检查/构建及 WebUI 校验工作流，结果并非全部成功。为遵守统一验收安排，后续提交使用 `[skip ci]`；需要注意，这不会代替本地或部署环境的实际验证。

### 国内 Provider 配置接线

- **腾讯股票搜索**：`MARKET_DATA_TENCENT_STOCK_SEARCH_URL` 已接入配置读取、URL 校验和启动装配；旧构造函数继续兼容，空值回落 Smartbox 默认端点。腾讯搜索没有可核验的结果总数，因此仍保持 `degraded`，服务层可回退到东方财富。
- **URL 环境变量**：配置校验清单中的 27 个 HTTP(S) Provider URL 字段已与 `services/market-data/.env.example` 对照，未发现遗漏。新增 URL 必须同步配置字段、环境变量、URL 校验、根/服务模板、启动装配和文档。Pytdx 主机列表、BaoStock 主机/端口是 TCP 配置，不算 URL。
- **默认能力文档**：已校正文案使其与 `providers.json` 对齐。当前默认配置启用网易行情/日 K 作为后置回退、BaoStock 财务/交易日历；这不代表相关上游在部署网络中已完成稳定性验收。Yahoo 默认关闭。README 已改为描述“部分国内数据域已接入”，同时明确字段覆盖、权限、质量和时点语义尚不等价。

### Tushare 财务报表扩展（2026-10-28）

- 已在现有 Tushare `FundamentalsProvider` 中增加 `income`、`balancesheet`、`cashflow` 三类报告端点，复用既有季度快照 API，不新增重复路由。
- 新增分区 `income_statement`、`balance_sheet`、`cash_flow_statement`，利润表/现金流量表的一季度使用 `report_type=1`（一季报本身为单季度累计口径），第二至第四季度使用 `report_type=2` 单季合并口径；资产负债表各季度均使用 `report_type=1` 期末合并口径，并筛选已公开公告版本；公告日期优先使用实际公告日期 `f_ann_date`，缺失时才回退 `ann_date`，未来或报告期前的公告版本不选用。 已扩展常见损益科目、费用/利润线、行业专属收入、应收应付组合项、租赁相关资产负债、经营/投资/筹资现金流及间接法项目；科目名称与原始字段一一对应，不合成不存在的上游科目。
- 三张报表端点权限/响应失败时只把对应分区标记为降级，不丢弃已有六组标准指标；缺字段保留缺失并记录质量原因，不补零。 行业/报表结构可缺失的可选明细科目为空时不会单独降级；关键汇总科目缺失才附加 `required_statement_metric_missing_or_invalid:<field>` 并降低分区质量。 反向情况同样处理：若 `fina_indicator` 失败而任一报表端点仍可用，六组指标保留为显式降级/unknown 分区，已获得的报表数据仍能组成 degraded 快照，不会因首个端点失败整体退出。 若 `fina_indicator` 成功但没有已披露候选，父快照的 `quality_reasons` 也保留 `no_published_indicator_row_for_requested_period`，不会只在六个子分区记录原因。
- 代码与 HTTP fixture 测试源码已提交，本轮未运行测试、构建、CI 或真实 Provider 请求。Tushare 账号权限和字段级数据仍须统一验收。

### 非空 unknown 快照的事件域回退补齐
- 总股本、流通股本字段已补异常值回归测试源码，覆盖总股本非正、流通股本为负及流通股本超过总股本；本轮仍未运行测试。

- 涨跌停池候选即使父级质量仍为 unknown，只要源端 `total` 大于返回 `items`，也保留 `provider_limit_pool_response_truncated`，但不会因此把 unknown 提升或降级成其他状态。
（2026-10-28）

- 涨跌停池与龙虎榜现在允许父级质量为 `unknown`、但条目本身结构完整且仍保留独立质量状态的快照进入回退链；unknown 不再被直接当作无效 Provider 错误。
- 所有 Provider 都没有返回非 unknown 结果时，服务层按条目数和可用字段覆盖选择候选，标记 `*_fallback_exhausted_without_usable_quality` 或既有空结果原因，再使用短 TTL；旧 unknown 缓存如果没有回退耗尽标记，会在多 Provider 场景跳过。
- 新增本地回归测试源码，未运行测试、构建、CI 或真实 Provider 请求。

### 个股所属板块与公告的非空 unknown 回退（2026-10-28）

- 个股所属板块和公告接口现在允许父快照质量为 `unknown`、但条目本身通过结构校验的结果进入回退链；多 Provider 场景会绕过未标记回退耗尽的 unknown 缓存。
- 若没有后续 Provider 提供非 unknown 结果，按条目数与可用字段覆盖保留候选，记录 `stock_boards_fallback_exhausted_with_unconfirmed_quality` 或 `announcements_fallback_exhausted_without_usable_quality`，使用短 TTL。
- 新增本地回归测试源码，未运行测试、构建、CI 或真实 Provider 请求。

### 质量与时间契约修复
- **指数目录 unknown 候选择优**：回退链耗尽后，优先保留条目更多的目录；条目数相同则优先有可核验源端时间、且源端时间更新的快照。
- 回退耗尽时使用字段覆盖评分选择 unknown 日 K 候选：先比较返回条目数，再比较涨跌幅、成交量/成交额及源端时间字段的覆盖数，覆盖相同时优先较新的源端时间。

| 数据域 | 当前服务层约束 |
|---|---|
| 市场广度 | 分类家数必须对账；成交额覆盖不完整时总额为 null、质量不得为 valid；非原子成交额聚合或未暴露源快照时间的质量原因不能被忽略；报告日期必须规范且不在未来。 |
| 指数行情/指数目录/指数日 K | 指数价格、金额单位及有限数值受校验；单项降级不能被父快照标成 valid；存在源时间时不能为零或明显晚于当前时间。指数日 K 父快照缺少源端快照时间时必须降级，不能标记 valid。未知/空指数目录仅缓存 5 分钟。 |
| 指数成分/指数日 K | 成分快照校验报告日期、源时间、来源返回/排除数量；缺名称/重复去重不能被隐藏为完整有效结果。指数日 K 校验交易日期、OHLC、涨跌幅、成交量/成交额单位与空值一致性，禁止父级 valid 掩盖降级条目；Provider 和服务层都拒绝未来 end_date，Tushare 成交量转换拒绝 >= 2^63 的浮点边界。 |
| 行业/概念排行、涨跌停池 | 校验记录总数、排名/代码唯一性、必填值、非有限数值、计数/金额范围和单位，并传播子项质量状态；Eastmoney/Tushare 涨跌停池 Provider 都拒绝未来交易日期；Tushare 空 unknown 不伪造源端报表日期，龙虎榜/涨跌停池 Provider 直接拒绝未来交易日期。东方财富计数字段拒绝小数、负数及浮点安全整数范围外的值，不再静默截断。 |
| 个股板块、龙虎榜、热度榜、公告 | 父级快照不能高于任一条目的质量等级；日期、分页与空结果语义必须一致。未确认的空列表保持 unknown，而不是伪装成“确认没有数据”。 |
| 财务数据 | 报告期必须是请求季度的期末且不晚于中国市场当前日期；披露日期必须规范、不得在未来，且不得早于对应报告期末。 |
| 证券主数据 | 缺少质量状态时降级并附加原因；明确返回 unknown 时继续回退，链路耗尽后保留 unknown 候选并短缓存，不自动提升为 valid。 |
| 筹码分布、资金流 | 缓存命中与新 Provider 回包使用同一套字段校验；拒绝越界比例、非有限/反向成本区间，以及观测数量不足的 5/10 日合计。由于当前模型没有精确来源可用时间字段，即使 Provider 或旧 Redis 缓存标 valid，也会降级/拒绝，并记录 `publication_availability_timestamp_unverified`。 |
| Tushare 日 K | 结束日期先规范化为 `YYYY-MM-DD` 再做未来日期比较，避免输入带首尾空格时校验口径不一致。 |

### 同花顺龙虎榜源端未来日期防护（2026-10-28）

- Provider 即使未收到显式查询日期，也必须核对响应的 `trade_date` 不晚于中国市场当前日期；对未来报告日期直接报错，不将其缓存为可用记录。
- 新增本地 HTTP fixture 测试源码，未发起真实 Provider 请求，也未运行测试、构建或 CI。

### 人气榜与估值 unknown 回退补强（2026-10-28）

- **指数成分 unknown 候选择优**：回退耗尽时先比较成分数，再比较权重、Provider 时间和报告日期覆盖；同等覆盖时优先更晚的源端时间/报告日。
- **筹码 Provider nil 防护**：原生筹码和补充数据 Provider 列表在去重前忽略 nil 项，避免空配置项触发 `Name()` panic。

- **筹码来源去重**：筹码 Provider 与补充 Provider 合并后，按名称去除首尾空白并忽略大小写去重，避免 `eastmoney` 与 ` EASTMONEY ` 被当作不同来源重复请求。
- **财务/资金流/筹码 unknown 候选择优**：如果所有 Provider 都没有给出可用质量的结果，财务优先保留已填充分区和指标更多的快照；资金流优先保留金额指标更多的候选；筹码分布优先保留成本区间、获利比例和集中度字段更多的候选。覆盖相同则按各域可用的源端日期/注册顺序进行稳定选择。

- **过期证券搜索结果择优**：当所有证券搜索 Provider 都未返回非过期结果时，保留匹配条目更多的 stale 候选；条目数相同则优先选择抓取时间较新的快照，避免最后一个来源覆盖更完整的目录结果。

- **证券搜索**：未知质量但非空的证券目录结果不再直接返回；继续尝试后续 Provider，并在全部来源失败后保留覆盖更完整的 unknown 候选。回退耗尽原因在迁移与契约文档中同步记录。

- 人气榜/飙升榜现在将“有条目但父快照质量为 unknown”视为可回退状态，不再直接返回并以正常榜单 TTL 缓存；旧 unknown 行缓存没有回退耗尽原因时，多 Provider 场景下会被绕过。 回退耗尽时若候选条目数相同，优先保留行情、热度、排名变化等非空字段更多的榜单；同覆盖且同源时间时保持原 Provider 顺序。
- 估值回退链耗尽时优先保留返回股票数量较多的 unknown 快照；同覆盖时比较非空估值指标数。未知质量但带条目的候选使用独立的 `valuations_fallback_exhausted_without_usable_quality` 标记并短缓存。
- 上述改动仅补源码和测试源码，本轮未运行测试、构建、CI 或真实 Provider 请求。

### 缓存策略修复

- **个股板块**：此前虽定义 unknown 空响应 5 分钟 TTL，但只写入 `HasData=true`；现在所有通过校验的响应均缓存，确认有数据缓存 12 小时，unknown 空响应缓存 5 分钟。
- **财务快照**：抽取 `fundamentalsCacheTTL` 统一读写策略：有效且有数据 12 小时；有数据但 degraded 为 1 小时；任何 `unknown` 快照（有无指标均如此）为 5 分钟。
- **Tushare 财务与指数日 K 质量隔离**：财务快照仅记录自身公告日期、字段完整性及对应质量原因；指数日 K 缺少源端快照时间的原因只附着于指数日 K 条目/快照，不污染财务结果。
- **指数日 K unknown 候选择优**：回退耗尽前一直尝试后备来源；多个 unknown 快照条目数相同时，优先保留成交量、成交额、涨跌幅和源端时间字段更完整的候选。
- **指数目录/成分/日 K**：valid 目录缓存 12 小时；degraded 缓存 1 小时；unknown 目录缓存 5 分钟。指数成分和指数日 K 对未知质量或无法确认的空结果会继续尝试后备源；多 Provider 场景跳过未记录回退耗尽标记的旧 unknown 缓存。指数日 K 本轮补齐了“有条目但父快照质量为 unknown”的回退，并保留非 valid 的 3 分钟短缓存策略。
- **补充数据缓存与回退**：筹码分布和资金流缓存继续通过共享字段校验；多 Provider 场景下，资金流/筹码的旧 unknown 缓存若未记录回退耗尽原因会被忽略并重新回源。资金流本轮补齐 unknown 指标快照的后备尝试、耗尽后短缓存和同名 Provider 去重；本轮只做静态检查，未运行测试或真实请求。

### 测试源码与待验收事项

本轮为以上约束补充或调整了测试源码，但**均未执行**。最新提交使用 `[skip ci]`，未触发新的 GitHub Actions。后续仍需由统一验收确认测试/构建结果，以及目标部署环境中的上游连通性、数据单位和真实字段完整性；静态校验不代表生产质量已验收。

完整设计见 [Go 行情数据服务迁移方案](./GO_MARKET_DATA_MIGRATION_PLAN.md)，数据源扩展顺序与真实验收门槛见 [数据源扩展计划](./DATA_SOURCE_EXPANSION_PLAN.md)，真实 NAS 取数、故障回退与 Eastmoney 网络故障证据见 [数据源真实环境验收与故障排查记录](./DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md)，Review 与警示见 [架构 Review 记录与工程规范](./ARCHITECTURE_REVIEW_AND_ENGINEERING_RULES.md)，数据域明细见 [Python Provider 到 Go 迁移矩阵](./PYTHON_PROVIDER_GO_MIGRATION_MATRIX.md)。

## 1. 当前已实现

### 主 API 与服务边界

- 根目录 `cmd/analysis-api` 通过 `internal/adapters/marketdata` 调用原生 Go market-data API。
- 实时行情使用 `GET /api/v1/quotes/{symbol}`；日 K 使用 `GET /api/v1/bars/{symbol}/daily`。这两类数据不再静默回退到旧 Python API。
- Go 服务在不同 Go module 之间通过 HTTP JSON 契约连接，未跨 module 导入对方的 `internal` 包。
- `internal/adapters/pythonapi` 保留给旧 smoke test、对照和兼容测试，但不是根主 API 的行情 fallback。
- `go-core` 的新闻检索已增加 Go 原生 Bocha/SearXNG 与个股公告候选源，但仍保留 Python bridge 兼容回退；LLM、部分非中国市场概况及其他辅助能力仍未全部迁移，不能直接删除 bridge。
- `services/market-data` 已支持以 `MARKET_DATA_DB_HOST/PORT/USER/PASSWORD/NAME/CHARSET/TIMEZONE` 拆分配置并在 Go 代码中生成 MySQL DSN；根 `.env.example` 不再要求手工填写 `MARKET_DATA_MYSQL_DSN`。旧 DSN 仅在全部拆分字段都未配置时临时兼容；分析 API 继续独立使用 `DB_*` / `REDIS_*`。
- `MARKET_DATA_ADJUSTMENT` 作为行情分析与行情服务共用的默认复权口径；仅当需要为行情服务单独覆盖时，才配置 `MARKET_DATA_DEFAULT_ADJUSTMENT`。

### 原生行情 Provider

| 能力 | Provider | 状态 |
|---|---|---|
| 实时行情 | 东方财富、腾讯、新浪、Yahoo Finance（可选）、网易财经（候选） | Yahoo 新增 Chart 1m 实时快照，缺成交额固定 degraded、默认关闭；网易报价在 GitHub Runner 上出现 DNS 解析失败；当前配置仍将网易作为启用的后置回退，生产稳定性未验收 |
| 日线 K 线 | 东方财富、BaoStock、腾讯、新浪（仅 none）、Yahoo Finance（可选）、网易财经 CSV（可选） | BaoStock 已进入默认日 K 回退链；新浪只支持不复权且缺成交额；Yahoo 已有真实返回、缺成交额固定 degraded；网易财经 CSV 只支持不复权、仅在成交额/成交量与价格区间一致时保留金额；当前配置启用为后置回退，上游稳定性仍未验收 |
| 筹码分布 | 东方财富 Go 原生 CYQ；妙想 MX_API 可选后备 | 东方财富筹码能力默认启用，返回估算快照并保持 degraded；MX 仅在显式启用时作为兼容后备 |
| 个股主力资金流 | 东方财富公开日资金流端点；妙想 MX_API 可选后备 | 已接入独立 CapitalFlowProvider；按 f51 日期与 f52 主力净流入（人民币元）计算 5/10 日完整窗口；接口地址可配置 |

补充数据接口：

- `GET /api/v1/stocks/{symbol}/chip-distribution`：东方财富 CYQ 估算优先，MX 可作为兼容后备。
- `GET /api/v1/stocks/{symbol}/capital-flow`：东方财富公开资金流优先，MX 作为显式启用的可选后备。

东方财富个股资金流默认独立启用，可通过 `MARKET_DATA_EASTMONEY_CAPITAL_FLOW_ENABLED=false` 关闭，并可使用 `MARKET_DATA_EASTMONEY_CAPITAL_FLOW_URL` 覆盖端点；妙想仍需显式设置 `MARKET_DATA_MX_ENABLED=true` 和 `MARKET_DATA_MX_API_KEY`（兼容原变量 `MX_APIKEY`），作为后备补充源。两类数据都不能证明精确 publication/available_at，必须保留 degraded 质量状态；不得用于严格的历史时点分析。

### 日志与任务事件

- 根分析 API 增加 JSON slog、X-Request-ID 传播、HTTP 状态码/耗时/响应字节记录。
- 根 API 记录安全的任务输入/输出摘要；Runner 每个 stage 记录开始、完成/失败和耗时日志。
- market-data HTTP 层记录 request ID、path、method、状态码、耗时与响应字节数；Provider 失败新增结构化日志，区分超时、网络连接失败、连接关闭、HTTP 状态、数据契约等错误类别，并清理日志消息中的 URL 查询串；MX 请求记录数据集、标的、状态和耗时，不记录 key 或查询正文。
- MariaDB stage event 已有持久化与查询接口；异步任务 API 已提供 `task_id`、状态查询和阶段事件轮询，SSE 与前端实时任务 UI 尚未实现。详见 [日志与任务步骤进度方案](./LOGGING_AND_TASK_PROGRESS_PLAN.md)。

## 2. 明确尚未完成

- 原 Python Provider 的完整替换与字段级等价尚未完成：Efinance、AkShare 等仍有 Python 聚合逻辑或本地算法需要逐函数对照；Pytdx、Tushare、TickFlow 已有部分 Go adapter，但权限、优先级、单位、日期覆盖和失败语义仍需确认；Longbridge、Futu、Finnhub、Alpha Vantage 等海外/账户型来源按需求暂缓。Yahoo Finance Chart API 提供可选日 K 与实时快照适配器（均默认关闭、缺成交额固定 degraded）；网易财经 Quote/CSV 已有独立 Go adapter，但实时行情与历史 CSV 必须分别验收。网易历史 CSV 曾返回 502，实时 Quote 在 GitHub Runner 上 DNS 解析失败；当前 `.env.example` 与 `configs/providers.json` 仍将二者默认启用为后置回退，生产稳定性未验收前可分别设置 `MARKET_DATA_NETEASE_QUOTE_ENABLED=false` 与 `MARKET_DATA_NETEASE_DAILY_BARS_ENABLED=false` 关闭。
- 多个国内数据域已有 Go 原生适配器，但覆盖与质量不等于完整迁移：核心指数快照、指数目录/当前成分/指数日 K（成分来源返回/排除条数已显式暴露）、市场广度、行业/概念排行、涨跌停池、财务/估值、龙虎榜、个股公告和新闻候选源已部分接入。Tushare 已新增按日期读取“不晚于请求日的最近权重报告”能力，但仍缺成员纳入/剔除的真实生效区间与历史版本归档；另外仍缺严格时点财报/公司行动因子、权威交易日历驱动的盘中和节假日判断、排行榜与涨跌停池的全量覆盖审计（池子 total 大于 items 时服务层会降级并标记截断原因）、资金流/筹码精确可用时间，以及旧 Python 接口字段级对账；详见迁移矩阵和数据源扩展计划。
- `go-core` 的新闻链路现有原生候选源并保留 bridge 回退；非中国市场概况、LLM、部分持久化及通知仍有 Python bridge 依赖。
- 同花顺 Go Provider 已按能力接入证券搜索、行情/日 K、指数目录/当前成分/指数历史 K、核心指数快照、滚动交易日历、财务指标与三张报表、批量估值、涨跌停池、龙虎榜、人气/飙升榜；各能力默认关闭或需显式开关且要求有效 API Key。历史成分生效区间（当前 as_of 报告日查询仅由实现专用接口的 Provider 提供）、部分行业历史指标、字段级 1:1 Python 等价与账号权限/限流尚未确认。不得把“代码已接入”解读为生产权限和质量已验收；参考接口仓库 [Kasign/Financial-API](https://github.com/Kasign/Financial-API)。
- 已实现 MariaDB `analysis_tasks`、有界 Go Worker、`POST /api/v1/analysis/tasks` 的 202 + task_id、任务状态查询和 `after_sequence` 阶段事件轮询；SSE、取消、自动重试与前端状态展示仍未完成。重启时遗留的 running 任务会显式标记为 `worker_interrupted`，不自动重跑。
- 已完成一部分真实 Provider 请求及 MariaDB/Redis 链路验证，但目标环境的完整生产回归仍未完成；CI 绿色不能替代真实网络和端到端验收。


### 数据源扩展评估（2026-10-22）

- BaoStock 原生 Go TCP 客户端与 DailyBarProvider 已实现，包含登录、CRC 校验、压缩帧、分页和单位/复权标记；当前默认启用作日 K 备用源，可通过 MARKET_DATA_BAOSTOCK_ENABLED=false 显式关闭。2026-10-22 的真实 NAS 测试与 2026-10-23 的 GitHub Runner 探测均取得有效日 K 样本；这不代表所有日期、证券或目标容器都已完成验收。完整证据见[真实环境验收记录](./DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md)。
- BaoStock 首期定位为历史日 K 备用源，后续按独立能力评估部分财务指标、复权因子、除权除息、股票基础信息、行业分类及指数成分；不把它当作实时行情、新闻公告、龙虎榜、市场资金流的全能来源。
- 接入采用 Go 原生 TCP 客户端，不在生产采集路径引入 Python 运行时。本轮本地 TCP 集成回归覆盖分页/空末页、上游错误码、断线后重新登录及登出；Provider 拒绝重复交易日和非法 OHLC。GitHub Runner 已连续多轮成功登录并查询，仍需在目标 NAS/容器继续扩大日期、证券及故障恢复样本。
- 官方文档指出其前复权算法可能与同花顺、通达信等不同。数据模型和验收计划必须记录复权算法与来源，不能把同名的前复权序列自动视为数值等价。
- Eastmoney 故障于 2026-10-22 进一步复现：容器可访问百度 HTTPS；两个 Eastmoney 域名根路径可建立 HTTPS 并返回 404，但真实历史 K 线接口在连接部分解析到的 IP 时出现 Network unreachable。根因未确定，不能断言运营商封锁或域名整体被封。详情见[真实环境验收记录](./DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md)。
- Tencent 已作为 BaoStock 故障回退源成功返回 600036.SH 日 K 样本，但成交额缺失被标记 degraded；仍需独立单源验收。未复权数据不得作为前复权兜底。

详细验收步骤、能力分工、候选数据源成本/权限条件见 [数据源扩展计划](./DATA_SOURCE_EXPANSION_PLAN.md)。

### BaoStock Go 实现阶段（2026-10-22）

- 已新增独立 internal/baostock 协议包与 provider.Baostock；协议包负责会话串行化、TCP 帧、CRC32、96 类型 zlib 响应、分页和上下文 deadline。
- Provider 只实现 DailyBarProvider。成交量标准单位为股、成交额标准单位为人民币元；成交额缺失时保持 null/unknown/degraded，不估算成交额。
- 新增 adjustment_method=baostock_pct_change，标明其复权算法，避免只靠 forward_adjusted 标签就认定与其他来源数值完全相等。
- Provider 已注册为默认日 K 备用源；目标容器需要继续完成真实登录、查询和逐日口径验收，如遇部署网络或复权问题可通过环境变量关闭。

## 3. 构建与自动化测试状态

BaoStock 第一轮代码与配置修复在 [CI 运行 39754783078](https://github.com/Kasign/stock_data_provider/actions/runs/39754783078) 全部通过，包括根模块、go-core、services/market-data、Compose 校验和 Docker 镜像构建。该结果只覆盖模拟服务与构建，不包含真实 BaoStock 登录查询或 NAS 网络验收。

GitHub Actions 运行 [39583673094（run #84）](https://github.com/Kasign/stock_data_provider/actions/runs/39583673094) 已在根 module、`go-core` 和 `services/market-data` 三个模块中分别通过 `go test ./...`、`go test -race ./...` 和 `go vet ./...`。该运行验证了最新 Go 源码，包括 MX 配置与接口、请求 ID/JSON 日志和中文注释调整。

异步任务模块加入后，GitHub Actions 运行 [39586848833（run #120）](https://github.com/Kasign/stock_data_provider/actions/runs/39586848833) 再次在三个 Go module 中通过 `go test ./...`、`go test -race ./...` 和 `go vet ./...`，覆盖了任务队列、HTTP 状态/事件轮询及 panic 恢复代码。配置拆分与 `gofmt` 检查调整后，运行 [39711711489（run #140）](https://github.com/Kasign/stock_data_provider/actions/runs/39711711489) 在三个 Go module 中通过检查；随后复权默认值共享逻辑的回归测试也在 [39712383338（run #147）](https://github.com/Kasign/stock_data_provider/actions/runs/39712383338) 通过。两个运行均覆盖格式检查、依赖下载、测试、竞态检测和 `go vet`。CI 不执行真实 MariaDB SQL migration 或供应商端到端测试。

后续变更按功能范围分组提交；已经创建的提交保持原样，不重写历史、不 force-push。**CI 通过不代表真实 MX Key/上游网络、东方财富/腾讯/新浪生产数据、目标服务器 MariaDB/Redis 或 Docker Compose 已完成端到端验收。**

目标验收命令：

```bash
# 分别在仓库根目录、go-core、services/market-data 执行
go test ./...
go test -race ./...
go vet ./...
```

此外还需要在真实部署环境测试：

1. Go API 访问 Go market-data（API Key、代码规范化、超时和错误状态）。
2. 东方财富、腾讯、新浪真实供应商接口的响应解析、单位、时间和限流。
3. MX 关闭时明确返回 Provider 不可用；启用有效 Key 后验证真实 payload schema、比例/金额单位与空值。
4. 已有 MariaDB/Redis 的读写、migration、cache freshness、健康检查和容器依赖顺序。
5. 从外部请求到 run_id、所有 stage events、最终结果/失败结果的完整关联。

### 行情 Provider 契约与最新自动化验收（2026-10-23）

- [`91eff930`](https://github.com/Kasign/stock_data_provider/commit/91eff930fcfee919f3707658f697dedba8156369)：新浪、腾讯、东方财富报价增加统一质量契约校验；腾讯与东方财富日 K 共用范围校验、去重、排序、坏行降级和 limit 截断；东方财富重定向只跟随 HTTPS 官方域名。
- [`f5725e1a`](https://github.com/Kasign/stock_data_provider/commit/f5725e1a606ee2f00aaa434dc63e5b28f76c9031)：将原腾讯前复权测试夹具最低价改为 43.00，修正收盘价 43.05 低于最低价 43.10 的错误样本。生产数据质量门禁没有放宽。
- [`3e94fdd3`](https://github.com/Kasign/stock_data_provider/commit/3e94fdd353320b8059382604a1afc0c738206520)：BaoStock 在显式起止日期下也执行 limit 上限，并按日期升序返回。最新真实探测中调用上限 8 实际返回 8 根。
- [Go test suite #206](https://github.com/Kasign/stock_data_provider/actions/runs/39885215590) 已完成并成功：根模块、`go-core`、`services/market-data` 的 gofmt、测试、race、vet，以及 Compose 校验和 Docker 镜像构建通过。
- [Live provider probe #15](https://github.com/Kasign/stock_data_provider/actions/runs/39885215595) 该轮探测 job 成功。新浪报价 valid；腾讯报价及两种日 K 返回但 quality degraded（供应商字段/成交额契约限制）；东方财富主报价和延迟报价返回但 quality degraded；东方财富前复权日 K 与 BaoStock 前复权日 K 各返回 8 根 valid。前一轮探测 #14 曾遇到东方财富两个报价入口 HTTP 502，说明端点有波动，不能宣称稳定。
- 报价与日 K 的真实探测是特定时间窗口的网络/数据证据，不取代 NAS 容器端验收。BaoStock 已纳入默认日 K 回退链，后续继续验证停牌、无数据日期和更长历史范围。

- [`619522a6`](https://github.com/Kasign/stock_data_provider/commit/619522a62a69e73b566336e9ec19584be00bb26e)：东方财富报价时间解析兼容 10 位 Unix 秒与 13 位毫秒，并拒绝明显不合理的数字时间值。基于真实 payload 的 `f86=1792719731` 样本，测试确认报价时间会被解析而非误报缺失。
- [Go test suite #208](https://github.com/Kasign/stock_data_provider/actions/runs/39885682985) 已完成并成功：根模块、`go-core`、`services/market-data` 的格式检查、测试、race、vet，以及 Compose 校验和 Docker 镜像构建全部通过。
- [Live provider probe #17](https://github.com/Kasign/stock_data_provider/actions/runs/39885682979) 的 Provider 探测 job 通过：Sina 报价 valid；Tencent 报价及两种日 K 有返回但质量仍 degraded；Eastmoney 主报价经 302→200 重定向后、延迟域报价均为 valid 且已带 quote_time；Eastmoney 日 K 与 BaoStock 日 K 各返回 8 根 valid 数据。

## 4. 下一步顺序

1. 用户恢复测试前的 `.env` 并重新创建 `market-data` 后，确认 BaoStock 正常路由恢复；首轮真实样本已通过，后续继续扩大交易日、停牌/无数据日期和更长历史范围的验收。
2. 独立测试 Tencent 日 K：只启用 Tencent，先验收不复权数据日期覆盖、OHLC、成交量/成交额单位和质量标记，再按真实响应测试复权能力；缺失成交额或未复权价格不得伪装成完整前复权数据。
3. 独立测试新浪报价与日 K：日 K 当前只支持 `adjustment=none`，且 amount unknown/degraded；目标 NAS/Docker 网络和服务实际路由仍需验收。
4. 持续跟踪 Eastmoney 历史行情接口 `Network unreachable`；根因未确定，按[真实环境验收记录](./DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md)保存证据并待后续排查。
5. 接着按[数据源扩展计划](./DATA_SOURCE_EXPANSION_PLAN.md)分域推进财务、基础资料、行业/指数、市场广度、板块、龙虎榜和新闻公告数据域；Tushare/THS 的权限、费用及速率先核实再接入。
6. 完成实际部署版本的 Provider 错误日志验证，以及 MariaDB/Redis 权限、缓存新鲜度、健康检查与 Docker Compose 端到端回归；之后继续实现 SSE 和前端任务进度展示。


### Tushare 国内数据域扩展（2026-10-26，待统一验收）

本轮在现有日 K Provider 基础上，新增 Go 原生 StockBasicProvider、TradingCalendarProvider、FundamentalsProvider 和 ValuationProvider。能力复用现有 Provider 注册、数据服务缓存与契约校验，不引入 Python 运行时依赖。
日 K 响应也会拒绝空字段名和重复列名，避免供应商返回表头变化后发生列索引覆盖和字段错读；对应回归源码已补充，仍待统一执行。
StockBasic 只接受 A 股普通股票代码；未知或缺失的上游市场分类直接拒绝，不再默认归类为“主板股票”，避免将 ETF/基金等非目标证券误报为普通股票。该防护已补回归测试源码，尚待统一执行。

- 证券基础资料使用 stock_basic，输出名称、行业、市场、上市/退市日期与上市状态；不会从该端点臆造 PE/PB、市值或完整公司资料。
- 交易日历使用 trade_cal，要求上游覆盖请求区间内每个自然日；缺日/重复日会返回错误，交由既有回退链继续处理。
- 季度财务指标使用 fina_indicator，只接受匹配股票及报告期的数据，并保留 ann_date 作为公告日期；缺字段的指标组明确降级，不补零。
- 批量估值使用 daily_basic 的 PE/PB/PS 字段，另行保存 provider_report_date。精确快照时间和 PCF TTM 不可由该端点确认，因此保持 degraded，PCF 留空。
- 所有 Tushare 能力维持默认关闭。启用前须配置有效 Token、总开关和对应能力开关，并确认账户积分/接口权限。本轮按需求没有运行测试；代码接入不等同于上游验收或生产可用。


### Tushare 国内事件数据扩展（2026-10-26，待统一验收）

- 涨跌停池基于 limit_list_d，支持 up/down/broken 三类；供应商金额字段未强制解释为人民币元，按 provider_defined 输出，并限制单次请求与返回条数。
- 龙虎榜基于 top_list，按供应商原始分页语义切页并过滤非 A 股行；买卖额、净额等保留在 Provider 专用金额字段，避免在单位未确认时误标为人民币元。
- 个股公告基于 anns_d，输出公告日期、源端发布时间、标题和原始来源 URL；发布时间无法解析时降级，返回条数达到上游单次上限时明确标记潜在截断。
- 三项能力均默认关闭。Tushare 文档标明涨跌停列表与龙虎榜有积分门槛，公告接口需要单独申请权限；本轮未运行测试，也未进行真实接口请求。


### Tushare 国内指数目录与历史行情（2026-10-26，待统一验收）

新增可选 `IndexCatalogProvider`，提供官方指数目录、最近可用的指数成分权重快照和指数日线历史。接口使用 `/api/v1/market/index-catalog?tag=tushare`、`/api/v1/market/indices/{symbol}/constituents` 与 `/api/v1/market/indices/{symbol}/daily?start=YYYY-MM-DD&end=YYYY-MM-DD`；指数代码保留 `.SH/.SZ/.CSI` 后缀，不能与个股 K 线接口混用。

Tushare 指数目录通过 `MARKET_DATA_TUSHARE_ENABLED=true`、`MARKET_DATA_TUSHARE_INDEX_CATALOG_ENABLED=true` 和有效 Token 显式开启。指数成分取最近可用权重日期，可能滞后于当前交易日；日线成交量保留手、成交额按千元转换成人民币元。本轮没有运行测试和真实请求，这些能力仍待统一验收。


### 雪球人气榜回退接入（2026-10-26，待统一验收）

新增可选 Go 原生 `Xueqiu` Provider，仅承载热度/人气排名，不加入个股报价或日 K 回退链。配置 `MARKET_DATA_XUEQIU_ENABLED=true` 与 `MARKET_DATA_XUEQIU_HOT_RANKINGS_ENABLED=true` 后，它才会进入既有榜单 Provider 链；需要登录态时可配置 `MARKET_DATA_XUEQIU_COOKIE`。该 Cookie 是敏感会话信息，不写入日志或仓库。

雪球热门榜可补充人气排名；`surge` 使用全市场涨幅排序做代理，属于近似榜单，不宣称与东方财富 hot-up 数据一致，保持 degraded。本轮未运行测试或真实请求。


### 雪球关注榜能力补充（2026-10-26，待统一验收）

`GET /api/v1/market/hot-stocks?type=followers` 使用现有榜单 Provider 路由，雪球通过 `type=12` 返回关注榜。东方财富和同花顺未实现该榜单类型时会返回能力不支持，由路由继续尝试下一个 Provider。需要启用 Xueqiu 后备；测试和真实上游响应留到统一验收。


### 指数成分报告日、权重与 OpenAPI 契约补齐（2026-10-26）

Tushare `index_weight` 适配现在将选择的最新权重报告日与逐证券权重写入响应（`provider_report_date`、`weight_percent`），避免丢失可用于解释成分快照时点的来源信息。指数目录、成分和历史日线的 OpenAPI schema 已补齐；热度榜 schema 同步加入 `followers` 类型。以上仅为代码/契约补齐，未运行测试或 CI。


### 龙虎榜原始成交金额字段保留（2026-10-26，待统一验收）

Tushare `top_list.l_amount` 现在独立映射为 `provider_deal_value`，与买入、卖出、净额等原始字段一并保留，避免单位尚未确认时错误写入人民币元字段。OpenAPI 同步描述供应商专用金额、比例及排名元数据。尚未运行测试；字段映射待统一验收。


### 静态代码 Review 第 1 轮（2026-10-26）

Review 范围：Tushare 新增数据域、指数契约与 Xueqiu 榜单接线；本轮不运行测试或触发 CI。

- **已修复：龙虎榜成交额字段漏请求。** `FetchDragonTiger` 读取 `l_amount`，但请求字段列表此前未包含该字段。通用列解析器会因此在有龙虎榜记录时返回“缺少字段”错误。现在将 `l_amount` 加入 `top_list` 请求字段列表。Tushare 文档将该字段定义为龙虎榜成交额。
- **已按可查参数调整、仍待统一验收：雪球 surge 请求契约。** 现改为 `type=sh_sz` 与 `order_by=percent`，作为全市场涨幅靠前的代理排序；仍需真实响应确认字段与排序语义。该榜单只能作为近似替代，不能宣称与东方财富飙升榜完全等价。
- **待统一验收：指数代码与上游映射。** 服务层支持 `.TI/.SH/.SZ/.CSI`，但 Tushare 实际返回哪些后缀及各接口接受的代码范围仍需用官方目录/真实响应确认；未确认前不宜从支持列表推断所有后缀都能查询成功。
- 本轮只做静态审阅与契约比对；未编译、未运行单元测试、未触发 GitHub Actions，也未请求真实上游。


### 市场涨跌统计与全市场成交额（2026-10-27，待统一验收）

- `GET /api/v1/market/breadth` 以东方财富 `getTopicZDFenBu` 分布桶提供上涨、下跌、平盘计数；涨停池、跌停池的供应商总数 `tc` 只在各自 `qdate` 与分布报告日期一致时返回。
- 全市场成交额另由东方财富 CLIST 接口分页读取 `f6`（人民币元），通过配置的 `MARKET_DATA_EASTMONEY_SECTOR_RANKING_URL` 指向同一 CLIST 地址。仅当分页总数稳定、每页行数完整、证券标识有效且无重复、每行报价日期与分布报告日一致，并且成交额覆盖数与分布计数一致时，才填写 `total_amount_cny`；否则金额保持 null、覆盖数为 0，并附带质量原因。
- 成功聚合仍会标记 `market_amount_quotes_collected_non_atomically`：分批请求不是同一时刻的原子快照；此外，分布计数没有经独立证券主数据核验，供应商也未提供可验证的盘中快照时间，因此整体质量仍为 degraded。
- 广度结果缓存使用 5 分钟 TTL，降低分页访问造成的请求放大。新增/调整了本地 CLIST fixture 与质量契约断言。本轮未运行编译、测试、构建、CI 或真实 Provider 请求，后续按统一验收计划执行。


### Tushare 指数成分按月查询（2026-10-26，待统一验收）

Review 发现旧实现将最近四个月的 `index_weight` 记录放在单次请求中，再从响应中提取最新日期。官方接口说明该数据按月提供，并建议按当月第一天至最后一天查询；跨月合并请求会增加触及上游返回限制、最新快照不完整且难以识别的风险。

现已改为从当前月份开始，最多逐月向前查询四个月；每次只请求一个自然月，并选取该月最新报告日期的记录，找到第一个有成分的月份后停止。单月返回量达到保守阈值时增加降级原因；仍需统一验收真实接口的返回完整度。此次没有运行测试、编译或 CI。


### Tushare 估值字段语义 Review（2026-10-26，待统一验收）

静态核对 Tushare `daily_basic` 官方字段说明后，发现 `pe` 定义为市值/净利润，`pe_ttm` 定义为市值/近 12 个月净利润；官方字段没有提供与公共模型 `pe_mrq` 完全等价的“最近季度”PE。此前把 `pe` 映射为 `pe_mrq` 存在语义误标风险。现新增可空 `pe_static`，将原始 `pe` 保留在该字段，并让 `pe_mrq` 保持 null；质量原因明确记录该源不提供 MRQ PE。

本轮仅静态核对和补代码/测试用例，没有请求真实上游、编译或运行测试。


### 财务指标的历史时点边界（2026-10-26，待规划增强）

`FetchFundamentals` 对指定报告期会选择供应商当前返回的最新公告版本，并暴露该版本的 `publication_date`。这能帮助消费者防止使用晚于策略参考时点的数据，但不能重建同一报告期此前发布的旧版本；API 也尚未提供 `as_of` 参数。历史回测应在研究流水线中依据公告日期做 point-in-time 门禁，不能将当前最新修订值直接回灌到早期时点。未来若要支持完整时点重建，需要确认供应商可返回历史修订记录，再设计贯穿 Provider、Service 和 API 的 as-of 契约。


### 指数成分与日线的来源显式选择（2026-10-26）

静态 Review 发现，指数目录支持 `tag=tushare`，但成分/日线原先只按代码走全局 Provider 优先级。启用同花顺与 Tushare 时，重叠的 `.SH/.SZ` 代码可能被另一个 Provider 优先处理，返回错误来源或错误代码语义。现为 `/constituents` 和 `/daily` 增加可选 `source=hithink|tushare`，建议调用方沿用目录响应中的 `source` 值；指定来源时只请求该 Provider，不跨来源回退。未指定时继续自动回退。缓存键升级并包含来源，避免同代码跨来源复用缓存。


## 4. 2026-10-26 静态 Review 补充：Provider 优先级校验

- 发现配置校验遗漏：内置 `provider_priority` 已包含 `xueqiu`，但优先级列表的白名单校验没有包含该名称。由于配置加载会先校验整个优先级列表，即使雪球功能默认关闭，也会在 Provider 装配之前报 `unknown provider "xueqiu" in priority list`，导致 market-data 服务启动失败。
- 已将 Provider 名称支持判断集中到 `isSupportedProviderName`，让供应商配置对象校验和优先级列表校验共用同一份允许名单，避免两处名单继续漂移。雪球仍保持默认关闭；允许它出现在优先级列表不代表会启用其能力。
- 已新增 `TestLoadAcceptsXueqiuInProviderPriority` 回归测试源码，覆盖“优先级含雪球、雪球能力关闭时，配置仍可加载”的场景。
- 本次仅做代码与配置静态审查，没有运行 Go 测试、构建、GitHub Actions 或实时 Provider 请求。该修复尚需用户后续统一回归确认。


## 5. 2026-10-26 静态 Review 补充：Provider 能力开关校验

- 原有布尔环境变量校验需要手工维护完整名单。部分能力开关（例如 `MARKET_DATA_BAOSTOCK_FUNDAMENTALS_ENABLED`、`MARKET_DATA_BAOSTOCK_TRADING_CALENDAR_ENABLED`）未被覆盖，填入 `sometimes` 等非法值时，`boolEnv` 会静默保留 JSON/代码默认值，导致部署人员以为配置已生效。
- 现在按支持的 Provider 名称与能力后缀统一校验所有 Provider 能力布尔环境变量；原有全局布尔项校验仍保留。未来添加能力开关时，即使未单独补入静态校验清单，也会被该规则覆盖。
- 已新增 `TestLoadRejectsInvalidBooleanForProviderCapability` 回归测试源码，覆盖 BaoStock 财务指标开关的非法值。测试尚未执行，留待统一回归。
- 本次修改只改配置校验、回归测试源码和文档，没有运行测试、构建、CI 或真实数据请求。


## 6. 2026-10-26 静态 Review 补充：部分日 K 回退择优

- 当所有 Provider 都无法达到最低 K 线覆盖阈值时，服务会尝试保留质量降级的最佳部分序列。旧逻辑只按返回条数选择；若两家 Provider 返回相同条数，优先级更高但最新交易日更早的那组会一直胜出。
- 现在先选经过验证的较长序列；条数相同时再比较最新交易日，优先保留更新的那组；条数和最新交易日都相同时维持现有 Provider 优先级，避免不必要的切换。
- 已新增 `TestPreferPartialBarsChoosesNewestSeriesWhenCountsTie` 回归测试源码，覆盖相同条数的更新/过时序列、较长序列及相同候选保留规则。测试尚未执行。


## 7. 2026-10-26 静态 Review 补充：网易报价端点配置校验

- 配置中已支持 `MARKET_DATA_NETEASE_QUOTE_URL`，但启动时的上游 URL 校验表只覆盖了网易历史 CSV 地址，遗漏了实时报价地址。错误的 `file://` 等协议因此不会在配置加载阶段被拒绝。
- 已把网易报价 URL 纳入统一 `validateEndpointURL` 检查，并新增 `TestLoadRejectsMalformedNeteaseQuoteEndpointURL` 回归测试源码。验证只检查配置形态，不代表目标上游当前可达。
- 本次测试源码没有执行；没有运行 CI、构建或实时 Provider 请求。


## 8. 2026-10-26 静态 Review 补充：默认 Provider 链的超时预算

- 默认配置启用的个股报价 Provider 共 5 个、日 K Provider 共 7 个；按照每次请求 12 秒、每个 Provider 重试 1 次以及 Bundle 先取日 K 再取报价的执行方式，配置加载时计算的顺序预算约为 291.2 秒。原代码默认值和 `.env.example` 的 `MARKET_DATA_REQUEST_TIMEOUT=3m` 低于该预算，导致默认配置会因超时预算不足而拒绝启动。
- 已将 market-data 的默认请求超时和 `.env.example` 统一调整为 `5m`，可覆盖上述静态预算并留有少量额外余量；若增加默认 Provider 数量、提高单源超时或重试次数，仍需重新评估预算。实际环境若显式设置了更短的 `MARKET_DATA_REQUEST_TIMEOUT`，启动校验仍会拒绝不足的配置。
- 已新增 `TestLoadDefaultTimeoutCoversFullDefaultProviderChain` 回归测试源码，覆盖完整启用链的默认预算。测试尚未执行。
- 注意：该预算是 market-data 服务端的保守上限计算，不代表每次正常行情请求都会耗时五分钟。分析 API 的 `ANALYSIS_TIMEOUT` 是独立总任务上限；代码默认与 `.env.example` 已同步到 `5m`，`HTTP_WRITE_TIMEOUT` 示例为 `5m10s`。多标的或多轮数据采集可能超过五分钟，部署方应根据工作负载调整，不能把此默认值视为任何任务规模的时长保证。


### 内置 Provider 默认值与 providers.json 一致性（2026-10-27）

静态对比发现，配置文件存在时与配置文件缺失时的行为有三处差异：BaoStock 的交易日历/财务指标能力在内置默认值中关闭但在 `configs/providers.json` 中开启；Yahoo 日 K 在内置默认值中开启但配置文件明确关闭；网易行情在内置默认值中关闭但配置文件及部署模板开启。现已把默认表抽为 `defaultProviderFile()`，统一以上三项策略，并增加默认值回归测试源码。这样在缺失配置文件的本地运行与常规部署配置下，国内源能力不会因加载路径不同而漂移。未运行测试、构建或 CI。

### 根目录环境变量模板覆盖范围补齐（2026-10-27）

根目录 `.env.example` 用于 Compose 部署，因此已补齐当前国内默认 Provider 的主要能力覆盖项：东方财富市场广度、板块、指数、涨跌停池、所属板块、龙虎榜、公告、基础资料和日 K/行情开关；腾讯行情与日 K 开关；新浪行情开关；以及 BaoStock 交易日历与财务指标开关。服务级模板补入东方财富基础资料开关，并移除重复的新浪日 K 开关。模板值与当前国内默认策略一致，部署时可直接按需改为 `false`。本次未运行测试、构建、CI 或真实 Provider 请求。

### SourceStatus OpenAPI 字段对齐（2026-10-27）

静态比对运行时模型与 OpenAPI 时发现，数据源状态响应已包含 `index_catalog_capable`，但 `SourceStatus` schema 遗漏了该字段。现已补齐契约字段，保持 Provider 能力状态的 API 文档与实际 JSON 一致；未运行测试或构建。

### 腾讯股票搜索端点可配置化（2026-10-27）

腾讯 Smartbox 搜索能力已经独立开关，但旧构造函数将搜索地址固定为代码常量。现新增 `MARKET_DATA_TENCENT_STOCK_SEARCH_URL` 配置并纳入 URL 形态校验；旧 `NewTencentConfigured` 保持兼容，新构造函数接收搜索地址，默认仍使用 Smartbox 地址。配置测试与本地 fixture 测试源码已补充，未运行测试、构建、CI 或真实上游请求。

### 全市场成交额分页聚合补齐（2026-10-27，待统一验收）

- `GET /api/v1/market/breadth` 新增东方财富 CLIST 全市场成交额汇总：按 A 股过滤条件分页读取 `f6`（人民币元），每页最多 100 条，最多 4 个并发请求，单次聚合预算为 60 秒。
- 每页要求返回完整条数且 `data.total` 在分页过程中保持一致；逐条校验证券代码/市场、非负成交额和 `f86` 报价日期；跨页证券重复、分页缺页、日期错位、全市场证券数与涨跌分布覆盖数不一致时均不输出总额。
- 全量成功且两个数据域的证券数一致后才写入 `total_amount_cny` / `amount_coverage_count`；多页抓取非原子快照、源端分布总体未由独立证券主数据核验，质量继续标记 `degraded`。
- 市场广度缓存从 90 秒调整为 5 分钟，避免每次缓存过期都重新扫描整个证券列表。新增分页聚合与报价日期门禁测试源码；按当前安排未执行测试、构建、CI 或真实上游请求，仍需后续统一回归。


### 东方财富人气榜与飙升榜端点配置化（2026-10-27）

- 静态审查发现，东方财富 Provider 的人气榜与飙升榜仍直接引用两个包级 URL 常量，配置层没有独立的端点覆盖入口；这与同一 Provider 的行情、日 K、板块、指数、龙虎榜和公告端点的配置方式不一致。
- 新增 `MARKET_DATA_EASTMONEY_HOT_RANKINGS_URL`（人气榜）与 `MARKET_DATA_EASTMONEY_HOT_SURGE_URL`（飙升榜），采用原公开端点作为默认值，纳入统一的绝对 HTTP(S) URL 形态校验，并在启动装配时传入 Provider。
- 旧的 `NewEastmoneyConfiguredWithAnnouncements` 等构造入口保留默认端点；新增的 `WithHotRankingURLs` 只覆盖非空配置值，因而不改变旧调用者行为。
- 已补充配置覆盖、非法协议拒绝及旧构造函数兼容的测试源码，并同步根目录与服务级环境模板、服务 README 和迁移矩阵。未执行测试、构建、CI 或真实上游请求；榜单没有可信的源端快照时间，响应仍应保持 `degraded`。


### 服务级环境模板请求超时同步（2026-10-27）

- 复核发现根目录 `.env.example` 和 Go 配置默认值已采用 `MARKET_DATA_REQUEST_TIMEOUT=5m`，但 `services/market-data/.env.example` 仍保留旧的 `3m`。该值低于默认 Provider 链的保守重试/回退预算，独立使用服务级模板可能触发启动阶段的超时预算校验失败。
- 已将服务级模板同步为 `5m`，并注释预算缘由。该修复只改环境示例与状态文档；未运行测试、构建、CI 或真实 Provider 请求。


### 东方财富国内补充能力默认状态与迁移矩阵对齐（2026-10-27）

- 当前 `defaultProviderFile()`、`services/market-data/configs/providers.json` 与服务级环境模板中，东方财富的指数报价、市场广度、行业/概念排名、涨跌停池、个股所属板块、龙虎榜、公告、筹码分布、个股资金流、人气榜/飙升榜均配置为默认启用，并可用各自独立能力开关关闭。
- 迁移矩阵先前部分条目仍写“默认关闭”或“未实现”，与当前代码/配置不符；现已按当前分支更新实现、开关默认值和验收限制。此前 Probe #100 报告内关于“当时默认关闭”的句子保留为历史记录，不再作为当前默认值的说明。
- 这些能力仍有真实部署网络、完整分页/交易日覆盖或源端快照时间方面的未验收项；默认启用仅描述配置行为，不得将 `degraded` 结果用于声称生产质量已达标。
- 本次仅修正文档，不运行测试、构建、CI 或真实 Provider 请求。


### 新浪报价时间语义与网易成交量单位质量修复（2026-10-27）

- 新浪报价解析了源端日期与时间并转换为 UTC，但标准模型此前遗漏 `quote_time_semantics`；现补为 `provider_snapshot_time`，并增加对应测试源码断言，避免下游将其与采集时间混淆。
- 网易历史日 K 的成交量单位校验会用成交金额与 OHLC 均价交叉验证“股/手”解释。此前若两种解释都不成立，虽然质量状态降级且成交额置空，仍把未转换的原始成交量写入标准 `volume`、强制标注为 `shares`。现将单位校验返回类型改为可空指针：明确为股时使用原值，明确推断为手时换算为股，单位无法判定时返回空 `volume` 和 `volume_unit=unknown`，由统一日 K 契约校验器剔除该行，不再伪造标准化单位。
- 已更新网易单位交叉校验测试源码并新增本地 HTTP fixture 测试，覆盖单位歧义时不得形成标准化股数；同步修正服务 README 中网易能力的默认状态描述，使其与 `providers.json` 及环境模板一致，并明确当前 DNS/502 稳定性仍未验收。未运行测试、构建、CI 或真实 Provider 请求。


### 腾讯行情时间解析的时区数据库回退（2026-10-27）

- 腾讯行情解析器已优先使用 IANA `Asia/Shanghai`，无法加载 zoneinfo 时回退到固定 UTC+8；本次将时区选择抽成可注入 helper，并新增“zoneinfo 不可用”与“本地行情时间转 UTC”测试源码。这样可在不改动系统时区数据库的情况下覆盖极简容器回退路径。
- 本次未运行测试、构建、CI 或真实 Provider 请求。


### 普通个股报价 OpenAPI 时间语义字段对齐（2026-10-27）

- 对照 `internal/model.Quote` 与 OpenAPI `Quote` schema 时发现，Go 模型已输出可选的 `quote_time_semantics`，但普通个股报价 schema 遗漏该字段；指数报价 schema 已包含同名字段。
- 已在 `services/market-data/openapi.yaml` 为个股 `Quote` 补入 `quote_time_semantics` 描述，明确这是供应商 `quote_time` 的语义，而不是采集时间。该字段保持可选，不改变已有 JSON 响应行为。
- 本次仅更新契约文档和状态记录；未运行测试、构建、CI 或真实 Provider 请求。


### 根 Compose 环境模板补齐国内上游 URL 覆盖项（2026-10-27）

- 静态检查 `docker-compose.yml` 后确认 `stock-analysis-data` 的 `env_file` 是仓库根目录 `.env`；`services/market-data/.env.example` 只是服务级手工部署模板，根 Compose 不会自动加载它。
- 根 `.env.example` 原先缺少多项虽已由 Go 配置读取、并通过启动 URL 形态校验的国内端点覆盖变量：东方财富行情、日 K、基础资料、板块、指数、市场广度、涨跌停池、所属板块、龙虎榜、公告；腾讯行情与日 K；新浪报价；以及雪球可选源配置。现已按 `config.go` 中的默认值补齐根模板，并补入东方财富公共 web token 的可选覆盖项。
- 保留原有默认 endpoint 与能力开关，不更改任何 Provider 的默认启停策略；这次只让 Compose 部署者更容易显式覆盖地址，避免误以为服务级模板会由根 Compose 自动加载。未运行测试、构建、CI 或真实 Provider 请求。


### 国内 Provider 静态 Review 补充（2026-10-27，本轮待统一验收）

- **Tushare StockBasic 证券范围**：格式合法的六位代码不等于 A 股普通股票。现在先按普通 A 股代码范围校验，ETF/基金及不支持的证券在发起上游请求前拒绝；未知或缺失的 `market` 不再默认映射为“主板股票”。
- **Tushare 日 K 响应字段表**：日 K 入口与其他 Tushare 能力采用一致的严格表头规则，拒绝空字段名和重复列名，避免重复列导致索引覆盖及字段错读。
- **估值快照时点**：服务层拒绝无效或晚于中国市场当前日期的 `provider_report_date`，拒绝零值或超过当前时间 60 秒的 `provider_timestamp`；缺少报告日期/精确时间的估值不能标记为 valid。
- 已新增对应回归测试源码。以上变更均只做了静态源码核对，**没有执行测试、构建、CI 或真实 Provider 请求**；仍需后续统一回归。


### Go 原生新闻检索质量策略增强（2026-10-27，本轮待统一验收）

- **可信来源排序**：按文章 URL 的实际 hostname 识别监管部门、交易所与信息披露网站，并为预置的可信财经媒体域名提供次级加分；不会仅相信搜索接口返回的 `siteName` 标签，也不会把可信域名当成文章真实的绝对保证。
- **垃圾/营销过滤**：默认过滤明确的荐股群、导流和开户链接用语；`GO_NEWS_BLOCKED_TERMS` 可以追加自定义过滤词。过滤词有意保持窄范围，避免把一般性市场情绪词误判成垃圾内容。
- **排序增强**：在股票代码/名称匹配基础上，增加监管、政策、指数、成交额与上市事件等市场主题词权重，标题命中高于摘要命中；原有新鲜度过滤、URL/标题去重和 Python 兼容 fallback 保持不变。
- **时间质量门禁**：拒绝晚于当前时间的发布时间，避免未来时间戳污染新闻排序与“最新消息”上下文。
- 配置示例已同步至根目录与 `go-core/.env.example`，技术说明同步至 `go-core/README.md`。本轮只新增/更新了回归测试源码，未执行测试、构建、CI 或真实新闻源请求；仍需统一验收。


### 中国市场概况转为 Go 原生聚合（2026-10-27，本轮待统一验收）

- `go-core/internal/adapters/marketdatahttp.Client` 新增原生市场概况聚合：并发请求指数快照、市场广度、行业排名和概念排名四个 Go API，减少 Python bridge 正常路径依赖与串行等待。
- 指数快照映射到统一 `MarketOverview.Indices`；上涨/下跌/平盘、涨跌停数量与人民币成交额映射到统一模型；行业/概念排名保留来源及质量提示。
- 对缺失成交额/涨跌停计数、部分市场覆盖、异常/过时的采集时间、非 valid 快照与接口失败保留明确 warnings。当前公开接口没有独立的板块跌幅榜，不伪造 `BottomSectors` / `BottomConcepts`，并输出对应限制提示。
- 原生路径只服务中国市场 live snapshot，拒绝把历史 `as_of` 伪装成实时数据。香港、美股、日股、韩股概况继续走 bridge。中国市场的原生组件不可用或结果无效时回退到 bridge，并在 warnings 标记；个股行情和日 K 仍不回退到 Python。
- 新增原生聚合和兼容回退回归测试源码。本轮未执行测试、构建、CI 或真实上游请求，仍待统一验收。


### Go-core 个股辅助数据并发迁移（2026-10-27，本轮待统一验收）

- `go-core/internal/adapters/marketdatahttp.Client` 在行情/日 K bundle 获取成功后，并发请求原生基本面、所属板块、筹码分布、资金流、龙虎榜摘要与个股 profile，并汇总为 `StockBundle`/分析上下文；辅助接口失败只增加 warning，不把可用行情 bundle 整体判失败。旧 Python `get_enhanced_data` 的严格字段/空值相等性仍需要按矩阵逐项对账。
- 基本面默认请求最近一个已结束季度，保留分区、公告日期、指标单位和来源质量信息；以不重复键兼容旧分析器，并使用分区前缀保留同名指标。
- 所属板块仅在 Provider 明确返回 `has_data=true` 时作为有效列表；筹码区间、资金流单位和质量信息保持原义，不将区间误作中位成本或将 null 变为零。
- 资金流新增到 Go `StockBundle`、`AnalysisContext` 及 Python 分析器桥接上下文。显式 `as_of` 请求不会附加仅支持当前快照的辅助数据；严格历史时点财报版本和辅助快照历史仍待实现。
- 已补对应客户端回归测试源码。本轮未执行测试、构建、CI 或真实上游请求，仍待统一验收。


- **报告期选择补充**：默认基本面查询改为以中国市场披露截止日判断最近可期待的报告期，避免三季报/年报披露窗口内普遍请求尚未披露的季度。对应回归测试源码已补，未执行测试、构建、CI 或真实上游请求。


### Go-core 个股公告并入原生新闻检索（2026-10-27，本轮待统一验收）

- `go-core/internal/adapters/marketdatahttp` 新增公告分页 DTO 与 `FetchStockAnnouncements`，校验请求日期/股票身份、分页计数、公告日期范围、源 URL、来源和质量状态；保持 `announcement_date`、`published_at`、`fetched_at` 的语义独立。
- 个股分析使用 market-data 的按股票公告接口，并发获取当前公告窗口，与 Bocha/SearXNG 候选按 URL/标题去重；公告候选按股票关联加权。即使未配置网页搜索服务，只要公告接口可用也可返回个股公告。
- 公告请求使用独立的短超时；单项不可用不会阻断核心分析和其他新闻来源。未提供源端精确发布时间的公告使用 `announcement_date` 作日期级过滤，不使用 `fetched_at` 伪造发布时间。
- 已增加客户端契约、公告独立来源以及与网页搜索合并去重的回归测试源码。
- 按约定，本轮未运行测试、格式化、构建、CI 或真实上游请求；待后续统一验收。


### Go 原生新闻多源聚合第二阶段（2026-10-27，本轮待统一验收）

- 将 Tavily News 与 Brave News 加入既有 Go 原生 Provider 接口；只有配置 API Key 才注册，不自动请求或启用任何公开 SearXNG 实例。
- Bocha、Tavily、Brave News、显式 SearXNG 均由统一调度器并发调用，失败来源单独记录，其他来源仍可提供结果；对所有候选统一执行发布时间门禁、可信域名加权、营销过滤、规范 URL/标题去重和个股相关性排序。
- Tavily 请求新闻 topic 并要求源端发布日期；Brave News 定向中国及简体中文检索。Brave 的 page_fetched 不视为发布时间；缺少可解析源端发布时间的结果会被质量过滤。
- API 凭据只从环境变量读取，日志不输出 key。新增 Tavily/Brave 本地 HTTP fixture、跨源聚合和去重测试源码；未运行测试、构建、CI 或真实供应商请求。
- 当前仍保留 Python bridge fallback 作为兼容边界；后续在统一验收与调用覆盖核实后，再评估是否可在部署配置中关闭新闻 fallback。


### Go-core 龙虎榜上下文摘要（2026-10-27，本轮待统一验收）

- `StockBundle.DragonTiger` 与 `AnalysisContext.DragonTiger` 新增个股龙虎榜摘要；从最新 10 个完整交易日读取现有 Go API 并派生 `is_on_list`、`recent_count`、`latest_date`。
- 摘要通过 `dragon_tiger_context` / `dragon_tiger` 传给兼容分析器。unknown/empty snapshots 不会强行转换成 `false`；覆盖不完整时保留 lower-bound 标记。
- 每日最多分页 10 页、每页 500 条，最多 4 个日期请求并发。未运行测试、构建、CI 或真实上游请求。


## 2026-10-27 — 构建阻塞修复与最新 HEAD 验证边界

- Eastmoney JSON struct tag 的非法字符问题已由 `50fecf2` 修复；此前 go-core 与 market-data 的格式问题已整理。多维新闻检索的两个文件由 `d692a0d` 格式化；双 Provider 聚合测试改为显式设置 `MaxSearches=2`，修正直接配置默认只执行一个维度与测试期望不一致的问题。
- `.github/workflows/go-test.yml` 现支持 `workflow_dispatch`，且格式检查输出脚本保留正确的单个 `\\n` 转义。
- 提交 `d551057` 对应的 [Go test suite #40449084361](https://github.com/Kasign/stock_data_provider/actions/runs/40449084361) 通过，根模块、go-core、market-data 的测试/race/vet、Compose 校验和两种 Go 镜像构建全部通过；同一 HEAD 的 [WebUI checks #40449084307](https://github.com/Kasign/stock_data_provider/actions/runs/40449084307) 与 [Live Provider Probe #40449084825](https://github.com/Kasign/stock_data_provider/actions/runs/40449084825) 也通过。
- 随后分支继续前进至 `24952dd` / `71993b1`，新增个股龙虎榜上下文摘要并补充相关交接/进度说明。因此，上述绿色 CI 仅覆盖 `d551057`，不能自动视为新增 `DragonTiger` 上下文代码已验证；最新 HEAD 应再跑 Go 全量工作流确认。


### Tushare 指数日 K 成交量溢出边界（2026-10-27）

- 静态审查发现 `float64(math.MaxInt64)` 会舍入为 `2^63`。原来仅拒绝 `rawVolume > float64(math.MaxInt64)` 会让精确的 `2^63` 通过检查，再转 `int64` 时溢出。
- 现改为在转换前拒绝 `rawVolume >= float64(math.MaxInt64)`；越界量保留为 `volume=null`、`volume_unit=unknown`，该条目标为 degraded 并记录稳定原因。
- 新增 `TestTushareIndexDailyBarsRejectsInt64VolumeOverflow` 测试源码。未执行测试、构建、CI 或真实 Provider 请求。


### 国内 Provider 浮点转整数边界复核（2026-10-27）

- 同花顺指数日 K、个股行情/日 K 的成交量和财报时间戳统一在 `int64` 转换前拒绝 `>= float64(math.MaxInt64)` 的边界；对时间戳同时要求整数毫秒值。
- TickFlow 整数解析增加正负 int64 范围检查，避免浮点输入越界后直接转换；腾讯报价成交量的预防性换算边界也改为包含等号的拒绝条件。
- 新增 `numeric_boundary_test.go`，覆盖同花顺、TickFlow 与腾讯的 `2^63` 边界。仅增加测试源码，未执行测试/构建/CI/真实 Provider 请求。


### 补充数据和财务快照的采集时间语义（2026-10-27）

- 修正筹码分布、个股资金流和财务快照对 Provider 回包的 `fetched_at` 处理：该字段表示本服务收到成功回包的 UTC 时间，由服务层统一盖章，不再直接信任上游同名值。避免未来时间污染响应语义，也避免写入缓存后因 `time.Since(fetched_at) < 0` 而持续缓存未命中。
- 筹码分布 `date` 与 `provider_date` 在完成严格 `YYYY-MM-DD` 格式校验后，还必须不晚于中国市场当前日期；未来日期会拒绝该 Provider 结果，不作为可用快照返回。
- 已补充对应回归测试源码；按协作约定，未运行测试、构建、CI 或真实 Provider 请求。


### 核心行情与日 K 时间字段统一由服务层盖章（2026-10-27）

- 实时行情此前只在 Provider 未填写 `fetched_at` / `normalized_at` 时补默认值；历史日 K 同样沿用 Provider 非零时间。静态审查确认，这会允许错误或未来时间进入 API、MariaDB 与 Redis，并使时间语义依赖具体 Provider 的实现方式。
- 现由行情服务在收到成功响应时统一写入 `fetched_at`，并在完成标准化和质量处理后写入 `normalized_at`。供应商时间仍放在单独的行情/日 K 源时间字段内，不复用服务时间字段。
- 已补对应回归测试源码；按约定未执行测试、构建、CI 或真实 Provider 请求。


### 缓存命中时校验服务时间字段（2026-10-27）

- 继 Provider 回包时间统一由服务层盖章后，补充缓存读取门禁：实时行情与日 K 的缓存条目若 `fetched_at` 或 `normalized_at` 缺失或超前当前时间容差，视为不可信缓存并触发上游刷新。
- 历史 K 线的交易日期虽可以很早，但这不等于服务采集时间可以缺失或位于未来；缓存时间检查独立于交易日期和行情源时间。
- 已添加缓存回归测试源码；未运行测试、构建、CI 或真实 Provider 请求。


### 日 K 日期字符串统一采用严格规范（2026-10-27）

- 修正请求入口和共享日 K 校验器：起止日期、Provider/缓存中的交易日期必须严格符合 `YYYY-MM-DD`，仅能被宽松解析但重新格式化后不相等的字符串不再接受。
- 该约束用于保证后续字典序范围筛选、日期排序与缓存键一致，不依赖单一 Provider 自行规范化数据。
- 已补充日期格式回归测试源码；按约定未执行测试、构建、CI 或真实 Provider 请求。


## 财务报告期查询与历史时点边界

- `GET /api/v1/stocks/{symbol}/fundamentals` 只接受 `year` 与 `quarter` 来定位报告期；当前不支持历史修订版本重建。若调用方传入 `as_of`（包括空值），HTTP 层明确返回 400 `VALIDATION_ERROR`，不静默忽略。
- Provider 当前返回请求时可获得的该报告期版本，并保留公告日期用于下游时点门禁。该行为不代表可回放某个历史日期当时已公开的财务数值；未来若建设完整 PIT 能力，需增加历史修订存储并确认源端能提供版本记录。


### 同花顺财务指标与三张报表服务契约

服务层财务校验允许六个标准指标分区外，再附带 `income_statement`、`balance_sheet`、`cash_flow_statement` 三个可选报表分区。先前的“必须正好六个分区”限制会拒绝同时返回指标与报表的同花顺快照；现允许六个标准分区与最多三个报表分区。若同花顺省略标准指标分区，适配器会补充 `unknown` 空分区并记录具体缺失原因，不补零；重复或未知指标能力会在适配器发起报表请求前被拒绝。相关修正尚未运行测试、构建、真实 Provider 请求或 CI。


### 国内证券搜索与未来财务报告期门禁（2026-10-27）

- 证券搜索路由和回归测试源码已存在，但 OpenAPI 曾遗漏 `GET /api/v1/stocks/search` 路径及响应模型；本轮补齐查询参数、错误状态、`StockSearchItem` / `StockSearchSnapshot` schema，并说明 `fetched_at` 不等于实时行情时间。
- 基本面接口现在会在读取缓存或访问 Provider 前拒绝尚未结束的年度/季度报告期，避免空数据分区绕过未来报告期校验。新增服务层与 HTTP 层测试源码覆盖该门禁。
- 本轮未运行测试、构建、CI 或真实 Provider 请求。


### 证券搜索缓存状态头语义（2026-10-27）

- 搜索服务没有独立缓存命中返回值，旧 HTTP handler 却把所有非 stale 响应都标为 `X-Market-Data-Cache: HIT`，可能将刚执行的上游查询误报为服务缓存命中。
- 已改为仅在快照明确 stale 时返回 `X-Market-Data-Cache: STALE`；普通响应不声明缓存命中，并补充回归测试源码。未运行测试、构建、CI 或真实 Provider 请求。


## 当前剩余任务与加速执行顺序（2026-10-27）

本阶段继续只做国内 A 股数据层；海外数据源、真实 Provider 请求、构建/测试/CI 均留到用户统一验收。每个功能批次合并独立能力相关改动、测试源码和文档，集中使用一个中文提交并附 `[skip ci]`，不为无关小改动拆分提交。

### P1：完成国内数据能力与字段语义差异

1. **按能力域核对 Efinance / AkShare 替代完整度**：以行情、日 K、证券搜索/基础资料、所属板块、指数/成分、市场广度、涨跌停池、热度榜、筹码、资金流和增强数据为清单，对照原 Python 实现和现有 Go 路由。Efinance / AkShare 多数是公开接口的组合包装，不再机械复制同名 Provider；确实缺失的返回字段、组合结果或筛选语义才新增代码。
2. **补齐 Pytdx / Tushare / TickFlow 已接入能力的契约差异**：逐项核对交易日期范围、复权支持、成交量/成交额单位、权限失败、分页截断和质量降级。能通过静态代码与 fixture 证明的问题直接修；账号权限、源端完整性和实时延迟只记录待验收，不以代码存在代表可用。
3. **处置公开国内源稳定性边界**：东方财富间歇性 302/502 已记录在 Issue #6；网易行情/日 K 有既往 DNS/502 失败记录。代码侧优先保证超时、错误分类、回退和质量状态明确，不新增无证据的生产可用性结论；真实可达性留到统一验收，不在每批开发中重复请求。

### P2：历史时点与数据可回放能力

1. **财务 Point-in-Time**：当前财务 API 返回请求时可取得的报告期版本，不还原历史修订。先确认上游是否提供历史版本/真实可用时间，再设计版本表、唯一键、缓存键和 `as_of` 查询；在历史数据不可获得前不伪造 PIT 支持。
2. **复权因子与指数成分历史**：现有前/后复权和最新成分快照并不保证可复原历史当时有效的数据。需按来源记录复权算法、因子版本/生效日和成分有效区间，再决定数据结构与回放接口，不能简单取消质量门禁。

### P3：统一验收（由用户安排）

- 一次性执行 Go tests/race/vet、OpenAPI/Compose 校验、容器构建、目标环境健康检查及国内源 smoke test。开发批次不重复启动这些流程。
- 复核配置默认值与根 `.env.example`、`configs/providers.json`、服务级模板一致；对不稳定/缺凭据的数据源显式显示 disabled、unknown 或 degraded，不将配置启用等同于上游可用。
- 合并前集中审查 PR 历史、文档状态、已知问题 Issue #6 和分支差异，再决定合并；在用户统一验收前保持 PR 草稿状态。

### 本轮已收口

- 同花顺指标缺失时显式补充 `unknown` 分区，不补零；未知/重复能力在发起三张财报请求前拒绝。
- 腾讯行情时区回退路径已有可注入测试源码，覆盖极简容器无 zoneinfo 情况。
- 财务报告期查询拒绝尚未结束的未来季度，避免空分区绕过未来报告期检查。
- 证券搜索 OpenAPI 契约已补齐；普通响应不再错误声明缓存命中。
- 以上改动均未运行测试、构建、CI 或真实 Provider 请求。


### TickFlow 核心指数报价补齐（2026-10-27）

- 原 Python TickFlowFetcher 可使用授权的 `quotes.get(symbols=[...])` 获取 6 个 A 股核心指数；Go 适配器此前仅实现个股 Quote 与日 K，未实现现有 `IndexQuoteProvider` 接口。
- 新增独立 `FetchIndexQuotes` 能力，限定 6 个核心指数，不把未知代码混入结果；字段保留来源报价时间，缺少时间/成交额/OHLC 时显式降级，不伪造源时间或补零。
- 默认关闭，新增 `MARKET_DATA_TICKFLOW_INDEX_QUOTES_ENABLED`。该能力需 TickFlow API Key，与免 Key 日 K 分开配置；配置缺 Key 时会在启动校验阶段失败。
- 已补接口夹具与配置门禁测试源码；按协作约定未运行测试、构建、CI 或真实 Provider 请求。市场广度与 SW1 行业榜已按官方 SDK 暴露的证券池和 universe 行情接口接入 Go 适配器，均默认关闭且需授权 Key；SW1 输出为成分股涨跌幅均值，并非源端行业指数行情。

### 国内 Provider 静态契约复核（2026-10-27）

- Tushare 日线的 `pct_chg` 不是 OHLC 主字段，但缺失时会影响涨跌幅完整性。当前适配器此前仅在字段存在但解析失败时降级，字段缺失或为 `null` 时可能保留 `valid`；本轮将两种情况统一标记 `degraded`，涨跌幅继续保留为空，不补零。
- TickFlow 报价时间按毫秒 Unix 时间戳处理。个股行情和核心指数行情统一拒绝早于 2000-01-01 或晚于当前时间 5 分钟以上的时间戳，且将其标记为缺失/无效；不设置固定“陈旧”阈值，避免将休市/盘后最后有效快照误判。市场广度另行报告逐证券报价时间覆盖数与最早/最新时间；全市场来源时间跨度超过 10 分钟时保持可展示但降级，缺少完整来源时间戳的快照不能标为 valid。
- 仍未完成的国内差异：TickFlow 市场广度已按官方 SDK 的 `GET /v1/universes/CN_Equity_A` 与 `GET /v1/quotes?universes=CN_Equity_A` 契约接入，先校验证券池成员，再要求返回唯一行情覆盖完整；默认关闭，用户 Key 的实时行情/证券池权限与目标环境仍待验收。申万一级行业榜已按 `CN_Equity_SW1_*` 证券池实现，输出为成分股涨跌幅均值，不等同于源端提供的直接行业指数收益；默认关闭，权限与源端覆盖待验收。Tushare / Pytdx / TickFlow 的字段单位、历史覆盖与回退语义仍需完成全量静态对照。历史财务修订、历史成分生效区间和复权因子时点重建依赖更完整的来源数据或存储设计，不能用最新值替代。
- 本轮只增加代码与测试源码，按协作约定未运行测试、构建、CI 或真实 Provider 请求。

### TickFlow 全市场 A 股涨跌统计接入（2026-10-27）

- 根据 TickFlow 官方 SDK 的接口契约，使用 `GET /v1/universes/CN_Equity_A` 读取证券池清单及 `symbol_count`，再调用 `GET /v1/quotes?universes=CN_Equity_A` 读取实时行情；两个请求均通过 API Key 鉴权。
- 统计前校验证券池数量、规范化后的唯一证券成员以及行情覆盖是否完整。缺证券、重复/未知代码、混合报告日期或没有有效来源日期时整体失败并让服务层回退，绝不把局部行情冒充全市场。
- 涨跌/平盘按最新价与昨收价计算；成交额仅在所有证券均有有效金额时汇总，金额覆盖不完整时保留覆盖数并将总额置空。涨跌停数量按原 Python A 股规则计算（北交所 30%、科创/创业板 20%、ST 5%、普通主板 10%），主板证券缺失名称、无法判定 ST 状态时不输出涨跌停总数。
- 新能力由 `MARKET_DATA_TICKFLOW_MARKET_BREADTH_ENABLED` 独立控制，默认关闭；启用时启动配置要求 TickFlow API Key。官方 SDK 契约不代表具体账号已开通对应接口权限。
- 新增隔离的 HTTP fixture 与配置门禁测试源码；按协作约定未运行测试、构建、CI 或真实 Provider 请求。

### Pytdx 保留交易所前缀，避免代码歧义（2026-10-27）

- 通达信 Go 客户端的 `GetQuote`、`GetKlineDay` 和 `GetGbbq` 支持带交易所前缀的代码；先前适配器却把 `600519.SH` 等标识裁剪为纯数字，让底层 `AddPrefix` 再根据代码形态猜测交易所。
- 本轮改为传入 `sh600519` / `sz000001` / `bj920000` 形式，并在实时行情响应中同时匹配代码和交易所；个股 Provider 入口还会拒绝指数代码，避免 `000001.SH` 被当成个股日 K 查询。
- 新增纯单元测试源码覆盖前缀映射、同代码错交易所与指数代码拒绝。按约定未运行测试、构建、CI 或真实 TDX 连接。

### 国内行情单位与证券匹配边界（2026-10-27）

- Pytdx 现在将来源返回的代码和交易所同时纳入实时报价匹配，拒绝同代码但不同交易所的响应；对应隔离单测覆盖沪深市场歧义和缺少 K 线数据。
- Pytdx 日 K 的价格/成交量可以独立保留；当来源成交额与成交量出现“一个为零、另一个为正”的矛盾时，现将归一化成交额置空并降级，而不让标准化层因此丢弃整根 K 线。原始成交额与单位仍保留用于排查。
- Tushare 日线 `vol` 的接口口径为“手”，只有整数手数才允许换算成股数。之前通用转换会对小数手数四舍五入，本轮改为拒绝小数成交量并触发上游回退，防止把格式异常数据悄悄改成看似有效的成交股数。
- 以上均为静态代码修正和测试源码补充；未执行测试、构建、CI 或真实 Provider 请求。

### 公共手单位成交量换算拒绝小数输入（2026-10-27）

- `normalize.HandsToShares` 现在统一要求来源“手”数为非负整数后再换算成股数；拒绝小数手数、非有限值和溢出值。
- 此门禁覆盖按“手”返回成交量的东方财富报价/日 K、腾讯按手口径分支、Pytdx 和 Tushare 等共用转换路径。TickFlow 的 A 股实时行情与日 K 原始成交量单位是“股”，必须使用 `RawSharesToInt64` 直接校验，不能进入手数换算路径。
- Tushare 保留 Provider 级错误上下文，具体原因由公共归一化错误返回；已扩展公共转换测试和 Tushare 数据夹具源码。未运行测试、构建、CI 或真实 Provider 请求。

### TickFlow 成交量单位修正（2026-10-27）

- 原项目同一 TickFlow 数据源的修复记录明确说明：A 股实时行情和日 K 的 `volume` 返回值已经是“股”，不是“手”；错误地乘以 100 会把成交量放大 100 倍。参考原项目修复记录：`https://github.com/ZhuLinsen/daily_stock_analysis/pull/2128`。
- Go 适配器现将 TickFlow 报价与日 K 的成交量按股数直接校验为非负整数，不再调用 `HandsToShares`；`raw_volume_unit` 和归一化 `volume_unit` 均明确为 `shares`。东方财富、腾讯等其他来源仍按各自单位契约转换。
- 将新增 Quote 与日 K HTTP 夹具，验证原始股数与标准化股数保持一致；未发起真实 TickFlow 请求，未运行测试、构建或 CI。

### 公共股单位成交量换算拒绝小数输入（2026-10-27）

- `normalize.RawSharesToInt64` 现在和 `HandsToShares` 一样只接受非负整数股数，不再对小数股数四舍五入。
- 该校验覆盖 TickFlow A 股实时行情/日 K，以及腾讯行情单位推断与科创板日 K 的原始股数路径。遇到小数股数会拒绝该 Provider 结果并进入已有回退，而不是悄悄修改成交量。
- 已补公共转换测试源码；未执行测试、构建、CI 或真实 Provider 请求。

### TickFlow 申万一级行业榜接入（2026-10-27）

- 基于官方 SDK 所暴露的 `GET /v1/universes`、`GET /v1/universes/CN_Equity_A`、`POST /v1/universes/batch` 和 `GET /v1/quotes?universes=CN_Equity_A` 组合实现。
- 只接受 `CN_Equity_SW1_*` 且名称以 `SW1` 开头的证券池；代码从 A 股全集白名单校验，行业详情缺失、重复成员、非 A 股代码、行情漏行或跨报告日时整体失败并回退。
- 行业涨跌幅沿用原 Python TickFlowFetcher 的口径：对该 SW1 证券池内有有效行情的成分股涨跌幅取算术平均。因为这不是源端直接提供的行业指数涨跌幅，且多请求采集并非原子快照，结果固定为 `degraded`。
- 新能力由 `MARKET_DATA_TICKFLOW_SECTOR_RANKINGS_ENABLED` 独立控制，默认关闭；启用需要 API Key。概念板块不由该能力提供，仍使用既有概念来源。
- 已补充 HTTP fixture 与配置门禁测试源码；本轮未运行测试、构建、CI 或真实 Provider 请求。

### Pytdx 成交额辅助声明位置修复（2026-10-27）

- 对最新 Pytdx 成交额质量降级代码进行静态复核时，发现辅助类型与函数声明被插入到 `pytdxDomesticSymbol` 函数体内部，导致 Go 源文件无法解析。
- 已将辅助声明移到文件级，并补齐成交额/成交量组合校验：当一个为零、另一个为正时保留原始金额并将归一化金额置空、标记 degraded；非法金额或非整数手数直接拒绝。
- 新增独立测试源码覆盖正常金额、两类零值矛盾和非法数字边界。未运行测试、构建或 CI。

### TickFlow 市场广度缺失涨跌停统计时的质量语义（2026-10-27）

- 全市场涨跌统计若因缺少主板股票名称而无法判断 ST 涨跌幅限制，或有任一证券无法计算限价，仍可以保留上涨/下跌/平盘和成交额等独立完整字段，但涨跌停计数维持 `null`。
- 当涨跌停数量无法覆盖完整证券池时，快照质量现在同时标记为 `degraded`，并附带 `provider_limit_counts_unavailable_or_incomplete`；不会让未提供的计数看起来像完整结果。
- 对应测试源码扩展了股票名称缺失的独立场景。按协作约定未运行测试、构建、CI 或真实 Provider 请求。

### Tushare 财务修订记录未来公告日期门禁（2026-10-27）

- `fina_indicator` 可能在一个报告期下返回多条修订记录；当前版本选择逻辑现只从报告期结束后、当前上海日期以前已公告的记录中选择公告日期最新的一条。
- 未来公告记录、报告期结束前的异常公告日期及缺失公告日期的记录均不参与版本选择；若没有可证明已公告的候选版本，Provider 返回不可用并交由服务层回退，而不是使用时点未知的财务数值。这不是历史 `as_of` 回放能力，历史修订版本仍未持久化。
- 新增 fixture 测试源码验证未来公告记录排在前面时，仍选择已公开版本。未运行测试、构建、CI 或真实 Provider 请求。
### 国内 Provider 字段口径静态审查与 TickFlow 日 K 边界修正（2026-10-27）

- **TickFlow 日 K：** `timestamp/open/high/low/close/volume` 为必需等长数组；`amount` 与 `prev_close` 可以整体缺省，但只要出现就必须与时间戳等长。长度不匹配现在作为上游契约错误拒绝，不再因数组较短而悄悄把后续金额视为缺失。
- **TickFlow 涨跌幅：** 若整组 `prev_close` 缺失，不再按供应商原始数组顺序回算。先按上海市场交易日期排序，再用前一条时间顺序的收盘价推导；首条无法推导时留 `null`。此回算不是供应商原始涨跌幅，涉及记录一律 `degraded` 并添加质量原因。若供应商提供 `prev_close`，则逐条使用并对无效值降级。
- **TickFlow SW1 行业榜：** 官方 SDK 源码确认 `universes.list/get/batch` 分别映射证券池列表、单池详情和批量详情端点，行情 SDK 也支持按 `CN_Equity_A` 查询；证券池详情字段包括 `id/name/symbol_count/symbols`，报价包含毫秒时间戳与 `ext`。但官方公开 SDK 未列出固定的 `CN_Equity_SW1_*` 分类清单，也未提供专用行业指数涨跌排行接口。因此现有 Go 实现必须继续将 SW1 排名定义为“发现到 SW1 证券池后，按成分股涨跌幅算术平均估算”，固定降级；不能将其称作供应商原生行业指数涨跌幅。具体账号可见证券池、整池行情权限及全量成员覆盖仍待用户验收。参考：[官方 universes SDK](https://github.com/tickflow-org/tickflow/blob/main/tickflow/resources/universes.py)、[官方 quotes SDK](https://github.com/tickflow-org/tickflow/blob/main/tickflow/resources/quotes.py)。
- **Tushare 日 K：** `vol` 是手数，必须为非负整数后换算成股；`amount` 按千元人民币换算成人民币元；日线端点仅支持不复权。缺失/非法涨跌幅保留 `null` 并降级；代码、日期、重复交易日、OHLC 或必需字段违反契约时拒绝该源结果，交由能力路由尝试后续来源。单次请求无供应商总记录数/游标可对账，不能仅凭响应成功宣称请求区间每个交易日均覆盖。
- **Pytdx 日 K：** 每页最多 800 条，扫描上限 40 页；请求起始日期未到达或后复权所需历史未完整获取时返回错误，不把截断历史伪装成完整结果。复权需要除权除息事件查询成功；个股报价没有可用供应商时间戳，所以保持降级。成交量按手转换为股，成交额异常与成交量明显矛盾时保留原始值、将标准化成交额置空并降级。
- 本批只更新实现、测试源码与状态说明，未运行测试、构建、CI 或真实 Provider 请求。以上仍属于静态契约检查，不代表真实接口权限、返回数据完整性或部署稳定性已验收。


## 2026-10-27 个股资金流能力补齐

- 已将东方财富公开 `/api/qt/stock/fflow/kline/get` 接入独立 `CapitalFlowProvider`，不再要求资金流 Provider 同时实现筹码分布。读取最近最多 10 个日观测，`f51` 作为 `provider_date`、`f52` 按人民币元解析；仅当 5/10 日窗口内每一条金额均有效时才返回对应合计。若最新金额、5 日合计、10 日合计全部缺失，服务层拒绝该响应并继续尝试后备源。
- 资金流端点、能力开关与 `providers.json`、根/服务环境样例及 OpenAPI 已同步。东方财富优先，旧的 MX `SupplementProvider` 继续作为后备兼容。
- 本轮只补充源码与契约测试，不执行测试、构建、CI 或真实 Provider 请求。端点访问、字段实际返回、当日未收盘数据语义与全窗口覆盖仍待统一验收；没有把静态接线当作运行验收。


### 涨跌停池未知空响应继续回退（2026-10-27）

- Provider 返回 `has_data=false`、`total=0` 且质量为 `unknown` 时，只表示无法确认当日是否确实没有事件；服务层现在继续尝试后续 Provider，不会将其当作成功结果提前终止回退。
- 若所有 Provider 都没有提供可确认的池数据，但至少一个返回了契约合法的未知空响应，则返回 `quality_unknown` 并增加 `limit_pool_fallback_exhausted_without_confirmed_data`。此结果不缓存，避免下一次请求继续跳过回退链。没有未知空响应、且所有源都报错或违反契约时，仍返回明确错误。
- 已补回归测试源码，覆盖先空后有数据、以及后备耗尽时保留 unknown 的情况；按协作约定未执行测试、构建、CI 或真实 Provider 请求。


### 涨跌停池供应商报告日期与请求日期一致性（2026-10-27）

- 有效的 `provider_report_date` 必须与请求的 `trade_date` 一致；返回其他日期的数据不能被贴上请求日期使用，服务层会拒绝该响应并继续 Provider 回退。若源端未暴露独立报告日期，现有契约允许调用方依据已校验的请求日期使用该快照，但仍保留供应商质量降级原因。
- 已补充日期不匹配的验证测试源码；未执行测试、构建、CI 或真实 Provider 请求。


### 个股所属板块未知空响应回退（2026-10-27）

- `has_data=false` 的所属板块响应必须保持 `quality_unknown`，不能解释为已确认无板块。服务层现在会继续尝试下一个启用 Provider；只有全部尝试完后才返回 unknown，并记录 `stock_boards_fallback_exhausted_without_confirmed_membership`。
- 仅在回退耗尽后缓存 unknown 结果 5 分钟；旧版没有记录“已耗尽回退链”的 unknown 缓存，在存在多个 Provider 时会被忽略，避免阻止新回退逻辑生效。已补相应测试源码，未运行。


### 龙虎榜未知空页的 Provider 回退（2026-10-27）

- 龙虎榜的无数据报告或整页非 A 股项目被排除后形成的 `quality_unknown` 空页，不会立即终止后备源尝试。只有所有启用来源均已尝试且没有任何来源返回可用 A 股条目时，才返回 unknown 并增加 `dragon_tiger_fallback_exhausted_without_confirmed_data`。
- 回退耗尽后保留 30 秒短缓存；多 Provider 场景会忽略不含回退耗尽标记的旧 unknown 缓存，防止旧逻辑挡住后备源。已补回归测试源码，未运行。


### 财务指标与指数目录/成分股未知空响应回退（2026-10-27）

- 财务指标 Provider 若返回合法但 `has_data=false` 的报告期快照、或带指标但质量为 `unknown`（例如无法证明公告日期），指数目录返回空 `unknown`，或指数成分股目录返回空 `unknown`，服务层会继续尝试其他已启用来源，不再让首源空/时点不明结果掩盖后备数据。财务 unknown 缓存只有在回退耗尽后才可在多 Provider 场景命中，TTL 为 5 分钟。
- 只有回退链耗尽时才返回 unknown，并分别增加 `fundamentals_fallback_exhausted_without_confirmed_data`、`index_catalog_fallback_exhausted_without_confirmed_data`、`index_constituents_fallback_exhausted_without_confirmed_data` 标记；未知空结果使用短缓存。多 Provider 场景会绕过旧版未记录回退耗尽标记的空缓存。
- 本轮只更新源码、文档和测试源码，未执行测试、构建、CI 或真实 Provider 请求。


### 指数目录缓存的类型边界静态修正（2026-10-27）

指数目录缓存命中直接返回目录快照；仅指数成分股快照需要执行 `markIndexConstituentCoverage`，两类模型不可混用。已删除指数目录分支中的误调用。按约定未执行构建、测试或 CI。


### 个股公告未知空结果回退（2026-10-27）

公告接口的 `total=0` 响应仍按 `quality_unknown` 处理，不视为已确认“没有公告”。服务层会继续尝试其他公告 Provider；只有回退链耗尽后才返回 unknown 并记录 `announcements_fallback_exhausted_without_confirmed_data`，再短缓存 30 秒。多 Provider 场景会绕过旧版未带回退耗尽标记的 unknown 空缓存。已补回归测试源码，未执行测试。


### 批量估值 unknown 回退（2026-10-27）

- 若估值 Provider 返回通过结构校验但 `quality_unknown` 的快照，服务层会继续尝试后续 Provider，避免一个来源的缺失数据遮住其他来源可用的估值。
- 只有回退链耗尽时才返回 unknown 并增加 `valuations_fallback_exhausted_without_confirmed_data`；仅该状态的 unknown 会短缓存 10 秒。多 Provider 场景会忽略旧版未带回退耗尽标记的 unknown 缓存。
- 已新增回归测试源码，未执行测试、构建、CI 或真实 Provider 请求。


### 指数历史日线未知空结果回退（2026-10-27）

- 指数历史日线 Provider 返回空 `items` 且 `quality_unknown` 时，不再立即成功返回并缓存 3 分钟；在默认自动来源模式下会继续尝试其他已注册的指数 Provider。
- 回退耗尽后保留 unknown，并记录 `index_daily_bars_fallback_exhausted_without_confirmed_data`，再进行 3 分钟短缓存。旧的未标记回退耗尽状态的空缓存，在多个 Provider 可用时会被绕过。传入明确 `source` 时仍保持来源固定，不跨代码空间自动切源。
- 已补回归测试源码，未运行测试、构建、CI 或真实 Provider 请求。


资金流空指标回退补充（2026-10-27）：合法的资金流响应必须至少包含一项可用金额指标（最新主力净流入、5 日合计或 10 日合计）。如果源端只返回日期/观测条数而所有金额都缺失，不会当作成功结果缓存，会继续尝试后备 Provider。已补测试源码，未运行。


### 人气/飙升榜未知空结果回退（2026-10-27）

- 人气、飙升、关注榜的 `items=[]` 且 `quality_unknown` 只代表当前来源无法确认榜单，服务层现在会继续尝试下一个已启用国内来源；仅在所有来源尝试完后返回 unknown 并标记 `hot_ranking_fallback_exhausted_without_confirmed_data`。
- 只有标记回退链已耗尽的 unknown 才会短缓存 10 秒；多 Provider 场景忽略旧版本未带该标记的 unknown 缓存，避免首源空榜遮住后备源。已补测试源码，未运行。


### 证券搜索未知空匹配回退（2026-10-27）

- 证券搜索遇到 `quality_unknown` 或 `quality_stale` 的空结果时，不能据此认定证券不存在；服务层现在继续尝试下一个国内搜索源。若首源返回确定有效的空结果（例如完整有效证券目录中确实无匹配），仍可直接返回。
- 只有未确认/过期的空匹配经过所有启用源后仍无确定结果时，才返回 `quality_unknown` 并附加 `stock_search_fallback_exhausted_without_confirmed_match`。已补测试源码，未运行。


证券搜索过期匹配的回退补充（2026-10-27）：当首个 Provider 只能提供 `quality_stale` 的匹配项时，服务层暂存该快照并继续查询后续来源；如果后续没有更新的可用结果，才返回原过期结果，并增加 `stock_search_fallback_exhausted_using_stale_directory`，保留 stale 状态。

### Tushare 个股日线证券类型边界（2026-10-27）

- Tushare `daily` 适配器现在在请求上游前要求代码属于普通 A 股股票范围；ETF、指数、B 股及其他非普通股票代码不再发给股票日线端点。指数历史数据仍走独立 `index_daily` 能力，ETF 相关日线暂不声明已由该 Provider 支持。
- 已补不触发真实 HTTP 请求的契约测试源码，未执行测试、构建、CI 或真实 Provider 请求。


### 当前资料/板块/榜单的时间质量门禁（2026-10-27）

- `StockBasic` 混合了当前估值/财务扩展字段，但 wire model 没有可核验的 Provider 快照时间；`StockBoards` 与 `SectorRanking` 同样没有独立快照时间字段。服务层现将供应商声明的 `valid` 下调为 `degraded` 并标记 `provider_snapshot_timestamp_not_exposed`；契约验证拒绝这些数据域在无 Provider 时间证据时保持 `valid`。旧的 valid 缓存会被验证拒绝并刷新。
- 已补 StockBasic、StockBoards、SectorRanking 的测试源码；依约没有运行测试、构建、CI 或真实 Provider 请求。


### 筹码快照未知质量的后备策略（2026-10-28）

筹码快照若虽包含有效平均成本但 Provider 明确标记 `quality_unknown`，服务层现在会继续尝试其他启用来源；只有来源链耗尽后才返回 unknown，并加 `chip_distribution_fallback_exhausted_without_usable_quality`，短缓存 5 分钟。多 Provider 场景会绕过未带耗尽标记的旧 unknown 缓存；同时按 Provider 名称去重，避免同一源因兼容接口重复注册而重复请求。本轮仅新增代码与测试源码，未运行测试、构建、CI 或真实 Provider 请求。


### 指数目录/成分存在条目但质量未知时继续回退（2026-10-28）

指数目录与当前成分快照即使返回条目，若质量仍为 `unknown`（例如无法确认源端时点），现在也会继续尝试其他启用 Provider；只有所有来源都未返回 `valid` 或 `degraded` 的可用结果时，才返回并短缓存 unknown。旧 unknown 缓存需带回退耗尽标记，才能在多 Provider 场景命中。显式指定指数目录来源时仍保持来源绑定，不跨代码空间回退。本轮新增测试源码，未运行测试、构建、CI 或真实 Provider 请求。


### 行业/概念领涨与领跌排名（2026-10-28）

- `GET /api/v1/market/sectors` 新增 `direction=up|down`，默认 `up` 保持旧调用兼容。东方财富按 `f3` 涨跌幅请求 `po=1/0` 并在服务端稳定排序；返回快照显式带 `direction`，缓存键按方向区分。
- 新增可选 `DirectionalSectorRankingProvider`，不要求现有 Provider 全部同步实现。TickFlow SW1 仅在能获得完整行业候选集合时提供领跌榜；若返回候选被截断，会报错并触发后备源，而不会将领涨前 N 项反向冒充领跌榜。TickFlow 结果仍是成分股平均涨跌幅估算并保持 degraded。
- 已补方向排序与服务契约测试源码；本轮未运行测试、构建、CI 或真实 Provider 请求。


### TickFlow SW1 行业榜整行业缺失质量说明（2026-10-28）

若某个 SW1 行业的所有成分股都缺少有效最新价/昨收价，估算榜会保留全集 `total`，跳过无法计算涨跌幅的行业，并新增 `one_or_more_industries_omitted_without_valid_constituent_quote_changes` 质量原因。此前仅对至少有一条可计算成分股的行业附加部分覆盖原因，无法显式说明整行业被跳过。新增测试源码覆盖该边界，未运行测试、构建、CI 或真实 Provider 请求。

## 2026-10-28 财报报表口径静态复核

- Tushare 三张报表的季度选择规则已逐项对照实现与迁移矩阵：利润表、现金流量表 Q1 使用 `report_type=1`，Q2-Q4 使用 `report_type=2` 的单季合并数据；资产负债表四个季度均使用 `report_type=1` 的期末合并数据。
- 季度口径测试源码扩展为三类报表 × 四个季度的 12 组组合，并澄清实现注释，避免将 Q1 也误读为 `report_type=2`。
- 本轮只补充源码与文档，未运行测试、构建、CI 或真实 Tushare 请求；仍留待统一验收。
## 2026-10-28 国内数据契约静态复核补充

- 已将父快照与条目 `source_version` 一致性校验扩展到指数报价、热度榜、涨跌停池、龙虎榜、公告、行业/概念排行、个股所属板块及指数日 K；财务、交易日历、市场广度、证券搜索、个股基础资料、资金流和筹码数据要求非空来源版本。
- 实时行情与日 K 的新 Provider 响应也校验归一化版本；显式旧版本继续回退，缓存要求当前版本。数据库日 K 回退分组包含来源版本，并按覆盖条数、最新交易日、采集时间及稳定 key 决胜，避免 Go map 遍历顺序造成不确定结果。
- 个股基础资料补齐总股本/流通股本、总市值/流通市值和上市/退市日期的边界校验；旧归一化版本缓存回源。
- Tushare 普通股票日 K 与指数日 K 的成交额千元→人民币元换算均有本地 HTTP fixture 断言；指数日 K 成交量继续保留手口径。报表季度口径测试源码覆盖三张报表 × 四个季度。
- 以上均为源码与文档静态完成状态。此轮未运行测试、构建、CI 或真实 Provider 请求，等待统一验收；代码已提交在当前目标分支，PR #1 仍保持 Open / Draft。
## 2026-10-28 数据层质量契约复核续记

- **来源版本门禁**：统一拒绝空版本和占位值 `unknown`（忽略大小写与首尾空白），覆盖行情、日 K、市场快照、指数/成分、财务、估值、公告、交易日历、证券搜索、基础资料、板块、筹码和资金流等主要数据域；行情、日 K 与财务/补充数据已增加代表性回归测试源码。
- **证券身份标准化**：个股基础资料与所属板块允许上游返回可规范化的无后缀代码，但服务返回及缓存命中结果统一为规范 `symbol`；指数报价、涨跌停池和龙虎榜校验 `symbol` 与 `code` 一致，避免错证券进入快照。
- **数值/分页一致性**：市场广度涨跌停数量不能超过覆盖证券数；财务分区指标 key 必须唯一且元数据非空；行业/概念榜的排名与 `up/down` 涨跌幅方向必须一致；热度榜名次必须唯一且按名次递增；涨跌停池 `total` 不能小于返回条数，矛盾计数会拒绝并回退。
- **日 K 回退与数据单位**：数据库日 K 候选按覆盖度、最新交易日、采集时间与稳定 key 决胜，不依赖 map 遍历顺序。Tushare 个股日 K 与指数日 K 的成交额单位换算有本地 HTTP fixture 测试源码，并区分个股成交量换算为股、指数成交量保留手的口径。
- 本轮仍只做源码、测试源码和文档更新。未运行 Go tests/race/vet、构建、CI 或真实 Provider 请求；PR #1 保持 Open / Draft，不合并。下一阶段继续静态对账剩余 Provider 字段/分页契约，最终由统一验收执行测试与真实数据源检查。


### 证券搜索目录分页一致性复核（2026-10-28）

- 东方财富全量证券目录抓取新增跨页 `total` 一致性门禁，并拒绝单页原始行数超过 `total` 所能容纳的条数；避免源端计数漂移或超额页被误当作完整有效目录。
- 新增两项 Provider HTTP fixture 测试源码，覆盖分页总数漂移与末页超额返回；并修整最近涨跌停池计数测试源码的缩进。已同步市场数据契约和迁移矩阵。
- 此批只完成静态审查和源码/文档修改，未运行测试、构建、CI 或真实 Provider 请求；仍待统一验收。PR #1 保持 Open / Draft，不合并。


### 公告与龙虎榜分页计数静态复核（2026-10-28）

- 公告列表校验当前页 returned 必须与 total/page/page_size 推导的预期条数一致；东方财富 Provider 与服务缓存/新响应验证均执行该规则。
- 龙虎榜校验 pages 必须与源端总数及页大小相符，当前页源端原始行数必须完整；服务层进一步验证每页 source_returned/returned/excluded_non_a_share_count 的关系，避免部分页被当成有效快照返回。
- 对应 Provider HTTP fixture 与服务层契约测试源码已添加。此次仅静态审查和源码更新，没有运行 Go 测试、构建、CI 或真实 Provider 请求；仍需统一验收。PR #1 继续保持 Open / Draft。


### 腾讯证券搜索身份分类复核（2026-10-28）

- 腾讯 Smartbox 搜索返回的市场标签不能单独区分股票与 ETF、指数、可转债等证券。增加沪深北普通 A 股代码段筛选，避免非股票品种进入个股搜索结果；若候选被过滤为空，仍触发后备 Provider。
- 增加覆盖沪深股票、北交所股票以及 ETF/指数/可转债过滤的测试源码。此项仅完成静态审查与源码修改，未运行测试、构建、CI 或真实 Provider 请求；待统一验收。


### 行业/概念排行返回覆盖度复核（2026-10-28）

- 增加行业/概念排行的原始返回条数与 total/limit 关系检查，源端返回不足或超额时走 Provider 回退；服务层要求返回条数与请求上限及总数相符，防止截断候选只因源端时间缺失而以通用 degraded 原因进入缓存。
- 若源端未给出可用 total，适配器会记录使用已返回条数作为下界的明确质量原因。相关 HTTP fixture 与服务层测试源码已补齐；未运行测试、构建、CI 或真实 Provider 请求，仍需统一验收。


### 指数成分旧缓存覆盖计数复核（2026-10-28）

- 修复指数成分缓存命中遗漏覆盖质量降级的问题：非空旧缓存缺少 `provider_returned` 时，同样追加 `provider_constituent_source_coverage_unreported`；原本 `valid/degraded` 的结果降级，原本 `unknown` 的结果仍保持 unknown。
- 新增旧缓存 valid 和 unknown 两种状态的回归测试源码。尚未运行测试、构建、CI 或真实 Provider 请求；统一验收仍需确认。


### 涨跌停池源端日期质量校验（2026-10-28）

- 服务层新增 valid 涨跌停池必须有 `provider_report_date` 的门禁；缺少源端报告日期的旧缓存或 Provider 回包不能被标为 valid，也不能绕过缓存校验。日期存在时仍须匹配请求交易日且不能晚于当前中国市场日期。
- 新增有效状态缺少报告日期与报告日期匹配的测试源码。未执行测试、构建、CI 或真实 Provider 请求，待统一验收。


### 公告源端分页身份静态复核（2026-10-28）

- 新增东方财富公告响应 `page_index/page_size` 与请求参数的一致性校验；源端明确返回错页或错页大小时拒绝候选，允许字段缺失但不伪造源端分页元数据。
- 新增源端页码及页大小错配的 HTTP fixture 测试源码；未运行测试或 CI，仍待统一验收。


### 龙虎榜源端失败状态复核（2026-10-28）

- 东方财富龙虎榜适配器新增 `success=false` 显式失败门禁，防止错误响应携带的残留 `result/data` 被当作有效候选。未提供 `success` 时不额外拒绝，仍执行既有计数和行级契约。
- 新增失败标志存在但数据体非空的 HTTP fixture 测试源码；未运行测试、构建、CI 或真实 Provider 请求。


### Tushare 公告重复记录静态复核（2026-10-28）

- 重复公告记录仍按稳定 article ID 去重，但不再静默丢弃：父快照增加 `provider_duplicate_announcement_rows_deduplicated`，质量降级并在迁移文档说明 `total/returned` 是去重后唯一记录数。
- 新增 Tushare HTTP fixture 测试源码，覆盖重复记录去重和质量原因；未执行测试、构建、CI 或真实 Provider 请求。


### 热度榜重复证券与报价覆盖静态复核（2026-10-28）

- 东方财富热度榜 Provider 不再静默跳过重复证券；行情补充响应也拒绝重复 `f12`，防止 map 写入时后续行覆盖先前值。
- 新增两项本地 HTTP fixture 测试源码，分别覆盖榜单重复证券和补充行情重复代码。尚未执行测试、构建、CI 或真实 Provider 请求。


### 热度榜热度值边界复核（2026-10-28）

- 热度榜服务层此前只检查价格、涨跌额与涨跌幅的有限性，漏掉可选 `heat`；现将热度分值纳入同一 NaN/Inf 校验，非法候选继续 Provider 回退。
- 新增 NaN 热度分值触发后备 Provider 的测试源码。没有运行 Go 测试或 CI，待统一验收。


### 东方财富热度榜证券类型复核（2026-10-28）

- 热度榜代码解析器现在验证普通沪深 A 股代码段，拒绝 ETF、指数、可转债、B 股及未支持代码，避免仅靠市场标签将非个股写入热度榜。
- 新增普通主板/科创板/创业板股票与 ETF/指数/可转债/B 股的代码段测试源码；未运行测试、构建、CI 或真实 Provider 请求。


### 行业/概念排行显式总数静态复核（2026-10-28）

- 将源端 `total` 改为可区分缺失与显式值；显式 `total=0` 且 `diff` 非空时拒绝该候选，缺失 `total` 时才使用已返回条数作为下界并追加明确质量原因。
- 新增显式零值与总数缺失两种响应的 HTTP fixture 测试源码；未执行测试、构建、CI 或真实 Provider 请求。


### 热度榜排名缺号检查（2026-10-28）

- 在现有名次为正、唯一、递增校验基础上，增加从 1 开始的连续排名校验；出现 1、3 等缺号序列时拒绝该候选并尝试后备源。
- 新增排名缺号触发回退的测试源码。未执行测试、构建、CI 或真实 Provider 请求。

## 2026-10-28 雪球榜单请求与名次变化质量复核

- 雪球热度榜调用方请求超过 50 条时，适配器实际请求上限固定为 50；新增 `provider_request_limit_capped_at_50`，避免将内部限额造成的覆盖不足静默呈现。
- 可选 `rank_change` 的非空非法整数或越界值会被省略，并同时写入父快照与条目的 `provider_rank_change_invalid_or_out_of_range`，不再静默吞掉源端字段错误。
- 已补本地 HTTP fixture 测试源码，覆盖 100 条请求被限为 50 条以及非法名次变化质量原因传播。本轮尚未执行 Go 测试、构建或真实上游请求。

## 2026-10-28 Tushare 涨跌停池过滤质量补充

Tushare `limit_list_d` 返回的非 A 股代码会被过滤，不进入标准化涨跌停池；此前该过滤没有留下质量原因。现在将 `provider_non_a_share_security_rows_excluded` 加入父快照原因列表，避免把过滤后的条目集合误认为与原始响应完全一致。已新增 HTTP fixture 回归测试源码，尚未执行。

## 2026-10-28 指数成分报告日与覆盖计数复核

- Tushare `index_weight` 现在先按源端日期选择最新报告日，再过滤非 A 股/无法标准化的成分记录，避免最新报告日因过滤后为空而错误回退到旧日期。
- `ProviderReturned` 覆盖所选报告日的源记录；`ExcludedCount` 同时统计不受支持证券与名称无法核验的记录，保持 `ProviderReturned = Total + ExcludedCount`。
- 最新报告日没有可用 A 股成分时，返回带日期与排除计数的 `unknown` 快照，让服务层继续回退，不再请求较旧月份补成分。新增两项 HTTP fixture 测试源码，尚未执行。

## 2026-10-28 雪球响应状态码门禁

雪球适配器原先只在 `error_code` 能解析且非零时拒绝响应；若字段存在但格式非法，则仍可能继续使用 `data`。现在任何存在但无法解析成整数的 `error_code` 都会拒绝当前响应，并新增本地 HTTP fixture 测试源码；未执行测试。

## 2026-10-28 Tushare 龙虎榜重复证券门禁

Tushare `top_list` 分页现在维护证券代码集合；同页中普通 A 股证券重复时直接拒绝 Provider 候选，避免同一证券重复参与龙虎榜统计。新增 HTTP fixture 测试源码，未执行 Go 测试或真实上游请求。

## 2026-10-28 Tushare 股票基础资料重复行门禁

Tushare `stock_basic` 按 `ts_code` 查询单只证券时，原适配器只接收首条匹配行，忽略后续重复行。现在一旦发现重复身份便拒绝当前 Provider 候选，并新增 HTTP fixture 回归测试源码；尚未执行测试或真实请求。

## 2026-10-28 东财涨跌停池覆盖不足提示与分页补齐

最初版本会保留源端 `tc` 作为 `total`，当第一页的 `pool` 条数不足时附加 `provider_total_exceeds_returned_rows`，但没有尝试请求后续页。后续功能提交已补齐内部分页：每页最多 30 条，按 `Pageindex` 递增，直到满足 `min(tc, limit)`；多页 `tc/qdate` 不一致、达到目标前出现空页或分页契约错误时，拒绝该候选让服务层尝试后备源。若调用方的 `limit` 低于 `tc`，仍会保留截断质量原因。

新增本地 HTTP fixture，模拟 65 条记录分三页完整返回，并断言记录顺序、页数和没有错误截断标记。测试源码已提交，尚未运行 Go 测试、构建、CI 或真实 Provider 请求；源端时间戳仍不可核验，因此快照不会仅因分页完整而升为 `valid`。

## 2026-10-28 Tushare 涨跌停池重复证券门禁

Tushare `limit_list_d` 现在检查同一响应中的普通 A 股证券代码唯一性；重复证券直接拒绝当前 Provider。已新增 HTTP fixture 回归测试源码，尚未执行 Go 测试、构建或真实上游请求。

## 2026-10-28 个股公告证券类型门禁

东方财富公告与 Tushare `anns_d` Provider 现会在请求上游之前校验证券是否属于受支持的普通 A 股代码段，拒绝 ETF、指数、可转债、B 股等非股票证券。已增加两个 Provider 的 HTTP fixture 测试源码，尚未执行。

## 2026-10-28 Tushare 龙虎榜非 A 股过滤原因

Tushare `top_list` 将非普通 A 股记录计入 `ExcludedNonAShareCount` 时，现在同步在父快照加入 `provider_non_a_share_security_rows_excluded`。新增混合普通股票/可转债回包的 HTTP fixture 测试源码，尚未执行。

## 2026-10-28 东方财富股票接口证券范围门禁

东方财富普通股票报价、日 K、基础资料、所属板块、筹码分布和个股资金流新增统一证券类型门禁，在请求上游前拒绝 ETF、指数、可转债、B 股等非普通 A 股证券。新增单项覆盖六个接口的 HTTP fixture 测试源码；尚未执行测试或真实 Provider 请求。

## 2026-10-28 东方财富涨跌停池普通 A 股证券校验

涨停、跌停及炸板池现在在证券代码标准化后复用普通 A 股范围校验，拒绝 ETF、指数、B 股及市场字段不一致的证券，防止非股票证券进入事件池。新增沪市/北交所正例和多类负例的 HTTP fixture 测试源码，尚未执行测试或真实 Provider 请求。

## 2026-10-28 东方财富证券搜索目录证券类型校验

证券搜索目录现在根据源端市场字段构建证券标识，并复用普通 A 股代码段校验；ETF、指数、B 股及市场字段错配记录会被过滤并通过 `provider_non_a_share_security_rows_dropped` 暴露，目录质量降级。已新增覆盖沪深/北交所正例和多类负例的 HTTP fixture 测试源码，尚未执行测试或真实 Provider 请求。

## 2026-10-28 东方财富涨跌停池触板时间校验

首次/最后触板时间现在除数字格式外，还校验时、分、秒范围；非空无效值将被清空，并在记录和父快照的质量原因中写入 `provider_limit_pool_touch_time_invalid`。新增边界和 HTTP fixture 测试源码，尚未执行测试或真实 Provider 请求。

## 2026-10-28 东方财富指数行情数值与计数校验

指数行情新增源端总量与返回行数对账、必需最新价和可选 OHLC 正数校验，以及 OHLC 范围一致性检查。源端总量不足以覆盖返回行时拒绝响应；总量大于返回行时将快照降级。负成交额不再作为有效 CNY 金额返回，而是置空并记录质量原因。新增了对应 HTTP fixture 测试源码，尚未执行测试或真实 Provider 请求。

## 2026-10-28 东方财富指数涨跌字段一致性

当回包提供昨收价时，现在对涨跌额和涨跌幅分别与最新价/昨收价进行交叉校验，超过合理舍入容差则排除该记录并标记快照降级。同步修订核心指数行情 fixture，使模拟价格、涨跌额和涨跌幅相互一致，并新增两类字段不一致测试源码；尚未执行测试或真实 Provider 请求。

## 2026-10-28 东方财富日 K 缺失证券身份降级

东方财富日 K 现在在源端代码缺失时保留可通过其他校验的数据，但将每条结果降级并添加 `provider_symbol_missing_assumed_from_request`；源端代码存在但与请求不一致时仍然拒绝回包。新增缺失身份 HTTP fixture 测试源码，尚未执行测试或真实 Provider 请求。

## 2026-10-28 TickFlow 指数涨跌字段交叉校验

TickFlow 指数行情现在始终使用最新价和昨收价计算涨跌额及涨跌幅，源端 `ext.change_amount` / `ext.change_pct` 只作为一致性检查；出现非数值或超出舍入容差时排除该行并降级父快照。新增多指数 HTTP fixture 测试源码，覆盖相互矛盾扩展字段；尚未执行测试或真实 Provider 请求。

## 2026-10-28 Tencent 个股接口证券范围校验

Tencent 个股报价和日 K 入口新增普通 A 股范围门禁，在上游请求前拒绝 ETF、指数、可转债和 B 股等非普通股票证券。新增覆盖两类接口、五种非普通证券的 HTTP fixture 测试源码，尚未执行测试或真实 Provider 请求。

## 2026-10-29 国内数据层分页与默认覆盖续审

- **东方财富涨跌停池分页行数校验**：每页原始 `pool` 行数必须等于源端 `tc`、`Pageindex` 和 `pagesize` 推导出的预期值；源端总数与原始行数矛盾时拒绝当前 Provider。已增加超出源端总数的 HTTP fixture 回归测试，未执行。
- **板块排行缺总数与过滤丢行**：当 CLIST 未报告可信 `total`，同时必填字段校验丢弃原始行时，服务层拒绝该候选并触发回退，不能把剩余条数当作完整榜单。已增加服务层回归测试，未执行。
- **热度榜补充行情身份集合**：补充行情必须有非空 `f12`，代码唯一且属于请求榜单的股票代码集合；额外代码或无身份行直接拒绝。已增加两项 HTTP fixture 回归测试，未执行。
- **市场广度时间戳测试稳定性**：时间偏差 fixture 在接近中国时区午夜时锚定到前一日收盘附近，避免故意制造的 11 分钟跨度误跨报告日。已更新测试源码，未执行。
- **Tushare 涨跌停池默认条数对齐**：HTTP 与 Service 默认最多返回 100 条；Tushare Provider 在 `limit<=0` 时此前使用 30 条，已对齐为 100，并新增 35 条模拟记录、`limit=0` 的回归测试。未执行。

以上提交仅有源码与测试源码，没有执行本地 Go 测试、构建、race、vet、CI 或真实 Provider 请求。目标运行环境无法在这轮完成 Go 依赖下载，所以这些新测试仍需统一验收。

## 仍未闭环的功能（2026-10-29）

1. **执行自动化验收**：对根 module、`go-core`、`services/market-data` 执行 `gofmt` 检查、`go test ./...`、`go test -race ./...`、`go vet ./...`；然后针对目标 Docker 网络做有控制的真实 Provider 请求与字段单位对账。静态审查不能替代这些验收。
2. **榜单与目录的源端覆盖账本**：涨跌停池、Tushare 龙虎榜、Eastmoney/同花顺热度榜、Eastmoney/TickFlow 行业排行、同花顺指数/板块目录及东方财富个股所属板块列表现已输出原始/发现条数、过滤数量与适用时的总数下界标记。后续继续补齐仍未具备来源计数元数据的其他列表接口；不能把过滤或来源上限造成的缺失伪装成完整结果。
3. **严格历史时点数据**：日 K 与财务快照现均具备追加式内容版本表；日 K 接口支持基于本地版本的 `as_of` 查询且逐根返回系统观察时间，财务接口也支持本地版本查询并按披露日期剔除无法证明在时点前公开的分区。但这些能力只覆盖启用后实际记录的版本。历史公司行动/复权因子版本，以及指数成分真实纳入/剔除生效区间仍未完整持久化。Tushare 指数成分的 `as_of` 仍只表示不晚于请求日期的最近权重报告，不能作为真实成分生效区间。
4. **原 Python 能力的字段级等价**：继续对照 Efinance/AkShare 相关调用链，验证证券范围、字段映射、单位、日期覆盖、空结果及错误/回退语义；不得仅凭 Go 路由已存在就标记迁移完成。
5. **交易制度与市场时间**：已新增交易日历驱动的市场统一交易时段接口、有效日期涨跌幅规则解析器、历史证券状态 Provider/API 和追加式 PIT 状态存储。Tushare 可按交易日读取历史名称、交易日历及停复牌证据；启用 MariaDB 时按证券/日期/来源留存内容变化，可传 `as_of` 回放系统当时已记录的版本。北交所上市交易日序号、退市整理/重新上市首日的精确事件、交易所临时停市仍不完整；规则解析器对无法核验的输入返回 unknown，不推断为普通股票。

## 2026-10-29 国内个股接口普通 A 股身份门禁补全

- **补齐通用个股 Provider 证券类型校验**：新浪、网易、Pytdx、BaoStock、同花顺与 TickFlow 的个股入口此前主要检查代码格式/市场后缀，可能将 ETF、指数、B 股或可转债传入只面向普通股票的接口。现统一使用普通 A 股代码段门禁，拒绝不支持证券；仍按各 Provider 能力限制交易所范围。
- **补齐 Tushare 财务与估值接口门禁**：`fina_indicator` 与 `daily_basic` 在发起上游请求前校验普通 A 股证券，避免以外部合法六位证券代码误调用个股财务/估值数据集。
- 新增 `domestic_stock_symbol_contract_test.go`，覆盖普通沪深股票正例与 ETF、指数、B 股、可转债负例，并验证 Tushare 财务/估值请求在拒绝非法证券时不会触发 HTTP 请求。
- 本次代码与测试源码已提交；当前尚未执行 Go 测试、race、vet、构建或真实 Provider 请求，测试结果未确认。由于提交要求使用 `[skip ci]`，常规提交触发路径不会运行；当前可用 GitHub 工具没有工作流手动 dispatch 写入能力，需在后续可用环境中统一验收。


## 2026-10-29 涨跌停池与龙虎榜源端覆盖账本

- `LimitPoolSnapshot` 新增 `provider_returned`、`excluded_non_a_share_count` 与 `total_is_lower_bound`。东方财富按实际收到的分页原始行累计 `provider_returned`；同花顺记录返回条数；Tushare 分别记录原始表行数和被过滤非 A 股行数。
- Tushare `limit_list_d` 返回达到 2500 行时，`total` 明确只是下界，并记录 `provider_response_may_be_capped_at_2500_rows` 与 `provider_limit_pool_total_is_lower_bound`。
- `DragonTigerSnapshot` 新增 `total_is_lower_bound`。Tushare `top_list` 达到 10000 行时记录 `provider_response_may_be_capped_at_10000_rows` 与 `provider_dragon_tiger_total_is_lower_bound`，避免把由截断行数计算出的分页数当作权威总页数。
- 新增 2500 行与 10000 行边界 HTTP fixture 测试源码，并更新 OpenAPI 与数据契约。当前仅提交代码和测试源码，没有执行 Go 测试、构建、CI 或真实 Provider 请求；待统一验收。


## 2026-10-29 热度榜源端计数与过滤账本

- `HotRankingSnapshot` 新增 `provider_returned`、`excluded_rows` 和 `total_is_lower_bound`。Eastmoney 记录热度榜原始返回条数；同花顺保留源端原始条数并计数去重时排除的重复证券。
- Eastmoney 与同花顺都没有可用于当前查询的权威排行榜总数，`total` 仍与实际输出 `items` 数保持一致以兼容服务层契约，同时明确标记为下界并增加 `provider_total_missing_using_returned_count`。
- 新增两个 HTTP fixture 测试源码，覆盖请求 limit 截断与同花顺重复证券排除计数；本轮仍不执行 Go 测试、构建、CI 或真实 Provider 请求，统一放到开发任务完成后验收。


## 2026-10-29 行业排行源端覆盖账本

- `SectorRankingSnapshot` 新增 `provider_returned`、`excluded_rows` 和 `total_is_lower_bound`。Eastmoney 保留原始 CLIST `diff` 行数并计入必填字段过滤行；源端不返回 `total` 时明确把使用解析条数得到的 `total` 标记为下界。
- TickFlow 以已完整核验的 SW1 行业池数量作为来源总数，记录行业池原始覆盖规模，并统计因没有有效成分股涨跌幅而未形成排行的行业。此情形继续降级，不再只靠一条文本质量原因表达遗漏数量。
- 新增 Eastmoney 缺失 `total` + 必填字段过滤的 HTTP fixture 断言，并扩展 TickFlow 行业排行覆盖断言。尚未执行测试、构建、CI 或真实 Provider 请求；留待开发收敛后统一验收。


## 2026-10-29 指数成分来源截断下界

- `IndexConstituentSnapshot` 新增 `total_is_lower_bound`。Tushare `index_weight` 月度响应达到 5000 行时，在质量原因之外同时暴露结构化下界标记；无论该报告日含有可用 A 股成分还是仅含不支持证券，都会保留标记。
- 新增 5000 行边界 HTTP fixture 测试源码，覆盖报告日、原始返回数、规范化条数及下界标记；未执行测试、构建、CI 或真实 Provider 请求。
- 该修改只解决源端行数边界的可见性，不声称恢复指数成分的真实生效区间或严格 PIT 历史能力。


## 2026-10-29 交易日历多来源择优与冲突标记

- `DataService.TradingCalendar` 不再在第一个结构完整的 Provider 处提前返回，而是比较全部已配置来源，并优先选择明确覆盖逐日日期的日历，而不是从交易日列表推断非交易日的来源。
- 如果来源对任一日期的 `is_trading_day` 状态有冲突，返回证据评分较高的候选，并降级质量、增加 `provider_trading_calendar_sources_disagree`。缓存仍保存所选结果和冲突状态。
- 新增服务层 fixture 测试源码，覆盖来源择优与跨源冲突；当前未执行 Go 测试、构建、CI 或真实 Provider 请求。权威交易所日历和临时交易安排仍需后续整体验收。


## 2026-10-29 日 K 内容修订版本留存

- 新增 `market_data_daily_bar_versions`，按来源、证券、交易日、周期、复权口径与内容指纹保存不同版本；同一版本重复入库幂等，变化内容保留新的 `recorded_at`。
- `UpsertBars` 在同一事务中先追加版本，再更新 `market_data_daily_bars` 当前值。新增 `ListBarsAsOf`，按观察时点查询各来源当时最新版本，不混淆上游报告日期与系统观察日期。
- 新增纯函数测试源码，验证抓取时间变化不会制造新版本，而收盘价变化会产生新指纹。未运行测试、构建、CI 或数据库操作。
- 该日 K 版本留存目前提供仓储层 `ListBarsAsOf`，没有独立的日 K HTTP PIT 查询端点；版本保留/压缩策略仍未实现。财报修订与财务 `as_of` API 已在后续提交补上，但没有解决复权因子历史可用时间和指数成分生效区间问题。


## 2026-10-29 财务快照修订与 PIT API

- 新增 `market_data_fundamental_versions`，按内容变化追加版本；只与该来源的最近一版去重，避免内容从 A→B→A 后无法记录恢复版本。
- 常规财务查询在 Provider 结果、缓存结果和 unknown 候选可用时尽力留存本服务所见版本。新增 `FundamentalsAsOf` 与查询参数 `as_of`：只读取截至指定系统时刻已记录的快照，不回退实时 Provider，并返回 `system_observed_at`。
- PIT 分区要求披露日期严格早于 `as_of` 对应的上海自然日；同日披露时刻不明或披露日期缺失的分区会被剔除，部分结果降级。没有可用版本返回 404，未启用本地版本存储返回 503。
- 添加 HTTP/服务/仓储测试源码，未执行测试、构建、CI、数据库操作、Provider 请求或整体回测。历史快照仅从版本化开始后可追溯；公司行动、复权因子与指数成分真实生效区间仍未闭环。


## 2026-10-29 指数/板块目录来源覆盖账本

- `IndexCatalogSnapshot` 新增 `provider_returned`、`excluded_rows` 与 `total_is_lower_bound`。同花顺目录记录原始返回条数；重复代码过滤时累计排除数量；达到本地 30000 项安全边界时显式下界并降级。
- 服务层校验来源计数能否与保留条数、过滤数量对账，拒绝无法自洽的覆盖元数据。
- 新增同花顺 HTTP fixture 测试源码，覆盖重复目录代码的原始数、过滤数和降级状态；未执行测试、构建、CI 或真实 Provider 请求。

## 2026-10-29 个股所属板块来源覆盖账本

- `StockBoardSnapshot` 新增 `provider_returned`、`excluded_rows` 与 `total_is_lower_bound`。东方财富所属板块接口记录实际收到的原始行数；返回总数达到请求的 1000 行边界时，明确置下界标记并降级，避免把源端上限误当完整板块数量。
- 服务层校验来源返回数与过滤数、保留数可对账；同一批次中无效/重复条目仍按 Provider 失败回退，不会被静默过滤成看似完整的结果。新增 1000 行 HTTP fixture 测试源码，未运行测试、构建、CI 或真实 Provider 请求。

## 2026-10-29 市场统一交易时段服务

- 新增 `MarketSessionSnapshot`、`DataService.MarketSession` 与 `GET /api/v1/market/session`。接口用 Provider-backed 交易日历确认日期是否交易日，再按 `Asia/Shanghai` 固定时段计算市场阶段；支持省略 `at` 使用当前时间或传入 RFC3339 时间戳查询。
- 将开盘集合竞价、竞价间隔、上午连续交易、午休、下午连续交易、收盘集合竞价及闭市拆分为独立状态，并分别表达是否处于交易时段、连续交易或集合竞价。
- 新增服务层时间边界和 HTTP fixture 测试源码，更新 OpenAPI 与契约文档；未执行测试、构建、CI 或回测。
- 该接口仅给出市场统一的计划时段，不判断个股停牌、ST/退市整理、证券特定涨跌幅限制及临时停市；这些规则仍是开发清单。

## 2026-10-29 日 K 历史版本 `as_of` API

- `/api/v1/bars/{symbol}/daily` 新增可选 `as_of`。提供该参数时只查本地追加式历史版本，拒绝未来时间、拒绝超过观察日期的 `end`，不请求实时 Provider、不回退当前值表。
- 仓储查询返回每条记录的系统 `recorded_at`，API 通过 `system_observed_at` 暴露；服务按来源/来源版本/复权参考日期/归一化版本择选一致序列，避免跨来源拼接。
- 增加 service/HTTP 边界测试源码并更新 OpenAPI 和契约文档；未运行测试、构建、CI、数据库操作或整体回测。缺少一致历史序列返回 404，版本存储不可用返回 503。真实历史公司行动/复权因子及指数成分生效日期仍未解决。

## 2026-10-29 有效日期涨跌幅规则解析器

- 新增 `PriceLimitRuleSnapshot` 与 `ResolvePriceLimitRule`，按证券代码识别主板、创业板、科创板和北交所，并对照有效日期返回比例、版本标识、生效区间及官方规则链接。
- 处理主板风险警示股票在 2026-07-06 前 5%、此日起 10%；创业板在 2020-08-24 前 10%、此日起 20%；科创板 20%；北交所 30%；IPO 初期无涨跌幅限制窗口按板块/改革日期区分。此解析只输出规则政策，不自动认定个股状态。
- 对风险警示标志、上市交易日序号、重新上市/退市整理首日等必需状态缺失时，返回 `status=unknown` 和质量原因，禁止猜测。增加规则边界测试源码，未运行测试、构建、CI 或回测。
- 仍未完成：提供历史有效日期的 ST/退市整理状态、个股停牌与复牌事件、交易所临时停市的数据源与落库版本，并将这些输入接入统一规则解析。

## 2026-10-29 历史证券交易状态 Provider 与 API

- 新增 `SecurityTradingStatusProvider`，Tushare 组合 `stock_basic`、`namechange`、`trade_cal` 和 `suspend_d`，按 `date` 查询历史有效名称、风险警示名称标记、上市交易日序号和停复牌状态。
- 新增服务层 Provider 回退/契约验证、HTTP `GET /api/v1/stocks/{symbol}/trading-status?date=YYYY-MM-DD`、数据源能力字段及 `security_trading_status` 配置项。该能力启用时要求同一 Provider 同时启用 `stock_basic` 与 `trading_calendar`。
- 未核验的历史名称、ST 状态、上市交易日序号或停复牌记录以 `unknown`/质量降级表达；当前北交所上市日序号暂不推断。新增 Provider、服务和 HTTP fixture 测试源码，未执行 Go 测试、构建、CI、真实 Provider 请求或整体回测。
- 后续仍需开发：补齐退市整理、重新上市首日、停复牌特殊事件和交易所临时停市等有明确生效日期的权威事件源；只有拿到可核验事件后，才将特别无涨跌幅状态接入 `ResolvePriceLimitRule`。

## 2026-10-29 证券交易状态追加式 PIT 存储

- 新增 `market_data_security_trading_status_versions`，为每个证券/交易日/来源只在内容变化时追加版本。指纹忽略 `fetched_at` 和 `system_observed_at`，避免重复抓取生成无意义版本；同内容修订不新建记录，后续纠正内容会生成新版本。
- 实时 `GET /api/v1/stocks/{symbol}/trading-status?date=YYYY-MM-DD` 在 MariaDB 可用时追加留存本次所见的内容版本，并返回 `system_observed_at`；带 `as_of=RFC3339` 时只查本地版本表，不请求 Provider；未启用存储返回 503，没有可用历史版本返回 404。
- 新增仓储指纹、服务 PIT 参数及 HTTP 边界测试源码，并更新 SQL 自动迁移、OpenAPI 和契约文档。未执行测试、构建、CI、真实 Provider 请求或回测。
- 保持未闭环项：Tushare 历史名称推断不等同于权威 ST/退市状态表，停复牌源端历史覆盖、北交所上市日序号、特殊首日交易限制和交易所临时停市事件仍需补齐与核验。


### 本轮补充：退市状态必须来自事件证据

- `delistingStatusFromHistoricalName` 不再把“退市整理”“退市”“摘牌”等历史名称直接转换成确定退市状态；在缺少可核验事件源时统一返回 `unknown`。
- 原因：名称记录描述的是名称在某段时间内生效，不等同于交易所对退市整理期、重新上市首日或特殊无涨跌幅限制的事件确认。若把名称线索写入确定状态，下游规则计算可能误给出确定结果。
- 对应测试覆盖普通名称、`*ST`、退市整理、退市、摘牌和 `PT` 名称均不会单独确立退市状态。测试源码已更新，但本轮没有在执行环境运行 Go 测试。
- 下一步应优先设计独立的、带来源/有效日期/公告或事件标识的退市事件证据契约，并让规则计算消费该契约；在该来源落地前，特殊事件状态继续保持未知。

### 本轮执行记录（2026-10-29）

- 规则服务测试补齐：覆盖主板风险警示规则切换日、各板块涨跌幅、IPO 无涨跌幅窗口、明确重新上市首日事件、未知关键输入、非法交易日、非法状态枚举和不支持的证券板块。
- 新增 Tushare 日 K 复权测试：核对前复权参考日期及 OHLC 换算、后复权不改动成交量/成交额、缺失/非法复权因子必须失败。该提交补的是测试契约，不代表已经完成真实 Provider 对账。
- 修复估值服务 metricCount 缺失循环闭合括号的语法错误，并处理 Go 核心客户端文件中已由 CI 指出的格式差异。
- CI 暴露出更大范围的格式债务：market-data 模块的 gofmt 检查涉及多个既有文件，导致该模块测试和镜像构建在格式检查阶段中止；本轮不对整个模块进行大规模机械格式化，以免把无关格式改动混入业务提交。
- Go 核心客户端测试另有 TestFetchStockBundleFetchesNativeAuxiliaryData 失败，断言为 income_statement.total_revenue 指标映射；需要独立核对 merge commit 下的映射实现与测试数据，不能把它当作本轮规则或复权测试失败。
- 截至本记录，最新代码提交已触发 CI；新增测试尚未获得完整通过结果。后续先修复明确的 CI 阻塞，再继续补充 Provider 功能缺口，不进行全仓库重复审查。

## 10. 后续开发路线重排（2026-10-29）

本节作为后续开发的执行顺序；不重复开启已接入能力的静态 Review。开发期只补明确的功能缺口、契约缺口和对应回归用例，统一测试、真实 Provider 取数、部署验收和回测留到功能收敛后集中执行。

### 阶段 A：历史证券状态证据与涨跌幅规则闭环（最高优先级）

**A1. 明确历史状态字段的证据边界**
- 检查 `SecurityTradingStatusSnapshot` 的每个状态字段是否能区分“有来源支持的事实”“由历史名称推断的候选”和“未知”。尤其是 `delisting_status`：历史名称中的“退市/摘牌/退市整理”只能作为线索，不能无提示地等价于交易所权威退市事件。
- 对来源缺失、历史覆盖范围外、名单结构不完整、历史名称与 `stock_st` 冲突等情况，保留 `unknown` 和可追踪的 `quality_reasons`；不得由 Provider 回退将未知状态提升为普通状态。
- 交付：必要的模型/Provider/服务层修复及定向测试源码。

**A2. 建立特殊交易事件的来源契约**
- 对重新上市、退市整理首日、复牌首日、交易所指定的无涨跌幅限制及临时停市，明确事件类型、证券/市场范围、交易日期或有效区间、来源 URL、原始公告标识和采集时间。
- 只有存在可核验的事件记录或能证明当日事件状态完整的权威来源时，才允许将特殊事件判定为“无例外”。普通查询未找到事件不能自动代表 `none`。
- 暂无可靠来源的事件类型继续为 `unknown`，先完成可扩展的数据契约与限制说明；不伪造历史事件，不为追求接口可用率放宽质量门禁。

**A3. 接通规则输入，但不提前开放误导性 API**
- 将经校验的历史风险警示状态、上市交易日序号、特殊事件证据与 `ResolvePriceLimitRule` 组合；对输入日期/证券身份不一致、字段未知、证据冲突、退市状态不适用普通 ST 规则等情况返回可追溯的未知结果。
- 北交所上市交易日序号继续保持未知，直至找到可核验的北交所历史交易日历/上市交易日数据；不能以沪深日历代算。
- 先交付服务层调用和单元测试源码，再评估是否需要单独 HTTP 路由。只有关键输入具备可靠来源、质量原因能向调用方传播时才开放确定规则结果。

### 阶段 B：国内 Provider 契约缺口按数据域收敛

不要再对所有 Provider 做无差别全量 Review；每批只处理一个数据域，并根据迁移矩阵中实际缺失的行为选取任务。

1. **日 K 主链路**：逐项收敛日期范围与分页、成交量/成交额单位、空值、停牌日、复权口径/算法标识、来源时间和跨源回退。请求口径不一致时必须继续回退或显式失败，不得把不同来源拼接成连续序列。
2. **报价及证券基础信息**：统一证券代码、交易所/板块归类、源端时间、未知字段、退市证券查询与回退质量；只修复契约不一致项。
3. **市场统计与榜单**：继续保留市场广度覆盖率、行业榜估算口径、涨跌停池总数/截断信息、榜单分页和报告日期边界。SW1 成分股涨跌幅均值只能标为估算，不得宣传为供应商原生行业指数涨幅。
4. **财务与补充数据**：核对公告日期、报告期、单位、缺字段、端点权限及分区质量隔离；优先修复会误导分析结果的缺失值或日期语义，不为可选字段缺失机械降级。
5. **Efinance/AkShare 对等迁移**：以 Python 原方法和 Go 路由对照表为准，只迁移真实缺失的参数、字段或组合语义；已经由同一公开上游覆盖的同名封装不再重复造 Provider。

每个数据域的交付物是：代码差异修复、可复现的本地 fixture/单元测试源码、简短的契约/迁移矩阵更新。供应商授权、账号权限、线上可用性和实际覆盖率进入集中验收，不在每轮开发中反复发起真实请求。

### 阶段 C：严格历史时点（PIT）能力

1. **公司行动/复权因子**：保留因子来源、原始版本、生效日期、复权算法及参考日期；只有可重建同一复权参考时点的完整序列才可参与 PIT 查询。
2. **指数成分历史有效区间**：区分报告日期与真正成员生效日期；最近一次权重报告不能冒充任意历史时点成分。来源只能提供报告日时，明确限制，不推算有效区间。
3. **财务修订和版本存储治理**：文档化系统实际观察历史的起始边界、保留策略、磁盘增长监控和清理约束；不能恢复的历史修订继续明确标注不可回放。

### 阶段 D：开发收敛后统一验收

在阶段 A–C 的实际代码缺口收敛后，再一次性运行根 Go 模块、`go-core`、`services/market-data` 的 gofmt/test/race/vet，Docker Compose 与镜像构建，随后再进行目标部署的 MariaDB/Redis、服务健康、国内 Provider smoke test 及跨源对账/回测。回测重点包含幸存者/前视风险、历史状态和特殊规则日期、复权基准差异、数据缺失传播、来源回退及质量状态是否被错误抬高。

当前仓库 `.github/workflows/go-test.yml` 已声明 `workflow_dispatch`，但提交附带 `[skip ci]` 不会触发自动检查；如需提前手动触发，应在 GitHub Actions 页面运行。当前自动化连接没有 workflow dispatch 写操作，因此本执行通道不能声称已启动该工作流。真实 Provider 探测与部署环境验收仍独立于 CI。

### 每轮执行约束

- 写入前核对目标分支 HEAD；创建提交后仅通过带 `expected_sha` 的非强制分支更新推进，遇到并发更新立即停止并重新检查。
- 代码/测试源码与文档分开提交；提交说明使用中文并以 `[skip ci]` 结尾。
- PR #1 保持 Open / Draft，不合并、不关闭；不创建其他分支。
- 每轮汇报实际修改、提交 SHA、当前 HEAD、PR 状态、测试/CI 的真实状态和下一项功能任务；没有运行的测试不写成通过。


## 11. 本轮开发记录（2026-10-29）

### 历史名称不再推断普通退市状态

- `delistingStatusFromHistoricalName` 仅对名称中明确包含“退市整理”“退市”或“摘牌”的记录返回候选分类。
- 普通名称以及仅带 `*ST` 前缀的名称不再被解释为 `delisting_status=normal`，统一返回 `unknown`。ST 风险警示与退市状态是两个不同维度，不能互相替代。
- 该名称分类仍然只是线索，并非交易所权威退市事件；后续规则调用链必须保留该证据边界。
- 已更新对应单元测试源码。本轮未运行测试，不能将源码更新视为测试通过。

本轮业务代码提交：`01e235a96c70889c4d331407306271b2374ddcd0`。该提交未附带 `[skip ci]`，预期由 PR 的 `pull_request` 工作流触发 Go 测试与容器构建检查；最终状态以 GitHub Actions 实际结果为准。


## 2026-10-29 本轮执行记录：CI 阻塞修复与特殊事件证据契约

- **基本面指标映射**：`fundamentalsMap` 保留唯一指标的兼容别名，同时把带分组前缀的指标（例如 `income_statement.total_revenue`）也映射到快照顶层，并保留在 `metrics` 子映射中。GitHub Actions 在提交 `463b25aa` 对应的 Go suite 中确认 `go-core` 测试通过；根 module 也通过。此前失败不是数值类型错误，而是分组前缀指标未暴露在测试/调用方访问的顶层路径。
- **格式检查阻塞**：market-data 的 Docker 构建和测试在 `gofmt` 门禁处提前失败，影响 61 个 Go 文件。已将这些格式修复集中到单独的纯格式提交 `9747e8be`，不混入业务逻辑差异。该提交对应的 Go suite 当时仍在运行；需要以其最终结果确认格式门禁和镜像构建是否解除。
- **特殊交易事件证据契约**：新增 `PriceLimitEventEvidence` 和 `ResolvePriceLimitRuleWithEvidence`。特别事件必须匹配请求证券和有效日期，具有来源名称、HTTP(S) 来源地址、观察时间及公告标识；“确认没有特殊事件”还必须由来源明确保证覆盖完整。证据缺失、时间范围不匹配或覆盖不完整时返回 `unknown`，不输出可用涨跌幅比例。新增对应边界测试源码，当前尚未获得该提交的 CI 结果。
- **当前代码状态**：分支 HEAD 为 `bbda4b7ded8ba7221b282dedf8915fd25760ac75`；PR #1 仍为 Open / Draft，未合并。`9747e8be` 的 Live provider probe 和 WebUI checks 已通过；Go suite 尚待最终状态。新的证据契约提交已触发新一轮 Actions，不能提前标记测试通过。
- **仍未闭环**：该证据契约提供校验入口，但尚未接入有权威历史事件覆盖的真实 Provider/数据库，也没有将其作为公开 HTTP 规则 API。北交所上市交易日序号、重新上市/退市整理首日、复牌首日和临时停市的权威事件源仍需逐项确认；缺少来源时继续保持未知。


## 2026-10-29 涨跌幅规则接入历史交易状态快照

- 新增 `ResolvePriceLimitRuleFromTradingStatus`，将证券历史交易状态快照与独立的特殊交易事件证据接入同一规则解析入口。
- 先校验证券代码与交易日期一致；交易日状态未知或明确为非交易日时，不使用传入的特殊事件证据生成确定涨跌幅结果，统一保留 `unknown` 并附加质量原因。
- 对已确认交易日，继续要求风险警示状态、上市交易日序号及特殊事件证据分别满足既有来源契约；不因事件查询缺失而默认“无特殊事件”。
- 新增服务层测试源码，覆盖完整交易日输入、未知交易日、非交易日及证券状态日期不一致。提交后 CI 状态待核实；此项仅完成服务层接入，尚未增加权威特殊事件 Provider、持久化表或公开 HTTP 规则接口。


## 2026-10-29 退市状态对涨跌幅规则的质量门禁

- `ResolvePriceLimitRuleFromTradingStatus` 现在检查退市状态证据：退市状态为空/未知时，不再仅凭普通板块政策输出确定涨跌幅；处于退市整理期时，也不再直接套用普通板块比例。
- 若存在与证券和交易日期匹配、通过来源契约校验的重新上市首日、退市整理首日或交易所指定特殊事件，则允许该事件自身决定对应的无涨跌幅结果；普通“无特殊事件”证据不能覆盖退市状态缺失或退市整理期规则缺口。
- 增加定向服务测试源码，覆盖退市状态未知、退市整理期及明确特殊事件。最新代码已触发 Go suite；测试/镜像构建最终结果待 GitHub Actions 返回，本记录不将其描述为通过。
- 当前仍没有权威退市整理期持续区间及其普通交易日涨跌幅规则 Provider，因此该类结果继续返回未知，不伪造来源或开放确定结果。
