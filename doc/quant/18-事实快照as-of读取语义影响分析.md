# 事实快照 as-of 读取语义影响分析（price_limit 收尾）

更新时间：2026-11-27
范围：`marketdata` 事实快照复用/绑定语义；Quant Risk 输入的时点绑定。只涉及涨跌停池（`limit_pool`），交易状态与盘口保持原语义。

## 1. 问题

`quant.risk` 收到的 `SystemAsOf` 来自 Run：`worker.go:280 → snapshot.Run.SystemAsOf`，而 `create_run.go:231` 把它设为**创建 Run 的时刻**。事实 Creator（涨跌停与盘口一致）要求 `observed_at <= system_as_of`。

因此「创建 Run 之后在 Stage 内现场采集」这条顺序在契约上永不成立：采集必然晚于 Run 创建。L3 之所以通过，是因为种子快照的 `system_as_of` 与 Run 完全相同，命中**精确复用**分支，从未真正现场采集。

## 2. 三个候选方案的实际代价（代码核实）

| 方案 | 做法 | 实际改动面 |
|---|---|---|
| A. as-of 读取 | 事实按发布时刻标注，读取取「≤ 评估时点」的最新快照 | 仓储查询 + Creator 复用 + Quant 绑定（3 处），结果层不动 |
| B. 评估时点前移 | Stage 用采集时刻作为评估时点 | **至少 6 处**强制等值：`resultvalidation/quant.go:242`、`risk.go:48`、`composer.go:414/428/484/529`，全部要求结果 `system_as_of` 等于 Run 的期望时点；还要改 Stage Envelope 语义 |
| C. 快照不绑评估时点 | DTO 的 `system_as_of` 改为观测时点 | 契约 MAJOR，三类事实 + 读取链 + 结果关联全改 |

结论：A 的改动面最小，且保留「Run 时点 = 评估时点」这一现有语义（结果层所有等值校验仍然成立）。

## 3. A 的具体语义

### 3.1 读取

`GetLatestLimitPoolFactsSnapshotAtOrBefore(ctx, symbol, trade_date, purpose, asOf)`：

- 身份维度仍精确：`payload_kind`、`symbol`、`trade_date`、`purpose_code`；
- 时间维度改为「`system_as_of <= asOf` 中最大者」；
- `system_as_of` 存于 `manifest_json` 且是变长 RFC3339Nano 字符串，SQL 侧字符串比较不可靠（`…:05.5Z` vs `…:05.05Z`），因此按身份收窄候选后在 Go 侧解析取最大值；候选规模是单证券单日量级。

### 3.2 安全论证（为什么「更早」是安全的）

1. **同日内容不随时间变化**：涨跌停池按交易日收盘后固化，同一 `(symbol, trade_date, purpose)` 的事实内容等价，只有发布时间戳不同。
2. **不可能取到盘中不完整池**：Creator 要求 `system_as_of` 不早于该交易日 15:00（市场本地时间），任何被持久化的快照都已收盘。
3. **不会跨交易日**：`trade_date` 是精确匹配维度。
4. **上界仍是硬约束**：发布晚于评估时点的快照在 PIT 上还没发生，Creator 与 Quant 绑定两处都拒绝。

### 3.3 绑定

- Creator：复用时不要求 `system_as_of` 完全相等，但必须 `<= 请求时点`；身份字段仍逐项核对。
- Quant `validateRiskLimitPoolBinding`：由「等于评估时点」改为「不晚于评估时点」。
- `TradabilityEvidence.Validate`：移除涨跌停池与交易状态事实的跨类等值要求。跨类时点关系由绑定（持有评估时点）裁定，结构性校验只保留各数据类自身的自洽性。

## 4. 前提与边界

- **A 要求事实先于 Run 存在**。当前生产只有 Risk Stage 内部调用 Creator，因此在没有独立采集入口前，真实 Run 会得到「`price_limit` 缺失 → 可追溯 `reject`」，而不是 `pass`。这比让 Run 失败更有价值，但仍不是可用状态。
- **交易状态与盘口仍是等值语义**，它们同样受这一时点问题影响；本次不动，因为它们各自需要独立的新鲜度策略（盘中事实的「多旧算旧」不能照搬按交易日固化的规则）。
- 不改变 `Run.system_as_of` 的含义，不触动 `resultvalidation` / `composer` 与 Run 时点的等值校验。

