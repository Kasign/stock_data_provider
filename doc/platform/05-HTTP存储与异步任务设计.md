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
- Worker 数量有上限，执行有总超时和取消 Context；队列根 Context 取消后必须停止接收新任务，入队过程中的数据库操作也必须响应该取消。
- 状态至少包含 queued、running、succeeded、failed、cancelled。
- 保存 task_id、任务类型、请求摘要、幂等键、创建来源、Request ID、开始/结束时间和安全失败原因。
- 相同幂等键和请求摘要返回已有任务；相同键不同请求返回冲突。
- Worker 正常终态写入必须确认恰好更新一条仍处于 running 的任务；panic 恢复时 Run 与 Task 终态必须在同一事务内提交。
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

- `OpenRedis(ctx, options, pingTimeout)` 接收调用方构造的 go-redis Options，统一归一化连接池参数：默认 PoolSize 20、MaxActiveConns 128、MaxIdleConns 20；最大活动连接数是硬上限，不能把 PoolSize 误认为总连接数上限。
- PoolSize/MaxIdleConns/MinIdleConns 必须满足容量关系，最大活动连接数不超过 128；Dial/Read/Write/Pool/连接空闲及生命周期超时拒绝负值，Ping 超时不超过 30 秒。
- Ping 失败自动关闭客户端；成功后生命周期由调用方持有，必须显式 Close。
- `PingRedis` 统一返回安全 Fault。Redis 是缓存/短期辅助状态，不得成为任务、业务事实或审计的唯一持久化来源。

### 9.3 Migration Runner

- `Migration{Version, Description, Statements}` 是领域提供的不可变迁移清单；平台不拥有领域 SQL 和表结构。
- `ApplyMigrations(ctx, db, namespace, migrations)` 必须由服务启动/部署的显式迁移步骤调用；OpenSQL 不自动执行 Migration。
- namespace 限制为小写字母、数字、点、下划线和连字符，最多 64 字符。
- 通过同一专用 SQL Conn 执行 MariaDB/MySQL `GET_LOCK`、Migration 和 `RELEASE_LOCK`，避免数据库连接池切换会话导致 advisory lock 失效。
- 历史表 `platform_schema_migrations` 按 namespace/version 唯一记录 description、SQL SHA-256 checksum、status、失败语句序号、started_at、finished_at 和固定 failure_code。保留旧 applied_at 字段以兼容既有表结构，不再将其作为迁移完成时间。
- Runner 启动时通过 MariaDB `ADD COLUMN IF NOT EXISTS` 补齐新增元数据列，兼容此前创建的历史表；旧记录不伪造 finished_at，只有本次执行的迁移写入准确开始/结束时间。
- Migration 先写入 running 和 started_at，再逐条执行 SQL；成功后更新 applied、finished_at；SQL 失败时尽最大努力写入 failed、failure_statement、finished_at 和固定 failure_code。不得持久化原始 SQL 错误文本。历史中存在 running/failed 时拒绝自动重跑，必须先人工检查部分 DDL 结果并恢复。
- Migration 状态转换必须校验 SQL `RowsAffected()` 恰好为 1。若 SQL 已执行但 `running → applied` 未实际更新一行，返回 `platform.storage.migration_record_failed`，保留人工检查入口，不得报告迁移成功；失败状态记录也必须核实更新行数。
- MariaDB advisory lock 绑定物理连接。`GET_LOCK` 请求报错时，即使客户端无法确认服务端是否已加锁，也必须将连接标记为坏连接并丢弃；`RELEASE_LOCK` 失败或无法确认释放时同样丢弃物理连接，不能仅调用 `sql.Conn.Close()` 将其归还连接池。
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


### 7.7 HTTP/2 连接池隔离审查（2026-11-05）

仓库 go.mod 声明最低 Go 版本为 1.22，Transport 显式启用 ForceAttemptHTTP2。对 Go 1.22 标准库实现的源码审查结论：

- net/http.Transport 的 HTTP/1 连接池 key 包含 proxy、scheme、authority；本 Client 禁用 Proxy，因此连接池按 scheme/目标 authority 区分。
- Go 1.22 bundled HTTP/2 client connection pool 以规范化后的 host:port 作为连接池 key；源码明确将“按证书名称共享不同 host 的连接”列为未实现 TODO。因此该版本实现不会仅凭证书 SAN 将 api.example.com 的 HTTP/2 连接合并给 cdn.example.com。
- 同一 host:port 的 HTTP/2 连接仍会复用；这与同源策略一致。当前策略没有独立 IP allowlist，不存在两个策略对同一 authority 施加不同 IP 范围后误复用的问题。
- go.mod 的 go 1.22 是最低语言/模块版本声明，不代表生产运行时一定是 Go 1.22。升级 Go 运行时后，应重新核对对应版本 net/http 与 bundled HTTP/2 连接池 key；不能把上述源码结论无条件外推到未来版本。
- 不因未经证实的跨 authority HTTP/2 合并而关闭 HTTP/2。若未来加入策略级 IP/出口约束，应采用按策略隔离 Transport/连接池或其他可证明的连接隔离方案，并补充运行时集成测试。


