# 项目级契约入口

- 当前阶段：`L1_contracts_frozen`
- 当前状态：43 项正式 CTR；37 项 `frozen`、4 项 `deferred`、2 项 `not_applicable`；无 `pending_evidence` 或 `pending_decision`。
- 冻结基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- 登记簿版本：`0.2.59-frozen`。

## 权威来源

| 内容 | 唯一权威来源 |
|---|---|
| 项目级 CTR 编号、当前状态、冻结范围、延期条件 | [PLAT-002 项目级契约冻结登记簿](./PLAT-002-项目级契约冻结登记簿.md) |
| 当前有效的项目级语义决策 | [项目级契约决策记录](./decisions/项目级契约决策记录.md) |
| GOV-CTR-001～004 公共规则 | [rules/](./rules/) |
| PLAT-CTR-001～007 Platform 公共技术契约 | [rules/](./rules/) |
| DATA-CTR-006 Data HTTP API 认证 | [DATA-CTR-006 认证契约](./rules/DATA-CTR-006-Data-HTTP-API调用认证契约.md) |
| GOV-CTR-006 AuthorizationRef/PolicySnapshot | [GOV-CTR-006 权威说明](../整体任务规划/02-契约治理/GOV-CTR-006-AuthorizationRef机器契约与Golden.md) |
| 领域字段级 DTO、Application Port、状态机、Schema、Golden | 对应模块专项文档及机器契约文件 |
| 契约版本和变更规则 | [契约版本与变更规则](./契约版本与变更记录.md) |

## 使用规则

1. 开发前先查登记簿，确认所依赖 CTR 的当前状态。
2. `frozen` 项按其权威定义实现；登记簿中的实现差异描述不构成对冻结语义的修改。
3. `deferred` 项不得被当作已发布契约；只在登记簿规定的重新进入条件满足后启动定点复核。
4. `not_applicable` 项属于模块内部约定，不纳入项目级跨模块契约。
5. 字段级语义以对应生产模块的权威文档/强类型 Port 为准；项目级决策用于消除跨模块冲突。
6. L1 冻结不代表 L2 实现符合性或 L3 真实集成验收通过。Schema/Golden/Validator 同步、消费者测试、CI、Server 装配和部署分别在后续实现阶段验收。

## 当前有效文件

- [项目级契约冻结登记簿](./PLAT-002-项目级契约冻结登记簿.md)
- [当前有效决策](./decisions/项目级契约决策记录.md)
- [公共规则](./rules/)
- [Schema](./schema/)
- [Golden 样例](./golden/)
- [版本与变更规则](./契约版本与变更记录.md)

本目录只维护当前有效契约定义、状态和变更规则，不存放阶段性审查进度、重复任务矩阵或历史状态快照。