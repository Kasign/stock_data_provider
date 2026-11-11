# Notification 模块

- 目标分支：`feat/notification`
- 代码目录：`internal/notification/**`
- 任务前缀：NTF
- 当前分支 HEAD 以 GitHub `feat/notification` ref 为准；文档中的历史 SHA/Workflow 仅作为历史证据，不代表当前 HEAD。当前阶段：FanoutWorkItem、订阅修订快照、稳定 keyset 分页、MySQL Repository 与有界 Dispatcher 已实现；当前提交 `f2dbe00` 的 CI 尚未执行，真实 MariaDB L3 尚未执行。NTF-CTR-001/002 已 L1 frozen，NTF-CTR-003 为 not_applicable；L1 不代表 L2/L3 验收通过。
- 当前代码事实：十类业务表及 v1～v6 Migration 已定义；新增订阅事务修订号分配、FanoutWorkItem MySQL Repository（与 Inbox 同事务写入）、数据库时钟 Claim、租约/fencing、CAS 页面推进/重试、固定修订快照的 Subscription keyset 分页和有界 FanoutDispatcher。FanoutPlanFactory 是明确的模块内注入端口，尚无基于真实 Channel/Template/Recipient/Authorization 数据的生产装配；Server Worker 与根 Migration 注册仍属集成任务。
- Delivery MariaDB 测试覆盖 Delivery 与不可变 Plan 快照一致性、唯一性、读取、16 Worker 单赢家、状态 CAS、状态事件追加失败回滚和租约释放。Attempt MariaDB 测试覆盖同一租约 8 路并发仅一个 Attempt 成功、attempt_count 投影、运行中/完成记录读取、重复完成拒绝和同 fencing token 二次追加拒绝。上述测试均为已编写的验收代码；DeliveryPlan、Delivery、Attempt、Subscription、Inbox、Template、ChannelConfig 七组 MariaDB 集成测试统一通过 `openNotificationIntegrationDB` 调用 Platform Storage.OpenSQL 初始化连接池，Migration 统一使用 Platform ApplyMigrations。此前 CI Run `43005827183` 未执行 Notification 测试；本轮 `f2dbe00` 新增的 Fanout 单测和 MariaDB 集成测试代码尚未获得 CI 执行证据。MariaDB L3 必须使用真实 `NOTIFICATION_MARIADB_TEST_DSN`，当前状态为 `not_comparable`。
- 本轮新增 Migration v6、订阅修订号分配器、FanoutWorkItem 表/Repository、Inbox 事务内 Writer、固定快照分页查询、Fanout Dispatcher 与对应单测/MariaDB 集成测试。没有新增对外 API；没有实现虚构渠道或默认接收目标。历史验收基线 `338d0ced374d5fdc42672812450d897d89cbe1d0` 对应 Run `42593868099`：`Test go-core` 通过（含测试、Race Detector、go vet）；根模块 `Test .` 在 gofmt 阶段被 `internal/quant/**` 阻断，Notification 文件未出现在差异列表，根模块测试未执行。真实 MariaDB DSN 执行证据仍缺。

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
| Schema/Golden 资产 | Schema/Golden 与测试代码已建立；本轮 CI 尚待执行 | 4 Schema、6 Golden；Draft 2020-12 Envelope/Payload Validator 和 Golden 测试已编写。此前 Run `43005827183` 未执行 Notification 测试；本轮 `f2dbe00` 的 Fanout 测试需在 integration 上获得执行证据。L1 状态以 PLAT-002 为准；真实 Adapter/Server 装配/生产渠道仍未验收 |
| Go Notification Domain/Application | 部分实现 | EventEnvelope、JCS Fingerprint、EventReceiver、Subscription/DeliveryPlan、FanoutWorkItem 与有界 FanoutDispatcher 已实现；FanoutPlanFactory 需由真实数据/Server 装配注入，CI 尚待执行 |
| Inbox/Subscription/DeliveryPlan/Fanout | 部分实现 | Inbox 与 FanoutWorkItem 同事务；Subscription 通过事务修订号固定快照并提供 keyset 分页；DeliveryPlan 复用既有幂等 Repository。单测和 MariaDB 集成测试已编写，尚待 CI/真实 DSN 执行 |
| Template/Renderer | 部分实现 | 固定版本 Registry、JSON Pointer 标量 allowlist、text/HTML 渲染、HTML 自动转义、CRLF Subject 拒绝和显式资源限制已实现；生产模板内容/持久化尚未实现 |
| Channel/Dispatcher/Retry | 部分实现 | DeliveryExecutionService 与有界 FanoutDispatcher 已实现；FanoutPlanFactory 真实装配、Server Worker 生命周期及真实渠道 Adapter 仍待集成/L3 |
| Repository/Migration | 部分实现 | 十类业务表及 Migration v1～v6；Fanout/Subscription Adapter 已实现；根 Migration 注册由 Server 负责，真实 MariaDB 执行证据仍缺 |
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


