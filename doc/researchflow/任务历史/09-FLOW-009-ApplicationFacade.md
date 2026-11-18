# FLOW-009：Application Facade

## 阶段结论

内部 Application Facade 已提供 CreateRun、GetRun、CancelRun、RetryStage、GetWorkflowTemplate 等用例入口，并通过 RequestIdentity、Run Access Authorizer 等边界保护调用。

## 已有证据

- `internal/researchflow/application/**`
- `doc/researchflow/04-ApplicationPort与运行时契约落地补充.md`
- Integration/Server 内部装配与调用验收见项目级交接记录。

## 未完成边界

HTTP API 按项目决议 deferred；不因内部 Facade 已存在而增加公网路由。具体 Authorizer、HardRiskGateRegistry 生产注入由真实模板和 Server 组合装配验收。
