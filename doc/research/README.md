# Research 模块

- 分支：`feat/research`
- 代码：`internal/research/**`
- 任务前缀：RSC
- 状态：规划中，尚未开始模块分支开发

Research 负责 Context、Evidence、Event、Claim、Hypothesis、Scenario 等研究语义模型。News 负责采集/原文/去重/检索；Data 负责行情/财务/PIT；Quant 负责数值计算。Research 不依赖 Quant 或 Researchflow。

设计文档：[01-研究领域模型与证据体系](01-研究领域模型与证据体系.md)。\n\n目标结构：
```text
internal/research/
  context/ evidence/ event/ claim/ hypothesis/ scenario/
```

事实、报道事实、推断、假设和未知必须区分；Evidence 记录来源/时间/版本/正反立场；Event 生命周期采用版本追加；Hypothesis 必须可验证、可证伪；Scenario 概率由 Quant 提供。

详细任务见 [02-任务拆分与验收](02-任务拆分与验收.md)。
