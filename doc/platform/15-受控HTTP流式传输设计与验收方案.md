# PLAT-CTR-005 扩展：受控 HTTP 流式传输设计与验收方案

- 所属模块：Platform
- 实现分支：`feat/platform-foundation`
- 代码范围：`internal/platform/httpclient/**`
- 文档状态：A1 验收通过，可供领域模块接入；A2/A3 仍待完成
- 消费方：LLM 首个消费者；其他领域按实际需求接入
- 契约归属：现有 PLAT-CTR-005（HTTP Client）扩展，不另行创建第二套 HTTP Client

## 1. 建设边界

在现有安全 HTTP Client 上增加流式响应消费方式。流式入口只负责受控请求、响应头交付、响应体增量读取、资源限制、取消、关闭和传输错误分类。

不解析 SSE、JSON、模型事件、工具调用或业务终态；不实现重试编排、模型协议和业务错误映射。

缓冲式 `Client.Do()` 与流式 `Client.OpenStream()` 是两个明确入口。现有 `Do()` 的完整缓冲、有限幂等重试语义不改变。

## 2. 现有实现审查结论

审查基线：`eb0a4b74082e6bd7a58d3ffe2eb418379c6a97e0`。

### 2.1 可直接复用

- `DestinationPolicy`、`validatePolicy`、`validateURL`：固定 Dependency、精确 Host/Port allowlist 和 URL 校验。
- `dialContext`、`resolveAndValidate`：每次新建连接重新解析 DNS、拒绝危险 IP、直接连接校验后的 IP，避免校验后再次解析。
- `http.Transport`：连接池、连接超时、TLS 握手超时、响应头超时和禁用环境代理。
- `validateRedirect`：默认拒绝重定向；显式开启时限制同源、HTTPS 不降级、跳数和循环。
- `BoundDoer`：固定绑定单一 endpoint 的 DestinationPolicy，可增加流式方法，不允许调用方逐请求替换策略。
- `observability.PropagateRequestID`：出站 Request ID 传播。
- `fault.Fault`：稳定 Class、Code、SafeMessage、Retryable 和内部 Cause。

### 2.2 不能直接复用的生命周期

当前 `Client.Do()` 在方法内创建 `context.WithTimeout`，并通过 defer 立即取消；同时在返回前完整读取响应体、关闭底层 Body 并释放 Semaphore。若简单返回未缓冲 Body，方法返回时 Context 已取消，连接并发额度也已释放，流式读取会被提前终止或绕过资源控制。

因此流式入口必须独立管理 Context cancel、响应体、并发额度和终止状态，不能复用 `Do()` 的 defer 生命周期。

### 2.3 本次实现范围

- Transport 已显式配置 `MaxResponseHeaderBytes`；默认值和硬上限已进入配置校验，仍需 CI 验证边界行为。
- 本次新增独立流式累计字节数上限，仍需 CI 验证实际边界行为。
- 本次新增流式并发额度、空闲读取超时和流生命周期统计，仍需 Race 验证。
- 流式入口已增加响应头超时、连接失败和 DNS 失败的独立 Fault 分类，仍需 CI 验证不同 Transport 错误类型的映射。

## 3. 公共 API 方向

建议保留现有 `BoundDoer`，在其上增加固定策略的流式入口：

```go
type StreamResponse struct {
    StatusCode int
    Header     http.Header
    Body       io.ReadCloser
}

func (r *StreamResponse) Stats() StreamStats

func (c *Client) OpenStream(
    ctx context.Context,
    request *http.Request,
    policy DestinationPolicy,
) (*StreamResponse, error)

func (d *BoundDoer) OpenStream(
    ctx context.Context,
    request *http.Request,
) (*StreamResponse, error)
```

接口已按上述方向提交实现；最终稳定契约以通过 A1 验收的代码为准。

契约要求：

