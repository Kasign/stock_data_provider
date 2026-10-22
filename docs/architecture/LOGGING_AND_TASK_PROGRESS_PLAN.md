# 日志系统与任务步骤进度方案

> 更新日期：2026-10-21  
> 当前状态：结构化 HTTP / 分析步骤日志已接入；异步任务创建、MariaDB 持久化、有界 Worker、状态与增量事件轮询已实现。SSE、取消、前端进度 UI 和真实服务器验收仍未完成。

## 1. 目标

从一个 request_id 或 run_id 中，维护者应能回答：谁发起了什么任务；任务经过哪些服务和步骤；每一步耗时多少；使用了哪个数据域和 Provider；是否命中缓存或触发 fallback；数据为何降级；失败在哪一步、是否可重试；部分结果是否已持久化；任务运行中能否拿到 task_id 并观察状态变化。

日志是运维诊断工具；数据库事件是任务审计记录；任务状态 API/SSE 是调用方进度 UI 的数据源。三者有关联，但不能互相替代。

## 2. 当前基础与缺口

### 已有或正在实现的基础

- 根目录 Go 分析 API 已改为 JSON slog 输出；`LOG_LEVEL` 在加载根目录 `.env` 后解析，避免读取到默认级别。
- 根 HTTP 层已加入 request ID 回传、HTTP 状态码/耗时/响应字节数。
- 分析 API 记录安全输入摘要（任务类型、市场、as_of、股票数量、策略数量）和输出摘要（run 状态、步骤事件数、信号/计划/告警数、质量门禁结果）。
- 分析 Runner 的 stage 函数对每一步开始、成功、失败输出结构化日志，同时将 stage event 持久化到 Store。
- market-data HTTP 层记录 request ID、method、path、状态码、耗时与响应字节数；MX 补充 Provider 会记录数据类型、标的、HTTP 状态和耗时。
- 根 API 已提供 GET /api/v1/analysis/runs/{run_id} 与 GET /api/v1/analysis/runs/{run_id}/events，底层已有 stage event 持久化。

### 当前缺口

- 已新增 `POST /api/v1/analysis/tasks`，持久化后返回 `202 + task_id`；原同步接口保留。调用方可以在任务执行期间轮询状态和阶段事件。
- 已实现任务状态 `queued/running/succeeded/partial/failed`、可选 `Idempotency-Key`、队列容量上限，以及 `after_sequence` 增量读取。事件序号复用 `analysis_stage_events.id`；SSE、取消与重试状态尚未实现。
- 根 API、market-data 和 go-core 的日志字段还没有完全统一，仍需补齐所有 Provider 成功/缓存/兜底分支的埋点。
- 尚未完成日志保留/脱敏规则、日志过滤面板、异步队列恢复和真实部署的端到端验收。
- 后端 task_id/status/events polling 已可供 UI 使用；仓库尚无对应前端 UI。排队期间还没有 Runner 阶段事件，事件数组为空属于正常状态。

## 3. JSON 日志标准

所有 Go 服务统一采用 JSON 结构化日志与 UTC 时间。建议字段如下：

| 字段 | 说明 |
|---|---|
| time、level、msg | 标准时间、日志级别、事件名 |
| service、service_version、environment | 服务标识、版本、运行环境 |
| request_id | 一次 HTTP 请求的相关标识 |
| task_id | 异步任务 ID；异步系统上线后使用 |
| run_id | 分析流程 ID，与 stage event 关联 |
| trace_id、span_id | 后续接入 OpenTelemetry 时的分布式追踪标识 |
| component、operation、stage | 子系统、操作、分析步骤 |
| method、route、status_code | HTTP 请求信息；不记录查询参数 |
| duration_ms、response_bytes | 耗时和响应字节数 |
| input_summary、output_summary | 脱敏后的输入/输出摘要，以字段计数、样本数和质量概况为主 |
| provider、dataset、cache_hit | 数据源、数据域、缓存命中情况 |
| fallback_from、fallback_reason | 兜底信息 |
| quality_status、quality_reasons、missing_fields | 数据质量、质量原因与缺失字段 |
| status、error_code、error_type | 操作结果、稳定错误码和错误类型 |

### 日志级别

