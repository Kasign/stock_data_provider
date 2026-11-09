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
- 当时 Eastmoney 暂保留全部 origin 校验；该临时策略已由文档后续“Eastmoney 出站依赖按业务能力收敛”阶段替代，当前以 `HTTPOriginsForCapabilities` 的调用链映射为准。
- 本批未执行构建或测试，按约定留待 integration 统一验收；本次仅完成静态请求路径核对和回归测试源码，不代表真实 BoundDoer 安全策略已接通。


### 2026-11-09：GoRedis typed nil 防御

- 实现提交：`95654831e9ae7a71cad604cdbfe8bad64270e472`。
- `NewGoRedis` 原先只检查 interface 是否为 nil；传入 `(*redis.Client)(nil)` 等 typed nil 指针时，interface 本身非 nil，会绕过构造校验并在首次调用时触发空指针风险。
- 构造入口现在同时检查 nil interface 与 nil 底层指针/接口/映射等 nil-able 动态值；增加 typed nil 回归测试。未改变 Adapter 的资源所有权，仍由 Root 持有并关闭 Redis Client。
- 本批未运行测试，待 integration 统一验收。


### 2026-11-09：TCP 严格校验测试覆盖修正

- 测试提交：`a4886d8d8f8bc0ce0d6a00e6f1bf6be63ad2f853`。
- 原 TCP 依赖测试同时配置 BaoStock 与 Pytdx，但断言在 BaoStock 首个失败后即结束，未真正覆盖 Pytdx 缺失 DialContext 的分支。
- 改为两个独立子测试，分别验证 BaoStock 与 Pytdx 缺少 TCP 注入时均 fail-closed；两者均显式开启 DailyBars，避免零能力跳过。
- 未执行测试，待 integration 统一验收。


### 2026-11-09：Eastmoney 出站依赖按业务能力收敛

- 实现提交：`34e747ed029b800c60207917f3985adbd5ed9a2a`；校验回归测试提交：`3faa442968fb92b9123970d37ddf454d31711a2f`。
- 新增 `EastmoneyCapabilities` 与 `HTTPOriginsForCapabilities`，由 Provider 根据真实调用链计算当前启用能力所需 origin；bootstrap 不再无条件要求全部配置端点。
- 已纳入跨能力附带请求：
  - MarketBreadth：市场分布、证券列表分页、涨跌停池。
  - HotRankings：人气榜/飙升榜及指数行情补全。
  - SectorRankings：行业榜及五日榜使用的 K 线。
  - ChipDistribution：用于筹码计算的日 K。
- 其他能力按其直接请求端点校验：Quote、DailyBars、StockBasic、StockSearch、IndexQuotes、LimitPools、StockBoards、Announcements、DragonTiger、CapitalFlow。
- 增加 Provider 级依赖映射测试，以及 bootstrap 层“仅 Quote 启用时只要求 Quote origin”的回归测试。
- 仍需在 integration 统一执行测试；本分支未运行构建或测试。该改动只收敛装配时依赖清单，不改变 BoundDoer 的 DNS/IP/重定向安全策略。


### 后续阻塞项：TCP 受控拨号能力

- 当前 Data Provider 已通过 `Dependencies.BaoStockDialContext` 与 `Dependencies.PytdxDialContext` 接收 Root 注入的 TCP 拨号函数；严格装配校验会在启用对应 Provider 能力但未注入时拒绝启动。
- 当前合入本分支的 `internal/platform/httpclient` 只有 HTTP BoundDoer，未发现可供 Root 使用的受控 TCP BoundDialer。
- BaoStock 与 Pytdx 在 nil DialContext 时仍保留独立兼容模式的标准 `net.Dialer` fallback；模块化 Root 必须由严格装配校验阻止此路径，不能将 fallback 视为安全出站。
- 该缺口属于 Platform 基础能力，不在 Data 分支重复实现。需要 Platform 提供可复用的 TCP 受控拨号能力后，Root 才能完成注入；在此之前启用 BaoStock/Pytdx 的模块化 Root 配置应保持 fail-closed。


