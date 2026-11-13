# Integration 当前任务交接与继续执行

- 仓库：Kasign/stock_data_provider
- 当前工作分支：integration
- 本文更新日期：2026-11-13
- 本文用途：下一轮新聊天的首要接续入口。新聊天先读本文，再核对 GitHub 当前 HEAD/CI；不得直接假设本文 SHA 仍是最新。
- 本轮验收 HEAD：23d07e7716aa6d7cd7f0fda4d3dbfa178c148a4e
- 对应 Integration CI Run：43362987745，结论 success，head_sha 与验收 SHA 完全一致。
- CI 链接：https://github.com/Kasign/stock_data_provider/actions/runs/43362987745

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

Run 43362987745 对 SHA 23d07e7 全部通过：

- 全仓 Go 格式检查；
- Root Module go vet ./...、go test ./...；
- go-core go vet ./...、go test ./...；
- Compose 配置检查；
- WebUI JavaScript 语法检查、单元测试和 Docker 镜像构建。

此前 Run 43362053690 和 43362555225 曾失败：前者暴露变量遮蔽、gofmt 和 Schema 夹具遗漏；后者修复了编译问题但测试夹具仍缺少 SchemaVersion。上述问题已在 23d07e7 修正，并由 Run 43362987745 的精确 SHA 全量验收通过。不要重复执行或重做这些已通过检查，除非后续改动触及相关代码。

## 2. Server / News / Research 当前代码事实

### 2.1 Server 现状

internal/server/bootstrap/bootstrap.go 当前仍以旧 Analysis Runner、旧 Analysis Task Queue 和旧 Analysis HTTP API 为主要业务运行链。Bootstrap 有共享 DB/Redis、Migration Registry、Lifecycle、Health/Readiness，以及 Research/Researchflow → Notification Outbox Relay。

当前 Bootstrap 尚未：
- 构造 News 的生产 Evidence Query Facade；
- 构造 Research Context/Evidence Repository 与 Research Application Service；
- 注入 News → Research 的 Importer/Authorizer；
- 装配 Researchflow Scheduler、WorkerPool 和生产 StageExecutor。

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

EvidenceCursorCodec 已要求 Key Secret 长度 32～256 字节，最多 8 个 Key；游标 TTL 为 15 分钟，轮换时旧 Key 至少保留到其签发的游标过期。目前 internal/news/config/config.go 尚未定义该游标密钥配置，Server 也未装配完整查询 Facade。

不得使用硬编码密钥、空 Key、Mock Facade、永远返回空列表的 ListEvidence，或跳过签名验证的方式绕过该缺口。下一步应先按 Platform Env/Secret 约定设计可轮换的密钥配置，并为缺失/非法/轮换场景增加测试。

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

### 2.5 Researchflow 当前边界

Researchflow 已有 Scheduler、Worker、WorkerPool、Claim/Lease/Fencing、候选扫描与 Finalizer 等模块内设施；但尚未形成 Server 生产 StageExecutor 组合。Quant 也不能直接作为生产 Stage Provider：Indicator/Factor/Trend 的公开 Application Port、Payload Registry 和冻结契约门禁仍有缺口。

因此当前最合理的先后顺序是先完成 News/Research 生产依赖的可验证装配准备，再按真实 Stage 定义、Output Registry 和生产 Application Port 接入 Researchflow。不得虚构 Stage Code、输出 Schema 或 Quant 计算入口。

## 3. 下一轮明确任务

按顺序推进，一次只处理一条真实依赖链：

1. News 游标密钥配置：检查 Platform Env/Secret 约定与现有配置测试；设计 Active Key ID + Key Ring 的配置形态，支持 15 分钟轮换重叠窗口；不提供默认密钥，不把明文写入日志/诊断。更新 doc/news/ 说明和示例配置。
2. News/Research 组合构造：在 Server Composition 层构造共享 DB 上的 News Repository、公共 Verifier、当前策略复核器、Get/List Evidence Facade、Research Repository 和 Research Service，再构造已有 Research News Importer/Authorizer。所有必需依赖均为真实实现。
3. 装配验收：为构造入口补 Bootstrap/Composition 测试；使用真实 MariaDB 集成验证从 News 授权快照读取、当前策略复核到 Research Evidence 持久化的调用链。测试应覆盖指纹篡改、策略撤销、PIT 不可见、事务失败和 Context 取消。
4. Researchflow Stage 接入：先读取当前冻结 Stage/Template/Output Registry 契约及真实模块代码。只有具体 Stage 输入/输出、Registry 版本、可信 CallerRef 和生产 Provider 均明确后才接入；不创建空壳 StageExecutor。
5. Quant：继续保持生产 Stage 禁止接入，直到其 Application Port、Result Payload/Registry 和对应 Data 精度门禁满足契约。

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
3. 查看 doc/contract/PLAT-002-项目级契约冻结登记簿.md、GOV-CTR-006、doc/news/README.md、doc/research/README.md、doc/server/README.md。
4. 从第 3 节第 1 项继续；若发现新 HEAD 已经包含对应工作，先核对证据，再推进下一项，不重复开发。
