package storage

import (
	"context"
	"database/sql"
	"fmt"

	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

const MigrationNamespace = "analysis"

// Migrations 返回 Analysis 当前完整 Schema 基线。
// Analysis 的持久化结构由 Root Server Migration Registry 统一创建。
func Migrations() []platformstorage.Migration {
	return []platformstorage.Migration{{
		Version:     1,
		Description: "create complete Analysis schema baseline",
		Statements: []string{
			"CREATE TABLE analysis_runs (run_id VARCHAR(64) NOT NULL, status VARCHAR(32) NOT NULL, task_type VARCHAR(64) NOT NULL, as_of DATETIME(6) NOT NULL, started_at DATETIME(6) NOT NULL, finished_at DATETIME(6) NULL, request_json LONGTEXT NOT NULL, result_json LONGTEXT NULL, created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6), updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6), PRIMARY KEY (run_id), KEY idx_runs_asof (as_of), KEY idx_runs_status (status)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci",
			"CREATE TABLE analysis_stage_events (id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, run_id VARCHAR(64) NOT NULL, stage VARCHAR(100) NOT NULL, status VARCHAR(32) NOT NULL, event_at DATETIME(6) NOT NULL, duration_ms BIGINT NOT NULL DEFAULT 0, event_json LONGTEXT NOT NULL, created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6), PRIMARY KEY (id), KEY idx_events_run (run_id), KEY idx_events_run_stage (run_id, stage), CONSTRAINT fk_events_run FOREIGN KEY (run_id) REFERENCES analysis_runs(run_id) ON DELETE CASCADE) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci",
			"CREATE TABLE analysis_snapshots (snapshot_id VARCHAR(96) NOT NULL, run_id VARCHAR(64) NOT NULL, snapshot_type VARCHAR(32) NOT NULL, as_of DATETIME(6) NOT NULL, payload_json LONGTEXT NOT NULL, created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6), PRIMARY KEY (snapshot_id), KEY idx_snapshots_run (run_id), KEY idx_snapshots_type_time (snapshot_type, as_of), CONSTRAINT fk_snapshots_run FOREIGN KEY (run_id) REFERENCES analysis_runs(run_id) ON DELETE CASCADE) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci",
			"CREATE TABLE analysis_tasks (task_id VARCHAR(64) NOT NULL, run_id VARCHAR(64) NOT NULL, status VARCHAR(24) NOT NULL, task_type VARCHAR(64) NOT NULL, request_json LONGTEXT NOT NULL, idempotency_key VARCHAR(128) NULL, current_stage VARCHAR(100) NULL, failure_code VARCHAR(100) NULL, error_message VARCHAR(300) NULL, created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6), started_at DATETIME(6) NULL, finished_at DATETIME(6) NULL, updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6), PRIMARY KEY (task_id), UNIQUE KEY uq_analysis_tasks_run (run_id), UNIQUE KEY uq_analysis_tasks_idempotency (idempotency_key), KEY idx_tasks_status_created (status, created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci",
		},
	}}
}

 
// ValidateSchema verifies that all Analysis tables from the current baseline
// exist before Server creates Redis and application repositories.
func ValidateSchema(ctx context.Context, db *sql.DB) error {
	if ctx == nil || db == nil {
		return fmt.Errorf("Analysis schema validation requires context and MariaDB")
	}
	const requiredTables = 4
	var found int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = DATABASE()
		AND table_name IN ('analysis_runs', 'analysis_stage_events', 'analysis_snapshots', 'analysis_tasks')`).Scan(&found); err != nil {
		return fmt.Errorf("check Analysis schema: %w", err)
	}
	if found != requiredTables {
		return fmt.Errorf("Analysis schema is incomplete: found %d of %d required tables", found, requiredTables)
	}
	return nil
}
