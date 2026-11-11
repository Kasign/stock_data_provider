# PLAT-CTR-003 身份、授权与 Session 原语契约

- 契约编号：PLAT-CTR-003
- 契约版本：1.0.0
- 状态：freeze-ready（待项目负责人确认冻结）
- 代码审计基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`
- 生产方：Platform Security 原语；Server 负责 HTTP 入口、Session Middleware 和路由装配
- 消费者：Root Server、服务端领域 Application、WebUI（仅 Cookie/CSRF 浏览器协议）
- 权威性：本文件定义身份来源、Principal、Bearer/Session 原语、Service Scope、Secret 和安全审计边界；Server 登录 HTTP API/路由矩阵由 SRV-CTR-001 定义，领域资源授权由 GOV-CTR-006 及各领域 CTR 定义。

## 1. 身份模型与凭证隔离

首期只支持以下四种认证来源：

| AuthMethod | Principal.Kind | Subject | Scopes | 凭证与用途 |
|---|---|---|---|---|
| anonymous | anonymous | anonymous | 空 | 显式公开的少数入口 |
| owner_bearer | owner | owner | 仅 `platform:admin` | 可信运维调用，使用独立 `PLATFORM_ADMIN_TOKEN` |
| owner_session | owner | owner | 空 | WebUI 单一 Owner 登录态，通过 HttpOnly Cookie |
| service_bearer | service | 稳定 Service Name | 一个或多个固定 Service Scope | 服务间调用，每个调用方独立 Secret |

硬性边界：

- Owner Bearer、Owner Session、Service Bearer、Data Provider/API Key 是不同凭证，不得互换。
- Owner Bearer 不下发浏览器；WebUI 只持有 HttpOnly Session Cookie 和运行时内存中的 CSRF Token。
- Service Secret 只由 Server/受控服务端 Adapter 持有；Provider Token 不得作为 Service Secret。
- Owner Session 证明单一 Owner 已登录，但不继承 `platform:admin` 或任何 Service Scope。它可按 Server 路由矩阵访问显式允许 owner_session 的应用 API；领域资源读取/写入仍由资源 Owner 按 GOV-CTR-006 复核。
- 不建设注册、多用户、组织/租户、OAuth/OIDC、动态 Service Key CRUD 或独立认证微服务。

## 2. Principal 与可信 Context

跨模块进程内身份使用唯一强类型：

```go
type IdentityKind string
type AuthMethod string

type Principal struct {
    Kind       IdentityKind
    AuthMethod AuthMethod
    Subject    string
    Scopes     []string
}

func WithPrincipal(ctx context.Context, principal Principal) (context.Context, error)
func PrincipalFromContext(request *http.Request) (Principal, bool)
```

固定枚举：

- IdentityKind：`anonymous`、`owner`、`service`。
- AuthMethod：`anonymous`、`owner_bearer`、`owner_session`、`service_bearer`。

Principal 组合约束：

- anonymous：Kind=anonymous、AuthMethod=anonymous、Subject=anonymous、Scopes 为空。
- owner_bearer：Kind=owner、AuthMethod=owner_bearer、Subject=owner、Scopes 仅包含 `platform:admin`。
- owner_session：Kind=owner、AuthMethod=owner_session、Subject=owner、Scopes 为空。
- service_bearer：Kind=service、AuthMethod=service_bearer、Subject 为合法 Service Name、Scopes 非空且全部属于固定 allowlist。

`WithPrincipal` 只供完成凭证/Session 验证的可信 Server Authentication Middleware 调用；必须校验上述组合并复制 Scopes，不能接受客户端传入的身份字段。领域模块不得自行构造或覆盖 Principal。读取时返回 Scope 切片副本。Principal 是进程内身份快照，不序列化到公开 DTO，不持久化原始凭证。

`AuthMethod` 是身份来源，不等同 Kind 或 Scope。所有授权决策必须同时保留身份类别与认证来源，不能仅凭 Kind=owner 推断 owner_bearer 与 owner_session 等价。

## 3. Bearer Authenticator 与 RoutePolicy

Platform Bearer API：

```go
type ServiceCredential struct {
    Name   string
    Secret config.Secret
    Scopes []string
}