- DEBUG：诊断细节，例如标准化阶段丢弃行数、字段校验失败类别；默认关闭，不包含完整 raw payload。
- INFO：请求开始/结束、任务开始/完成、阶段状态变化、Provider 成功、缓存命中/失效、正常 fallback。
- WARN：数据源返回空结果、质量降级、可恢复失败、fallback 触发。
- ERROR：任务失败、所有 Provider 失败、数据库/缓存持续不可用、关键审计事件写入失败、未捕获 panic。

建议由 LOG_LEVEL 选择默认级别，生产默认 info。Debug 级日志仍须遵守脱敏规则。

## 4. 各层的日志责任

| 层级 | 开始事件 | 完成/失败事件 | 允许记录的摘要 |
|---|---|---|---|
| HTTP API | request_id、method、route | status_code、duration_ms、response_bytes | 请求类型/体长度；不记录 body |
| 分析任务 | run_id/task_id、task_type、as_of、标的数量 | run status、duration_ms、failure code | symbols_count、strategy_count、信号/计划数量、质量门禁结果 |
| 分析 stage | run_id、stage、started | stage 状态、耗时 | 输入/输出条数、质量摘要、缺失数据域 |
| DataCollector | dataset、symbol_count、并发数 | 成功/失败/跳过数、耗时 | source chain、cache/fallback、quality status；不要逐个打印完整 watchlist |
| 单个 Provider | provider、dataset、symbol 或 batch size | HTTP 状态、耗时、retry、empty/error | source version、response bytes、解析行数、缺字段数 |
| 标准化/质量检查 | normalization version、adjustment | accepted/rejected/partial row counts | 归一化单位、缺失字段、错误原因统计 |
| 存储与缓存 | repository/cache operation | success/error、耗时 | 表/操作名、影响行数、cache 命中；不得记录 DSN/密码 |
| LLM / 搜索 | provider、model alias、阶段、耗时 | status、token usage（如可用）、fallback | 模型 alias、输入长度/Token 计数和输出摘要；禁止记录完整 prompt |

每一层尽量共享 request_id/run_id。失败信息应带稳定 error_code；message 只用于诊断，必须截断并清理控制字符。

## 5. 必须禁止进入日志的内容

- API Key、数据库密码、Redis 密码、Authorization/Bearer/Cookie 头。
- 完整 HTTP request/response body，除非本地临时 fixture 显式脱敏且不上传日志系统。
- 用户完整自选股、持仓、订单清单；线上一般只记录数量与获授权的标识。
- 完整 LLM prompt、新闻全文、第三方 API 完整自然语言查询和返回表格。
- 含密钥 URL 查询参数、数据库 DSN、未经审查的错误对象 dump。

不得因“调试方便”就默认打印完整入参/返回值。应为各数据对象实现安全摘要，只选择经过审查的字段；新增日志字段要附带脱敏规则测试。

## 6. 任务步骤进度设计

### 6.1 兼容现有 StageEvent

第一阶段沿用已有 run event 及存储逻辑，不再建一张仅存同样字段的冗余日志表。建议在兼容旧 API 的前提下扩展：

- sequence：每个 run 单调递增，用于增量轮询。
- task_id：将异步任务和 run 关联。
- event_type：task/step/progress/result/warning/error 等稳定类型。
- stage、status、at、duration_ms、error_code。
- input_summary / output_summary：安全摘要，不保存完整原始数据。
- progress_current / progress_total / progress_percent：只有阶段能够可靠计算进度时才给值；未知时为 null，不得编造百分比。
- retry_attempt、provider、dataset、quality_status、missing_fields：适用时填写。

Schema 变化必须配套数据库 migration，并保持旧的 GET /api/v1/analysis/runs/{run_id}/events 可读取。

### 6.2 新增异步任务 API

建议先做 polling，再增加 SSE。拟定接口如下，实施前须冻结为 OpenAPI 契约：

