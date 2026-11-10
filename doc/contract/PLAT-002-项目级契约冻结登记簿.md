# PLAT-002 项目级契约冻结登记簿

- 仓库：`Kasign/stock_data_provider`
- 核验分支：`origin/feat/design_doc`
- 核验基线 HEAD：`b6dfc85ba214fe16268c8cba780641a8bfe15086`
- 对照分支：`origin/integration`
- 对照结果：`identical`（ahead=0、behind=0）
- 登记簿版本：`0.2.2-review`
- 当前状态：`review_in_progress`
- 冻结结论：尚未整体冻结；当前无足够证据宣布 PLAT-002 全部通过
- 决策责任：项目负责人
- 复核原则：模块实现者自审不等于独立生产方/消费者确认

> 本登记簿是项目级契约状态与冻结结论的唯一权威入口。当前记录是核验起始快照；未逐项检查的契约不得因历史状态、模块 README 或旧 CI 记录而升级。

## 1. 本轮核验结论（起始快照）

### 1.1 集成基线

GitHub 远端 ref 核验：
- `feat/design_doc`：`b6dfc85ba214fe16268c8cba780641a8bfe15086`
- `integration`：`b6dfc85ba214fe16268c8cba780641a8bfe15086`
- GitHub compare：`identical`，0 个 ahead / 0 个 behind。

因此，当前 `feat/design_doc` 的代码树与 `integration` 完全一致，可以确认本轮核验基线包含当前 integration 聚合结果。此结论只针对该精确 SHA，不代表所有模块开发分支最新 HEAD 均已合入。

### 1.2 已确认的代码结构事实

| 领域 | 当前代码位置/入口 | 当前观察 |
|---|---|---|
| Platform | `internal/platform/**` | 存在 config、diagnostics、fault、httpapi、httpclient、observability、security、storage、taskqueue |
| Server | `cmd/analysis-api`、`internal/server/**` | 根入口委托 Bootstrap；Bootstrap 当前仍装配旧 Analysis 链路 |
| Data | `internal/marketdata/**` | 已有 app/Application、bootstrap、provider、repository、HTTP transport 等分层 |
| Data 旧/并行入口 | `cmd/market-data`、`services/market-data/**` | 独立 Market Data 启动入口和服务部署目录仍存在 |
| News | `internal/news/**` | 当前根 Go Module 内的领域代码 |
| Research | `internal/research/**` | 当前根 Go Module 内的领域代码 |
| Researchflow | `internal/researchflow/**` | 当前根 Go Module 内的领域代码 |
| Quant | `internal/quant/**` | 当前根 Go Module 内的领域代码 |
| LLM | `internal/llm/**` | 当前根 Go Module 内的协议适配代码 |
| Notification | `internal/notification/**` | 当前根 Go Module 内的领域与存储代码 |
| WebUI | `web/**` | 浏览器端实现 |

同时存在根目录 `go.mod` 和独立 `go-core/go.mod`。根 Module 为 `example.com/stock-analysis-flow`；`go-core` 是独立 Module。不能把“模块目录已经存在”解释为旧 Module/服务迁移已完成。

### 1.3 Server 当前事实与目标差异

已读取 `cmd/analysis-api/main.go`、`internal/server/bootstrap/bootstrap.go`、`internal/marketdata/bootstrap/module.go`：

- `cmd/analysis-api` 调用 `internal/server/bootstrap.Run`，Root Server 入口已落位。
- Bootstrap 当前显式使用旧 Analysis 相关的 `internal/adapters/marketdata`、`internal/adapters/storage/mysqlredis`、`internal/service`、`internal/platform/httpapi` 和 `internal/platform/taskqueue`。
- Bootstrap 已注册部分 News、Notification、Research Storage Migration，并创建共享存储、认证器、诊断与生命周期组件；这不等于所有业务模块 Application Port、Handler、Worker 均已完成装配。
- `cmd/market-data` 仍可独立创建 Data Store/Cache、运行 Data Migration、启动 Market Data Handler。
- Data Module 的 Dependencies 注释表明 Store/Cache 和受控出站执行器由调用方创建并拥有；但当前独立入口与 Root Server 的实际装配关系仍需继续核验。

因此，Server 是目标与现有根入口，但“所有模块已由唯一 Root Server 统一装配”和“Data 已完成同 Module 迁移”目前均不能确认。

### 1.4 本轮尚未完成

- 尚未逐项审计十个模块全部公开 Port、HTTP Handler、DTO、Schema、Golden 和消费者调用点。
- 尚未完成全部依赖边的静态 import 核验及 Repository/SQL 越界检查。
- 尚未逐条核对所有 Migration 的注册、顺序、幂等和失败恢复。
- 尚未在本基线运行全仓 gofmt、go test、race、vet、Schema Validator 或真实 MariaDB/Redis/Gateway/Provider E2E。
- 尚未取得各模块独立窗口对生产方/消费者契约的确认；GPT 自己实现和 Review 的记录不能替代该确认。


### 1.5 本轮补充核验的关键代码证据

以下 Blob SHA 均来自本轮核验基线 b6dfc85ba214fe16268c8cba780641a8bfe15086，可用于精确定位；这些事实不等于契约已冻结。

