# 五模块公开契约与 Schema 冻结基线（GOV-004）

- 状态：设计基线草案，待 Review 后冻结；不代表接口或数据库已经实现。
- 目标落点：根 Go Module 的 `internal/{quant,research,researchflow,llm,notification}`。
- Data 是独立 Go Module，通过 HTTP/OpenAPI；News 与其他业务模块在根 Go Module 同进程调用；Platform 是根 Go Module 内公共包。

## 1. 基础序列化约定

| 类别 | 约定 |
|---|---|
| ID | 不透明字符串；消费方不解析内部结构 |
| 时间戳 | RFC3339、带时区；跨模块统一 UTC |
| 交易日期 | YYYY-MM-DD，明确为 Asia/Shanghai 交易日 |
| 时间语义 | 分开事件发生、市场可获得、系统观察、采集、标准化时间 |
| 百分比 | 明确百分点或比例；Data 当前百分比按百分点 |
| 数值单位 | 价格 CNY/股、成交额 CNY、成交量 shares |
| 缺失值 | null 或 unknown；不得补成 0、false、空字符串或当前时间 |
| 版本 | Schema、领域对象、算法/策略/模型版本分别管理 |
| 错误 | 使用 Platform Fault 语义；不传播 SQL/SDK/Provider 原始错误和 Secret |
| Request ID | 跨 API、Adapter、后台任务和日志传播；不是业务主键 |

不建立全局 common/domain 巨型包。领域模型由所属模块持有，跨模块只交换 Application DTO、版本化引用和事件。

## 2. Data → Quant：数据输入契约

Quant 通过自己的 Data Adapter 调用 `services/market-data` OpenAPI；Domain 不依赖 HTTP DTO，也不得 import Data 的 internal 包。Data Service Key 由服务端配置注入，不得进入浏览器、Research 或 LLM。

首期优先数据能力：

- Security Master、证券搜索与 Profile；
- Quote、Daily Bars、Bundle；
- Trading Calendar、Market Session；
- Security Trading Status、Price Limit Rule；
- Corporate Actions、Adjustment Factors；
- Fundamentals（含报告期、公告/可获得时间、系统观察时间）；
- Index、Market Breadth、Sector/Industry。

资金流、筹码、龙虎榜、人气、公告等按具体策略逐项接入，不因 API 已存在就默认成为 Quant MVP 强依赖。

Quant 输入快照最小语义：

```text
MarketDataSnapshotRef {
  snapshot_id, security_id, data_kind, trade_date?, period?,
  as_of, source, source_version?, normalization_version?,
  adjustment_basis?, quality_status, quality_reasons[],
  provider_time?, fetched_at, normalized_at,
  system_observed_at?, unit_metadata, payload_ref
}
```

- Data 标准单位、复权口径、时间字段、质量状态、来源版本、标准化版本和 fallback 元数据必须保留。
- `as_of` 是系统信息截止时间；之后才被系统观察到的修订数据不能进入该时点回测。
- Data Probe、Validation、Freshness、Coverage 是 Data 的权威领域诊断；Quant 只引用结果，不复制规则。
- 输入 degraded 可按策略继续计算，但结果必须标记降级原因；invalid 输入不得作为有效数值事实。
- HTTP 200 不等于 valid；unknown/null 不得转换为 0。

## 3. Quant → Researchflow / Research：量化结果

Quant 只暴露 Application 用例，不暴露 Domain/Repository 私有类型。

```text
QuantResultRef {
  result_id, schema_version, security_id, as_of, analysis_period,
  algorithm_version, strategy_version?, input_snapshot_refs[],
  quality_status, result_status, created_at
}
QuantMetric {
  metric_code, value, unit, period, as_of,
  quality_status, calculation_version
}
RiskDecision {
  decision_id, status: pass|reject|unknown,
  rule_set_version, reasons[], evaluated_at, input_refs[]
}
```

数值必须有指标编码、单位、周期、时点和算法版本；不得只用无语义的 map[string]float64。精确货币/价格/数量需明确精度和舍入策略；浮点指标需定义误差容忍与固定样本。Risk unknown 不能视为 pass。Research 通过 Researchflow 接收 QuantResultRef/必要摘要，不直接 import Quant。

## 4. News → Research：新闻与证据

News 负责来源授权、身份、Observation、新闻主体、版本、内容载荷和采集质量；Research 负责 Event、Evidence、Claim、Hypothesis、Scenario。

