# GOV-004 跨模块统一契约基线与整体 Review

- 所属任务：GOV-004（项目级跨模块契约治理）；Platform 子契约对应 PLAT-002
- 文档归属：feat/platform-foundation / doc/platform
- 审查基线：dc85a9da4881b74da3c010c164369114e438c4a4
- 文档状态：Platform 侧跨模块 Review 与接入索引；项目级契约状态以 `feat/design_doc` 的契约目录为准；**不是全局冻结声明**
- Review 类型：规划文档静态交叉审查；不等同于当前所有业务分支的最新代码审计
- 适用范围：Data、News、Quant、Research、Researchflow、LLM、Notification、Platform、Server/API、WebUI 的服务端协作边界

## 1. 文档定位与权威关系

本文件把已有总规划和专项契约串成一套可执行的契约基线，负责统一解释、交叉约束、未决决策和冻结门禁；不复制每个模块的完整 Schema，也不取代模块自己的权威文档。

文档权威顺序：

1. 项目目标、模块职责、分支策略、阶段路线和全局门禁：[整体任务规划总览](../整体任务规划/整体任务规划总览.md)。
2. 项目级 GOV-004 冻结状态及阻断项：[GOV-004 最终 Review 与契约冻结门禁](../整体任务规划/GOV-004最终Review与契约冻结门禁.md)。
3. Platform 公共技术原语：[08-公共平台契约规范](08-公共平台契约规范.md)。
4. Data API 与 JSON 序列化：[Data-OpenAPI与Go模型契约修正清单](../整体任务规划/Data-OpenAPI与Go模型契约修正清单.md) 及 doc/api 下实际 OpenAPI。
5. News 对 Research 的只读证据查询：[News-Research只读证据查询契约](../整体任务规划/News-Research只读证据查询契约.md)。
6. Quant / Research DTO：[Quant-Research公开DTO字段级契约](../整体任务规划/Quant-Research公开DTO字段级契约.md)。
7. 五模块首批 Application Port、状态机和调用关系：五模块首批ApplicationPort与状态机契约细化。
8. 各模块后续字段、算法、Provider、Migration 和测试细节：各自模块专项文档。

发生冲突时，不以本文件中的概括性文字覆盖更具体的领域契约；应登记 Decision ID，确定生产方和消费方后修订权威文档。Platform 分支只维护 doc/platform；项目级总规划文档仍由规划文档 Owner 维护。本文件的定位是 Platform 侧跨模块 Review 和接入索引。

## 2. Review 总结

### 2.1 已形成一致的架构基线

- Data、News、Quant、Research、Researchflow、LLM、Notification 各自拥有领域事实和领域规则。
- Platform 提供技术原语，不拥有业务领域模型、Repository、业务状态机或领域诊断历史。
- Researchflow 是五模块中唯一跨业务模块的编排者；Server/API Bootstrap 负责技术装配，不承接研究业务编排。
- Quant 与 Research 不互相依赖；Research 不 import Quant。Quant 结果由 Researchflow 通过公开 Port 获取并装配为 Research 所需输入/证据。
- Notification 不依赖 Researchflow；上游通过版本化、与业务领域解耦的 Notification Command/Event 调用。
- LLM 不执行 Tool Call；工具授权、预算、执行和结果回填由 Researchflow 负责。
- Data 是独立 Go Module，通过 HTTP/OpenAPI 和本地 Adapter 对接；禁止跨 Module import 根模块 internal。
- News 的 Research Evidence Query 是同一根 Go Module 内的 News Application Facade，不新增 News HTTP 服务，也不把 News Repository 暴露给 Research。
- 根 DI、HTTP 路由、Worker 启动、Migration 总注册和部署入口由 Server/集成组合根集中装配；模块分支不抢占共享入口。
- MariaDB 保存事实；Redis 仅作缓存或短期辅助，不作为唯一事实来源。

### 2.2 本轮发现的主要治理问题

| 编号 | 问题 | 影响 | 本文件处理 |
|---|---|---|---|
| GOVR-01 | 总规划、GOV-004、Platform、News、Quant/Research 契约分散 | 同一语义可能被重复定义或发生漂移 | 建立权威文档层级与引用关系 |
| GOVR-02 | Research 消费 Quant 结果的描述容易被误读为直接依赖 | 造成 Go import 反向依赖 | 明确只能由 Researchflow 装配 |
| GOVR-03 | Notification 与 Researchflow 的依赖关系存在文字误读风险 | 可能引入不必要的模块依赖 | 明确通用版本化 Command/Event |
| GOVR-04 | 平台通用 Task、Trace、Storage 抽象容易被过度扩展 | 把平台做成万能框架 | 按现有 Platform 最小契约收敛 |
| GOVR-05 | GOV-004 契约冻结与真实生产集成门禁混在一起 | 模块无法独立开工，门禁范围过宽 | 拆分 L2 契约就绪与 L3 集成就绪 |
| GOVR-06 | 规划草案中的字段示例容易被误认为实现或冻结 Schema | 下游按未确认字段编码 | 统一状态标签和冻结证据要求 |
| GOVR-07 | Data API Key、Platform Service Secret、Owner Token 容易混淆 | 服务间认证和密钥注入错误 | 明确凭证分离和 Server Adapter 注入 |
| GOVR-08 | available_at、observed_at、fetched_at 等时间容易被互相推导 | 产生 PIT 未来数据泄漏 | 明确时间字段语义和 unknown 处理 |

### 2.3 当前结论

架构职责和依赖 DAG 已形成可用设计基线；字段级 Schema、部分 Application Port 和状态机仍有证据缺口。未决语义与待补证据以项目级契约目录逐项登记；Data OpenAPI/Go JSON 差异、News 查询/PIT/授权/分页实现、Platform 生产集成仍需按实际调用点验收。

因此当前状态仍是 **GOV-004 NO-GO / 未冻结**。本文件形成候选契约与审查结论，不把文档存在等同于契约冻结。

## 3. 模块所有权与依赖契约

### 3.1 领域事实所有权

| 模块 | 唯一权威所有权 | 不得接管的职责 |
|---|---|---|
| Data | Security Master、行情、财务、估值、交易日历、行业分类、PIT、数据来源与数据质量 | 研究语义、Quant 算法、News 事件解释 |
| News | Source/Endpoint/Provider、授权、采集、Observation、News Item Identity/Version、关系断言、内容引用和新闻查询 | Research Evidence/Event、Quant 结论 |
| Quant | Indicator、Factor、Trend、Market/Industry/Stock Analysis、Strategy、Backtest、Risk、Evaluation | 数据采集、新闻采集、研究语义、流程调度 |
| Research | Context、Evidence、Event、Claim、Hypothesis、Scenario | 新闻原文/授权事实、Quant 算法和数值计算、流程状态机 |
| Researchflow | Run、Stage、Attempt、Workflow Template、DAG、Context Assembly、Tool Policy、结果校验/综合、恢复 | Quant 算法、Research 领域事实、LLM 协议、通知渠道 |
| LLM | Model Request/Response、Provider Adapter、Structured Output、Stream Event、Usage/Invocation Metadata | Tool 权限与执行、研究状态、数值事实、风险门禁 |
| Notification | Subscription、Template、Channel、Dispatcher、Delivery、DeliveryAttempt、幂等与重试 | 告警条件计算、业务规则和研究状态 |
| Platform | Auth、Config/Secret 原语、Fault、Request ID、slog 约定、HTTP Client、Storage Bootstrap、Diagnostics Runner | 领域模型、领域 Repository、领域质量规则、Researchflow 状态机 |
| Server / 集成根 | Bootstrap、依赖注入、HTTP 路由/Middleware、Worker 注册、Migration 总注册、服务生命周期 | 领域规则、算法和跨模块业务决策 |
| WebUI | 页面、交互、API 消费和结果展示 | 服务端事实、质量判断、权限决策、计算和状态迁移 |

