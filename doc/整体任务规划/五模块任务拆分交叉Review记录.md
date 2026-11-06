# 五模块任务拆分交叉 Review 记录

- 所属治理任务：GOV-004 / GOV-005
- 规划分支：feat/design_doc
- 日期：2026-11-05
- 状态：历史规划侧 Review 记录；不构成个人 Owner 批准或模块开工/冻结声明，当前语义与状态以项目级契约目录为准。
- 范围：五模块任务拆分与验收文档、首批 Application Port 与状态机契约。

## 1. Review 结论

原任务表存在依赖语义过粗的问题：领域开发、契约评审、Adapter 接入、真实环境和根组合验收混在同一个“依赖”列。若照原表执行，容易把 Data/News/Platform 生产实现或其他模块 CI 变成不必要的全局阻断。

本轮已修正五份任务拆分文档的依赖列，并补充分层规则。以后任务依赖必须标明属于哪一层：

- **Domain**：本模块领域对象、算法、状态机和内存实现依赖。
- **Contract**：跨模块 DTO/Port 的 Owner Review 与版本冻结。
- **Adapter**：调用外部模块、Provider、HTTP、Secret、Storage 的适配实现。
- **Integration**：根 DI/HTTP/Worker/Migration、真实依赖和端到端验收。

Domain/Contract 不得因为无关模块的 Adapter/Integration 未完成而被动等待。

## 2. 本轮修正清单

| 模块 | 原问题 | 修正 |
|---|---|---|
| Quant | QNT-003 强制依赖 QNT-002；QNT-013 等待 QNT-001~012 全部完成和 Platform Storage | Factor 不再强制依赖 Indicator；Repository Port/Schema 可并行，真实 DB/根注册单列 L3 |
| Research | RSC-002 直接依赖 News 契约；RSC-003/004 依赖 News/Data 生产能力 | Evidence/Event/实体关联先依赖自身 DTO 与 Fixture；News/Data Adapter 单独验收 |
| Researchflow | FLOW-004 直接依赖 Platform Task；FLOW-007 等待 Platform Storage | 自有 Executor Port 优先；Platform Task Queue 仅可选复用；Repository Port/Schema 与真实 Storage 分层 |
| LLM | LLM-002 把 Platform HTTP 作为接口开发前置 | Provider Port 和 Fixture 先行，Platform HTTP Adapter 单独验收 |
| Notification | NTF-001 依赖 Platform；NTF-004 依赖 Platform Task；NTF-006 等待 Storage | Event/Subscription/Delivery 和 Dispatcher 先独立；真实渠道、Worker、Storage 分别作为 Adapter/Integration 验收 |

## 3. 首批任务建议执行顺序

### Quant
1. QNT-001：冻结本模块最小计算上下文、Indicator/Factor 子契约。
2. QNT-002 与 QNT-003：在各自子契约明确后并行开发。
3. QNT-004 及 QNT-005/006/007 按数据输入边界并行推进。
4. QNT-008/009/010 按策略、回测、硬风险门禁顺序。
5. QNT-013 Repository Port/Schema 可提前设计；生产存储接入不阻断纯计算能力。

### Research
1. RSC-001 与 RSC-002 可并行细化；共享基础时间/来源/授权语义。
2. RSC-003 依赖 Evidence 结构稳定，不依赖 News 生产 Facade 已上线。
3. RSC-004 先固定实体引用契约，Data Adapter 独立推进。
4. RSC-005 可在 Context/Evidence 首批契约稳定后开始。
5. RSC-007 Repository Port/Schema 与领域开发并行设计；真实存储单独验收。

### Researchflow
1. FLOW-001~003 为第一批独立核心。
2. FLOW-004 自有 Executor Port 和有界执行语义评审通过后开始，不默认复用 Platform Task Queue。
3. FLOW-005/006 使用已冻结 DTO 或 Fixture 开发；真实 Facade 组合另验。
4. FLOW-007 Repository Port/Schema 可提前设计。
5. FLOW-009 根 HTTP/DI/Worker 接入必须由集成 Owner 统一处理。

