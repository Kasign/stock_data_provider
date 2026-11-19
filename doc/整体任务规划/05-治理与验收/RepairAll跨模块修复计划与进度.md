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

## 5. 下一步任务

1. 先完成 Data Migration 定义的重构设计，重点解决条件式兼容 DDL 如何纳入不可变版本与 Platform 锁/checksum 保护；不直接将当前 Go helper 粗暴搬成不具备幂等语义的 ALTER SQL。
2. 扩展 Platform Migration 定义/执行能力，使条件式 Schema 操作可以在统一 namespace lock 下执行，且操作描述纳入 checksum；同时提供只读校验 API。
3. Data 导出静态 definitions，Store.Migrate 仅作为兼容入口委托公共 Runner，不能保留锁外 DDL。
4. Server 统一 Root Registry 注册 Data，移除 external namespace 和单独 Data Migration 调度。
5. 补足 MariaDB 集成测试和 Root 启动模式测试。
6. 再继续其他模块跨模块调用链 Review。

## 5. 当前执行任务与边界

### RA-CROSS-REVIEW-003：Data / News → Research 消费链路 Review

- 状态：IN_PROGRESS（只读代码与契约核对阶段）。
- 目标：逐项核对 Data、News 对 Research 暴露的公开 Application Port/DTO、Research Adapter/Importer 实际消费方式，以及 Server 当前装配事实。
- 本轮边界：只审查 Data/News → Research，不扩展到 Quant、Researchflow、LLM、Notification 或 WebUI；不因契约登记为 frozen 就推定实现/生产消费已验收。
- 首先核对：DATA-CTR-007、NEWS-CTR-001/002/003、RSC-CTR-001，以及 News-Research 只读证据查询契约。
- 重点判定：字段来源与时间语义、PIT cutoff/system_as_of、available_at 可比较性、AuthorizationRef 当前策略复核、精确版本读取与 TOCTOU、分页/Cursor 稳定性、错误/降级状态传播、Server DI 是否真实接线。
- 验收输出：每项记录 PASS / DEGRADED / FAIL / NOT_COMPARABLE，并给出代码路径、契约条款、测试/CI SHA；真实来源时间或生产消费者证据缺失时不得标记真实数据 PASS。
- 修改门禁：发现明确可复现缺陷后，先确认现有能力与归属，再在 repair_all 修复；不新增无调用场景的 API/字段，不复制 Platform/模块能力。若无缺陷，仅更新 Review 记录和卡点，不做无意义代码提交。

### 独立外部任务（不阻塞本轮 Review）

- Data Migration enabled/disabled 的生产数据库启停演练：需要真实部署环境和运维窗口；当前代码/CI 通过不能替代该项。
- News 合格 available_at 来源证据、Data 真实 Provider 证据：缺少来源/凭据/数据时保持 NOT_COMPARABLE 或 SHELVED，不制造样本。



## 14. RA-CROSS-REVIEW-003 当前基线校准（2026-11-19）

- repair_all 当前 HEAD：5334043ea8e1abc2c4006c371a3f95b123098946；最新提交为健康数据库 Migration disabled 启动路径回归测试。
- 最新完整 Integration CI：Run [44456052937](https://github.com/Kasign/stock_data_provider/actions/runs/44456052937)，head SHA 与当前 HEAD 精确一致，结论 success。
- 本轮新增测试证明健康物理 Schema 在 Migration disabled 时仍可正常启动；与此前缺列 fail-closed 测试互补，避免只验证拒绝路径而遗漏合法启动误拒绝。
- 当前唯一执行任务切换为 RA-CROSS-REVIEW-003：Data / News → Research 消费链路 Review。Research 模块现有 RSC-001/002/003/007/008 状态和项目级 DATA/NEWS 卡点作为输入，不重复其已通过的模块测试。
- 本任务先只读核对契约、生产代码、Server 装配和现有测试；发现真实缺陷再进入修复。不得以模块单测或历史 SHA 推断当前聚合分支真实生产链路已验收。
- 文档状态权威：本文件记录 repair_all 聚合任务；模块内部任务状态仍以各自模块总任务规划和唯一当前任务状态文档为准。