### 3.2 Go 依赖 DAG

允许的主要依赖：

- Quant → Platform；Quant 的数据输入经 Data HTTP/OpenAPI Adapter。
- Research → Platform；Research 的来源证据经 News Application Facade。
- Researchflow → Platform + Quant/Research/LLM/Notification 的公开 Application Facade；Data/News 输入通过明确的 Adapter/Facade。
- LLM → Platform。
- Notification → Platform。
- Server/Bootstrap → 各模块公开 Application Facade + Platform。
- WebUI → 已发布 HTTP/OpenAPI，不 import Go 包。

明确禁止：

- Platform → 任一业务模块。
- Quant ↔ Research。
- Research → Quant 或 Researchflow Go 包。
- Quant / Research / LLM / Notification → Researchflow。
- LLM → Quant、Research、Researchflow Domain。
- Notification → Researchflow 或具体业务 Domain。
- 任一模块访问其他模块的私有 Domain、Repository、SQL、Migration 实现或 Adapter。
- 根 Go Module import Data 独立 Module 的 internal 包。
- 通过 Platform 建设共享万能 Domain、Repository、Task Framework 或动态工作流框架。

### 3.3 业务调用与 Go import 必须区分

业务上需要 Quant 结果，不代表 Research 可以 import Quant；由 Researchflow 调用 Quant Facade，再把经过版本、质量和时间校验的结果转换成 Research 自己的 Evidence/Context DTO。

业务上需要发送通知，不代表 Notification 依赖 Researchflow；调用方通过 Notification 自有、版本化的 Command/Event DTO 发起请求。

API/Worker Bootstrap 只装配 Researchflow Facade 和必要 Platform 能力，不直接拼接 Quant/Research/LLM/Notification 私有实现。

## 4. 跨模块 DTO 与 Schema 通用规范

### 4.1 DTO 边界

- 模块公开边界只暴露 Application Port 和公开 DTO。
- Domain Entity、Repository、SQL Model、Provider SDK 类型和内部 Adapter 均为私有。
- 同一进程内调用也必须遵守公开 DTO 语义，不因同一 Go Module 直接传递对方 Domain Entity。
- Transport DTO、Application DTO、Domain Model 分层；HTTP Handler 不应把内部 Repository/SQL Model 当作公共 API。
- 不建设无边界的 common/shared 包；共享内容仅限稳定、最小、版本化的基础值和不透明引用。
- 不允许任意扩展 map[string]any 作为跨模块主 DTO；需要扩展时使用有命名空间和 Schema 版本的受控对象。

### 4.2 字段存在性

Schema 必须区分：

- required：响应/请求中必须出现；
- optional：允许缺省；
- nullable：字段出现时允许 null；
- empty：空字符串、空数组或空对象的合法性由字段逐项声明。

禁止把 unknown、missing、not_configured、not_supported、degraded、invalid 统一折叠成 0、false、空字符串、null 或成功状态。

Go JSON Tag、omitempty、指针、slice/map 的运行时输出不能单独决定 Schema。每个公开 DTO 必须有 JSON Schema/OpenAPI、Go JSON Tag 和真实 JSON Golden 对照。

### 4.3 命名、标识与枚举

- JSON 字段使用 snake_case。
- ID/Ref 为不透明、稳定、非空字符串；消费者不得解析内部格式。
- DTO、Event Payload 和可独立演进的 Application Contract 有明确 schema_version。
- 枚举值使用稳定机器标识；未知枚举必须按安全策略处理，不得默认为成功或放行。
- 不用模块软件版本代替 Schema 版本。
- 不暴露数据库主键类型、表名、SQL 字段或内部包路径。

### 4.4 时间字段

所有跨模块绝对时间使用 RFC3339 时间戳并带时区，持久化和比较统一 UTC。交易日期使用 YYYY-MM-DD，并明确 Asia/Shanghai 交易日语义。

必须区分：

| 字段 | 定义 | 禁止替代 |
|---|---|---|
| published_at | 来源声明的发布时间 | available_at、observed_at |
| available_at | 有来源证据证明外部可获得的时间 | published_at、fetched_at |
| valid_from / valid_to | 业务事实有效区间 | system_as_of |
| provider_time | Provider 声明的数据时间 | fetched_at |
| observed_at / system_observed_at | 本系统观察到该记录的时间 | available_at |
| fetched_at | 实际采集/下载完成时间 | observed_at |
| normalized_at | 解析/规范化完成时间 | fetched_at |
| system_as_of | Run 允许使用的系统知识截止时间 | trade_date、published_at |
| created_at / updated_at | 本地记录生命周期时间 | 来源时间、业务有效时间 |

没有可验证的 available_at 必须保持 unknown/null/缺省，不能从 published_at、observed_at 或 fetched_at 推导。observed_at 也不能为满足 PIT 被回填成 system_as_of。

### 4.5 system_as_of 传播

- Researchflow 在 CreateRun 时确定并持久化唯一的 UTC system_as_of。
- 所有下游请求、查询、Evidence/Context 写入、News Query 和 Quant 请求继承同一值。
- Stage 重试不得刷新 system_as_of。
- 子对象的 system_as_of 不得晚于 Run 的 system_as_of。
- 各模块仍需独立校验数据可见性、引用归属和质量状态，不能信任调用方伪造的质量标记。
- trade_date 是交易日，不是知识截止时间。

### 4.6 单位和数值精度

- 价格：CNY/股；成交额：CNY；成交量：shares。
- Data 现有百分比按其 API 约定使用百分点；比例/概率必须明确 ratio 或 percentage_point。
- 金额、价格、单价、成本和费用在 Quant/Research/Researchflow 跨模块 DTO 中建议使用 decimal string；计数使用 integer；比例、收益率、波动率和标准化统计量使用 number，并声明精度、舍入和误差容忍。
- 该建议与 Data 现有 API 的 number 字段通过 Data Adapter 转换，不要求为了新模块契约而无条件重写 Data API。
- 精确数值转换必须避免 float64 中间损失；Data Adapter 应使用精确解析/转换并提供边界测试。
- 数值公共表达规则以 GOV-CTR-001 为准；各字段精度、scale、舍入与容差由 DATA/QNT 对应 Schema/Registry 明确，证据未齐前不得标记 frozen。

## 5. 请求上下文、身份和错误契约

### 5.1 Request ID 与关联标识

