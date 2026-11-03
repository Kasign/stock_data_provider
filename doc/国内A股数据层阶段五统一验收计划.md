# 国内 A 股数据层阶段五统一验收计划

- 目标分支：`feat/full-go-data-providers-api-switch`
- 阶段定位：阶段五统一验收，不以“代码已实现”替代真实数据验收。
- 当前状态：验收准备与文档基线；真实 Provider、Python/Go 差分和部署验证需在统一验收窗口执行。
- 原则：每项结论必须绑定代码 SHA、样本范围、原始响应证据和质量判定；凭据不得进入文档或日志。

## 一、验收工作包

| 工作包 | 内容 | 交付物 | 通过条件 |
|---|---|---|---|
| A. Go/Python 字段级对账 | 对齐同一 Provider、证券、交易日、报告期和复权基准；逐字段比对 | 字段映射与差异报告 | 差异可解释，单位/精度/空值语义明确 |
| B. 黄金样例回归 | 覆盖沪深主板、创业板、科创板、北交所；正常/停牌/非交易日/新股/ST/退市样本 | 脱敏黄金样例集及回归结果 | 固定样例重复运行结果稳定，异常语义不丢失 |
| C. Provider 真实请求 | 按 Provider 能力矩阵执行行情、日 K、财务、指数、资金流、公告、公司行动等请求 | 真实请求验收记录 | 权限、覆盖、分页、时间戳、单位和降级均有证据 |
| D. API 契约检查 | OpenAPI、路由、参数校验、错误码、鉴权、分页、响应字段 | API 契约检查报告 | 文档与实现一致，关键契约测试通过 |
| E. CI/构建 | Go 测试、gofmt、静态检查、镜像构建、GitHub Actions | CI 与构建记录 | 目标分支检查全部通过 |
| F. MariaDB/部署 | 迁移、写入、查询、重复数据、重启恢复、数据库不可用降级 | 部署验收记录 | 数据持久化和故障语义符合契约 |
| G. 最终迁移评审 | 迁移矩阵、Provider 开关、质量状态、遗留差异与阻塞 | 《国内 A 股数据层迁移验收报告》 | 所有未通过/阻塞项有明确结论和后续动作 |

## 二、固定样本范围

- 股票：沪市主板、深市主板、创业板、科创板、北交所各选代表样本。
- 交易状态：正常交易、停牌、非交易日、新股上市初期、ST/风险警示、退市整理或已退市样本。
- 行情：同日 OHLC、涨跌幅、成交量、成交额；前复权、后复权必须固定相同参考日期。
- 财务：同一报告期、公告时间、披露修订版本；公告前不可见。
- 指数：当前成分与历史成分分开验收，月度权重报告日不得充当成员关系生效日。
- 公司行动：预案、实施、公告、登记、除权、派息日期分别核对。

## 三、真实权限前置条件

| 数据能力 | 已知权限/前置条件 | 验收状态 |
|---|---|---|
| Tushare adj_factor | 至少 2000 积分；当前接口未提供精确首次可用时间 | 权限待确认；PIT 可用时间证据缺口 |
| Tushare dividend | 至少 2000 积分；单次最多 2000 行，需核对分页完整性 | 权限与真实请求待执行 |
| Tushare index_weight | 至少 2000 积分；月度成分权重，不提供成员有效起止日期 | 权限待确认；不能作为完整成员生效区间证据 |
| Tushare market moneyflow | 至少 120 积分（具体接口以当前账号权限为准） | Token/权限待确认 |
| TickFlow | 需要有效 API Key 和对应能力授权 | 待执行 |
| 同花顺/妙想/雪球 | 需要各自 API Key、配额或登录态 | 待执行 |

## 四、统一验收记录字段

每项记录必须包含：执行时间、环境、完整 Git SHA、Provider/接口、脱敏样本、请求参数、响应状态、耗时、源端总数、返回行数、分页终止、重复/排除数量、字段差异、时间语义、质量状态、最终结论、复验条件。

结论只允许：通过、降级可接受、不通过、权限阻塞、数据源能力缺失、部署阻塞、未执行。不得把“未执行”写成通过。

## 五、执行顺序

1. 先完成第四阶段代码与 PIT 门禁收口，记录无法由现有 Provider 证明的时间字段。
2. 执行 Go/Python 字段级差分和黄金样例回归。
3. 执行 Provider 真实请求与权限检查，按能力逐项记录。
4. 执行 API 契约、Go 测试、静态检查、镜像构建和 CI。
5. 执行 MariaDB 迁移、写入、查询、重启恢复及故障降级验收。
6. 汇总差异、阻塞、Provider 开关建议，输出迁移验收报告。

## 六、当前边界

阶段五不重新扩展数据层架构，不为通过验收而降低 PIT、数据覆盖或质量门禁。真实凭据、Provider 实际响应和部署环境不可由静态代码审查替代。


## 七、阶段五首轮 CI 与真实 Provider 验收记录（2026-10-30）

- 验收代码 SHA：`a19a827d485e7273cafbae84664ce3f3a9d2d932`
- Go 测试工作流：`41018469135`
- Go/Python 质量契约工作流：`41018469120`
- 真实 Provider 探测工作流：`41018469167`
- WebUI 检查工作流：`41018469096`

| 验收项 | 结果 | 证据/说明 |
|---|---|---|
| 根模块 Go test/race/vet | 通过 | Go 测试工作流 |
| go-core Go test/race/vet | 通过 | Go 测试工作流 |
| market-data Go test/race/vet | 通过 | Go 测试工作流 |
| Go/Python 质量契约回归 | 通过 | Python-to-Go 契约测试、market-data 全量 Go 测试及旧 Python 质量标记检查通过 |
| MariaDB 11.4 迁移与 PIT 持久化 | 通过 | 重复迁移、因子/公司行动修订、重复采集去重、as_of 历史还原、重连读取均通过 |
| Docker Compose 配置与部署脚本语法 | 通过 | Go 测试工作流 |
| Docker 镜像构建 | 执行中 | 该记录编写时 market-data 镜像仍在构建，未标记通过 |
| WebUI 检查 | 通过 | WebUI 检查工作流 |
| 真实 Provider 探测 | 部分通过 | 25 项探测中 22 项返回，3 项 Eastmoney 请求遇到 HTTP 502 |

### 真实 Provider 探测观察

成功返回的能力包括 Sina/Tencent 报价与日 K、Tencent 股票搜索、Eastmoney 公告/股票板块/行业概念排行/指数行情/市场广度/涨跌停池/日 K/龙虎榜、Baostock 交易日历与日 K、Yahoo 报价与日 K。

本次 3 项失败均为 Eastmoney 股票搜索或报价请求 HTTP 502（包括备用 delay 域名报价），不是 Go 归一化断言失败。该失败必须保留在验收报告中，不能以其他 Eastmoney 数据域成功抵消。

部分返回数据按契约降级：
- Sina 日 K 未返回成交额，保持 `amount=null`，单位 unknown，质量降级。
- Tencent 报价的成交量单位需要根据行情字段关系推断，因此质量降级；日 K 未提供可验证成交额，同样保持 null。
- Yahoo 报价/日 K 未提供成交额，保持 null 并降级。
- Baostock 交易日历、Eastmoney 龙虎榜未提供精确快照时间，按时间戳缺失降级。

### 本轮尚未覆盖

- Tushare adj_factor、dividend、index_weight 的真实 Token 权限与数据响应验收。
- TickFlow 等需要密钥/授权的数据源真实请求。
- 生产 MariaDB 实例上的升级迁移、服务重启和数据库断连恢复；CI MariaDB 通过不等同于生产部署验收。
- Eastmoney 502 的跨时段复测和备用源切换效果。
- Go/Python 业务字段级真实样本差分与跨市场状态黄金样例集。当前契约回归通过不等于全量字段差分完成。

后续推进应优先补齐真实样本差分、Tushare 权限请求和生产部署验收；Eastmoney 502 作为外部源波动继续记录，不应通过放宽错误校验来消除失败。


## 八、真实黄金样例首批结果（2026-10-30）

已将本轮真实公网请求结果固化到：
- `doc/国内A股数据层黄金样例对账记录.md`
- `services/market-data/internal/provider/testdata/live_market_golden_20261030.json`
- 回归测试：`TestLiveGoldenCrossProviderQuoteAndDailyBarsParity`

首批样例：
- 贵州茅台（600519.SH）新浪/Tencent 实时行情：价格、涨跌幅、OHLC、昨收、成交额一致；成交量相差 29 股；保留新浪 valid、腾讯 degraded。
- 招商银行（600036.SH）2026-10-27 至 2026-10-29 日 K：三个交易日 OHLC 一致，成交量差异不超过 50 股；两个 Provider 的成交额都保持 null/unknown/degraded。
- 样例来源为真实 Provider probe 的 Go 归一化结果，记录 Actions Run ID 和源提交；不属于 Python/Go 双侧等价证明。

本轮 CI（源提交 `986ed9cabf7711884a288efb20044632b624d685`）结果：
- Go 根模块、go-core、market-data 的 test/race/vet：通过。
- MariaDB 11.4 PIT 持久化集成：通过。
- Go/Python 质量契约回归：通过。
- Compose、部署脚本语法、两个 Go 镜像构建：通过。
- WebUI 检查：通过。
- 真实 Provider 探测：诊断任务报告 5 项失败（Eastmoney HTTP 502/备用请求失败），不阻断普通 CI；其他返回项仅代表当次成功。Hithink 可选探测未在本轮启用。

下一批黄金样例按优先级覆盖：北交所、创业板、科创板、停牌/新股、除权日与非交易日；之后需要取得旧 Python manager 的相同证券/日期输出，才能进行 Go/Python 字段差分。


## 九、同花顺真实请求验收入口补齐（2026-10-30）

