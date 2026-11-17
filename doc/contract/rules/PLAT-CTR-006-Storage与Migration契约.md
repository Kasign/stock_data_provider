# PLAT-CTR-006 Storage Connection 与 Migration Runner 契约

- 契约编号：`PLAT-CTR-006`
- 契约版本：`1.0.0`
- 状态：`freeze-ready`
- Platform 责任：SQL/Redis 连接创建、连接池参数校验、Ping、连接生命周期辅助、namespace-scoped SQL Migration Runner
- 领域责任：Repository、领域 Schema、Migration SQL 内容、Migration 版本定义和领域数据保留策略
- Server 责任：共享资源构造与关闭、领域 Migration 注册、启动阶段显式执行和应用 Ready 判定
- 唯一权威 API：`internal/platform/storage`
- 设计依据：`doc/platform/05-HTTP存储与异步任务设计.md` 第2～4节、`doc/platform/08-公共平台契约规范.md` 第9节、`doc/platform/16-公共平台剩余开发任务与跨模块接入责任清单.md`
- 代码核对基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`

## 1. 目标与边界

Platform 提供基础连接生命周期与显式 Migration 执行原语，不拥有领域 Repository 或数据库 Schema。

- MariaDB 是业务事实、任务状态、凭证元数据和必要审计记录的持久化事实源。
- Redis 用于缓存和可丢失的短期辅助状态；不得成为业务事实、任务最终状态、凭证元数据或必要审计的唯一存储。
- 领域模块拥有自己的 Repository、SQL、表结构、Migration 清单和保留策略。
- Server 是唯一组合根，负责创建共享 SQL/Redis 资源、注入领域 Repository、注册 Migration 并控制启动/关闭顺序。
- Platform 不扫描领域包、不自动发现 Migration、不维护全局领域 Migration 注册表，也不替 Server 决定应用是否 Ready。

本契约不定义通用 Repository、ORM、Unit of Work、跨数据库抽象、分布式 Migration 编排或 Redis 分布式任务队列。

## 2. 公共 Go API

```go
type SQLConfig struct {
    MaxOpenConns    int
    MaxIdleConns    int
    ConnMaxLifetime time.Duration
    ConnMaxIdleTime time.Duration
    PingTimeout     time.Duration
}

func DefaultSQLConfig() SQLConfig
func OpenSQL(ctx context.Context, connector driver.Connector, config SQLConfig) (*sql.DB, error)
func PingSQL(ctx context.Context, db *sql.DB) error

func OpenRedis(ctx context.Context, options redis.Options, pingTimeout time.Duration) (*redis.Client, error)
func PingRedis(ctx context.Context, client *redis.Client) error

type Migration struct {
    Version     uint64
    Description string
    Statements  []string
}

