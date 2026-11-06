# GOV-004 最终 Review 与契约冻结门禁

- 所属治理任务：GOV-004
- Review 日期：2026-11-05
- 规划分支：`feat/design_doc`
- 结论：**未通过冻结门禁（NO-GO）**。
- 本文是当前审查状态与剩余门禁的唯一结论，不代表 Data、News、Platform 已完成实现验收。

## 1. Review 范围

GOV-004 负责冻结五模块之间的公开契约、Schema、时间/质量语义、认证边界、Repository/Migration 所有权和依赖方向。

本轮已完成规划侧的补充审查：
- Data：OpenAPI 与 Go JSON 字段/序列化语义的差异清单、双向契约测试要求；
- News：Research 只读证据查询 Facade、授权决策引用、PIT 和稳定分页目标契约；
- Platform：五模块可依赖包、禁止依赖和真实集成验收清单；
- Data 服务间认证：Data `X-API-Key` 与 Platform Service Secret 的边界；
- 五模块的 Repository/Migration 归属和 Go import 方向。

## 2. 已可作为稳定设计基线的内容

以下是架构和语义层面已形成一致结论的内容；可继续作为模块设计约束，但不等于所有字段级 DTO 已冻结。

### 2.1 模块职责与依赖方向

- Quant：确定性数值计算、指标/因子/趋势/市场/行业/个股、策略/回测/风险/评估。
- Research：Context、Evidence、Event、Claim/Hypothesis、Scenario。
- Researchflow：Run/Stage/Attempt、DAG、Workflow Template、编排、恢复和结果综合；唯一负责组合模块。
- LLM：模型协议和 Adapter，不执行工具、不负责研究编排和领域持久化。
- Notification：订阅、模板、渠道、Dispatcher、幂等和投递历史，不负责告警条件计算。
- Quant 与 Research 不互相依赖；Research 不 import Quant。
- LLM、Notification 不依赖 Quant/Research/Researchflow。
- Researchflow 组合各模块 Application Facade。
- Platform 不反向依赖任何业务模块。
- Data 独立 Go Module 通过 HTTP/OpenAPI，禁止跨 Module import internal。

### 2.2 时间、质量与数据语义

- 统一区分来源/事件时间、available_at、observed_at、fetched_at、normalized_at、system_as_of。
- 历史 Run 固定 system_as_of；先按系统知识时间过滤，再按业务有效时间解析。
- 价格 CNY/股、成交量 shares、成交额 CNY；百分比按 Data 约定使用百分点。
- 复权、来源、版本、质量状态、fallback、单位和时间元信息必须保留。
- unknown/null 不得被转换为 0/false/空字符串/当前时间。
- HTTP 200 不代表数据 valid；Data Probe/Validation/Freshness/Coverage 的权威所有权仍归 Data。
- Risk unknown 不得当作 pass；硬风险门禁不能被 LLM 或评分覆盖。

### 2.3 持久化与基础设施所有权

- 每个领域模块自有 Repository、SQL、表结构、Migration、保留策略和领域历史。
- Platform 只提供 SQL/Redis 生命周期、Ping、Migration Runner 等技术原语。
- MariaDB 是事实存储；Redis 只作缓存/短期辅助。
- 模块分支不提前修改根 DI、HTTP 路由、cmd、Migration 总注册入口。
- Platform Task Queue 不作为 Researchflow DAG/Run/Attempt 引擎。

## 3. 阻断冻结的事项

### 3.1 Data OpenAPI ↔ Go JSON

<badge color="danger">阻断项</badge>

已知差异已整理至 [Data OpenAPI 与 Go 模型契约修正清单](./Data-OpenAPI与Go模型契约修正清单.md)。

未通过原因：
- 15 个 Go JSON 字段缺口尚未由 Data 分支修正并验证；
- required 差异字段清单与摘要数量需要自动化重算；
- nullable 差异的旧摘要数量与逐字段列表不一致；
- 9 个 OpenAPI 多余字段需要证明实际 Handler 输出来源或删除；
- Go JSON Tag ↔ OpenAPI 双向自动化契约测试尚未提供通过证据；
- 枚举、类型、精度、格式、范围、嵌套引用仍需全量复核。

通过条件：
1. Data 分支从最新 Go Model/Handler/OpenAPI 自动生成双向差异报告；
2. 所有差异修复或登记为有明确理由的例外；
3. Handler JSON golden、required/nullable、枚举和嵌套 Schema 测试通过；
4. OpenAPI lint、Go 测试、契约测试和 CI 全部通过；
5. 提交 SHA 和 CI 结果可精确关联。

### 3.2 News ResearchEvidenceQuery 与持久化/PIT

<badge color="danger">阻断项</badge>

目标契约已整理至 [News → Research 只读证据查询契约](./News-Research只读证据查询契约.md)。

未通过原因：
- 当前没有 GetEvidence/ListEvidence 只读 Facade；
- News Repository/Migration 尚未形成完整持久化闭环；
- Observation、Version、SecurityIdentity、IndustryClassification、Relation 缺完整 system_as_of 查询过滤；
- 当前 PolicyDecision 不包含不可变策略版本/授权快照引用；
- available_at/fetched_at/normalized_at 尚无完整、可验证的模型来源；
- 稳定分页快照与游标一致性未实现；
- 内容引用与按用途/内容级别的授权读取未实现。

通过条件：
1. News 实现公开 Application DTO/Facade，Research 不访问 News 私有包；
2. Repository/Migration 和真实 MariaDB 集成通过；
3. 双时间轴过滤、历史关系解析和版本选择测试通过；
4. 授权决策快照、当前撤销检查和内容分级访问通过；
5. 稳定分页并发测试通过；
6. Research Adapter 只依赖 Facade，不跨模块读取 SQL/Domain。

### 3.3 Platform 真实集成验收

<badge color="danger">阻断项</badge>

目标清单已整理至 [Platform → 五模块公共依赖与集成验收清单](./Platform-五模块公共依赖与集成验收清单.md)。

未通过原因：
- 真实 MariaDB/Redis 集成未完成；
- Migration 重复执行、失败恢复、版本冲突和回滚/恢复演练未完成；
- HTTP Client 生产出口 SSRF 攻击矩阵未完成；
- Diagnostics 路由权限和各领域 Adapter 接入未完成；
- Worker Request ID 恢复、Fault HTTP 安全映射尚需 Server 集成验收。

通过条件：
1. 真实 MariaDB/Redis 环境集成通过；
2. Migration 故障演练和恢复策略通过；
3. 部署出口 DNS/IP/重定向/代理绕过/IPv4/IPv6 安全门禁通过；
4. Diagnostics 详细接口 owner/service 授权和脱敏通过；
5. Server/Worker 的 Request ID 与 Fault 接入通过。

### 3.4 Data 服务间认证

目标设计已整理至 [Data 服务间认证映射与 Secret 注入设计](./Data服务间认证映射与Secret注入设计.md)。

设计结论：
- Data 业务 API 当前 Operation 使用 `X-API-Key`；
- 首期 Root Server 服务端持有 Data 专用 Key，由 Data Adapter 注入 Header；
- 浏览器不接触 Data Key；
- Owner Token、Platform Service Secret、Data API Key、Provider Token 保持独立。

未通过原因：
- 这是规划设计，尚无 Server/Data 实际接入与部署 Secret 验收证据；
- Key 轮换/撤销、网络 ACL、Header 覆盖、日志脱敏和 readiness 诊断尚需验证。

### 3.5 Quant / Research 逐字段公开 DTO

<badge color="warning">待补齐</badge>

当前已冻结语义与所有权，但没有完成所有公开 DTO 的字段级 Schema 表。

需要逐项确认：
- Quant Data Adapter DTO 与 Data OpenAPI 之间的字段映射；
- QuantResultRef、QuantMetric、RiskDecision 的精度、单位、周期、as_of、质量和版本字段；
- Research Evidence/Context/Event/Claim/Hypothesis/Scenario 的引用、证据类型、授权和时间字段；
- Research 与 Quant 的结果传递必须经 Researchflow，不增加 Research → Quant import；
- 所有 DTO 的 required/optional/nullable、枚举、版本策略和兼容规则。

通过条件：公开 DTO 字段表、示例 JSON、Schema 校验、版本策略和模块间契约测试齐备。

## 4. GOV-004 当前状态表

| 审查项 | 设计审查 | 实现/集成验收 | 是否阻断冻结 |
|---|---|---|---|
| 模块职责与依赖 DAG | 完成 | 后续代码静态检查 | 否（持续门禁） |
| Data 输入语义/PIT/单位/质量 | 完成重点审查 | Data Schema 与自动化测试未通过 | 是 |
| Data OpenAPI 字段/序列化 | 差异清单完成 | 未修复/未通过双向测试 | 是 |
| News 领域边界与 Evidence | 目标契约完成 | Facade/Repository/PIT 未实现 | 是 |
| Platform 包职责边界 | 完成 | 真实集成/安全验收未通过 | 是 |
| Data 服务间认证 | 目标方案完成 | Server/Data 接入未验收 | 是 |
| Quant/Research DTO | 语义草案完成 | 逐字段契约未完成 | 是 |
| LLM/Notification 边界 | 设计基线完成 | 模块实现阶段继续验证 | 否（本阶段） |
| Repository/Migration 所有权 | 完成 | 各模块实现与集成阶段验收 | 否（持续门禁） |

## 5. GOV-004 通过标准

