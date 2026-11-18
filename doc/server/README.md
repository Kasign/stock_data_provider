# Server 文档索引

维护分支：feat/server  
代码目录：internal/server/**

## 唯一任务入口

- [模块总任务规划和进度](./模块总任务规划和进度.md)
- [当前任务状态和进度](./当前任务状态和进度.md)
- [任务历史](./任务历史/各个子任务完成状态.md)
- [Owner Session 专项接入记录](./Owner-Session接入进度.md)

## 设计与契约

- [Server 架构与组合根设计](./01-Server架构与组合根设计.md)
- [Server Bootstrap 与生命周期](./02-Bootstrap与生命周期.md)
- [HTTP/认证/错误投影](./03-HTTP与安全接入.md)
- [Migration/Diagnostics/Readiness](./04-Migration与Diagnostics.md)
- 项目级契约：doc/contract/ 与 doc/整体任务规划/。

## 维护规则

1. 总体任务和当前状态分别由两个任务文档维护。
2. README 只做索引；历史规划与 Review 归入任务历史。
3. Server 只负责组合装配，不实现领域规则或复制 Platform 原语。
4. Integration 只做聚合验收，不作为 Server 日常开发分支。
