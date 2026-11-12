# Notification Application Port 与投递状态契约

- 归属：Notification 模块，NTF-001/004/005/006
- 对应项目级契约：NTF-CTR-001、NTF-CTR-002；NTF-CTR-003 为 `not_applicable`，Channel Adapter/Resolver 属 Notification 模块内部 SPI。
- 状态：模块字段级契约与 Application Port 权威定义；项目级 L1 状态以 `doc/contract/PLAT-002-项目级契约冻结登记簿.md` 为准。NTF-CTR-001/002 已 frozen；L1 冻结不代表 L2 CI/实现符合性或 L3 真实依赖验收通过。
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
- `occurred_at`：业务事实发生时间，不作为投递时间。
- `published_at`：生产方将事件写入 Outbox 的发布时间；与 occurred_at 分离，重放时保持不变，不纳入 payload fingerprint。
- `payload_fingerprint`：按 NTF-CTR-001 指定字段和 RFC 8785 计算的小写十六进制 SHA-256（64 字符），由生产方计算并随 Envelope 传输；接收方必须重新计算并比较。
- `subject_ref`：业务主体不透明引用。
- `idempotency_key`：调用方对本次发布意图提供的稳定键；重试 PublishEvent 时不得变化。
- `payload`：必须通过该 event_type + schema_version 对应的 Schema 校验。
- `correlation_id`：跨模块关联 ID。

接收规则：
0. Registry 构造必须显式提供正数 ResourceLimits：MaxEnvelopeBytes、MaxPayloadBytes、MaxJSONDepth；不得使用未审批的隐式默认值。ValidateEvent 先检查原始 Envelope 字节数，再严格解析 JSON（重复对象键、非法 UTF-8、非法 Unicode surrogate、非法 JSON 和超过深度均拒绝），再检查原始 Payload 字节数，之后才允许 JCS 规范化、Schema 和指纹计算。深度按整个 JSON 文档中的 object/array 容器层级计数，Envelope 根对象计为 1；字节上限按原始输入字节计，包含空白和转义文本。超限返回稳定 resource-limit 错误，不截断、不降级。项目级 Review 建议的 Envelope 1 MiB / Payload 768 KiB / depth 32 尚未被统一批准，不能作为模块默认值或 frozen 契约；由集成配置显式注入最终值，并确保 Envelope 上限不超过 HTTP 入口限制。
1. 对 payload 做规范化后计算 fingerprint，覆盖 source_module、event_type、schema_version、subject_ref、occurred_at 和 payload；使用 RFC 8785 JSON Canonicalization Scheme 的 UTF-8 结果计算 SHA-256；不把 event_id、idempotency_key、correlation_id、published_at、接收时间或传输 Header 纳入 payload fingerprint。Envelope 必须携带 payload_fingerprint；接收方重新计算并拒绝不匹配的值。
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
- authorization_policy_versions：完整策略版本 tuple 数组，每项包含 policy_type、policy_ref、policy_version；不得只保留 policy_version 字符串；
- authorization_refs：对应 AuthorizationRef 的不透明审计引用；不是 bearer token，不参与 plan_fingerprint。
- delivery_id、delivery_idempotency_key。

Delivery 唯一键固定为：
`(source_module, event_id, subscription_id, subscription_version, channel_ref, channel_config_version, template_id, template_version, recipient_ref)`。

所有版本均属于计划快照，不能在原版本上原地修改。 Application 写入前必须先独立执行 DeliveryPlan.Validate()，再执行 VerifyFingerprint()；任一步失败均不得调用 Repository。Repository Adapter 仍须在自身事务边界再次验证唯一键和指纹，避免绕过 Application 的直接写入路径。

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

Attempt 生命周期边界：
- `started_at` 必须不早于 Delivery.created_at，且严格早于 Delivery.deadline_at；截止时刻及之后不得启动新的渠道请求。
- 渠道请求在截止时间前启动后，允许在 Delivery.deadline_at 之后返回；Attempt.finished_at 不因晚于 Delivery Deadline 而无效。
- `lease_expires_at` 用于 Worker 租约 fencing，与发送截止独立，可以晚于 Delivery.deadline_at。实际渠道调用 deadline 仍由单次超时、首次 Attempt 起算的 MaxElapsed 和 Delivery Deadline 共同取最早值。


## 3. recipient_scope 与安全边界

recipient_scope 不是地址，也不是 recipient_ref。建议 MVP 仅允许：
- `subscription_owner`：从 Subscription.owner_ref 解析；
- `preconfigured_target`：从已授权 Channel 配置解析。

不接受任意 URL、邮箱、手机号、用户 ID 或模型生成文本作为直接目标。若未来增加 scope，必须注册类型、定义解析 Owner、授权检查和审计字段。接收人解析必须产生稳定 recipient_ref；敏感地址只在受控 Channel Adapter 内解析。

发送前重新校验：
- Subscription 当前未退订/禁用；
- recipient_ref 当前仍在授权范围；
- Delivery 固定的 ChannelConfigVersion 必须精确解析，不能回退 latest；
- Channel 当前可用，具体 Adapter 必须在构造/解析阶段通过 Platform Config.Secret 取得所需 Secret；
- 测试发送的目标是调用方显式指定并通过 allowlist 的目标。

`application.DeliveryPreflight` 已实现只读发送前复核：重新读取当前 Subscription，缺失/暂停/禁用时返回 skip；按 Delivery 固定的 channel_ref + config_version 解析 Adapter，缺失版本/禁用配置时返回 block；通过 RecipientResolver 重新解析当前授权目标，recipient_ref 不再匹配或显式未授权时返回 skip；依赖故障返回 block 类结果和原始 error，不能伪装为用户退订。Subscription 仅版本递增但仍 active、且原 recipient_ref 仍获授权时，历史 Delivery 继续使用原 Channel 版本与原投递快照，不重写 DeliveryPlan。该函数不发送、不写状态、不证明真实 Secret 已可读取；实际 Secret 可用性仍由具体 ChannelAdapter/Resolver 在真实集成时证明。

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


### 4.1 DeliveryRecord 与状态事件的 Go 领域实现

当前 `internal/notification/domain/delivery.go` 实现 DeliveryRecord 的结构校验和纯状态迁移；它不执行数据库写入，也不代替 Repository CAS。

