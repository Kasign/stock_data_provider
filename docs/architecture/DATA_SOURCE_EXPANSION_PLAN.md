# 数据源扩展计划与真实验收清单

> 文档版本：V1.0  
> 制定日期：2026-10-22  
> 适用仓库：Kasign/stock_data_provider  
> 原则：Go 原生采集优先、免费/低成本优先、按数据能力分流、真实验收后才启用生产路由。

## 1. 决策摘要

**BaoStock 原生 Go 协议客户端与 DailyBarProvider 已完成一轮真实 NAS 容器验收，真实日 K 查询、字段归一化、多日范围与多复权口径样本均已通过。** 故障回退与 MariaDB 不复权历史回退也已通过实际请求验证；这并不代表所有数据源或所有证券/日期均已完成验收。当前真实测试细节与待处理问题统一记录在[数据源真实环境验收与故障排查记录](./DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md)。

BaoStock 最合适的第一阶段用途是历史日 K 及其明确声明的成交量、成交额和复权字段；后续再评估其支持的部分财务指标、复权因子、除权除息、证券基础资料、行业分类和指数成分数据。它不能替代实时行情、新闻公告、龙虎榜、板块资金流和筹码数据等全部能力。

实施遵循以下约束：

1. Provider 的能力按域注册；不要求只提供历史 K 线的源实现 QuoteProvider。
2. 当前 providers.json 和环境样例默认将 BaoStock 注册为日 K 备用源（MARKET_DATA_BAOSTOCK_ENABLED=true）；NAS 与 GitHub Runner 均已有真实日 K 返回证据。继续扩大交易日、证券类型、分页、停牌/无数据和故障恢复等验收范围；目标部署环境可用环境变量显式关闭。有限样本不能等同于生产 SLA 或全范围稳定性保证。
3. 同一响应中的 K 线必须来自单一 Provider、同一请求复权口径；不得拼接不同源或把未复权数据标为前复权。
4. 统一的 forward_adjusted / backward_adjusted 只描述复权方向，不足以表示各数据源的复权算法。对复权序列还应记录具体的 adjustment_method 与可确认的参考日期。
5. 免费公共服务不等于承诺 SLA 或无频率限制。应使用有限并发、连接复用、缓存、退避及可观测的 Provider 熔断，不对未知的服务端限制自行编造数字。
6. 生产数据采集与分析主流程使用 Go 原生实现。AKShare、Efinance 等 Python 项目可以作为字段和行为对照来源，但不应变成新增 Provider 的隐式 Python 运行时依赖。

## 2. 当前环境已确认与未确认事项

| 检查项 | 当前证据 | 结论 |
|---|---|---|
| BaoStock DNS | NAS 宿主机解析到 114.94.20.42 | DNS 当前可用，地址可能变化 |
| BaoStock TCP | NAS 宿主机连接 public-api.baostock.com:10030 成功 | 当前宿主机网络可达，尚未证明应用层可用或长期稳定 |
| BaoStock 容器网络及协议 | 容器内已完成实际 BaoStock 日 K 请求，返回多证券/日期记录 | 已通过首轮真实请求，需继续扩大样本并观察稳定性 |
| BaoStock 登录及 K 线查询 | 已在实际服务请求中获得真实日 K，source=baostock，quality_status=valid | 首轮通过；不代表 SLA 或全部证券范围保证 |
| 东方财富实时行情与历史 K 线 | 容器访问百度 HTTPS 为 200；Eastmoney 两个域名根路径的 HTTPS 可建立连接并返回 404；历史 K 线真实接口对部分解析到的 IPv4（101.42.128.173、61.129.129.199）返回 Network unreachable；宿主机根路径访问 14.103.188.89 成功建立连接并返回 404 | 故障保持待调查；目前证据不能证明域名整体被封，详细证据及排查步骤见[真实验收与故障排查记录](./DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md) |
| 腾讯贵州茅台日 K | 原始响应含 qfqday，Go 解析得到 OHLC 与成交量正确 | 已验证一个普通 A 股样本 |
| 腾讯中芯国际前复权 | 两个腾讯入口对本次请求均只返回 day、未返回 qfqday；adjustment=none 的 Go 结果与原始成交量一致 | 本次前复权要求不满足；未复权解析与科创板成交量单位转换通过 |

东方财富历史行情当前在目标环境中存在连接阶段异常；由于真实行情接口尚未收到完整响应，本轮不能断言这是 JSON 解析问题或数据不支持。应保留错误细节和健康状态，不能把“当前请求失败”自动推断为“服务商永久不支持该股票”。

## 3. 扩展数据源分工

### 3.1 按数据域建议的角色

