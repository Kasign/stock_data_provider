# Quant 模块文档索引

维护分支：`feat/quant`  
代码目录：`internal/quant/**`

## 唯一任务入口

- [模块总任务规划和进度](./模块总任务规划和进度.md)
- [当前任务状态和进度](./当前任务状态和进度.md)
- [任务历史](./任务历史/各个子任务完成状态.md)

## 设计与契约

- [量化引擎总体设计](./01-量化引擎总体设计.md)
- [首批输入输出契约与验收补充](./03-首批输入输出契约与验收补充.md)
- [RiskDecision/TrendResult 与计算上下文专项文档](./04-RiskDecision%20DTO.md)
- [Factor 质量策略执行入口](./07-Factor质量策略执行入口.md)
- [RiskDecision 硬门禁与版本化规则](./08-RiskDecision硬门禁状态聚合.md)
- [RepositoryPort 与不可变结果记录](./11-RepositoryPort与不可变结果记录.md)
- `schema/`、`golden/`：机器契约和固定样例。

## 维护规则

1. 总体任务与当前状态分别由两个任务文档维护。
2. README 只做索引，不复制任务状态表。
3. 项目级 QNT-CTR 状态以 `doc/contract/` 为准。
4. 真实 Data、生产授权/存储和 Researchflow 消费证据分别记录，不用 Fixture 或模块 CI 替代。
