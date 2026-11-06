package storage

import platformstorage "example.com/stock-analysis-flow/internal/platform/storage"

const MigrationNamespace = "research"

// Migrations returns Research-owned schema changes. The service composition
// root must register this list with Platform's migration runner; importing
// this package never executes DDL.
func Migrations() []platformstorage.Migration {
	return []platformstorage.Migration{
		{
			Version:     1,
			Description: "create append-only Research domain storage",
			Statements: []string{
				"CREATE TABLE research_aggregate_heads (\n" +
					"aggregate_type VARCHAR(32) NOT NULL,\n" +
					"aggregate_id VARCHAR(255) NOT NULL,\n" +
					"latest_version BIGINT UNSIGNED NOT NULL,\n" +
					"created_at DATETIME(6) NOT NULL,\n" +
					"updated_at DATETIME(6) NOT NULL,\n" +
					"PRIMARY KEY (aggregate_type, aggregate_id)\n" +
					") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin",
				"CREATE TABLE research_context_versions (\n" +
					"context_id VARCHAR(255) NOT NULL,\n" +
					"context_version BIGINT UNSIGNED NOT NULL,\n" +
					"system_as_of DATETIME(6) NOT NULL,\n" +
					"payload_sha256 CHAR(64) NOT NULL,\n" +
					"payload LONGTEXT NOT NULL,\n" +
					"created_at DATETIME(6) NOT NULL,\n" +
					"PRIMARY KEY (context_id, context_version),\n" +
					"KEY idx_research_context_asof (context_id, system_as_of)\n" +
					") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin",
				"CREATE TABLE research_evidence_records (\n" +
					"evidence_id VARCHAR(255) NOT NULL,\n" +
					"source_owner VARCHAR(16) NOT NULL,\n" +
					"source_ref VARCHAR(512) NOT NULL,\n" +
					"source_version VARCHAR(191) NULL,\n" +
					"available_at DATETIME(6) NULL,\n" +
					"system_observed_at DATETIME(6) NOT NULL,\n" +
					"system_as_of DATETIME(6) NOT NULL,\n" +
					"quality_status VARCHAR(16) NOT NULL,\n" +
					"content_level VARCHAR(16) NOT NULL,\n" +
					"authorization_ref VARCHAR(255) NOT NULL,\n" +
					"content_fingerprint CHAR(64) NOT NULL,\n" +
					"payload_sha256 CHAR(64) NOT NULL,\n" +
					"payload LONGTEXT NOT NULL,\n" +
					"created_at DATETIME(6) NOT NULL,\n" +
					"PRIMARY KEY (evidence_id),\n" +
					"KEY idx_research_evidence_source (source_owner, source_ref(191), source_version),\n" +
					"KEY idx_research_evidence_pit (system_as_of, available_at)\n" +
					") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin",
				"CREATE TABLE research_event_versions (\n" +
					"event_id VARCHAR(255) NOT NULL,\n" +
					"event_version INT UNSIGNED NOT NULL,\n" +
					"status VARCHAR(24) NOT NULL,\n" +
					"system_as_of DATETIME(6) NOT NULL,\n" +
					"payload_sha256 CHAR(64) NOT NULL,\n" +
					"payload LONGTEXT NOT NULL,\n" +
					"created_at DATETIME(6) NOT NULL,\n" +
					"PRIMARY KEY (event_id, event_version),\n" +
					"KEY idx_research_event_status_asof (status, system_as_of)\n" +
					") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin",
				"CREATE TABLE research_claim_versions (\n" +
					"claim_id VARCHAR(255) NOT NULL,\n" +
					"claim_version INT UNSIGNED NOT NULL,\n" +
					"epistemic_type VARCHAR(24) NOT NULL,\n" +
					"status VARCHAR(24) NOT NULL,\n" +
					"system_as_of DATETIME(6) NOT NULL,\n" +
					"payload_sha256 CHAR(64) NOT NULL,\n" +
					"payload LONGTEXT NOT NULL,\n" +
					"created_at DATETIME(6) NOT NULL,\n" +
					"PRIMARY KEY (claim_id, claim_version),\n" +
					"KEY idx_research_claim_status_asof (status, system_as_of)\n" +
					") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin",
			},
		},
	}
}
