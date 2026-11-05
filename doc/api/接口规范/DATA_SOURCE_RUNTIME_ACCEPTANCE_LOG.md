# 数据源真实环境验收记录

> 当前状态核对（2026-11-03，基线 `b423c1ef`）：本文早期记录是对应日期的历史运行证据，不代表当前 HEAD 的实时状态。统一 Probe、Reporter、字段诊断、Freshness 接入、DailyBar 跨 Provider 比较和可选多复权口径探测已有代码实现；真实 Provider 是否通过必须查看对应 HEAD 的最新 Live workflow 单项结果。CI 的 `continue-on-error` 或 workflow 总体 success 不能替代逐 Provider 结果。当前代码任务与优先级以 `doc/国内A股数据层阶段五统一验收计划.md` 为准。多复权口径 Probe 的 CI 和真实 Provider 复权结果尚未在本记录中判定通过。

- 适用分支：`feat/full-go-data-providers-api-switch`
- 文档更新日期：2026-10-30
- 最近代码基线：`4ee2df18621e27b2fd0b1706f69dff78f35b4dfa`
- 当前结论：已建立国内 A 股主要可选 Provider 的在线验收入口；除非本文件追加了带提交 SHA、日期和脱敏证据的记录，否则真实请求状态均为“未执行”。

## 1. 验收状态口径

| 状态 | 判定标准 |
|---|---|
| 未执行 | 没有真实请求证据 |
| 请求成功待核对 | HTTP/业务请求成功，但字段、单位、时间或覆盖尚未核对 |
| 通过 | 字段、单位、时间、覆盖和质量契约均符合预期 |
| 降级可接受 | 返回 degraded/unknown，原因明确，且不会污染可信数据 |
| 不通过 | 返回错误数据、错误单位、错误时间或质量状态丢失 |
| 权限阻塞 | Token、积分、Cookie、API Key 或账号权限不足 |
| 限流阻塞 | 上游限频或并发限制阻塞 |
| 暂缓 | 不属于当前国内 A 股验收范围 |

**代码已实现、配置已启用、真实请求成功、质量验收通过是四种不同状态，不得相互替代。**

## 2. Provider 验收矩阵

| Provider | 在线入口覆盖 | 真实请求状态 | 凭据 |
|---|---|---|---|
| TickFlow | quote、核心指数、全市场广度、申万一级行业排行 | 未执行 | API Key |
| Tushare | 证券资料、交易日历、日 K、历史交易状态、财务、估值、指数目录/成分/日 K | 未执行 | Token、对应积分/接口权限 |
| 同花顺 Hithink | quote、日 K、核心指数、人气榜、估值、交易日历、涨停池、龙虎榜、行业目录/成分/日 K、财务 | 未执行 | API Key、对应接口权限 |
| 雪球 | 人气榜、飙升榜 | 未执行 | Cookie 可选，视登录态/限流而定 |
| 妙想 | 筹码分布、主力资金流 | 未执行 | API Key |
| 东方财富、腾讯、新浪、网易、Pytdx、BaoStock | 当前主要默认数据链路，仍需按能力执行固定样本与质量对账 | 未执行 | 视接口而定；部分无需凭据 |
| Yahoo Finance | 海外市场能力 | 暂缓 | 不属于当前国内 A 股阶段二范围 |

本矩阵只描述自动化入口覆盖，不代表上游权限已开通，也不代表已通过真实数据验收。

## 3. 在线验收入口

所有真实请求测试均默认跳过。必须显式设置对应开关后才会访问上游。在 `services/market-data` 目录执行：

### TickFlow

```bash
MARKET_DATA_LIVE_TICKFLOW_TESTS=1 \
MARKET_DATA_TICKFLOW_API_KEY="$TICKFLOW_API_KEY" \
go test ./internal/provider -run '^TestLiveTickFlowCapabilities$' -count=1 -v
```

可选 URL：`MARKET_DATA_TICKFLOW_API_URL`、`MARKET_DATA_TICKFLOW_FREE_API_URL`。兼容环境变量：`TICKFLOW_API_KEY`、`TICKFLOW_BASE_URL`、`TICKFLOW_FREE_BASE_URL`。

### Tushare

```bash
MARKET_DATA_LIVE_TUSHARE_TESTS=1 \
MARKET_DATA_TUSHARE_TOKEN="$TUSHARE_TOKEN" \
go test ./internal/provider -run '^TestLiveTushareCapabilities$' -count=1 -v
```

可选 URL：`MARKET_DATA_TUSHARE_API_URL`。兼容 Token 环境变量：`TUSHARE_TOKEN`。财务能力固定抽查 2025 年第四季度；若无权限、尚无数据或上游不提供，必须记录真实失败原因，不得改写成通过。

### 同花顺 Hithink

```bash
MARKET_DATA_LIVE_HITHINK_TESTS=1 \
MARKET_DATA_HITHINK_API_KEY="$HITHINK_API_KEY" \
go test ./internal/provider -run '^TestLiveHithinkCapabilities$' -count=1 -v
```

可选 URL：`MARKET_DATA_HITHINK_BASE_URL`。兼容 Key 环境变量：`HITHINK_FINANCE_API_KEY`。验收包含交易日历、涨停池、龙虎榜、行业目录及其成分/日 K；其中金额单位未获供应商契约确认时，质量状态必须保留 degraded/provider_defined。

### 雪球

```bash
MARKET_DATA_LIVE_XUEQIU_TESTS=1 \
go test ./internal/provider -run '^TestLiveXueqiuHotRankings$' -count=1 -v
```

可选配置：`MARKET_DATA_XUEQIU_COOKIE`、`MARKET_DATA_XUEQIU_BASE_URL`。Cookie 仅在接口要求登录态时提供。

### 妙想

妙想复用统一的公共 Provider 真实探测入口，避免对按调用配额计费的自然语言接口重复发起请求：

```bash
MARKET_DATA_LIVE_PROVIDER_TESTS=1 \
MARKET_DATA_LIVE_MX_TESTS=1 \
MARKET_DATA_MX_API_KEY="$MX_API_KEY" \
go test ./internal/provider -run '^TestLiveProviderProbe

## 4. 安全与执行约束

- 凭据只从环境变量或 GitHub Actions Secrets 注入；禁止写入仓库、测试输出、文档或命令历史。
- 测试日志只记录 Provider、能力、状态、耗时、记录数、质量状态/原因和脱敏错误摘要；不得输出 API Key、Token、Cookie 或完整请求 URL。
- 真实请求测试默认跳过，不影响普通 CI。
- 不因单次请求成功而打开 Provider 默认开关；必须先完成字段和质量核对。
- 供应商空结果不能一概判为通过。需要区分真实无数据、非交易日、未发布、权限不足和接口异常。
- 由成分股行情聚合得到的行业排行、无法确认金额单位的资金数据、缺少源端发布时间的数据，必须保留 degraded/unknown，不得伪装成 fully valid。
- 对接口限流/配额敏感的来源按顺序串行验收，不并发轰击上游。

## 5. 每次真实验收必须记录

每个 Provider/capability 至少记录：

1. 日期、环境、分支、代码提交 SHA。
2. Provider、能力、脱敏证券代码、请求日期/区间。
3. HTTP/业务状态、耗时、重试次数和错误分类。
4. 返回行数、源端总数、覆盖率、分页终止证据、重复/排除数量。
5. 关键字段存在率及 null/zero 分布。
6. 价格、成交量、成交额单位和转换依据。
7. adjustment_basis、adjustment_method。
8. provider_time、report_date、announcement_time、fetched_at。
9. quality_status、quality_reasons、fallback 来源。
10. 验收结论、失败原因及后续处理。

不得在本文件记录真实凭据或未经脱敏的个人数据。

## 6. 固定样本范围

- 沪市主板、深市主板、创业板、科创板、北交所。
- 正常交易日、停牌日、非交易日。
- 除权除息前后及不同复权基准。
- 成交额缺失、成交量缺失、真实零值。
- 上游空响应、分页截断、重复行、超时和限流。
- 财务报告期与公告日不一致、财务修订。
- 新股上市、ST/风险警示、退市和涨跌幅特殊阶段。

## 7. 当前执行记录

| 日期 | Provider | 能力 | 代码 SHA | 结果 | 说明 |
|---|---|---|---|---|---|
| — | — | — | — | 未执行 | 当前阶段尚未注入真实凭据，未发起可选 Provider 真实请求 |

后续只追加有真实请求证据的记录。阶段二的代码入口和 CI 通过情况不得填入本表作为 Provider 在线验收通过证据。
 -count=1 -v
```

可选 URL：`MARKET_DATA_MX_API_URL`；未设置时使用 Provider 内置默认 HTTPS endpoint。兼容 Key 环境变量：`MX_APIKEY`。筹码和资金流是补充数据，验收必须校验 FetchedAt/ProviderDate/观察数，并保留发布时间不可用导致的 degraded 状态。

## 4. 安全与执行约束

- 凭据只从环境变量或 GitHub Actions Secrets 注入；禁止写入仓库、测试输出、文档或命令历史。
- 测试日志只记录 Provider、能力、状态、耗时、记录数、质量状态/原因和脱敏错误摘要；不得输出 API Key、Token、Cookie 或完整请求 URL。
- 真实请求测试默认跳过，不影响普通 CI。
- 不因单次请求成功而打开 Provider 默认开关；必须先完成字段和质量核对。
- 供应商空结果不能一概判为通过。需要区分真实无数据、非交易日、未发布、权限不足和接口异常。
- 由成分股行情聚合得到的行业排行、无法确认金额单位的资金数据、缺少源端发布时间的数据，必须保留 degraded/unknown，不得伪装成 fully valid。
- 对接口限流/配额敏感的来源按顺序串行验收，不并发轰击上游。

## 5. 每次真实验收必须记录

每个 Provider/capability 至少记录：

1. 日期、环境、分支、代码提交 SHA。
2. Provider、能力、脱敏证券代码、请求日期/区间。
3. HTTP/业务状态、耗时、重试次数和错误分类。
4. 返回行数、源端总数、覆盖率、分页终止证据、重复/排除数量。
5. 关键字段存在率及 null/zero 分布。
6. 价格、成交量、成交额单位和转换依据。
7. adjustment_basis、adjustment_method。
8. provider_time、report_date、announcement_time、fetched_at。
9. quality_status、quality_reasons、fallback 来源。
10. 验收结论、失败原因及后续处理。

不得在本文件记录真实凭据或未经脱敏的个人数据。

## 6. 固定样本范围

- 沪市主板、深市主板、创业板、科创板、北交所。
- 正常交易日、停牌日、非交易日。
- 除权除息前后及不同复权基准。
- 成交额缺失、成交量缺失、真实零值。
- 上游空响应、分页截断、重复行、超时和限流。
- 财务报告期与公告日不一致、财务修订。
- 新股上市、ST/风险警示、退市和涨跌幅特殊阶段。

## 7. 当前执行记录

| 日期 | Provider | 能力 | 代码 SHA | 结果 | 说明 |
|---|---|---|---|---|---|
| — | — | — | — | 未执行 | 当前阶段尚未注入真实凭据，未发起可选 Provider 真实请求 |