### LLM
1. 立即执行 LLM-001 协议盘点。
2. 盘点后冻结 LLM-002 最小 Provider Port。
3. 超时、结构化输出、用量、脱敏等能力按真实协议分开实现。
4. Stream 只在 Gateway 真实支持时实现；多模型路由继续 P2。

### Notification
1. NTF-001 先冻结事件类型、Payload Schema、Subscription 范围和 Delivery 状态。
2. NTF-002 Renderer 与 NTF-001 并行准备 Schema/Fixture，冻结后完成集成。
3. NTF-003 真实渠道 Adapter 独立验收。
4. NTF-004/005 先实现模块内有界 Dispatcher 和重试状态机。
5. NTF-006 Outbox/Repository Port 与 Schema 可并行设计，真实 DB 和根 Migration 另验。

## 4. 发现的契约风险与需要补充的门禁

### 4.1 Quant

- QNT-003 Factor 的输入不应被限定为 Indicator 输出；因子可直接消费标准化行情/财务/分类输入。
- QNT-009 回测必须绑定数据快照、交易制度版本、费用模型版本和策略版本，不能仅以“历史行情”作为可复现条件。
- QNT-010 RiskDecision 必须区分 pass/reject/unknown；输入质量或规则缺失不得默认 pass。
- QNT-013 的领域 Migration 文件归 Quant；根 Migration 注册和执行入口归集成 Owner。

### 4.2 Research

- Evidence 的 source_ref 和 authorization_ref 均为不透明引用；不得由 Research 自行访问来源内部表。
- Research 的 system_as_of 必须由 Researchflow Run 统一传入；不能每个 Stage 自行取当前时间。
- Evidence 的授权引用不能作为永久授权凭证；真实读取必须由来源 Owner 执行当前撤销/过期检查。
- Event 合并/拆分必须保留来源 Evidence 与修订关系，不允许覆盖旧归并结果。

### 4.3 Researchflow

- Executor 必须限制并发、排队容量、单 Stage deadline 和总 Run deadline；不能只定义 DAG 拓扑。
- Attempt 是不可变执行记录；重试创建新 Attempt，不能重写失败记录。
- Run cancel 后不能启动新 Stage；正在执行的 Attempt 必须接收取消信号并记录真实结束状态。
- partial 必须说明缺失阶段与结果覆盖范围；不能仅返回一个布尔值。
- 硬风险门禁是不可绕过的阶段约束；LLM 结果不能改变 RiskDecision。

### 4.4 LLM

- LLM-001 已完成现有 Gateway 的静态源码/测试盘点：客户端主协议为 Responses，/v1/responses 支持 JSON/SSE 转发；本地 llama.cpp 会改写 instructions/input，且真实本地 E2E 尚未执行，不能将远程 Provider Mock 通过等同于当前部署验收。
- text.format 目前只是透传条件，Gateway 未做 Responses JSON Schema 语义保证；本地模型 structured output 仍待验证，LLM Adapter 必须本地 Schema 校验。
- Responses tools 可透传但 Gateway 不执行；模型返回 tool call 只是待执行意图，Researchflow 必须独立做白名单、参数 Schema、权限和预算检查。
- Responses usage 可解析 input/output tokens；流式审计能提取 usage，但 Responses 文本 delta 当前未形成统一内容 delta/TTFT 事件；LLM Adapter 需独立解析 Responses SSE。
- LLM-002 建议首批采用单轮文本 ModelRequest、typed OutputItem、nullable Usage、Responses SSE event callback 和显式 CapabilityState；复杂 input/multimodal 与多 Provider 路由延后。
- Prompt/Response 日志必须脱敏并限制保存；调用元数据与内容正文分开管理。

### 4.5 Notification

- Event Inbox 幂等与 Delivery 幂等是两道不同防线。
- 网络超时后无法确认渠道是否接收时，需有 unknown/needs_reconciliation 状态或等价机制，不能直接盲目重发。
- Outbox 只能保证本地事务与待投递事件原子性，不能承诺外部渠道 exactly-once。
- recipient_scope 不能直接充当 recipient_ref；接收人只能由已授权 Subscription/Channel 配置解析。

