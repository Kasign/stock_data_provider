# WebUI 维度 2：Analysis Run 分析结果与持久化读取

**审计代码基线：** `origin/integration` @ `b6dfc85ba214fe16268c8cba780641a8bfe15086`  
**维护分支：** `feat/design_doc`  
**Run 读取接口：** `GET /api/v1/analysis/runs/{run_id}`  
**Run 事件接口：** `GET /api/v1/analysis/runs/{run_id}/events`  
**Task 关联接口：** `GET /api/v1/analysis/tasks/{task_id}`、`GET /api/v1/analysis/tasks/{task_id}/events`

## 1. Run 与 Task 的边界

- Run 是分析领域执行记录，使用 `domain.RunRequest` / `domain.RunResult`，由 `internal/analysis/app.Runner` 编排；RunStore 负责 Begin、AppendEvent、Finish。
- Task 是异步执行与排队载体，定义在 `internal/platform/taskqueue/queue.go`。Task 持有 task_id、run_id、状态、阶段摘要和错误摘要。
- 异步创建时 Queue 生成独立 task_id 和 run_id，写入 request_json 后返回；Worker 使用同一个 Analysis Runner 执行请求，不复制业务流程。
- 任务详情通过 task_id 读取；Run 结果通过 run_id 读取。WebUI 从任务状态取得 run_id，再调用 Run 结果接口。
- Run 状态：running、completed、blocked_data_quality、rejected_request、partial、failed。
- Task 状态：queued、running、succeeded、partial、failed。Task 的 succeeded 仅表示 Runner 已正常结束，不代表质量门禁通过；例如 Run=blocked_data_quality 映射 Task=succeeded。这两个状态域不能合并或相互替代。

## 2. Run DTO 与持久化

### RunRequest

字段包括：
- 身份/类型：run_id（可选）、task_type。
- 时间语义：as_of、as_of_mode（可选；explicit 或 realtime_snapshot）。
- Universe/证券：universe_id、symbols。
- 账户与市场：portfolio_id、market_id。
- 策略与版本：strategy_ids、strategy_config_version、risk_config_version、trading_rules_version、data_policy_version。
- 扩展参数：parameters（可选 string map）。

HTTP 创建入口会拒绝未知 JSON 字段，填充部分默认值；as_of 是否由调用方显式提供决定 as_of_mode，随后由 Handler 覆盖请求中传入的 as_of_mode。历史回放必须显式提供 as_of。Runner 进一步校验 symbols 为 1..100 个六位数字 A 股代码，校验 as_of、universe、market、portfolio、各配置版本和至少一个 strategy_id 等。

### RunResult

包含：
- run_id、status、原始 request、started_at、finished_at。
- Universe、原始/市场/组合快照 ID 及组合快照。
- Quality、MarketState、Factors、Signals、Tradability、Risk、Plans、PlanningRejections、ExecutionAssessment。
- 阶段 Events、Failure、Warnings。

RunResult 是分析完整结果，不是任务状态 DTO。字段具有不同的可选语义；Go 使用 omitempty 的字段在 nil/空值时省略。当前没有与 RunResult 对应的 OpenAPI Schema，也没有反射式 DTO/OpenAPI 契约检查。

### Store 与缓存

- MariaDB `analysis_runs` 是 Run 结果权威持久化源；Begin 保存 running 和 request_json，Finish 更新状态、finished_at、as_of、request_json、result_json。
- 阶段事件写入 `analysis_stage_events`，按数据库 id 顺序追加保存。
- Redis 以 `run:{run_id}` 缓存完整 RunResult；读取先校验缓存 JSON 和 run_id，缓存无效时删除并回查 MariaDB；MariaDB 命中后尽力回填 Redis。
- Redis 写入失败不会回滚 MariaDB 已持久化结果。
- GetResult 区分 run 不存在（404）与结果尚未写完（409 run_not_finished）；内部读取错误映射 500。
- Run Events 接口返回完整事件数组，按数据库插入 id 升序；不存在的 Run 返回 404，存在但尚无事件时返回空数组。
- Worker 启动恢复会把上次进程中断时仍 running 的关联 Run/Task 标记 failed，并保留已写入阶段事件；Run 结果中写入 worker_interrupted failure，避免状态永久卡在 running。

