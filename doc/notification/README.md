# Notification 模块

- 目标分支：`feat/notification`
- 代码目录：`internal/notification/**`
- 任务前缀：NTF
- 当前分支 HEAD 以 GitHub `feat/notification` ref 为准。当前阶段：NTF-001～005 的领域、Schema、Fake、Renderer、RetryPolicy、Preflight、unknown 对账能力已建立；NTF-006 已有六表 Migration、DeliveryPlan、Delivery、DeliveryAttempt、Subscription、Inbox 与 Template MySQL Adapter。unknown 对账使用独立 CAS Repository，并验证外部证据、Attempt 历史、不可变预算快照和 StateEvent 一致性。Inbox MySQL Adapter 与 Inbox+Fanout 同事务写入扩展点已建立；Fanout 生产 Writer/WorkItem 执行语义、活跃 Subscription 分页查询及完整 Dispatcher/Worker 仍未完成；其中依赖契约冻结的部分不得自行猜测实现。
- 当前代码事实：`internal/notification/storage` 已包含六表 Migration、DeliveryPlan、Delivery、DeliveryAttempt、Subscription、Inbox 与 Template MySQL Adapter。Subscription Adapter 注入 Server 管理的共享 `*sql.DB`、EventTypeLookup 和 FilterSchemaValidator；Create 仅接受 version=1，GetLatest 读取最新不可变版本，AppendVersion 在事务内锁定最新版本、比较 expectedVersion 并插入新版本，不更新历史行。Delivery Adapter 实现初始 queued 记录创建、读取、事务行锁领取、fencing token 递增、版本 CAS、状态事件同事务追加，以及离开 sending 时释放租约。Attempt Adapter 实现 StartWithLease、CompleteWithLease、ListByDelivery；Start 将 Attempt 插入与 Delivery.attempt_count 投影更新放在同一事务，Start/Complete 均在事务内用 `UTC_TIMESTAMP(6)` 校验数据库权威时钟、Delivery 版本、Worker、fencing token 和租约有效期；同一 Delivery 行锁串行化 Attempt 启动，并依赖 DDL 唯一索引作最终约束。三个 Adapter 均借用调用方提供的共享 `*sql.DB`，不管理连接池生命周期。
- Delivery MariaDB 测试覆盖 Delivery 与不可变 Plan 快照一致性、唯一性、读取、16 Worker 单赢家、状态 CAS、状态事件追加失败回滚和租约释放。Attempt MariaDB 测试覆盖同一租约 8 路并发仅一个 Attempt 成功、attempt_count 投影、运行中/完成记录读取、重复完成拒绝和同 fencing token 二次追加拒绝。上述测试均为已编写的验收代码；DeliveryPlan、Delivery、Attempt、Subscription、Inbox 五组 MariaDB 集成测试统一通过 `openNotificationIntegrationDB` 调用 Platform Storage.OpenSQL 初始化连接池，Migration 统一使用 Platform ApplyMigrations。最近一次代码 CI Run `42602399686`：go-core 通过但不覆盖 Notification；根模块在 gofmt 阶段被 Quant/Market Data 文件差异阻断，Notification 文件无 gofmt 差异，根模块测试未执行。真实 MariaDB DSN 执行证据仍缺，不能据此标记通过。
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
| Channel/Dispatcher/Retry | 部分实现 | DeliveryExecutionService 已串联 Preflight、固定版本 Renderer、请求构建、Attempt 原子启动、ChannelAdapter 调用与 Completion；已补执行成功及 Attempt 启动失败不调用渠道测试。Fanout 队列、Worker 调度循环和真实渠道 Adapter 仍未实现 |
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


### 本轮增量：单条 Delivery 执行链路

- 新增 `DeliveryExecutionService`，由 Server/Worker 将已 Claim 的 Delivery、事件快照、Attempt ID、注入的 Clock 及诊断 evidence_ref 传入；执行前核对 EventID、SourceModule 和 Payload Fingerprint 与 Delivery 快照一致；服务不扫描队列、不负责 Worker 生命周期。
- 执行顺序固定为 Preflight → TemplateRenderer → BuildChannelSendRequest → Attempt.StartWithLease → ChannelAdapter.Send → DeliveryCompletionService.Complete。
- Preflight 的 skip/block 结果通过 DeliveryPreflightCompletionService 持久化并释放租约；渲染或请求构建失败时 fail closed。
- Attempt.StartWithLease 失败立即返回，禁止调用渠道；渠道结果沿用 accepted/rejected/ambiguous/not_sent 分类，ambiguous 由 Completion 收敛到 unknown。
- 新增执行服务测试，覆盖 accepted 完整执行，以及 Attempt 持久化失败时渠道调用次数为 0。该提交尚待 CI；不得将测试代码存在视为通过。
- 最近代码 HEAD：`8d08cf07a651efcd42a514f3dafd94d659f21f97`。真实 MariaDB DSN、真实渠道与 Worker 端到端 L3 仍未验收。


