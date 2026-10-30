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
