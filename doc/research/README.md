# Research 模块

- 分支：`feat/research`
- 代码：`internal/research/**`
- 任务前缀：RSC
- 当前阶段：R1 Context + R2 Evidence + R3 Event + R5 Claim/Hypothesis 领域基础；R7 Storage Port/Schema/Context-Event-Claim SQL Adapter 第一阶段
- 当前状态：Context/Evidence/Event/Claim Domain 首批结构与校验已实现；Context 版本链、Evidence JCS/授权过期边界、Event 生命周期及 Claim 文本校验新增测试已写入但尚未执行。RSC-007 已新增四个领域 Repository Port 和 Research Migration v1/v2（v2 保留 RFC3339Nano PIT 纳秒精度）；Context/Event/Claim SQL Repository Adapter 已实现首批追加与精确版本读取；真实 MariaDB 集成测试、并发/故障注入和组合根注册尚未验收，Evidence SQL Adapter 受 RSC-OPEN-007 阻断。未完成仓库完整构建/CI、真实 Data/News、Owner 当前授权复核、Application、Outbox 和跨模块集成。

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
    repository.go
  evidence/
    model.go
    validate.go
    validate_test.go
    authorization.go
    authorization_test.go
    repository.go
  event/
    model.go
    validate.go
    validate_test.go
    repository.go
  claim/
    model.go
    validate.go
    validate_test.go
    repository.go
  storage/
    migrations.go
    migrations_test.go