func NewAuthenticator(ownerToken config.Secret, services []ServiceCredential) (*Authenticator, error)
func (a *Authenticator) Authenticate(request *http.Request) (Principal, error)
func NewRouter() *Router
func (r *Router) Handle(pattern string, authenticator *Authenticator, policy RoutePolicy, handler http.Handler) error
func (r *Router) HandleFunc(pattern string, authenticator *Authenticator, policy RoutePolicy, handler http.HandlerFunc) error
func (r *Router) Handler() http.Handler
```

凭证约束：

- Owner/Service Token 长度为 32～256 字节，只允许 URL-safe ASCII 字母、数字、`-`、`_`；部署生成必须使用至少 256 bit 密码学随机熵。
- Authenticator 最多接受 128 个 ServiceCredential。
- Service Name 先 TrimSpace，再匹配 `^[a-z0-9][a-z0-9._-]{0,63}$`；同一 Authenticator 内唯一。
- 每个 Service 必须配置独立 Secret 和至少一个 Scope；Scope 不得重复、不得未知，Service 不得获得 `platform:admin`。
- Owner 与 Service 凭证摘要必须唯一；Authenticator 保存 SHA-256 摘要，认证时对所有已配置摘要执行常量时间比较。
- Authenticator 是不可变配置快照；凭证轮换通过更新部署 Secret 并重建 Authenticator 完成，不提供运行时动态 CRUD 或 Redis 凭证缓存。
- 只读取单一 `Authorization: Bearer`；缺失、重复、错误 Scheme、非法 Token 或无效凭证均不得建立认证身份。Bearer Authenticator 不读取 Cookie，不接受 Session Token。
- 缺少/无效凭证为 401；已认证但 RoutePolicy 不允许为 403；未注册路由保持 404。
- 每条路由必须显式注册策略；不允许根 catch-all 绕过策略。

固定 Scope allowlist：

- `platform:admin`：仅 Owner Bearer。
- `data:read`、`data:diagnose`、`research:run`、`research:read`：可分配给 Service。
- 新增 Scope 必须先更新本契约并评审消费者，不得在实现中静默扩展。

现有 `AnonymousAccess`、`OwnerAccess`、`ServiceAccess(scopes...)`、`OwnerOrServiceAccess(scopes...)` 均属于 Bearer Router 策略；其中 `OwnerAccess` / `OwnerOrServiceAccess` 的 Owner 分支只代表 owner_bearer，不代表 owner_session。

Owner Session 由 Server 独立 Middleware 验证。Server 的每条路由必须显式声明 anonymous、owner_bearer、owner_session 或 service_bearer；不得把 Session 伪造成 Authorization Header 复用 Bearer Router。owner_session 不得自动继承 owner_bearer 或 Service Scope 权限。

## 4. Owner Password 与 CredentialStore

密码原语：

```go
type Argon2idParams struct {
    Memory      uint32 // KiB
    Iterations  uint32
    Parallelism uint8
    SaltLength  uint32
    KeyLength   uint32
}

func NewArgon2idHasher(params Argon2idParams) (*Argon2idHasher, error)
func (h *Argon2idHasher) Hash(password []byte) (string, error)
func (h *Argon2idHasher) Verify(encoded string, password []byte) (bool, error)
func (h *Argon2idHasher) NeedsRehash(encoded string) (bool, error)
```

- 使用 Argon2id PHC 格式，版本 19；不自研密码学算法。
- 不设置未经目标部署硬件基准的隐式生产默认参数；Server 必须显式提供参数。
- 支持边界：Memory 至少为 8×Parallelism KiB、最多 256 MiB；Iterations 1～10；Parallelism 1～8；Salt 16～64 字节；Key 16～64 字节；密码输入 1～1024 字节；编码哈希最多 512 字节。
- 密码明文、哈希不得进入日志、Fault、诊断、Audit Event 或公开 DTO。

CredentialStore：

```go
type OwnerCredential struct {
    Subject           string
    PasswordHash      string
    CredentialVersion uint64
}

type CredentialStore interface {
    GetOwnerCredential(ctx context.Context) (OwnerCredential, error)
    InitializeOwnerCredential(ctx context.Context, passwordHash string) (OwnerCredential, error)
    UpdateOwnerPassword(ctx context.Context, expectedVersion uint64, passwordHash string) (OwnerCredential, error)
}
```

- 首期固定单一 Owner，Subject 为 `owner`；MariaDB 是权威事实源。
- 初始化只允许创建一次，初始 CredentialVersion=1；重复初始化返回 conflict。
- Update 使用 expectedVersion 比较并交换；成功时在同一 SQL 更新中替换哈希并将版本加一；版本不匹配返回 conflict。
- 每次 Session 认证必须读取/核对 MariaDB 当前 CredentialVersion；版本不一致即拒绝 Session。密码修改/受控恢复递增版本，使旧 Session 失效。
- Platform 提供 `NewSQLCredentialStore(db)` 和 `OwnerCredentialMigrations()`；Server Migration Registry 必须显式注册 Migration，连接数据库不自动迁移。

## 5. Session Token 与 SessionStore

```go
type SessionRecord struct {
    Subject           string
    CreatedAt         time.Time
    LastSeenAt        time.Time
    IdleExpiresAt     time.Time
    AbsoluteExpiresAt time.Time
    CredentialVersion uint64
    CSRFTokenDigest   string
}

