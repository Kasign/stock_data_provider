# WebUI 与数据层接口协作清单

更新时间：2026-11-05
适用分支：
- WebUI：feat/webui-market-data-console
- 数据侧：feat/full-go-data-providers-api-switch

> 本文是 WebUI 与 Go 数据服务之间的跨端接口协作、差异登记和联调验收清单。它不是 OpenAPI，不替代任何一侧的总体开发规划，也不授权在当前分支直接修改另一侧代码。

## 1. 文档定位与职责边界

本文统一回答五个问题：
1. WebUI 实际需要什么接口、字段和交互语义。
2. 数据侧是否提供对应路由、Handler、服务逻辑和真实 Provider 能力。
3. WebUI 请求封装、数据侧实际 JSON 与权威 OpenAPI 是否一致。
4. 尚未具备的能力由哪一侧提出、评估、实施和验收。
5. 每项差异以什么代码、测试或真实环境证据关闭。

本文不承担：
- 不定义 HTTP 接口权威 schema；以对应服务 OpenAPI 和实际实现为准。
- 不维护 WebUI 总体任务排期；以 doc/web/Web整体规划.md 为准。
- 不维护数据侧总体开发排期；以 doc/api/数据侧整体规划.md 为准。
- 不重复编写 WebUI 页面功能说明、数据质量规则或 Provider 实现方案。
- 不把“已声明”“已注册”“已封装”“Provider 可用”和“真实数据验收通过”视为同一状态。

## 2. 权威信息来源

| 信息 | 权威来源 | 本文用途 |
|---|---|---|
| 行情服务 HTTP 契约 | services/market-data/openapi.yaml | 核对路径、方法、参数、请求/响应 schema、鉴权、错误和枚举 |
| 行情服务实际行为 | services/market-data/internal/httpapi、service、model、Provider 和测试 | 判断路由、字段序列化、数据映射及能力是否真实实现 |
| WebUI 实际请求 | doc/web/API_CONTRACT.md、web/src/api.js | 判断页面实际调用什么、参数如何传递、前端如何解释状态 |
| WebUI 产品需求 | doc/web/Web整体规划.md、功能说明和专项文档 | 确认接口需求是否仍有效及产品优先级 |
| 数据质量语义 | doc/api/数据质量与状态契约.md | 核对单位、时间、质量状态、缺失值和降级语义 |
| 数据侧任务状态 | doc/api/数据侧整体规划.md | 确认数据侧实施顺序、当前阶段和责任边界 |

发生冲突时：
1. 字段和协议语义以权威 OpenAPI、当前实现及其测试共同核实，不能只凭文档推断生产可用。
2. 实际实现与 OpenAPI 不一致时，先确认真实业务语义，再同步修正实现或契约，不能由 WebUI 私自猜测。
3. WebUI 调用行为以当前分支代码为准；产品需求变化先更新 WebUI 规划及功能文档。
4. 跨端任务优先级分别服从两侧总体规划；本文只维护跨端依赖和协作顺序。

## 3. 接口状态定义

| 状态 | 判定条件 |
|---|---|
| 需求已登记 | WebUI 有明确页面需求或已确认的产品场景 |
| 契约已定义 | 权威 OpenAPI 已定义路径、参数、响应和错误语义 |
| 服务端已实现 | 当前数据侧分支有路由、Handler、Service/Provider 映射及对应测试 |
| WebUI 已接入 | 当前 WebUI 分支有请求封装和实际页面调用 |
| 自动化契约通过 | 当前代码 SHA 对应的契约、单元或集成测试通过 |
| Provider 能力已验证 | 对应 Provider、数据域和权限条件有真实请求证据；不适用时说明原因 |
| 部署联调通过 | 目标部署环境验证请求、响应、权限、数据质量和错误降级 |
| 阻塞 / 不支持 / 跳过 | 分别记录环境依赖、明确能力边界或未配置凭据；不得统一记为失败 |

关闭一项跨端差异的最低条件：契约、服务端实现、WebUI 读取方式三者一致，并有自动化证据。若依赖真实 Provider 或部署环境，还必须单独记录真实验收状态；自动化通过不能替代真实验收。

## 4. 当前接口覆盖基线

以下是当前 WebUI API 契约文档记录的接入基线，用于安排核对，不等同于本次已完成逐接口代码审计。每次执行具体任务时，必须重新对照当前分支 HEAD。

| 业务域 | 主要接口 | 当前协作重点 |
|---|---|---|
| 行情与日 K | /quotes/{symbol}、/bars/{symbol}/daily、/bundles/{symbol}、/market/quotes | 价格/成交量/成交额单位、复权口径、行情时间、质量状态、Bundle 快照身份 |
| 全市场股票 | /market/stocks | 证券范围、分页总数是否下限、页内覆盖率、排序口径、MA 复权口径和逐股质量 |
| 指数与市场时段 | /market/indices、/market/index-catalog、/market/indices/{symbol}/constituents、/market/indices/{symbol}/daily、/market/session | 指数目录与行情快照区分、成分权重单位、历史日线复权、交易时段和时区 |
| 市场观察 | /market/breadth、/market/sectors、/market/sentiment、/market/sentiment/history、/market/limit-pools、/market/hot-stocks、/market/dragon-tiger | 统计分母、覆盖范围、分页/排序、历史快照真实性和数据日期 |
| 市场资金流 | /market/capital-flow | 与成交额、市场广度及个股资金流严格区分；明确数据源、权限、单位、时间和 503 降级 |
| 个股研究 | /stocks/{symbol}/profile、fundamentals、boards、announcements、chip-distribution、capital-flow、/market/valuations | 报告期/公告日、来源、质量、空数据、分页和 PIT 时间语义 |
| 交易规则 | /stocks/{symbol}/trading-status、/price-limit-rule、/market/trading-calendar | 交易日与 as_of、ST/停复牌/退市、规则生效日期和 unknown 语义 |
| 数据源与诊断 | /data-sources、/data-sources/diagnostics、POST /data-sources/probe 及探测历史接口 | 静态能力、被动运行统计、主动探测和持久化历史必须分开定义 |
| 分析任务 | 分析服务的任务、事件、Run 和健康检查接口 | 与行情服务分离；以 doc/api/分析任务.openapi.yaml 和分析服务实现为准 |

