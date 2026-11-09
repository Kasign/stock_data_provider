# Notification 契约 Fixture 与状态迁移验收清单

- 归属：NTF-001/002/003/004/005/006
- 对应契约：NTF-CTR-001～003（历史别名 Platform CONTRACT-009/010）；状态以项目级契约目录为准
- 状态：4 份 Draft 2020-12 Schema 与 6 个代表性 Golden 已校准；Go Envelope/Payload Validator、Golden、Inbox/Subscription/DeliveryPlan Fake 测试、Renderer/Channel Registry 测试已建立；NTF-004 新增 Attempt Repository 租约测试、过期租约恢复评估、Retry-After 持久化、RetryPolicy、ApplyRetryDecision 和 DeliveryPreflight 测试；NTF-005 新增 Channel retryable 白名单归一化、完整 Attempt 历史安全校验、Full Jitter/Retry-After/deadline 与状态事件/租约释放测试。当前 HEAD c8c250b 的 CI 尚未完成；最近根模块 Test job 仍被 Research context/validate.go 非法反斜杠和 evidence/authorization.go 非法 NUL 阻断，Notification tests 未执行；日志未再列出 Notification gofmt 差异。完整 CI Run 42201893234 是历史通过基线，不代表本轮新增测试通过。NTF-CTR-001～003 仍为 pending_evidence；不要求个人 Owner 签字
- 日期：2026-11-09
- NTF-004 第一批增量：已新增 DeliveryRecord/DeliveryStateEvent 领域实现与纯迁移测试；覆盖 queued→sending、sending→skipped 证明要求、unknown 对账证据、retry_wait 截止时间、failed 人工重试审计和 UTC/nullable 字段一致性。
- NTF-004 第二批增量：新增 DeliveryRepository Port 与内存 Fake；覆盖 32 Worker 并发领取仅一个赢家、sending 事件、租约/版本/fencing CAS、retry_wait 到期领取、旧 Worker 迟到写入拒绝和失败原子性。第三批新增 Attempt Repository Port/Fake，覆盖 Start/Complete 租约约束、Repository 权威时钟、单 claim 单 Attempt 和失败原子性。第四批新增只读过期租约恢复评估与 Retry-After 持久化；第五批新增 DeliveryPreflight 发送前复核。新增测试仍未取得 CI 通过证据。
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

资源上限需覆盖 Envelope/Payload 字节数恰好达到上限、超过 1 字节、JSON 容器深度达到/超过上限；严格解析需覆盖嵌套重复键、非法 UTF-8、孤立 surrogate、合法 surrogate pair。

仍需自动化覆盖：相同事件重放、相同 event_id 不同 fingerprint、同 source_module 幂等键冲突、未知 Schema 版本、非法 Payload、重复 JSON key、非法 Unicode/非 IEEE-754 数字、指纹不匹配、Registry digest 不可变、未知 source_module、超大小拒绝，以及 Outbox 提交/回滚边界。

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

NTF-004 Delivery Repository Fake 还覆盖：多 Worker 原子领取、单活租约、过期租约和错误 fencing token 拒绝、版本 CAS 冲突、状态事件与快照同步追加、retry_wait 到期调度和旧 Worker 迟到回写拒绝。该 Fake 不证明真实数据库锁或跨进程崩溃恢复。

NTF-004 Delivery 领域迁移测试已覆盖：状态版本递增且原对象不变、sending 未证明未发送不得转 skipped、sent 终态不可回退、unknown 无 evidence_ref 不得收敛、retry_wait 时间必须早于 deadline、failed 人工重试必须携带授权结果和审计上下文、非 UTC 时间与不匹配的 next_attempt_at 必须拒绝。

