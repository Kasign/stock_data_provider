# Quant 模块卡点

更新时间：2026-11-23  
维护分支：`repair_all`（聚合验收）  
责任模块：Quant  
说明：本文件登记需要其他模块或项目级决策方提供输入、装配或验收的事项。模块内详细依赖见 `doc/quant/依赖/01-跨模块依赖与解锁条件.md`。

> **口径收敛声明（2026-11-26）：** DATA-CTR-005/007、QNT-CTR-003 当前均为 `deferred`，以 `doc/contract/PLAT-002-项目级契约冻结登记簿.md` 第 77～79 行为准。下表 DATA-CTR-005/007 卡点针对的是**通用 OHLC Decimal 精度路径**，仍成立；但首批 Close-only Factor/SMA20 复用现有已授权 Close Series Snapshot（精确 Close、双指纹 SnapshotRef、Manifest、授权读取），该窄化路径不等待通用 DTO，其生产门禁归 QNT-CTR-003 自身 Factor Schema/Golden、Decimal Registry/精度策略、生产 Facade 与 Researchflow Stage 接线。QNT-015 的 Snapshot Adapter 已有 SMA20 先例，Factor 正式接线与端到端证据已由第⑥～⑧组补齐；不得据旧口径称"无可用 Snapshot"。

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

Quant 现有 GetResult 授权读取用例、SMA20 与三因子（`ma20_distance`/`momentum_5d`/`momentum_20d`）的生产计算 Facade，且 Factor 结果已进入 Envelope 白名单。仍不得把纯函数拼装成伪生产 Provider：未完成 Schema/Golden/Validator/Registry 的 Indicator/Factor 一律不得开放 Envelope。此处以最新章节（第⑥～⑧组）为准，2026-11-19 审查记录只表示当时的基线状态。

## 4. Quant 当前可独立推进

- QNT-013：完善模块内 Manifest、Record、Envelope、MemoryRepository 与公开读取契约；生产 Adapter 交由 Server/Platform。
- QNT-001/002：推进不依赖权威外部输入的代码契约、测试和映射审计；不猜测单位、精度、舍入或质量策略。
- QNT-010：维护 fail-closed 风险执行框架；没有权威规则目录和真实事实前，不自创业务阈值。
- QNT-014：SMA20 与首批三因子 Facade 已交付；新增指标仍须先有 Registry/Schema/Golden，不提前拼装。
- QNT-015：首批 Close-only Factor/SMA20 走已授权 Close Series Snapshot，不受通用 DATA-CTR-005/007 阻断；通用 OHLC Decimal 输入仍保持阻断。

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


## 2026-11-26 Factor Stage Output Schema 校准

