# price_limit 事实闭环方案与前置核实

- 状态：方案已定（主路径已更换），Step 1a 待实施
- 日期：2026-11-26
- 分支：`repair_all`，HEAD `548342452`
- 对应任务：执行清单 1.4 剩余部分 / QNT-010
- 前置文档：`doc/quant/09-risk-tradability硬门禁切片与前置.md`（8.5、8.6）、`doc/quant/11-Risk门禁Stage装配与拒绝路径影响分析.md`
- 相关决策：DEC-PLAT-002-060、061、062

## 1. 结论

1. **原主路径（制度解析 → 限价 → 触板）不可行，已放弃**。三条独立证据见 2.1：特殊事件无生产数据源、公告源无法给出"无事件"覆盖确认、限价需除权还原且无可靠依据。
2. **主路径改为：涨跌停池直接事实**。东财涨停/跌停/炸板三池是"按交易日全市场枚举"的源，可直接回答门禁真正要问的问题——该证券该日是否封在涨停/跌停。它同时绕开制度解析、特殊事件、限价计算与除权处理四个难点。证据见 2.2。
3. **上一轮"缺失项 2→1"的估计不成立**，需要在本次一并纠正：`tradable_volume` 仍缺失，`price_limit` 在改路径后**有希望真正 available**（而非只改善拒绝理由），但受三条硬约束限制（见 2.3）。
4. 主线仍建议切阶段 2.1，与 Step 1a 并行；1.4 的放行不解除任何下游阻塞。
5. `tradable_volume` 维持"先调研、后决议"（第 9.2 节）。

## 2. 前置核实

### 2.1 原路径为何被否

#### 2.1.1 特殊无涨跌停事件没有生产数据源

```
internal/marketdata/provider/provider.go:115   type PriceLimitEventProvider interface
internal/marketdata/service/price_limit_event.go:18   len(s.priceLimitEventProviders) == 0 → error
```

全仓 `FetchPriceLimitEventEvidence` 只有 test stub；`internal/marketdata/provider/` 下 30 余个 provider 无一实现。生产环境 `DataService.PriceLimitEventEvidence` 恒返回 `no enabled price-limit-event provider`。

而 `ResolvePriceLimitRule`（`service/trading_rules.go:87-127`）只有 `specialException == None`（"无特殊事件"已被来源确认）才可能产出 `limited + valid`；`Unknown` 一律返回 `Status=unknown / QualityStatus=unknown / LimitPercent=nil`。没有 provider ⇒ 永远 `Unknown` ⇒ 制度解析**永远不可用**。

这不是缺陷，是 fail-closed 的正确表现：不能因为"没查到事件"就断言"没有事件"。

#### 2.1.2 公告源无法提供"无事件"覆盖确认（实测）

2026-11-26 实测东财公告接口（`np-anotice-stock.eastmoney.com`）：

```
GET ...&begin_time=2026-11-22&end_time=2026-11-22&stock_list=600000
→ {"data":{"list":[],"page_index":1,"page_size":5,"total_hits":0},"error":"","success":1}
```

空区间与"该股当日无公告"返回值**完全一致**，接口不提供任何覆盖保证。本仓库代码已经承认这一点：`emptyEastmoneyAnnouncementSnapshot` 把空列表置为 `QualityUnknown` + `provider_returned_empty_announcement_list_not_confirmed_as_no_announcements`，已有离线回归测试 `TestEastmoneyFetchStockAnnouncementsKeepsEmptyListUnknown`。

因此 `CoverageConfirmed=true` 无法构造 ⇒ `specialException=None` 无法构造 ⇒ 即使补公告源也解不开 2.1.1。

#### 2.1.3 限价计算需要除权还原，现有依据不足

`limit_up = round_half_up(prev_close × (1+p), 2)` 依赖**除权参考价**作前收盘。`market_data_adjustment_factor_versions` 与 `market_data_corporate_action_versions` 存在，但覆盖度未经核验；一旦除权日因子缺失，限价会系统性算错，而"算错的限价"看起来和"算对的限价"一模一样。这是最危险的一类错误，不满足"可证实"的要求。

### 2.2 替代路径：涨跌停池（实测覆盖）