GOV-004 只有在以下全部条件满足时才能标记完成：
- [ ] Data OpenAPI/Go JSON 双向差异归零或所有例外经正式评审。
- [ ] Data required/nullable/枚举/类型/单位/时间 Schema 与真实 JSON 一致。
- [ ] Data 契约测试进入 CI 并通过。
- [ ] News 只读 Facade 与公开 DTO 完成。
- [ ] News Repository/Migration、双时间过滤、授权快照、稳定分页通过测试。
- [ ] Platform 真实 MariaDB/Redis 和 Migration 故障演练通过。
- [ ] Platform HTTP Client 生产出口安全门禁通过。
- [ ] Diagnostics、Fault、Request ID 的 Server/Worker 接入验收通过。
- [ ] Data Key 服务端注入、轮换、撤销和脱敏验收通过。
- [ ] Quant/Research 公开 DTO 字段表、示例和 Schema 版本策略完成。
- [ ] 跨模块 import DAG 与 Repository/Migration Owner 无冲突。
- [ ] 所有验收证据均关联精确提交 SHA 和 CI 结果。

## 6. GOV-005 前置说明

GOV-005 可以先做**规划设计**，包括：
- 共同开发基线候选 SHA；
- 五模块分支创建源；
- PR 目标和合并策略；
- 共享目录 Owner；
- 根 DI/路由/Migration 总注册入口归属；
- 跨模块集成顺序、冲突解决和验收流程。

但在 GOV-004 未通过前：
- 不得将候选 SHA 宣称为冻结共同基线；
- 不创建五个模块分支；
- 不启动依赖未冻结契约的并行实现；
- 不把 Data/News/Platform 当前分支 HEAD 自动视为共同集成点。

**最终结论：GOV-004 设计审查已有明显进展，但实现与集成门禁未通过，当前 NO-GO，不冻结，不创建模块分支。**


## 7. Quant / Research DTO 字段级设计进展（2026-11-05）

新增字段级草案：[Quant / Research 公开 DTO 字段级契约](./Quant-Research公开DTO字段级契约.md)。

本次补齐 QuantAnalysisRequest、QuantResultRef、QuantMetric、TrendResult、RiskDecision，以及 ResearchContext、Evidence、Event、Claim/Hypothesis、Scenario 的主要字段、状态、时间和依赖边界。因此，原第 3.5 节所述“尚无字段级 DTO 草案”的设计缺口已有文档级进展。

但以下内容仍未通过模块 Owner Review，不能将该阻断项标记为完成：
- 价格/金额 decimal 精度与 JSON 表达方式；
- 每个 Quant Use Case 的独立参数结构及 Schema；
- ContextSectionRef、OmittedSection、ScenarioCondition、RiskReason 等复合对象完整字段；
- Evidence 对 Data/Quant/Research 来源的授权审计引用语义；
- Event 多来源时间冲突的来源级表达；
- Quant/Research 示例 JSON、JSON Schema、Go JSON Tag 和 golden 测试；
- Quant/Research 与 Researchflow 的接口级契约测试。

本次仅补齐规划设计，不修改业务模块。GOV-004 仍为 **NO-GO**；Data、News、Platform 的实现/集成阻断项也未因此改变。


## 8. Quant / Research 复合 DTO 与 JSON 示例补齐（2026-11-05）

字段级草案已进一步补齐：
- ContextSectionRef、OmittedSection；
- ScenarioCondition；
- RiskReason；
- EvidenceAuthorizationRef；
- EventSourceTime；
- QuantMetric、RiskDecision、Evidence、ScenarioCondition 的最小 JSON 示例；
- required/optional/nullable 的四种组合及空数组/null/0 语义；
- 货币/价格优先采用 decimal string 的建议及 Data Adapter 边界转换策略。

这使 Quant/Research 的文档级字段定义覆盖了主要复合对象和关键序列化行为，但仍不等于 Schema 已冻结。未完成项：
1. 模块 Owner 对 decimal string 方案及 Data Adapter 转换精度的确认；
2. 各 Use Case 的 typed parameters 完整定义；
3. ContextSectionRef 等复合 DTO 的正式 JSON Schema 与 Go 类型；
4. EvidenceAuthorizationRef 与 Data/News/Quant/Research 各自授权系统的映射；
5. EventSourceTime 规范化规则和冲突裁决算法；
6. Quant/Research/Researchflow 三方示例 JSON、golden 和契约测试。

**当前状态保持 NO-GO。** 本次只更新设计文档，不代表 Data、News、Platform 的实现阻断项已经解决。


## 9. News 最新源码复核更新（2026-11-05）

News 分支当前 HEAD：`cefbbbb10ef418f8bca1d2ff3d6a1fef8ce203b6`，提交 `test(news): 修复证据归属校验格式并覆盖跨新闻版本`。相较前一核查点 `ea697fe437d44afffbd60874ad144adc4313e469`，ahead 4 / behind 0。

本轮确认：
- 新增跨 News Item 的 SourceVersion 证据归属拒绝测试和事务回滚断言，提升写入证据完整性。
- SecurityIdentity / IndustryClassification Resolver 仍只按业务有效时间解析，没有 `system_as_of` 知识过滤。
- Relation 的 `ResolveCurrentRelations` 仍是当前关系解析，不可用于历史 Run。
- `PolicyDecision` 仍只有 Allowed/ReasonCode/MaxRetention；没有不可变授权决策记录和策略快照引用。
- Application 中未发现 ResearchEvidenceQuery / GetEvidence / ListEvidence 只读 Facade；没有稳定分页快照/Cursor 查询实现。
- News README 仍将 N6 Repository/Migration 等列为待完成；本轮没有持久化闭环或真实 MariaDB 验收证据。

因此 News 阻断仍为：Facade/DTO、Repository/Migration、Observation/Version/Identity/Classification/Relation 的知识时间过滤、授权决策快照、当前撤销检查、稳定分页和受控内容读取。最新写入校验测试不能替代上述查询与历史复现验收。

**状态不变：GOV-004 NO-GO。** 本轮未修改 News 业务分支，也未创建模块分支。


## 10. Data / Platform 最新分支快照复核（2026-11-05）

### Data

- HEAD：`fd39c663c4ad59836acdf963d5fb8b2f9227abe8`（`docs(market-data): 记录 BaoStock 首事件前探测结果`）。
- 相较 Schema 核查基线 `4732f14d8998bcd06ffb1118846535b626865b2d`，ahead 7 / behind 0。
- Compare 仅变更 `doc/api/**` 两份文档和 BaoStock 复权探测测试文件；OpenAPI 与 `internal/model/**` 未变更。已知 Schema 差异未由这批提交修复。
- BaoStock 首事件前无 raw 日 K，fallback 实测状态为 not_applicable；AvailableAt 仍未知。该结果不能作为 PIT 可见性或 OpenAPI 契约通过证据。

### Platform

- HEAD：`eb5361172c524f098fc9b26c08e9c45753b76d23`（`docs(platform/diagnostics): define route access matrix`）。
- 相较前次基线 `a30ee12130d2a50bc3db672d9d12d1e14040e34f`，ahead 9 / behind 0。
- Diagnostics Runner 增加总 Deadline、Check 数量上限、重复 ID 校验、panic 捕获和 required skipped 防护；Security 增加显式 RoutePolicy/Router、服务凭据最小长度与 Scope 校验。
- 这些是公共原语进展；真实服务路由、领域 Adapter、真实 MariaDB/Redis、Migration 故障演练和生产出口 SSRF 验收仍未完成。

详细差异分别见 [Data OpenAPI 与 Go 模型契约修正清单](./Data-OpenAPI与Go模型契约修正清单.md) 和 [Platform 五模块公共依赖与集成验收清单](./Platform-五模块公共依赖与集成验收清单.md)。GOV-004 保持 NO-GO。


## 11. Data OpenAPI ↔ Go JSON 完整静态复核更新（2026-11-05）

基于 Data 最新 HEAD `fd39c663c4ad59836acdf963d5fb8b2f9227abe8`，重新静态扫描 11 个 Model 文件（57 个 struct，51 个同名 Schema），并检查关键 Handler 和现有 `openapi_contract_test.go`。

最新字段级结果：
- Go 有、Schema 缺失：15 个字段，8 个 Schema；
- Schema 多出 Go 无来源字段：9 个字段，3 个 Schema；
- required 不一致：15 个字段；
- nullable：122 个 `omitempty` 字段被 Schema 允许 null；另有 23 个字段在 Go 模型层可能输出 null，但 Schema 未允许（22 个非 omitempty slice/map + `LimitPoolItem.change_percent`）。
- AdjustmentBasis 响应枚举和 RequestAdjustmentBasis 请求枚举与 Go 解析器一致。

现有 `openapi_contract_test.go` 仅做 YAML、路由安全和少量字段字符串断言，没有 Go JSON Tag ↔ OpenAPI 的双向自动化检查，也没有完整 Handler JSON golden。关键 Handler 直接输出 Go Model，确认 Quote、StockBoard、PriceLimitEventEvidence 的部分 Schema 字段没有真实 Model 来源。

详细字段清单、6 个内部/内联类型分类、required/nullable 语义和自动化门禁见 [Data OpenAPI 与 Go 模型契约修正清单](./Data-OpenAPI与Go模型契约修正清单.md) 第 11 节。

**结论：Data Schema 阻断项比此前记录更完整，但没有被修复；GOV-004 继续 NO-GO。** 本次只在规划分支更新审查文档，没有修改 Data 业务分支，也没有运行 Data Go 测试。


## 11. Data Operation → Handler → Response Schema 复核更新（2026-11-05）

Data 分支最新 HEAD 已更新为 `4ff4359178be7a055fb81756a5099adb9fcf3f31`，相较原 Schema 核查基线 `4732f14d8998bcd06ffb1118846535b626865b2d` ahead 8 / behind 0。该批只变更两份 `doc/api/**` 文档和 BaoStock Live Probe 测试；OpenAPI、HTTP Handler、Model 未变更，原字段差异仍然有效。

