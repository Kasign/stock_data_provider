# 项目文档索引

本目录是项目当前有效的中文文档入口。数据侧文档统一归档在 [api/](api/)；数据侧整体开发任务以 [数据侧整体规划](api/数据侧整体规划.md) 为唯一主计划。公共平台专项文档统一归档在 [platform/](platform/)，公共平台开发任务以该目录的路线图为唯一主计划。

## 公共文档

1. WebUI功能说明.md：页面结构、模块能力、设置中心和本地数据边界。
2. WebUI提测验收清单.md：WebUI 功能批次与统一浏览器验收标准。
3. 开发部署说明.md：仓库模块、连接路径、安全与运行边界。
4. 接口规范/WebUI与数据层接口协作清单.md：WebUI 与数据层之间的跨端接口对账、缺口登记、责任划分和联调关闭证据。

## 跨模块设计与治理文档\n\n1. [整体任务规划/五模块跨模块实现前置契约复核与合并清单.md](整体任务规划/五模块跨模块实现前置契约复核与合并清单.md)：五模块已决议语义、待补实现约束、开发顺序与跨模块验收切片。\n2. [整体任务规划/跨模块契约目录与冻结状态.md](整体任务规划/跨模块契约目录与冻结状态.md)：跨模块契约编号、唯一权威位置、状态和阻断范围。\n\n## 公共平台文档（统一位于 platform/）

1. platform/README.md：公共平台专项文档入口和权威范围。
2. platform/00-平台架构与边界.md：平台目标架构、职责边界和关键决策。
3. platform/01-开发路线图与任务清单.md：PLAT-001~014 唯一任务状态清单。
4. platform/02-代码结构与模块详细设计.md：当前代码盘点、目标目录和模块依赖。
5. platform/03-安全认证与ServiceKey设计.md：身份模型、Service Key 和安全中间件。
6. platform/04-配置日志诊断与可观测性设计.md：Config、Logger、Error、Trace、Diagnostics 和 Metrics。
7. platform/05-HTTP存储与异步任务设计.md：HTTP Client、MariaDB、Redis、Migration 和 Task。
8. platform/06-分支协作与集成验收规范.md：分支职责、契约冻结和集成门禁。

## 数据侧文档（统一位于 api/）

1. api/数据侧整体规划.md：数据侧唯一整体开发任务入口，定义阶段顺序、优先级、状态和执行约束。
2. api/数据层API清单.md：数据服务 API 与 WebUI 接入映射。
3. api/数据质量与状态契约.md：时间、单位、质量、空数据和错误处理约定。
4. api/行情数据服务说明.md：服务职责、Provider 能力和运行诊断边界。
5. api/样例数据管理说明.md：黄金样例数据的脱敏要求、目录约定和基准维护规则。
6. api/国内A股数据层阶段五统一验收计划.md：阶段五诊断细节和专项验收证据。
7. api/国内A股数据层黄金样例对账记录.md：真实样本、差异和回归证据。
8. api/字段校验与诊断体系实施方案.md：Validation、Reporter、Adapter、Probe 架构及实施记录。
9. api/字段校验规则矩阵.md：Quote、DailyBar 等字段规则、Provider 覆盖和代码核对状态。
10. api/Freshness SLA与评估契约.md：各数据域时间新鲜度策略和证据要求。
11. api/阶段二三统一验收清单.md：数据服务阶段二、三专项验收记录。
12. api/接口规范/：数据源扩展、真实环境验收、Go 迁移和 Python/Go 差分专项资料。
13. api/分析任务.openapi.yaml、api/Python桥接.openapi.yaml：机器可读接口契约。

## 权威代码与契约

- 行情 API 机器契约：services/market-data/openapi.yaml。
- 行情 HTTP 路由：services/market-data/internal/httpapi/server.go。
- WebUI 请求封装：web/src/api.js。
- 页面结构：web/index.html。
- WebUI 样式：web/styles.css。

## 文档维护原则

数据侧整体任务顺序、优先级和状态只在 api/数据侧整体规划.md 维护；专项文档仅记录其领域内方案、契约、规则和验收证据。WebUI 页面事实以 WebUI 分支和当前合并代码为准；OpenAPI 描述以对应服务的权威文件为准。