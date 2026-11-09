# Research 模块

- 分支：`feat/research`
- 代码：`internal/research/**`
- 任务前缀：RSC
- 当前阶段：R1 Context + R2 Evidence + R3 Event + R5 Claim/Hypothesis 领域基础；R7 Storage Port/Schema/Context-Event-Claim-Evidence SQL Adapter；R8 Diagnostics 领域快照 Adapter 第一批
- 当前状态：Context/Evidence/Event/Claim Domain 首批结构与校验已实现；Context 版本链、Evidence JCS/授权过期边界、Event 生命周期及 Claim 文本校验新增测试已写入但尚未执行。RSC-008 已新增 Context/Event/Claim 快照诊断 Adapter，复用 Platform Check/Observation/Runner，不建设 Run/Stage/Attempt；Evidence 诊断仍需来源 Owner 当前授权复核能力。Researchflow 目前没有稳定的 Research Diagnostics Application Port；跨模块调用须等待 Research Application Port 与 Researchflow FLOW-009 Facade 对齐，不允许直接依赖 Research Domain/Repository。RSC-007 已新增四个领域 Repository Port 和 Research Migration v1/v2/v3（v2 保留 RFC3339Nano PIT 纳秒精度）；Context/Event/Claim SQL Repository Adapter 已实现事务追加与精确版本读取；Evidence SQL Repository Adapter 已按完整 payload 不可变策略实现 Append/Get。MariaDB 集成测试覆盖追加、重放、冲突、版本跳号、并发、回滚、取消和摘要篡改，但尚未执行；组合根 Migration 注册尚未验收。未完成仓库完整构建/CI、真实 Data/News、Owner 当前授权复核、Application、Outbox 和跨模块集成。

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
  application/
    application.go
    application_test.go
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
    repository.go
    context_repository.go
    event_repository.go
    claim_repository.go
    evidence_repository.go
    repository_test.go
    repository_integration_test.go
