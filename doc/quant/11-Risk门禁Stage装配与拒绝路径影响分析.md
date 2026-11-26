# Risk 门禁 Stage 装配与拒绝路径影响分析

- 状态：**已实施并验收**（四项决策已按推荐项确认，见 §8；实施结果与验收结论见 §9）
- 日期：2026-11-26
- 分支：`repair_all`，HEAD `295fe5aa0`（影响分析提交）
- 对应任务：执行清单 1.4 剩余部分 / QNT-010；前置条件 #45 已完成
- 相关文档：`doc/quant/09-risk-tradability硬门禁切片与前置.md`（8.5 物理链路）、`doc/quant/04-RiskDecision DTO.md`、`doc/quant/10-Signal契约最小切片.md`、`doc/contract/decisions/项目级契约决策记录.md`（DEC-PLAT-002-062）

## 0. 结论摘要

**可以做，但必须接受一个前提：装配完成后决策恒为 `reject`。** 本切片的验收标准是"拒绝可被完整追溯与读取"，不是"门禁放行"。任何把本切片描述为"硬门禁已通过"的说法都是错的。

交付范围：

1. **Risk Facade**（`internal/quant/application`）：读取已授权的交易事实快照 → 评估 `qnt-risk-tradability/1.0.0` → 产出 QNT-CTR-004 `RiskDecision` → 经既有 `resultPersister` 落库并返回内容寻址 Manifest。
2. **`quant.risk` Stage**（`internal/quant/stage` + `internal/server/bootstrap`）：参数 → 创建事实快照 → 调 Facade → 产出 Stage Output + Quality Summary；按 Registry Set 1.5.0 注册，接入 Dispatcher、模板、可信 Principal 注入、结果关联规格表。
3. **三份新 Schema + Golden**：`QNT-CTR-003-risk-stage-output-1.0.0`、`QNT-CTR-003-risk-quality-summary-1.0.0`、`quant-risk-parameters-1.0.0`。

明确不做：不注册 `hard_risk` 门禁类型、不让 Run 因 `reject` 失败、不生产 Signal、不投影到 Researchflow Composer 的 `RiskDecision`（RF-BLOCK-008 保持 OPEN）。

## 1. 现状核实（可验证的代码事实）

| 能力 | 状态 | 位置 |
|---|---|---|
| RuleSet/Evaluator/Aggregate/Decision | 已有，block 必 required、缺规则补 unknown、panic 转失败 | `internal/quant/risk/{rule_set,evaluator,aggregate,decision}.go` |
| 可交易性规则集与门禁 | 已有 `qnt-risk-tradability/1.0.0` + `tradability_unverified` | `internal/quant/risk/tradability.go:18-24` |
| 事实→门禁输入映射 | 已有 `AssessTradabilityFacts`，`price_limit`/`tradable_volume` 恒缺失 | `internal/quant/application/tradability_facts.go:151-201` |
| 决策构造入口 | 已有 `EvaluateTradabilityDecision(ctx, decisionID, evaluatedAt, snapshot)` | 同上 `:205-222` |
| `risk` Payload 与 Envelope | 已注册，`risk/1.0.0`，Manifest 侧允许 `result_type=risk`、`result_status=rejected` | `internal/quant/result/envelope.go:55-73`、`internal/quant/manifest/manifest.go:25-36,347-354` |
| Quant 结果存储 | 不限制 `result_type`，按 Manifest JSON 落库 | `internal/quant/storage/*`（无 result_type 白名单） |
| Data 交易事实链路 | 已通（#45）：快照创建、签发、授权读取、按 kind 分流 | `marketdata/bootstrap/snapshot_application.go:52-74` |
| Quant 侧 Data 适配器 | 已有 `quantTradingFactsAdapter`（创建 + 授权读取 + `Verify()`） | `internal/server/bootstrap/quant_trading_facts_adapter.go` |
| **Risk Facade** | **缺** | — |
| **`quant.risk` Stage Adapter / Output DTO / Validator** | **缺** | — |
| **Stage Output 契约（`quant.risk`）** | **缺** | `doc/quant/schema/` 只有 sma20/factor/trend |
| **参数 Schema / 适用性规则 / 模板 / Registry Entry** | **缺** | — |
| **Dispatcher 分支 / Principal 注入 / 关联规格表** | **缺** | 见 §5 风险 R3 |