| 数据域 | 当前/候选来源 | 计划角色 | 接入限制 |
|---|---|---|---|
| 实时个股行情 | 腾讯、新浪、东方财富、Yahoo Finance（可选） | 保持现有 QuoteProvider；Yahoo 采用 Chart 1m 元数据与分钟开盘，默认关闭，因缺成交额固定 degraded | BaoStock 不作为实时行情源；东方财富在当前 NAS 上的访问故障尚未定位 |
| 日线历史 K 线 | 东方财富、BaoStock、腾讯、新浪（仅不复权）、Yahoo Finance、网易财经 CSV（可选） | BaoStock 作为默认历史 K 线备用源；腾讯按复权口径兜底；新浪/网易只在 `none` 请求下使用；Yahoo 与网易默认关闭，完成目标网络验收前不进入默认回退链 | 验证交易日覆盖、成交量单位、成交额/价格关系与复权口径；新浪/Yahoo 缺可核验成交额，网易金额无法交叉验证时必须置 null/unknown/degraded |
| 周线/月线历史 | BaoStock 候选 | 在日线 Provider 验收后扩展 | 单独测试聚合周期、最后交易日规则和日期覆盖 |
| 部分财务指标 | BaoStock、后续可选 Tushare / THS | BaoStock 先覆盖官方文档有明确定义的财务数据集；Tushare/THS 依权限和成本另行评估 | 报告期、公告日、数据可用时间、单位和缺失值必须分别保存 |
| 复权因子、除权除息 | BaoStock 候选 | 为复权检查、公司行动追踪和数据源对账提供补充 | 不得只以相同的前/后复权标签认定两个源的绝对价格等价 |
| 股票基础资料、行业分类、部分指数成分 | BaoStock 候选 | 历史 K 线稳定后逐个独立能力接入 | 行业分类日期与指数成分生效时间要保留；不得把当前成分股倒灌为历史成分 |
| 市场广度、行业/概念排名、涨停池、龙虎榜 | 后续评估 Tushare/THS 与可核验的公开来源 | 作为独立 Provider 能力实施 | BaoStock 不应被假定能够覆盖这些域；逐个端点核对权限、字段与更新时点 |
| 新闻与公告 | 现有过渡 bridge、未来原生 Go 数据源 | 建立独立 NewsProvider / AnnouncementProvider | 保存标题、来源、发布时间、抓取时间与可用时间；不能把搜索命中时间当作发布时间 |
| 筹码、个股资金流 | 现有可选 MX_API；未来按需评估其他来源 | 保持 opt-in 补充能力 | 先验证字段标签、金额单位和时间可用性；不能直接作为无降级的历史数据 |

### 3.2 其他候选源的定位

- **Yahoo Finance：Go 日 K 与实时快照适配器均使用公开 Chart API，默认关闭。** GitHub Actions 已实测日 K 返回；实时快照以 `1d/1m` 序列提取当日开盘，quote 时间来自 provider 元数据。两种能力均因缺少可核验成交额固定 degraded，不可用于金额敏感分析；Chart API 没有 SLA，也不等同于 YFinance Python 库的完整能力。
- **新浪历史日 K：与实时行情端点分开接入，支持可配置公开 JSON endpoint。** 当前只接受 `none` 口径；接口不回显证券代码且没有经验证的成交额，固定 `degraded`，历史响应最多 1023 条，不能用作复权数据源。
- **网易财经历史 CSV：新增 Go adapter，默认关闭。** CSV 可请求成交量和成交金额字段，但该接口并非有 SLA 的官方开发者 API；适配器仅在成交额/成交量推导的成交均价落在当日 OHLC 区间附近时才接受金额单位；不匹配时 amount 保持 null / unit unknown / degraded。仅支持 `none`，在真实 Runner 与部署端点验收完成前不加入生产默认回退链。
- **Tushare Pro：可选、有权限条件的补充源。** 官方积分权限表显示，120 积分档只允许非复权日线等有限接口；财务、指标及其他数据需要更高积分或单独开通权限。因此，不把它作为“免费即可完整覆盖所有数据域”的默认方案。只有先确认具体账号的接口权限、请求频率和成本，再决定是否接入。
- **AKShare / Efinance：以参考、差异对照为主，不直接作为 Go 服务的 Python 运行依赖。** 它们是采集/数据接口封装库，不等同于单一上游服务；不同函数可能依赖不同网站接口。若未来有必要，可针对已核验的上游接口实现独立 Go Adapter。
- **Pytdx：保留为后续研究候选。** 在测试实际可用的行情服务器、连接稳定性、交易日覆盖和协议兼容前，不进入生产优先级。
- **THS Financial-API：继续保留为可选补充计划。** 先核实真实 API Key、数据集权限、速率与使用条款；不得把仓库存在等同于生产账户有权限。
- **Longbridge / Futu / Finnhub / Alpha Vantage：不列为当前 A 股免费主链路。** 只有需求扩展到相应市场、确认账户权限与预算后才评估。

### 3.3 Provider 路由应按能力配置

现有全局 Provider 顺序只能作为兼容默认值。后续应逐步支持按能力独立设置优先级，例如：

