# 数据库 Schema 基线盘点

> 状态：第一轮源码盘点完成，字段级最终基线审计进行中  
> 适用分支：`repair_all`  
> 盘点基准：`0e70a9909ba80f2cc7e8e0de75131d07dfa5f292`  
> 原则：新部署使用空数据库和当前最终 Schema 基线；不迁移旧业务数据，不保留旧 Schema 兼容分支。正常启动不得清库或删除表。

## 1. 盘点结论

当前 Root Server Migration Registry 注册了 8 个 namespace；Analysis 的 4 张表仍由旧 Analysis Store 独立创建，尚未纳入 Registry。当前启动顺序先打开 MariaDB、Redis 并构造 Analysis Store，之后才注册和执行 Root Migration，存在 Repository 先于领域 Schema 的问题。

本轮确认必须统一处理的持久化范围：

| 模块 | Namespace | 重建前历史版本范围 | 新基线版本 | 表数量 | Migration 定义来源 |
|---|---|---:|---:|---|
| Data / MarketData | `market_data` | V1–V9 | V1 | 17 | `internal/marketdata/repository/mysql.go` |
| News | `news` | V1–V10 | V1 | 14 | `internal/news/adapters/storage/migrations.go` |
| Platform Owner Credential | `platform.security` | V1 | V1（保持） | 1 | `internal/platform/security/credential_store.go` |
| Research | `research` | V1 | V1（保持） | 6 | `internal/research/storage/migrations.go` |
| Researchflow | `researchflow` | V1–V6 | V1 | 9 | `internal/researchflow/storage/migrations.go` |
| Quant | `quant` | V1–V5 | V1 | 5 | `internal/quant/storage/migrations.go` |
| Notification | `notification` | V1–V6 | V1 | 10 | `internal/notification/storage/migrations.go` |
| Server Security Audit | `server.security_audit` | V1 | V1（保持） | 1 | `internal/server/security/audit_writer.go` |
| Analysis | `analysis` | 独立初始化 | V1（新增） | 4 | `internal/adapters/storage/mysqlredis/store.go` |

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

## 6. Data / MarketData 字段级审计（第一轮完成）

审计来源：`internal/marketdata/repository/mysql.go` 的 V1–V9 Migration DDL、`internal/marketdata/repository/*.go` 的实际 SQL、以及 Data Repository 的物理 Schema 校验。当前 Data 结构已确认 17 张表；以下列出最终字段和关键索引。类型、NULL、默认值以源 DDL 为准。

| 表 | 最终字段 | 主键 / 唯一约束 | 关键索引 / 外键 |
|---|---|---|---|
| `market_data_provider_probe_runs` | `id BIGINT UNSIGNED AUTO_INCREMENT`、`run_id VARCHAR(64)`、`started_at/finished_at DATETIME(6)`、`status VARCHAR(32)`、`metadata JSON` | PK id；UK run_id | started_at |
| `market_data_sentiment_snapshots` | `id BIGINT UNSIGNED AUTO_INCREMENT`、`market VARCHAR(32)`、`trade_date DATE`、`fetched_at DATETIME(6)`、`source VARCHAR(128)`、`quality_status VARCHAR(16)`、`metadata JSON` | PK id | (market,trade_date,id)、fetched_at |
| `market_data_quotes` | `id BIGINT UNSIGNED AUTO_INCREMENT`、`symbol VARCHAR(16)`、`quote_time DATETIME(6) NULL`、`fetched_at/normalized_at DATETIME(6)`、`source VARCHAR(64)`、价格列 current_price/change_value/open_price/high_price/low_price/prev_close 为 `DECIMAL(20,8) NULL`、`change_percent DECIMAL(16,8) NULL`、`volume_shares BIGINT NULL`、`amount_cny DECIMAL(24,2) NULL`、`volume_ratio/turnover_rate DECIMAL(16,8) NULL`、`quality_status VARCHAR(16)`、`metadata JSON` | PK id | (symbol,quote_time)、fetched_at |
| `market_data_daily_bars` | `id BIGINT UNSIGNED AUTO_INCREMENT`、`symbol VARCHAR(16)`、`trade_date DATE`、`period VARCHAR(8) DEFAULT '1d'`、`adjustment_basis VARCHAR(24)`、`source VARCHAR(64)`、OHLC `DECIMAL(20,8)`、`volume_shares BIGINT NULL`、`amount_cny DECIMAL(24,2) NULL`、`change_percent DECIMAL(16,8) NULL`、`fetched_at/normalized_at DATETIME(6)`、`quality_status VARCHAR(16)`、`metadata JSON` | PK id；UK (symbol,trade_date,period,adjustment_basis,source) | (symbol,trade_date,adjustment_basis) |
| `market_data_daily_bar_versions` | `id BIGINT UNSIGNED AUTO_INCREMENT`、`symbol VARCHAR(16)`、`trade_date DATE`、`period VARCHAR(8) DEFAULT '1d'`、`adjustment_basis VARCHAR(24)`、`source VARCHAR(64)`、`revision_hash CHAR(64) ascii_bin`、`recorded_at DATETIME(6)`、OHLC `DECIMAL(20,8)`、`volume_shares BIGINT NULL`、`amount_cny DECIMAL(24,2) NULL`、`change_percent DECIMAL(16,8) NULL`、`fetched_at/normalized_at DATETIME(6)`、`quality_status VARCHAR(16)`、`metadata JSON` | PK id；无内容唯一约束 | (symbol,trade_date,period,adjustment_basis,source,recorded_at,id)、(symbol,trade_date,adjustment_basis,recorded_at)、recorded_at |
| `market_data_fundamental_versions` | `id BIGINT UNSIGNED AUTO_INCREMENT`、`symbol VARCHAR(16)`、`report_year SMALLINT`、`report_quarter TINYINT`、`source VARCHAR(64)`、`revision_hash CHAR(64) ascii_bin`、`recorded_at/fetched_at DATETIME(6)`、`quality_status VARCHAR(16)`、`metadata JSON` | PK id；无内容唯一约束 | (symbol,report_year,report_quarter,recorded_at)、recorded_at、(symbol,report_year,report_quarter,source,recorded_at,id) |
| `market_data_security_trading_status_versions` | `id BIGINT UNSIGNED AUTO_INCREMENT`、`symbol VARCHAR(16)`、`trade_date DATE`、`source VARCHAR(64)`、`revision_hash CHAR(64) ascii_bin`、`recorded_at/fetched_at DATETIME(6)`、`quality_status VARCHAR(16)`、`metadata JSON` | PK id | (symbol,trade_date,source,recorded_at,id)、(symbol,trade_date,recorded_at)、recorded_at |
| `market_data_adjustment_factor_versions` | `id BIGINT UNSIGNED AUTO_INCREMENT`、`symbol VARCHAR(16)`、`trade_date DATE`、`source VARCHAR(64)`、**`source_version VARCHAR(128) NOT NULL DEFAULT ''`**、`factor_basis VARCHAR(16)`、`revision_hash CHAR(64) ascii_bin`、`recorded_at/fetched_at DATETIME(6)`、`available_at DATETIME(6) NULL`、`quality_status VARCHAR(16)`、`metadata JSON` | PK id | (symbol,trade_date,source,factor_basis,recorded_at,id)、(symbol,trade_date,source,source_version,factor_basis,recorded_at,id)、recorded_at |
| `market_data_corporate_action_versions` | `id BIGINT UNSIGNED AUTO_INCREMENT`、`symbol VARCHAR(16)`、`provider_event_id VARCHAR(128)`、`source VARCHAR(64)`、`ex_date/announcement_date DATE NULL`、`revision_hash CHAR(64) ascii_bin`、`recorded_at/fetched_at DATETIME(6)`、`available_at DATETIME(6) NULL`、`quality_status VARCHAR(16)`、`metadata JSON` | PK id | (symbol,ex_date,source,recorded_at,id)、(symbol,provider_event_id,source,recorded_at,id)、(symbol,announcement_date)、recorded_at |
| `market_data_valuation_versions` | `id BIGINT UNSIGNED AUTO_INCREMENT`、`request_fingerprint CHAR(64) ascii_bin`、`source VARCHAR(64)`、`revision_hash CHAR(64) ascii_bin`、`recorded_at/fetched_at DATETIME(6)`、`quality_status VARCHAR(16)`、`metadata JSON` | PK id | (request_fingerprint,recorded_at)、(request_fingerprint,source,recorded_at,id)、recorded_at |
| `market_data_trading_calendar_snapshots` | `calendar_ref VARCHAR(96) ascii_bin`、`content_fingerprint CHAR(64) ascii_bin`、`market VARCHAR(32) ascii_bin`、`start_date/end_date DATE`、`source VARCHAR(64) ascii_bin`、`adapter_version VARCHAR(128) ascii_bin`、`schema_version VARCHAR(16) ascii_bin`、`quality_status VARCHAR(16) ascii_bin`、`fetched_at/observed_at DATETIME(6)`、`payload_json LONGTEXT utf8mb4_bin` | PK calendar_ref | (market,start_date,end_date,observed_at) |
| `market_data_daily_bars_close_series_snapshots` | `snapshot_ref VARCHAR(96) ascii_bin`、`manifest_fingerprint/payload_fingerprint CHAR(64) ascii_bin`、`manifest_schema_version/payload_schema_version VARCHAR(16) ascii_bin`、`manifest_json/payload_json LONGTEXT utf8mb4_bin`、`created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6)` | PK snapshot_ref | 无额外索引 |
| `market_data_authorization_policy_versions` | `policy_type VARCHAR(32) ascii_bin`、`policy_ref VARCHAR(128) ascii_bin`、`policy_version VARCHAR(128) ascii_bin`、`payload_json LONGTEXT utf8mb4_bin`、`payload_hash CHAR(64) ascii_bin`、`created_at DATETIME(6)` | 复合 PK (policy_type,policy_ref,policy_version) | (policy_type,policy_ref,created_at) |
| `market_data_current_authorization_policy_versions` | `policy_type VARCHAR(32) ascii_bin`、`policy_ref VARCHAR(128) ascii_bin`、`policy_version VARCHAR(128) ascii_bin`、`updated_at DATETIME(6)` | 复合 PK (policy_type,policy_ref) | (policy_type,policy_ref,policy_version)；复合 FK 指向策略版本表相同三列 |
| `market_data_authorization_decisions` | `authorization_ref/subject_ref/resource_ref VARCHAR(256) utf8mb4_bin`、`purpose_code VARCHAR(32) ascii_bin`、`policy_version VARCHAR(128) utf8mb4_bin`、`decided_at DATETIME(6)`、`expires_at DATETIME(6) NULL`、`decision_fingerprint/record_hash CHAR(64) ascii_bin`、`authorization_json/policy_snapshot_json LONGTEXT utf8mb4_bin` | PK authorization_ref | (subject_ref,resource_ref,purpose_code)、expires_at |
| `market_data_authorization_revocations` | `authorization_ref VARCHAR(256) utf8mb4_bin`、`revoked_at DATETIME(6)`、`revoked_by VARCHAR(256) utf8mb4_bin`、`reason_code VARCHAR(128) ascii_bin` | PK authorization_ref | revoked_at；FK authorization_ref 指向授权决策表 |
| `market_data_authorization_policy_publications` | `id BIGINT UNSIGNED AUTO_INCREMENT`、`policy_type VARCHAR(32) ascii_bin`、`policy_ref VARCHAR(128) ascii_bin`、`previous_policy_version VARCHAR(128) utf8mb4_bin NULL`、`new_policy_version VARCHAR(128) utf8mb4_bin`、`payload_hash CHAR(64) ascii_bin`、`actor_ref VARCHAR(256) utf8mb4_bin`、`request_id VARCHAR(128) ascii_bin`、`occurred_at DATETIME(6)` | PK id | (occurred_at,id)、request_id、(policy_type,policy_ref,new_policy_version) |

