# WebUI 当前 API 集成契约

更新日期：2026-10-27（新增数据中心检查范围）

## 结论

WebUI 使用的 HTTP API **不是现在从零新定的**。行情服务与 Go 分析服务已有实际路由、实现和部分 OpenAPI/部署文档。本文件是前端当前使用范围的接口清单，便于联调和 Review；它不是新版本 API，也不取代 Go 服务自己的协议定义。

现有的权威参照：

- [分析异步任务 OpenAPI](../docs/architecture/analysis-tasks.openapi.yaml)：任务创建、任务状态、阶段事件和请求/响应 schema。
- [仓库 Go API README](../README.md)：服务拆分、异步任务调用示例、事件续读和 Run 查询说明。
- [Go RunResult 领域模型](../internal/analysis/domain/types.go)：分析结果字段与 JSON 名称。
- [WebUI API 客户端](./src/api.js)：本页面实际发出的请求路径、参数和请求头。
- [WebUI Nginx 主配置](./nginx.conf) 与 [站点/上游模板](./default.conf.template)：主配置加载容器启动时生成的代理站点；根目录 .env 的 MARKET_DATA_SERVICE_NAME 与 ANALYSIS_SERVICE_NAME 决定实际的 Docker 上游服务名。

## 当前前端调用清单

以下路径中的前缀是浏览器路径。Nginx 会去除代理前缀，再转发至对应上游服务。

| 服务 | 浏览器请求 | 请求参数/请求头 | 用途 |
|---|---|---|---|
| Go 行情 | `GET /market-api/health/live` | 不附加 API Key | 行情服务存活检查（数据中心“立即检查”使用） |
| Go 行情 | `GET /market-api/health/ready` | 不附加 API Key | 行情服务就绪检查（数据中心“立即检查”使用） |
| Go 行情 | `GET /market-api/api/v1/data-sources` | `X-API-Key` | 读取 Provider 能力/状态 |
| Go 行情 | `GET /market-api/api/v1/quotes/{symbol}` | `X-API-Key`；股票代码作为路径段 | 查询实时行情 |
| Go 行情 | `GET /market-api/api/v1/bars/{symbol}/daily?limit=120&adjustment=forward_adjusted&start=2026-10-01&end=2026-10-23` | `X-API-Key`；`limit`、`adjustment` 可配置；`start`、`end` 可选 | 查询历史日 K；日期参数会原样传至现有行情 API |
| Go 行情 | `GET /market-api/api/v1/stocks/{symbol}/chip-distribution` | `X-API-Key` | 按需读取个股筹码分布；字段可缺失，按接口返回的质量状态和原因展示 |
| Go 行情 | `GET /market-api/api/v1/stocks/{symbol}/capital-flow` | `X-API-Key` | 按需读取个股资金流；5/10 日汇总可能为空，不能前端补造 |
| Go 分析 | `GET /analysis-api/healthz` | 不附加行情 API Key | 分析服务存活检查（数据中心“立即检查”使用） |
| Go 分析 | `GET /analysis-api/readyz` | 不附加行情 API Key | 分析服务就绪检查；当前可报告 MariaDB/Redis 依赖状态 |
| Go 分析 | `POST /analysis-api/api/v1/analysis/tasks` | JSON；可选 `Idempotency-Key` | 创建异步分析任务 |
| Go 分析 | `GET /analysis-api/api/v1/analysis/tasks/{task_id}` | task ID 作为路径段 | 查询任务状态 |
| Go 分析 | `GET /analysis-api/api/v1/analysis/tasks/{task_id}/events?after_sequence=0&limit=100` | 增量事件游标与单页条数 | 获取持久化阶段事件 |
| Go 分析 | `GET /analysis-api/api/v1/analysis/runs/{run_id}` | Run ID 作为路径段 | 读取单个 RunResult，并作为结构化分析结果展示的数据源；运行未结束时现有接口可能返回 409 `run_not_finished` |

## 分析任务与 Run 响应标识校验

任务状态响应必须包含与当前路径一致的 `task_id`；阶段事件响应必须包含相同的 `task_id`；Run 结果必须包含与请求路径一致的 `run_id`。缺失或不匹配的标识属于响应契约错误：前端不渲染错误内容，不把它当作瞬时网络故障反复重试。阶段事件身份错误只暂停事件续读，不阻塞任务状态的独立轮询。

## 2026-10-27 数据中心“立即检查”的真实调用范围

WebUI 已添加数据中心页面和手动“立即检查”操作。此页面当前实际调用：

- `GET /health/live` 与 `GET /health/ready`：检查 Go 行情服务存活/就绪。
- `GET /healthz` 与 `GET /readyz`：检查 Go 分析服务存活/就绪。
- `GET /api/v1/data-sources`：读取 Provider 配置与 capability 声明，需行情 API Key。
- WebUI/Nginx 页面路径：使用当前同源页面路径执行一次 GET；它只能确认此页面路径当时可响应，不代表容器内部每个进程都健康。
- MariaDB/Redis：只读取分析服务 ` /readyz ` 响应中的依赖子状态。分析服务不可达或未返回子状态时 UI 显示 unknown。

每项请求单独并发执行，最多等待约 6 秒；失败会显示状态、耗时和截断的安全错误摘要。此功能目前**没有**直接探测各 Provider 上游。Provider 清单中的 enabled/capability 只代表注册/配置声明，UI 会显示“已注册 · 未实测”。逐 Provider/capability 探测须等待 [数据能力缺口与服务巡检设计](./DATA_CAPABILITY_GAPS.md) 中描述的受保护诊断 API。

上述是当前前端行为，不代表本轮已运行测试或在 NAS/1Panel 上完成真实验收。
## 行情 Bundle 与快照一致性

- 未填写日 K 开始/结束日期时，WebUI 优先请求 `GET /market-api/api/v1/bundles/{symbol}?lookback_days=120&adjustment=forward_adjusted`，在一个响应中读取实时行情、日 K、质量警告、`snapshot_id` 与 `analysis_reference_time`。
- 页面用独立信息栏展示完整 `snapshot_id`、分析参考时间、采集完成时间、整体质量与服务端 `sources` 字段；支持复制脱敏后的快照元数据 JSON（不含 API Key）。
- `lookback_days` 依照服务端 OpenAPI 参数传递；WebUI 的回包适配以 `daily_bars` 为日 K 数组，并从返回 K 线的 `adjustment_basis` 展示实际复权口径。
- Bundle 可成功但不包含 `quote`；此时保留真实日 K，报价区明确显示未取得实时报价，不用旧报价或虚构报价填充。
- 只有 Bundle 路由返回 HTTP 404 时，才兼容旧服务并回退到 `/quotes/{symbol}` 与 `/bars/{symbol}/daily` 两个接口。其他 Bundle 错误不会立即再调用两套 Provider，避免无谓的重复负载。
- 用户填写了日 K 日期范围时，Bundle 不支持 `start/end`，因此继续使用实时行情 + 带日期范围的日 K 两个现有接口；结果不展示 Bundle 快照 ID。

