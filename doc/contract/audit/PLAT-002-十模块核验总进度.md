# PLAT-002 十模块核验总进度

**当前状态：** 第一阶段基线已建立；六维核验尚未完成。  
**代码基线：** `b6dfc85ba214fe16268c8cba780641a8bfe15086`（origin/integration）  
**文档维护分支：** feat/design_doc

## 1. 模块进度

状态说明：`not_started` 未开始；`in_progress` 正在核验；`partial` 已有局部证据但未完成；`verified` 六维核验完成且证据齐备；`blocked` 存在外部阻塞。当前不使用 verified 作为仅凭文档/目录存在即可达到的状态。

| 模块 | Port/调用点 | DTO/Schema/Golden | Fault/Diag/Request ID | 授权/Secret | Repo/Migration/Worker | 依赖边界 | 总状态 |
|---|---|---|---|---|---|---|---|
| Platform | partial（接口形态、Server 部分消费者已核；Data 受控出站接线及全量消费者待核） | partial | partial（Fault/Diagnostics/Request ID 基础实现与单测已核；消费者接入待核） | partial | partial | partial（发现 Task Queue 反向依赖旧 Analysis Domain，待裁决） | partial |
| Server | partial | not_started | partial | partial | partial | partial | partial |
| Data | partial | partial | partial | partial | partial | partial | partial |
| News | partial | partial | partial | partial | partial | partial | partial |
| Research | partial | partial | partial | partial | partial | partial | partial |
| Researchflow | partial | partial | partial | partial | partial | partial | partial |
| Quant | partial | partial | partial | partial | partial | partial | partial |
| LLM | partial | partial | partial | partial | partial | partial | partial |
| Notification | partial | partial | partial | partial | partial | partial | partial |
| WebUI | partial | partial | partial | partial | partial | partial | partial |

说明：partial 仅表示既有登记簿/起始快照中存在局部证据，不表示本轮已完成该维度复核。后续逐项更新并引用证据。

## 2. 当前执行状态

| 阶段 | 内容 | 状态 |
|---|---|---|
| A | 固定 SHA、模块清单、文件规模与并行修改观察 | in_progress（本次已建立初始基线，待完成差异复核） |
| B | 十模块六维接口级核验 | in_progress（Platform 维度 1 接口形态初审、维度 3 基础原语已核；消费者调用点及其余维度待核） |
| C | 跨模块调用图、授权链和依赖图 | not_started |
| D | 差异、待裁决、补证清单 | not_started |
| E | 责任模块修复/补证（本分支不改业务代码） | pending_audit |
| F | 第一轮完整性 Review | not_started |
| G | 第二轮正确性 Review | not_started |
| H | 第三轮冻结条件 Review | not_started |

## 3. 核验完成标准

每个模块六个维度必须分别记录：
- 生产方 Port/DTO/资源所有权；
- 每个直接消费者及真实调用点；
- 对应 Schema/Golden/Contract Test；
- 错误、诊断、Request ID 和授权传递；
- Repository/Migration/Worker 生命周期；
- 依赖方向与禁止依赖检查；
- 证据路径、符号或行范围、结论状态。

跨模块关系必须从生产方和消费者两端交叉验证。未找到消费者时，标记为“未发现消费者/待确认”，不得自动认定接口正确或无用。

## 4. 关键阻塞规则

- 未完成生产代码全量 import 扫描，不宣布依赖边界核验完成。
- 未完成核心 Port 的消费者追踪，不宣布接口核验完成。
- 未确认授权资源 Owner 和读取时复核，不宣布安全核验完成。
- 未核实 Worker 注册、启动、停止和故障退出，不宣布生命周期核验完成。
- P0 未解决或未裁决，不进入冻结建议。
- 三轮 Review 必须分别形成独立记录。


## 5. 本轮增量记录

### Platform — 维度 3：Fault、Diagnostics、Request ID

- 详细记录：[`Platform-维度3-Fault-Diagnostics-RequestID.md`](./Platform-维度3-Fault-Diagnostics-RequestID.md)
- 核验代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`
- 结论：Platform 这三类基础原语的实现与单元测试整体符合已审查的公共契约；本结论不代表各领域模块和 Server 已正确接入。
- 发现一项 P2 文档一致性问题：公共平台契约第 7.2 节对 Check 最小字段的描述，与第 21 节明确收敛后的实际 Go API 不一致，需由 Platform 文档责任方统一表述。
- Request ID 的旧兼容 Middleware 仍会信任单值入站 Header；新接入必须使用零值安全的 MiddlewareWithOptions。真实 Server 调用点尚未核验，列入后续 Server 维度 3。


### Platform — 维度 1：Application Port 与真实调用点（初审）

- 详细记录：[`Platform-维度1-Application-Port与调用点.md`](./Platform-维度1-Application-Port与调用点.md)
- 核验代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`
- 结论：Platform 是技术原语层，不应定义统一领域 Application Port；HTTP Client、Diagnostics 等能力通过标准 Go interface、配置、构造函数和回调供调用方组合。
- 发现 AUD-PLAT-PORT-001：`internal/platform/taskqueue` 直接依赖旧 `internal/analysis/domain`，违反 Platform 不依赖领域包的边界原则，登记为 P1 候选，等待旧 Analysis 迁移/Server 责任方裁决。
- 消费者调用点、Server 装配入口、Data 独立 Module 接入方式和全量 Platform import 扫描尚未完成；维度 1 保持 partial。


### Platform — 维度 1：第二批（Server 组合根与 Data Module）

- 详细记录：[`Platform-维度1-Application-Port与调用点.md`](./Platform-维度1-Application-Port与调用点.md)
- Server Bootstrap 已确认真实消费 Platform Config、Security、Storage、Diagnostics；Server Migration Registry 负责调度模块提供的 Migration，Diagnostics Service 组合模块 Check 并调用 Platform Runner。
- 固定基线 Server Bootstrap 仍以旧 Analysis HTTP/Worker 为中心；Migration 当前注册 News、Research、Notification。其他领域模块尚不能据此认定已接入 Server。
- Data 独立启动入口 `cmd/market-data/main.go` 仅注入 Store、Cache、Logger；Data Module 虽提供 HTTPDoer/Dialer 受控注入字段和完整性校验，但当前入口未注入这些依赖，也未启用 `RequireEgressDependencies`。
- 新增 AUD-PLAT-PORT-002：Data 独立入口未证明接入 Platform 受控出站执行器，列为 P1 候选待核；需先核对是否存在其他生产装配入口及 Provider 默认网络实现。
- Platform 维度 1 保持 partial；Request ID Middleware 和 Platform HTTP Client 的 Server 装配、其他模块消费者及全量 import 扫描仍待核。


### Platform — 维度 1：第三批（News 消费者）

- 详细记录：[`Platform-维度1-Application-Port与调用点.md`](./Platform-维度1-Application-Port与调用点.md)
- News Application 定义窄 `HTTPDoer` Port；Provider Adapter 使用 Platform `httpclient.Client.Bind` 构造 Provider 专属 `BoundDoer`，并将有效 Platform Fault 映射为 News `ProviderError`，对应测试覆盖目标 Host 拒绝、Fault 校验和 Cause 脱敏。
- News Diagnostics Check 使用 Platform Diagnostics/Fault，测试覆盖 skipped、degraded、unavailable 和脱敏。
- 固定基线 Server Bootstrap 只注册 News Migration；未构造 News Provider Set/Application，也未向 Server Diagnostics 注入 News Checks。新增 AUD-PLAT-PORT-003（P1 候选）：News Platform Adapter/Diagnostics 尚无 Server 生产装配证据。
- News Request ID 消费者仍待核。News 实现单测不能替代 Server 组合根接线证据。
- Platform 维度 1 保持 partial。

### Platform — 维度 1：第四批（Research 消费者）

- 详细记录：[`Platform-维度1-Application-Port与调用点.md`](./Platform-维度1-Application-Port与调用点.md)
- Research 自有 Application/Event/Claim Port，DTO 不暴露 Domain Entity；错误映射 Platform Fault。
- Research Diagnostics 构造 Platform Check，由调用方 Runner 执行；测试覆盖快照版本、质量状态、Run cutoff 和敏感内容隔离。
- Research 定义独立 Correlation ID，与 Platform Request ID 明确区分；Server 端可信上下文映射和异步端到端传播仍待核。
- Server 已注册 Research Migration，但未构造 Research Application/Repository/Authorizer，也未注入 Research Diagnostics Checks。新增 AUD-PLAT-PORT-004（P1 候选）。
- 新增 AUD-PLAT-PORT-005（P2 候选）：Research Correlation ID 与 Platform Request ID 端到端关联待证。
- Research 消费者核验为 module-level implementation evidenced / production wiring not evidenced；Platform 维度 1 保持 partial。


### Platform — 维度 1：第五批（Notification 消费者初审）