东财 push2ex 三池（`getTopicZTPool` / `getTopicDTPool` / `getTopicZBPool`）已在 `internal/marketdata/provider/eastmoney.go:2937` 实现，模型 `model.LimitPoolItem` 携带 `FirstTouchTime`、`LastTouchTime`、`BrokenCount`、`OpenTimes`、`ConsecutiveBoards`、`SealMoney`——**直接是涨跌停状态事实**，不需要任何制度推导。

2026-11-26 实测（`date=20261125`）：

| 池 | `tc`（源端总数） | 返回行数 | 代码前缀分布 |
|---|---|---|---|
| 涨停 `ZT` | 51 | 51 | 002×20, 600×10, 603×9, 000×4, 301×3, 605×2, 300×2, 601×1 |
| 跌停 `DT` | 43 | 43（须 `sort=fund:asc`） | 603×19, 002×9, 600×4, 605×3, 601×2, 000×2, 001×2, 300×1, 003×1 |
| 炸板 `ZB` | 30 | 30 | — |

其他日期核查：

- `20261124`：涨停 `tc=55`，含 **1 只 688（科创板）** ⇒ 科创板覆盖成立，不是"排除"；
- `20261120`：涨停 `tc=28`；
- `20261121`（周六）：`tc=0, rows=0` ⇒ 非交易日返回空，需与"零涨停"区分；
- 三个交易日**均未出现** 北交所代码（`4xxxxx` / `8xxxxx` / `920xxx`）⇒ 北交所覆盖未验证。

补充事实：`fbt:asc` 排序下 `DTPool` 会返回 `tc>0` 但 `pool=[]`（provider 源码注释已记录该行为并改用 `fund:asc`）。任何自行实现的取数必须沿用此规避。

### 2.3 池子路径的三条硬约束（每条都必须体现在实现里）

#### 约束 1：现有 `FetchLimitPool` 会**静默截断**，不能直接用于全市场快照

```go
// eastmoney.go:2954-2959
if limit <= 0 { limit = 100 }
if limit > 100 { return errors.New("limit-pool limit must not exceed 100") }

// eastmoney.go:3071-3078
targetRows := sourceTotal
if targetRows > limit { targetRows = limit }
if sourceTotal == 0 || len(poolRows) >= targetRows { break }
```

即：`limit` 上限 100，且取满 `limit` 即停止，**不报错**。强市日涨停可能超过 100 只，此时"没返回"的股票会被误判成"未涨停"——把涨停说成可买，是本项目最不可接受的错误类型。

**因此 Step 1a 必须新增"全量分页"读取入口**（不复用该截断语义），并在返回前断言 `len(items) == sourceTotal`；不相等即视为该日快照不可用。

#### 约束 2：覆盖范围必须显式声明，不得默认

- 沪深主板 / 创业板 / 科创板：已观察到实例，按覆盖处理；
- 北交所：**未验证**。在拿到实证前，北交所标的 `price_limit` 一律 `unavailable`，原因码 `limit_pool_board_coverage_unverified`；
- 港/美股、基金、债券：不在池内，`unavailable`。

#### 约束 3：非交易日与"零涨停"必须区分

`tc=0, rows=0` 在非交易日与"全市场无涨停"时返回值相同。构造事实前必须先由**交易状态快照**确认 `is_trading_day=true`；否则 `unavailable`，原因码 `trade_date_not_a_trading_day`。不得用日线是否存在反推交易日。

## 3. Step 1a：涨跌停池 PIT 存储与全量读取

### 3.1 存储形态

池子是"按交易日全市场"的枚举，天然适合**按交易日落一份全市场快照**，而不是按证券逐个建快照：

- 新表 `market_data_limit_pool_day_versions`：
  `(trade_date, pool_type, revision_hash, recorded_at, fetched_at, available_at, coverage_scope, source, source_version, item_count, provider_total, complete, quality_status, payload_json)`
  主键 `(trade_date, pool_type, revision_hash)`，as-of 索引 `(trade_date, pool_type, recorded_at)`。