### 7.8 News Provider HTTPDoer 注入契约（2026-11-05）

当前 News Provider Adapter 通过 Application 的窄接口 `HTTPDoer{ Do(*http.Request) (*http.Response, error) }` 接收传输能力。Platform 的 `(*httpclient.BoundDoer)` 结构上满足该接口；组合根可以直接注入，不需要 Platform 导入 News，也不需要 News 依赖 Platform 包。

接入边界：

- 由服务组合根创建一个生命周期受控的 `httpclient.Client`，并在服务退出时调用 `CloseIdleConnections()`。不要在每次 Search 中创建 Client/Transport。
- 对 Bocha、Tavily、Brave、SearXNG 的每个已配置 API endpoint 分别调用 `Client.Bind(DestinationPolicy)`，将返回的 BoundDoer 注入对应 Provider 构造函数。不能把裸 Client、默认 `http.Client` 或一个可任意访问目标的通用 Doer 注入生产 Provider。
- `AllowedHosts` 必须是运维配置/代码审核过的精确 API 主机名，不能从 Provider 响应、搜索结果 URL 或用户查询推导；`AllowedPorts` 首期只开放 443，`AllowHTTP=false`、`AllowRedirects=false`。自托管 SearXNG 必须显式配置准确主机和端口；若确需 HTTP，只能作为明确的内网部署例外单独评审，不能全局打开。
- `Dependency` 使用低基数固定标识，例如 `news.provider.bocha`、`news.provider.tavily`、`news.provider.brave`、`news.provider.searxng`。禁止拼接 source_id、provider_item_id、URL、租户 ID 或其他每请求变化字段；Client 按 Dependency 创建并长期持有 semaphore，动态高基数会造成无界 map 增长。
- Provider 并发上限由账号套餐、配额和业务成本确定，放在 Client 的 `DependencyLimits` 配置中；不要把平台并发上限当作供应商速率限制。更细粒度的 QPS/日配额仍由 News 领域负责。
- 首期 `MaxRetries=0` 是最保守选择。若开启有限重试，必须按 Provider/请求方法逐项评估：Brave/SearXNG GET 可在供应商语义确认后通过 `Idempotent=true` 开启；Bocha/Tavily POST 搜索请求默认 `Idempotent=false`，避免 502/503/504 或传输错误导致重复扣费/配额消耗。429 不由平台自动重试，配额等待属于 Provider 领域策略。
- `MaxResponseBytes` 按实际 Provider 返回规模统一设置并保留上限；默认 2 MiB，不得为绕过超限错误无上限放大。News Adapter 仍负责解析响应结构和字段质量。
- Provider API endpoint allowlist 与新闻原始发布来源授权是两套策略。搜索 Provider 返回的 article URL 不得复用 Provider API 的 BoundDoer；未来 Source Fetch 必须先通过 News 的 Source/Endpoint 授权，再按来源 host/port 建立受限 BoundDoer，并由 News 控制内容用途/保留规则。
- News Provider 当前没有生产组合根或真实构造调用点；因此上述是集成契约，不代表 News 已完成生产注入或真实网络验收。服务组合根应由集成分支统一装配，Platform 不直接依赖/修改 News 领域。

本契约仅说明固定 Provider API 的最小接入，不授予任意来源抓取权限，也不替代真实 Provider 账号、配额和来源授权审核。


### 7.9 Dependency semaphore 有界化（2026-11-05）

并发隔离只为 `Config.DependencyLimits` 中显式配置的依赖创建独立 semaphore。所有未配置的 Dependency 共用一个容量为 `MaxConcurrent` 的 fallback semaphore，不再按请求动态创建 channel。

- semaphore map 在 Client 构造时按配置一次性初始化，运行期间只读；其大小严格等于显式配置的依赖数量。
- 未配置依赖共享 fallback 池，动态依赖名不会导致 map 增长。调用方仍应使用固定低基数 Dependency；fallback 只是资源安全兜底，不代表推荐动态命名。
- 已配置依赖仍拥有独立并发额度，不会被未配置依赖占用；未配置依赖之间共享全局 fallback 容量。
- `DependencyLimits` 最多允许 64 个配置项；配置项数量由启动配置控制，禁止从请求或远端响应动态追加，并在 Client 构造时完成数量与单项校验。
- 不采用 LRU/TTL 淘汰 semaphore：淘汰仍被请求持有的 channel 会创建同名新池，破坏同一依赖的并发上限。

