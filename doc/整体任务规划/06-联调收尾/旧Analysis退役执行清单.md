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
| 1.1 | Data Snapshot 授权读取 Port 注入 Researchflow Worker，股票研究 Stage 消费已授权 Close Series Snapshot | Data + Server | 未开始 | Worker 组合根有真实 Data Snapshot 授权读取 Port；Stage 读取 SnapshotRef 而非旧 AnalysisCollector；无旧 Collector 调用 |
| 1.2 | 三因子生产化：MADistance20 / Momentum5 / Momentum20 从 `internal/quant/factor` 的 float64 纯函数补齐 QNT-CTR-004 Factor Schema/Golden、Decimal Registry 与精度策略、生产 Facade、结果持久化 | Quant | 未开始 | 三因子进入冻结 Schema/Golden 与生产 Registry；通过已授权 Close Series Snapshot 取精确输入；结果可授权读取。首版仅 Close-only，不等通用 OHLC DTO |
| 1.3 | trend-following 策略评估：正式策略契约 + 版本化计算 + 信号证据；旧实现仅作事实记录，不自动成为新规范 | Quant | 未开始 | 有生产策略 Stage 与契约；不在 Researchflow 重写算法 |
| 1.4 | tradability/risk 硬门禁：Quant 正式 RiskDecision 覆盖 unverified-tradability-block，不可被综合评分/LLM 覆盖 | Quant | 未开始 | 等价规则测试通过；缺证据继续 fail-closed；有正式规则装配来源 |
| 1.5 | 输入质量门禁：Data 质量事实 → Quant 适用性正式映射；历史回放缺 PIT 证据继续阻断 | Data + Quant | 未开始 | 旧 corePass 规则不再充当全局门禁；质量与适用性由各 Owner 分别提供 |
| 1.6 | market breadth 指标决策：保留则由 Quant 实现并正名（请求证券横截面，非全市场），否则书面废弃 | Quant + Research | 未开始 | 有书面结论；不再以“全市场广度”命名 |
| 1.7 | 占位能力直接退役：零现金账本、全拒 Planner、no-fill Execution 不迁移 | Quant | 未开始 | 依据迁移盘点 10.6，无真实业务能力，随旧链删除 |

阶段 1 内部顺序：1.1 与 1.7 不依赖契约冻结，可先行；1.2 是主线起手；1.3/1.4 依赖 1.2 的 Factor 结果契约。

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