- `pool_type ∈ {up, down, broken}`；`complete=true` 当且仅当 `item_count == provider_total` 且 `provider_total` 来自源端 `tc`。
- 三池必须在**同一次采集**里取齐；任一池缺失或 `complete=false` ⇒ 该日快照整体不可用于"未涨停"推断（可仍用于"在池中"推断，但要分字段表达，见 4.2 第 4 条）。

### 3.2 采集入口

- 新增 provider 方法 `FetchLimitPoolFull(ctx, poolType, tradeDate) (snapshot, error)`：分页取全、断言 `item_count == tc`、保留 `qdate` 与 `tc`；`limit`、`complete` 语义显式。
- 数据源版本：`eastmoney-limit-pool-v1`（沿用既有 `SourceVersion` 命名风格）。
- 采集须记录 `available_at`：池子是收盘后统计，`available_at` 不得早于当日收盘 + 结算延迟（保守取收盘后 30 分钟），并作为 PIT 可见性边界。

### 3.3 迁移

新增迁移 **V3**（V1 基线 + V2 `payload_kind` + V3 本表）。参照 `cb11af410` 的教训，迁移一改必须同时确认 `mysql_versions_test.go`、`bootstrap/module_test.go` 与 `internal/server/migration/registry_mariadb_integration_test.go`（后者已改为由 `MigrationDefinitions()` 推导，理论上无需改，但要跑一遍）。

## 4. Step 1b：`security_price_limit_status` Payload

### 4.1 结构

```json
{
  "schema_version": "1.0.0",
  "payload_kind": "security_price_limit_status",
  "symbol": "002548.SZ",
  "trade_date": "2026-11-25",
  "board": "main_board",
  "quality_status": "valid",
  "quality_reasons": [],
  "limit_status": "at_limit_up",
  "status_evidence": {
    "pool": "up",
    "first_touch_time": "09:31:02",
    "last_touch_time": "14:57:11",
    "open_times": 2,
    "seal_money": 123456789.0,
    "consecutive_boards": 2
  },
  "coverage": {
    "scope": "cn_a_share_main_board_chinext_star",
    "complete": true,
    "provider_total": 51,
    "item_count": 51,
    "pool_snapshot_ref": "data_snapshot:sha256:...",
    "available_at": "2026-11-25T07:30:00Z"
  },
  "source": "eastmoney",
  "source_version": "eastmoney-limit-pool-v1"
}
```

`limit_status ∈ {at_limit_up, at_limit_down, broken_limit_up, none}`。

### 4.2 六条实现要求（每条都是验收点）

1. **`none` 只能由完整池推出**。`limit_status=none` 要求三池 `complete=true` 且该 symbol 的上市板在 `coverage.scope` 内。任一不满足 ⇒ `quality_status=degraded` 且 `limit_status=unknown`，**不得**输出 `none`。
2. **优先级固定**：`up` > `broken` > `none`；`down` 与 `up` 互斥（同一证券同日不可能同时封涨停与跌停，若源端同时出现 ⇒ 该日快照 `degraded` + 原因码 `limit_pool_conflicting_membership`，不择一）。
3. **炸板不等于可交易，也不等于涨停**。`broken_limit_up` 表示"盘中曾涨停、收盘未封"，必须独立表达，不得折叠进 `at_limit_up` 或 `none`。
4. **可用性分层**：`limit_status=none` 的可用性依赖三池齐全；`limit_status=at_limit_up` 只依赖 `up` 池完整。因此 `coverage.complete` 需按池记录（`up_complete` / `down_complete` / `broken_complete`），不能只给一个布尔。
5. **前视防护**：`system_as_of` 必须 ≥ `coverage.available_at`，否则 `unknown`，原因码 `limit_pool_not_yet_available_at_system_as_of`。禁止用"当天实时池"回答收盘前的问题。
6. **不得由池反推涨跌停价或制度百分比**。本 payload 不含 `limit_percent` / `limit_up_price`，也没有这些字段的位置；任何消费者需要百分比必须另行取制度事实，不能从本事实反算。

### 4.3 与既有代码的关系

