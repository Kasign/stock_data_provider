# Notification 模块

- 目标分支：`feat/notification`
- 代码目录：`internal/notification/**`
- 任务前缀：NTF
- 当前状态：Design Ready；模块分支尚未创建。契约语义以项目级目录为准：NTF-CTR-001～003 为 pending_evidence，INT-CTR-001 为 pending_decision；不以个人 Owner Review 作为开工门槛。

Notification 是独立通知业务模块，负责事件接收、Subscription、Template、Channel、Dispatcher、Delivery、Attempt、幂等、重试和投递诊断。不计算业务告警条件，不依赖 Researchflow 内部实现。

设计入口：
- [通知领域与投递设计](01-通知领域与投递设计.md)
- [任务拆分与验收](02-任务拆分与验收.md)
- [Application Port 与投递状态契约](03-ApplicationPort与投递状态契约.md)
- [契约 Fixture 与状态迁移验收清单](04-契约Fixture与状态迁移验收清单.md)

关键边界：recipient_scope 不等于真实发送目标；外部渠道不承诺 exactly-once；发送结果不确定时不得盲目重试。Platform 提供 Secret、HTTP、Storage 和 Worker 基础设施，领域状态与 Repository 归 Notification 所有。

Notification 领域开发可按 NTF-CTR-001～003 的已决语义和当前 pending_evidence 状态推进；依赖未完成 Schema/Golden 的事件调用点不得宣称 L2。首批跨模块事件映射 INT-CTR-001 仍为 pending_decision，只阻断对应自动事件发布，不阻断 Notification 独立 Domain、Inbox/Delivery 状态机和 Fake Channel 测试。真实渠道、真实 Storage、根 DI/Worker/Migration 属独立 L3 集成门禁。
