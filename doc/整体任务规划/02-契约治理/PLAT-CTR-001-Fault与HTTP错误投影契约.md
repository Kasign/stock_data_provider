# PLAT-CTR-001：Fault 与 HTTP 错误投影契约

- 契约版本：1.0.0
- 状态：项目级公开定义 `freeze-ready`
- 适用范围：Platform 进程内 Fault，以及 Server/领域 HTTP Transport 对外错误投影
- 权威边界：本文定义跨模块错误语义和 HTTP 投影；Platform `doc/platform/08-公共平台契约规范.md` 第 4 节定义进程内 Fault 字段及校验；项目级 Schema/Golden 定义公开 JSON 结构与映射样例。模块内部错误码目录不在本文重复维护。

## 1. 进程内 Fault

Fault 是进程内错误，不是 HTTP DTO，不得直接 JSON 序列化。内部字段为：

| 字段 | 语义 | 是否公开 |
|---|---|---|
| Class | 稳定错误类别 | 是，映射为 `error.class` |
| Code | 稳定机器错误码 | 是，映射为 `error.code` |
| SafeMessage | 已脱敏的安全摘要 | 是，映射为 `error.message` |
| Retryable | 技术上是否可作为重试候选 | 是，映射为 `error.retryable` |
| Dependency | 稳定逻辑依赖标识 | 否 |
| Cause | 内部错误链 | 否 |

Dependency、Cause、堆栈、SQL/DSN、Token/Key、请求/响应 Body、原始 Provider 响应均不得进入公开错误响应。HTTP Transport 必须显式投影到独立 Error Envelope，不得序列化 Fault 本身。

## 2. 公开 Error Envelope

唯一结构为：

```json
{
  "error": {
    "class": "unavailable",
    "code": "news.provider_unavailable",
    "message": "news provider is temporarily unavailable",
    "retryable": true
  }
}
```

字段约束：

| 字段 | 类型 | 约束 |
|---|---|---|
| error | object | 必需；只包含下列定义字段 |
| error.class | string enum | 必需；取第 3 节固定集合 |
| error.code | string | 必需；稳定、机器可读、带 Owner 命名空间 |
| error.message | string | 必需；脱敏摘要，非空，最多 512 个 Unicode 字符；Platform 内部 Fault.SafeMessage 另受内部实现的 512 字节上限约束 |
| error.retryable | boolean | 必需；仅 unavailable/timeout 可为 true |
| error.request_id | string | 可选兼容字段；若存在必须与响应头 X-Request-ID 一致 |

`request_id` 是兼容性字段，不是第二个关联 ID 来源。HTTP 响应头 `X-Request-ID` 是权威值；消费者必须读取 Header。新 HTTP 错误响应无需重复写入 Body。

未知字段按该 DTO 的 Schema 策略处理；不得将内部 Fault 的 Dependency/Cause 加入公开 Envelope。

## 3. Class 与 HTTP 状态映射

| Class | HTTP 状态 | 语义 |
|---|---:|---|
| validation | 400 | 请求结构、参数、枚举或范围无效 |
| unauthenticated | 401 | 缺少或无效的认证凭证 |
| forbidden | 403 | 已认证但无权执行操作/访问资源 |
| not_found | 404 | 目标资源不存在，或按安全策略不可区分地隐藏 |
| conflict | 409 | 当前资源/状态/幂等键与请求冲突 |
| unavailable | 503 | 服务或依赖当前不可用 |
| timeout | 504 | 当前请求等待依赖/下游响应超时 |
| internal | 500 | 未分类内部错误或 Fault 元数据无效 |

HTTP 状态码由 Transport 根据 Class 映射，不由业务调用方任意覆盖。若某协议需要不同的状态映射，必须通过独立、版本化的协议契约声明，不得改变本项目统一 HTTP Error 投影。

本契约没有单独的 rate_limited Class。首期过载/限流按 unavailable/503 表达；如未来确需 429 和独立语义，必须先升级公开 Class/状态映射契约。

## 4. Code 命名与所有权

- Code 必须稳定、可机器处理、全小写 ASCII，并使用点分隔的 Owner 命名空间，例如 `platform.*`、`server.*`、`data.*`、`news.*`、`research.*`、`researchflow.*`、`quant.*`、`llm.*`、`notification.*`。
- 错误码由实际产生错误的 Owner 注册和维护；调用方不得重写为自己的命名空间。
- 同一命名空间内不得为不同语义复用同一 Code；Code 不得包含资源 ID、用户输入、URL、IP、凭证或动态错误文本。
- 新 Code 的新增不改变 Envelope Schema 版本；新增 Class、改变现有 Code 语义或改变 HTTP 状态映射属于破坏性契约变更。

## 5. Retryable 与重试边界

- Retryable 必须存在，默认 false。
- 只有 unavailable、timeout 允许为 true；其他 Class 为 true 属于无效 Fault/无效公开 DTO。
- Retryable 仅说明错误在技术上可能暂态，不构成重试命令或成功概率承诺。
- 消费者只有在请求幂等、调用策略允许、剩余预算充足且不超过总 Deadline 时才可重试。
- Provider/领域策略可以进一步禁止重试；调用方取消、参数错误、认证/授权失败、冲突、内容质量拒绝不得标记为可重试。
- Retry-After 是独立可选响应 Header；只有服务端明确给出等待时间时才发送，消费者不得仅凭 retryable 推断固定等待时长。

## 6. Request ID 与响应头

- 每个受本契约保护的 HTTP 响应必须返回 `X-Request-ID`。
- 若 Error Envelope 包含可选 `request_id`，其值必须与 `X-Request-ID` 完全一致。
- 消费者以 Header 关联日志、请求和错误；Body 字段只为旧消费者兼容，不得用于覆盖 Header。
- Error 响应使用 JSON；不得缓存错误正文。Transport 应设置 `Content-Type: application/json` 和 `Cache-Control: no-store`。

## 7. Schema 与 Golden

- Schema：`schema/PLAT-CTR-001-http-error-1.0.0.schema.json`。
- Golden：`golden/PLAT-CTR-001-http-error-1.0.0.golden.json`。
- Golden 覆盖全部八种 Class、固定 HTTP 状态、必需字段、Retryable 约束，以及 request_id 可选且与 Header 一致的规则。
- Schema 校验单个 Error Envelope；Golden/语义校验负责断言 HTTP 状态、Header 与 Body 的跨层关系。

## 8. 实现符合性边界

以下不属于项目级定义冻结门禁，后续按生产方/消费者 L2 验收：
- 各模块完整错误码目录及每个用例的错误分类；
- 各模块 Fault 构造、错误包装和 HTTP Handler 实际映射；
- 错误码是否与真实故障条件一致；
- Retryable 在 Provider/Storage/LLM 各依赖上的真实暂态识别；
- 各消费者对状态码、错误码和 Retryable 的实际处理；
- 真实 HTTP Header、中间件、日志关联与部署代理行为。

上述实现偏差不得反向改变本项目级契约定义；发现不符合时由相应 Owner 修复实现。
