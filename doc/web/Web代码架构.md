# Web 代码架构

> 本文记录 WebUI 当前代码结构、模块职责、依赖方向、状态所有权和目标架构。代码事实随实现更新；任务优先级与完成进度只维护在 [Web整体规划.md](./Web整体规划.md)，测试结果只维护在 [WebUI提测验收清单.md](./WebUI提测验收清单.md)。

## 1. 架构目标与边界

WebUI 当前采用原生 HTML/CSS/JavaScript ES Modules，构建和测试以 Node 原生测试及 Browser Smoke 为主。架构治理遵循渐进式拆分，不引入 React/Vue、状态管理框架或新的构建体系。

目标不是单纯缩短文件，而是明确：
- 页面/领域模块拥有自己的渲染、交互和请求生命周期。
- 应用入口负责启动装配、全局事件、页面路由和少量跨域协调。
- API 模块负责 HTTP 请求与响应契约，不承担页面渲染。
- 纯函数模块不访问 DOM、网络、localStorage 或页面全局状态。
- 跨模块共享状态有明确所有者，通过参数、访问器或回调传递，不通过互相 import 页面内部实现。
- 任务、行情、个股研究等异步请求必须保留取消、请求代次、连接变更失效和迟到响应隔离。

## 2. 当前目录结构

### 2.1 应用入口与基础能力

| 模块 | 当前职责 | 架构定位 |
|---|---|---|
| `app.js` | 应用启动、全局 state、页面切换、事件绑定、跨域协调及部分页面业务 | 当前组合入口；逐步收敛 |
| `api.js` | API 地址/配置持久化、HTTP 请求及各服务端接口封装 | API 基础设施 |
| `format.js` | 标的/路径归一化、数字/日期/价格格式化、API 错误描述 | 通用纯函数；目前依赖 preferences 的少量配置 |
| `chart.js` | 日 K 指标计算与图表渲染 | 图表领域能力 |
| `theme.js`、`preferences.js` | 主题和用户偏好读写 | 浏览器偏好基础能力 |

### 2.2 领域模块

| 领域 | 文件 | 当前职责 |
|---|---|---|
| 市场 | `market-overview.js`、`market-stocks.js`、`market-activity.js` | 市场总览、行情列表、市场活动展示和交互 |
| 个股研究 | `stock-research.js`、`research-notes.js`、`research-reports.js`、`research-meta.js` | 个股研究资料、研究记录、报告及元数据 |
| 自选股 | `watchlist.js`、`watchlist-groups.js`、`watchlist-sort.js`、`watchlist-batch.js` | 自选股数据、分组、排序和批量联动 |
| 分析任务 | `task-workflow.js`、`task-event-filter.js`、`task-event-format.js`、`recent-tasks.js`、`report.js` | 任务事件校验/过滤/展示、最近任务引用、报告导出；任务运行生命周期仍有较多逻辑在入口 |
| 选股 | `screening-state.js`、`screening-draft.js`、`screening-export.js` | 选股状态、条件/方案、本地结果筛选和导出 |
| 通知与问股 | `notification-state.js`、`notification-preferences.js`、`chat-state.js` | 页面状态派生、通知偏好和问股上下文状态 |
| 告警 | `alert-state.js`、`local-alert-drafts.js` | 告警状态派生与本地告警草稿 |
| 数据中心 | `data-center.js`、`health-check.js`、`existing-api-tools.js` | Provider 能力、诊断/Probe 展示与请求、健康检查和既有 API 工具 |
| 设置 | `settings-draft.js` | 连接配置草稿合并与变更判断；设置页面编排仍在入口 |

以上按当前文件名和导出职责归类，不代表每个领域已经形成完整独立控制器。部分模块仍由 `app.js` 注入状态访问器、DOM root 和回调。

## 3. 当前依赖关系

当前主要装配方向：

```text
HTML / CSS
    |
    v
app.js  (启动、全局 state、页面协调)
    |---- api.js
    |---- format.js / chart.js / theme.js / preferences.js
    |---- market-* / stock-research.js
    |---- watchlist-* / recent-tasks.js
    |---- task-workflow.js / task-event-* / report.js
    |---- screening-* / alert-* / notification-* / chat-state.js
    |---- data-center.js / health-check.js / existing-api-tools.js
    |---- research-notes.js / research-reports.js
```