func ApplyMigrations(ctx context.Context, db *sql.DB, namespace string, migrations []Migration) error
```

返回的 `*sql.DB` 和 `*redis.Client` 由调用方拥有；Platform 不在后台替调用方关闭连接。调用方必须在服务退出时调用各自标准 Close。Ping 函数只验证可用性，不转移或取得资源所有权。

## 3. SQL 连接契约

### 3.1 SQLConfig 默认值和边界

| 字段 | 默认值 | 约束 |
|---|---:|---|
| MaxOpenConns | 20 | 1～128；0 使用默认值 |
| MaxIdleConns | 5 | 0 使用默认值；不得大于 MaxOpenConns |
| ConnMaxLifetime | 30 分钟 | 0 使用默认值；负值拒绝 |
| ConnMaxIdleTime | 5 分钟 | 0 使用默认值；负值拒绝 |
| PingTimeout | 5 秒 | 0 使用默认值；最大 30 秒 |

- `OpenSQL` 要求非 nil Context 和 driver.Connector；不接受 Platform 自行猜测 DSN、凭据或业务数据库名。
- 连接池参数校验通过后创建 `*sql.DB`，并在 PingTimeout 内执行 Ping。
- Ping 成功后才将 DB 返回给调用方；Ping 失败时必须关闭刚创建的 DB 并返回错误。
- `PingSQL` 使用调用方 Context；不可用时返回 Platform Fault。
- 所有 SQL 操作必须由领域 Repository/调用方使用 Context；Platform 不替领域执行普通 CRUD。

### 3.2 SQL 资源所有权

- Server 组合根负责创建共享 SQL DB 并注入各领域 Repository。
- 领域 Repository 不得各自重复创建连接池，也不得绕过 Server 的连接配置创建第二套 DB。
- 资源所有者负责关闭 DB；关闭前应停止新请求和 Worker，并先完成依赖该 DB 的任务退出。
- Platform 不提供应用级 Ready 状态；OpenSQL 的 Ping 仅代表连接建立时的可达性，运行期间健康状态由 Server/领域 Diagnostics 组合。

## 4. Redis 连接契约

- `OpenRedis` 要求非 nil Context、有效 Redis Options 和有效 PingTimeout；Addr 必须非空，DB/连接池/超时参数不得为负，连接池上限不得超过 128。
- 默认 PoolSize 为 20；默认 MaxActiveConns 为 128；默认 MaxIdleConns 为 PoolSize。
- 必须满足 PoolSize ≤ MaxActiveConns、MaxIdleConns ≤ MaxActiveConns、MinIdleConns ≤ MaxIdleConns。
- PingTimeout 为 0 时使用 5 秒默认值，最大 30 秒。
- 创建 Client 后必须在 PingTimeout 内执行 Ping；成功才返回 Client；Ping 失败时关闭刚创建的 Client。
- `PingRedis` 使用调用方 Context；不可用时返回 Platform Fault。
- 返回的 Redis Client 由 Server 组合根拥有并关闭；SessionStore、RateLimiter、领域缓存等消费者复用注入的 Client，不得自行创建第二套连接池。
- Redis 故障时不得把持久事实静默降级写入进程内存；缓存可由领域按自己的契约降级，依赖 Redis 的安全认证/会话/限流能力必须按其安全契约 fail closed。

## 5. Migration 契约

### 5.1 Migration 输入模型

`Migration` 是领域提供的不可变有序 Schema 变更单元：

| 字段 | 约束 | 语义 |
|---|---|---|
| Version | uint64，必须大于 0，同 namespace 内唯一 | 严格排序和版本身份 |
| Description | 非空，最多 255 个 Unicode 字符 | 面向维护者的安全摘要，不放凭据或原始错误 |
| Statements | 非空数组；每条 SQL 去除空白后非空，UTF-8 字节数不超过 1 MiB | 按数组顺序逐条执行 |

- 单次调用 Migration 数量为 1～1000。
- Runner 复制输入切片后按 Version 升序执行；调用方不得在执行期间并发修改输入。
- 已发布 Migration 的 Version、Description、Statements 均不可修改；修复只能新增更高 Version 的前向 Migration。
- SQL checksum 为按原顺序将每条 Statement 的原始字节写入 SHA-256，并在每条后追加一个 NUL 分隔字节所得的十六进制摘要。Description 不参与 SQL checksum，但仍属于不可变发布元数据。

### 5.2 Namespace

- namespace 必须为 1～64 字节。
- 只允许小写 ASCII 字母、数字、点、下划线和连字符；不允许首尾空白或其他字符。
- namespace 是 Migration 历史和互斥锁的隔离边界；不同领域必须使用独立且稳定的 namespace。
- Namespace 命名由领域定义并由 Server 注册；Platform 负责校验，不替领域分配名称。

### 5.3 ApplyMigrations 执行语义

- Migration 必须由 Server 启动/部署流程显式调用；OpenSQL 不自动执行 Migration。
- Runner 对单一 namespace 串行执行；通过 MariaDB advisory lock 互斥，锁等待上限 10 秒。
- advisory lock、Migration SQL 和 RELEASE_LOCK 必须使用同一专用 SQL Conn，确保锁绑定同一物理连接。
- Migration History 使用共享表 `platform_schema_migrations`，以 namespace/version 唯一标识，记录 description、SQL checksum、status、failure_statement、started_at、finished_at、failure_code；保留历史 applied_at 字段用于兼容，不把它解释为当前执行完成时间。
- Runner 可为旧 History 表补充 started_at、finished_at、failure_code 列；不得伪造旧记录的 finished_at。
- 每个 Migration 先写入 running 与 started_at，再按 Statements 顺序逐条执行；全部成功后写入 applied 与 finished_at。
- DDL 可能隐式提交；不得假设整个 Migration 可事务回滚，也不得在失败后自动重跑。
- 失败时尽最大努力记录 failed、失败语句序号、固定 failure_code 和 finished_at；不持久化原始 SQL 错误文本。
- 成功/失败状态更新必须验证 RowsAffected 恰好为 1；否则返回 Migration History 错误，不得报告成功。
- Migration 执行中断、状态为 running/failed、History 写入失败或执行结果无法确认时，后续调用必须拒绝自动继续，要求先人工核对真实 Schema 并恢复。
- 修复采用人工确认后的回滚恢复或前向修复；已发布 SQL 不得修改，前向修复必须增加新版本。

### 5.4 History 一致性与失败保护

Runner 执行前必须验证完整清单并拒绝：
- 非法 namespace、空 Migration 集合、超过数量上限；
- Version 为 0、重复 Version、空 Description、空 Statements；
- 空白 SQL 或超过单条 1 MiB 的 SQL；
- 数据库中存在当前清单未登记的已应用版本；
- 已应用版本的 SQL checksum 与当前清单不一致；
- 已应用版本不构成当前注册清单按 Version 排序后的有序前缀；
- 历史中存在 running/failed 等未完成状态。

GET_LOCK 报错、返回 NULL 或无法确认锁归属时，必须丢弃该物理连接，不能将可能仍持有锁的连接归还连接池。RELEASE_LOCK 失败或无法确认释放时同样丢弃物理连接，并将释放失败作为调用失败返回。

### 5.5 Migration 失败恢复

- Runner 不自动重跑 running/failed Migration。
- 恢复前必须停止自动 Migration 启动流程、备份数据库并检查 namespace/version/checksum/status/failure_statement。
- 必须逐项核对真实 Schema；MariaDB DDL 可能部分提交，不能仅凭失败语句序号推断数据库状态。
- 只有在人工确认 Schema 已回滚到执行前状态后，才可清理失败记录并重新执行；若选择前向恢复，先人工补齐并验证预期 Schema，再将历史记录校准为 applied，后续结构变化另增版本。
- 若失败记录本身因数据库不可用而未写入，必须先检查 Schema 再决定如何补齐 History；不得假定 Migration 完全未执行。

## 6. Platform Fault 与错误边界

主要稳定错误语义包括：
- 连接配置非法：`platform.storage.invalid_sql_config`、`platform.storage.invalid_redis_config`。
- SQL/Redis 未配置或不可达：`platform.storage.invalid_sql`、`platform.storage.invalid_redis`、`platform.storage.sql_unavailable`、`platform.storage.redis_unavailable`。
- Migration 目标/定义非法：`platform.storage.invalid_migration_target`、`platform.storage.invalid_migrations`。
- Migration 连接/锁/History/执行失败：`platform.storage.migration_connection_failed`、`platform.storage.migration_lock_failed`、`platform.storage.migration_lock_busy`、`platform.storage.migration_history_failed`、`platform.storage.migration_apply_failed`、`platform.storage.migration_record_failed`、`platform.storage.migration_lock_release_failed`。
- 存在未完成 Migration 时返回 Conflict：`platform.storage.migration_requires_recovery`。
- SafeMessage 必须为固定安全摘要；原始 DSN、密码、Redis URL、SQL 内容和原始数据库错误文本不得进入公开错误消息或日志字段。Cause 仅供进程内诊断使用。

## 7. Ready、Shutdown 与组合根职责

- `OpenSQL`/`OpenRedis` 只有在初始 Ping 成功后才返回资源；这不是进程整体 Ready。
- `ApplyMigrations` 是同步显式步骤；返回 nil 表示本次 namespace 的登记 Migration 均已确认 applied，不代表其他 namespace、Worker、HTTP Handler 或外部 Provider 已 Ready。
- Server 负责先构造共享资源，再注册并执行各领域 Migration，随后装配 Handler/Worker 并决定对外 Ready。
- Shutdown 时 Server 先停止接收新请求/任务，再停止并等待 Worker 和正在使用资源的请求，最后关闭共享 Redis/SQL 资源。
- Platform Migration Runner 只对单次执行负责，不持有跨调用的后台 goroutine，也不管理服务全局生命周期。

## 8. 明确不属于 PLAT-CTR-006

- 领域 Repository 接口、SQL 查询、领域表结构、Migration SQL 和数据保留策略。
- Server Migration Registry 的 Register/Run/RunWithReport/ValidateApplied、跨 namespace 聚合报告和启动顺序。
- 各模块是否拥有 Migration、Server 是否遗漏某个模块注册、生产部署是否执行 Migration。
- 事务边界、Repository 并发访问和领域数据一致性。

这些内容分别归领域契约和 SRV-CTR-001/Server 实施验收；不得扩展成 Platform 的通用 Repository 或全局 Migration 自动发现机制。

## 9. 版本与兼容

- 契约版本：1.0.0。
- 本契约为纯 InProcess Go API；JSON Schema 不适用，强类型 Go API 与本规则文件为唯一权威定义。
- 新增可选连接配置或不改变执行语义的能力可按 GOV-CTR-004 递增 MINOR。
- 改变连接池默认值/上限、Redis 故障边界、Migration checksum、History 状态机、锁语义、失败恢复或资源所有权属于破坏性变更，必须递增 MAJOR 并先完成项目级变更审批。
