# 数据库 Schema 基线盘点

> 状态：第一轮源码盘点完成，字段级最终基线审计进行中  
> 适用分支：`repair_all`  
> 盘点基准：`0e70a9909ba80f2cc7e8e0de75131d07dfa5f292`  
> 原则：新部署使用空数据库和当前最终 Schema 基线；不迁移旧业务数据，不保留旧 Schema 兼容分支。正常启动不得清库或删除表。

## 1. 盘点结论

当前 Root Server Migration Registry 注册了 8 个 namespace；Analysis 的 4 张表仍由旧 Analysis Store 独立创建，尚未纳入 Registry。当前启动顺序先打开 MariaDB、Redis 并构造 Analysis Store，之后才注册和执行 Root Migration，存在 Repository 先于领域 Schema 的问题。

本轮确认必须统一处理的持久化范围：

| 模块 | Namespace | 当前版本 | 表数量 | Migration 定义来源 |
|---|---|---:|---:|---|
| Data / MarketData | `market_data` | V1–V9 | 17 | `internal/marketdata/repository/mysql.go` |
| News | `news` | V1–V10 | 14 | `internal/news/adapters/storage/migrations.go` |
| Platform Owner Credential | `platform.security` | V1 | 1 | `internal/platform/security/credential_store.go` |
| Research | `research` | V1 | 6 | `internal/research/storage/migrations.go` |
| Researchflow | `researchflow` | V1–V6 | 9 | `internal/researchflow/storage/migrations.go` |
| Quant | `quant` | V1–V5 | 5 | `internal/quant/storage/migrations.go` |
| Notification | `notification` | V1–V6 | 10 | `internal/notification/storage/migrations.go` |
| Server Security Audit | `server.security_audit` | V1 | 1 | `internal/server/security/audit_writer.go` |
| Analysis（待纳入） | 建议新增 `analysis` | 独立初始化 | 4 | `internal/adapters/storage/mysqlredis/store.go` |

表数量按当前 Migration DDL 中的业务表统计，不含共享 Migration History 表 `platform_schema_migrations`。总计 67 张业务表；其中 Analysis 的 4 张表尚未进入统一 Registry。

## 2. 各模块当前表清单

### 2.1 Data / MarketData（17 张）

来源：`internal/marketdata/repository/mysql.go`，当前 V1–V9。

- `market_data_provider_probe_runs`
- `market_data_sentiment_snapshots`
- `market_data_quotes`
- `market_data_daily_bars`
- `market_data_daily_bar_versions`
- `market_data_fundamental_versions`
- `market_data_security_trading_status_versions`
- `market_data_adjustment_factor_versions`
- `market_data_corporate_action_versions`
- `market_data_valuation_versions`
- `market_data_trading_calendar_snapshots`
- `market_data_daily_bars_close_series_snapshots`
- `market_data_authorization_policy_versions`
- `market_data_current_authorization_policy_versions`
- `market_data_authorization_decisions`
- `market_data_authorization_revocations`
- `market_data_authorization_policy_publications`

注意：V2 增加调整因子 `source_version` 和对应索引；V4 使用 EnsureColumn / EnsureNonUniqueIndex 处理旧结构；V5–V9 增加日历、收盘序列 Snapshot、授权策略版本、授权决策/撤销及发布审计结构。新基线必须直接定义最终列和索引，不得保留 V4 的兼容操作。Data Repository 还包含物理 Schema 校验，重构后要确认校验规则与新基线一致。

### 2.2 News（14 张）

来源：`internal/news/adapters/storage/migrations.go`，当前 V1–V10。

- `news_items`
- `news_identifiers`
- `news_observations`
- `news_item_versions`
- `news_relation_assertions`
- `news_relation_evidence`
- `news_item_relations`
- `news_authorization_snapshots`
- `news_evidence_snapshots`
- `news_identifier_status_history`
- `news_policy_versions`
- `news_current_policy_versions`
- `news_version_availability_evidence`
- `news_version_availability_decisions`

