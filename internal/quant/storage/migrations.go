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
	}
}
