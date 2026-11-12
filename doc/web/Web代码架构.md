# Web 代码架构

更新时间：2026-11-12  
核对基线：feat/webui-market-data-console @ e43dba62079c872d95919e5412e9c7cad21f477c

> 本文描述当前前端架构事实和目标职责边界，不维护任务排期。

## 1. 当前结构快照

| 项目 | 当前基线 |
|---|---:|
| web/src/*.js | 51 个文件 |
| web/src/app.js | 5,547 行 |
| app.js 直接静态 import | 47 个语句 |
| CSS 文件 | web/styles.css、web/screening-local-batch.css |

以上是指定 SHA 的快照，不作为后续提交的永久常量。旧文档以 54d80936 为基线的统计已被当前值替代。

## 2. 当前逻辑分层

### 2.1 浏览器应用组合根
web/src/app.js 是浏览器端 composition root，负责共享状态初始化、页面导航、领域模块装配、跨页面事件、跨领域事务、连接配置变更和 Task/Run 上下文协调。

app.js 行数和 import 数量本身不是拆分理由。只有出现独立状态所有权、独立生命周期、闭合依赖和可独立测试的边界，才评估迁移。

### 2.2 领域模块
当前职责簇包括：
- API/连接：api.js 等负责 HTTP 请求、路径配置、取消/超时和错误归一化。
- 市场：market-overview.js、market-stocks.js、market-activity.js。
- 个股研究：stock-research.js、图表与指标模块。
- Task/Run：task-workflow.js、task-event-*、task-result-view.js、recent-tasks.js。
- 自选/选股：watchlist-*、screening-*。
- 本地研究记录：research-notes.js、research-reports.js。
- 数据中心：data-center.js、health-check.js、existing-api-tools.js。
- 设置/偏好：theme.js、preferences.js、settings-*。
- 本地告警/通知：local-alert-drafts.js、notification-preferences.js 和状态 ViewModel。

这些前端模块与后端领域模块不是一一对应关系，不代表服务端能力已接入。

## 3. 目标职责边界

| 前端职责 | 主要内容 | 权威状态所有者 |
|---|---|---|
| App Shell | 导航、布局、当前页面、全局反馈、跨页协调 | app.js |
| Market View | 市场行情、市场活动、行情分页 | 市场领域模块；数据来自 Server |
| Research View | 研究对象、证据、事件、主张/假设/情景 | Research 服务端；本地笔记独立 |
| Quant View | 指标、因子、策略、回测、风险和评估 | Quant 服务端 |
| Researchflow View | Run/Stage/Attempt、进度、事件、终态、历史 | Researchflow 服务端 |
| Tracking View | 自选、订阅、告警和通知 | 各领域服务；本地功能需标识 |
| Data Center View | 各模块健康与诊断聚合 | Platform 原语 + 领域诊断语义 |
| Settings View | Session、个人偏好、服务配置 | Server/Platform 管认证；浏览器只保存允许的偏好 |

## 4. 依赖方向

目标依赖：HTML/CSS → app.js（浏览器组合根）→ 页面/领域模块 → api.js → Root Server HTTP API。

1. 页面模块不得访问 Go 内部包、数据库或 Provider。
2. 领域模块不反向 import app.js；依赖通过参数/绑定显式注入。
3. DOM adapter/ViewModel 接收显式输入，不读取全局 state/localStorage，不发请求，不拥有业务状态。
4. 不新增第二套路由、全局 EventBus 或无边界全局 Store。
5. 跨域业务流程由 Server/Researchflow 等业务编排者拥有，不由单一页面模块复制。
6. 共享 API 封装不得抹平不同服务的认证、超时、错误和取消语义。

## 5. 状态所有权与生命周期

- 行情查询、个股研究面板、数据中心探测等请求由对应领域模块持有 AbortController/generation。
- Task/Run 当前身份、跨页面协调和轮询由 app.js 与 Task 模块按现有边界管理；业务终态来自 Researchflow。
- 行情连接与分析连接是不同服务边界，失效请求时不能合并成一个无差别 generation。
- DOM adapter 不拥有状态；纯 ViewModel 不执行副作用。
- 本地状态必须有存储 owner、Schema、容量/冲突策略和失败恢复行为。
- LLM 流式内容是过程输出，不能直接更新 Run 终态。
- Quant RiskDecision=block 是服务端硬门禁，不能由 UI/LLM/用户评分覆盖。

## 6. 认证与 API 边界

Web 只调用 Root Server 对外路由，不直接调用模块内部 ApplicationPort 或 Repository。Web Session/Cookie、CSRF、登录限流和资源授权以 Server 实际接入为准；Platform 提供安全原语不等于端到端认证已完成。

不将 Owner Bearer、模型密钥或第三方 Provider 凭据写入浏览器。当前行情 API Key/localStorage 是迁移期兼容边界；迁移完成以真实 Server Session、Data Secret 注入和部署验收为准，不能凭设计文档提前移除。

## 7. 渐进演进原则

1. 优先扩展现有领域模块，不为目标导航名称创建空壳文件。
2. 新模块必须有明确输入/输出、状态、生命周期和测试边界。
3. CSS 单组件拆分需提供 DOM 作用域、级联和 CSSOM 回归证据。
4. app.js 迁移必须列出调用点、状态读写、请求代际和测试覆盖；不为降低行数机械拆分。
5. 当前静态依赖图结论只适用于本节基线；新增依赖后检查循环和反向依赖。

## 8. 架构验收标准

- 模块 import 方向无循环；领域模块不反向依赖 app.js。
- 异步请求具备取消/超时/代际保护，旧响应不能覆盖新上下文。
- 本地存储具备输入校验、写入回读、并发冲突保护和失败恢复。
- 页面状态覆盖 loading/success/empty/unknown/degraded/failed/not-integrated。
- 业务数据来自 Root Server 真实 API；Mock 只用于前端行为测试。
- Evidence 来源/授权、PIT、Quant 风险门禁和 Researchflow 状态均按服务端契约展示。
