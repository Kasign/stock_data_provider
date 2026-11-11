# PLAT-002 项目级契约冻结登记簿

- 仓库：`Kasign/stock_data_provider`
- 代码审计基线：`origin/integration` @ `b6dfc85ba214fe16268c8cba780641a8bfe15086`
- 审计文档维护分支：`origin/feat/design_doc`（状态以当前分支 HEAD 为准）
- 固定代码审计基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`；仅对该 SHA 的代码事实作结论，不随分支 HEAD 自动漂移
- 登记簿版本：`0.2.58-review`
- 当前状态：`L1_final_review_complete_pending_owner_freeze_confirmation`
- 当前状态统计（本轮最终收敛前快照）：43 项正式 CTR；14 freeze-ready、27 pending_evidence、0 pending-decision、2 not_applicable、0 frozen。该统计仅为起始快照，不代表最终复核结论。
- 项目级决策：DEC-PLAT-002-018～040 已记录；当前 pending-decision 为 0。DATA-FLOW-B、LLM-FLOW-B 保持条件性 deferred
- 决策责任：项目负责人；根据明确授权，DEC-PLAT-002-028～040 的技术取舍由 ChatGPT 代为 Review/裁决，并在决议中标记授权来源
- 复核原则：本轮只冻结契约定义；模块实现符合性、内部测试、CI、部署另行跟踪，不得混入本轮冻结门禁

> 本登记簿是项目级契约状态与冻结结论的唯一权威入口。第1节是固定代码基线的起始快照；当前各 CTR 状态以第3节为准，冻结汇总以第24节为准，后续增量 Review 记录见第25节起。本轮执行一次性最终收敛：只处理跨模块公开契约定义缺口与权威定义冲突；实现符合性、模块测试和 CI 不得混入契约定义冻结状态。

## 1. 本轮核验结论（起始快照）

### 1.1 集成基线

GitHub 远端 ref 核验（2026-11-10）：
- `origin/integration`：`b6dfc85ba214fe16268c8cba780641a8bfe15086`，作为本轮唯一代码审计基线。
- `origin/feat/design_doc`：`671b76db8879766f3b43990406eb0a1216a53424`，作为本轮审计文档维护分支。
- GitHub compare（base=integration, head=feat/design_doc）：ahead=7、behind=0。
- compare 返回的 14 个变更文件均位于 `doc/`；未发现业务代码变更。

因此，本轮以 `b6dfc85ba214fe16268c8cba780641a8bfe15086` 的 integration 代码树作为固定审计对象，在 `feat/design_doc` 上维护审计文档。四个正在修改的模块不自动纳入本轮基线；其后续变更单独登记，只有确认触及契约边界时才扩展审计。此结论不代表所有模块开发分支最新 HEAD 均已合入。

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
| Research | 项目级 schema/golden 已新增 RSC-CTR-002 Event、Claim/Hypothesis、Scenario 各 1.0.0 Schema/Golden；doc/research 责任模块文档未改动 | 机器定义已覆盖 DTO 的四类核心对象；跨字段引用存在性、无环、版本目标当前性和时间证据关联仍需 Research 权威语义校验 |
| Researchflow | doc/researchflow/schema 有 4 个 1.0.0 Schema；doc/researchflow/golden 有 11 个 Golden | 需逐项映射 FLOW-CTR-001～004，不能把 Run/Stage/Attempt DTO 文件存在视为消费者测试完成 |
| LLM | doc/llm/schema 有 6 个 1.0.0 Schema；internal/llm/testdata/json 与 sse 有 JSON/SSE 输入和 expected fixtures；未发现 doc/llm/golden 目录 | Fixture 存在，但需核对是否覆盖所有 LLM-CTR-001～003 终态/错误/断流语义，并确认 CI 实际执行 |
| Notification | doc/notification/schema 有 4 个 1.0.0 Schema（Envelope + 3 类事件）；doc/notification/golden 有 6 个 Golden | NTF-CTR-001 有基础覆盖；NTF-CTR-002/003 的状态机、幂等、渠道和重试覆盖需逐项映射 |
| 项目公共授权 | doc/整体任务规划/schema 有 9 个 Schema；doc/整体任务规划/golden 有 12 个 Golden | 覆盖 GOV-CTR-006、DATA-CTR-008、NEWS-CTR-002、QNT-CTR-005、RSC-CTR-001、RSC-CTR-002；不覆盖全部 GOV/DATA/业务契约 |

## 2. 状态定义

| 状态 | 含义 | 判定规则 |
|---|---|---|
| `confirmed` | 当前公开定义事实已核实 | 记录权威路径、版本和关键定义；不代表可冻结 |
| `has_difference` | 两份仍有效的公开契约定义之间存在语义冲突 | 必须指出冲突双方；纯实现偏差单独记录，不使用此状态阻断契约定义冻结 |
| `pending_decision` | 仍有会改变公开契约的业务/安全/协议取舍未裁决 | 列出选项和影响，等待项目负责人 |
| `pending_evidence` | 公开契约定义本身缺少必要字段语义、机器 Schema/等价定义、版本或唯一权威依据 | 不得把内部测试、CI、部署或实现接线缺失填入此状态 |
| `freeze-ready` | 公开语义、权威定义、适用边界和项目决议完整，可以提交项目负责人冻结 | 不代表模块实现已符合 |
| `deferred` | 有明确触发条件且不属于当前首期范围 | 不得用来掩盖关键语义未决 |
| `not_applicable` | 对该契约/消费者关系确实不适用 | 必须写明理由 |
| `frozen` | 项目负责人确认冻结的契约版本 | 记录契约版本、权威定义和适用消费者；实现符合性另行跟踪 |

`implemented`、`accepted`、`closed`、单模块 CI success、Fake 测试通过和文档已写均不是冻结状态。

## 3. 项目级契约登记总表

以下完整列出历史项目契约目录中已经分配的 43 个正式编号；本轮不新增或重编号。历史目录中 GOV-CTR-001/002/004/005 曾标记 accepted，但这只是历史语义评审状态，不是当前基线的冻结状态。当前状态按本轮代码、机器契约和消费者证据重新判定。未取得当前证据的条目一律不得继承历史 accepted/frozen。

| 契约编号 | 契约主题 | 定义方 | 直接消费者 | 当前权威定义/证据入口 | 契约版本 | 当前审查状态 | 本轮验收重点 |
|---|---|---|---|---|---|---|
| GOV-CTR-001 | JSON 类型、Decimal、整数、比例、null/缺失 | 项目级规则；各 DTO 生产方 | 所有 DTO 消费者 | [GOV-CTR-001 公共 JSON 数值与空值规则](./rules/GOV-CTR-001-公共JSON类型数值与空值规则.md)；各领域字段映射见对应 CTR | 1.0.0 | `freeze-ready` | 公共 JSON 类型、Decimal 语法、integer 安全范围、number 精度/舍入原则、单位/比例和缺失/null/zero/empty/unknown 语义已统一版本化；字段级 scale、单位、范围、舍入、tolerance 和 nullable 仍由对应领域 CTR 负责，不作为本公共规则冻结阻断 |
| GOV-CTR-002 | 时间、交易日、时区、PIT、system_as_of | 项目级时间语义；Data/News/Quant/Research/Researchflow | 所有历史研究及执行消费者 | [GOV-CTR-002 公共时间与 PIT 规则](./rules/GOV-CTR-002-公共时间与PIT规则.md)；各领域时间字段映射见对应 CTR | 1.0.0 | `freeze-ready` | RFC3339Nano UTC、交易日、occurred/published/available/system_observed/system_as_of/valid_time 等公共语义、Run 截止传播和严格 PIT 边界已版本化；Provider/领域字段映射与各 DTO Schema 约束仍由对应 CTR 负责，不作为公共规则冻结阻断 |
| GOV-CTR-003 | ID、Request/Correlation/Event/Idempotency 作用域 | Platform 提供 Request ID 原语；各模块拥有领域 ID | Server、所有模块、Worker、事件消费者 | [GOV-CTR-003 公共标识符与幂等作用域规则](./rules/GOV-CTR-003-标识符与幂等作用域规则.md)；运行时传播见 PLAT-CTR-002；字段映射见各领域 CTR | 1.0.0 | `freeze-ready` | request/originating/correlation/event/idempotency/trace 身份职责、唯一性边界、重放冲突语义和 fingerprint 原则已版本化；Header/Context/Worker/事件 Envelope 的实际字段映射与执行证据仍由 PLAT/FLOW/NTF/各生产方 CTR 负责，不作为公共规则冻结阻断 |
| GOV-CTR-004 | Schema 版本、兼容、废弃和迁移 | 各契约生产模块；项目级规则由治理登记 | 所有直接消费者 | [GOV-CTR-004 Schema 版本与兼容规则](./rules/GOV-CTR-004-Schema版本兼容与废弃规则.md)；逐 DTO/Event 版本矩阵见对应 CTR | 1.0.0 | `freeze-ready` | SemVer、破坏性变更边界、closed/open enum、未知字段策略、消费者支持版本、Schema/G​​olden 不可变、废弃和直接退役规则已版本化；逐 DTO/Event 兼容矩阵和消费者证据仍由各 CTR 负责，不作为公共规则冻结阻断 |
| GOV-CTR-005 | 契约状态、冻结证据和变更审批 | 项目契约治理 | 所有模块 | doc/contract/README、登记簿第5节、决策记录、契约版本与变更记录 | 0.2.19-review（治理规则） | `freeze-ready` | 项目级定义冻结门禁、状态、版本变更/解冻记录规则已统一；模块实现符合性不混入契约定义冻结 |
| GOV-CTR-006 | AuthorizationRef 公共审计快照结构与引用语义 | Data、News、Quant、Research 分别签发各自资源授权快照；公共结构由项目治理 | Research、Researchflow 及受限资源消费者 | GOV-CTR-006 AuthorizationRef/PolicySnapshot Schema/Golden；四类 Owner 的 Principal/Resource/Purpose/ContentLevel 映射 | 1.0.0（2 Schema / 4 Golden） | `freeze-ready` | 公共 DTO 字段、各自 schema_version、RFC3339Nano UTC、decision/conditions 关系、PolicySnapshot 非空/唯一/排序、policy_version 绑定、JCS+SHA-256 指纹及 Data/News/Quant/Research 映射均有权威定义和 Golden；运行时 Owner/Consumer/撤销竞态 Contract Test 属 L2 实现符合性，不阻断项目级契约定义冻结 |
| PLAT-CTR-001 | 内部 Fault 到公共 HTTP ErrorEnvelope 投影 | Platform 定义 Fault；Server 负责 HTTP 投影 | WebUI、所有 HTTP API 消费者 | doc/contract/rules/PLAT-CTR-001-HTTP错误投影契约.md；doc/contract/schema/PLAT-CTR-001-http-error-1.0.0.schema.json；doc/contract/golden/PLAT-CTR-001-http-error.golden.json；内部 Fault 见 doc/platform/08 | 1.0.0 | `freeze-ready` | 公共 ErrorEnvelope 字段、schema_version、错误类别、code/message/retryable、HTTP 状态映射、Request ID Header/body 关系和安全 Header 已定义并有覆盖 8 类错误及 413/415 的 Golden；Server 当前实现尚需对齐 schema_version 与可选 body request_id，属于实现符合性，不阻断契约定义冻结 |
| PLAT-CTR-002 | Request Context 与身份关联原语 | Platform | Root Server、服务端 HTTP/出站 Adapter；Worker 使用领域持久化关联标识 | [PLAT-CTR-002 Request Context 契约](./rules/PLAT-CTR-002-Request-Context契约.md)；`internal/platform/observability/request_id.go`；`internal/platform/security/security.go`；Request ID Golden | 1.0.0（Go Context API + Request ID Golden；JSON Schema 不适用） | `freeze-ready` | 同步请求 Request ID/Principal Context、可信入口、取消/Deadline、出站 Request ID 覆盖/删除及异步 Context 不继承规则已明确；持久化 ID 归 GOV-CTR-003，身份/Scope 语义归 PLAT-CTR-003 |
| PLAT-CTR-003 | 身份、授权、ServiceKey、Secret 边界 | Platform（认证原语）；Server（入口装配）；领域（资源授权） | Server、Data、News、Research、Quant、Researchflow、WebUI | [PLAT-CTR-003 身份/授权/Session 契约](./rules/PLAT-CTR-003-身份授权与Session原语契约.md)；DEC-PLAT-002-017、040；`internal/platform/security/**` | 1.0.0（Bearer/Session/Principal 强类型 Go API；JSON Schema 不适用） | `freeze-ready` | 身份/AuthMethod、Principal Context、Bearer Scope、Owner Credential、SessionStore、Cookie/CSRF、RateLimit 与 Audit 原语均已定义；AuthMethod/WithPrincipal 为后续 Platform 实现差异。Server A2/WebUI A2/部署 A3 不阻断定义冻结；资源授权归领域 Owner |
| PLAT-CTR-004 | Diagnostics Observation 与 Runner | Platform（Runner）；领域（reason_code/质量语义） | Root Server、各领域 Check Adapter | [PLAT-CTR-004 Diagnostics 契约](./rules/PLAT-CTR-004-Diagnostics-Observation与Runner契约.md)；Observation 1.0.0 Schema/2 Golden；`internal/platform/diagnostics/**` | 1.0.0（Observation Schema / 2 Golden；Go Runner 等价强类型定义） | `freeze-ready` | Observation 字段/五态/reason_code/UTC/纳秒 duration/字符串 attributes 边界、Schema 与 Go Validator 职责、Check API 和 Runner 聚合优先级已统一；完整 HTTP Report 归 Server CTR |
| PLAT-CTR-005 | HTTP Client、SSRF 与出口安全 | Platform | Data、News、LLM 及其他出站消费者 | [PLAT-CTR-005 HTTP Client 契约](./rules/PLAT-CTR-005-HTTP-Client与SSRF契约.md)；doc/platform/05、08 第8节、15；`internal/platform/httpclient/**` | 1.0.0（Go API 强类型契约；JSON Schema 不适用） | `freeze-ready` | Config/Client/BoundDoer/Do/OpenStream/StreamResponse 的 API、DestinationPolicy 精确 allowlist、DNS/IP 绑定、同源重定向、缓冲重试与流式资源生命周期/限额均已版本化；消费者 A2 和生产网络 A3 属实现符合性，不阻断定义冻结 |
| PLAT-CTR-006 | Storage、Repository、Migration 生命周期 | Platform（连接/Runner）；领域（Repository/Schema/Migration 内容）；Server（汇总注册） | 所有持久化模块 | [PLAT-CTR-006 Storage 与 Migration 契约](./rules/PLAT-CTR-006-Storage与Migration契约.md)；doc/platform/05 第2～4节；doc/platform/08 第9节；`internal/platform/storage` | 1.0.0（Go API 强类型契约；JSON Schema 不适用） | `freeze-ready` | SQL/Redis 连接 API、默认值/上限/资源所有权、Migration 强类型输入、namespace/checksum/History/锁/失败恢复语义已明确；Server 注册与全局 Ready/Shutdown 编排属于 SRV-CTR-001 实现符合性 |
| PLAT-CTR-007 | Typed Config 与 Secret 生命周期 | Platform（基础解析/脱敏）；各模块（配置 Schema）；Server（Bootstrap） | Server、所有服务端模块 | [PLAT-CTR-007 Typed Config 与 Secret 契约](./rules/PLAT-CTR-007-Typed-Config与Secret契约.md)；doc/platform/08 第6节；`internal/platform/config/**` | 1.0.0（Go API 强类型契约；JSON Schema 不适用） | `freeze-ready` | Env/Secret/基础校验 API、fallback/解析错误、脱敏、DotEnv 加载边界、不可变配置快照与重启轮换语义已明确；模块 Config Schema 和部署 Secret 注入属于各模块/Server 实现符合性 |
| DATA-CTR-001 | 标准数据 DTO、JSON/OpenAPI、字段缺失语义 | Data | Quant、Researchflow、Research（适用范围）、Server、WebUI | internal/marketdata/app/port.go；internal/marketdata/application/reader.go；internal/marketdata/httpapi/openapi.yaml；internal/marketdata/model/**；DEC-PLAT-002-027 | HTTP OpenAPI 1.1.0；Application Port 强类型 DTO；字段级映射未形成完整基线 | `freeze-ready` | Provider 原始字段不仅暴露于 HTTP Quote/DailyBar/DragonTiger/LimitPool DTO，也暴露于跨模块 QuoteSnapshot/DailyBar Application Port，并由 Reader 原样映射；LimitPool 金额仍为 Provider-reported 且依赖 provider_amount_unit。字段级 required/nullable/omitempty 映射仍未完整 |
| DATA-CTR-002 | 行情单位、复权、交易日和时间口径 | Data | Quant、Researchflow、Research、Server | doc/api/contracts/数据质量与状态契约.md；internal/marketdata/model/**；internal/marketdata/httpapi/openapi.yaml | 通用规则有文字定义；逐字段时间/单位映射未版本化 | `freeze-ready` | 通用规则已存在，但 Quote.quote_time、DailyBar.provider_timestamp/system_observed_at、Fundamental.available_at/publication_date、Valuation.provider_report_date/provider_timestamp、Announcement.published_at 等语义尚未形成逐 DTO 映射；IndexDailyBar.amount_unit 无闭合枚举，部分金额仍暴露 provider_defined/provider_amount_unit；百分点与 0..1 比例字段需区分 |
| DATA-CTR-003 | Provider 原始值标准化与精度保真 | Data 内部 Provider Adapter | Data 内部 Provider Adapter | doc/api/数据侧整体规划.md（仅内部适配约定） | 不属于跨模块公开契约版本 | `not_applicable` | 仅 Data 内部 Provider 标准化规则，不存在跨模块公开消费者；从 PLAT-002 跨模块契约冻结门禁排除，不再追踪 Provider Fixture/专项测试 |
| DATA-CTR-004 | Data Quality、覆盖率、来源和降级表达 | Data | Quant、Researchflow、WebUI、Server | internal/marketdata/model/**；internal/marketdata/httpapi/openapi.yaml；doc/api/contracts/数据质量与状态契约.md；Freshness SLA 与评估契约 | QualityStatus 枚举已存在；逐领域状态映射未版本化 | `freeze-ready` | QualityStatus、ProviderProbeItem.status、ProviderProbeCheckResult.status、freshness_status 是不同状态维度；Freshness 只在诊断 CheckResult 输出。MarketBreadth 与 Probe coverage 有不同层级/分母；其他 DTO 的 has_data/total/returned/quality_status 组合尚未形成完整适用映射 |
| DATA-CTR-005 | 精度保真的跨模块日线 DTO（v2） | Data | Quant、Researchflow | internal/marketdata/app/port.go；internal/marketdata/httpapi/openapi.yaml；DEC-PLAT-002-009/047；Quant/Research 公开 DTO 字段级契约 | v1.1.0 HTTP number / InProcess float64；v2 Decimal Schema 未发现 | `deferred` | 当前 HTTP 与 Application Port 均为 number/float64；仓库未发现 DailyBarsV2、Decimal DTO、v2 Schema/Golden。DEC-009 明确保留 v1、不静默改类型；v2 还必须为 Quant Factor 暴露可验证的 bar interval 与不可变交易日历版本引用，并补消费者输入边界、精度/scale/舍入/缺失语义及版本定义 |
| DATA-CTR-006 | Data HTTP API 调用认证 | Data HTTP Adapter；Server 负责默认 Root 入口和 Remote Adapter Secret | Server Remote Data Adapter；仅远程兼容模式下的受控 HTTP 调用方 | [DATA-CTR-006 认证契约](./rules/DATA-CTR-006-Data-HTTP-API调用认证契约.md)；internal/marketdata/httpapi/openapi.yaml；Data服务间认证映射与Secret注入设计.md；DEC-PLAT-002-017/038/041 | 1.0.0（远程 X-API-Key；同进程可信 Principal Context） | `freeze-ready` | 目标语义已收敛：默认同进程由 Root Server 注入可信 Context；远程兼容仅 X-API-Key，禁止 Bearer 别名，健康接口匿名。现有 OpenAPI 顶层 ApiKey/BearerAuth OR、Handler Bearer fallback、测试 Bearer 成功用例均违约；需 Data 责任范围同步后再核验 |
| DATA-CTR-007 | Research Data Snapshot/Evidence 消费边界 | Data（数据事实）；Research（Evidence）；Server（Adapter 注入） | Research、Researchflow | internal/marketdata/app/port.go；DATA-CTR-008 Manifest Schema/Golden；DEC-PLAT-002-020；Quant-Research 公开 DTO 字段级契约 | DailyBarsSnapshot 当前 DTO；DATA-CTR-008 Manifest 1.0.0；内容 DTO/引用映射未定义 | `deferred` | DailyBarsSnapshot 返回 bars 但不含 snapshot_ref/Manifest/AuthorizationRef；Bundle.snapshot_id 未证明等同内容寻址 SnapshotRef。DATA-CTR-008 Manifest 仅记录范围/来源版本元数据，不包含行情内容或 payload Schema。缺少不可变内容定位、DailyBarsV2/质量单位字段和 Research Evidence source_ref/version/locator 的权威映射 |
| DATA-CTR-008 | Data AuthorizationRef 签发与快照访问审计 | Data | Quant、Research | DATA-CTR-008 Snapshot Manifest Schema/Golden；GOV-CTR-006；DEC-PLAT-002-038/041 | 1.0.0（Snapshot Manifest Schema/Golden） | `freeze-ready` | Manifest 的 UTC、sources tuple 排序/唯一性尚未由 Schema/Validator 完整约束；caller_ref 必须等于可信 Principal 的 AuthorizationRef.subject_ref，API Key 不可作为主体，但远程可信 caller 传递机制未定义；Scope/purpose 映射及读取时当前策略复核仍缺 Data 权威契约 |
| NEWS-CTR-001 | News 只读 Evidence Query Facade | News | Research、Researchflow | doc/news/新闻API契约.md 第11节；doc/news/schema/NEWS-CTR-001-*；doc/news/golden/NEWS-CTR-001-*；GOV-CTR-002；DEC-PLAT-002-021/042 | 1.0.0（4 Schema / 3 Golden） | `freeze-ready` | business_time_from/to 实为可选且省略（Go omitempty/Schema optional string），仅正文误写 nullable；system_as_of 等 UTC Z 约束尚未落入 Schema；system_observed_at 应映射 version.observed_at，News 数据模型文档未明确该版本级权威字段；relation_refs 缺 relation_status，不能区分 not_provided 与已确认空关系 |
| NEWS-CTR-002 | AuthorizationRef 与内容访问授权 | News（授权决策 Owner） | Research、Researchflow | doc/news/新闻来源授权矩阵.md 第10节；doc/整体任务规划/schema/NEWS-CTR-002-authorization-binding-1.0.0.schema.json；对应 Golden；GOV-CTR-006；DEC-PLAT-002-043 | 1.0.0（Binding Schema/Golden） | `freeze-ready` | Schema/Golden 与 Go Validator 已覆盖 caller/subject、resource、purpose、content level、Provider/Source/Endpoint 策略集合、tuple 排序去重、conditional 和当前策略复核；但 News API 契约正文将 policy_version 描述为 Registry 版本、将 resource_ref 描述为 news_id+version_id，与 GOV-CTR-006、Golden 和代码的 snapshot_ref / news_id+version_no 定义冲突，需统一唯一权威文字定义 |
| NEWS-CTR-003 | News PIT、稳定 Cursor、排序与去重 | News | Research、Researchflow | doc/news/新闻API契约.md 第11.1/11.3/11.6/11.9节；NEWS-CTR-001 Cursor Claims Schema；GOV-CTR-002；DEC-PLAT-002-042/044 | 复用 NEWS-CTR-001 1.0.0 Cursor Claims Schema；无独立 CTR-003 Schema | `freeze-ready` | 当前 active 契约已明确双时间 PIT、available_at 严格过滤、固定 system_as_of、news-evidence-v1 稳定排序、15 分钟 HMAC Cursor 和不可变快照；旧 News-Research 设计文档仍写显式 version_id 可不传 system_as_of、旧内容级别枚举和旧过滤字段，需明确废止/归档；NEWS-CTR-001 的 relation_status/UTC Schema 同步完成后再复核 |
| QNT-CTR-001 | Metric/Indicator/Factor Registry 公共元数据 | Quant | Researchflow；Research 只消费编排结果 | doc/quant/schema/QNT-CTR-001-registry-entry-1.0.0.schema.json；doc/quant/03；DEC-PLAT-002-045 | 1.0.0（Registry Entry Schema；首批 Entry/Golden 未形成） | `freeze-ready` | 已核定 Registry Entry.code=metric_code、每个 metric_code 独立注册；Indicator 与 Metric 一对多映射、MACD/Bollinger 复合输出拆成多个 Metric；algorithm_version 由服务端活动版本映射选择，调用方不指定。首批实际 Entry 的 unit/precision/range/rounding/input_requirements/quality_policy 权威值仍未登记 |
| QNT-CTR-002 | Indicator Request/Result | Quant | Researchflow | doc/整体任务规划/03-模块接口契约/Quant-Research公开DTO字段级契约.md 第2.1/2.3/2.7节；doc/quant/schema/QNT-CTR-002-indicator-parameters-1.0.0.schema.json；doc/quant/03；DEC-PLAT-002-045/046 | 1.0.0（Indicator Parameters Schema；8 个算法参数定义） | `freeze-ready` | 已核定请求上下文/幂等边界、8 个 indicator_code 与 typed parameters 关系、服务端算法版本选择、单/复合输出统一为 QuantMetric[]；完整 Request/IndicatorResultPayload Schema/Golden 尚缺，Registry 的 sample_count 策略及首批 Entry 元数据也未齐 |
| QNT-CTR-003 | Factor Request/Result | Quant | Researchflow | doc/quant/schema/QNT-CTR-003-factor-parameters-1.0.0.schema.json；QNT-CTR-004 Factor QuantMetric Schema/Golden；DATA-CTR-005；DEC-PLAT-002-047 | 1.0.0（Factor Parameters Schema；3 个首批 Factor） | `deferred` | 已核定仅 stock 单证券日线、三个 factor_code 与 period/样本口径、factor_code=metric_code、单 Metric 强类型输出；DailyBars v2 尚缺可验证交易日历版本/interval 元数据，Factor Request/Result Payload Schema/Golden 与完整质量输入证据未形成 |
| QNT-CTR-004 | QuantResultRef、Envelope、Indicator/Factor/Trend/Risk Payload | Quant | Researchflow；Research 经 Researchflow 消费 | doc/整体任务规划/03-模块接口契约/Quant-Research公开DTO字段级契约.md 第2.2/2.3/2.4/2.5/2.7/2.8节；doc/quant/schema/QNT-CTR-004-*；DEC-PLAT-002-028/029/046/047/048/049 | 1.0.0（Ref/Metric/Trend/Risk 局部 Schema/Golden；Envelope Schema/Golden 未形成） | `freeze-ready` | 已核定 Envelope/Manifest/Ref 三层版本身份、payload discriminator、空输入授权、Ref 投影、Manifest 身份字段、Trend/Risk 领域状态与 Envelope 状态分离；仍缺完整 Envelope/IndicatorResultPayload/FactorResultPayload Schema/Golden，Ref Schema minItems=1 与 DEC-029 冲突，Manifest Schema/Go 实现未覆盖全部身份字段/空数组，Risk RuleSet 聚合及 Payload 一致性约束未机器化 |
| QNT-CTR-005 | QuantResult AuthorizationRef 签发与访问审计 | Quant | Researchflow | doc/整体任务规划/schema/QNT-CTR-005-result-manifest-1.0.0.schema.json；doc/整体任务规划/golden/QNT-CTR-005-result-manifest.golden.json；GOV-CTR-006；DEC-PLAT-002-028/029/048/050 | 1.0.0（Result Manifest Schema/Golden） | `freeze-ready` | 已核定 Quant Owner、可信 Principal.Subject、result_id 资源绑定、quant_analysis/full、PolicySnapshot/fingerprint 与每次读取重新授权；但当前只有静态 Golden 测试，没有运行时签发/当前策略读取/审计记录/TOCTOU 门禁；Manifest Go/Schema 同步仍缺 |
| RSC-CTR-001 | Research Evidence | Research | Researchflow | doc/整体任务规划/schema/RSC-CTR-001-evidence-1.0.0.schema.json；Golden；doc/research/01、03；GOV-CTR-006 | 1.0.0（Evidence Schema/Golden） | `freeze-ready` | Evidence 字段和 JCS 指纹语义已有；Schema 未约束 RFC3339Nano UTC，source_version 条件必填和 Evidence.system_as_of/available_at 跨字段 PIT 规则仍依赖未完成的权威适用映射。ResearchContext 不归本 CTR，按 FLOW-CTR-003 核定 |
| RSC-CTR-002 | ResearchEvent、Claim、Hypothesis、Scenario | Research | Researchflow | 项目级 DTO 第3.3～3.5节；schema/RSC-CTR-002-*；golden/RSC-CTR-002-*；doc/research/01、03 | Event、Claim/Hypothesis、Scenario 1.0.0 Schema/Golden 已建立 | `freeze-ready` | Event/Claim 核心字段已对齐 DEC-030～032；仍缺 EventRef 存在性/无环、source_times 与 evidence_refs/规范化值的跨字段校验、Claim 替代目标当前版本/并发冲突校验，且 Research 责任模块文档仍保留旧 supersedes_event_version 并引用已裁决待决项。Scenario expected_value 已按 DEC-033 收敛为 scalar/非空 scalar 数组，仍缺 metric_ref 对应 Registry value_type/unit 的权威映射；QuantResultRef 的 result_type=evaluation、system_as_of 一致性和引用 Schema 与 DEC-029 的一致性未闭合 |
| FLOW-CTR-001 | Run/Stage/Attempt 状态与结果传播 | Researchflow | Server、API、WebUI；业务模块作为 Stage Provider | doc/researchflow/03；doc/researchflow/schema/run/stage/attempt；对应 Golden | 1.0.0（Run/Stage/Attempt Schema/Golden） | `freeze-ready` | 状态枚举/传播矩阵已有；Run/Stage/Attempt Schema 的 UTC、终态字段关系和 ResultSummary 关联约束未完全机器化，完整结果归并定义需与 FLOW-CTR-004 统一 |
| FLOW-CTR-002 | Cancel、Retry、Deadline、Lease、Fencing | Researchflow | Server（命令入口）；Scheduler/Worker（内部执行） | doc/researchflow/03 第3～5节、第6.2节；Attempt Schema/Golden | 1.0.0（Attempt Schema/Golden）；命令 DTO Schema 未登记 | `freeze-ready` | 取消/重试/deadline 的外部可观察状态与稳定错误结果已定义；CancelRun/RetryStage 命令字段、幂等作用域/请求指纹和 deadline/cancel 冲突结果未形成唯一权威机器/强类型契约。CAS、锁、重启恢复测试不属于本轮门禁 |
| FLOW-CTR-003 | Run Execution Context 与 system_as_of 传播 | Researchflow | Data、News、Quant、Research、LLM | doc/researchflow/03 第6.1/6.2节；ResearchContext DTO 第3.1节；GOV-CTR-002 | Run Schema 1.0.0 部分承载；独立 Context/ResearchContext Schema 未登记 | `freeze-ready` | Run Context 字段语义已有；ResearchContext/ContextSectionRef、模板解析快照、可信 caller/correlation 与各下游 Adapter 的字段映射未形成单一权威 Schema/Port 契约 |
| FLOW-CTR-004 | Stage 输入输出、诊断与最终报告关联 | Researchflow | 所有 Stage Provider、Server、WebUI | doc/researchflow/03 第2.2/2.3/6.2节；Stage/ResultSummary Schema/Golden | 1.0.0（Stage/ResultSummary Schema/Golden）；Stage Provider Result Envelope Schema 未登记 | `freeze-ready` | Stage Provider Result 的 output_type/schema_version/payload/quality_summary/diagnostic_refs 只有字段表，无权威 Schema/Output Registry；ResultSummary.quality_status 仍为任意 string，coverage 分母及 summary 资源身份/版本绑定未定义完整 |
| LLM-CTR-001 | Provider Port、ModelRequest/Response、Usage、Tool Call | LLM | Researchflow | doc/llm/04 第2～5节；ProviderConfig/CapabilityState/ModelRequest/ModelResponse Schema 1.0.0 | 1.0.0（Request/Response/Config/Capability Schema 与 JSON Fixtures） | `freeze-ready` | 主字段与条件状态已有定义；UTC RFC3339Nano 未在相关 Schema 中约束，ContentPart.annotations Schema 允许任意值而 DTO 定义为 Annotation[]，部分未知/条件字段的结构约束仍过宽 |
| LLM-CTR-002 | Stream/SSE 事件、终止、断流和序号 | LLM | Researchflow | doc/llm/04 第6节；doc/llm/05；ModelEvent Schema 1.0.0 | 1.0.0（ModelEvent Schema 与 SSE Fixtures） | `freeze-ready` | terminal/EOF/序号连续性/未知事件/取消语义已有；ModelEvent received_at UTC 约束未体现在 Schema，sequence_number 初始/合法范围及 terminal 后事件的公开结构约束尚未形成完整机器定义 |
| LLM-CTR-003 | Platform Fault → ModelErrorInfo 映射 | LLM Adapter | Researchflow；依赖 Platform Fault | doc/llm/04 第7/11节；doc/llm/schema/model-error-info-1.0.0.schema.json；PLAT-CTR-001 | 1.0.0（ModelErrorInfo Schema） | `freeze-ready` | Platform Fault 八类公开分类、稳定 Code 命名和安全边界已由 PLAT-CTR-001 冻结；LLM 已逐类定义 ModelErrorKind 映射、特殊响应体积错误优先级、stable_code/SafeMessage/Retryable 投影及 Dependency/Cause 隔离，Schema 覆盖 ModelErrorInfo；LLM 内部错误码词汇与真实 Adapter/Consumer 测试属于 L2 实现符合性 |
| NTF-CTR-001 | NotificationEvent Envelope、Schema Registry、Fingerprint | Notification | Quant、Research、Researchflow | doc/notification/03 第2.1/8.1节、05；Envelope 与三类 Payload Schema/Golden | 1.0.0（4 Schema / 6 Golden） | `freeze-ready` | Envelope/首批事件映射/fingerprint 算法已有；Schema 未约束 RFC3339Nano UTC，Payload reason_code/opaque ref 的稳定格式及 subject_ref 与 payload 主体一致性仍依赖文字定义，需统一机器契约约束 |
| NTF-CTR-002 | Inbox、Subscription、Delivery、Attempt、幂等和状态机 | Notification | Server；Quant/Research/Researchflow 仅发布事件 | doc/notification/03 第2～7/9节、06；Notification Application Port 与 Domain 类型 | 1.0.0（Event Schema 已有；Subscription/Delivery/Attempt Schema 未登记） | `freeze-ready` | Event 接收/Inbox 双唯一键、Delivery 唯一键和状态机语义已有；Subscription、DeliveryPlan、DeliveryRecord、DeliveryAttempt、DeliveryStateEvent 的完整跨模块 DTO/Port 权威定义与版本/错误语义未形成可核对的机器 Schema/等价 Application Port 契约 |
| NTF-CTR-003 | Notification 内部 Channel Adapter SPI 与 ChannelConfigSnapshot | Notification 内部扩展点 | Notification 内部 Channel Adapter/Resolver | doc/notification/03 第8.4节；internal/notification/application/channel_ports.go；internal/notification/domain/channel.go | 内部 Port，不属于跨模块公开契约版本 | `not_applicable` | ChannelConfigSnapshot、ChannelAdapter、ChannelResolver、RecipientResolver、ChannelSendRequest/Result 仅由 Notification 内部组件消费；不作为 PLAT-002 跨模块冻结门禁。Delivery 的 retry/unknown 对外语义归 NTF-CTR-002，不重复登记 |
| SRV-CTR-001 | Root Server 组合根、统一 HTTP 宿主与 Researchflow 对外 API | Server | WebUI、Researchflow、所有服务端模块 | doc/server/README.md；doc/web/API_CONTRACT.md（旧 Analysis 路径）；doc/api/contracts/分析任务.openapi.yaml（已退役旧 API） | 新 Researchflow HTTP Schema/版本未定义 | `deferred` | 组合根/唯一 HTTP 宿主职责已有定义；新 API 路径/方法、CreateRun/Run/Stage/Event HTTP DTO、错误投影、认证授权、分页/Cursor 与 WebUI 消费映射均无唯一权威定义。旧 Analysis API 按 DEC-018/019 退出活动范围，不得复用其 DTO/约束 |
| INT-CTR-001 | Quant 风险决策、Research 事件生命周期、Researchflow Run 终态到 NotificationEvent 显式映射 | Quant/Research/Researchflow 产生业务事实；Notification 接收事件 | Notification、事件消费者 | doc/notification/05；doc/notification/03；3 类 Payload Schema/6 Golden | 1.0.0（3 Event Payload Schema；6 Golden） | `freeze-ready` | 触发时点、subject_ref、source_event_ref、幂等键和 JCS fingerprint 已定义；事件/业务时间 UTC 约束、subject_ref 与 payload 主体一致性、changed_block_reason_codes 排序和来源时间字段绑定尚未完整机器化 |

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

## 5. PLAT-002 契约定义冻结门禁（范围更正）

**本项目当前任务是冻结跨模块公开契约，不是验收模块实现。** 因此本节只规定“契约定义能否冻结”，不把模块内部实现质量、单元测试、模块内部 Provider 测试、CI、部署或真实依赖可用性作为契约定义冻结的前置条件。

每项公开契约只核对以下适用条件：

1. 生产方、直接消费者、Owner、调用方向和公开边界明确。
2. 唯一权威定义位置、契约版本和适用范围明确。
3. 公开字段/命令/事件的类型、必填、可空、缺失、零值、单位、精度、时间/PIT、ID/幂等作用域有明确语义。
4. 状态、错误、超时、取消、重试、降级和幂等规则有明确语义（适用时）。
5. 身份、Scope、资源授权、Secret 和内容级别边界明确（适用时）。
6. 有可核对的机器 Schema，或对纯 InProcess Port 有等价的、唯一权威的强类型契约定义。
7. 版本、兼容、废弃和迁移边界明确。
8. 项目级语义决议已登记；不存在仍会改变公开契约的未决取舍。

**不属于本轮冻结门禁：** 模块内部单元/集成测试、Provider 专项测试、实现正确性、CI、部署验收、真实 Provider/模型/通知渠道可用性、模块内部 Repository/Worker/Storage 生命周期测试。发现实现与已定义契约不一致时，分别记录实现偏差；不得因此反复修改正确的契约，也不得把内部测试缺失转成项目级契约待办。

契约状态按以下口径判定：

- `freeze-ready`：公开语义、权威定义和适用边界完整，可冻结；不代表实现已符合。
- `pending-decision`：仍有会改变公开契约的业务/安全/协议取舍未裁决。
- `pending-evidence`：公开契约定义本身缺少必要字段语义、机器 Schema/等价定义、版本或权威依据。
- `blocked`：公开定义之间存在无法同时成立的冲突，尚不能确定应冻结哪一方。
- `deferred`：有明确触发条件且不属于当前首期范围。
- `frozen`：项目负责人确认冻结的契约版本；实现符合性另行跟踪，不与契约定义状态混算。

生产方/消费者测试和 CI 可作为后续实现符合性证据，但不属于本轮“契约定义冻结”的必要条件。

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

## 9. 初始审查计划（历史记录）

1. 完成十个模块代码目录、Go Module、公开 Port、Handler、Schema/Golden 清单。
2. 建立真实 import/调用依赖图，核对项目依赖 DAG 与实际方向。
3. 逐项核验公共 Fault、Request ID、Diagnostics、Security、HTTP、Storage、Config 契约。
4. 逐项核验 Data/News/Quant/Research/Researchflow/LLM/Notification DTO、事件和生命周期。
5. 核验 Server Composition Root、Migration、Worker、资源生命周期和 WebUI HTTP/Session 边界。
6. 将差异分成已确认事实、待证据、待决策、延期和不适用，交由项目负责人裁决。
7. 对满足门禁的单项契约逐项冻结；不满足者保持未冻结。
8. 最后执行登记簿、决策记录、版本记录和专项文档交叉引用复核。

**当前结论：本轮建立的是绑定 `b6dfc85ba214fe16268c8cba780641a8bfe15086` 的审查起始基线，不是 PLAT-002 冻结完成声明。**


## 10. PLAT-002 当前冻结评估（2026-11-10，状态覆盖说明）

本节是当前冻结评估快照；正式契约编号及单项状态仍以第3节为唯一总索引，详细差异以 PLAT-002 差异矩阵第35节为执行索引。

### 10.1 L1 与 L2 状态必须分层

- **L1 Design Baseline 1.0.0：已冻结。** 不可变锚点为 `6d2966e35aab06ed30791701e4ca6fc036a07702`。冻结的是已明确的职责、依赖方向、公开边界、身份/时间/数值/授权/错误/状态/幂等/版本兼容原则及首批跨模块调用范围。
- **旧综合 L2 状态快照：** 第3节曾记录 0/43 项 frozen（39 项 pending_evidence、4 项 has_difference + pending_evidence）。该统计混合了契约定义、实现接线、测试和 CI 证据，不能用于回答本轮“公开契约定义是否完整”。当前契约定义状态按第5节新门禁及第13节剩余项重新核定。
- 因此项目状态准确表述为：**L1 设计基线冻结；L2 契约证据验收未完成；PLAT-002 整体尚未完成。**

### 10.2 审计工作项统计（不作为正式 CTR 计数）

69行、56个唯一编号工作项是 B1～B8 裁决前的历史审计规模快照，不是当前待决数量。DEC-PLAT-002-018～026 已覆盖原22个含 pending_decision 的工作项；当前非条件性项目级待决队列为0。相关工作项转为 pending_evidence；DATA-FLOW-B、LLM-FLOW-B 保持 deferred。该统计不是 CTR 计数，也不与43个正式 CTR 相加。

### 10.3 L1 可维持冻结的范围

- Platform 只拥有 Fault、Security、Request Context、Diagnostics、HTTP Client、Storage、Config 等横向技术原语。
- Server 是唯一目标组合根/HTTP 宿主，负责 DI、路由安全、共享资源、Worker/Migration 生命周期和 HTTP DTO 映射。
- Data、News、Quant、Research、Researchflow、LLM、Notification 各自拥有领域事实/结果/事件/授权语义及模块公开 Port。
- Researchflow 负责跨模块编排，不拥有 Data/News/Quant/Research 的领域语义；LLM 负责模型协议适配，不执行工具；Notification 消费版本化事件，不反向依赖生产模块领域对象。
- WebUI 只消费公开 HTTP 契约，不定义领域模型，也不持有 Provider/API Key 或 Owner Bearer。
- 这些是 L1 职责/语义冻结，不代表对应 Facade/Adapter/Session/Worker 已完成 A2 接入。

### 10.4 当前不具备 L2 冻结资格的范围

- 所有 43 个正式 CTR 仍需按自身门禁补机器契约、Golden、生产方与全部消费者 Contract Test、兼容/安全/状态机证据；适用项还需真实授权、幂等、PIT、并发和故障恢复测试。
- 原关键待决主题已由 DEC-PLAT-002-018～025 裁决：Data Snapshot/交易日历/stale 映射、News Evidence 来源/质量/关系、News→Researchflow 输入快照、Quant ResultRef/Stage 投影、Research 引用/幂等/Context 访问、LLM Prompt/模型配置/重试分类、WebUI 新 API/OpenAPI 边界。当前不再等待这些主题的项目级决策。
- 当前主要证据缺口包括 Data/News/Quant/Research/LLM 真实 Facade/Adapter，Researchflow Stage Provider，Outbox/Relay/Notification Fanout，Server 十模块 DI/Worker/Migration/Storage/Diagnostics/Secret 注入，以及 WebUI→Handler 跨端测试。
- P2 WebUI 字段和页面消费者差异按端点局部处理；不自动阻断无关契约。明确 deferred 的扩展能力按触发条件管理，不把延期项升级为全局冻结阻断。

### 10.5 PLAT-002 整体退出条件

PLAT-002 不因本次矩阵统计完成而结束。整体完成仍需：
1. 十模块六维证据覆盖完成，并从生产方和全部直接消费者两端确认 Port、Schema、授权、Fault/Diagnostics、Repository/Migration/Worker、依赖方向。
2. 完整 Go import/调用图、跨模块授权链和 Server 组合装配图完成，不再仅凭抽样确认。
3. 项目级非条件性 pending_decision 已由 DEC-PLAT-002-017～026 完成裁决；责任模块按决议完成对应实现和补证。
4. 第二轮正确性 Review 和第三轮冻结门禁 Review 分别形成记录。
5. 每个可冻结 CTR 满足第5节适用门禁后，逐项记录版本、生产方、消费者、Schema/Golden、双方测试、SHA/CI 证据并改为 frozen；不得批量改状态。
6. L3 真实 Provider/模型/通知渠道/部署验收单独记录，不混入 L2，也不作为无关契约的阻断条件。

本节不改变第3节任何 CTR 状态，不代表责任模块代码修复或 CI 通过。


## 11. 项目级决策批次状态（2026-11-10）

原 DEC-B1～B8 已完成项目负责人裁决，正式决议为 DEC-PLAT-002-018～025，汇总为 DEC-PLAT-002-026。另新增 DEC-PLAT-002-027，明确 Data 公开 DTO 不暴露 Provider 原始金额/单位字段，龙虎榜公开金额统一为 CNY。当前非条件性项目级待决队列为0；责任模块实现/证据缺口按 pending_evidence 跟踪。DATA-FLOW-B、LLM-FLOW-B 仅在各自触发条件出现时重新进入决策。


## 12. DEC-B1～B8 项目级决议状态（2026-11-10）

- 项目负责人已确认 B1～B8 的 Review 修订方向，正式决议编号为 DEC-PLAT-002-018～025，汇总决议为 DEC-PLAT-002-026；DEC-PLAT-002-027 另行明确 Data 公开 DTO 的 Provider 内部字段隔离和金额归一化原则。
- B1 不保留旧 Analysis API 兼容期：当前无正在服役的旧服务，WebUI 直接切换 Researchflow 新 API；新 API 验收后删除旧路由及专属适配代码。该决议不自动授权删除仍被新模块使用的通用能力、历史数据或 Migration。
- B2～B8 按正式决议执行，详细语义和验收要求以项目级决策记录为准。
- 本次批准确认项目级公开语义和边界；契约定义冻结按第5节判断。生产方/消费者测试、CI 和实现符合性另行跟踪，不作为本轮契约定义冻结门禁。
- 后续责任模块：Server/WebUI/Researchflow（B1/B2）；Data/Quant/Research（B3）；News/Research（B4）；News/Researchflow/Server（B5）；Quant/Researchflow/Server（B6）；Research/Researchflow（B7）；LLM/Researchflow/Server（B8）。

## 13. 契约定义未完成项收敛（2026-11-11）

本节按第5节更正后的“契约定义冻结”口径归类。它取代“43 个 CTR 全部 pending_evidence 就等于 43 份契约都未定义”的错误读法；第3节原状态是旧的综合证据状态快照，不能直接作为本轮契约定义冻结结论。此处不因缺模块内部测试、CI 或实现接线而保留契约待办。

### 13.1 已完成的项目级语义裁决

- DEC-PLAT-002-018～027 已完成原有项目级语义裁决；此前非条件性 pending_decision 为 0。2026-11-11 Quant 契约核定新发现 QNT-DEC-01/02 两项真实待决，当前待决项以最新第17节为准。
- DEC-018 旧 Analysis API 退役边界已明确；旧 Analysis HTTP 契约不再是活动冻结对象。
- DEC-019 只保留为旧 API 历史裁决，不得迁移为 Researchflow 新 API 的请求约束。
- DEC-020～025 已明确 Data Snapshot、News Evidence、News 输入物化、Quant ResultRef、Research Stage 读写和 LLM 请求/重试的关键职责语义。
- DEC-027 已明确 Dragon Tiger 对外只暴露标准化 CNY 字段，不暴露 Provider 原始金额及单位字段。
- DATA-FLOW-B、LLM-FLOW-B 是条件性 deferred，不计入首期未完成契约。

### 13.2 仍需补齐的公开契约定义

| 批次 | 契约编号 | 未完成的定义内容 | 不纳入本轮的事项 |
|---|---|---|---|
| 项目公共规则 | GOV-CTR-001～005 | 将 JSON/Decimal、时间/PIT、ID/幂等、Schema 版本兼容、契约变更规则落实为可引用的权威规则及字段适用映射；其中已 accepted 的原则不重开，只补尚缺的适用范围定义 | 各模块内部序列化测试、CI |
| 公共授权 | GOV-CTR-006 | 公共 AuthorizationRef 已有 Schema/Golden；仍需明确各领域签发快照与消费者读取时的资源绑定、purpose、撤销/过期和当前授权复核语义 | 真实权限服务部署和竞态测试 |
| Platform | PLAT-CTR-001～007 | Fault→公开错误投影、Request Context 可信传播、Session/Bearer/Service 身份边界、Diagnostics 必需字段与状态含义、出站 DestinationPolicy/流接口、Storage/Migration 所有权与生命周期、Config/Secret 注入边界；PLAT-CTR-004 已有 Schema，其他项需补权威机器定义或等价接口契约 | Server 实际接线、内部包测试、全模块部署 |
| Data | DATA-CTR-001、002、004～008 | 规范 DTO 的字段级缺失/质量语义；行情单位/复权/交易日/PIT 映射；v2 Decimal 日线 Schema；InProcess 与 Remote 认证边界；DailyBarsV2 Snapshot/Evidence 引用；Data AuthorizationRef 的资源绑定与访问语义。DEC-027 的 Dragon Tiger 公开字段边界也须纳入对应公开 DTO 定义 | DATA-CTR-003 的 Provider 原始值归一化属于 Data 内部 Provider 适配约定，不作为跨模块公开契约冻结项；Provider 专项测试不纳入 |
| News | NEWS-CTR-001～003 | Evidence Query 的公开输入/输出与过滤范围；内容授权绑定；system_as_of/available_at/版本观察、Cursor 稳定性、排序、去重及 Snapshot 生命周期。NEWS-CTR-003 尚缺独立机器定义 | Provider 抓取内部实现和内部测试 |
| Quant | QNT-CTR-001～005 | Registry 参数约束、Indicator/Factor 输入输出边界、Result Envelope/Manifest/Schema、RiskDecision 与质量状态关系、AuthorizationRef 绑定及读取语义；已有 Schema 需明确覆盖到对应 CTR 的完整字段与版本 | 指标算法内部实现测试、真实计算性能 |
| Research | RSC-CTR-001～002 | Evidence 的来源版本/PIT/授权/派生引用适用语义；ResearchEvent、Claim、Hypothesis、Scenario 的公开状态、引用与版本规则。RSC-CTR-002 尚缺专属机器定义 | Research 内部 Repository/领域单测 |
| Researchflow | FLOW-CTR-001～004 | Run/Stage/Attempt 状态与终态、Cancel/Retry/Deadline/Lease/Fencing、Run Context/system_as_of、Stage 输入输出/诊断引用的完整字段映射；重点补 FLOW-CTR-003 Context Schema 映射 | Worker 实际部署、内部调度/存储测试 |
| LLM | LLM-CTR-001～003 | Model Request/Response/Usage/Tool 定义；SSE 事件序号、终止和断流语义；Platform Fault 到 ModelErrorInfo 的稳定映射；现有 Schema/Fixture 需确认唯一权威版本和覆盖边界 | 真实模型服务可用性、Provider 内部性能测试 |
| Notification | NTF-CTR-001～003 | 事件 Envelope/Registry 版本兼容；Inbox/Delivery/Attempt 状态和幂等语义；Channel 能力、重试与 unknown 终态。现有 4 个 Schema 未覆盖完整状态机/渠道契约 | 渠道账号、真实投递和 Worker 部署 |
| Server / WebUI | SRV-CTR-001；沿用 FLOW-CTR-001～004 | Server 组合根与 HTTP 宿主边界；Researchflow 新 API 的路径、方法、HTTP 请求/响应 DTO、认证、错误投影、Run/Stage/Event Schema；WebUI 只消费该公开契约。旧 Analysis API 不再补契约 | 十模块 DI 实际装配、Session E2E、Nginx 部署验收 |

### 13.3 可冻结与不应继续扩张的边界

1. CTR-043 的龙虎榜分页计数公共语义可 `freeze-ready`；Hithink Provider 分页实现不符合属于 Data 内部实现问题。
2. DEC-018～019 覆盖的旧 Analysis HTTP 契约按退役决议退出活动冻结范围，不再修旧 Schema。
3. WebUI CTR-011～049 中，只有公开 Schema/字段语义本身不完整或错误的部分才保留契约定义待办；纯页面测试、DOM 回归、Provider 内部测试和产品未选用的预留页面不阻断服务端契约冻结。
4. DEC-027 的 Dragon Tiger Provider 原始字段泄漏是公开契约定义边界问题，仍未完成；不能误归为 Provider 内部测试事项。
5. 不新增 CTR。已有编号若仅覆盖模块内部实现细节，应在对应正式 CTR 的范围说明中标为“非 PLAT-002 跨模块冻结门禁”，不另开审计任务。

### 13.4 后续唯一执行方式

后续按 13.2 的批次逐批核对权威定义文件，只回答三个问题：公开契约是否完整、生产方与消费者的公开语义是否一致、是否可以冻结。每完成一批，直接更新正式登记状态；不再汇报单个文件读取或单个字段检查，不执行模块测试/CI，不处理责任模块实现。


## 14. GOV / Platform 首批契约定义核定（2026-11-11）

本批只核定公开定义，不检查模块实现、内部测试、CI 或部署。审查对象仍固定为代码/文档基线 `b6dfc85ba214fe16268c8cba780641a8bfe15086`；本登记簿本身以本次文档提交版本为准。

### 14.1 起始核定结果（历史快照，已由第24节及第30～33节更新）

| 契约 | 本轮状态 | 核定理由 |
|---|---|---|
| GOV-CTR-001 | `pending_evidence` | 公共规则已定义 JSON 类型、Decimal、整数、比例及缺失/null/零值原则；但规则没有独立稳定版本，字段级 Schema 适用映射尚未形成唯一索引，舍入/scale 等字段语义仍需由生产方逐项引用 |||| `freeze-ready` 
| GOV-CTR-002 | `pending_evidence` | UTC/RFC3339、交易日 Asia/Shanghai、system_as_of 与 PIT 原则已明确；但 published_at/available_at/observed_at/valid_time 等时间字段到领域 DTO 的权威映射不完整，不能将原则文档当作所有字段的实际定义 |||| `freeze-ready` 
| GOV-CTR-003 | `pending_evidence` | entity/request/originating_request/correlation/event/idempotency/trace/ref 的概念与不可替代关系已定义；但各跨模块命令、事件和异步上下文的生成方、唯一性作用域、传播字段仍未形成完整引用矩阵 |||| `freeze-ready` 
| GOV-CTR-004 | `pending_evidence` | 兼容与破坏性变更原则已定义；但各活动 DTO/Event 的 schema_version、未知字段/枚举策略、兼容窗口和废弃版本尚无统一权威矩阵 |||| `freeze-ready` 
| GOV-CTR-005 | `freeze-ready` | 项目级唯一入口、状态定义、范围、版本记录和变更审批规则现已统一到 doc/contract；旧的测试/CI冻结门槛已由登记簿第5节明确排除。该状态只表示治理规则定义可冻结，不代表本项目全部 CTR 已冻结 |||| `freeze-ready` 
| GOV-CTR-006 | `pending_evidence` | AuthorizationRef 与 PolicySnapshot 已有 1.0.0 Schema 和 Golden，且 allowed/conditional/过期/撤销/读取复核原则清楚；但 Schema 未约束 RFC3339Nano UTC、conditions 与 policies 的规范排序，Data/News/Quant/Research 的 resource_ref 覆盖粒度及各自 purpose/content_level 适用映射尚不完整 |||| `freeze-ready` 
| PLAT-CTR-001 | `pending_evidence` | Fault 的 Go 类型明确标注为进程内结构且禁止 JSON 序列化；HTTP Error DTO 的字段、状态码映射、错误码命名空间和稳定版本没有唯一权威公开定义。内部 Go Fault 不等同跨模块 HTTP 错误契约 |||| `freeze-ready` 
| PLAT-CTR-002 | `pending_evidence` | Request ID Header、可信入口、随机生成和出站覆盖规则明确且有 Golden；但 Principal/Scope/认证来源 Context 的公开字段与异步传播边界未定义完整，不能以 Request ID Golden 代表整个 Request Context 契约 |||| `freeze-ready` 
| PLAT-CTR-003 | `pending_evidence` | anonymous/owner_bearer/owner_session/service 的身份边界及 DEC-017 已明确；但 Platform Session 原语仍在模块文档中标为候选接口，缺少唯一权威的类型/操作/错误/生命周期契约。Server A2/WebUI A2/部署 A3 未完成不作为本轮阻断理由 |||| `freeze-ready` 
| PLAT-CTR-004 | `pending_evidence` | Observation Schema 与两个 Golden 已存在，但 attributes 的 string Schema 与“结构化 scalar”文字定义不一致；UTC、reason_code 条件约束和 Runner 聚合规则也分散在多处，尚未构成无歧义的单一契约定义 |||| `freeze-ready` 
| PLAT-CTR-005 | `pending_evidence` | 缓冲/流式请求、DestinationPolicy、DNS/IP、重定向、取消、Body 所有权、并发和终止语义已有详细设计；但稳定公开 Port/API 的权威版本、参数/返回错误语义及文档唯一入口尚未收敛 |||| `freeze-ready` 
| PLAT-CTR-006 | `pending_evidence` | Platform 与领域/Server 的 Migration 所有权原则明确；但连接/事务/Migration Runner 的权威接口、输入输出、namespace/version、失败和关闭语义不完整。Server 是否实际注册属于实现符合性，不列为本轮缺口 |||| `freeze-ready` 
| PLAT-CTR-007 | `pending_evidence` | 配置按服务所有、启动时不可变快照、Secret 显式访问和脱敏规则明确；但 Typed Config/Secret 的公共 API 版本、解析错误语义、轮换边界与各模块配置 Schema 的适用关系未形成唯一契约索引 |||| `freeze-ready` 

### 14.2 本批范围排除

- PLAT-CTR-003 的 Server Session Middleware、WebUI 登录/退出页面、CSRF 请求接线、HTTPS/可信代理部署属于实现符合性，不作为本轮契约定义冻结条件。
- PLAT-CTR-006 的 Migration 实际注册、启动顺序、数据库 Ready/Shutdown 测试属于 Server/Platform 实现验收；本轮只核定公开接口定义。
- PLAT-CTR-005 的真实 Provider/LLM 消费者接入、网络出口测试和生产 DNS 验收不属于契约定义冻结。
- PLAT-CTR-004 的领域 reason_code 词汇表归各领域 Owner；Platform 只定义通用 Observation 结构，不建立全领域 reason_code 大枚举。

### 14.3 本批结论

- 本批 13 项中：`freeze-ready` 1 项，`pending-evidence` 12 项，`pending-decision` 0 项，`blocked` 0 项。
- 当前可以提交项目负责人冻结的是 GOV-CTR-005；其余 12 项缺口均是公开契约定义或权威映射不完整，不涉及模块内部实现整改。
- 下一批进入 Data 公开契约核定；GOV-CTR-001～004/006 在各领域字段映射核定时只补证据，不重新讨论已明确的公共原则。

## 15. Data 首批公开契约定义核定（2026-11-11）

本批覆盖 DATA-CTR-001～008，只核对公开 DTO、API 安全定义、数据质量/时间语义和不可变 Snapshot/授权引用。Provider 内部原始值归一化、Provider 专项测试和 Data 内部算法均不属于本轮。

### 15.1 核定结果

| 契约 | 状态 | 核定结论 |
|---|---|---|
| DATA-CTR-001 | pending_evidence | HTTP Go Model/OpenAPI 与跨模块 QuoteSnapshot/DailyBar Application Port 均公开 Provider 原始 volume/amount/unit/source-field；DragonTiger、LimitPool 仍暴露 Provider 原始金额/单位；另有 Go JSON/OpenAPI required/nullable/omitempty 字段映射缺口 |||| `freeze-ready` 
| DATA-CTR-002 | pending_evidence | 已有通用时间/单位规则；Quote、DailyBar、Fundamentals、Valuations、Announcements、CorporateActions、Index 等逐字段时间映射未完整；IndexDailyBar amount_unit 未闭合，Provider-defined 金额和比例/百分点差异仍需字段级约束 |||| `freeze-ready` 
| DATA-CTR-003 | not_applicable | 仅覆盖 Data 内部 Provider Adapter 原始值标准化，没有跨模块公开 DTO/Port 消费者；退出 PLAT-002 跨模块冻结门禁，不再要求 Provider Fixture 或专项测试作为项目级任务 |||| `not_applicable` 
| DATA-CTR-004 | pending_evidence | QualityStatus、Probe Item/Check 状态、Freshness 状态必须分离；Freshness 仅属于诊断 CheckResult。MarketBreadth/Probe 覆盖维度有不同分母，其他 DTO 的 has_data/total/returned 与 quality_status 关系仍需逐域定义 |||| `freeze-ready` 
| DATA-CTR-005 | pending_evidence | 当前 HTTP OpenAPI 仍为 number、InProcess Port 为 float64；未发现 DailyBarsV2/Decimal DTO、Schema/Golden；DEC-009 要求保留 v1；v2 还需提供 Quant Factor 校验连续交易日所需的 bar interval 与不可变交易日历版本引用，并定义精度/scale/舍入/缺失语义 |||| `deferred` 
| DATA-CTR-006 | pending_evidence | 目标语义已明确为默认同进程可信 Principal Context、远程兼容 X-API-Key only；当前 OpenAPI ApiKey/BearerAuth OR、Handler Bearer fallback、Bearer 成功测试仍违约；待 Data 权威 OpenAPI/Handler 同步 |||| `freeze-ready` 
| DATA-CTR-007 | pending_evidence | DailyBarsSnapshot 有 bars 但无 snapshot_ref/Manifest/AuthorizationRef；DATA-CTR-008 Manifest 只定义范围/来源版本，不含行情正文；Bundle.snapshot_id 未证明等同内容寻址 SnapshotRef，Evidence source_ref/version/locator 映射缺失 |||| `deferred` 
| DATA-CTR-008 | pending_evidence | Manifest Schema/Golden 已有，caller_ref/purpose_code、内容寻址引用和 JCS/SHA-256 算法已有文字定义；UTC/排序约束及 Data 可信 caller/Scope/授权快照读取时复核映射仍不完整 |||| `freeze-ready` 

### 15.2 关键公开定义差异

1. DATA-CTR-006 安全声明冲突：OpenAPI components.securitySchemes 同时定义 ApiKey 与 BearerAuth，根级 security 同时列出两项；OpenAPI 语义为二选一。认证设计文档则明确远程 Data 业务 API 使用 Data 专用 X-API-Key，不应把 Platform Service Bearer 当作替代凭据。该冲突是公开契约定义缺口，不是运行时认证实现问题。
2. API 清单过期：doc/api/contracts/数据层API清单.md 仍指向旧 services/market-data/openapi.yaml 路径，并描述 WebUI 保存 Data API Key；当前权威 OpenAPI 已位于 internal/marketdata/httpapi/openapi.yaml，目标同进程模式明确浏览器不持有 Data Key。该文件不是唯一权威字段定义，但其冲突说明引用关系尚未收敛。
3. DATA-CTR-001 / DEC-027：当前 DragonTigerItem Go JSON Tag 与 OpenAPI 仍公开 provider_buy_value、provider_sell_value、provider_deal_value、provider_net_value、provider_net_rate、provider_org_net_value、provider_hot_money_net_value、provider_amount_unit。它们违反已接受的 DEC-027 公开 DTO 边界，仍属契约定义待补，不得归入纯 Provider 内部问题。
4. DATA-CTR-008 与 DATA-CTR-007：DATA-CTR-008 Manifest 仅绑定查询范围和来源版本元数据，不包含 DailyBars 内容结构。不得把 Manifest Schema 的存在解释为 DailyBarsV2 已有 Schema。

### 15.3 本批结论与范围

- 本批 8 项：7 项 pending_evidence，1 项 not_applicable；无 pending_decision、无 blocked、无 freeze-ready。
- DATA-CTR-003 已从 PLAT-002 跨模块冻结门禁排除；其内部质量要求不再列入本任务。
- 本轮没有修改 Data 责任分支、Go DTO、OpenAPI、Schema 或 Golden；没有执行测试/CI。所有已知实现与契约差异仅作为证据，不转为本轮实现整改。
- 下一批转入 News 公开契约核定；GOV-CTR-006 的公共规则只引用本批已发现的适用映射问题，不重复审查其公共 Schema 算法。

## 16. News 首批公开契约定义核定（2026-11-11）

本批覆盖 NEWS-CTR-001～003。当前 News 对跨模块消费者提供同进程只读 Application Port；doc/news/openapi.yaml 明确标注为未来 HTTP 草案，不注册、不属于当前活动公开接口。本轮不审查 Provider 抓取、采集 Worker、数据库实现或内部质量测试。

### 16.1 核定结果

| 契约 | 状态 | 核定结论 |
|---|---|---|
| NEWS-CTR-001 | pending_evidence | 4 Schema/3 Golden 已有；business_time_from/to 的 Go/Schema 均为可选省略，正文 nullable 是文档误写；UTC RFC3339Nano Z 尚未机器约束；system_observed_at 的版本级映射需在 News 数据模型文档明确；relation_refs 缺 relation_status，违反 DEC-021 对 not_provided/已确认空关系的区分 |||| `freeze-ready` 
| NEWS-CTR-002 | pending_evidence | Binding Schema/Golden、Go Validator 已覆盖主体/资源/用途/内容级别绑定、策略集合完整性/tuple 排序去重、conditional 与当前策略 fail-closed 复核；News API 契约正文的 policy_version（Registry vs snapshot_ref）和 resource_ref（version_id vs version_no）仍与机器契约冲突 |||| `freeze-ready` 
| NEWS-CTR-003 | pending_evidence | 当前 active 契约已定义严格双时间 PIT、固定 system_as_of、available_at 缺失排除、稳定排序、HMAC Cursor/15分钟 TTL；旧 News-Research 设计文档仍保留非 PIT 当前版本读取、旧内容级别和旧过滤字段，需明确标记为历史方案；Snapshot Store/查询接线是实现状态，不作为契约定义门禁 |||| `freeze-ready` 

### 16.2 必须保留的语义边界

1. **不新增 News HTTP API 契约：** 当前唯一跨模块读取边界是同进程 NewsEvidenceQuery/Application Port；doc/news/openapi.yaml 只作为未来候选草案，不能作为当前消费者接口或冻结对象。
2. **PIT 时间不能互相替代：** published_at 是发布者声明时间；available_at 必须有首次可用证据；fetched_at/normalized_at 是采集过程时间；observed_at 是单次 Observation 时间。不得用 fetched_at 或 published_at 推导 available_at。
3. **观察时间唯一来源待统一：** 新闻数据模型第14.1节将系统首次观察时间归于 first_observed_at/observation.observed_at，并声明不再使用独立 system_observed_at；NEWS-CTR-001 Evidence DTO Schema 与 NEWS-CTR-003 排序却要求 system_observed_at。需明确该字段是从哪一个权威时间派生，还是应改用现有观察字段。
4. **RelationRefs 状态不可丢失：** DEC-PLAT-002-021 要求区分关系未提供与确认无关系；当前 Evidence Page Schema 只有 relation_refs 数组，空数组无法独立表达 not_provided 与 confirmed-empty。必须明确字段级表达，不得由消费者猜测。
5. **AuthorizationRef 不是访问令牌：** News 仍是 Provider Output/Source/Endpoint 策略交集的唯一授权决策 Owner；Research/Researchflow 只消费不透明引用，读取时按当前策略重新授权。

### 16.3 本批结论与范围

- 本批 3 项均为 pending_evidence；无 pending_decision、blocked、freeze-ready。
- NEWS-CTR-001 的 Schema/Golden 已存在，不代表其字段语义完整；NEWS-CTR-003 可复用现有 Cursor Claims Schema，不因没有独立文件而机械新增 CTR。
- 不把 NEWS-CTR-002 的当前授权复核测试、数据库并发测试、Provider 来源条款核查或采集质量测试列入本轮门禁。
- 本轮只维护 feat/design_doc 项目级契约登记与审计文档，没有修改 News 责任分支文档/代码，没有执行测试或 CI。
- 下一批：Quant 公开契约定义核定；GOV-CTR-006 的公共算法不重开，只核对 Quant 的 resource_ref/purpose/content_level 映射。

## 17. Quant 首批公开契约定义核定（2026-11-11）

本批覆盖 QNT-CTR-001～005，只核对 Quant 对 Researchflow 暴露的公开 Application DTO、Registry、结果引用/Envelope、RiskDecision 和授权 Manifest 定义。纯计算算法实现、模块内 CI、Repository 并发实现和真实 Data Adapter 均不属于本轮。

### 17.1 核定结果

| 契约 | 状态 | 核定结论 |
|---|---|---|
| QNT-CTR-001 | pending_evidence | Registry Entry Schema/Go Validator 已有；DEC-045 已明确 code=metric_code、复合 Indicator 输出拆分为独立 Metric、服务端显式活动 algorithm_version；仍缺首批 8 Indicator 输出组/3 Factor 的正式 Entry/Golden，以及每个 metric_code 的单位、精度、舍入、范围、输入和质量策略值 |||| `freeze-ready` 
| QNT-CTR-002 | pending_evidence | DEC-046 已明确公共请求上下文、参数 Schema 版本边界、算法版本由服务端 Registry 选择、结果按 metric_code 映射为 QuantMetric[]，MACD/Bollinger 复合输出有固定顺序；仍缺完整 Request 与 IndicatorResultPayload Schema/Golden，并依赖 QNT-CTR-001 首批 Entry/样本数策略 |||| `freeze-ready` 
| QNT-CTR-003 | pending_evidence | DEC-047 已明确 stock 单证券日线输入、factor_code/period 对应、20/60 日窗口与 sample_count、factor_code=metric_code 及单 Metric Payload；Data DailyBars v2 缺交易日历版本/interval 证据，完整 Request/FactorResultPayload Schema/Golden 和输入质量分类仍缺 |||| `deferred` 
| QNT-CTR-004 | pending_evidence | DEC-028/029/048/049 已明确三层版本、空输入授权、Ref 投影、Manifest 身份字段及 Trend/Risk 状态语义；仍缺 Envelope/Indicator/Factor Payload Schema/Golden，Ref minItems=1 冲突，Manifest Schema/Go 身份字段未同步，Risk RuleSet 至少一个 block 规则及状态聚合尚未由 DTO Schema/Validator 约束 |||| `freeze-ready` 
| QNT-CTR-005 | pending_evidence | DEC-050 已明确 Quant AuthorizationRef 的 Principal/Resource/Purpose/ContentLevel、策略快照、审计和每次读取复核；当前仅有静态 Golden 校验，无运行时签发/策略 Provider/读取授权/竞态门禁。Manifest Go 仍缺 Schema 字段、拒绝空 refs，且未同步 DEC-048 的身份字段 |||| `freeze-ready` 

### 17.2 已裁决的 Quant 公开契约语义

**QNT-DEC-01：版本身份分离（DEC-PLAT-002-028）**

- 决议：Envelope 与 Manifest 的 schema_version 各自表示本 DTO 自身版本；二者均显式提供 payload_schema_version。Payload 以 (result_type, payload_schema_version) 唯一选择。QuantResultRef 不重复携带 Payload 版本。
- Manifest 1.0.0 直接采用清晰语义，不保留历史字段名特殊解释；三类版本独立演进，未知版本拒绝，不静默回退。
- 后续：对齐 Quant DTO、Manifest Schema、Golden 和指纹字段覆盖。

**QNT-DEC-02：白名单纯函数空输入（DEC-PLAT-002-029）**

- 决议：Request、Envelope、Manifest 的 input_snapshot_refs 均为必需数组；空数组表示明确无外部事实 Snapshot。仅当 Quant Registry 登记为无外部事实纯函数且 ResolvedTemplate 授权 allow_empty_inputs=true 时允许；Quant 必须校验，调用方不得自行授权。
- 不增加 input_mode；数组本身已表达输入集合。Schema 允许空数组，业务授权由 Quant Application 校验；算法版本、参数指纹、result_type、Payload 版本、system_as_of 和输入数组仍参与结果身份/指纹。
- 后续：对齐 Request/Envelope/Manifest Schema、指纹覆盖和校验规则。

### 17.3 其他定义缺口与明确排除

1. QNT-CTR-001 缺少实际 Registry Entry 内容；仅有 Registry Entry Schema 不能证明任何具体 metric/indicator/factor 的公开单位、精度、算法版本和质量策略已冻结。
2. QNT-CTR-002/003 的参数 Schema 不能替代完整 Request/Result 契约；局部算法 Golden 不能自动构成跨模块 DTO 定义。
3. QNT-CTR-004 已有的 Ref/Metric/Trend/Risk Schema 是局部 DTO，不等同完整 QuantResult Envelope；未知 result_type/payload_schema_version 的拒绝语义必须保留。
4. QNT-CTR-005 Manifest 绑定 payload 指纹和输入 Snapshot，但不替代 GOV-CTR-006 AuthorizationRef；Quant 资源 Owner、trusted subject_ref、resource_ref=result_id、purpose_code=quant_analysis、content_level=full 仍需在唯一权威映射中表达。
5. 不纳入本轮：算法计算正确性、指标性能、Quant 单元/Golden 测试、Repository 数据库并发、真实 Data v2 Adapter、授权撤销竞态测试和 CI。这些不改变本轮契约定义判定。

### 17.4 本批结论

- QNT-DEC-01/02 已由 DEC-PLAT-002-028/029 裁决，不再属于 pending-decision。
- QNT-CTR-004/005 转为 pending-evidence；待权威 DTO、Manifest Schema、Golden 和授权映射按决议对齐。
- 不新增 CTR；后续只核对决议落实到权威机器定义的完整性，不重开已裁决语义。

## 18. Research 首批公开契约定义核定（2026-11-11）

本批覆盖 RSC-CTR-001～002。ResearchContext 是 Researchflow Run Context 的组成部分，其跨模块结构归 FLOW-CTR-003 核定；RSC-CTR-001 专指 Evidence，避免把 Context 与 Evidence 混成同一 Schema。Research 不直接调用 Quant，Researchflow 是唯一组合者；DEC/RSC-DEC-001 不重开。

### 18.1 核定结果

| 契约 | 状态 | 核定结论 |
|---|---|---|
| RSC-CTR-001 | pending_evidence | Evidence 1.0.0 Schema/Golden 已有，字段、来源 Owner、derived_from_refs、content_level、AuthorizationRef、JCS 指纹语义较完整；但 Schema 的 date-time 未约束 RFC3339Nano UTC，source_version 条件必填规则缺少 source-owner/versioned-resource 适用映射，system_as_of/available_at 的 PIT 关系仍由文字约束承担 |||| `freeze-ready` 
| RSC-CTR-002 | pending_evidence | Event/Claim/Scenario 项目级 DTO、Schema/Golden 已按 DEC-030～033 对齐核心结构；剩余为 EventRef 存在性/无环、source_times 对应、Claim supersedes 当前版本/并发冲突、Scenario Metric Registry 类型/单位映射及 QuantResultRef 类型/时间绑定，并需对齐 Research 责任模块定义 |||| `freeze-ready` 

### 18.2 RSC-CTR-002 的已裁决项与机器定义进度

1. **RSC-OPEN-003（DEC-PLAT-002-030）：** ResearchEvent 增加必需 time_normalization_rule_version，标识统一 Research 时间规则集版本；source_times 保留原始证据；不增加 rule_id。
2. **RSC-OPEN-004（DEC-PLAT-002-031）：** 增加必需强类型 derived_from_refs，引用前序 Event 不可变版本；不增加 source_refs，来源追溯经 evidence_refs → Evidence.source_ref/source_version 完成。
3. **RSC-OPEN-005（DEC-PLAT-002-032）：** 增加可选 supersedes_version；语义替代时必须设置，只能指向同一 Claim 的当前有效历史版本；禁止并发分叉，历史版本不可变。

以上三项均是跨模块公开 DTO 结构/语义取舍，不是 Repository 或领域实现问题。不得由 Research 分支自行增加私有字段。

### 18.3 适用边界与排除

- ResearchContext 的字段定义在 Quant-Research DTO 文档第3.1节；其 Run 级装配/传播与 FLOW-CTR-003 的 Context Schema 映射一起核定，不新增 Research CTR。
- RSC-DEC-001 已明确 Research 不直接调用 Quant；Quant Evidence 只能由 Researchflow 复核授权后传递，不把该边界重新列为待决。
- RSC-DEC-002 的 changed_at 是 Event 状态迁移提交时间；Research Outbox → Notification Relay 接线属于实现装配，不阻断 DTO 定义冻结。
- RSC-OPEN-007 只影响同一 Evidence ID 下授权快照续期历史的额外能力；现行完整 DTO 不可变、冲突拒绝边界已明确，不阻断当前 RSC-CTR-001。
- 不纳入：Research Repository/MariaDB/Outbox 实现、关系图并发测试、Server Relay、授权撤销竞态测试和 CI。

### 18.4 本批结论

- RSC-OPEN-003/004/005 已由 DEC-PLAT-002-030～032 裁决；RSC-CTR-002 转为 pending-evidence，等待权威 DTO、Schema/Golden 按决议对齐。
- 不新增 CTR 或重复创建 DEC 编号。ResearchContext 仍归 FLOW-CTR-003，不计入 RSC-CTR-001 Evidence Schema。
- 下一步只处理已裁决语义的机器定义落地，不重新发起上述待决项。

## 19. Researchflow 首批公开契约定义核定（2026-11-11）

本批覆盖 FLOW-CTR-001～004。只核对 Server/业务模块消费者可见的 Run/Stage/Attempt、命令、Run Context、Stage Provider Result 与 ResultSummary 语义。Worker 锁、Repository 实现、数据库恢复、CAS 测试和部署不作为契约定义冻结门禁。

### 19.1 核定结果

| 契约 | 状态 | 核定结论 |
|---|---|---|
| FLOW-CTR-001 | pending_evidence | Run/Stage/Attempt 状态枚举、终态和 Template 传播矩阵已有，Run/Stage/Attempt 1.0.0 Schema/Golden 已有；Schema 未完整表达 UTC RFC3339Nano、终态时间/输出字段关系及 ResultSummary 资源绑定，需与 FLOW-CTR-004 收敛 |||| `freeze-ready` 
| FLOW-CTR-002 | pending_evidence | Cancel/Retry/Deadline 的可观察语义、优先级、停止确认和 retry allowlist 已详细定义；但 Server 命令边界的 CancelRun/RetryStage 字段、幂等作用域/指纹、冲突结果和稳定错误尚无唯一权威命令 DTO Schema/等价定义 |||| `freeze-ready` 
| FLOW-CTR-003 | pending_evidence | system_as_of/deadline/template/input_snapshot_refs/originating_request_id/correlation_id 字段规则已明确；但 ResearchContext/ContextSectionRef、resolved template snapshot、可信 caller 与各下游 Adapter 之间的 Context 字段映射缺少独立权威 Schema/Port 定义 |||| `freeze-ready` 
| FLOW-CTR-004 | pending_evidence | Stage Provider Result Envelope 已有字段表，但 output_type/schema_version/payload/quality_summary/diagnostic_refs 缺权威 Schema/Output Registry；ResultSummary Schema 的 quality_status 仍是任意 string，coverage 分母与 summary 资源身份/版本绑定不完整 |||| `freeze-ready` 

### 19.2 跨契约定义关系

1. Run Schema 已包含 system_as_of、deadline_at、template_ref/version/fingerprint、input_snapshot_refs、originating_request_id、correlation_id；这不等同完整 Run Execution Context，因为 ResearchContext/ContextSectionRef 和模板解析快照仍需明确如何被 Stage 消费及固定。
2. ResearchContext 的 DTO 字段表属于 FLOW-CTR-003 的下游 Context 输入边界；RSC-CTR-001 只覆盖 Evidence，不能用 Evidence Schema 替代 Context Schema。
3. Stage Schema 的 output_ref 是不透明资源引用；Stage Provider Result Envelope 是另一层公开 payload 契约，二者不可合并或以 output_ref 字符串代替 Envelope Schema。
4. ResultSummary Schema 的 quality_status 只约束为非空字符串，未固定 valid/degraded/invalid/unknown 等枚举；coverage 只有 [0,1] 范围，没有明确分子/分母和适用阶段集合。
5. Attempt 的 lease_owner、lease_expires_at、fencing_token 属于 Worker 执行模型；消费者可见的 Attempt 状态/引用字段可以冻结，但数据库锁、CAS、lease 恢复和 fencing 竞态测试不作为本轮门禁。

### 19.3 明确排除

- 不将 Run/Stage/Attempt Repository、CAS、数据库事务、锁顺序、租约回收、重启恢复和 Worker 停止确认测试列入契约定义冻结。
- 不将 Server HTTP Handler/路由装配、WebUI 状态展示和跨模块 E2E 作为本轮门禁。
- 不把 FLOW-CTR-002 的内部 lease/fencing 机制扩展成业务模块必须直接消费的 API。
- 不新增 CTR；Run Context 继续归 FLOW-CTR-003，Stage Provider Result/ResultSummary 继续归 FLOW-CTR-004。

### 19.4 本批结论

- 本批 4 项均为 pending_evidence；无新增 pending_decision、blocked、freeze-ready。
- 当前项目级待决项总数保持 5 项：QNT-DEC-01/02、RSC-OPEN-003/004/005。
- 下一批：LLM 公开契约定义核定；不重复审查 FLOW 已明确的状态机/取消/重试原则，不检查 Researchflow 内部 Worker/Storage 实现。

## 20. LLM 首批公开契约定义核定（2026-11-11）

本批覆盖 LLM-CTR-001～003。只核对 Researchflow → LLM 的 Provider Port、Request/Response/Usage/Tool、SSE Event 与 Platform Fault 投影，不核验真实模型/Gateway 能力、Provider 性能、模块测试或 CI。

### 20.1 核定结果

| 契约 | 状态 | 核定结论 |
|---|---|---|
| LLM-CTR-001 | pending_evidence | ProviderConfig、CapabilityState、ModelRequest、ModelResponse 1.0.0 Schema 与 JSON Fixtures 已有，协议范围和字段语义详细；但时间 Schema 只使用 date-time、未约束 RFC3339Nano UTC；ContentPart.annotations Schema 为任意值，而 DTO 约定 Annotation[]；部分条件字段/未知类型约束仍不够精确 |||| `freeze-ready` 
| LLM-CTR-002 | pending_evidence | ModelEvent 1.0.0 Schema 与 SSE Fixture 覆盖完整/中断/terminal/sequence 等主要情形，事件终止和重试规则已明确；但 received_at UTC 约束、sequence_number 初始/合法范围，以及 terminal 后事件的完整公开结构约束未全部落实到机器契约 |||| `freeze-ready` 
| LLM-CTR-003 | pending_evidence | ModelErrorInfo Schema/映射规则已有，Platform Fault Class 到 ModelErrorKind、stable_code、SafeMessage、Retryable 和 Cause 隔离均有文字定义；但依赖 PLAT-CTR-001 的 Fault 公开分类和稳定 Code 语义尚未冻结 |||| `freeze-ready` 

### 20.2 已接受、不再重开的语义

- 首期仅单轮文本输入；多轮/多模态/background/previous_response_id 延后。
- ToolDefinition 是受控可选字段；LLM 只解析 Tool Call，不执行、不批准。
- reasoning_effort/temperature/top_p 等能力仅在当前 Provider+Model+Gateway 证据 supported 且未过期时使用；unknown 不放行。
- 本地 Schema 语义固定 Draft 2020-12；上游 Responses strict 子集不支持的约束不得静默删除。
- Usage.total_tokens 只在 input/output 均明确且无溢出时派生；上游值不一致保留原值并记录诊断。
- SSE sequence_number 若全流提供必须严格连续；整流均缺失可继续但标记 unavailable；同流混合有无按协议异常处理。
- Request ID、Correlation ID、Gateway Request ID、Upstream Request ID 分层保存。
- LLM Adapter 默认不自动重试；是否重试由 Researchflow 的 Deadline/预算/幂等策略决定；流一旦发出事件禁止透明重试。

### 20.3 明确排除

- 不以本地 Qwen/llama.cpp 或 Gateway 当前真实能力未验证为由阻断纯协议契约冻结；能力状态属于运行时证据，unknown 是安全默认值。
- 不检查真实 Gateway E2E、模型输出质量、Token 吞吐、并发性能或 Provider 专项测试。
- Platform HTTP Client/受控 SSE 真实接入与部署验收不属于本轮。
- 不修改 LLM Provider Adapter、Schema、Golden 或任何责任模块代码。

### 20.4 本批结论

- 本批 3 项均为 pending_evidence；无新增 pending_decision、blocked、freeze-ready。
- LLM-CTR-003 依赖 PLAT-CTR-001，不能先于 Platform Fault 公开契约冻结；本轮只登记依赖关系，不重开 Platform 任务。
- 当前项目级待决项仍为 5 项：QNT-DEC-01/02、RSC-OPEN-003/004/005。
- 下一批：Notification 公开契约定义核定；不重复审查 LLM 已接受的协议/重试决议。

## 21. Notification 首批公开契约定义核定（2026-11-11）

本批覆盖 NTF-CTR-001～003。区分跨模块公开边界与 Notification 内部 Channel Adapter 扩展点；不审查真实渠道账号、投递成功率、Worker/Repository 测试或部署。

### 21.1 核定结果

| 契约 | 状态 | 核定结论 |
|---|---|---|
| NTF-CTR-001 | pending_evidence | Envelope + 三类业务 Payload 共 4 份 Draft 2020-12 Schema、6 个 Golden 已有；fingerprint 算法、event_type/schema_version/source_module 注册键和首批业务事件映射已明确；但 occurred_at/published_at/业务时间均只使用 date-time、未约束 RFC3339Nano UTC，Payload reason_code/opaque ref 格式和 subject_ref 与 payload 主体一致性未完整机器化 |||| `freeze-ready` 
| NTF-CTR-002 | pending_evidence | Inbox 双唯一键、Publish 幂等键、Delivery 唯一键、Subscription/Channel/Template 快照、Delivery 状态机、unknown 对账和人工重试语义已有详细文字；但 Subscription、DeliveryPlan、DeliveryRecord、DeliveryAttempt、DeliveryStateEvent 缺完整 Schema 或唯一权威跨模块 Application Port 定义 |||| `freeze-ready` 
| NTF-CTR-003 | not_applicable | ChannelConfigSnapshot、ChannelAdapter、ChannelResolver、RecipientResolver、ChannelSendRequest/Result 是 Notification 内部 Adapter SPI；Server 只提供 Platform Secret/BoundDoer 等组合依赖，不作为独立跨模块业务契约。其内部能力/重试适配不属于 PLAT-002 |||| `not_applicable` 

### 21.2 必须保留的公开边界

1. NotificationEvent.event_id 由生产方生成，不得复用 RunRef、RiskDecision ID 或 ResearchEvent ID；source_event_ref 指向来源领域对象/版本；subject_ref 是业务主体引用；三者不可混用。
2. Fingerprint 使用 RFC 8785 JCS + SHA-256，覆盖 event_type、schema_version、source_module、subject_ref、occurred_at 和 payload；不覆盖 event_id、idempotency_key、correlation_id、published_at、接收时间或传输 Header。
3. Inbox=(source_module,event_id)，Publish=(source_module,idempotency_key)；同键同 fingerprint 幂等返回，冲突 fingerprint 拒绝；不能把这两种唯一键合并。
4. Delivery 状态 queued/sending/retry_wait/sent/skipped/blocked/failed/unknown 必须区分；unknown 表示外部副作用不确定，先对账或依赖已验证幂等能力，不得盲目重发；sent 只表示渠道明确接受，不代表用户已阅读。
5. Subscription、ChannelConfig、Template 的版本在 DeliveryPlan 创建时固定；历史 Delivery 不因配置更新而改写。发送前重新复核当前退订/授权，但不得把最新版本静默替代原快照。
6. Channel SPI 不扩展为公共协议：未来新增渠道只扩展 Notification 内部 Adapter Registry；不因新增渠道抽取 Platform 通用通知协议，也不要求 Quant/Research/Researchflow 直接了解渠道能力。

### 21.3 本批结论

- 本批 3 项：NTF-CTR-001/002 pending_evidence，NTF-CTR-003 not_applicable；无 pending_decision、blocked、freeze-ready。
- NTF-CTR-003 的内部 Channel Adapter SPI 从 PLAT-002 跨模块冻结门禁移出；Delivery retry/unknown 外部语义保留在 NTF-CTR-002。
- 不纳入：真实渠道 E2E、账号/Secret 可用性、MariaDB 唯一约束测试、Worker 恢复、各渠道幂等/对账能力验收、CI。
- 不新增 CTR；下一批核对 Server/WebUI 对 Researchflow 新 API 的公开 HTTP 契约，并收敛十模块整体冻结统计。

## 22. Server / WebUI Researchflow HTTP 契约核定（2026-11-11）

本批只核对 Server 作为唯一 HTTP 宿主时，WebUI 与 Researchflow 之间的跨模块公开 HTTP 契约。Server 内部 DI、Worker 装配、Migration 注册、部署和 Nginx 路由验收不属于本轮。

### 22.1 核定结果

| 契约 | 状态 | 核定结论 |
|---|---|---|
| SRV-CTR-001 | pending_evidence | Server Composition Root/统一 HTTP Host 的职责和边界已定义；但 DEC-018/019 退役旧 Analysis API 后，新 Researchflow HTTP 契约没有唯一权威 OpenAPI/Schema 或字段级定义。缺少新 API 路径/方法、CreateRun/Run/Stage/Event 请求响应 DTO、错误投影、认证授权、分页/Cursor 与 WebUI 消费映射 |||| `deferred` 

### 22.2 证据与边界

1. doc/server/README.md 已明确 Server 是唯一组合根和 HTTP 宿主；模块提供 Application Port/Handler，Server 只注册经批准的公开 Handler，不自动暴露所有内部 Port。这一架构职责可作为 SRV-CTR-001 的稳定边界。
2. doc/web/API_CONTRACT.md 第3节仍记录 POST /api/v1/analysis/tasks、GET /analysis/tasks/{task_id}、旧 Task Events 和 /analysis/runs/{run_id}；这些是 DEC-018/019 已退役的旧 Analysis API，不是新 Researchflow API 的候选契约。
3. doc/api/contracts/分析任务.openapi.yaml 同样属于旧 Analysis API。按 DEC-019，旧 CreateTask/同步 Run 的未知 TaskType、symbols 上限100、旧 idempotency 规则等历史约束不得迁移到新 CreateRunCommand。
4. 当前文档树中未发现新的 Researchflow HTTP OpenAPI/JSON Schema；Researchflow Application Port/Run/Stage DTO 是同进程模块契约，不会自动定义 HTTP 路径、状态码、认证、错误响应、分页和事件传输方式。
5. 新 HTTP API 至少需要明确：CreateRun 请求与响应、GetRun/RunDetail、CancelRun、Run/Stage/Event 查询边界、错误码到 HTTP 状态码映射、认证/资源授权、Request/Correlation ID、分页/Cursor 和事件顺序/重放语义。这里只登记缺口，不替项目负责人决定路径命名、SSE 或轮询等传输选择。
6. PLAT-CTR-001 的 Fault→HTTP 错误投影、PLAT-CTR-003 的 Session/Principal 原语以及 FLOW-CTR-001～004 的 Run/Stage/Event DTO 是依赖项；不重复创建 Server 侧同义字段定义。

### 22.3 WebUI 范围

- WebUI 是新 HTTP 契约的消费者，不拥有 Run/Stage/Attempt/ResultSummary 领域语义。
- WebUI CTR-011～049 中纯页面、布局、交互、DOM 回归、产品预留功能不属于跨模块公开 API 契约，不阻断 SRV-CTR-001。
- WebUI 对 Data API 的消费仍归 DATA-CTR-001/004/006；本批不重复审查行情接口。
- 旧 Analysis API 前端路径只作为退役历史记录；不得据此冻结旧 API，也不得要求新 API 兼容旧路径。

### 22.4 本批结论

- SRV-CTR-001：pending_evidence；无新增 pending_decision、blocked 或 freeze-ready。
- 旧 Analysis API 已退出活动冻结范围；新 Researchflow HTTP API 是唯一待补的服务端公开契约，不新增 CTR。
- 不纳入：Server DI/Bootstrap 实际装配、Session Middleware 实现、WebUI 登录页面、Nginx/HTTPS/部署、HTTP E2E、CI。
- 下一步应收敛当前十模块契约状态与待决项总数，再按既有 CTR 逐项登记 freeze-ready；不再进行实现接线审查。

## 23. INT-CTR-001 业务事件映射契约核定（2026-11-11）

INT-CTR-001 是 43 个正式 CTR 之一，覆盖 Quant、Research、Researchflow 三类业务事件生产方到 Notification Event Envelope 的公开映射。本批只核定事件类型、Payload 字段、时间/身份和幂等语义，不审查 Outbox/Inbox 实现或事件发布接线。

### 23.1 核定结果

| 契约 | 状态 | 核定结论 |
|---|---|---|
| INT-CTR-001 | pending_evidence | 三类事件 event_type/schema_version/source_module、触发时点、subject_ref、source_event_ref、idempotency_key、correlation_id、Payload 字段及 JCS fingerprint 输入已明确；Envelope/Payload Schema 与 6 个 Golden 已有；但时间字段 Schema 未约束 RFC3339Nano UTC，subject_ref 与 Payload 主体字段一致性、occurred_at 与来源时间的逐事件绑定、changed_block_reason_codes 字典序规则未完整机器化 |||| `freeze-ready` 

### 23.2 已明确的三类映射

- researchflow.run.terminal：Run 进入 succeeded/partial/failed/cancelled 终态并通过持久化线性化后发布一次；occurred_at=finished_at；subject_ref=run_ref；payload 含 source_event_ref、run_ref、status、termination_reason、system_as_of、finished_at、result_summary_ref、template_ref/version。
- quant.risk_decision.changed：首次形成决策，或 status/硬阻断 reason code 集合发生变化时发布；occurred_at=evaluated_at；subject_ref 是单一风险评估主体，不得使用 decision_id 代替；changed_block_reason_codes 去重并按字典序排序。
- research.event.lifecycle_changed：ResearchEvent 生命周期状态真正迁移并提交后发布；occurred_at=changed_at；subject_ref=research_event_ref；source_event_ref 指向不可变领域版本；普通 Evidence 追加或描述修订不触发事件。

### 23.3 保留边界

1. NotificationEvent.event_id 是 Notification 身份，不得复用 RunRef、RiskDecision ID 或 ResearchEvent ID；source_event_ref 与 subject_ref 是不同字段，不可互换。
2. payload_fingerprint 只覆盖 event_type、schema_version、source_module、subject_ref、occurred_at、payload；不覆盖 event_id、idempotency_key、correlation_id、published_at、接收时间和 Header。
3. Outbox 原子提交、Inbox 双唯一键、重复投递和事件生产方真实接线属于实现符合性，不作为本轮定义冻结条件。
4. 资源限制数值由组合根显式注入；当前没有统一批准的固定默认值，不得把建议数值写成冻结契约。

### 23.4 本批结论

- INT-CTR-001 保持 pending_evidence；无 pending_decision、blocked 或 freeze-ready。
- 三类事件语义已接受，不新增待决项；剩余是时间/跨字段约束在权威 Schema 中的表达。
- 不新增 CTR，不修改 Quant/Research/Researchflow/Notification 责任模块，不执行测试/CI。

## 24. PLAT-002 十模块契约定义状态总表（历史快照，已由第59节覆盖）

本表以第3节 43 个正式 CTR 为唯一统计口径。只统计当前跨模块公开契约定义；模块内部 SPI、旧 API 退役别名、UI 页面级项目和实现符合性证据不计入正式 CTR 状态。

| 状态 | 数量 | 含义 |
|---|---:|---|
| freeze-ready | 37 | 最终收敛状态见第59节；包括公共规则、Platform、Data 首期、News、Quant 首期、Research、Researchflow、LLM、Notification 和业务事件映射 |
| pending-evidence | 0 | 最终收敛后没有未分类的公共契约定义缺口；机器 Schema/Golden/实现同步差异按 DEC-051 归入 L2 符合性，或列入明确 deferred 范围 |
| deferred | 4 | DATA-CTR-005、DATA-CTR-007、QNT-CTR-003、SRV-CTR-001；触发条件见第59节 |\n| pending-decision | 0 | 所有待决均已由既有 DEC-PLAT-002-028～050 及 DEC-051 裁决 |
| blocked | 0 | 当前没有无法通过权威决议消解的公开定义冲突 |
| not_applicable | 2 | DATA-CTR-003、NTF-CTR-003；仅模块内部 Provider/Channel SPI，不属于跨模块公开契约 |
| frozen | 0 | 尚无项目负责人确认冻结的正式 CTR |
| **正式 CTR 合计** | **43** | 37 freeze-ready + 4 deferred + 2 not_applicable；不新增编号，不重复计算历史别名 |

### 24.1 五项已裁决内容

五项待决已全部裁决：QNT-DEC-01/02 对应 DEC-PLAT-002-028/029；RSC-OPEN-003/004/005 对应 DEC-PLAT-002-030～032。当前不再存在 pending-decision 项。

### 24.2 已排除或退役范围

- DATA-CTR-003、NTF-CTR-003 为 not_applicable，不要求项目级跨模块冻结；其内部扩展约定归责任模块管理。
- DEC-018/019 退役的旧 Analysis HTTP 契约、CTR-047 及旧 Task/StageEvent HTTP DTO 不计入当前活动 CTR 冻结统计；新 Researchflow HTTP 契约归 SRV-CTR-001 + FLOW-CTR-001～004。
- WebUI CTR-011～049 中页面布局、交互、DOM、产品预留项不是跨模块 API 契约；WebUI 只消费 Server/模块已冻结的公开 HTTP DTO。
- DATA-FLOW-B、LLM-FLOW-B 是条件性 deferred 决策，不是新增正式 CTR，也不计入 43 项状态统计。

### 24.3 阶段结论

- 43 项正式 CTR 已全部完成本轮定义口径核定；不再保留“待审查模块/待全量盘点”的开放任务。
- 本轮最终收敛状态见第59节：37 项 freeze-ready、4 项 deferred、2 项 not_applicable；0 项 pending-evidence、0 项 pending-decision。
- 37 项 freeze-ready 可提交项目负责人统一确认冻结；4 项 deferred 不属于当前首期冻结范围。不得重新启动十模块全量扫描或把 L2 实现符合性重新加入 L1 冻结门禁。


## 25. 五项待决裁决完成记录（2026-11-11）

- 五项裁决依据用户明确授权，由 ChatGPT 完成技术 Review 并直接作出最优逻辑决议；正式记录见 [项目级契约决策记录 DEC-PLAT-002-028～032](./decisions/项目级契约决策记录.md)。
- 决议覆盖 QNT-DEC-01/02、RSC-OPEN-003/004/005；均为项目级公开契约语义决议，不代表 DTO/Schema/Golden 已完成。
- QNT-CTR-004、QNT-CTR-005、RSC-CTR-002 从 pending-decision 转为 pending-evidence；43 项正式 CTR 总数不变。
- 本轮不修改责任模块契约、DTO、Schema 或实现，不执行测试/CI/部署。


## 26. Quant / Research 裁决落地第一批（2026-11-11）

本批只更新项目级权威 DTO、项目级 Schema/Golden 和登记簿；不修改 `doc/quant`、`doc/research` 责任模块文档/Schema/代码，不执行模块测试或 CI。

### 26.1 本批完成

1. QuantAnalysisRequest、QuantResultRef、QuantResult Envelope 的项目级 DTO 统一空输入数组语义；QuantResult Envelope 新增必需 `payload_schema_version`，Payload 选择键固定为 `(result_type, payload_schema_version)`。
2. QuantResult Manifest Schema/Golden 已补齐 `payload_schema_version`、`algorithm_version`、`parameter_fingerprint`，并增加非空输入与纯函数空输入两份 Golden；责任模块 Build/Verify 的非空输入限制和旧 schema_version 语义仍冲突。Manifest `schema_version` 表示 Manifest 自身版本；manifest_fingerprint 按 JCS 对除 result_id/manifest_fingerprint 外全部字段计算。
3. ResearchEvent DTO 增加必需 `time_normalization_rule_version`、`derived_from_refs`；Claim/Hypothesis DTO 增加可选 `supersedes_version`，并将合并/拆分语义从旧 `source_refs` 统一收敛到 Event 血缘字段。
4. 新增项目级 ResearchEvent、Claim/Hypothesis、Scenario 1.0.0 JSON Schema 与 Golden，明确强类型 EventRef、EventSourceTime、Hypothesis 条件字段、ScenarioCondition 与递归 ConditionGroup。

### 26.2 尚未通过的项目级契约证据

- QNT-CTR-004：Quant 责任模块 QuantResultRef Schema 仍限制 input_snapshot_refs 至少一项；Envelope 缺完整 Schema/Golden；result_type 的强类型 Payload 覆盖未完整。
- QNT-CTR-005：Manifest Schema/Golden 已更新，但需与 Quant Record/VerifyPayload 权威指纹算法及 AuthorizationRef 映射定义核对；项目级文档不替代 Quant 模块实现契约。
- RSC-CTR-002：Scenario Schema/Golden 已补齐；Event/Claim/Scenario 的引用存在性、派生无环、版本目标当前性和时间证据关联需要 Research 权威语义规则明确。

### 26.3 本批状态

- QNT-CTR-004、QNT-CTR-005、RSC-CTR-002 均保持 `pending_evidence`；不把新增文件等同于完整冻结。
- 本批不新增 CTR、不重开 DEC、不改动责任模块目录。
- 下一步仅核对剩余的权威定义缺口：Quant ResultRef/Envelope/Payload Schema 映射、Manifest 指纹与授权映射、Research 跨字段约束及责任模块定义一致性。


## 27. Quant 权威定义冲突复核（2026-11-11）

### 27.1 QNT-CTR-004

- QuantResultRef 责任模块 Schema `doc/quant/schema/QNT-CTR-004-quant-result-ref-1.0.0.schema.json` 将 `input_snapshot_refs` 设置为 `minItems: 1`；项目级 DTO 已按 DEC-029 允许经双重授权的空数组。两者不一致，不能宣告 Ref 契约证据闭合。
- 责任模块已有 Factor QuantMetric、TrendResult、RiskDecision 局部 Schema/Golden；Factor Metric 仅覆盖首批三个 Factor，Trend/Risk 是各自局部 DTO。不能将这些局部结构视为完整 QuantResult Envelope。
- Indicator 尚无独立完整 Payload Schema；market/industry/stock/strategy/backtest/evaluation 尚无可确认的强类型 Payload Schema/Golden。枚举列出类型不等于 Registry 已开放类型。
- QuantResult Envelope Schema/Golden、Ref/Envelope/Manifest 字段投影与一致性规则仍缺。项目级 Schema 不复制或替代 Quant 模块内部 DTO 实现定义。

### 27.2 QNT-CTR-005

- 项目级 Manifest Schema/Golden 已将 Manifest `schema_version` 与 `payload_schema_version` 分离，并允许空输入数组。
- Quant 责任模块 Manifest Build/Verify 文档仍要求非空 Snapshot；历史文档还将 Manifest `schema_version` 描述为 Payload Schema 版本，与 DEC-028/029 冲突。
- Quant Manifest 规范化指纹、Record/VerifyPayload 的权威字段集合、Ref 与 Manifest 共同身份字段，以及 AuthorizationRef 的可信主体和资源映射尚未形成一致权威定义。
- 上述属于责任模块契约证据缺口；本轮仅记录差异，不修改 `doc/quant/**`、实现或测试。

### 27.3 处理结论

- QNT-CTR-004、QNT-CTR-005 继续保持 `pending_evidence`，不新增 CTR、不重开 DEC。
- 后续如需调整 Quant 责任模块 Schema/文档，应在明确的 Quant 模块任务中按本登记簿已冻结语义执行；本项目级契约冻结任务不越权代改模块定义。


## 28. RSC-CTR-002 跨字段语义与责任模块差异复核（2026-11-11）

### 28.1 ResearchEvent

- 项目级 DTO/Schema 已具备必需 `time_normalization_rule_version`、`source_times`、强类型 `derived_from_refs`，Golden 的时间来源引用可与 `evidence_refs` 对照。
- Schema 只能验证 EventRef 结构，不能证明目标版本存在、不是自引用或整张派生图无环；这些必须由 Research Domain 在发布前按不可变版本图校验。
- `source_times[].evidence_ref` 必须属于当前 Event 的 `evidence_refs`；非 null 的 occurred_at/published_at/available_at 必须能由同 time_type 原始证据及规则版本复核。当前 JSON Schema 未表达这些跨数组/规范化关系，责任模块文档只记录了部分来源类型存在性检查。
- 发现项目级 Schema 曾包含 DTO 未定义的 `supersedes_event_version`。按 DEC-031，Event 版本血缘统一由 `derived_from_refs` 表达，已从项目级 DTO/Schema 移除该冗余字段；Research 责任模块文档仍保留旧字段，后续模块对齐时需清理。

### 28.2 Claim / Hypothesis

- 项目级 Schema 已包含可选 `supersedes_version`，并在设置时要求 `claim_version >= 2`。
- JSON Schema 无法验证 `supersedes_version < claim_version`、目标版本存在且为写入前当前有效版本、并发替代不产生分叉；这些必须由 Research 权威写入校验完成。
- Research 责任模块文档仍标记 `supersedes_version` 未解决/缺失，与 DEC-032 和项目级 DTO 不一致。

### 28.3 Scenario

- Scenario Schema 已支持递归 ConditionGroup，并在 `probability_status=available` 时要求 `quant_evaluation_ref`。
- `expected_value` 当前允许任意非空 object，未按 condition_type/metric_ref/operator/unit 定义封闭类型；这不满足 typed scalar/object 的强类型要求。
- `quant_evaluation_ref` 仍引用 Quant 责任模块 Ref Schema；当前 Ref Schema 的 Snapshot 最小项约束与 DEC-029 冲突。项目级跨字段还需验证 result_type=evaluation、QuantResultRef.system_as_of 与 Scenario.system_as_of 完全一致及当前授权复核责任。

### 28.4 状态与边界

- RSC-CTR-002 继续保持 `pending_evidence`；不新增 CTR、不重开 DEC。
- 本轮仅更新项目级 DTO/Schema 和治理登记；未修改 `doc/research/**`、Research 实现或测试，不执行 CI/部署。


## 29. ScenarioCondition 类型结构裁决落地（2026-11-11）

- DEC-PLAT-002-033 已确定 `expected_value` 只允许 string/number/boolean scalar 或非空 scalar 数组；`in/not_in` 使用数组，`exists/not_exists` 省略，其他比较运算使用单值。
- 项目级 Scenario DTO/Schema 已按上述结构收紧；禁止任意 object/map、嵌套对象和表达式。
- Metric 条件仍需将 `metric_ref` 对应 Registry 的 value_type/unit/precision 映射到比较值校验；该权威映射由 Quant Registry 提供，Researchflow 负责按可信 Registry 元数据校验，不由 Research 复制。
- RSC-CTR-002 保持 pending_evidence；本裁决不代表 Registry 映射或 QuantResultRef 绑定证据已通过。


## 30. GOV-CTR-001 公共规则版本化与冻结评估（2026-11-11）

- DEC-PLAT-002-034 已确定 GOV-CTR-001 公共规则 1.0.0，唯一权威文本为 `doc/contract/rules/GOV-CTR-001-公共JSON类型数值与空值规则.md`。
- 公共规则覆盖 JSON 类型选择、Decimal 语法/无损转换、integer 安全范围、number 近似数值和舍入声明、ratio/percentage_point、单位声明、缺失/null/zero/empty/unknown/unavailable/missing/degraded/invalid 区分及质量/执行状态分离。
- 项目级公共规则不规定各字段统一 scale/精度/范围/舍入/容差；这些由字段生产方 Schema/Registry 定义，分别在 DATA/QNT/Research/News/LLM 等 CTR 验收，不再作为 GOV-CTR-001 本身的冻结阻断。
- GOV-CTR-001 状态调整为 `freeze-ready`；43 项总数不变，freeze-ready 2、pending-evidence 39、pending-decision 0、blocked 0、not_applicable 2、frozen 0。


## 31. GOV-CTR-002 公共时间与 PIT 规则版本化（2026-11-11）

- DEC-PLAT-002-035 已确定 GOV-CTR-002 公共规则 1.0.0，唯一权威文本为 `doc/contract/rules/GOV-CTR-002-公共时间与PIT规则.md`。
- 公共规则明确 RFC3339Nano UTC、交易日日期、occurred_at/published_at/available_at/system_observed_at/system_as_of/valid_time/fetched_at/normalized_at/created_at/deadline_at 的不同语义；固定 Run system_as_of 传播、available_at 不推断和严格 PIT 双边界。
- 本规则不替代各领域 Provider 时间来源映射、规范化优先级、交易日历版本或字段 required/nullable；这些仍由对应领域 CTR 验收。
- GOV-CTR-002 状态调整为 `freeze-ready`；43 项总数不变，freeze-ready 3、pending-evidence 38、pending-decision 0、blocked 0、not_applicable 2、frozen 0。


## 32. GOV-CTR-003 公共标识符与幂等规则版本化（2026-11-11）

- DEC-PLAT-002-036 已确定 GOV-CTR-003 公共规则 1.0.0，唯一权威文本为 `doc/contract/rules/GOV-CTR-003-标识符与幂等作用域规则.md`。
- 公共规则区分领域 ID、request_id、originating_request_id、correlation_id、event_id、idempotency_key 和 trace_id/span_id；明确各自作用域、稳定性、不可替代关系及通用幂等冲突语义。
- 命令级 fingerprint 精确字段、保留期、Header/Context/Worker 传播、事件 Envelope 字段和真实幂等执行仍由各领域 CTR/PLAT-CTR-002 验收，不阻断公共语义本身冻结。
- GOV-CTR-003 状态调整为 `freeze-ready`；43 项总数不变，freeze-ready 4、pending-evidence 37、pending-decision 0、blocked 0、not_applicable 2、frozen 0。


## 33. GOV-CTR-004 Schema 版本与兼容规则版本化（2026-11-11）

- DEC-PLAT-002-037 已确定 GOV-CTR-004 公共规则 1.0.0，唯一权威文本为 `doc/contract/rules/GOV-CTR-004-Schema版本兼容与废弃规则.md`。
- 公共规则明确独立 DTO Schema 版本、SemVer 变化边界、closed/open enum、未知字段策略、消费者显式支持版本、Schema/Golden 不可变、废弃字段元数据和无消费者初版接口的直接退役条件。
- 逐 DTO/Event 的 schema_version、unknown field/enum 行为、消费者支持矩阵和迁移证据仍由各领域 CTR 验收，不阻断公共规则本身冻结。
- GOV-CTR-004 状态调整为 `freeze-ready`；43 项总数不变，freeze-ready 5、pending-evidence 36、pending-decision 0、blocked 0、not_applicable 2、frozen 0。


## 34. GOV-CTR-006 公共授权快照契约定义冻结评估（2026-11-11）

- AuthorizationRef 与 PolicySnapshot 各自必需 `schema_version=1.0.0`；两份 Schema/四份 Golden 已同步。
- Schema 已约束 RFC3339Nano UTC、conditions 唯一、policies 非空/唯一；字典序、policy_version/snapshot_ref 绑定由明确的语义 Validator 规则负责。
- 四类 Owner 的 subject/resource/purpose/content_level 映射、conditional/allowed/denied/unknown 语义、读取时当前授权复核、撤销/过期/竞态边界及 JCS+SHA-256 指纹算法均有唯一权威定义；Data AuthorizationRef 与 DATA-CTR-008 Snapshot Manifest Golden 的 resource_ref、subject_ref、purpose_code 已逐字段核对一致。
- GOV-CTR-006 转为 `freeze-ready`。运行时 Owner/Consumer/CI/并发安全测试属于 L2 实现符合性，不阻断本轮项目级契约定义冻结。
- 43 项统计更新为 6 freeze-ready、35 pending-evidence、0 pending-decision、2 not_applicable、0 frozen。


## 35. PLAT-CTR-001 Fault 与 HTTP 错误投影契约定义收敛（2026-11-11）

- Platform Fault 仅是进程内错误，不得直接 JSON 序列化；Cause、Dependency 和内部错误链不进入公开响应。Fault 的内部字段及 Validate 规则继续以 Platform 第 4 节为准。
- 新增项目级唯一 HTTP Error Envelope：`{"error":{"class","code","message","retryable","request_id?"}}`。`request_id` 为兼容性可选字段；若返回，必须与响应头 `X-Request-ID` 一致。消费者必须以响应头为权威关联值，不得依赖 Body 字段存在。
- 固定 Class→HTTP 状态映射：validation=400、unauthenticated=401、forbidden=403、not_found=404、conflict=409、unavailable=503、timeout=504、internal=500。不得通过新增 Class 暗中扩展；新增类别须升级契约。
- `code` 使用稳定、机器可读、带 Owner 命名空间的标识；Owner 负责其命名空间内的错误码词汇与语义，禁止复用、临时拼接请求数据或将凭证/上游原文写入 Code。
- `message` 是可公开的脱敏摘要，不得包含 Cause、SQL/DSN、Token/Key、完整 Provider 响应或堆栈。内部 Fault 的 SafeMessage 映射为公开 message。
- `retryable` 只是技术重试候选提示；仅 unavailable/timeout 可为 true。它不要求消费者重试；重试仍受幂等性、策略、预算和总 Deadline 约束。Retry-After 只有在服务端明确给出等待时间时才返回，不由 retryable 自动推导。
- 已新增项目级 HTTP Error Schema 和覆盖 8 种 Class/状态码映射的 Golden。PLAT-CTR-001 转为 `freeze-ready`；模块各自错误码目录、Fault 生产/HTTP Adapter 映射测试和消费者行为属于 L2 实现符合性，不阻断本轮定义冻结。
- 43 项统计更新为 7 freeze-ready、34 pending-evidence、0 pending-decision、2 not_applicable、0 frozen。


## 36. LLM-CTR-003 Platform Fault 映射定义冻结评估（2026-11-11）

- Platform 依赖项 PLAT-CTR-001 已形成 1.0.0 项目级公开 Fault Class、Code 命名和安全投影规则，不再存在“底层 Fault 公共分类尚未冻结”的阻断。
- LLM 责任模块文档第 7/11 节已定义全部 Platform Fault Class 到 ModelErrorKind 的映射，并明确 response_too_large 特殊优先级、stable_code、SafeMessage、Retryable 保留及 Dependency/Cause/原始错误文本隔离。
- ModelErrorInfo 1.0.0 Schema 已明确 kind、stable_code、retryable、可选 http_status/retry_after_seconds、safe_message 的类型和约束；Request ID 由 ModelResponse.request_id 承载，不在嵌套错误对象重复。
- LLM-CTR-003 转为 `freeze-ready`。LLM Adapter 对真实 Fault 的映射、错误码词汇和 Researchflow 消费者行为属于 L2 实现符合性，不阻断项目级公开映射定义冻结。
- 43 项统计更新为 8 freeze-ready、33 pending-evidence、0 pending-decision、2 not_applicable、0 frozen。


## 37. PLAT-CTR-002 Request Context 契约定义收敛（2026-11-11）

### 37.1 定义核对

- Platform 的 Request ID 原语、可信入站策略、出站传播和 Request ID Golden 已有唯一可核对的 Go API 与行为定义。
- `Principal` 的进程内结构为 `Kind/AuthMethod/Subject/Scopes`；由可信 Platform Guard/Server Session Middleware 根据认证结果写入 Context，`PrincipalFromContext` 返回副本。其身份种类、认证来源、凭证和 Scope 规则归 PLAT-CTR-003，不在本 CTR 重复定义。
- `context.Context` 只承载同步调用树的 Request ID、Principal 和取消/Deadline 信号；异步任务不持久化 Context/Principal，来源 ID 和相关性由 GOV-CTR-003 及领域 CTR 定义。
- 出站只传播可信 Request ID，不自动传播 Authorization、Cookie 或其他凭证；无可信 Context ID 时删除调用方预置 Header。

### 37.2 冻结门禁判断

- 已新增唯一项目级权威文件 `doc/contract/rules/PLAT-CTR-002-Request-Context契约.md`，版本 1.0.0。
- 该契约由 Header 规则与进程内 Go API 构成，无独立 JSON DTO；JSON Schema/OpenAPI 不适用，现有 Request ID Golden 和 Go Contract Test 是等价机器证据。
- 既有审计指出的 Root Server 到真实出站调用端到端测试、旧 Middleware 的生产暴露拓扑证明，属于实现符合性/部署验收，不是公开定义缺口，不阻断本轮契约定义冻结。
- 不把 Worker 的 correlation_id、originating_request_id 或领域授权快照扩展为 Platform Context；其权威分别由 GOV-CTR-003 和领域契约负责。

### 37.3 结论

- PLAT-CTR-002：由 `pending_evidence` 调整为 `freeze-ready`。
- 本次不标记 `frozen`；正式冻结仍需项目负责人确认 1.0.0。
- 43 项正式 CTR 统计更新为：9 freeze-ready、32 pending_evidence、0 pending-decision、0 blocked、2 not_applicable、0 frozen。
- 不修改 Platform 责任模块文档、代码、Schema 或测试；不修改其他分支，不执行 CI。


## 38. PLAT-CTR-004 Diagnostics Observation 与 Runner 定义收敛（2026-11-11）

### 38.1 公开定义核对

- Observation 的五态、字段类型、reason_code 条件必填、safe_message/evidence_ref/attributes 边界、UTC 时间和纳秒 duration 已形成完整定义。
- attributes 值类型明确收敛为 `map[string]string`；不承载 number、boolean、array、object 或任意领域 JSON。该定义与现有 Go DTO 和 1.0.0 Schema 一致，优先采用已经存在的窄类型，不为未存在的消费者扩大类型。
- Schema 定义结构与基础约束；Go `Observation.Validate()` 负责 UTF-8 字节长度、TrimSpace 非空、Unicode 控制字符和 attributes 总字节预算。Schema 字符长度是结构性上限，不能替代 Go 运行时验证。
- Runner 输出时统一将 checked_at 转为 UTC；缺失时间和 duration 由 Runner 补齐。非 healthy 必须提供 reason_code；healthy 可省略。
- Check/Runner API、执行上限、超时、取消、顺序稳定、required skipped 归一化及 Summary 聚合优先级已汇总为唯一进程内强类型定义。
- Platform Report 不是完整公开 Diagnostics HTTP DTO；Server 的外层响应 Schema/Golden 归 Server CTR，不在 PLAT-CTR-004 重复定义。

### 38.2 冻结门禁判断

- 已新增唯一项目级权威定义 `doc/contract/rules/PLAT-CTR-004-Diagnostics-Observation与Runner契约.md`，版本 1.0.0。
- 现有 Observation 1.0.0 Schema 和 healthy/unavailable Golden 继续作为机器契约资产，不复制或修改 Platform 责任模块文件。
- Go Validator 比 JSON Schema 执行更严格的运行时字节/总量约束，是明确的职责分工，不再视为未裁决的类型冲突；Schema 验证通过不替代 Go Validator。
- Schema 与 Go 边界双重校验测试、Server 完整 HTTP Report Schema、领域 Adapter 和生产消费者接线均属于实现符合性/Server 契约，不阻断本 CTR 定义冻结。

### 38.3 结论

- PLAT-CTR-004：由 `pending_evidence` 调整为 `freeze-ready`。
- 本次不标记 `frozen`；正式冻结仍需项目负责人确认 1.0.0。
- 43 项正式 CTR 统计更新为：10 freeze-ready、31 pending_evidence、0 pending-decision、0 blocked、2 not_applicable、0 frozen。
- 不修改 Platform 责任模块文档、代码、Schema 或测试；不修改其他分支，不执行 CI。


## 39. PLAT-CTR-003 身份、授权与 Session 原语定义收敛（2026-11-11）

### 39.1 定义核对与裁决

- DEC-PLAT-002-017 已明确 Owner Session 与 Owner/Service Bearer 分离；本轮新增 DEC-PLAT-002-040，明确 Principal 必须保留 AuthMethod，防止 owner_bearer 与 owner_session 因 Kind=owner 而混权。
- Principal 统一为 Kind/AuthMethod/Subject/Scopes；owner_session 不携带 platform:admin 或 Service Scope；Bearer Router 仍只处理 Bearer，Server Session Middleware 独立验证 Cookie/Session/CredentialVersion。
- Platform Session 原语的密码哈希、Owner CredentialStore、SessionToken/SessionStore、Cookie、CSRF、LoginRateLimiter 和 SecurityAuditWriter 已按现有强类型 API 收敛为 1.0.0 契约。
- Service Scope 固定为 platform:admin（仅 Owner Bearer）、data:read、data:diagnose、research:run、research:read（Service 可分配）；不为未出现的服务调用预建 Scope。
- Server 登录 HTTP API、路由矩阵、WebUI 交互、HTTPS/可信代理部署仍归 SRV-CTR-001 和后续实现符合性；领域资源授权继续归 GOV-CTR-006/领域 Owner。

### 39.2 冻结门禁判断

- 已新增唯一项目级权威文件 `doc/contract/rules/PLAT-CTR-003-身份授权与Session原语契约.md`，版本 1.0.0。
- Principal、Authenticator、SessionStore、CredentialStore、RateLimiter、AuditWriter 等均为进程内 Go API；Cookie/Header 是 HTTP 凭证载体，不存在独立公开 JSON 身份 DTO。无需为 Principal/SessionRecord/OwnerCredential 创建公开 JSON Schema。
- Redis Session JSON 是短期内部存储，可在升级时整体失效；Owner Credential 的表结构由 Platform Migration 管理；SecurityAuditEvent 是注入式内部事件，不是公开 HTTP DTO。
- 现有代码缺少 Principal.AuthMethod 与可信 WithPrincipal 写入 API；这是 Platform L2 实现符合性差异，不再是公开契约定义缺口，不阻断定义冻结。
- Server A2、WebUI A2、部署 A3 尚未完成不作为本轮契约定义冻结门禁。

### 39.3 结论

- PLAT-CTR-003：由 `pending_evidence` 调整为 `freeze-ready`。
- 本次不标记 `frozen`；正式冻结仍需项目负责人确认 1.0.0。
- 43 项正式 CTR 统计更新为：11 freeze-ready、30 pending_evidence、0 pending-decision、0 blocked、2 not_applicable、0 frozen。
- 不修改 Platform/Server 责任模块代码或模块文档；不修改其他分支，不执行 CI。


## 40. PLAT-CTR-005 HTTP Client 与 SSRF 契约定义收敛（2026-11-11）

### 40.1 权威定义与审计基线

- 新增唯一项目级规则文件：`doc/contract/rules/PLAT-CTR-005-HTTP-Client与SSRF契约.md`，版本 1.0.0。
- 核对 Platform 设计来源：`doc/platform/05-HTTP存储与异步任务设计.md`、`doc/platform/08-公共平台契约规范.md` 第8节、第15节、第23节、第25节，以及 `doc/platform/15-受控HTTP流式传输设计与验收方案.md`。
- 对照代码基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`；核对 `internal/platform/httpclient/client.go`、`bound_doer.go`、`stream.go`、`address.go`。
- 该契约是纯 Go InProcess API，使用唯一权威的强类型 Go 定义，不另建 JSON Schema；网络响应 DTO 不属于 Platform HTTP Client 契约。

### 40.2 定义收敛

已明确并版本化：
1. `Config`、`DestinationPolicy`、`Client`、`BoundDoer`、`StreamResponse`、`StreamStats` 的公开 API 和调用职责。
2. 固定 Endpoint 的精确 Host/Port allowlist；不支持 wildcard；默认 HTTPS；HTTP 必须显式允许；禁止 userinfo、非允许端口、自定义 Host 和自定义 Accept-Encoding。
3. DNS A/AAAA 全量结果校验，任何危险地址均拒绝；实际 Dial 连接已校验 IP，避免 DNS rebinding；禁用环境代理。
4. 默认拒绝重定向；启用后逐跳复核且仅允许同源，最多 3 跳，禁止 HTTPS 降级。
5. 缓冲请求的响应体上限、超时、并发、幂等/可重放重试条件、重试状态码和退避；HTTP 状态码本身仍作为正常响应交付。
6. 流式请求独立入口、Body 所有权、Close/EOF/取消清理、独立并发池、解压后字节上限、单次 Read 空闲超时、流总期限及终态统计。
7. Fault 的安全摘要、Dependency 和 Retryable 语义；Platform 不解析 SSE/JSON/模型协议，不做应用层流式重试。

### 40.3 冻结门禁判断

- 生产方：Platform；直接消费者：Data、News、LLM，以及未来显式采用该能力的服务端模块。
- 唯一权威定义、API 版本、策略字段、超时/资源边界、安全目标约束、重试语义和流式生命周期均已明确。
- 对于纯 InProcess Go API，强类型 Go API 与本规则文件构成等价机器契约；JSON Schema 不适用。
- Platform A1 的测试/CI 已在 Platform 专项文档记录，但不以其作为本轮冻结前置条件；消费者 A2、生产网络/Secret/egress A3 均属于后续实现符合性。
- 未发现需要新增项目级语义决议的未决取舍；不修改 Platform 代码、模块消费者代码或其他分支。

**结论：PLAT-CTR-005 由 `pending_evidence` 调整为 `freeze-ready`，契约版本 1.0.0。**



## 41. PLAT-CTR-006 Storage 与 Migration Runner 契约定义收敛（2026-11-11）

### 41.1 权威定义与审计基线

- 新增唯一项目级规则文件：`doc/contract/rules/PLAT-CTR-006-Storage与Migration契约.md`，版本 1.0.0。
- 核对来源：`doc/platform/05-HTTP存储与异步任务设计.md` 第2～4节、`doc/platform/08-公共平台契约规范.md` 第9节、Platform 剩余任务责任文档。
- 对照代码基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`；核对 `internal/platform/storage/storage.go` 和 `internal/server/migration/registry.go`。
- 本契约只定义 Platform `internal/platform/storage` 的纯 InProcess Go API；JSON Schema 不适用。Server Registry 和领域 Repository 不属于本契约。

### 41.2 定义收敛

已明确并版本化：
1. SQLConfig、OpenSQL/PingSQL 与 OpenRedis/PingRedis 的输入、返回、默认值、上限、初始 Ping 和资源所有权。
2. Migration 输入结构、namespace 字符集/长度、Version 唯一性、Description/Statements 限制、SQL checksum 算法及不可变规则。
3. ApplyMigrations 显式执行、单 namespace advisory lock、History 状态、顺序前缀、checksum 漂移、DDL 非事务性和失败恢复语义。
4. 锁获取/释放结果不确定时丢弃物理连接；History 状态更新必须恰好影响一行。
5. Ready/Shutdown 分层：Platform 只负责连接初始 Ping 与单次 Migration 执行；Server 负责跨 namespace 注册、全局 Ready、Worker 停止和资源关闭。
6. 明确不提供通用 Repository、ORM、自动发现 Migration 或分布式 Migration 编排。

### 41.3 冻结门禁判断

- Platform 连接/Runner、领域 Repository/Schema/Migration 内容、Server 注册/执行三方职责边界明确。
- API、版本、默认值/上限、History 状态机、错误语义、资源生命周期及失败恢复均有唯一强类型/文字权威定义。
- 真实 MariaDB/Redis 测试、Server 注册完整性、部署 Migration 执行和全局 Ready/Shutdown 验收均属于实现符合性，不阻断本轮契约定义冻结。
- 未发现需要新增项目级语义决议的未决取舍；不修改 Platform/Server/领域业务代码或其他分支。

**结论：PLAT-CTR-006 由 `pending_evidence` 调整为 `freeze-ready`，契约版本 1.0.0。**



## 42. PLAT-CTR-007 Typed Config 与 Secret 生命周期契约定义收敛（2026-11-11）

### 42.1 权威定义与审计基线

- 新增唯一项目级规则文件：`doc/contract/rules/PLAT-CTR-007-Typed-Config与Secret契约.md`，版本 1.0.0。
- 核对设计来源：`doc/platform/08-公共平台契约规范.md` 第6节。
- 对照代码基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`；核对 `internal/platform/config/env.go` 与 `dotenv.go`。
- 本契约只定义 Platform 配置读取/Secret 原语；各模块 Config Schema、业务默认值和 Server 部署装配不属于本契约。

### 42.2 定义收敛

已明确并版本化：
1. Env 的构造、Key 字符集、String/RequiredString/Bool/Int/Duration 的缺失、fallback、解析和错误脱敏语义。
2. Secret 的可选/必填读取、Configured/IsZero、唯一明文 Value 入口，以及 fmt/slog/JSON/Text 全路径脱敏。
3. DotEnv 文件不存在、解析语法、进程环境优先、重复 Key 首次生效、先完整解析再写环境的行为。
4. 基础值校验辅助与 errors.Join 语义；业务范围和跨字段校验由各模块负责。
5. Bootstrap 配置快照、业务代码禁止直接读环境变量、Secret 轮换通过服务实例重建生效；明确不承诺 Go string 内存零化、热更新或 Secret Manager。

### 42.3 冻结门禁判断

- Platform、模块 Config Owner、Server Bootstrap 的职责边界明确。
- 公开 API、缺失/fallback 语义、类型解析、Secret 明文访问、脱敏序列化、配置快照和轮换边界均有唯一强类型/文字权威定义。
- 模块环境变量清单、业务 Config Schema、部署 Secret 注入与轮换演练属于模块/Server 实现符合性，不阻断本轮 Platform 公共契约定义冻结。
- 未发现需要新增项目级语义决议的未决取舍；不修改 Platform/Server/领域业务代码或其他分支。

**结论：PLAT-CTR-007 由 `pending_evidence` 调整为 `freeze-ready`，契约版本 1.0.0。**



## 43. DATA-CTR-001 Provider 内部金额字段公开边界复核（2026-11-11）

### 43.1 核对证据

固定代码基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`。

- `internal/marketdata/model/dragon_tiger.go` 的 `DragonTigerItem` 暴露 `provider_buy_value`、`provider_sell_value`、`provider_deal_value`、`provider_net_value`、`provider_net_rate`、`provider_org_net_value`、`provider_hot_money_net_value`、`provider_amount_unit`。
- `internal/marketdata/model/model.go` 的 `Quote` 与 `DailyBar` 均暴露 `raw_amount`、`raw_amount_unit`、`raw_amount_field`；同文件 `LimitPoolItem` 暴露 `provider_amount_unit`。
- `internal/marketdata/httpapi/openapi.yaml` 1.1.0 对上述字段有对应公开 Schema 声明；这不是仅存在于 Provider Adapter 或 Repository 内部的字段。
- `doc/contract/decisions/项目级契约决策记录.md` 中 DEC-PLAT-002-027 已明确：公开 DTO 表达规范化领域数据，不公开 Provider 原始金额/单位或 Provider 专属适配细节；单位未核实的值不得冒充 CNY。
- `doc/api/contracts/数据层API清单.md` 仍描述 `/market-api`、浏览器保存 X-API-Key、旧 `services/market-data/openapi.yaml` 权威路径，与当前根 Module 内 `internal/marketdata/httpapi/openapi.yaml` 和 Root Server 目标边界不一致；该文件只能作为历史接入说明，不能覆盖当前契约。

### 43.2 核定结论

1. DATA-CTR-001 的公开定义与 DEC-PLAT-002-027 存在可直接定位的冲突。冲突范围至少包括 DragonTiger、Quote、DailyBar 和 LimitPool 的 Provider 原始金额/单位字段。
2. 该问题属于公开 DTO/OpenAPI 契约定义差异，不是 Data 内部 Provider 实现细节；不能通过 WebUI 隐藏字段或前端单位判断来规避。
3. DEC-027 已完成语义裁决，本轮不新增决策，不在 `feat/design_doc` 修改 Data 模型、OpenAPI 或 Data 专项文档。
4. DATA-CTR-001 继续保持 `pending_evidence`；冻结前必须由 Data 权威公开 DTO/OpenAPI 移除 Provider 内部金额字段，并明确规范化 CNY 字段的缺失/质量语义，同时完成字段级 required/nullable/omitempty 映射。

**结论：补充了 DATA-CTR-001 的直接冲突证据；未改变 43 项正式 CTR 的状态统计。**



## 44. DATA-CTR-006 Data HTTP API 认证语义收敛（2026-11-11）

### 44.1 核对证据

固定代码基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`。

- `doc/整体任务规划/03-模块接口契约/Data服务间认证映射与Secret注入设计.md` 已明确 Root Server + Data 同进程为默认模式；默认模式通过可信 Principal/Scope Context 调用 Data Application Port，不使用 Data API Key。Data HTTP API Key 仅属于远程兼容模式。
- 同一设计明确 Data API Key、Owner Token、Platform Service Secret、Provider Token 必须隔离；Remote Adapter 由 Server 侧注入 Data 专用 X-API-Key。
- `internal/marketdata/httpapi/openapi.yaml` 当前全局 security 同时声明 ApiKey 与 BearerAuth，两个 requirement object 形成 OR；健康接口显式 `security: []`，业务接口继承全局定义。
- `internal/marketdata/httpapi/server.go` 的 `Server.auth` 在 X-API-Key 缺失时读取 Authorization Bearer，并将两者与同一个 Data API Key 比较。
- `internal/marketdata/httpapi/platform_contract_test.go` 将 Bearer 认证作为成功用例。
- PLAT-CTR-003 1.0.0 与 DEC-PLAT-002-017/038 分别约束凭证隔离和可信调用主体来源。

### 44.2 项目级语义决议

依据 DEC-PLAT-002-041：
1. 默认同进程模式只由 Root Server 完成外部认证/路由授权并注入可信 Principal Context；Data Application Port 不重复校验 Data API Key。
2. 远程兼容模式的 Data HTTP 业务 API 只接受 X-API-Key；Authorization Bearer 不是别名，也不与 X-API-Key 形成 OR 认证。
3. 健康接口保持显式匿名，返回最小信息；其余业务 HTTP Operation 继承唯一 ApiKey Security Requirement。
4. Data API Key 只认证 Remote Adapter/服务调用方，不等同逻辑 caller，不可直接成为 AuthorizationRef.subject_ref 或 Snapshot Manifest.caller_ref。
5. 跨 HTTP 的可信逻辑 caller 传递/绑定继续由 DATA-CTR-008 定义；不能用普通客户端自报 Header/Body/Query 替代可信 Principal Context。

### 44.3 核定结论与状态

- 新增 DATA-CTR-006 1.0.0 项目级唯一语义规则文件。
- 已明确部署模式、唯一认证 Header、OpenAPI Security 表达、健康接口、凭据隔离、轮换和错误分类。
- 当前 OpenAPI、Handler 和契约测试仍共同接受 Bearer 认证，与本次收敛语义冲突；这不是单纯的实现接线差异，而是权威机器契约本身尚未同步。
- 本轮只更新项目级治理文档，不修改 Data 代码、OpenAPI、测试、模块专项文档或其他分支。

**结论：DATA-CTR-006 语义决议已完成，但继续保持 `pending_evidence`；待 Data 权威 OpenAPI/Handler 契约定义同步后重新核验。43 项统计仍为 14/27/0/2/0。**



## 45. DATA-CTR-001 Provider 原始值与金额公开边界扩展核查（2026-11-11）

### 45.1 核对范围与证据

固定代码基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`。对照 Data 所有 `internal/marketdata/model/*.go` JSON Tag 与 `internal/marketdata/httpapi/openapi.yaml`。

**Quote / DailyBar：**
- `model.Quote` 与 `model.DailyBar` 均公开 `raw_volume`、`raw_volume_unit`、`raw_amount`、`raw_amount_unit`、`raw_amount_field`。
- OpenAPI 的 Quote Schema 对应属性位于第 68～72 行，DailyBar Schema 对应属性位于第 118～122 行。
- 这些字段描述 Provider 原始数值、原始单位或 Provider 字段名；与同一 DTO 中标准化的 `volume`/`volume_unit=shares`、`amount`/`amount_unit=CNY` 并存，属于 Provider 内部归一化证据，不是跨模块消费者需要依赖的领域事实。

**DragonTiger：**
- Go JSON Tag 与 OpenAPI 同步公开 `provider_buy_value`、`provider_sell_value`、`provider_deal_value`、`provider_net_value`、`provider_net_rate`、`provider_org_net_value`、`provider_hot_money_net_value`、`provider_amount_unit`。
- OpenAPI 第 942～949 行直接将其描述为 Provider raw value，且承认金额单位可能未记录或由 Provider 定义。
- 同 DTO 已有明确的 `*_amount_cny`、`close_price_cny` 标准化字段，说明原始值不是消费者必需的公开字段。

**LimitPool：**
- `model.LimitPoolItem` 与 OpenAPI 同步公开 `provider_amount_unit`。
- `turnover`、`seal_money`、`max_seal_money` 的 OpenAPI 描述仍是 Provider-reported；其金额单位由 `provider_amount_unit` 决定，且 Schema 明确说明 `provider_defined` 不代表 CNY。
- 这使同一个公开 DTO 暴露无法统一解释的金额数值和 Provider 单位，而不是标准化后的领域金额。

### 45.2 DEC-PLAT-002-027 的适用核定

DEC-027 不只是禁止 DragonTiger 的七个 Provider 金额字段；它确立的公开边界是：跨模块 DTO 只暴露经过验证、已标准化的领域值，不暴露 Provider 原始字段、单位不确定的值或 Provider 专属适配细节。

因此：
1. Quote/DailyBar 的 raw volume/amount、raw unit、raw source field 均属于 Data 内部归一化证据，不应出现在跨模块公开 DTO/OpenAPI。
2. DragonTiger 的 Provider raw values、Provider raw rate 和 Provider unit 均应留在 Data 内部；公开层只保留规范化 CNY 金额、CNY/股价格、百分点比例及公共质量/来源元数据。
3. LimitPool 的公开金额字段只能表达已验证的 CNY 金额；不得把 `provider_defined` 或其他未验证单位作为公开金额单位。不能可靠换算为 CNY 时，金额按缺失语义省略/为空，并通过 quality_status/quality_reasons 表达 degraded 或 unknown。
4. DEC-027 不构成对所有 `provider_*` 命名字段的机械禁令。诸如 provider_report_date、provider_timestamp、provider_returned 等来源时间、报告日期或覆盖计数，需按各自领域语义判断；不得将它们与原始金额/单位字段混为一类。
5. `source`、`source_version`、`fetched_at`、`quality_status` 等已定义的公共来源与质量元数据继续保留，不因本决议删除。

### 45.3 核定结论

- 当前 Go JSON Tag 和 OpenAPI 对上述 Provider 原始字段保持同步公开；这不是单纯的文档遗漏，而是公开 DTO 本身违背 DEC-027。
- LimitPool 的金额字段还缺少统一的规范化单位语义；不能要求 WebUI 根据 `provider_amount_unit` 自行判断、换算或猜测。
- 本轮只补充项目级证据，不修改 Data 模型、OpenAPI、Provider Adapter、WebUI 或 Data 专项文档。
- DATA-CTR-001 继续保持 `pending_evidence`。Data 权威 DTO/OpenAPI 必须移除 Provider 原始字段/单位，并确保所有公开金额只表达可验证的 CNY 值；未验证值按缺失及质量语义处理。其余字段级 required/nullable/omitempty 映射仍需单独完成。

**结论：DATA-CTR-001 的 Provider 原始字段冲突范围已扩展核实；未改变 43 项正式 CTR 的状态统计。**



## 46. DATA-CTR-001 Application Port 公开 DTO 边界复核（2026-11-11）

### 46.1 核对证据

- `internal/marketdata/app/port.go` 明确声明该包定义 Market Data 对其他模块公开的 Application Port，且不暴露 Provider、Repository、Cache 或 HTTP 传输类型。
- 该 Port 的 `QuoteSnapshot` 和 `DailyBar` 仍包含 `RawVolume`、`RawVolumeUnit`、`RawAmount`、`RawAmountUnit`、`RawAmountField`。
- `internal/marketdata/application/reader.go` 的 `mapQuote`、`mapDailyBar` 将上述字段从内部 model 原样复制到 Application DTO，没有在 Port 边界剥离。
- 该 Port 是模块对其他模块公开的强类型边界；它不因为不是 HTTP JSON DTO 就变成 Data 内部 Provider 实现细节。

### 46.2 核定结论

1. DEC-PLAT-002-027 的 Provider 原始字段隔离原则同样适用于 Data Application Port，不能只修 HTTP OpenAPI 而保留跨模块 Port 泄漏。
2. QuoteSnapshot/DailyBar 的跨模块公开 DTO 应只保留标准化领域值、来源/时间/质量等公共语义；Provider 原始值、原始单位和 Provider 字段名留在 Data 内部。
3. 这项边界不要求 Port DTO 与 HTTP DTO 结构相同；二者可按各自消费场景保持不同形状，但都必须遵守相同的 Provider 内部信息隔离原则。
4. 本轮只补充项目级证据，不修改 Data Application Port、Reader 或 Provider 实现。

**结论：DATA-CTR-001 的冲突范围扩展到跨模块 Application Port；继续保持 pending_evidence。**


## 47. DATA-CTR-005 / DATA-CTR-007 v2 DTO 与 Snapshot 内容映射复核（2026-11-11）

### 47.1 DATA-CTR-005 精度 DTO 证据

- `internal/marketdata/httpapi/openapi.yaml` 当前 API 版本为 1.1.0；Quote/DailyBar 的价格、金额、均线等数值字段仍使用 OpenAPI `number`。
- `internal/marketdata/app/port.go` 的 QuoteSnapshot/DailyBar 数值字段仍使用 `float64` / `*float64`；没有 DailyBarsV2 或 Decimal 强类型。
- 固定 integration 文件树未发现 DailyBarsV2、Data Decimal DTO、独立 v2 Schema 或 v2 Golden。
- DEC-PLAT-002-009 明确：保留既有 v1 number，不得静默将 number 改为 string；v2 只能作为新版本契约，且需先明确消费者输入边界、精度要求和转换位置。
- Quant/Research 字段级契约虽然要求价格/金额使用 decimal string，但没有提供 Data DailyBarsV2 的字段级精度、scale、舍入和缺失 Schema，不能替代 Data 自己的权威定义。

**结论：** DATA-CTR-005 的 v2 不是已存在能力；目前只有 v1 number/float64。必须先定义独立 v2 DTO/Schema/Golden，再明确 v1→v2 转换责任、精度/scale/舍入/缺失规则和消费者版本选择。不得修改或重新解释 v1。

### 47.2 DATA-CTR-007 Snapshot 与 Evidence 内容绑定证据

- `DATA-CTR-008-snapshot-manifest-1.0.0.schema.json` 的字段为 snapshot_ref、manifest_fingerprint、caller_ref、purpose_code、symbol、日期范围、adjustment_basis、system_as_of、sources；Manifest 本身是范围/来源版本元数据，不包含日线正文、payload_schema_version 或内容 DTO 引用。
- 对应 Golden 仅验证 Manifest 元数据的 JCS/SHA-256 指纹，不是 DailyBars 数据正文 Golden。
- `app.DailyBarsSnapshot` 返回 Symbol、AdjustmentBasis、QualityStatus、Warnings、SystemObservedAtCutoff 和 Bars；没有 snapshot_ref、manifest_ref 或 authorization_ref。
- `model.Bundle` 存在 `snapshot_id`，但当前项目级权威定义没有证明它与 DATA-CTR-008 的内容寻址 `snapshot_ref` 等价；不得仅凭字段名将两者合并。
- Quant/Research DTO 要求 input_snapshot_refs、Evidence 的 source_ref/source_version/source_locator 和授权引用；当前未形成从 Data 实际行情内容到这些引用字段的权威映射。

**结论：** DATA-CTR-007 不只是缺少 DailyBarsV2 Schema，还缺少“不可变行情内容—SnapshotRef/Manifest—AuthorizationRef—Research Evidence”的可验证绑定定义。DATA-CTR-008 Manifest 不能被误认为行情内容 DTO；必须分别定义内容版本/定位与 Manifest 元数据的关系、内容完整性/版本、质量单位、PIT 时间和 Evidence source_ref/version/locator 映射。

### 47.3 状态与边界

- DATA-CTR-005、DATA-CTR-007 均继续保持 pending_evidence；本轮没有新增 pending_decision。
- 不修改 Data DTO、OpenAPI、Application Port、Snapshot 实现或 Quant/Research 责任模块文件。
- Provider 内部精度算法、数据库内部快照实现、模块单测和 WebUI 展示不是本轮项目级公开契约定义门禁；只有其产生/消费的公开 DTO、引用和字段语义纳入本轮。

**结论：DATA-CTR-005/007 的缺口已从笼统的“v2 未定义”细化为可执行的契约定义任务；43 项状态统计不变。**



## 48. DATA-CTR-008 Snapshot Manifest 与可信授权绑定复核（2026-11-11）

### 48.1 Manifest 机器定义核对

固定代码基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`。

- `doc/整体任务规划/schema/DATA-CTR-008-snapshot-manifest-1.0.0.schema.json` 已定义 Manifest 必需字段、purpose_code 枚举、SnapshotRef 与 fingerprint 格式、sources 非空数组。
- Schema 的 `system_as_of` 只声明 `format: date-time`，没有约束 UTC `Z`；JSON Schema format 本身不等于 UTC 约束。
- `sources` 声明 `uniqueItems: true`，只能阻止完全相同对象重复，不能阻止相同 `source_ref` 搭配不同 `source_version` 的 tuple 重复，也不能表达按 `(source_ref, source_version)` 字典序排序。
- 当前 Golden 只有一个 source 项，不能验证多 source 的排序、tuple 唯一和规范化稳定性。
- Manifest fingerprint 的文字算法已定义为：排除 `snapshot_ref` 与 `manifest_fingerprint` 后，对其余字段执行 RFC 8785 JCS + SHA-256；`snapshot_ref` 为 `data_snapshot:sha256:<fingerprint>`。但 Schema/Golden 尚未覆盖全部排序与 UTC 约束边界。

### 48.2 可信 caller 与 AuthorizationRef 绑定

- DEC-PLAT-002-038 已明确：Manifest `caller_ref` 必须与对应 AuthorizationRef.subject_ref 完全一致；二者均来自 Server 注入的可信 Principal/Scope Context，不能由请求体、普通 Header 或 Query 自报。
- Data API Key/Remote Service Credential 只认证 Remote Adapter/服务调用方，不得直接作为 `caller_ref` 或 `subject_ref`；DEC-PLAT-002-041 已进一步明确 X-API-Key 不是逻辑调用主体。
- GOV-CTR-006 已要求 Data 的 AuthorizationRef.resource_ref 与 Manifest.snapshot_ref 逐字一致，purpose_code 一致；Manifest 指纹和 AuthorizationRef 指纹必须分别验证。
- 当前 Data 服务间认证设计仍有“API Key 认证后映射 caller principal”的旧表述，与 DEC-038/041 的主体隔离原则不完全一致；需要以决议为准校准。
- 默认同进程模式可从 Root Server 注入的可信 Principal Context 取得逻辑主体；远程兼容模式下，X-API-Key 只能证明 Remote Adapter 身份，如何在不信任普通客户端声明的前提下保留原始逻辑 caller，尚无独立的受信任传递/绑定契约。
- `purpose_code` 的枚举已在 Manifest Schema 中定义，但请求入口如何将可信 Principal Scope 与 purpose_code 组合校验、谁负责签发 Data AuthorizationRef、每次 Snapshot 读取如何与当前策略版本保持同一可证明边界，尚未形成完整 Data 权威定义。

### 48.3 核定结论

1. DATA-CTR-008 已有 Manifest Schema/Golden 和指纹文字算法，不应将其描述为“完全没有机器契约”。
2. 现有机器定义仍缺 UTC 形式约束、sources tuple 唯一/排序的语义校验，以及跨 Manifest/AuthorizationRef 的字段一致性校验。
3. 可信 caller 的同进程来源已由 DEC-038 明确；远程模式不得把 API Key 直接转为 caller_ref，缺少可信 caller 传递协议时不得降级信任客户端自报值。
4. Manifest 是查询范围/来源版本的授权与审计元数据，不等同行情正文；行情内容定位与 Evidence 映射仍由 DATA-CTR-007 处理。
5. 本轮不新增项目级语义决议，不修改 Data Schema/Golden/代码或其他模块；后续若启用远程授权 Snapshot 能力，必须先补可信 caller 传递契约，否则该路径 fail closed。

**结论：DATA-CTR-008 继续保持 pending_evidence；其缺口已收敛为 UTC/排序 Validator、跨对象绑定、可信 caller/Scope/purpose 和读取时授权复核。**



## 49. DATA-CTR-002 时间与单位逐字段映射缺口核查（2026-11-11）

### 49.1 已有公共规则与当前模型事实

- `doc/api/contracts/数据质量与状态契约.md` 已区分 report_date、provider_time、available_at、fetched_at、normalized_at、system_observed_at；明确它们不能互相替代，并给出严格 PIT 的基本规则。
- 该文档也定义价格 CNY/股、成交量股、成交额 CNY、一般百分比为百分点；但这是通用规则，不等于每个 DTO 字段已明确绑定到相应时间/单位语义。
- `Quote` 使用 `quote_time` + `quote_time_semantics`，而 `DailyBar` 使用 `provider_timestamp`、`fetched_at`、`normalized_at` 和可选 `system_observed_at`；两者不能统一映射成一个“行情时间”。
- `FundamentalSection` 同时含 `report_date`、`publication_date`、`available_at`；`FundamentalsSnapshot` 另有快照级 `fetched_at` 和 `system_observed_at`。逐分区可用时间与父级系统观察时间的绑定语义仍需明确。
- `ValuationSnapshot` 含 `provider_report_date`、`provider_timestamp`、`fetched_at`；模型注释明确 provider_timestamp 只是五项估值中最新有效时间，不代表每个指标都在该时刻更新。ValuationItem 没有逐指标 available_at/system_observed_at。
- `StockAnnouncement` 含日期型 `announcement_date`、可选精确时间 `published_at` 和服务端 `fetched_at`；published_at 不能未经来源证据直接等同首次可用时间 available_at。
- `CorporateAction` / `AdjustmentFactor` 已分别含 available_at、fetched_at、system_observed_at，并保留 provider_time 或事件公告日期；但各字段与 PIT 版本选择的逐 DTO 绑定仍需权威映射。
- `IndexConstituent` 含 effective_from/effective_to、available_at、system_observed_at；其父级快照另含 provider_report_date、provider_time、fetched_at。报告日不能替代成员生效区间。
- `MarketSessionSnapshot.at` 是请求观察时刻，`server_time` 是服务端当前时钟，`fetched_at` 是服务端获取快照的时间；三者语义不同。

### 49.2 单位与比例字段边界

- Quote/DailyBar 明确声明 volume_unit=shares、amount_unit=CNY；公开 amount 字段可空。
- SectorRanking 与 MarketIndexQuote 的 amount_unit 为 CNY/unknown；需要明确 amount 为 unknown 时必须为空且质量降级/unknown，不能仅凭 amount_unit 字符串让消费者解释原始 Provider 数值。
- IndexDailyBar 的 amount_unit/volume_unit 当前是自由 string，OpenAPI 未给出闭合枚举，无法从 Schema 排除任意 Provider 单位。
- DragonTiger/LimitPool 仍暴露 provider_amount_unit；其中 LimitPool 金额字段的公开单位仍依赖 Provider 原始单位，已在 DATA-CTR-001/DEC-027 记录为冲突。
- 数据质量契约写明指数成交额单位不确定时保留 provider_defined；而 DEC-027 已要求跨模块公开 DTO 只表达标准化金额，未验证单位的原始金额不得冒充 CNY。两者需按 DEC-027 校准：内部可保留 provider_defined，公共 DTO 不得输出未经归一的原始金额。
- 百分比通常以百分点表达，但 ChipDistribution.profit_ratio/concentration_70/concentration_90 明确为 0..1 比例；不能把全局“百分比=百分点”机械应用到这些比例字段。需要字段级 unit/scale 定义。

### 49.3 核定结论

1. 已有时间与单位通用原则，不需要重写一套泛化规则；剩余工作是逐 DTO/字段建立权威映射。
2. PIT 可用时间必须来自可验证 available_at；report_date、announcement_date、provider_time、provider_timestamp、fetched_at、normalized_at、system_observed_at 均不得自动替代。
3. 需要明确 Valuation 等没有逐指标 available_at 的数据域，在严格 PIT 场景下应保持不可用/unknown，而不是拿批次 provider_timestamp 代替每项数据时间。
4. IndexDailyBar amount_unit、provider_defined 公开金额、LimitPool/DragonTiger 原始金额，以及百分点/比例差异应与 DEC-027 和 GOV-CTR-001 保持一致。
5. 本轮只记录公开字段定义缺口，不修改 Data 模型、OpenAPI、Provider、WebUI 或专项文档。

**结论：DATA-CTR-002 继续保持 pending_evidence；后续只补逐字段时间/单位映射，不重写既有公共规则。**



## 50. DATA-CTR-004 Quality、Freshness 与 Coverage 状态分层核查（2026-11-11）

### 50.1 已有机器定义

- OpenAPI `QualityStatus` 是闭合枚举：valid、degraded、stale、invalid、unknown。
- `ProviderProbeItem.status` 是单个 Provider × capability 的执行/返回状态：returned、returned_degraded、returned_unknown、failed、skipped、not_configured、not_supported。
- `ProviderProbeCheckResult.status` 是单项检查执行结果：passed、degraded、skipped、failed、not_configured、not_supported。
- `ProviderProbeCheckResult.freshness_status` 是独立 Freshness 状态：fresh、stale、unknown、not_applicable；Freshness SLA 文档明确它不修改 QualityStatus。
- ProviderProbeItem 同时携带 QualityStatus 和执行 status；二者不可互相映射或覆盖。缺少凭据/不支持/跳过不是业务数据质量 failed。
- `ProviderProbeCoverageDimension` 已定义 dimension、population、source、total、returned、validated、missing、excluded、ratio；OpenAPI 明确 ratio=coverage_validated/coverage_total，分母未知时省略。
- Freshness SLA 文档第14节明确 MarketBreadth 的 quote_response、price_classification、quote_timestamp、amount 四个字段覆盖维度使用 `total_securities` 作为分母，并定义 total/returned/validated/missing/excluded 的计数关系。

### 50.2 业务响应中的覆盖与空数据语义

- MarketBreadthSnapshot 有 total_securities、covered_securities、unclassified_count、amount_coverage_count、quote_timestamp_coverage_count、oldest/newest_quote_time；其 OpenAPI 明确 total_securities 来自源端分布桶、未独立与 Security Master 核验，不能无条件解释为真实全市场证券全集。
- LimitPoolSnapshot 有 has_data、total、total_is_lower_bound、items、quality_status；OpenAPI 已声明 has_data=false 表示未知/不可用而非确认零，并给出与 total/items 的组合约束。
- DragonTigerSnapshot 有 has_data、total、returned、source_returned、excluded_non_a_share_count、has_more；has_data 的含义是当前 Provider 页是否有支持的 A 股行，不等于指定日期完整龙虎榜是否为空。
- FundamentalsSnapshot 有 has_data 和分区级 QualityStatus；ValuationSnapshot 用 requested_symbols、total、items、missing_symbols 表达返回覆盖；它们的缺失/空结果不能直接沿用 LimitPool 的 has_data 语义。
- 多数业务 DTO 有 QualityStatus/quality_reasons，但 quality_status 是整体数据质量，不是请求状态，也不是覆盖率本身。total、returned、excluded 只有在明确总体分母时才构成覆盖率。

### 50.3 核定结论与剩余定义

1. 不能将 ProviderProbeItem.status、ProviderProbeCheckResult.status、freshness_status、QualityStatus 统一为一个 status 字段或同一状态机。
2. Freshness 目前只作为 Provider Probe 的诊断检查维度输出；不要求所有业务 DTO 新增 freshness_status。业务 DTO 是否需要 Freshness 字段必须由具体消费者用例和数据域契约决定。
3. skipped、not_configured、not_supported 是执行/能力边界，不是业务质量 invalid 或 failed；质量状态也不能反向证明 Provider 检查已执行。
4. MarketBreadth/Probe coverage 的分母及维度已有局部定义；不能把它们扩展成所有 DTO 的通用覆盖率。其他领域只有在总体分母可证明时才允许输出覆盖率。
5. 仍需建立逐响应 DTO 映射：每个响应的 QualityStatus 聚合规则、quality_reasons 条件、has_data/total/items 组合、coverage denominator、source/source_version 和 freshness 适用性；不要求在本轮重复定义已有的通用枚举。
6. 本轮不修改 Data DTO、OpenAPI、Freshness Evaluator、Provider Probe 或 WebUI。

**结论：DATA-CTR-004 继续保持 pending_evidence；缺口是逐响应状态/覆盖适用映射，不是缺少通用 QualityStatus 或 Freshness 枚举。**



## 51. NEWS-CTR-001 Evidence 时间与 RelationRefs 状态核定（2026-11-11）

### 51.1 business_time_from/to 空值语义

- `ListEvidenceRequest.BusinessTimeFrom/To` 为 `*time.Time`，JSON Tag 使用 `omitempty`。
- `NEWS-CTR-001-list-evidence-request-1.0.0.schema.json` 中两个字段均为可选属性，类型为 string/date-time，不允许 null。
- 因此机器 Schema 与 Go JSON 序列化一致：字段缺省表示没有该边界；提供时必须是有效非空时间戳。News API 契约正文中的“可选 nullable”是文字错误，不应为了匹配该措辞把 Schema 改为允许 null。
- DEC-PLAT-002-042 将其核定为“optional + omitted when absent”，不属于当前机器契约冲突。

### 51.2 system_observed_at 权威字段映射

- `GOV-CTR-002` 明确 News 单次版本观察时间映射为公共 `system_observed_at`，主体级 first/last observed 聚合时间不得替代具体版本观察时间。
- `internal/news/domain/version_record.go` 的 `NewsItemVersion.ObservedAt` 与 `news_item_versions.observed_at` 表示具体不可变版本的观察/可见时间。
- `internal/news/domain/observation.go` 的 `Observation.ObservedAt` 是候选观测时间；Repository PIT 查询同时要求版本和所属 Observation 均不晚于 system_as_of。
- `internal/news/adapters/storage/evidence_query.go` 按 `v.observed_at` 选择/排序版本；`internal/news/app/get_evidence.go` 将 `pit.Version.ObservedAt.UTC()` 映射为 ResearchEvidenceRecord.system_observed_at。
- 因此 Evidence DTO 的 system_observed_at 权威映射应为选中 NewsItemVersion 的 observed_at；Observation.observed_at 是另一项必须满足 PIT 的来源发现证据，不能替代版本观察时间。NewsItem.first_observed_at/last_observed_at 仅为主体级聚合值。
- 当前 `新闻数据模型与质量契约.md` 第3.2节的时间权威表未明确列出版本级 `observed_at`，容易与 Observation.observed_at 混淆；需由 News 责任范围对齐文档。项目级语义以 GOV-CTR-002 与本决议为准。

### 51.3 RelationRefs 的状态表达

- `ResearchEvidenceRecord` 当前只有必填 `relation_refs: []string`；Schema 只定义 string array，没有关系提供状态字段。
- `GetEvidence` 当前将 relation_refs 初始化为空数组；现有 DTO 无法区分“当前 Query 未提供关系能力”与“关系能力已执行且确认没有关系”。
- DEC-PLAT-002-021 已明确要求区分 not_provided 与已确认无关系，不得只用空数组表达。
- DEC-PLAT-002-042 固定最小表达：
  - 新增必填 `relation_status`，枚举 `not_provided` / `provided`。
  - `relation_refs` 保持必填数组。
  - `relation_status=not_provided` 时，`relation_refs` 必须为空数组；表示本次 Evidence Query 未提供关系数据，不得解释为确认无关系。
  - `relation_status=provided` 时，`relation_refs` 可为空或非空；空数组表示关系查询已完成且确认无适用关系，非空表示返回对应关系引用。
  - 首期当前 Facade 未接入关系查询，应输出 `not_provided`，不能输出 `provided + []`。
- 本字段是跨模块 DTO 语义，不能由 Researchflow 根据数组长度自行推断。

### 51.4 UTC Schema 约束

- `GOV-CTR-002` 规定跨模块绝对时间使用 RFC3339Nano UTC 并以 `Z` 结尾。
- NEWS-CTR-001 的 Request/Response/Cursor Schema 多处仅声明 `format: date-time`，没有将 UTC Z 约束表达为机器规则；format 只验证 date-time 结构，不能表达本项目的 UTC 规范。
- Go Application 已在请求校验和 PIT 映射中执行 UTC 检查/转换，但 Schema 仍比项目公共时间规则宽松。所有适用绝对时间字段（system_as_of、published_at filter、business_time filter、published_at/available_at、system_observed_at、fetched_at、normalized_at 及 Cursor 时间）应与 GOV-CTR-002 对齐，并由 Schema + 语义 Validator + Golden 共同验证。
- 本轮不修改 News Schema/Golden/Go DTO；该同步属于 News 责任范围。

### 51.5 核定结论

1. business_time_from/to 的空值语义已可确定，不是冻结阻断项；应修正模块文档措辞，不改 Schema/Go 类型。
2. system_observed_at 的公共含义已由 GOV-CTR-002 决定，具体映射为 NewsItemVersion.observed_at；News 数据模型文档需要补齐版本级时间权威表。
3. relation_status 缺失是实际公开 DTO/Schema 缺口；必须新增机器字段并遵守 DEC-021/042 的跨字段语义。
4. UTC RFC3339Nano Z 约束仍未完整进入 Schema/Golden。
5. NEWS-CTR-001 继续保持 pending_evidence；未来 HTTP OpenAPI 草案不属于当前活动契约，也不作为本 CTR 冻结门禁。

**结论：NEWS-CTR-001 的语义取舍已收敛；当前剩余门禁是 Schema/DTO/Golden 与 News 数据模型文档同步。**



## 52. NEWS-CTR-002 Authorization Binding 资源与策略版本引用核定（2026-11-11）

### 52.1 机器契约与 Validator 已有定义

- `NEWS-CTR-002-authorization-binding-1.0.0.schema.json` 要求 caller_ref、resource_ref、purpose_code、requested_content_level、provider_ref、source_ref、endpoint_ref、authorization_ref、policy_snapshot；provider_ref/endpoint_ref 可显式为 null。
- 对应 Golden 的 AuthorizationRef.subject_ref 等于 caller_ref，resource_ref 等于 binding.resource_ref，purpose_code 等于 binding.purpose_code，content_level 等于 requested_content_level；AuthorizationRef.policy_version 等于 policy_snapshot.snapshot_ref。
- `internal/news/domain/authorization.go` 的 `ValidateNewsAuthorizationBinding` 已校验上述字段绑定、policy_version/snapshot_ref 一致、decision/conditions 组合、expires_at 晚于 decided_at，以及 PolicySnapshot 非空。
- `validNewsPolicySnapshot` 按 (policy_type, policy_ref, policy_version) 严格排序并拒绝重复 tuple；必须有且只有一个 Source Policy，Provider Output/Endpoint Policy 是否存在与 binding 的显式引用一致。
- `ValidateNewsAuthorizationBindingForEvidence` 将 binding.resource_ref 绑定到具体 NewsItem/Version/Observation，并核验 Observation 中实际采用的 Source、Endpoint、Provider Output Policy 版本。
- News Authorization Matrix 已定义 Endpoint Policy 默认只能收紧 Source Policy、Provider/Source/Endpoint 策略取交集、conditional 条件满足后重新签发 allowed 引用、每次读取重新检查当前策略。
- `authorization_read.go` 通过项目 JCS Runtime Verifier 端口验证 decision_fingerprint，并从可信策略 Owner 读取完整当前策略集合；策略版本变化、撤销、pending、过期、当前用途/内容级别不允许或依赖不可用均拒绝读取。

### 52.2 发现的权威文字冲突

**policy_version：**
- GOV-CTR-006 的公共规则、NEWS-CTR-002 Golden 和 Go Validator 均规定 `AuthorizationRef.policy_version == PolicySnapshot.snapshot_ref`。
- PolicySnapshot.policies 中逐项保存实际适用的 Provider Output、Source、Endpoint 策略版本。
- `doc/news/新闻API契约.md` 第11.2节却把 policy_version 描述为“不可变 News Authorization Policy Registry 版本”，容易被解释为单个 Registry 版本号。
- 核定：policy_version 是组合 PolicySnapshot 的不透明 snapshot_ref；单项策略版本只出现在 PolicySnapshot.policies，不另设第二个 policy_version 语义。

**resource_ref：**
- NEWS-CTR-002 Golden 使用 `news_evidence_version:news_001:v1`。
- `NewsEvidenceResourceRef(newsID, versionNo)` 生成 `news_evidence_version:{news_id}:v{version_no}`；`ValidateNewsAuthorizationBindingForEvidence` 以 NewsID + VersionNo 校验 resource_ref，并同时校验独立 version_id 与 Observation 绑定。
- `doc/整体任务规划/03-模块接口契约/News-Research只读证据查询契约.md` 的历史描述却将 resource_ref 解释为 news_id + version_id 的稳定 opaque ref。
- 核定：resource_ref 的规范格式是 `news_evidence_version:{news_id}:v{version_no}`。version_no 是单个 NewsItem 内不可复用、单调递增的稳定版本序号；version_id 仍是 DTO/存储层的独立不透明对象 ID，不拼入 resource_ref。

**Binding 资源引用字段：**
- `provider_ref` 表示 Provider Output Policy 的策略引用，不是 Provider 原始响应或凭据；`source_ref` 表示发布来源策略引用；`endpoint_ref` 表示独立入口策略引用。
- Provider/Endpoint 策略不存在时必须显式 null；PolicySnapshot 不得虚构对应策略 tuple。Source Policy 始终必需。Endpoint 未配置独立策略时按 Source Policy 继承，不新增 endpoint tuple。

### 52.3 核定决议

依据 DEC-PLAT-002-043：
1. `AuthorizationRef.policy_version` 唯一等于 `PolicySnapshot.snapshot_ref`；实际策略版本集合由 PolicySnapshot.policies 表达。
2. `AuthorizationRef.resource_ref` 唯一使用 `news_evidence_version:{news_id}:v{version_no}`；version_id 只用于精确定位具体 DTO/存储版本，不替代规范资源引用。
3. Provider/Source/Endpoint 三类策略引用语义和显式 null/继承规则按本节及 NEWS Authorization Matrix 执行。
4. 已有 Schema/Golden/Go Validator 与上述决议一致；当前阻断点是 News API 契约及跨模块旧设计文字仍保留相反解释，需由 News 责任范围统一。
5. 本轮不修改 News 代码、Schema、Golden 或模块专项文档。

**结论：NEWS-CTR-002 的语义取舍已收敛，但因权威文字来源仍有冲突，继续保持 pending_evidence。**



## 53. NEWS-CTR-003 PIT 与 Cursor 版本边界核定（2026-11-11）

### 53.1 当前活动契约

当前唯一活动的 News Evidence Query 定义为：
- `doc/news/新闻API契约.md` 第11节；
- `doc/news/schema/NEWS-CTR-001-*.schema.json` 与对应 Golden；
- `NEWS-CTR-001` 在项目级冻结登记簿中的语义决议（DEC-PLAT-002-042 等）。

`doc/整体任务规划/03-模块接口契约/News-Research只读证据查询契约.md` 是早期“建议字段/目标契约”设计稿，内容明确标注不代表当前代码已有能力；它不得继续作为当前 v1.0 DTO/Schema 的并列权威定义。后续应标记为历史设计参考或删除重复的过时字段定义。

### 53.2 版本化请求边界

DEC-PLAT-002-044 固定 NEWS-CTR-001 v1.0 请求语义：
1. GetEvidence 与 ListEvidence 均必须提供 `system_as_of`，即使 GetEvidence 指定了 version_id 也不例外；显式版本仍必须证明该版本和所属 Observation 均不晚于 system_as_of。v1 不提供无 PIT 的“当前版本直读”模式。
2. `requested_content_level` 唯一枚举为 metadata、summary、snippet、full；旧设计稿中的 title、abstract、full_text 不属于 v1。
3. ListEvidence filters 是闭合集合：news_ids、source_refs、canonical_symbols、published_at_from/to、quality_statuses、business_time_from/to；不接受旧稿中的 subject_refs、identity_statuses、observed_from/to 或 include_relations。
4. purpose 字段的当前名称为 purpose_code，首期枚举 research_analysis、risk_review、user_display。
5. 严格 PIT 必须同时满足版本 system_observed_at <= system_as_of、所属 Observation.observed_at <= system_as_of、available_at 非空且 available_at <= system_as_of。published_at 不能替代 available_at；缺少 available_at 证据时排除，不自动降级宽松 PIT。
6. 分页排序版本固定为 `news-evidence-v1`：system_observed_at DESC、news_id ASC、version_id ASC。Cursor 绑定 caller、purpose、content level、规范化 filters 指纹、page_size、system_as_of、snapshot_ref、排序版本和最后排序键；TTL 15 分钟，HMAC-SHA256 防篡改。不得 OFFSET、不得过期重建快照。

### 53.3 PIT 时间与关系边界

- 具体版本的 system_observed_at 映射 NewsItemVersion.observed_at；Observation.observed_at 独立用于发现证据 PIT 过滤；主体级 first/last observed 不替代版本时间（DEC-PLAT-002-042）。
- RelationRefs 首期状态为 not_provided；没有关系查询能力时不得将空数组解释为已确认无关系。关系查询若未来接入，必须按相同 system_as_of 过滤关系断言和 supersedes 链。
- 证券/行业身份和关系历史事实必须先按 system_as_of 过滤系统知识时间，再按 valid_at/业务时间解析；不得先取当前映射再回填历史。

### 53.4 旧设计稿冲突与核定结论

旧 `News-Research只读证据查询契约.md` 中的以下内容已被当前 v1.0 规则取代：
- version_id 指定时允许省略 system_as_of；
- requested_content_level 使用 title/abstract/full_text；
- ListEvidence 使用 subject_refs、identity_statuses、observed_from/to 和 include_relations；
- DTO 使用 ObservedAt 而不是明确的 system_observed_at。

上述旧字段不得作为当前跨模块 API 使用。当前实现尚未完成 Snapshot Store 到 Query Facade 的所有接线，属于实现符合性/验收问题，不改变 v1.0 公开契约语义，也不阻断本轮定义冻结。

**结论：NEWS-CTR-003 的 PIT/Cursor 语义已收敛；由于 NEWS-CTR-001 的 relation_status/UTC Schema 机器定义仍未同步，NEWS-CTR-003 继续保持 pending_evidence。**



## 54. QNT-CTR-001 Metric Registry 身份与复合 Indicator 输出映射核定（2026-11-11）

### 54.1 当前实现与 Schema 证据

- `QNT-CTR-001-registry-entry-1.0.0.schema.json` 的 Registry Entry 描述单一 code 的 value_type、unit、precision、range、algorithm_version、input_requirements、parameters_schema、quality_policy、supports_interval。
- `internal/quant/registry.Entry.Code` 与 `internal/quant/metric.Metric.MetricCode` 在 `Registry.ValidateMetric` 中逐字比较；Entry 是对单个 Metric 标量语义的定义，不是 Indicator 函数本身的定义。
- `internal/quant/indicator` 中 SMA/EMA/Return/Volatility/RSI/ATR 返回单值；MACD 返回 DIF/DEA/Histogram 三个值；Bollinger 返回 Middle/Upper/Lower 三个值。
- `internal/quant/factor.Result.Code` 与 `Metric.MetricCode` 对应，首批 Factor 是 momentum_20d、momentum_60d、volatility_20d。
- Registry 代码已支持按 (code, schema_version) 查询、显式活动 algorithm_version、历史版本查询、版本切换和 Freeze；不依赖注册顺序。

### 54.2 DEC-PLAT-002-045 规范

1. Registry Entry 的 `code` 唯一语义为 `metric_code`；每个 metric_code + schema_version 是一个 Registry Entry。不得把 indicator_code、factor_code、result_type 和 metric_code 混为一个标识。
2. 单输出 Indicator 的首批 metric_code 为：sma、ema、return、volatility、rsi、atr。
3. 复合 Indicator 的每个标量输出分别注册：
   - MACD：`macd_dif`、`macd_dea`、`macd_histogram`。
   - Bollinger：`bollinger_middle`、`bollinger_upper`、`bollinger_lower`。
4. 首批 Factor 每个输出一个 Metric，metric_code 为 momentum_20d、momentum_60d、volatility_20d。
5. Indicator Request 的 indicator_code 选择计算算法；计算结果按上述固定映射投影为一个或多个 QuantMetric。每个输出 Metric 独立命中自己的 Registry Entry，独立声明 unit/value_type/precision/range/quality_policy；不得把 MACD/Bollinger 复合对象伪装成一个标量 Metric。
6. 同一次 Indicator 算法执行的所有输出必须记录同一算法版本；每个输出 Metric 的 calculation_version 必须与其命中的 Registry Entry.algorithm_version 一致。
7. Registry 的版本职责：
   - schema_version 表示 Registry Entry JSON 结构版本。
   - algorithm_version 表示该 metric_code 的计算和值语义版本，覆盖公式、seed/window、参数解释、输入要求、单位、数值类型、精度、舍入、范围和质量策略。
   - 仅当 Entry JSON 结构本身变化时升级 schema_version；上述任一指标语义变化必须升级 algorithm_version，不得覆盖历史版本。
8. 公共计算请求不指定 algorithm_version。服务端按 metric_code/schema_version 的显式活动映射选择唯一 active algorithm_version；结果必须记录实际版本。历史复算/结果验证按已持久化的 algorithm_version 精确查找，不因活动版本切换而改变历史语义。
9. 同一 metric_code/schema_version 同时最多一个活动 algorithm_version；活动切换必须显式指定 expected previous version，未知版本或并发冲突 fail closed；Registry Freeze 后禁止新增/切换。

### 54.3 仍未满足的冻结门禁

- 当前仓库有 Indicator/Factor 计算 Golden，但没有正式的 QNT-CTR-001 Registry Entry Golden 文件；算法输出 Golden 不等于 Registry 元数据 Golden。
- 尚未登记 6 个单输出 Indicator、6 个复合输出 Metric 和 3 个 Factor Metric 的完整 Registry Entry 值（单位、数值类型、整数/小数精度、舍入、容差、范围、输入要求、参数 Schema、质量策略）。
- 对价格类指标使用 decimal_string 还是 number、统计量容差、RSI/ratio 的单位表达等必须由具体指标定义和精度要求决定；不得为填满 Entry 任意套用统一 scale/tolerance。
- 本轮只收敛 Registry 身份/映射/版本选择语义，不生成虚构的 Registry 元数据或 Golden，不修改 Quant 代码、Schema、Golden 或模块专项文档。

**结论：QNT-CTR-001 的 Registry 结构语义已收敛，但缺首批真实权威 Entry/Golden，继续保持 pending_evidence。**



## 55. QNT-CTR-002 Indicator Request/Result 边界核定（2026-11-11）

### 55.1 契约职责与版本层级

- QNT-CTR-002 定义首批八种 Indicator 的调用参数、计算入口与 Metric 输出映射。
- QuantAnalysisRequest 的共同调用上下文以 `Quant-Research公开DTO字段级契约.md` 第2.1节为准；Indicator 参数选择由 `indicator_code + parameters` 表达。
- `QNT-CTR-002-indicator-parameters-1.0.0.schema.json` 中的 `schema_version=1.0.0` 表示 Indicator Parameters 契约版本，不是 QuantAnalysisRequest DTO 的 schema_version，也不是 QNT-CTR-001 Registry Entry 的 schema_version。三者各自独立演进。
- `indicator_code` 是算法入口标识；`metric_code` 是输出标量身份；二者不可混用。八种入口为 sma、ema、return、volatility、rsi、macd、atr、bollinger。

### 55.2 请求字段和可信边界

Indicator 调用使用共同 QuantAnalysisRequest 上下文，必须遵守：
1. 必须提供请求 DTO schema_version、request_id、correlation_id、scope、system_as_of、period、input_snapshot_refs、parameters、quality_policy、idempotency_key；security_id 按 scope 条件必需，trade_date 可选。
2. 首批八种 Indicator 均计算价格序列；必须提供可比较的 adjustment_basis。unknown/缺失复权口径拒绝计算；输入序列内不得混合 adjustment_basis 或 reference_date。
3. input_snapshot_refs 必须非空、引用不透明、不得重复；它是无序集合，请求指纹前按 UTF-8 字节序升序规范化（DEC-048）。无行情快照的纯函数调用不属于该公开 Indicator Request。
4. system_as_of 是系统知识截止时间，必须使用 RFC3339Nano UTC；不得由 trade_date、published_at、fetched_at 或本机当前时间替代。trade_date 是目标交易日，不能代替 system_as_of。
5. period 必须与输入 Snapshot 的实际 interval 一致，并满足所选 Registry Entry.input_requirements.interval；Registry 未声明支持的 interval 时拒绝，不得由 supports_interval=true 推断支持任意 interval。
6. parameters 必须按 indicator_code 命中 QNT-CTR-002 的对应 discriminated schema；拒绝未知 indicator_code、未知参数字段、缺失必需参数和越界值。跨字段约束（例如 MACD fast_window < slow_window）由语义 Validator 补足，不能只依赖 JSON Schema。
7. 调用方不得提交 algorithm_version。服务端按 DEC-PLAT-002-045 的活动版本映射选择唯一 algorithm_version；拒绝未知/未注册指标或无活动版本。结果记录实际版本。
8. 幂等作用域固定为 (authenticated_caller_ref, use_case, idempotency_key)。use_case 由稳定的 Quant Facade 路由映射，不由客户端自由指定；同键同指纹返回原结果引用，同键异指纹返回 idempotency_conflict。request_id/correlation_id 不进入请求指纹。

### 55.3 Indicator 输出映射

- Indicator 结果使用 QNT-CTR-004 定义的共同 QuantResult Envelope，`result_type=indicator`；QNT-CTR-002 不另造一套平行 Envelope。
- Indicator payload 使用强类型 `IndicatorResultPayload`，包含 `indicator_code` 和 `metrics: QuantMetric[]`；禁止将算法原始 Go Result、MACDResult、BandsResult 直接作为跨模块 DTO，也禁止任意 map。
- 单输出入口返回恰好一个 Metric：
  - sma → sma
  - ema → ema
  - return → return
  - volatility → volatility
  - rsi → rsi
  - atr → atr
- 复合输出入口返回固定数量、固定顺序的 Metric：
  - macd → macd_dif、macd_dea、macd_histogram
  - bollinger → bollinger_middle、bollinger_upper、bollinger_lower
- 每个 metric_code 独立命中 QNT-CTR-001 Registry Entry；unit、value_type、precision、range、rounding、quality_policy 只从对应 Entry 读取，不从 indicator_code、Go float64 或样本值推断。
- 同一 Indicator 调用的所有输出 Metric 使用同一 algorithm_version；每个 Metric.calculation_version 必须等于对应 Registry Entry.algorithm_version，且等于 Envelope.algorithm_version。复合输出不允许各自选择不同活动版本。
- Metric.period 和 Metric.system_as_of 必须分别等于请求 period 和 system_as_of。available 必须有非 null value；不可用状态必须显式 null，不得用 0 代替。
- 首批八种 Indicator 均为确定性描述性计算，不提供预测置信区间；lower_bound/upper_bound 必须同时为 null。若未来引入区间估计，须通过新的指标语义版本和明确 Registry 约束，不得把 Bollinger 上下轨当作统计置信区间。
- MACD 三个输出及 Bollinger 三个输出共享本次输入、参数、质量判定和计算状态；不能只返回其中部分输出而将其余输出静默省略。

### 55.4 质量、样本与终态映射

1. `quality_status`、`metric_status`、`result_status` 分别表达输入/结果质量、单个 Metric 数值可用性、整个计算结果状态；不得互相替代。
2. 只有所有预期输出 Metric 均完成计算且满足对应 Registry.quality_policy，Envelope.result_status 才可为 completed。degraded 输入是否允许 available，逐 metric 由 Registry.quality_policy 决定。
3. 由于请求有效但有效样本数不足以满足算法最小窗口，Metric 使用 insufficient_sample、value=null；整体结果使用 insufficient_sample。数据源/快照缺失或覆盖不足导致无法建立所需输入时，按质量策略表达 insufficient_data/unknown，不得伪装为样本窗口不足。
4. 参数不合法、输入违反硬约束、复权/时间口径不可比较时不得输出 available 数值；参数/请求校验失败使用稳定 Fault，业务门禁拒绝使用 rejected，非预期执行错误使用 failed。
5. sample_count 表示该 Metric 实际参与计算的有效观测样本数量，不等于原始输入数组长度。不同算法的样本单位可以不同（价格 Bar 或收益观测），其精确定义必须由对应 Registry Entry 声明；同一复合 Indicator 的输出共享样本口径。当前 QNT-CTR-001 Entry Schema 尚无独立 sample_count 口径字段，需在首批 Entry 定义/Schema 完善时补齐，不得仅按现有 Go SampleCount 赋值推断公开语义。
6. 计算过程不得静默填充缺失交易日、混合复权基准或把 invalid/unknown 质量提升为 valid。真实输入的交易日连续性、PIT、available_at、Bar 级质量和复权参考日由 Data Snapshot/Adapter 证据及 Quant 输入边界共同证明；纯计算函数通过不代表这些门禁通过。

### 55.5 当前缺口与冻结门禁

- 现有 QNT-CTR-002 Schema 只验证参数选择和参数对象，不是完整 QuantAnalysisRequest Schema。
- 当前没有完整 IndicatorResultPayload Schema/Golden，不能把八份算法数值 Golden 当作跨模块 DTO Golden。
- Registry 首批 8 Indicator 输出组/3 Factor 的 Entry/Golden 未形成；单位、数值类型、精度、舍入、范围、quality_policy 不能臆造。
- Registry 尚未将 sample_count 的 required/optional、样本单位和计数口径纳入权威定义。
- 现有 MACD 参数 Schema 无法单靠 Draft 2020-12 表达 fast_window < slow_window，必须由语义 Validator 和负例 Golden 覆盖。
- 以上均属于公开机器契约或 Registry 元数据缺口；模块内 CI、纯计算函数测试、真实 Data Adapter、Researchflow 消费接线不是本轮项目级语义定义门禁。

**结论：QNT-CTR-002 的请求与输出语义已收敛；因完整 Request/IndicatorResultPayload 机器定义及 Registry 元数据仍缺，继续保持 pending_evidence。**



## 56. QNT-CTR-003 Factor Request/Result 边界核定（2026-11-11）

### 56.1 首批范围与请求身份

- 首批 Factor 仅包括 `momentum_20d`、`momentum_60d`、`volatility_20d`，均为单证券时间序列因子；请求 `scope=stock` 且必须提供 `security_id`。
- 不包含市场/行业横截面因子、排序、中性化、winsorize、z-score、rank 或组合因子；这些属于后续独立契约范围。
- `factor_code` 唯一选择计算因子，并与输出 `metric_code` 一一对应；首批不允许一个 Factor 输出多个 Metric。
- `QNT-CTR-003-factor-parameters-1.0.0.schema.json` 中 `schema_version=1.0.0` 表示 Factor Parameters 契约版本，不是 QuantAnalysisRequest DTO 或 Registry Entry 的 schema_version。
- 首批参数固定为空对象 `{}`；拒绝任意附加参数。未来参数变化必须升级参数 Schema/版本，不得静默扩展空对象。

### 56.2 请求上下文与输入快照

1. Factor Request 复用 QuantAnalysisRequest 的共同上下文和幂等规则（DEC-046）：request_id、correlation_id、scope、security_id、system_as_of、trade_date、period、input_snapshot_refs、adjustment_basis、quality_policy、idempotency_key。
2. 首批 Factor 必须且只能使用一个完整的 DailyBars Snapshot；input_snapshot_refs 非空且不得重复。快照必须覆盖目标计算窗口及其所需前序交易日，不得从多个不一致快照拼接。
3. 输入 interval 固定为日线 `1d`；请求 `period` 表示因子计算窗口，不表示 Bar interval：
   - momentum_20d：period=`20d`
   - momentum_60d：period=`60d`
   - volatility_20d：period=`20d`
   period 必须与 factor_code 对应，调用方不得覆盖。
4. adjustment_basis 必须明确且可比较；所有输入 Bar 的 adjustment_basis/reference_date 必须一致。unknown、混合复权口径或缺少必要复权证据时不得计算有效值。
5. system_as_of 是系统知识截止时间；DailyBars Snapshot 与其所有组成 Bar/来源观察证据必须满足 GOV-CTR-002 PIT 边界。trade_date 若提供，必须等于快照最后一根目标 Bar 的交易日；若省略，结果 trade_date 取该快照最后一根 Bar 的交易日，不得取本机当前日期。
6. Data 必须提供不可变、可审计的交易日历版本引用，并使该引用与 DailyBars Snapshot 的身份/指纹绑定；同时明确 Snapshot 的实际 interval。Quant 不自行创建、推断或更新交易日历版本。
7. Quant 必须按上述交易日历版本验证 Bar 的交易日连续性、唯一性和升序。不能按自然日差判断交易日，也不能在缺失交易日时压缩数组后继续计算。
8. 当前 DATA-CTR-005 v2 尚不存在，DATA-CTR-008 Manifest Schema 也未包含交易日历版本和 interval 元数据。因此上述第6/7项是明确的 Data 契约依赖缺口；在 Data 提供相应权威证据前，不得宣称真实 Factor 输入已满足交易日连续性/PIT。

### 56.3 因子公式、样本数和单位

| factor_code / metric_code | 最小输入 | 计算定义 | Metric.period | sample_count |
|---|---:|---|---|---:|
| momentum_20d | 21 个连续有效日线 close | close_t / close_(t-20) - 1 | 20d | 参与窗口的 close Bar 数，最多/正常为 21 |
| momentum_60d | 61 个连续有效日线 close | close_t / close_(t-60) - 1 | 60d | 参与窗口的 close Bar 数，最多/正常为 61 |
| volatility_20d | 21 个连续有效日线 close | 最近 20 个 simple daily returns 的样本标准差，ddof=1，不年化 | 20d | 实际参与标准差计算的 return 数，最多/正常为 20 |

- 三个 Metric 的 unit 均为 `ratio`；现有 QNT-CTR-004 Factor QuantMetric Schema 已将 value JSON 类型定义为 `number`，首批契约沿用该公开类型。Registry Entry 必须与此保持一致；不得仅因 Go 使用 float64 而推导额外精度/容差。
- sample_count 不是统一的输入 Bar 数：动量计数价格 Bar，波动率计数收益观测。不可把 volatility_20d 的 21 根输入 Bar 错报为 21 个有效收益样本。
- 可用结果必须达到对应最小窗口；不足窗口时保留真实有效样本数，不得补零或填充。
- 首批 Factor 是确定性描述性统计，不提供置信区间；lower_bound/upper_bound 必须同时为 null。
- precision、rounding_mode、tolerance、range 和 quality_policy 的具体数值仍由各 metric_code 的 QNT-CTR-001 Registry Entry 提供；不得从 synthetic Golden、当前样本值或 float64 表示推导。

### 56.4 Factor Result 映射

- Factor 结果使用 QNT-CTR-004 共同 QuantResult Envelope，`result_type=factor`；QNT-CTR-003 不定义平行 Envelope。
- Payload 使用强类型 `FactorResultPayload`，包含 `factor_code` 和 `metrics`；metrics 必须恰好包含一个 QuantMetric，且其 metric_code 必须等于 factor_code。
- Metric.calculation_version 与 Envelope.algorithm_version 均取服务端 Registry 活动版本；调用方不得指定版本，历史结果复算按已持久化版本精确查找。
- Metric.period 必须匹配 56.3 的因子窗口；system_as_of 必须与请求一致；unit/value_type/precision/range/quality_policy 取对应 Registry Entry。
- quality_status、metric_status、result_status 分域表达；available 必须有非 null value，其他不可用状态必须显式 null，不得将缺失值转换为 0。

### 56.5 样本不足、缺失数据与质量状态

1. `insufficient_sample` 表示数据覆盖完整、交易日历连续，但证券在目标截止日前可用的有效历史本身不足以满足最小窗口（例如上市历史不足）。
2. `insufficient_data` 表示按交易日历应存在的 Bar 缺失、快照覆盖不完整或输入数据不完整；不能把缺失交易日删除后仍按足够样本计算。
3. `unknown` 表示交易日历版本、来源可获得时间、复权证据或关键质量事实无法确认；不得降级成 valid/degraded。
4. `invalid` 表示输入违反硬约束（非正/非有限价格、重复交易日、乱序、混合复权等）；不得输出 available Metric。
5. `degraded` 是否允许 available 由对应 Registry.quality_policy 决定；strict 不得放行 degraded，allow_degraded 也不绕过 Registry 限制。
6. Metric 的不可用原因和整体 Result 状态必须保留区分；纯计算函数内部的 `StatusInsufficientData` 不是公开状态的充分依据，Application 必须结合 Data 覆盖/日历证据区分 insufficient_sample 与 insufficient_data。
7. 参数 Schema 错误或请求不合法使用稳定 Fault；有效请求被业务质量门禁拒绝时 result_status=rejected；非预期执行错误为 failed。completed 不代表 quality_status=valid。

### 56.6 冻结门禁与结论

- 当前 Factor 参数 Schema 只定义 factor_code 和空 parameters，不是完整 QuantAnalysisRequest Schema。
- 当前 Factor QuantMetric Schema/Golden 仅覆盖局部 Metric 子对象，不定义 FactorResultPayload 或 QuantResult Envelope。
- DATA-CTR-005 未提供 DailyBars v2 Decimal DTO、实际 interval 和交易日历版本引用；当前纯计算函数只接收 `[]float64`，不能证明 Bar 时间、PIT、复权和交易日历连续性。
- 首批 Registry Entry 尚未提供每个 metric_code 的精度/舍入/容差/值域/质量策略；sample_count 口径虽在本节明确，仍需由 Registry 机器契约正式承载。
- 模块内算法 Golden/CI 通过不等于真实 Data 输入、完整 Result Payload 或 Researchflow 消费契约已完成。

**结论：QNT-CTR-003 的首批 Factor 范围、输入语义、样本口径与结果映射已收敛；由于 Data DailyBars v2 元数据、完整 Request/FactorResultPayload Schema/Golden 和 Registry Entry 仍缺，继续保持 pending_evidence。**



## 57. QNT-CTR-004 QuantResultRef、Envelope、Payload 与 Manifest 身份核定（2026-11-11）

### 57.1 三层版本身份

依据 DEC-PLAT-002-028，版本字段严格分层：
- QuantResultRef.schema_version：Ref DTO 自身版本。
- QuantResult Envelope.schema_version：Envelope DTO 自身版本。
- Result Manifest.schema_version：Manifest DTO 自身版本。
- Envelope.payload_schema_version 与 Manifest.payload_schema_version：同一个强类型 Payload Schema 版本。
- Payload Schema 唯一选择键为 `(result_type, payload_schema_version)`。
- Ref 不重复携带 payload_schema_version；消费者读取 Manifest/Envelope 后再选择 Payload Schema。
- 三类 DTO 版本与 Payload 版本独立演进；未知 Envelope/Manifest Schema 或未知 Payload 组合必须明确拒绝，不允许近似版本回退。

### 57.2 QuantResultRef 与 Envelope 的关系

1. QuantResultRef 是结果资源的轻量索引/引用视图，不包含 Payload；它不是 Envelope 的别名，也不是独立生成的第二份结果事实。
2. Ref 的 result_id、result_type、scope、security_id、system_as_of、trade_date、period、algorithm_version、parameter_fingerprint、input_snapshot_refs、quality_status、result_status 必须从已验证 Manifest/不可变结果记录投影，不允许由调用方重新填写或与原结果不一致。
3. Ref.schema_version 由 Ref DTO 自身定义；authorization_ref 是当前调用主体访问该 result_id 的授权证明，created_at 是持久化记录的原始创建时间。两者不参与结果内容寻址身份。
4. GetResult 必须返回完整 QuantResult Envelope（含强类型 Payload），不能只返回 Ref 或裸 Payload；Ref 用于列表、关联和定位，读取内容通过 Quant Facade 执行当前授权复核后返回 Envelope。
5. Envelope 必须同时包含 schema_version 与 payload_schema_version。Envelope 的 result_type、payload_schema_version、system_as_of、input_snapshot_refs、algorithm_version、parameter_fingerprint 以及其他 Manifest 身份字段必须与 Manifest 完全一致。

### 57.3 Manifest 的不可变结果身份

现有 Manifest 已绑定 result_type、Manifest schema_version、payload_schema_version、algorithm_version、parameter_fingerprint、system_as_of、input_snapshot_refs 和 result_fingerprint，但无法单独证明 Envelope 的 scope/security_id/trade_date/period/quality_status/result_status 未被替换。

为避免同一 result_id 被投影成不同证券、周期或质量结论，DEC-PLAT-002-048 将下列 Envelope 语义字段加入 Manifest 不可变身份：
- 必需：scope、period、quality_status、result_status。
- 条件/可选：security_id（scope=stock 时必需）、trade_date（无目标交易日时缺省）。
- 已有字段继续必需：result_type、Manifest schema_version、payload_schema_version、algorithm_version、parameter_fingerprint、system_as_of、input_snapshot_refs、result_fingerprint。
- result_fingerprint 只对规范化强类型 Payload 计算；manifest_fingerprint 对 Manifest 除 result_id/manifest_fingerprint 外的全部字段按 RFC 8785 JCS 计算；result_id 为 `quant_result:sha256:{manifest_fingerprint}`。
- authorization_ref 不进入 Manifest/结果指纹：它是当前访问主体的授权证明，可能随调用主体和当前策略变化；必须在每次读取时重新签发/复核。
- created_at 不进入内容寻址指纹：它是持久化记录时间，不是计算结果语义；由不可变记录元数据保存并由 Ref 投影。
- Envelope.schema_version 不进入 Manifest 身份：Envelope 结构版本与 Payload/结果资源语义独立；升级 Envelope 表示层不得改变相同结果资源的内容身份。

### 57.4 输入快照引用集合语义

- `input_snapshot_refs` 是无序集合，不是有业务顺序的数组；数组顺序不得承载输入角色或优先级。
- 请求指纹与 Manifest 指纹计算前，必须对引用按 UTF-8 字节序升序规范化；重复引用拒绝。
- 相同引用集合的不同输入排列必须得到相同规范化指纹和相同 Manifest 身份。
- 数组字段始终必需；空数组表示无外部事实输入，只在 DEC-PLAT-002-029 的 Quant Registry 白名单和 Researchflow ResolvedTemplate 双重授权同时成立时允许。不得用 null 或省略表示空输入。
- 本节修正 DEC-PLAT-002-046 中“input_snapshot_refs 顺序属于请求语义”的表述；该顺序不是业务语义，必须规范化排序。

### 57.5 Payload 注册与状态边界

- `result_type` 枚举只定义稳定类型名称，不代表所有类型均已开放。只有存在完整注册的 `(result_type, payload_schema_version)` Payload Schema/Golden 和对应 Validator 时才允许产生/读取该类型；否则返回 unsupported_result_type/unsupported_schema。
- 首批已定义的 Payload 形状：
  - indicator：DEC-046 的 IndicatorResultPayload（indicator_code + QuantMetric[]）。
  - factor：DEC-047 的 FactorResultPayload（factor_code + 恰好一个 QuantMetric）。
  - trend：TrendResult 1.0.0 强类型对象。
  - risk：RiskDecision 1.0.0 强类型对象。
- market、industry、stock、strategy、backtest、evaluation 目前是预留 result_type；没有各自完整 Payload Schema/Golden 前必须拒绝，不能借用其他类型 Payload。
- RiskDecision.status=reject 表示风险评估得出拒绝结论，不等同 Envelope.result_status=rejected。若规则完整执行并生成有效 RiskDecision，Envelope.result_status 可为 completed；Envelope.result_status=rejected 表示 Quant 请求/质量/授权门禁拒绝产生正常结果。
- Envelope.quality_status 与 Envelope.result_status 独立；Trend/Metric/Risk 子对象的领域状态不能直接替代 Envelope 状态。各 Payload 必须定义与 Envelope 的一致性校验。

### 57.6 当前机器定义缺口与结论

- QuantResultRef Schema 的 input_snapshot_refs 仍有 minItems=1，违反 DEC-PLAT-002-029 的空数组表达。
- QNT-CTR-004 尚无完整 Envelope Schema/Golden，也没有 IndicatorResultPayload/FactorResultPayload 完整机器定义。
- QNT-CTR-005 Manifest Schema/Golden 尚未加入 DEC-048 的 scope/security_id/trade_date/period/quality_status/result_status；Go Manifest 还缺 Schema 已声明的 payload_schema_version/algorithm_version/parameter_fingerprint，并拒绝空 input_snapshot_refs。
- 当前 Ref.BuildRef 从独立 RefInput 接收多项结果元数据，未证明这些字段与不可变 Manifest/Envelope 完全一致；需改为从已验证结果记录投影。
- Trend/Risk 现有 Schema 只验证局部结构；与 Envelope 的字段一致性、RiskDecision 规则状态组合仍需明确 Validator 规则。
- 当前不修改 Quant 代码、模块 Schema/Golden；机器定义同步由对应 QNT-CTR-004/005 责任范围后续执行。



### 57.7 Trend 与 Risk Payload 的跨层语义

**TrendResult：**
- `direction` 与 `phase` 是独立维度；unknown 不强制另一字段为 unknown，也不得将 unknown 转成 sideways。现有 Golden 的 direction=unknown、phase=emerging 是合法组合。
- `strength` 可为 null；非 null 必须是有限数且位于 [0,1]，不根据 direction/phase 推断或填充。
- `evidence_metrics` 与 `conflict_refs` 必须显式提供数组，允许空数组。
- TrendResult.period、system_as_of、algorithm_version、quality_status 必须分别与 Envelope 对应字段完全一致；evidence_metrics 中每个 Metric 的 system_as_of 必须等于 TrendResult.system_as_of。Metric 的单位/值类型/精度仍由各自 Registry Entry 决定。

**RiskDecision：**
- RuleSet 必须非空，并至少包含一条 block 级规则；所有 block 规则必须 required。缺少任何必需 block 规则执行器时，必须为该规则生成 status=unknown 的 block Reason，不得忽略。
- 同一 RuleSet 中 Reason.code 唯一；Reason.severity 必须与 RuleDefinition 一致；Reason 按 RuleSet 声明顺序输出，不按字典序重排。未知 RuleSet 外的 Reason、重复 code 或严重级别不匹配均拒绝。
- RiskDecision.status 必须由规则结果聚合，调用方不得自行指定：
  - 任一 block Reason 为 triggered：reject（优先于其他 block unknown）。
  - 没有 block triggered，且至少一个 block Reason 为 unknown：unknown。
  - 至少存在一条 block 规则且所有 block Reason 均为 not_triggered：pass。
  - warn/info 不改变 pass/reject/unknown；其自身 status 仍必须是合法枚举。
- RiskDecision.reasons 必须显式提供且至少一项；input_refs 必须显式提供，允许空数组，不用空数组伪造来源证据。
- RiskDecision.system_as_of 必须等于 Envelope.system_as_of；evaluated_at 是实际执行时间，必须为 UTC 且不早于 system_as_of。rule_set_version 是独立的风险规则版本，不得与 Envelope.algorithm_version 混用。
- RiskDecision.status=reject 是业务风控结论；Envelope.result_status=rejected 是请求在生成正常结果前被门禁拒绝，两者不得互相映射。RiskDecision.status 不直接决定 Envelope.result_status；后者表达结果生成/执行生命周期。

**Envelope/Payload 一致性：**
- Trend Payload 的 period/system_as_of/algorithm_version/quality_status 必须与 Envelope 一致。
- Risk Payload 的 system_as_of 必须与 Envelope 一致；rule_set_version 保持独立。
- Indicator/Factor Payload 的 period/system_as_of/algorithm_version/quality_status 按 DEC-046/047 与 Envelope 一致。
- Payload Validator 必须先校验 Payload 自身 Schema，再执行上述跨字段语义校验；仅通过局部 Schema 不代表完整 Envelope 合法。

**结论：QNT-CTR-004 的版本身份、Ref/Envelope/Manifest 关系、内容寻址和首批 Payload 形状已收敛；由于完整 Envelope/Payload 机器定义和 QNT-CTR-005 Manifest/Go 同步未完成，继续保持 pending_evidence。**



## 58. QNT-CTR-005 Quant AuthorizationRef 签发与读取审计边界（2026-11-11）

### 58.1 Owner 与可信主体

- Quant 是 Quant Result 资源的唯一授权 Owner；Data/News/Research 的授权结论不能代替 Quant 对自身 result_id 的访问决策。
- `subject_ref` 必须来自 Server 注入的可信 Platform Principal Context；禁止从 Request DTO、普通 Header、Query、AuthorizationRef 自报字段或客户端参数读取主体身份。
- Quant 的直接跨模块调用方是 Researchflow；MVP 的 Quant AuthorizationRef.subject_ref 使用可信 Researchflow service Principal 的稳定 Subject，项目 Golden 示例为 `service:researchflow`。若实际 Principal.Subject 不同，必须使用可信 Context 中的真实值，不得硬编码或伪造 Golden 主体。
- Quant 不解析 Bearer、Session Cookie、API Key 或 Provider Token；认证和 Principal 建立由 Platform/Server 负责，Quant 只消费可信 Principal/Scope。
- `resource_ref` 必须逐字等于本次结果的内容寻址 `result_id`；不得使用 result_type、security_id、业务 ID 或 ManifestFingerprint 的非规范变体代替。

### 58.2 Quant 首批授权绑定

Quant 首批结果访问 AuthorizationRef 固定：
- `policy_owner=quant`
- `subject_ref=可信 Principal.Subject`
- `resource_ref=result_id`
- `purpose_code=quant_analysis`
- `content_level=full`

其他字段遵循 GOV-CTR-006：
- `policy_version` 必须等于完整不可变 PolicySnapshot.snapshot_ref，不是单条策略版本。
- PolicySnapshot 必须至少包含一项策略，并完整列出本次决策实际生效的全部策略版本；首批 Golden 使用 `quant_result_access / quant-result-access / 1.0.0`。
- `decision_fingerprint` 必须按 GOV-CTR-006 规定，将 AuthorizationRef 字段与完整 evaluated_policy_versions 组合后执行 RFC 8785 JCS + SHA-256；不得仅哈希 policy_version。
- allowed 的 conditions 必须为空；conditional 必须有条件，且不是访问许可。条件满足后必须重新评估并签发新的 allowed 引用。
- denied/unknown 不得返回结果 Payload。expires_at=null 只表示无固定过期时间，不代表永久授权或免除读取时复核。
- AuthorizationRef 是不可猜测的审计引用，不是 Bearer Token，不得单独作为访问凭证，也不得放入 Manifest/result_fingerprint。

### 58.3 签发时点与读取时复核

1. Quant 创建结果并获得 Manifest/result_id 后，必须基于当前可信 Principal 和当前 Quant PolicySnapshot 对该 result_id 作出授权决策；只有 allowed 才能将结果内容返回给调用方。
2. 每次 GetResult、Ref 投影或其他可能暴露结果元数据/内容的读取，都必须由 Quant Owner 按当前策略重新授权；历史 allowed AuthorizationRef 仅供审计，不能作为后续读取的通行凭证。
3. 每次读取的 AuthorizationRef 是当前调用主体的授权证明；因此它可随 subject、当前策略和 decided_at 改变，不进入 Manifest 或结果内容身份。Ref/Envelope 返回的 authorization_ref 必须对应本次当前授权，而不是无条件复用创建时的历史引用。
4. denied、unknown、conditional、过期或授权依赖不可用时，不得返回受限 Payload；不得回退缓存的历史 allowed，不得将授权失败转换为无授权的成功响应。
5. 读取流程必须有明确线性化点：在释放 Payload 前，确认授权决策所依据的策略版本仍是当前有效版本。若策略在检查后、内容释放前发生变化，必须重新授权或丢弃内容并拒绝；无法证明授权检查与内容读取处于一致策略边界时 fail closed。
6. 对 allowed、denied、conditional、unknown 的签发/读取决策均保留可追溯审计记录，至少关联 AuthorizationRef、PolicySnapshot、subject_ref、resource_ref、purpose_code、content_level、decision_fingerprint 和 decided_at。审计记录不得包含 Secret、原始凭证或受限 Payload 正文。

### 58.4 与结果身份及跨模块职责的关系

- AuthorizationRef 与 PolicySnapshot 由 Quant Owner 签发/保存；Researchflow 只消费并验证引用，不复制 Quant 的策略判断。
- Result Manifest/result_id 标识不可变结果内容和计算语义；AuthorizationRef 标识某个主体在某个时点对该结果的授权决策。两者分离，避免同一结果因访问主体变化而改变 result_id。
- `QuantResultRef.authorization_ref` 与 `QuantResult Envelope.authorization_ref` 都必须表示当前请求的授权结果；它们不是 Manifest 字段。
- 读取时必须验证 AuthorizationRef.policy_version 与对应 PolicySnapshot.snapshot_ref 一致、策略 tuple 完整/排序/唯一、fingerprint 正确，并向 Quant 当前策略 Owner 复核；消费者不得仅验证静态 fingerprint 就放行。
- Server 负责认证和可信 Principal 注入；Quant 负责资源授权决策和结果访问门禁；Researchflow 负责保留引用并遵守 Quant 返回的授权结论。

### 58.5 当前证据与冻结结论

- 当前项目 Golden 已验证 Quant AuthorizationRef 的 policy_owner、subject_ref、resource_ref、purpose_code、content_level、PolicySnapshot 和 JCS/SHA-256 指纹映射。
- 当前 `internal/quant/registry/manifest_authorization_test.go` 只执行静态 Manifest/AuthorizationRef/PolicySnapshot Schema 与 Golden 校验；没有运行时 Quant Policy Provider、AuthorizationRef 签发服务、可信 Principal 绑定、每次读取的当前策略复核、拒绝无正文断言或策略变更竞态测试。
- 当前仓库没有独立 Quant Authorization Application/Port/Adapter；不能将静态 Golden 测试描述为授权能力已实现。
- QNT-CTR-005 继续保持 pending_evidence；本轮不修改 Quant 代码、Schema、Golden 或 CI。

**结论：QNT-CTR-005 的授权主体/资源/用途/内容级别、策略快照、fingerprint 与读取时 fail-closed 语义已收敛；运行时签发、当前策略复核、审计持久化和竞态一致性仍未形成证据。**


## 59. 项目级契约最终收敛结论（2026-11-11）

本节覆盖此前各批次的阶段统计。正式 CTR 编号仍为原有 43 项，不新增、不重编号。最终裁决依据为 DEC-PLAT-002-051；本节是当前状态与下一步的唯一有效汇总。

### 59.1 最终状态

| 状态 | 数量 | 契约 |
|---|---:|---|
| `freeze-ready` | 37 | GOV-CTR-001～006；PLAT-CTR-001～007；DATA-CTR-001/002/004/006/008；NEWS-CTR-001～003；QNT-CTR-001/002/004/005；RSC-CTR-001/002；FLOW-CTR-001～004；LLM-CTR-001～003；NTF-CTR-001/002；INT-CTR-001 |
| `deferred` | 4 | DATA-CTR-005、DATA-CTR-007、QNT-CTR-003、SRV-CTR-001 |
| `not_applicable` | 2 | DATA-CTR-003、NTF-CTR-003 |
| `pending_evidence` | 0 | 无 |
| `pending_decision` | 0 | 无 |
| `frozen` | 0 | 等待项目负责人对 37 项候选统一确认 |

### 59.2 四项明确延期及重新进入条件

| CTR | 延期原因 | 重新进入条件 |
|---|---|---|
| DATA-CTR-005 | DailyBars Decimal v2 尚无足够真实字段/精度依据；保留现有 v1 number/float64，不静默改类型 | Data v2 的字段、Decimal scale/舍入、单位/缺失语义、interval 和交易日历版本引用形成完整权威定义 |
| DATA-CTR-007 | Snapshot Manifest 当前只定义快照范围/来源元数据，不能冒充行情 Payload 内容寻址引用；不把 Bundle.snapshot_id 猜作 SnapshotRef | Data 定义不可变行情 Payload、内容寻址 SnapshotRef、Manifest 与 Payload 绑定，以及 Research Evidence source_ref/version/locator 映射 |
| QNT-CTR-003 | 首批 Factor 的正确性依赖 DATA-CTR-005/007 的完整日线 Snapshot、interval 和不可变交易日历版本；缺少这些输入证据时冻结会制造虚假可比性 | DATA-CTR-005/007 达到 freeze-ready，Quant Factor 输入/结果定义与其引用关系完成定点复核 |
| SRV-CTR-001 | Root Server 组合根职责已明确；新 Researchflow HTTP API 尚无正式活动路由/消费者范围。旧 Analysis API 已退役，不得复用旧 DTO 冒充新接口 | 确定 WebUI 与 Server 的新 HTTP 用例、路由/方法、请求响应 DTO、认证矩阵、错误投影、分页/Cursor 和事件读取后，形成唯一 OpenAPI |

延期是明确的范围控制，不是遗漏，也不阻断当前 37 项契约定义冻结。任何延期项不得被解释为已发布接口。

### 59.3 已完成定义与后续实现符合性边界

1. 项目公共规则 GOV-CTR-001～006 和 Platform 原语 PLAT-CTR-001～007 均为 freeze-ready。
2. Data 首期公开语义以 DEC-027/038/041、数据质量与状态契约、DATA-CTR-006/008 权威规则为准。旧 OpenAPI/Handler 中 Provider 原始字段或 Bearer 兼容行为属于实现偏差，不反向改变冻结定义；Data 责任范围后续按契约同步。
3. News 当前 v1 以 DEC-042～044 和 `doc/news/新闻API契约.md` 第11节为准；旧 News-Research 建议稿降级为历史参考。
4. Quant 首期定义以 DEC-045～050 和 Quant/Research 字段级契约为准。没有 Registry Entry 的 metric_code 不对外可用；不为未来指标伪造单位、精度或质量阈值。
5. Research、Researchflow、LLM、Notification 的跨模块边界分别由其 Application Port/DTO/事件契约定义。纯 InProcess Port 的强类型 DTO 与字段级契约可作为等价机器定义；内部实现、Provider SDK、Worker、Repository、CAS、Lease 和真实 Gateway 接入属于 L2。
6. Schema/Golden/Go Validator 未同步、消费者测试、CI、部署和真实依赖验收不再作为 L1 定义冻结门禁；但任何已经作为活动 JSON/OpenAPI/事件定义发布的机器契约仍必须在后续 L2 中与冻结语义一致，不得长期双权威并存。
7. INT-CTR-001 复用 GOV-CTR-002 时间语义及 NTF-CTR-001 Envelope/Payload 规则，不复制独立时间/指纹规则。

### 59.4 本轮结论

- 43 项正式 CTR 均已获得明确终态分类；没有待决决策，也没有未分类的定义缺口。
- 37 项达到 freeze-ready，可提交项目负责人一次性统一确认冻结；4 项有明确依赖和触发条件的 deferred；2 项 not_applicable。
- 本轮契约定义审查到此结束。除非项目负责人指出具体错误或发生契约变更，不再重复全量 Review，不再逐项开启同类审查。
- 后续工作属于各模块 L2 实现符合性与集成验收，不属于本轮契约冻结任务。