约束与已知边界：
- `app.js` 是当前主要组合根，直接 import 多个领域模块；这不意味着领域模块可以反向依赖 app。
- 已观察到 `api.js` 依赖 `format.js`，`data-center.js` 依赖 `api.js`、`health-check.js` 和 `format.js`，`format.js` 当前依赖 `preferences.js`。
- 上述是已核实的部分依赖，不作为“全量无循环依赖”的结论。完整 import/export 依赖图和循环扫描列入架构治理验收。
- 新增模块必须使用相对路径显式 import/export；禁止动态拼接模块路径和隐式全局注册。

## 4. 状态所有权与生命周期

### 4.1 当前入口共享状态

`app.js` 的 state 当前混合了多个领域状态，主要分组如下：

| 状态域 | 代表字段 | 目标所有者 |
|---|---|---|
| 应用/行情 | `settings`、`view`、`quote`、`bundleSnapshot`、`bars`、`chartView` | 应用协调器 / 行情工作区 |
| 分析任务上下文 | `taskId`、`runId`、`taskSymbols`、`taskStatus`、`taskEvents`、`selectedTaskEventSequence` | 任务工作流控制器 |
| 任务请求生命周期 | `taskContextGeneration`、`taskHistoryGeneration`、`analysisConnectionGeneration`、`pollGeneration`、`runResultGeneration`、`pollController`、`runResultController` | 任务工作流控制器；必须整体管理 |
| 行情查询 | `quoteQueryGeneration`、`quoteQueryController`、`quoteQueryKey`、`isQuerying` | 行情工作区 |
| 股票搜索 | `stockSearchTimer`、`stockSearchController`、`stockSearchGeneration`、`stockSearchResults` | 股票搜索控制器 |
| 自选股行情 | `watchlistQuotes`、`watchlistQuoteSymbols`、`watchlistQuoteGeneration`、`watchlistQuoteController` | 自选股控制器 |
| 跨域上下文 | `chatStockContext`、`chatStockContextCleared`、`selectedStockSearchItem` | 应用协调器；由所属领域通过接口更新 |

该表是当前结构的归属目标，不代表迁移已经完成。后续每次迁移前，必须补齐字段读写方、写入入口和失效触发点，禁止仅按字段名称搬迁。

### 4.2 异步请求强约束

以下机制属于正确性边界，不能为追求模块独立而拆散：
1. 每类可替换请求具有明确 generation/token，旧请求返回时必须校验代次。
2. 新请求、页面/标的切换、连接配置变更时，按业务规则 abort 旧请求并恢复 loading/button 状态。
3. 任务上下文 generation、任务历史 generation、分析连接 generation 分离，不能合并成单一计数器。
4. 轮询停止、任务切换、结果重载和组件/页面生命周期必须有明确清理路径。
5. 跨标签页 storage 事件更新有效连接配置时，统一进入配置应用与失效流程；损坏配置不得覆盖当前有效配置。
6. 不能把 Playwright route handler 结束等同于浏览器网络取消完成；测试应验证用户可观察状态和迟到响应隔离。

## 5. 目标分层

```text
应用装配层
  app.js / app-coordinator.js
  - 启动、路由、全局生命周期、跨域导航和依赖装配

页面/领域控制器层
  task-controller.js
  watchlist-controller.js
  settings-controller.js
  market-controller.js
  research-controller.js
  screening-controller.js
  notification-controller.js
  - 页面 DOM、事件、业务状态、请求生命周期
  - 通过显式参数/回调与其他领域协作

领域能力层
  task-workflow.js / task-event-*.js / recent-tasks.js
  watchlist-*.js / screening-*.js / research-*.js
  - 可复用的领域规则和数据处理

基础设施层
  api.js / format.js / preferences.js / theme.js / chart.js
  - 网络、格式化、浏览器偏好、图表能力
```

目标文件名只是职责示意，不要求按此一次性创建。优先扩展已有领域模块，只有当模块同时承载多种独立职责且有清晰接口时才新建 controller。

## 6. 模块接口规范

