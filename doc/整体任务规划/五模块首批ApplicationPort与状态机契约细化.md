# 五模块首批 Application Port 与状态机契约细化

- 所属治理任务：GOV-004
- 规划分支：feat/design_doc
- 日期：2026-11-05
- 状态：接口设计草案，待对应模块 Owner Review；不得视为已冻结或已实现。
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

### 4.1 状态枚举建议

Run：
- created
- queued
- running
- succeeded
- partial
- failed
- cancelled

Stage：
- pending
- ready
- running
- succeeded
- degraded
- skipped
- failed
- timed_out
- cancelled
- blocked

Attempt：
- running
- succeeded
- degraded
- failed
- timed_out
- cancelled

Attempt 是不可变执行历史；重试创建 attempt_no + 1。不得将 Attempt 的 timed_out 直接映射成 Run timed_out，Run 的最终状态由必需/可选阶段策略和取消语义决定。

### 4.2 合法迁移约束

| 对象 | 起始状态 | 允许迁移 |
|---|---|---|
| Run | created | queued / cancelled |
| Run | queued | running / cancelled |
| Run | running | succeeded / partial / failed / cancelled |
| Stage | pending | ready / skipped / blocked / cancelled |
| Stage | ready | running / cancelled |
| Stage | running | succeeded / degraded / failed / timed_out / cancelled |
| Attempt | running | succeeded / degraded / failed / timed_out / cancelled |

终态不可直接重开。重试必须由新的 Attempt 表达；Stage 从失败/超时进入下一次执行前，Application 必须校验重试策略、剩余 deadline 和次数上限，并记录新的 Attempt。若需要重新运行已终态 Run，应创建新的 Run 并引用原 Run。

### 4.3 Run 结果归并

- 必需 Stage failed/timed_out/blocked：Run failed，除非显式的模板策略定义了可恢复路径。
- 可选 Stage skipped/degraded/failed：允许 Run partial；必须列出 stage_code、reason_code 和缺失输出。
- 主动取消：Run cancelled；未开始 Stage 标记 cancelled 或按策略保留 pending 并在 Run 结束时封存，规则需统一。
- 所有必需阶段成功且无可选阶段缺失：Run succeeded。
- Stage degraded 是否导致 Run partial，由 Template 对该 Stage 的质量传播策略显式决定，不能靠通用默认值隐藏。
- timeout 是执行预算耗尽；cancel 是外部取消请求；两者 reason_code 不同。

### 4.4 最小 Researchflow Facade

逻辑签名建议：

- CreateRun(ctx, CreateRunCommand) -> ResearchRun
- GetRun(ctx, RunRef) -> ResearchRunDetail
- CancelRun(ctx, CancelRunCommand) -> ResearchRun
- RetryStage(ctx, RetryStageCommand) -> ResearchStage
- GetWorkflowTemplate(ctx, TemplateRef) -> WorkflowTemplate

CreateRun 必须固定 template_id/version/fingerprint、system_as_of、输入快照引用和调用者范围。RetryStage 不允许绕过硬风险门禁或改变原 Run 的知识时点。

## 5. LLM 首批 Port 的冻结边界

LLM-001 先盘点现有 Gateway 协议，不在调查前冻结完整 DTO。盘点表至少覆盖：

- endpoint/path 与 wire protocol；
- 认证 Header/Secret 读取方式；
- model 名称和模型别名；
- messages/input 的实际结构；
- temperature/top_p/max_tokens 等参数支持情况；
- JSON Schema / structured output 支持程度；
- streaming 事件类型、结束标记和异常中断；
- tool call 返回结构及其是否仅为请求；
- usage 字段、计费单位和缺失语义；
- 429/5xx/timeout/connection reset/invalid response 错误；
- deadline、客户端取消和服务端取消行为；
- provider request ID、model version 和响应时间；
- 最大输入/输出、响应体大小和并发限制。

协议调查输出后，再冻结最小 ModelRequest/ModelResponse。初始 Port 目标为：
- Generate(ctx, ModelRequest) -> ModelResponse
- Stream(ctx, ModelRequest) -> Stream[ModelEvent]

若当前 Gateway 不支持真正流式，Stream 应标记 unsupported，不得通过一次性响应伪装增量事件。Tool Call 作为结构化输出返回，不由 LLM 执行。

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
- PublishEvent 只接受已注册的 event_type/schema_version。
- recipient_scope 必须是受控类型；真实 recipient_ref 由订阅和渠道配置解析，不接受模型输出或任意上游文本直接指定。
- 相同 event_id + subscription_version + channel_ref 的有效投递只产生一次；唯一键具体组成需在 Repository 设计评审中确认。
- 每次发送重试追加 DeliveryAttempt，不能覆盖历史。
- 渠道未配置、用户未授权或订阅不匹配应产生明确 skipped reason；系统依赖故障应 blocked/failed，不能伪装 skipped。
- 通知投递失败不回滚已完成研究结果。

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
| CONTRACT-006 | Run partial 与 Stage degraded 的模板传播矩阵 | Researchflow | 状态机和结果归并 |
| CONTRACT-007 | Cancel 时未开始 Stage 的封存状态 | Researchflow | Run 终态一致性 |
| CONTRACT-008 | LLM Gateway 真实协议调查 | LLM | ModelRequest/Response |
| CONTRACT-009 | Notification recipient_scope 与投递幂等唯一键 | Notification | Subscription/Delivery |
| CONTRACT-010 | 事件 Payload Schema 注册与兼容策略 | Notification | PublishEvent |

## 9. 评审和冻结要求

每项 Owner 决策至少记录：
- 决策 ID 与结论；
- Owner 和 Review 日期；
- DTO/Schema 版本；
- 正向与反向 JSON 示例；
- required/optional/nullable 行为；
- 兼容性和迁移策略；
- 契约测试与 golden 的位置。

Owner 未确认的字段不得标为冻结。可以先冻结没有争议的 DTO 子集，并将未决字段隔离在独立能力/Adapter 中。

**当前结论：本文件把首批 Application Port 与状态机细化到可评审粒度；仍待 Owner Review，不代表接口实现或契约冻结。**