| 字段组 | 来源 | 语义/约束 |
|---|---|---|
| Delivery 身份与订阅/渠道/模板/接收目标版本 | 已校验 DeliveryPlan 快照 | 创建后不可变；不从最新配置重新推导历史计划 |
| payload_fingerprint / plan_fingerprint / delivery_idempotency_key | DeliveryPlan | 复用已验证指纹和稳定幂等键，不在状态迁移时重算 |
| status / version | Notification 状态机 | version 从 1 开始，每次成功迁移递增；持久化时必须 CAS |
| attempt_count / max_attempts | 真实渠道调用计数 / 显式 RetryPolicy | 仅真实渠道调用增加 attempt_count；max_attempts 必须显式配置 |
| next_attempt_at / deadline_at | Retry 调度器 / Delivery 创建时显式截止时间 | 仅 retry_wait 有 next_attempt_at；必须为 UTC 且早于 deadline_at |
| reason_code / last_error_code | Notification 领域决策 / Adapter 归一化错误 | 稳定机器码，不包含原始异常、响应正文或敏感目标 |
| provider_message_ref | 渠道明确返回的回执引用 | 不透明引用；accepted/sent 语义不等于最终送达或阅读 |
| created_at / updated_at | 注入的系统 Clock | RFC3339Nano UTC；状态变更时间不得倒退 |

DeliveryStateEvent 的 version/from/to/changed_at 来自每次状态迁移；evidence_ref 仅来自对账证据；actor_ref/request_id/actor_reason 仅来自经授权的人工命令审计上下文。领域方法返回新快照和事件，不修改原对象；Repository 必须在同一事务中 CAS 更新快照并追加事件。

Repository 持久化前必须再次校验 StateEvent 与目标 Delivery 快照：delivery_id/version/to/changed_at/reason_code 必须一致，事件迁移必须满足领域状态机；自动迁移不得携带 actor_ref/request_id/actor_reason，只有 failed -> queued 的授权人工重试允许这些审计字段。该校验必须在事务开始前完成，且追加事件时再次执行领域事件校验。

DeliveryRecord 领域模型与纯函数迁移测试、DeliveryRepository Port/Fake、DeliveryAttempt 领域快照、Attempt Repository、租约超时恢复、单条 DeliveryExecutionService 和 FanoutDispatcher 均已有实现。Fanout Dispatcher 只负责把 Inbox Event 展开为 DeliveryPlan；真实 FanoutPlanFactory 装配、Delivery Worker 生命周期和 Server 根 Migration 注册仍属集成任务。代码与测试存在不代表当前 HEAD CI 或真实 MariaDB L3 已通过。


### 4.2 DeliveryRepository Port 与 Fake 的第一阶段实现

当前 `internal/notification/application/ports.go` 定义 DeliveryRepository；`delivery_repository_test.go` 提供内存 Fake。接口调用场景仅限 Notification Dispatcher：创建初始 Delivery、读取状态、领取到期任务，以及持有有效租约的 Worker 完成状态迁移。

| 接口/字段 | 来源 | 语义与边界 |
|---|---|---|
| Create(delivery) | 已校验 DeliveryPlan + 显式 max_attempts/deadline_at + Clock | 只接受 queued、version=1、attempt_count=0 的初始快照；DeliveryID 唯一 |
| Get(delivery_id) | Delivery 稳定 ID | 返回深拷贝快照；不存在时 found=false，不伪造空记录 |
| ClaimDue(worker_id, now, lease_expires_at) | Dispatcher Worker 身份 + 注入 Clock + 显式租约时长 | 原子选择 queued 或到期 retry_wait；同时迁移 sending、追加状态事件、分配单调 fencing_token；不增加 attempt_count，因为尚未证明已发起渠道调用 |
| DeliveryLease.worker_id | Worker 注册/启动时分配的稳定引用 | 不透明 Worker 标识，不包含主机敏感信息 |
| DeliveryLease.fencing_token | Repository 对每个 Delivery 单调递增计数 | 旧 token 永久失效；不可由调用方指定或回退 |
| DeliveryLease.expires_at | Claim 时的 UTC now + 显式 lease duration | 仅用于当前租约有效性判定；不得晚于 Delivery.deadline_at；过期后不允许旧 Worker 写入 |
| UpdateWithLease(expected_version, fencing_token, next, event, now) | Delivery.Transition 输出 + 当前 Worker Lease | 同一原子操作内校验版本/token/租约、更新快照并追加事件；终止本次 sending 后释放租约 |
| ErrDeliveryVersionConflict / ErrDeliveryLeaseConflict | Repository CAS 判定 | 分别表示快照版本竞争和租约/token 失效；调用方不得将冲突当作可覆盖更新 |

当前 Fake 只证明模块内并发/CAS 语义，不代表 MariaDB 锁、事务、跨进程 fencing 或崩溃恢复已验收。ClaimDue 将租约截止限制为 min(调用方请求截止, Delivery.deadline_at)，不回收过期 sending；后续 Dispatcher Recovery 必须先判断请求是否发出，能证明未发出才可安全重排，结果不明必须进入 unknown。


### 4.3 DeliveryAttempt 追加式领域模型

当前 `internal/notification/domain/delivery_attempt.go` 实现 DeliveryAttempt 快照、结构校验和纯函数完成操作；它不负责分配 attempt_no、不执行渠道调用，也不持久化记录。

| 字段 | 来源 | 语义与约束 |
|---|---|---|
| attempt_id / delivery_id | Notification ID 生成器 / 当前 Delivery | 不透明稳定引用；一条 Attempt 只归属一条 Delivery |
| attempt_no | DeliveryAttempt Repository 按 Delivery 原子分配 | 从 1 开始；每次真实渠道调用递增；发送前检查失败不创建 Attempt |
| worker_id / fencing_token / lease_expires_at | DeliveryRepository 成功 Claim 返回的 Worker 与租约 | 必须关联当前 Claim；token 由 Repository 单调分配，不能由 Worker 自行生成 |
| started_at | 注入 Clock，在真实渠道调用开始边界记录 | UTC；不能用 Claim 时间代替渠道调用开始时间 |
| finished_at / outcome | ChannelAdapter 的受控 Send 结果与注入 Clock | accepted/rejected/ambiguous/not_sent；完成后不可通过 Complete 再次修改 |
| provider_message_ref / error_code | ChannelAdapter 归一化结果 | 仅保存不透明回执引用和稳定错误码，不保存原始异常/响应正文 |
| retryable | Channel 错误分类的技术建议 | 不直接授权重试；必须由 `ChannelSendResult.IsRetryable` 基于精确 Channel 快照白名单得出，再经过 RetryPolicy、预算和 deadline 判断 |
| retry_after_at | ChannelAdapter 验证后的 Retry-After 绝对时间 | 仅 rejected 可携带；只有 supports_retry_after=supported 时采信；持久化供 Worker 崩溃恢复使用 |
| duration_ms | finished_at - started_at | 由领域方法计算，毫秒整数；运行中为 null |
| evidence_ref | 能证明 not_sent 或支持后续对账的受控证据系统 | 不透明引用；not_sent 必须提供证明，不能仅凭普通错误字符串推断未发送 |

