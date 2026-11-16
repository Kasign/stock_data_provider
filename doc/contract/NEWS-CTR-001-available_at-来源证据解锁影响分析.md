# NEWS-CTR-001 `available_at` 来源证据解锁：契约变更影响分析（提案）

- 文档状态：**结构提案已获项目负责人确认，来源准入与公开契约决策仍待完成**。本次确认仅批准以两张追加式表作为后续结构设计基线，不等于 NEWS-CTR-001 2.0.0 已冻结，不授权 Migration、Provider 接入、PIT SQL 或 Go 业务代码实施。
- 提出日期：2026-11-14
- 触发：项目负责人授权 Integration「工作面 A」（《[Integration 当前任务交接与继续执行](../整体任务规划/05-治理与验收/Integration当前任务交接与继续执行.md)》第 10.4 节），要求以契约 Owner + News/Research Owner 身份推进 P0。
- 治理依据：`doc/contract/PLAT-002-项目级契约冻结登记簿.md:85-92` 第 5 节——「公开语义变更必须先完成影响分析、正式决策、消费者影响确认、版本升级和迁移边界定义」；交接文档第 10.5 节反空转规则第 2 条——授权 A 后先输出契约变更影响分析并取得确认，再动代码。
- 核对基线（2026-11-16 Review）：`integration` 代码验收 SHA `d90c6fc5f15c6ea076ce31bc02e866ef4fe5197f`；Integration CI Run `43788740211`（#73）对精确 SHA success。其后的文档同步提交不改变该代码验收证据。登记簿版本 `0.2.60-frozen`，43 项 CTR（37 frozen / 4 deferred / 2 not_applicable）。

---

## 0. 结论摘要（先看这里）

> **状态（2026-11-16）：`reopened / proposal-only`（重新启动，仅推进结构提案）。** 项目负责人已确认重新启动 P0，并同意以本提案第 1.9 节的“两表追加式模型”作为结构设计基线。历史 `shelved` 决议保留为过程记录；当前来源准入仍未通过，公开契约版本尚未升级。因此当前只推进结构定义、消费者影响和决策材料；不得提前实施 Migration、Provider、PIT SQL 或生成非空 `available_at`。

1. **A 的法定第一步已完成，结论是「冻结 Golden 指定的 `sec-edgar` 与仓库已接入的全部 Provider 均不合格」**：SEC 官方文档逐字声明 `There is no timestamp to indicate when filing content is first available on sec.gov.`；`Acceptance Time` 是「被 EDGAR 接受」的 EST 系统时间，到 sec.gov 可见还有 1-3 分钟且**不可预测**的 lag；PDS 规范里唯一的 `<TIMESTAMP>` 只用于「受理后更正」且由 R/A Subsystem 提供。详见第 1 节。
2. **2026-11-16 Review 更正**：第 1.5 节此前把时间精度和时区误当成语义证明。EDINET `submitDateTime` 目前只能列为提交时间候选，尚未证明为首次公众可获取时间；LSE RNS FIX `SendingTime` 是消息传输时间，不能直接等同公众可获取时间。两者均未通过准入 1 的语义证明，且属于海外市场，不符合当前中国 A 股项目范围。不能声称已找到合格来源。
3. **因此当前不得动代码**。`新闻模块开发任务与验收清单.md:799` 冻结了「当前来源没有合格字段，因此**不新增无实际来源支撑的字段/表，也不修改 PIT SQL**」；`新闻数据模型与质量契约.md:48` 规定「现阶段所有记录的 `available_at` 保持 NULL」。先加列、先建证据表、先改 PIT SQL 或先造 verified 值，都是契约明令禁止的「造壳」，不是解锁。
4. **挂起而非取消的理由**：当前 A 股范围内尚无通过全部准入条件的来源证据。海外 EDINET/RNS 候选既未证明时间语义，又超出当前项目范围；HKEX 时间精度不足；CNINFO 公开接口仅日期精度。若未来扩展海外市场或取得 A 股来源的官方字段规范与授权，必须重新按契约逐项审核。在此之前本提案保持挂起，不做版本升级、不做迁移、不动代码。

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

### 1.5 来源候选复核与适用范围（2026-11-16 Review 更正）

本节只登记候选和已知排除项，不把「字段有秒/毫秒精度」当作「首次向公众公开可获取时间」的语义证明。契约准入必须同时满足语义、授权、证据留痕、精确时间点、内容版本绑定等条件。

| 来源 | 已知字段 / 观察 | 精度 / 时区 | 当前适用性 | 当前判定 |
|---|---|---|---|---|
| 日本 EDINET | `submitDateTime`；字段名称指向提交时间，尚未取得足以证明其等于公众首次可获取时间的官方语义证据 | 公开样例呈秒级本地时间；须以当前官方 API 规范确认时区与解析约束 | 日本市场，超出当前中国 A 股范围 | `candidate`，准入 1 未通过，不得标记 `verified` |
| LSE RNS / MIT304 | FIX `SendingTime` 定义为消息传输时间（message transmitted），不是公众可获取时间的直接证明 | 规范格式含毫秒及 UTC | 英国市场，超出当前中国 A 股范围；另有市场数据授权限制 | `rejected_for_available_at`（现有证据不满足准入 1）；如未来取得不同的官方公开时间字段，再重新评估 |
| HKEXnews | 页面可见 `Release Time` 样例 | 分钟级 / HKT | 香港市场，超出当前中国 A 股范围 | 当前精度不满足契约准入 4；不作为候选通过项 |
| 巨潮 CNINFO 公开查询接口 | `announcementTime` 公开接口样例 4/4 条时间部分均为 `00:00:00` | 实际仅日期精度 | 中国 A 股范围内 | 当前公开接口不满足准入 4；付费服务字段字典未取得，保持 `not_comparable`，不得推定其提供秒级时间 |
| Bocha / Tavily / Brave / SearXNG | 当前已接入字段为搜索/聚合发布时间或页面年龄 | 不构成来源原始首次公开时间 | 当前 News 已接入来源 | 依据契约负面清单，不合格 |

**证据边界与修正：**