- `subscription_append_version_concurrent.test`：多个并发更新竞争同一 expected_version，只允许一个版本追加成功，其他返回 version conflict。
- `subscription_append_only.test`：更新生成新版本，旧版本保持不变；读取副本的修改不得反向污染 Repository。
- `repository_failure_no_partial_write.test`：Inbox、Subscription、DeliveryPlan Fake 在持久化前注入失败，不得留下部分记录；再次执行后可成功写入。
- `fanout_duplicate_concurrent.test`：相同 Event 并发 fan-out，唯一约束只生成一份 DeliveryPlan。
- `delivery_two_workers_one_lease.test`：两个 Worker 竞争同一 Delivery，只允许一个 fencing token 有效。
- `delivery_stale_worker_completion.test`：租约过期后旧 Worker 回写，必须拒绝。
- `delivery_lease_expired_before_dispatch.test`：能证明请求未发出，允许安全回队。
- `delivery_lease_expired_after_dispatch.test`：可能已发出，转 unknown。
- `delivery_plan_fingerprint_stable.test`：相同事件和订阅版本生成相同 plan fingerprint。
- `delivery_plan_policy_tuple_fingerprint.test`：policy_type、policy_ref、policy_version 任一变化都改变 plan_fingerprint；tuple 顺序变化不改变 fingerprint。
- `delivery_plan_authorization_ref_audit_only.test`：AuthorizationRef 仅作审计引用，变化不得改变 plan_fingerprint；不完整/重复策略 tuple 必须拒绝。
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
- ChannelConfigSnapshot 能力状态必须显式为 unknown/supported/unsupported，max_payload_bytes 必须为正数；重复或非法 retryable_error_code 拒绝；
- ChannelResolver 只能解析精确 channel_ref + config_version，不得回退 latest；相同版本重复注册拒绝；
- RecipientTarget 只允许 opaque ref，且必须绑定 channel_ref/config_version；URL/地址形态拒绝；
- supports_idempotency 未验证/不支持时禁止发送 idempotency_key；支持时必须发送稳定 delivery_idempotency_key；
- supports_retry_after 未验证时拒绝采信 Retry-After；Retry-After 仅适用于明确 rejected；
- accepted + provider_message_ref；
- 明确 retryable rejection + Retry-After；
- 永久拒绝（无效目标/权限）；
- 超时且请求未发出；
- 超时且请求可能已被接收；
- 支持稳定 provider idempotency key；
- 不支持 idempotency key；
- 返回畸形响应/超大响应；
- Secret 不存在、撤销或轮换；仅在具体 Adapter 内显式读取 Platform config.Secret，日志/序列化不得泄露明文；
- 最终序列化请求体超过 max_payload_bytes 时，在发起网络请求前拒绝。

Fake Adapter 只能验证分类和状态机，不证明真实渠道行为。RecipientTarget 的领域测试只证明不透明引用与版本绑定，不证明真实身份目录授权。每个真实 Channel 的支持能力需要独立证据记录。

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
3. DeliveryPlan 在 fan-out 时冻结 Subscription/Channel/Template 版本的行为需有回归测试；Application 写入前须分别执行 Validate() 和 VerifyFingerprint()，任一步失败不得调用 Repository。
4. unknown 的自动对账能力和人工处理 SLA 按真实渠道逐项登记；不支持对账时不得自动重发。
5. 渠道幂等键支持情况及保留期限按 Channel Capability 提供证据；未验证不得标记 supported。
6. Schema 兼容级别和版本命名规则需补兼容矩阵与 Registry 不可变性测试。
7. Delivery deadline、max_attempts、max_elapsed 必须显式配置并测试边界，不依赖隐式默认值。

**本清单是模块级测试设计基线；根模块 gofmt/go test/race/vet 已通过，Inbox/Subscription/DeliveryPlan 当前仍使用 Fake Repository；未连接 Notification 真实 MariaDB、Worker 或渠道，不能据此宣称 NTF-001 完整通过或 NTF-001~006 已通过。**

## 10. INT-CTR-001 / NTF-CTR-001～003 Contract Test 执行矩阵

以下测试应由模块 CI 以 Fake Repository、Fake Clock、Fake Channel 执行；数据库唯一约束、事务隔离和进程恢复属于后续 L3。每项必须断言持久化副作用和外部调用次数。