后续只追加有真实请求证据的记录。阶段二的代码入口和 CI 通过情况不得填入本表作为 Provider 在线验收通过证据。


## 8. 统一主动诊断接口开发记录（2026-10-30）

目标分支：`feat/full-go-data-providers-api-switch`。

本轮代码新增/扩展：

- ProviderProbeItem 错误阶段、脱敏错误摘要、HTTP 状态码。
- ProviderProbeRun 未知、跳过、未配置、不支持计数。
- 未注册的核心能力显式返回 `not_configured`；全市场目录扫描在轻量探测中明确 `skipped`。
- 扩展非行情能力：板块、涨跌停池、龙虎榜、公告、交易日历、财务、估值、人气榜、个股资金流、筹码分布、指数目录、证券交易状态、涨跌幅特殊事件。
- 交易日历、公告、财务、估值、人气榜复用业务质量校验函数。

执行限制：本轮没有发起任何真实 Provider 请求，没有要求或读取新的 Key/Token。GitHub Actions 是否覆盖最新 SHA 仍需确认；未取得最新 SHA 的成功结果前，不记录为构建或测试通过。实时 Provider workflow 继续按无凭据跳过策略执行，不能将 workflow success 视为全部 Provider 可用。


## 9. 统一诊断接口请求范围与 Handler 自检（2026-10-30）

目标分支：`feat/full-go-data-providers-api-switch`。

- `POST /api/v1/data-sources/probe` 保持原路由和空请求体默认行为。
- 新增可选请求体：`scope`（`all`/`core`）、`capabilities` 白名单、`timeout_seconds`（1–30 秒，默认 12 秒，按单项 Provider 请求计时）。
- 指定 `capabilities` 时仅执行列表内能力；未指定时由 scope 选择默认集合。core 默认集合为 quote、daily_bars、stock_basic、trading_calendar、index_quotes、market_breadth。
- 能力白名单未知项、重复项、core 范围外能力、非法 JSON、未知字段、非对象 JSON、多个 JSON 值、超时越界和超过 8 KB 的请求体均返回 400。
- 响应批次记录 scope、requested_capabilities、timeout_seconds；旧历史记录缺少这些字段时通过 omitempty 保持 JSON 输出兼容。
- OpenAPI 已补充请求体、能力枚举、超时边界和 API Key 安全声明。
- 新增 httptest：鉴权、空 body 兼容、core 过滤、能力白名单、非法请求、超大 body；Service 层增加能力范围与参数校验测试。
- 本次只使用无 Provider 的测试服务，不触发真实供应商请求，不需要 Key/Token。

后续仍需实现：checks 过滤、多板块/多证券批量过滤，以及逐项请求摘要和完整覆盖率分母契约。单板块/单证券定向过滤和主要能力的样本/行数/时效证据已在后续章节补充；未覆盖能力仍保持证据字段为空，不以系统采集时间冒充源端时间。CI 状态以当前分支最新 SHA 对应工作流为准。


## 10. 核心行情探测证据字段（2026-10-30）

ProviderProbeItem 新增 `sample_symbol`、`sample_market`、`sample_date`、`rows_returned`、`rows_validated`、`provider_time`。

- quote：记录请求样本 `600519.SH`、市场 SH、返回/校验数及 ProviderTime。
- daily_bars：记录请求样本 `600036.SH`、最新有效交易日、原始返回数、校验后数量及可用 ProviderTime。
- 不使用 FetchedAt 或 CheckedAt 冒充 ProviderTime；上游未提供时间时保持空值。
- 其他能力的记录数、覆盖率、请求摘要和时效证据尚未全部接入，后续按快照契约逐项扩展。

最新代码提交需以 PR #1 的当前 HEAD 为准；本节不记录未经 CI 确认的构建结论。


## 10. 市场与证券定向诊断请求（2026-10-30）

统一诊断 API `POST /api/v1/data-sources/probe` 新增请求字段：

- `markets`：当前最多一个板块，支持 `SH_MAIN`、`SZ_MAIN`、`CHINEXT`、`STAR`、`BSE`。
- `symbols`：当前最多一个 A 股证券代码，格式为六位代码加 `.SH`、`.SZ` 或 `.BJ`。
- 指定任一过滤条件时，必须同时显式指定 `capabilities`，且所有能力必须属于单证券范围；市场广度、指数、行业排行等全市场能力不允许与单证券过滤混用。
- 仅指定板块时选择该板块固定样本；同时指定证券时校验证券代码与板块归属。只指定证券时按证券代码选择样本。
- 响应批次回显 `requested_markets` 和 `requested_symbols`，各项继续返回实际样本证券/交易所证据。

当前约束是单请求单板块、单证券，不代表全板块覆盖率，也不执行全市场扫描。多证券批量探测暂缓，待异步任务与限速策略设计后再实施。

本节记录的是代码能力，不是 Provider 真实请求验收结果。未配置 Key/Token 的 Provider 仍不发起真实请求；代码实现、CI 通过和真实数据质量通过分别记录。


## 11. 跨 Provider 日 K 对账契约开发记录（2026-10-30）

- 新增内部比较器 `CompareCrossProviderDailyBars`，输入必须携带 Provider、来源版本、证券代码、复权口径、质量状态和逐日行情。
- 不同证券、不同复权口径、不同交易日期集合、重复交易日、非法日期、成交量单位非 shares 均判为 `not_comparable`，不会将不一致条件误报为行情差异。
- 质量状态必须双方均为 `valid`；任何一侧为 degraded/unknown 时拒绝作为对账基准。
- OHLC 容差 0.001；成交量容差 50 股；双方均有 CNY 成交额时容差 1 CNY。成交额缺失/单位不明保留字段级 `not_comparable`，整体标记 `partial`。
- 结果状态：`matched`（所有可比较字段通过）、`partial`（核心字段可比较但可选字段缺失/不可比较）、`mismatch`（至少一个字段超差）、`not_comparable`（身份、日期、质量或单位前置条件不满足）。
- 本轮只增加本地静态契约与单测，不执行真实 Provider 请求，不需要 Key/Token；不代表任何供应商线上验收通过。

后续：将同一比较契约接入跨市场黄金样例；补齐创业板、科创板、北交所、停牌、新股及除权日真实样本。批量 Provider 对账应放入受控异步任务，不能扩张同步诊断请求。


## 12. 跨 Provider 日 K 比较器接入固定黄金样例（2026-10-30）

代码基线：`b2a08820e41c4afffdf320e172670421b014f68f`。

- 当前真实黄金夹具为 `600036.SH`，新浪和腾讯均为 `degraded`，成交额为 null/unknown。
- 因比较器要求两侧均为 valid，该夹具必须得到 `not_comparable / quality_status_must_be_valid`，不得通过删除质量状态或伪造成交额来制造 matched。
- valid 输入下的 matched/partial/mismatch 仍由独立合成契约测试验证；合成数据只证明比较器逻辑，不代表真实行情验收。
- 下一步：在同一测试入口继续补充 OHLC 合法性、负成交量、负/非有限成交额边界；完成后再扩展跨市场夹具。

## 13. 阶段五后续任务唯一清单

阶段五完整 P0/P1/P2 任务、依赖关系和验收标准统一维护在《国内A股数据层阶段五统一验收计划》第十五节。本文件只记录实际执行证据，不重复维护第二份计划。每次开发后追加代码 SHA、测试/CI Run ID、Provider 请求结论；没有真实请求证据时保持未执行。


## 14. 比较器输入边界与夹具单位声明补充（2026-10-30）

- 比较器新增 OHLC 必须为正数且有限、High/Low/Open/Close 关系合法、成交量非负、成交额非负且有限的前置校验；非法行情返回 `not_comparable`，不误报为跨源 mismatch。
- 新增空日期集合、非法日历日期、重复日期、NaN/Inf OHLC、负成交量、负成交额回归用例。
- 真实 600036.SH 夹具的两侧来源级 `volume_unit` 显式记录为 shares；夹具转换函数读取该声明，不在测试代码中自行假设。
- 以上是代码和静态夹具契约，不代表新的 Provider 在线验收。新 SHA 的 CI 尚未确认。


## 15. Go 格式检查问题与修复记录（2026-10-30）

- `35d442fa1049011219d38f2932bc3dcb611d1bca` 的 Go suite 中，market-data Job 的 gofmt 检查发现 `cross_provider_daily_compare_test.go` 函数间存在多余空行。
- market-data Docker 镜像构建执行相同 gofmt 前置检查，因此同步失败；并非镜像编译或运行时问题。
- 已移除多余空行，后续以修复 SHA 的 Go test/race/vet 和 Docker 构建结果作为验收依据。


## 16. 北交所定向诊断不支持状态（2026-10-30）

现有 Provider 代码映射测试确认：Sina、Tencent、NetEase、BaoStock 不接受 BJ 代码；Pytdx、Hithink、TickFlow、Tushare、Eastmoney 有代码级映射。代码级映射不代表上游在线能力已验收。

统一诊断对 BSE 样本遇到明确的交易所不支持错误时，现标记 `not_supported / market_not_supported`，计入 NotSupported，不计入 Failed。此分类仅针对适配器在请求前明确拒绝的 BJ 代码；DNS、EOF、HTTP 5xx、权限问题仍按原错误分类。新增 Service 单测覆盖该状态汇总。


## 17. 北交所诊断测试 gofmt 修复（2026-10-30）

提交 `bdc3e8820e188926cdb4ea17172c5ce5b80b9831` 的 Go suite 仅因 `provider_probe_test.go` 新增类型前多一行空白而在 gofmt 检查失败；Docker 镜像构建在相同前置检查处中止。已移除多余空行，后续需以新 SHA 的 Go test/race/vet 和镜像构建结果确认。


## 18. 覆盖率分母字段首批实现（2026-10-30）

主动诊断 ProviderProbeItem 与 OpenAPI 新增 coverage_total/returned/validated/missing/excluded。当前仅市场广度有明确总体证券数，按 TotalSecurities 与 CoveredSecurities 计算；其他能力无可信分母时保持字段缺失。ratio 使用 validated/total，不用 returned/total 替代质量覆盖率。新增 Service 单测校验分母、缺失、排除和比例关系。


## 19. 比较器与覆盖率任务状态更新（2026-10-30）

- 比较器已接入真实 600036.SH 黄金夹具；夹具 degraded 时得到 not_comparable；合成 valid 输入测试覆盖 matched/partial/mismatch。
- 输入边界覆盖正数/有限 OHLC、OHLC 关系、负成交量、负/NaN/Inf 成交额、空/非法/重复日期集合。
- 市场广度诊断新增明确覆盖分母字段；其他能力尚未确认分母时不输出 coverage_ratio。
- 北交所映射矩阵已落到既有测试；不支持 Provider 在 BSE 定向探测中归类为 not_supported。
- 当前最新提交与 CI 结论以后续远端 HEAD 和对应 Actions 为准。


## 20. 覆盖率字段 gofmt 修复（2026-10-30）

提交 `69736d1baaf8ef8dffe9b373d6edbcb5ba02d9c5` 的 market-data gofmt 检查发现两个结构体字段对齐不符合 gofmt，以及测试函数前存在多余空行。已修复格式，需以新 SHA 的完整 Go suite 与 Docker 构建结果复核。