本轮读取路由注册、OpenAPI 200 Response，并跟踪关键 Handler：
- `GET /api/v1/quotes/{symbol}` → `quote` → `*model.Quote`；
- `GET /api/v1/market/quotes` → `batchQuotes` → 私有批量包装 DTO，嵌套同一 `*model.Quote`；
- `GET /api/v1/stocks/{symbol}/boards` → `stockBoards` → `*model.StockBoardSnapshot`，其 items 为 `StockBoard`；
- `GET /api/v1/stocks/{symbol}/price-limit-rule` → `priceLimitRule` → Resolver 结果，嵌套 `*model.PriceLimitEventEvidence`；
- 日 K、Bundle、Probe、Probe History、Market Stocks、Fundamentals、Index Catalog/Constituents/Daily、Trading Calendar、Market Session、Valuations、Market Sentiment、Market Capital Flow 等响应存在 Handler 包装或直接 Service 返回两种模式，已在 Data 修正清单 11.9 建立 Operation 映射表。

三组无 Go 来源字段已确认处于真实响应链路：
1. Quote Schema 的 6 个 validation 字段没有 `model.Quote` 输出来源，且批量行情复用该类型；
2. StockBoard Schema 的 `change_percent_5d`、`provider_report_date` 没有 Go 子项来源；
3. PriceLimitEventEvidence Schema 的 `adjustment_basis` 没有 Go Evidence 字段来源。

`writeJSON` 通过标准 `json.Marshal` 输出，没有统一 nil slice/map 归一化层。Bundle 的 DailyBars/Sources/Warnings 存在显式初始化证据，但不能据此推定其他 Snapshot 的集合字段均非 nil。22 个非 `omitempty` 集合字段仍需逐项追踪 Provider、缓存、降级和空数据路径，并以真实 Handler JSON Golden 验收。

详细 Operation 映射、包装/内联对象、nil/empty 边界和 DATA-CONTRACT-001~006 执行拆分见 [Data OpenAPI 与 Go 模型契约修正清单](./Data-OpenAPI与Go模型契约修正清单.md) 第 11.9 节。

**GOV-004 Data 阻断维持打开**：当前没有 Schema 修复，没有双向 Go/OpenAPI 契约 CI，也没有完整真实 Handler Golden；本轮未运行 Data Go 测试。整体结论仍为 **NO-GO**，不冻结共同契约，不创建五个模块分支。


## 12. Data 最新 HEAD / CI 复核（2026-11-05）

Data 最新 HEAD 更新为 `c22470bec87beb89961b76ad9856d5c8709aac73`。相较 Operation 映射核查 SHA `4ff4359178be7a055fb81756a5099adb9fcf3f31` ahead 1 / behind 0；该提交只修复 BaoStock 探测测试中的转义字符，并更新两份 `doc/api/**` 文档，没有修改 OpenAPI、HTTP Handler 或 Model。

最新 SHA 的 Go test suite run `42012445022` 已 completed / success，包含 market-data gofmt、tests、race、vet、MariaDB/PIT/Probe 集成和 Docker market-data image build；Parity run `42012445049`、WebUI checks run `42012444875` 也 completed / success。Live provider probe run `42012444966` 整体 success，但 BaoStock historical adjustment-factor consistency 专项步骤为 skipped，不能视为八只证券真实探测通过。

因此最新 CI 只证明修复后的现有构建/测试门禁通过，不代表 OpenAPI ↔ Go JSON 契约差异已修复。Data 的 15 个 Go-only 字段、9 个无来源 Schema 字段、15 处 required 差异、122 个过宽 nullable、23 个潜在 null 字段，以及完整 Handler Golden/双向契约测试仍待处理。

详细映射与执行拆分见 [Data OpenAPI 与 Go 模型契约修正清单](./Data-OpenAPI与Go模型契约修正清单.md) 第 11.9 节。**GOV-004 继续 NO-GO**；不冻结共同基线、不创建五个模块分支。


## 13. Data 集合字段 nil/empty 构造路径审查（2026-11-05）

本轮基于 Data HEAD `c22470bec87beb89961b76ad9856d5c8709aac73` 追踪了此前 22 个非 omitempty slice/map 字段的 Service Validator、Provider 返回、缓存反序列化和构造代码。

结果分为三类：
- **11 个字段确认存在 nil 可通过风险**：StockAnnouncementSnapshot.items、DragonTigerSnapshot.items、StockBoardSnapshot.items、StockSearchSnapshot.items、LimitPoolSnapshot.items、FundamentalSection.metrics、ValuationSnapshot.items、IndexCatalogSnapshot.items、IndexConstituentSnapshot.items、IndexDailyBarsSnapshot.items、HotRankingSnapshot.items。
- **3 个字段边界待核实**：MarketSentimentHistory.items、ProviderProbeHistory.items（直接使用 Store List 返回值）；Bundle.daily_bars（直接接收 DailyBars 返回值，未归一化）。
- **8 个字段已有非 nil 约束或显式构造**：Bundle.sources、SectorRankingSnapshot.items、IndexQuoteSnapshot.items、FundamentalsSnapshot.sections、ValuationSnapshot.requested_symbols、MarketCapitalFlowSnapshot.items、ProviderProbeRun.items、TradingCalendarSnapshot.items。
- Bundle.warnings 不属于此前 22 个非 omitempty 字段；它使用非 nil 构造，但空值因 omitempty 被省略。

上述 11 项的 Validator 允许空集合以 nil slice 表示；部分是 unknown/empty fallback 分支，不能仅靠正常有数据样本覆盖。当前没有生产 HTTP 响应抓包证据，故结论是“代码路径允许 null”，而不是宣称已经观察到实际线上 null。

详细逐字段路径、证据与 DATA-CONTRACT-004-A~E 修正任务见 [Data OpenAPI 与 Go 模型契约修正清单](./Data-OpenAPI与Go模型契约修正清单.md) 第 11.11 节。

**GOV-004 Data 阻断继续打开**：尚未实施 DTO 归一化、Store 边界确认、Redis 历史 nil 缓存回归和真实 Handler Golden；没有运行 Data 测试。不得因静态审查完成而冻结契约。


## 14. nullable + omitempty 语义复核：修正 122 项阻断统计（2026-11-05）

重新解析 OpenAPI nullable 属性并与 Go JSON Tag 交叉比对后，确认 122 项（121 个顶层 Schema 字段 + 1 个 Price Limit Rule 内联字段）均未列入对应 Schema 的 required 列表。此前将 122 项整体描述为“过宽 nullable / 契约错误”不准确，现予以修正：

- 对 optional 属性，Go 的 omitempty 在 nil/零值时省略字段，符合“字段可以缺席”的 Schema 约束；nullable=true 允许 null，但不要求必须输出 null。
- 122 项不是 122 个 JSON Schema 结构性错误。它们属于 API 允许显式 null、当前 Go 实现选择省略的语义差异，是否需要统一取决于字段说明和调用方是否需要区分 null 与缺席。
- 已定位 Quote.volume_ratio 的描述明确写了“不支持时为 null”，但 Go Tag 是 omitempty，nil 时会省略。该字段必须裁决：修正文档为“不支持时省略”，或由 Response DTO 输出显式 null。
- 其余字段逐项检查描述中的 null 承诺和业务含义；不要为形式统一而全部移除 omitempty 或全部取消 nullable。
- 本次 required 交叉扫描未发现 nullable + omitempty 字段与 required 的交集，但原先 15 处 Go/OpenAPI required 差异仍独立存在，不能据此关闭。

详细审查和 DATA-CONTRACT-003-B 见 [Data OpenAPI 与 Go 模型契约修正清单](./Data-OpenAPI与Go模型契约修正清单.md) 第 11.12 节。

**GOV-004 阻断计数修正**：不得再把 122 项作为 122 个独立契约错误。阻断应聚焦于明确承诺 null 却实际省略的字段、required 差异、23 个潜在 null 字段、无来源 Schema 字段和缺失的 Handler Golden/双向契约测试。


## 15. Data 最新审查基线更新（2026-11-05）

Data 分支最新 HEAD 为 `bc55db99fc6908c77c3c0ec6a213651373a857b2`（`docs(api): 同步 BaoStock 历史探测窗口计划与契约`），相较上一核查点 c22470b ahead 2 / behind 0。变更涉及文档和 BaoStock Live Probe 测试，没有修改 OpenAPI、HTTP Handler 或 Model，故本轮静态契约判断仍适用。

最新 CI：WebUI checks、Go-Python parity、Live provider probe 已 completed / success；Go test suite run `42017149908` 在本次核查时仍为 in_progress，不能视为通过。

最新基线、Workflow 状态及 11.9/11.11/11.12 复核细节见 [Data OpenAPI 与 Go 模型契约修正清单](./Data-OpenAPI与Go模型契约修正清单.md)。

GOV-004 仍为 NO-GO。不得因非契约文档更新或部分 Workflow 成功关闭 Data 契约门禁。


## 16. LimitPoolItem.change_percent 可达性复核（2026-11-05）

此前 23 个 Go 可 null、Schema 未 nullable 的字段中，LimitPoolItem.change_percent 虽为非 omitempty 指针，但真实 Service Validator 在 HasData=true 时强制 ChangePercent 非 nil；HasData=false 时又要求 Items 为空。因此该字段在当前 DataService → Handler 成功路径上不可达 null，不应计入运行时契约阻断。

此前 23 项应修正为：22 个 slice/map 字段按第 13 节和 Data 修正清单 11.11 继续处理；LimitPoolItem.change_percent 保留 Validator nil 拒绝测试即可。若将来出现绕过 DataService Validator 的 Handler 路径，必须重新审查。

详见 Data 修正清单第 11.14 节。此结论同样说明 Go 类型静态可空不等于 HTTP 响应可达 null。


## 17. required 差异可达性复核（2026-11-05）

原始 15 处 Go/OpenAPI required 差异经重新交叉扫描后，不能全部计为运行时违约。51 个同名 Go/OpenAPI 类型中，发现 3 项 OpenAPI required 但 Go Tag 带 omitempty：

