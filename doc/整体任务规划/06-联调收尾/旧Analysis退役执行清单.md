# 旧 Analysis 退役执行清单

更新时间：2026-11-26
执行分支：`repair_all`（集中开发，受根目录 `AGENTS.md` 约束）
主线目标：在保留真实研究能力的前提下，按依赖顺序退役旧 Analysis 生产链，不制造“已退役”假象。

## 0. 本文定位与权威来源

本文是旧 Analysis 退役主线的**唯一可勾选执行清单**，把原先分散在以下文档中的多套任务分解收敛为一份口径。事实判断以代码为准，历史文档按记录日期理解。

- 退役边界与逐阶段能力映射：`./WebUI旧Analysis到Researchflow迁移盘点.md` 第 8、10 节（尤其 10.1 当前边界、10.3 执行顺序、10.4 进度、10.6/10.7 可退役与阻塞判定）。
- 跨模块顺序与 CI 验收：`./RepairAll跨模块修复计划与进度.md`。
- 契约状态：以 `doc/contract/PLAT-002-项目级契约冻结登记簿.md` 第 42/44/51/77-79 行为准。DATA-CTR-005/007、QNT-CTR-003 继续 `deferred`。
- 执行纪律：根目录 `AGENTS.md` 十七条强制规则。

**契约口径（已收敛）**：首批 Close-only Factor 可复用现有已授权 DailyBars Close Series Snapshot（精确 Close、Manifest/Payload 双指纹、内容寻址 SnapshotRef、Migration V6、授权读取），不被通用 OHLC DATA-CTR-005/007 阻断；其生产门禁是 QNT-CTR-003 自身的 Factor Schema/Golden、Decimal Registry/精度策略、生产 Facade 与 Researchflow Stage 接线。不得把 float64 纯函数或 SMA20 单指标当作生产 Factor 已迁移。

## 1. 阶段依赖总览

```
0 盘点冻结（已完成）
   ↓
1 补领域生产能力（当前唯一卡点）
   ↓
2 结果闭环与授权
   ↓
3 WebUI 迁移
   ↓
4 删除与清表（一次性工作组）
   ↓
5 最终验收
```

阶段 1 未闭环前不得进入阶段 4。删除旧链会导致 WebUI 研究结果页失效，属于 `AGENTS.md` 规则 9/11 禁止的“为通过而清理”。

## 2. 阶段 0 · 盘点冻结（已完成）

| # | 任务 | 主责 | 状态 | 证据 |
|---|---|---|---|---|
| 0.1 | 纠正 Researchflow 职责边界：内部执行编排，不直接暴露 HTTP；Root Server 唯一对外 | Integration | 已完成 | 迁移盘点 10.4 |
| 0.2 | 刷新生产模板数量与旧链实际生产装配基线 | Integration | 已完成 | 迁移盘点 10.4、第 1/9 节 |
| 0.3 | 确认旧 Analysis 仍有 WebUI、Server Router、Bootstrap、Worker、Migration 实际引用 | Integration | 已完成 | 迁移盘点 10.1/10.4 |
| 0.4 | 完成旧 Runner 逐阶段能力映射，核对 Quant 因子/趋势/风险构件与生产 Stage 差异 | Integration | 已完成 | 迁移盘点 10.5/10.7 |

## 3. 阶段 1 · 补领域生产能力（当前卡点）