最终结构必须合并 V4–V10 的变化：标识状态历史、Provider Output Policy 字段、Observation 的 Endpoint/Policy 绑定、关系后继唯一约束、Observation 保留周期、Policy 当前版本指针，以及版本可用性证据/决策表和复合外键。V4 中从旧表回填历史状态的 INSERT 不应进入空库基线。

### 2.3 Platform Owner Credential（1 张）

来源：`internal/platform/security/credential_store.go`，当前 `platform.security` V1。

- `platform_owner_credentials`

最终字段以当前定义为准：`owner_id`、`password_hash`、`credential_version`、`created_at`、`updated_at`；主键为 `owner_id`，并限制 `owner_id = 1`。密码哈希不得出现在日志或诊断输出中。

### 2.4 Research（6 张）

来源：`internal/research/storage/migrations.go`，当前 V1。

- `research_aggregate_heads`
- `research_context_versions`
- `research_evidence_records`
- `research_event_versions`
- `research_claim_versions`
- `research_notification_outbox`

当前模块已经采用最终结构单 V1，无需拆分历史版本。字段级审计仍需对照对应 Repository 的 INSERT/SELECT/UPDATE 列清单。

### 2.5 Researchflow（9 张）

来源：`internal/researchflow/storage/migrations.go`，当前 V1–V6。

- `researchflow_runs`
- `researchflow_stages`
- `researchflow_attempts`
- `researchflow_run_idempotency`
- `researchflow_result_summaries`
- `researchflow_outbox`
- `researchflow_workflow_templates`
- `researchflow_capacity_guard`
- `researchflow_stage_output_resources`

最终结构必须合并 V2 Outbox Lease/重试字段与索引、V3 Workflow Template、V4 Capacity Guard、V5 Run 幂等唯一约束、V6 Stage Output Resource 及其外键/CHECK 约束。V4 的 Guard 初始行属于运行数据初始化，不应误当成 Schema 定义；基线执行是否需要插入该固定行，应作为显式、可重复且有唯一约束保护的初始化步骤审查。

### 2.6 Quant（5 张）

来源：`internal/quant/storage/migrations.go`，当前 V1–V5。

- `quant_results`
- `quant_access_audit`
- `quant_result_access_policy_versions`
- `quant_result_access_policy_head`
- `quant_policy_admin_audit`

最终结构必须合并 V3 的 Run Scope 字段/索引、V4 授权决策证据字段、V5 不可变授权策略版本/Head 和 Owner 管理审计。V5 中 Head 的初始数据行是初始化数据，不是表结构；需单独审查是否仍由业务初始化逻辑负责。

### 2.7 Notification（10 张）

来源：`internal/notification/storage/migrations.go`，当前 V1–V6。

- `notification_subscriptions`
- `notification_delivery_plans`
- `notification_deliveries`
- `notification_delivery_attempts`
- `notification_delivery_state_events`
- `notification_inbox`
- `notification_template_versions`
- `notification_channel_config_versions`
- `notification_subscription_revision`
- `notification_fanout_work_items`

最终结构必须包含 V5 删除冗余索引后的状态，以及 V6 的订阅 revision 字段/索引、revision 单例表和事务性 fanout work item 表。不得将已删除的 `idx_notification_channel_config_kind` 放回新基线。

### 2.8 Server Security Audit（1 张）

来源：`internal/server/security/audit_writer.go`，当前 `server.security_audit` V1。

- `server_security_audit_events`

当前字段与索引以 Migration 定义为准；审计数据保留/归档策略未定义，本次不擅自增加清理策略。

### 2.9 Analysis（4 张，待纳入 Registry）

来源：`internal/adapters/storage/mysqlredis/store.go` 的 `Migrate()`，当前不受 Root Registry 管理。

- `analysis_runs`
- `analysis_stage_events`
- `analysis_snapshots`
- `analysis_tasks`