- X-Request-ID 标识一次 HTTP 请求；入口缺失或非法时由可信入口生成。
- 响应返回最终 Request ID；内部 HTTP 调用按信任边界传播。
- Request ID 不是身份、权限或幂等键。
- 异步 Run/任务持久化 originating_request_id 作为创建来源元数据，并使用 correlation_id 关联后续 Worker 和重试。
- Worker 执行时使用自己的执行请求上下文，不把 originating_request_id 冒充当前 Request ID。
- 当前 Platform 契约不要求 traceparent、span_id、baggage、Trace SDK 或独立 Trace Collector。

### 5.2 身份类型与凭证隔离

首期只区分 anonymous、owner、service。所有路由必须显式声明访问模式，默认拒绝。

凭证必须分离：

| 凭证 | 用途 | 持有方 |
|---|---|---|
| PLATFORM_ADMIN_TOKEN | Owner 管理操作 | 部署环境 / Server |
| Platform Service Secret | 内部服务身份 | 对应服务端 Adapter |
| Data API Key | Data API 当前 X-API-Key 认证 | Server 的 Data Adapter |
| Provider Token | 外部 Data Provider | Data |
| News Source Credential | 新闻来源采集 | News |
| LLM Gateway/API Key | 模型服务调用 | LLM Adapter |
| Notification Channel Credential | 外部投递渠道 | Notification Adapter |

Platform Service Secret 不等于 Data API Key。首期 Data 保持现有 X-API-Key；Server 持有 Data 专用 Key 并在服务端 Adapter 注入 Header。浏览器、WebUI、Research 和 LLM 不得获得 Data Key。

401 表示身份缺失/无效；403 表示身份有效但无权限。Owner Token 与 Service Secret 分离；Service 不得获得 platform:admin。动态 Service Key CRUD 不属于首期契约。

### 5.3 Config / Secret

- 每个服务拥有自己的 Config Struct、字段 Schema 和 Validate；Platform 不集中定义业务配置。
- 环境变量只在 Bootstrap/Config 装载边界读取；配置装载后作为不可变快照使用。
- dotenv 仅用于本地辅助，不覆盖已有进程环境变量。
- 生产 Secret 由部署环境注入；Secret 的格式化、日志和序列化必须脱敏。
- 配置错误只报告键名和期望格式，不包含原始敏感值。
- Platform 不提供远程配置中心、热更新或动态 Secret 管理。

### 5.4 Fault

Platform 通用 Fault 最小语义：

- class：validation、unauthenticated、forbidden、not_found、conflict、unavailable、timeout、internal。
- code：稳定机器标识，领域模块使用自己的命名空间。
- safe_message：脱敏、安全、可展示摘要。
- dependency：可选逻辑依赖名，不得包含 URL、IP、DSN、Secret 或用户输入。
- retryable：默认 false，仅表示技术层面可作为重试候选。
- cause：仅内部错误链，不序列化。

HTTP 状态映射由 Transport 层负责。retryable 不自动触发重试；调用方还必须校验幂等、请求体可重放、重试预算、Deadline 和领域策略。原始 err.Error()、SQL、DSN、Token、上游完整响应和堆栈不得进入 safe_message 或 API 响应。

## 6. Platform 技术能力契约

### 6.1 Security

Platform 提供 owner/service 身份基础、可信调用上下文、显式 RoutePolicy 和默认拒绝。业务模块仍负责资源级业务授权；Platform 不建设用户中心、租户体系或动态策略引擎。

### 6.2 Diagnostics

Platform 负责 Check Contract、有限并发 Runner、Timeout/Deadline 和基础聚合。领域模块负责诊断规则、领域状态、reason_code、safe_message、evidence_ref、历史和证据。

Observation 最小字段：status、reason_code、safe_message、checked_at、duration、evidence_ref（可选）、attributes（可选）。

- reason_code 使用领域命名空间。
- evidence_ref 是不透明引用，由领域证据 Owner 解释、授权和决定保留期限。
- attributes 必须有界、白名单、低敏感结构化标量，不承载正文、snippet、原始响应。
- required 依赖缺失不得伪装 skipped；optional 未配置可 skipped 且必须有 reason。
- required Check 返回 skipped 按契约错误处理；因总 Deadline/取消而未启动的 Check 是 unavailable，不是 skipped。
- Runner 不提供 HTTP 路由，不负责认证授权，不建设通用诊断历史库。
- Data 的 Probe/Validation/Freshness/Coverage 仍归 Data，不被 Platform Observation 取代。

### 6.3 HTTP Client

调用方必须为每个外部服务/Provider 提供固定 DestinationPolicy。平台网络安全执行至少包括：

- 精确 Host/Port/协议约束；
- DNS A/AAAA 解析和私有、保留、特殊地址拦截；
- 受控 DialContext 连接已校验 IP，避免校验与实际拨号二次解析；
- 默认禁止重定向，显式允许时逐跳重新校验；
- 不可信来源默认禁用环境代理；
- Context Deadline、连接/响应头超时、响应体大小上限和并发上限；
- 仅对明确幂等且请求体可重放的请求有限重试。

News 负责 Source/Endpoint 授权和允许访问的来源策略；Platform 负责 DNS、拨号、重定向和代理等网络层安全执行。Provider 配额、业务错误、数据质量和熔断归领域 Adapter。单测通过不等于部署出口 SSRF 验收通过。

### 6.4 Storage / Migration

Platform 负责 SQL/Redis 连接生命周期、Ping、连接池、Migration Runner 等技术原语。

领域模块负责自己的 Repository、SQL、表结构、事务边界、Migration 文件、保留策略和领域审计历史。

- MariaDB 是事实存储；Redis 只作缓存或短期辅助。
- Migration namespace/version 唯一；已发布 Migration 不可修改，修复新增版本。
- 根注册和启动执行顺序由 Server/集成根负责。
- 领域模块分支不修改根 Migration 总注册、DI、cmd、全局路由。
- 不建设公共万能 Repository。
- Platform Storage 包级测试不等于真实 MariaDB/Redis 集成通过。

### 6.5 Task / Workflow

Platform 只提供必要的通用执行基础设施，不定义 ResearchRun、Stage、Attempt、DAG 或研究恢复语义。当前 Platform Task Queue 不作为 Researchflow 引擎；不因理论上的未来复用提前建设通用 Task Framework、分布式调度、消息队列或多集群租约系统。

## 7. Data 契约

### 7.1 边界与认证

Data 是独立 Go Module，公开入口为 HTTP/OpenAPI。根模块不 import Data internal。调用方使用自己的 Data Adapter 映射 Data API DTO 到 Quant/Research 所需 DTO。

首期 Data 认证保持 X-API-Key。Data Key 由 Server 端 Data Adapter 注入，不暴露给浏览器或业务模块。Provider Token 与 Data API Key 也必须分离。

### 7.2 数据引用与质量

Data 对 Security、MarketData、Financial、Calendar、Industry、PIT、Snapshot、Provider 和 Data Quality 负责。

研究输入应保留可追溯信息：数据类型、证券/范围、交易日、来源及版本、单位、复权口径、质量、覆盖率、Provider 时间、fetched_at、normalized_at、system_observed_at 和快照引用。

