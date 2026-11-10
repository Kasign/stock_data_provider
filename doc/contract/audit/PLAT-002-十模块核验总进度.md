# PLAT-002 十模块核验总进度

**当前状态：** 阶段 A 固定基线已完成；阶段 B（十模块六维核验）和阶段 C（完整 import/消费者/授权链）仍为 partial；阶段 D 项目级决策已收敛，B1～B8 已正式裁决，当前非条件性项目级 pending_decision 为 0；阶段 E（责任模块实现/补证）进入待执行状态；阶段 F 第一轮完整性 Review 为 partial；阶段 G 正确性 Review、阶段 H 冻结门禁 Review 尚未开始。L1 设计基线已冻结，L2 正式 CTR 当前 0/43 frozen。
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
| A | 固定 SHA、模块清单、文件规模与并行修改观察 | complete（基线固定；仅维护并行变更观察） |
| B | 十模块六维接口级核验 | partial（跨模块边界已覆盖主要关系，但十模块六维证据尚未逐项齐备） |
| C | 跨模块调用图、授权链和依赖图 | partial（主要边界已抽样交叉核验；全仓 import 图、全部消费者调用点和授权链尚未完成） |
| D | 差异、待裁决、补证唯一登记 | decision_complete（B1～B8 已裁决；差异和证据工作项唯一登记） |
| E | 责任模块实现/补证（本分支不改业务代码） | ready_for_execution |
| F | 第一轮完整性 Review | partial（WebUI 维度 2 与主要跨模块边界已完成首轮切片；十模块六维全覆盖尚未完成） |
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
- StockBasic/Profile 实际确认 12 个 omitempty 字段被 OpenAPI 声明 nullable=true（ipo_date/out_date 未声明 nullable）；SecurityTradingStatusSnapshot 确认 system_observed_at 1 个；PriceLimitRuleSnapshot 仅 limit_percent 1 个可确认（special_exception_evidence 是无同层 type 的 nullable $ref，不计入）。合计 14 个字段纳入 CTR-010。
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
- 截至子批次 9，CTR-010 专门分批清单经子批次 6 纠偏后累计核对 67 个字段；此前其他数据集章节零散登记的字段尚待统一去重汇总。无业务代码修改。


### WebUI — 维度 2：CTR-010 字段清单复核（子批次 10）

- 详细记录：WebUI-维度2-完整性Review-第一轮.md 第 17 节。
- 本批复核 Limit Pool、Dragon Tiger、Market Sentiment、Market Capital Flow 与个股 Capital Flow。确认既有 CTR-010 字段 42 个：Limit Pool 7、Dragon Tiger 18、Market Capital Flow 14、CapitalFlow 3；Market Sentiment 未发现新增 omitempty + nullable 冲突。
- 42 个字段均已在此前数据集章节零散登记，本批属于重复核验，不新增 CTR 编号，也不重复计入专门分批清单。LimitPoolItem.seal_money/max_seal_money 等 omitempty 但 OpenAPI 非 nullable 的字段明确排除。
- 51 模型字段/required 反射测试覆盖上述 DTO，但未验证 omitempty→非 nullable 的反向约束；JSON 序列化测试只覆盖 MarketSentimentSnapshot 的三个非 omitempty 计数指针及 LimitPoolItem.change_percent、seal_money/max_seal_money，没有覆盖 42 个差异字段的 nil 省略。Dragon Tiger、Market Capital Flow、CapitalFlow 缺 DTO 序列化样例。
- 本批未执行固定基线 CI，无业务代码修改。CTR-010 全量字段去重汇总仍未完成。


### WebUI — 维度 2：CTR-010 字段清单复核（子批次 11）

- 详细记录：WebUI-维度2-完整性Review-第一轮.md 第 18 节。
- Provider Probe 新确认 9 个此前未登记字段：ProviderProbeFieldComparison 4、ProviderProbeCheckResult 2、ProviderProbeItem 3，均为 Go omitempty + OpenAPI nullable=true，归入既有 CTR-010，不新增编号。
- ProviderProbeFieldComparison 是嵌套元素，未单独加入 51 模型反射注册表；父模型反射只核对字段名，不递归核验嵌套元素。三个 DTO 均缺 JSON nil 省略样例。
- MarketStockOverviewItem 内联 Schema 字段与 Go 空值语义一致；AdjustmentFactor、CorporateAction、StockUniverseSnapshot 当前未找到对应 OpenAPI 响应 Schema；ProviderProbeOptions 是内部选项结构，不纳入当前 HTTP 响应字段清单。
- 子批次10计数已更正：7 + 18 + 14 + 3 = 42，均为重复核验，不计入此前专门分批累计。子批次11本轮为新增字段登记。本批未执行固定基线 CI，无业务代码修改。


### WebUI — 维度 2：CTR-010 字段清单复核（子批次 12）

- 详细记录：WebUI-维度2-完整性Review-第一轮.md 第 19 节。
- HotRankingItem 确认 latest_price、change、change_percent、rank_change、heat 五个 omitempty 指针字段与 OpenAPI nullable=true 冲突；HotRankingSnapshot.provider_time 再确认一个，共 6 个字段，归入既有 CTR-010。
- 51 模型字段/required 反射测试已登记两个 DTO，但未检查 omitempty→nullable 反向冲突；JSON 序列化测试只覆盖 HotRankingItem 基础字段和其他可选字段省略，没有覆盖这六个字段的 nil 省略，也没有 HotRankingSnapshot 样例。
- 本批只做 DTO/OpenAPI/测试资产静态核验，未执行固定基线 CI，无业务代码修改。


### WebUI — 维度 2：CTR-010 全量唯一字段汇总

- 详细记录：WebUI-维度2-完整性Review-第一轮.md 第 20 节。
- 固定基线下 51 个具名 DTO 的 Go JSON tag/OpenAPI properties 静态对照确认 119 个 omitempty + nullable=true 字段；补入 3 个未进入具名注册表的 inline 响应 DTO：PriceLimitRuleSnapshot（limit_percent 1）、ProviderProbeFieldComparison（4）、MarketStockOverviewItem（0）。最终覆盖 54 个 HTTP 响应 DTO，确认 124 个唯一字段，分布于 29 个 DTO；其余 25 个 DTO 无此类差异。
- 更正子批次 6：StockBasic 从 14 调整为 12；PriceLimitRuleSnapshot 从 2 调整为 1；该批从 17 调整为 14。子批次 5—9 累计从 70 调整为 67。ipo_date/out_date 未声明 nullable；special_exception_evidence 是无同层 type 的 nullable $ref，不纳入已证实差异。
- 子批次10的42项、子批次12的6项均为历史重复复核，不再重复累计；子批次11的9项中，5项已在51 DTO扫描集合，4项来自嵌套 inline ProviderProbeFieldComparison。
- 中央 JSON 序列化 Golden 未对本节124个差异字段中的任何一个断言 nil/空值省略（0/124）；51 DTO 反射测试没有 omitempty→非 nullable 的反向约束，且三个 inline DTO 不在注册表中。
- 本节为固定 SHA 静态清单，不代表 CI 通过或 CTR-010 已修复。下一步只进行 CTR-010 关闭方案 Review，不扩展新的 WebUI 数据集。


### WebUI — 维度 2：CTR-010 契约差异处置边界 Review

- 详细记录：`WebUI-维度2-完整性Review-第一轮.md` 第 21 节。
- 固定基线下 CTR-010 已确认 124 个唯一字段、29 个受影响 DTO、54 个纳入范围的响应 DTO；Go `omitempty` 与 OpenAPI `nullable: true` 存在静态描述差异，中央 JSON Golden 对这 124 个字段的 nil/空值省略行为覆盖为 0/124。
- 更正此前“关闭目标已裁决”的表述：optional + non-nullable 只是候选方案，尚未完成公共契约语义裁决。不能仅依据当前 Go 序列化行为，直接推导最终契约允许/禁止 JSON null。
- 当前阶段只需对照权威契约完成字段缺失、null、零值语义裁决，并记录兼容性影响；不修改 Data DTO、OpenAPI 或契约测试，不要求 Data/API 分支实施。
- 裁决后将实现差异单独登记为冻结后的实现对齐任务；只有影响契约语义确定或触发明确冻结门禁时，才作为冻结前阻断处理。
- CTR-010 状态：差异事实已确认、字段清单已去重、目标语义待裁决、实现修复未启动。WebUI 维度 2 第一轮完整性 Review 仍在进行中。

### CTR-010 责任分支增量状态检查

- 检查分支：`feat/full-go-data-providers-api-switch`，检查时 HEAD 为 `56524658395ebc137ef5934ba72abe53372f3b38`。
- 与固定审计基线 `b6dfc85ba214fe16268c8cba780641a8bfe15086` 比较时，该分支与基线已明显分叉（merge base `d1b8201f822d0f19c474c23eeae7cad723f617c0`），因此本次只检查 CTR-010 相关文件，不把该分支整体作为审计基线，也不进行合并。
- `internal/marketdata/httpapi/openapi.yaml` 与固定基线 blob SHA 相同（`cd8a6e3dfb8e3162ee789b11e883be9283c59f32`）；`internal/marketdata/httpapi/openapi_contract_test.go` 与固定基线 blob SHA 相同（`7f97f075a23bd698a7e3fc32e90509d1e999217b`）。因此截至该 HEAD，CTR-010 的 124 个 OpenAPI nullable 声明及中央契约测试缺口均未在责任分支体现修复。
- 该分支最新提交主题为固定样本 Provider 原始响应诊断；本次不评价其余 Data 工作，也不因分支存在大量其他提交而扩大审计范围。
- 本轮只读取责任分支并更新审计进度文档；未修改 Data 代码、未切换/合并分支、未运行 CI。该责任分支检查记录仅说明截至对应 SHA 未出现相关文件变化；CTR-010 当前状态为差异已确认、契约语义待裁决、实现修复未启动。

### WebUI — 维度 2：Dragon Tiger CTR-043 / CTR-044 冻结影响复核

- 详细记录：`WebUI-维度2-完整性Review-第一轮.md` 第 22 节。
- CTR-043：DTO 和 Service 已明确 `source_returned` 是本页原始记录数，且计数需与接受数/非 A 股排除数守恒；Hithink 却以全量 `stock_items` 数量填充并在本地分页，重复 symbol 还被静默丢弃。判定为公共契约明确、Adapter 实现不符合，P1 保留。冻结评审需显式作出修复或延期决议，不在契约核验阶段修改实现。
- CTR-044：公共 API 接受任意有效历史日期，但 Hithink 仅支持最近一年。判定为 Provider 能力边界披露方式待裁决；不应将单一 Provider 的限制收窄为公共 API 全局限制。冻结前需明确 capability/诊断元数据或权威能力文档的表达方式。
- 本批仅完成既有差异的冻结影响分类；未修改业务代码、未执行 CI、未新增差异编号。WebUI 维度 2 第一轮完整性 Review 继续进行。

