# 跨模块契约样例与 Golden 验收矩阵

- 权威维护分支：`feat/design_doc`
- 配套目录：[跨模块契约目录与冻结状态](./跨模块契约目录与冻结状态.md)
- 状态：Golden 设计输入；以下案例是必须实现的测试场景，不代表测试代码已存在或已通过。
- 适用原则：每个契约至少覆盖正常、边界、异常、兼容四类案例；字段级 DTO 还必须补充 required/optional/nullable、零值、空数组和未知枚举案例。

## 1. 公共基础契约

| 契约 | 正常样例 | 边界样例 | 异常样例 | 兼容样例 |
|---|---|---|---|---|
| GOV-CTR-001 类型/精度 | `price="123.4500"`、`volume=1000`、`return=0.0125 ratio` | `change_percent=1.25 percentage_point` 与 `return=0.0125 ratio` 表示同一百分比 | Decimal 出现 `1.2e3`、千分位、NaN/Inf；必须拒绝 | 新增 optional 字段可兼容；将 price 从 decimal string 改 number 必须升 MAJOR |
| GOV-CTR-001 空值 | required+nullable 字段存在且值为 null | 空数组 `[]` 表示已评估且无成员 | 把 unknown/null/缺失/0 混成一个状态；拒绝 | optional 字段缺失仅在 Schema 明确允许时接受 |
| GOV-CTR-002 时间/PIT | `system_as_of=2026-11-06T01:00:00Z`，读取不晚于该时点可见的版本 | 交易日 `2026-11-06` 按 Asia/Shanghai；available_at 同日但无精确时刻时严格 PIT 排除 | 用 published_at 或 fetched_at 冒充 available_at；拒绝 | Data v1 查询参数 `as_of` 由 Adapter 显式映射到 system_as_of |
| GOV-CTR-003 ID | X-Request-ID 通过入口校验并传播；Worker 生成自己的 request_id | 128 字节合法 Header 接受；129 字节或重复 Header 由入口重生成 | correlation_id 被当 event 去重键；拒绝 | 新增 trace_id/span_id 不改变现有 request_id 语义，需单独版本/契约 |
| GOV-CTR-004 Schema 版本 | 已注册 schema_version 精确解析 | 新增 optional 字段的 MINOR 版本由旧消费者按未知可选字段策略处理 | 未知 MAJOR、未知必需枚举不得回退旧 Schema | 旧版本在迁移窗口内继续解析，废弃至少两个兼容发布周期 |
| GOV-CTR-005 冻结门禁 | Schema+Golden+Contract Test+消费者映射齐备后 frozen | 语义 accepted 但 Golden 未齐时保持 pending_evidence | 仅文档写“已完成”即标 frozen；拒绝 | 证据路径迁移需保留旧证据引用和新路径映射 |
| GOV-CTR-006 AuthorizationRef | [机器契约/Schema/四方 Golden](./GOV-CTR-006-AuthorizationRef机器契约与Golden.md)；allowed/denied/conditional/unknown、Principal/Resource、PolicySnapshot、JCS 指纹 | 22 项 Schema/Fingerprint/Owner/Read/Race 测试矩阵已定义；conditional 不可直接读取；expires_at=null 仍需当前复核；多策略快照必须完整 | 用历史 allowed 跳过当前撤销/过期检查；漏记实际生效策略；把 conditional 当 allowed；仅做 JSON 解析不做业务断言 | L1 Schema/Golden + L2 四方 Owner/消费者/并发安全测试均有 CI 证据后才能 frozen；新增策略追加快照条目；字段或 fingerprint 输入变更升级 Schema MAJOR |

## 2. Platform 技术原语