`DeliveryAttempt.Complete` 返回新快照，不修改原对象。accepted 不允许携带 error_code/retryable；rejected 和 ambiguous 必须携带稳定 error_code；not_sent 必须同时携带 error_code 与 evidence_ref，且不能携带 provider_message_ref。ambiguous 即使 retryable=true 也不构成自动重试授权。`CompleteWithRetryAfter` 仅为 rejected 保存 UTC `retry_after_at`；实际重试决策由 NTF-005 完成。

本切片已补充 DeliveryAttemptRepository Port/Fake：StartWithLease 原子追加运行中 Attempt 并更新 attempt_count 计数投影；CompleteWithLease 仅允许当前有效租约完成一次；ListByDelivery 返回按序深拷贝。具体原子边界如下：

Attempt Repository 的模块级原子边界：
- StartWithLease：使用 Repository 内部权威时钟（MariaDB Adapter 在事务 SQL 中使用 UTC_TIMESTAMP(6)，Fake 使用可控时钟），校验租约在 now 时刻仍有效、StartedAt 不晚于 now 且不早于本次 sending 领取时间；同时校验当前 Delivery 为 sending、expected version 与 fencing token 匹配、Worker 与租约一致、attempt_no 等于当前 Attempt 数量 + 1，并确保同一 fencing token 最多只能创建一个 Attempt（无论前一 Attempt 是否已完成）；在同一 Repository 原子操作中追加运行中 Attempt 并更新 attempt_count 投影。重试必须先完成 sending -> retry_wait 等状态迁移并释放旧租约，再重新 ClaimDue 获得新 fencing token，不能在同一 claim 下启动第二次渠道调用。不能仅凭调用方传入的 StartedAt 早于 ExpiresAt 推断租约当前仍有效。
- CompleteWithLease：只允许当前有效租约、当前 Delivery 版本和同一 Attempt 身份将运行中记录完成一次；完成时间不得越过租约截止；旧 Worker 的迟到完成必须拒绝。
- ListByDelivery：按 attempt_no 顺序返回深拷贝历史；未知 Delivery 返回 not found。
- attempt_count 是已追加 Attempt 数量的计数投影；StartWithLease 与 Attempt 追加同事务更新，不改变 Delivery 状态版本，也不产生伪造的状态迁移事件。真实 Repository 必须保证 Attempt 行、计数投影和租约检查的事务原子性；建议数据库增加 `UNIQUE(delivery_id, fencing_token)`，并在事务中锁定 Delivery 行，防止同一 claim 并发创建第二个 Attempt。

新增只读 `AssessExpiredDeliveryLease` 恢复评估：要求完整 Attempt 历史与已过期租约；当前 fencing token 无 Attempt 表示外部 Send 尚未开始，可在剩余预算内安全重试；运行中 Attempt 或 ambiguous 结果必须进入 unknown；accepted 可收敛 sent 并保留 provider_message_ref；明确 rejected 按持久化 retryable 标志交给 RetryPolicy 或结束 failed，同时保留 retry_after_at；not_sent 必须保留 evidence_ref。评估函数不执行状态写入，调用方仍须以当前 Delivery version + fencing token 做 CAS，并在同一事务中释放租约/追加状态事件。

当前仅有模块内 Fake；其锁只能证明单进程测试语义，不证明数据库事务、跨进程租约 fencing 或真实进程崩溃恢复。只读评估不等于已实现恢复扫描器或 MariaDB 恢复事务。
Fake/单测代码未获得 CI 通过证据前不得标记模块级验收通过；真实 MariaDB Adapter、Worker 崩溃恢复和真实渠道证据仍未实现。

## 5. Attempt、租约与并发恢复

DeliveryAttempt 是追加式执行历史，至少记录：
- attempt_id、delivery_id、attempt_no；
- worker_id、lease/fencing token；
- started_at、finished_at；
- outcome：accepted/rejected/ambiguous/not_sent；
- provider_message_ref（如有）、error_code、retryable、duration；
- `retry_after_at`：仅当渠道已验证支持 Retry-After，且 Adapter 返回的 rejected 结果通过能力校验后持久化；用于 Worker 崩溃恢复，不能只保存在内存中；
- 安全摘要和受控 evidence_ref。

每次真实渠道调用创建一个新 Attempt。完成后的 Attempt 结果不可覆盖；修正通过新的对账记录/状态事件追加。

Worker 取得 Delivery 时必须原子完成 queued/retry_wait -> sending 和租约分配。租约过期后：
- 若当前 fencing token 没有 Attempt，且 Attempt 历史完整，可证明 StartWithLease 尚未成功，允许在预算内安全重试；
- 若当前 Attempt 仍运行或结果为 ambiguous，必须转 unknown；
- 若 Attempt 已完成为 accepted，恢复为 sent；明确 rejected 根据 Attempt.retryable 与 RetryPolicy 决定重试或 failed；not_sent 必须携带 evidence_ref；
- 旧 Worker 的迟到完成写入必须因 fencing token 失效而被拒绝。

同一 Delivery 同时最多一个有效发送租约。并发上限按 Channel/Provider 设有界容量，不允许无界 goroutine 或无限队列。

## 6. 重试与外部 exactly-once 边界

- 只对渠道明确拒绝且标记 retryable 的结果自动重试。`retryable` 必须来自 `ChannelSendResult.IsRetryable` 对当前精确 ChannelConfigSnapshot 的 retryable_error_codes 白名单判断，不得直接采信 Provider 原始布尔字段。