| 核验主题 | 代码/文档证据 | 当前确认事实 | 契约影响 |
|---|---|---|---|
| Fault 内部/外部边界 | internal/platform/fault/fault.go Blob ef831c8371d1b9a0aba44e6e5d05096d49e928f3；internal/server/bootstrap/http_middleware.go Blob 0ba274509dd14e3da3e1d936b19181fb44126851 | Platform Fault 全字段 json:"-"；Server 映射独立 publicError（class/code/message/retryable），Request ID 以响应 Header 传播 | PLAT-CTR-001 需核对 Data/News/旧 Analysis 的错误 envelope、HTTP 状态映射和 request_id 兼容字段 |
| Bearer 与 Session | internal/platform/security/security.go Blob b9a21dd479684ef39c1261ee3587ebf79edcbbc9；internal/platform/security/session.go Blob 1fb7cff7d76216092ddc0724d2f249137f8b687a；internal/server/bootstrap/router.go Blob 2597c998236a8e981773635eced7915a28d6b2ed | Platform 同时存在 Bearer Authenticator/RoutePolicy 和独立 SessionStore 原语；当前 Server 路由矩阵实际只注册 Anonymous/Owner Bearer，没有 owner_session 路由策略或 Session Middleware | PLAT-CTR-003 / SRV-CTR-001 存在实现与目标接入差异；Platform L1 不代表 Server A2 |
| Request ID | internal/platform/observability/request_id.go Blob b9e2d9ddc298ba23bead6ed1fcc9d6ec58ef707b；Server Middleware Blob 0ba274509dd14e3da3e1d936b19181fb44126851 | 旧 Middleware 默认信任入站 ID 且已 Deprecated；Server 使用 MiddlewareWithOptions 零值配置重新生成 ID | PLAT-CTR-002 的 Root Server 入口策略有代码证据；其他入口/Worker/跨进程传播仍需逐项核验 |
| Diagnostics | internal/platform/diagnostics/observation.go Blob 3c0b351cf340a87d18434609a7afd6b0fb37fa28；runner.go Blob b93f6ef40f525f86f29e29eb9250e36d2d890cdf | Observation 状态为 healthy/degraded/unavailable/skipped/failed；非 healthy 要求 reason_code；Runner 会将 Required Check 的 skipped 归一为 failed | PLAT-CTR-004 需核对 Server/领域 Adapter 是否保留原始状态、Required 语义及 evidence_ref |
| HTTP Client/流 | internal/platform/httpclient/client.go Blob 5e62857b6f85fe0f8e88fd5e5e07dde06a2c4508；stream.go Blob 3c2b3bd0736ac18aeeb23abad9e8f5f52528c30b | Platform 提供逐请求 DestinationPolicy 和独立流式 API | PLAT-CTR-005 需逐消费者确认 BoundDoer 注入、Body Close、取消和生产出口策略 |
| Server 组合根 | internal/server/bootstrap/bootstrap.go Blob 92df61265f173251ac7811f99a7ab9f41ac470ed；internal/server/migration/registry.go Blob dbb81dec2f1ab18a647ecea16a9ac6b0da03b483 | Bootstrap 当前仍创建旧 Analysis Runner/TaskQueue/HTTP API；Migration Registry 当前只注册 News、Research、Notification 三个 namespace | SRV-CTR-001 / PLAT-CTR-006 与目标“全模块统一装配/迁移汇总”存在差距；需逐模块区分无 Migration 与漏注册 |
| Data 独立入口与 InProcess 能力 | cmd/market-data/main.go Blob 06fc460cbea38c1aad34b9e23c5124004a91f3fe；internal/marketdata/bootstrap/module.go Blob 859df0ca2fc26f02dab2399fe684617da9f2fc89；internal/marketdata/httpapi/server.go Blob 7b54b54b3c6182ecab830c2c43f91c548dc60dba | 独立入口自行创建 Store/Cache、执行 Migration、启动 Handler；Module 已提供 Application、Handler、InProcessHandler 和 Root 注入依赖结构 | DATA/INT/SRV 契约必须区分能力已具备与 Root 实际接线；InProcessHandler 不含认证中间件，必须由 Root 包装 |
| WebUI 凭据和入口 | web/src/api.js Blob b8bcbee5908b43bfa1bc2ca744363190af38287f；web/default.conf.template Blob d13735e1da540ff24a5c59b7a5f0febf317c4161 | WebUI 将 Data API Key 写入 localStorage 并发送 X-API-Key；Nginx 将 /market-api 直接代理到 Market Data 8091，将 /analysis-api 代理到 Analysis 8092 | PLAT-CTR-003 / DATA-CTR-006 / SRV-CTR-001 与 Root Server 单一外部入口、HttpOnly Session 目标存在差异 |
| Data 数值类型 | internal/marketdata/app/port.go Blob ea4720121f2832610e3906389f1bf4535d7a9549；internal/marketdata/httpapi/openapi.yaml Blob cd8a6e3dfb8e3162ee789b11e883be9283c59f32 | Application Port 和当前 OpenAPI 1.1.0 的价格/金额字段使用 float64/number；现有契约材料另有 v1 number 与 v2 Decimal 演进约定 | DATA-CTR-005 需确认当前生产版本、Quant 实际消费版本、转换精度及 v2 Decimal 是否为本轮冻结范围 |
| Go Module | 根 go.mod Blob 2ce4960db7df0e1ba4199aaa2878824c9fdd11cd；go-core/go.mod Blob 9af6bf4b64eae9eb3f6adf002a2d66f049ae6156 仍是独立 Module | 根 Module 与 go-core Module 同时存在 | GOV/SRV/Data/Researchflow 需核对旧 Module 调用和退役门禁，不能仅按目录认定迁移完成 |

### 1.6 当前确认的跨模块差异

1. **浏览器入口差异**：WebUI 仍持有并发送 Data API Key，Nginx 仍把 Market Data 与 Analysis 转发到两个不同上游；这与 Root Server 唯一外部 API 宿主及浏览器 HttpOnly Session 目标不一致。属于迁移差异，不在本次改业务代码。
2. **Server 装配差异**：Root Server 已有入口和 Bootstrap，但当前 Bootstrap 仍以旧 Analysis 为中心；Data InProcess Module 尚未被该 Bootstrap 实际构造，Root Migration Registry 仅注册 News/Research/Notification。
3. **Session 接入差异**：Platform Session 原语已存在，Server 当前路由仍仅接受 Anonymous/Owner Bearer；不得宣称浏览器登录已端到端接入。
4. **Data 数值版本差异待核**：当前 OpenAPI 1.1.0 和 Application Port 使用 number/float64；是否符合现有 v1 兼容契约、v2 Decimal 的切换条件及 Quant 消费边界，仍需按实际调用点确认。
5. **历史 Platform 包归属**：internal/platform/httpapi 和 internal/platform/taskqueue 实际依赖旧 Analysis Domain/TaskQueue；现有项目总览与 Server 文档已将其定义为历史兼容代码，不是通用 Platform 原语。按既有架构决议禁止新增领域消费者，后续由 Server/旧 Analysis 迁移处理，不再列为待项目负责人重新裁决。

### 1.7 模块公开接口形态与装配差异