三个最容易漏的接线点（历史上踩过同类）：

1. `isQuantStageCode`（`internal/server/bootstrap/researchflow_runtime.go:406-413`）只列了 sma20/factor/trend；不加 `quant.risk`，生产 Worker 里 Data 会以 `ErrDataAuthorizationDenied` 拒绝，而直接调 Adapter 的单测不会暴露。
2. `researchflowStageDispatcher`（`internal/server/bootstrap/researchflow_stage_dispatcher.go:14-52`）按 Stage Code 分派；不加分支则模板注册成功、执行期找不到执行器。
3. `quantStageAssociationSpecs`（`internal/server/bootstrap/quant_result_association.go:58-89`）是结果授权读取的白名单；不登记 `quant.risk`，决策落库了也读不回来（`GetResult` 只会返回"无关联"）。

## 2. 关键设计判断（已确认，D1–D4 均按推荐项通过）

### D1 拒绝表达在决策里，而不是 Stage 失败里

两种可选语义：

| | 语义 | 后果 |
|---|---|---|
| A（推荐） | Stage `succeeded`，Stage Output 里的 `decision_status=reject`，Manifest `result_status=rejected` | 决策落库、可授权读取、可关联；下游必须读决策才知道结论 |
| B | Stage `failed`，reason `risk_rejected` | Run 立即失败；失败 Attempt 不产出 Stage Output → **拒绝结论无法被授权读取，也无法追溯证据** |

推荐 A。理由：本切片的验收目标就是"可追溯的拒绝"，B 恰好把可追溯性丢掉。风险是 A 可能被误读成"放行"——由 §5 R1/R2 的口径约束与 `decision_status` 字段强制消费者读决策来补偿。

### D2 本轮不注册 `hard_risk` 门禁类型

模板框架已支持 `GateType="hard_risk"`：要求 Stage 必须在 `HardRiskGateRegistry` 注册、必须 `Required`、四类传播全部 fail-closed、模板 `RequiresHardRiskGate` 与已声明门禁数量一致（`internal/researchflow/template/template.go:139-165`）。但：

- 组合根当前传的是 `nil`（`researchflow_execution_composition.go:212`、`researchflow_composition.go:54`），本轮若声明 `hard_risk`，必须先实现注册表并改两个组合根签名；
- hard_risk 会进入 Run 终态判定（`run_finalizer.go`、`deadline_finalizer.go` 使用 `HardRiskStageCodes`），而当前决策**恒 reject**，等于让所有含该 Stage 的 Run 必然失败——用门禁摧毁整条流水线；
- 这属于"门禁语义与 Run 生命周期"的独立治理动作，应等涨跌停/可成交量事实补齐、决策可能出现 `pass` 之后再立项。

结论：本轮模板 `GateType` 留空、`RequiresHardRiskGate=false`，与 sma20/factor/trend 一致。

### D3 不要求 Signal 引用

Signal 目前只有 DTO 与 Schema（QNT-CTR-006 `deferred`），没有存储、没有产出链路。若参数里要求 `signal_ref`，只能信任调用方自述，等于把信任边界降级——与 `doc/quant/09` 第 6 节留白的理由相同。

技术上也无需改契约：QNT-CTR-004 `RiskDecision.input_refs` 只是字符串数组（`QNT-CTR-004-risk-decision-1.0.0.schema.json`），Signal 生产就绪后追加一个 signal 引用即可，不需要版本升级。因此本轮 `input_refs = [facts_snapshot_ref, authorization_ref]`。

