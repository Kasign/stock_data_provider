# Researchflow 模块

- 分支：\`feat/researchflow\`
- 代码：\`internal/researchflow/**\`
- 任务前缀：FLOW
- 状态：P0 领域和 CreateRun Application 首轮已落地；FLOW-005/006 已有模块内第一切片；Go 测试证据、生产 Adapter、真实持久化与并发验收待完成

Researchflow 是研究应用编排层，通过 Data、News、Quant、Research、LLM 的公开 Application Port 编排研究流程。Notification 不作为同步 DAG Stage；Run 终态 CAS 成功后按 INT-CTR-001 写本地 Outbox。它拥有 Run/Stage/Attempt 状态，不拥有其他模块内部领域规则。Platform Task Runner 只提供执行基础设施，不替代 Researchflow 业务状态机。

设计文档：[01-研究流程与状态机设计](01-研究流程与状态机设计.md)。

契约决议：[FLOW-CTR-001/002 最终决议（历史别名 CONTRACT-006/007）](03-CONTRACT-006与007最终决议.md)。

Application Port 与运行时契约补充：[04-ApplicationPort与运行时契约落地补充](04-ApplicationPort与运行时契约落地补充.md)。

FLOW-005 Context Assembly：[05-FLOW-005上下文装配实施说明](05-FLOW-005上下文装配实施说明.md)。

FLOW-006 Result Validation：[06-FLOW-006结果综合与校验实施说明](06-FLOW-006结果综合与校验实施说明.md)。

当前代码：\`domain\`（Run/Stage/Attempt 领域模型）、\`application\`（CreateRun 用例编排）、\`publicdto\`（独立公开 DTO）、\`parameterschema\`（不可变参数 Schema 校验）、\`template\`（模板/DAG）、\`state\`（状态机/重试/归并）、\`contextassembly\`（上下文预算/Tokenizer Port/Research DTO 投影/模型输入）、\`toolpolicy\`（工具白名单与调用授权）、\`resultvalidation\`（Quant Envelope Registry 与硬风险校验）。公开 DTO Schema 与 Golden 位于 \`doc/researchflow/schema/\`、\`doc/researchflow/golden/\`。当前不包含 Research/Quant 生产 Facade Adapter、真实 Tokenizer、Researchflow Repository、HTTP、Migration 或 Worker。

目标结构：
\`\`\`text
internal/researchflow/
  application/ domain/ template/ state/ publicdto/
  contextassembly/ toolpolicy/ resultvalidation/
  diagnostics/ persistence/
\`\`\`

职责：研究请求、流程模板、DAG、状态机、超时/取消/重试/恢复、Context 装配、工具权限、结果综合、运行诊断和 Application Facade。

详细任务见 [02-任务拆分与验收](02-任务拆分与验收.md)。
