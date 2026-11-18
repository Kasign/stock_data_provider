# Notification 模块卡点

更新时间：2026-11-18  
维护分支：`feat/notification`  
代码范围：`internal/notification/**`  
当前基线：`f92f83e5f49b1083b49a9d9e4430c09ec901088a`

## 1. 当前结论

Notification 模块的核心领域能力、存储、状态机、诊断和首个真实渠道连通性已经完成阶段验收。当前没有发现可以在 Notification 分支独立闭环、且有明确业务依据的 P0/P1 新功能。

当前状态不是“模块全部生产验收完成”，而是“模块核心能力已完成，生产闭环受业务输入和 Server 装配阻塞”。

## 2. 卡点清单

| 编号 | 卡点 | 责任方 | 当前状态 | 解阻条件 |
|---|---|---|---|---|
| NTF-BLOCK-01 | 真实业务事件 Schema、版本、Payload、触发时点及幂等键未全部冻结 | 各业务事件生产方 Owner（Research、Researchflow、Quant 等） | BLOCKED | 生产方提交正式事件契约、Schema/Golden 和契约测试 |
| NTF-BLOCK-02 | Subscription Filter 的字段语义、版本和匹配规则未由事件 Owner 冻结 | 对应业务事件 Owner | BLOCKED | Owner 冻结 Filter 契约；Notification 按契约实现/验收，不自创 DSL |
| NTF-BLOCK-03 | 真实业务模板文案、变量绑定、空值语义和渠道格式未提供 | 各业务模块 Owner | BLOCKED | 提供模板版本、变量清单及每个变量对应的 Payload 字段/类型/来源 |
| NTF-BLOCK-04 | Recipient Directory 尚未由 Server 提供并装配 | Server | BLOCKED | Server 提供按 opaque recipient_ref 解析授权目标的实现和集成测试 |
| NTF-BLOCK-05 | Fanout Worker、Delivery Worker 尚未在 Server 生产运行时装配 | Server | BLOCKED | 完成 Worker 启停、并发、批次、唤醒/轮询、退避和优雅关闭装配 |
| NTF-BLOCK-06 | Notification Repository、Template Repository、Channel Registry、RecipientResolver、Platform Secret/HTTP Client 尚未完成生产组合装配 | Server | BLOCKED | Server Bootstrap 完成依赖注入并通过根模块集成测试 |
| NTF-BLOCK-07 | Notification v1～v6 Migration 的根入口注册及部署环境迁移仍需 Server 集成确认 | Server | BLOCKED | 根 Migration 汇总入口包含 Notification 迁移，并在集成环境验证升级路径 |
| NTF-BLOCK-08 | 多实例并发、进程崩溃、租约恢复、Unknown 对账和优雅停止缺少真实 Server 运行验收 | Server + Notification | BLOCKED | 在真实服务运行环境完成故障注入和恢复验收，保留日志/诊断证据 |
| NTF-BLOCK-09 | 生产环境 Webhook Secret 的注入、轮换和撤销流程尚未作为部署流程验收 | Server/部署运维 | BLOCKED | Secret 不进入代码/日志；完成生产配置注入、轮换与撤销流程验证 |

## 3. 已完成的阶段证据

### 3.1 Notification 核心能力

- NTF-001～NTF-009 的代码 Review、核心实现和现有测试证据已归档。
- Notification MariaDB lifecycle integration 已通过此前 CI。
- NTF-005 租约恢复与 Attempt 对账缺陷已修复。
- NTF-006 Delivery 不可变字段保护已修复，并有 MariaDB 篡改测试。
- 模块诊断、安全 DTO 和状态一致性检查已完成当前代码复核。

### 3.2 首个真实渠道

- 渠道：企业微信群机器人 Webhook。
- GitHub Actions：[Run 44295239517](https://github.com/Kasign/stock_data_provider/actions/runs/44295239517)。
- 分支：`feat/notification`。
- 执行 SHA：`31697d34f7eebc79cb1de69c51f51c7c257cb549`。
- Adapter 单测与 `TestLiveWebhook` 均通过。
- 用户确认目标企业微信群实际收到测试消息。
- 结论：首个真实渠道连通性 PASS。

验收边界：这不等于完整生产 Worker 生命周期验收；群机器人不提供服务端幂等、Delivery Lookup、Retry-After 或可靠送达回执。

## 4. Notification 当前仍可执行的工作

1. 维护本模块代码和依赖文档与真实集成状态一致；外部依赖到位后按正式契约接入。
2. 对 Server 提供的 Worker/Recipient Directory 实现进行契约 Review 和集成测试。
3. 对业务 Owner 提供的真实 Schema、Filter、模板进行校验，补齐 Golden/Contract Test。
4. 在 Server 集成环境执行端到端投递、失败分类、租约恢复、多实例和优雅停止验收。

在上述输入未到位前，不新增猜测性的业务事件、模板、Filter DSL、Recipient Directory 或 Worker 框架；不以 Fake 或合成数据将 BLOCKED 改为 PASS。

## 5. 推荐解阻顺序

1. Server 先完成 Recipient Directory、Worker 与依赖装配、根 Migration 注册。
2. 业务模块并行冻结首批事件 Schema/Golden、Filter 语义和模板变量映射。
3. Notification 根据正式契约补齐适配与测试。
4. Server + Notification 联合完成真实运行环境端到端验收。
5. 部署侧完成 Webhook Secret 的生产注入、轮换和撤销验收。

## 6. 关联文档

- [Notification 当前任务状态和进度](../notification/当前任务状态和进度.md)
- [Notification 模块总任务规划和进度](../notification/模块总任务规划和进度.md)
- [业务事件与模板决议依赖](../notification/依赖/01-业务事件与模板决议依赖.md)
- [Server Worker 与身份目录依赖](../notification/依赖/02-Server投递Worker与身份目录依赖.md)
- [真实渠道与凭据依赖](../notification/依赖/03-真实渠道与凭据依赖.md)
