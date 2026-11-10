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