| Test ID | 场景 | 必须断言 |
|---|---|---|
| INT001-SCHEMA-01 | 四份 Schema + 六个 Golden | Draft 2020-12 校验通过；date-time format assertion 开启；Payload 按 event_type + schema_version 精确选择；published_at/payload_fingerprint 必填；接收方重算指纹一致 |
| INT001-SCHEMA-02 | 未知 event_type、未知 schema_version、未知 source_module | PublishEvent 明确拒绝；不回退旧 Schema；不写 Inbox/Delivery |
| INT001-SCHEMA-03 | 缺字段、未知字段、错误 enum、错误时间、超大 Payload | 明确校验失败；无字段静默补全；无持久化副作用 |
| INT001-ID-01 | 相同 event_id、相同 fingerprint 重放 | 返回原接收结果；只 fan-out 一次 |
| INT001-ID-02 | 相同 event_id、不同 fingerprint | 返回稳定 idempotency_conflict；不覆盖原事件、不 fan-out |
| INT001-ID-03 | 相同 source_module + idempotency_key、相同 fingerprint | 返回原 PublishResult；不创建第二个事件 |
| INT001-ID-04 | 相同 source_module + idempotency_key、不同 fingerprint | 返回 idempotency_conflict；不通过更换 event_id 绕过 |
| INT001-ID-05 | 不同 source_module 使用相同 idempotency_key | 按不同生产方作用域分别处理；Inbox 唯一键不得跨 source_module 误冲突 |
| INT001-FP-01 | 变更 event_type/schema_version/source_module/subject_ref/occurred_at/payload 任一字段 | payload fingerprint 必须变化 |
| INT001-FP-02 | 仅变更 event_id 或 idempotency_key | payload fingerprint 不变化；对应唯一键仍按各自规则处理 |
| INT001-OUTBOX-01 | 领域状态与 Outbox 同事务提交 | 两者同时提交；Outbox 事件字段与领域事实一致 |
| INT001-OUTBOX-02 | 领域状态写入成功但 Outbox 写入失败 | 整个本地事务回滚；不允许出现已提交事实但缺 Outbox |
| INT001-OUTBOX-03 | Outbox 已发送，确认前进程崩溃 | 重启后可重复发送；Inbox 幂等吸收重复事件 |
| INT001-RUN-01 | Run queued/running/cancelling | 不发布 terminal 事件 |
| INT001-RUN-02 | Run 终态 succeeded/partial/failed/cancelled 经 CAS 固化 | 每个 Run 只发布一次；occurred_at=finished_at；重试复用原 envelope |
| INT001-QUANT-01 | RiskDecision 首次形成 | previous_status=null；发布一条事件 |
| INT001-QUANT-02 | status 变化或硬阻断 reason code 集合变化 | 发布一条新事件；reason codes 去重并字典序排序 |
| INT001-QUANT-03 | 仅 warning/info 变化，或硬阻断原因顺序变化但集合相同 | 不发布新事件 |
| INT001-QUANT-04 | 一个 RiskDecision 涉及多个主体 | 按主体拆分；每条事件只有一个 subject_ref；各自 idempotency_key 稳定且不同 |
| INT001-RESEARCH-01 | ResearchEvent 首次创建或生命周期状态迁移 | event_version 单调递增；previous/current 正确；occurred_at=changed_at |
| INT001-RESEARCH-02 | Evidence 追加、来源补录、描述修订但状态不变 | 不发布 lifecycle_changed |
| INT001-RESEARCH-03 | invalidated/closed 后再次更正或迁移 | 追加新版本事件；不修改或删除历史事件 |
| NTF002-SUB-01 | Subscription active/paused/disabled/filter mismatch | 仅 active 且匹配时生成 Delivery；其他情况无外部调用 |
| NTF002-PLAN-01 | Subscription/Channel/Template 版本在 fan-out 前后变更 | Delivery 固定使用单一版本快照；历史计划不原地改写 |
| NTF002-PLAN-02 | 同一 Event 并发 fan-out | 唯一约束/幂等只生成一份 DeliveryPlan |
| NTF002-CAS-01 | 两个 Worker 同时领取同一 Delivery | 仅一个 lease/fencing token 有效；旧 Worker 回写被拒绝 |
| NTF002-CAS-02 | 旧 Worker lease 过期后提交成功结果 | CAS 拒绝迟到状态覆盖；保留审计记录 |
| NTF002-ATT-01 | 有效租约开始真实渠道调用 | Attempt ID 唯一、attempt_no 连续递增；attempt_count 与追加历史原子更新 |
| NTF002-ATT-02 | 旧版本、旧 fencing token 或其他 Worker 尝试开始 Attempt | 拒绝追加，不修改 attempt_count 和 Attempt 历史 |
| NTF002-ATT-03 | 当前有效租约完成 Attempt | 仅允许运行中记录完成一次；完成后不可覆盖 |
| NTF002-ATT-04 | Attempt 完成时租约已过期 | 拒绝旧 Worker 迟到完成，交由恢复流程判定 unknown/安全重排 |
| NTF002-ATT-05 | Attempt 追加/完成持久化失败 | 不产生部分 Attempt 或计数投影更新 |
| NTF002-ATT-06 | 同一 fencing token 重复启动第二个 Attempt（前一条运行中或已完成） | 拒绝第二次启动；重试必须经过状态迁移并重新领取新 token |
| NTF003-RETRY-01 | 明确 retryable rejection + 有效 Retry-After | next_attempt_at 取本地退避与 Retry-After 的较晚值；不超过总 deadline |
| NTF003-RETRY-02 | permanent rejection / retry budget exhausted | 结束为 failed；不超过 max_attempts |
| NTF003-RETRY-03 | timeout-after-write / 请求可能已到渠道 | 进入 unknown；不自动重发 |
| NTF003-RETRY-04 | unknown 且对账证明渠道未接受 | 按剩余预算进入 retry_wait 或 failed；保留对账 evidence_ref |
| NTF003-RETRY-05 | unknown 且渠道不支持查询/幂等 | 保持 unknown；人工重试请求被拒绝 |
| NTF003-SECRET-01 | Secret 缺失、撤销、轮换或授权失效 | 发送前复核；未调用渠道；不把 Secret 写入日志或错误摘要 |