```

R1/R2/R3/R5 当前实现 Context、Evidence、Event、Claim/Hypothesis 领域结构及本地校验；R7 已完成 Repository Port、Migration Schema 及 Context/Event/Claim SQL Adapter 第一批实现：包括 Evidence 内容指纹/PIT、GOV-CTR-006 授权快照完整性、Event 追加版本和生命周期转换、Claim/Hypothesis 认识论字段与验证条件。Research Application 首批 Context/Evidence 用例已实现；没有 HTTP 接口、真实数据 Owner Adapter、Migration 执行、Outbox 或 Run 编排接入；Context/Event/Claim SQL Repository Adapter 已按 Migration v1/v2/v3 实现，但尚无真实 MariaDB 集成验收。

旧 `go-core/internal/contextpack` 和 `internal/analysis` 是历史分析流程，不等同于新 Research Domain；仅作为迁移行为参考，不重复建设其已有的数据整理能力。

## 当前契约状态

- Context：字段依据 `doc/整体任务规划/03-模块接口契约/Quant-Research公开DTO字段级契约.md` 第 3.1、9.1、9.2、9.8 节；已实现结构校验、ValidateNextVersion 版本链校验、Context v1.0.0 JSON Golden 和 Draft 2020-12 Schema 候选；新增 DTO 字段一致性测试（待执行）。Schema 仍是 Research 分支候选，尚未接入完整 JSON Schema Validator CI/消费方 Contract Test，不代表跨模块契约已冻结；SQL Repository 已支持事务追加和版本链校验，尚待真实 MariaDB 验收。
- Evidence：遵循项目级 `RSC-CTR-001` Evidence Schema/Golden；已实现 Go Domain、结构校验、JCS/SHA-256 内容指纹和严格 PIT 本地校验。当前补充 GOV-CTR-006 AuthorizationRef/PolicySnapshot 结构绑定、策略 tuple 校验、decision_fingerprint 复算及可信调用主体/资源/用途/内容级别绑定校验；这些本地校验不等于 Owner 当前授权复核或授权签发。真实来源接入仍未完成。该契约属于 Evidence，不属于 Context。
- AuthorizationRef：只验证不可变快照结构和指纹，并绑定 Adapter 提供的可信调用上下文；不把引用当 bearer token，不以历史 allowed 代替读取时 Owner 当前策略复核。
- News → Research：News 分支已出现 ResearchEvidenceQuery 请求/响应 DTO、Port、Cursor/Snapshot 结构及快照存储实现；但项目级 NEWS-CTR-001/002 仍为 pending_evidence，Research 尚未接入，也未完成当前授权复核的端到端验收。
- Research → Researchflow 的 Context 消费接口：字段语义已形成设计基线，Schema/Golden/Contract Test 尚未完成。
- RSC-DEC-001：Quant 结果通过 Researchflow 获取；Research 不新增 Quant Port、不直接调用 Quant、不自行解引用。
- RSC-DEC-002：changed_at 的语义已确定为状态迁移提交时间，随 lifecycle_changed Outbox 事件持久化。
- Claim/Hypothesis 状态迁移已按项目契约第 9.9 节补充 Domain 门禁：proposed→testing→结论状态，同状态允许追加修订；明确终态不允许回退。unknown 的恢复路径尚无契约定义，当前按 fail-closed 处理。测试尚未执行。
- Event SourceTime confidence Validator 已覆盖 [0,1]、NaN/Inf 以及 confidence_method/version 成对绑定规则；本轮补充边界单测，尚未执行。
- RSC-OPEN-002：Quant-Research DTO 尚未定义 changed_at 字段，具体 DTO/Outbox 映射仍待项目级契约统一。
- RSC-OPEN-003：Event 规范化时间要求可追溯到规则版本，但当前 DTO 没有 normalization_rule_version；未决前不声称规范化时间来源链完整。
- RSC-OPEN-004：Event 合并/拆分要求 source_refs/derived_from_refs，但 ResearchEvent 字段表未定义这两个字段；未决前不实现跨 Event 归并关系。
- RSC-OPEN-005：Claim/Hypothesis 版本治理要求 supersedes_version，但字段表未定义该字段；当前仅验证版本号连续性，不能声称显式替代关系完整。
- RSC-004 等待 Data 提供稳定 Security/Industry 引用和公开查询端口；不基于旧 Analysis 的 UniverseResolver 提前开发实体 Adapter。
- RSC-OPEN-006：ScenarioCondition.expected_value 的 typed scalar/object 具体类型、ConditionGroup 联合结构和按 metric_ref/unit 的校验规则尚未形成机器契约；不使用 any/map 假装完成。
- RSC-OPEN-007：Evidence.content_fingerprint 排除 authorization_ref，但完整 payload_sha256 包含该字段；当前 Evidence SQL Adapter 已按完整 payload 不可变策略实现，相同 payload 幂等、不同 payload 冲突。多次授权签发历史若需保留，仍需项目契约定义独立授权记录/关联模型。

详细设计见 [01-研究领域模型与证据体系](01-研究领域模型与证据体系.md)，字段与授权链补充见 [03-Context与Evidence字段语义及授权链补充](03-Context与Evidence字段语义及授权链补充.md)，任务与验收见 [02-任务拆分与验收](02-任务拆分与验收.md)。


## 本轮补充：Evidence 固定过期时间与 Claim 字段规范化

- Evidence 新增 `ValidateEvidenceAuthorizationBindingAt`：在已有 AuthorizationRef 指纹、主体/资源/用途/Owner/内容级别绑定校验后，使用调用方传入的可信 UTC 检查时点执行固定过期时间门禁；`now >= expires_at` 拒绝读取，`expires_at=null` 不构成固定过期。
- 该方法不替代来源 Owner 的实时策略复核、撤销检查和策略检查至内容读取之间的一致性保护。生产 Adapter 仍须由 Owner 执行完整授权读取流程；Research 不得仅凭历史 allowed 快照读取上游受限内容。
- Claim/Hypothesis 的 validation_method 与 expected_observation 现在复用统一必填字符串校验，拒绝空白及首尾空格；不新增状态迁移或 supersedes 私有字段。
- 固定过期时间前/到期/过期后、无固定过期和无效检查时点测试已写入，尚未执行；不代表 Owner 授权 E2E 或真实来源验收通过。



- RSC-OPEN-007：Evidence 内容指纹排除 authorization_ref，但完整 payload 摘要包含它；同一 evidence_id 重新授权后的幂等/留档语义待项目契约确认。当前不覆盖旧 Evidence 行；Evidence SQL Adapter 已按完整 payload 不可变策略实现。



- RSC-007 Context/Event/Claim SQL Adapter 已实现首批追加和精确版本读取；真实 MariaDB 集成、并发/故障注入与组合根注册尚未验收。
- RSC-OPEN-007 仍阻断多次授权签发历史留档，不阻断当前 Evidence 不可变写入和读取。


## RSC-007 本轮实现：Context/Event/Claim SQL Repository Adapter

- 新增 `storage.ContextRepository`、`storage.EventRepository`、`storage.ClaimRepository`，分别实现对应领域 Repository Port。
- Append 使用 InnoDB 事务；先创建/锁定 `research_aggregate_heads` 对应行，再读取最新版本。新版本必须连续递增；已有版本只有完整 JSON payload 的 SHA-256 完全一致才按幂等重放处理。
- 对新版本读取上一版 payload 并调用各领域 `ValidateNextVersion`；插入不可变版本快照后再更新 aggregate head，任一步失败均回滚。
- GetVersion 按 ID+版本精确读取，反序列化后重新执行领域 Validate，并核对返回对象身份。
- 构造器注入 `*sql.DB` 和可选 Clock；不自动执行 Migration。SQL 表名/列名为 Adapter 内固定常量。
- PIT 字段按 Migration v2 使用 UTC RFC3339Nano 文本保存；完整 JSON payload 保留纳秒精度。
- Evidence Adapter 仍未实现，RSC-OPEN-007 不变。
- 本轮未运行 go test、未连接 MariaDB；因此只记录代码实现，不标记事务、并发、幂等和故障恢复验收通过。


- 新增环境变量门禁的 MariaDB 集成测试：设置 `RESEARCH_TEST_MYSQL_DSN` 后执行 Research Migration，并验证 Context/Event/Claim 的首次追加、相同 payload 重放、冲突重放、版本跳号、连续追加和精确版本读取；并验证 Context 并发相同 payload 幂等、并发不同 payload 仅一个胜出、插入失败时 aggregate head 回滚、请求取消不遗留部分记录。未设置 DSN 时测试会 Skip。当前尚未执行该测试。


## RSC-007 本轮补充：版本列范围与溢出保护

- 复核发现 Event/Claim 的 Go 版本字段为 `int`，Repository 内部按有符号整数处理，而 Migration v1 使用 `INT UNSIGNED`；数据库列宽与代码支持范围不一致。
- 新增 Migration v3，将 `research_event_versions.event_version`、`research_claim_versions.claim_version` 升级为 `BIGINT UNSIGNED`，不修改已发布的 v1/v2 历史迁移。
- Context、Event、Claim 的 `ValidateNextVersion` 在执行 `previous+1` 前检查各自整数上限，避免极值回绕导致版本链校验错误。
- Migration 单测和三个领域版本溢出测试已补充；尚未执行 Go 测试，也未在 MariaDB 验证 v1→v2→v3 升级。


## RSC-007 本轮补充：版本列范围与测试边界

- Event/Claim 的 Go 版本字段为 `int`，Repository 按有符号整数处理；Migration v1 的 `INT UNSIGNED` 列宽不足以覆盖 64 位 Go `int` 的范围。
- 新增 Migration v3，将 Event/Claim 版本列升级为 `BIGINT UNSIGNED`，不修改已发布的 v1/v2 历史迁移；Context/Event/Claim 的 `ValidateNextVersion` 在递增前检查整数上限。
- MariaDB 集成测试增加 v3 后列类型断言。原 Context Cancel 用例是在调用前取消请求，只能证明预先取消不会创建版本行/head；不代表事务执行中取消回滚。事务中取消与锁等待取消仍待独立故障注入。
- 新增测试尚未执行，未完成真实 MariaDB 升级验收。


## RSC-008 本轮实现：Research 领域快照诊断 Adapter

- 新增 `internal/research/diagnostics`，提供 Context/Event/Claim 的 Platform Check 工厂；构造时对输入 DTO 做 JSON 深拷贝，诊断期间不受调用方后续修改影响。
- Context 诊断将 valid/degraded/unknown/invalid 映射为 healthy/degraded/unavailable/failed；Event/Claim 仅执行各自本地结构校验，不把生命周期状态或认识论状态误作质量等级。
- Observation 仅输出稳定错误码、安全摘要及版本/质量状态属性，不输出研究问题、Evidence 内容或底层 Validator 原始错误。
- Evidence 诊断暂不接入：本地结构与授权快照校验不能替代来源 Owner 当前授权复核。该 Adapter 也不查询 Repository、不创建 Run/Stage/Attempt、不代表完整研究质量诊断。
- 新增 Context 质量映射、Event/Claim 结构校验、敏感字段不外泄、快照隔离和 Runner 取消行为测试；尚未执行。


## RSC-008 并发与调用边界补充

- 新增同一个 Context Check 被 24 个 goroutine 并发执行的测试，验证诊断闭包只读快照、每次调用独立生成 Observation。
- Platform Runner 已有并发上限、单项超时、总超时、父 Context 取消和排队任务超时测试；Research 快照 Check 不执行网络/数据库 I/O，不能人为制造慢调用来重复测试 Platform Runner。
- Researchflow 当前只定义了模块边界和 FLOW-008/009 任务，尚无稳定的 Research Diagnostics Application Port。Researchflow 不得直接 import Research Domain/Repository；待 Research Application Port 与 Researchflow Facade 明确后再接线。
- Research Check 只在 Run 开始时检查 Context 取消；本地结构校验是有界 CPU 工作，不提供可中断的远端 I/O。超时由 Platform Runner 的 Check.Timeout/TotalTimeout 统一约束。


- RSC-007 新增真实 MariaDB 锁等待取消故障注入用例：事务 A 持有 Context aggregate head 行锁，事务 B 在 Append 中等待；测试通过 `information_schema.PROCESSLIST` 确认 Append SQL 已到达数据库后取消 Context，验收要求 Append 返回 `context.Canceled`、head 仍为旧版本且没有新版本行。该用例依赖 `RESEARCH_TEST_MYSQL_DSN`，尚未执行。

## MariaDB 集成测试环境前置条件

- `RESEARCH_TEST_MYSQL_DSN` 必须指向可执行 Research Migration 的专用 MariaDB 测试库；测试会创建/升级 Research 自有表并清理测试聚合数据，不得指向生产库。
- 锁等待取消故障注入用例通过 `information_schema.PROCESSLIST` 观察独立连接正在执行的 Research aggregate-head SQL。测试账号必须具有查看其他连接进程信息的 `PROCESS` 权限；仅能查看自身连接的账号会导致测试无法确认真实锁等待，不能将其结果解释为取消路径通过。
- 若环境不允许授予 `PROCESS`，应在独立的受控测试实例验证该用例，或调整测试观测机制；不得删除“确认 SQL 已进入锁等待后再取消”的断言来规避权限问题。
- 当前未连接真实 MariaDB，以上是执行前置条件，不代表锁等待取消、事务回滚或 Migration 升级已验收。


## RSC-001 Context v1 Golden

- 内部回归样例：`internal/research/context/testdata/context_v1.golden.json`。
- 测试：`TestContextGoldenJSONV1`，校验 Context v1.0.0 的确定性 JSON 序列化结果。
- Golden 变更必须先 Review 字段契约和兼容性，不得仅为通过测试而直接刷新样例。
- JSON Schema 和真实 Validator 测试已补齐；GitHub Actions 强制门禁和跨模块 Contract Test 尚未完成，新增测试尚未执行。


## RSC-001 Context v1 Schema 候选

- Schema：`internal/research/context/schema/context-v1.schema.json`（JSON Schema Draft 2020-12）。
- Golden：`internal/research/context/testdata/context_v1.golden.json`。
- 一致性测试：`internal/research/context/schema_test.go`；覆盖 JSON 可解析、DTO JSON Tag 与 Schema properties/required 双向一致及 schema_version。
- 实际 Schema Validator 测试已接入；CI 强制门禁、跨模块 Contract Test 和跨字段语义由 Go Validator 覆盖的联合验收仍未完成。


## Context v1 Schema / Domain 一致性补充

- v1 当前只接受 `schema_version=1.0.0`，常量定义于 `context.SchemaVersion`。
- Context 与 Section 的四个时间字段要求零时区偏移；JSON Schema 进一步要求 RFC3339 date-time 字符串以 `Z` 结尾。
- 新增版本门禁、非 UTC 时间测试和 Schema 时间字段约束测试；均尚未执行。


## Context v1 JSON Schema Validator

- 复用根模块已有 `github.com/santhosh-tekuri/jsonschema/v6`，不新增依赖。
- `TestContextJSONSchemaValidatesGoldenAndRejectsInvalidPayloads` 实际编译 Draft 2020-12 Schema，验证 Golden，并拒绝缺字段、null 数组、未知枚举、额外字段及非 UTC 时间。
- `TestContextJSONSchemaRejectsNullNestedObject` 覆盖必需对象为 null 的情况。
- 该 Validator 测试尚未执行；GitHub Actions 强制门禁和跨模块 Contract Test 仍待接入。


## Research Application 首批接口

- `application.Port` 暴露 `CreateContext`、`AddEvidence`、`GetContext`、`GetEvidence`，调用方只依赖 Application 自有命令/引用/DTO；Domain 类型仅在 Application 内部映射。
- Context/Evidence 标识由 Research 服务端生成；Evidence 内容指纹由服务端计算，调用方提供的 ID/指纹不会被信任。
- `AddEvidence` 和 `GetEvidence` 均必须通过 `EvidenceAuthorizer`，由来源 Owner Adapter 执行当前授权/撤销检查；读取在授权成功前不得向调用方返回内容，不能以历史 AuthorizationRef 快照校验替代。
- `ListEvidence` 尚未实现：当前 Evidence Repository Port 只有 Get/Append，且项目契约对 Cursor 查询尚无冻结 Schema；不提供假分页接口。


### Event Application

- `EventPort` 提供 `CreateEvent` 与 `GetEvent`，对外只暴露 Application DTO。
- 创建时由服务端生成 EventID，版本固定为 1；精确版本读取在返回前重新执行领域校验。
- 生命周期版本追加和 Outbox 暂不接入，等待 changed_at / INT-CTR-001 与事务边界冻结。


### Claim / Hypothesis Application

- `ClaimPort` 提供 `CreateClaim` 与 `GetClaim`；Hypothesis 使用 `epistemic_type=hypothesis`，复用同一 Claim 聚合。
- 创建时由 Research 服务端生成 ClaimID，版本固定为 1；领域校验强制 Hypothesis 验证方法、预期观察和失效条件。
- `UpdateClaim` 仅支持基于 `ExpectedVersion` 的追加修订，并执行 Domain 状态迁移门禁；终态后修订仍关闭，`supersedes_version` 字段缺口继续保留，不允许借此表达跨 Claim 替代关系。


### Research Claim Application 状态迁移用例（2026-11-09）
- 增加 UpdateClaim Application 用例，调用方必须提交 ClaimID 与 ExpectedVersion；Research 读取指定前版并构造连续的新版本，不允许调用方覆盖聚合 ID 或版本号。
- 新版本在写入前执行 Domain ValidateNextVersion，复用已冻结的 proposed→testing、testing→终态/unknown、同状态修订规则；终态和 unknown 不开放恢复路径。
- Repository Append 继续负责事务内最新版本连续性与并发冲突检测；Application 的前读不替代 Repository 的原子约束。过期 ExpectedVersion 会因精确版本不存在或 Repository 冲突而失败。
- 本批增加成功迁移与旧版本重放拒绝测试；Go 测试及 MariaDB 并发验收尚未执行。


### Research Diagnostics Application Port（2026-11-09）
- 新增 Application Diagnostics Port，提供 Context/Event/Claim 精确版本的 Check 构造方法；读取并校验对应 Repository 快照后，复用既有 Research Domain Diagnostics Adapter。
- Application 只构造 Platform Check，不自行执行调度；并发上限、单项/总超时、取消及报告聚合继续由调用方交给 Platform Runner。
- Check 的 Required 标记由调用方明确传入；诊断仍是本地结构/Context 质量快照检查，不代表跨模块数据完整性、Evidence 当前授权或整个研究流程质量结论。
- Evidence Check 暂不提供：来源 Owner 实时授权与撤销检查必须由 Owner Adapter 执行，不能以本地快照诊断替代。
- 新增精确版本读取和 Check 构造用例；Go 测试及跨模块接线尚未执行。