### 4.1 已知边界，联调时不得混淆

- GET /api/v1/data-sources 是 Provider 配置/静态能力信息，不代表 Provider 当前健康。
- GET /api/v1/data-sources/diagnostics 在 WebUI 契约中作为只读运行诊断快照使用；不得把它描述成主动上游连通性测试。
- POST /api/v1/data-sources/probe 属于主动探测流程，应按数据侧探测契约、权限、并发、超时和凭据状态执行；不能与被动诊断快照混为一谈。
- 未配置 Key/Token 的 Provider 应按 skipped、not_configured 或正式契约定义的状态表达，不应制造网络失败。
- 市场级资金流、个股资金流、成交额和市场广度是不同数据概念，不能相互替代。
- /market/capital-flow 当前 WebUI 契约记录为依赖具备权限的 Tushare moneyflow_mkt_dc；接口已注册不代表部署环境已配置权限或能取得数据。
- 通知渠道 CRUD、订阅管理、发送测试和投递记录尚不能仅凭浏览器本地偏好实现；本地草稿不代表服务端通知能力。
- 全市场本地筛选不等于服务端全市场策略扫描；板块涨幅/成交额排序也不等于近 5 日涨幅或板块资金净流入排行。

## 5. 跨端字段语义基线

以下规则是接口协作时必须核对的语义，不替代数据侧质量契约和 OpenAPI 的字段定义。

### 5.1 单位与数值

- 价格、成交额、成交量、权重、比例和百分比必须有明确单位及示例。
- 同一字段不能因 Provider 不同而改变单位或数值表达；Provider 原始单位转换由数据侧完成并有测试。
- 必须区分百分点与比例值。例如 1.25 表示 1.25% 时，不能在另一 Provider 返回 0.0125 却沿用相同字段语义。
- null、缺字段、零值和空数组含义不同；WebUI 只按契约展示，不推断或补零。

### 5.2 时间、交易日与历史有效性

- 区分 Provider 行情/事件时间、报告期、公告日期、生效日期、采集时间、标准化时间和分析参考时间。
- 日期按市场/交易所日历语义处理；带时区时间戳使用 RFC3339/ISO 8601，并明确时区。
- 财务、公告、交易状态和规则类数据保留 as_of、报告期或生效时间，避免未来信息污染历史查询。
- 日 K 和指数历史行情明确实际复权口径；请求参数不能冒充 Provider 已实际支持的复权能力。
- 交易时段和市场状态优先使用服务端返回的时间及状态，不由浏览器本地时钟推算。

### 5.3 质量、来源与错误

- 质量状态、质量原因、Provider/source、fetched_at、覆盖率及样本数量按各接口正式 schema 返回。
- unknown、stale、degraded、invalid 等状态不得被前端压缩成“正常/异常”两态；具体枚举以当前 OpenAPI 为准。
- 不完整样本不能展示为完整全市场结果；覆盖率必须明确分母和适用范围。
- 错误响应应保留稳定错误码、可读消息和适用的 request_id；不得向前端返回 API Key、Token、密码或上游敏感响应。
- Provider 未支持、未配置、限频、超时、数据质量拒绝和服务内部错误应尽量区分。

## 6. 跨端缺口与协作任务池

任务状态只描述跨端协作，不取代两侧主计划中的开发状态。首次登记时先标记“待当前 HEAD 复核”，避免沿用旧分支快照。

| 编号 | 协作事项 | 初始状态 | 主责侧 | 关闭条件 |
|---|---|---|---|---|
| API-01 | 全量接口覆盖对账：WebUI 请求封装、OpenAPI、路由、Handler、JSON 输出逐项映射 | 待当前 HEAD 复核 | 双方 | 形成接口矩阵；缺失/过期/未接入项均有明确分类 |
| API-02 | 指数目录、成分、指数日线、交易时段、交易状态和涨跌幅规则契约 | 待当前 HEAD 复核 | 数据侧主责，WebUI 配合 | 参数、枚举、日期边界、响应字段和测试一致 |
| API-03 | 全市场分页与批量行情的范围、总数下限、覆盖率、MA 和复权语义 | 待当前 HEAD 复核 | 双方 | 服务端分页与字段质量可验证，WebUI 不将页内结果解释为全市场排名 |
| API-04 | 市场情绪当前快照与历史快照的持久化、缺失交易日和覆盖语义 | 待真实验收 | 数据侧主责，WebUI 配合 | 有真实历史数据、数据库读写证据和无数据/缺日契约 |
| API-05 | 市场级资金流真实数据与 Provider 权限验收 | 依赖凭据/权限 | 数据侧主责 | 真实请求可复现；权限缺失、Provider 不可用和空数据状态可区分 |
| API-06 | 开发中 | Provider 清单、诊断、Probe、历史 API 已存在；跨 Provider raw 比较已支持成交量源粒度与成交额量化误差上界；本批补齐一侧 daily_bars Provider 失败时的配对诊断，复用现有响应字段，不新增契约 | P0-50：当前 SHA 的 BaoStock/Pytdx raw 双源真实请求；P0-51：阶段 B 整体 Review 与收口 | BaoStock AvailableAt 未知，PIT 继续 fail-closed；真实 Provider 最新专项尚未执行 | `6d62617973f106cf4eb8690ea76d5c663f47928e`（本批变更见当前提交） | 前序 Go suite `42027994789`、PR Go suite `42028409432`、Parity `42028409433`、WebUI `42028409422` success；当前 SHA `6d626179` 的 gofmt、market-data tests、MariaDB PIT 已通过，race/vet、Docker、Parity 待结果；WebUI success | Artifact `13410429933` 为历史响应离线重评，两证券各 54/54 字段 matched；不代表当前 SHA 新鲜验收 | 接口层已接入，待真实验收 |
| API-07 | 板块多维排行（近 5 日涨幅、资金净流入等） | 待能力评估 | 数据侧主责，WebUI 配合 | 有真实来源、完整候选范围、排序/分页契约及质量证据；不以成交额冒充净流入 |
| API-08 | 通知渠道、订阅、发送测试与投递记录服务端能力 | 未提供服务端接口 | 数据侧主责；WebUI 后续接入 | 服务端安全存储、鉴权、审计、重试和投递查询契约完整 |
| API-09 | 服务端选股策略、全市场扫描、候选理由与历史记录 | 待产品/API 设计 | 双方 | 明确策略 schema、扫描范围、异步任务、结果快照和可追溯理由 |
| API-10 | 全球市场指数与交易日历 | 暂缓 | 双方 | 国内 A 股主线稳定后，先完成数据源授权、币种、时区、延迟和交易时段评估 |

