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

### 4.1 状态枚举与职责

Run：
- 非终态：created、queued、running、cancelling。
- 终态：succeeded、partial、failed、cancelled。
- 不设置 Run.degraded 或 Run.timed_out：degraded 是阶段输出质量，timed_out 是一次执行结果；Run 只表达整体完成度/最终控制结果。

Stage：
- pending、ready、running、succeeded、degraded、skipped、failed、timed_out、cancelled、blocked。
- blocked 表示调度前置条件或依赖输出不满足，且本次 Run 内不再具备启动条件；它不是一次 Attempt，也不创建 Attempt。
- skipped 仅表示模板明确裁剪/判定不适用的阶段，必须有 reason_code；运行期依赖失败不得伪装为 skipped。

Attempt：
- running、succeeded、degraded、failed、timed_out、cancelled。
- Attempt 记录一次真实执行，创建后只允许从 running 写入一次终态；终态记录不可修改或重开。
- Stage 的 failed/timed_out 在仍有重试预算时属于可重试中间结果；耗尽预算后才参与 Run 最终归并。

### 4.2 合法迁移约束

| 对象 | 起始状态 | 允许迁移 |
|---|---|---|
| Run | created | queued / cancelled |
| Run | queued | running / cancelling / cancelled |
| Run | running | cancelling / succeeded / partial / failed |
| Run | cancelling | cancelled / succeeded / partial / failed（仅 deadline_exceeded 收敛） |
| Stage | pending | ready / skipped / blocked / cancelled / timed_out |
| Stage | ready | running / cancelled / blocked / timed_out |
| Stage | running | succeeded / degraded / failed / timed_out / cancelled |
| Stage | failed / timed_out | ready（仅 RetryStage 校验通过后） |
| Attempt | running | succeeded / degraded / failed / timed_out / cancelled |

约束：
- Run、Stage、Attempt 的状态写入必须通过带前置状态条件的原子更新；重复请求返回已提交状态，不重复产生副作用。
- Stage 的 skipped/blocked/cancelled 为本次 Run 的终态；failed/timed_out 在重试预算尚有剩余且 Run deadline 未到达时可通过显式 RetryStage 回到 ready。Run deadline 到达产生的 timed_out 不得重试。
- Stage degraded 表示本次执行有可用输出但质量受限，默认不自动重试；是否重试必须由模板策略显式允许。
- RetryStage 必须校验 Run=running、未进入 cancelling、Run deadline 尚未到达、Stage 具备可重试失败结果、依赖输出仍有效、重试次数/总预算未耗尽。每次重试创建 attempt_no + 1，复用原 Run.as_of 和输入快照。
- 不允许对已终态 Run 重开；需要重新研究时创建新 Run，并可通过 parent_run_ref 关联原 Run。

### 4.3 Stage 结果到 Run 的归并（CONTRACT-006）

模板必须为每个 Stage 显式声明结果传播策略，至少包括：
- on_degraded：accept / partial / fail；
- on_skipped：no_impact / partial / fail；
- on_failed：partial / fail；
- on_timed_out：partial / fail；
- on_blocked：partial / fail。
策略按 stage_code + template_version 固定并纳入 fingerprint；不得使用运行时隐式默认值。required Stage 不允许配置 on_skipped=no_impact；若创建 Run 前判定不适用，应从本次 DAG 中裁剪并在 resolved_template_snapshot 中记录 excluded_stage_code 与 reason_code。

归并规则：
1. 先确定每个 Stage 的最终状态；failed 只有重试预算耗尽后才作为最终结果，deadline 到达产生的 timed_out 直接参与归并。
2. 任一 Stage 的传播结果为 fail，Run=failed。
3. 没有 fail、但至少一个 Stage 传播结果为 partial，Run=partial。
4. 所有 Stage 均完成且传播结果均为 accept/no_impact，Run=succeeded。
5. Run 的 result_summary 必须同时包含 stage_outcomes、missing_stage_codes、degraded_stage_codes、blocked_stage_codes、reason_codes 和 coverage；partial 不能只用布尔值表达。
6. Stage degraded 不等于 Run partial：模板可选择 accept，此时 Run 可 succeeded，但 Run/结果质量摘要仍必须标记 degraded；选择 partial 时才影响 Run 完成度。
7. 可选 Stage skipped 只有模板明确声明 no_impact 时才不降低完成度；因运行期依赖不可用而跳过必须用 blocked 或 failed，并按传播策略归并。
8. 硬风险门禁阶段不得配置可忽略失败/降级的策略；RiskDecision=unknown 或门禁未执行不得被归并为通过。

