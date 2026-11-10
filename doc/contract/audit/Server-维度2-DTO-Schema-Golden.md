# Server 维度2：DTO、Schema、Golden 契约核验

**固定代码审计基线：** `origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`  
**维护分支：** `feat/design_doc`  
**核验范围：** Root Server 当前实际注册的 HTTP 路由及其请求/响应 DTO、OpenAPI、JSON 序列化和测试证据。  
**状态：** 首轮切片进行中（Root Server Health/Diagnostics + 旧 Analysis HTTP 入口）。

## 1. 范围与排除项

本轮核对：
- `internal/server/bootstrap/router.go` 路由注册和实际Handler映射；
- `internal/server/health/handler.go` 的 Health/Readiness/Diagnostics 响应；
- Root Server 组合的旧 Analysis Handler：`internal/platform/httpapi/server.go`、`tasks.go`；
- 实际传输结构、Go JSON Tag、OpenAPI、现有Handler测试；
- WebUI对Root Server路径及错误字段的消费。

不在本轮核验：
- 路由认证/授权策略正确性（维度3/4）；
- Request ID可信入口、传播算法（维度1/3）；
- 业务分析规则、任务队列状态机、数据库事件cursor持久化（相应领域维度/既有CTR关闭条件）；
- Market Data独立HTTP API的DTO/OpenAPI（Data模块负责）；
- 模块业务API未来是否由Root Server注册，除非固定基线已有实际路由。

## 2. Root Server 实际HTTP路由清单

固定基线 `internal/server/bootstrap/router.go` 注册13条 method-pattern：
- `GET/HEAD /healthz`
- `GET/HEAD /readyz`
- `GET/HEAD /diagnostics`
- `GET /{$}`
- `POST /api/v1/analysis/runs`
- `GET /api/v1/analysis/runs/{run_id}`
- `GET /api/v1/analysis/runs/{run_id}/events`
- `POST /api/v1/analysis/tasks`
- `GET /api/v1/analysis/tasks/{task_id}`
- `GET /api/v1/analysis/tasks/{task_id}/events`

HEAD共用对应Health Handler；Root与旧Analysis业务路径由 `internal/platform/httpapi.Server` 提供。此处记录的是固定基线真实路由，不将 `internal/marketdata/httpapi` 的独立API混算为Root Server接口。

## 3. Health、Readiness、Diagnostics DTO

### 3.1 生产响应结构

- `GET /healthz`：HTTP 200，JSON对象 `status`、`time`。
- `GET /readyz` 成功：HTTP 200，JSON对象 `status=ready`、`dependencies`（check ID到状态字符串的Map）、`time`。
- `GET /readyz` 失败：HTTP 503，`error{code,message}`；draining时code为`server_draining`，依赖异常时为`dependencies_unready`。
- `GET /diagnostics` 成功：HTTP 200，返回 `serverdiagnostics.Report`，包含 `diagnostics`（Platform Report）和 `readiness`（ready/reason_code/safe_message）。
- `/diagnostics`执行失败：HTTP 503，返回 `error{code,message}`，不回显内部错误。
- HEAD返回与GET一致的状态和响应头，但不写Body。

代码证据：
- `internal/server/health/handler.go`：实际JSON字段和状态码；
- `internal/server/diagnostics/aggregator.go`：`Readiness`、`Report` JSON Tag；
- `internal/platform/diagnostics/aggregate.go`：`Summary`、`Counts`、`Result` JSON Tag；
- `internal/platform/diagnostics/observation.go`：Observation字段及JSON Tag；
- `internal/server/bootstrap/router.go`：真实路由接线。

### 3.2 机器证据

`internal/server/health/handler_test.go`覆盖：
- Health不依赖Readiness依赖检查；
- Diagnostics成功返回结构化报告；
- Diagnostics/Readiness内部错误不回显；
- Readiness未就绪和draining返回503；
- 依赖状态映射healthy为ok、其他状态保留原值；
- GET/HEAD响应行为和Cache-Control/Content-Type。

现有测试以fake checker和部分字段断言为主，没有发现完整Server Report JSON Golden、Server Report Schema校验或对所有响应分支的精确JSON契约断言。

### 3.3 差异 SRV2-D2-001（归属既有 SRV-CTR-001 / PLAT-CTR-004）

Root Server公开暴露 `/diagnostics` 的复合Report和 `/readyz` 的响应对象，但未发现Server专属Schema/Golden。Platform已有Observation Schema不能覆盖Server新增的Readiness与Report包装结构；Platform Runner测试也不能替代Root Handler序列化证据。

性质：P2契约证据缺口；不是已证实的运行时字段错误。按既有唯一契约归属，不新建CTR：
- Server Root路由、响应封装和OpenAPI归属SRV-CTR-001；
- 嵌入的Platform Observation字段语义继续归PLAT-CTR-004，不在Server重复定义。

关闭条件：
1. 明确Health/Readiness/Diagnostics是否作为稳定运维HTTP契约长期保留；
2. 若保留，定义Server层版本化Schema，引用Platform Observation Schema或以一致方式约束嵌入结构；
3. 为healthz、readyz成功/失败、diagnostics成功/失败补精确JSON Golden/Handler Contract Test，并覆盖HEAD无Body；
4. 若项目决定不为运维端点提供Schema/Golden，需在权威契约登记中记录明确的不适用理由和替代测试要求。

## 4. 旧 Analysis HTTP DTO 与 OpenAPI 覆盖

### 4.1 真实请求/响应

