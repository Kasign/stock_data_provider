# WebUI 当前 API 集成契约

更新日期：2026-10-23

## 结论

WebUI 使用的 HTTP API **不是现在从零新定的**。行情服务与 Go 分析服务已有实际路由、实现和部分 OpenAPI/部署文档。本文件是前端当前使用范围的接口清单，便于联调和 Review；它不是新版本 API，也不取代 Go 服务自己的协议定义。

现有的权威参照：

- [分析异步任务 OpenAPI](../docs/architecture/analysis-tasks.openapi.yaml)：任务创建、任务状态、阶段事件和请求/响应 schema。
- [仓库 Go API README](../README.md)：服务拆分、异步任务调用示例、事件续读和 Run 查询说明。
- [Go RunResult 领域模型](../internal/analysis/domain/types.go)：分析结果字段与 JSON 名称。
- [WebUI API 客户端](./src/api.js)：本页面实际发出的请求路径、参数和请求头。
- [WebUI Nginx 代理](./nginx.conf)：浏览器同源路径转发到两个 Go 服务的方式。

## 当前前端调用清单

以下路径中的前缀是浏览器路径。Nginx 会去除代理前缀，再转发至对应上游服务。

| 服务 | 浏览器请求 | 请求参数/请求头 | 用途 |
|---|---|---|---|
| Go 行情 | `GET /market-api/health/ready` | 不附加 API Key | 行情服务就绪检查 |
| Go 行情 | `GET /market-api/api/v1/data-sources` | `X-API-Key` | 读取 Provider 能力/状态 |
| Go 行情 | `GET /market-api/api/v1/quotes/{symbol}` | `X-API-Key`；股票代码作为路径段 | 查询实时行情 |
| Go 行情 | `GET /market-api/api/v1/bars/{symbol}/daily?limit=120&adjustment=forward_adjusted&start=2026-10-01&end=2026-10-23` | `X-API-Key`；`limit`、`adjustment` 可配置；`start`、`end` 可选 | 查询历史日 K；日期参数会原样传至现有行情 API |
| Go 行情 | `GET /market-api/api/v1/stocks/{symbol}/chip-distribution` | `X-API-Key` | 按需读取个股筹码分布；字段可缺失，按接口返回的质量状态和原因展示 |
| Go 行情 | `GET /market-api/api/v1/stocks/{symbol}/capital-flow` | `X-API-Key` | 按需读取个股资金流；5/10 日汇总可能为空，不能前端补造 |
| Go 分析 | `GET /analysis-api/readyz` | 不附加行情 API Key | 分析服务就绪检查 |
| Go 分析 | `POST /analysis-api/api/v1/analysis/tasks` | JSON；可选 `Idempotency-Key` | 创建异步分析任务 |
| Go 分析 | `GET /analysis-api/api/v1/analysis/tasks/{task_id}` | task ID 作为路径段 | 查询任务状态 |
| Go 分析 | `GET /analysis-api/api/v1/analysis/tasks/{task_id}/events?after_sequence=0&limit=100` | 增量事件游标与单页条数 | 获取持久化阶段事件 |
| Go 分析 | `GET /analysis-api/api/v1/analysis/runs/{run_id}` | Run ID 作为路径段 | 读取单个 RunResult；运行未结束时现有接口可能返回 409 `run_not_finished` |
| Go 分析 | `GET /analysis-api/api/v1/analysis/runs/{run_id}` | run ID 作为路径段 | 获取结构化分析结果 |

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
docker build --tag stock_analysis_web:local .
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
| 反向代理与部署 | Nginx 配置、同源路径、容器镜像构建已由源码/CI 覆盖 | 在真实 Docker 网络确认 market-data:8091 与 stock-analysis-go:8092 上游 DNS/端口、外部 OpenResty 路径重写、容器健康状态、真实 API Key 配置及请求 ID 诊断 |

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

`MARKET_DATA_API_KEY` 是部署管理员自行生成的内部服务访问密钥，不是行情 Provider 的个人 API Key，也不是 WebUI 自动签发的令牌。建议在部署服务器上通过 `openssl rand -hex 32` 生成，并把同一值配置给 `market-data` 和 `stock-analysis-go` 的 `MARKET_DATA_API_KEY` 环境变量。WebUI 的设置项只将该值保存在浏览器 `localStorage`，调用受保护的行情路由时发送 `X-API-Key`；它不会更新后端配置。该共享 Key 不等于用户登录鉴权，公网访问仍必须额外配置访问控制。


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
