package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

var requiredResearchflowSchemaColumns = map[string][]string{
	"researchflow_runs": {
		"run_ref", "schema_version", "status", "version", "system_as_of", "deadline_at",
		"template_ref", "template_version", "template_fingerprint", "input_snapshot_refs",
		"resolved_template_snapshot", "parameters_snapshot", "originating_request_id",
		"correlation_id", "result_summary_ref", "result_ref", "termination_reason", "created_at",
		"started_at", "finished_at",
	},
	"researchflow_stages": {
		"stage_ref", "run_ref", "stage_code", "status", "version", "required",
		"dependency_stage_refs", "retry_policy_ref", "attempt_no", "retry_at", "output_ref",
		"quality_summary_ref", "diagnostic_refs", "quality_status", "reason_codes",
		"risk_decision", "created_at", "started_at", "finished_at",
	},
	"researchflow_attempts": {
		"attempt_ref", "run_ref", "stage_ref", "attempt_no", "status", "lease_owner",
		"lease_expires_at", "fencing_token", "started_at", "finished_at", "fault_ref", "output_ref",
	},
	"researchflow_run_idempotency": {
		"caller_ref", "operation", "idempotency_key", "request_fingerprint", "run_ref", "created_at",
	},
	"researchflow_result_summaries": {
		"result_summary_ref", "run_ref", "schema_version", "payload", "created_at",
	},
	"researchflow_run_results": {
		"result_ref", "run_ref", "schema_version", "payload", "created_at",
	},
	"researchflow_outbox": {
		"outbox_ref", "run_ref", "event_type", "schema_version", "idempotency_key",
		"correlation_id", "payload_fingerprint", "payload", "created_at", "published_at",
		"lease_owner", "lease_expires_at", "lease_generation", "next_attempt_at",
		"attempt_count", "last_error_code",
	},
	"researchflow_workflow_templates": {
		"template_ref", "version", "fingerprint", "definition", "created_at",
	},
	"researchflow_capacity_guard": {
		"guard_id",
	},
	"researchflow_stage_output_resources": {
		"resource_ref", "run_ref", "stage_ref", "attempt_ref", "resource_kind",
		"stage_code", "template_version", "registry_version", "registry_fingerprint",
		"output_type", "schema_version", "schema_ref", "content", "created_at",
	},
}

type researchflowSchemaIndexDefinition struct {
	table   string
	name    string
	unique  bool
	columns []string
}

type researchflowSchemaIndexState struct {
	unique  bool
	columns []string
}

var requiredResearchflowSchemaIndexes = []researchflowSchemaIndexDefinition{
	{table: "researchflow_runs", name: "PRIMARY", unique: true, columns: []string{"run_ref"}},
	{table: "researchflow_runs", name: "idx_researchflow_runs_status_deadline", columns: []string{"status", "deadline_at"}},
	{table: "researchflow_runs", name: "idx_researchflow_runs_correlation", columns: []string{"correlation_id"}},
	{table: "researchflow_stages", name: "PRIMARY", unique: true, columns: []string{"stage_ref"}},
	{table: "researchflow_stages", name: "uq_researchflow_stage_run_code", unique: true, columns: []string{"run_ref", "stage_code"}},
	{table: "researchflow_stages", name: "uq_researchflow_stage_ref_run", unique: true, columns: []string{"stage_ref", "run_ref"}},
	{table: "researchflow_stages", name: "idx_researchflow_stages_run_status", columns: []string{"run_ref", "status"}},
	{table: "researchflow_attempts", name: "PRIMARY", unique: true, columns: []string{"attempt_ref"}},
	{table: "researchflow_attempts", name: "uq_researchflow_attempt_stage_no", unique: true, columns: []string{"stage_ref", "attempt_no"}},
	{table: "researchflow_attempts", name: "uq_researchflow_attempt_stage_fencing", unique: true, columns: []string{"stage_ref", "fencing_token"}},
	{table: "researchflow_attempts", name: "idx_researchflow_attempts_run_status", columns: []string{"run_ref", "status"}},
	{table: "researchflow_attempts", name: "idx_researchflow_attempts_lease", columns: []string{"status", "lease_expires_at"}},
	{table: "researchflow_run_idempotency", name: "PRIMARY", unique: true, columns: []string{"caller_ref", "operation", "idempotency_key"}},
	{table: "researchflow_run_idempotency", name: "uq_researchflow_idempotency_run", unique: true, columns: []string{"run_ref"}},
	{table: "researchflow_result_summaries", name: "PRIMARY", unique: true, columns: []string{"result_summary_ref"}},
	{table: "researchflow_result_summaries", name: "uq_researchflow_result_summary_run", unique: true, columns: []string{"run_ref"}},
	{table: "researchflow_run_results", name: "PRIMARY", unique: true, columns: []string{"result_ref"}},
	{table: "researchflow_run_results", name: "uq_researchflow_run_result_run", unique: true, columns: []string{"run_ref"}},
	{table: "researchflow_outbox", name: "PRIMARY", unique: true, columns: []string{"outbox_ref"}},
	{table: "researchflow_outbox", name: "uq_researchflow_outbox_run_event", unique: true, columns: []string{"run_ref", "event_type"}},
	{table: "researchflow_outbox", name: "uq_researchflow_outbox_idempotency", unique: true, columns: []string{"idempotency_key"}},
	{table: "researchflow_outbox", name: "idx_researchflow_outbox_pending", columns: []string{"published_at", "created_at"}},
	{table: "researchflow_outbox", name: "idx_researchflow_outbox_claim", columns: []string{"published_at", "next_attempt_at", "lease_expires_at", "created_at"}},
	{table: "researchflow_workflow_templates", name: "PRIMARY", unique: true, columns: []string{"template_ref", "version"}},
	{table: "researchflow_workflow_templates", name: "idx_researchflow_templates_fingerprint", columns: []string{"fingerprint"}},
	{table: "researchflow_capacity_guard", name: "PRIMARY", unique: true, columns: []string{"guard_id"}},
	{table: "researchflow_stage_output_resources", name: "PRIMARY", unique: true, columns: []string{"resource_ref"}},
	{table: "researchflow_stage_output_resources", name: "uq_researchflow_stage_output_attempt_kind", unique: true, columns: []string{"attempt_ref", "resource_kind"}},
	{table: "researchflow_stage_output_resources", name: "idx_researchflow_stage_output_run_stage", columns: []string{"run_ref", "stage_ref"}},
}

