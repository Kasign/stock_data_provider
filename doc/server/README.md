# Server 文档索引

维护分支：feat/server  
代码目录：internal/server/**

## 唯一任务入口

- [模块总任务规划和进度](./模块总任务规划和进度.md)
- [当前任务状态和进度](./当前任务状态和进度.md)
- [任务历史](./任务历史/各个子任务完成状态.md)
- [依赖与跨模块准入](./依赖/README.md)
- [Owner Session 专项接入记录](./Owner-Session接入进度.md)

## 设计与契约

- [Owner Session HTTP API 契约](./contracts/SRV-CTR-002-owner-session-http-api.md)
- [Owner Session 接入进度](./Owner-Session接入进度.md)
- [历史架构、Bootstrap、Migration 与 Integration 决议](./任务历史/各个子任务完成状态.md)
- 项目级契约：doc/contract/ 与 doc/整体任务规划/。

## 维护规则

1. 模块总任务规划是 Server 任务状态唯一权威来源；当前任务文档只能有一个。
2. README 只做索引；历史规划与 Review 归入任务历史。
3. 跨模块依赖和准入条件统一登记在依赖目录，不在当前任务文档临时扩展任务。
4. Server 只负责组合装配，不实现领域规则或复制 Platform 原语。
5. Integration 只做聚合验收，不作为 Server 日常开发分支。