- 纯函数：使用具名 export；输入输出明确；不得读取 DOM、全局 state、网络或浏览器持久化。
- 页面/领域模块：优先采用 `bindXxx({ root, getContext, onXxx })` 或明确的依赖对象；返回有限、稳定的公开方法。
- 状态读取：通过只读 getter/context 获取；状态写入由唯一所有者提供命令式回调，不把整个 app state 暴露给所有模块。
- DOM 边界：模块优先限定在传入 root 内查询；全局导航/弹层等真正全局节点由应用层管理。
- API：领域模块调用 API client，不自行拼接基础 URL；API client 不 import 页面模块。
- 错误：保留结构化错误与取消语义；不要在底层统一吞错或将取消显示为业务失败。
- 持久化：storage key 和数据格式属于兼容契约；迁移不得改 key、版本或清理语义，除非单独规划并测试。

## 7. 迁移策略

1. 先做静态依赖图、函数调用点和 state 读写清单。
2. 先迁移无副作用纯函数，再迁移边界清晰的领域渲染和事件。
3. 任务、自选股、设置等控制器按业务生命周期迁移，不拆散请求取消、代次校验和失效机制。
4. 每批保持 API、DOM ID、CSS class、storage key、用户行为和可访问性语义。
5. 每批增加单元测试和针对性 Browser Smoke；全量 WebUI Checks 与 Browser Smoke 通过后才进入下一批。
6. 迁移完成后删除旧实现，禁止保留长期双实现或只转发的无意义薄层。

## 8. 当前迁移基线

- 当前入口 `app.js` 为 6,256 行；仍承担全局 state、页面装配、多个领域页面协调和任务/行情请求生命周期。
- 当前已存在三十余个 ES Module；任务展示已逐步抽离为 `task-event-format.js`、`task-event-view.js`、`task-event-node.js`，最近任务列表 DOM 由 `recent-task-view.js` 构建。
- 当前基线提交为 `b6781f2cae649390f658d43de74e74307fd40710`。WebUI Checks #41810819659 与 Chromium Browser Smoke #41810819828 均通过。
- JSR-2C2 只迁移单个任务事件节点 DOM 构建；列表排序、筛选、选中/展开状态、事件监听、详情联动和异步请求生命周期仍由 `app.js` 管理，不将 `renderTaskEvents` 整体搬迁。
- 下一阶段转向自选股领域。现有 `watchlist.js`、`watchlist-groups.js`、`watchlist-sort.js`、`watchlist-batch.js` 已分别承担本地 CRUD/校验/存储、分组、排序和批量事务；页面 DOM、交互、批量行情请求生命周期和跨页协调仍集中在 `app.js`。迁移前必须建立完整调用图和 state 读写映射。

## 9. 变更维护规则

- 本文只维护架构事实、模块边界、接口规范和目标结构，不维护任务优先级/完成率。
- 任务排期、当前执行阶段和下一步只更新 [Web整体规划.md](./Web整体规划.md)。
- 测试用例、CI 运行链接、人工验收结果只更新 [WebUI提测验收清单.md](./WebUI提测验收清单.md)。
- Web 专属文档统一放在 `doc/web/`；跨模块公共 API 契约仍维护在上级 `doc/`。


## 10. 任务工作流拆分前的函数与状态边界（2026-11-04）

本节是下一阶段迁移的审查基线，不代表下列函数已经迁出 `app.js`。当前 `app.js` 为 6,391 行、167 个具名函数；任务相关函数按调用职责分组如下。

### 10.1 最近任务历史

| 函数 | 主要职责 | 关键状态/调用 |
|---|---|---|
| `safeReadRecentTasksState`、`safeReadRecentTasks` | 读取并校验本地最近任务 | 不写入任务运行状态 |
| `renderRecentTasks` | 最近任务列表渲染 | 读取 settings；调用历史读取和反馈 |
| `persistRecentTaskHistory` | 保存历史并刷新列表 | 调用 render/read/feedback |
| `persistTaskReference`、`removeRecentTaskReference`、`clearRecentTaskHistory` | 当前任务引用与历史增删 | 读取/更新 taskId、taskAnalysisBase、suppressedHistoryTaskId/Base；清空历史还会推进 taskHistoryGeneration、taskContextGeneration |

### 10.2 任务展示簇