此前 `TestLiveHithinkCapabilities` 已存在，但 Live provider workflow 未设置同花顺环境变量，因此普通 CI 不会执行该测试。现已补齐手动验收入口：
- GitHub Actions：Live provider probe
- 手动运行时启用 `include_hithink`
- 在仓库 Actions Secrets 中配置 `HITHINK_FINANCE_API_KEY`
- Workflow 将密钥仅注入环境变量，不打印、不写入报告
- 测试覆盖 quote、daily_bars（none/forward/backward 三种复权）、核心指数行情、人气榜、估值、交易日历、涨停池、龙虎榜、指数目录、指数成分、指数日 K、财务指标与报表

未配置 Secret 或未手动启用时，同花顺探测仍会跳过；这不是通过，也不会影响无凭据的普通 CI。探测通过仅证明当次账号权限和返回契约，不自动打开生产 Provider 开关。


## 十、原 Python Provider 固定基线与静态字段差异已补齐（2026-10-30）

- 原项目固定提交：`Kasign/daily_stock_analysis@7a21cd76952c7fe44a7fc5a6113a5b2b8a16bb32`（2026-10-17）。
- 已确认旧 Python 实时行情链：Efinance → AkShare EM → AkShare Sina → AkShare Tencent。
- 已确认默认日 K 顺序：Efinance → AkShare → Pytdx → BaoStock → YFinance → Tencent；Tushare/TickFlow 等依赖凭据加入。
- 已识别旧 Python Efinance 缺失 volume/amount 补零、缺 open 时用 close 补 OHLC 的行为；Go 必须继续保留 null 和严格质量门禁。
- 已识别 MA 指标字段差异：Python MA5/MA10/MA20 + volume_ratio；Go DailyBar 当前 MA5/MA20/MA120/MA250。需评估是否为接口能力缺口，不能直接把指标口径混为一致。
- 以上为固定源码静态对照，不是 Python/Go 实际数据值差分。双侧真实样本仍待在原 Python 运行环境导出。

下一步：把固定 Python 基线对应的导出字段模板和 Go API 快照格式对齐，形成可重复的双侧样本差分入口；继续补充跨板块黄金样例。

## 十一、最新验收流水线状态（2026-10-30）

最新文档/基线提交：`25e2b3e010ba06257a1980cdb35d6925c32f7395`

| 工作流 | Run ID | 结果 | 备注 |
|---|---:|---|---|
| Go/Python 质量契约回归 | 41019948107 | 通过 | Python-to-Go 契约测试、market-data Go 全量测试、旧 Python 质量标记检查均通过 |
| Go 测试套件 | 41019702930 | 部分完成 | 根模块测试/race/vet 通过；go-core 测试/race/vet 通过；market-data 全量测试通过；market-data race 在后续提交触发并发取消后中止，vet 未执行 |
| Docker / MariaDB | 41019702930 | 通过 | 两个 Go 镜像构建成功；MariaDB 11.4 PIT 集成通过；Compose 与部署脚本语法通过 |
| WebUI 检查 | 41019703044 | 通过 | 未修改 WebUI 代码，仅验证分支工作区 |
| 真实 Provider 探测 | 41019702953 | 诊断失败 | 5 项失败均来自 Eastmoney HTTP 502/EOF；该工作流 continue-on-error，不阻断普通 CI |
| 同花顺可选探测 | 41019702953 | 跳过 | 未勾选 include_hithink，且没有本轮真实账号权限证据 |

真实 Provider 5 项失败：
- Eastmoney stock_search：HTTP 502。
- Eastmoney sector_rankings：HTTP 502。
- Eastmoney quote（600036.SH）：HTTP 502。
- Eastmoney delay quote（600036.SH）：HTTP 502。
- Eastmoney daily_bars（600036.SH）：连接 EOF。

这些失败必须保留为源端可用性问题，不允许通过忽略错误或降级质量门禁来让探测变绿。


## 十二、最新复测补记（2026-10-30 07:26 UTC）

- 最新代码基线：`9318f27c69067bf3cdad19706d425551ff302a8f`（本节为文档更新，不改变 Go 代码）。
- Go/Python 质量契约回归 Run `41019954339`：通过。
- WebUI 检查 Run `41019954568`：通过。
- 真实 Provider Run `41019954185`：探测步骤仍有 4 项失败，均为 Eastmoney HTTP 502（stock_search、sector_rankings、quote、备用 quote）；其余请求本轮返回成功。工作流设置 `continue-on-error`，不能把 Run conclusion=success 当作所有 Provider 全部通过。
- 同花顺 `TestLiveHithinkCapabilities`：跳过，尚未启用 `include_hithink`，无真实 API Key 权限证据。
- Go 测试 Run `41019954395`：根模块、go-core 的 test/race/vet 均通过，market-data 全量测试通过，MariaDB PIT 集成通过；market-data race 与镜像构建在后续提交触发并发取消后未完成。因此最新 market-data race/vet 仍需一次不被中断的完整验收。

后续不要为消除 Eastmoney 502 而放宽 Provider 错误检查；应保留失败证据并继续推进可独立完成的字段差分和样本覆盖。


## 2026-10-30 跨市场样本探测补充

阶段五新增实时探测覆盖，当前仅扩展到公开端点已实现适配的市场：

- 创业板：`300750.SZ`（宁德时代），新浪、腾讯快照与未复权日 K。
- 科创板：`688981.SH`（中芯国际），新浪、腾讯快照与未复权日 K。
- 主板原有样本继续保留，用于对照；科创板腾讯日 K 成交量按源端约定使用股，其他市场按手转换，必须分别校验，不能共用转换规则。
- 北交所当前新浪/腾讯适配器明确不支持，不能将其强行加入上述探测并视为 Provider 缺陷；待确认支持北交所的真实数据源及其成交量单位契约后，另建独立探测。

本次只增加探测入口，不预先写入创业板/科创板黄金值。只有真实探测返回并通过证券代码、OHLC、单位、复权和质量契约后，才可将脱敏结果固化为 fixture。2026-10-30 当前主板黄金样例仍不能代表跨市场验收已完成。

## 2026-10-30 跨市场探测结果复核与校验修复

针对提交 `4798fc2718c5fcec5807fd6414996a419d449c70` 的验收结果：

- Go 测试套件 Run `41024020532`：全部 5 个 Job 通过，包括 market-data MariaDB/PIT、go-core、market-data、Compose/Docker 镜像构建及根模块测试。
- WebUI 检查 Run `41024020560`：通过，仅为检查，不代表本分支修改了 WebUI。
- 真实 Provider 探测 Run `41024020648`：探测 Job 失败，不能记为通过。

### 跨市场日 K 失败原因

复核日志后确认，创业板 `300750.SZ` 和科创板 `688981.SH` 的新浪/腾讯日 K 失败并非已证明的源端证券代码错误，而是探测器把展示标签（例如 `sina-chinext`）误当作数据记录中的规范 Provider 标识（`sina`）进行比较，导致身份契约误报。

已修复探测调用：展示标签与规范来源标识分别传递，身份校验仍严格要求记录中的证券代码和规范 Provider 来源匹配；新增回归测试覆盖展示标签与来源标识不同的情况。修复提交：
- `c007c685edd56769a36f2deb0f695fac379e6119`：修正探测身份校验使用的来源标识。
- `fc7895b17d78c757bb369df43c87836a251c6b1b`：补充展示标签/规范来源分离的回归测试。

上述修复尚待新一轮 CI 和实时探测验证。在复测通过前，创业板、科创板仍不得标记为已完成黄金样例验收。

### 仍然存在的真实外部问题

同一轮探测日志还记录到：
- Eastmoney 股票搜索、行业/概念排行、报价及备用报价 HTTP 502。
- BaoStock 前复权日 K 登录请求超时。
- NetEase 行情因 hosted runner DNS 解析失败而跳过，历史 CSV 返回 HTTP 502。
- Yahoo 日 K 能返回，但成交额缺失且金额单位未知，继续保持 `null / unknown / degraded`。

这些问题与上述探测器身份标签误报相互独立。不得因修复探测器而放宽源端错误、成交额缺失或数据质量门禁。

## 十三、实时 Provider 凭据与探测结果分类规则（2026-10-30）

本节作为后续实时探测的固定操作规范，后续复测直接引用本节，不再重复排查“失败是否由未配置 Key/Token 引起”。

### 状态分类与处理规则

| 状态/原因 | 判定条件 | 处理方式 | 是否计入探测失败 |
|---|---|---|---|
| skipped（未启用） | Provider 的 opt-in 开关未开启 | 不发请求，报告明确写出启用条件 | 否 |
| skipped（缺少凭据） | 已选择执行可选 Provider，但必需 Token/API Key/Cookie 未配置 | 跳过该 Provider；报告写明缺少哪类配置，不打印凭据 | 否 |
| failed（凭据无效/权限不足） | 已配置凭据并发出请求，源端返回鉴权错误或 capability 无权限 | 保留失败及错误码；区分账号认证与单项能力授权 | 是 |
| failed（外部接口/网络） | 公共接口 HTTP 5xx、EOF、DNS、连接超时等 | 记录 Provider、能力、端点、错误类型和复测条件 | 是 |
| returned_degraded | 请求成功，但成交额、成交量单位、时间戳或其他质量证据不完整 | 保留数据并降级质量，不伪造字段、不补零 | 不作为请求失败；质量门禁仍按契约判定 |
| passed / returned | 请求与身份、字段、时间、单位等契约通过 | 记录当次通过能力和样本范围 | 否 |