// ValidateSchema performs a read-only check of the physical Researchflow
// tables, columns and indexes. Applied migration checksums do not detect
// manual DDL drift after a migration has been recorded as applied.
func ValidateSchema(ctx context.Context, db *sql.DB) error {
	if ctx == nil || db == nil {
		return fmt.Errorf("Researchflow schema validation requires context and MariaDB")
	}

	const tableNames = "'researchflow_runs', 'researchflow_stages', 'researchflow_attempts', " +
		"'researchflow_run_idempotency', 'researchflow_result_summaries', 'researchflow_run_results', 'researchflow_outbox', " +
		"'researchflow_workflow_templates', 'researchflow_capacity_guard', 'researchflow_stage_output_resources'"
	rows, err := db.QueryContext(ctx, "SELECT table_name, column_name\n"+
		"FROM information_schema.columns\n"+
		"WHERE table_schema = DATABASE() AND table_name IN ("+tableNames+")\n"+
		"ORDER BY table_name, ordinal_position")
	if err != nil {
		return fmt.Errorf("inspect Researchflow schema columns: %w", err)
	}
	columns := make(map[string]map[string]struct{}, len(requiredResearchflowSchemaColumns))
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read Researchflow schema columns: %w", err)
		}
		if columns[table] == nil {
			columns[table] = make(map[string]struct{})
		}
		columns[table][column] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("read Researchflow schema columns: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close Researchflow schema column inspection: %w", err)
	}
	if err := validateResearchflowSchemaColumns(columns); err != nil {
		return err
	}

	indexRows, err := db.QueryContext(ctx, "SELECT table_name, index_name, non_unique, seq_in_index, column_name\n"+
		"FROM information_schema.statistics\n"+
		"WHERE table_schema = DATABASE() AND table_name IN ("+tableNames+")\n"+
		"ORDER BY table_name, index_name, seq_in_index")
	if err != nil {
		return fmt.Errorf("inspect Researchflow schema indexes: %w", err)
	}
	indexes := make(map[string]map[string]researchflowSchemaIndexState, len(requiredResearchflowSchemaColumns))
	for indexRows.Next() {
		var table, name, column string
		var nonUnique, sequence int
		if err := indexRows.Scan(&table, &name, &nonUnique, &sequence, &column); err != nil {
			_ = indexRows.Close()
			return fmt.Errorf("read Researchflow schema indexes: %w", err)
		}
		if indexes[table] == nil {
			indexes[table] = make(map[string]researchflowSchemaIndexState)
		}
		state := indexes[table][name]
		state.unique = nonUnique == 0
		state.columns = append(state.columns, column)
		indexes[table][name] = state
	}
	if err := indexRows.Err(); err != nil {
		_ = indexRows.Close()
		return fmt.Errorf("read Researchflow schema indexes: %w", err)
	}
	if err := indexRows.Close(); err != nil {
		return fmt.Errorf("close Researchflow schema index inspection: %w", err)
	}
	if err := validateResearchflowSchemaIndexes(indexes); err != nil {
		return err
	}
	return nil
}

func validateResearchflowSchemaColumns(actual map[string]map[string]struct{}) error {
	tables := make([]string, 0, len(requiredResearchflowSchemaColumns))
	for table := range requiredResearchflowSchemaColumns {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		found, exists := actual[table]
		if !exists {
			return fmt.Errorf("Researchflow schema is incomplete: required table %s is missing", table)
		}
		missing := make([]string, 0)
		for _, column := range requiredResearchflowSchemaColumns[table] {
			if _, ok := found[column]; !ok {
				missing = append(missing, column)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			return fmt.Errorf("Researchflow schema is incomplete: table %s is missing required columns: %s", table, strings.Join(missing, ", "))
		}
	}
	return nil
}

func validateResearchflowSchemaIndexes(actual map[string]map[string]researchflowSchemaIndexState) error {
	for _, expected := range requiredResearchflowSchemaIndexes {
		found, exists := actual[expected.table][expected.name]
		if !exists {
			return fmt.Errorf("Researchflow schema is incomplete: table %s is missing required index %s", expected.table, expected.name)
		}
		if found.unique != expected.unique || !equalResearchflowSchemaColumns(found.columns, expected.columns) {
			return fmt.Errorf(
				"Researchflow schema index mismatch: table %s index %s has unique=%t columns=%v; want unique=%t columns=%v",
				expected.table, expected.name, found.unique, found.columns, expected.unique, expected.columns,
			)
		}
	}
	return nil
}

func equalResearchflowSchemaColumns(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
