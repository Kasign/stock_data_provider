# WebUI 任务交接上下文

最后更新：2026-10-27
用途：从新对话恢复 stock_data_provider WebUI 产品扩展与阶段 Review。

## 当前 GitHub 真值（2026-10-27；全站易用性 Review 前核实）

- 仓库：https://github.com/Kasign/stock_data_provider
- WebUI 工作分支：`feat/webui-market-data-console`；编辑前 HEAD：`dce4b5b03d8c722fb13f73d72fa767b76ee703ca`。
- WebUI PR #13：https://github.com/Kasign/stock_data_provider/pull/13，标题 `WebUI: improve market readability and refresh diagnostics`，状态 Open、未合并、mergeable；head 与 WebUI 分支一致，base 为 `feat/full-go-data-providers-api-switch`。
- PR #13 最近一次已确认的 WebUI checks：[Actions #40434835833](https://github.com/Kasign/stock_data_provider/actions/runs/40434835833) 成功；该结果只对应当时的 HEAD，不代表本轮新提交已经通过。
- PR #2 已关闭并合并，旧的 PR #2 HEAD 不再作为本轮基线。数据 PR #1 仍是独立变更流。
- 本轮只提交到 WebUI 工作分支，改动限定在 `web/**`；不合并 PR、不更新数据分支、不修改 Go/Provider/数据库/API/Compose。
- 本轮 Review 优先改善长行情页定位与全站信息可读性。真实浏览器与 NAS/Docker/OpenResty 实测仍由后续统一验收。

## 本轮已交付且尚未做运行时验收的事项

- 共享状态契约已添加到数据分支：https://github.com/Kasign/stock_data_provider/blob/5776313e7b50648ea0e7d741c7eb1c02b8d20758/docs/architecture/WEBUI_DATA_STATE_CONTRACT.md。数据层新功能开发前须先阅读它，再对照领域契约与 OpenAPI。
- WebUI 主行情、日 K、股票搜索、市场总览、健康检查、Provider 清单已补充请求序号/generation 与旧响应保护。
- Task/Run 与 analysis_base 绑定；切换分析 API 路径后停止旧轮询、清理旧活动结果，并保留带来源地址的最近任务引用，避免将旧 task ID 发给新服务。
- 统一部署入口调整为根目录 `docker-compose.yml`，服务名和容器名为 `stock-analysis-data`、`stock-analysis-server`、`stock-analysis-web`；依赖服务名通过根目录 `.env` 注入。默认宿主机映射仅绑定 `127.0.0.1:8092` 和 `127.0.0.1:8093`。
- PR #2 描述已更新以反映容器名、根目录 .env 和跨层状态约定。
- 本轮按用户要求没有运行测试、镜像构建或 GitHub Actions。源码 Review 不等于测试通过或真实部署验收。

## 当前数据层缺口与前端行为

现有接口包括行情健康检查、Provider 清单、实时行情、日 K 与部分个股补充数据，以及分析服务就绪、创建 Task、查询 Task、阶段事件和单个 RunResult。

未核实到持久通知收件箱、Agent Chat/会话/流式/取消、选股策略/扫描/候选/历史、告警规则 CRUD/后台评估/触发/投递查询、通用服务端配置写入、服务端任务列表或 Run Flow DAG 快照 API。

这些页面保留可审阅的 UI 和状态模型，但生产页面必须继续显示后端不可用并禁用真实写操作；不能用 localStorage 冒充服务端状态，也不能把 fixture 当成生产数据。各项资源与最小契约要求见 web/API_CONTRACT.md 中的“未接入功能的后端契约需求”。

## 每个批次的工作方式

1. 编辑前重新查询 WebUI 与数据分支 HEAD、PR 状态和最近提交。
2. 直接读取实际客户端、Nginx、Go Handler/领域类型与 OpenAPI，不能仅依据 fixture 推断契约。
3. 先规划一组逻辑完整的修改，再按功能成组提交；不逐文件提交。
4. 更新 web/PROGRESS.md 与 web/TECHNICAL_NOTES.md，维护状态、部署约定和跨层依赖。
5. 按当前用户要求，不运行测试、CI 或镜像构建，并在总结中明确说明；只报告实际完成的源码 Review。
6. 不合并 PR、不强推、不重写历史；后端能力缺口先记录为契约/依赖，不夹带 Go 源码修改。
7. 在主要模块完成后做整体 Review，再由用户统一测试并在 NAS/Docker/OpenResty 上验收。

