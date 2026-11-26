# 08 · market breadth 指标决策与正名（执行清单 1.6）

- 状态：书面结论完成（2026-11-26），**待 M1 决议**；本文件不新增代码、Schema、Registry Entry 或 Stage。
- 范围：决策"旧 Runner 的 market breadth 指标是否保留、若保留如何正名与实现"。
- 关联：`旧Analysis退役执行清单.md` 1.6、`WebUI旧Analysis到Researchflow迁移盘点.md` 9.x/10.x、`doc/quant/01-量化引擎总体设计.md` 第 37 行（Market 需明确全集/分母/覆盖率）、`doc/quant/模块总任务规划和进度.md` QNT-005。

## 1. 代码级事实：旧指标不是市场广度

`internal/service/wiring.go` 的 `marketAnalyzer.Analyze`（约 210–255 行）做的是：

1. 遍历 `MarketSnapshot.Bars`，跳过不足 20 根 bar 的证券；
2. 分母 `total` = **满足样本数的证券数**，分子 `above` = 最新收盘价高于自身 20 日均线的证券数；
3. 输出 `watchlist_above_ma20_pct`（percent，`float64`）与 `watchlist_symbols_with_20_bars`；
4. 以 65 / 35 为阈值把结果映射成 `Regime` 字符串：`watchlist_trend_positive` / `watchlist_trend_weak` / `watchlist_mixed`，无样本时为 `insufficient_cross_section`；
5. 算法版本 `mvp-watchlist-market-v1`。

而 `MarketSnapshot.Bars` 的来源是 `universeResolver.Resolve`（`wiring.go:35-39`）直接复制 `req.Symbols`——即**调用方传入的自选股列表**。代码自身也在缺指数日 K 时写入 `"市场环境只使用自选股横截面，不能视为完整市场广度分析"` 的 data issue。

因此旧指标的真实语义是**请求证券集合内的参与度（cross-section participation）**：
- 分母随请求变化，同一只股票在不同 Run 里的"广度"不可比；
- 覆盖的是调用方恰好请求的证券，与"全市场 A 股"没有稳定映射；
- 65/35 阈值没有出处，属猜测性常数；
- 不含停牌/ST/退市/新股/涨跌停等任何市场级分类。

## 2. 消费者：只有旧链，没有新链

| 消费者 | 事实 |
|---|---|
| `internal/analysis/app/runner.go:247` | 只校验 `MarketState.Regime` 非空，不解释取值 |
| `web/src/report.js:63,79,87` | 旧报告页以"市场状态"一行展示 `regime` |
| `web/src/research-reports.js:99` | 读取 `market_state.as_of` 作为报告时点回落 |
| Researchflow / Quant 新链 | **无消费者**：`quant` 无 MarketStage/MarketState DTO；2.1 完整研究 Template 不含 Market Stage；QNT-005 仍 NOT_STARTED |

旧链（`internal/analysis`、`internal/service`、旧 WebUI 报告页）随阶段 4 删除，因此该指标与 `regime` 字符串届时一并消失。

## 3. 与 Data 已有能力的边界：真·市场广度已经存在，但不是 Quant 可消费的快照

Data 已具备市场级广度事实（`internal/marketdata/model/model.go` 的 `MarketBreadthSnapshot`）：

- 口径固定为 `market=cn_a_share`，字段含 `total_securities`、`covered_securities`、`advancing/declining/unchanged/unclassified_count`、`limit_up/limit_down_count`、`total_amount_cny`、`amount_coverage_count`、行情时间覆盖与最旧/最新报价时间；
- 门禁是**数量级对账**而不是"看起来合理"：`covered ≤ total`；`requireFullCoverage` 时要求 `covered == total`；四类家数必须精确加总到 `covered`；涨跌停家数不得超出覆盖范围；部分覆盖时 `quality_status` 必须是 `degraded`（`internal/marketdata/service/data.go:491+`）；
- 新鲜度证据用最旧成分报价时间（`freshness.MarketBreadthEvidence`，`oldest_component_quote_time`），避免用最新一笔掩盖过期成分；
- 对外已有 `GET /api/v1/market/breadth`（`internal/marketdata/httpapi/server.go:189`），WebUI 市场总览消费的正是它。

**关键缺口**：Data 的授权快照（Quant 唯一合规入口）目前只有一种 payload：`daily_bars_close_series`（单证券）。`MarketBreadthSnapshot` 只有 HTTP 展示路径，没有"授权快照 → Quant 授权读取"的契约。也就是说，Quant 现在**拿不到可追溯的市场广度事实**；要拿，必须先在 Data 侧新增授权快照 payload 与授权语义（DATA-CTR 变更）。

## 4. 差距矩阵

| 维度 | 旧 `watchlist_above_ma20_pct` | Data `MarketBreadthSnapshot` | `doc/quant/01` 对 Market 的要求 |
|---|---|---|---|
| 统计母体 | 请求的 symbols | `cn_a_share` 全市场 | 全市场全集 |
| 分母 | 满足 20 根 bar 的请求证券数 | `covered_securities`（须对账到 `total_securities`） | 须明示分母 |
| 单位/精度 | percent、float64 | 整数家数、`*float64` 成交额 | 须绑定精度策略 |
| 排除规则 | 无（只按样本数过滤） | 涨跌平/未分类四类精确对账 | 停牌/ST/退市/新股规则 |
| 质量证据 | 无（仅 data issue 文案） | `quality_status` + 覆盖率 + 最旧报价时间 | 须有覆盖率与质量状态 |
| 阈值 | 65 / 35（无出处） | 无阈值（纯事实） | 不得引入无出处常数 |
| PIT | `AsOf` = 最后一根 bar 的 trade_date | `fetched_at`/报价时间 + 报告日期 | 须 `system_as_of` |
| 可消费性 | 旧 Runner 内部 | HTTP 展示 | 需授权快照 |
| 新链消费者 | 无 | 无（Quant 侧） | QNT-005 未开始 |