- 详细记录：[`Platform-维度1-Application-Port与调用点.md`](./Platform-维度1-Application-Port与调用点.md)
- Notification 自有 Application Port 覆盖 Subscription、Delivery Plan、Delivery、Attempt、租约恢复、Inbox、模板和渠道适配；Inbox 事务要求同时写入 Fanout 工作项。
- DeliveryExecutionService 将扫描/Claim 留给上层 Worker；LeaseRecoveryService 仅提供调用方驱动的单次恢复入口。
- Migration 已由 Server 注册；固定基线 Server Bootstrap 没有构造 Notification Application/Repository/FanoutWriter，也没有注册 Notification Worker；Server Diagnostics 未注入 Notification Checks。
- Migration 持久化 Inbox `correlation_id` 和 Delivery State Event `request_id`，但端到端映射未证。
- 新增 AUD-PLAT-PORT-006（P1 候选）：Notification Application/Fanout/Delivery Worker 未接入 Server 生产组合根。
- 新增 AUD-PLAT-PORT-007（P2 候选）：Notification Request ID 与 Correlation ID 端到端映射待证。
- Notification Fault 映射及 Application 全目录错误路径尚未完成扫描；本批结论为 module-level implementation evidenced / production wiring not evidenced。维度 1 保持 partial。


### Platform — 维度 1：第六批（Quant 消费者初审）

- 详细记录：[`Platform-维度1-Application-Port与调用点.md`](./Platform-维度1-Application-Port与调用点.md)
- Quant 当前是纯计算/结果契约组件；目录包括 domain、factor、indicator、manifest、metric、registry、result、risk、trend，未见 Application/Adapter/Storage/Diagnostics。
- Manifest/Result Save Port、MemoryRepository 和 Risk RuleSet fail-closed 执行骨架已有实现；MemoryRepository 明确不替代生产 Storage。
- BuildRef 只构造引用，不签发/验证 AuthorizationRef；当前没有结果读取接口。
- Server 未构造 Quant 服务、未注册 Quant Migration、未接入 Quant Diagnostics；Quant 任务规划明确生产 Storage、可信规则集发布和事实接入尚未完成。
- 新增 AUD-PLAT-PORT-008（P1 候选，阶段性阻断）：Quant 生产 Application/Storage/Diagnostics 接入尚未形成；这是后续生产集成门禁，不直接定性为纯计算 Domain 缺陷。
- 新增 AUD-PLAT-PORT-009（P1 候选，安全边界）：Quant AuthorizationRef 生产签发与验证责任待证；未发现读取接口，当前不认定存在授权绕过路径。
- Quant 维度 1 状态 partial；domain/contract implementation evidenced / production application wiring not yet implemented by current stage。


### Platform — 维度 1：第七批（LLM 消费者初审）

- 详细记录：[`Platform-维度1-Application-Port与调用点.md`](./Platform-维度1-Application-Port与调用点.md)
- LLM Responses JSON/SSE Adapter 定义窄 HTTP Port；Server llmtransport 将共享 Platform Client 绑定单一 Gateway Host/Port，要求 HTTPS、拒绝重定向、关闭传输层重试。
- ProviderConfig 只存 APIKeySecretRef，Adapter 显式注入 platform/config.Secret；Secret 格式化/序列化脱敏；但生产 SecretRef 解析与绑定调用点未找到。
- CapabilityState 绑定 Provider/Model/Gateway 版本及有效期/实测证据；未知、过期能力拒绝调用。
- Platform Fault 映射与 Dependency/Cause 脱敏有测试证据。
- Server Bootstrap 未构造 LLM Adapter、未注入 Provider/Secret/CapabilityState，也未接入 LLM Diagnostics；LLM 当前没有独立 Worker 生命周期要求。
- 新增 AUD-PLAT-PORT-010（P1 候选）：LLM Gateway Adapter 尚未进入生产组合根。
- 新增 AUD-PLAT-PORT-011（P2 候选）：LLM DTO RequestID 与可信 Context Request ID 的入口映射待证。
- LLM 维度 1 状态 partial；adapter/transport implementation evidenced / production composition wiring not evidenced。


### Platform — 维度 1：第八批（Researchflow 消费者初审）

- 详细记录：[`Platform-维度1-Application-Port与调用点.md`](./Platform-维度1-Application-Port与调用点.md)
- Researchflow 自有 Application Port 覆盖 Run、Stage、Worker、FaultNormalizer、Outbox、RequestIdentity 等；StageExecutor/OutboxPublisher 由 Server 组合根注入。
- ContextAssembly 映射 Research Application DTO；ToolPolicy 使用 LLM Schema/Tool Call DTO，但模型 Tool Call 仍必须经过 Registry.Authorize；ResultValidation 使用 Research/LLM DTO 和本地 Quant Result Envelope，LLM Explanation 明确 non_authoritative，硬风险 block/unknown 不可被模型覆盖。
- MySQL Storage、Migration、Worker Lease/Fencing、Deadline、RetryPolicy、Outbox at-least-once 和 Platform Diagnostics Observation Adapter 已有模块内实现。
- Run 保存 originating_request_id 与 correlation_id；但 StageExecutionRequest/Outbox 只携带 correlation_id，异步 Request ID 传播待证。
- Server Bootstrap 未注册 Researchflow Migration，未构造 Application/StageExecutor/Worker/DeadlineScanner/OutboxDispatcher，未接入 Diagnostics 或 Lifecycle。
- 新增 AUD-PLAT-PORT-012（P1 候选）：Researchflow Application、Storage 与异步运行组件尚未进入 Server 生产组合根。
- 新增 AUD-PLAT-PORT-013（P2 候选）：Researchflow originating Request ID 未证明贯穿 Worker 与 Outbox。
- Researchflow 维度 1 状态 partial；application/storage execution skeleton evidenced / production composition and cross-module execution wiring not evidenced。


### Platform — 维度 1：第九批（Server Root Composition 收尾核验）

- 详细记录：[`Platform-维度1-Application-Port与调用点.md`](./Platform-维度1-Application-Port与调用点.md)
- Server 当前仍是 transitional legacy Analysis runtime：Bootstrap 构造旧 Analysis Runner/TaskQueue，HTTP Router 仅暴露旧 Analysis API 与 Health/Readiness/Diagnostics。
- Migration Registry 只注册 News、Research、Notification；单测固定断言这三个 namespace。
- Lifecycle 只注册旧 Analysis Worker 和可选 HTTP Server；通用 Lifecycle 启停/回滚测试通过，但新模块组件尚未注册。
- Resources 已持有共享 Platform SQL/Redis/HTTP Client，并提供 BindHTTPDestination；Bootstrap 尚未把共享 BoundDoer 注入新模块 Adapter。
- HTTP Request ID Middleware 默认重生成不可信入站 ID，有对应测试；Diagnostics 仅执行共享 MariaDB/Redis/Schema Checks，Bootstrap 未传入模块 Checks。
- Service Credential/Scope 仍明确暂缓；当前 Owner-only 旧 Analysis 路由符合现阶段配置注释，不单独记为安全缺陷。
- Platform TaskQueue 仍直接依赖旧 Analysis Domain，与 AUD-PLAT-PORT-001 相互印证。
- 新增 AUD-PLAT-PORT-014（P1 候选，项目级集成阻断）：Root Server 尚未完成从旧 Analysis 到模块化 Application Composition 的迁移；该项汇总关联各模块生产接线缺口，不替代模块责任项。
- Server 维度 1 状态 partial；Root Server infrastructure evidenced / new-module production composition not evidenced。
- Platform 维度 1 仍未全量完成；WebUI 和其他未完成消费者扫描及跨模块调用图仍待执行。


### WebUI — 维度 1：对外契约消费核验

- 详细记录：[`Platform-维度1-Application-Port与调用点.md`](./Platform-维度1-Application-Port与调用点.md)
- 浏览器 API Client 使用同源 `/market-api` 与 `/analysis-api`；未发现直接访问 Provider/DB/Redis 的证据。
- Nginx 当前分别代理至独立 Market Data:8091 与 Analysis Server:8092，仍是双后端兼容拓扑，不是目标 Root Server 单一模块化 API 装配。
- Nginx 覆盖浏览器传入的 `X-Request-ID`，使用自身 Request ID；Web API Client 在失败时读取错误 envelope/header Request ID。
- 行情 API Key 当前保存在浏览器 localStorage 并注入 `X-API-Key`；WebUI API_CONTRACT 已明确此为迁移期兼容状态，目标禁止浏览器持有。新增 AUD-WEBUI-CTR-001（P1 候选，迁移安全门禁）。
- Analysis API Client 不发送 Owner Bearer/Session/CSRF；Server 当前旧 Analysis 路由 Owner-only。WebUI API_CONTRACT 明确浏览器请求预期 401。新增 AUD-WEBUI-CTR-002（P1，明确联调阻断）；禁止将 Owner Token 放进浏览器或匿名开放。
- API 单测和 Deployment Contract 测试均不能替代真实 Server 鉴权与浏览器联调。
- WebUI 维度 1 partial；same-origin API client and reverse-proxy implementation evidenced / target Root Server session-auth integration not evidenced。
- 本批未完成逐页面 DTO/OpenAPI/Handler/Golden 映射，后续归维度 2 和 API-01 矩阵核验。


### WebUI — 维度 2：Analysis Run DTO/OpenAPI 映射初审

