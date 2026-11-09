# Research 模块

- 分支：`feat/research`
- 代码复核基线：`51cde38549d8861a71cf5333ed35a56649bd872e`（2026-11-09，合并 Data 与 integration 更新）
- 已聚合到 integration：`a5ac63c2caa71153332d1229475139fd8b637c53`（PR #53）
- 代码范围：`internal/research/**`
- 文档范围：`doc/research/**`
- 任务与验收权威记录：[02-任务拆分与验收](./02-任务拆分与验收.md)
- 领域设计：[01-研究领域模型与证据体系](./01-研究领域模型与证据体系.md)
- 字段语义及授权链：[03-Context与Evidence字段语义及授权链补充](./03-Context与Evidence字段语义及授权链补充.md)

## 模块职责

Research 拥有 Context、Evidence、Event、Claim、Hypothesis、Scenario 等研究语义模型。

- Data：行情、财务、证券主数据、PIT 数据及其质量。
- News：新闻采集、原文、身份、版本、授权和只读证据查询。
- Quant：确定性数值计算及其版本化结果；Research 不直接依赖或调用 Quant。
- Researchflow：Run/Stage/Attempt 生命周期、单次 Run 的 Context Assembly、分区选择、Token Budget 和裁剪。
- Platform：提供 Fault、Storage、Diagnostics Runner 等公共基础能力，不接管 Research 领域规则。

Research 不采集新闻、不拥有行情事实、不重算 Quant 指标，不依赖 Quant 或 Researchflow。跨模块输入必须经公开 Application Port/Adapter；不得访问其他模块私有 Repository、SQL 或 Domain。Research 不因本地 Fixture 验收而宣称真实 Data/News 接入通过。

## 当前实现

| 任务 | 当前实现 | 未完成边界 |
|---|---|---|
| RSC-001 Context | Domain、结构与版本链校验、v1 Golden/Schema Validator、Application Create/Get、SQL Repository | 完整 CI 门禁、消费方 Contract Test、远端引用与授权版本验证 |
| RSC-002 Evidence | Domain、JCS/SHA-256 指纹、严格 PIT、AuthorizationRef 绑定、Application Add/Get、SQL Repository | News 生产 Adapter、Owner 当前授权 E2E、ListEvidence 查询契约 |
| RSC-003 Event | Domain、SourceTime、版本追加、生命周期、事务 Outbox、Application Create/Update | 多来源归并/拆分、规范化时间规则版本、Server Relay 接线 |
| RSC-004 实体关联 | 未实现 | 等待 Data 提供稳定 Security/Industry 引用及公开 Application Port |
| RSC-005 Claim/Hypothesis | Domain、认识论校验、状态迁移、版本追加、Application、SQL Repository | Evidence/Scope 真实引用解析、supersedes_version 契约缺口、真实样本 |
| RSC-006 Scenario | 仅有部分引用模型 | 等待 typed expected_value 与 ConditionGroup 机器契约 |
| RSC-007 Storage/Outbox | Context/Event/Claim/Evidence Repository、Migration v1-v5、事务 Outbox、领取/租约/重试/确认/拒绝 | 专用 MariaDB 实测、组合根 Migration 注册、故障注入验收 |
| RSC-008 Diagnostics | Context/Event/Claim/Evidence 快照 Check；Evidence Check 通过 Application GetEvidence 先执行 Owner 当前授权复核，并执行严格 PIT；复用 Platform Runner | 真实 Owner Adapter、完整 CI 执行、Researchflow FLOW-009 Facade 接线 |

## 诊断边界

- Context：结构非法或必需分区缺失却标 valid 为失败；可选分区缺失且原因明确为降级；知识截止/引用版本缺少可比较证据时不可比较。
- Evidence：结构或指纹错误、明确授权拒绝/撤销、明确 PIT 越界为失败；来源质量受限且 Owner 明确允许为降级；available_at 未知、授权服务不可用或状态 unknown 为不可比较/不可用。严格 PIT 不得将不可比较证据纳入有效事实集合。
- Event：结构、来源时间引用或生命周期版本非法为失败；来源时间冲突但完整保留为降级；缺少时间/精度证据时不可比较。
- Claim/Hypothesis：结构、状态迁移或正反 Evidence 冲突规则非法为失败；证据覆盖不足但引用可解析为降级；引用版本或知识时点不可比较时不得推断结论。
- Platform Runner 负责 Check 调度、并发、超时和聚合；Research 只提供领域 Check，不建设 Run/Stage/Attempt 或第二套诊断 Runner。

## 当前验收结论

- 本次代码复核以 `51cde385` 为基线；文档对齐提交为 `05b3400`，并已通过 PR #53 聚合到 integration。
- 截至本次检查，未发现以 integration 合并提交 `a5ac63c` 为 head 的新 Workflow Run；旧 Research 分支 Workflow 不能作为当前合并提交的构建证据。此前失败项包含跨模块 gofmt、News MariaDB 和 Market-data 镜像构建，不得误判为 Research 代码失败。
- Research 的新增领域、Application、Diagnostics 与 MariaDB 集成测试尚未在当前 HEAD 完整执行；没有真实 Data/News Owner Adapter 和真实来源证据，因此不标记真实数据验收通过。
- 构建/集成验收按项目约束在 `origin/integration` 聚合后执行；不得直接修改 integration 分支。

## 维护规则

1. 先核对现有代码和公共 Platform 能力，已有能力只修缺陷。
2. 新接口必须对应明确调用场景；新字段必须记录来源和语义。
3. 不新增无真实调用场景的查询/API，不为 WebUI 提前扩展 DTO。
4. 阶段状态、诊断标准和测试证据更新在本目录现有文档；不重复创建进度文档。
5. 契约字段定义以项目级契约目录和生产方 DTO 为准；本模块文档只记录 Research 的消费规则与实现进度。
