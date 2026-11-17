# WebUI 统一诊断接入计划

更新时间：2026-11-12  
核对基线：feat/webui-market-data-console @ e43dba62079c872d95919e5412e9c7cad21f477c

> 本文只记录诊断消费、字段展示和验收约束；总体任务状态以《WebUI 整体规划》为准。

## 1. 目标与职责边界

WebUI 是统一诊断工作台的消费端：
- Platform 提供 Request ID、Fault、Observation 等公共诊断原语。
- 各领域模块负责本领域诊断事实、业务状态、质量规则和证据。
- Server 负责将模块诊断能力装配到 Root HTTP API。
- WebUI 负责按领域分组展示请求状态、诊断证据和可诊断错误。

WebUI 不实现探测器，不复制 Provider/业务质量规则，不推算覆盖率分母，不把领域状态压成单一平台状态，也不访问模块内部存储。

## 2. 当前接入基线

当前 Web 已有 Web 静态检查、行情/分析服务健康端点、Provider 注册清单、只读运行诊断、用户显式触发的主动探测和探测历史。

历史自动化结果见《WebUI 提测验收清单》。Mock API 自动化只证明前端展示行为；Provider 真实可用性、真实质量和 MariaDB 历史持久化仍需部署环境证据。

## 3. 目标诊断聚合映射

| 诊断分组 | 责任方 | Web 展示 | Web 不得推断 |
|---|---|---|---|
| Platform/Server | Platform + Server | HTTP/依赖故障、Request ID、Fault、服务就绪 | 业务质量是否合格 |
| Data | Data | Provider 能力、请求观测、字段校验、覆盖率、Freshness | Provider 质量规则、分母、字段对账结论 |
| News | News | 抓取/解析状态、来源、授权、采集时间、失败原因 | 新闻真实性、来源授权、事件归并 |
| Research | Research | Evidence 查询状态、来源授权、引用完整性、对象关联 | Claim/Hypothesis 正确性或证据充分性 |
| Quant | Quant | 计算状态、数据版本、PIT 时点、风险决策、评估摘要 | 因子质量、回测结果、风险门禁 |
| Researchflow | Researchflow | Run/Stage/Attempt、阶段耗时、重试/取消/失败原因 | Run 终态和阶段状态机 |
| LLM | LLM | 模型调用状态、耗时、结构化输出校验和错误 | 模型结论真实性 |
| Notification | Notification | 订阅、渠道、发送尝试、重试和最终投递状态 | UI 展示成功等于消息送达 |

这是目标展示责任映射，不表示上述诊断 API 已全部存在或注册。

## 4. 统一展示约束

1. Platform Observation 是公共载体，不替代领域诊断字段；领域状态、质量、Freshness、授权和风险门禁保留原语义。
2. 每条诊断尽可能显示模块/依赖、状态、reason_code、Request ID、发生时间、证据引用和可重试性；未提供字段显示未知。
3. pass、degraded、fail、not_comparable、skipped、not_configured、not_supported 按契约区分。
4. skipped/not_configured/not_supported 不得映射为 pass。
5. Quality 与 Freshness 分开展示；provider/source time、fetched/observed time、checked time 不互相替代。
6. 覆盖率仅显示服务端 ratio、分子/分母和维度；缺失分母时显示不可比较/未知。
7. 诊断明细和实际值遵循服务端脱敏约束，使用安全文本节点。
8. 当前请求与历史请求分别管理生命周期；历史读取失败不得清空当前结果。
9. 取消、超时和旧连接响应按领域 generation/AbortController 规则处理。
10. LLM 输出不得覆盖 Researchflow 终态或 Quant 风险阻断状态。

## 5. Data 主动探测字段约束

当前 Data 探测展示已使用：
- validation_total_count、validation_error_count、validation_warning_count、validation_info_count。
- validation_truncated、validation_issues。
- quote_response、price_classification、quote_timestamp、amount 等覆盖维度。
- provider_time、fetched_at、checked_at。
- quality_status/quality_reasons 与独立 Freshness 字段。

规则：
- 字段问题计数使用服务端计数；页面明细最多展开 30 条，服务端截断需提示。
- 实际值只显示服务端脱敏摘要。
- 覆盖率使用服务端 ratio，不从前端返回记录数推导。
- 时间字段分别展示，缺失不替代。
- Quality 与 Freshness 不合并。
- 主动探测必须由用户显式触发；普通页面打开或健康检查不得隐式访问 Provider。
- 第三方 Provider 凭据由服务端管理，Web 不读取、保存或回显。

## 6. 接入顺序与验收门禁

| 阶段 | 工作 | 准入条件 | 验收证据 |
|---|---|---|---|
| D0 | 保持当前 Data 诊断展示 | 当前响应契约稳定 | 现有单测/Browser Smoke 历史记录 |
| D1 | Platform/Server 诊断基础接入 | Platform 原语稳定，Server 有统一诊断出口 | 真实 Root Server 请求、Request ID/Fault/Observation 样例 |
| D2 | News/Research/Quant 诊断 | 领域诊断 DTO、脱敏和证据引用契约实现 | 各模块真实响应、降级/失败/不可比较样例 |
| D3 | Researchflow/LLM/Notification 诊断 | Run 状态、模型调用和投递状态 API 可用 | 真实 Run、LLM 错误和 Notification 投递记录 |
| D4 | 聚合页端到端验收 | 目标模块由 Server 注册且权限可用 | 真实部署全链路与错误注入记录 |

不得为填满页面而虚构模块健康状态或新增无来源字段。契约缺口回到领域/Server 契约流程解决。

## 7. 当前结论

- 保留现有 Data 诊断展示，不重做字段校验、覆盖率和时间语义。
- Data 主动探测的真实 Provider、跨源黄金样例和历史持久化仍以数据侧真实验收为准。
- Platform 有 Observation 原语或模块文档存在，不代表 Web 已接入该模块诊断。
- Web 后续只做真实接口接入、状态表达和展示问题修正，不扩展第二套诊断规则。