| # | 任务 | 主责 | 状态 | 退出条件 / 可验证证据 |
|---|---|---|---|---|
| 1.1 | Data 授权 Snapshot 读取链路核实：Port 已由 `quant.sma20` 完整注入 Worker 并真实消费 SnapshotRef | Data + Server | 已完成（代码证实 2026-11-26） | `quant_sma20_stage.go:63-84` 构造 `quantDataSnapshotAdapter` 注入 `CreateSMAService`；`researchflow_execution_composition.go:98-104,143-156` 装入 dispatcher/runtime；`quant_data_snapshot_adapter.go:119-120` 实现 Quant 侧 `CloseSeriesSnapshotReader`；旧 `AnalysisCollector` 是独立链、不被 Stage 使用。本项无需重复施工 |
| 1.2 | 三因子生产化：MADistance20 / Momentum5 / Momentum20 从 `internal/quant/factor` 的 float64 纯函数补齐 Factor Schema/Golden、Decimal Registry 与精度策略、生产 Facade、结果持久化 | Quant | 已完成（2026-11-26，①～⑧ 组；实现闭环，契约仍 `deferred` 待重新进入审批） | 三因子进入 Schema/Golden 与生产 Registry；通过已授权 Close Series Snapshot 取精确输入；结果可授权读取。首版仅 Close-only，不等通用 OHLC DTO。**已完成**：① big.Rat 精确计算层（`decimal_factor.go`，scale=12/half-even）+ 测试 + `doc/quant/06` 决策文档；② additive 修订 QNT-CTR-004/003 1.0.0 Schema（DEC-PLAT-002-057）、`DecimalQuantMetric` DTO、3 条 Decimal Registry Entry、算法级与结果级 Golden；③ 通用生产 Facade `CreateFactorService`（封闭分派表/Registry Entry 精度断言/复用 SMA 的 Snapshot 门禁/`CalculationContext` 显式校验/产出强类型 Metric + ManifestInput），并抽出 SMA/Factor 共用 Snapshot 校验；修正第②组遗留的 AlgorithmVersion 非 semver 缺陷（统一 `1.0.0`）。全套测试 factor/registry/application/quant 全包与 race PASS。**第④组已落地（2026-11-26）**：QNT-CTR-003 Factor Stage Output/Quality 1.0.0 Schema、强类型 DTO 与运行时 Validator；三因子 available/insufficient 正例、契约违规负例与 Draft 2020-12 实例校验全部 PASS。**第⑤组已落地（2026-11-26）**：经 DEC-058 授权对 frozen Envelope Schema additive 追加 factor 分支，强类型 FactorPayload 注册进 Envelope 四处白名单，CreateFactorService 复用 resultPersister 真实落库并返回内容寻址 result_id，新增 factor envelope Golden；落库幂等、失败不落库、Schema 匹配、Manifest 漂移与非法 metric 拒绝全部 PASS。**第⑥组已落地（2026-11-26）**：Factor Stage Provider 与 Server 装配——`internal/quant/stage/factor.go` 的 Stage Adapter（授权 Snapshot → Facade → 强类型 Output/Quality 并自校验）、`quant.factor` Output Registry（独立版本 1.3.0，避免 Registry Set 版本冲突）、`quant.factor` 冻结 Workflow Template、参数 Schema（`factor_code` 三值 enum + symbol/date/limit）、`quant.factor.always` 适用性规则与 Stage Executor、Dispatcher 路由与 Runtime 前置校验。`factor_code` 只作 Stage 层分派，不透传给 Facade（Facade 的 parameters 仍为严格 `{}`）；三因子最小样本下限在 Adapter 精确校验，Schema 只做粗筛。**第⑦组已落地（2026-11-26）**：Quant 结果关联校验从硬编码 `quant.sma20` 泛化为已支持 Quant Stage 的封闭规格表（`quant.sma20` + `quant.factor`），按 Run Stage 清单前置枚举候选，精确区分"未成功产出"（无关联）与"已成功但证据不一致或不可读"（fail-closed 上抛，不降级为无关联）；Factor 结果现可经 `GetResult` 授权读取，且授权读取路径改为任一 Quant Stage 存在即注入。**第⑧组已落地（2026-11-26）**：端到端质量证据——只把 MariaDB Store 换成内存 Store，用真实 Data 代码（Snapshot Creator 双指纹/内容寻址、Authorization Owner 签发与复核、Authorized Snapshot Reader）贯通生产 `quantDataSnapshotAdapter` → `FactorAdapter` → `CreateFactorService` → 结果 Repository，断言 QualitySummary 各字段逐项等于已持久化 Data Snapshot 的 coverage evidence（fixture 让 `minimum_required_bars` 与 `observed_bars` 不同，冒充即失败）、因子值由独立十进制计算校验、`result_id` 等于真实落库 Manifest 并可被 `quantResultAssociationVerifier` 按 Run/Stage 读到、insufficient 显式 null 仍落库且持有 result_id；跨模块契约测试——Output Registry Entry 与关联规格表 SchemaRef/Validator 一致、`stageoutput.Registry.Validate` 接受真实 Stage Envelope、文档化 Draft 2020-12 Schema 与运行时 Validator 判定一致（数值型 `value` 双侧拒绝）、模板 `OnDegraded=accept` 与 Executor degraded 映射一致、`Module` 实现 `quantDataSnapshotPort` 编译期断言。**剩余**：契约状态升级需按 PLAT-002 重新进入审批；生产门禁转入阶段 2.1 完整研究 Template 与 2.3 报告读取 DTO |
| 1.3 | trend-following 策略评估：正式策略契约 + 版本化计算 + 信号证据；旧实现仅作事实记录，不自动成为新规范 | Quant | 已完成（2026-11-26；D1–D5 按推荐项通过后落地，契约仍 `deferred`） | 核实结论：旧 Runner 的 trend 只有 `trendFactor` 三因子（percent/float64）+ `trendStrategy` 三正合取（只出 `long_candidate`），没有 direction 四值/phase/strength/conflict/多周期；因此差距是**语义契约缺口**而非代码搬迁。不补算法的理由：①旧逻辑不覆盖 `TrendResult` 字段；②DEC-049 禁止方向/阶段/强度互推、`doc/quant/05` 列出的 7 项语义至今未决；③单位 percent→ratio、时间 trade_date→system_as_of/trade_date 分离；④产物层次不同（趋势状态 ≠ 交易候选，QNT-008 Signal 仍 NOT_STARTED）。交付 `doc/quant/07-trend-following策略差距与契约结论.md`：代码级事实、10 维差距矩阵、5 项决议请求（D1 周期集合 5/20 或 5/20/60 + 需补 `momentum_60d` Decimal Entry；D2 direction 纯符号；D3 phase v1 恒 unknown；D4 strength 恒 null；D5 先只做 TrendResult）、决议通过后的最小可交付范围。本轮不新增代码/Schema/Golden/Registry Entry/Stage，不改任何契约状态；旧 `trendFactor`/`trendStrategy` 保持原样随阶段 4 删除。**决议后落地（2026-11-26）**：D1–D5 全部按推荐项通过（周期 5/20/60、direction 纯符号、phase 恒 `unknown`、strength 恒 `null`、只产出 TrendResult）。① 新增 `momentum_60d` 精确算法与 Registry Entry（61 根），它是趋势输入的最小样本与覆盖率来源；② `internal/quant/trend` 只做无阈值判定（`DirectionOf` + `assessQuality`），拒绝 8 类非规范证据，不实现 phase/strength 推导；③ `CreateTrendService` 一次授权 Snapshot 读取产出四个证据 Metric（MA20 距离、5d、20d、60d，顺序即契约），`quality_status=valid/degraded/unknown` 与 `result_status=completed/insufficient_data` 严格对应（`unknown ⇔ insufficient_data`），两种降级都真实落库并持 `result_id`；④ `quant.trend` Stage Adapter、Output/Quality Schema 1.0.0、Output Registry 1.4.0、冻结 Template、参数 Schema（symbol/start_date/end_date/limit≥61，无 `window` 字段）、`quant.trend.always` 适用性规则、Dispatcher 路由与关联规格表条目全部落地；⑤ 生产缺口修正：Quant service Principal 注入由“仅 `quant.sma20`”泛化为任一 Quant Stage，否则 Trend/Factor 在生产 Worker 会被 Data 拒绝（单测直调 Adapter 掩盖过该缺口）。契约状态未变：QNT-CTR-003 仍 `deferred`、004/005 仍 `frozen`；QNT-008 Signal 仍 NOT_STARTED（D5 明确先只做趋势状态） |
| 1.4 | tradability/risk 硬门禁：Quant 正式 RiskDecision 覆盖 unverified-tradability-block，不可被综合评分/LLM 覆盖 | Quant | 未开始 | 等价规则测试通过；缺证据继续 fail-closed；有正式规则装配来源 |
| 1.5 | 输入质量门禁：Data 质量事实 → Quant 适用性正式映射；历史回放缺 PIT 证据继续阻断 | Data + Quant | 已完成（2026-11-26） | 旧 corePass 规则不再充当全局门禁；质量与适用性由各 Owner 分别提供。**落地**：`internal/quant/application/quality_applicability.go` 定义 7 个封闭原因码（PIT 不可信/快照质量/日历质量/Data 覆盖率/样本低于下限/Registry 覆盖率/逐 bar 质量）与固定优先级，表外原因一律不猜测；`validateAuthorizedCloseSeriesSnapshot` 拆为“结构完整性”与“质量事实→适用性”两段，历史回放缺 PIT 证据继续阻断；`InputNotApplicableError` unwrap 到 `ErrCloseSeriesSnapshotQuality`，三个 Facade 经 `classifyCloseSeriesValidationError` 统一保留原因码（PIT/质量/覆盖率类失败改判为各自的 `ErrXxxSnapshotQuality`，Snapshot 自身不自洽仍是 `ErrInvalidXxxSnapshot`）；Stage Executor 把原因投影为 `quant_{sma20|factor|trend}_input_*`，不再退化成 `*_execution_failed`。变异验证：删除覆盖率分支、删除 PIT 映射、去掉原因码透传三处均使测试失败 |
| 1.6 | market breadth 指标决策：保留则由 Quant 实现并正名（请求证券横截面，非全市场），否则书面废弃 | Quant + Research | 已完成（2026-11-26；M1 决议：**书面废弃**） | 有书面结论；不再以“全市场广度”命名。**核实结论**：旧 `marketAnalyzer`（`internal/service/wiring.go:210-255`，`mvp-watchlist-market-v1`）的分母是 `universeResolver` 复制的 `req.Symbols`（自选股集合），指标实为 `watchlist_above_ma20_pct` + 65/35 无出处阈值映射的 `regime` 字符串；代码自身已在缺指数数据时声明“不能视为完整市场广度分析”。消费者只有旧链（`analysis/app/runner.go` 校验 regime 非空、旧报告页展示“市场状态”），Researchflow/Quant 新链无消费者，QNT-005 仍 NOT_STARTED。Data 已有真·市场广度 `MarketBreadthSnapshot`（`cn_a_share` 全集、四类家数精确对账到 `covered_securities`、部分覆盖必须 degraded、最旧成分报价时间做新鲜度证据、`GET /api/v1/market/breadth`），但**没有授权快照路径**（授权快照只有单证券 `daily_bars_close_series`），因此 Quant 现在拿不到可追溯广度事实。判定与选项（A 保留为正名为 `cross_section_participation_ma20`，A1 N 次单证券快照破坏 PIT 一致性、A2 需新增横截面 payload 属 DATA-CTR 变更；B 书面废弃）见 `doc/quant/08-market-breadth指标决策与正名.md`；本轮只交付结论与命名规范，不新增代码/Schema/Stage/契约，不改 QNT-005 与各契约状态。**M1 决议（2026-11-26）**：按选项 B 书面废弃——不迁移、不正名、不注册为 Quant 指标，也不新建“自选股广度”替代物；`internal/service/wiring.go` 的 `marketAnalyzer` 加退役注释（仅注释），随阶段 4 删除旧链一并退役；真实市场环境需求归 QNT-005（前置项见 `doc/quant/08` 第 8 节）；第 6 节命名规范与第 7 节复活条件继续生效 |
| 1.7 | 占位能力直接退役：零现金账本、全拒 Planner、no-fill Execution 不迁移 | Quant | 未开始 | 依据迁移盘点 10.6，无真实业务能力，随旧链删除 |