- 固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- 已核对 `web/src/api.js`、`web/src/task-workflow.js`、`web/tests/api.test.mjs`、`doc/api/contracts/分析任务.openapi.yaml` 和 Root Server Router。
- WebUI `getAnalysisRun` 与 Root Server 均使用 `GET /api/v1/analysis/runs/{run_id}`，但 OpenAPI 未声明该路径和 Run 响应 Schema。
- 新增 **AUD-WEBUI-CTR-003**（P2，契约文档缺口）：补齐 Run 查询 DTO、状态码和 Golden/契约测试后关闭；不据此认定运行时路由缺失。
- 本批只完成 Analysis Run 单路径映射；AnalysisRequest/Task/TaskEvent 逐字段核验及 Market Data 全量 DTO 映射待后续批次。WebUI 维度 2 保持 partial。
- 详细证据见 `Platform-维度1-Application-Port与调用点.md` 第 16 节。


### WebUI — 维度 2：全市场分页股票 DTO 映射初审

- 已核对 WebUI API Client/页面、Market Data Handler、Go Model、OpenAPI Schema 与 Go OpenAPI Contract Test。
- `page/page_size/include_ma/adjustment` 请求参数与 Handler/OpenAPI 默认值和边界一致；Go 测试反射校验 51 个 Model 的字段、required 和 pointer-nullable 语义。
- 页面正确区分当前页 coverage、目录 total 下界、null 缺失、质量状态和旧快照；未发现已证实字段语义错配。
- 新增 **AUD-WEBUI-CTR-004**（P2，测试补证）：前端缺少对全市场分页 API 参数序列化/默认值/边界归一化的直接断言；当前不是已证实运行时错误。
- WebUI 维度 2 仍为 partial；Analysis Run 与全市场分页两个切片已核，其他接口和 DTO 仍待核。详细证据见审计明细第 17 节。


### WebUI — 维度 2：Analysis Task 创建请求 Schema 初审

- WebUI 当前创建请求发送 `task_type + symbols`，可选传递 Idempotency-Key；阶段事件消费者验证 task_id、events、分页游标和可选 run_id 身份。
- OpenAPI 声称 AnalysisRequest 与 Domain RunRequest 一致，但缺少 `as_of_mode`、`parameters`、TaskType 枚举和 required 约束。
- 新增 **AUD-WEBUI-CTR-005**（P2）：先明确 HTTP Request DTO 与 Domain 类型的映射边界，再补齐 Schema 或修正其描述，并增加契约测试。当前 WebUI 最小请求不受该差异直接影响。
- Task/TaskEvent 响应 DTO 逐字段核验及服务端 JSON 解码策略仍待检查；维度 2 保持 partial。详见审计明细第 18 节。


### WebUI — 维度 2：Task / TaskEvent 响应 DTO 初审

- Task 字段集合、required 集合与 Go Task 基本对齐；Task 状态枚举与 Worker 状态映射一致，并有状态映射单测。
- 新增 **AUD-WEBUI-CTR-006**（P2）：StartedAt/FinishedAt 在 Go 中为 `omitempty` 指针，空值会省略；OpenAPI 却声明 `nullable: true`，需要统一缺失/null 语义。
- 新增 **AUD-WEBUI-CTR-007**（P2）：Go Event 嵌入 StageEvent，但 OpenAPI event 仅是任意 object，缺少字段、状态枚举、时间格式和可选字段 Schema。
- WebUI 已验证 Task ID、事件序号、分页游标和可选 run_id；服务端 next_after_sequence 计算、JSON Golden 与未知字段解码策略仍待核。
- 维度 2 保持 partial。详细证据见审计明细第 19 节。


### WebUI — 维度 2：TaskEvent 服务端分页核验

- Handler 使用排他游标：返回本页最后一条 sequence；空页时保持请求游标。Queue 按事件数据库 ID 升序读取 ID 大于游标的记录。
- 单测目前只覆盖单页增量读取，未覆盖空页、连续分页、limit 和非法游标边界。
- 新增 **AUD-WEBUI-CTR-008**（P2，测试补证）：补齐分页边界测试，并用 Migration/DDL 证据确认事件 ID 的唯一、单调和不可变语义。
- WebUI 读取 event.error_code，但当前 Go StageEvent 和 HTTP Handler 未提供该字段；暂不认定为运行时错误，待 DTO 收敛时裁决是否删除或正式支持。
- 维度 2 仍为 partial。详见审计明细第 20 节。


### WebUI — 维度 2：日 K 响应消费与 Go/OpenAPI 空值语义

- 固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- 已核对 WebUI API Client、行情查询流程、图表消费、Go DailyBar、HTTP Handler、OpenAPI 和契约测试。
- 日 K 请求参数及普通/PIT 两种响应分支与 Schema 基本一致；DailyBar 的字段集合、required、非 omitempty pointer-nullable 和 JSON 实际序列化形态由 Go 契约测试覆盖。
- WebUI 保留缺失值，不补零；图表校验日期、OHLC 和成交量；技术指标由前端基于收盘价计算，不消费服务端 MA 字段。
- 默认 limit=120 不足以计算 250 个有效收盘样本的 MA250，但这是窗口/指标可用性问题，不是 DTO 契约错配；本批未新增差异编号。
- 本批未发现已证实的日 K 字段语义错配。Bundle、证券详情、指数日 K 等接口仍待逐项核验；WebUI 维度 2 保持 partial。详细证据见审计明细第 21 节。


### WebUI — 维度 2：Bundle 聚合响应 DTO 与快照元数据

- 已核对 Bundle Handler、Service、Go Model、OpenAPI、模型反射/JSON 序列化契约测试及 WebUI 真实消费调用点。
- Bundle 字段集合、required/omitempty 与 OpenAPI 一致；Quote 可选时按字段缺失表达，WebUI 对顶层和嵌套 Quote 的证券身份分别校验。
- WebUI 正确处理 Quote 缺失、质量降级、warnings、daily_bars、快照 ID/参考时间和来源；仅在 Bundle 返回 404 时启用旧服务兼容回退。
- 新增 AUD-WEBUI-CTR-009（P2）：lookback_days 省略时由 Service 注入的 defaultBars 决定，OpenAPI 未说明省略语义；当前 WebUI 显式传参，不受影响。
- WebUI 维度 2 仍为 partial；后续继续核验证券详情及其他行情 API。详细证据见审计明细第 22 节。


### WebUI — 维度 2：证券资料、历史交易状态与涨跌幅规则

- 已核对 Profile、Trading Status、Price Limit Rule 的路由、Handler、Go DTO、OpenAPI 和相关 WebUI API Client。
- 新增 AUD-WEBUI-CTR-010（P2）：若干 Go `omitempty` 字段在 OpenAPI 直接声明 nullable=true，但实际 JSON 会省略字段；复核后不把仅在 $ref 旁声明 nullable 的 special_exception_evidence 计入已证实范围。第 25 节将 FundamentalsSnapshot.system_observed_at 纳入已证实范围。
- 新增 AUD-WEBUI-CTR-011（P2）：PriceLimitRuleSnapshot 的 OpenAPI required 漏列八个 Go 非 omitempty 字段，且两个状态字段缺少 Go 已定义枚举；该 DTO 未进入 51 模型契约登记表，缺少完整 JSON Golden。
- Review 已确认 stock-research.js 只实际调用 Profile；Trading Status / Price Limit Rule 请求函数未接入该页面，现有对应区域仅重置占位状态。不能视为端到端已完成。
- 本批未证明运行时业务结果错误；WebUI 维度 2 继续 partial。详细证据见审计明细第 23 节。


### WebUI — 维度 2：个股所属板块 StockBoards

- 固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- 已核对 Provider Port、Eastmoney 实现、DataService 校验/回退/缓存、HTTP 路由与 Handler、Go DTO、OpenAPI、契约测试及 WebUI API Client/页面调用链。
- StockBoard 与 StockBoardSnapshot 的 Go 字段集合、required 集合和 OpenAPI 一致；契约反射测试已登记两种模型。页面实际调用 `getStockBoards`，并校验响应证券身份。
- 将 StockBoard.change_percent 纳入 AUD-WEBUI-CTR-010：Go `omitempty` 指针空值会省略，OpenAPI 却声明 nullable=true。
- 新增 **AUD-WEBUI-CTR-012**（P2）：Provider 已提供 `total_is_lower_bound`，但 WebUI 未消费该字段；源端 1000 条上限时，页面可能将下界呈现为确定总数。
- Provider 1000 条上限、unknown 空列表、后备源回退及缓存策略均有对应代码/测试证据；本批未证明板块成员运行时数据错误。
- WebUI 维度 2 继续 partial。详细证据见审计明细第 24 节。


### WebUI — 维度 2：Fundamentals 财务指标与 PIT 消费

- 固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- 已核对 Go DTO、BaoStock/HiThink Provider、DataService 校验/回退/缓存、MariaDB PIT 版本存储、HTTP Handler、OpenAPI、Provider/Service/HTTP/Repository 测试及 WebUI API Client/页面消费。
- 时间语义区分明确：report_date=报告期末、publication_date=披露日期、available_at=精确可用时间、fetched_at=服务采集时间、system_observed_at=系统记录版本时间。未发现把报告期末或采集时间冒充披露时间。
- 缺失指标不补零；缺失分区显式 unknown；PIT 只读本地版本，日期级披露证据采用保守过滤，精确 available_at 可支持同日时刻判断。
- FundamentalMetric、FundamentalSection、FundamentalsSnapshot 字段及 required 与 OpenAPI 一致；将 FundamentalsSnapshot.system_observed_at 纳入 AUD-WEBUI-CTR-010（omitempty 指针被 OpenAPI 过度声明 nullable）。
- 新增 **AUD-WEBUI-CTR-013**（P2，待产品范围确认）：服务端支持 as_of PIT 查询，但 WebUI API Client/页面未暴露该参数或历史时点查询动作。
- 页面已消费分区质量、报告期、披露日期、available_at、指标、单位、质量原因及 system_observed_at；未发现缺失值补零。
- 未发现已证实运行时财务数值错误；WebUI 维度 2 继续 partial。详细证据见审计明细第 25 节。


