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
  quality_status, metric_status, sample_count?,
  lower_bound, upper_bound, calculation_version
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
  observed_at, available_at?, published_at?, valid_time_start?,
  valid_time_end?, system_as_of, polarity: supports|contradicts|neutral,
  confidence?, quality_status, authorization_ref, limitations[],
  content_fingerprint?, created_at
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

Research Application 至少提供 Context、Evidence、ResearchEvent、Claim/Hypothesis、Scenario 用例。ResearchEvent 是研究领域内多来源证据归并对象，与 NotificationEvent（跨模块集成事件）不是同一实体，ID、版本和生命周期不可互换。

- Event 是多来源证据归并后的研究对象，保留合并依据与生命周期。
- Claim/Hypothesis 必须有支持/反证、验证方法、预期观察和失效条件。
- Scenario 表达条件和可能路径；概率引用 Quant Evaluation，样本不足为 unknown。
- Research 不拥有 Run/Stage 状态机，不调用 LLM，不直接调用 Data HTTP，不负责通知投递。

## 6. Researchflow：Run / Stage / Attempt

```text
ResearchRun {
  run_id, request_id, request_type, security_ids[], as_of,
  workflow_template_id, workflow_template_version, status,
  termination_reason?, deadline_at,
  input_snapshot_refs[], result_ref?, result_summary?,
  created_at, started_at?, finished_at?
}
ResearchStage {
  stage_id, run_id, stage_code, stage_version, required,
  depends_on[], status, latest_attempt_no, propagation_policy,
  blocked_by_stage_codes[], reason_code?
}
ResearchAttempt {
  attempt_id, stage_id, attempt_no, status, started_at?, finished_at?,
  deadline_at, lease_owner?, lease_expires_at?, fencing_token,
  input_refs[], output_ref?, error_code?, safe_message?
}
```

- Run：created/queued/running/cancelling 为非终态；succeeded/partial/failed/cancelled 为终态。Run 不设置 degraded/timed_out；质量降级由结果摘要表达，超时由 Stage/Attempt 表达并按模板归并。termination_reason 区分 user_cancelled 与 deadline_exceeded。
- Stage：pending/ready/running/succeeded/degraded/skipped/failed/timed_out/cancelled/blocked；Attempt：running/succeeded/degraded/failed/timed_out/cancelled。blocked 不创建 Attempt；skipped 仅用于模板裁剪/不适用。
- Stage 的传播策略随 Workflow Template 版本化；Stage degraded 可被接受但仍标记结果质量 degraded，也可映射 Run partial/failed。Run partial 必须给出缺失阶段、原因和 coverage。
- 重试创建新 Attempt，不覆盖历史；Run cancel 先进入 cancelling，拒绝新执行并等待在途 Attempt 收敛；Run deadline 触发超时路径，不增加 Run.timed_out。
- cancel/完成通过原子 CAS 决胜；deadline 是硬边界；Worker 租约过期后以 fencing token 阻止旧 Worker 迟到写入。DAG 必须无环；并行阶段共享同一 Run 的 as_of 与输入快照。
- Workflow Template、Stage、算法均版本化。Run/Stage/Attempt 存储归 Researchflow，不归 Platform。

## 7. LLM：模型调用

```text
ModelRequest {
  request_id, purpose_code, model, instructions, input_text,
  max_output_tokens, temperature?, top_p?, reasoning_effort?,
  response_schema?, tools[], tool_choice?, deadline_at, correlation_id?, trace_id?
}
ModelResponse {
  request_id, provider_id, response_id?, status, output_items[], output_text?,
  structured_json?, tool_calls[], usage?, model_requested, model_actual?,
  model_version?, gateway_request_id?, upstream_request_id?, incomplete_reason?, error?,
  started_at, finished_at, duration_ms
}
ModelEvent {
  type, sequence_number?, response_id?, output_index?, content_index?,
  item_id?, call_id?, item?, content_part?, delta?, arguments?,
  usage?, error?, received_at
}
Usage {
  input_tokens?, output_tokens?, total_tokens?, input_cached_tokens?, output_reasoning_tokens?, source
}
CapabilityState {
  provider_id, model_id, gateway_version, capability_code,
  state: supported|unsupported|unknown, checked_at, evidence_ref?
}
ModelErrorInfo {
  kind, stable_code, retryable, http_status?, retry_after?, safe_message
}
```

