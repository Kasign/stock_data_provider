# 09 · Risk tradability 硬门禁切片与 1.4 前置（执行清单 1.4）

- 状态：**门禁切片已落地（2026-11-26）**；1.4 记为"部分完成"，其余部分阻塞于 Signal 契约、Data 可交易性授权事实与权威规则目录，不由 Quant 自行猜测。
- 范围：把旧 Analysis 链的 `unverified-tradability-block` 正式化为版本化量化规则，并核实 1.4 的真实前置。
- 关联：`旧Analysis退役执行清单.md` 1.4、`doc/quant/04-RiskDecision DTO.md`、`doc/quant/01-量化引擎总体设计.md`（RiskEngine）、`doc/quant/模块总任务规划和进度.md` QNT-010 / QNT-008。

## 1. 代码级事实：旧链的 tradability 是"恒拒绝"

`internal/service/wiring.go:373-389` 的 `tradabilityChecker.Check` 对**每一条候选信号**都返回 `Allowed=false`，理由是：

> 上游未提供可信的 ST、停牌、涨跌停和可成交量字段，无法确认可交易性

`internal/analysis/app/runner.go:552-579` 的 `approvedForPlanning` 进一步 fail-closed：**缺少可交易性判定**或**缺少风险判定**都直接拒绝，且要求"显式放行"才可能进入仓位规划。

因此旧链实际语义不是"某些证券不可交易"，而是"可交易性无法验证，所以一律不放行"。1.4 要覆盖的正是这个结论——它是**事实门禁**，不是业务阈值规则。

## 2. Quant 侧现状：框架齐全，缺三样东西

| 能力 | 现状 | 证据 |
|---|---|---|
| RiskDecision DTO | 已有，`pass/reject/unknown` + Reason 结构 + status 聚合校验 | `internal/quant/risk/decision.go`、QNT-CTR-004（`frozen`） |
| RuleSet / Evaluator | 已有：block 必须 required、缺失必需规则补 unknown、panic 转执行失败、顺序稳定 | `internal/quant/risk/rule_set.go`、`evaluator.go` |
| 状态聚合 | 已有：block triggered 优先 reject，其次 unknown，全不触发才 pass；warn/info 不得覆盖 | `internal/quant/risk/aggregate.go` |
| 结果表示 | 已有：`RiskPayload` 已注册进 Envelope（`risk/1.0.0`） | `internal/quant/result/envelope.go` |
| 规则目录 | **缺**：无权威业务规则清单，文档明示"不得由 Quant 自行猜测规则" | `模块总任务规划和进度.md:273-274` |
| 输入对象 | **缺**：`Signal`（QNT-008）NOT_STARTED，Quant 内无 Signal DTO | `当前任务状态和进度.md:34` |
| 事实来源 | **缺**：授权快照只有单证券 `daily_bars_close_series` | `internal/marketdata/app/daily_bars_close_series.go:15` |

## 3. 1.4 的真实前置（逐条可验证）

1. **输入对象**：RiskDecision 评估的是"某个待执行的候选交易"。`doc/quant/07` 第 125 行已判定"1.4 的 Risk 硬门禁以 Signal 为输入"，而 QNT-008 仍 NOT_STARTED。没有 Signal，Risk Facade/Stage 就没有可评估对象，建出来是零调用方接口。
2. **Data 可交易性事实**：`model.SecurityTradingStatusSnapshot` 已有 `RiskWarningStatus`（ST）、`SuspensionStatus`、`DelistingStatus`、`IsTradingDay`/`TradingDayKnown`、`ListingStatusKnown`/`ListingTradingDayNumber`、`quality_status`，但：
   - **没有涨跌停状态，也没有可成交量**——旧链要求的四类事实里有两类在任何 Data 模型中都不存在；
   - 没有针对它的授权快照 payload，Quant 无法以"授权 + PIT + 覆盖率"方式消费。
3. **权威规则目录**：QNT-010 文档明确"尚缺权威业务规则目录……不得由 Quant 自行猜测规则"。因此本轮不定义任何风险阈值、权重或综合评分规则。

## 4. 本轮落地：`tradability_unverified` 硬门禁切片

只落地"事实缺失即阻断"这一条，因为它与旧链等价、且不包含任何需要业务决策的阈值。

- 规则集版本：`qnt-risk-tradability/1.0.0`（`risk.TradabilityRuleSetVersion`）。
- 规则：`tradability_unverified`，`severity=block`、`required=true`——满足 `RuleSet.Validate` 对 block 必须 required 的约束。
- 封闭事实枚举（四项，与旧链理由一一对应）：`risk_warning_status`、`suspension_status`、`price_limit`、`tradable_volume`。
- 结论规则：
  - 事实未验证 → `block + triggered` → `reject`；
  - 声明已验证但缺证据引用、或证据引用为空/带首尾空白/非法 UTF-8 → 规则执行失败 → `block + unknown` → `unknown`；
  - 出现表外事实项 → 同样 `block + unknown`，而不是静默忽略。
