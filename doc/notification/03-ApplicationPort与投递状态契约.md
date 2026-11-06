# Notification Application Port 与投递状态契约

- 归属：Notification 模块，NTF-001/004/005/006
- 对应项目契约：NTF-CTR-001、NTF-CTR-002、NTF-CTR-003（历史别名 CONTRACT-009/010）
- 状态：Notification 字段级契约输入；当前状态和阻断范围以 `doc/整体任务规划/跨模块契约目录与冻结状态.md` 为准
- 日期：2026-11-06
- 代码边界：`internal/notification/**`；根 DI、HTTP、Worker、Migration 注册由 Integration Owner 负责

## 1. 目标与非目标

Notification 接收版本化业务事件，按已授权 Subscription 生成投递计划，使用固定版本的 Template/Channel 执行投递，保存可审计的 Delivery 与 Attempt 历史，并对可确认的临时失败实施有界重试。

Notification 不计算告警条件、不决定研究结论、不执行模型输出中的接收人指令，也不承诺外部渠道 exactly-once。外部发送是不可回滚副作用；本地数据库事务只能保证本地事件/投递状态一致。

## 2. 核心对象与版本边界

### 2.1 NotificationEvent

必需字段：
- `event_id`：事件所有者生成的稳定、不透明 ID；同一逻辑 NotificationEvent 重放时不得变化；ResearchEvent.event_id 等领域事件 ID 不得直接复用为 NotificationEvent.event_id，领域来源通过 payload 中受控的 source_event_ref 表达。
- `event_type`、`schema_version`：注册表中的精确键。
- `source_module`：受控枚举 quant/research/researchflow；首批映射见 `doc/notification/05-首批业务事件映射决议.md`。
- `occurred_at`：业务事件发生时间，不作为投递时间。
- `subject_ref`：业务主体不透明引用。
- `idempotency_key`：调用方对本次发布意图提供的稳定键；重试 PublishEvent 时不得变化。
- `payload`：必须通过该 event_type + schema_version 对应的 Schema 校验。
- `correlation_id`：跨模块关联 ID。

接收规则建议：
1. 对 payload 做规范化后计算 fingerprint，覆盖 source_module、event_type、schema_version、subject_ref、occurred_at 和 payload；使用 RFC 8785 JSON Canonicalization Scheme 的 UTF-8 结果计算 SHA-256；不把 event_id、idempotency_key、correlation_id、接收时间或传输 Header 纳入 payload fingerprint。
2. Inbox 唯一约束为 `(source_module, event_id)`；发布幂等唯一约束为 `(source_module, idempotency_key)`；同一 source_module 范围内该键全局唯一，不按 event_type 分桶，生产方不得跨发布意图复用。
3. 相同 source_module/event_id 且 fingerprint 相同：返回原接收结果，不重复 fan-out。
4. event_id 或 idempotency_key 已存在但 fingerprint/事件身份不一致：返回 idempotency_conflict，不覆盖旧事件。
5. 首次接收时持久化事件与 fan-out 工作项；如果需要跨模块本地事务，由调用方 Outbox 与 Notification Inbox 形成可靠交付，不依赖跨领域 Repository 事务。

### 2.2 Subscription

建议字段：
- subscription_id、owner_ref、version；
- status：active/paused/disabled；
- event_type 与该事件类型对应的受限过滤条件；
- recipient_scope、recipient_ref 来源；
- channel_ref、template_id；
- created_at、updated_at、effective_at。

Subscription 更新采用版本递增和不可变版本快照。任何影响匹配、目标、渠道或模板的更新都必须递增 version。用户退订/禁用是当前授权状态，发送前必须重新检查；历史 Delivery 保留原版本，不被重写。

### 2.3 DeliveryPlan 与 DeliveryRecord

每个 Delivery 在入队时固定本次投递计划快照：
- source_module + event_id；
- subscription_id + subscription_version；
- channel_ref + channel_config_version；
- template_id + template_version；
- recipient_ref（由已授权 Subscription/Channel 解析得到）；
- payload_fingerprint、plan_fingerprint；
- delivery_id、delivery_idempotency_key。

Delivery 唯一键固定为：
`(source_module, event_id, subscription_id, subscription_version, channel_ref, channel_config_version, template_id, template_version, recipient_ref)`。

所有版本均属于计划快照，不能在原版本上原地修改。

Template 与 Channel 配置版本始终属于唯一键和 Delivery 计划快照；任何影响渲染/发送行为的变更都必须创建新版本，禁止原版本原地修改。Channel 只保存版本化 Secret 引用，不在 Delivery 中复制 Secret 明文。

