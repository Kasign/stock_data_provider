# Integration 当前任务交接与继续执行

- 仓库：Kasign/stock_data_provider
- 当前工作分支：integration
- 本文更新日期：2026-11-16
- 本文记录时核对的 integration HEAD：`7378b035c88dabb13c6486f7a54d0df2663d96d5`（Live Probe gofmt 修复提交）。**该字段是记录时快照，不是实时值**：后续 docs-only 提交会让 HEAD 前移，实际当前值一律按第 4 节规则查远端。最近代码验收 SHA `7378b035c88dabb13c6486f7a54d0df2663d96d5`，Integration CI Run [43796597261](https://github.com/Kasign/stock_data_provider/actions/runs/43796597261)（#83）对精确 SHA success；全量检查和最终汇总均通过。历史尝试：`a992f4c` / Run `43788290026`、`4f17032` / Run `43788537976` 均因门禁匹配规则缺陷误失败，已由 `d90c6fc` 修正；不代表业务测试失败。此前代码验收：`c25d881c0` / Run `43574975820`（#70）success。
- 本文用途：下一轮新聊天的首要接续入口。新聊天先读本文，再核对 GitHub 当前 HEAD/CI；不得直接假设本文 SHA 仍是最新。
- 2026-11-14 联调补记：新增 `internal/server/bootstrap/research_news_mariadb_integration_test.go`，落地第 3 节 A 组跨模块 MariaDB 验收用例与无 DB 自检；提交 `b0aabaa2e` 已推送 integration，Integration CI Run [43561072301](https://github.com/Kasign/stock_data_provider/actions/runs/43561072301) success（head_sha 精确匹配，全部检查项通过），**A 组状态由 `not_comparable` 升级为 `pass`**。本机无 MariaDB/Docker，本地 `t.Skip` 只代表开发态；已用非空假 DSN 在本地验证三个 DB 子测试均进入真实连接阶段而非 Skip，且 CI job env 提供 `SERVER_RELAY_TEST_MYSQL_DSN` 与 mariadb:11.4 service，可确认 CI 上真库执行。匿名 job 日志下载需权限（403）；2026-11-14 完成 `gh` 登录后已取得 Run 68 逐步日志，实证 `mariadb:11.4` service healthy、三个 DSN 非空、`internal/server/bootstrap` 包 `ok 0.627s`。但 CI 使用 `go test ./...`（无 `-v`），Run 68 日志不含逐用例 PASS/SKIP 明细；该限制已由 Run 69 新增的 `test-integration-verbose` 步骤消除并取得逐用例实证，详见第 8.2 节。B 组成功写入链路经源码核对确认被 `available_at` 硬阻断；该状态于 2026-11-14 曾记为 `blocked`，随后按项目负责人决议改为 `shelved`。B 组成功路径用例已落地，当前按契约事实 `t.Skip`。
- 最近代码验收 SHA：`7378b035c88dabb13c6486f7a54d0df2663d96d5`；Integration CI Run [43796597261](https://github.com/Kasign/stock_data_provider/actions/runs/43796597261)（#83）success，head_sha 精确匹配。Run #83 覆盖 gofmt、root/go-core vet/test、News/Research 逐用例 MariaDB 门禁、Worker Bootstrap MariaDB/Redis 生命周期、Compose、WebUI 检查和汇总。
- 上一份完整代码验收：`c25d881c041eb349450c004c42c4cf227c9bf437` / Run [43574975820](https://github.com/Kasign/stock_data_provider/actions/runs/43574975820) success；历史：`4752dc104` / Run `43571558064`、`b0aabaa2e` / Run `43561072301`、`dde7d4f8` / Run `43370552552`。
- News→Research 组合代码 SHA：d664b9f6ec08d358b914242735fde52f49e9c7c1；Run 43369529083 success。`dde7d4f` 仅移除一次误加的重复 LLM 传输适配器，当前代码树不包含重复实现。
- News→Research 组合构造提交：06722fdcd2e178c69ee4ae7bb3bf06caf907545d，Run 43368950457 success。
- 启用 News/Research MariaDB 集成测试后，Run 43369220849 对 SHA 570b001 暴露既有 News 集成夹具缺少 NEWS-CTR-002 schema_version；已在 SHA d664b9f 修正，Run 43369529083 success。
- CI 现设置 NEWS_MARIADB_TEST_DSN 与 RESEARCH_TEST_MYSQL_DSN，News 与 Research 模块级 MariaDB 集成测试已进入根模块测试并通过；不代表跨模块 Importer 写入链路已验收。
- 2026-11-14 第 4 项补记：完成 Researchflow Stage/Template/Output Registry 冻结契约与真实代码的**只读盘点**，判定**第 4 项 `blocked`（不是 `fail`）**——Stage 输入/输出、Output Registry 版本与注册项、生产 Provider 三项门禁未通过，仅可信 CallerRef 在契约定义层满足、运行时绑定仍有 L2 缺口。完整证据、责任归属与重新进入条件见本文第 6 节。本轮未新增任何 Stage / StageExecutor / Stage Code / 输出 Schema 代码，未改动模块领域代码，未触发新 CI；阶段判定仍为 INT-B 进行中。
- 2026-11-14 第 6 项补记（远端提交 `a98823abe`）：LLM Provider 与 Notification Delivery 生产装配条件只读复核已完成，**均判定 `blocked`**，详见本文第 7 节。至此第 3 节 1～6 项全部处理完毕：1、2、3A 为 `pass`；3B 为 `shelved`、4/5/6 为 `blocked`。**当前没有可推进的 Integration 业务开发任务**，在新证据或正式生产消费端出现前停止新增 Integration 代码（见第 7.4 节）。这一「等待态」的根因、工作面选择与反空转规则见本文第 10 节；后续若用户未授权跨模块/契约变更，不应再重复盘点或制造文档进度。
- 2026-11-14 CI 可观测性补记：本机 `gh` 2.104.0 已由用户在本机终端完成网页授权登录（`Kasign` / keyring / `repo,read:org`），`gh run view 43561072301 --log` 已成功取得 Run 68 逐步日志，确认 MariaDB service healthy、三个 DSN 非空、bootstrap 包 `ok`；匿名 REST API 的 job 日志下载返回 403。因 CI 未启用 `-v`，Run 68 日志不含逐用例明细；该限制已由 Run 69 的 `test-integration-verbose` 步骤永久消除。工具链细节、HTTPS fetch 兜底与证据级别见本文第 8 节。
- 2026-11-14 工作面复盘补记：用户指出「连续多轮没有实质性任务进度」。核对后确认 Integration 分支可独立完成的编码工作已完结，5 条 `blocked` 链全部需要其他模块领域代码 / L1 冻结契约 / 真实外部来源证据；此前把「等 Owner」当作分支内任务反复盘点，是策略偏差。本轮新增第 10 节固化工作面判定、P0 精确解锁清单、三种工作面与反空转规则，并交付两项 Integration 侧代码资产（B 组成功路径用例 + CI `-v` 逐用例证据），Run `43571558064` 对代码 SHA `4752dc104` success。`.workbuddy/memory/2026-11-14.md` 经用户确认已提交（`0f7aab244`），不加入 `.gitignore`。
- 2026-11-14 工作面 A 补记：项目负责人已授权先做影响分析；提案保持待正式决策，P0 后续已由负责人决定 `shelved`。SEC 官方证据只否决 EDGAR 接受时间作为首次公开可获取时间，不证明所有市场来源均不存在。来源候选复核已在 2026-11-16 Review 中降级为待证实候选，详见第 10.8、10.9 节及提案第 1.5 节。

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
4. **当前总览（2026-11-16 校准；最新代码验收 SHA `d1b58b49fee809c5204fafe3409dc36da9615794` / Run [43790923451](https://github.com/Kasign/stock_data_provider/actions/runs/43790923451)（#81）success）**：第 3 节 1、2、3A `pass`；3B `shelved`；4、5、6 `blocked`。A 组真实 MariaDB 验收已通过；B 组成功路径仅有测试资产、未真实执行。Server Worker Bootstrap 真实 MariaDB/Redis 生命周期验收已通过，详见第 10.10 节。**当前没有新的可推进 Integration 业务领域开发任务**；只在出现新的外部证据或模块 Owner 解锁条件后推进对应链路。第 6 项 LLM / Notification 的 `blocked` 详见第 7 节；Researchflow Stage 与 Quant 按第 6 节条件阻断。P0 `available_at` 维持 `shelved`：现有候选均未证明满足首次公开可获取语义和当前 A 股范围。具体以第 10.9 节及提案第 1.5 节为准。
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
| 可信 CallerRef | **Worker → StageExecutor 内部传播已实现；Server 生产身份注入仍待接入** | 已具备：CreateRun 通过可信 RequestIdentity 形成 caller-scoped 幂等记录；Migration v5 将 run_ref 唯一绑定到一条 CreateRun 身份记录；Worker Store 在 Repeatable Read 快照读取该 caller_ref，Worker 在 claim 前 fail-closed、claim 后复核一致，并注入 StageExecutionRequest.AuthenticatedCallerRef。该字段不进入公共 DTO。仍缺：Server 生产 RequestIdentity 装配必须注入决议要求的 `service:researchflow`；若未来改为跨进程 Provider 调用，必须另行定义认证传输，不能把内部字段直接当远程可信凭据。 |
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
- 因此第 3 节 1～6 项全部处理完毕（1/2/3A `pass`，3B `shelved`，4/5/6 `blocked`），**Integration 处于等待态，不是有未完成编码任务**。按第 7.4 节，新证据出现前停止新增 Integration 代码（项目负责人按第 10.4 节明确授权工作面 A 的除外）；工作面为何会形成「无进度」的循环、以及如何切换工作面，见第 10 节。
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

- 第 3 节 1、2、3A 已 `pass`；3B `shelved`，4、5、6 `blocked`。这 1 条搁置链与 3 条模块阻断链的共同点是：**解锁条件不在 Server 组合根，而在其他模块领域代码、L1 冻结契约、或真实外部来源证据**。
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

> **历史结论校正：** 本节 2026-11-14 对 EDINET/RNS「形式合格」的判断已由 2026-11-16 第 10.9 节 Review 纠正。EDINET 提交时间、RNS 消息传输时间均未证明等于首次公众可获取时间；本项目范围为中国 A 股，海外来源不构成当前验收证据。以第 10.9 节及提案第 1.5 节为准。

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


### 10.9 2026-11-16 Integration Review 与门禁修正

- 开始前核对远端 `integration` HEAD：`a50302a87c0969eae2377ee3d8ca82d5fe697e5a`；本次只在当前分支工作。
- Review 覆盖从 `396fa05d10bcb65eec2087cd4ef060b1a7eda720` 到该 HEAD 的 15 个提交。未发现本轮引入可确认的生产业务代码回归；主要问题集中在 CI 逐用例门禁、来源分析证据等级和验收文档过期。
- CI 原门禁用 `grep -c '^--- PASS: Test'` 统计顶层 PASS，实际会把子测试 PASS 一并计数；B 组只要没有匹配到 SKIP 就会被写成「已启用」，未验证明确 PASS，也未拒绝缺失结果。
- CI 门禁修复分三次提交完成：`a992f4c`（Run #71）首次把顶层/子测试结果纳入门禁，但未兼容 Go 子测试缩进；`4f17032`（Run #72）补充缩进兼容，但错误地把两个根级子测试归入 `read_chain`；最终 `d90c6fc` 按实际测试层级拆分路径匹配。Run [43788740211](https://github.com/Kasign/stock_data_provider/actions/runs/43788740211)（#73）对精确 SHA `d90c6fc` success。实际日志确认三个顶层测试 PASS、A 组 7 个 MariaDB 子测试全部 PASS、B 组成功路径明确 SKIP；其他全量检查均通过。B 组 SKIP 对应 `shelved`，不是 fail，也不是成功写入验收。
- 来源提案第 1.5 节原结论过度：EDINET `submitDateTime` 只能先视为提交时间候选，尚未证明为首次公众可获取时间；LSE RNS FIX `SendingTime` 定义为消息传输时间，不能直接推定为公众可获取时间；HKEX 分钟级时间不满足契约精度；CNINFO 公开接口实测值仅日期精度。项目范围是中国 A 股、海外市场暂缓，EDINET/RNS 不得作为当前 A 股验收来源。所有候选均不得标记 `verified`。
- 现有 SEC 官方 FAQ 仅支持「EDGAR Acceptance Time 不等于 sec.gov 首次公开可获取时间」的否定结论；不得由此推出市场上不存在其他合格来源。
- 项目总览第 3 节已按当前联调授权修正：Integration 可维护跨模块测试、CI 门禁和现有验收证据；不得修改模块领域代码、公共平台实现或冻结契约。该职责是当前阶段性授权，不改变长期模块 Owner 边界。
- 本轮没有修改 Go 业务代码、冻结契约、Schema 或 Golden；没有新增接口/字段；没有合并其他分支。


- 2026-11-16 最终门禁验收：Run #71（`43788290026`）因 grep 未匹配缩进后的子测试输出误失败；Run #72（`43788537976`）因将 `fingerprint_tamper_rejected` 与 `current_policy_version_change_rejected` 错归到 `read_chain_and_fail_closed_import` 路径误失败。两次失败均为新增门禁自身的匹配逻辑缺陷，实际测试用例均 PASS。Run #73（`43788740211`）在 `d90c6fc5f15c6ea076ce31bc02e866ef4fe5197f` 上通过，已按真实 Go 测试层级分别验证 5 个 read_chain 子测试、2 个根级子测试，及三个顶层测试；B 组明确 SKIP。不得将 Run #71/#72 记录为业务测试失败，也不得省略其门禁缺陷修复过程。

### 10.10 2026-11-16 Server Worker Bootstrap 真实依赖生命周期验收

- 本项是 Server 组合根基础设施验收，不改变模块领域代码或冻结契约。开始前确认 `integration` HEAD 为 `9bcc8f7e`；当前仅在 `integration` 分支工作。
- 复核发现 `internal/server/bootstrap/worker_lifecycle_test.go` 已覆盖 `legacyAnalysisWorker` 的启停、禁用与启动失败回滚，但使用模拟 SQL Driver，只是组件级单测；不重复扩展该测试。
- 新增 `internal/server/bootstrap/worker_bootstrap_integration_test.go`：父测试从 CI 提供的 `SERVER_RELAY_TEST_MYSQL_DSN` 创建隔离 MariaDB database，使用真实 Redis 地址启动独立子进程并执行完整 `bootstrap.Run`；开启模块 Migration 与旧 Analysis schema migration；等待 Worker 启动日志后向子进程发送真实 `SIGTERM`，要求 `Run` 正常返回并完成其生命周期停止及共享资源关闭 defer，最后清理隔离数据库。
- HTTP 不启动的负向验证：父测试先占用一个 TCP 地址，将该地址作为 `HTTP_ADDR` 传入子进程，同时设置非法 `HTTP_READ_TIMEOUT`。Worker 角色若错误加载 HTTP 配置或尝试启动 HTTP Server，子进程将无法通过启动；真实测试通过说明 Worker 角色跳过 HTTP 配置和监听器装配。
- `.github/workflows/integration-ci.yml` 增加独立 verbose 步骤 `test-worker-bootstrap`，精确要求 `TestWorkerBootstrapMariaDBRedisIntegration` 顶层 `PASS`；缺失结果或 `SKIP` 均失败，并纳入最终检查汇总。Job 使用现有 MariaDB 11.4 / Redis 7.4 服务，不引入新依赖。
- 首轮 Run [43790574994](https://github.com/Kasign/stock_data_provider/actions/runs/43790574994)（#80）中 Worker 真库集成步骤已 `success`，根模块测试、vet、News/Research 门禁及其他检查通过；唯一失败为新增 Go 测试文件 map 字面量未 gofmt。已修正格式，不是运行逻辑缺陷。
- 最终提交 `d1b58b49fee809c5204fafe3409dc36da9615794` 的 Run [43790923451](https://github.com/Kasign/stock_data_provider/actions/runs/43790923451)（#81）对精确 SHA `success`。逐步日志显示 Worker Bootstrap MariaDB/Redis 集成用例 `--- PASS`；独立门禁步骤通过；gofmt、root/go-core vet/test、News/Research 逐用例门禁、Compose、WebUI 语法/单测/镜像构建和最终汇总全部 success。
- 验收范围：`pass` 的是当前 Worker 角色下完整 Bootstrap 启动、真实依赖连接、SIGTERM 触发关闭、Run 正常退出及 HTTP 不监听。消费者先停止、共享资源后关闭的通用顺序仍由 `TestStopConsumersThenCloseResourcesPreservesOrder` 验证；本测试不代表 Researchflow Stage Worker 已接入，也不代表新业务链路达到 INT-C。
- 后续任务：Integration 侧不再为 Worker 生命周期重复补测试。按第 9.2 节优先级，下一条主干是 Researchflow Stage；但当前仍 `blocked`，须等 Researchflow/契约 Owner 提供冻结 Stage 三元组、Output Registry/Schema、生产 StageExecutor 和可信 CallerRef 运行时绑定后，Integration 才进行装配与 MariaDB 端到端验收。P0 `available_at` 仍 `shelved`，不得用本任务绕过。

### 10.11 2026-11-16 A股交易所公告首次公开可获取时间二次调查

- 本轮继续固定在 integration，开始时远端 HEAD 为 cd3edd33474a277473532f381fbf9a2e6dfa79a8；未切换分支、未修改业务代码、冻结契约、Schema 或 Golden。
- 新增官方流程证据：上交所科创板信息披露办理指南明确非直通公告经审查后由发行人确认发布；选择上交所网站披露时，公告确认发布后直接上网。上交所直通车官方答记者问同时说明 15:30 前确认发布的公告会从 15:30 起集中披露，15:30 后确认发布则按提交时间披露。北交所信息披露办理指南也规定最终由发行人确认发布，且其他媒体披露不得早于规定平台。
- 深交所《数据接口规范（Ver4.72）》明确行情系统信息公告结构头含发送日期/时间，SJSHQ.dbf 的 HQBSL4 是当日最近一次收到信息公告的时间（HHMMSSss）。它是行情分发接收时间且非逐条公告字段，不能作为首次公众可获取时间；不能因为有亚秒精度就准入。
- 本轮结论：上交所/北交所“确认发布”是新的 A 股候选业务事件，比搜索引擎时间、公告日期和首次抓取时间更接近公开可获取边界；但尚未取得面向消费者的逐公告时间字段规范、时区、Endpoint 授权、原始响应证据和内容版本绑定，也未证明确认动作时间等于公众实际可访问时间。候选不等于 verified。
- P0 available_at 仍为 shelved，但不是停止调查。下一步明确为向交易所/官方信息服务运营方核实机器接口：确认事件时间字段、网站实际上线时间、15:30 集中披露行为、公告更正版本、时区精度、历史覆盖、授权和留存许可；以真实公告逐条核对时间顺序。只有满足 NEWS-CTR-001 全部准入条件并完成正式 DEC/版本升级后，才实施 V10、证据载体、PIT 与 B 组真实成功路径。
- 参考官方材料：上交所指南 https://www.sse.com.cn/lawandrules/guide/stock/kcbxxpljg/c/c_20250425_10777769.shtml；上交所直通车说明 https://big5.sse.com.cn/site/cht/www.sse.com.cn/aboutus/mediacenter/hotandd/c/c_20150912_3988653.shtml；北交所指南 https://www.bse.cn/cxjg_list/200025676.html；深交所接口规范 https://www.szse.cn/www/marketServices/technicalservice/history/P020180328468171229352.pdf。
- 本轮没有新增业务代码或测试，没有触发 CI。代码验收 SHA 仍为 d1b58b49fee809c5204fafe3409dc36da9615794 / Run #81 success；本轮文档提交不改变该代码验收证据。

### 10.12 2026-11-16 上证信息官方公告文件 API 候选核实

- 在 10.11 的交易所披露流程调查基础上，发现上证所信息网络有限公司（SSE InfoNet）有正式“公告文件下载接口服务”。其官方说明书列出沪市主板、科创板和深市上市公司公告；按天查询元数据，包含索引、证券代码、标题、分类、更新时间、下载地址，并支持 7×24 小时轮询。产品需申请试用/审核开通认证，正式使用需签约。
- 官方价目表公开公告文件服务 6 万元人民币/年。对个人低频研究场景成本明显偏高；在拿到技术字段字典、试用样例和个人用途授权确认前，不购买、不接入生产。
- 关键未知：元数据“更新时间”究竟是交易所确认发布、官网实际可访问、上证信息采集/入库，还是文件更新/供应商同步时间；其精度、时区、公告版本/更正关联和历史覆盖均未由公开产品说明书定义。字段名本身不能通过 NEWS-CTR-001 准入。
- 官方材料：产品说明书 https://www.sseinfo.com/services/data/szjr/ggwjfw/c/10010710/files/5f5989f49fb142f380fb027bb9997179.pdf；价目表 https://www.sseinfo.com/services/charge/pricelist/c/10010718/files/6e4e8d001bbd4554a254aeac11ee0e50.pdf；申请指引 https://bsp.sseinfo.com/business/?id=1703679027550273537；产品入口 https://www.sseinfo.com/services/data/szjr/。公开咨询电话 4008888400（按键 2-5），邮箱 ywsun@sse.com.cn。
- 北交所《交易支持平台数据接口规范（V2.1）》第十一章也定义公告文件发送日期/时间及 HQBSL4 最近一次接收时间；均属于交易支持/分发链路，不足以证明公众首次可获取时间。规范：https://www.bse.cn/uploads/6/file/public/202209/20220902201701_kas8vw9r25.pdf。
- 当前判断：SSE InfoNet API 是迄今最有价值的官方 A 股候选，因为存在可申请的结构化公告 API 和更新时间字段，且覆盖沪深；但仍为 candidate/not_comparable，不是 verified。下一步应索取最新技术文档和原始响应样例，逐字段确认更新时间的产生事件、时区精度、公告版本绑定、授权留存及个人套餐，再决定是否进入 DEC 和实现。
- P0 available_at 仍 shelved；没有新增业务代码/测试，没有触发 CI。代码验收 SHA 仍为 d1b58b49fee809c5204fafe3409dc36da9615794 / Run #81 success。文档提交不改变代码验收证据。


### 10.13 2026-11-16 `available_at` 后续实现的校验与可观测性要求

- 项目负责人提出：`available_at` 来源字段存在较大疑虑，开发时应增加校验逻辑和日志信息，以便核对真实数据。本意见已同步至来源影响分析提案第 1.8 节。
- 该要求是**未来实现验收要求**，不改变 P0 当前 `shelved` 状态，不构成新增字段/表、修改 PIT SQL 或解除 B 组 Skip 的授权。
- 解锁后必须保留来源/Endpoint/策略版本、字段路径、原始时间文本、原始时区、解析后 UTC、解析器版本、公告及内容版本绑定、系统观察时间、证据引用/摘要和核验规则版本。
- 校验至少覆盖：精度及时区、来源时间晚于首次观察时间、公告版本绑定、更正公告追加式修订、跨来源冲突、披露日期与时区一致性、重复采集幂等、verified 以外状态 fail-closed。
- 结构化诊断不得记录密钥或敏感响应正文；不得把 observed_at/retrieved_at/fetched_at 当作 available_at。来源冲突不自动挑选最早/最晚值，必须保留原始证据与 reason_code。
- 目前没有真实合格来源，故本轮不写 News 领域代码、不增加 Migration、不改 Schema/Golden/PIT SQL；等待来源与正式 DEC 条件满足后按第 10.2 节顺序执行。

### 10.14 2026-11-16 FLOW-CTR-004 Stage Provider Result / Output Registry 影响分析

- 本轮开始前核对远端 `integration` HEAD：`17ca61fad64e87da8c557f80bdfe66cc71b0bfcd`，工作区通过 GitHub 远端文件读取核对；未切换分支。
- 按此前交接任务顺序，完成 FLOW-CTR-004 的定点代码/Schema/Golden 盘点，新增提案：`doc/contract/FLOW-CTR-004-Stage-Provider-Result-Envelope影响分析.md`。提案只分析冻结字段与 L2 实现缺口，不改冻结契约、Schema、Golden 或业务代码。
- 已核实的实现缺口：
  1. `WorkflowTemplate.OutputRegistryVersion` / `ResolvedTemplate.OutputRegistryVersion` 只是字符串快照；没有可供运行时按版本读取的 Stage Output Registry 实例和权威注册项。
  2. `StageExecutionResult` 目前只有状态、`output_ref`、`quality_summary_ref`、诊断/原因引用及自由字符串质量/风险状态；没有冻结 Envelope 要求的 `output_type + schema_version + payload + typed quality_summary + completed_at`。
  3. `ValidateStageExecutionResult` 只校验执行状态与引用非空，不执行按 `(output_type, schema_version)` 的强类型 Payload Schema 校验。
  4. `internal/researchflow/resultvalidation/quant.go` 中的 Registry 是 Quant Result Envelope 消费校验器，不是 Stage Output Registry；两者不能混用或仅因都有 schema version 就视为已实现。
  5. `ResultSummary.quality_status` 仍为任意 string；当前 Schema 也只约束非空字符串。coverage 只有 0..1 数值，没有可复算的分子/分母定义；Summary 资源的身份/版本绑定也未形成完整约束。
  6. `StageExecutionRequest` 没有可信 Principal/CallerRef 字段；CreateRun 的 caller identity 虽来自可信 RequestIdentity，但未证明运行时下游 Adapter 可获得同一可信主体。
  7. 目前没有生产 StageExecutor 实现；Quant 已有若干 QNT-CTR-004 Schema/Golden 和领域 DTO，但 QNT-CTR-003、DATA-CTR-005/007 等 deferred 项仍阻断 Factor/行情证据链；现有 Quant 结果读取/验证能力不等于可执行 Stage Provider。
- 责任边界：Researchflow 负责 Envelope/Output Registry/通用 Validator/Stage 执行门禁；各领域模块负责自己的 Stage Code、Payload Schema/Golden、质量语义和真实 Provider；Server 负责按冻结版本装配 Registry、可信 CallerRef 与生产 Executor，不得在 Server 重算领域逻辑。
- 下一步顺序与门禁见提案第 5 节。先完成正式契约决策/机器 Schema 和注册表，再接入各模块生产 Facade/Provider，最后做真实 MariaDB 跨模块端到端；不能先造空壳 Executor 或猜测质量枚举/coverage 分母。
- 本轮没有 Go 代码、测试、Workflow、冻结契约、Schema 或 Golden 变更；文档提交不触发 CI，不更新最近代码验收 SHA。当前最近代码验收仍为 `d1b58b49fee809c5204fafe3409dc36da9615794` / Run [43790923451](https://github.com/Kasign/stock_data_provider/actions/runs/43790923451)（#81）。


### 10.15 2026-11-16 新闻 Provider 时间字段映射与真实样本核验边界

- 开始前核对 `integration` HEAD：`9a37aa6a3646471769bada057419619cc1ebc67d`；本轮只在 `integration` 工作，未切换分支。
- 按项目负责人确认的方向，复核 News 搜索 Provider、MarketData 东方财富公告 Adapter、对应测试夹具和字段文档。新增规范章节《新闻Provider接入与诊断规范.md》第 16 节，记录逐 Provider 原始字段路径、现有解析行为、时区/日期边界、固定样本证据和真实数据缺口。
- 当前代码事实：
  - Bocha 使用 `data.webPages.value[].datePublished`；Tavily 使用 `results[].published_date`；Brave 仅将 `published_date` 作为发布时间候选，明确不使用 `page_age`；SearXNG 依次读取 `publishedDate/published_date/published_at`。
  - News 候选保留 `PublishedAtRaw`；公共标准化当前只解析带明确时区的 RFC3339Nano、RFC1123Z、RFC822Z。日期级或无时区值不推断为精确时间。
  - 东方财富公告 Adapter 解析 `display_time`，按 Asia/Shanghai 处理无偏移的本地时间；`eiTime`、`sort_date`、`notice_date` 仅保留，不作为自动回退。纯日期不会变成零点时间。
- 重要的证据限制：
  - 仓库内东方财富样本来自 httptest 固定夹具，不是真实上游响应。夹具里 `display_time` 与 `eiTime` 相同、`sort_date` 晚一分钟，只能证明测试输入与解析行为，不能证明生产字段语义或真实冲突率。
  - News `TestLiveProviderProbe` 默认跳过，只有 `NEWS_LIVE_PROBE=1` 且实际 Provider 配置/凭证齐备才执行；当前测试只记录候选数量和耗时，没有时间字段覆盖率统计。
  - 本轮尝试通过公开文档/网页获取东方财富 `np-anotice-stock` 原始 JSON，网页读取端点不可访问；没有取得可审计的实时响应。因此 `display_time/eiTime/sort_date` 的真实存在率、非空率、解析率、精度分布和冲突率均记为 `not_comparable`，不能拿测试夹具冒充实测。
- 外部定义复核：东方财富 EMQuant 官方文档说明 `datetime` 是展示时间（公告可能仅展示日期，盘后公告可能显示为下一交易日），`eitime` 是生产时间且只含日期；这足以说明不能把两者直接当成首次公众可获取时间，但不等于已完成当前 HTTP Endpoint 的字段语义验证。SearXNG 官方 Result 文档将 `publishedDate` 定义为 datetime；Bocha 文档确认 `datePublished` 字段存在，但当前未取得足以冻结其格式的完整规范。
- 本轮结论：现有 Provider 已有多种原始时间字段和部分标准化能力，但尚无逐字段溯源持久化，也没有真实在线样本覆盖率/冲突率证据。所有搜索/聚合 Provider 仍不得作为 NEWS-CTR-001 `available_at` 来源；不新增 `available_at_estimate`，不新增字段/表，不改 PIT SQL，不启用 B 组成功路径。
- 后续明确任务：在不改变业务语义的前提下，给现有受控只读 Live Probe 增加脱敏聚合时间统计（raw_present、parsed、date_only、unparseable、future、timezone/format 分类），默认仍 skipped、不记录原始时间/标题/凭证/正文；用固定样本测试统计逻辑。取得真实响应后再评估是否需要 Provider-specific parser 或字段溯源实现。若要把时间证据用于新的业务字段或 PIT 边界，必须先走契约决策。
- 本轮仅新增文档，没有 Go 代码、测试、Schema、Golden、Migration 或 Workflow 变更，不触发 CI。文档提交：`b4ee08f996a48dd69215f3ff5c111639ba39a540`；最近代码验收仍为 `d1b58b49fee809c5204fafe3409dc36da9615794` / Run [43790923451](https://github.com/Kasign/stock_data_provider/actions/runs/43790923451)（#81），不能把本次文档 HEAD 描述为新代码 CI 通过。


### 10.16 2026-11-16 News Live Probe 发布时间字段聚合统计

- 在 10.15 已明确的真实样本缺口基础上，为现有可选、只读的 `TestLiveProviderProbe` 增加发布时间聚合统计，不改变 Provider 请求、候选入库、业务字段或 NEWS-CTR-001 语义。
- 每个 Provider 的 Live Probe 现在额外输出：候选总数、原始时间字段非空/缺失数、按现有 Domain 解析器可解析数、仅日期数、无时区本地日期时间数、未知格式不可解析数、明确时区数，以及超过观察时间 5 分钟的未来时间数。
- 日志只输出聚合计数和 Provider Code；不输出原始时间、标题、URL、摘要、响应正文、凭证或 Header。默认仍由 `NEWS_LIVE_PROBE` 控制，未启用时测试继续 `skipped`，不在普通 CI 中触发外网请求。
- 新增固定样本单测，覆盖 RFC3339 +08:00、RFC1123Z、日期级、无时区本地日期时间、不可解析值、未来时间和缺失值；统计函数只返回聚合结果。
- 本项不新增生产 DTO 字段、DB 列/表、Migration、Schema、Golden 或 PIT SQL。它只提供后续真实在线探测时的低风险数据质量观测；不能把搜索 Provider 发布时间升级为 `available_at`。


### 10.17 2026-11-16 Live Probe gofmt 门禁修复

- Integration CI Run [43796272232](https://github.com/Kasign/stock_data_provider/actions/runs/43796272232)（#82）对代码提交 `4ad77c2c813503e05aeb41c541ebaad295ac3a14` 的结果为 failure。逐步日志确认根模块 go vet/test、News/Research 跨模块测试、Worker MariaDB/Redis 生命周期、go-core vet/test、Compose 与 WebUI 检查均通过。
- 唯一失败项是全仓 gofmt：`internal/news/adapters/provider/live_probe_test.go` 的结构体字段对齐不符合 gofmt，并存在多余空行。测试逻辑本身通过；CI 汇总步骤因 GOFMT=failure 而失败。
- 本轮只修正该测试文件格式，不改变统计逻辑、断言、Provider 请求或业务语义。修复提交 `7378b035c88dabb13c6486f7a54d0df2663d96d5` 的 Run [43796597261](https://github.com/Kasign/stock_data_provider/actions/runs/43796597261)（#83）对精确 SHA 全量 `success`：gofmt、Root Go vet/test、News/Research 跨模块测试、Worker Bootstrap MariaDB/Redis 集成、go-core vet/test、Compose、WebUI 语法/单测/镜像构建及最终汇总全部通过。Run #82 仍记为失败，不回写为通过。


### 10.18 2026-11-16 `available_at` P0 重新启动：两表结构提案获确认

- 开始前核对 `integration` 远端 HEAD：`17bac6e9625733faf02d3bf98beb07868a6c4e02`；本轮仅更新契约提案和任务跟踪文档。
- 项目负责人明确同意重新启动 `available_at` P0，并确认以两张追加式表作为结构提案基线：
  1. `news_version_availability_evidence`：绑定精确 `version_id`，保存来源身份/策略版本、字段路径、原始时间文本、UTC 候选值、原始时区、解析器版本、受控证据引用/摘要、`observed_at` 和 `recorded_at`。
  2. `news_version_availability_decisions`：保存审核决策事件、状态、原因码、规则版本、决策分组、被替代决策引用和 `recorded_at`。
- 结构原则：News 内容版本保持不可变；不把 `available_at` 写入 `news_item_versions`，不增加可变 current-state 表；PIT 根据 `system_as_of` 推导当时已记录的审核状态，只有唯一有效 `verified` 证据可提供时间。冲突/撤销/未审核均 fail-closed。
- 已将提案写入 `doc/contract/NEWS-CTR-001-available_at-来源证据解锁影响分析.md` 第 1.9 节，并在 News 任务清单新增一百零八节。
- **本次授权边界**：批准结构提案基线，不代表 NEWS-CTR-001 2.0.0 已冻结；不代表 A 股来源已合格。SSE InfoNet“更新时间”仍为 candidate，需取得官方字段语义、真实响应和个人用途授权证据。
- **未实施**：没有 Migration V10、Go 领域模型/Repository/Provider/PIT SQL、1.0.0 Schema/Golden 修改，也未解除 B 组 SKIP；没有业务代码 CI 结果可新增。代码验收仍以最近精确代码 SHA `7378b035c88dabb13c6486f7a54d0df2663d96d5` / Run [43796597261](https://github.com/Kasign/stock_data_provider/actions/runs/43796597261)（#83）为准。
- 下一步按提案第 1.9.6 节完成两表细节评审，再完成来源语义/授权取证与正式 DEC；未满足来源门禁前不进入数据库或业务代码实施。


### 10.19 2026-11-16 available_at 两表决策与时间精度细化

- 开始前核对远端 integration HEAD：`87c80e910d5f038763ad943b51a36590c568f93e`，并复读三份任务/契约文档及 Research Evidence 存储实现。
- Research `internal/research/storage/repository.go` 的 `formatPIT` 固定使用 `2006-01-02T15:04:05.000000000Z`；Research Migration 对 PIT 时间使用 `CHAR(30) ascii_bin`，不是 DATETIME(6)。因此修订 News 提案：候选时间、observed_at、recorded_at 均使用固定 30 字符 UTC 纳秒格式；非 PIT 审计时间可用 DATETIME(6)。
- 决策表改为每行一条完整状态快照：verified/conflict/unresolved/revoked。单条证据解析失败写入 evidence 的 parse_status/reason_code，不直接覆盖版本既有 verified 状态。
- 决策按 version_id 使用单调递增 decision_seq 排序；同版本写入事务先锁定对应不可变版本行，再分配序号；唯一索引兜底。每条事件通过 supersedes_decision_id 链接前一决策。
- 冲突证据集合由 considered_evidence_ids 规范化 JSON 数组记录，不增加第三张关联表或可变 current-state 表。
- 精度超出纳秒或不能无损表达时拒绝，禁止静默舍入/截断。
- 本轮仅同步提案、News 清单和交接文档；没有修改 Migration、Go 业务代码、Provider、PIT SQL 或 1.0.0 Schema/Golden。来源准入、消费者兼容和正式 DEC 仍是后续门禁。
- 下一步评审 MariaDB JSON 存储/校验、索引和外键策略，再结合来源准入证据形成 NEWS-CTR-001 2.0.0 正式变更决议。


### 10.20 2026-11-16 available_at Evidence 与 Observation 去重及 MariaDB 约束评审

- 开始前核对 integration HEAD：`8b62aea9435ae7b5c33994112b3454d9b7108028`；读取 News Migration V1-V9、News Repository 和 Research PIT 存储实现。
- 确认 `news_observations` 已保存 source_id/provider_code/endpoint_id、Source/Endpoint/Provider Output Policy Version、observed_at、candidate_json 和 retention 生命周期。Evidence 不再复制来源身份/策略字段，改为必填 observation_id + version_id，来源元数据从 Observation 关联读取。
- Evidence 绑定的 observation 必须是实际包含该字段值的响应；验证 Observation 与 version 属于同一 news_id，不能默认使用版本首次 Observation。
- 明确区分 raw_value_hash 与原始响应 response_hash：前者只能证明本地字段值未变，不能证明供应商响应真实性。当前 Observation 是否保留原始响应引用/摘要仍待核实。
- MariaDB 方向：复用现有 JSON、InnoDB 外键和 ascii_bin ID；建议 Evidence/Decision 复合外键保证选中证据及前序决策属于同一版本，considered_evidence_ids 由 Go 层校验数组语义。
- 明确 Evidence 字段保留/清理必须服从 Provider Output Policy，追加式审计不能突破来源授权期限。
- 本轮仅更新结构提案和任务跟踪，没有 Migration、Go 代码或 PIT SQL 变更。
- 下一步：核查 Provider 原始响应证据链及 retention 实际能力；之后完成目标 MariaDB DDL 兼容验证和索引计划，仍须在正式 Schema 2.0.0/DEC 前保持不实施。


### 10.21 2026-11-16 available_at Provider 原始响应证据链核查

- 开始前核对 integration HEAD：`d0390977f7d03fff3153e5f86a1c53b9b15ecbf7`；只读检查 Provider HTTP/Parser、Candidate、Observation、RSS/Atom CollectSource 和 Observation retention。
- `provider/http.go` 会读取受限响应字节，但 `providers.go` 将响应解析成 Candidate 后原始字节即丢弃；Candidate 只有 PublishedAtRaw，没有通用 field_path/raw_value/time-type 证据。
- `news_observations.candidate_json` 是规范化 Candidate，不是完整原始响应；现有 retention 会清空 candidate_json 并保留 tombstone。RSS/Atom 同样只保存 Candidate，不保存原始响应或摘要。
- 结论：当前没有可直接复用的通用 available_at 原始来源证据持久化链路。具体来源准入后，必须在该来源 Parser 边界提取获批字段原始值/路径，绑定实际 Observation，再按 Provider Output Policy 留存；不应提前为所有 Provider 增加泛化 RawResponse。
- 本轮仅更新证据链核查文档、News 清单和交接文档，没有修改 Provider、Candidate、Observation、Migration 或 PIT SQL。
- 下一步继续完成结构层的留存/删除语义和时钟边界评审；来源未准入前不进入代码实施。


### 10.22 2026-11-16 available_at Evidence 留存与到期清理边界

- 开始前核对 integration HEAD：`fd62c0e345eef2573f5be28146a40b13ee246885`；复核 Observation 的 retention_until/payload_purged_at、到期清理事务和 Provider Output Policy 关联。
- 两表结构提案增加 Evidence 留存约束：原始字段、证据引用/摘要和派生时间均不得绕过 Observation 对应策略延长保留；Evidence 与 Observation candidate_json 清理需同一事务，保留最小 tombstone。
- payload 已清理时默认不再输出 available_at；只有来源/Provider Output Policy 明确允许保留派生时间时才例外，不能因历史 verified 决策推断可永久保留。
- 本轮仅更新结构提案、News 清单和交接文档，没有修改 retention Job、Migration 或业务代码。
- 下一步仍需来源级策略证据才能冻结具体保留规则；在此之前采用 fail-closed 默认，不扩大数据留存。


### 10.23 2026-11-16 available_at MariaDB 约束、精度和授权交集复核

- 开始前核对 integration HEAD：`09c753e0a2f1e2a8aceeb2979f883f5d9ee96354`；检查 News Migrations、MariaDB Repository 集成测试、Source/Endpoint/ProviderOutputPolicy、Ingest retention 计算和 Observation retention 清理事务。
- 当前 News Migration 已使用 InnoDB/JSON/外键/复合索引/DATETIME(6)；集成测试要求 NEWS_MARIADB_TEST_DSN，但 docker-compose.yml 未固定 MariaDB 镜像/版本，本轮无可用真实 DB DSN。因此只完成静态结构评审，不声称 DDL 通过目标版本验证。
- Evidence/Decision 约束方向：Evidence 以 (version_id,evidence_id) 唯一键供 Decision 复合 FK；Decision 以 (version_id,decision_seq) 唯一键及 supersedes 自引用/唯一后继约束避免分叉。considered_evidence_ids JSON 的数组元素不能由普通 FK 逐个引用，必须在事务内由 Go 层验证。仅现有单列主键不能强制 observation 与 version 属于同一 news_id，需复合父键/冗余 news_id 或事务锁定校验。
- 重要修正：Research PIT 用 CHAR(30) 九位 UTC；News Observation、Version、Snapshot PIT 当前均使用 DATETIME(6)，Snapshot Repository 明确比较到微秒。此前 1.10 “News 新表统一 CHAR(30) 纳秒”的表述不能视为已冻结；须先由 News/Research 契约确定统一精度和边界归一化，不能局部引入更高精度。
- Ingest 的最终 retention 来自 Source MaxRetention、Endpoint MaxRetention 和 Provider MaxRequest/MaxPersistenceRetention 授权交集，写入 Observation.retention_until。Evidence 不得延长；未来 Evidence 载荷清理必须与 Observation candidate_json/tombstone 在同一事务完成。Policy 撤销/到期后既有 Evidence 的可读/留存行为及 available_at 字段级 StorageMode/ContentKind 尚未定义。
- 本轮仅更新设计提案、News 验收清单、Integration 交接文档；无 Migration/Go/Provider/PIT SQL 变更。
- 下一步应冻结跨模块 PIT 精度契约与字段级留存策略；获得真实 MariaDB 版本/DSN 后再执行 DDL 和索引 EXPLAIN。上述门禁未完成前保持 proposal-only。

### 10.24 2026-11-16 available_at 秒级精度与字段级授权提案

- 开始前以 `integration` 为唯一工作分支，基于上一轮 `ff4eb3677284e7613001545a6afaa80f565fe34c` 的 available_at 评审记录继续；本轮只处理契约/任务/交接文档，不实施 Migration 或业务代码。
- 项目负责人明确 available_at 业务精度采用秒级，不需要微秒/纳秒。契约提案现明确：
  - API 使用 UTC RFC 3339 秒级形式；
  - News 新增候选业务时间建议使用 `DATETIME(0)`；
  - Research 现有 `CHAR(30)` 九位格式可将秒级值编码为尾部九个零，不代表来源具备纳秒精度，也不要求改造 Research 通用 PIT 存储；
  - 非零小数秒来源值不静默截断/四舍五入，暂按 `unsupported_precision` 处理。
- 重要边界：秒级只适用于 `available_at` 业务时间。News 现有 `observed_at` / `recorded_at` / `system_as_of` 等系统审计与 PIT 边界保留既有微秒精度；Research 的系统 PIT 时间继续使用现有固定格式。两类时间不要求同精度。
- 核查 `internal/news/domain/policy.go`：现有 ContentKind 为 metadata/title/snippet/abstract/full_text/attachment，StorageMode 为 none/transient/persistent_metadata/persistent_excerpt/persistent_full_text。现有模型无法区分普通 metadata、来源时间原始证据和规范化派生 available_at 的独立持久化授权。
- 提出但未实现两个授权类别：`source_time_evidence`（raw_value、field_path、timezone、evidence_ref/hash）与 `derived_availability_time`（规范化秒级候选值及审核后派生值）。需要分别经过 Source / Endpoint / Provider Output Policy 授权；不得从 metadata 许可自动继承。
- 策略生命周期提案区分自然到期、明确撤销、明确 grandfather：无来源授权文本时默认停止新采集并对既有 Evidence 读取/PIT 输出 fail-closed；只有明确授权既有数据在原 retention 内继续使用时才允许例外。现有 Observation 只存策略版本引用和最终 retention_until，尚不足以推导各策略独立到期/撤销影响范围。
- 本轮没有修改 Go、Migration、Provider、Schema、Golden、PIT SQL；没有目标 MariaDB DSN/版本验证，不新增 DDL 兼容性结论。
- 下一步按顺序：①冻结 News / Research API 秒级表示、PIT 边界及存储映射；②决定字段级 ContentKind / StorageMode 和 Provider Output Policy 独立留存许可；③取得真实来源授权文本，明确自然到期、撤销及 grandfather 规则；④完成这些契约门禁后，再进入目标 MariaDB 版本 DDL/Repository 集成测试和正式 Schema/DEC。来源准入仍未通过前不得实施 Migration 或 Provider 采集。

#### 10.24 补充：Schema / Research Adapter 兼容性核查

- 核实 1.0.0 NEWS-CTR-001 Schema 的通用 `nullableDateTime` 允许 1～9 位小数秒，并被多个时间字段复用。旧 1.0.0 Schema / Golden 不得原地修改。
- 未来 2.0.0 只为 `available_at` 定义秒级专用 nullable schema；其他 `published_at`、`system_observed_at`、`fetched_at`、`normalized_at` 保留各自现有精度，不修改通用定义。
- Research News Adapter 当前将 AvailableAt 映射为 `time.Time`，校验 UTC、非空和 cutoff，但未校验小数秒为零；正式接入 2.0.0 时必须拒绝非整秒值。
- Research `formatPIT` 固定输出九位小数；整秒值可编码为 `...SS.000000000Z`，无需单独迁移 Research 表。
- Research 纳秒 `system_as_of` 传入 News、而 News Snapshot 现有数据库边界为微秒的转换仍是独立待评审问题，不得因 available_at 采用秒级而降低系统 cutoff 精度。
- 本补充仅同步评审证据，没有修改 Schema / Golden / Adapter / Migration / PIT SQL。



### 10.25 2026-11-16 available_at 字段级授权与策略生命周期评审

- 开始前确认 `integration` 与 `d28eda3df2fcdc5c9c26e9cac639bb9c71c394c6` 完全一致（compare status=identical，ahead/behind=0）；本轮只更新契约/任务/交接文档。
- 复核 `internal/news/domain/policy.go`：现有 ContentKind 为 metadata/title/snippet/abstract/full_text/attachment；StorageMode 为 none/transient/persistent_metadata/persistent_excerpt/persistent_full_text。当前 `contentCompatibleWithStorage` 的 persistent_metadata 仅允许 metadata/title，直接在策略配置写新字符串并不能获得有效授权。
- 形成最小提案：新增 `source_time_evidence` 与 `derived_availability_time` 两个独立 ContentKind；暂不新增 StorageMode，复用 none/transient/persistent_metadata。两个 ContentKind 分别经 Source、Endpoint、Provider Output Policy 授权交集，分别计算留存期限；不得继承 metadata 许可。
- 复核当前策略行为：ExpiresAt 到期即不 active；revoked/pending 状态拒绝。策略模型没有 grandfather 范围字段。NEWS-CTR-002 已有当前策略指针下的原子 Evidence 授权读取能力；available_at 未来读取必须复用，不可绕过，但新 ContentKind 尚未纳入读取授权契约。
- 明确默认生命周期：自然到期/明确撤销/策略收紧后停止新采集；读取和 PIT 输出 fail-closed；载荷清理依授权条款执行且不得超过 retention。Grandfather 仅在授权文本明确允许既有数据、用途和截止时间时生效。retention_until 不等于读取权。
- 本轮仅更新 NEWS-CTR-001 影响分析、News 验收清单和 Integration 交接；无 Go、Migration、Provider、Schema、Golden、PIT SQL 变更。没有目标 MariaDB DSN/版本验证，不新增 DDL/CI 结论。
- 下一步：①项目契约 Owner 审批两个 ContentKind 与兼容矩阵；②确认 NEWS-CTR-002 对新类别的读取授权/撤销清理适用性；③取得具体来源授权文本；④门禁通过后才更新 Policy、Schema/DEC 与数据库实现。继续保持 proposal-only。

### 2026-11-16 available_at MVP 方向纠偏（优先于 10.2 历史两表方案）

项目负责人明确要求先定义简单可落地的结构，数据源缺失单独处理。此前两张追加式 Evidence/Decision 表、双 ContentKind、grandfather 和完整决策状态机均降为历史提案，不再作为 MVP 基线。

**最小结构**：在 `news_item_versions` 上承载 `available_at`、`available_at_raw`、`available_at_field_path`。现有 `observation_id` 已关联 `news_observations`，可追溯 source_id、endpoint_id、provider_code 和策略版本，不重复建表/复制来源字段。只保存必要的原始时间文本与字段路径，不持久化完整 Provider 响应。

**规则**：秒级业务精度，存储沿用 `DATETIME(6)` 并在应用层拒绝非零小数秒；没有合格来源、来源语义未确认、时区不明、解析失败或冲突时 `available_at=NULL`。不得使用 published_at、fetched_at 或 observed_at 替代。Research 已有 nullable available_at 存储与消费校验，News DTO 也已有该字段；保持 JSON 形状时不预设必须升级 Schema 主版本。

**当前状态**：结构提案已收缩并记录；本轮没有实施代码。下一项独立任务是核验中国 A 股真实来源的官方字段语义、真实响应样本、明确时区、内容版本绑定和个人用途授权。来源通过前，不新增 Migration、不改 Provider、不改 PIT SQL、不解除 B 组 SKIP。来源通过后再一次性完成三字段写入、Get/List PIT 过滤、Research 映射及 MariaDB 端到端验收。

**工作规则修正**：不得再以两表设计、授权状态机、跨模块时间精度统一或泛化 RawResponse 作为前置任务；若来源核验失败，只记录具体来源阻塞，不重新扩张结构模型。

### 2026-11-16 DOC_PUBTIME 候选核验结论

已直接核对深交所官网 JR/T 0313—2024：附录 A 定义 `DOC_PUBTIME`（可选，文件发布时间）、`DOC_VER`（可选，版本）、`DOC_VAL`（必选，有效性）及 `RES_TYPE`（含更新关系）。因此标准字段存在性已确认；但标准没有把发布时间定义为首次市场可获取时刻，字段也不是必填，不能直接映射为 PIT `available_at`。

已核对上证所信息官方服务资料：公告文件服务属于 API 技术传输产品；信披数据服务须提交申请、审批并签署服务协议；公开价目表公告文件服务为 6 万元/年。沪深交易所联合授权声明要求机构或个人接收/使用交易所证券信息申请许可。

**状态：标准字段 candidate 已确认；实际接口/时间语义/个人许可未确认，来源准入 blocked。** 未取得用户已有许可前，不接入上证信息/交易所服务，不调用非公开接口，不将网页可浏览视为允许程序化采集与持久化。若已有适用许可，再以真实样本核验时区、首次可获取语义、版本和更正关系；否则转向明确允许个人程序化使用的替代来源。不要重复搜索 JR/T 0313 标准，也不要因此扩大 MVP 结构。

本轮没有 Go、Migration、Provider、PIT SQL 变更；`available_at` 仍保持 NULL，现有 SKIP 不变。

### 2026-11-16 巨潮资讯网替代来源候选

交易所商业公告 API 许可阻塞后，找到巨潮资讯网官方公告列表字段 `announcementTime`（页面显示“公告时间”）。巨潮资讯网由深圳证券信息有限公司运营；深交所 2026 年修订指南明确了直通/非直通公告送达符合条件媒体的时点规则。当前公开页面样本多显示 16:00，但这不能证明该字段等于逐公告首次公开可获取时间；网站公开浏览也不自动证明自动化采集/持久化许可。

**状态：secondary candidate / not verified。** 下一步用少量真实样本核对早间、午间、盘后直通及非直通公告的 `announcementTime`、规则媒体送达时点、首次可访问时间，并检查更正/替换版本行为和个人自动化访问许可。验证前不接入 Provider、不生成 available_at、不新增 Migration 或修改 PIT SQL。


### 2026-11-16 available_at 内部标准纠偏与本轮继续任务

项目负责人明确：`available_at` 是 Kasign 内部字段，应按 PIT 研究诉求定义；外部数据源众多，不应把任何一个来源字段作为唯一标准。已将该原则同步到 NEWS-CTR-001 影响分析和 News 验收清单。

内部唯一语义：**具体内容版本首次公开、可被市场参与者获取的时间**。各 Provider Adapter 将不同来源的字段名、格式、时区/偏移、发布流程及版本标识转换为内部 UTC 秒级值。来源资格逐一核验，不要求所有来源都具备该字段，也不设全局权威源；不能证明语义/版本或使用许可的来源不参与写入，无法解释的多源冲突保持 NULL。`available_at_raw` / `available_at_field_path` 只追溯实际产生所选值的 Observation，不新增原始响应存档或 Evidence/Decision 表。

本轮补充检索再次观察到巨潮公告列表的 `announcementTime` 样本普遍显示 `2026-11-13 16:00`，官方页面确认其为“公告时间”，但这不足以证明每条公告的首次公开可获取时刻。巨潮公开页面免责声明没有明确授予自动化批量访问、持久化和再利用许可；官方数据 API 的公开文档可见与实际数据接口授权是两回事，社区整理资料报告数据端点存在未授权访问响应，不能作为绕过授权的依据。当前候选仍为 `not verified / blocked for ingestion`。

下一步继续围绕同一目标：找可合法使用的来源样本，验证原始时间字段、版本身份和首次公开可获取语义；巨潮只有在样本及许可门禁通过后才可进入 Provider。当前不改代码、Migration、PIT SQL，不解除 B 组 SKIP，不将任何 `announcementTime` 写入 `available_at`。

### 2026-11-16 available_at 外部时间精度归一化与 PIT 保守边界

项目负责人确认将外部时间格式尽量归一化，但补齐格式不得制造比来源证据更精确、或早于真实可获取时刻的 PIT 时间。适配层应同时保留来源原始时间文本、字段路径和实际时间精度；补零只用于统一解析/展示格式，不代表来源提供了被补齐的时间部分。

**归一化与 PIT 使用规则：**

- 来源提供可信日期、时分秒，且字段语义、时区/偏移、具体内容版本及使用许可均通过准入审核：按来源时区解释后转换为 UTC 秒级时间，可写入内部 `available_at`。
- 来源精确到分钟：不得简单补 `:00` 后作为 PIT 时点，因为真实时刻可能落在该分钟后续 59 秒内。通过其他准入审核后，采用该分钟结束时刻作为保守可用时点（例如来源本地时间 `10:32`，按其已确认时区解释为 `10:32:59`，再转换 UTC）；这会最多延迟 59 秒，但不会因分钟补零而提前使用信息。原始精度仍标记为 minute。
- 来源只有年月日：可以解析并保留日期级候选信息，但不能补成 `00:00:00` 写入严格 PIT 的 `available_at`。当天真实公开时刻未知，午夜补零会人为制造过早时点。
- 来源只有年月或年份：可以保留为低精度辅助元数据，不生成严格 PIT `available_at`。
- 来源只有时分秒但没有日期：无法组成完整时间点，不生成 `available_at`。
- 来源未提供时区/偏移：只有在官方字段规范或稳定来源约定明确规定时区时才可解释；不得默认套用服务器时区、用户时区或市场时区。
- 来源语义不明、版本无法绑定、解析失败、时区无法确认或多来源冲突无法解释：`available_at=NULL`。格式补齐不能替代任何来源准入条件。

**精度标记建议：** 在 Provider 适配/候选处理层保留 `second`、`minute`、`date`、`month`、`year` 等实际来源精度；不要求为此新增数据库表或公共 API 字段。秒级内部业务精度保持不变。分钟级候选即使按分钟末尾形成保守 PIT 时点，也必须保留 minute 精度，不能伪装成来源秒级数据。

本规则只定义内部归一化和 PIT 安全边界，不代表任何当前候选来源已通过语义、授权或版本准入；不改变巨潮候选的 `not verified / blocked` 状态，不解除 B 组 SKIP。本次仅更新文档，不实施 Provider、Migration、Schema/Golden、Repository 或 PIT SQL 变更。

### 2026-11-16 巨潮公告时间真实页面样本补充核验

本轮仅通过巨潮资讯网公开网页及其公开可访问的公告 PDF 核验页面样本；未调用网页内部未公开 API、未绕过访问控制、未批量抓取或保存公告正文。

**页面样本观察（公告列表显示时间，精度到分钟）：**

| 页面/分类 | 日期 | 股票代码 | 公告标题摘要 | 页面显示时间 |
|---|---|---:|---|---|
| 公告查询（债券等） | 2026-11-13 | 002736 | 国信证券 2025 年第五期永续次级债券 2026 年付息公告 | 10:00 |
| 公告查询（债券等） | 2026-11-13 | 000987 | 越秀资本 2025 年第四期公司债券付息公告（品种二） | 09:32 |
| 公告查询（债券等） | 2026-11-13 | 000776 | 广发证券债券付息公告 | 09:16 |
| 公告查询（债券等） | 2026-11-13 | 000877 | 天山股份相关关注公告 | 08:38 |
| 公告查询（债券等） | 2026-11-13 | 000686 | 东北证券债券兑付公告 | 08:14 |
| 公告查询（债券等） | 2026-11-12 | 000498 | 山东路桥公司债券票面利率公告 | 12:58 |
| 公告查询（债券等） | 2026-11-12 | 300145 | 南方泵业公司债券相关公告 | 11:48、10:02 |
| 公告查询（银行间债务融资工具） | 2026-11-13 | 000402 | 金融街 2023 年度第八期中期票据付息及回售兑付公告 | 03:45 |
| 公告查询（银行间债务融资工具） | 2026-11-13 | 000402 | 金融街 2026 年度第七期中期票据发行情况公告 | 16:00 |

页面来源：
- [巨潮公告查询（一般公告）](https://www.cninfo.com.cn/new/commonUrl/pageOfSearch?checkedCategory=category_gszq_szsh&url=disclosure/list/search)
- [巨潮公告查询（债券/银行间相关分类）](https://www.cninfo.com.cn/new/commonUrl/pageOfSearch?checkedCategory=category_qtrz_szsh&url=disclosure/list/search)
- [巨潮最新公告列表](https://www.cninfo.com.cn/new/commonUrl?url=disclosure/list/notice)

**样本结论：**

1. 页面显示值确实存在不同日内时刻，不是统一的 16:00；当前可见样本均为分钟粒度，没有从页面结果证明秒级精度。
2. 这只能证明列表字段具有日内时间差异，不能证明该时间是公告首次向市场参与者公开可获取的时刻。页面未给出字段的事件定义、时间生成规则或可审计的首次可访问时间。
3. 页面样本未能区分直通/非直通流程，也没有与符合条件媒体的实际送达时间建立逐公告对应关系。不能仅凭时间落在盘前/盘中/盘后推断披露流程。
4. 对更正、补充、更换文件的版本身份与先后关系，本轮没有取得足以确认的官方版本字段规范或成组样本；不得把同一公告标题或唯一 PDF 文件地址视为已证明的版本链。
5. 另一个可访问 PDF 样本为上海银行公告：巨潮静态文件路径日期为 2026-11-14，公告正文落款为 2026-11-13，列表显示时间为 2026-11-13 16:00。文件路径日期、正文落款日期和列表显示时间是不同信息；路径日期不能被解释为首次公开时间，也不能用来验证列表时间的真实性。
6. 巨潮页面页脚将“数据 API”“数据平台/数据商城”列为独立服务入口；公开网页可浏览本身不等于授予自动化访问、批量采集、持久化及再利用许可。现有页面免责声明主要是准确性/责任声明，没有给出足以确认本项目所需程序化使用范围的明确授权文本。

**当前判定更新：** `announcementTime` 从“公开页面样本普遍为 16:00”的初步观察，修正为“公开页面样本存在多种分钟级日内时间”。它仍然只是 `secondary candidate / not verified`，准入 1（首次公开可获取语义）、版本绑定和个人程序化使用/持久化许可均未通过。不得把分钟值按分钟末尾规则转换成 `available_at`，因为分钟末尾规则只解决已合格时间字段的精度保守性，不能弥补字段语义和授权缺失。

本轮没有调用非公开接口，没有接入 Provider，没有修改 Migration、Schema/Golden、Repository 或 PIT SQL；`available_at` 继续保持 NULL，B 组 SKIP 不变。下一步不重复收集普通页面时间样本；只有取得官方字段语义/授权文件，或取得可审计且明确授权的来源响应与首次可访问证据后，才继续该来源准入。否则转向其他合法来源候选。

### 2026-11-16 交易所公告确认发布事件作为来源取证线索

本轮补充核对上交所官方《科创板上市公司自律监管指南第1号——信息披露业务办理（2025年4月修订）》及上交所关于直通车披露流程的官方说明：

- 指南规定，上市公司通过上交所信息披露系统提交公告并确认发布后，早间/非交易日时段公告通过上交所网站即时披露；午间/盘后公告在相应披露时段内通过网站披露。
- 指南区分直通车与非直通车流程，并要求非直通车公告经过审核、反馈修订（如有）和再次确认发布。
- 上交所直通车业务说明亦描述了确认发布与网站披露时间安排，并要求错误、遗漏或误导内容通过补充/更正公告纠正。

官方资料：
- [上交所科创板上市公司自律监管指南第1号——信息披露业务办理](https://www.sse.com.cn/lawandrules/guide2025/stock/kcbxxpljg/c/c_20251211_10801386.shtml)
- [上交所关于上证直通车业务答记者问](https://big5.sse.com.cn/site/cht/www.sse.com.cn/aboutus/mediacenter/hotandd/c/c_20150912_3988653.shtml)

**对 Kasign 的意义：** “公告确认发布/上交所网站实际上网”是比公告正文落款日期、发布时间标签或系统抓取时间更接近内部 `available_at` 语义的事件候选。未来向交易所或其授权数据服务商核实字段时，应明确询问是否能提供逐公告的确认发布时间/实际上网时间、对应公告版本 ID、直通/非直通状态及更正/撤回关系。

**证据边界：** 上述指南只证明官方披露流程及其事件顺序，不证明当前公开页面或已获授权 API 暴露该事件时间字段，也不证明项目已有 EKEY/数据服务授权。指南中办理系统使用上市公司 EKEY；不能把公司侧操作时间当成公众首次可获取时间，除非官方明确证明其与网站实际上网事件一致。该线索因此标记为 `official workflow evidence / field not exposed / access not authorized`，不是已准入来源。

当前没有可直接用于本项目的合格字段或授权响应；不接入公司侧披露系统、不调用非公开接口、不新增 Provider/Migration，不修改 PIT SQL。后续只有取得授权数据产品的字段字典及逐公告真实响应，才能验证该事件是否可映射为 `available_at`。



## 2026-11-16 项目负责人新增授权：available_at 内部链路继续开发

### 授权与范围

项目负责人明确授权：在唯一工作分支 `integration` 直接实施 News / Research 的 `available_at` 内部链路，包括必要的 Migration、Repository、DTO、Adapter 和测试。此授权覆盖此前交接文档中“Integration 没有新的业务领域开发任务”的旧结论；本阶段不切分支、不合并分支。

执行边界：
- `available_at` 仍为某个具体内容版本首次向约定市场参与者公开可获取的时间，不允许由 `published_at`、`observed_at`、`fetched_at` 或抓取时间推导。
- 业务精度为秒级；明确时区的分钟级值按分钟末秒保守归一化，并保留 minute 精度；日期/月/年、无时区和亚秒值不生成有效候选。
- Parser 的 parsed 仅表示可解析，不表示来源语义、授权、版本绑定或留存审核通过。
- 两张追加式表仍为结构基线：`news_version_availability_evidence` 与 `news_version_availability_decisions`。Evidence 绑定精确 Version 和实际 Observation，来源及策略元数据复用 Observation。
- 不把现有任何 Provider 或 CNINFO / SSE 页面字段映射为有效 available_at；不回填、不解除 B 组 SKIP、不绕过 PIT fail-closed。
- Evidence 原始值、派生值、hash/ref 的持久化必须服从字段级 Source / Endpoint / Provider Output Policy 授权和 retention。授权未覆盖时只允许实现解析/拒绝路径，不得持久化受限内容。
- 不原地修改 NEWS-CTR-001 1.0.0 Schema/Golden；公开契约变更仍按版本治理。

### 本轮首批代码

- 新增 `internal/news/domain/availability_time.go` 和 `availability_time_test.go`。
- 支持秒级 RFC3339、明确时区的分钟级保守末秒归一化；记录 raw value、时区/偏移、precision、status、reason_code。
- 日期/月/年、无时区、亚秒、无效格式均 fail-closed，不产生候选时间。
- 隔离 Go 测试通过；尚未运行仓库根模块全量测试或 MariaDB 集成，不能标记 CI passed。

### 下一项明确任务

1. 定点完成 Evidence / Decision Domain 与 DDL：按 1.10～1.15 现有设计，解决 JSON 列校验、版本/Observation 同主体约束、decision_seq 串行化、复合外键、留存清理联动、目标 MariaDB 版本和秒级 available_at / 微秒系统时间边界。
2. 实现 News Migration / Repository / PIT 查询还原与 DTO 传递；所有无 verified 决策、证据清理、授权过期/撤销和冲突情况均返回 NULL。
3. [x] Research News Adapter 已增加 available_at 整秒、available_at 不晚于 system_observed_at 的校验；Importer 和 Authorizer 均 fail-closed，新增测试覆盖亚秒和时间顺序冲突。
4. 新增 News / Research MariaDB 真实集成测试；不重复 A 组已通过用例。
5. 来源准入仍是独立 blocked/shelved 项；只有真实字段语义、授权、版本绑定及留存证据齐备后，才允许 Provider 映射和 B 组成功路径。

下次继续前先查询 integration 最新 HEAD/Actions，再按本节任务推进；本节优先于本文此前“当前没有可推进 Integration 业务领域开发任务”的旧快照。


### 2026-11-16 Research 消费门禁首批实现

- 已在 `internal/research/adapters/news/evidence.go` 增加 News `available_at` 整秒校验，以及 `available_at <= system_observed_at <= system_as_of` 顺序校验。
- Importer 不合格时不调用 Research AddEvidence；Authorizer 重新查询 News 后也执行同样约束，避免已存 Evidence 被异常精度/顺序绕过。
- `internal/research/adapters/news/evidence_test.go` 新增亚秒和 available_at 晚于 system_observed_at 的拒绝用例。
- 此项不修改 News 的现有 NULL 输出，不产生任何真实来源 available_at；B 组仍保持 shelved。
- 本轮第二批代码提交及 CI 状态以后续精确 SHA/Run 为准。


### Run #85 格式门禁排障

- 精确 SHA：`ea79dc5503f36280af2b944d3557fd0fd4ccb08f`。
- Run [43810902921](https://github.com/Kasign/stock_data_provider/actions/runs/43810902921) 失败原因仅为 `internal/research/adapters/news/evidence_test.go` 中一条匿名函数复合字面量未按 gofmt 展开。
- 同一 Run 的根模块 vet/test、News/Research 跨模块 MariaDB 逐用例、Worker Bootstrap MariaDB/Redis、go-core vet/test、Compose 和 WebUI 检查均 success。
- 已按日志 diff 修复格式；Run #85 不得标记通过，等待新 SHA 的全量 CI。
- CI 使用的 MariaDB 镜像为 `mariadb:11.4`，日志实际版本 `11.4.13-MariaDB`。后续 Migration DDL 测试可以使用该已知版本；生产数据库版本仍需核实。


### 2026-11-16 News V10 Evidence / Decision 存储结构首批落地

- 在 Migration V9 后新增 V10；V1～V9 保持不变。
- V10 新增 `news_version_availability_evidence` 和 `news_version_availability_decisions` 两张追加式表。
- Evidence 通过复合外键约束 `version_id + news_id` 与 `observation_id + news_id`，避免跨 News 主体错绑；Decision 的 selected Evidence 和前序 Decision 使用同版本复合外键；唯一后继约束避免决策链分叉。
- 候选业务时间为 `DATETIME(0)` 秒级；系统观察、记录和 retention 使用 `DATETIME(6)`。
- `requiredNewsSchemaTables` 与 Migration 静态测试、MariaDB 集成迁移测试已同步到 V10。
- Integration CI 的 MariaDB 服务是 11.4，Run #85 日志实际版本 11.4.13。V10 DDL 必须在后续精确 SHA CI 中实际执行成功，不能仅凭静态测试关闭。
- 本轮只建立 schema，不新增 Provider 映射、不写 Evidence/Decision 记录、不改现有 PIT 输出；字段级授权与来源准入继续 fail-closed。


### V10 首轮 CI 结构错误修正记录

- Run #87 对提交 `b384a1ee1b68313c9af195202732de669a2a6723` 的失败发生在 Go 解析阶段：V10 插入位置少关闭 V9 Migration 对象，导致 migrations.go:269 报告 mixture of field:value and value elements in struct literal。
- 该 Run 未进入 Migration DDL 的真实 MariaDB 验证，不能据此判断 SQL 成功或失败。
- 已补齐 V9 对象闭合并移除多余闭合；修复后的精确 SHA 及新 CI 结果待记录。


### 2026-11-16 Availability Evidence / Decision Domain 首批实现

- 新增 `internal/news/domain/availability_evidence.go` 与单元测试。
- Evidence Validator 检查 Version/Observation 绑定标识、字段路径/解析器版本、精度/解析状态、UTC 时间顺序、候选值整秒及不晚于 Observation、Retention 和 payload purge tombstone。
- Decision Validator 检查 verified/conflict/unresolved/revoked 状态形状、selected Evidence 必须包含在 considered 列表、列表显式非 NULL/排序/去重、decision_seq 与 supersedes 关系。
- 该层只定义并校验领域记录，不提供写入入口，不代表任何来源已通过授权或可产生 verified 决策。
- 隔离 Go 测试通过；全仓 CI 尚待本提交精确 SHA 验证。


### V10 与 Domain CI 结果、PIT Resolver 后续

- Run #88 `43811921861` 对精确 SHA `ed4abae47509c7e7fd0caf3a0117681eab36a90b` 全量 success：gofmt、root vet/test、News/Research MariaDB 跨模块测试、Worker Bootstrap MariaDB/Redis、go-core、Compose、WebUI 全部通过。V10 迁移在 MariaDB 11.4 CI 服务中随 News Storage 集成测试实际执行成功。
- Run #89 `43812146279` 对 `b8a847b5bcdfec626393aedd60f09ee7f60bf8d5` 失败原因仅为新 Domain 文件 gofmt 对齐；不是逻辑测试或编译错误。已按日志修复，修复后的精确 SHA 另行验收。
- 当前下一步是将“最新 system_as_of 可见 Decision → selected Evidence”解析接入 News Evidence PIT 查询；只有最新决策状态为 verified、Evidence 未清理且 retention 有效、候选时间不晚于 Observation 和 system_as_of 时返回非空 available_at。冲突/未决/撤销/缺失/过期/清理均返回 NULL。


### News PIT Resolver 的原子复核补充（2026-11-16）

- Run #90 `43812913297` 失败暴露出原子授权重读路径仍复用扩展后的 `evidencePITSelect`，但只传入旧的 4 个参数；该路径现补齐 5 个 system_as_of 子查询参数。
- `GetEvidence` 在原子授权事务中再次读取同一版本当前可见的 `available_at`，并与事务外 PIT 预检值比对；若决策状态/候选值发生变化则 fail-closed。
- PIT SQL 同时要求 raw_value 仍存在、Evidence/Observation 均未清理且当前 retention 有效；最新 Decision 非 verified 时不回退到旧 verified 决策。
- MariaDB 集成用例覆盖 verified → unresolved → re-verified 的 system_as_of 历史切换，以及 payload purge 后 available_at 回到 NULL。
- 上述修复的精确 SHA 和新 CI 结果待记录；Run #90 不得标记通过。


### PIT 原子复核 CI 排障记录

- Run #91 `43813886523` 对提交 `6ac0d43510e1b7e0c0d581f154d43425077e0cb4` 失败：`evidence_authorization_read.go` 调用了 `sameAvailabilityTime`，但 helper 未实际写入文件；这是本轮补丁条件误判（检测到了调用点，误以为 helper 已存在）。
- 已补充 helper 定义，按 nil/非 nil 一致性、时间点相等和整秒精度比较。
- Run #91 未进入根模块编译后的测试阶段；修复提交需重新执行完整 CI。


### 2026-11-16 PIT 决策读取端完整性补强

- 复核发现 PIT SQL 仅检查最新决策 `status='verified'`，未再次确认 `selected_evidence_id` 属于 `considered_evidence_ids`；Domain Validator 不能替代读取端对异常历史记录的防御。
- `internal/news/adapters/storage/evidence_query.go` 已新增 MariaDB `JSON_CONTAINS` 条件；最新 verified 决策的选中证据不在 considered 集合时，不返回 `available_at`。
- `internal/news/adapters/storage/availability_pit_integration_test.go` 新增异常决策真库夹具，验证上述情况 fail-closed。
- 代码提交：`65954360b7db2605546047fb8ea69e6a47d99eb8`；测试提交：`805e79159fe5e29e6919b06e8b42dfd995a19032`。当前尚未取得精确 SHA 的全量 CI 结果，不能标记通过。
- 测试夹具是内部一致性测试，不是来源证据；不得据此创建生产 Evidence、映射 Provider 或解除 B 组 SKIP。
- 后续先检查该 SHA 的 Integration CI。通过后继续检查 Decision 序列化/追加写入的事务边界；任何 Evidence 原始值持久化仍须先满足字段级授权与 retention。


### PIT Decision 读取端完整性修正（2026-11-16）

- 读取端条件要求最新可见决策 `status='verified'`，并且 `selected_evidence_id` 必须属于 `considered_evidence_ids`；使用 MariaDB `JSON_CONTAINS` 实现，不依赖 Domain Validator 单层保护。
- 对应真库集成测试新增异常 verified 决策（selected 不在 considered 集合），预期 PIT 返回 NULL。
- 有效 SQL 修复提交为 `d5da0853ae5cd5cd7f15094fa04b4d3028b5cce0`；测试提交为 `805e79159fe5e29e6919b06e8b42dfd995a19032`。`6595436` 曾产生截断 SQL 字符串，已从 `6ac0d43` 的完整文件恢复并修复；不得把该提交视为有效版本。
- 当前查询 `d5da085` 的 Combined Status 与 PR workflow runs 均为空；CI 未验证，不能标记通过。
- 下一项仍按本节原任务：检查并实现 Decision 追加写入的并发串行化/序号与 supersedes 连续性；不扩展到其他领域，不创建真实来源证据，不解除 B 组 SKIP。


### 2026-11-16 Decision 追加写入事务串行化已实现

- `MySQLRepository.AppendAvailabilityDecision` 已在 `internal/news/adapters/storage/mysql_repository.go` 实现。
- 每次追加先锁定目标 `news_item_versions` 行，再读取当前最新 Decision；同一 Version 的首写和后续写入均被串行化，不依赖“锁定空 Decision 集合”。
- 序列规则：首条必须 `decision_seq=1` 且无 supersedes；后续必须为最新序号加一、supersedes 精确指向最新 Decision，`recorded_at` 不得早于前序记录。
- 在同一事务内校验全部 considered Evidence 均属于目标 Version；选中 Evidence 必须保持 parsed、候选时间存在、raw payload 未清理、Evidence 记录/观察时间不晚于 Decision，且 retention 覆盖 Decision 时间。相关 Evidence 行在检查期间加锁，防止并发清理改变选中证据状态。
- 决策插入与校验同事务提交；数据库 V10 的唯一序号、唯一后继和复合外键继续作为最终约束。
- MariaDB 集成测试覆盖并发首写（恰好一个成功、另一个序列冲突）、错误 supersedes、有效 seq 连续追加，以及 considered Evidence 不属于当前 Version 的拒绝路径。
- 本次只实现内部 Decision 持久化；没有创建生产 Evidence、接入 Provider 或改变来源准入结论。`available_at` 仍按原规则 fail-closed，B 组 SKIP 不变。
- 实现提交：`dc56115a4822301915109813000c9ce8694bb417`；测试相关提交：`bcff3b7319b36e30847d1c637bc8385cbfad0425`、`fa7ddeb12482aeca1dc311333deadd4d3a930cf1`、`e3d714c38bfce05fdf16246602df682d7c525893`、`06e94b80ef07a7001d44a8ce831ca11e47b8d51f`。
- 本交接文档及 News 验收清单已同步。当前最新代码测试 SHA `06e94b80ef07a7001d44a8ce831ca11e47b8d51f` 尚无可见 CI status / workflow run；文档提交不代表 CI 通过。下一步仅核验该 SHA 的 CI 和 MariaDB 11.4 集成测试，依据真实失败修复；不另开旁支任务。

- 后续集成测试另补充了有效 `verified` Decision 路径：选中 Evidence 必须存在于同一 Version、保持 parsed、候选时间有效、payload 未清理且 retention 覆盖决策时间；该用例包含在最新测试提交中。

- 后续格式整理提交：实现文件 `3eca05a5f3544b37576efeb8ea32bbfad331d9b9`，集成测试 `06e94b80ef07a7001d44a8ce831ca11e47b8d51f`；最终测试 SHA 仍未返回 Combined Status / workflow run，验收状态保持 pending。


### 2026-11-16 available_at Evidence 留存执行缺口修复

- 前一轮 Decision 追加写入与 PIT 读取完整性修复已由精确 SHA `997b4c1a2989d6db20db6c976fc6ddf8d0af0778` 的 Integration CI Run [43818069869](https://github.com/Kasign/stock_data_provider/actions/runs/43818069869) 全量 success。
- 本轮发现 V10 Evidence 有独立 `retention_until`，但现有 Observation Retention Cleaner 只清理父 Observation 的候选载荷，未清理 Evidence 自身原始值。已在同一有界清理事务中增加 Evidence 独立到期清理：清除 raw/candidate/ref/hash 并保留 tombstone，保留 Decision 追加历史；结果单独报告 Evidence scanned/purged 数量。
- 新增 MariaDB 集成覆盖；代码提交 `4040b8aaa27c58641ee097b5b91b4d848d2fb8e3`，Run [43818693172](https://github.com/Kasign/stock_data_provider/actions/runs/43818693172) 的最终状态待验收。
- 下一步按 available_at 内部链路整体收尾：核对 Evidence 创建/持久化是否具备字段级授权凭据入口；若无，补齐受授权策略约束的写入边界，不允许提供可绕过策略的通用 raw Evidence 写入 API。来源仍未准入，不创建生产 Evidence、不解除 B 组 SKIP。


### 2026-11-16 available_at 字段级授权写入闭环（历史任务，已完成）

当前唯一主线仍是 News 内部 `available_at` 证据链，不转向其他模块。

已确认缺口：既有 Source / Endpoint / Provider Output Policy 只覆盖 metadata 类型、存储模式和留存期限，不能表达具体字段授权。已在 News Domain 增加 `AllowedMetadataFields` 精确路径许可，并将其纳入统一 Source ∩ Endpoint ∩ Provider 策略评估。Endpoint 只能继承或收紧 Source 字段集合；Source / Provider 未明确列出字段时拒绝持久化。旧策略版本不自动获得该权限。

已实现的第一版内部写入边界：
- Application `RecordAvailabilityEvidence` 只接受已解析候选与字段路径，不接受调用方传入授权结论。
- 通过可信 SourceCatalog / Provider Output Policy Resolver 核对 Observation 记录的策略版本。
- 在同一 Identity DB 事务中锁定当前 Source / Endpoint / Provider 策略指针，评估个人研究、metadata、persistent_metadata 和剩余 retention。
- MySQL Adapter 在同一事务中复核 Version / Observation 主体、策略版本、观察时间、父载荷清理状态和 retention 后写入 V10 Evidence。
- 该写入能力不判定来源时间语义，不创建 Decision，不允许未准入 Provider 写生产 Evidence。

已完成并由最终代码 SHA `7d43043ed0eed070c3313c64f8aa7d6dc13e8fa3` 的 Integration CI Run [43823338222](https://github.com/Kasign/stock_data_provider/actions/runs/43823338222) 全量验收。MariaDB 写入测试覆盖当前策略锁定、字段授权拒绝、策略版本变化拒绝、成功追加和父 Observation 留存上限；策略载荷测试覆盖非法/重复字段路径及历史策略默认拒绝。详细验收结论见下节。


### 2026-11-16 字段级授权写入边界验收完成

- 已在 News Source / Endpoint / Provider Output Policy 中增加 `AllowedMetadataFields` 精确字段许可；Source 与 Provider 必须明确列出，Endpoint nil 继承、非 nil 只能收紧，旧策略缺少字段许可时默认拒绝新 Evidence 写入。
- 已实现统一策略评估中的 `CollectionPolicyRequest.FieldPath`，仅适用于 `metadata + persistent_metadata`；字段路径为精确点分路径，不接受通配符或表达式。
- 已实现 `RecordAvailabilityEvidence` Application 用例：可信目录版本核对、Identity 事务内锁定当前策略指针、字段授权评估、个人研究用途、持久化元数据和留存上限交集，再追加 Evidence。
- 已实现 MySQL 事务写入端对 Version / Observation / Source / Endpoint / Provider 策略版本、父载荷清理状态及 retention 的数据库复核。没有暴露无策略保护的通用 Repository raw Evidence 写入入口。
- Static Source / Provider Policy Catalog 深拷贝新增字段数组，防止可变引用污染授权快照。
- 单测覆盖三层字段许可、Endpoint 不可扩权、非法/重复路径、历史策略默认拒绝；MariaDB 11.4 测试覆盖成功追加、父 Observation retention 上限、策略版本变更和字段未授权拒绝。
- 最终代码 SHA：`7d43043ed0eed070c3313c64f8aa7d6dc13e8fa3`。Integration CI Run [43823338222](https://github.com/Kasign/stock_data_provider/actions/runs/43823338222) 全量 success。
- 当前阶段内部写入授权边界验收完成；不代表任何真实来源已准入。没有获准来源前，不生成生产 Evidence，不创建 verified Decision，B 组保持 SKIP。

**下一项主线：** 来源准入仍是唯一阻塞生产 Evidence 的条件。等待官方/授权服务提供字段语义、逐公告版本绑定、可审计真实响应，以及个人用途的程序化访问、持久化与再利用许可；一旦具备这些证据，再按 Adapter 逐来源接入 `RecordAvailabilityEvidence`。不重复采样已 blocked 的公开页面，不调用非公开接口。


### 2026-11-16 Researchflow 可信 CallerRef 执行期传播

**发现并修复的真实 L2 缺口：** CreateRun 已从可信 RequestIdentity 获取 caller_ref，并持久化在 `researchflow_run_idempotency`；但 Worker 的执行快照此前没有读取该身份，`StageExecutionRequest` 也未传递 caller_ref。下游 Provider 因此无法获得创建 Run 时的可信调用主体。这是 Researchflow 使用方/编排方未履行运行时传播责任，不应要求 Provider 自行从普通 Header 或请求参数推断身份。

- Researchflow Migration v5 为 `researchflow_run_idempotency.run_ref` 增加唯一约束，保证每个 Run 只有一条 CreateRun 身份记录；已有重复 run_ref 数据会使迁移失败并要求先核实，不自动合并或覆盖身份。
- Worker Store 在 Repeatable Read 执行快照中关联 CreateRun 幂等记录读取 caller_ref；无身份时 fail-closed。
- Worker 在 ClaimAttempt 前校验身份存在，claim 后第二次读取时确认身份未变化，再注入内部 `StageExecutionRequest.AuthenticatedCallerRef`。字段不进入公共 DTO，不由 Provider 提供。
- 按项目级决议，Server 生产 RequestIdentity 仍须注入固定服务主体 `service:researchflow`；本次实现只完成 Worker → StageExecutor 的进程内传递，不宣称 Server 生产身份装配或远程认证传输已完成。
- Provider 侧职责：消费可信 caller_ref，并调用自身 Owner 授权 Port；CallerRef 不是 AuthorizationRef/Bearer Token。
- 已补 Worker 单测及现有 MariaDB WorkerStore 集成断言；新 SHA 的全量 CI / MariaDB 11.4 执行结果待记录。

**卡点重判：** Worker → StageExecutor 的 CallerRef 传播不再是外部阻塞，属于 Researchflow 自身缺口，现已实现。Server 生产 RequestIdentity 装配仍归 Server；Stage Provider 的实际 Application Port、输出 Schema 和领域语义仍归对应业务模块。不要把这项修复误报为生产 StageExecutor 或跨模块 E2E 已完成。

### 2026-11-16 Researchflow CallerRef 传播验收与后续卡点重判

- 当前 integration HEAD：`4c21ecfa84dc2bb349ae05c4b0d6c41c9fe1c110`。
- Researchflow CallerRef 实现提交：`e34089a27f`，后续仅有字段格式整理提交；Integration CI Run [43827807352](https://github.com/Kasign/stock_data_provider/actions/runs/43827807352) 对精确 HEAD `4c21ecfa84dc2bb349ae05c4b0d6c41c9fe1c110` 全量 success。Run 包含根模块测试、News/Research MariaDB 跨模块测试、Worker Bootstrap MariaDB/Redis、go-core、Compose 与 WebUI 检查。
- Worker Store 在执行快照中读取 CreateRun 身份，Worker 在 ClaimAttempt 前 fail-closed、Claim 后复核身份一致，并将 `AuthenticatedCallerRef` 传入 StageExecutor。该内部传播 L2 缺口已关闭，不再作为 Stage 准入阻塞项。
- Server 生产 RequestIdentity 注入仍未实现，但当前没有 Researchflow CreateRun 的生产 HTTP/业务消费者；`SRV-CTR-001` 仍为 deferred。此时单独注入固定 `service:researchflow` 而没有实际 CreateRun 调用场景，只会形成未使用装配，不能宣称生产身份链路已通过。待正式 CreateRun 消费入口确定后，Server 必须从可信身份上下文构造 RequestIdentity；系统服务主体只用于明确由该服务发起的 Run，不得覆盖真实用户主体。
- 重新划分真实阻塞：
  1. Researchflow 负责 FLOW-CTR-004 的 Stage Provider Result Envelope、版本化 Stage Output Registry、Validator 装载与执行边界校验；这些是当前可独立推进的模块能力。
  2. 质量状态枚举、coverage 的 scope/numerator/denominator、ResultSummary 资源身份/版本绑定需要补齐权威口径；未形成口径前不得猜测并写入 Schema/Golden。
  3. 各领域 Provider 负责自己的 Application Port、Payload Schema/Golden、质量语义和真实执行能力。Quant Factor 仍受 DATA-CTR-005/007、QNT-CTR-003 deferred 阻断；不能据此阻断与 Factor 无关的其他 Stage。
  4. Server 负责在前述 Registry/Provider/可信身份门禁具备后装配 Scheduler、WorkerPool、StageExecutor 和生命周期；当前不应创建空壳 Executor 或未被消费的身份适配器。
  5. Notification 是终态事件后的异步投递链，不是同步 DAG Stage Provider；真实渠道未就绪不阻断 Researchflow Stage Envelope/Registry 开发。
- 下一项任务：先完成 FLOW-CTR-004 已冻结字段的机器化 Envelope DTO/Schema/Golden 与 Registry 通用框架；质量摘要按领域 Provider 注册的强类型 Schema 校验。ResultSummary 的质量枚举、coverage 细则和 Summary 身份/版本绑定作为单独决策项处理，不以猜测值阻塞 Envelope 基础结构。未有真实 Provider 的 stage_code 不得注册占位项。
- 本轮没有修改 Server、Quant、LLM、Notification 领域代码；没有创建生产 Stage Provider；没有改变任何 deferred 契约状态。

### 2026-11-16 Stage Provider Envelope / Output Registry 通用框架

- 代码提交：`8e0b040badbec3aa295f452ce83f32498d22ecc1`；新增 `internal/researchflow/stageoutput`、Envelope JSON Schema 1.0.0、通用 Golden 和实施记录。
- 后续格式修正代码 SHA：`a46d9f16db3975d05d7f521ba9eb7982fa29fe28`；Integration CI Run [43832742482](https://github.com/Kasign/stock_data_provider/actions/runs/43832742482) 对该精确 SHA 全量 success（gofmt、root/go-core vet/test、News/Research MariaDB 跨模块测试、Worker Bootstrap MariaDB/Redis、Compose、WebUI 检查与构建均通过）。随后 docs-only 提交 `d90129280e6dc0f3ad545d2e47d532b72825538d` 修正 README 换行；当前 HEAD 以远端查询为准。此前 Run 43831178908/43831780222/43832200394 的失败由新文件 gofmt 引起，非业务测试失败，已在 a46d9f1 修复。
- Provider 只返回领域输出 discriminator、payload、quality_summary 和 diagnostic_refs；Run/Stage/Attempt/StageCode/TemplateVersion/CompletedAt 由 Orchestrator 组装。精确 Registry version 与 stage_code/output_type/schema_version 不匹配时 fail-closed。
- Registry 以不可变版本、精确 Schema Ref/Validator 和稳定 SHA-256 fingerprint 标识；拒绝重复注册、缺失 Validator、未知组合、非对象 JSON 和递归重复 JSON Key。
- 当前没有注册任何业务 Stage Entry；空 Registry 仅用于框架构建，不构成生产 Provider 可用证明。没有虚构 News/Quant/Research/LLM 注册项。
- 未定义质量枚举、Coverage 分子/分母、Summary 身份/版本绑定；这些仍须契约决议。ResultSummary 不在本次实现范围。
- 下一步：先取得该代码 SHA 全量 CI 结果；随后将 Registry/Envelope Validator 接入 Researchflow Worker 的提交前边界（保持无注册项时成功业务输出不可提交），再由各模块按真实 Application Port 和领域 Schema 提供注册项。Server 生产装配仍在真实 Provider 完备后进行。


### 2026-11-16 Worker 提交前 Registry 接入的真实边界

- FLOW-CTR-004 的 Envelope / Output Registry 通用框架已由代码 SHA `a46d9f16db3975d05d7f521ba9eb7982fa29fe28` 全量 CI 验收通过；docs-only 修订为 `d90129280e6dc0f3ad545d2e47d532b72825538d`。
- 下一项仍是将 Envelope/Registry Validator 接入 Worker 提交前边界；无注册项或未注册的 `stage_code + output_type + schema_version` 不得提交成功结果。
- 实施前发现一个不能靠 Validator 单独解决的内部存储缺口：冻结 Envelope 要求 Provider 返回 `payload` 与 `quality_summary` 原始对象，Orchestrator 负责生成引用；当前 `StageExecutionResult` 只有 `output_ref` / `quality_summary_ref`，Worker 直接把引用写入 `researchflow_stages`，现有 Migration 也没有持久化 Provider Payload / QualitySummary 的资源表或写入 Port。若只把 Envelope 校验后丢弃 payload，或继续接受 Provider 自带引用，就会出现“验证了但未保存”或“Provider 越权生成引用”的契约违例。
- 这是 Researchflow 自身的实现缺口，不是等待 Quant/News/LLM，也不是外部数据卡点。合理的闭环是由 Researchflow 在受 fencing 保护的结果提交事务中持久化已验证 Envelope 的 payload/quality_summary，并由 Orchestrator 生成 `output_ref` / `quality_summary_ref`；需要据此确定最小资源表、不可变引用规则和读取 Port，再将 Registry 校验放在该事务之前。不得在没有持久化闭环的情况下只做表面 Validator 接线。
- 本轮未新增此存储设计或 Migration；下一步应先按 FLOW-CTR-004 和现有 Run/Stage/Attempt 原子提交边界完成该最小设计，再一次性实现 Migration、Worker、MySQL 事务和 MariaDB 集成测试。质量枚举、Coverage 和 ResultSummary 身份口径仍保持独立 deferred，不随本任务猜测补齐。


### 2026-11-16 Stage Output 不可变资源与 Worker 原子提交设计

- 已新增设计文档：[Stage Output 不可变资源与 Worker 原子提交设计](../../researchflow/08-Stage-Output不可变资源与Worker原子提交设计.md)。
- 设计基于现有 `CommitAttemptResult` 事务，不另起提交流程：保持 Run -> Stage -> Attempt 锁顺序、数据库 Deadline、Lease Owner、FencingToken 和 AttemptNo/version 校验。
- 成功/降级 Provider 结果由 Worker 依据持久化 ResolvedTemplate 的 TemplateVersion / OutputRegistryVersion 构造 Envelope；Registry 精确校验通过后，才允许进入原子提交。
- 新增的持久化模型为每个 Attempt 保存两条不可变资源：payload 与 quality_summary；资源记录包含 Schema Ref、OutputType、SchemaVersion、RegistryVersion/Fingerprint、Run/Stage/Attempt 归属和数据库创建时间。引用由 Storage 在事务内生成，Stage/Attempt 引用与资源插入同事务提交。
- 失败、重试、超时、取消结果不生成输出资源，沿用既有状态机。未配置 Registry、无匹配 Entry 或 Validator 失败均不得成功提交；不保留旧 Provider 自报引用的生产旁路。
- 本次只完成实现设计与文档更新，没有修改 Worker / Migration 代码，没有宣称 Worker 接入已完成。
- **下一项代码任务：** 按该设计实现 Migration v6、Stage Output 资源读写、Worker Snapshot/Envelope/Registry 接线、CommitAttemptResult 原子写入及 MariaDB 集成测试；更新现有成功提交测试，不重复执行已经通过的状态机/Worker 基础验收。


### 2026-11-16 FLOW-CTR-004 Worker Envelope / Stage Output 原子提交闭环已实现

- 实现代码精确 SHA：`7392caaba59aa4d322d4b91e1d34a7294d2f6505`；Integration CI Run [43837079196](https://github.com/Kasign/stock_data_provider/actions/runs/43837079196) 对该 SHA 全量 success。gofmt、根模块 go vet/test（包含 Researchflow MariaDB 集成）、News/Research 跨模块 MariaDB、Worker Bootstrap MariaDB/Redis、go-core vet/test、Compose、WebUI 语法/单测/镜像构建及最终汇总均通过。
- Researchflow Migration v6 新增 `researchflow_stage_output_resources`，每个成功 Attempt 保存不可变 Payload 与 QualitySummary 两份资源，保留 Stage/Attempt/Run 归属、StageCode、TemplateVersion、Registry Version/Fingerprint、OutputType、SchemaVersion/SchemaRef、原始 JSON 和数据库创建时间。
- Worker 执行快照从持久化 ResolvedTemplate 读取 OutputRegistryVersion；成功/降级 ProviderResult 由 Worker 注入 Run/Stage/Attempt/StageCode/TemplateVersion/CompletedAt 构造 Envelope，精确匹配注入 Registry 并校验 Payload / QualitySummary Validator。
- Provider 自报 `output_ref` / `quality_summary_ref` 的成功结果被拒绝。Storage 在原有 Run → Stage → Attempt 锁、Deadline、Lease Owner、FencingToken 和 AttemptNo/version 检查之后，在同一事务插入两份资源、生成不透明引用并更新 Attempt / Stage。事务失败不会留下部分资源或成功状态。
- 增加按 resource_ref 读取资源的 Storage 能力；MariaDB 集成测试覆盖模板 Registry Version 快照、成功提交、Payload / QualitySummary 资源读取。Worker 单测覆盖 Registry 缺失和 Provider 引用越权拒绝。
- 文档已更新：`doc/researchflow/README.md`、`doc/researchflow/08-Stage-Output不可变资源与Worker原子提交设计.md`。实现设计不改变 FLOW-CTR-004 冻结字段，不注册虚构业务 Stage。
- 当前 integration HEAD（文档更新后）：`6bae6b2606181281dd2d9579ab854313236271aa`。这是 docs-only 提交；最近代码验收仍以 SHA `7392caaba59aa4d322d4b91e1d34a7294d2f6505` / Run 43837079196 为准。
- **本阶段完成边界：** 通用 Envelope / Registry / Worker 校验 / 原子持久化已闭环；没有真实业务 Registry Entry，没有生产 StageExecutor Adapter，也没有 Server 生产 Worker 装配。不得将本阶段报告为 News、Research、Quant 或 LLM Stage 已可生产运行。

**下一项主线不再继续打磨 Stage Output 细节：**

1. 按现有模块公开 Application Port 和已存在领域 Schema/Golden，推进第一个真实可用的 Stage Provider；优先核实 News / Research 是否已有完整可消费能力，缺少领域质量语义或 Schema 的部分由对应模块 Owner 决定，不由 Researchflow 猜测。
2. Quant Factor 继续受 DATA-CTR-005/007、QNT-CTR-003 deferred 阻断，不影响与 Factor 无关的 Stage。
3. LLM Provider / CapabilityState 与 Notification Delivery 仍按第 7 节 blocked 处理；Notification 仍是 Run 终态后的异步链路，不作为同步 DAG Stage。
4. 至少一个真实 Provider 与对应 Registry Entry 完备后，再推进 Server 的 Registry / StageExecutor / Scheduler / WorkerPool 生命周期装配及端到端验收；不先创建空壳 Executor。
5. `available_at` 来源准入维持项目负责人已确认的 shelved 状态；不重新采样或解除 B 组 SKIP。



### 2026-11-16 第一个真实 Stage Provider 选型复核

本轮按主线审查 News / Research 现有能力是否能直接成为第一个 Researchflow 业务 Stage；没有修改领域代码，也没有重新打开已搁置的来源准入任务。

**News 现有能力核查**

- `internal/news/app.SearchCandidates` 是真实 Application 用例，接收 `SearchRequest{Query, QueryDimension, Limit}`，调用 ProviderSet 并返回 `SearchResult`；结果保留候选、Provider 级状态、错误分类、耗时和观察时间。
- 已有 Bocha、Tavily、Brave News、SearXNG 四种协议 Adapter；`NewPlatformProviderSet` 可按配置创建 Provider，并为各 endpoint 绑定 Platform HTTP Doer。
- 当前 Server Bootstrap 没有构造或消费 `SearchCandidates` / `NewPlatformProviderSet`；现有 News→Research 组合只装配 Evidence Query / Importer。搜索能力因此是模块内可复用能力，不是已经接通的生产 Stage。
- 当前没有与该搜索用例对应的 Stage Payload Schema/Golden、QualitySummary Schema/Golden 或业务 Registry Entry。不能把通用 Envelope Golden 当成搜索结果契约。

**Research 现有能力核查**

- Research 已有 Context、Evidence、Event、Claim/Hypothesis、Scenario 等领域与 Application 能力，但没有一个已定义输入、输出和质量语义的独立 Stage Provider。
- News Evidence Importer 虽已装配，但真实写入仍受 News 来源证据与 PIT 门禁限制；当前 `available_at` 来源准入已搁置。不能把搜索候选直接当成 Research Evidence，也不能以测试 Fixture 代替生产来源。

**选型结论**

当前没有可直接注册并投入生产的完整 Stage Provider。唯一有现成真实业务入口、且不依赖被搁置 `available_at` 的候选是 **News 搜索发现能力**：它只能作为“发现候选线索”的 Stage，输出必须明确标注 Provider 来源、搜索观察时间和字段缺失情况；不能宣称候选已成为 Research Evidence 或已验证事实。

建议以现有 `SearchRequest` / `SearchResult` 为边界实现该候选 Stage，暂拟 Stage Code `news.search_candidates`。实施时需在 News Owner 文档中固化：
1. 参数 Schema：`query`、`query_dimension`、`limit`，语义直接沿用现有 SearchRequest。
2. Payload Schema/Golden：按现有 SearchResult、Candidate、ProviderOutcome 映射，不引入无来源字段。
3. News 自有 QualitySummary Schema/Golden：明确 provider success/empty/failed/skipped 与候选缺失字段的统计口径；不借用或猜测 Researchflow 的全局 quality_status 枚举。
4. 状态映射：尤其明确“未配置 Provider / 全部 Provider skipped”如何映射 Stage 终态；不能把 skipped 伪装成成功或普通空结果。
5. 只有上述契约、真实 Adapter 装配和执行测试完成后，才注册该 Stage Entry；没有可用 Provider 配置时必须保持 fail-closed。

**下一项实施任务：** 落地 News 搜索发现 Stage 的领域输出契约与真实 Adapter，并在 Server 组合边界装配现有 Platform ProviderSet；完成后再接入 Researchflow Registry。暂不装配完整 Worker 生命周期，不创建无真实业务意义的模板，不触碰 Research Evidence 导入、`available_at`、Quant 或 Notification。

本轮结论是选型审查完成，不代表 Stage Provider 已实现或生产可用。


### 2026-11-16 News 搜索发现 Stage 输出契约与适配层

- 本轮基于 integration HEAD `4e80d5ccf4e5a89bf5fe9a6e40224675f5306ab0` 开始实施第一个真实业务 Stage；未重复研究 Researchflow Envelope、Registry、Worker 原子提交等已验收通用框架。
- 新增 News-owned `internal/news/stage` 适配层：复用既有 `app.SearchCandidates`，将 `SearchResult` 映射为版本化 JSON Payload / QualitySummary；News 不导入 Researchflow，避免反向模块依赖。
- Stage Code / Output Type 均为 `news.search_candidates`，Schema Version 为 `1.0.0`。输入沿用 SearchRequest 的 `query`、`query_dimension`、`limit`；参数 Schema 不暴露 Provider endpoint 或 API Key。
- Payload 保留 Candidate 全部现有语义字段、Provider provenance、原始发布时间、可解析发布时间、ObservedAt、MissingFields，以及每个 Provider 的状态、错误码、Retryable、候选数量、耗时纳秒和观察时间。没有增加来源推断字段。
- QualitySummary 只包含可直接统计的搜索状态、Provider 状态数量、候选数量和 MissingFields 计数；不定义全局 quality_status，不把候选升级为 Research Evidence。
- 对 Provider 未配置/全部 skipped，Payload 保留 `status=skipped` 和原始 failure_code；禁止映射成 empty 或成功 Stage。Server StageExecutor 必须将该状态映射为非成功执行结果，不能提交成功 Output。
- 已新增参数/Payload/QualitySummary Schema 及 Payload Golden，适配层单测与强类型 JSON Validator。下一步先由精确 SHA 的 Integration CI 反馈修复真实问题，再在 Server 组合根使用现有 Platform HTTP Client 构造 ProviderSet、构造 Registry Entry（引用 News 自有 Validator），并实现 StageExecutor 状态映射。只有 Registry 与 ProviderSet 均真实装配后，才进入 Worker 消费链。
- 当前不触碰 Research Evidence Import、available_at、Quant、Notification；不创建无消费端的 Workflow Template，不启动空壳 Worker 生命周期。


- 首轮实现 CI：Run [43844764284](https://github.com/Kasign/stock_data_provider/actions/runs/43844764284)，精确 SHA `5737f7f8e3e52eb40b18ad877eae9b2c42762066`。全仓 Go vet、News/Research 跨模块 MariaDB、Worker Bootstrap MariaDB/Redis、go-core、Compose、WebUI 均通过；全量 Go test 唯一失败为 Stage 参数解码测试，原因是直接将无 JSON tag 的领域 `SearchRequest` 用于小写 JSON 字段解码；Go 格式检查也只指出新增 Stage 文件的格式差异。已改为 News-owned JSON 参数 DTO 并完成格式修正；该 Run 保持 failure，不作为验收通过证据，等待修复 SHA 的新 CI。


### 2026-11-16 News Stage Server 组合与状态映射

- Server Bootstrap 新增 `newNewsSearchStageComposition`：根据 News 配置决定是否创建 Platform HTTP Client；配置 Provider 时通过既有 `NewPlatformProviderSet` 装配 Bocha/Tavily/Brave/SearXNG，不复制 HTTP Client 或 Provider 协议；未配置 Provider 时使用空 ProviderSet，不创建 HTTP Client。
- Server 侧 `newsSearchStageExecutor` 将 News 搜索状态映射为 Researchflow 执行状态：success/empty → succeeded；partial → degraded；全部失败且存在 retryable Outcome → retry_wait（仍受模板 RetryPolicy reason-code allowlist 控制）；skipped / 不可重试失败 / 参数错误 → failed。没有使用 Researchflow 不支持的 StageSkipped。
- Registry 1.0.0 注册 `news.search_candidates` 的 Payload/QualitySummary 1.0.0 精确 Schema Ref 和 News-owned Validator；新增 Server 组合、状态映射、Registry Envelope 精确校验测试。
- 重要边界：当前 `Run` 尚未接入生产 Researchflow Worker 生命周期，本轮只提供可测试的组合工厂与 StageExecutor，不虚构 Worker 消费链，不创建 Workflow Template。后续必须在 Worker 的真实多模块 Executor 装配点接入该 factory，并保证同一 Registry Version 与持久化 Template 一致。
- 已识别诊断持久化缺口：Worker 只为 succeeded/degraded Stage 构造 StageOutput Envelope；当所有 Provider 失败时，Stage Executor 只能提交稳定 reason code，ProviderOutcome 的逐 Provider 细节不会持久化。不要通过伪造成功/降级输出规避；应在 Fault/diagnostic 持久化契约接入后补齐失败详情。
- 本阶段不触碰 Research Evidence Import、available_at、Quant、Notification；不启动空壳 Worker 生命周期。


- Server Stage 组合 CI Run `43847107485` 对 SHA `b6d5b2e9b1f10738c19623ebe14d060cab2dcf93`：根模块全量 Go 测试（含新增 News Stage / Server Bootstrap 测试）、News/Research 跨模块测试、Worker Bootstrap MariaDB/Redis、go-core vet/test、Compose、WebUI 测试和镜像构建全部通过；唯一失败为新增两个 Go 文件的 gofmt 格式检查。已按 CI diff 修正纯格式问题，等待下一 SHA 验收；该 Run 不作为通过证据。


- 第二轮 Server 组合 CI Run `43847712560`：所有代码、MariaDB/Redis、跨模块、WebUI、Compose 和镜像构建检查通过；仅 gofmt 检查指出测试文件中第二处 StageExecutionRequest 字段对齐。已统一修正两处字段对齐，等待最终验收。


- 最终验收 Run [43848194023](https://github.com/Kasign/stock_data_provider/actions/runs/43848194023) 对精确 SHA `92b90d1b4727cd9f4e572790869da754ee65a306` 全量 success：Go 格式、根模块 vet/test、News/Research 跨模块测试、Worker Bootstrap MariaDB/Redis、go-core vet/test、Compose、WebUI 语法/单测、WebUI 镜像构建全部通过。此前 Run 43844764284 / 43847107485 / 43847712560 的失败均已修复，不作为最终验收结果。


### 2026-11-16 News Stage 共享 HTTP Client 所有权修正

- 复核 Root Server 资源所有权后发现，上一版 `newNewsSearchStageComposition` 虽使用 Platform HTTP Client 实现，但自行创建并关闭了另一个 Client；这不符合 `server/resources.Resources` 持有共享 Client 生命周期的组合根约束。
- 已将 factory 改为显式接收 Root Server 传入的 `*platformhttpclient.Client`。News ProviderSet 继续通过 `NewPlatformProviderSet` 为每个 endpoint 绑定独立 DestinationPolicy；factory 不再创建或关闭 Client。
- 有 Provider 配置但共享 Client 为 nil 时启动装配失败关闭；无 Provider 配置时允许 nil Client，不创建出站依赖。
- 测试覆盖 configured + nil client 拒绝、configured + shared client 成功、unconfigured + nil client 成功，以及精确 Registry Entry 保留。
- 该修正为后续 Researchflow Worker 生产装配准备正确的 HTTP 资源注入边界；本次尚未把 Worker 生命周期接入 Bootstrap，也未创建 Workflow Template。


- 共享 HTTP Client 修正首轮 CI Run `43850461274` 对 SHA `32655a8bafdb56f06636b6faccfd175e7b0a653c`：gofmt、其余独立模块检查和 WebUI/Compose 步骤通过；根模块编译失败的唯一原因是测试仍断言旧的 composition.httpClient 字段，属于重构后的过期测试断言，不是运行逻辑缺陷。已移除该旧断言，保留「未配置 Provider 可无 Client、已配置 Provider 缺 Client 必须失败、注入共享 Client 后装配成功」三项行为验证；该 Run 不作为通过证据。

### 2026-11-16 News Search Stage → Researchflow MariaDB 端到端执行验收

- 本轮代码验收精确 SHA：`651bf9a054788311d61e3edf241719bd6256185e`；Integration CI Run [43860850724](https://github.com/Kasign/stock_data_provider/actions/runs/43860850724) 对该 SHA 全量 success。
- 新增 `newNewsSearchStageCompositionWithProviders`，将 News SearchCandidates Adapter 与 Output Registry 的组合逻辑集中在同一工厂；生产配置工厂和测试 ProviderSet 共用同一组合路径，避免测试复制另一套 Stage 装配。
- 新增 `internal/server/bootstrap/news_search_stage_mariadb_integration_test.go` / `TestNewsSearchStageResearchflowMariaDBIntegration`：使用隔离 MariaDB 与 Researchflow Migration，发布仅供测试的 Workflow Template 和参数 Schema，经正式 `CreateRun` Application Service 创建 Run，再由 Scheduler → WorkerPool → Worker 执行 News `SearchCandidates` Stage。
- 测试验证 Worker 通过 News Adapter 执行、Output Registry 校验、Attempt 成功提交；Stage / Attempt 的 OutputRef 一致；Payload 与 QualitySummary 两份不可变资源均能读取，资源的 StageCode、OutputType、SchemaVersion、RegistryVersion 等元数据与冻结 Registry 一致。
- Provider 使用确定性测试 Fixture，不访问外部新闻源，不代表真实 Provider 网络可用性或真实市场数据验收。测试 Template、参数 Schema、Applicability 和 ResourcePolicyRef 都是测试夹具，不是生产模板注册；本轮没有开放 Researchflow HTTP API，也没有创建生产 Run 触发入口。
- CI Run 43860850724 全量 success：Go 格式检查、根模块 go vet/test、News/Research/Researchflow 逐用例跨模块测试（4/4 顶层测试）、Worker Bootstrap MariaDB/Redis、go-core vet/test、Compose、WebUI 语法/单测和镜像构建、最终汇总全部通过。

**当前完成边界与下一步**

1. 已证明单个 News Search Stage 可以沿正式 CreateRun / Scheduler / Worker / Output Registry / MariaDB 持久化链路完成执行；不再需要重复验证该 Stage 的单模块适配细节。
2. 这不是生产 Worker Runtime 完成。Server Bootstrap 尚未装配生产 WorkerPool、Scheduler 周期调度、过期 Attempt 扫描、Deadline Scanner、Finalization 和优雅停止生命周期；本测试也没有验证 Run 终态 Finalization。
3. 生产 Template 与生产 Run 来源仍未建立。下一项先按现有 SearchRequest / News Stage Schema 固化正式模板的业务语义（参数、Applicability、Retry / Propagation、ResourcePolicyRef、OutputRegistryVersion），并提供内部 CreateRun 调用入口及可信 Caller Identity；不擅自开放外部 HTTP API。
4. 正式模板与真实 Run 来源明确后，再装配 Server Worker Runtime。进程本地 Worker 并发必须与数据库全局 `RESEARCHFLOW_MAX_WORKERS` 容量上限分开配置；多实例 Worker ID 必须唯一；Lease / Heartbeat / 扫描周期遵循 Worker 时间约束；关闭时先停止 Scheduler / Worker / Scanner，再关闭共享 Resources。
5. 失败 Provider 的逐 Provider Outcome 诊断持久化仍是独立缺口；不得伪造成功 Output 来保存失败信息。它不影响本轮成功路径验收，但应在正式 Runtime / 诊断契约接入时按既有 Fault / Diagnostics 设计处理。



### 2026-11-16 生产 News Template 发布准入复核

对正式 Template 的代码依赖做了进一步只读核验，当前不能安全发布生产版本：

- `internal/researchflow/parameterschema/validator.go` 只有 Validator 和 Registry 接口；仓库中未找到 Server 生产 Registry 实现或正式 News Search 参数 Schema。测试 Stub 固定返回指纹，不做真实 JSON Schema 校验。
- `internal/researchflow/template/template.go` 只有 ApplicabilityRule 数据结构和 Evaluator 接口；未找到 `news.search_candidates` 的生产规则实现。测试中的恒 true evaluator 不能作为生产规则证据。
- `WorkflowTemplate.ResourcePolicyRef` 是必填并参与 Template fingerprint 的不透明引用；未找到相应 Resource Policy Registry/Resolver/预算执行实现。Researchflow 运行时契约要求 Server 在调用前按 Template 资源策略授权，单纯填充字符串不满足要求。
- 没有真实生产 CreateRun 消费方负责输入快照、可信 CallerRef、授权和幂等键。不能为无消费者的模板注入固定服务主体，也不能新增未经契约要求的公网路由。

因此正式 Template 发布的最小前置条件是：确定真实内部消费场景和身份/授权责任；提供严格且不可变的参数 Schema Registry；提供版本化 Applicability 规则；明确 Resource Policy 的解析与授权执行边界。条件齐备后再使用现有不可变 `MySQLTemplateRegistry.Publish` 发布并做 MariaDB 验收。此复核不改变 News Stage 已完成的 Fixture-backed MariaDB 执行结论，也不把真实 Provider 网络验收列为上述开发的前置条件。


## 2026-11-17 主流程续接：Owner Session Server 闭环完成，转 WebUI 消费接入

本节覆盖此前“Integration 无可推进业务开发任务”的历史等待态判断；该等待态形成后，项目负责人继续授权推进 Owner Session 主流程。

- 当前代码基线：`5f23e1a0723a25c90ef2456ff318e25fa3e61b94`。
- Integration CI Run [43998030882](https://github.com/Kasign/stock_data_provider/actions/runs/43998030882) 对精确 SHA success；全仓 gofmt、root/go-core vet/test、跨模块测试、MariaDB/Redis Bootstrap、Compose、WebUI 检查与构建全部通过。
- Owner Session Server HTTP Handler 已按 SRV-CTR-002 挂载；Schema/Golden/OpenAPI 配套资产已补齐；真实 MariaDB/Redis HTTP 生命周期测试通过。
- 当前 Server 端仍保持安全隔离：Owner Session 不继承 Owner Bearer 权限；旧 Analysis/运维诊断路由不接受 Owner Session；Owner 初始化仅受控本机 Secret 命令。
- 当前下一项唯一主流程任务：WebUI 消费 SRV-CTR-002，完成同源 Cookie 登录态、内存态 CSRF、刷新恢复、登出与密码修改。具体进度和验收边界见 `doc/server/Owner-Session接入进度.md` 与 `doc/web/API_CONTRACT.md`。
- WebUI 完成前不得声称浏览器登录闭环完成；Nginx/Compose 真实部署验证仍是后续验收项。
- Researchflow 新 HTTP API 仍属于 SRV-CTR-001 deferred，不得把 Owner Session 接口当作 Researchflow 业务 API，也不得提前开放业务路由。


## 2026-11-17 当前状态校正与下一主线

本节为当前 integration 任务交接的最新状态；覆盖此前“Owner Session WebUI 尚未接入”的历史下一步记录。

### 当前基线

- 当前唯一工作分支：`integration`。
- 本次文档核对时的代码基线：`446cc1a65c307726a1171e1341d529452ea09f55`。
- Owner Session 文档状态同步提交：`4fd28bdeb621ae1d88224b7541afc5979d2fb204`。
- 代码基线 `446cc1a` 的 Integration CI Run `43999751873` 为 success；`4fd28bd` 是文档状态更新，不把前一 SHA 的 CI 误记为新 SHA 的代码验收。
- Owner Session Server + MariaDB/Redis HTTP 生命周期、WebUI API/UI 和自动化测试已完成；目标环境 Nginx/Compose/HTTPS 真实浏览器验收未完成。
- SRV-CTR-002 状态保持 `freeze-ready`；SRV-CTR-001 仍为 `deferred`。

### 当前主线任务

Owner Session 不再安排新的功能开发。目标环境部署验收可在具备目标服务器访问条件时完成；仓库内下一主线转为 Researchflow 生产入口，而不是重复验证已通过的 Stage/Worker 测试。

1. **Server 内部 Researchflow 消费组合**：目标场景为已登录 Owner 在 WebUI 研究任务工作区发起 Run。先实现/验证进程内 CreateRun、GetRun、CancelRun 的组合与可信 RequestIdentity；不开放公网 HTTP 路由。
2. **Run 访问授权**：CreateRun CallerRef 从 Server 已验证 Principal.Subject 派生；Owner Session Subject 为 `owner`，AuthMethod 由 Server 显式路由策略判断。GetRun/CancelRun 必须检查持久化 Run 创建者关联，不能仅凭登录态访问任意 Run。当前 idempotency 表已有 caller_ref/operation/run_ref 关联，可作为实现基础。
3. **News Search 生产准入**：基于现有 News Stage 输入与验证实现严格、不可变的参数 Schema Registry 和版本化 Applicability Rule。
4. **Resource Policy**：当前只有 Template 必填的 ResourcePolicyRef 字符串，没有权威策略定义、Registry/Resolver/执行实现。该项必须先按现有契约/项目决策查清，不得填充虚构策略或发布生产 Template。
5. **正式 Template 与生产 Runtime**：以上条件满足后发布不可变 News Search Template，再装配 Scheduler、WorkerPool、过期 Attempt 扫描、Deadline Scanner、Finalization 和优雅停止。

### 继续执行约束

- 不重做 News Search Stage 的 Fixture-backed MariaDB 验收。
- 不把 DEC-PLAT-002-050 的 `service:researchflow` Quant 授权主体误用为 Owner 创建 Run 的 CallerRef；Owner Run 应由可信 Owner Principal 标识。
- 不在 SRV-CTR-001 deferred 时开放 Researchflow HTTP API。
- 不使用恒 true Applicability、固定 Schema Fingerprint、空 ResourcePolicy 或测试 Caller 作为生产实现。
- 不重启 `available_at` 来源准入；该项保持 shelved。
- 每次提交按完整功能切片聚合，完成后更新本交接文档与模块任务文档。


### 2026-11-17 News Search 参数准入第一切片更新

已在 Server Bootstrap 包实现并测试：
- 不可变 News Search Parameter Schema Registry（ref `doc/researchflow/schema/news-search-parameters-1.0.0.schema.json`，version `1.0.0`）。
- 参数字段来源于现有 News Stage：query、query_dimension、limit；limit 范围 1～100，拒绝额外字段。
- 版本化 Applicability Rule `news.search_candidates.always@1.0.0`；未知规则或版本 fail closed。
- **尚未生产接线：** `bootstrap.Run` 当前没有调用 `newNewsSearchStageComposition`；这些依赖由 factory 组合并通过单测验证，待正式 Worker Runtime 接入后才成为生产运行链的一部分。

当前剩余准入重点是 ResourcePolicyRef 的权威语义/执行责任和正式 Owner CreateRun 消费组合；之后还需把已实现的 Schema/Applicability factory 接入生产 Worker Runtime。未发布正式 Template，未开放 SRV-CTR-001 HTTP API，未装配生产 Worker Runtime。
