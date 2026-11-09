# 安全认证与服务凭证设计

## 1. 目标与威胁模型

系统由单一所有者使用，但 API 可能经过公网反向代理，也存在多个内部服务调用。首期目标是明确边界、默认拒绝、凭证隔离和可轮换，不建设独立凭证管理系统。

重点防止未授权调用、伪造身份 Header、凭证泄露、Provider Token 冒充内部服务，以及管理接口暴露。

## 2. 首期身份模型

| 身份 | 用途 | 凭证来源 |
|---|---|---|
| anonymous | 明确允许的 liveness 等少数接口 | 无凭证，路由显式白名单 |
| owner | 所有者管理操作 | 独立 `PLATFORM_ADMIN_TOKEN` |
| service | 服务间调用 | 每个调用方独立的环境 Secret |

不实现用户注册/登录、组织、租户和独立 job 身份。后台任务记录创建来源、Request ID 和 task_id；执行身份来自服务端任务记录，不接受客户端自报身份。

## 3. 首期凭证方案

采用部署 Secret 注入，不建立 Service Key 数据库表，不提供创建/禁用/撤销 API，不建设管理页面。

- 每个调用方使用独立 Secret，不能让所有内部服务共用一个万能 Token。
- owner Token 与任意 service Token 分离。
- Secret 使用密码学安全随机数生成，建议至少 256 bit 随机熵。
- 通过 Authorization: Bearer 传递；禁止通过 URL Query 或普通自定义身份 Header 传递。
- 服务端只从启动配置读取 Secret；不散落读取环境变量。
- 使用常量时间比较验证，避免普通字符串比较泄漏比较时序。
- Secret 不写日志、错误、诊断、响应体或普通配置快照。
- 轮换通过部署 Secret 更新和服务重启/滚动切换完成；切换后旧凭证拒绝。
- Provider Token 与内部服务 Secret 完全分离。

对于少量固定调用方，部署 Secret 已能满足独立凭证和轮换要求。数据库摘要、Key ID、动态撤销和管理 API 会额外引入表、迁移、缓存一致性和管理流程，当前没有足够收益。

## 4. 未来动态凭证的触发条件

只有出现以下需求，才重新启动动态 Service Key 设计：
- 调用方数量明显增长，部署 Secret 管理开始不可维护；
- 需要不重启服务地创建、禁用或撤销某个调用方；
- 需要凭证到期策略、独立审计或无停机轮换；
- 多用户/多租户正式成为产品目标。

触发后再评估 Key ID + 高熵 Secret、DB 摘要存储、权限 Scope、撤销缓存和审计。当前不预建相关表或 package。

## 5. 最小授权

首期采用少量固定权限组，按真实路由裁剪：
- `platform:admin`：owner 管理接口；
- `data:read`：行情读取；
- `data:diagnose`：行情诊断；
- `research:run`：触发研究任务；
- `research:read`：读取研究结果。

无凭证、权限不足和未声明路由默认拒绝。不支持通配符、隐式继承或动态策略引擎。身份凭证只证明调用方身份，Scope 决定其允许访问的能力。

## 6. 路由访问策略

| 路由 | 访问要求 |
|---|---|
| liveness | 可匿名，只返回进程存活 |
| readiness | 由部署边界保护；仅返回最小依赖状态 |
| 普通业务读取 | 根据真实用途明确 owner/service |
| 业务写入、研究任务 | owner 或具备对应权限的 service |
| 管理和诊断明细 | owner；必要时允许指定 service 权限 |

所有路由显式声明策略；未声明默认拒绝。不能通过内网 IP、反向代理路径或客户端自定义身份 Header 绕过认证。

## 7. 管理入口与审计

管理入口只接受 owner Token，并优先限制在私有入口。首期不建设管理 UI 或凭证 CRUD API。

只记录必要的敏感管理事件，例如关键配置变更和未来动态凭证操作。日志记录主体类型、动作、结果、时间和 Request ID，不记录凭证明文。普通业务读取不做逐请求审计。

