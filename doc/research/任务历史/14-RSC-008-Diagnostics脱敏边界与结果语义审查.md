# RSC-008 Diagnostics 脱敏边界与结果语义审查

- 日期：2026-11-19
- 分支：`feat/research`
- 验收 HEAD：`0d9dc0651a6e0bda3888d695f2ffec5545c248bd`
- Workflow Run：[44439983842](https://github.com/Kasign/stock_data_provider/actions/runs/44439983842)
- 任务性质：Research 既有领域 Diagnostics 的边界审查与脱敏回归测试；不新增诊断协议、通用 Runner 或跨模块接口。

## 1. 审查范围

- `internal/research/diagnostics/checks.go`
- `internal/research/diagnostics/checks_test.go`
- `internal/research/application/diagnostics.go`
- `internal/research/application/diagnostics_test.go`
- Platform Diagnostics Observation/Runner 的消费边界
- Context、Event、Claim、Evidence 的 PIT、版本、授权和快照校验路径

## 2. 审查结论

1. Context Check 严格要求快照的 `system_as_of` 与 Run 截止时间一致。
2. Event、Claim Check 拒绝 `SystemAsOf` 晚于 Run 截止时间的版本。
3. Evidence 缺少 `available_at` 时返回不可比较/不可用结果，不把缺失时间当作通过；严格 PIT 不满足时返回失败。
4. Evidence Check 在构建诊断前重新执行 Owner 授权检查。
5. 诊断输出不携带 Evidence 正文、授权引用或来源引用。
6. 快照使用独立副本，避免调用方在 Check 构建后修改输入快照影响诊断结果。
7. 本轮 Review 未发现需要修改的生产 Diagnostics 逻辑缺陷；新增测试用于固定敏感信息不外泄边界。

## 3. 脱敏测试与修复

新增 `TestEventAndClaimChecksDoNotExposeDomainText`，验证以下信息不会进入诊断 SafeMessage 或 Attributes：

- Event 标题、Event ID；
- Claim 正文、Claim ID；
- SubjectRef、EvidenceRef；
- 其他敏感领域文本。

测试最初构造了 `EventVersion=3`，但未提供同一 Event 的直接前一版本引用，违反 Research 已有版本血缘不变量，导致 Event Check 返回 failed。该问题属于测试样本不合法，不是生产诊断逻辑缺陷。

修复提交 `0d9dc0651a6e0bda3888d695f2ffec5545c248bd` 将测试样本改为合法的 `EventVersion=1`，保留敏感标题、标识和引用值，继续验证脱敏行为；没有修改生产逻辑，也没有放宽 Event 版本血缘约束。

## 4. CI 验收

Run [44439983842](https://github.com/Kasign/stock_data_provider/actions/runs/44439983842)：

| Job | 结果 |
|---|---|
| Research module format, tests, race and vet | success |
| Research MariaDB migration and repository integration | success |
| Test go-core | success |
| Test . | success |

Run 整体结论为 `success`，HEAD 与本记录验收 SHA 一致。Researchflow/Notification 相关实际执行 Jobs 也通过；Compose、Market Data、News 等 Job 为 skipped，不计为通过。

## 5. 未完成的跨模块验收

- Researchflow 尚未提供真实 Diagnostics Facade/Check 消费点，RSC-008 的消费者状态映射与运行时装配验收继续等待。
- Platform Runner 的并发、超时和通用执行能力继续复用，不在 Research 复制。
- Server 负责真实生产身份、依赖注入和系统组合装配。
- News/Data 的真实来源证据和授权一致性问题不由 Research Diagnostics 代替解决。

## 6. 最终结论

Research 本地 Diagnostics 结果语义与脱敏测试已通过当前精确 SHA 的 Research 专项 CI。此结论仅覆盖 Research 本地检查和对应自动化测试，不代表 Researchflow 消费端、Server 生产装配或真实来源数据验收已经完成。
