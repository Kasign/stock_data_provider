# WebUI 真实 API 与配置边界

更新时间：2026-11-12  
核对基线：feat/webui-market-data-console @ 7f1b8510d349b9d166b6efceddee874ddb7703c0

> 本文只记录当前请求封装、仓库可核对的路由事实和接入门禁。必须区分 OpenAPI 声明、Handler 注册、Root Server 挂载、部署可达和真实数据验收。

## 1. Web API 原则

1. 浏览器只访问 Root Server 对外 HTTP API，不访问模块内部 Repository、ApplicationPort 或 Provider。
2. 不根据模块名称自行设计 URL、字段、枚举、游标和错误语义。
3. 前端封装不代表服务端注册；Handler 注册不代表 Root Server 挂载；路由可达不代表 Provider 有权限或返回真实数据。
4. Mock/Fixture 只证明前端行为，不证明真实服务端联调。
5. 新 API 必须明确用户场景、调用时机、字段来源、权限、分页/超时/取消语义和验收证据。

## 2. 当前服务配置与认证边界

| 配置 | 当前行为 | 迁移目标 | 限制 |
|---|---|---|---|
| 行情 API 基础路径 | 浏览器配置代理路径 | Root Server Market Data Adapter | 以 Server 路由和部署验证为准 |
| 行情 API Key | 迁移期由浏览器配置并用于行情请求 | Server 注入 Data 服务凭据 | 当前兼容实现不是目标安全模型 |
| 分析 API 基础路径 | 使用分析兼容代理路径 | Root Server 注册 Researchflow 路由 | 需统一 Server 入口 |
| 分析认证 | 当前分析路由使用 Owner Bearer；Web 不持有该凭据 | Web Session/Cookie + CSRF + 资源授权 | 不把 Owner Token 放入 localStorage，不匿名开放受保护接口 |

行情健康检查使用 /health/live、/health/ready；分析服务使用 /healthz、/readyz。两组路径属于不同服务契约。

## 3. 当前 Web 已封装的行情 API

以下路径来自当前 API 封装及行情 Adapter 路由核对；仍需 Root Server 装配、部署可达和真实 Provider 验收，才能视为目标环境可用。

| 能力 | 方法与路径 | 用途 |
|---|---|---|
| Provider 清单/运行诊断 | GET /api/v1/data-sources、GET /api/v1/data-sources/diagnostics | 能力声明与只读进程内请求统计 |
| 主动探测/历史 | POST /api/v1/data-sources/probe、GET /api/v1/data-sources/probe/history | 用户显式触发探测、查询历史 |
| 批量报价 | GET /api/v1/market/quotes | 查询明确传入的证券 |
| 指数 | GET /api/v1/market/indices、index-catalog、indices/{symbol}/constituents、indices/{symbol}/daily | 指数快照、目录、成分和日线 |
| 市场行情/活动 | GET /api/v1/market/breadth、sectors、stocks、sentiment、sentiment/history、hot-stocks、limit-pools、dragon-tiger、trading-calendar、session | 市场总览、全市场分页和市场活动 |
| 个股行情/日线 | GET /api/v1/quotes/{symbol}、bars/{symbol}/daily、bundles/{symbol} | 单股报价、日线和组合行情 |
| 个股资料 | GET /api/v1/stocks/{symbol}/profile、fundamentals、announcements、boards | 个股研究补充资料 |
| 市场资金流 | GET /api/v1/market/capital-flow | Tushare 市场级资金流历史，依赖 Provider 权限 |
| 个股资金流/筹码 | GET /api/v1/stocks/{symbol}/capital-flow、chip-distribution | 个股级资金流和筹码，不可替代市场级资金流 |

完整参数、响应字段和函数名以当前 web/src/api.js、internal/marketdata/httpapi/server.go 与行情 OpenAPI 为准；本文不复制完整 Schema。

## 4. 当前分析任务 API

当前 Web 使用独立分析服务连接配置：
- POST /api/v1/analysis/tasks：创建任务。
- GET /api/v1/analysis/tasks/{task_id}：读取任务状态。
- GET /api/v1/analysis/tasks/{task_id}/events：读取阶段事件。
- GET /api/v1/analysis/runs/{run_id}：读取 Run 结果。

字段契约以 doc/api/contracts/分析任务.openapi.yaml 为准。最近任务区只保存有限数量的浏览器任务引用，不是服务端完整历史。

当前分析路由 Owner Bearer 与浏览器 Session 尚未形成可验收的认证闭环。服务端历史分页、模板、取消和综合结果接口未完成 Application API、Server 注册、认证和部署验证前，Web 不新增推测请求。

## 5. 跨模块目标能力与接入状态

| 领域 | Web 目标场景 | 前置条件 | 当前处理 |
|---|---|---|---|
| News | 新闻、来源、事件线索 | 公开查询边界、来源授权字段 | 未接入为正式页面数据 |
| Research | Context、Evidence、Event、Claim、Hypothesis、Scenario | Research Application API；News-Research 只读 Evidence Query；来源授权/引用定位 | 个股行情/资料可用；结构化研究明确 not_integrated，本地笔记不是 Evidence |
| Quant | 指标、因子、趋势、策略、回测、风险/评估 | Quant-Research DTO、Application API、PIT/质量/风险语义 | 当前仅本地派生指标和当前页筛选 |
| Researchflow | Run/Stage/Attempt、模板、取消、历史、综合结果 | ApplicationPort/状态机契约、Server 路由、认证和分页 | 继续消费现有 Task/Run，不另建状态机 |
| LLM | 模型选择、结构化解释、受控生成 | LLM Gateway、Server 代理、凭据托管、权限和输出 Schema | 不在浏览器保存模型密钥 |
| Notification | 订阅、渠道、投递历史 | Notification API、权限和投递状态契约 | 仅保留本地通知偏好 |
| Portfolio/Alert/Report | 组合、规则、服务端报告 | 领域归属和持久化边界评审 | 不从本地功能推断服务端模块存在 |

跨模块契约见 doc/整体任务规划/03-模块接口契约/。契约冻结不代表实现、Server 注册、部署或真实数据验收完成。

## 6. 数据、诊断与安全约束

- 质量、覆盖率、Freshness、来源时间和采集时间由服务端给出；前端不补算业务口径。
- skipped、not_configured、not_supported 不得显示为健康/成功。
- Evidence 展示来源、授权范围、事件时间、可用时间、采集时间及引用定位；缺字段显示未知。
- Quant RiskDecision=block 是硬门禁，不能被 LLM 分数或 UI 覆盖。
- Run 最终状态由 Researchflow 决定，LLM 流式输出只是过程。
- 浏览器不存 Owner Token、模型密钥或第三方 Provider Key。
- OpenAPI、Handler、Root Server 路由、部署和真实数据是独立验收项。

## 7. 新增 API 准入模板

新增 Web API 前必须明确：
1. 用户场景和页面入口。
2. 领域 Owner 与 Server 路由 Owner。
3. 方法、路径、请求/响应 Schema、字段来源和单位。
4. 身份认证、资源授权和数据来源授权。
5. 分页、排序、超时、取消、幂等和错误语义。
6. 空数据、跳过、降级、部分成功和不可比较的表达。
7. 单元/契约/浏览器测试及真实部署/真实数据验收证据。

不满足条件时标记“待契约/待服务端接入”，不写成已实现 API。
