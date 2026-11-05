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
