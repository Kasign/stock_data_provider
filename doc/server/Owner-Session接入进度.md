# Owner Session 接入进度

> **状态口径（2026-11-18）：** 本文件保留 Owner Session 专项实现和历史验收证据；当前 Server 总体状态以 [当前任务状态和进度](./当前任务状态和进度.md) 为准。Run 43869239700 是历史 Integration SHA 的验收，不代表当前 Server HEAD 或目标生产环境部署已通过。WebUI Checks/Browser Smoke 已在 Web 代码验收基线上通过；Owner Session 目标环境部署、安全运维和 Cookie/Origin/CSRF 实测仍待完成。

## 当前阶段：WebUI 已接入，目标环境部署验收待完成

当前文档核对基线：`integration` @ `446cc1a65c307726a1171e1341d529452ea09f55`。下方早期内容保留为历史记录；以文末最新状态校正为准。

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

## 契约归属复核（纠正上一版判断）

本轮继续核对了项目级决策记录、契约登记簿、契约变更规则和契约入口：

- `DEC-PLAT-002-017` 已明确 Owner 密码登录的总体决策、凭证隔离、Credential/Session/CSRF 原则、Server/Web/部署责任分层。
- `PLAT-CTR-003` 已冻结身份来源、Session 原语和路由认证边界；其第 9～10 节将登录 HTTP API 的路径、请求/响应、错误和路由矩阵指向 `SRV-CTR-001`。
- 但登记簿中 `SRV-CTR-001` 的正式名称是“Root Server 组合根、统一 HTTP 宿主与 Researchflow 对外 API”，状态为 `deferred`；重新进入条件明确针对 Researchflow 的消费者范围、活动路由、DTO、认证矩阵、错误投影、分页/Cursor 和事件读取。
- DEC-PLAT-002-017 第 55 行仅将 `SRV-CTR-001` 列为相关项目契约，并未为 Owner Session HTTP API 单独建立路径/DTO 定义。
- 契约版本规则要求：已冻结契约语义不得原地改变；若新增或改变公开契约，必须登记决策、消费者影响、迁移方案和兼容边界。

因此，准确结论不是“SRV-CTR-001 已经包含 Owner Session API”，也不是“必须把 Owner Session API 塞进 SRV-CTR-001”。而是：

1. Owner Session 的项目级安全决策已经存在（DEC-PLAT-002-017），不需要重新讨论身份模型。
2. Owner Session HTTP 路由/DTO 仍缺少权威定义。
3. 当前登记簿中的 SRV-CTR-001 处于 deferred，且其重新进入条件指向 Researchflow API；不能把它当作已发布的 Owner Session HTTP 契约。
4. 需要项目契约治理修正 PLAT-CTR-003 对 Server HTTP 契约的引用，明确 Owner Session HTTP API 的权威文档归属。若仅属于 Server 内部管理 API，可由 DEC-017 下的 Server 专项 API 契约承载；是否新增项目级 CTR 编号，应由登记簿治理按跨模块消费者范围决定，不能由实现代码先行决定。
5. 在引用修正和权威定义落地前，不新增公开路由、DTO 或 Web 调用约定。

**上一版“SRV-CTR-001 可能补充或另建 Server HTTP Auth CTR”的表述过于笼统；本节以当前登记簿和 DEC-017 为准。**

## 代码已确定的 HTTP 接入约束

进一步核对 `internal/server/security/session_service.go`、`session.go` 与 `config/session.go` 后，以下内容无需再从零设计：

- 登录业务固定为单 Owner，内部 `LoginInput` 只有 Password、由可信入口解析的 SourceIP 和 RequestID；不存在用户名/邮箱字段。HTTP DTO 不应自行引入账号选择。
- 登录密码上限已由 Service 校验为 1024 字节；空密码拒绝。HTTP 层仍需设置合理请求体上限并拒绝未知/重复 JSON 字段。
- 登录来源 IP 必须由 Server 可信入口策略解析后传入；Lifecycle Service 明确不读取 `X-Forwarded-For`。当前配置尚未展示可信代理 CIDR 配置项，因此生产接入前仍须确定可信代理边界。
- 登录成功返回内部 `SessionIssue`：HttpOnly Session Cookie + 明文 CSRF Token。HTTP 层必须将 Cookie 放入 Set-Cookie，并通过响应 DTO 将 CSRF Token 交给 Web；不能返回 Session Token。
- 默认 CSRF Header 已在 Server 代码中定义为 `X-CSRF-Token`，配置项为 `OWNER_SESSION_CSRF_HEADER`。
- 当前默认 Cookie Name 为 `__Secure-owner_session`，Path 为 `/api`；Public Origin 必须显式配置；HTTPS Origin 强制 Secure Cookie。
- 登录限流配置已有默认值：账号失败阈值 5、IP 失败阈值 10；Window/Backoff 参数由配置结构提供。不能在 Handler 内另造一套限流。
- Lifecycle 的 `ChangePassword` 当前只接受当前密码、新密码和 RequestID；成功后 CredentialVersion 更新使旧 Session 认证失效。HTTP 层应在调用前完成当前 Session 身份、Origin/Host 和 CSRF 校验。
- `InitializeOwner` 是内部运维用例，不是浏览器 API；调用后必须从 Secret 来源清理初始 Secret，方法会清理传入字节切片。