### WebUI — 维度 2：Market Sentiment History CTR-038～CTR-040 冻结影响 Review

- 详细记录：`WebUI-维度2-完整性Review-第一轮.md` 第 23 节。
- CTR-038：Service 在 Repository LIMIT 后设置 `total=len(items)`；OpenAPI 未定义 total 语义；WebUI 固定 limit=120 并显示 `items.length/total`。当日期范围超过 120 个已保存交易日时无法识别截断。注意公共范围最多 366 个自然日、每日最多一条；API 默认 limit=366、最大500，本身足以覆盖完整范围，实际风险来自 UI 显式 limit=120。归入 DATA-CTR-001/GOV-CTR-001，total 的“范围总数”或“返回数”语义待裁决；审计建议优先定义为 limit 前的范围内不同交易日记录总数。未修改代码。
- CTR-039：Repository 每日选 `MAX(id)`，注释却称“最近一次采集”；id 是插入顺序，不能代表 fetched_at。归入 DATA-CTR-001/GOV-CTR-002，需裁决按 fetched_at DESC、id DESC 选择，还是按最后写入选择。审计建议采用 fetched_at DESC、id DESC；尚未形成正式决议。未修改代码。
- CTR-040：OpenAPI 与 Handler 已明确闭区间 366 自然日限制，Handler 有 366/367 日边界测试；WebUI 调用日期校验时未传 maxCalendarDays，367 日请求会在服务端被拒。公共规则明确，属于 WebUI 实现未对齐。另发现 DataService 的日期差校验对 367 个包含首尾的自然日存在一日边界差异；当前 HTTP Handler 会先拦截，尚无绕过 Handler 的实际消费者证据，暂并入 CTR-040 的防御性核验，不新建编号。
- 项目级映射仅引用既有 DATA-CTR-001、GOV-CTR-001、GOV-CTR-002，不新增正式契约编号。CTR-038/039 需要语义裁决；CTR-040 需要记录消费者未对齐/延期状态。
- 本批未修改权威登记簿、业务代码、OpenAPI 或测试；未执行 CI。WebUI 维度 2 第一轮完整性 Review 继续进行。

### WebUI — 维度 2：Limit Pool CTR-042 金额单位证据与冻结影响 Review

- 详细记录：`WebUI-维度2-完整性Review-第一轮.md` 第 24 节。
- 固定基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`。Hithink 将 `seal_money`、`max_seal_money`（up 池）和 `turnover`（broken 池）原值写入 DTO，不换算；`provider_amount_unit=provider_defined`，并附单位未证实原因、保持 degraded。Eastmoney 不提供这些扩展金额字段且默认启用，Hithink 默认 disabled；WebUI 不消费这些金额扩展字段。
- OpenAPI 已说明 provider_defined 不代表 CNY、金额解释需结合 provider_amount_unit；但仓库内没有 Hithink 对三个金额字段的权威单位定义，也没有对应单位/保真专项测试。turnover_ratio_percent 是百分数，不属于本 CTR。
- CTR-042 准确性质为 Provider 单位证据缺失、归一化能力不可用，不是已证实换算错误。当前实现通过保留原始值、标记 provider_defined 和 degraded 避免误标 CNY。
- 映射既有 DATA-CTR-002（单位口径）和 DATA-CTR-003（Provider 原始值标准化与精度保真），不新增正式编号。建议明确 provider_defined 值禁止换算、跨 Provider 比较、求和和按金额排序；如展示须标明单位未知，或由产品选择隐藏。此为待裁决建议，不是已批准决议。
- Limit Pool 可在未知单位安全消费边界明确后冻结公共字段语义；但 Hithink 金额值仍 not_comparable，不能宣称归一化完成。DATA-CTR-002/003 整体仍需其他字段/Provider 证据。
- 本批未修改代码、OpenAPI、DTO 或测试，未执行 CI；WebUI 维度 2 第一轮完整性 Review 继续进行。

### WebUI — 维度 2：Stock Search CTR-045 冻结影响 Review

- 详细记录：`WebUI-维度2-完整性Review-第一轮.md` 第 25 节。固定代码基线仍为 `b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- Tencent Smartbox 无可核验匹配总数，非空候选为 degraded / `provider_search_total_not_reported`；无候选时返回错误触发后备。Service 对 unknown 空/非空结果继续回退，耗尽后保留 unknown 和明确的 fallback-exhausted 原因；stale 候选也只有在后备耗尽后才返回。
- OpenAPI 要求调用方按 quality_status / quality_reasons 处理未知或降级质量，但未明确空结果在各质量状态下能否宣告“无匹配”。HTTP Handler 原样返回质量元数据，没有把 unknown 空结果转成确定无匹配。
- WebUI `loadStockSearch` 对所有空 items 直接显示“没有匹配的股票”；非空结果只处理 stale，忽略 degraded、unknown 和 quality_reasons。现有 view/API/Service/Provider 测试分别覆盖局部能力，但没有 app.js 搜索质量状态集成 DOM 回归。
- **CTR-045 保留 P2，证据成立。** 核心是 WebUI 质量状态消费缺失，伴随公共契约空结果语义不够明确；不新增 CTR。建议裁决 valid + empty 才能显示确定无匹配，degraded/unknown 空结果提示证据不足，非空候选按 valid/degraded/stale/unknown 区分提示并消费质量原因。该建议待正式裁决。
- 关闭条件：补齐四种状态的空/非空分支处理、质量原因安全提示，以及 app.js DOM 回归，特别验证 unknown 空结果绝不显示成确定无匹配；可补 API Client 元数据保留测试，但不能替代 DOM 测试。
- 本批未修改业务代码、OpenAPI 或测试，未执行 CI。WebUI 维度 2 第一轮完整性 Review 继续进行。

### WebUI — 维度 2：Analysis Run CTR-046 / CTR-047 冻结影响 Review

- 详细记录：`WebUI-维度2-完整性Review-第一轮.md` 第 26 节。固定代码基线仍为 `b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- **CTR-046 保留 P2**：Server 实际提供 Run GET 与 Run Events；Run GET 返回 RunResult，状态含 200/404/409/500；Run Events 返回 `{run_id, events}`，状态含 200/404/500。权威分析任务 OpenAPI 缺少两条路径及 RunResult/事件响应 Schema。已有 Run GET 404、500 脱敏测试；缺成功、409 和 Run Events Handler 专项测试。Task Events 测试不能替代 Run Events。该项阻断 Analysis Run 对外契约冻结。
- **CTR-047 保留 P2**：CreateTask Handler 将 JSON 解码为领域 RunRequest，OpenAPI却只列部分字段并声称两者一致。Handler 会补 task_type、as_of、universe_id、portfolio_id、market_id、strategy_ids、配置版本默认值；historical_replay 必须显式 as_of；as_of_mode 根据是否显式提供 as_of 被服务端覆盖；Queue 生成并覆盖 run_id；parameters 是 map[string]string，Runner 可从 parameters.symbols 回退取标的。不能机械复制领域字段为 HTTP required。
- CTR-047 关闭前须裁决独立 CreateTaskRequest、TaskType 四值枚举/非法值处理时机、默认字段、historical_replay 条件约束、as_of_mode/run_id 服务端所有权、parameters 类型与校验时机，并同步 OpenAPI、Handler 和测试。CTR-005 的遗留枚举/required要求由 CTR-047 承接；CTR-003 的路径缺失由 CTR-046 承接，不重复计数。
- 本批未修改业务代码、OpenAPI、DTO 或测试，未执行 CI；不把 RunResult 嵌套 Golden、DB migration、Worker 并发审计扩展到本轮。

### WebUI — 维度 2：全市场分页 API Client CTR-004 证据链复核

- 详细记录：`WebUI-维度2-完整性Review-第一轮.md` 第 27 节。固定代码审计基线仍为 `b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- **AUD-WEBUI-CTR-004 保留 P2**：`getMarketStocks` 已实现 page 1..10000、pageSize 1..100、includeMA 布尔序列化和 adjustment 白名单回退；Server/OpenAPI 已明确对应参数范围与默认值。
- 原始“缺少前端直接断言”仍成立，但需收窄：`market-stocks-lifecycle-browser.mjs` 已覆盖页面交互的翻页、每页数量和复权口径；`api.test.mjs` 仅覆盖 getMarketStocks 超时/AbortSignal，缺 API Client 级查询参数默认值、边界归一化、include_ma 序列化断言。没有证据证明正常页面交互存在运行时错误。
- 关闭建议：补 `getMarketStocks` 直接测试，覆盖默认值、正常参数、page/pageSize 越界、未知 adjustment、includeMA true/false，并断言 URL 与请求配置；明确 includeMA 只接受布尔值，或裁决字符串输入转换规则。
- **冻结影响有限**：不单独阻断 Market Stocks 服务端 OpenAPI/DTO 语义冻结，但 WebUI 消费者参数适配的测试证据仍不完整；维度 2 整体不能据此宣称消费者 Contract Test 全部齐备。
- 范围校正：AUD-WEBUI-CTR-001（行情 API Key 浏览器存储迁移）和 CTR-002（Analysis Owner-only 鉴权联调）属于 WebUI 维度 1，不在本轮维度 2 范围；不复核、不改状态，也不并入维度 2。上一轮将 CTR-001/002 列为本维度下一批任务是规划错误，已纠正。
- 本批仅更新审计文档；未修改业务代码、OpenAPI、DTO、测试，未执行 CI；无新增编号。维度 2 第一轮完整性 Review 仍为 partial。



## 6. 任务收敛与唯一执行索引

- 唯一任务矩阵：[`PLAT-002-差异唯一登记与剩余任务矩阵.md`](./PLAT-002-差异唯一登记与剩余任务矩阵.md)。
- 本矩阵将历史编号别名、撤销项、聚合 CTR、已完成冻结影响 Review、WebUI 维度 2 剩余批次和项目级阶段状态集中管理。
- 已完成初审的数据集不再重新初审；CTR-004、CTR-038～040、CTR-042～049 的既有冻结影响/证据链 Review 不重复执行。
- 下一批仅处理 WebUI 维度 2 的 Task/TaskEvent 一组：CTR-006/007/008，一次完成 DTO 空值、事件 Schema、分页与事件 ID 语义的关闭条件收敛。
- 本次只更新审计索引和状态，没有修改业务代码、OpenAPI、DTO 或测试，没有执行 CI；固定代码基线不变。