### Data Repository SQL 对照

| Repository 文件 | 实际 SQL 依赖 |
|---|---|
| `mysql.go` | Quotes 全部行情列；Daily Bars 的 symbol/date/period/adjustment/source/OHLC/volume/amount/change/fetch/normalize/quality/metadata；Daily Bar 与 Fundamental revision 的身份列、revision_hash、recorded_at；Quotes 清理依赖 fetched_at |
| `corporate_actions.go` | Adjustment Factor 的 source_version、factor_basis、available_at、revision_hash、recorded_at、fetched_at；Corporate Action 的 provider_event_id、ex_date、announcement_date、available_at 等 |
| `security_trading_status.go` | symbol/trade_date/source/revision_hash/recorded_at/fetched_at/quality_status/metadata |
| `trading_calendar.go` | 日历 Snapshot 全部身份、版本、质量、时间和 payload 列；并验证 PK 与 identity lookup index |
| `daily_bars_close_series_snapshot.go` | Snapshot Ref、两个 fingerprint、两个 schema version、manifest/payload JSON、created_at；并验证字段类型及主键 |
| `market_sentiment.go` | market/trade_date/fetched_at/source/quality_status/metadata；按 market/date 取最新 id |
| `provider_probe.go` | run_id/started_at/finished_at/status/metadata；依赖 run_id 唯一键和 started_at 排序 |
| `data_authorization_policy.go` | 策略版本表、当前指针表、发布审计表的写入与查询字段；当前指针通过复合 FK 锚定版本 |
| `data_authorization_decision.go` | 授权决策记录及撤销记录的全部写入/读取字段 |
| `data_authorization_snapshot.go` | 授权决策 JSON、撤销状态、当前策略版本和 Close Series Snapshot 读取 |

### Data 新基线必须保留的最终结构规则

1. `market_data_quotes.volume_ratio` 和 `turnover_rate` 均为可空、有符号 `DECIMAL(16,8)`。当前 V1 DDL 已有这两列；V4 EnsureColumn 是旧结构补齐，不代表新基线要再执行 ALTER。
2. `market_data_adjustment_factor_versions.source_version` 是 V2 后的正式身份维度，最终列为 `VARCHAR(128) NOT NULL DEFAULT ''`；必须保留 V2 对应的复合索引。V2 中从 JSON metadata 回填旧记录的 UPDATE 属于历史数据迁移，新空库基线不执行。
3. Daily Bar 与 Fundamental revision 表的内容唯一约束已被当前模型移除；新基线应使用当前 DDL 的非唯一身份/时间索引，不能重新加入旧的 `uq_market_bar_revision_content` 或 `uq_market_fundamental_revision_content`。V4 的 LegacyName 删除行为不进入新基线。
4. V4 其余 EnsureColumn / EnsureNonUniqueIndex 操作也不进入新基线；最终列和目标索引直接体现在 V1 DDL。
5. Trading Calendar、Close Series Snapshot、授权策略版本/当前指针、授权决策/撤销、策略发布审计是 V5–V9 的正式业务结构，必须全部纳入 Data 新 V1。
6. 当前 `ValidateCompatibilitySchema` 不只是历史兼容代码：它还校验 Quotes 列精度/可空性、两个 revision 索引的非唯一性及列顺序、Calendar/Close Series 主键和列类型、授权策略 Schema。后续应改为面向最终基线的 `ValidateSchema`（或等价名称），保留有效物理结构校验并更新注释。
7. `Store.Migrate()` 仍直接调用 Platform Migration Runner，是 Data 的独立迁移入口。统一 Root Bootstrap 完成后，应移除该入口及调用方，避免 Registry 外再次运行 Data Migration。
8. 当前 DDL 使用 `CREATE TABLE IF NOT EXISTS`。新空库基线应使用明确的 `CREATE TABLE`，让意外存在的同名旧表立即报错，而不是悄悄接纳未经确认的结构。