| 契约 | 正常样例 | 边界样例 | 异常样例 | 兼容样例 |
|---|---|---|---|---|
| PLAT-CTR-001 Fault | `class=unavailable`、稳定 code、safe_message、dependency、retryable=false | dependency 缺省；Cause 仅内部保留 | forbidden/validation 被标 retryable=true；或 Cause/DSN 泄漏到 JSON | 新增稳定 code 不改变既有 code；改变 class 语义需 major |
| PLAT-CTR-002 Request Context | X-Request-ID 入口生成、响应回显、内部调用传播 | 合法最大长度 128 字节 | 重复 Header、非法字符；生成新 ID，不选择其中一个 | correlation_id 增加不覆盖 request_id |
| PLAT-CTR-003 身份/Secret | owner token 访问 owner route；service Secret 仅访问声明 Scope | token 32/256 字节边界；常量时间比较 | 无凭据、错误凭据、service 获得 platform:admin、Secret 被日志打印 | Provider Token、Data API Key、Platform Service Secret 彼此独立，不互相自动兼容 |
| PLAT-CTR-004 Diagnostics | healthy Observation，无 reason_code | non-healthy 必须有 reason_code；attributes 16 项/2048 字节边界 | 未知 status、超长字段、敏感内容进入 attributes | 领域新增 reason_code 不要求修改 Platform 全局枚举 |
| PLAT-CTR-005 HTTP/SSRF | 精确 allowlist Host + 公网 IP + 受控 Dial 成功 | DNS 返回多个地址时逐个校验；允许的有界 Redirect 逐跳校验 | 任一解析结果命中私网/loopback/link-local/reserved、DNS rebinding、代理绕过；拒绝 | 新增 DestinationPolicy 字段只能显式默认，不改变旧安全策略 |
| PLAT-CTR-006 Storage/Migration | namespace 内按版本顺序执行 Migration | 已执行历史记录是注册清单的有序前缀 | 修改已发布 Migration、缺版本、锁超时；停止启动 | 新 namespace 独立版本空间，不改变其他模块 Migration |
| PLAT-CTR-007 Config/Secret | Bootstrap 装载配置并校验后注入不可变快照 | optional 配置缺省应用文档声明的默认值 | 必填缺失、范围错误、原始 Secret 出现在错误/日志 | 新增配置项必须声明默认值和生效条件；旧配置不被静默改义 |

## 3. Data 契约

| 契约 | 正常样例 | 边界样例 | 异常样例 | 兼容样例 |
|---|---|---|---|---|
| DATA-CTR-001 API/DTO | OpenAPI 与 Go JSON Tag、Handler Golden 一致 | required nullable、omitempty、空 slice 分别验证 | Schema 声明字段但 Handler 无来源；或 Go 必出字段未列 required | v1 已发布字段不静默改名/改类型 |
| DATA-CTR-002 单位/复权/PIT | 股票价格 CNY/股、成交量 shares、成交额 CNY；明确 adjustment_basis | percentage_point 1.25 映射 Quant ratio 0.0125 | 指数成交额无单位却强制当 CNY；PIT 缺因子时回退当前复权 | v1 none/forward_adjusted/backward_adjusted 参数保持原义 |
| DATA-CTR-003 Provider 标准化 | 原始数字词法→Decimal→标准 DTO，源精度可追溯 | 源数据精度声明低于目标精度，标记 degraded/拒绝精确消费 | float64 中转后 FormatFloat 伪造无损 Decimal | Provider schema_version/normalization_version 变化有独立 Fixture |
| DATA-CTR-004 Quality | valid 且 coverage/source/time 完整 | empty result + coverage unknown 保持 unknown | 把空结果直接转成 0 或 valid | 增加新 reason_code 不改变既有质量状态语义 |
| DATA-CTR-005 精度 v2 | `close="123.4500"`、`amount="123456.78"`、`volume=100000` | volume 达 JS safe integer 上限 | v1 float64/number 被 Quant 当成精确输入；拒绝 | v1 保持 number 兼容；v2 新增 Decimal DTO，不破坏 v1 |
| DATA-CTR-006 Data API Auth | Server Adapter 发送一个 X-API-Key 或 Bearer 凭据 | Key 轮换后新值成功、旧值失败 | 两种 Header 同时存在且冲突、空 Key、日志泄漏 | Data API Key 不与 Platform Service Secret 自动互换 |

## 4. News → Research 契约