## 5. 仍然不能宣称通过的事项

- 所有 DTO/Port 仍待各模块 Owner Review；本轮规划 Review 不能代替 Owner 签字。
- Quant 数值精度、Indicator/Factor 首批参数对应 GOV-CTR-001、QNT-CTR-001～003；公共语义已定，Schema/Golden 仍为 pending_evidence。
- Research AuthorizationRef 与 system_as_of 分别对应 GOV-CTR-006/002、NEWS-CTR-002、RSC-CTR-001、FLOW-CTR-003；语义已形成基线，证据仍为 pending_evidence。
- Researchflow FLOW-CTR-001/002（历史别名 CONTRACT-006/007）的设计决议已由项目统一决策方关闭；Schema/Golden、传播矩阵自动测试、CAS/竞态/fencing 证据仍未完成。
- LLM-001 静态盘点已完成，当前部署本地 llama.cpp Responses JSON/SSE 在线 E2E、structured output 和工具调用能力仍待真实验收。
- Notification NTF-CTR-001～003 的语义已形成基线，Schema/Golden/状态迁移证据仍待补；首批跨模块事件映射 INT-CTR-001 仍为 pending_decision。
- 根 DI、HTTP、Worker、Migration 注册及真实 DB/渠道验收均属于 Integration，不在本轮完成。

## 6. 本轮文件变更

已更新：
- doc/quant/02-任务拆分与验收.md
- doc/research/02-任务拆分与验收.md
- doc/researchflow/02-任务拆分与验收.md
- doc/llm/02-任务拆分与验收.md
- doc/notification/02-任务拆分与验收.md

**最终结论：五模块任务拆分的依赖边界已按 Domain / Contract / Adapter / Integration 分层修正。模块仍处于 Design Ready；Owner Review 通过后按模块确认 Module Ready。没有理由要求五个模块等待其他业务分支同时编译。**


## 7. Quant / Research 字段级契约 Review（2026-11-05）

本轮细化内容属于历史评审建议，当前有效编号和状态以项目级目录为准：GOV-CTR-001/002 已 accepted；GOV-CTR-006、QNT-CTR-001～005、NEWS-CTR-002、RSC-CTR-001、FLOW-CTR-003 均按 pending_evidence 追踪。

| 决策 | 本轮建议 | 当前门禁 |
|---|---|---|
| GOV-CTR-001 / QNT-CTR-001 | 价格/金额 decimal string；计数 integer；比例/收益率等统计量 number；按 metric_code 固定 JSON Schema、精度和误差 | 公共语义 accepted；Quant Registry Schema/Golden pending_evidence |
| QNT-CTR-002 | 首批 SMA、EMA、Return、Volatility、RSI、MACD、ATR、Bollinger；每个指标独立 typed 参数 Schema | pending_evidence：补参数 Schema/默认值/范围 Golden |
| QNT-CTR-003 | 首批优先 momentum_20d、momentum_60d、volatility_20d；Factor 与 Indicator 解耦，横截面母体和覆盖率必须固定 | pending_evidence：补 Registry/算法 Schema/Golden |
| GOV-CTR-006 / NEWS-CTR-002 / RSC-CTR-001 | Evidence 保存统一不透明 AuthorizationRef；来源责任模块负责当前授权复核；历史授权快照不可覆盖当前决策 | pending_evidence：补统一 Schema、签发方映射和撤销竞态测试 |
| GOV-CTR-002 / FLOW-CTR-003 | ResearchRun 固定唯一 system_as_of；所有 Stage 和下游模块沿用，不允许重试时刷新 | 公共时间语义 accepted；传播/Adapter Golden pending_evidence |

同时修正 QuantMetric 原字段表中 value/bounds 的类型矛盾：数值类型由 metric_code 对应 Schema 固定；value 与区间边界按 required 字段显式输出 null 或有效值，禁止省略来隐藏未知状态。Quant 与 Research 的任务验收表已同步加入对应要求。

### 本轮仍需注意

