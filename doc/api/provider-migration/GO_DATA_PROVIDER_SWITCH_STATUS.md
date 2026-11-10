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

### 2026-11-10：Provider 公共协议与能力注册第一批 Review

- 审查基线：`66e7a67f2283a13104db070b27e88b0caf40f2c2`。
- 审查范围：`provider/provider.go`、核心领域模型、`bootstrap/module.go`、`service/data.go`、Provider 配置及 Probe 能力清单；本批不覆盖各数据商 Adapter 的字段映射验收。
- 结论：按能力拆分的接口结构合理。Quote/DailyBars 分离，`NamedProvider` 作为最小发现接口，旧版组合 `Provider` 保留兼容；DataService 主要依赖能力接口，Bootstrap 对启用但未实现的能力 fail-fast。当前没有增加万能接口或重做领域模型的必要。
- 待收口项一：`StockUniverseProvider` 没有独立配置开关，业务注册复用 `StockSearch` Provider 列表；主动 Probe 则从全部启用 Provider 中发现该接口，业务可用范围与 Probe 发现范围不完全一致。需确认完整证券目录是否继续作为 StockSearch 的子能力；若是，Probe 应与业务注册范围一致；若否，再拆独立配置能力。
- 待收口项二：`PriceLimitEventProvider` 未纳入 `ProviderSettings`，但 Bootstrap 会从全部启用 Provider 自动发现并注册。需明确它是随 Provider 启用自动启用的基础证据能力，还是独立可配置能力；在语义明确前，不扩大自动注册范围。
- 文档问题：`DragonTigerProvider` 声明前的注释误写为交易日历说明，应在协议文档整理批次修正。
- 兼容性观察：`SupplementProvider` 与独立筹码、资金流接口存在方法重叠，当前作为旧补充源兼容入口保留；本批不删除，后续仅在确认无兼容调用后收敛。
- 本批未修改业务代码，未运行 gofmt/build/test；静态 Review 结论不代表 Adapter 字段映射、单位/时间/分页语义或真实 Provider 请求已经验收。
- 本批结论冻结。后续只处理上述明确边界，再进入核心 Quote/DailyBars Adapter 契约审查，不重新打开已通过的接口总体设计。

### 2026-11-10：Provider 核心行情 / 日 K 协议第二批静态 Review

- 审查基线：`630339634149520385dc8e7ba2669e6a5870ed71`。
- 审查范围：Tushare、Eastmoney、Hithink、Tencent、Sina、NetEase、BaoStock、Pytdx、TickFlow、Yahoo 的 Quote/DailyBars 入口与映射路径；公共 payload 归一化、Quote/DailyBar 字段校验、DataService 接收与质量处理；未执行测试或真实上游请求。
- 已确认的共同保护：日 K 统一校验证券身份、交易日期、OHLC、成交量单位、成交额单位/交叉关系、复权枚举、来源和质量状态；归一化会排序、去重、按请求区间过滤并限制条数。Quote 统一校验身份、OHLC、昨收/涨跌一致性、成交量/成交额单位及交叉关系。DataService 进一步核验 SourceVersion、NormalizationVer、时间戳、复权方法/参考日一致性和缓存质量。
- 已确认的适配差异：Tushare 通过原始日线加复权因子生成复权序列；Eastmoney/Hithink/TickFlow 按接口参数请求复权；BaoStock 使用 adjustFlag；Pytdx 使用 GBBQ 公司行动；Sina/NetEase 只接受 none；Tencent 缺少可验证成交额时明确 degraded；Yahoo 非官方源且缺成交额，保持 degraded。差异属于源端能力边界，不能强行抹平。
- 确认缺口：DataService 接收 Quote 后无条件将 `q.Source` 覆盖为 `p.Name()`；接收 DailyBars 时也无条件将每条 `Source` 覆盖为 `p.Name()`。当前会掩盖 Adapter 返回空来源或错误来源的协议违约，导致来源映射错误不能被识别/诊断。应在覆盖前校验：非空来源必须与 Provider.Name 一致；空来源按现有兼容策略可选择降级补齐或直接拒绝，但必须留下明确诊断。需补充 Quote 和 DailyBars 两类回归测试。
- 需要后续核验而非本批直接判错：各 Adapter 的成交量单位、成交额来源、百分比单位和时区仍需按 Provider 逐项对照真实接口/参考实现及黄金样本；本批仅确认代码路径和已有测试，不能替代真实数据证据。
- 复权逻辑核验：Service 已要求同一批日 K 的复权方法/参考日一致，并验证参考日格式、未来日期及与 K 线日期的方向关系；累计因子后复权允许无单一参考日。该规则本批未发现应推翻之处。
- 本批不改业务代码，不运行 gofmt/build/test。来源身份被覆盖是已确认缺口，进入下一修复批；字段映射与真实数据差分继续按 Provider 分组审查。

