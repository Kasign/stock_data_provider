# SRV-CTR-001 Researchflow HTTP API 契约

- 契约编号：SRV-CTR-001
- 契约版本：1.0.0
- 状态：FROZEN
- 冻结日期：2026-11-23
- 适用分支：repair_all
- 生产方：Root Server（HTTP Handler、Owner Session 认证适配、Run/Stage DTO 投影）
- 消费方：WebUI
- 领域 Owner：Researchflow（Run 创建、读取、取消、资源级授权与执行状态）
- 依赖：SRV-CTR-002、PLAT-CTR-001、PLAT-CTR-002、PLAT-CTR-003、Researchflow Run/Stage/ResultSummary Schema 1.0.0

本契约取代同目录下早期 DRAFT 提案内容。契约冻结只表示 HTTP 边界、字段和认证规则已确定，不代表 Handler、Router、WebUI 或生产运行验收已经完成。

## 1. 范围与接口

首期只开放以下三个接口：

| 方法 | 路径 | Application | 用途 |
|---|---|---|---|
| POST | `/api/v1/researchflow/runs` | `CreateRunService.CreateRun` | 创建 Run 或按幂等键重放 |
| GET | `/api/v1/researchflow/runs/{run_ref}` | `GetRunService.GetRun` | 查询 Run 与 Stage 聚合状态 |
| POST | `/api/v1/researchflow/runs/{run_ref}/cancel` | `CancelRunService.CancelRun` | 持久化取消意图并返回最新聚合状态 |

不开放 Run 列表/搜索、模板 CRUD、事件流、Attempt 明细、Stage Output 内容读取、通知管理、Quant/Research 独立 API。WebUI 首期通过 GET 轮询 Run/Stage 聚合状态。

## 2. 认证与请求安全

1. 三个接口均只允许已启用且通过验证的 Root Owner Session。
2. 认证凭据为 SRV-CTR-002 定义的 HttpOnly Cookie（`__Secure-owner_session`）；不得接受 Owner Bearer、Service Token 或匿名访问。
3. 所有请求必须通过现有 Server `SessionAuthenticator`。GET 执行 Session/CredentialVersion/Host/Origin 校验；不得绕过 Session Middleware。
4. 两个 POST 必须校验 Host、唯一且匹配 PublicOrigin 的 Origin，以及 `X-CSRF-Token`；校验必须使用 Platform 原子 Session/CSRF 操作，错误 CSRF 不得 Touch idle TTL。
5. 同时提供 Session Cookie 与 Authorization Header 时拒绝请求，不进行身份优先级选择。
6. Owner Session 只提供可信 Owner Caller 身份，不继承 `platform:admin`、Service Scope、Owner Bearer，也不直接授予 Quant 或其他领域资源权限。
7. CallerRef 必须从 SessionAuthenticator 写入的可信 Principal 解析；禁止从请求体、Query、Path 或自定义 Header 接收 caller_ref。
8. Request ID 从 Platform Request Context 获取；缺失时由 Root HTTP Middleware 生成，不信任客户端传入值。
9. 所有响应设置 `Cache-Control: no-store`；JSON 使用 UTF-8。请求体受 Root 已配置的全局大小限制。

## 3. 创建 Run

### 请求

`POST /api/v1/researchflow/runs`

必须设置 `Content-Type: application/json` 并提交：

```json
{
  "schema_version": "1.0.0",
  "idempotency_key": "client-generated-stable-key",
  "template_ref": "news.search",
  "template_version": "1.0.0",
  "input_snapshot_refs": [],
  "parameters": {
    "query": "用户提交的研究问题",
    "limit": 20
  }
}
```

字段规则：

- `schema_version` 固定为 `1.0.0`。
- `idempotency_key` 为客户端生成的稳定逻辑提交键，长度和字符校验遵循现有 Researchflow Domain；网络重试必须复用同一个键。
- `template_ref` 与 `template_version` 必须精确指定已发布模板，不接受 latest、隐式默认版本或服务端猜测。
- `input_snapshot_refs` 必须显式提供数组；News Search 模板允许空数组。Server 不生成或伪造 SnapshotRef。
- `parameters` 必须为 JSON object；首期 News Search 参数按 `news-search-parameters-1.0.0.schema.json` 校验，必填 query、limit（1–100），可选 query_dimension，拒绝额外字段。
- 拒绝未知顶层字段、非 JSON、尾随第二个 JSON 值和非 object 的 parameters。

Server 不接收或信任 `caller_ref`、`request_id`、`correlation_id`、`run_ref`、`stage_ref`、`system_as_of`、`deadline_at`。这些由可信 Principal、Platform Request Context、Database UTC Clock、Researchflow IDGenerator 和现有 Deadline 配置生成。

幂等作用域和指纹完全沿用现有 CreateRun Application：authenticated caller + CreateRun + idempotency_key；指纹绑定 Schema 版本、精确模板身份、规范化输入引用及 JCS 参数。同意图返回原 Run，不同意图返回 409。幂等重放先于模板解析和数据库时钟读取。

