package storage

import platformstorage "example.com/stock-analysis-flow/internal/platform/storage"

const MigrationNamespace = "research"

// Migrations 只描述初版正式结构。Research 尚无历史数据，因此直接创建
// 最终表结构，不保留逐步升级、历史回填或旧结构兼容迁移。
func Migrations() []platformstorage.Migration {
	return []platformstorage.Migration{{
		Version:     1,
		Description: "create final Research storage schema",
		Statements: []string{
			"CREATE TABLE research_aggregate_heads (aggregate_type VARCHAR(32) NOT NULL, aggregate_id VARCHAR(255) NOT NULL, latest_version BIGINT UNSIGNED NOT NULL, created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL, PRIMARY KEY (aggregate_type, aggregate_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin",
			"CREATE TABLE research_context_versions (context_id VARCHAR(255) NOT NULL, context_version BIGINT UNSIGNED NOT NULL, system_as_of CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, payload_sha256 CHAR(64) NOT NULL, payload LONGTEXT NOT NULL, created_at DATETIME(6) NOT NULL, PRIMARY KEY (context_id, context_version), KEY idx_research_context_asof (context_id, system_as_of)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin",
			"CREATE TABLE research_evidence_records (evidence_id VARCHAR(255) NOT NULL, source_owner VARCHAR(16) NOT NULL, source_ref VARCHAR(512) NOT NULL, source_version VARCHAR(191) NULL, available_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL, system_observed_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, system_as_of CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, quality_status VARCHAR(16) NOT NULL, content_level VARCHAR(16) NOT NULL, authorization_ref VARCHAR(255) NOT NULL, content_fingerprint CHAR(64) NOT NULL, payload_sha256 CHAR(64) NOT NULL, payload LONGTEXT NOT NULL, created_at DATETIME(6) NOT NULL, PRIMARY KEY (evidence_id), KEY idx_research_evidence_source (source_owner, source_ref(191), source_version), KEY idx_research_evidence_pit (system_as_of, available_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin",
			"CREATE TABLE research_event_versions (event_id VARCHAR(255) NOT NULL, event_version BIGINT UNSIGNED NOT NULL, status VARCHAR(24) NOT NULL, system_as_of CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, payload_sha256 CHAR(64) NOT NULL, payload LONGTEXT NOT NULL, created_at DATETIME(6) NOT NULL, PRIMARY KEY (event_id, event_version), KEY idx_research_event_status_asof (status, system_as_of)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin",
			"CREATE TABLE research_claim_versions (claim_id VARCHAR(255) NOT NULL, claim_version BIGINT UNSIGNED NOT NULL, epistemic_type VARCHAR(24) NOT NULL, status VARCHAR(24) NOT NULL, system_as_of CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, payload_sha256 CHAR(64) NOT NULL, payload LONGTEXT NOT NULL, created_at DATETIME(6) NOT NULL, PRIMARY KEY (claim_id, claim_version), KEY idx_research_claim_status_asof (status, system_as_of)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin",
			"CREATE TABLE research_notification_outbox (event_id VARCHAR(255) NOT NULL, event_type VARCHAR(128) NOT NULL, schema_version VARCHAR(32) NOT NULL, source_module VARCHAR(32) NOT NULL, occurred_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, published_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, subject_ref VARCHAR(512) NOT NULL, idempotency_key VARCHAR(512) NOT NULL, correlation_id VARCHAR(128) NOT NULL, payload_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, payload LONGTEXT NOT NULL, delivery_status VARCHAR(16) NOT NULL DEFAULT 'pending', available_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, lease_owner VARCHAR(128) NULL, lease_token CHAR(32) CHARACTER SET ascii COLLATE ascii_bin NULL, lease_until CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL, attempt_count INT UNSIGNED NOT NULL DEFAULT 0, last_error_code VARCHAR(128) NULL, delivered_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL, created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL, PRIMARY KEY (event_id), UNIQUE KEY uq_research_outbox_idempotency (source_module, idempotency_key), KEY idx_research_outbox_created (created_at), KEY idx_research_outbox_delivery (delivery_status, available_at, created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin",
		},
	}}
}