## 8. 失败策略与测试

- 凭证缺失、错误或配置为空：受保护路由拒绝。
- owner Token 不得访问仅 service 可访问的接口，反之亦然，除非路由明确允许。
- Key/Token 存储不依赖 Redis；Redis 故障不改变认证结果。
- 配置加载失败时服务启动失败，不以空 Token 启动受保护服务。
- 日志不得输出 Authorization 和 Secret。
- 测试覆盖有效/无效凭证、身份混用、伪造 Header、路由默认拒绝、常量时间比较调用路径和日志脱敏。



## 9. 平台认证原语实现补充（2026-11-05）

实现位于根 Module 的 `internal/platform/security`：

- owner 与 service 凭证通过 `config.Secret` 注入；owner 和每个 service 使用不同 Token。
- Token 至少 32 字节；服务启动时拒绝空值、弱长度、重复凭证和重复 service 名。部署侧应使用密码学安全随机数生成至少 256 bit 随机熵的 URL-safe Token；长度检查不能替代随机熵生成。
- 服务端只保存 SHA-256 摘要用于匹配；每次认证遍历全部凭证摘要并执行固定长度常量时间比较。高熵随机 Token 使摘要被离线猜测的风险保持在可接受边界。
- service 必须配置至少一个固定 Scope；拒绝未知 Scope、重复 Scope 和 owner 专属 `platform:admin` Scope。
- `Guard` 只信任 Bearer 凭证生成的 Principal，忽略客户端身份 Header；Principal 通过 Context 提供给 handler。
- Principal 写入 Context 和通过 `PrincipalFromContext` 读取时均复制 Scope 切片；调用方修改自己的 Principal 副本不能改变同一请求上下文中的授权快照。
- `Router.Handle/HandleFunc` 要求每条注册路由显式传入策略；未配置策略不能注册，未注册路由由 ServeMux 返回 404。禁止注册 `/`、host 根路径以及 `/{path...}`、`host/{path...}` 等 ServeMux 根级兜底模式，避免匿名/宽权限 Handler 意外接管所有未匹配 URL；精确根路由使用 Go 1.22 的 `/{$}`。
- `OwnerOrServiceAccess` 表示 owner 被路由显式允许，service 必须同时满足该路由声明的全部 Scope。owner 不会被自动映射成 service，也不会隐式继承业务 Scope。
- 匿名路由仅使用 `AnonymousAccess()`；若匿名请求携带错误 Bearer 凭证，仍返回 401，避免错误凭证被静默忽略。

该包不负责启动入口和现有服务路由改造。集成方必须将所有路由统一注册到受策略约束的 Router，不能保留旁路 ServeMux 或未保护的 Handler 注册路径。


### 9.1 凭证快照与轮换边界

`NewAuthenticator` 在构造时校验配置，并将凭证摘要、主体和 Scope 复制为进程内快照；运行中的 Authenticator 不监听环境变量变化，也不提供单条凭证撤销、替换或热加载接口。调用方修改构造参数或认证结果中的 Scope 切片，不得改变已构造的授权状态。

首期轮换因此属于**协调式重启轮换**，不是无停机轮换：

1. 生成新 Token，并通过部署 Secret 配置分发给调用方和服务端。
2. 协调调用方切换时点；服务端实例重启后只接受该实例启动时装载的新 Token。
3. 多实例部署应按统一窗口协调调用方切换与实例重启，避免旧、新 Token 在不同实例间出现不可预测的部分可用状态。
4. 验证旧 Token 在所有实例均已重启后失效，再清理旧 Secret。

首期不支持同一主体并存新旧 Token，不提供即时撤销保证。若未来要求无停机轮换、即时撤销或按主体独立禁用，应先评估 Key ID、双凭证过渡、版本化凭证快照和多实例一致性，再决定是否引入动态凭证存储；不得仅通过延长旧 Token 有效期或在业务层绕过认证实现。

