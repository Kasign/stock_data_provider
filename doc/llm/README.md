# LLM 模块文档索引

维护分支：`feat/llm`  
代码目录：`internal/llm/**`

## 唯一任务入口

- [模块总任务规划和进度](./模块总任务规划和进度.md)：唯一任务状态权威来源。
- [当前任务状态和进度](./当前任务状态和进度.md)：唯一当前执行任务。
- [任务历史](./任务历史/01-LLM-001至008历史执行记录.md)：完整历史记录，不维护当前状态。

## 架构

- [模型适配与工具边界](./架构/01-模型适配与工具边界.md)
- [Provider Port 与 DTO 字段契约](./架构/02-ProviderPort与DTO字段契约.md)
- [Adapter 与 Platform/Server 接入边界](./架构/03-Adapter与Platform接入边界.md)

## 依赖

- [Platform 公共能力依赖](./依赖/01-Platform公共能力依赖.md)
- [Server 生产装配依赖](./依赖/02-Server生产装配依赖.md)
- [Researchflow 消费者依赖](./依赖/03-Researchflow消费者依赖.md)

## 说明与验证资产

- [现有 Gateway 协议盘点](./说明/01-Gateway协议盘点.md)
- [JSON 与 SSE 契约 Fixture 清单](./说明/02-JSON与SSE契约Fixture清单.md)
- [旧任务拆分文档迁移说明](./说明/03-旧任务拆分文档迁移说明.md)
- `schema/`：版本化机器契约 Schema。该路径由 DTO Schema 测试直接引用，保持稳定。

## 文档维护规则

1. 总规划是任务状态唯一权威来源；当前任务文档只能有一个。
2. 历史文档按任务编号归档，保留完整执行记录。
3. 架构文档描述稳定职责、依赖和接入边界；说明文档保存协议调查、Fixture 和机器契约。
4. 文档只记录已验证事实；Mock、Fixture、静态代码检查不代表真实 Gateway 能力通过。
5. Platform/Server 的公共能力由对应模块维护，LLM 只记录依赖和消费方式，不复制实现。
