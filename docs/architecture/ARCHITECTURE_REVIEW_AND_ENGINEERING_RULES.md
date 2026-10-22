# 架构 Review 记录与后续工程规范

> Review 日期：2026-10-21  
> 仓库：Kasign/stock_data_provider  
> 范围：仓库设计文档、Go 主流程与数据服务、Provider 接入、配置/部署、日志和任务步骤状态。  
> 这是持续维护的 Review 记录，不表示所有规划项已完成；每一项状态必须以代码、测试和部署验收为准。

## 1. 总体结论

**当前方向总体正确，但项目仍处于分域迁移阶段，不应宣称已经完成“全 Go 化”。**

正确的方向包括：

- Go 主 API、Go 核心流程、Go 行情数据服务与 Python bridge 在目录和 Go module 层面物理分隔。
- 通过明确的 HTTP 契约连接独立服务，不跨 module 导入对方的 internal 包。
- 按能力拆分 Provider 接口，不要求只支持一种数据类型的供应商假装支持所有数据域。
- 将来源、单位、复权、时间戳与质量原因写入标准数据契约，避免把“拿到了一串数字”当作有效行情。
- 保留 Python bridge 供尚未迁移的新闻、LLM、部分市场概况及兼容功能使用，不因行情 Go 化而误删非行情能力。

必须保持清醒的边界：

- 根目录的 cmd/analysis-api 已将实时行情与日 K 主采集切到 Go market-data HTTP API。
- Go 原生行情 Provider 当前有东方财富、腾讯，以及仅提供实时行情的新浪。
- 妙想 MX_API 被设计为显式 opt-in 的筹码/资金流补充源，不提供日 K，不参与行情 fallback。
- go-core 仍有 Python bridge 依赖；原 Python 数据源与各数据域尚未全部迁移。
- 自动化测试通过只能证明当前模块的代码契约与测试样本通过，不代表真实供应商网络、生产 API Key、MariaDB/Redis 权限或 Docker 部署已经验收。

## 2. 发现与处理记录

| 优先级 | 发现 | 风险 | 当前状态 / 处理原则 |
|---|---|---|---|
| P0 | 新增 Provider 与既有代码出现过 package 级重复 helper，导致 Go 模块编译失败 | 整个 market-data 服务无法构建 | 已删除重复 helper；新增源必须复用公共校验函数，并由 CI 编译全部 Go module |
| P1 | Provider 能力注册曾隐含假设每个 Provider 同时支持行情和 K 线 | 只能报价的源会被错误排除，或被错误声明支持 K 线 | 已拆分 NamedProvider、QuoteProvider、DailyBarProvider，并引入 SupplementProvider |
| P1 | 新增 data-sources 能力查询后，测试装配中 nil service 曾触发 panic | 能力诊断接口在不完整 wiring 场景下崩溃 | 已对 nil service 做防御处理；服务入口仍必须通过 New/依赖校验保证生产依赖完整 |
| P1 | 根 API 原日志主要是 printf，没有统一 request ID、状态码、响应字节数和结构化字段 | 请求和任务执行链路难以排查，日志不利于检索聚合 | 已开始将根 API 与 market-data HTTP 访问日志改为 JSON slog 结构化记录；还需要对齐 go-core 与各 Provider 的日志字段 |
| P1 | 已有 run_events/stage events，但 POST 分析接口同步执行 | 调用方通常要等任务结束才拿到 run_id，不能自然呈现运行中的步骤状态 | 已有可查询的事件基础；异步任务、任务 ID、排队状态、轮询/SSE 仍是下一阶段，不应误称实时进度功能已完成 |
| P1 | 两份迁移设计文档内容完全相同 | 两份文件逐渐漂移，修改无从判断权威版本 | 保留 docs/architecture/GO_MARKET_DATA_MIGRATION_PLAN.md 为唯一正文，旧路径改成兼容入口 |
| P1 | 英文 README/数据契约和一些旧中文说明仍有旧 Python API 接入表述 | 操作人员可能按过时环境变量部署，误以为主 API 仍依赖 Python 行情服务 | 已翻译部分服务说明和契约；剩余文档应统一为中文并区分“当前状态”与“目标计划” |
| P1 | 部分数据域尚无 Go 原生 Provider，却容易被概念设计误认为已覆盖 | 缺失数据可能被默认值或自选股样本伪装成完整市场数据 | 必须维护迁移矩阵；未实现的路由/能力要明确标为未完成，不得用 mock 或临时静态数据冒充生产数据 |
| P2 | MX 类自然语言查询响应可能有多种 table envelope，且金额单位和发布时间存在不确定性 | 误选表、金额单位放大、使用未来可知的当前快照进行历史分析 | 使用明确候选 envelope 和字段名；只识别明确的货币字段；完整度不等于时点可用性，缺少精确 available_at 时保持 degraded |
| P2 | 当前本地时钟用于判断当日 K 线是否完成 | 节假日、临时休市和异常交易日可能判断错误 | 在引入权威交易日历前须明确限制；未来由日历数据服务判定交易时段 |
| P2 | 请求/响应正文若被直接写日志，可能泄漏 API Key、用户自选股、公告查询内容等 | 密钥泄漏和数据范围过度暴露 | 日志只记 summary、counts、status、duration、质量标记；不记录 Authorization、API Key、完整请求/响应体 |