```text
NewsObjectRef {
  source_id, source_endpoint_id?, news_item_id?, observation_id,
  item_version?, source_policy_version, provider_policy_version?,
  content_ref?, identity_status, business_quality_status,
  authorization_status, published_at?, published_at_raw?,
  available_at?, observed_at, fetched_at?, normalized_at?
}
Evidence {
  evidence_id, evidence_type: fact|reported_fact|inference|hypothesis|unknown,
  source_owner: data|news|quant|research, source_ref, source_version?,
  observed_at?, available_at?, published_at?, valid_time_start?,
  valid_time_end?, polarity: supports|contradicts|neutral,
  confidence?, quality_status, authorization_status, created_at
}
```

- Search Candidate/Observation 不自动成为 News Item 或发布者事实。
- 来源未识别、身份冲突或授权未通过时，不得伪造稳定新闻主体/正文引用。
- 正文/附件仅在 Provider 与 Publisher/Endpoint 授权交集允许时提供 content_ref。
- 新闻主体版本与每次 Observation 不同；重复观察不制造内容版本。
- 转载保留 source-specific 身份和关系证据，不自动合并不同来源主体。
- `published_at`、`available_at`、`observed_at`、`fetched_at`、`normalized_at` 不得互相替代。
- fact 与 reported_fact 不等价；inference/hypothesis 必须保留推导来源和失效条件。正反证据都必须可追溯。
- Research 引用 News 稳定 ID/版本/证据定位，不复制未获授权全文，不重建 News Provider/授权体系。

## 5. Research：领域对象边界

Research Application 至少提供 Context、Evidence、Event、Claim/Hypothesis、Scenario 用例。

- Event 是多来源证据归并后的研究对象，保留合并依据与生命周期。
- Claim/Hypothesis 必须有支持/反证、验证方法、预期观察和失效条件。
- Scenario 表达条件和可能路径；概率引用 Quant Evaluation，样本不足为 unknown。
- Research 不拥有 Run/Stage 状态机，不调用 LLM，不直接调用 Data HTTP，不负责通知投递。

## 6. Researchflow：Run / Stage / Attempt

```text
ResearchRun {
  run_id, request_id, request_type, security_ids[], as_of,
  workflow_template_id, workflow_template_version, status,
  input_snapshot_refs[], result_ref?, created_at, started_at?, finished_at?
}
ResearchStage {
  stage_id, run_id, stage_code, stage_version, required,
  depends_on[], status, latest_attempt_no
}
ResearchAttempt {
  attempt_id, stage_id, attempt_no, status, started_at?, finished_at?,
  deadline_at, input_refs[], output_ref?, error_code?, safe_message?
}
```

- Run 状态：queued/running/succeeded/partial/failed/cancelled。
- Stage/Attempt 状态：pending/queued/running/succeeded/degraded/skipped/failed/timed_out/cancelled/blocked。
- 可选阶段未执行为 skipped 且有 reason；必需依赖缺失不能 skipped。
- 重试创建新 Attempt，不覆盖历史；DAG 必须无环；并行阶段共享同一 Run 的 as_of 与输入快照。
- 部分结果必须标记 partial/degraded 并指出缺失阶段。
- Workflow Template、Stage、算法均版本化。Run/Stage/Attempt 存储归 Researchflow，不归 Platform。

## 7. LLM：模型调用

```text
ModelRequest {
  request_id, purpose, model, messages[], response_format?,
  max_output_tokens?, deadline, metadata_allowlist
}
ModelResponse {
  request_id, provider, model, output, finish_reason,
  usage?, latency, provider_request_id?
}
```

LLM 只实现模型协议和 Adapter，不执行工具、不持有研究领域事实、不编排工作流。Tool Call 是模型返回的请求，由 Researchflow 授权、校验和执行。Structured Output 必须按调用方 Schema 校验；解析失败不能伪装成功。Prompt 正文默认不持久化。首期不要求多模型路由/自动降级。

## 8. Notification：事件与投递

```text
NotificationCommand {
  event_id, event_type, schema_version, idempotency_key,
  occurred_at, recipient_scope, template_id, template_version?,
  payload, correlation_id
}
DeliveryRecord {
  delivery_id, event_id, channel, recipient_ref,
  status: queued|sending|sent|failed|skipped,
  attempt_count, next_attempt_at?, sent_at?, failure_code?, safe_message?
}
```

