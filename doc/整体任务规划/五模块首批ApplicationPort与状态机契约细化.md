# 五模块首批 Application Port 与状态机契约细化

- 所属治理任务：GOV-004
- 规划分支：feat/design_doc
- 日期：2026-11-05
- 状态：五模块首批接口设计基线；CONTRACT-006/007 已由项目统一决策关闭，其余契约按项目决议状态推进；不代表代码实现或自动化验收完成。
- 适用范围：五模块第一批独立开发任务。

## 1. 设计约束

1. 模块边界只暴露 Application Port 和公开 DTO；Domain Entity、Repository、SQL Model、Provider SDK 类型均为私有。
2. 所有跨模块调用均由调用方传递 Context、Request ID、deadline 和授权上下文；不得依赖隐式全局状态。
3. DTO 使用稳定 JSON Schema 语义。Go struct 的字段名、omitempty 或内部类型不能自行决定公开契约。
4. 本文只定义首批可用的最小接口。未进入首批范围的字段登记为 deferred，不通过任意扩展 map 预留。
5. 同进程模块调用仍遵守与 HTTP 一致的契约语义，不能因同一 Go Module 而直接传递对方 Domain 类型。

## 2. Quant 首批 Application Port

### 2.1 公共计算上下文

建议定义 QuantAnalysisContext，作为各分析请求的组合字段，不单独暴露为可任意构造的全局上下文服务。

| 字段 | 类型 | 必需 | 约束 |
|---|---|---:|---|
| request_id | string | 是 | 跨模块关联 ID |
| as_of | timestamp | 是 | 系统知识截止时间，UTC |
| trade_date | date | 条件必需 | 交易日语义，Asia/Shanghai |
| period | enum/string | 是 | 使用稳定周期代码 |
| input_snapshot_refs | string[] | 是 | 可为空数组仅限纯算法测试；生产分析至少一个 |
| adjustment_basis | enum | 条件必需 | 价格序列分析必需，unknown 不可作为有效复权 |
| quality_policy | enum | 是 | strict / allow_degraded |
| algorithm_version | string | 否 | 请求指定时必须可识别；否则由服务端选择并在结果回显 |
| deadline_at | timestamp | 是 | UTC 绝对截止时间 |

每个 SnapshotRef 的实际内容至少可追溯：数据类型、证券/范围、交易日、来源和来源版本、复权口径、单位、质量、采集/标准化/系统观察时间。Quant Domain 不直接消费 Data HTTP DTO。

### 2.2 Indicator 首批请求

建议 IndicatorRequest：

| 字段 | 类型 | 必需 | 说明 |
|---|---|---:|---|
| context | QuantAnalysisContext | 是 | 公共计算上下文 |
| security_id | string | 条件必需 | 单证券指标必需 |
| indicator_codes | string[] | 是 | 稳定指标代码，至少一个 |
| input_series | enum/string | 是 | close/open/high/low/volume/amount 等受控序列 |
| window | integer | 条件必需 | 对窗口类指标必需，范围由指标定义约束 |
| parameters | typed indicator-specific object | 否 | 每个 indicator_code 对应独立版本化参数 Schema，不允许任意 map |
| output_policy | enum | 是 | full / latest_only |

规则：
- 一个请求包含多个指标时，必须能逐指标返回成功、数据不足或参数错误；不能因一个可选指标失败而丢失其他成功结果。
- 窗口长度不足时返回 insufficient_data，不补零、不复制首值。
- 停牌、缺失交易日、复权切换和异常值处理必须由指标定义声明，不由调用方猜测。
- 指标代码与参数 Schema 版本必须可追溯。

### 2.3 Factor 首批请求

建议 FactorRequest：

| 字段 | 类型 | 必需 | 说明 |
|---|---|---:|---|
| context | QuantAnalysisContext | 是 | 公共计算上下文 |
| factor_codes | string[] | 是 | 版本化因子代码 |
| universe_ref | string | 是 | 证券全集/筛选结果引用 |
| classification_ref | string | 否 | 行业分类体系及版本 |
| normalization | enum | 是 | none / winsorize / zscore / rank 等明确支持的算法 |
| neutralization | enum | 是 | none / industry / market 等 |
| missing_policy | enum | 是 | reject / exclude_with_coverage |
| minimum_coverage | decimal/number | 是 | [0,1] 范围，精度由契约版本固定 |