- 本节更新后续事实；此前“Server 只装配 News Search”“没有 Quant 正式 Stage Output Schema”等历史描述不再作为当前现状。`quant.sma20` 已有生产 Template/Executor/Output Registry Entry，SMA20 Stage Payload 携带落库后的 Quant `result_id`。
- 首批三个 Close-only Decimal Factor 已完成 `CreateFactorService` 与 Stage Output/Quality 1.0.0 Schema、强类型 DTO、严格运行时 Validator（`QNT-CTR-003-factor-stage-output-1.0.0.schema.json`、`QNT-CTR-003-factor-quality-summary-1.0.0.schema.json`、`internal/quant/stage/factor_output.go`）。Draft 2020-12 编译/实例校验、三因子正例与违规负例、Quant 全包与 race 均 PASS。
- 第⑥组已装配 `quant.factor` Stage Provider：Stage Adapter（授权 Snapshot → Facade → 强类型 Output/Quality 并自校验）、Output Registry（1.3.0，避免与 News/SMA20 版本冲突）、冻结 Workflow Template、参数 Schema（`factor_code` 三值 enum + symbol/date/limit）、`quant.factor.always` 适用性规则、Dispatcher 路由与 Runtime 前置校验。`factor_code` 只作 Stage 层分派，不透传给 Facade（其 parameters 仍为严格 `{}`）；三因子最小样本下限在 Adapter 先校验。该组结束时剩余卡点为 Factor Run↔Result 关联与端到端真实质量证据 + Contract Test，其中前者已由第⑦组解决。
- 第⑦组已完成 Factor Run↔Result 关联：`internal/server/bootstrap/quant_result_association.go` 把硬编码 `quant.sma20` 的校验泛化为已支持 Quant Stage 的封闭规格表（`quant.sma20` + `quant.factor`）。校验先按 Run 的 Stage 清单前置枚举候选，再只读取已成功产出的 Stage：未成功产出判为无关联；已成功但缺输出/质量引用、同码 Stage 重复、Run 引用不一致或读取失败一律 fail-closed，不把真实故障降级为"无关联"。授权读取路径同步改为任一 Quant Stage 存在即注入，修掉"仅装配 `quant.factor` 时没有授权读取"的缺口。Factor 结果现可经 `GetResult` 授权读取；剩余卡点只有端到端真实质量证据与 Quant→Researchflow Contract Test。
- QNT-CTR-003 仍 `deferred`、QNT-CTR-004/005 仍 `frozen`；Factor Envelope Payload 已开放写入/持久化（DEC-058），Run↔Result 关联能力已具备（第⑦组），但授权读取仍受 GOV-CTR-006 策略约束，且端到端证据与 Contract Test 未完成，因此未开放生产消费。本校准不升级契约状态。
- 第⑧组已补齐此前"未取得"的两项证据，仍不改契约状态与生产行为：①端到端质量证据 `internal/server/bootstrap/quant_factor_end_to_end_test.go`——只替换持久化为内存 Store，保留真实 Data 代码（Snapshot Creator 双指纹/内容寻址、Authorization Owner 签发与复核、Authorized Snapshot Reader 复核）并贯通生产 `quantDataSnapshotAdapter` → `FactorAdapter` → `CreateFactorService` → 结果 Repository；断言 QualitySummary 逐字段等于已持久化 Data Snapshot 的 coverage evidence、因子值经独立十进制计算核对、`result_id` 等于真实落库 Manifest 且可被 Run/Stage 关联读到、insufficient 显式 null 仍以 `result_status=insufficient_data` 落库并持有 result_id。②Quant→Researchflow 契约测试 `internal/server/bootstrap/quant_researchflow_contract_test.go`——Output Registry Entry 与关联规格表一致、`stageoutput.Registry.Validate` 接受真实 Envelope、文档化 Draft 2020-12 Schema 与运行时 Validator 判定一致、模板 degraded 传播与 Executor 映射一致、`Module` 实现 `quantDataSnapshotPort`。两条测试均有变异验证证据。
- 因此 Quant 侧"三因子生产化"在 Stage/结果闭环范围内无剩余卡点；后续门禁属 Researchflow 完整 Template（2.1）与 Server 报告读取 DTO（2.3），以及按登记簿对 QNT-CTR-003 的重新进入审批。仍需真实 MariaDB 集成与完整 CI 才能宣称生产验收。
- 1.3 trend-following 核实结论（2026-11-26）：卡点不在输入、也不在代码搬迁，而在**趋势语义契约未决**。旧 Runner 的 `trendFactor`/`trendStrategy` 只实现"MA20 距离与 5/20 日收益同时为正"的布尔合取并输出单一 `long_candidate`，无法映射 `TrendResult` 的 direction/phase/strength/conflict 四类语义；DEC-PLAT-002-049 又明确禁止互推且不定义阈值。因此本轮不实现算法、不注册 `quant.trend` Stage，结论与 D1–D5 决议请求见 `doc/quant/07-trend-following策略差距与契约结论.md`。解除卡点的唯一路径是取得该 5 项决议；在此之前任何 Trend 实现都属猜测性规则。

## 2026-11-26 执行清单 1.3 / 1.5 落地（D1–D5 决议通过后）