### 成功响应

HTTP 200，返回 Run 聚合响应（见第 5 节）。新建与幂等重放使用同一响应 DTO；不增加重复的 replay 状态字段。

## 4. 查询 Run

`GET /api/v1/researchflow/runs/{run_ref}`

- `run_ref` 必须是非空、无首尾空白、长度不超过 255 的现有 Researchflow Run 引用。
- 必须先由 `GetRunService` 执行 Run 资源级读取授权，再读取 Run/Stage。
- 不提供旁路 Repository 查询，不允许 Server 自行重做 Run/Stage 归属校验。
- 成功返回 HTTP 200 和第 5 节聚合 DTO。

## 5. Run 聚合响应 DTO

响应结构：

```json
{
  "schema_version": "1.0.0",
  "run": {},
  "stages": [],
  "result_summary": null
}
```

- 顶层 `schema_version` 固定为 `1.0.0`。
- `run` 严格遵循 `doc/researchflow/schema/run-1.0.0.schema.json`。
- `stages` 中每一项严格遵循 `doc/researchflow/schema/stage-1.0.0.schema.json`。
- `result_summary` 为 null 或严格遵循 `doc/researchflow/schema/result-summary-1.0.0.schema.json`。
- 终态 Run 必须同时包含非空 `run.result_summary_ref` 和顶层 `result_summary`；非终态 Run 的 `result_summary_ref` 必须为 null，顶层 `result_summary` 必须为 null。
- DTO 必须显式投影 Domain。禁止直接 JSON Marshal Domain：Run 的 ResultSummaryRef 与 ExcludedStages 不直接导出；Stage Domain 中的 QualityStatus、RiskDecision 等不属于当前 Stage Schema，不能意外泄露。
- Stage DTO 必须包含 Schema 要求的 `schema_version`、`retry_at`、`output_ref`、`quality_summary_ref`、`diagnostic_refs` 等字段；空集合输出空数组，空引用输出 null。
- 首期不返回 Attempt 明细、Stage Output 内容或内部 Domain/Storage 字段。

## 6. 取消 Run

`POST /api/v1/researchflow/runs/{run_ref}/cancel`

- 不要求请求体；空请求体允许，非空请求体拒绝，避免客户端误以为可提交未定义参数。
- 必须通过 Owner Session、Origin/Host 与 CSRF 校验。
- 由 `CancelRunService` 执行资源级取消授权、终态检查、并发版本控制和取消意图持久化。
- 成功返回 HTTP 200 和第 5 节最新 Run 聚合响应。
- 成功只表示取消意图已经持久化，不表示 Worker 已立即停止；最终状态由 Researchflow Worker/Finalizer 决定。
- 已取消意图的幂等重放仍返回当前聚合状态；终态/不可取消/并发冲突返回 409。

## 7. HTTP 状态与错误投影

所有错误使用 Platform/Server 既有 Public Error Envelope；不创建 SRV-CTR-001 私有错误信封，不序列化内部 Fault、SQL 错误或堆栈。

| HTTP | 语义 |
|---|---|
| 400 | JSON/Schema/路径引用无效，未知字段，非空取消请求体，或同时提交 Session 与 Authorization 凭据 |
| 401 | Owner Session 缺失、无效、过期或 Principal 不符合 Owner Session 身份 |
| 403 | Origin/Host/CSRF 拒绝，或 Researchflow Run 资源级授权拒绝 |
| 404 | Run 不存在；仅当 Repository 返回可识别的 not-found 时映射 |
| 409 | 幂等键意图冲突、Run 终态/不可取消或版本并发冲突 |
| 413 | Root 请求体大小限制拒绝 |
| 429 | 仅当现有 Session/平台限流机制对该请求明确返回限流时 |
| 5xx | 存储、授权依赖、内部故障；按现有 Platform Fault 分类映射，不得伪装成 404 或成功 |

具体稳定错误码必须复用实现层既有 Fault/Public Error Code；不得在 Handler 中另造一套平行领域错误码。实现时对每个状态码建立错误映射测试。

## 8. 业务边界与首期验收含义

- 首期已注册模板为 News Search。该 API 仅验收 Researchflow Run 创建、Worker 执行、状态查询与取消基础链路。
- News Search 成功不代表完整 A 股研究分析、Quant 生产计算、Research Context/Evidence、LLM 分析、硬风险门禁或最终研究报告已经完成。
- Provider 数据质量、算法正确性、证据 PIT 和领域规则仍归各自模块；不改变本 HTTP 契约。
- API 不绕过 Run/Stage 的现有持久化、授权、Worker、Finalizer 或 Outbox 机制。

## 9. 版本与变更规则

