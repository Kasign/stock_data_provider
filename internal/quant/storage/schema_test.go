package storage

import (
	"strings"
	"testing"
)

func completeQuantSchemaColumns() map[string]map[string]struct{} {
	actual := make(map[string]map[string]struct{}, len(requiredQuantSchemaColumns))
	for table, columns := range requiredQuantSchemaColumns {
		actual[table] = make(map[string]struct{}, len(columns))
		for _, column := range columns {
			actual[table][column] = struct{}{}
		}
	}
	return actual
}

func completeQuantSchemaIndexes() map[string]map[string]quantSchemaIndexState {
	actual := make(map[string]map[string]quantSchemaIndexState)
	for _, expected := range requiredQuantSchemaIndexes {
		if actual[expected.table] == nil {
			actual[expected.table] = make(map[string]quantSchemaIndexState)
		}
		actual[expected.table][expected.name] = quantSchemaIndexState{
			unique: expected.unique,
			columns: append([]string(nil), expected.columns...),
		}
	}
	return actual
}

func TestValidateQuantSchemaColumnsAcceptsCompleteBaseline(t *testing.T) {
	if err := validateQuantSchemaColumns(completeQuantSchemaColumns()); err != nil {
		t.Fatalf("validateQuantSchemaColumns() error = %v", err)
	}
}

func TestValidateQuantSchemaColumnsRejectsMissingTableAndColumn(t *testing.T) {
	missingTable := completeQuantSchemaColumns()
	delete(missingTable, "quant_access_audit")
	if err := validateQuantSchemaColumns(missingTable); err == nil || !strings.Contains(err.Error(), "quant_access_audit") {
		t.Fatalf("missing table error = %v", err)
	}

	missingColumn := completeQuantSchemaColumns()
	delete(missingColumn["quant_access_audit"], "authorization_snapshot_json")
	if err := validateQuantSchemaColumns(missingColumn); err == nil ||
		!strings.Contains(err.Error(), "authorization_snapshot_json") {
		t.Fatalf("missing column error = %v", err)
	}
}

func TestValidateQuantSchemaIndexesRejectsMissingAndDriftedIndexes(t *testing.T) {
	missing := completeQuantSchemaIndexes()
	delete(missing["quant_access_audit"], "idx_quant_access_audit_run_id")
	if err := validateQuantSchemaIndexes(missing); err == nil ||
		!strings.Contains(err.Error(), "idx_quant_access_audit_run_id") {
		t.Fatalf("missing index error = %v", err)
	}

	drifted := completeQuantSchemaIndexes()
	drifted["quant_policy_admin_audit"]["idx_quant_policy_admin_audit_policy"] =
		quantSchemaIndexState{columns: []string{"policy_ref", "policy_type", "audit_id"}}
	if err := validateQuantSchemaIndexes(drifted); err == nil ||
		!strings.Contains(err.Error(), "idx_quant_policy_admin_audit_policy") {
		t.Fatalf("drifted index error = %v", err)
	}
}