Subscription、Template、Channel、Delivery、Retry 和投递历史归 Notification。上游传递版本化事件，不要求 Notification import 业务领域类型。幂等键在副作用前确定；永久错误不无限重试；无凭据/未配置渠道应明确 skipped/blocked。告警条件由 Quant/Research/Researchflow 对应业务规则计算，Notification 只负责投递。

## 9. Platform 公共能力使用边界

| 包 | 允许复用 | 不允许接管 |
|---|---|---|
| config | 类型化配置、Secret 脱敏 | 领域配置 Schema/业务策略 |
| security | owner/service 身份和可信上下文 | 用户中心/租户系统 |
| fault | 稳定错误类别、Code、安全摘要、Dependency、Retryable | 领域错误状态/质量规则 |
| observability | Request ID、日志字段、脱敏 | 领域证据和业务审计历史 |
| diagnostics | Check、Observation、超时/并发、基础聚合 | 领域质量算法和诊断历史 |
| httpclient | 连接安全、超时、响应上限、有限幂等重试 | Provider 配额、来源授权、业务语义 |
| storage | SQL/Redis 生命周期、Ping、Migration Runner | Repository、表结构、SQL、保留策略 |
| task | 未来经验证可复用的技术原语 | Research Run/Stage/DAG/Attempt |

Data 独立 Module 通过 HTTP/JSON 使用服务，不 import 根模块 Platform internal。Platform Observation 是执行结果外壳，Data/News/Quant/Research/Researchflow/Notification 保留各自质量规则、证据和历史。

## 10. Repository / Migration 所有权

| 模块 | 目标持久化对象 |
|---|---|
| Quant | 算法/策略版本、计算结果、回测、Evaluation、风险评估 |
| Research | Context、Evidence、Event、Claim、Hypothesis、Scenario |
| Researchflow | Run、Stage、Attempt、Workflow Template、输入/输出引用 |
| LLM | 仅必要调用元数据/用量摘要；默认不保存 Prompt 正文 |
| Notification | Subscription、Template、Delivery、投递 Attempt/状态 |
| Data | 证券、行情、财务、Provider、Probe、质量及历史（独立 Module） |

Migration namespace 由模块固定且唯一，如 quant/research/researchflow/notification。Migration 版本不可变，修复使用更高版本。Migration 清单由模块提供，总注册和执行入口由服务端集成分支装配。任何模块分支不得提前修改根 Migration 总注册、DI、HTTP 路由或启动入口。MariaDB 是事实存储；Redis 只作缓存/短期辅助。

## 11. Go import 方向

允许：Quant → Data HTTP Adapter/Platform；Research → Platform；Researchflow → Quant Application、Research Application、LLM Application、Notification Application、Platform；LLM → Platform；Notification → Platform。

禁止：
- Quant ↔ Research；Research → Quant Go 包。
- Quant/Research/LLM/Notification → Researchflow。
- LLM/Notification → Quant 或 Research 领域包。
- 任一模块引用其他模块 Domain/Repository/Adapter 私有包。
- 任一根模块包引用 Data 独立 Module 的 internal 包。
- Platform → 任一业务领域包。

## 12. GOV-004 验收门禁与未决项

- 每个模块仅暴露 Application DTO/Port，不跨模块暴露 Domain 类型。
- Data Adapter 保留单位、复权、质量、来源版本、时间和 PIT 语义。
- Evidence 区分事实/报道事实/推断/假设/未知，保留来源、时间和授权引用。
- Run/Stage/Attempt 可表达超时、取消、重试、部分成功和历史版本。
- LLM 不执行工具、不覆盖数值或硬风险门禁；结构化输出必须校验。
- Notification 事件版本、幂等键、投递状态和重试边界明确。
- Repository/Migration 所有权明确；无共享万能 Repository/Task/Domain 包。
- Data OpenAPI 的 Probe validation/coverage 字段需再次与 Go 响应模型逐项核对，不能沿用历史 Schema 缺口。
- News N6 持久化需等待 Platform Storage 真实 MariaDB 集成验收。
- Platform service Secret 与 Data 当前 X-API-Key 的映射需明确；服务间 Secret 只在服务端注入。
- Platform Task Queue 仍存在分析领域耦合；Researchflow 首期使用自有 Run/Stage/Attempt 存储和有界 Worker。
- Platform Storage 包级 CI 通过不代表真实 MariaDB/Redis 集成和故障演练完成；HTTP Client 单测通过不代表生产网络出口安全验收完成。
- 以上是语义基线草案；最终字段/Schema 需结合 Data OpenAPI、News Application 输出和各模块评审冻结。