该设计将运行期 semaphore 数量从“请求期间出现过的不同 Dependency 数量”收敛为“启动时配置的 Dependency 数量 + 1”，避免高基数输入导致无界内存增长。


### 7.10 HTTP Client 并发与连接生命周期补充（2026-11-05）

- `MaxConcurrent` 仅是未显式配置 Dependency 的共享 fallback 池容量，不是 Client 全局并发上限。
- `DependencyLimits` 中每个显式依赖拥有独立 semaphore；多个显式依赖的并发总和可以超过 `MaxConcurrent`。如需真正全局并发上限，应另行设计全局调度，不得将现有配置误读为全局限制。
- 依赖槽位覆盖上游请求和响应体缓冲读取。响应体读取完成后，上游 Body 已关闭，返回给调用方的是有界内存 Body；调用方仍应关闭返回 Body，但不会因忘记关闭而占用依赖槽位。
- `CloseIdleConnections` 只关闭空闲连接，不取消活动请求；调用后 Client 仍可继续使用。它不是 Client 的永久 Close，也不替代服务退出时的请求 Context 取消与生命周期管理。
- 慢速响应体必须验证在读取完成前占用依赖槽位；关闭空闲连接必须验证不影响活动请求且后续请求仍可执行。


### 7.11 IANA 特殊用途地址覆盖补充（2026-11-05）

专项复核发现：仅依赖 `IsGlobalUnicast`、`IsPrivate` 等通用判断和原有静态前缀表，不能覆盖 IANA 特殊用途地址注册表中的全部条目。部分特殊用途前缀标记为 Globally Reachable=true，仍属于专用协议/服务地址，不应因可全局路由就自动作为普通 Provider 目标放行。

本轮将当前 IANA IPv4/IPv6 Special-Purpose Address Registry 中原前缀表遗漏的条目补入拒绝列表，包括 IPv4 的 AS112/AMT 前缀，以及 IPv6 的 Dummy Prefix、Benchmarking、AMT、AS112、ORCHIDv2、协议实体标签、SRv6 SID 和 AS112 服务前缀。现有较大拒绝前缀（如 `2001::/23`）已覆盖的子网不重复添加。

- `isPublicIP` 的语义是“允许作为普通公网 HTTP 目标”，不是简单判断是否可全局路由。
- 新增地址回归用例应同时覆盖 IPv4/IPv6 特殊用途条目，避免未来维护时只补私网段。
- IANA 注册表会更新；新增特殊用途分配或前缀变化时，必须重新核对地址表和测试。生产网络出口仍应叠加部署层 egress 防火墙，代码内静态列表不是唯一安全边界。

