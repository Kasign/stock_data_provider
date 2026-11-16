# Owner Session 接入进度

## 当前阶段：HTTP 契约核对完成，等待 Server API 定义

当前 `integration` 基线：`c6b6894b3cd6ba3d241da1c9a6badeac5f145cc5`。

对应 Integration CI Run `43869239700`：success。该 SHA 的 gofmt、根模块测试、跨模块测试、MariaDB/Redis Bootstrap 集成、go-core、Compose 与 WebUI 检查均通过。

Server Bootstrap 已通过共享 MariaDB / Redis 资源组合以下能力，不创建独立连接池，也不复制 Platform 安全实现：

- Platform SQL Credential Store
- Platform Redis Session Store
- Platform Redis Login Rate Limiter
- Platform Argon2id Hasher
- Server SQL Security Audit Writer
- Server SessionLifecycleService
- Server SessionAuthenticator

Owner Session 默认关闭；启用时必须提供显式配置与经过目标硬件基准测试的 Argon2id 成本参数。依赖缺失或构造失败时启动 fail-closed。

## 文档复核结果

本轮核对了：

- `doc/platform/18-Web登录与Session安全原语设计.md`
- `doc/contract/rules/PLAT-CTR-003-身份授权与Session原语契约.md`
- `doc/contract/rules/PLAT-CTR-001-HTTP错误投影契约.md`
- `doc/server/README.md` 第 12.10、12.63～12.65 节及后续 Owner Session 记录
- `doc/web/API_CONTRACT.md`
- `doc/contract/PLAT-002-项目级契约冻结登记簿.md`

### 已有明确描述，可直接按现有约束实现

1. **身份与凭证隔离**
   - Owner Bearer 仅用于可信运维，不下发浏览器。
   - Owner Session 使用 HttpOnly Cookie，身份为 `owner_session`，不继承 `platform:admin` 或 Service Scope。
   - Bearer 与 Session 必须独立验证；不得伪造 Authorization Header 复用 Bearer Router。
   - 每条路由必须显式声明允许的认证来源。

2. **密码与 Credential**
   - 单 Owner，Subject 固定为 `owner`；MariaDB 是权威事实源。
   - 使用 Argon2id；生产参数必须经过目标硬件基准，不能使用未经验证的默认成本。
   - Credential 初始化仅允许成功一次；密码更新使用 CredentialVersion CAS。
   - 密码修改/受控恢复递增 CredentialVersion，使旧 Session 失效。

3. **初始化与恢复原则**
   - Platform 设计文档规定首次初始化通过一次性部署 Secret 提供初始密码；仅当尚无 Owner Credential 时生效。
   - 初始化成功后清理初始 Secret，后续重启不能再次重置密码。
   - 账号恢复只允许受控本机/运维流程，不提供匿名 HTTP 重置接口。
   - 因此可以实现受控运维初始化，但不能新增匿名初始化路由。具体 Secret 注入、消费、清理和运维操作方式仍需在 Server 配置/运维文档中落到可执行步骤。

4. **Session 生命周期**
   - Token 为 256-bit CSPRNG；持久化只保存 SHA-256 Digest。
   - Idle TTL 12 小时、Absolute TTL 7 天；Touch 不得延长绝对期限。
   - 登录创建新 Session；轮换原子撤销旧 Token；登出撤销当前 Session。
   - Redis/MariaDB 故障必须 fail closed；认证时核对 MariaDB 当前 CredentialVersion。

5. **Cookie、CSRF 与来源校验**
   - Cookie 固定 HttpOnly、SameSite=Lax、无宽泛 Domain、受限 Path；HTTPS 公网部署 Secure=true。
   - Cookie 有效期不得超过 Session 剩余期限；登出清除 Cookie 必须复用相同作用域属性。
   - Cookie 认证的状态变更请求必须校验绑定当前 Session 的 CSRF Token。
   - Server 必须校验配置的 Origin/Host；GET/HEAD/OPTIONS 不得执行业务状态变更。
   - 反向代理头只在可信代理链明确配置后参与安全决策；WebUI 的 CSRF Token 只存运行时内存并通过专用 Header 回传。

6. **登录限流与安全审计**
   - 登录同时执行账号和来源 IP 两个维度限流；Redis 故障 fail closed。
   - 登录成功/失败、限流、登出、Session 创建/轮换/撤销、密码变更/恢复等事件进入 Security Audit。
   - Server 已明确审计写入失败策略，不能静默吞掉安全事件。

7. **统一错误投影**
   - HTTP 错误应遵循 PLAT-CTR-001 的公开错误 DTO 和状态码映射，不得直接序列化内部 Fault。
   - 登录失败不得泄露 Owner Credential 是否存在；密码、哈希、Session/CSRF Token、Cookie、Authorization Header 不得进入日志、Fault 或审计。

