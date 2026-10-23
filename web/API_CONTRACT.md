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


## 个股补充研究接口

WebUI 提供一个**手动触发**的补充数据面板，调用现有接口：

- `GET /api/v1/stocks/{symbol}/chip-distribution`：使用 `symbol`、`date` / `provider_date`、`average_cost`、`profit_ratio`、`concentration_90`、`concentration_70`、`source`、`source_version`、`fetched_at`、`quality_status` 和 `quality_reasons` 等服务端字段。
- `GET /api/v1/stocks/{symbol}/capital-flow`：使用 `main_net_inflow_yuan`、`inflow_5d_yuan`、`inflow_10d_yuan`、`observations`、`source`、`source_version`、`fetched_at`、`quality_status` 和 `quality_reasons` 等服务端字段。

筹码比例字段由后端规范化为 `[0,1]` 小数，UI 仅将其乘以 100 后格式化为百分比，不重新计算筹码数据。资金流以人民币元展示，空的 5/10 日汇总仍显示“—”。两个请求并行且独立呈现：其中一个接口失败不会覆盖另一个成功结果，也不会中断行情查询、日 K 或分析流程。面板不会自动请求，避免未启用的补充 Provider 在主流程中引入额外请求或等待。
