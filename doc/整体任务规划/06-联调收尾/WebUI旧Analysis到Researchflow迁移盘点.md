# WebUI 旧 Analysis API → Researchflow 迁移盘点

- 仓库：`Kasign/stock_data_provider`
- 核对分支：`repair_all`
- 核对基线：`8be3b57306fad9df39a923945f3df6f11790f683`
- 核对日期：2026-11-24
- 状态：**盘点完成；业务生产链路未就绪；尚未切换、尚未删除旧接口**
- 目标：WebUI 业务任务统一使用 SRV-CTR-001 Researchflow API 和 Owner Session；不保留旧 Analysis API 的浏览器兼容入口，不增加 Bearer/Session 双认证。

## 1. 当前结论

现在存在两套并行业务执行边界：

1. WebUI 的分析任务主流程仍调用旧 `/api/v1/analysis/**` API，并依赖独立 Analysis Base、Task ID、Run ID、事件游标和旧任务状态模型。
2. Root Server 已有 SRV-CTR-001 的 Researchflow Run API，并有对应 WebUI API helper；但页面主流程尚未使用它。
3. Researchflow 生产注册目前只有 `news.search_candidates@1.0.0`。它只能证明 News Search 的 Run/Worker/Stage 基础链路，不覆盖股票分析、批量分析、市场复盘或最终研究报告。

因此，**现在不能只把旧 URL 替换为新 URL，也不能把股票分析请求映射为 News Search**。必须先形成真实、可验收的 Researchflow 股票研究模板和生产 Stage，再迁移 WebUI，最后删除旧 HTTP API。

## 2. 权威边界

### 2.1 新接口

SRV-CTR-001 v1.0.0（FROZEN）定义的唯一 WebUI 业务执行 API：

| 方法 | 路径 | 作用 |
|---|---|---|
| POST | `/api/v1/researchflow/runs` | 创建 Run |
| GET | `/api/v1/researchflow/runs/{run_ref}` | 查询 Run + Stage 聚合状态 |
| POST | `/api/v1/researchflow/runs/{run_ref}/cancel` | 提交取消意图 |

认证：Researchflow Handler 只接受经现有 SessionAuthenticator 验证的 Owner Session Cookie；拒绝 Authorization Header，Session Cookie 与 Authorization 同时出现也拒绝。POST 复用 Host/Origin/CSRF 校验。CallerRef 从可信 Principal 获取，资源授权由 Researchflow Application 执行。

契约权威文件：`doc/server/contracts/SRV-CTR-001-researchflow-http-api.md`；OpenAPI：`doc/server/openapi/researchflow.openapi.yaml`。

### 2.2 不应混淆的认证范围

- SRV-CTR-001 Researchflow 业务路由：仅 Owner Session。
- Root Server 现有部分 Data/运维路由：仍有单独的 Owner Bearer/Owner Session 路由策略；不能把它们的认证矩阵复制到 Researchflow。
- SRV-CTR-002 当前文档版本为 1.2.0，状态标记为 freeze-ready；其浏览器凭据规则是 HttpOnly Session Cookie，CSRF Token 仅存内存。具体契约状态以权威文档后续冻结记录为准。
- 不得为旧 Analysis API 增加 Session 兼容认证；也不得把旧 Bearer 认证搬进新的 Researchflow 页面调用。

## 3. WebUI 实际调用盘点

### 3.1 旧 API 封装仍在使用

文件：`web/src/api.js`

| 函数 | 当前请求 | 旧调用语义 |
|---|---|---|
| `createAnalysisTask` | POST `/api/v1/analysis/tasks` | 提交分析任务，携带旧 Idempotency-Key Header |
| `getAnalysisTask` | GET `/api/v1/analysis/tasks/{task_id}` | 查询 Task 状态 |
| `getAnalysisTaskEvents` | GET `/api/v1/analysis/tasks/{task_id}/events?after_sequence=...&limit=100` | 按事件序列增量读取事件 |
| `getAnalysisRun` | GET `/api/v1/analysis/runs/{run_id}` | 读取分析结果 |

这些封装当前仍是旧 API，不是 SRV-CTR-001 的别名。

### 3.2 页面主流程仍依赖旧 API

文件：`web/src/app.js`