阶段 1 内部顺序：1.1 经代码核实已完成（授权 Snapshot 读取链路已由 `quant.sma20` 注入 Worker，无需重复施工）；1.2 是主线起手；1.3/1.4 依赖 1.2 的 Factor 结果契约；1.7 为占位退役，随阶段 4 一并删除。

**1.3 结论（2026-11-26）**：1.2 的 Factor 结果契约已闭环，因此 1.3 的输入侧不再是阻塞点；真实阻塞是趋势语义决议（`doc/quant/07` D1–D5）。在决议落地前，1.3 不重复施工、不注册 `quant.trend` Stage；1.4 的 Risk 硬门禁与本决议无关，可并行推进。

**1.6 结论与决议（2026-11-26）**：旧指标是“请求证券集合内的参与度”，不是市场广度；保留的两条实现路径都要新增契约或破坏 PIT 一致性，且当前零消费者。**M1 决议：按推荐项书面废弃**，真实市场环境需求归入 QNT-005，其前置项精确化为“Data 为 `MarketBreadthSnapshot` 增加授权快照 payload + 交易状态/排除规则事实 + 指数授权快照”。命名规范（`breadth` 只指 Data 的全市场口径，横截面比例必须 `cross_section_*`/`*_participation` 且显式写分母）在废弃后继续生效；复活条件为“真实调用方 + 横截面授权快照 + 正名与独立算法版本”三条同时满足。

