# RSC-003：Event 时间证据与版本血缘规则级验收

- 日期：2026-11-18
- 分支：`feat/research`
- 代码范围：`internal/research/event/**`、`internal/research/application/application_test.go`、`internal/research/storage/**`
- 代码验收 SHA：`c01e613c1180380a0220311484bdc774f1bf58f3`
- CI Run：[44288143013](https://github.com/Kasign/stock_data_provider/actions/runs/44288143013)

## 1. 复核发现

冻结的 RSC-CTR-002 明确要求：同一 `event_id` 的版本修订必须通过 `derived_from_refs` 指向直接前一不可变版本；合并 Event 还可以记录其他直接父版本。

既有代码已能在 Repository 事务内解析精确 Event 版本、拒绝缺失引用、自引用和循环，但 Domain `ResearchEvent.Validate()` 未强制 EventVersion 大于 1 时必须包含同一 event_id 的 version-1 引用。导致不完整血缘可通过 Domain/Application 校验，直到后续 Repository 解析时才可能被发现。

## 2. 修复内容

- Domain 对 `EventVersion > 1` 强制要求 `derived_from_refs` 包含同一 `event_id` 的直接前一版本。
- 允许在该直接前序引用之外保留额外直接父版本，不阻断合并关系。
- 保留 Repository 事务内精确版本解析、完整血缘递归检查和环检测。
- Application UpdateEvent 在持久化前拒绝缺少直接前序引用的修订。
- 生产代码未新增字段、接口、Schema、Golden、Migration 或外部授权策略。

## 3. 规则级测试

- Domain：Event v2/v3 缺少直接前序引用或引用更旧版本时拒绝；正确直接前序引用通过；可额外携带合并父版本；重复派生引用拒绝。
- SourceTime：`asserted_by` 缺失或带首尾空白时拒绝。冻结契约只定义其为来源或系统主体引用，没有规定命名空间/格式；本轮不增加猜测性解析。
- Application：UpdateEvent 缺少直接前序引用时拒绝，Repository 不会收到 Append。
- MariaDB：缺失父 Event、存在相同 event_id 但缺少指定 version 的引用均拒绝；自引用和多节点循环拒绝；合法 Event v2 通过；版本跳号仍由 Repository 冲突校验拒绝；历史版本损坏后追加仍 fail-closed。
- Outbox：生命周期事件 v2 测试样本均补齐直接前序引用，确保测试验证的是 Outbox 行为而非缺失血缘的前置拒绝。

## 4. 验收证据

Run `44288143013` 的 head SHA 与代码验收 SHA 完全一致：

- Research module validation：gofmt、`go test ./internal/research/...`、`go test -race ./internal/research/...`、`go vet ./internal/research/...` 全部 success。
- Research MariaDB Migration/Repository integration：success。
- 根模块 `go test ./...`：success。完整根模块 race/vet 不作为本任务的直接验收依据。
- 后续文档提交触发分支更新后，该 GitHub Actions Run 的整体结论为 cancelled；上述 Research 专项 Job 和根模块测试步骤在取消前已 success。根模块 race/vet 未完成，不宣称完整工作流通过。

## 5. 尚待外部证据

RSC-003 整体任务尚未全部完成，当前只关闭规则级缺口。以下事项必须等待 News/Data Owner 提供可审计来源证据：

1. 真实 occurred_at、published_at、available_at 原始值、来源版本和时间精度/时区证据。
2. 真实多来源合并/拆分样本及其直接父 Event 版本。
3. `asserted_by` 来源/系统主体引用如何映射到真实来源主体的证据。

Fixture 只证明规则行为，不等于真实来源验收。没有来源依据时不添加主体命名空间、枚举、注册表或时间推断规则。

## 6. 后续状态

- RSC-003：规则级实现/测试完成，等待 News/Data 真实来源证据。
- RSC-008：等待 Researchflow 真实 Facade/Check 调用点。
- RSC-002：SHELVED；RSC-004：BLOCKED；RSC-006：WAITING_DEPENDENCY。
- RSC-001 的 Server 生产 CallerRef 注入和真实 Context Owner Authorizer 装配仍属于 Server/Integration 外部门禁。