## 7. W2-A Task/TaskEvent 合并批次完成记录

- CTR-006：StartedAt/FinishedAt Go nil+omitempty 与 OpenAPI nullable 语义不一致，P2 保留。
- CTR-007：StageEvent 只有泛化 object 描述，缺少字段级 Schema，P2 保留。
- CTR-008：Handler/Queue 的 exclusive after 分页逻辑与默认/边界已核；数据库事件表 DDL/持久化约束及真实 DB 分页测试证据不足，P2 保留。
- 本批冻结影响 Review 已完成，问题尚未修复/裁决；不再重复初审，后续只做增量验收。
- 详细关闭条件见 `PLAT-002-差异唯一登记与剩余任务矩阵.md` 第 9 节及 WebUI 维度 2 Review 第 29 节。
- 下一批：DailyBar 查询窗口 CTR-009，随后继续维度 2 其余差异集中收敛。


## 8. W2-B DailyBar Bundle lookback_days 批次完成记录

- CTR-009 冻结影响 Review 已完成，P2保留。
- WebUI日K用户偏好会显式传入lookback_days；API Client省略选项时默认120；HTTP Handler省略参数后由Service使用环境配置默认值，默认配置120、最大值默认1000。
- OpenAPI只声明20..2000范围，没有说明省略时使用服务端配置。不可将120写成固定公共default。
- 关闭条件：OpenAPI说明配置化默认、Client URL默认/显式值测试、Handler/Service省略/显式值测试及配置边界测试。
- 本批只更新审计文档，没有修改业务代码、OpenAPI、DTO或测试，没有执行CI。
- 下一批：W2-C，基于现有证据集中归类CTR-011～037，不重做初审、不新增编号。


## 9. W2-C CTR-011～037 集中归类完成记录（2026-11-10）

- W2-C已基于既有切片完成一次性归类，不重新读取固定基线业务代码，不新增CTR。
- CTR-011～037均保持P2；没有依据升级P0/P1。它们不是同质冻结阻断项：Schema/能力/单位描述错误影响对应端点准确冻结；数据语义缺证要求限制下游解释；页面消费缺口需产品范围决议，不自动阻断服务端DTO冻结。
- CTR-015、019、020涉及的nullable字段语义应与CTR-010统一裁决，禁止重复计数字段级问题。
- 产品范围待决项：CTR-013、023、024、030、033、036；不因页面尚未消费而判定服务端API错误。
- 详细唯一归类表、冻结影响及关闭条件见 PLAT-002-差异唯一登记与剩余任务矩阵.md 第11节。
- 下一批：W2-D，准备CTR-010缺失/null/零值权威语义裁决问题；不改DTO/OpenAPI/Golden/业务代码。随后W2-E收尾WebUI维度2第一轮完整性Review，再回归十模块六维核验主线。
- 本批只更新审计文档，未修改业务代码、OpenAPI、DTO、测试，未执行CI；固定代码审计基线不变。


## 10. W2-D CTR-010 空值语义裁决准备完成记录（2026-11-10）

- 对照 GOV-CTR-001，确认项目级缺失/null/零值规则已标记 accepted：缺失仅表示字段不适用于 DTO 版本/请求；显式 null 表示字段存在但当前无值；required + nullable 必须输出 null；合法零值不得省略；unavailable 使用状态语义；nil slice、空数组、null、字段缺席、零值分别进入 Golden。
- CTR-010不能简单依据当前Go omitempty统一移除OpenAPI nullable。124字段逐项适用条件和无值原因仍缺领域证据，CTR-010保持开放。
- 逐字段裁决模板、当前可确认/不可确认边界见 PLAT-002-差异唯一登记与剩余任务矩阵.md 第12节。
- 下一批：W2-E，收尾WebUI维度2第一轮完整性Review，汇总覆盖、唯一编号、责任、关闭条件及遗留证据项；不重新初审代码、不修改业务实现。之后回归十模块六维核验主线。
- 本批只更新审计文档，未修改业务代码、OpenAPI、DTO、Golden、测试，未执行CI；固定代码审计基线不变。

## 11. W2-E WebUI维度2第一轮完整性Review收尾（2026-11-10）

- W2-E完成，状态为 `complete_with_open_items`。数据集覆盖、API Client/页面消费者映射、路由/OpenAPI入口、编号去重、唯一技术主责、协作方和遗留关闭条件均已登记。
- API盘点第5节的过期“待核数据集”状态已纠正；Hot Rankings、Market Sentiment History、Limit Pool、Dragon Tiger、Stock Search、Analysis Run、Provider Probe History均有既有审计章节/Review，不再重做初审。
- API Client仍为44个导出函数、41个远端入口、40个主要页面模块引用；`getMarketSentiment`是唯一未发现主要页面消费者的入口，保留产品范围待确认，不新增CTR。
- CTR去重口径：003→046、005→047；041 withdrawn；010为聚合CTR；038与048独立；001/002归维度1。无新增CTR。
- CTR-010逐字段适用条件/无值原因证据、其他活动CTR的裁决/实现/测试/产品决议仍开放。Review完成不等于修复、CI通过或契约冻结。
- 唯一主责模块及协作关系见 `PLAT-002-差异唯一登记与剩余任务矩阵.md` 第13.2节；完整性Review结论见该矩阵第13.3节和WebUI Review第33节。
- WebUI维度2停止全量重复Review；后续只按责任模块新提交、正式裁决或测试证据增量验收。
- 下一步回到阶段B十模块六维核验：先从当前总进度/唯一矩阵确认未完成模块与维度，再执行一个模块的一个未完成维度切片。固定代码基线不变，不检查Data新分支。
- 本批仅修改审计文档，未修改业务代码、OpenAPI、DTO、Golden或测试，未执行CI。
## 12. W2-E完成后的PLAT-002主线恢复点

- WebUI维度2第一轮完整性Review已完成（complete_with_open_items），不再作为当前执行批次；其开放CTR保留在唯一矩阵，后续仅增量验收。
- 阶段B仍未完成。十模块六维矩阵中，Platform的DTO/Schema/Golden仍为partial，Server该维度为not_started；本轮选择先推进Platform维度2，而不是继续扩展WebUI维度2。
- 下一批唯一任务：**Platform — 维度2：DTO、Schema、Golden 契约一致性核验（固定SHA）**。从Platform自身公开结构、序列化、Schema/Golden和直接消费者映射开始，先盘点已存在的权威契约/测试资产，再选一个边界明确的DTO切片核验。重点判断Fault、Diagnostics、Request ID、HTTP Client配置/结果等Platform公共原语是否有唯一Schema与稳定序列化约束；不将领域模块DTO混入Platform范围，不重复维度1 Port/调用点审查，不修改Platform实现。
- 本轮下一任务的完成产物应为：一个明确DTO切片的生产方字段/JSON tag、Schema、Golden/Contract Test、直接消费者证据及差异登记；如不存在适用的外部Schema，需说明内部Go API的契约测试方式，不为形式完整而新增OpenAPI。
- 固定代码审计基线继续为 `b6dfc85ba214fe16268c8cba780641a8bfe15086`；只维护 `feat/design_doc` 审计文档，不查看Data新分支、不合并代码、不改业务实现、不执行CI，除非后续证据表明静态审查无法判定且需明确批准执行。
## 13. Platform 维度2：Diagnostics Observation DTO 首批切片（2026-11-10）

- 审计记录：`Platform-维度2-DTO-Schema-Golden.md`；固定代码基线 `b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- 本批仅核 `internal/platform/diagnostics.Observation`：Go DTO/Validator、PLAT-CTR-004 JSON Schema、两份 Golden、Go Contract Test，以及 News/Research 模块级生产方和 Server Diagnostics/Health Handler 消费路径。
- 发现四项证据缺口：Go字节长度与Schema字符长度不一致；safe_message/attributes key空白语义不一致；attributes总字节预算未在Schema表达；Go Golden测试未证明Schema与Go对同一边界样例双重校验。全部归入PLAT-CTR-004，不新增CTR。
- PLAT-CTR-004继续 `pending_evidence`。Observation切片为partial；Platform维度2整体仍为partial。Server完整Diagnostics Report Schema/HTTP Golden归Server维度2核验。
- 下一批：Platform维度2核对Fault内部对象及其领域/Transport映射边界。不得扩大到Server完整HTTP Error Envelope，不修改实现。
- 本批仅更新审计文档，未修改业务代码、Schema、Golden、测试，未执行CI。
## 14. Platform 维度2：Fault 内部对象与映射边界第二切片（2026-11-10）

- 审计记录：`Platform-维度2-DTO-Schema-Golden.md` 第7节；固定基线 `b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- 已核 `Fault` 的Class/字段/JSON隐藏/Validate/Unwrap、Platform HTTP Client缓冲与流式Fault构造、Diagnostics Runner归一化，以及LLM `ModelErrorFromPlatformFault` 直接消费映射和测试。
- Fault本身是进程内对象，全部字段 `json:"-"`，直接序列化为 `{}`，不应设计公开Schema/Golden。Platform Runner有Validate覆盖；LLM独立Mapper没有Validate，需补非法元数据映射边界测试/信任边界说明。
- PLAT2-D2-005并入既有P002-D02 / PLAT-CTR-001，不新增CTR。LLM Owner负责消费者侧补证；P002-D02和PLAT-CTR-001保持开放/pending_evidence。
- Market Data/Platform HTTP独立输出HTTP Error DTO，未发现直接消费Fault的证据；Server完整HTTP Error Envelope留在Server维度2。
- 本切片结论：Fault内部契约基本齐备，跨模块映射partial；Platform维度2仍partial。
- 下一切片：Platform维度2核对Request ID公共原语与直接Transport消费者，不重审Fault/Observation，不扩大到Server完整HTTP Error Envelope。
- 本轮仅修改审计文档，未修改业务代码、Schema、Golden、测试，未执行CI。
## 15. Platform 维度2：Request ID 原语与传播边界第三切片（2026-11-10）