- 实时行情：QuoteProvider 专属优先级；
- 日 K：DailyBarProvider 专属优先级；
- 指数/市场广度、财务、行业排名、新闻公告分别维护各自能力路由。

当前日 K 配置顺序为“东方财富 → BaoStock → 腾讯”。路由必须依据本次请求的复权需求、Provider 实际能力与数据质量做准入；某源没有返回所需的 qfq/hfq 序列时，应继续尝试其他满足同一口径的 Provider，绝不能以未复权数据冒充复权数据。

## 4. BaoStock 接入方案（Go 原生）

### 4.1 第一阶段范围

优先只实现历史日 K 的 DailyBarProvider。建议请求并保存 date、code、open、high、low、close、preclose、volume、amount、adjustflag、turn、tradestatus、pctChg 等可用字段；具体字段组合以官方 API 当前支持为准。

官方 Python API 文档列出的复权标记为：1 后复权、2 前复权、3 不复权；日 K volume 单位为股、amount 单位为人民币元。官方文档也说明其复权方法与同花顺、通达信等可能不同。实现时应把 Provider 的复权算法标识保留为独立元数据，不能假定跨源绝对价格完全相等。

### 4.2 Go 客户端实现要求

BaoStock 官方维护的公开 API 使用 Python 客户端。仓库目标是 Go 原生运行，因此计划实现独立的 Go TCP 客户端，而非在生产请求路径中执行 Python：

1. 明确请求/响应帧边界、版本、消息类型、消息体长度、CRC、编码与压缩响应；协议细节须以官方客户端源码和真实响应交叉验证。第三方协议文档只能作为参考，不是官方兼容保证。
2. 封装连接、登录、查询、分页、登出、重连和连接关闭，明确每个阶段的 deadline 与错误类型。
3. 处理上游错误码、空结果、数据截断、压缩帧、坏 CRC、未知响应版本和上下文取消。
4. 登录会话与连接复用由客户端内部管理；并发查询必须有明确的连接互斥/池化策略，避免共享 socket 响应串线。
5. 所有网络调用都受超时、有限重试和并发上限约束；日志不包含密码、完整原始载荷或敏感凭据。
6. 将协议实现与 DailyBarProvider 适配器分层，协议客户端可单独测试，不把协议细节散落到业务路由中。

### 4.3 配置规划

通过环境变量控制启停与连接，不硬编码服务器地址：

- MARKET_DATA_BAOSTOCK_ENABLED
- MARKET_DATA_BAOSTOCK_DAILY_BARS_ENABLED
- MARKET_DATA_BAOSTOCK_HOST（默认 public-api.baostock.com）
- MARKET_DATA_BAOSTOCK_PORT（默认 10030）
- MARKET_DATA_BAOSTOCK_CONNECT_TIMEOUT
- MARKET_DATA_BAOSTOCK_REQUEST_TIMEOUT

当前 Go 客户端使用单个长连接并通过互斥门禁串行化请求，实际并发上限固定为 1，以避免共享 socket 的响应串线；因此暂不提供 `MARKET_DATA_BAOSTOCK_MAX_CONCURRENCY`。未来若实现独立连接池，再增设该变量并覆盖连接池并发测试。

### 4.4 集成与质量验收

分为两个门槛：

**网络/协议门槛**

- 从 NAS 宿主机和 stock-market-data 容器分别验证 DNS、TCP 连接。
- 使用 Go 客户端执行真实登录、查询、登出；至少查询 600519.SH 与 688981.SH。
- 验证 1 次请求中完整响应、分页、空结果与上游错误码处理。
- 以受控低频验证重连、超时及连接关闭；不要通过高并发压测未知的公共服务限制。
- 记录实测时间、成功/失败次数、耗时、返回行数和错误类别；未通过时不注册为可用生产源。

**数据契约门槛**

- 对 600519.SH、688981.SH 请求 2026-09-02 至 2026-10-21 的日 K，分别测试前复权、后复权和不复权。
- 验证日期、OHLC、成交量、成交额、调整标记、停牌记录、排序、重复日期与覆盖率。
- 原始 volume 以 shares 处理，amount 以 CNY 处理；金额缺失必须保持 null，不能用价格乘成交量估算。
- 复权结果与其他来源对账时，先统一交易日和复权算法。优先比较未复权价格、成交量/成交额、收益率和因子关系；不要因为复权后绝对价格数值不同就立即判定某一方错，也不要把差异默认为正常而忽略检查。
- 对非交易日、停牌、退市或不支持证券、无数据区间与被限制 IP 等情况建立确定的回归测试。
- 任何请求只允许返回与请求口径一致的完整序列；不得把 day 数据标为 qfqday，不得跨源拼成看似连续的 K 线序列。