HTTP 200、Provider Probe success 不等于业务数据 valid。missing、unknown、not_supported、not_configured、degraded、invalid 必须保留各自语义。质量状态由 Data 产生，消费者不能自行伪造或升级。

### 7.3 OpenAPI ↔ Go JSON 冻结要求

- 从实际 Go Model、JSON Tag、Handler 和 OpenAPI Operation 生成双向差异报告。
- 对 Go 有而 Schema 缺失、Schema 有而 Go 无来源、required、nullable、枚举、类型、format、单位、精度和嵌套引用逐项处理。
- 以真实 Handler JSON Golden 验证，不只 marshal 单个 Model。
- nil slice、空数组、null、字段缺席、零值必须逐字段确定。
- Schema 多余字段必须证明有真实响应来源，否则删除；Go 字段必须确认对外是否公开。
- 允许存在有理由的显式白名单，但不得用白名单掩盖未审查差异。
- DATA-CONTRACT-001～008 的执行拆分以 Data 专项清单为准；本文件不复制旧统计数字作为当前事实。

## 8. News → Research 契约

### 8.1 所有权与调用路径

News 拥有 Source、Provider、授权、采集、Observation、News Item Identity/Version、关系断言、内容层和授权内容读取。

News Application 提供只读 ResearchEvidenceQuery Facade。Research 只依赖公开接口/DTO，不访问 News 私有 Domain、Repository、SQL 或 Adapter。Researchflow 把 Run 的 system_as_of、用途和内容级别传入 News Facade。该 Facade 是同一根 Go Module 内调用，不新增 News HTTP 服务。

News DTO 转换为 Research 自己的 Evidence Domain，由 Research 完成；News 不创建 Research Event、Claim、Hypothesis 或 Scenario。

### 8.2 最小查询能力

- GetEvidence：按 news_id 和可选 version_id 查询。
- ListEvidence：按固定过滤条件、知识时间、业务时间和受控内容级别分页。
- 请求必须包含可信 caller_ref、受控 purpose、requested_content_level；历史查询必须包含 system_as_of。
- 不接受任意 URL、SQL 条件、字段名、排序表达式或抓取指令。
- page_size 有硬上限；Cursor 不透明、防篡改，绑定 snapshot、system_as_of、过滤指纹、排序版本和稳定唯一排序键。
- 分页会话读取同一知识快照；不能仅依赖 OFFSET 保证并发一致性。

### 8.3 公开记录与授权

News 对外记录应区分 news_id、observation_id、version_id、source_ref、provider_ref、identity_status、business_quality_status、published_at、available_at、observed_at、fetched_at、normalized_at、content_level、content_ref、authorization_ref 和 system_as_of。

字段是否 required/optional/nullable 以 News 专项 DTO/Schema 决策为准。当前没有来源证据的 available_at 必须 unknown；当前模型未分别提供的 fetched_at/normalized_at 不得由 observed_at 推导。

Source、Endpoint、Provider Output Policy 的授权取交集。授权决策必须可追溯到不可变策略版本/快照；历史授权引用用于审计，当前读取仍需检查撤销、过期、用途和内容级别。未经授权的全文不得被 Research、LLM 或 WebUI 复制、展示或持久化。

### 8.4 PIT 查询顺序

对于 system_as_of=T，先过滤 T 时已被系统观察到的 Observation、Version、Identity、Classification、Relation，再在过滤结果上按 published_at、valid_from/valid_to 等业务时间解析。不得先解析当前关系再伪装历史状态；不得用当前版本覆盖历史版本；不得由发布时间早于 T 推导系统已知。

详细字段、错误状态、历史关系解析和分页策略以 News-Research只读证据查询契约为准。

## 9. Quant 契约

Quant 提供公开 Application Facade，首批 Port 草案包括：

- CalculateIndicators
- CalculateFactors
- EvaluateRisk
- GetResult

输入上下文应明确 request_id、system_as_of/as_of、trade_date（条件必需）、period、input_snapshot_refs、adjustment_basis（价格序列场景必需）、quality_policy、algorithm_version 和 deadline_at。

规则：

- indicator_codes/factor_codes 使用稳定代码。
- 每个 Indicator 参数使用独立 typed、版本化 Schema，不使用任意 map。
- 窗口样本不足返回 insufficient_data，不补零、不复制首值。
- Factor 必须绑定 universe_ref、交易日、分类版本和可用时点，输出样本数、覆盖率和排除原因。
- 质量和结果状态分离；completed 不等于 valid。
- RiskDecision 的 pass/reject/unknown 必须显式；规则缺失、输入质量不足或未知状态不得 pass。
- 硬风险 block 不可被 LLM、评分、Research 解释或人工摘要覆盖。
- 同一 idempotency_key + 相同规范化请求返回同一逻辑结果；相同 key 对应不同请求返回冲突。

价格/金额精度、Indicator/Factor 首批代码和参数 Schema 的语义与证据状态分别登记在 DATA-CTR-003/005、QNT-CTR-001/002/003；详见 Quant-Research公开DTO字段级契约和首批 Application Port 文档。

## 10. Research 契约

Research 拥有 Context、Evidence、Event、Claim、Hypothesis、Scenario。

Evidence 至少表达 evidence_type（fact/reported_fact/inference/hypothesis/unknown）、source_owner/ref/version、时间、system_as_of、subject_refs、stance、quality_status、limitations 和 authorization_ref。

- fact 必须有可验证来源与时间。
- reported_fact 只表示来源声称。
- inference 必须附推导依据。
- hypothesis 不得伪装成已证实事实。
- unknown 表示证据不足。
- 支持证据和反证均可追溯。
- Research 不复制 News 未授权正文；历史读取仍检查当前授权。
- Event、Claim、Hypothesis、Scenario 的修订追加新版本，不覆盖历史事实。
- Scenario 的概率和收益分布由 Quant Evaluation 提供；Research 负责情景语义和证据，不自行生成伪精确概率。
- Research 不 import Quant 或 Researchflow；Quant 结果由 Researchflow 转换为 Research 所需公开 DTO。

Research DTO 的字段、复合类型、授权引用和 JSON 示例以 Quant-Research公开DTO字段级契约为准。

## 11. Researchflow 契约

Researchflow 是唯一跨业务模块编排者，拥有 Run、Stage、Attempt、Workflow Template、DAG、Context Assembly、Tool Policy、结果验证、恢复和综合。

### 11.1 Run / Stage / Attempt 状态

Run：

- 非终态：created、queued、running、cancelling。
- 终态：succeeded、partial、failed、cancelled。
- 不设置 Run.degraded 或 Run.timed_out；degraded 是阶段输出质量，timed_out 是执行结果。

Stage：

- pending、ready、running、succeeded、degraded、skipped、failed、timed_out、cancelled、blocked。
- blocked 是调度前置条件或依赖不满足，不创建 Attempt。
- skipped 只用于模板明确裁剪/不适用的阶段，必须有 reason_code。
- 运行期依赖失败不得伪装 skipped。

Attempt：