## 21. 北交所证券代码直指定路径补充

BSE 不支持 Provider 的诊断分类同时覆盖市场过滤与证券代码过滤；仅指定 `920001.BJ` 时也应输出 `not_supported`，不能误计 failed。测试使用本地 fake，不触发上游请求。


## 22. 创业板/科创板真实黄金样例（Run 41042950111）

- 已将 `300750.SZ`（创业板）和 `688981.SH`（科创板）的 Sina/Tencent quote 与日 K 脱敏样例固化至 `live_market_cross_market_golden_20261030.json`。
- Sina/Tencent 两侧日 K 均返回 8 条记录，artifact 中保留 3 条样本行；两边质量状态均 degraded，成交额缺失、金额单位 unknown，不能作为 matched 基准。
- 新增回归测试逐市场调用统一比较器，必须返回 not_comparable；不把 quote returned 或 daily bars returned 等同于 quality valid。
- Run 41042950111 真实探测失败项：Eastmoney stock_search、quote、delayed quote HTTP 502；NetEase quote DNS 失败、daily CSV HTTP 502 并按策略 skipped。不是凭据缺失。


## 23. 2026-11-02 接续检查与 gofmt 修复

- 接续时远端 HEAD：`04932a7aed4bcc4bac45ef2c5f2042833e2a4708`；PR #1 Open Draft，未合并。
- Run `41043889122` 的 Go suite 失败仅由 `live_market_golden_test.go` 多余空行触发 gofmt；Docker market-data image 在同一 gofmt 前置步骤失败。根模块、go-core、MariaDB migration/PIT persistence、WebUI checks、Go/Python parity 均通过。
- 已清理该文件空行，等待修复 SHA 的 Actions 结果。
- 创业板/科创板固定样例来源 Run `41042950111`；日 K 两侧均 degraded，成交额缺失，不可作为 matched 证据。
- 当前覆盖率字段在 market_breadth 有显式总体分母；其他能力无可信分母时省略。
- Live probe workflow 的最终 success 不等于所有 Provider 成功：同一批记录 Eastmoney HTTP 502、NetEase DNS/HTTP 502。


## 24. 统一诊断 checks 过滤首批实现（2026-11-02）

- 请求体新增 checks；允许 connectivity、contract、quality、freshness、cross_provider、storage；空值默认全部，未知或重复值 400。
- 响应新增 requested_checks、check_results；历史记录通过 JSON metadata 保存新字段，旧记录缺字段仍可反序列化。
- connectivity、contract、quality 从已有 ProviderProbeItem 和错误阶段归类；storage 根据 MariaDB 实际写入结果归类。
- freshness 尚无按 capability 的 SLA，只记录源端时间/age_seconds，状态保持 skipped；cross_provider 尚未接入在线逐日 bars snapshot，状态保持 skipped。
- 一个 Provider 响应被多个检查维度复用，不会按 check 数量重复调用上游。
- 新增 HTTP/Service 单测；CI 待最新 SHA 确认。


## 25. 在线诊断跨 Provider 日 K 对账

- 日 K Provider 单次请求返回后，Service 将标准化 bars snapshot 放入本轮 evidence，不重复请求。
- 同一证券至少两个 Provider 返回快照时，调用统一比较器；比较结果映射到 cross_provider check_results。
- matched → passed；partial → degraded；mismatch → failed；not_comparable → skipped。
- 结果附 compared_dates、field_count、mismatch_count；无双源样本时明确 skipped。
- 质量门禁保持不变：degraded/unknown 不作为可信对账基准。


## 26. checks 首批实现编译问题

提交 `ce4fdd76400d157ee4169481097f2ef438a65309` 的 Go 编译失败源于 `provider_probe.go` 模型文件缺少 `type ProviderProbeRun struct {` 声明。Go suite、Docker market-data build、MariaDB PIT 集成及 Go/Python parity 均因同一语法错误失败，非数据库/质量算法独立故障。已恢复结构体声明，待新 SHA CI 验证。


## 27. checks 跨源接入格式修复（2026-11-02）

`ce17ac11cd559385de24d4e0aa284904d1c6ab5a` 的 market-data Go suite 和 Docker image build 在 gofmt 阶段失败，涉及 model/service 字段对齐及两个测试文件的多余空行。MariaDB PIT persistence、根模块和 go-core 已通过；修复格式后等待新 SHA 验收。


## 28. cross_provider 合成测试夹具质量修正（2026-11-02）

Service 级 matched 测试的合成日 K 原先缺少成交额；Normalize 按既有契约将其标记 degraded，导致比较器应当拒绝。测试夹具已补充 CNY 成交额（仅合成测试），以验证 valid 输入下在线探测链路的 matched 分支；不作为真实 Provider 证据。


## 29. checks 与 capability 白名单边界修正

当请求只包含 quote 而默认 checks 包含 cross_provider 时，cross_provider 结果不应伪造 daily_bars capability 项。现要求同批次显式选择 daily_bars 才能执行跨源比较；否则返回 capability 为空的 skipped 结果，保持 capability 白名单响应边界。


## 30. 能力白名单回归格式检查修复

Run `41376039461` 的 gofmt 检查发现 cross_provider skipped 结果字面对齐和 Service 测试文件尾多余空行。已修复；该 Run 的 MariaDB PIT、根模块、go-core 通过，market-data Go suite/Docker 尚未进入编译阶段。

## 31. 2026-11-04 凭据型 Provider 真实探测复核与诊断增强

真实探测基线：
- Workflow Run：`41802232526`；对应 Job 的 Provider 探测失败。Workflow 顶层显示 success 是因为该诊断 Job 配置了 `continue-on-error: true`，不能据此认定 Provider 全部通过。
- 分支探测提交：`0a2263bf146ff75cec9f596a6f487d70dd55edb7`。
- 本节只复核 TickFlow、Tushare、妙想三个凭据型 Provider；不重新发起线上请求，不消耗额外 API 配额。

### 31.1 请求协议与 Secret 传递核对

- Tushare：代码使用 POST + JSON body，包含 `api_name/token/params/fields`，与官方 HTTP 接口协议一致。当前真实探测的多个能力返回权限错误（含 code 40203）；权限不足不能直接推断 Token 错误，也不能将所有接口都要求通过。账号积分档位和单接口权限需分别判断。
- TickFlow：代码的 GET 行情请求、POST 批量 Universe 请求及 `x-api-key` 认证方式与官方 SDK 的调用方式一致。当前失败集中在指数行情请求、行情字段一致性及证券池过滤；尚无证据证明是请求方法错误。
- 妙想：Workflow 将仓库 Secret `MX_APIKEY` 映射为 `MARKET_DATA_MX_API_KEY`；探测代码读取该变量并传入 Provider。Provider 使用 POST `/finskillshub/api/claw/query`、`apikey` Header、JSON `toolQuery`，与公开调用示例一致。上次回包提示 Key 不存在或失效，但目前没有请求侧 Header 证据，不能直接要求用户更换 Key。

### 31.2 已提交的诊断改进

- 提交 `46f58f9dde658e20cd22fe263d7db18e9ac90d8e`：Live Probe HTTP 记录新增 request_method、request_content_type、request_body_bytes、api_key_header_present；只记录 Key 是否存在，不记录 Key 值。新增测试验证请求元信息可见且 Secret 不会进入报告。
- 提交 `bed3a03f1cc0cd540161c6e10eeeda503c3dad24`：Tushare Live Probe 将明确的接口权限错误标记为 `permission_required`，不再与网络故障、Token 错误或 Provider 全局不可用混为 `failed`；其他错误仍按失败处理。新增权限错误分类单测。
- 上述提交通过 GitHub Contents API 写入当前功能分支；本轮没有触发线上 Provider 探测，也没有执行完整 Go suite。代码测试结果仍待可运行的 CI/本地测试环境确认。

### 31.3 下一步

1. 先运行本地/CI 单测，确认两个诊断改动通过编译、gofmt 和测试。
2. 下一次妙想探测只在用户确认可继续消耗两次查询额度后执行；重点核对报告中的 POST、JSON Content-Type、请求体长度和 apikey_header_present，再结合响应状态判断是否发生重定向或上游认证拒绝。
3. Tushare 按账号实际权限建立基线：先单独核验普通日线接口，再把有明确权限要求的能力标记为 permission_required；不得将未授权能力计入 Provider 可用能力。
4. TickFlow 分别隔离指数代码请求、报价单位校验和 Universe 非普通 A 股成员过滤；不降低 OHLC/金额/覆盖率质量门禁。
5. 本轮未确认任何一个凭据型 Provider 的全部能力通过；代码协议正确不等于线上凭据、接口权限和数据质量已验收。

## 32. 2026-11-04 TickFlow 证券池非普通 A 股成员处理

代码基线：`81e5257ccdbfc59cd0f2ac2312b0a833c5dd13c8`。

- `CN_Equity_A` 上游证券池可能包含项目当前普通 A 股行情适配器不支持的证券代码。此前市场广度和 SW1 行业排名直接对每个成员调用 `tickflowDomesticSymbol`，单个可识别但不支持的成员会导致整项能力失败。
- 新增证券池成员分类：规范化失败或交易所标识非法仍判为数据错误；合法 SH/SZ/BJ 代码但不属于当前普通 A 股支持范围的成员单独计入排除数量。
- 市场广度新增 `excluded_non_a_share_count`，保留上游 `total_securities` 分母；`covered_securities` 只统计实际纳入分析的普通 A 股。排除成员会触发 degraded 原因 `provider_universe_contains_non_common_equities_excluded`，不会被静默算作覆盖。
- 当存在排除成员时，不输出全市场成交额；涨跌停统计仍要求完整可计算，不放宽原质量门禁。
- SW1 行业成分和行情回包复用同一排除名单；只有同时属于上游 A 股证券池排除名单的非普通 A 股成员才会被跳过，未知/池外证券仍然报错。行业排名快照同步输出排除数量和 degraded 原因。
- 新增市场广度与 SW1 行业排名夹具用例，均覆盖 `302132.SZ` 这类上游池内、当前普通 A 股适配器不支持的成员；验证排除计数、原始覆盖分母和 degraded 原因。Run `41807290300` 已在提交 `84b3f1d0` 上完整通过 Go tests、Race Detector、Vet、MariaDB 集成和 Docker 构建；该 Run 早于行业排名夹具提交 `aea927a2`，因此不能作为该新增用例的验证证据。
- 本轮未重新请求 TickFlow，不消耗线上额度。指数行情 HTTP 400 尚未定位；仍需通过逐指数隔离的定向诊断确认具体代码/参数，不直接改为多次线上请求。

## 33. TickFlow HTTP 错误响应诊断信息保留

代码基线：`f419dd2193803d4ffa6e9c50dcd282c4682c2469`。

- 之前 TickFlow GET/POST 请求遇到非 2xx 状态只返回 HTTP 状态码，丢弃上游错误响应体，导致指数行情 HTTP 400 无法区分代码格式、参数约束、权限或接口限制。
- 现在保留最多 2 KiB 的错误响应摘要；超长内容明确标记截断。若上游响应体意外回显当前 API Key，会在错误文本中替换为 `[REDACTED]`。
- 新增本地 HTTP fixture，覆盖 GET/POST 错误体可诊断且 API Key 不泄露。
- 未重新调用 TickFlow。下一次定向探测可据上游错误正文定位指数请求问题；本次代码和单测尚未由 Go suite/CI 验证。

