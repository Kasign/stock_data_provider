# Notification 模块

- 目标分支：`feat/notification`
- 代码目录：`internal/notification/**`
- 任务前缀：NTF
- 当前阶段：NTF-001～005 的领域、Schema、Fake、Renderer、RetryPolicy、Preflight 契约已建立；NTF-006 已有五表 Migration、DeliveryPlan、Delivery、DeliveryAttempt 与 Subscription MySQL Adapter。Subscription Adapter 使用 append-only 版本行和事务行锁实现 expectedVersion CAS；新增 MariaDB 集成测试通过 Platform Storage.OpenSQL 建立连接池，覆盖并发追加单赢家。上述新增代码尚待 CI 编译/测试和真实 DSN 执行验证。Inbox Adapter、租约恢复事务与 Fanout/Dispatcher/Worker 仍未完成。
- 当前代码事实：`internal/notification/storage` 已包含五表 Migration、DeliveryPlan、Delivery、DeliveryAttempt 与 Subscription MySQL Adapter。Subscription Adapter 注入 Server 管理的共享 `*sql.DB`、EventTypeLookup 和 FilterSchemaValidator；Create 仅接受 version=1，GetLatest 读取最新不可变版本，AppendVersion 在事务内锁定最新版本、比较 expectedVersion 并插入新版本，不更新历史行。Delivery Adapter 实现初始 queued 记录创建、读取、事务行锁领取、fencing token 递增、版本 CAS、状态事件同事务追加，以及离开 sending 时释放租约。Attempt Adapter 实现 StartWithLease、CompleteWithLease、ListByDelivery；Start 将 Attempt 插入与 Delivery.attempt_count 投影更新放在同一事务，Start/Complete 均在事务内用 `UTC_TIMESTAMP(6)` 校验数据库权威时钟、Delivery 版本、Worker、fencing token 和租约有效期；同一 Delivery 行锁串行化 Attempt 启动，并依赖 DDL 唯一索引作最终约束。两个 Adapter 均借用调用方提供的共享 `*sql.DB`，不管理连接池生命周期。
- Delivery MariaDB 测试覆盖 Delivery 与不可变 Plan 快照一致性、唯一性、读取、16 Worker 单赢家、状态 CAS、状态事件追加失败回滚和租约释放。Attempt MariaDB 测试覆盖同一租约 8 路并发仅一个 Attempt 成功、attempt_count 投影、运行中/完成记录读取、重复完成拒绝和同 fencing token 二次追加拒绝。上述测试均为已编写的验收代码；当前 HEAD 的 Go workflow 尚未完成，真实 MariaDB DSN 执行证据仍缺，不能据此标记通过。
- 本轮功能提交：`c85e539`（Delivery MariaDB Repository）、`cdbf8bd`（Delivery 集成测试）、`2e7102d`（gofmt 修复）、`4bcb0fa`（Delivery 快照一致性与事务回滚）、`d38c8d1`（DeliveryAttempt Adapter）、`c61c132`（Attempt 完成租约校验修正）、`c548ebd`（Attempt MariaDB 集成测试）、`48c19f7`（修正集成测试时钟基线）、`50353a9`/`7b38be3`（对齐 DATETIME(6) 微秒精度）。PR #48 指向 `integration`；当前 HEAD `7b38be3` 对应 Run `42575686306` 的根模块 `Test .` 再次在 gofmt 阶段失败，日志差异只涉及 Research 文件，未出现 Notification 文件；因此 Go tests/race/vet 被跳过，Notification Adapter 未取得编译/测试通过证据。其他独立 Job 仍在执行；真实 MariaDB DSN 证据缺失。

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
| Go Notification Domain/Application | 部分实现 | EventEnvelope、JCS Fingerprint、Envelope/Payload Validator、EventReceiver、Subscription/DeliveryPlan 领域校验已实现；Inbox/Subscription 真实 Repository Adapter 尚未实现 |
| Inbox/Subscription/DeliveryPlan | 部分实现 | 双唯一 Inbox Port/Fake、Subscription append-only Repository Port/Fake 与 MariaDB Adapter（版本追加、CAS 并发）及 DeliveryPlan 唯一性 Port/Fake/Adapter 已实现；Inbox 真实持久化和 fan-out 事务仍待实现；Subscription MariaDB 测试通过 Platform Storage.OpenSQL 建立连接池，尚待 CI/真实 DSN 验证 |
| Template/Renderer | 部分实现 | 固定版本 Registry、JSON Pointer 标量 allowlist、text/HTML 渲染、HTML 自动转义、CRLF Subject 拒绝和显式资源限制已实现；生产模板内容/持久化尚未实现 |
| Channel/Dispatcher/Retry | 部分实现 | Channel Domain、精确版本内存 Registry、Adapter/RecipientResolver Port 与 Fake Adapter 测试已建立；Delivery/Attempt 领域模型、Repository Port/Fake、RetryPolicy、Preflight 已建立；真实 Dispatcher/Worker、租约恢复仍未实现 |
| Repository/Migration | 进行中 | 已建立五表 DDL、DeliveryPlan/Delivery/DeliveryAttempt/Subscription MySQL Adapter 和可选 MariaDB 集成测试；Inbox Adapter、根 Migration 注册和真实 MariaDB 执行证据仍缺 |
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