### D4 `evaluated_at` 取 Run 的可信 `system_as_of`，不取墙钟

`Decision.Validate` 要求 `evaluated_at ≥ system_as_of`（`internal/quant/risk/decision.go:66-70`）。取墙钟会让同一份事实在重放时得到不同 `result_id`（决策内容含评估时刻），破坏内容寻址的可复现性；取 `system_as_of` 则：

- 与事实快照的 PIT 时刻完全对齐，决策可复现；
- 真实执行时刻由 Researchflow 的 Attempt 时间戳承载，信息不丢失；
- 满足"不早于 system_as_of"（相等合法）。

代价：`evaluated_at` 不再等于真实墙钟时间。这是一条口径权衡，需要确认。

### D5（无需确认，设计假设）参数与资源策略

- 参数：`symbol`、`trade_date`；`system_as_of` 来自 Run 可信时钟，不接受参数覆盖。
- Facade 校验 `trade_date ≤ system_as_of` 的日期部分；未来日期直接拒绝。
- 资源策略复用 `resourcepolicy.QuantDailyBarsProvidersRef`：事实快照由 Data 从其自身落库数据产出，与日线同源；新增独立策略引用会牵动 resourcepolicy 注册表与 Provider 清单，收益不足。

## 3. 变更清单

### 新增

| 文件 | 内容 |
|---|---|
| `internal/quant/application/create_risk.go` | `CreateRiskService`：校验请求 → 读授权事实快照 → `EvaluateTradabilityDecision` → `NewRiskPayload` → `resultPersister.persist`；构造 `manifest.Input`（`result_type=risk`、`scope=stock`、`trade_date`、`period=1d`、`quality_status` 由事实快照质量决定、`result_status=rejected/completed`、`algorithm_version`=规则集版本） |
| `internal/quant/application/create_risk_test.go` | 拒绝路径、证据引用、时间校验、质量映射、落库失败 |
| `internal/quant/stage/risk.go` | `RiskAdapter`：参数校验 → 调 Data 创建事实快照（`purpose=risk_review`）→ 调 Facade → 输出 DTO |
| `internal/quant/stage/risk_output.go` | `RiskStageCode/OutputType/SchemaVersion`、`RiskOutput`、`RiskQualitySummary`、严格 Validator |
| `internal/quant/stage/risk_output_test.go` | 输出契约拒绝测试 |
| `internal/server/bootstrap/quant_risk_stage.go` | 组合（Registry 1.5.0、模板、Executor、错误映射） |
| `internal/server/bootstrap/quant_risk_stage_test.go` | 6–8 个执行/模板/分派/版本唯一性测试 |
| `internal/server/bootstrap/quant_risk_end_to_end_test.go` | 端到端：真实 Data Store（内存）+ 真实 Quant 代码 → 断言 reject 可追溯 |
| `doc/quant/schema/QNT-CTR-003-risk-stage-output-1.0.0.schema.json` | Stage Output |
| `doc/quant/schema/QNT-CTR-003-risk-quality-summary-1.0.0.schema.json` | Quality Summary |
| `doc/researchflow/schema/quant-risk-parameters-1.0.0.schema.json` | 参数 Schema（strict、additionalProperties=false） |
| `doc/quant/golden/risk/*` | 上述两份 Schema 的 Golden |

### 修改

| 文件 | 改动 |
|---|---|
| `internal/server/bootstrap/researchflow_execution_composition.go` | 组合 risk Stage、Registry Set 追加 1.5.0、模板入列、Dispatcher 字段 |
| `internal/server/bootstrap/researchflow_stage_dispatcher.go` | 新增 `risk` 字段与 `quant.risk` 分支 |
| `internal/server/bootstrap/researchflow_runtime.go` | `isQuantStageCode` 增加 `quant.risk` |
| `internal/server/bootstrap/quant_result_association.go` | 规格表登记 `quant.risk`（含 ResultID 提取） |
| `internal/server/bootstrap/researchflow_template_prerequisites.go` | 新增 `quantRiskApplicabilityEvaluator`（`quant.risk.always`） |