Data 字段级审计状态：**通过第一轮**。尚未对所有字段逐个与上层 DTO/Provider 映射做语义验证；该项属于数据契约/业务字段审计，不阻塞本轮 Schema 重建，但不得借 Schema 基线改动字段语义或删字段。

## 7. News 字段级审计（第一轮完成）

审计来源：`internal/news/adapters/storage/migrations.go` V1–V10，以及 `internal/news/adapters/storage/mysql_repository.go`、`authorization_policy_resolver.go`、`authorization_snapshot.go`、`evidence_authorization_read.go`、`evidence_query.go`、`evidence_snapshot.go`、`identity_history.go`、`news_item_relation_query.go`、`policy_version.go` 的实际 SQL。

### News 最终表字段与约束

| 表 | 最终字段 | 主键 / 唯一约束 | 关键索引 / 外键 |
|---|---|---|---|
| `news_items` | `news_id VARCHAR(128) ascii_bin`、`origin_source_id VARCHAR(128) ascii_bin`、`created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6)` | PK news_id | (origin_source_id,created_at) |
| `news_identifiers` | `source_id VARCHAR(128) ascii_bin`、`identifier_type VARCHAR(32) ascii_bin`、`identifier_hash CHAR(64) ascii_bin`、`identifier_value MEDIUMTEXT`、`identifier_status VARCHAR(16) ascii_bin`、`news_id VARCHAR(128) ascii_bin`、`created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6)` | 复合 PK (source_id,identifier_type,identifier_hash) | news_id；FK news_id → news_items |
| `news_observations` | `observation_id VARCHAR(128) ascii_bin`、`idempotency_key VARCHAR(191) ascii_bin`、`source_id VARCHAR(128) ascii_bin`、`provider_code VARCHAR(64) ascii_bin`、`query_dimension VARCHAR(128) DEFAULT ''`、`endpoint_id VARCHAR(128) DEFAULT ''`、`source_policy_version_id VARCHAR(128) DEFAULT ''`、`endpoint_policy_version_id VARCHAR(128) DEFAULT ''`、`provider_output_policy_ref VARCHAR(128) DEFAULT ''`、`provider_output_policy_version_id VARCHAR(128) DEFAULT ''`、`policy_evaluated_at DATETIME(6) NULL`、`candidate_json JSON`、`observed_at DATETIME(6)`、`retention_until DATETIME(6) NULL`、`payload_purged_at DATETIME(6) NULL`、`identity_status VARCHAR(16) ascii_bin`、`news_id VARCHAR(128) ascii_bin NULL`、`reason_code VARCHAR(128) DEFAULT ''` | PK observation_id；UK idempotency_key；UK (observation_id,news_id) | (source_id,observed_at)、(news_id,observed_at)；FK news_id → news_items |
| `news_item_versions` | `version_id VARCHAR(128) ascii_bin`、`news_id VARCHAR(128) ascii_bin`、`version_no BIGINT UNSIGNED`、`fingerprint CHAR(64) ascii_bin`、`title TEXT`、`publisher_name VARCHAR(512) DEFAULT ''`、`publisher_domain VARCHAR(253) ascii_bin DEFAULT ''`、`author VARCHAR(512) DEFAULT ''`、`published_at DATETIME(6) NULL`、`content_json JSON`、`observed_at DATETIME(6)`、`observation_id VARCHAR(128) ascii_bin` | PK version_id；UK (news_id,version_no)；UK (version_id,news_id) | (news_id,fingerprint)、(news_id,observed_at,version_no)；FK news_id → items；FK observation_id → observations |
| `news_relation_assertions` | `relation_id VARCHAR(128) ascii_bin`、`news_id VARCHAR(128) ascii_bin`、`subject_type VARCHAR(16) ascii_bin`、`subject_json JSON`、`relation_kind VARCHAR(32) ascii_bin`、`method VARCHAR(32) ascii_bin`、`status VARCHAR(16) ascii_bin`、`supersedes_relation_id VARCHAR(128) ascii_bin NULL`、`valid_from/valid_to DATETIME(6) NULL`、`observed_at DATETIME(6)` | PK relation_id；UK supersedes_relation_id | (news_id,status,observed_at)；FK news_id → items；自引用 FK supersedes_relation_id |
| `news_relation_evidence` | `evidence_id VARCHAR(128) ascii_bin`、`relation_id VARCHAR(128) ascii_bin`、`evidence_type VARCHAR(32) ascii_bin`、`source_version_id/observation_id VARCHAR(128) ascii_bin NULL`、`rule_version/model_version VARCHAR(128) DEFAULT ''`、`field_path VARCHAR(255) DEFAULT ''`、`evidence_hash CHAR(64) DEFAULT ''` | PK evidence_id | relation_id；FK relation_id → assertions；source_version_id → item_versions；observation_id → observations |
| `news_item_relations` | `item_relation_id VARCHAR(128) ascii_bin`、`from_news_id/from_source_id/to_news_id/to_source_id`、`relation_kind VARCHAR(32) ascii_bin`、`status VARCHAR(16) ascii_bin`、`supersedes_relation_id/evidence_observation_id/from_version_id/to_version_id VARCHAR(128) ascii_bin NULL`、`asserted_at/observed_at DATETIME(6)` | PK item_relation_id；UK supersedes_relation_id（替代原普通 supersedes 索引） | (from_news_id,status,observed_at)、(to_news_id,status,observed_at)；FK from/to items、self supersedes、observation、from/to versions |
| `news_authorization_snapshots` | `authorization_ref/subject_ref/resource_ref VARCHAR(256) utf8mb4_bin`、`decision VARCHAR(16) ascii_bin`、`purpose_code VARCHAR(32) ascii_bin`、`content_level VARCHAR(16) ascii_bin`、`decided_at DATETIME(6)`、`expires_at DATETIME(6) NULL`、`decision_fingerprint CHAR(64) ascii_bin`、`payload_json JSON`、`payload_hash CHAR(64) ascii_bin` | PK authorization_ref | (subject_ref,decided_at)、(resource_ref,decided_at)、expires_at |
| `news_evidence_snapshots` | `snapshot_ref/caller_ref VARCHAR(256) utf8mb4_bin`、`filters_fingerprint CHAR(64) ascii_bin`、`page_size SMALLINT UNSIGNED`、`system_as_of DATETIME(6)`、`sort_version VARCHAR(64) ascii_bin`、`created_at/expires_at DATETIME(6)`、`item_count INT UNSIGNED`、`payload_json JSON`、`payload_hash CHAR(64) ascii_bin` | PK snapshot_ref | (caller_ref,expires_at)、expires_at |
| `news_identifier_status_history` | `history_id BIGINT UNSIGNED AUTO_INCREMENT`、`source_id VARCHAR(128) ascii_bin`、`identifier_type VARCHAR(32) ascii_bin`、`identifier_hash CHAR(64) ascii_bin`、`identifier_status VARCHAR(16) ascii_bin`、`observed_at DATETIME(6)` | PK history_id | (source_id,identifier_type,identifier_hash,observed_at,history_id)；复合 FK → news_identifiers |
| `news_policy_versions` | `policy_type VARCHAR(32) ascii_bin`、`policy_ref VARCHAR(256) utf8mb4_bin`、`policy_version VARCHAR(128) utf8mb4_bin`、`payload_json JSON`、`payload_hash CHAR(64) ascii_bin`、`created_at DATETIME(6)` | 复合 PK (policy_type,policy_ref,policy_version) | (policy_type,policy_ref,created_at) |
| `news_current_policy_versions` | `policy_type VARCHAR(32) ascii_bin`、`policy_ref VARCHAR(256) utf8mb4_bin`、`policy_version VARCHAR(128) utf8mb4_bin`、`updated_at DATETIME(6)` | 复合 PK (policy_type,policy_ref) | (policy_type,policy_ref,policy_version)；复合 FK → news_policy_versions |
| `news_version_availability_evidence` | `evidence_id/news_id/version_id/observation_id`、`field_path VARCHAR(255)`、`raw_value MEDIUMTEXT NULL`、`source_timezone_or_offset VARCHAR(64) NULL`、`source_precision VARCHAR(16)`、`parse_status VARCHAR(32)`、`parse_reason_code VARCHAR(128) DEFAULT ''`、`candidate_available_at_utc DATETIME(0) NULL`、`parser_version VARCHAR(128)`、`evidence_ref VARCHAR(512) NULL`、`evidence_hash CHAR(64) NULL`、`observed_at/recorded_at/retention_until DATETIME(6)`、`payload_purged_at DATETIME(6) NULL` | PK evidence_id；UK (version_id,evidence_id) | (version_id,observed_at,recorded_at)、observation_id；复合 FK version_id+news_id → item_versions；observation_id+news_id → observations；news_id → items |
| `news_version_availability_decisions` | `decision_id VARCHAR(128) ascii_bin`、`version_id VARCHAR(128) ascii_bin`、`decision_seq BIGINT UNSIGNED`、`status VARCHAR(24) ascii_bin`、`selected_evidence_id VARCHAR(128) ascii_bin NULL`、`considered_evidence_ids JSON`、`reason_code/review_rule_version VARCHAR(128) ascii_bin`、`supersedes_decision_id VARCHAR(128) ascii_bin NULL`、`recorded_at DATETIME(6)` | PK decision_id；UK (version_id,decision_seq)、(version_id,decision_id)、supersedes_decision_id | (version_id,recorded_at,decision_seq)；FK version_id → item_versions；复合 FK (version_id,selected_evidence_id) → availability_evidence；复合自引用 FK (version_id,supersedes_decision_id) |