## 日 K 日期范围参数

现有行情 API 的日 K 路由已支持可选 `start=YYYY-MM-DD` 与 `end=YYYY-MM-DD` 参数。WebUI 仅在用户填写日期时附加相应参数；不填时不发送空值，使用后端现有默认行为。日期范围仍受 `limit` 和后端允许的日期校验约束，前端按 `Asia/Shanghai` 当日检查未来日期与起止顺序。复权口径始终通过原有 `adjustment` 参数传递并在返回后展示。

## 后端待议：历史分析列表 API 建议（不是现有接口）

当前已存在按 ID 读取 Run 的 `GET /api/v1/analysis/runs/{run_id}`，但没有可供 WebUI 使用的服务端历史列表接口。浏览器端最近列表只保存少量 task/run 引用，不应据此声称具有跨设备历史、服务器检索或回放功能。

为支持后续的历史筛选与比较，建议由后端独立任务评估以下集合接口。**此处是需求草案，不代表接口已经实现，也不应由当前 WebUI PR 修改后端：**

```http
GET /api/v1/analysis/runs?symbol=600519&status=completed&task_type=post_market_scan&from=2026-10-01T00:00:00Z&to=2026-10-23T00:00:00Z&limit=20&cursor=<opaque>
```

建议约束：

- 默认每页 20 条、最大 100 条；使用 opaque cursor 稳定分页，并按创建/开始时间倒序、Run ID 次序稳定打破并列。
- 每条列表摘要至少包含 `run_id`、关联 `task_id`（如可用）、`status`、`task_type`、`symbols`、`started_at`、`finished_at`、`quality.core_pass`、`quality.partial` 与失败代码；不在列表中重复返回完整 RunResult。
- 筛选字段按后端现有 Run/Task 持久化字段确认；时间范围统一按带时区 RFC3339 解释。后端需说明数据保留策略、鉴权范围及记录是否已持久化。
- 点击一条历史摘要后仍通过已有 `GET /api/v1/analysis/runs/{run_id}` 读取完整报告；没有新 API 与持久化保证之前，前端只提供浏览器本地的最近任务引用。

## 已固定的前端调用行为

### 鉴权与传输

- 两组 API 使用同源路径；浏览器不直接拼接另一个站点的 URL。
- 行情服务的 `X-API-Key` 仅发送给数据源、实时行情和日 K 接口；不会发给行情健康检查或分析 API。
- 请求默认带 `Accept: application/json`；带请求体时使用 `Content-Type: application/json`。
- API 错误会尽可能保留 HTTP 状态、服务端错误码和请求 ID。非 JSON 的代理错误会使用截断后的响应文本作为诊断信息，避免无界错误内容进入页面。

### 分析请求与状态

当前 UI 的单股入口按已有调用契约提交：

```json
{
  "task_type": "post_market_scan",
  "symbols": ["600519"]
}
```

分析代码按现有 Go API 的六位 A 股代码规则规范化；行情查询可继续使用交易所后缀。前端不将 `post_market_scan` 改名成新任务类型，也不以 UI 标签代替后端业务语义。

## 任务创建表单与当前能力边界

- 任务请求使用表单输入框中的股票代码。WebUI 的“载入本地自选股”只从当前浏览器 `localStorage` 读取已有清单并填入表单，随后将任务类型设为 `watchlist_scan`；此操作不会自动提交任务，也不会把本地清单同步到服务端。
- `position_review` 仍是后端定义的任务类型，但当前 WebUI 将其禁用。当前 `internal/service/wiring.go` 的组合提供器只返回零可用现金、零组合权益和版本 `unconfigured-no-paper-ledger-v1`，没有真实持仓账本，因而不能称为实际持仓复核。
- 只有在后端接入并验证真实持仓/组合数据契约后，才应重新开放持仓复核入口。前端不通过空组合或固定占位结果模拟持仓信息。

异步任务状态与 Run 状态是两个层级，必须分别解释：

- Task：`queued`、`running`、`succeeded`、`partial`、`failed`。
- Run：`running`、`completed`、`blocked_data_quality`、`rejected_request`、`partial`、`failed`。

Task 状态表示异步任务的处理结果，Run 状态表示分析结果本身，两者不能混用。当前 Go Worker 的终态映射为：Run `completed` 或 `blocked_data_quality` 映射为 Task `succeeded`；Run `partial` 映射为 Task `partial`；Run `failed` 或 `rejected_request` 映射为 Task `failed`。特别是 `rejected_request` 即使成功生成了拒绝报告，也不能将 Task 标记为成功。

阶段事件采用 `after_sequence` 增量读取，后端返回的事件为空时不表示任务已完成；页面也不根据阶段数计算假百分比。创建任务时使用可选的 `Idempotency-Key`，便于同一请求在网络重试时避免重复创建。

## 重试与资源失效处理（WebUI 行为）

- 网络异常以及 HTTP 408、409、425、429、5xx 被视为可能暂时性失败；任务状态轮询对这些情况保留自动重试。
- 其他 4xx（例如 401/403 权限问题、404 任务或 Run 不存在）会停止自动任务轮询，避免对无效任务 ID 或错误配置每 5 秒无限请求；用户修正配置后仍可手动刷新。
- Run 结果读取最多进行 3 次有间隔的自动重试。再次切换任务或重新读取结果时，会使旧重试循环失效，旧响应不能覆盖当前任务的 UI。
- 这些重试只针对读取操作，不会自动重新提交分析任务，也不会改变后端任务状态。

## 契约文档覆盖缺口

目前不是所有端点都已集中在同一份 OpenAPI 中：

1. `analysis-tasks.openapi.yaml` 覆盖异步任务创建、状态与阶段事件，但尚未完整描述 WebUI 使用的 Run 结果读取端点。
2. 行情服务的健康检查、数据源、行情和日 K 路由由 Go 服务实现与调用代码约束，尚未在这份分析任务 OpenAPI 中统一描述。
3. 单个 `symbols` 配合 `post_market_scan` 的真实业务含义，以及运行中查询 Run 的精确错误/重试语义，仍需用实际 Go Worker 响应完成端到端验收。

