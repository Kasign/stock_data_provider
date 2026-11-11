# Quant 模块

- 分支：`feat/quant`
- 代码范围：`internal/quant/**`
- 文档范围：`doc/quant/**`
- 任务前缀：QNT

## 当前状态

当前代码基线：`2c3c4720f5b1049b34f84cf1df25021525b13439`（2026-11-11）。

已具备首批 Indicator/Factor 纯计算、CalculationContext、基础 Registry、RiskDecision/RuleSet 执行框架、TrendResult DTO、Manifest/ResultRef 和 Repository Save Port/MemoryRepository。当前已按 DEC-PLAT-002-048 完成 Manifest 身份字段和 Ref 投影的代码/机器契约同步：

- Manifest 必需绑定 `result_type`、`scope`、条件必需的 `security_id`、可选 `trade_date`、必需 `period`、`quality_status`、`result_status`，以及原有版本、算法、参数、时间、输入快照和 Payload 指纹字段。
- 上述字段全部进入 RFC 8785 JCS + SHA-256 Manifest 指纹；`result_id` 由该指纹派生。空 `input_snapshot_refs=[]` 仍是合法结构，nil/null/省略拒绝。
- `BuildRef` 的输入现在只接受 Ref 自身 `schema_version`、当前 `authorization_ref` 和记录 `created_at`；其他业务字段全部从已验证 Manifest 投影，避免调用方重复传入造成漂移。
- 项目级 QNT-CTR-005 Schema/Golden、GOV-CTR-006 Quant AuthorizationRef Golden 已同步。当前 Manifest ResultID 为 `quant_result:sha256:97d6d042ef4202bf8879e4f5773803f53a9940033f791166443cdb5d2b371b5d`；GOV-CTR-006 `resource_ref` 已绑定该值，授权指纹为 `5b1e06a35a82faa85d1762ce339705f2eb1edd62dfd753d79a22151c141a3c83`。

**CI 尚未对当前代码 SHA 完成验收。** Quant Workflow `43011022598` 与 Go test suite `43011022585` 对代码 SHA `2c3c4720f5b1049b34f84cf1df25021525b13439` 已触发，记录本文件时仍处于 queued/pending；不得提前标记通过。之前几轮 CI 暴露的格式、测试样本缺字段和 Schema 正则问题已修正，最终以当前 SHA 的完整结果为准。

项目级契约状态已校正：QNT-CTR-004 为 `pending_evidence`，因为完整 Envelope/IndicatorResultPayload/FactorResultPayload Schema/Golden 和 Risk/Payload 跨层 Validator 尚缺；QNT-CTR-005 的 Manifest 机器字段定义已对齐并恢复 `frozen`，但 DEC-PLAT-002-050 的运行时授权签发、每次读取重新授权、审计与 TOCTOU 门禁仍未完成。契约冻结不代表实现验收完成。

当前主要外部阻塞：
- Data：DailyBars v2 Decimal 输入、真实 PIT/复权/交易日历及质量证据；没有真实数据证据，不标记真实数据验收通过。
- Server/GOV：可信 Principal 注入、Quant Owner 当前策略授权 Adapter 与每次读取复核。
- Platform/Server：正式 Storage/事务接入、根 Migration 注册和统一 Diagnostics 装配。
- Researchflow：ResolvedTemplate 空输入授权校验及完整消费者 Contract Test。
- Quant：Risk RuleSet 完整性/聚合与 Trend/Risk/Indicator/Factor Payload 对 Envelope 的跨层语义 Validator。

空快照目前仅完成 Manifest/Ref/Schema 结构支持。真正允许空输入仍要求 Researchflow 校验 `ResolvedTemplate.allow_empty_inputs=true`，Quant 校验可信 Principal、已注册用例和 Registry 无外部事实依赖声明；当前没有完整 Application 门禁，不能宣称空输入端到端授权验收通过。

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