- Indicator 八项是候选首批清单，不意味着八项都必须同时进入 MVP；Owner 可根据固定样本和业务优先级缩减。
- Factor 的首批三个指标建议优先，其他候选需先验证数据覆盖率和 PIT 可用性。
- decimal string 是 JSON 边界表达，不要求所有内部算法使用字符串运算；内部必须选择可证明精度的十进制/定点策略，统计算法可用浮点但必须给出误差预算。
- AuthorizationRef 是审计引用，不是访问令牌；历史允许不代表当前允许。
- system_as_of 是知识截止时间，不得伪造 observed_at，也不能替代 trade_date。

## 8. Researchflow 状态机契约 Review（FLOW-CTR-001/002；历史别名 CONTRACT-006/007，2026-11-05）

本轮对 Application Port、公共 Schema、Researchflow 设计和 FLOW-001~004 验收项进行一致性 Review，发现 Run 状态枚举存在三处不一致：旧设计文档使用 degraded/timed_out 作为 Run 终态，而首批 Port 与公共 Schema 使用 partial/failed。该差异会导致结果聚合、持久化枚举和 API 状态解释不一致。

### 8.1 决议建议

| 决策 | 统一建议 | 验收影响 |
|---|---|---|
| FLOW-CTR-001 | Run 只表达完成度：succeeded/partial/failed/cancelled；Stage/Attempt 承载 degraded/timed_out。Template 显式定义每种 Stage 结果传播策略 | 状态迁移/传播矩阵 Schema、Golden 和 Contract Test（pending_evidence） |
| FLOW-CTR-002 | Run 增加 cancelling 中间态；cancel/deadline/retry 使用 CAS 与绝对 deadline；在途 Attempt 收敛后才允许 Run 终态 | CAS/竞态/fencing/重启恢复 Contract Test（pending_evidence） |

### 8.2 本轮发现及修正

1. Run.degraded 混淆完成度和质量：一个 Run 可以完整完成但某个 Stage 输出质量降级；模板接受该输出时 Run 应 succeeded，同时结果质量标记 degraded。只有覆盖范围不完整才是 partial。
2. Run.timed_out 混淆单次 Attempt/Stage 预算耗尽和整个 Run 结果：Stage/Attempt timed_out 后按模板传播策略归并，不增加 Run.timed_out。
3. 原 Stage 状态遗漏 blocked，却在其他章节把 blocked 当作调度结果；现统一 blocked 为无 Attempt 的调度终态，skipped 仅表示模板裁剪/不适用。
4. 原取消语义允许 Run 终态时仍存在运行中 Attempt；现增加 cancelling，禁止新派发，等待在途执行收敛后才进入终态。
5. cancel 与正常完成竞争通过事务/CAS 决定先后；Attempt 终态只允许一次写入，迟到 Worker 通过 fencing token 防止覆盖。
6. 原 Executor 只提并发和 deadline，缺少队列容量、租约和重启恢复；现补入容量拒绝、持久化队列、租约回收与 fencing 约束。
7. 重试仅由 Researchflow Application 统一计数和决策，避免 Adapter/Executor 多层重试造成实际次数不可控。

### 8.3 FLOW-001~004 验收矩阵补充

- FLOW-001：Run/Stage/Attempt 枚举、termination_reason、deadline_at、传播策略、result_summary、lease/fencing 字段具有契约测试。
- FLOW-002：每个 Stage 的传播策略随 Template 版本化并参与 fingerprint；必需风险门禁禁止配置为可忽略失败/降级。
- FLOW-003：覆盖状态合法迁移、blocked/skipped 区分、失败重试新 Attempt、partial 聚合、Run cancelling、deadline、CAS 竞争与迟到结果拒绝。
- FLOW-004：覆盖容量满拒绝、并发上限、单 Run 并行上限、queued 恢复、租约过期回收、fencing token、单层重试和进程重启恢复。

### 8.4 历史待决项（已由第 17 节项目决议覆盖）