- `POST /api/v1/analysis/runs` 和 `POST /api/v1/analysis/tasks` 均直接解码到 `analysis/domain.RunRequest`；启用DisallowUnknownFields，限制请求体1MiB，并要求单个JSON对象。
- Handler会补默认值并根据as_of是否显式提供设置as_of_mode；历史回放缺少显式as_of返回400。
- 同步创建成功及GET Run成功直接序列化 `domain.RunResult`。
- GET Run Events返回 `{run_id, events}`，events为 `[]domain.StageEvent`。
- 创建Task返回 `{task,status_url,events_url}`；GET Task返回 `{task}`；Task Events返回 `{task_id,events,next_after_sequence}`。
- Task/TaskEvent具体字段来自 `internal/platform/taskqueue/queue.go` 的 `Task`、`Event`，内嵌 `domain.StageEvent`。

### 4.2 OpenAPI现状

唯一找到的旧Analysis OpenAPI文件为 `doc/api/contracts/分析任务.openapi.yaml`：
- 覆盖POST Task、GET Task、GET Task Events；
- 未覆盖Root Router已注册的POST Run、GET Run、GET Run Events；
- 未覆盖Root、Health、Readiness、Diagnostics；
- `AnalysisRequest` 描述为与 `domain.RunRequest` 一致，但Schema当前未列出RunRequest的全部JSON字段：`run_id`、`as_of_mode`、`parameters`未定义；也没有required列表/TaskType枚举，不能视为完整请求Schema；
- `TaskEvent.event`仅为无字段约束的object，StageEvent字段、状态枚举、时间格式未展开；
- Task.started_at/finished_at声明nullable，但Go字段为指针且带omitempty，nil时实际省略字段。该差异已由既有CTR-006承接；
- CTR-047已承接CreateTask请求DTO语义与TaskType/required约束；不得新建重复差异。
- GET Run/Events缺失OpenAPI由既有CTR-046承接；不得新建重复差异。

### 4.3 Go测试证据

`internal/platform/httpapi/server_test.go`覆盖同步Run默认值、拒绝未知字段、错误状态映射、错误脱敏、请求体大小/Content-Type和Request ID局部行为。

`internal/platform/httpapi/tasks_test.go`覆盖Task创建、幂等Key透传、历史回放as_of约束、事件增量cursor、队列满、任务不存在、幂等冲突、请求体大小和Content-Type。

测试主要使用fake Runner/Store/TaskService；本切片未发现：
- 以OpenAPI Schema对真实Handler输入/输出进行双向校验的测试；
- 覆盖RunResult、StageEvent、Task、TaskEvent的稳定Golden文件；
- 同步Run/Run Events与Task Events所有错误响应分支的精确JSON结构契约测试。

### 4.4 差异与既有编号承接

| 观察 | 唯一归属 | 当前结论 |
|---|---|---|
| Run创建/读取/Events路由未纳入Analysis OpenAPI | CTR-046（GET Run/Events）及SRV-CTR-001（Root公开路由登记） | OpenAPI覆盖不完整；同步POST Run也需纳入统一API清单，不与CTR-047的Task请求混为一谈 |
| Task请求Schema未完整表达RunRequest字段/约束 | CTR-047 | 已有差异，不新增CTR |
| StageEvent Schema过于宽泛 | CTR-007（Researchflow为领域结构主责；Server为HTTP契约协作） | 已有差异，不新增CTR |
| Task时间指针omitempty与nullable不一致 | CTR-006 | 已有差异，不新增CTR |
| 多种Handler错误Envelope字段不同 | PLAT-CTR-001（Server负责HTTP映射，旧Analysis链路为消费者） | 见第5节；不新建错误契约编号 |

## 5. Root Server错误响应映射

Root Server Middleware定义 `publicErrorEnvelope`：
`error{class,code,message,retryable}`，用于请求体超限、并发过载和Platform Fault映射；非法Fault元数据统一映射为内部错误，Dependency/Cause不输出。

旧Analysis Handler的 `writeError` 则输出 `error{code,message}`；Health Handler也输出 `error{code,message}`。两者都不包含class/retryable。WebUI Client兼容读取error.message、error.code，并优先从X-Request-ID响应Header获取Request ID。

这意味着Root Server当前并非一个统一的错误响应DTO；但各Handler对外消息均做安全收敛，现有测试覆盖若干内部错误不泄漏场景。该问题并入既有PLAT-CTR-001，不新建CTR。关闭时应由Server明确统一Envelope还是按端点类型保留多个Envelope；若统一，必须同步HTTP状态码、OpenAPI、WebUI解析和Golden，不能只机械补字段。

## 6. WebUI消费映射（本切片范围）

`web/src/api.js`：
- `analysisLiveness`请求`/healthz`；
- `analysisHealth`请求`/readyz`；
- 请求错误解析`error.message`、`error.code`，Request ID优先读响应Header `X-Request-ID`；
- 本文件中未发现对Root `/diagnostics` 的消费。

WebUI当前Analysis请求基址默认是`/analysis-api`，与旧Analysis服务独立入口路径一致；Root Server迁移后路由代理/基址调整属于SRV-CTR-001与WebUI/部署集成验收，不在本轮改动。

## 7. 首轮切片结论

**Server维度2首轮切片为 `partial / complete_with_open_items`，不能标记完成或冻结。**

已完成：
- Root Server真实路由与Health/Readiness/Diagnostics响应结构核验；
- 旧Analysis Run/Task Handler、Go JSON DTO、OpenAPI现状及测试覆盖核验；
- WebUI Health/Readiness和错误字段消费映射；
- 所有已识别差异均归入既有SRV-CTR-001、PLAT-CTR-001、PLAT-CTR-004、CTR-006、CTR-007、CTR-046、CTR-047，不新增CTR。

