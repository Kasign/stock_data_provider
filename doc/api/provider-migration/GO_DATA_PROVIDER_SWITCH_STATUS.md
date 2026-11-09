# Go 数据服务迁移状态

> 本文记录阶段一至阶段三的历史迁移盘点及后续迁移证据，不作为整体任务入口。当前整体开发顺序、优先级和执行队列统一以 [数据侧整体规划](../数据侧整体规划.md) 为准。本文保留 Go 模块路径迁移、Provider 能力和迁移遗留项的专项事实；不得从历史待办表独立启动整体任务。

- 适用分支：`feat/full-go-data-providers-api-switch`
- 静态盘点基线：`4c603172448102058f95e6bc625494436a8402c9`
- 更新日期：2026-10-30
- 状态口径：只记录当前代码/配置可证明的状态，不把静态实现当作真实生产可用。


### 2026-11-09：M4 严格出站校验测试前置条件修复与 TickFlow 出口按请求模式收敛

- 代码基线：`c7c5946c266a9170506d5fa2e596934f3292d19c`；实现提交：`037eee6b08094a177e575ea3c6d556f29c338671`；TickFlow 回归测试提交：`9c407c96b3e8766586dc1fda4ce6ba17d9e49dd3`。
- 修复既有严格校验测试的配置前置条件：Tushare、Eastmoney、BaoStock、Pytdx 测试显式启用至少一项业务能力，避免被“零能力 Provider 跳过”规则提前短路，造成测试误通过。
- 根据 TickFlow 实际调用点确认：Quote、IndexQuotes、MarketBreadth、SectorRankings 使用认证 API；DailyBars 在配置 API Key 时使用认证 API，否则使用 Free API。严格校验现在只要求对应请求模式的执行器，不再无条件要求认证与公开两个执行器同时存在。
- 新增 TickFlow 组合测试源码覆盖 Quote-only、无 Key 日 K、带 Key 日 K，以及缺失对应执行器的失败场景。
- Eastmoney 暂保留 Provider 实际配置的全部 origin 校验。原因是当前 endpoint 被跨能力复用（例如 Quote enrichment、MarketBreadth 的 LimitPool 与 StockList 分页）；尚未完成所有调用点到领域能力的可靠映射，不能仅按 endpoint 字段名称推断最小依赖集。
- 本批未执行构建或测试，按约定留待 integration 统一验收；本次仅完成静态请求路径核对和回归测试源码，不代表真实 BoundDoer 安全策略已接通。


### 2026-11-09：GoRedis typed nil 防御

- 实现提交：`95654831e9ae7a71cad604cdbfe8bad64270e472`。
- `NewGoRedis` 原先只检查 interface 是否为 nil；传入 `(*redis.Client)(nil)` 等 typed nil 指针时，interface 本身非 nil，会绕过构造校验并在首次调用时触发空指针风险。
- 构造入口现在同时检查 nil interface 与 nil 底层指针/接口/映射等 nil-able 动态值；增加 typed nil 回归测试。未改变 Adapter 的资源所有权，仍由 Root 持有并关闭 Redis Client。
- 本批未运行测试，待 integration 统一验收。
