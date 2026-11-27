# WebUI 旧 Analysis API → Researchflow 迁移盘点

- 仓库：`Kasign/stock_data_provider`
- 核对分支：`repair_all`
- 核对基线：`43d773877a20f99325843650f1f8d0799d03ffe8`
- 核对日期：2026-11-26
- 状态：**旧 Analysis 生产链仍在运行；Researchflow 已有 News 与 Quant 的生产模板切片，但尚不具备完整股票研究报告闭环；本次更新为退役执行基线，尚未删除旧链**
- 目标：WebUI 业务任务统一使用 SRV-CTR-001 Researchflow API 和 Owner Session；不保留旧 Analysis API 的浏览器兼容入口，不增加 Bearer/Session 双认证。

## 1. 当前结论

现在存在两套并行业务执行边界：

1. WebUI 的分析任务主流程仍调用旧 `/api/v1/analysis/**` API，并依赖独立 Analysis Base、Task ID、Run ID、事件游标和旧任务状态模型。
2. Root Server 已有 SRV-CTR-001 的 Researchflow Run API，并有对应 WebUI API helper；但页面主流程尚未使用它。
3. Researchflow 当前生产注册至少包含 `news.search_candidates`、`news.feed_collection`、`quant.sma20` 和 `quant.factor` 四个模板。它们分别覆盖 News 候选检索、Feed 采集、单指标 SMA20 计算和三因子（`ma20_distance`/`momentum_5d`/`momentum_20d`）计算切片；仍不等于完整股票研究、批量分析、市场复盘或最终研究报告闭环。

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

**2026-11-28 补充**：`web/tests/api.test.mjs` 已为三个 helper 增加契约测试，覆盖创建/查询/取消路径、同源 Root Server 请求、body 内幂等键与 `Idempotency-Key` header 的区别、内存 CSRF、无 API Key/Authorization 泄漏、`run_ref` URL 编码、非法输入与缺失 CSRF 在发请求前失败。把 `createResearchflowRun` 的 `csrf: true` 临时改为 `false` 后，CSRF 断言有效失败；恢复生产代码后 WebUI 523 个测试全通过、`npm run check` 通过。本轮只验证 helper 契约，不代表页面主流程迁移或端到端验收完成。

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
- 旧 Runner 的 Universe Resolver 直接以请求中的 `symbols` 形成研究范围；Market Analyzer 的核心广度指标命名为 `watchlist_above_ma20_pct`，并明确证据描述为“自选股”横截面。因此当前 `post_market_scan` 也不是全市场扫描，不能仅凭页面标签认定它已经覆盖全市场盘后复盘。
- 旧 `portfolioProvider` 返回 `unconfigured-no-paper-ledger-v1`、现金和权益均为 0；`positionPlanner` 对每个信号都返回“不生成仓位/执行计划”的拒绝；`hardRiskRule` 对所有候选信号 fail-closed 阻断。它不是已具备真实持仓账本、仓位计算和交易计划的生产能力。
- 所以旧结果页中的 Factors/Signals 可以作为旧 MVP 研究结果；Plans/Execution 目前不能当作真实可执行交易计划。新模板必须保持硬风险门禁独立，不得因综合评分或 LLM 结论覆盖阻断。

### 独立 Go Core 的边界

`go-core/cmd/analysisd` 与 `go-core/internal/transport/http/server.go` 还暴露 `/internal/v1/analysis/stock`、`stocks`、`market-review` 三类内部工作流端点；它们由独立的 StockFlow/MarketReviewFlow 组合，并依赖 Python Bridge 和独立配置。当前 Root Server 的 `/api/v1/analysis/tasks` 并不直接调用这组 go-core HTTP 端点，而是走本节上述 `internal/analysis` + `platform/taskqueue`。后续盘点或迁移不得把两套旧实现混为同一个当前生产调用链。
### 独立 Python API Adapter 的边界

这条链与 Go Core 使用的 Python Bridge 不同，不能合并统计：

- `internal/adapters/pythonapi` 中的 `Client` 访问独立旧 Python API，支持 Quote、History、Profile 和 DataOverview。
- `internal/adapters/pythonapi.Collector` 实现旧 `internal/analysis/app.DataCollector`，但当前 Root Server Bootstrap 没有构造它；Root 旧 Analysis 实际使用 `servermarketdata.NewAnalysisCollector(marketDataModule.Application(), ...)`。
- 当前确认的直接命令调用点是 `cmd/pythonapi-smoke`，用于人工连通性/响应契约检查，不运行生产分析 Runner。
- 因此，源码中存在 Python API Adapter 不等于 Root Server 生产链依赖它；也不能据此断言外部脚本或目标环境没有调用者。退役前仍需完成全仓库调用点复核和目标环境只读核验。

### 当前标准部署与源码保留状态

- 根 `docker-compose.yml` 只定义 `stock-analysis-server` 和 `stock-analysis-web`；`scripts/deploy-go.sh` 也只构建、启动这两个服务。
- `go-core/Dockerfile` 与 `python_bridge/Dockerfile` 是独立构建入口；`go-core/go.mod` 是嵌套独立 Go Module，不属于 Root Server 的 Go 包依赖。
- 标准部署脚本会在部署前阻断仍运行的 `stock-analysis-go`、`stock-market-data`、`stock-analysis-data` 旧容器；但根 Compose 未定义 Go Core/Python Bridge 服务，脚本也不能证明目标主机上不存在手工部署的 `analysisd` 或 `python-bridge`。
- 目标环境是否仍有旧容器、systemd 进程、外部调用方或流量，必须在可访问部署主机时只读核验；本次源码检查不把“未出现在标准 Compose”误判为“生产已无调用”。

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