| 函数 | 主要职责 | 关键状态/调用 |
|---|---|---|
| `renderRunStatus` | Run 状态展示 | 读取 runId；目标迁移为显式传入目标节点、状态和标签函数 |
| `renderTaskFlowSummary` | 事件摘要/节点概览 | 读取 taskEventsLoaded、taskId、taskEvents、taskEventsError、lastRunResult；暂留 app.js |
| `renderSelectedTaskEvent` | 当前选中事件详情 | 使用事件对象、DOM 节点和格式化函数；首批迁移候选 |
| `renderTaskEvents` | 事件列表、过滤和展开状态 | 读取 taskEvents、taskEventFilter、selectedTaskEventSequence、expandedTaskEventSequences；更新 selected/expanded 状态并绑定 toggle；暂留 app.js |
| `renderTask` | 任务状态与任务主体展示 | 读取任务身份、类型、状态、标的、结果和页面 view；调用任务引用持久化、Run 状态及摘要 |
| `renderTaskResult` | Run 结果状态展示 | 读取 lastRunResult、runResultReadStatus/Error、runId、view；调用任务引用持久化、Run 状态及摘要 |

展示簇不是无状态纯函数：包含 DOM 更新并读取共享 state。迁移时应作为同一个视图边界设计接口，不能只搬一个 render 函数后让新模块反向 import app.js。

### 10.3 异步请求与轮询生命周期

| 函数 | 主要职责 | 必须共同保护的状态 |
|---|---|---|
| `loadTaskRunResult`、`abortRunResultRequest` | Run 结果加载/取消 | runResultController、runResultGeneration、analysisConnectionGeneration、taskId/runId、taskAnalysisBase、lastRunResult、runResultReadStatus/Error |
| `stopTaskPolling`、`pollTask`、`pollTaskSafely` | 轮询、事件分页、状态恢复 | pollTimer、pollController、pollGeneration、taskContextGeneration、analysisConnectionGeneration、taskEventCursor、taskEvents、taskStatusLoading/Error/Retrying |
| `createTask` | 创建任务并初始化完整任务上下文 | isCreatingTask、taskContextGeneration、taskHistoryGeneration、runResultGeneration、pollGeneration 及全部任务/事件状态 |
| `activateTaskReference`、`restoreLastTask` | 历史任务恢复和切换 | 任务上下文、事件分页、轮询代次；restoreLastTask 委托 activateTaskReference |
| `invalidateActiveTaskForAnalysisBaseChange` | 分析连接变更时统一失效 | taskContextGeneration、analysisConnectionGeneration、runResultGeneration、任务上下文、事件和轮询状态 |

### 10.4 迁移顺序与阻断条件

1. **JSR-2A：任务展示契约测试修复。** 测试应读取 `task-event-format.js` 的实际模块源码，不再要求已删除的 app.js 旧实现存在；同时保留 app.js 调用点和 CSS class 契约断言。
2. **JSR-2B：最近任务历史边界。** 先核对 `recent-tasks.js` 已有 API，迁移前只补调用关系和存储读写契约，不重复实现存储工具；优先拆无异步请求的列表展示/反馈接口。
3. **JSR-2C：任务事件展示簇。** 将摘要、列表、选中事件和任务主体的 DOM 边界整体设计；状态读写通过窄接口注入，保持 DOM ID、过滤、展开、选中和空态行为。
4. **JSR-2D：Run 结果请求。** 将请求取消、generation、连接代次、任务身份校验和按钮恢复作为一个整体迁移；不得拆成独立 fetch helper。
5. **JSR-2E：任务轮询与事件分页。** 将 pollTimer、AbortController、pollGeneration、taskEventCursor、去重和分页校验作为一个生命周期迁移。
6. **JSR-2F：创建/恢复/连接失效协调。** 最后处理 createTask、activateTaskReference、restoreLastTask 与连接变更失效，避免先拆生命周期再补协调。

每个子批次必须先有迁移前后行为基线，并至少覆盖：任务创建、运行中轮询、终态停止、事件分页与去重、切换历史任务、清空历史、切换分析 API 地址、旧请求迟到响应、Run 结果重载失败/取消后的按钮恢复。禁止将 fixture 测试当作真实 API 联调；本阶段不需要真实 Provider Key。

任何一项出现 DOM 结构/交互变化、请求重复、旧响应回填、轮询未停止、取消被显示为失败或按钮未恢复，立即回退该子批次，不继续叠加迁移。


## 11. 最近任务视图模块

`recent-tasks.js` 继续独立负责本地历史存储、记录规范化、读写确认、去重、筛选和类型标签。DOM 列表展示由 `recent-task-view.js` 负责，接收显式参数：根节点、计数节点、存储状态、筛选条件、当前分析服务地址及日期/状态格式化函数。该模块不读取全局 `state`、`localStorage`，不发起 API 请求，也不绑定事件。