未完成/开放：
- SRV2-D2-001：Health/Readiness/Diagnostics复合响应缺少Server级Schema/Golden或明确不适用裁决；
- CTR-046：Run GET/Events等OpenAPI缺失关闭条件；
- CTR-047：CreateTask请求DTO/schema约束；
- CTR-007：StageEvent字段级Schema；
- CTR-006：Task时间字段序列化语义；
- PLAT-CTR-001：Root错误Envelope统一策略及各Handler映射证据；
- 同步POST Run未进入当前分析任务OpenAPI，需在Server权威路由/API清单中明确纳入，不与Task DTO差异重复计数。

本轮仅做静态代码与文档证据核验；未修改业务代码、OpenAPI、DTO、Golden或测试，未运行CI。


## 8. Server 维度2第二切片：错误 DTO 与消费者契约交叉核验（2026-11-10）

### 8.1 Root 路由范围复核

固定基线 internal/server/bootstrap/router.go 仍只注册第2节列出的13条 method-pattern：Health/Readiness/Diagnostics、Root discovery、Analysis Run/Events、Analysis Task/Events。没有发现Root Router注册News、Research、Researchflow、Quant、LLM、Notification或Market Data模块Handler的其他路径。本切片不扩大路由范围，也不将独立Market Data HTTP API计入Root Server。

### 8.2 Root错误生产路径与DTO

| 生产路径 | 状态/触发 | JSON错误形状 | 附加HTTP语义 |
|---|---|---|---|
| Bootstrap请求体上限 | 413，声明Content-Length超过Root上限 | error{class,code,message,retryable} | Cache-Control no-store；安全头 |
| Bootstrap并发门控 | 503，非/healthz请求超过并发上限 | error{class,code,message,retryable}，code=server.overloaded、retryable=true | Retry-After: 1 |
| Platform Fault映射 | 按Fault Class映射400/401/403/404/409/503/504/500 | error{class,code,message,retryable} | 只输出SafeMessage；不输出Dependency/Cause；非法Fault元数据转500通用错误 |
| Platform Security Guard | 401/403 | error{code,message} | 401带WWW-Authenticate: Bearer；Cache-Control no-store |
| Health Handler | 405/503 | error{code,message} | 405带Allow: GET, HEAD；503包括draining、dependencies_unready、diagnostics_unavailable |
| 旧Analysis Handler | 400/404/409/413/415/429/500/502/503等 | 通常为error{code,message} | 同步Run执行失败另带partial_result；Run rejected是RunResult响应而非通用error对象 |

补充实现证据：
- internal/server/bootstrap/http_middleware.go：publicError、请求体上限、并发门控、Platform Fault到HTTP状态码映射。
- internal/platform/security/security.go：Guard的401/403响应仍是旧的error{code,message}。
- internal/server/health/handler.go：Health/Readiness/Diagnostics错误分支和405。
- internal/platform/httpapi/server.go、tasks.go：旧Analysis的writeError及Run执行失败分支。
- Root Middleware对声明超限请求直接返回server.request_too_large；进入旧Analysis解码器的超限错误则使用request_too_large。二者均为413，但错误码不同，说明即使HTTP状态一致，错误码也不是统一映射。
- publicError.retryable为非omitempty bool，当前此类Envelope会输出该字段；旧Envelope没有class/retryable。旧Analysis错误不会自动经过Fault映射，因为Handler将领域/存储错误显式映射为状态码和安全消息。

### 8.3 WebUI真实消费行为校正

web/src/api.js 的统一request函数：
- 错误消息读取顺序：error.message、字符串型error、顶层message，最后回退HTTP状态文本；
- 错误码读取error.code或顶层code，缺省为http_error；
- Request ID读取顺序实际是**响应体error.request_id优先，其次响应头X-Request-ID**，不是Header优先。web/tests/api.test.mjs有专门测试：当Body与Header同时存在且不同时，Body值胜出；
- 不读取/保留error.class和error.retryable，生成的Error对象包含status、code、requestId；不把retryable转为客户端重试决策；
- 非JSON响应、超长消息截断、HTTP状态和Request ID有WebUI测试覆盖。

因此，原第5、6节“Request ID优先读Header”的表述需要以本节为准：Header是Root Server实际统一提供的关联通道，但WebUI保留Body优先的兼容逻辑。Root错误DTO当前没有统一在Body中输出request_id。Request ID可信入口/传播正确性属于PLAT-CTR-002及维度1/3，本切片只记录消费者优先级，不重审其信任边界。

### 8.4 OpenAPI、测试与唯一CTR核对

- doc/api/contracts/分析任务.openapi.yaml没有定义可复用Error Schema，也没有为已有Task端点的错误响应声明统一JSON content/schema；当前主要是状态码及文字描述。
- 同一OpenAPI缺少Run创建/读取/Events路径，已由CTR-046承接；CreateTask请求字段/required/枚举不完整，已由CTR-047承接。本批只确认它们与错误响应契约相邻但不是同一差异，不重新登记、不重复统计。
- Bootstrap Middleware测试覆盖超限、并发过载、Fault Class到状态码矩阵、错误内部信息不泄漏及无效Fault元数据降级；目前主要检查字段存在/状态/不泄漏，不是完整JSON Golden。
- Security Guard、Health Handler、旧Analysis Handler各自有局部测试；WebUI测试覆盖错误码、状态、Body/Header Request ID优先级和非JSON代理错误。没有发现跨Root全路由的统一Error Schema校验或Golden。
- 本切片没有新建CTR。错误Envelope形状/语义唯一归入PLAT-CTR-001；Root公开路由及运维端点契约归SRV-CTR-001；Request ID传播仍归PLAT-CTR-002；CTR-046/047保持既有唯一归属。