因此，准确的说法是：**已有接口协议和实现，但统一、完整的机器可校验契约仍有文档覆盖缺口**。本 WebUI PR 不借机改后端协议；如果需要补充 OpenAPI 或调整业务语义，应基于 Go 路由、handler、领域类型与后端测试，在独立后端改动中确定和验证。

## 验证

从仓库根目录执行：

```bash
cd web
npm run check
npm test
docker build --tag stock-analysis-web:local .
```

CI 通过仅验证前端契约测试、JavaScript 语法和镜像构建；真实上游可达性、生产 API Key 与单股分析业务语义仍需 NAS/1Panel 联调确认。



## 联调验收状态矩阵

以下状态区分源码核对、自动化测试与真实部署验收。**CI 通过不等于 NAS 上的 WebUI 已与运行中的 Go 服务完成端到端联调。**

| 项目 | 已由源码/自动化覆盖 | 仍需 NAS / Docker 实测 |
|---|---|---|
| 实时行情 | 已核对浏览器请求路径、X-API-Key、Nginx /market-api/ 转发、Go quote 路由和 Quote JSON 字段；WebUI 单元测试检查请求 URL 与请求头 | 在真实 WebUI 上查询一只有效 A 股，确认数据源、行情时点、价格、质量状态与错误提示均来自运行中的服务；检查无效/缺失 Key、Provider 超时及市场休市时行为 |
| 历史日 K | 已核对 limit、adjustment、可选 start/end 参数，以及响应的 bars、adjustment_basis、quality_status 和 warnings；有 URL 构造测试 | 在真实服务验证无日期范围与有日期范围、不同复权口径、日期边界、空结果及 Provider 错误；确认表格、图表日期顺序和质量警告一致 |
| Provider 状态 | 已核对 GET /api/v1/data-sources 的 {sources, normalization_version} 包装和能力字段；前端按 enabled、quote_capable、daily_bar_capable 等字段渲染 | 确认部署配置加载后的 Provider 清单、启用开关和补充能力与实际容器配置一致。该接口是注册/启用能力清单，不应解读为对每个 Provider 的实时可用性探测 |
| 异步分析任务 | 已核对创建任务 202 / 幂等重放 200、{task,status_url,events_url} 包装、Task 状态结构、事件增量游标及错误信封；Go 单元测试和 WebUI 契约测试覆盖状态映射与请求路径 | 从 WebUI 实际创建任务，核对数据库持久化、排队/运行/终态；验证同一幂等键重放、冲突键 409、队列满 429、缺失任务 404 与服务不可用 |
| 阶段事件 | 已核对 after_sequence、limit、next_after_sequence，并确认空事件数组不能表示任务已完成；Go HTTP 测试覆盖增量读取 | 在真实 Worker 生命周期内验证事件可继续增量读取、序号递增、队列阶段为空时 UI 不误判完成，以及切换任务后旧轮询不会覆盖新页面 |
| 分析报告 | 已核对 GET /api/v1/analysis/runs/{run_id} 与 Go RunResult JSON 字段，前端结构化展示直接读取返回字段；Run 未完成时 409 run_not_finished 可触发有限重试 | 从已结束任务读取真实 Run，核对任务与 Run 状态、质量门禁、市场状态、因子、信号、风险/可交易性、计划、拒绝原因、警告及原始 JSON；核对 409 后最终结果、404 和失败状态的呈现 |
| 反向代理与部署 | Nginx 配置、同源路径、容器镜像构建已由源码/CI 覆盖 | 在真实 Docker 网络确认默认服务别名 stock-analysis-data:8091 与 stock-analysis-server:8092（或通过根目录 .env 的 MARKET_DATA_SERVICE_NAME 与 ANALYSIS_SERVICE_NAME 配置的实际别名）可解析、端口可达；再验证外部 OpenResty 路径重写、stock-analysis-web 容器健康状态、真实 API Key 配置及请求 ID 诊断 |

### 已验证边界

- **源码核对完成**：本表列出的路径、参数和响应字段来自当前 Git 分支的客户端、Nginx、Go Handler 与领域结构体，而非从 fixture 推断。
- **自动化已通过**：当前数据分支上的 Go test suite 与 Live Provider probe 对提交 7e47aec66cb000cf403ed0cfbbcbadfe655467a3 均显示成功；此前 WebUI 基线 CI 也通过。新增加的 WebUI 回归测试应以最新提交的 Actions 结果为准。
- **NAS/Docker 端到端仍待执行**：GitHub-hosted runner 的 Provider probe 不验证你的 NAS 上的容器网络、环境变量、OpenResty 路由、数据库持久化或真实 WebUI 行为。未取得这些运行结果前，不应标记为“真实服务已联调通过”。
- **认证/配置管理不在本轮**：当前 WebUI 的行情 API Key 存在浏览器本地；它不是用户登录系统，也不会修改服务器 .env。密码设置/删除及服务器 .env 导入导出需要独立的后端认证、权限、审计和安全配置接口。


## 个股补充研究接口

WebUI 提供一个**手动触发**的补充数据面板，调用现有接口：

- `GET /api/v1/stocks/{symbol}/chip-distribution`：使用 `symbol`、`date` / `provider_date`、`average_cost`、`profit_ratio`、`concentration_90`、`concentration_70`、`source`、`source_version`、`fetched_at`、`quality_status` 和 `quality_reasons` 等服务端字段。
- `GET /api/v1/stocks/{symbol}/capital-flow`：使用 `main_net_inflow_yuan`、`inflow_5d_yuan`、`inflow_10d_yuan`、`observations`、`source`、`source_version`、`fetched_at`、`quality_status` 和 `quality_reasons` 等服务端字段。

筹码比例字段由后端规范化为 `[0,1]` 小数，UI 仅将其乘以 100 后格式化为百分比，不重新计算筹码数据。资金流以人民币元展示，空的 5/10 日汇总仍显示“—”。两个请求并行且独立呈现：其中一个接口失败不会覆盖另一个成功结果，也不会中断行情查询、日 K 或分析流程。面板不会自动请求，避免未启用的补充 Provider 在主流程中引入额外请求或等待。


## API 与数据库的职责边界

WebUI 只通过同源 HTTP 路径调用 Go 服务，Nginx 根据路径把请求转发到行情服务或分析服务。浏览器端不会直接连接 MariaDB 或 Redis，也没有单独的 WebUI 数据库。

