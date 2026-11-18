# Notification Relay 最终验收记录

## 验收目的

完成 Researchflow Terminal Run → Outbox → Notification Relay → Inbox/Fanout 的最终故障、重试、幂等验收。

## 已有验收边界

- 正常 Terminal Run：Outbox 进入 Notification Inbox。
- 接收失败：Outbox 保持未发布并进入 Retry。
- Retry 成功：Outbox 发布，Inbox/Fanout 正常落库。
- 重复 Relay：Inbox 与 FanoutWorkItem 保持幂等，不产生重复通知。
- 失败 Run：Finalization 生成唯一 ResultSummary 与 terminal Outbox。
- Provider Outcome 与 Platform Diagnostic 不混淆；没有独立 Diagnostic Record 时保持 DiagnosticRefs=[]。

## 当前执行

最终 Integration CI Run 44251844794 已对精确 SHA `5e41240d3fcc4f7ce98057e1729d5f9d0b97303e` 成功；其中 relay 相关集成测试通过。

真实 Tavily Researchflow 单 Worker、双 Worker 生产链路此前已成功；Provider Outcome、QualitySummary、ResultSummary、终态 Outbox 均已验证。

受控 Provider Retry/Non-Retry 场景已覆盖：RateLimited/Timeout 重试后成功，Malformed Response 不重试并失败；Notification Relay 对成功与失败终态的投递由既有 MariaDB 集成测试覆盖。

真实 Tavily 成功链路只用于成功路径，不通过修改真实凭证制造失败。

## 收口标准

1. Notification Relay ACK/Retry 边界：PASS。
2. 重复投递 Inbox/Fanout 幂等：PASS。
3. Integration CI 对最新代码 SHA：PASS（Run 44251844794）。
4. Provider Outcome 与 Platform Diagnostic 边界：PASS。
5. 本轮集中联调：可封板。

## 明确不纳入本轮

- Bocha/Brave 未配置不作为当前阻塞项。
- 不从 published_at/published_date 推导 available_at。
- 不开放 Researchflow HTTP API。
- 不新增没有契约依据的 Diagnostic 持久化系统。
