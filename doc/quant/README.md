# Quant 模块

- 分支：`feat/quant`
- 代码范围：`internal/quant/**`
- 文档范围：`doc/quant/**`
- 任务前缀：QNT

## 当前状态

当前代码基线：`532515aa327f0821006074752864ca6b71e0d109`（2026-11-11）。

已具备首批 Indicator/Factor 纯计算、CalculationContext、基础 Registry、RiskDecision/RuleSet 执行框架、TrendResult DTO、Manifest/ResultRef 和 Repository Save Port/MemoryRepository。当前完成的契约修正：

- DEC-PLAT-002-048：Manifest 已绑定 `scope`、条件必需的 `security_id`、可选 `trade_date`、必需 `period`、`quality_status`、`result_status`；六个字段全部进入 JCS 指纹和 ResultID。
- Ref 投影已收敛：调用方只能提供 Ref 自身 `schema_version`、当前 `authorization_ref` 和记录 `created_at`；业务字段从已验证 Manifest 派生。
- 项目级 QNT-CTR-005 Schema/Golden、GOV-CTR-006 AuthorizationRef Golden 已同步；Manifest ResultID 与 AuthorizationRef `resource_ref` 一致。
- DEC-PLAT-002-049：Risk RuleSet 必须非空且至少含一个 block；所有 block 必须 required；Reason 唯一、severity 匹配、按规则集顺序输出；RiskDecision reasons 非空、status 与聚合结果一致、evaluated_at 不早于 system_as_of。
- TrendResult 的每个 evidence Metric 必须与 TrendResult 使用相同 `system_as_of`。

<badge color="success">Quant 专项 CI 通过</badge> Workflow `43012698608` 对代码 SHA `532515aa327f0821006074752864ca6b71e0d109` 的 gofmt、Quant 全量单测、Manifest Fuzz、Race、Vet 全部通过。

**仓库级 Go Test Suite 未通过**（Workflow `43012698527`，同一代码 SHA）。失败来自其他模块：
- LLM：`responses_json_test.go` string/rune 类型不匹配。
- Notification：Migration 测试 uint64/int 类型不匹配，Application 多项投递生命周期测试失败。
- Researchflow：`resultvalidation/composer.go` 引用未定义的 `validSourceOwner`、`evidenceGraphHasCycle`。
Quant 专项工作流全部通过；未修改这些模块。

项目级状态：QNT-CTR-004 为 `pending_evidence`，因为 Envelope/IndicatorResultPayload/FactorResultPayload 机器定义和跨层 Validator 尚未完成；QNT-CTR-005 Manifest 字段定义已对齐并保持 `frozen`，但运行时授权仍未完成。

当前剩余阻塞：
- Data：DailyBars v2 Decimal 输入、真实 PIT/复权/交易日历及质量证据；没有真实数据证据，不标记真实数据验收通过。
- Quant/Envelope：当前没有完整 Envelope DTO/读取路径，不能完成 Trend/Risk/Indicator/Factor Payload 与 Envelope 的字段逐项一致性 Validator。
- Quant 授权：Platform 已提供可信 `security.Principal` 和请求上下文读取能力；但 Quant 尚无 Application/GetResult/Envelope 读取入口、PolicySnapshot Provider 和审计/原子授权读取实现。不能将 Principal 已存在误报为 Quant 运行时授权已完成。
- Platform/Server：正式 Storage/事务接入、根 Migration 注册和统一 Diagnostics 装配。
- Researchflow：ResolvedTemplate 空输入授权校验及完整消费者 Contract Test。

空快照目前仅完成 Manifest/Ref/Schema 结构支持。真正允许空输入仍要求 Researchflow 校验 `ResolvedTemplate.allow_empty_inputs=true`，Quant 校验可信 Principal、已注册用例和 Registry 无外部事实依赖声明；当前没有完整 Application 门禁，不能宣称端到端授权验收通过。

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