**1.3 决议通过（2026-11-26）**：D1–D5 按推荐项通过（D1=5/20/60、D2=纯符号、D3=phase 恒 `unknown`、D4=strength 恒 `null`、D5=只产出 TrendResult），1.3 因此由“书面结论”转为落地并已完成。落地范围与证据见上表 1.3 行与 `doc/quant/07` 第 9 节；契约状态仍为 `deferred`，重新进入审批属独立动作，不在本组范围内。

**1.1 缺口修正说明（2026-11-26）**：执行前核查推翻了清单初稿的前提——Data 授权 Snapshot Reader 已存在且已通过 `quant.sma20` 注入 Researchflow Worker，不是"尚未注入"。真正未闭环的不是 Port 注入，而是：① 除 `quant.sma20` 外没有消费 Close Series Snapshot 的股票研究 Stage/Template（归 2.1）；② 缺少把 SnapshotRef/Manifest/AuthorizationRef 绑定为 Researchflow 正式公开输入 DTO 的契约（迁移盘点 195 行）。因此阶段 1 的真实起手应直接落在 1.2 三因子生产化，不重复建设 Snapshot 注入链。

**1.2 地基工作组进度（2026-11-26）**：精度路线经决策采用"Decimal + big.Rat / 固定 scale=12 / half-even"，而非照抄 SMA 的 exact（比率因子通常是无限循环小数，exact 会拒绝合法输入），也不是沿用现有 float64（违背精确 Snapshot 输入初衷）。决策依据与边界见 `doc/quant/06-因子精确计算精度策略决策.md`。本工作组只交付精确计算层 + 测试 + 决策文档，已通过针对性测试；生产接入与契约变更属后续工作组，QNT-CTR-003 仍为 `deferred`。按每组工作边界，本组到此提交并汇报，不连续推进剩余层。