### 8.5 是否统一Envelope：审计建议与决策条件

**建议Root Server稳定错误响应逐步统一为同一嵌套Error DTO，至少保留现有code、message并补齐class、retryable；不建议在本审计批次直接修改实现。**

理由：
1. 当前错误生产路径至少有Middleware/Fault、Security、Health、旧Analysis多种实现入口，消费者不得不容忍字段缺失和错误码分叉；
2. WebUI目前只依赖code/message/status，新增class/retryable可保持向后兼容，但不能据此推断所有未来消费者都不使用旧形状；
3. 统一后可明确retryable的逐错误语义、Fault Class到HTTP状态映射、错误码命名空间及请求体超限的唯一错误码；
4. Run执行失败的partial_result属于端点级附加数据，不应因此把Error DTO与成功RunResult混为一体。

进入实现前仍需项目/Server责任方确认：
- 是否将所有Root错误（含Security/Health/Analysis）纳入同一Envelope；是否保留端点级扩展字段；
- class枚举、retryable真值规则、HTTP状态码及稳定code清单；
- Root请求体上限与Handler解码上限是否共用同一code；
- Request ID是否只以响应Header为权威，Body中的request_id是否继续兼容；
- OpenAPI对错误响应的覆盖范围，以及WebUI是否需要消费class/retryable。
未获正式决议前，不应只为字段形状一致而机械填充class/retryable，也不应把现状误判为运行时安全漏洞。

### 8.6 本切片结论

本切片完成错误DTO生产路径、HTTP状态映射、WebUI消费者和现有测试证据交叉核验，状态为complete_with_open_items。确认Root路由没有新增模块Handler；不重复Health/Analysis成功响应初审；不重复CTR-046/047登记。

遗留项仍归既有CTR：
- PLAT-CTR-001：错误Envelope统一决议、分类/retryable语义、全路由Schema/Golden/消费者补证；
- SRV-CTR-001：Root公开HTTP路由与运维端点契约登记；
- PLAT-CTR-002：Request ID可信入口/传播与Body/Header权威性，留原维度处理；
- CTR-046/047：OpenAPI路由及Task请求Schema，按原关闭条件增量验收。

本批仅做固定SHA静态核验与审计文档更新；未修改业务代码、OpenAPI、DTO、Golden、测试，未执行CI。


### 8.7 Request ID 组合链路的增量风险（归既有 PLAT-CTR-002）

本轮沿错误响应的Request ID通道继续核对Root组合顺序，发现上一轮“外层Root Middleware先重建ID，因此内层兼容Middleware不会再信任原始输入”的推断不成立：

1. internal/server/bootstrap/http_middleware.go 通过 observability.MiddlewareWithOptions(next, zero-value options) 包裹Root Router；该中间件生成新ID、写响应Header并写入Context，但不修改传入Request的X-Request-ID Header。
2. internal/platform/httpapi.Server.Handler() 返回的旧Analysis Handler仍包裹 observability.Middleware(next)，其兼容默认值 TrustIncomingRequestID=true。
3. 该内层中间件仍可从原始Request Header读取调用方提交的合法ID，并再次覆盖Context与响应X-Request-ID。因此Root Analysis路由上的外层重新生成结果可能被内层覆盖；Health Handler没有这层旧包装。
4. 这会削弱Analysis路径的Request ID防伪与日志关联完整性，不是认证绕过；本切片不判定更高严重级别，也不修改代码。

该发现归入既有PLAT-CTR-002 / P002-D03，不新建CTR。关闭证据应由Server/Integration责任方提供：移除组合场景中的兼容型内层Middleware，或确保内层只使用外层已验证Context且不能重新信任原始Header；增加Root完整组合测试，传入caller-controlled ID后验证最终响应Header、Handler日志Context及后续传播均保持Root生成ID。需同步纠正Platform维度2 Request ID切片中“Root外层重建即可覆盖内层兼容Middleware”的既有结论。


## 9. Server 维度2第三切片：Run/Task DTO 字段语义与 OpenAPI 增量核验（2026-11-10）

本切片只增量核对既有 CTR-046/047/006/007，不重新初审错误Envelope、Health/Readiness或WebUI错误解析。

### 9.1 RunRequest真实字段、输入与默认值

代码来源：internal/analysis/domain/types.go 的 RunRequest；Handler：internal/platform/httpapi/server.go 的 handleCreateRun、fillDefaults；异步入口：internal/platform/httpapi/tasks.go 的 handleCreateTask。

| JSON字段 | Go类型/标签 | Handler实际行为 | OpenAPI现状 |
|---|---|---|---|
| run_id | string / omitempty | 解码允许；同步入口不在Handler重写；Queue.Create会生成新RunID并覆盖传入值 | 缺失 |
| task_type | TaskType / 无omitempty | 空值默认post_market_scan；Handler不在解码阶段验证枚举 | string，无enum |
| as_of | time.Time / 无omitempty | 零值补为Asia/Shanghai当前时间；historical_replay要求请求显式提供 | date-time，未声明required/default |
| as_of_mode | AsOfMode / omitempty | 接受解码，但随后依据as_of是否显式提供强制设为explicit或realtime_snapshot | 缺失 |
| universe_id | string / 无omitempty | 空白值默认request-watchlist | string |
| symbols | []string / omitempty | 可省略；当前HTTP Handler不在解码层校验最多100项 | array，声明maxItems=100 |
| portfolio_id | string / 无omitempty | 空白值默认paper-unconfigured | string |
| strategy_ids | []string / 无omitempty | 空数组默认trend-following | array |
| market_id | string / 无omitempty | 空白值默认CN-A | string |
| strategy_config_version | string / 无omitempty | 空值默认trend-following-mvp-v1 | string |
| risk_config_version | string / 无omitempty | 空值默认unverified-tradability-block-v1 | string |
| trading_rules_version | string / 无omitempty | 空值默认not-configured-v1 | string |
| data_policy_version | string / 无omitempty | 空值默认baseline-quality-v1 | string |
| parameters | map[string]string / omitempty | 可省略；若提供则按string map解码 | 缺失 |