- 永久错误不重试；存在有效 Retry-After 时，next_attempt_at = max(本地 full-jitter 退避时间, Retry-After 指定时间)，不得缩短本地退避，也不得达到或越过 Delivery deadline；若计算结果不早于 deadline_at，则结束为 failed（reason_code=deadline_exceeded），不再安排重试。
- 使用 capped exponential full jitter；策略固定 max_attempts、max_elapsed_ms、per_attempt_timeout_ms、base_delay_ms、max_delay_ms 和总 deadline。所有 duration 为正整数 milliseconds；max_elapsed_ms/per_attempt_timeout_ms/base_delay_ms>0，max_delay_ms>=base_delay_ms。MVP 算法固定 exponential_full_jitter_v1：第 n 次 Attempt 失败后的上限 delay=min(max_delay_ms, base_delay_ms*2^(n-1))，实际等待从 [0,delay] 均匀抽取。单次调用截止=min(now+per_attempt_timeout_ms, deadline_at, first_attempt_at+max_elapsed_ms)。
- attempt_count 只统计实际发起的渠道调用，不统计 Schema/权限/配置预检查。
- 同一 Delivery 的渠道 idempotency key 在各 Attempt 间保持稳定；仅在渠道明确支持幂等时发送该键。
- 超时、连接中断或进程崩溃发生在请求可能已到达渠道之后时，标记 unknown；只有渠道查询/回执/幂等重放能证明安全时才可自动收敛。
- 不支持幂等且无法查询结果的渠道，unknown 进入人工/运维对账，不自动重发。
- Outbox + Inbox 可保障本地事件接收可靠性；不能声称外部渠道 exactly-once。实际语义是受限 at-least-once 尝试 + 幂等/对账控制重复风险。

当前 `application.RetryPolicy` 提供纯函数重试决策：策略参数必须显式提供，所有 duration 使用正整数毫秒；`ScheduleRetry` 接收完整 Attempt 历史与外部注入的 `[0,1)` 随机样本，校验 Delivery/Attempt 关联、连续序号、Attempt ID 唯一性、fencing token 严格递增和调用时间不重叠；只允许前序 Attempt 为 retryable rejected 或有证据的 not_sent，不允许历史出现 accepted、ambiguous 或 permanent rejected 后继续自动重试；当前 Attempt 必须是明确 rejected 且 retryable；按 `min(max_delay, base_delay*2^(attempt_no-1))` 计算 capped full-jitter 上限，所有小于数据库微秒精度的延迟提升到 1µs，取本地退避与 Retry-After 的较晚时间，并以 `min(deadline_at, first_attempt_at+max_elapsed)` 截止；达到 `max_attempts` 返回 `retry_budget_exhausted`，越过总时间边界返回 `deadline_exceeded`。`AttemptDeadline` 返回 `min(now+per_attempt_timeout, deadline_at, first_attempt_at+max_elapsed)`。

`CompleteAttemptFromChannelResult` 校验精确 ChannelConfigSnapshot 与 ChannelSendResult，映射 accepted/rejected/ambiguous/not_sent；Attempt.retryable 只由 `ChannelSendResult.IsRetryable` 对该 Channel 的 retryable_error_codes 白名单计算；Retry-After 只有在 supports_retry_after=supported 且结果为 rejected 时才能进入 Attempt；not_sent 必须附 evidence_ref。`RetryPolicy` 还校验完整 Attempt 历史的 attempt_no 连续、Attempt ID 唯一、fencing token 严格递增、前一 Attempt 已完成且后续 Attempt 未早于前一完成时间。

`RetryDecision` 绑定 delivery_id、delivery_version、attempt_id、fencing_token 和 evaluated_at，避免跨 Delivery/Attempt/版本复用决策。`ApplyRetryDecision` 校验当前 sending 租约、已完成的 retryable rejected Attempt 和决策绑定后，将结果转换为 retry_wait 或 failed，再调用 `DeliveryRepository.UpdateWithLease`；该 Port 的原子契约要求同一操作 CAS 快照、追加状态事件，并在状态离开 sending 时释放租约。Fake 测试覆盖 retry_wait 与失败收敛、事件追加、租约释放和旧 token/非法决策拒绝。此处仍未实现真实 MariaDB 事务、随机数生成、Worker 调度或渠道端到端验证。

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

### 8.2 Template Definition 与 Renderer

TemplateDefinition 的固定字段为：
- `template_id`、正整数 `version`；
- 精确绑定 `event_type` + `schema_version`；
- `format`：`text` 或 `html`；
- 可选 `subject_template`、必填 `body_template`；
- `variable_bindings`：模板变量名到 payload JSON Pointer 的映射。

版本注册键为 `(template_id, version)`。相同键、相同 JCS digest 可幂等注册；相同键、不同 digest 必须拒绝；内容变化必须增加版本。当前模块 Registry 是内存实现，不代表已有持久化 Template Repository。

模板仅支持直接标量插值 `{{.variable}}`，不支持 if/range/模板嵌套/函数或任意表达式。变量必须显式列入 allowlist，并由 JSON Pointer 从 payload 取值；只允许 string/bool/number，不允许把整个 payload map 传入模板。缺失路径、非标量、未声明变量和未使用绑定均 fail closed。

HTML body 使用 `html/template` 上下文转义；text body 使用纯文本渲染。Subject 作为纯文本输出，拒绝 CR/LF 和其他控制字符。模板定义字节数、合并渲染输出字节数、payload JSON 深度均由显式 Limits 注入；没有项目已批准的默认数值，不允许隐式放开。渲染结果保留 template digest 和 event payload fingerprint，供 Delivery 快照审计。

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

```go
type TemplateRenderer interface {
    Render(ctx context.Context, event NotificationEvent, templateID string, version int64) (RenderedContent, error)
}
```

TemplateRenderer 只接受已通过 Event Schema/Envelope 校验的事件；具体实现仍须重新验证 payload fingerprint、event/schema 精确匹配和渲染资源限制。

### 8.4 NTF-003 Channel Config 与 Adapter Port

Channel 配置是不可变版本快照，注册键为 `(channel_ref, config_version)`。同一键不得原地修改；Resolver 必须精确解析指定版本，不允许回退到 latest。

**ChannelConfigSnapshot：**

| 字段 | 类型 | 约束 |
|---|---|---|
| channel_ref | string | 不透明稳定引用，必填 |
| config_version | integer | 正整数，影响发送行为的配置变更必须递增 |
| channel_kind | string | 适配器注册类型；不由用户输入动态加载代码 |
| enabled | boolean | 当前配置是否允许执行发送 |
| secret_ref | string/null | Platform Secret 引用；不保存 Secret 明文 |
| capabilities | object | supports_idempotency / supports_delivery_lookup / supports_retry_after，取 unknown/supported/unsupported；max_payload_bytes 必须为正整数 |
| retryable_error_codes | array<string> | 渠道适配器归一化后的稳定错误码白名单，去重；不接受原始 HTTP 文本或 Provider 错误描述 |