### Channel 配置持久化 CI 反馈

- Go workflow Run `42609082056` 对应 ChannelConfig Repository 代码提交：`Test go-core` 通过，但不覆盖 Notification；根模块 `Test .` 仍因仓库其他模块 gofmt 差异失败，日志未列出 `internal/notification/**` 差异，因此根模块单测/Race/vet 未执行。
- MariaDB 集成 Job skipped；当前没有 `NOTIFICATION_MARIADB_TEST_DSN` 的执行证据。ChannelConfig 的 Migration v4 和集成测试代码已提交，但 L3 未验收。


### 最新分支 CI：ChannelConfig 摘要单测增量

- Run `42609496390` 对应代码 HEAD `70189fc`：`Test go-core` 成功；根模块 `Test .` 仍在全仓 gofmt 门禁失败，日志未列出 `internal/notification/**` 差异，根模块单测/Race/vet 未执行。
- MariaDB 集成 Job skipped；没有真实 `NOTIFICATION_MARIADB_TEST_DSN` 执行证据。ChannelConfig Repository 的摘要规范化单测已加入，但当前尚无 CI 实际运行通过证据。


### DeliveryExecutionService 完整 Attempt Deadline 接入（2026-11-09）

- 执行服务在创建 Attempt 前读取完整历史，要求历史数量与 `Delivery.AttemptCount` 一致、AttemptNo 连续、DeliveryID 一致、Attempt 校验通过、FencingToken 严格递增、前序 Attempt 已完成且时间不倒退；历史不完整或冲突时 fail closed，不触发渠道。
- 使用 `RetryPolicy.AttemptDeadline` 计算渠道调用的绝对截止时间：`min(now + PerAttemptTimeout, firstAttemptAt + MaxElapsed, Delivery.DeadlineAt)`；将其通过 `context.WithDeadline` 传给单次 Channel Send。
- 预算耗尽时不创建 Attempt、不调用渠道，保留 sending/租约事实交给租约恢复路径收敛，避免把未知外部副作用错误转成安全重试。
- 新增执行服务测试，验证 MaxElapsed 比 PerAttemptTimeout 更早时，渠道 context deadline 精确受 MaxElapsed 限制。
- 本次代码与测试已提交；尚未取得该提交对应 CI 的 Notification 根模块测试执行证据，也未执行真实 MariaDB L3。


### 完整执行期限与租约恢复预算统一（2026-11-09）

- `DeliveryExecutionService` 在创建 Attempt 前读取并校验完整历史，随后通过 `RetryPolicy.AttemptDeadline` 将 `PerAttemptTimeout`、首次 Attempt 起算的 `MaxElapsed`、Delivery `DeadlineAt` 合并为最早 deadline；渠道调用使用该绝对 deadline。
- `DeliveryAttemptRepository.ListByDelivery` 明确要求按 AttemptNo 升序返回完整历史；执行服务检查数量、连续序号、Delivery 身份、FencingToken 单调递增和前序完成时间。
- `LeaseRecoveryService` 对 `not_sent`/无当前 Attempt 等安全重试路径补齐 MaxElapsed 检查；即使状态评估判定可安全重试，也不能越过首次 Attempt 起算的 MaxElapsed 或 Delivery Deadline。
- 新增测试覆盖 MaxElapsed 收紧渠道 deadline，以及 MaxElapsed 已耗尽时安全恢复仍必须终止为 failed/deadline_exceeded。
- 本轮测试代码已加入；CI 尚未提供该提交的 Notification 根模块测试执行结果，MariaDB L3 仍待真实 DSN。


### 过期租约扫描并发窗口收紧

- 过期租约扫描在初次批量查询 ID 后，逐条回读租约时再次使用数据库 UTC 时钟过滤 `lease_expires_at <= UTC_TIMESTAMP(6)`。
- 防止初次扫描后租约已被续期、但二次读取仍把该 Delivery 返回为过期候选；最终恢复写入仍由 Repository 通过版本、fencing token 和数据库时钟 CAS 兜底。
- MariaDB 集成用例增加 active → expired → renewed 场景，验证扫描候选随数据库当前租约状态变化；真实 DSN 尚未执行。


