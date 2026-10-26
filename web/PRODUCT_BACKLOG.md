# WebUI 产品功能扩展与实施任务清单

更新时间：2026-10-26
目标仓库：Kasign/stock_data_provider
实现分支：feat/webui-market-data-console
参考项目版本：ZhuLinsen/daily_stock_analysis，main = ec66bf46caf6993f673bd4be59c883754667b686

## 1. 规划结论

以 daily_stock_analysis 的 Web 产品能力作参考，不复制它的 React 技术栈，也不把其后端能力误认为本仓库已具备。我们的 WebUI 保持 web/ 独立目录、原生 HTML/CSS/JavaScript ES Modules、Nginx 同源代理及 Docker 部署。新功能可以先把页面、导航、表单、状态与交互做好；一旦涉及真实任务提交、配置保存、通知发送、聊天回答、选股候选、告警触发，就必须由真实 API 支撑。没有 API 的地方显示“后端接口待接入”，不得用 mock/fixture 冒充生产返回。

参考源码入口：
- [参考项目主页](https://github.com/ZhuLinsen/daily_stock_analysis)
- [参考 Web 页面目录](https://github.com/ZhuLinsen/daily_stock_analysis/tree/main/apps/dsa-web/src/pages)
- [参考设置页](https://github.com/ZhuLinsen/daily_stock_analysis/blob/main/apps/dsa-web/src/pages/SettingsPage.tsx)
- [参考问股页](https://github.com/ZhuLinsen/daily_stock_analysis/blob/main/apps/dsa-web/src/pages/ChatPage.tsx)
- [参考选股页](https://github.com/ZhuLinsen/daily_stock_analysis/blob/main/apps/dsa-web/src/pages/StockScreeningPage.tsx)
- [参考告警页](https://github.com/ZhuLinsen/daily_stock_analysis/blob/main/apps/dsa-web/src/pages/AlertsPage.tsx)
- [参考任务流面板](https://github.com/ZhuLinsen/daily_stock_analysis/blob/main/apps/dsa-web/src/components/run-flow/RunFlowPanel.tsx)
- [参考配置与 WebUI 说明](https://github.com/ZhuLinsen/daily_stock_analysis/blob/main/docs/full-guide.md)
- [参考通知能力说明](https://github.com/ZhuLinsen/daily_stock_analysis/blob/main/docs/notifications.md)

## 2. 对比结果：参考库有、我们当前还没有的核心能力

| 模块 | 参考库的主要体验 | 我们当前 WebUI | 本项目处理方式 |
|---|---|---|---|
| 全局布局与导航 | 有首页工作区、问股、选股、组合、决策信号、回测、告警、数据中心、用量和设置；有统一 Shell 和移动导航 | 主要是行情工作台、分析任务两个视图，另有连接设置对话框 | 优先完成导航、页面层级和公共状态样式；本轮只规划用户明确提出的功能，不同时复制组合/回测等全部模块 |
| 首页/行情 | 历史、自选、今日工作区；可查询、发起分析、查看任务/历史 | 已有报价、日 K、Provider 能力、自选和最近分析 | 保留已有功能，改善布局与信息密度；服务端历史仍需后端列表 API |
| 设置中心 | 按基础、数据源、AI 模型、通知、Agent、系统/调度等分组；配置 schema、校验、未保存变更保护、密钥遮蔽、测试与备份 | 目前仅有 API 路径和行情 API Key 设置，Key 存在当前浏览器 | 做分组式设置页 UI；本地偏好可本地保存，服务器配置须有真实 API 才可保存；密码/.env 管理不能假实现 |
| 通知 | 设置中有渠道配置/单渠道测试；告警页里还有发送尝试记录 | 没有持久通知中心或通知发送 API | 新增通知中心视图、空/错误/不可用状态和角标规则；没有 API 时不显示虚假未读数或历史通知 |
| 问股 | 多轮对话、会话列表、股票上下文、策略选择、流式响应、停止、导出/复制、追问上下文保护 | 没有 Agent Chat、会话或流式 API | UI 结构可先做，发送操作在真实 Chat API 接入前标记不可用；不返回固定假答案 |
| 选股 | 市场/策略/参数、异步任务、候选股解释、热点主题、选股历史、候选联动单股分析 | 没有策略清单、选股任务和候选/历史 API | 先建设配置/候选/进度页面和状态模型；多股分析不等于条件选股，不可伪造候选 |
| 告警 | 规则列表、创建/启停/删除、单次测试、触发历史、通知尝试诊断 | 没有告警规则与监控调度 API | UI 可先做，规则保存与触发历史须等后端持久化/调度 API |
| 点击任务看运行流 | 任务项可打开 Run Flow；运行摘要、阶段图、事件列表、节点详情、刷新与诊断 | 有 Task 状态和 StageEvent 增量接口，但没有服务端任务列表和 DAG 拓扑快照 | 先实现真实事件时间线与任务详情抽屉；不绘制虚构拓扑或百分比 |
| 视觉与可用性 | 共用设计 token、卡片、抽屉、表单、错误边界、移动端滚动与无障碍 | 已有主题、移动导航和基础 CSS，但页面仍偏密集，app.js 已较大 | 先定统一布局/设计 token，再逐页迁移；保持原生 JS，按功能拆模块，不做大规模技术栈重写 |

## 3. 当前后端能力核实：哪些可以接，哪些必须先做 UI

### 当前已有且可直接接入
- 行情服务健康检查：GET /health/ready
- Provider 清单/能力：GET /api/v1/data-sources
- 实时行情：GET /api/v1/quotes/{symbol}
- 历史日 K：GET /api/v1/bars/{symbol}/daily
- 个股筹码分布与资金流：GET /api/v1/stocks/{symbol}/chip-distribution、GET /api/v1/stocks/{symbol}/capital-flow
- 分析服务就绪：GET /readyz
- 创建异步分析任务：POST /api/v1/analysis/tasks
- 查询单个 Task 与阶段事件：GET /api/v1/analysis/tasks/{task_id}、GET /api/v1/analysis/tasks/{task_id}/events
- 查询单个 Run 报告：GET /api/v1/analysis/runs/{run_id}

已有调用的准确路径和字段继续以 web/API_CONTRACT.md、Go Handler、领域模型和测试为准。

### 当前不存在、需独立后端任务/契约的能力
- 通用服务端配置 schema/读取/保存/字段校验/配置版本冲突、密码与认证、服务器 .env 导入导出。
- Agent 问股、会话历史、流式响应、服务端取消。
- 选股状态、策略列表、扫描任务、候选列表、选股历史和热点主题。
- 告警规则 CRUD、后台评估调度、触发历史、通知尝试查询。
- 通知收件箱/未读状态/通知历史。
- 服务端分析任务列表，以及参考库那种丰富的 Run Flow DAG/拓扑快照。

在这些 API 到位前，页面应展示能力状态、空态和依赖说明；保存/运行按钮应禁用或说明尚不可用。不得只写 localStorage 后声称修改了服务器配置，也不得把 fixtures 送进生产页面。

## 4. 执行顺序与任务 Issue

优先级综合考虑你提出的产品需求、目前可复用的 API、真实数据依赖和全站样式一致性。建议一批完成后 Review、补测、按逻辑成组中文 commit，再复查 CI；不要按文件逐个提交。

### P0：先建立一致的页面骨架

- [ ] **全局布局、导航、响应式和共享状态组件** — [Issue #3](https://github.com/Kasign/stock_data_provider/issues/3)
  - 导航：行情/总览、任务、通知、问股、选股、告警、设置。
  - 共用页面标题、状态徽标、卡片、表格、表单、Toast、抽屉、加载/空/错误/未接入状态。
  - 统一 CSS tokens、间距/字号、桌面密度、移动端菜单和滚动。
  - 页面导航与浏览器返回可预测；关键操作有键盘支持和清晰状态反馈。
  - 视觉验收覆盖 320/375/768/1280+ 宽度；不能只依靠 CSS 字符串测试或 CI 过关。

### P1：优先完成功能上已有数据支撑的任务运行流

- [ ] **点击任务进入任务运行流** — [Issue #4](https://github.com/Kasign/stock_data_provider/issues/4)
  - 点击任务打开详情抽屉/页面，显示 Task 与 Run 的独立状态、ID、标的、时间、当前阶段、错误信息。
  - 基于真实 sequence 增量事件形成时间线，支持手动刷新和刷新后恢复。
  - 处理旧响应覆盖、事件去重、空事件、暂时性与永久性错误。
  - 目前只做线性事件时间线。待后端提供拓扑数据后再加 DAG 图；当前不是服务端任务列表。

### P1：设置中心先搭结构，再逐类别连真实能力

- [ ] **设置页完整分组和配置安全边界** — [Issue #5](https://github.com/Kasign/stock_data_provider/issues/5)
  - 进度：源码与 CI 完成。WebUI checks [#40256618763](https://github.com/Kasign/stock_data_provider/actions/runs/40256618763) 通过（72/72 Node tests、JS syntax、Nginx build）；真实浏览器和 NAS/Docker 联调待做。
  - 连接、基础偏好、数据源、AI/问股、通知渠道、调度、选股、系统安全和备份分组。
  - 未保存提醒、字段解释、校验、加载/错误/冲突/成功状态，密钥默认遮蔽。
  - 仅纯 UI 偏好可本地保存；服务器配置须有 API。密码/.env 管理需后端认证和审计设计。

### P1：通知能力的 UI 和配置入口

- [ ] **通知中心与状态模型** — [Issue #7](https://github.com/Kasign/stock_data_provider/issues/7)
  - 进度：通知列表/详情布局、筛选禁用态和五态模型的源码与 CI 已完成。WebUI checks [#40256904732](https://github.com/Kasign/stock_data_provider/actions/runs/40256904732) 通过（75/75 Node tests、JS syntax、Nginx build）；真实通知 API 尚未接入。
  - 先设计消息列表、分类/状态筛选、详情和空态；顶栏角标只显示真实 API 提供的数据。
  - 通知渠道配置与单渠道测试放在设置中心；告警投递尝试与通知收件箱分开。
  - 没有持久通知 API 时，不生成真实未读数或历史记录。

### P2：问股工作台

- [ ] **问股 UI 与 Chat API 契约** — [Issue #8](https://github.com/Kasign/stock_data_provider/issues/8)
  - 进度：问股工作台、股票上下文隔离与五态框架的源码/CI 完成；WebUI checks [#40257299173](https://github.com/Kasign/stock_data_provider/actions/runs/40257299173) 通过（78/78 Node tests、JS syntax、Nginx build）。Chat API 尚未接入，发送操作保持禁用。
  - 对话、会话、新会话、股票上下文、策略区域、消息复制/导出、流式状态与停止状态。
  - 先实现可审查的 UI 和状态转换；真实发送必须等 Chat API 契约和服务端实现。

### P2：选股工作台

- [ ] **选股表单、运行状态与候选结果** — [Issue #9](https://github.com/Kasign/stock_data_provider/issues/9)
  - 进度：选股工作台与 capability/task/candidate/history 状态模型源码和 CI 已完成。WebUI checks [#40257622418](https://github.com/Kasign/stock_data_provider/actions/runs/40257622418) 通过（82/82 Node tests、JS syntax、Nginx build）；真实选股 API 尚未接入。
  - 市场/股票池/策略/参数、提交前摘要、运行状态、候选解释和历史筛选。
  - 策略列表/候选由后端提供；没有 API 时不能展示假候选，不用普通多股分析冒充筛选引擎。

### P2：告警中心

- [ ] **规则管理、触发历史与通知尝试** — [Issue #10](https://github.com/Kasign/stock_data_provider/issues/10)
  - 进度：规则、评估、触发历史、投递尝试 UI 与状态模型源码/CI 完成。WebUI checks [#40258089134](https://github.com/Kasign/stock_data_provider/actions/runs/40258089134) 通过（86/86 Node tests、JS syntax、Nginx build）；告警后端 API 尚未接入。
  - 规则类型/范围/阈值/级别/启停、单次测试、触发历史和投递诊断。
  - 规则 CRUD、定时监控、冷却/去重、触发持久化及通知发送均需后端。
  - 在真实后台 API 没完成前，把 UI 标记为未接入，不保存虚构规则。

### P3：统一视觉 Review 和部署验收（贯穿以上批次）

- [ ] 先在每次功能提交中自查布局，再在所有页面完成后做全局一致性 Review。
- [ ] 检查桌面信息密度、字号、卡片层级、操作优先级、状态颜色语义、表格可扫读性、移动端触控与滚动、焦点/无障碍。
- [ ] 对真实 API 错误、空返回、后端未启用、网络中断/恢复逐页验收。
- [ ] 在 NAS/Docker/OpenResty 环境验证真实代理、API Key、数据库任务持久化和页面刷新恢复。
- [ ] CI 成功只代表 CI 覆盖的检查通过，不等于 NAS 的真实集成完成。

## 5. 视觉设计与代码结构原则

- 页面目标是“直观、方便、信息能快速扫读”，不是做成信息块很多的监控大屏。一个页面只突出一个主要任务；次要配置分组折叠，危险操作与主操作分开。
- 统一颜色语义：成功/可用、警告/降级、失败/阻断、未知/未接入保持稳定；不要仅靠颜色传达状态。
- 正文以可读为先，辅助文本不能缩小到难阅读；桌面使用清晰的主内容宽度和合理留白，小屏幕切为单列/抽屉。
- 统一所有请求状态：loading、empty、unavailable、error、loaded。错误可以重试时提供重试操作；永久错误不要无限轮询。
- 继续原生 HTML/CSS/JS。不必立刻把整个 app.js 推倒重写；新增页面时按职责分出 pages/views、components、API adapter 与纯函数测试，逐步降低耦合。
- 测试 fixtures 仅用于 CI，不是后端模拟服务，更不是生产备用数据；页面要显示能力不可用时必须如实说明。
- 提交原则：按一组可独立 Review 的逻辑修改使用 feat/fix/docs/test 等类型，中文提交信息；提交后立即查询对应 HEAD 和 Actions 结果。

## 6. 验收门槛（所有功能通用）

每个功能批次需满足：
1. 产品行为和数据契约先明确；UI 只呈现后端真实字段。
2. 完成 UI 所有状态和响应式布局，不留可点击但无效的按钮。
3. 覆盖成功、空、缺字段、错误、未启用、不可用、加载中和重试边界。
4. 更新 API_CONTRACT.md（只记真实接口；建议接口标注为草案）及 PROGRESS.md。
5. 本地/CI 自动化验证通过；按功能需要安排实际浏览器与 NAS/Docker 实测。
6. Review 不再发现阻断问题后才进入下一个批次；PR #2 保持未合并，除非你之后明确决定。