- **浏览器 `localStorage`**：只保存行情 API Key、最多 50 条自选股（代码/自定义名称/更新时间）与最多 10 条最近任务引用（task/run ID、标的、类型、状态、更新时间）。不保存完整 RunResult、报告正文或分析过程事件。
- **Go 行情服务**：通过自己的配置连接 Provider、MariaDB 和 Redis。WebUI 的请求本身不会直接写数据库；行情数据是否由后端持久化/缓存取决于具体端点实现。
- **Go 分析服务**：异步任务先写入 MariaDB，阶段事件、分析结果与快照由后端持久化；Redis 作为缓存使用，MariaDB 是最终事实源。WebUI 通过 API 查询这些服务端记录。

本地移除最近任务、清空自选股或清除站点数据，只会改变浏览器本地状态；不会删除服务端任务、RunResult 或数据库记录。


### 行情服务 API Key 由谁生成

`MARKET_DATA_API_KEY` 是部署管理员自行生成的内部服务访问密钥，不是行情 Provider 的个人 API Key，也不是 WebUI 自动签发的令牌。建议在部署服务器上通过 `openssl rand -hex 32` 生成，并把同一值配置给 `stock-analysis-data` 和 `stock-analysis-server` 的 `MARKET_DATA_API_KEY` 环境变量。WebUI 的设置项只将该值保存在浏览器 `localStorage`，调用受保护的行情路由时发送 `X-API-Key`；它不会更新后端配置。该共享 Key 不等于用户登录鉴权，公网访问仍必须额外配置访问控制。


## 任务运行流前端呈现边界（Issue #4）

本功能复用现有 Task、StageEvent 与 RunResult 接口，不增加后端路由：

- Task 状态使用现有的 `queued`、`running`、`succeeded`、`partial`、`failed`；Run 状态单独使用现有的 `running`、`completed`、`blocked_data_quality`、`rejected_request`、`partial`、`failed`。不能把 Task 的成功映射直接显示成 Run 已完成。
- Task 元数据按当前 OpenAPI/JSON 读取 `task_id`、`run_id`、`task_type`、`current_stage`、`completed_stages`、`created_at`、`started_at`、`finished_at`、`failure_code`、`error_message`。可选时间戳缺失时显示未报告，不从浏览器时间推算任务开始/结束。
- StageEvent 以服务端 `sequence` 排序并用现有 `after_sequence` 增量续读。仅当事件真实提供 `duration_ms` 时显示耗时，不根据前端接收时刻反推；只有返回真实 `error_code` 时才显示该字段。
- 运行流是线性时间线，不构造未由 API 提供的 DAG、进度百分比或实时推送。空事件列表只表示当前没有读到持久化事件，不代表任务完成。
- 当前没有服务端 Task 列表 API。可重新打开和刷新恢复的是浏览器本地保存的 task/run 引用；这不是跨设备的服务端历史。详情打开后仍通过既有 Task / StageEvent / RunResult 接口重新读取。


## 按中文名称/代码搜索证券（Issue #11）

新增接口在数据分支 PR #1 中实现，WebUI 分支可独立开发并在部署未更新时明确提示接口不可用。部署版本尚未包含该接口时，搜索请求返回的 404 会显示“请更新 market-data 服务”，不会回退到硬编码股票清单。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/stocks/search?q=贵州茅台&limit=10` | 按中文名称、代码或标准证券代码搜索 A 股目录 |

请求必须提供 `X-API-Key` 或 `Authorization: Bearer`。参数 `q` 会去除首尾空白，不能为空，最多 64 个 Unicode 字符；`limit` 默认 10，允许 1–50。成功响应包含 `query`、`total`、`items`、`source`、`source_version`、`fetched_at`、`quality_status` 与可选 `quality_reasons`。每个结果包含规范化的 `symbol`（如 `600519.SH`）、`code`、`name`、`exchange`。

证券身份来自真实的东方财富 A 股证券目录字段，名称/代码匹配排序为精确匹配、前缀匹配、包含匹配；不会返回行情价格。`fetched_at` 是目录采集时间，不代表实时报价时间。目录按进程缓存 6 小时并合并并发刷新；刷新失败但有旧目录时返回 `quality_status=stale` 和原因，目录不完整则标记降级，无任何可用目录时返回 Provider 错误。前端通过防抖、取消旧请求和响应代次校验，避免慢请求覆盖用户最新输入；支持中文输入法、方向键、Enter、Esc 与鼠标选择。


## Run Flow 阶段增强（Issue #12 Phase 1）

Phase 1 仍使用已有接口 `GET /api/v1/analysis/tasks/{task_id}`、`GET /api/v1/analysis/tasks/{task_id}/events?after_sequence=N&limit=100` 与 `GET /api/v1/analysis/runs/{run_id}`，不新增后端路由。

- 摘要栏中的事件数量和最新事件时间来自已成功读取的 StageEvent 集合；事件接口尚未成功读取时显示等待状态，不把它显示为 0 条。Run 未返回前，质量门禁和警告数显示等待结果。
- 点击阶段事件可查看该事件的服务端序号、阶段标识、状态、事件时间、可选错误码/耗时与原始 JSON。没有服务端提供的字段就不显示，不通过浏览器时间推算耗时。
- 默认优先选中近期的失败、错误、阻断、降级或运行中事件；用户选择在同一组事件轮询更新时保持。该排序只改善诊断入口，不表示阶段依赖关系。
- 事件列表以键盘可访问的按钮呈现，使用真实事件序号做选择；不把事件序列转换成 DAG，不显示虚假百分比，也不提供目前不存在的取消任务操作。
- 下一阶段完整图形化 Run Flow 仍依赖数据服务明确的 Stage Registry 和持久化 Flow Snapshot API，边关系必须来自真实 Runner 定义。


## 市场概览新增 API（真实行情服务端点）

市场概览使用四个互相独立的只读请求。浏览器使用同源 /market-api/ 前缀，Nginx 转发至 Go market-data；四个端点均要求行情 API Key，并通过 X-API-Key 发送。

| 请求 | 参数和展示边界 |
|---|---|
| GET /market-api/api/v1/market/indices | 显示 items、latest_price、change、change_percent、quote_time、source、fetched_at、quality_status；change_percent 使用百分点口径。 |
| GET /market-api/api/v1/market/breadth | 显示上涨/下跌/平盘/未分类家数、total_securities、covered_securities、来源/时间/质量。total_securities 是来源分布桶汇总，不是经证券主数据独立核验的总量；未提供全市场成交额时明确显示未提供。 |
| GET /market-api/api/v1/market/sectors?type=industry&limit=10 | type=industry/concept，limit 默认 10、范围 1–100；按服务端排名展示，涨跌幅使用百分点。 |
| GET /market-api/api/v1/market/limit-pools?type=up&limit=30 | type=up/down/broken，limit 默认 30、范围 1–100，date 可选；has_data=false 不能解释为 0 只股票。 |