### 不改

- QNT-CTR-004 的 `RiskDecision`/Envelope 字段（`risk`、`rejected` 都已在枚举内）；
- QNT-CTR-006 Signal、DATA-CTR-007/008；
- sma20/factor/trend 的 Output Schema 与 Golden；
- `internal/quant/risk/**` 的任何规则或聚合语义。

## 4. 契约影响

- **QNT-CTR-003 additive 扩展**：新增第三份 Stage Output 与第四份 Quality Summary。沿用 factor/trend 的先例（"L2 机器契约补充，不升级 QNT-CTR-003 状态"）。决策记录 **DEC-PLAT-002-062** 已登记（`accepted_by_project_owner`），并已同步 `契约版本与变更记录`。
- **Researchflow 参数 Schema**：新增 `quant-risk-parameters-1.0.0`，属 Stage 层参数契约，不改 CTR。
- **状态不变**：QNT-CTR-003 仍 `deferred`、QNT-CTR-004/005 仍 `frozen`、QNT-CTR-006 仍 `deferred`、DATA-CTR-007 仍 `deferred`、DATA-CTR-008 仍 `frozen`。
- **RF-BLOCK-008 保持 OPEN**：Researchflow Composer 的 `RiskDecision` 与 QNT-CTR-004 不 wire-compatible（`internal/researchflow/resultvalidation/risk.go:29`）。本切片产出的是 Quant `risk` Payload，**不做**到 Composer 的投影；该卡点需要 Quant + Researchflow 双 Owner 决议，不能用"Stage 已装配"冒充已解除。

## 5. 风险与失败模式

| 编号 | 风险 | 缓解 |
|---|---|---|
| R1 | 恒 `reject` 被读成"门禁通过"或"链路坏了" | 文档、测试名（`...ProducesTraceableRejection`）、Quality Summary 显式给出 `missing_facts` |
| R2 | Stage `succeeded` 被下游当成放行 | Stage Output 必带 `decision_status`；Quality Summary 带 `rule_set_version`；文档明确"消费者必须读决策，Stage 状态只表示评估完成" |
| R3 | 生产 Worker 缺 Quant 服务身份，单测不暴露 | 显式补 `isQuantStageCode` + 专门测试（注入缺失 → 拒绝执行） |
| R4 | 事实快照每次执行都会新建（授权不同） | 快照内容寻址可去重；决策 `input_refs` 含授权引用，审计可追；决策身份本就含评估时刻，不做"重放必得同一 result_id"的承诺 |
| R5 | `trade_date` 与 `system_as_of` 错位，读到未来事实 | Facade 强制 `trade_date ≤ system_as_of` 日期，非法即拒绝；事实快照自身按 PIT 时刻读取 |
| R6 | 事实质量 `degraded` 被升级为 `valid` | `quality_status` 直接投影事实快照质量；五项事实齐但快照降级 → Manifest 也 `degraded` |
| R7 | 输出 Schema 与实际 Validator 漂移 | 契约测试：Schema 文件 + Golden + 运行时 Validator 三方一致（沿用 factor/trend 测试模式） |
| R8 | 新增 Registry 版本（1.5.0）与既有冲突 | 版本唯一性测试（Registry Set 以 version 为键） |

## 6. 验收方案（"装配 + 拒绝路径"优先）

### 6.1 单测

`internal/quant/application/create_risk_test.go`：
1. 事实五项齐全但仍缺涨跌停/可成交量 → `reject`，`MissingFacts=[price_limit,tradable_volume]`，`EvidenceRefs` 含两个引用；
2. 事实快照 `degraded` → Manifest `quality_status=degraded`，决策仍 `reject`；
3. `evaluated_at` 早于 `system_as_of` → 拒绝（结构性校验）；
4. 快照身份/kind 被篡改 → 拒绝，不落库；
5. 落库失败 → `ErrResultPersistenceFailed`，不返回 Manifest 假成功。