### 本轮执行预算与租约扫描 CI 反馈

- Go workflow Run `42611257814` 对应 HEAD `f4d100a`：`Test go-core` 成功，但不覆盖 Notification；根模块 `Test .` 在全仓 gofmt 门禁失败，差异来自 Market Data、News、Quant 等其他目录，日志未列出 `internal/notification/**` 差异。
- 根模块单测、Race Detector、go vet 未执行；Notification 新增执行期限/恢复预算测试及 MariaDB 扫描集成用例尚无实际执行通过证据。
- MariaDB 集成 Job skipped；真实 `NOTIFICATION_MARIADB_TEST_DSN` L3 未执行。


### 执行期限与 Attempt 历史一致性（代码复核修正，2026-11-09）

- 当前代码已将 `RetryPolicy.AttemptDeadline` 接入 `DeliveryExecutionService`：渠道调用 deadline 取单次超时、首次 Attempt 起算 MaxElapsed、Delivery Deadline 的最早值；预算耗尽时不创建 Attempt、不调用渠道。
- 执行前读取完整 Attempt 历史，校验数量、连续序号、完成状态、FencingToken 递增、时间顺序，以及前序 Attempt 必须为可重试拒绝或 `not_sent`；拒绝接受终态后又出现新 Attempt 的历史。
- 租约恢复评估采用相同历史约束；安全重试路径补充首次 Attempt 起算的 MaxElapsed 检查，避免恢复流程绕过执行预算。
- 新增历史不可能性回归测试；既有 MaxElapsed 执行 deadline 与恢复测试对应当前代码。


### 模板仓储装载一致性收口（2026-11-09）

- `Registry.LoadFromRepository` 按 `template_id + version` 精确读取，不回退 latest；校验记录身份、定义 JCS Digest 与仓储 Digest 一致后才注册。
- 装载前后检查 context；仓储缺失、取消、身份错配、Digest 错配均 fail closed。
- 重复装载相同版本/相同定义幂等；同一版本不同定义仍由 Registry 不可变保护拒绝。
- 新增空仓储、取消上下文、身份错配、重复装载及同版本冲突测试。


### 持久化渠道快照与运行时 Adapter 绑定校验（2026-11-09）

- 新增 `ChannelRegistry.RegisterSnapshot(expected, adapter)`，由 Server 在读取指定版本持久化快照并完成 Adapter 装配后调用。
- 注册前同时校验持久化快照与 Adapter 自报快照合法，并逐字段比较；任何 SecretRef、能力、渠道类型、启用状态或版本不一致均拒绝注册。
- 重试错误码集合按集合语义比较，顺序差异不构成配置冲突；注册表只保留经验证的不可变快照副本。
- 原 `Register(adapter)` 保留兼容，内部复用同一注册路径；新增匹配、错配及集合顺序测试。
- Adapter 构造和 Secret 解析仍由 Server 负责，Notification 不读取 Secret 明文，也不承担 composition root 职责。


### CI 阻断记录（Run `42612681306`）

- `Test go-core` 成功；根模块 `Test .` 在全仓 gofmt 门禁失败，日志列出的差异来自 Market Data、News、Quant 等其他目录，未列出 `internal/notification/**`。
- 因门禁失败，根模块测试、Race、vet 未执行；Notification 新增测试尚无 CI 实际执行通过证据。
- MariaDB 集成 Job skipped，未获得真实 DSN L3 证据。


### 租约恢复批次重复候选保护（2026-11-09）

- 恢复批次在执行任何状态迁移前，先检查 DeliveryID 非空且候选唯一；发现重复候选时整批 fail closed，不依赖扫描顺序决定先后赢家。
- 新增重复 Delivery 候选测试，确认 Repository 不会收到任何恢复写入。


### 模板启动批量装载原子性（2026-11-09）

- 新增 `Registry.LoadVersionsFromRepository`，按调用方明确提供的 `TemplateID + Version` 列表逐条精确读取，不查询或回退 latest。
- 所有记录先完成身份、Digest、定义校验和模板编译，再在 Registry 写锁内统一检查已有版本冲突；全部通过后一次性登记。
- 仓储缺失、Digest 错配、定义无效、重复请求键或与已装载定义冲突时，整批失败且不留下部分新增版本。
- 完全相同的已装载版本允许重复批量装载；新增测试覆盖中途损坏、成功装载、重复装载和已有版本冲突的原子性。


### 渠道配置精确版本加载边界（2026-11-09）