1. `ctx` 是本次流请求的权威 Context，必须同时控制排队、DNS、连接、响应头和 Body 读取；请求对象通过 `Clone(ctx)` 绑定该 Context。
2. `BoundDoer.OpenStream` 始终使用创建时复制并固定的 DestinationPolicy。
3. 成功返回后，响应头和状态码立即交付；Body 是底层响应体的受控包装，不等待 EOF，不整体缓冲。
4. `Header` 返回独立快照；调用方不能通过修改 Header 改变底层请求或传输策略。
5. Body 所有权转移给调用方。调用方必须在 EOF、错误或提前退出时 Close；Close 幂等，并负责取消请求、关闭底层 Body 和释放并发额度。
6. Body 到达 EOF、读取失败或 Context 取消时，Platform 自动执行一次清理；调用方仍应 defer Close。
7. HTTP 4xx/5xx 是正常收到的 HTTP 响应，不由传输层自动转换成 Fault；协议/业务错误由消费者判断。
8. 流式请求不执行自动重试。尤其在任何数据交付后，绝不能透明重试或拼接第二条响应。

使用示例（由调用方处理业务数据；Platform 不解析流协议）：

```go
bound, err := client.Bind(policy)
if err != nil {
    return err
}
stream, err := bound.OpenStream(ctx, request)
if err != nil {
    return err
}
defer stream.Body.Close()

// 状态码和 Header 在读取 Body 前已经可用。
if _, err := io.Copy(destination, stream.Body); err != nil {
    return err
}
stats := stream.Stats()
_ = stats // 由调用方写入自身的结构化日志/诊断记录
```

## 4. 生命周期与资源控制

### 4.1 生命周期

```text
申请流式并发额度
    -> 绑定 Context 与固定 DestinationPolicy
    -> 安全 DNS/IP 校验及受控 Dial
    -> 等待响应头（受 ResponseHeaderTimeout 和 Context 控制）
    -> 返回状态码、Header 和受控 Body
    -> 调用方增量 Read
    -> EOF / Read Error / Context Cancel / Close
    -> 关闭底层 Body、取消派生 Context、释放流式额度
```

流式额度必须从请求执行前持有到流终止，不能在收到响应头后释放。流式额度与缓冲式请求额度分离，避免长连接占满普通 Provider 请求的全部并发槽位；未配置专属 Dependency 的流共享有界默认池，不能按请求输入动态创建 Semaphore。

### 4.2 建议的独立配置

以下是首期建议默认值和硬上限，编码前需在 Platform 配置约束中固化：

| 配置 | 建议默认值 | 建议硬上限 | 说明 |
|---|---:|---:|---|
| `MaxResponseHeaderBytes` | 1 MiB | 4 MiB | 设置到共享 Transport，显式限制响应头 |
| `MaxStreamBytes` | 16 MiB | 64 MiB | 按实际交付给调用方的解压后字节累计 |
| `StreamIdleTimeout` | 30 秒 | 5 分钟 | 单次阻塞 Read 等待上游数据的最长时间 |
| `StreamTimeout` | 10 分钟 | 30 分钟 | 流生命周期总上限；调用方 Context 更短时以调用方为准 |
| `MaxConcurrentStreams` | 2 | 8 | 独立于缓冲请求的全局流并发额度 |
| `StreamDependencyLimits` | 未配置时使用默认池 | 每项不超过全局上限 | 按稳定 Dependency 做流并发隔离 |

`Config.Timeout` 继续只约束缓冲式 `Do()`，不能将现有 15 秒默认值直接套到模型长流。流式请求使用独立 `StreamTimeout` 派生 Context，父 Context 的 deadline/cancel 优先生效。

连接超时（覆盖 DNS 解析和实际 Dial）、TLS 握手超时和响应头超时继续复用现有配置并分别映射 Fault。响应头超时与流空闲读取超时不是同一个概念。

### 4.3 增量读取与背压

首期直接包装 `http.Response.Body`，不建立后台读取泵、不创建无界 Channel、不预读完整响应。调用方的 Read 速度就是网络消费速度，TCP/Transport 自然形成背压。

空闲超时在每次阻塞 Read 期间生效：Read 开始时启动/重置定时器，成功返回数据或 Read 结束时停止；定时器触发时关闭底层 Body，使阻塞 Read 退出。调用方两次 Read 之间主动处理数据的时间不计为上游空闲，避免慢消费者被误判为上游故障。

