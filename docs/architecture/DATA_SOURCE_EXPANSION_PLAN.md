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
2. 代码默认 MARKET_DATA_BAOSTOCK_ENABLED=false；NAS 已通过首轮真实日 K 查询和部分数据契约验收，但生产启用仍应由部署配置显式控制，并继续扩大交易日、证券类型、分页、停牌/无数据和故障恢复等验收范围。有限样本不能等同于生产 SLA 或全范围稳定性保证。
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
| 实时个股行情 | 腾讯、新浪、东方财富 | 保持现有独立 QuoteProvider；按真实可用性与字段质量切换 | BaoStock 不作为实时行情源；东方财富在当前 NAS 上的访问故障尚未定位 |
| 日线历史 K 线 | 东方财富、BaoStock、腾讯 | 东方财富恢复可用时保留；BaoStock 作为优先新增的历史 K 线备用源；腾讯仅在返回数据明确满足所需复权口径时参与兜底 | 不因 HTTP 200 / TCP 连通就认定数据有效；必须验证交易日覆盖、单位、字段和复权算法 |
| 周线/月线历史 | BaoStock 候选 | 在日线 Provider 验收后扩展 | 单独测试聚合周期、最后交易日规则和日期覆盖 |
| 部分财务指标 | BaoStock、后续可选 Tushare / THS | BaoStock 先覆盖官方文档有明确定义的财务数据集；Tushare/THS 依权限和成本另行评估 | 报告期、公告日、数据可用时间、单位和缺失值必须分别保存 |
| 复权因子、除权除息 | BaoStock 候选 | 为复权检查、公司行动追踪和数据源对账提供补充 | 不得只以相同的前/后复权标签认定两个源的绝对价格等价 |
| 股票基础资料、行业分类、部分指数成分 | BaoStock 候选 | 历史 K 线稳定后逐个独立能力接入 | 行业分类日期与指数成分生效时间要保留；不得把当前成分股倒灌为历史成分 |
| 市场广度、行业/概念排名、涨停池、龙虎榜 | 后续评估 Tushare/THS 与可核验的公开来源 | 作为独立 Provider 能力实施 | BaoStock 不应被假定能够覆盖这些域；逐个端点核对权限、字段与更新时点 |
| 新闻与公告 | 现有过渡 bridge、未来原生 Go 数据源 | 建立独立 NewsProvider / AnnouncementProvider | 保存标题、来源、发布时间、抓取时间与可用时间；不能把搜索命中时间当作发布时间 |
| 筹码、个股资金流 | 现有可选 MX_API；未来按需评估其他来源 | 保持 opt-in 补充能力 | 先验证字段标签、金额单位和时间可用性；不能直接作为无降级的历史数据 |

### 3.2 其他候选源的定位

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

预计日 K 的初步候选顺序为“东方财富 → BaoStock → 腾讯”，但只有在 BaoStock 真实查询与复权验收通过后才能启用。路由必须依据本次请求的复权需求、Provider 实际能力与数据质量做准入；某源没有返回所需的 qfq/hfq 序列时，应继续尝试其他满足同一口径的 Provider。

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

通过以上门槛后，再把 BaoStock 加入 DailyBarProvider 的可用列表；其财务、基础资料、行业和指数成分接口另开任务逐域验收。

## 5. 分阶段实施顺序

| 阶段 | 任务 | 验收条件 |
|---|---|---|
| 0 | 保持当前服务稳定，保留东方财富故障诊断与腾讯复权校验 | 不降低现有数据质量门禁，不更改未复权/前复权语义 |
| 1 | Go 原生 TCP 协议客户端（已实现） | 已新增本地 TCP 集成回归覆盖分页、空末页、上游错误码及异常断线后下一次调用重新登录；登出流程也纳入测试。GitHub Go CI #202 通过。模拟测试不能替代目标容器的真实网络验收，仍需扩大真实数据样本并观察故障恢复。 |
| 2 | Go BaoStock DailyBarProvider、配置与单元测试（已实现，代码默认关闭） | 已有 600519.SH、688981.SH、000001.SZ、600036.SH 的真实样本，以及多复权和故障回退结果；继续扩大日期覆盖并完成停牌/非交易日/异常数据专项验收 |
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