默认不对历史事件做新订阅回补。若未来提供 backfill，必须使用独立显式命令、权限和幂等范围，不得由普通 PublishEvent 重放隐式触发。


### 2.4 Subscription 字段级契约

| 字段 | JSON 类型 | 必填/nullable | 约束 |
|---|---|---|---|
| subscription_id | string | 必填 / non-null | 不透明稳定 ID |
| owner_ref | string | 必填 / non-null | 订阅所有者引用，不是收件地址 |
| version | integer | 必填 / non-null | 从 1 开始单调递增；每个版本不可变 |
| status | enum | 必填 / non-null | active/paused/disabled |
| event_type | string | 必填 / non-null | 单个已注册事件类型；MVP 不在一个 Subscription 内混合不同事件 Schema，多个事件分别创建 Subscription |
| filter_schema_version | string/null | 必填字段 / nullable | 非 null 时必须匹配该 event_type 的受控过滤 Schema |
| filters | object/null | 必填字段 / nullable | null 表示无附加过滤；非 null 必须按 filter_schema_version 校验，禁止任意 map |
| recipient_scope | enum | 必填 / non-null | subscription_owner/preconfigured_target |
| recipient_ref | string/null | 必填字段 / nullable | preconfigured_target 必填不透明目标引用；subscription_owner 必须为 null，DeliveryPlan 阶段从 owner_ref 解析；不存敏感地址 |
| channel_ref | string | 必填 / non-null | 不透明渠道引用 |
| template_id | string | 必填 / non-null | 不透明模板 ID |
| created_at / updated_at / effective_at | RFC3339Nano UTC | 必填 / non-null | 创建、更新、当前版本生效时间 |

更新 Subscription 必须产生新 version；旧版本只读保留。单条 Subscription 只绑定一个 event_type；filters=null 时 filter_schema_version 必须为 null；filters 非 null 时 filter_schema_version 必填且必须由该 event_type 注册。不能以通用 JSON 查询语言执行任意逻辑。

### 2.5 DeliveryRecord 与 DeliveryAttempt 字段级契约

**DeliveryRecord：**

| 字段 | JSON 类型 | 必填/nullable | 约束 |
|---|---|---|---|
| delivery_id | string | 必填 / non-null | 不透明稳定 ID |
| source_module | enum | 必填 / non-null | quant/research/researchflow，来源事件生产模块 |
| event_id | string | 必填 / non-null | 在 source_module 范围内稳定唯一的来源事件 ID |
| subscription_id / subscription_version | string/integer | 必填 / non-null | 创建 Delivery 时固定 |
| channel_ref / channel_config_version | string | 必填 / non-null | 创建 Delivery 时固定 |
| template_id / template_version | string | 必填 / non-null | 创建 Delivery 时固定 |
| recipient_ref | string | 必填 / non-null | 已授权的稳定收件目标引用，不含明文地址 |
| payload_fingerprint / plan_fingerprint | string | 必填 / non-null | 规范化 Payload 与完整投递计划的 SHA-256 指纹 |
| delivery_idempotency_key | string | 必填 / non-null | 同一 Delivery 全部 Attempt 稳定不变 |
| status | enum | 必填 / non-null | Delivery 状态机枚举 |
| version | integer | 必填 / non-null | 从 1 开始，每次 CAS 状态迁移递增 |
| attempt_count / max_attempts | integer | 必填 / non-null | 非负 / 正整数；attempt_count 仅统计真实渠道调用 |
| next_attempt_at | RFC3339Nano UTC/null | 必填字段 / nullable | 仅 retry_wait 有值，其余状态为 null |
| deadline_at | RFC3339Nano UTC | 必填 / non-null | 最晚允许继续尝试的绝对截止时间 |
| reason_code / last_error_code | string/null | 必填字段 / nullable | 稳定机器码，不拼接原始异常 |
| provider_message_ref | string/null | 必填字段 / nullable | 渠道明确返回时设置 |
| created_at / updated_at | RFC3339Nano UTC | 必填 / non-null | 创建和最后状态更新时间 |

**DeliveryAttempt：**