### Migration 历史变化与空库基线处理

1. V1 创建基础新闻实体、标识、观察记录、内容版本、关系断言/证据/关系、授权快照和证据快照。
2. V4 增加 `news_identifier_status_history`，并执行从 `news_identifiers` 的历史状态回填。新空库基线只建表，不执行回填 INSERT；后续新标识状态由 Repository 同事务写入 history。
3. V5 在 observations 增加 Provider Output Policy Version 字段。
4. V6 在 observations 增加 endpoint_id、provider_output_policy_ref，并为 relation assertions / item relations 增加 supersedes 唯一约束。
5. V7 增加 observations 的 retention_until、payload_purged_at 和 retention 索引。数据保留/清除是业务行为，不应从 Schema 初始化自动执行。
6. V8 增加 news_policy_versions 与 news_current_policy_versions，后者通过复合 FK 锚定不可变策略版本。
7. V9 增加 observations 与 item_versions 的 (id,news_id) 复合唯一键，为跨实体一致性外键提供引用目标。
8. V10 增加 Availability Evidence / Decision 两张表，含 PIT 索引、复合外键和决策 supersedes 唯一约束。

### News SQL 对照与结构注意事项

- `mysql_repository.go` 实际读写 observations 全部最终策略绑定与 retention 列；读写 identifiers/status history；读写 item versions；写 availability evidence/decision；并按 retention_until/payload_purged_at 进行有界清理。新基线不能漏掉 V5–V7 列。
- `identity_history.go` 使用复合标识键和 history_id 排序，依赖 status history 的复合 FK 和组合索引。
- `authorization_policy_resolver.go` / `source_collection_policy_resolver.go` 通过 current pointer 读取不可变 policy version。
- `news_item_relation_query.go` 使用递归 CTE 遍历 supersedes 链，两个关系表的 supersedes 唯一约束必须保留。
- `evidence_query.go` 以 observed_at / version_no / version_id 做 PIT 读取，依赖版本与观察记录的双向关联及对应索引。
- `evidence_snapshot.go` 依赖 caller_ref + expires_at 和 expires_at 索引执行授权读取、过期清理。
- `evidence_authorization_read.go` 使用 item_versions 与 observations 的 news_id 关联，并按 system_as_of 双重约束。
- `authorization_snapshot.go` 依赖 authorization_ref 主键和 payload hash 验证。
- V4 回填历史状态的 INSERT 不进入新基线；新基线也不执行 retention 清理。V1–V10 里已有的最终列、索引、唯一约束、复合外键必须合并为一个完整 V1。
- 新基线 DDL 应按外键依赖排序：news_items → identifiers/observations → item_versions → relation evidence/relations；policy_versions → current_policy_versions；item_versions + observations → availability_evidence → availability_decisions。每张表使用明确 CREATE TABLE，不能用 IF NOT EXISTS 掩盖旧结构。
 
News 字段级审计状态：**第一轮通过**。下一步仍需对照迁移文件完整 DDL 和全部 SQL 查询逐个核对列的大小写/字符集/NULL 语义；当前确认的版本演进与 Repository 读写字段已覆盖全部 14 张表。

## 8. Researchflow、Quant、Notification 字段级审计（第一轮完成）

审计来源为三个模块的 Migration DDL 与主要 Repository SQL。此处特别区分 **Schema** 和 **必须存在的初始运行记录**：初始记录不能遗漏，也不能错误地当作业务表结构。

### 8.1 Researchflow（9 张）

Migration：§internal/researchflow/storage/migrations.go§，V1–V6。

