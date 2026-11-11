# PLAT-CTR-005 HTTP Client、SSRF 与出口安全契约

- 契约编号：`PLAT-CTR-005`
- 契约版本：`1.0.0`
- 状态：`freeze-ready`
- 生产方：Platform
- 直接消费者：Data、News、LLM；其他服务端模块仅在确有外部 HTTP 请求用例时接入
- 唯一权威 API：`internal/platform/httpclient`
- 设计依据：`doc/platform/05-HTTP存储与异步任务设计.md`、`doc/platform/08-公共平台契约规范.md` 第8节及其补充章节、`doc/platform/15-受控HTTP流式传输设计与验收方案.md`
- 代码核对基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`

## 1. 目标与边界

Platform 提供受控的出站 HTTP 传输原语，统一连接复用、超时、响应体/响应头限额、有限并发、目标地址校验、重定向约束、Request ID 传播和传输 Fault。

Platform 不负责：
- Provider 配额、账号级限流、业务成本或来源内容授权；这些由 Data/News 等领域负责。
- 业务 HTTP DTO、业务响应解释、SSE/JSON/模型事件解析、工具调用或业务终态。
- 应用层通用重试编排、熔断平台、自适应限流、分布式 Retry Budget。
- 自动推断可信目标。每次请求必须携带显式 DestinationPolicy，或使用已绑定固定策略的 BoundDoer。

HTTP Client 的目标安全校验与领域授权是两层独立门禁：领域先确定某个 endpoint/source 是否允许访问；Platform 再确认该网络目标和实际连接地址安全。任何一层都不能替代另一层。

## 2. 公共 Go API

以下为契约级公开形态；实现可以增加非破坏性内部细节，不得改变这些调用语义。

```go
type Config struct {
    Timeout                time.Duration
    DialTimeout            time.Duration
    TLSHandshakeTimeout    time.Duration
    ResponseHeaderTimeout  time.Duration
    MaxResponseHeaderBytes int64
    MaxResponseBytes       int64
    MaxRetries             int
    MaxConcurrent          int
    DependencyLimits       map[string]int

    StreamTimeout          time.Duration
    StreamIdleTimeout      time.Duration
    MaxStreamBytes         int64
    MaxConcurrentStreams   int
    StreamDependencyLimits map[string]int

    Resolver Resolver
}

type DestinationPolicy struct {
    Dependency     string
    AllowedHosts   []string
    AllowedPorts   []uint16
    AllowHTTP      bool
    AllowRedirects bool
    MaxRedirects   int
    Idempotent     bool
}

