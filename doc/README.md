# 项目文档索引

本目录是项目当前有效的中文文档入口。历史交接记录、重复的实施日志和过期路线图不作为当前开发依据。

## 项目与 WebUI

1. [WebUI功能说明](./WebUI功能说明.md)：页面结构、模块能力、设置中心和本地数据边界。
2. [数据层API清单](./数据层API清单.md)：当前分支 OpenAPI、HTTP 路由和 WebUI API 接入映射。
3. [WebUI提测验收清单](./WebUI提测验收清单.md)：WebUI 功能批次和浏览器验收标准。
4. [开发部署说明](./开发部署说明.md)：仓库模块、连接路径、安全与运行边界。
5. [行情数据服务说明](./行情数据服务说明.md)：服务职责、Provider 能力和运行诊断边界。

## 数据层迁移与质量

1. [Go 与 Python 数据质量对比报告](./接口规范/GO_PYTHON_DATA_QUALITY_PARITY_REPORT.md)：质量契约保护声明、差异与验收方法。
2. [Python Provider → Go 迁移矩阵](./接口规范/PYTHON_PROVIDER_GO_MIGRATION_MATRIX.md)：Provider 代码能力、默认开关、Python bridge 调用和迁移缺口。
3. [Go 数据服务迁移状态](./接口规范/GO_DATA_PROVIDER_SWITCH_STATUS.md)：当前完成情况和未完成事项。
4. [国内 A 股数据源扩展计划](./接口规范/DATA_SOURCE_EXPANSION_PLAN.md)：下一阶段数据源验证和新增能力顺序。
5. [数据源真实环境验收记录](./接口规范/DATA_SOURCE_RUNTIME_ACCEPTANCE_LOG.md)：真实请求验收证据及记录模板。
6. [数据质量与状态契约](./数据质量与状态契约.md)：时间、单位、质量、空数据和错误处理约定。
7. [黄金样例数据说明](./黄金样例数据说明.md)：脱敏基准样例的组织方式与维护要求。

## 机器可读接口规范

- 行情 API：`services/market-data/openapi.yaml`。
- 分析任务 API：`接口规范/分析任务.openapi.yaml`。
- Python 桥接 API：`接口规范/Python桥接.openapi.yaml`。

## 权威代码位置

- 行情 HTTP 路由：`services/market-data/internal/httpapi/server.go`。
- Provider 接口：`services/market-data/internal/provider/provider.go`。
- Provider 注册和启动装配：`services/market-data/cmd/market-data/main.go`。
- Provider 配置：`services/market-data/configs/providers.json`。
- WebUI 请求封装：`web/src/api.js`。

## 文档维护原则

页面功能和 API 状态以当前分支代码为准；OpenAPI 是对应服务的机器可读接口契约。静态代码实现、默认配置启用和真实运行验收必须分别标注。历史进度只保留在 Git 提交历史和有证据的验收记录中。