### WebUI — 维度 2：Announcements 公告 DTO、日期范围与分页消费

- 已核对 StockAnnouncement/StockAnnouncementSnapshot Go DTO、Eastmoney Provider、Service 日期范围与分页校验、HTTP Handler、OpenAPI、模型反射/序列化测试及 WebUI API Client/页面/浏览器测试。
- 公告日期、源端发布时间、服务采集时间严格区分；源端时间不可解析时不伪造时间戳，条目降级并写字段诊断；空列表保持 unknown。
- Provider 与 Service 校验页码、页大小、总页数、返回条数、has_more、公告日期范围和 article_id 页内唯一性；Provider 在源端返回 codes[] 时校验请求证券代码，空 codes[] 时依赖 stock_list 查询过滤；当前 API 为页码分页，不提供稳定游标/快照令牌。
- 将 StockAnnouncement.published_at 纳入 AUD-WEBUI-CTR-010：Go omitempty 指针 nil 时省略，OpenAPI 却声明 nullable=true。
- 新增 **AUD-WEBUI-CTR-014**（P2）：WebUI 首次及后续公告分页请求均未复用首屏响应解析出的 start_date/end_date；跨中国自然日边界可能把不同查询窗口的页面合并，且没有跨页 article_id 去重/稳定集合保证。
- WebUI 已接通公告 API，处理空状态、质量、来源时间、安全链接和刷新时旧分页响应失效；但跨页查询范围与集合一致性仍待修复。WebUI 维度 2 继续 partial。详细证据见审计明细第 26 节。


### WebUI — 维度 2：Market Valuations 估值字段、时间与消费语义

- 固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- 已核对同花顺 Provider、Valuations Service、字段校验、HTTP Handler、Go DTO、OpenAPI、模型反射测试及 WebUI API Client/个股页面/浏览器测试。
- PE/PB/PS/PCF 倍数允许负数；数值 0、null、无效数值、未返回证券分别处理，不补零。核心五项指标缺失/非法会降级并写字段诊断；missing_symbols 与返回项互斥且覆盖请求。
- provider_timestamp、provider_report_date、fetched_at 语义区分；同花顺未提供独立报告日期，因此即使有供应商时间戳，快照仍为 degraded。供应商时间不代表每个指标均在该时刻更新。
- ValuationItem/ValuationSnapshot 字段和 required 集合与 OpenAPI 一致；新增 **AUD-WEBUI-CTR-015**（P2）：五个 omitempty 扩展指标被 OpenAPI 过度声明 nullable，归入 CTR-010 同类问题。
- WebUI 已接通估值接口，显示五项核心倍数、缺失状态、质量原因及供应商/采集时间；未发现已证实的运行时估值数值错误。详细证据见审计明细第 27 节。
- WebUI 维度 2 继续 partial。


### WebUI — 维度 2：Chip Distribution 筹码分布字段、算法与消费语义

- 固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- 已核对 ChipDistribution DTO、独立 ChipDistributionProvider Port、Eastmoney/MiaoXiang Provider、Service 校验/回退/缓存、HTTP Handler、OpenAPI、模型契约测试及 WebUI 页面/浏览器测试。
- 已有统一 Provider 协议，不需要重复抽象；比例统一为 0..1，成本为人民币元/股；Service 校验正成本、区间顺序、比例边界、日期与采集时间。两 Provider 均因缺少可验证发布时间/可用时间而不能标记 valid。
- AUD-WEBUI-CTR-010 影响范围增加 ChipDistribution 的九个 omitempty 字段，OpenAPI 过度声明 nullable。
- 新增 **AUD-WEBUI-CTR-016**（P2）：Eastmoney 计算字段与 MiaoXiang 源端字段的 average_cost/集中度口径没有等价性证据；provider_date 实际已归一化，但 OpenAPI 描述为原始日期。
- 新增 **AUD-WEBUI-CTR-017**（P2）：Eastmoney 使用 fqt=0 未复权日 K 计算最近最多 120 根筹码分布，除权窗口可能混用价格基准；没有算法专用测试或参考 Golden。
- WebUI 已接通并正确显示比例、成本、日期、质量状态；has_data 分支不属于当前 Go DTO/OpenAPI 契约。未证实线上数值错误，准确性风险待责任方处理。详细证据见审计明细第 28 节。
- WebUI 维度 2 继续 partial。


### WebUI — 维度 2：个股 Capital Flow 主力资金流字段、窗口与消费语义

- 固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`。本节仅审查个股 `/api/v1/stocks/{symbol}/capital-flow`，市场级资金流历史是独立接口。
- 已核对 CapitalFlow DTO、独立 Provider Port、Eastmoney/MiaoXiang Provider、Service 校验/回退/缓存、HTTP Handler、OpenAPI、模型反射测试和 WebUI 页面/浏览器测试。
- 两个 Provider 均按日期降序取最新观测；金额统一人民币元，保留正负方向；缺失金额不补零，5/10 日合计要求对应观测金额完整。Service 统一验证来源版本、证券代码、日期、采集时间、观测数量和有限数值。
- 新增 **AUD-WEBUI-CTR-018**（P2）：5/10 日汇总只按最近 N 条记录计算，未用交易日历证明窗口无漏日；诊断文案要求连续观测但实现未检查交易日完整性。
- 新增 **AUD-WEBUI-CTR-019**（P2）：三个 omitempty 金额指针字段被 OpenAPI 过度声明 nullable。
- WebUI 已接通并正确显示元单位、负数、缺失指标、观测数量及质量状态；has_data 分支不是 Go DTO/OpenAPI 字段。未发现已证实的金额符号或元单位换算错误。
- 跨 Provider 资金分类口径没有同标的同日期对照证据；源端发布时间不可验证，因此快照保持 degraded/unknown。详细证据见审计明细第 29 节。
- WebUI 维度 2 继续 partial。


### WebUI — 维度 2：Market Capital Flow 市场级资金流历史

- 固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`；本节与第 29 节个股 Capital Flow 分开。
- 已核对独立 MarketCapitalFlowItem/Snapshot DTO、MarketCapitalFlowProvider、Tushare `moneyflow_mkt_dc`、Service 校验/缓存、HTTP Handler、OpenAPI、契约测试及 WebUI API Client/市场概览消费。
- 当前固定基线只有 Tushare 市场级 Provider；官方接口文档明确市场级金额字段单位为人民币元、比例为百分数值，当前 Provider 直接映射金额、不乘万元换算系数，正负号保留。
- `net_amount` 与超大单/大单/中单/小单金额之间的代数关系未被官方字段文档明确，代码和测试也没有建立恒等约束；新增 **AUD-WEBUI-CTR-021**（P2，来源定义补证），禁止消费者无依据地重算主力净额。
- 日期请求为闭区间、最多 2000 个自然日；Provider/Service 校验日期格式、范围、重复和升序，但未按交易日历核对缺日；新增 **AUD-WEBUI-CTR-022**（P2，覆盖完整性）。
- 新增 **AUD-WEBUI-CTR-020**（P2）：MarketCapitalFlowItem 的 14 个 Go omitempty 数值字段被 OpenAPI 声明 nullable=true，实际 nil 时 JSON 省略。
- WebUI API Client 已接通且有 URL/鉴权/取消/日期范围测试；页面只展示指数和主力净额，不消费四类分类金额/比例，页面渲染缺少直接回归测试。四类字段是否必须展示待产品范围确认。
- 未发现已证实的金额单位换算或符号错误；数据源时间不可验证，快照保持 degraded。详细证据见审计明细第 30 节。
- WebUI 维度 2 继续 partial。


### WebUI — 维度 2：Market Session 市场交易时段

