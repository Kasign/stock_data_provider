# Freshness SLA 与评估契约

更新日期：2026-11-03

## 1. 目标与边界

Freshness 描述数据相对于其更新预期是否及时，是独立于 `QualityStatus` 的维度。

- `valid/degraded/stale/invalid/unknown` 继续表达数据质量，不因 Freshness 结果而自动升级或降级。
- `fresh/stale/unknown/not_applicable` 表达新鲜度判断。
- `fetched_at` 只表示本服务收到响应的时间，不能替代 Provider 源端时间。
- `provider_time`、`quote_time`、`available_at`、`trade_date` 等必须明确语义后才能作为对应数据域的判断依据。
- 缺少源端时间、时间语义或必要市场上下文时返回 `unknown`，不能用采集时间补齐。

## 2. 核心实现

核心纯函数位于 `services/market-data/internal/freshness`：

- `Policy`：数据域 SLA、允许的市场阶段、是否要求交易日历、停牌时是否不适用。
- `Evidence`：源端数据时间及其语义；采集时间仅用于诊断展示。
- `Context`：调用方显式传入的评估时刻、市场阶段、交易日历和证券交易状态。
- `Evaluate`：无 I/O、不读取系统时钟、不访问 Provider/缓存/数据库，确保同一输入得到同一结果。

当前首批核心评估支持基于持续时间的 SLA。日 K、财务披露、公告和静态数据的业务日/披露周期策略需要在后续按各自时间语义单独扩展，不得直接套用实时时间窗口。

## 3. 状态与原因

| 状态 | 含义 |
|---|---|
| fresh | 源端时间在 SLA 窗口内 |
| stale | 在应更新的市场阶段内，源端时间超过 SLA |
| unknown | 缺少源端时间、语义、评估时间、必要日历/证券状态证据，或源端时间明显位于未来 |
| not_applicable | 非交易日、非更新阶段或已确认停牌，当前不应按实时 SLA 判定 |

核心原因码包括：`within_sla`、`exceeds_sla`、`source_time_missing`、`source_time_semantics_missing`、`trading_calendar_evidence_missing`、`non_trading_day`、`market_not_updating`、`security_suspended`、`security_trading_status_unknown`、`source_time_in_future`。

## 4. 首批建议策略

以下为策略初始建议，不代表所有 Provider 已满足该更新频率；接入时应结合源端实际刷新频率和真实环境测量校准。

| 数据域 | 初始 SLA | 适用阶段/语义 |
|---|---:|---|
| 个股实时行情 | 90 秒 | 交易所允许更新的竞价/连续交易阶段；停牌单独处理 |
| 核心指数 | 120 秒 | 指数对应的有效交易阶段 |
| 市场宽度、板块行情 | 5 分钟 | 盘中更新阶段 |
| 资金流 | 5 分钟 | 仅在源端明确更新时间时评估 |
| 日 K | 最近一个已完成交易日 | 按交易日与收盘/Provider 发布时点判断，不使用 90 秒窗口 |
| 估值 | 最近有效交易日或源端快照时间 | 保留源端时间语义 |
| 财务 | 披露周期与 `available_at` | 报告期末不能替代首次可用时间 |
| 公告 | 发布时间/`available_at` | 日期字段不能伪造成精确时刻 |
| 交易日历 | 覆盖范围、版本和完整性 | 12 小时缓存 TTL 不等于业务 Freshness SLA |
| 历史/静态资料 | 按版本或有效期 | 不适用实时 SLA |

## 5. A 股市场上下文

调用方应复用已有交易日历、交易时段和证券停复牌证据，再组装 `freshness.Context`。Evaluator 不直接调用这些服务。

- 午休、收盘后、非交易日等不应简单按盘中 90 秒 SLA 判 stale。
- 停牌不等于源端数据失效；确认停牌时实时行情可返回 `not_applicable`。
- 临停、复牌应以证券级证据为准；状态未知时不能假设正常交易。
- 交易日历缺失或覆盖不完整时，要求日历的策略返回 `unknown`。
- 集合竞价、连续交易、收盘竞价是否要求更新，由策略的 `ActivePhases` 明确指定。

## 6. 后续工作