规则：
- 因子横截面必须绑定 universe_ref、trade_date、分类版本和可用数据时点。
- 输出需包含有效样本数、全集样本数、覆盖率、排除原因和算法版本。
- 缺失样本不能按 0 进入标准化；排除样本必须可解释。
- 首批不定义任意用户脚本或动态表达式执行。

### 2.4 最小 Quant Facade

逻辑签名建议：

- CalculateIndicators(ctx, IndicatorRequest) -> IndicatorResult
- CalculateFactors(ctx, FactorRequest) -> FactorResult
- EvaluateRisk(ctx, RiskEvaluationRequest) -> RiskDecision
- GetResult(ctx, ResultQuery) -> QuantResult

约束：
- Application Port 不暴露 Repository。
- Result Query 必须校验 result_ref 归属、权限和版本。
- 同一 idempotency_key + 相同规范化请求返回同一逻辑结果；相同 key 对应不同请求应返回冲突错误。
- EvaluateRisk 遇到规则缺失、输入质量不足或未知状态时不得 pass。

## 3. Research 首批 Application Port

### 3.1 AuthorizationRef

Research 不持有 News/Data 的授权策略，但必须保存来源方的可审计授权决策引用。

建议 AuthorizationRef：

| 字段 | 类型 | 必需 | 说明 |
|---|---|---:|---|
| owner | enum | 是 | data / news |
| decision_ref | string | 是 | 不透明、不可变的决策引用 |
| policy_ref | string | 是 | 策略及版本引用 |
| content_level | enum | 是 | metadata / excerpt / summary / full_text |
| evaluated_at | timestamp | 是 | 授权评估时间 |
| scope_ref | string | 是 | 被授权对象/用途范围引用 |
| expires_at | timestamp | 否 | 授权到期时间；无到期语义时省略 |

AuthorizationRef 只是决策引用，不代表永久授权。历史研究读取时仍需来源模块检查当前撤销/过期策略。Research 不自行推导 Allowed。

### 3.2 EvidenceDraft 与 EvidenceRecord

区分写入命令和已持久化记录，避免客户端伪造 evidence_id/version。

EvidenceDraft：
- evidence_type：fact / reported_fact / inference / hypothesis / unknown
- source_owner、source_ref、source_version
- source_locator（可选）
- published_at、available_at、observed_at
- system_as_of
- subject_refs
- stance：supports / contradicts / neutral
- quality_status、limitations
- authorization_ref
- content_fingerprint、summary（受内容级别约束）

EvidenceRecord 在 Draft 基础上由 Research 分配 evidence_id、version、created_at，并保留 append-only 修订关系。

### 3.3 最小 Research Facade

逻辑签名建议：

- CreateContext(ctx, CreateContextCommand) -> ResearchContext
- AddEvidence(ctx, AddEvidenceCommand) -> EvidenceRecord
- GetContext(ctx, ContextRef) -> ResearchContext
- GetEvidence(ctx, EvidenceRef) -> EvidenceRecord
- ListEvidence(ctx, EvidenceQuery) -> EvidencePage

ListEvidence 约束：
- 必须带 system_as_of；不可用 published_at 替代。
- 查询条件限制为主体引用、来源 Owner、证据类型、业务时间窗和质量状态。
- page_size 有上限；排序键固定为 observed_at + evidence_id，使用不透明 Cursor。
- Cursor 绑定查询指纹、system_as_of、排序版本和最后一条唯一键；查询条件改变时 Cursor 失效。
- 不接受任意 URL 或抓取指令。

News Evidence Query 仍由 News 提供来源事实和内容；Research Evidence Facade 只管理研究侧 Evidence/Context，不取代 News 只读查询 Facade。

## 4. Researchflow 首批 Application Port 与状态机

CONTRACT-006/007 的完整状态迁移、传播矩阵、重试、取消/deadline 竞争和 Executor 规则，以 [03-CONTRACT-006与007最终决议](../researchflow/03-CONTRACT-006与007最终决议.md) 为唯一权威定义。

首批 Facade：
- CreateRun(ctx, CreateRunCommand) -> ResearchRun
- GetRun(ctx, RunRef) -> ResearchRunDetail
- CancelRun(ctx, CancelRunCommand) -> ResearchRun
- RetryStage(ctx, RetryStageCommand) -> ResearchStage（仅供 Scheduler 推进 retry_wait，不是人工绕过预算的重跑入口）
- GetWorkflowTemplate(ctx, TemplateRef) -> WorkflowTemplate