结论：
- OpenAPI AnalysisRequest并非完整DTO Schema，CTR-047既有缺口得到字段级确认；不得新增CTR。
- required不能简单按Go非指针字段生成：TaskType、AsOf、UniverseID、PortfolioID、StrategyIDs、MarketID及版本字段会在Handler补默认值；但historical_replay对显式as_of有条件必填规则，应通过条件约束或描述表达。
- TaskType应列出post_market_scan、watchlist_scan、position_review、historical_replay四个代码枚举值；当前HTTP解码层未验证未知TaskType，Schema enum只能描述目标契约，不能假称运行时已有同等校验。
- as_of_mode属于服务端派生值：客户端即使传入也会被覆盖。权威请求Schema应明确其不接受输入或明确忽略规则，不应把它作为普通客户端可控字段。
- run_id在异步CreateTask路径会被Queue生成值覆盖；同步Run路径是否允许客户端指定run_id需由Run契约责任方裁决。共享Schema不能隐藏两路径差异。
- symbols.maxItems=100目前仅见于OpenAPI，本轮检查的Handler解码逻辑没有相应数量限制；在验证下游是否另有限制前，不应宣称该约束已由HTTP入口执行。
- OpenAPI缺失parameters、run_id、as_of_mode均作为CTR-047字段级证据；Run路径覆盖由CTR-046承接。

### 9.2 Task与TaskEvent真实序列化字段

代码来源：internal/platform/taskqueue/queue.go 的 Task、Event；领域事件来源：internal/analysis/domain/types.go 的 StageEvent。

Task字段：
- 始终输出：task_id、run_id、status、task_type、completed_stages、created_at。
- 空值省略：current_stage、failure_code、error_message。
- started_at、finished_at为* time.Time且带omitempty：nil时省略；非nil时输出时间值。
- OpenAPI Task required六字段与Go非omitempty字段基本一致；可选字段未列required也与omitempty字段基本一致。
- 唯一已确认的不一致仍是started_at/finished_at的nullable=true：Go nil时省略，不会输出显式null。归CTR-006。

Task.status：
- Queue映射输出queued、running、succeeded、partial、failed。
- domain.RunStatus与Task状态不是同一枚举；RunStatus包括running、completed、blocked_data_quality、rejected_request、partial、failed。blocked_data_quality映射为Task succeeded，rejected_request映射为Task failed。
- OpenAPI Task.status当前枚举与Queue映射结果一致；不应把RunStatus枚举直接复用为Task.status枚举。

TaskEvent / StageEvent：
- Event外层始终输出sequence（int64）和event。
- StageEvent始终输出run_id、stage、status、at；duration_ms、message、attributes为空时省略。
- StageEvent.status枚举为started、succeeded、failed、skipped；at为time.Time，JSON时间格式为RFC3339/RFC3339Nano。
- OpenAPI TaskEvent.event当前只声明type: object，没有字段、required、status enum或date-time约束。归CTR-007。
- StageEvent的attributes是map[string]string；message是string；duration_ms是int64。Schema应明确这些类型及可选性，而不是只复制Go结构体描述。

### 9.3 Run响应契约边界

- POST Run成功和GET Run成功直接序列化domain.RunResult；GET Run Events返回{run_id, events}，events为StageEvent数组。
- 当前分析任务OpenAPI没有POST Run、GET Run、GET Run Events路径，也没有RunResult响应Schema；CTR-046承接路由和响应契约覆盖缺口。
- RunResult是较大的嵌套领域结果DTO，包含Request、Quality、MarketState、Factors、Signals、Tradability、Risk、Plans、Execution、Events、Failure、Warnings等结构；不能用一个泛化object或只定义RunID/Status冒充完整Schema。
- 本切片不展开RunResult全部嵌套子结构；其完整Schema拆解留在CTR-046关闭工作中，避免本批变成无边界领域DTO全仓审查。

### 9.4 Golden、Schema与测试证据边界

- server_test.go证明同步Run默认值、未知字段拒绝、请求体限制和Content-Type行为；tasks_test.go证明Task入口默认值、历史回放as_of约束、幂等与事件cursor Handler映射。
- 现有测试未形成RunRequest/Task/StageEvent完整稳定JSON Golden；也未发现Handler输入输出与OpenAPI Schema双向校验。
- Task nil时间字段的omitempty行为可从Go标签直接确定，但当前未发现专门断言queued/running/terminal三类状态下started_at/finished_at省略/出现规则的Golden。
- 本切片没有找到覆盖旧Analysis taskqueue所用analysis_tasks、analysis_stage_events表的独立迁移文件；本结论仅说明固定基线文件树中未定位到对应DDL，不推断生产数据库中不存在表。DDL来源和cursor持久化约束属于CTR-008/Researchflow存储证据，不在本切片扩大审计。
- 不以Fake Handler测试替代数据库持久化证据；不把Researchflow自身的新表迁移误认作旧Analysis taskqueue表DDL。