- 上述“1.3 卡在趋势语义未决”的结论已被决议取代：D1–D5 按推荐项通过（周期 5/20/60、direction 纯符号、phase 恒 `unknown`、strength 恒 `null`、只产出 TrendResult），1.3 由“书面结论”转为落地完成。决议表、落地范围与原最小范围的差异见 `doc/quant/07-trend-following策略差距与契约结论.md` 第 9 节。
- `quant.trend` 已是真实生产 Stage：Output/Quality Schema 1.0.0 + 强类型 DTO + 运行时 Validator（会复算方向，拒绝“证据与结论打架”的 payload）、Output Registry 1.4.0（Registry Set 以 version 为唯一键，必须与 News 1.0.0/1.2.0、SMA20 1.1.0、Factor 1.3.0 区分）、冻结 Template、参数 Schema（symbol/start_date/end_date/limit≥61，**无** `window` 字段）、`quant.trend.always` 适用性规则、Dispatcher 路由、Runtime 前置校验、关联规格表条目。
- 质量语义按 D5 之后的判断显式化：`valid`（四证据可用）/`degraded`（方向可判定但长周期证据不足）/`unknown`（方向不可判定），且 `unknown ⇔ insufficient_data`、其余必为 `completed`；两种降级都真实落库并持 `result_id`。Manifest 的 `quality_status` 取判定值而非输入 Snapshot 质量，以满足 QNT-CTR-005 对 payload 与 manifest 一致的要求；输入质量另以 `InputQualityStatus`/`result_quality_status` 分开表达，避免下游把“输入可信”误读为“结论可用”。
- 顺带修正一个生产级缺口：`principalStageExecutor` 原先只对 `quant.sma20` 注入 Quant service Principal，而 Data 的 Snapshot Creator 在 context 中解析可信 Principal，因此 `quant.factor`/`quant.trend` 在生产 Worker 会被 Data 拒绝——单测因直调 Adapter 掩盖了该缺口。现改为任一 Quant Stage 均注入。
- 1.5 输入质量门禁同步落地（Data + Quant 边界）：Quant 侧只消费 Data 已记录的 Snapshot 质量事实，经 `internal/quant/application/quality_applicability.go` 的封闭表得到“可计算 / 不可计算 + 唯一 reason code”（PIT 不可信 → 快照质量 → 日历质量 → Data 覆盖率 → 样本低于下限 → Registry 覆盖率 → 逐 bar 质量，顺序即优先级）。`validateAuthorizedCloseSeriesSnapshot` 拆成“结构完整性”与“质量事实→适用性”两段；历史回放缺 PIT 证据继续硬阻断。
- 错误分类的语义变化（需知悉）：此前除“覆盖率低于 Registry 策略”外，质量类失败一律落到 `ErrInvalidXxxSnapshot`（结构错误）。1.5 起“输入存在但不可用”统一以 `InputNotApplicableError` 表达并 unwrap 到 `ErrCloseSeriesSnapshotQuality`，三个 Facade 经 `classifyCloseSeriesValidationError` 保留原因码；Snapshot 自身不自洽（身份/Purpose/复权/日期/Provider 绑定/空 bars）仍是 `ErrInvalidXxxSnapshot` 且不带原因码。对应单测期望已同步更新，不涉及任何门禁放宽——两类都继续 fail-closed。
- Stage 侧不再把质量门禁失败笼统记为 `*_execution_failed`：`quant_input_reason.go` 把原因投影为 `quant_sma20_input_*`/`quant_factor_input_*`/`quant_trend_input_*`（表外原因回退为 `*_input_not_applicable`，不猜测具体原因）。变异验证三处（删覆盖率分支、删 PIT 映射、去掉原因码透传）均使测试失败。
- 契约状态未变：QNT-CTR-003 仍 `deferred`、QNT-CTR-004/005 仍 `frozen`；QNT-008 Signal 仍 NOT_STARTED。1.3/1.5 的完成只表示实现追平已通过决议，不等于对外生产发布。

## 2026-11-26 执行清单 1.6 market breadth 指标决策（M1 决议：书面废弃，已完成）

