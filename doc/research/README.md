# Research 模块

- 分支：`feat/research`
- 代码：`internal/research/**`
- 任务前缀：RSC
- 当前阶段：R1 Context 领域基础
- 当前状态：Context Domain 首批实现已完成 7/7 临时独立 Go package 单元测试；这不等同于仓库完整构建/CI。Application、Repository 和跨模块集成尚未完成。

## 模块职责

Research 负责 Context、Evidence、Event、Claim、Hypothesis、Scenario 等研究语义模型。

- Data：行情、财务、证券主数据、PIT 数据及其质量。
- News：新闻采集、原文、身份、版本、授权和只读证据查询。
- Quant：确定性数值计算及其版本化结果；Research 不直接依赖或调用 Quant。
- Research：组织研究语义、证据、事件、主张、假设和情景。
- Researchflow：Run/Stage/Attempt 生命周期、单次 Run 的 Context Assembly、分区选择、Token Budget 和裁剪。

Research 不采集新闻、不拥有行情事实、不重算 Quant 指标，不依赖 Quant 或 Researchflow。Research 保存上游稳定引用，不访问其他模块私有 Repository、SQL 或 Domain。

## 当前代码

```text
internal/research/
  context/
    model.go
    validate.go
    validate_test.go
```

R1 当前只实现 Context 领域结构和结构校验。没有 HTTP 接口、真实数据 Adapter、Repository、Migration 或 Run 编排接入。

旧 `go-core/internal/contextpack` 和 `internal/analysis` 是历史分析流程，不等同于新 Research Domain；仅作为迁移行为参考，不重复建设其已有的数据整理能力。

## 当前契约状态

- Context：字段依据 `doc/整体任务规划/Quant-Research公开DTO字段级契约.md` 第 3.1、9.1、9.2、9.8 节；尚无专属冻结 Schema/Golden。本模块当前实现是领域校验模型，不代表跨模块契约已冻结。
- Evidence：遵循项目级 `RSC-CTR-001` Evidence Schema/Golden；该契约属于 Evidence，不属于 Context。
- News → Research 只读证据查询：目标契约尚未实现/冻结；不得据此宣称真实新闻接入通过。
- Research → Researchflow 的 Context 消费接口：字段语义已形成设计基线，Schema/Golden/Contract Test 尚未完成。
- RSC-DEC-001：Quant 结果通过 Researchflow 获取；Research 不新增 Quant Port、不直接调用 Quant、不自行解引用。
- RSC-OPEN-002：ResearchEvent.changed_at 与 INT-CTR-001 事件时间映射存在字段缺口；不阻断 Context/Evidence。

详细设计见 [01-研究领域模型与证据体系](01-研究领域模型与证据体系.md)，字段与授权链补充见 [03-Context与Evidence字段语义及授权链补充](03-Context与Evidence字段语义及授权链补充.md)，任务与验收见 [02-任务拆分与验收](02-任务拆分与验收.md)。
