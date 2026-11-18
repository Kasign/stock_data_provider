# Notification Relay 最终验收记录

更新时间：2026-11-18  
验收分支：`integration`  
当前集成 SHA：`bceadab032639d78bd7d0cd81533e183781e6720`

## 1. 验收范围

验证 Researchflow Deadline Finalization、Terminal Outbox、Notification Relay、Inbox/Fanout 的失败处理、重试、幂等和最终集成：

1. Deadline 到期后，Run 能被 Finalizer 收敛为失败终态。
2. ResultSummary 与 Terminal Outbox 在同一持久化事务中写入。
3. 失败终态 Outbox 能进入 Notification Inbox。
4. 接收失败时 Outbox 保持未发布并进入 Retry；重试成功后 Published。
5. Inbox 与 FanoutWorkItem 幂等；重复 Relay 不产生重复记录。
6. Outbox ACK/Retry 使用租约和 fencing，过期 Worker 不得覆盖新 Worker 的结果。

## 2. 本轮补齐的失败终态证据

### 2.1 Researchflow Deadline Finalization 与 MariaDB 持久化

扩展 `internal/researchflow/storage/mysql_run_deadline_integration_test.go` 中的 `TestMariaDBExpireRunDeadlineAtomicallyTimesOutPendingStages`：

- 创建真实 MariaDB Run/Stage 记录并将 Deadline 置为过期。
- `ExpireRunDeadline` 将 Stage 标记为 timed_out，并记录 `deadline_exceeded`。
- 调用真实 `DeadlineFinalizer`，将 Run 以 CAS 方式收敛为 `failed`，版本从 2 增至 3。
- 验证 ResultSummary 恰好一条，Run 的 ResultSummaryRef 与实际记录一致。
- 验证 Terminal Outbox 恰好一条，Payload 含 `status=failed`、`termination_reason=deadline_exceeded` 和对应 ResultSummaryRef。

精确证据：
- Researchflow 测试代码 SHA：`7f460ba624a12419d223c35da0223f34e070b7a7`
- Go test suite Run [44268307409](https://github.com/Kasign/stock_data_provider/actions/runs/44268307409)：success。
- 合入 integration 前的 PR 验收 Run [44269002477](https://github.com/Kasign/stock_data_provider/actions/runs/44269002477)：success。
- MariaDB lifecycle 日志中该测试在普通和 race 两次运行均为 PASS。

### 2.2 失败终态事件经 Notification Relay 投递

扩展 `internal/server/relay/researchflow_notification_integration_test.go`：

- 保留成功终态测试 `TestResearchflowOutboxToNotificationInboxMariaDB`。
- 新增 `TestResearchflowFailedTerminalEventNotificationMariaDB`，使用 `status=failed`、`termination_reason=deadline_exceeded` 的终态事件。
- 两种终态共用同一 Relay 流程：首次接收失败后 Outbox 进入 Retry；再次 Relay 后接收成功并 Published。
- 验证 Inbox 与 FanoutWorkItem 各只有一条；重复 Relay 不产生重复记录。

精确证据：
- Server 测试代码 SHA：`b6e94d4f9048a89ac7a80b70b94fd78152fa52ec`
- Server package validation Run [44266997745](https://github.com/Kasign/stock_data_provider/actions/runs/44266997745)：success。
- 该 Run 覆盖 gofmt、Server package tests、race、MariaDB/Redis、vet、Notification schema/image、容器启动和 SIGTERM 关闭。

### 2.3 当前 Integration 精确 SHA 验收

**PASS** Run [44269583998](https://github.com/Kasign/stock_data_provider/actions/runs/44269583998)

- Integration SHA：`bceadab032639d78bd7d0cd81533e183781e6720`
- 结果：success。
- Root Go formatting、go vet、Go tests/compile、News/Research/Researchflow cross-module tests、Worker Bootstrap MariaDB/Redis、go-core vet/tests、Compose、JavaScript syntax、WebUI unit tests 和 WebUI image build 全部通过。
- 日志确认 `internal/researchflow/storage`、`internal/server/relay`、Notification application/domain/schema/storage/template 包通过；Worker Bootstrap MariaDB/Redis 集成测试 PASS。
- 本次没有运行真实 Tavily/News Provider 请求；相应 Live workflow 为 skipped，未配置或未显式触发的 Provider 不作为本轮失败。

## 3. 既有边界验收

| 验收项 | 结果 | 证据/说明 |
|---|---|---|
| 正常 Terminal Run 投递 | PASS | Researchflow Outbox→Notification Inbox MariaDB Relay 测试 |
| 接收失败进入 Retry | PASS | fail-once Receiver 集成测试 |
| Retry 后 Published | PASS | 同一测试验证第二次 Relay 成功 |
| Inbox/Fanout 幂等 | PASS | 验证唯一记录及重复 Relay 不重复落库 |
| 失败 Run Finalization | PASS | DeadlineFinalizer MariaDB 集成测试 |
| 失败终态通知 | PASS | failed/deadline_exceeded 事件 Relay MariaDB 集成测试 |
| Outbox ACK/Retry fencing | PASS | Researchflow Storage 的租约、并发 Claim、过期 Worker 和 CAS 测试 |
| Provider Outcome / Platform Diagnostic 边界 | PASS | 无独立 Diagnostic Record 时保持 DiagnosticRefs=[]；Provider Outcome 不伪装成 Platform Diagnostic |
| 当前 Integration SHA 全量 CI | PASS | Run 44269583998，SHA bceadab... |

## 4. 验收结论

**Notification Relay 最终验收：PASS，可对该链路封板。**

本结论仅覆盖本文件列出的 Researchflow Deadline Finalization、Terminal Outbox、Notification Relay、Inbox/Fanout 和 ACK/Retry 边界；不代表所有模块均达到生产部署级 L3，也不代表所有 Provider Live Probe 已完成。

## 5. 明确不纳入本轮

- Bocha/Brave 未配置不作为本轮 Notification Relay 阻塞项。
- 不从 published_at/published_date 推导 available_at。
- 不开放 Researchflow HTTP API。
- 不新增没有契约依据的 Diagnostic 持久化系统。
- 不通过修改真实凭证或真实 Provider 行为来制造失败场景。
