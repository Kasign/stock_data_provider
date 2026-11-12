# Research 模块

- 分支：`feat/research`
- 本轮文档复核基线（修改前 HEAD）：`bc3d48510de567610442f1bc469e709ac083e051`。
- 最近代码修复提交/验收 SHA：`0129675185ab29905b0ef14a21e1d564961e3dde`。
- 本轮文档复核仅修改 `doc/research/`；具体问题、历史状态口径及修订范围见 [02-任务拆分与验收](./02-任务拆分与验收.md) 的“文档复核记录”。
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

## 当前实现与剩余边界

| 任务 | 当前实现与验收 | 未完成边界 |
|---|---|---|
| RSC-001 Context | Domain、结构/版本链校验、v1 Golden/Schema Validator、Application Create/Get、SQL Repository；Research 全模块 gofmt、go test、race、vet 已通过 | 消费方 Contract Test、远端引用与授权版本验证 |
| RSC-002 Evidence | Domain、JCS/SHA-256 指纹、严格 PIT、AuthorizationRef 绑定、Application Add/Get、SQL Repository；本地授权调用边界已测试 | News 生产 Adapter、Owner 当前授权复核与内容读取 TOCTOU、available_at 来源证据及授权续期历史语义 |
| RSC-003 Event | Domain、SourceTime、版本追加、生命周期、事务 Outbox、Application Create/Update；补齐 time_normalization_rule_version、derived_from_refs 字段及结构校验，并在同一事务内校验精确版本存在性与引用图 | 多来源归并/拆分、News Owner Adapter |
| RSC-004 实体关联 | 尚未实现 | 等待 Data 提供稳定 Security/Industry 引用、版本/时间语义及公开 Application Port |
| RSC-005 Claim/Hypothesis | Domain、认识论校验、状态迁移、版本追加、supersedes_version 字段及当前版本头校验、Application、SQL Repository | Evidence/Scope 真实引用解析、真实研究样本 |
| RSC-006 Scenario | 新增 Scenario/Condition/ConditionGroup 领域模型，expected_value 限定为标量或一层标量数组，递归 ConditionGroup 校验按 DEC-033 执行 | Quant Metric Registry/结果引用由 Quant Owner 校验；Scenario Application/Storage 接入待完成 |
| RSC-007 Storage/Outbox | Context/Event/Claim/Evidence Repository、Migration v1（初版最终结构；无 v2-v5 兼容迁移）、事务 Outbox、领取/租约/重试/确认/拒绝；最终结构初始化测试待本轮 CI 验证 | 根 Server Migration 注册、Server→Notification Relay 接线、授权续期历史留档语义 |
| RSC-008 Diagnostics | Context/Event/Claim/Evidence 快照 Check、Run PIT、脱敏和快照隔离；历史 Research CI 曾通过；Evidence Check 通过 Application GetEvidence 执行 Owner Authorizer 调用 | 真实 Owner Adapter、Researchflow FLOW-009 Facade 和 Runner 集成 |

## 诊断边界

- Context：结构非法或必需分区缺失却标 valid 为失败；可选分区缺失且原因明确为降级；知识截止/引用版本缺少可比较证据时不可比较。
- Evidence：结构或指纹错误、明确授权拒绝/撤销、明确 PIT 越界为失败；来源质量受限且 Owner 明确允许为降级；available_at 未知、授权服务不可用或状态 unknown 为不可比较/不可用。严格 PIT 不得将不可比较证据纳入有效事实集合。
- Event：结构、来源时间引用、available_at 晚于 system_as_of 或生命周期版本非法为失败；来源时间冲突但完整保留为降级；缺少时间/精度证据时不可比较。
- Claim/Hypothesis：结构、状态迁移或正反 Evidence 冲突规则非法为失败；证据覆盖不足但引用可解析为降级；引用版本或知识时点不可比较时不得推断结论。
- Platform Runner 负责 Check 调度、并发、超时和聚合；Research 只提供领域 Check，不建设 Run/Stage/Attempt 或第二套诊断 Runner。

## 当前验收结论

- 历史 Workflow Run [42616788237](https://github.com/Kasign/stock_data_provider/actions/runs/42616788237) 对旧代码 SHA `0129675185ab29905b0ef14a21e1d564961e3dde` 的 Research gofmt、go test、race、vet 与旧版 MariaDB 迁移测试通过；不代表本轮新提交已验收。当前 `.github/workflows/go-test.yml` 的 push 分支列表不包含 `feat/research`，本轮提交没有自动触发该 Workflow。
- 同一 Workflow 的整体 conclusion 是 failure：根模块 `Test .` 被 Marketdata、News、Notification、Quant 的 gofmt 差异阻断。该失败不属于 Research 文件；因此不能表述为全仓 Go Workflow 通过，也不能把根模块全量编译/测试视为已验收。
- 此前 Workflow 的 MariaDB 结果仅证明旧 v1-v5 迁移链；本轮删除该链后，最终结构初始化与当前 Repository 集成尚待重新验收。
- 本轮代码复核发现并修复 Event.available_at PIT 边界；本次进一步按 RSC-CTR-002 补齐 Event 时间规则版本/派生引用、Claim supersedes_version 和 Scenario 闭合条件模型。新改动的 CI 验收状态以对应提交后的 Workflow 为准。
- 尚无真实 Data/News Owner Adapter 与完整来源证据；跨模块接入及真实数据验收仍未完成。根 Server Migration 注册和 Notification Relay 由 Server 组合根负责。

## 维护规则

1. 先核对现有代码和公共 Platform 能力，已有能力只修缺陷。
2. 新接口必须对应明确调用场景；新字段必须记录来源和语义。
3. 不新增无真实调用场景的查询/API，不为 WebUI 提前扩展 DTO。
4. 阶段状态、诊断标准和测试证据更新在本目录现有文档；不重复创建进度文档。
5. 契约字段定义以项目级契约目录和生产方 DTO 为准；本模块文档只记录 Research 的消费规则与实现进度。


### 初版存储结构收敛（2026-11-12）

- Research 当前为初版且无历史业务数据，Migration 已收敛为单个 version 1，直接创建最终结构。
- 移除 v2-v5 中为既有表回填 PIT 精度、扩大版本列、升级 Outbox 状态所做的过渡迁移；最终字段类型直接写入初始 DDL。
- 删除 v1→v5 历史数据升级集成测试，保留最终结构初始化、Repository、并发、PIT 与 Outbox 行为测试。
- 不创建 legacy/v2 表，不做数据复制或兼容迁移。该变更要求 Research 数据库按初版空库重新初始化；不得对已有非空数据库直接执行。