| 模块 | 当前公开/应用层形态 | 当前可确认的缺口或待核 |
|---|---|---|
| Data | internal/marketdata/app 提供 MarketDataReader；bootstrap.Module 暴露 Application、Handler、InProcessHandler；HTTP API 有按用例拆分的 Application 接口 | Root Bootstrap 尚未实际构造该 Module；InProcessHandler 不含认证/传输中间件；Data API v1 number 与 DATA-CTR-005 v2 Decimal 必须分版本 |
| News | internal/news/app 有 Provider、ProviderSet、HTTPDoer、Evidence Query Facade、Authorization Snapshot/Read | 需核实 Server 是否注入生产 Provider、BoundDoer、Owner Authorizer 和实际 Evidence Query Port；当前 Root Bootstrap 未装配 News Application |
| Research | internal/research/application 有 Application 用例，storage 有 Repository/Migration | 需核实 Server/Researchflow 的真实 Adapter 与 Owner AuthorizationRef 消费；Root 当前只注册 Research Storage Migration，不等于装配 Application |
| Researchflow | application 有 StageExecutor、WorkerStore、Worker、Scheduler、Finalizer；storage 有 MySQL Repository/Migration | README 中“当前不包含 Worker/Repository”的说法与代码目录不一致；更准确的缺口是具体 StageExecutor 生产 Adapter、Server Worker/Scheduler 生命周期和端到端流程接线 |
| Quant | 当前 internal/quant 由 domain、indicator、factor、manifest、metric、registry、result、risk、trend 等包组成 | 未发现独立 application/Facade 包；Quant README 又声明 Researchflow 是 Quant Application 的直接消费者。需核对是否存在受支持的 Application Port，不能让 Researchflow 直接依赖零散内部包 |
| LLM | internal/llm 根包提供 Provider Port、Responses JSON/SSE Adapter、Schema 和 Fault 映射 | 需要 Server 注入 Platform BoundDoer.OpenStream 的真实装配和 Researchflow 消费者 Contract Test；LLM 不拥有 HTTP Client/SSRF 策略 |
| Notification | application 定义 Repository/Channel/Delivery 等 Port；storage 有 MySQL Adapter/Migration | README 已指出完整 Dispatcher/Worker 和生产 Fanout 仍未闭环；需核实 Server Worker/Relay/事件生产方接线 |
| Server | Bootstrap 当前仍装配旧 Analysis；Migration Registry 只注册 News/Research/Notification | Data、Researchflow、Quant、LLM、Notification Worker 等完整装配未被当前 Bootstrap 证明 |

### 1.8 模块依赖方向抽样核验

以下为当前基线的公开接口/核心 Application 文件抽样，不是全仓 import 图的最终结论：

| 模块/包 | 当前抽样 import 事实 | 依赖方向判断 | 尚待完成 |
|---|---|---|---|
| Quant indicator/factor/risk | indicator.go、factor.go、risk/decision.go 仅引入 Go 标准库；result.Repository 仅依赖 Quant 自身 manifest | Quant 核心计算未直接依赖 Data、Research、Researchflow、LLM、Notification，符合纯计算边界 | 全包 import 图、Application Facade、Data v2 输入 Adapter 和 Researchflow 调用点 |
| Research Application | application.go 依赖标准库与 Platform Fault，不直接 import Quant/News/Researchflow | 符合 Research 不依赖 Quant/Researchflow 的目标方向 | Researchflow 对 Research Application 的真实 Adapter、AuthorizationRef 与 Data/News Evidence 读取 |
| Researchflow Application | executor.go/worker.go 主要依赖自身 domain/template；StageExecutor、WorkerStore 作为注入接口 | 编排层没有在抽样文件中直接 import Data/News/Quant/Research/LLM 实现，依赖反转方向正确 | 具体 StageExecutor Adapter、生产 Worker 装配和跨模块调用点 |
| News Application | app/ports.go 依赖自身 domain 与 net/http，定义 Provider/ProviderSet/HTTPDoer；EvidenceQueryFacade 组合本模块用例 | News 对出站 HTTP 使用窄 Port，不直接依赖 Platform 实现，适合由组合根注入 BoundDoer | Root ProviderSet/BoundDoer 实际注入、Research/Researchflow 调用和内容授权 |
| Notification Application | application/ports.go 依赖自身 domain，定义 Repository/Lease/Attempt 等 Port | Notification 不直接依赖 Researchflow 或事件生产方领域包，方向符合预期 | Server Relay/Worker、各生产方 Outbox Adapter 和真实渠道实现 |
| LLM | llm.go 提供模型协议类型；platform_fault.go 单独做 Platform Fault 映射 | LLM 协议适配不应依赖业务模块；Platform Fault 依赖由 Adapter 边界承接 | Platform BoundDoer/OpenStream 注入、Researchflow 真实消费者和流式终态测试 |
| Server | bootstrap.go 当前仍 import 旧 Analysis、internal/platform/httpapi、taskqueue 和 legacy adapters；未见新业务模块 Application Facade 的完整注册 | Root 入口存在，但新的模块依赖装配尚未形成完整 Composition Root | 完整 import DAG、构造图、Handler/Worker/Migration 注册矩阵 |

抽样文件 Blob：Quant indicator 802b0aeb9189e4537233b661700dab2c53c14d59；Quant factor 24a4863013bedad2411a6b107ba82998f483ceb7；Research Application 38998f41d16a2fafc2e9e8c33c9fe43f07a140c3；Researchflow Executor 1b8a6a3f6ac0df79a9ab72bca70b787788f05c9a；Researchflow Worker c08ee149de33742fea5ca1b30384308cc72cdd1b；News Ports da1d571deba9dc038db4270c72ec5c7a025ce369；Notification Ports 2cda82ab5da00aab4c8898f9ac0e384fb9c32c66；LLM llm.go cb7e0bb9a8b79a31d0d3a9bd213d03423354c87a。

### 1.9 Schema / Golden 文件盘点（存在性，不代表校验通过）

本轮只核对文件路径和 Git Blob 存在性；没有运行 JSON Schema Validator、Golden 复算或模块 Contract Test。文件存在不能标记为验收通过。

