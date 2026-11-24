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