### 9.5 唯一CTR归属与本切片结论

| 差异 | 唯一归属 | 本轮增量结论 |
|---|---|---|
| Run创建/读取/Events路由与RunResult响应未进入OpenAPI | CTR-046；Root路由目录仍由SRV-CTR-001承接 | 已确认缺口；RunResult完整Schema未展开 |
| AnalysisRequest字段缺失、required/default/TaskType约束不足 | CTR-047 | 字段级差异已确认；还需权威契约决议和Schema/测试实现 |
| Task.started_at/finished_at nullable与omitempty不一致 | CTR-006 | Go nil省略已确认；需选定省略或null并三方对齐 |
| TaskEvent.event未展开StageEvent结构 | CTR-007 | 字段、状态枚举、时间、可选属性类型已核实；Schema/Golden仍缺 |
| Task Events cursor持久化/DDL约束 | CTR-008 | 本切片不重做；仅记录旧Analysis DDL未定位，按原关闭条件补证 |

本切片状态：complete_with_open_items。字段事实已核实并回写唯一登记；没有CTR关闭，因为尚无权威Schema/Golden、运行时校验或责任方裁决证据。Server维度2整体仍为in_progress / partial。



## 10. Server 维度2第四切片：Run/Task 契约决策依据与关闭路径（2026-11-10）

本切片不重做第9节字段盘点，只核对项目权威契约、Researchflow DTO 责任边界和既有差异的关闭依赖。

### 10.1 权威契约与责任边界

固定基线中的权威依据：

- `doc/contract/PLAT-002-项目级契约冻结登记簿.md`：Server 是唯一 Composition Root，负责HTTP路由、DI和模块接线；Researchflow 是唯一跨业务模块流程编排者，拥有Run/Stage/Attempt及其状态机；Server不拥有领域DTO语义。
- `doc/researchflow/03-CONTRACT-006与007最终决议.md`：FLOW-CTR-001/002（历史别名CONTRACT-006/007）状态机设计决策为accepted；明确Run/Stage/Attempt读取模型字段、状态、Ref、版本和时间语义。
- `doc/researchflow/04-ApplicationPort与运行时契约落地补充.md`：Researchflow的Domain JSON Tags不构成公开DTO；公开Run/Stage/Attempt DTO必须独立定义Schema/Golden。权威关系明确由03最终决议定义状态/竞态，项目级契约目录定义编号、公共字段和状态。
- 实现证据：`internal/researchflow/publicdto/dto.go`、`dto_test.go`；Schema位于`doc/researchflow/schema/{run,stage,attempt,result-summary}-1.0.0.schema.json`，Golden位于`doc/researchflow/golden/`。测试包含稳定字段、Schema/Golden和Domain投影校验代码。既有决议记录定向Go Test/Race/Vet尚无执行证据（blocked_by_external_module_gofmt），故只能确认测试代码存在，不能报告通过。

因此必须区分两套模型：

1. **旧Analysis HTTP模型**：`internal/analysis/domain/types.go`中的RunRequest/RunResult/StageEvent，加上`internal/platform/taskqueue`中的Task/Event，由旧Analysis Handler直接暴露；当前`doc/api/contracts/分析任务.openapi.yaml`描述的也是这套任务API。
2. **Researchflow正式模型**：`internal/researchflow/domain`及`internal/researchflow/publicdto`中的Run/Stage/Attempt/ResultSummary，由Researchflow拥有领域语义和公开DTO；Server只负责组合根、HTTP适配和对外路由。

旧Analysis的`StageEvent`不是Researchflow的`Stage` DTO，也不是Researchflow状态机事件的替代定义。Researchflow当前公开读取模型的Stage具有stage_ref、run_ref、stage_code、status、version、required、依赖和结果/诊断引用等字段；旧StageEvent则是旧Analysis执行进度事件（run_id、stage、status、at等）。不得把两者合并为一个Schema，也不得将旧StageEvent字段裁决责任转移给Researchflow。

### 10.2 对既有CTR的决策状态归并

| CTR | 已有权威决议 | 尚需项目/责任方裁决 | 实现与机器证据责任 |
|---|---|---|---|
| CTR-046 | Server为HTTP组合根；Researchflow拥有Run/Stage/Attempt领域和公开DTO；Researchflow Application Port已有CreateRun/GetRun/CancelRun等定义 | 旧Analysis同步Run API是否继续作为兼容入口；对外Run API最终映射旧Analysis还是Researchflow Application；Root路径、版本和响应包装策略 | Server负责Root路由、HTTP适配和OpenAPI；Researchflow负责自身Run/Stage/Attempt DTO/Schema/Golden；两侧共同提供真实Handler/Port映射和集成测试 |
| CTR-047 | Researchflow CreateRunCommand必填字段、调用主体从可信Context注入、system_as_of/deadline_at独立、参数按固定Template Schema校验等语义已接受；但这些规则不自动适用于旧Analysis RunRequest | 旧Analysis同步Run是否允许客户端提供run_id；旧Task与同步Run是否继续共享请求Schema；symbols最多100是否为正式运行时限制；未知TaskType是否应拒绝 | 旧Analysis/Server负责请求校验和默认值实现；OpenAPI与运行时Contract Test同步；不得把Researchflow CreateRunCommand字段机械复制为旧Analysis请求 |
| CTR-006 | 当前Go Task DTO的started_at/finished_at为指针+omitempty，nil时省略；项目公共JSON规则要求Schema明确区分null与缺失 | 无需扩大到Researchflow Run字段；在保持当前Task序列化行为前提下，按实际输出修正Schema即可。若要改成显式null，必须另行批准兼容性变化 | 旧Analysis Task DTO/OpenAPI维护方将nullable改为非nullable可省略语义，补queued/running/terminal JSON Golden和Schema一致性测试 |
| CTR-007 | Researchflow Stage与旧Analysis StageEvent是不同模型；旧事件字段事实已在第9节核实 | 是否继续公开旧Analysis Events端点、是否未来由Researchflow提供独立进度事件流，属于API产品/兼容性决策；不得默认复用Researchflow Stage DTO | 当前旧Analysis Events的生产方/HTTP适配方负责StageEvent Schema、Golden和Handler校验；若未来新增Researchflow事件流，由Researchflow定义独立事件契约，Server负责路由和映射 |

