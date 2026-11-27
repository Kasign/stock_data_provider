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

## 6. 后续状态

1. **采集入口（已落地）**：见第 7 节 CLI 与第 7.6 节收盘后调度 Worker；在 Run 之前按 `(symbol, trade_date, purpose)` 产出事实快照。
2. **交易状态/盘口的 as-of 与新鲜度（已落地）**：见第 9 节与 DEC-PLAT-002-077；交易状态按 trade_date 约束，盘口按盘中 15 分钟 / 收盘后当日收盘约束。
3. **L3「预采集 → Run 复用」断言（已落地）**：见第 8 节；预采集时点严格早于 Run，不再依赖种子时点偶然相等。

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

### 7.5 收盘后调度 Worker（已落地）

`internal/server/bootstrap/limit_pool_collection.go` 在 `worker`/`all` 角色的 Server 启动时注册收盘后预采集循环：目标为最近一个已收盘交易日，按 `SERVER_LIMIT_POOL_COLLECT_SYMBOLS` 逐证券调用同一个 `LimitPoolFactsSnapshotCollector.Persist`；每天每证券只采集一次，失败按 `MaxAttempts` 有界重试。默认关闭，配置样例见 `.env.example`。它同样只落库、不签发授权。

### 7.6 仍未覆盖

- 交易状态依赖原始 PIT 表在评估时点可读；盘口预采集入口见第 10 节，已补齐独立 CLI 与非授权落库能力。
- 真实部署的证券池需由运维配置 `SERVER_LIMIT_POOL_COLLECT_SYMBOLS`；未配置时不会猜测标的。
- 盘口预采集没有收盘后批量 Worker：盘口是日内事实，应由调用方贴近 Run 评估时点触发，并由 Quant 的 15 分钟/收盘边界拒绝陈旧事实。

## 8. L3「预采集 → Run 复用」验收（2026-11-27）

`TestQuantResearchMariaDBRedisL3` 不再把涨跌停种子写成与 `Run.SystemAsOf` 完全相同的时点：

1. `seedQuantResearchL3LimitPool` 走生产 `LimitPoolFactsSnapshotCollector.Persist`，用确定性全量池来源在 `Run.SystemAsOf - 2m` 落快照；
2. 测试用例断言该快照的系统时点严格早于 Run 评估时点，且 `quant.risk` 的 `input_refs[4]` 正是这个预采集 `SnapshotRef`；
3. 授权仍由 Risk Stage 在复用快照时签发，采集侧没有预先授权。

变异验证：把预采集时点改回 `Run.SystemAsOf`（旧的精确相等路径），L3 立即失败于 `precollected limit-pool as-of ... must precede Run system_as_of ...`；恢复后真库用例 PASS。

## 9. 交易状态与盘口 as-of/新鲜度（DEC-PLAT-002-077）

- **交易状态**：`GetLatestSecurityTradingStatusFactsSnapshotAtOrBefore` 按 `symbol + trade_date + purpose` 身份精确、时间取上界最新；Creator 先复用，未命中才读 append-only PIT 表。Quant 绑定由等值改为上界，`trade_date` 必须等于参数日。新鲜度由交易日身份保证，不引入任意小时阈值。
- **盘口**：`GetLatestOrderBookFactsSnapshotAtOrBefore` 按 `symbol + purpose` 身份精确、时间取上界最新；Creator 先复用，未命中才读实时 Provider。Quant 绑定要求：发布不晚于评估；盘中观测不超过 15 分钟；收盘后评估要求观测日等于 `trade_date` 且不早于当日 15:00 收盘。
- **结果层修复**：`EvaluateTradabilityDecision` 现在显式接收 `request.SystemAsOf` 作为决策 `system_as_of`，不再从交易状态快照读取。若沿用旧写法，A 方案下「更早快照 + 更晚评估」会得到 payload/manifest 时点不一致的失败；该失败在新增单测中真实暴露后修复。
- **冻结契约**：无 Schema/Golden/版本字节变化；DATA-CTR-007 1.0.0、QNT-CTR-003 3.0.0 均保持原状。
- **验收与变异**：仓储真库覆盖 `-1min` miss / `+30s` 命中 / 跨用途、跨日 miss；Quant 单测覆盖更早发布可用、晚于评估拒绝、隔日盘口拒绝、盘中过旧拒绝；L3 真库 PASS。关闭交易状态/盘口仓储上界分别使对应真库用例失败，关闭盘中 15 分钟检查使新鲜度用例失败，恢复后全绿。
- **边界**：盘口预采集入口已在第 10 节补齐；交易状态依赖原始 PIT 表在评估时点可读。若 Owner 否决 15 分钟或收盘边界，须以新 DEC 覆盖。

## 10. 盘口事实预采集入口（DEC-PLAT-002-077 补充）

- **拆分**：`OrderBookFactsSnapshotCollector.Persist` 只校验、复用或真实取值、构建并落库，**不签发授权**；`OrderBookFactsSnapshotCreator.Create` 复用同一实现后再 `IssueFor`。两层共用一套取值/构建/落库逻辑，无平行实现。
- **时点**：`-system-as-of` 未指定时取 Provider 的系统观测时刻。调用方若先取「现在」再采集，观测必然晚于该时点，PIT 会正确拒绝；手工指定仍要求 UTC，且观测不得晚于该时点。
- **复用**：按 `(symbol, purpose)` 身份精确、时间取 `<= system_as_of` 的最新快照；复用时只校验 PIT 上界，不重新调用 Provider。旧盘口能否使用仍由 Quant 的盘中 15 分钟/收盘后边界裁决。
- **入口**：`cmd/order-book-collect` + `make order-book-collect ARGS="-symbol 600519.SH"`；复用 `config.LoadForCollector()`，不需要 Redis 或 Owner 级凭据。
- **权限边界**：真实 Eastmoney + MariaDB 连续两次运行返回同一 `SnapshotRef`（`data_snapshot:sha256:10706cc0...`），该快照 `authorization_decisions=0`；授权仍由 Risk Stage 读取时签发。
- **变异**：关闭 PIT 上界检查后 `TestOrderBookFactsSnapshotCollectorRejectsObservationLaterThanAsOf` 有效失败；把默认观测时点改成 `time.Now()` 后 `...PersistsWithoutAuthorization` 有效失败；恢复后两用例通过。
