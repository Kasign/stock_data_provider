# NEWS-CTR-001 `available_at` 来源证据解锁：契约变更影响分析（提案）

- 文档状态：**提案，待项目负责人正式决策**。本文不是已批准的契约变更，未修改任何冻结契约、Schema、Golden 或 Go 代码。
- 提出日期：2026-11-14
- 触发：项目负责人授权 Integration「工作面 A」（《[Integration 当前任务交接与继续执行](../整体任务规划/05-治理与验收/Integration当前任务交接与继续执行.md)》第 10.4 节），要求以契约 Owner + News/Research Owner 身份推进 P0。
- 治理依据：`doc/contract/PLAT-002-项目级契约冻结登记簿.md:85-92` 第 5 节——「公开语义变更必须先完成影响分析、正式决策、消费者影响确认、版本升级和迁移边界定义」；交接文档第 10.5 节反空转规则第 2 条——授权 A 后先输出契约变更影响分析并取得确认，再动代码。
- 核对基线：`integration` HEAD `a7a2f3054`；最近代码验收 SHA `c25d881c0` / Integration CI Run `43574975820`（#70）success；登记簿版本 `0.2.60-frozen`，43 项 CTR（37 frozen / 4 deferred / 2 not_applicable）。

---

## 0. 结论摘要（先看这里）

> **状态（2026-11-14）：`搁置`（P0 挂起）。** 项目负责人决定：本项暂不推进，结论固化于本文，等待新的外部条件（采购/授权到合格的法定披露类来源）再重启。挂起不改变 `available_at` 语义，也不解除 `新闻模块开发任务与验收清单.md:799` 的「不得新增字段/表、不得改 PIT SQL」禁令。

1. **A 的法定第一步已完成，结论是「冻结 Golden 指定的 `sec-edgar` 与仓库已接入的全部 Provider 均不合格」**：SEC 官方文档逐字声明 `There is no timestamp to indicate when filing content is first available on sec.gov.`；`Acceptance Time` 是「被 EDGAR 接受」的 EST 系统时间，到 sec.gov 可见还有 1-3 分钟且**不可预测**的 lag；PDS 规范里唯一的 `<TIMESTAMP>` 只用于「受理后更正」且由 R/A Subsystem 提供。详见第 1 节。
2. **⚠️ 上一轮结论的更正**：第 1 节只能证明「`sec-edgar` 与现有搜索类 Provider 不合格」，**不能推出「不存在合格来源」**。2026-11-14 补充的全渠道复核（第 1.5 节）已找到形态合格的来源（日本 EDINET `submitDateTime` 秒级、JST、免费公开 API；LSE RNS `SendingTime` 毫秒级、UTC、法定披露渠道）。因此正确的表述是：**合格来源存在，但它属于「法定披露」内容品类，而本模块现有来源是「新闻报道/搜索聚合」品类——是品类不匹配，不是供给为零。**
3. **因此当前不得动代码**。`新闻模块开发任务与验收清单.md:799` 冻结了「当前来源没有合格字段，因此**不新增无实际来源支撑的字段/表，也不修改 PIT SQL**」；`新闻数据模型与质量契约.md:48` 规定「现阶段所有记录的 `available_at` 保持 NULL」。先加列、先建证据表、先改 PIT SQL 或先造 verified 值，都是契约明令禁止的「造壳」，不是解锁。
4. **挂起而非取消的理由**：要让 B 组真跑通，需要的不是「换一家数据商」，而是**为 News 模块新增一类法定披露来源**（EDINET / RNS / HKEX 形态）。这是 News 的产品定位与输入结构变更，超出「补一个字段」的范围，须由项目负责人决策。在该决策作出前，本提案保持挂起，不做版本升级、不做迁移、不动代码。

---

## 1. 来源语义审核（工作面 A 第 1 步的正式产物）

