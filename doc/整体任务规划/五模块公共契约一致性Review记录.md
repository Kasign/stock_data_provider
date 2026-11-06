# 五模块公共契约一致性 Review 记录

> **架构修订提示（2026-11-06）：** 本记录中“Data 通过独立 HTTP/OpenAPI Module 接入”的结论反映当时的迁移前代码边界，已被方案 B 取代。当前目标是 Data 迁入根 Go Module 的 `internal/marketdata`，通过 Application Port 同进程调用；本文件保留历史 Review 结论，不再作为 Data 部署/Module 边界的现行依据。现行依据见《Market Data并入Root Server模块化单体调整方案》和《跨模块契约目录与冻结状态》。

- 所属治理：GOV-004
- 分支：`feat/design_doc`
- 日期：2026-11-06
- 状态：规划侧交叉 Review 完成；正式 Schema/Golden/Contract Test 待完成，语义决策由项目统一决策方负责，不设个人 Owner 签字门槛

## 1. 范围

核对五模块公共 Schema、Application Port、Quant/Research 字段级 DTO、LLM Provider Port、Notification Port 的时间、ID、幂等、错误、质量和状态语义。本轮只读规划文档，不读取或修改其他业务分支，不推断模块实现状态。

## 2. 发现与修订

### CONSISTENCY-001：QuantMetric 公共结构不完整
公共摘要遗漏 metric_status、sample_count、lower_bound、upper_bound；字段级 DTO 已定义这些字段。已补齐公共摘要，value/bounds 类型由 metric_code 对应 Schema 固定。数值精度仍待 Quant Owner。

### CONSISTENCY-002：Evidence 授权字段混用
公共摘要使用 authorization_status，字段级 DTO 使用 authorization_ref/EvidenceAuthorizationRef。已统一 Evidence 公共结构为 authorization_ref，并补 system_as_of、limitations、content_fingerprint。NewsObjectRef.authorization_status 仍可表达 News 对象来源状态，不替代 Evidence 的授权审计引用。受限内容读取仍需来源 Owner 当前复核。

### CONSISTENCY-003：ResearchEvent 与 NotificationEvent 命名边界
ResearchEvent 是多来源证据归并后的领域对象；NotificationEvent 是通知模块消费的版本化集成消息。两者 ID、版本和生命周期独立，转换必须显式映射并生成独立 Notification event_id。

### CONSISTENCY-004：as_of 与 system_as_of
ResearchRun.as_of 是单次 Run 唯一系统知识截止值。Quant/Research DTO 的 as_of 与 Evidence/News Query 的 system_as_of 在该 Run 语境中表达同一截止时间；下游不得刷新。observed_at 保留真实观察时间；Data API 时间字段由 Quant Adapter 映射，不改变 Data 自身契约。

### CONSISTENCY-005：Platform Fault 与 ModelErrorInfo
LLM Adapter 内部使用 Platform Fault，对外映射为 ModelErrorInfo.kind/stable_code/retryable/http_status/retry_after/safe_message。Dependency 只保留在受控诊断元数据，Cause 只写内部日志。ModelResponse.request_id 负责关联，ModelErrorInfo 不重复携带 request_id。合法 incomplete/failed 协议响应不自动等同于 Platform Fault。

### CONSISTENCY-006：状态枚举分域
quality_status、Quant result_status、RiskDecision、Researchflow Run/Stage/Attempt、ModelResponse.status、Notification Delivery.status 各自独立。Run partial 表示覆盖不完整；Run succeeded 仍可有 degraded 质量摘要；Risk unknown 不得 pass。Notification skipped/blocked/unknown 不是 Platform Fault 的同义词。跨域映射必须显式且有 reason_code。

### CONSISTENCY-007：ID 与幂等
request_id 标识一次调用；correlation_id 关联跨模块链路；event_id 标识稳定事件；idempotency_key 标识幂等命令意图。异步消费者使用自身 Inbox 去重，不使用 correlation_id。幂等命令由 Owner 定义作用域和规范化 fingerprint；相同键同指纹返回原结果，不同指纹冲突。

## 3. 架构检查
当时未发现需要改变五模块依赖方向的冲突：Quant 与 Research 互不依赖；Researchflow 是唯一跨模块组合者；LLM 不执行工具；Notification 不计算告警条件；Platform 不反向依赖业务模块；Data 在该次 Review 的迁移前基线中通过独立 HTTP/OpenAPI Module 接入。该 Data 传输/Module 边界已由 2026-11-06 方案 B 决议替代。