| 字段 | JSON 类型 | 必填/nullable | 约束 |
|---|---|---|---|
| attempt_id | string | 必填 / non-null | 不透明追加式 ID |
| delivery_id | string | 必填 / non-null | 关联 Delivery |
| attempt_no | integer | 必填 / non-null | 从 1 开始递增，包含每次真实渠道调用 |
| worker_id | string | 必填 / non-null | 执行 Worker 引用 |
| fencing_token | integer | 必填 / non-null | 单调递增；旧 token 永久失效 |
| lease_expires_at | RFC3339Nano UTC | 必填 / non-null | 当前执行租约截止 |
| started_at / finished_at | RFC3339Nano UTC/null | 必填字段 / nullable | finished_at 在 Attempt 结束前为 null |
| outcome | enum/null | 必填字段 / nullable | accepted/rejected/ambiguous/not_sent；运行中为 null |
| provider_message_ref | string/null | 必填字段 / nullable | 渠道回执引用 |
| error_code | string/null | 必填字段 / nullable | 稳定错误码 |
| retryable | boolean | 必填 / non-null | 技术建议，不覆盖 Channel retryable 白名单 |
| duration_ms | integer/null | 必填字段 / nullable | 结束后非负毫秒值；运行中 null |
| evidence_ref | string/null | 必填字段 / nullable | 不透明诊断证据引用 |

Attempt 完成后不可覆盖；结果纠正通过新对账事实追加。DeliveryRecord 是当前投递状态快照，DeliveryAttempt 是不可变执行历史，两者不能合并成同一可覆盖记录。


## 3. recipient_scope 与安全边界

recipient_scope 不是地址，也不是 recipient_ref。建议 MVP 仅允许：
- `subscription_owner`：从 Subscription.owner_ref 解析；
- `preconfigured_target`：从已授权 Channel 配置解析。

不接受任意 URL、邮箱、手机号、用户 ID 或模型生成文本作为直接目标。若未来增加 scope，必须注册类型、定义解析 Owner、授权检查和审计字段。接收人解析必须产生稳定 recipient_ref；敏感地址只在受控 Channel Adapter 内解析。

发送前重新校验：
- Subscription 当前未退订/禁用；
- recipient_ref 当前仍在授权范围；
- Channel 当前可用且 Secret 可读取；
- 测试发送的目标是调用方显式指定并通过 allowlist 的目标。

未匹配订阅/过滤条件不创建 Delivery，记录受控的 fan-out 统计；因用户退订而不再发送的已排队 Delivery 转为 skipped，并记录 reason_code。不能将系统依赖不可用伪装成 skipped。

## 4. Delivery 状态机

状态定义：
- queued：已持久化，等待执行。
- sending：Worker 已取得租约，正在进行发送前检查或调用渠道。
- retry_wait：渠道明确未接受，且错误可重试；等待 next_attempt_at。
- sent：渠道明确确认接受；不等同于最终用户已阅读。
- skipped：根据业务/授权规则有意不发送，例如退订、过滤不匹配、目标不再授权。
- blocked：因缺少必要配置、Secret、权限或依赖而暂不能发送；尚未产生可确认的外部发送副作用。
- failed：永久错误，或重试预算耗尽且最终结果明确。
- unknown：请求可能已被渠道接受，但本地未取得可靠确认；必须对账或依赖渠道幂等能力，不得盲目重发。

允许迁移：
- queued -> sending / skipped / blocked
- blocked -> queued / skipped / failed
- sending -> sent / retry_wait / failed / skipped / unknown（转 skipped 仅限可证明渠道调用尚未发出；请求可能已到达渠道时必须转 unknown）
- retry_wait -> sending / skipped / blocked / failed
- unknown -> sent / retry_wait / failed（仅通过对账或渠道幂等协议给出有证据的裁决）
- failed -> queued 仅允许显式、授权、审计的人工重试命令；必须重新校验策略预算。
- sent、skipped 为终态；不得通过普通 RetryDelivery 回退。
- unknown 不是普通可重试失败，不能直接转 queued。

`blocked` 表示执行前置条件未满足，不消耗渠道发送 Attempt；`skipped` 表示规则决定不发送；`failed` 表示有明确失败结论；`unknown` 表示外部副作用结果不确定。四者不可合并为一个 error 状态。

状态变更必须通过 Repository 条件更新/CAS 完成，写入状态版本、reason_code、changed_at 和必要的 evidence_ref。非法迁移返回稳定冲突错误，不静默覆盖。

## 5. Attempt、租约与并发恢复

DeliveryAttempt 是追加式执行历史，至少记录：
- attempt_id、delivery_id、attempt_no；
- worker_id、lease/fencing token；
- started_at、finished_at；
- outcome：accepted/rejected/ambiguous/not_sent；
- provider_message_ref（如有）、error_code、retryable、duration；
- 安全摘要和受控 evidence_ref。

