# SRV-CTR-002 Owner Session HTTP API 契约

- 契约编号：SRV-CTR-002
- 契约版本：1.0.0
- 状态：freeze-ready（待项目负责人确认冻结）
- 生产方：Root Server（HTTP Handler、Cookie、Session Middleware、路由认证矩阵）
- 消费者：WebUI；运维初始化流程
- 依赖：PLAT-CTR-001、PLAT-CTR-002、PLAT-CTR-003 1.1.0、GOV-CTR-004
- 范围：单一 Owner 的浏览器登录态，不定义 Researchflow 业务 API，也不定义领域资源授权策略。

## 1. 目标与硬边界

1. 浏览器只持有 HttpOnly Session Cookie；CSRF Token 只保存在 WebUI 运行时内存。
2. Owner Bearer 仅供受控运维/管理调用，不下发浏览器；Owner Session 不继承 `platform:admin` 或 Service Scope。
3. 所有 Session 请求必须经 Server Session Middleware；不得伪造 Authorization Header 复用 Platform Bearer Router。
4. 每条路由必须显式声明允许的 AuthMethod。未明确允许 `owner_session` 的业务路由不得接受 Session。
5. 登录、登出、轮换、密码修改均受 Origin/Host 校验；状态变更请求还必须校验 CSRF。
6. GET/HEAD/OPTIONS 不得产生业务状态变更。认证读取可更新 Session idle TTL，但仅在身份/CSRF 检查成功后更新。
7. 本契约不增加注册、多用户、匿名密码重置、OAuth/OIDC 或独立认证服务。

## 2. 公共路径与方法

Root Server 同源 API 前缀固定为 `/api/v1/owner`：

| 方法 | 路径 | 用途 | 允许身份 |
|---|---|---|---|
| POST | `/api/v1/owner/session` | 使用 Owner 密码登录并创建 Session | anonymous |
| GET | `/api/v1/owner/session` | 查询当前 Session 状态 | owner_session |
| POST | `/api/v1/owner/session/csrf` | 页面刷新后原子轮换并恢复 CSRF Token | owner_session（Origin/Host 必须匹配；不要求旧 CSRF） |
| DELETE | `/api/v1/owner/session` | 撤销当前 Session 并清除 Cookie | owner_session |
| POST | `/api/v1/owner/session/rotate` | 轮换当前 Session Token 与 CSRF Token | owner_session |
| PUT | `/api/v1/owner/password` | 修改 Owner 密码并递增 CredentialVersion | owner_session |

Owner 初始化不提供 HTTP 路由。它只能通过受控本机/运维命令消费一次性部署 Secret 完成；不得暴露匿名初始化、重置或恢复 HTTP 接口。

## 3. 请求与响应 DTO

所有 JSON 请求必须设置 `Content-Type: application/json`；拒绝未知字段、重复 JSON 值、尾随第二个 JSON 值和超出 Server 请求体上限的输入。所有响应使用 UTF-8 JSON，并遵循 PLAT-CTR-001 错误投影。Cookie 通过 Set-Cookie Header 发放/更新/清除，不在 JSON 中返回 Session Token。

### 3.1 登录

`POST /api/v1/owner/session`

请求：

```json
{"password":"<owner-password>"}
```

- password 必须为字符串，UTF-8 编码后长度 1～1024 字节；不得 Trim 或静默改写密码。
- 登录必须校验 Host 与 Origin，Origin 必须恰好一个且与配置的 PublicOrigin 完全匹配（scheme/host 不区分大小写比较）；不接受缺失、重复、null 或不匹配 Origin。
- 登录由 Server 从 RemoteAddr 取得来源 IP；仅在显式可信代理配置下解析转发链，不默认信任 X-Forwarded-For。
- 成功：HTTP 200，发放 HttpOnly Session Cookie；JSON：
  ```json
  {"schema_version":"1.0.0","authenticated":true,"csrf_token":"<csrf-token>","expires_at":"<RFC3339Nano UTC>"}
  ```
