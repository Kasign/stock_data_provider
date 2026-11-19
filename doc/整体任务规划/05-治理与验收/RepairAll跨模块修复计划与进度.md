# Repair All 跨模块修复计划与进度

- 仓库：`Kasign/stock_data_provider`
- 工作分支：`repair_all`
- 当前代码验收基线：`5d97dc89acfd0493b8b7f4c316db6a5d5dc609e9`
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
| `a1d56c1f7034d2c77ecfee9bc6daf574237f9847` | 代码已纳入后续验收 | RA-NEWS-001 代码提交；由后续 repair_all CI 覆盖 |
| `ef3fa3acca68dd108098f324e077d22732c58582` | CI 运行中 | 为 repair_all 增加 Integration CI push 触发；Run [44446102934](https://github.com/Kasign/stock_data_provider/actions/runs/44446102934) |



### RA-CI-001：repair_all 完整集成 CI 触发

- 原因：原 `.github/workflows/integration-ci.yml` 仅监听 `integration` 分支 push；repair_all 的代码提交不会自动触发全量验收。
- 修改：将 `repair_all` 加入同一工作流 push 分支列表，复用现有全量检查，不复制 CI 工作流。
- 提交：`ef3fa3acca68dd108098f324e077d22732c58582`。
- Run：[44446102934](https://github.com/Kasign/stock_data_provider/actions/runs/44446102934)，当前查询为 in_progress；尚未得到最终结论。


## 7. RA-MIGRATION-001：Data Migration 三方统一（2026-11-19）

<badge color="success">完整 Integration CI 已通过</badge>

### 7.1 已实施内容

- Platform `Migration` 增加可校验的 `MigrationOperation`，当前支持：
  - `ensure_column`：在 namespace lock 内查询 information_schema，缺失时补列。
  - `ensure_non_unique_index`：在 namespace lock 内清理旧唯一索引/错误唯一索引，并确保目标非唯一索引存在。
- Migration operation 描述纳入 SHA-256 checksum。没有 operations 的旧迁移不增加任何 hash 字节，因此 Data 已发布 V1-V3 的 statement checksum 算法保持不变。
- Apply 与只读校验共用 `acquireMigrationLock`，锁绑定单个物理连接；无法确认锁释放时丢弃连接，避免带锁连接回到连接池。
- Platform 新增 `ValidateAppliedMigrations`：复用 `prepareMigrations`，只读检查 namespace 的版本集合、版本连续性、状态和 checksum；不创建 history 表、不执行 Schema DDL、不改 history。
- Data `repository.MigrationDefinitions()` 导出静态 V1-V4 定义：V1-V3 SQL 保持原样；V4 将两组历史索引修复和 `market_data_quotes.volume_ratio` / `turnover_rate` 补列转为四个可校验操作。
- `Store.Migrate` 仅作为兼容入口委托 Platform Runner，不再保留锁外兼容 DDL。
- Data Bootstrap 暴露静态 Migration definitions；Server Root Registry 正式注册 `market_data`。
- Root `MIGRATION_ENABLED=true` 统一由 Registry 执行所有已注册 namespace；关闭时 Registry 通过 Platform 只读校验覆盖 Data checksum，不再单独调用 Data MigrationRunner，也不再使用 `market_data` external namespace 过渡登记。

### 7.2 主要提交

| SHA | 提交 | 主要变更 |
|---|---|---|
| `172de9aded2a97097b665fdd1d93cae822e9fe28` | feat(platform): 支持可校验的幂等迁移操作 | MigrationOperation、checksum、锁内执行 |
| `880166409194cacb417a9d359e127783efc6c6dd` | feat(platform): 增加只读迁移 checksum 校验 | Platform 只读校验入口 |
| `fe050ac842dcbbb97c9d79b981e2c7fd869b4a01` | refactor(data): 导出静态 Schema 迁移定义 | Data 静态定义、V4 兼容操作 |
| `1c303ecfeb0257205d2636f6b3959e8ddde9de21` | refactor(server): 统一注册并校验 Data Migration | Root Registry 统一执行/校验 |
| `3f5e193b494df727b44035e1dc9f48591a63692e` | refactor(platform): 复用迁移命名空间锁生命周期 | Apply/Validate 共用锁 helper |
| `aeed898da826f8952fb235ff57b0ef19042f8c64` | test(server): 验证 Data 旧库迁移与多实例并发 | 旧库兼容、双实例并发和 checksum 篡改集成测试 |

### 7.3 验收边界

- 已新增单测：旧 statement-only checksum 稳定、operations 改动导致 checksum 改变、非法操作拒绝、Data V1-V3 不变、V4 仅含兼容操作、Registry 接受 operations-only migration。
- 已新增真实 MariaDB 集成用例：先应用 Data V1-V3，再模拟旧库缺列/旧唯一索引，两个 Root Registry 并发执行完整定义，最后验证 V4 兼容结果和 history。
- 已扩展 MariaDB Registry 集成用例：篡改已应用 checksum 后，`ValidateApplied` 必须拒绝。
- 上述新测试已由后续精确 SHA `5d97dc89acfd0493b8b7f4c316db6a5d5dc609e9` 的完整 Integration CI 证实，Run [44452547201](https://github.com/Kasign/stock_data_provider/actions/runs/44452547201) success。
- Migration 三方统一的早期完整基线为 Run [44450277491](https://github.com/Kasign/stock_data_provider/actions/runs/44450277491)，HEAD `5b9ebe6a2b8103b4c63a48c3c0f0ddb004d583ec`，success；物理 Schema 门禁与其后的测试修正由 Run [44452547201](https://github.com/Kasign/stock_data_provider/actions/runs/44452547201) 在精确 SHA `5d97dc89acfd0493b8b7f4c316db6a5d5dc609e9` 上完整验收。
- 物理 Schema drift 仍不是 checksum 校验的职责；必须由 Data Schema Diagnostics 检测。本轮新增测试也不等于生产数据库部署演练。

### 7.4 下一步

1. Migration 代码和 CI 验收已完成；仍需独立完成生产环境 Migration enabled/disabled 演练及物理 Schema drift Diagnostics 注册验收。
2. News Validator uniqueItems 缺口已修复并在完整 Integration CI 通过；Researchflow RF-BLOCK-009 在 repair_all 聚合基线解除。
3. 继续按 Data/News→Research→Quant→Researchflow→LLM→Server→WebUI/Notification 调用链 Review；真实外部凭据/Provider 证据缺失时保持 NOT_COMPARABLE，不造数据。


## 8. RA-CROSS-REVIEW-002：News / Researchflow 集成契约复核（2026-11-19）

- 修正 `doc/news/新闻API契约.md` 第 11.5、11.9 节的过期状态：同进程 `EvidenceQueryFacade`、单条 GetEvidence、ListEvidence、固定 Snapshot、PIT Repository 与授权复核已有实现，且 Server 已装配给 Research Adapter；这不等于独立 HTTP API。
- 严格 PIT 的合格 `available_at` 来源证据仍缺失。当前已接入来源不能提供合格首次公开可获取时间，News DTO 返回 null，News→Research B 组继续 SHELVED；没有将 published_at/fetched_at/observed_at 代替 available_at。
- News Search Payload Validator 的 `uniqueItems` 缺口已在 repair_all 修复，Researchflow StageOutputRegistry 复用修复后的 Validator；RF-BLOCK-009 在 repair_all 基线解除。
- 完整验收基线：`5b9ebe6a2b8103b4c63a48c3c0f0ddb004d583ec`，Run [44450277491](https://github.com/Kasign/stock_data_provider/actions/runs/44450277491) success。
- 当前继续执行：Migration 生产启停演练/物理 Schema Diagnostics；再审 Research/Quant/Researchflow/LLM/Server/WebUI/Notification 的真实生产调用链。模块分支不因本次聚合分支修复而自动改变。


## 9. RA-DATA-SCHEMA-001：Data 物理 Schema 漂移就绪门禁（2026-11-19）

- Review 发现：Platform `ValidateAppliedMigrations` 只验证 migration history/checksum；Data 原 `Ready` 仅 Ping MariaDB/Redis，不检查 V4 引入的物理列/索引是否被人工删除或改型。
- Data Repository 新增 `Store.ValidateCompatibilitySchema`，只读检查 `market_data_quotes.volume_ratio`、`turnover_rate` 必须为 nullable `DECIMAL(16,8)`；检查 DailyBar/Fundamental revision 索引列序、非唯一属性和 legacy unique index 不存在。
- Data `Ready` 将物理结构检查纳入 MariaDB readiness；Schema 漂移返回 `market_data.schema_drift`，不将 Migration checksum 当作物理结构证明。
- Server Root 在 `MIGRATION_ENABLED=true/false` 两条路径完成各自 Migration 执行/只读校验后，都会在启动 Worker 或接受流量前执行 Data 依赖与物理 Schema 就绪检查。此前只在 disabled 路径调用 Ready 的不一致已收敛。
- MariaDB 集成测试已增加：先通过 Root Registry 执行 Data V1-V4，再验证新迁移 Schema Ready、删除兼容列/索引后 Ready fail-closed、恢复后 Ready 通过；Run [44452547201](https://github.com/Kasign/stock_data_provider/actions/runs/44452547201) success。


## 10. RA-DATA-SCHEMA-001 验收结果与测试修正（2026-11-19）

- 最终代码基线：`5d97dc89acfd0493b8b7f4c316db6a5d5dc609e9`。
- 完整 Integration CI：[Run 44452547201](https://github.com/Kasign/stock_data_provider/actions/runs/44452547201)，精确 SHA 匹配，结论 `success`。
- 全部检查通过：Go 格式、根模块 vet/tests、News/Research/Researchflow 跨模块测试、Worker MariaDB/Redis 集成、go-core vet/tests、Compose、WebUI 语法/单测/Docker 构建。
- 本轮修正了两处 CI 暴露的问题：`mysql.go` 注释格式；`TestRegistryMariaDBIntegration` 原先只执行测试专用 Migration，却在未创建 Data 表的数据库上断言 Data Ready。现测试将 Data 静态 Migration definitions 注册到同一 Root Registry，执行成功后再检查 Data Ready 和物理 Schema 漂移。
- 新增测试没有放宽 Schema 规则。真实 MariaDB Worker Bootstrap 集成 Job 与根模块测试均通过。
- 结论：代码层面的 Data V4 物理 Schema 就绪门禁在当前聚合 SHA 上通过；生产数据库 Migration enabled/disabled 演练仍未执行，不能据 CI 关闭生产演练卡点。

当前下一步：继续跨模块生产调用链 Review，优先检查 Server 真实装配边界和各模块公开 Application Port；外部来源、凭据、消费者契约缺失时继续保留 BLOCKED/NOT_COMPARABLE。


## 11. RA-DATA-SCHEMA-001 物理漂移与 Migration Checksum 边界验收（2026-11-19）

- 最新代码 SHA：`ff9a98457220ae307b9dc4a62ce274849cd60f12`。
- 完整 Integration CI：[Run 44453457919](https://github.com/Kasign/stock_data_provider/actions/runs/44453457919)，精确 SHA 匹配，结论 `success`。
- MariaDB 测试进一步证明：删除 V4 兼容列或 revision 索引后，Root Registry 的只读 Migration checksum 校验仍通过（迁移定义与 history 未变化），但 Data Ready 必须返回精确 Fault `market_data.schema_drift`。
- 该断言明确区分“Migration 定义/执行历史完整性”和“数据库物理 Schema 完整性”，避免将其中一种检查误当作另一种检查。
- 全量检查：gofmt、root vet/tests、News/Research/Researchflow 跨模块测试、Worker MariaDB/Redis、go-core vet/tests、Compose、WebUI syntax/tests/build 全部 success。
- 结论：当前代码级漂移检测与 Fault 语义验收通过；生产环境启停演练仍未完成。
