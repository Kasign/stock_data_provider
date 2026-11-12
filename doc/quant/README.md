# Quant 模块

- 分支：`feat/quant`
- 代码范围：`internal/quant/**`
- 文档范围：`doc/quant/**`
- 任务前缀：QNT

## 当前状态

当前验收 HEAD：`4ce2fd155c21e7f8fe850c6f7f8c8d105b3dd47e`（2026-11-12）。

已具备首批 Indicator/Factor 纯计算、CalculationContext、基础 Registry、RiskDecision/RuleSet 执行框架、TrendResult DTO、Manifest/ResultRef、Repository Save Port/MemoryRepository，以及 QuantResult Envelope 初版。

本轮完成：
- DEC-PLAT-002-048：Manifest 绑定结果资源语义字段，全部进入 JCS 指纹和 ResultID。
- Ref 投影收敛：业务字段从已验证 Manifest 派生；当前 authorization_ref 与记录 created_at 独立提供。
- DEC-PLAT-002-049：Risk RuleSet、Reason、RiskDecision 聚合及 TrendResult evidence Metric 时间一致性校验。
- Envelope 1.0.0：从 Manifest 投影元数据；payload 按 `(result_type, payload_schema_version)` 显式解码；未知组合 fail closed。
- 当前仅注册 `trend/1.0.0`、`risk/1.0.0` 两种强类型 Payload；Trend/Risk 与 Envelope 的重复字段逐项校验，Envelope 与 Manifest 逐字段校验并验证 Payload 指纹。
- Envelope JSON Schema、Trend/Risk Golden、Schema 回归、序列化/反序列化和非法输入测试已补齐。
- Envelope Marshal/Unmarshal 均执行结构校验；input_snapshot_refs 必须显式存在、唯一且按 Manifest 规范顺序排列，允许空数组。
- QNT-CTR-001 Registry Entry 2.0.0 已接入 Go `SampleCountPolicy`；`ValidateMetric` 按 `required_in_metric` 检查字段存在性，保留缺失、null、0 的差异；Schema 1.0.0 保留历史投影。

<badge color="success">Quant 专项 CI 通过</badge> Workflow `43142021410` 对验收 SHA `4ce2fd155c21e7f8fe850c6f7f8c8d105b3dd47e` 的 gofmt、Quant 全量单测、Manifest 30 秒随机 Fuzz、Race、Vet 全部通过。

**Payload 支持边界：**
- 已注册：`trend/1.0.0`、`risk/1.0.0`。
- 尚未注册：`indicator`、`factor`。虽然已有部分 Metric DTO/Schema/Golden，但完整 IndicatorResultPayload、FactorResultPayload 容器 Schema/Golden、跨字段 Validator 和 Registry 元数据尚未齐备；不得据此生成或消费完整 Envelope。
- `market/industry/stock/strategy/backtest/evaluation` 仍为预留类型，当前必须拒绝，不能用宽泛 object/map 兜底。

项目级状态：QNT-CTR-004/005 均为 `frozen`（DEC-PLAT-002-051）。未完成的 Payload 子类型、运行时授权和消费者验收属于 L2 实现符合性待办，不改变契约冻结状态。

当前剩余阻塞：
- Indicator/Factor：补齐各自强类型 Payload 容器 Schema/Golden、字段/顺序校验及 Registry 驱动的 Metric 语义校验；完成前保持未注册。
- Quant 授权：尚无完整 Application/GetResult 读取链、当前 PolicySnapshot Provider、审计和授权检查至 Payload 释放的 TOCTOU 保护。
- Data：DailyBars v2 Decimal 输入、真实 PIT/复权/交易日历及质量证据；没有真实数据证据，不标记真实数据验收通过。
- Platform/Server：正式 Storage/事务接入、根 Migration 注册和统一 Diagnostics 装配。
- Researchflow：Envelope discriminator/Schema/版本消费校验、当前授权引用传递及双方 Contract Test。

全仓 Go Test Suite 最近一次执行（Workflow `43142021397`，验收 SHA `4ce2fd155c21e7f8fe850c6f7f8c8d105b3dd47e`）未通过：根模块 `Test .` 的失败集中在 Notification application 测试，独立 Researchflow 模块测试 Job 也失败；Researchflow MariaDB lifecycle integration 通过。Quant 包在同一 SHA 的专项 CI 全部通过。上述失败不属于 Quant，本分支未修改其他模块。

