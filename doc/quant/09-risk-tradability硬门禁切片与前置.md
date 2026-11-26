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
