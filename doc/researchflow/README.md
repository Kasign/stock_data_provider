# Researchflow 模块文档索引

维护分支：`feat/researchflow`  
代码目录：`internal/researchflow/**`

## 唯一任务入口

- [模块总任务规划和进度](./模块总任务规划和进度.md)
- [当前任务状态和进度](./当前任务状态和进度.md)
- [任务历史](./任务历史/各个子任务完成状态.md)

## 设计与契约

- [研究流程与状态机设计](./01-研究流程与状态机设计.md)
- [FLOW-CTR-001/002 最终决议](./03-CONTRACT-006与007最终决议.md)
- [Application Port 与运行时契约](./04-ApplicationPort与运行时契约落地补充.md)
- [FLOW-005 Context Assembly](./05-FLOW-005上下文装配实施说明.md)
- [FLOW-006 Result Validation](./06-FLOW-006结果综合与校验实施说明.md)
- [Stage Provider Envelope 与 Output Registry](./07-Stage-Provider-Envelope与Output-Registry实施记录.md)
- [Stage Output 不可变资源与 Worker 原子提交](./08-Stage-Output不可变资源与Worker原子提交设计.md)
- `schema/`、`golden/`：机器契约和固定样例。

## 维护规则

1. 总体任务只在总规划维护；当前执行状态只在当前任务状态文件维护。
2. README 只做索引；历史 Commit/Run/阶段记录统一归入任务历史。
3. 项目级契约状态以 `doc/contract/` 为准。
4. Server 生产装配以当前 Integration/Server 代码和精确 SHA 验收证据为准。
5. 不创建没有真实 Provider、Schema 和消费者的占位 Stage。
