# Notification 模块文档索引

维护分支：`feat/notification`  
代码目录：`internal/notification/**`

## 唯一任务入口

- [模块总任务规划和进度](./模块总任务规划和进度.md)
- [当前任务状态和进度](./当前任务状态和进度.md)
- [任务历史](./任务历史/各个子任务完成状态.md)

## 领域设计与契约

- [通知领域与投递设计](./01-通知领域与投递设计.md)
- [Application Port 与投递状态契约](./03-ApplicationPort与投递状态契约.md)
- [契约 Fixture 与状态迁移验收](./04-契约Fixture与状态迁移验收清单.md)
- [首批业务事件映射决议](./05-首批业务事件映射决议.md)
- [事件接收与投递状态契约对齐补充](./06-事件接收与投递状态契约对齐补充.md)
- `schema/`、`golden/`：机器契约和固定样例。

## 维护规则

1. 总体任务与当前状态分别由两个任务文档维护。
2. README 只做索引；Commit/Run/历史实施记录统一归入任务历史。
3. 事件 Payload、Filter 和模板语义由业务 Owner 冻结，Notification 不自行发明。
4. Outbox→Inbox 接收与外部渠道发送分开验收。
