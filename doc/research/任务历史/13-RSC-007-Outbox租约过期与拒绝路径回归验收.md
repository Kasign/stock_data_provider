# RSC-007 Outbox 租约过期与拒绝路径回归验收

- 日期：2026-11-18
- 分支：`feat/research`
- 代码范围：`internal/research/storage/**`
- 文档范围：`doc/research/**`
- 前置 HEAD：`f7c646d0e54217ad06795663b9dd402ac5a060b6`
- 任务性质：已完成 Outbox 能力的验收补充；不新增业务能力、状态、接口或数据库结构。

## 1. Review 结论

既有实现与测试已覆盖：
- Event 版本写入与 Outbox 记录处于同一事务，Outbox 写入失败会回滚 Event 版本和聚合头。
- 并发 Worker 不会重复获得同一条有效租约。
- retry_wait 到期后可重新领取，并生成新 lease token。
- 旧 token 不能确认新租约；相同有效租约的重复发布确认具备幂等性。
- Payload fingerprint 在领取前重新计算校验。

本轮未发现需要直接修改的事务实现缺陷。发现自然租约过期后的重领和终态拒绝路径缺少一组完整 MariaDB 回归覆盖，因此仅补测试。

## 2. 新增验收场景

新增 `TestResearchOutboxExpiredLeaseCanBeReclaimedAndFenced`：
1. 创建 Event，验证初始生命周期 Outbox 同事务产生。
2. Worker A 领取短租约，等待数据库租约自然过期。
3. Worker B 重新领取同一消息，必须获得新的 lease token。
4. Worker A 使用过期 token 执行 MarkPublished、ScheduleRetry、Reject，三者均必须返回 ErrConflict。
5. Worker B 使用当前 token 执行 Reject，操作成功。
6. rejected 消息不可再次领取；租约 owner/token/until 清空，attempt_count 为 2，last_error_code 保留拒绝原因。
7. rejected 终态不能再通过旧确认路径变为 published。

测试使用已有 `openResearchIntegrationDB`，要求设置 `RESEARCH_TEST_MYSQL_DSN`，并沿用数据库名必须以 `_test` 结尾的安全限制。没有 DSN 时按现有集成测试约定 Skip，不得把 Skip 记为 Pass。

## 3. 验收边界

- 这是 MariaDB 集成测试，不是纯 Mock 测试；只有对应 CI Job 实际连接 MariaDB 并执行该用例后才能记录通过。
- 不修改 Outbox SQL、Migration、Application Port 或 Notification Relay。
- 不代表 Server Root Relay、Worker 生命周期或 Notification 生产回执闭环已完成。
- 本轮提交 SHA、Workflow Run 和 Job 结果待提交后补记。
