# Go 原生研究分析系统：跨模块架构与集成文档

本目录只维护五模块共同的架构原则、跨模块流程、契约边界和集成策略。模块内部设计、任务和验收以对应模块目录为准。

## 模块文档
- [Quant](../quant/README.md)：`feat/quant`，`internal/quant/**`
- [Research](../research/README.md)：`feat/research`，`internal/research/**`
- [Researchflow](../researchflow/README.md)：`feat/researchflow`，`internal/researchflow/**`
- [LLM](../llm/README.md)：`feat/llm`，`internal/llm/**`
- [Notification](../notification/README.md)：`feat/notification`，`internal/notification/**`

## 本目录现有文档
- [01-总体架构与分析流程](01-总体架构与分析流程.md)：跨模块完整流程。
- [02-Quant量化引擎设计](02-Quant量化引擎设计.md)：早期设计稿；细节以 `doc/quant/` 为准。
- [03-Research研究与证据模型设计](03-Research研究与证据模型设计.md)：早期设计稿；细节以 `doc/research/` 为准。
- [04-Research-Orchestrator与运行诊断设计](04-Research-Orchestrator与运行诊断设计.md)：早期设计稿；细节以 `doc/researchflow/`、`doc/llm/`、`doc/notification/` 为准。
- [05-任务拆分与迁移验收计划](05-任务拆分与迁移验收计划.md)：旧 ANA 拆分参考；当前编号以 QNT/RSC/FLOW/LLM/NTF/GOV 为准。

模块分支只更新自己的 `doc/<module>/**`。跨模块边界、共同契约或集成顺序变化，先更新项目总览和本目录。Data/News/Platform/WebUI 专项文档继续由原分支维护，不在这里复制进度。