该方案不额外创建常驻读取 goroutine。Context 取消回调、超时定时器和 Close 必须通过幂等终止状态协调，防止重复关闭、重复释放或终止原因被覆盖。

### 4.4 字节上限

- 不能依赖 Content-Length；必须在每次 Read 路径累计实际字节数。
- 统计解压后的 Body 字节，避免 gzip 小包解压成大内容绕过上限。
- 恰好达到上限不立即判定超限；下一次读取需探测是否存在额外字节。若存在，关闭流并返回明确的超限 Fault；若上游恰好 EOF，则正常结束。
- 不得向调用方交付超过上限的探测字节。
- 超限后自动关闭底层 Body、取消 Context 并释放并发额度。

## 5. 安全策略继承

流式请求必须使用与 `Do()` 相同的 `http.Client`、Transport、DestinationPolicy 和受控 Dial 路径：

- DNS A/AAAA 全量校验，混合公网/私网答案整体拒绝。
- 每次新建连接重新解析和校验，实际 Dial 到已校验 IP。
- 私网、环回、link-local、保留地址和 IPv4-mapped IPv6 拒绝。
- 环境代理关闭；拒绝自定义 Host 和自定义 Accept-Encoding。
- 重定向默认拒绝；显式允许时继续使用现有同源限制、逐跳 URL 校验、跳数限制和 HTTPS 降级拒绝。
- 不允许流式入口接受裸 `http.Client`、自定义 Transport 或跳过 DestinationPolicy。
- 共享 Transport 显式设置响应头上限；流式 Body 包装层执行解压后累计字节上限。

首期不放开跨源重定向。若未来确需跨源 CDN 跳转，必须另行设计凭据剥离/重建规则，不能仅凭 Host allowlist 放行。

## 6. 错误与 Fault 契约

Read 错误必须通过稳定 Fault Code 区分；Fault 的 SafeMessage 为固定摘要，Cause 只留在内部错误链：

| 场景 | 建议 Code | Class | Retryable |
|---|---|---|---|
| 目标策略拒绝 | `platform.httpclient.stream_destination_rejected` | validation | false |
| 响应头超过上限 | `platform.httpclient.stream_headers_too_large` | validation | false |
| 建连失败 | `platform.httpclient.stream_connect_failed` | unavailable | 仅在策略明确幂等且尚未返回响应时可为 true |
| 建连超时 | `platform.httpclient.stream_connect_timeout` | timeout | 按请求幂等性判断 |
| TLS 握手超时 | `platform.httpclient.stream_tls_handshake_timeout` | timeout | 按请求幂等性判断 |
| 响应头超时 | `platform.httpclient.stream_header_timeout` | timeout | 按请求幂等性判断 |
| 单次读取空闲超时 | `platform.httpclient.stream_idle_timeout` | timeout | false |
| 调用方 Context 取消 | `platform.httpclient.stream_canceled` | unavailable | false |
| Context / Stream 总期限到期 | `platform.httpclient.stream_deadline_exceeded` | timeout | false |
| Body 超限 | `platform.httpclient.stream_body_too_large` | validation | false |
| 上游提前断开/Unexpected EOF | `platform.httpclient.stream_upstream_truncated` | unavailable | false |
| 其他读取错误 | `platform.httpclient.stream_read_failed` | unavailable | false |

补充规则：

- 流式模式不自动重试；Retryable 仅表达调用方是否可考虑重新发起一条新请求，不授权 Platform 重放。
- 已向调用方交付任何响应数据后，所有读取终止错误 Retryable 必须为 false。
- 调用方主动 Close 不作为上游故障，不生成 timeout/failed Fault。
- 父 Context 取消与父 Context deadline 必须分别分类；不能把调用方取消记为上游不可用。
- HTTP 状态码错误由调用方按协议处理，不与连接/读取 Fault 混为一类。
- 不把 URL Query、Authorization、Cookie、API Key、请求/响应 Body、原始上游错误正文放入 Code、SafeMessage、Dependency 或公开诊断字段。

