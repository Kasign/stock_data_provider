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