`app.js` 保留薄协调函数 `renderRecentTasks`：读取当前状态和控件值，调用视图模块，处理反馈提示，并在列表变化后刷新研究标的列表。任务移除、清空历史、活动任务引用迁移以及 generation 保护仍留在 app.js，暂不迁移。

迁移验收重点：不可读/损坏存储提示、空列表/无匹配项、旧 API 地址标识、任务类型/状态标签、失败信息、任务打开/移除 data 属性、ARIA 标签和原有 CSS class 均保持不变。


### 11.1 依赖方向修正

筛选仍由 `app.js` 调用既有的 `filterRecentTasks` 完成，再把筛选后的记录及总数传入视图模块。这样 `recent-task-view.js` 不反向依赖存储/数据模块，保持纯 DOM 展示边界；它只接收展示数据，不读取筛选控件或全局状态。


### 10.2.1 JSR-2C 迁移边界（先拆纯展示，不拆事件状态）

JSR-2B 已完成：`recent-task-view.js` 只接收 app.js 计算好的过滤记录和总数，负责最近任务列表 DOM 构建；`recent-tasks.js` 仍负责本地存储、筛选与历史记录操作。提交 `72cff581` 的 WebUI Checks（运行 `41809117356`）和 Chromium Browser Smoke（运行 `41809117453`）均通过。

JSR-2C 采用两层推进，禁止一口气迁移整个事件列表：
1. **JSR-2C1：低风险展示 helper。** 新增 `task-event-view.js`，先迁移 `renderRunStatus` 与 `renderSelectedTaskEvent`。模块只接收 DOM 节点、事件/状态值、`runId`、日期格式化函数及 `task-event-format.js` 导出的状态映射函数；不得读取 app.js state、查询全局 DOM、绑定事件、写 localStorage 或发 API 请求。
2. **JSR-2C2：事件节点列表评估（不预先承诺迁移）。** `renderTaskEvents` 目前会读取并修改 `taskEventFilter`、`selectedTaskEventSequence`、`expandedTaskEventSequences`，计算筛选计数、选择默认事件并绑定 details toggle。只有先把这些状态转成显式输入/回调并证明展开/选中行为一致，才允许单独迁移列表 DOM 构建；否则继续留在 app.js。
3. `renderTaskFlowSummary` 依赖 taskEvents Map、加载/错误状态和 lastRunResult；本批暂留 app.js。`renderTask` 与 `renderTaskResult` 会更新业务 state、持久化引用并触发其他页面刷新，不属于纯展示 helper，本批不迁移。

JSR-2C1 验收必须覆盖：无选中事件时隐藏详情并移除布局 class；选中事件时标题/序号/状态徽标/时间/错误代码/耗时/JSON 详情一致；缺字段不生成空字段；非法耗时不展示；状态未知值仍按既有映射展示；Run 状态无值、未知值和四类 tone 映射一致。DOM ID、CSS class、data 属性、ARIA 属性和 textContent 安全写入保持不变。


JSR-2C1 迁移实现：`task-event-view.js` 负责 Run 状态徽标和选中事件详情 DOM；通过显式参数接收目标节点、数据和格式化/状态映射函数。`app.js` 的同名函数保留为协调包装，只负责查找页面节点、传递当前 runId 和依赖函数。该模块不访问全局 state/document/localStorage，不绑定事件、不发请求。


JSR-2C2（第一步）实现：新增 `task-event-node.js`，仅负责根据显式输入构建单个阶段事件 `<details>` 节点。模块接收 document、事件记录、selected/expanded 状态、日期格式化和状态映射函数；返回 DOM 节点，不绑定 toggle/click 事件、不读取全局状态、不请求 API。`app.js` 继续负责排序、筛选计数、默认选中、toggle 状态集合维护、列表挂载和详情联动。


## 12. JSR-2C2 验收与 JSR-3 自选股迁移预审

### 12.1 任务事件节点构建验收

当前提交 `b6781f2cae649390f658d43de74e74307fd40710` 的 WebUI Checks #41810819659 和 Chromium Browser Smoke #41810819828 均通过。新增 `task-event-node.js` 只接收 document、事件数据、选中/展开状态和格式化/状态映射函数，返回单个 `<details>` 节点。

`app.js` 继续持有事件列表排序、过滤、计数、默认选中、展开状态集合、toggle/click 监听、列表挂载、详情联动及请求生命周期。没有迁移整个 `renderTaskEvents`，也没有拆分 poll/generation/AbortController。

