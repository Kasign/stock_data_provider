# 项目文档索引

本目录是项目当前有效的中文文档入口。数据侧整体开发任务统一以 [数据侧整体规划](api/数据侧整体规划.md) 为唯一主计划；其他文档只承担专项方案、接口契约、规则矩阵和验收证据，不再独立维护整体任务优先级。

## 当前有效文档

1. api/数据侧整体规划.md：数据侧唯一整体开发任务入口，定义阶段顺序、优先级、状态和执行约束。
2. WebUI功能说明.md：页面结构、模块能力、设置中心和本地数据边界。
3. 数据层API清单.md：根据当前分支 OpenAPI、HTTP 路由和 WebUI API 客户端核对的接口表。
4. WebUI提测验收清单.md：WebUI 功能批次与统一浏览器验收标准。
5. 数据质量与状态契约.md：时间、单位、质量、空数据和错误处理约定。
6. 行情数据服务说明.md：服务职责、Provider 能力和运行诊断边界。
7. 开发部署说明.md：仓库模块、连接路径、安全与运行边界。
8. 样例数据管理说明.md：黄金样例数据的脱敏要求、目录约定和基准维护规则。
9. api/：分析任务和 Python 桥接的机器可读 OpenAPI 契约。
10. 国内A股数据层阶段五统一验收计划.md：阶段五诊断缺口和验收专项细节；整体顺序以数据侧整体规划为准。
11. 字段校验与诊断体系实施方案.md：已建设的 Validation、Reporter、Adapter、Probe 架构及实施记录。
12. 字段校验规则矩阵.md：Quote、DailyBar 等字段规则、Provider 覆盖和代码核对状态。
13. Freshness SLA与评估契约.md：各数据域时间新鲜度策略和证据要求。
14. 国内A股数据层黄金样例对账记录.md：真实样本、差异和回归证据。
15. 数据源真实环境验收记录（接口规范/DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md）：真实 Provider 请求与权限验收证据。
16. Go 数据服务迁移状态（接口规范/GO_DATA_PROVIDER_SWITCH_STATUS.md）：Go Provider 迁移与遗留能力历史记录。

## 权威代码与契约

- 行情 API 机器契约：services/market-data/openapi.yaml。
- 行情 HTTP 路由：services/market-data/internal/httpapi/server.go。
- WebUI 请求封装：web/src/api.js。
- 页面结构：web/index.html。
- WebUI 样式：web/styles.css。

## 文档维护原则

页面功能和 API 状态以当前分支代码为准；OpenAPI 描述以对应服务的权威文件为准。历史进度只保留在 Git 提交历史，不再通过长期累积的 HANDOFF/PROGRESS/REVIEW 日志维护。
