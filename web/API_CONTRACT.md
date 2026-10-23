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
| Go 行情 | `GET /market-api/api/v1/bars/{symbol}/daily?limit=120&adjustment=forward_adjusted` | `X-API-Key`；`limit`、`adjustment` 可配置 | 查询历史日 K |
| Go 分析 | `GET /analysis-api/readyz` | 不附加行情 API Key | 分析服务就绪检查 |
| Go 分析 | `POST /analysis-api/api/v1/analysis/tasks` | JSON；可选 `Idempotency-Key` | 创建异步分析任务 |
| Go 分析 | `GET /analysis-api/api/v1/analysis/tasks/{task_id}` | task ID 作为路径段 | 查询任务状态 |
| Go 分析 | `GET /analysis-api/api/v1/analysis/tasks/{task_id}/events?after_sequence=0&limit=100` | 增量事件游标与单页条数 | 获取持久化阶段事件 |
| Go 分析 | `GET /analysis-api/api/v1/analysis/runs/{run_id}` | run ID 作为路径段 | 获取结构化分析结果 |

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