- 新增 `LoadChannelConfigSnapshot`，统一按 `channel_ref + config_version` 精确读取；缺失版本返回 `ErrChannelConfigNotFound`，仓储不可用、持久化记录损坏分别有独立错误分类。
- 加载层检查领域约束、请求身份与 SHA-256 Digest 格式，并复制可变切片；Digest 内容完整性由 Repository（MySQL 实现已校验 JCS 摘要）负责。
- `SecretRef` 的解析、密钥可用性检查及 Adapter 构造仍归 Server/Secret Provider；Notification 不解析密钥、不读取明文。SecretRef 无法解析时由 Server 映射为启动依赖故障，不伪装成配置记录损坏。
- 新增精确版本、缺失、摘要格式错误、身份错配、仓储错误及取消上下文测试。


### Attempt 历史校验复用（2026-11-09）

- 新增 `ValidateDeliveryAttemptHistory`，统一校验 Attempt 数量、连续序号、Delivery 身份、Attempt ID 唯一性、FencingToken 单调递增、时间顺序及前序结果可重试约束。
- 最后一条 Attempt 允许仍在运行，以支持租约恢复识别“渠道调用期间 Worker 退出”；任何前序 Attempt 必须完成且只能是可重试拒绝或已证明未发送。
- Delivery 执行前与租约恢复评估均调用该校验；各自仍保留当前租约 Worker/Token/ExpiresAt 的专项校验。
- 新增纯单元测试覆盖运行中末条记录、数量不符、终态后续 Attempt、重复 ID 与 fencing token 不递增。
- 本次仅增加代码和测试，尚未取得该提交的 CI 执行证据；真实 MariaDB L3 不受此单测替代。


### Delivery 与 Attempt 交叉一致性评估（2026-11-09）

- 新增只读 `AssessDeliveryAttemptConsistency`，将 Delivery 当前状态与完整 Attempt 历史进行保守交叉校验。
- `sent` 必须对应最后一次已完成的 accepted；`retry_wait` 必须对应最后一次可重试 rejected；`skipped` 有 Attempt 时必须对应 not_sent；`unknown` 允许 ambiguous 或仍运行中的 Attempt。
- `sending` 允许无 Attempt（StartWithLease 前退出）、运行中 Attempt，以及 Attempt 已完成但 Delivery CAS 尚未完成的两阶段写入中间态；后者明确返回 intermediate，不误报损坏。
- `queued` 保留人工重试和 not_sent 恢复语义；`blocked` 不凭 Attempt 快照单独推断矛盾。该评估不替代 StateEvent/Repository CAS 核验，也不执行修复写入。
- 新增纯单元测试覆盖 sending/unknown 合法中间态与 sent/运行中 Attempt 的明确矛盾。CI 尚未验证。


### Delivery StateEvent 历史连续性校验（2026-11-09）

- 新增只读 `ValidateDeliveryStateHistory`，核验事件数量与 Delivery.version 对应关系、版本连续性、DeliveryID、From/To 状态链、事件时间单调性，以及末事件与当前快照的状态/版本/更新时间一致性。
- 明确创建快照 `queued/version=1` 不产生初始 StateEvent；因此状态事件从 version=2 开始，不能错误要求存在 version=1 事件。
- MySQL Repository 新增完整状态事件读取接口，按 version 升序返回，不做静默截断；事件与快照仍由现有事务原子提交，不存在合法的两阶段中间态。
- 新增初始快照、完整链、缺失事件、版本跳号、状态链断裂、快照不一致和时间倒退测试。 同时按领域迁移图校验每条 From/To，拒绝终态逆转和非法跳转，并验证 unknown 离开时的 EvidenceRef、sending→skipped 的 not_sent 证据及 failed→queued 的人工重试审计字段。该校验为显式只读能力，不自动修复数据。


### Delivery 一致性统一只读诊断

- 新增 `AssessDeliveryConsistency`，聚合 Delivery 快照、Attempt 历史与 StateEvent 历史的独立校验结果，不访问数据库、不自动修复，也不引入 Server 依赖。
- 两类历史分别通过 `AttemptsComparable` / `StateEventsComparable` 显式声明是否已完整读取；读取失败、未读取或完整性无法证明时标记 `not_comparable`，不得把空切片冒充完整历史。
- 单项状态统一为 `pass / degrade / fail / not_comparable`。Attempt 已完成但 Delivery 仍为 sending 的合法竞态标记 `degrade`；明确矛盾或非法 StateEvent 链标记 `fail`。
- 聚合优先级为 `fail > not_comparable > degrade > pass`：任一独立检查已证明矛盾时不被其他数据源不可用掩盖；任一必需历史不可比较时不允许给出整体 pass。
- 该纯函数不保证三个输入来自同一数据库时点。调用方若分开读取，必须将读取失败/快照不可比标记为不可比较；本能力不声称提供原子快照读取。
- 已增加纯函数测试覆盖全量一致、中间态、明确矛盾、非法状态事件历史、历史不可读及冲突优先级。CI 与真实 MariaDB L3 仍需独立执行验收。