- 审计记录：`Platform-维度2-DTO-Schema-Golden.md` 第8节；固定基线 `b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- Request ID格式、随机生成、默认不信任入口、显式可信Gateway选项、Context存取、出站Header覆盖/删除有实现、Golden和本地Contract Test；缓冲/流式HTTP Client均传播Context ID。
- Root Server Bootstrap外层使用零值安全 `MiddlewareWithOptions`；内层旧Analysis Handler仍保留Deprecated兼容Middleware，但当前组合根外层先重建ID。独立暴露旧Handler的路径仍需满足可信Gateway约束。
- Research Correlation ID使用独立Context Key，不继承Request ID；GOV-CTR-003的异步/事件ID完整链路本轮不审、不关闭。
- PLAT2-D2-006：缺少Root Server入口到出站HTTP Client的端到端组合测试，属集成证据缺口。P002-D03继续开放，待Server/Integration补部署入口和兼容Handler迁移/隔离证据；不新增CTR。
- 本切片结论：Platform Request ID原语和局部消费者测试基本齐备，组合链路partial；PLAT-CTR-002仍不能标记frozen，GOV-CTR-003保持独立pending_evidence。
- 下一切片：Platform维度2核对HTTP Client配置、DestinationPolicy、缓冲/流式结果元数据及PLAT-CTR-005 Golden/Contract Test；不重审SSRF算法、不审Server LLM Transport业务映射。
- 本轮仅更新审计文档，未修改业务代码、Schema、Golden、测试，未执行CI。
## 16. Platform 维度2：HTTP Client配置与结果契约第四切片（2026-11-10）

- 审计记录：`Platform-维度2-DTO-Schema-Golden.md` 第9节；固定基线 `b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- 已核Config缓冲/流式默认值和上限、Dependency并发隔离、配置复制、DestinationPolicy/BoundDoer、缓冲响应资源所有权、StreamResponse/StreamStats和News/LLM消费者适配。
- A1流式专项验收矩阵21项有对应测试覆盖；本轮未重新运行CI。未发现已证实的HTTP Client实现错误。
- PLAT2-D2-007：MaxResponseHeaderBytes/流式默认值与StreamDependencyLimits配置边界测试不完整，归入PLAT-CTR-005，不新增CTR。
- PLAT2-D2-008：LLM Transport未向LLM窄接口暴露StreamStats，需确认A2是否要求领域层采集Platform传输终态/字节数/耗时；条件性缺口，不预设新增Platform API。
- PLAT-CTR-005是内部Go API契约，JSON Schema/Golden不适用；项目登记簿仍需明确不适用理由和Go Contract Test/消费者测试替代证据。PLAT-CTR-005仍pending_evidence，A2真实消费者联调和A3生产部署验收未完成。
- 本轮HTTP Client切片完成后不立即转Server维度2。项目Platform契约目录仍有PLAT-CTR-003（身份/授权）、PLAT-CTR-006（Storage/Migration）、PLAT-CTR-007（Typed Config/Secret）未完成维度2适用性核验。下一切片先核PLAT-CTR-003身份/授权结构、Header/Context、序列化适用性和直接消费者；权限算法/授权语义留在维度4。完成Platform维度2剩余契约范围核对后，再转Server维度2。
- 本轮仅修改审计文档，未修改业务代码、Schema、Golden、测试，未执行CI。

## 17. Platform 维度2：PLAT-CTR-003 身份、授权与 Secret 边界（2026-11-10）

- 审计记录：`Platform-维度2-DTO-Schema-Golden.md` 第10节；固定基线 `b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- PLAT-CTR-003原始批准范围为anonymous、owner Bearer、service Bearer。Platform A1原语已实现：凭证摘要与常量时间比较、Token格式/长度、固定Scope白名单、Owner/Service隔离、Principal Context快照隔离、显式RoutePolicy和默认拒绝；Secret脱敏有测试。
- Server Bootstrap仅以OwnerToken构造Authenticator，ServiceCredential列表为空；当前Router只有匿名/Owner路由，没有Service路由。Server Service身份生产接入未证明，不能仅凭Platform包级能力标记契约frozen。
- Principal/RoutePolicy是进程内结构，不适用JSON Schema/Golden；以Go Contract Test、Router与Context消费者测试替代。Secret不进入Golden。
- Owner Web Session虽有Platform原语，但Server A2未接入；不属于当前PLAT-CTR-003原始Bearer范围。SessionRecord是Redis内部JSON记录，schema_version/Golden证据缺口归既有P002-D12～D14/Session扩展验收，不新增CTR。
- 未完成全仓Principal/Scope消费者扫描；领域资源授权和AuthorizationRef归维度4。
- 结论：Platform Bearer A1基本符合，Server Service装配和部署Secret证据仍待补；PLAT-CTR-003保持pending_evidence。仅更新审计文档，无业务代码/Schema/Golden/测试/CI变更。
- 下一步唯一任务：**Platform 维度2 — PLAT-CTR-006 Storage、Repository、Migration 契约的DTO/机器证据适用性核验**。不重审Storage连接算法与Migration执行正确性（维度5），固定代码基线不变。

## 18. Platform 维度2：PLAT-CTR-006 Storage/Repository/Migration（2026-11-10）

- 审计记录：`Platform-维度2-DTO-Schema-Golden.md` 第11节；固定基线 `b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- Platform SQLConfig/Migration和Server Registry输入为进程内Go API，不适用公开JSON Schema/Golden；RunReport仅用于Bootstrap日志，未发现HTTP公开输出。
- Migration字段与机器约束已有Platform/Server测试；仍存在注册阶段边界验证时机差异（PLAT2-D2-009）、Description不进入checksum的不可变语义待裁决（PLAT2-D2-010）。
- 固定基线有News/Research/Researchflow/Notification四个模块Migration生产方；Root Server Registry只注册News/Research/Notification，遗漏Researchflow（PLAT2-D2-011）。这是明确的消费者接线缺口，待Server责任模块修复，维度5验收启动生命周期。
- 真实MariaDB/Redis集成测试受环境变量门控，本轮未运行；A2/L3资源与Migration运行证据留维度5。
- 结论：PLAT-CTR-006维度2适用性已完成首轮核验，但整体契约保持partial / pending_evidence。仅审计文档更新，无业务代码/Schema/Golden/测试/CI变更。
- 下一步唯一任务：**Platform维度2 — PLAT-CTR-007 Typed Config/Secret字段、脱敏、消费者映射及机器测试适用性核验**。固定基线不变；不重审本轮Migration算法和生命周期。


## 14. Platform 维度2首轮收尾与Server维度2接续（2026-11-10）

- 固定代码审计基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`；维护分支：`feat/design_doc`。
- 本轮完成Platform维度2最后一项 PLAT-CTR-007 Typed Config/Secret：Platform Env/Secret基础API及脱敏测试已核；Server Process、News、Market Data Adapter、Storage Adapter配置消费者已建立映射；旧Market Data/Python API环境读取入口登记待核。
- 新差异PLAT2-D2-012～014均并入PLAT-CTR-007，不新增CTR；其中Secret类型保护丢失属于消费者安全封装风险，尚无实际日志泄漏证据；兼容环境读取入口的生产活跃状态需由Server/迁移维度核实。
- Config/Secret为启动时进程内配置API，不适用公开HTTP JSON Schema/Golden；以Go Config Contract Test、Bootstrap注入、消费者配置输出脱敏和快照隔离测试作为替代机器证据。
- PLAT-CTR-007保持pending_evidence。Platform维度2的PLAT-CTR-001～007均已完成首轮适用性切片，但多个契约仍有跨模块映射/测试/部署证据缺口；不得把“首轮覆盖完成”表述为“Platform契约冻结”。
- 本轮未修改业务代码、Schema、Golden或测试，未执行CI；仅更新Platform维度2审计记录、差异唯一矩阵和本总进度。
- **下一步唯一任务：Server — 维度2：DTO、Schema、Golden。** 审核Root Server公开HTTP请求/响应DTO、Handler、OpenAPI、Golden及真实WebUI/内部消费者映射。先完成Server维度2首轮切片，不重做Platform、不提前进入Server其他五个维度；Data继续使用固定基线，不检查Data新分支。


## 15. Server 维度2首轮切片（2026-11-10）

- 固定代码审计基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`；维护分支：`feat/design_doc`。
- 本批范围：Root Server Health/Readiness/Diagnostics、Root实际路由、旧Analysis Run/Task HTTP Handler、Analysis OpenAPI、相关Go测试和WebUI基础API消费。
- 已核对Root Router实际13条method-pattern；Health Handler真实响应包含healthz、readyz成功/失败、diagnostics成功/失败及HEAD行为。Platform Observation Schema不覆盖Server新增的Readiness/Report复合包装。
- Analysis OpenAPI只覆盖Task创建/查询/Events，未覆盖同步Run创建/读取/Events和Root Health/Diagnostics。AnalysisRequest Schema未完整列出domain.RunRequest字段；StageEvent仍是泛化object；Task时间指针omitempty与nullable差异已由既有CTR-006承接。
- Root错误响应存在Middleware publicError（class/code/message/retryable）与旧Analysis/Health Handler error（code/message）两种结构；错误信息已有部分脱敏测试，统一策略与全路由契约证据仍待完成，归PLAT-CTR-001。
- WebUI `web/src/api.js`消费/healthz、/readyz及error.message/code，Request ID从X-Request-ID响应Header读取；未发现该文件消费/diagnostics。
- 新增审计子观察SRV2-D2-001，归既有SRV-CTR-001（Root HTTP契约）；嵌入Observation仍归PLAT-CTR-004。CTR-006/007/046/047及PLAT-CTR-001均沿用既有编号，没有新增CTR。
- Server维度2首轮仍为 `in_progress / partial`；本批只完成一个切片，不代表整个Server维度2完成或冻结。
- 本轮仅修改审计文档，未修改业务代码、OpenAPI、DTO、Golden、测试，未执行CI。
- **下一步唯一任务：Server维度2的增量路由/消费者盘点与错误DTO契约补证。** 先以固定SHA确认Root Server是否还注册模块Handler或其他HTTP Transport；若没有，停止扩大路由范围，转而将现有Root错误Envelope差异、WebUI消费和CTR-046/047的Schema关闭条件做交叉核对。不得重复本批Health/Analysis代码初审，不进入Server其他五个维度。


## 16. Server 维度2第二切片：HTTP错误 DTO 与消费者契约交叉核验（2026-11-10）

