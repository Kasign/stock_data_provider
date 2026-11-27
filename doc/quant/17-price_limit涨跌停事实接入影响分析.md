# 17 · `price_limit` 涨跌停事实接入影响分析（执行清单 1.4 收尾）

- 日期：2026-11-27
- 分支：`repair_all`
- 范围：解除 1.4 唯一剩余事实缺口 `price_limit`；不改任何已发布 Schema 字节，不推导涨跌幅比例。

## 1. 事实纠偏：数据源已在仓库，不是外部制度表

此前文档把 `price_limit` 描述为「需交易所涨跌幅制度表」，实际语义是**该证券当日是否触及涨跌停（能否成交）**，不是「涨跌幅比例」。

证据链：

1. `internal/quant/risk/tradability.go` 的封闭事实枚举 `TradabilityFactPriceLimit = "price_limit"`，`Describe` 返回「涨跌停状态」；门禁只判断「已核验/缺失」，不消费比例。
2. `internal/quant/application/tradability_facts.go` 的 `assess` 模式：事实可用且命中已知不可交易取值 → `BlockingFacts`；否则视为可交易。`price_limit` 只需「是否涨跌停」二值。
3. `internal/marketdata/provider/provider.go` 的 `FullLimitPoolProvider` 注释直接点名消费者：「需要『该证券不在池内』这类完整性结论的消费者（例如**可交易性事实**）只能走本接口，并在返回后断言 `Total == len(Items)`」。
4. `internal/marketdata/model/model.go` 的 `LimitPoolSnapshot` 携带 `Total`、`Items`、`TotalIsLowerBound`、`HasData`、`QualityStatus`；Eastmoney 能力 `LimitPools: true` 已开启，`FetchFullLimitPool` 支持指定 `trade_date`（历史可回溯）。

**结论**：`price_limit` 的数据源 = Provider 全量涨停池 + 全量跌停池（`up`/`down`）。不需要新增外部制度表，也不需要推导比例。

## 2. 语义与时点约束

- `limit_up`：证券出现在**完整**涨停池（`Total == len(Items)` 且非下界）→ 不可买。
- `limit_down`：证券出现在**完整**跌停池 → 不可卖。
- `none`：两个池都完整且证券都不在池内 → 可正常涨跌。
- `unknown`：任一池不完整（`TotalIsLowerBound` 或 `Total != len(Items)`）且证券不在已知池内 → 不能断言「未涨跌停」，按缺失处理。

**PIT 时点**：涨跌停池是交易日**收盘后**的当日事实。盘中采集时池子不完整，`FetchFullLimitPool` 的完整性断言会失败，Creator 必须 fail-closed；`unknown` 只能来自「池不完整」这一显式状态，不能由盘中静默产生。

## 3. 方案：新建 `limit_pool` 授权快照

不复用 `security_trading_status_facts` 1.0.0：后者是单证券五项事实、已 frozen 且有真库快照；涨跌停池是**全量池**结构，硬并入需要 MAJOR 改动且会让 payload 语义混杂。按 DEC-PLAT-002-066 已建立的 `order_book` 模式新增独立数据类，复用共享不可变快照表按 `payload_kind` 分流。

Payload（`limit_pool` 1.0.0）：

| 字段 | 语义 |
|---|---|
| `symbol` | 被判定证券 |
| `trade_date` | 池所属交易日（`YYYY-MM-DD`） |
| `observed_at` | 来源池数据的观察时刻（收盘后） |
| `system_observed_at` | 系统采集时刻 |
| `limit_status` | `none` / `limit_up` / `limit_down` / `unknown` |
| `up_pool_total` / `down_pool_total` | 两个池的 `Total` |
| `up_pool_complete` / `down_pool_complete` | 完整性断言结果 |
| `source` / `quality_status` | 来源与质量 |

授权策略：新增 `limit-pool-facts-access`（独立于 `security-status-facts-access` 与 `order-book-facts-access`），不与任何既有策略共用。

## 4. Quant 消费与契约影响

- `TradabilityEvidence` 从两份事实扩为三份：交易状态、盘口、涨跌停池；每份都必须有身份/时点一致的授权引用。
- `AssessTradabilityFacts` 的 `price_limit` 改为：`limit_up`/`limit_down` → `BlockingFacts`；`none` → 可核验；`unknown`/缺失 → `MissingFacts`。`Verified` 只有在三项事实全部可核验时才可能为 true。
- Risk Stage Output 的 `evidence_refs` 从 4 段扩为 6 段（新增池快照 + 池授权），需按 GOV-CTR-004 以 MAJOR 发布 `3.0.0`；2.0.0 保持冻结。

## 5. 分组与退出条件

| 组 | 内容 | 退出条件 |
|---|---|---|
| D1 | Data `limit_pool` payload + Build/Validate + 单元测试 | 四状态与完整性规则全部有负例；不完整池不得产出 `none` |
| D2 | Data Creator：Provider 全量池 + 完整性断言 + 授权签发 + 持久化 + 真库测试 | 真实 MariaDB 快照可写可读；盘中 fail-closed |
| D3 | Data Authorized Reader + `limit-pool-facts-access` 策略 | 跨策略串读被拒；撤销后读取失败 |
| Q1 | Quant 消费 + 证据三元 + Risk Stage Output 3.0.0 | `missing_facts` 不再含 `price_limit`；涨跌停时进入 `BlockingFacts` |
| V1 | L3 种子 + 全链真库 + CI + 文档回填 | L3 `missing_facts` 为空或只剩真实缺失项；CI 按 SHA 回填 |

## 6. 边界

- 不推导涨跌幅比例，不使用成交量/流动性模型，不把「未在池中」当成「无涨跌幅限制」以外的推断。
- 不修改 `security_trading_status_facts` 1.0.0、`order_book` 1.0.0、Risk 2.0.0 的任何字节。
- 盘中不产出 `none`；池不完整只产出 `unknown`。
- 不伪造 Provider 响应；L3 若无法取得真实池数据，保持 `unknown` 并如实记录，不用夹具冒充生产事实。