## 34. TickFlow 指数行情请求协议与官方 SDK 交叉核对

核对基线：`efa423fcc5fcb7d440f1c12163681b6ce2983968`。

- 官方 Python SDK 的 `Quotes.get` 实现：1–20 个 symbols 使用 GET `/v1/quotes`，Query 参数为 `symbols`，多个代码以英文逗号拼接；超过 20 个才使用 POST JSON。当前项目 6 个指数使用 GET + 逗号拼接，与官方 SDK 实现一致。
- 官方 SDK 认证使用 `x-api-key` Header；当前项目同样使用该 Header。指数代码格式 `代码.市场后缀` 也与 SDK 文档示例一致。
- 因此目前没有证据支持“GET/POST 方法、symbols 参数拼接或指数代码后缀错误”这一判断。不能仅凭 HTTP 400 擅自切换 POST 或修改代码格式。
- 官方参考：`https://github.com/tickflow-org/tickflow/blob/main/tickflow/resources/quotes.py`、`https://github.com/tickflow-org/tickflow/blob/main/tickflow/_base_client.py`。
- 尚未取得本项目线上 400 的实际错误响应正文；现有代码已保留最多 2 KiB 的脱敏错误体。下一步应使用单次定向请求取得错误正文，再按响应信息决定是否逐指数隔离；未取得错误正文前不批量增加请求次数。

本节是官方 SDK 源码协议核对，不代表 TickFlow 指数行情线上验收通过。

## 35. Tushare 免费/低积分账号接口权限分层核对

核对基线：`f8a89e962b2c0f47580ff5f76b49fe41973f4104`。

根据 Tushare 官方积分权限文档（`https://tushare.pro/document/1?doc_id=290`、`https://tushare.pro/document/1?doc_id=108` 及各接口说明）：

- 普通 A 股未复权日线 `daily`：最低 120 积分。
- 股票基础信息 `stock_basic`：2000 积分起。
- 交易日历 `trade_cal`：2000 积分。
- 每日指标 `daily_basic`：至少 2000 积分。
- 指数日线 `index_daily`：2000 积分。
- 复权行情 `pro_bar`：2000 积分起；分钟、实时日线及部分特色能力另有独立授权规则。

因此低积分账号对 `stock_basic`、`trade_cal` 等接口返回 40203 属于预期权限拒绝，不代表 Token 无效。普通日线 `daily` 是最低权限基线，若仍返回 40203，应单独核对账号是否达到 120 积分、请求接口名和账号权限状态；在取得成功响应前，不标记为可用。

当前 Live Probe 已将明确的 40203/权限不足响应归类为 `permission_required`，但测试入口仍会依次探测多项能力（含基础资料、日历、日线、估值、指数等），不适合作为低配额账号的“单接口最小探测”。后续应提供可选的 daily-only 探测模式，先以单次日线请求建立最低权限基线，再按用户明确授权逐项扩展；不应为了减少失败数量而跳过权限分类或降低数据校验。

本节为官方权限规则核对，不代表用户账号的当前积分或 `daily` 线上请求已经验证。

## 36. 真实 Provider 探测按能力缩小范围

- Live Probe 之前在启用任一凭据型 Provider 时仍会同时执行通用公共 Provider 探测和新闻检索，容易产生无关请求与额外配额消耗。
- 工作流改为显式标记：`[include_public]` 才执行通用公共 Provider 探测；`[include_news]` 才执行新闻检索；TickFlow 可用 `[tickflow_index_only]` 仅调用指数行情；Tushare 可用 `[tushare_daily_only]` 仅调用普通 A 股日线。
- Tushare daily-only 模式跳过基础资料、交易日历、估值和指数等高权限接口，只发起一次日线能力请求；TickFlow index-only 模式只发起一次指数行情请求。常规测试仍保留完整能力探测。
- 这些模式只缩小请求范围，不改变 Provider 请求参数、数据校验和质量门禁。提交需同时带对应 Provider include 标记，例如 `[include_tickflow] [tickflow_index_only]`。


## 37. 2026-11-04 TickFlow 指数行情单次标的数量限制修复

- 定向线上探测 Run 41809379496 的实际 Job 126504567940 失败；上游明确返回 TOO_MANY_SYMBOLS，请求 6 个指数，单次最多 5 个。该问题不是 Key、网络或权限故障。
- FetchIndexQuotes 已调整为每批最多 5 个标的，6 个核心指数分两次 GET 获取，再合并结果进入原有白名单、重复项、缺失项、价格、涨跌幅、时间戳和金额质量校验；不降低质量门禁。
- 单测覆盖每次请求标的数上限、6 个指数拆为两批及合并后的完整快照；异常字段测试夹具按每次请求的 symbols 返回对应行，避免模拟出重复数据。
- 本节记录代码修复，线上复测仍待执行，不代表 TickFlow 指数行情已验收通过。


## 38. TickFlow 指数分批修复后的线上复测安排

- 代码基线：`9dc5bfa8980b392d912903782e2af4b93c90cce0`；Go 全量 CI Run `41811940330` 的全部 5 个 Job 已通过，包含 market-data gofmt、tests、race detector、vet、MariaDB migration/PIT/Probe HTTP 集成及 Docker 构建。
- 本次仅触发 TickFlow `index_only` 真实探测，验证 6 个核心指数按每批最多 5 个标的拆分请求、合并后覆盖完整且通过原有质量校验。
- 探测仍由 `continue-on-error` 隔离，不以 Workflow 顶层 success 代替实际 Job/步骤和 `LIVE_TICKFLOW_RESULT` 报告。
- 该记录仅表示启动复测，不代表线上验收已通过。


## 39. Tushare 普通日线最小权限基线复测安排

- 当前已具备 `MARKET_DATA_LIVE_TUSHARE_DAILY_ONLY=1` 模式：只请求 `600519.SH` 的未复权日线，跳过 stock_basic、交易日历、估值和指数接口。
- 本次独立触发 `[include_tushare] [tushare_daily_only]`，仅核验普通 `daily` 的账号权限、返回数据和日 K 契约；40203 将报告为 `permission_required`，不会误判为 Token 无效或 Provider 故障。
- 本节为探测安排，不代表账号日线权限已通过。


## 40. Hithink 日 K 分页重复诊断

- 单次日线探测 Run 41812896392 首次发现重复交易日 2026-10-08；补充页码诊断后的 Run 41813388423 确认重复出现在第 2 页、offset=20、页内 row=0，说明这是跨页重复，不是同一页内重复。未通过日 K 完整性校验；不能将其视为免费账号权限拒绝，也不能把该结果计为可用日线。
- 分页重复错误现在同时附带首次出现和重复出现的页码、offset、页内行号，便于判断是否为相邻页边界重叠；重复仍然拒绝整次结果，不自动去重、不放宽质量门禁。
- 新增本地 HTTP fixture 模拟相邻页重复，验证请求 offset 递增和错误诊断字段；该测试不调用真实 Provider。
- 下一次单次日线探测仅用于取得重复发生的页码/offset 证据，再据此核对服务端分页语义；不扩大到复权、指数、财务或特色接口。


## 41. 2026-11-04 现有 Provider 校验结果汇总

### 41.1 TickFlow 指数行情修复线上通过

- 线上 Live Probe Run：`41812823875`；实际 Job：`126516047955`。
- 日志报告：`LIVE_TICKFLOW_RESULT [{"capability":"index_quotes","status":"passed","quality":"valid","rows":6}]`。
- 6 个核心指数完整返回，质量为 valid；说明每批最多 5 个标的的拆分请求已解决此前 TOO_MANY_SYMBOLS 问题。
- 本次使用 index-only 模式，没有探测 quote、market breadth、行业排名或日 K；不得扩大结论范围。

### 41.2 Hithink 未复权日线仍失败

- Live Probe Run：`41813388423`；实际 Job：`126517966406`，Job 失败。
- 唯一请求能力为 `daily_bars_none`；返回错误：重复交易日 `2026-10-08`，第 2 页、offset=20、页内 row=0。
- 该请求发生在首次位置诊断字段合入前；报告只包含重复位置，尚不能从本次日志确定同一交易日首次出现在哪一页/offset。
- 当前代码已扩展错误信息，记录首次与重复出现的 page/offset/row；本地 HTTP fixture 覆盖相邻页重复，仍拒绝整次结果，不去重。
- 最新 Go CI Run `41813766294` 已通过，包含 market-data gofmt、tests、race detector 和 vet。诊断字段扩展已完成代码验证，但尚未使用该版本再次调用真实 Hithink。
- 下一步先核对分页接口语义；只有在能明确解释请求边界且确有必要时，才进行一次 daily-only 复测。

### 41.3 Tushare 普通日线权限结果

- Live Probe Run：`41813259182`，只调用一次 `600519.SH` 的未复权日线。
- 上游返回 `40203`，探针归类为 `permission_required`。
- 当前账号普通 `daily` 权限未通过；不能据此认定 Token 无效，也不能将 Tushare 日线列为可用。
- 不再请求 stock_basic、trade_cal、daily_basic、index_daily 或其它接口，避免无意义配额消耗。

### 41.4 最新代码 CI 全部通过

Run `41813766294` 对应 HEAD `a3f64cf740b30bdba4cbb0fda89067cd9cd93212`：
- `Test .`、`Test go-core`、`Test services/market-data`：gofmt、tests、race detector、go vet 全部 success。
- MariaDB migration/PIT persistence 与 Probe HTTP end-to-end Job success。
- Docker Compose validation 和 Go 镜像构建 success。
- 此结果只代表代码与模拟集成验证通过，不代表真实 Provider 全部可用。

### 41.5 现有公共源历史日 K 基线

经核对相关 Provider 实现文件在当前 HEAD 与此前探测基线之间未变化，历史探测结果仍可作为当前实现的参考：
- Sina：none 8 条，成交额缺失，degraded。
- Tencent：none/qfq/hfq 各 8 条，成交额缺失，degraded。
- Eastmoney：qfq 因复权参考日未验证而 degraded；hfq 样本 valid。
- BaoStock：TCP 连接被拒绝。
- Yahoo：qfq/hfq 各 8 条，成交额缺失，degraded。
- 这些只是有限日期窗口的历史样本，不能推断完整历史覆盖、停牌边界或所有复权语义均通过。

### 41.6 下一步

1. 核对 Hithink historical daily endpoint 的 offset/排序/分页语义，不先改成自动去重。
2. 独立核对现有公共源的 none 日 K、成交额字段和单位；真实请求按 Provider 和接口最小化。
3. TickFlow 仅指数行情已通过；先盘点其余能力的接口与低成本探测方式，不触发全量市场广度/行业排名请求。
4. 继续阶段 B 交易边界和黄金事件样本工作；2025 事件窗口没有可追溯源数据前保持 blocked。
5. 妙想暂不重复请求；先保证请求认证元信息可验证，再决定是否做单次真实诊断。


## 42. 公共 Provider 未复权日 K 最小探测模式