- 固定代码基线：b6dfc85ba214fe16268c8cba780641a8bfe15086；维护分支：feat/design_doc。
- Root Router仍只有既有13条method-pattern，没有发现News/Research/Researchflow/Quant/LLM/Notification等模块Handler新增注册。
- 错误生产方核验确认Middleware/Fault Envelope与Security/Health/旧Analysis Legacy Envelope并存；错误码在部分同状态场景也存在分叉；OpenAPI没有统一Error Schema。
- WebUI消费error.message/code/status/requestId，不消费class/retryable；Request ID实际Body优先、Header其次，纠正了上一轮“Header优先”的文档表述。
- 统一Envelope是建议而非已批准决议；必须先明确端点范围、class/retryable语义、状态码/code映射、超限错误码、Request ID权威通道和OpenAPI/Golden要求。
- CTR-046/047没有重新登记；错误DTO归PLAT-CTR-001，Root路由/运维契约归SRV-CTR-001，Request ID信任与传播归PLAT-CTR-002。
- Server维度2第二切片完成，状态complete_with_open_items；维度2整体仍partial，项目阶段B十模块六维核验继续进行。
- 本批仅更新审计文档，无业务代码、OpenAPI、DTO、Golden、测试或CI变更。
- 下一步唯一任务：**Server维度2 — Run/Task DTO字段与现有OpenAPI Schema差异的增量核验**。只以CTR-046/047、CTR-006/007已有登记及既有代码证据为入口，查明真实JSON字段、必填/可空/省略语义与Schema差异是否已被完整登记；不重做错误Envelope、Health/Readiness、WebUI错误解析，也不检查Data新分支。


## 17. Server 错误DTO切片发现的 Request ID 组合链路风险（2026-11-10）

- 补充核对Root与旧Analysis Handler的嵌套Middleware：外层MiddlewareWithOptions只重建Context/响应Header，不清除原始请求Header；旧Analysis内层兼容Middleware仍信任X-Request-ID，可能再次覆盖Context和响应Header。
- 该风险归既有PLAT-CTR-002 / P002-D03，不新增CTR；需要Server组合测试和内层兼容Middleware隔离证据。此前“Root外层重建即可保证内层不再信任调用方ID”的判断需纠正。
- 本发现仅登记，不修改Platform/Server实现；不改变当前Server维度2错误Envelope切片的范围和结论。


## 18. Server 维度2第三切片：Run/Task DTO字段与OpenAPI增量核验（2026-11-10）

- 固定代码基线仍为 b6dfc85ba214fe16268c8cba780641a8bfe15086；维护分支仍为 feat/design_doc。
- RunRequest字段与OpenAPI AnalysisRequest逐项核对：OpenAPI缺run_id、as_of_mode、parameters，无required和TaskType枚举；Handler补默认值、覆盖as_of_mode，historical_replay要求显式as_of。CTR-047保持开放，未重复登记。
- CTR-046：同步Run创建/读取/Events路由及RunResult响应Schema仍缺；RunResult是多层嵌套DTO，本批不展开全部领域结构。
- CTR-006：Task.started_at/finished_at的指针omitempty导致nil时省略；OpenAPI nullable=true语义不一致，需权威决议并补三方Golden。
- CTR-007：StageEvent真实字段、可选字段类型和status枚举已核实；OpenAPI TaskEvent.event仍为泛化object，缺字段级Schema。
- Task.status实际输出queued/running/succeeded/partial/failed，与domain.RunStatus不同，不可直接复用领域枚举。
- 旧Analysis taskqueue对应DDL本轮未在固定基线文件树中定位；不据此推断生产表不存在，CTR-008按原责任和关闭条件处理。
- 本切片complete_with_open_items；没有CTR关闭或新增。无业务代码、OpenAPI、DTO、Golden、测试、CI变更。
- 下一步唯一任务：Server维度2第四切片——既有Run/Task Schema差异的关闭准备与证据缺口归并Review。仅检查CTR-046/047/006/007本轮字段结论是否完整、责任和决策输入是否明确、是否存在重复登记；输出最小决策清单和可交付关闭顺序。不重做字段初审、不展开RunResult全部领域子结构、不进入Server其他五维或其他模块。



## 19. Server维度2第四切片：Run/Task权威契约与责任边界Review（2026-11-10）

- 固定代码审计基线仍为`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`；审计文档维护分支仍为`feat/design_doc`。
- 对照项目级契约登记簿、Researchflow最终决议和Application Port补充文档，确认Server是唯一Composition Root；Researchflow是Run/Stage/Attempt及跨模块流程编排的领域Owner；Server负责HTTP路由/适配，不拥有Researchflow领域DTO语义。
- 发现并明确两套不能混用的契约：旧Analysis RunRequest/RunResult/StageEvent + taskqueue Task/Event；Researchflow publicdto Run/Stage/Attempt/ResultSummary。旧StageEvent不是Researchflow Stage DTO。
- Researchflow已有独立publicdto实现、run/stage/attempt/result-summary Schema、对应Golden及Schema投影测试代码；FLOW-CTR-001/002设计决议为accepted，但定向Go Test/Race/Vet执行证据仍缺，不能标记frozen。
- CTR-046：需要项目负责人确认旧Analysis同步Run/Task/Events API保留还是迁移到Researchflow API，再由Server补最终路由/OpenAPI和真实组合测试。
- CTR-047：Researchflow CreateRunCommand规则不得机械套用到旧Analysis RunRequest；旧Analysis仍需明确同步run_id、异步Task run_id覆盖、未知TaskType处理和symbols上限是否强制。
- CTR-006：按现有Task序列化行为，nil started_at/finished_at应省略；默认只修正OpenAPI nullable语义，不改运行时DTO，并补queued/running/terminal Golden。
- CTR-007：只针对保留的旧Analysis Events端点定义StageEvent Schema/Golden；不得复用Researchflow Stage Schema。
- CTR-008的DDL/cursor持久化证据不属于本轮，不重审。
- 本轮只更新三份审计文档；未修改业务代码、OpenAPI、DTO、Schema、Golden、测试，未执行CI。无新增CTR、无CTR关闭。
- Server维度2仍为`in_progress / partial`，第四切片状态`complete_with_open_items`。
- **下一步唯一任务：Server维度2决策输入收敛**——整理CTR-046/047/007所需的最小项目负责人决策项，按已有事实给出推荐默认方案与兼容影响；CTR-006按既有序列化事实准备直接Schema修正验收项。仍不进入Server其他五个维度、不审查Data新分支、不扩展到其他模块。



## 20. Server维度2第五切片：项目级决策输入收敛（2026-11-10）

- 固定代码审计基线仍为`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`；维护分支仍为`feat/design_doc`。
- 本轮只读取固定基线权威契约、旧Analysis Handler/Task Handler/DTO/OpenAPI及Researchflow决议；没有检查Data新分支，也没有扩大模块审计范围。
- 形成三项待负责人裁决提案：
  - CTR-046 / D-SRV-API-01：旧Analysis Run/Task/Events路径兼容保留；Researchflow API独立版本化；旧路径不得静默切换语义。旧Analysis请求缺少Researchflow CreateRunCommand必填字段，不能直接映射。
  - CTR-047 / D-SRV-REQ-01：建议未知非空TaskType拒绝、symbols的100项OpenAPI限制在Handler落实、异步Task run_id由服务端生成且拒绝非空客户端值；同步Run run_id先补Runner实现证据再裁决。
  - CTR-007 / D-SRV-EVT-01：建议旧Analysis Events兼容保留并定义独立StageEvent；Researchflow事件流如建设则独立版本化。
- CTR-006 / D-SRV-TIME-01具备直接实施条件：保持started_at/finished_at nil时省略，修正OpenAPI nullable语义，补queued/running/terminal Golden；本轮没有擅自修改Schema。
- 关键边界：Researchflow CreateRunCommand中的system_as_of、deadline_at、template_ref/version、input_snapshot_refs、idempotency_key等语义不能从旧Analysis RunRequest猜测或自动映射；as_of不等于system_as_of。
- 本轮决策仅为建议，未被项目负责人确认；CTR-046/047/007保持pending_decision，CTR-006为ready_for_implementation但仍未验收。无新增CTR、无CTR关闭。
- 本轮只更新三份审计文档，无业务代码、OpenAPI、DTO、Schema、Golden、测试变更，未执行CI。
- Server维度2整体仍为`in_progress / partial`。第五切片状态为`complete_with_open_items`。
- **下一步唯一任务：按已形成的决策提案进行项目级裁决输入核对；在用户/项目负责人未确认前，不把建议当成accepted，也不擅自实施CTR-046/047/007。可独立推进的唯一实现项是CTR-006的Schema/Golden修正，但应先确认当前工作授权是否允许修改审计基线以外的业务契约文件；若只允许审计分支，则继续完善验收工单，不碰业务文件。** 仍不进入Server其他维度、不切换模块、不审查Data新分支。



## 21. PLAT-002执行方式收敛：停止无限拆分审计切片（2026-11-10）

### 本轮Review结论

- 现有唯一差异矩阵已覆盖CTR编号至049，并明确CTR-003/005历史承接、CTR-041撤销、CTR-010聚合及CTR-038/048独立计数规则。
- WebUI维度2第一轮完整性Review已有complete_with_open_items结论；不得再对其数据集和API Client做相同全量初审。
- Platform维度2已有七项契约适用性核验；Server维度2已有五个切片。二者后续均应围绕原差异的修复/决议/证据做增量验收，不再增加重复盘点。
- 当前审计材料存在历史章节中的“下一切片”文字与后续实际进展并存的情况；自本节起，历史“下一步”仅作为当时记录，不再作为当前执行依据。当前唯一批次入口是差异矩阵第27节。
- 不把全部P2一律认定为项目整体冻结阻断。按契约边界局部冻结；P0/P1未解决或未批准延期时，不允许整体冻结。
- 审计分支只维护证据与计划；业务代码、OpenAPI、Schema、Golden、测试由责任模块分支修复，再按新SHA增量验收。

### 有限执行批次

1. **B0 安全与P1阻断**：WebUI认证/迁移门禁、Platform/Server组合根和授权边界P1候选。
2. **B1 项目决策集中处理**：CTR-046/047/007、CTR-038/039、CTR-042/044、CTR-048/049、CTR-010字段适用性及产品范围待决项。一次形成权威决议，不逐CTR开轮。
3. **B2 契约/Schema修正**：同一API/DTO批量修正，补Golden和Contract Test；CTR-010只接收领域责任方逐字段映射，不在审计分支猜测。
4. **B3 业务语义与消费者修复**：按数据集批量处理算法、完整性、PIT、质量、单位、分页和UI消费问题。
5. **B4 集成验收与冻结**：真实DB/Root/跨模块测试、精确SHA CI；满足条件的契约分批冻结，其余明确deferred或继续阻断。

详细编号、责任和退出条件以差异矩阵第27节为准。

### 当前第一执行批次