| 模块/契约域 | 当前文件事实 | 覆盖与缺口 |
|---|---|---|
| Platform | doc/platform/schema 有 1 个 PLAT-CTR-004 Observation 1.0.0 Schema；doc/platform/golden 有 3 个 Golden（Request ID 1、Observation 2） | PLAT-CTR-001/003/005/006/007 未发现独立机器 Schema；不等于 Go API 单测不存在 |
| Data | internal/marketdata/httpapi/openapi.yaml 当前为 1.1.0；项目 schema/golden 有 DATA-CTR-008 Manifest 1.0.0 各 1 个 | DATA-CTR-005 v2 Decimal Schema 当前未发现；DATA-CTR-001～004/006/007 仍需逐字段/行为契约盘点 |
| News | doc/news/schema 有 NEWS-CTR-001 的 4 个 1.0.0 Schema；doc/news/golden 有 3 个 Golden；项目 schema/golden 另有 NEWS-CTR-002 Authorization Binding 各 1 个 | NEWS-CTR-003 PIT/Cursor/排序/去重未发现独立 Schema/Golden |
| Quant | doc/quant/schema 有 7 个 1.0.0 Schema；doc/quant/golden 有 Indicator 9、Factor 3、Result 9 个 Golden；项目 schema/golden 另有 QNT-CTR-005 Manifest 各 1 个 | 文件覆盖了部分 QNT-CTR-001～005；仍需核对 Schema 与实际导出 DTO/Registry/消费者一致 |
| Research | doc/research 下未发现 schema/golden 目录；项目 schema/golden 有 RSC-CTR-001 Evidence 1.0.0 各 1 个 | RSC-CTR-002 Event/Claim/Hypothesis/Scenario 未发现专属 Schema/Golden |
| Researchflow | doc/researchflow/schema 有 4 个 1.0.0 Schema；doc/researchflow/golden 有 11 个 Golden | 需逐项映射 FLOW-CTR-001～004，不能把 Run/Stage/Attempt DTO 文件存在视为消费者测试完成 |
| LLM | doc/llm/schema 有 6 个 1.0.0 Schema；internal/llm/testdata/json 与 sse 有 JSON/SSE 输入和 expected fixtures；未发现 doc/llm/golden 目录 | Fixture 存在，但需核对是否覆盖所有 LLM-CTR-001～003 终态/错误/断流语义，并确认 CI 实际执行 |
| Notification | doc/notification/schema 有 4 个 1.0.0 Schema（Envelope + 3 类事件）；doc/notification/golden 有 6 个 Golden | NTF-CTR-001 有基础覆盖；NTF-CTR-002/003 的状态机、幂等、渠道和重试覆盖需逐项映射 |
| 项目公共授权 | doc/整体任务规划/schema 有 6 个 Schema；doc/整体任务规划/golden 有 8 个 Golden | 覆盖 GOV-CTR-006、DATA-CTR-008、NEWS-CTR-002、QNT-CTR-005、RSC-CTR-001；不覆盖全部 GOV/DATA/业务契约 |

## 2. 状态定义

| 状态 | 含义 | 判定规则 |
|---|---|---|
| `confirmed` | 当前代码/文档事实已核实 | 必须附精确路径、符号/行范围或 Blob SHA；不代表已冻结 |
| `has_difference` | 实际代码、专项文档或项目约定存在不一致 | 必须记录两侧证据、影响和归属 |
| `pending_decision` | 语义、架构取舍、安全边界或所有权尚待裁决 | 列出选项、建议、影响，等待项目负责人 |
| `pending_evidence` | 语义已有候选基线，但机器契约/消费者/测试证据不足 | 不得标记 frozen |
| `deferred` | 有明确延期理由、触发条件和不阻断范围 | 不得用来掩盖关键语义未决 |
| `not_applicable` | 对该契约/消费者关系确实不适用 | 必须写明理由 |
| `frozen` | 该 SHA 上满足全部适用冻结门禁 | 必须记录版本、生产方、全部直接消费者、Schema/Golden、双方测试、兼容与安全证据 |

`implemented`、`accepted`、`closed`、单模块 CI success、Fake 测试通过和文档已写均不是冻结状态。

## 3. 项目级契约登记总表

以下完整列出历史项目契约目录中已经分配的 43 个正式编号；本轮不新增或重编号。历史目录中 GOV-CTR-001/002/004/005 曾标记 accepted，但这只是历史语义评审状态，不是当前基线的冻结状态。当前状态按本轮代码、机器契约和消费者证据重新判定。未取得当前证据的条目一律不得继承历史 accepted/frozen。

