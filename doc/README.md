# 项目文档索引

本目录是项目当前有效的中文文档入口。历史交接记录、重复的实施日志、过期路线图和重复 API 清单已从原目录清理。

## 当前有效文档

1. WebUI功能说明.md：页面结构、模块能力、设置中心和本地数据边界。
2. 数据层API清单.md：根据当前分支 OpenAPI、HTTP 路由和 WebUI API 客户端核对的接口表。
3. WebUI提测验收清单.md：本轮功能批次与下一次统一浏览器验收标准。
4. 数据质量与状态契约.md：时间、单位、质量、空数据和错误处理约定。
5. 行情数据服务说明.md：服务职责、Provider 能力和运行诊断边界。
6. 开发部署说明.md：仓库模块、连接路径、安全与运行边界。
7. 样例数据管理说明.md：黄金样例数据的脱敏要求、目录约定和基准维护规则。
7. 接口规范/：分析任务和 Python 桥接的机器可读 OpenAPI。
8. 黄金样例数据说明.md：脱敏基准样例的组织方式与维护要求。

## 权威代码与契约

- 行情 API 机器契约：services/market-data/openapi.yaml。
- 行情 HTTP 路由：services/market-data/internal/httpapi/server.go。
- WebUI 请求封装：web/src/api.js。
- 页面结构：web/index.html。
- WebUI 样式：web/styles.css。

## 文档维护原则

页面功能和 API 状态以当前分支代码为准；OpenAPI 描述以对应服务的权威文件为准。历史进度只保留在 Git 提交历史，不再通过长期累积的 HANDOFF/PROGRESS/REVIEW 日志维护。
