# Notification 模块

- 目标分支：`feat/notification`
- 代码目录：`internal/notification/**`
- 任务前缀：NTF
- 当前阶段：NTF-001 核心领域实现进行中；EventEnvelope、JCS Fingerprint、Envelope/Payload Schema Registry/Validator、EventReceiver、Inbox replay、Subscription 校验和 DeliveryPlan 指纹基础类型已实现，真实 Repository 与快照持久化尚未完成。
- 当前代码事实：`internal/notification/domain`、`internal/notification/schema` 和 `internal/notification/application` 已建立；接收用例先校验 Envelope/Payload，再调用 InboxRepository。Repository Port 要求原子双唯一键检查并将 Inbox 与 fan-out 工作项一起持久化；当前只有 Fake Repository 测试，没有真实持久化 Adapter。旧 Python/Web 通知能力不属于 Go Notification Domain。
- 契约证据：NTF-CTR-001～003 均为 `pending_evidence`。4 份 Schema、6 个 Golden 已校准；最近验证基线 `c8db1b9b` 的完整 Go workflow 已通过（根模块 gofmt/go test/race/vet、go-core、market-data 测试、MariaDB 集成及 Docker 构建）。当前提交新增领域边界测试后仍需重新跑 CI。

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

关键边界：recipient_scope 不等于真实发送目标；外部渠道不承诺 exactly-once；发送结果不确定时不得盲目重试。Platform 提供 Secret、HTTP、Storage 和 Worker 基础设施，领域状态与 Repository 归 Notification 所有。

## 当前开发状态

| 范围 | 状态 | 证据/限制 |
|---|---|---|
| 领域设计与字段契约 | 已建立 | 以 01/03 文档为准；发现契约矛盾先修正文档并评审 |
| Schema/Golden 资产 | 已校准，待 CI | 4 Schema、6 Golden；Draft 2020-12 Envelope/Payload Validator 和 Golden 测试已编写；根模块 go test、race、vet 已通过（Run 42192962140）；其余 workflow job 仍在执行 |
| Go Notification Domain/Application | 部分实现 | EventEnvelope、JCS Fingerprint、Envelope/Payload Validator、EventReceiver、Subscription/DeliveryPlan 领域校验已实现；真实 Repository Adapter 尚未实现 |
| Inbox/Subscription/DeliveryPlan | 部分实现 | Fake Repository 覆盖双唯一键重放/冲突/32 路并发；Subscription event_type/recipient_scope/filter/version 校验和 DeliveryPlan 指纹/唯一键已实现；持久化与 fan-out 仍待实现 |
| Template/Channel/Dispatcher/Retry | 未开始 | 无实现；不得以旧 Python 通知能力替代 |
| Repository/Migration | 未开始 | Platform Storage 已有通用基础能力；Notification 专属模型/迁移尚无 |
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