**1.2 第③组进度（2026-11-26）**：契约审查确认状态准确（004=frozen/003=deferred）但描述过时，已回填登记簿/版本变更记录/字段级契约（提交 `87e27f28e`）。第③组交付通用生产 Facade `internal/quant/application/create_factor.go`：一个 `CreateFactorService` 用封闭分派表覆盖三因子（新增因子只加 case，不需复制用例），精度从活动 Registry Entry 读取并断言（scale/rounding/tolerance/value_type/sample_count 漂移即拒），Snapshot 身份/PIT/质量/覆盖率门禁抽出为 SMA/Factor 共用实现（`close_series_snapshot_validation.go`，避免重复建设），Decimal 因子显式调用 `CalculationContext.Validate` 补上 `factor/context.go` 只覆盖 float64 的缺口。产出强类型 `DecimalQuantMetric` + 可持久化 `ManifestInput`，但**不落库**（Factor Payload 属第⑤组）。执行中发现并修正第②组遗留缺陷：Registry `AlgorithmVersion` 原用 `*-decimal-half-even-v1` 非 semver，与 QNT-CTR-004 `calculation_version` 的 semver pattern、结果级 Golden（`1.0.0`）、DTO 校验冲突，导致 DTO→ValidateMetric 消费链无法通过；统一为 `1.0.0` 后全链打通。Facade 层不建立 Run 关联、不签发授权；QNT-CTR-003 仍 `deferred`，不代表可生产发布。