### 一致性诊断快照读取

- 新增 `DeliveryConsistencySnapshotReader` Port 与 `DeliveryConsistencyService`，Application 不再要求调用方分别拼装 Delivery/Attempt/StateEvent 三组数据。
- MySQL Adapter 在单个只读 `REPEATABLE READ` 事务内读取 Delivery 快照及两类完整历史；首个 Delivery SELECT 建立 InnoDB 一致性读视图，后续历史查询共享该视图，避免把读取期间发生的合法状态更新误判为历史矛盾。
- 三组读取任一失败时整体返回读取错误，不把部分结果交给诊断器；Delivery 不存在通过 `found=false` 与数据库错误区分。
- 该读取器仅保证单个 InnoDB 数据库事务的一致性快照，不涵盖外部渠道事实，也不替代跨库一致性协议。
- 已增加 Application Service 测试，覆盖成功诊断、读取错误透传和记录不存在。真实 MariaDB 快照隔离行为仍需 L3 DSN 测试执行确认。


### NTF-007 诊断结果安全 DTO 与证据摘要

- `DeliveryConsistencyResult` 是 Application 层可安全传输的诊断 DTO，仅输出聚合状态、分项状态、稳定 reason_code 和 allowlist 证据摘要。
- reason_code 固定为机器可读标识：`delivery_invalid`、`attempt_history_conflict`、`attempt_delivery_intermediate`、`state_event_history_conflict`；禁止拼接数据库错误、渠道响应或自由文本。
- 证据摘要仅包含 Delivery 状态/版本、预期与观察到的 Attempt 数量、StateEvent 数量、最近 Attempt 结果类别/完成标志、非空 evidence_ref 数量。
- 明确禁止输出 Delivery/Recipient/Channel/Subscription ID、EvidenceRef 原文、Payload、ProviderMessageRef、原始 Provider 错误、ActorRef、RequestID、Secret 或 SQL 错误。证据引用仅可在受权限保护的内部审计路径按需解析，不进入通用诊断响应。
- 本 DTO 不携带原始 Domain 对象，后续 Server HTTP 层仍须定义独立公开 DTO 并做权限检查；不得直接序列化 Domain 记录或 Platform Fault。
- 已补充安全摘要字段的 Application 单元测试；尚未进行 JSON/HTTP 泄漏扫描及根 Server 权限接入验收。

- 一致性诊断 Service 将无效查询参数（空/空白 DeliveryID、nil context）与快照读取器不可用分开返回：`ErrInvalidDeliveryConsistencyQuery` 与 `ErrDeliveryConsistencyReaderUnavailable`；数据库读取错误原样透传，记录不存在仍由 `found=false` 表达。


### Fanout 实现边界（2026-11-11）

- 实现提交：`f2dbe005b1c323a26f28b5128cfce3baa642d464`。
- 新增 `MySQLFanoutWorkItemRepository`，并作为 `InboxFanoutWriter` 注入 `MySQLInboxRepository`；Inbox 重放不新增工作项。
- 新增 Subscription revision 单行事务分配器；Create/AppendVersion 在同一事务递增修订号，Fanout 首次 Claim 固定高水位，页面查询先选快照内最新版本再过滤 active/event_type。
- 新增 `FanoutDispatcher.RunOnce`，每次最多处理一个 WorkItem 的一个页面；DeliveryPlan 继续复用现有 `DeliveryPlanWriter.CreateOrGet`，计划持久化失败不推进游标。
- 已编写 Fake/单元测试及 MariaDB 集成测试；未在本轮直接执行测试。必须先按分支规则合并到 integration 再执行 CI。
- `FanoutPlanFactory` 目前是明确的内部注入端口，具体匹配、授权策略、ChannelConfig/Template 版本解析及 RecipientRef 解析依赖真实业务数据；未伪造实现。
- Server 仍需装配 MySQLFanoutWorkItemRepository、Inbox Repository、Subscription PageReader、DeliveryPlanWriter、FanoutPlanFactory 和 Worker，并注册 Notification Migration。该项不在本分支范围。
