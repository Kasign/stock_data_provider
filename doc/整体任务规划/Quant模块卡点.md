# Quant 模块卡点

更新时间：2026-11-19  
维护分支：`feat/quant`  
责任模块：Quant  
说明：本文件登记需要其他模块或项目级决策方提供输入、装配或验收的事项。模块内详细依赖见 `doc/quant/依赖/01-跨模块依赖与解锁条件.md`。

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
| 生产 Storage、事务、Migration | Save Port、MemoryRepository | Server 装配，Platform 提供公共基础能力 | 生产 Repository Adapter、数据库唯一约束/事务、根 Migration 和连接生命周期注入 | Integration 验证幂等、冲突、并发与持久化 |
| 可信 Principal 与授权撤销一致性 | PrincipalResolver Port、GetResult 授权用例 | Server/Platform | 从可信认证上下文注入 Principal；提供策略快照、撤销复核及一致性边界 | Integration 验证授权读取和撤销竞态 |
| Access Audit 持久化 | AccessAuditor Port | Server/Platform | 审计持久化 Adapter、保留策略和故障处理 | Integration 验证审计落库及失败语义 |
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
- Server Migration Registry 没有 Quant namespace；不能通过空注册或 Server 自建 SQL 表绕过模块存储归属。Quant 需要提供模块拥有的持久化定义/接口，Server 负责组合注入，Platform 提供公共事务/Storage 原语。
- 当前 `GetResult` 的用途为 `quant_analysis`、内容级别 `full`。但授权策略 Owner、subject_ref 可信映射、策略撤销的线性化边界、审计保留和失败处置尚无可据以实现的生产契约/决议。不得默认 Allow，不得把 Owner Session 身份直接当成 Quant 资源授权。
- 解锁顺序：① Quant/项目契约 Owner 明确策略 Owner、策略快照读取/撤销一致性和审计语义；② Quant/Server/Platform 按现有 Port 与模块边界实现生产 Repository、Migration、Principal/Authorized Reader/Audit Adapter；③ Server Root 注入并做 MariaDB 并发/冲突/撤销竞态/审计失败集成验收；④ 具备明确真实消费场景后再接 Researchflow Quant Stage。
- QNT-013 模块内取消传播子项已 CLOSED；生产闭环仍 OPEN。QNT-014 Facade 与 Researchflow Stage 不因本次复核提前开放。


## RepairAll QNT-013 持久化结构审查补充（2026-11-20）

- 已核对 Quant Manifest/Record/Save Port、Platform Storage/Migration Runner、Server Migration Registry 和 Research SQL Repository 范式。
- 结果存储建议最小单表：result_id 唯一主键、manifest_fingerprint、result_fingerprint、manifest_json、payload_json、created_at；字段直接来源于现有 Manifest/Record/Repository 契约。该模型尚待 MariaDB 实现验证，不视为已冻结 Schema。
- Save 必须依赖 DB 唯一约束原子仲裁；重复相同指纹幂等成功且不覆盖首次 created_at，异内容冲突 ErrResultConflict；禁止 REPLACE/覆盖 UPSERT/无锁先查后插。
- Quant 拥有自己的 Storage Adapter/Migration；Server 只注册 Quant Migration 和注入；Platform 复用现有连接池/迁移能力。没有查询用例前不新增二级索引。
- 授权读取和 Access Audit 不得并入 Save Repository。AuthorizedResultReader 需策略快照与结果读取一致性边界；AccessAuditor 写入失败必须拒绝释放结果。
- 授权策略 Owner、撤销一致性、可信 subject_ref 映射、审计保留/失败处置仍未明确，生产授权 Reader 不得先行伪实现。
- 当前结论：持久化设计审查完成；SQL Schema/Adapter/Migration 未实现；QNT-013 生产闭环继续 OPEN。下一步核对 GOV-CTR-006 和 Platform Authorization 已有能力，避免重复建设。