## 10. PLAT-003 代码验证记录

最终验证提交：`f9334281ba0f90406fcd7de0f239afe24845bb23`。Platform packages workflow 已通过 gofmt、平台包单测、race test 和 go vet。

首次提交的 gofmt 检查未通过，已修正后重跑，最终提交 CI success。服务入口和真实路由接入尚未实施；凭证轮换的部署演练也未执行，因此本任务仍处于开发中。


### 9.2 多实例凭证轮换的可用性边界与建议方案

当前 Authenticator 的配置模型是一主体一个 Token：owner 只有一个 Secret，ServiceCredential 只有一个 Secret；同一 service 名不能重复。Authenticator 构造后只保留不可变摘要快照。因此，当前实现**不能执行无停机双凭证轮换**。

仅在调用方和服务端同时替换环境变量并滚动重启是不安全的：滚动期间，旧实例可能只接受旧 Token，新实例可能只接受新 Token；调用方切换到新 Token 后仍可能命中旧实例，产生间歇性 401。多实例下“协调一个切换时点”不能消除这个窗口，除非停止入口流量或接受短暂不可用。

首期可选择以下两种运行方式：

**A. 维护窗口轮换（当前代码即可执行）**
1. 暂停或排空受保护业务流量，确认后台任务和调用方进入可控状态。
2. 生成新 Token，通过部署 Secret 分发到全部服务端和调用方。
3. 重启全部服务端实例，确认每个实例均加载新 Token。
4. 更新/恢复调用方，执行受保护路由验收。
5. 确认旧 Token 已失效，再清理旧 Secret。
6. 恢复入口流量。若任一阶段失败，保持流量关闭并回滚到一套一致的旧配置，不允许新旧配置混合对外服务。

该方案简单，但会产生维护窗口；不能宣称无停机。

**B. 配置快照双凭证过渡（未来需要无停机时采用，不等同于动态凭证中心）**

当确实要求无停机轮换时，优先扩展静态配置模型，而不是先引入数据库、Redis 或动态管理 API：
1. 凭证配置允许同一主体关联多个独立 Secret；每个 Secret 仍只在启动时装载并转换为摘要。
2. 所有服务端实例先滚动升级为同时接受旧、新 Token；在全部实例确认完成前，调用方继续使用旧 Token。
3. 调用方逐个切换到新 Token，并由部署清单/发布记录确认所有调用方已完成切换。
4. 服务端配置移除旧 Token，再滚动升级全部实例；全部完成后旧 Token 才真正失效。
5. 任一阶段失败时按阶段回退：第二阶段之前可让调用方继续使用旧 Token；旧 Token 已从服务端全部移除后，不应为方便回滚而长期重新开放旧 Token。

建议的配置语义：
- owner 与 service 都可以在同一主体下配置多个 Token；主体身份和 Scope 保持一致。
- 每个 Token 必须全局唯一；禁止同一 Token 被不同主体或不同 Scope 复用。
- 多 Token 仅是部署配置快照，不增加运行时 CRUD、持久化凭证表、撤销缓存或跨实例广播。
- 配置变更仍通过实例重启生效；单实例移除旧 Token 后立即拒绝，多实例只有全部完成滚动后才可确认全局撤销。
- 如果未来需要自动识别旧 Token 是否仍被使用，应先定义不包含凭证明文的稳定 Credential ID 和审计策略；不得通过记录 Token、Token 摘要或 Authorization Header 实现。

### 9.3 当前实现的轮换执行门禁

在双凭证配置尚未实现前，生产轮换必须使用 9.2 A 维护窗口流程。以下条件全部满足才允许恢复入口流量：
- [ ] 所有服务端实例运行同一份凭证配置版本。
- [ ] 所有调用方已更新为同一套新 Token。
- [ ] owner 和各 service 的身份/Scope 路由矩阵通过验收。
- [ ] 使用旧 Token 请求每个服务端实例均返回 401。
- [ ] 使用新 Token 请求各自允许的路由成功，越权路由仍返回 403。
- [ ] 日志、响应和诊断结果未出现 Token 明文或可还原凭证的信息。
- [ ] 旧 Secret 已从部署配置和发布系统中清理。