- 页面提交入口在约 3803 行调用 `createAnalysisTask`。
- 任务轮询同时调用 `getAnalysisTask` 和 `getAnalysisTaskEvents`。
- 结果读取调用 `getAnalysisRun`，并校验旧响应中的 `task_id`、`run_id`、`symbols`。
- 页面状态包含 Task ID、Run ID、Task Status、Event Cursor、事件列表、事件过滤、事件展开、旧 Analysis Base 和按 Task 保存的本地历史。
- 旧流程会区分创建超时的 unknown outcome，避免盲目重复提交；迁移时必须保留幂等键和未知提交结果处理，不能退化成简单失败提示。
- 页面当前仍把 Task 事件作为执行详情的重要数据源；SRV-CTR-001 首期没有事件流/Attempt 明细接口，改造后应以 Run/Stage 聚合轮询为准，重新定义执行详情展示，不能伪造事件。

### 3.3 新 API helper 已有但没有接入页面主流程

文件：`web/src/api.js`

- `createResearchflowRun`
- `getResearchflowRun`
- `cancelResearchflowRun`

三个 helper 已指向 SRV-CTR-001，并在 POST 使用当前内存 CSRF Token。当前 `app.js` 未导入/调用这些 helper。**Helper 存在不代表页面已迁移或端到端已接通。**

## 3.4 旧 Task API 的实际后端执行链

旧 Task API 不是单纯的 HTTP 转发。Root Server 当前仍直接装配一套旧 Analysis Runner 和异步队列：

- `internal/server/bootstrap/bootstrap.go`：通过 Data Application 创建 `servermarketdata.NewAnalysisCollector`，然后调用 `service.NewRunnerWithDataCollector`。
- 同一 Bootstrap 创建 `platform/taskqueue.New(store.DB, runner, ...)`，并注册 `legacyAnalysisWorker` 生命周期。
- API 侧通过 `platform/httpapi.NewWithTasks(runner, store, analysisTimeout, tasks)` 提供旧同步 Run、Task 创建、Task 状态、Run 结果和事件读取。
- Worker 直接执行同一旧 Analysis Runner；Task Queue 在 MariaDB 持久化 Task/Run/事件，并在启动时恢复/终结中断任务。

旧 `RunRequest` 的业务类型包括：

- `post_market_scan`
- `watchlist_scan`
- `position_review`
- `historical_replay`

页面当前 `createTask` 只提交 `task_type` 和 `symbols`，其余领域参数由旧 API 的 `fillDefaults` 填充。旧 Runner 当前的默认策略为 `trend-following`，包含 MA20 距离、5/20 日收益等因子；旧硬风险规则 `unverified-tradability-block` 会因 ST/停牌/涨跌停、可成交量、组合仓位与账户权益尚未验证而阻断交易计划。历史回放也会因上游缺少逐条 available_at、Provider 和复权口径而 fail-closed。

**迁移含义：** 旧 Runner/TaskQueue 是现存业务链，不应因新 API helper 已存在就删除；但也不能把它直接包装成一个“通用 Researchflow Stage”来掩盖旧模块职责。迁移前必须逐项核对上述四类 Task 的输入默认值、实际计算阶段、输出字段、硬风险规则和数据时间语义，并决定哪些领域能力应迁入 Data/Quant/Research Owner Facade，哪些旧逻辑应淘汰。特别是不能把旧默认值（如 `paper-unconfigured`、`not-configured-v1`）直接当作新生产模板参数。

### 旧 Task 类型的实际差异

- WebUI 当前只开放 `post_market_scan` 和 `watchlist_scan`；`position_review` 在表单中 disabled；`historical_replay` 没有出现在当前创建表单中。
- 旧 Runner 对四种类型均接受，但在当前 `internal/analysis/app/runner.go` 与 `internal/service/wiring.go` 中，只有 `historical_replay` 触发专门的 PIT 阻断分支。
- `post_market_scan` 与 `watchlist_scan` 当前都使用同一个 Universe Resolver、DataCollector、Quality、Market、Factor、Strategy、Risk、Planner 流程；没有发现按这两个 TaskType 分叉的专属业务阶段。它们目前主要是请求元数据/产品命名差异，不能在迁移时直接假定为两个不同的计算引擎。
- 因此阶段 A 的首要工作不是立即创建两个模板，而是确认这两个页面选项是否本来就应合并为一个“指定标的研究”任务，以及盘后市场复盘是否需要独立市场广度/指数/全市场输入链。只有真实输入与输出不同，才定义不同 Template。