- Bundle.warnings：**真实违约风险**。Service 虽显式构造非 nil 空 slice，但 encoding/json 对 omitempty 的空 slice 仍会省略，OpenAPI 又将其列为 required。建议移除 Go JSON Tag 的 omitempty，确保无警告时输出 []。
- HotRankingItem.source_version：Validator 要求与 Snapshot.SourceVersion 一致，且 Snapshot SourceVersion 必须可识别非空；正常成功路径不可达缺失。
- HotRankingSnapshot.source_version：Validator 强制可识别非空；正常成功路径不可达缺失。

其余 12 处是 Go 无 omitempty、OpenAPI optional；Go 输出字段比 Schema 最低要求更严格，不构成违约。指针字段在 nil 时输出 null，相关 Schema 已允许 nullable。

详细分析与 DATA-CONTRACT-003-C 见 Data 修正清单第 11.15 节。required 阻断应聚焦 Bundle.warnings，并为 HotRanking 的 Validator 约束补测试，不应将 15 项全部视为缺陷。


## 18. Data 最新 HEAD 与 CI 状态更新（2026-11-05）

Data 最新 HEAD 为 `266ac21f5c840207f79aae8984021dac74185652`，相较上一基线 bc55db9 ahead 2 / behind 0。新增的是 BaoStock 八证券复权证据文档和 Live Probe 测试，没有修改 OpenAPI、HTTP Handler 或 Model；本轮字段级静态结论仍适用。

最新 SHA 的 Live provider probe（run `42017899688`）、WebUI checks（`42017899766`）、Go test suite（`42017899888`）均在本次复核时 in_progress；Go-Python parity（`42017899832`）为 pending。不得沿用上一 SHA 的 CI success 作为最新 SHA 的验收证据。

当前 GOV-004 仍 NO-GO；最新 CI 完成后再更新验收状态。


## 19. Data 最新 HEAD 更新至 587f276（2026-11-05）

Data 分支最新 HEAD：`587f27683c8422810d2f7fc917a2c9324eb147b0`，提交锁定 BaoStock 上市日首因子样本边界。相较 266ac21 ahead 1 / behind 0，仅修改 Live Probe 测试文件（增加 22 行），未修改 OpenAPI、Handler、Model 或 Service 业务实现；当前静态契约结论不变。

最新 SHA 的 Go test suite pending、Live provider probe in_progress、Go-Python parity in_progress、WebUI checks success。不得将 pending/in_progress 记录为通过。

详细基线和 Run ID 见 Data 修正清单第 11.17 节。GOV-004 继续 NO-GO。

## 20. Data 缺失 Model 字段的真实 API 路径审查（2026-11-05）

本轮按 Operation → 路由 → Handler → Service/Provider → 实际响应对象核查原 15 个“Go 有字段、OpenAPI 缺字段”问题。结论修正为 **13 个应补 Schema、2 个应从 Go Model 删除**，不应机械地将 15 个字段全部新增到 OpenAPI。

### 应补 Schema 的 13 个字段

| Schema | 字段 | 实际 API 路径 | 运行时证据 |
|---|---|---|---|
| ProviderProbeCheckResult | left_adjustment_method / right_adjustment_method | POST probe、GET probe/history → ProviderProbeRun.check_results[] | Service 跨 Provider 对比结果明确赋值 |
| ProviderProbeCheckResult | left_adjustment_reference_date / right_adjustment_reference_date | 同上 | Service 从两侧 Provider 结果提取参考日期 |
| MarketBreadthSnapshot | excluded_non_a_share_count | GET /market/breadth | TickFlow 使用 len(excluded) 赋值 |
| LimitPoolItem | seal_money / max_seal_money | GET /market/limit-pools → items[] | Hithink 对涨停池解析非负金额；单位由 provider_amount_unit 表达，当前为 provider_defined |
| FundamentalSection | available_at | GET /stocks/{symbol}/fundamentals → sections[] | 当前 Provider 未赋值，但 Service PIT 校验读取该字段，Model 注释定义了精确市场可用时间 |
| IndexConstituent | effective_from / effective_to / available_at / system_observed_at | GET /market/indices/{symbol}/constituents → items[] | 当前 Provider 未赋值；Service 历史区间质量门禁读取这些字段，缺失时降级 |
| SourceStatus | market_capital_flow_capable | GET /data-sources → sources[] | Server 根据 MarketCapitalFlowProviders 能力集合赋值 |

补充规则：
- Provider Probe 四字段均为 optional string；Reference Date 在格式统一完成前不承诺 date format。
- MarketBreadth excluded count 为 optional integer，minimum 0。
- seal_money/max_seal_money 为 optional nullable number，不能写死 CNY。
- Fundamental available_at 为 optional nullable date-time；不从 report_date/publication_date/fetched_at 推导。
- IndexConstituent 有效区间为 optional date，available_at/system_observed_at 为 optional nullable date-time；不得从 provider_report_date 推导。
- SourceStatus capability 为 optional boolean；false 因 omitempty 可缺席。

### 应从 Go Model 删除的 2 个字段

- SectorRanking.provider_report_date：当前 SectorRanking Provider/Service 没有生产赋值点；报告日期已由 SectorRankingSnapshot.provider_report_date 表达，逐条排名重复该字段没有必要。
- SectorRankingSnapshot.excluded_non_a_share_count：当前 Sector Ranking Provider/Service 没有生产赋值点；不能把其他 MarketBreadth/LimitPool 路径的计数移植为行业排名的排除计数。

详细 Operation → Handler → Model 对照、逐字段证据和建议见 [Data OpenAPI 与 Go 模型契约修正清单](./Data-OpenAPI与Go模型契约修正清单.md) 第 11.18 节。

**重要限制**：上述 13 个补充字段尚未修改 Data 分支，也未通过真实 Handler Golden 验证。FundamentalSection 和 IndexConstituent 的 PIT 字段目前由 Provider 缺省，新增 Schema 仅冻结“有可验证来源证据时可返回”的可选契约，不能因此宣称 PIT 数据已可用。

## 21. Data 最新 HEAD 与 CI 复核（2026-11-05）

Data 最新 HEAD 更新为 cce59e8de493813a4b04f7d0bd24211e1e919bb1（style(baostock): 修正专项报告结构体格式 [historical_baostock_factors]），相较 587f276 ahead 1 / behind 0；唯一改动是 internal/baostock/live_adjustment_probe_test.go 的 gofmt 排版。

587f276 的 Go test suite 和 Docker market-data image build 曾因同一测试文件未 gofmt 失败，导致 market-data 单测/race/vet 步骤跳过；其 MariaDB Migration/PIT integration、go-core 和根模块测试通过。cce59e8 修正格式后，最新 SHA 的 Go test suite、Live provider probe、WebUI checks、Go-Python parity 全部 completed / success。

CI 全绿不等于 OpenAPI 与 Go Model 契约问题已经修复。GOV-004 仍 NO-GO。


## 22. Data OpenAPI 多余字段反向审查（2026-11-05）

原先 9 个“OpenAPI 有、Go Model 无”的字段，经 Operation → Handler → 实际序列化对象追踪，确认当前没有 DTO 或自定义序列化路径承载这些字段，建议从 OpenAPI 删除，而不是向 Model 增加空字段：

- Quote.validation_total_count
- Quote.validation_error_count
- Quote.validation_warning_count
- Quote.validation_info_count
- Quote.validation_truncated
- Quote.validation_issues

单条 Quote Handler 直接写出 model.Quote；批量行情的嵌套 Quote 也是 *model.Quote。Go Quote 没有上述字段，也没有 ValidationIssue 包装对象。

- StockBoard.change_percent_5d
- StockBoard.provider_report_date

StockBoards Handler 直接写出 StockBoardSnapshot，Items 类型为 []model.StockBoard。Go StockBoard 只有单日 change_percent，没有五日涨跌幅或逐项报告日字段。

- PriceLimitEventEvidence.adjustment_basis

PriceLimitRule Handler 直接写出 PriceLimitRuleSnapshot。Go PriceLimitEventEvidence 没有 AdjustmentBasis，Service 验证和规则解析也不消费该字段；该字段与事件证据语义无关。

详细路由、Handler 和 Go Model 证据见 [Data OpenAPI 与 Go 模型契约修正清单](./Data-OpenAPI与Go模型契约修正清单.md) 第 11.20 节。

**DATA-CONTRACT-004-F**：删除 9 个 OpenAPI 多余属性；补双向契约测试，禁止 Schema 出现无真实 DTO/Model 来源的字段。若未来确需这些功能，必须先建立 Response DTO 和赋值逻辑，再更新 Handler/Schema/Golden。

## 23. Data 最新 HEAD 与 Workflow 状态更新（2026-11-05）

Data 最新 HEAD 为 `0afec7ead92672ce9c68a7a8f419990471e01620`（复权因子持久化链路审查文档），相较 cce59e8 ahead 1 / behind 0；只改动 doc/api 与接口协作文档，没有修改 OpenAPI、Handler、Model 或 Service。

最新 SHA 的 Go test suite in_progress、Go-Python parity cancelled、WebUI checks success、Live provider probe success。Parity cancelled 不能视为通过，Go test suite 未完成前不能标记全绿。静态字段结论仍适用，GOV-004 继续 NO-GO。


## 24. Data 最新审查基线更新至 1358847（2026-11-05）

Data 最新 HEAD 为 `13588476d000ebeee801e8b7a782fc6678f25dd4`，相较 0afec7e ahead 1 / behind 0。提交只修改 Data/API 规划与 WebUI 协作文档，未修改 OpenAPI、Handler、Model 或 Service；第 20、22 节静态结论仍适用。