| 表 | 最终字段 | 主键 / 唯一约束 / 索引 |
|---|---|---|
| §researchflow_runs§ | run_ref、schema_version、status、version、system_as_of、deadline_at、template_ref/version/fingerprint、input_snapshot_refs、resolved_template_snapshot、parameters_snapshot、originating_request_id、correlation_id、result_summary_ref、termination_reason、created_at/started_at/finished_at | PK run_ref；(status,deadline_at)、correlation_id |
| §researchflow_stages§ | stage_ref、run_ref、stage_code、status、version、required、dependency_stage_refs、retry_policy_ref、attempt_no、retry_at、output_ref、quality_summary_ref、diagnostic_refs、quality_status、reason_codes、risk_decision、created_at/started_at/finished_at | PK stage_ref；UK (run_ref,stage_code)、UK (stage_ref,run_ref)；(run_ref,status)；FK run_ref → runs |
| §researchflow_attempts§ | attempt_ref、run_ref、stage_ref、attempt_no、status、lease_owner、lease_expires_at、fencing_token、started_at/finished_at、fault_ref、output_ref | PK attempt_ref；UK (stage_ref,attempt_no)、UK (stage_ref,fencing_token)；(run_ref,status)、(status,lease_expires_at)；FK run_ref → runs、(stage_ref,run_ref) → stages |
| §researchflow_run_idempotency§ | caller_ref、operation、idempotency_key、request_fingerprint、run_ref、created_at | PK (caller_ref,operation,idempotency_key)；V5 UK run_ref（替代原普通 run_ref 索引）；FK → runs |
| §researchflow_result_summaries§ | result_summary_ref、run_ref、schema_version、payload、created_at | PK result_summary_ref；UK run_ref；FK → runs |
| §researchflow_outbox§ | outbox_ref、run_ref、event_type、schema_version、idempotency_key、correlation_id、payload_fingerprint、payload、created_at、published_at、attempt_count、last_error_code、lease_owner、lease_expires_at、lease_generation、next_attempt_at | PK outbox_ref；UK (run_ref,event_type)、UK idempotency_key；pending 与 claim 索引；FK → runs |
| §researchflow_workflow_templates§ | template_ref、version、fingerprint、definition、created_at | PK (template_ref,version)；fingerprint 索引 |
| §researchflow_capacity_guard§ | guard_id TINYINT UNSIGNED | PK guard_id |
| §researchflow_stage_output_resources§ | resource_ref、run_ref、stage_ref、attempt_ref、resource_kind、stage_code、template_version、registry_version、registry_fingerprint、output_type、schema_version、schema_ref、content、created_at | PK resource_ref；UK (attempt_ref,resource_kind)；(run_ref,stage_ref)；resource_kind CHECK；FK run_ref → runs、(stage_ref,run_ref) → stages、attempt_ref → attempts |

关键迁移合并：
- V2 Outbox 增加 lease_owner、lease_expires_at、lease_generation、next_attempt_at 和 claim 复合索引。
- V3 增加不可变 workflow template 表。
- V4 增加 capacity guard 表，并插入 guard_id=1。
- V5 为 run_idempotency.run_ref 增加唯一约束，确保一个 Run 只能对应一条 CreateRun 身份记录。
- V6 增加 Stage Output Resource 表、attempt/resource_kind 唯一约束、复合外键和 CHECK 约束。

Repository 对照：
- §mysql_run_repository.go§ 实际读写 runs、stages、attempts、run_idempotency、result_summaries 的上述列。
- §mysql_outbox.go§ 读写 Outbox 租约、fencing_token、重试时间和错误码。
- §mysql_capacity.go§ 在事务中执行 §SELECT guard_id ... WHERE guard_id=1 FOR UPDATE§，因此 guard_id=1 是运行必需的初始记录，不能只建空表。
- §mysql_template_registry.go§ 读写 template_ref/version/fingerprint/definition。
- §mysql_stage_output_resource.go§ 读写 Stage Output Resource 全部列。

### 8.2 Quant（5 张）

Migration：§internal/quant/storage/migrations.go§，V1–V5。

| 表 | 最终字段 | 主键 / 唯一约束 / 索引 |
|---|---|---|
| §quant_results§ | result_id、manifest_fingerprint、result_fingerprint、manifest_json、payload_json、created_at | PK result_id |
| §quant_access_audit§ | audit_id、subject_ref、run_ref、resource_ref、purpose_code、content_level、decision、reason_code、authorization_ref、authorization_snapshot_json、policy_snapshot_ref、policy_snapshot_json、decision_fingerprint、occurred_at | PK audit_id；V3 增加 run_ref 索引 |
| §quant_result_access_policy_versions§ | policy_version、enabled、snapshot_ref、policy_snapshot_json、created_at | PK policy_version；UK snapshot_ref；enabled CHECK (0,1) |
| §quant_result_access_policy_head§ | policy_type、policy_ref、current_version、updated_at | PK (policy_type,policy_ref) |
| §quant_policy_admin_audit§ | audit_id、actor_ref、policy_type、policy_ref、action、previous_version、new_version、occurred_at | PK audit_id；(policy_type,policy_ref,audit_id)；action CHECK ('publish','revoke') |

关键迁移合并：
- V3 在 access audit 增加 run_ref 和对应索引。
- V4 增加 content_level、authorization_snapshot_json、policy_snapshot_ref、policy_snapshot_json、decision_fingerprint；其中 content_level 默认 full。
- V5 增加策略版本表、Head 表、Owner 管理审计表及 CHECK 约束。

Repository 对照：
- §repository.go§ 写入/读取 Quant Result 的 fingerprints 和 JSON payload。
- §audit.go§ 写入授权审计全部最终字段。
- §policy_owner.go§ 锁定并更新 Head，写入不可变策略版本和 Owner 管理审计；current_version=0 表示尚未发布策略，读取时返回 disabled 状态。

V5 的初始 Head 记录必须保留：§(policy_type='quant_result_access', policy_ref='quant-result-access', current_version=0, updated_at='1970-01-01T00:00:00.000000000Z')§。它是系统初始状态，不是用户策略，也不应被误删或改成默认放行。

### 8.3 Notification（10 张）

Migration：§internal/notification/storage/migrations.go§，V1–V6。

| 表 | 最终字段 | 主键 / 唯一约束 / 索引 |
|---|---|---|
| §notification_subscriptions§ | subscription_id、version、revision_id、owner_ref、status、event_type、filter_schema_version、filters、recipient_scope、recipient_ref、channel_ref、template_id、created_at、updated_at、effective_at | PK (subscription_id,version)；(event_type,status,subscription_id,version)、(subscription_id,revision_id,version) |
| §notification_delivery_plans§ | delivery_id、source_module、event_id、subscription_id/version、channel_ref/config_version、template_id/version、recipient_ref、payload_fingerprint、authorization_policy_versions、authorization_refs、plan_fingerprint、delivery_idempotency_key、plan_json、created_at | PK delivery_id；UK 完整计划身份组合；(source_module,event_id)、plan_fingerprint |
| §notification_deliveries§ | delivery_id、source_module、event_id、subscription_id/version、channel_ref/config_version、template_id/version、recipient_ref、payload_fingerprint、plan_fingerprint、delivery_idempotency_key、status、version、attempt_count、max_attempts、lease_worker_id、lease_expires_at、fencing_token、next_attempt_at、deadline_at、reason_code、last_error_code、provider_message_ref、created_at、updated_at | PK delivery_id；UK 计划身份组合；UK (channel_ref,recipient_ref,delivery_idempotency_key)；due/lease/subscription 索引；FK → delivery_plans |
| §notification_delivery_attempts§ | attempt_id、delivery_id、attempt_no、worker_id、fencing_token、lease_expires_at、started_at、finished_at、outcome、provider_message_ref、error_code、retryable、retry_after_at、duration_ms、evidence_ref | PK attempt_id；UK (delivery_id,attempt_no)、UK (delivery_id,fencing_token)；(delivery_id,started_at)；FK → deliveries |
| §notification_delivery_state_events§ | delivery_id、version、from_status、to_status、changed_at、reason_code、evidence_ref、actor_ref、request_id、actor_reason | PK (delivery_id,version)；(delivery_id,changed_at)；FK → deliveries |
| §notification_inbox§ | source_module、event_id、idempotency_key、event_type、schema_version、subject_ref、occurred_at、published_at、correlation_id、payload_fingerprint、payload、received_at | PK (source_module,event_id)；UK (source_module,idempotency_key)；received_at |
| §notification_template_versions§ | template_id、version、event_type、schema_version、format、subject_template、body_template、variable_bindings、digest、created_at | PK (template_id,version)；(event_type,schema_version,template_id,version) |
| §notification_channel_config_versions§ | channel_ref、config_version、snapshot_json、digest、created_at | PK (channel_ref,config_version)；V5 删除与主键重复的 idx_notification_channel_config_kind，不应在新基线中重建 |
| §notification_subscription_revision§ | singleton_id、current_revision | PK singleton_id |
| §notification_fanout_work_items§ | work_item_id、source_module、event_id、event_type、payload_fingerprint、snapshot_revision、cursor_subscription_id、status、attempt_count、lease_worker_id、lease_expires_at、fencing_token、next_attempt_at、last_error_code、created_at、updated_at | PK work_item_id；UK (source_module,event_id)；due/lease 索引；复合 FK (source_module,event_id) → inbox |