- SEC 官方 FAQ 明确说明 EDGAR Acceptance Time 是提交被 EDGAR 接受的时间，且 SEC 网站内容可见存在不保证、不可预测的延迟；该证据只支持否决 Acceptance Time 作为 `available_at`，不支持推断其他市场不存在合格来源。官方出处：[SEC Webmaster FAQ](https://www.sec.gov/about/webmaster-frequently-asked-questions)。
- EDINET 的 `submitDateTime` 不因秒级格式或「提交文件经 EDINET 公开」的概括性描述，就自动证明该字段记录的是公众可获取的准确时刻。必须取得现行官方 API 字段定义、发布时间流程及字段与公众页面实际可见时刻的对应证据。
- LSE FIX `SendingTime` 只说明消息发送时间。消息发送、接收、供应商分发与公众可获取可能是不同事件；没有官方规则明确证明其对应首次公开时刻，不得通过准入 1。
- HKEX 分钟级字段不满足当前契约要求的精确时间点；CNINFO 公开接口的日期级值也不满足。CNINFO 付费 API 未取得字段字典，属于 `not_comparable`，不是通过或失败的实测结论。
- 项目总览明确当前范围为中国 A 股、海外市场暂缓。因此 EDINET、LSE RNS、HKEX 即使未来通过各自市场的语义审核，也不能直接作为当前 A 股业务验收证据，除非项目负责人先批准范围变更。
- 本次未找到可在当前 A 股范围内标记为 `verified` 的 `available_at` 来源。P0 维持 `shelved`；不新增字段/表、不修改 PIT SQL、不生成或回填 `available_at`。

**本节结论：** 目前没有足够证据证明存在符合当前项目范围与 NEWS-CTR-001 全部准入条件的已授权来源。这里的结论是「当前证据不足」，不是「市场上绝对不存在合格来源」。

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
| **P3 维持现状** | 冻结契约保持不变，继续 `available_at = NULL`、B 组 `shelved` | 无需变更 | 否，但合规 |

**推荐 P1**：Research 侧 DTO/Schema/表/校验全部就绪，P1 只需 News 侧补来源证据与实现，改动面最小、语义风险最低。**P2 不能作为「绕过 P1」的捷径**：Research `mapNewsEvidence` 要求 `record.AvailableAt != nil`，新区间字段无法替代它。

**必须由项目负责人确认的决策点**：① 选择 P1/P2/P3；② 若 P1，指定来源与授权凭据；③ 是否同意按 MAJOR 升级版本（见第 4 节）；④ 消费者影响确认签字（见第 3.1 节）。未确认前不进入代码实施。

> **2026-11-14 决议：本项 `搁置`，P1/P2/P3 均未选定。**
>
> 依据第 1.5 节，P1 的真实前置不是「找到来源」，而是「为 News 新增法定披露类来源品类」这一产品定位决策；P2 按契约 `:47` 不能替代 `available_at`、且单靠新字段打不通 B 组；P3 无需变更即维持现状。三者都不构成 Integration 分支内可执行的开发动作。
>
> **重启条件（唯一）**：取得符合当前中国 A 股项目范围的官方来源字段规范、授权证明和首次公众可获取时间语义证据，并由项目负责人批准按冻结流程推进。若考虑海外来源，必须先批准项目范围扩展，再独立完成该市场的语义、授权和版本绑定审核；不得直接复用当前 A 股验收结论。
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

## 1.6 2026-11-16 A股交易所官方披露流程二次调查

本节为新增证据补充，不覆盖 1.5 节既有判定，不改变 NEWS-CTR-001 冻结语义，也不代表已取得数据服务授权。

### 官方流程证据

| 市场/来源 | 官方材料及可确认事实 | 对 available_at 的意义 | 当前判定 |
|---|---|---|---|
| 上交所 | 《科创板上市公司自律监管指南第1号——信息披露业务办理（2025年4月修订）》第六、十八条：非直通公告审查通过后由发行人确认发布；发行人选择在上交所网站披露的，公告在确认发布后直接上网。https://www.sse.com.cn/lawandrules/guide/stock/kcbxxpljg/c/c_20250425_10777769.shtml | “确认发布”是比提交/审核/公告日期更接近公众可获取边界的官方业务事件；但规则没有给出公众消费者可读取的逐公告、带时区精确时间字段，也未证明系统确认事件时间与公众页面实际可访问时间完全一致 | candidate；准入 1 有流程依据，准入 2/3/4/5 未满足 |
| 上交所直通车 | 上交所官方答记者问说明：直通车公告若 15:30 前确认发布，网站从当日 15:30 起集中披露；15:30 后确认发布则按上市公司提交时间披露。https://big5.sse.com.cn/site/cht/www.sse.com.cn/aboutus/mediacenter/hotandd/c/c_20150912_3988653.shtml | 说明确认动作与网站可见时间之间可能存在集中披露/调度规则；不能简单将确认动作时间当作实际公开时间 | candidate；必须进一步确认实际上网时间事件 |
| 北交所 | 《上市公司业务办理指南第7号——信息披露业务办理》规定最终由上市公司确认发布；其他媒体披露时间不得早于规定信息披露平台。https://www.bse.cn/cxjg_list/200025676.html | 存在法定/规则指定披露平台及确认发布流程，但尚无消费者可用的精确时间字段规范、访问授权与公告版本绑定证据 | candidate；需进一步取得平台机器记录 |
| 深交所 | 《深圳证券交易所数据接口规范（Ver4.72）》描述行情系统信息公告接口：结构头含发送日期、发送时间；SJSHQ.dbf 的 HQBSL4 记录当日最近一次收到信息公告的时间（HHMMSSss）。https://www.szse.cn/www/marketServices/technicalservice/history/P020180328468171229352.pdf | 这是行情/信息公告分发链路的发送或接收时间，不是明确的首次公众可获取时间；HQBSL4 还是当日最近一次接收时间，非逐条公告字段，不能作为该公告的 available_at | rejected_for_available_at（当前字段）；可继续询问深交所是否另有逐公告发布确认/网站上线日志 |

### 结论和后续核验

1. 之前将 A 股来源探索仅归结为 CNINFO announcementTime 日期级字段，覆盖面不足。上交所、北交所官方披露流程存在“确认发布”事件，值得作为新的正式候选调查方向。
2. “确认发布”是发行人业务系统中的动作，不自动等于“公众首次可获取”。上交所直通车还存在 15:30 集中披露规则；必须区分发行人确认时间、交易所处理/发布事件、网站实际可访问时间和第三方媒体接收时间。
3. 深交所 HQBSL4 当前定义为最近一次收到信息公告的时间，不能将其映射到单条公告，也不能将行情系统接收时间等同公众首次公开时间。旧版接口结构头中的发送时间同样需要证明发送对象及公众可获取边界。
4. 下一轮应取得交易所或其官方信息服务运营方的现行接口字段字典、样例原始响应、服务授权/合同范围，以及公告版本/更正版本标识；至少对上交所、深交所、北交所各选真实公告核对“确认发布—平台可访问—媒体接收”的时间顺序。无法取得这些材料时，记录为 not_comparable，不得用页面时间或抓取时间替代。
5. 本次没有获得授权凭据、稳定 Endpoint、字段级精确时间样例或公告版本绑定证据。因此 P0 仍为 shelved；不新增 V10、不改 Schema/Golden/PIT SQL、不回填、不启用 B 组成功路径。只有来源证据及项目负责人正式决策满足提案第 3～6 节后，才进入实施。

## 1.7 2026-11-16 上证信息官方公告文件 API 候选

本节记录本轮进一步发现的正式数据服务。它使来源路径从“只有披露流程规则”推进到“存在官方 API 产品”，但仍不能据此认定 API 的更新时间就是首次公众可获取时间。

| 项目 | 官方文档可确认事实 | 尚未确认 |
|---|---|---|
| 服务主体 | 上证所信息网络有限公司（SSE InfoNet）提供公告文件下载接口服务 | 具体合同主体、个人用户是否可申请、个人研究用途的授权边界 |
| 覆盖范围 | 产品说明书列明沪市主板、科创板、深市上市公司公告，以及上交所债券、基金和监管问询公告 | 北交所公告是否覆盖；具体历史覆盖、遗漏/更正公告范围 |
| API 形式 | 可按天查询公告元数据；字段包括索引、证券代码、标题、分类、更新时间、下载地址；支持 7×24 小时手动及轮询访问更新 | “更新时间”的正式定义、精度、时区、更新时间是否为首次上网/文件入库/供应商同步时间 |
| 身份与授权 | 产品流程需申请试用，审核后开通认证信息；正式使用需签约 | 试用授权能否持久化响应、可保存字段和期限、可否用于个人研究 |
| 费用 | 官网公开价目表列公告文件服务 6 万元人民币/年 | 当前报价、税费、是否有个人/低频研究套餐、试用额度及可单独购买范围 |

官方文档：
- 《上证金融数据公告文件服务说明书》：https://www.sseinfo.com/services/data/szjr/ggwjfw/c/10010710/files/5f5989f49fb142f380fb027bb9997179.pdf。第 2.1 节列明沪市主板/科创板和深市上市公司公告；第 2.3 节说明元数据含“更新时间”、索引、证券代码、标题、分类和下载地址；支持 7×24 小时轮询更新。
- 《上证信息金融数据服务产品价目表》：https://www.sseinfo.com/services/charge/pricelist/c/10010718/files/6e4e8d001bbd4554a254aeac11ee0e50.pdf，公告文件服务公开价 6 万元人民币/年。
- 官方产品入口：https://www.sseinfo.com/services/data/szjr/；申请指引：https://bsp.sseinfo.com/business/?id=1703679027550273537。

**判定：高优先级待核实候选，不是 verified。** 与此前 CNINFO 公开查询接口不同，该产品明确提供 API、公告元数据和更新时间，并覆盖沪深公告；但“更新时间”没有字段语义说明，不能仅凭名称映射到 available_at。且公开年费较高，不应在没有字段字典、试用样例和个人用途授权确认前购买。

**下一步只需向服务方提出一次结构化核实：** 索取《公告文件下载接口使用说明书》和《深市公告使用说明书》当前版本、API 原始 JSON 样例、更新时间字段定义（产生系统/触发事件/是否对应官网首次可访问）、时间精度及时区、公告更正版本处理、历史回溯能力、授权留存条款，以及试用和个人低频研究报价。官方页面公开咨询电话为 4008888400（按键 2-5），产品页面亦列业务咨询邮箱 ywsun@sse.com.cn。拿到材料后先做准入评审，不先接入生产。

补充：北交所公开的《交易支持平台数据接口规范（V2.1）》第十一章包含公告文件发送日期/时间及 HQBSL4 最近一次公告接收时间；这些仍是分发链路时间，不等同于公众首次可获取时间。规范样例同时表明公告文件可来自北交所，但接口对象为交易支持平台参与方，不能据此认定普通用户可访问或取得持久化授权。https://www.bse.cn/uploads/6/file/public/202209/20220902201701_kas8vw9r25.pdf。

因此 P0 继续保持 shelved，直到取得 API 字段定义、真实样例、版本绑定和授权证据，并完成项目负责人 DEC。不得将“更新时间”直接写入 available_at，不新增 V10、不改 Schema/Golden/PIT SQL、不回填、不启用 B 组成功路径。


## 1.8 后续实现必须具备的时间证据核验与诊断能力（2026-11-16）

项目负责人补充意见：由于 `available_at` 的来源字段存在较大语义疑虑，未来进入实现阶段时，除来源准入外，还应通过校验逻辑和日志保留可复核证据，便于持续核对真实数据。

该意见作为后续实现验收要求记录，**不改变当前 shelved 状态，也不授权提前新增字段、证据表或修改 PIT SQL**。只有取得合格来源并完成正式 DEC/版本升级后，才能按批准的数据模型实现。

### 必须保留的结构化证据

对每个公告内容版本，至少保留以下来源核验信息（最终字段名以获批 Schema 为准）：

- 来源身份：`source_id`、`endpoint_id`、授权策略版本、供应商/官方服务标识。
- 原始值：字段路径、原始时间文本、原始时区/偏移、解析后的 UTC 时间、解析器版本。
- 内容身份：公告唯一标识、内容版本/更正版本标识、公告详情 URL 或原文定位符。
- 采集证据：系统 `observed_at` / `retrieved_at`、证据引用/摘要、原始响应留存位置及其访问权限。
- 核验结果：稳定的校验结果、reason_code、核验规则版本、核验时间；不通过时保留原始值，不生成 `available_at`。

原始响应或日志不得包含 API Key、Authorization Header、Cookie 等凭据；日志只记录必要的引用、摘要和脱敏元数据。

### 必须执行的校验

1. **时间格式与时区**：拒绝只有日期、无时区本地时间、含义不明的数值时间戳和无法无歧义解析的值；保留原始文本与解析结果。
2. **采集先后关系**：来源声明的公开时间晚于本系统首次观察/获取时间时，判为证据冲突，不得生成 `available_at`。
3. **内容版本绑定**：公告 ID、版本 ID、原文 URL/摘要必须绑定到同一份证据；更正公告作为新版本追加，不得覆盖旧版本时间。
4. **跨来源冲突**：同一内容版本出现不同来源时间时，不自动取最早/最晚值；记录来源差异、时间差和各自证据，转为待核验状态。
5. **披露日期一致性**：校验时间点与来源披露日期、时区换算后的自然日关系；不一致时记录独立 reason_code，不以简单日期相等代替语义证明。
6. **重复与修订**：同一来源、公告版本和字段路径重复采集时做幂等比较；原始值变化必须形成追加式修订记录，包含旧证据引用、新证据引用和修订原因。
7. **PIT 边界**：只有来源准入状态为 verified 且证据完整、版本绑定通过的记录才允许提供 `available_at`；unknown / not_comparable / 冲突记录一律 fail-closed。

### 结构化日志/诊断建议

建议以单条公告版本为粒度输出结构化诊断事件，至少包含：

- `announcement_id`、`content_version_ref`、`source_id`、`endpoint_id`
- `source_time_raw`、`source_time_parsed_utc`、`observed_at`
- `field_path`、`parser_version`、`evidence_ref`
- `verification_status`、`reason_code`、`rule_version`
- `cross_source_delta_ms`（仅在存在可比较的多源时间时输出）

日志不得将采集时间写成来源公开时间，也不得仅输出一个最终时间而丢失原始字段与核验依据。诊断状态必须区分通过、降级/待核验、失败和不可比较；不得把缺少授权、字段语义不明或无真实样本的情况记成业务数据失败。

### 对验收的补充

未来来源接入至少要提供：原始响应脱敏样本、字段语义证明、时间解析测试、时区/跨日测试、未来时间拒绝、采集先后冲突拒绝、版本更正追加、跨来源冲突留痕、重复采集幂等，以及真实 MariaDB 持久化后按证据引用回读核对。单元测试或模拟响应不能代替真实来源样本验收。


## 1.9 `available_at` 证据与审核决策的两表结构提案（2026-11-16）

### 1.9.1 设计目标与边界

本节记录项目负责人已确认的结构设计方向。**它是 NEWS-CTR-001 公开契约变更的提案基线，不是已冻结 Schema 或数据库实现。**

设计目标：

- `available_at` 仍表示某一具体内容版本首次向约定市场参与者公开可获取的精确时间点；不改变 GOV-CTR-002 / NEWS-CTR-001 的既有语义。
- 将来源原始证据与系统审核决策分开保存，二者均采用追加式记录，不覆盖历史。
- 证据必须绑定精确 `version_id`，不能仅绑定 `news_id`。
- 不把可变 `available_at` 写入不可变的 `news_item_versions` 内容快照，也不把其纳入内容版本 fingerprint。
- 不额外维护可变的 current-state 表；当前有效状态由截至指定 `system_as_of` 的决策事件推导。
- 来源没有合格证据时，DTO 仍返回 `available_at: null`；不为了表达缺失而伪造 evidence/decision 行。

### 1.9.2 表一：`news_version_availability_evidence`

一行代表某个 News 内容版本从一个已授权来源响应中取得的一份原始时间证据。记录不可更新、不可物理覆盖；纠错或重新采集产生新 evidence 行。

| 字段 | 建议类型 | 约束与含义 |
|---|---|---|
| `evidence_id` | VARCHAR(128) | 主键，稳定唯一 ID |
| `version_id` | VARCHAR(128) | 必填；外键/逻辑引用 `news_item_versions.version_id` |
| `source_id` | VARCHAR(128) | 来源身份 |
| `endpoint_id` | VARCHAR(128) | 实际请求的 Endpoint |
| `source_policy_version` | VARCHAR(128) | 采集时使用的 Source 策略版本 |
| `provider_output_policy_version` | VARCHAR(128) NULL | Provider 输出许可策略版本；不适用时 NULL |
| `field_path` | VARCHAR(255) | 响应中的精确字段路径 |
| `raw_value` | TEXT | 原始时间值文本，不以解析结果替代 |
| `candidate_available_at_utc` | DATETIME(6) NULL | 解析候选值；仅当精确解析成功时填写，按 UTC 存储 |
| `source_timezone_or_offset` | VARCHAR(64) NULL | 原始时区/偏移文本，保留来源表达 |
| `parser_version` | VARCHAR(128) | 解析器版本 |
| `evidence_ref` | VARCHAR(512) NULL | 受控原始响应/证据定位符；不得包含凭据或敏感 query 参数 |
| `evidence_hash` | CHAR(64) NULL | 原始证据规范化摘要或内容哈希 |
| `observed_at` | DATETIME(6) | 系统首次取得该证据的 UTC 时间 |
| `recorded_at` | DATETIME(6) | 该证据写入系统的 UTC 时间 |

补充约束：

- MySQL `DATETIME(6)` 本身不携带时区；所有 UTC 列必须由应用层统一转换、校验并以 UTC 写入/读取，Schema 明确使用 RFC3339Nano UTC 表达。
- 原始响应留存必须服从 Source/Endpoint 授权和保留期限；表中只保存受控引用/摘要，不默认复制完整正文。
- `candidate_available_at_utc` 只是候选，不因非 NULL 自动获得可信资格。
- 原始值、字段路径、证据引用和版本绑定共同构成可复核证据；缺少任一必需项不能进入 verified。

### 1.9.3 表二：`news_version_availability_decisions`

一行代表针对一个内容版本及一份证据的审核决策事件。决策只追加，不更新旧行。

| 字段 | 建议类型 | 约束与含义 |
|---|---|---|
| `decision_id` | VARCHAR(128) | 主键 |
| `decision_group_id` | VARCHAR(128) | 同一次审核/冲突裁决的分组 ID；冲突可关联多份 evidence |
| `version_id` | VARCHAR(128) | 必填；与 evidence 的版本归属一致 |
| `evidence_id` | VARCHAR(128) NULL | 对应证据；无证据的撤销/状态事件可为空，但必须有 reason_code |
| `status` | VARCHAR(24) | `verified` / `conflict` / `rejected` / `revoked` |
| `reason_code` | VARCHAR(128) | 稳定原因码，禁止只依赖自由文本 |
| `review_rule_version` | VARCHAR(128) | 审核规则版本 |
| `supersedes_decision_id` | VARCHAR(128) NULL | 被本事件修订/替代的决策 |
| `recorded_at` | DATETIME(6) | 决策写入系统的 UTC 时间；PIT 认知边界 |

状态语义：

- `verified`：该 evidence 通过来源语义、授权、精度/时区、版本绑定和采集先后校验；一个决策组只能选定一份 evidence 作为当前可信候选。
- `conflict`：同一版本的多份证据发生无法按冻结规则消解的冲突；该版本在冲突解决前不得输出 available_at。
- `rejected`：证据不符合准入规则，保留记录用于审计，但不得参与 PIT。
- `revoked`：撤销此前的有效决策；必须通过追加事件指向被撤销决策，不删除历史。
- 无证据/未审核不强制写入 `unknown` 行；无可用 verified 决策即表现为 NULL。避免把“没有记录”与“经过审核确认 unknown”混为一谈。

### 1.9.4 决策一致性与 PIT 规则

对某个 `version_id` 和 `system_as_of`：

1. 只考虑 `recorded_at <= system_as_of` 的决策事件。
2. 只考虑 `observed_at <= system_as_of` 的证据事件。
3. 按 `decision_group_id` 解析截至该时点最新的决策状态；不得读取未来才记录的 verified、conflict 或 revoked 事件。
4. 只有截至该时点的有效决策为 `verified`，且唯一选定的 evidence 完整、版本匹配时，才允许返回候选时间。
5. 必须满足 `candidate_available_at_utc <= observed_at <= system_as_of`，并满足 `candidate_available_at_utc <= system_as_of`；违反时作为证据冲突拒绝。
6. 若截至该时点存在未解决的冲突或已生效撤销，必须 fail-closed，返回 NULL。
7. 同一版本存在多个可选 verified 决策时，不能按最早/最晚时间自动挑选；必须由审核决策显式给出唯一选择。
8. 所有决策的生效顺序、同一时间戳并列处理和唯一性约束必须在正式 Schema/SQL 设计中明确，不能依赖数据库返回顺序。

### 1.9.5 为什么不把字段直接加到 `news_item_versions`

`news_item_versions` 是内容快照，按版本指纹不可变；而来源证据可能在版本首次采集后才到达，审核结果也可能被后续追加修订。直接在版本表上更新 `available_at` 会破坏不可变快照和历史 PIT 认知；仅在版本表加 nullable 列也无法留存多来源冲突、证据原文和审核历史。

因此提案采用两张独立表：证据表回答“来源实际给出了什么”，决策表回答“系统在何时基于哪些证据认可了什么”。不另建可变状态表，避免重复维护 current pointer。

### 1.9.6 尚待正式冻结的细节

以下内容进入 Schema/DDL 设计前必须完成，不在本次确认中视为已决：

- ID 格式、字段长度、索引、外键策略、删除/保留策略及 Migration 回滚边界。
- `decision_group_id` 的分组规则、同组多证据冲突表达，以及撤销/替代决策的完整状态转移矩阵。
- 多个 verified 证据之间的权威选择规则；不能简单按时间排序。
- `DATETIME(6)` UTC 写入/读取约定、RFC3339Nano 输出、微秒精度损失/舍入政策。
- evidence_ref 的安全存储、脱敏、访问控制、原始响应保留期限及哈希算法/规范化方式。
- 是否需要额外的证据采集批次/Provider 请求关联 ID；如需，必须复用已有 ID，不重复发明。
- NEWS-CTR-001 Schema/Golden 版本升级方式、Research 消费者兼容策略及旧 1.0.0 保留规则。

### 1.9.7 当前实施门禁

项目负责人已确认本两表方案作为结构提案基线；但当前 A 股候选（包括 SSE InfoNet 的“更新时间”）仍未通过首次公开可获取时间语义证明，且授权/字段规范尚未取得。因此本次只更新提案和任务跟踪，不新增 Migration V10、不改 Go 领域模型/Repository/PIT SQL、不修改 1.0.0 Schema/Golden，不启用 B 组成功路径。正式实施仍需：来源准入证据 → 决策记录/消费者影响确认 → NEWS-CTR-001 版本升级与迁移批准 → 代码和真库验收。


## 1.10 两表结构的事件顺序、状态快照与时间精度细化（2026-11-16）

本节细化并修订 1.9。若与 1.9 中早期字段类型建议冲突，以本节为准；仍属于结构提案，不表示 Schema/DDL 已冻结。

### 1.10.1 决策表采用“一行一个完整状态事件”

每条决策记录都是某个 `version_id` 在该时点的完整可见性状态快照，不采用同一个 decision_group_id 多行拼接，也不把单条 evidence 的 rejected 结果直接当作版本整体状态。

建议字段：
- `decision_id`：决策事件 ID。
- `version_id`：精确内容版本。
- `decision_seq`：该 version_id 内严格递增；唯一约束 `UNIQUE(version_id, decision_seq)`。
- `status`：`verified`、`conflict`、`unresolved`、`revoked`。
- `selected_evidence_id`：仅 verified 时必填，且必须包含于 considered_evidence_ids。
- `considered_evidence_ids`：本次决策实际审查的 evidence ID 集合，以规范化、去重、稳定排序的 JSON 数组保存，精确还原冲突裁决范围。
- `reason_code`、`review_rule_version`：决策原因和规则版本。
- `supersedes_decision_id`：指向本版本前一条决策事件；首条为空。
- `recorded_at`：决策进入系统的 UTC 纳秒时间。

| 状态 | selected_evidence_id | considered_evidence_ids | PIT 结果 |
|---|---|---|---|
| verified | 必填且唯一 | 必须包含选中证据 | 可返回被选证据候选时间 |
| conflict | NULL | 至少两份冲突证据 | NULL，fail-closed |
| unresolved | NULL | 可为空或包含本轮审查证据 | NULL |
| revoked | NULL | 可包含被撤销决策所依据证据 | NULL |

`rejected` 不作为版本整体状态。单条证据解析失败或不符合来源准入条件，记录在 evidence 行的 `parse_status` / `parse_reason_code`。拒绝一条新候选证据，不自动撤销此前仍有效的 verified 决策；只有明确追加新的完整状态事件才改变版本状态。

### 1.10.2 证据行补充解析状态

`news_version_availability_evidence` 增加：
- `parse_status`：`parsed`、`unparseable`、`ambiguous`、`unsupported_precision`。
- `parse_reason_code`：稳定原因码；解析成功时可为空。
- `candidate_available_at_utc`：仅在 parse_status=parsed 且时间可无损表示时非 NULL。

解析成功只表示字段可无歧义转换为时间，不代表来源语义通过审核；仍须经过来源准入和决策审核。

### 1.10.3 决策序号与并发写入

- 同一 version_id 的决策写入必须在单个数据库事务内完成。
- 事务先对对应 `news_item_versions` 行执行 `SELECT ... FOR UPDATE`，再读取该版本最大 decision_seq 并分配下一序号；同版本决策写入串行化。
- 写入完整决策事件并提交；唯一索引 `UNIQUE(version_id, decision_seq)` 作为并发防线。
- decision_seq 是同版本决策顺序的权威依据；recorded_at 用于 system_as_of 可见性过滤，不以客户端时间或单独时间排序。
- 新决策必须引用前一条决策 ID；前序不匹配时拒绝写入，避免决策链分叉。
- 证据采集与审核分离：审核只能引用已持久化且属于同一 version_id 的证据。

### 1.10.4 system_as_of 的确定性还原

1. 证据行必须满足 observed_at <= system_as_of 且 recorded_at <= system_as_of。
2. 决策行必须满足 recorded_at <= system_as_of。
3. 在可见决策行中选择最大 decision_seq 的完整状态事件。
4. 最新状态只有 verified 才能返回 selected_evidence_id 指向的证据。
5. 被选证据必须属于相同 version_id、出现在该决策的 considered_evidence_ids 中且 parse_status=parsed。
6. 必须满足 candidate_available_at_utc <= observed_at <= system_as_of；候选时间晚于系统首次观察时间时判为冲突，不输出 available_at。
7. 最新状态为 conflict、unresolved 或 revoked 时返回 NULL；只有后续新追加的 verified 完整状态事件才能恢复非空结果。
8. 没有可见决策事件时返回 NULL，不能用未来 verified 决策修正过去的 system_as_of。

### 1.10.5 PIT 时间统一采用固定九位 UTC 字符串

对照 Research 的 `internal/research/storage/repository.go`：
- Research `formatPIT` 固定使用 `2006-01-02T15:04:05.000000000Z`，总长 30 字符、9 位小数、UTC。
- Research Migration 对 available_at、system_observed_at、system_as_of 使用 `CHAR(30) CHARACTER SET ascii COLLATE ascii_bin`，保证字典序与时间顺序一致。
- 因此 News 新表中参与 PIT/as-of 比较的 candidate_available_at_utc、observed_at、recorded_at 统一采用相同固定 30 字符格式，不用 DATETIME(6)。
- Go 层统一 UTC 格式化为固定九位小数；读取时严格解析并验证规范格式。
- 纳秒精度必须无损往返；来源精度超出系统表达范围或转换会舍入/截断时，标记 unsupported_precision，禁止静默 round/truncate。
- created_at 等不参与 PIT 的审计字段可用 DATETIME(6)，不得作为历史知识边界。
- 若现有 News Observation 时间来自 DATETIME(6)，格式化为九位时尾部补零仅表示数据库现有微秒精度，不得声称来源具备纳秒精度。

### 1.10.6 对 1.9 早期提案的修订

1.9 中建议使用 DATETIME(6) 的 candidate_available_at_utc、observed_at、recorded_at，现统一修订为固定 30 字符 UTC RFC3339Nano 字符串。1.9 中 decision_group_id 多行分组模型不再采用，改为一行一个完整状态事件、decision_seq 严格递增、considered_evidence_ids 规范化 JSON 数组。两表数量不变，不新增第三张关联表或可变状态表。

### 1.10.7 本轮评审结论与未完成门禁

- [x] 决策状态定义为完整状态快照，避免单条证据 rejected 误覆盖既有 verified。
- [x] 决策顺序使用每个 version_id 内单调递增 decision_seq；事务锁定版本行并以唯一索引兜底。
- [x] 冲突证据集合通过 considered_evidence_ids 规范化 JSON 数组保存，不增加第三张关联表。
- [x] PIT 时间格式与 Research 已有 CHAR(30) 固定九位 UTC 规则对齐。
- [x] 精度不支持时拒绝，不允许静默舍入或截断。
- [ ] 正式评审 JSON 字段的 MariaDB 类型/校验、索引组合、外键策略和删除/保留策略。
- [ ] 取得来源字段语义、真实样本和个人用途授权准入证据。
- [ ] 完成 NEWS-CTR-001 Schema 2.0.0、Research 消费者兼容和正式 DEC。
- [ ] 上述门禁完成前不实施 Migration、Go 业务代码、Provider、PIT SQL 或 1.0.0 Schema/Golden 修改。


## 1.11 与既有 Observation 的来源绑定及 MariaDB 约束评审（2026-11-16）

本节基于当前 News Migration V1/V5/V6/V8/V9 和 Repository 结构，对两表提案进一步去重。仍为设计提案，不实施 DDL。

### 1.11.1 Evidence 必须绑定实际 Observation

当前 `news_observations` 已保存：
- `observation_id`
- `source_id`、`provider_code`、`endpoint_id`
- `source_policy_version_id`、`endpoint_policy_version_id`
- `provider_output_policy_version_id`
- `observed_at`、`candidate_json`
- `retention_until`、`payload_purged_at`

因此 Evidence 表不重复保存 source_id、endpoint_id 和三类策略版本，改为增加必填 `observation_id` 外键。来源身份、Provider、Endpoint 和采集时使用的策略版本通过 observation_id 关联读取，避免两处副本不一致。

Evidence 表最小必要字段调整为：
- `evidence_id`、`version_id`、`observation_id`
- `field_path`、`raw_value`、`source_timezone_or_offset`
- `candidate_available_at_utc`、`parse_status`、`parse_reason_code`
- `parser_version`
- `evidence_ref`、`evidence_hash`（仅在有真实、可访问且授权允许留存的证据对象时填写）
- `observed_at`、`recorded_at`
- 受来源策略约束的 `retention_until`、`payload_purged_at`（如该来源输出必须按期清理）

`observation_id` 必须指向实际包含该字段值的请求/响应，不得简单复用版本首次创建时的 observation_id。写入前校验 observation 与 version 属于同一 news_id，且本次采集内容确实包含对应 version 和 field_path。Evidence 自己保留最小原始时间字段，避免 Observation 的 candidate_json 按既有 retention 规则清理后失去字段级复核材料。

### 1.11.2 不重复存储来源元数据，但必须验证策略快照

- `source_id`、`provider_code`、`endpoint_id`、Source/Endpoint/Provider Output Policy Version 均以 Observation 记录为权威。
- 生成 Evidence 时必须确认 Observation 的授权/留存策略允许保存该时间字段和证据摘要；不能因为 Evidence 是内部表就绕过 Provider Output Policy。
- `evidence_ref` 不得包含 API Key、Cookie、Authorization Header 或敏感 query 参数；只允许受控内部定位符。
- `evidence_hash` 若只是 raw_value 哈希，只能证明本地值未变，不能证明供应商原始响应真实性。只有实际捕获并计算的原始响应摘要才可标注为 response hash；不可混用两种语义。
- 当前 Observation 表未证明存在完整原始响应持久化能力；若来源准入要求原始响应级证据，而当前 Provider 链无法提供，必须记录为来源证据缺口，不得伪称 evidence_ref/hash 已闭环。

### 1.11.3 MariaDB 字段、外键和索引方向

当前 News Migration 已使用 MariaDB `JSON`、InnoDB 外键、ascii_bin ID 和复合索引，因此不需要引入新存储技术。

建议约束：
- Evidence 的 `version_id`、`observation_id` 使用与现有主表完全一致的 `VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin`，分别外键引用 `news_item_versions(version_id)` 与 `news_observations(observation_id)`。
- Decision 的 `version_id` 同样引用 `news_item_versions(version_id)`。
- Evidence 增加 `UNIQUE(version_id, evidence_id)`，Decision 的 `(version_id, selected_evidence_id)` 使用复合外键指向该唯一键，数据库层保证被选 Evidence 属于同一版本。
- Decision 的 `(version_id, supersedes_decision_id)` 使用复合自引用外键指向同一版本的前一决策；`UNIQUE(version_id, decision_seq)` 保证序号唯一，`UNIQUE(supersedes_decision_id)` 防止一个决策产生多个后继分叉（NULL 可重复）。
- Evidence 建议索引：`(version_id, observed_at, recorded_at)`、`(observation_id)`；Decision 建议索引：`(version_id, recorded_at, decision_seq)`、`(version_id, decision_seq)`。最终索引须以实际 PIT SQL 的 EXPLAIN 和真库数据量验证，不为猜测查询添加冗余索引。
- `considered_evidence_ids` 可沿用仓库已有 MariaDB JSON 列能力；数据库负责 JSON 文档有效性，数组形状、ID 去重/排序、同版本归属和 selected ID 必须包含于数组等规则由 Go Domain/Repository 校验。该字段用于审计，不作为 PIT 主过滤索引。
- 现有 `news_observations` 的 `candidate_json` 有清理生命周期。Evidence 原始字段的留存/清理必须遵循同一来源许可；若需定期清理，不能把“追加式”误解为违反法定/合同保留期限的永久保留。应通过受控字段脱敏/清理事件记录清理结果，具体实现和审计要求须在 DDL 冻结前确定。

### 1.11.4 当前可确认与仍待验证

- [x] 来源身份与策略版本复用 Observation，不再在 Evidence 表复制同一组元数据。
- [x] Evidence 绑定精确 observation_id + version_id，并要求两者属于同一新闻主体。
- [x] 明确区分 raw_value_hash 与原始响应 response_hash，避免错误声称证据真实性。
- [x] 确认当前 MariaDB 已使用 JSON、外键和复合索引，可沿用既有技术路径。
- [ ] 核实 Provider/Observation 是否能产生可复核的原始响应引用或响应摘要；不能时将其列为真实来源证据缺口。
- [ ] 完成 Evidence 原始字段与响应摘要的保留期限、清理审计和授权策略映射。
- [ ] 以目标 MariaDB 版本执行 JSON/复合外键 DDL 验证，并用实际 PIT 查询执行 EXPLAIN。
- [ ] 完成正式 Schema 2.0.0、消费者确认和 DEC 前不实施 Migration/业务代码。


## 1.12 当前 Provider 原始响应证据链核查（2026-11-16）

本节是对当前代码的只读核查结果，用于明确未来来源接入的真实实现缺口；不授权提前修改 Provider。

### 已确认事实

1. `internal/news/adapters/provider/http.go` 的 `config.do` 会读取受大小限制的成功响应原始字节并返回给具体 Provider Parser。
2. `internal/news/adapters/provider/providers.go` 中 Bocha、Tavily、Brave、SearXNG Parser 将响应映射为 `domain.Candidate` 后，原始响应字节不再向 Application/Repository 传递。
3. 当前 `domain.Candidate` 仅保留 `PublishedAtRaw` 这一发布时间原始文本，没有通用的 `field_path + raw_value + source_time_type` 证据集合，也没有 `available_at` 专用来源字段。
4. `news_observations.candidate_json` 保存的是规范化后的 Candidate，不是完整原始 Provider Response；`PurgeExpiredObservationPayloads` 到期后会将其置为 `JSON_OBJECT()` 并保留 tombstone。
5. RSS/Atom 的 `CollectSource` 读取响应并解析 Entry，再映射为 Candidate；当前 Observation 同样只保存 Candidate，不保存完整原始 XML/JSON 响应或响应摘要。

### 结论

- 当前 Provider/Feed 链路可以保留一部分发布时间原始文本，但**没有通用、可追溯的来源时间证据通道，也没有已确认的原始响应级 response_hash/evidence_ref 持久化链路**。
- 因此未来即使某个来源被准入，也不能仅靠现有 Candidate/Observation 结构就声称完成 available_at 证据留痕。
- 来源准入完成后，需在对应 Provider/Feed Adapter 的响应解析边界提取获批字段的原始值、字段路径和精确 Observation 关联，并通过受控端口交给 Evidence Repository；是否留存完整响应、仅留存字段级证据或响应摘要，必须先由 Provider Output Policy 和来源授权确定。
- 不应现在为所有 Provider 统一增加泛化 RawResponse 字段；只针对已通过准入的来源和字段实现最小证据提取，避免持久化无授权的整份供应商数据。
- 本项是来源准入后的实现依赖，不改变当前来源未合格的事实，也不解除 Migration/业务代码门禁。


## 1.13 Evidence 原始字段留存与到期清理边界（2026-11-16）

当前 News Observation 已有 `retention_until` / `payload_purged_at` 和到期清理机制。新增 Evidence 不得成为绕过 Provider Output Policy 的第二份长期副本。

### 1.13.1 留存约束

- Evidence 的 `observation_id` 是留存策略来源；Evidence 保存的 `raw_value`、`evidence_ref`、响应摘要及派生候选时间都必须受该 Observation 对应的 Provider Output Policy / Source Policy 约束。
- Evidence 的有效留存截止时间不得晚于来源授权允许的截止时间；如需独立字段，`retention_until` 必须由可信策略计算，不接受调用方任意延长。
- 追加式是指证据与决策历史不被覆盖，不代表可违反来源授权永久保留原始数据。依法/依约到期清理是受控生命周期操作。
- 到期清理时至少要清除策略要求清除的 raw_value、evidence_ref、响应摘要及其他来源派生载荷，并写入 `payload_purged_at`；保留最小审计元数据、证据 ID、版本/Observation 关联、清理时间和清理原因（前提是策略允许保留这些元数据）。
- 证据载荷清理与对应 Observation 的 `candidate_json` 清理应在同一数据库事务中完成，避免一个已清理、另一个仍保留造成策略不一致。
- 清理后 Evidence 行作为 tombstone 保留，不物理删除；哈希是否允许保留也由策略决定，不能默认哈希永远不属于受限派生数据。

### 1.13.2 PIT 输出门禁

- 若选中的 Evidence 已被清理，默认不再输出非空 `available_at`，除非来源/Provider Output Policy 明确允许在原始载荷到期后继续保留并使用该规范化时间。
- 允许保留派生时间时，必须有可审计的策略版本/授权依据；不能仅因为决策曾经 verified 就推断派生时间可永久保留。
- 如果策略要求派生时间同步到期，应追加 revoked/unresolved 决策事件或由 PIT 查询按可见的 retention 截止时间 fail-closed；不得物理删除历史决策。
- 历史 `system_as_of` 查询仍须遵守当时的策略可见性和当前依法/依约可保留范围；不能为了历史回放重新暴露已到期载荷。

### 1.13.3 待决

- [ ] 明确各 Source/Provider Output Policy 对原始时间字段、规范化 available_at、证据摘要和审计元数据的分别留存权限。
- [ ] 确认到期清理是 Evidence/Observation 同一事务，及其 tombstone 诊断 reason_code。
- [ ] 确认 payload purged 后 verified 决策是否可继续提供派生时间；默认否，除非策略明确允许。


## 1.14 MariaDB 最终约束评审与精度/授权边界复核（2026-11-16）

本节复核 1.10～1.13 的提案，区分已由仓库实现证实的约束、可提出的 DDL 方向，以及仍不能冻结的事项。没有执行目标 MariaDB 实例 DDL；仓库未提供可确认的目标 MariaDB 精确版本和本轮可用的 NEWS_MARIADB_TEST_DSN。

### 1.14.1 数据库兼容性证据

- News Migration 使用 InnoDB、JSON、复合索引、外键、`DATETIME(6)`；现有集成测试通过 `NEWS_MARIADB_TEST_DSN` 连接真实 MariaDB，并应用完整 Migration。
- `docker-compose.yml` 没有声明 MariaDB 服务或 MariaDB 镜像版本；数据库由外部环境提供。
- 因此只能确认当前代码依赖上述 MariaDB 能力，不能据此声称已在某个指定 MariaDB 版本验证新增 DDL。下一阶段必须从实际部署/CI DSN 对应数据库读取 `SELECT VERSION()`，并在相同版本跑 Migration 集成测试。
- 本轮没有可用目标 DB 实例，不执行伪造的语法验证，也不把静态字符串测试当成真实 MariaDB 兼容性验收。

### 1.14.2 两表关系与完整性约束

建议正式 DDL 至少考虑以下关系：

**Evidence**
- `PRIMARY KEY(evidence_id)`。
- `UNIQUE(version_id, evidence_id)`，供 Decision 复合外键引用，保证 selected evidence 属于同一 version。
- `FOREIGN KEY(version_id) REFERENCES news_item_versions(version_id)`。
- `FOREIGN KEY(observation_id) REFERENCES news_observations(observation_id)`。
- `KEY(version_id, observed_at, evidence_id)` 与 `KEY(observation_id)` 作为候选索引，最终需按 PIT 查询 EXPLAIN 决定是否保留。
- Evidence 还必须满足 observation 与 version 属于同一 news_id。现有主键只分别保证 version_id、observation_id 唯一，无法直接以单列外键表达“同一 news_id”。正式 DDL 可选择在父表增加 `UNIQUE(news_id, version_id)`、`UNIQUE(news_id, observation_id)`，Evidence 冗余保存 news_id 并用两组复合外键保证；或在 Repository 同一事务内校验并锁定两行。不得只依靠调用方传入的 ID 声称数据库已保证跨主体一致性。
- Evidence 必须关联实际包含该 field_path/raw_value 的 Observation；单纯同 news_id 仍不足以证明字段值来自该响应，须由 Adapter/Repository 输入证据和 observation 内容的一致性验证。

**Decision**
- `PRIMARY KEY(decision_id)`、`UNIQUE(version_id, decision_seq)`。
- `FOREIGN KEY(version_id) REFERENCES news_item_versions(version_id)`。
- `(version_id, selected_evidence_id)` 复合外键引用 Evidence 的 `UNIQUE(version_id, evidence_id)`；selected_evidence_id 为空时由状态校验规则允许。
- `(version_id, supersedes_decision_id)` 复合自引用，确保前序事件属于同一 version；`UNIQUE(supersedes_decision_id)` 防止决策链分叉。
- `KEY(version_id, recorded_at, decision_seq)` 是候选 PIT 索引；由于决策按序号串行化，读取逻辑可先按 system_as_of 筛选，再取最大可见 decision_seq。实际索引列顺序须以真实查询和 EXPLAIN 验证。
- JSON `considered_evidence_ids` 不建立主查询索引。MariaDB JSON 不能提供数组元素到 Evidence 的普通关系外键；Go 层必须验证 JSON 数组格式、去重/排序、每个 ID 存在、同版本、均为本次审查对象，并验证 verified 选择项在数组中。它是审计集合，不是数据库强引用关系。
- 状态组合约束（verified 必须且只能选一份 evidence、conflict 至少两份 evidence、revoked 不得选 evidence）须在 Domain/Repository 事务中强校验；若目标 MariaDB 版本的 CHECK 行为不作为唯一防线，不可依赖它替代应用校验。

### 1.14.3 决策并发与事件链写入

- 当前 `news_item_versions` 是不可变内容版本行，适合作为同 version 决策串行化锁。
- 事务锁定 version 行后读取最大 decision_seq 和最新 decision_id，检查调用方 supersedes_decision_id 等于当前链尾，分配下一序号并插入；唯一键和唯一后继约束兜底。
- 所有验证和插入必须在同一事务内。发生重复序号、前序不匹配或 selected evidence 归属不匹配时整个事务回滚。
- decision_seq 是逻辑事件顺序；recorded_at 是 system_as_of 可见时间。两者不能互相替代。若 recorded_at 因数据库精度归一化发生相等，仍按 decision_seq 确定同一版本事件先后。
- 由于决策事件写入后不得回填更早的 recorded_at，Repository 应使用数据库生成/校验的 UTC 当前时间，或明确拒绝早于链尾 recorded_at 的写入；不能接受调用方任意指定历史时间。

### 1.14.4 News 与 Research 的 PIT 精度差异必须解决，不能直接声称已统一

- Research `formatPIT` 使用固定九位小数 UTC 字符串；Research PIT 字段使用 `CHAR(30) ascii_bin`。
- News `news_observations.observed_at`、`retention_until`、`news_item_versions.observed_at`、当前 Evidence Snapshot 的 `system_as_of` 使用 `DATETIME(6)`。现有 Repository 明确按微秒精度比较 Snapshot 元数据。
- 因此 1.10 将 News 新证据时间全部改为 CHAR(30) 的提议，尚未证明能与现有 News PIT 读取、Observation 采集时间和 retention 边界无损兼容。固定九位字符串只是格式化，不会把已有微秒 Observation 变成纳秒精度。
- 本提案暂不冻结 News 新表的最终时间列类型。必须由 NEWS-CTR-001 / Research 消费契约明确统一精度、边界归一化规则和 API 表示：要么 News 整个 PIT 边界继续采用微秒并显式映射到 Research；要么在单独契约升级中统一迁移 News PIT 时间列及查询，不能只给新表局部升级到纳秒。
- 在上述决议前，候选时间的源原文必须保留足以复核精度的 raw_value；规范化值不能静默舍入/截断。对于超过目标合同精度的数据，按 unsupported_precision 处理。

### 1.14.5 留存期限从现有授权交集继承

当前 Ingest 由上游策略评估得出 ObservationPolicyTrace.Retention，并在 Observation 上写入 `retention_until = observed_at + retention`。授权规则分别限制：
- SourcePolicy.MaxRetention；
- EndpointPolicy.MaxRetention（若有覆盖）；
- ProviderOutputPolicy.MaxRequestRetention / MaxPersistenceRetention（按 StorageMode 区分）。

因此 Evidence 不得自行配置 retention。其到期时间必须不晚于 Observation.retention_until，也不得晚于任一适用授权的有效截止时间。当前 Observation schema 没有独立存储 Source/Endpoint/Provider 三个策略各自的 retention deadline，只有策略版本引用、评估时间和最终 retention_until；Evidence 只能继承已计算的最小有效期限，不能重新推导不存在的授权信息。

**还需确认：**
- available_at 的字段级原始证据属于哪个 ContentKind/StorageMode，不能默认 metadata 的许可自动覆盖新增的来源时间字段。
- Provider Output Policy 是否允许持久化 raw_value、规范化候选时间、hash/ref 各自要分别确认；允许处理请求不等于允许持久化。
- Source/Endpoint/Provider Policy 任一到期或撤销后，既有 Evidence 是否立即停止读取、仅停止新增采集，或按既有 retention 到期；当前规则没有明确该字段级差异。必须在来源准入与策略版本合同中明确。
- Observation 现有 retention 清理事务只更新 `news_observations.candidate_json` 和 `payload_purged_at`，没有 Evidence 表联动；未来若增加 Evidence，必须在同一清理事务锁定到期 Observation，并清理所有关联 Evidence 受限载荷后，再标记 Observation tombstone。任一 Evidence 清理失败应回滚整批。
- 哈希和 evidence_ref 也是可能受限的派生数据，不能默认永久保留；tombstone 仅保留策略明确允许的最小审计元数据。

### 1.14.6 本轮结论

- [x] 已确认现有 MariaDB 能力与测试入口，但没有目标数据库精确版本和本轮 DSN，不能声称新增 DDL 已兼容。
- [x] 明确 Evidence/Decision 复合外键、链尾唯一性和 JSON 应用层校验方向。
- [x] 明确现有主键不足以用单列 FK 保证 Observation 与 Version 同属一个 news_id。
- [x] 发现 News 当前 PIT 边界为微秒 DATETIME(6)，与 Research 纳秒 CHAR(30) 不一致；1.10 的“统一纳秒”结论降级为待契约决议。
- [x] 明确 Evidence retention 必须继承 Observation 最终授权交集，并纳入未来同事务清理。
- [ ] 取得目标 MariaDB 版本并运行真实 DDL/Repository 集成测试。
- [ ] 冻结 News 与 Research PIT 精度、边界和 API 表示契约。
- [ ] 明确 available_at 字段级 StorageMode/ContentKind 及策略撤销后的留存行为。
- [ ] 上述事项完成前不新增 Migration、Evidence 代码、Provider 采集、PIT SQL 或 Schema 2.0.0 正式版。

## 1.15 available_at 秒级业务精度与字段级授权提案（2026-11-16）

本节记录项目负责人对精度方向的明确意见：**available_at 业务精度采用秒，不要求微秒或纳秒**。本节是跨模块契约评审提案；在正式 DEC / NEWS-CTR-001 2.0.0 冻结前，不代表允许实施 Migration、Provider 或 PIT SQL。

### 1.15.1 秒级业务时间与系统审计时间分离

| 时间字段 | 语义 | 建议精度/存储 | 说明 |
|---|---|---|---|
| `available_at` | 内容版本首次向约定市场参与者公开可获取的业务时间 | UTC 秒级；News 候选列建议 `DATETIME(0)` | 不携带小数秒；API 使用 RFC 3339 秒级格式 |
| `candidate_available_at_utc` | Evidence 解析出的候选业务时间 | UTC 秒级；候选 DDL 可用 `DATETIME(0)` | 只有通过来源准入和审核后才可能被决策选中 |
| `observed_at` | 系统实际观察来源响应的时间 | 保留 News 现有 `DATETIME(6)` | 系统侧审计时间，不因 available_at 降精度 |
| `recorded_at` | Evidence / Decision 被系统记录、对 PIT 可见的时间 | 保留 News 现有 `DATETIME(6)` | 决策事件可在同一微秒内发生，使用 decision_seq 解决同版本事件逻辑顺序 |
| `system_as_of` | Researchflow/Research 的系统知识截止时间 | 保留各模块现有 PIT 表示与精度 | 不为单一业务字段而全局降低系统截止时间精度 |

- API 的 `available_at` 规范形式为 `YYYY-MM-DDTHH:MM:SSZ`，不输出小数秒；明确偏移的来源时间先转换为 UTC。
- Research 当前 PIT 存储使用固定 30 字符、九位小数 UTC 字符串。该格式可以将秒级 available_at 编码为 `...SS.000000000Z`，但尾部零只表示存储格式，不表示来源有纳秒精度；Research 的通用 PIT 存储格式不因此改造。
- News 当前 Observation、Version、Snapshot 的系统时间仍为 `DATETIME(6)`。新 Evidence/Decision 的系统观察/记录时间应沿用既有系统时间精度；不应把这些系统时间统一降为秒。
- PIT 比较继续分别使用 `available_at <= system_as_of` 与 `system_observed_at <= system_as_of`。秒级业务时间与更高精度系统 cutoff 比较是有效的，不要求两者存储精度相同。
- `decision_seq` 只解决同一版本决策事件的逻辑顺序；不能用它推断同一秒内两个外部内容版本的真实先后。秒级相同的 available_at 视为同一业务时间粒度，不制造亚秒顺序。

### 1.15.2 来源精度与归一化规则

- Evidence 必须保留原始时间文本、原始时区/偏移、字段路径和解析器版本；建议额外记录来源声明的时间精度分类（date / minute / second / subsecond），不得仅从数据库列类型反推来源精度。
- 只有来源字段语义明确、包含明确时区且能精确落在整秒边界的时间，才可生成秒级候选值。
- 仅日期、仅分钟、无时区本地时间继续不满足现行准入规则。
- 若来源给出非零小数秒，不能静默截断或四舍五入为秒；在秒级契约下先记为 `unsupported_precision`，除非正式契约另行批准有来源语义依据的转换规则。
- 原始字符串仍用于复核，不意味着系统必须把微秒/纳秒作为业务排序精度保存。

### 1.15.3 字段级授权模型缺口与最小提案

现有 `ContentKind` 只有 metadata/title/snippet/abstract/full_text/attachment；`StorageMode` 有 none/transient/persistent_metadata/persistent_excerpt/persistent_full_text。现有策略可判断内容类别和保存方式，但不能区分“普通新闻 metadata”与“新增来源时间证据”的授权，也不能分别表达原始证据与规范化派生时间的持久化权限。

建议在契约层评估两个独立内容类别（仅提案，不新增代码枚举）：

1. `source_time_evidence`：来源原始时间文本、字段路径、时区/偏移、来源证据引用及响应/字段摘要。该类别的持久化必须有 Source、Endpoint、Provider Output 三方明确许可；哈希和引用不得自动豁免留存限制。
2. `derived_availability_time`：由获准原始证据解析出的规范化秒级 UTC 候选时间，以及经审核可供 PIT 使用的派生值。它必须单独获得持久化许可，不能因为允许处理原始响应就推定允许长期保存派生值。

两类内容均应显式绑定 `PurposeInternalPersonalResearch` 与实际 `StorageMode`；默认不允许 public_display、redistribution 或 model_training。若具体来源授权不允许持久化，则最多按获准的 transient 处理，不得写入 Evidence 长期载荷。Evidence 的最小 tombstone 元数据也须确认是否允许保留，不能默认由上述两类许可覆盖。

### 1.15.4 策略到期、撤销与既有 Evidence 的默认行为提案

| 事件 | 新采集/新写入 | 既有 Evidence 读取与 PIT 输出 | 清理 |
|---|---|---|---|
| Source / Endpoint / Provider Policy 自然到期 | 立即拒绝新的采集和证据写入 | 默认 fail-closed；只有策略明确允许到期后按既有 retention 继续使用，才可继续读取 | 不晚于既有授权与 Observation.retention_until 中较早者 |
| 任一适用 Policy 被明确撤销 | 立即拒绝新的采集和证据写入 | 默认立即停止读取原始证据和派生 available_at，PIT 返回 NULL | 按撤销决定要求清除受限载荷；仅在允许时保留 tombstone |
| Policy 明确声明既有数据可按原期限 grandfather | 停止新采集（除非另有授权） | 仅对撤销/到期前已合法取得的数据，在原 retention 截止前按明确范围读取 | 到原授权截止或 Observation.retention_until，取较早者 |

- Source、Endpoint、Provider Output 三类策略是授权交集；任一适用策略拒绝或撤销，不能由其他策略的允许状态覆盖。
- 自然到期与明确撤销必须分开：自然到期是否 grandfather、撤销是否追溯影响既有数据，必须有来源授权文本/策略版本证据，不得从当前状态枚举自行推断。
- 当前 Observation 只保存策略版本引用、评估时间和最终 retention_until，没有分别保存三类策略的独立到期时刻及撤销影响范围；因此当前数据模型不足以可靠执行 grandfather 例外。未补齐明确授权证据前，执行上述 fail-closed 默认。
- 到期/撤销导致 Evidence 不可再使用时，应追加 unresolved/revoked 决策事件或由 PIT 查询按策略可见性 fail-closed；不删除历史 Decision。受限载荷清理必须与 Observation payload 清理同一事务。

### 1.15.5 本节结论与门禁

- [x] 项目负责人确认 available_at 采用秒级业务精度，不需要微秒/纳秒业务精度。
- [x] 区分 available_at 业务精度与 observed_at / recorded_at / system_as_of 系统审计精度；不做全局降精度。
- [x] 明确 Research 固定 9 位存储格式可以承载秒级值，但不代表来源纳秒精度。
- [x] 识别现有 ContentKind / StorageMode 无法单独授权来源时间证据和规范化派生时间。
- [x] 提出策略到期/撤销的 fail-closed 默认和 grandfather 例外边界。
- [ ] News / Research Owner 确认 API 秒级格式与 Research 存储映射，并补充 NEWS-CTR-001 / Research 消费契约决议。
- [ ] 契约 Owner 决定是否新增独立 ContentKind，以及 Provider Output Policy 的字段级持久化许可和留存期限。
- [ ] 取得来源授权文本，确认自然到期、明确撤销和既有数据 grandfather 规则。
- [ ] 上述正式契约与来源准入门禁完成前，不新增 Migration、Evidence 业务代码、Provider 采集、PIT SQL 或 Schema 2.0.0 正式版。

### 1.15.6 现有 Schema 与 Research Adapter 兼容性核查

- 当前 NEWS-CTR-001 1.0.0 Schema 的 `available_at` 引用通用 `nullableDateTime`，其正则允许 1～9 位小数秒；同一通用定义也被其他时间字段复用。
- 1.0.0 Schema / Golden 属于已发布契约，不得原地收紧。秒级约束应只在未来 2.0.0 Schema 中为 `available_at` 定义独立的 nullable-second-date-time 规则；不得修改通用定义，否则会意外限制 `published_at`、`system_observed_at`、`fetched_at`、`normalized_at` 等其他时间字段。
- 2.0.0 的 `available_at` 规则应要求 UTC `Z`、整秒格式且无小数部分；其他系统时间字段继续使用现有 date-time 精度规则。
- 当前 Research News Adapter 的 `mapNewsEvidence` 将 News `AvailableAt` 作为 Go `time.Time` 校验 UTC、非空和不晚于 `SystemAsOf`，尚未验证纳秒部分为零。正式接入秒级契约后，Adapter 必须拒绝非整秒值，不能依靠 Research 的 `formatPIT` 静默格式化。
- Research Repository 的 `formatPIT` 固定输出九位小数。整秒 `time.Time` 会被编码为 `...SS.000000000Z`，可继续存入现有 `CHAR(30)`，不需要为 available_at 单独迁移 Research 表。
- PIT 比较可直接比较秒级 `available_at` 与更高精度 `system_as_of`；两者语义和精度不同并不构成冲突。系统 cutoff 从 Research 纳秒格式传入 News、而 News Snapshot 现有数据库边界为微秒的转换规则，是独立的跨模块系统时间问题，不得通过降低 available_at 精度顺带处理。
- 本轮只核查 Schema 与 Adapter 现状，不修改 1.0.0 Schema/Golden、Research Adapter 或数据库代码。



## 1.16 available_at 字段级授权模型与策略生命周期评审（2026-11-16）

本节在 1.15 的提案基础上，对照当前 `internal/news/domain/policy.go` 的实际枚举与授权评估逻辑，明确最小扩展方向。仍为契约提案，不修改枚举、Policy、Migration 或读取代码。

### 1.16.1 ContentKind 独立，StorageMode 复用现有值

当前 ContentKind 只有 metadata/title/snippet/abstract/full_text/attachment；StorageMode 只有 none/transient/persistent_metadata/persistent_excerpt/persistent_full_text。当前 `contentCompatibleWithStorage` 对 persistent_metadata 只允许 metadata/title。因此只在 Policy 配置中写入两个新字符串，并不能真正获得可执行授权：Domain 兼容性校验也必须在未来契约实现时明确扩展。

建议只增加两个独立 ContentKind：
- `source_time_evidence`：供应商/发布来源提供的原始时间证据，包括原始值、字段路径、来源时区/偏移及获准保留的证据定位符/摘要。
- `derived_availability_time`：由获准来源证据解析并审核得到的 UTC 秒级候选值，以及被授权保留的派生结果。

不建议新增 StorageMode。继续复用：
- `none`：不保留；
- `transient`：仅请求/处理期间短暂存在，不允许跨请求缓存，除非现有 Provider Policy 明确允许；
- `persistent_metadata`：仅当 Source、Endpoint、Provider Output Policy 三方分别授权对应 ContentKind 和 StorageMode 时允许持久化。

两个 ContentKind 在未来 Domain 兼容矩阵中只能映射到 `none`、`transient` 或 `persistent_metadata`；不得映射到 excerpt/full_text。原始证据和派生时间必须分别授权、分别计算 retention，不能因为允许保存其中一个就推导允许保存另一个。当前代码尚不兼容这两个新 ContentKind，故在正式契约落地前不可写入 Policy 配置或声称授权已生效。

### 1.16.2 三方授权的判定规则

一次 available_at 证据处理至少需要分别评估：
1. Source Policy：来源是否允许该访问方式、个人研究用途、对应 ContentKind 和 StorageMode。
2. Endpoint Policy：该具体入口是否允许相同 ContentKind/StorageMode；Endpoint 只能收紧 Source 权限，不能自行扩大。
3. Provider Output Policy：若通过搜索/聚合 Provider 获取输出，还必须单独允许该 ContentKind、StorageMode 和请求处理/持久化期限。Provider 允许处理不等于原始发布来源允许采集；Source 允许也不等于 Provider 输出可持久化。

对 `source_time_evidence` 与 `derived_availability_time` 分别运行授权交集；分别使用各自获批的 StorageMode 和 retention 上限。任一适用策略缺失、状态不明、证据引用缺失、过期或不允许对应维度，均拒绝该类数据操作。不能从 metadata、title 或其他 ContentKind 的许可自动继承。

### 1.16.3 策略生命周期矩阵（当前实现能力与建议默认行为）

| 事件 | 新采集/新持久化 | 已有 Evidence 读取 | PIT available_at 输出 | 载荷清理 |
|---|---|---|---|---|
| 策略自然到期 | 立即拒绝 | 当前读取授权复核不通过时拒绝 | fail-closed，返回 NULL | 按既定 retention 到期清理；若授权要求立即清理则提前清理 |
| 明确撤销 | 立即拒绝 | 立即拒绝 | 立即 NULL | 按撤销条款立即清理或按指定期限清理；不能自行延长 |
| 新版本策略收紧 | 按新策略执行 | 使用当前策略指针和既有授权读取门禁重新评估 | 重新评估失败即 NULL | 按新策略/撤销条款处理 |
| 明确 grandfather | 仅在授权文本明确范围内继续 | 仅对明确列出的既有数据、用途和期限允许 | 仅在明确允许派生时间继续使用时输出 | 不晚于授权和 retention 的较早截止点 |
| 授权信息缺失/无法判定 | 拒绝 | 拒绝 | NULL | 按最严格适用要求处理；无法确定时不得扩大保留 |

说明：
- 当前 `policyTimeActive` 对 ExpiresAt 采用到期即不 active；当前状态 revoked/pending 也会拒绝授权。现有策略模型没有 grandfather 标记或既有数据继续使用范围字段。
- News 已有 NEWS-CTR-002 Evidence 授权读取能力，会在读取事务中复核当前策略指针与资源版本。available_at 若要对外输出，不得绕开该机制或另建一套较宽松的授权读取路径；但其两种新 ContentKind 尚未进入现有读取授权契约。
- `retention_until` 是允许保留的最晚时点，不是自动授予读取权的期限。策略到期/撤销可以早于 retention_until 终止读取；物理清理按授权文本和清理义务执行。
- Grandfather 不是默认推定，也不是“旧数据一律保留到 retention_until”。必须有可审计授权文本明确允许既有 Evidence / 派生时间在指定用途、范围和截止时间内继续读取。
- 由于 Observation 目前只保存策略版本引用和最终 retention_until，不能从它反推出 Source、Endpoint、Provider 各自独立的撤销/清理截止时间。未来若需执行分层 grandfather，必须在策略快照/授权记录中保存可验证的适用规则或引用；不得猜测。

### 1.16.4 结论与冻结边界

- [x] 确认两个新 ContentKind 是必要的最小区分；普通 metadata 不能代表来源时间证据或派生 available_at 的授权。
- [x] 确认暂不新增 StorageMode；复用 none/transient/persistent_metadata，并由 ContentKind 分别隔离权限。
- [x] 确认现有 Domain 兼容矩阵尚不接受两个新 ContentKind；未来实现必须同步更新兼容矩阵及策略测试。
- [x] 明确 Source、Endpoint、Provider Output Policy 三方独立授权和 retention 交集。
- [x] 明确到期/撤销默认停止读取和 PIT 输出；grandfather 必须有明确授权依据。
- [ ] 项目契约 Owner 批准两个 ContentKind 的正式枚举值、兼容矩阵和 API/策略配置版本。
- [ ] 确认 NEWS-CTR-002 对两种 ContentKind 的读取授权适用范围和撤销时的清理义务。
- [ ] 取得具体 A 股来源的授权文本，确认个人用途、字段级持久化和既有数据留存范围。
- [ ] 完成以上契约与来源准入前，不修改 Policy Go 枚举/评估器、Migration、Evidence Repository、Provider 或 PIT SQL。

## 2026-11-16 最小可用结构复核（优先于前述两表提案）

项目负责人要求避免把当前字段交付扩展为通用证据治理工程：先给出可落地的最小结构；真实数据源若暂时缺失，作为独立接入阻塞处理。本节调整此前两张追加式表、双 ContentKind 和完整决策事件模型的设计方向。此前内容保留为历史评审材料，不再作为 MVP 实施基线。

### 最小结构建议

复用现有 `news_item_versions` 与其 `observation_id` 外键，不新增 Evidence / Decision 两张表：

| 字段 | 落点 | 含义 |
|---|---|---|
| `available_at` | `news_item_versions` | 可验证的首次公开可获取时间；NULL 表示当前版本没有合格证据 |
| `available_at_raw` | `news_item_versions` | 来源字段的原始时间文本，保留原始偏移/时区表达 |
| `available_at_field_path` | `news_item_versions` | 来源响应中的字段路径，便于定位解析来源 |

来源身份、Endpoint、Provider 和策略版本已由该 Version 关联的 `news_observations` 保存，不重复复制。`observation_id` 已将版本绑定到产生该版本的观测。原始值、字段路径与规范化时间随不可变 Version 保存；不保存完整 Provider 原始响应，不增加通用 RawResponse 系统。

### 最小规则

1. `available_at` 仅在来源字段语义经过确认、值可解析且含明确时区、版本归属明确时写入；否则保持 NULL。
2. 业务精度按秒；数据库可沿用 News 现有 `DATETIME(6)`，写入前要求小数秒为零，不改变 `observed_at` / `system_as_of` 精度。
3. `available_at_raw` 保留来源原文，不从规范化值反推；`available_at_field_path` 必须是实际 Provider 响应字段路径。
4. 缺失、解析失败、时区不明或来源语义未确认时，不写入 `available_at`；不新增状态表。冲突时不选值，保持 NULL 并记录现有诊断/错误证据。
5. PIT 只消费非 NULL 且不晚于 `system_as_of` 的 `available_at`。Research 现有字段和 Adapter 已有 fail-closed 校验可复用。
6. 不新增 ContentKind，不实现 grandfather，不新增决策事件、冲突状态机或通用授权框架。字段持久化仍必须服从现有 Observation 所绑定的授权策略；具体来源授权不明确时，该来源不接入。

### 契约与实施边界

- News DTO 已包含 nullable `available_at`，Research 存储也已有 `available_at`；若保持现有字段语义和 JSON 表示，不因“有值”本身升级整个 NEWS-CTR-001 Schema 主版本。需在现有契约中补充字段来源/秒级精度约束及相应测试；只有确实改变冻结 Schema 结构或公共语义时才启动版本升级。
- 本提案是结构基线，不代表当前已找到合格 A 股来源，也不授权立即生成非空值。Migration / Provider / PIT SQL 实施仍需至少有一个真实来源字段通过语义、时区、版本绑定和个人用途授权核验。
- 当前已接入搜索 Provider 的 `published_date`、RSS/Atom 发布时间、抓取时间仍不合格；不得映射。
- 暂不执行代码变更。本轮只将后续实现范围收缩到上述最小结构；来源寻找与准入作为独立下一任务。

## 2026-11-16 A 股公开披露时间字段候选补充

本轮对 A 股官方公开材料继续检索，发现一个值得后续验证的候选规范：证监会发布的金融行业标准 **JR/T 0313—2024《证券发行人信息披露文件编码规则》**。标准附录 A 将 `DOC_PUBDATE` 定义为“发布日期”，将可选字段 `DOC_PUBTIME` 定义为“发布时间”，并要求符合 GB/T 7408—2005 第 5.4.1 条；同时定义 `DOC_VER`（文件版本）与 `DOC_NO`（当日发布序列号）。标准第 5 节要求披露场所持续更新基础元数据。官方原文：[JR/T 0313—2024（上交所官网 PDF）](https://www.sse.com.cn/lawandrules/regulations/csrcannoun/c/10762082/files/bd183a9f24934b87bd0862c310f8f4b1.pdf)。

**当前判定：candidate / not verified。** 该标准证明 A 股信息披露元数据体系存在“发布时间”字段规范，并且提供了版本元数据线索；但它没有在字段定义中明确写出“首次向约定市场参与者公开可获取的准确时刻”，也没有证明某个实际 API/Endpoint 对每条公告均返回该字段。因此不能直接将 `DOC_PUBTIME` 映射为 `available_at`，也不能据此实施非空写入。

下一步只需针对这一候选做定向核验：①上交所/深交所或上证信息实际产品是否提供 `DOC_PUBTIME`；②接口字段是否对应文件在披露场所正式上线/可获取时刻，而非发行人提交、预约披露或数据分发时间；③是否能将 `DOC_VER` 或其他稳定标识绑定到具体文件版本；④个人用途的接口/下载授权是否允许持久化该时间字段。若任何一项无法证实，保留候选状态，不扩展结构、不伪造 available_at。
