# FLOW-010：旧 Analysis 退役

## 当前状态

**进行中：旧链调用与能力映射已完成，生产链切换和代码退役尚未开始。**

本文件仅保留 FLOW-010 历史任务编号和状态入口，不再维护第二份迁移方案，避免旧前置条件与当前 Server/Researchflow 契约重复或冲突。

## 唯一执行文档

旧 Analysis 的真实调用链、模块职责映射、Researchflow/Server/WebUI 迁移顺序、数据库清理步骤和逐项进度，统一维护于：

- `doc/整体任务规划/06-联调收尾/WebUI旧Analysis到Researchflow迁移盘点.md`

## 已完成

1. 确认 Root Server 是唯一外部 HTTP 入口，Researchflow 仅承担内部执行编排。
2. 确认旧 Analysis 仍由 WebUI、Server Router、Bootstrap、Worker 和 Migration 实际引用。
3. 完成旧 Runner 阶段到 Data、Quant、Research、Researchflow、Server 的职责映射。
4. 核对 Quant 已有因子/趋势/风险构件与实际生产 Stage 的差异。

## 尚未完成

- Quant 正式指标/因子/策略/硬风险 Application 与 Researchflow Stage 闭环。
- Data Snapshot 作为统一研究输入的授权读取和关联。
- Researchflow 研究结果与领域结果的强类型关联。
- Server 授权报告读取契约和 WebUI 迁移。
- 删除旧 Analysis 生产链、旧配置、测试和 Migration。
- 一次性清理旧 Analysis 表及 Migration History namespace。

## 退役约束

不得先删除旧 Runner/API 再补替代能力。旧持仓账本、仓位计划和执行评估占位实现不迁移；真实因子、策略和风险硬门禁必须先由其领域 Owner 正式接管。旧数据不迁移、不兼容。