- 失败凭证统一返回 401，不泄露 Owner Credential 是否存在；限流拒绝按 PLAT-CTR-001 的稳定错误投影返回 429，并提供 Retry-After（若可确定）。
- 登录成功后立即提供 CSRF Token，避免首次状态变更必须额外猜测接口。

### 3.2 当前 Session 状态

`GET /api/v1/owner/session`

- 只接受 owner_session；必须完成 Cookie、Session、CredentialVersion 校验。
- 返回 HTTP 200：`{"schema_version":"1.0.0","authenticated":true,"expires_at":"<RFC3339Nano UTC>"}`。
- 不返回 CSRF Token；不得返回 Session Token、摘要或 CredentialVersion。
- 未认证/过期返回 401。

### 3.3 登出

`DELETE /api/v1/owner/session`

- 必须通过 owner_session、Origin/Host 和 CSRF 校验。
- 成功撤销当前 Session，返回 HTTP 204，并以相同 Cookie Name/Path/Domain/Secure/SameSite 属性清除 Cookie。
- 即使审计写入失败，也必须尽力清除浏览器 Cookie；响应按既有 SessionLifecycleService 的安全审计失败策略映射，不得恢复已撤销 Session。

### 3.4 Session 轮换

`POST /api/v1/owner/session/rotate`

- 必须通过 owner_session、Origin/Host 和当前 CSRF 校验。
- 成功原子撤销旧 Token 并创建新 Token；保留 created_at、credential_version、absolute_expires_at；更新 CSRF Token。
- 返回 HTTP 200，JSON 与登录成功响应相同，并发放新 Cookie。
- 旧 Cookie/CSRF Token 自轮换成功后立即失效；冲突或过期返回 401，不允许降级沿用旧 Session。

### 3.5 修改密码

`PUT /api/v1/owner/password`

请求：

```json
{"current_password":"<current-password>","new_password":"<new-password>"}
```

- 必须通过 owner_session、Origin/Host 和 CSRF 校验。
- 两个密码字段均为字符串，UTF-8 编码后长度 1～1024 字节；不得 Trim 或静默改写。
- 成功时使用 CredentialVersion CAS 更新密码并递增版本，使所有既有 Session 失效；当前 Session 也必须失效并清除 Cookie。
- 返回 HTTP 204。当前密码错误返回 401；新密码或输入不符合规则返回 400；并发版本冲突返回 409。
- 不返回密码哈希、CredentialVersion 或任何 Token。

## 4. Cookie、CSRF 与 Origin/Host

1. Session Cookie 固定 HttpOnly、SameSite=Lax、无 Domain、Path 使用 Server 配置的最小 API 路径；HTTPS 公网必须 Secure。
2. Cookie Max-Age/Expires 不得超过 Session 剩余 idle/absolute TTL 中较早者。
3. CSRF Header 固定为 `X-CSRF-Token`；必须恰好一个非空值。Token 由 Platform CSPRNG 生成并与 Session 绑定。
4. 所有非安全方法必须由 Platform `CSRFSafeSessionStore` 在原子 Touch 操作中比较 CSRF Digest；错误/缺失 CSRF 不得刷新 idle TTL。
5. Session 请求必须校验 Host 与配置 PublicOrigin；非安全方法必须有恰好一个匹配 Origin。登录也执行同等 Origin/Host 校验以防 Login CSRF。
6. GET/HEAD/OPTIONS 不得执行任何业务状态变更。CSRF Token 恢复属于凭证状态变更，必须使用专用 POST 端点，不得借 GET 执行。
7. `POST /api/v1/owner/session/csrf` 是唯一 CSRF 恢复例外：只接受有效 owner_session Cookie，必须严格校验恰好一个匹配的 Origin 和 Host，并通过 Platform 原子操作确认 Session 有效、替换 CSRF Digest。该端点不要求旧 CSRF Header（因为页面刷新后 Token 已丢失），但不得执行其他业务状态变更；其防跨站依据是强制 Origin/Host 校验与同源部署。返回 `{"schema_version":"1.0.0","csrf_token":"<csrf-token>"}`。新登录和 Session 轮换响应直接返回新 CSRF Token；页面刷新后 WebUI 使用此端点恢复内存 Token。该端点不得 Touch 后才验证 CSRF。

