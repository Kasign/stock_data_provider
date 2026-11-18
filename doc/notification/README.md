# Notification 模块文档索引

维护分支：`feat/notification`  
代码目录：`internal/notification/**`

## 唯一任务入口

- [模块总任务规划和进度](./模块总任务规划和进度.md)
- [当前任务状态和进度](./当前任务状态和进度.md)
- [任务历史目录](./任务历史/)
- [原始完整执行记录](./任务历史/00-历史完整执行记录.md)

## 按编号归档

- [NTF-001：Event / Inbox / Subscription](./任务历史/NTF-001-事件接收与订阅.md)
- [NTF-002：Template / Renderer](./任务历史/NTF-002-模板与渲染.md)
- [NTF-003：Channel Adapter](./任务历史/NTF-003-渠道适配器.md)
- [NTF-004：Fanout / Dispatcher](./任务历史/NTF-004-Fanout与Dispatcher.md)
- [NTF-005：Retry / Attempt / Recovery / Unknown](./任务历史/NTF-005-重试与恢复.md)
- [NTF-006：Repository / Migration](./任务历史/NTF-006-存储与迁移.md)
- [NTF-007：Diagnostics / Consistency](./任务历史/NTF-007-诊断与一致性.md)
- [NTF-008 文档治理](./任务历史/NTF-008-文档治理.md)

## 领域设计与契约

- [通知领域与投递设计](./01-通知领域与投递设计.md)
- [Application Port 与投递状态契约](./03-ApplicationPort与投递状态契约.md)
- [契约 Fixture 与状态迁移验收](./04-契约Fixture与状态迁移验收清单.md)
- [首批业务事件映射决议](./05-首批业务事件映射决议.md)
- [事件接收与投递状态契约对齐补充](./06-事件接收与投递状态契约对齐补充.md)
- `schema/`、`golden/`：机器契约和固定样例。

## 依赖

- [业务事件与模板决议](./依赖/01-业务事件与模板决议依赖.md)
- [Server Worker 与身份目录](./依赖/02-Server投递Worker与身份目录依赖.md)
- [真实渠道与凭据](./依赖/03-真实渠道与凭据依赖.md)

## 维护规则

1. 总规划是唯一任务状态权威来源；当前任务文档只记录一个任务。
2. 每个 NTF 编号对应独立历史文档；00 文档保留原始完整记录。
3. README 只做索引，不复制任务状态或 CI 结论。
4. 业务事件、Filter 和模板语义由业务 Owner 冻结。
5. Outbox→Inbox 与外部渠道发送分开验收。
