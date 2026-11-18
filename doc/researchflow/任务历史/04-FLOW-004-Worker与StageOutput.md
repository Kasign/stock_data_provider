# FLOW-004：Scheduler、Worker、容量、Lease/Fencing 与 Stage Output

## 阶段结论

Scheduler 候选扫描、固定 WorkerPool、优雅 Shutdown、全局容量守卫、Claim/Lease/Fencing、ExpiredAttemptScanner、Stage Provider Envelope、不可变 Stage Output 资源和事务提交链已实现。

## 已有证据

- `internal/researchflow/application/scheduler*`、`worker*`、`expired_attempt_scanner.go`
- `internal/researchflow/stageoutput/**`
- `internal/researchflow/storage/mysql_attempt_*`、`mysql_capacity*`、`mysql_stage_output_resource.go`
- 真实 News/Tavily 单/双 Worker 及受控故障验收见 Integration 交接记录；相关历史 CI 记录见原始归档。

## 未完成边界

不重复建设 Worker/队列。新 Stage Provider 的接入必须补对应 Output Schema/Golden 和真实运行证据；Server 负责生产生命周期装配。