### 10.3 最小决策清单

只保留会改变API兼容性或运行时行为的决策，不重复字段清单：

1. **CTR-046：API边界决策**——旧同步Run/Task/Events API是保留兼容、逐步迁移，还是由Researchflow新API取代；确定最终Root路径和版本。此项决定Schema应该描述哪套生产方。
2. **CTR-047：旧Analysis请求输入决策**——同步Run的run_id是否由客户端指定；Task创建是否忽略/覆盖run_id并由服务端生成；TaskType未知值是否拒绝；symbols的100项限制是否作为真实服务端限制。默认值与historical_replay显式as_of规则按当前Handler事实记录，不得通过Schema required误表达。
3. **CTR-006：序列化兼容决策**——默认保持当前nil省略行为；除非项目负责人明确要求输出null，否则只修Schema，不改运行时DTO。
4. **CTR-007：事件端点生命周期决策**——明确旧Analysis StageEvent是兼容事件契约，不等同Researchflow Stage；如要替换，必须单独定义迁移和兼容策略。

第1、2、4项需要项目负责人确认产品/API边界；CTR-006可按现有生产行为进行契约对齐，不构成独立领域语义争议。

### 10.4 关闭顺序与依赖

按以下顺序交付，避免先写Schema后推翻API边界：

1. **先裁决API边界（CTR-046）**：确认旧Analysis Run/Task/Events保留范围，以及是否接入Researchflow Application Port。没有该决策，不展开RunResult全量Schema。
2. **再冻结请求行为（CTR-047）**：明确同步/异步run_id、TaskType校验和symbols上限；将默认值、条件必填、服务端派生字段写入权威请求定义。
3. **并行修正Task序列化（CTR-006）**：保持nil省略时移除OpenAPI nullable=true，补queued/running/terminal Golden。
4. **按端点边界补事件Schema（CTR-007）**：只为保留的旧Events端点定义StageEvent Schema；若端点退役，提供兼容/废弃策略，不把Researchflow Stage Schema套用过来。
5. **最后完成CTR-046响应Schema**：依据最终路由选择补RunResult或Researchflow Run/Stage/Attempt响应Schema、正常/异常Golden、OpenAPI与真实Handler/Adapter双向Contract Test。
6. **执行验证**：运行定向Go测试、Schema校验和Root集成测试；记录精确SHA与CI结果。Researchflow自身测试通过证据与旧Analysis HTTP测试证据分别记录，不能相互替代。

CTR-008的旧Task Events持久化/DDL/cursor证据不属于本轮决策清单，继续按原唯一登记处理；本轮不重新审查。

### 10.5 本切片结论

- 已找到Researchflow正式Run/Stage/Attempt公开DTO、独立Schema、Golden和投影校验测试代码；这修正了“Researchflow没有DTO/Schema/Golden资产”的潜在误读，但测试执行状态仍未证明通过。
- CTR-046/047/006/007均保持开放；没有新增CTR，也没有关闭任何CTR。
- 当前唯一阻断Schema最终形态的项目级决策是旧Analysis API的保留/迁移边界及请求行为；CTR-006可直接按当前真实序列化行为对齐Schema。
- 本切片状态：complete_with_open_items。仅固定SHA文档/代码证据核对；未修改业务代码、OpenAPI、DTO、Schema、Golden、测试，未执行CI。
- 下一步仍留在Server维度2，先处理项目负责人决策输入与责任方确认；未完成前不进入Server其他五个维度，不切换其他模块。



## 11. Server 维度2第五切片：项目级决策输入收敛（2026-11-10）

本切片将第10节待决项整理为可裁决提案。以下为审计建议，不是已批准的项目决议；在项目负责人确认前，相关CTR保持开放，不擅自修改业务实现或权威契约。

### 11.1 CTR-046：旧Analysis API与Researchflow API边界

**建议默认方案：兼容保留旧路径，Researchflow对外API独立版本化；禁止原路径静默切换底层语义。**

- 保留现有`/api/v1/analysis/runs`、`/api/v1/analysis/tasks`及对应查询/Events路径作为旧Analysis兼容入口；当前消费者和OpenAPI按旧Analysis DTO定义。
- Researchflow若需要对外暴露Run/Stage/Attempt，应使用独立、明确版本化的资源契约；具体路径在API设计决议中确定，不在本审计中臆造。
- 不允许将旧RunRequest直接映射成Researchflow CreateRunCommand：旧请求缺少template_ref/version、system_as_of、deadline_at、input_snapshot_refs、idempotency_key等必填语义；as_of不能自动等同system_as_of。
- 在存在显式、可审计的兼容映射（包括模板选择、时间语义、快照引用、身份与幂等键）前，旧路径不得静默改接Researchflow，也不得声称其已由Researchflow编排。
- 后续迁移应先新增Researchflow API及消费者，再给旧路径增加弃用窗口、迁移指南和停止服务条件；不得原地破坏旧响应结构。