## 4. 同步文件
- 五模块公共契约与 Schema 冻结基线
- Quant / Research 字段级 DTO
- LLM Provider Port 与 DTO 字段级契约
- 五模块任务拆分交叉 Review 记录
- GOV-004 最终 Review 与契约冻结门禁
- 整体任务规划总览

## 5. 初始 Review 记录（历史快照，当前不作为 Owner/状态登记表）

本节原始 Review 表格曾按模块列出待确认项，属于 2026-11-06 初始评审时点的过程记录，不再作为当前契约 Owner、决策状态或个人签字门槛。项目不存在独立模块 Owner；跨模块语义由项目统一决策方裁定，模块责任角色提供实现和测试证据。

当前唯一有效的契约编号、生产方/消费者、权威定义位置、状态、证据缺口及阻断范围，以[跨模块契约目录与冻结状态](./跨模块契约目录与冻结状态.md)为准。本轮未生成正式 JSON Schema、Go DTO、Mapper 或契约测试；GOV-004 继续 NO-GO。

**结论：规划文档间主要语义差异已完成校准；后续工作是按目录补齐正式 JSON Schema、Golden、Go DTO 和契约测试，不等待个人 Owner 签字。**


## 6. Researchflow 状态机项目决议更新（2026-11-06）

FLOW-CTR-001/002（历史别名 CONTRACT-006/007）不再等待模块 Owner 签字，已由项目统一决策方形成最终设计决议；契约状态仍为 pending_evidence。新增 Stage.retry_wait、有限自动重试、硬风险门禁/actionability、queued 取消与 Worker claim CAS、deadline 对未启动阶段处理、Run 终态不变量及副作用幂等边界。详细内容见 [Researchflow 最终决议](../researchflow/03-CONTRACT-006与007最终决议.md)。本轮只关闭设计决策，不代表实现或测试通过；其他模块决议仍按项目统一决策流程逐项完成。


## 7. Researchflow FLOW-CTR-001/002 当前有效决议（历史别名 CONTRACT-006/007，2026-11-06）

本节优先于第 5 节历史表格中 Researchflow“待确认”的状态。FLOW-CTR-001/002（历史别名 CONTRACT-006/007）已由项目统一决策方关闭设计决策，契约状态仍为 pending_evidence，不再等待模块 Owner 签字。当前权威文本为 [Researchflow 状态机最终决议](../researchflow/03-CONTRACT-006与007最终决议.md)。本轮补充 CAS 线性化、取消与 Worker claim 竞态、deadline 与 retry 竞争、Attempt 停止确认/fencing、容量原子预留和 actionability 优先级。FLOW-CTR-001～004 的实现与自动化测试仍需按项目级目录推进。

## 8. 第二轮实现前置契约复核（2026-11-06）

本轮复核以 `feat/design_doc` 当前规划文档为准，补充结论和执行清单见 [五模块跨模块实现前置契约复核与合并清单](./五模块跨模块实现前置契约复核与合并清单.md)。

本轮纠正初始 Review 中已被后续规划决议覆盖的判断：

- UTC 绝对时间统一为 RFC3339Nano UTC（Z），已由 GOV-CTR-002 接受；后续缺口是各模块序列化/边界测试证据，不是重新决定时区策略。
- ResearchEvent 通知 payload 已定义 `changed_at` 为状态迁移提交时间，Envelope `occurred_at` 映射该值；后续缺口是领域状态与 Outbox 的原子固化及生产映射测试，不应直接把 Notification 接收时间或 `system_observed_at` 当作替代值。
- Quant `input_snapshot_refs` 必须存在且为空数组仅允许契约白名单中的纯函数；不得将其解释为所有 Quant 计算都可无输入。
- Data v1 number/float64 与 Data v2 Decimal/PIT/AuthorizationRef 的边界已经明确；不得在 Quant/Research Adapter 中混用或伪称无损。
- Evidence `authorization_ref`、派生引用链和来源 Owner 当前授权复核已有语义定义；后续主要补真实读取路径和撤销/过期竞态证据。
- Notification Payload 已明确使用 RFC 8785 JCS + SHA-256；本轮未发现 Envelope 原始字节数、Payload 字节数和 JSON 嵌套深度的统一入口上限，列为 P0 待决策/实现项。

新增待补实现约束包括：命令 Request Fingerprint 的统一 JCS 规范、LLM Clock/Deadline 注入与传播、Notification DeliveryPlan 持久化前 Validate + VerifyFingerprint 双重校验，以及上述 Envelope 输入资源上限。上述内容属于 Review 发现/建议，不自动改变《跨模块契约目录与冻结状态》的 accepted/pending_evidence 状态。

本轮只修改规划分支文档，不读取或修改五个业务分支代码；不代表任何模块 CI、Go 测试或跨模块集成验收已通过。