关键迁移合并：
- V5 删除 §notification_channel_config_versions§ 上与主键重复的 §idx_notification_channel_config_kind§；新基线必须省略它。
- V6 给 subscriptions 增加 revision_id 及 (subscription_id,revision_id,version) 索引。
- V6 增加 subscription_revision 单例表，并插入 (singleton_id=1,current_revision=0)。
- V6 增加 fanout_work_items，使用 inbox 复合外键，保证工作项引用已接收事件。

Repository 对照：
- §mysql_subscription_repository.go§ 写入 subscription revision，并以 singleton_id=1 锁定/递增/读取 current_revision；按 revision_id 读取订阅快照。
- §mysql_fanout_work_item_repository.go§ 读写 fanout 工作项、租约/fencing 状态，并联接 inbox 事件。
- §mysql_delivery_repository.go§ 读写 deliveries、plans、state_events 的执行状态、租约、重试和审计字段。
- §mysql_delivery_attempt_repository.go§ 读写 attempt 的 fencing、lease、outcome、retryable、evidence_ref 等字段。
- §mysql_delivery_plan_repository.go§ 写入/读取完整计划身份、授权引用和 fingerprint。
- §mysql_inbox_repository.go§ 写入事件信封及双幂等身份。
- §mysql_template_repository.go§ / §mysql_channel_config_repository.go§ 读写不可变模板和渠道配置版本。

### 三个模块的初始记录与 DDL 的边界

1. Researchflow：必须在 Schema 创建后插入 §researchflow_capacity_guard(guard_id=1)§，否则 Capacity Guard 事务锁查询无法工作。
2. Quant：必须初始化唯一的 Result Access Policy Head，初始 current_version=0，表示尚未发布策略；不能将它解释为允许访问。
3. Notification：必须初始化 §notification_subscription_revision(singleton_id=1,current_revision=0)§，否则订阅版本追加与 fanout revision 读取无法正常工作。
4. 上述记录应由明确的模块初始化定义负责，随该模块基线在 Migration 内执行一次，或由统一的模块 seed 接口执行一次；如果放在 Migration 中，需保证 checksum 覆盖该语句。不得放入 Repository 构造函数中隐式补数据。
5. Notification V5 删除的冗余索引不得重新出现。Researchflow V2/V5 和 Quant V3/V4 的后续字段/索引必须并入各自完整 V1。

本轮三个模块完成第一轮字段级盘点；尚需进一步审计每个模块全部 Repository 文件与查询参数/扫描列的逐一对应，并审查初始 seed 语句是否具备幂等/失败恢复语义。

## 9. Research、Analysis、Owner Credential、Server Security Audit 字段级审计（第一轮完成）

### 9.1 Research（6 张）

来源：§internal/research/storage/migrations.go§，当前单一 V1；Repository 分布于 claim/context/event/evidence/outbox/repository 文件。

| 表 | 字段 | 主键 / 索引 |
|---|---|---|
| §research_aggregate_heads§ | aggregate_type VARCHAR(32)、aggregate_id VARCHAR(255)、latest_version BIGINT UNSIGNED、created_at/updated_at DATETIME(6) | PK (aggregate_type,aggregate_id) |
| §research_context_versions§ | context_id VARCHAR(255)、context_version BIGINT UNSIGNED、system_as_of CHAR(30) ascii_bin、payload_sha256 CHAR(64)、payload LONGTEXT、created_at DATETIME(6) | PK (context_id,context_version)；(context_id,system_as_of) |
| §research_evidence_records§ | evidence_id VARCHAR(255)、source_owner VARCHAR(16)、source_ref VARCHAR(512)、source_version VARCHAR(191) NULL、available_at CHAR(30) NULL、system_observed_at/system_as_of CHAR(30) ascii_bin、quality_status/content_level VARCHAR(16)、authorization_ref VARCHAR(255)、content_fingerprint/payload_sha256 CHAR(64)、payload LONGTEXT、created_at DATETIME(6) | PK evidence_id；(source_owner,source_ref(191),source_version)；(system_as_of,available_at) |
| §research_event_versions§ | event_id VARCHAR(255)、event_version BIGINT UNSIGNED、status VARCHAR(24)、system_as_of CHAR(30) ascii_bin、payload_sha256 CHAR(64)、payload LONGTEXT、created_at DATETIME(6) | PK (event_id,event_version)；(status,system_as_of) |
| §research_claim_versions§ | claim_id VARCHAR(255)、claim_version BIGINT UNSIGNED、epistemic_type/status VARCHAR(24)、system_as_of CHAR(30) ascii_bin、payload_sha256 CHAR(64)、payload LONGTEXT、created_at DATETIME(6) | PK (claim_id,claim_version)；(status,system_as_of) |
| §research_notification_outbox§ | event_id VARCHAR(255)、event_type VARCHAR(128)、schema_version/source_module VARCHAR(32)、occurred_at/published_at CHAR(30) ascii_bin、subject_ref/idempotency_key VARCHAR(512)、correlation_id VARCHAR(128)、payload_fingerprint CHAR(64) ascii_bin、payload LONGTEXT、delivery_status VARCHAR(16) DEFAULT pending、available_at CHAR(30) ascii_bin、lease_owner VARCHAR(128) NULL、lease_token CHAR(32) ascii_bin NULL、lease_until CHAR(30) ascii_bin NULL、attempt_count INT UNSIGNED DEFAULT 0、last_error_code VARCHAR(128) NULL、delivered_at CHAR(30) ascii_bin NULL、created_at/updated_at DATETIME(6) | PK event_id；UK (source_module,idempotency_key)；created_at；(delivery_status,available_at,created_at) |

本模块目前只有 V1，没有后续 Migration 合并。Outbox 是模块内持久化事件交付状态，lease、attempt_count、delivery_status 等均属于当前完整 Schema，不应在基线重建时遗漏。未发现需要在 Schema Migration 阶段预置的业务记录。

### 9.2 Analysis（4 张，新增 Registry namespace）

来源：§internal/adapters/storage/mysqlredis/store.go§ 的旧 §Migrate()§。

