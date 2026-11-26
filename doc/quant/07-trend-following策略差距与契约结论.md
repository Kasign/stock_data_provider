# 07 · trend-following 策略差距核实与契约结论

- 文档状态：差距核实与契约决议请求（本轮**不**实现趋势算法、不注册 `quant.trend` Stage、不改任何冻结契约）
- 核实基线：`repair_all` HEAD `1201092fc`（第⑧组与本轮契约口径修正之后）
- 对应执行清单：`doc/整体任务规划/06-联调收尾/旧Analysis退役执行清单.md` 1.3
- 结论一句话：旧 Runner 的 trend 不是"待迁移的算法"，而是一个**只有布尔合取的 MVP 过滤器**；现有 `internal/quant/trend` 只有 DTO。两者之间的差距是**语义契约缺口**，不是代码搬迁缺口。因此本轮不补算法，先出书面结论并请求 5 项语义决议。

## 1. 核实方法

逐项读代码而非读旧卡点，四类证据：

| 证据类型 | 位置 |
|---|---|
| 旧 Runner 的 trend 因子实现 | `internal/service/wiring.go:258-305`（`trendFactor`） |
| 旧 Runner 的 trend 策略实现 | `internal/service/wiring.go:307-351`（`trendStrategy`） |
| 旧 Runner 的策略质量门禁 | `internal/service/wiring.go:152-200`（`qualityAssessor.corePass` / `StrategyEligible`） |
| 旧 Runner 的引擎校验与装配 | `internal/analysis/modules/engines.go:122-194`、`internal/service/wiring.go:433-460` |
| Quant 侧现有 trend 资产 | `internal/quant/trend/trend.go`（仅 DTO + `Validate`）、`internal/quant/result/envelope.go:37-48,212`（trend Payload 已注册） |
| 新 Factor 生产 Entry | `internal/quant/registry/production_factor.go:17-31,37-86` |
| 既有决议与前置核验 | `DEC-PLAT-002-049`、`doc/quant/05-TrendResult DTO.md` 第 39-63 行、`doc/quant/模块总任务规划和进度.md` 第 13 节 |

## 2. 旧 Runner 的 trend 事实（代码级）

### 2.1 三个输入因子（float64，单位 percent）

`trendFactor{id, window, kind}` 一个类型生成三个实例（`wiring.go:428`）：

| factor_code | 公式 | 最小样本 | 单位 |
|---|---|---|---|
| `trend.ma20_distance_pct` | `(close[-1] / mean(close[-20:]) - 1) * 100` | 20 根 | percent |
| `trend.return_5d_pct` | `(close[-1] / close[-1-5] - 1) * 100` | 6 根（`len > window`） | percent |
| `trend.return_20d_pct` | `(close[-1] / close[-1-20] - 1) * 100` | 21 根（`len > window`） | percent |

样本不足时不报错：`value.Value = nil` 且写 `MissingReason = "至少需要 N 根日 K；当前 M 根"`（`wiring.go:296-298`）。`AsOf` 取最后一根 bar 的 `TradeDate`（`wiring.go:299-301`），旧链没有独立的 `system_as_of` 概念。

### 2.2 策略判定（`trendStrategy`，version `mvp-v1`）

```
信号条件： ma20_distance > 0  &&  return_5d > 0  &&  return_20d > 0
```

即**三个数严格为正的合取**（`wiring.go:340`）。没有任何幅度阈值、没有阶段、没有强度、没有多周期聚合、没有跨周期冲突判断。命中后输出一条 `CandidateSignal`：

- `direction = "long_candidate"`（单一取值，不存在 bearish/sideways 表达）
- `valid_until = as_of + 24h`（自然日 24 小时，不绑定交易日历）
- `expected_hold_days = 5`、`reference_price = 最后一根 close`
- `entry_rule / exit_rule / invalidation_rule` 为**中文自然语言字符串**（"若收盘价跌破 20 日均线…"）
- `evidence_refs = [三个 factor_code]`，`attributes` 为 `%` 保留 4 位的字符串
- 信号 ID = `runID-symbol-yyyyMMdd`，由引擎再校验唯一性（`engines.go:177-183`）

### 2.3 质量门禁与失败表达

`qualityAssessor` 把 `corePass` 定义为"至少一只自选股具 ≥21 根有效日 K"，`TaskHistoricalRun` 一律 `corePass=false`（`wiring.go:185-192`）。`StrategyEngine` 对 `StrategyEligible[id]` 缺失或为 false 的请求**直接 `continue`**（`engines.go:135-138`）：不报错、不产出原因，最终结果是空信号列表。也就是说旧链的"不可评估"与"评估后无信号"在输出上不可区分。