## 下一步执行指令

继续检查通知、问股、选股、告警和设置模块的当前状态模型、共享状态语义与文档一致性。页面骨架/状态模型已基本存在，真正的能力缺口在后端契约；先将缺口和最小验收要求写清楚，不新增假接口，也不提前启用按钮。之后继续跨模块源码 Review，合适节点汇总问题与后续数据层依赖。

历史续接记录保留在下文以供参考；其中标记的旧 HEAD、旧 PR base 和旧 CI SHA 不是当前 GitHub 真值。

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

## 历史协作约定（2026-10-23；如与顶部约定冲突，以顶部为准）

1. 开始前重新查询两个分支 HEAD、PR #2 状态、最近 commits、Actions 状态，不沿用旧状态。
2. 直接读取源码与 Go Handler/领域类型核对契约；不要仅基于 fixture 设计前端。
3. 先计划一批要改的文件与行为，再成组修改；不逐文件提交。
4. 按 Conventional Commit 用中文提交信息，按 feat/fix/docs/test 等类型归类。
5. 提交后核验 commit、PR 文件清单和新 Actions。失败时读日志修复，再跑 CI。
6. 更新 web/PROGRESS.md 与本文件，记录哪些是源码核实、测试覆盖、真实 NAS 联调。
7. 不合并 PR #2，不强推、不重写旧历史。后端功能缺口写成独立契约/issue，不改无关数据源逻辑。

## 历史执行指令（2026-10-23）

原先的“从 Issue #3 开始”计划已完成页面骨架阶段，不再作为当前下一步。当前下一步以交接文档顶部“下一步执行指令”为准。

说明：ChatGPT 无法在本对话内监测上下文使用量、自动创建并切换到另一个聊天窗口，也不能在用户离线时继续后台执行。此文件作为 GitHub 持久化交接源；下次用户启动新聊天并粘贴交接指令后继续执行。


## 历史续接记录（2026-10-23；以下 SHA/CI 仅为当时记录，非当前状态）
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


