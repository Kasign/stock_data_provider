# HTTP、存储与异步任务设计

## 1. HTTP Client

统一最基本的外部请求治理，避免 Provider 慢响应、异常大响应、重试放大和不可信目标地址导致的 SSRF。

必须具备：
- 连接复用，不为每次请求创建 Transport；
- 连接、响应头和总请求 Deadline；
- 最大响应体字节数；
- 明确的有限重试次数和退避；
- 非幂等请求默认不重试；
- 记录依赖名、操作、耗时、attempt 和安全错误码；
- context 取消后及时终止请求。

首期不建设通用熔断平台、自适应限流或复杂 Retry Budget 服务。对 Provider 的配额和领域限流由 Data Adapter 管理。

## 2. MariaDB

MariaDB 是业务事实、任务状态、凭证元数据和必要审计记录的持久化来源。

平台只统一连接池参数校验、Ping、Close 和生命周期。领域 Repository、SQL、表结构和保留策略由领域拥有。

- 所有 DB 操作接受 context。
- 事务尽量短。
- 关键事实写入失败不能静默降级到 Redis 或内存。
- 连接池参数根据当前单实例负载设置，避免盲目扩大连接数。
- 连接池耗尽、慢查询和连接失败应能通过日志/诊断区分。

## 3. Redis

Redis 仅承担缓存和可丢失的短期辅助状态。所有缓存有 TTL 和失效策略。

- 不将任务最终状态、凭证元数据、审计或业务事实只存 Redis。
- 首期不依赖 Redis 实现分布式锁、分布式任务调度或复杂协调。
- Redis 不可用时，缓存可降级；依赖 Redis 的非关键功能应明确关闭或降级。
- 只有确有共享并发控制需求时，才增加 Redis 协调机制。

## 4. Migration

领域拥有自己的 Migration SQL；平台提供统一命名、版本记录和执行约定。

- Migration 使用唯一 namespace/version。
- 执行入口由服务端集成分支装配，生产环境显式执行。
- 执行记录包含版本、开始时间、结束时间和安全失败摘要。
- 考虑 MariaDB DDL 的隐式提交，Migration 必须可检测当前状态、可重入或提供前向修复。
- 首期按单实例/单执行者设计，不建设多集群迁移编排。
- 不允许平台分支与集成分支同时修改总注册入口。

## 5. 长任务

只对耗时较长、需要离开 HTTP 请求执行的研究/分析工作使用持久化任务。普通快速查询不进入任务队列。

首期约束：
- MariaDB 保存任务状态和幂等记录。
- Worker 数量有上限，执行有总超时和取消 Context。
- 状态至少包含 queued、running、succeeded、failed、cancelled。
- 保存 task_id、任务类型、请求摘要、幂等键、创建来源、Request ID、开始/结束时间和安全失败原因。
- 相同幂等键和请求摘要返回已有任务；相同键不同请求返回冲突。
- 启动时明确处理遗留 running 任务：可安全重试的重新排队；不安全的标记为需要人工处理/失败，不盲目重复执行。
- 每个任务类型明确是否可重入及副作用幂等方式。

首期按单实例执行，不建设多实例租约、fencing token、通用工作流和 Outbox。若部署拓扑变为多实例，PLAT-010 必须重新评审任务领取和恢复方案。


### 5.1 任务能力所有权与当前实现边界

平台只拥有经多个领域验证确实可复用的技术原语，不拥有 Research/Analysis 的任务语义、Run/Stage 事件、领域请求 DTO 或业务 Runner。

当前 `internal/platform/taskqueue/queue.go` 直接依赖 `internal/analysis/domain`，其 Runner、RunRequest、RunResult、StageEvent、`analysis_tasks` 和 `analysis_stage_events` 均属于分析领域。该实现具有领域耦合，不能作为平台通用 Task Queue 的完成证据，也不得继续向其中加入 News、Notification 或其他领域类型。

合并后的 Research 设计将 `go-core` 明确为独立 Go Module，并规定首期不引入分布式队列；只有跨进程 Worker、持久租约或恢复需求经验证后才接入平台 Task 能力。因此：

