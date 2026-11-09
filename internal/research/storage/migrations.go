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
		{
			Version:     2,
			Description: "preserve RFC3339Nano precision for point-in-time timestamps",
			Statements: []string{
				"ALTER TABLE research_context_versions ADD COLUMN system_as_of_exact CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL",
				"UPDATE research_context_versions SET system_as_of_exact = CONCAT(DATE_FORMAT(system_as_of, '%Y-%m-%dT%H:%i:%s.%f'), '000Z')",
				"ALTER TABLE research_context_versions DROP INDEX idx_research_context_asof, DROP COLUMN system_as_of, CHANGE COLUMN system_as_of_exact system_as_of CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, ADD KEY idx_research_context_asof (context_id, system_as_of)",
				"ALTER TABLE research_evidence_records ADD COLUMN available_at_exact CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL, ADD COLUMN system_observed_at_exact CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL, ADD COLUMN system_as_of_exact CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL",
				"UPDATE research_evidence_records SET available_at_exact = CASE WHEN available_at IS NULL THEN NULL ELSE CONCAT(DATE_FORMAT(available_at, '%Y-%m-%dT%H:%i:%s.%f'), '000Z') END, system_observed_at_exact = CONCAT(DATE_FORMAT(system_observed_at, '%Y-%m-%dT%H:%i:%s.%f'), '000Z'), system_as_of_exact = CONCAT(DATE_FORMAT(system_as_of, '%Y-%m-%dT%H:%i:%s.%f'), '000Z')",
				"ALTER TABLE research_evidence_records DROP INDEX idx_research_evidence_pit, DROP COLUMN available_at, DROP COLUMN system_observed_at, DROP COLUMN system_as_of, CHANGE COLUMN available_at_exact available_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL, CHANGE COLUMN system_observed_at_exact system_observed_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, CHANGE COLUMN system_as_of_exact system_as_of CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, ADD KEY idx_research_evidence_pit (system_as_of, available_at)",
				"ALTER TABLE research_event_versions ADD COLUMN system_as_of_exact CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL",
				"UPDATE research_event_versions SET system_as_of_exact = CONCAT(DATE_FORMAT(system_as_of, '%Y-%m-%dT%H:%i:%s.%f'), '000Z')",
				"ALTER TABLE research_event_versions DROP INDEX idx_research_event_status_asof, DROP COLUMN system_as_of, CHANGE COLUMN system_as_of_exact system_as_of CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, ADD KEY idx_research_event_status_asof (status, system_as_of)",
				"ALTER TABLE research_claim_versions ADD COLUMN system_as_of_exact CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL",
				"UPDATE research_claim_versions SET system_as_of_exact = CONCAT(DATE_FORMAT(system_as_of, '%Y-%m-%dT%H:%i:%s.%f'), '000Z')",
				"ALTER TABLE research_claim_versions DROP INDEX idx_research_claim_status_asof, DROP COLUMN system_as_of, CHANGE COLUMN system_as_of_exact system_as_of CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, ADD KEY idx_research_claim_status_asof (status, system_as_of)",
			},
		},
		{
			Version:     3,
			Description: "align Event and Claim version columns with Go int range",
			Statements: []string{
				"ALTER TABLE research_event_versions MODIFY COLUMN event_version BIGINT UNSIGNED NOT NULL",
				"ALTER TABLE research_claim_versions MODIFY COLUMN claim_version BIGINT UNSIGNED NOT NULL",
			},
		},
		{
			Version:     4,
			Description: "add transactional Research lifecycle notification Outbox",
			Statements: []string{
				"CREATE TABLE research_notification_outbox (\n" +
					"event_id VARCHAR(255) NOT NULL,\n" +
					"event_type VARCHAR(128) NOT NULL,\n" +
					"schema_version VARCHAR(32) NOT NULL,\n" +
					"source_module VARCHAR(32) NOT NULL,\n" +
					"occurred_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,\n" +
					"published_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,\n" +
					"subject_ref VARCHAR(512) NOT NULL,\n" +
					"idempotency_key VARCHAR(512) NOT NULL,\n" +
					"correlation_id VARCHAR(128) NOT NULL,\n" +
					"payload_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,\n" +
					"payload LONGTEXT NOT NULL,\n" +
					"created_at DATETIME(6) NOT NULL,\n" +
					"PRIMARY KEY (event_id),\n" +
					"UNIQUE KEY uq_research_outbox_idempotency (source_module, idempotency_key),\n" +
					"KEY idx_research_outbox_created (created_at)\n" +
					") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin",
			},
		},
		{
			Version: 5,
			Description: "add reliable Research Outbox delivery leases and retry state",
			Statements: []string{
				"ALTER TABLE research_notification_outbox ADD COLUMN delivery_status VARCHAR(16) NOT NULL DEFAULT 'pending', ADD COLUMN available_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL, ADD COLUMN lease_owner VARCHAR(128) NULL, ADD COLUMN lease_token CHAR(32) CHARACTER SET ascii COLLATE ascii_bin NULL, ADD COLUMN lease_until CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL, ADD COLUMN attempt_count INT UNSIGNED NOT NULL DEFAULT 0, ADD COLUMN last_error_code VARCHAR(128) NULL, ADD COLUMN delivered_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL, ADD COLUMN updated_at DATETIME(6) NULL",
				"UPDATE research_notification_outbox SET available_at = CONCAT(DATE_FORMAT(created_at, '%Y-%m-%dT%H:%i:%s.%f'), '000Z'), updated_at = created_at",
				"ALTER TABLE research_notification_outbox MODIFY COLUMN available_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, MODIFY COLUMN updated_at DATETIME(6) NOT NULL, ADD KEY idx_research_outbox_delivery (delivery_status, available_at, created_at)",
			},
		},
	}
}