**1.2 第④组进度（2026-11-26）**：契约复核确认 QNT-CTR-003 仍 `deferred`、QNT-CTR-004/005 仍 `frozen`，DEC-PLAT-002-057 只授权 Factor 结果表示的 additive 修订，不授权状态升级。新增 `QNT-CTR-003-factor-stage-output-1.0.0.schema.json` 与 `QNT-CTR-003-factor-quality-summary-1.0.0.schema.json`，并在 `internal/quant/stage/factor_output.go` 提供强类型 DTO 和严格运行时 Validator。Output 以 `result_id` 绑定 QNT-CTR-005 Manifest、以十进制字符串承载 value、按 metric_code 固定 period 与算法有效样本上限；Quality Summary 的 `minimum_required_bars` 是 Data 覆盖率门槛而非因子算法窗口，跨字段大小关系由 Go Validator 补充。Quality Summary 保留 `metric_code` 以绑定同一次 Factor 计算；不把 result_id 推导为本地伪造值。因第③组 Facade 尚未落库、没有可信 result_id，本组只交付 Schema/DTO/Validator 与机器验证，不提前实现 `factor_stage.go`；Stage Provider、Persistence Payload、Run 关联、Server 装配和真实质量证据仍是后续工作组。QNT-CTR-003 仍 `deferred`。