### 当前执行服务 CI 证据（Run `42601621426`）

- `Test go-core`：通过；该 Job 不覆盖 `internal/notification/**`，不能作为本模块测试通过证据。
- 根模块 `Test .`：失败于 `Check gofmt`，差异位于 `internal/quant/**` 与 `internal/marketdata/provider/sina.go`；本轮日志未列出 Notification 文件差异。
- 根模块依赖下载、单测、Race Detector、go vet 均因 gofmt 门禁失败而跳过；Notification 执行服务测试尚无 CI 运行证据。
- MariaDB 集成 Job 未执行；真实 `NOTIFICATION_MARIADB_TEST_DSN` 仍缺。


### 最新 CI 证据（Run `42602399686`，HEAD `8d08cf07a651efcd42a514f3dafd94d659f21f97`）

- `Test go-core`：通过；不覆盖 Notification。
- 根模块 `Test .`：失败于 gofmt，差异仅来自 Quant 与 Market Data 目录；Notification 文件未出现在本轮差异列表。
- 根模块测试、Race Detector、go vet 未执行；本轮执行服务测试仍未取得 CI 执行通过证据。
- MariaDB 集成 Job 未执行，真实 DSN 验收仍缺。


### Delivery 执行服务边界测试补充（HEAD `da05abab86fd2b00f2357720daa74cf3c8122719`）

- [x] 覆盖渠道已返回 accepted、但调用上下文随后取消：执行服务不得在取消上下文下伪造完成；保留未完成 Attempt 与 sending Delivery，交由租约恢复/Unknown 对账保守收敛。
- [x] 覆盖显式可重试 rejected：必须通过 RetryPolicy 转为 retry_wait，保留单次 Attempt，不在当前执行调用内二次发送。
- [ ] 本轮 Go workflow Run `42603330820`：go-core 通过但不覆盖 Notification；根模块 `Test .` 在 gofmt 阶段失败，日志无 Notification 文件差异，根模块测试未执行。新增测试尚无 CI 执行通过证据。



### Delivery 执行输入校验增量（`16d98fa5ce2db66291eff2944c45737db7a2ec55`）

- [x] 在任何 Preflight、Attempt 写入和渠道副作用之前校验 RetryPolicy 随机采样值必须为有限数且位于 [0,1)；非法值直接拒绝，避免渠道已发生副作用后才因退避输入无效而无法完成状态收敛。
- [x] 增加负数、1、NaN、正无穷输入测试，断言不调用渠道且 Delivery/Attempt 不变。
- [ ] 最新提交 Go workflow Run `42603698251` 已排队；测试尚无 CI 执行证据。



### CI gofmt 反馈与修复记录

- Run `42603698251` 对应提交 `16d98fa`：根模块 gofmt 门禁同时发现 Notification 执行测试一处字段对齐差异，以及 Quant 多处格式差异。
- Notification 自身差异已在提交 `1dba2f9` 修复；Quant 差异属于其他模块，本分支不越界修改。
- 提交 `1dba2f9a0cce73c3b581976e72be667828d9774c` 的 Go workflow Run `42603890369` 已排队；在其完成前，不宣称本轮 gofmt/测试通过。
- go-core Job 通过不覆盖 Notification；根模块 gofmt 门禁失败时，根模块单测、Race Detector、go vet 均不会执行。



### 渠道调用超时边界

- [x] `DeliveryExecutionService` 使用 RetryPolicy 的 `PerAttemptTimeout` 为单次 ChannelAdapter.Send 创建独立超时上下文。
- [x] 发送超时后使用父上下文持久化渠道返回的 ambiguous 结果为 Unknown；避免超时上下文已取消导致无法写入对账起点。
- [x] 父上下文被外部取消仍保持未完成 Attempt，由租约恢复路径处理，不将未知副作用转成可重试失败。
- [x] 新增 deadline 存在性及 timeout→Unknown 集成单测。
- [ ] 提交 `ca2c48bf7c93ea17de3dd68ecb80353240485232` 的 Run `42604048525` 正在执行；尚无最终 CI 证据。



### 超时测试 gofmt 修正

