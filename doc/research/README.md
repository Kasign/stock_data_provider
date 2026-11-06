# Research 模块

- 分支：`feat/research`
- 代码：`internal/research/**`
- 任务前缀：RSC
- 当前阶段：R1 Context + R2 Evidence + R3 Event + R5 Claim/Hypothesis 领域基础
- 当前状态：Context/Evidence Domain 已有临时独立 Go 1.22 package 测试证据；Evidence 新增 GOV-CTR-006 授权快照校验、Event Domain/状态机及 Claim/Hypothesis Domain 测试已写入但尚未执行。未完成仓库完整构建/CI、真实 Data/News、Owner 当前授权复核、Application、Repository 和跨模块集成。

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
  evidence/
    model.go
    validate.go
    validate_test.go
    authorization.go
    authorization_test.go
  event/
    model.go
    validate.go
    validate_test.go
  claim/
    model.go
    validate.go
    validate_test.go
```

R1/R2/R3/R5 当前只实现 Context、Evidence、Event、Claim/Hypothesis 领域结构及本地校验：包括 Evidence 内容指纹/PIT、GOV-CTR-006 授权快照完整性、Event 追加版本和生命周期转换、Claim/Hypothesis 认识论字段与验证条件。没有 HTTP 接口、真实数据 Adapter、Repository、Migration、Outbox 或 Run 编排接入。

旧 `go-core/internal/contextpack` 和 `internal/analysis` 是历史分析流程，不等同于新 Research Domain；仅作为迁移行为参考，不重复建设其已有的数据整理能力。

## 当前契约状态

- Context：字段依据 `doc/整体任务规划/Quant-Research公开DTO字段级契约.md` 第 3.1、9.1、9.2、9.8 节；尚无专属冻结 Schema/Golden。本模块当前实现是领域校验模型，不代表跨模块契约已冻结。
- Evidence：遵循项目级 `RSC-CTR-001` Evidence Schema/Golden；已实现 Go Domain、结构校验、JCS/SHA-256 内容指纹和严格 PIT 本地校验。当前补充 GOV-CTR-006 AuthorizationRef/PolicySnapshot 结构绑定、策略 tuple 校验、decision_fingerprint 复算及可信调用主体/资源/用途/内容级别绑定校验；这些本地校验不等于 Owner 当前授权复核或授权签发。真实来源接入仍未完成。该契约属于 Evidence，不属于 Context。
- AuthorizationRef：只验证不可变快照结构和指纹，并绑定 Adapter 提供的可信调用上下文；不把引用当 bearer token，不以历史 allowed 代替读取时 Owner 当前策略复核。
- News → Research 只读证据查询：目标契约尚未实现/冻结；不得据此宣称真实新闻接入通过。
- Research → Researchflow 的 Context 消费接口：字段语义已形成设计基线，Schema/Golden/Contract Test 尚未完成。
- RSC-DEC-001：Quant 结果通过 Researchflow 获取；Research 不新增 Quant Port、不直接调用 Quant、不自行解引用。
- RSC-OPEN-002：ResearchEvent.changed_at 与 INT-CTR-001 事件时间映射存在字段缺口；不阻断 Event Domain。
- RSC-OPEN-003：Event 规范化时间要求可追溯到规则版本，但当前 DTO 没有 normalization_rule_version；未决前不声称规范化时间来源链完整。
- RSC-OPEN-004：Event 合并/拆分要求 source_refs/derived_from_refs，但 ResearchEvent 字段表未定义这两个字段；未决前不实现跨 Event 归并关系。
- RSC-OPEN-005：Claim/Hypothesis 版本治理要求 supersedes_version，但字段表未定义该字段；当前仅验证版本号连续性，不能声称显式替代关系完整。

详细设计见 [01-研究领域模型与证据体系](01-研究领域模型与证据体系.md)，字段与授权链补充见 [03-Context与Evidence字段语义及授权链补充](03-Context与Evidence字段语义及授权链补充.md)，任务与验收见 [02-任务拆分与验收](02-任务拆分与验收.md)。
