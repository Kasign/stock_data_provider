# Notification 契约 Fixture 与状态迁移验收清单

- 归属：NTF-001/004/005/006
- 对应契约：NTF-CTR-001～003（历史别名 Platform CONTRACT-009/010）；状态以项目级契约目录为准
- 状态：4 份 Draft 2020-12 Schema 与 6 个代表性 Golden 已提交；NTF-CTR-001～003 仍为 pending_evidence，自动化 Validator/Golden/Contract Test 未完成；不要求个人 Owner 签字
- 日期：2026-11-06
- 说明：以下均为合成数据与测试场景，不代表已有实现或真实渠道能力。

## 1. Fixture 原则

1. 每个场景断言输入、持久化结果、状态迁移、幂等结果、Attempt 记录和安全日志。
2. 领域单测使用内存/Fake Repository 与 Fake Channel；不得因真实凭据缺失跳过模块级测试。
3. 真实渠道、MariaDB、Worker、根 Outbox/Migration 验收单独记录为 L3。
4. 所有时间由可控 Clock 注入；重试测试不得依赖真实 sleep。
5. 测试必须断言外部调用次数，不能只断言最终 Delivery.status。
6. 日志断言 Secret、完整目标地址、模板敏感变量和 payload 正文均未泄露。

## 2. Event JSON Golden

首批事件映射与触发语义见 [INT-CTR-001 首批业务事件映射决议](05-首批业务事件映射决议.md)。机器可读 Schema 位于 `schema/`，包括 Envelope 和三类 Payload：

- `notification-event-envelope-1.0.0.schema.json`
- `researchflow-run-terminal-1.0.0.schema.json`
- `quant-risk-decision-changed-1.0.0.schema.json`
- `research-event-lifecycle-changed-1.0.0.schema.json`

已提交的代表性 Golden 位于 `golden/`：

- `researchflow-run-terminal-succeeded.golden.json`、`researchflow-run-terminal-cancelled.golden.json`
- `quant-risk-decision-first-evaluation.golden.json`、`quant-risk-decision-reject.golden.json`
- `research-event-lifecycle-created.golden.json`、`research-event-lifecycle-invalidated.golden.json`

仍需自动化覆盖：相同事件重放、相同 event_id 不同 fingerprint、同 source_module 幂等键冲突、未知 Schema 版本、非法 Payload、Registry digest 不可变、未知 source_module、超大小拒绝，以及 Outbox 提交/回滚边界。

## 3. Subscription 与 recipient Fixture

| Fixture | 场景 | 预期 |
|---|---|---|
| subscription_active_match | active 且事件类型匹配 | 创建投递计划 |
| subscription_paused | paused | 不创建新 Delivery |
| subscription_disabled | disabled | 不创建新 Delivery |
| subscription_filter_mismatch | 过滤条件不匹配 | 不创建 Delivery，记录有界统计 |
| subscription_version_changed | fan-out 前后配置更新 | Delivery 绑定确定版本，不混用字段 |
| subscription_revoked_before_send | Delivery queued 后退订 | 发送前复核并转 skipped |
| subscription_owner_changed | recipient_ref 变更 | 版本递增，旧计划不被原地改写 |
| recipient_scope_unknown | 未注册 scope | Publish/配置校验拒绝 |
| recipient_from_model_output | payload 含模型生成地址 | 不解析为真实目标 |
| channel_config_version_changed | 渠道配置轮换 | 已创建 Delivery 保留原配置版本引用；Secret 当前授权仍需复核 |

## 4. Delivery 状态机 Fixture

| Fixture | 状态路径/触发 | 关键断言 |
|---|---|---|
| delivery_success | queued→sending→sent | 1 个 Attempt；provider accepted；sent 不等于 read |
| delivery_retryable_rejection | queued→sending→retry_wait→sending→sent | 每次真实调用 1 个 Attempt；next_attempt_at 有界 |
| delivery_permanent_rejection | queued→sending→failed | 不再自动重试 |
| delivery_retry_budget_exhausted | 多次 retryable 后 failed | 实际调用次数不超过 max_attempts |
| delivery_missing_secret | queued→blocked | 0 个渠道 Attempt；不伪装 skipped |
| delivery_unsubscribed | queued→skipped | 0 个渠道 Attempt；reason_code 明确 |
| delivery_filter_mismatch | fan-out 过滤不匹配 | 不创建 Delivery |
| delivery_ambiguous_timeout | 请求可能已到渠道后超时 | sending→unknown；不自动重发 |
| delivery_definitely_not_sent | 发送前 DNS/配置校验失败 | blocked 或 retry_wait，取决于错误性质；0 个外部调用 |
| delivery_unknown_reconcile_accepted | 对账证据确认渠道接受 | unknown→sent；记录 evidence_ref |
| delivery_unknown_reconcile_absent | 渠道查询证明不存在 | unknown→retry_wait/failed，按预算处理 |
| delivery_unknown_no_reconcile | 渠道不支持查询/幂等 | 保持 unknown，不能盲目重发 |
| delivery_manual_retry | failed 且授权通过 | 命令幂等；新增 Attempt，不覆盖旧 Attempt |
| delivery_manual_retry_unknown | unknown 请求人工 RetryDelivery | 拒绝，必须先对账 |
| delivery_retry_sent | sent 请求 RetryDelivery | 拒绝 |
| delivery_invalid_transition | 非法状态迁移 | CAS/领域错误；原状态不变 |

