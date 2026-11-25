package storage

import (
	"strings"
	"testing"
)

func completeResearchflowSchemaColumns() map[string]map[string]struct{} {
	actual := make(map[string]map[string]struct{}, len(requiredResearchflowSchemaColumns))
	for table, columns := range requiredResearchflowSchemaColumns {
		actual[table] = make(map[string]struct{}, len(columns))
		for _, column := range columns {
			actual[table][column] = struct{}{}
		}
	}
	return actual
}

func completeResearchflowSchemaIndexes() map[string]map[string]researchflowSchemaIndexState {
	actual := make(map[string]map[string]researchflowSchemaIndexState)
	for _, expected := range requiredResearchflowSchemaIndexes {
		if actual[expected.table] == nil {
			actual[expected.table] = make(map[string]researchflowSchemaIndexState)
		}
		actual[expected.table][expected.name] = researchflowSchemaIndexState{
			unique: expected.unique, columns: append([]string(nil), expected.columns...),
		}
	}
	return actual
}

func TestValidateResearchflowSchemaColumnsAcceptsCompleteBaseline(t *testing.T) {
	if err := validateResearchflowSchemaColumns(completeResearchflowSchemaColumns()); err != nil {
		t.Fatalf("complete Researchflow schema rejected: %v", err)
	}
}

func TestValidateResearchflowSchemaColumnsRejectsMissingTableAndColumn(t *testing.T) {
	t.Run("missing table", func(t *testing.T) {
		actual := completeResearchflowSchemaColumns()
		delete(actual, "researchflow_attempts")
		err := validateResearchflowSchemaColumns(actual)
		if err == nil || !strings.Contains(err.Error(), "researchflow_attempts") {
			t.Fatalf("missing table error = %v", err)
		}
	})
	t.Run("missing column", func(t *testing.T) {
		actual := completeResearchflowSchemaColumns()
		delete(actual["researchflow_stage_output_resources"], "registry_fingerprint")
		err := validateResearchflowSchemaColumns(actual)
		if err == nil || !strings.Contains(err.Error(), "registry_fingerprint") {
			t.Fatalf("missing column error = %v", err)
		}
	})
}

func TestValidateResearchflowSchemaIndexesRejectsMissingAndDriftedIndexes(t *testing.T) {
	t.Run("missing index", func(t *testing.T) {
		actual := completeResearchflowSchemaIndexes()
		delete(actual["researchflow_attempts"], "uq_researchflow_attempt_stage_fencing")
		err := validateResearchflowSchemaIndexes(actual)
		if err == nil || !strings.Contains(err.Error(), "uq_researchflow_attempt_stage_fencing") {
			t.Fatalf("missing index error = %v", err)
		}
	})
	t.Run("wrong order and uniqueness", func(t *testing.T) {
		actual := completeResearchflowSchemaIndexes()
		actual["researchflow_outbox"]["idx_researchflow_outbox_claim"] = researchflowSchemaIndexState{
			unique: true, columns: []string{"published_at", "lease_expires_at", "next_attempt_at", "created_at"},
		}
		err := validateResearchflowSchemaIndexes(actual)
		if err == nil || !strings.Contains(err.Error(), "idx_researchflow_outbox_claim") {
			t.Fatalf("drifted index error = %v", err)
		}
	})
}
