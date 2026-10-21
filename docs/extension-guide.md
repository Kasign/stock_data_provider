# 模块扩展与组装约定

## 一、模块在启动期注册，不在运行期热插拔

Runner 只依赖 `app.Dependencies` 中的稳定接口。`cmd/analysis-worker` 作为 composition root：加载配置、构建各注册表、注册模块、校验版本与依赖、冻结注册表、组装引擎，最后构建 Runner。Runner 运行后不允许模块注册表改变。

典型装配顺序：

```go
factorRegistry := registry.New[modules.FactorCalculator]()
// factorRegistry.Register(实现 modules.FactorCalculator 的具体因子模块)
factorRegistry.Freeze()

strategyRegistry := registry.New[modules.Strategy]()
// strategyRegistry.Register(实现 modules.Strategy 的具体策略模块)
strategyRegistry.Freeze()

riskRegistry := registry.New[modules.RiskRule]()
// riskRegistry.Register(实现 modules.RiskRule 的具体风险规则)
riskRegistry.Freeze()

factorEngine, err := modules.NewFactorEngine(factorRegistry)
strategyEngine, err := modules.NewStrategyEngine(strategyRegistry)
riskEngine, err := modules.NewRiskEngine(riskRegistry)
requirementCatalog, err := modules.NewRegistryRequirementCatalog(
    strategyRegistry,
    factorRegistry,
    coreRequirements,
)
```

上面是组装顺序示意，具体模块、日志、配置加载和错误处理由实际应用实现；本仓库骨架不提供不存在的策略实现。

## 二、因子依赖自动聚合

每个 `modules.Strategy` 通过 `FactorIDs()` 声明依赖哪些因子，并通过 `Requirements()` 声明策略自身必需的数据。`RegistryRequirementCatalog` 会：

1. 验证请求中的策略 ID 已注册；
2. 聚合被选策略的数据需求；
3. 对所需因子去重并排序；
4. 检查每个因子已注册，并读取因子数据需求；
5. 将 `FactorIDs`、`ByStrategy`、`ByFactor` 和核心需求返回给数据采集/质量门禁。

因子引擎仅计算 `RequirementSet.FactorIDs` 中的因子，不会因为注册了新因子就无条件对全市场运行所有计算器。加入因子后，需要在使用它的策略 `FactorIDs()` 中声明依赖；若只想做研究输出，也应通过明确配置将其列为本次运行的因子需求。

## 三、风控规则合并约定

每条 `RiskRule` 对输入的每个候选信号都必须返回一条判定。不得用“没有返回拒绝”表达允许。返回缺项、重复信号、未知信号，或拒绝时没有理由，整个风控阶段报错并使本次运行失败，而不是默认放行。

多个风险规则使用 AND 语义：任何规则拒绝后，后续规则不能重新批准。主流程还会执行第二道校验，要求信号同时具有可交易性放行和风险放行记录，否则不会传给仓位规划器。

## 四、适配器职责

第一阶段接入现有 `daily_stock_analysis` Python 服务的实现位于 `internal/adapters/pythonapi`，并在根目录 `.env` 使用 `PYTHON_API_BASE_URL` 配置服务地址。它提供 `Client`（quote/history/profile/data overview）和实现 `app.DataCollector` 的 `Collector`，以及 `cmd/pythonapi-smoke` 真实 HTTP 冒烟入口。详细地址、认证、超时和数据缺失语义见 [`Go 接入 Python API 文档`](Go-Python-API-adapter-中文.md)。该适配器只是 transport/data boundary，并未实现生产 `Normalizer`、完整 `QualityAssessor` 或真实策略。

- `DataSource` 只封装一个源的请求协议、解析和来源元数据，不决定全局数据源优先级。
- `DataCollector` / source router 负责数据集和字段能力匹配、超时、限流、有限重试、降级、字段冲突处理、`available_at` 检查，以及写入来源留痕。
- `Normalizer` 负责统一代码/单位/交易日，处理复权和公司行动，并保证原始值与调整值口径不混用。
- `QualityAssessor` 把“核心数据失败”与“某个策略必需数据缺失”分开判定。对请求中的每个策略都必须写入 `StrategyEligible`；没有判定时主流程按不合格处理。
- `PortfolioProvider` 必须读取 `request.AsOf` 时点可复现的现金、持仓、可卖数量、行业暴露和未完成订单；不得用执行时的最新持仓替代历史分析时点的组合状态。
- `PositionPlanner` 只能为主流程传入的获准信号创建计划，并执行组合仓位、行业集中度、风险预算、现金和可卖数量约束。它必须对每个获准信号返回一个计划或带原因的拒绝。
- `ExecutionAssessor` 针对当前可用快照评估计划，并对每个计划返回显式状态。由于新计划的 `EarliestExecuteAt` 必须晚于 `request.AsOf`，初次评估通常应返回 `pending`；不能用未来数据假装已经成交。
- 后续模拟成交由独立 Paper Execution Worker 负责：读取未过期计划、加载其后可用的市场快照、按版本化的交易制度/费用/滑点/成交量参与率模型模拟成交，再更新模拟组合账本与可卖数量。这个 worker 尚未包含在当前骨架里。
- 全历史回测另由独立的 `BacktestService` 推进时间并写交易事件账本；不要把未来行情注入每日 Runner 的单次分析输入。

## 五、什么时候允许改 Runner

只在流程顺序、业务状态语义或必需审计阶段发生实质变化时才改 Runner。新增数据源、因子、策略、风险规则、交易规则版本、费用模型、报告格式、告警或回测服务都不应修改 `Runner.Run`。若新需求只是为某个模块提供更多信息，优先修改对应的强类型输入契约，不要在 Runner 中增加来源/策略名称的条件分支。