能力未验证时必须为 unknown；unknown 与 unsupported 一样不能启用对应行为。只有 supports_idempotency=supported 才能向渠道发送稳定 delivery_idempotency_key；只有 supports_retry_after=supported 才能采信 Retry-After；只有 supports_delivery_lookup=supported 才能执行渠道查询对账。

**ChannelConfigRepository：**

Notification 持久化层提供按 `(channel_ref, config_version)` 精确读取的不可变快照 Repository；相同版本相同内容幂等，不同内容冲突。Repository 只保存 `secret_ref`，不保存 Secret 明文。Server 组合根读取快照后，按 `channel_kind` 选择已编译进程序的具体 Adapter，并通过 Platform Config.Secret 注入 Secret；禁止从快照动态加载代码或从 latest 回退。

**ChannelAdapter / Resolver：**

`ChannelResolver.Resolve(ctx, channel_ref, config_version)` 返回绑定该精确版本的 Adapter。Adapter 提供只读 ConfigSnapshot 和 Send。发送请求只包含 delivery_id、稳定 recipient_ref/adapter_target_ref、已渲染内容快照，以及在已验证幂等能力下才允许携带的 idempotency_key。

`RecipientResolver` 根据 Subscription 与 ChannelConfigSnapshot 解析目标，返回 channel_ref、config_version、recipient_ref 和 adapter_target_ref。所有字段均为不透明引用；不得返回邮箱、手机号、URL 或其他明文地址。目标必须与 Channel 的精确配置版本绑定，版本不一致时拒绝发送。实际地址解析只在具体 Adapter 内完成。

**发送结果与重试分类：**

- accepted：渠道明确接受；error_code/retry_after 必须为空。
- rejected：渠道明确拒绝；必须返回稳定 error_code；只有命中该 Channel 的 retryable_error_codes 白名单才是自动重试候选。
- ambiguous：请求可能已到达渠道但结果不确定；必须返回稳定 error_code，进入 unknown/对账，不自动重试。
- not_sent：适配器能够证明请求尚未发出；必须返回稳定 error_code，不自动按 rejected 重试。
- retry_after 只允许出现在 rejected 结果，且必须同时满足 supports_retry_after=supported；时间使用 RFC3339Nano UTC。
- Adapter 返回 Go error 时仍必须给出可验证的 Outcome；无法证明请求未发出时按 ambiguous 处理。不得把网络超时一律映射为 rejected。
- 不把 Provider 原始错误、响应 Body、目标地址或 Secret 写入 error_code、日志或 Attempt 摘要。

**Platform 接入边界：**

具体 Adapter 由组合根注入 Platform `config.Secret` 与固定 DestinationPolicy 的 `httpclient.BoundDoer`；不得自行读取环境变量、创建裸 `http.Client/Transport` 或绕过 DestinationPolicy。Secret 明文只在 Adapter 构造/请求认证的必要路径通过显式 `Value()` 访问，禁止日志和序列化。Platform 没有通用动态 Secret Store，本阶段不新增重复 Secret Repository/Secret Port；SecretRef 到部署 Secret 的映射由服务配置/组合根负责。

`max_payload_bytes` 是渠道最终序列化请求体的硬上限。Domain 对 subject+body 做前置字节数检查；具体 Adapter 必须在序列化后再次检查真实请求体字节数，不能以渲染文本长度代替最终 Payload 大小。

## 10. NTF 任务验收映射

| 任务 | 模块级验收 | L3 集成验收 |
|---|---|---|
| NTF-001 | Event Schema 注册/校验、Inbox 幂等、冲突、Subscription 版本和 recipient_scope | 上游 Outbox -> Notification Inbox 真实事务/故障恢复 |
| NTF-002 | 固定模板版本、变量 allowlist、转义、缺失变量、大小/深度限制 | 真实渠道渲染内容核验 |
| NTF-003 | Channel Port、错误分类、目标解析、Secret Port Fake 测试 | 真实凭据/渠道受控发送；无凭据 skipped/blocked |
| NTF-004 | 有界 Dispatcher、状态机、CAS、租约/fencing、单 Delivery 单活 Worker | 真实 Worker 并发和进程崩溃恢复 |
| NTF-005 | retryable/permanent/ambiguous 分类、退避预算、Retry-After、unknown 不盲重试 | 真实渠道超时/回执丢失/幂等行为验证 |
| NTF-006 | Repository Port、唯一约束、Outbox/Inbox Schema 设计 | MariaDB 事务、唯一冲突、锁竞争、Migration/恢复演练 |

当前 `internal/notification/storage.Migrations()` 已包含 v1～v6 Migration，覆盖 Subscription、DeliveryPlan、Delivery、Attempt、DeliveryStateEvent、Inbox、Template、ChannelConfig、Subscription Revision 与 FanoutWorkItem 持久化结构。`MySQLFanoutWorkItemRepository` 实现 Inbox 同事务写入、数据库时钟 Claim、租约/fencing、CAS 页面推进和 Retry；`MySQLSubscriptionRepository` 实现事务修订号分配与固定快照 keyset 分页；`FanoutDispatcher` 提供单 WorkItem 单页面有界执行。FanoutPlanFactory 的真实业务数据装配、Server 根 Migration 注册与 Worker 生命周期仍由集成层负责。MariaDB 真实 DSN 执行证据缺失时，不得标记 L3 通过。
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
| published_at | RFC3339Nano UTC | 必填 / non-null | 生产方写入 Outbox 的发布时间；重放不变，不参与 payload fingerprint |
| subject_ref | string | 必填 / non-null | 不透明业务主体引用 |
| idempotency_key | string | 必填 / non-null | 发布命令稳定键，重试时保持不变 |
| correlation_id | string | 必填 / non-null | 跨模块关联，不作为去重键 |
| payload_fingerprint | string | 必填 / non-null | 小写 64 位 SHA-256；接收方重新计算校验 |
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

Delivery plan fingerprint 使用 RFC 8785 JSON Canonicalization Scheme + SHA-256，覆盖 source_module/event_id、subscription_id/version、channel_ref/config_version、template_id/version、recipient_ref、payload_fingerprint，以及完整授权策略版本 tuple 数组（policy_type、policy_ref、policy_version，按三元组字典序规范排序）；不得把不同 policy_type/policy_ref 下的相同 policy_version 合并。authorization_refs 仅用于审计，不参与 plan_fingerprint；不覆盖 delivery_id、创建时间、Attempt、lease 和可变状态。相同 Delivery 唯一键且 plan_fingerprint 相同返回原 Delivery；唯一键相同但 fingerprint 不同视为契约冲突，不覆盖旧计划。