**影响：** 短期保留两套API契约与适配成本；避免把旧Analysis输入语义错误转换成Researchflow命令，也避免现有客户端无提示破坏。长期目标仍须遵守项目级唯一流程编排者原则，不能无限期维持两个独立的跨模块编排实现。

**需确认：** 项目负责人是否接受“旧路径兼容保留、Researchflow独立版本化、未来显式迁移”的默认方案；以及旧Analysis执行链当前是否仅作为迁移期兼容实现。

### 11.2 CTR-047：旧Analysis请求行为

依据固定基线当前Handler/Queue行为，建议明确如下：

| 决策项 | 当前代码事实 | 建议目标契约 | 兼容影响 |
|---|---|---|---|
| 同步Run的run_id | Handler解码后直接交给Runner，Handler不重写；最终是否保留由Runner实现决定 | 不把客户端run_id承诺为权威资源ID；建议由执行端生成权威run_id。若现有Runner确实依赖调用方run_id，先记录为兼容行为，待迁移版本废弃 | 可能影响依赖客户端指定ID的内部调用，需消费者盘点 |
| 异步Task的run_id | CreateTask路径将请求交给Queue；Queue生成并使用自己的RunID | 明确为服务端生成，忽略/拒绝客户端run_id（二选一，建议拒绝非空以避免误导） | 对传入run_id的旧调用方需迁移 |
| task_type | 空值默认post_market_scan；非空未知值在HTTP decode层不被拒绝 | 空值继续默认；非空必须属于四个已定义值，否则400 | 收紧非法请求，合法请求不变 |
| symbols | OpenAPI已有maxItems=100；本轮Handler没有对应数量校验 | 保留100项上限并在HTTP入口执行；Schema和运行时一致 | 超过100项的请求从下游失败/不确定变为明确400 |
| as_of | 默认Asia/Shanghai当前时间；historical_replay必须显式提供 | 保持现有默认与条件必填语义，在Schema描述/条件规则中表达，不能机械标成全局required | 合法请求不变 |
| as_of_mode | 客户端输入会被Handler按as_of是否显式提供重新派生 | 不作为客户端可控输入；从请求Schema移除或明确拒绝该字段，响应如需展示则单独定义 | 传入该字段的客户端可能需要调整 |

**需确认：** 是否接受未知TaskType拒绝、symbols上限由Schema落实到Handler，以及异步run_id非空时拒绝。同步Run的最终run_id规则须先补Runner实现证据，不以Handler未重写推断执行端语义。

### 11.3 CTR-007：旧Analysis Events生命周期

**建议默认方案：保留现有旧Events端点为兼容契约，单独定义StageEvent；Researchflow事件流若建设则另立Schema。**

- `GET /api/v1/analysis/runs/{run_id}/events`与`GET /api/v1/analysis/tasks/{task_id}/events`在无替代消费者迁移方案前继续保留。
- Run Events中的旧Analysis StageEvent按第9节已核实字段定义独立Schema/Golden；Task Events的外层sequence/event与内部事件类型分别定义，不能用Researchflow Stage Schema替代。
- 新Researchflow API如需事件流，必须单独定义事件类型、顺序/cursor、重放边界和版本；不得仅因两者都有stage字段就共享DTO。
- 旧端点弃用必须以消费者迁移完成、兼容窗口和回滚策略为条件。

**影响：** 保持旧客户端稳定，但短期会并存两套进度表达；避免事件字段、状态含义和cursor语义混淆。

**需确认：** 是否接受旧Events兼容保留及独立Schema的方案；是否已有必须立即退役的消费者/安全原因。

### 11.4 CTR-006：可直接执行的Schema修正

该项不需要等待Run API迁移决策：

- Task.started_at/finished_at继续按Go指针+omitempty行为序列化：nil时字段缺失，非nil时输出时间值。
- OpenAPI将nullable语义移除，字段仍为可选；不得把字段设为required，也不得将nil编码为null。
- Golden至少覆盖queued（started_at/finished_at均缺失）、running（started_at存在、finished_at缺失）、terminal（started_at和finished_at存在）；同时验证字段类型为date-time。
- Schema校验应明确区分“可缺失”与“可为null”。

**建议验收：** 三种Task状态的序列化Golden、OpenAPI Schema校验和Handler响应Contract Test全部通过后，才关闭CTR-006。

### 11.5 项目负责人决策清单与状态

| 决策ID | 建议默认方案 | 决策性质 | 当前状态 |
|---|---|---|---|
| D-SRV-API-01（CTR-046） | 旧Analysis路径兼容保留；Researchflow API独立版本化；禁止静默切换 | API边界/兼容策略 | pending_decision |
| D-SRV-REQ-01（CTR-047） | 未知TaskType拒绝；symbols最多100由Handler执行；异步Task run_id服务端生成且拒绝非空客户端值；同步Run ID先补Runner证据 | 请求行为/运行时约束 | pending_decision |
| D-SRV-EVT-01（CTR-007） | 旧Events兼容保留并独立定义StageEvent；Researchflow事件流独立版本化 | 事件生命周期/兼容策略 | pending_decision |
| D-SRV-TIME-01（CTR-006） | 保持nil省略，修正Schema并补三态Golden | 与现有行为对齐，无新增领域决策 | ready_for_implementation |

项目负责人确认前，不把建议写成accepted；责任方确认后，才进入相应Schema/Handler/Golden修复。CTR-046/047/007继续开放，CTR-006仍开放但具备直接实施条件。本切片没有新增CTR、关闭CTR或业务代码变更。
