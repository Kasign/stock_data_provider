# FLOW-002：Workflow Template/Registry

## 阶段结论

通用 Template/Registry 机制已实现：DAG/拓扑校验、Applicability、Retry/Propagation、HardRiskGate fail-closed、JCS 指纹、参数 Schema 不可变快照、资源策略快照和精确版本读取；Stage Output Registry 精确绑定 Stage/OutputType/SchemaVersion，并校验 Envelope 身份及强类型 Payload/QualitySummary。

## 已有证据

- `internal/researchflow/template/**`
- `internal/researchflow/parameterschema/**`
- `internal/researchflow/stageoutput/**`
- `internal/researchflow/storage/mysql_template_registry.go` 及对应测试
- News Search 正式 Template/Stage 的生产接入证据见 Integration 交接记录。

## 未完成边界

通用注册机制不代表所有领域 Stage 已生产注册。仍需逐项盘点实际 Template、HardRiskGateRegistry、Output Registry Entry、生产 Provider、Schema/Golden 和真实消费者；不得为缺失依赖注册占位条目。

## 原始记录

完整历史执行记录保留于 [各个子任务完成状态.md](./各个子任务完成状态.md)。