该 SHA 的 WebUI checks、Live provider probe、Go-Python parity 均成功；Go test suite run `42018997889` 仍 in_progress。不能把当前 SHA 标记为 CI 全绿。


## 25. Data 最新基线更新至 7abd507（2026-11-05）

Data 最新 HEAD 为 `7abd5078e85ec7e6a5785ec9a4253d862af2a39c`，相较 1358847 ahead 1 / behind 0；只更新 Data API 规划与 WebUI 协作文档，没有修改 OpenAPI、Handler、Model 或 Service。当前 Operation/Handler 静态审查结论不变。

最新 SHA 的 WebUI checks、Live provider probe success；Go test suite 与 Go-Python parity 仍 in_progress。不得记录为 CI 全绿。


## 26. 2026-11-05 三分支最新基线复核：Data / News / Platform

### 26.1 Data 最新基线与阻断

- 分支：`feat/full-go-data-providers-api-switch`
- 最新 HEAD：`8c0136fdcdc5ae6a0d0d293371685d7f684a9f20`
- 相较上一轮记录 `7abd5078e85ec7e6a5785ec9a4253d862af2a39c`：ahead 2 / behind 0。
- 新增/变更涉及 Data API 规划文档及 `services/market-data/internal/provider/pytdx_live_probe_test.go`。
- 最新 Go test suite failure：pytdx_live_probe_test.go 未 gofmt；market-data tests/race/vet 因格式门禁跳过；market-data Docker image build 同样失败。根模块、go-core、MariaDB migration/PIT integration 成功；Live provider probe、WebUI checks、Go-Python parity 成功。
- 这是最新 SHA 的 CI 状态，不得引用上一 SHA 的全绿结果。

处理顺序：先恢复 gofmt 和完整 CI，再执行 DATA-CONTRACT-001～008。任务拆分和逐项验收见 Data 修正清单 11.24。

### 26.2 News 最新基线与实际进展

- 分支：`feat/research-news-ingestion`
- 最新 HEAD：`9ece502acc5008bfa064fe94f9570ab3524ec53d`
- 相较此前记录 `cefbbbb10ef418f8bca1d2ff3d6a1fef8ce203b6`：ahead 21 / behind 0。
- 本轮代码新增/加强 Atom/RSS Feed 内容类型识别、XHTML 结构校验、附件元信息、转载关系 Application、转载双方版本证据校验、版本/关系事务规则和回归测试。
- N4 的领域与 Application 首版已明显超出 README 旧状态描述；转载关系生命周期文档明确列出 Domain/Application/内存事务测试已实现，生产 Repository、数据库唯一约束、跨实例图级串行化仍待 N6。
- N3 离线 Feed 解析与附件元信息已有实现；在线抓取仍必须等待 PLAT-008 领域注入、来源授权和生产 DNS/出口安全验收。
- 最新整仓 Go test suite failure 的直接阻断仍是 market-data 子模块 gofmt；根模块测试、go-core、MariaDB Migration/PIT integration 成功，分析 API Docker 镜像构建成功，market-data Docker 镜像失败。该 Workflow 失败不能归因于 News N4 领域测试失败。
- News README 和部分接入矩阵仍引用旧 Platform HEAD `ac06a544`，且 README 的 N4 状态与开发任务清单/转载关系生命周期文档不一致。只记录为 News 文档同步待办，不在本规划分支替 News 修改专项文档。

### 26.3 Platform 最新基线与实际进展

- 分支：`feat/platform-foundation`
- 最新 HEAD：`235cebe61465a5b60d2578fc75c38ea1e40882c5`
- 最新提交：`fix(platform/httpclient): bound dependency semaphore pools`。
- 本轮 HTTP Client 按 Dependency 有界化 semaphore pool，补充未知 Dependency 的并发槽位测试；平台仍明确要求 Provider BoundDoer 由组合根按 endpoint 注入。
- 当前任务表已正式使用 PLAT-001～PLAT-014：PLAT-013 是 Data/News/Research 领域接入验收，PLAT-014 是跨模块故障演练。PLAT-011 是按需 Metrics，PLAT-012 是必要安全审计。News 文档部分旧内容仍使用 PLAT-011/012 表示接入/故障演练，必须以后续平台正式任务表为准。
- PLAT-003/005/006/007/008/009 均仍为开发中或接入待验收；PLAT-002 待评审；PLAT-010 待评审；PLAT-011 暂缓；PLAT-012 未开始；PLAT-013/014 未开始。
- 最新 Platform Go checks failure：`internal/platform/httpclient/client_test.go` gofmt 未通过，导致平台包 tests/race/vet 被跳过。最新提交不能标记为 CI 通过。
- PLAT-009 已有 Storage/Migration 原语与 fake connector/dialer 包级测试，但真实 MariaDB/Redis 集成、现有分析 Store 接入、Migration 总入口装配和失败恢复演练仍未完成。
- PLAT-008 的 BoundDoer 和受控网络执行实现已有代码与专项测试，但生产 DNS/出口、News Provider 真实注入、真实来源授权和端到端 SSRF 验收仍待完成。

### 26.4 GOV-004 门禁判断

1. Data：契约问题已定位并拆成 DATA-CONTRACT-001～008，但尚未在 Data 分支实施；当前 CI 另有 gofmt 阻断。
2. News：N3/N4 纯领域能力进展明显；N6 Repository/Migration、PIT 知识时间查询 Facade、授权决策快照和稳定分页仍未完成。
3. Platform：公共原语已有实现，但服务入口/DI/路由/领域 Adapter 接入和真实 MariaDB/Redis/SSRF 验收未完成。
4. Quant/Research DTO 与三方契约尚未通过模块 Owner Review 和真实 Schema/Golden 验证。

因此 GOV-004 仍为 **NO-GO**。当前可以继续推进 Data/News 的纯领域和契约工作；不得冻结五模块公共契约，不创建五个模块开发分支，不宣称整体模块开发已可启动。


## 27. Data / News 分支 HEAD 继续前进后的复核（2026-11-05）

在本轮文档更新期间，Data 和 News 分支均再次前进，重新读取最新 HEAD 后修正基线：

### Data

- 最新 HEAD：`80cf2e9a05b1bf89dcd9eef9851a14682c2e647a`。
- 相较 8c0136f ahead 1 / behind 0；只改动 `pytdx_live_probe_test.go`。
- 格式修正提交后 Go test suite 仍 failure，market-data gofmt 仍未通过；Docker market-data image build failure；market-data tests/race/vet 被跳过。
- 根模块、go-core、MariaDB Migration/PIT integration 成功；Live provider probe、WebUI、Go-Python parity 成功。
- 因此 DATA-CONTRACT-001 仍未关闭。

### News

- 最新 HEAD：`3755431b1c8ed9effe447a19c62a5ccede08dc81`。
- 相较 9ece502 ahead 2 / behind 0；新增 Atom 正文与附件隔离测试。
- 最新 Go test suite failure 的直接原因是 `services/market-data/internal/service/provider_probe_live_test.go` 未 gofmt；market-data 测试/race/vet 被跳过。该问题属于 Data 子模块，不是 News Feed 测试断言失败。
- News 新增 Feed 测试后，本 SHA 的 Workflow 未提供可独立证明 News 包测试全套通过的结果；不能从旧 SHA 或根模块旧结果推断最新 News SHA 已通过。

Platform 最新 HEAD 仍为 `235cebe61465a5b60d2578fc75c38ea1e40882c5`，其 Platform Go checks 仍因 `internal/platform/httpclient/client_test.go` gofmt 失败，测试/race/vet 被跳过。

当前三个分支都不能标记为整体 CI 全绿。后续必须以每个最新 SHA 的实际 Workflow 结果为准。


## 28. 最新 HEAD 再校准（2026-11-05）

分支在上节记录后再次推进，当前复核基线如下：

- Data：`125720fe045bb7a5f68a873ddec5e9d2a46cf65f`。第二次 Pytdx gofmt 修正后，最新 Go test suite 仍 failure；market-data gofmt 仍失败，tests/race/vet 跳过，Docker market-data image build 失败。根模块、go-core、MariaDB Migration/PIT integration、WebUI 和 parity 成功；另一个 Live probe run 尚在进行。
- News：`855bac22ecf4b039ccd572f0b9144d5f6e3d4475`。相较 3755431 ahead 3 / behind 0，新增 Feed 附件测试格式修正；最新 Go test suite 仍因 `services/market-data/internal/service/provider_probe_live_test.go` gofmt 失败，News 整仓 Workflow 不能视为通过。
- Platform：`30bcc92434d8c0eb5e9beabd82f6ceee2c055d61`。相较 235cebe ahead 7 / behind 0，新增 HTTP Client 生命周期测试并提交格式修正；该 SHA 对应 Platform Go checks 为 cancelled，尚无最新 SHA 的成功验收证据。此前 235cebe 的 Platform Go checks 因 `client_test.go` gofmt 失败。

当前三分支仍无完整、最新 SHA 的 CI 全绿证据。不得把格式修正提交或 cancelled Workflow 视为通过。GOV-004 继续 NO-GO。


## 29. 最新 Data 编译阻断与 News/Platform 基线（2026-11-05）

### Data

最新 HEAD：`e21447e46b5c2c7e0b50336e4fbbe67885b268fb`。相较此前 125720fe 又新增 3 个提交，涉及跨源日 K 对账测试和 Pytdx 测试。

最新 Go test suite 已通过 gofmt，但 market-data Provider 测试编译失败：`cross_provider_daily_compare_test.go:20:43` 对 `CrossProviderDailyBar` 设置了不存在的 `VolumeGranularity` 字段。race/vet 跳过，Docker market-data 镜像失败。Go-Python parity 中样例比较工具和 parity contract tests 成功，但 market-data Go tests 因同一编译错误失败。根模块、go-core、MariaDB Migration/PIT integration、Live provider probe 和 WebUI checks 成功。