## 4.1 旧 Analysis DataCollector 不能直接充当新 Data Stage

文件：`internal/server/marketdata/analysis_collector.go`

旧 Runner 通过 `servermarketdata.NewAnalysisCollector(marketDataModule.Application(), ...)` 调用 Data 的 `MarketDataReader`，按证券分别读取日线/行情，并读取配置的指数日线。该适配器是旧 Analysis 的 `DataCollector`，不是 Data Snapshot Manifest/授权读取链路。

对照 `internal/marketdata/app/port.go` 的 `DailyBar`，旧 Collector 当前只映射 Symbol、TradeDate、OHLC、Volume、Amount、Source、FetchedAt、AdjustmentBasis；未传播 SourceVersion、ProviderTime、NormalizedAt、SystemObservedAt、AdjustmentFactorRevisionHash/SetHash、NormalizationVersion 和 ExactValues 等已有元数据。旧 `RawBar` 也没有从 Data DTO 映射可用于 PIT 的逐条时间/来源修订证据。

同时它对每个证券独立调用 `GetDailyBars`，没有建立可传入 Researchflow `input_snapshot_refs` 的统一不可变 SnapshotRef，也没有把 Data 授权引用带入 Run。

**结论：** 旧 Collector 可以服务当前旧 Runner，但不能直接复用为 Researchflow 股票研究的生产 Data Stage。新 Stage 必须消费 Data Owner 正式的 Snapshot/Manifest 与授权读取 Port，保留来源版本、系统观察时间、复权证据和逐条可用性语义；不能从旧 RawBar 或 FetchedAt 猜造 PIT。

## 5. Researchflow 生产能力现状

### 5.1 当前生产注册

Server 生产组合根：

- `internal/server/bootstrap/researchflow_execution_composition.go`
- `internal/server/bootstrap/researchflow_composition.go`
- `internal/server/bootstrap/news_search_workflow_template.go`
- `internal/server/bootstrap/news_search_stage.go`
- `internal/server/bootstrap/researchflow_template_prerequisites.go`

当前生产注册包含四个 Template：`news.search_candidates`、`news.feed_collection`、`quant.sma20` 和 `quant.factor`。分别由 News Search、News Feed Collection、Quant SMA20 Stage Executor 和 Quant Factor Stage Executor 执行，并通过对应参数 Schema、Applicability、Stage Output Registry 和 Server Dispatcher 装配。未知 Stage Code 明确失败，不存在通用占位 Executor。Quant 已接入 SMA20 与首批三因子两个 Stage，但仍不代表 Trend、Risk 或完整股票研究链已生产闭环。

### 5.2 现有可复用构件与未闭环能力

| 领域 | 已有构件/Owner 入口 | 生产接入缺口 |
|---|---|---|
| News | 真实 News Search Provider、Stage Adapter、Validator、Template/Executor/Output Registry | 已装配；只覆盖候选新闻检索，不代表完整股票分析 |
| Research | Context Application 的 CreateContext/GetContext、Context Schema/Storage；Researchflow 有 DTO 映射和 Context Assembly | 缺可信 CallerRef/当前授权 Adapter、Run.SystemAsOf 一致性校验、Server/Worker Stage 接线 |
| Quant | GetResult 授权读取 Application、Trend/Risk Envelope；SMA20 CreateSMAService 精确 Decimal 计算/持久化；SMA20Adapter 已进入 Server Stage Dispatcher、合并 Output Registry、参数 Schema 和 quant.sma20@1.0.0 Template；Data Snapshot Creator/Authorized Reader Adapter、Quant Repository/Registry 已装配；Factor 纯计算新增 MA20 距离和 5 日动量函数 | 生产 Researchflow Stage 已含 `quant.sma20` 与 `quant.factor`；Factor 结果可落库并经 `GetResult` 授权读取，Stage 输出/质量 Schema 与 Run↔Result 关联均为强类型 fail-closed。首批 Close-only Factor 可复用现有已授权 Close Series Snapshot，不被通用 DATA-CTR-005/007 deferred 阻断；其生产门禁（Factor Schema/Golden、Decimal Registry/精度策略、生产 Facade、Researchflow Stage 接线）已于第②～⑧组完成。缺真实 MariaDB 的 Worker→Data→Quant→StageOutput E2E，以及 Trend/Risk 生产计算和完整股票研究 Template（见 PLAT-002 登记簿第 51/79 行及 5.2.1 节）。 |
| LLM | Provider.Generate/Stream、ModelRequest、Researchflow 预算/Tool Policy/结果映射 | 缺 Server 生产 Provider 注入、真实调用链和目标模型 Tokenizer；不能把估算器冒充生产 Tokenizer |
| RiskDecision | Quant 的 `risk.Decision` 是 Quant 领域结果 | Researchflow Composer 仍接收独立 RiskDecision；唯一来源、字段映射、多结果聚合和关联身份未解决（RF-BLOCK-008） |
| Data | 已有 DailyBars Close Series Snapshot Creator、Manifest/Payload 双指纹、授权签发和同一 MariaDB 事务内 Authorized Snapshot Reader | 代码能力已有，但 Root 尚未注入 Researchflow 消费者；服务 Principal/Owner Policy 管理入口与 Stage 输入/输出契约未闭环；DATA-CTR-007 仍 deferred。旧 AnalysisCollector 不使用该 Snapshot 链路且丢失关键 PIT/来源元数据 |
| Notification | 已有 Run 终态 Outbox/Relay 相关能力 | 终态通知不应塞入同步 DAG；按独立事件消费链路接入 |