若无法逐实例验证，应通过负载均衡摘流/维护模式确保所有实例完成切换；不能只抽样请求一次就宣称旧凭证全局失效。


### 9.4 Bearer Token 格式与输入长度约束

认证配置与请求头使用一致的 Token 语法：32 至 256 字节、仅允许 URL-safe 无填充字符集 [A-Za-z0-9_-]。这与密码学安全随机字节的 Base64URL 无填充编码兼容。该校验只约束格式和输入成本，不能证明 Token 具有足够随机熵；部署仍必须从密码学安全随机源生成至少 256 bit 随机数据。

认证入口拒绝非法字符、Unicode、标点注入和超过 256 字节的 Token。此限制避免对异常超长凭证执行不必要的摘要计算，也避免配置端和请求端采用不同的 Token 语法。不要使用带填充的标准 Base64 或任意口令作为内部 Bearer Token。


### 9.5 服务凭据数量上限

Authenticator 当前采用逐个摘要比较，确保认证结果不因首个匹配位置提前返回而泄漏匹配位置。为限制每次认证的固定工作量，单个 Authenticator 最多接受 128 个 service 凭据；超出上限时必须在启动构造阶段失败，不得截断配置或静默忽略凭据。该上限是平台实现的资源保护边界，不代表建议实际部署配置 128 个调用方；首期仍应按真实服务调用关系配置最小数量的独立凭据。

## 10. ServiceCredential 主体名称约束（2026-11-09）

`ServiceCredential.Name` 会成为认证后 `Principal.Subject`，因此不能把它当作任意显示名称。Authenticator 构造时先去除首尾空白，再要求名称为 1～64 字节；仅允许小写 ASCII 字母、数字、点、下划线和连字符，且首字符必须为小写字母或数字。同一 Authenticator 内名称必须唯一。

不符合格式的名称在启动配置构造阶段直接拒绝。该字段不得承载用户输入、凭证明文或其他敏感信息。对应实现位于 `internal/platform/security/security.go`，边界测试覆盖合法名称、空名称、超长名称、大小写混合、空格、斜线、控制字符和非法前缀。

