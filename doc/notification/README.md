# Notification 模块

- 目标分支：`feat/notification`
- 代码目录：`internal/notification/**`
- 任务前缀：NTF
- 当前阶段：NTF-001 Event/Subscription/DeliveryPlan 领域校验、Schema/Golden、Fake 与并发测试已建立；NTF-002 Renderer 核心已实现；NTF-003 Channel Domain/Port/内存 Registry/Fake 已建立；NTF-004/005 已具备 Delivery 状态机、CAS/Fake、Attempt Port/Fake、租约恢复评估、发送前 Preflight 和 RetryPolicy。NTF-006 已有五表 Migration 与 DeliveryPlan MySQL Adapter；Delivery、Attempt、Subscription、Inbox 的真实 MariaDB Repository Adapter、恢复扫描事务、Fanout/Dispatcher/Worker 仍未完成。
- 当前代码事实：`internal/notification/domain`、`schema`、`application`、`template` 和 `storage` 已建立；Inbox/Subscription/DeliveryPlan、Delivery、DeliveryAttempt 的 Port/Fake 已建立。DeliveryPlan MySQL Adapter 使用唯一业务 tuple、冲突回读和 fingerprint 校验，保存不可变 `plan_json` 快照；其 MariaDB 集成测试由 `NOTIFICATION_MARIADB_TEST_DSN` 显式启用，目前尚无真实执行证据。Migration v1 定义 Subscription、DeliveryPlan、Delivery、Attempt、DeliveryStateEvent 五类表，尚未接入 Server 根 Migration。Inbox/Fanout 表因工作项读写和订阅分页契约未冻结而暂缓；不得在模块内臆造契约。旧 Python/Web 通知能力不属于 Go Notification Domain。
- 本轮代码变更提交：`386e04e`（Delivery lease/fencing DDL）、`c61e735`（DeliveryPlan MySQL Adapter）、`39f6983`（返回独立的 DeliveryPlan 快照）、`7883908`（修复 MariaDB 集成测试 gofmt）。当前 HEAD：`7883908b1d40929f9a4fa541d9e70d283ebbe022`。最近 CI Run `42566174010` 对应前一 HEAD `39f69834754460237a3294267f6fc150c265de42`：根模块在 gofmt 阶段失败，失败列表同时包含 Notification 集成测试和多个其他模块文件；Notification 集成测试的格式问题已在 `7883908` 修复，其他模块文件不属于本分支。该 Run 未执行依赖下载、根模块测试、race 或 vet；其他独立 job（go-core、Market Data、News MariaDB、Docker）通过。`7883908` 后的新 CI 结果尚待确认。Notification MariaDB 集成测试尚未在真实 DSN 环境执行。NTF-CTR-001～003 继续保持 `pending_evidence`，不标记真实验收通过。

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
| Go Notification Domain/Application | 部分实现 | EventEnvelope、JCS Fingerprint、Envelope/Payload Validator、EventReceiver、Subscription/DeliveryPlan 领域校验已实现；真实 Repository Adapter 尚未实现 |
| Inbox/Subscription/DeliveryPlan | 部分实现 | 双唯一 Inbox Port/Fake、Subscription append-only Repository Port/Fake（版本追加、CAS 并发）与 DeliveryPlan 唯一性 Port/Fake、完整授权策略 tuple 指纹冲突测试已实现；真实持久化和 fan-out 事务仍待实现 |
| Template/Renderer | 部分实现 | 固定版本 Registry、JSON Pointer 标量 allowlist、text/HTML 渲染、HTML 自动转义、CRLF Subject 拒绝和显式资源限制已实现；生产模板内容/持久化尚未实现 |
| Channel/Dispatcher/Retry | 部分实现 | Channel Domain、精确版本内存 Registry、Adapter/RecipientResolver Port 与 Fake Adapter 测试已建立；Delivery 状态领域模型/迁移测试及 Repository Port/Fake、Claim/CAS 并发测试已编写；DeliveryAttempt 领域模型与 Attempt Repository Port/Fake 也已建立，测试尚未获得本轮 CI 通过证据；真实 Repository Adapter、Dispatcher/Worker、租约恢复和 Retry 尚未实现 |
| Repository/Migration | 进行中 | 已建立首批 DDL、DeliveryPlan MySQL Adapter 和可选 MariaDB 集成测试；Delivery/Attempt/Subscription/Inbox Adapter、根 Migration 注册和真实 MariaDB 执行证据仍缺 |
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