- Research/Analysis 分支负责确定 Analysis Run/Task/Stage 的领域模型、状态机、事件和持久化所有权；领域 Worker 与 Runner 也由该分支维护。
- Platform 分支不迁移或重写领域队列，不让 `internal/platform` 依赖 `internal/analysis`、`go-core/internal` 或其他领域包。
- Platform 只有在至少两个实际调用方出现相同需求后，才抽取最小的存储/执行原语；接口不得包含领域 DTO、领域状态枚举或领域表名。
- 首期单实例下优先使用领域自己的 MariaDB 持久化与有界 Worker。未证明多实例竞争前，不引入租约、fencing、分布式锁或通用工作流。
- 旧 `internal/platform/taskqueue` 暂列为待归属/迁移的历史分析实现；在 Research 分支确认新方案和集成切换前，不删除、不搬迁、不扩大其使用范围。

PLAT-010 的退出条件不只是队列代码可运行，还必须先完成领域归属评审、单实例恢复语义和副作用重放策略确认。若最终没有跨领域公共调用方，PLAT-010 应收敛为平台契约与领域接入约定，不为完成任务而制造通用队列包。


## 6. 性能与可靠性基线

- 所有网络和数据库操作必须有超时/取消边界。
- 并发上限按 Provider 配额、DB 连接池和机器资源设置，不用无限 goroutine。
- 缓存必须有 TTL，且缓存失效不能破坏事实数据。
- 大响应和大任务输入设置大小上限。
- 性能优化以 p95/p99、资源占用、连接池等待和 Provider 延迟等实际测量为依据，不提前建设分布式组件。


## 7. 不可信 URL 与 SSRF 防护（强制安全契约）

HTTP Client 必须区分固定 Provider API Endpoint 与不可信来源 URL。新闻 Source Fetcher 只接受已登记的 source_id/endpoint_id 解析出的 URL，不接受用户直接提交任意 URL；来源授权、允许域名和用途策略由 News 持有，底层网络目标校验由平台 HTTP Client 执行。两层校验缺一不可。

### 7.1 URL 与目标地址校验

- 仅允许显式配置的 HTTP/HTTPS scheme；默认拒绝其他 scheme、userinfo、空 host、IPv6 zone 和非允许端口。
- Source Fetch 场景必须执行来源 host/port allowlist 校验；不能把任意公网地址等同于已授权来源。
- 拒绝 loopback、private、link-local、unspecified、multicast、保留/特殊用途地址、IPv4-mapped IPv6，以及策略明确禁止的地址段。
- DNS 解析结果必须完整检查；同一 hostname 返回的地址中只要存在禁止地址，默认整体拒绝，不能只挑选一个公网地址继续。
- IP literal 与 DNS 解析结果执行相同地址策略。
- 不信任 X-Forwarded-*、客户端身份 Header 或 URL 文本中的主机声明作为安全依据。

### 7.2 DNS rebinding 防护

不能采用“先 LookupIP 校验，随后让默认 Transport 再解析一次 hostname”的实现。

必须由受控 DialContext 完成解析、地址校验和连接：
1. 对目标 hostname 执行 DNS 解析；
2. 检查全部 A/AAAA 结果及特殊地址；
3. 选择通过策略的 IP；
4. 使用该 IP literal 直接建立 TCP 连接，不再对 hostname 二次解析；
5. TLS ServerName 和 HTTP Host 仍使用原始 hostname，保持证书校验和虚拟主机语义。

即校验通过的 IP 必须与实际连接 IP 绑定。每次新建连接都重新执行解析和校验；不能依赖不受控的长 TTL DNS 结果作为授权依据。

### 7.3 重定向逐跳校验

- 默认禁止自动重定向。
- 需要重定向的 Source Fetch 必须显式启用有限跳数策略（首期建议最多 3 跳）。
- 每一跳重新校验 scheme、userinfo、host、port、来源 allowlist 和 DNS/IP；最终连接仍通过上述受控 DialContext。
- 不允许 HTTPS 降级到 HTTP；首期只允许同源重定向（scheme、host、port 均一致），防止 Authorization、Cookie 或 Provider 自定义凭据随跳转发送到其他主机。即使目标 host 也在 allowlist 中，跨 host 跳转仍拒绝；未来确有需求时，必须单独设计凭据剥离和目标授权策略。
- 重定向循环、超限、目标不合规时立即失败，不跟随。
- 不能通过自定义 CheckRedirect 校验后，再让默认 Dialer 独立解析目标地址。

### 7.4 Proxy 与并发边界