DATA-CONTRACT-001 的 gofmt 子项已恢复；完整 CI 阻断转为测试编译错误，必须先修正该测试与类型定义的契约，再重跑完整 CI。

### News

最新 HEAD：`ff08b35d40ef71e6c96b8b13378c8f2c3bdfa675`。相较 855bac2 ahead 1 / behind 0，修正异常 Atom 正文载荷保留逻辑。最新 Go test suite failure 仍由 Data 子模块 market-data 测试构建失败触发；News 自身根模块测试在该 Workflow 中通过，go-core 通过，分析 API 镜像构建成功。News 新增代码的单独验收仍应以其独立包测试和后续整仓成功 Workflow 为准。

### Platform

最新 HEAD：`d93134def4ac9a4b0632f6fb98675a890b835a2a`。相较 30bcc92 又有 4 个提交，主要更新 HTTP Client 生命周期测试、实现和接入文档；当前 SHA 没有可用的 Workflow run 记录。此前 235cebe Go checks 曾因 gofmt 失败，30bcc92 的检查为 cancelled。故最新 Platform 代码尚无最新 SHA 的成功 CI 证据。

当前三个分支仍未形成完整、最新 SHA 的全绿验收链。GOV-004 继续 NO-GO。


## 30. GOV-004 / GOV-005 门禁边界修订：契约冻结不等于全量集成通过

本节与 GOV-005《模块独立开发与集成治理规范》配套；如前文把提供方实现验收、其他分支 CI 或生产环境验收列为所有五模块共同开工的前置条件，以本节的分层门禁解释为准。

### 30.1 三类结论必须分开

| 门禁 | 验收对象 | 通过条件 | 不要求 |
|---|---|---|---|
| 公共契约冻结（GOV-004） | 当前批次跨模块 DTO/Port、字段语义、依赖方向和版本策略 | 契约文档/Schema/示例经 Owner Review；必需字段、单位、时间、质量、空值、错误和兼容规则明确 | 所有提供方业务实现完成；所有业务分支同时编译；真实 Provider/数据库/模型/渠道均可用 |
| 模块独立开工（GOV-005 L2） | 单个模块当前开发批次 | 本模块首批任务依赖的契约已冻结，未就绪依赖可隔离在 Adapter/调用点之外 | 与本模块无关的模块契约和 CI；其他模块真实环境验收 |
| 跨模块集成（L3） | 明确 SHA 组合的完整系统 | 参与模块构建、契约测试、组合根、真实依赖和端到端测试通过 | 不适用；这是最终组合验收 |

### 30.2 当前阻断项重新分类

| 既有审查事项 | 正确阻断范围 | 是否阻断所有五模块开工 |
|---|---|---|
| Data OpenAPI 与 Go JSON 已知字段差异 | 阻断受影响 Data Adapter 的字段映射和 Data 提供方契约验收；可先开发不依赖这些字段的 Quant 算法与固定样本 | 否 |
| Data Go 测试/CI 编译失败 | 阻断 Data 当前 SHA 的 Provider CI 结论和依赖该构建的真实集成；不影响无 Data 依赖的模块单测/领域开发 | 否 |
| News ResearchEvidenceQuery 生产 Facade/Repository/Migration 未完成 | 若目标 DTO/Port 已在契约文档中明确，Research 可开发自身 Evidence Domain 和契约测试；News 生产 Adapter、历史查询和真实证据读取保持 blocked | 否 |
| News system_as_of、授权快照、稳定分页实现未完成 | 阻断 News 历史证据查询的生产验收；若其语义尚未在契约中明确，则只阻断依赖该语义的 Research Evidence Adapter | 否 |
| Platform 真实 MariaDB/Redis、Migration 故障演练 | 阻断 Platform Storage 的生产集成验收和依赖真实 Storage 的持久化集成；不阻断领域模型、Repository 接口及隔离测试开发 | 否 |
| Platform 生产出口 SSRF 验收 | 阻断 News 在线抓取等受控出网功能的生产验收；不阻断离线解析和不联网领域逻辑 | 否 |
| Data API Key 服务端注入/轮换验收 | 阻断 Server→Data 的真实服务间调用验收；不阻断 Quant 领域算法或基于契约 Fixture 的 Adapter 单测 | 否 |
| Quant/Research DTO 未完成 Owner Review 的具体字段 | 阻断使用这些字段的模块间调用点；对应模块可继续不依赖该 DTO 的领域工作 | 只阻断相关调用点，不自动阻断全部模块 |

### 30.3 GOV-004 的最小冻结范围

进入五模块并行开发前，GOV-004 只需冻结首批开发所必需的最小公共契约：
1. 五模块职责、依赖 DAG、公开接口所有权和禁止 import 方向；
2. 公共基础语义：ID、时间、交易日期、单位、null/unknown、质量状态、版本、错误与 Request ID；
3. Quant 首批计算输入/输出的最小 Application DTO，以及 Researchflow 传递 Quant 结果的引用方式；
4. Research 首批 Context/Evidence 的最小 DTO，News Evidence Query 的目标 Port、授权输入、双时间轴和分页语义；
5. Researchflow Run/Stage/Attempt 首批状态机和模块调用 Port；
6. LLM ModelRequest/ModelResponse 最小协议边界；
7. Notification Event/Command/Delivery 最小契约；
8. Repository/Migration 所有权和共享入口限制。

暂不进入首批开发的字段、能力和实现可登记为 deferred，不要求一次冻结所有未来场景。凡是列入首批契约的字段，不得以“后续再说”留下含义不明的状态。

### 30.4 当前结论修订

- GOV-004 的总体架构和大量语义审查已完成；最小契约是否达到冻结条件，应逐项基于文档和 Owner Review 判断。
- Data/News/Platform 的真实实现与 CI 是各自模块/集成验收证据，不再作为所有五模块同时开工的共同前置条件。
- Quant/Research 字段级 DTO Owner Review、News Query Port 公开 DTO、Researchflow 首批调用 Port 等，仍需完成后才能判断对应调用批次是否达到 Module Ready。
- 本节不把 GOV-004 自动改为 GO，也不授权创建分支；它只修正门禁范围，防止把实现集成阻断误判为全局开工阻断。

**后续执行顺序：** 完成首批最小 DTO/Port 的字段级 Review → 按模块逐个确认 L2 Module Ready → 用户确认共同基线并授权后创建分支 → 模块独立 CI → 按明确参与 SHA 进行集成验收。


## 31. 五模块首批开工契约预审

本节只审查第一批任务实际需要的契约，不把后续高级能力或提供方生产实现作为全局开工条件。结论属于规划侧预审，必须由模块 Owner Review 后才能冻结。

### 31.1 首批开发范围

| 模块 | 首批任务 | 可独立开发的内容 | 当前必须明确的契约 |
|---|---|---|---|
| Quant | QNT-001~003 | 计算上下文、Indicator、Factor、固定样本和纯计算 Domain | 输入时间/单位/复权/质量；Indicator/Factor typed parameters；Metric 单位和精度 |
| Research | RSC-001~002 | Context、Evidence Domain、来源引用、事实/推断分类和版本规则 | Evidence 类型、source_ref/source_version、双时间轴、授权引用、质量摘要 |
| Researchflow | FLOW-001~003 | Run/Stage/Attempt、模板、状态机、DAG 纯逻辑 | 状态迁移、必需/可选阶段、Attempt 重试、deadline/cancel/partial |
| LLM | LLM-001 | 现有模型服务协议盘点和调用样本 | 本批先形成协议调查；LLM-002 再按真实协议冻结 Adapter Port |
| Notification | NTF-001 | Event、Subscription、Delivery 领域模型和幂等规则 | event_type/schema_version/idempotency_key/recipient_scope/payload Schema/status |

### 31.2 Quant 首批字段缺口

- QuantAnalysisRequest.parameters 仍是 typed object 占位。QNT-001 必须先定义 IndicatorRequest 的指标代码、周期、窗口、输入引用和用例参数；Factor 参数需定义母体、分类版本、标准化和异常值策略。
- QuantMetric.value 目前为 number/null，价格/金额的公开精度和 JSON 表达仍待 Owner 决定。不得在不同指标中随意混用浮点和 decimal string。
- Data 百分比使用百分点，Quant 内部比例计算必须通过明确 unit 区分 percentage_point 与 ratio。
- MarketDataSnapshotRef 的 payload_ref 应是不透明引用；输入快照必须保留 PIT、复权、来源版本和质量语义。
- Data OpenAPI/Go JSON 未完成的字段只阻断对应 Data Adapter 和真实 Data 集成；不阻断 Indicator/Factor 纯计算实现。

### 31.3 Research 首批字段缺口

- Evidence.authorization_ref 当前只是字符串占位。建议定义不透明 AuthorizationRef DTO，至少可追溯授权决策引用、策略引用、允许的内容级别和评估时间；具体授权规则仍归 Data/News。
- source_ref 应保持 owner namespace 下的不透明引用，Research 不解析或拼接其他模块内部 ID。
- ResearchContext/Evidence 的知识时间字段统一使用 system_as_of；业务有效时间独立使用 valid_time，避免多个 as_of 混义。
- 首批 Research 只存 metadata、必要摘要、fingerprint 和授权引用；全文由来源模块按当前授权按需读取。
- News Query Facade/Repository 尚未生产实现，只阻断 News 生产 Adapter、历史查询和内容读取，不阻断 Research Context/Evidence Domain 与契约测试。

### 31.4 Researchflow 首批字段缺口（状态机部分由第 32 节更新）