补充约束：
- skipped 不等于 Provider 可用，也不能作为 Provider 已验证、可启用生产路由的证据。
- 无凭据 Provider 不应阻断无凭据普通 CI；如果用户显式启用探测但没有配置凭据，仍按 skipped 处理，不应制造一次必然失败的网络请求。
- 凭据不得通过聊天消息、日志、报告或文档传递；只允许使用本地环境变量或 GitHub Actions Secrets。
- 已配置凭据后，鉴权失败、配额不足、单项 capability 无权限属于真实验收失败，不能再归类为“缺少凭据”。
- 不得将公共接口 502、EOF、DNS 或 TCP 超时归因于 API Key 缺失；应保留独立网络故障记录。
- 探测工作流的 continue-on-error 仅表示诊断结果不阻断普通 CI；必须检查 Job 日志和报告，不能只看 workflow conclusion。

### 可选 Provider 的触发条件

| Provider | 启用条件 | 凭据 | 当前规则 |
|---|---|---|---|
| Tushare | MARKET_DATA_LIVE_TUSHARE_TESTS=1；Actions 手动运行时由 include_tushare 控制 | MARKET_DATA_TUSHARE_TOKEN 或 TUSHARE_TOKEN；Actions 使用 TUSHARE_TOKEN Secret | 未启用或缺少 Token 时 skipped；配置 Token 后才验证真实权限 |
| 同花顺 | MARKET_DATA_LIVE_HITHINK_TESTS=1；Actions 手动运行时由 include_hithink 控制 | MARKET_DATA_HITHINK_API_KEY 或 HITHINK_FINANCE_API_KEY；Actions 使用后者 Secret | 未启用或缺少 Key 时 skipped；配置 Key 后才验证各 capability 权限 |
| 妙想 | MARKET_DATA_LIVE_MX_TESTS=1 | MX_APIKEY / MARKET_DATA_MX_API_KEY | 未启用或缺少 Key 时 skipped；该探测会消耗配额，按需手动执行 |
| 雪球 | MARKET_DATA_LIVE_XUEQIU_TESTS=1 | MARKET_DATA_XUEQIU_COOKIE | 未启用时 skipped；需要登录态时才执行 |
| NetEase | MARKET_DATA_LIVE_NETEASE_TESTS=1 | 无 API Key；公共端点 | 默认不重复触发不稳定请求；手动开启后 DNS/HTTP 错误仍记为网络失败 |
| BaoStock | 由基础实时探测覆盖 | 无 API Key；TCP 登录服务 | 登录超时后依赖同一连接的后续能力可以 skipped，不能重复发起相同超时请求 |
| Sina / Tencent / Eastmoney / Yahoo | 基础实时探测 | 公共端点 | 无凭据门槛；网络与数据质量问题必须如实记录 |

### 最近一次实时探测结果（Run 41025531370）

- 代码 SHA：f5d2b1a0c5338c21c55a3717c658632831241bc5
- Job：123962636260，结论 failure；这是诊断探测，不影响普通 CI 的其他独立工作流。
- 同轮 Go 测试 Run 41025531466：5 个 Job 全部通过，包含根模块、market-data、go-core、MariaDB/PIT、Compose 和 Docker 镜像构建。
- Go/Python 质量契约 Run 41025535204：通过。
- WebUI 检查 Run 41025531397：通过。

实时探测日志中 7 项失败：
1. Eastmoney 股票搜索：HTTP 502。
2. Eastmoney 板块排行：HTTP 502（两项请求）。
3. Eastmoney 实时行情：HTTP 502。
4. Eastmoney 备用行情端点：HTTP 502。
5. Eastmoney 历史日 K：连接 EOF。
6. BaoStock 日 K 登录：TCP 读取超时。

因此 Eastmoney 本轮实际有 6 项失败，BaoStock 有 1 项失败，合计 7 项。BaoStock 交易日历因前序登录 TCP 会话未建立而跳过，未重复请求。

本轮其他状态：
- NetEase quote 因 hosted runner DNS 解析失败而 skipped；历史 CSV 返回 HTTP 502 后按当前可选探测策略 skipped。手动启用后应重新检查并记录真实网络结果。
- 同花顺未启用（MARKET_DATA_LIVE_HITHINK_TESTS 未设置为 1），测试 skipped；本轮没有同花顺账号权限结论。
- 妙想 chip_distribution / capital_flow 因 opt-in 未启用且无 API Key 而 skipped；本轮没有妙想账号权限结论。
- Tushare 可选能力测试没有包含在本轮 workflow 的 go test 正则中，因此本轮属于未执行，不应写成 skipped 或通过。后续已纳入手动探测入口后，再按 Token 配置情况判定。
- 新浪、腾讯主板、创业板、科创板 quote / daily_bars 均有 returned 记录；跨市场身份标签修复后未再出现此前的身份误报。returned 仅表示响应通过对应探测断言，不替代跨市场黄金样例、成交量单位和数据质量验收。
- Yahoo quote / daily_bars 返回数据；成交额缺失/单位不可验证时继续按 null/unknown/degraded 处理。
- Eastmoney 的其他能力（公告、股票板块、指数行情、市场广度、涨跌停池、龙虎榜等）有 returned 记录；不能用这些成功项抵消失败接口。

后续执行时只需：
1. 确认本次要测的可选 Provider 是否启用、Secret 是否已配置；未配置即 skipped，不追问、不重复试错。
2. 读取报告中的失败能力；区分鉴权错误、网络错误和数据质量降级。
3. 仅对新增代码或明确要求复测的能力发起真实请求。
4. 把新的 Run ID、SHA、失败/跳过/降级明细追加到本节或黄金样例记录，避免覆盖历史结果。

## 十四、北交所 Provider 代码能力矩阵（静态代码核查，2026-10-30）

本节描述当前 Go 适配器代码明确支持的证券代码映射，不等于对应源站当前网络可用，也不等于真实账号权限验收通过。

| Provider | 北交所代码映射/入口 | 当前代码结论 | 证据与限制 |
|---|---|---|---|
| Eastmoney | eastmoneySecID 将 .BJ 映射为 0.代码；证券目录/涨停池解析支持 .BJ | 代码支持 | 本轮实时探测 Eastmoney 部分接口 502；北交所行情/日 K 尚未作为独立黄金样例完成真实验收 |
| Tushare | 证券交易所后缀校验接受 BJ；A 股代码识别接受 4/8/92 开头六位代码 | 代码支持 | Token、接口权限、真实返回和分页仍需单独验证 |
| TickFlow | tickflowDomesticSymbol 接受 SH/SZ/BJ，并通过 A 股普通股票代码校验 | 代码支持 | 需 API Key 和对应能力授权；未配置时跳过 |
| 同花顺 | hithinkDomesticSymbol 接受 SH/SZ/BJ，并通过 A 股普通股票代码校验 | 代码支持 | 需 API Key 和具体 capability 权限；未配置时跳过 |
| Pytdx | pytdxDomesticSymbol 接受 SH/SZ/BJ；pytdxWireSymbol 保留交易所前缀，例如 920000.BJ -> bj920000 | 代码支持 | 代码与协议映射有单测；仍需真实服务器样本确认覆盖与成交量单位 |
| Sina | sinaCode 只映射 SH/SZ | 明确不支持 | BJ 后缀返回 unsupported，不应发起请求 |
| Tencent | tencentCode 明确拒绝 BJ | 明确不支持 | BJ 后缀返回 unsupported，不应发起请求 |
| NetEase | neteaseCode 只映射 SH/SZ | 明确不支持 | BJ 后缀返回 unsupported，不应发起请求 |
| BaoStock | baostockSecurityCode 只映射 SH/SZ | 明确不支持 | BJ 后缀返回 unsupported，不应发起请求 |

北交所能力验收规则：
- 代码映射支持与真实源端可用性分开记录；不支持的 Provider 应明确 unsupported，不能记为 DNS、HTTP 或账号失败。
- 代码映射支持的 Provider，仍须验证证券代码身份、交易所、OHLC、成交量单位、复权口径、交易日期和质量状态。
- 不允许只凭 isTushareAShareSymbol 接受 BJ 就推断所有 Provider 都支持北交所；必须逐个检查 Provider 的代码转换函数。
- 北交所黄金样例暂不固化，待至少一个可用 Provider 返回真实、可校验的报价和日 K 样本后再增加 fixture。

## 十四、统一数据 API 诊断能力规划（2026-10-30）

### 现状核对

当前已存在以下接口，不重复新建同类入口：

| 接口 | 当前职责 | 当前边界 |
|---|---|---|
| `GET /api/v1/data-sources` | 返回已注册 Provider 与能力配置 | 配置/能力清单，不代表真实连通或质量通过 |
| `GET /api/v1/data-sources/diagnostics` | 返回业务请求形成的 Provider 运行诊断 | 当前主要覆盖 quote、daily_bars 熔断器状态；不主动探测，不做全量数据质量检查 |
| `POST /api/v1/data-sources/probe` | 主动探测已启用 Provider 的能力 | 已覆盖 quote、daily_bars、stock_basic、stock_search、index_quotes、market_breadth、sector_rankings、market_capital_flow 等部分能力；结果字段目前以状态、质量、错误分类为主，详细错误证据和业务 API 端到端覆盖仍需补齐 |
| `GET /api/v1/data-sources/probe/history` | 查询已持久化的探测批次 | 依赖 MariaDB；未配置存储时不能提供历史记录 |
| `GET /health/live`、`GET /health/ready` | 进程存活与配置依赖就绪 | 不代表上游 Provider 可用，更不代表行情准确 |

结论：以现有 `POST /api/v1/data-sources/probe` 为统一主动诊断入口进行增强；保留现有路由与旧字段兼容。运行态 `diagnostics`、Provider 能力清单及健康检查继续承担各自职责，不合并成含义模糊的单一状态。

### 目标

为所有重要数据服务提供可重复执行、可追溯、可解释的诊断能力，覆盖从 HTTP API 到 Provider，再到归一化数据质量和持久化的关键链路。诊断结果必须能明确回答：