- running、succeeded、degraded、failed、timed_out、cancelled。
- 每次 Attempt 表示一次真实执行；创建后只从 running 写入一次终态，终态不可修改或重开。
- Stage failed/timed_out 在重试预算未耗尽时可作为中间结果；Run deadline 到达导致的 timed_out 不得重试。

### 11.2 状态迁移和并发

- 状态更新使用带前置状态条件的原子更新；重复命令返回已提交状态，不重复产生副作用。
- Stage failed/timed_out 仅在 Run=running、未 cancelling、未过 deadline、依赖输出仍有效、预算允许时，经显式 RetryStage 回到 ready；每次重试创建新的 attempt_no。
- RetryStage 复用原 Run.as_of 和输入快照。
- 不允许重开终态 Run；重新研究创建新 Run，可通过 parent_run_ref 关联。
- Run cancel/deadline 与正常完成使用数据库 CAS/事务作为线性化点。
- 进入 cancelling 后拒绝新 Stage/Attempt/Retry；向在途 Attempt 传播取消并等待收敛。
- context cancellation 只是协作信号，不证明 Worker 已停止；超过 grace period 由租约/看门狗回收，fencing token 拒绝旧 Worker 迟到写入。
- Run 终态时不得仍有 running Attempt。
- DAG 必须无环；Executor 必须有全局 Worker、排队容量、单 Run 并行上限和 deadline。

### 11.3 结果传播

Workflow Template 必须按 stage_code + template_version 显式定义：

- on_degraded：accept / partial / fail；
- on_skipped：no_impact / partial / fail；
- on_failed：partial / fail；
- on_timed_out：partial / fail；
- on_blocked：partial / fail。

required Stage 不允许 on_skipped=no_impact。硬风险门禁不能配置为忽略失败/降级。Run 归并：

1. 任一 Stage 传播结果为 fail，Run=failed。
2. 没有 fail，但至少一个 Stage 传播结果为 partial，Run=partial。
3. 全部 Stage 完成且传播结果均为 accept/no_impact，Run=succeeded。
4. Run result_summary 至少包含 stage_outcomes、missing_stage_codes、degraded_stage_codes、blocked_stage_codes、reason_codes 和 coverage。
5. Stage degraded 不必然导致 Run partial，必须按模板策略归并；即使 accept，也要保留结果质量摘要。

具体迁移表、取消/deadline 竞争和 Executor 租约细节以五模块首批 Application Port 与状态机契约细化为准。

## 12. LLM 契约

LLM 只提供模型协议 Adapter，不执行工具、不管理研究状态、不拥有领域数据。

当前 Gateway 盘点以 OpenAI Responses 为首期主协议。建议 Port：

- Generate(ctx, ModelRequest) -> ModelResponse
- Stream(ctx, ModelRequest, onEvent) -> ModelResponse

规则：

- ModelRequest 首期使用 instructions + input_text；不把 Chat Completions messages 作为内部默认结构。
- 仅在模型 Capability=supported 时发送 temperature、top_p、reasoning_effort 等可选参数。
- 解析 output item、status、incomplete_details 和 usage；usage 缺失保持 unknown/null。
- Tool Call 作为结构化输出返回，由 Researchflow 校验权限、预算、参数 Schema 后执行。
- structured output 必须在 Adapter/调用方校验 Schema；不能因 Gateway 接受 text.format 就认定模型输出有效。
- Stream 必须识别 terminal event；无 terminal event 的 EOF 为 stream_interrupted，不得伪装成功。
- 区分 request_id、correlation_id、gateway_request_id、upstream_request_id。
- 本地模型真实 E2E 未验收的能力必须标记 blocked/not_run/unsupported，不能用 Fixture 代替。

## 13. Notification 契约

Notification 只负责事件接收、订阅匹配、模板渲染、渠道发送、Delivery 幂等/重试/记录和诊断；业务告警条件由上游模块计算。

公开边界采用通用、版本化 Notification Command/Event，不要求 Notification import 业务模块 Domain，也不依赖 Researchflow。

最小事件语义：

- event_id、event_type、schema_version、occurred_at、source_module、subject_ref、idempotency_key、payload、correlation_id。
- payload 必须由 event_type + schema_version 对应强类型 Schema 约束，不允许无约束任意 map。
- Subscription、Template、Channel、Delivery、DeliveryAttempt 由 Notification 拥有。
- Inbox/Event 幂等与 Delivery 幂等分开；相同幂等键和相同 fingerprint 返回原结果，不同 fingerprint 冲突。
- 外部渠道不承诺 exactly-once；结果不确定时记录 unknown，不盲目重试。
- 发送失败不回滚业务研究结果；永久错误不无限重试。
- Secret 由部署环境注入并按渠道边界脱敏。

具体 Event Schema、recipient_scope、状态和恢复语义以 NTF-CTR-001～003 及 INT-CTR-001 的项目级登记为准；首批事件映射仍为 pending_decision。

## 14. Repository、Migration 与生命周期

### 14.1 Repository / Migration 所有权

| 模块 | 领域持久化所有权 |
|---|---|
| Data | 证券、行情、财务、Provider、Probe、质量和历史 |
| News | Observation、Version、Identity、Relation、Authorization |
| Quant | 指标、策略、回测、评估、风险结果 |
| Research | Context、Evidence、Event、Claim、Hypothesis、Scenario |
| Researchflow | Run、Stage、Attempt、Workflow Template |
| LLM | 必要调用元数据和 Usage；默认不持久化完整敏感 Prompt/Response |
| Notification | Subscription、Template、Delivery、Attempt |
| Platform | 不拥有领域业务表 |

每个领域模块拥有 Repository、SQL、事务、Migration 文件和保留策略。Platform 提供连接生命周期和 Migration Runner。Migration namespace/version 必须唯一，已发布版本不可修改；修复新增版本。根 Migration 注册由 Server/集成根维护。

### 14.2 服务生命周期

Server Bootstrap 统一组织：

1. Config Load / Validate；
2. Secret Load；
3. Storage Initialize；
4. Migration；
5. 必需依赖检查；
6. Application / Adapter 构造；
7. HTTP 路由和 Worker 注册；
8. Ready；
9. Graceful Shutdown。

必需依赖初始化失败不得伪装 ready。可选依赖失败按模块策略明确 degraded 或 skipped，不把缺失能力伪装成功。

Shutdown 停止新请求和新任务派发，收敛在途请求/Attempt，关闭 HTTP Server，再关闭 Redis/MariaDB。具体超时和顺序由 Server 集成设计验证。

## 15. 兼容性与契约变更流程

### 15.1 兼容性

- 新增可选字段通常可兼容，但消费者必须能处理未知字段。
- 新增必需字段、删除字段、修改字段类型/单位/时间语义、改变枚举含义、改变状态迁移或默认传播策略均视为破坏性变更。
- 新增枚举值也可能破坏旧消费者；消费者必须对未知值 fail safe。
- Schema 版本独立于软件版本。
- 不允许静默改变默认值来改变业务语义。

### 15.2 变更流程

