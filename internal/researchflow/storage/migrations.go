package storage

import platformstorage "example.com/stock-analysis-flow/internal/platform/storage"

const MigrationNamespace = "researchflow"

// Migrations returns Researchflow-owned tables. The Server composition root
// registers these migrations with Platform storage; importing this package
// never executes DDL.
func Migrations() []platformstorage.Migration {
	return []platformstorage.Migration{
		{
			Version:     1,
			Description: "create transactional Researchflow run, stage, attempt, result and outbox storage",
			Statements: []string{
				`CREATE TABLE researchflow_runs (
					run_ref VARCHAR(255) NOT NULL,
					schema_version VARCHAR(32) NOT NULL,
					status VARCHAR(16) NOT NULL,
					version BIGINT UNSIGNED NOT NULL,
					system_as_of CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					deadline_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					template_ref VARCHAR(255) NOT NULL,
					template_version VARCHAR(64) NOT NULL,
					template_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					input_snapshot_refs LONGTEXT NOT NULL,
					resolved_template_snapshot LONGTEXT NOT NULL,
					parameters_snapshot LONGTEXT NOT NULL,
					originating_request_id VARCHAR(128) NULL,
					correlation_id VARCHAR(128) NOT NULL,
					result_summary_ref VARCHAR(255) NULL,
					termination_reason VARCHAR(64) NULL,
					created_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					started_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL,
					finished_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL,
					PRIMARY KEY (run_ref),
					KEY idx_researchflow_runs_status_deadline (status, deadline_at),
					KEY idx_researchflow_runs_correlation (correlation_id)
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`,
				`CREATE TABLE researchflow_stages (
					stage_ref VARCHAR(255) NOT NULL,
					run_ref VARCHAR(255) NOT NULL,
					stage_code VARCHAR(128) NOT NULL,
					status VARCHAR(16) NOT NULL,
					version BIGINT UNSIGNED NOT NULL,
					required TINYINT(1) NOT NULL,
					dependency_stage_refs LONGTEXT NOT NULL,
					retry_policy_ref VARCHAR(255) NOT NULL,
					attempt_no INT UNSIGNED NOT NULL DEFAULT 0,
					retry_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL,
					output_ref VARCHAR(255) NULL,
					quality_summary_ref VARCHAR(255) NULL,
					diagnostic_refs LONGTEXT NOT NULL,
					quality_status VARCHAR(16) NULL,
					reason_codes LONGTEXT NOT NULL,
					risk_decision VARCHAR(16) NULL,
					created_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					started_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL,
					finished_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL,
					PRIMARY KEY (stage_ref),
					UNIQUE KEY uq_researchflow_stage_run_code (run_ref, stage_code),
					UNIQUE KEY uq_researchflow_stage_ref_run (stage_ref, run_ref),
					KEY idx_researchflow_stages_run_status (run_ref, status),
					CONSTRAINT fk_researchflow_stages_run FOREIGN KEY (run_ref)
						REFERENCES researchflow_runs (run_ref)
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`,
				`CREATE TABLE researchflow_attempts (
					attempt_ref VARCHAR(255) NOT NULL,
					run_ref VARCHAR(255) NOT NULL,
					stage_ref VARCHAR(255) NOT NULL,
					attempt_no INT UNSIGNED NOT NULL,
					status VARCHAR(16) NOT NULL,
					lease_owner VARCHAR(128) NULL,
					lease_expires_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL,
					fencing_token BIGINT UNSIGNED NOT NULL,
					started_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					finished_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL,
					fault_ref VARCHAR(255) NULL,
					output_ref VARCHAR(255) NULL,
					PRIMARY KEY (attempt_ref),
					UNIQUE KEY uq_researchflow_attempt_stage_no (stage_ref, attempt_no),
					UNIQUE KEY uq_researchflow_attempt_stage_fencing (stage_ref, fencing_token),
					KEY idx_researchflow_attempts_run_status (run_ref, status),
					KEY idx_researchflow_attempts_lease (status, lease_expires_at),
					CONSTRAINT fk_researchflow_attempts_run FOREIGN KEY (run_ref)
						REFERENCES researchflow_runs (run_ref),
					CONSTRAINT fk_researchflow_attempts_stage FOREIGN KEY (stage_ref, run_ref)
						REFERENCES researchflow_stages (stage_ref, run_ref)
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`,
				`CREATE TABLE researchflow_run_idempotency (
					caller_ref VARCHAR(255) NOT NULL,
					operation VARCHAR(64) NOT NULL,
					idempotency_key VARCHAR(128) NOT NULL,
					request_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					run_ref VARCHAR(255) NOT NULL,
					created_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					PRIMARY KEY (caller_ref, operation, idempotency_key),
					KEY idx_researchflow_idempotency_run (run_ref),
					CONSTRAINT fk_researchflow_idempotency_run FOREIGN KEY (run_ref)
						REFERENCES researchflow_runs (run_ref)
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`,
				`CREATE TABLE researchflow_result_summaries (
					result_summary_ref VARCHAR(255) NOT NULL,
					run_ref VARCHAR(255) NOT NULL,
					schema_version VARCHAR(32) NOT NULL,
					payload LONGTEXT NOT NULL,
					created_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					PRIMARY KEY (result_summary_ref),
					UNIQUE KEY uq_researchflow_result_summary_run (run_ref),
					CONSTRAINT fk_researchflow_result_summaries_run FOREIGN KEY (run_ref)
						REFERENCES researchflow_runs (run_ref)
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`,
				`CREATE TABLE researchflow_outbox (
					outbox_ref VARCHAR(255) NOT NULL,
					run_ref VARCHAR(255) NOT NULL,
					event_type VARCHAR(128) NOT NULL,
					schema_version VARCHAR(32) NOT NULL,
					idempotency_key VARCHAR(255) NOT NULL,
					correlation_id VARCHAR(128) NOT NULL,
					payload_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					payload LONGTEXT NOT NULL,
					created_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
					published_at CHAR(30) CHARACTER SET ascii COLLATE ascii_bin NULL,
					attempt_count INT UNSIGNED NOT NULL DEFAULT 0,
					last_error_code VARCHAR(128) NULL,
					PRIMARY KEY (outbox_ref),
					UNIQUE KEY uq_researchflow_outbox_run_event (run_ref, event_type),
					UNIQUE KEY uq_researchflow_outbox_idempotency (idempotency_key),
					KEY idx_researchflow_outbox_pending (published_at, created_at),
					CONSTRAINT fk_researchflow_outbox_run FOREIGN KEY (run_ref)
						REFERENCES researchflow_runs (run_ref)
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`,
			},
		},
	}
}