- 固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`。已核对 MarketSession DTO、TradingCalendar Port/DTO、BaoStock 日历 Provider、Service 阶段计算与日历完整性校验、HTTP Handler、OpenAPI、模型契约测试、配置接线及 WebUI API Client/市场活动页。
- 市场时段使用完整自然日交易日历判定交易日，再按 Asia/Shanghai 固定时间表计算；不是按 weekday 推断。BaoStock 对每个自然日逐行校验，缺日/乱序拒绝；Service 再次校验完整日期覆盖、交易日计数和来源元数据。
- 固定基线只有 BaoStock 实现 TradingCalendarProvider，默认启用该能力；源端快照时间不可验证，故质量保持 degraded。当前没有第二个已证实的日历 Provider 用于故障回退或交叉校验。
- 已核对阶段：09:15–09:25 开盘集合竞价、09:25–09:30 竞价暂停、09:30–11:30 上午连续竞价、11:30–13:00 午休、13:00–14:57 下午连续竞价、14:57–15:00 收盘集合竞价。测试覆盖主要边界；HTTP 测试覆盖 +08:00 输入。
- MarketSession 只描述市场计划时段，不代表个股停牌、临时停市或订单可撤销/可成交状态；开盘集合竞价没有细分 09:20 前后规则。
- 新增 **AUD-WEBUI-CTR-023**（P2）：API Client 有 getMarketSession，但市场活动页面只消费交易日历，没有展示 MarketSession。
- 新增 **AUD-WEBUI-CTR-024**（P2）：集合竞价是粗粒度阶段，不足以作为订单行为判定；当前无下单功能，先明确契约边界，不提前扩展。
- OpenAPI required 与 DTO 非 omitempty 字段对齐；API Client 默认路径和鉴权有测试，但显式 at 编码/AbortSignal 透传未单独覆盖。
- 未发现已证实的阶段边界计算错误。详细证据见审计明细第 31 节；WebUI 维度 2 继续 partial。


### WebUI — 维度 2：Market Breadth 市场涨跌家数与市场广度

- 固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`；详细证据见审计明细第 32 节。
- 已核对 MarketBreadthProvider Port、Eastmoney/TickFlow Provider、默认能力配置、DataService 分类/覆盖校验与回退、MarketSentiment 聚合、HTTP Handler、OpenAPI、模型反射契约测试及 WebUI API Client/市场概览页面。
- 默认启用 Eastmoney 市场广度；TickFlow 实现同一 Port 但默认关闭。Eastmoney 将源端涨跌分布桶计数汇总为上涨/下跌/平盘，未提供逐证券停牌或未返回分类；总数与覆盖数相等只代表分布桶内部总和，不是独立证券主数据覆盖证明。
- Service 强制四类计数合计等于 covered，并要求业务快照 covered==total；TickFlow 部分覆盖只可进入 Probe 的宽松校验，不能作为业务回退候选。
- Eastmoney 缺少可验证盘中时间；成交额、涨跌停池分别请求，非原子快照。MarketSentiment 对辅助池报告日期为空时仍可能附加计数，且跨来源只保留来源标记，没有同一时刻/总体证明。
- 新增 **AUD-WEBUI-CTR-025**（P2）：市场广度总体分母无法证明覆盖停牌及未返回证券，且 Provider 间总体口径未证明等价。
- 新增 **AUD-WEBUI-CTR-026**（P2）：市场情绪辅助池缺少完整日期证据时仍可能附加计数。
- AUD-WEBUI-CTR-010 影响范围增加 MarketBreadthSnapshot.oldest_quote_time/newest_quote_time：Go omitempty 指针 nil 时省略，OpenAPI 却声明 nullable=true。
- WebUI 已消费并展示四类家数和覆盖元数据；API Client 无直接 getMarketBreadth 请求测试，浏览器测试未直接覆盖 renderBreadth。未发现已证实的涨跌分类算术错误；WebUI 维度 2 继续 partial。


### WebUI — 维度 2：Market Indices 核心指数行情

- 固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`；详细证据见审计明细第 33 节。
- 已核对 IndexQuoteProvider Port、Eastmoney/同花顺/TickFlow 三个实现、配置与装配、DataService 校验/回退/15 秒缓存、HTTP Handler、OpenAPI 及 WebUI 消费。
- 三个 Provider 使用相同七个核心指数白名单。规范涨跌额为最新价减昨收价，涨跌幅为该差值除以昨收价乘 100；同花顺和 TickFlow 以计算值为准并交叉校验源端扩展值，Eastmoney 对源端值做一致性校验。未发现已证实的公式或符号错误。
- 固定默认配置仅 Eastmoney 启用 index_quotes；同花顺和 TickFlow 实现了 Port，但对应能力默认关闭。Service 允许返回部分指数集合并标记 degraded，不强制七项完整覆盖。
- 同花顺使用响应级 timestamp；Eastmoney/TickFlow 逐指数记录时间；TickFlow 七个指数分两批请求。Service 仅拒绝明显未来时间，不校验最大历史年龄或跨指数时间跨度。新增 **AUD-WEBUI-CTR-027**（P2）：核心指数快照缺少统一时间跨度与陈旧度质量约束。
- AUD-WEBUI-CTR-010 影响范围增加 MarketIndexQuote 的 latest_price、change、change_percent、open、high、low、prev_close、amount、quote_time：Go omitempty 字段 nil 时省略，OpenAPI 却声明 nullable=true。
- WebUI 已接通并逐指数显示最新价、涨跌额、涨跌幅、质量和行情时间；API Client 缺少 getMarketIndices 直接请求测试，浏览器测试缺少 renderIndices DOM 回归。
- 未发现已证实的指数涨跌公式错误；时间一致性、陈旧度和页面回归覆盖仍待补强。WebUI 维度 2 继续 partial。


### WebUI — 维度 2：Index Catalog 指数目录

- 固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`；详细证据见审计明细第 34 节。
- 已核对 IndexCatalogItem/Snapshot DTO、IndexCatalogProvider Port、Hithink 目录实现、Service 规范化/去重/回退/缓存、配置装配、HTTP Handler、OpenAPI 和 WebUI API Client。
- 服务端校验六位数字代码及允许后缀、非空名称、tag 一致性、唯一 symbol、来源计数和空目录质量；Provider 对重复代码去重并降级，触及 30000 项安全边界时标记 lower-bound。
- 固定配置未启用任何 IndexCatalog Provider；Hithink/Tushare 均 disabled 且 index_catalog=false。当前只确认 Hithink 有目录实现，且仅支持 cn_concept/region/tszs/industry；Tushare 标签虽被 API/Client/OpenAPI 接受，但没有对应 FetchIndexCatalog 实现。
- 新增 **AUD-WEBUI-CTR-028**（P2）：Tushare 目录标签与真实 Provider 能力不一致；新增 **AUD-WEBUI-CTR-029**（P2）：目录顺序没有稳定契约及回归测试；新增 **AUD-WEBUI-CTR-030**（P2）：WebUI 只有 API Client 函数，没有页面消费者及请求/DOM 测试。
- provider_time 仅是 Hithink 响应级 timestamp，缺少独立目录发布日期/生效日；source_version 是适配器版本而非数据集修订号。provider_time 纳入 **AUD-WEBUI-CTR-010**（omitempty 指针被 OpenAPI 声明 nullable）。
- 未发现默认运行中目录数据的运行时错误；默认目录能力未启用，当前结论是契约可调用性和端到端接入缺口。WebUI 维度 2 继续 partial。


### WebUI — 维度 2：Index Constituents 指数成分与历史权重报告

- 固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`；详细证据见审计明细第 35 节。
- 已核对当前/历史两种 Provider Port、Hithink 当前成分实现、Tushare index_weight 历史报告实现、Service 历史能力筛选与校验、HTTP/OpenAPI、WebUI API Client 和请求测试。
- 当前成分与历史报告分离；as_of 只调用 HistoricalIndexConstituentProvider，Hithink 当前成分不会被用于历史查询。Tushare 按月回溯，选取不晚于 as_of 的最新报告日，最多 120 个月；权重按百分点映射，缺失不补零，5000 行边界标 lower-bound。
- 历史报告不等于成员生效区间回放。Tushare 未提供 effective_from/to、available_at、system_observed_at；Service 会将缺少 PIT 证据的历史快照降级，未发现将其误标为 valid 的路径。
- 新增 **AUD-WEBUI-CTR-031**（P2）：历史报告缺少最大陈旧度策略和专门诊断；新增 **AUD-WEBUI-CTR-032**（P2）：历史成分名称使用当前 stock_basic 目录，缺少历史名称 PIT 证据；新增 **AUD-WEBUI-CTR-033**（P2）：WebUI 有 API Client 和请求测试，但没有页面消费与 DOM 测试。
- IndexConstituent.weight_percent 与 IndexConstituentSnapshot.provider_time 的 omitempty 指针被 OpenAPI 声明 nullable，纳入 **AUD-WEBUI-CTR-010**。
- 未发现已证实的报告日期越界或权重单位换算错误；时间陈旧度、历史名称和 UI 闭环仍待补强。WebUI 维度 2 继续 partial。


### WebUI — 维度 2：Index Daily Bars 指数历史日线

- 固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`；详细证据见审计明细第 36 节。
- 已核对 IndexDailyBar/IndexDailyBarsSnapshot DTO、IndexCatalogProvider Port、Hithink/Tushare 实现、Service 来源选择/校验/回退/缓存、HTTP Handler、OpenAPI、Go 模型契约测试及 WebUI API Client/API 请求测试。
- Hithink 和 Tushare 均校验请求日期范围、重复日期、OHLC；Service 校验日期范围、唯一性、升序、来源/版本、计数和单位组合。Tushare 成交量保留 hands，成交额由千元换算为 CNY；Hithink 成交量为 shares，turnover 单位未证实为 CNY，故标 provider_defined 并降级。
- Tushare 不提供 Provider 快照时间戳，条目/快照保守降级；Hithink 使用响应级 timestamp。空结果为 unknown，不补零。
- 新增 **AUD-WEBUI-CTR-034**（P2）：没有与交易日历核对预期交易日集合，不能证明请求区间无漏日；新增 **AUD-WEBUI-CTR-035**（P2）：OpenAPI 未完整枚举/说明跨 Provider 成交量和成交额单位；新增 **AUD-WEBUI-CTR-036**（P2）：WebUI 有 API Client 和请求测试，但没有页面消费者及 DOM 测试。
- IndexDailyBar.change_percent/volume/amount/provider_time 与 IndexDailyBarsSnapshot.provider_time 纳入 **AUD-WEBUI-CTR-010**（omitempty 指针被 OpenAPI 声明 nullable）。
- 未发现已证实的 OHLC 算术或 Tushare 单位换算错误；交易日完整性、Hithink amount 可比性和 UI 闭环仍待补强。WebUI 维度 2 继续 partial。