### 10.1 关键事件字段绑定断言

- Run：envelope.subject_ref == payload.run_ref；occurred_at == payload.finished_at；source_event_ref 不得复用 event_id。
- Quant：每条事件只对应一个业务主体；occurred_at == evaluated_at；reason code 集合按去重后的字典序比较。
- Research：envelope.subject_ref == payload.research_event_ref；event_version 严格递增；previous_state != current_state；occurred_at == changed_at。
- 三类事件：重试时 event_id、idempotency_key、occurred_at、correlation_id、payload 必须逐字节语义一致；更正事实必须生成新的领域版本/发布意图。
- Notification：不得根据 event_type 猜测 source_module；不得从模型输出或 payload 解析真实 recipient；发送前重新检查 Subscription、授权和 Secret。

### 10.2 CI 完成定义

1. L1：INT001-SCHEMA-*、ID-*、FP-* 全通过，并将六个 Golden 纳入自动测试。
2. L2：INT001-OUTBOX-*、RUN-*、QUANT-*、RESEARCH-* 由三个生产方分别运行并上传测试报告/提交 SHA。
3. L2：NTF002-*、NTF003-* 在 Fake Repository/Fake Channel 下全通过。
4. L3：MariaDB 唯一约束/事务、真实 Outbox/Inbox、Worker 崩溃恢复、真实渠道幂等/对账能力单独验收，不得混入 L2 结论。
5. 任一契约只有在对应层级的全部适用测试都有可复现 CI 证据后才可升级状态；本清单本身不代表测试已经执行。


## 12. Template/Renderer Fixture

- `template_version_immutable.test`：相同 template_id/version 同 digest 幂等；不同 digest 拒绝覆盖；新版本允许注册。
- `template_allowlist_json_pointer.test`：仅暴露显式 JSON Pointer 标量；缺失路径、非标量、未声明变量和未使用绑定均拒绝。
- `template_html_escape.test` / `template_text_output.test`：HTML 上下文转义，纯文本不执行 HTML 转义。
- `template_output_depth_limit.test`：模板源字节、输出字节和 payload 深度达到/超过上限。
- `template_subject_header_injection.test`：Subject 中 CR/LF/控制字符拒绝。
- `template_event_binding.test`：event_type/schema_version 不匹配或 payload fingerprint 不匹配拒绝。