| 契约编号 | 契约主题 | 定义方 | 直接消费者 | 当前权威定义/证据入口 | 契约版本 | 当前审查状态 | 本轮验收重点 |
|---|---|---|---|---|---|---|
| GOV-CTR-001 | JSON 类型、Decimal、整数、比例、null/缺失 | 项目级规则；各 DTO 生产方 | 所有 DTO 消费者 | 各生产模块 Schema/DTO；旧目录保留历史规则 | 规则版本待核验；字段 Schema 分别版本化 | `pending_evidence` | 类型、精度、scale、舍入、范围、required/nullable/omitempty、unknown/zero 语义和 Golden |
| GOV-CTR-002 | 时间、交易日、时区、PIT、system_as_of | 项目级时间语义；Data/News/Quant/Research/Researchflow | 所有历史研究及执行消费者 | 各领域时间字段定义；旧目录保留历史规则 | 规则版本待核验；字段 Schema 分别版本化 | `pending_evidence` | system_as_of、available_at、published_at、occurred_at、valid_time、fetched_at、normalized_at 映射；UTC/RFC3339Nano、Asia/Shanghai 交易日、严格 PIT |
| GOV-CTR-003 | ID、Request/Correlation/Event/Idempotency 作用域 | Platform 提供 Request ID 原语；各模块拥有领域 ID | Server、所有模块、Worker、事件消费者 | Platform observability；各模块事件/任务 DTO | 规则版本待核验；Golden 待核验 | `pending_evidence` | ID 作用域、生成方、异步传播、幂等 fingerprint、跨进程关联和 Header/Context Golden |
| GOV-CTR-004 | Schema 版本、兼容、废弃和迁移 | 各契约生产模块；项目级规则由治理登记 | 所有直接消费者 | 各模块 Schema Registry；doc/contract 版本规则 | 规则版本待核验；兼容矩阵待核验 | `pending_evidence` | 每个 DTO/Event 的版本、兼容级别、未知字段/枚举、废弃窗口、迁移和回滚 |
| GOV-CTR-005 | 契约状态、冻结证据和变更审批 | 项目契约治理 | 所有模块 | doc/contract/README、登记簿、决策记录 | 0.2.4-review（治理规则） | `pending_evidence` | 冻结门禁、证据 SHA、生产方/消费者确认、变更审批和解冻规则 |
| GOV-CTR-006 | AuthorizationRef 公共审计快照结构与引用语义 | Data、News、Quant、Research 分别签发各自资源授权快照；公共结构由项目治理 | Research、Researchflow 及受限资源消费者 | 各模块签发 Schema；旧目录 GOV-CTR-006 机器契约/Golden | 1.0.0（2 Schema / 4 Golden） | `pending_evidence` | Principal/Resource 映射、Scope、purpose、snapshot fingerprint、过期/撤销、当前授权复核和竞态 |
| PLAT-CTR-001 | Fault 结构与分类 | Platform | Server、所有服务端模块 | doc/platform/08 第 4 节；internal/platform/fault | Go API 版本待核验；未见独立 Schema/Golden | `pending_evidence` | Fault 内部结构、HTTP Error DTO、状态码、错误码注册、Retryable 语义和各模块映射 Golden |
| PLAT-CTR-002 | Request Context 与身份关联原语 | Platform | Server、所有服务端模块、Worker | doc/platform/08 第 5 节；observability/security Context | Request ID Golden 已有；Schema 未见 | `pending_evidence` | 可信入口、Request ID、Principal/Scope 传播、异步 Context、取消和出站 Header 覆盖 |
| PLAT-CTR-003 | 身份、授权、ServiceKey、Secret 边界 | Platform（认证原语）；Server（入口装配）；领域（资源授权） | Server、Data、News、Research、Quant、Researchflow、WebUI | doc/platform/03、08、18；Server router/bootstrap | Bearer/Session 原语版本待核验；Schema 未见 | `has_difference` + `pending_evidence` | 按 DEC-PLAT-002-017：owner_session 独立于 owner_bearer/service；Server A2 接入 Session Middleware、路由认证矩阵、CSRF/Origin/Host、credential_version 与审计；WebUI A2 接入密码登录/Cookie/CSRF；部署 A3 验收 HTTPS/可信代理/故障 fail-closed。资源授权仍归领域 Owner |
| PLAT-CTR-004 | Diagnostics Observation 与 Runner | Platform（Runner）；领域（reason_code/质量语义） | Server、各领域模块、WebUI | doc/platform/04、08；internal/platform/diagnostics | 1.0.0（Observation Schema / 2 Golden） | `pending_evidence` | Observation Schema、required/skipped、聚合优先级、reason_code Registry、evidence_ref 和领域 Adapter |
| PLAT-CTR-005 | HTTP Client、SSRF 与出口安全 | Platform | Data、News、LLM 及其他出站消费者 | doc/platform/05、15；internal/platform/httpclient | HTTP Client/Stream API 版本待核验；Schema 未见 | `pending_evidence` | DestinationPolicy、BoundDoer、DNS/IP、redirect、流式 Body 生命周期、超时/并发和真实消费者 A2 |
| PLAT-CTR-006 | Storage、Repository、Migration 生命周期 | Platform（连接/Runner）；领域（Repository/Schema/Migration 内容）；Server（汇总注册） | 所有持久化模块 | doc/platform/05；各模块 storage/migration | Migration API 版本待核验；Schema 未见 | `has_difference` + `pending_evidence` | 连接唯一性、事务边界、namespace/version、Migration Registry、锁/失败恢复/Ready/Shutdown |
| PLAT-CTR-007 | Typed Config 与 Secret 生命周期 | Platform（基础解析/脱敏）；各模块（配置 Schema）；Server（Bootstrap） | Server、所有服务端模块 | doc/platform/08 第 6 节；各模块 Config | Config API 版本待核验；Schema 未见 | `pending_evidence` | 配置所有权、读取时点、不可变快照、Secret 注入/轮换/脱敏和启动校验 |
| DATA-CTR-001 | 标准数据 DTO、JSON/OpenAPI、字段缺失语义 | Data | Quant、Researchflow、Research（适用范围）、Server、WebUI | doc/api/数据层API清单.md；internal/marketdata/httpapi/openapi.yaml | OpenAPI 1.1.0（字段级 Schema 待逐项核验） | `pending_evidence` | Go JSON Tag/OpenAPI/Schema/required/nullable/omitempty/错误码/Golden 双向一致 |
| DATA-CTR-002 | 行情单位、复权、交易日和时间口径 | Data | Quant、Researchflow、Research、Server | doc/api/数据质量与状态契约.md；MarketData model/normalizer | 规则版本待核验 | `pending_evidence` | CNY/股、shares、CNY、百分点/ratio、复权口径、交易日和 PIT 映射 |
| DATA-CTR-003 | Provider 原始值标准化与精度保真 | Data | Data 内部 Provider Adapter；间接影响 Quant/Researchflow | doc/api/数据侧整体规划.md；Provider Fixture/对账证据 | Provider 版本/Fixture 覆盖待核验 | `pending_evidence` | 原始类型、精度、单位换算、有损转换、异常值和逐 Provider Fixture |
| DATA-CTR-004 | Data Quality、覆盖率、来源和降级表达 | Data | Quant、Researchflow、WebUI、Server | doc/api/数据质量与状态契约.md；Data model/diagnostics | Quality Schema/Golden 待核验 | `pending_evidence` | quality_status/reason_code/coverage/source_ref、unknown/degraded、缺失和分母语义 |
| DATA-CTR-005 | 精度保真的跨模块日线 DTO（v2） | Data | Quant、Researchflow | internal/marketdata/httpapi/openapi.yaml；Data v2 Decimal Schema（待核） | v1.1.0 number；v2 Decimal Schema 当前未见 | `pending_evidence` | v1 number/float64 兼容；v2 Decimal string、Provider 原始精度链路、Golden 和真实 Quant Adapter |
| DATA-CTR-006 | Data HTTP API 调用认证 | Data；Server 负责 Root 边界和 Remote Adapter Secret | Server、Quant Data Adapter；迁移期独立 HTTP 调用方 | internal/marketdata/httpapi/server.go；Data Module InProcessHandler | API Key 兼容契约版本待核验；InProcess 模式无 Key | `has_difference` + `pending_evidence` | InProcess 不使用 Data API Key；Remote 模式仅 Server 持有 Key；浏览器不得持有 Key；冲突 Header/轮换/脱敏 |
| DATA-CTR-007 | Research Data Snapshot/Evidence 消费边界 | Data（数据事实）；Research（Evidence）；Server（Adapter 注入） | Research、Researchflow | Data v2 Schema；Research Evidence DTO/Adapter | DailyBarsV2 Schema 待补 | `pending_evidence` | DailyBarsV2、snapshot_ref、authorization_ref、source/version/quality/units；禁止跨模块 Repository/SQL |
| DATA-CTR-008 | Data AuthorizationRef 签发与快照访问审计 | Data | Quant、Research | Data Snapshot Manifest Schema/Golden；GOV-CTR-006 | 1.0.0（Snapshot Manifest Schema/Golden） | `pending_evidence` | Principal/Scope、purpose_code、内容寻址 Manifest、请求范围绑定、撤销和读取复核 |
| NEWS-CTR-001 | News 只读 Evidence Query Facade | News | Research、Researchflow | internal/news/app/evidence_query_facade.go；doc/news/新闻API契约.md | 1.0.0（4 Schema / 3 Golden） | `pending_evidence` | Query 输入、PIT、Cursor、分页、稳定排序、来源过滤、权限与返回 DTO |
| NEWS-CTR-002 | AuthorizationRef 与内容访问授权 | News（授权决策 Owner） | Research、Researchflow | doc/news/新闻来源授权矩阵.md；NEWS-CTR-002 Schema；GOV-CTR-006 | 1.0.0（Authorization Binding Schema/Golden） | `pending_evidence` | Source/Article 绑定、内容级别、授权快照、过期/撤销和派生 Evidence 链 |
| NEWS-CTR-003 | News PIT、稳定 Cursor、排序与去重 | News | Research、Researchflow | doc/news/新闻数据模型与质量契约.md；Evidence Query | 专属 Schema/Golden 当前未见 | `pending_evidence` | system_as_of、观察版本、available_at、Cursor 稳定性、排序/去重/快照生命周期 |
| QNT-CTR-001 | Metric/Indicator/Factor Registry 公共元数据 | Quant | Researchflow；Research 只消费编排结果 | Quant schema/manifest/registry；doc/quant/03 | 1.0.0（Registry Entry Schema） | `pending_evidence` | code、schema_version、参数 Schema、输入要求、单位、精度、舍入、范围、容差、算法版本和质量规则 |
| QNT-CTR-002 | Indicator Request/Result | Quant | Researchflow | internal/quant/indicator；Quant DTO Schema/Golden | 1.0.0（Indicator Parameters Schema） | `pending_evidence` | indicator_code、typed 参数、required/default/range、输入窗口、缺失样本、结果单位和边界样例 |
| QNT-CTR-003 | Factor Request/Result | Quant | Researchflow | internal/quant/factor；doc/quant/03 与 Golden | 1.0.0（Factor Parameters Schema） | `pending_evidence` | momentum_20d/60d、volatility_20d 的输入、算法、精度、缺失样本和结果状态 |
| QNT-CTR-004 | QuantResultRef、QuantMetric、Trend、Risk 输出 | Quant | Researchflow；Research 经 Researchflow 消费 | internal/quant/result、risk、trend；Quant DTO Schema | 1.0.0（4 DTO Schema / Golden） | `pending_evidence` | Result Envelope、Manifest、status/coverage/sample_count、RiskDecision 不可被质量状态覆盖 |
| QNT-CTR-005 | QuantResult AuthorizationRef 签发与访问审计 | Quant | Researchflow | QNT-CTR-005 Result Manifest Schema/Golden；GOV-CTR-006 | 1.0.0（Result Manifest Schema/Golden） | `pending_evidence` | trusted caller Scope、result_id/Manifest 绑定、不可变结果、读取时当前授权复核和撤销竞态 |
| RSC-CTR-001 | ResearchContext 与 Evidence | Research | Researchflow | internal/research/application；RSC-CTR-001 Evidence Schema/Golden；GOV-CTR-006 | 1.0.0（Evidence Schema/Golden） | `pending_evidence` | content_level、source_owner、authorization_ref、derived_from_refs、来源版本、PIT 和撤销复核 |
| RSC-CTR-002 | ResearchEvent、Claim、Hypothesis、Scenario | Research | Researchflow | internal/research/domain/application；doc/research/01-03 | 专属 Schema/Golden 当前未见 | `pending_evidence` | 状态迁移、引用完整性、版本、事件时间、认识论约束和跨对象一致性 |
| FLOW-CTR-001 | Run/Stage/Attempt 状态与结果传播 | Researchflow | Server、API、WebUI、Worker；业务模块作为 Stage 执行方 | doc/researchflow/03；internal/researchflow/application/domain | 1.0.0（Run/Stage Schema/Golden） | `pending_evidence` | Run/Stage/Attempt Schema、状态转移、输出传播、结果归并和终态 |
| FLOW-CTR-002 | Cancel、Retry、Deadline、Lease、Fencing | Researchflow | Scheduler、Worker、Repository、Server | internal/researchflow/application/worker.go、storage/mysql_*；doc/researchflow/03 | 1.0.0（Attempt Schema/Golden） | `pending_evidence` | CAS/事务线性化、租约接管、迟到写入、deadline、取消确认和重启恢复 |
| FLOW-CTR-003 | Run Execution Context 与 system_as_of 传播 | Researchflow | Data、News、Quant、Research、LLM | doc/researchflow/03；GOV-CTR-002 | 1.0.0（Run/Stage Context Schema 待映射） | `pending_evidence` | 唯一 system_as_of、输入快照、版本固定、Correlation ID 和下游 Adapter Golden |
| FLOW-CTR-004 | Stage 输入输出、诊断与最终报告关联 | Researchflow | 所有 Stage Provider、API/WebUI | internal/researchflow/application/executor.go；doc/researchflow/03 | 1.0.0（Result Summary Schema/Golden） | `pending_evidence` | StageExecutor typed payload、schema_version/output_type、diagnostic_refs、Fault 与领域结果分层 |
| LLM-CTR-001 | Provider Port、ModelRequest/Response、Usage、Tool Call | LLM | Researchflow | internal/llm/llm.go、responses_adapter.go；doc/llm/04 | 1.0.0（Model Request/Response Schema） | `pending_evidence` | ModelRequest/Response、Usage、ToolDefinition、system_as_of、模型能力和错误语义 |
| LLM-CTR-002 | Stream/SSE 事件、终止、断流和序号 | LLM | Researchflow | internal/llm/responses_sse.go、responses_stream.go；doc/llm/05 | 1.0.0（Model Event Schema；SSE fixtures） | `pending_evidence` | terminal event、EOF/断流、sequence_number、delta、取消、Body Close 和资源上限 |
| LLM-CTR-003 | Platform Fault → ModelErrorInfo 映射 | LLM Adapter | Researchflow；依赖 Platform Fault | internal/llm/platform_fault.go；PLAT-CTR-001 | 1.0.0（Model Error Info Schema） | `pending_evidence` | stable_code/kind/retryable/safe_message、Cause 不出线、incomplete 与技术 Fault 分类 |
| NTF-CTR-001 | NotificationEvent Schema Registry 与版本 | 各业务事件生产方提供 payload；Notification 管 Registry | Notification、事件生产方、Worker | doc/notification/03、05；Schema/Golden | 1.0.0（Envelope + 3 Event Schema / 6 Golden） | `pending_evidence` | event_type/version、payload Schema、Registry digest、兼容矩阵和生产方映射测试 |
| NTF-CTR-002 | Inbox、PublishEvent、Delivery 幂等与状态机 | Notification；事件生产方负责 Outbox 原子事实 | 事件生产方、Notification Worker、运维入口 | internal/notification/application/ports.go、storage；doc/notification/03 | 1.0.0 Envelope；状态机 Golden/Schema 覆盖待核验 | `pending_evidence` | Inbox、幂等键/fingerprint、Delivery/Attempt 状态、CAS/fencing、unknown 对账和恢复 |
| NTF-CTR-003 | Channel Adapter、重试和投递结果语义 | Notification | 外部渠道 | internal/notification/application/channel_ports.go；doc/notification/03 | Channel/Retry 专属 Schema 未见；测试矩阵待核验 | `pending_evidence` | 不承诺 exactly-once、渠道能力、明确未接受才可重试、Retry-After 和 unknown 终态 |
| SRV-CTR-001 | 唯一组合根、Bootstrap、依赖注入与生命周期 | Server | 所有服务端模块、WebUI | internal/server/bootstrap、router、migration；doc/server | 架构契约版本待裁定/登记 | `has_difference` + `pending_evidence` | 按 DEC-PLAT-002-012/016 装配 Quant Application Facade 到 Researchflow；按 DEC-PLAT-002-017 接入 Owner Session；所有模块装配、Root 单一入口、共享资源、Data InProcess/Remote、Migration/Worker/Shutdown |
| INT-CTR-001 | Quant 风险决策、Research 事件生命周期、Researchflow Run 终态到 NotificationEvent 显式映射 | Quant/Research/Researchflow 产生业务事实；Notification 接收事件 | Notification、事件消费者 | doc/notification/05；doc/notification/03 | 1.0.0（3 Event Payload Schema；6 Golden） | `pending_evidence` | event_type、触发时点、subject_ref、幂等键、Outbox/Inbox 原子性、重放/撤回和三生产方映射 |

