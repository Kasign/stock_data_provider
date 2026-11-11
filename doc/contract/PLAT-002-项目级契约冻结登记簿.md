# PLAT-002 项目级契约冻结登记簿

- 仓库：`Kasign/stock_data_provider`
- 代码审计基线：`origin/integration` @ `b6dfc85ba214fe16268c8cba780641a8bfe15086`
- 审计文档维护分支：`origin/feat/design_doc`（状态以当前分支 HEAD 为准）
- 固定代码审计基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`；仅对该 SHA 的代码事实作结论，不随分支 HEAD 自动漂移
- 登记簿版本：`0.2.25-review`
- 当前状态：`L1_frozen_contract_definition_review_in_progress`
- 冻结结论：43 个正式 CTR 已完成定义口径核定：1 freeze-ready、37 pending_evidence、3 pending_decision、2 not_applicable、0 frozen；5 项具体待决见第24节
- 项目级决策：DEC-PLAT-002-018～027 已完成原有裁决；当前另有 5 项领域公开契约待决（QNT-DEC-01/02、RSC-OPEN-003/004/005）；DATA-FLOW-B、LLM-FLOW-B 保持条件性 deferred
- 决策责任：项目负责人
- 复核原则：本轮只冻结契约定义；模块实现符合性、内部测试、CI、部署另行跟踪，不得混入本轮冻结门禁

> 本登记簿是项目级契约状态与冻结结论的唯一权威入口。第1节及第3节保留旧综合审查状态；当前“契约定义能否冻结”按第5节门禁和第13节收敛项判断。实现符合性、模块测试和 CI 不得混入本轮契约定义冻结状态。

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
| GOV-CTR-002 | 时间、交易日、时区、PIT、system_as_of | 项目级时间语义；Data/News/Quant/Research/Researchflow | 所有历史研究及执行消费者 | 各领域时间字段定义；旧目录保留历史规则 | 规则版本待核验；字段 Schema 分别版本化 | `pending_evidence` | system_as_of、available_at、published_at、occurred_at、valid_time、fetched_at、normalized_at 映射；UTC/RFC3339Nano、Asia/Shanghai 交易日、严格 PIT |
| GOV-CTR-003 | ID、Request/Correlation/Event/Idempotency 作用域 | Platform 提供 Request ID 原语；各模块拥有领域 ID | Server、所有模块、Worker、事件消费者 | Platform observability；各模块事件/任务 DTO | 规则版本待核验；Golden 待核验 | `pending_evidence` | ID 作用域、生成方、异步传播、幂等 fingerprint、跨进程关联和 Header/Context Golden |
| GOV-CTR-004 | Schema 版本、兼容、废弃和迁移 | 各契约生产模块；项目级规则由治理登记 | 所有直接消费者 | 各模块 Schema Registry；doc/contract 版本规则 | 规则版本待核验；兼容矩阵待核验 | `pending_evidence` | 每个 DTO/Event 的版本、兼容级别、未知字段/枚举、废弃窗口、迁移和回滚 |
| GOV-CTR-005 | 契约状态、冻结证据和变更审批 | 项目契约治理 | 所有模块 | doc/contract/README、登记簿第5节、决策记录、契约版本与变更记录 | 0.2.19-review（治理规则） | `freeze-ready` | 项目级定义冻结门禁、状态、版本变更/解冻记录规则已统一；模块实现符合性不混入契约定义冻结 |
| GOV-CTR-006 | AuthorizationRef 公共审计快照结构与引用语义 | Data、News、Quant、Research 分别签发各自资源授权快照；公共结构由项目治理 | Research、Researchflow 及受限资源消费者 | 各模块签发 Schema；旧目录 GOV-CTR-006 机器契约/Golden | 1.0.0（2 Schema / 4 Golden） | `pending_evidence` | Principal/Resource 映射、Scope、purpose、snapshot fingerprint、过期/撤销、当前授权复核和竞态 |
| PLAT-CTR-001 | Fault 结构与分类 | Platform | Server、所有服务端模块 | doc/platform/08 第 4 节；internal/platform/fault | Go API 版本待核验；未见独立 Schema/Golden | `pending_evidence` | Fault 内部结构、HTTP Error DTO、状态码、错误码注册、Retryable 语义和各模块映射 Golden |
| PLAT-CTR-002 | Request Context 与身份关联原语 | Platform | Server、所有服务端模块、Worker | doc/platform/08 第 5 节；observability/security Context | Request ID Golden 已有；Schema 未见 | `pending_evidence` | 可信入口、Request ID、Principal/Scope 传播、异步 Context、取消和出站 Header 覆盖 |
| PLAT-CTR-003 | 身份、授权、ServiceKey、Secret 边界 | Platform（认证原语）；Server（入口装配）；领域（资源授权） | Server、Data、News、Research、Quant、Researchflow、WebUI | doc/platform/08 第2～3节及第18节候选定义；DEC-PLAT-002-017 | Bearer/Session 原语版本待核验；完整 Principal/Session 契约定义未登记 | `pending_evidence` | 身份类型和凭证隔离语义已有决议；Platform Session 候选接口仍未形成权威冻结定义。Server 路由接入、WebUI 登录和部署证据属于后续实现符合性，不作为本轮门禁。资源授权仍归领域 Owner |
| PLAT-CTR-004 | Diagnostics Observation 与 Runner | Platform（Runner）；领域（reason_code/质量语义） | Server、各领域模块、WebUI | doc/platform/08 第7、14、24节；PLAT-CTR-004 Schema/Golden | 1.0.0（Observation Schema / 2 Golden） | `pending_evidence` | Schema 将 attributes 限定为 string，而规范描述为结构化 scalar；UTC、reason_code 条件约束及 Runner 聚合语义需统一为一份完整权威定义。Adapter/真实消费者测试不纳入本轮 |
| PLAT-CTR-005 | HTTP Client、SSRF 与出口安全 | Platform | Data、News、LLM 及其他出站消费者 | doc/platform/05、15；internal/platform/httpclient | HTTP Client/Stream API 版本待核验；Schema 未见 | `pending_evidence` | DestinationPolicy、BoundDoer、DNS/IP、redirect、流式 Body 生命周期、超时/并发和真实消费者 A2 |
| PLAT-CTR-006 | Storage、Repository、Migration 生命周期 | Platform（连接/Runner）；领域（Repository/Schema/Migration 内容）；Server（汇总注册） | 所有持久化模块 | doc/platform/08 第9节及 Migration 专项定义 | Migration API 版本待核验；唯一权威接口契约未登记 | `pending_evidence` | 平台连接/Runner 与领域 Repository/Schema/Migration 内容的职责已原则性明确；Migration 接口输入输出、namespace/version、失败/Ready/Shutdown 语义仍缺完整权威定义。Server 注册差异属于实现符合性 |
| PLAT-CTR-007 | Typed Config 与 Secret 生命周期 | Platform（基础解析/脱敏）；各模块（配置 Schema）；Server（Bootstrap） | Server、所有服务端模块 | doc/platform/08 第 6 节；各模块 Config | Config API 版本待核验；Schema 未见 | `pending_evidence` | 配置所有权、读取时点、不可变快照、Secret 注入/轮换/脱敏和启动校验 |
| DATA-CTR-001 | 标准数据 DTO、JSON/OpenAPI、字段缺失语义 | Data | Quant、Researchflow、Research（适用范围）、Server、WebUI | internal/marketdata/httpapi/openapi.yaml；doc/api/contracts/数据层API清单.md（存在过期路径/认证描述） | OpenAPI 1.1.0；字段级映射未形成完整基线 | `pending_evidence` | 公开 DTO 的字段存在/缺失/null/零值/required 语义映射不完整；DragonTiger DTO 仍暴露 DEC-027 明确禁止的 Provider 原始金额/单位字段；API 清单仍引用旧 OpenAPI 路径和旧浏览器 Key 模型 |
| DATA-CTR-002 | 行情单位、复权、交易日和时间口径 | Data | Quant、Researchflow、Research、Server | doc/api/contracts/数据质量与状态契约.md；internal/marketdata/httpapi/openapi.yaml | 领域规则版本未登记 | `pending_evidence` | 通用单位/质量/复权/PIT 原则已有；Quote/DailyBar/Index/Financial 等字段到 provider_time、available_at、fetched_at、normalized_at、system_observed_at 的映射及各数据域单位适用表仍不完整 |
| DATA-CTR-003 | Provider 原始值标准化与精度保真 | Data 内部 Provider Adapter | Data 内部 Provider Adapter | doc/api/数据侧整体规划.md（仅内部适配约定） | 不属于跨模块公开契约版本 | `not_applicable` | 仅 Data 内部 Provider 标准化规则，不存在跨模块公开消费者；从 PLAT-002 跨模块契约冻结门禁排除，不再追踪 Provider Fixture/专项测试 |
| DATA-CTR-004 | Data Quality、覆盖率、来源和降级表达 | Data | Quant、Researchflow、WebUI、Server | doc/api/contracts/数据质量与状态契约.md；internal/marketdata/httpapi/openapi.yaml | QualityStatus 已有枚举；各质量/覆盖 DTO 映射未完整登记 | `pending_evidence` | QualityStatus 与 Probe 状态已有定义；各公开响应中 quality_status、quality_reasons、freshness_status、coverage dimensions/分母和 source/version 的适用字段映射仍不完整，不能将诊断内部状态等同业务质量 |
| DATA-CTR-005 | 精度保真的跨模块日线 DTO（v2） | Data | Quant、Researchflow | internal/marketdata/httpapi/openapi.yaml；Data v2 Decimal Schema 尚未登记 | v1.1.0 number；v2 版本与 Schema 未定义 | `pending_evidence` | v1 number 保持既有语义；v2 的 Decimal string 字段、精度/scale/舍入/缺失规则、版本和与 Snapshot 引用的关系尚无权威 Schema。Provider 内部精度实现不纳入本轮 |
| DATA-CTR-006 | Data HTTP API 调用认证 | Data；Server 负责 Root 边界和 Remote Adapter Secret | Server、Quant Data Adapter；仅远程兼容模式下的 HTTP 调用方 | internal/marketdata/httpapi/openapi.yaml；Data服务间认证映射与Secret注入设计.md；DEC-PLAT-002-017 | Remote X-API-Key；InProcess 可信 Principal/Scope Context | `pending_evidence` | 模式边界已明确；但 OpenAPI 全局 security 同时声明 ApiKey 与 BearerAuth（OR 语义），与认证设计中“业务 API 仅 X-API-Key”的定义冲突；旧 API 清单仍描述浏览器持有 Key。需统一公开安全定义。实现接线不纳入本轮 |
| DATA-CTR-007 | Research Data Snapshot/Evidence 消费边界 | Data（数据事实）；Research（Evidence）；Server（Adapter 注入） | Research、Researchflow | Data v2 DTO/Schema（待定义）；DATA-CTR-008 Snapshot Manifest | DailyBarsV2 Schema 未登记 | `pending_evidence` | Snapshot Manifest 只定义查询范围和来源版本，不等同 DailyBarsV2 行情内容 DTO；DailyBarsV2 字段、版本、质量/单位、Snapshot 引用与 Evidence 消费映射尚缺权威定义 |
| DATA-CTR-008 | Data AuthorizationRef 签发与快照访问审计 | Data | Quant、Research | DATA-CTR-008 Snapshot Manifest Schema/Golden；GOV-CTR-006 AuthorizationRef/PolicySnapshot | 1.0.0（Snapshot Manifest Schema/Golden） | `pending_evidence` | Manifest 字段与 JCS/SHA-256 规则已定义；Schema 未约束 system_as_of 的 UTC 形式、sources 规范排序/去重完整语义；Data caller_ref、Scope/purpose 到可信主体及授权读取复核的公开映射仍需收敛 |
| NEWS-CTR-001 | News 只读 Evidence Query Facade | News | Research、Researchflow | doc/news/新闻API契约.md 第11节；doc/news/schema/NEWS-CTR-001-*；doc/news/golden/NEWS-CTR-001-* | 1.0.0（4 Schema / 3 Golden） | `pending_evidence` | Application Port 查询定义已较完整；Schema 与文字存在 business_time nullable、UTC、system_observed_at 字段语义及 relation_refs 未提供/空集合状态差异，需统一公开 DTO 定义。未来 HTTP OpenAPI 草案不属于当前活动接口 |
| NEWS-CTR-002 | AuthorizationRef 与内容访问授权 | News（授权决策 Owner） | Research、Researchflow | doc/news/新闻来源授权矩阵.md 第10节；NEWS-CTR-002 Authorization Binding Schema/Golden；GOV-CTR-006 | 1.0.0（Authorization Binding Schema/Golden） | `pending_evidence` | News-specific caller/resource/purpose/content_level 与 Provider/Source/Endpoint 策略交集已定义；仍依赖 GOV-CTR-006 的时间/规范排序及通用授权快照适用语义收敛 |
| NEWS-CTR-003 | News PIT、稳定 Cursor、排序与去重 | News | Research、Researchflow | doc/news/新闻API契约.md 第11.3/11.6/11.9节；NEWS-CTR-001 Cursor Claims Schema；doc/news/新闻数据模型与质量契约.md | 复用 NEWS-CTR-001 1.0.0 Cursor Schema；无独立 CTR-003 Schema | `pending_evidence` | PIT 过滤、15分钟快照、HMAC Cursor、排序版本已有定义；system_observed_at 与 observation/first_observed_at 权威归属不一致，且 Evidence relation_refs 未表达 DEC-021 的 not_provided 与 confirmed-empty 区分 |
| QNT-CTR-001 | Metric/Indicator/Factor Registry 公共元数据 | Quant | Researchflow；Research 只消费编排结果 | doc/quant/schema/QNT-CTR-001-registry-entry-1.0.0.schema.json；doc/quant/03 | 1.0.0（Registry Entry Schema；正式首批 Registry Entry/Golden 未形成） | `pending_evidence` | Registry Entry 结构 Schema 已有；但 code 对应的首批实际 Registry 条目及其 unit/precision/range/algorithm_version/input_requirements/quality_policy 权威值未形成完整登记，不能仅冻结空壳 Schema |
| QNT-CTR-002 | Indicator Request/Result | Quant | Researchflow | doc/quant/schema/QNT-CTR-002-indicator-parameters-1.0.0.schema.json；doc/quant/03 | 1.0.0（Parameters Schema；8 个算法参数定义） | `pending_evidence` | 参数 Schema/算法口径已有；完整 Indicator Request/Result DTO、指标输出字段/单位/精度/状态映射及每个指标的 Registry 元数据仍未形成唯一完整契约 |
| QNT-CTR-003 | Factor Request/Result | Quant | Researchflow | doc/quant/schema/QNT-CTR-003-factor-parameters-1.0.0.schema.json；QNT-CTR-004 Factor QuantMetric Schema/Golden | 1.0.0（Parameters Schema；3 个首批 Factor） | `pending_evidence` | 首批三个 Factor 的参数/算法及局部 QuantMetric Schema 已有；完整 Factor Request/Result DTO、交易日历版本引用、输入质量和结果 Envelope 映射仍不完整 |
| QNT-CTR-004 | QuantResultRef、Factor QuantMetric、Trend、Risk 输出 | Quant | Researchflow；Research 经 Researchflow 消费 | doc/quant/schema/QNT-CTR-004-*；doc/quant/03 第10.26节；项目级 Quant-Research公开DTO字段级契约第2节 | 1.0.0（Ref/Metric/Trend/Risk 局部 Schema/Golden） | `pending_evidence` | 已核验责任模块 QuantResultRef Schema 仍以 minItems=1 限制 input_snapshot_refs，与 DEC-029 冲突；项目级 QuantResultRef DTO/Golden 已允许经双重授权的空数组。Envelope Schema/Golden 尚缺；已有 Factor Metric、Trend、Risk 是局部 Payload，Indicator 尚无独立完整 Payload Schema，其他预留 result_type 无完整强类型 Payload。不得将局部 Schema 推断为完整 Envelope 支持；需补齐 Ref/Envelope/已开放 Payload 的权威映射后才能 freeze-ready |
| QNT-CTR-005 | QuantResult AuthorizationRef 签发与访问审计 | Quant | Researchflow | doc/整体任务规划/schema/QNT-CTR-005-result-manifest-1.0.0.schema.json；doc/整体任务规划/golden/QNT-CTR-005-result-manifest.golden.json；GOV-CTR-006 | 1.0.0（Result Manifest Schema/Golden） | `pending_evidence` | 项目级 Manifest Schema/Golden 已按 DEC-028/029 区分 Manifest/Payload 版本并允许空数组；但 Quant 责任模块 Manifest Build/Verify 仍要求非空 input_snapshot_refs，且旧文档仍将 Manifest schema_version 解释为 Payload 版本，均与正式决议冲突。另需核对 Record/VerifyPayload 的 JCS 指纹权威输入、Manifest 与 Ref 的身份绑定，以及 AuthorizationRef 的可信主体/资源/用途/内容级别映射 |
| RSC-CTR-001 | Research Evidence | Research | Researchflow | doc/整体任务规划/schema/RSC-CTR-001-evidence-1.0.0.schema.json；Golden；doc/research/01、03；GOV-CTR-006 | 1.0.0（Evidence Schema/Golden） | `pending_evidence` | Evidence 字段和 JCS 指纹语义已有；Schema 未约束 RFC3339Nano UTC，source_version 条件必填和 Evidence.system_as_of/available_at 跨字段 PIT 规则仍依赖未完成的权威适用映射。ResearchContext 不归本 CTR，按 FLOW-CTR-003 核定 |
| RSC-CTR-002 | ResearchEvent、Claim、Hypothesis、Scenario | Research | Researchflow | 项目级 DTO 第3.3～3.5节；schema/RSC-CTR-002-*；golden/RSC-CTR-002-*；doc/research/01、03 | Event、Claim/Hypothesis、Scenario 1.0.0 Schema/Golden 已建立 | `pending_evidence` | Event/Claim 核心字段已对齐 DEC-030～032；仍缺 EventRef 存在性/无环、source_times 与 evidence_refs/规范化值的跨字段校验、Claim 替代目标当前版本/并发冲突校验，且 Research 责任模块文档仍保留旧 supersedes_event_version 并引用已裁决待决项。Scenario expected_value 已按 DEC-033 收敛为 scalar/非空 scalar 数组，仍缺 metric_ref 对应 Registry value_type/unit 的权威映射；QuantResultRef 的 result_type=evaluation、system_as_of 一致性和引用 Schema 与 DEC-029 的一致性未闭合 |
| FLOW-CTR-001 | Run/Stage/Attempt 状态与结果传播 | Researchflow | Server、API、WebUI；业务模块作为 Stage Provider | doc/researchflow/03；doc/researchflow/schema/run/stage/attempt；对应 Golden | 1.0.0（Run/Stage/Attempt Schema/Golden） | `pending_evidence` | 状态枚举/传播矩阵已有；Run/Stage/Attempt Schema 的 UTC、终态字段关系和 ResultSummary 关联约束未完全机器化，完整结果归并定义需与 FLOW-CTR-004 统一 |
| FLOW-CTR-002 | Cancel、Retry、Deadline、Lease、Fencing | Researchflow | Server（命令入口）；Scheduler/Worker（内部执行） | doc/researchflow/03 第3～5节、第6.2节；Attempt Schema/Golden | 1.0.0（Attempt Schema/Golden）；命令 DTO Schema 未登记 | `pending_evidence` | 取消/重试/deadline 的外部可观察状态与稳定错误结果已定义；CancelRun/RetryStage 命令字段、幂等作用域/请求指纹和 deadline/cancel 冲突结果未形成唯一权威机器/强类型契约。CAS、锁、重启恢复测试不属于本轮门禁 |
| FLOW-CTR-003 | Run Execution Context 与 system_as_of 传播 | Researchflow | Data、News、Quant、Research、LLM | doc/researchflow/03 第6.1/6.2节；ResearchContext DTO 第3.1节；GOV-CTR-002 | Run Schema 1.0.0 部分承载；独立 Context/ResearchContext Schema 未登记 | `pending_evidence` | Run Context 字段语义已有；ResearchContext/ContextSectionRef、模板解析快照、可信 caller/correlation 与各下游 Adapter 的字段映射未形成单一权威 Schema/Port 契约 |
| FLOW-CTR-004 | Stage 输入输出、诊断与最终报告关联 | Researchflow | 所有 Stage Provider、Server、WebUI | doc/researchflow/03 第2.2/2.3/6.2节；Stage/ResultSummary Schema/Golden | 1.0.0（Stage/ResultSummary Schema/Golden）；Stage Provider Result Envelope Schema 未登记 | `pending_evidence` | Stage Provider Result 的 output_type/schema_version/payload/quality_summary/diagnostic_refs 只有字段表，无权威 Schema/Output Registry；ResultSummary.quality_status 仍为任意 string，coverage 分母及 summary 资源身份/版本绑定未定义完整 |
| LLM-CTR-001 | Provider Port、ModelRequest/Response、Usage、Tool Call | LLM | Researchflow | doc/llm/04 第2～5节；ProviderConfig/CapabilityState/ModelRequest/ModelResponse Schema 1.0.0 | 1.0.0（Request/Response/Config/Capability Schema 与 JSON Fixtures） | `pending_evidence` | 主字段与条件状态已有定义；UTC RFC3339Nano 未在相关 Schema 中约束，ContentPart.annotations Schema 允许任意值而 DTO 定义为 Annotation[]，部分未知/条件字段的结构约束仍过宽 |
| LLM-CTR-002 | Stream/SSE 事件、终止、断流和序号 | LLM | Researchflow | doc/llm/04 第6节；doc/llm/05；ModelEvent Schema 1.0.0 | 1.0.0（ModelEvent Schema 与 SSE Fixtures） | `pending_evidence` | terminal/EOF/序号连续性/未知事件/取消语义已有；ModelEvent received_at UTC 约束未体现在 Schema，sequence_number 初始/合法范围及 terminal 后事件的公开结构约束尚未形成完整机器定义 |
| LLM-CTR-003 | Platform Fault → ModelErrorInfo 映射 | LLM Adapter | Researchflow；依赖 Platform Fault | doc/llm/04 第7/11节；ModelErrorInfo Schema 1.0.0；PLAT-CTR-001 | 1.0.0（ModelErrorInfo Schema） | `pending_evidence` | Fault Class→ModelErrorKind、stable_code、SafeMessage、Retryable 和 Cause 隔离规则已有；但 Platform Fault 的公开分类/稳定 Code 契约尚未冻结，跨层映射无法独立冻结 |
| NTF-CTR-001 | NotificationEvent Envelope、Schema Registry、Fingerprint | Notification | Quant、Research、Researchflow | doc/notification/03 第2.1/8.1节、05；Envelope 与三类 Payload Schema/Golden | 1.0.0（4 Schema / 6 Golden） | `pending_evidence` | Envelope/首批事件映射/fingerprint 算法已有；Schema 未约束 RFC3339Nano UTC，Payload reason_code/opaque ref 的稳定格式及 subject_ref 与 payload 主体一致性仍依赖文字定义，需统一机器契约约束 |
| NTF-CTR-002 | Inbox、Subscription、Delivery、Attempt、幂等和状态机 | Notification | Server；Quant/Research/Researchflow 仅发布事件 | doc/notification/03 第2～7/9节、06；Notification Application Port 与 Domain 类型 | 1.0.0（Event Schema 已有；Subscription/Delivery/Attempt Schema 未登记） | `pending_evidence` | Event 接收/Inbox 双唯一键、Delivery 唯一键和状态机语义已有；Subscription、DeliveryPlan、DeliveryRecord、DeliveryAttempt、DeliveryStateEvent 的完整跨模块 DTO/Port 权威定义与版本/错误语义未形成可核对的机器 Schema/等价 Application Port 契约 |
| NTF-CTR-003 | Notification 内部 Channel Adapter SPI 与 ChannelConfigSnapshot | Notification 内部扩展点 | Notification 内部 Channel Adapter/Resolver | doc/notification/03 第8.4节；internal/notification/application/channel_ports.go；internal/notification/domain/channel.go | 内部 Port，不属于跨模块公开契约版本 | `not_applicable` | ChannelConfigSnapshot、ChannelAdapter、ChannelResolver、RecipientResolver、ChannelSendRequest/Result 仅由 Notification 内部组件消费；不作为 PLAT-002 跨模块冻结门禁。Delivery 的 retry/unknown 对外语义归 NTF-CTR-002，不重复登记 |
| SRV-CTR-001 | Root Server 组合根、统一 HTTP 宿主与 Researchflow 对外 API | Server | WebUI、Researchflow、所有服务端模块 | doc/server/README.md；doc/web/API_CONTRACT.md（旧 Analysis 路径）；doc/api/contracts/分析任务.openapi.yaml（已退役旧 API） | 新 Researchflow HTTP Schema/版本未定义 | `pending_evidence` | 组合根/唯一 HTTP 宿主职责已有定义；新 API 路径/方法、CreateRun/Run/Stage/Event HTTP DTO、错误投影、认证授权、分页/Cursor 与 WebUI 消费映射均无唯一权威定义。旧 Analysis API 按 DEC-018/019 退出活动范围，不得复用其 DTO/约束 |
| INT-CTR-001 | Quant 风险决策、Research 事件生命周期、Researchflow Run 终态到 NotificationEvent 显式映射 | Quant/Research/Researchflow 产生业务事实；Notification 接收事件 | Notification、事件消费者 | doc/notification/05；doc/notification/03；3 类 Payload Schema/6 Golden | 1.0.0（3 Event Payload Schema；6 Golden） | `pending_evidence` | 触发时点、subject_ref、source_event_ref、幂等键和 JCS fingerprint 已定义；事件/业务时间 UTC 约束、subject_ref 与 payload 主体一致性、changed_block_reason_codes 排序和来源时间字段绑定尚未完整机器化 |

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

### 14.1 核定结果

| 契约 | 本轮状态 | 核定理由 |
|---|---|---|
| GOV-CTR-001 | `pending_evidence` | 公共规则已定义 JSON 类型、Decimal、整数、比例及缺失/null/零值原则；但规则没有独立稳定版本，字段级 Schema 适用映射尚未形成唯一索引，舍入/scale 等字段语义仍需由生产方逐项引用 |
| GOV-CTR-002 | `pending_evidence` | UTC/RFC3339、交易日 Asia/Shanghai、system_as_of 与 PIT 原则已明确；但 published_at/available_at/observed_at/valid_time 等时间字段到领域 DTO 的权威映射不完整，不能将原则文档当作所有字段的实际定义 |
| GOV-CTR-003 | `pending_evidence` | entity/request/originating_request/correlation/event/idempotency/trace/ref 的概念与不可替代关系已定义；但各跨模块命令、事件和异步上下文的生成方、唯一性作用域、传播字段仍未形成完整引用矩阵 |
| GOV-CTR-004 | `pending_evidence` | 兼容与破坏性变更原则已定义；但各活动 DTO/Event 的 schema_version、未知字段/枚举策略、兼容窗口和废弃版本尚无统一权威矩阵 |
| GOV-CTR-005 | `freeze-ready` | 项目级唯一入口、状态定义、范围、版本记录和变更审批规则现已统一到 doc/contract；旧的测试/CI冻结门槛已由登记簿第5节明确排除。该状态只表示治理规则定义可冻结，不代表本项目全部 CTR 已冻结 |
| GOV-CTR-006 | `pending_evidence` | AuthorizationRef 与 PolicySnapshot 已有 1.0.0 Schema 和 Golden，且 allowed/conditional/过期/撤销/读取复核原则清楚；但 Schema 未约束 RFC3339Nano UTC、conditions 与 policies 的规范排序，Data/News/Quant/Research 的 resource_ref 覆盖粒度及各自 purpose/content_level 适用映射尚不完整 |
| PLAT-CTR-001 | `pending_evidence` | Fault 的 Go 类型明确标注为进程内结构且禁止 JSON 序列化；HTTP Error DTO 的字段、状态码映射、错误码命名空间和稳定版本没有唯一权威公开定义。内部 Go Fault 不等同跨模块 HTTP 错误契约 |
| PLAT-CTR-002 | `pending_evidence` | Request ID Header、可信入口、随机生成和出站覆盖规则明确且有 Golden；但 Principal/Scope/认证来源 Context 的公开字段与异步传播边界未定义完整，不能以 Request ID Golden 代表整个 Request Context 契约 |
| PLAT-CTR-003 | `pending_evidence` | anonymous/owner_bearer/owner_session/service 的身份边界及 DEC-017 已明确；但 Platform Session 原语仍在模块文档中标为候选接口，缺少唯一权威的类型/操作/错误/生命周期契约。Server A2/WebUI A2/部署 A3 未完成不作为本轮阻断理由 |
| PLAT-CTR-004 | `pending_evidence` | Observation Schema 与两个 Golden 已存在，但 attributes 的 string Schema 与“结构化 scalar”文字定义不一致；UTC、reason_code 条件约束和 Runner 聚合规则也分散在多处，尚未构成无歧义的单一契约定义 |
| PLAT-CTR-005 | `pending_evidence` | 缓冲/流式请求、DestinationPolicy、DNS/IP、重定向、取消、Body 所有权、并发和终止语义已有详细设计；但稳定公开 Port/API 的权威版本、参数/返回错误语义及文档唯一入口尚未收敛 |
| PLAT-CTR-006 | `pending_evidence` | Platform 与领域/Server 的 Migration 所有权原则明确；但连接/事务/Migration Runner 的权威接口、输入输出、namespace/version、失败和关闭语义不完整。Server 是否实际注册属于实现符合性，不列为本轮缺口 |
| PLAT-CTR-007 | `pending_evidence` | 配置按服务所有、启动时不可变快照、Secret 显式访问和脱敏规则明确；但 Typed Config/Secret 的公共 API 版本、解析错误语义、轮换边界与各模块配置 Schema 的适用关系未形成唯一契约索引 |

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
| DATA-CTR-001 | pending_evidence | OpenAPI 1.1.0 是当前代码内 API 定义，但 Go JSON 与 OpenAPI 的字段存在/缺失/nullable/required 适用映射仍未形成完整权威表；DEC-027 的 DragonTiger Provider 原始金额字段仍公开暴露；API 清单引用旧路径且认证说明过期 |
| DATA-CTR-002 | pending_evidence | 质量契约已明确价格/成交量/成交额/百分点、复权和 PIT 基本原则；具体数据域字段到 provider_time、available_at、fetched_at、normalized_at、system_observed_at 的映射仍不完整，不能用一套泛化时间规则代替逐字段语义 |
| DATA-CTR-003 | not_applicable | 仅覆盖 Data 内部 Provider Adapter 原始值标准化，没有跨模块公开 DTO/Port 消费者；退出 PLAT-002 跨模块冻结门禁，不再要求 Provider Fixture 或专项测试作为项目级任务 |
| DATA-CTR-004 | pending_evidence | QualityStatus 已有 valid/degraded/stale/invalid/unknown；Probe 的维度状态、ProviderOutcome、freshness_status、coverage_ratio 与业务 quality_status 的关系已有文字规则，但公开响应字段适用映射及覆盖率分母语义仍需明确 |
| DATA-CTR-005 | pending_evidence | 当前 OpenAPI 1.1.0 DailyBar 仍是 number；未找到 DailyBarsV2 的独立 Decimal Schema/版本定义，v2 精度/scale/舍入/缺失语义尚未成为权威契约 |
| DATA-CTR-006 | pending_evidence | 同进程 Root Server 注入可信 Principal/Scope Context、远程兼容模式使用 Data 专用 X-API-Key 的边界已明确；但 OpenAPI 全局 ApiKey/BearerAuth 的 OR 声明与“业务接口仅 X-API-Key”文字冲突，旧 API 清单仍描述浏览器持有 Key |
| DATA-CTR-007 | pending_evidence | DATA-CTR-008 Manifest 定义了快照范围和来源版本，但它不是 DailyBarsV2 内容 Schema；跨模块行情 DTO、质量/单位、SnapshotRef 与 Research Evidence 的字段级关联尚缺定义 |
| DATA-CTR-008 | pending_evidence | Manifest Schema/Golden 已有，caller_ref/purpose_code、内容寻址引用和 JCS/SHA-256 算法已有文字定义；UTC/排序约束及 Data 可信 caller/Scope/授权快照读取时复核映射仍不完整 |

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
| NEWS-CTR-001 | pending_evidence | Query 请求/响应已有 1.0.0 Schema/Golden 和详细字段定义；但 business_time_from/to 文档允许 nullable、Schema 仅允许 string；system_as_of 要求 RFC3339Nano UTC、Schema 只声明 date-time；system_observed_at 与 News 数据模型声明的观察时间权威归属存在冲突；relation_refs 未表达 DEC-021 的 not_provided 与已确认空关系的区别 |
| NEWS-CTR-002 | pending_evidence | Authorization Binding Schema/Golden 已定义 caller/resource/purpose/content_level 与 GOV-CTR-006 引用；News 侧签发与读取复核、Provider/Source/Endpoint 策略继承和 conditional 规则已明确；仍依赖 GOV-CTR-006 的 UTC/排序和公共授权快照语义收敛 |
| NEWS-CTR-003 | pending_evidence | PIT 可见性、available_at 不可推断、稳定排序、15 分钟固定 Snapshot、HMAC Cursor、key rotation 与错误分类均有明确文字和 NEWS-CTR-001 Cursor Claims Schema；但系统观察时间排序键与 News 领域时间权威定义不一致，且与 NEWS-CTR-001 的 relation_refs 状态表达存在关联缺口 |

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
| QNT-CTR-001 | pending_evidence | Registry Entry 1.0.0 Schema 已有，字段集合覆盖 code/schema_version/value_type/unit/precision/range/algorithm_version/input_requirements/parameters_schema/quality_policy/supports_interval；但缺少可作为首批公开定义的实际 Registry 条目及每个 code 的单位、精度、舍入、输入要求和质量策略映射 |
| QNT-CTR-002 | pending_evidence | 八种 Indicator 参数 Schema 与算法语义已有；但参数 Schema 不等同完整 Indicator Request/Result DTO，输出类型、单位、精度、状态和 Registry 元数据映射未完整定义 |
| QNT-CTR-003 | pending_evidence | 三个首批 Factor 参数 Schema/Golden 和部分 QuantMetric Schema 已有；但完整 Factor Request/Result、交易日历版本引用、输入质量和结果 Envelope 映射不完整 |
| QNT-CTR-004 | pending_evidence | QuantResultRef 责任模块 Schema 的 minItems=1 与 DEC-029 冲突；项目级 DTO/Golden 已允许双重授权空数组。Envelope Schema/Golden 缺失；已有 Factor Metric/Trend/Risk 仅为局部 Payload，Indicator 与其他预留类型尚无完整强类型 Payload，Ref/Envelope/Manifest 映射仍未闭合 |
| QNT-CTR-005 | pending_evidence | 项目级 Manifest Schema/Golden 已区分 Manifest/Payload 版本并允许空数组；责任模块 Build/Verify 仍要求非空 Snapshot，旧文档仍将 Manifest schema_version 解释为 Payload 版本；Record/VerifyPayload JCS 指纹、Manifest/Ref 绑定和 AuthorizationRef 映射仍待权威对齐 |

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
| RSC-CTR-001 | pending_evidence | Evidence 1.0.0 Schema/Golden 已有，字段、来源 Owner、derived_from_refs、content_level、AuthorizationRef、JCS 指纹语义较完整；但 Schema 的 date-time 未约束 RFC3339Nano UTC，source_version 条件必填规则缺少 source-owner/versioned-resource 适用映射，system_as_of/available_at 的 PIT 关系仍由文字约束承担 |
| RSC-CTR-002 | pending_evidence | Event/Claim/Scenario 项目级 DTO、Schema/Golden 已按 DEC-030～033 对齐核心结构；剩余为 EventRef 存在性/无环、source_times 对应、Claim supersedes 当前版本/并发冲突、Scenario Metric Registry 类型/单位映射及 QuantResultRef 类型/时间绑定，并需对齐 Research 责任模块定义 |

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
| FLOW-CTR-001 | pending_evidence | Run/Stage/Attempt 状态枚举、终态和 Template 传播矩阵已有，Run/Stage/Attempt 1.0.0 Schema/Golden 已有；Schema 未完整表达 UTC RFC3339Nano、终态时间/输出字段关系及 ResultSummary 资源绑定，需与 FLOW-CTR-004 收敛 |
| FLOW-CTR-002 | pending_evidence | Cancel/Retry/Deadline 的可观察语义、优先级、停止确认和 retry allowlist 已详细定义；但 Server 命令边界的 CancelRun/RetryStage 字段、幂等作用域/指纹、冲突结果和稳定错误尚无唯一权威命令 DTO Schema/等价定义 |
| FLOW-CTR-003 | pending_evidence | system_as_of/deadline/template/input_snapshot_refs/originating_request_id/correlation_id 字段规则已明确；但 ResearchContext/ContextSectionRef、resolved template snapshot、可信 caller 与各下游 Adapter 之间的 Context 字段映射缺少独立权威 Schema/Port 定义 |
| FLOW-CTR-004 | pending_evidence | Stage Provider Result Envelope 已有字段表，但 output_type/schema_version/payload/quality_summary/diagnostic_refs 缺权威 Schema/Output Registry；ResultSummary Schema 的 quality_status 仍是任意 string，coverage 分母与 summary 资源身份/版本绑定不完整 |

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
| LLM-CTR-001 | pending_evidence | ProviderConfig、CapabilityState、ModelRequest、ModelResponse 1.0.0 Schema 与 JSON Fixtures 已有，协议范围和字段语义详细；但时间 Schema 只使用 date-time、未约束 RFC3339Nano UTC；ContentPart.annotations Schema 为任意值，而 DTO 约定 Annotation[]；部分条件字段/未知类型约束仍不够精确 |
| LLM-CTR-002 | pending_evidence | ModelEvent 1.0.0 Schema 与 SSE Fixture 覆盖完整/中断/terminal/sequence 等主要情形，事件终止和重试规则已明确；但 received_at UTC 约束、sequence_number 初始/合法范围，以及 terminal 后事件的完整公开结构约束未全部落实到机器契约 |
| LLM-CTR-003 | pending_evidence | ModelErrorInfo Schema/映射规则已有，Platform Fault Class 到 ModelErrorKind、stable_code、SafeMessage、Retryable 和 Cause 隔离均有文字定义；但依赖 PLAT-CTR-001 的 Fault 公开分类和稳定 Code 语义尚未冻结 |

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
| NTF-CTR-001 | pending_evidence | Envelope + 三类业务 Payload 共 4 份 Draft 2020-12 Schema、6 个 Golden 已有；fingerprint 算法、event_type/schema_version/source_module 注册键和首批业务事件映射已明确；但 occurred_at/published_at/业务时间均只使用 date-time、未约束 RFC3339Nano UTC，Payload reason_code/opaque ref 格式和 subject_ref 与 payload 主体一致性未完整机器化 |
| NTF-CTR-002 | pending_evidence | Inbox 双唯一键、Publish 幂等键、Delivery 唯一键、Subscription/Channel/Template 快照、Delivery 状态机、unknown 对账和人工重试语义已有详细文字；但 Subscription、DeliveryPlan、DeliveryRecord、DeliveryAttempt、DeliveryStateEvent 缺完整 Schema 或唯一权威跨模块 Application Port 定义 |
| NTF-CTR-003 | not_applicable | ChannelConfigSnapshot、ChannelAdapter、ChannelResolver、RecipientResolver、ChannelSendRequest/Result 是 Notification 内部 Adapter SPI；Server 只提供 Platform Secret/BoundDoer 等组合依赖，不作为独立跨模块业务契约。其内部能力/重试适配不属于 PLAT-002 |

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
| SRV-CTR-001 | pending_evidence | Server Composition Root/统一 HTTP Host 的职责和边界已定义；但 DEC-018/019 退役旧 Analysis API 后，新 Researchflow HTTP 契约没有唯一权威 OpenAPI/Schema 或字段级定义。缺少新 API 路径/方法、CreateRun/Run/Stage/Event 请求响应 DTO、错误投影、认证授权、分页/Cursor 与 WebUI 消费映射 |

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
| INT-CTR-001 | pending_evidence | 三类事件 event_type/schema_version/source_module、触发时点、subject_ref、source_event_ref、idempotency_key、correlation_id、Payload 字段及 JCS fingerprint 输入已明确；Envelope/Payload Schema 与 6 个 Golden 已有；但时间字段 Schema 未约束 RFC3339Nano UTC，subject_ref 与 Payload 主体字段一致性、occurred_at 与来源时间的逐事件绑定、changed_block_reason_codes 字典序规则未完整机器化 |

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

## 24. PLAT-002 十模块契约定义状态总表（2026-11-11）

本表以第3节 43 个正式 CTR 为唯一统计口径。只统计当前跨模块公开契约定义；模块内部 SPI、旧 API 退役别名、UI 页面级项目和实现符合性证据不计入正式 CTR 状态。

| 状态 | 数量 | 含义 |
|---|---:|---|
| freeze-ready | 2 | GOV-CTR-001、GOV-CTR-005；公共规则定义完整，可提交项目负责人确认冻结 |
| pending-evidence | 39 | 公开契约定义/Schema/权威映射仍有缺口 |
| pending-decision | 0 | 本轮五项待决均已由 DEC-PLAT-002-028～032 裁决 |
| blocked | 0 | 当前没有无法通过权威决议消解的公开定义冲突 |
| not_applicable | 2 | DATA-CTR-003、NTF-CTR-003；仅模块内部 Provider/Channel SPI，不属于跨模块公开契约 |
| frozen | 0 | 尚无项目负责人确认冻结的正式 CTR |
| **正式 CTR 合计** | **43** | 不新增编号，不重复计算历史别名 |

### 24.1 五项已裁决内容

五项待决已全部裁决：QNT-DEC-01/02 对应 DEC-PLAT-002-028/029；RSC-OPEN-003/004/005 对应 DEC-PLAT-002-030～032。当前不再存在 pending-decision 项。

### 24.2 已排除或退役范围

- DATA-CTR-003、NTF-CTR-003 为 not_applicable，不要求项目级跨模块冻结；其内部扩展约定归责任模块管理。
- DEC-018/019 退役的旧 Analysis HTTP 契约、CTR-047 及旧 Task/StageEvent HTTP DTO 不计入当前活动 CTR 冻结统计；新 Researchflow HTTP 契约归 SRV-CTR-001 + FLOW-CTR-001～004。
- WebUI CTR-011～049 中页面布局、交互、DOM、产品预留项不是跨模块 API 契约；WebUI 只消费 Server/模块已冻结的公开 HTTP DTO。
- DATA-FLOW-B、LLM-FLOW-B 是条件性 deferred 决策，不是新增正式 CTR，也不计入 43 项状态统计。

### 24.3 阶段结论

- 43 项正式 CTR 已全部完成本轮定义口径核定；不再保留“待审查模块/待全量盘点”的开放任务。
- 当前未完成项为 39 项 pending-evidence；不再有 pending-decision。模块内部实现/测试/CI/部署事项仍不属于本轮契约定义冻结门禁。
- 下一阶段按登记簿中剩余 pending-evidence CTR 增量收敛；GOV-CTR-001 已独立 freeze-ready，Quant/Research 仍按 DEC-PLAT-002-028～033 校准权威 DTO、Schema、Golden 和映射；不重新启动十模块全量扫描，不新增重复 CTR。


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