### 发现的真实协议缺口：页面刷新后的 CSRF 恢复

登录和 Session 轮换会返回明文 CSRF Token，但 Platform SessionStore 只保存 CSRF Digest；当前 Server 没有“为已登录 Session 安全重新签发/恢复 CSRF Token”的独立用例。

这会造成实际浏览器问题：CSRF Token 只保存在 JS 内存，页面刷新后内存丢失；仅靠 HttpOnly Session Cookie 无法恢复原始 CSRF Token。不能通过读取 Digest 还原 Token，也不能把 Token 改存 localStorage 来绕过。

HTTP 契约必须明确安全恢复方案。当前可评估的最小方向是：提供一个严格同源、校验 Host/Origin、只接受有效 Owner Session Cookie 的 CSRF 引导操作，重新生成并绑定 CSRF Token 后返回给 Web；但该操作会改变 Session CSRF Digest，不能伪装成无副作用 GET，也必须在契约中明确其 CSRF 豁免边界、重放/并发行为和审计语义。现有 `RotateSession` 会同时轮换 Session Token，不应未经确认就把它当成普通页面刷新接口。

此外，当前 `SessionAuthenticator` 在验证 CSRF 前调用 Platform `LookupAndTouch`，错误 CSRF 请求可能刷新 Idle TTL；该问题已有 Server README 记录，需 Platform 提供只读 Lookup 或原子校验/Touch 原语，Server 不复制 Redis 实现。

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


## 2026-11-16 主流程决策更新

当前 integration 基线：`c40e58482f5bded7fbdaeb8958c674be63be2d74`。本轮依据项目负责人授权形成 DEC-PLAT-002-053，明确两项前置工作：

1. **契约归属**：新增独立 `SRV-CTR-002` 管理 Owner Session HTTP/运维协议；`SRV-CTR-001` 保持 Researchflow 对外 API 的 deferred 状态，不将登录 DTO、CSRF 协议或 Owner 初始化混入 Researchflow 契约。
2. **CSRF 原子 Touch**：保留既有 `SessionStore` 接口，增加可选 `CSRFSafeSessionStore` 能力。Platform Redis 实现必须在单个原子操作中比较 CSRF Digest 并 Touch；错误 CSRF 不得刷新 idle TTL。Server 对非安全 Session 请求缺少该能力时 fail closed。
3. **浏览器预期**：Cookie 为 HttpOnly/SameSite=Lax/无 Domain；登录和轮换返回内存态 CSRF Token；页面刷新后通过 `POST /api/v1/owner/session/csrf` 恢复新 Token。WebUI 不保存 Owner Bearer 或 CSRF Token 到 localStorage。
4. **权限预期**：owner_session 不继承 platform:admin；旧运维/诊断入口继续 owner_bearer；Researchflow 浏览器业务路由待 SRV-CTR-001 明确后逐条允许 owner_session，领域资源授权仍由领域 Owner 完成。
5. **初始化预期**：Owner 首次初始化与密码恢复均不提供匿名 HTTP 接口，只能走受控本机/运维 Secret 流程。

本次只完成契约定义与任务边界调整，不宣称 Platform 原子操作、Server HTTP 路由或 WebUI 登录已经实现。下一步直接实现 Platform 可选扩展接口与 Redis 原子脚本，再接 Server SessionAuthenticator；契约 Schema/Golden/OpenAPI 与 HTTP Handler 按同一 SRV-CTR-002 定义推进。


## 2026-11-17 实现状态校正（以本节为最新）

本节覆盖并纠正前文“等待 HTTP API 定义 / HTTP 接入未完成”的历史状态。权威接口定义已存在于 `SRV-CTR-002`；当前实现状态如下。