- 新增 `MARKET_DATA_LIVE_PUBLIC_DAILY_ONLY=1`：仅对 Sina、Tencent、Eastmoney、Yahoo 的 `600036.SH` 发起一次 `AdjustmentNone` 日线请求。
- 模式复用统一日 K 契约检查和脱敏 HTTP 报告；只输出请求的四个 Provider 结果。
- 跳过 quote、qfq/hfq、跨市场样本、股票搜索、公告、行业、市场广度、新闻、妙想等其他请求。
- 通过提交标记 `[include_public] [public_daily_only]` 触发；不改变常规 Provider 探测行为。
- 当前仅为探测入口实现，Go CI 和真实 Provider 结果待验收。


## 43. 2026-11-04 公共源未复权日 K 最小探测真实结果

### 43.1 执行证据

- Live Workflow Run：`41815545410`；实际 Job：`126525230010`；Job 最终 success。
- 代码基线：`5d21d0cd4228d8c0a88fec323b3d5c608c6259a5`。
- 日志明确记录 `MARKET_DATA_LIVE_PROVIDER_TESTS=1`、`MARKET_DATA_LIVE_PUBLIC_DAILY_ONLY=1`；本次不再是此前 marker 缺失导致的 skipped。
- 唯一探测证券为 `600036.SH`，复权口径 `none`；Sina、Tencent、Eastmoney、Yahoo 各发起一次日线请求，四个上游 HTTP 均返回 200。
- 每家返回 8 条日 K。日志结果样本展示最近三条交易日：2026-10-30、2026-11-02、2026-11-03；本节不将三条展示样本误写为完整 8 条逐日核对。

### 43.2 Provider 结果

| Provider | 返回条数 | 结果状态 | 样本质量 | 成交额 | 成交量单位 | 结论 |
|---|---:|---|---|---|---|---|
| Sina | 8 | returned | degraded | 缺失 | shares | 可返回日 K，但缺少成交额，不满足完整日 K 质量要求 |
| Tencent | 8 | returned | degraded | 缺失 | shares | 可返回日 K，但缺少已验证成交额字段，不满足完整日 K 质量要求 |
| Eastmoney | 8 | returned | valid | CNY | shares | 本次样本满足当前统一日 K 质量契约 |
| Yahoo | 8 | returned | degraded | 缺失 | shares | 可返回日 K，但缺少成交额，不满足完整日 K 质量要求 |

### 43.3 字段对照与质量观察

- 最近三条样本的 OHLC 在四家之间一致（浮点表示存在微小精度差异）；成交量单位均规范化为 shares。Sina/Yahoo 原始成交量为 shares；Tencent/Eastmoney 原始成交量为 hands，规范化后乘以 100，与其他源样本基本一致。
- Sina 的降级原因包括上游日 K 未提供 amount、未回显 symbol，以及响应契约警告；Tencent 的降级原因是日 K 未提供经过验证的 amount 字段；Yahoo 的降级原因是 amount 缺失且单位未知。
- Eastmoney 样本 amount 字段来自 K 线字段 6，原始单位 CNY，规范化单位 CNY；最近三条样本成交额分别为 4,441,649,958、5,564,635,011、7,095,698,519 CNY。
- Sina、Tencent、Yahoo 的成交额缺失不得用 close × volume 推算填补，也不得将其 quality 提升为 valid。Provider 返回成功与数据质量通过是两个独立结论。
- 本次只对单只证券、有限窗口、未复权日 K 做最小探测；未验证全历史覆盖、停牌/非交易日边界、复权因子和其他证券适用性。

### 43.4 结论与后续

- 本次四家公共源真实请求均成功返回，但只有 Eastmoney 在本次样本上达到 `valid`；Sina、Tencent、Yahoo 均保持 `degraded`，不计为完整可信日 K 数据源。
- 不因本次结果修改统一质量门禁，不增加自动成交额推算，不新增 Provider。
- 下一步优先只读核对 Sina/Tencent/Yahoo 是否存在可稳定、可追溯且同口径的成交额字段；若 Provider 的日 K 接口本身不提供成交额，应明确标注能力限制，不以额外高配额请求反复试错。
- 后续再进行同证券同交易日的字段对账、交易日边界与分页完整性核验；比较时仅使用质量有效且单位/复权口径一致的数据。



## 44. 公共源日 K 成交额缺失的代码审查结论

代码基线：`d83e2135d8cae3fded8ebdf0377520bcf153862e`。

- Sina（`sina.go`）：历史日 K 使用 `CN_MarketData.getKLineData`。解析结构只包含 day/open/high/low/close/volume；代码注释明确该端点没有可验证的成交额字段和复权参数。当前将 Amount 保持 nil 并降级，未发现解析遗漏。
- Tencent（`tencent.go`）：未复权日 K 使用 `appstock/app/kline/kline`，解析已观察到的六个字段 date/open/close/high/low/volume。代码明确拒绝推断额外的 row[6] 字段，因为没有公开、可验证的字段契约。Tencent 实时 quote 的另一个响应结构可从 field 35/37 读取成交额，但它是当前快照，不是历史日 K 各交易日的 amount，不能用于回填历史日 K。
- Yahoo（`yahoo.go`）：历史日 K 使用 Chart API 的 `indicators.quote`，当前响应结构只有 OHLCV 数组和 adjclose；没有 amount/turnover 数组。代码将 Amount 保持 nil 并降级，未发现解析遗漏。实时 quote 的 meta.regularMarketVolume 同样不提供历史逐日成交额。
- 因此三家当前缺失成交额属于所用历史接口字段能力限制，不是已发现的解析 bug。不能把 Tencent 实时 quote 成交额或任何当前行情快照映射到历史日期，也不能用 OHLCV 推算成交额。
- 本轮不修改 Provider 解析逻辑，不增加请求，不新增数据源；维持 `amount_missing` → `degraded` 的既有质量门禁。
- 后续若要提升这三家历史日 K 的成交额完整性，必须先找到官方/稳定的历史成交额字段或独立历史接口，并用可追溯样本验证日期、单位和复权口径后再设计接入；在此之前，Eastmoney 是本轮四家中唯一通过未复权日 K 质量检查的 Provider。



## 45. 2026-11-04 Hithink 日 K 跨页重复首次位置复测

- Live Workflow Run：`41816257453`；实际 Job：`126527637932`；Job 内 `Query market-data providers and capture sanitized responses` 步骤失败，Workflow 顶层 success 不能覆盖该失败结论。
- 代码基线：`54663b4e7c12fff9b2257a885019977bc0b3adde`；环境变量确认 `MARKET_DATA_LIVE_HITHINK_TESTS=1`、`MARKET_DATA_LIVE_HITHINK_DAILY_ONLY=1`。
- 唯一能力为 `daily_bars_none`。真实错误：重复交易日 `2026-10-08`，首次位置 `first_page=1 first_offset=0 first_row=0`，重复位置 `duplicate_page=2 duplicate_offset=20 duplicate_row=0`。
- 客户端每页请求均显式设置 `offset`，并在收到非空响应后按 `len(data.Item)` 递增；本次从 offset 0 到 offset 20，第二页首行仍与第一页首行日期相同。现有证据可确认跨页重叠，但尚不足以断定服务端忽略 offset，也不能排除上游分页排序/快照变化。
- 现有本地 HTTP fixture 已验证客户端 offset 随响应行数递增、跨页重复拒绝整次结果并输出首次/重复位置；fixture 不能证明真实上游对 offset 的实现语义。
- 不自动去重、不放宽重复日期门禁、不把 Hithink 日线标记可用。后续代码已补充页级 item count、每页首尾日期；见第 46 节。当前不再重复真实请求，待后续必要验证时再观察新诊断输出。



## 46. Hithink 历史日 K 分页契约与诊断增强

### 46.1 官方公开契约核对