### 2026-11-10：Provider 核心字段映射 / 单位 / 时间语义第三批 Review

- 审查基线：`fd8a6a9703299d5808aa2610b543a1674bc41651`。
- 审查范围：Tushare、Eastmoney、Hithink、Tencent、Sina、NetEase、BaoStock、Pytdx、TickFlow、Yahoo 的 Quote/DailyBars 字段映射和对应契约测试；检查跨 Provider 日 K 比较器及其测试。仅静态检查代码和测试定义，不代表本轮执行了测试或真实上游采样。
- 已核实的单位处理：
  - Tushare 日 K 成交量 hands 转 shares，成交额 thousand_CNY 转 CNY，并保留 Raw 值和原始单位。
  - Eastmoney 日 K/Quote 成交量 hands 转 shares，成交额按 CNY；请求显式设置 `fltt=2`，要求价格/百分比按元和百分点返回。
  - TickFlow Quote/Kline 成交量 hands 转 shares；换手率按接口 ratio 转为百分点。
  - Tencent 对成交量单位进行交叉核验，无法确认时降级；成交额字段有精度/可信度判断，不把尾部字段直接当成交额。
  - Pytdx 对 amount 与 volume 不一致时将成交额置空并降级，不伪造成交额。
  - Sina、NetEase、BaoStock 对单位无法确认/可选字段缺失均有降级路径；Yahoo 未提供成交额时明确标记 unknown/degraded。
- 已核实的百分比处理：Tushare `pct_chg`、Eastmoney `fltt=2` 输出、Sina/NetEase/Tencent 基于价格与昨收计算的涨跌幅，以及 TickFlow 基于价格与昨收计算的涨跌幅，目标均为百分点语义；TickFlow turnover ratio 显式乘 100。需要用真实样本再次确认上游契约没有发生变化。
- 已核实的时间处理：Eastmoney、Tencent、Sina、NetEase 对中国本地时间/时间戳有显式解析；Hithink、TickFlow 对 Unix 毫秒时间戳解析；Yahoo 使用 Unix 秒时间戳；时间缺失或超前会走降级或拒绝路径，不以本地采集时间伪装 Provider 时间。Quote 的 `FetchedAt` 与 `QuoteTime` 语义分开。
- 已核实的跨 Provider 比较保护：日 K 比较器要求证券身份、日期集合、复权方法/参考日期可比较；对成交量粒度、成交额来源精度、缺失金额和 degraded baseline 有专门判定，不把不可比较当作不一致。
- 本批未发现可仅凭静态代码确定的新增单位换算错误；现有测试主要是 mock/fixture 契约测试，不能证明第三方接口当前线上字段定义仍与代码一致。
- 仍未完成的真实证据项：固定证券与交易日样本，对各 Provider 原始响应、Raw 字段、归一化字段和 Python/权威参考值做同日差分；尤其要覆盖 Tencent 成交量推断边界、NetEase/BaoStock 成交量单位识别、Hithink 成交额单位、复权因子跨源口径、交易时段内 Quote 时间偏差。无源端文档或可靠证据的字段继续标记 not_comparable/degraded，不得推定通过。
- 下一步转向 Adapter 契约边界：请求区间和 limit 是否严格遵守、分页/截断是否可检测、证券身份字段缺失时是否降级、可选字段解析失败是否保留结构化诊断，以及错误是否能正确区分上游故障与数据契约故障。
- 本批未修改业务代码，未运行 gofmt/build/test。此前记录的 Source 身份覆盖问题仍是独立待修复项，不因本批通过而关闭。

### 2026-11-10：Provider 请求范围、分页与截断边界第四批 Review

- 审查基线：`8ec74e13df37569f0e0283840b961156d293c53d`。
- 审查范围：Tushare、Eastmoney、Hithink、Tencent、Sina、NetEase、BaoStock、TickFlow、Pytdx 的日 K 请求参数、响应分页/截断、空响应、证券身份和公共归一化行为；本轮以静态代码检查为主，不代表运行测试或上游在线验收。
- 已确认的保护：
  - Hithink 日 K 按最多 10 年分段，并使用 offset 分页；重复日期、超区间日期、达到分页安全上限仍未结束均拒绝整批返回，避免静默截断。
  - Eastmoney 日 K 请求显式携带证券 secid、日期区间、复权参数和 limit；返回证券代码缺失时降级，代码不匹配时拒绝。
  - Sina 日 K 对上游 1023 条最大窗口有显式截断识别；当达到上限且最早返回日期仍晚于显式请求起点时拒绝不完整结果。
  - TickFlow 日 K 同时传递 start/end/count，校验必需数组长度、可选数组长度、日期区间和重复日期。
  - Tencent 日 K 传递日期区间和 limit，校验响应证券对象、行列数和日期范围；成交额未验证时不从未定义尾列推算。
  - BaoStock 校验每行证券代码、日期、复权标记和重复交易日；显式日期区间仍由公共归一化器执行 limit 上限。
  - NetEase CSV 校验必需列、每行证券代码和日期范围；响应无有效行时返回错误。
  - 公共归一化器按日期升序、去重、过滤区间，并在超过 limit 时保留最新 limit 条；截断有结构化 info 诊断，不作为质量降级原因。
