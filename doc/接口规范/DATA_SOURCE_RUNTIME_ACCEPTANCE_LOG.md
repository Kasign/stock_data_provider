# 数据源真实环境验收记录

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
