# Repair All 跨模块修复计划与进度

- 仓库：`Kasign/stock_data_provider`
- 工作分支：`repair_all`
- 当前代码验收基线：`5334043ea8e1abc2c4006c371a3f95b123098946`（2026-11-19 最新 HEAD）
- 更新日期：2026-11-19

## 1. 分支用途与执行原则

`repair_all` 是当前统一的跨模块 Review、修复和集成验收工作分支。用户授权在该分支直接修改现有模块代码和文档，以避免在多个模块分支间反复切换。

- 可以直接修复任意现有模块中的真实缺陷；修改位置由问题归属和架构职责决定。
- 不复制模块、不创建平行实现、不增加新的业务模块，不绕过现有 Port、Adapter、Repository、Platform 或 Server 装配边界。
- 多模块问题应同时检查生产者、消费者、契约、调用链、持久化和启动装配。
- 不为通过测试降低契约要求，不用 Mock/Fixture 代替真实生产验收。
- 复用已有公共能力；公共能力缺失时在其既有归属模块扩展。
- 每轮开始重新核对 `repair_all` HEAD 和 Actions；所有写入使用 expected SHA，防止覆盖并发更新。
- 变更按完整功能聚合提交，使用中文提交信息；模块专项文档和本文件同步记录。
- 历史模块分支文档中的“只允许修改模块目录”是旧分支协作约束，不限制用户已明确授权的 repair_all 聚合修复；但模块职责边界仍然有效。

## 2. 修复顺序

| 顺序 | 工作项 | 涉及模块 | 状态 |
|---|---|---|---|
| P0 | Data Migration 定义、Platform 只读校验、Server Root 统一注册与启动路径 | Data / Platform / Server | 代码与完整 CI 验收通过；生产启停演练待完成 |
| P0 | News Search Candidates Validator 与冻结 Schema 一致性 | News / Researchflow | 已修复，完整 Integration CI 已通过 |
| P1 | Data、News 到 Research 的数据/时间/Evidence 契约和真实消费边界 | Data / News / Research | 待系统 Review |
| P1 | Quant Application Port、结果 Envelope、Researchflow 消费接口 | Quant / Researchflow / Server | 待系统 Review |
| P1 | Research Context/Evidence 与 Researchflow Run/Stage/Result 语义及唯一持久化 Owner | Research / Researchflow / Server | 待系统 Review |
| P2 | LLM Gateway 生产 Provider、Secret、Capability Evidence 与真实调用 | LLM / Platform / Server / Researchflow | 待系统 Review |
| P2 | Notification 事件、Recipient、Fanout/Delivery、Worker 和恢复链 | Notification / Platform / Server | 待系统 Review |
| P3 | Diagnostics 注册、身份/审计传播、Worker 恢复、WebUI Root API 对接 | Platform / Server / 各业务模块 / WebUI | 待系统 Review |
| P3 | 完整启动、迁移、跨模块端到端和故障恢复验收 | 全模块 | 待实施 |

## 3. 本轮已完成的修复

### RA-NEWS-001：Search Candidates Payload 唯一性校验

- 基线 SHA：`37d37f3107befbd42c98891917efc608c354330b`
- 提交 SHA：`a1d56c1f7034d2c77ecfee9bc6daf574237f9847`
- 修改：
  - `internal/news/stage/search_candidates_validation.go`
  - `internal/news/stage/search_candidates_test.go`