每次真实渠道调用创建一个新 Attempt。完成后的 Attempt 结果不可覆盖；修正通过新的对账记录/状态事件追加。

Worker 取得 Delivery 时必须原子完成 queued/retry_wait -> sending 和租约分配。租约过期后：
- 若可证明请求尚未发送，允许释放回 queued/retry_wait；
- 若请求可能已发出，必须转 unknown；
- 旧 Worker 的迟到完成写入必须因 fencing token 失效而被拒绝。

同一 Delivery 同时最多一个有效发送租约。并发上限按 Channel/Provider 设有界容量，不允许无界 goroutine 或无限队列。

## 6. 重试与外部 exactly-once 边界

- 只对渠道明确拒绝且标记 retryable 的结果自动重试。
- 永久错误不重试；存在有效 Retry-After 时，next_attempt_at = max(本地 full-jitter 退避时间, Retry-After 指定时间)，不得缩短本地退避，也不得达到或越过 Delivery deadline；若计算结果不早于 deadline_at，则结束为 failed（reason_code=deadline_exceeded），不再安排重试。
- 使用 capped exponential full jitter；策略固定 max_attempts、max_elapsed_ms、per_attempt_timeout_ms、base_delay_ms、max_delay_ms 和总 deadline。所有 duration 为正整数 milliseconds；max_elapsed_ms/per_attempt_timeout_ms/base_delay_ms>0，max_delay_ms>=base_delay_ms。MVP 算法固定 exponential_full_jitter_v1：第 n 次 Attempt 失败后的上限 delay=min(max_delay_ms, base_delay_ms*2^(n-1))，实际等待从 [0,delay] 均匀抽取。单次调用截止=min(now+per_attempt_timeout_ms, deadline_at, first_attempt_at+max_elapsed_ms)。
- attempt_count 只统计实际发起的渠道调用，不统计 Schema/权限/配置预检查。
- 同一 Delivery 的渠道 idempotency key 在各 Attempt 间保持稳定；仅在渠道明确支持幂等时发送该键。
- 超时、连接中断或进程崩溃发生在请求可能已到达渠道之后时，标记 unknown；只有渠道查询/回执/幂等重放能证明安全时才可自动收敛。
- 不支持幂等且无法查询结果的渠道，unknown 进入人工/运维对账，不自动重发。
- Outbox + Inbox 可保障本地事件接收可靠性；不能声称外部渠道 exactly-once。实际语义是受限 at-least-once 尝试 + 幂等/对账控制重复风险。

## 7. RetryDelivery 与取消/退订竞态

RetryDelivery 必须包含 delivery_id、operator/owner 身份、reason、request_id/idempotency_key。命令本身幂等；相同命令重放返回同一结果。默认仅允许对明确 failed 且策略允许的 Delivery 发起；unknown 必须先对账，sent/skipped 不允许重试。人工超出原预算的 override 必须单独权限、审计和明确风险提示。

退订/禁用：
- queued/retry_wait：在事务内转 skipped。
- blocked：重新评估后转 skipped 或保留 blocked 并记录原因。
- sending：尝试取消尚未发出的本地工作；如果外部请求已发出，不能承诺撤回，最终按渠道确认或 unknown 收敛。
- 已 sent：不回滚、不删除历史。

## 8. Payload Schema 与模板版本策略

NTF-CTR-001（历史别名 CONTRACT-010）规则：
1. Schema 注册键为 `(event_type, schema_version)`，注册后不可原地修改；变更语义必须新增版本。
2. 注册信息包含 schema_digest、owner_module、status、registered_at、compatibility_policy。
3. PublishEvent 在持久化前校验 payload；未知版本、Schema 不可用、校验失败均明确拒绝，不做隐式字段补全或静默降级。
4. 模板绑定明确 event_type/schema_version 和变量 allowlist；模板版本不可变。
5. 渲染仅暴露已声明字段，不将整个 payload map 无约束传给模板；默认 HTML 转义，限制渲染后字节数和嵌套深度。
6. 向后兼容规则按事件类型单独声明；新增可选字段是否兼容必须通过 Schema diff 和消费者测试，不仅依赖“字段是 optional”的推断。
7. 未知事件版本不自动尝试旧 Schema；由事件 Owner 发布迁移/兼容策略。

## 9. Application Port 建议

