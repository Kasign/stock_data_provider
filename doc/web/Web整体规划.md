# WebUI 整体规划

更新时间：2026-11-12  
文档核对基线：feat/webui-market-data-console @ 902525b3cf7926df294e153329328c5f52d92809  
范围：web/** 与 doc/web/**。

> 本文是 WebUI 产品路线、任务状态、跨模块依赖和下一步安排的唯一入口。本轮先完成 W0 文档规划，不代表已经实现目标信息架构或接入新模块。

## 1. 产品定位

WebUI 从行情与数据控制台逐步升级为以研究任务为中心的个人 A 股研究工作台。

核心闭环：市场观察 → 研究对象 → 研究任务 → Run/Stage/Attempt 执行 → 证据与量化结果 → 研究结论 → 跟踪与通知。

Web 负责交互、请求编排、状态展示和有限的浏览器本地工作区；业务事实由 Server 暴露的真实 API 提供。Web 不访问模块内部 Repository、数据库或 Provider，不复制数据质量、研究推理、量化策略、告警评估和任务状态机，不提供券商交易或自动下单。

继续使用原生 HTML、CSS、JavaScript ES Modules，不引入 React。现有目录优先复用，不为匹配目标模块名称而机械拆文件。

## 2. 目标信息架构

| 目标导航 | 核心内容 | 当前承载情况 |
|---|---|---|
| 市场观察 | 指数、市场宽度、行业/概念、市场活动、新闻与事件线索 | 现有市场总览、市场活动和个股行情中的市场分页 |
| 研究中心 | 个股/行业/事件研究；Context、Evidence、Event、Claim、Hypothesis、Scenario | 现有个股研究、本地研究笔记/报告；结构化 Research 服务待接入 |
| 量化研究 | 指标、因子、趋势、策略、回测、风险与评估 | 当前只有日 K 派生指标和当前页本地筛选；Quant 服务待接入 |
| 研究任务 | 模板、Run、Stage、Attempt、执行过程、取消、历史与综合结果 | 现有分析任务/Run Flow；Researchflow 完整能力待接入 |
| 跟踪中心 | 自选、研究对象跟踪、订阅、告警与通知 | 自选、告警草稿和通知偏好目前主要是浏览器本地能力 |
| 数据中心 | Data、News、Research、Quant、Researchflow、LLM、Notification 运行状态与诊断 | 当前以行情服务、Provider 能力和探测诊断为主 |
| 系统设置 | Session、安全、连接、模型、通知、数据源和个人偏好 | 当前 10 组设置中部分已实现，服务端配置待接入 |

七个导航是目标信息架构，不要求本轮立即建立七个真实页面。W1 先完成现有页面映射和入口收敛；对应服务 API、权限和数据契约具备后再开发新页面。

## 3. 模块职责与 Web 依赖

| 模块 | 领域职责 | Web 消费原则 | 前置条件/限制 |
|---|---|---|---|
| Platform | Session/CSRF、Fault、Request ID、HTTP Client、Diagnostics 等公共原语 | 经 Server 对外能力消费 | 原语设计不等于登录闭环可用 |
| Server | Root HTTP 入口、组合根、认证和模块 API 注册 | Web 唯一业务 API 入口 | 以实际路由注册和部署验收为准 |
| Data | 行情、证券目录和基础数据 | 消费真实数据 API 与质量字段 | 迁移/认证边界需按实际接入确认 |
| News | 新闻采集、来源、授权、解析和事件线索 | 通过公开查询契约消费 | 不读取 News 内部 Repository |
| Research | Context、Evidence、Event、Claim、Hypothesis、Scenario | 消费公开 DTO/应用用例 | News-Research 只读 Evidence Query 是依赖项 |
| Quant | 指标、因子、趋势、策略、回测、风险与评估 | 展示公开 DTO 和风险决策 | DTO/纯计算能力存在不等于完整 API 可用 |
| Researchflow | 研究任务编排及 Run/Stage/Attempt 状态机 | 任务状态、执行过程和终态的权威来源 | Web 不创建第二套任务状态机 |
| LLM | 模型 Gateway、结构化输出和流式能力 | 经 Server/Researchflow 受控调用 | 流式输出结束不代表 Run 成功 |
| Notification | 订阅、渠道、投递和通知状态 | 展示服务端投递事实 | 本地通知偏好不等于服务端订阅 |
| Portfolio/Alert/Report | 候选领域能力 | 暂不假设为已批准独立模块 | 需领域归属评审 |

## 4. 不可违反的产品与数据规则

1. Run/Stage/Attempt 状态以 Researchflow 为准；Web 不推断终态。
2. Quant RiskDecision=block 是硬门禁；LLM 解释、综合评分和用户偏好不能覆盖。
3. Evidence 保留来源、授权范围、时间、引用定位和原始记录身份；无权访问的内容不能展示为可用证据。
4. 事件时间、源端时间、采集时间、分析时点和可用时间分开表达；PIT 结果展示 as-of 时间和数据可用性边界。
5. unknown 不等于 0；缺失、跳过、未配置、不支持、降级和失败分别表达。
6. Web 不判断 Provider 质量、不计算服务端风险、不执行策略、不评估告警、不生成虚构证据。
7. 自选、笔记、报告归档、告警草稿、通知偏好必须标明本地范围，不伪装为服务端持久化。
8. 不把 Owner Token、服务端密钥或第三方 Provider Key 暴露到浏览器；认证迁移完成前保留当前兼容边界。

## 5. 当前 Web 基线

按本轮代码验收基线的 Git Tree 与文件内容核对：
- web/src 下有 51 个 JavaScript 文件。
- web/src/app.js 为 5,574 行，直接静态 import 47 个模块。
- CSS 当前为 web/styles.css 和 web/screening-local-batch.css 两个文件。
- 已有市场总览、个股行情、全市场分页、自选股、个股研究、选股、分析任务/Run Flow、本地研究笔记/报告归档、数据中心、告警草稿、通知偏好和设置中心。
- 当前选股是已加载分页上的本地筛选，不是全市场策略扫描。
- 最近任务是有限数量的浏览器任务引用，不是服务端完整历史。
- 本轮代码验收 SHA `82460cad6668c34ff6fde103efc28815ee21a02d` 的 WebUI Checks（43160974526）与 Chromium Browser Smoke（43160974640，attempt 2）均通过。

## 6. W0-W5 执行路线

| 阶段 | 目标 | 主要任务 | 依赖/完成条件 | 状态 |
|---|---|---|---|---|
| W0 | 产品规划和文档基线 | 统一目标 IA；梳理页面与服务能力；清理过期排期/路径/状态；更新 API、架构、诊断和验收边界 | 文档内部链接、事实、基线和 Git diff 复核 | 已完成 |
| W1 | 现有工作台收口 | 目标导航映射；明确研究对象/过程/结果/跟踪边界；页面状态和响应式体验收口；认证迁移准备 | 不新增业务 API；不假设 Session 已上线 | W1-A/W1-B/W1-C/W1-D 已完成；当前 SHA 的 CI、浏览器历史导航、Reduced Motion 与七视口验收通过 |
| W2 | 研究任务工作区升级 | 真实模板、Run/Stage/Attempt、执行进度、取消、服务端历史分页、结果综合和历史回看 | Researchflow Application Facade、Server HTTP/DI/Worker/Migration、Session/CSRF 与分页/取消契约可用 | 已完成 Web 侧准备审计；阻塞于 Server/Researchflow HTTP 与认证运行时接入 |
| W3 | 研究中心与量化研究 | Research Evidence/Context/Event/Claim/Hypothesis/Scenario；Quant 指标/因子/趋势/策略/回测/风险 | Research、News Evidence Query、Quant DTO/API、PIT/质量/风险契约及 Server 注册可用 | 等待依赖 |
| W4 | 跟踪与通知 | 服务端订阅、通知渠道、投递状态、告警规则/触发历史 | Notification API 和告警领域归属评审完成 | 等待依赖 |
| W5 | 端到端验收 | 真实数据闭环、权限与证据授权、PIT、风险门禁、失败/降级、历史回看、部署和响应式 | 纳入范围的 API/模块均注册且目标环境可验证 | 待开始 |

### W0 文档任务

- [x] 核对当前分支 HEAD 和 doc/web 文件清单。
- [x] 核对当前 JS/CSS 文件数量及 app.js 规模。
- [x] 建立目标七导航与现有页面/能力映射。
- [x] 更新总体规划、功能说明、代码架构、API_CONTRACT。
- [x] 更新统一诊断计划、提测清单和专项历史文档标记。
- [x] 复核文档内部链接、重复内容、过期“下一步”和 Git diff。

### W1 当前任务状态

- [x] W1-A：核对现有导航、页面容器、视图切换和局部导航。
- [x] W1-A：建立当前 11 个入口到目标七导航的映射。
- [x] W1-A：形成页面职责、导航兼容和分批实施方案，见《WebUI 页面结构调整计划》。
- [x] 用户确认七导航分组；旧 hash 沿用原 view id，market-data 归一到 market。
- [x] W1-B：七个一级导航分组、子视图入口、活动态和深链刷新恢复已实现。
- [x] W1-D：WebUI Checks 与 Chromium Browser Smoke 在 SHA 3845bdc50a52121d6d875b6071f85217ef3d86e1 通过；覆盖七入口、hash 直达/刷新、响应式断点和既有主要交互验收。
- [x] W1-D：当前 SHA `82460cad6668c34ff6fde103efc28815ee21a02d` 的浏览器前进/后退、活动导航状态及 Reduced Motion 断言通过；Browser Smoke（43160974640，attempt 2）。
- [x] W1-C：研究中心明确结构化 Evidence 等 Research 服务端能力待接入；区分行情资料、本地笔记和 Run 结果副本；核对任务引用、跟踪本地数据及数据中心/设置职责边界。
- [x] W1-D：W1-C 提交 SHA `7f1b8510d349b9d166b6efceddee874ddb7703c0` 的 WebUI Checks（43160148859）和 Chromium Browser Smoke（43160149003）通过，覆盖 320/375/600/768/900/1280/1600px。
- [x] W1-D：真实 API 与 Mock 的验收边界已记录在 API_CONTRACT 和提测清单；本轮 Mock 浏览器测试不作为真实服务端联调证据。

### W2 准备审计与执行拆分（2026-11-12）

**当前结论：W2 尚不能进入真实业务 API 开发。** Web 侧现有任务链路已经具备创建旧版 Analysis Task、查询 Task、增量读取事件和查询 Run 结果的能力；Researchflow 模块已有部分 Application Service，但 Root Server 尚未将其作为 Web 可消费的 HTTP API 暴露。先完成接入门禁，不在 Web 分支猜测路径或 DTO。

#### 现有代码事实

| 检查项 | 当前可核对事实 | 对 W2 的影响 |
|---|---|---|
| Web 现有请求 | `web/src/api.js` 封装旧版 POST Task、GET Task、GET Task Events、GET Run；事件按 `after_sequence` 增量读取 | 可复用现有任务过程 UI，但不能视为 Researchflow Run/Stage/Attempt 新契约 |
| 旧版 API 契约 | `doc/api/contracts/分析任务.openapi.yaml` 仅声明旧版 Task 创建/查询/事件；描述中明确当前不包含取消和自动重试 | 不能据此添加模板、取消、历史分页或 Retry 请求 |
| Root Server 路由 | `internal/server/bootstrap/router.go` 只注册旧版 `/api/v1/analysis/tasks...`、`/api/v1/analysis/runs...` 等显式路由，均使用 OwnerAccess；没有 Researchflow HTTP 路由 | 新 Application Service 尚未成为浏览器可调用接口 |
| Root Server 运行时 | `internal/server/bootstrap/bootstrap.go` 当前装配旧版 Analysis Runner/Task Queue；Migration Registry 注册 News、Research、Notification，未注册 Researchflow | Researchflow Migration、Scheduler/Worker 生命周期尚未进入 Server 组合根 |
| Researchflow Application | 已有 CreateRun、GetRun、CancelRun、RetryStage、GetWorkflowTemplate 等用例；模板读取要求精确 ref + version，不提供 latest 回退 | 领域/应用能力可作为 Server Adapter 输入，但方法存在不等于 HTTP 接口可用 |
| Researchflow 运行时 | `doc/researchflow/README.md` 与任务拆分记录指出 Scheduler 运行协调、常驻 Worker Pool、生产 StageExecutor Adapter、跨进程取消通知及真实 MariaDB/并发验收仍未闭环 | 仅挂载 Handler 仍不足以证明 Run 可实际执行、取消和恢复 |
| 浏览器认证 | `web/src/api.js` 使用 same-origin credentials，但分析请求没有 Authorization Bearer；当前 `request()` 也没有 CSRF Token 注入 | Cookie 自动携带不能通过当前 OwnerAccess Bearer 校验 |
| Nginx 代理 | `web/default.conf.template` 将 `/analysis-api/` 直接代理到 Server，没有注入 Authorization；`docker-compose.yml` 也没有 Web 到 Server 的可信凭据注入配置 | 仓库内当前部署链路没有证明浏览器分析请求可通过 Owner 认证 |
| Platform Session | Platform 已有 Session/CSRF 原语，但当前 Server bootstrap/router 尚未装配 Session 登录、Cookie 认证和 CSRF 中间件 | 不能把 Platform 原语存在写成 Web Session 闭环已完成 |

**认证阻塞为 P0：** 当前 Root Server 的 `OwnerAccess` 由 Platform Authenticator 的 Authorization Bearer 认证；浏览器不能通过 localStorage 保存 Owner Token 来绕过此问题。必须由 Server/Platform 完成受控 Session/Cookie + CSRF + 资源授权，或提供经过安全评审的可信代理认证方案。不得在 Web 侧补存 Owner Token，也不得让 Nginx 注入长期 Owner Token 作为临时捷径。

#### W2 分批任务

| 批次 | 任务 | 完成条件 | 当前状态 |
|---|---|---|---|
| W2-A | Server/Platform 认证闭环 | 登录/会话建立、HttpOnly Cookie、CSRF、会话续期/撤销、Owner 身份到应用调用者身份映射、资源授权；OpenAPI 安全声明与真实中间件一致 | 阻塞，Web 不改认证 |
| W2-B | Researchflow HTTP 契约与 Handler | 冻结模板读取/发现、Run 创建/查询、Stage/Attempt/ResultSummary 投影、取消、Retry、历史分页的 DTO、错误、幂等、游标和权限；Root Server 注册真实 Handler | 阻塞，当前只有部分 Application Service |
| W2-C | Server 运行时接入 | Researchflow Migration 注册；Scheduler/Worker/StageExecutor/取消通知按角色装配；Health/Readiness 与优雅关闭可诊断；MariaDB 并发/恢复验收 | 阻塞，不能只接路由 |
| W2-D | Web 研究任务工作区 | 仅按冻结 OpenAPI 接入模板和参数 Schema、Run/Stage/Attempt、服务端历史分页、取消状态与 ResultSummary；本地最近任务引用继续单独标识 | 待 W2-A/B/C 通过 |
| W2-E | 真实联调验收 | 当前 SHA 的 WebUI Checks；真实 Root Server + MariaDB 环境验证登录/CSRF、创建/幂等、事件续读、取消竞态、历史游标、结果摘要、失败/降级/阻断与权限隔离 | 待 W2-A/B/C/D 完成 |

#### Web 分支可独立完成的准备项

- [x] 盘点旧版 Analysis API、Web 请求封装和事件分页行为。
- [x] 对照 Root Server 路由注册，确认现有旧路由与缺失的 Researchflow HTTP 路由。
- [x] 对照 Researchflow Application Service 与模块运行时说明，区分已实现用例和未完成运行时。
- [x] 核对浏览器请求、Nginx 代理与 OwnerAccess 认证方式，确认当前仓库部署链路缺少可验收的浏览器认证闭环。
- [x] 明确 W2-A 至 W2-E 的依赖顺序和验收门禁；不添加推测 API、不改变旧 Task/Run 状态机。
- [ ] 等 Server/Platform 提供可验证 Session/CSRF 与资源授权入口。
- [ ] 等 Researchflow HTTP/OpenAPI、Root DI/Migration/Worker 接入和真实 MariaDB 验收。
- [ ] 依照冻结契约实施 W2-D，不提前构造模板或历史分页接口。

**进入 W2-D 的硬门禁：** OpenAPI 有安全声明且与 Handler 一致；Root Server 已注册真实路由；Session/CSRF/资源授权可在目标部署环境验证；Researchflow Migration、Scheduler/Worker 和生产 Adapter 已满足运行要求；模板、Run、Stage、Attempt、ResultSummary、取消、重试和历史分页均有字段级契约。任一项缺失时，Web 仅修复已确认的前端缺陷，不通过 Mock 冒充真实接入。

### 后续执行门禁

- 功能进入开发前，必须明确用户场景、领域 Owner、服务端契约、数据来源、权限和验收证据。
- API 不完整时，不猜测 URL、字段、枚举、游标和错误语义。
- 跨模块阻塞时，优先完成 Web 分支可独立的页面信息架构、状态表达、错误/空态、可访问性和测试准备。
- 发现后端缺口时记录责任模块，不在 Web 分支实现 Go/数据库/Provider 逻辑。
- 复杂页面先提交拆分和验收方案，未经批准不扩大范围。

## 7. 明确暂缓或不做

- 海外市场、券商交易、自动下单。
- Portfolio、Alert、Report 独立服务模块，等待领域评审。
- 可视化 DAG 编辑器；先消费服务端固定模板和真实状态机。
- 浏览器策略计算/回测引擎、Provider 质量判定、告警后台评估和 AI 工具执行。
- Web 直接访问 Provider、数据库或模块内部 Repository。
- 第二套 Task/Run 状态机、全局 EventBus、全局 Store 或第二套路由。
- 为匹配目标模块名称而大规模重拆 JS/CSS。
- 使用 Mock/Fixture 声称真实服务端或真实 Provider 验收通过。

## 8. 验收与文档维护

- WebUI Checks、Chromium Browser Smoke 必须绑定被测 SHA；历史成功不覆盖新 HEAD。
- 真实 API 联调记录环境、路由/认证状态、响应样例和数据来源；Mock 只证明前端行为。
- 跨模块验收检查 ApplicationPort、DTO、Server 路由注册、权限和部署路径，不只检查 OpenAPI。
- 每批任务结束更新本文和《WebUI 提测验收清单》；专项文档不另立总排期。
- 删除或废弃 API 时同步清理页面入口、API 封装、测试和文档引用。

## 9. 本轮执行记录

2026-11-12：W0/W1-A/W1-B/W1-C/W1-D 已完成。W1-C 提交 `7f1b8510d349b9d166b6efceddee874ddb7703c0` 的 CI/浏览器视口验收通过；W1-D 在提交 `82460cad6668c34ff6fde103efc28815ee21a02d` 增加浏览器前进/后退与 Reduced Motion 断言，WebUI Checks（43160974526）及 Chromium Browser Smoke（43160974640，attempt 2）通过。Browser Smoke 首次 attempt 在既有行情重试断言处超时，重跑通过；记录为测试稳定性观察。