- 需要继续核验的边界：
  - Tushare daily 请求本身不带 limit/offset，完整性依赖上游按日期范围返回全部行以及响应体安全上限；公共归一化器会对超过 limit 的数据保留最新 N 条并产生 `bars_truncated_to_limit` 信息诊断。需确认 DataService 的诊断消费路径能让调用方知道“请求范围内结果被 limit 截断”，不能把它解释成上游分页失败。
  - NetEase 历史 CSV 没有可见的 total/分页游标；若上游对长日期范围静默缩短结果，当前实现无法仅凭响应证明覆盖完整。需要真实样本或稳定的上游行数/边界约定；未证实前不标记为完整覆盖。
  - Eastmoney 的空 K 线数组会被记录为 empty_result，最终由公共归一化器以无有效日 K 错误拒绝；应在后续契约测试确认诊断同时保留。
  - BaoStock、NetEase、Sina 等不提供可验证总数的接口，不能单凭“返回非空”断言显式日期范围内没有漏行；应通过固定交易日历和真实样本做差分，而不是按自然日推断缺失交易日。
- 本批未确认需要立即修改的代码缺陷；以上未证实点保留为 coverage evidence 项，不以猜测改造接口或领域模型。
- 下一步按既定计划进入接收边界修复：DataService 不得无条件覆盖 Adapter 的 Source；非空来源与 Provider.Name 不一致应拒绝并记录诊断，空来源按明确兼容策略降级补齐并留痕；同时检查 Quote/DailyBars 两条路径及缓存/数据库回源。
- 本批未修改业务代码，未运行 gofmt/build/test。

### 2026-11-10：DataService Provider 来源身份接收边界修复

- 修复范围：Quote 与 DailyBars 的 Provider 接收入口。
- 非空 `Source` 必须与当前调用的 `Provider.Name()` 一致；不一致时整份 Provider 响应按契约失败处理，触发后备 Provider，不再静默覆盖真实来源。
- 空 `Source` 保留有限兼容：允许以当前 `Provider.Name()` 补齐，但必须将结果降级，并添加 `provider_source_missing_assumed_from_provider_name` 质量原因。
- DailyBars 在逐条处理前先检查整批 Source，避免一批数据中存在错误来源时只接受部分数据。
- 新增 Quote / DailyBars 的来源不匹配回退测试，以及来源缺失显式降级测试。
- 缓存回源路径：Quote 缓存和 DailyBars 缓存均要求 Source 非空并通过质量/版本校验；MariaDB 历史日 K 回源后按 Source + SourceVersion + 复权元数据分组，缺少来源的历史行不会进入可复现序列。本修复针对实时 Provider 接收路径，不改变历史存量数据。
- 尚需核对其他非 Quote/DailyBars 能力入口中“空值补齐”和“非空来源不匹配”的一致性；本次不扩大修改范围。
- 本轮仅完成代码与回归测试编写；按开发分支约定，未运行 gofmt/build/test。

### 2026-11-10：证券目录类 Provider 来源身份接收边界修复

- 范围：StockBasic、StockSearch、StockBoards 三类接收入口。
- 三类响应的非空 Source 必须与当前 Provider.Name() 一致；不匹配时拒绝当前响应并继续后备 Provider，不再将错误来源覆盖成当前 Provider 名称。
- 空 Source 继续兼容，但补齐来源时显式降级并追加 `provider_source_missing_assumed_from_provider_name`。
- StockBoards 同时检查快照级 Source 和每个成员项的 Source；任一非空来源不匹配即拒绝整份快照，任一来源缺失则对应成员和快照保留降级证据。
- 新增 8 项回归测试覆盖 StockBasic、StockSearch、StockBoards 的来源不匹配回退和来源缺失降级。
- 本批不涉及 ChipDistribution、CapitalFlow；这两类当前仍存在无条件覆盖 Source 的路径，留待下一批按同一接收边界规则处理。
- 本批仅完成代码和测试编写，未运行 gofmt/build/test。

### 2026-11-10：筹码分布与个股资金流来源身份接收边界修复

