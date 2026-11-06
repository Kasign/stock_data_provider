# LLM 模块

- 分支：`feat/llm`
- 代码：`internal/llm/**`
- 任务前缀：LLM
- 状态：规划中，尚未开始模块分支开发
- LLM-001：静态协议盘点完成；在线真实端点验收未执行

只负责 Go 侧模型服务协议适配，复用现有独立模型服务；不重建推理 Gateway，不管理研究状态机、工具权限或业务结论。

设计文档：[01-模型适配与工具边界设计](01-模型适配与工具边界设计.md)；协议调查：[03-现有模型Gateway协议盘点](03-现有模型Gateway协议盘点.md)；Port/DTO 契约：[04-ProviderPort与DTO字段级契约](04-ProviderPort与DTO字段级契约.md)；JSON/SSE Fixture 清单：[05-JSON与SSE契约Fixture清单](05-JSON与SSE契约Fixture清单.md)。

目标结构：
```text
internal/llm/
  adapter/ protocol/ output/ streaming/
```

职责：Provider/Model Adapter、协议转换、超时/限流/错误分类、结构化输出、流式响应、用量/延迟元数据和脱敏。Tool Registry/白名单/权限/预算由 Researchflow 管理；LLM 不执行模型工具调用。

详细任务见 [02-任务拆分与验收](02-任务拆分与验收.md)。
