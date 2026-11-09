# Notification 模块

- 目标分支：`feat/notification`
- 代码目录：`internal/notification/**`
- 任务前缀：NTF
- 当前 HEAD：`cdac763a6e613a99855c72be1375080c9b10c389`。当前阶段：NTF-001～005 的领域、Schema、Fake、Renderer、RetryPolicy、Preflight、unknown 对账能力已建立；NTF-006 已有五表 Migration、DeliveryPlan、Delivery、DeliveryAttempt 与 Subscription MySQL Adapter。unknown 对账使用独立 CAS Repository，并验证外部证据、Attempt 历史、不可变预算快照和 StateEvent 一致性。Inbox MySQL Adapter 与 Inbox+Fanout 同事务写入扩展点已建立；Fanout 生产 Writer/WorkItem 执行语义、活跃 Subscription 分页查询及完整 Dispatcher/Worker 仍未完成；其中依赖契约冻结的部分不得自行猜测实现。
- 当前代码事实：`internal/notification/storage` 已包含五表 Migration、DeliveryPlan、Delivery、DeliveryAttempt 与 Subscription MySQL Adapter。Subscription Adapter 注入 Server 管理的共享 `*sql.DB`、EventTypeLookup 和 FilterSchemaValidator；Create 仅接受 version=1，GetLatest 读取最新不可变版本，AppendVersion 在事务内锁定最新版本、比较 expectedVersion 并插入新版本，不更新历史行。Delivery Adapter 实现初始 queued 记录创建、读取、事务行锁领取、fencing token 递增、版本 CAS、状态事件同事务追加，以及离开 sending 时释放租约。Attempt Adapter 实现 StartWithLease、CompleteWithLease、ListByDelivery；Start 将 Attempt 插入与 Delivery.attempt_count 投影更新放在同一事务，Start/Complete 均在事务内用 `UTC_TIMESTAMP(6)` 校验数据库权威时钟、Delivery 版本、Worker、fencing token 和租约有效期；同一 Delivery 行锁串行化 Attempt 启动，并依赖 DDL 唯一索引作最终约束。三个 Adapter 均借用调用方提供的共享 `*sql.DB`，不管理连接池生命周期。
- Delivery MariaDB 测试覆盖 Delivery 与不可变 Plan 快照一致性、唯一性、读取、16 Worker 单赢家、状态 CAS、状态事件追加失败回滚和租约释放。Attempt MariaDB 测试覆盖同一租约 8 路并发仅一个 Attempt 成功、attempt_count 投影、运行中/完成记录读取、重复完成拒绝和同 fencing token 二次追加拒绝。上述测试均为已编写的验收代码；DeliveryPlan、Delivery、Attempt、Subscription、Inbox 五组 MariaDB 集成测试统一通过 `openNotificationIntegrationDB` 调用 Platform Storage.OpenSQL 初始化连接池，Migration 统一使用 Platform ApplyMigrations。当前 HEAD 的 Go workflow 尚未完成，真实 MariaDB DSN 执行证据仍缺，不能据此标记通过。
- 本轮新增 Inbox MySQL Repository 与 v2 Migration；新增事务内 Fanout Writer 扩展点和可选 MariaDB 集成测试。Fanout 表结构/生产 Writer、claim/ack/retry 语义仍未实现；没有把扩展点误记为完整 Fanout。真实 MariaDB DSN 执行证据仍缺失。历史验收基线 `338d0ced374d5fdc42672812450d897d89cbe1d0` 对应 Run `42593868099`：`Test go-core` 通过（含测试、Race Detector、go vet）；根模块 `Test .` 在 gofmt 阶段被 `internal/quant/**` 阻断，Notification 文件未出现在差异列表，根模块测试未执行。真实 MariaDB DSN 执行证据仍缺。

Notification 是独立通知业务模块，负责事件接收、Subscription、Template、Channel、Dispatcher、Delivery、Attempt、幂等、重试和投递诊断。不计算业务告警条件，不依赖 Researchflow 内部实现。

设计入口：
- [通知领域与投递设计](01-通知领域与投递设计.md)
- [任务拆分与验收](02-任务拆分与验收.md)
- [Application Port 与投递状态契约](03-ApplicationPort与投递状态契约.md)
- [契约 Fixture 与状态迁移验收清单](04-契约Fixture与状态迁移验收清单.md)
- [首批业务事件映射决议（INT-CTR-001）](05-首批业务事件映射决议.md)
- [事件接收与投递状态契约对齐补充](06-事件接收与投递状态契约对齐补充.md)
- [首批 JSON Schema](schema/)
- [首批 Golden 样例](golden/)

