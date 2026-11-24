# 06-联调收尾

本目录是 `repair_all` 分支后续跨模块联调、阻断修复和端到端验收的**唯一任务执行依据**。

## 文件职责

- [RepairAll跨模块修复计划与进度](./RepairAll跨模块修复计划与进度.md)：跨模块主线、优先级、任务顺序、阶段状态、精确 SHA、CI 与端到端验收记录。它是本目录的主控文档。
- [Data模块卡点](./Data模块卡点.md)
- [News模块卡点](./News模块卡点.md)
- [Quant模块卡点](./Quant模块卡点.md)
- [Research模块卡点](./Research模块卡点.md)
- [Researchflow模块卡点](./Researchflow模块卡点.md)
- [WebUI旧Analysis到Researchflow迁移盘点](./WebUI旧Analysis到Researchflow迁移盘点.md)：旧 Task/Event API、新 Run/Stage API、认证边界、生产模板缺口和退役条件。
- [LLM模块卡点](./LLM模块卡点.md)
- [Platform模块卡点](./Platform模块卡点.md)
- [Server模块卡点](./Server模块卡点.md)
- [Notification模块卡点](./Notification模块卡点.md)

模块卡点文档负责记录模块边界内的具体阻断、责任归属、依赖、证据和解锁条件；不得各自另立跨模块总计划。

## 后续联调规则

1. 后续联调任务先读取本目录主控文档，再读取本轮涉及模块的卡点文档。
2. 新发现的问题先定位真实调用链和责任模块，再登记到对应卡点文档；跨模块顺序、状态和验收口径统一回写主控文档。
3. 每个任务必须记录代码位置、当前事实、阻断原因、依赖、修复方案和可验证验收标准。
4. 区分已实现且已验证、已实现但未接入、部分实现、缺失、证据不足；不得把未运行或无真实消费者的代码标记为生产通过。
5. 以当前 `repair_all` 分支 HEAD 和精确 CI SHA 为准；每轮重新核对，不沿用过期 SHA。
6. 本目录不复制契约、Schema、Golden 或模块设计。它们的权威定义仍在 `doc/contract/` 和各模块专项文档中；本目录只记录联调所需引用和状态。
7. 完成或解除卡点时同步更新本目录主控文档与对应模块卡点文档，避免状态分叉。

## 端到端目标

以用户从 WebUI 发起一次真实 A 股研究任务为验收主线，验证任务创建、Server 调度、Data/News 输入、Quant 计算、Research/Researchflow 编排、LLM 分析、风险门禁、结果与证据持久化、WebUI 回读展示，以及配置启用时的 Notification 投递。不得以单模块单测、Mock 或仅有接口定义代替完整流程验收。
