# GOV-CTR-003 公共标识符与幂等作用域规则

- 契约编号：GOV-CTR-003
- 规则版本：1.0.0
- 状态：freeze-ready（待项目负责人确认冻结）
- 适用范围：所有跨模块请求、异步工作链、业务事件、命令幂等和分布式追踪元数据
- 权威性：本文件是 GOV-CTR-003 公共身份/关联/幂等语义的唯一权威来源。Platform 运行时传播由 PLAT-CTR-002 定义；各领域 DTO/Port/事件契约定义字段级映射和操作专属幂等规则。

## 1. 标识符类别与不可替代关系

| 标识符 | 作用域与用途 | 不得替代 |
|---|---|---|
| entity/business ID | 领域对象生命周期身份，由对象 Owner 定义 | Request ID、Correlation ID |
| `request_id` | 单次同步请求或单次实际执行的排障身份 | 业务主键、幂等键、认证身份 |
| `originating_request_id` | 异步任务创建时的来源请求 | Worker 当前 request_id |
| `correlation_id` | 跨模块业务工作链关联，贯穿 Run/Stage/Attempt/重试 | event_id 去重、幂等键、授权 |
| `event_id` | 生产方创建的稳定事件身份，重放时保持不变 | correlation_id、source domain object ID |
| `idempotency_key` | 一次命令/副作用意图的稳定键 | request_id、correlation_id、资源 ID |
| `trace_id/span_id` | 可选分布式追踪元数据 | 业务身份、幂等键、授权凭证 |

所有 ID/Ref 均按不透明值消费；除字段 Schema 明确规定的格式校验外，消费者不得解析内部结构或从 ID 推断权限、业务状态和数据内容。

## 2. Request 与异步来源标识

- `request_id` 标识一次实际请求/执行，只用于请求排障和日志关联。
- 同步入口的生成、校验、信任 Gateway、响应回显和出站传播由 PLAT-CTR-002 负责；本规则不替代其运行时 Header/Context 契约。
- Worker 每次实际执行/Attempt 使用独立 request_id；重试不得复用来源 HTTP request_id 作为 Worker 当前 request_id。
- `originating_request_id` 记录创建异步任务的来源请求，从可信 Request Context 注入。纯内部触发且没有来源请求时显式 null，不伪造 system-origin Request ID。
- request_id 不作为业务主键、认证身份、授权凭证或幂等键。

## 3. Correlation ID

- `correlation_id` 标识跨模块业务工作链；同一 Run/业务链及其 Stage、Attempt、事件和重试保持稳定。
- 有父业务 Context 时必须从可信父 Context 原样继承；无父链路时由入口/编排方生成高熵值。
- 消费者不得信任客户端自报值作为可信父 Context。
- correlation_id 不用于去重、唯一资源身份、授权或请求认证。

## 4. Event ID

- `event_id` 标识生产方创建的稳定事件身份；同一逻辑事件的发布重试/重放不得改变 event_id。
- 跨模块 Inbox 唯一性按 `(source_module, event_id)` 表达；同一 source_module 范围内 event_id 必须唯一。
- 若领域事件有版本，event_id 表示稳定对象/事件身份，event_version 表示不可变版本；二者不可混用。
- Notification 集成事件的 event_id 属于独立消息身份，不得复用 RunRef、RiskDecision ID 或 ResearchEvent ID；来源领域对象通过 source_event_ref 表达。
- correlation_id 不作为 Inbox 去重键；event_id 不作为命令幂等键。

## 5. Idempotency Key 与 Request Fingerprint

每个有副作用的命令必须由其生产方/Owner 定义：
- idempotency_key 的生成方和唯一性作用域；作用域至少包含调用主体与操作/use_case；
- key 的保留期、规范化请求规则和 fingerprint 输入字段；
- 相同 key + 相同 fingerprint 的原结果返回语义；
- 相同 key + 不同 fingerprint 的稳定冲突语义；
- 并发重复命令的线性化点及唯一约束；
- 外部副作用结果不确定时的 unknown/对账/重试规则。

公共行为：
- 相同 scope + key + fingerprint 返回原结果，不重复产生副作用。
- 相同 scope + key + 不同 fingerprint 必须返回稳定冲突，不得静默覆盖或复用旧结果。
- fingerprint 覆盖影响命令语义/副作用的规范化业务输入；不覆盖 request_id、correlation_id、originating_request_id、接收时间或传输 Header。
- 精确 fingerprint 字段集合由每个命令契约明确；消费者不得自行推断，也不得把 idempotency_key 自身或传输元数据混入业务指纹，除非命令契约明确规定。
- retryable=true 或 correlation_id 相同都不授权盲目重放。外部副作用结果 ambiguous 时，必须按操作幂等能力和对账接口处理，必要时保留 unknown。

## 6. 格式与长度

- `correlation_id`、`event_id`、`idempotency_key` 使用 1..128 字节非空不透明字符串。
- `request_id`、`originating_request_id` 遵循 PLAT-CTR-002 的 X-Request-ID 格式、生成和可信入口规则。
- 领域对象 ID 的格式、长度、版本关系和唯一性范围由领域 Owner 的 Schema 定义。
- `gateway_request_id` 与 `upstream_request_id` 如需保留，必须是独立字段，不能覆盖 request_id 或 originating_request_id。

## 7. Trace 元数据

- trace_id/span_id 是可选分布式追踪元数据，不是业务主键、幂等键或授权凭证。
- 首期不要求 Trace SDK、span_id 或 baggage 全链路传播；只有真实跨服务并行调用追踪需求出现时再独立扩展。
- 不能因缺少 trace_id/span_id 而改变业务关联或幂等语义。

## 8. 字段级责任边界

本规则只定义通用身份职责和幂等原则。以下内容由对应契约定义：
- X-Request-ID Header、可信 Gateway、Go Context、HTTP 出站传播和 Worker request_id 生成：PLAT-CTR-002；
- Run/Stage/Attempt 的 originating_request_id、correlation_id 传播和 CreateRun 幂等指纹：FLOW-CTR-001/002/003；
- NotificationEvent event_id、source_module、Inbox/Publish 唯一键和 payload fingerprint：NTF-CTR-001/002、INT-CTR-001；
- Quant/Research 命令的调用主体、use_case、idempotency_key 和 fingerprint：QNT/RSC 对应 CTR。

这些字段映射、Golden、并发唯一约束和实际传播测试仍由对应 CTR 验收，不反向阻断 GOV-CTR-003 公共规则本身冻结。