- Stage 传播策略是否采用本轮五项枚举，以及是否需要针对特定 reason_code 增加更细粒度规则。
- 硬风险门禁 Stage 的识别方式（Template 元数据标记或受控 stage_code Registry）；建议两者校验，不能仅靠自由文本。
- 取消宽限期、Worker lease/heartbeat 周期、队列容量与并发默认值，需结合部署资源压测确定，不在公共契约中写死数值。
- deadline 到达时 optional Stage 的传播策略是否默认 partial；建议默认 partial，Owner 可在明确无影响时配置 no_impact，但不能放宽 required/risk gate。
- 数据库 CAS、租约和 fencing token 的具体字段/唯一约束属于 FLOW-007 Repository/Schema 评审，当前为逻辑契约建议。

**历史结论已由第 17 节项目最终决议覆盖；实现、自动化契约测试和 L2 Module Ready 仍未完成。**


## 9. LLM-001 现有 Gateway 协议盘点（2026-11-05）

- 只读核验 Kasign/llm-gateway dev @ 3cd5aacced882143628c23ee00e06243616d1a5f；未修改 Gateway 仓库。
- 客户端协议确认采用 OpenAI Responses：POST /v1/responses；调用方 Gateway Key 使用 Authorization: Bearer 或 X-API-Key。Admin Key、Web Session 和上游 Provider Key 均独立。
- 本地路径会合并 instructions/system/developer，保留 tools，并在流式请求中补 stream_options.include_usage；远程 Provider 路径主要透传请求并重写服务端 model。
- Gateway 有 Responses JSON/SSE Mock 测试和 opt-in 真实 Provider E2E 测试；后者默认跳过。当前部署的本地 llama.cpp/Qwen 真实链路尚未验证。
- 本地 Responses SSE 中断后未发现专用 event:error 补发；Responses output_text.delta 也未被当前 Chat delta 审计解析器识别为内容/TTFT。Usage 的 response.usage 可解析。
- LLM-001 静态盘点完成、在线验收未执行，整体状态 partial。LLM-002 Port/Fixture 可继续；LLM-004/005 只能先做独立解析/校验准备，不能宣称真实模型能力已通过。
- 详细证据与测试矩阵见 doc/llm/03-现有模型Gateway协议盘点.md。


## 10. LLM-002 Provider Port 与 DTO 字段级 Review（2026-11-05）

- 首批协议建议固定为 OpenAI Responses，不做 Chat Completions/Anthropic 多协议统一抽象。
- ModelRequest 建议先采用 instructions + input_text 单轮文本；max_output_tokens、可选 temperature/top_p/reasoning_effort、response_schema、受控 ToolDefinition、deadline 和 correlation/trace metadata 均有明确字段。
- ModelResponse 以 status=completed/incomplete/failed 表达终态，按 OutputItem 类型解析 message/function_call/refusal/unknown；Usage 缺失保持 nil，不能补 0。
- Stream 建议采用同步事件回调提供背压；必须收到 terminal event，EOF 无 terminal 视为 stream_interrupted；首个事件后禁止透明重试。
- CapabilityState 使用 supported/unsupported/unknown，按 Provider + Model + Gateway 版本和验证日期记录；当前本地 Responses/Structured Output/Tool Call 真实能力均不能因 Mock 通过而标记 supported。
- ToolDefinition 是受控声明，LLM 只传输和解析；Researchflow 仍负责白名单、权限、预算和执行。
- 完整字段表、JSON Schema、错误分类、资源边界和 Owner 未决项见 doc/llm/04-ProviderPort与DTO字段级契约.md。
- 本轮仅形成 LLM-002 规划侧建议稿，待 LLM/Platform Owner Review；不代表接口冻结或模块开工。


## 11. LLM-002 Owner Review 深化（2026-11-05）

- 发现原 Stream Port 只返回 error，没有交付 terminal 聚合后的 ModelResponse；Generate 也没有区分合法上游 failed/incomplete envelope 与 Adapter/transport error。建议统一为 Generate/Stream 返回 (ModelResponse,error)，Stream 通过同步 callback 提供事件并返回最终聚合响应。
- 七项建议决议：MVP 单轮文本；ToolDefinition 纳入但默认禁用、只在能力 supported 时允许 auto；reasoning_effort 保留可选但默认关闭；本地 Schema 使用 Draft 2020-12、上游 strict 子集独立验证；Usage.total_tokens 有条件派生；SSE 序号严格校验；request/correlation/Gateway/upstream 四类 ID 分离。
- SSE 若全流提供 sequence_number，必须连续递增；缺号/重号/倒退不能标记 completed。若整流均不提供可继续但记录序号能力 unavailable；混合有无按协议异常。
- ModelResponse.status 表达上游合法终态；Go error 表达传输、HTTP、协议、Schema、callback 和 Context 错误。无 terminal EOF 返回部分 Response + stream_interrupted。
- 以上为规划侧建议，待 LLM/Platform Owner Review；不构成签字冻结。详细字段和决议见 doc/llm/04-ProviderPort与DTO字段级契约.md。