1. API 是否可访问、鉴权是否正确、响应结构是否符合契约。
2. 对应 Provider 是否已配置、能力是否启用、凭据是否具备；是否实际发起请求。
3. 上游连接是否成功，HTTP 状态、超时、DNS、EOF、限流和权限错误分别是什么。
4. 返回的数据是否属于请求的证券、市场、日期和能力，关键字段、单位、时间及复权口径是否正确。
5. 数据是否完整、是否陈旧、是否降级；与可比来源是否存在超出阈值的差异。
6. 失败发生在哪个阶段、具体错误原因、证据摘要及建议复验动作是什么。

### 诊断范围

| 维度 | 检查内容 |
|---|---|
| API 连通性与契约 | 路由可达、HTTP 状态、鉴权、参数校验、响应 JSON/schema、耗时、超时 |
| Provider 配置 | 注册状态、能力开关、必需凭据是否配置；只检查凭据存在性，不输出凭据 |
| Provider 连通性 | DNS/连接/TLS、HTTP 状态、响应体类型、空响应、解析错误、限流、权限错误 |
| 证券身份 | 规范代码、交易所、板块、普通股票类型；拒绝 ETF、指数、可转债等不匹配样本 |
| 行情准确性 | 价格范围、OHLC 关系、昨收/涨跌额/涨跌幅关系、成交量与成交额单位 |
| 日 K 准确性 | 日期连续性、重复记录、OHLC、成交量/额、复权基准、交易日边界 |
| 数据完整性 | 必填字段、null/zero 语义、记录数、分页终止、覆盖率、重复/缺失 |
| 数据时效性 | Provider 时间、采集时间、交易日期、缓存年龄、陈旧数据 |
| 其他业务能力 | 证券资料、搜索、指数、市场广度、行业概念、涨跌停池、龙虎榜、交易日历、财务、估值、资金流、公告、成分股等按各自契约校验 |
| 跨源一致性 | 相同证券/日期/复权口径下，对价格、OHLC、成交量和可验证金额做字段差异比较 |
| 存储链路 | MariaDB 写入/读取、重复写入、历史探测查询；Redis 可用性与缓存命中状态 |

### API 设计与兼容策略

保留现有接口：

- `POST /api/v1/data-sources/probe`：执行主动诊断。
- `GET /api/v1/data-sources/probe/history`：查询历史批次。
- `GET /api/v1/data-sources/diagnostics`：查询线上业务流量形成的运行态诊断。
- `GET /api/v1/data-sources`：查询配置能力清单。

在现有 POST 请求体中增加可选诊断参数，默认行为保持兼容：

- `scope`：`all`、`core` 或指定能力集合。
- `markets`：沪市主板、深市主板、创业板、科创板、北交所。
- `symbols`：受控样本证券列表；默认使用固定脱敏样本。
- `checks`：connectivity、contract、quality、freshness、cross_provider、storage。
- `include_details`：是否返回字段级差异和脱敏证据。
- `timeout_seconds`：受服务端上限约束，不允许调用方无限延长请求。

当前同步执行模式先保持不变；当诊断项增多、执行时间可能超过 HTTP 超时后，再基于现有 run_id 与 MariaDB 记录扩展异步任务查询能力，不提前引入重复任务系统。

### 统一结果模型

每个诊断批次和每个检查项分别给出状态。批次汇总至少包含 total、passed、degraded、skipped、failed、not_configured；每项至少包含：

- check_id、category、provider、capability、symbol、market、checked_at。
- status、stage、error_code、error_message、recommendation。
- elapsed_ms、http_status、attempt_count、response_bytes。
- request_summary（脱敏）、response_summary（脱敏）。
- expected、actual、difference、tolerance（适用时）。
- quality_status、quality_reasons、source_version、provider_time、fetched_at。
- evidence_refs 或有限大小的脱敏样本摘要。

状态口径统一为：`passed`、`degraded`、`skipped`、`failed`、`not_configured`、`not_supported`。批次状态由检查项聚合产生，不允许用单一 Provider 成功覆盖其他能力失败。

错误阶段至少区分：configuration、credential、dns、connect、tls、http、rate_limit、provider_auth、provider_permission、decode、identity、contract、quality、freshness、cross_provider、storage、timeout、cancelled。

### 凭据、网络和质量门禁

- Tushare、同花顺、TickFlow、妙想等需要凭据的 Provider：未配置凭据时标记 `skipped` 或 `not_configured`，不发真实请求，不计为失败，也不计为通过。
- 已配置凭据后出现鉴权、积分、配额或单项能力授权问题，必须作为具体失败记录。
- 公共接口仍可执行探测；DNS、连接超时、EOF、HTTP 502/5xx、限流分别保留，不能归因于缺少凭据。
- 不输出 API Key、Token、Cookie、完整带查询参数 URL、原始敏感响应；错误消息需脱敏并限长。
- `skipped`、`not_configured`、`not_supported` 均不能作为 Provider 可用性证据。
- HTTP 200 不等于数据正确；字段、证券身份、交易日期、单位、复权、时效和质量状态仍需逐项检查。
- 缺失成交额不得补零；未知单位、时间戳或复权语义继续降级；不因诊断接口建设而降低现有数据质量门槛。
- 跨源比较只在样本身份、交易日、复权基准和单位一致时执行；不可比字段标记 not_applicable，不制造差异结论。
- 诊断不得写入正式行情缓存或污染业务熔断统计；诊断请求并发、样本量和超时必须受限。

### 实施顺序与验收

1. **P0：诊断结果模型与错误细节**。扩展现有 ProviderProbeItem，增加错误阶段、脱敏错误摘要、检查阶段、样本身份和有限证据；保留现有 JSON 字段和历史记录兼容。
2. **P0：覆盖面扩展**。按已注册能力矩阵补齐核心市场/个股/财务/指数/资金流/公告等检查项；无 Provider、未配置、未支持分别输出，不得静默漏项。
3. **P0：数据质量断言**。统一接入现有 normalize 校验和各能力专用校验，覆盖证券身份、OHLC、成交量/额单位、日期、分页、时间和质量状态。
4. **P0：跨源对账**。以固定黄金样例执行同日同口径比较，输出逐字段 expected/actual/difference；阈值必须有契约依据。
5. **P1：API 契约与历史**。完善 OpenAPI、参数验证、历史详情、存储不可用语义和向后兼容测试。
6. **P1：WebUI 对接**。数据中心提供一键诊断、批次概览、Provider/能力明细、失败阶段筛选、错误详情和历史报告；WebUI 仅展示 API 返回的真实状态。
7. **最终验收**。使用 httptest 模拟成功、空响应、错误身份、单位异常、超时、HTTP 502、权限失败、凭据缺失、存储故障；再执行无凭据公共源诊断。需要真实 Key 的能力仅在用户配置凭据后单独验收。

交付标准：每个重要数据能力都有明确检查项和状态；每个失败项都有可定位的错误阶段、错误代码和脱敏原因；未执行/跳过/不支持不会被显示为通过；报告可持久化查询；OpenAPI、自动化测试与 WebUI 展示口径一致。



### 当前开发进度补充（2026-10-30）

统一诊断接口仍复用 `POST /api/v1/data-sources/probe`，没有新建重复路由。当前代码已完成以下扩展：

- 主动探测结果新增 `error_stage`、脱敏 `error_message`、`http_status`；区分 HTTP、限流、鉴权、权限、网络、超时和契约错误。
- 新增 `unknown`、`skipped`、`not_configured`、`not_supported` 汇总计数；质量未知不再伪装为普通降级返回。
- 固定核心能力清单：未注册 Provider 的能力以 `not_configured` 明确展示，避免“没有探测项”被误读为通过。
- 新增板块归属、涨跌停池、龙虎榜、公告、交易日历、财务、估值、人气榜、个股资金流、筹码分布、指数目录、证券交易状态、涨跌幅特殊事件的主动探测。
- 全市场证券目录可能返回数千行，默认轻量诊断不下载全量目录，明确标记为策略性 `skipped`，避免一次诊断造成大流量请求。
- 交易日历、公告、财务、估值、人气榜等探测优先复用业务 API 的正式质量校验函数，避免诊断和业务接口各自形成不同质量口径。

当前边界与后续工作：

1. 已支持请求体 `scope=all/core`、`capabilities` 白名单和 `timeout_seconds=1..30`（默认 12 秒）；显式能力列表会精确收窄探测范围，core 与白名单冲突时返回参数错误。`markets`、`symbols` 已实现单板块/单证券定向过滤；`checks` 仍未实现。
2. 已新增真实 HTTP Handler 的 `httptest` 覆盖：API Key 鉴权、空请求体兼容、core 范围、能力白名单、非法 JSON/字段/能力/超时、重复能力、超大请求体和响应汇总。OpenAPI 同步声明请求体及安全要求。
3. Provider 级逐项结果仍需补充请求摘要、返回行数/覆盖率、样本证券和数据时效证据；当前仍以状态、质量原因和脱敏错误证据为主。
4. 对未注册 Provider 的能力标记为 not_configured；凭据已配置但授权不足、限流等情况必须继续走真实错误分类。无凭据 Provider 不发请求。
5. 本轮请求范围功能尚待最新 HEAD 的 Go 单测、Race、Vet、OpenAPI 契约和 Docker 构建结果确认；未完成最新 SHA 验收前不记录为通过。



### 诊断证据字段补充（2026-10-30）

- ProviderProbeItem 新增请求样本证券、市场、样本日期、原始返回行数、校验后行数及 Provider 数据时间字段。
- quote 探测已输出固定样本 `600519.SH`、市场 `SH`、返回/校验记录数和 ProviderTime。
- daily_bars 探测已输出固定样本 `600036.SH`、最新有效交易日、原始返回行数、校验后行数及可用的 ProviderTime。
- 证据只在 Provider 实际返回相应字段时填写，不从系统抓取时间伪造 Provider 数据时间；字段缺失继续保持空值。
- OpenAPI 已同步新增字段。其余能力的行数/覆盖率与时效证据仍需按各自快照结构逐步补齐。