### WebUI — 维度 2：API Client 与 DTO 全量盘点（第一轮）

- 详细记录：[`WebUI-维度2-API-DTO全量盘点.md`](./WebUI-维度2-API-DTO全量盘点.md)
- 代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- `web/src/api.js` 共 44 个导出函数（3 个本地设置函数、41 个远端请求/健康检查入口）；41 个远端入口中，40 个在主要页面模块找到精确引用；`getMarketSentiment` 是唯一已确认的孤儿 Client，需确认是否为预留接口，不自动登记 P2。
- Market Data HTTP Adapter 的 35 个 OpenAPI path 与 `server.go` 路由清单已对照；Analysis Task OpenAPI 覆盖任务三条路径，但未描述 Client 使用的 `GET /api/v1/analysis/runs/{run_id}`，登记为待补 Schema/契约证据，不在本轮直接定性运行时错误。
- `web/tests/api.test.mjs` 已覆盖多类请求；Market Hot Stocks、Market Indices、Sentiment/History、Breadth、Limit Pool、Dragon Tiger、两类 Liveness 等函数未见该文件直接请求测试，需再核浏览器测试覆盖。
- 待核数据集收敛为 Hot Rankings、Market Sentiment History、Limit Pool、Dragon Tiger、Stock Search、Analysis Run、Provider Probe History；既有第 1–36 节已审数据集不重复展开。
- 本轮只完成入口盘点，不代表 DTO/Schema/Golden 完整；WebUI 维度 2 保持 partial。


### WebUI — 维度 2：Hot Rankings 人气榜与飙升榜

- 详细记录：[`WebUI-维度2-Hot-Rankings.md`](./WebUI-维度2-Hot-Rankings.md)
- 已核对 HotRanking DTO、统一 HotRankingProvider Port、Eastmoney/同花顺/雪球实现、默认 Provider 配置、Bootstrap 装配、Service 校验/回退/缓存、HTTP Handler、OpenAPI、WebUI API Client/市场概览消费及相关测试。
- 默认只启用 Eastmoney HotRankings；Hithink、Xueqiu 虽实现相同 Port，但默认 disabled。榜单是延迟排名数据，成功结果保持 degraded；热度量纲由 Provider 定义，不应跨源直接比较。
- 新增 **AUD-WEBUI-CTR-037（P2）**：HotRankingItem.change_percent 的 OpenAPI 描述错误引用 LimitPoolSnapshot 的 has_data 约束，可能误导消费者。HotRankingItem 五个 omitempty 数值字段及 Snapshot.provider_time 的 nullable 不一致纳入 CTR-010。
- WebUI 当前只开放 popular/surge；服务端/OpenAPI/雪球 Provider 另支持 followers，归为产品范围待决，不直接登记缺陷。
- `getMarketHotStocks` 未见 API Client 专用请求测试，页面测试也未直接验证榜单真实响应 DOM；已有源码结构断言和 Provider/Service 测试。WebUI 维度 2 继续 partial。


### WebUI — 维度 2：Market Sentiment History 市场情绪历史

- 详细记录：[`WebUI-维度2-Market-Sentiment-History.md`](./WebUI-维度2-Market-Sentiment-History.md)
- 已核对 MarketSentimentSnapshot/History DTO、DataService 采集与历史读取、MariaDB Repository/迁移、HTTP Handler/OpenAPI、51 模型反射契约测试、WebUI API Client/历史表格及相关测试。
- 历史 API 只读已持久化记录；当前快照接口负责采集并追加保存。历史页不触发采集；`getMarketSentiment` Client 无页面消费者，每日采集责任需 Server/部署侧补证。
- DTO/OpenAPI 字段集合和 required 已有反射检查；辅助池计数显式 null，缺失不补零；下界语义在 UI 显示为“≥N”。
- 新增 **AUD-WEBUI-CTR-038（P2）**：history.total 实际是本次返回条数，不能表达范围总量或截断状态；WebUI 的 N/N 显示无法判断是否还有记录。
- 新增 **AUD-WEBUI-CTR-039（P2）**：Repository 以 MAX(id) 选择每日最新记录，但注释语义是最近采集；并发乱序写入时 ID 顺序不保证 fetched_at 顺序。
- 新增 **AUD-WEBUI-CTR-040（P2）**：WebUI 日期范围校验未传入服务端要求的 366 自然日上限，超限只能请求后收到 400。
- API Client 未见直接请求测试；History Handler/Repository 缺少专用回归测试；页面有日期/取消/下界/时间证据单元测试，但缺真实历史表格 DOM 测试。
- 本切片完成后，盘点中剩余待审数据集：Limit Pool、Dragon Tiger、Stock Search、Analysis Run、Provider Probe History。WebUI 维度 2 继续 partial。


### WebUI — 维度 2：Limit Pool 涨停池、跌停池与炸板池

- 详细记录：[`WebUI-维度2-Limit-Pool.md`](./WebUI-维度2-Limit-Pool.md)
- 已核对 LimitPoolItem/LimitPoolSnapshot DTO、LimitPoolProvider Port、Eastmoney/Hithink Provider、默认配置与 Bootstrap、Service 校验/回退/缓存、Handler/OpenAPI、WebUI Client/页面和相关测试。
- 默认仅启用 Eastmoney；Hithink 实现同一 Port 但默认 disabled。Eastmoney 会跨页读取至调用方 limit，Hithink 当前只读第一页。两家均将无数据响应保持 unknown，不作为已确认零事件；超出返回条数的源端 total 会使快照 degraded。
- Eastmoney 不填充金额扩展字段；Hithink 的 seal_money/max_seal_money/turnover 单位未得到权威证明，标记 provider_defined 并降级。
- 原登记 **AUD-WEBUI-CTR-041 已撤销**：复核固定基线发现 SealMoney/MaxSealMoney 实际均带 omitempty；OpenAPI 未将其列为 required/nullable，JSON Golden 也要求省略，三方一致。原差异源于审计时误读 JSON tag，不作为 P1 门禁。
- 新增 **AUD-WEBUI-CTR-042（P2）**：Hithink 金额字段缺少可验证单位，跨 Provider 无法安全比较/换算。
- CTR-010 影响范围增加 LimitPoolItem 六个 omitempty 数值字段及 LimitPoolSnapshot.provider_report_date，共七个字段。
- 页面显示主要榜单字段、总量/返回量/请求上限、来源时间和质量，不消费金额扩展字段；是否需要展示待产品确认。API Client 与 Limit Pool DOM 专项测试缺失，已有 Provider/Service 覆盖分页、部分覆盖、重复证券和 unknown 回退。
- WebUI 维度 2 继续 partial。盘点剩余待审数据集：Dragon Tiger、Stock Search、Analysis Run、Provider Probe History。


### WebUI — 维度 2：Dragon Tiger 龙虎榜

- 详细记录：[`WebUI-维度2-Dragon-Tiger.md`](./WebUI-维度2-Dragon-Tiger.md)
- 已核对 DragonTigerItem/Snapshot DTO、DragonTigerProvider Port、Eastmoney/Hithink Provider、默认配置与 Bootstrap、DataService 校验/回退/缓存、Handler/OpenAPI、WebUI Client/市场活动页面和现有测试。
- 默认仅启用 Eastmoney；Hithink 虽实现相同 Port，但默认 disabled。Eastmoney 对无报告和空报表保持 unknown，排除可转债/B 股并保留本页原始/接受/排除计数；因源端没有精确快照时间，有数据的结果保持 degraded。
- Hithink 金额单位未获明确证明，原始金额独立保存在 provider_defined 字段，不映射为 CNY。其 Provider 当前一次读取 stock_items 全集并本地分页。
- 新增 **AUD-WEBUI-CTR-043（P1）**：Hithink 将整份 stock_items 数量写入 source_returned，但 Service 要求当前页原始记录数；总记录数超过 page_size 时会被拒绝，导致其无法有效承担回退。此外重复 symbol 被静默丢弃却不计入排除数，也破坏 source_returned=returned+excluded 计数恒等式。
- 新增 **AUD-WEBUI-CTR-044（P2）**：Hithink 显式日期只支持最近一年，但 DataService/Handler/OpenAPI 公共契约未披露 Provider 范围限制；默认配置 disabled，启用回退时才影响较早日期查询。
- CTR-010 影响范围增加 DragonTigerItem 16 个 omitempty 指针字段，以及 provider_amount_unit、provider_report_date 两个 omitempty 字符串；OpenAPI 均声明 nullable=true，与 Go nil 时省略不一致。现有 JSON 序列化 Golden 未覆盖 DragonTiger DTO。
- WebUI 已展示分页计数、排除数、质量与来源时间，并只展示明确 CNY 金额字段；API Client 无直接请求测试，页面无龙虎榜分页/渲染 DOM 回归。当前也缺 Hithink 龙虎榜 Provider 专项测试。
- WebUI 维度 2 继续 partial。剩余待审数据集：Stock Search、Analysis Run、Provider Probe History。