- 复用 `model.LimitPoolItem` 的字段语义，不新建平行模型；
- **不复用** `FetchLimitPool`（截断语义，见 2.3 约束 1）；
- 不改 `ResolvePriceLimitRule` / `ResolvePriceLimitRuleFromTradingStatus` / `/price-limit-rule` 端点：它们仍是"制度解析"能力，与本事实并行存在，本事实**不消费**它们。

## 5. Step 1c：授权数据类与读写通道

- 新增 data class policy ref：`security-price-limit-status-access`（与 `SecurityStatusFactsPolicyRef` 分开；撤销粒度不同，理由同 DEC-061）。
- 快照写入复用共享不可变快照表 + `payload_kind` 分流（表名仍不动），与 `security_trading_status_facts` 同构；日快照行与证券级 payload 通过 `pool_snapshot_ref` 互相引用。
- `ReadDataAuthorizedSecurityPriceLimitStatusSnapshot`：必须**同时**校验 policy ref 与 `payload_kind`（只凭 `snapshot_ref` 无法区分类型）。
- `bootstrap.Module` 对外 `Create...` / `ReadAuthorized...` 入口；Owner 策略路由复用 `/api/v1/owner/data/...` 模式新增一条。

## 6. Step 1d：Quant 侧映射

- 新增 `PriceLimitStatusReader` 接口，不改既有 `TradingFactsReader`（避免破坏既有测试桩）。
- `AssessTradabilityFacts` 接受第二份快照；`price_limit` 的映射规则：

| 事实状态 | 映射 |
|---|---|
| `at_limit_up` / `at_limit_down` | `BlockingFacts`（已核验且结论为不可交易），**不是** missing |
| `broken_limit_up` | 不 block（盘中曾涨停但收盘未封不影响次日可交易性）；但必须在 `QualitySummary` 留痕 |
| `none` 且 `quality=valid` 且覆盖内 | 视为已核验，不写入 missing |
| `unknown` 或 `quality=degraded` 或覆盖外 | 写入 `missing_facts`，附 reason code |

- 不拆分 `TradabilityFact` 枚举（第 7 节已论证）。
- 不声明 `hard_risk`。

## 7. 契约影响

| 契约 | 影响 | 处置 |
|---|---|---|
| DATA-CTR-007 | 新增 payload kind、日池快照表与数据类策略引用 | 追加实现补充，状态仍 `deferred` |
| DATA-CTR-008 | 无影响（close series 不变） | 不改 |
| QNT-CTR-004 | `TradabilityFact` 是否拆分 | **不拆**（见下） |
| QNT-CTR-003 | 不拆即无需变更 `missing_facts` / `blocking_facts` 封闭 enum | 不改 |
| PLAT-002 | 新增 DEC 登记本 payload、覆盖声明语义与六条实现要求 | 新增 DEC-PLAT-002-063 |

**为什么不把 `price_limit` 拆成 `price_limit_rule` + `price_limit_touch`**：语义更细，但要同时改 `TradabilityRuleSetVersion`（`qnt-risk-tradability/1.0.0 → 1.1.0`）、标签表、两份 Stage Schema 的封闭 enum，而**门禁结论不因此改变**。改为在 Schema `description` 与 DEC 中写清"`price_limit` 表达该交易日是否封在涨跌停"，语义同样不模糊，且不给冻结契约制造无收益的版本变更。

## 8. 验收方案

### 8.1 测试点（12 条）

| # | 测试点 | 层次 |
|---|---|---|
| 1 | 池快照：同 `(trade_date, pool_type)` 两版本，按 `as_of` 取到较旧版本 | Data repository |
| 2 | 全量读取：`tc=137` 时断言取满 137 条（现有 `FetchLimitPool` 在 `limit=100` 处截断，必须失败） | provider |
| 3 | `complete=false` ⇒ `limit_status=none` 被拒绝，返回 `unknown` + reason | Data app |
| 4 | 非交易日（`tc=0` 且非交易日）⇒ `unavailable`，不是 `none` | Data app |
| 5 | 北交所标的 ⇒ `unavailable` + `limit_pool_board_coverage_unverified` | Data app |
| 6 | 同证券同时出现在 `up` 与 `down` ⇒ `degraded` + `limit_pool_conflicting_membership`，不择一 | Data app |
| 7 | `broken` 成员 ⇒ `broken_limit_up`，不折叠为 `at_limit_up` 或 `none` | Data app |
| 8 | 前视防护：`system_as_of` 早于 `available_at` ⇒ `unknown` | Data app |
| 9 | 授权：用 close-series 策略读本 payload 必须拒绝（policy ref 与 kind 双重校验） | repository 集成（需 DSN） |
| 10 | Quant：`at_limit_up` ⇒ `BlockingFacts` 而非 `missing_facts` | Quant application |
| 11 | Quant：`broken_limit_up` 不 block 但留痕；`unknown` ⇒ missing | Quant application |
| 12 | 端到端：拒绝路径的 `missing_facts` 与 reason code 与 DEC 一致，Stage 仍 `succeeded` | bootstrap |