各模块独立处理加载、空/缺字段、认证错误、接口 404 和上游失败。旧版 market-data 返回 404 时仅该模块提示升级服务；质量等级、来源时间和质量原因保留真实后端值。

## 个股档案、板块、财务与公告 API

| 请求 | 参数和说明 |
|---|---|
| GET /market-api/api/v1/stocks/{symbol}/profile | 证券主数据：证券类型、上市状态、上市/退市日期、来源和质量状态。 |
| GET /market-api/api/v1/stocks/{symbol}/boards | 个股所属行业/概念板块；has_data=false 代表源端没有确认数据，不代表确认没有板块。 |
| GET /market-api/api/v1/stocks/{symbol}/fundamentals?year=2026&quarter=2 | 显式财报年度与季度，返回按盈利、营运、成长、偿债、现金流和杜邦分类的指标。数值保留源端单位；report_date 与 publication_date 分开显示，历史研究应依据披露日期判断当时可见性。 |
| GET /market-api/api/v1/stocks/{symbol}/announcements?page=1&page_size=20 | 默认查询中国时区最近 7 个日历日；响应包含源端公告日期、可解析时才存在的发布时间、采集时间、质量状态及 source_url。空列表和 unknown 质量不证明没有公告；分页使用 has_more。 |

个股研究区按需加载，不阻塞报价、日 K 或分析任务。每个请求独立处理认证失败、Provider 未启用、旧服务 404、空数据和质量降级；公告链接仅允许 HTTPS 的 Eastmoney 官方域名。

## 龙虎榜与交易日历 API

| 请求 | 参数与数据解释 |
|---|---|
| GET /market-api/api/v1/market/dragon-tiger?date=2026-10-26&page=1&page_size=50 | 日期可省略（默认中国时区当日）；page 从 1 开始，page_size 为 1–500。展示 A 股明细、净买入/成交金额（人民币元）、涨跌幅及成交占比（百分点）、上榜原因、质量与获取时间。source_returned 和 excluded_non_a_share_count 用于解释源端原始记录及被过滤的非 A 股产品；has_data=false 不等于确认当日无人上榜。 |
| GET /market-api/api/v1/market/trading-calendar?start=2026-10-01&end=2026-10-31 | start/end 必填、闭区间。页面展示服务端逐日 is_trading_day 标记，不以星期几推断交易状态，并核对 items 数量与 total_days 的一致性。 |

龙虎榜支持服务端分页，并可跳转到已有个股行情查询。交易日历支持指定范围和月份快捷切换。Provider 未启用、旧服务缺少接口或源端尚未发布数据时明确显示状态。前端按请求范围的真实日期对齐网格；缺失日期显式标示“未返回”，`is_trading_day` 缺失或非布尔值显示“未确认”，不得误显示为“休市”。

## Provider 能力清单的前端映射

行情总览的数据源面板按接口实际返回的布尔字段逐项显示 Provider 声明的能力：实时行情、日 K、证券档案、行业/概念排行、核心指数、市场涨跌分布、涨跌停池、个股所属板块、季度财务指标、个股估值指标、个股人气榜/飙升榜、公告、龙虎榜、交易日历、筹码分布与个股资金流。字段缺失或 false 时不显示为已支持；它只表示当前回包未声明对应能力，不等于服务整体不可用。Provider 的启用状态与具体能力分开呈现。

## 主行情、日 K 与统一快照响应标识校验

实时行情、日 K 与 Bundle 统一快照若带有顶层 `symbol`，WebUI 会与本次查询标的比较；兼容纯六位代码和已知交易所后缀。顶层 Bundle 代码不匹配时整份快照忽略；Bundle 内嵌实时行情代码不匹配时只忽略该行情子项并显示警告；独立行情或日 K 代码不匹配时将该子请求标记为失败，另一项有效响应仍可单独显示。缺少 `symbol` 的旧版响应保持兼容。

## 单股研究响应标识校验

单股公司档案、板块、财报、公告、筹码和资金流响应若包含顶层 `symbol`，WebUI 会将其与当前请求标的比较（兼容已知 A 股六位代码与交易所后缀形式）；代码不匹配时忽略响应并提示错误，不把其它股票的数据放入当前面板。旧版本响应未提供 `symbol` 时保留兼容行为，不凭空推断代码。

## 个股估值快照

| 方法 | 路径 | 参数 |
|---|---|---|
| GET | `/market-api/api/v1/market/valuations?symbols=600519.SH` | `symbols` 为逗号分隔的 1–100 个规范 A 股代码，每个代码需带 SH/SZ/BJ 后缀。 |

