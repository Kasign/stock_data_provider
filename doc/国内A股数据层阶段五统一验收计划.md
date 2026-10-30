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
