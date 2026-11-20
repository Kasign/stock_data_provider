# Researchflow 模块文档索引

维护分支：`feat/researchflow`  
代码目录：`internal/researchflow/**`

## 唯一任务入口

- [模块总任务规划和进度](./模块总任务规划和进度.md)：唯一总体任务状态权威
- [当前任务状态和进度](./当前任务状态和进度.md)：唯一当前执行入口
- [任务历史](./任务历史/)：按 FLOW 编号拆分；[原始完整历史归档](./任务历史/各个子任务完成状态.md)保留旧记录

## 任务历史

- [FLOW-001 Run/Stage/Attempt 与公开契约](./任务历史/01-FLOW-001-公开契约.md)
- [FLOW-002 Workflow Template/Registry](./任务历史/02-FLOW-002-Template与Registry.md)
- [FLOW-003 状态机/Deadline/Cancel](./任务历史/03-FLOW-003-状态机与Deadline.md)
- [FLOW-004 Scheduler/Worker/Lease/Output](./任务历史/04-FLOW-004-Worker与StageOutput.md)
- [FLOW-005 Context Assembly/Tool Policy](./任务历史/05-FLOW-005-ContextAssembly.md)
- [FLOW-006 Result Composer/Validator](./任务历史/06-FLOW-006-ResultValidation.md)
- [FLOW-007 Storage/Outbox](./任务历史/07-FLOW-007-Storage与Outbox.md)
- [FLOW-008 Diagnostics](./任务历史/08-FLOW-008-Diagnostics.md)
- [FLOW-009 Application Facade](./任务历史/09-FLOW-009-ApplicationFacade.md)
- [FLOW-010 Legacy Flow 迁移](./任务历史/10-FLOW-010-Legacy迁移.md)

## 依赖

- [模块依赖关系](./依赖/01-模块依赖关系.md)
- [Application Port 接入契约](./依赖/02-ApplicationPort接入契约.md)
- [生产 Stage 注册与验收准入](./依赖/03-生产Stage注册与验收.md)

## 设计与契约

- [研究流程与状态机设计](./01-研究流程与状态机设计.md)
- [任务拆分与验收（原始历史归档入口）](./02-任务拆分与验收.md)
- [CONTRACT-006/007 最终决议](./03-CONTRACT-006与007最终决议.md)
- [Application Port 与运行时契约](./04-ApplicationPort与运行时契约落地补充.md)
- [FLOW-005 Context Assembly](./05-FLOW-005上下文装配实施说明.md)
- [FLOW-006 Result Validation](./06-FLOW-006结果综合与校验实施说明.md)
- [Stage Provider Envelope 与 Output Registry](./07-Stage-Provider-Envelope与Output-Registry实施记录.md)
- [Stage Output 不可变资源与 Worker 原子提交](./08-Stage-Output不可变资源与Worker原子提交设计.md)
- `schema/`、`golden/`：机器契约和固定样例。

## 维护规则

1. 总体任务只在模块总规划维护；当前执行状态只在当前任务文件维护。
2. 历史任务按 FLOW 编号归档；旧集中历史作为完整原始档案保留，不再作为当前状态来源。
3. 项目级契约状态以 `doc/contract/` 和 `doc/整体任务规划/` 为准。
4. Server 生产装配以当前 Server/Integration 代码和精确 SHA 验收证据为准。
5. 不创建没有真实 Provider、Schema 和消费者的占位 Stage。