### WebUI — 维度 2：Stock Search 证券搜索

- 详细记录：[`WebUI-维度2-Stock-Search.md`](./WebUI-维度2-Stock-Search.md)
- 已核对 StockSearchItem/Snapshot DTO、StockSearchProvider Port、Tencent Smartbox 与 Eastmoney Provider、独立 Provider 优先级、默认配置、Service 校验/回退、Handler/OpenAPI、WebUI API Client/自动补全和测试。
- 默认 Tencent 与 Eastmoney 均启用，搜索专用顺序为 Tencent → Eastmoney → 配置优先级。Tencent 非空结果通常以 degraded 返回并直接使用；无匹配时才回退 Eastmoney。Eastmoney 全量 A 股目录缓存 6 小时，刷新失败可回退 stale 缓存。
- Tencent 不提供可核验匹配总数，total 是本次解析匹配候选数，quality 固定 degraded；Eastmoney 从完整证券目录进行匹配，total 是本地目录中的匹配数量。DTO/OpenAPI required/nullable 一致。
- 新增 **AUD-WEBUI-CTR-045（P2）**：WebUI 仅识别 stale，忽略 degraded/unknown 和 quality_reasons。尤其 unknown 空结果实际表示回退耗尽仍无法确认，却显示“没有匹配的股票”，容易被误解为证券不存在。需按四种质量状态展示并增加 app.js 搜索流程 DOM 回归。
- API Client 已有中文 query 编码、API Key、AbortSignal 测试；Handler 覆盖成功、空 q 和超限 limit；view 组件覆盖候选安全渲染和 ARIA，但缺 loadStockSearch 质量状态集成 DOM 测试。
- WebUI 维度 2 继续 partial。剩余待审数据集：Analysis Run、Provider Probe History。


### WebUI — 维度 2：Analysis Run 分析结果与持久化读取

- 详细记录：[`WebUI-维度2-Analysis-Run.md`](./WebUI-维度2-Analysis-Run.md)
- 已核对 RunRequest/RunResult/RunStatus/StageEvent、Analysis Runner/RunStore Port、MariaDB/Redis 结果读取与写入、TaskQueue 创建/幂等/Worker/状态映射/事件游标、HTTP Handler、WebUI API Client/轮询/Run 结果视图及相关测试。
- 已明确 Run 与 Task 是不同状态域：Run=blocked_data_quality 映射 Task=succeeded，表示任务执行已结束，不代表质量门禁通过；RunResult 从 MariaDB 读取，Redis 仅为缓存；Task Events 使用数据库自增 id 作为 sequence 游标。
- 新增 **AUD-WEBUI-CTR-046（P2）**：WebUI 实际调用的 GET /api/v1/analysis/runs/{run_id} 及 Run Events 未进入正式 OpenAPI，且缺 RunResult/RunStatus/Failure/StageEvent 等响应 Schema；HTTP Server 已有 GET Run 404 和 500 脱敏测试；缺 GET Run 成功、409 run_not_finished 与 Run Events 专项测试。
- 新增 **AUD-WEBUI-CTR-047（P2）**：AnalysisRequest 声称与 domain.RunRequest 一致，但遗漏 as_of_mode、parameters；run_id 在异步队列由服务端生成覆盖，as_of_mode 由 Handler 根据 as_of 重设，需明确异步请求 DTO 与字段行为。
- WebUI 已校验 task_id/run_id/事件游标/事件身份，Run 结果读取校验 run_id 和证券集合，并对可重试错误有限重试；API Client 测试验证 Run URL、分析端不附带 Market API Key 和结构化 409 错误。
- 本轮未执行固定基线 CI；DB migration 索引/唯一约束、多实例 Worker claim、RunResult 嵌套 DTO Golden 仍未完成专项核验。
- WebUI 维度 2 继续 partial。剩余待审数据集：Provider Probe History。


### WebUI — 维度 2：Provider Probe History 主动探测历史

- 详细记录：[`WebUI-维度2-Provider-Probe-History.md`](./WebUI-维度2-Provider-Probe-History.md)
- 已核对 ProviderProbeRun/History DTO、DataService 探测汇总与脱敏、MariaDB 表/写入/最近批次读取、HTTP POST/GET、OpenAPI、WebUI API Client/数据中心页面、API/浏览器/MariaDB 集成测试资产。
- 已确认历史 GET 只读 MariaDB 已持久化批次，不触发 Provider 探测；默认最近 20 批、最大 100 批；历史按 started_at DESC、id DESC 排序。API Key 认证、limit 校验、请求取消、历史读取失败提示和敏感错误脱敏均有实现。
- 新增 **AUD-WEBUI-CTR-048（P2）**：Repository 仅查询 LIMIT N，Service 将 total 设置为 len(items)，并非数据库总记录数；OpenAPI 未定义 total 语义，UI“服务端记录 X 批”可能误导。需定义全量 total 或改为 returned_count/has_more。
- 新增 **AUD-WEBUI-CTR-049（P2）**：MariaDB 写入失败由 Service 返回持久化错误，但 POST Handler 把所有 Service error 映射成 400 VALIDATION_ERROR；OpenAPI 却声明 500，且 Handler 丢弃带 persisted=false 的探测结果。需区分存储错误并补 Handler 故障测试。
- MariaDB 集成测试覆盖真实写入、重启后历史读取和证据保留，但依赖 MARKET_DATA_TEST_MYSQL_DSN，本轮未执行；缺少独立 GET Handler 对 limit/503/SQL 失败的单元测试。
- WebUI 维度 2 三个剩余数据集（Stock Search、Analysis Run、Provider Probe History）均已审完。下一步进入维度 2 完整性 Review，不再扩展数据集清单。


### WebUI — 维度 2：第一轮完整性 Review（子批次 1）

- 详细记录：[`WebUI-维度2-完整性Review-第一轮.md`](./WebUI-维度2-完整性Review-第一轮.md)
- 三个最后数据集 Stock Search、Analysis Run、Provider Probe History 均已形成独立审计切片；本轮不增加数据集范围。
- 对照 API Client 总盘点：44 个导出函数中 41 个远端入口，40 个在主要页面模块找到引用；getMarketSentiment 仍是唯一已确认的孤儿 Client，保持产品范围待确认，不自动定性缺陷。
- 编号收敛：CTR-003 的 Run OpenAPI 缺失核心问题由范围更完整的 CTR-046 承接，CTR-003 标记为历史初审编号，不重复计数；CTR-005 与 CTR-047 部分重叠，CTR-047 关闭条件必须继续覆盖 TaskType 四值枚举及 AnalysisRequest required 约束。
- CTR-010 的多次出现是跨 DTO 影响字段扩充；CTR-038 与 CTR-048 虽同为历史 total 语义问题，但属于不同 API，不合并。
- P1 门禁复核（后续更正）：CTR-041 已依据固定 SHA 的 DTO/OpenAPI/Golden 一致性撤销；CTR-043 的源码逻辑冲突仍成立，等待责任模块修复和专项测试，本轮未执行 CI。
- 本轮只完成第一轮完整性 Review 的编号/覆盖/跨数据集主题子批次；逐项核对 CTR-001..049 证据链、字段级 Golden、固定基线 CI 及跨维度交叉检查仍未完成。WebUI 维度 2 保持 partial。

- Analysis Task 请求契约补充核对：Handler/fillDefaults 会为 task_type、as_of、universe_id、portfolio_id、market_id、strategy_ids 和配置版本注入默认值；historical_replay 则要求显式 as_of；Runner 只接受四种 TaskType。故 CTR-005/047 的关闭条件应描述 HTTP 请求默认值、TaskType 四值枚举及历史回放条件约束，不应机械地把所有 RunRequest 非 omitempty 字段列为 HTTP required。

- P1 门禁复核更正：CTR-041 经固定 SHA 源码、OpenAPI 和 JSON Golden 对照后撤销；LimitPoolItem.seal_money/max_seal_money 的 omitempty、非 required/非 nullable、JSON omitted 三方一致。当前维度 2 已确认仍需处理的 P1 为 CTR-043（Hithink 龙虎榜分页计数契约冲突）；本轮未执行 CI。

- CTR-010 影响范围抽查（Limit Pool/Dragon Tiger）：两组 DTO 的已登记 omitempty 字段与 OpenAPI nullable=true 不一致属实；LimitPoolItem.seal_money/max_seal_money 则为 omitempty、OpenAPI 非 nullable/非 required、Golden omitted，明确排除在 CTR-010 外。CTR-010 全量字段复核仍待后续子批次。

### WebUI — 维度 2：第一轮完整性 Review（子批次 2）

- 详细记录：WebUI-维度2-完整性Review-第一轮.md 第 9 节。
- 本批只复核 Provider Probe History 的 CTR-048、CTR-049，不扩展数据集范围。
- CTR-048 已由 Repository 的 LIMIT 查询、Service 的 Total=len(items)、OpenAPI 未定义 total 语义及 WebUI 文案共同证实，保留 P2；关闭时需明确全量 total 或 returned_count 语义。
- CTR-049 已由 Service 持久化错误返回、POST Handler 统一映射 400、OpenAPI 声明 500 共同证实，保留 P2；需区分参数错误和存储错误并补 Handler 故障测试。
- 补充确认 API Client 与浏览器测试覆盖历史正常/异常流程；MariaDB HTTP 集成测试覆盖重启后读取但依赖 MARKET_DATA_TEST_MYSQL_DSN，本轮未执行。历史排序/limit/重复 run_id 更新及 GET Handler 独立故障测试仍缺。
- 本批无新增 CTR 编号、无业务代码修改。维度 2 第一轮完整性 Review 继续 partial。
### WebUI — 维度 2：第一轮完整性 Review（子批次 3）