本轮另外将重复交易日和 OHLC 区间关系作为生产适配器的数据契约：同一请求里出现重复日期，或 low/high/open/close 不满足日 K 合理区间时，整批返回错误，不将异常数据标为 valid。相应表驱动回归测试已加入，见本分支的 `services/market-data/internal/provider/baostock_test.go`。

BaoStock 已注册为 DailyBarProvider 的日 K 备用源；其财务、基础资料、行业和指数成分接口仍需另开任务逐域验收。

## 5. 分阶段实施顺序

| 阶段 | 任务 | 验收条件 |
|---|---|---|
| 0 | 保持当前服务稳定，保留东方财富故障诊断与腾讯复权校验 | 不降低现有数据质量门禁，不更改未复权/前复权语义 |
| 1 | Go 原生 TCP 协议客户端（已实现） | 已新增本地 TCP 集成回归覆盖分页、空末页、上游错误码及异常断线后下一次调用重新登录；登出流程也纳入测试。GitHub Go CI #202 通过。模拟测试不能替代目标容器的真实网络验收，仍需扩大真实数据样本并观察故障恢复。 |
| 2 | Go BaoStock DailyBarProvider、配置与单元测试（已接入并默认作为日 K 备用源） | 已有 600519.SH、688981.SH、000001.SZ、600036.SH 的真实样本，以及多复权和故障回退结果；继续扩大日期覆盖并完成停牌/非交易日/异常数据专项验收 |
| 3 | 更新 Provider 配置、数据源能力列表、健康状态和逐能力路由 | 未启用、超时、无数据与复权能力不匹配时能正确回退并解释原因 |
| 4 | 增加复权算法元数据并做来源一致性检查 | 能识别 BaoStock 与其他源不同的复权算法，不跨源拼接 |
| 5 | 接入 BaoStock 支持的财务/基础资料/行业/指数能力 | 每种能力有独立模型、Provider 开关、单位、时间与数据质量测试 |
| 6 | 逐项评估 Tushare/THS 和市场广度、板块、龙虎榜、新闻公告等域 | 逐项确认权限、费用、更新时点和接口契约后再实施，不一次接入所有能力 |
| 7 | 按迁移矩阵移除确实已被替代的 Python 数据源依赖 | Go 覆盖、真实环境验收和兼容对账通过后才删除对应 bridge |

## 6. 发布决策

BaoStock 的 Go 客户端和 DailyBarProvider 已实现，并已在 NAS 容器完成首轮真实日 K 取数与部分逐日口径验收；当前状态是“真实样本通过，仍需扩大生产验收范围”，不是“已经证明生产稳定”。

发布前必须确认：

- 服务端实际请求可以从 Docker 容器内完成，而不仅仅是 NAS 宿主机；
- 源支持当前请求的标的、周期与复权口径；
- 字段单位和空值语义有文档/实测依据；
- 请求失败不会返回另一复权口径的伪装数据；
- 访问规则、低频策略、熔断和健康状态已经实现；
- CI 测试、真实上游集成测试和部署环境 smoke test 分别通过。

### 当前代码实现状态（2026-10-22）

Go 原生 TCP client 与日 K Provider 已完成首轮真实容器取数：600519.SH、688981.SH、000001.SZ、600036.SH 的样本查询通过，返回 source=baostock、quality_status=valid 的结果，并已验证不复权数据库回退与多源故障回退。真实测试输出、数值样本与故障注入证据见[数据源真实环境验收与故障排查记录](./DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md)。

BaoStock 保持有限范围验收状态；其他 Provider 仍需独立测试。Eastmoney 接口网络异常原因未定位，不能据此判断域名整体被封。

启用验收建议：

1. BaoStock 真实查询已有首轮通过证据；继续增加日期区间、证券类型、停牌/无数据日期与复权样本，并保留低频请求。
2. 检查 source=baostock、复权算法标签、成交量/成交额与质量警告；对比时区分未复权价格和不同来源的复权算法。
3. 保持 Eastmoney 根因待调查，不因失败绕过数据质量门禁。
4. 任何扩展都要在真实请求、质量校验与 CI 通过后再评估路由；尚未验收的数据域不得标记为生产可用。

### 协议与数据契约回归补充（2026-10-22）

本轮代码提交 `ae451a3b55d99c2cddf58249b2c8e054b5deb52d` 作为一批提交，包含 BaoStock 日 K 适配器的重复日期/OHLC 防护，以及本地 TCP 集成测试。自动化覆盖分页和空末页、上游查询错误返回、断线后下一次独立调用重新连接登录/查询/登出。Go CI #202 全部通过；GitHub Runner 真实探测仍有独立的上游访问失败，详见[真实环境验收记录](./DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md)。这代表协议测试覆盖已加强，不代表 BaoStock 在所有网络环境都稳定。

### 多源行情契约与验收补充（2026-10-23）

本轮在 BaoStock 之外，继续收紧新浪、腾讯、东方财富的报价和日 K 数据契约：