- 不可覆盖性（结构性，不靠约定）：
  - `TradabilityGateInput` 只有 `DecisionID/EvaluatedAt/SystemAsOf/InputRefs/Facts`，**不含 Status、RuleSetVersion、评分或模型建议**；规则集版本由门禁函数固定注入，调用方无法指定；
  - `Decision.Status` 只能由 `AggregateStatus` 推导（`NewDecision` 不接受外部 status）；
  - 有白名单测试守住"新增字段必须先经过审视"，防止未来偷偷加 override 通道。

### 4.1 与旧链的等价性

| 旧链行为 | 新门禁行为 | 是否等价 |
|---|---|---|
| 缺 ST/停牌/涨跌停/可成交量 → `Allowed=false` | 事实未验证 → `block + triggered` → `reject` | 等价 |
| 缺可交易性判定 → 规划阶段拒绝 | 必需 block 规则缺失/执行失败 → `block + unknown` → 非 pass | 等价（更严） |
| 无任何"放行"依据时的默认结果 | 未验证即 reject，无默认放行分支 | 等价 |

## 5. 本轮不做与原因

- **不做 Risk Facade，不装 `quant.risk` Stage**：没有 Signal 输入，也没有可读的可交易性事实；此时建接口与 Stage 会是零调用方建设，且 Stage 只能恒定 reject，无法产生有意义的研究产物。
- **不定义其余风险规则**（集中度、流动性、波动、行业暴露等）：缺权威规则目录，Quant 猜测阈值会直接违反 QNT-010 纪律与 DEC-PLAT-002-049 同类原则。
- **不改任何契约**：QNT-CTR-004 仍 `frozen`，Envelope 已注册的 `risk/1.0.0` 未改动；RiskDecision Schema/Golden 未新增字段。

## 6. 推进条件（满足后才继续 1.4 剩余部分）

同时满足以下三条，才把 1.4 的其余部分重新排入执行：

1. **Signal 契约（QNT-008）就位**：有正式的候选交易 DTO 与版本化产出，RiskDecision 才有评估对象；
2. **Data 可交易性事实可授权消费**：为交易状态增加授权快照 payload，并补齐涨跌停状态与可成交量事实（或明确以"不可得即阻断"作为长期口径，并写入契约）；
3. **权威规则目录**：给出规则清单、严重级别、必需性与规则集发布/加载来源，Quant 只负责执行与 fail-closed，不负责发明规则。

在此之前，`tradability_unverified` 切片作为可独立复用的门禁原语保留；任何 Stage 装配都必须以它为 `required block` 规则之一。

## 7. 验证

- `go test ./internal/quant/risk/` 全绿；新增 9 个用例覆盖：事实不可用即拒绝、四类缺失事实全部进入安全消息、仅有证据才放行、声明已验证但无证据 → unknown、非法证据引用 → 非 pass、表外事实 → unknown、规则集自校验、输入无覆盖通道、决策身份非法即拒绝。
- 变异验证两处均使测试失败：
  - 把"事实未验证"的结论从 `triggered` 改为 `not_triggered` → 决策变成 `pass`，两条断言失败；
  - 去掉"已验证必须有证据引用"的强制 → 无证据场景变成 `pass`，fail-closed 断言失败。

## 8. 三条前置的推进记录（2026-11-26）

### 8.1 前置① Signal 契约：已闭环（契约层）

新增 QNT-CTR-006 Signal 1.0.0（Schema/Golden/Go DTO + 一致性测试，DEC-PLAT-002-059），Risk 从此有可机读的评估对象；明细见 `doc/quant/10-Signal契约最小切片.md`。仍不装 Stage：Strategy 评估与 Signal 产出属 QNT-008 剩余工作。

### 8.2 前置③ 规则目录：已闭环（目录与加载）

新增 `RuleCatalog`（`qnt-risk-catalog/1.0.0`）：条目携带依据来源与状态（published / registered_not_published），另有 pending 占位（主题 + 预留 code + 缺失原因）。发布规则集只能引用 published 条目，引用未登记/未发布/pending code 一律加载失败；目录版本必须与发布集合一致。当前发布 `tradability_unverified`，登记 `input_quality_not_applicable` 但不发布，集中度/流动性/波动率/行业暴露列为 pending。收敛测试断言 `Load(tradability 版本)` 与 `TradabilityRuleSet()` 完全相等。

