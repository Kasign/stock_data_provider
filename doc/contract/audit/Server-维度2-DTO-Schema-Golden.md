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