## 5. 验收

- 仓储真库：同一身份下 `as_of = 发布时间` 命中；`+30s`（更晚）仍命中；`-1min`（更早）必须 miss；存在多条时取「≤ 评估时点」中最新的一条；不同 `trade_date` miss。
- Creator 单测：复用到「发布晚于请求时点」的快照必须拒绝（端口实现有缺陷时的防御）。
- Quant 单测：更早发布的池可用于更晚的评估；发布晚于评估时点必须 `ErrInvalidRiskSnapshot`。
- 真实端到端（`TestLiveLimitPoolFactsProductionChain`）：在真实池与 MariaDB 上追加「更晚评估时点复用同一 `SnapshotRef`」的断言，直接证明「先采集、后评估」的生产顺序成立。
- 变异：仓储去掉 as-of 上界过滤、Quant 取消上界判断、Creator 取消上界守卫，三处都必须失败。

## 6. 后续（未做）

1. **采集入口**：在 Run 之前按 `(symbol, trade_date, purpose)` 产出事实快照（调度任务或受限 API），否则 A 的收益无法体现在真实 Run 上。
2. **交易状态/盘口的 as-of 与新鲜度**：需要各自定义「可接受的最大陈旧度」，例如盘口只接受 N 分钟内的观测。
3. L3 增加「预采集 → Run 复用」的断言，替代当前「种子快照与 Run 时点完全相同」的偶然通过路径。

## 7. 采集入口（已落地，2026-11-27）

A 方案要求事实先于 Run 存在，因此补了一个最小采集入口：

```bash
# 收盘后预置当日涨跌停事实（只落库、不签发授权）
make limit-pool-collect ARGS="-symbol 600519.SH -trade-date 2026-11-27 -purpose risk_review"
# 等价于
go run ./cmd/limit-pool-collect -symbol 600519.SH -trade-date 2026-11-27
```

### 7.1 结构

- `internal/marketdata/application`：把原 Creator 拆成两层——
  - `LimitPoolFactsSnapshotCollector.Persist`：校验 → as-of 复用 → 拉两个全量池 → 构建载荷/快照 → 落库，**不签发授权**；
  - `LimitPoolFactsSnapshotCreator.Create`：复用 `Persist` 后追加 `IssueFor`，供 Risk Stage 使用。
  两层共用同一份取值/构建/落库实现，没有第二套逻辑。
- `internal/marketdata/config.LoadForCollector()`：采集工具只需要存储 DSN 与 Provider 超时，不要求 Redis，也不要求行情凭据；存储缺失仍硬失败，不静默降级。
- `cmd/limit-pool-collect`：参数解析 + 装配 + 读回校验 + 输出。

### 7.2 权限边界

采集器**不签发任何授权**，因此不需要 Owner 级凭据。授权仍由 Risk Stage 在读取时签发。本地实测（真实 Provider + MariaDB）：`market_data_authorization_decisions = 0`、`authorization_policy_versions = 0`，即确实没有产生授权决策。

### 7.3 时点语义（一个必须说清的细节）

`-system-as-of` **默认不取「现在」**。默认取「实际观测时刻」：调用方若先取现在再采集，观测必然晚于该时点，PIT 会正确拒绝（实测报 `ErrLimitPoolFactsSnapshotStaleAsOf`）。这与「事实可用的最早时点就是它被观测到的时刻」一致。

### 7.4 幂等

同一 `(symbol, trade_date, purpose)` 重复采集会复用已发布快照，不会重复造。本地连续运行两次得到同一 `SnapshotRef`。

### 7.5 仍未覆盖

- 采集调度（cron / 平台任务）尚未接线，需要运维侧按交易日收盘后触发。
- 交易状态与盘口仍为等值语义，且没有采集入口；它们的 as-of 与最大陈旧度需要各自定义。