Researchflow 的 Context Assembly、Quant Envelope Validator、Result Composer、Diagnostics Adapter 等内部构件，不等于完整股票研究能力。当前 quant.sma20 是首个已注册的 Quant 生产 Stage 切片；只有完整 Owner Facade、授权、输入输出 Schema/Golden、真实 Provider、Stage Executor、Registry、结果关联和 Server Worker 注入全部齐备，才能将其视为可交付的完整研究模板。

## 5.2.1 Data Snapshot 与 DailyBars V2 的真实实现边界（2026-11-26）

当前 `repair_all` 已有 Data Close Series Snapshot Creator、Manifest/Payload 双指纹、授权签发和同一 MariaDB 事务内 Authorized Snapshot Reader；不得再把这些能力列为“待从零建设”，也不得在 Quant/Researchflow 复制 Snapshot 存储或授权读取实现。

`internal/marketdata/app/port.go` 已定义 `DailyBarsV2Snapshot`、`DailyBarsV2Reader`、`DailyBarExactValues`；`internal/marketdata/application/reader.go` 已实现 `GetDailyBarsV2`，输出 `1d` interval、覆盖率证据和同次评估取得的 TradingCalendarRef。日历证据不可靠时会降级。

但 `DailyBar.Close`/OHLC 等计算字段仍为 float64，ExactValues 只是部分原始十进制证据侧车；Data Snapshot 能力也尚未通过公开 DTO 将内容寻址 SnapshotRef、Payload、Manifest、AuthorizationRef 和 Research Evidence source_ref/version/locator 绑定为一个可由 Researchflow 消费的正式输入。因此 DATA-CTR-005/007 与 QNT-CTR-003 继续 deferred 是合理的，但阻塞点应准确描述为“精度消费契约与跨模块快照引用/授权映射未闭环”，不是“V2/Snapshot 代码不存在”。

下一步优先复用现有 Creator/Reader：核实其实际 payload、fingerprint、事务和授权行为；补齐最小公开输入 DTO/Schema/Golden 与 Data Owner Application Port；再由 Server 注入 Researchflow Stage。不得新增重复 Snapshot Repository、Manifest 或授权框架。

执行核实补记（2026-11-26，`旧Analysis退役执行清单` 1.1）："由 Server 注入 Researchflow Stage"已由 `quant.sma20` 完成——`internal/server/bootstrap/quant_sma20_stage.go:63-84` 构造 `quantDataSnapshotAdapter` 注入 `CreateSMAService` 与 `SMA20Adapter`，`researchflow_execution_composition.go:98-104,143-156` 装入 Worker dispatcher/runtime，`quant_data_snapshot_adapter.go:119-120` 实现 Quant 侧 `CloseSeriesSnapshotReader` 并在同一 MariaDB 事务内读取授权 Snapshot。因此授权读取 Port 注入不是缺口，不应重复施工；真正的未完成项是"除 SMA20 外的股票研究 Stage/Template"与"SnapshotRef/Manifest/AuthorizationRef 的 Researchflow 公开输入 DTO 绑定"，对应执行清单 2.1 与 1.2。

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

- 分支/HEAD：`repair_all` / `dfaad086d992ed7bfaefde17ff5745e7c48ab78c`。本轮相对用户提供的 `f2d210c515df7cb60cec7b28d1ba582033b54dff` 基线前进 13 个提交；其中 12 个提交仅新增/修正 Quant SMA 能力，最新 1 个提交增加 Researchflow 按 Run 版本解析 Stage Output Registry 的能力。
- SRV-CTR-001 文档状态 FROZEN；SRV-CTR-002 当前文档标记 freeze-ready。
- 生产模板盘点：截至该记录 SHA `dfaad086d992ed7bfaefde17ff5745e7c48ab78c` 时仅 News Search 一项（历史快照）；当前生产注册至少包含 `news.search_candidates`、`news.feed_collection`、`quant.sma20` 三项，见本文第 1 节第 3 点。
- 页面主流程：仍调用旧 Analysis API。
- 新 Researchflow helper：已存在但未被 app 主流程使用。
- 旧 Root 路由：仍存在，Owner Bearer 认证。
- 当前 HEAD 的代码核查确认：Root Server 仍装配旧 Analysis Runner/Task Queue/Worker/HTTP API；WebUI 主流程仍调用旧 Analysis API；Researchflow helper 尚未接入页面；Root Server Researchflow Worker 当前仍只装配 News Search Stage/Template。Data Module 与 Data Owner Policy 已进入 Root Server，但 Data Authorized Snapshot Reader 尚未接入 Researchflow Worker；Research News 组合依赖 Key Ring 装配，但当前没有 Stage/HTTP 用例消费；Quant Policy 管理已装配，SMA 生产 Facade 尚未进入 Server/Researchflow 组合；LLM 未见生产 Provider 注入；Notification 通过终态 Outbox/Relay 接入，不应作为同步 DAG Stage。

### 历史 CI 观察（8be3b57306fad9df39a923945f3df6f11790f683）

- WebUI checks 失败：519 个测试中 1 个失败，测试为 `UIR-2 typography roles use semantic tokens without changing their baseline values`，断言仍要求旧 CSS 规则选择器/声明形式；需核对当前 CSS 与测试契约后修复，不能简单放宽断言。
- WebUI browser smoke 失败：Screening CSSOM baseline 发生真实计算样式差异；例如 320/768/1280/1600 视口下 `select-all` 宽度由基线 90px 变为 111px，`selected-count` 57.58px 变为 71.58px，`add-watchlist-button` 84px 变为 102px。需判断这是预期文案/布局变化还是回归，不能直接刷新 Golden 掩盖变化。
- Integration CI 的 Go format/vet/test、Quant MariaDB、News/Research/Researchflow 集成、Worker Bootstrap、go-core tests、Compose、WebUI image build 等前置步骤均通过；最终 Summary 因 WebUI 检查失败而失败。该结论不等于完整 Integration CI 通过。