`internal/quant/stage/risk_output_test.go`：Schema/Golden/Validator 三方一致 + 未知字段/非法枚举拒绝。

`internal/server/bootstrap/quant_risk_stage_test.go`：
6. Executor 产出 reject 决策输出（`decision_status=reject`、`missing_facts` 非空、`result_status=rejected`）；
7. 错误参数 / 错误资源策略 → `quant_risk_invalid_parameters`，且不调用 Adapter；
8. Data 授权拒绝 → `data_authorization_denied`（非重试）；
9. 模板冻结断言（参数 Schema、Registry 1.5.0、传播策略、`RequiresHardRiskGate=false`）；
10. 参数 Schema 严格性与不可变性（仿 factor/trend）；
11. Dispatcher 路由到 `quant.risk`；
12. Registry 版本不冲突 + 关联规格表登记。

`internal/server/bootstrap/quant_risk_end_to_end_test.go`：
13. 真实 Data 内存 Store + 真实 Quant 代码：创建 → 授权 → 评估 → 落库 → 以 `result_id` 读回风险结果，断言 `status=reject`、`reasons[0].code=tradability_unverified`、`evidence_refs` 与 `input_refs` 精确等于事实快照引用与授权引用。

### 6.2 变异验证（至少 5 处，恢复后必须全绿）

1. 去掉 `price_limit`/`tradable_volume` 的强制缺失 → 决策变 `pass`；
2. 把 `rejected` 改成 `completed` → Manifest 断言失败；
3. 去掉 `isQuantStageCode` 的 `quant.risk` → 身份测试失败；
4. Executor 把 Data 授权拒绝映射成可重试失败 → 错误映射测试失败；
5. 关联规格表漏登记 → 结果读回失败；
6. 参数 Schema 放开 `additionalProperties` → 严格性测试失败。

### 6.3 不做（不得冒充）

- 真实 MariaDB 集成（需 DSN，本轮不执行）；
- 真实 Provider 行情与真实交易状态数据准确性；
- CI 结论预填（以推送后为准）；
- `hard_risk` 门禁注册与 Run 终态因 reject 失败。

## 7. 非目标

- 不新增业务风险规则（集中度/流动性/波动率/行业暴露仍在规则目录里 pending）；
- 不补涨跌停制度表与可成交量模型（另行立项）；
- 不生产 Signal，不做 Strategy→Signal 评估；
- 不做 Researchflow Composer 的 RiskDecision 投影（RF-BLOCK-008）。

## 8. 决策确认结果

| 编号 | 决策 | 结论 |
|---|---|---|
| D1 | 拒绝表达在决策里（Stage succeeded）还是 Stage failed | **A：Stage `succeeded` + `decision_status=reject` + Manifest `result_status=rejected`** |
| D2 | 本轮是否声明 `hard_risk` 门禁 | **否**：`GateType` 留空、`RequiresHardRiskGate=false`，另立项 |
| D3 | 是否要求参数携带 Signal 引用 | **否**：`input_refs = [facts_snapshot_ref, authorization_ref]` |
| D4 | `evaluated_at` 取 `system_as_of` 还是墙钟 | **取 `system_as_of`**（可复现，真实执行时刻由 Attempt 承载） |

四项全部按推荐项执行，已写入 DEC-PLAT-002-062。

## 9. 实施结果与验收结论

### 9.1 实际交付

