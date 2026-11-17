# PLAT-CTR-001 公共 HTTP Error Envelope 与 Fault 投影

- 契约编号：PLAT-CTR-001
- 契约版本：1.0.0
- 状态：freeze-ready（待项目负责人确认冻结）
- 适用范围：Root Server 及所有通过 HTTP 对外暴露的模块 API
- 权威性：本文件定义公共 HTTP Error DTO 与内部 Fault 的投影边界；领域错误码由各模块 Owner 维护。

## 1. 内部 Fault 与公开 DTO

- `platform.Fault` 仅供进程内部使用，不得直接 JSON 序列化。
- Server 必须显式投影到独立 ErrorEnvelope。
- 映射关系：
  - `Fault.Class` → `error.class`
  - `Fault.Code` → `error.code`
  - `Fault.SafeMessage` → `error.message`
  - `Fault.Retryable` → `error.retryable`
  - 可信 Request Context → 可选 `error.request_id`
- `Fault.Dependency`、`Fault.Cause`、堆栈、原始 Provider 错误和内部诊断细节不得进入公开 DTO。

## 2. ErrorEnvelope 结构

顶层必需字段：
- `schema_version`：ErrorEnvelope 自身 Schema 版本，首版固定 `1.0.0`；
- `error`：错误信息对象。

`error` 必需字段：
- `class`：稳定错误类别；
- `code`：稳定机器错误码；
- `message`：安全的人类可读摘要；
- `retryable`：是否存在可重试的瞬时条件。

`error.request_id` 可选。若返回，必须与响应 Header `X-Request-ID` 完全一致；Header 是权威值。新消费者不得只依赖 body request_id。

不定义 `status`、`dependency`、`cause`、`stack`、`details` 或原始响应字段。HTTP 状态码由响应状态行表达，不重复写入 body。

## 3. 稳定错误类别

| class | HTTP 状态 | 说明 |
|---|---:|---|
| validation | 400 | 请求字段、格式或业务输入不满足契约 |
| validation（请求体超限） | 413 | HTTP transport 限制拒绝请求体 |
| validation（媒体类型不支持） | 415 | HTTP transport 不接受当前 Content-Type |
| unauthenticated | 401 | 缺少或无效认证 |
| forbidden | 403 | 已认证但无权执行 |
| not_found | 404 | 目标资源不存在或按安全策略不可见 |
| conflict | 409 | 当前资源/幂等状态与命令冲突 |
| unavailable | 503 | 服务或依赖当前不可用 |
| timeout | 504 | 当前请求/依赖调用超时 |
| internal | 500 | 未分类或内部错误 |

类别集合是 closed enum。未知 class 不得映射为 success、validation 或其他默认类别；无效内部 Fault 必须归一为 internal/500。

## 4. 字段规则

- `code`：1..160 字符，小写 namespaced identifier，匹配 `^[a-z0-9][a-z0-9._-]{0,159}$`。Code 稳定且供机器判断；各 Owner 维护自己的 namespace。
- `message`：1..512 字符，安全、人类可读；不得包含堆栈、内部 Cause、凭证、敏感信息、原始 Provider 错误或请求体。
- `retryable`：必需 boolean。true 只允许用于 unavailable 或 timeout；其他 class 必须为 false。
- `request_id`：可选，遵循 PLAT-CTR-002 的 X-Request-ID 格式；若 body 出现，必须与同一响应的 X-Request-ID 相等。
- `schema_version`：必需，固定为当前 ErrorEnvelope Schema 的 `1.0.0`。

`retryable=true` 不是客户端自动重试命令。调用方仍须检查 GOV-CTR-003 的幂等键、操作副作用、重试预算和 deadline；相同 correlation_id 不代表可安全重放。

## 5. HTTP Header 与安全要求

- `Content-Type: application/json; charset=utf-8`
- `Cache-Control: no-store`
- `X-Content-Type-Options: nosniff`
- `X-Request-ID` 由 PLAT-CTR-002 负责生成/传播，错误响应仍须回显。
- `Retry-After` 可选，仅在 Server 明确给出退避时间时返回；不得仅由 retryable 推导固定延迟。
- 不得暴露 Fault.Dependency、Cause、堆栈、SQL、内部 URL/IP、Provider 原始报错、凭证或其他敏感信息。

## 6. 责任边界

- Platform 负责内部 Fault 结构、校验和稳定类别。
- Server 负责 Fault → ErrorEnvelope/HTTP status/Header 的投影。
- 各领域模块负责自己的稳定 Error.Code、SafeMessage 和业务错误归类；不得自行定义第二套公共 Envelope。
- PLAT-CTR-002 负责 X-Request-ID Header 和可信 Request Context；GOV-CTR-003 负责 Request/Correlation/Idempotency 身份边界。
- 旧 Data API 如保留 body request_id，必须遵守“若出现则与 X-Request-ID 一致”；新 API 以公共 ErrorEnvelope 为准。

## 7. Golden 覆盖

规范 Golden 文件覆盖 validation 400/413/415、unauthenticated 401、forbidden 403、not_found 404、conflict 409、unavailable 503、timeout 504、internal 500，并覆盖：
- retryable=true 仅出现在 unavailable/timeout；
- body request_id 可选且若存在与 X-Request-ID 一致；
- 公开 body 不含 Dependency/Cause/stack；
- 所有响应 Header 满足 Content-Type/no-store/nosniff；Retry-After 只在显式退避时出现。

## 8. 版本与兼容

ErrorEnvelope 按 GOV-CTR-004 独立使用 schema_version。新增/删除字段、改变 class 语义、code 约束、HTTP 映射或 retryable 规则按 GOV-CTR-004 判定版本；不得原地改变 1.0.0。
