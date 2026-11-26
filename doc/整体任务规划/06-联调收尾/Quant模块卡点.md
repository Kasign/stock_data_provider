# Quant 模块卡点

更新时间：2026-11-23  
维护分支：`repair_all`（聚合验收）  
责任模块：Quant  
说明：本文件登记需要其他模块或项目级决策方提供输入、装配或验收的事项。模块内详细依赖见 `doc/quant/依赖/01-跨模块依赖与解锁条件.md`。

> **口径收敛声明（2026-11-26）：** DATA-CTR-005/007、QNT-CTR-003 当前均为 `deferred`，以 `doc/contract/PLAT-002-项目级契约冻结登记簿.md` 第 77～79 行为准。下表 DATA-CTR-005/007 卡点针对的是**通用 OHLC Decimal 精度路径**，仍成立；但首批 Close-only Factor/SMA20 复用现有已授权 Close Series Snapshot（精确 Close、双指纹 SnapshotRef、Manifest、授权读取），该窄化路径不等待通用 DTO，其生产门禁归 QNT-CTR-003 自身 Factor Schema/Golden、Decimal Registry/精度策略、生产 Facade 与 Researchflow Stage 接线。QNT-015 的 Snapshot Adapter 已有 SMA20 先例，剩余为 Factor 正式接线；不得据旧口径称"无可用 Snapshot"。

## 1. Data 模块卡点

| 卡点 | 对 Quant 的影响 | 责任方 | 解锁条件 | Quant 解锁后动作 |
|---|---|---|---|---|
| DATA-CTR-005：DailyBars Decimal v2 | 价格类 Indicator/Factor 无法证明原始十进制精度；统计指标生产误差预算缺少可信输入域 | Data | 提供精度可保真的 DTO、scale/舍入、单位/缺失语义、bar interval、交易日历版本及真实 DTO/Golden；满足登记簿重新进入条件 | QNT-001/002 按 metric_code 完成生产精度策略、Entry 和端到端误差验证 |
| DATA-CTR-007：Snapshot/Evidence | 缺少生产输入快照身份、内容寻址与可追溯证据 | Data | 冻结 Snapshot Manifest、引用与访问契约，并提供可验证实现 | QNT-015 实现 Data Snapshot Adapter 和质量门禁 |
| PIT/复权/交易日历/停牌事实 | 无法完整构造 input_requirements/quality_policy，无法确认有效样本及时间口径 | Data | 提供 available_at、source_version、adjustment_basis/reference_date、版本化交易日历及停牌/缺失区分 | Quant 建立真实输入质量验证；不以数组长度代替有效样本 |

Data v1 DailyBar OHLC/Amount/ChangePercent 为 float64，不能通过格式化成 String 恢复源精度。Quant 不复制 Provider、Snapshot 存储、交易日历或主数据事实。DATA-CTR-005/007 未解锁前，不发布伪真实结果。

## 2. Server / Platform 模块卡点

| 卡点 | 当前 Quant 端口/能力 | 责任方 | 解锁条件/交付 | 后续动作 |
|---|---|---|---|---|
| 生产 Storage、事务、Migration | Quant 自有 MySQLRepository、Migration v1；Server 已注册 quant namespace | Quant 维护结果存储；Server 注入共享 DB/生命周期；Platform 提供 Migration Runner | 已有不可变 Save、幂等/冲突/并发 MariaDB 测试；Root 注册已存在 | 继续验证 Root 生产启动与实际依赖注入；不得重复建设结果 Repository |
| 可信 Principal 与授权撤销一致性 | PrincipalResolver Port、GetResult 授权用例 | Server/Platform | 从可信认证上下文注入 Principal；提供策略快照、撤销复核及一致性边界 | Integration 验证授权读取和撤销竞态 |
| Access Audit 持久化 | AccessAuditor Port、MySQLAccessAuditor、Migration v2-v4（含 RunRef、content_level、授权/策略快照 JSON、decision_fingerprint） | Quant 实现追加审计；Server 注入 DB/Clock | Go Test Suite、Integration CI 与 Quant 专项 CI 已验证 MariaDB 持久化；审计失败由 GetResult fail-closed | 审计保留/归档/删除授权仍待治理决议；补 Root 生产注入验收 |
| Clock / Diagnostics / Fault | Quant 复用 Platform 公共语义 | Server 注入 Clock；Platform 维护通用能力 | 提供可信 UTC Clock 和统一公共诊断/故障能力 | Quant 只消费，不复制通用框架 |