核心约束：Run/Stage 使用 version/CAS；Attempt 终态不可变；Stage.retry_wait 由版本化 retry_policy 控制；Run 终态事务必须确认所有 Stage 终结且无 running Attempt。硬风险门禁和 actionability_status 分离；租约 fencing 不代替下游副作用幂等。

## 5. LLM 首批 Port 的冻结边界

LLM-001 已完成现有 Gateway 静态协议盘点：客户端主协议为 OpenAI Responses，调用入口 POST /v1/responses。当前本地 llama.cpp/Qwen 真实端到端尚未验收；详细证据见 doc/llm/03-现有模型Gateway协议盘点.md。

当前已确认的协议边界：
- Base URL 以 /v1 结尾；调用路径为 /responses；调用方使用 Gateway API Key，Secret 通过服务端 SecretRef 注入。
- ModelRequest 使用 instructions + input_text；首批不把 Chat Completions messages 作为内部默认结构。
- 输出 token 参数使用 max_output_tokens；temperature/top_p/reasoning_effort 仅在当前 Model Capability=supported 时发送。
- Responses JSON 需解析 output item 类型、status、incomplete_details 和 usage；usage 缺失保持 unknown/null。
- Responses SSE 需识别文本 delta、OutputItem/ContentPart、function_call arguments delta、refusal 和 completed/incomplete/failed/error terminal event；断流且没有 terminal event 不算成功。
- text.format 目前只能确认可透传，不能认定本地模型 structured output 已通过；Adapter 必须本地 Schema 校验。
- tools 可以透传，但 LLM 不执行；Researchflow 负责授权、参数校验、预算、执行和结果回填。
- 当前 Gateway 本地路径会合并 instructions 与 input 中 system/developer 项，并可能注入 stream_options.include_usage；Adapter Fixture 必须覆盖这些改写。
- ID 明确区分：request_id（调用方生成并回显）、correlation_id（跨模块关联）、gateway_request_id（Gateway X-Request-ID）、upstream_request_id（上游 Request-Id）。

LLM-002 当前建议接口：
- Generate(ctx, ModelRequest) -> (ModelResponse, error)
- Stream(ctx, ModelRequest, onEvent func(ModelEvent) error) -> (ModelResponse, error)

Stream 使用同步回调提供背压，并返回最终聚合 Response。Go error 表示 Adapter 执行错误；合法上游 incomplete/failed 由 Response.status/error 表达。无 terminal event 的 EOF 返回部分 Response 与 stream_interrupted error。详细 DTO、CapabilityState、Responses SSE event、Error Contract 和 Owner 未决项见 doc/llm/04-ProviderPort与DTO字段级契约.md。

LLM-001 静态盘点已完成，在线真实调用未执行；LLM-002 Port/DTO 草案待 LLM/Platform Owner Review。Stream 的生产可用状态必须由实际本地上游 E2E 决定；若不支持应标记 unsupported，不得通过一次性响应伪装增量事件。Tool Call 作为结构化输出返回，不由 LLM 执行。

## 6. Notification 首批 Application Port

### 6.1 NotificationEvent

| 字段 | 类型 | 必需 | 说明 |
|---|---|---:|---|
| event_id | string | 是 | 上游生成或由事件所有者生成的稳定 ID |
| event_type | string | 是 | 稳定事件类型 |
| schema_version | string | 是 | Payload Schema 版本 |
| occurred_at | timestamp | 是 | 业务事件发生时间 |
| source_module | enum | 是 | quant / research / researchflow |
| subject_ref | string | 是 | 业务主体不透明引用 |
| idempotency_key | string | 是 | 业务事件到投递命令的幂等依据 |
| payload | typed object | 是 | 必须匹配 event_type + schema_version |
| correlation_id | string | 是 | Run/请求关联 ID |

payload 不允许无约束 map 直接进入模板。事件版本升级不得静默改变既有字段含义；消费者对未知 schema_version 应拒绝或进入明确的兼容处理路径。

### 6.2 最小 Notification Facade

- PublishEvent(ctx, NotificationEvent) -> PublishResult
- CreateSubscription(ctx, SubscriptionCommand) -> Subscription
- UpdateSubscription(ctx, UpdateSubscriptionCommand) -> Subscription
- ListDeliveries(ctx, DeliveryQuery) -> DeliveryPage
- RetryDelivery(ctx, RetryDeliveryCommand) -> DeliveryRecord