## 3. 现有 Quant 资产事实

| 资产 | 状态 |
|---|---|
| `internal/quant/trend.Result` | 仅 DTO + 结构 `Validate`：direction/phase 枚举、strength 可空且 ∈[0,1]、evidence_metrics/conflict_refs 必填、`system_as_of` 与每个 evidence Metric 一致。注释明确"趋势推断、强度归一化和质量策略由上层用例负责"（`trend.go:58`） |
| `trend/1.0.0` Envelope Payload | 已注册，可持久化，跨层字段一致性已由 DEC-049 约束 |
| 趋势算法 | **不存在**。无周期输入模型、无推断函数、无聚合器、无冲突生成器 |
| Trend Registry Entry | 不存在。`internal/quant/registry` 只有 SMA + 三个 Decimal Factor |
| `quant.trend` Stage | 不存在。无 Output/Quality Schema、无参数 Schema、无 Template、无关联规格表条目 |
| 首批 Decimal Factor | `ma20_distance`（20 根）、`momentum_5d`（6 根）、`momentum_20d`（21 根），`ratio`、scale=12、half_even、tolerance 末位 |
| `momentum_60d` | 只有 float64 旧函数（`internal/quant/factor/factor.go:64`）与 Schema period 枚举（`5d/20d/60d`），**没有** Decimal 生产 Entry |

## 4. 差距矩阵

| 维度 | 旧 Runner | 现有 Quant | 差距性质 |
|---|---|---|---|
| 输入算术 | MA20 距离、5/20 日收益，float64 | 同三项，`big.Rat` scale=12 精度 | **可替换**：语义等价，新实现精度更高。旧 float64 实现属重复，随旧链删除即可 |
| 单位 | `percent`，×100 | `ratio`，不 ×100 | 表示差异。旧 `attributes` 文案与"跌幅百分比"表述不能继承 |
| 时间口径 | `AsOf` = 最后一根 bar 的 trade_date | `system_as_of` + `trade_date` 分离，PIT 由 Data 快照保证 | 旧链缺 PIT 表达，不能作为新契约口径 |
| direction | 仅 `long_candidate`（有信号即看多） | 4 值枚举，且要求与 phase 独立、unknown≠sideways | **语义缺口**：旧逻辑无法产生 bearish/sideways/unknown |
| phase | 无 | 6 值枚举 | **语义缺口**：旧逻辑完全无对应物 |
| strength | 无 | 可空、∈[0,1]，且 DEC-049 禁止由 direction/phase 反推 | **语义缺口**：需要归一化公式，旧逻辑无 |
| 多周期与冲突 | 无（5/20 只作为同期输入，不构成冲突判断） | `conflict_refs` 必填 | **语义缺口**：冲突判定与排序规则未定义 |
| 不可评估表达 | 空信号列表，与"无信号"不可区分 | `direction=unknown` + `quality_status` | 新契约能表达，旧链丢失信息 |
| 产物形态 | `CandidateSignal`（含 entry/exit/invalidation 自然语言、hold days、参考价） | `TrendResult`（趋势状态），**无 Signal 类型** | **层次缺口**：趋势结果 ≠ 交易候选；QNT-CTR-004 不含 Signal |
| 装配 | 旧 Runner 内部直接调策略引擎 | 无 Stage | 距离最远：需 Registration/Output Schema/Template/关联 |

## 5. 为什么不能"直接补契约并实现"

四条硬理由，均可核对：

1. **旧逻辑不覆盖新契约的字段语义。** `TrendResult` 要求 direction 四值、phase 六值、strength ∈[0,1]、conflict_refs 显式。旧 `trendStrategy` 只能回答"三个数是否同时为正"，映射到该 DTO 必然要**新增**方向/阶段/强度判定规则——这不是迁移，是新设计。
2. **已有决议明确禁止推断。** `DEC-PLAT-002-049` 第 1 条与"明确排除"段落禁止根据方向/阶段推导强度，也不定义任何具体阈值；`doc/quant/05` 第 50-57 行列出的 7 项语义（周期映射、输入边界、单周期推断、strength 归一化、多周期聚合、不完整输入、算法版本）至今没有决议。
3. **单位与时间口径都变了。** percent→ratio、trade_date→system_as_of/trade_date 分离。旧 `entry_rule`/`invalidation_rule` 是围绕"20 日均线"和百分比写的中文句子，直接搬会形成两套冲突口径（违反 AGENTS 规则 16）。
4. **产物层次不同。** 旧链一次产出"交易候选"；新契约里 `TrendResult` 只是趋势状态，Signal 契约（QNT-008，`doc/quant/模块总任务规划和进度.md:65`）仍是 NOT_STARTED。若把 CandidateSignal 塞进 Trend Payload，等于在 frozen Envelope 里私增字段。

