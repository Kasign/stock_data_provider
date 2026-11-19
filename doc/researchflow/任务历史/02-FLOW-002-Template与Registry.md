# FLOW-002：Workflow Template/Registry

## 阶段结论

通用 Template/Registry 机制已实现：DAG/拓扑校验、Applicability、Retry/Propagation、HardRiskGate fail-closed、JCS 指纹、参数 Schema 不可变快照、资源策略快照和精确版本读取；Stage Output Registry 精确绑定 Stage/OutputType/SchemaVersion，并校验 Envelope 身份及强类型 Payload/QualitySummary。

## 已有证据

- `internal/researchflow/template/**`
- `internal/researchflow/parameterschema/**`
- `internal/researchflow/stageoutput/**`
- `internal/researchflow/storage/mysql_template_registry.go` 及对应测试
- Server 生产装配：`internal/server/bootstrap/news_search_workflow_template.go`
- Server Stage 生产装配：`internal/server/bootstrap/news_search_stage.go`
- Runtime Worker 只注入该 News Search Executor 与其精确 Stage Output Registry：`internal/server/bootstrap/researchflow_runtime.go`

## 2026-11-18 生产注册盘点

当前 Server Composition Root 只发布一个真实生产 Template：

- Template：`news.search_candidates@1.0.0`
- Stage：`search_candidates`，Required=true，无依赖
- Parameter Schema：`news-search-parameters@1.0.0`
- Applicability：`news.search_candidates.always@1.0.0`
- ResourcePolicy：`news.search.configured-providers@1.0.0`，快照只包含实际装配的 Provider Code
- Output Registry：`1.0.0`，只登记该 Stage 对应的 OutputType/SchemaVersion/PayloadSchema/QualitySummarySchema
- Retry：max_attempts=2、exponential_full_jitter_v1、base=100ms、max=1000ms、attempt_timeout=30s
- Propagation：degraded=accept；skipped/failed/timed_out/blocked=fail
- `RequiresHardRiskGate=false`：这是 News 搜索/发现流程，不是投资决策或交易建议流程，因此不虚构 HardRiskGate。任何未来输出投资决策的 Template 必须声明真实注册的 HardRiskGate，并由 Template.Validate 强制 required + fail-closed。

检查结论：

1. 当前 Template 与 Stage/Output Registry 真实对应，没有为 Data/Quant/Research/LLM/Context 缺失能力注册占位条目。
2. 当前 Worker 生产执行器只注入 News Search Stage；没有证据表明 Context、Quant、Research 或 LLM Stage 已进入 Server 生产 Runtime。
3. 这不是通用 Registry 机制缺失，而是其他领域 Stage Provider/Adapter 尚未完成生产接入；新增 Template 必须以实际 Provider、Schema/Golden、消费者和端到端测试为前提。
4. 当前 News-only Template 不应被描述成完整股票研究分析流程；其范围是检索候选新闻。

## 未完成边界

通用注册机制已完成，当前生产注册盘点只确认 News Search 单 Stage。仍需逐项盘点并接入实际 Data/Quant/Research/LLM/Context Stage；不得为缺失依赖注册占位条目。项目级 FLOW-CTR-004 Envelope 登记簿与模块 Schema/Golden/Registry 的状态差异由项目契约 Owner/Integration 核对，不在模块分支自行修改项目登记簿。

## 验收状态

- 通用 Template/Registry/Output Registry：已实现并有模块测试。
- News Search 生产 Template/Stage/Output Registry：已在 Server Composition Root 装配。
- 其他领域 Stage：未生产注册，等待实际 Provider/Adapter。
- 完整股票研究流程：未完成，不能由 News-only Template 代表。

## 原始记录

完整历史执行记录保留于 [各个子任务完成状态.md](./各个子任务完成状态.md)。

## 2026-11-18 News Search Payload Validator 一致性 Review

- 生产 Output Registry 将 `newsstage.ValidatePayloadJSON` 注册为 `news.search_candidates` Payload Validator。
- 权威 Schema `doc/news/schema/NEWS-STAGE-search-candidates-payload-1.0.0.schema.json` 对 `candidate.missing_fields` 声明 `uniqueItems: true`。
- `internal/news/stage/search_candidates_validation.go::ValidatePayloadJSON` 当前逐项检查 missing field 枚举，但没有检查重复值；因此重复项可通过代码 Validator，却违反冻结 Schema。
- 这是 News Owner 的领域 Validator 缺口，不在 Researchflow 分支复制校验逻辑；卡点见 RF-BLOCK-009。修复前该 Payload Validator 不能标记为完全符合 Schema。