## 5. 错误、审计与隐私

- 所有错误使用 PLAT-CTR-001 ErrorEnvelope，不序列化内部 Fault。
- 401：缺失/无效/过期 Session 或密码验证失败；403：Origin/Host/CSRF 拒绝；409：Credential CAS/Session 轮换冲突；413：请求体超限；429：登录限流；503：必要安全依赖不可用。
- 登录失败不得区分账号不存在与密码错误。
- 安全审计覆盖登录成功/失败/限流、Session 创建/轮换/撤销、退出、密码修改和初始化。
- 日志、错误和审计不得包含密码、密码哈希、Session/CSRF Token、摘要、Cookie、Authorization Header、原始 IP 或完整请求体。
- 安全审计 Writer 失败按 SessionLifecycleService 已定义策略 fail closed；撤销已经发生时不得回滚撤销。

## 6. 初始化与运维

1. 一次性初始密码由部署系统通过专用 Secret 文件/受控 Secret 注入，不放入普通环境变量、镜像、Compose 明文或 WebUI。
2. 初始化命令仅允许本机受控运维执行；读取 Secret 后在内存中消费，命令退出时清零缓冲区，并要求部署侧删除/卸载 Secret。
3. MariaDB CredentialStore 的一次性初始化 CAS 是最终防重放门禁；重复执行不得覆盖现有凭证。
4. 初始化完成后，Server Diagnostics/运维命令只报告 initialized/not_initialized，不回显 Secret、哈希或账户材料。
5. 密码恢复仅允许受控本机运维流程，不提供匿名 HTTP reset；恢复必须递增 CredentialVersion 并撤销既有 Session。

## 7. 路由认证矩阵

- 本契约六个浏览器接口按表中身份执行；CSRF Token 恢复端点为 `POST /api/v1/owner/session/csrf`，身份为 owner_session，且是唯一不要求旧 CSRF Header 的状态端点。
- `/healthz` 保持 anonymous；`/readyz`、`/diagnostics` 保持 owner_bearer，除非各自独立契约明确改变。
- 现有旧 Analysis/兼容管理路由继续按当前 owner_bearer 策略，不因启用 Session 自动放宽。
- Researchflow 新业务路由由 SRV-CTR-001 定义；需要 WebUI 浏览器访问的用户用例必须明确声明 owner_session，并由领域 Owner 继续执行资源级授权。Server 不得把 owner_session 映射成 owner_bearer。
- 未登记的路由默认拒绝，不允许匿名兜底或隐式 Owner 双认证。

## 8. Schema、Golden 与版本

- 为每个 JSON 请求/响应建立独立 Schema/Golden，统一放在 `doc/server/schema/SRV-CTR-002/` 与 `doc/server/golden/SRV-CTR-002/`。
- OpenAPI 权威文件为 `doc/server/openapi/owner-session.openapi.yaml`；它只描述本契约路由，不合并/复用已退役旧 Analysis API Schema。
- DTO schema_version 固定 1.0.0；契约版本与 DTO Schema 版本独立。
- 任何路径、方法、字段、认证矩阵、Cookie/CSRF/Origin 语义或错误语义的非兼容变化必须升级 SRV-CTR-002 主版本并评估 WebUI 迁移。
- 当前定义不等于实现已完成；实现符合性必须通过 Schema/Golden、HTTP 集成测试、真实 MariaDB/Redis、Nginx 同源部署验收。

## 9. 明确不在本契约范围

- Researchflow CreateRun/Run/Stage/Event、分页、Cursor、取消和事件流；归 SRV-CTR-001。
- Data、News、Research、Quant 的资源授权规则；归各领域 Owner 与 GOV-CTR-006。
- 多用户、注册、邮箱验证、OAuth/OIDC、匿名密码重置。
- Owner Bearer 的运维凭证轮换策略；继续遵循 PLAT-CTR-003。