本表是编号和状态总索引；具体字段 Schema、Golden、DTO、状态机和验收标准必须继续引用生产模块的唯一字段级权威文件。没有机器契约或消费者证据时，登记簿明确记录缺失，不以历史目录的详细描述代替当前实现。

## 4. Platform 17 号差异清单的处理索引

Platform 文档中的 P002-D01～D15 是 Platform 侧差异/待决事项索引，不是项目级裁决。当前按以下原则纳入本登记簿：

- D01：Data Go Module 目标与实际迁移阶段分开核验。
- D02：内部 Fault 与公开 HTTP Error DTO 分开核验。
- D03：Request ID 可信入口由 Server/部署拓扑决定。
- D04：Principal/Scope 不等于领域资源授权。
- D05：Diagnostics Observation 不等于领域质量。
- D06：HTTP 流式传输原语与 LLM 消费者接入分层验收。
- D07：Platform Migration Runner、领域 Migration 内容、Server 注册执行三者分权。
- D08：旧 Platform TaskQueue 不自动成为 Researchflow 通用任务能力。
- D09：实现状态与项目级契约冻结状态分离。
- D10：历史跨分支矩阵不作为当前 SHA 事实。
- D11：`internal/platform/httpapi` 的历史 Analysis 依赖需要专项归属核验。
- D12～D15：Web Owner Session、Credential、Redis Session、CSRF/限流/Audit 等候选能力逐项核对；不得将 Platform L1 实现等同 Server A2/部署 A3。