首批协议以 OpenAI Responses 为基线，内部 ModelRequest 暂定单轮 input_text + instructions；复杂 input item、多模态和多协议路由 deferred。ModelResponse 必须保留 output item 类型与 completed/incomplete/failed 状态；Usage 缺失保持 unknown/null。Stream 使用同步 callback 并返回最终 ModelResponse；无 terminal 的 EOF 视为 stream_interrupted。ToolDefinition 是受控可选项，默认 tool_choice=none，只有 capability=supported 才允许发送；LLM 不执行工具，Researchflow 负责授权、校验和执行。Structured Output 本地 Schema 基线建议 Draft 2020-12，上游 strict 子集能力单独验证。Request ID、Gateway ID、上游 ID 和 correlation ID 分开保存。Prompt 正文默认不持久化。字段细节见 doc/llm/04-ProviderPort与DTO字段级契约.md。

## 8. Notification：事件与投递

```text
NotificationEvent {
  event_id, event_type, schema_version, source_module, subject_ref,
  idempotency_key, occurred_at, payload, correlation_id
}
DeliveryRecord {
  delivery_id, event_id, subscription_id, subscription_version,
  channel_ref, channel_config_version, template_id, template_version,
  recipient_ref, plan_fingerprint,
  status: queued|sending|retry_wait|sent|failed|skipped|blocked|unknown,
  attempt_count, next_attempt_at?, sent_at?, reason_code?, safe_message?
}
DeliveryAttempt {
  attempt_id, delivery_id, attempt_no, worker_id, fencing_token,
  started_at, finished_at?, outcome: accepted|rejected|ambiguous|not_sent,
  provider_message_ref?, error_code?, retryable?, duration_ms?
}
Subscription {
  subscription_id, owner_ref, version, status: active|paused|disabled,
  event_types[], recipient_scope, channel_ref, template_ref
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
| `feat/full-go-data-providers-api-switch` | `4732f14d8998bcd06ffb1118846535b626865b2d` | `test(market-data): 补齐复权重建同源测试夹具` |
| `feat/research-news-ingestion` | `ea697fe437d44afffbd60874ad144adc4313e469` | `style(news): 对齐行业分类领域代码格式` |
| `feat/platform-foundation` | `a30ee12130d2a50bc3db672d9d12d1e14040e34f` | `docs(platform): record request correlation and fault safety boundaries` |

### 13.1 Data OpenAPI ↔ Go Model

本轮最新 Data 分支 HEAD 已推进到 `4732f14d8998bcd06ffb1118846535b626865b2d`。通过 compare 检查，`c1ae8bb1f02a10a7f8c73a40f125fce3f1e4da3c` 至该 HEAD 之间只修改了 Data 文档与 PIT 测试夹具，没有修改 `services/market-data/openapi.yaml` 或 `internal/model/**`；因此下述 Schema 发现仍适用于最新分支。

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

1. 在本轮最新 Data HEAD `c1ae8bb1f02a10a7f8c73a40f125fce3f1e4da3c` 上重跑静态对照后，确认 **9 组、15 个字段**存在 Go JSON Tag 已定义但 OpenAPI 未声明的情况：`ProviderProbeCheckResult` 缺 `left_adjustment_method`、`right_adjustment_method`、`left_adjustment_reference_date`、`right_adjustment_reference_date`；`SectorRanking` 缺 `provider_report_date`；`SectorRankingSnapshot` 与 `MarketBreadthSnapshot` 各缺 `excluded_non_a_share_count`；`LimitPoolItem` 缺 `seal_money`、`max_seal_money`；`FundamentalSection` 缺 `available_at`；`IndexConstituent` 缺 `effective_from`、`effective_to`、`available_at`、`system_observed_at`；`SourceStatus` 缺 `market_capital_flow_capable`。这些对象均可由当前 HTTP API 路径或响应引用到达，应视为真实 OpenAPI 缺口，而不是内部字段。
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

**本轮新增领域模型（最新 HEAD：`ea697fe437d44afffbd60874ad144adc4313e469`）**

- 新增 `SecurityIdentityRecord`：以 `IdentityNamespace + IdentityValue` 表达稳定证券级身份，同时保留 Market/Symbol/Name、代码状态、上市生命周期、ValidFrom/ValidTo、ObservedAt、Source 和 EvidenceRef。解析结果明确区分 resolved/not_found/ambiguous；不会跨市场按相同代码合并。
- 新增 `IndustryClassificationRecord`：以 ClassificationSystem + ClassificationVersion + IndustryCode 定义行业代码空间，保留父级、名称、有效期、观察时间和来源证据；同一代码跨版本不自动映射。
- `RelationAssertion` 现支持 Security / Industry 两种 Subject，关系断言可 proposed/confirmed/rejected/retracted，修正通过 supersedes 追加；RelationEvidence 定位到 SourceVersion、Observation、规则/模型版本、字段路径和证据哈希，不复制正文。

**新增模型的 PIT 与持久化边界**

- 两类 Identity/Classification Resolver 当前只按业务有效时间（ValidFrom/ValidTo）解析；记录虽含 ObservedAt，但解析函数没有 system-known-at 参数，也不会自行过滤 ObservedAt。当前测试也未覆盖知识时间截断。
- 这不代表有效时间解析逻辑本身错误，但 Researchflow 的历史 Run 若要求“当时已知事实”，必须由 Repository/Query Facade 在装配输入前执行 `ObservedAt <= system_as_of` 的知识时间过滤，或将双时间参数显式纳入 Resolver。不能只传一个 as_of 并把它同时解释为业务有效时间和系统观察时间。
- SecurityIdentityRecord 要求 IdentityNamespace/IdentityValue，但当前 ResolveSecurityIdentity 的查询键仍是 Market + Symbol + valid time；跨代码变更的同一证券身份链路仍需通过稳定 IdentityValue 或显式映射关系维护。
- 新增内容目前是纯 Domain 类型与解析/校验函数，没有 Repository、Migration、Application 查询 Facade 或 Data Provider Adapter。不得将其描述为已接入 Data 证券主数据或已具备持久化 PIT 查询。
- 新增关系的 `ResolveCurrentRelations` 是当前关系状态解析，不是历史知识时点查询；Research Evidence Snapshot 仍需固定本次运行看到的关系断言和证据版本。

**对 GOV-004 草案的修订要求**

1. 当前 Candidate/Observation/NewsItemVersion 没有 `available_at` 字段。NewsObjectRef 的 available_at 只能由明确的可获得性证据/上游来源契约补充；不能从 published_at、observed_at 或 fetched_at 推导。
2. 当前模型没有独立 `fetched_at` 与 `normalized_at` 字段。ProviderOutcome.ObservedAt 表示查询结果观察时间；Observation.ObservedAt 表示候选观察时间。若未来需要精确采集/标准化耗时点，必须在 News 自有模型中明确新增，不可借用同一 ObservedAt。
3. 当前 SourcePolicy/EndpointPolicy/ProviderOutputPolicy 没有 `policy_version` 字段。策略版本可先用不可变策略记录 ID 或明确版本化策略快照表达；在没有持久化版本模型前，不应把字符串版本字段当作已有实现。
4. 当前 ContentLayers/ContentPayload 以内嵌内容表示，未提供稳定 `content_ref` 或授权内容仓库 ID。Research 只能在授权范围内通过 News Application 获得必要内容，不可假定已经有可持久化正文引用。
5. 当前没有完整公开的 Research Query Facade。已存在的 `ApplyResearchUpdate` 是新闻版本/关系写入用例，不等于 Research 模块可直接调用的只读 Evidence Query API。需在 News 持久化闭环与跨模块 Facade 设计阶段补齐。
6. News 的 `IdentityRepository` 与 `ResearchUpdateRepository` 是事务端口；当前代码树尚未实现对应 Repository/Migration，故 Evidence 持久化和跨重启引用稳定性仍未验收。

**Research 只读查询 Facade：目标契约（设计建议，尚未实现）**

Research 不应直接访问 News Repository、SQL 表或内部 Domain 聚合；建议在 News Application 暴露稳定的只读 Facade，由 News 自己完成授权校验、知识时间过滤、版本选择和内容裁剪。

建议最小端口：

```go
type ResearchEvidenceQuery interface {
    GetEvidence(ctx context.Context, request GetEvidenceRequest) (EvidenceRecord, error)
    ListEvidence(ctx context.Context, request ListEvidenceRequest) (EvidencePage, error)
}
```

请求语义：
- `GetEvidence` 必须指定 `news_id`，并可选指定 `version_id`；不指定版本时必须显式提供 `system_as_of`，由 News 选择该知识时点可见的最新版本。
- `ListEvidence` 以明确的时间窗、来源/主体引用和分页上限查询；不能把任意外部 URL 作为抓取指令。
- 所有请求均包含调用方用途（personal research / internal business 等）和所需内容级别；News 按 Source + Endpoint + Provider Output Policy 重新评估权限，不因内部调用而绕过授权。
- `system_as_of` 是系统知识时间；`published_at` 是来源声明的发布时间；`available_at` 只有获得可验证证据才返回，三者不能互相替代。
- 结果保留 `news_id`、`observation_id`、`version_id/version_no/fingerprint`、Source/Provider 标识、身份状态、质量状态、时间字段和授权决策引用。
- 正文/摘要采用按需字段，不默认返回全量内容；响应只提供本次授权允许的内容级别。Research 若要保存证据快照，应保存引用、指纹、必要摘要和授权依据，不得绕过许可复制全文。
- 查询结果应返回 `observed_at` 和被选版本的明确标识；分页必须稳定，不能在分页期间悄悄切换版本集合。

返回模型建议命名为 `ResearchEvidenceRecord`，属于 News Application 的公开 DTO，不直接暴露 `domain.NewsItemVersion`、`ContentLayers` 或 Repository Entity。Research 将它转换为自己的 Evidence Domain，并自行维护 Claim/Hypothesis/Scenario 与证据关系。

**PIT 必须区分两条时间轴：**
1. 业务有效/来源时间：`published_at`、行业/证券关系 `valid_from/valid_to`；
2. 系统知识时间：`observation.observed_at`、`version.observed_at`、身份/分类记录 `observed_at`。

对于历史 Run，Researchflow 传入统一 `system_as_of`；News Query Facade 必须先按知识时间过滤 Observation、Version、Identity、Classification 和 Relation，再按业务时间解析。当前 SecurityIdentity / IndustryClassification Resolver 仅按业务有效时间解析，尚不能独立保证这项门禁。若某个来源事实没有可证明的 `available_at`，只能以系统实际观察时间作为“本系统已知”边界，不得伪造来源首次公开时间。

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
- News Application 当前输出、持久化端口、身份、版本、内容、授权和关系能力核对；新增证券身份、行业分类和领域关系模型已纳入审查；
- Platform 核心包导出 API 与依赖限制核对。

**仍未完成：**
1. 已完成 11 个 Model 文件的字段名和主要类型静态对照；已发现 required 不一致 15 处、nullable 不准确 10 处、OpenAPI 多余字段 9 个。仍需人工确认全部 Schema 的格式/范围/枚举/嵌套引用，并把 Go JSON Tag 与 OpenAPI 的双向对照固化为自动化测试。Data 最新 HEAD 相对本次 Schema 核查基准仅有文档和测试夹具变更，未改变模型/OpenAPI。
2. 修正 Data OpenAPI 已识别的 9 组、15 个字段缺口、15 处 required 不一致、10 处 nullable 不准确及 9 个多余字段；6 个未按同名 Schema 匹配的结构体已完成内外部归类；增加双向契约测试。
3. 按本节目标契约为 News 增加 Research 只读查询 Facade；补充可获得时间、采集时间、策略版本和授权内容引用；为 SecurityIdentity/IndustryClassification/Relation 增加 Repository/Migration 与 system_as_of 知识时间过滤；等待 N6 Repository/Migration 实现。
4. 对 Platform Storage 执行真实 MariaDB/Redis 集成和故障演练；对 HTTP Client 执行部署环境出口安全验收。
5. 结合以上结果修订模块 DTO，完成 Quant/Research 的公开契约字段表与 Schema 版本策略。

因此 GOV-004 状态更新为：**Data / News / Platform 重点字段级 Review 已完成；全量 Schema 对照和未决契约补齐仍待完成，尚未冻结。**


### 13.5 Data OpenAPI ↔ Go JSON 修正执行清单

已将已知字段差异整理为独立执行文档：[Data OpenAPI 与 Go 模型契约修正清单](./Data-OpenAPI与Go模型契约修正清单.md)。该清单只在 `feat/design_doc` 维护，实际 Data Schema/Model/测试修改归 Data 分支。

本轮把已知差异拆成四类：
- Go JSON Tag 已有、OpenAPI 缺失：15 个字段，需按真实响应路径补齐。
- Required：已知差异清单为 15 项，其中 12 项是 Go 始终输出但 Schema 未列 required，3 项是 Go `omitempty` 但 Schema 列 required。
- Nullable：原审查摘要记录 10 处不准确，但可逐项明确的 Go `string + omitempty` 字段目前只有 9 项；另有 `LimitPoolItem.change_percent` 应补 nullable。不得凭猜测补出第 10 个字段。
- OpenAPI 多余字段：9 个，需证明由真实 Handler 输出，否则从 Schema 移除。

**重要校正：** 既有 Review 的 required/nullable 汇总数量与逐字段列表存在计数差异。当前不把“15/10”直接视为已经穷尽的最终差异；Data 分支必须通过双向契约脚本从最新 Go JSON Tag、Handler 实际 JSON 和 OpenAPI 响应引用路径重新生成差异报告。字段表中的已知项必须全部核验，新增差异也必须补录。该校正避免把审查摘要误当作准确的机器核对结果。

执行门禁：
1. 先核对实际 Handler 序列化来源，再修 OpenAPI；不为 Schema 虚构 Go 字段。
2. 双向比较 Go 可输出字段与 OpenAPI 声明字段，并处理嵌入结构、自定义 MarshalJSON、内联对象和 Handler 包装 DTO。
3. 用 JSON golden/fixture 验证 required、omitempty、nullable、零值和 null。
4. 对枚举、单位、日期/时间、精度、范围、嵌套引用和分页约束做契约验证。
5. 契约测试进入 Data CI；OpenAPI lint、Go 测试、Handler 测试和契约测试全部通过后，才允许 Data Schema 进入 GOV-004 冻结评审。

本轮没有修改 Data 分支、OpenAPI 或 Go 业务代码；没有创建模块分支。


### 13.6 News → Research 只读证据查询 Facade 契约

新增目标契约文档：[News → Research 只读证据查询契约](./News-Research只读证据查询契约.md)。

本轮重新读取 `feat/research-news-ingestion` 的 Application / Domain 文件，并将分支与既有核查 SHA `ea697fe437d44afffbd60874ad144adc4313e469` 比较：当前分支 ahead 1 个提交，变化集中在新闻关系文档、领域类型与测试。当前 Application 仍只有 SearchCandidates、IngestCandidate、ApplyResearchUpdate 等能力，没有 Research 只读 Query Facade。

**当前已实现与未实现的边界：**
- 已有 Candidate/Observation/NewsItemVersion、来源/入口/Provider 授权策略、Identity 和追加式 Relation 模型，以及写入事务端口。
- 尚无 ResearchEvidenceQuery、GetEvidence/ListEvidence DTO、Repository 查询实现、system_as_of 历史知识过滤、稳定分页、不可变授权决策引用和完整内容引用。
- PolicyDecision 当前只包含 Allowed/ReasonCode/MaxRetention；策略模型没有可用于历史审计的不可变版本引用。
- SecurityIdentity/IndustryClassification 虽保存 ObservedAt，现有 Resolver 只按业务有效时间解析；ResolveCurrentRelations 也不是历史知识时点查询。

**目标契约已明确：**
1. News Application 提供 GetEvidence/ListEvidence 只读 Facade；Research 不访问 News Domain/Repository/SQL。
2. 请求明确传入 system_as_of、purpose、requested_content_level、可信 caller_ref；不接受任意 URL 或任意 SQL 过滤。
3. 返回 DTO 保留主体/Observation/版本/指纹/来源/Provider/身份/质量/时间/授权引用；只有获得许可才提供摘要、正文或 content_ref。
4. PublishedAt、AvailableAt、ObservedAt、FetchedAt、NormalizedAt 分开；available_at 只允许由可验证来源证据产生。
5. 历史查询先按 system_as_of 过滤 Observation、Version、Identity、Classification、Relation，再按 valid_at / published_at 解析业务事实。
6. 授权交集覆盖 Source、Endpoint、Provider；新增不可变 AuthorizationDecisionRef，历史授权审计与当前内容读取授权分离。
7. ListEvidence Cursor 绑定固定 system_as_of、过滤条件、排序版本和快照，禁止 OFFSET 并发分页导致漏项/重复。

本轮只在 `feat/design_doc` 增加契约文档；News 分支未修改。News N6 Repository/Migration 仍等待 Platform Storage 真实 MariaDB 验收。GOV-004 未冻结。


### 13.7 Platform → 五模块公共依赖与集成验收

新增清单：[Platform → 五模块公共依赖与集成验收清单](./Platform-五模块公共依赖与集成验收清单.md)。

本轮重新比较 `feat/platform-foundation` 与已记录核查点 `a30ee12130d2a50bc3db672d9d12d1e14040e34f`：当前分支 ahead 2 个提交，变更涉及 dotenv、security 测试和配置/日志/诊断文档；未发现既有 Fault、Diagnostics、HTTP Client、Storage、Task Queue 职责边界发生改变。

文档把五模块可依赖的 Platform 能力和禁止事项整理为矩阵：
- 可复用：config/Secret、security trusted context、Request ID/slog、Fault、Diagnostics Check/Observation/Runner、受控 HTTP Client、SQL/Redis 生命周期和 Migration Runner。
- 不作为稳定通用依赖：历史 Analysis HTTP transport、当前与 `internal/analysis/domain` 耦合的 Task Queue。
- 领域所有权不转移：Repository/SQL/Migration、业务授权、领域诊断规则/历史、Researchflow Run/Stage/Attempt、Provider 配额和质量规则。
- 真实 MariaDB/Redis、Migration 故障恢复、生产出口 SSRF、Diagnostics 路由/领域 Adapter、Worker Request ID 恢复仍需集成验收。

**依赖规则：** Platform 不得反向依赖业务领域；Data 独立 Go Module 不得 import 根模块 Platform internal。模块分支不得提前修改根 DI、HTTP 路由、cmd 或 Migration 总注册入口。

本轮只更新规划文档，不修改 Platform 分支。GOV-004 仍未冻结。


### 13.8 Data API Key 与 Platform Service Secret 边界

新增设计文档：[Data 服务间认证映射与 Secret 注入设计](./Data服务间认证映射与Secret注入设计.md)。

本轮重新读取 Data OpenAPI：
- `ApiKey` 定义为 `X-API-Key`；
- `BearerAuth` 是另一个独立 Security Scheme；
- 全局 Security 声明两者 OR 关系，但业务 API Operation 显式使用 ApiKey；
- `/health/live` 与 `/health/ready` 显式无认证。

因此，不能因为 OpenAPI 顶层还声明 BearerAuth，就推断业务 Data API 已接受 Platform Service Secret。首期推荐保持 Data 当前认证协议：根 Server 服务端持有 Data 专用 Key，由 Quant/Data Adapter 注入 `X-API-Key`；WebUI 不接触该 Key。Data Provider Token、Data API Key、Root Server Owner Token、Platform Service Secret 各自独立。

文档明确 Secret 注入、Header 覆盖、固定 DestinationPolicy、健康接口、环境隔离、轮换/撤销、日志脱敏和未来统一服务身份的迁移门槛。Data 独立 Go Module 不 import 根模块 Platform internal。

本轮只更新规划文档，不修改 Data/Platform/Server 代码。该设计待 Server/Data 负责人确认后成为实现契约；GOV-004 仍未冻结。


### 13.9 GOV-004 最终 Review 结论：NO-GO

新增审查结论文档：[GOV-004 最终 Review 与契约冻结门禁](./GOV-004最终Review与契约冻结门禁.md)。

当前可作为稳定设计基线的部分：
- 五模块职责、依赖 DAG 和禁止 import 方向；
- Data 单位/质量/复权/PIT 语义；
- News/Research 所有权和 Evidence 语义；
- Researchflow Run/Stage/Attempt 与 LLM/Notification 边界；
- Repository/Migration 领域所有权。

当前阻断冻结的实现/验收项：
1. Data OpenAPI/Go JSON 字段、required/nullable/枚举差异修正和双向契约测试；
2. News ResearchEvidenceQuery Facade、Repository/Migration、system_as_of 历史查询、授权快照和稳定分页；
3. Platform 真实 MariaDB/Redis、Migration 故障演练、生产出口 SSRF；
4. Data Key 服务端注入、轮换/撤销和脱敏验收；
5. Quant/Research 逐字段公开 DTO、Schema 示例和版本策略。

**结论：GOV-004 当前 NO-GO，不冻结共同契约，不创建五个模块分支。** GOV-005 可先做规划设计，但不能冻结共同基线 SHA 或启动模块并行实现。


## 14. 五模块公共契约一致性规则（2026-11-06）

字段级权威定义：Quant/Research 见 `Quant-Research公开DTO字段级契约.md`，LLM 见 `doc/llm/04-ProviderPort与DTO字段级契约.md`，Notification 见 `doc/notification/03-ApplicationPort与投递状态契约.md`。本节统一跨模块语义，不合并各领域状态机。

### 14.1 时间
- 时间戳统一 RFC3339 UTC；交易日仍为 Asia/Shanghai 的 YYYY-MM-DD。
- Researchflow 的 `ResearchRun.as_of` 是一次 Run 唯一系统知识截止时间。Quant Request/Result、Research Context/Claim/Scenario 的 `as_of`，以及 Evidence/News Query 的 `system_as_of`，在该 Run 语境中表达同一截止值；所有下游沿用原值，不得自行刷新。
- `observed_at` 永远是真实观察时间，不得回填为 as_of；`occurred_at` 是业务事件发生时间；`published_at` 是来源声明时间；`available_at` 仅在有证据时提供；`fetched_at/normalized_at` 分别表示采集/标准化时间。
- NotificationEvent.occurred_at 不等于接收/投递时间；Delivery/Attempt 自行记录处理时间。

### 14.2 ID 与幂等
- `request_id` 标识一次调用；`correlation_id` 关联跨模块工作链；`event_id` 标识稳定事件；`idempotency_key` 标识调用方的幂等命令意图。四者不得互换。
- 异步消费者用自身 Inbox 去重，不用 correlation_id 去重。
- 幂等命令由 Owner 定义作用域和规范化请求 fingerprint：相同作用域/key/fingerprint 返回原结果；相同 key 不同 fingerprint 返回冲突，不覆盖历史。唯一键作用域由命令 Owner 定义，禁止全局共享幂等键空间。
- Quant 计算、Researchflow CreateRun、Notification PublishEvent 分别拥有自己的幂等约束；Notification Inbox、PublishEvent、Delivery 三层幂等不能合并。

### 14.3 错误与状态
- Platform Fault 是同进程执行错误的基础语义，保留稳定类别/Code、Dependency、Retryable、安全摘要；Cause 仅内部日志。
- 模块可定义受控公开错误投影（如 ModelErrorInfo），但必须给出 Fault 到 kind/stable_code/retryable 的确定映射；不得暴露原始 Cause、Secret 或 Provider 正文。ModelResponse.request_id 负责关联，ModelErrorInfo 不重复携带 request_id。
- quality_status、Quant result_status、RiskDecision、Researchflow Run/Stage/Attempt、ModelResponse.status、Notification Delivery.status 各自独立。禁止按同名字符串直接跨域转换。
- Run partial 表示覆盖不完整；Run succeeded 仍可有 degraded 质量摘要；Risk unknown 不得映射为 pass。Notification skipped/blocked/unknown 不是 Platform Fault 的同义词。

### 14.4 事件与版本
- ResearchEvent 是 Research 领域聚合对象；NotificationEvent 是集成消息。二者 ID、版本和生命周期独立；转换必须显式映射并生成独立 Notification event_id。
- DTO/event_type 的 schema_version 由生产 Owner 管理且不可变；兼容性按消费者可见语义评估。
- QuantMetric 的 value/bounds 类型由 metric_code Schema 固定；Evidence 必须携带 system_as_of 和 authorization_ref。授权快照不代表当前访问授权。
- 公共 Schema 对象是摘要，模块字段级文档是完整字段权威清单；二者不一致时必须先同步文档，禁止实现方自行猜测。

本轮规则为规划建议，待各模块 Owner Review，不代表正式 JSON Schema 或 Go DTO 已实现。
