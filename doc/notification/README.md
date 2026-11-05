# Notification 模块

- 分支：`feat/notification`
- 代码：`internal/notification/**`
- 任务前缀：NTF
- 状态：规划中，尚未开始模块分支开发

独立通知业务模块，可供 Researchflow 和其他模块调用。负责 Subscription、Template、Channel、Dispatcher、Delivery、Attempt、幂等、重试和投递诊断；不计算业务告警条件，不依赖 Researchflow 内部实现。

目标结构：
```text
internal/notification/
  domain/ application/ delivery/ channels/ persistence/ diagnostics/
```

Platform 提供认证、Secret、日志、Trace、HTTP Client、任务和存储基础设施；上游模块产生通知事件或命令。发送失败不能回滚业务结果，测试发送必须显式指定目标。

详细任务见 [02-任务拆分与验收](02-任务拆分与验收.md)。
