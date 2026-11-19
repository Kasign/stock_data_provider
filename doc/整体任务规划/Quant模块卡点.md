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