- 不可信 URL 抓取默认禁用环境代理和隐式 ProxyFromEnvironment；如将来使用受控代理，必须由代理端实施等价的 DNS/IP/重定向防护，并有明确配置。
- HTTP Client 按逻辑 dependency 提供有限并发上限，避免单一来源拖垮其他调用；Provider 配额、账号级速率限制和业务成本仍由 News/Data Adapter 管理。
- DNS、连接、TLS、响应头、总请求均受 context/deadline 约束；响应体和重定向次数有上限。

### 7.5 最小安全测试矩阵

覆盖 IPv4/IPv6 私网、loopback、link-local、保留地址、IPv4-mapped IPv6、混合公网/私网 DNS 结果、DNS 结果变化/rebinding、重定向到私网/其他 host/HTTP 降级、userinfo、非标准端口、代理环境变量、重定向循环和连接超时。

固定 Provider API 也必须使用明确的 endpoint 策略；Source Fetch 的来源授权策略不能被通用 HTTP Client 代替。未完成上述安全测试前，不开放任意来源真实抓取。


## 8. PLAT-008 首批代码实现状态（2026-11-05）

首批实现位于 `internal/platform/httpclient`，包含目的地址策略、精确 Host/Port allowlist、受控 DNS 解析与 IP literal Dial、重定向逐跳检查、禁用环境代理、请求/响应上限和 dependency 并发控制。重试仅在调用方显式声明幂等且配置了 MaxRetries 时启用，最多 2 次。

专项 CI 已通过 gofmt、go test、go test -race 和 go vet；已增加基于本地 HTTP Server 的 Transport 连接路径测试，覆盖受控 IP Dial、Host 保留、重定向策略、DNS 结果变化、响应体上限和幂等重试。真实生产网络出口与完整 SSRF 攻击矩阵仍待验收，未完成前不得开放任意来源抓取。


## 9. PLAT-009 平台存储原语首批实现

包位置：`internal/platform/storage`。

### 9.1 SQL 生命周期

- `SQLConfig` 统一 MaxOpenConns、MaxIdleConns、ConnMaxLifetime、ConnMaxIdleTime 和 PingTimeout。
- `DefaultSQLConfig()` 提供单实例保守默认值：最大连接 20、最大空闲 5、连接最大生命周期 30 分钟、最大空闲时间 5 分钟、Ping 5 秒。
- 最大连接数上限 128；空闲连接不能大于最大连接；负数参数拒绝。
- `OpenSQL(ctx, connector, config)` 不拼接 DSN、不读取环境变量；数据库驱动和凭证配置由调用方负责。Ping 失败自动关闭 DB。
- `PingSQL` 统一验证依赖可用性，Fault 对外使用安全摘要。

### 9.2 Redis 生命周期

- `OpenRedis(ctx, options, pingTimeout)` 接收调用方构造的 go-redis Options，校验地址、DB、连接池规模和超时参数，启动前 Ping。
- Ping 失败自动关闭客户端；成功后生命周期由调用方持有，必须显式 Close。
- `PingRedis` 统一返回安全 Fault。Redis 是缓存/短期辅助状态，不得成为任务、业务事实或审计的唯一持久化来源。

### 9.3 Migration Runner

- `Migration{Version, Description, Statements}` 是领域提供的不可变迁移清单；平台不拥有领域 SQL 和表结构。
- `ApplyMigrations(ctx, db, namespace, migrations)` 必须由服务启动/部署的显式迁移步骤调用；OpenSQL 不自动执行 Migration。
- namespace 限制为小写字母、数字、点、下划线和连字符，最多 64 字符。
- 通过同一专用 SQL Conn 执行 MariaDB/MySQL `GET_LOCK`、Migration 和 `RELEASE_LOCK`，避免数据库连接池切换会话导致 advisory lock 失效。
- 历史表 `platform_schema_migrations` 按 namespace/version 唯一记录 description、SQL SHA-256 checksum、status、失败语句序号和 applied_at。
- Migration 先写入 running，再逐条执行 SQL，成功后更新 applied；SQL 失败时尽最大努力写入 failed 和失败语句序号。历史中存在 running/failed 时拒绝自动重跑，必须先人工检查部分 DDL 结果并恢复。
- 执行前验证完整 Migration 清单；拒绝重复版本、空 SQL、无效 namespace、历史 checksum 漂移、数据库存在未登记版本和历史版本缺口。
- 每条 SQL 单独执行；MariaDB DDL 可能隐式提交，因此不能假设整个 Migration 可事务回滚。失败记录写入本身也可能因数据库不可用而失败，此时错误仍保持安全摘要，但需要通过数据库侧检查确认实际执行状态。修复应采用新的前向 Migration，不得修改已发布版本。

