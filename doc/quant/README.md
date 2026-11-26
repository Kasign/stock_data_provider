# Quant 模块文档索引

维护分支：`feat/quant`  
代码目录：`internal/quant/**`

## 唯一任务入口

- [模块总任务规划和进度](./模块总任务规划和进度.md)
- [当前任务状态和进度](./当前任务状态和进度.md)
- [任务历史](./任务历史/各个子任务完成状态.md)
- [跨模块依赖与解锁条件](./依赖/01-跨模块依赖与解锁条件.md)

## 设计与契约资料

- [量化引擎总体设计](./01-量化引擎总体设计.md)
- [首批输入输出契约与验收补充](./03-首批输入输出契约与验收补充.md)
- [RiskDecision DTO](./04-RiskDecision%20DTO.md)
- [TrendResult DTO](./05-TrendResult%20DTO.md)
- [trend-following 策略差距与契约结论](./07-trend-following策略差距与契约结论.md)
- [market breadth 指标决策与正名](./08-market-breadth指标决策与正名.md)
- [Risk tradability 硬门禁切片与前置](./09-risk-tradability硬门禁切片与前置.md)
- [Signal 契约最小切片（QNT-CTR-006）](./10-Signal契约最小切片.md)
- [Risk 门禁 Stage 装配与拒绝路径影响分析](./11-Risk门禁Stage装配与拒绝路径影响分析.md)
- [price_limit 事实闭环方案与前置核实](./12-price_limit事实闭环方案与前置核实.md)
- `schema/`：机器可校验的项目契约 Schema。
- `golden/`：算法固定样例、结构样例及负例验收输入。

## 文档维护规则

1. 任务状态只维护在模块总任务规划和进度中；当前任务文档只描述唯一正在执行的任务。
2. 历史文档追加完整执行证据，不随任务切换删除。
3. 项目级契约编号、冻结状态和跨模块决议以 `doc/contract/` 为准。
4. 本目录不保留已经迁入任务历史的重复阶段说明；机器 Schema/Golden 不作为普通说明文档清理。
