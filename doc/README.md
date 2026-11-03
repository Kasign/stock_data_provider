# 项目文档索引

本目录按职责组织文档。WebUI 专属计划、功能说明和验收记录统一放在 [web/](./web/README.md)；数据侧文档及跨模块公共契约保留在本目录和既有子目录，不随 WebUI 文档迁移。

## WebUI 文档

- [WebUI 文档索引](./web/README.md)

## 数据层与公共文档

1. [数据层 API 接口清单](./数据层API清单.md)：根据 OpenAPI、HTTP 路由和 WebUI 客户端核对接口映射。
2. [数据质量与状态契约](./数据质量与状态契约.md)：时间、单位、质量、空数据和错误处理约定。
3. [行情数据服务说明](./行情数据服务说明.md)：服务职责、Provider 能力和运行诊断边界。
4. [开发部署说明](./开发部署说明.md)：仓库模块、连接路径、安全与运行边界。
5. [样例数据管理说明](./样例数据管理说明.md)：黄金样例数据脱敏、目录约定和基准维护规则。
6. [国内 A 股数据层阶段五统一验收计划](./国内A股数据层阶段五统一验收计划.md)：当前数据层剩余任务、优先级和统一验收入口。
7. [字段校验与诊断体系实施方案](./字段校验与诊断体系实施方案.md)：Validation、Reporter、Adapter、Probe 架构及实施记录。
8. [字段校验规则矩阵](./字段校验规则矩阵.md)：Quote、DailyBar 等字段规则、Provider 覆盖和代码核对状态。
9. [Freshness SLA 与评估契约](./Freshness%20SLA与评估契约.md)：数据域时间新鲜度策略和证据要求。
10. [国内 A 股数据层黄金样例对账记录](./国内A股数据层黄金样例对账记录.md)：真实样本、差异和回归证据。
11. [阶段二三统一验收清单](./阶段二三统一验收清单.md)：阶段二、三验收记录。
12. [接口规范](./接口规范/)：数据源扩展、真实环境验收、Go 迁移和 WebUI/API 公共契约说明。
13. [API OpenAPI 契约](./api/)：分析任务与 Python 桥接机器可读契约。

## 权威代码与契约

- 行情 API 机器契约：services/market-data/openapi.yaml。
- 行情 HTTP 路由：services/market-data/internal/httpapi/server.go。
- WebUI 请求封装：web/src/api.js。
- 页面结构：web/index.html。
- WebUI 样式：web/styles.css。

## 文档维护原则

页面功能和前端行为以 WebUI 文档及当前代码为准；服务端接口以 OpenAPI、Handler、服务层实现和测试为准。接口已注册或前端已封装，不代表 Provider 已启用或真实数据已验收。历史进度以对应分支的 Git 提交和验收记录为准。