以上仅验证通用 Renderer 核心；没有真实业务文案和渠道样例，不代表三类首批通知内容或真实渠道渲染已验收。


## 13. NTF-004/005 Attempt 恢复、RetryPolicy 与发送前复核增量

以下场景已在 internal/notification/application 下建立测试代码；当前全仓根模块 Test job 被 Research 语法/NUL 错误阻断，尚未执行，不得标记通过。

| Test ID | 场景 | 必须断言 |
|---|---|---|
| NTF004-REC-01 | 已过期租约且当前 fencing token 没有 Attempt | 仅在完整历史、预算和 deadline 允许时判 safe_to_retry |
| NTF004-REC-02 | 当前 Attempt 仍运行或结果 ambiguous | 判 unknown；不得直接重发 |
| NTF004-REC-03 | Attempt accepted | 收敛 sent，并保留 provider_message_ref |
| NTF004-REC-04 | Attempt rejected | 按持久化 retryable 与预算返回 RetryPolicy 决策；保留 retry_after_at |
| NTF004-REC-05 | Attempt not_sent | 必须保留 evidence_ref 才能安全重试 |
| NTF004-REC-06 | Attempt 历史数量、Delivery/Worker/token/租约不一致 | fail closed，不产生恢复动作 |
| NTF005-RETRY-01 | 首次与后续 Attempt 的 Full Jitter | delay cap 按 base_delay × 2^(attempt_no-1) 指数增长并受 max_delay 限制；随机样本由调用方注入 |
| NTF005-RETRY-02 | Retry-After 晚于本地退避 | next_attempt_at 取 Retry-After；不得缩短本地退避 |
| NTF005-RETRY-03 | Retry-After 早于本地退避 | 使用本地退避，不被过去的 Retry-After 提前 |
| NTF005-RETRY-04 | jitter 低于数据库微秒精度 | 结果至少 1µs；不能被 TIMESTAMP(6) 截断为 now |
| NTF005-RETRY-05 | max_attempts、max_elapsed、deadline 到期 | 不再 retry；返回稳定 retry_budget_exhausted/deadline_exceeded |
| NTF005-RETRY-06 | 非法策略 duration、NaN/Inf/越界随机样本 | 拒绝计算 |
| NTF005-RETRY-07 | Attempt 历史顺序/唯一性 | attempt_no 连续、Attempt ID 唯一、fencing token 严格递增、调用时间不重叠；前序 accepted/ambiguous/永久 rejected 后不得继续自动重试 |
| NTF005-RETRY-08 | Channel rejected 错误分类 | retryable 仅由精确 ChannelConfigSnapshot 的 retryable_error_codes 白名单决定；白名单外错误不可自动重试 |
| NTF005-RETRY-09 | RetryDecision 应用 | 决策绑定 Delivery ID/version、Attempt ID、fencing token、evaluated_at；通过 DeliveryRepository CAS 追加 retry_wait/failed 事件并释放租约 |
| NTF004-PREFLIGHT-01 | Subscription 仍 active、版本已递增、历史 Channel 版本仍可解析且 recipient_ref 未变 | ready；保留 Delivery 的旧 Channel 配置版本 |
| NTF004-PREFLIGHT-02 | Subscription 缺失/paused/disabled | skip；不调用 RecipientResolver/ChannelAdapter.Send |
| NTF004-PREFLIGHT-03 | 当前 recipient_ref 不匹配或显式未授权 | skip；不得发送历史 Delivery |
| NTF004-PREFLIGHT-04 | Channel 精确版本缺失、配置禁用或 Adapter 为空 | block；不得回退 latest 或发送 |
| NTF004-PREFLIGHT-05 | Subscription/Recipient 依赖故障 | block 并保留 error；不得伪装成 skip |
| NTF004-PREFLIGHT-06 | 当前 Subscription 状态非法或版本落后于 Delivery | block；fail closed |
| NTF004-PREFLIGHT-07 | Secret 缺失/撤销/轮换 | 真实 Adapter/Resolver L3 验收；当前 Fake Preflight 不证明真实 Secret 可读 |
