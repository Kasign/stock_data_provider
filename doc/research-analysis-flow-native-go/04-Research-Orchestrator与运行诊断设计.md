# Research Orchestrator、持久化、LLM 与运行诊断设计

## 1. Orchestrator 职责

Orchestrator 是应用层流程执行器，不是 Quant/Research 的领域实现。负责：
- 解析研究请求和流程模板；
- 创建 Run、Stage 和 Attempt；
- 按依赖调度阶段；
- 管理 deadline、取消、重试和并发；
- 调用 Quant、Research、LLM、Repository 和 Notification Ports；
- 校验阶段输出和最终结果；
- 保存状态、诊断和版本引用。

不负责 Provider 选择、新闻采集、指标算法、事件事实存储或模型推理。

## 2. 运行状态机

Run：
`created → queued → running → succeeded | degraded | failed | cancelled | timed_out`

Stage：
`pending → ready → running → succeeded | degraded | skipped | failed | cancelled | timed_out`

Attempt 记录每次执行，失败重试不能覆盖之前的 Attempt。状态迁移必须校验合法前置状态并保持幂等。取消和超时应向下游传播 context cancellation。

失败策略：
- 必需阶段失败：默认 Run failed；
- 可选阶段失败：Run degraded，保留部分结果和原因；
- 可恢复依赖故障：在 deadline、幂等和重试预算内重试；
- 数据质量阻断：不是技术重试错误；
- 依赖缺失：按 required/optional 决定 failed/unavailable/skipped；
- 人工取消：保留已完成阶段和取消来源。

## 3. 并发与资源治理

- 每个 Run 有总 deadline、最大阶段并发、模型调用预算和数据请求预算。
- 阶段 DAG 只允许无环依赖；相互独立的市场/行业/个股子分析可并行。
- 同一证券/同一快照的重复计算可在明确缓存键和版本约束下复用。
- 不跨不同 as_of、复权基准、来源版本或质量状态共享缓存。
- 重试必须有最大次数、退避和总 deadline；禁止 Orchestrator、Adapter、HTTP Client 多层叠加重试。
- 首期不引入分布式队列；只有跨进程 Worker、持久租约或恢复需求经验证后再接入平台 Task 能力。

## 4. LLM Adapter

复用现有独立模型服务。Adapter 负责：
- OpenAI-compatible 或实际服务协议的请求/响应转换；
- 模型标识、参数、超时、限流和请求 ID；
- 结构化输出 Schema 校验；
- 用量、延迟、错误类别和模型版本记录；
- 敏感上下文脱敏和允许字段过滤。

LLM Adapter 不决定研究流程，不管理工具权限，不直接写业务表。Tool Registry/Policy 由应用层管理，工具调用必须限定白名单、参数 Schema、调用预算和数据权限。模型返回的工具参数需重新校验。

## 5. Persistence

分析领域拥有自己的 Repository 和 Migration 文件；不复制平台连接池、事务工具或 Migration Runner。MariaDB 保存：
- ResearchRun / Stage / Attempt；
- ResearchRequest 参数和流程模板版本；
- SnapshotRef / ContextVersion / EvidenceRef；
- StageResult / FinalResult / ResultVersion；
- LLMInvocation 元数据（不保存不必要的完整敏感上下文）；
- NotificationSubscription / DeliveryRecord（如启用）。

Redis 仅用于有界缓存、短期锁或协调，不作为 Run/Result 的唯一事实源。存储结构必须支持幂等、版本追加、事务边界、查询索引、保留策略和删除/归档策略。

Migration 由领域提供文件和注册信息；根注册/执行入口在 ANA-018 集成时协调，早期不得改共享 Migration Runner。

## 6. Research Diagnostics

诊断所有权：
- Data：Provider、字段、Freshness、Coverage、交叉数据质量和 Probe。
- News：来源授权、搜索/抓取/解析/去重/证券关联/持久化。
- Platform：公共 Check 执行器、超时/并发限制、基础聚合和统一 Fault/Observation。
- Research：Run/Stage、输入充分性、依赖、Context、Evidence、Quant、LLM 输出和最终结果质量。
- Notification：渠道和投递诊断。

分析诊断记录建议：
- run_id、stage_id、attempt_id、check_id；
- status：healthy / degraded / unavailable / skipped / failed；
- reason_code、safe_message、checked_at、duration；
- evidence_ref、attributes（白名单低敏字段）；
- dependency、retryable（按公共 Fault 契约映射）。

诊断重点：
1. 输入快照是否完整、PIT 是否满足；
2. 市场/行业/个股覆盖率是否达到任务阈值；
3. Context 是否缺少关键维度或裁剪掉关键反证；
4. Evidence 来源、时间和引用是否可验证；
5. Quant 指标是否因输入不足而 unknown/degraded；
6. LLM 请求是否超时、限流、Schema 错误或引用校验失败；
7. 阶段依赖、耗时、重试、取消和失败位置；
8. 最终结果是否存在无证据结论、数值矛盾或风险门禁覆盖企图。

平台诊断 Runner 不重复建设；Research 提供领域 Check 和结果，按平台契约注册。领域诊断历史和证据由 Research 自己持有。

## 7. Notification

Notification 只负责订阅、模板、渠道、发送、重试、幂等和投递记录。告警触发条件由 Quant/Research 产生业务事件，Notification 不计算指标和策略。

通知失败不能回滚已完成研究结果；投递记录独立记录 queued/sent/failed/retrying/skipped。首期先支持明确必要的渠道，不为未来渠道预建复杂插件框架。

## 8. API 与系统集成门禁

分析模块先定义 Application API/Ports，不提前修改共享 HTTP 路由和服务启动入口。最终由服务端集成协调：
- API DTO/OpenAPI；
- DI/wiring；
- Worker 生命周期；
- 公共安全、日志、Fault、Diagnostics 接入；
- Migration 注册和部署配置；
- WebUI 消费契约。

以上集中在 ANA-018，不允许多个分支同时修改共享入口。