仍然聚焦Server维度2差异关闭，不再新增审计切片：
- CTR-006：责任分支修正Task时间字段Schema并补queued/running/terminal Golden和Handler Contract Test；保持Go DTO当前nil省略行为。
- CTR-047：只补同步Runner run_id的真实执行/持久化/查询证据；不重做RunRequest字段盘点。
- CTR-046/007：集中确认旧Analysis API与Events兼容保留边界，再一次性补保留端点的OpenAPI/Schema/Golden/Root Contract Test。
- CTR-008：只补真实DDL、事件ID唯一单调和DB分页证据，不重审已核过的Handler cursor算法。
- 上述项目完成后，Server维度2统一执行定向测试和CI；按原CTR关闭，不新增编号。

### 本轮状态与边界

- 本轮仅完成唯一差异矩阵与有限执行批次Review；没有重读业务代码，没有改变任何CTR状态。
- 固定审计基线仍为`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`；文档维护分支仍为`feat/design_doc`。
- 本轮仅更新审计文档，无业务代码、OpenAPI、DTO、Schema、Golden、测试或CI变化。
- PLAT-002仍处于阶段B/D/F推进中；C/G/H未完成，整体未冻结。

### 下一步唯一任务

**按差异矩阵第27.3节，进入Server维度2关闭批次。先在固定基线中只补CTR-047同步Runner run_id缺失证据，同时确认责任分支当前是否已有CTR-006修复提交；若没有修复提交，不在审计分支代改，输出一次性可执行验收任务给Server责任分支。** 不重新审查DTO字段、不再写重复切片总结、不改变固定基线、不检查Data新分支。



## 22. CTR-047 同步Run ID证据增量（2026-11-10）

- 本轮按第21节有限批次计划，只补CTR-047同步Runner run_id这一项缺失证据，没有重做RunRequest字段审计。
- 固定基线证据确认：Runner保留合法非空客户端run_id、空值时生成ID、非法值拒绝；Store.Begin对analysis_runs主键执行INSERT，Finish/GetResult按同一ID持久化和读取；重复ID不会覆盖已有Run。
- 缺口仅保留为：重复ID的HTTP稳定409映射、Runner显式ID/非法ID/重复ID测试、真实MariaDB冲突集成证据。同步Run建议契约已可裁决为“客户端合法ID可保留、缺省服务端生成、重复ID拒绝”；异步Task仍由Queue生成权威run_id。
- CTR-047整体保持开放；同步ID行为从未知事实变为ready_for_decision，不标记关闭。
- 本轮只更新审计文档；无业务代码、OpenAPI、DTO、Schema、Golden、测试变更，未执行CI。
- 下一步唯一任务：在不改变固定审计基线的前提下，确认CTR-006是否已有责任分支修复提交；若无，基于现有结论形成一次性修复/验收任务，等待明确指定责任分支后实施。不要继续拆Server维度2审查切片，不重新读取RunRequest/Task字段，不检查Data新分支。


## 当前执行状态更新：CTR-006修复工单已收敛（2026-11-10）

- 本轮按PLAT-002有限执行清单第27节推进CTR-006，没有重做Task DTO字段审计。
- 固定基线唯一契约文件已定位为`doc/api/contracts/分析任务.openapi.yaml`；started_at/finished_at当前nullable与Go指针+omitempty的nil省略行为不一致。
- 已在差异唯一矩阵第28节形成一次性修复/验收工单：只移除两个字段的nullable、保持可缺失、补queued/running/terminal Golden及Schema/Handler Contract Test；不修改Go DTO。
- 本轮没有检查或修改其他责任分支，没有业务代码、OpenAPI、Golden、测试或CI变更。责任分支尚未指定，因此CTR-006仍为ready_for_implementation，未关闭、未冻结。
- 审计维护分支：feat/design_doc；固定代码审计基线：b6dfc85ba214fe16268c8cba780641a8bfe15086。
- **下一步唯一任务：取得CTR-006责任模块分支及修复提交SHA，直接按差异矩阵第28.4节做增量验收。** 不再重审字段事实；如尚无责任分支/修复提交，则把该工单交由Server/旧Analysis契约责任方执行，不在审计分支代改。
- CTR-006通过后，回到第27.3节，集中处理CTR-046/047/007的项目级决策，不另开重复审计切片；随后按B2、B4完成契约修复、跨模块验收和分批冻结。


## 23. Data→Researchflow 跨模块输入边界审查（2026-11-10）

- 固定代码审计基线保持 `origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`；维护分支为 `feat/design_doc`。
- 本轮完成 Data→Researchflow 输入边界初审，详细差异唯一登记见矩阵第27节。
- 已确认：Researchflow 只持有不透明 `input_snapshot_refs` 并固定传播 Run 的 `system_as_of`；Data 拥有 Snapshot Manifest、来源版本和 AuthorizationRef。无需新增行情 DTO 或改变依赖方向。
- 记录差异：真实 Stage Adapter 尚未验证 Manifest.system_as_of 与 Run.system_as_of 一致；Stage 级 Snapshot 最小权限范围未明确；可信 Principal/Scope、purpose_code、Manifest 与 AuthorizationRef 的实际映射缺少 Adapter 证据；当前未发现生产 Data Stage Adapter。
- 状态：职责/依赖方向 `freeze-ready`；Run 上下文语义 `accepted`；Stage 级资源范围为条件性 `pending_decision`；真实 Data 消费交互 `pending_evidence`。
- 沿用 DATA-CTR-005/008、FLOW-CTR-003/004，不新增 CTR；不修改 Data、Researchflow、Server 责任模块代码，不执行 CI。
- 下一步唯一审查边界：**News→Researchflow**，只核对 News Evidence Query/Cursor/授权/PIT 引用如何进入 Run/Stage；不得重复 News→Research 已完成的首轮检查，除非发现 Researchflow 消费路径有独立差异。


## 24. News→Researchflow 跨模块消费边界审查（2026-11-10）

- 固定代码审计基线保持 `origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`；维护分支为 `feat/design_doc`。
- 本轮完成 News→Researchflow 输入生命周期审查，详细差异唯一登记见矩阵第28节。
- 已确认：News ListEvidence 的 `snapshot_ref` 固定分页结果集，Cursor 仅用于同一快照续页；Researchflow 的 `input_snapshot_refs` 在 CreateRun 时固定并由 Stage/Attempt/重试继承；News 每次查询必须接收 Run 的原始 `system_as_of`，授权 Owner 每次读取仍需复核当前策略。
- 关键差异：News 分页 Snapshot/Cursor TTL 仅15分钟，不能直接作为长生命周期 Run 输入；News 首屏生成 snapshot_ref 的时点与 CreateRun 先固定 input_snapshot_refs 的顺序未定义；Researchflow authenticated_caller_ref 到 News caller_ref 的主体/委托映射，以及 purpose/content level/filters 的版本化固定方式未明确；真实 News Consumer Adapter、Server 装配和联合测试缺证据。
- 状态：职责/依赖方向 `freeze-ready`；PIT 时间传播原则 `accepted`；快照生命周期与主体/查询条件映射 `pending_decision`；真实交互 `pending_evidence`。
- 沿用 NEWS-CTR-001/002/003、FLOW-CTR-003/004、GOV-CTR-002/006，不新增 CTR；NEWS→Research 已登记问题不重复登记。
- 本轮未修改业务代码、Schema、Golden、测试，未执行 CI。
- **下一步唯一任务：转入 Quant→Researchflow 边界审查**，只核对 QuantResult/Manifest/AuthorizationRef 与 Run/Stage 输入引用、system_as_of、结果版本和重试固定关系；不得重复 Quant DTO/指标算法审查。


## 25. Quant→Researchflow 结果消费边界审查（2026-11-10）

- 固定代码审计基线保持 origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086；维护分支 feat/design_doc。
- 本轮完成 Quant→Researchflow 结果消费边界初审，详细差异唯一登记见矩阵第29节。
- 已确认：Quant 拥有不可变 Result Manifest、结果 payload 和 AuthorizationRef；Researchflow 拥有 Run/Stage/Attempt 执行关联和结果综合。Manifest 绑定 result_type/schema_version/system_as_of/input_snapshot_refs/result_fingerprint；AuthorizationRef.resource_ref 绑定 result_id，Quant 每次读取须重新授权。
- 核心未决：QuantResultRef 与完整 Envelope 的关系及 GetResult 返回边界未明确，但 FLOW-006 已按完整 Envelope 消费；Quant 结果到 Researchflow Stage output_ref 的字段投影尚未定义。记录为 QNT-FLOW-A/E pending_decision。
- 证据缺口：强类型 Payload Schema/Golden 未覆盖所有实际 discriminator；真实 Quant Facade 当前授权复核、Manifest/Run 输入对齐、Consumer Adapter、Server 装配和联合 Contract Test 未完成。记录为 QNT-FLOW-B/C/D/F pending_evidence。
- 状态：职责/依赖方向 freeze-ready；Manifest/授权绑定原则 accepted；Ref/Envelope/Stage output_ref 投影 pending_decision；生产消费链路 pending_evidence。
- 沿用 QNT-CTR-001～005、FLOW-CTR-003/004、GOV-CTR-002/006，不新增 CTR；不修改 Quant、Researchflow、Server 责任模块代码，不执行 CI。
- 下一步唯一任务：转入 Research→Researchflow 边界审查，只核对 Research Context/Evidence/Claim 的公开读取与写入、来源授权复核、PIT、Stage 输出引用及重试幂等；不得重做 Data→Research 已审结的来源字段和 PIT 语义盘点。


## 26. Research→Researchflow 研究对象消费边界审查（2026-11-10）

- 固定审计基线：origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086；维护分支 feat/design_doc。详细差异唯一登记见矩阵第30节。
- 已确认：Research 拥有 Context/Evidence/Claim 语义与版本；Researchflow 负责 Run/Stage/Attempt、分区选择、预算裁剪和组合。FLOW-005 的 ToResearchCreateContextCommand 是纯 DTO 投影，不创建对象、不读取正文、不做授权；Context.system_as_of 必须等于 Run，Evidence.system_as_of 可早于 Run 但不得晚于 Run。
- 关键待决：CreateContext/CreateClaim 等写操作由 Research 生成 ID 且没有调用幂等键；Attempt 超时/响应丢失后重试可能重复创建对象。Context Get 无 caller/purpose 授权边界；按显式 EvidenceRef/ClaimRef 加载的批量/失败策略也未定义。记录 RSC-FLOW-A/B/C/E pending_decision。
- 证据缺口：Claim 的 supporting/contradicting EvidenceRefs、ScopeRefs 真实引用解析未完成；Claim Get 无来源 Owner 授权复核；生产 Research Facade Adapter、Server/Worker 装配和联合 Contract Test 未形成。记录 RSC-FLOW-D/F pending_evidence。
- 状态：职责/依赖方向 freeze-ready；Context 映射与 Run 时间传播 accepted；元数据访问、写入幂等、固定引用加载策略 pending_decision；真实引用/授权/生产消费链路 pending_evidence。
- 沿用 RSC-CTR-001/002、FLOW-CTR-003/004、GOV-CTR-003/005/006，不新增 CTR；不修改责任模块代码，不执行 CI。
- 下一步：进入 Researchflow→LLM 边界审查，核对 ModelRequest/Response、Template 版本、Prompt/工具 Schema、调用身份/Scope、模型重试和结果引用；不重复已审的 Tool Policy 领域规则，只核对跨模块映射和生产 Adapter 证据。


