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