### 2026-11-09：补齐严格出站校验回归覆盖

- 测试提交：`c5a2beaa3d6f178ba2a0cc8939e9a277a3ed1d9d`。
- 增加 Hithink、Yahoo、Xueqiu 单 HTTP 出口缺失时的 fail-closed 测试。
- 增加 Tencent DailyBars 必须同时具备注入复权与未复权 K 线执行器的测试；只注入其中一个时必须失败，两个都注入时通过静态装配校验。
- 现有测试仍未执行，统一留待 integration。

### 2026-11-09：禁用 TickFlow 的残留能力不再触发凭证校验

- 配置审查发现 TickFlow API Key 校验只检查能力开关，没有同时检查 Provider 的 Enabled 状态；禁用 TickFlow 但保留历史能力配置时，仍会错误阻断启动。
- 凭证校验现在与 Hithink/Tushare 一致，仅在 TickFlow 已启用且启用了认证 API 能力时要求 API Key；日 K 的公开 API 模式仍不要求 Key。
- 增加回归测试，保留禁用状态下的历史 IndexQuotes/MarketBreadth 开关并清空 TickFlow Key，验证配置仍可加载。
- 未执行构建或测试，按约定留待 integration 统一验收。

### 2026-11-09：严格出站装配拒绝 typed nil 执行器

- 复核发现 HTTP Doer 以接口类型注入时，typed nil 指针转换成接口后不等于 nil，原严格出站校验可能误判为已注入。
- 增加统一 nil-able 动态值检测，覆盖各 Provider HTTP Doer、Eastmoney origin map，以及 TCP DialContext；普通非 nil 实现保持兼容。
- 增加 Tushare 与 Eastmoney 两类 typed nil 回归测试，验证其按缺失出站依赖处理。
- 未执行构建或测试，留待 integration 统一验收。
### 2026-11-09：Provider 能力配置与出站装配一致性复核

- 对照 ProviderSettings、hasConfiguredProviderCapability 和 Module.build 的能力注册分支，确认当前 20 项配置能力均纳入“是否存在业务能力”判断，并在模块装配时执行对应接口断言；启用但未实现的能力会明确返回错误，不会静默注册。
- 对照各 Provider 的出站装配，确认 Tencent 双 K 线出口、Sina/NetEase 分能力出口、TickFlow 认证/公开请求模式和 Eastmoney 能力级 origin 映射均有对应严格校验。
- 发现并修复 TickFlow 禁用状态下残留认证能力仍要求 API Key 的配置问题。
- 严格出站依赖检查现已识别 typed nil，避免接口非 nil 表象绕过装配门禁。
- Dependencies.RequireEgressDependencies 是显式布尔门禁，默认值为 false，以兼容独立部署旧入口；模块化 Root 必须显式设置为 true。Data 分支不代替 Server 修改组合根，Root 实际调用点由 Server 分支验收。
- BaoStock/Pytdx 的 TCP 受控拨号仍等待 Platform 提供 BoundDialer；当前 Data 保持严格模式 fail-closed，不在本分支实现第二套安全拨号器。
- 本轮未执行构建、测试或真实 Provider 请求；新增测试源码留待 integration 统一 CI，真实出站验收仍需运行环境证据。

### 2026-11-09：Module Cache 注入拒绝 typed nil

- 继续复核基础设施依赖时发现 Dependencies.Cache 是接口类型，typed nil 可绕过 Redis 必需依赖检查，也会阻止未配置 Redis 时回退到 Disabled Cache。
- Module.New 现在使用统一 nil 检测：Redis 已配置时拒绝 typed nil Cache；Redis 未配置时将 typed nil Cache 视为未注入并使用 Disabled Cache。
- 新增两种启动路径回归测试；未执行构建或测试，留待 integration 统一验收。