### 1.1 契约准入条款（逐条，均来自 `doc/news/新闻数据模型与质量契约.md:31-48`）

| 条款 | 行号 | 要求 |
|---|---|---|
| 定义 | `:33` | `available_at` 是某一具体内容版本**首次向约定的市场参与者公开可获取**的时间，不是发布者写入的发布时间，也不是本系统首次发现它的时间 |
| 准入 1 | `:37` | 来源文档或稳定字段规范**明确说明**该字段语义是「首次公开可获取/公开上线时间」，不能只写发布时间、创建时间、更新时间或索引时间 |
| 准入 2 | `:38` | 字段来自已通过 Source/Endpoint 授权的发布来源响应；记录来源 ID、Endpoint ID、策略版本和证据 URL |
| 准入 3 | `:39` | 保留字段路径、原始时间文本、解析结果、原始时区/偏移、解析器版本、证据采集时间，以及受控证据摘要/引用 |
| 准入 4 | `:40` | 时间必须可解析为精确时间点并包含明确时区；只有日期、无时区本地时间或含义不明的数值均不准入 |
| 准入 5 | `:41` | 证据必须能绑定到具体内容版本；来源字段冲突、版本归属不明、证据缺失或语义未经审核时状态为 `unknown`，不生成 `available_at` |
| 准入 6 | `:42` | **证据采集时间是系统侧证明材料，不是 `available_at`**；来源声明时间晚于证据采集时间时拒绝准入 |
| 负面清单 | `:44` | Provider `datePublished/published_date/page_age`、RSS `pubDate`、Atom `published/updated`、HTML `datePublished/dateModified/article:published_time`、HTTP `Date/Last-Modified`、搜索索引时间、系统 `fetched_at/normalized_at/observed_at` 一律不能单独填充 |
| 首次抓取 | `:46` | 只能证明「在抓取时已可获取」，不能证明首次可获取的准确时间；首期不回填，也不引入语义相同的伪精确值 |
| 追加式修订 | `:48` | 已验证证据不得被后续抓取静默覆盖；纠错需保留旧证据、修订原因和新证据引用；**当前没有符合上述条件的已接入来源，因此现阶段所有记录保持 NULL** |

### 1.2 外部官方证据（2026-11-14 联网核实）