```

R1/R2/R3/R5 当前实现 Context、Evidence、Event、Claim/Hypothesis 领域结构及本地校验；R7 当前仅完成 Repository Port 与 Migration Schema 清单：包括 Evidence 内容指纹/PIT、GOV-CTR-006 授权快照完整性、Event 追加版本和生命周期转换、Claim/Hypothesis 认识论字段与验证条件。没有 HTTP 接口、真实数据 Adapter、Migration 执行、Outbox 或 Run 编排接入；Context/Event/Claim SQL Repository Adapter 已按 Migration v1/v2 实现，但尚无真实 MariaDB 集成验收。

旧 `go-core/internal/contextpack` 和 `internal/analysis` 是历史分析流程，不等同于新 Research Domain；仅作为迁移行为参考，不重复建设其已有的数据整理能力。

## 当前契约状态

- Context：字段依据 `doc/整体任务规划/03-模块接口契约/Quant-Research公开DTO字段级契约.md` 第 3.1、9.1、9.2、9.8 节；已实现结构校验和 ValidateNextVersion 版本链校验，但尚无专属冻结 Schema/Golden、Repository 追加存储或 CAS。本模块当前实现是领域校验模型，不代表跨模块契约已冻结。
- Evidence：遵循项目级 `RSC-CTR-001` Evidence Schema/Golden；已实现 Go Domain、结构校验、JCS/SHA-256 内容指纹和严格 PIT 本地校验。当前补充 GOV-CTR-006 AuthorizationRef/PolicySnapshot 结构绑定、策略 tuple 校验、decision_fingerprint 复算及可信调用主体/资源/用途/内容级别绑定校验；这些本地校验不等于 Owner 当前授权复核或授权签发。真实来源接入仍未完成。该契约属于 Evidence，不属于 Context。
- AuthorizationRef：只验证不可变快照结构和指纹，并绑定 Adapter 提供的可信调用上下文；不把引用当 bearer token，不以历史 allowed 代替读取时 Owner 当前策略复核。
- News → Research 只读证据查询：目标契约尚未实现/冻结；不得据此宣称真实新闻接入通过。
- Research → Researchflow 的 Context 消费接口：字段语义已形成设计基线，Schema/Golden/Contract Test 尚未完成。
- RSC-DEC-001：Quant 结果通过 Researchflow 获取；Research 不新增 Quant Port、不直接调用 Quant、不自行解引用。
- RSC-DEC-002：changed_at 的语义已确定为状态迁移提交时间，随 lifecycle_changed Outbox 事件持久化。
- RSC-OPEN-002：Quant-Research DTO 尚未定义 changed_at 字段，具体 DTO/Outbox 映射仍待项目级契约统一。
- RSC-OPEN-003：Event 规范化时间要求可追溯到规则版本，但当前 DTO 没有 normalization_rule_version；未决前不声称规范化时间来源链完整。
- RSC-OPEN-004：Event 合并/拆分要求 source_refs/derived_from_refs，但 ResearchEvent 字段表未定义这两个字段；未决前不实现跨 Event 归并关系。
- RSC-OPEN-005：Claim/Hypothesis 版本治理要求 supersedes_version，但字段表未定义该字段；当前仅验证版本号连续性，不能声称显式替代关系完整。
- RSC-004 等待 Data 提供稳定 Security/Industry 引用和公开查询端口；不基于旧 Analysis 的 UniverseResolver 提前开发实体 Adapter。
- RSC-OPEN-006：ScenarioCondition.expected_value 的 typed scalar/object 具体类型、ConditionGroup 联合结构和按 metric_ref/unit 的校验规则尚未形成机器契约；不使用 any/map 假装完成。
- RSC-OPEN-007：Evidence.content_fingerprint 排除 authorization_ref，但完整 payload_sha256 包含该字段；授权续期后的同 evidence_id 留档/幂等语义未决，暂缓 Evidence SQL Adapter。

详细设计见 [01-研究领域模型与证据体系](01-研究领域模型与证据体系.md)，字段与授权链补充见 [03-Context与Evidence字段语义及授权链补充](03-Context与Evidence字段语义及授权链补充.md)，任务与验收见 [02-任务拆分与验收](02-任务拆分与验收.md)。


## 本轮补充：Evidence 固定过期时间与 Claim 字段规范化

- Evidence 新增 `ValidateEvidenceAuthorizationBindingAt`：在已有 AuthorizationRef 指纹、主体/资源/用途/Owner/内容级别绑定校验后，使用调用方传入的可信 UTC 检查时点执行固定过期时间门禁；`now >= expires_at` 拒绝读取，`expires_at=null` 不构成固定过期。
- 该方法不替代来源 Owner 的实时策略复核、撤销检查和策略检查至内容读取之间的一致性保护。生产 Adapter 仍须由 Owner 执行完整授权读取流程；Research 不得仅凭历史 allowed 快照读取上游受限内容。
- Claim/Hypothesis 的 validation_method 与 expected_observation 现在复用统一必填字符串校验，拒绝空白及首尾空格；不新增状态迁移或 supersedes 私有字段。
- 固定过期时间前/到期/过期后、无固定过期和无效检查时点测试已写入，尚未执行；不代表 Owner 授权 E2E 或真实来源验收通过。



- RSC-OPEN-007：Evidence 内容指纹排除 authorization_ref，但完整 payload 摘要包含它；同一 evidence_id 重新授权后的幂等/留档语义待项目契约确认。当前不覆盖旧 Evidence 行，Evidence SQL Adapter 暂缓该边界。



- RSC-007 Repository Port 已明确追加事务边界、版本连续性、完整 payload 幂等与冲突规则；SQL Adapter 尚未实现。
- RSC-OPEN-007 阻断 Evidence SQL Adapter 的授权快照续期边界，不阻断 Context/Event/Claim SQL Adapter。


## RSC-007 本轮实现：Context/Event/Claim SQL Repository Adapter

- 新增 `storage.ContextRepository`、`storage.EventRepository`、`storage.ClaimRepository`，分别实现对应领域 Repository Port。
- Append 使用 InnoDB 事务；先创建/锁定 `research_aggregate_heads` 对应行，再读取最新版本。新版本必须连续递增；已有版本只有完整 JSON payload 的 SHA-256 完全一致才按幂等重放处理。
- 对新版本读取上一版 payload 并调用各领域 `ValidateNextVersion`；插入不可变版本快照后再更新 aggregate head，任一步失败均回滚。
- GetVersion 按 ID+版本精确读取，反序列化后重新执行领域 Validate，并核对返回对象身份。
- 构造器注入 `*sql.DB` 和可选 Clock；不自动执行 Migration。SQL 表名/列名为 Adapter 内固定常量。
- PIT 字段按 Migration v2 使用 UTC RFC3339Nano 文本保存；完整 JSON payload 保留纳秒精度。
- Evidence Adapter 仍未实现，RSC-OPEN-007 不变。
- 本轮未运行 go test、未连接 MariaDB；因此只记录代码实现，不标记事务、并发、幂等和故障恢复验收通过。