关键边界：recipient_scope 不等于真实发送目标；外部渠道不承诺 exactly-once；发送结果不确定时不得盲目重试。Platform 提供 Secret、HTTP Client、Storage 等公共技术原语；Notification 自己拥有领域状态、Repository 和有界 Dispatcher。当前 `internal/platform/taskqueue` 耦合旧 Analysis 领域，不作为 Notification Worker 复用；根 Worker/DI 装配由集成方负责。

## 当前开发状态

| 范围 | 状态 | 证据/限制 |
|---|---|---|
| 领域设计与字段契约 | 已建立 | 以 01/03 文档为准；发现契约矛盾先修正文档并评审 |
| Schema/Golden 资产 | 模块 CI 已通过，跨模块证据待补 | 4 Schema、6 Golden；Draft 2020-12 Envelope/Payload Validator 和 Golden 测试已编写；完整 CI Run 42201893234 已通过；NTF-CTR-001～003 仍因跨模块/真实 Adapter 证据不足保持 pending_evidence |
| Go Notification Domain/Application | 部分实现 | EventEnvelope、JCS Fingerprint、Envelope/Payload Validator、EventReceiver、Subscription/DeliveryPlan 领域校验已实现；Inbox MySQL Repository 已实现，Fanout Writer 生产实现尚未完成 |
| Inbox/Subscription/DeliveryPlan | 部分实现 | 双唯一 Inbox Port/Fake 与 MySQL Adapter（完整 Envelope 重放校验、事务内 Fanout Writer 扩展点）、Subscription append-only Repository Port/Fake 与 MariaDB Adapter（版本追加、CAS 并发）及 DeliveryPlan 唯一性 Port/Fake/Adapter 已实现；Inbox 表及双唯一键已持久化；Fanout 具体表/Writer 尚未冻结实现，Repository 通过事务内 Writer 扩展点确保新 Inbox 与 Fanout 原子提交；四组 MariaDB 测试统一通过 Platform Storage.OpenSQL 建立连接池，尚待 CI/真实 DSN 验证 |
| Template/Renderer | 部分实现 | 固定版本 Registry、JSON Pointer 标量 allowlist、text/HTML 渲染、HTML 自动转义、CRLF Subject 拒绝和显式资源限制已实现；生产模板内容/持久化尚未实现 |
| Channel/Dispatcher/Retry | 部分实现 | Channel Domain、精确版本内存 Registry、Adapter/RecipientResolver Port 与 Fake Adapter 测试已建立；Delivery/Attempt 领域模型、Repository Port/Fake、RetryPolicy、Preflight 已建立；真实 Dispatcher/Worker、租约恢复仍未实现 |
| Repository/Migration | 进行中 | 已建立五表基础 DDL + Inbox v2 Migration、Inbox/DeliveryPlan/Delivery/DeliveryAttempt/Subscription MySQL Adapter 和可选 MariaDB 集成测试；Fanout Writer 生产实现、根 Migration 注册和真实 MariaDB 执行证据仍缺 |
| 三生产方事件接入 | 未验收 | 仅按各生产方契约和真实测试证据逐项开放 |
| 真实渠道与 L3 集成 | 未开始/不可比较 | 无真实凭据、回执、MariaDB/Worker 集成证据时不得标记通过 |

## 开发边界

1. 每项开发先检查当前分支代码和测试，已有能力只修缺陷。
2. 新接口必须写明调用方、调用时机和失败语义；无真实调用场景不新增接口。
3. 新字段必须写明来源、语义、是否必填、版本和持久化要求。
4. 诊断结果使用通过/降级/失败/不可比较；缺少真实证据不得判通过。
5. 不为 WebUI 展示提前增加无实际数据支撑的 API。
6. 根 DI、HTTP、Worker、Migration 注册及跨模块生产方修改不属于本分支自主范围，必须有明确集成任务。
7. 模块级 Fake 测试与真实渠道、MariaDB、Worker 的 L3 验收分开记录。


### 最新增量：Inbox 接收时间精度

- Application 与 MySQL Inbox Repository 统一将接收时间截断到微秒，匹配 MariaDB `DATETIME(6)`；首次接收与重复接收返回同一持久化时间精度。
- 新增纳秒时钟输入回归测试，验证 AcceptedAt 微秒精度及重放稳定性。
- 当前 HEAD：`f7eb85a0ed185a3ce28a749a51c9ae66292382d7`；Go workflow Run `42597043749` 已排队，尚无测试结果。