参考：IANA [IPv4 Special-Purpose Address Registry](https://www.iana.org/assignments/iana-ipv4-special-registry)（Last Updated 2025-10-09）与 [IPv6 Special-Purpose Address Registry](https://www.iana.org/assignments/iana-ipv6-special-registry)（Last Updated 2025-10-09）。


### 7.12 压缩响应与响应体上限（2026-11-05）

`MaxResponseBytes` 必须约束 HTTP Client 实际交付给调用方的响应体字节数，而不只是网络上传输的压缩字节数。Go Transport 在调用方未指定 `Accept-Encoding` 时会自行协商 gzip，并在返回响应体前透明解压；此时平台对解压后的数据执行有界读取。

- 调用方不得自行设置 `Accept-Encoding`。Client 对该 Header 名称执行大小写不敏感检查，发现任何显式设置均拒绝请求，避免显式 gzip 让 Transport 跳过透明解压，导致大小限制只覆盖压缩数据。
- Client 保留 Transport 默认压缩协商行为；响应体缓冲读取限制作用于 Transport 交付的字节流。gzip 解压后的超限内容必须返回 `platform.httpclient.response_too_large`。
- 不要将 `Content-Length` 当作唯一大小依据；压缩响应、分块传输和缺失长度都必须通过实际读取字节数限制。
- 若未来需要支持调用方控制压缩算法，必须引入受控的解压层，并对解压后字节数、解压 CPU/时间和嵌套编码制定上限；不能简单放开该 Header。


### 7.13 PLAT-008 HTTP Client 最终代码审查记录（2026-11-05）

审查基线：`f019223e409072da71577c51b1312e1a8ff9f030`。

本轮对 `internal/platform/httpclient`、`BoundDoer`、地址策略、重试/并发控制和响应体处理进行静态复核，并结合 Platform Go checks 专项 CI 结果确认包级验证。

**代码审查结论：**

- 目标 URL 在请求执行前校验精确 Host/Port、Scheme、UserInfo 和自定义 Host；DNS 解析结果必须全部通过 IP 策略校验，混合公网/非公网答案整体拒绝。
- 新建连接时由受控 DialContext 直接连接已校验 IP，不再把原始域名交给第二次系统解析；DNS 校验语义是“每次新建连接”，不是“每个 HTTP 请求”。连接复用是预期行为，当前策略没有按依赖区分的 IP allowlist。
- 禁用环境代理；HTTPS 最低 TLS 1.2；HTTP/2 连接池按当前 Go 1.22 标准库实现审查。生产 Go 运行时版本仍须由部署环境确认，升级运行时后重新审查 Transport/HTTP2 连接池行为。
- 重定向默认关闭；启用时限制同源、禁止 HTTPS 降级，并限制跳数与循环。跨主机跳转即使目标也在 allowlist 中仍拒绝，避免 Authorization、Cookie 和 Provider 自定义凭据跨 Origin 传播。
- BoundDoer 固定持有私有 DestinationPolicy；构造时复制 Host/Port 切片，调用方无法通过修改原始切片扩大目标范围。Provider 应只接收 BoundDoer，不注入裸 Client。
- 重试要求调用方显式声明幂等，最多 2 次；仅处理受控传输错误及 502/503/504，不自动处理 429。重试退避期间释放依赖槽位，结束后重新竞争。
- 响应体读取使用实际字节数限制；显式 Accept-Encoding 被拒绝，Transport 自动 gzip 解压后的字节流仍受 MaxResponseBytes 限制。响应体读取完成、失败、超限或取消均关闭上游 Body 并释放槽位。
- Dependency semaphore 仅由启动配置创建；未配置依赖共用 fallback 池，配置项最多 64 个，运行期不会因动态依赖名扩张 map。
- IANA 特殊用途地址拒绝列表覆盖当前核对的 IPv4/IPv6 注册条目；策略刻意保守，对部分专用协议地址和注册表父网段采取整体拒绝。注册表更新时需要重新核对，部署层仍必须配置 egress 防火墙。

**专项 CI 证据：**

- 提交：`f019223e409072da71577c51b1312e1a8ff9f030`
- Actions Run：`42025170211`
- gofmt：通过
- `go test ./internal/platform/...`：通过
- `go test -race ./internal/platform/...`：通过
- `go vet ./internal/platform/...`：通过

**仍未完成的生产接入验收：**

- 生产环境 DNS、路由和出口防火墙规则尚未进行真实部署验证。
- News Provider 尚无生产组合根注入证据；各 endpoint 的 BoundDoer 装配、真实凭证、配额与服务退出生命周期仍由集成分支验收。
- 本次 CI 是平台包级专项验证，不代表整个仓库、其他服务或生产环境验收通过。

结论：PLAT-008 平台 HTTP Client 原语达到当前代码与专项 CI 验收要求，可以进入领域组合根接入阶段；生产抓取开放仍受上述部署与集成门禁约束。


### 9.7 PLAT-009 Redis 连接上限与 Migration 时间元数据修复（2026-11-05）

- Redis 的 `PoolSize` 仅表示基础池容量，不能作为活动连接总上限；`MaxActiveConns` 才是硬上限。平台默认设置 PoolSize=20、MaxActiveConns=128、MaxIdleConns=20，并验证 PoolSize <= MaxActiveConns、MinIdleConns <= MaxIdleConns <= MaxActiveConns。
- Redis 池容量和连接超时参数拒绝非法负值，MaxActiveConns 不允许超过 128；这保证 go-redis 运行时的连接总量受到显式限制。
- Migration 历史新增 started_at、finished_at、failure_code；保留旧 applied_at 兼容既有历史表。通过 MariaDB `ADD COLUMN IF NOT EXISTS` 补齐旧表字段，不为旧记录推断或伪造完成时间。
- 新迁移开始时写 started_at，成功/失败结束时写 finished_at；失败码使用固定 `platform.storage.migration_apply_failed`，只存失败语句序号和稳定错误码，不保存 SQL Driver 原始错误。
- fake Driver 测试覆盖池配置边界、迁移开始/完成时间、失败元数据和历史表列升级。真实 MariaDB/Redis 运行时集成仍待部署验收。


### 9.8 PLAT-009 修复提交与专项 CI 证据

最终代码提交：`fbbb10681194286ccf52405baa66331d045655d2`。Platform Go checks：[`42026901950`](https://github.com/Kasign/stock_data_provider/actions/runs/42026901950)，最终结论 success；gofmt、`go test ./internal/platform/...`、`go test -race ./internal/platform/...`、`go vet ./internal/platform/...` 均通过。之前提交 `b7ac6bef` 的初次 CI 曾因变量重复声明失败，已修复；不能把该失败 SHA 记为通过。