| 表 | 字段 | 主键 / 索引 / 外键 |
|---|---|---|
| §analysis_runs§ | run_id VARCHAR(64)、status VARCHAR(32)、task_type VARCHAR(64)、as_of/started_at DATETIME(6)、finished_at DATETIME(6) NULL、request_json LONGTEXT、result_json LONGTEXT NULL、created_at/updated_at DATETIME(6) | PK run_id；as_of；status |
| §analysis_stage_events§ | id BIGINT UNSIGNED AUTO_INCREMENT、run_id VARCHAR(64)、stage VARCHAR(100)、status VARCHAR(32)、event_at DATETIME(6)、duration_ms BIGINT DEFAULT 0、event_json LONGTEXT、created_at DATETIME(6) | PK id；run_id；(run_id,stage)；FK run_id → analysis_runs，ON DELETE CASCADE |
| §analysis_snapshots§ | snapshot_id VARCHAR(96)、run_id VARCHAR(64)、snapshot_type VARCHAR(32)、as_of DATETIME(6)、payload_json LONGTEXT、created_at DATETIME(6) | PK snapshot_id；run_id；(snapshot_type,as_of)；FK run_id → analysis_runs，ON DELETE CASCADE |
| §analysis_tasks§ | task_id VARCHAR(64)、run_id VARCHAR(64)、status VARCHAR(24)、task_type VARCHAR(64)、request_json LONGTEXT、idempotency_key VARCHAR(128) NULL、current_stage VARCHAR(100) NULL、failure_code VARCHAR(100) NULL、error_message VARCHAR(300) NULL、created_at/updated_at DATETIME(6)、started_at/finished_at DATETIME(6) NULL | PK task_id；UK run_id；UK idempotency_key（可空）；(status,created_at) |

Analysis 需要新增 namespace §analysis§，定义完整 V1。现有 Store 构造路径会调用 §initializeSchema()§，而 §Migrate()§ 独立创建上述表；统一 Registry 接入后必须删除这条隐式 Schema 路径。Analysis Store 仅在 Migration 成功后创建。

### 9.3 Platform Owner Credential（1 张）

来源：§internal/platform/security/credential_store.go§，namespace §platform.security§ V1。

§platform_owner_credentials§：
- 字段：§owner_id TINYINT UNSIGNED§、§password_hash VARCHAR(512) CHARACTER SET ascii COLLATE ascii_bin§、§credential_version BIGINT UNSIGNED§、§created_at/updated_at DATETIME(6)§。
- PK owner_id；CHECK owner_id=1。
- Repository 实际读写 password_hash 和 credential_version；初始化是单条 Owner Credential，密码采用 Argon2id PHC 编码。
- 新基线应保留 CHECK 和字段字符集。不得把 password_hash 写入日志、诊断、普通 API 输出。

### 9.4 Server Security Audit（1 张）

来源：§internal/server/security/audit_writer.go§，namespace §server.security_audit§ V1。

§server_security_audit_events§：
- 字段：audit_id BIGINT UNSIGNED AUTO_INCREMENT、event_type VARCHAR(64) ascii_bin、subject VARCHAR(16) ascii_bin、result VARCHAR(16) ascii_bin、occurred_at DATETIME(6)、request_id VARCHAR(128) ascii_bin、source VARCHAR(16) ascii_bin、reason_code VARCHAR(128) ascii_bin。
- PK audit_id。
- 索引：(occurred_at,audit_id)、(subject,occurred_at,audit_id)、request_id。
- Writer 只写入经过 Platform Security Audit Event.Validate() 校验的白名单字段。
- 保留/归档策略尚未定义；本次不增加 TTL、自动删除或清理 Job。

## 10. 全模块第一轮盘点汇总

- Data / MarketData：17 张，V1–V9，已核对最终列、索引、关键外键、Repository SQL 依赖及 Schema 校验。
- News：14 张，V1–V10，已核对最终字段演进、关键复合约束和 Repository SQL 依赖。
- Researchflow：9 张，V1–V6，已核对最终字段、索引、外键及 Capacity Guard 初始记录。
- Quant：5 张，V1–V5，已核对授权审计字段、策略 Head 和初始禁用状态。
- Notification：10 张，V1–V6，已核对最终订阅 revision、fanout 工作项、冗余索引删除和单例初始记录。
- Research：6 张，V1，已核对 Migration 最终定义。
- Analysis：4 张，当前独立建表，已核对旧 DDL，待纳入新 namespace。
- Platform Owner Credential：1 张，V1。
- Server Security Audit：1 张，V1。

当前共 67 张业务表。模块 Schema 的第一轮最终结构盘点已完成；M2/M3 实施状态见第 12 节。字段语义/Provider 映射正确性不属于本次数据库 Schema 基线重建范围。

## 11. Platform Migration Runner 与 Root Bootstrap 收口审查

审计来源：§internal/platform/storage/storage.go§、§internal/server/migration/registry.go§、§internal/server/bootstrap/bootstrap.go§、§internal/server/bootstrap/owner_session_init.go§、§internal/server/resources/storage.go§。

### 11.1 Migration History 当前行为

当前 §platform_schema_migrations§ 字段：
- namespace VARCHAR(64) ascii_bin
- version BIGINT UNSIGNED
- description VARCHAR(255)
- checksum CHAR(64) ascii_bin
- status VARCHAR(16)
- failure_statement INT UNSIGNED
- applied_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6)
- started_at DATETIME(6) NULL
- finished_at DATETIME(6) NULL
- failure_code VARCHAR(64) DEFAULT ''

主键：(namespace,version)。

当前执行时每个 namespace 都会尝试 CREATE TABLE IF NOT EXISTS，然后执行三条 ADD COLUMN IF NOT EXISTS 补齐 started_at、finished_at、failure_code。后面三条是旧 History 表兼容逻辑，应删除。§applied_at§ 在当前 Runner 的写入/读取路径中没有实际用途，属于旧记录时间字段；新 History 基线不再保留该字段，统一使用 started_at/finished_at 表示迁移执行时间。

新 History 表应一次定义完整字段：namespace、version、description、checksum、status、failure_statement、started_at、finished_at、failure_code。保留 CREATE TABLE IF NOT EXISTS 用于首次建账本；这不是旧结构兼容。不要自动 ALTER 既有 History 表。

### 11.2 Runner 的执行和失败语义

1. §ApplyMigrations§ 为每个 namespace 获取独立 MariaDB advisory lock，并在专用物理连接上执行 SQL。
2. 新版本执行前写入 running 记录；语句失败写入 failed、failure_statement、failure_code、finished_at；成功后改为 applied。
3. 已存在 running/failed 状态时直接要求人工恢复，不会自动重跑。
4. 已登记版本 checksum 改变、存在未知版本或中间版本缺失时拒绝执行。
5. MariaDB DDL 可能隐式提交，不能依赖事务回滚；某个 namespace 中途失败时，需要人工检查该 namespace 已落地的对象。
6. Registry 按 namespace 字典序逐个执行，遇首个失败停止；已成功的前序 namespace 保留。不存在跨 namespace 原子事务。
7. §ValidateAppliedMigrations§ 是只读校验，不创建 History 表；Registry 的 §ValidateApplied§ 还会核对 namespace 和 version 集合必须精确匹配。因此当前 MIGRATION_ENABLED=false 且数据库没有 History 表时，报 “migration history could not be read” 是当前代码行为，不代表业务表一定不存在。
8. Registry 当前只注册 8 个 namespace；新增 Analysis 后必须是 9 个。新部署第一次运行时，ApplyMigrations 自动创建 History 表并执行所有模块 V1。

### 11.3 必须移除的旧入口