## 13. GOV-004 字段级 Review：Data / News / Platform（2026-11-05）

**核查方式：** 通过 GitHub Branch API 读取分支 HEAD，再按该分支 ref 读取源码和 OpenAPI。以下 SHA 是本次核查快照，不是未来模块开发的共同基线。

| 分支 | 本次核查 HEAD | 提交信息 |
|---|---|---|
| `feat/full-go-data-providers-api-switch` | `40c2a63656ad83c756b3ef12ce69eabc89275b07` | `docs(market-data): 记录历史日历质量边界复验` |
| `feat/research-news-ingestion` | `0eb727b4c6a42f568818bdf9a7161afd09db5284` | `fix(news): 补齐版本更新代码格式` |
| `feat/platform-foundation` | `b7896ea7cc49d925d896f770c479b84c04688c67` | `docs(platform): record policy-bound HTTP adapter validation` |

### 13.1 Data OpenAPI ↔ Go Model

核查文件：
- `services/market-data/openapi.yaml`
- `services/market-data/internal/model/provider_probe.go`
- `services/market-data/internal/httpapi/provider_probe.go`
- `services/market-data/internal/validation/report.go`
- `services/market-data/internal/model/model.go`
- `services/market-data/internal/repository/provider_probe.go`

**已确认一致的核心字段**

| 契约对象 | OpenAPI 字段 | Go JSON 字段 | Review 结论 |
|---|---|---|---|
| ProviderProbeFieldIssue | stage/field/provider_field/code/expected/actual_type/actual_preview/severity/message/row_index/symbol/trade_date/count | 同名 JSON tag | 一致；枚举、长度和脱敏规则应由 API 契约测试继续覆盖 |
| ProviderProbeCoverageDimension | dimension/population/source/total/returned/validated/missing/excluded/ratio | 同名 JSON tag | 一致；ratio 在 Go 中是非指针 float64，Schema 不应允许 null |
| ProviderProbeItem | provider/capability/status/elapsed_ms/source_version/adjustment_basis/quality_status/validation counts/issues/error/coverage/time fields | 同名 JSON tag | 主体一致；coverage_total/returned/validated/missing/excluded/ratio 在 Go 中是指针并 omitempty，Schema 允许缺省；ratio 可为 null |
| ProviderProbeRun | run_id/scope/requested*/check_results/status/persisted/timestamps/counters/items | 同名 JSON tag | 一致；状态枚举需持续同步 |
| ProviderProbeHistory | items/total | 同名 JSON tag | 一致 |
| Validation Report | total_count/error_count/warning_count/info_count/issues/truncated | validation.Report 同名 JSON tag | 内部结构一致；Probe Item 将其展平为 validation_* 字段，不应误认为 API 直接暴露 Report 对象 |
| Quote | 价格、成交量/额、源时间、抓取/标准化时间、单位、复权、质量、fallback、版本 | model.Quote 同名 JSON tag | 核心字段一致；nullable 数值用指针表达，避免缺失变零 |
| DailyBar | 交易日、OHLC、MA、成交量/额、复权、因子观察时间、源时间、抓取/标准化/系统观察时间、质量和版本 | model.DailyBar 同名 JSON tag | 核心字段一致；MA/成交量/额为指针，PIT 因子时间可缺省 |

**发现的 Schema 缺口**