| 契约 | 正常样例 | 边界样例 | 异常样例 | 兼容样例 |
|---|---|---|---|---|
| NEWS-CTR-001 Evidence Query | caller_ref/purpose/content_level/system_as_of/filter/page_size 完整，返回固定快照页 | page_size=1/100；空结果 items=[]、next_cursor=null | 任意 URL/SQL/排序字段、未知 purpose/content_level；validation Fault | 新增可选 filter 字段不改变现有 filters 语义 |
| NEWS-CTR-002 AuthorizationRef | allowed 快照关联 policy_version/subject_ref/fingerprint，当前读取重新授权 | expires_at=null 表示无固定过期，不代表永久放行 | 历史 allowed 但当前 revoked/expired/unknown；拒绝受限正文 | 新增 condition_code 不改变既有 decision 枚举 |
| NEWS-CTR-003 PIT/Cursor | 同一 snapshot_ref 多页返回稳定排序 | published_at 同时刻时由唯一排序键稳定续页 | Cursor 篡改/过期/过滤指纹不一致；明确失败 | sort_version 升级时旧 Cursor 不被新排序解释 |

## 5. Quant / Research 契约

| 契约 | 正常样例 | 边界样例 | 异常样例 | 兼容样例 |
|---|---|---|---|---|
| QNT-CTR-001 Registry | code+schema_version 固定 value_type/unit/precision/algorithm/input/quality | lower_bound/upper_bound 同类型且同时存在或同时 null | 未注册 code、未知 schema_version、NaN/Inf；拒绝 | 算法升级新增 algorithm_version，不覆盖历史结果 |
| QNT-CTR-002 Indicator | sma(window=20) 输入满足 20 个有效 close | 恰好 window 个有效样本 | window 缺失/越界、复权口径混合；拒绝 | 新增 Indicator 不改变既有 code/参数 Schema |
| QNT-CTR-003 Factor | momentum_20d 以 21 个有效复权 close 计算 ratio | 只有 20 个 close 返回 insufficient_data | 把缺失样本当 0 或把 percentage_point 当 ratio | 新因子以新 factor_code 注册，不能复用旧 code 改算法 |
| QNT-CTR-004 QuantResult | result_ref、status、coverage、sample_count、metric_schema 可追溯 | 区间上下界同时 null | RiskDecision unknown 被转换 pass | 新增 optional result metadata 不改变旧 metric 类型 |
| RSC-CTR-001 Evidence | fact/reported_fact/inference/hypothesis 类型、来源版本、system_as_of、authorization_ref 完整 | available_at unknown 时保留限制 | 未授权正文复制进 Research/LLM/WebUI | 历史 Evidence 保留旧 authorization_ref，当前读取仍重评授权 |
| RSC-CTR-002 Event/Claim/Scenario | 追加式版本、正反证据和失效条件齐全 | supporting/contradicting refs 可为空数组 | 用新版本覆盖历史版本、Hypothesis 缺验证条件 | 新增 event_type 不改变旧版本语义 |

## 6. Researchflow 契约

| 契约 | 正常样例 | 边界样例 | 异常样例 | 兼容样例 |
|---|---|---|---|---|
| FLOW-CTR-001 状态/传播 | 所有 Stage 终态且无活动 Attempt 后归并 Run | optional skipped/no_impact 不降低覆盖；required skipped 不允许 | retry_wait 参与 Run 归并、Run 有活动 Attempt 进入终态 | 新增 Stage 结果枚举必须同步传播矩阵和 Template version |
| FLOW-CTR-002 Cancel/Retry/Fencing | Cancel/claim/retry 通过 Run.version CAS 线性化 | db_now==deadline_at 时 deadline 生效 | 旧 fencing_token 写入、cancelling 后创建新 Attempt；拒绝 | Template 新版本可调整策略，历史 Run 保留旧快照 |
| FLOW-CTR-003 system_as_of | Run 固定 system_as_of 并传给所有下游 | 重试仍使用原 system_as_of | Stage 重新取当前时间、用 deadline_at 替代知识截止；拒绝 | Data API as_of 仅由 Adapter 显式映射 |
| FLOW-CTR-004 Stage Output | Orchestrator 写 envelope，Provider 返回 typed payload/quality/diagnostic refs | payload 合法但 quality degraded | Provider 伪造 run_ref 或用业务 reject 包装成 Platform Fault | payload schema_version 升级不改变 envelope 状态语义 |