### 当前 HEAD CI 状态（dfaad086d992ed7bfaefde17ff5745e7c48ab78c）

- Go test suite：Run `45379809366`，当前 `in_progress`。
- Integration CI：Run `45379809512`，当前 `in_progress`。
- 前一 HEAD `65fc6a3ff2ebbf6fde8b464a2018fa38fa17f6fd` 的 Quant module validation 与 Go test suite 均 `success`；Integration CI Run `45379025719` 为 `failure`。不能将前一 SHA 的通过结果视为当前 HEAD 的完整验收。
- 本轮未连接部署主机；容器/进程/外部调用者状态仍未核实。

## 10. 当前执行结论

当前主线先解决真实业务链路与接口调用关系，不先清理旧接口。下一步先把单股分析、批量分析、市场复盘分别拆成真实业务流程，逐项核对各领域 Owner Facade 与生产能力；优先解除 Quant/Research/Research/LLM/风险语义阻塞。生产模板未就绪前，旧 API 暂时保留为待退役资产，不添加兼容认证，不让新旧 API 同时承载同一业务。

## 8. 旧 Analysis / Python Bridge / Python API Client 退役映射（RA-01）

本节区分三套互不等价的旧链，防止把“源码里有 Python Client”误说成 Root Server 当前生产依赖。

### 8.1 Root Server 旧 Analysis（当前真实生产链）

