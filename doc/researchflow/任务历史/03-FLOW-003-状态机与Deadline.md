# FLOW-003：状态机、Deadline、Cancel 与 Finalization

## 阶段结论

状态机、Deadline Scanner/Finalizer、取消线性化、取消恢复、过期 Attempt 恢复和 Run Finalization 已实现。MariaDB 事务竞争测试覆盖已登记的正常、取消和 Deadline 终态路径。

## 已有证据

- `internal/researchflow/state/**`
- `internal/researchflow/application/*finaliz*`、`*deadline*`、`*cancellation*`
- `internal/researchflow/storage/mysql_run_*` 与对应集成测试
- 历史真实失败终态测试：Actions Run 44268307409，SHA `7f460ba624a12419d223c35da0223f34e070b7a7` success。

## 未完成边界

DONE 仅限已覆盖场景。新增部署恢复、跨进程故障或状态机修改时按影响面补测，不重复建设状态机。

## 原始记录

完整历史执行记录保留于 [各个子任务完成状态.md](./各个子任务完成状态.md)。