### 统一诊断证据扩展（2026-10-30）

本轮在 quote、daily_bars 基础上，继续为主要业务快照补充诊断证据：

- 指数行情、指数目录：返回/校验条数、可用样本证券、Provider 时间（仅在源端提供时）。
- 市场广度：覆盖证券数、样本交易日、可用最新行情时间，并新增 `coverage_ratio = covered_securities / total_securities`；总证券数为 0 时不计算比例。
- 行业排行、个股板块、涨跌停池、龙虎榜、公告、交易日历：返回/校验条数、可用样本证券或交易日。
- 财务：报告章节数量；估值：样本证券、记录数量、Provider 时间；人气榜：样本证券、记录数量、Provider 时间。
- 个股资金流：按源端观察数记录；筹码分布：记录单证券样本和源端数据日期。
- `coverage_ratio` 仅用于有明确分母的市场广度，不将分页返回比例误标为全市场覆盖率。
- Provider 的 `FetchedAt` 不冒充 ProviderTime；只有源端明确提供的 Provider 时间才写入 `provider_time`。

仍未完成：多板块/多证券批量过滤、各能力完整的覆盖率分母契约、HTTP API 的端到端运行验收，以及需要凭据的真实 Provider 请求。单板块/单证券定向过滤已在后续小节实现并记录。


### 市场与证券定向诊断（2026-10-30）

统一诊断接口已开始支持 `markets`、`symbols` 两个请求字段，仍复用 `POST /api/v1/data-sources/probe`，不新增路由。

- `markets` 当前接受单个板块：`SH_MAIN`（沪市主板）、`SZ_MAIN`（深市主板）、`CHINEXT`（创业板）、`STAR`（科创板）、`BSE`（北交所）。
- `symbols` 当前接受单个证券代码，格式为六位代码加交易所后缀，例如 `600519.SH`、`000001.SZ`、`920001.BJ`。
- 单次请求最多指定一个 `market` 和一个 `symbol`；批量多证券/多板块任务暂不在同步轻量诊断中展开，避免请求耗时和上游并发不可控。
- 仅允许与单证券样本直接相关的能力：quote、daily_bars、stock_basic、stock_search、security_trading_status、price_limit_event、stock_boards、announcements、fundamentals、valuations、capital_flow、chip_distribution。
- 指定 markets/symbols 时必须显式提供 `capabilities`；市场广度、行业排行、指数行情等市场级能力不接受证券范围过滤，混合请求会被拒绝，避免响应看似已过滤但实际仍查询全市场。
- 仅指定 market 时使用该板块的固定样本证券；同时指定 symbol 时会校验证券代码是否属于该板块。指定 symbol 但不指定 market 时，根据代码后缀记录 SH/SZ/BJ 交易所。
- 响应批次回显 `requested_markets`、`requested_symbols`；每项仍记录实际样本证券与交易所。
- OpenAPI、HTTP Handler 和 Service 层均增加参数约束及测试。

当前限制：市场过滤是按板块固定样本证券定向探测，不是对该板块全量证券扫描；一次只支持一个样本。全市场证券目录仍按轻量诊断策略跳过。后续如需批量诊断，应采用受控异步批次和限速，不应直接扩大同步请求并发。


### 跨 Provider 日 K 对账契约（2026-10-30）

新增内部可复用比较器 `CompareCrossProviderDailyBars`，当前用于固定夹具和契约测试，不新增线上 HTTP 接口，也不发起真实 Provider 请求。

- 比较前强制校验 Provider 不同、来源版本存在、证券代码一致、复权口径一致、交易日期集合一致。
- 仅允许 `quality_status=valid` 的两侧数据进入比较；`degraded`、`unknown` 不作为可信基准，返回 `not_comparable`，不统计为通过。
- 成交量必须已归一化为 `shares`；OHLC 容差为 0.001，成交量容差为 50 股。
- 成交额仅在双方均提供且单位均为 CNY 时比较，容差为 1 CNY；缺失或单位不明时字段状态为 `not_comparable`，整体结果为 `partial`，不反推成交额。
- 结果区分 `matched`、`partial`、`mismatch`、`not_comparable`，并保留逐日期、逐字段差异和容差。
- 固定测试覆盖复权口径不一致、证券不一致、质量降级、日期集合不同、成交额缺失和字段超差。

当前黄金样例中的新浪/腾讯日 K 两侧均为 degraded（成交额缺失），因此可用于归一化行为回归，但不能作为该比较器的 `matched` 证据。真实 Provider 结果仍需在凭据与网络条件满足后单独验收。


---

## 十五、阶段五剩余任务总清单与执行顺序（2026-10-30，接续基线）

本节是后续开发的唯一任务入口。后续每轮先更新本节状态，再推进代码；历史章节保留作为证据，不再重复从头 Review。

### 当前基线

- 目标分支：`feat/full-go-data-providers-api-switch`。
- 本节初始整理基线为 `b2a08820e41c4afffdf320e172670421b014f68f`；本轮接续时远端 HEAD 为 `04932a7aed4bcc4bac45ef2c5f2042833e2a4708`。
- PR #1：Open Draft，未合并；禁止合并或改动 PR 目标分支。
- 上一提交的 Go test suite、WebUI checks、Go/Python quality parity 均通过；Live provider probe 的探测 Job 失败，不能把 workflow 的 continue-on-error/最终状态当作 Provider 全部可用。
- 现有新浪/腾讯 600036.SH 日 K 黄金样例两侧均为 degraded，成交额缺失；只可做质量降级回归，不可作为 matched 证据。

### P0：代码闭环（先做，不等待用户 Key）

| 顺序 | 任务 | 当前状态 | 完成标准 |
|---|---|---|---|
| P0-1 | 跨 Provider 日 K 比较器接入黄金夹具 | 已完成（真实降级夹具调用路径已接入；真实 matched 证据仍待） | 真实黄金夹具调用统一比较器；降级样例明确得到 not_comparable；独立 valid 合成契约样例验证 matched/partial/mismatch |
| P0-2 | 比较器输入边界校验 | 已完成（静态契约边界与单测已补齐） | 拒绝非正/NaN/Inf OHLC、OHLC 关系错误、负成交量、负/非有限成交额；日期重复/非法和空集合状态明确 |
| P0-3 | 跨市场黄金样例 | 部分完成（创业板/科创板 Sina/Tencent 真实降级样例已归档；有效 matched 样例待取得） | 创业板 300750.SZ、科创板 688981.SH 有真实有效样例后固化；按 Provider 分别验证代码映射、成交量单位和质量状态 |
| P0-4 | 北交所支持矩阵 | 部分完成（代码映射矩阵和 not_supported 诊断分类已补齐；真实在线样本待验） | 逐 Provider 记录 BJ 代码映射和能力支持；新浪、腾讯、网易、BaoStock 当前明确不支持时标记 not_supported，不发请求、不计失败；至少一个真实支持源返回有效样本后再固化黄金夹具 |
| P0-5 | 特殊交易样例 | 待完成 | 停牌、新股上市、除权除息、非交易日各有固定样本与预期行为；不得用工作日推断交易日 |
| P0-6 | 统一诊断 checks 过滤 | 待完成 | `POST /api/v1/data-sources/probe` 支持 connectivity、contract、quality、freshness、cross_provider、storage；未知/重复/冲突值明确 400，空值保持旧请求行为 |
| P0-7 | 覆盖率分母契约 | 部分完成（市场广度已有 total/returned/validated/missing/excluded/ratio；其他能力待扩展） | 明确 total、returned、validated、missing、excluded、coverage_ratio；分母来源可追溯，分母未知时 ratio=null，禁止用返回数自作分母 |
| P0-8 | 诊断逐项证据 | 部分完成 | 每项记录脱敏 request_summary、返回/有效记录数、源端时间、质量原因、错误阶段；不适用字段为空，不用 FetchedAt 冒充 ProviderTime |
| P0-9 | 历史诊断与兼容 | 待完成 | MariaDB 保存/查询新字段；旧历史 JSON 可读；存储未配置/故障明确 storage 状态，不伪装诊断通过 |
| P0-10 | 统一诊断端到端 | 待完成 | HTTP Handler → Service → Provider → 标准化/质量校验 → MariaDB 持久化/查询的集成测试覆盖成功、降级、失败、跳过和不支持 |

### P0：业务数据质量契约补齐

按现有模型和校验函数增量补齐，不重写 Provider 架构：

1. quote：证券身份、价格范围、涨跌额/涨跌幅关系、OHLC、昨收、成交量/成交额单位。
2. daily_bars：日期排序/重复、OHLC 关系、复权口径、成交量单位、成交额缺失语义。
3. stock_basic/security status：代码、交易所、板块、上市/退市状态和生效日期。
4. fundamentals：报告期、公告时间、修订版本、字段缺失与零值语义。
5. valuations：PE/PB/市值字段单位、时点和负值/无穷值边界。
6. capital_flow/chip_distribution：观察日期、金额单位、价格区间、集中度及源端发布时间。
7. sector/index：成分归属、生效时间、排行统计口径、覆盖率分母。
8. limit_pool/dragon_tiger/announcements/trading_calendar：交易日期、状态、证券身份、席位金额单位、公告关联、交易所日历完整性。

完成标准：每类能力有有效、缺失、异常、过期和不支持样例；质量状态与 API/OpenAPI/诊断输出一致；不得补零或推断源端时间。

### P1：真实 Provider 验收（凭据/网络具备后执行）