## 7. LLM / Notification 契约

| 契约 | 正常样例 | 边界样例 | 异常样例 | 兼容样例 |
|---|---|---|---|---|
| LLM-CTR-001 Request/Response | Responses completed，Usage 明确 | Usage 缺失为 unknown/null，不补 0 | 非法 output item/Schema 错误 | 新 output item 保留未知类型和顺序，不执行副作用 |
| LLM-CTR-002 Stream/SSE | 收到 terminal event 后返回最终聚合 Response | 全流均无 sequence_number 时标记能力 unavailable | EOF 无 terminal、sequence gap、重复/倒退序号 | 新 SSE event 作为 UnknownEvent 保留类型和顺序 |
| LLM-CTR-003 Fault Mapping | Platform Fault 映射 stable_code/kind/retryable/safe_message | dependency 只进受控诊断元数据 | Cause/原始上游响应进入 ModelErrorInfo | 新 Fault code 映射为 unknown-safe kind，不默认重试 |
| NTF-CTR-001 Event Schema | event_type+schema_version 命中不可变注册表 | optional payload field 缺失按 Schema 处理 | 未注册版本、同版本 schema_digest 改变；拒绝 | 新事件语义使用新 schema_version |
| NTF-CTR-002 Idempotency/Delivery | 相同 key+fingerprint 返回原结果 | 同一事件重复投递不重复 fan-out | 同 key 不同 fingerprint 冲突；unknown 盲目重发；拒绝 | 新 Delivery status 必须定义状态迁移与兼容策略 |
| NTF-CTR-003 Channel Retry | 明确未接受且 retryable 的错误进入 retry_wait | Retry-After 与总 deadline 取更严格边界 | 外部请求可能已接受但回执丢失，标 unknown | 渠道新增幂等能力需单独能力版本声明 |

## 8. Server / Integration 契约

| 契约 | 正常样例 | 边界样例 | 异常样例 | 兼容样例 |
|---|---|---|---|---|
| SRV-CTR-001 Composition/Lifecycle | 单一 Bootstrap 创建共享 DB/Redis/HTTP Client，显式 Migration 后启动 Handler/Worker | Worker drain 达到 deadline 后按 fencing 撤权 | 模块重复创建共享连接、Migration 部分失败仍启动 Worker | 新模块通过构造函数注入依赖，不改变既有资源所有权 |
| INT-CTR-001 Quant/Research/Researchflow→Notification | Run 终态、RiskDecision 实质变化、ResearchEvent 生命周期迁移分别映射到固定 event_type/schema_version；NotificationEvent.event_id 独立生成 | 同一领域版本重放保持 event_id/idempotency_key；风险多主体按 subject_ref 拆分 | 把领域 ID 直接复用为 NotificationEvent.event_id；重复状态触发重复通知；Notification 反推业务事实 | 4 份 Schema 与 6 个 Golden 已提交并通过本地 Draft 2020-12 Validator；模块 CI、重放/冲突/回滚 Contract Test 未完成；新增事件/语义必须新增 Schema 版本和映射决议 |

## 9. 使用与冻结规则

1. 上述案例转为各模块可执行 JSON/SSE Fixture、Schema Validation 和 Contract Test；描述性文字本身不是测试证据。
2. 每个案例必须断言字段值、状态、错误码、幂等结果和副作用边界；不能只验证 JSON 可解析。
3. 兼容样例必须由生产方和直接消费者共同运行；未知字段/枚举的策略必须显式测试。
4. 每个契约至少四类案例齐备并通过，才可将相应契约推进到 `frozen`；真实 Provider、数据库、网络、Worker、模型和通知渠道仍按 L3 验收。
5. 当前本文只是测试矩阵草案；所有案例尚未被统一声明为已执行或通过。
