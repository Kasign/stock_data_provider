package storage

import platformstorage "example.com/stock-analysis-flow/internal/platform/storage"

const MigrationNamespace = "quant"

// Migrations returns Quant-owned schema changes. Server only registers and
// executes these definitions through the Platform migration runner.
func Migrations() []platformstorage.Migration {
	return []platformstorage.Migration{
		{
			Version:     1,
			Description: "create immutable Quant result storage",
			Statements: []string{`CREATE TABLE quant_results (
				result_id VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
				manifest_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
				result_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
				manifest_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
				payload_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
				created_at DATETIME(6) NOT NULL,
				PRIMARY KEY (result_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`},
		},
		{
			Version:     2,
			Description: "create append-only Quant access audit storage",
			Statements: []string{`CREATE TABLE quant_access_audit (
				audit_id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
				subject_ref TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
				resource_ref VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
				purpose_code VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
				decision VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
				reason_code VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
				authorization_ref TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL,
				occurred_at VARCHAR(35) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
				PRIMARY KEY (audit_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`},
		},
		{
			Version:     3,
			Description: "persist Researchflow Run scope on Quant access audit events",
			Statements: []string{`ALTER TABLE quant_access_audit
				ADD COLUMN run_ref VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER subject_ref,
				ADD KEY idx_quant_access_audit_run_id (run_ref, audit_id)`},
		},
		{
			Version:     4,
			Description: "补充 Quant 授权审计的完整决策证据",
			Statements: []string{`ALTER TABLE quant_access_audit
				ADD COLUMN content_level VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'full' AFTER purpose_code,
				ADD COLUMN authorization_snapshot_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL AFTER authorization_ref,
				ADD COLUMN policy_snapshot_ref VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL AFTER authorization_snapshot_json,
				ADD COLUMN policy_snapshot_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL AFTER policy_snapshot_ref,
				ADD COLUMN decision_fingerprint VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER policy_snapshot_json`},
		},

		{
			Version:     5,
			Description: "persist immutable Quant result-access policy versions and Owner administration audit",
			Statements: []string{
				`CREATE TABLE quant_result_access_policy_versions (
					policy_version BIGINT UNSIGNED NOT NULL,
					enabled TINYINT(1) NOT NULL,
					snapshot_ref VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
					policy_snapshot_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
					created_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					PRIMARY KEY (policy_version),
					UNIQUE KEY uq_quant_result_access_policy_snapshot (snapshot_ref),
					CONSTRAINT chk_quant_result_access_policy_enabled CHECK (enabled IN (0, 1))
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`,
				`CREATE TABLE quant_result_access_policy_head (
					policy_type VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					policy_ref VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					current_version BIGINT UNSIGNED NOT NULL,
					updated_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					PRIMARY KEY (policy_type, policy_ref)
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`,
				`INSERT INTO quant_result_access_policy_head
					(policy_type, policy_ref, current_version, updated_at)
					VALUES ('quant_result_access', 'quant-result-access', 0, '1970-01-01T00:00:00.000000000Z')`,
				`CREATE TABLE quant_policy_admin_audit (
					audit_id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
					actor_ref VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					policy_type VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					policy_ref VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					action VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					previous_version BIGINT UNSIGNED NOT NULL,
					new_version BIGINT UNSIGNED NOT NULL,
					occurred_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					PRIMARY KEY (audit_id),
					KEY idx_quant_policy_admin_audit_policy (policy_type, policy_ref, audit_id),
					CONSTRAINT chk_quant_policy_admin_action CHECK (action IN ('publish', 'revoke'))
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`,
			},
		},
	}
}