- HiThink-Tech/Financial-API 的历史行情端点文档列出 `offset` 参数，默认值为 0，并定义单标的、毫秒时间戳区间和 `adjust` 参数。
- 该文档未定义历史日 K 的 `limit/page_size` 参数、固定页大小、排序字段/方向、分页终止条件或跨请求稳定快照语义。不能把其他端点的 `limit/offset` 或 `page/size` 规则套用到历史日 K。
- 参考：[HiThink-Tech 历史行情端点文档](https://github.com/HiThink-Tech/Financial-API/blob/main/skills/hithink-finance/references/api/endpoints-prices.md)。

### 46.2 代码诊断增强

- 提交 `775d139` 新增重复日期所在两页的 item count、首日期和末日期诊断；辅助解析兼容数值/字符串 `date_ms`。
- 本地 fixture 验证第一页/第二页边界元数据、offset 按实际返回条数递增，以及重复日期仍然拒绝整次结果。
- 提交 `775d139` 的首次 CI 仅因 gofmt 失败；提交 `c557a83` 修正格式后，Go CI Run `41816944944` 全部通过：根模块、go-core、market-data gofmt/tests/race/vet、MariaDB migration/PIT 集成、Compose 校验及两个 Go Docker 镜像构建。
- 此次增强尚未再次真实请求 Hithink，因此页级诊断字段的线上输出待下次必要复测确认；不为验证日志而额外消耗 API 配额。

### 46.3 当前判断

- 已确认真实响应跨页重复：第一页 offset 0 的 row 0 与第二页 offset 20 的 row 0 同为 `2026-10-08`。
- 客户端确实在请求中设置 offset，并按上一页返回条数递增；官方文档未提供足以判断服务端稳定分页行为的契约。
- 当前不能断言服务端忽略 offset，也不能确认是否因排序变化或服务端分页窗口重叠。保持失败，不自动去重，不标记 Hithink 日 K 可用。



## 47. 2026-11-04 TickFlow 单股实时报价成交量单位修复与线上复测

### 47.1 线上证据

- Live Workflow Run：`41819666046`；实际 Job：`126539361088`；仅执行 `quote` 能力，`MARKET_DATA_LIVE_TICKFLOW_QUOTE_ONLY=1`。
- 实际只发起一次 `GET https://api.tickflow.org/v1/quotes`，请求标的 `600519.SH`；HTTP 200，日志确认 `x-api-key` 认证头存在，未记录 Key 值。
- Live 结果：`status=passed`、`quality=valid`、`rows=1`。
- 必要响应字段：last_price=1446.44、prev_close=1480、open=1470、high=1480.6、low=1439、volume=65434、amount=9536448000 CNY。

### 47.2 单位判断与代码修复

- 若把原始 volume=65,434 当作 shares，则 amount/volume 约为 145,741 CNY/股，明显超出 OHLC 区间。
- 按 A 股常用手单位换算为 6,543,400 shares 后，amount/volume 约为 1,457.41 CNY/股，落在 [1439, 1480.6] 区间，满足统一报价交叉校验。
- `FetchQuote` 已改用 `normalize.HandsToShares`；RawVolume 保留 65,434、RawVolumeUnit=`hands`，Volume 归一化为 6,543,400、VolumeUnit=`shares`。只修改 `/v1/quotes` 快照路径；日 K `/v1/klines` 暂保留独立 shares 口径，等待 daily-only 真实样本。
- 新增回归 fixture 覆盖 quote hands→shares 与日 K shares 保持原样。该实测仅为单只 A 股样本；官方 SDK Quote 类型列出 amount/volume 字段但未声明单位，参考 [TickFlow SDK Quote model](https://github.com/tickflow-org/tickflow/blob/main/tickflow/generated_model.py)。因此本节结论限于当前 A 股快照实现和样本交叉校验，不把它扩展为所有市场/所有 endpoint 的单位契约。

### 47.3 CI 状态

- Go CI Run `41819900049`（HEAD `8f6dd66e`）全部通过：根模块、go-core、market-data gofmt/tests/race/vet、MariaDB migration/PIT 集成、Compose 校验及两个 Go Docker 镜像。
- 单股 quote-only 通过不代表全市场广度或行业排名通过；两项仍保持未验收，避免触发大规模行情请求。


## 48. 2026-11-04 TickFlow DailyBar 标准化诊断上下文修复

### 48.1 当前问题与代码核验

- 代码基线：`48a09a2b51d8d76baa72933db1cf3fdeb674551a`。
- `normalizeProviderDailyBarsWithReport`、`normalize.ValidateAndSortBarsWithReport` 和 `validation.Reporter` 均已存在；不需要重新建设。
- TickFlow `FetchDailyBars` 原来调用兼容入口 `normalizeProviderDailyBars`，该入口使用 `context.Background()`，因此标准化报告无法关联到调用方 Reporter。Live daily-only 也没有输出 Validation 报告，失败只留下泛化错误。

### 48.2 本批次修复范围

- TickFlow 日 K 只改为调用 `normalizeProviderDailyBarsWithReport(ctx, ...)`，使既有标准化诊断进入请求上下文；不改单位换算、OHLC 门禁、复权参数或返回字段。
- daily-only 探针附带 `validation` 报告，并对响应提供有界摘要：各数组条数及最多 4 行原始 timestamp/OHLC/volume/amount/prev_close；HTTP BodySample 在写入结果前清空，避免输出完整响应。
- 不记录 URL Query、API Key 或认证头值；HTTP 记录只保留端点、状态码和认证头是否存在。
- 新增本地回归测试验证 amount/volume 隐含均价不一致导致整批过滤时，调用方 Reporter 能收到 `amount,volume/unit_mismatch` 诊断；新增 raw summary 解析测试。

### 48.3 自动化验证与真实数据状态

- 实现提交：`bed1abe510527aead4c8c125f8fa894117f168c2`；格式修正提交：`a6da06c3118edeac1979ff5258550198b26c2db8`、`0e2eb661c2920c15a5066a208d388cae5f891d68`。前两次 CI 仅因 gofmt 失败，修正后 Go test suite Run `41823789910` 全部 5 个 Job 通过：根模块、go-core、market-data gofmt/tests/race/vet、MariaDB migration/PIT 集成、Compose 校验和两个 Go Docker 镜像。
- Go-Python parity Run `41823797220`、WebUI checks Run `41823797336` 通过。通用 Live Probe Run `41823797262` 通过，但未启用 TickFlow daily-only marker，不构成 TickFlow 日 K 真实验收。
- 本节记录实现范围；TickFlow daily-only 真实复测仍待执行。
- 真实 TickFlow 日 K 的成交量单位尚未重新验收；本次不把 Quote 的 hands→shares 结论推广到 `/v1/klines`。下一次仅执行一次 `600519.SH`、8 条、none 口径 daily-only 请求。
- 在取得真实响应并完成 OHLC/amount/volume 交叉核验前，日 K 单位维持“待证实”；质量无法比较时标记 `not_comparable`，不擅自调整单位。


## 49. 2026-11-04 TickFlow A 股日 K 成交量单位真实根因

### 49.1 真实响应证据

- Live Workflow Run：`41824457415`；Job：`126555518622`；仅执行 `daily_bars_none`；证券 `600519.SH`；HTTP 200；返回 8 条。
- 请求级 Validation 报告：`total_count=8`、`error_count=8`；8 条全部命中 `field=amount,volume`、`code=unit_mismatch`，日期覆盖 2026-10-23 至 2026-11-03。
- 有界原始摘要：第一条 OHLC 1310.00～1343.88、close 1335.00、volume 57686、amount 7677216283 CNY；末条 OHLC 1435.10～1480.00、close 1480.00、volume 74121、amount 10825780463 CNY。所有 8 条的 amount/volume 隐含均价约为 OHLC 的 100 倍。
- 例如首条按原始 volume 计算约 133,100 元/股；volume 乘以 100 后约 1,331 元/股，落在 OHLC 区间。末条也同样成立。

### 49.2 根因与修复边界

- 结论限于 TickFlow A 股 `/v1/klines` 当前真实样本：volume 按手返回；原代码误用 `RawSharesToInt64`，导致 8 条均因 amount/volume 交叉校验被拒绝。
- 修复复用既有 `normalize.HandsToShares`；RawVolume 原值保留，RawVolumeUnit 改为 `hands`，规范 Volume 乘以 100 且 VolumeUnit 为 `shares`。
- 不修改 `/v1/quotes`（其手转股已另有真实证据）；不改 amount CNY、OHLC、复权口径、标准化门禁；不把结论扩展到非 A 股市场。
- 本节为真实根因定位记录。代码修复、自动化验证和修复后真实复测必须分别记录，未完成前不得标记 TickFlow DailyBar 真实验收通过。


## 50. 2026-11-04 TickFlow 日 K 手转股修复后真实复测

### 50.1 修复提交与自动化验证

- 单位修复：`211a58da787561227b62cbc2432534ee728cd0f0`，将 A 股 `/v1/klines` 的 raw volume 由按股误解改为按手解释，复用 `normalize.HandsToShares`；RawVolume 保留原值，RawVolumeUnit=`hands`，VolumeUnit=`shares`。
- 回归测试覆盖普通日 K、none/forward/backward 复权元数据、标准化失败 Reporter。修正历史 fixture 后，Go test suite Run `41825777135` 全部 5 个 Job 通过；Parity Run `41825788560`、WebUI checks Run `41825788574` 通过。
 
### 50.2 修复后真实证据

- Live Run：`41825777178`；Job：`126560000555`；能力：`daily_bars_none`；证券：`600519.SH`；请求 8 条；HTTP 200；API Key Header 存在。
- 结果：`status=passed`、`quality=degraded`、`rows=8`；Validation `total_count=0`、`error_count=0`；timestamp/OHLC/volume/amount 数组长度均为 8；body_truncated=false。
- 原始响应未返回 `prev_close`（数组长度 0）。因此第一条涨跌幅无可用基准，后续记录使用排序后的前一交易日收盘价回算；所有受影响记录按既有规则降级。不得将本次结果标为全量 valid。
- 8 条日 K 的 amount/volume 在 volume×100 后均通过 OHLC 宽区间交叉校验。样本首条 57,686 手→5,768,600 股，隐含均价约 1,330.86 元；末条 74,121 手→7,412,100 股，隐含均价约 1,460.56 元。
 
### 50.3 当前能力边界

- TickFlow A 股 `/v1/klines` 手转股修复：当前 600519.SH、8 条样本通过；不推广到其他市场。
- DailyBar 质量：因缺少 prev_close 保持 degraded；不伪造 prev_close，不从其他 Provider 回填。
- Market breadth 和 sector rankings：本轮未执行，仍未验收。
- 下一 P0 转为现有公共源 none 日 K 的单位/口径复核；只复用历史证据或在有必要时单次定向请求。


## 51. 2026-11-04 公共 Provider 未复权日 K 复核收口

### 51.1 证据基线

- 分支代码基线：f17c9645744d79b5e16cb5469c46d62194bbb3de。
- 复用 Live Probe Run：41815545410；Job：126525230010。
- 标的：600036.SH；复权：none；Sina、Tencent、Eastmoney、Yahoo 各一次请求；每家 8 条；四家 HTTP 200。
- 本轮为历史证据与当前代码只读复核，没有再次请求上游，也没有消耗 Provider 配额。

### 51.2 样本结论

| Provider | 原始成交量口径 | 标准成交量 | 成交额 | 样本质量 |
|---|---|---|---|---|
| Sina | shares | shares | 缺失 | degraded |
| Tencent | hands | shares（×100） | 缺失 | degraded |
| Eastmoney | hands | shares（×100） | CNY | valid |
| Yahoo | shares | shares | 缺失 | degraded |

- 四家最近三个共同交易日 OHLC 一致（仅存在浮点表示精度差异），标准成交量基本一致。
- Sina/Tencent/Yahoo 的历史接口当前没有可验证的逐日成交额字段；不得用实时 Quote 成交额回填历史日期，也不得从 OHLCV 推算成交额。
- Eastmoney 当前样本的成交额来自 K 线字段 6，单位 CNY；当前样本通过统一日 K 字段质量检查。
- BaoStock 代码路径解析成交量为 shares，成交额字段存在时按 CNY 处理；历史真实连接拒绝记录仍有效，本批次未重新请求，因此真实可用性仍未通过。
- returned 与 valid 分开判定：HTTP 200/返回行数只证明请求返回，不代表字段质量通过。

### 51.3 代码审查与变更边界

- 已复核当前 HEAD 的 sina.go、tencent.go、eastmoney.go、yahoo.go、baostock.go。
- 未发现 Sina/Tencent/Yahoo 历史成交额字段存在可直接修复的解析遗漏；Eastmoney amount 解析与单位声明已有实现。
- 本批次无代码缺陷需要修复，无新增字段/API/Provider；不调整 amount 缺失降级规则。
- 通过：本次 Eastmoney 有限样本满足当前字段质量规则。降级：其他三家有日 K 返回但缺少可信 amount。失败：BaoStock 历史连接拒绝。不可比较：不同复权基准或字段单位/质量不满足严格比较前置条件。
- 本节不证明全历史完整性、交易边界正确性、复权准确性或阶段 B 完成。

### 51.4 下一步

转入现有 DailyBar 交易上下文能力核对：检查实际 Fetch/Probe 调用链是否接入交易日历与证券交易状态诊断，并通过本地测试验证停牌、临停、上市前、未知状态、交易日缺失日 K 等边界。真实事件窗口数据缺失继续保持 blocked；不重复运行近期行情 Probe 来替代事件样本。


## 52. 2026-11-04 DailyBar 交易上下文降级日历修复

### 52.1 问题与根因

- 既有 assessDailyBarTradingContext 仅识别 provider_trading_calendar_sources_disagree，未统一处理日历 QualityStatus=degraded。
- BaoStock 完整日期日历明确带有 provider_snapshot_timestamp_not_exposed，QualityStatus=degraded；在证券状态字段完整时，旧逻辑仍可能把交易上下文结果保留为 valid，丢失日历证据本身的降级信息。

### 52.2 修复

- 提交 92ae3d849822aa20405bbeca904f549594d8adf1：日历质量非 valid 时，追加 daily_bar_context_calendar_degraded 并将上下文至少降级；继续检查证券状态，保留 suspended/intraday/pre-listing 等可解释原因。
- 多来源日历冲突仍按原逻辑返回 daily_bar_context_calendar_sources_disagree，不据冲突日历确认缺失日 K。
- 新增回归测试覆盖“日历 degraded + 证券状态已知停牌”：整体上下文 degraded，同时保留 daily_bar_excluded_suspended 证据。
- 修正原有有效日历测试夹具，显式设置 QualityValid；避免零值 QualityStatus 被当成有效证据。

### 52.3 自动化验证状态

- 修复提交：92ae3d849822aa20405bbeca904f549594d8adf1；格式修正：3eac25678f3c5198530dfc15cd83af1a6ff5a1f9；测试夹具修正：d346f209fc46c56992302025b8608aa3b2e91b6b。
- 对应 SHA 的 gofmt 与 market-data 单元测试已通过；race detector、vet、Docker 构建当时仍在执行中，不能提前记为全套 CI 通过。
- MariaDB migration/PIT 与 Probe HTTP/MariaDB E2E、Data quality contract regression、WebUI checks 已通过。
- 自动触发的公共 Provider Probe Job 已结束 success；本次没有将其作为新 Provider 数据验收证据，也没有因该结果改变 Provider 状态。

### 52.4 范围与未完成项

- 不改变普通 DailyBars 的请求行为；交易上下文仍用于 Provider Probe 诊断链路。普通日 K 获取维持现有字段校验和交易日历覆盖率判断，避免历史大范围查询逐日追加证券状态请求。
- 不新增 API、字段、Provider 或 WebUI 契约；接口协作清单无需更新 API 进度。
- 本修复只保证降级日历不会产出 valid 上下文，不构成真实停牌、新股、除权事件窗口验收；阶段 B 继续开放。


### 2026-11-04 CI 结果补记

- 代码提交 d346f209fc46c56992302025b8608aa3b2e91b6b 对应 GitHub Actions Run 41830773041：根模块测试、go-core 测试、market-data gofmt/tests/race/vet、MariaDB migration/PIT 与 Probe HTTP E2E、Compose 配置及两个 Go Docker 镜像构建均通过；Data quality contract regression Run 41830782245 通过；WebUI checks Run 41830782914 通过。
- 以上 CI 对应代码提交 d346f209，后续仅文档提交不改变代码。自动触发的公共 Provider Probe Run 41830782109 成功，但不将其视为新的真实行情验收证据。
- 因此本次交易上下文代码修复的自动化验证通过；真实交易边界样本仍未齐备，阶段 B 不关闭。


## 53. 2026-11-04 日 K Probe 交易上下文端到端诊断复用

### 53.1 复用范围

- 新增测试从 `ProbeProviderCapabilitiesWithOptions` 入口触发日 K Probe，复用生产调用链中的 `ValidateAndSortBars`、`TradingCalendar`、`SecurityTradingStatus`、`assessDailyBarTradingContext`。
- 未创建第二套状态判定逻辑；不增加 API、字段、Provider、表结构，也不改变普通 `DailyBars` 的逐日状态请求策略。

### 53.2 覆盖场景与判定

| 场景 | 预期结果 |
|---|---|
| 缺失日 K 对应证券状态为 Suspended | 解释为停牌排除，不确认 Provider 漏数 |
| 缺失日 K 对应证券状态为 NoEvent | 标记 confirmed trading day missing，整体降级 |
| 证券状态查询失败 | 标记 status unavailable，保持未知，不确认漏数 |
| 日历 degraded 且证券状态为 Suspended | 整体降级，同时保留停牌解释 |
| 多来源日历日期状态冲突 | 标记 calendar sources disagree，不确认漏数 |

### 53.3 提交与验证

- 测试提交：`a1b1ac889e07fc84dce0c1cb15c4014ee2b24129`；格式修正：`145003b99d47898a4601723c4004b61e52a66f27`、`efd1409237547b387a2cbd6b490aa1c2b8455687`。
- 首轮 CI 检出新增测试文件 gofmt 不符合要求，已按 CI 的 gofmt diff 修正。最终格式修正提交的 Go test suite 当前仍在排队/执行，尚无完整通过结论。
- 之前代码提交自动触发的 Live Provider Probe 为 success；不把自动探针成功等同于新增场景的真实数据验收，也不改变 Provider 能力状态。

### 53.4 边界与后续

- 本节是自动化模拟链路覆盖，不是真实停牌/临停/上市前行情验收。
- 不修改 WebUI API 协作清单，因为没有 API 或 Schema 变化。
- 下一步等待最终 Go suite；通过后再以已有历史证据或必要的单次真实样本核验停牌/临停与上市边界。不得重复消耗 Provider 配额来替代缺失的事件证据。

## 54. 2026-11-04 P0 批次 3 最终 CI 结果校正

### 54.1 代码基线

- 当前分支文档 HEAD：`b85507f2b8cd3d03a21448b08c682f227feb59f3`。
- 对应本批次代码 SHA：`efd1409237547b387a2cbd6b490aa1c2b8455687`。
- `b85507f2` 为文档提交，未改变 P0 批次 3 的代码。

### 54.2 最终自动化验证结果

| 检查 | Run | 结果 |
|---|---:|---|
| Go test suite（Push） | 41833418185 | passed，全部 Job 成功 |
| Go test suite（PR） | 41833428826 | passed，全部 Job 成功 |
| Data quality contract regression | 41833428871 | passed |
| WebUI checks | 41833428940 | passed |
| Live Provider Probe | 41833428913 | Workflow/Job success |

Go suite 覆盖根模块、go-core、market-data 的格式/测试/race/vet、MariaDB migration/PIT、Compose 校验及 Docker 镜像构建。上述 CI 证明自动化测试和构建链路通过，不代表真实历史交易边界或复权数据验收通过。

### 54.3 真实数据边界

- Live Provider Probe Run 41833428913 成功，不等于取得 600036.SH（2025-07-07 至 2025-07-17）和 002982.SZ（2025-05-26 至 2025-06-06）的完整目标样本。
- 本批次没有新的目标事件窗口原始数据、公司行为或因子证据；两个窗口继续 blocked。
- 停牌、临停、新股上市边界仍只有本地 Fake/自动化链路覆盖，真实样本未验收。
- 不将 Workflow success、HTTP 200 或模拟测试提升为真实数据 passed。

### 54.4 后续

- 不重复执行本批次 Go suite、Parity、WebUI checks 或近期公共 Provider Probe。
- 下一步先只读核对现有 Provider 的历史日期范围、复权支持和公司行为能力，确认可采集后再安排最小真实请求。
- 若开展采集，复用现有 Provider、Normalize、Validator、Reporter、TradingCalendar、SecurityTradingStatus 和 CrossProviderDailyCompare；不另建诊断体系。
- 无 API/Schema 变化，WebUI 与数据层接口协作清单无需更新。

## 55. 2026-11-04 Probe 历史日 K 日期窗口能力实现

### 55.1 现有能力核对

- Provider 的 DailyBar 接口已统一接受 startDate/endDate/limit/adjustment；BaoStock、Eastmoney、Hithink、Pytdx、Tencent、TickFlow、Tushare、Yahoo 具备显式历史日期请求实现；Sina、Netease 仅接受 none，且历史返回受源端 K 线条数上限约束。Xueqiu 当前不是 DailyBar Provider。
- 代码支持日期范围不等于目标历史数据可用；Provider 权限、源端历史保留、成交字段质量和复权算法仍需独立真实验收。Pytdx 复权依赖 GBBQ，公司行为事件缺失/连接失败必须保留原始错误分类。
- 旧 Probe 固定调用空日期、limit=5，不能通过既有诊断入口针对历史事件窗口执行完整日期校验和交易上下文核验。
- 业务 DailyBars API 已有日期参数，但它是聚合业务读取，不能替代 Provider × 复权口径独立 Probe 和历史诊断结果。

### 55.2 本批次变更

- 扩展既有 `POST /api/v1/data-sources/probe`，无新增路由。
- 请求新增 `daily_bar_start_date`、`daily_bar_end_date`，必须成对、仅 daily_bars 使用、Asia/Shanghai 闭区间、最多 31 个自然日、结束日期不得晚于今日。
- 响应新增 `requested_daily_bar_start_date`、`requested_daily_bar_end_date`，仅回显请求日期，不代表源端发布时间或 PIT available_at。
- 不传日期时保持原有近期 5 条 Probe；传日期时使用指定区间和 2000 条安全 limit，并将同一日期边界传给 Provider、Validator 和交易上下文诊断。
- 测试新增 HTTP 参数校验、历史日期回显及 Service 向 Provider 传递日期范围/limit 的回归覆盖。

### 55.3 真实数据状态

- 本批次只扩展诊断入口，不触发真实 Provider 请求。
- 两个 2025 事件窗口仍 blocked；不得将历史窗口参数测试或 CI 成功标记为真实行情、公司行为或复权验收通过。
- 下一步先按 Provider 核对目标窗口是否支持、凭据是否配置、已知权限状态是否变化，再执行最小必要真实请求。

### 55.4 跨端契约

- 更新 `services/market-data/openapi.yaml`、`doc/api/数据层API清单.md` 和 `doc/接口规范/WebUI与数据层接口协作清单.md`。
- WebUI 当前不使用历史 Probe 参数，不要求改动页面；API-06 仍待真实数据验收。

### 55.5 自动化验收

- 最终代码 SHA：`331930f4274b8b7b30adc1a1dc7951cbead4790b`。
- Go test suite Push Run `41836528814` 全部 5 个 Job 通过，包括 market-data gofmt/tests/race/vet、MariaDB migration/PIT、Compose 和 Docker 镜像构建。
- Go-Python data quality parity Run `41836540034`、WebUI checks Run `41836540427` 通过；Live Provider Probe Run `41836540261` Workflow success。
- Live Provider Probe 为常规近期探测，没有使用历史日期窗口，不构成 2025 事件窗口真实数据验收。
- 初轮 CI 仅发现 gofmt 格式问题，已根据 CI diff 修正；最终 SHA 自动化验证通过。真实历史数据尚未请求，阶段 B 不关闭。


## 56. 2026-11-05 目标历史窗口不复权日 K 真实 Provider 验收

### 56.1 探测入口与 Sina 回溯修复

- GitHub Actions 已支持通过提交标记触发指定历史窗口探测，并可一次指定多只股票；本次窗口为 2025-07-07 至 2025-07-17，样本为 600036.SH、002982.SZ。
- 首轮探测发现 Sina K 线端点只接受 `datalen`，不接受结束日期。原实现按目标窗口长度估算条数，导致上游返回 2026 年近期数据，历史窗口归一化后为空。
- 修正 `sinaKlineDataLength`：历史请求按目标起始日到当前最新日期估算所需返回条数，仍受 1023 条上限保护；之后由既有归一化流程按请求结束日期过滤。增加历史窗口请求条数回归测试。
- 历史窗口模式将完整返回行写入脱敏报告；常规近期探测仍只保留最新三条。未新增生产 API 或 Provider。

### 56.2 真实请求结果

- GitHub Actions Live Provider Probe Push Run：`41950643180`，Workflow conclusion success，内部探测步骤 success。
- 请求范围：`2025-07-07..2025-07-17`；股票：`600036.SH`、`002982.SZ`；复权：`none`。
- Sina、Tencent、Eastmoney、Yahoo 对两只股票均返回 9 条日 K，日期覆盖 2025-07-07 至 2025-07-11、2025-07-14 至 2025-07-17，未发现重复日期或窗口外日期。
- 两只股票的 9 个交易日 OHLC 在四家 Provider 间按人民币分精度一致。
- 成交量表现为两组：Sina/Yahoo 返回精确股数；Tencent/Eastmoney 返回以百股为粒度的数量。差异为小幅数量精度差，不应在没有来源单位证据时强制判为相同。
- Eastmoney 9 条均为 `valid`，成交额字段可用；Sina、Tencent、Yahoo 因源端没有可验证成交额字段，9 条均为 `degraded`，成交额保持 null，不进行推算。
- 本次 002982 的 Eastmoney 请求曾出现一次 EOF；后续合并窗口探测成功返回 9 条。前次 EOF 作为瞬时连接关闭记录，不覆盖最终成功结果。

### 56.3 验收边界

- 本次证明四家 Provider 在这两个目标窗口能取回完整 9 条未复权日 K，并且 OHLC 有跨源一致性证据；不等于已获得交易所/官方基准数据，也不构成绝对数据准确性证明。
- 本次使用 Provider 真实请求测试入口及其行契约检查，不等同于生产 `POST /api/v1/data-sources/probe` 完整执行；尚未通过生产 Probe 链路完成 CrossProviderDailyCompare、TradingCalendar、SecurityTradingStatus 和 MariaDB 历史持久化的真实窗口联验。
- 未测试 qfq/hfq、复权参考日期、复权因子及公司行为。阶段 B 继续 in_progress，不关闭。
- 下一步：以这两个窗口继续执行 qfq/hfq 最小真实请求，保留每条 K 线的 `AdjustmentMethod`、`AdjustmentRefDate`；仅在比较器确认同一绝对基准后才允许 matched。随后再通过生产 Probe 历史日期选项补齐交易上下文与持久化联验。
- 最新代码 SHA：`23365dd16b39d5be3077d8870d178c3c50a457a9`。该 SHA 的 Go test suite Push Run `41950643179` 在记录时仍在执行；Live Provider Probe 已完成。CI 未完成前不提前标记代码验收全部通过。


## 57. 2026-11-05 qfq/hfq 与生产 Probe 历史窗口联验

### 57.1 qfq/hfq 真实请求

- GitHub Actions Live Provider Probe Run：`41951184379`。该轮直接 Provider 报告用于查看每条样本的调整方法和参考日期；Eastmoney 在该轮遇到连接 EOF。
- 随后的生产 Probe 历史联验 Run：`41952778969`，两只样本均对 Sina、Tencent、Eastmoney、Yahoo 执行 `none`、`forward_adjusted`、`backward_adjusted`，每项均完成 9 条返回/验证或明确 not_supported。
- Sina 的 forward/backward 均为 `not_supported`，错误语义明确为只支持 none，没有把不支持误报为请求失败。
- Tencent forward/backward 分别返回 `tencent_qfqday`、`tencent_hfqday`；当前模型未得到可验证的 `AdjustmentRefDate`。
- Eastmoney forward/backward 分别返回 `eastmoney_fqt_1`、`eastmoney_fqt_2`；当前模型未得到可验证的 `AdjustmentRefDate`。forward 另有 `daily_bar_adjustment_reference_date_unverified` 与响应契约 warning。
- Yahoo forward 使用 `yahoo_adjclose_ratio_rebased_to_end`，参考日期为请求窗口末日 `2025-07-17`；backward 使用 `yahoo_adjclose_ratio_rebased_to_start`，参考日期为窗口首日 `2025-07-07`。这是适配器按请求窗口重基准的日期，不应误解为 Yahoo 原始数据的永久复权基准。

### 57.2 生产 Probe 链路结果

- 生产服务层测试直接调用 `DataService.ProbeProviderCapabilitiesWithOptions`，使用 `daily_bars`、`connectivity/contract/quality/cross_provider` 检查，传入两只股票、历史日期和三种复权口径。
- 两只股票各自 12 个 Provider/口径项：`total=12`、`failed=0`、`not_supported=2`、`degraded=10`、`returned=0`、`status=degraded`；10 个可支持项均返回并验证 9 条，2 个 Sina adjusted 项明确 not_supported。
- 本次测试未注入交易日历 Provider，故各可支持项带有 `daily_bar_context_calendar_unavailable`；同时 Sina/Tencent/Yahoo 成交额缺失也使其字段质量降级。该测试没有 MariaDB Store，`persisted=false` 是测试配置预期，不代表生产数据库写入失败。
- 所有 cross_provider 对账项被现有严格门禁标记为 `skipped / comparison_not_comparable`，原因是 `quality_status_must_be_valid`。这不是数据 mismatch，也不能计为 matched。
- 调整元数据仍有独立缺口：Tencent/Eastmoney 缺少可验证参考日期；Yahoo 参考日期是窗口重基准日期，三家方法标识不同。即使以后质量状态满足门禁，也不能仅凭 forward/backward enum 相同直接比较。

### 57.3 结论与后续

- none 的 Provider 直连样本已证明两只股票 9 日 OHLC 按分跨源一致；qfq/hfq 已证明三家 Provider 能返回数据，但复权基准仍不可比。
- 当前不修改 CrossProviderDailyCompare 的质量门禁，不允许降级数据进入 matched；也不为 Tencent/Eastmoney 猜测参考日期。
- 下一步优先核对 Tencent/Eastmoney 源端复权定义和可追溯参考日期证据；如果源端不暴露，应明确标记缺失并保持 not_comparable。再评估是否有可信公司行为/复权因子源可作为独立基准。
- 本轮生产 Probe 测试只注入四家日 K Provider，未注入生产日历 Provider，也未连接 MariaDB；交易上下文和历史持久化仍需单独联验。
- 当前代码 HEAD：`e3a0bbb2ba973abe3317f26fe4bd4f0e250a7d4c`。Live Provider Probe Run `41952778969` 成功；Go test suite Run `41952778933` 在记录时 Go 格式检查及普通测试已通过，race/vet 和 Docker 构建仍在执行。


## 58. 2026-11-05 生产历史 Probe 测试装配修正

### 58.1 上轮问题复核

- 上轮 GitHub Actions 服务层历史 Probe Run `41951967119` 执行成功，但测试构造的 DataService 没有注入生产使用的交易日历/证券状态 Provider，也没有 MariaDB Store。
- 结果为两只股票各 12 项诊断：10 项 returned_degraded、2 项 Sina adjusted not_supported、0 failed；交易日历上下文不可用，CrossProviderDailyCompare 因 `quality_status_must_be_valid` 全部 skipped；`persisted=false` 是测试未配置 Store 的结果。
- 这不是 Provider 硬失败，也不是跨源 mismatch，更不能作为交易上下文或持久化验收通过。

### 58.2 测试入口修正

- 历史 Probe opt-in 测试现在按生产装配方式注入 BaoStock TradingCalendarProvider。
- 当 GitHub Secret `TUSHARE_TOKEN` 存在时，注入 Tushare SecurityTradingStatusProvider；不把 Tushare 加入本轮日 K Provider 列表，避免扩大请求范围。
- Live Provider Probe job 增加临时 MariaDB 11.4 service；测试启动后执行既有 `Store.Migrate`，Probe 执行 storage check，随后通过 `ProviderProbeHistory` 读回并核对 RunID。
- 不修改生产 Probe 业务逻辑、不放宽质量门禁、不增加生产 API。
- 最近一次 Go test suite 中 market-data 的 gofmt 与普通测试已通过；race/vet 和 MariaDB 集成仍以完整 Run 结论为准。

### 58.3 下一轮真实验收

- 通过提交标记 `[historical_service_probe]` 重新触发相同历史窗口，检查 BaoStock 日历是否真实可用、Tushare 状态证据是否返回，以及 MariaDB 是否成功持久化并读回。
- 只有实际结果支持时才更新交易上下文、cross-provider、storage 结论；若 BaoStock/Tushare 无法提供证据，按 unavailable/degraded 记录，不将缺失上下文误标通过。


## 59. 2026-11-05 BaoStock 日历与 MariaDB 生产 Probe 复验结果

### 59.1 真实服务层执行

- GitHub Actions Live Provider Probe Push Run：`41954775830`；服务层历史 Probe 步骤执行失败，但报告已生成并上传。
- 两个目标股票各产生一条 Probe Run，均为 `persisted=true`；storage check 为 `passed`，测试随后通过历史查询读回对应 RunID。MariaDB 临时服务、迁移、写入和读回链路已获得真实执行证据。
- 每只股票共 12 项日 K 探测：7 项 returned_degraded、3 项 Eastmoney network_error/EOF、2 项 Sina adjusted not_supported；无 Provider mismatch 结论。
- BaoStock 交易日历已被注入并参与日 K 上下文校验，但其快照质量为 degraded（源端未暴露可验证快照时间），所以所有返回 K 线附加 `daily_bar_context_calendar_degraded`。这是上下文质量降级，不是日历完全不可用。
- 本次没有单独请求 `security_trading_status` 能力；日 K 窗口本身完整，无缺失交易日需要证券状态回补，不能据此宣称 Tushare 历史证券状态已验收。
- Eastmoney 三种复权口径在服务层本次均遇到 EOF；此前直连批次曾成功返回，按瞬时网络错误处理，不推翻此前样本，但本次不能计为成功。
- CrossProviderDailyCompare 全部按现有门禁 skipped，错误为 `quality_status_must_be_valid`；未产生 matched，也未放宽门禁。

### 59.2 Pytdx 历史日 K / GBBQ 探测

- GitHub Actions Run：`41955146155`，Pytdx 专项步骤成功，目标窗口 2025-07-07 至 2025-07-17。
- 600036.SH、002982.SZ 均返回 9 条 none/qfq/hfq 日 K，日期覆盖完整。
- none 两只股票均为 9/9 valid；OHLC 与前述四家 Provider 样本按分一致。Pytdx 成交量/成交额单位和数量精度仍需保留各自证据，不能与其他源简单要求逐位相同。
- qfq 两只股票均返回 9 条，方法标记 `pytdx_qfq`，但 9/9 quality degraded；未提供可验证复权参考日期，不能作为 matched 证据。
- hfq 两只股票均返回 9 条，方法标记 `pytdx_hfq`，9/9 valid；但方法标记本身不等于绝对参考基准已验证。当前报告没有可证明与其他 Provider 同一参考基准的 ref date / factor evidence。
- 这批 Pytdx 样本证明 GBBQ 路径可以产出历史 qfq/hfq 数值，不证明公司行为完整、复权基准一致或复权值绝对准确。

### 59.3 结论

- MariaDB Probe 持久化与历史读回：passed。
- BaoStock 交易日历：returned_degraded，已实际参与上下文校验，但快照时间证据缺失。
- Eastmoney：本次服务层调用 transient EOF；之前 direct probe 成功记录保留。
- Pytdx none：样本级 valid；qfq degraded；hfq valid 但绝对参考基准未证实。
- CrossProviderDailyCompare：not_comparable，保持原门禁。
- 阶段 B 继续 in_progress。下一步执行 Tushare 历史复权因子/公司行为专项探测，并将 Pytdx 与独立公司行为证据对齐；不凭 qfq/hfq 名称做跨源比较。