cmd/analysis-api → internal/server/bootstrap → marketDataModule.Application() → internal/server/marketdata.NewAnalysisCollector → service.NewRunnerWithDataCollector → internal/analysis/app.Runner → internal/platform/taskqueue → internal/platform/httpapi → /api/v1/analysis/**

Bootstrap 当前仍构造旧 Runner、MariaDB Task Queue 和 legacyAnalysisWorker；router.go 仍注册旧 Runs/Tasks/Events 路由。此链没有通过 internal/adapters/pythonapi.Collector 读取数据，Root Server 使用 servermarketdata.NewAnalysisCollector(marketDataModule.Application(), ...)。

旧 Runner 的业务阶段及目标 Owner：

| 旧阶段/能力 | 当前实际职责与限制 | 新归属/处置 |
|---|---|---|
| Universe Resolver | 将请求 symbols 转成证券范围；旧版本直接按请求证券列表，不是全市场 Universe | Researchflow Template/Run 参数和 Research Context 的范围解析；不得将旧请求 Resolver 原样移入 Data |
| Requirement Catalog | 汇总因子/策略需要的数据字段 | Quant/Research 的声明式输入需求，最终由 Researchflow Template 固化依赖 |
| DataCollector | 按证券读取行情/日线；旧 analysis_collector.go 丢失多项来源、PIT、复权和 ExactValues 证据 | Data Owner Snapshot Creator/Authorized Reader；新 Stage 必须消费 Data Snapshot/Authorization，不复用旧 Collector |
| Normalizer | 将旧 RawSnapshot 转成 MarketSnapshot，部分字段校验/排序 | 数据标准化和字段证据由 Data Owner 负责；业务上下文组装由 Research 负责 |
| Quality Gate | 检查历史条数、SourceNotes、策略可运行性；旧规则只验证旧 Runner 的简化模型 | Data 负责字段/来源/覆盖质量事实，Researchflow 负责 Stage 传播，Quant/Risk 负责各自输入可用性；不能把旧 corePass 规则当作新全局门禁 |
| Market Analyzer | 计算 watchlist_above_ma20_pct、指数 20 日收益；代码明确它是自选股横截面，不是完整市场广度 | 需要的市场/横截面指标由 Quant 提供；市场背景与证据汇总由 Research 组织。全市场指标未有数据证据前不得复用旧名称或宣称全市场 |
| FactorEngine | 执行 MA20 距离、5/20 日收益等注册因子 | Quant；逐项核对算法、输入口径、版本、样本数和缺失状态，不把旧 float 结果直接迁入 |
| StrategyEngine | 按显式 strategy_ids 运行策略，检查信号归属/时间等 | Quant；旧 trend-following 只是 MVP 基线，需重新核对策略合同和真实证据，不默认迁移 |
| Tradability | 旧风险链中的可交易性判定 | Data 提供停牌、ST、涨跌停等有来源和时点证据的事实；Quant RiskDecision 做独立硬门禁。任何缺证据情况继续 fail-closed |
| Portfolio | 旧默认实现返回 unconfigured-no-paper-ledger-v1、权益/现金均为 0 | 旧占位实现直接淘汰；未来若需要真实组合分析，先由明确的 Portfolio Owner/账本能力提供正式输入，不伪造持仓 |
| RiskEngine | 对候选信号执行规则并产出阻断原因 | Quant RiskDecision；硬 Gate 独立于评分和 LLM，不能被综合分覆盖 |
| Planner / ExecutionAssessment | 旧 Planner 不生成真实可执行计划；Execution 只是评估，不是券商下单 | 当前不接券商订单；旧占位 Planner/Execution 逻辑不迁移。未来若需要模拟计划，另行定义明确业务合同 |
| Snapshots / Store | 旧 Analysis Run/Raw/Market Snapshot 和历史结果持久化 | Data 负责输入 Snapshot；Quant/Research 各自持久化领域结果；Researchflow 负责 Run/Stage/Attempt/Output，不再保留旧 Analysis Store |
| Events / Task Queue | 旧 Task、Run、事件序列和恢复逻辑 | Researchflow Run/Stage/Attempt/Finalization/Outbox；WebUI 不再依赖旧 Task Event Cursor，不伪造 Attempt 明细 |

### 8.2 独立 Go Core + Python Bridge（与 Root Analysis 不是同一链）

真实入口为 go-core/cmd/analysisd → go-core/internal/application.StockFlow / MarketReviewFlow → go-core/internal/adapters → python_bridge/capability_app.py → python_bridge/capabilities/{market_data,search,llm,persistence,notification}.py。

analysisd 当前明确要求 GO_CORE_TOKEN、PYTHON_BRIDGE_TOKEN、PYTHON_BRIDGE_BASE_URL 和 MARKET_DATA_BASE_URL；ReadyCheck 同时检查 Python Bridge 与 Go Market Data。虽然行情主链已要求 Go Market Data，MarketDataAdapter 仍保留 Python stock-bundle fallback；StockFlow/MarketReviewFlow 还直接注入 Python Bridge 的 Search、Analyzer、Persistence、Notification Adapter。它不是 Root Server 的 /api/v1/analysis/tasks 后端。

| go-core / Bridge 能力 | 当前旧实现 | 新 Owner / 退役方式 |
|---|---|---|
| Market Data | marketdatahttp + pythonbridge.MarketDataAdapter，含 Python auxiliary/fallback | Root Data Module；旧独立 HTTP Client 和 Python fallback 随 go-core 一起退役 |
| News Search | go-core/internal/adapters/newssearch，同时使用 native Provider 与 Python Search Adapter | News Module 的 ProviderSet/News Stage；不得保留第二套 News 聚合器 |
| LLM Stock Analyzer / Market Review | pythonbridge.AnalyzerAdapter 调 Python llm.py | LLM Gateway/LLM Module 提供模型调用，Research 负责 Context/Evidence/Claim 和报告编排 |
| Persistence | Python persistence.py 保存 DailyBars、StockAnalysis、MarketReview | 分别由 Data、Quant、Research 的 Repository/Application 持有；禁止继续用 Python 通用持久化旁路 |
| Notification | Python notification.py 发送股票分析/市场复盘通知 | Notification Outbox/Relay 异步消费 Researchflow 终态事件，不作为同步 Stage |
| Go Core HTTP API | /internal/v1/analysis/stock、stocks、market-review | 由 Root Server Researchflow Run API + Template 取代；新 WebUI 不再访问 go-core |

### 8.3 独立 Python API Client（不属于上述 Bridge）

- internal/adapters/pythonapi.Client 调用独立旧 daily_stock_analysis API，支持 Quote、History、Profile、DataOverview。
- internal/adapters/pythonapi.Collector 实现旧 internal/analysis/app.DataCollector，但当前 Root Server Bootstrap 没有构造它。
- 当前确认的直接命令入口只有 cmd/pythonapi-smoke；其用途是连通性/响应契约检查，不是 Root Server 生产分析链。
- 退役目标：删除 internal/adapters/pythonapi/** 和 cmd/pythonapi-smoke/**，并清理 PYTHON_API_* 配置、测试、文档和 CI 引用。删除前仍需全仓库精确引用搜索，不能仅凭 Bootstrap 未使用就断言不存在外部脚本调用者。

### 8.4 一次性删除清单（必须在新链路和调用方迁移完成后执行）

1. Root Analysis：internal/analysis/**、internal/service/** 中仅供旧 Runner 的 wiring、internal/server/marketdata/analysis_collector.go、internal/server/bootstrap 的旧 Runner/Task Queue/Worker 装配、internal/platform/httpapi 旧 handler、internal/platform/taskqueue（仅在确认无其他消费者后）、旧 Analysis Migration 和 /api/v1/analysis/** 路由。
2. WebUI：旧 Analysis API helper、Task/Event Cursor 状态、旧 RunResult 字段消费和旧结果导出依赖；迁移后移除旧 Task/Run 双 ID 状态，统一使用 Researchflow Run Ref 和正式结果资源读取契约。
3. Python API Client：internal/adapters/pythonapi/**、cmd/pythonapi-smoke/**、对应配置和 CI。
4. Python Bridge：整个 python_bridge/**，包括 FastAPI app、capabilities、schemas、tests、Dockerfile；以及 go-core/internal/adapters/pythonbridge/**。
5. 独立 Go Core：整个 go-core/**（含嵌套 go.mod、analysisd、StockFlow/MarketReviewFlow、native News/MarketData adapters、HTTP transport、Dockerfile），前提是确认无其他仓库/部署调用方。
6. 部署/CI/配置：清理 Python Bridge/Go Core 的 Docker build、Python tests、PYTHON_BRIDGE_*、GO_CORE_*、MARKET_DATA_BASE_URL 等只属于旧入口的配置；清理旧容器阻断逻辑前先完成目标环境只读核验。
7. 文档：删除旧服务部署、API、配置和排障说明；保留一份最终迁移记录，记录删除范围、无调用方证据和 CI 验收。

### 8.5 删除门禁

- 新 Researchflow Template 已覆盖需要保留的业务输入/计算/风险/结果/通知语义；明确废弃的旧占位能力有书面结论。
- Data、Quant、Research、LLM、Notification 的真实 Stage/Facade、Schema、Output Registry、模板和 Server DI 已闭环；Data 授权主体与策略验证通过。
- Run/Stage/Attempt 状态、取消、Deadline、重试、失败诊断、结果摘要和 Outbox 有真实 MariaDB 集成验收。
- 完整报告/结果正文由唯一 Owner 持久化，并通过正式授权读取 API 供 WebUI 使用；不让 UI 访问 Repository。
- WebUI 创建、幂等重放、未知提交结果、轮询、取消、历史、结果读取和导出全部迁移，旧 /api/v1/analysis/**、Python API、go-core、Python Bridge 调用点归零。
- 对 repair_all 全仓执行旧路径/环境变量/容器名/工作流引用搜索，逐项清理；旧目标环境的外部进程/调用者已核验或明确不在范围。
- 以上全部通过后，才允许在一个完整逻辑工作组中删除旧入口、配置、Migration、容器、CI 和文档；不保留双链路兼容期。

## 10. 2026-11-26 退役执行基线与实施顺序

本节覆盖旧版盘点中已过时的基线描述。后续执行以本节和当前代码为准，不再沿用 2026-11-24 的单模板数量结论。

可勾选的任务清单、责任模块与退出条件已收敛到 [旧Analysis退役执行清单](./旧Analysis退役执行清单.md)；本节继续作为该清单的事实源与调用链证据，任务状态与勾选以清单为准。

### 10.1 当前边界（已由代码确认）

- Root Server 是唯一外部 HTTP 入口和组合根；Researchflow 只提供内部 Application/执行编排，不独立启动 HTTP Listener、不直接暴露外部 API。
- WebUI 通过 Root Server 调用 SRV-CTR-001；Server 负责认证、路由、DTO 和领域 Owner Facade 装配。
- 旧 Analysis 仍在 Bootstrap 中创建旧 Runner、Task Queue、HTTP API，并注册 legacyAnalysisWorker；Router 仍注册 /api/v1/analysis/**。因此旧链不是无调用方的死代码，不能直接删除。
- cmd/analysis-api 当前调用 bootstrap.Run，是 Root Server 的程序入口；不得因为命名含 Analysis 而删除该入口或部署构建目标。
- internal/platform/httpapi 除旧 Analysis API 外还承载根路径服务信息响应；退役时须把仍需要的根路径行为迁至 Root Server，再删除旧 Handler。
- Migration Registry 严格校验已应用 namespace 集合。移除 analysis 注册前，必须按一次性退役步骤清理旧四表及 platform_schema_migrations.namespace='analysis' 记录；不保留永久兼容 namespace。

### 10.2 退役前不可跳过的真实能力缺口

**2026-11-27 校准**：Researchflow 已注册 `news.search_candidates`、`news.feed_collection`、`quant.sma20`、`quant.factor`、`quant.trend`、`quant.risk`，并以 `quant.research@1.0.0` 合并四 Stage（单 Run 可产出因子/趋势/风险结果）。仍未提供完整报告正文（RF-BLOCK-007 OPEN，projector 未注入）、Attempt 明细或事件流；WebUI 报告导出因此仍不能等价迁移，这是阶段 3.1 的硬前置。

因此不得先删旧 API/Runner 再补业务能力；否则会直接造成 WebUI 任务入口和结果页失效。必须先完成真实 Owner 能力接线、Run 结果与领域结果强类型关联、Server 授权读取 DTO，再切换 WebUI。

### 10.3 以真实调用链为准的执行顺序

1. 冻结旧链调用面清单：复核 WebUI 所有旧 API 的实际调用点、Server 路由、Bootstrap 装配、Worker 生命周期、配置项、Migration、测试和部署入口；区分 Root Server 旧 Analysis、独立 go-core、独立 Python Bridge 和 Python API Adapter，不将它们混为一条生产链。
2. 完成能力归属和缺口：旧 Runner 的数据采集/质量归 Data；指标、因子、策略和硬风险结果归 Quant；研究上下文/证据/结论归 Research；跨模块 DAG 编排归 Researchflow；外部 HTTP、认证和报告读取 DTO 归 Server。只迁移真实能力，旧占位持仓/计划逻辑不迁移。
3. 闭环 Researchflow 股票研究执行：通过 Server 注入 Data Snapshot 授权读取 Port、Quant 正式计算/结果授权读取 Port、Research Context/Evidence Port，以及需要的 LLM Provider；定义真实模板与 Stage 输入输出契约，完成 Worker→Owner→StageOutput 持久化和关联。不得在 Researchflow 重写领域算法。
4. 补齐 Server 读取契约：沿用 SRV-CTR-001 Run Create/Get/Cancel；按需要增加 Server 层结果/报告读取契约，鉴权后调用领域 Owner 授权读取能力。Run/Stage 聚合不冒充完整报告，不直接访问 Repository。
5. 迁移 WebUI：将任务创建、幂等提交、未知提交结果、状态轮询、取消、历史列表、详情和报告导出改为 Root Server Researchflow 契约；执行详情以真实 Run/Stage 状态为准，不伪造旧事件流。移除 Task ID、旧 Event Cursor、Analysis Base 和旧 RunResult 依赖。
6. 整体删除旧 Analysis：确认全仓库无生产调用方后，删除旧 HTTP API、Task Queue、Runner 生产装配、旧 Analysis 领域/存储、旧 DataCollector、路由、配置、前端调用、测试夹具和过时文档；保留 Root Server 通用认证、Health/Ready、平台能力及 cmd/analysis-api 启动入口。
7. 一次性清理旧数据结构：在停服窗口清理 analysis_runs、analysis_stage_events、analysis_snapshots、analysis_tasks 和 Migration History 中的 analysis namespace 记录；确认外键、表定义和清理顺序后执行。旧数据不迁移、不兼容。
8. 最终验收：验证旧路由/旧 Worker/旧配置/旧表均不存在；Researchflow Run 创建、执行、查询、取消和授权报告读取正常；WebUI 无旧 API 请求；Owner Session、Health/Ready 和标准部署正常。最后对最终 SHA 执行完整测试与 CI，本阶段不以中途 CI 状态作为停工条件。

### 10.4 当前进度状态

- [x] 纠正 Researchflow 的职责边界：内部执行编排，不直接暴露 HTTP；Root Server 唯一对外。
- [x] 刷新 Researchflow 模板数量和旧链实际生产装配基线。
- [x] 确认旧 Analysis 仍有 WebUI、Server Router、Bootstrap、Worker 和 Migration 实际引用。
- [x] 完成旧 Runner 逐阶段能力映射；已核对 Quant 现有因子/趋势/风险构件与生产 Stage 的差异。
- [x] 补齐完整股票研究模板、领域结果关联和 Server 授权读取（2026-11-27 校准：2.1 `quant.research@1.0.0` 四 Stage、2.2 Quant/News 授权结果读取、2.3 `GET /api/v1/researchflow/runs/{run_ref}/results` 均已完成；完整报告正文仍待 RF-BLOCK-007）。
- [ ] WebUI 完成 Researchflow 迁移。
- [ ] 删除旧 Analysis 生产链和相关代码。
- [ ] 一次性清理旧 Analysis 表及 Migration namespace。
- [ ] 最终完整验收。

执行原则：以真实代码调用关系推进，优先连续完成同一工作组；不等待中途 CI 结果，不降低测试标准，不为迁移保留旧兼容实现。若真实能力缺口尚未闭环，明确记录阻塞项，不以删除旧入口制造“已退役”假象。
### 10.5 旧 Runner 阶段能力映射（按当前实现核对）

以下映射来自 internal/analysis/app/runner.go、internal/analysis/app/ports.go 和 internal/service/wiring.go，不把旧阶段名称直接当成目标模块已完成的能力。

| 旧 Runner 阶段/能力 | 当前旧实现的实际行为 | 目标归属 | 当前可确认的新模块能力 | 退役判断 |
|---|---|---|---|---|
| validate_request / resolve_universe | 校验请求；Universe 直接由请求 symbols 构造，交易状态标记 unknown，板块/交易所部分由代码前缀推断 | Server 输入校验 + Data 证券主数据 | Server 有 Researchflow 请求契约；Data 有证券/行情 Application 能力 | 不迁移旧 Resolver；以 Data 真实证券主数据和 Server DTO 校验替代 |
| resolve_requirements / resolve_active_requirements | 从旧 Strategy/Factor Registry 汇总 daily_bars、quote、index_bars 需求 | Quant | 当前 Quant 有 SMA20 计算与结果存储，不等于旧需求注册表和完整因子需求体系 | 在 Quant 正式需求/策略能力落地时替换 |
| collect_data | 由 Server 旧 AnalysisCollector 按证券读取日线、行情和指数 | Data | Data 已有 DailyBars Close Series Snapshot Creator、Manifest 和授权 Snapshot Reader；旧 Collector 未使用该链且丢失来源/PIT 元数据 | 新 Data Stage 必须消费授权 Snapshot；旧 Collector 可随旧 Runner 删除 |
| persist_raw_snapshot / normalize_data / persist_market_snapshot | 旧 Analysis 自己保存 Raw/Market Snapshot；Normalizer 丢弃无效报价/日线并按日期排序 | Data | Data Snapshot 已有 Manifest/Payload 双指纹及授权读取；没有理由在 Analysis/Researchflow 复制一套行情快照存储 | Snapshot 归 Data；Researchflow 只保存输入引用和 Stage Output 关联 |
| quality_gate | 至少一只证券有 21 根有效日 K 才通过；缺指数或上游数据会标记 partial；historical_replay 强制阻断 | Data 质量诊断 + Quant 输入适用性 | Data 有质量/诊断能力，但旧 Runner 规则尚未证明已映射到 Data/Quant 新契约 | 重建正式输入质量门禁；历史回放保持缺 PIT 证据即阻断 |
| market_analysis | 计算自选股收盘价高于 MA20 的比例；指数 20 日收益为辅助；算法版本 mvp-watchlist-market-v1 | Quant | 当前 Quant 正式生产 Stage 仅 SMA20；没有对应市场横截面/指数市场状态 Stage | 需确认该指标是否保留；若保留由 Quant 实现，禁止继续称为全市场广度。**2026-11-26 结论（1.6，M1 决议：书面废弃）**：该指标分母是请求 `symbols`，不具备市场广度语义；已决议不迁移、不正名，真实市场环境归 QNT-005，口径与前置依赖见 `doc/quant/08-market-breadth指标决策与正名.md`；旧实现随阶段 4 删除 |
| factor_calculation | 计算 MA20 距离、5 日收益、20 日收益，使用旧 Factor Engine/Registry | Quant | 当前没有对应的三个正式 Quant Factor 生产 Stage/批量结果能力 | 这是实际功能缺口，不能删除旧 Runner 后声称已迁移 |
| strategy_evaluation | trend-following：三项因子都为正才产生 long_candidate；信号带有入场/退出/失效规则和证据引用 | Quant | 当前未发现等价的正式策略评估 Application/Researchflow Stage | 需由 Quant 定义正式策略契约和版本化计算；不在 Researchflow 重写算法 |
| tradability_check | 因缺少可信 ST、停牌、涨跌停和可成交量信息，对每个候选信号均阻断 | Quant 风险/可交易性门禁，数据事实归 Data | 目前旧逻辑明确 fail-closed；不能因迁移而放宽 | 保留硬门禁语义；由 Quant 正式风险决策输出，不允许评分或 LLM 覆盖 |
| load_portfolio / persist_portfolio_snapshot | 返回 unconfigured-no-paper-ledger-v1，现金/权益为 0，无真实持仓账本 | Quant/组合能力（若未来真实需要） | 当前旧 Provider 是占位实现，没有可迁移的真实组合数据 | 直接退役，不迁移占位账本 |
| risk_assessment | unverified-tradability-block 对所有候选信号 Allowed=false | Quant | 当前 Quant 已有 Risk Envelope/Decision 类型，但需核对正式决策服务和结果来源是否覆盖该硬门禁 | 仅在 Quant 生产决策和测试证明等价后删除旧实现；硬阻断不能弱化 |
| portfolio_planning / execution_assessment | Planner 对所有信号拒绝生成计划；Execution 只生成 no-fill-mvp-v1 占位评估 | Quant（未来真实组合决策） | 未发现真实组合账本、仓位计划或模拟成交能力 | 两段当前均为占位行为，直接退役，不迁移假计划/假成交 |
| Run/Stage orchestration、持久化和事件 | 旧 Runner 自己驱动阶段、写旧 Run/Stage Events 和旧 Result | Researchflow | Researchflow 已具备 Run/Stage/Worker/Output 基础框架及正式模板 | 由 Researchflow 接管编排；不迁移旧事件表/旧 Task Queue 模型 |
| HTTP、Task Queue、WebUI 状态/报告 | 旧 Platform HTTP API、Task Queue 和 WebUI Task/Event/RunResult 绑定 | Server + WebUI | Server 已有 SRV-CTR-001 Run Create/Get/Cancel；WebUI 主流程仍调用旧 API | Server 结果读取契约与 WebUI 迁移完成后删除 |

### 10.6 现在可以直接退役与必须先替换的内容

可直接判定为无真实业务能力、无需迁移：
- 旧 portfolioProvider 的零现金/零权益虚拟组合快照。
- 旧 positionPlanner 的全量拒绝占位计划。
- 旧 executionAssessor 的 no-fill 占位执行评估。
- 旧 market breadth 命名中的“全市场”误导；当前实际是请求证券横截面。指标是否保留由 Quant 新契约明确。

必须先完成 Owner 替换后才能删除：
- Data Snapshot 的生产 Stage 消费和授权读取。
- Data 输入质量事实与 Quant 适用性门禁的正式映射。
- MA20 距离、5/20 日收益因子计算。
- trend-following 策略评估及信号证据。
- tradability/risk 硬门禁及不可被综合评分覆盖的决策结果。
- Run/Stage Output 与 Quant/Research 领域结果的强类型关联、授权读取，以及 WebUI 结果页所需的 Server DTO。

据此，当前旧 Analysis 退役的真正阻塞不是 Task Queue 技术迁移，而是 Quant 领域能力尚未覆盖旧 Runner 的核心因子/策略/风险结果，Data Snapshot 尚未进入股票研究 Stage，且 Server/WebUI 尚未建立完整报告读取闭环。先删除旧链会导致原有研究结果功能消失，因此不执行这种破坏性删除。
### 10.7 Quant 现有构件与生产接入的区别（2026-11-27 校准）

本节原结论是 2026-11-26 基线；后续 1.2/1.3/1.4 已落地，旧结论不再成立，按当前代码改写如下。

- `internal/quant/factor`：已实现 `MADistance20`、`Momentum5`、`Momentum20` 的 Decimal（big.Rat, scale=12, half-even）精确计算，并由 `CreateFactorService` 通过已授权 Close Series Snapshot 产出可持久化结果；`quant.factor` Stage 已装配。**仍不等于 QNT-CTR-003 可生产发布**（契约仍 `deferred`）。
- `internal/quant/trend`：已实现 `DirectionOf` 无阈值方向判定与 `assessQuality`，`CreateTrendService` 一次授权读取产出四个证据 Metric（MA20 距离、5d、20d、60d）；`quant.trend` Stage 已装配，`phase` 恒 `unknown`、`strength` 恒 `null`。
- `internal/quant/risk`：`qnt-risk-tradability/1.0.0` 规则集与 `RuleCatalog` 已落地，`CreateRiskService` + `quant.risk` Stage 已装配，证据四元化为 `[交易状态快照, 交易状态授权, 盘口快照, 盘口授权]`。**决策仍恒 `reject`**：`tradable_volume` 已由 DEC-PLAT-002-066 用 Eastmoney 五档盘口解决，`price_limit` 仍缺合格来源。
- `internal/quant/stage`：已从"仅 SMA20"扩展为 SMA20/Factor/Trend/Risk 四个 Stage；Server 的 Researchflow Execution Composition 已装配全部四个，并支持 `quant.research@1.0.0` 合并模板。

因此 10.7 原判断"核心阻塞是 Quant 构件未接入生产"已关闭；**当前退役的核心阻塞变为**：完整报告唯一 Owner（RF-BLOCK-007）与 WebUI 页面迁移。
## 10. 本轮 Quant 因子补齐进度（2026-11-26）

- 在 `internal/quant/factor` 增加 `Momentum5` 和 `MADistance20` 纯计算函数，统一返回 ratio、warm-up 状态和样本数；并增加 CalculationContext 入口校验。
- 增加 5 日动量、MA20 距离的窗口边界/样本计算测试。
- 这两项仅补齐 Quant Domain 纯计算能力；没有加入冻结的 QNT-CTR-004 Factor Metric Schema/Golden，也没有注册生产 Registry、持久化或 Researchflow Stage。
- 首批 Close-only Factor 可复用现有已授权 Close Series Snapshot，不被通用 DATA-CTR-005/007 deferred 阻断；其生产发布门禁是 QNT-CTR-003 自身的 Factor Schema/Golden、Decimal Registry/精度策略、生产 Facade 与 Researchflow Stage 接线（见 5.2.1 节和 PLAT-002 登记簿第 51/79 行）。不得绕过该门禁，不得把旧 Analysis 的 float64 计算直接提升为生产发布结果。
- 本轮未运行 CI；按任务要求待整组处理完成后统一检查。