### 8.3 前置② Data 交易事实：契约层已闭环，物理读取待接入

按"只承载真实拥有的事实"口径新增 DATA-CTR-007 扩展 payload `security_trading_status_facts` 1.0.0 与同族 Manifest（DEC-PLAT-002-060）：

| 事实项 | 来源 | 说明 |
|---|---|---|
| `risk_warning_status` | `SecurityTradingStatusSnapshot.RiskWarningStatus` | ST/风险警示 |
| `suspension_status` | `SuspensionStatus` | 停牌状态 |
| `delisting_status` | `DelistingStatus` | 退市状态 |
| `listing_date` | `ListingDate` / `ListingStatusKnown` | 上市日与上市状态 |
| `is_trading_day` | `IsTradingDay` / `TradingDayKnown` | 是否交易日 |

- 逐项 `available`（带 value、不带原因）/ `unavailable`（带原因、不带 value）；五项全可用 → `valid`，任一不可用 → `degraded`。
- **不含涨跌停与可成交量**：Provider 从未提供，写入等于把推导当事实；缺项由 `tradability_unverified` 门禁阻断。
- 指纹沿用 JCS + 双指纹 + 组合 `SnapshotRef`；新增 Schema/Golden，测试覆盖 Golden↔Schema↔DTO、派生 key（`price_limit`/`tradable_volume`）双侧拒绝、质量一致性、身份与指纹漂移、信任边界（caller_ref/purpose/UTC as-of/sources）。
- **后续已接入**（2026-11-26 深夜，见 8.5）：存储列与 payload kind 分流、授权签发与 Authorized Reader 分流、`SecurityTradingStatus` Creator、MariaDB 集成测试与 Quant 消费端 Contract Test 已全部落地。

### 8.4 因此 1.4 的状态

记录为"部分完成"，但阻塞从"三条全缺"收敛为一条：**交易事实的物理读取链路**。Signal 与规则目录就位后，Risk 已具备"有输入、有规则、有目录"的评估前提；剩下的接入工作不再是"设计缺口"，而是"实现与集成验证"。

该唯一阻塞已在 8.5 关闭；但 1.4 仍**不记为完成**——门禁事实集里的涨跌停与可成交量依旧不存在，`tradability_unverified` 必然产出 `reject`。真正阻塞"门禁放行"的是"能否产出非拒绝结论"，不是"链路是否打通"。

> 口径修正（2026-11-26，见 8.6）：装配工作本身不必等非拒绝结论——把验收目标定为"拒绝可被完整追溯与读取"后已按期落地。但**门禁放行**仍然阻塞于涨跌停/可成交量事实，1.4 因此仍为部分完成。

### 8.5 前置② 物理读取链路：已闭环，但门禁结论恒为拒绝（2026-11-26 深夜）

按用户三项决议（DEC-PLAT-002-061）落地：共享表 + `payload_kind` 列、独立 `policy_ref`、门禁事实集维持四项。

| 环节 | 落地位置 | 关键约束 |
|---|---|---|
| 迁移 | `repository/mysql.go` V2 | 只 `ADD COLUMN payload_kind`；**不改表名**（DDL 非事务，RENAME 失败会留下 `status=failed` 阻断启动） |
| 存储分流 | `daily_bars_close_series_snapshot.go` / `security_trading_status_facts_snapshot.go` | 写入显式给 kind；读取 `snapshot_ref` 必须带 kind 条件（前缀对所有类型相同） |
| 授权 | `authorization/policy.go`、`application/data_authorization.go` | 策略引用闭集；`Issue`/`AuthorizeRead`/Owner 发布/HTTP 四处校验"策略 `policy_ref` == 请求数据类"，不匹配 fail-closed，不回落日线 |
| Reader | `application/security_trading_status_facts_authorized_reader.go` | 独立方法 + 独立 Port 接口（不扩既有签名）；`policy_snapshot`/`current`/`Manifest.PurposeCode` 三重校验后才 `Evaluate` |
| Creator | `application/security_trading_status_facts_snapshot.go` | 源记录 `invalid`/`unknown` 拒绝；`degraded` 即使五项齐全也拒绝；枚举 `unknown` 一律 `unavailable` + 原因 |
| Quant 消费 | `quant/application/tradability_facts.go`、`server/bootstrap/quant_trading_facts_adapter.go` | 转换前 `Verify()`；`price_limit`/`tradable_volume` 恒入 `MissingFacts` |
| HTTP | `/api/v1/owner/data/security-status-facts-access-policy[/versions]` | manager 不支持按数据类时返回 503 不可用，**不回落**日线策略 |