### 6.1 任务推进原则

1. API-01 先建立当前 HEAD 的真实基线；其余任务不能只凭本表中的旧状态直接宣布完成。
2. 先修正已有接口的契约差异，再讨论新增接口；不得为填补 UI 空白而重复造 API。
3. 新增能力先定义业务口径、数据来源、质量和错误语义，再设计 OpenAPI；不能先由 UI 字段倒推虚构数据。
4. 数据侧优先复用已有 Provider、标准化、Validation、Reporter、Probe 和 Freshness 体系；不得另建平行诊断框架。
5. 需要数据库迁移、复杂业务模型、权限变化或服务架构调整时，先形成设计和验收方案，由用户确认后再开发。
6. WebUI 侧只负责请求、状态和交互；不在浏览器复制 Provider 选择、数据标准化或质量判定逻辑。
7. 每个任务只在对应代码分支实施：数据侧改 Go/OpenAPI/数据库，WebUI 侧改 web/**；跨端文档可在当前获准分支同步。
8. 同一差异需要双方修改时，先确定服务端契约，再由 WebUI 按已确认契约接入；不得依赖未合并分支中的临时字段。

### 6.2 数据侧 API 开发进度（由数据侧维护）

本节是数据侧更新 API 开发进度的固定区域。已复核代码基线 `687b49a190e661bed0855ab3f5bfd367bf9bd4c1` 的 Push Go suite Run `42017059947`、Push Live Probe Run `42017059794` success；BaoStock 专项实际执行并产出八只证券报告。报告主窗口 raw/qfq/hfq 各 9 行且 OHLC mismatch=0；首因子日期与独立上市日期相同，首事件前 fallback 对本批样本 not_applicable。当前最新代码 SHA `2c3fb0b0373d72a45ee408a8ccc09a5d1ded170d` 已触发新 Push/PR 工作流，待完整验收。数据侧每完成一个开发节点、测试节点或真实数据验证节点，都应更新对应行；不需要等待 WebUI 联调完成才更新。

**填写规则：**
1. 只更新自己负责的 API 任务行，不覆盖 WebUI 接入状态。
2. “开发状态”反映数据侧实现进度；“跨端状态”反映双方联调结果，两者不能互相替代。
3. 未核实的内容填写“待核实”，未执行的测试填写“未执行”，未配置凭据填写“跳过/未配置”，不要推断为通过或失败。
4. 每次更新至少填写数据侧 HEAD；有代码变更时填写对应提交 SHA。测试结果和真实请求结果分别记录。
5. 已完成的开发项仍需保留测试证据；如果后续发现回归，应更新为“阻塞”或“开发中”，不要删除历史事实。
6. 数据侧总体阶段和任务优先级仍以 doc/api/数据侧整体规划.md 为准；本表只记录与 WebUI 相关的 API 交付进度。

**开发状态枚举：** 未开始、设计中、开发中、待测试、已完成、阻塞、暂缓。

**跨端状态枚举：** 待 WebUI 接入、待联调、联调中、联调通过、待真实数据验收、阻塞、暂不适用。

历史日 K Probe 日期窗口契约：`daily_bar_start_date` 与 `daily_bar_end_date` 必须成对提供，仅作用于 `daily_bars`，按 Asia/Shanghai 的闭区间日期解释，最多 31 个自然日且结束日期不能晚于今日；省略时保持原有近期 5 条探测行为。响应中的 `requested_daily_bar_start_date/end_date` 仅回显请求参数，不代表 Provider 数据时间或 PIT available_at。WebUI 当前不依赖这两个字段，也无需因此改变页面请求；数据侧历史验收可直接使用该能力。

| 编号 | 数据侧开发状态 | 已完成内容 / 当前进度 | 未完成内容 / 下一步 | 阻塞与依赖 | 数据侧 HEAD / 提交 SHA | 自动化测试证据 | 真实数据验证 | 跨端状态 |
|---|---|---|---|---|---|---|---|---|
| API-01 | 开发中 | 当前 HEAD 已有 OpenAPI、HTTP 路由和 WebUI API 清单；已补齐 `SectorRanking.change_percent_5d`、`ProviderProbeItem.validation_*` Schema；`coverage_dimensions` 原已存在 | 继续完成所有端点的 OpenAPI Schema、Handler、JSON 字段逐项核对 | 剩余差异待逐接口复核 | `8decb2a8d796f54d6f1552928bb599889aa93c0a` | Go test suite Run `41755222811` 通过；Schema 契约测试检查字段归属 | 未执行全量真实接口对账 | 进行中 |
| API-02 | 已完成 | 指数目录、成分、指数日线、指数行情、交易日历、交易时段、证券交易状态、涨跌幅规则均有路由和 OpenAPI 定义；WebUI 已有对应请求封装 | 逐 Provider 补充真实数据覆盖和边界证据 | Provider 能力按源不同 | `05949306eba5c4cbb5b9c62bc2d656a1efc24db9` | Go test suite Run `41622693853`；WebUI checks Run `41622693832` 均通过 | 未完成逐 Provider 真实验收 | 待真实数据验收 |
| API-03 | 已完成 | `/api/v1/market/stocks` 提供分页行情与 MA 参数；批量 quote 最多 50 只；WebUI 已封装并用于当前分页选股及自选股批量刷新 | 补全分页总数/覆盖边界、均线复权语义及真实样本质量验证；明确当前选股仅作用于已加载分页 | 不具备全市场筛选策略执行 API | `05949306eba5c4cbb5b9c62bc2d656a1efc24db9` | Go test suite Run `41622693853`；WebUI checks Run `41622693832` 均通过 | 未执行全市场真实覆盖验收 | 待真实数据验收 |
| API-04 | 开发中 | 市场情绪快照/历史和市场级资金流历史接口及 WebUI 请求封装已存在 | 核实市场情绪历史缺失交易日、幂等写入、保留策略和真实数据库数据；补专门集成证据 | 需要 MariaDB 持久化与真实历史数据 | `05949306eba5c4cbb5b9c62bc2d656a1efc24db9` | Go test suite Run `41622693853` 的 MariaDB migration/PIT 集成通过；未证明 sentiment 专项全链路 | 未执行生产数据历史验收 | 待真实数据验收 |
| API-05 | 已完成 | `/api/v1/market/capital-flow` 已提供，使用 Tushare `moneyflow_mkt_dc`；WebUI 已封装 | 核对 Token 权限、字段单位、交易日覆盖、源端时间和历史数据准确性 | 依赖 Tushare 权限与凭据 | `05949306eba5c4cbb5b9c62bc2d656a1efc24db9` | Go test suite Run `41622693853` 通过 | 真实权限/历史值验收未完成；未配置凭据时应跳过 | 阻塞真实验收 |
| API-06 | 开发中 | Provider 静态清单、被动诊断、主动 Probe、Probe 历史均有 API；DailyBar 交易上下文、复权元数据、跨 Provider raw 比较及源精度容差已实现；P0-49 补齐失败 Provider 与成功快照的配对诊断，不新增 API/Schema/数据库字段 | P0-50 当前 SHA BaoStock/Pytdx raw 双源真实请求；P0-51 阶段 B Review；之后按整体规划进入 Fundamentals | BaoStock AvailableAt 未知，PIT fail-closed；当前 SHA 真实 Provider 请求未执行 | `6d62617973f106cf4eb8690ea76d5c663f47928e`（当前代码 HEAD `6d62617973f106cf4eb8690ea76d5c663f47928e`） | 前序 Go suite `42027994789`、PR Go suite `42028409432`、Parity `42028409433`、WebUI `42028409422` success；当前 SHA `6d626179` 的 gofmt、market-data tests、MariaDB PIT 已通过，race/vet、Docker、Parity 待结果；WebUI success | 历史 Artifact `13410429933` 离线重评 600036.SH、002982.SZ 各 54/54 字段匹配；当前 SHA 新鲜验收未执行 | 接口层已接入，待真实验收 |
| API-07 | 开发中 | 板块排行支持涨幅、成交额、近 5 日涨幅排序；`change_percent_5d` OpenAPI Schema 已补齐；WebUI 已封装并支持排序切换 | 评估真实资金净流入来源和完整候选集；补领涨股标准证券代码字段前不开放跳转 | 当前没有独立资金净流入 API；`leading_stock_symbol` 缺失 | `8decb2a8d796f54d6f1552928bb599889aa93c0a` | Go test suite Run `41755222811`、WebUI checks Run `41755222944` 通过 | 板块资金净流入真实来源未验收 | 部分联调完成 |
| API-08 | 暂缓 | 当前没有服务端通知渠道、订阅和投递记录闭环 | 需要先形成服务端安全设计与 API 方案 | 用户确认复杂方案后再实施 | `05949306eba5c4cbb5b9c62bc2d656a1efc24db9` | 不适用 | 不适用 | 暂不适用 |
| API-09 | 暂缓 | 当前分页行情不等同于服务端策略执行或全市场扫描 | 先定义策略 schema、异步任务、结果快照和可追溯理由 | 需先确认产品/API 设计 | `05949306eba5c4cbb5b9c62bc2d656a1efc24db9` | 不适用 | 不适用 | 暂不适用 |
| API-10 | 暂缓 | 国内 A 股主线优先 | 国内主线稳定后评估全球市场指数、日历、币种及时区 | 数据源授权和优先级 | `05949306eba5c4cbb5b9c62bc2d656a1efc24db9` | 不适用 | 不适用 | 暂不适用 |

**单项进度更新示例：**

| 字段 | 示例 |
|---|---|
| 编号 | API-03 |
| 数据侧开发状态 | 开发中 |
| 已完成内容 / 当前进度 | 已完成分页参数校验和总数口径统一 |
| 未完成内容 / 下一步 | 补充 MA 复权口径测试及覆盖率边界测试 |
| 阻塞与依赖 | 无 |
| 数据侧 HEAD / 提交 SHA | 填写实际提交 SHA |
| 自动化测试证据 | 填写实际执行命令、通过/失败及失败摘要 |
| 真实数据验证 | 未执行 / 通过 / 失败，并说明 Provider、日期和样本 |
| 跨端状态 | 待 WebUI 接入 |

## 7. 差异登记与关闭模板

发现具体差异时，按以下格式登记；一个问题对应一个编号，避免在段落中混合多个责任方。

| 字段 | 必填内容 |
|---|---|
| 编号/标题 | 稳定编号及简明问题描述 |
| 影响场景 | 哪个页面、操作或数据流程受到影响 |
| 当前 HEAD | WebUI SHA、数据侧 SHA；未检查的一侧标记未检查 |
| 预期契约 | OpenAPI 路径/字段/参数/状态及业务口径 |
| 实际证据 | Handler、Service、JSON、Provider 返回或浏览器请求证据 |
| 差异分类 | 路由缺失、schema 缺失、字段不一致、能力不支持、权限阻塞、UI 未接入、真实数据异常等 |
| 主责与配合 | 数据侧 / WebUI / 双方，以及配合事项 |
| 修复与验证 | 代码提交、测试命令、CI SHA、真实环境请求证据 |
| 结论 | 已关闭、降级可接受、阻塞、不支持或待复核 |

关闭问题时必须记录实际执行过的测试和环境。未执行的测试标记“未执行”；无凭据标记“跳过/未配置”；历史 SHA 的通过记录不能代表当前 HEAD。

## 8. 本文维护规则

1. 本文只保留跨端协作任务、接口差异和关闭证据；具体字段 schema 留在 OpenAPI，WebUI 调用细节留在 doc/web/API_CONTRACT.md。
2. WebUI 总体任务状态只更新 doc/web/Web整体规划.md；数据侧总体任务状态只更新 doc/api/数据侧整体规划.md；数据侧与 WebUI 相关的 API 实施进度必须同步更新本文第 6.2 节。
3. 新发现的差异先核实当前两侧 HEAD，再登记；旧记录若已失效，标记“历史记录/待复核”，不直接覆盖为当前事实。
4. 数据侧每完成一个 API 开发、测试或真实数据验证节点，应及时更新第 6.2 节；每次跨端联调完成后，更新任务状态、双方 SHA、证据和未解决项。
5. 变更接口路径或文件位置时，检查 doc/README.md、双方规划、WebUI API 契约、OpenAPI 引用及相关 Markdown 链接。
6. 不在本文维护与接口协作无关的 UI 页面排期、Provider 内部实现细节或数据库内部设计。

## 9. 本次重整说明

- 将原英文文件名 webui-api-contract-gaps.md 更名为 WebUI与数据层接口协作清单.md。
- 将文档从“数据侧整改指令清单”调整为跨端协作台账，明确权威契约、双方责任和关闭证据。
- 原有重点接口、单位/时间/质量语义、市场资金流、诊断、通知等内容按当前协作主题重新归类。
- 原文中的历史提交 SHA 和“最新 CI 待结果”等快照状态不再作为当前结论；所有任务要求在当前 HEAD 重新复核。
- 本次不修改 Go、Provider、数据库、OpenAPI 或 WebUI 业务代码。


## 10. 2026-11-03 当前 HEAD 进度复核

- 当前分支 HEAD：`8decb2a8d796f54d6f1552928bb599889aa93c0a`；业务 Schema/测试变更提交 `064707e2`、`428878fc`。
- Go test suite Run `41755222811`：根模块、go-core、market-data 的 gofmt/go test/race/vet，MariaDB migration/PIT 集成，Compose 校验、部署脚本语法和两个 Go 镜像构建均通过；三个模块任务中的旧 Go 模块路径扫描均通过。
- WebUI checks Run `41755222944`、Live provider probe Run `41755222948` 均通过；Live workflow 通过不代表所有 Provider/能力真实验收通过。
- Go-Python parity Run `41755222861` 在当前 HEAD 通过；旧文档路径问题已由 `437af45c` 修复。
- 已发现 OpenAPI Schema 缺口：`SectorRanking.change_percent_5d` 及 `ProviderProbeItem.validation_*`、`coverage_dimensions`。数据侧按协作清单和整体规划优先补齐，WebUI 不复制服务端质量规则。


## 11. 2026-11-04 DailyBar 复权专项测试进度

- 数据侧当前 HEAD：`24f6e24da44dd81d541f84b56316b65503534ea8`；本轮新增代码/测试基线：`e245c100a9d94a4c41a01f97327561b14ab0f1b8`。
- 已增加 Eastmoney、BaoStock、HiThink、TickFlow 的三种复权参数与 metadata 对应测试；Pytdx 增加 GBBQ 公司行动事件驱动的 QFQ/HFQ 协议级样例。
- Probe 错误分类增加 Eastmoney、BaoStock、HiThink、TickFlow、Pytdx 和 Tushare 的失败负向样例，避免 HTTP、鉴权、连接、超时和因子缺失误报 `not_supported`。
- `e771284b` Go test suite Run `41757355037` 通过；当前 HEAD `e245c100` 的 Go test suite Run `41757748059` 尚待结果。
- 真实 Provider × 复权口径请求未执行；Pytdx 适配器的 GetGbbq 网络失败端到端模拟仍未完成。WebUI 本轮未修改业务代码，跨端状态保持待真实数据验收。


## 11. 2026-11-04 数据侧 DailyBar 诊断进度

- 数据侧 HEAD：`ed18e0642f777646eb778f8b9521abb7fed4d518`。
- 当前 HEAD 的 Go test suite Run `41758382946`、Go-Python parity Run `41758383176`、Live provider probe Run `41758383404`、WebUI checks Run `41758382953` 均通过。
- 已有 Normalize 测试覆盖日期包含边界、非法日期、重复交易日过滤和 limit 截断；已有 DailyBar Validator/Reporter 基础测试。当前首轮审计发现重复记录与 limit 截断尚未完整映射为结构化 Reporter 计数，开始日期前记录存在静默过滤路径。
- Pytdx GetGbbq 失败路径尚无确定性端到端测试；生产代码直接依赖具体 `*tdx.Client`。下一步先评估最小内部客户端注入方案，不改变公开 API 或生产请求行为。
- 本轮未执行真实 Provider × 复权口径请求；CI Live Probe 通过不替代真实逐口径验收。

## 12. 2026-11-04 DailyBar 诊断实现进度（历史快照，以第 13 节为准）

- 数据侧本轮代码集中提交 1160d221，包含 DailyBar Reporter 计数、Probe 字段映射、OpenAPI Schema、Pytdx 日线客户端注入及回归测试。
- Reporter 已支持 FieldIssue.count 聚合计数；Normalize 阶段重复日期和 limit 截断已纳入结构化报告，日期范围问题由 Validate 阶段 out_of_range 统一表达，避免双计。
- limit 截断使用 Info 级诊断，count 为实际截断行数；诊断明细仍受报告 maxIssues 限制，总计数不受明细截断影响。
- Pytdx 日线复权客户端新增内部注入接口，覆盖 GBBQ 超时、连接失败和空事件列表；Probe 测试锁定网络超时不能被分类为 not_supported。
- 本轮没有执行 Go 测试命令，所有新增测试等待当前分支 CI 验证；真实 Provider × 复权口径请求仍未执行。WebUI 无需修改业务代码，当前接口响应的 validation_issues[].count 仅为诊断聚合字段，不改变既有路由或业务参数。

- 2026-11-04：诊断 FieldIssue 增加可选 count 聚合字段并同步 OpenAPI Schema；首次 CI 检查发现新增文件 gofmt 格式问题，已按日志修正，最新 SHA 的 Go test suite 待重新验证。WebUI checks 在 SHA 24013849、eefdeb19 均通过；其余工作流状态以最新 SHA 的 Actions 为准。

- 本轮代码功能按批次集中提交为 1160d221；PR #1 仍为 Open / Draft。最新 Go test suite、Parity 和 Live Probe 的结果以该 SHA 对应 Actions 为准；当前 Go test suite / Parity 尚未完成，WebUI checks 已通过。


## 13. 2026-11-04 DailyBar Reporter 回归修复

- 当前数据侧 HEAD：`c4438a9d1822c292eae4d4313011718ce2fd0c0a`。
- 修复 `Report.Add` 未按 `FieldIssue.Count` 加权累计的问题；Count 缺省/非正数按 1 兼容，TotalCount 与 Error/Warning/Info 计数保持一致。
- 修复 `Report.Merge` 将聚合记录数量误判为明细 Truncated 的问题；只传播显式截断或合并时实际触发的明细截断。
- 修复 Pytdx Probe 测试使用不存在的 `SetDailyBarProviders`；改为在 `NewWithCapabilities` 构造时传入 DailyBarProvider，不增加生产 API。
- 新增聚合 Merge 和兼容回退测试。父提交 `e1e61f68` 的 Go suite、Parity 失败；修复后 HEAD 尚无 Actions 运行记录，当前 CI 状态为未验证。
- WebUI 请求封装、响应字段和路由未变化，无需 WebUI 修改；API-06 跨端状态仍为“接口层已接入，待真实验收”。
- 真实 Provider × 复权口径请求和 MariaDB Probe 历史部署验收未执行。


## 14. 2026-11-05 数据侧 API-06 复权元数据进度更新

- 当前业务代码复核基线：`cf69ccc4e8d81cfa44b11a9a61cde3666ee4d50f`。
- 该代码基线的 Go suite Run `41960803425`、Go-Python parity Run `41960803760`、WebUI checks Run `41960803407`、Live Provider Probe Run `41960803434` 均通过。
- 本批次仅核验 Provider 实现、已有测试和源端公开资料；没有修改 API 路由、请求/响应 Schema 或 WebUI 调用，因此 WebUI 不需要调整。
- BaoStock 的复权因子算法和 qfq/hfq 基准定义有官方资料；当前 K 线 Bar 仍未填 `AdjustmentRefDate`，须通过因子历史与实际 K 线样本建立可重复证据后再决定是否填充。
- Eastmoney `fqt=1/2`、Tencent `qfq/hfq` 的请求映射已在代码测试覆盖，但响应不提供独立参考日期；保持 `not_comparable` 门禁，不从请求日期推断。
- Tushare qfq 使用实际因子集合最大交易日作为参考日期；hfq 不填参考日期。当前账号 `adj_factor`/分红权限 40203，不重复请求。
- 跨端状态仍为“接口层已接入，待真实验收”；本轮无新增 WebUI 工作项。


## 12. 2026-11-05 DailyBar 日期边界诊断同步

- 数据侧本轮变更前 HEAD：`850d3b26931685a2bee7e704ac90d8caddb3a3db`。
- 发现并修复开始日期之前的 Provider Bar 在普通标准化 warnings 中静默丢弃的问题；新增 `bar_before_start_date_dropped`，与结束日期越界告警对称。
- 标准化层对倒置日期区间返回 `invalid_date_range`；结构化 Reporter 返回一条请求级 Error，避免按每行重复累计。
- 新增 Normalize 与 DataService 回归测试；未改变 OpenAPI、HTTP 路由、请求/响应字段或 WebUI 调用，WebUI 无需同步代码。
- 变更前基线 Go suite Runs `41963474550`、`41963482209`、Parity `41963482323`、WebUI checks `41963482255` 通过；Live Probe `41963482213` 工作流成功但真实 Provider 测试全部 Skip。本轮提交 CI 待验收。
- 跨端状态仍为“接口层已接入，待真实数据验收”；真实停牌样本和 Provider 复权锚点未完成。


## 13. 2026-11-05 DailyBar 请求级诊断与 MariaDB 日期边界同步

- 数据侧变更前 HEAD：`79c99fdfe8f40536a10c560f4d6052b2c9267837`。
- 重复日期判断移至行字段校验之后；无效重复行保留 OHLC/字段错误，不再被普通 warnings 单独归类为 duplicate；普通与结构化结果保持一致。
- 非法 start/end、倒置区间及非法 requested symbol 在结构化路径按请求级错误处理，每项一次，不按 Bar 行数重复计数。
- 增加 Reporter 120 行错误样本，验证明细截断时聚合计数不丢失。
- 复核 MariaDB 当前表和版本表均使用 start/end 闭区间；新增数据库集成测试验证闭区间和 PIT recorded_at 截止。未修改 SQL 查询、API 路由或 Schema。
- WebUI 无需改动；真实 Provider 日内停牌样本和复权锚点仍未验收。新提交 CI 待验证。


## 14. 2026-11-05 DailyBar 缓存与 MariaDB fallback 诊断保留

- 数据侧变更前 HEAD：`6b46bf0c4fa8388468a48105fbb4cd7fe7a7e8d6`。
- MariaDB fallback 不再丢弃 `ValidateAndSortBars` 返回的 warnings；最终响应合并来源降级、单来源/复权约束和行过滤原因。
- MariaDB 来源组选择调整为最新交易日优先，其次比较覆盖行数、采集时间和稳定 key；仍禁止跨来源、版本、复权 method/ref date 或 normalization version 混合。
- 缓存命中路径增加回归覆盖，确保缓存写入时 warnings 与本次重新标准化 warnings 均保留。
- 无 API、Schema、数据库结构、Provider 请求或 WebUI 变更；本轮 CI 待验收。


## 15. 2026-11-05 API-06 BaoStock 专项探测窗口修正

- 数据侧变更基线：`c22470bec87beb89961b76ad9856d5c8709aac73`；本批代码提交：`687b49a190e661bed0855ab3f5bfd367bf9bd4c1`。仅调整 BaoStock 专项测试探测，不涉及 WebUI 调用或 API Schema。
- 因子历史与首事件前 raw/qfq 请求统一从 1990-01-01 起查；报告 `history_start_date` 仅代表请求窗口起点。
- 首事件不晚于请求起点可判 `not_applicable`；首事件晚于起点但无 raw 行不能据此断言无历史，改为 `unavailable`；无效日期同样 `unavailable`。
- 新提交 CI 待验；八只证券真实报告尚未执行。WebUI 无需改动；API-06 继续“接口层已接入，待真实验收”。BaoStock AvailableAt 仍未知，不改变 PIT 门禁。


## 16. 2026-11-05 API-06 BaoStock 真实数据验收与上市日期判定

- 代码 SHA `687b49a190e661bed0855ab3f5bfd367bf9bd4c1` 的 Push Live Probe Run `42017059794` 实际执行 BaoStock 八只证券专项并上传报告；2025 主窗口 raw/qfq/hfq 各 9 行，OHLC mismatch=0。
- 八只证券首因子日期与独立上市日期证据一致；首事件前没有上市交易日，fallback 对这些样本 not_applicable，不计 matched。
- 新代码 SHA `2c3fb0b0373d72a45ee408a8ccc09a5d1ded170d` 将上市日期及来源 URL 纳入测试报告，修正分类；不新增生产 API 或 Schema。新 SHA CI 与专项报告待验。
- WebUI 无需改动；API-06 仍为“接口层已接入，待真实数据验收”。BaoStock AvailableAt 未知，PIT 继续 fail-closed。


## 17. 2026-11-05 API-06 复权因子持久化链路审查

- 当前代码基线 `cce59e8de493813a4b04f7d0bd24211e1e919bb1`。PIT 因子读取继续要求 Provider `AvailableAt` 有证明，系统 `SystemObservedAt` 从数据库 `recorded_at` 恢复；DailyBarsAsOf 使用 PIT 专用重建，不混用 Current helper。
- Current helper 当前没有生产调用者，普通 DailyBars 仍消费 Provider 原生复权 K 线。没有明确业务调用场景前不增加 Current 因子读取 API 或字段。
- 本批没有 WebUI 契约变化，WebUI 无需修改。BaoStock AvailableAt 未知，PIT fail-closed 不变。


## 18. 2026-11-05 API-06 上市日期分类重跑状态

- 代码 SHA `cce59e8de493813a4b04f7d0bd24211e1e919bb1` 的 Go suite `42018395454` success。Live Probe `42018395554` 外层 success，但 BaoStock 专项步骤 failure；3/8 样本有真实因子响应并验证 `not_applicable`，5/8 因登录 TCP timeout 为 unavailable。
- 完整八证券主窗口真实证据仍引用 SHA `687b49a1` 的报告；本次不将 5 个网络失败计为通过。无 WebUI/API 契约变化，Current helper 仍无生产调用场景，不新增接口。


## 19. 2026-11-05 API-06 DailyBar 复权能力矩阵复核

- 已核对 Provider 实现及 cross-provider comparator。adjusted 跨源比较需要 basis、quality、method、reference date 全部可比较；缺失或不一致返回 `not_comparable`，保持现有契约。
- 多个 Provider 未提供可信 `AdjustmentRefDate`，不允许用请求 end date 或当前日期推断。Provider method 标识不同也不能仅凭 qfq/hfq 名称相同而归并。
- 本轮无 WebUI/API 变化，无需 WebUI 修改；真实跨源 adjusted 样本不足，API-06 继续等待真实数据验收。


## 20. 2026-11-05 API-06 Pytdx 参考日期与 raw 跨源证据边界

- Pytdx qfq/hfq 参考日期按完整加载历史确定，而非请求返回窗口：qfq 可晚于请求 end，hfq 为完整历史最早日。Yahoo、Tushare 的参考日期定义不同，不能推断或统一覆盖。
- 600036.SH Sina/Tencent raw 样例存在 amount 缺失与 Sina symbol 未回显，质量降级，比较结果 not_comparable；不作为跨源验收通过。
- 下一项为 BaoStock/Pytdx raw K 真实双源对账诊断，无 WebUI/API 契约变更；PIT fail-closed 保持。


## 21. 2026-11-05 API-06 raw 双源成交量粒度与成交额差异

- 内部比较输入新增 `VolumeGranularity`，来源 `RawVolumeUnit`，hands=100 shares、shares=1、unknown=0；仅用于 comparator，不改变公共 API/数据库契约。
- 真实 raw 双源样本离线重评后，成交量差异在 100 股 TDX 粒度内；成交额仍有 17/18 行超过 1 CNY，整体 mismatch。下一项核查 amount precision/aggregation，不放宽容差。
- WebUI 无需修改；API-06 继续等待真实跨 Provider 数据验收。


## 22. 2026-11-05 API-06 成交额精度边界

- TDX 日 K amount 以整数 CNY 表示（协议 Price 内部为毫元，解码回元）；BaoStock amount 标记为 CNY，但暂无明确官方小数精度保证。真实样本 17/18 行超过 1 CNY，保持 mismatch。
- 不放宽 amount tolerance、不新增字段/API。下一项扩大不同成交额量级的真实样本并核查供应商聚合口径；WebUI 无需修改。


## 2026-11-05 API-06 成交量源精度容差修正

- 内部比较器容差修正为 `max(1, left.VolumeGranularity, right.VolumeGranularity)`；两侧 shares 为 1 股，包含 TDX hands 时为 100 股，未知粒度仍 not_comparable。移除固定 50 股下限，避免将来源粒度为 1 股的数据差异误放宽。
- 仅内部对账逻辑与测试变化，不新增 DailyBar API、数据库字段或 WebUI 展示字段。成交额容差仍为 1 CNY；既有真实样本整体 mismatch，等待不同成交额量级的新证据。
- 代码 SHA `5adb663ca41d6083847c7d497ca387a2931e2422` 的 Go suite/Parity 待验；WebUI 无需变更。

## 23. 2026-11-05 API-06 raw 双源成交额精度闭合

- 内部 comparator 增加 `AmountErrorBound`，Probe 从 DailyBar 的 Provider 来源、RawAmountUnit、RawAmountField、RawAmount 传递精度。Pytdx 按半个 float32 ULP 加 0.001 CNY Price 截断误差计算，BaoStock 按 4 位小数保守取 0.0001 CNY；容差为两侧绝对误差上界之和。未知来源/字段/单位仍 not_comparable。
- 真实 artifact `13410429933` 离线重评：600036.SH 与 002982.SZ 各 9 个共同交易日，OHLC/volume/amount 各项全部匹配，两只证券分别 54/54 字段 matched。成交量最大差 84/96 股，均在 TDX 100 股粒度内；成交额最大差 120.74/7.90 CNY，均在对应金额量级的动态量化容差内。
- 不新增公共 API、数据库字段或 WebUI 展示；仅服务端内部对账逻辑与测试变化。此次不是新鲜实时重跑，PR Live Probe 专项步骤 skipped；PIT fail-closed 规则不变。

## 2026-11-05 API-06 成交额误差上界修正

- Pytdx 先解码 float32，再将 CNY 乘以 1000 转为 `Price`（厘），存在不足 0.001 CNY 的截断误差；故误差上界为半个 ULP + 0.001 CNY。BaoStock 4 位小数未明确舍入方式，保守取完整 0.0001 CNY。
- `AmountGranularity` 更名为 `AmountErrorBound`，容差按两侧误差上界相加；新增 100,000 CNY 小额边界测试。公共 API、数据库、WebUI 无变化，PIT fail-closed 不变。


## 2026-11-05 API-06 P0-49 Probe 缺失侧诊断

- 当前分支 HEAD：`ee3726312ef78cc8d5b08541952312e69f79a6e4`。代码审查确认：一侧 daily_bars Provider 失败、另一侧成功时，原 cross_provider builder 仅遍历成功快照，最终可能只生成系统级 `cross_provider_pair_unavailable`；失败侧虽在普通 Provider 检查中存在，但未与成功侧形成配对诊断。
- 数据侧补充 Service 内部配对结果，复用现有字段记录左右 Provider、证券、复权口径、失败分类及原始错误摘要；网络/超时/HTTP/Provider 故障为 `provider_unavailable`，限流、鉴权/权限、契约错误分别分类。未新增 API、Schema、数据库字段，WebUI 无需修改。
- 新增连接失败回归测试；本批 CI 待验。没有执行真实 Provider 请求，不改变 API-06“待真实数据验收”状态。下一步 P0-50 使用当前提交执行 BaoStock/Pytdx raw 双源真实验收。
- 首轮 Push Go suite Run `42030476834` 的 market-data gofmt 检查发现新增结构体字面量两行字段未对齐，尚未进入测试阶段；已按 gofmt 输出修正，不涉及逻辑变化。PR Go suite `42030490186`、Parity `42030489938`、WebUI checks `42030490023`、PR Live Probe `42030489937` 当时仍在执行；修正后的 SHA 需重新验收。
- 修正提交 SHA `6d62617973f106cf4eb8690ea76d5c663f47928e` 已通过 gofmt、market-data package tests 与 MariaDB PIT 集成；race/vet、Docker build、Parity 待最终状态，WebUI checks success。PR Live Probe 未执行专项真实请求。



## 24. 2026-11-05 API-06 P0-50 生产 Probe 实时验收结果

- 生产 Probe Run `42031080952`（Artifact `13416895510`）对 `600036.SH`、`002982.SZ` 的 2025-07-07 至 2025-07-17 none/qfq/hfq 请求均完成 MariaDB 持久化与历史回读；每只证券 total=15、returned=0、returned_degraded=7、failed=6、not_supported=2。storage 通过，但生产测试因 Provider 失败而失败；Workflow 外层 success 由 job 的 continue-on-error 产生，不代表真实数据验收通过。
- BaoStock 登录 TCP timeout，Eastmoney 历史 K 线请求 EOF；Sina/Tencent/Yahoo 虽返回 9 行，但因 amount 缺失、日历上下文不可用等原因质量为 degraded。Tencent forward-adjusted 的参考日期也未验证。跨源比较因此按质量门禁 skipped，qfq/hfq 不得视为已验收。
- 每只证券已产生 14 条 BaoStock/Eastmoney 缺失侧 `provider_unavailable` 配对诊断；另 5 条因成功侧质量 degraded 而 skipped。P0-49 诊断修复在真实生产服务链路中生效，但没有一致性通过结论。
- Pytdx 专项被前序生产 Probe 失败通过默认 success() 隐式跳过；数据侧修正 Live Provider workflow 的独立专项条件为 `always() && ...`，后续仅运行 Pytdx/GGBQ 专项，不重复生产 Probe。该修正不改变 API、Schema、数据库或 WebUI 契约。
- 当前 WebUI 无需改动；API-06 继续标记“接口层已接入，待真实数据验收”，PIT fail-closed 不变。