当前数据分支 `feat/full-go-data-providers-api-switch` 的 `services/market-data/internal/httpapi/server.go` 已注册此路由，响应定义可见 Go 模型 `ValuationSnapshot` / `ValuationItem`；数据分支已在提交 [`c37270a`](https://github.com/Kasign/stock_data_provider/commit/c37270a41a593488767982e5da819d99ba9f2189) 中补齐该路由及 `ValuationSnapshot` / `ValuationItem` Schema，同时修复了 `SourceStatus` 声明并补齐相关 Provider 能力字段。旧部署仍可能返回 404；无启用估值 Provider 时会返回 503。旧部署可能返回 404，前端会提示更新 market-data；无启用估值 Provider 时会显示服务端 503，不伪造指标。

响应快照包含 `requested_symbols`、`total`、`items`、可选 `missing_symbols`、可选 `provider_timestamp`、`source`、`fetched_at`、`quality_status` 和质量原因。 WebUI 允许既有输入框中的纯六位代码用于查询；调用估值接口前，对可识别的 A 股代码补齐交易所后缀（6 开头补 `.SH`，0/3 开头补 `.SZ`，4/8/92 开头补 `.BJ`），已带后缀的代码保持不变。无法从代码前缀可靠识别交易所时，前端提示用户输入后缀，不发送猜测的估值请求。每只股票的指标字段为 `pe_ttm`、`pe_mrq`、`pb_mrq`、`ps_ttm`、`pcf_ttm`，可以为 null 或负数；前端保留该语义，不补零，不据此生成买卖信号。


## 个股筹码分布与资金流 API

| 请求 | 数据解释 |
|---|---|
| GET /market-api/api/v1/stocks/{symbol}/chip-distribution | 返回单只 A 股的平均持仓成本、可用的 90%/70% 成本区间、获利比例与成本集中度，并带来源、数据日期、获取时间和质量原因。比例字段由服务归一化为 0–1，UI 显示为百分比；可选字段缺失时不补零。 |
| GET /market-api/api/v1/stocks/{symbol}/capital-flow | 返回主力净流入、可用的 5/10 日净流入合计、实际观测数量、来源、获取时间和质量原因。金额单位为人民币元；5/10 日合计只有在必需观测值完整时才会填充。 |

以上路由已在 Go Handler 注册，并已在数据分支提交 b02eb41 中补入 services/market-data/openapi.yaml。这些补充接口由用户单独触发，不阻塞实时行情或日 K；Provider 未启用时显示真实服务端错误，不使用静态样本代替。


## 个股人气榜与飙升榜

| 方法与路径 | 参数 / 响应说明 |
|---|---|
| `GET /market-api/api/v1/market/hot-stocks?type=popular&limit=20` | `type` 支持 `popular`（人气榜）与 `surge`（飙升榜）；`limit` 范围 1–100。响应包含 Provider 的 `rank`、股票代码和名称、可选最新价/涨跌、可选 `rank_change`、来源、采集时间和质量信息。 |

该接口已由数据分支 `services/market-data/openapi.yaml` 描述并由 Go Handler 注册。它是有界榜单，`total` 只表示本次返回的记录数，不能当作全市场覆盖率。上游没有可独立核验的行情快照时间，因此成功快照可能仍为 `degraded`；`fetched_at` 是采集时间，不应冒充行情发生时间。前端原样展示 `rank_change` 的源端数值，不推断正负号代表名次上升或下降。

## 前端本地计算的技术指标（无新增 API）

技术指标视图仅使用当前日 K 请求或 Bundle 已返回的 bars 收盘价计算，不发送额外 API 请求、不新增后端字段，也不把指标写入服务端。支持价格图叠加 MA5/MA20/MA60，并在独立面板展示 RSI(14)、MACD(12,26,9) 的数值和历史曲线。

| 指标 | WebUI 口径 | 样本不足时 |
|---|---|---|
| MA5 / MA20 / MA60 | 最近 N 个有效收盘价的简单算术平均；叠加线按交易日期与现有日 K 对齐 | 显示“样本不足”，不绘制不存在的值 |
| RSI(14) | 使用 14 个有效收盘价变化，采用 Wilder 平滑；首次可见至少需要 15 个有效收盘价 | 数值与曲线均显示样本不足提示 |
| MACD(12,26,9) | EMA12 / EMA26 分别用首个 12 / 26 个有效收盘价的 SMA 初始化；DIF=EMA12−EMA26；DEA 对首个 9 个 DIF 取平均后采用 EMA 平滑；柱值=2×(DIF−DEA) | 完整 DEA/柱值通常需要至少 34 个有效收盘价 |

缺失、null、非数值或非正价格不会当成 0。技术指标是基于当前服务返回序列的描述性统计，不具备 point-in-time 历史修订保证；页面明确提示不构成买卖信号。图表切换、均线显隐、RSI/MACD 曲线不影响后端请求 payload 或 RunResult。



## 未接入功能的后端契约需求（设计草案，不是现有接口）

> 本节描述通知、问股、选股、告警与服务端设置所需的资源和语义，**不是可调用的 endpoint 清单，也不代表 Go Handler 已实现**。当前 WebUI 对这些能力初始化为 supported=false，写操作保持禁用。后端开始实现前须审核需求、确定真实路由并写入对应 OpenAPI；在此之前，前端不得猜路径或启用虚构交互。

实施时必须先阅读[《WebUI 与数据层状态及响应契约》](https://github.com/Kasign/stock_data_provider/blob/5776313e7b50648ea0e7d741c7eb1c02b8d20758/docs/architecture/WEBUI_DATA_STATE_CONTRACT.md)。该文档定义跨层共同规则，包括请求状态与数据质量分离、空集合语义、时间/单位、错误信封、查询条件回显与分页行为。

### 1. 通知中心与投递配置（Issue #7）

当前缺少持久通知收件箱、未读状态/计数、标记已读/未读和通知历史查询接口。建议后端契约覆盖：

- **通知列表与详情**：支持稳定分页；每条记录至少区分通知 ID、标题/摘要、来源类别、严重级别、服务端创建/发布时间、已读状态、关联 Task/Run/告警 ID（有则提供）和详情/诊断字段。未读数必须来自服务端，不能从当前页面已加载的记录数量推导。
- **阅读状态变更**：标记已读/未读必须持久化、授权检查并返回最终状态；重复调用应定义幂等语义。前端只能在服务端确认后更新记录状态。
- **投递诊断**：告警通知发送尝试与收件箱通知是两种不同资源。投递记录应有独立 attempt ID、渠道、状态、尝试时间、结束时间、错误码/安全摘要及可用的 request ID。不得把告警触发直接等同于通知发送成功。
- **过滤条件**：来源类别、已读状态、级别、关键词、时间范围及分页参数要在契约中定义；返回数据应说明筛选后的总数/是否还有下一页（可提供时）。

### 2. 问股与 Agent Chat（Issue #8）

当前没有已核实的 Chat、会话、消息、流式输出或服务端取消 API。建议契约至少覆盖：

- **会话资源**：服务端创建、列出、读取与删除会话；会话所属用户/权限范围、标题、创建/更新时间和当前状态由服务端定义。新建会话不得自动复用另一个会话的股票或消息上下文。
- **消息资源**：用户消息和模型回复有不同的 role、message ID、时间与状态；引用股票/分析报告的上下文必须显式绑定到该会话或请求，不得由 UI 暗中串用旧会话状态。
- **生成生命周期**：如果使用 SSE/其他流式协议，需定义开始、增量 token、完成、失败和断线恢复事件及关联 ID；取消操作必须对应明确的生成/请求 ID。服务端若不支持取消，UI 不应显示“已停止”。
- **模型/策略配置**：可选的模型或分析模式必须来自服务端真实能力与 schema。回复不可用时返回结构化错误；不得用普通 RunResult 或固定文本冒充模型回答。

### 3. 条件选股（Issue #9）

当前没有已核实的策略清单、扫描任务、候选股、候选解释或历史 API。建议后端契约拆分为：

- **能力与策略 schema**：明确能力开关和支持状态；策略清单应包含稳定 ID、显示名、可用性与 schema 版本。动态参数需声明类型、必填、范围、枚举值、默认值及错误提示，不能仅由前端硬编码。
- **扫描任务**：创建操作需返回真实 task ID、接受状态与状态查询引用；运行中的 Task 和最终扫描结果状态分开定义。扫描专属状态如无后端映射约定，不得套用多股票分析任务状态来假冒。
- **候选集合**：每个候选记录需区分证券标识、真实评分（如该策略确实计算）、解释/命中条件、所用数据快照/参考时点与质量警告。没有评分或解释时保持缺失，不由 UI 推导。
- **历史和分页**：支持按策略、执行时间、状态分页查询过去的真实扫描；说明数据保留与结果是否可复现。空候选必须由真实成功响应确认，而不能将 API 不可用显示为“0 只”。

### 4. 告警规则、评估结果与投递尝试（Issue #10）

告警配置、一次评估、后台触发历史和消息投递结果必须是分离的资源/状态，不可使用单个布尔值或一个统一列表代替。

- **规则 schema 与持久化**：明确支持的规则类型、范围、参数类型与边界、启停状态、严重级别、冷却/去重策略和版本；保存/修改/删除需要授权、校验和持久化结果。
- **单次测试与后台评估**：返回明确的评估状态（例如触发、未触发、降级、跳过、执行错误）；数据不足或评估错误不可静默映射成“未触发”。
- **触发历史**：真实规则 ID、证券/范围、规则版本、评估参考时间、触发状态和质量/原因应可审计。后台调度和重复事件去重由服务端保障，而非依赖浏览器保持运行。
- **通知投递尝试**：独立记录渠道、投递状态、尝试次数、时间与安全错误摘要。规则触发不保证通知已发送；部分渠道失败不得被整体“成功”掩盖。

### 5. 服务端设置和密钥配置（Issue #5）

当前 WebUI 只有浏览器侧 API 路径与行情 API Key 设置，不具备服务器通用设置 CRUD、认证/密码、.env 修改或导入导出接口。若今后需要服务器设置中心，建议先建立有权限控制的配置契约：

- 读取 schema 与配置时返回字段类型/限制、可编辑性、敏感字段标记、服务端版本/ETag；秘密字段读取时返回“已配置/未配置”而不是明文密钥。
- 更新操作使用明确的版本冲突/乐观并发语义，并区分校验失败、无权限、版本冲突、保存失败和成功后的版本。
- 单渠道通知测试等操作应明确说明测试目标、请求 ID、真实执行结果与错误；不能用前端静态校验冒充成功。
- **部署环境变量仍属于部署/运维边界**。根目录 .env.example 中的 MARKET_DATA_SERVICE_NAME、ANALYSIS_SERVICE_NAME、WEBUI_BIND_ADDR、WEBUI_PORT 用于 Compose/Nginx 部署；不应通过浏览器把秘密返回给 WebUI，也不应将直接修改服务器 .env 当成普通设置表单功能。

### 6. 新能力进入开发的最低门槛

1. 确认由哪个 Go 服务负责该资源，确定权限/身份边界、字段 schema、稳定错误码和分页规则。
2. 在对应服务代码中实现真实 Handler、领域/持久化行为与 OpenAPI；文档明确它是新接口而不是已有能力。
3. 添加成功、空结果、缺字段、降级/质量原因、权限失败、参数校验失败、上游/内部错误、重复写入与并发请求的契约测试（按资源适用）。
4. WebUI 按共享契约实现 loading、empty_unconfirmed、empty、error、unavailable 等请求状态，同时独立展示 quality_status。页面字段缺失、null 和数值 0 必须维持不同语义。
5. 只有真实部署确认路由已注册后，才把对应视图从 supported=false 切换到实际 API 能力；不能只因 mock/fixture 或静态断言存在就启用按钮。

以上均为后续契约需求，不扩大当前 WebUI PR 的后端变更范围。


## 2026-10-27 产品排版与新增数据需求登记

本轮新的页面结构见 [UI_LAYOUT_PLAN.md](./UI_LAYOUT_PLAN.md)；当前未实现数据能力与后端诊断需求以 [DATA_CAPABILITY_GAPS.md](./DATA_CAPABILITY_GAPS.md) 为集中登记入口。

下面是需求登记，**不代表路由已实现**：
- 市场情绪历史快照与按交易日/时间范围查询；
- 市场级资金净流入，以及板块级资金净流入；
- 板块成交额排行的完整候选/排序/分页、板块近 5 日表现；
- 通用全球指数目录/实时快照、币种、时区和交易日语义；
- 面向研究列表的批量股票概览与统一 MA/成交活跃度字段；
- 逐服务依赖、Provider 和 capability 的真实探测与最近结果。

### 服务状态与 Provider 巡检（设计草案）

现有实际接口：
- 行情服务：`GET /health/live`、`GET /health/ready`、`GET /api/v1/data-sources`。
- 分析服务：`GET /healthz`、`GET /readyz`；ready 响应可返回 MariaDB 和 Redis 的依赖状态。
- `/api/v1/data-sources` 是 Provider 配置/能力清单，不是实时上游健康检测。

为数据中心的“立即检查”后续设计一组受保护的诊断操作和最近结果读取能力。每条探测需要返回 service/dependency/provider/capability、状态、checked_at、耗时、安全错误摘要、最近成功时间（存在可信记录时）、source/version 和数据质量。Provider 的不同 capability 必须分别探测，不能因为行情接口成功就推断该 Provider 的公告、财务或资金流也正常。

诊断操作必须限制并发、单项/总超时和频率；不能允许浏览器传入任意 URL、直连 MariaDB/Redis 或暴露密钥/Cookie/内部敏感地址。若分析服务不可达且无其他独立证据，数据库/Redis 状态显示 unknown，而不是推断为故障。以上均为需求草案，需先实现 Handler、模型/OpenAPI 与安全边界，再由 WebUI 接入。

### 市场总览新增指标（设计草案）

- 市场广度 `total_amount_cny` 与板块 `amount` 代表成交额，不等于资金净流入。
- 市场情绪原始指标必须定义分子/分母、覆盖范围、交易日/采集时间及缺失语义。历史图表需要定时持久化快照，当前快照不得重复用作历史。
- 板块多维排名需要明确维度、排序方向、完整候选集/总数、分页、报告日期、来源及质量；近 5 日表现依赖稳定的板块标识和历史数据。
- 全球行情需要资产标识、交易所/市场、币种、时区、交易时段、当地交易日期、行情时间、延迟说明与质量。
- 个股概览批量 API 需要明确证券集合/分页、批量字段、MA 算法版本、日 K 复权口径、最后有效交易日和逐标的质量状态。


## 浏览器本地工作区导入/导出（无新增 API）

设置中心的“浏览器本地数据”分组只提供前端本地 JSON 文件功能，不调用服务端配置 API，也不代表服务器备份。导出 schema 带有 format 与 version，数据字段使用 allowlist：

- theme：当前 dark/light 主题；
- connection.marketBase、connection.analysisBase：同源绝对 API 路径，不允许协议、外部主机、查询、片段或空白字符；
- watchlist：当前浏览器最多 50 条自选股；
- recent_tasks：当前浏览器最多 10 条任务引用，保留任务 ID、Run ID、标的、类型、状态、分析服务路径与更新时间。

**不导出、不导入 API Key**。导入时保留浏览器已有 API Key；若当前浏览器尚未设置 Key，导入后仍为空。行情报价、日 K/图表缓存、完整分析报告、服务器 .env、服务端设置/密码都不在文件中。

导入要求版本、字段、条数、证券代码和重复项校验通过，并要求用户明确确认覆盖。任务引用包含标的/任务标识，应像其他本地研究资料一样妥善保管；不要将其误认为服务端完整历史，或不经审查直接公开分享。文件损坏、格式版本不支持或本地存储写入失败时，UI 显示错误；浏览器存储回滚属于尽力处理，不构成服务器级事务保证。
 
## 数值字段类型边界
 
前端数值格式化仅接收有限 number 或非空数字字符串。布尔值、对象、数组、空白字符串、null 与 undefined 均按缺失/不可用展示，不通过 JavaScript 隐式转换变成 0 或 1；这只影响显示与正负样式，不改写 API 响应或质量状态。

 
## 任务创建结果不完整时的处理
 
任务创建接口返回成功状态但响应缺少有效 `task_id` 时，WebUI 不会把它简单标记为“创建失败”，而是提示任务创建结果暂时无法确认，并建议先核对服务端任务记录，以免盲目重复提交。若提交期间分析 API 地址发生变化，仍优先提示核实旧连接；只有有有效 task_id 时才会将旧连接任务保存到最近任务列表。

 
## 任务轮询响应契约错误与网络错误区分
 
任务状态必须是 `queued`、`running`、`succeeded`、`partial` 或 `failed`。任务状态响应结构错误、状态值未知以及阶段事件响应结构无效都归类为响应契约错误，不重复网络重试；阶段事件请求暂停时，任务状态轮询仍可独立工作。仅真正可重试的网络/服务端错误使用退避重试。

 
## Run 结果与当前任务标的绑定
 
读取 Run 结果时，前端同时检查请求的任务 ID、Run ID、分析 API 地址代际和当前任务标的集合。当前任务标的已知时，RunResult 的 `request.symbols` 必须存在并与任务标的集合相同；比较时忽略顺序、重复项和已知 A 股交易所后缀差异。缺失列表或标的集合不一致视为响应契约错误，不渲染该报告、不进行网络错误重试。仅当旧本地任务引用没有可用标的列表时，才跳过标的集合检查以兼容历史记录。

 
## Task 与 Run 关联稳定性
 
任务状态响应除了必须匹配当前 `task_id`，还必须返回非空 `run_id`。如果当前任务已经建立了 Run 关联，后续状态响应的 `run_id` 必须保持一致；缺失或改变关联视为响应契约错误，不覆盖当前引用，也不自动打开另一个 Run 的报告。恢复历史任务时若本地引用没有 Run ID，则允许首次有效任务状态回包补齐关联。

 
## 个股研究股票代码比较
 
行情 API 允许纯六位代码与带交易所后缀的 A 股代码。WebUI 在判断当前 Quote、研究摘要、自选列表行与日 K 均线是否属于同一标的时，统一按已知前缀补齐交易所后缀后比较；因此 `600519` 与 `600519.SH` 视为同一证券。研究摘要的 MA5/MA20 也必须通过 `sameMarketSymbol` 判断日 K 标识与当前查询是否对应，不能使用原始字符串严格比较。报价价格、行情时间、来源与质量状态仅在 Quote 匹配当前股票时显示；报价缺失不代表同一标的的有效日 K 一定无效，但股票切换或日 K 请求失败后不得沿用旧均线。无法规范化的标识不匹配。

 
## 自选股报价与最近任务标的比较
 
自选股本地记录、当前行情 Quote、最近任务与活动任务的代码比较统一采用规范化后的 A 股代码；已知纯六位代码与带交易所后缀代码视为同一标的。自选卡片的行情时间、质量状态与价格只在当前 Quote 匹配该卡片时显示，不匹配时提示尚未加载。 分析 API 地址变更时，活动任务与已载入 Run 会从当前页面状态中清空；若自选股页正处于显示状态，必须立即重新渲染卡片，避免旧 task_id/Run 结果残留。最近任务引用继续保留原分析地址，不能发送到新地址。 当自选股页保持可见时，后台 Task 状态轮询和 Run 结果读取完成后也应重渲染卡片，使活动任务状态与“本页已载入结果”摘要及时更新；此展示仅使用当前 Task/Run 的标识、标的和分析地址匹配结果。 最近任务与今日分析的本地记录可能来自其他已配置过的分析 API 地址；当 `analysis_base` 与当前设置不一致时，卡片会显式标注“旧分析 API 地址”。这类记录可以作为本地历史线索，但不能解读为当前分析服务的任务状态或结果。

 
## 阶段事件条目的必需字段
 
阶段事件数组中的每个条目必须包含 JSON 数值型正整数 `sequence` 和非数组对象 `event`。前端不使用 `Number()` 隐式转换序号，因此布尔值、数字字符串、小数、非安全整数以及缺少/错误类型的 `event` 都会被忽略，并在阶段事件读取提示中计入无效条目数。

 
## 阶段事件增量合并
 
增量事件合并会保留此前已读取的事件：当前批次中的无效条目、已存在序号或不大于请求前游标的旧序号都会被忽略，不能覆盖既有事件内容。新批次内即使返回顺序不一致，只要序号合法且未重复，仍按序号存储并由 UI 排序显示。

 
## Task 状态与 Run 报告的标的集合一致性
 
任务状态回包如果包含 `symbols`，该字段必须为数组；当前任务已有明确标的时，服务端回包的标的集合必须匹配当前任务。RunResult 的 `request.symbols` 使用同一规范化集合比较，去重并忽略输入顺序及已知交易所后缀差异。任务状态回包缺失可选 `symbols` 时保留当前任务已有集合；不匹配的数据不会覆盖任务上下文。