每项均须重新读取当前代码和直接消费者证据；本段只建立追踪关系，不自动继承 Platform 文档的关闭状态。

## 5. 项目级冻结门禁

每一项契约必须完成以下适用门禁；不适用必须写理由：

1. 生产方、直接消费者、Owner Role、调用方向和公开边界明确。
2. 唯一权威字段定义路径、版本和兼容策略明确。
3. 字段类型、必填/可空/缺失/零值、单位、精度、时间/PIT、ID 作用域明确。
4. 错误分类、超时、取消、重试、降级和幂等语义明确。
5. 身份、Scope、资源授权、Secret 和数据内容级别边界明确（适用时）。
6. 存在可机器校验的 Schema 或等价契约定义。
7. 正常、边界、异常、兼容 Golden 齐备（适用时）。
8. 生产方序列化/行为 Contract Test 通过。
9. 每个直接消费者的反序列化、字段映射和行为 Contract Test 通过；不能由同一份复制实现自测替代。
10. 状态机、并发、幂等、重试、取消、恢复和竞态测试通过（适用时）。
11. 记录测试命令、结果、精确代码 SHA、CI Run/测试报告链接。
12. 迁移、废弃、回滚和版本兼容窗口明确。
13. 生产环境依赖验收与契约冻结分层记录；不把 A1 包级通过冒充 A2/A3。

缺少关键语义时为 `pending_decision`；语义明确但证据不足为 `pending_evidence`。只有全部适用门禁满足才允许 `frozen`。

## 6. 模块核验责任矩阵