| 类别 | 文件 |
|---|---|
| Facade | `internal/quant/application/create_risk.go`、`create_risk_test.go`（6 测试） |
| Stage 输出契约 | `internal/quant/stage/risk_output.go`、`risk_output_test.go`（4 测试，含 14 个篡改变异子用例） |
| Stage Adapter | `internal/quant/stage/risk.go` |
| 组合根 | `internal/server/bootstrap/quant_risk_stage.go`、`quant_risk_stage_test.go`（9 测试） |
| 端到端 | `internal/server/bootstrap/quant_risk_end_to_end_test.go`（4 测试） |
| Schema | `doc/quant/schema/QNT-CTR-003-risk-stage-output-1.0.0.schema.json`、`QNT-CTR-003-risk-quality-summary-1.0.0.schema.json`、`doc/researchflow/schema/quant-risk-parameters-1.0.0.schema.json` |
| Golden | `doc/researchflow/golden/quant-risk-parameters-1.0.0.golden.json` |
| 四处接线 | `researchflow_runtime.go`（`isQuantStageCode`）、`researchflow_stage_dispatcher.go`、`quant_result_association.go`、`researchflow_template_prerequisites.go`；组合于 `researchflow_execution_composition.go` |

与 §3 计划的差异（均已收敛，非缩水）：

1. Stage Output / Quality Summary 的 Golden **未新增**——上一轮已确认 sma20/factor/trend 的同类 Golden 由契约测试内联 fixture 承担，`doc/quant/golden/` 下并无对应文件；本轮改为在 `quant_researchflow_contract_test.go` 中补 `quantRiskAssociationPayload` 等 fixture，与既有做法一致，避免制造"文档里有 Golden 但没人读"的空壳。参数 Schema 的 Golden 按既有约定落在 `doc/researchflow/golden/`。
2. 输出校验的 `evaluated_at == system_as_of`、`Evaluate` 的两清单非空约束、`EvidenceRefs == InputRefs`，由运行时 Validator 强制（JSON Schema 无法表达），已在两份 Schema 的 `description` 中显式声明分工，避免后续误判为"Schema 已覆盖"。

### 9.2 测试结果（本地，非 CI）

- 新增 23 个测试，全部 PASS；`go test ./internal/...` 全绿。
- `go test -race ./internal/quant/... ./internal/server/bootstrap/` 无告警。
- `gofmt -l ./internal ./cmd` 空、`go vet ./internal/...` 空。

关键断言：`decision_status=reject`、`missing_facts=[price_limit, tradable_volume]`、`evidence_refs` 精确等于 `[facts_snapshot_ref, authorization_ref]`、`evaluated_at == system_as_of`、`result_id` 可解到已落库记录且 `record.Validate()` 通过、两个 Validator 均通过。

### 9.3 变异验证（6 处，全部有效失败并已恢复）

| # | 变异 | 捕获者 |
|---|---|---|
| 1 | 去掉 `price_limit`/`tradable_volume` 的强制缺失 | 决策变 `status=pass`，Stage 测试失败 |
| 2 | `reject` 映射成 `completed` | Manifest `result_status=completed`，断言失败 |
| 3 | 去掉 `isQuantStageCode` 的 `quant.risk` | 身份注入测试失败 |
| 4 | 授权拒绝映射成 `quant_risk_execution_failed` | 错误映射断言失败 |
| 5 | 关联规格表漏登记 `quant.risk` | 原契约测试**未**捕获（遍历随表变短）→ 补 `TestQuantRiskStageIsRegisteredForResultAssociation` 显式断言后捕获 |
| 6 | 参数 Schema 放开 `additionalProperties` | `root additionalProperties must be false` |

变异 5 暴露了一个真实缺陷：按规格表遍历的契约测试无法发现"整条漏登记"。已用显式断言补齐，而非调低要求。

### 9.4 本轮**未**完成（不得冒充）

- `hard_risk` 门禁注册与 Run 终态因 `reject` 失败（D2 明确不做）；
- RF-BLOCK-008 仍 OPEN：Researchflow Composer 的 `RiskDecision` 投影；
- 真实 MariaDB 集成（需 DSN）与真实 Provider 交易状态数据准确性；
- 涨跌停制度表、可成交量模型——决策仍恒 `reject`；
- CI 结论（以推送后为准）。
