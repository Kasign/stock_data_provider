# HTTP、存储与异步任务设计

## 1. 统一 HTTP Client

### 1.1 设计目标

统一外部依赖调用的超时、总 Deadline、重试、响应体大小、并发、连接池、Trace、错误分类和脱敏。业务模块仍拥有自己的请求 DTO、Provider Adapter 和响应解析。

### 1.2 Policy

建议 Policy 包含：
- ConnectTimeout、TLSHandshakeTimeout、ResponseHeaderTimeout
- RequestTimeout / TotalDeadline
- MaxResponseBytes
- MaxIdleConns、MaxConnsPerHost、IdleConnTimeout
- RetryMaxAttempts、RetryableMethods、RetryableStatusCodes
- Backoff、Jitter、RetryBudget
- PerDependencyConcurrency
- Proxy / TLS / Redirect policy

### 1.3 重试原则

- 默认不重试非幂等写操作。
- GET/HEAD 等幂等请求才可按策略重试；POST 只有具备明确幂等键或上游幂等契约才允许。
- 重试受总 Deadline 和最大尝试次数双重约束。
- 429 尊重 Retry-After，但不得突破调用总预算。
- 只对网络暂时故障、超时、明确可重试状态码重试；参数错误、认证失败和数据质量错误不重试。
- 业务层、Adapter 层和 Client 层不得叠加无上限重试。
- 请求体必须可重放才允许重试。
- 每次重试记录 attempt 和 reason，不记录敏感 Header/body。

## 2. MariaDB

### 2.1 角色

MariaDB 是任务、Service Key 元数据、审计和业务事实的持久化事实源。各领域拥有自己的表和 Repository。

### 2.2 连接管理

平台统一连接池参数校验、Ping、Close、生命周期和基础健康检查。连接池参数由各服务配置，不设一组适合所有服务的固定值。

### 2.3 Repository 边界

平台 storage 只管理连接和基础事务能力；领域 Repository 自己维护 SQL、表结构和领域映射。平台不能直接查询 analysis_runs、news_articles 或 market_data 等领域表。

### 2.4 事务与故障

- 所有数据库操作接受 context。
- 事务范围尽可能短。
- 对可重试死锁/瞬时断连制定有限重试策略，避免事务副作用重复。
- 连接池耗尽和连接失败必须可诊断。
- 关键写入失败不能静默降级到 Redis 或内存。

## 3. Redis

Redis 只用于缓存、短期协调、速率限制计数、分布式租约辅助和可恢复队列加速。任务最终状态、Service Key 元数据、审计事实不能只存在 Redis。

- 所有 Key 必须有模块前缀和环境隔离。
- 缓存必须有 TTL 和明确失效策略。
- 凭证撤销缓存必须有安全 TTL，必要时主动失效。
- Redis 故障时，按能力分级：缓存可降级；关键协调/限流失效则明确拒绝或限制能力。
- 禁止把 Redis 可用性等同于业务数据可用性。

## 4. Migration

- 领域拥有 Migration SQL，平台/服务端统一协调执行入口和排序。
- Migration 编号/版本必须全局或按明确命名空间唯一。
- 执行前记录版本，成功后原子更新版本状态。
- 生产环境默认显式执行迁移，不在每个服务启动时并发自动改表。
- Migration 必须可审查、可回滚或提供前向修复方案。
- 不允许平台分支和数据侧分支同时修改 Migration 总注册入口。
- 公共平台新增表限于 service_credentials、audit_events、必要的 platform_task metadata 等真正归平台所有的数据。

## 5. 异步任务

### 5.1 任务模型

通用任务只承载执行生命周期元数据，不存储业务大对象。建议字段：
- task_id、task_type、status、request_hash、idempotency_key
- actor_type、actor_id、service_id、scope_snapshot
- created_at、scheduled_at、started_at、heartbeat_at、finished_at
- attempt、max_attempts、lease_owner、lease_until
- cancel_requested_at、failure_code、safe_error_message
- trace_context、result_ref

业务请求和结果由业务领域持久化，任务只引用业务对象或结果 ID。

### 5.2 状态机

    accepted -> queued -> running -> succeeded
                           |       -> failed
                           |       -> retry_wait -> queued
                           |       -> cancel_requested -> cancelled
                           |       -> timed_out
    queued -> cancelled

状态只能通过受控转换函数推进。终态不能被普通 Worker 任意覆盖。

### 5.3 Worker

- 使用数据库租约/锁领取任务，避免多实例重复执行。
- Worker 心跳更新 lease_until；租约过期后允许其他 Worker 接管。
- 执行有总超时和可取消 Context。
- 崩溃后根据任务类型决定重试、失败或人工恢复。
- 不可幂等的副作用必须有业务幂等保护或 Outbox。
- 任务恢复要保留原始 Actor、Scope 快照、Request ID 和 Trace 关联。

### 5.4 幂等

幂等键作用域至少包含调用主体、操作类型和 Key。服务端计算规范化请求摘要：
- 相同幂等键 + 相同摘要：返回已有任务。
- 相同幂等键 + 不同摘要：409 冲突。
- 过期键是否可重用必须有明确 TTL 和唯一约束策略。
- 幂等记录与任务创建必须在同一事务中。

当前分析 Task Queue 已有 MariaDB 持久化和 Idempotency-Key 逻辑，但通用租约、跨实例领取、任务 Actor 快照和完整取消语义仍需按 PLAT-001 逐项核验，不能直接认定已具备。

## 6. Outbox

只有当业务事务与异步消息/通知必须可靠联动时才引入 Outbox。Outbox 与业务写入同事务提交，后台 Dispatcher 负责投递和重试；接收方按 event_id 幂等。首期不为了抽象而先建设通用事件总线。