- 详细记录：WebUI-维度2-完整性Review-第一轮.md 第 10 节。
- 本批只复核 Stock Search 的 CTR-045。Tencent 正常非空结果固定为 degraded；Service 在所有搜索源耗尽后可返回 unknown 空结果；OpenAPI 明确要求消费者处理质量状态。
- WebUI loadStockSearch 对空 items 直接显示无匹配，对非空结果只识别 stale，未消费 degraded/unknown/quality_reasons。CTR-045 证据成立，保留 P2。
- StockSearchItem/StockSearchSnapshot 的 Go JSON tag 与 OpenAPI required/nullable 一致；total 语义已明确为 Provider 匹配候选数，不是全市场证券总量。本批未发现新的 DTO/Provider 契约差异。
- view 组件测试覆盖文本安全渲染和 ARIA，但缺 app.js 搜索流程质量状态 DOM 集成测试。本批未执行固定基线 CI。
- 本批无新增 CTR 编号、无业务代码修改。维度 2 第一轮完整性 Review 继续 partial。
### WebUI — 维度 2：第一轮完整性 Review（子批次 4）

- 详细记录：WebUI-维度2-完整性Review-第一轮.md 第 11 节。
- 本批复核 CTR-046、CTR-047。Run GET 与 Run Events 路由真实存在，但权威分析任务 OpenAPI 缺少两条路径及 RunResult/事件响应 Schema；CTR-046 保留 P2。
- 更正 CTR-046 测试资产描述：server_test.go 已覆盖 GET Run 404 和 500 脱敏；仍缺成功响应、409 run_not_finished 与 Run Events Handler 专项测试。
- CTR-047 经 RunRequest、TaskType 常量、CreateTask Handler 默认/时间模式处理及 Queue 生成覆盖 run_id 的代码核对，字段差异和异步请求 Schema 描述不准确成立，保留 P2。关闭时按 HTTP CreateTaskRequest 语义定义 required/default，不能机械复制领域结构体。
- 本批没有新增 CTR 编号或业务代码修改；固定基线 CI、RunResult 全字段 Golden、migration/Worker 并发专项仍未执行。
### WebUI — 维度 2：CTR-010 字段清单复核（子批次 5）

- 详细记录：WebUI-维度2-完整性Review-第一轮.md 第 12 节。
- 本批只核对 ChipDistribution。确认 date、cost_90_low/high、cost_70_low/high、profit_ratio、concentration_90/70、provider_date 共九个字段：Go tag 带 omitempty，OpenAPI 声明 nullable=true，均非 required，九项全部属于 CTR-010。
- source_version、quality_reasons 虽也 omitempty，但 OpenAPI 未声明 nullable，不纳入差异；average_cost、symbol、source、fetched_at、quality_status 的 required 与 Go 一致。
- 通用模型登记测试覆盖字段/required，但没有对 omitempty 的 nullable 反向断言；JSON serialization test 未覆盖 ChipDistribution。CTR-010 的九处冲突确认，未新增编号；本轮未执行固定基线 CI。
- 本批只完成 ChipDistribution 子集，CTR-010 全量字段清单仍未完成。
### WebUI — 维度 2：CTR-010 字段清单复核（子批次 6）

- 详细记录：WebUI-维度2-完整性Review-第一轮.md 第 13 节。
- StockBasic/Profile 确认 14 个 omitempty 字段被 OpenAPI 声明 nullable=true；SecurityTradingStatusSnapshot 确认 system_observed_at 1 个；PriceLimitRuleSnapshot 确认 special_exception_evidence、limit_percent 2 个。合计 17 个字段纳入 CTR-010。
- 同 DTO 中其他 omitempty 字段的 OpenAPI 未声明 nullable，不计入 CTR-010；required 集合与 Go 非 omitempty 字段总体一致。
- 测试已覆盖 StockBasic、SecurityTradingStatusSnapshot 的字段/required，但没有 omitempty→非 nullable 反向断言或对应 JSON 序列化 Golden；PriceLimit Rule inline Schema 缺 Go DTO 映射测试。本轮未执行固定基线 CI。
- 本批无新增 CTR 编号或业务代码修改；CTR-010 全量字段清单仍未完成。
### WebUI — 维度 2：CTR-010 字段清单复核（子批次 7）

- 详细记录：WebUI-维度2-完整性Review-第一轮.md 第 14 节。
- Fundamentals：FundamentalsSnapshot.system_observed_at 1 个 omitempty + nullable 差异；FundamentalSection.available_at 已正确未声明 nullable。
- Announcements：StockAnnouncement.published_at 1 个 omitempty + nullable 差异。
- Valuations：ValuationItem 五个扩展指标指针字段均 omitempty + nullable；核心六项非 omitempty 指针 nullable 正确。
- 合计确认 7 个字段，归入 CTR-010，不新增编号。反射测试覆盖字段集合/required，但未检查 omitempty 字段 nullable 反向冲突；对应 JSON 序列化样例缺失。本批未执行固定基线 CI。
- 本批无业务代码修改；CTR-010 全量字段清单仍未完成。
### WebUI — 维度 2：CTR-010 字段清单复核（子批次 8）

- 详细记录：WebUI-维度2-完整性Review-第一轮.md 第 15 节。
- Market Breadth 确认 oldest_quote_time、newest_quote_time 2 个 omitempty + nullable 字段；Market Indices 的 MarketIndexQuote 确认 9 个；Index Catalog 确认 IndexCatalogSnapshot.provider_time 1 个。
- Index Constituents 确认 IndexConstituent.weight_percent、IndexConstituentSnapshot.provider_time 共 2 个；Index Daily Bars 确认 IndexDailyBar.change_percent、volume、amount、provider_time 及 IndexDailyBarsSnapshot.provider_time 共 5 个。
- 本批合计 19 个字段，归入既有 CTR-010，不新增编号；专门分批清单累计核对 52 个字段。此前其他数据集章节零散登记的 CTR-010 字段尚待总表去重汇总，因此 52 不是最终全量数量。
- 51 模型反射测试覆盖本批 DTO 的字段集合/required，但未验证 omitempty→非 nullable 反向约束；JSON 序列化测试没有本批 DTO 样例。本批未执行固定基线 CI。
- 本批无业务代码修改；CTR-010 全量字段清单仍未完成。

### WebUI — 维度 2：CTR-010 字段清单复核（子批次 9）

- 详细记录：WebUI-维度2-完整性Review-第一轮.md 第 16 节。
- Quote 确认 volume_ratio、turnover_rate、raw_volume、raw_amount 共 4 个 omitempty + nullable 字段；DailyBar 确认 adjustment_factor_system_observed_at、provider_timestamp、system_observed_at、raw_volume、raw_amount 共 5 个。
- Bundle.quote 是 omitempty 可选引用，Schema 未声明 nullable，行为一致；StockBoard.change_percent 确认 1 个差异。
- SectorRanking 确认 latest_price、change、change_percent、change_percent_5d、amount、advancing_count、declining_count、leader_change_percent 共 8 个差异。合计 18 个字段，归入既有 CTR-010。
- 反射测试覆盖本批 DTO 字段集合/required，但未检查 omitempty→nullable 反向冲突；现有 JSON 序列化测试只覆盖 Quote/DailyBar 核心非 omitempty 指针及 Bundle.quote 省略，没有覆盖本批 18 个差异字段。本批未执行固定基线 CI。
- 截至子批次 9，CTR-010 专门分批清单累计核对 70 个字段；此前其他数据集章节零散登记的字段尚待统一去重汇总。无业务代码修改。


### WebUI — 维度 2：CTR-010 字段清单复核（子批次 10）

- 详细记录：WebUI-维度2-完整性Review-第一轮.md 第 17 节。
- 本批复核 Limit Pool、Dragon Tiger、Market Sentiment、Market Capital Flow 与个股 Capital Flow。确认既有 CTR-010 字段 41 个：Limit Pool 7、Dragon Tiger 18、Market Capital Flow 14、CapitalFlow 3；Market Sentiment 未发现新增 omitempty + nullable 冲突。
- 41 个字段均已在此前数据集章节零散登记，本批属于重复核验，不新增 CTR 编号，也不重复计入专门分批清单。LimitPoolItem.seal_money/max_seal_money 等 omitempty 但 OpenAPI 非 nullable 的字段明确排除。
- 51 模型字段/required 反射测试覆盖上述 DTO，但未验证 omitempty→非 nullable 的反向约束；JSON 序列化测试只覆盖 MarketSentimentSnapshot 的三个非 omitempty 计数指针及 LimitPoolItem.change_percent、seal_money/max_seal_money，没有覆盖 41 个差异字段的 nil 省略。Dragon Tiger、Market Capital Flow、CapitalFlow 缺 DTO 序列化样例。
- 本批未执行固定基线 CI，无业务代码修改。CTR-010 全量字段去重汇总仍未完成。