因此本轮判定：**先书面结论，不补算法契约**。补契约所需的语义输入尚未存在，任何实现都会变成"把主观选择伪装成既定规则"。

## 6. 决议请求（阻塞 1.3 的 5 项）

每项给出可选项与本节推荐。全部决议后，才可开"Trend 算法 + `quant.trend` Stage"工作组。

### D1 周期集合

- 选项 A：仅 5d/20d（复刻旧逻辑最小集，零新增因子）
- 选项 B：5d/20d/60d（短/中/长，符合 `01-量化引擎总体设计` 第 36 行"按短/中/长周期计算"）
- 推荐 **B**，但须同时新增 `momentum_60d` 的 Decimal Registry Entry（当前只有 float64 函数与 Schema 枚举），否则 60d 无法用精确 Close 计算。

### D2 direction 判定

- 选项 A：纯符号——`ma20_distance>0` 且 `momentum_20d>0` → bullish；两者皆负 → bearish；符号不一致 → sideways；样本不足 → unknown
- 选项 B：符号 + 最小幅度阈值（需给出阈值来源）
- 推荐 **A**。它是对旧"三正合取"的最小语义扩展，不引入无出处的常数；代价是极靠近均线时也会给出方向，需在 `quality_status` 或 evidence 上表达"弱证据"。

### D3 phase 判定

- 选项 A：v1 恒为 `unknown`，phase 待独立决议
- 选项 B：按 `momentum_5d` 与 `momentum_20d` 的符号与相对大小映射（同号且 |m5|>|m20| → accelerating；同号且 |m5|<|m20| → mature/weakening；异号 → reversing；|m20| 很小 → emerging）
- 推荐 **A**。B 的映射看似"自然"，但 emerging/mature/weakening 的边界是业务判断，且需要"很小"的阈值；在无决议前写入 Golden 会造成返工。A 可让 v1 先交付方向与证据，phase 后期 additive 补齐。

### D4 strength

- 选项 A：v1 恒为 `null`（契约允许）
- 选项 B：给定归一化公式（如 `min(1, |ma20_distance| / s)`），需说明 `s` 的来源
- 推荐 **A**。DEC-049 只规定取值范围，没有给公式；`null` 是契约内的合法表达，且避免"任意尺度"污染结果。

### D5 产物层次

- 选项 A：先只做 `TrendResult`（趋势状态），交易候选（Signal）另立契约
- 选项 B：同时定义 Signal 契约（direction/entry/exit/invalidation/hold days/参考价），把旧 CandidateSignal 正式化
- 推荐 **A**。QNT-008 依赖稳定 Trend/Market/Industry/Stock；1.4 的 Risk 硬门禁也以 Signal 为输入。先把 Trend 状态做对，Signal 才有可信输入。旧 `CandidateSignal` 的自然语言规则字段不适合作为跨模块机器契约。

## 7. 若 D1–D5 全部按推荐项通过：可交付的最小范围

仅作为后续工作组的范围说明，**不是**本轮交付：

1. `internal/quant/trend` 增加纯函数 `Direction(ma20, m20) (Direction, error)`，无阈值、无加权，输入为 Decimal 结果而非 float64。
2. `TrendResult` 组装：`trend_code=close_trend_1.0.0`、`period=20d`、`strength=null`、`phase=unknown`、`evidence_metrics` = 三个 Factor Metric、`conflict_refs` = 空数组（单周期无冲突）、`quality_status` 由输入因子状态传播（任一 `insufficient_data` → `unknown`）。
3. Golden 覆盖：三正、三负、符号冲突、样本不足、输入 degraded 五类。
4. 之后才谈 Registry Entry、`quant.trend` Output/Quality Schema、Template、关联规格表条目与 Stage 装配。

## 8. 本轮结论与影响