### 8.3 Delivery 状态迁移与重试边界（属于 NTF-CTR-002）

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
| NTF-CTR-001 Event Schema Registry | event_type + schema_version 不可变注册键；生产方拥有 payload 语义；未知版本拒绝，不回退旧 Schema | `frozen`（L1）；Schema 时间/跨字段机器约束、Validator/Golden/生产方 Contract Test 属 L2，真实 Outbox/Inbox 属 L3 |
| NTF-CTR-002 Inbox/Publish/Delivery 幂等与状态机 | Inbox=(source_module,event_id)；Publish=(source_module,idempotency_key)；Delivery 唯一键含 source_module/event/subscription/channel config/template/recipient；unknown 先对账，不盲目重发 | `frozen`（L1）；Application Port/持久化实现、CI、MariaDB 并发与 Worker 恢复分别按 L2/L3 验收 |
| NTF-CTR-003 | ChannelConfigSnapshot、ChannelAdapter、ChannelResolver、RecipientResolver 是 Notification 内部 SPI，不构成跨模块 CTR；Delivery retry/unknown 语义归 NTF-CTR-002 | `not_applicable` |



**本文件保留 Notification 领域字段与状态机细节；项目级语义和契约状态以 `doc/contract/PLAT-002-项目级契约冻结登记簿.md` 为准。NTF-CTR-001/002 已达到 L1 frozen，NTF-CTR-003 为 not_applicable。L1 冻结不代表模块 L2 实现符合性、CI 或 L3 真实依赖验收通过。**


## 8.5 Delivery 快照到 ChannelSendRequest 的绑定

渠道发送请求必须由 Notification Application 根据同一份不可变 Delivery 快照构造，不允许由调用方分别拼接 delivery、recipient、template 和 payload 字段。`BuildChannelSendRequest` 执行以下绑定校验：

- Channel 的 `channel_ref/config_version` 必须与 Delivery 完全一致，且该版本已启用。
- Recipient 的 `recipient_ref/channel_ref/config_version` 必须与 Delivery 和 Channel 快照完全一致。
- RenderedContent 的 `template_id/template_version/event_payload_fingerprint` 必须与 Delivery 冻结快照完全一致。
- 只有 Channel 明确声明 `supports_idempotency=supported` 时，才将 Delivery 稳定的 `delivery_idempotency_key` 放入请求；unknown/unsupported 均不发送。
- 最终请求仍须通过 `ChannelSendRequest.Validate` 的能力和 payload 字节数校验。

该构造器是 Worker/Dispatcher 调用 Adapter 前的统一边界，防止合法但属于另一 Delivery 的内容或接收目标被错误发送。


## 8.6 Preflight 决策的状态收敛

Preflight 在 Delivery 已领取为 sending、但尚未创建 Attempt/调用渠道前执行。ready 保持 sending 并继续执行；skip 收敛为 skipped；block 收敛为 blocked。skip/block 必须携带指向本次 Preflight 判定记录的 evidence_ref，状态事件与 Delivery CAS 更新原子写入，并由 Repository 同时释放租约。sending -> blocked 仅用于明确发生在外部渠道调用之前的 Preflight 阻断；不得用于渠道调用后的结果，后者由 DeliveryCompletionService 处理。


## 12. Worker 租约恢复批次调用与诊断约定

Server Worker 是定时/并发调度所有者；Notification 只提供有界的 `LeaseRecoveryService.RecoverExpiredOnce`。Worker 每次调用必须提供有效 UTC 时钟、正数批次上限（最大 1000）和可取消 Context，不得在 Notification 内创建定时器或后台 Goroutine。

批次报告计数必须满足：

```go
Scanned = Recovered + Conflicted + OutcomeUnknown + Failed + Unprocessed
len(OutcomeUnknownDeliveryIDs) = OutcomeUnknown
```

其中 `Scanned` 是扫描器返回的候选数，不代表均已处理；`Unprocessed` 表示 Context 取消后尚未处理的候选。`Conflicted` 是租约/版本已变化或恢复不再适用的并发结果，不应当作持久化故障自动重试。

若 `OutcomeUnknown > 0`，Worker 必须逐个读取 `OutcomeUnknownDeliveryIDs` 对应 Delivery 的一致性快照（Delivery + Attempt + StateEvent），据此判断事务已提交还是未提交。不得解析聚合错误文本、重放整批写入或调用渠道 Send。已提交则接受当前状态；未提交则基于新快照重新评估，不能沿用旧版本/token。

`LeaseRecoveryReport.ToDiagnosticsObservation` 是 Notification 到 Platform Diagnostics 的唯一批次映射入口。Diagnostics 仅输出计数属性，不输出 Delivery 标识或底层错误。状态优先级为普通失败 `failed`，提交不确定 `degraded`，CAS 竞争 `degraded`，未处理候选 `degraded`，无异常 `healthy`。该观察结果不等同于重试授权；Worker 必须通过 `errors.Is(err, ErrPersistenceOutcomeUnknown)` 识别错误链并执行读后对账。


## 8.7 Delivery Unknown 到渠道查询请求的受控绑定