- 范围：ChipDistribution、CapitalFlow 两类独立补充数据入口。
- 非空 Source 必须与当前 Provider.Name() 一致；来源不匹配时拒绝当前响应并继续后备 Provider。
- 空 Source 兼容补齐为当前 Provider 名称，同时将质量降级并追加 `provider_source_missing_assumed_from_provider_name`。
- 保留原有 SourceVersion、Normalization/字段质量校验和 fetched_at 采集时间语义；不改变缓存键、TTL 或 Provider 协议。
- 新增两组回归测试，分别覆盖两类能力的来源不匹配回退与来源缺失降级。
- MarketCapitalFlow 已有严格来源匹配校验，本批未重复修改；该入口当前将空来源也作为不匹配拒绝。
- 本批仅完成代码和测试编写，未运行 gofmt/build/test。

### 2026-11-10：指数行情、市场广度、板块排行来源身份接收校验

- 范围：IndexQuotes、MarketBreadth、SectorRankings 三个快照入口。
- 在原有结构/字段校验前增加当前 Provider 身份核验：快照 Source 非空但不等于 Provider.Name() 时，记录契约失败并尝试后备 Provider。
- Source 为空仍由原有领域校验拒绝，不擅自推断来源；快照成员与快照来源不一致仍由原有校验器拒绝。
- 本批不改变质量状态、缓存策略、领域模型及 Provider 接口。
- 未运行 gofmt/build/test。

### 2026-11-10：补齐市场快照来源身份回归测试

- 范围：IndexQuotes、MarketBreadth、SectorRankings 三个 DataService 接收入口。
- 新增回归测试：首选 Provider 返回与自身名称不一致的非空快照来源时，必须拒绝该结果并继续调用后备 Provider；后备 Provider 的快照来源与其 Name 一致时，返回后备结果。
- 测试使用最小合法降级快照，避免把字段质量规则或 Provider 时间戳要求混入来源身份测试的断言。
- 本批只补齐来源不匹配回退测试；空来源仍由各自既有领域校验拒绝，成员来源一致性继续由快照 validator 检查。
- 测试源码已提交，但按开发分支约定未运行 gofmt/build/test；测试结果待 integration 统一验收。

### 2026-11-10：涨停池与龙虎榜来源身份接收校验

- 范围：LimitPool、DragonTiger 两个分页/快照接收入口。
- 在原有快照校验前核对非空快照 Source 与当前 Provider.Name()；不匹配时拒绝该候选并继续后备 Provider。
- 空 Source 不做来源推断，继续由原有 validator 拒绝；快照成员 Source 与快照 Source 不一致仍由原有成员校验拒绝。
- 两个入口原本没有接入该路径的 Provider 熔断器契约失败计数，本次不改变其既有熔断/重试策略，仅按现有失败收集与后备顺序处理。
- 不改变 HasData/QualityUnknown 候选保留策略、分页计数、缓存 TTL、缓存键及领域模型。
- 本批完成接收逻辑修复；专项回归测试尚未补齐，未运行 gofmt/build/test，不能视为测试验收完成。

### 2026-11-10：补齐涨停池与龙虎榜来源身份回归测试

- 新增 LimitPool、DragonTiger 两项 DataService 回归测试。
- 两项测试均使用来源身份有效的 unknown 空快照作为后备结果：首选 Provider 返回其他来源名称时应被拒绝，后备 Provider 被调用，最终保留后备来源的 unknown 候选。
- 测试同时断言结果不是缓存命中，并检查两个 Provider 的调用次数，覆盖来源错误不会提前结束后备链。
- 该测试验证来源身份与既有 unknown 语义共存，不把“空结果 unknown”误判为成功确认的零事件/零涨停池。
- 测试源码已提交；按开发分支约定未运行 gofmt/build/test，需由 integration 统一执行。

### 2026-11-10：财务、公告、估值 Provider 来源身份接收校验

- 范围：Fundamentals、StockAnnouncements、Valuations 三个 DataService 入口。
- 三个入口在领域 validator 前核对非空快照 Source 与当前 Provider.Name()；不匹配时拒绝当前 Provider 候选并继续后备 Provider。
- 空 Source 不推断、不补齐，继续交由原领域 validator 拒绝；Valuations 的成员 Source/SourceVersion 与父快照一致性、公告成员来源/版本一致性仍由原 validator 负责。
- Fundamentals 的服务接收时间戳仍在来源身份检查通过后写入，避免错误来源候选被改写后进入正常质量校验。
- 新增三项回归测试，分别覆盖财务、公告、估值的错误来源回退；测试使用符合原领域约束的快照，并断言后备 Provider 被调用及最终来源正确。
- 不改变 PIT、财务分区、公告分页、估值缺失证券计数、质量候选选择、缓存键与 TTL，也不新增 Provider 接口或领域字段。
- 测试源码已提交；按开发分支约定未运行 gofmt/build/test，需由 integration 统一执行。
