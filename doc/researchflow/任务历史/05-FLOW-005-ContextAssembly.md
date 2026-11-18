# FLOW-005：Context Assembly 与 Tool Policy

## 阶段结论

Context 分区规划、稳定排序、Token Budget、必选/受保护内容 fail-closed、Research DTO 投影、Tokenizer Port/Provenance、最终输入复核、Tool Registry/Scope/参数 Schema/调用预算和 LLM Tool Call 意图适配已有模块实现。

## 已有证据

- `internal/researchflow/contextassembly/**`
- `internal/researchflow/toolpolicy/**`
- `doc/researchflow/05-FLOW-005上下文装配实施说明.md`

## 未完成边界

没有证据表明生产 Research Facade/Owner 授权 Adapter、目标模型真实 Tokenizer 和真实 LLM 调用已经闭环。测试 Tokenizer 只证明接口边界，不代表生产 token 计数准确。授权由 Research Owner 复核，Researchflow 不访问 Research Repository。