## 5. 幂等与并发 Fixture

- `fanout_duplicate_concurrent.test`：相同 Event 并发 fan-out，唯一约束只生成一份 DeliveryPlan。
- `delivery_two_workers_one_lease.test`：两个 Worker 竞争同一 Delivery，只允许一个 fencing token 有效。
- `delivery_stale_worker_completion.test`：租约过期后旧 Worker 回写，必须拒绝。
- `delivery_lease_expired_before_dispatch.test`：能证明请求未发出，允许安全回队。
- `delivery_lease_expired_after_dispatch.test`：可能已发出，转 unknown。
- `delivery_plan_fingerprint_stable.test`：相同事件和订阅版本生成相同 plan fingerprint。
- `delivery_plan_config_version_change.test`：配置版本变化产生新计划身份，不修改历史 Delivery。
- `retry_command_duplicate.test`：相同 RetryDelivery request_id 重放只执行一次命令。
- `retry_command_different_payload_same_key.test`：相同幂等键不同命令参数返回冲突。

## 6. Outbox / Inbox 与进程恢复 Fixture

- `outbox_business_commit_atomic.test`：业务提交与 Outbox 写入同事务；任一失败均不能留下半提交。
- `outbox_dispatch_crash_after_publish.test`：发送到 Inbox 后 Outbox 未确认，重复投递由 Inbox 幂等吸收。
- `inbox_crash_after_accept.test`：事件持久化后进程崩溃，恢复后 fan-out 工作可继续且不重复。
- `dispatcher_restart_queued.test`：重启后 queued/retry_wait 可被重新调度。
- `dispatcher_restart_sending_uncertain.test`：sending 租约过期且请求结果不明，转 unknown 而非直接 queued。
- `migration_unique_constraint.test`：真实 DB 验证唯一键、事务隔离和冲突处理，属于 L3。

## 7. Channel Adapter Fixture

Fake Adapter 至少覆盖：
- accepted + provider_message_ref；
- 明确 retryable rejection + Retry-After；
- 永久拒绝（无效目标/权限）；
- 超时且请求未发出；
- 超时且请求可能已被接收；
- 支持稳定 provider idempotency key；
- 不支持 idempotency key；
- 返回畸形响应/超大响应；
- Secret 不存在、撤销或轮换。

Fake Adapter 只能验证分类和状态机，不证明真实渠道行为。每个真实 Channel 的支持能力需要独立证据记录。

## 8. 统一断言矩阵

每个 Fixture 至少断言：
1. Inbox/Event/Delivery 是否创建及唯一键结果。
2. Event/Subscription/Channel/Template 版本引用和 fingerprint。
3. Delivery 状态迁移、reason_code、状态版本。
4. Attempt 数量、attempt_no、outcome 和不可变历史。
5. 渠道实际调用次数、幂等键是否稳定。
6. unknown 是否被禁止盲目重试。
7. CAS/fencing 是否拒绝并发和迟到写入。
8. 退订、权限撤销和 Secret 失效是否在发送前生效。
9. 上游事务是否不受外部发送失败回滚影响。
10. Secret、目标和 payload 正文是否从日志/错误摘要中脱敏。

## 9. 尚待证据验收项

1. recipient_scope 的允许值、解析路径和未授权拒绝行为需由 Schema/Contract Test 固化。
2. Event Inbox 与 PublishEvent 双唯一键、相同指纹重放和不同指纹冲突需完成并发测试。
3. DeliveryPlan 在 fan-out 时冻结 Subscription/Channel/Template 版本的行为需有回归测试。
4. unknown 的自动对账能力和人工处理 SLA 按真实渠道逐项登记；不支持对账时不得自动重发。
5. 渠道幂等键支持情况及保留期限按 Channel Capability 提供证据；未验证不得标记 supported。
6. Schema 兼容级别和版本命名规则需补兼容矩阵与 Registry 不可变性测试。
7. Delivery deadline、max_attempts、max_elapsed 必须显式配置并测试边界，不依赖隐式默认值。

**本清单是模块级测试设计基线；未生成真实测试代码，未连接真实渠道或数据库，不能据此宣称 NTF-001~006 已通过。**