## 3. 后续架构必须遵守的原则

### 3.1 模块边界

- services/market-data 是行情/标准化/供应商接入的权威边界。
- 根目录 analysis API 是一套独立 Go API 和分析主流程。
- go-core 是另一套独立 Go module，暂时通过 python_bridge 调用尚未迁移的 Python 能力。
- Python bridge 只能作为过渡的明确适配边界；不得在多个调用方重复手写 Python 服务 URL、鉴权和 JSON 解析。
- 各 Go module 不得跨 module 导入 internal 包；通过版本化 HTTP JSON 契约协作。
- 数据适配器不应直接依赖 HTTP handler；HTTP 层不应包含供应商解析/单位换算逻辑；业务分析不应猜测原始数据单位。

### 3.2 Provider 的能力边界

每个 Provider 只能声明它确实实现并通过测试的能力。建议持续采用能力接口，如：

- NamedProvider：名称与诊断身份。
- QuoteProvider：实时行情。
- DailyBarProvider：历史 K 线。
- SupplementProvider：筹码/个股资金流等补充数据。
- 后续按域增加 IndexProvider、MarketBreadthProvider、SectorRankingProvider、FundamentalsProvider、LimitUpPoolProvider、DragonTigerProvider、NewsProvider 等。

不能用一个全局优先级把所有数据域混在一起。Quote fallback 不等于 daily-bar fallback；一个源的财务授权失败不应把其行情能力判定为不可用；只支持补充数据的源不能偷偷进入日 K 主链路。

### 3.3 数据来源、单位与时间

所有规范化数据应按可用情况保留：

- 底层 source、source_version、请求/抓取/标准化时间。
- provider_timestamp 与本地 fetched_at 的区别。
- raw_volume/raw_volume_unit、raw_amount/raw_amount_unit。
- 标准单位、复权基准、quality_status、quality_reasons。
- fallback_from、fallback_reason、request_id/run_id。

禁止事项：

- 不得用 fetched_at 伪装 provider_timestamp 或 publication available_at。
- 不得将缺失字段转为 0；不知单位就标为 unknown/degraded。
- 不得只因为某字段有数值就认为字段质量完整。
- 不得把自选股列表的涨跌家数当作全市场广度。
- 不得把今天可查询的筹码、财务、行业或复权后的历史结果倒灌到历史 as_of 回放中。
- 不得合并不同复权口径的 K 线计算同一组技术指标。

### 3.4 配置与密钥

- 新 Provider 默认关闭或显式 opt-in；启用后必须验证 API Key、URL、超时、重试和能力开关。
- 禁止将真实密钥、数据库密码或 API Key 放进 Git、README、测试日志或 CI 输出。
- 环境变量名称需要清晰、模块化，旧变量应明确写兼容周期；不得在多个地方维护互相冲突的默认值。
- 数据库连接优先采用服务级拆分字段：行情服务使用 `MARKET_DATA_DB_*` / `MARKET_DATA_REDIS_*`，分析 API 使用 `DB_*` / `REDIS_*`；由服务代码构建驱动 DSN，不要求部署人员重复拼接用户名、密码、主机、库名和连接参数。
- 兼容旧环境变量时必须明确迁移窗口和回退优先级。只有新字段完全未设置时才允许读取旧 `MARKET_DATA_MYSQL_DSN`；一旦新字段开始配置，不能再静默混用两种配置来源。
- 若 Compose 的 `env_file` 已传递应用所需的连接字段，就不要再维护只用于覆盖相同字段的 `GO_DB_HOST` / `GO_REDIS_ADDR` 等别名变量。
- Docker Compose 应复用既有 MariaDB/Redis，不因新增 Go module 就再开一套重复的生产数据库。
- 配置无效应 fail-fast；不能把错误值静默降级为默认值后继续运行。