- 契约版本：1.0.0；请求/响应 DTO Schema Version：1.0.0。
- OpenAPI 权威文件：`doc/server/openapi/researchflow.openapi.yaml`。
- JSON Schema 权威文件：`doc/server/schema/SRV-CTR-001/`。
- Golden：`doc/server/golden/SRV-CTR-001/`。
- 任何路径、方法、请求字段、响应投影、认证矩阵、CSRF/Origin 语义或错误语义的非兼容变更必须升级 SRV-CTR-001 主版本，并评估 WebUI 迁移。
- 本契约是 HTTP 边界权威定义；Researchflow Domain/Application Schema 继续由 Researchflow 模块拥有，Server 不复制其领域定义。

### 9.1 请求 Schema 版本演进：create-run-request-1.1.0

- `create-run-request-1.1.0.schema.json` 是本契约的**新增请求版本**，由 DEC-PLAT-002-063 决议引入；`create-run-request-1.0.0.schema.json` 保持冻结、字节不变，其 News Search 绑定语义继续有效。
- 动机：1.0.0 把 `parameters` 直接 `$ref` 到 `news-search-parameters-1.0.0`，并把 `template_ref`/`template_version` 固定为 `news.search`/`1.0.0`，使 Quant 与 per-Stage 模板在契约层没有合法请求形态。这与实现不一致——Server 运行时**不**按该 Schema 校验请求，参数实际由 `template_ref` 指向模板的 `ParameterSchemaRef` 校验。
- 1.1.0 只放宽三处：`schema_version` 的 `const` 改为 `1.1.0`；`template_ref`/`template_version` 从固定值改为非空无首尾空白字符串；`parameters` 从 News 参数 `$ref` 改为不透明 JSON object。**HTTP 路径、方法、认证矩阵、CSRF/Origin 语义、响应 DTO 与错误语义均未改变**，因此不升级 SRV-CTR-001 主版本。
- `parameters` 的具体形状由目标模板的参数 Schema 拥有：News Search 仍为 `news-search-parameters-1.0.0`；per-Stage 模板（如 `quant.research`）为 `{"stages":{"<stage_code>":{...}}}` 信封。1.1.0 只约束 `parameters` 必须是 JSON object，不复制任何模板参数约束。
- 兼容性：1.0.0 的 News 请求在 1.1.0 下依然合法；客户端按模板选择对应 `schema_version`。本版本属 additive 文档演进，不改变运行时行为，也不提前解冻任何契约。

### 9.2 结果读取响应版本演进：result-read-response-1.1.0（additive 路由与响应）

- `result-read-response-1.1.0.schema.json` 是本契约的**新增响应版本**，由 DEC-PLAT-002-064 决议引入；1.0.0 的三条 Run 路由及其响应 Schema 保持冻结、字节不变。
- **新增路由**：`GET /api/v1/researchflow/runs/{run_ref}/results`。认证矩阵与第 2 节完全一致：仅 Root Owner Session（`__Secure-owner_session`），拒绝 Owner Bearer、Service Token、匿名，GET 执行 Session/CredentialVersion/Host/Origin 校验。路径 `run_ref` 必须是非空、无首尾空白、长度不超过 255 的现有 Run 引用。
- **语义**：鉴权后必须先由 `GetRunService.GetRun` 执行 Run 资源级读取授权（禁止旁路 Repository），再由 Server 以进程内可信身份调用各领域 Owner 的授权读取用例（Quant `GetResult` / News `GetEvidence`），把已授权结果投影为受限公开 DTO。**这不是完整研究报告**：`resource_kind` 固定为 `authorized_domain_results`，不含 Run/Stage 聚合字段、不含完整报告正文。
- **响应**：`schema_version` 固定 `1.1.0`；`results[]` 每项为已授权 Envelope 的受限投影，只允许 `indicator`/`factor`/`trend`/`risk` 四种 `result_type`。`risk` 项**禁止**携带 `payload`（RiskDecision 原始结构归 RF-BLOCK-008 / 2.4 决议），其 `result_status` 为 `rejected` 或 `unknown`；其余三项携带经 QNT-CTR-004 校验的强类型 `payload`。Stage Output 内容、Attempt 明细、`domain.Run` 内部字段与 `RiskDecision` 原始结构均不得出现在响应中。
- **可用性门控**：仅当组合根已装配 Quant 授权结果读取链（Stage Output Reader + GetResult）时注册该路由；未装配时返回 404 `researchflow.result_read_unavailable`，不返回空壳结果。
- **错误语义**：`401` 未认证、`403` Run 读取授权拒绝、`404` Run 不存在或结果读端点未装配、`5xx` 存储/授权/关联完整性故障。跨 Run、无关联、伪造 RunRef、当前策略撤销期间的读取一律 fail-closed，不得降级为空结果或 200。
- **兼容性**：本版本是 additive 路由与响应演进，不改变第 1 节的接口清单、第 3～6 节既有路由语义或第 7 节错误码；不升级 SRV-CTR-001 主版本。`doc/server/openapi/researchflow.openapi.yaml` 增加该路径，既有三条路径不变。
