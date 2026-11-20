# FLOW-007：Run/Stage Storage 与 Outbox

## 阶段结论

MySQL/MariaDB Migration、Run/Stage/Attempt 持久化、Template Registry、Worker Store、Deadline/Finalization、ResultSummary 和 Outbox 原子写入/Claim/Ack/Retry 已实现并有 MariaDB 集成验收。

## 已有证据

- `internal/researchflow/storage/**`
- 历史模块 MariaDB 生命周期/Race 验收见原始任务归档和 Integration 交接记录。
- Integration 全量 CI 历史 Run 44269583998 success；不作为当前模块 HEAD 的 CI 替代。

## 未完成边界

新增变更只做定点回归。Outbox 终态事件与 Notification 的幂等消费/渠道投递分属跨模块责任；Researchflow 不建设 Notification Relay。


## 2026-11-18 时间权威修正

- 原 `OutboxDispatcher` 使用进程 `time.Now()` 计算并持久化 `next_attempt_at`。该时间被多个 Server/Worker 实例共同用于后续调度，不能以单机时钟为权威。
- `MySQLRunRepository` 现实现 `application.DatabaseClock`；`MySQLDatabaseClock` 通过 Platform `time.Service` 包装 MariaDB UTC Source；Outbox Dispatcher 在安排 RetryAt 前读取同一数据库 UTC 时钟。
- 缺少 DatabaseClock 能力或时钟读取失败时 fail-closed，不写入 retry 时间；新增时钟不可用测试。
- 代码验收 SHA：`0aaab4172d8cda075549bd6c69f2b6e1d465706b`；Researchflow 模块 gofmt/tests/race/vet Job 已通过，见 Actions Run 44296025284。