1. 生产模块提出契约变更提案，列出 DTO/Schema、字段语义、兼容性和迁移方式。
2. 消费模块 Review；跨三个及以上模块或改变依赖 DAG 的变更需 GOV Review。
3. 更新 Application Port、OpenAPI/JSON Schema、Go JSON Tag、示例 JSON、Golden 和契约测试。
4. 明确旧消费者兼容窗口和切换顺序。
5. 通过后更新契约版本/冻结记录；不得先在消费分支复制生产方私有结构。

## 16. 新编号契约索引与历史编号迁移

本节只保留 Platform 对跨模块契约的接入索引。项目级语义、编号、状态和阻断范围的唯一权威来源为 `feat/design_doc` 分支的 `doc/整体任务规划/跨模块契约目录与冻结状态.md`；Platform 分支不复制领域字段级 Schema，也不自行裁决业务契约。

状态解释以项目级目录为准：`accepted` 仅表示语义已确定；`pending_evidence` 表示语义已确定但 Schema/Golden/消费者映射/Contract Test 等证据未齐；只有证据齐备才可标记 `frozen`。不得将旧表的 `pending_decision` 批量继承为新契约状态，也不得因编号映射完成就将新契约标记为 frozen。

### 16.1 Platform 历史编号迁移表

以下旧编号仅指本文件历史版本中的 Platform/跨模块决策表，不代表仓库内其他同号历史编号。旧编号保留在此用于追溯；新文档、代码注释、测试和提交不得继续使用旧编号作为有效契约标识。

| 历史编号（来源限定） | 历史语义 | 新契约编号 | 当前状态（逐项按项目级目录） |
|---|---|---|---|
| Platform/跨模块决策表 CONTRACT-001 | 数值 JSON 类型、精度、舍入及 Data number 到 decimal 的无损边界 | GOV-CTR-001、DATA-CTR-003、DATA-CTR-005、QNT-CTR-001 | GOV-CTR-001=accepted；DATA-CTR-003=pending_evidence；DATA-CTR-005=pending_evidence；QNT-CTR-001=pending_evidence |
| Platform/跨模块决策表 CONTRACT-002 | 首批 Indicator 代码、参数 Schema、默认值和版本规则 | QNT-CTR-001、QNT-CTR-002 | QNT-CTR-001=pending_evidence；QNT-CTR-002=pending_evidence |
| Platform/跨模块决策表 CONTRACT-003 | 首批 Factor 代码、universe、分类版本、标准化和覆盖率口径 | QNT-CTR-001、QNT-CTR-003 | QNT-CTR-001=pending_evidence；QNT-CTR-003=pending_evidence |
| Platform/跨模块决策表 CONTRACT-004 | AuthorizationRef 字段、快照不可变性、来源授权与当前撤销检查 | GOV-CTR-006、NEWS-CTR-002、RSC-CTR-001；签发方：DATA-CTR-008、QNT-CTR-005 | GOV-CTR-006=pending_evidence；NEWS-CTR-002=pending_evidence；RSC-CTR-001=pending_evidence；签发方：DATA-CTR-008=pending_evidence；QNT-CTR-005=pending_evidence |
| Platform/跨模块决策表 CONTRACT-005 | Run 唯一 system_as_of 的产生、传递、持久化和重试 | GOV-CTR-002、FLOW-CTR-003 | GOV-CTR-002=accepted；FLOW-CTR-003=pending_evidence |
| Platform/跨模块决策表 CONTRACT-006 | Stage 结果按模板策略归并为 Run 结果 | FLOW-CTR-001、FLOW-CTR-004 | FLOW-CTR-001=pending_evidence；FLOW-CTR-004=pending_evidence |
| Platform/跨模块决策表 CONTRACT-007 | cancel/deadline/retry/lease/fencing 原子迁移和迟到写入拒绝 | FLOW-CTR-002 | FLOW-CTR-002=pending_evidence |
| Platform/跨模块决策表 CONTRACT-008 | Responses 请求/响应字段、SSE 事件及 unsupported 能力表达 | LLM-CTR-001、LLM-CTR-002、LLM-CTR-003 | LLM-CTR-001=pending_evidence；LLM-CTR-002=pending_evidence；LLM-CTR-003=pending_evidence |
| Platform/跨模块决策表 CONTRACT-009 | Notification Inbox、Delivery、Attempt 幂等及 unknown 恢复 | NTF-CTR-002、NTF-CTR-003 | NTF-CTR-002=pending_evidence；NTF-CTR-003=pending_evidence |
| Platform/跨模块决策表 CONTRACT-010 | event_type + schema_version 到强类型 Payload Schema 注册 | NTF-CTR-001 | NTF-CTR-001=pending_evidence |
| Platform/跨模块决策表 CONTRACT-011 | News PIT 过滤、授权快照/当前撤销、Cursor 快照和稳定排序 | GOV-CTR-002、NEWS-CTR-001、NEWS-CTR-002、NEWS-CTR-003 | GOV-CTR-002=accepted；NEWS-CTR-001=pending_evidence；NEWS-CTR-002=pending_evidence；NEWS-CTR-003=pending_evidence |
| Platform/跨模块决策表 CONTRACT-012 | Data OpenAPI、Go JSON Tag、真实 Handler JSON 双向一致性 | DATA-CTR-001、DATA-CTR-002 | DATA-CTR-001=pending_evidence；DATA-CTR-002=pending_evidence |
| Platform/跨模块决策表 CONTRACT-013 | Server 唯一组合根、装配入口和 Migration 总注册边界 | SRV-CTR-001 | SRV-CTR-001=pending_evidence |

### 16.2 Platform 技术契约状态

| 新契约编号 | 契约 | 权威文档 | 当前状态 |
|---|---|---|---|
| PLAT-CTR-001 | Fault 结构与分类 | `doc/platform/08-公共平台契约规范.md` 第 4 节 | pending_evidence |
| PLAT-CTR-002 | Request Context 与身份关联原语 | `doc/platform/08-公共平台契约规范.md` 第 5 节；标识符语义以 GOV-CTR-003 为准 | pending_evidence |
| PLAT-CTR-003 | 身份、授权、ServiceKey、Secret 边界 | `doc/platform/03-安全认证与ServiceKey设计.md` | pending_evidence |
| PLAT-CTR-004 | Diagnostics Observation 与 Runner | `doc/platform/04-配置日志诊断与可观测性设计.md` 第 7 节 | pending_evidence |
| PLAT-CTR-005 | HTTP Client、SSRF 与出口安全 | `doc/platform/05-HTTP存储与异步任务设计.md` 第 8、15、18、19、23 节 | pending_evidence |
| PLAT-CTR-006 | Storage、Repository、Migration 生命周期 | `doc/platform/05-HTTP存储与异步任务设计.md` 第 9、20 节 | pending_evidence |
| PLAT-CTR-007 | Typed Config 与 Secret 生命周期 | `doc/platform/08-公共平台契约规范.md` 第 6 节 | pending_evidence |

### 16.3 数值契约候选内容的迁移处理