- 核实：旧 `marketAnalyzer`（`internal/service/wiring.go:210-255`，`mvp-watchlist-market-v1`）的分母来自 `universeResolver` 复制的 `req.Symbols`，指标是 `watchlist_above_ma20_pct` 加 65/35 无出处阈值映射出的 `regime` 字符串；缺指数数据时它自己就写了“不能视为完整市场广度分析”。消费者只有旧链（`analysis/app/runner.go` 校验 regime 非空、旧报告页展示“市场状态”），新链零消费者，QNT-005 仍 NOT_STARTED。
- Data 侧的真·市场广度已经存在且口径严格：`MarketBreadthSnapshot` 固定 `market=cn_a_share`，四类家数必须精确对账到 `covered_securities`，`requireFullCoverage` 时要求 `covered==total`，部分覆盖必须标 `degraded`，新鲜度取最旧成分报价时间；对外有 `GET /api/v1/market/breadth`。但它**只有 HTTP 展示路径**，没有授权快照——Quant 唯一合规入口（授权快照）目前只有单证券 `daily_bars_close_series`。
- 判定建议：**书面废弃**旧指标（不迁移），真实市场环境需求归 QNT-005。理由是分母随请求变化导致跨 Run 不可比、与 Data 广度同名不同义、65/35 阈值无出处；保留的两条路径（N 次单证券快照破坏 PIT 一致性 / 新增横截面 payload 属 DATA-CTR 变更）都缺真实调用方。结论、差距矩阵、命名规范与复活条件见 `doc/quant/08-market-breadth指标决策与正名.md`。
- QNT-005 前置项据此精确化：① Data 为 `MarketBreadthSnapshot` 增加授权快照 payload；② 停牌/ST/退市/新股排除规则需 Data 交易状态事实；③ 指数授权快照（旧 `index_20d_return_pct` 同样缺路径）。在契约就位前不以“先用 HTTP 广度凑”的方式启动 QNT-005。
- 命名规范（废弃后继续生效）：`breadth`/`广度` 只指 Data 的全市场口径；请求证券集合内的比例必须叫 `cross_section_*`/`*_participation` 且显式写分母。旧四个 `watchlist_*`/`insufficient_cross_section` 字符串随阶段 4 删除，不进新契约。
- 本轮不改任何契约状态：QNT-CTR-003 `deferred`、004/005 `frozen`、QNT-005 `NOT_STARTED`；不新增代码/Schema/Stage。

- **M1 决议（2026-11-26）**：按推荐项执行选项 B 书面废弃。旧 `watchlist_above_ma20_pct` 不迁移、不正名、不注册为 Quant 指标，也不新建“自选股广度”替代物；`internal/service/wiring.go` 的 `marketAnalyzer` 加退役注释（仅注释、不改行为），随阶段 4 删除旧链一并退役。真实市场环境需求归 QNT-005，前置项不变（广度授权快照 + 交易状态排除事实 + 指数授权快照）。第 6 节命名规范与第 7 节复活条件继续生效，1.6 记为已完成。

## 2026-11-26 执行清单 1.4 tradability 硬门禁切片（部分完成）

- 旧链事实：`internal/service/wiring.go:373-389` 的 `tradabilityChecker` 对每条候选恒 `Allowed=false`（理由：未提供可信的 ST/停牌/涨跌停/可成交量）；`analysis/app/runner.go:552-579` 的 `approvedForPlanning` 对“缺少可交易性判定”同样拒绝。所以 1.4 要覆盖的是**事实门禁**，不是业务阈值规则。
- Quant 侧现状：`internal/quant/risk` 已有 RuleSet/Evaluator/Decision/aggregate 全套（block 必 required、缺失必需规则补 unknown、panic 转执行失败、warn 不得覆盖 block），`RiskPayload` 也已注册进 Envelope（`risk/1.0.0`，QNT-CTR-004 `frozen`）。缺的是规则目录、输入对象与事实来源。
- 本轮落地：`internal/quant/risk/tradability.go` 新增规则集 `qnt-risk-tradability/1.0.0` 与规则 `tradability_unverified`（block + required）；四类事实枚举与旧链理由一一对应；未验证 → reject，声明已验证但缺证据/证据非法/表外事实 → unknown，均不产生 pass；`TradabilityGateInput` 无 Status/RuleSetVersion/评分/模型建议字段，规则集版本由门禁函数固定注入，并有白名单测试守住覆盖通道。
- 仍然阻塞（三条，缺一不可）：① `Signal` 契约（QNT-008 NOT_STARTED）——无评估对象，建 Facade/Stage 即零调用方接口；② Data 可交易性事实——`SecurityTradingStatusSnapshot` 缺**涨跌停状态与可成交量**，且无授权快照 payload（授权快照只有单证券 `daily_bars_close_series`）；③ 权威规则目录——QNT-010 明示不得由 Quant 猜测规则。
- 边界：不建 Risk Facade、不装 `quant.risk` Stage、不定义其余风险规则、不改任何契约。等价性对照、未做原因与推进条件见 `doc/quant/09-risk-tradability硬门禁切片与前置.md`。
- 验证：`go test ./internal/quant/risk/` 全绿（新增 9 用例）；变异验证两处均失败有效——把未验证结论改为 `not_triggered`（决策变 pass）、去掉“已验证必须有证据引用”（无证据变 pass）。