type SessionStore interface {
    Create(ctx context.Context, tokenDigest string, record SessionRecord) error
    LookupAndTouch(ctx context.Context, tokenDigest string, now time.Time, idleTTL time.Duration) (SessionRecord, error)
    Rotate(ctx context.Context, oldTokenDigest, newTokenDigest string, record SessionRecord, now time.Time) error
    Revoke(ctx context.Context, tokenDigest string) error
}
```

- Token 使用 CSPRNG 生成 32 字节随机值，以无填充 URL-safe Base64 交给浏览器；原始 Token 只在创建时返回一次。
- 服务端仅使用原始 Token 的 SHA-256 小写十六进制摘要作为 SessionStore key；SessionRecord 不包含原始 Token。
- 空闲期限默认/最大 12 小时；绝对期限默认/最大 7 天。Touch 可滑动 idle expiry，但不得延长 absolute expiry。
- SessionRecord 必须固定 Subject=owner、CredentialVersion 非零；CreatedAt、LastSeenAt、IdleExpiresAt、AbsoluteExpiresAt 必须满足时间顺序和 TTL 上限；CSRFTokenDigest 可选但若存在必须是 SHA-256 小写十六进制摘要。
- Create、LookupAndTouch、Rotate、Revoke 对同一 Session 必须具备原子语义。Rotate 必须原子创建新摘要并撤销旧摘要，同时保留原 Subject、CreatedAt、CredentialVersion 和 AbsoluteExpiresAt。
- SessionStore 只接受 Token Digest；Redis 实现复用 Server 注入的现有 go-redis Client，不创建或关闭第二套连接池。
- Redis 读写/Touch/Rotate/Revoke 失败必须 fail closed；不回退内存 Session、Bearer 或匿名身份。Redis 数据丢失时所有 Session 视为失效，用户重新登录。
- Redis 内部记录与 Key 格式是 Platform 私有实现，不构成公开 JSON DTO；Redis Session 数据允许在版本升级时整体失效，不承诺跨版本持久兼容。

## 6. Cookie 与 CSRF

Cookie：

```go
type SessionCookieConfig struct {
    Name     string
    Path     string
    Domain   string
    Secure   bool
    SameSite http.SameSite
}

func (c SessionCookieConfig) Validate() error
func NewSessionCookie(config SessionCookieConfig, rawToken string, expiresAt, now time.Time) (*http.Cookie, error)
func ClearSessionCookie(config SessionCookieConfig) (*http.Cookie, error)
```

- Name 只允许 ASCII 字母、数字、`-`、`_`；Path 必须为非根路径，不能含 `..`、分号或换行；Domain 必须为空；SameSite 固定 Lax。
- Cookie 固定 HttpOnly=true；HTTPS 公网部署必须 Secure=true；不得设置宽泛 Domain。
- Cookie Expires/MaxAge 必须不超过 Session 剩余有效期；退出/撤销时使用相同 Name/Path/Domain/Secure/SameSite 属性清除。
- 公开 Cookie Name、Path、Secure 和可信代理网络由 Server/部署配置，不由 Platform 从环境变量自行读取。

CSRF：

```go
func NewCSRFToken() (rawToken, tokenDigest string, err error)
func CSRFTokenDigest(rawToken string) (string, error)
func VerifyCSRFToken(rawToken, expectedDigest string) bool
```

- CSRF Token 使用 32 字节 CSPRNG，输出无填充 URL-safe Base64；Session 只存 SHA-256 摘要。
- Verify 使用常量时间比较，并要求 Token 与当前 Session 绑定。
- Server 负责状态变更方法分类、Origin/Host 校验、Cookie 路由 CSRF 接线和可信代理识别；GET/HEAD/OPTIONS 不得产生业务状态变更。
- WebUI 通过受保护的同源响应获取 CSRF Token，只存运行时内存并通过专用 Header 回传，不写 localStorage。
- SameSite=Lax 不能替代 CSRF Token 与 Origin/Host 校验。

## 7. Login RateLimit

```go
type LoginRateLimitPolicy struct {
    AccountMaxFailures int64
    IPMaxFailures      int64
    Window             time.Duration
    BaseBackoff        time.Duration
    MaxBackoff         time.Duration
}

type LoginRateLimitResult struct {
    Allowed         bool
    AccountFailures int64
    IPFailures      int64
    RetryAfter      time.Duration
}