### Inbox 时间精度修复 CI 结果

- 代码提交：`f7eb85a0ed185a3ce28a749a51c9ae66292382d7`；Run `42597043749`。
- `Test go-core`：通过（gofmt、单测、Race Detector、go vet）。
- 根模块 `Test .`：仍在 gofmt 阶段失败；失败门禁来自仓库其他目录的格式差异，根模块测试未执行。本次不能据此声称 Notification 根模块测试通过。
- MariaDB 集成 Job 未执行；真实 DSN 验收仍缺。


### 本轮增量：持久化时间精度统一

- Delivery Repository：Create、ClaimDue、UpdateWithLease 对输入时间统一转换为 UTC 微秒精度，匹配 MariaDB `DATETIME(6)`；避免首次返回值与重读值不一致，以及 lease/fencing 时间比较因纳秒截断产生偏差。
- Subscription Repository：Create、AppendVersion 的 created_at、updated_at、effective_at 统一转换为 UTC 微秒精度，保证版本追加时 created_at 与数据库事实一致。
- 新增 Delivery、Subscription 时间规范化单元测试，覆盖非 UTC 时区、纳秒截断和不修改调用方对象。
- Inbox 时间精度修复已在前一提交完成。
- Delivery 修复提交 `af05f6595b18717408e91a7650e3e89b3b6de236` 的 Run `42597457159`：go-core 全部通过；根模块在 gofmt 阶段失败，未执行根模块测试，不能认定为 Notification 根模块测试失败或通过。失败门禁来自 Quant 目录格式问题。
- Subscription 修复提交 `28d0a2b1111a65bf6c8b52402557f4fedfa87ad4` 的 Run `42597527290`：go-core 已通过；根模块 gofmt 门禁失败，根模块测试未执行。MariaDB 集成 Job 未执行。
- 当前真实 MariaDB 验收仍缺 `NOTIFICATION_MARIADB_TEST_DSN`。


### 补充：恢复与对账路径时间精度

- 复核发现 Delivery 时间精度统一还遗漏两条写入路径：过期租约恢复 `RecoverExpiredLease` 与 Unknown 对账 `ReconcileUnknown`。两者同样写入 `DATETIME(6)`，此前可能保留领域纳秒时间并在数据库中截断。
- 已在 Repository 边界统一规范化 Delivery 快照时间与 StateEvent.changed_at；过期租约恢复同时规范化传入的 now，再执行时间边界校验。
- 补齐 Subscription 时间精度辅助函数所需的标准库依赖，避免根模块编译遗漏。
- 最近一次 CI Run `42598066759`：go-core 的 gofmt、单测、Race Detector、go vet 全部通过；根模块在 gofmt 阶段仍失败，根模块测试未执行。该 Run 对应提交 `d65b57d`，后续恢复/对账精度修复提交 `722f5fb` 尚待 CI。
- 真实 MariaDB L3 仍未执行，缺少 `NOTIFICATION_MARIADB_TEST_DSN`。

### 本轮增量：Attempt 序号与 Claim 快照对齐

- `ClaimDue` 返回的 Delivery 是开始本次 Attempt 前的快照，`attempt_count` 只会由 `StartWithLease` 原子递增；因此完成服务必须校验 `attempt_no == claim.delivery.attempt_count + 1`，不能要求二者相等。
- 修正完成服务的序号边界校验。保留 Delivery 版本、Worker、fencing token、租约截止时间及渠道配置版本校验；Attempt 历史仍由 Repository 负责原子落库。
- 现有完成服务测试覆盖 accepted、ambiguous、not_sent 和 retryable rejection 的完整路径，修复后应由 Notification go-core CI 统一验证。真实 MariaDB L3 仍需 `NOTIFICATION_MARIADB_TEST_DSN`。


### 本轮增量：DeliveryPlan 快照与冗余列一致性

- DeliveryPlan 幂等重放现在同时读取 `plan_json` 和全部冗余列，校验 Delivery 身份、订阅/渠道/模板版本、接收目标、Payload/Plan Fingerprint、授权策略版本、审计授权引用及幂等键完全一致。
- 任一冗余列与不可变 JSON 快照不一致，或授权 JSON 列无法解析，均 fail closed，不返回幂等成功。
- 新增存储层单元测试，覆盖身份字段、版本、Fingerprint、授权策略/引用 JSON、幂等键及非法 JSON。
- 真实 MariaDB 行损坏/重放场景仍需 `NOTIFICATION_MARIADB_TEST_DSN` 执行；当前不标记 L3 通过。
