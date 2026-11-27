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
- **空池的歧义属于「交易日是否可用」，不属于「该池是否零事件」**（2026-11-27 修正）：
  同一天只要有一个池返回了真实行，该交易日即可用，另一个 `tc=0 / pool=[]` 的空池就是**零事件**，
  此时空池按「完整且为空」处理，但载荷质量必须降为 `degraded`（结论依赖兄弟池存活性推断）。
  两个池都没有行时无法区分「当日零涨跌停」与「该交易日不可用」，只能 `unknown`。
- `unknown`：任一池不完整（`TotalIsLowerBound` 或 `Total != len(Items)`）、或两池皆空、或证券不在已完整确认的池内 → 不能断言「未涨跌停」，按缺失处理。

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

## 7. 实施结果（2026-11-27）

| 组 | 提交 | 结果 |
|---|---|---|
| D1 | `1aff3d01d` | `limit_pool` payload + 完整性校验；3 处变异有效 |
| D2a | `f1ba657e2` | Manifest/快照/JCS 双指纹；识别 `trade_date` 在 manifest 指纹中的等价冗余 |
| D2b | `c676e6ff8` | Creator + 仓储 + `limit-pool-facts-access` + CI 真库步骤；收盘门禁变异有效 |
| D3 | `c6035b801` | 授权读取器 + 授权读分支 + 跨数据类隔离；绑定错数据类变异有效 |
| Q1 | `7775fb96f` + `7f3d43015` + `dc898c2fc` | Quant 证据三元 + `create_risk`/Stage/组合根 + Risk Output 3.0.0 |

### 7.1 门禁结论变化（已验收）

L3 `TestQuantResearchMariaDBRedisL3` 在真实 MariaDB/Redis 下断言：`quant.risk` 终态 `succeeded`、payload `status=pass`、`result_status=completed`、`missing_facts=[]`、`input_refs` 为 6 段且三个快照引用互不相同。这是本项目第一次在真库链路上得到**非拒绝**的可交易性结论。

### 7.2 首轮 CI 失败与根因（如实记录）

- Q1 首轮提交 `7775fb96f` 的 `Full integration checks` 失败。根因不是实现错误：L3 隔离库没有 `limit_pool` 快照，`RiskAdapter` 创建快照时失败，`quant.risk` 因此不产出结果。
- 修复：`quant_research_l3_seed_test.go` 新增 `seedQuantResearchL3LimitPool`（两池完整、不含该证券，测试专用），并在 `seedQuantResearchL3` 发布 `limit-pool-facts-access` 策略；Creator 走精确 `system_as_of` 复用，不触网。
- 第二轮 `7f3d43015` 仍失败，但根因已变为测试断言过期：L3 的 `input_refs` 仍按 4 段断言。修正为 6 段 + 三个快照互异后，`dc898c2fc` 的 13 项 check run 全部 success。

### 7.3 已知边界（不夸大）

- **Provider 不可用时的行为**（2026-11-27 部分改善）：Provider 请求本身失败（网络/协议/安全上界）时 `CreateLimitPoolFactsSnapshot` 仍是 fail-closed，`quant.risk` 会**失败**；但「两个池都返回空」不再报错，而是产出 `unknown` 载荷，让门禁给出可追溯的 `reject`。因此当前剩余的是"请求失败"这一条，而不是"当日无涨跌停"这一条。
- **真实来源运行证据**：L3 用的是隔离库测试专用快照与种子策略，证明的是代码链与持久化/PIT 语义；真实交易日的 Provider 响应与盘中/盘后行为仍需以真实运行证据复核。
- 契约状态：QNT-CTR-003 仍 `deferred`；本次只新增 `risk-stage-output` 3.0.0（MAJOR）与 `limit_pool` 1.0.0，未升级契约状态。

## 8. 真实来源证据与空池语义缺陷（2026-11-27）

### 8.1 真实探针

新增 `TestLiveEastmoneyFullLimitPoolCompleteness`（`internal/marketdata/provider/eastmoney_limit_pool_live_test.go`），由 `MARKET_DATA_LIVE_LIMIT_POOL_TESTS=1` 门控，接入 `.github/workflows/live-provider-smoke.yml`（提交标记 `[include_limit_pool]` 或手动 dispatch 触发），并上传 `live-limit-pool-report.json`。它验证：

- `Total == len(Items)`、`!TotalIsLowerBound`、`ProviderReturned == Total`（全量入口的完整性前提）；
- 池内证券符号唯一、涨停池只有上涨证券、跌停池只有下跌证券（可捕获上下池接反）；
- 同日两池互斥；
- 真实响应能驱动 `none` / `limit_up` / `limit_down` 三个分支，且 `none` 载荷能通过 `Validate`。

### 8.2 首次真实运行暴露的缺陷（已修复）

真实响应（2026-11-27）为：涨停池 `417/417`，跌停池 `tc=0, pool=[]`。这是一个普通的强势交易日，但按原实现会走成：

1. `FetchFullLimitPool("down")` 返回 `Total=0, HasData=false, QualityStatus=unknown`，原因 `provider_returned_empty_pool_cannot_distinguish_zero_events_from_unavailable_date`；
2. `BuildLimitPoolFactsPayload` 直接拒绝（"pool has no data"），`limitPoolQuality` 也拒绝 `unknown`；
3. `CreateLimitPoolFactsSnapshot` 失败 → `RiskAdapter` 失败 → `quant.risk` 失败 → 整个 Run 失败。

即：**只要当天没有跌停（或没有涨停），`price_limit` 就不可核验，整个 Run 直接失败**。这是把「池为空」与「交易日不可用」混为一谈导致的真实可用性缺陷。