type LoginRateLimiter interface {
    Check(ctx context.Context, accountDigest, sourceIPDigest string) (LoginRateLimitResult, error)
    RecordFailure(ctx context.Context, accountDigest, sourceIPDigest string, policy LoginRateLimitPolicy) (LoginRateLimitResult, error)
    ResetAccount(ctx context.Context, accountDigest string) error
}
```

- Account 与 Source IP 两维度必须同时参与；接口只接收 SHA-256 小写十六进制摘要，不接收原始账号/IP。
- Server 负责规范化账号和 IP 并生成摘要；同一身份/来源必须使用稳定规范化规则。
- 计数使用固定时间窗口；超过阈值后按指数退避，退避上限由 MaxBackoff 限制。策略不设置隐式业务阈值，由 Server 配置。
- Policy 边界：两维 MaxFailures 均为 1～100000；Window、BaseBackoff 为 1ms～24h；MaxBackoff 不小于 BaseBackoff 且不超过 24h。
- ResetAccount 只清理账号维度失败计数/阻断，不清理来源 IP 维度风险计数。
- Redis 原子操作失败时登录必须 fail closed；不得在限流不可用时无保护放行。

## 8. Security Audit

```go
type SecurityAuditEvent struct {
    EventType  SecurityAuditEventType
    Subject    string
    Result     SecurityAuditResult
    OccurredAt time.Time
    RequestID  string
    Source     SecurityAuditSource
    ReasonCode string
}

type SecurityAuditWriter interface {
    Write(ctx context.Context, event SecurityAuditEvent) error
}

func WriteSecurityAudit(ctx context.Context, writer SecurityAuditWriter, event SecurityAuditEvent) error
```

- EventType closed enum：login.succeeded、login.failed、login.rate_limited、logout、session.created、session.rotated、session.revoked、password.changed、password.recovered、security.sensitive_action。
- Result closed enum：success、failure、blocked。
- Source closed enum：anonymous、owner_bearer、owner_session、service、internal。
- Subject 仅允许 owner、anonymous、system；RequestID 遵循 PLAT-CTR-002；ReasonCode 为 1～128 字节小写稳定标识。
- 事件只包含受限元数据；禁止密码、密码哈希、Session/CSRF Token、Authorization Header、Cookie、完整请求体和原始 IP。
- Platform 校验事件并调用注入的 Writer；Platform 不选择数据库或存储后端。Writer 失败返回稳定 Fault；Server 必须显式决定关键安全事件写入失败时的业务处理策略，不得静默吞掉。

## 9. Server 路由与职责边界

- Platform Bearer Router 只验证 Bearer，不解析 Cookie；现有 `OwnerAccess` 代表 owner_bearer，不能解释为 owner_session。
- Server Session Middleware 负责读取 Cookie、校验 Session Token、读取 SessionStore、核对当前 CredentialVersion、执行 CSRF/Origin/Host 校验，并以 `AuthMethod=owner_session` 建立 Principal Context。
- Server 每条路由显式声明允许的 AuthMethod；owner_session 不自动继承 owner_bearer/service_bearer 权限。
- Server 负责登录、退出、密码修改、受控恢复、Cookie 发放/清除、Session 轮换和 Audit 事件产生。
- Platform 只提供密码、CredentialStore、SessionStore、Cookie、CSRF、RateLimiter、Audit 等技术原语，不提供登录 Handler、用户中心或资源授权规则。
- Data/News/Quant/Research/Researchflow 等领域仍是各自资源 Owner；Principal 只提供可信调用主体，不替代 AuthorizationRef 签发和读取时复核。

## 10. Schema、Golden 与验收适用性

本契约主要由进程内 Go 类型/接口、Authorization Header、HttpOnly Cookie 和 CSRF Header 组成，不存在独立的公开 JSON 身份 DTO；因此不为 Principal、SessionRecord、OwnerCredential 或 SecurityAuditEvent 创建公开 JSON Schema/HTTP Golden。

等价机器定义为 Platform Go API 与 Contract Test。SessionRecord 的 Redis JSON 是短期内部存储格式，可在升级时整体失效；OwnerCredential 由 Platform Migration 管理；SecurityAuditEvent 是注入式内部事件，不是浏览器或领域模块的公共 HTTP DTO。

Server 登录 HTTP API 的路径、请求/响应、错误、路由认证矩阵、Origin/Host 和 CSRF 行为由 SRV-CTR-001 另行定义；WebUI 登录交互和部署 HTTPS/可信代理属于后续实现符合性，不阻断 PLAT-CTR-003 定义冻结。

## 11. 版本与兼容

本契约首版为 1.0.0。身份类别/AuthMethod、Principal 字段、凭证格式、Service Scope、路由身份隔离、Session 生命周期、CredentialVersion 撤销、CSRF、限流或审计事件语义发生不兼容变化时，按 GOV-CTR-004 升级版本。新增 Service Scope 或 Audit EventType 不得只改代码，必须更新本契约并评估消费者。
