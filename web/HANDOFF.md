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


## Issue #4 首轮 CI 诊断（2026-10-23）
- WebUI checks [#39942197604](https://github.com/Kasign/stock_data_provider/actions/runs/39942197604) 对提交 `395e0f647a7a44fdbb2c06492dfb763fa533bd23` 失败；JS 语法检查通过，Node 测试为 67/68 通过，Nginx build 因测试阶段失败未运行。
- 唯一失败为新增 UI 契约测试将 `after_sequence` 检查在 `app.js` 上执行；该参数在 `web/src/api.js` 的事件请求中组装，所以断言应针对测试已读取的 `apiClient` 源码。此测试误判已在修正提交中更改；必须等待新的 GitHub Actions 成功后才能记为通过。


## Issue #4 第二轮 CI 诊断（2026-10-23）
- 第二轮 WebUI checks [#39942345586](https://github.com/Kasign/stock_data_provider/actions/runs/39942345586) 仍为 67/68 测试通过；唯一失败依旧在新增的源码契约测试中，断言期待 `item.sequence - b.sequence`，而现有事件列表使用 `(a, b) => a.sequence - b.sequence` 排序。不是生产代码排序失败。
- 已把测试改成匹配排序表达式本身；`after_sequence` 检查仍针对 `apiClient`。等下一次 CI 结果再判断通过，不能把语法检查通过误算成整个测试通过。


## Issue #4 验收结果（最新核验）
- 功能提交：`23879eca559a2b9a197d34a7a2fd437f53691894`；WebUI checks [#39942477418](https://github.com/Kasign/stock_data_provider/actions/runs/39942477418) 成功，Node 测试 68/68 通过，JS 语法检查与 Nginx image build 均通过。
- 两次早期失败 #39942197604 和 #39942345586 都是新测试的源代码定位/形参名称假设不准确；已改为核对 `api.js` 的 `after_sequence` 请求参数和真实的 `(a, b) => a.sequence - b.sequence` 表达式，并由成功的 CI 复验。
- 数据 PR #1 已更新至 `366f20df60c24bc04e26193110b00e5ab93d7f0a`（Open，未合并）。当前对应 Go suite [#39942050736](https://github.com/Kasign/stock_data_provider/actions/runs/39942050736) 与 Live Provider probe [#39942050842](https://github.com/Kasign/stock_data_provider/actions/runs/39942050842) 均成功；先前 `38193c1e` 的失败流水线不是最新状态。
- PR #2 仍 Open、未合并；本节记录的功能 head 是 `23879eca`，base SHA 仍是 `7e47aec66cb000cf403ed0cfbbcbadfe655467a3`，在此快照上 mergeable=true。明确禁止合并。
- 运行流只呈现现有 Task、StageEvent、RunResult 的真实数据；时间只读 Task 返回的 started/finished 时间，耗时/错误码只在 event 返回时显示。无服务端 Task list/DAG；最近任务列表仍只是浏览器本地 task/run 引用。
- 自动化通过不代表视觉验收与实机集成：本轮没有实际浏览器截图，也未执行 NAS/Docker/OpenResty 到 Go 服务/MariaDB/Redis 的端到端验证。


## Issue #5 本批交接（2026-10-26，CI 待回填）
- 本批开始时重新核实 PR #2 head 为 `3ece8b0f1802b4979f08e20bf5264d15e2c4989b`，Open、未合并；base SHA 字段仍为 `7e47aec66cb000cf403ed0cfbbcbadfe655467a3`。本批开始前最新 WebUI checks #39942655031 对该 SHA 成功。
- 数据 PR #1 已更新到 `2f247400e18f647120526cdfd37ea5160223b9f0`，Open、未合并；最新 Live provider probe #40256158208 成功，Go suite #40256158177 在开始时仍运行中，另一个较早 SHA `e4595f57` 的 Go suite #40256076386 失败。WebUI 本批不改数据分支。
- Issue #5 建立九个设置分组、现有 health API 的连接检查、现有 Provider 清单快捷导航、连接配置草稿/未保存保护。
- 打开设置时 API Key 不会从 state 填回 DOM；留空保留当前 Key，输入新值才替换，清除本地设置需确认。该 Key 仍只属于浏览器连接配置，不是用户认证，也不会写服务器 .env。
- 通用服务器配置、模型/Agent、通知渠道、调度、选股配置、密码/备份没有真实 API，相关按钮禁用且不写 localStorage。
- 新增 `web/src/settings-draft.js` 与 `web/tests/settings-draft.test.mjs`；`package.json` 的语法检查包含新模块。CI 结果需对本批提交 SHA 核验并回填。
- 真实浏览器/移动端截图、键盘/Escape 走查及 NAS/Docker/OpenResty 联调未执行。


## Issue #5 首轮 CI 诊断（2026-10-26）
- WebUI checks [#40256527920](https://github.com/Kasign/stock_data_provider/actions/runs/40256527920) 失败，72 项 Node tests 中 69 项通过；JS 语法检查通过，Nginx image build 未执行。
- 三项失败都是旧测试对文案或实现形式的绑定：market API key help 没有新的环境变量名说明；close 事件从单行回调变成带状态重置的块回调；旧安全说明已被分组式设置中心取代。已更新断言并在帮助文案补充变量名与“不自动修改服务器配置或生成后端密钥”的界限，仍明确 Key 不回显。
- 修复提交后的 WebUI Actions 必须完全通过后再继续后续功能批次。