1. 自动字段名对照进一步发现 8 组 OpenAPI 缺口：`ProviderProbeCheckResult` 缺 `left_adjustment_method`、`right_adjustment_method`、`left_adjustment_reference_date`、`right_adjustment_reference_date`；`SectorRanking` 缺 `provider_report_date`；`SectorRankingSnapshot` 与 `MarketBreadthSnapshot` 各缺 `excluded_non_a_share_count`；`LimitPoolItem` 缺 `seal_money`、`max_seal_money`；`FundamentalSection` 缺 `available_at`；`IndexConstituent` 缺 `effective_from`、`effective_to`、`available_at`、`system_observed_at`。这些字段均存在 Go JSON tag，若对应响应对外序列化，OpenAPI 必须补齐；不得静默丢弃。
2. `ProviderProbeCheckResult.calendar_known`、`is_trading_day`、`age_seconds`、`freshness_max_age_seconds` 在 Go 中为指针并 omitempty，OpenAPI 应保持可选；若字段存在，布尔 false/整数 0 都是有效值，不能用 omitempty 的客户端语义误判为未知。
3. `ProviderProbeItem.coverage_ratio` 在 Go 中是 `*float64`，且仅在分母可证明时输出；OpenAPI 的 nullable 语义是必要的。Coverage 缺失不等于 0%。
4. Probe 的 `status`、`error_stage` 和 `error_code` 是不同层级：status 描述探测结果，error_stage 描述失败阶段，error_code 是稳定机器分类。客户端不可把三者合并为一个通用失败状态。
5. `ProviderProbeRun.persisted=false` 明确表示本次探测未持久化；历史查询在 Storage 未启用时返回服务不可用，不能伪造空历史。
6. 本轮对 11 个 model Go 文件中的 57 个结构体执行字段名静态对照，其中 51 个结构体可按同名 Schema 直接匹配；在这些匹配项中发现上述 8 组字段缺口。其余 6 个结构体未按同名 Schema 匹配，可能是内部模型、嵌套结构或由外层响应包装，需逐个确认是否对外暴露，不能简单判为缺失。该静态检查只比较 JSON tag 名称，不验证类型、required/nullable、枚举、嵌套引用和条件语义。OpenAPI 合约测试也未逐字段反射比较所有 Go JSON tag；新增 DTO 字段必须同步补充契约测试。

**Data Adapter 最小输入 DTO 决策**

Quant 首期只建立自己的 Adapter DTO，不复制整个 Data Model：

- Security Identity：symbol/security identity、exchange、security type、listing/trading status 及其 known 标记。
- DailyBar：trade_date、OHLC、volume/amount 及单位、period、is_partial、adjustment_basis、adjustment method/reference date/factor source/version、factor system_observed_at、provider timestamp、fetched_at、normalized_at、system_observed_at、source/source_version、quality_status/reasons、fallback metadata、normalization_version。
- Quote：nullable 价格/成交量/额、quote_time + quote_time_semantics、fetched_at/normalized_at、单位、复权、source/version、quality/reasons、fallback 和 normalization version。
- Calendar/Session：交易日明确值、交易所时区、请求时点、服务生成时点、交易阶段和质量。
- Fundamentals：report_date、publication_date、system_observed_at（可验证时）、source/version、quality；当前接口的 publication_date 是所选 Provider 修订的披露日期，不等于完整 PIT 修订选择器，不能据此宣称财务历史版本已完整 PIT。
- Market Breadth/Sector：报告日期、覆盖分母/分子、total lower-bound、质量、来源版本；聚合字段需保留覆盖证据。
- 其他资金流/筹码/龙虎榜/公告等作为按策略启用的可选输入，不纳入所有 Quant 用例的默认必需依赖。

**PIT 重要边界**

- DailyBars 响应具有 point_in_time/as_of 语义；DailyBar 的 system_observed_at 和复权因子 system_observed_at 必须纳入快照。
- Fundamentals 当前 schema 有 as_of 查询时的 system_observed_at，但 publication_date 的说明明确不是历史修订选择器。财务 PIT 完整性需作为独立验收项，不能只看响应含有 as_of 参数。
- Quote、Sector、MarketBreadth 等实时/快照对象通常只有 fetched_at 或 provider_report_date；它们不自动具备可回溯历史 PIT 能力。
- `analysis_reference_time`、`provider_report_date`、`provider_time`、`fetched_at` 语义不同，不可映射到同一个通用 timestamp 字段。

### 13.2 News Application ↔ Research Evidence

核查文件：
- `internal/news/domain/types.go`
- `internal/news/domain/observation.go`
- `internal/news/domain/identity.go`
- `internal/news/domain/version_record.go`
- `internal/news/domain/content.go`
- `internal/news/domain/policy.go`
- `internal/news/domain/quality.go`
- `internal/news/domain/news_item_relation.go`
- `internal/news/domain/relation.go`
- `internal/news/app/ports.go`
- `internal/news/app/ingest.go`
- `internal/news/app/research_update.go`
- `internal/news/app/search.go`

**当前真实 Application 能力**