- Run/Stage/Attempt 的基础字段已形成草案；状态迁移、结果归并、取消竞态和 Executor 恢复语义由第 32 节 CONTRACT-006/007 Review 细化。
- timeout 是 Stage/Attempt 的执行结果；主动 cancel 是 Run 控制请求，两者不得合并。
- Run 仅使用 succeeded/partial/failed/cancelled 作为终态；Stage degraded 是否传播为 Run.partial，由版本化 Template 策略决定，Run 质量摘要仍保留 degraded。
- Run 固定 workflow_template_id、template_version、fingerprint、system_as_of 和 deadline；重试不得改变原 Run 的知识时点和输入快照。
- FLOW-001~003 可通过纯领域/内存测试独立验收，不依赖 Platform Task Queue 或其他模块生产 Facade。

### 31.5 LLM 首批范围

- LLM-001 可立即独立执行：核实现有模型服务请求/响应、认证、超时、流式事件、错误、usage 和结构化输出行为。
- 在协议盘点完成前，不冻结完整 ModelRequest/ModelResponse 字段，不建设多模型路由。
- LLM Adapter 不执行工具调用；工具授权、预算和执行属于 Researchflow。
- 真实模型凭据缺失只阻断真实调用验收，不阻断协议解析和 Fixture 测试。

### 31.6 Notification 首批字段缺口

- NTF-001 可独立定义 Event、Subscription、Delivery 领域对象和状态迁移。
- payload 必须由 event_type + schema_version 对应的明确 Schema 约束，不能使用任意 map 直接渲染发送。
- recipient_scope 必须是受控范围类型，不允许任意字符串成为真实发送目标。
- Delivery 的重试必须保留 Attempt 历史；永久错误不无限重试。
- Platform Secret/HTTP 和真实渠道凭据只阻断真实 Channel Adapter 验收，不阻断领域模型、幂等和状态机测试。

### 31.7 首批 Owner Review 状态

| 契约 | 参与 Owner | 当前状态 | 待确认 |
|---|---|---|---|
| Quant Request/Metric/ResultRef | Quant + Researchflow | 待 Review | typed parameters、数值精度、单位 |
| Research Context/Evidence | Research + Researchflow | 待 Review | system_as_of、AuthorizationRef、内容策略 |
| News Evidence Query Port | News + Research + Researchflow | 目标契约已写，待确认 | DTO、知识时间过滤、当前授权、Cursor、错误 |
| Researchflow Run/Stage/Attempt | Researchflow + Platform | 规划侧 CONTRACT-006/007 Review 已完成，待 Owner Review | 传播矩阵、硬风险门禁标记、取消宽限期、租约/并发参数、Repository CAS/fencing 约束 |
| LLM Model Adapter | LLM + Platform | LLM-001 静态盘点完成；LLM-002 Port/DTO 草案完成，待 Owner Review | Responses 字段契约、CapabilityState、Schema validator、SSE 事件策略；本地 E2E 未验收 |
| Notification Event/Delivery | Notification + Platform | 待 Review | payload Schema、recipient_scope、幂等和 Attempt |
| Data Snapshot Input | Data + Quant | 总体语义已定义，字段映射待 Review | PIT、单位、复权、quality、source version |

规划侧提出建议不等于模块 Owner 批准。未记录 Owner 结论的契约不得标记冻结。

### 31.8 当前结论

- 五模块均已达到 Design Ready；是否达到 Module Ready 必须按模块、按首批任务分别确认。
- LLM-001 静态协议盘点已完成，当前本地 llama.cpp 在线 E2E 未执行；LLM-002 的 Port/Fixture 可独立开始，真实模型能力验收仍需补齐。
- Quant、Research、Researchflow、Notification 可继续完善各自不依赖未决字段的领域设计和测试方案；相关跨模块 Adapter/调用点待 Owner Review 后实现。
- Data/News/Platform 的真实实现、真实环境和 CI 仅在对应 Adapter/Integration 批次验收，不作为无关模块的共同前置条件。
- 本节不宣称 GOV-004 已冻结，不自动创建任何模块分支。


## 32. Researchflow CONTRACT-006 / 007 状态机 Review 更新（2026-11-05）

本轮已同步更新《五模块首批ApplicationPort与状态机契约细化》《五模块公共契约与Schema冻结基线》《Researchflow研究流程与状态机设计》《Researchflow任务拆分与验收》和五模块交叉 Review 记录。

### 32.1 统一状态语义

- Run 非终态：created、queued、running、cancelling；终态：succeeded、partial、failed、cancelled。
- Run 不设置 degraded/timed_out。degraded 表示 Stage 输出质量，timed_out 表示 Stage/Attempt 执行预算耗尽。
- Stage 包含 blocked；blocked 不创建 Attempt。skipped 仅用于模板预先裁剪或明确不适用，运行期依赖失败不得伪装 skipped。
- Stage/Attempt 的 failed/timed_out 可在 Run deadline 前按预算重试；每次重试创建新 Attempt，历史记录不可覆盖。

### 32.2 CONTRACT-006 结果传播

Workflow Template 按 stage_code + template_version 固定 on_degraded、on_skipped、on_failed、on_timed_out、on_blocked 策略，并纳入 fingerprint。传播优先级为 fail > partial > succeeded。Stage degraded 可被模板接受，Run 仍可 succeeded，但结果质量必须标记 degraded；只有传播策略明确要求不完整时 Run 才 partial。Run partial 必须携带缺失阶段、降级/阻断原因和 coverage。硬风险门禁不能忽略失败、降级或未执行。

### 32.3 CONTRACT-007 取消与执行控制

- Run 增加 cancelling 中间态；用户取消先原子进入 cancelling，停止新派发并等待在途 Attempt 收敛后才进入 cancelled。
- deadline 是硬边界；deadline_exceeded 与 user_cancelled 使用 termination_reason 区分。deadline 到期的 Stage/Attempt 按传播策略归并为 failed/partial，不增加 Run.timed_out。
- cancel 与正常完成通过事务/CAS 决胜；Attempt 终态不可覆盖，迟到 Worker 通过 lease/fencing token 被拒绝。
- Executor 必须限制全局并发、队列容量、单 Run 并行数，持久化 queued 状态，并支持租约续期、过期回收和进程重启恢复。
- 重试由 Researchflow Application 单点负责；不得在 Executor/Adapter 重复叠加业务重试。

### 32.4 尚未通过的门禁

- 本轮是规划侧 Review 建议，不是 Researchflow Owner 签字；CONTRACT-006/007 仍待 Owner Review。
- Stage 传播策略与 reason_code 的细粒度规则、硬风险门禁 Registry、取消 grace period、Worker lease/heartbeat、队列和并发默认值仍需 Owner 决策或压测。
- FLOW-007 的 SQL CAS、租约唯一约束、fencing token 单调性和恢复事务仍需 Repository/Schema 设计评审。
- 未实现状态机、Executor、持久化或竞态测试；不宣称 FLOW-001~004 已完成。
- GOV-004 整体仍为 NO-GO；不冻结共同契约，不创建模块分支。

**本轮结论：Researchflow 的状态机草案已达到可供 Owner 逐项确认的粒度；FLOW-001~003 仍可独立进行领域开发准备，FLOW-004 需在 Executor 资源与租约契约确认后进入实现。**


## 33. LLM-001 现有 Gateway 协议盘点更新（2026-11-05）

- 核验对象：Kasign/llm-gateway，dev 分支提交 3cd5aacced882143628c23ee00e06243616d1a5f；本轮仅读取，不修改该仓库。
- 当前客户端主协议为 OpenAI Responses，POST /v1/responses；Gateway 调用方认证支持 Bearer 与 X-API-Key，调用方凭据与管理员凭据、上游 Provider 凭据分离。
- Gateway 本地 llama.cpp 路径会改写 instructions/input 中 system/developer 内容，并对流式请求补 include_usage；Responses bridge 不转为 Chat Completions。远程 Provider 有 JSON/SSE Mock 透传测试。
- Structured Output 仅能确认请求字段可透传，不能确认当前本地模型具备完整 JSON Schema/strict 能力。Tool definitions 可透传但不执行。
- Responses usage 的 JSON/SSE 解析已存在；Responses 文本增量的 TTFT/内容审计存在缺口。本地 SSE 中断后的 Responses error event 行为未与远程 Provider 对齐。
- LLM-001 静态盘点完成；真实本地 llama.cpp Responses JSON/SSE、Structured Output、Tool Call 在线验收未执行，故总体状态 partial。详细结果见 doc/llm/03-现有模型Gateway协议盘点.md。
- LLM-002 的 Port/Fixture 可基于已确认协议先行；不得据此把 LLM-004/005 的真实模型能力标记通过。GOV-004 仍未冻结，不创建模块分支。


## 34. LLM-002 Provider Port / DTO 字段级契约更新（2026-11-05）

- 已新增 doc/llm/04-ProviderPort与DTO字段级契约.md，定义首批 Responses Protocol、ProviderConfig、CapabilityState、ModelRequest、ModelResponse、Usage、OutputItem、Stream event、Error Contract 与资源限制。
- 建议首批仅支持单轮文本 input；多轮 item、多模态、background response、多协议路由 deferred，避免将 Gateway 宽松 JSON 直接扩展为无边界内部 DTO。
- Stream 采用同步 callback 事件接口，必须收到 terminal event；无 terminal 的 EOF 是 stream_interrupted，首个事件后不得透明重试。
- CapabilityState 默认 unknown，必须绑定 Provider/Model/Gateway 版本与验证时间；本地 Responses、Structured Output、Tool Call 未经真实 E2E 不得设为 supported。
- 本地 Schema 校验、Usage null、工具不执行和 SecretRef 隔离均纳入契约建议。
- 当前仍待 LLM/Platform Owner Review；未选择 JSON Schema validator 具体库，未实现 Port/Adapter/Fixture，也未创建模块分支。GOV-004 继续 NO-GO。


## 35. LLM-002 Owner Review 建议决议（2026-11-05）