Quant 不在 `internal/quant/**` 实现 SQL/Redis、通用 Storage、认证、授权、审计持久化或通用诊断框架。Server 是唯一组合根。

## 3. Researchflow 模块卡点

| 卡点 | 责任方 | 解锁条件 | 后续动作 |
|---|---|---|---|
| 计算 Facade 消费契约 | Quant 提供公开 Application Port；Researchflow 消费 | QNT-001/002 生产结果契约和 QNT-014 Facade 具备可发布条件 | Researchflow 通过公开 Port 调用，不依赖 Quant 内部包 |
| CallerRef / Envelope / Payload 消费验收 | Researchflow | 传播可信 CallerRef、校验已注册 Payload、消费完整 Envelope | Integration 以精确 SHA 验证实际 Stage Provider 链路 |

当前 Quant 只有 GetResult 授权读取用例，没有 Indicator/Factor 计算 Facade。不得把纯函数拼装成伪生产 Provider；Indicator/Factor 未完成 Schema/Golden/Validator/Registry 前不得开放 Envelope。

## 4. Quant 当前可独立推进

- QNT-013：完善模块内 Manifest、Record、Envelope、MemoryRepository 与公开读取契约；生产 Adapter 交由 Server/Platform。
- QNT-001/002：推进不依赖权威外部输入的代码契约、测试和映射审计；不猜测单位、精度、舍入或质量策略。
- QNT-010：维护 fail-closed 风险执行框架；没有权威规则目录和真实事实前，不自创业务阈值。
- QNT-014：生产 Facade 受 QNT-001/002 阻断，不提前拼装。
- QNT-015：DATA-CTR-005/007 解锁前保持阻断。

## 5. 状态治理

- 外部依赖未满足时标记 blocked/deferred/not_comparable，不把 skipped 当 pass。
- 责任方交付必须关联契约编号、文档或代码路径、精确 SHA 和验收证据。
- Quant 不修改其他模块分支；跨模块实现由对应 Owner 完成，跨模块验收由 Integration/Server 组织。


## RepairAll Quant→Researchflow→Server 结果链路审查（2026-11-19）

- 审查基线：repair_all SHA b52c8afde2f2e32fca498a78987d34f96fba4335。
- Quant application 当前仅提供 GetResult；未发现计算 Application Facade。QNT-014 保持未完成，不能把纯函数组合成生产计算入口。
- Quant result 仅有 Save Repository Port 与 MemoryRepository；GetResult 所需 AuthorizedResultReader、PrincipalResolver、AccessAuditor 仍为注入端口，未发现生产 Storage/授权/审计 Adapter。QNT-013 生产闭环未完成。
- Envelope 仅接受 trend/risk 1.0.0；IndicatorPayload 类型存在但未注册，Factor Payload 未注册。与当前正式 Registry/Schema/数据契约状态一致，保持 fail-closed，不放宽 DecodePayload。
- 解锁责任：Quant 完成 QNT-001/002 正式 Registry/Payload 和 QNT-014 Facade；Server/Platform 完成生产 Repository、授权一致性、审计持久化与 Root 注入；Data 解锁 DATA-CTR-005/007 后才可推进相应输入链路。


## RepairAll QNT-013 模块内取消传播补充（2026-11-20）