## 27. Researchflow→LLM 模型调用与工具往返边界审查（2026-11-10）

- 固定审计基线：origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086；维护分支 feat/design_doc。详细差异唯一登记见矩阵第31节。
- 已确认：Researchflow 编排模型调用和工具；LLM 只执行单次 Generate/Stream 协议适配，不授权、不执行工具、不决定业务重试。Run/Stage 上下文继承 system_as_of/deadline/input refs/template version/correlation_id；每个 Attempt 独立 request_id。ToolPolicy 已实现模型 ToolCall → Intent → Scope/Schema/预算授权的模块内路径。
- 关键待决：StageExecutionRequest 到 LLM ModelRequest 的 provider/model/purpose/prompt 配置及 Prompt 版本审计映射未定义（LLM-FLOW-A）；LLM 只支持单轮文本，却提供 ToolCall 意图，工具执行后如何回传模型的 continuation 未定义（LLM-FLOW-B）；LLM ambiguous outcome 与 Researchflow Retry ReasonCode 的映射未定义（LLM-FLOW-C）。均为 pending_decision。
- 证据缺口：Tool Executor 尚无生产绑定 Run/Stage/Attempt/fencing、持久化幂等和 ambiguous outcome 对账证据（LLM-FLOW-D）；真实 Gateway/Model 能力 E2E、LLM Stage Adapter、Server DI/Secret 注入、Platform Stream 包装和联合 Contract Test 未完成（LLM-FLOW-E/F pending_evidence）。
- 状态：职责/依赖方向 freeze-ready；基础调用上下文语义 accepted；ModelRequest/Prompt 映射、Tool Call 往返、重试错误分类 pending_decision；生产工具执行与模型调用链 pending_evidence。
- 沿用 LLM-CTR-001～003、FLOW-CTR-002/003/004、GOV-CTR-003/004/005，不新增 CTR；不修改责任模块代码，不执行 CI。
- 下一步：进入业务事件生产方→Notification 边界审查，按 Data/News/Quant/Research/Researchflow 的实际事件生产责任核对事件 Envelope、schema_version、event_id、occurred_at、correlation_id、授权/敏感字段、Outbox/Inbox 幂等和 Delivery 反馈；不重复 Notification 内部 Channel/状态机规则。


## 28. 业务事件生产方→Notification 首批事件交互审查（2026-11-10）

- 固定审计基线：origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086；维护分支 feat/design_doc。详细差异见矩阵第32节。
- 已确认 INT-CTR-001 首批三类事件：Researchflow Run 终态、Quant 风险决策变化、Research Event 生命周期迁移；生产方本地事务 Outbox + Notification 双键 Inbox，at-least-once，业务事实生产方拥有事件语义，Notification 拥有接收和投递语义。
- Research：Event 版本与 Outbox 同事务；已实现完整生命周期 Envelope、稳定 event_id/idempotency_key、业务 occurred_at/published_at、JCS fingerprint 和 Outbox 领取/租约/重试；Server Relay 尚未接线。
- Researchflow：Run+ResultSummary+Outbox 原子提交已实现，但当前 event_type `researchflow.run.finalized`、Payload 字段/空值、Envelope 身份字段和原始 SHA-256 fingerprint 与已接受 INT-CTR-001 不一致；登记 BUS-EVT-A，禁止当前 Outbox 直接接入 Notification。
- Quant：有 RiskDecision 构造和结果写入 Port，固定基线未见风险事实事务 Outbox/持久化链；登记 BUS-EVT-B。
- Notification：EventReceiver 与 MySQL Inbox 双唯一键接收实现已建立，事务内 FanoutWriter 仍仅为扩展点，生产 Fanout/Worker 不完整；登记 BUS-EVT-D。Server Relay/DI/端到端集成缺证据，登记 BUS-EVT-C/E。
- 状态：事件职责/依赖方向 freeze-ready；INT-CTR-001 语义 accepted；Research 生产端基本对齐但 Relay pending_evidence；Researchflow 实现偏差、Quant Outbox、Notification Fanout、Server Relay 均 pending_evidence。
- 沿用 INT-CTR-001、NTF-CTR-001～003、QNT-CTR-005、RSC-CTR-003、FLOW-CTR-001/003、GOV-CTR-003/004，不新增 CTR；不修改业务代码，不执行 CI。
- 下一步唯一任务：审查 WebUI→Server 对外 API 契约边界，核对页面消费的公开 HTTP DTO/Schema/错误与认证语义、Server Handler 映射和契约测试；不重做 WebUI 视觉/交互审查，也不把内部模块 DTO 当作 Web 公共协议。


## 29. WebUI→Server 对外 HTTP API 契约边界审查（2026-11-10）

- 固定审计基线：origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086；维护分支 feat/design_doc。详细差异见矩阵第33节。
- 已确认：WebUI 同源 Nginx 分别代理 Market Data 8091 与 Analysis/Root Server 8092；Root Server 目前只装配 Legacy Analysis API/Worker 和 Platform Health/Router，未装配 Market Data Adapter 或 Researchflow Facade。Market Data 仍使用浏览器 localStorage API Key 作为迁移期兼容认证；Analysis 所有业务路由 Owner Bearer 保护，WebUI 不发送 Owner Token，故当前请求预期 401。
- 安全基础正确：Root Router 明确区分 Anonymous/Owner/Service Scope；Root Middleware 重生成 Request ID、限制请求大小/并发；Fault 对外只暴露 class/code/safe message/retryable，Request ID 在响应头；WebUI 兼容解析结构化错误与 Request ID。
- 关键待决：浏览器 Web Session/Cookie+CSRF+资源授权模型未定（WEB-SRV-B）；Analysis OpenAPI 未覆盖 WebUI/Root 已使用的 Run 查询路径且 DTO/认证/错误定义不完整（WEB-SRV-C）；Market Data/Root Error Request ID body/header 权威关系未统一（WEB-SRV-F）。
- 证据缺口：Data API Key 从浏览器迁移到 Server 注入（WEB-SRV-A）；Researchflow/Data Facade 正式 HTTP 装配与 Legacy Analysis 退役策略（WEB-SRV-D）；OpenAPI 驱动的跨端 Contract Test 和 Session/代理端到端验收（WEB-SRV-E）。
- 状态：职责边界 freeze-ready；当前路由/认证状态 accepted（仅迁移事实）；浏览器认证、Analysis OpenAPI、错误 Request ID 规范 pending_decision；目标装配/密钥迁移/联合测试 pending_evidence。
- 沿用 MDM-006、PLAT Security/Fault/Observability、FLOW-CTR-001/003/004、GOV-CTR-001/002/003，不新增 CTR；不修改 WebUI/Server/Data 业务代码，不执行 CI。
- 下一步唯一任务：核验 Server 组合根装配与 Platform 横向技术契约的实际使用，确认十模块 DI、资源生命周期、HTTP Client/Storage/Security/Diagnostics/Fault/Request ID 是否存在重复实现、越权承载领域语义或未接入；只登记证据和差异，不替模块修复。


## 30. Server 组合根与 Platform 横向技术契约接入核验（2026-11-10）

- 固定审计基线：origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086；维护分支 feat/design_doc。详细差异见矩阵第34节。
- 已确认 Root 唯一入口 `cmd/analysis-api` 委托 `internal/server/bootstrap.Run`；Server Resources 通过 Platform Storage 创建共享 SQL/Redis、创建 Platform HTTP Client；Lifecycle 启动/逆序停止、失败回滚，Bootstrap 在消费者停止后关闭共享资源。Platform Security Router、Fault、Observability Request ID、Diagnostics Runner 和 Migration Runner 均有实际使用证据。
- 但当前 Bootstrap 仍以 Legacy Analysis Runner/Queue/API 为主；Migration Registry 仅注册 News/Research/Notification；没有十模块完整 Facade/Adapter/Worker 装配。Market Data 仍独立运行，直接创建 SQL 连接池并使用自定义 Redis RESP 客户端。Root HTTP Client 只有创建和 BoundDoer 工厂，未发现业务上游绑定/模块注入生产链路；Diagnostics 只覆盖当前 DB/Redis/Schema。
- 状态：Server/Platform/模块职责边界 freeze-ready；当前共享资源、生命周期和 Platform 基础原语接入 accepted；十模块组合装配、Data 资源统一、BoundDoer 生产注入、模块 Diagnostics/Config 接入均 pending_evidence。Web Session A2 已在 WEB-SRV-B 登记，不重复。
- 差异登记 SRV-PLAT-A～E 为审计工作项，非新增 CTR；沿用 PLAT-CTR-005、PLAT-015～019 和现有模块契约。未修改业务代码，未执行 CI。
- **PLAT-002 边界审查已覆盖本轮预定的模块间交互与 Server/Platform 装配方向。下一步不再扩展无边界的代码审查；应汇总全部差异矩阵，按 freeze-ready / blocked / pending-decision / pending-evidence / deferred 形成最终冻结评估，并明确哪些契约可先冻结、哪些只冻结职责/语义而保留实现证据门禁。**


## 31. PLAT-002 全量差异矩阵收敛与最终冻结评估（2026-11-10）

> 历史快照：本节记录 B1～B8 获批前的待决统计；当前待决状态以第34节为准。

