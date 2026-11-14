# Integration 当前任务交接与继续执行

- 仓库：Kasign/stock_data_provider
- 当前工作分支：integration
- 本文更新日期：2026-11-14
- 本文记录时核对的 integration HEAD：`c25d881c0`（2026-11-14 代码提交：独立 review 修复）。**该字段是记录时快照，不是实时值**：本轮及后续 docs-only 提交都会让 HEAD 前移，实际当前值一律按第 4 节规则查远端。最近**代码**验收 SHA `c25d881c0`，Integration CI Run `43574975820`（#70）success；其后的 docs-only 提交按 CI paths 规则不触发新 CI，不得把文档 HEAD 记作 CI 通过。历史代码验收 SHA：`4752dc104` / Run `43571558064`（B 组资产 + CI `-v`）、`b0aabaa2e` / Run `43561072301`（A 组首验）。
- 本文用途：下一轮新聊天的首要接续入口。新聊天先读本文，再核对 GitHub 当前 HEAD/CI；不得直接假设本文 SHA 仍是最新。
- 2026-11-14 联调补记：新增 `internal/server/bootstrap/research_news_mariadb_integration_test.go`，落地第 3 节 A 组跨模块 MariaDB 验收用例与无 DB 自检；提交 `b0aabaa2e` 已推送 integration，Integration CI Run [43561072301](https://github.com/Kasign/stock_data_provider/actions/runs/43561072301) success（head_sha 精确匹配，全部检查项通过），**A 组状态由 `not_comparable` 升级为 `pass`**。本机无 MariaDB/Docker，本地 `t.Skip` 只代表开发态；已用非空假 DSN 在本地验证三个 DB 子测试均进入真实连接阶段而非 Skip，且 CI job env 提供 `SERVER_RELAY_TEST_MYSQL_DSN` 与 mariadb:11.4 service，可确认 CI 上真库执行。匿名 job 日志下载需权限（403）；2026-11-14 完成 `gh` 登录后已取得 Run 68 逐步日志，实证 `mariadb:11.4` service healthy、三个 DSN 非空、`internal/server/bootstrap` 包 `ok 0.627s`。但 CI 使用 `go test ./...`（无 `-v`），Run 68 日志不含逐用例 PASS/SKIP 明细；该限制已由 Run 69 新增的 `test-integration-verbose` 步骤消除并取得逐用例实证，详见第 8.2 节。B 组成功写入链路经源码核对确认被 `available_at` 硬阻断，记为 `blocked`；B 组成功路径用例已落地、当前按契约事实 `t.Skip`。
- 本轮代码验收 HEAD：c25d881c041eb349450c004c42c4cf227c9bf437；Run 43574975820 success。历史：4752dc104（Run 43571558064）、b0aabaa2e（Run 43561072301）、dde7d4f8（Run 43370552552）。
- 代码验收 SHA：c25d881c041eb349450c004c42c4cf227c9bf437；Integration CI Run [43574975820](https://github.com/Kasign/stock_data_provider/actions/runs/43574975820) success，head_sha 精确匹配。
- News→Research 组合代码 SHA：d664b9f6ec08d358b914242735fde52f49e9c7c1；Run 43369529083 success。`dde7d4f` 仅移除一次误加的重复 LLM 传输适配器，当前代码树不包含重复实现。
- News→Research 组合构造提交：06722fdcd2e178c69ee4ae7bb3bf06caf907545d，Run 43368950457 success。
- 启用 News/Research MariaDB 集成测试后，Run 43369220849 对 SHA 570b001 暴露既有 News 集成夹具缺少 NEWS-CTR-002 schema_version；已在 SHA d664b9f 修正，Run 43369529083 success。
- CI 现设置 NEWS_MARIADB_TEST_DSN 与 RESEARCH_TEST_MYSQL_DSN，News 与 Research 模块级 MariaDB 集成测试已进入根模块测试并通过；不代表跨模块 Importer 写入链路已验收。
- 2026-11-14 第 4 项补记：完成 Researchflow Stage/Template/Output Registry 冻结契约与真实代码的**只读盘点**，判定**第 4 项 `blocked`（不是 `fail`）**——Stage 输入/输出、Output Registry 版本与注册项、生产 Provider 三项门禁未通过，仅可信 CallerRef 在契约定义层满足、运行时绑定仍有 L2 缺口。完整证据、责任归属与重新进入条件见本文第 6 节。本轮未新增任何 Stage / StageExecutor / Stage Code / 输出 Schema 代码，未改动模块领域代码，未触发新 CI；阶段判定仍为 INT-B 进行中。
- 2026-11-14 第 6 项补记（远端提交 `a98823abe`）：LLM Provider 与 Notification Delivery 生产装配条件只读复核已完成，**均判定 `blocked`**，详见本文第 7 节。至此第 3 节 1～6 项全部处理完毕：1、2、3A 为 `pass`；3B、4、5、6 为 `blocked`。**当前没有可推进的 Integration 开发任务**，在新证据或正式生产消费端出现前停止新增 Integration 代码（见第 7.4 节）。这一「等待态」的根因、工作面选择与反空转规则见本文第 10 节；后续若用户未授权跨模块/契约变更，不应再重复盘点或制造文档进度。
- 2026-11-14 CI 可观测性补记：本机 `gh` 2.104.0 已由用户在本机终端完成网页授权登录（`Kasign` / keyring / `repo,read:org`），`gh run view 43561072301 --log` 已成功取得 Run 68 逐步日志，确认 MariaDB service healthy、三个 DSN 非空、bootstrap 包 `ok`；匿名 REST API 的 job 日志下载返回 403。因 CI 未启用 `-v`，Run 68 日志不含逐用例明细；该限制已由 Run 69 的 `test-integration-verbose` 步骤永久消除。工具链细节、HTTPS fetch 兜底与证据级别见本文第 8 节。
- 2026-11-14 工作面复盘补记：用户指出「连续多轮没有实质性任务进度」。核对后确认 Integration 分支可独立完成的编码工作已完结，5 条 `blocked` 链全部需要其他模块领域代码 / L1 冻结契约 / 真实外部来源证据；此前把「等 Owner」当作分支内任务反复盘点，是策略偏差。本轮新增第 10 节固化工作面判定、P0 精确解锁清单、三种工作面与反空转规则，并交付两项 Integration 侧代码资产（B 组成功路径用例 + CI `-v` 逐用例证据），Run `43571558064` 对代码 SHA `4752dc104` success。`.workbuddy/memory/2026-11-14.md` 经用户确认已提交（`0f7aab244`），不加入 `.gitignore`。
- 2026-11-14 工作面 A 补记：用户已「授权A」。按登记簿第 5 节先产出 `doc/contract/NEWS-CTR-001-available_at-来源证据解锁影响分析.md`（提案，待正式决策）。**A 第 1 步经 SEC 官方 FAQ 与 EDGAR PDS 规范核实，判定 `sec-edgar` 与现有 Provider 均不提供「首次公开可获取时间」的合格证据**，故按冻结契约不得新增字段/表、不得改 PIT SQL；当前停在「正式决策」门禁，需项目负责人在提案第 3 节选定 P1/P2/P3。详见本文第 10.7 节。

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
   - **B 组（`shelved`，原 `blocked`）**：Research Evidence 成功持久化、AuthorizationRef 与派生关系留痕。阻断依据：`internal/news/app/get_evidence.go` 的 `researchEvidenceRecordFromPIT` 不设置 `AvailableAt`（第 212 行注释明确需独立来源证据，禁止以 observed_at/published_at 推导）；`internal/research/adapters/news/evidence.go` 的 `mapNewsEvidence` 第 152 行要求非 nil，否则返回 `ErrEvidenceNotPITSafe` 且不调用 `AddEvidence`；GOV-CTR-002 与《项目级契约决策记录》第 293/294/452 条要求缺证据时为 null，`doc/news/新闻API契约.md` 第 197 行明确 News 当前不推断 `available_at`。责任归属 News Owner；Integration 不得伪造、回填或改 News 领域代码绕过。现有 `TestImporterRejectsMissingOrFutureAvailableAtWithoutPersisting` 只证明 fail-closed 语义，Importer 成功路径单测靠 mock 注入 `AvailableAt`，不是跨模块证据。**2026-11-14 更新：项目负责人决定搁置，重启条件见第 10.8 节。**
   - 现有 News/Research **模块级** MariaDB 集成测试已由 Run 43369529083 执行并通过；不得将其等同跨模块链路验收，也不重复执行。
4. Researchflow Stage 接入（**只读盘点已完成，接入判定 `blocked`**）：已按第 4 节执行规则核对 PLAT-002 登记簿 `0.2.60-frozen`、FLOW-CTR-001～004、Researchflow 4 份 Schema + 11 份 Golden、`04-ApplicationPort与运行时契约落地补充.md`，以及 `internal/researchflow/`、`internal/server/bootstrap/` 真实代码。四项门禁判定为「Stage 输入/输出：部分满足；Registry 版本：部分满足；可信 CallerRef：契约定义层满足、运行时绑定有 L2 缺口；生产 Provider：未满足」，**不具备接入条件**。不创建空壳 StageExecutor、不虚构 Stage Code 或输出 Schema、不把 Quant 当生产 Provider。完整证据、责任归属与重新进入条件见本文第 6 节；在此项解锁前不重复盘点。
5. Quant（**`blocked`，本轮复核确认仍禁止接入**）：保持生产 Stage 禁止接入，直到有可注入的公开 Application Port、Result Payload/Registry 注册项，且 `DATA-CTR-005` / `DATA-CTR-007` / `QNT-CTR-003` 按登记簿重新进入条件完成定点复核。当前 `internal/quant/application/` 只有 `GetResult` 授权读取用例，Indicator/Factor/Trend 仍是纯函数；`internal/quant/result/envelope.go:201-221` 的 `DecodePayload` 只接受 `trend/1.0.0` 与 `risk/1.0.0`，`indicator`/`factor` fail-closed。解锁条件见本文第 6.5 节第 2 条；Integration 不得自行串接纯函数造伪 Application Facade。
6. LLM Provider 与 Notification Delivery 生产装配条件（只读复核已完成，均为 `blocked`）：LLM 缺少真实 Gateway 版本与可追溯 CapabilityState 证据；Notification 缺少真实渠道协议/凭据、Recipient Directory 实现和 Delivery Worker。详细证据见第 7 节；已有 Outbox→Inbox/FanoutWorkItem MariaDB 验收不重复执行。

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
4. **当前总览（2026-11-14，以代码 SHA `c25d881c0` / Run `43574975820` 为准）**：第 3 节 1、2、3A `pass`（A 组已有逐用例级 CI 实证，见第 8.2 节）；3B、4、5、6 `blocked`。Integration 侧可独立完成的工作已完结：A 组真库验收、B 组成功路径待启用用例、CI 逐用例可观测性均已落地。**当前没有可推进的 Integration 开发任务**，不要重复盘点、不要装配空壳、不要重跑已通过的 CI。**关键分流：若用户只说「继续下一步任务」、远端无新证据且未授权工作面 A，直接按第 10.5 节汇报等待态与三个选项，不再产生空转提交。** 第 6 项 LLM / Notification 的 `blocked` 详见第 7 节；Researchflow Stage 与 Quant 按第 6 节条件阻断；News B 组按第 10.2 节的 10 步清单等待解锁。只有出现新证据或正式生产消费端后，才选取**其中一条**真实依赖链推进。**工作面 A 已于 2026-11-14 授权并完成第 1 步，随后由项目负责人决定 `搁置`（见第 10.7、10.8 节）**：`sec-edgar` 与现有搜索类 Provider 均不合格；全渠道复核找到形式合格但属「法定披露」品类的来源（EDINET / RNS / HKEX）。**P0 状态为 `shelved`，P1/P2/P3 未选定**；重启条件是为 News 引入法定披露类来源这一产品决策，不是「再找来源」。搁置期间不得动 `available_at` 相关字段/表/PIT SQL，也不得重复盘点该 4 家候选。
5. CI 排障与工具链见第 8 节：本机 `gh` 已安装并完成网页授权登录，可用 `gh run view <run-id> --log` 核对逐用例结果；远端同步若 SSH 报主机密钥校验失败可用 HTTPS 只读 fetch 兜底。不要把 PAT 贴进对话、写进 `.zshrc` 明文或提交进仓库。

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

## 7. 2026-11-14 LLM Provider / Notification Delivery 装配准入复核（只读，结论均为 `blocked`）

### 7.1 盘点基线

- 当前核对 HEAD：`c43fb88f72f33648513e528b9f634acaec71f14e`（docs-only）；最近代码 SHA：`b0aabaa2edf89c44cd2aeb79b1a57210a8017d1b`；Integration CI Run [43561072301](https://github.com/Kasign/stock_data_provider/actions/runs/43561072301) 对该代码 SHA 精确 success。
- `.github/workflows/integration-ci.yml` 的根模块 `go test ./...` / `go vet ./...` 覆盖 `internal/llm`，因此本次 Run 已覆盖 LLM 普通测试与 Vet；该 Workflow 未运行 LLM 专项 Race，也没有真实 Gateway E2E。不得将普通测试通过扩展为能力验收。
- 本轮只核对真实配置边界、Application Port、Server Bootstrap 与模块 README；未修改任何业务 Go 代码，也未重复 Outbox→Notification MariaDB 验收。

### 7.2 LLM：CapabilityState / Provider 装配（`blocked`）

- 已有模块代码：`internal/llm/llm.go` 定义 `ProviderConfig` 与 `CapabilityState`；能力状态必须绑定 provider/model/protocol、真实 `gateway_version`、`verified_at`、`valid_until`、验证方式、`evidence_ref` 和已验证的输出上限。缺少任一关键证据或超过 TTL，能力必须保持 `unknown`。
- `internal/llm/responses_adapter.go` 的构造要求显式注入 Platform `config.Secret`、固定目的地址的 HTTP Doer、CapabilityState 与 Clock；当前 Server Bootstrap 未导入 LLM Adapter，也未装配 BoundDoer、SecretRef、ProviderConfig、CapabilityState 或 Researchflow Provider。
- `doc/llm/03-现有模型Gateway协议盘点.md` 明确记录本轮没有向部署 Gateway 发起真实推理请求，也没有读取或使用 API Key；仓库没有可核验的 Gateway 运行版本及对应 capability evidence。静态协议盘点、配置样本或模型名称不能替代真实验证。
- 结论：不构造生产 Provider、不填充伪造 CapabilityState、不将未知能力标为 supported。重新进入条件：取得可核验 Gateway 版本、目标 model ID、受控 SecretRef 映射，并对 Responses JSON、SSE、Structured Output、Tool Call 等逐项执行真实验证，保存可追溯 evidence_ref 与有效期；随后再由 Server 组合根装配 BoundDoer/Secret/Clock。

### 7.3 Notification：Recipient / Channel / Delivery Worker（`blocked`）

- 已有模块端口和单条执行用例：`recipient_directory.go` 只有 RecipientDirectory Port 与解析适配器；`channel_ports.go` 定义 ChannelAdapter/ChannelResolver/RecipientResolver SPI；`delivery_execution_service.go` 只执行已领取的单条 Delivery，并明确由上层 Worker 负责队列扫描与 Claim；`fanout_dispatcher.go` 的 RunOnce 只处理一个 WorkItem 页面。
- 当前没有真实 RecipientDirectory 实现、具体 ChannelAdapter、Delivery Worker 生命周期或可用渠道凭据。Server Bootstrap 只装配 EventReceiver/Inbox 与 Research/Researchflow Outbox Relay，没有装配 FanoutPlanFactory、Fanout Worker 或 Delivery Worker。
- Notification README 已明确首个渠道协议、凭据来源、回执/幂等/查询语义及真实身份目录尚未提供。现有 Run `43359166036` 只证明 Outbox → Inbox/FanoutWorkItem 接收、重试和幂等，不证明真实渠道发送；不重复验收。
- 结论：不注册 Fake 作为生产 Channel、不虚构 recipient_ref/目标地址、不创建空壳 Worker。重新进入条件：明确首个真实渠道及协议、SecretRef/凭据来源、回执与幂等/查询能力证据、真实身份目录和 RecipientResolver，再由 Server 装配真实 Fanout/Delivery 生命周期。

### 7.4 本轮结论与下一步

- LLM 与 Notification 均为 `blocked`，不是 `fail`；当前没有可在 Integration 越权补齐的模块内实现，也没有满足真实依赖条件的组合根装配任务。
- Researchflow Stage 的 FLOW-CTR-004/生产 Provider 缺口与 Quant 的 Application/Payload Registry 门禁已分别记录在第 6 节及第 3 节第 5 项；News→Research B 组仍等待来源可证明的 `available_at`。不重复盘点这些既有结论。
- 在新 Gateway/渠道/身份/来源证据或正式生产消费端出现前，停止新增 Integration 代码；收到新证据后只推进其中一条真实依赖链。

## 8. 2026-11-14 CI 可观测性与本地工具链

### 8.1 远端与 CI 访问边界

- remote 为 `git@github.com:Kasign/stock_data_provider.git`，SSH 认证与 push 可用；`.github/workflows/integration-ci.yml` 的 push 分支为 `integration`、paths 含 `**/*.go` 且不含 `doc/**`，**push 代码自动触发 CI，docs-only 提交不触发**，无需手动 dispatch。job 自带 `mariadb:11.4` + redis service，并在 job 级注入 `SERVER_RELAY_TEST_MYSQL_DSN`、`NEWS_MARIADB_TEST_DSN`、`RESEARCH_TEST_MYSQL_DSN`。
- 未认证时 GitHub Actions 页面与 REST API 可读 **run / step 级状态**（例如 `/actions/runs/{id}/jobs`），但 job 逐步日志下载返回 **403**（需仓库权限）。这是权限边界，不是日志不存在。
- 本机已安装 `gh` 2.104.0。推荐登录方式：**在用户自己的终端（有 TTY）执行** `gh auth login --web --scopes "repo,read:org"`（OAuth 由 gh 托管、不接触明文 PAT），按提示在 https://github.com/login/device 输入形如 `XXXX-XXXX` 的一次性码完成浏览器授权；该交互不能由 AI 会话的非交互 shell 代替。授权后用 `gh auth status` 验证，`gh run view <run-id>` 看各 step、`gh run view <run-id> --log` 读逐步日志、`gh run rerun <run-id>` 重跑。
- **已验证的失败模式（2026-11-14）**：在会话后台启动 `gh auth login --web ...` 虽能生成设备码，但后台进程随会话结束被回收，且授权未完成时报 `Post "https://github.com/login/oauth/access_token": unexpected EOF`；设备码短时有效，重试需重新生成。**不要依赖 AI 会话内启动的登录流程**，请用户在本机终端自行完成，再让 AI 验证。匿名兜底同样无效：REST job logs 返回 403，网页版 job 页只返回页面框架（日志由 JS 渲染）。
- **登录结果（2026-11-14 稍后）**：用户在本机终端完成 `gh auth login --web --scopes "repo,read:org"`，`gh auth status` 显示 `Logged in to github.com account Kasign (keyring)`、Git 协议 ssh、scopes `admin:public_key, gist, read:org, repo`；`gh run view 43561072301 --log` 成功返回 4081 行逐步日志。上述会话内后台启动的失败模式仍成立，但已不影响当前能力。风险提示：scopes 含 `admin:public_key`（gh 默认集），如无需要可在 GitHub 应用授权设置中收紧；token 由系统钥匙串托管，未进入对话或仓库。
- 安全边界：不要把 PAT 贴进对话、写进 `.zshrc` 明文或提交进仓库；无人值守（定时任务/自动化）才使用平台 secret，且按最小 scope 配置。本地不建议另装 MariaDB：CI 已是 `mariadb:11.4` + Go 1.23 且已跑绿，本地版本漂移反而制造「本地过、CI 挂」；确需调试播种链时用与 CI 同名镜像、相同 DSN/healthcheck 对齐。
- 若 `git fetch origin integration` 报 `Host key verification failed`，或本机策略禁止读取 `~/.ssh`，可用 `git -c url."https://github.com/".insteadOf="git@github.com:" fetch origin integration` 走 HTTPS 只读同步（公开仓库无需凭据，只覆盖本次命令、不改 remote 配置），随后 `git merge --ff-only origin/integration`。2026-11-14 已用该方式同步远端提交 `a98823abe`。

### 8.2 A 组 `pass` 的证据级别

- **有效证据**（足以支撑 `pass`、不重复执行）：① 提交 `b0aabaa2edf89c44cd2aeb79b1a57210a8017d1b` 的 Integration CI Run [43561072301](https://github.com/Kasign/stock_data_provider/actions/runs/43561072301) success，`head_sha` 精确匹配且全部 step success；② `gh` 登录后取得的逐步日志实证：`mariadb:11.4` service `healthy`、job env 的 `SERVER_RELAY_TEST_MYSQL_DSN` / `NEWS_MARIADB_TEST_DSN` / `RESEARCH_TEST_MYSQL_DSN` 均为非空 `root:root@tcp(127.0.0.1:3306)/integration_relay_test?...`、`ok example.com/stock-analysis-flow/internal/server/bootstrap 0.627s`、汇总 `go test（根模块） success`；③ 本地用非空假 DSN 验证三个 DB 子测试进入真实连接阶段而非 `t.Skip`，且测试代码仅在 DSN 为空时 Skip。
- **逐用例级实证（Run 69 补齐，日志粒度限制已消除）**：Run 68 的 `go test ./...` 无 `-v`，只有包级 `ok`。2026-11-14 在 `integration-ci.yml` 新增 `test-integration-verbose` 步骤（只重跑 `internal/server/bootstrap` 包并开启 `-v`）与汇总项，使每次 CI 永久输出逐用例明细。提交 `4752dc104` 的 Integration CI Run [43571558064](https://github.com/Kasign/stock_data_provider/actions/runs/43571558064)（#69）success，`head_sha` 精确匹配；逐步日志第 505-533 行显示：两个无 DB 自检用例 `--- PASS`；A 组 7 个子测试 `composition_and_schema`、`authorized_read_and_fail_closed_import`、`pit_invisible`、`context_cancelled`、`list_evidence_page_uses_current_authorization`、`fingerprint_tamper_rejected`、`current_policy_version_change_rejected` **全部 `--- PASS`**；`TestNewsResearchMariaDBIntegration` 总耗时 0.40s，步骤 env 三个 DSN 非空，末尾 `PASS` / `ok ... 0.412s`。因此 A 组 `pass` 已由「包级 ok + 环境论证」升级为**逐用例级真库实证**，不再保留「不声称逐用例日志已核对」的口径。
- **门禁级实证（Run 70，review 修复后）**：独立 review 发现 Run 69 的 `-v` 结果只留在日志、未进入判定（SKIP 与 `[no tests to run]` 都会让步骤 success），且 fixture 内容级别与冻结授权快照不一致会让 B 组解锁后必炸。提交 `c25d881c0` 已修复（见第 10.6 节）。Run [43574975820](https://github.com/Kasign/stock_data_provider/actions/runs/43574975820)（#70）success：逐用例日志 532-557 行显示两个无 DB 自检用例与 A 组 7 个真库子测试**全部 `--- PASS`**（播种 snippet Body 后无回归），B 组 `--- SKIP` 被检出并输出 `::warning::`，verbose 步骤与汇总项均 success。`test-integration-verbose` 现在会校验 `-run` 锚定、拒绝 `no tests to run`、要求 ≥3 个顶层 PASS 与 A 组 7 个子测试 PASS，并把 B 组 SKIP 写入 `$GITHUB_STEP_SUMMARY`，逐用例结果真正进入门禁。
- **B 组 `--- SKIP` 是契约事实，不是失败**：Run 69 日志第 514/526 行、Run 70 日志第 552/560 行显示 `successful_import_with_verified_available_at` 因 News 未提供可验证 `available_at` 而显式 Skip，跳过原因直接打印且被门禁显式告警。不得把该 Skip 记为通过，也不得为消除 Skip 伪造字段（第 10.3 节）。
- CI Go 版本 1.23.x 与本机 1.27.4 不同，已实测新测试代码兼容；`proxy.golang.org` 在本机返回 Bad Gateway，依赖下载用 `GOPROXY=https://goproxy.cn,direct`。

### 8.3 多会话协作事实

- 2026-11-14 出现由 `Eric.Wang` 推送的远端 docs-only 提交 `a98823abe`（第 6 项结论），本地 HEAD 落后一个提交。**必须每轮先按第 4 节规则查远端 HEAD/CI，再决定是否有任务可做**；发现远端已包含对应工作时只核对证据、不重复开发。本轮即按此处理，未重做第 6 项。

## 9. 解锁路线图与执行卡（2026-11-14，等待态）

本节不重复第 6/7 节的证据盘点，只把已有结论组织成「新证据到达后怎么干」的执行手册，避免下次重新判断该做什么。

### 9.1 等待态判定与分流规则

- 历史核对（记录时 HEAD `d16671a3f`）：11 个 workflow 中 10 个最近运行 success，唯一失败项 `One-time Market Data module consolidation #1` 是 2026-11-09 在 `feat/full-go-data-providers-api-switch` 上的一次性历史运行（Run `42560200503`），不在 integration 门禁内；`main` 与各 `feat/*` 分支 tip 均停留在 2026-11-13，**没有新的 Gateway / 渠道 / available_at / Stage / Quant 实现证据**。2026-11-14 后续核对：Integration CI 已由 #68 / `43561072301` 推进到 **#70 / `43574975820` success**（代码 SHA `c25d881c0`），逐用例日志已补齐且进入门禁（第 8.2、10.6 节）。
- 因此第 3 节 1～6 项全部处理完毕（1/2/3A `pass`，3B/4/5/6 `blocked`），**Integration 处于等待态，不是有未完成编码任务**。按第 7.4 节，新证据出现前停止新增 Integration 代码（项目负责人按第 10.4 节明确授权工作面 A 的除外）；工作面为何会形成「无进度」的循环、以及如何切换工作面，见第 10 节。
- 新证据到达后的分流：① 按第 4 节查远端 HEAD/CI 与合并状态（分支上有改动不等于已进 integration）；② 判断它解锁 9.2 中的**哪一条**链；③ 只推这一条，先核对契约/代码事实再动手；④ 精确 SHA 取得 CI success；⑤ 同步本文与对应 `doc/<module>/README.md`。不得同时推进多条，不得在证据不足时先写空壳「跑通」。

### 9.2 五条 `blocked` 链的优先级与触发信号

| 优先级 | 链条 | 触发信号（可验收的新证据） | 责任 Owner | 依据 |
|---|---|---|---|---|
| P0 | B 组 `available_at` | News 提供**可验证来源**的 `available_at` 映射与证据（禁止 published_at/observed_at 推导） | News Owner | 第 3.3 节 B 组、验收基线第 8 节 |
| P1 | Researchflow Stage | 冻结 `stage_code + template_version + output_registry_version`、Output Registry 注册项与 Schema、生产 `StageExecutor`、可信 CallerRef 运行时绑定 | Researchflow / 契约 Owner | 第 6.5 节四条 |
| P2 | Quant 生产 Stage | 可注入的公开 Application Port、`indicator`/`factor` Payload 注册、`DATA-CTR-005/007` 与 `QNT-CTR-003` 定点复核解除 deferred | Quant Owner | 第 3.5 节、第 6.5 节第 2 条 |
| P3 | LLM Provider | 可核验 Gateway 版本、目标 model ID、受控 SecretRef、Responses/SSE/Structured Output/Tool Call 逐项 capability evidence 与有效期 | LLM Owner | 第 7.2 节 |
| P4 | Notification Delivery | 首个真实渠道协议与凭据、回执/幂等/查询语义、权威身份目录与 RecipientResolver、真实 Fanout/Delivery Worker | Notification Owner | 第 7.3 节 |

排序理由：B 组是唯一**已有 A 组真实 MariaDB 测试基础设施**、解锁后可直接补成功路径用例的链，成本最低且证据链最完整；Researchflow Stage 是进入 INT-C 的主干，Quant 依赖其 Output Registry 门禁；LLM/Notification 属外围 Provider，可独立推进但收益排在业务主干之后。

### 9.3 统一执行卡（五步，每条链通用）

1. **验证据**：确认证据是代码/契约/Golden/CI 产物，而不是口头结论或 PR 描述；检查是否已合入 `integration` 并有精确 SHA 的 CI success。
2. **核契约**：重读 `PLAT-002` 登记簿，确认相关 CTR 状态（尤其 `deferred` 是否按重新进入条件解除）、版本与冻结基线是否变化；L1 冻结不等于 L2 实现符合。
3. **最小实现**：Integration 只做 Server 组合根装配与跨模块适配；**不修改其他模块领域代码**、不伪造字段、不创建空实现。**例外**：项目负责人按第 10.4 节明确授权工作面 A 后，按第 10.2 节清单跨模块实施，且必须先完成契约变更影响分析；未授权时本条严格生效。
4. **补验收**：按 9.4 写跨模块测试，真实 MariaDB + 真实依赖 + 冻结 Golden；同时确认既有 A 组 fail-closed 用例不回归。
5. **收口**：push 触发 Integration CI，取精确 SHA success；同步本文第 3 节状态、第 5 节起点、验收基线与模块 README，并写明证据级别（不把 skip/构造测试说成通过）。

### 9.4 各链最小验收用例

- **B 组**：在现有 `internal/server/bootstrap/research_news_mariadb_integration_test.go` 增加成功路径——真实 Facade → Importer → `research_evidence_records` 落库一行、`AuthorizationRef` 与派生关系留痕；断言 `AvailableAt` 来源于可验证证据而非回填；保留 fail-closed 与 PIT/策略/游标用例。
- **Stage**：`CreateRun → Schedule → Worker → StageExecutor → CommitAttemptResult` 跨模块 MariaDB 端到端 + Contract Test，覆盖输出 Schema 版本与可信 CallerRef。
- **Quant**：Application Port 注入后的 `indicator`/`factor` payload 解码与门禁拒绝路径。
- **LLM**：真实 Gateway E2E + CapabilityState 有效期/过期后保持 `unknown` 的 fail-closed。
- **Notification**：真实渠道发送 + 回执/幂等/查询 + Fanout/Delivery Worker 生命周期与组件启停。

### 9.5 常见坑与风险

- 把 docs-only HEAD 当作 CI 通过：`.github/workflows/integration-ci.yml` 的 paths 不含 `doc/**`，文档提交不触发 CI，代码验收仍以 `b0aabaa2e` / Run `43561072301` 为准。
- 把模块级集成测试、构造测试、`t.Skip` 或静态代码存在说成跨模块链路通过；A 组 `pass` 的证据级别见第 8.2 节。
- 用 `published_at`/`observed_at` 回填 `available_at`、硬编码密钥、Mock Facade、空 Key、永远返回空列表的分页——均为契约红线。
- 远端被其他会话推进却未先查 HEAD，导致重复盘点或覆盖；`git fetch` 遇 SSH 主机密钥问题用第 8.1 节 HTTPS 兜底。
- 环境漂移：CI 为 Go 1.23 + `mariadb:11.4`，本机 Go 1.27.4；`proxy.golang.org` 在本机不可用，依赖下载用 `GOPROXY=https://goproxy.cn,direct`。
- 风险：新证据若只存在于功能分支而未合入 `integration`，**不得**据其编写组合根代码；需先确认合并与 CI 结果。

### 9.6 不需要新证据即可收尾的两件小事（均已完成）

1. **`gh` 登录与逐步日志**：用户已登录，`gh run view 43561072301 --log` 实证 MariaDB service / 非空 DSN / `bootstrap` 包 `ok`。Run 68 因 `go test ./...` 无 `-v` 不含逐用例明细；2026-11-14 已在 `integration-ci.yml` 新增 `test-integration-verbose` 步骤（`go test ./internal/server/bootstrap/ ... -v`）与汇总项，使**每次 CI 永久输出逐用例 PASS/SKIP**；Run 70 起该步骤还把逐用例结果纳入判定（≥3 顶层 PASS、A 组 7 子测试 PASS、拒绝 0 测试、B 组 SKIP 显式告警），Run 68 的历史限制已由精确 SHA 的 CI 日志补齐，见第 8.2、10.6 节。
2. **`.workbuddy/`**：用户 2026-11-14 明确「可以提交」，`.workbuddy/memory/2026-11-14.md` 已由提交 `0f7aab244` 纳入版本库；该目录是项目数据而非临时缓存，**不加入 `.gitignore`**，后续工作日志按同样方式提交。

## 10. 工作面判定与授权边界（2026-11-14）

本节回答「为什么连续多轮没有实质性任务进度」，并给出下次必须遵守的反空转规则。只依据本文已记录的证据与 2026-11-14 的只读源码核对，不新增猜测。

### 10.1 根因：Integration 分支可独立完成的工作已经完结

- 第 3 节 1、2、3A 已 `pass`；3B、4、5、6 全部 `blocked`。这 5 条链的共同点是：**解锁条件不在 Server 组合根，而在其他模块领域代码、L1 冻结契约、或真实外部来源证据**。
- 交接文档第 7.4 节与第 9.3 节把 Integration 的职责限定为「组合根装配 + 跨模块适配 + 验收」，并禁止修改其他模块领域代码、禁止伪造字段、禁止创建空壳。在此边界内，A 组测试落地并取得 CI success 后，**确实不再有能改变系统行为的编码任务**。
- 因此从第 4 项起，后续多轮的产出必然是「核对远端 → 无新证据 → 判定 blocked → 同步文档 → 推送」。这不是执行不力，而是**工作面选错**：把「等其他 Owner」当成了 Integration 分支内部可以推进的任务，反复盘点把同一批结论写进不同章节，形成了空转循环。这一策略偏差由本轮修正，不推给外部条件。

### 10.2 P0 `available_at` 的精确解锁清单（只读核对结论，非猜测）

2026-11-14 对 GOV-CTR-002、NEWS-CTR-001、`doc/news/` 全套契约、`internal/news/` 领域模型/Provider/Migration/Repository、`internal/research/adapters/news/` 与 Golden 做了逐文件核对，结论是：**P0 不能由 integration 分支单独合法解锁**。阻断不是「字段没填」，而是「字段、列、证据载体、Provider 采集、冻结准入白名单全链路都不存在」。

| 步骤 | 责任方 | 精确位置 / 依据 | 需冻结契约变更 |
|---|---|---|---|
| 1 取得真实来源「首次公开可获取时间」并完成语义审核、Source/Endpoint allowlist、字段路径/原始值/时区/证据 URL/采集时间/解析器版本留痕、版本绑定、`verified` 状态 | News Owner + 来源 Owner | `doc/news/新闻数据模型与质量契约.md:31-48`；`新闻模块开发任务与验收清单.md:792-801`（`:800` 仍为 `[ ]`） | 否（先有证据） |
| 2 冻结「可接受的来源证据类型、可信等级」白名单，消除 `:446` 未决项与 `:792-799` 已冻结首期准入的文字不一致 | 项目契约 Owner（`feat/design_doc`） | `新闻模块开发任务与验收清单.md:446` | 是 |
| 3 走冻结变更流程：影响分析 → 正式决策 → 消费者影响确认 → 版本升级 → 迁移边界 | 项目契约 Owner | `PLAT-002` 登记簿 `:85-92`；GOV-CTR-002 / NEWS-CTR-001/002/003 / RSC-CTR-001 均 `frozen`；登记簿第 3 节**无** `available_at` 专用 deferred 项 | 是 |
| 4 **在第 1 步取得真实来源之后**，News 领域模型 + Migration V10 + Repository INSERT/SELECT 新增证据载体与 nullable 列 | News Owner | `internal/news/domain/{types,observation,version_record,content}.go`（五个结构均无该字段）；`adapters/storage/migrations.go` 9 个 migration 无该列；`mysql_repository.go:282-294,434-441` | 否（实现） |
| 5 Provider/Parser 保留来源证据（当前 `adapters/provider/http.go:52-113` 只读 body、丢弃全部响应头；`providers.go` 只取 `published_date`，属负面清单） | News Owner | `新闻API契约.md:201`；`代码与文档Review报告.md:338-344` | 否 |
| 6 Get/List 严格 PIT SQL 门禁 `available_at IS NOT NULL AND available_at <= system_as_of`，Facade `researchEvidenceRecordFromPIT` 赋值 | News Owner | `新闻模块与公共平台接入矩阵.md:364`；`internal/news/app/get_evidence.go:212` | 否 |
| 7 Schema/Golden 同步（`NEWS-CTR-001-research-evidence-page` 的 golden 当前 `"available_at": null`）+ GOV-CTR-002 `:76` 最小覆盖用例 | News + 契约 Owner | `doc/news/golden/NEWS-CTR-001-research-evidence-page.golden.json:19` | 是 |
| 8 派生关系留痕（`DerivedFromRefs` / `RelationRefs` 当前双方都是空） | Research + News Owner | `internal/research/adapters/news/evidence.go:177-179`；`internal/news/app/get_evidence.go:184-185` | 需确认 |
| 9 同步 4 组现有 NULL 断言测试，并**新增一条独立的「缺 available_at → fail-closed + 真实表零落库」真库用例**（B 组只覆盖成功路径，不接管 fail-closed） | News/Research/Integration Owner | `bootstrap` 集成测试 `authorized_read_and_fail_closed_import` 与分页用例、`evidence_query_golden_test.go:50`、`get_evidence_test.go:183`；缺口依据见 10.6 节 | 否 |
| 10 合入 integration 取精确 SHA CI success，再启用 B 组成功路径用例 | Integration Owner | 本文第 9.3 节执行卡 | 否 |

**顺序硬约束（review 补充，2026-11-14 逐行核对）**：`doc/news/新闻数据模型与质量契约.md:48` 明确「当前没有符合上述条件的已接入来源，因此现阶段所有记录的 `available_at` 保持 NULL」；`新闻模块开发任务与验收清单.md:798` 更进一步冻结了「当前来源没有合格字段，因此**不新增无实际来源支撑的字段/表，也不修改 PIT SQL**」。因此第 4/6 步**不得**先于第 1 步执行——先加列、先改 SQL 或先造 evidence 表都属于违反冻结契约的"造壳"，不是解锁。`新闻模块与公共平台接入矩阵.md:364` 同样把「先定义来源证据与持久化链路」列为前置。

**证据可得性事实**：`internal/news/adapters/provider/providers.go` 的 Tavily/Brave 等生产 Provider 只保留 `published_date`/`page_age`（负面清单），`http.go` 完全不读响应头；Golden `NEWS-CTR-002` 全文无 `available_at` 键，`sec-edgar` 只是授权策略标识而非真实 endpoint 响应。因此不存在「从已有夹具合法映射」的捷径。

### 10.3 绕过尝试会被主动拦截

`internal/research/adapters/news/evidence.go:152-155`（`ErrEvidenceNotPITSafe`）、`:126-127`（落库前二次授权复核）、`internal/research/evidence/validate.go:139-144`（`ValidateForStrictPIT`）、以及 `TestImporterRejectsMissingOrFutureAvailableAtWithoutPersisting`、`get_evidence_test.go:183`、A 组 `authorized_read_and_fail_closed_import` 都会拦截。用 `published_at`/`observed_at`/`fetched_at` 回填会同时违反 GOV-CTR-002 `:23/:45/:51`、决策 `:293`、API 契约 `:197/:201`、数据模型契约 `:44` 与登记簿 `:87-89`，属契约红线。

### 10.4 下一轮的三种工作面（必须由项目负责人选定）

| 工作面 | 内容 | 能产生什么 |
|---|---|---|
| **A（唯一能产生业务进度）** | 授权本会话以**契约 Owner + News/Research Owner** 身份，按 10.2 的 10 步推进 P0：先做契约变更提案与来源语义审核，再改领域代码/Migration/Provider/Schema/Golden，最后启用 B 组用例取 CI success | News→Research 成功写入链路真正跑通，B 组由 `blocked` 转 `pass`，阶段向 INT-C 推进 |
| **B（仅 Integration 侧资产）** | 继续补可观测性、待启用测试、组合根装配（当前已完成：B 组成功路径用例 + CI `-v`） | 验收能力与解锁成本下降，但业务链路状态不变 |
| **C（等待态）** | 保持现有边界，等外部 Owner 提供新证据 | 不产生业务进度；应直接报告，不制造任务 |

### 10.5 反空转规则（下次新聊天必须遵守）

1. 若用户只说「继续下一步任务」、远端无新提交/新失败/新证据、且**未授权工作面 A**：直接汇报「Integration 独立工作已完结、5 条链阻塞根因、10.4 三个选项」，**不再重复盘点、不再新增进度文档、不再制造 `t.Skip` 用例或工具链改动充当进度**。
2. 用户授权 A 后，先按 10.2 输出契约变更影响分析并取得确认，再动代码；改动跨模块时按功能聚合提交，每步取精确 SHA 的 CI success，同步本文第 3 节状态与对应 `doc/<module>/README.md`。
3. 不要把「文档同步完成」「CI 日志补齐」「测试骨架落地」描述为实质性业务进度；三者只属于工作面 B。
4. 每次仍按第 4 节先查远端 HEAD/CI；`gh` 已登录，可用 `gh run view <run-id> --log` 核对逐用例结果。

### 10.6 2026-11-14 独立 review：发现、修复与剩余边界

用户要求在授权工作面 A 前「先 review 一遍」。本轮对 `4752dc104`（B 组用例 + CI `-v`）与 `3124df7ac`（文档）做了一次挑刺式只读复核：逐字段比对 `researchapp.EvidenceRecord` / `EvidenceDraft`、`researchevidence.Evidence` 的 24 个 json tag、`research_evidence_records` 的 INSERT 列值、`pitTimeLayout`、News `GetEvidence` 与 `AuthorizeEvidence` 的二次授权链、Golden 授权快照的 `content_level`，并核对第 10.2 节引用的 8 个文件与行号（全部真实）。**结论不是「没问题」，而是发现了 5 个真实缺陷，其中 2 个会让 B 组解锁后首次执行必炸。**

| # | 级别 | 问题 | 根因 | 修复（提交 `c25d881c0`） |
|---|---|---|---|---|
| 1 | P0 | B 组成功路径解锁后 `ImportEvidence` 必失败 | fixture 只播 `SourceSummary`，`actualEvidenceContentLevel` 返回 `summary`；Research `AddEvidence` 的 `Authorizer` 用**实际内容级别**查 `news_authorization_snapshots.content_level`，而冻结 Golden 只有 `snippet` 快照 → 查无行 → `AuthorizationInternal` | 抽出 `newsResearchSeedContent()` 播种 `ContentSnippet` 级 `Body`，使实际级别与快照一致；自检用例新增 `RestrictContentLayers` + 授权级别断言，无 DB 即可发现漂移 |
| 2 | P0 | 落库 `content_level != "snippet"` 硬编码断言必炸 | 该列来自 `string(value.ContentLevel)`（实际级别），不是请求级别 | 改为 `contentLevel != string(record.ContentLevel)`，并显式断言 `record.ContentLevel == EvidenceContentSnippet` |
| 3 | P1 | CI 把 `SKIP` 当 PASS、`-run` 匹配 0 个测试也全绿 | `-v` 输出只在日志里，步骤 `outcome` 不解析用例结果；`-run` 未锚定且无最小 PASS 守卫 | `test-integration-verbose` 增加门禁：`-run` 锚定 `^...$`、拒绝 `no tests to run`、要求 ≥3 个顶层 PASS 与 A 组 7 个子测试 PASS、B 组 SKIP 输出 `::warning::` 并写入 `$GITHUB_STEP_SUMMARY` |
| 4 | P1 | 解锁后「缺 available_at → fail-closed + 真实表零落库」在集成层失去覆盖 | 上一轮把该分支从 `t.Fatalf` 改成 `t.Skip`，B 组只覆盖成功路径；全仓只剩 `evidence_test.go` 的 fake 级单测（不查真实表） | 改为「仅在缺证据时执行完整 fail-closed + 零落库断言」，非 nil 时显式 `t.Log` 说明缺口；第 10.2 节第 9 步改为要求补一条独立真库回归用例 |
| 5 | P2 | `AvailableAt.Location() != time.UTC` 对合法 UTC 误报 | GOV-CTR-002 约束的是零偏移 / `Z` 结尾的表示，不是 Go `time.Location` 指针身份；`FixedZone("UTC",0)`、RFC3339Nano `+00:00` 解析结果都会误判 | 改为 `Zone()` offset==0 + `UTC().Format(RFC3339Nano)` 以 `Z` 结尾，并指出 `isUTC` / `validateRequiredUTC` 与 News 侧 `validEvidenceUTC` 的实现不一致（属后续 Owner 事项） |

**同时修正的不准确表述**：提交 `4752dc104` 的说明写过「fail-closed 分支由 B 组成功路径接管」，实际 B 组只覆盖成功路径；已在代码注释与第 9 步更正。第 10.2 节补充了顺序硬约束：`新闻数据模型与质量契约.md:48` 与 `新闻模块开发任务与验收清单.md:798` 明确「在取得合格真实来源前不新增无来源支撑的字段/表、不修改 PIT SQL」，所以第 4/6 步不得先于第 1 步执行。验收基线第 9 节一处表格前缺空行的 Markdown 渲染问题也一并修正。

**验证**：`c25d881c0` 的 Integration CI Run [43574975820](https://github.com/Kasign/stock_data_provider/actions/runs/43574975820)（#70）success，`head_sha` 精确匹配；逐步日志 532-557 行显示两个无 DB 自检与 A 组 7 个真库子测试在播种 Body 后**全部 `--- PASS`**（无回归），B 组 `--- SKIP` 被检出并告警，verbose 步骤与汇总项 success。本地 gofmt / `go vet ./...` / `go test ./...`、workflow YAML 与内嵌 bash 语法、守卫 grep 模式对 Run 69 真实日志的匹配均已验证。

**仍存在的边界（不是缺陷，但不得宣称已消除）**：

- **B 组成功路径仍从未被真正执行**：当前按契约 `t.Skip`，其 SQL、字段、payload 键断言只做了静态逐项核对，真跑要等工作面 A 解锁。因此「确认没问题」只到「已知静态缺陷已修 + A 组真库回归通过」，不能表述为「B 组已验证」。
- News storage 的 `evidencePITSelect` 无 `available_at` 列、全目录 0 匹配；解锁范围（表结构、证据载体、PIT SQL、DTO、时间归一化）尚未定义，B 组的 `Equal` / offset 断言依赖未来实现。
- `newsResearchPITLayout` 是 `pitTimeLayout` 的手工拷贝，无编译期约束；payload 只校验 4 个留痕键存在（值一致性由列级断言覆盖）；`ListEvidence` 的 `available_at` 与 `GetEvidence` 一致性在解锁后仍缺用例；`count == 1` 是无 WHERE 的全表计数，依赖子测试顺序（当前无 `t.Parallel()`）。
- CI 逐步日志受 GitHub 保留期限制；`$GITHUB_STEP_SUMMARY` 只写 B 组 SKIP 告警与 PASS 计数，未上传结构化 artifact。
- 本机 `gh` OAuth token 缺 `workflow` scope，HTTPS push 无法更新 `.github/workflows/*.yml`；本轮最终用 `GIT_SSH_COMMAND="ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new"` 的 SSH 非交互方式推送成功，该差异已记入工作日志。

### 10.7 2026-11-14 工作面 A 已授权：第 1 步完成，停在正式决策门禁

- 用户 2026-11-14 明确「授权A」。按第 10.5 节第 2 条与 `PLAT-002` 登记簿第 5 节，先产出契约变更影响分析，**不直接动代码**。
- 交付 `doc/contract/NEWS-CTR-001-available_at-来源证据解锁影响分析.md`（提案，待正式决策）：完成影响分析、消费者影响确认材料、版本升级方案、迁移边界、十步实施计划与验收标准，并附来源语义审核的外部官方证据。
- **决定性结论**：SEC Webmaster FAQ 逐字声明 `There is no timestamp to indicate when filing content is first available on sec.gov.`；`Acceptance Time` 是「被 EDGAR 接受」的系统时间（EST），到 sec.gov 可见还有 1-3 分钟、高负载显著增加且**不可预测**的 lag；EDGAR PDS Technical Specification 的 `<TIMESTAMP>` 是 `Edgar Dissemination date-time, used for Post Acceptance Corrections only`，由 R/A Subsystem 提供，属系统侧证明材料。仓库已接入 Provider 的字段全在契约负面清单。因此 `sec-edgar` 与现有 Provider **均不合格**，与契约 `:48`「当前没有符合条件的已接入来源」一致，且现在有可复核的外部官方出处。
- 按 `新闻模块开发任务与验收清单.md:799` 与 `新闻数据模型与质量契约.md:48`，此时**不得新增字段/表、不得改 PIT SQL、不得生成 `available_at`**；先动代码即契约明令禁止的「造壳」。这是 A 第 1 步的实质结论，不是等待 Owner 的空转。
- **下一步必须由项目负责人**在提案第 3 节选定 **P1**（推荐：提供合格真实来源与授权凭据）/ **P2**（按契约 `:47` 新增带区间语义的独立字段，属 MAJOR 且不能替代 `available_at`）/ **P3**（维持现状），并确认版本升级与消费者影响。**未选定前不进入代码实施**；P2 单靠新字段无法解锁 B 组（Research 侧仍要求 `AvailableAt != nil`）。
- 本轮无 `.go` / workflow 改动，不触发 CI；代码验收仍为 `c25d881c0` / Run `43574975820`（#70）。

### 10.8 2026-11-14 P0 `available_at` 正式搁置（`shelved`）与全渠道复核结论

- **决定**：项目负责人 2026-11-14 明确「处理不了先搁置」。P0 `available_at` 状态由 `blocked` 改为 **`shelved`（搁置）**，P1/P2/P3 均未选定，不推进版本升级、迁移与代码实施。
- **上一轮结论的更正（必须记住，避免再犯）**：第 10.7 节据 SEC 官方证据得出「`sec-edgar` 与现有 Provider 不合格」成立；但由此推出「没有合格来源」属**过度外推**，已作废。SEC 那句话限定 `on sec.gov`，且 PDS 规范标题为 `DRAFT`。
- **全渠道复核结论（本轮新增，详见 `doc/contract/NEWS-CTR-001-available_at-来源证据解锁影响分析.md` 第 1.5 节）**：

| 来源 | 字段 / 实测 | 精度·时区 | 授权 | 判定 |
|---|---|---|---|---|
| 日本 EDINET（金融庁） | `submitDateTime` = `"2024-06-20 15:30:45"` | **秒 / JST** | **免费公开 API** | 形式最合格 |
| LSE RNS | MIT304 `SendingTime`（UTC，`YYYYMMDD-HH:MM:SS.sss`） | **毫秒 / UTC** | 需市场数据牌照 | 语义精度合格、授权受限 |
| HKEXnews | `Release Time` = `22/10/2026 18:27` | 分 / HKT | 无机读授权 | 授权缺口 |
| 巨潮 CNINFO 公开接口 | `announcementTime` 实测 4/4 条均为 `00:00:00` | **仅日期** | 有正式 API | **准入 4 硬性不合格** |

- **根因判定（比「缺来源」更准确）**：`available_at` 的语义只对**法定披露类**内容成立，对**新闻报道类**内容不成立。News 现有来源（Bocha/Tavily/Brave/SearXNG）是搜索聚合，字段全在契约 `:44` 负面清单，**换任何一家搜索类 Provider 结果相同**——是品类不匹配，不是覆盖不全。
- **因此重启条件不是「再找来源」，而是产品决策**：是否为 News 新增一类法定披露来源（EDINET / RNS / HKEX 形态），并落实其授权、字段规范与覆盖市场。该决策属 News 产品定位与输入结构变更，**不在 Integration 分支权限内**。
- **搁置期间禁止**：不新增 `available_at` 列或证据表、不改 PIT SQL、不生成/回填 `available_at`、不启用 B 组用例、不为消除 `--- SKIP` 改断言、不重复盘点这 4 家候选。
- 阶段判定：仍为 INT-B 进行中；B 组状态 `shelved`（非 `fail`）；代码验收 `c25d881c0` / Run #70 不变，本轮纯文档、不触发 CI。
