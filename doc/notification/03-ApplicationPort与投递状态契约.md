# Notification Application Port 与投递状态契约

- 归属：Notification 模块，NTF-001/004/005/006
- 对应治理决议：CONTRACT-009、CONTRACT-010
- 状态：规划侧 Owner Review 建议稿，未冻结
- 日期：2026-11-06
- 代码边界：`internal/notification/**`；根 DI、HTTP、Worker、Migration 注册由 Integration Owner 负责

## 1. 目标与非目标

Notification 接收版本化业务事件，按已授权 Subscription 生成投递计划，使用固定版本的 Template/Channel 执行投递，保存可审计的 Delivery 与 Attempt 历史，并对可确认的临时失败实施有界重试。

Notification 不计算告警条件、不决定研究结论、不执行模型输出中的接收人指令，也不承诺外部渠道 exactly-once。外部发送是不可回滚副作用；本地数据库事务只能保证本地事件/投递状态一致。

## 2. 核心对象与版本边界

### 2.1 NotificationEvent

必需字段：
- `event_id`：事件所有者生成的稳定、不透明 ID；同一逻辑事件重放时不得变化。
- `event_type`、`schema_version`：注册表中的精确键。
- `source_module`：受控枚举 quant/research/researchflow。
- `occurred_at`：业务事件发生时间，不作为投递时间。
- `subject_ref`：业务主体不透明引用。
- `idempotency_key`：调用方对本次发布意图提供的稳定键；重试 PublishEvent 时不得变化。
- `payload`：必须通过该 event_type + schema_version 对应的 Schema 校验。
- `correlation_id`：跨模块关联 ID。

接收规则建议：
1. 对 payload 做规范化后计算 fingerprint，覆盖 event_type、schema_version、subject_ref、occurred_at 和 payload；不把接收时间或传输 Header 纳入指纹。
2. Inbox 唯一约束为 `(source_module, event_id)`；发布幂等唯一约束为 `(source_module, idempotency_key)`。
3. 相同 source_module/event_id 且 fingerprint 相同：返回原接收结果，不重复 fan-out。
4. event_id 或 idempotency_key 已存在但 fingerprint/事件身份不一致：返回 idempotency_conflict，不覆盖旧事件。
5. 首次接收时持久化事件与 fan-out 工作项；如果需要跨模块本地事务，由调用方 Outbox 与 Notification Inbox 形成可靠交付，不依赖跨领域 Repository 事务。

### 2.2 Subscription

建议字段：
- subscription_id、owner_ref、version；
- status：active/paused/disabled；
- event_type allowlist 与受限过滤条件；
- recipient_scope、recipient_ref 来源；
- channel_ref、template_ref；
- created_at、updated_at、effective_at。

Subscription 更新采用版本递增和不可变版本快照。任何影响匹配、目标、渠道或模板的更新都必须递增 version。用户退订/禁用是当前授权状态，发送前必须重新检查；历史 Delivery 保留原版本，不被重写。

### 2.3 DeliveryPlan 与 DeliveryRecord

每个 Delivery 在入队时固定本次投递计划快照：
- event_id；
- subscription_id + subscription_version；
- channel_ref + channel_config_version；
- template_id + template_version；
- recipient_ref（由已授权 Subscription/Channel 解析得到）；
- payload_fingerprint、plan_fingerprint；
- delivery_id、delivery_idempotency_key。

建议唯一键：
`(event_id, subscription_id, subscription_version, channel_ref, channel_config_version, recipient_ref)`。

Template 版本必须由 Subscription 版本快照固定；如果实现允许同一 Subscription 版本切换 Template，则必须把 template_id/template_version 加入唯一键，并禁止原版本原地修改。Channel 配置同理，版本化 Secret 引用，不在 Delivery 中复制 Secret 明文。

默认不对历史事件做新订阅回补。若未来提供 backfill，必须使用独立显式命令、权限和幂等范围，不得由普通 PublishEvent 重放隐式触发。

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
- sending -> sent / retry_wait / failed / skipped / unknown
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
- 永久错误不重试；Retry-After 优先于本地退避上限，但不得越过 Delivery deadline。
- 使用 capped exponential backoff + jitter；策略固定 max_attempts、max_elapsed、per_attempt_timeout 和总 deadline。
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

CONTRACT-010 建议：
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

## 11. Owner Review 待确认

| 决议 | 建议 | Owner |
|---|---|---|
| CONTRACT-009-A Event Inbox 幂等键 | source_module + event_id；同一 key 不同 fingerprint 冲突 | Notification |
| CONTRACT-009-B PublishEvent 幂等键 | source_module + idempotency_key；稳定重放返回原结果 | Notification |
| CONTRACT-009-C Delivery 唯一键 | event + subscription/version + channel/config version + recipient_ref | Notification |
| CONTRACT-009-D 状态集合 | 增加 retry_wait、blocked、unknown；unknown 先对账再重试 | Notification + Platform |
| CONTRACT-009-E 交付语义 | 明确不承诺外部 exactly-once；按渠道能力使用幂等键/对账 | Notification |
| CONTRACT-010-A Schema 注册 | event_type + schema_version 不可变注册表 | Notification + 各事件 Owner |
| CONTRACT-010-B 兼容策略 | 逐事件类型声明兼容级别，禁止隐式旧版本回退 | Notification + 各事件 Owner |
| CONTRACT-010-C 模板绑定 | Template 固定事件 Schema 和版本，变量 allowlist | Notification |

**本文件是设计建议和 Review 基线，不代表 Owner 已批准、Schema 已冻结或任何实现/测试已通过。**