- 评估基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`；维护分支 `feat/design_doc`。
- 核对权威登记簿 43 个正式 CTR：39 个 `pending_evidence`，4 个 `has_difference + pending_evidence`，0 个 L2 `frozen`。
- 差异矩阵有状态工作项 56 条：22 个 `pending_decision`、1 个 `pending_decision + pending_evidence`、29 个 `pending_evidence`、2 个 `has_difference`、1 个 `accepted`、1 个 `deferred`。审计工作项不是 CTR，不与正式契约数相加。
- L1 Design Baseline 1.0.0 已冻结，锚点 `6d2966e35aab06ed30791701e4ca6fc036a07702`；仅覆盖明确稳定的职责、依赖、身份/时间/数值/授权/错误/状态/幂等/版本原则和首批范围。L1 冻结不等于 L2 实现证据冻结。
- L2 当前 0/43 frozen；主要 pending_decision 集中在 Data/Quant/Research/News/Researchflow/LLM/WebUI/Server 的条件字段映射、快照生命周期、结果引用投影、幂等/重试和浏览器 Session/OpenAPI/Error 规范；主要 pending_evidence 集中在真实 Facade/Adapter、Schema/Golden/双方 Contract Test、授权复核、Outbox/Relay/Fanout、Server DI/Worker/Storage/Diagnostics/Secret 注入。
- 可维持 L1 冻结的边界：模块职责/依赖方向；Data/News/Quant/Research/Researchflow/LLM/Notification 的领域 Owner；Server 唯一组合根；Platform 横向技术原语；WebUI 仅消费公开 HTTP 契约；Outbox/Inbox 与工具执行的职责边界。条件性未决子范围不得由实现方猜测。
- 不得宣告 PLAT-002 整体完成：十模块六维全覆盖、完整 import/消费者/授权链、责任模块修复补证、第二轮正确性 Review、第三轮冻结门禁 Review 尚未完成。
- 后续唯一主线：集中裁决非条件性 pending_decision 并写入正式 decisions；责任模块修复/补证；PLAT-002 按新 SHA 增量核验；满足 L2 门禁的 CTR 单项冻结。不重复已完成初审，不新增重复 CTR。


## 32. PLAT-002 项目决策提案批次整理（2026-11-10）

> 历史快照：本节的 proposal_pending 状态已由第33节正式裁决记录覆盖，不再作为当前待批准事项。

- 已读取差异矩阵全部状态表格行：69 行，其中 56 个带唯一显式编号的审计工作项；其余为边界/契约聚合行。Data→Quant、Data→Research 同名各有两条不同差异，不合并。
- 形成 8 个项目级决策提案 DEC-B1～B8，集中覆盖旧 Analysis 兼容/API、请求校验、Data Snapshot 映射、News Evidence 语义、News→Researchflow 输入快照、Quant ResultRef、Research Stage 读写幂等、LLM 请求/重试映射。
- 已有 DEC-PLAT-002-017 覆盖 WEB-SRV-B；Platform 08 第5节已覆盖 WEB-SRV-F；GOV-CTR-006 已覆盖 NEWS-RSC-C 的公共授权语义。这三项转为实现证据，不重复裁决。
- Data→Research available_at 属于字段/来源事实核验，转为 pending_evidence；DATA-FLOW-B 和 LLM-FLOW-B 为条件性扩展，转为 deferred。
- DEC-B1～B8 均为 proposal_pending_project_owner，不是批准决议；正式 CTR 冻结状态未变。本轮仅维护 PLAT-002 文档，不修改业务模块代码，不执行 CI。
- 下一步：项目负责人一次性对 DEC-B1～B8 逐项接受/修改/拒绝/延期；批准后责任模块按决议修复，PLAT-002 只做新 SHA 增量验收。


## 33. DEC-B1～B8 Review 裁决完成（2026-11-10）

- 项目负责人确认 B1～B8 的 Review 修订方向，正式登记 DEC-PLAT-002-018～025，汇总状态 DEC-PLAT-002-026。
- B1 根据项目负责人确认的部署事实调整为直接切换：当前无正在服役的旧 Analysis 服务，WebUI 直接调用 Researchflow 新 API，不保留旧 API 兼容期；新 API 验收后删除旧路由和专属适配代码。
- B2 修订同步 Run ID 规则：同 ID/同请求指纹返回既有 Run；同 ID/不同指纹返回 409。symbols 首期上限100由 Handler 强制，后续调整须有负载/性能依据并同步更新契约。
- B3～B8 按 Review 结论明确数据质量、News 关系状态、Run 输入物化范围、Quant Payload 所有权、Research Stage 幂等范围、LLM Run 配置固定和错误重试分类。
- 只更新 PLAT-002 项目级文档；未修改业务模块代码、未运行 CI、未改变任何正式 CTR 的冻结状态。
- 下一步主线：按决议责任边界执行模块实现/补证；优先处理 B1 新 API/WebUI 切换和旧路由退役方案、B2 Server 请求规则，然后按模块依赖推进 B3～B8。PLAT-002 只按新 SHA 增量验收，不重做全量初审。



## 34. 项目级决策 Review 通过与阶段转换（2026-11-10）

- 本轮复核对象：项目级决策记录、差异唯一登记矩阵、契约冻结登记簿、十模块总进度四份权威文档。
- Review 结论：B1～B8 正式决议 018～025 与汇总决议 026 已存在；决策记录旧“待项目负责人裁决”标题、矩阵旧 pending_decision 汇总/退出条件、登记簿旧待决入口已发现并完成同步修正。
- 当前项目级非条件性 pending_decision 为 0；原决策工作项转为 pending_evidence。DATA-FLOW-B、LLM-FLOW-B 保持条件性 deferred。正式 CTR 状态未因项目级决议批准而改变，当前仍为 0/43 L2 frozen。
- 阶段转换：项目级决策阶段 D 完成，下一阶段转为 E——责任模块按正式决议实施和补证。PLAT-002 维护分支继续只做治理文档和新 SHA 增量验收，不修改责任模块代码。
- 首批实施主线：B1 新 Researchflow API / WebUI 直接切换 / 旧 Analysis 专属路由退役；B2 Server 请求校验与同步 Run ID 幂等。后续按依赖推进 B3～B8。
- 分支边界检查：当前可见分支包含 `feat/researchflow`、`feat/webui-market-data-console`、`feat/platform-foundation` 等，但没有独立 Server 分支。Server 代码实施前必须明确其责任分支；不得默认把 Server 代码写入 Platform 或 integration 分支，也不得擅自创建新分支。
- 本轮未修改业务模块代码、未运行 CI、未改变任何正式 CTR 冻结状态。


## 35. CTR-006 责任分支增量核验结果（2026-11-10）

- 本轮仅按既有有限执行清单核对 CTR-006 是否已有责任分支修复，不新增审查切片。
- **更正第34节历史分支判断：** 独立 Server 责任分支 `feat/research-analysis-flow-native-go` 确实存在；此前“没有独立 Server 分支”的表述不准确。该分支 HEAD 为 `a7efeff9e8802e69cc58a0e7d17f3f37a8eee873`，但它是固定审计基线 `b6dfc85ba214fe16268c8cba780641a8bfe15086` 的祖先，不能提供基线之后的修复证据。
- 固定基线 OpenAPI 仍将 Task.started_at/finished_at 标记为 nullable，与 Go 指针字段 omitempty 的缺失语义不一致；CTR-006 既定 queued/running/terminal Golden、Schema 双向校验和 Handler Contract Test 尚无新 SHA 证据。
- 固定基线 Check Runs 为 0、Commit Status 为空；本轮未执行测试/CI，未修改责任模块代码。
- 结论：CTR-006 保持 pending_evidence，不关闭、不冻结。矩阵第37节记录了精确证据与下一步验收条件。后续仅在责任分支出现新修复 SHA 后增量验收，不再重复字段语义审查。


## 36. CTR-043 Dragon Tiger P1 冲突定位结果（2026-11-10）

- 本轮按 B0 有限执行批次增量核验既有 CTR-043，没有新增 CTR，也没有修改 Data/WebUI 责任模块。
- 已定位冲突：同花顺 Provider 将单位未明确的金额保留在 `provider_*_value`，并标记 `provider_amount_unit="provider_defined"`，不填充 `*_cny`；WebUI 龙虎榜表格只展示 `net_buy_amount_cny` 与 `deal_amount_cny`。当同花顺作为回退源返回有效 degraded 数据时，页面可显示证券/涨跌幅/质量，但资金金额列无法展示。
- 不应将同花顺原始金额擅自转换或冒充 CNY。当前 Data Go DTO 与 OpenAPI 对原始值/CNY 的字段区分一致；差异位于 Provider 回退输出与 WebUI 展示能力之间，并缺真实 Provider→HTTP→WebUI Fixture/Contract Test 闭环。
- 关闭条件：Data 补真实同花顺 Provider 响应 Golden/Contract Test，保持单位边界；WebUI 明确原始金额展示或不可比较/未提供策略，并补真实响应 Fixture 渲染测试；两端定向测试与 CI 通过。详细文件证据和验收边界见差异矩阵第38节。
- 状态：CTR-043 保持 P1 open / L2 blocked，不冻结。此次仅静态核验，未执行测试/CI；不将静态证据表述为测试通过。


## 37. CTR-043 契约冻结与实现符合性边界更正（2026-11-10）

- **本节为 CTR-043 当前状态的权威更正，优先于此前将其称为“P1 契约冻结阻断”或将其改写为金额展示冲突的记录。** 原始 CTR-043 是 Hithink 龙虎榜分页计数不符合 Data 公共契约：`source_returned` 应表示当前页 Provider 原始记录数，且 `source_returned = returned + excluded_non_a_share_count`；Hithink 却以全量 `stock_items` 数量填充并本地分页，重复 symbol 去重也未计入排除数。
- 公共分页计数语义已经由 DTO、Service 校验和 OpenAPI 明确。生产方和消费方无需知晓 Provider 内部如何抓取、分页或去重。因此这是 Data Adapter 对既有公共契约的实现符合性问题，不是公共契约待定义问题。
- **PLAT-002 状态修正：CTR-043 公共契约语义 `freeze-ready`，不再作为 L2 契约冻结阻断项。** Hithink Adapter 修复、真实 Provider 专项测试及 CI 属于 Data 模块内部工程质量和实现整改，另由 Data 模块跟踪，不属于 PLAT-002 契约冻结门禁。
- Data 对外仍须满足已冻结的分页计数语义；如果运行实现不满足，应修复实现，而不是改变契约或要求 WebUI 了解 Provider 细节。
- 同花顺 `provider_*_value` / `provider_amount_unit` 与 WebUI 金额展示不是 CTR-043 原始问题。本轮不据此要求 WebUI 增加消费逻辑或渲染测试，也不新增 CTR。若未来决定清理公共 DTO 中 Provider 原始字段，应单独评估公开 DTO 范围和兼容影响。
- 此更正只更新项目级契约审计文档；未修改 Data/WebUI 代码，未执行测试或 CI。差异矩阵第38节为详细依据。