type Resolver interface {
    LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

func New(config Config) (*Client, error)
func (c *Client) Do(ctx context.Context, request *http.Request, policy DestinationPolicy) (*http.Response, error)
func (c *Client) Bind(policy DestinationPolicy) (*BoundDoer, error)
func (c *Client) OpenStream(ctx context.Context, request *http.Request, policy DestinationPolicy) (*StreamResponse, error)
func (c *Client) CloseIdleConnections()

func (d *BoundDoer) Do(request *http.Request) (*http.Response, error)
func (d *BoundDoer) OpenStream(ctx context.Context, request *http.Request) (*StreamResponse, error)

type StreamResponse struct {
    StatusCode int
    Header     http.Header
    Body       io.ReadCloser
}

func (r *StreamResponse) Stats() StreamStats

type StreamStats struct {
    RequestID         string
    Dependency        string
    StatusCode        int
    StartedAt         time.Time
    Duration          time.Duration
    BytesRead         int64
    TerminationReason string
}
```

`Resolver` 是 DNS 解析依赖注入点；生产默认使用系统 Resolver。注入自定义 Resolver 不得绕过完整 DNS 结果校验或受控 Dial。结构体中的字段均为配置/API 字段，不意味着消费者可以自行创建或替换底层 Transport。

### 2.1 调用入口

- `Client.Do(ctx, request, policy)`：缓冲式请求；ctx 是请求的权威 Context；返回前读取完整响应体并执行响应体上限检查，返回的 Body 仍由调用方 Close。
- `Client.OpenStream(ctx, request, policy)`：流式请求；成功收到响应头后立即返回，不等待 Body EOF。
- `Client.Bind(policy)`：验证并复制策略，生成一个固定目标的 BoundDoer。一个 BoundDoer 对应一个逻辑 Dependency 和固定 Endpoint 策略。
- `BoundDoer.Do(request)`：使用 request.Context() 和创建时绑定的策略。
- `BoundDoer.OpenStream(ctx, request)`：使用显式 ctx 和创建时绑定的策略。
- `CloseIdleConnections()`：关闭空闲连接；Client 的创建和最终关闭由组合根/资源所有者管理，领域消费者不得各自创建重复 Transport 来绕过共享策略。

请求对象不得通过自定义 Host 或 Host Header 覆盖目标主机；不得自行设置 Accept-Encoding。请求必须使用标准 URL/HTTP 请求语义。

## 3. DestinationPolicy 与目标约束

### 3.1 策略字段

| 字段 | 约束与语义 |
|---|---|
| `Dependency` | 必填稳定逻辑标识；仅允许小写 ASCII 字母、数字、点、下划线、连字符；最多 128 字节；不得使用 IP 地址 |
| `AllowedHosts` | 必填精确 Host allowlist，1～64 项；不支持通配符；Host 比较忽略大小写并移除尾随点 |
| `AllowedPorts` | 可选精确端口 allowlist，最多 16 项；每项 1～65535；非空时只允许列表中的端口 |
| `AllowHTTP` | 默认 false；只有显式 true 才允许 HTTP。未提供 AllowedPorts 时 HTTPS 默认 443，HTTP 默认 80 |
| `AllowRedirects` | 默认 false；默认拒绝所有重定向 |
| `MaxRedirects` | 0～3；仅在 AllowRedirects=true 时可设置正值；允许重定向但未显式设置时有效默认最多 3 跳 |
| `Idempotent` | 默认 false；只有调用方能证明操作幂等时才可设 true，作为缓冲重试和流式 Open Fault Retryable 的必要条件之一 |

策略在 Client.Do/OpenStream 每次请求前校验；Bind 在创建时校验并复制 Host/Port 切片，之后调用方修改原切片不得改变绑定策略。

### 3.2 URL 与 Host 校验

- 只允许 HTTPS；HTTP 必须由 AllowHTTP 显式开启。
- 拒绝 opaque URL、userinfo、空 Host、非法 Host、IPv6 zone、非 allowlist Host 和非允许端口。
- 拒绝自定义 Host 覆盖和自定义 Accept-Encoding。
- IP literal 与 DNS 名称执行同一目标地址安全策略。
- 不信任 X-Forwarded-*、客户端身份 Header 或 URL 文本之外的声明作为目标授权依据。

### 3.3 DNS、IP 与实际连接绑定

- DNS 查询必须检查完整 A/AAAA 结果集合；只要任一结果不合规，整个目标拒绝，不允许只挑选其中公网 IP 继续。
- 拒绝非全局可路由地址及 loopback、private、link-local、unspecified、multicast、IPv4-mapped IPv6 和保留/特殊用途地址段。
- DNS 校验通过后，受控 Dial 必须直接连接已校验的 IP literal；TLS ServerName 与 HTTP Host 仍保留原始 hostname。
- 每次新建连接重新解析并校验；不得采用“预先校验一次 DNS，之后让默认 Dialer 再解析一次”的模式。
- DNS、连接和 TLS 过程均受请求 Context/Deadline 与 Dial/TLS 超时控制。
- 禁用环境代理和隐式 ProxyFromEnvironment；不得以自定义 Transport、裸 http.Client 或代理设置绕过策略。

### 3.4 重定向

- 默认拒绝自动重定向。
- 显式开启后，每一跳重新校验 scheme、userinfo、Host、Port、来源 allowlist 和 DNS/IP 目标。
- 仅允许同源重定向：scheme、规范化 hostname、有效 port 均一致；禁止 HTTPS 降级到 HTTP。
- 最多 3 跳；拒绝循环、超限及任何跨源跳转，即使新 Host 也在 allowlist 中。
- 不允许只在 CheckRedirect 校验后再由默认 Dialer 独立解析目标。
- 未来若确需跨源 CDN 跳转，必须另行定义凭据剥离/重建与目标授权策略，不得在本契约下放宽为跨 Host。

## 4. 缓冲式请求语义

### 4.1 默认值与硬上限

`New(Config)` 对非正数配置使用默认值；超过硬上限的配置必须返回错误。MaxRetries=0 表示关闭应用层重试，不自动提升为默认重试次数。

| Config 字段 | 默认值 | 硬上限/范围 |
|---|---:|---:|
| Timeout | 15 秒 | 必须为正值 |
| DialTimeout | 5 秒 | 必须为正值 |
| TLSHandshakeTimeout | 5 秒 | 必须为正值 |
| ResponseHeaderTimeout | 8 秒 | 必须为正值 |
| MaxResponseHeaderBytes | 1 MiB | 4 MiB |
| MaxResponseBytes | 2 MiB | 16 MiB |
| MaxRetries | 0 | 0～2 |
| MaxConcurrent | 4 | 1～32 |
| DependencyLimits | 空 | 最多 64 个 Dependency；每项 1～32 |
| StreamTimeout | 10 分钟 | 30 分钟 |
| StreamIdleTimeout | 30 秒 | 5 分钟 |
| MaxStreamBytes | 16 MiB | 64 MiB |
| MaxConcurrentStreams | 2 | 1～8 |
| StreamDependencyLimits | 空 | 最多 64 个 Dependency；每项 1～全局流上限 |

流式配置虽与缓冲式 API 一并由 Config 承载，但两类请求使用独立并发池。Dependency 未单独配置时共享有界默认池，不得根据请求输入动态创建 Semaphore。

### 4.2 响应体与状态码

- 缓冲请求在返回前完整读取响应体；按实际读取字节执行 MaxResponseBytes 限制，不依赖 Content-Length。
- 超限时关闭底层 Body 并返回 Fault，不向调用方交付部分成功响应。
- 返回成功响应时 Body 是已缓冲内容的可关闭 Reader；调用方仍负责 Close。
- 4xx/5xx 是正常 HTTP 响应，不由 Platform 自动转成 Fault；响应协议/业务含义由消费者处理。

### 4.3 重试

- 默认不重试；只有 DestinationPolicy.Idempotent=true、MaxRetries>0 且请求体为空或提供可重建请求体 GetBody 时，才允许应用层重试。
- 最多重试 MaxRetries 次（总尝试次数最多 1+MaxRetries）。
- 可重试 HTTP 状态仅为 502、503、504；可重试传输错误限于可识别暂态网络错误。策略拒绝、DNS 解析失败、Context 取消/Deadline、响应体超限不得重试。
- 退避依次为 100ms、200ms；等待必须受总请求 Context 控制，退避期间不占用 Dependency/全局并发槽位。
- 每次重试必须重建请求体；GetBody 失败时停止并返回错误。
- Request ID 在每次尝试中传播；Platform 不改变业务幂等键，也不保证上游业务副作用的幂等性。
- Go net/http Transport 可能在响应头交付前按标准库规则执行有限的底层连接恢复重试；这不等于 Platform 应用层重试。消费者不得把二者混为一谈。

## 5. 流式请求语义

### 5.1 入口与响应交付

- 使用独立 `OpenStream`，复用同一 Client/Transport/DestinationPolicy/受控 Dial；不得另建流式 Client 或 SSRF 实现。
- 成功后立即交付 StatusCode、独立 Header 快照和受控 io.ReadCloser；不得预读到 EOF 或整体缓冲。
- HTTP 状态码本身不构成传输 Fault；消费者读取状态码并按自身协议处理。
- Platform 不解析 SSE、JSON、WebSocket、Responses API 或模型协议。

### 5.2 Body 所有权和终止

- 成功返回后 Body 所有权转移给调用方；调用方必须在 EOF、错误或提前退出时 Close，建议立即 defer Close。
- EOF、读取错误、父 Context 取消/Deadline、流总时限、空闲读取超时、Body 超限和主动 Close 均须幂等清理底层 Body、派生 Context 和并发额度。
- Close 幂等；主动 Close 不作为上游故障。
- Header 是独立快照，调用方修改 Header 不得影响底层请求/传输策略。
- 流式并发额度从请求执行前持有到流终止；Dependency 子额度先于全局额度获取；等待子额度不得占用全局槽位。
- 流式并发池与缓冲请求池隔离。

### 5.3 流式资源上限

| 配置 | 默认值 | 硬上限 | 语义 |
|---|---:|---:|---|
| MaxResponseHeaderBytes | 1 MiB | 4 MiB | 共享 Transport 的响应头上限 |
| MaxStreamBytes | 16 MiB | 64 MiB | 按实际交付的解压后字节累计 |
| StreamIdleTimeout | 30 秒 | 5 分钟 | 单次阻塞 Read 等待上游数据的上限；调用方两次 Read 间的处理时间不计入 |
| StreamTimeout | 10 分钟 | 30 分钟 | 整条流生命周期总上限；父 Context 更短时以父 Context 为准 |
| MaxConcurrentStreams | 2 | 8 | 独立全局流并发上限 |

- 不依赖 Content-Length；每次 Read 累计实际解压后字节。
- 恰好达到上限时，下一次读取探测是否还有额外字节；若有则不交付探测字节并以超限 Fault 终止。
- 直接包装底层 Body 形成背压；不建立无界 Channel 或后台常驻读取泵。
- 空闲超时只作用于单次阻塞 Read，不惩罚调用方处理已读数据所花费的时间。

### 5.4 流式重试与 Retryable

- Platform 不执行流式应用层重试。
- 响应头交付前的 Open Fault 只有在策略显式幂等、请求体为空/NoBody/可由 GetBody 重建，且错误属于可识别暂态时，才可标记 Retryable=true；该标记只表示消费者可评估是否重新发起，不授权 Platform 自动重放。
- 一旦响应头已交付，所有 Body 读取终止错误均不得标记 Retryable=true；不得拼接、切换或透明重放另一条响应。
- 调用方主动 Close 不产生超时/失败 Fault。

### 5.5 StreamStats

`Stats()` 返回只读快照，至少包含 RequestID、Dependency、StatusCode、StartedAt、Duration、BytesRead、TerminationReason。流终止后 Duration 和终止原因固定。统计不得包含 URL/Query、凭据、请求/响应 Body 或原始上游错误正文。

## 6. Fault 与安全诊断

- Fault 使用 Platform Fault 类型；Dependency 必须是稳定逻辑标识。
- SafeMessage 只能是固定、脱敏摘要；Cause 只保留在进程内错误链。
- 缓冲请求至少区分：invalid_request、invalid_dependency、custom_host_rejected、accept_encoding_rejected、destination_rejected、dns_failed、response_too_large、timeout、canceled、transport_timeout、request_failed。
- 流式请求至少区分：stream_destination_rejected、stream_dns_failed、stream_headers_too_large、stream_connect_failed、stream_connect_timeout、stream_tls_handshake_timeout、stream_header_timeout、stream_idle_timeout、stream_canceled、stream_deadline_exceeded、stream_body_too_large、stream_upstream_truncated、stream_read_timeout、stream_read_failed。
- 不得把 URL/Query、Authorization、Cookie、API Key、请求/响应 Body 或原始上游错误正文放入 Code、SafeMessage、Dependency 或公开诊断字段。
- 流式终态统计只记录 Request ID、Dependency、状态码、耗时、读取字节数和终止原因等安全元数据。

## 7. 生命周期与所有权

- Server 组合根负责创建共享 Client、为固定 Endpoint 创建 BoundDoer，并向领域 Adapter 注入窄接口。
- 领域模块只消费注入的 Doer/BoundDoer，不自行创建裸 http.Client、Transport 或自定义 Dialer 绕过平台策略。
- Client/Transport 的 CloseIdleConnections 由资源所有者在服务关闭时调用；消费者不得擅自关闭共享 Client。
- 领域负责 endpoint/source 授权、Provider 配额和业务错误；Platform 负责底层网络目标校验、传输资源上限和传输 Fault。

## 8. 版本与兼容

- 契约版本：1.0.0。
- 本契约为纯 InProcess Go API；JSON Schema 不适用，强类型 API 是唯一机器可核对定义。
- 新增可选配置或向后兼容能力可按 GOV-CTR-004 递增 MINOR；改变安全默认值、allowlist 语义、重试条件、超时/限额语义、Fault 分类或 Body 所有权属于破坏性变更，必须递增 MAJOR 并先完成项目级变更审批。
- Platform 不得静默放宽 SSRF、代理、跨源重定向或凭据转发限制。