### 独立 Go Core 的边界

`go-core/cmd/analysisd` 与 `go-core/internal/transport/http/server.go` 还暴露 `/internal/v1/analysis/stock`、`stocks`、`market-review` 三类内部工作流端点；它们由独立的 StockFlow/MarketReviewFlow 组合，并依赖 Python Bridge 和独立配置。当前 Root Server 的 `/api/v1/analysis/tasks` 并不直接调用这组 go-core HTTP 端点，而是走本节上述 `internal/analysis` + `platform/taskqueue`。后续盘点或迁移不得把两套旧实现混为同一个当前生产调用链。

## 4. Root Server 路由盘点

文件：`internal/server/bootstrap/router.go`

当前旧路由仍注册并交给 legacy handler，认证策略是 Owner Bearer：

- `POST /api/v1/analysis/runs`
- `GET /api/v1/analysis/runs/{run_id}`
- `GET /api/v1/analysis/runs/{run_id}/events`
- `POST /api/v1/analysis/tasks`
- `GET /api/v1/analysis/tasks/{task_id}`
- `GET /api/v1/analysis/tasks/{task_id}/events`

Researchflow 路由在 Owner Session 启用且 Researchflow composition 已装配时注册：

- `POST /api/v1/researchflow/runs`
- `GET /api/v1/researchflow/runs/{run_ref}`
- `POST /api/v1/researchflow/runs/{run_ref}/cancel`

Researchflow Handler 在 `internal/server/bootstrap/researchflow_http.go`。当前 Handler 对 Authorization Header fail-closed，不接受 Bearer/Service Token，也不允许 Session 与 Authorization 混用。

旧 API 路由必须在真实业务迁移、调用方清零和测试迁移完成后整体删除；不得只隐藏 UI 菜单而保留旧接口，也不得为迁移期间新增双认证。

## 5. Researchflow 生产能力现状

### 5.1 当前生产注册

Server 生产组合根：

- `internal/server/bootstrap/researchflow_execution_composition.go`
- `internal/server/bootstrap/researchflow_composition.go`
- `internal/server/bootstrap/news_search_workflow_template.go`
- `internal/server/bootstrap/news_search_stage.go`
- `internal/server/bootstrap/researchflow_template_prerequisites.go`