- 1.3 的退出条件"有生产策略 Stage 与契约"**未满足**，本组只完成核实与书面结论，状态应记为"书面结论完成，待 5 项决议"，不得记为已完成。
- 不新增任何代码、Schema、Golden、Registry Entry 或 Stage；不改 QNT-CTR-003/004/005 状态（仍 `deferred`/`frozen`/`frozen`）。
- 旧 `trendFactor`/`trendStrategy` 保持原样，随阶段 4 删除旧链一并退役；**不**在 Quant 复制其 float64 实现，也不在 Researchflow 重写算法。
- 1.4（tradability/risk 硬门禁）不依赖本轮：Risk 规则集与 RuleSet 评估机制已存在，缺的是权威规则目录，与本结论无关。
- 2.1（完整股票研究 Template）不含 trend Stage，不受阻塞；但若要交付"趋势"页面能力，必须先完成 D1–D5。

## 9. D1–D5 决议通过（2026-11-26）与 1.3 落地记录

### 9.1 决议结论

| 决议 | 选项 | 结论 |
|---|---|---|
| D1 周期集合 | B | 5d / 20d / 60d；同时新增 `momentum_60d` 的 Decimal Registry Entry |
| D2 direction | A | 纯符号：`ma20_distance` 与 `momentum_20d` 同正 → `bullish`、同负 → `bearish`、异号 → `sideways`、样本不足 → `unknown`；不设幅度阈值 |
| D3 phase | A | v1 恒为 `unknown`，phase 的判定边界留待独立决议 |
| D4 strength | A | v1 恒为 `null`（契约允许），不引入无来源的归一化公式 |
| D5 产物层次 | A | v1 只产出 `TrendResult`；交易候选（Signal）仍属 QNT-008，保持 NOT_STARTED |

### 9.2 落地范围（与第 7 节的差异）

实际交付比第 7 节的最小范围多两处，均因 D1 的多周期要求而起，并已在实现中固化：

1. **证据是四个 Metric，不是三个**：`ma20_distance`、`momentum_5d`、`momentum_20d`、`momentum_60d`，顺序即契约（`TrendResult.evidence_metrics` 不随调用方变化）；`minimum_bars = 61` 来自最长窗口，而不是任取一个窗口。
2. **质量状态是三值而非布尔**：`valid`（四个证据皆可用）、`degraded`（方向可判定但长周期证据不足）、`unknown`（方向不可判定）；并与 `result_status` 双向锁定——`unknown ⇔ insufficient_data`，其余必为 `completed`。两种降级都必须真实落库并持 `result_id`，否则 Stage Output 与授权读取拿不到可引用的结果。

### 9.3 实现落点

- 计算：`internal/quant/factor/decimal_factor.go`（`DecimalMomentum60`）、`internal/quant/registry/production_factor.go`（`momentum_60d` Entry，61 根）。
- 语义：`internal/quant/trend/assess.go` 只做符号判定与质量分类，拒绝空值/非 ratio/样本不足等非规范证据；不实现 phase/strength 推导，不 import `registry`（避免测试期循环依赖，一致性由 `TestTrendEvidenceCodesMatchRegistry` 收敛）。
- 用例：`internal/quant/application/create_trend.go`（一次 Snapshot 读取、四证据、`ValidateMetric` 对齐 Entry、`result.NewTrendPayload` 落库、Manifest `quality_status` 取判定值以满足 QNT-CTR-005）。
- Stage：`internal/quant/stage/trend.go` + `trend_output.go`（自校验复算方向，拒绝“证据与结论打架”的 payload）、`internal/server/bootstrap/quant_trend_stage.go`（Output Registry 1.4.0、冻结 Template、`completed`→succeeded / `insufficient_data`→degraded + `quant_trend_insufficient_data`）。
- 装配：参数 Schema（无 `window` 字段）、`quant.trend.always` 适用性规则、Dispatcher 路由、Runtime 前置校验、关联规格表条目；Quant service Principal 注入泛化为“任一 Quant Stage”。
- 测试：trend 包纯符号与质量边界、8 类非规范证据拒绝、Facade 三类质量结果、5 类 Stage Golden（bullish/bearish/sideways/degraded/unknown）、文档 Schema 与运行时同判定、跨模块契约（Registry Entry ↔ 关联规格表 ↔ 真实 Stage Envelope）。

### 9.4 未改变的部分

- 契约状态：QNT-CTR-003 仍 `deferred`，QNT-CTR-004/005 仍 `frozen`；本组只是让实现追平已通过的决议，不构成对外发布授权。
- QNT-008 Signal 仍 NOT_STARTED；DEC-PLAT-002-049“禁止 direction/phase/strength 互推”继续生效——phase/strength 的 `unknown`/`null` 是**契约内的显式取值**，不是“由 direction 推出”。
- 旧 `trendFactor`/`trendStrategy` 未改一行，随阶段 4 删除旧链一并退役。