```go
type NotificationApplication interface {
    PublishEvent(ctx context.Context, event NotificationEvent) (PublishResult, error)
    CreateSubscription(ctx context.Context, command CreateSubscriptionCommand) (Subscription, error)
    UpdateSubscription(ctx context.Context, command UpdateSubscriptionCommand) (Subscription, error)
    DisableSubscription(ctx context.Context, command DisableSubscriptionCommand) error
    ListDeliveries(ctx context.Context, query DeliveryQuery) (DeliveryPage, error)
    GetDelivery(ctx context.Context, deliveryID DeliveryID) (DeliveryDetail, error)
    RetryDelivery(ctx context.Context, command RetryDeliveryCommand) (DeliveryRecord, error)
}
```

PublishResult 应明确 accepted/duplicate；冲突通过稳定错误返回。不要让上游直接调用 Channel Adapter 或 Repository。诊断/对账入口按权限独立暴露，不把运维能力塞入普通 PublishEvent。

## 10. NTF 任务验收映射

| 任务 | 模块级验收 | L3 集成验收 |
|---|---|---|
| NTF-001 | Event Schema 注册/校验、Inbox 幂等、冲突、Subscription 版本和 recipient_scope | 上游 Outbox -> Notification Inbox 真实事务/故障恢复 |
| NTF-002 | 固定模板版本、变量 allowlist、转义、缺失变量、大小/深度限制 | 真实渠道渲染内容核验 |
| NTF-003 | Channel Port、错误分类、目标解析、Secret Port Fake 测试 | 真实凭据/渠道受控发送；无凭据 skipped/blocked |
| NTF-004 | 有界 Dispatcher、状态机、CAS、租约/fencing、单 Delivery 单活 Worker | 真实 Worker 并发和进程崩溃恢复 |
| NTF-005 | retryable/permanent/ambiguous 分类、退避预算、Retry-After、unknown 不盲重试 | 真实渠道超时/回执丢失/幂等行为验证 |
| NTF-006 | Repository Port、唯一约束、Outbox/Inbox Schema 设计 | MariaDB 事务、唯一冲突、锁竞争、Migration/恢复演练 |
| NTF-007 | Delivery 查询、脱敏诊断、reason_code 和审计 | 根 HTTP/权限/监控接入 |


### 8.1 NTF-CTR-001 Event Envelope 与 Fingerprint

首批 Envelope 与 Payload Schema 文件：
- `schema/notification-event-envelope-1.0.0.schema.json`：校验 Envelope 字段，并按 event_type/schema_version/source_module 选择 Payload Schema。
- `schema/researchflow-run-terminal-1.0.0.schema.json`
- `schema/quant-risk-decision-changed-1.0.0.schema.json`
- `schema/research-event-lifecycle-changed-1.0.0.schema.json`

上述文件是 NTF-CTR-001/INT-CTR-001 的机器可读设计基线；本地 Draft 2020-12 Validator 已校验当前 Schema/Golden；模块 CI 仍需直接加载仓库文件执行 Validator、Golden、Registry 不可变性与兼容矩阵测试。


首批 Event Envelope 字段：

| 字段 | JSON 类型 | 必填/nullable | 约束 |
|---|---|---|---|
| event_id | string | 必填 / non-null | 生产方生成的稳定 ID；同一逻辑事件重放不得变化 |
| event_type | string | 必填 / non-null | 全局注册名，由 Registry 校验 |
| schema_version | string | 必填 / non-null | SemVer，精确命中注册项 |
| source_module | enum | 必填 / non-null | MVP 为 quant/research/researchflow；新增生产模块须先登记并评审 |
| occurred_at | RFC3339Nano UTC | 必填 / non-null | 业务事实发生时间，不是发布或接收时间 |
| subject_ref | string | 必填 / non-null | 不透明业务主体引用 |
| idempotency_key | string | 必填 / non-null | 发布命令稳定键，重试时保持不变 |
| correlation_id | string | 必填 / non-null | 跨模块关联，不作为去重键 |
| payload | object | 必填 / non-null | 由 event_type + schema_version 唯一 Schema 校验 |

Payload fingerprint 使用 RFC 8785 JSON Canonicalization Scheme 的 UTF-8 结果计算 SHA-256，覆盖 event_type、schema_version、source_module、subject_ref、occurred_at 和 payload。event_id/idempotency_key 不进入 payload fingerprint，而分别参与 Inbox/Publish 唯一性约束。Decimal string 不得规范化为 JSON number。

### 8.2 NTF-CTR-002 幂等与 Delivery 计划快照

