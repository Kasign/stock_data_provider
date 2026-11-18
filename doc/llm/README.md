# LLM 模块文档索引

维护分支：`feat/llm`  
代码目录：`internal/llm/**`

## 唯一任务入口

- [模块总任务规划和进度](./模块总任务规划和进度.md)
- [当前任务状态和进度](./当前任务状态和进度.md)
- [任务历史](./任务历史/各个子任务完成状态.md)

## 设计与契约

- [模型适配与工具边界设计](./01-模型适配与工具边界设计.md)
- [现有模型 Gateway 协议盘点](./03-现有模型Gateway协议盘点.md)
- [ProviderPort 与 DTO 字段级契约](./04-ProviderPort与DTO字段级契约.md)
- [JSON 与 SSE 契约 Fixture 清单](./05-JSON与SSE契约Fixture清单.md)
- [Provider 与 Responses 协议实现边界补充](./06-Provider与Responses协议实现边界补充.md)
- `schema/`：机器契约与结构化输出 Schema。

## 维护规则

1. 总体任务与当前执行状态分别由两个任务文档维护。
2. README 只做索引，历史实施记录统一归入任务历史。
3. 真实 Gateway 能力必须有版本和可追溯运行证据；未知能力保持 unknown。
4. HTTP Client、SSRF、Secret 解析和生产组合根归 Platform/Server，不在 LLM 重复实现。