- Eastmoney：公共接口 502/EOF 跨时段复测，分别记录接口，不用其他能力成功抵消失败。
- BaoStock：登录超时、日 K、交易日历；区分网络超时和账号问题。
- Sina/Tencent/NetEase：行情与日 K 的原始单位、标准单位、金额字段和质量原因。
- TickFlow：只有配置 API Key 后验收行情、市场广度、行业排行。
- Tushare：只有配置 Token 并确认积分后验收日 K、复权因子、财务、估值和指数。
- 同花顺：只有配置 API Key 并启用手动 workflow 后逐项验收能力权限。
- 妙想、雪球：分别按 API Key/Cookie 和接口权限验收资金流、筹码、人气榜。
- 未配置凭据的 Provider 继续 skipped/not_configured；明确不支持的市场/能力为 not_supported；公共源网络故障单独 failed。

真实请求记录必须包含 SHA、Provider/capability、脱敏样本、请求范围、耗时、HTTP/业务状态、返回/有效行数、覆盖率分母、字段单位、源端时间、质量状态和结论。

### P2：集中验收与收尾

1. Go 全模块 test/race/vet、gofmt、Go/Python 质量契约、MariaDB PIT 集成。
2. Docker Compose、两个 Go 镜像构建和部署脚本。
3. OpenAPI 与 Handler/Service/历史数据兼容检查。
4. 真实 Provider 结果归档，明确未配置、权限阻塞、不支持、网络失败和质量降级。
5. 输出国内 A 股 Provider 能力矩阵及《国内 A 股数据层迁移验收报告》。

### 执行约束

- 先完成 P0 代码闭环，再集中跑全量回归；开发中不反复触发完整 CI。
- 需要 Key/Token 的真实请求不作为代码任务阻塞条件；用户未配置时跳过并记录。
- 先用固定夹具和 httptest 补齐契约；真实数据样例只能来自有来源、时间、SHA、Run ID 的脱敏真实响应。
- 多证券/多 Provider 批量诊断必须使用受控异步任务、限速和并发控制，不扩大同步 HTTP 请求。
- 每轮更新本节任务状态、运行验收日志和黄金样例记录；完成项写提交 SHA，未完成项保留负责人/依赖/验收条件。
- PR #1 保持 Open Draft，不执行合并。


### 本轮任务状态更新（2026-10-30）

- 比较器已接入真实黄金夹具测试入口；现有两侧 degraded 样例被明确拒绝为 `not_comparable`，不会伪装成 matched。
- 比较器已增加 OHLC 正数/有限值、OHLC 区间关系、非负成交量、非负有限成交额校验。
- 固定日 K 夹具显式记录 `volume_unit=shares`，测试转换时读取夹具单位声明，不再在适配代码中硬编码推断。
- 新增空交易日集合、非法日期、重复日期、NaN/Inf OHLC 和负金额边界测试。
- 对应提交：`35d442fa1049011219d38f2932bc3dcb611d1bca`（比较器首次接入与任务清单）；本次补充提交 SHA 以远端分支最新 HEAD 为准。
- 本次新增代码尚未触发或完成新 SHA 的 CI；不记录为测试通过。下一项代码工作进入统一诊断 `checks` 的分层设计与实现前置梳理。


### 最新提交检查结果补记（2026-10-30）

- 中间提交 `35d442fa1049011219d38f2932bc3dcb611d1bca` 的 Go suite 失败原因已定位：仅 `cross_provider_daily_compare_test.go` 存在多余空行，触发 gofmt 检查；Docker market-data 镜像构建在同一 gofmt 前置检查处终止，并非 Dockerfile 或业务构建错误。
- 随后的 `7e3b6c2c8cb148106d4f5f98fa4184d0c52e795b` 已排队执行全量工作流；该 SHA 结果以 Actions 实际结论为准。
- 本次修复仅规范 Go 格式，不调整业务逻辑；修复提交 SHA 以分支最新 HEAD 为准。


### 北交所 Provider 代码映射矩阵复核（2026-10-30）

现有 `TestDomesticStockProviderBeijingSymbolSupportMatrix` 和板块代码映射测试已明确以下**代码级**结论：

| Provider | BJ 代码映射 | 当前代码结论 | 在线数据验收 |
|---|---|---|---|
| Sina | 不支持 | not_supported | 未执行 |
| Tencent | 不支持 | not_supported | 未执行 |
| NetEase | 不支持 | not_supported | 未执行 |
| BaoStock | 不支持 | not_supported | 未执行 |
| Pytdx | 支持 `bj######` | 已映射 | 未证明上游在线可用 |
| Hithink | 保留 `######.BJ` | 已映射 | 需要 Key/权限 |
| TickFlow | 保留 `######.BJ` | 已映射 | 需要 Key/权限 |
| Tushare | 保留 `######.BJ` | 已映射 | 需要 Token/积分 |
| Eastmoney | 映射为 `0.######` | 已映射 | 公共端点在线可用性待复验 |

代码映射不等于供应商承诺支持，也不等于真实请求成功。BSE 定向诊断遇到 Sina/Tencent/NetEase/BaoStock 明确的交易所不支持错误时，结果应标记 `not_supported`，而不是 `failed`；适配器必须在发出上游 HTTP 请求前拒绝该证券。其他映射支持 Provider 仍需真实核对权限、代码身份、交易日期、OHLC、成交量单位和质量状态。


### 北交所诊断分类回归的格式检查修复（2026-10-30）

- 提交 `bdc3e8820e188926cdb4ea17172c5ce5b80b9831` 的 Go suite 未进入编译/测试阶段，market-data Job 的 gofmt 发现 `provider_probe_test.go` 新增类型前有多余空行。
- 同步的 Docker market-data 镜像在相同 gofmt 前置步骤停止，不是代码编译或镜像运行问题。
- 已清理测试文件格式；下一 SHA 重新执行 Go suite 和镜像构建。


### 覆盖率分母契约首批落地（2026-10-30）

主动诊断结果新增可选字段 `coverage_total`、`coverage_returned`、`coverage_validated`、`coverage_missing`、`coverage_excluded`。字段定义：

- total：可证明的预期样本总量。
- returned：该分母范围内源端实际返回数量。
- validated：返回且通过身份/质量校验的数量。
- missing：预期但源端未返回的数量。
- excluded：已返回但被明确排除的数量。
- ratio = validated / total；分母未知或为 0 时不输出比例和分母字段。

当前先在市场广度探测落地：total 使用快照的 TotalSecurities，returned/validated 使用 CoveredSecurities，missing 为两者差值，excluded 为 0。其他能力未取得可信分母时继续不输出，不以分页数或样本数冒充全市场覆盖率。后续按各业务快照的真实总数/排除数逐项扩展。


### 最新代码推进状态（2026-10-30）

- 比较器接入和边界校验：P0-1/P0-2 已完成代码侧工作；真实跨 Provider valid 黄金样例仍未取得，不能宣布真实 matched 验收通过。
- 北交所：已复核现有映射矩阵，并将明确不支持的 BJ Provider 探测归类为 not_supported；在线权限和数据样本仍待验。
- 覆盖率：市场广度已输出 total/returned/validated/missing/excluded 与 validated/total；其他能力只在获得可信分母后逐项扩展。
- 最近相关代码提交：`35d442f`、`7e3b6c2`、`8226d6d`、`bdc3e88`、`5cc765d`、`69736d1`。其中部分中间提交因 gofmt 失败或被后续提交取消；最终验收只看分支最新 SHA。


### 覆盖率字段提交的格式检查修复（2026-10-30）

- `69736d1baaf8ef8dffe9b373d6edbcb5ba02d9c5` 的 Go suite 在 gofmt 阶段失败，涉及 ProviderProbeItem 与 providerProbeEvidence 字段对齐，以及 Service 测试函数前多余空行。
- 已按 gofmt 输出统一字段对齐并清理空行；本次失败未进入编译、单测或 Docker 镜像实际构建阶段。


### 北交所证券代码直指定路径补齐（2026-10-30）

BSE 的 not_supported 识别同时覆盖 `markets=["BSE"]` 和仅指定 `symbols=["920001.BJ"]` 两种请求形式。比较器 NaN/Inf/负金额边界测试按 gofmt 规范排版。对应新提交以最新 HEAD 为准。


### 创业板/科创板真实样例采集（2026-10-30，Run 41042950111）

从 Live provider probe artifact 固化了创业板 `300750.SZ` 和科创板 `688981.SH` 的真实报价与日 K 样例，新增 `testdata/live_market_cross_market_golden_20261030.json`：

- Sina/Tencent 在两个板块的 quote 和 daily_bars 均返回；Sina quote 为 valid，Tencent quote 为 degraded（成交量单位推断/volume_ratio 不支持）。
- Sina/Tencent 日 K 均为 degraded：成交额缺失且 amount_unit=unknown；Sina 另有 Provider 日 K 响应未回显证券代码的契约告警。
- 两侧日 K 成交量单位均为 shares，复权口径均为 none；样例覆盖 2026-10-27 至 2026-10-29 三个交易日。
- 新测试把这些真实样例送入统一比较器，要求结果为 `not_comparable / quality_status_must_be_valid`。这证明拒绝降级数据的链路，不代表日 K 已对账成功。
- 采集来源：commit `720f2fc685376e29452ce3742ba398ff79089a03`，GitHub Actions Live provider probe Run `41042950111`。原始脱敏报告作为 workflow artifact 保存。

本轮同一 Run 的其他真实 Provider 结论：Eastmoney stock_search/quote/delayed quote 返回 HTTP 502；NetEase quote DNS 失败、历史 CSV HTTP 502 后按现有策略 skipped。此类公共源故障不归因于缺少 Key/Token。


### 接续检查与本轮修复（2026-11-02）

**当前远端基线（修复前）：** `04932a7aed4bcc4bac45ef2c5f2042833e2a4708`。PR #1 仍为 Open Draft，未合并。

