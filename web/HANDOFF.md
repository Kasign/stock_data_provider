# WebUI 任务交接上下文

最后更新：2026-10-23
用途：在新的 ChatGPT 对话中恢复 stock_data_provider WebUI 产品扩展工作。

## 当前 GitHub 真值（重新验证后记录）

- 仓库：https://github.com/Kasign/stock_data_provider
- WebUI 分支：feat/webui-market-data-console
- 本批开始前 WebUI HEAD：`4b2ae6190a2c7e54cb7628ba2183cf352c642d74`（本批代码提交后应重新核实最新 HEAD）。
- 数据分支：feat/full-go-data-providers-api-switch
- 本批开始前数据分支 PR #1 HEAD：`38193c1e9360e8f7293eabf45403565dad33e594`（Open，未合并；base `main`）。
- PR #2：https://github.com/Kasign/stock_data_provider/pull/2，Open、未合并，mergeable=true；head 是 WebUI 分支，base 是数据分支。
- Issue #3 导航与页面骨架提交 `4b2ae6190a2c7e54cb7628ba2183cf352c642d74` 的 WebUI checks [#39941562678](https://github.com/Kasign/stock_data_provider/actions/runs/39941562678) 已成功，JavaScript 语法、Node 测试、Nginx 镜像构建均通过。
- 数据分支最新 Go test suite [#39941383763](https://github.com/Kasign/stock_data_provider/actions/runs/39941383763)（head `38193c1e`）失败：`services/market-data` 单元测试失败，构建 Go market-data 镜像失败；Go core 和根包测试通过。
- 数据分支 Live Provider probe [#39941383744](https://github.com/Kasign/stock_data_provider/actions/runs/39941383744)（head `38193c1e`）成功；它不是 NAS/1Panel 端到端验证。
- 近期 Go 修复提交：7e47aec66cb000cf403ed0cfbbcbadfe655467a3，修正 Run rejected_request 错误映射为 Task failed，并增加状态映射测试。
- 最新 WebUI 功能提交（Issue #3）：`4b2ae6190a2c7e54cb7628ba2183cf352c642d74`，增加统一导航、hash 路由、页面不可用态和响应式样式；本批 Issue #4 继续增强 Task/Run 运行流抽屉。

## 规划文档和 Issue

- 主任务清单：web/PRODUCT_BACKLOG.md
- 原单股分析计划：web/ROADMAP.md（较早阶段的详细计划保留；新任务执行顺序以 PRODUCT_BACKLOG.md 为准）
- 运行进度：web/PROGRESS.md
- 交接：web/HANDOFF.md
- 全局布局/视觉：[Issue #3](https://github.com/Kasign/stock_data_provider/issues/3)
- 任务详情/运行流：[Issue #4](https://github.com/Kasign/stock_data_provider/issues/4)
- 设置中心：[Issue #5](https://github.com/Kasign/stock_data_provider/issues/5)
- 通知中心：[Issue #7](https://github.com/Kasign/stock_data_provider/issues/7)
- 问股 UI：[Issue #8](https://github.com/Kasign/stock_data_provider/issues/8)
- 选股 UI：[Issue #9](https://github.com/Kasign/stock_data_provider/issues/9)
- 告警中心：[Issue #10](https://github.com/Kasign/stock_data_provider/issues/10)

## 参考库核对

- 参考仓库：https://github.com/ZhuLinsen/daily_stock_analysis
- 此次核对时 main HEAD：ec66bf46caf6993f673bd4be59c883754667b686
- 具体参考：apps/dsa-web/src/pages/HomePage.tsx、SettingsPage.tsx、ChatPage.tsx、StockScreeningPage.tsx、AlertsPage.tsx；components/run-flow/RunFlowPanel.tsx、components/tasks/TaskPanel.tsx；api/systemConfig.ts、agent.ts、screening.ts、alerts.ts；docs/full-guide.md、docs/notifications.md。
- 参考功能包括首页历史/自选/今日工作区、分组设置与配置验证、通知配置和单次测试、多轮问股、策略选股及任务历史、告警规则/触发历史/投递尝试、点击任务查看运行流。
- 不复制参考项目的 React 技术栈；本仓库继续原生 HTML/CSS/JS ES Modules。

## 重要的数据层事实

已有接口：行情健康检查、Provider 清单、实时行情、日 K、筹码/资金流；分析服务就绪、创建 Task、查询 Task、按序号查询 StageEvent、查询单个 RunResult。

没有核实到通用服务器配置写入/密码/.env 管理、通知中心、Agent Chat/会话/SSE、选股策略/候选/历史、告警规则/触发/投递查询、服务端 Task 列表或 Run Flow DAG 快照 API。

因此：
- UI 可先开发，但无后端支持的真实操作必须显示“接口待接入/不可用”，禁用会造成误解的提交操作。
- fixtures 只能用于测试，不允许作为生产页面的模拟通知、模型答案、选股候选或告警触发记录。
- 不能用 localStorage 冒充服务器配置、安全认证、服务器通知或服务端历史。
- 第一项新增功能优先完成统一布局/导航，再完成基于现有真实 Task API 的详情/运行流时间线；之后设置中心与通知 UI，再依次问股、选股、告警。
- 对运行流当前只做真实 StageEvent 线性时间线；没有后端拓扑时不得画虚假 DAG 或虚假进度百分比。

## 每个批次的工作方式

1. 开始前重新查询两个分支 HEAD、PR #2 状态、最近 commits、Actions 状态，不沿用旧状态。
2. 直接读取源码与 Go Handler/领域类型核对契约；不要仅基于 fixture 设计前端。
3. 先计划一批要改的文件与行为，再成组修改；不逐文件提交。
4. 按 Conventional Commit 用中文提交信息，按 feat/fix/docs/test 等类型归类。
5. 提交后核验 commit、PR 文件清单和新 Actions。失败时读日志修复，再跑 CI。
6. 更新 web/PROGRESS.md 与本文件，记录哪些是源码核实、测试覆盖、真实 NAS 联调。
7. 不合并 PR #2，不强推、不重写旧历史。后端功能缺口写成独立契约/issue，不改无关数据源逻辑。

## 下一步执行指令

从 Issue #3 开始：读取当前 HTML/CSS/app.js 全量结构，做一次页面信息架构和布局审查，计划需要修改的模块，然后成组实现统一导航与公共状态组件；新增视图先用明确的 unavailable/empty UI，不加入假数据。之后按 PRODUCT_BACKLOG.md 的顺序推进 Issue #4、#5、#7、#8、#9、#10。每一批都 review、补测试、看 CI，再进入下一批。

说明：ChatGPT 无法在本对话内监测上下文使用量、自动创建并切换到另一个聊天窗口，也不能在用户离线时继续后台执行。此文件作为 GitHub 持久化交接源；下次用户启动新聊天并粘贴交接指令后继续执行。


## 最新续接状态（2026-10-23，重新核实）
- 当前工作分支：`feat/webui-market-data-console`；本批基于 HEAD `f8c8921764a3ccfe9531c0565a8844e0e9c70aba` 修改。更新提交后的 HEAD 和 Actions 必须重新核实。
- 数据分支 PR #1 当前 HEAD：`760be7ebcfaa82302800e879fadacb017577b1e0`；PR #1 Open、未合并，base 是 `main`。
- PR #2 Open、未合并，head 在 WebUI 分支，base 字段当前仍记为数据分支提交 `7e47aec66cb000cf403ed0cfbbcbadfe655467a3`；当前 mergeable=true。不得合并 PR #2，不得强推或重写历史。
- 最新 WebUI checks（本批开始前）：[Actions #39940332400](https://github.com/Kasign/stock_data_provider/actions/runs/39940332400)，对提交 `f8c8921764a3ccfe9531c0565a8844e0e9c70aba` 成功。
- 最新数据分支 Actions：Go test suite [#39940458317](https://github.com/Kasign/stock_data_provider/actions/runs/39940458317) 失败，分别在 `services/market-data` 的 gofmt 检查、market-data Docker 镜像构建步骤；Live provider probe [#39940458334](https://github.com/Kasign/stock_data_provider/actions/runs/39940458334) 成功。这不是 WebUI workflow，WebUI 本批不应触碰数据源业务。
- 参考项目 `ZhuLinsen/daily_stock_analysis` 的 main HEAD：`ec66bf46caf6993f673bd4be59c883754667b686`。已重读 SettingsPage、ChatPage、StockScreeningPage、AlertsPage、HomePage、TaskPanel、RunFlowPanel、full-guide、alerts 与 notifications 文档；继续原生 HTML/CSS/JS，不复制 React 技术栈。
- 本批实现 Issue #3：七项导航与 hash 路由，通知/问股/选股/告警/设置 UI 骨架，本地主题按钮与现有连接设置弹窗链接，响应式样式和 UI 契约测试。新页面只呈现能力未接入状态，不能据此声称 #4/#5/#7/#8/#9/#10 完成。
- 后端 API 缺口维持不变：没有通用服务器配置写入/密码/.env 管理、通知收件箱、Agent Chat/SSE/取消、选股策略/候选/历史、告警规则/触发/通知尝试 API；已存在的 Task/StageEvent/Run API 供 #4 使用。不要新增虚假的 API 路径。
- 验证必须分三类报告：源码/契约核对；GitHub Actions 的 JS check、Node tests、Nginx 镜像构建；实际浏览器截图与 NAS/Docker/OpenResty 联调。CI 成功不代表后两类验收通过。
- 下一步严格按 backlog：先完善 #4 任务详情抽屉/真实事件时间线，再 #5 设置中心、#7 通知中心、#8 问股、#9 选股、#10 告警；每批逻辑成组提交、Review、补测和复查 Actions。


## Issue #4 更新（2026-10-23；本批新提交的 Actions 待回填）
- Issue #3 的 WebUI workflow 已在提交 `4b2ae619` 上通过，执行步骤包括 JavaScript syntax check、Node unit tests 与 Nginx image build；并非真实浏览器截图验收。
- Issue #4 把原有 Task 状态/事件面板迁入可访问的右侧 dialog 抽屉，现有 Task API 继续负责数据。点击最近任务会重新拉取真实状态并打开抽屉；创建新 Task 得到实际 task_id 后自动打开。关闭抽屉不代表取消后端任务。
- Task 和 Run 状态单独呈现；started/finished 时间与错误诊断只读服务端字段。事件仅显示真实返回的耗时/错误码，保留 sequence 排序、after_sequence 增量读取和既有 stale-response generation 防护。
- 本批同时移除重复的侧栏分组标题；未改 Go、Provider、数据源选择或其他业务逻辑。
- 本批开始前重新核实：PR #2 Open/未合并/mergeable=true，head `4b2ae619...`、base SHA 字段仍是 `7e47aec...`；数据 PR #1 Open/未合并，head `38193c1e...`。数据分支最近 Go suite 失败（market-data tests 与镜像构建），Live Provider probe 通过；与 WebUI workflow 分开处理。
- 后续验证分开记录：GitHub Actions 检查 / Node 源级测试 / Nginx 镜像构建；真实浏览器截图；NAS/Docker/OpenResty 与 MariaDB/Redis 联调。