- 新浪、腾讯、东方财富报价在 Provider 返回前统一调用 `normalize.ValidateQuote`，拒绝非有限数值、关键价格缺失、OHLC 不一致及成交量单位不符合标准的结果。报价源返回不完整字段时不能简单标记为 valid。
- 腾讯和东方财富日 K 走共享归一化流程：验证/排序交易日期、剔除重复日期、请求范围外日期、非法 OHLC 和不可解析行，强制遵守返回条数上限。若上游同批次存在坏行，保留的结果会明确降级并附质量原因；腾讯未确认的日 K 成交额继续保持 null/unknown/degraded，不推断尾部字段为成交额。
- 东方财富报价仅允许 HTTPS 下的 `eastmoney.com` 官方域名重定向，限制跳转次数；拒绝外部域名跳转及 HTTPS 降级到 HTTP。主报价从 `push2.eastmoney.com` 跳到 `push2delay.eastmoney.com` 时可继续请求，但公开端点仍有瞬时 502，不能把一次成功认定为稳定可用。
- BaoStock 日 K 现在与其他源一致执行 `limit` 上限并按交易日升序返回；真实探测中以显式日期区间请求 8 根的回归问题已修复。

代码变更按功能分组提交：[行情契约与东方财富重定向](https://github.com/Kasign/stock_data_provider/commit/91eff930fcfee919f3707658f697dedba8156369)、[修正前复权测试样本](https://github.com/Kasign/stock_data_provider/commit/f5725e1a606ee2f00aaa434dc63e5b28f76c9031)、[BaoStock 条数上限](https://github.com/Kasign/stock_data_provider/commit/3e94fdd353320b8059382604a1afc0c738206520)。[Go CI #206](https://github.com/Kasign/stock_data_provider/actions/runs/39885215590) 全部通过；[真实 Provider 探测 #15](https://github.com/Kasign/stock_data_provider/actions/runs/39885215595) 的探测 job 通过。按第 5.13 节保留真实端点结果和质量状态。

#### 东方财富报价时间戳兼容（2026-10-23）

真实报价 payload 中，`f86` 除了常见的 `YYYYMMDDHHmm` / `YYYYMMDDHHmmss` 字符串，也出现过数字型 Unix 秒时间戳。提交 [`619522a6`](https://github.com/Kasign/stock_data_provider/commit/619522a62a69e73b566336e9ec19584be00bb26e) 让解析器兼容 10 位秒和 13 位毫秒时间戳，并检查时间值的合理范围；对应测试覆盖原格式、秒/毫秒转换和完整报价契约。最新 [实时探测 #17](https://github.com/Kasign/stock_data_provider/actions/runs/39885682979) 中，东方财富主报价经历 HTTP 302 后跟随官方 HTTPS 重定向到延迟域 HTTP 200，报价时间成功解析，quality 从缺少时间戳时的 degraded 恢复为 valid；延迟域直连也返回 valid。更早的探测仍出现 502，因此继续将上游稳定性视为待观察项，而不是认为单次成功等于稳定。

## 7. 参考资料

- [BaoStock 官方网站](https://www.baostock.com/)
- [BaoStock Python API 文档（历史 K 线、财务、复权因子、行业等）](https://github.com/baostock/baostock/blob/master/docs/pythonAPI.md)
- [BaoStock 官方 A 股 K 线数据说明](https://www.baostock.com/baostock/index.php/A%E8%82%A1K%E7%BA%BF%E6%95%B0%E6%8D%AE)
- [BaoStock 协议社区参考（非官方，实施时须与官方客户端源码及真实响应核对）](https://github.com/millken/baostock/blob/main/BAOSTOCK_PROTOCOL.md)
- [Tushare Pro 积分与频次权限表](https://tushare.pro/document/2?doc_id=290)
- [Kasign Financial-API（同花顺候选参考）](https://github.com/Kasign/Financial-API)

## 8. 国内原生 Provider 统一接线（待集中验收，2026-10-26）

本批次按“国内数据源优先，海外来源暂不请求”的要求调整默认配置：
- 默认优先级去掉 Yahoo Finance；Yahoo 代码保留但 `enabled=false`，其行情/日 K 不会进入默认请求链。
- 东方财富默认开启行情、日 K、证券基础资料、行业/概念排名、指数、市场广度、涨跌停池、所属板块、龙虎榜和公告。
- BaoStock 默认开启历史日 K、交易日历和季度基本面；仍不把它当作实时行情源。
- 腾讯、新浪和网易财经纳入国内行情回退配置；新浪/网易的历史日 K 仅支持不复权口径，无法匹配请求的复权口径时必须返回错误并允许路由继续回退。
- 妙想仍是需要显式 API Key 的补充源，不伪造凭据或在没有 Key 时开启；新闻搜索的 Bocha / SearXNG 也继续由各自凭据/URL 配置控制。
- 本批先统一已有国内 Provider 的默认接线，再陆续新增 Tushare、TickFlow 与 Pytdx 原生 Go 适配器；代码均尚未统一测试，真实上游可用性、复权/单位契约和目标部署连通性统一留到后续测试。Efinance/Akshare 是 Python 多端点包装，当前仅覆盖其部分共享上游能力，未做逐函数等价迁移。

## 9. 同花顺官方 A 股数据源首批接入（2026-10-26）

- 默认服务地址为 `https://fuyao.aicubes.cn`，由 `MARKET_DATA_HITHINK_BASE_URL` 配置；API Key 从 `MARKET_DATA_HITHINK_API_KEY` 或 `HITHINK_FINANCE_API_KEY` 读取，只通过 `X-api-key` Header 发送。
- Go 适配器覆盖官方行情快照和历史日 K 端点。日 K 支持 `none` / `forward` / `backward`，时间范围按小于十年的区间分段，offset 分页设置安全上限。
- 指定单股行情接口不提供可用快照时间，所以返回的 Quote 会标记 `degraded`；缺失成交量/成交额、非法 OHLC、代码不匹配、越界日期及业务错误不会伪装成有效数据。
- 同花顺是凭据和调用额度控制的可选国内源，默认禁用；财务报表、估值、交易日历、指数/板块和特色数据仍需后续按能力接入。
- 本次只接入代码、配置与文档，没有运行测试；统一测试阶段再核实账号权限、分页、额度与真实数据质量。

### 同花顺财务指标能力补充（2026-10-26）

- 新增 `FundamentalsProvider` 对接：`GET /api/a-share/financials/indicators?thscode=...&report=YYYY-Q`，报告期可由 year/quarter 精确映射。
- 五类能力块按供应商实际 `abilities[]` 结构解析，不假设它是以类别为键的对象。空值和非数值不补零。
- 上游响应未提供可靠披露时间与每个指标的单位，字段保留供应商 `index_id` 作为 key/source_field，并将单位标记为 `provider_defined`；整体质量保持 `degraded`，不可直接作为严格 point-in-time 财务值。
- 通过 `MARKET_DATA_HITHINK_FUNDAMENTALS_ENABLED=true` 显式启用；仍需具备财务指标端点权限的同花顺 API Key。本次未运行测试。

### 同花顺证券目录搜索补充（2026-10-26）

- 新增 `StockSearchProvider`，使用 `GET /api/meta/tickers/search` 按中文名称、代码或 THSCode 检索，显式限定 `asset_type=a-share`，不把指数、基金或外汇混入股票候选。
- 返回的名称、代码、交易所均来自官方目录，不由本地规则猜测；无匹配、重复结果及无效代码均有明确处理。
- 同花顺搜索回退与行情/日 K/财务能力共用 API Key，但只有将 `MARKET_DATA_HITHINK_ENABLED=true` 且配置 Key 后才会注册；当前不默认启用。
- 本次仅更新实现与文档，没有运行测试。

### 同花顺指数快照与交易日历（2026-10-26）

- 新增核心指数白名单快照，只请求 7 个预设沪深指数，绝不调用未指定标的的全市场模式。来源时间戳缺失/不合理、价格/成交额缺失或返回列表不完整会显式降级。
- 新增同花顺滚动一年交易日历回退；超出上游可用日期范围时返回错误，以便继续尝试 BaoStock。因上游只返回交易日，适配器生成的非交易日是推断结果，整体质量保持 `degraded`，不覆盖默认启用且数据契约更完整的 BaoStock 路径。
- 通过 `MARKET_DATA_HITHINK_INDEX_QUOTES_ENABLED=true` 和 `MARKET_DATA_HITHINK_TRADING_CALENDAR_ENABLED=true` 分别启用；两项默认关闭且均需要同花顺 API Key。本次没有运行测试。

### 同花顺三张财务报表（2026-10-26）

- 在现有 `FundamentalsProvider` 中新增同花顺利润表、资产负债表、现金流量表接口调用，按 `period_end_ms` 对齐所请求的年度/季度末，避免返回相邻报告期。
- 报表字段保留来源字段名、显示名称、单位和披露日期；对上游缺失/非法值不补零，且把对应报表段落降级。
- 一个报表端点失败时保留其他报表和财务指标已取到的数据，并把失败段落标为 degraded；只有用户显式启用同花顺 fundamentals 时才会发出这些请求。
- 当前只映射官方公开契约列出的基础报表字段；完整字段覆盖与数据权限仍待后续统一验收。本次未运行测试。

### 同花顺涨停池与龙虎榜（2026-10-26）

- 涨跌停池通过同一 `LimitPoolProvider` 接入三类官方端点：涨停 `/api/a-share/special-data/limit-up-pool`、跌停 `/api/a-share/special-data/limit-down-pool`、炸板 `/api/a-share/special-data/limit-break-pool`。适配器只读取第一页，最高 100 条；缺少最新价、证券身份或必需涨跌幅时拒绝错误数据，未知金额单位保持 `provider_defined`。
- 龙虎榜使用 `/api/a-share/special-data/dragon-tiger-list?board_type=all`，支持最近一年内的指定交易日；因为官方响应不提供分页参数，Go 适配器在内存中按统一接口做分页。
- 同花顺特别数据文档没有明示金额字段单位；新增通用 `provider_*_value` 字段和 `provider_amount_unit=provider_defined`，不把这些金额伪装成 CNY。相关结果保持 degraded。
- 通过 `MARKET_DATA_HITHINK_LIMIT_POOLS_ENABLED=true` 与 `MARKET_DATA_HITHINK_DRAGON_TIGER_ENABLED=true` 分别开启，两项默认关闭且需要同花顺 API Key。本轮未运行测试。

### 同花顺批量估值快照（2026-10-26）

- 新增独立 `ValuationProvider` 能力，并通过 `MARKET_DATA_HITHINK_VALUATIONS_ENABLED=true` 控制。默认关闭；启用前必须同时设置 `MARKET_DATA_HITHINK_ENABLED=true` 和有效的同花顺 API Key。
- 新增 `GET /api/v1/market/valuations?symbols=600519.SH,000001.SZ`，最多接受 100 个原始代码 token；必须使用带交易所后缀的 SH/SZ/BJ 股票代码，按输入顺序去重。
- 返回 PE TTM、PE MRQ、PB MRQ、PS TTM、PCF TTM。上游的 `null` 与负数原样保留，源快照时间缺失、请求股票缺失或任何指标字段缺失都会标记 degraded/unknown。
- 估值端点只查询指定的有限股票列表，不进行全市场扫描；无测试/CI 执行，本次只做实现及配置文档接线。

### 同花顺指数/板块目录与指数历史日 K（2026-10-26）

- 新增 `IndexCatalogProvider` 独立能力，不复用股票搜索、不把指数代码传入个股 K 线方法。
- 目录按 `cn_concept`、`region`、`tszs`、`industry` 四类查询，同花顺无分页时设 30,000 项防护上限；当前成分股按单个指数/板块代码查询，只代表当前成分。
- 指数/板块历史 K 线走专用 `/api/a-share-index/prices/historical`，日期范围最大十年；指数无复权语义，不暴露 adjustment 字段。时间戳缺失、成交量/成交额缺失会降级，异常 OHLC、越界日期与重复日期拒绝返回。
- 新增路由 `GET /api/v1/market/index-catalog`、`GET /api/v1/market/indices/{symbol}/constituents`、`GET /api/v1/market/indices/{symbol}/daily`，统一使用现有缓存与 Provider 回退模式。
- 通过 `MARKET_DATA_HITHINK_INDEX_CATALOG_ENABLED=true` 单独启用；默认关闭。本次未运行测试或 CI。

## Efinance / AkShare 国内数据域迁移盘点（2026-10-26）

### 已覆盖或可由独立 API 组合

- **实时行情、历史日 K、指数行情、市场涨跌统计、行业/概念排行榜、个股所属板块、涨跌停池、龙虎榜、公告**：已有 Go 原生能力链路。Efinance/AkShare 原方法多数是对公开端点的封装，并不代表独立上游。
- **指数/板块目录、当前成分股、指数/板块历史日 K**：新增同花顺独立接口；不混用股票代码归一化，也不使用个股复权语义。
- **筹码分布**：新增东方财富 Go 原生计算，复刻 AkShare CYQCalculator 最新指标逻辑。输入为最近 120 根未复权日 K 与换手率，返回最新获利比例、平均成本、70%/90% 区间和集中度；由于上游没有该指标独立时间戳，质量为 degraded。
- **增强数据 get_enhanced_data**：是日 K、行情、基本资料/板块或筹码的组合封装，不是单一上游。Go 用户可调用各自路由；目前没有一个将所有域一次聚合返回的完全等价接口。

### 仍未等价迁移的国内能力

- **人气榜/飙升榜**：AkShare 的 `stock_hot_rank_em` 和 `stock_hot_up_em` 已由 Go 原生 `HotRankingProvider` 对接东方财富；另有可选同花顺回退端点。榜单仍有延迟且不作为实时交易信号，雪球关注榜回退仍未迁移。
- **个股基本资料扩展字段**：已将 Efinance `get_base_info` 的动态 PE/PB、行业、总市值、流通市值、板块编号、ROE、净利率、净利润和毛利率映射到 Go `StockBasic`。这些当前 profile 字段无独立上游时间戳，整体质量标记 degraded；与季度财务报表分开，不拼造成无披露日期的历史财务值。
- **统计口径差异**：Python 市场统计会自行推断涨跌停计数，Go 当前分为市场广度和涨跌停池接口；字段名相似不代表语义完全等价。
- 海外市场本轮按既定范围暂缓，不在国内数据域迁移完成前引入。

本轮没有运行测试或 CI。下一项优先处理人气榜/飙升榜，将其做成独立可配置榜单能力。

### 东方财富个股人气榜 / 飙升榜（2026-10-26）

- 新增独立 `HotRankingProvider` 能力，调用东方财富 `getAllCurrentList`（人气榜）和 `getAllHisRcList`（飙升榜），然后用东方财富公开行情列表 API 补充名称、最新价与涨跌幅。
- 新增 `GET /api/v1/market/hot-stocks?type=popular|surge&limit=10`，上限 100 条；榜单能力独立于行情和 K 线 fallback。
- 校验股票代码、排名、重复项和行情数值。行情映射缺失时保留排名条目并降级；由于来源没有可信快照时间，正常响应仍为 `degraded`。
- 通过 `MARKET_DATA_EASTMONEY_HOT_RANKINGS_ENABLED=true|false` 控制，默认开启；新增独立短缓存，减少重复请求。本次未运行测试或 CI。
### Efinance get_base_info 个股 profile 字段迁移（2026-10-26）

- 用 Go 原生东方财富行情字段补齐 `StockBasic` 的动态 PE、PB、行业、总市值、流通市值、板块编号、ROE、净利率、净利润、毛利率，对应 Efinance `EASTMONEY_BASE_INFO_FIELDS`（f162/f167/f127/f116/f117/f198/f173/f187/f105/f186）。
- PE/PB 与百分比字段使用上游原值；市场/流通市值与净利润单位按人民币元解释。缺失和无效值保留为 `null`，不补 0。
- 因行情接口没有每个 profile 字段的独立时间戳，返回整体标记 `degraded`；季度 fundamentals 仍负责带报告期与披露日期的财务指标，不与 profile 当前字段混合成 point-in-time 财务数据。
- 身份/上市日期来自 Eastmoney F10；扩展行情请求失败不丢弃已经取得的身份信息，而是返回降级原因。本轮未运行测试或 CI。

### 同花顺跌停池与炸板池扩展（2026-10-26）

- Go 适配器现在支持 `up/down/broken` 三类池，并按各端点的有效排序字段固定请求第一页，避免把不同池的字段名混用。
- 统一模型扩展最新价、跌停首次/末次触及时间、炸板开板次数、换手率和供应商成交额字段；旧客户端字段保持兼容，新增字段均为可选。
- 数据仍保持 `degraded`：同花顺金额/成交额单位未在当前接口契约中明确，快照时间缺失或请求日期与上游时间不一致会显示质量原因。该能力只在显式启用 `MARKET_DATA_HITHINK_LIMIT_POOLS_ENABLED=true` 后参与回退链。
- 本轮未运行测试或 CI；真实 API 账号权限、非交易日空池语义与目标容器访问由后续统一验收。


### 同花顺人气榜与飙升榜后备源（2026-10-26）

- 复用已有 `HotRankingProvider` 和 `GET /api/v1/market/hot-stocks?type=popular|surge&limit=100`，不新增一套榜单路由。东方财富仍为默认优先数据源，同花顺只在显式启用后加入源优先级回退链。
- `popular` 对应 `/api/a-share/special-data/hot-stock-list?period=day`，`surge` 对应 `/api/a-share/special-data/skyrocket-list?period=day`。这些端点提供榜单字段，但不保证行情快照；Go 模型新增 `heat`、`rank_trend` 和可选 `provider_time`，不补造价格或涨跌幅。
- 榜单按数据延迟/非交易信号处理，整体保持 `degraded`；供应商时间戳若存在，只表示数据准备时间。空榜单保持 `unknown`，不等于已证实无人上榜。
- 启用条件：`MARKET_DATA_HITHINK_ENABLED=true`、`MARKET_DATA_HITHINK_API_KEY` 和 `MARKET_DATA_HITHINK_HOT_RANKINGS_ENABLED=true`；该能力默认关闭。本轮未运行测试或 CI。


### Tushare 指数数据能力接入（2026-10-26）

已增加 Go 原生 `index_basic`、`index_weight`、`index_daily` Provider 适配器，与同花顺的概念/行业指数目录分开标记；Tushare 目录使用 `tag=tushare`。指数成分只代表最近可用权重快照，指数日线保留 Tushare 成交量的“手”单位并将成交额千元转换为人民币元。该能力默认关闭，因账号权限、数据更新时点和单位仍需统一验收。


### 雪球国内热度榜后备（2026-10-26）

加入可选 Xueqiu Go Provider，仅覆盖现有热度排名接口。登录 Cookie 允许通过环境变量配置，但不提交真实 Cookie；默认关闭。Xueqiu 人气榜可作为后备，飙升榜使用一小时排名变化的代理口径并明确降级，避免误称与东方财富榜单等价。后续统一验收需确认是否无需 Cookie、真实字段映射、响应结构和频率限制。
