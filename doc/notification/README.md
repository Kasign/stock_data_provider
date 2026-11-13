# Notification 模块

- 目标分支：`feat/notification`
- 代码目录：`internal/notification/**`
- 文档目录：`doc/notification/**`
- 任务前缀：NTF
- 当前分支代码验收基线：`2542086af19578bc439d2d3ffa1b4e6fd26c6c44`。其后仅有本分支文档提交，不改变代码验收基线。
- 最新完整 CI：Run `43196936868`，对应代码 SHA `2542086af19578bc439d2d3ffa1b4e6fd26c6c44`，结论 `success`。根模块 gofmt、`go test ./...`、`go test -race ./...`、`go vet ./...` 均通过；Notification MariaDB lifecycle integration 及其 race 测试通过。
- CI 中 Market Data、News 及 Docker/迁移相关 Job 因分支条件未满足而 skipped；不得据此认定这些模块已在本分支验收。
- MariaDB L3 证据仅覆盖当前 CI 配置下的 Notification 生命周期、存储和并发测试，不等同于用户部署环境或真实渠道 Provider 验收。
- 项目级契约：NTF-CTR-001/002 已冻结；NTF-CTR-003 为 not_applicable；INT-CTR-001 已冻结。契约冻结、模块 CI 通过和生产渠道验收是不同阶段。

## 设计入口

- [通知领域与投递设计](01-通知领域与投递设计.md)
- [任务拆分与验收](02-任务拆分与验收.md)
- [Application Port 与投递状态契约](03-ApplicationPort与投递状态契约.md)
- [契约 Fixture 与状态迁移验收清单](04-契约Fixture与状态迁移验收清单.md)
- [首批业务事件映射决议（INT-CTR-001）](05-首批业务事件映射决议.md)
- [事件接收与投递状态契约对齐补充](06-事件接收与投递状态契约对齐补充.md)
- [首批 JSON Schema](schema/)
- [首批 Golden 样例](golden/)

## 当前开发状态

| 范围 | 状态 | 证据/限制 |
|---|---|---|
| Event / Inbox / Subscription | 模块核心实现完成 | 当前代码基线全仓测试、Race、Vet 通过；MariaDB 生命周期集成通过 |
| Template / Renderer / Repository | 模块核心实现完成 | 真实业务模板文案及渠道专属格式仍需业务决议，不使用占位模板 |
| Channel Domain / Config / Registry | Port、Fake、精确版本选择及持久化完成 | 首个真实 Provider、协议及凭据来源尚未决议 |
| Fanout Factory / Dispatcher | 模块内实现完成 | 非空 Filter 的具体事件语义需事件 Owner 冻结 Schema |
| Delivery / Attempt / Retry / Recovery | 模块核心实现完成 | 真实渠道回执、幂等和 Lookup 语义需由具体 Provider 验收 |
| Diagnostics / Consistency | 模块内只读诊断实现完成 | 根 HTTP、权限和审计接入归 Server |
| Recipient Directory / Worker / 根 Migration | 部分接入 | Integration 已注册根 Migration，并接入 Research/Researchflow Outbox → Inbox/FanoutWorkItem Relay；真实身份目录、Fanout/Delivery Worker 和渠道 Provider 仍未装配 |

## 未完成事项与责任边界

1. **真实渠道 Adapter**：Notification 提供 ChannelAdapter SPI、配置快照和精确版本注册能力。只有确定首个渠道、通信协议、凭据来源及回执/幂等/查询语义后，才能实现真实 Provider；不得用 Fake 或虚构配置宣称生产完成。
2. **非空 Filter Matcher**：当前已有 Matcher Port，非空 Filter 在没有匹配器时 fail closed。具体字段、版本和匹配语义必须由事件 Owner 冻结，Notification 不自行发明 DSL。
3. **生产装配**：根 Migration、EventReceiver/Inbox 和 Research/Researchflow Outbox Relay 已在 integration 接入；真实 Recipient Directory、Fanout/Delivery Worker、渠道 Provider 和平台依赖装配仍由 Server 组合根负责；本次 Inbox 接收不代表渠道投递。
4. **业务模板**：真实模板文案及渠道专属格式由业务决议提供；不创建占位业务模板。

## 关键安全边界

- `recipient_ref` 只能是不透明稳定引用；敏感地址由受控 Channel/Recipient Adapter 解析。
- `recipient_scope` 不等于真实发送目标。
- 外部渠道不承诺 exactly-once；发送结果不确定时进入 unknown 并对账，不盲目重试。
- Channel、Template 均按 DeliveryPlan 固定版本精确解析，不回退 latest。
- Platform 提供 Secret、HTTP Client、Storage 等公共技术原语；Notification 拥有自身领域状态、Repository 和有界 Dispatcher。
- `internal/platform/taskqueue` 与旧 Analysis 领域耦合，不作为 Notification Worker 复用。

## 开发与验收边界

1. 每项开发先检查当前分支代码和测试；已有能力只修复有证据的缺陷。
2. 新接口必须有真实调用方、调用时机和失败语义；无真实场景不新增接口。
3. 新字段必须说明来源、语义、必填性、版本和持久化要求。
4. 诊断结果使用通过/降级/失败/不可比较；缺少真实证据不得判通过。
5. 根 DI、HTTP、Worker、Migration 注册及跨模块生产方修改不属于本分支自主范围。
6. 不要求先合并到 integration 才能执行本分支自身测试或 CI；本分支独立验收以其当前 HEAD 对应的工作流结果为准。

当前详细任务状态和历史实施证据以 [任务拆分与验收](02-任务拆分与验收.md) 为准。历史提交记录中的旧 CI 结果仅作为历史证据，不代表当前状态。


## Integration 接入状态（2026-11-13）

Server 在 integration 已通过统一 EventReceiver 构造入口装配 Notification Schema Registry、MySQL Inbox Repository 和 FanoutWorkItem Writer，并接入 Research 与 Researchflow 两条 Outbox Relay。Researchflow Relay 的 MariaDB 成功接收、失败重试、幂等和组件启停测试通过；Integration Run #54（`43359166036`，SHA `63f49065a5ef3379d95948a6c2c6e504a555a63f`）全流程 CI 通过。

当前完成边界是 Outbox → EventReceiver → Inbox/FanoutWorkItem。Recipient Directory、真实 Channel Provider、Fanout/Delivery Worker、真实回执与 Unknown 对账仍未完成，不得将 Inbox 接收计为渠道发送成功。