### 1.4 三条前置推进（2026-11-26 晚）

- 前置① Signal：新增 QNT-CTR-006 Signal 1.0.0（`internal/quant/signal` + Schema + Golden，DEC-059，`deferred`）。最小信号不含价格与自然语言规则；`direction`/`status` 独立枚举、禁止互推；`strategy_rule_set_version` 与 RiskDecision 的 `rule_set_version` 分层。
- 前置③ 规则目录：新增 `RuleCatalog`（`qnt-risk-catalog/1.0.0`），条目带依据来源与发布状态，pending 占位显式列出缺权威来源的规则主题；发布规则集只能引用已发布条目，引用未登记/未发布/pending 一律加载失败。收敛测试断言 `Load()` 与 `TradabilityRuleSet()` 完全相等。
- 前置② Data 事实（契约层）：新增 DATA-CTR-007 扩展 payload `security_trading_status_facts` 1.0.0 + 同族 Manifest 1.0.0（DEC-060）。五项事实逐项 available/unavailable，五项全可用→valid、任一不可用→degraded；不含涨跌停与可成交量（Provider 未提供，写入即等于把推导当事实）。测试覆盖派生 key 双侧拒绝、质量一致性、身份/指纹漂移与信任边界。
- 剩余阻塞收敛为一条：交易事实的存储列与授权读取分流（需与 DATA-CTR-008 Manifest/授权语义一起改，并在真实 MariaDB 环境验证），之后才谈 Risk Facade/Stage 装配。

### 1.4 前置② 物理读取链路接入（2026-11-26 深夜，#45）

- 三项决议（DEC-061）：共享表 + `payload_kind` 列、新增独立 `policy_ref` `security-status-facts-access`、门禁事实集维持四项。
- 迁移 V2 只 `ADD COLUMN payload_kind`（`VARCHAR(48)` ascii_bin 非空）；**刻意不改表名**——MySQL DDL 非事务，`RENAME` 失败会留下 `status=failed` 半完成迁移并阻断启动，代价远大于名字不贴切。
- 存储/授权双通道隔离：写入显式给 kind，读取 `snapshot_ref` 必须带 kind 条件（前缀对所有类型相同，仅凭 ref 无法区分）；策略引用为闭集，`Issue`/`AuthorizeRead`/Owner 发布/HTTP 四处校验数据类一致，不匹配 fail-closed 且不回落日线。
- Creator 只搬运不改判：源 `invalid`/`unknown` 拒绝，`degraded` 即使五项齐全也拒绝；枚举 `unknown` 一律 `unavailable` + 原因；仍不生成涨跌停与可成交量。
- Quant 消费端（历史状态，R2 后已被下文 2026-11-27 小节取代）：当时 `price_limit`/`tradable_volume` 恒入 `MissingFacts` → `Verified` 恒 false → 决策恒 `reject`。端到端测试曾断言 `reject + MissingFacts=[price_limit,tradable_volume] + EvidenceRefs=[snapshotRef,authorizationRef]`。
- **1.4 仍为部分完成**，但阻塞性质变了：不再缺链路，而是缺事实——没有涨跌停制度表与可成交量模型，门禁只能给出可追溯的拒绝。Facade/Stage 装配的实质前置变成"能产出非拒绝结论"，需先行立项（另行任务）。
- 验证：`gofmt`/`go vet`/`go test ./internal/...` 全绿，`-race` 覆盖三个改动模块；变异验证六处均有效失败。MariaDB 集成测试需 `MARKET_DATA_TEST_MYSQL_DSN`，**未在真实库执行**。
- 契约状态未变：QNT-CTR-003 `deferred`、004/005 `frozen`、006 `deferred`、DATA-CTR-007 `deferred`、008 `frozen`。

