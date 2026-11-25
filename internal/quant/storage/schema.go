package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

var requiredQuantSchemaColumns = map[string][]string{
	"quant_results": {
		"result_id", "manifest_fingerprint", "result_fingerprint",
		"manifest_json", "payload_json", "created_at",
	},
	"quant_access_audit": {
		"audit_id", "subject_ref", "run_ref", "resource_ref", "purpose_code",
		"content_level", "decision", "reason_code", "authorization_ref",
		"authorization_snapshot_json", "policy_snapshot_ref", "policy_snapshot_json",
		"decision_fingerprint", "occurred_at",
	},
	"quant_result_access_policy_versions": {
		"policy_version", "enabled", "snapshot_ref", "policy_snapshot_json", "created_at",
	},
	"quant_result_access_policy_head": {
		"policy_type", "policy_ref", "current_version", "updated_at",
	},
	"quant_policy_admin_audit": {
		"audit_id", "actor_ref", "policy_type", "policy_ref", "action",
		"previous_version", "new_version", "occurred_at",
	},
}

type quantSchemaIndexDefinition struct {
	table   string
	name    string
	unique  bool
	columns []string
}

type quantSchemaIndexState struct {
	unique  bool
	columns []string
}

var requiredQuantSchemaIndexes = []quantSchemaIndexDefinition{
	{table: "quant_results", name: "PRIMARY", unique: true, columns: []string{"result_id"}},
	{table: "quant_access_audit", name: "PRIMARY", unique: true, columns: []string{"audit_id"}},
	{table: "quant_access_audit", name: "idx_quant_access_audit_run_id", columns: []string{"run_ref", "audit_id"}},
	{table: "quant_result_access_policy_versions", name: "PRIMARY", unique: true, columns: []string{"policy_version"}},
	{table: "quant_result_access_policy_versions", name: "uq_quant_result_access_policy_snapshot", unique: true, columns: []string{"snapshot_ref"}},
	{table: "quant_result_access_policy_head", name: "PRIMARY", unique: true, columns: []string{"policy_type", "policy_ref"}},
	{table: "quant_policy_admin_audit", name: "PRIMARY", unique: true, columns: []string{"audit_id"}},
	{table: "quant_policy_admin_audit", name: "idx_quant_policy_admin_audit_policy", columns: []string{"policy_type", "policy_ref", "audit_id"}},
}

// ValidateSchema performs a read-only check of the physical Quant tables,
// required columns and indexes. Migration history alone cannot detect manual
// DDL drift after a migration has been marked applied.
func ValidateSchema(ctx context.Context, db *sql.DB) error {
	if ctx == nil || db == nil {
		return fmt.Errorf("Quant schema validation requires context and MariaDB")
	}

	const tableNames = "'quant_results', 'quant_access_audit', " +
		"'quant_result_access_policy_versions', 'quant_result_access_policy_head', " +
		"'quant_policy_admin_audit'"
	rows, err := db.QueryContext(ctx, "SELECT table_name, column_name\n"+
		"FROM information_schema.columns\n"+
		"WHERE table_schema = DATABASE() AND table_name IN ("+tableNames+")\n"+
		"ORDER BY table_name, ordinal_position")
	if err != nil {
		return fmt.Errorf("inspect Quant schema columns: %w", err)
	}
	columns := make(map[string]map[string]struct{}, len(requiredQuantSchemaColumns))
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read Quant schema columns: %w", err)
		}
		if columns[table] == nil {
			columns[table] = make(map[string]struct{})
		}
		columns[table][column] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("read Quant schema columns: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close Quant schema column inspection: %w", err)
	}
	if err := validateQuantSchemaColumns(columns); err != nil {
		return err
	}

	indexRows, err := db.QueryContext(ctx, "SELECT table_name, index_name, non_unique, seq_in_index, column_name\n"+
		"FROM information_schema.statistics\n"+
		"WHERE table_schema = DATABASE() AND table_name IN ("+tableNames+")\n"+
		"ORDER BY table_name, index_name, seq_in_index")
	if err != nil {
		return fmt.Errorf("inspect Quant schema indexes: %w", err)
	}
	indexes := make(map[string]map[string]quantSchemaIndexState, len(requiredQuantSchemaColumns))
	for indexRows.Next() {
		var table, name, column string
		var nonUnique, sequence int
		if err := indexRows.Scan(&table, &name, &nonUnique, &sequence, &column); err != nil {
			_ = indexRows.Close()
			return fmt.Errorf("read Quant schema indexes: %w", err)
		}
		if indexes[table] == nil {
			indexes[table] = make(map[string]quantSchemaIndexState)
		}
		state := indexes[table][name]
		state.unique = nonUnique == 0
		state.columns = append(state.columns, column)
		indexes[table][name] = state
	}
	if err := indexRows.Err(); err != nil {
		_ = indexRows.Close()
		return fmt.Errorf("read Quant schema indexes: %w", err)
	}
	if err := indexRows.Close(); err != nil {
		return fmt.Errorf("close Quant schema index inspection: %w", err)
	}
	if err := validateQuantSchemaIndexes(indexes); err != nil {
		return err
	}
	return nil
}

func validateQuantSchemaColumns(actual map[string]map[string]struct{}) error {
	tables := make([]string, 0, len(requiredQuantSchemaColumns))
	for table := range requiredQuantSchemaColumns {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		found, exists := actual[table]
		if !exists {
			return fmt.Errorf("Quant schema is incomplete: required table %s is missing", table)
		}
		missing := make([]string, 0)
		for _, column := range requiredQuantSchemaColumns[table] {
			if _, ok := found[column]; !ok {
				missing = append(missing, column)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			return fmt.Errorf("Quant schema is incomplete: table %s is missing required columns: %s", table, strings.Join(missing, ", "))
		}
	}
	return nil
}

func validateQuantSchemaIndexes(actual map[string]map[string]quantSchemaIndexState) error {
	for _, expected := range requiredQuantSchemaIndexes {
		found, exists := actual[expected.table][expected.name]
		if !exists {
			return fmt.Errorf("Quant schema is incomplete: table %s is missing required index %s", expected.table, expected.name)
		}
		if found.unique != expected.unique || !equalQuantSchemaColumns(found.columns, expected.columns) {
			return fmt.Errorf(
				"Quant schema index mismatch: table %s index %s has unique=%t columns=%v; want unique=%t columns=%v",
				expected.table, expected.name, found.unique, found.columns, expected.unique, expected.columns,
			)
		}
	}
	return nil
}

func equalQuantSchemaColumns(left, right []string) bool {
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