规则：
- PublishEvent 只接受已注册的 event_type/schema_version；相同 source_module + event_id 且 fingerprint 相同返回原结果，不同 fingerprint 返回冲突。
- recipient_scope 是受控类型，不是发送地址；recipient_ref 只能由已授权 Subscription/Channel 配置解析。
- Event Inbox 幂等与 Delivery 幂等分开；Delivery 唯一键建议包含 event、subscription/version、channel/config version 和 recipient_ref。
- DeliveryPlan 固定 Subscription/Channel/Template 版本；发送前仍检查当前退订和授权状态。
- 每次实际渠道调用追加 DeliveryAttempt；租约过期且无法证明请求未发出时进入 unknown。
- Delivery 状态需区分 retry_wait、blocked、skipped、failed 和 unknown；unknown 不允许盲目重试。
- 外部渠道不承诺 exactly-once；RetryDelivery 必须授权、幂等和审计。
- 详细状态机、重试、Schema 注册和 Port 见 doc/notification/03-ApplicationPort与投递状态契约.md。

## 7. 跨模块 Port 依赖表

| 调用方 | 提供方 | 首批调用 | 边界 |
|---|---|---|---|
| Researchflow | Quant | CalculateIndicators / CalculateFactors / EvaluateRisk / GetResult | 只传 Quant Application DTO |
| Researchflow | Research | CreateContext / AddEvidence / GetContext / GetEvidence | 不访问 Research Repository |
| Researchflow | LLM | Generate（Stream 按协议盘点结果决定） | 只传模型协议 DTO |
| Researchflow | Notification | PublishEvent | 只传版本化事件 |
| Quant | Data | HTTP/OpenAPI Adapter | 不 import Data internal |
| Research | News | Research Evidence Query Port | 来源授权与知识时间由 News 执行 |
| 全部模块 | Platform | Fault / Config / Storage / Observability 等稳定能力 | Platform 不依赖业务领域 |

## 8. 尚需 Owner 决策的事项

| ID | 决策 | 建议 Owner | 影响 |
|---|---|---|---|
| CONTRACT-001 | Quant 价格/金额 decimal string 或 number 精度策略 | Quant | Quant DTO、JSON golden |
| CONTRACT-002 | Indicator 首批指标代码与各自参数 Schema | Quant | QNT-001~003 |
| CONTRACT-003 | Factor 首批因子、母体和标准化参数 | Quant | QNT-003 |
| CONTRACT-004 | AuthorizationRef 字段和当前撤销检查责任 | Research + News | Research Evidence、News Query |
| CONTRACT-005 | system_as_of 在 Context/Evidence/Run 的唯一传递规则 | Research + Researchflow | 历史研究一致性 |
| CONTRACT-006 | Stage 结果传播矩阵、Run partial/degraded 语义与硬风险门禁策略 | 项目统一决策方 | closed；权威决议见 Researchflow/03 |
| CONTRACT-007 | cancel/deadline/retry 竞争、cancelling 收敛、租约与 fencing 语义 | 项目统一决策方 | closed；权威决议见 Researchflow/03 |
| CONTRACT-008 | LLM Gateway 真实协议调查 | LLM | ModelRequest/Response |
| CONTRACT-009 | Notification Inbox/Delivery 幂等、recipient_scope、状态与未知结果恢复 | Notification + Platform | PublishEvent/Subscription/Delivery/Attempt |
| CONTRACT-010 | 事件 Payload Schema 注册、不可变版本与兼容策略 | Notification + 各事件 Owner | PublishEvent/Template |

## 9. 评审和冻结要求

本项目由统一决策方作出架构与契约决议，不再将不存在的独立模块 Owner 签字作为设计冻结门槛。每项决议至少记录：
- 决策 ID 与结论；
- Owner 和 Review 日期；
- DTO/Schema 版本；
- 正向与反向 JSON 示例；
- required/optional/nullable 行为；
- 兼容性和迁移策略；
- 契约测试与 golden 的位置。

Owner 未确认的字段不得标为冻结。可以先冻结没有争议的 DTO 子集，并将未决字段隔离在独立能力/Adapter 中。

**当前结论：本文件定义首批 Application Port 的规划基线。CONTRACT-006/007 设计决策已关闭；其余契约须按项目决议逐项关闭。任何设计决议关闭均不代表 Go DTO、JSON Schema、Golden、自动化测试或 L2 Module Ready 已通过。**