已确认前序开发结果：
- 比较器真实夹具入口和输入边界校验已存在；600036.SH、300750.SZ、688981.SH 的 Sina/Tencent 日 K 因 amount 缺失、amount_unit unknown 保持 degraded，测试要求 not_comparable。
- 创业板、科创板真实样例来源为 Live provider probe Run `41042950111`，源提交 `720f2fc685376e29452ce3742ba398ff79089a03`。两市场 quote 返回，但 Tencent quote degraded；两侧日 K 均 degraded。没有真实 matched 证据。
- 北交所明确不支持 Provider 的诊断状态已覆盖 markets=BSE 与 symbols=920001.BJ 两种入口；Sina/Tencent/NetEase/BaoStock not_supported，其他代码映射不代表在线能力通过。
- 覆盖率字段已在模型和 OpenAPI 声明：coverage_total/returned/validated/missing/excluded/ratio。当前只有 market_breadth 提供可证明的全市场分母；其他能力继续省略，不能以分页行数替代总数。

**Run 41043889122（SHA 04932a7）检查结果：**
- Go test suite：失败在 market-data 的 gofmt 检查，唯一文件为 `internal/provider/live_market_golden_test.go`，新增测试函数前存在多余空行；没有进入该模块的编译/单测阶段。
- Docker market-data image：同一 gofmt 前置检查失败，未进入镜像编译阶段。
- Go 根模块、go-core、MariaDB migration/PIT persistence：通过。
- WebUI checks、Go/Python data quality parity：通过。
- Live provider probe workflow：success，但真实探测内部仍记录 Eastmoney HTTP 502、NetEase DNS/HTTP 502；不能视为所有 Provider 可用。
- 已移除 `live_market_golden_test.go` 的多余空行；本次修复 SHA 的完整 Go suite 与 Docker 结果待新 Actions 确认。

**下一开发项：** 实现统一诊断 checks 过滤。先完成请求校验、响应回显、向后兼容和独立检查结果模型设计，再按 connectivity/contract/quality/freshness/cross_provider/storage 拆分执行，不能只增加一个未生效的请求字段，也不能为每个 check 重复请求上游。


### 2026-11-02：统一诊断 checks 过滤首批实现

- `POST /api/v1/data-sources/probe` 新增 `checks` 白名单：connectivity、contract、quality、freshness、cross_provider、storage。
- 空 checks 保持兼容，默认选择全部六类；未知值、重复值返回 400；响应回显排序后的 requested_checks，并输出 check_results。
- 单次 Provider 请求结果被多个检查维度复用，不会按 check 数量重复请求上游。
- connectivity/contract/quality 根据已有 Provider 结果和错误阶段分类；storage 在 MariaDB 写入成功后标记 passed，未配置存储标记 not_configured。
- freshness 当前只记录 ProviderTime 和 age_seconds；尚未定义各能力的 freshness SLA，因此即使有时间戳也保持 skipped（freshness_sla_not_configured），不能将“有时间戳”当作“数据新鲜”。
- cross_provider 当前明确 skipped（cross_provider_snapshot_not_captured）：线上诊断 job 尚未把逐日 K 线样本传入比较器。固定黄金夹具仍独立覆盖比较器，不伪装成在线跨源诊断。
- 当前 checks 是诊断结果维度过滤，不会减少所选 capability 的 Provider 请求；后续可在异步诊断任务中按 check 精确规划数据采集范围。
- 新增 Service/HTTP 测试覆盖默认/指定 checks、未知/重复值、分类和未实现能力显式 skipped。完整 Go suite 与 Docker 结果以本提交 SHA 的 Actions 为准。


### 2026-11-02：checks 在线跨 Provider 对账接入

- 日 K 探测在一次 Provider 请求中捕获归一化 snapshot（仅在所有样本成交量存在时），请求结束后按相同证券两两调用 `CompareCrossProviderDailyBars`。
- check_results 记录参与 Provider、状态、比较日期、字段数量和 mismatch 数量；degraded/unknown 样本由比较器拒绝，输出 skipped，不计为通过。
- 若不足两个不同 Provider 返回同证券日 K，则输出 `cross_provider_pair_unavailable`，不会伪造 matched。
- 本轮只比较同一请求样本和 `AdjustmentNone` 口径；其他复权口径、跨市场与多证券异步对账仍待扩展。


### checks 首批实现编译修复（2026-11-02）

- 提交 `ce4fdd76400d157ee4169481097f2ef438a65309` 因模型编辑时遗漏 `type ProviderProbeRun struct {` 声明，Go 编译与 market-data Docker 构建失败；MariaDB PIT Job 也因同一编译错误终止，不是数据库迁移故障。
- 已恢复 ProviderProbeRun 结构体声明。修复 SHA 的 CI 结果待确认。
- 此次失败不作为功能测试通过证据；后续需重新验证 Go suite、MariaDB PIT、Docker 和 Go/Python parity。


### checks 在线对账阶段的格式修复（2026-11-02）

- `ce17ac11cd559385de24d4e0aa284904d1c6ab5a` 的 market-data gofmt 检查发现 4 个 Go 文件仅有结构体字段对齐和测试函数前多余空行问题；Go test、Docker 构建因此在 gofmt 前置步骤停止。
- MariaDB PIT persistence 在该 SHA 已通过，说明此前模型语法错误已修复。
- 已统一 gofmt 格式，后续以最新 SHA 的完整 Go suite、Docker 和 parity 结果验收。


### 在线 cross_provider 测试样本修正（2026-11-02）

- 检查 Service 测试夹具后发现：测试日 K 原先 amount 为 nil，Normalize 会按契约将其降级并把 amount_unit 设为 unknown；这会触发比较器 not_comparable，与测试预期 matched 冲突。
- 已为合成 valid fixture 补充单位为 CNY、与 OHLC/成交量量级一致的成交额；该数据只用于验证比较器链路，不代表真实行情。
- 最新 SHA 的 CI 仍待确认。


### checks 过滤边界修正（2026-11-02）

- 发现只请求 quote capability、但默认 checks 包含 cross_provider 时，系统仍会输出一条 capability=daily_bars 的 cross_provider skipped 项，破坏 capability 白名单语义。
- 已调整为：只有同一批次显式请求 daily_bars 时才执行跨源比较；否则返回不带 capability 的 skipped 结果（daily_bars_capability_not_requested）。
- 对应回归测试补齐；最新 SHA CI 待确认。


### 2026-11-02：能力白名单回归后的格式修复

- Run `41376039461`（SHA `7a8104a`）的 market-data Job 在 gofmt 阶段仅发现 `provider_probe.go` 一个结果结构体字面对齐和 `provider_probe_test.go` 文件尾多余空行。
- 已按 gofmt 规范修正；未进入该 Job 的编译/单测阶段。MariaDB PIT、根模块和 go-core 在该 SHA 均通过。


### 2026-11-03：统一诊断体系收口（第一批）

本轮以当前 Go 诊断实现为基线，继续完善诊断结果的可解释性，不扩大 Provider 请求数量，也不改变业务数据质量门禁。

- [x] 质量状态为 `unknown` 时，quality check 显式返回 `degraded / quality_unknown`，不再与未执行的 `skipped` 混淆。
- [x] Cross Provider 日 K 对账新增字段级证据：字段路径、比较状态、左右值、差值、容差和不可比较原因。
- [x] 对 amount 缺失或单位不是 CNY 的字段，只返回 `not_comparable` 与原因，不把 Go 默认零值作为左右金额暴露。
- [x] Cross Provider 汇总新增 matched / mismatch / not_comparable 字段计数；既有 `field_count` 仍表示参与结果的字段总数。
- [x] 同步 OpenAPI 字段契约和 Service 回归测试。

本轮边界与未完成项：
- Freshness 仍只输出源端时间和 age_seconds，并明确 `freshness_sla_not_configured`。暂不设置统一固定阈值：报价、日 K、财务披露、资金流和筹码数据具有不同更新语义；交易时段、非交易日、盘后及节假日也必须纳入策略，否则会制造错误过期告警。下一步应先定义按 capability + 时间语义 + 市场交易阶段的 SLA 策略，再启用 passed/degraded 判定。
- Coverage 分母仍只在有可证明总体数量的市场广度能力上输出。分页返回数、抽样数和当前页行数不得冒充全市场分母；其他能力需逐项确认 total、returned、validated、missing、excluded 的来源契约。
- 当前在线 Cross Provider 仍只覆盖同轮、同证券、同复权基准的日 K 快照；字段级证据完善不代表多证券、跨复权基准或真实 Provider 已对账通过。
- 本轮不调整 Python 迁移矩阵，不新增 Python 代码清理工作。

本轮代码改动待本提交 SHA 的 Go test/race/vet、OpenAPI 契约测试和 Docker market-data 构建验证；未完成的检查不得提前标记通过。


### 2026-11-03：统一诊断字段证据格式修复记录

- 首轮 Go test suite 与 Docker market-data 构建在 gofmt 前置检查发现 `internal/model/provider_probe.go` 中模型声明之间缺少一行空行；没有进入 market-data 编译和单元测试阶段。
- 已按 gofmt 要求修复，并补充价格字段 mismatch 的左右值、差值和容差断言。
- 修复后完整 Go test/race/vet、Docker 构建结果以新提交 SHA 的 Actions 为准。

### 2026-11-03：覆盖率分母来源可追溯性（第二批）