## 12. LLM-002 JSON/SSE Fixture 设计

- Fixture 不以 Mock 通过代替真实上游能力验证；其目标是锁定 Adapter 的输入解析、归一化输出、错误和事件语义。
- JSON 必须覆盖 completed/incomplete/failed、message/function_call/refusal/unknown item、Usage nullable/derived/mismatch、Schema invalid 和结构异常。
- SSE 必须覆盖 terminal、unknown event、sequence 连续/缺号/重复/倒退/混合缺失、断流、callback error 和取消。
- 每个 fixture 应断言 wire 原文、DTO、终态、错误类型、usage.source、sequence 诊断及可否认定 completed。
- 样例和建议命名见 doc/llm/05-JSON与SSE契约Fixture清单.md；样例是契约测试设计，不代表本地 llama.cpp/Qwen 已支持对应能力。


## 13. LLM-002 Stream Tool Call 事件覆盖补充

- 复核发现 ToolDefinition 已纳入受控 MVP，但原 ModelEvent 首批事件列表只覆盖文本 delta，缺少 function_call 的 output item 与 arguments 增量事件，导致 Streaming Tool Call 契约不完整。
- 已补充 response.output_item.added/done、response.function_call_arguments.delta/done、response.content_part.added/done、response.refusal.delta/done；ModelEvent 增加 item/content_part/arguments 字段。
- 新增 stream_function_call.sse Fixture，要求按 item_id/call_id 聚合参数增量，并校验最终 arguments 与 delta 聚合一致；仍只产出待授权 ToolCall，不执行工具。
- 本地模型是否支持这些事件仍由真实 E2E 决定；Fixture 通过不能把 function_tool_calls 或 responses_stream 设为 supported。


## 14. Notification NTF-CTR-001～003 字段级 Review（历史别名 CONTRACT-009/010，2026-11-06）

### 发现
- 旧 DeliveryRecord 仅列 queued/sending/sent/failed/skipped，未完整表达 retry_wait、blocked 和外部结果未知；与 Outbox/租约恢复要求不一致。
- 幂等键只有“event_id + subscription_version + channel_ref”的概念描述，未拆分 Event Inbox 去重、PublishEvent 命令重放和 Delivery 唯一键。
- Subscription/Channel/Template 版本快照与退订后发送前复核未形成规则，事件 fan-out 与配置变更存在竞态。
- DeliveryAttempt、租约过期、fencing、RetryDelivery 权限和 unknown 对账缺少可测试迁移规则。
- Payload Schema 有 event_type + schema_version 约束，但注册不可变、兼容策略、模板变量 allowlist 尚未完整定义。

### 建议决议
- Event Inbox 唯一键 `(source_module,event_id)`；PublishEvent 幂等键 `(source_module,idempotency_key)`；相同 fingerprint 重放返回原结果，不同 fingerprint 冲突。
- Delivery 唯一键包含 event、subscription/version、channel/config version、recipient_ref；DeliveryPlan 固定模板版本和目标引用。
- 状态集合增加 retry_wait、blocked、unknown。unknown 不得盲目重试；过期租约若无法证明请求未发出则进入 unknown，迟到 Worker 受 fencing token 拒绝。
- 每次实际渠道调用追加 Attempt；明确不承诺外部 exactly-once。渠道幂等键稳定跨 Attempt；不支持幂等/查询的 ambiguous outcome 进入对账。
- 退订会抑制尚未发送的 queued/retry_wait；sending 中的外部副作用不承诺可撤回。
- Schema 注册键 `(event_type,schema_version)`，注册后不可原地修改；模板绑定明确版本和变量 allowlist。

