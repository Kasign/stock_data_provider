# WebUI 与 Go 数据层 API 缺口及字段契约整改清单

更新时间：2026-10-29  
提出方：WebUI 接入与契约核对  
目标分支：`feat/full-go-data-providers-api-switch`  
状态：**供数据层逐项核对与整改；未确认的项目标记为“待核实”，不得视为已确认缺陷。**

## 1. 目的与边界

本文把 WebUI 已有页面的实际需求、已发现的数据能力缺口及必须遵守的字段语义集中到公共 `docs/` 文档区，供数据层直接逐项核对。本文不要求为了满足 UI 而删除既有 Go 数据标准化、单位统一、质量门禁或来源元数据逻辑。

权威参照：
- [行情服务 OpenAPI](../../services/market-data/openapi.yaml)：实际 HTTP 请求/响应 schema 的首要协议来源。
- [WebUI API 接入契约（WebUI 分支）](https://github.com/Kasign/stock_data_provider/blob/feat/webui-market-data-console/web/API_CONTRACT.md)：当前前端实际请求路径和兼容行为。
- [WebUI 数据能力缺口（WebUI 分支）](https://github.com/Kasign/stock_data_provider/blob/feat/webui-market-data-console/web/DATA_CAPABILITY_GAPS.md)：产品能力、诊断与后端需求背景。
- [仓库 README](../../README.md)：服务架构、运行边界及数据质量门禁。

**重要：** API 在 OpenAPI 中声明，不代表当前部署版本一定已注册路由、接入真实 Provider 或能返回完整数据。请以 Handler、服务层实现、测试和实际运行结果逐项确认。

## 2. 先做接口覆盖对账

WebUI 已接入/计划使用的核心能力包括：

| 能力 | 现有/预期接口 | 数据层核对要求 |
|---|---|---|
| 实时个股行情、日 K、行情 Bundle | `GET /api/v1/quotes/{symbol}`、`/bars/{symbol}/daily`、`/bundles/{symbol}` | 确认路由、参数、回包字段、复权口径及来源/质量元数据一致；Bundle 的 `daily_bars`、`snapshot_id`、`analysis_reference_time` 等字段必须与 OpenAPI 和实际 JSON 一致。 |
| 指数列表 | `GET /api/v1/market/indices` | 与指数目录 `/api/v1/market/index-catalog` 区分：前者当前核心指数快照，后者目录/候选标的；明确两者用途，避免用目录数据冒充行情。 |
| 指数目录、成分、历史日线、交易时段 | `GET /api/v1/market/index-catalog`、`/market/indices/{symbol}/constituents`、`/market/indices/{symbol}/daily`、`/market/session` | 在当前目标分支逐项确认 Handler 注册、参数名称、支持的 source 枚举、日期边界、响应 schema、Provider 能力和测试覆盖。WebUI 已按 OpenAPI 声明接入；若实现缺失或契约不符，请修复服务端或同步更新正式 OpenAPI，并说明原因。 |
| 个股历史交易状态、涨跌幅规则 | `GET /api/v1/stocks/{symbol}/trading-status`、`/price-limit-rule` | 确认交易日、`as_of` 时间语义、ST/风险警示、停复牌、退市状态、板块/上市阶段涨跌幅规则以及未知状态表达。不得把缺失证据默认成正常交易或固定百分比。 |
| 公司资料、板块、财务、估值、公告、筹码、个股资金流 | `/stocks/{symbol}/profile`、`/boards`、`/fundamentals`、`/market/valuations`、`/announcements`、`/chip-distribution`、`/capital-flow` | 逐项核对 OpenAPI、Handler JSON、Go model JSON tag、实际 Provider 映射和前端读取字段；明确可选字段、空结果、分页、报告期/公告日与质量状态。 |
| 市场广度、板块排行、涨跌停池、人气榜、龙虎榜、交易日历 | `/market/breadth`、`/market/sectors`、`/market/limit-pools`、`/market/hot-stocks`、`/market/dragon-tiger`、`/market/trading-calendar` | 确认排序、范围、总数/分页、交易日期、采集时间、覆盖率和质量字段；实际未支持的排序或字段不得仅在 OpenAPI 声明。 |

请将核对结果按 **已实现且一致 / Handler 缺失 / OpenAPI 缺失或过期 / 字段名不一致 / Provider 未支持 / 需要新增数据能力** 分类，避免把所有问题混为“前端 Bug”。

## 3. 字段契约必须保留的语义

### 3.1 统一单位与标准化

- 价格统一为人民币元/股（CNY/share）；成交额统一为人民币元（CNY）；成交量统一为股（shares）。若 Provider 原始单位不同，必须在 Go 数据层归一化并用测试覆盖，不能要求 WebUI 猜单位或重复换算。
- 百分比字段必须明确是百分数值（如 `1.25` 表示 1.25%）还是比例值（如 `0.0125` 表示 1.25%）；同一字段不可因 Provider 不同而改变语义。OpenAPI 应明确单位及示例。
- 金额、成交量、价格、比例不得通过前端格式化掩盖后端单位不一致；新增/修改字段时同步更新 schema、模型、归一化逻辑和测试。

### 3.2 时间、交易日和历史有效性

- 分开保留 Provider 事件/行情时间、报告日期、采集时间、标准化时间及分析参考时间；不要把抓取时间伪装成行情发生时间。
- 日期按交易所/市场日语义表达；带时区的时间戳使用 RFC3339/ISO 8601 并明确时区。交易时段的当前状态由服务端返回，前端不以本地时钟自行推断。
- 财务、公告、交易状态及涨跌幅规则必须保留报告期、公告/生效日期与 `as_of` 时点，避免把后来才知道的信息倒灌到历史。
- 历史日 K 必须明确复权口径（`none`、`forward_adjusted`、`backward_adjusted` 或实际正式枚举），不能返回复权值却缺失实际口径。

### 3.3 数据质量、来源和缺失语义

- 保留并一致输出 `quality_status`（当前协议枚举：`valid`、`degraded`、`stale`、`invalid`、`unknown`）、质量原因/警告、source/provider、fetched_at 和适用的覆盖/样本数量。
- `unknown`、`null`、空数组、零值和确定无记录不是同一回事。不得把未知值转成 0、空字符串或“正常”；不得将不完整样本标为完整全市场结果。
- 若接口响应中有 `request_id`，错误响应也应保留稳定的错误码、消息与请求编号，方便按编号查日志；禁止输出 API Key、密码等凭据。
- 如果字段由 Provider 可选提供，应在 OpenAPI 明确可选性和缺失语义；不能只在 UI 文案中临时兜底。

## 4. 已知的数据能力缺口（需要数据层确认/评估）

以下是现有 WebUI 需求文档中登记的缺口。请确认当前分支是否已有新实现；若尚未实现，请给出是否支持、所需 Provider、估算/依赖及建议交付顺序。

| 优先级 | 缺口 | 期望交付/验收 |
|---|---|---|
| P1 | 全市场/批量股票概览 | 明确证券范围、分页/筛选、行情字段、MA 算法和复权口径、最后有效交易日、每只股票质量；避免前端逐只请求大量接口。 |
| P1 | 历史市场情绪快照 | 明确涨跌家数、涨停/跌停、炸板、连板等指标的分子分母/覆盖范围；定时持久化并按交易日查询；不得伪造历史曲线。 |
| P1 | 市场级资金流 | 明确净流入口径、单位、数据时间、报告日期、覆盖、来源与质量；不能把成交额字段当作净流入。 |
| P1 | 板块多维排行 | 今日涨幅、成交额、近 5 日涨幅、资金净流入分别定义排序、完整候选范围、分页和质量；不能只排序截断样本却宣称全市场 Top。 |
| P1 | Provider/能力真实诊断 | 受保护的逐 Provider、逐 capability 探测，具备并发限制、超时、冷却/限频、安全错误和最近成功信息；Provider 配置清单不等于实时健康。 |
| P2 | 全球市场指数与日历 | 先确认数据源、授权/延迟、币种、时区、市场交易时段和可比时点，再决定 API。 |
| P2 | 诊断历史 | 持久化最近探测结果、故障持续时间和状态变化，并明示存储不可用时的降级语义。 |

## 5. 本轮新增接口的重点核对清单

以下 6 个接口已由 WebUI 增加调用封装。请数据层对照当前分支确认 **真实 Handler、参数、响应字段和 Provider 支持**，不要只检查 YAML：

- [ ] `GET /api/v1/market/index-catalog`：`tag` 筛选是否实现；目录项至少包含 `symbol`、`name`、`tag`；快照的 `total`、`provider_returned`、`excluded_rows`、`total_is_lower_bound` 是否与真实过滤逻辑一致。
- [ ] `GET /api/v1/market/indices/{symbol}/constituents`：`source`、`as_of` 是否受支持；成分代码、名称、权重、权重单位、生效/报告日期、来源和质量是否定义清晰。
- [ ] `GET /api/v1/market/indices/{symbol}/daily`：`start`、`end`、`source` 是否受支持；OHLC、成交量/额单位、日期、复权口径、来源和质量是否完整。
- [ ] `GET /api/v1/market/session`：响应是否明确服务端当前时间、市场交易日期、会话阶段、时区，以及休市/未知状态；前端不应自行推算。
- [ ] `GET /api/v1/stocks/{symbol}/price-limit-rule?date=YYYY-MM-DD`：日期参数和规则生效日是否一致；规则缺失时是否明确 unknown，而不是返回猜测的固定涨跌幅。
- [ ] `GET /api/v1/stocks/{symbol}/trading-status?date=YYYY-MM-DD&as_of=...`：交易日和观察时点的含义是否明确；ST、停复牌、退市等状态的证据来源和未知状态是否保留。

如当前 Handler 与 OpenAPI 不一致，以真实业务语义为准修复两者，并添加 handler/service 单测；若接口在当前版本尚未实现，应明确记录未实现并避免 UI/文档声称生产可用。

## 6. 数据层执行步骤与交付格式

请按以下顺序完成一次可审查的契约整改，不要求一次实现全部新增数据能力：

1. 对 6 个新接口做路由、Handler、service、model、JSON tag、Provider、测试的逐层核对。
2. 运行 OpenAPI 与实现差异检查；补齐或修正 schema、必填/可选字段、枚举、单位、时间和缺失语义。
3. 修复已有 API 中已确认的字段命名/类型/单位不一致；保留已有 Go 统一单位、标准化、质量降级与门禁逻辑，不可为迁就前端而删除。
4. 为每项修复增加可重复的 Go 单元/Handler 契约测试，覆盖缺字段、null、unknown、空结果、非法参数、Provider 降级和真实单位转换。
5. 对尚缺的批量概览、历史情绪、市场资金流、板块多维榜、Provider 诊断分别记录接口草案/依赖，不要将规划标记为已实现。
6. 在本文更新结果表，列出修复提交、测试命令/结果、未解决项与原因；没有执行的测试不得标为通过。

### 整改结果跟踪表

| 项目 | 当前结论 | 修复提交/证据 | 测试结果 | 状态 |
|---|---|---|---|---|
| 6 个新增接口的路由/Handler | 已在 `server.go` 注册；目录、成分、指数日线、交易时段、涨跌幅规则、交易状态各有独立 Handler | `a65ac54d` 静态核对 | 相关 Handler 单测存在；最新代码 CI 待结果 | 已完成代码级初核，待核 Provider 能力与实际响应 |
| 单位、百分比和时间语义 | OpenAPI 已声明价格 CNY/share、成交量 shares、金额 CNY、百分比百分点；交易时段补充 `server_time` 与 `time_zone=Asia/Shanghai` | `72e9faf` | Go 测试待结果 | 部分修复，需继续对照实际 JSON |
| 涨跌幅规则空质量原因数组 | OpenAPI 将 `quality_reasons` 列为必填，但 Go JSON tag 使用 `omitempty`，导致无原因时字段消失；已改为始终输出数组并新增序列化回归测试 | 本轮修复 | 最新 HEAD CI 待结果 | 已修复，待 CI |
| 涨跌幅特殊事件证据结构 | Go `PriceLimitEventEvidence` 含事件状态、生效区间、来源版本、质量、公告 ID、观察时间与覆盖确认；OpenAPI 原先仅声明空 `object`，现已补齐组件 schema 并由规则响应引用 | 本轮契约修复 | 最新 HEAD CI 待结果 | 已修复，待 CI |
| 指数成分覆盖计数字段 | Go `IndexConstituentSnapshot.ProviderReturned` 无 `omitempty`，响应始终序列化；OpenAPI 原先未将 `provider_returned` 列为必填，现已对齐并加入契约测试 | `806679f` | 最新 HEAD CI 待结果 | 已修复，待 CI |
| 质量状态与 null/unknown 语义 | 涨跌幅规则缺少可信状态/特殊事件证据时返回 unknown；历史交易状态保留 unknown 与 PIT 存储边界；指数快照校验覆盖和来源质量 | 当前 Handler/Service/Model 核对 | 既有测试覆盖部分边界 | 已有保护，仍需补齐跨层契约测试 |
| OpenAPI 与鉴权定义一致性 | 修复 `ApiKeyAuth` 引用与定义的名称不一致；为六个重点接口统一 `ApiKey` 鉴权声明；发现并删除指数目录操作重复的 `security` 键；契约测试校验 YAML 可解析、六个接口各自仅声明一次鉴权 | `72e9faf`、`eb4524c`、`940a3a8` | 最新 HEAD 的 Go 全量测试/契约测试 CI 待完成；旧 HEAD 结果不能代替本次验证 | 代码已修复，等待最新 CI 确认 |
| 全市场批量概览 | 指定股票批量行情已实现；全市场分页证券池/概览仍需单独确认。日 K 与 Bundle 已提供服务端 MA5/MA20/MA120/MA250，MA120/MA250 分别近似半年/全年交易窗口；指标沿用日 K 复权口径，历史不足时为 null | `batch_quotes.go`、`service/moving_average.go`、`DailyBar` 与 OpenAPI | 新增 MA 代码与契约；全市场分页概览仍未实现 | 不得把指定股票批量行情等同全市场扫描；全市场概览仍需明确证券范围、分页、最后有效交易日、复权口径和逐股质量 |
| 市场广度、板块排行、个股资金流 | `/market/breadth`、`/market/sectors`、`/stocks/{symbol}/capital-flow` 已有路由与 Provider 能力接口；但市场广度成交额不等于净流入，板块排行当前仅定义 `type`、`direction`、`limit`，没有近 5 日涨幅/板块资金净流入等排序契约，个股资金流不等于市场级资金流 | `server.go`、`provider.go`、`model.go`、`openapi.yaml` | 部分已实现；未证明所有 Provider 都可用 | 保留现有能力；后续补板块多维真实排序与独立市场级资金流，不得以成交额冒充净流入 |
| 历史市场情绪快照 | 当前即时广度和涨跌停池已有 API；未见历史情绪快照的定时持久化与按交易日查询 API | 当前路由及模型目录核对 | 尚未实施 | 确认缺口；先定义分子/分母、覆盖率和质量，再新增存储及历史查询 |
| Provider/能力真实诊断 | `/api/v1/data-sources` 返回 Provider 配置与静态能力清单；不等于实时探测，未见逐 Provider/capability 主动探测、冷却/限频和最近成功时间的诊断 API | `server.go` 的 `dataSources` 与 `SourceStatus` | 部分已有静态清单；实时诊断尚未实施 | 确认缺口；在限流、超时、并发上限及敏感错误脱敏约束下新增 |
| 全球市场指数与日历 | 当前数据域以国内 A 股及配置的国内指数为主，未确认完整全球指数报价与全球交易日历能力 | 当前 Provider 能力接口与 OpenAPI 核对 | 尚未实施/待评估 | P2；先确认来源、授权、时区、币种和延迟，再决定接口 |


## Provider 运行诊断（已实现：五类真实业务请求的只读运行统计）

- 新增 `GET /api/v1/data-sources/diagnostics`，与静态能力清单 `GET /api/v1/data-sources` 分离。
- 当前诊断范围为 `quote`、`daily_bars`、`index_quotes`、`market_breadth`、`sector_rankings` 五类能力，状态直接来自 DataService 熔断器的真实业务调用结果。
- 返回最近成功/失败时间、累计成功/失败次数、连续失败次数及熔断/半开状态；未发生调用的能力为 `not_observed`，不将“已启用”误报为“健康”。
- 诊断接口不主动请求 Provider，不额外消耗上游额度；统计为进程内数据，服务重启后清零。
- 当前诊断覆盖 `quote`、`daily_bars`、`index_quotes`、`market_breadth`、`sector_rankings` 五类能力；状态来自对应 DataService 真实调用链的熔断器统计。
- 指数行情、市场广度和板块排行仅在真实请求成功且通过服务层数据校验后记录成功；Provider 请求错误或响应校验失败记录失败。请求取消释放半开探测，不记为 Provider 故障。
- 不支持的板块排行方向属于能力不支持，不记为 Provider 故障；熔断开启时跳过该 Provider，并继续尝试后备源。
- OpenAPI 的 `capability` 枚举必须与上述五类实现一致；WebUI 已按五类能力提供中文名称映射。
- 同花顺不执行主动真实请求测试，不校验第三方 API Key。WebUI 的行情服务 API Key 仅用于读取受保护的诊断快照，与 Provider 自身 API Key 不同。未配置同花顺 Key 时只按部署配置/静态注册状态展示，不判定为上游故障。
- 其他 Provider 能力（公告、资金流、龙虎榜、人气榜、涨跌停池等）尚未纳入统一运行诊断，不能据此推断其健康状态。

## 通知渠道接入边界

- 当前行情服务未提供通知渠道 CRUD、渠道凭据更新/脱敏读取、订阅管理、单渠道发送测试、投递记录查询等服务端 API。
- 设置中心通知渠道表单仅作为能力规划展示，保存渠道和发送测试按钮保持禁用；不得调用浏览器本地存储伪装服务端配置，不得展示虚构发送结果。
- `notification-preferences.js` 保存的是当前浏览器本地关注事件与免打扰时段，不代表服务端订阅、渠道启用或通知已发送。
- 后续服务端接入至少需定义渠道类型/配置、凭据安全存储、订阅关系、测试发送审计、投递记录及失败重试策略，并在 OpenAPI 中明确鉴权和错误语义。
