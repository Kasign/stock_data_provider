# RSC-007 Outbox 租约过期与拒绝路径回归验收

- 日期：2026-11-18
- 分支：`feat/research`
- 代码范围：`internal/research/storage/**`
- 文档范围：`doc/research/**`
- 首轮测试提交：`b7200f41eb4cc55459b8b8c87a4ac2cdb1909f31`
- 修复提交：待本轮提交完成后填写
- 任务性质：已完成 Outbox 能力的验收补充；不新增业务能力、状态、接口或数据库结构。

## 1. Review 结论

既有实现与测试已覆盖：
- Event 版本写入与 Outbox 记录处于同一事务，Outbox 写入失败会回滚 Event 版本和聚合头。
- 并发 Worker 不会重复获得同一条有效租约。
- retry_wait 到期后可重新领取，并生成新 lease token。
- 旧 token 不能确认新租约；相同有效租约的重复发布确认具备幂等性。
- Payload fingerprint 在领取前重新计算校验。

## 2. 新增验收与实际缺陷

新增 `TestResearchOutboxExpiredLeaseCanBeReclaimedAndFenced`：
1. 创建 Event，验证初始生命周期 Outbox 同事务产生。
2. Worker A 领取短租约，等待数据库租约自然过期。
3. Worker B 重新领取同一消息，必须获得新的 lease token。
4. Worker A 使用过期 token 执行 MarkPublished、ScheduleRetry、Reject，三者均必须返回 ErrConflict。
5. Worker B 使用当前 token 执行 Reject，操作成功。
6. rejected 消息不可再次领取；租约 owner/token/until 清空，attempt_count 为 2，last_error_code 保留拒绝原因。

首轮 MariaDB CI 真实执行上述用例时发现：
- RejectOutbox 按设计将 lease_owner、lease_token、lease_until 置为 SQL NULL。
- 随后调用 MarkOutboxPublished，UPDATE 未命中后进入幂等状态查询。
- 原查询将可空的 lease_owner/lease_token 扫描到 Go string，导致 `converting NULL to string is unsupported`，无法按接口约定返回 ErrConflict。

修复：
- 使用 `sql.NullString` 承接可空租约列。
- 只有状态为 published 且 owner/token 均非 NULL、并与当前调用匹配时，才返回幂等成功。
- rejected 或租约已清空的记录正常返回 ErrConflict。
- 同步移除测试中重复的终态领取断言，保留一次完整断言。

## 3. 验收证据与边界

- 首轮 Workflow Run [44309799495](https://github.com/Kasign/stock_data_provider/actions/runs/44309799495)，HEAD `b7200f41eb4cc55459b8b8c87a4ac2cdb1909f31`。Research MariaDB 用例在新测试的“confirmation after rejection”断言处失败；其余既有 Outbox 原子性、重试、并发领取、存储集成测试通过。该 Run 明确为失败，不得作为验收通过证据。
- 修复提交的 SHA/Workflow Run/Research MariaDB 逐用例结果待补。
- 测试要求设置 `RESEARCH_TEST_MYSQL_DSN`，并沿用数据库名必须以 `_test` 结尾的安全限制。没有 DSN 时按现有集成测试约定 Skip，不得把 Skip 记为 Pass。
- 不修改 Outbox Migration、Application Port 或 Notification Relay。
- 不代表 Server Root Relay、Worker 生命周期或 Notification 生产回执闭环已完成。
