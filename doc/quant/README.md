# Quant 模块

- 分支：`feat/quant`
- 代码范围：`internal/quant/**`
- 文档范围：`doc/quant/**`
- 任务前缀：QNT
- 当前文档基线：2026-11-09，分支 HEAD `41fc0dc0dcffae90f3ba82a4179729d0d348ceb8`

## 当前状态

Quant 已具备首批 Indicator/Factor 纯计算、CalculationContext、基础 Registry、RiskDecision/RuleSet 执行框架、TrendResult DTO、Manifest/ResultRef 和 Repository Save Port 等模块内能力。最近一次 Quant 独立 CI 对应 SHA 为 `471dae4e8b55e999719a691c7761df3cee8f8e8c`，Workflow `42598371072`，结果成功，覆盖 gofmt、Quant 全包测试、Manifest 30 秒随机 Fuzz、Race 和 Vet。

上述 CI 证明的是该 SHA 的 Quant 模块内代码验证。当前 HEAD 后续仅增加文档提交，没有代码变化；不能将 Workflow 记为当前 HEAD 直接触发的 CI。它也不代表真实行情、生产 Registry 元数据、运行时授权、完整结果 Envelope、Platform Storage 或 Server 集成验收完成。

当前主要外部阻塞：
- Data：DailyBars v2 Decimal 输入、真实 PIT/复权/交易日历及质量证据。
- Server/GOV：可信 Principal 注入、Quant Owner 授权签发与每次读取的当前授权复核。
- Platform/Server：正式 Storage/事务接入、根 Migration 注册和统一 Diagnostics 装配。
- 项目契约/Researchflow：Envelope 与 Payload 版本语义、强类型 Payload 映射及消费者 Contract Test。

QNT-CTR-001～005 的权威状态以项目级 [跨模块契约目录与冻结状态](../整体任务规划/02-契约治理/跨模块契约目录与冻结状态.md) 为准；当前均不得因模块内 CI 通过而标记为 `frozen`。

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