### 9.4 当前验收边界

最终验证提交 `86429e008d53f3208e0a414583cbcfe9c5635f8d` 对应 Platform Go checks 已通过 gofmt、平台包单测、race test 和 vet，CI：[42002814937](https://github.com/Kasign/stock_data_provider/actions/runs/42002814937)。

测试使用 fake SQL Connector/Driver 和注入 Redis Dialer，不依赖外部数据库服务。真实 MariaDB/Redis 集成、故障恢复和现有分析 Store 的平台化接入尚未验收；服务入口和领域 Migration 总注册仍由集成分支负责。


### 9.5 Migration 顺序保护补充

执行前不仅检查未知版本和 checksum 漂移，还检查已应用版本必须构成当前注册清单的有序前缀。若数据库记录了较新版本，但更早版本缺失，直接拒绝执行，防止旧 Migration 被补跑到新 Schema 上。

验证提交：`b63c2ca64d165f3fb207070190821da76a02dedb`；CI：[41998916359](https://github.com/Kasign/stock_data_provider/actions/runs/41998916359)，全部专项检查通过。


### 9.6 Migration 失败人工恢复步骤

Migration 状态为 `running` 或 `failed` 时，Runner 不会自动重跑。处理步骤：

1. 停止该服务的自动启动迁移流程，避免持续重试；保留数据库备份和错误日志中的 namespace/version/失败语句序号。
2. 查询 `platform_schema_migrations` 对应 namespace/version，核对 checksum、status、failure_statement；checksum 必须与当前发布的 Migration 清单一致。
3. 按 Migration 原始语句逐项检查实际 Schema。MariaDB DDL 可能已经部分提交，不能仅凭失败语句序号推断此前语句都未执行。
4. 如果决定回滚，先人工恢复到该版本执行前的 Schema 状态，确认无该 Migration 的残留对象后，才允许删除对应的 failed/running 历史行，再重新执行同一版本。
5. 如果决定前向修复，人工补齐并验证该 Migration 的全部预期 Schema 变更后，将历史行状态更新为 applied；后续结构调整必须新增更高版本 Migration，不能改写原版本 SQL。
6. 记录恢复人、时间、数据库实例、namespace/version、执行的修复 SQL 和验证结果，再恢复服务启动流程。

不得在未检查真实 Schema 的情况下直接删除失败记录或将状态改为 applied。若历史行无法写入（例如数据库在失败时不可用），先检查 Schema，再决定补建历史状态；不能假设该版本完全未执行。


### 7.6 同源重定向与凭据隔离补充（2026-11-05）

当前 HTTP Client 的 CheckRedirect 采用同源限制：scheme、规范化 hostname 和有效 port 必须与上一跳一致。跨主机跳转即使目标在 AllowedHosts 中也拒绝，避免标准库对同主域名子域跳转保留 Authorization 等敏感 Header 时造成凭据扩散。若未来需要跨主机 CDN 跳转，必须明确哪些凭据可剥离、哪些自定义凭据不允许转发，并以独立策略和集成测试证明安全性；不能仅依赖 Host allowlist。


## PLAT-008 并发槽位与重试退避补充（2026-11-05）

- 依赖级并发槽位只覆盖实际请求执行和响应体读取，不得在指数/固定退避等待期间占用槽位。
- 收到可重试状态或可重试传输错误后，先关闭本次响应（如有）并释放依赖槽位，再等待退避；等待结束后必须重新竞争槽位，并继续遵守原请求 Context/总 Deadline。
- 退避期间其他同依赖请求应能使用空出的并发容量；重新竞争失败或 Context 取消时不得遗留槽位。
- DNS/IP 校验发生在新建连接的 DialContext 阶段。Transport 复用已建立连接时不会为每个 HTTP 请求重新解析 DNS；安全验收应区分“每次新建连接校验”与“每次请求解析”，不得混淆。
- 当前 DestinationPolicy 的目标约束为精确 Host/Port 与公共 IP 校验，没有独立的目标 IP allowlist。若未来增加按 Provider/租户区分的 IP 范围策略，必须重新评审连接池隔离，不能直接共享跨策略连接池。
- HTTP/2 连接合并是否可能发生，需通过目标 Go 版本与 Transport 行为专项验证；当前不把未经验证的连接合并当作已发生漏洞。
