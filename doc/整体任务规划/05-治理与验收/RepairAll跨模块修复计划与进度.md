# Repair All 跨模块修复计划与进度

- 仓库：`Kasign/stock_data_provider`
- 工作分支：`repair_all`
- 当前基线：`a1d56c1f7034d2c77ecfee9bc6daf574237f9847`
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
| P0 | Data Migration 定义、Platform 只读校验、Server Root 统一注册与启动路径 | Data / Platform / Server | REVIEWED，待实施 |
| P0 | News Search Candidates Validator 与冻结 Schema 一致性 | News / Researchflow | 已修复，待 CI |
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
- 验收：尚未获得 repair_all 分支 CI 结果；不能标记 PASS。

## 4. P0 Migration 代码 Review 结论

当前代码存在完整且可复现的三方缺口：

1. **Data**
   - `internal/marketdata/bootstrap.MigrationRunner` 只公开 `Migrate(ctx)`，没有暴露静态 Migration definitions。
   - `internal/marketdata/repository/mysql.go` 的 V1-V3 通过 Platform Runner 执行后，仍在锁外执行 `ensureNonUniqueHistoryIndex` 和 `ensureColumn`。
   - 因此这些兼容 DDL 不在同一 namespace advisory lock、Migration history 和 checksum 保护范围内。
2. **Platform**
   - `internal/platform/storage/storage.go` 已有 `prepareMigrations`、SHA-256 checksum、namespace advisory lock 和 Apply Runner。
   - 缺少只读的、与 Apply 使用同一 checksum preparation 的 `ValidateAppliedMigrations` 入口。
3. **Server**
   - `internal/server/bootstrap/bootstrap.go` 先调用 Data `MigrationRunner.Migrate`，再执行 Root Registry。
   - `market_data` 通过 `RegisterExternalNamespace` 过渡登记，Root 无法验证 Data 的预期版本集合和 checksum。
   - Migration 关闭时 Root 只校验静态 Registry，Data 的 checksum 仍未验证。

### 实施约束

- 保留已经发布的 Data V1-V3 SQL 和 checksum，不得编辑旧版本定义。
- 将历史兼容 DDL 迁入新的 Data Migration 版本；必须兼容全新数据库和已升级数据库。
- 条件式 DDL 不能继续留在 Apply Runner 锁外。实现必须在 Platform namespace lock 内执行，并且其定义必须纳入可校验的版本/checksum；不得靠 Server 复制 Data SQL。
- Platform 只读校验必须复用 `prepareMigrations`，只读检查 history，不创建 history 表、不执行 DDL、不修改记录；至少检查缺失版本、未知版本、非 applied 状态、版本空洞和 checksum 不一致。
- Server 通过 Data 导出的静态定义注册 `market_data`，启用时只通过 Root Registry 执行，禁用时只通过 Platform 只读校验；移除 external namespace 过渡登记。
- checksum 只证明已登记 Migration 定义未被篡改，不等于物理 Schema 完整性；物理 Schema 漂移应由 Data 的 Schema Diagnostics 负责，不将两种校验混为一谈。
- 验收必须覆盖新库、从 V1/V2/V3 升级的旧库、双实例并发、Migration enabled/disabled、缺失/未知版本、checksum 篡改、失败迁移恢复和物理 Schema 漂移诊断。

## 5. 下一步任务

1. 先完成 Data Migration 定义的重构设计，重点解决条件式兼容 DDL 如何纳入不可变版本与 Platform 锁/checksum 保护；不直接将当前 Go helper 粗暴搬成不具备幂等语义的 ALTER SQL。
2. 扩展 Platform Migration 定义/执行能力，使条件式 Schema 操作可以在统一 namespace lock 下执行，且操作描述纳入 checksum；同时提供只读校验 API。
3. Data 导出静态 definitions，Store.Migrate 仅作为兼容入口委托公共 Runner，不能保留锁外 DDL。
4. Server 统一 Root Registry 注册 Data，移除 external namespace 和单独 Data Migration 调度。
5. 补足 MariaDB 集成测试和 Root 启动模式测试。
6. 再继续其他模块跨模块调用链 Review。

## 6. 验收记录

| SHA | 结果 | 说明 |
|---|---|---|
| `37d37f3107befbd42c98891917efc608c354330b` | 历史 CI success | Integration CI Run 44443549022；该 SHA 的全量检查通过 |
| `a1d56c1f7034d2c77ecfee9bc6daf574237f9847` | 待验收 | RA-NEWS-001 提交后尚未观察到 repair_all 对应 Actions Run |