详细状态机、字段、唯一键、Port 和验收映射见 `doc/notification/03-ApplicationPort与投递状态契约.md`。上述为规划建议，待 Notification/Platform/事件 Owner 签字。


## 15. Notification Fixture 与验收覆盖补充

- 新增合成 Event JSON Golden、Inbox/Publish 幂等冲突、Subscription/recipient 变更、Delivery 状态迁移、unknown 对账、RetryDelivery、租约/fencing、Outbox/Inbox 恢复和 Channel Fake Adapter 场景。
- 明确断言外部调用次数、Attempt 历史、幂等键、状态版本和敏感信息脱敏；不能只断言最终 status。
- 真实渠道行为、MariaDB 事务和 Worker 崩溃恢复属于 L3，Fake Fixture 通过不代表真实集成通过。
- 详细清单见 `doc/notification/04-契约Fixture与状态迁移验收清单.md`；仍待 Owner Review。


## 16. 五模块公共契约一致性 Review（2026-11-06）

1. QuantMetric 公共摘要缺少 metric_status、sample_count、lower_bound、upper_bound；已补齐，数值类型由 metric_code Schema 固定。
2. Evidence 公共摘要使用 authorization_status，而字段级 DTO 使用 authorization_ref/EvidenceAuthorizationRef；已统一为 authorization_ref，并补 system_as_of、limitations、content_fingerprint。NewsObjectRef 的 authorization_status 仍可表达 News 来源状态，但不能替代 Evidence 授权审计引用。
3. ResearchEvent 与 NotificationEvent 是不同领域对象/集成消息，ID、版本和生命周期不可复用；转换必须显式映射。
4. ResearchRun.as_of 是唯一系统知识截止值；下游 as_of 与 Evidence/News system_as_of 在该 Run 语境中表达同一值；observed_at 不得回填。
5. Platform Fault 与 LLM ModelErrorInfo 增加显式映射；request_id 只保留在 ModelResponse 层，Cause 不对外暴露。
6. quality_status、Quant result_status、Researchflow 状态、ModelResponse.status、Notification Delivery.status 分域管理，不按同名字符串直接转换。
7. request_id/correlation_id/event_id/idempotency_key 分工明确；幂等使用 Owner 定义的作用域与 fingerprint。

本轮未发现需要改变五模块依赖方向的架构冲突。字段级差异已在规划文档修订；Owner 签字、正式 JSON Schema、Go DTO/golden 和契约测试仍未完成。GOV-004 继续 NO-GO。


## 17. FLOW-CTR-001/002 项目最终决议（历史别名 CONTRACT-006/007，2026-11-06）

本节取代第 8 节中“待 Researchflow Owner 确认”的历史状态。项目不存在独立模块 Owner；由项目统一决策方形成决议。设计决策关闭，代码实现与自动化验收仍未完成。

- FLOW-CTR-001：Run 只表达完成度；Stage 增加 retry_wait；Template 固定传播矩阵和有限 retry_policy；硬风险门禁使用 Registry + gate_type 双重标识；RiskDecision reject/unknown 是有效业务结果但 actionability blocked；门禁未执行或执行层失败必须 Run.failed。
- FLOW-CTR-002：created/queued 无运行中 Attempt 时可直接取消；running 进入 cancelling；数据库 UTC 时钟决定 deadline 优先级；deadline 将未启动 Stage 标为 timed_out，在途 Attempt 停止/fence 后 timed_out；Run 终态必须无活动 Stage/Attempt，并以事务/CAS 固化。
- 租约 fencing 仅阻止本地迟到写入；外部副作用必须使用下游幂等键，ambiguous outcome 不自动重试。
- 详细决议与验收矩阵见 `doc/researchflow/03-CONTRACT-006与007最终决议.md`（文件名保留历史编号；正文契约编号为 FLOW-CTR-001/002）。

本决议不代表 FLOW-001~004 已实现或测试通过，也不单独使 GOV-004/L2 Module Ready 通过。