渠道查询使用独立可选的 `ChannelDeliveryLookup` Port，不扩展必需的 `ChannelAdapter` 发送接口。只有 Channel 快照声明 ``supports_delivery_lookup=supported`，且该精确版本 Adapter 实现查询 Port 时，Registry 才允许解析；声明与实现不一致、能力为 unknown/unsupported、版本不存在时均失败关闭。

`BuildChannelDeliveryLookupRequest` 只接受状态为 unknown 的 Delivery、同一 channel_ref/config_version 的 Channel 快照，以及该 Delivery 最后一次已完成且结果为 ambiguous 的 Attempt。请求身份由这三份快照构造，调用方不得自行替换 DeliveryID、AttemptID 或配置版本。只有 `supports_idempotency=supported` 时才携带 Delivery 稳定幂等键；Attempt 已有 Provider Message Ref 时可作为辅助查询键。请求不携带收件人明文地址、模板正文或完整发送请求。

`ChannelRegistry.LookupUnknownDelivery` 是当前查询到对账证据的受控入口：它通过精确版本 Registry 调用 Adapter，校验结果身份与请求一致后，才转换为 `UnknownDeliveryEvidence`。查询超时、认证失败、限流、解析失败等错误原样返回，不能转换为 `not_accepted`；三态结果分别映射为 accepted/not_accepted/indeterminate。Adapter 对 Provider 最终一致性窗口的判断属于具体渠道实现责任；窗口内“暂未查到”必须返回 indeterminate。

该入口只保证模块内部调用边界与身份绑定，不宣称 Go 类型系统能证明 Adapter 查询事实真实或阻止同进程内部代码伪造证据。权威性仍依赖受控 Adapter 实现、正式 Provider 查询语义和逐渠道 Contract Test。查询结果后续仍由既有 `AssessUnknownDelivery`、RetryPolicy 与 Delivery.version CAS 收敛状态；本 Port 不直接修改 Delivery 状态。


## 8.8 Unknown 对账的强制 Application 边界

生产状态收敛统一通过 `UnknownDeliveryReconciliationService.Reconcile` 执行，不允许调用方分别提供 Delivery、Attempt、Channel 快照或手工构造的 `UnknownDeliveryEvidence` 后直接持久化。

该用例按以下顺序执行：

1. 通过 `DeliveryConsistencySnapshotReader` 在同一数据库一致性快照中读取 Delivery、完整 Attempt 历史和 StateEvent 历史。
2. 使用 `AssessDeliveryConsistency` 校验两类历史；结果必须为 `pass`，Delivery 必须仍为 `unknown`。fail、degrade、not_comparable 均停止，不查询渠道。
3. 从 Registry 按 Delivery 固定的 `channel_ref + config_version` 读取不可变 Channel 快照，并执行 Lookup。
4. Registry 查询错误或 Context 取消立即返回，不调用状态持久化。
5. 查询结果包装为 application 包内不可构造的 `verifiedUnknownDeliveryEvidence`，绑定 Delivery version、最后一次 Ambiguous Attempt ID 和完整 Channel 快照；只有包内收敛函数能消费该类型。
6. 使用读取快照中的 Delivery version 执行现有 CAS。查询期间 Delivery 被其他 Worker 更新时，CAS 冲突退出；不得把旧查询证据应用到重新读取的新版本。

`AssessUnknownDelivery` 仍是接收普通 Evidence 的纯规则评估函数，可用于单元测试和规则验证，但不具备持久化能力。原先直接接收调用方 Evidence 的导出状态收敛函数已收紧为包内函数。外部调用方必须使用一致性快照服务。

此边界能阻止模块外部调用方伪造 Evidence 绕过 Registry，并避免把不同时间的 Delivery/Attempt/StateEvent 读取拼成一组输入；它不证明 Provider 自身查询语义真实，也不消除查询后到 CAS 前的竞态，后者由版本 CAS 明确拒绝。


## 13. FanoutWorkItem 与 Subscription 稳定分页契约（NTF-CTR-002 模块内细化）

本节定义 Inbox 接收后的模块内部 Fanout 执行协议。它不新增跨模块公开 API，不改变 NTF-CTR-001/002 已冻结的事件、幂等和 Delivery 状态语义。

### 13.1 责任与处理边界

- 一个成功新建的 Inbox 事件必须且只能在同一数据库事务中创建一个 FanoutWorkItem；Inbox 重放不得新增 WorkItem。
- FanoutWorkItem 只负责将已接收事件展开为不可变 DeliveryPlan，不调用渠道，不创建 Delivery Attempt，不承担 Delivery 发送重试。
- Fanout Dispatcher 每次只处理一个 WorkItem 的一个有界 Subscription 页面；不得在 Notification 内创建后台 Goroutine、Ticker 或无限循环。调度生命周期归 Server Worker。
- 每个候选 Subscription 由注入的 FanoutPlanFactory 评估过滤条件，并解析精确 ChannelConfig/Template 版本、授权策略版本及不透明 RecipientRef；不匹配时明确返回 skipped decision，不生成 DeliveryPlan。
- 匹配时必须通过现有 DeliveryPlanWriter.CreateOrGet 持久化计划。不得绕过 Delivery 唯一 tuple、JCS Fingerprint 或冲突校验。
- Factory/Repository 错误不得推进页面游标。计划已写入但游标未推进时允许重放，由 DeliveryPlan 唯一键与 Fingerprint 保证幂等。
- Fanout 完成仅表示该事件的订阅展开已完成，不代表任何 Delivery 已发送或用户已收到通知。

### 13.2 Subscription 修订序号与快照边界

仅用 subscription_id/version 游标或 MAX(auto_increment) 不能建立可靠快照：订阅变更事务可能先分配序号、后提交，导致分页过程中出现迟到记录。

为建立跨事务可恢复的快照，Notification 使用单行 notification_subscription_revision 作为事务内修订序号分配器：

- 每次 Subscription Create/AppendVersion 在同一事务内锁定并递增 singleton 修订计数，再将新值写入该 Subscription 版本的 revision_id。
- 修订计数更新与版本行插入必须同事务提交；事务回滚不得消耗已提交修订号。
- 并发订阅写入由该单行锁串行化。Fanout 首次 Claim 时读取已提交的 current_revision，并将其写入 WorkItem 的 snapshot_revision；后续租约恢复和重试不得重新取快照。
- revision_id 是 Notification Storage 内部字段，来源为上述数据库分配器；它不暴露给业务 API，也不替代 Subscription 的业务 version。
- 既有 Subscription 版本在迁移 v6 中统一以 revision_id=0 作为历史基线；后续新增版本从 1 开始。查询以 (revision_id, version) 作为同一 Subscription 的全序。
- 页面查询必须先选取 revision_id <= snapshot_revision 范围内每个 Subscription 的最新版本，再判断该版本的 event_type 与 status=active。不得先筛 active 再选最新版本，否则已暂停/禁用或已改 event_type 的旧版本会被错误选中。
- 分页使用 subscription_id > cursor_subscription_id 的 keyset 游标，按二进制稳定顺序升序；不使用 OFFSET。每页 limit 由 Worker 配置，范围 1～200。
- 快照一经创建即固定；快照之后新建或追加的 Subscription 版本不进入该 WorkItem。新的业务事件会使用新的快照。

### 13.3 FanoutWorkItem 字段与来源

| 字段 | 类型 | 来源与语义 | 约束 |
|---|---|---|---|
| work_item_id | string | SHA-256(source_module + NUL + event_id) 的稳定编码，作为内部工作项身份 | 必填、不可变 |
| source_module / event_id | string | 对应已提交 Inbox 的双唯一身份 | 必填；唯一键 (source_module,event_id) |
| event_type / payload_fingerprint | string | 从 Inbox 不可变事件复制，供路由和一致性校验 | 必填；不得由 Worker 改写 |
| snapshot_revision | uint64 nullable | 首次 Claim 时读取的订阅修订高水位；未 Claim 前为空 | 首次设置后不可变 |
| cursor_subscription_id | string | 最后一个已完整处理的 Subscription ID；空串表示页面尚未开始 | 每次页面成功后单调前进 |
| status | enum | pending、processing、retry_wait、completed、failed | 仅允许本节定义的迁移 |
| attempt_count | uint32 | 成功取得 Fanout 租约的次数，包括过期租约重新领取 | Claim 成功时递增 |
| lease_worker_id / lease_expires_at | string / UTC timestamp | 当前 Worker 租约所有者及数据库时钟截止时间 | 仅 processing 有值 |
| fencing_token | uint64 | 每次成功 Claim 单调递增，拒绝过期 Worker 写入 | 大于 0 后只增不减 |
| next_attempt_at | UTC timestamp nullable | retry_wait 的最早再次领取时间 | 其他状态为空 |
| last_error_code | string nullable | 最后一次失败的稳定内部错误分类，不存原始错误文本 | 安全码；不含用户内容/Secret |
| created_at / updated_at | UTC timestamp | 数据库持久化时间 | DATETIME(6) 精度 |

### 13.4 状态迁移、租约与 fencing

| 当前状态 | 触发条件 | 下一状态 | 原子要求 |
|---|---|---|---|
| pending | Worker 成功 Claim | processing | attempt_count、fencing_token 递增；设置 worker/lease；首次 Claim 固定 snapshot_revision |
| retry_wait | next_attempt_at 到期且 Claim 成功 | processing | 同上 |
| processing | 当前租约过期，其他 Worker Claim 成功 | processing | fencing_token 递增，替换 worker/lease；保留 cursor/snapshot |
| processing | 页面完成且还有后续页 | pending | CAS 校验 worker/token/有效租约；推进 cursor 并释放租约 |
| processing | 页面完成且无后续页 | completed | 同上；清空租约和 next_attempt_at |
| processing | 可重试错误且未达最大 Claim 次数 | retry_wait | 不推进 cursor；记录稳定错误码与 next_attempt_at；释放租约 |
| processing | 达到最大 Claim 次数或不可恢复错误 | failed | 不推进未完成页面；记录稳定错误码并释放租约 |

- Claim/Advance/Retry/Fail 必须以 work_item_id + status + lease_worker_id + fencing_token 条件更新，并由 MariaDB UTC_TIMESTAMP(6) 判断租约有效性；不能信任 Worker 本地时间证明租约仍有效。
- Claim 使用事务行锁选择最早到期项，单次最多返回一个 WorkItem；不依赖 Redis、旧 Platform Task Queue 或 SKIP LOCKED 版本特性。
- Worker ID 必须非空且长度不超过 128 字节；租约时长必须由 Server 显式配置为正数。
- attempt_count 达到配置的 max_attempts 后不得继续自动 Claim；错误恢复策略必须显式提供 max_attempts、base_delay、max_delay，不设隐式全局默认值。
- Retry 延迟采用有上限的指数退避；next_attempt_at 由 Application Clock 计算并规范为 UTC 微秒，Repository 仍使用数据库时钟校验当前租约。
- ErrPersistenceOutcomeUnknown 时，Worker 不得依据返回对象推断 Claim/Checkpoint/Retry/DeliveryPlan 写入已提交；Dispatcher 不得继续用当前 Claim 执行 Retry/Fail/Advance，必须丢弃本次 claim 并等待租约到期后重新领取和读取持久状态。DeliveryPlan 是否已提交由唯一 tuple + fingerprint 在重放时对账。
- failed 是需要人工/显式运维命令处理的终态；不得在普通 Worker 循环中自动重置为 pending。

### 13.5 页面处理与幂等提交

1. Claim 返回 WorkItem、同一 Inbox 的完整不可变 EventEnvelope 和租约。
2. 按固定 snapshot_revision 读取一页活跃 Subscription，最多请求 page_size+1 条以判断是否还有下一页。
3. 对页面内每个 Subscription 调用 FanoutPlanFactory.Build。明确不匹配的项视为已处理，不创建 DeliveryPlan；匹配项必须生成通过 Validate/VerifyFingerprint 的完整计划。
4. 逐项调用现有 DeliveryPlanWriter.CreateOrGet。任一错误发生时不推进 cursor；已成功写入的计划允许在下次处理时幂等重放。
5. 页面全部处理成功后，使用当前 Worker/FencingToken 执行一次 AdvanceWithLease：cursor 前进到本页最后一个 Subscription ID；若无更多页则转 completed，否则回到 pending。
6. 页面为空时直接转 completed。快照中不存在 active Subscription 是正常完成，不是失败或降级。

FanoutPlanFactory 是模块内部应用端口，不是新增对外 Facade。它的唯一调用场景是 Dispatcher 将一个已校验 Inbox Event 与一个固定版本 Subscription 转为 DeliveryPlan decision。Dispatcher 必须校验 Subscription 为 active 且 event_type 与 Event 一致，并校验 DeliveryPlan 的 source_module/event_id/payload_fingerprint 与 Event 一致、subscription_id/version 与当前 Subscription 一致。Factory 实现不得调用渠道 Send；真实 Recipient/Channel/Template/Authorization 数据缺失时必须返回明确错误，不得制造默认接收目标或伪造版本。

### 13.6 持久化与验收边界

- Migration v6 新增订阅修订分配器、Subscription revision_id、FanoutWorkItem 表及 due/lease 索引；已发布 v1～v5 不得修改。
- Inbox 与 FanoutWorkItem 插入必须复用同一个 *sql.Tx；Inbox 唯一冲突时不得再插入 FanoutWorkItem。
- FanoutWorkItem 的完整 EventEnvelope 从 Inbox 读取；不得复制 Payload 到工作项表，避免双份事实和数据不一致。
- DeliveryPlan 仍由现有 Repository 持久化；Fanout cursor 仅在本页所有匹配计划成功 CreateOrGet 后推进。
- 单元测试必须覆盖状态迁移、游标固定、租约过期重领、旧 fencing token 拒绝、重复页面幂等、订阅快照并发更新、无订阅完成和失败不推进游标。
- MariaDB L3 必须验证 Inbox/Fanout 同事务回滚、双唯一重放不重复工作项、并发 Claim 单赢家、快照修订序号提交顺序、分页无漏项、过期租约 fencing 与进程重启恢复。
- 没有真实 NOTIFICATION_MARIADB_TEST_DSN 执行结果时，L3 状态为 not_comparable，不能标记通过。
