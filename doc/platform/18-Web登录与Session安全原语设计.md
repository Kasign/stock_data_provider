# Web 登录与 Session 安全原语设计

- 所属仓库：Kasign/stock_data_provider
- 所属分支：feat/platform-foundation
- 适用范围：Platform 公共安全原语候选设计
- 状态：安全模型与跨层职责为候选设计；Argon2id 哈希原语已进入实现/CI 验收，CredentialStore 及 Web Session 相关契约仍待消费者确认
- 关联文档：《03-安全认证与ServiceKey设计》《08-公共平台契约规范》《01-开发路线图与任务清单》《16-公共平台剩余开发任务与跨模块接入责任清单》

## 1. 设计结论

采用单一 Owner + Web Session Cookie 的浏览器认证模型。Platform 提供密码、Session、CSRF、限流和安全审计技术原语；Server 负责初始化、登录/退出/密码管理业务、认证中间件、路由策略和审计事件产生；Web 负责交互。Nginx 仅反向代理，不是认证边界。

现有 Owner Bearer（`PLATFORM_ADMIN_TOKEN`）和 Service Bearer 保留，分别用于可信运维和服务间调用。Web Session 是独立认证方式，不能伪装成 Bearer，也不能默认继承所有 Owner Bearer 路由权限。

首期不建设注册、多用户、组织/租户、OAuth/OIDC、MFA 管理平台、动态 Service Key、独立认证微服务或全量业务审计平台。

## 2. 身份和路由认证方式

| 身份/认证方式 | 凭证 | 主要用途 | 约束 |
|---|---|---|---|
| anonymous | 无 | 显式公开的登录、liveness 等少数路由 | 白名单；登录仍限流并校验 Origin/Host |
| owner_bearer | PLATFORM_ADMIN_TOKEN | 可信运维调用 | 不下发浏览器 |
| service | 调用方独立 Bearer Secret | 服务间调用 | 固定 Scope；不与 Provider Token 混用 |
| owner_session | HttpOnly Cookie | 浏览器 Owner 登录 | 独立校验链路；路由显式声明接受 |

Server 统一入口必须为每条路由声明允许的认证方式和 Scope。Web Session 成功认证后可以映射为可信 Owner Principal，但 Principal 需保留认证来源；不能把 Cookie Session 当作 Owner Bearer。直接访问 Server 端口和经 Nginx 访问必须执行同样的应用认证。

## 3. 密码与 Owner Credential

### 3.1 算法和参数

使用成熟 Argon2id 实现，不自研密码学算法。候选默认参数：
- memory：64 MiB
- iterations：3
- parallelism：1
- salt：16 bytes
- output：32 bytes

参数需在目标部署硬件基准测试，确认登录耗时、并发内存峰值和拒绝服务风险后定稿。哈希编码保存算法与参数版本；支持验证旧版本，并在成功登录后按新参数重新哈希。

密码输入设定合理最大长度和请求体上限，避免超大输入消耗资源；不以固定字符拼盘规则替代长度与泄露密码检查。密码、哈希和验证输入不得进入日志、Fault、诊断或审计。

### 3.2 Credential 持久化

MariaDB 是 Owner Credential 的权威事实源，至少保存固定 owner subject、Argon2id encoded hash、credential_version、created_at/updated_at。明文密码不落库。Credential 哈希与 credential_version 更新必须在同一事务。Platform 负责 CredentialStore 契约和平台表/迁移边界；Server 负责调用和业务流程。Server 通过 Migration Registry 显式注册 Platform 自有 Migration，OpenSQL 不自动执行迁移。

### 3.3 初始化、修改和恢复

- 首次启动时，Server 通过一次性部署 Secret 提供初始密码；仅当尚无 Owner Credential 时生效。
- 初始化成功后清理初始 Secret；后续重启不能重置密码，也不能把环境变量作为长期密码存储。
- 密码修改需要当前有效 Session/CSRF 与必要的当前密码验证；成功后原子更新哈希和 credential_version。
- 账号恢复只允许受控本机/运维流程，不提供匿名 HTTP 重置接口；恢复成功同样递增 credential_version。
- 首期低并发下，每次 Session 认证读取 MariaDB 权威版本，优先保证撤销正确性，避免缓存失效竞态。

## 4. Session 与 Redis

### 4.1 Token 与记录
Session Token 由 CSPRNG 生成，至少 256 bit，使用 URL-safe Base64 无填充编码。浏览器拿到原始 Token；服务端只保存 SHA-256 摘要。不得将原始 Token 写入日志、错误、审计或数据库。

Session 记录至少包括 token_digest、owner subject、created_at、last_seen_at、idle_expires_at、absolute_expires_at、credential_version、csrf_token_digest 和必要轮换元数据。