1. 为 quote、index、market breadth、sector、fund flow 建立集中 SLA 策略注册表，并接入真实源端时间字段。
2. 增加日 K 的最近已完成交易日策略，覆盖收盘、非交易日、节假日和 Provider 延迟发布。
3. 为财务、公告、估值定义基于 `available_at`、报告期及发布节奏的策略。
4. 将 Freshness 证据和结果加入统一数据诊断逐 Provider 明细及 OpenAPI。
5. 通过固定时间输入、特殊交易日样例和 Provider 实际数据完成端到端验收。

## 7. 测试覆盖

核心单元测试覆盖 SLA 内外、未来源端时间、采集时间不可替代源端时间、缺失时间语义、交易日历缺失、非交易日、午休/非更新阶段、停牌和证券状态未知。


## 8. SLA 策略注册表与首批模型映射

集中策略入口位于 `internal/freshness.PolicyForDomain`，当前初始配置：

| 数据域 | SLA | 更新阶段 | 当前时间证据 |
|---|---:|---|---|
| quote | 90 秒 | 开盘竞价、连续交易、收盘竞价 | `Quote.QuoteTime` + `QuoteTimeSemantics` |
| index | 120 秒 | 开盘竞价、连续交易、收盘竞价 | `MarketIndexQuote.QuoteTime` + `QuoteTimeSemantics` |
| market_breadth | 5 分钟 | 上午/下午连续交易 | 使用覆盖成分中最旧的 `OldestQuoteTime`，避免最新时间掩盖陈旧成分 |
| sector | 5 分钟 | 上午/下午连续交易 | 当前只有 `ProviderReportDate`，不具备精确源端更新时间，判定保持 unknown |
| fund_flow | 5 分钟 | 上午/下午连续交易 | 当前模型只有业务日期，未暴露精确源端更新时间，判定保持 unknown |

策略返回值会复制阶段切片，避免调用方修改全局策略。首批证据映射函数：

- `QuoteEvidence`、`IndexQuoteEvidence`：只读取明确的 Provider 行情时间及时间语义。
- `MarketBreadthEvidence`：使用最旧成分行情时间；`FetchedAt` 只保留作诊断展示。
- `SectorEvidence`、`FundFlowEvidence`：只携带 `FetchedAt`，不把报告日期、交易日期或采集时间伪装成源端时间。

### 当前限制与下一步

1. Quote/Index 的 Provider 适配层必须稳定填充时间语义；缺失时 Evaluator 返回 unknown。
2. 市场宽度使用最旧成分时间作为保守聚合证据；还需在诊断明细中展示覆盖数量及最旧/最新时间。
3. 板块与资金流要从 Provider 原始响应补充可信的精确更新时间，或明确改为基于交易日/报告周期的独立策略；在此之前不能按 5 分钟 SLA 输出 fresh/stale。
4. 当前策略注册表尚未接入诊断 API，也没有完成 Go 测试、gofmt、Race、Vet 或构建验证。


## 9. 接入统一 Provider Probe 诊断

本轮已将 Freshness Evaluator 接入现有 `provider_probe` 的 freshness 检查维度，不增加第二套诊断接口。

- 每个 Provider 检查结果新增 `freshness_status`、`freshness_reason`、`freshness_domain`、`freshness_max_age_seconds`、`market_phase`；原有 `age_seconds` 只在评估证据足够时填写。
- Provider 行情源端时间语义通过 `provider_time_type` 传递，缺失时保持 unknown，不使用 `fetched_at` 补值。
- Quote 使用 `QuoteTime` 和 `QuoteTimeSemantics`；Index 使用首个有效指数快照的 `QuoteTime` 和时间语义；Market Breadth 改用 `OldestQuoteTime`，并标记为 `oldest_component_quote_time`。
- 每次诊断只解析一次市场日历/交易阶段；只有取得质量为 valid 的日历证据时，才向 Evaluator 提供已知交易上下文。
- Quote 额外解析证券级停复牌状态；缺少有效证券状态时保持 unknown，不假设证券正在正常交易。
- freshness 检查结果状态映射：fresh → passed，stale → degraded，unknown/not_applicable → skipped。该映射只表达新鲜度检查，不修改 Provider 的 `QualityStatus`。
- Sector/Fund Flow 仍缺少精确源端时间，检查结果保持 unknown；日 K 暂不套用实时 SLA。

### 本轮验证边界

已增加策略接入测试，覆盖有效交易阶段的市场宽度 freshness 判定，以及缺失交易日历时不得误报 fresh。尚未在真实 Go 执行环境运行测试、gofmt、Race、Vet 或构建；提交前需继续做静态 Review，并在可执行环境完成验证。