## 4. 同花顺数据服务接入计划

参考仓库：[Kasign/Financial-API](https://github.com/Kasign/Financial-API)。截至本次 Review，该仓库为公开仓库，README 将其描述为同花顺金融数据服务接入项目，并提供 REST API、MCP、CLI、Python 等调用方式；本仓库只把它视作候选上游/接口参考，不把它当作已集成组件，也不假设每个上游能力都可在当前账户调用。

分阶段建议：

1. **接入调研**：固定参考仓库 commit；核对接口文档、API Key/订阅权限、速率限制、返回单位、分页、数据时点和服务条款；不能以仓库 README 替代真实账户权限验证。
2. **能力映射**：优先评估最新行情和日 K；其次评估指数/板块、财务、交易日历、涨跌停/炸板/连板/龙虎榜等特定域。新闻原文、分钟 K、tick、海外、宏观或研报等能力只有经文档确认后才列入支持范围。
3. **独立适配器**：新增 Ths/Tonghuashun provider 模块和独立配置（例如 THS_BASE_URL、THS_API_KEY、THS_TIMEOUT）；默认关闭，且不与现有供应商共享密钥。
4. **单位与时间对照**：用同一股票、同一交易日将 THS 与已验证数据源做差异核对，验证成交量/额单位、复权、报告期与披露日、时间戳语义、缺失字段和错误码。
5. **按能力接入**：只把已通过对照测试的能力加入相应 Provider 链；若某 endpoint 只提供日终快照，不得注册为盘中实时行情能力。
6. **灰度与回滚**：保持其 opt-in，输出来源/质量/fallback 诊断；对比错误率、延迟、可用率、单位差异后再调整优先级。

当前不承诺在本轮直接完成同花顺生产接入；该事项已列入迁移路线图。

## 5. 结构化日志与任务追踪

详细方案见 [日志与任务步骤进度方案](./LOGGING_AND_TASK_PROGRESS_PLAN.md)。

方向要求：

- 控制台日志采用 JSON 结构化字段，并统一 request_id、run_id/task_id、service、component、operation、status、duration_ms 等字段。
- HTTP 日志需有状态码、耗时和响应字节数；Provider 日志需有 provider、dataset、fallback、cache_hit、质量原因和错误类型。
- 任务每一步的状态同时作为可查询的 stage event 持久化；stdout 日志不能替代历史任务事件。
- 输入和输出日志只记录经过脱敏的 summary/字段计数，不默认打印完整请求体、行情列表、LLM 提示词或响应正文。
- 接入异步任务状态时应先返回 task_id，再通过任务状态和事件接口/事件流展示进度；不要让调用方只能等待同步 HTTP 请求结束。
- 状态机必须定义 queued、running、succeeded、partial、failed、cancelled、retrying 等状态之间的合法转换，以及重试/取消/超时语义。

## 6. 修改与 Review 规则

以后每次改动按一个完整功能模块提交，而不是改一个文档就提交一次：

1. 先确认唯一设计文档、接口契约、迁移矩阵和当前状态，写清本次任务范围与不做什么。
2. 一次修改覆盖所需的接口、配置、实现、测试、部署变量和中文文档；不要只加入 Provider 文件却没有注册、HTTP 路由、校验和测试。
3. 对外 API 的 method/path/query/body/status/error/schema 均需契约测试；改现有字段必须写兼容说明或版本边界。
4. 最少运行根模块、go-core、services/market-data 各自的 go test、race 和 vet。更改容器部署时再运行 Compose 配置和容器 smoke test。
5. 新增源必须提供真实响应 fixture 或脱敏样例，覆盖成功、空数据、错误状态、限流、超时、未知单位/时间和缺字段。
6. 写中文注释和运维说明，注释应解释边界、单位和不变量，不要逐行重复代码。
7. 一批功能收尾后统一提交，并附上变更清单、测试证据、限制和回滚方法。

本文件与迁移矩阵是后续开发警示清单；发现新的高优先级问题时，追加一条有状态的记录，并确保修复和验收证据可追踪。
