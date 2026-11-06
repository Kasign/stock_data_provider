# Researchflow 模块

- 分支：`feat/researchflow`
- 代码：`internal/researchflow/**`
- 任务前缀：FLOW
- 状态：规划中，尚未开始模块分支开发

Researchflow 是研究应用编排层，通过公开 Application Port 调用 Data、News、Quant、Research、LLM。Run 终态通知通过本地 Outbox 和 INT-CTR-001 交付，不把 Notification 作为同步 DAG Stage 或直接调用其内部发送接口。它拥有 Run/Stage/Attempt 状态，不拥有其他模块内部领域规则。Platform Task Runner 只提供执行基础设施，不替代 Researchflow 业务状态机。

设计文档：[01-研究流程与状态机设计](01-研究流程与状态机设计.md)。

契约决议：[FLOW-CTR-001/002 最终决议（历史别名 CONTRACT-006/007）](03-CONTRACT-006与007最终决议.md)。

Application Port 与运行时契约补充：[04-ApplicationPort与运行时契约落地补充](04-ApplicationPort与运行时契约落地补充.md)。\n\n目标结构：
```text
internal/researchflow/
  application/ orchestration/ workflow/ contextbuilder/
  tools/ diagnostics/ persistence/
```

职责：研究请求、流程模板、DAG、状态机、超时/取消/重试/恢复、Context 装配、工具权限、结果综合、运行诊断和 Application Facade。

详细任务见 [02-任务拆分与验收](02-任务拆分与验收.md)。