- 原因：冻结 Schema 对 `candidates[].missing_fields` 声明 `uniqueItems: true`，但 Go Validator 只检查枚举，未检查重复值。
- 结果：重复的 missing field 现在被拒绝，并有回归测试。
- 治理：未改 Schema、未复制 Validator、未更改 Researchflow 注册方式。
- 验收：已由完整 Integration CI Run [44450277491](https://github.com/Kasign/stock_data_provider/actions/runs/44450277491) 在精确 SHA `5b9ebe6a2b8103b4c63a48c3c0f0ddb004d583ec` 覆盖通过；后续聚合基线持续包含该修复。

## 4. P0 Migration 三方统一 Review 结论（已实施，保留历史问题记录）

P0 原始问题及实施约束作为审计记录保留；当前状态以第 7、9～13 节的精确 SHA 和 CI 证据为准，不再将本节历史问题描述为当前未修复问题。

- Data 已导出 V1-V4 静态 Migration definitions；V1-V3 保持原始定义，V4 纳入兼容索引/补列操作。
- Platform 已提供统一 MigrationOperation、namespace lock、checksum 和只读 ValidateAppliedMigrations。
- Server Root 已统一注册 Data definitions；enabled 由 Root Registry 执行，disabled 由只读校验，移除 external namespace 过渡路径。
- Migration checksum 与物理 Schema 漂移分开判定；Data Ready 负责物理结构校验。
- 代码层 MariaDB/Redis 集成与 Root Bootstrap 验收已通过；生产环境 Migration enabled/disabled 部署演练仍未执行，不能标记生产验收完成。

## 5. 当前执行任务与边界

### RA-CROSS-REVIEW-003：Data / News → Research 消费链路 Review

- 状态：REVIEW_DONE_WITH_BLOCKERS（2026-11-19，代码/契约/装配只读审查完成；未发现可据现有契约安全修复的生产代码缺陷）。
- 范围：Data、News、Research、Server；本轮不扩展到 Quant、Researchflow、LLM、Notification 或 WebUI。
- 核对内容：DATA-CTR-007、NEWS-CTR-001/002/003、RSC-CTR-001；Data Application Port、Research Adapter/Importer、News PIT/Authorization 实现、Server Composition Root。
- 结论：
  1. Data→Research：Data 已有 MarketDataReader 及 DailyBarsSnapshot，但 Research 代码树没有 Data Adapter；DATA-CTR-007 明确为 deferred，且缺不可变行情内容寻址 SnapshotRef、Manifest 与 Payload 绑定、Research Evidence source_ref/version/locator 映射。当前不是遗漏一段可直接接线的现成能力，不能自行发明映射或把普通 DailyBarsSnapshot 伪装成不可变 Evidence。保持 BLOCKED/DEFERRED。
  2. News→Research：News ResearchEvidenceQuery、单条 PIT 查询、稳定 Cursor/Snapshot 分页、当前策略复核、精确版本内容原子读取及 Research Importer/Authorizer 均已存在。News 存储层通过当前策略指针行锁、Observation/Version 复读、授权回调、正文读取及事务提交构成 TOCTOU 门禁；当前代码不应重复实现。
  3. Server：newResearchNewsComposition 已装配 News Query、Research Service、Importer、Authorizer，并复用共享 DB；但组合仅在 Cursor Key Ring 配置时构造，且当前没有 Stage/HTTP/Worker 业务调用者，Bootstrap 只记录等待业务消费端接入。因此只能判定为装配代码存在，生产消费链路 NOT_COMPARABLE，不能标记端到端 PASS。
  4. News Evidence available_at 缺合格来源证据，News 卡点 NEWS-BLK-07 仍阻塞真实 Evidence 成功导入；不得以 published_at、fetched_at 或 observed_at 替代。
- 诊断判定：
  - Data→Research：DEFERRED / NOT_COMPARABLE（契约明确 deferred，缺 Manifest/内容定位和授权读取契约）。
  - News→Research 代码机制：PASS（静态代码与已有测试所覆盖的机制）；真实来源数据：NOT_COMPARABLE。
  - Server 生产消费：NOT_COMPARABLE（无真实调用者/运行证据）。
- 证据路径：internal/marketdata/app/port.go、internal/research/adapters/news/evidence.go、internal/news/app/get_evidence.go、internal/news/app/list_evidence.go、internal/news/adapters/storage/evidence_authorization_read.go、internal/news/adapters/storage/authorization_policy_resolver.go、internal/server/bootstrap/research_news.go、internal/server/bootstrap/bootstrap.go；项目级登记见 doc/contract/PLAT-002-项目级契约冻结登记簿.md 的 DATA-CTR-007、NEWS-CTR-001/002/003、RSC-CTR-001。
- 修复结论：本轮不修改业务代码。缺口属于尚未冻结的跨模块数据契约及缺少真实业务消费者，不是通过添加适配器即可安全修复的局部 Bug。
- 后续进入条件：Data/项目治理先完成 DATA-CTR-007 的 Snapshot 内容寻址、Manifest/Payload 绑定、AuthorizationRef 与 Evidence 映射；Server/Researchflow 明确真实调用场景和可信 Principal 注入；News 提供可审计 available_at 来源证据。满足条件后再登记新任务并实施跨模块接线。

### 外部/依赖卡点（不阻塞其他独立 Review）

- Data Migration enabled/disabled 的生产数据库启停演练：需要真实部署环境和运维窗口。
- News 合格 available_at 来源证据：保持 NEWS-BLK-07 / SHELVED，不制造样本。
- Data Snapshot/Evidence 契约：DATA-CTR-007 保持 deferred，不在实现层自行冻结字段语义。

## 14. RA-CROSS-REVIEW-003 当前基线校准（2026-11-19）

- repair_all 当前 HEAD：5334043ea8e1abc2c4006c371a3f95b123098946；最新提交为健康数据库 Migration disabled 启动路径回归测试。
- 最新完整 Integration CI：Run [44456052937](https://github.com/Kasign/stock_data_provider/actions/runs/44456052937)，head SHA 与当前 HEAD 精确一致，结论 success。
- 本轮新增测试证明健康物理 Schema 在 Migration disabled 时仍可正常启动；与此前缺列 fail-closed 测试互补，避免只验证拒绝路径而遗漏合法启动误拒绝。
- 当前唯一执行任务切换为 RA-CROSS-REVIEW-003：Data / News → Research 消费链路 Review。Research 模块现有 RSC-001/002/003/007/008 状态和项目级 DATA/NEWS 卡点作为输入，不重复其已通过的模块测试。
- 本任务先只读核对契约、生产代码、Server 装配和现有测试；发现真实缺陷再进入修复。不得以模块单测或历史 SHA 推断当前聚合分支真实生产链路已验收。
- 文档状态权威：本文件记录 repair_all 聚合任务；模块内部任务状态仍以各自模块总任务规划和唯一当前任务状态文档为准。

## 14. RA-CROSS-REVIEW-003 基线与验收证据（2026-11-19）

- 本轮开始时 repair_all HEAD：b3a5b03e126a5754128367f578ef9b14b4d0755a，提交为本轮总体计划校准文档。
- 上一代码基线 5334043ea8e1abc2c4006c371a3f95b123098946 的完整 Integration CI Run [44456052937](https://github.com/Kasign/stock_data_provider/actions/runs/44456052937) 精确 SHA success；该 CI 早于本轮文档提交，不将其冒充为本轮 HEAD 的 CI。
- Data/News→Research 审查结论如第 5 节；News 授权原子读取机制有现有实现，不重复建设；Data→Research 受 DATA-CTR-007 deferred 阻断；Server 组合装配存在但没有真实消费者。
- 本轮未修改生产代码；当前完成的是契约/代码/装配 Review 和卡点同步。生产端到端验收仍未通过。
- 本文件为 repair_all 聚合任务状态权威；模块职责与模块内部任务状态仍以各自模块总规划和唯一当前任务文档为准。