### 4.2 生命周期
- 空闲期限：12 小时；有效请求可滑动续期。
- 绝对期限：7 天；任何请求不得延长绝对期限。
- 登录成功创建新 Session，不升级匿名 Cookie 或沿用未认证 Session。
- 轮换创建新 Token 并使旧 Token 失效；并发轮换必须只有一个结果有效。
- 退出撤销当前 Session。
- 密码修改/恢复递增 credential_version；认证时版本不一致即拒绝旧 Session，不依赖逐条删除 Redis Key 保证撤销。
- Cookie Max-Age/Expires 不得超过 Session 剩余有效期；滑动续期同步更新 Cookie。

### 4.3 Redis 故障策略
复用 `storage.OpenRedis` 返回的既有 go-redis Client；Platform security 不得自行创建第二套 Redis 连接池。现有 Storage 仅提供 Redis 生命周期和 Ping，并未提供 Session Store 或原子限流。

Redis 是短期可丢失 Session/限流状态存储，不是 Owner Credential 的事实源。Session Store 读写、Touch、Rotate、Revoke 失败时认证 fail closed；不回退内存 Session、Bearer 或匿名模式。Redis 数据丢失时所有 Session 视为失效，用户重新登录。

SessionStore 的原子性覆盖创建、读取并刷新、轮换、撤销和 TTL 更新；推荐使用 Redis Lua/事务实现关键多键操作，并用真实 Redis integration + 并发测试证明语义。具体 Key 格式和脚本在实现前评审。

## 5. Cookie 与 CSRF

Cookie 默认 HttpOnly；HTTPS 部署 Secure=true；SameSite=Lax；不设置宽泛 Domain；Path 限制为实际 Web/API 公共前缀；Cookie 名称由 Server 配置；退出时以相同 Name/Path/Domain 属性清除。反向代理后的 HTTPS 识别只信任明确配置的代理网络和转发头，不能无条件信任 X-Forwarded-Proto。

Platform 提供 Session 绑定 CSRF Token 的生成、摘要保存和常量时间验证。Server 负责方法分类、Origin/Host 校验及路由接入：
- 每个 Session 独立生成高熵 CSRF Token。
- 通过受保护的同源引导响应交给前端；前端只保存在运行时内存，通过专用 Header 回传，不写 localStorage。
- Cookie 认证的所有状态变更请求必须校验 CSRF；登录入口也需校验 Origin/Host，防止 Login CSRF。
- 状态变更请求严格匹配配置的公开 Origin；Host 不可信或 Origin 缺失/不匹配默认拒绝。
- GET/HEAD/OPTIONS 不允许产生业务状态变更。
- SameSite=Lax 是纵深防御，不能替代 CSRF Token 和 Origin/Host 校验。

## 6. 登录限流

Platform 提供 RateLimiter 接口和 Redis 原子窗口计数/退避实现；Server 配置阈值并识别可信来源 IP。
- 账号标识和来源 IP 双维度限流，两个维度都必须满足策略。
- 失败次数、时间窗口和 retry_after 由策略配置；逐步退避。
- 不使用永久账号锁定，避免攻击者故意输错密码造成 Owner 拒绝服务。
- 账号维度键与 IP 维度键采用摘要形式，避免 Redis Key 暴露原始标识。
- 来源 IP 默认取直接连接地址；只有配置可信代理链时才读取对应转发头，禁止无条件信任 X-Forwarded-For。
- 成功登录按策略清理/衰减失败计数；返回结果不得泄露账号是否存在。
- Redis 限流操作失败时首期 fail closed，不允许无保护登录。

## 7. 安全审计

Platform 提供 SecurityAuditEvent Schema、字段约束/脱敏和注入式 AuditWriter；Server 产生具体事件并装配持久化后端。事件至少覆盖登录成功/失败、退出、Session 创建/轮换/撤销、密码修改/恢复、登录限流触发和敏感管理操作。

事件至少包含 event_type、subject、result、occurred_at、request_id、source 分类和安全 reason_code。不得包含密码、哈希、Session/CSRF Token、Authorization Header 或完整敏感请求体。

不建设全量业务审计平台。Writer 写入失败必须可诊断并有明确策略，不允许静默吞掉关键安全事件。保留期限和查询界面由 Server/运维按实际需求确定。

## 8. Platform / Server / Web 责任矩阵

