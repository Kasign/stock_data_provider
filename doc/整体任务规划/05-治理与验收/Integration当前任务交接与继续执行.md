# Integration 当前任务交接与继续执行

- 仓库：Kasign/stock_data_provider
- 当前工作分支：integration
- 本文更新日期：2026-11-14
- 本文用途：下一轮新聊天的首要接续入口。新聊天先读本文，再核对 GitHub 当前 HEAD/CI；不得直接假设本文 SHA 仍是最新。
- 2026-11-14 联调补记：新增 `internal/server/bootstrap/research_news_mariadb_integration_test.go`，落地第 3 节 A 组跨模块 MariaDB 验收用例与无 DB 自检；提交 `b0aabaa2e` 已推送 integration，Integration CI Run [43561072301](https://github.com/Kasign/stock_data_provider/actions/runs/43561072301) success（head_sha 精确匹配，全部检查项通过），**A 组状态由 `not_comparable` 升级为 `pass`**。本机无 MariaDB/Docker，本地 `t.Skip` 只代表开发态；已用非空假 DSN 在本地验证三个 DB 子测试均进入真实连接阶段而非 Skip，且 CI job env 提供 `SERVER_RELAY_TEST_MYSQL_DSN` 与 mariadb:11.4 service，可确认 CI 上真库执行。公开仓库逐步日志下载需管理员权限（403），故证据组合为「workflow 配置 + DSN 守卫代码 + 精确 SHA success」，不声称逐用例日志已核对。B 组成功写入链路经源码核对确认被 `available_at` 硬阻断，记为 `blocked`。
- 本轮代码验收 HEAD：b0aabaa2edf89c44cd2aeb79b1a57210a8017d1b；Run 43561072301 success。上一轮代码验收 SHA dde7d4f8fb071912ffa34bb9c50d46507d1a388b（Run 43370552552 success）保留为历史记录。
- 代码验收 SHA：b0aabaa2edf89c44cd2aeb79b1a57210a8017d1b；Integration CI Run [43561072301](https://github.com/Kasign/stock_data_provider/actions/runs/43561072301) success，head_sha 精确匹配。
- News→Research 组合代码 SHA：d664b9f6ec08d358b914242735fde52f49e9c7c1；Run 43369529083 success。`dde7d4f` 仅移除一次误加的重复 LLM 传输适配器，当前代码树不包含重复实现。
- News→Research 组合构造提交：06722fdcd2e178c69ee4ae7bb3bf06caf907545d，Run 43368950457 success。
- 启用 News/Research MariaDB 集成测试后，Run 43369220849 对 SHA 570b001 暴露既有 News 集成夹具缺少 NEWS-CTR-002 schema_version；已在 SHA d664b9f 修正，Run 43369529083 success。
- CI 现设置 NEWS_MARIADB_TEST_DSN 与 RESEARCH_TEST_MYSQL_DSN，News 与 Research 模块级 MariaDB 集成测试已进入根模块测试并通过；不代表跨模块 Importer 写入链路已验收。
- 2026-11-14 第 4 项补记：完成 Researchflow Stage/Template/Output Registry 冻结契约与真实代码的**只读盘点**，判定**第 4 项 `blocked`（不是 `fail`）**——Stage 输入/输出、Output Registry 版本与注册项、生产 Provider 三项门禁未通过，仅可信 CallerRef 在契约定义层满足、运行时绑定仍有 L2 缺口。完整证据、责任归属与重新进入条件见本文第 6 节。本轮未新增任何 Stage / StageExecutor / Stage Code / 输出 Schema 代码，未改动模块领域代码，未触发新 CI；阶段判定仍为 INT-B 进行中。

## 1. 当前已完成的工作

### 1.1 GOV-CTR-006 公共运行时指纹验证器

新增 internal/platform/authorization/fingerprint.go：

- 使用 RFC 8785 JCS（github.com/gowebpki/jcs）和 SHA-256。
- 从完整 AuthorizationRef 与 PolicySnapshot 重建 evaluated_policy_versions，不信任调用方提供的 fingerprint_input 或 canonical_json。
- 校验两个原始 JSON 对象的 JCS 输入，拒绝重复 JSON 成员。
- 校验 schema_version=1.0.0、策略快照引用与授权引用版本一致、策略 tuple 非空且严格有序。
- 以常量时间比较验证决策指纹。
- fingerprint_test.go 覆盖 GOV-CTR-006 的 Data、News、Quant、Research 四份 Golden，以及错误指纹、乱序策略和重复 JSON 字段。

### 1.2 News 消费者适配

新增 internal/news/adapters/platform/authorization_verifier.go，实现 News 的窄接口 NewsAuthorizationFingerprintVerifier，委托 Platform 唯一实现，不在 News 复制 JCS。

NEWS-CTR-002 DTO 和 Golden 已补齐 AuthorizationRef.schema_version 与 PolicySnapshot.schema_version，并与 GOV-CTR-006 News Golden 对齐。增加适配器通过 Golden 和篡改指纹的测试。

注意：这只表示 Runtime Verifier 和 News Adapter 代码已实现；不表示 Server 已注入，更不表示受限内容披露已开启。

### 1.3 本轮 CI 验收

Run 43362987745 对 SHA 23d07e7 的 GOV-CTR-006/News 代码全量通过；Run 43364123334 对 SHA 19394e8 的游标密钥配置代码全量通过；本轮 Run 43369529083 对 SHA d664b9f 的全量 CI 及 News/Research MariaDB 集成测试通过；随后 Run 43370552552 对当时的 HEAD dde7d4f 再次全量通过：

- 全仓 Go 格式检查；
- Root Module go vet ./...、go test ./...；
- go-core go vet ./...、go test ./...；
- Compose 配置检查；
- WebUI JavaScript 语法检查、单元测试和 Docker 镜像构建。

此前 Run 43362053690 和 43362555225 曾失败：前者暴露变量遮蔽、gofmt 和 Schema 夹具遗漏；后者修复了编译问题但测试夹具仍缺少 SchemaVersion。上述问题已在 23d07e7 修正，并由 Run 43362987745 的精确 SHA 全量验收通过。不要重复执行或重做这些已通过检查，除非后续改动触及相关代码。

2026-11-14 新增：提交 `b0aabaa2edf89c44cd2aeb79b1a57210a8017d1b`（A 组跨模块 MariaDB 联调验收测试与三份文档同步）触发 Integration CI Run 43561072301 success，head_sha 精确匹配；Checkout、Go 1.23 环境、全仓 gofmt、root/go-core vet 与 test、Compose、WebUI 语法/单测/镜像构建及汇总步骤全部 success。该 Run 是 A 组从 `not_comparable` 升级为 `pass` 的唯一有效证据。

## 2. Server / News / Research 当前代码事实

### 2.1 Server 现状

internal/server/bootstrap/bootstrap.go 当前仍以旧 Analysis Runner、旧 Analysis Task Queue 和旧 Analysis HTTP API 为主要业务运行链。Bootstrap 有共享 DB/Redis、Migration Registry、Lifecycle、Health/Readiness，以及 Research/Researchflow → Notification Outbox Relay。

当前 Bootstrap 已新增 News→Research 组合构造入口，并在 API/Worker 角色共同加载 News Config；当 Key Ring 已配置时，构造 News Evidence Query Facade、Research Repository/Service、Importer/Authorizer。当前仍未将 Importer 接入具体业务消费者，也未装配 Researchflow Scheduler、WorkerPool 和生产 StageExecutor。

不要将 Migration 已注册、模块代码已存在或模块单测通过描述为生产调用链已接通。

### 2.2 News 可复用能力

internal/news/adapters/storage.MySQLRepository 借用 Server 共享 *sql.DB，当前实现了以下接口能力：

- News Evidence PIT Repository；
- 原子授权策略锁定、Observation/Version 复读和精确正文读取；
- AuthorizationSnapshotRepository；
- CurrentNewsAuthorizationPolicyResolver；
- EvidenceSnapshotRepository。

可复用的 Application 构造入口：

- NewEvaluateNewsEvidenceReadAuthorization(verifier, policies)；
- NewGetEvidence(repository, snapshots, authorizer)；
- NewCreateEvidenceSnapshotForQuery(query, repository, newID)；
- NewListEvidenceSnapshotPage(factory, codec)；
- NewListEvidence(pages, records)；
- NewEvidenceQueryFacade(get, list)。

GetEvidence 的原子内容读取必须保留策略当前指针锁、授权复核和精确 Version 正文读取的同一 MariaDB 事务；不得在 Server 重新实现这一领域逻辑。

### 2.3 当前真实装配缺口

完整 ResearchEvidenceQuery 同时要求 GetEvidence 和 ListEvidence。ListEvidence 的生产构造还需要：

- Evidence Snapshot Factory 的安全 ID 生成器；
- EvidenceCursorCodec 的 HMAC Key Ring；
- Active Key ID 和历史 Key；
- Snapshot Page 与 ListEvidence Facade。

EvidenceCursorCodec 已要求 Key Secret 长度 32～256 字节，最多 8 个 Key；游标 TTL 为 15 分钟，轮换时旧 Key 至少保留到其签发的游标过期。News Config 已新增 Active Key ID + JSON Key Ring 的可轮换 Secret 配置解析；Server Bootstrap 已消费该配置，构造 EvidenceCursorCodec 和完整查询 Facade。

不得使用硬编码密钥、空 Key、Mock Facade、永远返回空列表的 ListEvidence，或跳过签名验证的方式绕过该缺口。Bootstrap 组合构造已完成；下一步是以真实 MariaDB 验证分页和旧 Key 轮换期间的验签行为。

### 2.4 Research 已有实现

internal/research/adapters/news/evidence.go 已存在，不要重写：

- Importer：调用 News ResearchEvidenceQuery 获取指定 PIT 版本，校验授权、内容级别和时间字段，再调用 Research AddEvidence；
- Authorizer：Research Evidence 读取/写入时重新向 News Owner 查询当前授权，不把持久化 AuthorizationRef 当作 Bearer Token。

Research Repository 构造入口：

- researchstorage.NewContextRepository(shared.DB, clock)
- researchstorage.NewEvidenceRepository(shared.DB, clock)

Research Application 入口：

- researchapplication.NewService(contexts, evidence, authorizer, clock, ids, contextAuthorizers...)

NewService 的必需依赖是 Context Repository、Evidence Repository、EvidenceAuthorizer；Clock 和 ID Generator 有默认值，ContextAuthorizer 可选但若提供必须非 nil。

### 2.5 News → Research 本轮组合构造

新增 `internal/server/bootstrap/research_news.go`：

- 使用共享 DB 构造 News MySQL Repository；同一 Repository 提供 PIT 查询、授权快照、当前策略解析、事务内当前授权复核/正文读取及 Evidence Snapshot 存储。
- 使用 News Platform Adapter 委托 GOV-CTR-006 公共 Runtime Verifier。
- 构造 GetEvidence、Evidence Snapshot Factory、EvidenceCursorCodec、ListEvidenceSnapshotPage、ListEvidence 和 EvidenceQueryFacade。
- 构造 Research Context/Evidence MariaDB Repository、Research Service、News Authorizer、News Importer。
- Snapshot ID 使用 crypto/rand；随机源失败时返回空 ID，由领域校验拒绝，不降级为可预测 ID。
- Key Ring 未配置时 Bootstrap 不构造分页 Facade，不生成默认密钥；已配置但非法时 News Config 加载/构造失败关闭。
- Bootstrap 在 API/Worker 两种角色均加载 News Config，并在 Key Ring 配置后执行真实依赖构造。

重要边界：该组合目前尚无 Researchflow Stage/HTTP 业务消费者；构造成功只证明构造函数和依赖图可创建，不代表 News Evidence 已被业务入口消费或持久化。真实 MariaDB 的授权读取、分页、Key 轮换与 Importer fail-closed 链路已由 Run 43561072301 验收通过；Research Evidence 成功写入因 `available_at` 硬阻断仍为 `blocked`。

### 2.6 Researchflow 当前边界

Researchflow 已有 Scheduler、Worker、WorkerPool、Claim/Lease/Fencing、候选扫描与 Finalizer 等模块内设施；但尚未形成 Server 生产 StageExecutor 组合。Quant 也不能直接作为生产 Stage Provider：Indicator/Factor/Trend 的公开 Application Port、Payload Registry 和冻结契约门禁仍有缺口。

因此当前最合理的先后顺序是先完成 News/Research 生产依赖的可验证装配准备，再按真实 Stage 定义、Output Registry 和生产 Application Port 接入 Researchflow。不得虚构 Stage Code、输出 Schema 或 Quant 计算入口。

## 3. 下一轮明确任务

按顺序推进，一次只处理一条真实依赖链：

1. News Cursor Key Ring 配置已完成，Run 43364123334 对 SHA 19394e8 success；Bootstrap 已消费配置并构造 EvidenceCursorCodec，部署轮换时旧 Key 至少保留游标 TTL 15 分钟。
2. News→Research 组合构造代码及 Bootstrap 构造测试已提交；Run 43368950457 对构造代码 SHA 06722fd success。启用 MariaDB 集成测试暴露了既有 News 集成夹具缺少 schema_version，已在 SHA d664b9f 修正，Run 43369529083 success。
3. 真实 News→Research 跨模块 MariaDB 端到端验收，经源码核对后按 `available_at` 硬阻断拆为 A/B 两组：
   - **A 组（已完成，`pass`）**：授权快照读取、当前策略复核、PIT 不可见、指纹篡改、当前策略版本变更、Context 取消、分页快照逐条授权、游标旧 Key 轮换验签，以及 Importer fail-closed 零落库。实现见 `internal/server/bootstrap/research_news_mariadb_integration_test.go`：复用 `SERVER_RELAY_TEST_MYSQL_DSN` 自建隔离 database、注册 news/research 两个 Migration namespace、调用真实 `newResearchNewsComposition`、读取冻结 NEWS-CTR-002 Golden 并用公共 GOV-CTR-006 `Fingerprint` 重算决策指纹；不使用 Mock Facade、空壳 DB、硬编码密钥或伪造时间字段。验收证据为提交 `b0aabaa2edf89c44cd2aeb79b1a57210a8017d1b` 的 Integration CI Run 43561072301 success（gofmt、root/go-core vet 与 test、Compose、WebUI 检查与构建全部通过）。本地无 DSN 时 `t.Skip` 仅为开发态；已用非空假 DSN 确认 DB 子测试不会 Skip，CI 提供同名 env 与 MariaDB service，故该 success 包含真实 MariaDB 执行。不重复执行。
   - **B 组（`blocked`）**：Research Evidence 成功持久化、AuthorizationRef 与派生关系留痕。阻断依据：`internal/news/app/get_evidence.go` 的 `researchEvidenceRecordFromPIT` 不设置 `AvailableAt`（第 212 行注释明确需独立来源证据，禁止以 observed_at/published_at 推导）；`internal/research/adapters/news/evidence.go` 的 `mapNewsEvidence` 第 152 行要求非 nil，否则返回 `ErrEvidenceNotPITSafe` 且不调用 `AddEvidence`；GOV-CTR-002 与《项目级契约决策记录》第 293/294/452 条要求缺证据时为 null，`doc/news/新闻API契约.md` 第 197 行明确 News 当前不推断 `available_at`。责任归属 News Owner；Integration 不得伪造、回填或改 News 领域代码绕过。现有 `TestImporterRejectsMissingOrFutureAvailableAtWithoutPersisting` 只证明 fail-closed 语义，Importer 成功路径单测靠 mock 注入 `AvailableAt`，不是跨模块证据。
   - 现有 News/Research **模块级** MariaDB 集成测试已由 Run 43369529083 执行并通过；不得将其等同跨模块链路验收，也不重复执行。
4. Researchflow Stage 接入（**只读盘点已完成，接入判定 `blocked`**）：已按第 4 节执行规则核对 PLAT-002 登记簿 `0.2.60-frozen`、FLOW-CTR-001～004、Researchflow 4 份 Schema + 11 份 Golden、`04-ApplicationPort与运行时契约落地补充.md`，以及 `internal/researchflow/`、`internal/server/bootstrap/` 真实代码。四项门禁判定为「Stage 输入/输出：部分满足；Registry 版本：部分满足；可信 CallerRef：契约定义层满足、运行时绑定有 L2 缺口；生产 Provider：未满足」，**不具备接入条件**。不创建空壳 StageExecutor、不虚构 Stage Code 或输出 Schema、不把 Quant 当生产 Provider。完整证据、责任归属与重新进入条件见本文第 6 节；在此项解锁前不重复盘点。
5. Quant（**`blocked`，本轮复核确认仍禁止接入**）：保持生产 Stage 禁止接入，直到有可注入的公开 Application Port、Result Payload/Registry 注册项，且 `DATA-CTR-005` / `DATA-CTR-007` / `QNT-CTR-003` 按登记簿重新进入条件完成定点复核。当前 `internal/quant/application/` 只有 `GetResult` 授权读取用例，Indicator/Factor/Trend 仍是纯函数；`internal/quant/result/envelope.go:201-221` 的 `DecodePayload` 只接受 `trend/1.0.0` 与 `risk/1.0.0`，`indicator`/`factor` fail-closed。解锁条件见本文第 6.5 节第 2 条；Integration 不得自行串接纯函数造伪 Application Facade。
6. 复核 LLM Provider 与 Notification Delivery 的生产装配条件；已有 Outbox→Inbox/FanoutWorkItem MariaDB 验收不重复执行。只推进存在真实契约与可运行依赖的缺口。

## 4. 执行规则

- 只在用户授权的 integration 分支继续；本轮后续不要擅自切换分支或执行分支合并。
- 每轮开始先通过 GitHub API 核对 integration 最新 HEAD、Actions Run 和代码树；如果 HEAD 已变化，以新 HEAD 为准。
- 先读 doc/contract/PLAT-002-项目级契约冻结登记簿.md 和相关冻结契约，再读模块专项文档和实际代码。
- 不重复已通过的 CI/模块验收；只有改动触及的范围或新的失败证据才重新验证。
- 不修改其他模块领域代码来替代其 Owner 的契约/实现工作；Integration 可以做组合根装配和跨模块适配，但不得篡改模块所有权。
- 中文文档、注释和提交信息；按功能聚合提交，不为每个小改动单独提交。
- 不把 skipped、not_comparable、静态代码存在或构造测试通过说成真实生产可用。
- 每完成一阶段同步更新本文及对应 doc/<module>/README.md，让下一次新聊天能从文档继续。

## 5. 下一次新聊天的起始操作

1. 打开本文。
2. 查询 integration 最新 HEAD 和最新 CI（不要复用本文 SHA 作为假定当前值）。
3. 查看 doc/contract/PLAT-002-项目级契约冻结登记簿.md、GOV-CTR-006、doc/news/README.md、doc/research/README.md、doc/researchflow/README.md、doc/server/README.md；处理第 4 项时另读 FLOW-CTR-001～004 登记行、doc/researchflow/04-ApplicationPort与运行时契约落地补充.md 与本文第 6 节。
4. 从**第 3 节第 6 项「复核 LLM Provider 与 Notification Delivery 的生产装配条件」**继续（一次只处理一条真实依赖链）：第 1、2 项与第 3 项 A 组已完成，A 组由 Run 43561072301 对 SHA `b0aabaa2e` 的精确 success 验收；第 4 项 Researchflow Stage 接入与第 5 项 Quant 生产接入均为 `blocked`，解锁条件见本文第 6 节，在解锁前**不要重复盘点或空转**。若发现新 HEAD 已包含对应工作，先核对证据再推进，不重复开发。B 组在 News Owner 补齐 `available_at` 来源证据前保持 `blocked`。

## 6. 2026-11-14 第 4 项 Researchflow Stage 契约准入盘点（只读，结论 `blocked`）

### 6.1 盘点基线与范围

- 核对时 `integration` 本地与远端 HEAD 均为 `c43fb88f7`（2026-11-14 docs-only 提交，按 CI paths 不触发 CI）；最近**代码** SHA 为 `b0aabaa2e`，Integration CI Run [43561072301](https://github.com/Kasign/stock_data_provider/actions/runs/43561072301) success。本轮是纯文档盘点，未改动任何 `.go` 文件，其结论以 docs-only 提交落地，**不改变代码验收 SHA**。若后续 HEAD 因文档提交前移，按第 4 节规则以查询到的最新 HEAD/CI 为准，不要用本文记录的反推当前值。
- 契约权威来源：`doc/contract/PLAT-002-项目级契约冻结登记簿.md`，版本 `0.2.60-frozen`，阶段 `L1_contracts_frozen`，正式 CTR 43 项（`frozen` 37 / `deferred` 4 / `not_applicable` 2），固定代码审计基线 `origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`。**该基线是 L1 契约冻结时的代码审计点，不是当前 HEAD**；L1 冻结只代表公开契约定义已冻结，不代表 L2 实现符合性或 L3 集成验收通过。
- Researchflow 的正式契约编号是 **FLOW-CTR-001～004**（登记簿第 55～58 行）。全仓**未找到** `RSF-CTR-*` / `RESEARCHFLOW-CTR-*` 命名的契约，也**未找到**独立的 Stage Registry / Output Registry / Payload Registry 冻结契约文件、冻结的 Stage Code 清单或逐 Stage 输入/输出 Schema。
- 已有机器可读资产：`doc/researchflow/schema/` 4 份 JSON Schema（run / stage / attempt / result-summary，均 1.0.0）与 `doc/researchflow/golden/` 11 份 Golden；文字契约见 `doc/researchflow/03-CONTRACT-006与007最终决议.md` 与 `04-ApplicationPort与运行时契约落地补充.md`。
- `doc/server/README.md` 第 12 章已有 Server 侧门禁核查（约第 1697～1912 行），本轮**不重复其结论**，只把 Integration 侧准入判定与下一轮起点固化到本文。

### 6.2 四项门禁逐条判定

| 门禁 | 判定 | 关键证据 |
|---|---|---|
| Stage 输入/输出 | **部分满足** | 已具备：`FLOW-CTR-001` 冻结 Run/Stage/Attempt 1.0.0 Schema/Golden；`03-CONTRACT-006与007最终决议.md:159-171` 有 Stage Provider Result Envelope **字段表**；`04-ApplicationPort与运行时契约落地补充.md:53` 要求强类型 payload。缺口：登记簿第 58 行「只有字段表，**无权威 Schema/Output Registry**，Stage Provider Result Envelope Schema 未登记」；`doc/researchflow/schema/stage-1.0.0.schema.json:7-25` 只有不透明 `output_ref`，无 input / payload / output_type；`04-...补充.md:101`「Output Registry 目前只固定版本引用，尚未验证 Provider 的 output_type/schema_version」 |
| Registry 版本 | **部分满足** | 已具备：Template Registry 精确 `Ref + Version` 查询、禁止 latest / 别名回退、指纹复算、insert-only 发布（`04-...补充.md:428-441`）；`WorkflowTemplate` 保留 `output_registry_version` 字段。缺口：`doc/server/README.md:1737/1805`「未发现可供 Server 装配的完整 Researchflow Output Registry 实例/注册清单」；`internal/researchflow/template/template.go:58` 只有字符串字段，无版本常量、无注册函数、无条目；代码中的 `outputs-v1/v3/v7` 全是测试夹具，不是冻结版本 |
| 可信 CallerRef | **契约定义层满足，运行时绑定有 L2 缺口** | 已具备：`项目级契约决策记录.md:559` 定义 MVP 直接调用主体为 Researchflow service Principal，Golden 值 `service:researchflow`；`NEWS-CTR-002` Golden 第 3 行 `caller_ref` 与第 51 行 `subject_ref=caller_ref`；`04-...补充.md:24`、`新闻来源授权矩阵.md:121` 要求由可信认证 Context 注入、禁止请求体自报；多处 fail-closed 校验（`internal/researchflow/application/create_run.go:107-110`、`cancel_run.go:68-77`、`internal/news/domain/authorization.go:167`）。缺口：登记簿第 44 行「远程可信 caller 传递机制未定义」、第 57 行「可信 caller/correlation 与各下游 Adapter 字段映射未形成单一权威 Schema/Port 契约」；生产代码中 `service:researchflow` **零出现**，仅见于测试常量与 Golden，不是配置项 |
| 生产 Provider | **未满足** | `internal/researchflow/application/executor.go:47-49` 的 `StageExecutor` **只有接口**，全仓生产实现为零（`ExecuteStage` 实现仅 `worker_test.go` 三个 fake）；`internal/researchflow/resultvalidation/quant.go:70-97` 的 Registry 生产注册表为空；`internal/quant/result/envelope.go:201-221` 的 `DecodePayload` 只接受 `trend/1.0.0`、`risk/1.0.0`，`indicator`/`factor` fail-closed；`internal/server/bootstrap/bootstrap.go:176-183/233-235/432` 只装配 News→Research 组合、Outbox Relay 与 Migration，无任何 Scheduler / Worker / WorkerPool / StageExecutor 生产构造；`doc/researchflow/README.md:32` 明确 StageExecutor 是注入端口、生产 Adapter 未闭环 |

### 6.3 真实代码事实（避免下次重新盘一遍）

- `internal/researchflow/` 有 11 个子包，**没有** `stage` / `executor` / `registry` / `output` 包；`StageExecutor` 及 `StageExecutionRequest/Result` 定义在 `application/executor.go:17-49`，`ValidateStageExecutionResult` 在同文件第 56 行。**未找到** `StageInput` / `StageOutput` / `StageProvider` 类型。
- `StageDefinition`（`template/template.go:42-51`）只有 `stage_code / required / dependencies / priority / applicability / retry_policy / propagation / gate_type`，没有输入或输出 Schema 绑定。
- `internal/server/bootstrap/router.go:22-34` 只有 `healthz/readyz/diagnostics` 与旧 `/api/v1/analysis/*`，**无 researchflow 业务路由**。
- 已完成的跨模块链路仅 `Researchflow 终态 Outbox → Notification Inbox/FanoutWorkItem`（`bootstrap.go:323-347`、`internal/server/relay/researchflow_notification.go:211-239`），**不创建或执行 Run**。
- `internal/researchflow/` 有 55 个测试文件，Stage 执行链覆盖全为模块单测 + fake；**不存在** `CreateRun → Schedule → Worker → StageExecutor → CommitAttemptResult` 的跨模块 MariaDB 端到端测试。
- Quant 侧 `internal/quant/application/` 只有 `GetResult` 授权读取用例，Indicator/Factor/Trend 是纯函数，没有可注入的计算 Application Port；`doc/quant/README.md:34` 明确 `indicator`/`factor` 未注册。

### 6.4 `deferred` 契约与阻断范围

登记簿第 75～78 行的 4 项 `deferred`：`DATA-CTR-005`（DailyBars Decimal v2）、`DATA-CTR-007`（Snapshot 内容寻址与 Evidence locator）、`QNT-CTR-003`（首批 Factor）、`SRV-CTR-001`（新 Researchflow HTTP API）。另有 2 项 `not_applicable`（`DATA-CTR-003`、`NTF-CTR-003`），不阻断任何接入。

- 按登记簿第 15 行「deferred 不得作为已发布接口使用」，前三项**直接阻断 Quant 作为生产 Stage Provider**，`SRV-CTR-001` **直接阻断新 Researchflow HTTP API**；二者都不阻断 Researchflow 自身的 Stage 定义与编排层工作。
- 但真正卡住第 4 项的是 **frozen 契约的 L2 实现缺口**（FLOW-CTR-004 无 Output Registry / Envelope Schema），不是 deferred 状态本身。按 `doc/server/README.md:1745`「在 Output Registry 权威版本、具体 Stage 清单和各 Stage 输出 Schema 尚未逐项对齐前，不注册推测性的 Stage，不启动可接收生产 Run 的 Worker」。

### 6.5 结论、责任归属与重新进入条件

**结论：第 3 节第 4 项接入判定为 `blocked`（不是 `fail`），阶段仍为 INT-B 进行中，系统整体未达到 INT-C 端到端业务联调条件。** 本轮不产生任何 Stage 相关代码。

按「Owner 先行、Integration 收尾」的顺序，解锁需同时具备以下可验收条件：

1. **Researchflow / 契约 Owner**：冻结具体 `stage_code + template_version + output_registry_version` 三元组；发布 Output Registry 版本号常量、`(output_type, schema_version)` 注册项与对应 Schema + Go Validator；补齐 Stage Provider Result Envelope Schema 与逐 Stage 输入/输出 Schema（当前只有字段表）。
2. **各业务模块 Owner**：提供公开 Application Port；Quant 需可注入的计算 Facade 并在 Envelope Registry 注册 `indicator`/`factor`；`DATA-CTR-005/007` 与 `QNT-CTR-003` 按登记簿重新进入条件完成定点复核后才可解除 deferred。
3. **Researchflow / 模块边界**：提供生产 `StageExecutor` 实现（Server 只注入，不复制领域逻辑），并明确可信 CallerRef 运行时绑定（远程可信 caller 传递机制、下游 Adapter 字段映射的单一权威契约）。
4. **验收证据**：新增 `CreateRun → Schedule → Worker → StageExecutor → CommitAttemptResult` 的跨模块 Contract Test 与 MariaDB 端到端验收，并在精确 SHA 上取得 CI success。

### 6.6 Integration 边界：现在能做什么、不能做什么

- **解锁后可以做（组合根职责）**：用现有 `ResearchflowCapacity`（`internal/server/config/researchflow.go:14-18`）把已存在的 `NewMySQLRunRepository`、`NewMySQLTemplateRegistry`、`NewScheduler`、`NewWorker`、`NewWorkerPool`、`NewSchedulerCandidateScanner` 装配进 `runtime.Lifecycle`；补齐 Server 侧基础设施 Adapter（可信身份 `RequestIdentity`、`DatabaseClock`、`IDGenerator`）；Migration 已在 `bootstrap.go:432` 注册；保持 Outbox Relay 与 Health/Readiness 不变。
- **任何情况下不能做**：凭空造生产 `StageExecutor`、虚构 Stage Code 或输出 Schema、把 Quant 纯函数串成伪 Application Facade、绕过 `DATA-CTR-005/007` 门禁、用空实现冒充生产实现、把构造成功或模块单测说成端到端业务链路通过。

### 6.7 不要重复执行的事项

- `doc/server/README.md` 第 12 章 Server 侧门禁核查已存在，不重写。
- Researchflow 模块级 gofmt / test / race / vet 与 MariaDB 普通/Race 集成已通过（Run `43008530832`、PR Run `43008538854`，见 `doc/researchflow/02-任务拆分与验收.md:23`），不重跑。
- Outbox→Notification MariaDB 验收（Run `43359166036`）与第 3 项 A 组（Run `43561072301`）不重复。
- 第 4 项在 6.5 的解锁条件满足前不重复盘点；若新 HEAD 已包含对应工作，先核对证据再推进。
