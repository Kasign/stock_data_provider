# 五模块公共契约一致性 Review 记录

- 所属治理：GOV-004
- 分支：`feat/design_doc`
- 日期：2026-11-06
- 状态：规划侧交叉 Review 完成；Owner Review 和正式契约测试待完成

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
未发现需要改变既有依赖方向的冲突：Quant 与 Research 互不依赖；Researchflow 是唯一跨模块组合者；LLM 不执行工具；Notification 不计算告警条件；Platform 不反向依赖业务模块；Data 仍通过独立 HTTP/OpenAPI Module 接入。

## 4. 同步文件
- 五模块公共契约与 Schema 冻结基线
- Quant / Research 字段级 DTO
- LLM Provider Port 与 DTO 字段级契约
- 五模块任务拆分交叉 Review 记录
- GOV-004 最终 Review 与契约冻结门禁
- 整体任务规划总览

## 5. Owner Review 与门禁

| 项目 | Owner | 状态 |
|---|---|---|
| QuantMetric 精度、指标类型和边界 | Quant | 待确认 |
| EvidenceAuthorizationRef 与当前授权检查 | Research + News | 待确认 |
| system_as_of 传播与字段语义 | Research + Researchflow + Quant | 待确认 |
| ResearchEvent 到 NotificationEvent 映射 | Research + Notification | 待确认 |
| Platform Fault 到 ModelErrorInfo 映射 | LLM + Platform | 待确认 |
| 状态映射与幂等作用域 | 各模块 Owner | 待确认 |

本轮未生成正式 JSON Schema、Go DTO、Mapper 或契约测试；未执行代码测试。GOV-004 继续 NO-GO。没有用户授权，不创建五模块开发分支。

**结论：规划文档间主要语义差异已完成校准；下一步由各模块 Owner 对建议决议签字，并将正式 JSON Schema/Golden 纳入模块契约测试。**