历史 CONTRACT-001 下的数值候选决议不再作为 Platform 文档中的独立字段级定义。公共 JSON 类型、Decimal、Integer、比例和 null/缺失规则归 GOV-CTR-001；Data Provider 原始精度与 v2 DTO 分别归 DATA-CTR-003、DATA-CTR-005；Quant Metric/Indicator/Factor Registry 归 QNT-CTR-001。具体字段的 JSON 类型、scale、范围、舍入和容差必须以各生产方 Schema/Registry 为准。Platform 不统一规定所有业务数值的舍入模式，也不要求 Data v1 API 为迁移而改变既有 number 类型。

### 16.4 冻结门禁

- 每条契约必须能定位生产方、直接消费者、权威 DTO/Port、字段语义、行为/错误/时间/幂等规则、Schema 版本、正常/边界/失败样例和自动验证证据。
- `accepted` 不等于实现完成；`pending_evidence` 不得用于宣称 L2 已通过；`frozen` 必须具备可复核的 Schema、Golden、Contract Test 和消费者确认记录。
- 冻结和阻断按“模块 + 当前批次 + 实际调用点”判断。未冻结契约只阻断其直接依赖调用点，不扩大为无关模块的全局阻断。
- Platform 只负责 PLAT-CTR 技术原语及其实现证据；业务契约由对应领域生产方定义，项目级目录负责跨模块编号、状态和裁决索引。

### 16.5 本轮迁移记录

- 迁移基线：`feat/platform-foundation`，提交 `fa25033e5e58deea235be14c023e20694d942031`。
- 对照来源：`feat/design_doc` 当前项目级契约目录及临时新旧编号对照表；本次按目录逐条读取状态，不继承历史表中的 `pending_decision`。
- 迁移结果：旧 CONTRACT-001～013 全部建立来源限定映射；历史数值候选段落移交 GOV/DATA/QNT 权威契约；Platform 技术契约 PLAT-CTR-001～007 单独列示。
- 本次仅完成文档编号和索引迁移，不代表任何契约新增冻结证据，不改变项目级目录中的状态。

## 17. GOV-004 / GOV-005 门禁分层## 17. GOV-004 / GOV-005 门禁分层

门禁按**当前批次真实依赖的契约和集成范围**判断，不按整个项目所有模块是否全部完成判断。GOV-004 管理跨模块契约；GOV-005 管理模块独立开发和集成顺序。两者不是同一张“全部通过/全部不通过”的总开关。

### L1 Design Ready（设计可进入评审）

必须满足：

- 模块职责、领域事实所有权和 Go 依赖 DAG 明确。
- 当前批次的公开 Port、DTO、状态机草案及生产方/消费方明确。
- 未决项有 Decision ID、责任模块、直接阻断范围和待决问题。
- 允许继续领域设计、契约细化和独立测试；不得将草案当作冻结接口。

### L2 Module Ready（当前批次契约冻结，可独立实现）

当前模块批次**实际使用的每条跨模块契约**必须满足：

- 生产方和所有直接消费方完成 Review；责任角色明确，不要求必须指定个人姓名。
- 字段表明确 required/optional/nullable、空值、单位、精度、枚举、时间、状态、错误和幂等语义。
- JSON Schema/OpenAPI、Go DTO/JSON Tag、样例和 Golden/Contract Test 相互一致；若生产实现尚未就绪，可用生产方确认的 Fixture 验证消费者，但不得宣称真实集成通过。
- 依赖 DAG、Repository/Migration 所有权、认证和 Secret 边界无冲突。
- 当前批次所有 P0 Decision 已 accepted；非本批次能力必须标记 deferred，并明确不阻断范围。
- 记录契约版本、参与模块确认 SHA、测试入口和测试证据。

**L2 的判断单位是“模块 + 当前批次 + 实际调用点”，不是整个仓库。** 某个 Operation/DTO 不合格，只阻断消费它的调用点；不能据此笼统阻断与它无关的领域工作。

L2 不要求真实 Provider 在线成功、生产 DNS/出口 SSRF 演练、所有模块真实数据库部署、真实模型/通知渠道可用或完整端到端研究 Run。这些属于 L3。

L2 只表示该模块批次具备按契约独立实现的技术条件，**不自动授权创建分支、启动代码修改、创建 PR 或合并**。这些操作仍必须遵循 GOV-005 的共同基线确认和用户明确授权；本文件不授予任何分支操作权限。

### L3 Integration Ready（明确 SHA 组合的真实集成通过）

- 明确参与模块和精确 SHA，不使用“最新代码”作为测试基线。
- 真实 Data API、News Facade、Quant/Research/Researchflow/LLM/Notification Adapter 接入完成。
- 真实 MariaDB/Redis、Migration、Worker、取消/重试/恢复通过集成测试。
- Platform HTTP Client 生产出口 SSRF/DNS/重定向/代理安全验收通过。
- Server 路由、Worker 的 Diagnostics、Fault、Request ID、认证/授权接入通过。
- 按启用能力完成真实模型 Stream、通知渠道和授权内容访问验收。
- 端到端 Run 可追溯输入快照、算法/模型版本、证据、质量、Stage/Attempt 和失败原因；证据关联精确 SHA、Workflow/测试运行和环境。

### 17.1 当前批次阻断规则

| 问题 | 阻断什么 | 不阻断什么 |
|---|---|---|
| Quant 数值/Indicator/Factor 决策未完成 | 使用对应字段/Port 的调用点 | 不依赖这些字段的 Quant 内部领域工作 |
| News Query 的 PIT/授权/Cursor 语义未冻结 | News Evidence Query 契约和消费者 Adapter | Research 自己的 Evidence Domain、Fixture 和非 News 来源工作 |
| News 生产 Facade/Repository/Migration 未实现 | 真实 News 查询与 L3 集成 | 已冻结 DTO 上的消费者开发和 Contract Test |
| Researchflow 状态传播或取消竞态未冻结 | 依赖该行为的 Executor/Worker 实现 | 不依赖该行为的模板定义、纯状态机单测 |
| Data 某个 Operation 的 OpenAPI/Go JSON 不一致 | 该 Operation 的 L2 消费接入 | 其他已通过双向 Golden 的 Operation |
| Server 组合根入口未明确 | 共享 DI/路由/Worker/Migration 注册的修改与 L3 集成 | 各模块公开 Facade、Adapter 和独立 Migration 文件开发 |
| 生产 DNS、真实 Provider、数据库或渠道未验收 | 对应 L3 集成 | 已通过契约测试的模块 L2 独立开发 |

### 17.2 当前状态

- 架构职责和依赖 DAG：设计基线已形成，仍需结合各模块最新代码复核。
- Platform 公共技术语义：文档已形成；跨模块消费边界仍需逐项确认。
- Data OpenAPI/Go JSON：当前分支存在已记录差异；按 Operation 逐项核对，未通过的 Operation 不得作为 L2 消费契约。
- News Query：目标 Port 已描述；NEWS-CTR-001～003 的 Schema、Golden、Cursor 与授权竞态证据尚未全部闭合。
- Quant/Research：字段级草案存在；QNT-CTR-001～005、RSC-CTR-001～002 仍需按目录补齐 Schema/Golden/Contract Test；GOV-CTR-001/002 的公共语义已 accepted，不能据此宣称相关字段实现已冻结。
- Researchflow：FLOW-CTR-001～004 的设计语义已形成基线，当前登记为 pending_evidence；需补状态迁移、CAS/竞态、传播矩阵及 Adapter Golden。
- LLM/Notification：首批 Port/Event Schema 仍需明确哪些能力属于当前批次。
- Server 组合根：责任角色定义为 Server/集成组合根 Owner；具体仓库入口和目标集成分支仍需登记。