### 8.2 变异验证（6 处，每处必须实际失败并恢复）

| # | 变异 | 期望失败点 |
|---|---|---|
| 1 | 全量读取改回 `FetchLimitPool`（100 条截断） | 测试点 2 |
| 2 | `complete=false` 时仍输出 `none` | 测试点 3 |
| 3 | `tc=0` 一律当作 `none`（不看交易日） | 测试点 4 |
| 4 | 北交所标的按"不在池中即未涨停"处理 | 测试点 5 |
| 5 | 去掉 `system_as_of >= available_at` 校验 | 测试点 8 |
| 6 | Quant 把 `at_limit_up` 仍算作 missing | 测试点 10 |

## 9. Step 2 / Step 3

### 9.1 Step 2：主线切阶段 2.1（建议与 1a 并行，优先）

依据：`quant.risk` 决策为 `reject` 时 Stage 仍 `succeeded`，完整 Template 可跑通；`Run↔Result` 关联与报告读取 DTO 都不依赖 tradability 放行。1.4 的"放行"不解除任何下游阻塞。

### 9.2 Step 3：`tradable_volume` 先调研后决议

规则目录 `liquidity` 仍 **pending**（缺权威来源），"当日成交量 × 参与率上限"不是想走就能走。

| 选项 | 内容 | 代价 | 前置 |
|---|---|---|---|
| A | 真实成交量（`market_data_daily_bar_versions.volume_shares` 已有 PIT）+ 版本化参与率假设 | 需权威出处，找不到就不能发布为规则 | 权威来源 |
| B | 新增盘口通道（provider + normalize + model + 快照 + 授权 + 契约） | 一条完整数据通道；唯一零假设路径 | 至少一个 Provider 真实可行 |
| C | 维持恒 reject，登记"数据能力受限，长期可追溯拒绝" | 零成本，门禁永久不放行 | 无 |

A 的证据基础比上一轮估计好（成交量有 PIT），但假设本身无权威出处仍是硬阻塞；B 目前全仓零盘口能力（`Quote` 无买一卖一、能力清单无 orderbook）。

## 10. 非目标

- 不做"制度 → 限价 → 触板"推导，不做 `limit_percent` / `limit_up_price`（2.1 已论证不可行，2.3 约束 6 禁止反算）；
- 不修改 `ResolvePriceLimitRule` 系列函数与 `/price-limit-rule` 端点的既有 fail-closed 行为；
- 不降低 `tradability_unverified` 的 severity 或 `Required`；
- 不拆分 `TradabilityFact` 枚举；
- 不把 `tradable_volume` 移交给尚不存在的执行层（1.7 的 Planner/Execution 仍是占位）；
- 不用"不在池中"推断覆盖范围外的证券；
- 本轮不声明 `hard_risk` 门禁。

## 11. 停止条件（整体）

任一条成立即停止并回退到选项 C（登记"数据能力受限，长期可追溯拒绝"），不得靠调整断言强行推进：

1. 全量读取无法稳定断言 `item_count == tc`（分页期间 `tc` 变化、`qdate` 漂移）；
2. 池子覆盖范围无法界定（哪些上市板在池内说不清），导致"不在池中"不可解释；
3. 池子与日线/交易状态出现系统性矛盾（例如池内证券当日日线缺失），且无法定位原因；
4. 任何实现需要把"未知"写成"正常"或把"涨停"写成"可交易"才能通过测试。
