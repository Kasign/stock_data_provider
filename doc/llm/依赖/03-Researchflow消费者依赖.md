# LLM → Researchflow 消费者依赖

## 1. 依赖方向

项目总规划规定 Researchflow 是跨模块流程编排与结果综合层，LLM 是模型协议适配层。允许 Researchflow 通过 LLM 公开 Provider Port 调用模型；LLM 不得 import Researchflow、Research 或 Quant。

## 2. LLM 提供的调用契约

- `llm.Provider.Generate(ctx, ModelRequest)`：单次非流式模型调用。
- `llm.Provider.Stream(ctx, ModelRequest, callback)`：单次 Responses SSE 调用，回调同步执行形成背压。
- `ModelRequest`：由调用方提供 request/correlation ID、purpose、模型、受控输入、输出 token 上限、可选 Schema/Tool definitions、deadline 和 system_as_of。
- `ModelResponse`：返回模型输出、状态、Usage、模型/响应标识、调用耗时和安全错误。
- `ModelToolCall`：只表示模型提出的工具调用意图，不代表已授权或已执行。

## 3. Researchflow 必须负责

1. 将 Run/Stage 上下文映射为 LLM 自有 ModelRequest；不得把 Research/Quant Domain 类型泄漏到 LLM。
2. 决定调用时机、调用预算、总 Deadline、模型选择和业务重试。
3. 对模型返回的结构化结果执行业务 Schema、Evidence 引用、数值来源和质量校验。
4. 对 Tool Call 使用自身 Tool Registry、白名单、权限、参数校验、预算和执行机制；禁止直接执行未经授权的模型输出。
5. 保存必要调用元数据并关联 Run/Stage/Attempt；默认不持久化完整敏感 Prompt/Response。
6. 对取消、超时、部分流输出和 incomplete 状态制定流程层结果策略。

## 4. LLM 不负责

- Run/Stage/Attempt、DAG、模板、恢复和业务状态机。
- 工具注册、工具授权、工具执行、调用预算和业务重试。
- Research Evidence/Context/Event/Hypothesis 领域语义。
- Quant 指标、概率、收益和硬风险门禁。
- 将模型输出直接升级为事实或风险结论。

## 5. 当前接入证据

项目总规划和 LLM/Researchflow 公开边界明确，但在当前代码树中尚未发现 Researchflow 对 `llm.Provider` 的实际调用接线。Server 文档也将 SRV-010 生产 Provider 装配列为阻塞项。因此当前状态是接口边界已定义、运行时消费者接入待完成；不能把跨模块静态契约核对描述成端到端模型调用已通过。

## 6. 验收标准

- PASS：真实 Researchflow Stage 调用 Provider，Run/Stage/Attempt 与 Request/Correlation ID 可关联；工具授权和业务校验仍由 Researchflow 控制。
- DEGRADED：模型能力部分可用，流程明确标记 incomplete/degraded 并禁止依赖缺失能力的阶段。
- FAIL：LLM 反向依赖业务模块、LLM 执行工具、绕过预算/权限，或未经校验把模型输出作为事实。
- NOT_COMPARABLE：仅有接口编译、Mock Provider 或单元测试，未完成 Server 生产装配与真实模型调用。