- `GetResult.Execute` 已覆盖 PrincipalResolver、可信 Clock、授权 Reader 返回后的取消检查；本轮补齐 Auditor 回调返回后的取消检查。
- 精确 SHA `0a10cb08daa7dcedc6707349ab96d576472bf7a0` 的完整 Integration CI Run [44661192496](https://github.com/Kasign/stock_data_provider/actions/runs/44661192496) 全项 success。
- 该取消传播子项 CLOSED，不改变跨模块责任：生产 Repository/事务/Migration、可信授权 Reader 和 AccessAuditor 持久化仍由 Server/Platform 装配与验收。


## RepairAll QNT-013 生产链路复核更新（2026-11-20）

- 最新复核基线：repair_all HEAD `487edf052b44721d5b8ba3c5429e5d200ec24772`；代码验收基线 `0a10cb08daa7dcedc6707349ab96d576472bf7a0` 的 Integration CI Run [44661192496](https://github.com/Kasign/stock_data_provider/actions/runs/44661192496) 全项 success。
- 已核查 `internal/quant/result`、`internal/quant/application`、Server Bootstrap、Migration Registry、Researchflow Composition/Runtime：目前仍只有 Save Port/MemoryRepository 和 GetResult 授权读取 Port；无生产 SQL Repository、Quant Migration、PrincipalResolver/AuthorizedResultReader/AccessAuditor Adapter，也无 Root DI。
-（截至本节旧复核基线）Server Migration Registry 尚无 Quant namespace；后续 Quant 已提供模块自有 SQL Repository/Migration，Server 已正式注册 `quant` namespace。不得通过空注册或 Server 自建 SQL 表绕过模块存储归属。
- 当前 `GetResult` 的用途为 `quant_analysis`、内容级别 `full`。GOV-CTR-006 已明确 policy_owner=quant、MVP subject_ref=Researchflow service principal、resource_ref=result_id。当前缺口是 Quant Owner 策略规则/存储/签发、策略更新与读取竞态实现，以及审计保留/失败处置；不得默认 Allow，不得把 Owner Session 身份直接当成 Quant 资源授权。
- 解锁顺序：① Quant Owner 按 GOV-CTR-006 实现策略规则、不可变 PolicySnapshot 签发/读取、撤销与读取竞态门禁，并明确审计保留/失败处置实现；② Quant 实现生产 Repository/Migration，Server/Platform 复用现有 Storage 并完成 Root 注入；③ Quant/Server 完成 MariaDB 并发/冲突/撤销竞态/审计失败集成验收；④ 具备明确真实消费场景后再接 Researchflow Quant Stage。
- QNT-013 模块内取消传播子项已 CLOSED；生产闭环仍 OPEN。QNT-014 Facade 与 Researchflow Stage 不因本次复核提前开放。


## RepairAll QNT-013 持久化结构审查补充（2026-11-20）

- 已核对 Quant Manifest/Record/Save Port、Platform Storage/Migration Runner、Server Migration Registry 和 Research SQL Repository 范式。
- 结果存储建议最小单表：result_id 唯一主键、manifest_fingerprint、result_fingerprint、manifest_json、payload_json、created_at；字段直接来源于现有 Manifest/Record/Repository 契约。该模型尚待 MariaDB 实现验证，不视为已冻结 Schema。
- Save 必须依赖 DB 唯一约束原子仲裁；重复相同指纹幂等成功且不覆盖首次 created_at，异内容冲突 ErrResultConflict；禁止 REPLACE/覆盖 UPSERT/无锁先查后插。
- Quant 拥有自己的 Storage Adapter/Migration；Server 只注册 Quant Migration 和注入；Platform 复用现有连接池/迁移能力。没有查询用例前不新增二级索引。
- 授权读取和 Access Audit 不得并入 Save Repository。AuthorizedResultReader 需策略快照与结果读取一致性边界；AccessAuditor 写入失败必须拒绝释放结果。
- GOV-CTR-006 已明确授权 Owner 和主体/资源/用途映射；尚缺 Quant Owner 策略规则/持久化/签发、当前策略复核与撤销竞态实现，以及审计保留/失败处置。生产授权 Reader 不得先行伪实现。
- 当前结论：持久化设计审查完成；SQL Schema/Adapter/Migration 未实现；QNT-013 生产闭环继续 OPEN。下一步核对 GOV-CTR-006 和 Platform Authorization 已有能力，避免重复建设。


## RepairAll QNT-013 结果存储实现更新（2026-11-20）

- Quant 已新增模块自有 `internal/quant/storage`：MariaDB `MySQLRepository.Save` 与 `quant` namespace Migration；Server 仅注册 Migration，Platform 复用现有连接池和 Migration Runner。
- Save 已按不可变内容寻址语义实现：写入前验证 Manifest/Payload；DB 主键负责并发仲裁；重复写入验证存储记录完整性和两个指纹；同内容幂等，不覆盖首次创建时间；异内容冲突；损坏行拒绝。
- MariaDB Integration 覆盖重放/等价 JSON/created_at、16 并发写入、损坏行、取消；Integration CI #551 [44664065668](https://github.com/Kasign/stock_data_provider/actions/runs/44664065668) 在精确 SHA `7f1162ba2b6472c4653b108232862f7e9d5af1cd` 上全项 success，Quant MariaDB 四项逐例 PASS 门禁通过。
- 当前仅结果写入持久化完成，AuthorizedResultReader、PolicySnapshot 当前策略复核/撤销一致性、AccessAuditor 仍未实现；不得把写入能力误报为 QNT-013 完成。
- 授权语义依 GOV-CTR-006：policy_owner=quant、Researchflow service principal、resource_ref=result_id、purpose=quant_analysis、content_level=full；Quant Golden 进一步固定策略组合为 `policy_type=quant_result_access`、`policy_ref=quant-result-access`（示例版本 `1.0.0`）。仍需该策略的真实规则与可撤销当前版本存储/签发；Golden 示例不等于生产 Allow 规则。可借鉴 News 的不可变版本+当前指针+事务行锁模式，但不可复制 News 领域策略。


## 2026-11-20 Quant 模块分支 QNT-013 更新（尚未合并到 repair_all）

- feat/quant 已同步 Quant 自有 MySQLRepository 和 quant namespace Migration v1；后续新增 MySQLAccessAuditor 与 quant_access_audit Migration v2，审计表只追加决策元数据，不保存结果 Payload。
- Quant module validation Run 44666849911 对精确代码 SHA 160541a316e3c9c621451d0c82b6cd5dfbd1327e 全项 success；Go Test Suite Run 44666849903 同 SHA success。
- 该实现目前在 feat/quant 分支，未合并到 repair_all；不得据此宣称聚合分支或 Server Root 已具备完整生产链路。
- QNT-013 仍 OPEN：Policy Payload Schema/真实 Allow-Deny 规则/策略发布权限/版本指针和撤销线性化语义缺失；AuthorizedResultReader 未实现；Server Root 的 Repository/Reader/PrincipalResolver/Auditor 注入未完成。GOV-CTR-006 Golden 不是生产 Allow 规则。
- Access Audit 保留、查询、归档、删除授权和生产访问控制没有契约/运维决议；当前只完成追加写入，不自行增加清理策略。


## RepairAll QNT-013 当前代码状态校准（2026-11-23）

- 当前 `repair_all` SHA `6139bcbe7b40bc82564783ffb4af41710be32b09` 已有 Quant 自有 MySQLRepository、MySQLAccessAuditor、Migration v1-v3；v3 将 RunRef 写入审计记录。Server Root 已注册 Quant Migration namespace。
- 仍缺 Quant Policy Owner 的规则/策略版本持久化/签发撤销、AuthorizedResultReader 生产实现和 Root 可信 Principal/Reader/Auditor/Clock 完整注入。
- Researchflow StageOutputResource 只证明 Stage Output 属于某 Run/Stage/Attempt；没有 Quant Stage Provider 和显式 Quant result_id 输出契约，不能证明 Run→QuantResult 关联存在。
- Q-01 继续 BLOCKED；GOV-CTR-006 Golden 不是生产 Allow 规则。只有在策略治理权限和关联契约明确后，才进入策略存储、撤销线性化和 MariaDB 并发验收。


## RepairAll QNT-013 授权与审计证据更新（2026-11-23）

- 结果访问授权校验现在要求 PolicySnapshot 包含固定领域身份 `quant_result_access / quant-result-access`；policy_version 仍由 Owner 管理，不硬编码 Golden 的示例版本。
- AccessAudit Migration v4 已持久化完整 AuthorizationRef/PolicySnapshot JSON、PolicySnapshotRef、DecisionFingerprint 和 content_level；不保存结果 Payload。MariaDB 集成测试明确执行且未跳过。
- Go Test Suite [45102027281](https://github.com/Kasign/stock_data_provider/actions/runs/45102027281)、Integration CI [45102027283](https://github.com/Kasign/stock_data_provider/actions/runs/45102027283)、Quant module validation [45102582473](https://github.com/Kasign/stock_data_provider/actions/runs/45102582473) 均通过。
- Platform 已有 Owner Bearer + `platform:admin` 的 `OwnerAccess()` 管理认证路径；Owner Session 不具备该管理权限。是否将其正式绑定为 Quant Policy 发布/撤销权限，仍需治理决议。
- Q-01 仍 BLOCKED：Policy Owner 规则/版本/current pointer/发布撤销、生产 AuthorizedResultReader、Researchflow 强类型 Run→QuantResult 关联和 Server Root 完整注入尚未完成。


## RepairAll 2026-11-23 QNT-013 写入基础能力与 QNT-014 真实 Facade 边界

- 复核基线：`repair_all` SHA `ceb26596fd365e6c49aa0767de9e8d9f76850d5c`。
- 新增的 `result.BuildRecord` 和 `application` 包内私有 `resultPersister` 仅持久化已经完成计算的 Trend/Risk 强类型 Payload；不执行计算、不负责活动算法版本选择、可信输入 Snapshot 校验、Caller/Use Case 幂等作用域或 Run 关联。该能力归入 QNT-013 结果写入基础，不代表 QNT-014 Facade 完成。
- Trend 目录目前是 DTO/结构校验，没有生产趋势推断算法；Risk 目录目前是规则执行/聚合框架，RuleEvaluator 由上层注入，缺少权威生产规则目录、真实事实读取和完整规则覆盖。不可把现有纯 DTO/框架包装为生产计算结果。
- Researchflow StageOutput 已有精确 Registry、Envelope 校验、StageOutput Resource 持久化及 Attempt/Stage 原子提交；但无 Quant Stage 的正式业务类型、Schema、Provider、Registry Entry。无真实 Stage 合同前，不得发明 StageCode 或把通用 resource_ref 解释成 Quant result_id。
- GOV-CTR-006 对 Quant 的固定映射为可信 Researchflow service principal、精确 result_id、purpose=quant_analysis、content_level=full；每次读取必须按 Owner 当前策略复核。现有 Enabled 只是策略总开关，不是主体/资源授权。
- 当前 Q-01 仍 BLOCKED，QNT-014 仍 NOT_STARTED。解锁依赖：①明确真实计算用例与输入事实来源；②有可验证的 Quant 结果生产 Facade；③Researchflow 定义并注册该真实 Stage 的强类型输出，提交事务中保存 result_id 与 Run/Stage/Attempt 关联；④Quant Reader 经公开关联 Port 验证当前 Run 关联，并在策略撤销线性化边界内读取；⑤Server 完成可信依赖装配和 MariaDB 同 Run/跨 Run/无关联/撤销竞态/审计失败验收。
- 不进入 Data D2；QNT-015 所需 DATA-CTR-005/007 输入质量门禁仍按原计划等待 Data 契约解锁。


- 补充边界：结果持久化步骤已收敛为 Quant Application 包内私有 `resultPersister`，不向 Server/Researchflow 暴露为独立业务用例；未来必须由具备可信 Caller、正式 Registry、真实输入门禁和幂等语义的 QNT-014 Facade 调用。


## 2026-11-23 QNT-013 授权撤销一致性推进

- 已建立 `ResultAccessPolicyReadGuard`，通过 Quant Policy Head 行锁把当前策略读取与授权读取回调绑定在同一线性化区间。
- Publish/Revoke 和受保护读取争用同一 Head 行锁；撤销在读取回调结束前不能提交。MariaDB 集成测试覆盖“读持锁期间撤销等待、读完成后撤销推进版本”。
- 该 Guard 是生产 Reader 的基础构件，不是 Reader 本身。当前仍缺 Researchflow 对外 Run-result 强类型关联 Port 和不可变关联约束，不能用跨模块任意 SQL 或主体级 Allow 替代。
- 后续：先完成当前 SHA 的 Quant Fuzz/Race/Vet 与全仓 Go/Integration CI；然后基于 Researchflow 正式关联 Port 实现 AuthorizedResultReader，并确保策略锁覆盖关联检查与结果读取；再补审计失败、跨 Run、无关联、撤销竞态和 Server 装配验收。Q-01 未完成前不进入 Data D2。


- 本轮代码验收通过：`ccf3a04dc1ea1161b3fdf3991e1b9918ede6e679` 的 Quant module validation（Run `45110456094`）、Go test suite（Run `45110456110`）、Integration CI（Run `45110456093`）均 success；文档 SHA `aedc2844395422126288ceb791f1a99da20d3229` 的 Quant validation（Run `45110645391`）亦 success。
- MariaDB 实测 `TestMySQLResultAccessPolicyReadGuardSerializesRevoke` PASS，证明策略撤销会等待受保护读取回调结束。该结果仅验收锁保护机制，不代表生产 Reader、Run-result 关联和授权规则已完成；Q-01 仍 BLOCKED。


## 2026-11-23 Researchflow StageOutput 与 Quant 关联链路复核

- 复核基线：`repair_all` HEAD `9386366c71976e282a249d76c86ab7cc723d0689`。
- Researchflow 已有 StageOutput 不可变资源表（Migration v6）：每条资源记录包含 `run_ref`、`stage_ref`、`attempt_ref`、`resource_kind`、`stage_code`、`output_type`、`schema_version`、`schema_ref` 和原始内容；Stage/Run 通过复合外键约束归属，资源按 Attempt/resource_kind 唯一。Worker 在同一事务写入 Payload、QualitySummary 并更新 Attempt/Stage 引用。
- `GetStageOutputResource(ref)` 仅按不透明 ref 精确读取资源，不负责证明 Quant result_id 关联；不得扫描任意 JSON 或把通用 resource_ref 当作 Quant result_id。
- Server 当前正式装配的 Stage 仍只有 News Search。Researchflow Output Registry 的通用框架允许精确注册，但没有 Quant 的正式 StageCode、OutputType、SchemaVersion、Payload/QualitySummary Schema、Provider 和 Registry Entry。
- Researchflow 的 `resultvalidation.NewQuantPayloadRegistry` 已复用 Quant 自有解码器校验 trend/risk 1.0.0；`FromQuantEnvelope` 只负责验证和映射，不签发授权，也不建立 Run 关联。它不是 Quant Stage Provider。
- GOV-CTR-006 已冻结 Quant 映射：可信主体为 Researchflow service principal，资源为精确 Quant result_id，用途 `quant_analysis`，级别 `full`。Golden 只用于契约测试，不构成生产放行规则。
- 最新文档提交 `9386366c71976e282a249d76c86ab7cc723d0689` 的 Quant module validation Run `45110894577` 已 completed/success；全部步骤（gofmt、Quant 单元/MariaDB 集成、不可变结果用例未跳过、Manifest Fuzz、Race、Vet）通过。
- 当前可执行结论：Researchflow Storage 已有可复用的 Run/Stage/Attempt 归属载体，但缺少能把 Quant result_id 写入该载体的正式业务 Stage 契约与 Provider。现在新增关联 Port 会被迫猜测 Stage discriminator 或扫描任意 JSON，因此暂不实现。Q-01 继续 BLOCKED，Data D2 不启动。
- 下一步：继续核实 Quant 已冻结的输入/结果契约与真实可用算法用例，确认是否存在可落地的真实 Trend/Risk 生产 Facade；只有形成真实 Facade 与 Stage 输出 Schema 后，才实现 Researchflow 强类型关联 Port、Quant AuthorizedResultReader 和 Server 装配。


## 2026-11-23 Quant 结果读取主体门禁补齐

- 代码基线：`repair_all` SHA `5b8aba15a3148ca36c818ffc2b05ee609f5a62ae`。
- GOV-CTR-006 对 Quant 的 MVP 主体映射为可信 Researchflow service principal（`service:researchflow`）。此前 `GetResult.Execute` 只依赖注入的 PrincipalResolver 返回非空主体，再把主体传给 Reader；Application 本身没有强制该固定 MVP 主体。
- 已在 Quant Application 增加精确主体门禁：主体不是 `service:researchflow` 时，在调用 AuthorizedResultReader 前拒绝；记录 `denied / principal_not_authorized` 审计事件；审计写入失败仍 fail-closed，不返回结果内容。
- 新增单测 `TestGetResultRejectsNonResearchflowPrincipalBeforeReading`，验证非 Researchflow 主体不会调用 Reader、不会获得 Envelope，并写入包含主体和目标 result_id 的拒绝审计。
- Quant module validation Run `45115231452`（SHA `5b8aba15a3148ca36c818ffc2b05ee609f5a62ae`）已 success，包含 gofmt、Quant 单元/MariaDB 集成、不可变结果用例未跳过、Manifest Fuzz、Race、Vet。
- 同一 SHA 的 Go test suite Run `45115231420` 与 Integration CI Run `45115231437` 当时仍在运行；不得把 Quant 专项通过等同于全仓验收通过。
- 此修复只收紧 Quant Application 的调用主体，不代表生产 AuthorizedResultReader 已实现；当前仍缺 Researchflow 正式 Quant Stage 输出 Schema/Provider/Registry Entry、强类型 Run→result_id 关联查询、策略锁内的关联验证与结果读取，以及 Server 完整装配。Q-01 继续 BLOCKED，Data D2 不启动。


## 2026-11-23 QNT-014 生产输入与 Indicator Registry 复核

- 复核基线：`repair_all` SHA `c9e25e18732674eb766699776b3190513993cabb`。
- Quant 侧：`internal/quant/indicator/indicator.go` 仍是 float64 纯计算；`indicator/context.go` 只校验 CalculationContext 后调用纯函数，不加载输入快照、不执行 Data 质量过滤、不生成 QuantMetric。
- Registry 侧：`internal/quant/registry/entry.go` 提供 Entry/Registry 的通用注册、版本选择、冻结和校验框架；当前代码树没有首批 Indicator 正式 Entry 清单或生产注册装配。现有参数 Schema、Payload Schema、synthetic Golden 和样本数测试不等于生产 Registry 元数据。逐 metric_code 的 unit/value_type/precision/rounding/range/quality_policy/sample_count 仍不能从测试 Fixture 推导。
- Data 侧：`internal/marketdata/app/port.go` 的 `DailyBarsV2Snapshot` 暴露 interval、adjustment_basis、质量、覆盖率和 TradingCalendarRef；`DailyBar` 可携带 ExactValues。Data Application `mapDailyBar` 会把 `model.DailyBar.ExactValues` 及其 Provider 精度证据传到 Application DTO，说明内部精度侧车不是完全缺失。
- 但当前该内部 Snapshot DTO 没有 DATA-CTR-005 所要求的内容寻址 `snapshot_ref`、GOV-CTR-006 所要求的 `authorization_ref`，也没有可供 Quant 直接验证的完整 Snapshot Manifest。ExactValues 还允许为空；覆盖率/日历证据不能替代快照身份与访问授权。不得在 Quant 侧自行生成 Data SnapshotRef 或将 float64 格式化为 Decimal。
- 因此当前没有满足 QNT-001/002 生产元数据、可信 Snapshot 身份/授权和质量门禁的 Indicator 用例；QNT-014 不能安全落地。也不能只挑 SMA/EMA 等简单算法绕过共同输入契约，因为它们同样依赖真实交易日序列、复权口径、有效样本和可追溯输入快照。
- 本轮没有修改 Data 或 Quant 生产代码，没有新增 Facade/Registry Entry，也没有启动 Data D2。QNT-014 继续 NOT_STARTED；DATA-CTR-005/007 继续按契约状态阻断真实计算输入。
- 下一步由 Data Owner 按 DATA-CTR-005/007 完成 Snapshot Manifest、snapshot_ref/authorization_ref 与 Decimal/PIT DTO 的正式 Application Port；Quant 收到正式契约后，先选取可完整证明输入与数值语义的首个指标，完成正式 Registry Entry、生产 Metric 构造、Payload 注册和 Facade，再由 Researchflow 接入真实 Stage。
- 当前 SHA `c9e25e18732674eb766699776b3190513993cabb` 的 GitHub Commit Status 返回 `pending`、`total_count=0`、无已报告 Status；这不是 CI 通过证据。父提交的历史 CI 不作为当前 SHA 的验收结论。


## 2026-11-24 SMA20 Facade / Stage 代码状态校准

- 核对基线：`repair_all` SHA `25f66ae30470a76a734afffe974247f0401fd4ba`。
- 此前文档中“QNT-014 NOT_STARTED / 没有 Indicator 计算 Facade”的表述已被当前代码推进覆盖，不能继续作为现状：`internal/quant/application/create_sma.go` 已实现 `CreateSMAService`，通过 Data 的 `CloseSeriesSnapshotReader` 读取授权 Snapshot，按活动 Quant Registry Entry 校验 SMA20 参数、精度、质量和 PIT 时间，再执行精确十进制计算并通过 Quant 私有 `resultPersister` 持久化不可变 Indicator Result。
- `internal/quant/stage/sma20.go` 已实现 `SMA20Adapter`：在同一 UTC `SystemAsOf` 下调用 Data Snapshot Creator，再调用 Quant Application；输出携带明确 `result_id`、`snapshot_ref`、指标值/状态和 Coverage QualitySummary，并执行严格 JSON/Schema 语义校验。
- Data 侧已具备 `DailyBarsCloseSeriesSnapshotCreator`、MariaDB Snapshot Repository、授权签发和 Authorized Reader；`internal/marketdata/bootstrap.Module` 提供创建/授权读取入口。Snapshot Creator 从可信 Security Principal 上下文解析 caller，不接受调用方自报 CallerRef。
- 上述能力仍不是 Researchflow 生产 Stage：当前 `internal/server/bootstrap/researchflow_execution_composition.go` 只创建 News Search Stage/Template/Output Registry；`researchflow_runtime.go` 的 Worker 仍只注入 News Search Executor/Registry。没有 SMA20 的生产 Template、Executor 路由、Output Registry Entry、Server DI 和 Worker 集成验收。
- Server 生产接入的必要条件：①从 Researchflow Worker 的可信执行身份构造受限 Service Principal，并保证 Data Snapshot Creator 识别到的 CallerRef 与 Data 当前授权策略主体完全一致；②注入 Data Snapshot Creator/Authorized Reader；③注入 Quant 活动 Registry、MariaDB Repository 和可信 UTC Clock，构造 `CreateSMAService`；④将 Stage Adapter 注册到 Researchflow Executor 与精确 Output Registry，并定义正式参数 Schema、Payload/QualitySummary Schema、Golden 和 Template DAG；⑤通过真实 MariaDB 集成测试验证 Data 授权、Quant 持久化、Run/Stage/Attempt 关联和失败回滚/重试语义。
- Quant 的 `CreateSMAService` 当前不建立 Run 关联，也不提供完整结果授权读取。不得把 Stage Output 的通用资源引用直接当成 Quant `result_id`；Run→QuantResult 的正式强类型关联与策略撤销线性化读取仍须完成。
- 本次仅校准状态与执行门槛，不把 Adapter 单测等同生产接入，不把旧历史记录中的 CI 结果外推到当前 SHA。