### 8.3 修复语义（唯一正确判据）

- 引入 `dateLive := up.HasData || down.HasData`：交易日可用性是**日期级**属性，由同日任一个非空池证明。
- `upComplete := dateLive && limitPoolComplete(up)`，`downComplete` 同理；空池在日期可用时按「完整且为空」处理。
- 两池皆空 → `dateLive=false` → 两者都不完整 → `unknown`（fail-closed），且**返回载荷而不是报错**，让门禁给出可追溯拒绝。
- 质量：空池只能由源端声明歧义（`unknown`）。空池声称 `valid/degraded`、或有行却声明 `unknown`，都属于自相矛盾，直接拒绝；只要存在空池，最终质量一律 `degraded`。

### 8.4 TDD 与变异

- 修复前失败证据：`empty limit-down pool next to a populated limit-up pool must be usable: invalid limit pool facts payload: pool has no data`；`two empty pools must yield an unknown payload, not an error`。
- 三处变异均有效失败并恢复：① 忽略 `dateLive`（两池皆空会被判成 `none`）；② 允许空池声称 `valid`；③ 空池结论不降级质量。
- 全仓 `go test ./... -count=1`、`go vet ./...`、`git diff --check` 通过；本地真实探针 PASS（`2026-11-27`，涨停 417、跌停 0，`600519.SH` = `none`，quality `degraded`）。

### 8.5 另一个真实行为（不是缺陷，但必须记住）

请求历史交易日时，Eastmoney 的 `qdate` 仍返回**最新交易日**（实测请求 `20261126` 返回 `qdate=20261127`），而 `tc` 随请求日变化（417 / 65 / 51），说明 `date` 参数确实生效。因此**请求日才是权威**，与 Provider 自己声明的 `requested_trade_date_is_authoritative` 一致；探针只断言 `qdate` 可解析且不早于请求日，绝不把它当作交易日身份。

## 9. 真实端到端与一个未决的时点架构问题（2026-11-27）

### 9.1 同一缺陷在取数层的另一半（已修复）

第 8 节只修了载荷层，**真实端到端测试证明还不够**：`DataService.validateFullLimitPool` 仍要求 `snapshot.HasData`，所以强势日（跌停 `tc=0`）的空池在到达载荷层之前就被判成「源不可用」。修复：

- `len(Items)==0 && Total==0` 视为合法零事件池，原样返回；`Total>0 && len(Items)==0` 仍按部分池拒绝。
- 新增失败分类哨兵：`ErrLimitPoolSourceUnavailable`（无来源/不实现全量接口/请求失败）与 `ErrLimitPoolSourceIncomplete`（部分池、下界总量、身份不符）。前者重试可能有救，后者没有意义。

### 9.2 真实端到端证据

`TestLiveLimitPoolFactsProductionChain`（`MARKET_DATA_LIVE_LIMIT_POOL_CHAIN=1` + `MARKET_DATA_TEST_MYSQL_DSN`）走真实路径：Eastmoney 全量池 → `CreateLimitPoolFactsSnapshot`（摘要指纹 + 授权签发）→ MariaDB 不可变快照 → `ReadAuthorizedLimitPoolFactsSnapshot` → Quant 消费的 DTO。2026-11-27 实测：

```
status=none up=417 down=0 quality=degraded
```

同时验证：精确 `(symbol, trade_date, purpose, system_as_of)` 复用返回同一 `SnapshotRef`；同一 ref 走盘口读取入口必须失败（跨数据类隔离）。

负例 `TestLiveLimitPoolFactsRejectsEvaluationBeforeObservation` 断言「评估时点早于采集」必须被 `ErrLimitPoolFactsSnapshotStaleAsOf` 拒绝。变异「移除 Creator 的时点校验」后仍被快照构建的第二层校验拒绝，说明这是双保险。

### 9.3 未决问题：在线采集与 `system_as_of` 顺序冲突（需要设计决议）

代码事实：

- `quant.risk` Stage 收到的 `SystemAsOf` 来自 Run：`worker.go:280 → snapshot.Run.SystemAsOf`，而 `create_run.go:231` 把它设为**创建 Run 的时刻**。
- Creator（涨跌停与盘口都一样）要求 `observed_at <= system_as_of`，否则以 `ErrLimitPoolFactsSnapshotStaleAsOf` 拒绝。

推论：**「Run 创建 → Stage 内现场采集」这个顺序在契约上永远不成立**——采集一定晚于 Run 创建。L3 之所以通过，是因为它预先把种子快照写成与 Run 相同的 `system_as_of`，命中精确复用路径，从未真正现场采集。

可选方案（未实施，需决议）：

| 方案 | 做法 | 代价 |
|---|---|---|
| A. 预采集（保持现状语义） | 由独立采集任务按 `(symbol, trade_date, purpose, system_as_of)` 先落快照，Stage 只走复用 | 需要调度与对齐机制；Run 与快照时点必须精确匹配 |
| B. Stage 内现场采集 + 评估时点前移 | 允许 Creator 用「采集时刻」作为 `system_as_of`，Stage 输出按该时点标注 | 改变「评估时点」的权威来源，需重审 PIT 可复现性与结果关联 |
| C. 事实快照不绑定评估时点 | DTO 的 `system_as_of` 改为「观测时点」，Run 的时点只用于筛选 | 契约语义变更（MAJOR），影响所有事实类快照 |

本轮不擅自选择：这决定「Run 的时点是什么」这一根语义，属治理决议。已登记为 OPEN。