当前只有一个生产 Template：`news.search_candidates@1.0.0)，一个 News Search Stage Executor，以及一条精确 Stage Output Registry Entry。未知 Stage Code 明确失败，不存在通用占位 Executor。生产参数 Schema 和 Applicability Rule 也仅覆盖 News Search。

### 5.2 现有可复用构件与未闭环能力

| 领域 | 已有构件/Owner 入口 | 生产接入缺口 |
|---|---|---|
| News | 真实 News Search Provider、Stage Adapter、Validator、Template/Executor/Output Registry | 已装配；只覆盖候选新闻检索，不代表完整股票分析 |
| Research | Context Application 的 CreateContext/GetContext、Context Schema/Storage；Researchflow 有 DTO 映射和 Context Assembly | 缺可信 CallerRef/当前授权 Adapter、Run.SystemAsOf 一致性校验、Server/Worker Stage 接线 |
| Quant | Quant GetResult Application、Trend/Risk Envelope；Researchflow 有 Envelope 校验/映射 | 缺完整生产计算 Facade/Stage Provider/结果生产与持久化接线；读取必须走 Quant Application 授权和审计，不能访问 Repository 旁路 |
| LLM | Provider.Generate/Stream、ModelRequest、Researchflow 预算/Tool Policy/结果映射 | 缺 Server 生产 Provider 注入、真实调用链和目标模型 Tokenizer；不能把估算器冒充生产 Tokenizer |
| RiskDecision | Quant 的 `risk.Decision` 是 Quant 领域结果 | Researchflow Composer 仍接收独立 RiskDecision；唯一来源、字段映射、多结果聚合和关联身份未解决（RF-BLOCK-008） |
| Data | 现有 Data API/数据服务 | 尚无满足 Researchflow Stage 输入、快照、来源、时间与质量契约的可注册 Data Stage |
| Notification | 已有 Run 终态 Outbox/Relay 相关能力 | 终态通知不应塞入同步 DAG；按独立事件消费链路接入 |

Researchflow 的 Context Assembly、Quant Envelope Validator、Result Composer、Diagnostics Adapter 等内部构件，不等于真实生产 Stage。只有 Owner Facade、授权、输入输出 Schema/Golden、真实 Provider、Stage Executor、Registry 和 Server Worker 注入全部齐备，才能注册生产模板。

## 5.3 当前 WebUI 结果页依赖的完整报告字段

文件：`web/src/task-result-view.js`、`web/src/report.js`

旧页面不是只显示 Run 状态。结果页会直接读取旧 `RunResult` 的：

- Request：symbols、task_type、market_id、as_of、as_of_mode、strategy_ids、策略/风险配置版本。
- 快照与运行元数据：universe、raw_snapshot_id、market_snapshot_id、started_at、finished_at。
- 数据质量：quality.core_pass、partial、issues、strategy_eligible、strategy_blockers、policy_version。
- 分析结果：market_state、factors、signals、tradability、risk、plans、planning_rejections、execution_assessment。
- 执行过程和结论：events、failure、warnings。

导出 Markdown/JSON 也以完整旧 RunResult 为数据源。SRV-CTR-001 的 Run/Stage 聚合 DTO 不包含上述完整报告正文；契约明确首期不返回 Stage Output 内容和 Attempt 明细。当前 `ResultSummary` 也不能被假定为完整研究报告。

**这是独立于 Stage 生产链路的结果读取阻塞：** 在切换 WebUI 结果页前，必须明确完整报告/结果资源的唯一持久化 Owner、资源 ID 与授权读取 Application Port，并定义 WebUI 可用的正式读取契约。不能让页面直接访问 Quant/Research/Storage Repository，也不能在 Run 聚合 DTO 中随意塞入各模块完整结果。

## 6. 旧 Task 模型与新 Run 模型的语义差异

| 旧页面概念 | SRV-CTR-001 当前能力 | 迁移处理 |
|---|---|---|
| Task ID | 无 Task 资源；以 Run Ref 标识 | 页面状态和本地历史改为 Run Ref |
| Run ID | Run Ref 本身 | 统一只保留 Run Ref，不维护两个 ID |
| Task Status | Run Status + Stage Status | 由 Run/Stage 聚合状态驱动 UI |
| Task Event Cursor / Events | 首期没有事件流接口 | 移除事件 API 依赖；按 Stage 状态轮询，详情只展示已返回字段 |
| Attempt 明细 | 首期不返回 | 不伪造 Attempt；需要时另行评估新契约版本 |
| Run Result 旧响应 | ResultSummary + Stage 聚合 | 以正式 DTO Schema 渲染；结果正文/报告必须由已授权的领域结果能力提供，不能假设 Summary 包含完整报告 |
| 取消任务 | Cancel Run，返回最新聚合状态 | UI 明确显示“取消意图已提交”，等待 Worker/Finalizer 的最终状态 |
| analysis_base | 同源 Root Server API | 新流程移除独立 Analysis Base 配置；旧历史数据如何处置需在切换前定案 |
| Idempotency-Key Header | CreateRun DTO 的 `idempotency_key` | 生成稳定键并写入请求体；超时后重试必须复用同一个键 |

## 7. 迁移前置条件（未满足不得删除旧功能）

1. **业务范围确认**：明确首批要替代的旧 Task 类型（单股分析、批量分析、市场复盘分别定义），不得用一个模板暗含不同语义。
2. **领域生产者就绪**：Data 输入、Quant 计算、Research Context/Evidence、LLM 生成、风险门禁的 Owner Facade 和真实 Provider 均可用。
3. **契约齐备**：每个 Stage 有冻结参数 Schema、Output/QualitySummary Schema、Golden、来源/PIT/版本语义、失败/降级/重试策略和授权规则。
4. **Run 生产装配**：Template/Stage Executor/Output Registry/Applicability/必要 HardRiskGate Registry 由 Server 组合根真实注入，Worker 能在 MariaDB/Redis 生产式运行。
5. **结果闭环**：确定 ResultSummary 与完整报告/Quant Result/Research Evidence 的唯一持久化 Owner 和授权读取路径；不能让 WebUI 直接读取模块 Repository。
6. **风险语义闭环**：解除 RF-BLOCK-008，Quant Decision 为唯一权威来源；硬风险门禁不得被总评分覆盖。
7. **端到端验收**：真实创建、幂等重放、状态轮询、取消、资源授权隔离、CSRF/Origin/Host、失败与部分成功、PIT、结果回读和 Worker 恢复。
8. **历史数据策略**：明确旧 Task/Run 历史引用的展示、只读保留或迁移方案；不得把旧 Task ID 伪装成新 Run Ref。
9. **调用方清零**：WebUI 页面、模块、脚本、测试、文档和部署配置不再引用旧 Analysis API/Analysis Base。
10. **清理验收**：删除旧 API helper、旧路由、旧 OpenAPI/配置/环境变量和无消费者代码；完整 CI 与部署冒烟通过后才标记切换完成。

## 8. 建议实施顺序

| 阶段 | 工作 | 主要 Owner | 退出条件 |
|---|---|---|---|
| A | 明确旧任务类型、请求参数、旧结果结构和报告字段的业务映射 | WebUI + Server + Researchflow + 各领域 Owner | 每种任务有可验证目标业务链和结果契约 |
| B | 补齐 Data/Quant/Research/LLM 真实生产能力与风险语义 | Data / Quant / Research / LLM | Owner Facade、授权、真实 Provider 和数据证据可验收 |
| C | 建立对应 Researchflow Template/Stage/Registry 和 Server Worker 装配 | Researchflow + Server | 真实 Run 能完成并产生持久化结果 |
| D | WebUI 页面切换为 Run Ref + Run/Stage 聚合轮询 | WebUI | 页面不再依赖 Task/Event/Analysis Base，真实业务可操作 |
| E | 历史任务处理、旧调用方清零、删除旧路由与封装 | WebUI + Server | 仓库无生产旧 API 消费者，旧路由不可达 |
| F | 全量 CI、浏览器验收和部署验收 | Integration | 精确 SHA 全项通过，真实部署流程通过 |

## 9. 本轮验证与未完成事项

- 分支/HEAD：`repair_all` / `8be3b57306fad9df39a923945f3df6f11790f683`。
- SRV-CTR-001 文档状态 FROZEN；SRV-CTR-002 当前文档标记 freeze-ready。
- 生产模板盘点：仅 News Search 一项。
- 页面主流程：仍调用旧 Analysis API。
- 新 Researchflow helper：已存在但未被 app 主流程使用。
- 旧 Root 路由：仍存在，Owner Bearer 认证。
- 本次只完成代码/契约盘点，未修改生产代码，未切换页面，未删除旧路由。

### 独立 CI 观察（该 SHA）

- WebUI checks 失败：519 个测试中 1 个失败，测试为 `UIR-2 typography roles use semantic tokens without changing their baseline values`，断言仍要求旧 CSS 规则选择器/声明形式；需核对当前 CSS 与测试契约后修复，不能简单放宽断言。
- WebUI browser smoke 失败：Screening CSSOM baseline 发生真实计算样式差异；例如 320/768/1280/1600 视口下 `select-all` 宽度由基线 90px 变为 111px，`selected-count` 57.58px 变为 71.58px，`add-watchlist-button` 84px 变为 102px。需判断这是预期文案/布局变化还是回归，不能直接刷新 Golden 掩盖变化。
- Integration CI 的 Go format/vet/test、Quant MariaDB、News/Research/Researchflow 集成、Worker Bootstrap、go-core tests、Compose、WebUI image build 等前置步骤均通过；最终 Summary 因 WebUI 检查失败而失败。该结论不等于完整 Integration CI 通过。

## 10. 当前执行结论

当前主线先解决真实业务链路与接口调用关系，不先清理旧接口。下一步先把单股分析、批量分析、市场复盘分别拆成真实业务流程，逐项核对各领域 Owner Facade 与生产能力；优先解除 Quant/Research/Research/LLM/风险语义阻塞。生产模板未就绪前，旧 API 暂时保留为待退役资产，不添加兼容认证，不让新旧 API 同时承载同一业务。