验收提交：`348b8a692dade915c1d11f35663e57a9777a3422`；Platform Go checks Run [42578653237](https://github.com/Kasign/stock_data_provider/actions/runs/42578653237) 全部通过。


## 11. Web Owner 登录与 Session 安全模型（2026-11-09 当前设计）

本节补充首期 Bearer 认证之后的 Web 登录安全设计，不替换 anonymous/owner/service 身份模型。浏览器登录使用独立 Web Session；Owner Bearer 与 Service Bearer 继续用于可信运维/服务间调用，三者不得互换。本文记录 Platform 公共原语边界，不定义 Server 登录页面、HTTP 路由或业务恢复流程。

### 11.1 身份与凭证隔离
- 首期仍为单一 Owner，不建设注册、多用户、组织、租户或通用 IAM。
- Owner Web 密码只在 Server 端验证；持久化 Argon2id 哈希及算法参数，不保存明文，不记录密码/哈希。
- 浏览器只持有高熵随机 Web Session Cookie；Owner Bearer（`PLATFORM_ADMIN_TOKEN`）和 Service Bearer 永不下发浏览器。
- Web Session 只证明当前 Owner 登录状态；具体路由仍执行显式授权策略。登录状态不能自动覆盖公开路由边界、服务身份或未来领域资源授权。
- Web 登录、Owner Bearer、Service Bearer 使用不同凭证、不同验证路径和不同配置项。禁止复用 Web 密码作为 API Key，禁止把 Owner Bearer 写入 localStorage、Cookie 或前端构建产物。

### 11.2 密码原语与账号生命周期
Platform 提供成熟 Argon2id 实现的封装、参数校验、哈希和验证，不自研密码学算法。候选默认参数为 memory=64 MiB、iterations=3、parallelism=1、salt=16 bytes、output=32 bytes；实现前需在目标部署硬件上做耗时/内存基准，保持登录可用性并限制暴力破解成本。参数必须版本化并随哈希编码保存，未来升级参数时允许验证旧版本并在成功登录后重新哈希。
- 密码验证限制输入长度和请求体大小，避免超大输入消耗资源；不以固定字符拼盘规则替代长度策略。
- 初始密码由 Server 一次性初始化流程从部署 Secret 注入；初始化完成后不得每次启动重置密码，也不得把环境变量作为长期密码数据库。
- Owner 凭据哈希及 credential_version 作为持久化事实保存于 MariaDB；Platform 定义最小存储契约/迁移边界，Server 负责初始化、修改、恢复入口和运维流程。
- 密码修改必须递增 credential_version，使旧 Session 即使仍残留于 Redis 也不能继续通过认证。
- 账号恢复必须是显式的本机/受控运维流程，不能提供匿名密码重置接口；恢复成功后递增 credential_version。

### 11.3 Session Token、Store 与生命周期
- 登录成功后生成至少 256 bit CSPRNG 随机 Token，使用 URL-safe Base64 无填充编码；浏览器只拿到原始 Token，服务端存储 Token 的 SHA-256 摘要，不存明文。
- Platform 定义 SessionStore 接口及不可变 Session 记录；首期 Redis 实现复用现有 `internal/platform/storage.OpenRedis` 创建的 Redis Client，不重复创建连接池。
- Session 记录至少包含 Token 摘要标识、Owner Subject、created_at、last_seen_at、idle_expires_at、absolute_expires_at、credential_version、CSRF Token 摘要和必要轮换元数据；禁止保存 Cookie 明文。
- 空闲期限 12 小时，绝对期限 7 天。有效访问原子刷新 last_seen/idle TTL，但不能延长 absolute_expires_at；过期检查同时校验两种期限。
- 登录建立新 Session，不把匿名/旧 Cookie 会话升级为认证会话；轮换创建新 Token 并使旧 Token 失效；退出撤销当前 Session；密码修改/恢复通过 credential_version 使所有旧 Session 失效。
- Redis 仅保存可丢失的短期 Session。Redis 不可用、超时或原子操作失败时，受保护请求必须 fail closed；不得退回内存 Session、Bearer 或匿名模式。Redis 数据丢失意味着重新登录。
- SessionStore 的创建、读取并刷新、轮换、撤销必须具备明确并发语义；读写/TTL 更新和轮换不能留下可重复使用的旧 Token。Redis Lua/事务实现需有并发与故障测试。
- Server 负责将 Session 解析为可信 Principal，并在每次认证时核对 MariaDB 权威 credential_version；低并发个人系统优先保证撤销正确性，不以减少一次 DB 读取为由弱化版本校验。

### 11.4 Cookie 与 CSRF
Cookie 候选默认值：
- `HttpOnly=true`；HTTPS 部署 `Secure=true`，生产环境不得因代理头缺失静默降级。
- `SameSite=Lax`；不设置宽泛 Domain；Path 限定到 Web/API 实际公共前缀，若同源应用共用根路径则使用 `/` 并明确这是最小可行范围。
- Cookie 生命周期不超过 Session 剩余有效期；滑动续期时同步更新 Max-Age/Expires。
- Cookie 名称由 Server 配置；退出、撤销和过期响应以相同 Name/Path/Domain 属性清除。
- 外部 HTTPS 识别只能信任明确配置的反向代理网络/头部；不能无条件相信任意客户端的 `X-Forwarded-Proto`。

Platform 提供 CSRF Token 生成、摘要绑定和常量时间验证原语；Server 负责安全方法/状态变更方法划分、路由接入和 Origin/Host 校验。
- 每个 Session 绑定独立高熵 CSRF Token；通过受保护的同源引导响应提供给前端，前端仅保存在运行时内存并通过专用 Header 回传，不放入 localStorage。
- 所有 Cookie 认证的状态变更请求（包括密码修改、退出及其他写操作）校验 CSRF；登录入口也校验 Origin/Host，防止 Login CSRF。
- 状态变更请求严格校验配置的公开 Origin；缺失/不匹配默认拒绝。Host 校验防止 Host Header 注入，不能把任意 Host 反射为允许来源。
- GET/HEAD/OPTIONS 不得产生业务状态变更；SameSite 只是纵深防御，不能替代 CSRF Token 和 Origin 校验。

### 11.5 登录限流与安全审计
Platform 提供可注入 RateLimiter 接口和 Redis 原子计数实现；Server 选择登录策略并接入真实来源信息。
- 至少按账号标识和来源 IP 两个维度共同限流；每个维度有时间窗口、失败计数和逐步退避。
- 不对单一 Owner 账号实施永久/长时间硬锁定，避免攻击者通过故意输错密码造成拒绝服务。
- 限流结果包含 retry_after 等稳定字段，不泄露账号是否存在。
- 来源 IP 只接受直接连接地址或来自显式可信代理的转发值；禁止无条件信任 X-Forwarded-For。限流键对 IP/账号做摘要，避免 Redis Key 暴露原始标识。
- Redis 限流原子操作失败时首期 fail closed，不允许无保护登录。
- Platform 提供 SecurityAuditEvent 模型、字段脱敏和 AuditWriter 接口；Server 产生登录成功/失败、退出、Session 轮换/撤销、密码修改/恢复、限流触发和敏感管理操作事件。
- 审计事件至少包含 event_type、subject、result、occurred_at、request_id、source 分类及安全 reason_code；不包含密码、哈希、Session/CSRF Token、Authorization Header 或敏感请求体。
- 不建设全量业务审计平台。审计持久化后端由 Server 注入 Writer；关键安全事件写入失败必须有明确策略和诊断，不能静默丢弃。

### 11.6 明确不做与边界
首期不建设 OAuth/OIDC、注册、多用户/多租户、MFA 管理平台、通用 IAM、动态 Service Key、复杂角色引擎、独立审计查询产品或独立认证微服务。若未来增加第二个实际用户、外部身份提供商或多实例高可用需求，重新评审身份模型、Session 一致性与恢复流程。

### 11.7 验收门禁
Platform 本地验收：
- 密码哈希/验证正确；错误参数、异常长度、格式篡改和未知算法版本安全失败；参数可升级且成功验证后可重新哈希。
- Session Token 使用 CSPRNG；存储只出现摘要；过期、滑动空闲期限、绝对期限、轮换、撤销、并发刷新和 credential_version 失效有测试。
- Redis 错误、超时、数据丢失和原子操作失败均 fail closed；不得出现内存降级。
- CSRF Token 与 Session 绑定，错误/缺失/跨 Session Token 拒绝，比较使用常量时间路径。
- 限流按账号/IP 双维度原子执行，窗口边界、并发竞争、退避和 Redis 故障有测试。
- 审计模型拒绝/脱敏敏感字段，Writer 错误可观测。

Server/部署集成验收：
- 登录、初始化、修改密码、受控恢复、退出、Session 续期及权限矩阵端到端通过。
- Web 不接触 Owner/Service Bearer；Cookie 属性、HTTPS 代理信任、Origin/Host、CSRF、登录限流和安全审计在真实入口验证。
- 直接访问 Server 端口与经 Nginx 访问的认证结果一致；除明确白名单外，不存在绕过认证的路由。
- Redis 不可用时已登录请求和登录请求均拒绝；MariaDB credential_version 不匹配时所有旧 Session 失效。
- 验收分别记录 Platform L1、Server A2、部署 A3，不以包级测试替代端到端验收。