## Issue #5 最终验收（2026-10-26）
- 功能提交 `8e34b25574e6d6502f51123510cd1fe900632f17` 的首轮 Actions #40256527920 曾失败，72 项中 69 项测试通过；三处失败是旧源码契约断言与新设置页面/回调的文案和结构不一致。
- 修复提交 `2c09afc56714a4082056db2f8e5e8f75969240f6` 的 WebUI checks [#40256618763](https://github.com/Kasign/stock_data_provider/actions/runs/40256618763) 成功：72/72 Node tests 通过、JavaScript 语法检查通过、Nginx image build 成功。
- Issue #5 源码与 CI 阶段完成。尚缺实际浏览器/移动端截图、键盘/焦点走查和 NAS/Docker/OpenResty 的端到端联调；不得标记为真实环境验收已完成。
- 本节写入时 PR #2 仍 Open、未合并，head `2c09afc56714a4082056db2f8e5e8f75969240f6`；base SHA 字段仍为 `7e47aec66cb000cf403ed0cfbbcbadfe655467a3`。数据 PR #1 当前 head `b927362fc6482863b0085dbaaa5c4a0f156e10c6`，Open、未合并；具体最新 Actions 以 GitHub 页面复核。
- 下一步执行 Issue #7 通知中心 UI：真实 API 未接入，因此筛选、读状态、计数和历史均不得产生虚构结果；缺少 API 时显示可解释的 unavailable 状态。


## Issue #7 本批交接（2026-10-26，CI 待回填）
- 实施前重新核实 PR #2 head `1fa1f423bb627748df9e683648c7c0e28c656103`，Open/未合并；其文档提交 WebUI checks #40256730067 在开始时仍处于运行中。PR #1 head `b927362fc6482863b0085dbaaa5c4a0f156e10c6`，Open/未合并；对应 Go suite #40256492883 与 Live probe #40256492893 均成功。
- 重新读取参考项目当前 `main` 的 `docs/notifications.md` 与页面目录。参考库通知能力涉及渠道、路由、降噪和发送；当前仓库没有收件箱、未读状态/计数及通知投递历史查询接口。
- 本批 UI 仅呈现真实能力缺失：筛选、刷新、详情读写状态操作禁用；不会以“0 条”误报通知为空，不会产生未读角标或模拟记录。
- 新增五态状态模型，当前生产页固定从 `supported:false` 解析为 unavailable。其余状态仅为接入真实 API 后的状态框架和纯函数测试，不表示真实通知端点已经存在。
- 本批源码与 CI 状态待最终 Actions 结果确认；浏览器真实截图与 NAS/Docker/OpenResty 联调未完成。


## Issue #7 首轮 CI 诊断（2026-10-26）
- WebUI checks [#40256844942](https://github.com/Kasign/stock_data_provider/actions/runs/40256844942) 中 75 项 Node 测试 74 项通过；语法检查通过，Nginx build 因测试失败未执行。
- 唯一失败是旧 UI 合约测试对文案的精确匹配：期望“不会展示模拟通知”，而新 UI 写“不会生成模拟通知”。断言已更新为接受这两个表达；后续提交 Actions 仍须全绿后再继续。


## Issue #7 最终验收（2026-10-26）
- 修复提交 `b5ed8af4851c1723b9df5e1e509c6bdb64768a48` 对应 WebUI checks [#40256904732](https://github.com/Kasign/stock_data_provider/actions/runs/40256904732) 全部成功：75/75 Node tests、JavaScript syntax check、Nginx image build 均通过。
- 首轮 [#40256844942](https://github.com/Kasign/stock_data_provider/actions/runs/40256844942) 唯一失败是旧测试精确匹配“不会展示模拟通知”，现已兼容“不会生成模拟通知”文案并通过重跑。
- 当前 PR #2 Open、未合并，已验证 head `b5ed8af4851c1723b9df5e1e509c6bdb64768a48`；base SHA 仍为 `7e47aec66cb000cf403ed0cfbbcbadfe655467a3`。PR #1 Open、未合并，当前 head `b927362fc6482863b0085dbaaa5c4a0f156e10c6`；Go suite [#40256492883](https://github.com/Kasign/stock_data_provider/actions/runs/40256492883) 与 Live provider probe [#40256492893](https://github.com/Kasign/stock_data_provider/actions/runs/40256492893) 成功。
- 通知 API 仍不存在；生产页只会显示 unavailable，不生成未读数/虚构通知。浏览器视觉验收和 NAS/Docker 联调仍未执行。
- 下一项 Issue #8 将继续建设问股界面状态与股票上下文展示，保持发送禁用直到后端 Chat API 契约存在。


## Issue #8 本批交接（2026-10-26，CI 待回填）
- 本批开始核对的 PR #2 head：`af918da0484f872664e0ab1aa099c488b8558cd7`，Open、未合并；PR #2 base SHA 字段仍为 `7e47aec66cb000cf403ed0cfbbcbadfe655467a3`。该 head 的 WebUI checks [#40256998908](https://github.com/Kasign/stock_data_provider/actions/runs/40256998908) 已成功。
- Issue #8 新增问股工作台布局和 Chat 五态状态函数。真实 Chat API、session CRUD、streaming、cancel/stop、策略清单都不存在，所以生产页面只显示 unavailable；发送/停止/策略/会话/复制/导出/重试均禁用。
- 股票上下文只来自当前页面最后一次成功行情查询的 state.quote，进入问股页时显示，且用户可以清除；没有从旧聊天或 localStorage 恢复上下文，也不将其发送至服务端。
- 新增 `web/src/chat-state.js`、`web/tests/chat-state.test.mjs`，package check 纳入新 JS 模块。CI 需对本批提交 SHA 核验后再回填结果。
- 浏览器视觉与移动端实际测试、NAS/Docker/OpenResty Chat API 联调未执行。


## Issue #8 首轮 CI 诊断（2026-10-26）
- WebUI checks [#40257236494](https://github.com/Kasign/stock_data_provider/actions/runs/40257236494) 中 78 项 Node tests 77 项通过；JS 语法检查通过，Nginx build 因测试失败而跳过。
- 唯一失败为上下文安全文案断言：契约测试期望“不会自动复用历史上下文”，而页面首版写“不会复用上次会话的旧上下文”。页面文案已改成同时明确“不会自动复用历史上下文，也不会把旧会话的股票上下文带入新会话”；这是 UI 文案同步，不涉及新后端/API。
- 修复提交后的 WebUI Actions 必须全绿，才能确认 Issue #8 自动化验收通过。


## Issue #8 最终验收（2026-10-26）
- 问股功能提交 `12996063025387818fff969d8d02b674403cd6e9` 引入 Chat UI/状态模型与当前行情上下文；修正文案提交 `0107256d975a6e2a324889d194b95b854480f17e` 同步上下文隔离提示。
- WebUI checks [#40257299173](https://github.com/Kasign/stock_data_provider/actions/runs/40257299173) 成功：78/78 Node tests、JavaScript syntax check、Nginx image build 均通过。首轮 #40257236494 的单条失败是 UI 文案断言不一致，已修复复验。
- 当前仍没有 Chat/session/stream/cancel API。问股页面只展示 unavailable，发送、停止、策略、会话、复制、导出与重试操作禁用。股票上下文只取当前页面真实行情 state，不持久化或跨会话恢复。
- 本节记录时 PR #2 Open/未合并，head `0107256d975a6e2a324889d194b95b854480f17e`，base SHA `7e47aec66cb000cf403ed0cfbbcbadfe655467a3`。PR #1 当前 head `b7bc88499e7491446e5250abfbc3da112012b4b5`，Open/未合并；不得将数据 PR merge into WebUI 或反向合并。
- 真实浏览器/移动端验收与 NAS/Docker/OpenResty Chat API 联调未执行。下一项进行 Issue #9 选股 UI，严禁用普通多股票分析任务伪装选股候选。


## Issue #9 本批交接（2026-10-26，CI 待回填）
- 本批开始时 PR #2 head `a192a040c51c8b71598abee43ccd34c2d2f57466`，Open/未合并，base SHA `7e47aec66cb000cf403ed0cfbbcbadfe655467a3`。问股功能对应 WebUI checks #40257299173 成功（78/78 tests）。
- Issue #9 将选股能力、任务、候选结果、热点和历史分开建模。当前没有真实 strategy/schema/scan/candidate/history API，生产页按 `supported:false` 展示 unavailable，所有写操作与筛选/翻页禁用。
- 新增 `web/src/screening-state.js` 与 `web/tests/screening-state.test.mjs`，覆盖 enabled/disabled/unknown/unavailable/loading/error、任务状态和 collection 状态。生产页面不显示股票代码、候选分数或模拟选股理由。
- 本批 CI 结果待对新提交 SHA 核验后回填；真实浏览器和 NAS/Docker/OpenResty 验收未执行。


## Issue #9 首轮 CI 诊断（2026-10-26）
- WebUI checks [#40257548521](https://github.com/Kasign/stock_data_provider/actions/runs/40257548521) 中 82 项 Node tests 81 项通过；语法检查通过，Nginx image build 因测试失败跳过。
- 唯一失败是旧的通用 UI 契约测试要求文案“不能提交，不会产生模拟结果”，新的选股页面将边界说明具体化为“不能提交扫描，也不会用普通分析任务伪装成选股”。已更新断言，未修改后端或生产逻辑；修复后的 Actions 仍需全绿。


## Issue #9 最终验收（2026-10-26）
- 选股 UI/状态模块提交 `29101f922d716f7cd5c120df1338be86243cc12b`，契约测试修复提交 `c54b8d75137652a242d0f46598d36b8574737ce0`。
- 最终 WebUI checks [#40257622418](https://github.com/Kasign/stock_data_provider/actions/runs/40257622418) 全绿：82/82 Node tests、JavaScript syntax check、Nginx image build 均成功。首轮 #40257548521 的唯一失败是旧通用测试仍要求旧版选股占位文案；已更新测试并复验。
- 选股状态模块分别区分 capability（enabled/disabled/unknown/unavailable/error）、扫描任务状态、候选集合与历史集合状态。当前生产页面全部以 `supported:false` 初始化；无 API 时不生成候选、分数、扫描成功状态或“0 条”结果。
- 本节写入时 PR #2 Open/未合并，head `c54b8d75137652a242d0f46598d36b8574737ce0`；base SHA `7e47aec66cb000cf403ed0cfbbcbadfe655467a3`。PR #1 当前 head `b7bc88499e7491446e5250abfbc3da112012b4b5`，仍 Open/未合并。
- 真实浏览器视觉验收和 NAS/Docker/OpenResty 选股联调未做。下一步进入 Issue #10 告警规则/触发历史/通知投递尝试 UI，所有写操作继续在无 API 状态下禁用。


## Issue #10 本批交接（2026-10-26，CI 待回填）
- 本批开始时 PR #2 head `934ecc4292950ee650432f8f810cfac13b834c74`，Open、未合并，base SHA `7e47aec66cb000cf403ed0cfbbcbadfe655467a3`；最近对应 WebUI checks #40257745847 在开始时排队中。
- 告警页将规则 CRUD、评估结论、触发历史、通知投递尝试分开呈现。当前服务没有规则 CRUD/后台调度/触发/投递 API，写操作与筛选禁用且状态统一 unavailable；不伪造规则或发送结果。
- 新增 `web/src/alert-state.js`、`web/tests/alert-state.test.mjs`，覆盖 capability/evaluation/collection states；生产页默认 `supported:false`。
- 本批 CI 结果需按最新提交 SHA 核验并回填。浏览器真实布局与 NAS/Docker/OpenResty 端到端告警/投递联调未完成。


## Issue #10 首轮 CI 诊断（2026-10-26）
- WebUI checks [#40257878022](https://github.com/Kasign/stock_data_provider/actions/runs/40257878022) 中 86 项 Node tests 85 项通过；JS 语法检查通过，Nginx build 因测试失败跳过。
- 唯一失败是沿用旧文案的通用契约测试要求“不会在浏览器里假保存规则”，而新页面使用“不会假保存规则”。已将断言改为匹配实际不可用边界说明；修复提交的完整 Actions 必须全绿后再记录验收通过。


## Issue #10 最终验收（2026-10-26）
- 告警功能提交 `8e62f77f8baa1b3508ca8cf6feedb082fa49bba9`；更新旧 UI 文案契约后的修复提交 `b159908cc716b7b1036bfc7d5ea764ff68ea23ce`；进一步源码 Review 清理提交 `43b5f4d1b3f9510fd56512c242f9ac8fbc0ddc18` 分离“告警能力”与“规则列表”徽标并去除重复 DOM 状态循环。
- 最新 WebUI checks [#40258089134](https://github.com/Kasign/stock_data_provider/actions/runs/40258089134) 成功：86/86 Node tests、JavaScript syntax 与 Nginx image build 全部成功。首轮 #40257878022 仅有一条旧文案断言失败，修复提交 #40257952929 与最终 Review 提交均已通过 CI。
- 告警页将规则 CRUD、单次评估结果、触发历史和投递尝试分开。当前无相应后端 API；所有规则修改/测试/筛选操作禁用，生产页状态固定为 unavailable，不存虚假规则、不展示模拟触发/发送成功、错误码或耗时。
- 本节写入时 PR #2 仍 Open/未合并，head `43b5f4d1b3f9510fd56512c242f9ac8fbc0ddc18`、base `7e47aec66cb000cf403ed0cfbbcbadfe655467a3`。PR #1 head 已更新到 `8c6eef2dab65d59a02d31e67b03b030b9fa79f9a`，仍 Open/未合并；该数据提交的 Go suite [#40258059581](https://github.com/Kasign/stock_data_provider/actions/runs/40258059581) 失败在 `services/market-data` gofmt 检查和 market-data 镜像构建；Live probe [#40258059378](https://github.com/Kasign/stock_data_provider/actions/runs/40258059378) 成功。这是独立数据分支工作，本 WebUI 批次未触碰数据源代码。
- 浏览器桌面/移动端截图与实际交互验收、NAS/Docker/OpenResty 到告警调度/数据库/通知渠道的端到端联调仍未完成。Issue #3/#4/#5/#7/#8/#9/#10 的源码和 CI 阶段已按顺序推进；尚欠全局视觉 Review 与真实部署验收。


## Issue #11／#12 最新交接（2026-10-26）

- 本轮开始时 WebUI HEAD 为 `c3c7dfdea36c7d4eac0fe363280555efaccd6944`，PR #2 继续保持 Open/未合并。
- 股票搜索：数据分支增加真实证券目录搜索接口，返回证券身份元数据而不是行情；目录缓存、并发刷新、过期目录降级和输入参数边界由 Go 服务处理。前端新增无障碍建议列表、中文输入法支持、键盘/鼠标选择与旧响应保护。部署到尚未包含新 API 的版本时，UI 会明确提示更新服务。
- 数据分支目前 HEAD `f5b719f9c506b64aae63effe71b314a19f7c45ba`；对应 Go suite #40260305214 和 Live Provider probe #40260305265 成功。现有 probe 不代表已经对中文搜索本身做了真实上游核验，需要新增专项 probe 或对实际部署发起一次接口请求。
- Run Flow 对标 `daily_stock_analysis`：当前服务没有真实 DAG snapshot/SSE API，Issue #12 分两步处理：先提升现有真实时间线的摘要、事件/节点详情与错误定位；服务端阶段注册表与 topology snapshot 准备好后，再实现真正节点图。禁止虚构依赖边、百分比或取消成功。
- 当前 NAS 已部署，不再把通用部署重复验证当作阻塞；只记录新功能的具体端点/交互验收结果。PR #2 不合并，不强推、不改写历史；WebUI 只修改 `web/**` 与相关检查配置。


## Issue #12 Run Flow Phase 1 — 最新状态（2026-10-26）

- WebUI 当前分支已包含 Issue #11 中文名称/代码自动补全，提交 `b2a30049e2f70b4009c2ff3ab9179adad299cb9f` 的 WebUI checks [#40260695238](https://github.com/Kasign/stock_data_provider/actions/runs/40260695238) 成功。
- 此批在原生 WebUI 内对现有 Run Flow 添加真实事件摘要、可选状态/时间字段、可点击事件选择、事件详情与原始 JSON；不推断依赖边或进度百分比。
- 完整 DAG Flow 仍待数据层的阶段注册表和持久化 topology snapshot API，不能只在前端仿画视觉图。
- 股票搜索的后端提交 `2118cfb945aaa0562e7b7e6480eb62b84cd68f51`、格式修复 `f5b719f9c506b64aae63effe71b314a19f7c45ba` 的 Go suite [#40260305214](https://github.com/Kasign/stock_data_provider/actions/runs/40260305214) 与通用 Live Provider probe [#40260305265](https://github.com/Kasign/stock_data_provider/actions/runs/40260305265) 均成功；尚未专项核验真实中文搜索结果。
- PR #2 持续保持 Open/未合并；提交前再次核对 HEAD，避免覆盖并行更新。


## 2026-10-26 — 市场概览功能交接

- 行情总览新增核心指数、A 股涨跌分布、行业/概念排行、涨停/跌停/炸板池。
- 新增 src/market-overview.js 与 API client 方法；模块独立加载和错误处理，筛选变更只刷新对应模块。旧服务 404 时提示更新 market-data。
- 后端接口：/api/v1/market/indices、/api/v1/market/breadth、/api/v1/market/sectors、/api/v1/market/limit-pools。展示源端时间和质量字段，不使用 mock 数据。
- 本批未运行测试或主动启动 CI；PR #2 保持 Open/未合并。

## 2026-10-26 — 个股研究功能交接

- 行情页新增公司档案、所属行业/概念、季度财务指标和近期公告。
- 真实端点：/api/v1/stocks/{symbol}/profile、/boards、/fundamentals?year=&quarter=、/announcements?page=&page_size=。
- 财务报告期与披露日期分开显示；公告分页与外链来源验证已接入；has_data=false 和 quality unknown 不转成“没有数据”或数值 0。
- 本批未运行测试或 CI；待用户对已部署 market-data 的 Provider 开关和端点执行统一验证。PR #2 继续保持 Open/未合并。

## 2026-10-26 — 龙虎榜与交易日历交接

- 新增龙虎榜查询、服务端分页，以及点击 A 股记录跳转至现有实时行情和日 K。
- 新增交易日历范围查询、上月/本月/下月快捷切换、来源时间与覆盖数量核对；只展示后端明确的交易日标记，不推断节假日。
- API：/api/v1/market/dragon-tiger、/api/v1/market/trading-calendar。未发布、接口缺失、Provider 不可用和覆盖不完整均有说明。
- 本批未运行测试或触发 CI，PR #2 继续保持 Open/未合并。

## 2026-10-26 — Provider 能力面板交接

- Provider 明细按数据服务返回的能力字段显示已知能力，并提供可展开的能力标签；兼容旧回包中缺少新增 capability 字段的情况。
- 能力缺失/false 不显示为支持，注册状态仍独立展示。
- 数据源请求失败时清除旧计数和归一化版本，避免页面显示过期摘要。本批未运行测试或 CI。

## 2026-10-26 — Run Flow 样式续接说明

- 最近核对的 WebUI 基线：`e93a1e6dd2dc57c9c7d85bba7610d5c8112714c4`；PR #2 为 Open、未合并，分支 HEAD 与 PR head 一致。本轮提交后以远程分支实际 HEAD 为准。
- 本轮围绕现有 `StageEvent` 加强线性时间线视觉：状态徽标、节点连接线、选中态、状态色、事件元信息以及移动端布局。
- 该视觉增强只使用真实的 StageEvent 字段；不增加模拟节点、虚构边、估算进度或取消任务按钮。
- 用户要求暂不运行测试和 CI，本轮遵循此约定；提交附带 `[skip ci]`。


## 2026-10-26 — 行情失败状态的旧数据清理

- 查询开始时显示当前标的的加载状态并清空旧报价；报价请求失败时不再保留先前标的的价格/来源/时间。
- 当前报价与日 K 为独立请求，仍允许单项成功时保留该项真实回包。
- 本批不运行测试或 CI，后续重点验收“先查成功标的 A，再查询报价失败标的 B”的界面状态。


## 2026-10-26 — 行情 Bundle 接入续接

- WebUI 在无自定义日 K 日期范围时优先调用 `/api/v1/bundles/{symbol}`，并显示快照 ID 和分析参考时间。
- Bundle 响应的 `quote` 允许缺失；此时清空报价区但仍呈现 Bundle 的 `daily_bars` 与整体警告。
- 自定义日期范围仍使用旧的实时行情与日 K 请求；旧版本服务只有在 Bundle 返回 404 时才回退到双请求。其他错误不会自动重复调用数据 Provider。
- 本批未运行测试或 CI；建议用户重点测试较旧服务的 404 兼容、Bundle 的缺报价回包与自定义日期路径。


## 2026-10-26 — 全局导航与键盘可访问性

- WebUI 增加跳转主内容链接、可聚焦主区域、页面切换的滚动重置与标题聚焦，以及移动导航 Escape 关闭后的焦点恢复。
- 增加减少动态效果的系统偏好样式。页面导航仍保留既有 hash 路由，不新增路由框架。
- 本批未运行测试或 CI；后续验收重点为键盘 Tab/Enter/Escape、浏览器前进/后退以及 320–600px 的菜单焦点行为。


## 2026-10-26 — 全站响应式一致性续接

- 新增全局视图 max-width 和居中，限制超宽屏的行宽；面板标题、状态徽标和操作区可收缩/换行。
- 问股工作台在 1100px 以下始终用单列布局，会话栏置于对话区之后且不再 sticky；通知过滤器和 feature 卡片按平板/手机尺寸调整。
- 关键视觉验收：320/375/600/768/900/1280/1600px；重点检查标题状态徽标、功能页按钮、通知过滤、问股会话布局和设置导航。
- 本批未运行测试或 CI。


## 2026-10-26 — Bundle 元数据与问题定位

- Bundle 成功时独立展示服务端快照 ID、分析参考时间、采集完成时间、整体质量与 sources 字段；来源列表只呈现后端实际返回值。
- 快照元数据可复制为 JSON，面向问题排查；字段不包含 API Key。
- 新请求和 Bundle 404 兼容路径均清除上一快照的展示状态，避免沿用旧 ID。
- 本批未运行测试或 CI。


## 2026-10-26 — Task / StageEvent 轮询可靠性

- Task 与 events 不再由单个 Promise.all 绑定；任意一端成功时会保留其有效结果。
- events 端点错误时保留已显示的历史事件，显示独立诊断；404/权限类永久失败会暂停重复请求，但 Task 状态仍按 2.5 秒刷新。
- events 可手动重试；每次新建任务或恢复本地 task 引用时清除旧事件错误/暂停标记。
- StageEvent 必须是正整数 sequence 才合并；按 sequence map 覆盖重复事件并只用合法序号推进游标。
- 本轮未运行测试或 CI；待统一验收临时/永久接口错误、任务终态与坏序号回包。


## 2026-10-26 — 个股估值 API 接入

- 个股研究新增估值卡片与独立“读取估值指标”操作，调用 `/api/v1/market/valuations?symbols={canonical_symbol}`。
- 展示 PE TTM、PE MRQ、PB MRQ、PS TTM、PCF TTM 及源端质量、来源时间；缺失/null 不转为零。负倍数保持真实数值，不根据估值指标生成买卖判断。
- Provider 能力清单加入季度财务和估值能力字段。
- 数据分支提交 [`c37270a`](https://github.com/Kasign/stock_data_provider/commit/c37270a41a593488767982e5da819d99ba9f2189) 已将估值路由和响应 Schema 加入 `services/market-data/openapi.yaml`。若部署版本较旧会收到 404，若没有启用估值 Provider 则会收到 503；前端分别展示对应错误。
- 本批未运行测试或 CI。


## 2026-10-26 — 个股筹码分布与资金流

- 个股研究区新增“筹码分布”和“资金流摘要”两个独立按需加载卡片，不与主资料加载捆绑，避免新增请求阻塞现有报价。
- 筹码字段来源于 getChipDistribution：平均成本、90%/70% 成本带、获利比例、成本集中度；后端比例口径为 0–1，前端显示成百分数。
- 资金流字段来源于 getCapitalFlow：主力净流入、5/10 日合计、观测数；金额单位人民币元，缺失字段显示“未提供”。
- 错误、质量、来源时间和质量原因分别呈现。数据分支提交 b02eb41 已同步补齐对应 OpenAPI 路由与 Schema。
- 本批未运行测试、未触发 CI。


## 2026-10-26 — 人气榜 / 飙升榜技术交接

- 市场总览新增“个股人气与飙升榜”，请求 `/api/v1/market/hot-stocks`，类型使用 `popular` 或 `surge`，当前页面请求 20 条。
- 表格记录可点击后进入现有行情查询；统一回调会阻止行情查询进行中时再次启动，并清理旧的中文搜索状态。
- `rank_change` 的正负含义不由 API 契约保证，因此仅展示源端数值；`total` 是本次有界响应记录数；`fetched_at` 是前端可以展示的采集时间，不是上游市场快照时间。
- OpenAPI 说明源端缺少可核验行情快照时间，响应质量可能为 degraded；UI 保留该质量状态和质量原因。
- 详细架构、错误处理和数据解释见 `web/TECHNICAL_NOTES.md`。本轮未运行测试或 CI。


## 2026-10-27 — 市场总览与市场活动可读性批次

- 本轮编辑前 WebUI 分支 HEAD 为 737153756f76de15157024ae63e528deb3b08757，数据分支当时同一 HEAD。本批只更改 web/styles.css、web/tests/ui-contract.test.mjs 和 web/ 内的进度/技术/交接文档。
- 目标：提升市场概况辅助文字可读性，扩大龙虎榜/日历控制触控面积，改善 320–600px 日期日历布局。没有修改 API 路径、数据模型、Go 源码、Provider、数据库或根 Compose。
- 对应 CI 状态应查看本批提交的 WebUI checks；静态样式契约不替代真实浏览器视觉验收。


## 2026-10-27 — 市场总览全局刷新结果反馈

- 全局刷新现在根据五个模块的实际请求结果汇总提示；404、503、401/403、其他读取错误、缺少 API Key 和取消请求各自计数。
- “收到响应”不代表数据质量通过；每个模块的质量状态、原因、来源和时间仍独立显示。
- 新增 web/tests/market-overview.test.mjs，覆盖完整响应、混合失败、缺 Key、取消和未知状态。
- 这批修改仅限 web/**。开始时 WebUI HEAD 为 9fdb97693669510bef8416a86a7a0097e792559d；PR #13 仍 Open/未合并。数据分支在此期间新增的提交仅涉及数据层文件，本批不修改、不合并数据层代码。


## 2026-10-27 — Run Flow 阶段事件筛选

- WebUI 分支在前一批 00f301fc37e530bbb52af054e6c005dcb1239aa9 基础上增加阶段事件筛选：全部、异常/降级、运行中、成功/跳过，并显示当前已收到事件的分类计数。
- 过滤只是视图操作；原始 StageEvent、服务端序号/游标、Task/Run 状态均保持不变。未知状态仍可在“全部”中检查，筛选无匹配会单独提示。
- 新增 web/src/task-event-filter.js 与 web/tests/task-event-filter.test.mjs；把模块加入语法检查并更新产品/技术/进度文档。
- 本批应继续只提交到 feat/webui-market-data-console。PR #13 保持 Open/未合并；数据分支和任何 Go/Provider/数据库/API/Compose 文件均不在此改动范围。