**结论：当前整体尚未达到 L2，不冻结全局共同契约。允许已满足 L2 的模块/调用批次独立开发；未满足的契约只阻断其直接依赖范围。**

## 18. 整体 Review 发现与后续动作

### P0：当前首批契约必须完成

1. 按 GOV-CTR-001、DATA-CTR-003/005、QNT-CTR-001/002/003 的权威定义补齐数值类型、精度链路、Indicator 参数和 Factor Registry 的 Schema/Golden。
2. 按 GOV-CTR-006、DATA-CTR-008、NEWS-CTR-001/002/003、QNT-CTR-005、RSC-CTR-001 补齐 AuthorizationRef Schema/Golden、当前撤销竞态、PIT 过滤和稳定 Cursor 证据。
3. GOV-CTR-002 的公共时间语义已 accepted；继续补齐 FLOW-CTR-003 及各 DATA/NEWS/QNT/RSC 字段映射 Golden，验证唯一 system_as_of 传播与重试不刷新。
4. 按 FLOW-CTR-001/002/004 完成状态传播矩阵、取消/deadline 竞争、Attempt 终态、CAS/fencing 与迟到写入 Contract Test。
5. Data 专项历史执行编号 DATA-CONTRACT-001～008（不属于新 `<DOMAIN>-CTR-NNN` 编号空间）继续按 Data 专项清单追踪；首批 Operation 对应 DATA-CTR-001/002 的 OpenAPI/Go JSON 双向 Golden 未通过前，不纳入该 Operation 的 L2 消费清单。
6. Server/集成组合根 Owner 登记 cmd、DI、HTTP 路由、Worker、Migration 总注册的实际入口；模块 Owner 只提交公开 Facade、Adapter 和独立 Migration。

### P1：对应模块批次启用前完成

7. News 完成 ResearchEvidenceQuery DTO/Schema/Fixture 的双向 Review；生产 Facade、PIT Repository 和真实授权读取按 L3 验收。
8. LLM 完成首批 Responses/SSE Port；未实现能力显式标记 unsupported/deferred。
9. Notification 完成首批 Event Schema、recipient_scope、Inbox/Delivery 幂等和 unknown 恢复语义。
10. 所有当前批次公开 DTO 补齐正常、边界、失败 Golden 和未知枚举测试。

### P2：L3 真实集成

11. 真实 Data/News/LLM/Notification Adapter 联调。
12. 生产 HTTP 出口 SSRF、安全 DNS、重定向、代理和响应大小演练。
13. 真实数据库、Worker Lease/Fencing、Migration 恢复、Run 取消/超时/重试故障演练。
14. 端到端 Run、通知和 WebUI 真实结果闭环。

## 19. 多轮 Review 复核记录（2026-11-06）

本轮在 Platform 分支执行了以下复核，结论只覆盖文档与 GitHub 分支元数据，不代表代码测试通过。

### Review 1：权威关系与重复定义

- 本文件只作为跨模块索引和裁决规则，不复制领域字段级 Schema。
- News、Quant/Research、Researchflow、Data 的字段细节仍由各自专项文档拥有。
- 发现“字段/状态在总览文档重复定义”时，以专项权威文档为准；本文件只保留边界、冻结状态和冲突登记。
- 本文件不直接修改 doc/整体任务规划，避免越过规划文档维护边界。

### Review 2：语义能否直接实现

重点检查 required/optional/nullable、空值、单位/精度、时间、状态迁移、错误、幂等、取消、重试、授权和分页。原登记表仅记录“决策主题 + pending”，无法区分语义未决和实现证据不足；已改为六种明确状态，并为每项补充生产方、消费者和实际阻断范围。

### Review 3：门禁是否互相矛盾

原文把“所有 P0 Decision 已决策”“Data 双向 Golden”“真实生产集成”放在相邻条款，但没有清楚区分当前批次契约验证与完整生产集成。现明确：
- L2 是模块/批次/调用点级别，不是仓库级一刀切。
- L2 可用生产方确认的 Fixture 做独立契约验证，但不得声称真实集成通过。
- 真实 Provider、生产网络出口、真实数据库部署、模型/通知渠道和端到端 Run 属于 L3。
- 某个 Data Operation 不一致，只阻断该 Operation 的消费，不扩大为全部 Data API 阻断。
- Server 组合根的共享入口归集成组合根 Owner；不因此阻断模块独立实现。

### Review 4：责任归属是否可执行

“集成 Owner”“各模块 Owner”不再作为模糊并列责任。生产方负责定义和维护契约；直接消费者负责确认消费语义；Server/集成组合根 Owner 唯一负责共享装配入口；Platform 只提供技术原语并协助集成，不接管业务 Owner。责任角色可以先于个人姓名确定，不能因尚未指定个人而悬空。

### 本轮分支快照

- Platform 变更前基线：`feat/platform-foundation` — `24f282ec8889ea51df9335a429cc719ad406512e`
- Data：`feat/full-go-data-providers-api-switch` — `b7d60a97b78273cf653a2910b6d1a21ac7ad6ac7`
- News：`feat/research-news-ingestion` — `7b509c184386028802699c7e3520b7944c4ed504`
- Researchflow：`feat/research-analysis-flow-native-go` — `bc7952845cd812ab22a4dfda33a35fd2cabbcecc`
- WebUI：`feat/webui-market-data-console` — `8cd13620d13da97433eb4fcd5cc9cfa3d6c8f559`
- 当前分支列表未发现独立 Quant 分支；Quant 契约按仓库中的 Quant 专项文档和模块代码归属审查，不虚构分支。

### 本轮 Review 限制

- 已读取 Platform 分支上的总规划、GOV-004/GOV-005、公共 Schema、Application Port、News Query、Quant/Research DTO 和 Platform 契约文档。
- 已读取各开发分支的最新 ref SHA；尚未对所有分支业务代码逐文件做实现审计。
- 未运行 Go 构建、测试、Provider、数据库、模型或端到端验证。
- 分支 SHA 仅是本轮审查快照；任何后续合并都会使快照失效，必须重新读取。

## 20. 本轮范围与限制

本文件基于审查基线 dc85a9da 中的项目规划与契约文档完成静态交叉 Review。它没有证明 Data、News、Platform 或五模块当前代码已经满足本契约，也没有重新运行各分支 CI、Provider、模型、数据库或部署环境测试。

任何状态从 pending 转为 accepted/frozen/passed，都必须记录：
- Decision ID；
- 决策结论与适用 Schema/Port 版本；
- Owner 和消费方 Review；
- 精确分支/提交 SHA；
- 契约测试/CI 证据；
- 未覆盖范围和后续集成条件。

本文件不授权创建模块分支、不授权跨分支合并，也不代表 GOV-004 已通过。
