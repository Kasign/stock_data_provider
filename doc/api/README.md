# 数据侧文档索引

本目录维护 Data/Market Data 的领域规划、API 契约、质量规则、真实数据验收和工程迁移记录。

## 唯一任务入口

- [模块总任务规划和进度](./模块总任务规划和进度.md)
- [当前任务状态和进度](./当前任务状态和进度.md)
- [任务历史](./任务历史/各个子任务完成状态.md)

## 专项文档

- [数据侧 API 与契约](./contracts/)
- [架构与运行说明](./architecture/)
- [字段质量与诊断](./quality/)
- [真实数据验收](./acceptance/)
- [Market Data 模块化单体迁移](./migration/MarketData模块化单体迁移计划与进度.md)
- [Provider 迁移与 Python/Go 对照](./provider-migration/)

## 维护规则

1. 总体任务和当前状态分别由两个任务文档维护。
2. README 只做索引；旧数据侧整体规划已归档为任务历史。
3. Provider 数据质量验收与 Market Data 工程迁移分开记录。
4. 契约、OpenAPI、质量规则、样例和真实验收证据继续保留原目录，不因三层结构迁移而删除。
