# Notification 模块

- 目标分支：`feat/notification`
- 代码目录：`internal/notification/**`
- 任务前缀：NTF
- 当前阶段：NTF-001 Fake/Domain 测试已覆盖并发与失败恢复，真实 Repository Adapter 留待 NTF-006；NTF-002 Renderer 核心已实现；NTF-003 已建立 Channel Domain/Port/精确版本内存 Registry 与 Fake Adapter 测试；NTF-004 已实现 Delivery 状态领域模型与迁移测试、DeliveryRepository Port/Fake，以及 DeliveryAttempt 追加式领域快照和 Complete 纯函数；Attempt Repository Port/Fake 已建立并覆盖开始/完成租约约束；真实 Attempt 持久化 Adapter、Dispatcher/Worker 和租约恢复尚未实现。
- 当前代码事实：`internal/notification/domain`、`schema`、`application` 和 `template` 已建立；ChannelConfigSnapshot、ChannelAdapter/Resolver、RecipientResolver Port 与内存 ChannelRegistry 已建立；InboxRepository、SubscriptionRepository、DeliveryPlanRepository 已定义 Port。当前有 Inbox/Subscription/DeliveryPlan Fake Repository 测试覆盖并发、CAS、不可变版本和失败后无部分写入/重试；DeliveryPlan 授权策略版本按 GOV-CTR-006 完整 tuple 保存并参与指纹，没有真实持久化 Adapter。Event/Template 资源限制均显式配置，项目建议值尚未冻结，不在模块内设隐式默认值。Template Registry 仅提供内存不可变版本和 Renderer 核心，尚无生产模板内容或持久化 Adapter。旧 Python/Web 通知能力不属于 Go Notification Domain。
- 契约证据：NTF-CTR-001～003 均为 `pending_evidence`。4 份 Schema、6 个 Golden 已校准；上一轮完整 CI Run 42201893234 已通过；前一 HEAD `eed8b817c7872cb1f07106fc901f15024e93e074` 对应 Run 42208589747 的根模块 Test job 在 gofmt 阶段被其他模块的 `internal/research/context/validate.go` 语法错误及 `internal/research/evidence/authorization.go` 非法 NUL 字符阻断；未修改其他模块，Delivery 状态迁移新增测试尚未获得通过证据；本轮 Go test suite Run `42550387738`（PR）/`42550382415`（push）未通过：根模块 gofmt 被 Research 文件语法/NUL 问题阻断，market-data 测试因日历上下文缺失失败；PR #42 Run `42551537558` 再次出现同样的根模块 gofmt 阻断，market-data 测试返回 2026-11-06、期望 2026-11-08，Notification package tests 未执行；go-core、News/Market Data MariaDB 集成和 Market Data MariaDB migration/PIT 集成通过。Notification package tests 未执行，新增 Fake 仅为待 CI 验证代码。NTF-CTR-001～003 仍因跨模块/真实 Adapter 证据不足保持 pending_evidence。

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
| Repository/Migration | 未开始 | Platform Storage 已有连接/事务/Migration Runner 原语；Notification 专属 Repository、表模型和迁移尚无 |
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
