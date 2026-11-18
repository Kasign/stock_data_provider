# FLOW-007：Run/Stage Storage 与 Outbox

## 阶段结论

MySQL/MariaDB Migration、Run/Stage/Attempt 持久化、Template Registry、Worker Store、Deadline/Finalization、ResultSummary 和 Outbox 原子写入/Claim/Ack/Retry 已实现并有 MariaDB 集成验收。

## 已有证据

- `internal/researchflow/storage/**`
- 历史模块 MariaDB 生命周期/Race 验收见原始任务归档和 Integration 交接记录。
- Integration 全量 CI 历史 Run 44269583998 success；不作为当前模块 HEAD 的 CI 替代。

## 未完成边界

新增变更只做定点回归。Outbox 终态事件与 Notification 的幂等消费/渠道投递分属跨模块责任；Researchflow 不建设 Notification Relay。