- Inbox 唯一键：`(source_module, event_id)`。
- Publish 幂等唯一键：`(source_module, idempotency_key)`；同一 source_module 范围内 idempotency_key 全局唯一，不按 event_type 分桶，生产方不得跨发布意图复用。
- 相同唯一键且 fingerprint 相同，返回原接收/发布结果，不重复 fan-out；相同唯一键不同 fingerprint 返回 idempotency_conflict。
- Delivery 唯一键：`(source_module, event_id, subscription_id, subscription_version, channel_ref, channel_config_version, template_id, template_version, recipient_ref)`。
- delivery_idempotency_key 在同一 Delivery 全部 Attempt 间稳定，作用域为 channel_ref + recipient_ref + delivery_id；只有 Channel Capability 已验证 supports_idempotency=true 时才发送。
- Subscription、Channel、Template 任一影响匹配、目标、渲染或发送行为的变更必须产生新版本；历史 Delivery 保留原版本。
- Outbox/Inbox 只保证业务事件到 Notification 的本地可靠接收，不代表外部渠道 exactly-once。

Delivery plan fingerprint 使用 RFC 8785 JSON Canonicalization Scheme + SHA-256，覆盖 source_module/event_id、subscription_id/version、channel_ref/config_version、template_id/version、recipient_ref、payload_fingerprint 和授权策略版本；不覆盖 delivery_id、创建时间、Attempt、lease 和可变状态。相同 Delivery 唯一键且 plan_fingerprint 相同返回原 Delivery；唯一键相同但 fingerprint 不同视为契约冲突，不覆盖旧计划。

### 8.3 NTF-CTR-003 状态迁移与渠道能力

- 状态迁移通过 Delivery.version CAS；每次成功迁移递增 version，并追加不可变状态事件。
- sending -> skipped 仅在可证明渠道调用尚未发出时允许；请求可能到达渠道时必须进入 unknown 或等待明确回执，不得以退订/取消覆盖外部副作用不确定性。
- unknown -> retry_wait 仅在对账证明未接受，或已验证同一幂等键安全重放时允许；否则保持 unknown。
- max_attempts 包含首次发送，必须为正整数；max_elapsed_ms、per_attempt_timeout_ms、base_delay_ms、max_delay_ms 和总 deadline 必须显式配置且满足上面的范围，不设隐式全局重试默认值。
- 自动重试仅针对渠道明确 rejected 且 reason_code 在该 Channel 的 retryable 白名单内；有效 Retry-After 与本地退避取较晚时间（不得缩短本地退避）；若 next_attempt_at 不早于总 deadline，则不再重试并按 deadline_exceeded 结束。ambiguous、timeout-after-write、连接中断且无法证明未发送均进入 unknown。
- Channel Capability 必须逐渠道注册 supports_idempotency、supports_delivery_lookup、supports_retry_after、max_payload_bytes；未验证能力按 false/unknown 处理。
- sent 仅表示渠道明确接受，不代表最终用户已送达/阅读。若渠道提供 delivered/read 回执，使用独立 DeliveryReceipt 事实，不改写 sent 语义。


## 11. 项目统一契约决议与证据状态

| 新契约 | 已接受语义 | 当前状态 |
|---|---|---|
| NTF-CTR-001 Event Schema Registry | event_type + schema_version 不可变注册键；生产方拥有 payload 语义；未知版本拒绝，不回退旧 Schema | pending_evidence：首批 Schema/Golden 已提交，仍待模块 CI、Registry digest 不可变和兼容矩阵测试 |
| NTF-CTR-002 Inbox/Publish/Delivery 幂等与状态机 | Inbox=(source_module,event_id)；Publish=(source_module,idempotency_key)；Delivery 唯一键含 source_module/event/subscription/channel config/template/recipient；unknown 先对账，不盲目重发 | pending_evidence：测试矩阵已定义，仍待模块 CI、DB 唯一约束、并发/重复命令/恢复测试 |
| NTF-CTR-003 Channel 重试与外部副作用 | 不承诺外部 exactly-once；仅明确未被渠道接受且可重试的错误自动重试；ambiguous outcome 进入 unknown | pending_evidence：错误分类/unknown/Retry-After 测试矩阵已定义，仍待模块 CI 与逐渠道能力声明 |



**本文件保留 Notification 领域字段与状态机细节；项目级语义和契约状态以统一契约目录为准。当前 NTF-CTR-001~003 均未达到 frozen；不代表任何真实渠道或 L3 集成验收已通过。**