- 代码验收基线：`46109c58d440327973a21d833f215928543a14b6`。
- Integration CI Run [43996341918](https://github.com/Kasign/stock_data_provider/actions/runs/43996341918)：对该精确 SHA 为 `success`；覆盖全仓 gofmt、根模块 go vet/test、News/Research/Researchflow 跨模块测试、Worker Bootstrap MariaDB/Redis、go-core vet/test、Compose、WebUI 检查和镜像构建。
- `internal/server/bootstrap/router.go` 已在 Owner Session 启用时挂载 `/api/v1/owner/` Handler；旧 Analysis、/readyz、/diagnostics 仍保持 Owner Bearer，不自动接受 Owner Session。
- `internal/server/bootstrap/owner_session_http.go` 已实现 SRV-CTR-002 定义的登录、当前 Session、CSRF 恢复、登出、Session 轮换和密码修改路由。
- `cmd/analysis-api owner-init` 已提供受控本机初始化命令；通过 `OWNER_SESSION_INITIAL_SECRET_FILE` 读取仅 Owner 可访问的普通文件。命令不会自动删除 Secret 文件，运维流程必须在成功后删除/卸载该文件；重复初始化由 MariaDB 一次性 Credential 插入约束拒绝。
- HTTP 单测覆盖登录、Cookie 属性、当前 Session、CSRF 恢复和轮换、错误 CSRF 不 Touch、登出清 Cookie、Origin 拒绝、密码修改后旧 CredentialVersion Session 失效，以及 Bearer/Session 身份隔离。
- 密码修改 Service 已补齐当前密码与新密码均为 1～1024 字节的限制；超限输入在访问 Credential 和写审计前拒绝。

### 当前仍未完成的验收

1. **SRV-CTR-002 L1 配套资产**：契约登记簿仍为 `freeze-ready`；`doc/server/schema/SRV-CTR-002/**`、`doc/server/golden/SRV-CTR-002/**` 和 `doc/server/openapi/owner-session.openapi.yaml` 尚未落地。契约定义本身不得自行扩展或改写，配套资产应逐项映射现有 DTO 和路径。
2. **真实存储端到端**：现有 CI 分别验证 MariaDB Credential Store、Redis Platform 原语和 Bootstrap 依赖组合，但尚无一条真实 MariaDB + Redis 的完整 HTTP 生命周期测试，覆盖初始化→登录→认证→CSRF→轮换→密码修改→旧 Session 失效→重新登录→登出。
3. **WebUI 消费端**：`web/src/api.js` 尚无 Owner Session API 封装，页面也没有登录/退出和内存态 CSRF 恢复流程；`doc/web/API_CONTRACT.md` 已更新服务端路由事实，但 WebUI 尚未接入。
4. **部署验收**：尚未验证目标 Nginx/Compose 环境中的同源 Cookie、Secure、Path、Origin/Host 和反向代理行为；不得将 CI 的 Handler 测试描述为生产部署闭环。

### 下一步执行顺序

先补齐 SRV-CTR-002 Schema/Golden/OpenAPI 并核对实现与契约逐项一致；再增加真实 MariaDB/Redis HTTP 生命周期集成测试；通过后接入 WebUI 登录、Session 恢复、CSRF 内存态、登出和密码修改。完成上述步骤前，旧 Analysis Bearer 路由保持原策略，Researchflow 浏览器业务路由不提前开放。


## 2026-11-17 SRV-CTR-002 契约资产补齐

依据既有 SRV-CTR-002 1.0.0 文本定义，没有改变接口路径、字段、认证矩阵或安全语义，补齐契约配套文件：

- JSON Schema：登录请求、Session 签发响应、当前 Session 响应、CSRF 恢复响应、密码修改请求，共 5 份。
- Golden：登录、当前 Session、CSRF 恢复、Session 轮换、登出、密码修改，共 6 份。
- OpenAPI：`doc/server/openapi/owner-session.openapi.yaml`，覆盖六条接口及 Cookie/CSRF 安全方案。
- OpenAPI Schema 相对引用指向 `doc/server/schema/SRV-CTR-002/`；Cookie/CSRF 原始值只以测试占位符表达，不记录任何真实凭证。

契约登记簿状态仍为 `freeze-ready`，没有擅自改为 `frozen`。配套资产补齐不等于契约冻结审批，也不等于真实 MariaDB/Redis 生命周期或 WebUI 部署验收完成。

下一项主流程工作：增加真实 MariaDB + Redis 的完整 HTTP 生命周期集成测试。测试通过后，再实现 WebUI 同源登录、页面刷新 CSRF 恢复、登出与密码修改。


## 2026-11-17 真实 MariaDB/Redis HTTP 生命周期验收

<badge color="success">已通过</badge>

- 代码验收 SHA：`5f23e1a0723a25c90ef2456ff318e25fa3e61b94`。
- Integration CI Run [43998030882](https://github.com/Kasign/stock_data_provider/actions/runs/43998030882)：对该精确 SHA 为 `success`。CI 的根模块测试使用真实 MariaDB 11.4 与 Redis 7.4 服务；`PLATFORM_STORAGE_INTEGRATION=1`、MariaDB DSN 和 Redis 地址均由 Workflow 注入。
- 新增 `internal/server/bootstrap/owner_session_mariadb_redis_integration_test.go`。测试创建隔离 MariaDB Database、执行 Root Server 全部 Migration，并使用唯一 Redis Key Prefix；不复用 CI 其他测试的 Owner Credential 或 Session。
- 完整 HTTP 生命周期实际执行：一次性初始化 Owner → 密码登录 → GET 当前 Session → POST CSRF 恢复 → POST Session 轮换 → PUT 密码修改 → 验证旧 CredentialVersion Session 返回 401 → 使用新密码重新登录 → DELETE 登出并清 Cookie。
- 使用真实 SQL Credential Store、SQL Security Audit Writer、Redis Session Store、Redis Login Rate Limiter 和 Root Server 路由。测试校验审计记录数量，结束后清理隔离 Database 和 Redis 前缀数据。
- 该结果证明测试环境的 MariaDB/Redis 与 Handler 生命周期闭环通过；不等于目标服务器 Nginx/Compose 部署已验收，也不等于 WebUI 已接入。

### 当前剩余主流程

1. WebUI 增加 Owner Session API 封装：登录、当前状态、页面刷新 CSRF 恢复、登出、Session 轮换、密码修改；CSRF Token 只保存在运行时内存，Owner Bearer 与行情 API Key 不进入浏览器。
2. 增加登录/退出界面和受保护页面的登录态初始化；登录失败、Session 过期、服务不可用分别显示，不得将未认证误判为网络成功。
3. WebUI 单测覆盖 Cookie 同源凭据、CSRF Header、页面刷新恢复、退出清理、密码修改后重新登录；随后执行真实 Nginx/Compose 同源部署验收。
4. SRV-CTR-002 当前仍为 `freeze-ready`，未擅自冻结；Schema/Golden/OpenAPI 已建立，后续需做契约校验和消费者符合性验收。

Owner Session 的 Server 端真实存储生命周期已不再是阻塞项。WebUI 接入是当前 P0 下一步；Researchflow 新业务 API 仍保持 SRV-CTR-001 的 deferred 边界，不得提前开放。


## 2026-11-17 WebUI 接入状态校正（以本节为最新）

当前代码基线：`integration` @ `446cc1a65c307726a1171e1341d529452ea09f55`。

### 已完成的 WebUI 消费接入

- `web/src/api.js` 已封装 SRV-CTR-002 六条 Owner Session 接口：登录、当前 Session、CSRF 恢复、登出、Session 轮换、密码修改。
- Owner Session 请求使用同源 Cookie；不附加 Owner Bearer，也不附加行情 API Key。
- CSRF Token 保存在模块级运行时内存，通过 `X-CSRF-Token` 发送；无 localStorage/sessionStorage 持久化。
- 页面初始化先恢复当前 Session，再调用专用 POST CSRF 恢复接口；401 清理内存 Token，网络/服务端异常显示为不可用，不误报为已登出成功。
- 设置中心包含 Owner 密码登录、退出和密码修改表单；密码修改成功后清理当前 CSRF Token 并要求重新登录。
- 旧 Analysis、`/readyz`、`/diagnostics` 仍保持 Owner Bearer 认证；Owner Session 不继承 `platform:admin`，也不自动授权旧路由。

### 自动化验收证据

- 精确 SHA：`446cc1a65c307726a1171e1341d529452ea09f55`。
- Integration CI Run [43999751873](https://github.com/Kasign/stock_data_provider/actions/runs/43999751873)：success。
- CI 覆盖全仓 gofmt、根模块 go vet/test、News/Research/Researchflow 跨模块测试、Worker Bootstrap MariaDB/Redis、go-core vet/test、Compose 配置、JavaScript 语法、WebUI 单测和 Web 镜像构建。
- `web/tests/api.test.mjs` 覆盖 same-origin credentials、登录/当前 Session/CSRF 恢复/轮换/登出顺序、CSRF Header、运行时内存状态、密码字节长度限制和密码错误后保留 CSRF。
- `web/tests/ui-contract.test.mjs` 覆盖登录表单、退出和密码修改入口、刷新恢复调用、CSRF 不写入 localStorage/sessionStorage，以及 Owner Session 与服务密钥边界。

### 仍未完成

1. 目标服务器的外部 Nginx / Compose / HTTPS 部署验收。
2. 真实浏览器在目标域名下验证登录 Cookie、代理 Cookie Path、Secure、SameSite、Origin/Host。
3. 真实浏览器验证刷新 CSRF 恢复、登出清 Cookie、密码修改后旧 Session 失效并重新登录。
4. SRV-CTR-002 项目契约登记状态仍为 `freeze-ready`；需由契约治理流程完成冻结，不能以代码和 CI 通过代替正式冻结。

### 下一步

Owner Session 不再安排新的功能开发。目标环境验收完成后，将开发主线转回 Researchflow 生产入口和运行时；SRV-CTR-001 仍保持 deferred，不能提前开放 Researchflow 浏览器业务 API。