- POST /api/v1/analysis/tasks：验证请求后创建 task_id，写入 queued 后立即返回 202 Accepted，不等待分析完成。
- GET /api/v1/analysis/tasks/{task_id}：返回 queued/running/succeeded/partial/failed/cancelled、已完成阶段数、当前/最近阶段、创建/开始/结束时间、失败码和对应 run_id。
- GET /api/v1/analysis/tasks/{task_id}/events?after_sequence=N：增量拉取阶段事件与进度。
- GET /api/v1/analysis/tasks/{task_id}/events/stream：后续通过 Server-Sent Events（SSE）推送；断线后按 sequence 继续读取。
- POST /api/v1/analysis/tasks/{task_id}/cancel：未来可选，只在支持协作取消的阶段生效。

既有同步 POST /api/v1/analysis/runs 暂时保留供兼容和 smoke test；待异步任务通过黄金样例、并发和失败测试后，再决定生产默认入口。

### 6.3 Worker 与持久化

- 任务及事件保存在 MariaDB；Redis 可用于短期缓存/通知优化，但不能成为唯一任务记录来源。
- 第一版已采用 MariaDB `analysis_tasks` 表 + 有界 Go worker pool；没有引入额外消息队列。
- 已实现任务超时、`Idempotency-Key`、Worker 并发上限和最大排队数。进程重启后 queued 任务会继续等待 Worker；重启时遗留的 running 任务会显式标记为 `failed/worker_interrupted`，避免永久卡住；自动重试、取消与多实例队列容量协调尚未实现。
- 每个 task 只创建一个 run；需区分任务重试、单 Provider retry 和 stage 重试，防止重复执行整条分析流程。
- 任务状态机须明确 queued/running/succeeded/partial/failed/cancelled/retrying 的合法迁移。
- stage 事件需要先持久化再承诺对 UI 可见；存储失败时按 fail-closed 处理，不能假装事件完整。

### 6.4 对上游任务进度体验的接入方式

上游 [ZhuLinsen/daily_stock_analysis](https://github.com/ZhuLinsen/daily_stock_analysis) 有按步骤展示任务状态的设计。本项目参考其“阶段状态清晰、用户能看到当前在做什么”的交互，但应沿用当前仓库的 Go stage/event 模型，不直接耦合到 Python 内部状态类。

拟展示步骤包括：校验输入、获取行情、获取辅助数据、标准化、质量门禁、计算指标、生成信号、风险/可交易性检查、生成计划、保存结果。UI 状态应由实际执行事件生成；跳过的步骤标为 skipped 并附原因，不能把未执行当成功。

## 7. 验收标准

### 日志验收

- 每个 HTTP 请求均具备 request_id、method、path、status_code、duration_ms 和 response_bytes。
- 一次分析可通过 run_id 找到全部阶段日志与数据库事件；异步上线后可通过 task_id 查完整链路。
- Provider 日志有 provider/dataset/retry/cache/fallback 的关键摘要。
- 自动化测试证明日志不输出 API Key、完整授权头或原始响应正文。
- 日志为可检索 JSON，而不是只靠自由文本 grep。

### 任务进度验收

- POST 异步任务接口在实际工作开始前返回 202 和 task_id。
- 状态/事件接口可以在任务结束前观察 queued、running 与各阶段变化。
- 每个执行阶段有持久化 started 和 finished event；失败时明确 error_code 与 stage。
- 任务使用独立 task_id/run_id；阶段事件使用数据库自增 ID 作为 sequence。已提交事件不会因 Worker 进程重启而被删除；正在运行的任务在重启后标记失败，不自动重跑。
- 断线后能按 after_sequence 增量续读；SSE 是可选增强，不能替代持久化事件 API。
- 黄金测试覆盖成功、部分成功、所有 Provider 失败、取消、超时、事件写入失败与 Worker 重启。

## 8. 优先级安排

1. P0 当前模块：对齐根 API、market-data、go-core 的日志字段；完成基础 request/stage/provider 结构化日志和脱敏。
2. P1 任务体验：增加任务表、Worker pool、202 创建接口、状态与事件增量轮询。
3. P1 UI 接入：显示阶段、耗时、告警、数据源/回退与缺失数据状态。
4. P2 推送体验：增加 SSE 和断线恢复；只有需要时再评估 WebSocket。
5. P2 深度观测：按需接入 OpenTelemetry traces/metrics、慢请求指标、Provider 成功率与错误率面板。

每个阶段都应作为完整功能模块提交，包含代码、schema migration、测试、中文文档、部署变量和可回滚说明。