### 4.4 取消、deadline 与竞争语义（CONTRACT-007）

- Run deadline_at 是绝对 UTC 时间；所有 Stage deadline 为 min(stage_budget_deadline, Run.deadline_at)，所有重试共用同一 Run deadline，不得续期。
- 显式取消在 Run 尚未到 deadline 且 Run 非终态时，以原子状态迁移进入 cancelling，并设置 termination_reason=user_cancelled。进入 cancelling 后立即拒绝新 Stage/Attempt 和 RetryStage，未开始 Stage 标记 cancelled，向运行中 Attempt 传播 context cancellation。
- Run deadline 到达且尚未有更早被接受的 user_cancelled 请求时，设置 termination_reason=deadline_exceeded，停止派发并取消在途执行；达到预算的 Attempt/Stage 记录 timed_out。
- 若 user_cancelled 在 deadline 前先被原子接受，则取消原因保持不变；deadline 之后只负责强制收敛超时 Worker，不得把 Run 的取消原因改写为 deadline_exceeded。
- Run 进入 cancelling 后，等待运行中 Attempt 完成收敛；Attempt 通过原子终态写入记录 cancelled，或保留在取消线性化点前已提交的真实完成结果。已写入终态的 Attempt 不得被覆盖；取消后迟到的输出不得回写为成功结果。
- termination_reason=user_cancelled 时，所有在途 Attempt 收敛、Stage 封存后 Run 转为 cancelled；termination_reason=deadline_exceeded 时，按 CONTRACT-006 的模板策略归并为 failed 或 partial。不得出现 Run 已终态但仍有 running Attempt。
- 取消与正常完成竞态采用数据库原子 CAS/事务作为线性化点：先成功提交 Run 终态或 cancelling 的操作获胜。若正常归并先提交，后续 Cancel 返回 terminal_conflict；若 cancel 先进入 cancelling，则正常归并不得提交 succeeded/partial/failed。deadline 检查在控制操作和最终归并时执行。
- context cancellation 只是协作信号，不代表 Attempt 已停止；Worker 必须在有界 grace period 内确认结束。超过 grace period 由租约/看门狗回收，旧 Worker 的迟到写入通过 fencing token 拒绝。

### 4.5 Executor 容量、租约与恢复约束

- Executor 必须配置全局 worker 上限、全局排队容量、单 Run 并行 Stage 上限、单 Stage 执行预算和 Run 总 deadline。
- 排队容量已满时 CreateRun 返回明确可重试的 capacity_exceeded，不创建 queued Run；相同 idempotency_key + 相同请求指纹返回原 Run，不同指纹返回冲突。
- queued Run/Stage 状态必须持久化；进程重启后可重新派发。running Attempt 使用带 owner/fencing token/lease_expires_at 的执行租约，Worker 定期续租。
- 租约过期后由唯一回收者原子收敛旧 Attempt，按重试策略创建新 Attempt；旧 Worker 的完成写入因 fencing token 过期而被拒绝，避免双执行覆盖。
- 重试只由 Researchflow Application 统一决策和计数；Executor/Adapter 不得再叠加业务重试。网络客户端仅允许其明确安全的有限传输重试。
- DAG 无环；就绪 Stage 按稳定优先级/创建序排序，容量不足时保持 ready/queued，不得无界创建 goroutine 或无限内存队列。

### 4.6 最小 Researchflow Facade

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
| CONTRACT-006 | Stage 结果传播矩阵、Run partial/degraded 语义与硬风险门禁策略 | Researchflow | 状态机和结果归并 |
| CONTRACT-007 | cancel/deadline/retry 竞争、cancelling 收敛、租约与 fencing 语义 | Researchflow | Run 终态一致性 |
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