| 能力 | Platform | Server | Web / 部署 |
|---|---|---|---|
| 密码 | Argon2id、参数版本、CredentialStore | 初始化、修改、恢复流程 | 密码交互、初始 Secret 注入 |
| Session | Token 摘要、SessionStore、Redis 原子实现、生命周期 | 登录创建、Cookie 解析、credential_version 校验、退出撤销 | HttpOnly Cookie 使用、HTTPS |
| Cookie | 属性校验和创建/清除辅助 | 名称、Path、公开 Origin、代理信任配置 | 不读取 HttpOnly Token |
| CSRF | Token 生成/绑定/验证 | 路由接入、Origin/Host、安全方法校验 | 运行时内存保存 CSRF Token 并用 Header 回传 |
| RateLimit | 接口、原子窗口计数和退避 | 登录策略、可信来源识别 | 代理网络边界 |
| Audit | 事件 Schema、脱敏、Writer 接口 | 产生事件、选择 Writer、失败策略 | 运维日志/保留策略 |

## 9. Platform 候选任务与依赖

| 编号 | 任务 | 主要交付 | 依赖 |
|---|---|---|---|
| PLAT-015 | Owner Password / Credential Store | Argon2id、参数版本、CredentialStore、MariaDB 事实源 | Config/Secret、Storage |
| PLAT-016 | Web Session / Cookie | SessionStore、Redis 原子实现、过期/轮换/撤销、Cookie helper | PLAT-009、PLAT-015 |
| PLAT-017 | CSRF | Session 绑定 Token 原语 | PLAT-016 |
| PLAT-018 | Login RateLimit | RateLimiter、Redis 原子窗口计数和退避 | PLAT-009 |
| PLAT-019 | Security Audit Contract | 事件 Schema、脱敏、AuditWriter | Logging/Fault |

执行顺序：契约评审 → PLAT-015 → PLAT-016 → PLAT-017/018 → PLAT-019 → Server A2 → 部署 A3。接口稳定后 PLAT-017/018/019 可并行。

## 10. 验收分层

**Platform L1**
- Argon2id 参数、哈希格式、输入限制、未知版本和 NeedsRehash 有测试。
- Session Token 高熵、仅存摘要；Touch、Rotate、Revoke、双过期时间、并发竞争和 credential_version 有测试。
- Redis 真实集成验证原子操作、TTL、故障和数据丢失 fail closed。
- CSRF Token 绑定、错误/跨 Session Token 拒绝和常量时间验证有测试。
- RateLimit 双维度原子性、窗口边界、退避和故障策略有测试。
- Audit Event 敏感字段约束、脱敏和 Writer 错误有测试。

**Server A2**
- 登录、退出、密码修改、受控恢复、路由认证方式矩阵和 Owner 权限端到端通过。
- Origin/Host、CSRF、账号/IP 限流和安全审计事件实际接入。
- Nginx 与直连 Server 的认证行为一致；无旁路 Handler。

**部署 A3**
- HTTPS/Secure Cookie、可信代理配置、入口不可绕过。
- Redis 不可用时登录和已登录请求均 fail closed。
- Redis 数据丢失后的重新登录流程可用。
- 密码恢复、credential_version 撤销和安全事件留存有真实环境演练。

## 11. 当前状态与未决门禁

截至 Platform HEAD `fd45b6c299dfb54d2530bd1436589fe4708d8e96`：
- 现有 `internal/platform/security` 只有 Bearer Authenticator、Router Policy 和测试。
- 现有 `internal/platform/storage.OpenRedis` 只提供 Redis Client 生命周期，没有 Session Store 或 RateLimiter。
- 当前 go.mod 已有 `github.com/redis/go-redis/v9`，但没有 Argon2id 直接依赖；实现时需评估并引入成熟的 `golang.org/x/crypto/argon2` 依赖。
- Argon2id Hash/Verify/NeedsRehash 原语已新增，使用显式参数，不提供未经硬件基准确认的生产默认值；其余 CredentialStore、Session、Cookie、CSRF、RateLimiter 和 Security Audit 原语仍未实现。
- 尚未新增平台 Credential Migration、CredentialStore 表或 Redis Key。
- Argon2id 参数需目标硬件基准；CredentialStore 表结构/迁移注册方式、Redis 原子脚本、限流阈值和 Audit Writer 持久化后端需评审。
- 本文是 Platform 技术设计与实现边界文档；Web 登录路由/权限/交互契约由 Server Owner 跟进，Platform 不代替 Server 修改其业务文档，也不改变项目级契约登记簿状态。

## 12. 安全设计原则

1. 认证失败默认拒绝；Session/RateLimit 依赖不可用不降级。
2. Web Session 与 Owner/Service Bearer 彻底隔离。
3. MariaDB 保存密码事实与版本；Redis 保存短期会话/限流状态。
4. 撤销正确性优先于减少一次数据库读取。
5. Token 只以摘要持久化，明文只在签发响应/浏览器 Cookie 的必要路径短暂存在。
6. 不信任未配置的代理头、Host 或客户端身份 Header。
7. Platform 只交付技术原语，Server 负责流程与入口，Web 负责交互。
8. 设计完成不等于实现完成；L1/A2/A3 分别验收。