来源 A：[SEC Webmaster FAQ – "How can I understand EDGAR Timestamps?"](https://www.sec.gov/about/webmaster-faq.htm)（页面标注 `Last Reviewed or Updated: Aug. 23, 2024`）

逐字引用：

> `The EDGAR Filer System assigns various timestamps to the filing when approved.`

> `- Acceptance Date: Date the submission was accepted by EDGAR. Format is YYYYMMDD`
> `- Acceptance Time: Time (EST) at which the submission was accepted by EDGAR. Format is HHMMSS`

> **`There is no timestamp to indicate when filing content is first available on sec.gov.`**

同一 FAQ 关于接受时间到公开可见之间的延迟：

> `Filings are often available on sec.gov within 1-3 minutes of the EDGAR system timestamp. The lag time can increase significantly with high server load. We don't guarantee and cannot predict this lag.`

来源 B：[EDGAR PDS Technical Specification (PDF, 83 页, March 2025)](https://www.sec.gov/info/edgar/specifications/pds_dissemination_spec.pdf)（FAQ 指向的权威定义文档，已下载并解析全文）

逐字引用 `<TIMESTAMP>` 数据元素定义：

> `DATA ELEMENT: TIMESTAMP TAG: <TIMESTAMP> DESCRIPTION: Edgar Dissemination date-time, used for Post Acceptance Corrections only. Defines the time which the correction is disseminated by EDGAR, supplied by the R/A Subsystem. Optional. LENGTH: 15`

### 1.3 逐条判定：`ACCEPTANCE-DATETIME` / PDS `<TIMESTAMP>`

| 准入条款 | 判定 | 依据 |
|---|---|---|
| 1 语义明确声明 | **不合格** | SEC 官方明确「不存在表示内容首次在 sec.gov 可获取的时间戳」；`Acceptance Time` 定义为「被 EDGAR 接受」的系统时间，PDS `<TIMESTAMP>` 定义为「更正被分发」的系统时间，均非「首次公开可获取/公开上线时间」 |
| 2 授权来源响应 | 部分 | `sec-edgar` 在冻结 Golden 中是授权策略标识，不是已接入的真实 Endpoint 响应；PDS 为订阅服务，接入需另行授权与凭据 |
| 3 证据留痕 | 当前不具备 | `internal/news/adapters/provider/http.go:52-113` 只读响应体、丢弃全部响应头；`providers.go` 只取属负面清单的 `published_date/page_age` |
| 4 精确时间点含时区 | **不合格** | `Acceptance Time` 为 `HHMMSS` + `EST`，且官方说明到 sec.gov 可见有 1-3 分钟、高负载显著增加且**不可预测**的延迟；用接受时间充当公开时间会把尚未公开的内容计入历史分析，正是严格 PIT 要防的错误 |
| 5 版本绑定 / unknown | 不合格 | 无字段语义审核结论、无 `verified` 状态、无来源证据载体 |
| 6 系统侧时间不得替代 | **不合格** | PDS `<TIMESTAMP>` 由 `R/A Subsystem` 提供，且「used for Post Acceptance Corrections only」；属系统侧分发证明材料，契约 `:42` 明确禁止 |
| 负面清单 | 不合格 | 仓库已接入的 Bocha/Tavily/Brave/SearXNG 字段全在 `:44` 负面清单内（`internal/news/adapters/provider/providers.go`） |

**审核结论（范围已修正）：`sec-edgar` 以及当前仓库已接入的全部 Provider（Bocha / Tavily / Brave / SearXNG）均不能作为 NEWS-CTR-001 `available_at` 的合格来源证据。**

**该结论的边界必须写清**：本节只完成「对已接入来源与 Golden 指定来源的否决」，**不构成「市场上不存在合格来源」的证明**。原稿曾据此推出「0 个合格来源」，属过度外推，已作废；全渠道复核见第 1.5 节。

### 1.4 契约已预留的合法出路（不能靠改 `available_at` 语义）

`新闻数据模型与质量契约.md:47` 明确：

> 若未来业务需要利用该类证据，只能**另行定义有明确区间语义的时间边界，不得复用 `available_at`**。

因此即便项目负责人决定放宽，也**不能**把「首次抓取时间/系统分发时间」塞进 `available_at`；只能新增一个带区间语义的独立字段并重新定义 PIT 边界。这属于 GOV-CTR-004 `:28` 的 MAJOR 语义变更，必须重走本文第 4 节版本升级流程。

### 1.5 全渠道复核（2026-11-14 补充）：合格来源存在，但属另一内容品类

第 1.3 节只否决了「已接入来源」，为排除「是不是根本没找过」的疑问，本节按**渠道品类**重扫一遍。这类字段的分布规律如下：

| 渠道品类 | 代表 | 字段规范可公开获取 | 语义是否可能合格 |
|---|---|---|---|
| ① 监管机构 API | 日本 EDINET、LSE RNS（MIT304）、SEC EDGAR API | **是，官方免费公开** | 可能合格 |
| ② 交易所披露平台 | HKEXnews、巨潮 CNINFO、TDnet | 部分（页面可见，无字段规范） | 视精度而定 |
| ③ 商业数据厂商 | Wind / Choice / Refinitiv / Bloomberg / RavenPack | **否（付费墙或 NDA 后的字段字典）** | 通常最好 |
| ④ 学术/爬虫数据集 | GDELT、Common Crawl | 是 | 不合格（系统侧时间，契约 `:44` 负面清单） |

逐项核查结果：

| 来源 | 字段与实测值 | 精度 / 时区 | 授权 | 判定 |
|---|---|---|---|---|
| **日本 EDINET**（金融庁法定开示） | `submitDateTime`，实例 `"2024-06-20 15:30:45"` | **秒级 / JST** | **免费公开 API + 公开规格** | **准入 1、2、4 可能全过**，形式最合格 |
| **LSE RNS**（FCA 批准 Primary Information Provider） | MIT304 规范 FIX 字段 `SendingTime`：`Time the message was transmitted specified in UTC and in the YYYYMMDD-HH:MM:SS.sss format` | **毫秒级 / UTC** | 市场数据牌照，无免费通道 | 语义与精度合格，**授权受限** |
| **HKEXnews**（港交所披露易） | `Release Time`，实例 `22/10/2026 18:27` | 分钟级 / HKT | 官方 RSS 不含上市公司公告，无机读授权 | 语义有规则背书（见下），**授权缺口** |
| **巨潮 CNINFO** | 公开查询接口 `announcementTime`，实测 4/4 条：`1792771200000 → 2026-10-24 00:00:00`、`1791820800000 → 2026-10-13 00:00:00`，毫秒位均为 0 | **仅日期** | 有正式 API（Access Key/Secret + token） | **契约准入 4 明确「只有日期…均不准入」→ 公开接口硬性不合格**；付费 API 字段字典无法验证 |

补充事实与依据：

- **EDINET 语义**：FSA 官方原文 `The corporate disclosure documents submitted by these entities are publicly available on the Internet through EDINET.` —— 提交即在 EDINET 公开，不存在 EDGAR 那种「接受 ≠ 公开」的两阶段。
- **HKEXnews 语义**：港交所规则官网 FAQ 原文 `Under the Listing Rules, the release of information by way of a formal announcement via the HKEx website is regarded as the formal channel of communication with the investing public.`
- **巨潮实测方法**：`POST http://www.cninfo.com.cn/new/hisAnnouncement/query`（HTTP 200，4 条平安银行公告）。测的是免费公开查询接口；**付费数据服务（`p_sysapi1075`）的字段字典在付费墙后，本轮无法验证其是否提供时分秒**，故巨潮只能记为「公开层已排除、付费层待验证」。
- **深交所自律监管指南第 1 号（2026 修订）** 说明精度丢失发生在字段层而非规则层：`交易日 6:00-7:30 完成公告提交，本所技术平台于当日 7:30 后自动发送…；7:30-8:00 提交，实时发送；…11:30 后…；15:30 后…；16:00 后…`。即每份公告本有明确披露时刻，是平台字段只给到日期。

**本节结论（也是本次挂起的核心理由）**：

1. 契约要求的语义**不是乌托邦**——法定披露系统确有「发布即公开」的秒级时间字段，且 EDINET 官方免费公开。
2. 但**现有搜索类 Provider 在结构上不可能提供该字段**：Tavily `published_date`、Brave `page_age`、SearXNG `published` 全部是索引/聚合时间，逐条命中契约 `:44` 负面清单。换任何一家搜索类 Provider 结果相同。
3. 因此 `available_at` 只对**法定披露类**内容成立，对**新闻报道类**内容不成立。本项目 News 模块当前是后者。要解锁就必须**新增一类法定披露来源**（改变 News 输入结构），属产品范围决策，不在 Integration 分支权限内。



### 2.1 权威契约 / 规则文件

| 文件 | 需改动的位置 | 性质 |
|---|---|---|
| `doc/contract/rules/GOV-CTR-002-公共时间与PIT规则.md` | `:23` 字段表、`:45-51` §4、`:62-67` §6、`:76` §7 | 公共语义**不改**（保持 1.0.0），只把「具体来源字段映射」落到 News 字段级契约 |
| `doc/news/新闻数据模型与质量契约.md` | `:35-48`（准入白名单、`:48` NULL 结论）、`:74/:76/:89`（列归属）、`:266`、`:292/:369`（指纹输入）、`:404-410`、`:158-160/:173`（迁移） | 字段级契约，需新增来源条目与证据载体 |
| `doc/news/新闻API契约.md` | `:197/:199-203`（「当前不推断/均为 null」）、`:153/:221`（严格 PIT 落地状态）、`:123-128/:179`（DTO/schema_version）、`:4` 状态行、`:107` 引用 | 字段级契约 |
| `doc/news/新闻模块开发任务与验收清单.md` | `:799-801`（不新增字段/表、待实施项）、`:446`（`[ ]` 与 `:792-799` 已冻结状态的文字不一致）、`:829-838/:851-856` | **按 `:5` 历史快照规则，不回写历史 `[x]`，新增解锁章节并更新 `:3` 当前状态** |
| `doc/news/新闻模块与公共平台接入矩阵.md` | `:364` 表格行、`:370-374` 验收口径、`:352` 已完成段 | 状态同步 |
| `doc/news/代码与文档Review报告.md` | `:336-344` §九 | 状态同步 |
| `doc/contract/PLAT-002-项目级契约冻结登记簿.md` | `:4` 版本、`:25/:45/:47/:53` §2 行、`:8` 统计 | 版本升级（第 4 节） |
| `doc/contract/契约版本与变更记录.md`、`doc/contract/README.md`、`doc/contract/decisions/项目级契约决策记录.md` | 新增变更记录与 DEC | 正式决策 |

### 2.2 Schema / Golden / 契约测试

- `doc/news/schema/NEWS-CTR-001-research-evidence-page-1.0.0.schema.json:71`（`available_at` required）、`:153-155`（nullableDateTime）、`:6/:59`（`additionalProperties: false`）→ 新增字段/改语义必须发布 **2.0.0**，旧 1.0.0 不可原地修改（GOV-CTR-004 `:49-52`）。
- `doc/news/golden/NEWS-CTR-001-research-evidence-page.golden.json:19` 当前 `"available_at": null` → 须新增 2.0.0 Golden，旧文件保留。
- Research 侧 `doc/整体任务规划/golden/RSC-CTR-001-evidence.golden.json:15/:45/:61` 与 `RSC-CTR-002-research-event.golden.json:15/:36` **早已是真实非 null 值**；`internal/research/storage/migrations.go:16` 已有 `available_at` 列与 PIT 索引。阻塞纯在 News 侧。
- 现有 NULL 断言测试：`internal/news/app/evidence_query_golden_test.go:50`、`internal/news/app/get_evidence_test.go:183`、`internal/server/bootstrap/research_news_mariadb_integration_test.go`（A 组条件分支、B 组 Skip、分页注释）、`internal/research/adapters/news/evidence_test.go:97-127`（fake 级 fail-closed）。
- PIT SQL 依赖测试集中在 `internal/news/adapters/storage/mysql_repository_integration_test.go`（`:864-899`、`:1255`、`:1470-1571`、`:1862-1893`）；一旦 WHERE 加 `available_at IS NOT NULL AND <= ?`，所有 fixture 若未播种 verified 证据将**整组回归红**，必须同步。
- `internal/news/adapters/storage/migrations_test.go:10/51/77/93/165` 硬编码 `len(migrations) != 9`，新增 V10 需改。

### 2.3 顺带发现的三处引用不一致（待决策后按登记簿 `:89` 同步，本提案不改冻结文件）

1. `GOV-CTR-002…md:5` 写 `状态：freeze-ready（待项目负责人确认冻结）`，而登记簿 `:25` 为 `frozen`、`:10` 声明登记簿是状态唯一权威。
2. `新闻API契约.md:107` 引用旧的 `doc/整体任务规划/跨模块契约目录与冻结状态.md`，与登记簿 `:10` 冲突。
3. `新闻模块开发任务与验收清单.md:446` 仍为 `[ ] 需要先冻结…`，而 `:792-799` 已记录 2026-11-10 冻结首期准入。

### 2.4 顺序硬约束

第 1 节结论 + `新闻模块开发任务与验收清单.md:799` + `新闻模块与公共平台接入矩阵.md:364` 共同决定：**「取得合格真实来源 + 冻结白名单」必须先于任何字段/表/PIT SQL/verified 实现**。交接文档第 10.2 节十步清单的第 4、6 步不得提前。

---

## 3. 二、正式决策（需项目负责人选定，登记簿第 5 节第 2 条第二项）

| 路径 | 内容 | 契约后果 | 能否产生业务进度 |
|---|---|---|---|
| **P1（推荐）提供合格真实来源** | 项目负责人指定一个真实数据源，其官方稳定字段规范明确声明「首次公开可获取时间」，并提供 Endpoint 授权/凭据与字段文档；由来源 Owner 完成语义审核、allowlist、证据留痕与版本绑定 | 冻结语义**不变**；新增具体来源条目 + L2 实现；NEWS-CTR-001 因公开字段/Schema 变化仍需 MAJOR 版本升级 | **能**，这是唯一能让 B 组真正跑通且不动 PIT 语义的路径 |
| **P2 新增带区间语义的时间边界** | 按契约 `:47` 另行定义「在抓取时已可获取」的区间边界字段（**不复用 `available_at`**），重新设计 PIT 边界 | 属 GOV-CTR-004 `:28` MAJOR 语义变更，需 DEC + 全消费者影响确认 + Schema/Golden 2.0.0；Research 侧 `available_at` 仍须非 nil，因此**单靠新字段并不能直接解锁 B 组** | 部分（研究性，不直接打通现有链路） |
| **P3 维持现状** | 冻结契约保持不变，继续 `available_at = NULL`、B 组 `blocked` | 无需变更 | 否，但合规 |

**推荐 P1**：Research 侧 DTO/Schema/表/校验全部就绪，P1 只需 News 侧补来源证据与实现，改动面最小、语义风险最低。**P2 不能作为「绕过 P1」的捷径**：Research `mapNewsEvidence` 要求 `record.AvailableAt != nil`，新区间字段无法替代它。

**必须由项目负责人确认的决策点**：① 选择 P1/P2/P3；② 若 P1，指定来源与授权凭据；③ 是否同意按 MAJOR 升级版本（见第 4 节）；④ 消费者影响确认签字（见第 3.1 节）。未确认前不进入代码实施。

> **2026-11-14 决议：本项 `搁置`，P1/P2/P3 均未选定。**
>
> 依据第 1.5 节，P1 的真实前置不是「找到来源」，而是「为 News 新增法定披露类来源品类」这一产品定位决策；P2 按契约 `:47` 不能替代 `available_at`、且单靠新字段打不通 B 组；P3 无需变更即维持现状。三者都不构成 Integration 分支内可执行的开发动作。
>
> **重启条件（唯一）**：项目负责人决定为 News 引入法定披露类来源（EDINET / RNS / HKEX 形态任一），并落实其授权（凭据或牌照）、字段规范文档与覆盖市场。达到后按第 6 节实施计划从第 1 步重走，届时本文 §1.5 的候选评估可直接复用。
>
> **搁置期间禁止**：不新增 `available_at` 列或证据表、不改 PIT SQL、不生成或回填 `available_at`、不启用 B 组用例、不为消除 `--- SKIP` 而改断言。

### 3.1 三、消费者影响确认（登记簿第 5 节第 2 条第三项）

| 消费者 | 结论 | 依据 |
|---|---|---|
| Research（`internal/research/`） | 领域代码**不必改**（`available_at` 列、Schema、校验、Importer/Authorizer 均已就绪）；须签字确认，并可选统一 `isUTC`（`adapters/news/evidence.go:242-245`）与 News 侧 `validEvidenceUTC` 的零偏移判定 | `validate.go:139-144`、`:151-172`；`storage/evidence_repository.go:51-59` |
| Server（`internal/server/bootstrap/`） | 组合根**零改动**（V10 随 `Migrations()` 自动纳入）；测试与 CI 必改 | `bootstrap.go:429`、`research_news.go` |
| Researchflow / Quant / Notification / WebUI / Market Data / 旧 analysis | **不受影响**（已逐目录核对；WebUI 与 Market Data 的 `available_at` 属 DATA-CTR-002 独立语义，严禁混淆） | 登记簿 `:38` |
| `content_fingerprint` / 幂等 | News `BuildVersionFingerprint`（`domain/version.go:15`，`news-version-v2`）**不得纳入新字段**；Research `ComputeContentFingerprint`（`validate.go:151-172`）已含 `available_at`，新记录指纹变化属预期；**不得重算/改写已落库行** | 数据模型契约 `:292/:369`、GOV-CTR-003 `:59-60`、GOV-CTR-004 `:52-53` |

---

## 4. 四、版本升级（登记簿第 5 节第 2 条第四项）

- GOV-CTR-002 公共规则 **1.0.0 不变**（语义未改，登记簿 `:19/:92`）。
- NEWS-CTR-001 `1.0.0（4 Schema / 3 Golden）`（登记簿 `:45`）→ 发布 **2.0.0**，旧版保留不原地修改；依据 GOV-CTR-004 `:28`（改变 PIT 语义属 MAJOR）、`:31`、`:49-51`，决策记录 `:336`。同步 NEWS-CTR-003（`:47`）、GOV-CTR-002 行（`:25`）、RSC-CTR-001 行（`:53`）的后续关注点列。
- 登记簿版本 `0.2.60-frozen` → 递增（建议 `0.2.61-frozen`），`契约版本与变更记录.md` 新增条目（范式见 `:28-32` 的 QNT-CTR-001 1.0.0→2.0.0），`README.md:4/:6` 同步。
- 统计数字：按「现有 CTR 版本升级」路径 **43 / 37 / 4 / 2 全部不变**；仅当项目契约 Owner 决定把「来源证据类型白名单」登记为独立新 CTR 时才改为 44 / 38（需正式决策，不能默认）。
- 不是 deferred 复活：登记簿 `:71-78` 的 4 项 deferred 中无 `available_at` 专用条目。

---

## 5. 五、迁移边界（登记簿第 5 节第 2 条第五项）

- 只能新增 **V10 前向 Migration**（PLAT-CTR-006 `:104` 禁止修改已发布的 V1-V9）；当前最高 `Version: 9`（`internal/news/adapters/storage/migrations.go:244`）。
- 列归属：`news_item_versions` 加 `available_at DATETIME(6) NULL` + **追加式证据载体表**（满足 `:48` 修订留痕）；`news_items` / `news_authorization_snapshots` 不改（数据模型契约 `:74-76/:89`）。
- **既有行一律保持 NULL，不回填**（范式 `migrations.go:235-240` V8；任务清单 `:275/:788`）。解锁后历史 NULL 行被严格 PIT 排除是预期 fail-closed，不是缺陷；重新获得 verified 必须走追加式证据，不得 UPDATE 旧行。
- DDL 隐式提交，不得假设整体事务回滚（PLAT-CTR-006 `:122/:143-147`）。

**不得做什么（逐条依据）**：① 从 `published_at/observed_at/fetched_at/normalized_at`/首次抓取时间推导或回填（GOV-CTR-002 `:23/:45/:51`、数据模型契约 `:44/:46/:48`）；② 修改 V1-V9（PLAT-CTR-006 `:104`）；③ 静默覆盖已验证证据（数据模型契约 `:48`、API 契约 `:203`）；④ 原地修改/覆盖已发布 Schema/Golden 或重算历史指纹（GOV-CTR-004 `:49-53`）；⑤ 把新字段纳入 `news-version-v2` 指纹（数据模型契约 `:292/:369`）；⑥ 把 `available_at` 写入 `news_items` 或跨表冗余（`:74-76/:89`）；⑦ 用负面清单字段映射（`:44`）；⑧ 证据不完整/时区不明/版本绑定不明时标 `verified`（`:41-42`）；⑨ 跳过本文第 3 节正式决策直接动代码（登记簿 `:88`）。

---

## 6. 实施计划与验收标准（决策批准后执行）

按交接文档第 10.2 节十步清单，**第 1 步当前判定为不合格、第 2～10 步全部被阻断**。批准 P1 后的执行顺序：

1. 来源 Owner 完成字段语义审核、Source/Endpoint allowlist、证据留痕规范与版本绑定 → 形成 DEC。
2. 按第 3/4/5 节完成正式决策、消费者确认、版本升级与迁移边界；发布 NEWS-CTR-001 2.0.0 Schema/Golden（保留 1.0.0）。
3. V10 Migration + 追加式证据表 + `news_item_versions.available_at`；`migrations_test.go` 改 `len=10` 并补 V9→V10 既有数据保留用例。
4. Provider 保留受控响应证据；负面清单字段仍只进 `published_at`。
5. `Get/List` 严格 PIT SQL（`available_at IS NOT NULL AND <= system_as_of`）+ 事务内回读门禁；`researchEvidenceRecordFromPIT` 仅在 verified 且证据完整时赋值。
6. 同步全部 PIT fixture、4 组 NULL 断言测试、Golden Contract Test；新增 GOV-CTR-002 `:76` 四类边界（缺失/晚于/等于/早于 cutoff）的 MariaDB 用例。
7. Bootstrap：去掉 B 组 `t.Skip` 让成功路径真跑；**新增独立真库用例**覆盖「缺证据仍 `ErrEvidenceNotPITSafe` + `research_evidence_records` 零落库」（review #4 遗留缺口）；补 `ListEvidence`/`GetEvidence` 一致性断言；CI `-v` 门禁改为要求 B 组 PASS（SKIP 即失败）。
8. 按功能聚合提交，取精确 SHA 的 Integration CI / News MariaDB Job success，再同步交接文档与模块 README。

---

## 7. 附：证据与引用

| 证据 | 出处 |
|---|---|
| SEC 官方声明「无首次公开可获取时间戳」 | https://www.sec.gov/about/webmaster-faq.htm （`How can I understand EDGAR Timestamps?`，Last Reviewed or Updated: Aug. 23, 2024） |
| PDS `<TIMESTAMP>` 定义（仅用于 Post Acceptance Corrections 的系统分发时间） | https://www.sec.gov/info/edgar/specifications/pds_dissemination_spec.pdf （DRAFT EDGAR PDS Technical Specification, March 2025, 83 页；本机已下载并解析全文） |
| 契约准入与负面清单 | `doc/news/新闻数据模型与质量契约.md:31-48` |
| 冻结顺序约束 | `doc/news/新闻模块开发任务与验收清单.md:792-801`、`:446`；`doc/news/新闻模块与公共平台接入矩阵.md:364` |
| 冻结变更流程 | `doc/contract/PLAT-002-项目级契约冻结登记簿.md:85-92` |
| 版本升级规则 | `doc/contract/rules/GOV-CTR-004-Schema版本兼容与废弃规则.md:26-31/:47-53`；`doc/contract/契约版本与变更记录.md:13-16/:28-32` |
| 十步解锁清单与 review 结论 | `doc/整体任务规划/05-治理与验收/Integration当前任务交接与继续执行.md` 第 10.2 / 10.6 节 |

**本文只做只读调研与影响分析，未修改任何冻结契约、Schema、Golden 或 Go 代码。**