**1.2 第⑤组进度（2026-11-26）**：审查确认 PLAT-002 登记簿要求"factor 结果须另经治理决策后方可注册进 Envelope 白名单"，据此新增 DEC-PLAT-002-058 授权对 frozen Envelope Schema 做 additive 修订。改动：① `QNT-CTR-004-quant-result-envelope-1.0.0.schema.json` 的 `result_type` enum、`oneOf`、`$defs` 追加 factor/严格 factor_payload（十进制字符串/null、code→period、样本边界），既有 trend/risk/indicator 与其 Golden 全部保持合法；② `internal/quant/result/factor_payload.go` 新增强类型 FactorPayload 并注册进 `DecodePayload`/`supportedPayloadValue`/`Envelope.Validate`/`BuildRecord` 四处白名单，保留 DisallowUnknownFields、重复键/UTF-8 检测与 Manifest 逐字段绑定；③ `factor.DecimalQuantMetric.Validate()` 作为构造与反序列化的单一校验真相源；④ `CreateFactorService` 复用既有 `resultPersister`/`result.Repository` 真实落库并返回内容寻址 Manifest（result_id），不再只产出 ManifestInput；⑤ 新增 `quant-result-envelope-factor-1.0.0.golden.json`。测试覆盖落库幂等、失败不落库、白名单版本拒绝、Envelope 往返/漂移、非法 metric、null sample_count、深拷贝与 Schema 匹配，`go test ./internal/quant/...` 全包 PASS。本组不建 Run 关联、不装配 Stage Provider/Server；`factor_stage.go` 现已具备可信 result_id 前提，属下一组。QNT-CTR-004 仍 `frozen`、QNT-CTR-003 仍 `deferred`。CI（`0b8e42ed0`）：Go Test Suite [https://github.com/Kasign/stock_data_provider/actions/runs/45771814359](https://github.com/Kasign/stock_data_provider/actions/runs/45771814359)、Integration CI [https://github.com/Kasign/stock_data_provider/actions/runs/45771814387](https://github.com/Kasign/stock_data_provider/actions/runs/45771814387)、Quant module validation [https://github.com/Kasign/stock_data_provider/actions/runs/45771814360](https://github.com/Kasign/stock_data_provider/actions/runs/45771814360)、Required checks [https://github.com/Kasign/stock_data_provider/actions/runs/45771814376](https://github.com/Kasign/stock_data_provider/actions/runs/45771814376) 全部 success。

**1.2 第⑥组进度（2026-11-26）**：交付 Factor Stage Provider 与 Server 装配，不改任何契约版本。① `internal/quant/stage/factor.go` 新增 `FactorAdapter`：创建授权 Data Snapshot（复用 `quantDataSnapshotAdapter` 与 `QuantPurposeCode`）→ 调用 `CreateFactorService` → 把已落库 Manifest/Metric 投影为强类型 `FactorOutput`/`FactorQualitySummary` 并自校验；`factor_code` 只用于 Stage 层封闭分派，**不透传**给 Facade（Facade 的 parameters 契约仍为严格 `{}`），逐因子最小样本下限在 Adapter 先校验，避免向 Data 发起必然不足样本的请求。② `internal/server/bootstrap/quant_factor_stage.go` 新增 `quant.factor` 组合：Output Registry（`QNT-CTR-003-factor-stage-output/quality-summary-1.0.0`）、冻结 Workflow Template（参数 Schema、ResourcePolicy、Retry、Propagation）、Stage Executor（`available`→succeeded、`insufficient_data`→degraded、Data 授权/参数/执行错误按固定 reason code 失败）。③ `doc/researchflow/schema/quant-factor-parameters-1.0.0.schema.json` + Golden：`factor_code` 三值 enum、`limit` 下限 6（三因子最小算法窗口）。④ 组合根/Dispatcher/Runtime 接入：Output Registry 版本 `1.3.0`（Registry Set 以 version 为键，必须与 News 1.0.0/1.2.0、SMA 1.1.0 区分）；Quant Provider inventory 由 SMA/Factor 合并去重；Runtime 对任一 Quant Stage 都要求 `data:read` 的 Quant service principal。⑤ 测试：Adapter 三因子正例、insufficient 显式 null、非法请求不触达 Data、Schema 文档一致性与 Golden、Executor 状态映射、Template 冻结断言、Dispatcher 路由与 Registry 版本唯一性，`go test ./internal/quant/... ./internal/server/...` 全包 PASS。**未完成**：Factor Run↔Result 关联仍硬编码 `quant.sma20`，Factor 结果暂不能被 GetResult 授权读取；也未取得端到端真实质量输入证据与 Quant→Researchflow Contract Test。QNT-CTR-003 仍 `deferred`，不代表 Factor 可对外生产发布。CI（`d574976ef`）：Go Test Suite [https://github.com/Kasign/stock_data_provider/actions/runs/45773733316](https://github.com/Kasign/stock_data_provider/actions/runs/45773733316)、Integration CI [https://github.com/Kasign/stock_data_provider/actions/runs/45773733461](https://github.com/Kasign/stock_data_provider/actions/runs/45773733461)、Quant module validation [https://github.com/Kasign/stock_data_provider/actions/runs/45773733036](https://github.com/Kasign/stock_data_provider/actions/runs/45773733036)、Required checks [https://github.com/Kasign/stock_data_provider/actions/runs/45773732918](https://github.com/Kasign/stock_data_provider/actions/runs/45773732918) 全部 success。

**1.2 第②组进度（2026-11-26）**：经用户明确授权"就地改 004"并选"完整闭环"，对 frozen 的 QNT-CTR-004 1.0.0 与 deferred 的 QNT-CTR-003 1.0.0 做 additive 修订以承载比率因子的 decimal_string 表示（决策留痕 `DEC-PLAT-002-057`）。修订只放宽 enum/type、追加条件分支，不删除或收紧任何现有约束；现有 3 个 float64 因子与其 6 个结果级 Golden 全部保持合法。交付：`DecimalQuantMetric` DTO（value 为 JSON string、样本不足为 null）、`factorPeriod` 扩展、3 条 Decimal Registry Entry（decimal_string/scale=12/half_even/tolerance 末位/RequiresSnapshot）、算法级与结果级 Decimal Golden、全套测试（factor/registry/quant 全包 PASS、race PASS）。**契约状态未变**：QNT-CTR-004 仍 `frozen`、QNT-CTR-003 仍 `deferred`；本组不代表 Factor 已可生产发布，生产 Facade/Stage/持久化/Run 关联与端到端证据仍待后续工作组。

## 4. 阶段 2 · 结果闭环与授权

| # | 任务 | 主责 | 状态 | 退出条件 / 可验证证据 |
|---|---|---|---|---|
| 2.1 | 完整股票研究 Template / Stage / Registry + Root Server Worker 装配（当前仅 `quant.sma20` 单指标切片） | Researchflow + Server | 未开始 | 真实 Run 能跑完并持久化领域结果；SRV-CTR-001 Run Create/Get/Cancel 可用 |
| 2.2 | Run ↔ Quant Result / Research Evidence 强类型关联 + 生产授权读取（`resultvalidation/quant.go` 当前零生产调用） | Quant + Researchflow | 未开始 | 同 Run 关联可读；跨 Run、无关联、伪造 RunRef、撤销竞态均 fail-closed |
| 2.3 | Server 报告读取 DTO：鉴权后调用领域 Owner 授权读取；Run/Stage 聚合不冒充完整报告，不直接访问 Repository | Server | 未开始 | WebUI 有正式结果读取路径；UI 不访问模块 Repository |
| 2.4 | FLOW-005/006 上下文一致性与风险语义：可信 CallerRef、SystemAsOf、Deadline 贯穿读取；RiskDecision 唯一来源统一 | Researchflow + Quant | 未开始 | 无第二份无来源绑定的 RiskDecision；block/unknown/缺失/撤销 fail-closed |

## 5. 阶段 3 · WebUI 迁移

| # | 任务 | 主责 | 状态 | 退出条件 / 可验证证据 |
|---|---|---|---|---|
| 3.1 | 任务创建/幂等重放/轮询/取消/历史/详情/报告导出改接 Run Create/Get/Cancel 契约 | WebUI | 未开始 | 页面主流程不再调用旧 Analysis API；迁移盘点 3.1/3.2 的调用点归零 |
| 3.2 | 清除 Task ID / Event Cursor / Analysis Base / 旧 RunResult 依赖 | WebUI | 未开始 | `web/src/api.js` 等无旧引用；执行详情以真实 Run/Stage 状态为准，不伪造事件流 |

## 6. 阶段 4 · 删除与清表（一次性工作组，全部前置满足后执行）

| # | 任务 | 主责 | 状态 | 退出条件 / 可验证证据 |
|---|---|---|---|---|
| 4.1 | 删 Root Analysis 生产链：Runner / TaskQueue / Worker / HTTP API / Collector / 旧路由 / 配置 / 测试夹具；保留通用认证、Health/Ready 与 `cmd/analysis-api` 启动入口 | Server | 未开始 | 全仓无生产调用方证据；迁移盘点 8.4 第 1 项 |
| 4.2 | 删独立 go-core + Python Bridge + Python API Client + 相关 CI/环境变量/容器配置 | Integration | 未开始 | 确认无外部调用方；迁移盘点 8.4 第 3–6 项 |
| 4.3 | 停服窗口清表 `analysis_runs` / `analysis_stage_events` / `analysis_snapshots` / `analysis_tasks` 与 `namespace='analysis'` 迁移记录；旧数据不迁移不兼容 | Integration | 未开始 | 表与 namespace 均不存在；外键/清理顺序确认 |
| 4.4 | 文档清理：删旧服务部署/API/配置/排障说明，保留一份最终迁移记录（删除范围、无调用方证据、CI 验收） | Integration | 未开始 | 迁移盘点 8.4 第 7 项 |

## 7. 阶段 5 · 最终验收

| # | 任务 | 主责 | 状态 | 退出条件 / 可验证证据 |
|---|---|---|---|---|
| 5.1 | 全仓旧路径/环境变量/容器名/工作流引用扫描归零 | Integration | 未开始 | 迁移盘点 8.5 与 10.3 第 8 步 |
| 5.2 | Researchflow Run 全流程 + 授权报告读取 + WebUI 无旧请求 + Owner Session / Health / Ready 端到端 | Integration | 未开始 | 真实流程验收通过 |
| 5.3 | 最终 SHA 完整 CI + 浏览器验收 + 部署冒烟 | Integration | 未开始 | `required-checks` 聚合检查全项通过；不以中途 CI 状态作为停工条件 |

## 8. 执行口径

1. 每组工作开始前核对分支与 HEAD，读取涉及模块文档/卡点/契约；先证实问题再改，控制在一个逻辑问题及其必要上下游。
2. 先补或确认能暴露问题的测试，再改实现；不删断言、不放宽校验、不跳过失败、不造数据。
3. 每组完成后更新对应模块文档、卡点和本清单状态，执行针对性测试；按中文 Conventional Commits 提交，提交前后核对 diff 与 SHA。
4. CI 状态在无对应 SHA 结论时记 `not_run`，不得标记完整验收。按约定不等待中途 CI，迁移与清理完成后统一检查。
5. 不切分支、不合并其他分支、不 rebase、不强推、不重置历史。
6. 阶段 1 未闭环时不删除旧 Runner 的因子/策略/风险路径，不以 SMA20 单 Stage 代替完整研究流程。
