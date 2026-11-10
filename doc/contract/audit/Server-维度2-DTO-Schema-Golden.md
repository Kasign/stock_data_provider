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