- **结论诚实登记**：五项事实到位后 `Verified` 仍恒为 false（`AssessTradabilityFacts` 无条件补两项缺失），决策恒 `reject`。端到端测试名即为 `...ProducesTraceableRejection`，不写成"门禁通过"。
- 验证：`gofmt`/`go vet`/`go test ./internal/...` 全绿；`-race` 覆盖 `internal/marketdata/...`、`internal/quant/...`、`internal/server/bootstrap`。变异验证四处均有效失败：① 去掉两项强制缺失 → 决策变 `pass`；② 把 `RiskWarningUnknown` 当已核验 → value 泄漏 `unknown`；③ 去掉数据类一致性校验 → 能签出绑定错策略的授权；④ 去掉 adapter 的 `Verify()` → 接受被篡改 payload；⑤ 让 HTTP 回落日线策略 → 返回 `policy_ref=dailybars-v2-access`；⑥ 发布时传空 policyRef → 发布引用为空。
- MariaDB 集成测试（`MARKET_DATA_TEST_MYSQL_DSN` 门控）覆盖：迁移/`ValidateSchema`/幂等/跨 kind 隔离/SQL 篡改检测。**未在真实 MariaDB 环境执行**，不得宣称已验证。
- 剩余工作不属于本切片：涨跌停制度表与可成交量事实（另行立项）。Signal→RiskDecision 的 Facade/Stage 装配已在 8.6 完成。

### 8.6 Signal→RiskDecision 的 Facade/Stage 装配：已落地，结论仍恒为拒绝（2026-11-26）

按用户四项决议（DEC-PLAT-002-062，影响分析见 `doc/quant/11`）落地：

| 环节 | 落地位置 | 关键约束 |
|---|---|---|
| Facade | `quant/application/create_risk.go` | 读授权事实快照 → `AssessTradabilityFacts` → `EvaluateTradabilityDecision` → `NewRiskPayload` → `resultPersister.persist`；`trade_date > system_as_of` 直接拒绝 |
| Stage 输出契约 | `quant/stage/risk_output.go` + 两份 QNT-CTR-003 Schema | `reasons` 恰 1 条（`tradability_unverified`/`block`）；`input_refs` 恰 2 条且首条为 snapshot；`status`↔`result_status`↔`reason.status` 由 Schema 硬绑；`evaluated_at == system_as_of`、`evidence_refs == input_refs`、reject 时清单非空由运行时 Validator 强制 |
| Stage Adapter | `quant/stage/risk.go` | 参数校验（symbol 格式、UTC、`trade_date ≤ system_as_of`）→ 创建事实快照（`purpose=risk_review`）→ 调 Facade → 双 Validator 自检 |
| 组合根 | `server/bootstrap/quant_risk_stage.go` | Registry Set **1.5.0**；模板 `GateType` 留空、`RequiresHardRiskGate=false`；`OnDegraded=Accept` 其余 Fail；`unknown` 决策不产出 Stage Output（`quant_risk_decision_unknown`） |
| 四处接线 | `researchflow_runtime.go`（`isQuantStageCode`）、`researchflow_stage_dispatcher.go`、`quant_result_association.go`（规格表登记）、`researchflow_template_prerequisites.go`（`quant.risk.always` + 参数 Schema 文档） | 缺任一处即"注册成功但生产不可用/读不回"，已各配专门测试 |

- **拒绝语义**：Stage `succeeded` + `decision_status=reject` + Manifest `result_status=rejected`。消费者必须读决策；Stage 状态只表示评估已执行完成。禁止把 `reject` 映射为 Stage 失败（失败 Attempt 不产出 Stage Output，拒绝将不可追溯）。
- **本轮未声明 `hard_risk`**：决策恒 reject，声明门禁会让所有含该 Stage 的 Run 必然失败；属独立治理动作，待事实补齐后另立项。
- 验证：新增 23 个测试全 PASS；`gofmt`/`go vet`/`go test ./internal/...` 全绿；`-race` 覆盖 `internal/quant/...` 与 `internal/server/bootstrap`。变异验证 6 处均有效失败（其中"关联规格表漏登记"原契约测试未捕获，已补显式断言）。
- **结论仍是拒绝**：`AssessTradabilityFacts` 无条件补 `price_limit`/`tradable_volume`，`Verified` 恒 false。1.4 因此仍为"部分完成"——链路与装配已通，**门禁不能放行**。
- 未完成：Researchflow Composer 的 `RiskDecision` 投影（RF-BLOCK-008 仍 OPEN，wire 不兼容需双 Owner 决议）；真实 MariaDB 集成；CI 结论以推送后为准。