### 1.4 Facade/Stage 装配与拒绝路径验收（2026-11-26，DEC-062）

- 影响分析见 `doc/quant/11-Risk门禁Stage装配与拒绝路径影响分析.md`；四项决策按推荐项通过：拒绝表达在决策里、不声明 `hard_risk`、参数不要求 Signal 引用、`evaluated_at = system_as_of`。
- 交付 Risk Facade（`quant/application/create_risk.go`）与 `quant.risk` Stage（`quant/stage/risk.go`、`risk_output.go`、`server/bootstrap/quant_risk_stage.go`），Output Registry Set 1.5.0，接入 `isQuantStageCode`／Dispatcher／模板与适用性规则／结果关联规格表四处。
- 契约：新增 `QNT-CTR-003-risk-stage-output-1.0.0`、`QNT-CTR-003-risk-quality-summary-1.0.0` 与 `quant-risk-parameters-1.0.0`（+Golden），属 additive L2 补充，**不升级** QNT-CTR-003 状态、不改任何 frozen 契约。
- **拒绝语义**：Stage `succeeded` + `decision_status=reject` + Manifest `result_status=rejected`。禁止映射为 Stage 失败——失败 Attempt 不产出 Stage Output，拒绝结论将不可授权读取、不可追溯。消费者必须读决策；Stage 状态只表示"评估已执行完成"。决策为 `unknown` 时不产出 Stage Output（fail-closed）。
- **不声明 `hard_risk`**：决策恒 `reject`，声明门禁会让所有含该 Stage 的 Run 必然失败，等于用门禁摧毁流水线。待涨跌停/可成交量事实补齐、决策可能出现 `pass` 后另立项。
- 验收：新增 23 个测试全 PASS；`gofmt`/`go vet`/`go test ./internal/...` 全绿；`-race` 覆盖 `internal/quant/...` 与 `internal/server/bootstrap`。变异验证 6 处均有效失败（"关联规格表漏登记"原契约测试未捕获，已补显式断言）。
- **1.4 当时仍为部分完成（历史状态，R2 后见文末）**：装配已通、拒绝可追溯，但门禁**不能放行**——当时 `price_limit`/`tradable_volume` 两项事实仍不存在。RF-BLOCK-008 当时仍 OPEN，已在 R1 根治，不得用"Stage 已装配"冒充已解除。

### R2 真实盘口事实闭环当前口径（2026-11-27）

- `tradable_volume` 不再恒缺失：Quant 通过 `TradabilityEvidence` 显式消费交易状态授权快照与独立 `order_book` 授权快照；身份、system_as_of、payload kind、五档完整性与正成交量任一不满足即 fail-closed。
- 当前唯一缺失的门禁事实是 `price_limit`；RiskDecision 仍 `reject`，Stage 仍 `succeeded + decision_status=reject`，不得表述为硬门禁通过。
- Decision / Manifest / Stage Output 证据升级为四元 `[交易快照, 交易授权, 盘口快照, 盘口授权]`；QNT-CTR-003 Stage Output 发布 2.0.0，Quality Summary 发布 1.0.1；Output Registry Set 风险 1.7.0、合并 1.8.0。
- L3 quant.research 已改为发布盘口策略、启用 Eastmoney quote/order-book，并断言只缺 `price_limit` 且四段证据齐备；本地无 MariaDB/Redis 环境变量时按既有门控 SKIP，CI 结果需按精确 SHA 回填。
- **L3 交易时段依赖修复（2026-11-27）**：首轮 CI 失败根因不是 Quant 门禁，而是午间休市 Eastmoney 不返回五档深度；L3 现按 Run 的精确 `system_as_of` 预置合法盘口事实快照，Risk Stage 通过 Data 精确复用链消费，仍只缺 `price_limit`。修复提交 `1423d29c9` 的 Integration CI 45937832229 success，L3 PASS 7.02s，`quant.risk=succeeded`。
