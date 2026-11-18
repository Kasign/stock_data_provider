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

本次通过受控真实链路工作流继续执行最终 Researchflow 验收；真实 Tavily 配置只用于成功链路，不通过修改真实凭证制造失败。

## 收口标准

1. Notification Relay ACK/Retry 边界有明确 PASS 证据。
2. 重复投递保持 Inbox/Fanout 唯一。
3. Integration CI 对最新代码 SHA 全绿。
4. 完成最终 Review 后关闭本轮集中联调。

## 明确不纳入本轮

- Bocha/Brave 未配置不作为当前阻塞项。
- 不从 published_at/published_date 推导 available_at。
- 不开放 Researchflow HTTP API。
- 不新增没有契约依据的 Diagnostic 持久化系统。