## 5. 选项与判定

### 选项 A：保留，正名为横截面参与度并由 Quant 实现

正名建议：`cross_section_participation_ma20`（分母显式写入 Metric 与 Quality Summary，禁止命名 `breadth`/`market`）。实现有两条路径：

- **A1：N 次单证券授权快照**——沿用现有 `daily_bars_close_series`，由 Stage 对 N 只证券各建一次快照。问题：N 次授权/读取/内容寻址落库；N 份快照的 `system_as_of` 无法保证同一时点，横截面一致性与 PIT 语义失真；成本随请求线性放大。
- **A2：新增"证券横截面"授权快照 payload**——一次快照承载多证券 + 同一 `system_as_of` + 统一覆盖率。问题：属 DATA-CTR 契约变更，需要授权模型、Manifest、读取 Reader、校验与集成测试全套；收益仅在"自选股参与度"这一个指标上。

两条路径都**没有消费者**：QNT-005 未开始，2.1 Template 不含 Market Stage，旧报告页随阶段 4 删除。按本仓库既有口径（1.4/1.5 的判定逻辑：先有真实调用方，再建接口），现在建接口属无调用方建设。

### 选项 B：书面废弃（推荐）

结论：**不保留为 Quant 指标，不迁移 `mvp-watchlist-market-v1`**，理由：

1. 语义不可比：分母是调用方集合，跨 Run 不可比，无法进入报告或 Gate 的稳定口径。
2. 与真实市场广度重复且更弱：Data 已有带精确对账的 `cn_a_share` 广度，Quant 再用自选股重算一个"广度"只会制造两个同名不同义的指标。
3. 阈值 65/35 无出处，属于 DEC-PLAT-002-049 同理禁止的"猜测性规则"。
4. 实现代价与收益倒挂：A1 破坏 PIT 一致性，A2 是跨模块契约变更，而当前零消费者。
5. 废弃不损失能力：真实需求（盘后市场复盘的市场环境）由 QNT-005 承接，输入是 Data 已有的广度事实——缺的只是 Quant 可消费的授权快照契约，那属于 QNT-005 的前置项，应由真实需求驱动，而不是为一个旧指标顺带开工。

### 选项 C：推迟（不推荐单列）

等同 B + 显式登记"若未来需要自选股参与度"的复活条件（见第 7 节）。B 已包含该登记，因此不单列。

## 6. 命名规范（废弃后仍生效）

- `market breadth`/`市场广度` **只**指 Data 的 `MarketBreadthSnapshot`（`cn_a_share` 全集、四类家数对账、覆盖率与最旧报价时间证据）。
- 请求证券集合内的比例必须命名为 `cross_section_*` / `*_participation`，且分母（证券数与样本数）必须出现在 Metric/Quality Summary 中。
- 禁止把 `watchlist_above_ma20_pct`、"自选股广度"、"全市场广度" 三者混用；旧链路内的 `watchlist_trend_positive`/`watchlist_trend_weak`/`watchlist_mixed`/`insufficient_cross_section` 四个字符串随阶段 4 删除，不进入新契约。

## 7. 复活条件（若未来确认需要）

同时满足以下三条才重新进入设计，且必须走 A2（不允许 A1）：

1. 存在真实调用方（例如某个正式 Stage/报告明确需要"我的自选股参与度"这一指标，而不是笼统的"市场环境"）；
2. Data 侧先具备"证券横截面授权快照"契约（多证券 + 同一 `system_as_of` + 统一覆盖率 + 显式分母）；
3. 指标以 `cross_section_participation_*` 命名，分母与样本数进入 Metric 与 Quality Summary，并绑定独立 `algorithm_version` 与 Golden。

## 8. 对 QNT-005 的前置依赖（登记项）

QNT-005（Market 环境分析）当前 `NOT_STARTED`，等待"全市场 Universe、分母、覆盖率和交易状态输入契约"。基于本文件事实，前置项应精确化为：

1. **Data 授权快照**：为 `MarketBreadthSnapshot` 增加授权快照 payload（与 `daily_bars_close_series` 并列），使 Quant 能以"授权 + PIT + 覆盖率"方式消费，而不是读 HTTP 展示数据；
2. **交易日历与交易状态**：停牌/ST/退市/新股的排除规则需由 Data 事实支撑（现有 `security_trading_status` 能力），否则 QNT-005 无法给出可解释的分母；
3. **指数输入**：旧链的 `index_20d_return_pct` 同样缺授权快照路径，属同一类前置。

在上述契约未就位前，QNT-005 不应以"先用 HTTP 广度凑"的方式启动。

## 9. 本轮结论与不变项

- 本轮只交付书面结论与命名规范，**不新增**代码、Schema、Golden、Registry Entry、Stage 或 Data 契约。
- 不改任何契约状态：QNT-CTR-003 仍 `deferred`、004/005 仍 `frozen`；QNT-005 仍 `NOT_STARTED`（前置项已精确化）。
- 旧 `marketAnalyzer`、`MarketState.Regime`、旧报告页"市场状态"均保持原样，随阶段 4 删除旧链一并退役；不在 Quant 复制其 float64 实现。
- 1.6 的退出条件"有书面结论；不再以全市场广度命名"在本文件与第 6 节规范下成立；是否记为已完成取决于 M1 决议。