## 7. 诊断信息

每条流通过 `StreamResponse.Stats() StreamStats` 提供只读统计快照，至少包括：

- Request ID；
- 逻辑 Dependency；
- HTTP Status Code（已获得响应头时）；
- 开始时间、持续时间；StartedAt 从 OpenStream 调用开始计时，Duration 覆盖并发排队、DNS/Dial、TLS、响应头等待和 Body 生命周期；
- 实际交付字节数；
- 终止原因：EOF、caller_close、context_cancel、deadline、idle_timeout、body_limit、read_error。

统计字段只包含固定白名单元数据，不包含 URL、Query、Header、Body 或原始错误字符串。调用方在流结束后可将快照写入自身结构化日志/领域诊断记录。Platform 不建设 HTTP 流历史数据库，也不把每次请求伪装成 Diagnostics Runner Check。

## 8. 测试与验收矩阵

| # | 验收项 | 核心断言 |
|---:|---|---|
| 1 | EOF 前收到首段 | 服务端分段写入，客户端在服务端结束前读到首段 |
| 2 | 慢速持续输出 | 首段及时交付；内存不随总响应大小增长 |
| 3 | Context cancel | 阻塞 Read 及时退出，返回 canceled Fault |
| 4 | Context deadline | 阻塞 Read 及时退出，返回 deadline Fault |
| 5 | 主动 Close | 并发额度释放，底层连接资源关闭；重复 Close 安全 |
| 6 | 上游提前断开 | Content-Length/分块场景返回截断错误，不伪装 EOF |
| 7 | Body 超限 | 超过配置上限即终止，超限探测字节不交付 |
| 8 | 空闲读取超时 | 单次阻塞 Read 超时退出；慢消费者暂停读取不误触发 |
| 9 | 重定向 SSRF | 默认拒绝、同源跳转可用、跨源/HTTPS 降级拒绝，危险目标不发生 Dial |
| 10 | 慢消费者背压 | 无后台无界队列；单流额外缓冲有固定上限 |
| 11 | Fault/脱敏 | 错误码分类稳定，Cause 不进入公开统计/日志字段 |
| 12 | 并发退出 | 并发 Close/Cancel/Read Error 后额度只释放一次，无 goroutine/连接泄漏 |
| 13 | 附加资源边界 | 响应头上限、解压后 Body 上限、BoundDoer 固定策略和无自动重试均有回归测试 |
| 14 | 建连超时分类 | Dial timeout 与 TLS handshake timeout 不误归类为响应头超时 |
| 15 | 诊断耗时 | StreamStats.Duration 覆盖 OpenStream 到流终止的完整耗时 |

测试必须同时覆盖 HTTP/1.1 和 HTTP/2 可适用的生命周期行为；使用本地 httptest、可控 Resolver/Dial 和短超时，不依赖公网。至少执行 gofmt、平台包单测、Race、Vet。

## 9. 实施顺序与准入门禁

1. 先冻结本设计中的 API、默认值、错误码和终止状态语义。
2. 实现流式并发额度、Context 生命周期和幂等清理。
3. 实现增量 Body、空闲读取超时、解压后字节上限和响应头上限。
4. 接入 Fault 与终态统计。
5. 完成第 8 节全部测试及 Race/Vet。
6. 更新 Platform 接入示例；由 LLM 使用 BoundDoer 流式入口做跨模块验收。

A1 的 gofmt、Platform 单测、Race、Vet、Schema/Golden 和 Storage integration 已全部通过，当前可供业务模块接入。A2 需要 LLM 真实调用与取消/关闭联调；A3 仍需部署网络和运行时证据。

## 10. 明确不做

- SSE event/id/data/retry 解析；
- Responses API 事件识别、模型输出聚合和工具调用；
- 领域重试、业务终态、模型错误语义；
- 通用流媒体框架、WebSocket、HTTP Upgrade；
- 为流式读取另建一套 DNS、Transport 或 SSRF 实现。