### 12.2 自选股模块当前职责

| 模块/位置 | 已有职责 | 迁移约束 |
|---|---|---|
| `watchlist.js` | 本地自选 CRUD、记录规范化、读写回读确认、损坏数据保护、50 条上限 | 不在 controller 重复实现存储规则或修改 storage key/schema |
| `watchlist-groups.js` | 分组读取/规范化/创建/删除/单条和批量归属 | 分组 key 与异常存储保护保持不变 |
| `watchlist-sort.js` | 按更新时间、代码、名称、价格、涨跌幅、成交额、换手率排序；缺失行情排末尾 | 排序规则保持纯函数，缺失值排序语义不变 |
| `watchlist-batch.js` | 从选股结果批量加入自选，可选分组；失败时回滚自选与分组两个 storage key | 两键事务与回滚必须保留整体 |
| `app.js` | `renderWatchlist`、导入预览/确认、反馈、行操作、批量选择/删除/分组、筛选控件、分组控件、自选行情刷新及跨页联动 | 先划分纯 DOM 构建与协调边界，不直接搬迁整个函数簇 |

### 12.3 自选股状态与跨域边界

- 批量行情状态：`watchlistQuotes`、`watchlistQuoteSymbols`、`watchlistQuoteRequestSymbols`、`watchlistQuoteStatus`、`watchlistQuoteMessage`、`watchlistQuoteGeneration`、`watchlistQuoteController`。
- 批量行情请求需校验请求 generation、AbortController、请求前后自选成员集合、API 配置及迟到响应；连接变更和成员变更必须使旧快照失效。
- 自选数据还被研究标的列表、分析任务创建、选股批量加入、设置页显示字段偏好、浏览器数据导入/导出使用。不能将跨页协调逻辑隐藏进自选页面模块。
- `watchlistImportPreview` 是页面导入流程状态；预览和确认之间需防止旧预览覆盖当前存储状态，需在专项测试中核验。
- 自选页 DOM 模块优先通过显式数据和回调工作；不得读取 app 全局 state、直接访问 localStorage 或自行发起行情请求。

### 12.4 JSR-3 执行门禁

1. 先补齐自选股函数调用关系、DOM 事件入口、state 读写方及失效触发点清单。
2. 先评估从 `renderWatchlist` 抽离单条自选 DOM 节点构建的可行性；传入完整 view model、分组列表、当前选中状态和格式化结果，不让新模块反向 import app.js。
3. 单条节点构建模块不负责本地存储、筛选、排序、行情请求、跨页导航或批量事务；事件监听是否迁移必须明确约定，不能同时在 app.js 和新模块重复绑定。
4. 先覆盖空列表、损坏存储、名称编辑、分组选择、过滤/排序、行情缺失/失败/旧快照、批量选择与操作、导入预览/确认、任务页导入自选、连接变更及迟到响应。
5. WebUI Checks 与 Chromium Browser Smoke 均通过后才进入下一小批；异步行情生命周期未完成整体设计前，禁止创建一个声称拥有完整自选页面的 controller。



### 12.5 JSR-3A：自选股单项视图构建

新增 `watchlist-item-view.js`，导出 `buildWatchlistItemNode`。参数为 document、record、trackingFields、groups、selectedGroup；返回完整自选项 DOM。模块只负责节点创建与 class/data/ARIA 属性，不读取全局状态、不读写 localStorage、不请求 API、不绑定事件。

`app.js` 负责把行情/任务/Run 状态计算成 trackingFields，再按用户配置的顺序与可见字段筛选，调用视图模块。列表根节点继续委托 click/change 事件；名称编辑通过 `data-watchlist-name-symbol` 识别并交由 app 的 `handleWatchlistNameChange` 处理。现有浏览器验收验证了重命名持久化路径，且保留分组、批量选择和 action 按钮 data 属性。

JSR-3A 验收：WebUI Checks #41813659508、Chromium Browser Smoke #41813659580 均通过（提交 `6570f798`）。本次没有修改自选存储 schema、分组事务、筛选/排序规则、行情 API、请求 generation 或跨页联动。

后续拆分顺序应优先评估导入预览解析/展示和辅助字段 view model；不得把批量行情请求生命周期或浏览器数据导入/导出事务搬入 view 模块。
