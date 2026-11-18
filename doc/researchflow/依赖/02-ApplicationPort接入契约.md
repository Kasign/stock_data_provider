# Researchflow Application Port 接入契约

更新时间：2026-11-18  
维护分支：`feat/researchflow`

## 1. 接入原则

- Researchflow 只依赖被调用模块公开的 Application Port/DTO。
- Server 负责组装具体 Adapter 和生命周期；模块业务分支拥有 Facade 和领域授权规则。
- 不允许直接导入其他模块 Repository/Storage。
- 不允许同进程通过 localhost HTTP 回调。
- 不为未实现的接口创建空实现、占位 Provider 或默认成功 Adapter。

## 2. 现有接入边界

| 能力 | Researchflow 侧现有构件 | 生产 Adapter Owner | 验收要求 |
|---|---|---|---|
| News Search | 正式 Stage/Provider/参数 Schema | News + Server 组合装配 | 真实 Provider 响应、来源语义、Stage Output 和 MariaDB Run 证据 |
| Research Context | contextassembly Research DTO 投影 | Research Facade/授权由 Research；装配由 Server | 当前 Owner 授权、真实 Context 创建/读取、PIT 和质量字段一致 |
| Research Evidence/Claim | resultvalidation DTO 消费与引用闭环 | Research | 当前授权复核、Evidence/Claim 版本及来源引用闭环 |
| Quant Result | Quant Envelope 校验与 Validator Registry Port | Quant 提供 Schema/Manifest；Server 注入 | 精确 result_type/schema_version、真实 Payload、Result Manifest 与 Snapshot 引用闭环 |
| LLM Generate/Tool Call | ModelResponse/Tool Intent 适配、Tokenizer Port | LLM 提供 Provider/Tokenizer；Researchflow 执行工具授权 | 真实模型调用、结构化响应状态、模型与 Tokenizer 版本一致 |
| Terminal Event | Outbox Dispatcher | Server 装配；Notification 消费 | 终态事件原子写入、幂等消费/失败重试 |

## 3. 新增 Adapter 的强制材料

每个 Adapter 在开发前必须登记：
- 唯一调用场景和所属 FLOW 任务编号。
- 被调用 Application Port 的精确类型/方法及 Owner。
- 每个输入字段的来源、语义、版本、PIT/时区口径。
- 每个输出字段的来源、语义、可空条件和 Schema 版本。
- 授权主体、资源 Owner 和当前授权复核位置。
- 超时、取消、重试、幂等和资源释放规则。
- 诊断四态判定和可引用的真实 Evidence。
- 单测、契约测试、MariaDB 集成测试和真实 Provider/模型验收边界。

若被调用模块尚未提供正式 Port/Schema/授权语义，状态标记 BLOCKED_BY_DEPENDENCY，不由 Researchflow 猜测接口或自建替代能力。