## 3. TaskQueue 与异步生命周期

- Queue 依赖 MariaDB 和 Runner；worker 数量默认 2、上限 32；maxQueued 小于 worker 数时会被调高到 worker 数的 10 倍；任务轮询默认 250ms。
- Create 对 Idempotency-Key 去空白、最大 128 字符；同 key 同请求返回已有 Task，不同请求返回冲突；比较请求时忽略 Queue 生成的 RunID，并对 realtime_snapshot 忽略服务端生成的 as_of。
- Queue 在写入前检查 queued 数量是否达到 maxQueued；满队列返回 429。
- Worker 以事务行锁领取 queued 任务，运行统一 Runner；任务终态由 RunStatus 映射：completed/blocked_data_quality → succeeded，partial → partial，failed/rejected/意外非终态 → failed。
- Task 的 completed_stages 是状态为 succeeded 或 skipped 的不同 stage 数量；current_stage 是最近插入的阶段事件 stage，不是前端推算值。
- Task Events 以 task_id 隔离，再使用全局事件表自增 id 作为 sequence 游标；after_sequence 是数据库 id 游标，不是每个 Run 从 1 连续递增的业务序号。
- 任务创建、状态、事件接口均不要求 Market API Key；分析服务自身认证/部署边界另行处理。WebUI 不向 analysisBase 请求附加 Market API Key。

## 4. HTTP 与 WebUI 消费链

### HTTP

- `internal/platform/httpapi/server.go` 注册同步 Run 创建、Run 读取/事件读取、异步 Task 创建、Task 读取/事件读取。
- `GET /api/v1/analysis/runs/{run_id}` 返回完整 RunResult。
- Run 结果未完成返回 409 `run_not_finished`；Run 不存在返回 404 `run_not_found`。
- `GET /api/v1/analysis/runs/{run_id}/events` 返回 `{run_id, events}`。
- Task 事件接口支持 after_sequence 和 limit，limit 1..500，返回 task_id、events、next_after_sequence。
- 分析 HTTP API 使用 request ID middleware；日志记录 request_id、路由、状态码、耗时和响应字节，不记录请求体、响应体、查询参数或鉴权头。

### WebUI

- `api.js` 的 createAnalysisTask 发送 POST tasks，并可携带 Idempotency-Key；超时被标记为 creationOutcomeUnknown，提示先核实任务记录，不盲目重复创建。
- getAnalysisTask / getAnalysisTaskEvents / getAnalysisRun 均使用 analysisBase，不附加 Market API Key；run_id/task_id 路径段使用 encodeURIComponent。
- API Client 测试覆盖任务创建 payload/幂等键、事件增量读取、Run URL、分析端不携带 Market API Key，以及结构化 409 错误 code/status/request_id 保留。
- app.js 的 pollTask 并行读取 Task 状态和增量事件，校验 task_id、run_id、status、证券集合，隔离不同任务/连接的晚到响应。
- task-workflow.js 校验事件响应 task_id、游标前进、事件结构、重复 sequence 和可选 run_id 一致性；坏事件计数不阻断游标继续推进。
- Run 结果读取校验 run_id 和证券集合；对可重试错误按有限延迟重试，保留此前已成功读取的同一 Run 结果并明确提示本次未确认更新。
- 页面将 Task 状态与 Run 状态分别显示；Run 结果页消费 quality、market_state、factors、signals、plans、warnings 等字段，不在前端重算分析结论。
- 最近任务历史只在浏览器本地保存有限引用，不将其视为服务端权威记录；身份使用 task_id + analysis_base，避免不同分析服务相同 task_id 冲突。

## 5. 已登记差异

### AUD-WEBUI-CTR-046（P2）：Run 读取 API 与 RunResult 缺少正式 OpenAPI 契约

