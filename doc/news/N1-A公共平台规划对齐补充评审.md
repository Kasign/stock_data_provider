# N1-A 公共平台规划对齐补充评审

- 检查日期：2026-11-05
- 新闻分支：feat/research-news-ingestion
- 新闻检查 HEAD：90a5721954d54b37511c31263c06613010a3aa4a
- 平台只读检查 HEAD：44d10cd6197ca446b4bbf29dff9f62408b32bbfe
- 评审范围：平台架构、正式路线图、包边界、安全凭证、Config/Logger/Diagnostics、HTTP、Storage/Migration、Task、跨分支契约。
- 修改边界：只更新新闻分支 doc/news/**；未对 feat/platform-foundation 写入、提交或切换分支。

## 1. 结论

新闻模块采用根 Go Module 内模块化单体仍然是合适路线。公共平台提供横切能力，News 保留新闻领域所有权。需要修正的是接入计划对平台正式任务编号和能力抽取条件的理解，不能把平台设计文档中出现的能力都当成已承诺、已实现的公共 API。

## 2. 平台最新规划核对

| 能力/任务 | 平台最新规划 | 对 News 的影响 |
|---|---|---|
| PLAT-002 | v1.0.0-review，待接入方评审 | 不直接依赖未冻结 package |
| PLAT-003 | owner Token + 每调用方独立环境 Secret；默认拒绝 | News 同进程 Application 不需要新闻专属认证 |
| PLAT-004 | 动态 Service Key，P2 条件任务，暂缓 | 不进入新闻首期依赖 |
| PLAT-005 | 类型化 Config/Secret | Provider 凭证注入，不进入新闻业务表 |
| PLAT-006 | slog、稳定 Fault、Request ID | News 保留业务错误与部分成功语义 |
| PLAT-007 | 最小 Diagnostics Runner/Result | News 实现领域 Check 与质量规则 |
| PLAT-008 | 统一 HTTP Transport、Deadline、响应体上限、有限幂等重试 | 真实网络调用的基础依赖；还需明确安全 Redirect/Dial 钩子 |
| PLAT-009 | DB/Redis 生命周期和 Migration 目标 | 具体公共 package 以复用方和冻结契约为准；News 不自建重复连接池 |
| PLAT-010 | 长任务持久化与恢复 | 仅有真实异步采集需求时接入 |
| PLAT-011 | Data/News/Research 按需接入 | News 正式跨模块集成阶段 |
| PLAT-012 | 关键故障演练与运行手册 | News 集成后参与必要演练 |

Metrics/Audit 在平台架构/契约材料中被提及，但没有独立 PLAT 编号；不能把它们映射为 PLAT-011/012，也不应虚构 PLAT-013/014。

## 3. 对新闻架构的关键调整

### 3.1 认证不进入新闻模块

新闻没有独立 HTTP Server、Handler 或公网路由。新闻 Application 由根服务同进程调用，因此不引入 Service Key、新闻身份中间件或独立认证。未来由既有 Server API 暴露新闻能力时，使用根服务统一的 owner/service 身份与路由 Scope。

### 3.2 平台存储能力与新闻 Repository 分离

- News 实现新闻 Repository Port、SQL、领域表、索引、保留策略和 Migration 内容。
- Platform 不实现新闻 Repository，不拥有新闻业务表。
- PLAT-009 是否抽出公共 Storage/Migration package，取决于最终契约和真实调用方；平台代码结构明确反对无调用方抽象。
- 根服务组合根可注入现有 DB 资源；全局 Migration 注册入口由服务端集成分支协调。
- Redis 不用于首期 Provider 结果缓存，也不作为新闻事实存储；无跨进程协调需求时不接入。

### 3.3 HTTP 安全是新闻真实抓取的硬门禁

普通超时、响应大小限制和重试不能替代 SSRF 防护。News 必须提供来源授权和 URL 安全策略；底层 Client 必须支持：
- 只允许批准的协议和端口，拒绝凭据 URL；
- 拦截 loopback、私网、链路本地、保留地址及 IPv4-mapped IPv6；
- 初始 URL 和每次 Redirect 逐跳验证；
- 将 DNS/IP 校验结果绑定实际 Dial，防 DNS rebinding；
- 禁止绕过安全策略的代理/自定义 Transport；
- 有界重定向、超时、响应体大小和并发。

PLAT-002/008 未明确底层安全钩子前，只做固定样本、fake Transport 和离线解析，不做任意来源真实抓取。

### 3.4 新闻质量和平台诊断分层

News 负责 quality、freshness、authorization、identity、content completeness 和 relation evidence。Platform 负责 Check 执行超时、有限并发、结果聚合和受保护查询。平台 Check failed 表示执行器异常，不表示新闻内容 invalid。诊断 Attributes 只允许低敏感摘要、稳定 Reason Code 和受控 Evidence Reference，不允许正文/snippet/完整 Provider 响应/Secret。

### 3.5 长任务按实际场景接入

同步搜索和纯解析不进入任务队列。只有定时采集、长耗时采集或进程中断后需要恢复的任务才评估 PLAT-010。平台 Task 只保存执行生命周期元数据；Observation、News Version、Relation Evidence 仍归 News。

## 4. 修订后的新闻执行路线

1. **N1-A 用户评审：** 本次补充报告和原架构评审一起确认；未确认前不进入 N2。
2. **N2 Domain/Application/Provider DTO：** 可立即开发纯领域模型、端口、Provider 协议解析和 fake 契约测试，不依赖平台内部 package。
3. **N3 离线解析与安全策略：** 完成 URL 规范化、授权决策、Feed/HTML 解析、时间标准化；真实抓取等待 PLAT-002/008 安全钩子和来源授权。
4. **N4 Identity/Version/Relation：** 完成身份凭据、hash 冲突核验、转载隔离、版本指纹、关系断言和证据；使用 fake Repository 验证。
5. **N5 公共运行能力接入：** PLAT-002 冻结且 PLAT-005/006/008 对应代码和测试可用后，接入 Config/Secret、slog/Fault/Request ID 和 HTTP Client。
6. **N6 持久化：** PLAT-009 的 DB 生命周期/事务/Migration 契约明确后，News 实现 SQL Repository 和 Migration 内容；集成分支装配 Migration。
7. **N7 Diagnostics/Task：** PLAT-007 可用后接入领域 Check；Task 仅按实际异步需求接入；Audit/Metrics 无独立门禁。
8. **N8 集成验收：** 按 PLAT-011 接入，按 PLAT-012 执行必要故障演练。

## 5. 进入下一阶段的门禁

- N1-A 必须由用户确认。
- N2/N3/N4 的纯业务代码不需要等待平台分支全部完成。
- 真实网络调用、真实持久化和诊断接入分别按 PLAT-008、PLAT-009、PLAT-007 的实际实现验收。
- 任何平台依赖接入前，确认接口冻结、代码存在、测试通过、根服务组装方式明确。
- 不修改平台分支，不因新闻需求提前实现平台通用组件。
- 不为旧 Python/go-core 新闻服务增加兼容层。

## 6. 本轮未做事项

- 未修改 feat/platform-foundation。
- 未修改新闻业务代码、DB Schema、部署配置或根服务 DI。
- 未进入 N2，也未运行 Go 测试。
- 本轮只修订新闻专项规划与跨分支接入说明。