- Data：§internal/marketdata/repository/mysql.go§ 的 §Store.Migrate()§ 直接调用 Platform Runner。统一 Registry 接管后删除此独立入口，并调整只为其服务的测试 setup。
- Analysis：§internal/adapters/storage/mysqlredis/store.go§ 的 §initializeSchema()§ / §Migrate()§ 自动建表入口必须删除；Repository 构造函数不得隐式改 Schema。
- Root：§bootstrap.Run()§ 现在先调用 §OpenLegacyAnalysisResources()§，其中先打开 SQL、Redis、HTTP Client 并创建 Analysis Store，之后才注册/执行 Migration。必须拆成 SQL-only migration connection 和完整应用资源构造两步。
- Owner Init：§InitializeOwner()§ 现在也调用 §OpenLegacyAnalysisResources()§，并根据 MIGRATION_ENABLED 分支执行或只读校验。必须复用同一个 Database Bootstrap，先迁移再创建 Owner Credential。
- §MIGRATION_ENABLED§ 和 §DB_AUTO_MIGRATE§ 从配置结构、加载器、错误信息、环境模板、部署脚本检查、部署说明中全部删除；§MIGRATION_TIMEOUT§ 保留。
- Data 的 §ValidateCompatibilitySchema§ 改为最终物理 Schema 校验，Migration 完成后、Repository/Worker/HTTP 装配前执行。

### 11.4 统一启动目标顺序

1. 加载 Server/Data 配置。
2. 只打开 MariaDB SQL 连接（不打开 Redis，不构造任何 Repository）。
3. 注册所有模块 Migration（Data、News、Owner Credential、Research、Researchflow、Quant、Notification、Server Security Audit、Analysis）。
4. 执行 Registry.RunWithReport，应用所有尚未应用的 V1；已 applied 的 V1 校验 checksum 并跳过。
5. 对 Migration History、namespace/version/checksum 做精确校验。
6. 执行 Data 最终物理 Schema 校验。
7. 关闭 migration-only SQL 连接，或将其安全转交给完整 Resources；不能重复创建不同配置的连接池。
8. 创建 Redis、共享 HTTP Client、Analysis Store 和其他模块 Repository。
9. 装配 Application、Worker、Diagnostics、HTTP，最后启动服务。

Owner Init 使用相同步骤 1–6；如果 Owner Credential 初始化本身只依赖 SQL，则无需创建 Redis/HTTP/Analysis Store。需在实现前检查 Owner Session Composition 的实际依赖，避免为了初始化一个凭据而启动完整 Server 资源。

### 11.5 M1 完成结论

- 全部 67 张业务表的模块归属和当前版本范围已盘点。
- 多版本模块的最终字段/索引/约束和关键 Repository SQL 依赖已完成第一轮对照。
- 三条必要的初始运行记录已确认：Researchflow Capacity Guard、Quant 初始禁用策略 Head、Notification Subscription Revision。
- Migration History 的旧字段补齐逻辑和旧 applied_at 字段已定位。
- Root/Owner Init 资源创建顺序和 Analysis/Data 独立迁移入口已定位。

**M1 第一轮盘点完成。M2 已将所有模块收敛为单一 V1；M3 已开始统一 Migration History 与 Root Bootstrap。** 当前实现进度和剩余项见第 12 节。


## 12. 基线重建与统一启动实施进度

### 12.1 已完成的模块 V1 基线

| 模块 | 新 Migration | 基线语句数 | 处理内容 |
|---|---:|---:|---|
| Data / MarketData | V1 | 17 | 17 张表合并；纳入 source_version 与索引；删除历史回填/Ensure 操作；删除独立 Store.Migrate 入口 |
| News | V1 | 14 | 14 张表合并；不执行 Identifier Status History 历史回填；合并 V5–V10 字段/索引/复合外键；移除被唯一索引覆盖的冗余 supersedes 索引 |
| Researchflow | V1 | 10 | 9 张表 + Capacity Guard 初始行；合并 Outbox Lease、Run 幂等唯一约束、Stage Output Resource；移除被唯一索引覆盖的冗余普通索引 |
| Quant | V1 | 6 | 5 张表 + 初始禁用 Policy Head；合并授权审计字段和策略版本结构 |
| Notification | V1 | 11 | 10 张表 + Subscription Revision 初始行；合并 revision/fanout；不重建 V5 已删除的冗余索引 |
| Analysis | V1 | 4 | 新增 analysis namespace；将旧 Store 建表 DDL 纳入 Registry；删除 Store 隐式 initializeSchema/Migrate |
| Research | V1 | 保持 | 已有完整 V1，不改动 |
| Platform Owner Credential | V1 | 保持 | 已有 V1，改为严格 CREATE TABLE |
| Server Security Audit | V1 | 保持 | 已有 V1，不改动 |

基线 SQL 使用严格 CREATE TABLE，不用 IF NOT EXISTS 掩盖旧业务表结构。唯一保留 CREATE TABLE IF NOT EXISTS 的是共享 Migration History 表首次建账本入口；不对旧 History 表自动补列。

### 12.2 Platform Migration Runner 已调整

- 删除 MigrationOperation、EnsureColumn、EnsureNonUniqueIndex 和相应 checksum/执行逻辑；Migration 仅保留有序 Statements。
- History 表改为完整结构：namespace、version、description、checksum、status、failure_statement、started_at、finished_at、failure_code；删除 applied_at。
- 删除旧 History 的 ADD COLUMN IF NOT EXISTS 自动补列逻辑。
- status 不设默认值，必须由 Runner 显式写入 running/applied/failed，并增加状态 CHECK。
- 保留 namespace 级 advisory lock、checksum、失败记录、人工恢复和 fail-closed 语义；不承诺跨 namespace 原子事务。

### 12.3 Root Server 与 Owner Init 启动路径已调整

- 新增 `internal/server/bootstrap/database.go` 的共享 `bootstrapDatabase()`，由正常 Server 与 owner-init 共用。
- 新路径先打开 SQL-only MariaDB 连接，执行全部 9 个 namespace Migration，校验精确 History/namespace/version/checksum，并在创建 Redis 前执行 Data 最终物理 Schema 校验。
- Migration 成功后才打开 Redis、创建 HTTP Client、Analysis Store 和其他 Repository。
- `OpenLegacyAnalysisResources` 已替换为 `OpenSQL` + `OpenApplicationResources` 两阶段资源 API。
- 删除 `DB_AUTO_MIGRATE`、`MIGRATION_ENABLED` 配置项；保留 `MIGRATION_TIMEOUT`。
- `.env.example`、`scripts/deploy-go.sh`、`doc/开发部署说明.md` 已同步改为自动迁移和空数据库切换流程。
- PLAT-CTR-006 已升级到 2.0.0，项目级登记簿和 DEC-PLAT-002-056 已记录破坏性变更。

### 12.4 当前尚未完成

1. 对生成的全部 V1 DDL 做最终静态审查，重点检查外键引用列/索引顺序、列字符集、可空性、默认值与 Repository SQL 的一致性；不要把字段业务语义验证混入本任务。
2. 更新/补齐剩余模块 Migration 测试，确认所有单一 V1 的语句数量、必要 seed 和已删除冗余索引。
3. 全仓搜索确认 `DB_AUTO_MIGRATE`、`MIGRATION_ENABLED`、`MigrationOperation`、`OpenLegacyAnalysisResources`、`Store.Migrate()`、`initializeSchema()` 没有残留运行时代码或旧部署指引。
4. 检查 Root/Owner Init 资源所有权和失败清理路径，确保 SQL Migration 失败时 DB 关闭，Redis 不会提前打开。
5. 获取 CI/Actions 结果；CI 未通过前不宣称完成。真实数据库部署和 Provider 准确性验收留到用户准备空数据库后执行。

本阶段没有连接或修改用户真实数据库，也没有修改服务器 .env。