这些表必须迁入 Analysis 自有 Migration namespace（建议 `analysis`）并以完整最终结构 V1 创建。删除 Store 内的 `initializeSchema()` / `Migrate()` 自动建表入口，避免保留第二套 Schema 管理机制。

## 3. 当前启动与 Migration 机制问题

1. `internal/server/bootstrap/bootstrap.go` 先调用 `OpenLegacyAnalysisResources()`，它会打开 MariaDB、Redis 并创建 Analysis Store；Store 会先执行 `initializeSchema()`。之后才创建 Registry 并根据 `MIGRATION_ENABLED` 执行或只读校验。
2. `internal/server/bootstrap/owner_session_init.go` 走另一套相同模式：先打开 MariaDB/Redis 和 Analysis Store，再按 `MIGRATION_ENABLED` 执行/校验 Migration。
3. `internal/platform/storage/storage.go` 创建 History 表时仍使用 `CREATE TABLE IF NOT EXISTS`，并通过三条 `ADD COLUMN IF NOT EXISTS` 补齐旧 History 字段。这些 History 结构兼容步骤应删除，改为单一完整定义。
4. `DB_AUTO_MIGRATE` 仍由 `internal/adapters/storage/mysqlredis/config.go` 读取；`MIGRATION_ENABLED` 仍由 `internal/server/config/migration.go` 读取。两项都应从配置结构、模板、文档和部署脚本中整体移除。
5. `MIGRATION_TIMEOUT` 是迁移执行时限，不属于模式开关，保留。
6. Registry 按 namespace 字典序分别执行、分别加锁。不同 namespace 之间不是一个原子事务；中途失败时已成功 namespace 会保留。MariaDB DDL 可能隐式提交，不能承诺跨模块回滚。失败状态继续 fail-closed，需人工检查后恢复，不自动重跑失败版本。

## 4. 新基线目标结构

- 保留现有 namespace 名称，新增 `analysis`；每个 namespace 只保留一条 V1 Migration。
- V1 直接创建该模块当前最终 Schema，包括全部列、默认值、主键、唯一约束、普通索引、外键和 CHECK 约束。
- 不在新基线中执行旧数据回填 SQL；固定运行所需的初始记录应与 DDL 分离，并明确由哪个模块初始化。
- 不保留 EnsureColumn、EnsureNonUniqueIndex 等旧结构修补操作。
- `platform_schema_migrations` 仍然保留，作为未来增量迁移的唯一历史账本；新数据库由完整定义一次创建，不对已有 History 表做自动补列。
- 正常 Server 启动与 `owner-init` 共用同一个 Database Bootstrap：先打开 MariaDB SQL 连接、注册并执行 Migration、完成 History/checksum 校验，再创建 Redis、Repository、Worker 和 HTTP 依赖。
- 正常启动绝不 DROP DATABASE、TRUNCATE 或自动清理业务表。旧库切换到新基线必须由部署者显式准备空数据库。

## 5. 字段级审计状态与下一步

本文件完成了 namespace、版本范围、表清单、已知版本变更和启动路径的第一轮盘点；它不是最终字段级 Schema 清单。接下来逐模块对照 Migration DDL 与 Repository 实际 SQL，补齐每张表的字段/类型/NULL/默认值、PK/UK/索引/FK、读写代码位置，并核实所有运行时查询依赖的列是否都在最终 DDL 中。

执行顺序：
1. Data：17 张表，优先核实 V1–V9 最终字段、V4 旧索引替换和 Data Schema 校验。
2. News：14 张表，核实 V4–V10 合并字段与复合外键。
3. Researchflow、Quant、Notification：逐表核对 Repository SQL 和后续 Migration 变更。
4. Research、Owner Credential、Server Security Audit、Analysis：完成字段级核对并纳入最终清单。
5. 全部字段级审计完成后，才生成新 V1 DDL 和统一启动实现。

本阶段不修改业务 Migration、不删除兼容代码、不执行真实数据库操作。
