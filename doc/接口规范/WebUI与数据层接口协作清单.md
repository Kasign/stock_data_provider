# WebUI 与数据层接口协作清单

更新时间：2026-11-04
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
| API-06 | Provider 静态清单、被动诊断、主动 Probe、诊断历史的边界与字段对齐 | 待当前 HEAD 复核 | 数据侧主责，WebUI 配合 | 三类能力分离；OpenAPI、Handler、状态分类和持久化证据一致 |
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

本节是数据侧更新 API 开发进度的固定区域。当前数据侧分支 HEAD 为 `ed18e0642f777646eb778f8b9521abb7fed4d518`（2026-11-04）；当前批次完成 DailyBar 复权/边界首轮代码审计，未修改 WebUI 业务代码。数据侧每完成一个开发节点、测试节点或真实数据验证节点，都应更新对应行；不需要等待 WebUI 联调完成才更新。

**填写规则：**
1. 只更新自己负责的 API 任务行，不覆盖 WebUI 接入状态。
2. “开发状态”反映数据侧实现进度；“跨端状态”反映双方联调结果，两者不能互相替代。
3. 未核实的内容填写“待核实”，未执行的测试填写“未执行”，未配置凭据填写“跳过/未配置”，不要推断为通过或失败。
4. 每次更新至少填写数据侧 HEAD；有代码变更时填写对应提交 SHA。测试结果和真实请求结果分别记录。
5. 已完成的开发项仍需保留测试证据；如果后续发现回归，应更新为“阻塞”或“开发中”，不要删除历史事实。
6. 数据侧总体阶段和任务优先级仍以 doc/api/数据侧整体规划.md 为准；本表只记录与 WebUI 相关的 API 交付进度。

**开发状态枚举：** 未开始、设计中、开发中、待测试、已完成、阻塞、暂缓。

**跨端状态枚举：** 待 WebUI 接入、待联调、联调中、联调通过、待真实数据验收、阻塞、暂不适用。

| 编号 | 数据侧开发状态 | 已完成内容 / 当前进度 | 未完成内容 / 下一步 | 阻塞与依赖 | 数据侧 HEAD / 提交 SHA | 自动化测试证据 | 真实数据验证 | 跨端状态 |
|---|---|---|---|---|---|---|---|---|
| API-01 | 开发中 | 当前 HEAD 已有 OpenAPI、HTTP 路由和 WebUI API 清单；已补齐 `SectorRanking.change_percent_5d`、`ProviderProbeItem.validation_*` Schema；`coverage_dimensions` 原已存在 | 继续完成所有端点的 OpenAPI Schema、Handler、JSON 字段逐项核对 | 剩余差异待逐接口复核 | `8decb2a8d796f54d6f1552928bb599889aa93c0a` | Go test suite Run `41755222811` 通过；Schema 契约测试检查字段归属 | 未执行全量真实接口对账 | 进行中 |
| API-02 | 已完成 | 指数目录、成分、指数日线、指数行情、交易日历、交易时段、证券交易状态、涨跌幅规则均有路由和 OpenAPI 定义；WebUI 已有对应请求封装 | 逐 Provider 补充真实数据覆盖和边界证据 | Provider 能力按源不同 | `05949306eba5c4cbb5b9c62bc2d656a1efc24db9` | Go test suite Run `41622693853`；WebUI checks Run `41622693832` 均通过 | 未完成逐 Provider 真实验收 | 待真实数据验收 |
| API-03 | 已完成 | `/api/v1/market/stocks` 提供分页行情与 MA 参数；批量 quote 最多 50 只；WebUI 已封装并用于当前分页选股及自选股批量刷新 | 补全分页总数/覆盖边界、均线复权语义及真实样本质量验证；明确当前选股仅作用于已加载分页 | 不具备全市场筛选策略执行 API | `05949306eba5c4cbb5b9c62bc2d656a1efc24db9` | Go test suite Run `41622693853`；WebUI checks Run `41622693832` 均通过 | 未执行全市场真实覆盖验收 | 待真实数据验收 |
| API-04 | 开发中 | 市场情绪快照/历史和市场级资金流历史接口及 WebUI 请求封装已存在 | 核实市场情绪历史缺失交易日、幂等写入、保留策略和真实数据库数据；补专门集成证据 | 需要 MariaDB 持久化与真实历史数据 | `05949306eba5c4cbb5b9c62bc2d656a1efc24db9` | Go test suite Run `41622693853` 的 MariaDB migration/PIT 集成通过；未证明 sentiment 专项全链路 | 未执行生产数据历史验收 | 待真实数据验收 |
| API-05 | 已完成 | `/api/v1/market/capital-flow` 已提供，使用 Tushare `moneyflow_mkt_dc`；WebUI 已封装 | 核对 Token 权限、字段单位、交易日覆盖、源端时间和历史数据准确性 | 依赖 Tushare 权限与凭据 | `05949306eba5c4cbb5b9c62bc2d656a1efc24db9` | Go test suite Run `41622693853` 通过 | 真实权限/历史值验收未完成；未配置凭据时应跳过 | 阻塞真实验收 |
| API-06 | 开发中 | Provider 静态清单、被动诊断、主动 Probe、Probe 历史均有 API；WebUI 已封装；validation/coverage Schema、DailyBar 多复权口径 Probe 契约及 Provider 专项测试已推进；首轮审计发现 Reporter 对重复/截断/部分过滤计数尚未完整结构化 | 补 Pytdx GetGbbq 请求失败确定性测试；完成 DailyBar 过滤/丢弃计数闭环；验证 MariaDB 探测历史持久化和逐 Provider 真实复权口径请求 | Provider 凭据影响真实探测；默认关闭 Provider 不生成逐项 Probe 结果；Pytdx 当前直接依赖具体 `*tdx.Client`，需要评估内部注入测试缝隙 | `ed18e0642f777646eb778f8b9521abb7fed4d518` | Go test suite Run `41758382946`、Parity Run `41758383176`、Live provider probe Run `41758383404`、WebUI checks Run `41758382953` 全部通过；Normalize/Validator 既有测试覆盖日期、重复、limit 和字段规则 | Provider × 复权口径真实请求未执行；Probe 历史持久化仍需部署验收 | 接口层已接入，待真实验收 |
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