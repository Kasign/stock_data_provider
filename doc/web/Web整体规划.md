# WebUI 整体规划

更新时间：2026-11-12  
规划基线：feat/webui-market-data-console @ 7f1b8510d349b9d166b6efceddee874ddb7703c0  
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

按当前 HEAD 的 Git Tree 与文件内容核对：
- web/src 下有 51 个 JavaScript 文件。
- web/src/app.js 为 5,574 行，直接静态 import 47 个模块。
- CSS 当前为 web/styles.css 和 web/screening-local-batch.css 两个文件。
- 已有市场总览、个股行情、全市场分页、自选股、个股研究、选股、分析任务/Run Flow、本地研究笔记/报告归档、数据中心、告警草稿、通知偏好和设置中心。
- 当前选股是已加载分页上的本地筛选，不是全市场策略扫描。
- 最近任务是有限数量的浏览器任务引用，不是服务端完整历史。
- 当前 HEAD 尚无本轮对应 CI/Browser Smoke 新验收证据；旧 SHA 通过记录只作为历史证据。

## 6. W0-W5 执行路线

| 阶段 | 目标 | 主要任务 | 依赖/完成条件 | 状态 |
|---|---|---|---|---|
| W0 | 产品规划和文档基线 | 统一目标 IA；梳理页面与服务能力；清理过期排期/路径/状态；更新 API、架构、诊断和验收边界 | 文档内部链接、事实、基线和 Git diff 复核 | 已完成 |
| W1 | 现有工作台收口 | 目标导航映射；明确研究对象/过程/结果/跟踪边界；页面状态和响应式体验收口；认证迁移准备 | 不新增业务 API；不假设 Session 已上线 | W1-A/W1-B/W1-C 已完成；W1-C 当前 SHA 的 CI/浏览器视口验收通过，W1-D 仍有浏览器前进/后退专项待补验 |
| W2 | 研究任务工作区升级 | 真实模板、Run/Stage/Attempt、执行进度、取消、服务端历史分页、结果综合和历史回看 | Researchflow Application Facade、Server 路由、权限及分页/取消契约可用 | 等待依赖 |
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
- [ ] W1-D：浏览器前进/后退与 reduced-motion 专项仍待补验。
- [x] W1-C：研究中心明确结构化 Evidence 等 Research 服务端能力待接入；区分行情资料、本地笔记和 Run 结果副本；核对任务引用、跟踪本地数据及数据中心/设置职责边界。
- [x] W1-D：W1-C 提交 SHA `7f1b8510d349b9d166b6efceddee874ddb7703c0` 的 WebUI Checks（43160148859）和 Chromium Browser Smoke（43160149003）通过，覆盖 320/375/600/768/900/1280/1600px。
- [ ] W1-D：补齐浏览器前进/后退专项；Reduced Motion 测试上下文已启用 `reduce`，仍需核验具体动效行为。

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

2026-11-12：W0/W1-A/W1-B/W1-C 已完成。W1-C 提交 `7f1b8510d349b9d166b6efceddee874ddb7703c0` 的 WebUI Checks（43160148859）和 Chromium Browser Smoke（43160149003）通过，覆盖七个视口和现有主要页面交互；W1-D 仅剩浏览器前进/后退及 Reduced Motion 具体动效专项。