| 模块 | 必须核验的生产方事实 | 必须核验的消费者事实 | 重点代码范围 |
|---|---|---|---|
| Platform | Fault/Context/Security/Diagnostics/HTTP/Storage/Config 原语 API 与边界 | Server 与各消费者实际调用、绕过路径 | `internal/platform/**` |
| Server | Composition、Config、资源、路由、Migration、Worker 生命周期 | 是否仅依赖公开 Port；是否漏装/重复创建资源 | `cmd/analysis-api/**`、`internal/server/**` |
| Data | Data Port/DTO、Provider、质量/PIT、Repository/Migration | Quant/Research/Researchflow/Server 的实际适配 | `internal/marketdata/**`、`cmd/market-data/**`、`services/market-data/**` |
| News | Article/Source/Search/Evidence Query/授权 | Research/Researchflow 实际查询和引用 | `internal/news/**` |
| Research | Evidence/Context/Event/Claim 等定义与授权链 | Researchflow/Server/WebUI 读取和派生 | `internal/research/**` |
| Researchflow | Run/Stage/Attempt、状态机、Port、Worker | Server/LLM/Quant/Research/Notification 调用 | `internal/researchflow/**` |
| Quant | Indicator/Factor/Metric/Risk/Result | Data 输入适配、Researchflow 消费和结果授权 | `internal/quant/**` |
| LLM | Provider Port、JSON/SSE、Usage、错误与 Body 生命周期 | Researchflow 注入、Platform BoundDoer 消费 | `internal/llm/**` |
| Notification | Event/Inbox/Subscription/Delivery/Attempt | 各事件生产方、Server Worker、渠道适配 | `internal/notification/**` |
| WebUI | 请求、Session/Cookie/CSRF、错误和数据展示边界 | Server 实际路由/OpenAPI/权限与 DTO 对应 | `web/**`、`doc/web/**` |

## 7. 未决和阻塞事项

| 编号 | 事项 | 当前判断 | 影响 | 下一步 |
|---|---|---|---|---|
| PLAT002-Q01 | 旧项目级目录仍自称契约状态权威 | 文档权威冲突未完全清理 | 状态来源可能分裂 | 将旧目录改为历史/详细定义索引，并清除其独立状态权威声明 |
| PLAT002-Q02 | Data 双入口与 Module 迁移 | 已确认独立 `cmd/market-data` 与 Root Server 同时存在；迁移完成状态未确认 | Data/Server/部署/API | 追踪 Bootstrap 注册、调用路径、部署配置和迁移证据 |
| PLAT002-Q03 | Server 装配范围 | 根入口和 Bootstrap 存在，但当前仍装配旧 Analysis 链路 | 所有模块与 WebUI | 对照模块构造入口、Handler、Worker、Migration 注册清单 |
| PLAT002-Q04 | 历史 Platform HTTP API/TaskQueue | 目录位于 Platform，但被历史 Analysis 业务使用 | Platform/Server | 核验 import 依赖、对外路由和迁移责任；不得继续视为通用公共能力 |
| PLAT002-Q05 | Owner Session 端到端边界 | Platform L1 与 Server A2/部署 A3 必须分层 | Platform/Server/WebUI | 核验 Session Middleware、路由认证矩阵、CSRF、Origin、审计和故障策略 |
| PLAT002-Q06 | 根 Go Module 与 go-core Module | 双 Module 现状已确认；目标退役/迁移状态待核验 | Server、旧 Analysis、Researchflow | 盘点 go-core 包调用、Python Bridge、部署入口和剩余迁移清单 |
| PLAT002-Q07 | 模块专项 README 的契约登记簿链接仍指向旧目录 | Server README、Quant README 明确链接旧目录；Research README 使用未指明路径的“项目级契约目录” | 文档权威引用仍可能误导模块开发 | 本轮只修项目级文档；模块专项 README 由对应模块文档责任方按新登记簿更新 |
| PLAT002-Q08 | Researchflow README 与代码事实不一致 | README 声称当前不包含 Worker/Repository；代码已有 application Worker/WorkerStore 和 storage MySQL Migration/Repository | FLOW-CTR-001/002、Server 接入评估可能误判完成度 | 修正文档语义为“模块内 Worker/Storage 已实现，生产 StageExecutor Adapter 与 Server 运行装配待完成”；由 Researchflow 文档责任方确认 |
| PLAT002-Q09 | Quant Application Port 缺少可识别的独立 Facade | internal/quant 当前无 application 包；Quant 规划明确 Researchflow 是直接编排消费者 | QNT-CTR-001～005、Researchflow Adapter | 已形成 DEC-PLAT-002-012/016：采用窄 Application Facade + 强类型用例 Port；Quant 实现、Researchflow 适配、Server 装配；待代码与消费者 Contract Test 验收 |
| PLAT002-Q10 | 全仓模块 import/依赖 DAG 尚未完成静态核验 | 已抽样核心 Application/Port 文件，未见新业务模块直接循环依赖；Server 仍引用旧 Analysis/Platform HTTP API/TaskQueue | 不能仅凭抽样确认全仓无循环 import、内部 Repository 越界或 Platform 领域依赖 | 下一轮按完整 Go 文件清单核对 import graph、构造图和直接消费者；当前结论只标记为抽样 |

## 8. 本轮审查记录

- 代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`
- 远端分支比较：`feat/design_doc` 与 `integration` identical。
- 当前已读取：项目总览、契约治理规范/目录、Platform README/08/17、Server README、Data README、News/Research/Researchflow/Quant/LLM/Notification/Web 专项入口及代表性 Go Module/Bootstrap 文件。
- 本轮未运行全仓测试；GitHub 文件读取不等于本地编译或 CI 验收。
- 模块间独立确认尚未取得；不得标记为已确认消费者签字。

## 9. 下一步执行顺序

1. 完成十个模块代码目录、Go Module、公开 Port、Handler、Schema/Golden 清单。
2. 建立真实 import/调用依赖图，核对项目依赖 DAG 与实际方向。
3. 逐项核验公共 Fault、Request ID、Diagnostics、Security、HTTP、Storage、Config 契约。
4. 逐项核验 Data/News/Quant/Research/Researchflow/LLM/Notification DTO、事件和生命周期。
5. 核验 Server Composition Root、Migration、Worker、资源生命周期和 WebUI HTTP/Session 边界。
6. 将差异分成已确认事实、待证据、待决策、延期和不适用，交由项目负责人裁决。
7. 对满足门禁的单项契约逐项冻结；不满足者保持未冻结。
8. 最后执行登记簿、决策记录、版本记录和专项文档交叉引用复核。

**当前结论：本轮建立的是绑定 `b6dfc85ba214fe16268c8cba780641a8bfe15086` 的审查起始基线，不是 PLAT-002 冻结完成声明。**