## 文档索引

| 文档 | 权威用途 |
|---|---|
| [01-量化引擎总体设计](01-量化引擎总体设计.md) | Quant 职责、架构边界、子系统和旧资产迁移决策 |
| [02-任务拆分与验收](02-任务拆分与验收.md) | 唯一模块任务清单、阶段状态、验收证据和跨模块依赖 |
| [03-首批输入输出契约与验收补充](03-首批输入输出契约与验收补充.md) | 首批输入输出细节、算法口径、Schema/Golden 和实现边界 |
| [04-RiskDecision DTO](04-RiskDecision%20DTO.md) | RiskDecision DTO 字段、校验和验收边界 |
| [05-TrendResult DTO](05-TrendResult%20DTO.md) | TrendResult DTO 字段和校验边界 |
| [06-Indicator计算上下文接入](06-Indicator计算上下文接入.md) | Indicator 计算上下文接入说明 |
| [07-Factor质量策略执行入口](07-Factor质量策略执行入口.md) | Factor 质量策略与执行入口 |
| [08-RiskDecision硬门禁状态聚合](08-RiskDecision硬门禁状态聚合.md) | 风险硬门禁状态聚合规则 |
| [09-RiskDecision构造入口](09-RiskDecision构造入口.md) | RiskDecision 构造边界 |
| [10-RiskDecision版本化规则清单](10-RiskDecision版本化规则清单.md) | 版本化风险规则集 |
| [11-RepositoryPort与不可变结果记录](11-RepositoryPort与不可变结果记录.md) | Repository Port、结果身份和不可变记录 |
| [12-RiskRuleSet执行入口](12-RiskRuleSet执行入口.md) | RuleSet 执行顺序、失败和取消语义 |
| [13-MemoryRepository参考实现](13-MemoryRepository参考实现.md) | 测试用内存 Repository，不是生产存储 |
| `schema/` | Quant DTO、Manifest、Registry 和参数 Schema |
| `golden/` | 固定算法样本、DTO/Schema Golden 和自动复算输入 |

## 文档维护规则

1. 本 README 只维护模块状态摘要和文档导航，不复制任务明细或契约字段表。
2. 任务状态和本模块验收证据以 [02](02-任务拆分与验收.md) 为准；算法/输入输出细节以 [03](03-首批输入输出契约与验收补充.md) 及专项文档为准。
3. QNT-CTR 编号、跨模块公共语义和 `pending_evidence/frozen` 状态只以项目级契约目录为准；本模块文档不得另行冻结公共契约。
4. 历史 CI 记录保留用于追溯。当前状态必须同时写明被测 SHA、Workflow ID 和覆盖范围；文档提交后的新 HEAD 不得冒充已执行过代码 CI。
5. Fixture/Golden、Mock、模块内 CI、真实数据验收和跨模块集成验收必须分层描述，不得互相替代。

## 模块职责

Quant 负责确定性、可复算的数值分析：Indicator、Factor、Trend、Market、Industry、Stock、Strategy、Backtest、Risk、Evaluation。不得调用 LLM，不负责研究编排，不直接访问 Provider/HTTP/SQL/Redis。

输入由 Data 提供证券、行情、财务、估值、交易日历、行业历史成分、PIT 和质量契约；输出主要由 Researchflow 消费。**Researchflow 是 Quant Application 的直接编排消费者；Research 不直接 import Quant，也不直接调用 Quant 内部实现。** Quant 风险事件通过本地 Outbox 遵循 INT-CTR-001 发布。

## 目标结构

```text
internal/quant/
  domain/
  indicator/
  factor/
  trend/
  market/
  industry/
  stock/
  strategy/
  backtest/
  risk/
  evaluation/
```

首期只按实际任务创建必要包，不为目录齐全创建空包。旧代码迁移结论见 [01](01-量化引擎总体设计.md) 第 6 节；输入输出契约和 Golden 设计见 [03](03-首批输入输出契约与验收补充.md)。

- QNT-CTR-001 Registry Entry Schema 2.0.0 与 Go Entry.SampleCountPolicy 已对齐，ValidateMetric 按 required_in_metric 拒绝缺失字段并保留显式 null/0 的差异；旧版 1.0.0 保留。首批真实 Registry Entry/Golden、各 metric_code 的权威 counting_rule 仍属 L2 待办。