- 原 Port 存在 Stream 不能返回最终聚合 Response、Generate 未定义 Response.status 与 Go error 分工的问题，已在规划建议中修正为 Generate/Stream 均返回 (ModelResponse,error)。
- MVP 输入单轮文本；ToolDefinition 受控纳入但默认 tool_choice=none；reasoning_effort 保留可选但默认关闭；JSON Schema 本地基线建议 Draft 2020-12；Usage.total_tokens 仅在分项完整时派生；SSE sequence_number 严格连续；四类 Request/Correlation ID 分开保存。
- 上游合法 incomplete/failed 由 ModelResponse.status/error 表达；Adapter 执行错误由 Go error 表达；EOF 无 terminal event 返回部分 Response + stream_interrupted。
- 本轮只形成规划侧建议，待 LLM/Platform Owner 签字；validator 库/许可证、真实 Gateway ID header 映射、本地模型 Responses/Schema/Tool/Reasoning/SSE E2E 尚未完成。
- LLM-001 仍 partial；LLM-002 仅契约草案；不宣称 LLM-004/005 能力通过，不创建 feat/llm 分支，GOV-004 继续 NO-GO。


## 36. LLM-002 JSON/SSE Fixture 验收准备

- 已形成 Fixture 设计清单，覆盖 Responses JSON output item、status、Usage、Schema 校验和异常响应，以及 SSE terminal、未知事件、sequence_number 异常、EOF、callback error 和 Context cancel。
- Fixture 验收必须断言原始 wire payload 到内部 DTO 的映射、状态与 typed error、Usage 来源、序号诊断及 completed 判定；不能仅断言 parser 不 panic。
- Fixture 仅证明 Adapter 协议解析与归一化行为，不证明当前 Gateway/本地模型具备 Structured Output、Tool Call、Reasoning 或稳定 SSE 能力。
- 本轮仍为规划设计；没有实现 Fixture 或执行测试。Owner Review、validator 选型和真实 Gateway E2E 仍未完成，GOV-004 NO-GO。


## 37. LLM-002 Stream Tool Call 事件契约补充

- ToolDefinition 作为受控可选字段进入 MVP 后，Stream 事件必须覆盖 function_call 的 output item 生命周期和 arguments delta；仅定义 output_text.delta 不足以承载工具调用。
- ModelEvent 已补充 item/content_part/arguments，首批识别 output_item、content_part、function_call_arguments、refusal 和文本事件。
- Fixture 增加 stream_function_call.sse，验证增量聚合、item/call ID 关联、最终参数一致性和不执行工具。
- 该补充仍属规划侧契约建议；本地模型真实流式工具调用未验收，GOV-004 NO-GO。


## 38. Notification CONTRACT-009/010 Review 建议决议（2026-11-06）

- Notification 已形成 Application Port、Inbox/Delivery 幂等、recipient_scope、Delivery 状态、Attempt/租约/fencing、退订竞态、重试和 Schema 注册建议。
- 关键修正：Delivery 增加 retry_wait、blocked、unknown；unknown 表示外部副作用结果不确定，不能直接重试。过期 Worker 的迟到写入由 fencing token 拒绝。
- 幂等分三层：Event Inbox `(source_module,event_id)`、PublishEvent `(source_module,idempotency_key)`、Delivery 投递计划唯一键；相同 fingerprint 重放返回原结果，不同 fingerprint 冲突。
- DeliveryPlan 固定 Subscription/Channel/Template 版本和 recipient_ref；发送前重新检查当前退订和授权状态。
- 外部渠道不承诺 exactly-once；仅明确 retryable 且可确认未接受的错误自动重试。ambiguous outcome 进入 unknown/对账。
- Schema 注册按 `(event_type,schema_version)` 不可变；模板只暴露变量 allowlist。
- 任务拆分将领域模块测试与真实渠道、Storage、Worker、根 Migration 的 L3 集成验收分开。
- 本轮只更新规划文档，未实现代码、Migration、Worker 或真实渠道测试；CONTRACT-009/010 仍待 Owner Review，GOV-004 不因此转为 GO。


## 39. Notification Fixture 验收准备

- 已形成 NTF-001/004/005/006 的模块级 Fixture 设计，覆盖事件幂等、Schema 版本、订阅变更、投递状态、unknown 对账、重试预算、并发租约/fencing、Outbox/Inbox 和渠道错误分类。
- 测试需断言实际外部调用次数、Attempt 追加历史、幂等键稳定性、状态迁移和敏感信息脱敏。
- Fake Adapter/内存 Repository 只证明模块逻辑，不证明真实渠道、MariaDB、Worker 或根 Outbox/Migration 集成。
- 本轮没有生成测试代码或执行测试；CONTRACT-009/010 仍待 Owner Review，GOV-004 NO-GO。


## 40. 五模块公共契约一致性 Review（2026-11-06）

本轮对公共 Schema、Quant/Research 字段级 DTO、Researchflow Port、LLM Provider Port 和 Notification Port 交叉核对：
- 修正 QuantMetric 摘要，补齐 metric_status、sample_count、lower_bound、upper_bound。
- Evidence 授权字段统一为 authorization_ref，并补 system_as_of、limitations、content_fingerprint；历史授权快照不替代来源 Owner 当前授权检查。
- 明确 ResearchEvent 与 NotificationEvent 是不同对象，ID/版本不共享。
- 明确 ResearchRun.as_of 是唯一系统知识截止值；Evidence/News system_as_of 与其语义一致，observed_at 保留真实观察时间。
- LLM ModelErrorInfo 增加 Platform Fault 映射；request_id 只在 ModelResponse 层表达，Cause 不对外暴露。
- 明确质量、计算结果、流程、模型响应和通知投递状态分域管理；统一幂等和关联 ID 基本规则。

未发现需要调整五模块依赖方向的架构冲突。Owner 签字、正式 JSON Schema、Go DTO/golden 与契约测试仍未完成；GOV-004 保持 NO-GO，不创建模块分支。


## 41. CONTRACT-006 / 007 项目最终决议（2026-11-06）

本节为最新有效决议，取代第 32 节“待 Researchflow Owner Review”的历史状态。项目不存在独立模块 Owner；由项目统一决策方作出设计决议，不再把 Owner 签字作为门槛。

### 41.1 最终设计决策

- Run：created/queued/running/cancelling/succeeded/partial/failed/cancelled；不设置 Run.degraded/timed_out。
- Stage 增加 retry_wait。Template 固定 max_attempts（含首次）、retryable_reason_codes、backoff_policy、attempt_timeout；仅白名单错误自动重试，Attempt 历史追加保存。
- Template 固定 on_degraded、on_skipped、on_failed、on_timed_out、on_blocked 传播策略；required Stage 适用性在 CreateRun 前解析，不适用记录 excluded_stages；resolved DAG 不允许为空。
- 硬风险门禁必须同时匹配 Registry stage_code 和 gate_type=hard_risk；执行层失败/降级/跳过/阻断/超时均 fail。RiskDecision=reject/unknown 是有效业务结果，actionability_status=blocked；Template 必须声明 requires_hard_risk_gate：可执行结论模板必须包含至少一个适用门禁，否则拒绝 CreateRun；纯观察/研究模板 actionability_status=not_evaluated。只有 Run=succeeded 且所有适用硬门禁明确 pass 才可能 actionable。
- RetryStage 只推进 retry_wait，不重开终态 Stage，不修改 Run.as_of、输入快照、Template 版本或重试预算；命令具备幂等键。
- created/queued 无运行中 Attempt 时取消可直接终结；running 取消进入 cancelling。queued Worker claim 与取消由 CAS 决胜。
- deadline 使用数据库 UTC 时钟；db_now >= deadline_at 时 deadline 优先。未启动 Stage 记 timed_out/run_deadline_exceeded；运行中 Attempt 停止或 fence 后记 timed_out；deadline 超时不重试。
- Run 终态必须保证所有 Stage 终结、无 running Attempt、结果摘要固定；聚合使用事务和 Run.version CAS。
- 租约使用单调 fencing token；fencing 仅保护本地写入。外部副作用必须具备下游幂等键；结果不确定且下游不支持幂等时禁止自动重发。

### 41.2 实现验收边界

- CONTRACT-006/007 的设计决策状态：closed。
- FLOW-001~003 领域实现：not_started；需完成状态机、Template、聚合、取消/重试和内存/Repository 契约测试。
- FLOW-004 Executor：not_started；需完成有界队列/并发、租约恢复、fencing 和重启恢复测试。
- 真实数据库事务、进程崩溃、跨模块 Adapter、真实模型/工具副作用属于 L3 Integration，不作为设计决策关闭的前置条件。
- 本轮只更新规划文档，没有生成 Go DTO/JSON Schema/Golden，没有运行代码测试。GOV-004 仍需完成其余模块契约和公共 Schema 自动化验收；不因本决议自动转为 GO。

决议正文：[03-CONTRACT-006与007最终决议](../researchflow/03-CONTRACT-006与007最终决议.md)。


## 16. Researchflow CONTRACT-006/007 当前有效状态（2026-11-06）

本节优先于本文历史章节中“待 Owner Review”的旧表述。项目不存在独立模块 Owner，CONTRACT-006/007 已由项目统一决策方关闭设计决策。当前权威文本为 [Researchflow 状态机最终决议](../researchflow/03-CONTRACT-006与007最终决议.md)。本轮进一步补齐取消/Worker claim/重试/deadline 的 CAS 线性化规则、Attempt 停止确认与 fencing 收敛、队列容量原子预留，以及纯观察模板与可执行建议模板的 actionability 优先级。

设计决策关闭不代表实现验收通过：FLOW-001~004 尚未实现；Go DTO、JSON Schema、Golden、状态迁移 Fixture、模块 CI 和 L3 真实集成仍按各自层级验收。GOV-004 整体仍为 NO-GO，不创建模块分支。