- WebUI API Client 实际调用 `GET /api/v1/analysis/runs/{run_id}`；HTTP Server 也注册该路由并返回完整 RunResult。
- 当前 `doc/api/contracts/分析任务.openapi.yaml` 只描述异步 Task 创建、Task 状态、Task 事件三条路径；没有 Run 读取和 Run 事件路径，也没有 RunResult、RunStatus、Failure、StageEvent、Quality 等结果 Schema。
- 现有 API Client 测试验证 Run URL 和 409 错误解析；HTTP Server 测试覆盖同步创建及输入校验，但没有发现 GET Run 成功、404、409 和 Run Events 的 Handler 专项测试。
- 影响：消费者无法从唯一正式 OpenAPI 契约获知 RunResult 字段、状态枚举、可选字段、错误码和 Run Events 响应结构；接口变更缺少 Schema 约束。
- 关闭条件：在权威分析 API OpenAPI 中补齐 Run 读取/事件路径和完整响应 Schema；为状态枚举、必填/可选字段和错误响应建立契约测试；增加 Handler 成功、404、409、事件读取测试。不能只在 WebUI Client 测试中断言 URL 即视为接口契约闭环。

### AUD-WEBUI-CTR-047（P2）：AnalysisRequest OpenAPI 与 domain.RunRequest 字段集合不一致

- `doc/api/contracts/分析任务.openapi.yaml` 将 AnalysisRequest 描述为“字段定义与 domain.RunRequest 一致”，但 Schema 未列出 Go DTO 中的 `as_of_mode` 和 `parameters`。
- `parameters` 是 Runner 实际读取的扩展参数 map；`as_of_mode` 是 RunRequest 的显式时间语义字段，但 HTTP 创建 Handler 会根据 as_of 是否显式提供而重新赋值，因此客户端传入值不会成为最终执行语义。
- Go DTO 还有 run_id；异步 Queue 会生成并覆盖 run_id，因此它应明确标为服务端生成字段，而不是客户端可控字段。
- 影响：OpenAPI 对可发送字段的声明不完整，且“与 RunRequest 一致”的描述掩盖了异步入口对 run_id/as_of_mode 的特殊处理。
- 关闭条件：明确区分通用 RunRequest 与异步 CreateTaskRequest；补充 parameters 的对象/值类型定义；明确 as_of_mode 由服务端根据 as_of 推导（或正式定义客户端字段行为）；明确 run_id 由服务端生成并忽略/拒绝客户端值；增加 Schema/Handler 契约测试。

## 6. 测试覆盖与未证明项

已发现的覆盖：
- Runner：成功完整持久化轨迹、失败/质量阻断等应用层测试。
- TaskQueue：状态映射、幂等请求比较、ID 格式、生命周期关闭、恢复/队列行为等单元测试。
- HTTP Task：创建 202、幂等、历史任务 as_of 校验、事件游标、队列满、未知 Task 404、幂等冲突、请求体大小与 Content-Type 等测试。
- WebUI：API Client、Task workflow cursor/事件验证、Task polling 生命周期、Run 结果视图、浏览器流程与结果场景测试。

本轮未执行固定基线 Go/Node CI；本结论为源码和现有测试文件审计，不代表测试在本轮实际通过。尚未逐项核对 DB migration 的索引/唯一约束、Worker claim 并发语义与多实例部署策略，也未逐项核验 RunResult 每个嵌套 DTO 的 OpenAPI/JSON Golden（当前缺少 RunResult Schema，需先补契约）。

## 7. 本切片结论

Run 与 Task 的身份、状态、存储和 WebUI 消费边界整体清晰：Task 是异步调度状态，Run 是业务分析结果；MariaDB 是持久化权威，Redis 只是 RunResult 缓存；事件序号使用数据库自增 id 作为增量游标。当前主要已确认问题不是 Run 执行链路断裂，而是正式 OpenAPI 没有覆盖 Run 查询契约，以及异步请求 Schema 对 RunRequest 的描述不准确。新增 CTR-046、CTR-047，WebUI 维度 2 继续 partial。