- [x] 诊断项新增 `coverage_population` 与 `coverage_source`，使分母不仅有数值，还能说明总体范围和证明来源。
- [x] 当前唯一可证明的主动诊断分母为 `market_breadth`：`coverage_population=provider_universe`，`coverage_source=market_breadth.total_securities`。
- [x] `coverage_total` 取 Provider 返回的 A 股证券全集数量；`coverage_returned` 取全集行情覆盖数量；`coverage_validated` 表示通过市场广度聚合快照契约校验的覆盖数量；`coverage_excluded=0`。
- [ ] 当前 `validateMarketBreadth` 要求 `CoveredSecurities == TotalSecurities`，只有完整覆盖快照能进入计数逻辑，因此成功结果的 `coverage_missing` 必然为 0、`coverage_ratio` 必然为 1。部分覆盖目前会作为能力失败返回，无法在失败项中保留部分计数。
- [ ] 下一步需让 Provider/诊断路径传递可验证的部分覆盖证据，同时不放宽正常业务接口质量门禁；诊断输出还需区分总体覆盖率与字段级质量。聚合快照通过校验不代表每个字段均有效。
- [x] 新字段仅在总量与计数关系通过一致性校验、且分母来源非空时输出；未知总体继续省略全部覆盖率字段。
- [ ] 其他能力不得复用该分母定义。后续必须分别找到全量证券主数据、指数成分、行业成分、财报应披露记录等权威总体来源，才能输出对应覆盖率。
- [ ] 需继续检查 MarketBreadth 聚合 Provider 是否有能力在部分数据返回时返回降级快照；若只返回完整快照或整体失败，应在 Provider 级别明确记录这一限制。
- 验收边界：当前字段是“市场广度快照覆盖率”，不是 quote Provider 的全市场逐字段质量率，也不是全市场证券目录覆盖率。

### 2026-11-03：市场广度部分覆盖证据（第三批）

- [x] TickFlow 市场广度 Provider 不再因报价数量少于证券池总量而丢弃整份响应；仍严格拒绝重复证券、证券池外证券及不合法证券代码。
- [x] `covered_securities` 表示实际返回且通过证券身份校验的唯一证券数；`total_securities` 来自 `CN_Equity_A` 证券池元数据。
- [x] 缺失证券不参与涨跌分类，分类计数只对实际返回证券求和；未分类证券计入 `unclassified_count`，不补零、不推测涨跌。
- [x] 零返回也保留可信证券池分母，允许 Provider 报告日期缺失；该结果仅供诊断，不代表业务数据可用。
- [x] 部分总体覆盖标记为 degraded，并且不暴露部分成交额作为全市场成交额。
- [x] 主动诊断使用允许部分总体覆盖的结构校验，正常业务 `MarketBreadth` 仍使用严格校验，部分总体覆盖不能进入业务快照/缓存。
- [x] 覆盖率口径：`coverage_returned=covered_securities`；`coverage_missing=total_securities-covered_securities`；`coverage_excluded=unclassified_count`；`coverage_validated=coverage_returned-coverage_excluded`。该 validated 仅表示身份和必需价格字段可用于涨跌分类，不代表成交额、时间戳、涨跌停等所有字段均完整。
- [ ] 完成 Service 回归、Go/race/vet、OpenAPI 和 Docker 验收后，再标记本批通过。


### 2026-11-03：跨 Provider 对账来源证据补齐

- Cross Provider 日 K check result 除兼容保留的组合 `provider` 字段外，新增 compared_symbol、left/right provider、left/right source_version、adjustment_basis 和双方 quality_status。
- 每条字段比较现在能独立定位两侧数据来源、版本、证券和复权口径，不需要通过组合字符串解析或再与 ProviderProbeItem 跨数组关联。
- 字段级左右值、差值、容差和 not_comparable 原因维持原语义；degraded/unknown 输入仍然拒绝作为 matched 基准。
- 对应模型、OpenAPI 和 Service 回归测试已同步；以新 SHA 的 market-data test/race/vet、OpenAPI 和 Docker 检查为准。


### 2026-11-03：统一诊断第一批收口与验收状态

**代码侧完成：**

- Freshness SLA：纯函数 Evaluator、按数据域策略、市场交易阶段上下文解析和 Provider Probe 接入已完成；`provider_time` 与 `fetched_at` 分离，采集时间不参与 freshness 年龄计算。
- 特殊交易边界：覆盖开盘/收盘集合竞价、竞价暂停、午休、非交易日、全天停牌、盘中临停、复牌、源端时间超前；Service 层使用 fake TradingCalendar/SecurityTradingStatus Provider 验证停牌、临停、复牌、无停牌事件及证券状态缺失映射。
- 新股首日：当前没有足够可信的首日交易状态输入，不设置隐式 freshness 宽限期；测试要求超出普通 Quote SLA 时仍为 stale。
- P0-7 覆盖率：Market Breadth 输出四个字段级维度（quote_response、price_classification、quote_timestamp、amount），每项包含 total/returned/validated/missing/excluded/ratio；所有维度共享证券池权威分母。其他能力尚无可证明总体分母时继续不输出比例。
- P0-8 Cross Provider：日 K 对账结果包含证券、左右 Provider、双方 SourceVersion、复权口径、双方 QualityStatus；字段比较保留左右值、差值、容差和不可比较原因，历史 JSON 兼容。

**最新验收 SHA：** `76fd6f2fc683c05869c224ec6328564eca0c84c1`

**Run 41573144555 / 41573144564 / 41573144577 / 41573144590：**
- Go test suite：通过；market-data 和 go-core 的 gofmt、tests、race、vet 均通过。
- MariaDB Migration / PIT persistence：通过。
- Docker Compose、Go market-data 与 analysis API 镜像构建：通过。
- Live Provider probe、WebUI checks、Go/Python parity：通过。
- 真实 Provider 工作流通过只表示本轮在线探测流程完成，不等于取得停牌/临停/复牌的历史真实样例；真实特殊交易事件仍需真实源数据验收。

**第一批状态：** 代码实现与自动化检查完成；真实特殊交易样例属于外部数据验收待办，不阻塞进入第二批。

**第二批下一任务：Quote 与 Daily Bars 边界收口。** 先审计现有 Normalize/Validate/Service 三层对字段边界、跨字段关系、日期区间、复权口径、单位和缺失字段的处理，再补齐边界测试；发现行为缺陷后才修改业务规则，避免只为增加测试而改变现有数据契约。随后推进 Fundamentals/Valuations 校验。


### 第二批进度：Quote / Daily Bars 边界收口（2026-11-03）

**本轮审计发现并修正：**

- `normalize.ValidateQuote` 原先用 map 遍历必填价格和可选数值字段，导致多个字段同时异常时，返回的第一个错误不稳定；改为固定字段顺序，保持校验规则不变。
- Quote 可选数值错误信息此前对所有字段都写“非负”，但 change/change_percent 允许负值；现拆分为“必须有限”与“必须非负”两类准确错误。
- 增加负涨跌额/负涨跌幅合法性、负量比/换手率非法性和确定性首错回归测试。
- Daily Bars 增加缺失/负成交量、成交量单位、负金额、金额/成交量零值不匹配、金额单位、复权口径、周期、非有限涨跌幅等边界测试；并覆盖日期区间包含端点、重复交易日丢弃和 limit 保留最新数据的行为。
- 本轮只修正确定性和错误描述，不改变现有数据接纳/丢弃规则。

后续继续检查 Service/Provider 层的源字段身份、复权基准、时间戳、当日未收盘 K 线和停牌日行为，避免 Normalize 单测通过但 Service 流程覆盖不足。


Quote/Daily Bars 边界审计补充：Service 层 DailyBars 对 Provider 返回空 Symbol 的处理原先只根据请求参数补齐 Symbol，但没有同步降低质量；这与 Quote 的缺失身份降级策略不一致。现改为补齐请求 Symbol 的同时标记 QualityDegraded，并在行级 QualityReasons 和响应 warnings 中明确记录 `provider_symbol_missing_assumed_from_request`。非空但错误/不匹配的 Symbol 仍然触发 Provider 整体契约失败，不会被覆盖。


质量状态门禁补充：Quote/Daily Bars 在使用 Provider fallback、补齐请求 Symbol、标记缺失成交额或标记当日未完成 K 线时，统一只允许将空值/valid 提升为 degraded；已有 unknown、stale、invalid 不得被覆盖，尤其不能把 invalid 通过 fallback 逻辑降成 degraded 后继续接纳。新增 Quote 与 DailyBars 回归测试，使用 fallback + 缺失 Symbol + 缺失 Amount 的 invalid 输入，确认仍然拒绝。
Normalize 层另发现一个相同类别的边界：DailyBar 缺失 Source 时会将 QualityStatus 无条件改为 unknown，可能把 invalid 行恢复为可接纳状态。现调整为只对空值/valid 设置 unknown，保留 degraded/stale/unknown/invalid 原状态，并增加 invalid + source_missing 的丢弃回归测试。


### 第二批补充：Fundamentals 采集时间边界

- Fundamentals snapshot 已要求 source、source_version 和 fetched_at 非空，但此前缺少 fetched_at 未来时间校验；这与 Valuations 已有的未来采集时间门禁不一致。
- 现增加 5 分钟时钟偏差容忍：超过当前 UTC 时间 5 分钟的 fetched_at 拒绝；不使用 Provider 报告期末、公告日期或 available_at 替代采集时间。
- 增加未来 fetched_at 回归测试。该校验只约束采集时间，不改变财报报告期、披露日期及 PIT 可用时间的独立语义。


### 第二批补充：资金流与筹码数据采集时间边界（2026-11-03）

本轮审计已确认个股资金流已有以下校验：证券身份、Provider 日期格式与未来日期、来源/版本、采集时间未来偏差、观测数量与 1/5/10 日指标关系、有限金额，以及缺少可验证发布时间时禁止标记 valid。

新增补齐两处一致性缺口：

- 筹码分布快照此前检查了日期、成本区间、比例范围、来源版本和采集时间非空，但未拒绝未来 fetched_at。新增 5 分钟时钟偏差容忍，避免缓存中的异常采集时间被接受；服务实时回包仍以本服务收到响应的时间重写 fetched_at。
- 市场级资金流历史此前检查日期区间、日期唯一且升序、金额有限、来源版本和必须 degraded，但未拒绝未来 fetched_at。新增相同的 5 分钟时钟偏差门禁，保护 Provider 回包及缓存读取。

两项均不推断 Provider 发布时刻，不将 fetched_at 当作 available_at；市场资金流在缺少可验证源端快照时间时仍保持 degraded。新增未来时间回归测试。