### 文档尚未定义，不能自行猜测的 HTTP 内容

以下不是缺少安全原则，而是缺少 Server 对外 HTTP 契约：

- 登录、登出、Session 轮换、当前 Session/CSRF 引导、密码修改的正式 Method 与 Path。
- 登录请求 DTO 的字段名、格式和大小限制；成功响应 DTO 的字段及 Cookie/CSRF Token 交付方式。
- 登出、轮换、密码修改的请求/响应 DTO、状态码和错误码细节。
- 每条现有与新增业务路由的认证矩阵：哪些接受 `owner_session`，哪些仅接受 `owner_bearer`，哪些接受 Service Bearer。
- 登录入口 Origin/Host 缺失、重复、错误时的 HTTP 错误投影；CSRF Header 的正式名称（Platform 原语不替 Server 定义浏览器 Header 名称）。
- 一次性初始化 Secret 的 Server 配置名、注入方式、消费/清理时机、部署运维命令及初始化结果检查流程。
- Cookie Name/Path、公开 Origin、可信代理 CIDR、登录限流策略等生产配置项的最终默认值与配置约束。

这些内容应形成 Server 的权威 HTTP/运维契约，再实现 Handler 和路由。当前 `doc/web/API_CONTRACT.md` 也明确记录 Web 尚未接入 Session/Cookie/CSRF，不构成登录 API 定义。

## 契约编号引用问题

`PLAT-CTR-003` 第 9～10 节明确要求 Server 独立实现 Session Middleware，并说明登录 HTTP API 的路径、请求/响应、错误和路由认证矩阵由 `SRV-CTR-001` 定义。

但当前 `doc/contract/PLAT-002-项目级契约冻结登记簿.md` 中的 `SRV-CTR-001` 描述为“唯一组合根、Bootstrap、依赖注入与生命周期”，其权威内容是组合根和运行生命周期，不是 Owner Session HTTP API。Server README 中对 `SRV-CTR-001` 的部分说明也把 Researchflow HTTP API 混入其中。

因此当前存在**契约编号/权威路径引用不一致**。在项目契约登记簿明确归属之前，不把组合根契约当作登录 HTTP API 契约，也不在 Server 代码中自行发明公开路由和 DTO。应由项目级契约治理明确：Owner Session HTTP API 是补充到现有 Server 契约，还是单独登记新的 Server HTTP Auth 契约。

## 当前代码与测试证据

Bootstrap 组合测试覆盖：

- Session 关闭时不创建依赖；
- 缺少共享 MariaDB / Redis 时拒绝组合；
- Argon2id 参数无效时拒绝组合；
- Integration CI 提供 MariaDB / Redis 时验证完整 Lifecycle 与 Authenticator 组合。

Server 内部已有：

- `SessionAuthenticator`：校验 Host/Origin、唯一 Session Cookie、Redis Session、MariaDB CredentialVersion、unsafe method CSRF，并建立 `owner_session` Principal。
- `SessionLifecycleService`：内部提供 Owner 初始化、登录、登出、Session 轮换、密码修改和审计策略。
- SQL Security Audit Writer 与 Platform Owner Credential Migration 注册。

以上是内部能力，不代表 HTTP 接入已完成。SessionStore 只读 Lookup 缺口及其对错误 CSRF 请求可能刷新 idle TTL 的影响，仍按 README 记录的 Platform 消费者问题跟踪；不得在 Server 直接访问 Redis 私有实现。

## 尚未完成

1. 明确并登记 Owner Session HTTP API、错误投影和逐路由认证矩阵。
2. 明确一次性初始 Secret 的可执行运维流程与生产配置。
3. 按契约实现登录、登出、Session 轮换、Session/CSRF 引导、密码修改 Handler。
4. 将 SessionAuthenticator 接入明确允许 `owner_session` 的路由；Owner Bearer 与 Service Bearer 原行为保持独立。
5. 完成 Origin/Host、可信代理、CSRF、Cookie 续期/清除、限流、审计失败策略的 HTTP 集成测试。
6. 使用真实 MariaDB/Redis 验证初始化、登录、认证、轮换、登出、密码修改后旧 Session 失效。
7. 更新 Web API 契约并完成 WebUI 同源 Cookie/CSRF 接入；通过前不得宣称浏览器认证闭环完成。

**当前结论：Owner Session 内部安全能力和 Bootstrap 依赖组合已具备；HTTP 契约与 HTTP 接入仍未完成。不得接入 Researchflow Owner API，也不得开放匿名业务路由。**