| 能力 | 当前契约/用例 | 状态与限制 |
|---|---|---|
| Search Candidate | Candidate、SearchRequest、SearchResult、ProviderOutcome、SearchCandidates.Execute | 已有多 Provider 查询聚合；搜索用例明确不持久化、不缓存 |
| Observation | Observation + IdentityRepository/IdentityTransaction + IngestCandidate.Execute | 有事务端口、来源内身份解析和幂等追加语义；尚不能据此认定已有生产 Repository |
| Item Identity | SourceID + source_native_id/normalized_url 凭据 | 来源内匹配；Hash 命中后比较原文；多主体命中隔离为 conflict |
| Content | ProviderSummary/SourceSummary/Body 分层；Body 含 origin/format/kind/text | 搜索 Provider 摘要不自动提升为发布来源摘要或正文；当前是内存载荷模型，不是稳定 content_ref 存储服务 |
| Version | NewsItemVersion：NewsID/VersionNo/Fingerprint/Title/Publisher/PublishedAt/Content/ObservedAt/ObservationID | 不可变版本快照；指纹排除 Provider 摘要、采集时间和查询上下文 |
| Source/Endpoint/Provider Policy | SourcePolicy、EndpointPolicy、ProviderOutputPolicy、PolicyDecision | 分别评估来源入口授权与 Provider 输出权利；授权策略模型当前没有统一的显式 policy_version 字段 |
| Business Quality | valid/degraded/invalid/unknown + metadata/content/source 维度 | 与 Platform Diagnostics 状态分离；来源身份必须由身份流程提供 |
| Repost Relation | NewsItemRelation + Observation/Version evidence | 不合并不同来源主体；校验重复边和循环转载链 |
| Research Relation | RelationAssertion + RelationEvidence | 追加式、可 supersede；证据定位到版本/Observation/字段/规则/模型 |
| Research Update | ApplyResearchUpdate + ResearchUpdateRepository/Transaction | 版本追加与关系更新在一个事务端口中执行；仍是抽象端口，无已确认持久化 Adapter |

**对 GOV-004 草案的修订要求**

1. 当前 Candidate/Observation/NewsItemVersion 没有 `available_at` 字段。NewsObjectRef 的 available_at 只能由明确的可获得性证据/上游来源契约补充；不能从 published_at、observed_at 或 fetched_at 推导。
2. 当前模型没有独立 `fetched_at` 与 `normalized_at` 字段。ProviderOutcome.ObservedAt 表示查询结果观察时间；Observation.ObservedAt 表示候选观察时间。若未来需要精确采集/标准化耗时点，必须在 News 自有模型中明确新增，不可借用同一 ObservedAt。
3. 当前 SourcePolicy/EndpointPolicy/ProviderOutputPolicy 没有 `policy_version` 字段。策略版本可先用不可变策略记录 ID 或明确版本化策略快照表达；在没有持久化版本模型前，不应把字符串版本字段当作已有实现。
4. 当前 ContentLayers/ContentPayload 以内嵌内容表示，未提供稳定 `content_ref` 或授权内容仓库 ID。Research 只能在授权范围内通过 News Application 获得必要内容，不可假定已经有可持久化正文引用。
5. 当前没有完整公开的 Research Query Facade。已存在的 `ApplyResearchUpdate` 是新闻版本/关系写入用例，不等于 Research 模块可直接调用的只读 Evidence Query API。需在 News 持久化闭环与跨模块 Facade 设计阶段补齐。
6. News 的 `IdentityRepository` 与 `ResearchUpdateRepository` 是事务端口；当前代码树尚未实现对应 Repository/Migration，故 Evidence 持久化和跨重启引用稳定性仍未验收。

**NewsObjectRef 语义调整**

当前可由现有 News 事实直接支持的字段为：
- `source_id`、`news_item_id`（身份已解析时）、`observation_id`；
- `provider_code`、来源原生标识/规范 URL 的身份类型（通过受控引用，不直接暴露敏感原文）；
- `version_id`、`version_no`、`fingerprint`（版本已建立时）；
- `identity_status`、`business_quality_status`；
- `published_at` / `published_at_raw`、`observed_at`；
- `authorization_decision_ref`（需要引用来源、Endpoint 与 Provider 权利评估，不是单一 bool）。