- Run `42604048525` 发现本模块超时测试的结构体字段与结果字段对齐问题，已由提交 `6731fb8` 修正。
- 同一 Run 还存在 Quant/Market Data/News 的格式差异；本分支不修改其他模块。
- 最新提交 `6731fb82aa6053be2a0256f021a08149e15c6d68` 的 Run `42604232233` 已排队，需以该 Run 结果确认本模块 gofmt 后的门禁状态。



### 最新执行服务 CI 结果（Run `42604409852`）

- 对应代码 HEAD：`395054c0b04b8dcd1fe5e78b36f5dbfb273752f0`。
- `Test go-core`：正在/已执行独立模块检查；该 Job 不覆盖 Notification。
- 根模块 `Test .`：失败于 gofmt；本轮日志未列出任何 `internal/notification/**` 差异，Notification 本模块格式问题已清除。
- 根模块 gofmt 失败仍由其他模块文件阻断；依赖下载、根模块单测、Race Detector、go vet 均未执行。Notification 新增执行服务测试仍没有 CI 实际运行证据。
- MariaDB 集成 Job 未执行，真实 DSN L3 仍不可比较。


### 模板版本持久化增量（2026-11-09）

- 新增 `template.Repository`：提供不可变版本 `Create/Get`；相同 `template_id/version` 与相同 JCS Digest 幂等，内容不同返回 `ErrTemplateImmutable`。
- Registry 暴露统一的 `ValidateDefinition` 与 `DefinitionDigest`，持久化 Adapter 与内存 Registry 共用同一校验和 RFC 8785 JCS + SHA-256 摘要算法，不另造摘要规则。
- 新增 `MySQLTemplateRepository`，由 Server 注入共享 `*sql.DB` 和显式 Template Limits；读取时重新校验定义并重算 Digest，发现列内容与摘要不一致则 fail closed；返回的变量绑定 Map 为独立副本。
- Notification Migration v3 新增 `notification_template_versions`，主键 `(template_id, version)`，保存事件绑定、格式、Subject/Body、JSON Pointer 绑定和 Digest；不包含预置业务文案。
- 新增 MariaDB L3 集成测试代码，覆盖相同版本幂等、相同版本内容冲突、新版本追加、读取和副本隔离；未设置 `NOTIFICATION_MARIADB_TEST_DSN` 时会 Skip，不能视为已执行。
- 当前限制：本轮没有真实 MariaDB DSN 和完整根模块 CI 结果；模板 Repository 已实现代码与测试，但 CI/数据库验收仍待执行。Server 根 Migration 注册由 Server 分支负责，本分支不越界修改。


### 模板 Repository 与 Registry 装载、最新 CI 证据

- Registry 新增 `LoadFromRepository`：调用方必须显式指定 template_id/version；不回退 latest；先校验持久化 Digest 与定义重算结果一致，再注册到内存 Renderer，避免不一致内容先进入 Registry。
- 新增 Fake Repository 测试，覆盖固定版本装载、缺失版本不回退、Digest 不一致拒绝。
- Go workflow Run `42608383500`（HEAD `1460cd8`）：`Test go-core` 通过，但该 Job 不覆盖 Notification；根模块 `Test .` 仍在 gofmt 门禁失败，日志未列出 `internal/notification/**` 格式差异，因此根模块单测/Race/vet 未执行；MariaDB 集成 Job skipped，真实 DSN L3 未执行。
- 当前结论：模板存储与装载代码已提交，Notification gofmt 门禁无本模块差异；不能据此宣称 Notification 包测试或 MariaDB 集成通过。


### Channel 配置快照持久化增量（2026-11-09）

- 新增 `application.ChannelConfigRepository`，按 `(channel_ref, config_version)` 提供不可变 Create/Get Port；同版本相同快照幂等，不同快照冲突。
- 新增 `MySQLChannelConfigRepository`，只保存 ChannelConfigSnapshot 与 SecretRef，不接触 Secret 明文；重试错误码先排序再按 JCS 计算 SHA-256，避免集合顺序造成伪冲突；读取时重算摘要并验证身份/快照。
- 新增 Notification Migration v4：`notification_channel_config_versions`，主键 `(channel_ref, config_version)`，保存 JSON 快照和摘要。
- 新增 MariaDB 集成测试代码，覆盖幂等、错误码顺序规范化、同版本变更拒绝、新版本追加和读取副本隔离；真实 DSN 尚未执行。
- `ChannelRegistry` 仍只负责运行时 Adapter 注册与精确版本解析；持久化 Repository 与具体 Adapter/Platform Secret 装配分离，Server 负责读取快照并构造具体 Adapter。
