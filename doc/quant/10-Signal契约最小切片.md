# 10 · Signal 契约最小切片（QNT-CTR-006）

- 状态：**契约 1.0.0 已建立（2026-11-26，`deferred`）**；只完成 Schema/Golden/强类型 DTO 与一致性测试，未新增 Stage、未落库、未建立 Run 关联。
- 范围：为 1.4 的 Risk 评估提供"可机读的交易候选"输入对象，替掉旧链自然语言 `CandidateSignal` 作为机器契约的用法。
- 关联：`旧Analysis退役执行清单.md` 1.4、`doc/quant/09-risk-tradability硬门禁切片与前置.md`、`doc/quant/07` 第 89 行（自然语言规则不适合作为跨模块契约）、DEC-PLAT-002-059、登记簿 QNT-CTR-006。

## 1. 为什么需要这个契约

1.4 的 `RiskDecision` 评估的是"某个待执行的候选交易"：没有候选，Risk 只能对空集合做评估，Facade/Stage 都是零调用方接口。旧链把这个对象叫 `CandidateSignal`（`internal/analysis/domain/types.go:320-335`），字段包含：

```
id / symbol / strategy_id / strategy_version / direction / signal_at / valid_until /
entry_rule / exit_rule / invalidation_rule / expected_hold_days /
reference_price(float64) / evidence_refs / attributes
```

其中 `entry_rule`/`exit_rule`/`invalidation_rule` 是**自然语言**，`reference_price` 是 float64。`doc/quant/07` 已判定：自然语言字段不适合作为跨模块机器契约，float64 价格也不满足精度要求。因此不能整体搬迁。

## 2. 决议（DEC-PLAT-002-059）

项目负责人在三个口径中选择**最小信号**：

| 字段 | 类型 | 说明 |
|---|---|---|
| `signal_id` | string | 不透明稳定标识；非空、无首尾空白、有效 UTF-8 |
| `symbol` | string | 证券代码；不做规范化（规范化属 Data 入口边界） |
| `direction` | enum `long`/`close` | v1 不含 `short`：融券规则未进入本契约 |
| `status` | enum `candidate`/`invalidated` | 生命周期状态；失效信号仍保留以便审计 |
| `strategy_ref` | object `{id, version}` | 产出该信号的策略身份与版本 |
| `system_as_of` | string | RFC3339Nano UTC `Z`；拒绝偏移时区与尾部冗余零 |
| `trade_date` | string | `YYYY-MM-DD`；Schema 校验形状，Go 补充真实日历日期 |
| `evidence_refs` | array\<string\> | 非空、去重、每项非空；指向已授权 Quant 结果（如 TrendResult result_id） |
| `strategy_rule_set_version` | string | 产出该信号的策略规则集版本 |

### 2.1 刻意不做的三件事

1. **不含价格**：`reference_price` 不进 Signal。价格口径要绑定复权基准与 PIT，塞进 Signal 会造成"同一信号在不同 Run 有不同价格却无法审计"。Risk/Planner 需要价格时走授权 Snapshot。
2. **不含自然语言规则**：`entry_rule`/`exit_rule`/`invalidation_rule` 不迁移。机器契约只表达可校验事实，策略解释属于报告层。
3. **不让方向与状态互推**：`status` 不得由 `direction` 推出，`direction` 也不得由 `status` 推出；`TrendResult.direction` 更不能自动投射为 Signal——Trend 只是信号证据之一，信号是策略层决策。

## 3. 与旧链字段的映射

| 旧 `CandidateSignal` | 新 `Signal` | 处理 |
|---|---|---|
| `id` | `signal_id` | 重命名，语义不变（仍是不透明标识） |
| `symbol` | `symbol` | 保留 |
| `strategy_id` + `strategy_version` | `strategy_ref{id,version}` | 合并为结构化引用，避免两个平行字符串 |
| `direction` | `direction`（`long`/`close`） | 旧链只有 `long_candidate`；`close` 为新枚举值，不表示旧行为 |
| `signal_at` | `system_as_of` | 语义从"信号时间"改为"系统观察时点"，符合 GOV-CTR-002 |
| — | `trade_date` | 新增：信号所属交易日与观察时点分离 |
| — | `status` | 新增：候选/失效，替代旧链隐式状态 |
| `entry/exit/invalidation_rule` | — | **不迁移**（自然语言不适合机器契约） |
| `expected_hold_days` | — | 不迁移（属 Planner 语义，不是信号事实） |
| `reference_price` | — | 不迁移（价格另行授权读取，避免复权口径歧义） |
| `evidence_refs` | `evidence_refs` | 保留并收紧：必须非空、去重 |
| `attributes` | — | 不迁移（开放 map 会破坏 closed 契约） |
| — | `strategy_rule_set_version` | 新增：策略规则集版本，与 Risk 的 `rule_set_version` 分层 |

## 4. 实现落点

- `internal/quant/signal/signal.go`：强类型 DTO、`Validate`、`NewSignal`（复制切片，避免别名）。
- `doc/quant/schema/QNT-CTR-006-signal-1.0.0.schema.json`：Draft 2020-12，`additionalProperties: false`，全部字段 required。
- `doc/quant/golden/signal/signal-1.0.0.golden.json`：long + candidate 正例。
- 测试（`signal_test.go`）：Golden↔Schema↔DTO 三方一致；14 类结构违规必须**同时**被 Schema 与 DTO 拒绝（防止两套判定漂移）；额外字段只由 Schema 拒绝；`2026-02-30` 这类"形状合法但日历不存在"的日期由 Go 拒绝（Schema 表达能力边界，非漂移）。

## 5. 验证

- `go test ./internal/quant/signal/` 全绿。
- 一致性策略：结构违规用表驱动同时断言 Schema 与 DTO；Schema 独有约束（未知字段）与 Go 独有约束（真实日历）分别单独覆盖，并把差异写进测试注释。

## 6. 后续（尚未完成）

1. **规则目录**（`doc/quant/09` 前置③）：Signal 有了，但 Risk 仍只有一条 `tradability_unverified` 规则；其余规则需权威来源。
2. **Data 交易状态事实授权快照**（前置②）：Signal 的 `evidence_refs` 可解析，但 tradability 门禁仍拿不到事实。
3. **装配**：Signal → RiskDecision 的 Facade/Stage 尚未建立；必须等 1、2 就位，否则是"对空集合评估"或"恒定 reject 的 Stage"。
4. **契约状态**：QNT-CTR-006 为 `deferred`，升级须按登记簿第 3 节启动定点审查并另行批准。