以下字段暂列为未来契约能力，未完成领域建模前不可视为现有输出：`available_at`、`fetched_at`、`normalized_at`、`source_policy_version`、`provider_policy_version`、`content_ref`。Evidence 中的 valid-time、极性、置信度和事实分类属于 Research 自己的领域判断，不应伪装成 News 输出。

### 13.3 Platform 公共包 ↔ 五模块依赖

核查文件：`internal/platform/{fault,diagnostics,httpclient,storage,taskqueue,config}` 对应源码。

| 包 | 实际可用 API / 语义 | 五模块使用限制 |
|---|---|---|
| fault | `Fault{Class, Code, SafeMessage, Dependency, Retryable, Cause}`、`New`；Cause JSON 忽略 | 领域 Adapter 可映射为 Fault；New 不会自动填 Dependency/Retryable，调用方需明确设置 |
| diagnostics | `Check{ID,Name,Required,Timeout,Run}`、`Runner.Run`、Observation.Validate | required check 返回 skipped 会被提升为 failed；领域状态需 Adapter 映射，不要直接复用 Data 的 passed/unknown 枚举 |
| httpclient | `New(Config)`、`Client.Bind(DestinationPolicy)`、`BoundDoer.Do` | 推荐每个外部 Endpoint 创建固定策略 BoundDoer；不能让业务调用方每次请求动态放宽 Host/Port/Redirect Policy |
| storage | `OpenSQL`/`OpenRedis`、`PingSQL`/`PingRedis`、Migration Runner | 只提供连接和迁移执行原语；真实 MariaDB/Redis 部署集成仍是独立门禁 |
| config | `Env`、`Secret`、`RequiredSecret` 等 | Secret.Value 仅在明确构造认证客户端时使用；日志/格式化/JSON 会脱敏 |
| taskqueue | Queue.Runner 直接接收 `analysis/domain.RunRequest` 并处理旧分析 Run/Stage 事件 | 明确不是通用任务队列/DAG 引擎；Researchflow 不得依赖其领域模型 |

**额外发现**

- Diagnostics 的 Observation 状态是 healthy/degraded/unavailable/skipped/failed，与 Data Probe 的 passed/degraded/skipped/failed/not_configured/not_supported 不同；必须通过 Adapter 映射并保留 reason_code/evidence_ref。
- Diagnostics Runner 会将 Required=true 的 skipped 结果转换为 failed，正好支撑“必需依赖缺失不能伪装 skipped”的规则。
- HTTP Client 的 BoundDoer 将目的地址策略固定在构造期，适合 News Provider Adapter，但仍需真实部署环境验证 DNS/IP/出口边界。
- Platform Task Queue 源码仍直接依赖 `internal/analysis/domain`，Researchflow 首期继续自有 Run/Stage/Attempt 与 Worker。
- Storage 真实集成、Migration 失败/回滚演练、Redis 故障演练和生产出口 SSRF 验收均不能由包级单测替代。

### 13.4 本轮 Review 结论与剩余工作

**已完成：**
- Data OpenAPI 与 Provider Probe Go JSON 字段的主要逐字段核对；
- Data Quote/DailyBar 与质量、时间、单位、复权字段核对；
- News Application 当前输出、持久化端口、身份、版本、内容、授权和关系能力核对；
- Platform 核心包导出 API 与依赖限制核对。

**仍未完成：**
1. 对 Data 所有业务响应 DTO（证券、财务、交易制度、指数、资金流、市场宽度、公告等）逐个完成 Go JSON tag ↔ OpenAPI 的全量自动化对照；本轮只做了 Quant 首期数据面和 Probe/Validation/Coverage 的重点审查。
2. 补齐 Data OpenAPI 已识别的 8 组字段缺口，并确认未按同名 Schema 匹配的 6 个 Go 结构体是否对外暴露；增加能捕获 Go JSON tag 漂移的契约测试。
3. 为 News 增加可供 Research 调用的只读查询 Facade、明确可获得时间和采集时间语义、策略版本引用及授权内容引用；等待 N6 Repository/Migration 实现。
4. 对 Platform Storage 执行真实 MariaDB/Redis 集成和故障演练；对 HTTP Client 执行部署环境出口安全验收。
5. 结合以上结果修订模块 DTO，完成 Quant/Research 的公开契约字段表与 Schema 版本策略。

因此 GOV-004 状态更新为：**Data / News / Platform 重点字段级 Review 已完成；全量 Schema 对照和未决契约补齐仍待完成，尚未冻结。**
