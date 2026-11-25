package storage

import (
	"strings"
	"testing"
)

func completeResearchSchemaColumns() map[string]map[string]struct{} {
	actual := make(map[string]map[string]struct{}, len(requiredResearchSchemaColumns))
	for table, columns := range requiredResearchSchemaColumns {
		actual[table] = make(map[string]struct{}, len(columns))
		for _, column := range columns {
			actual[table][column] = struct{}{}
		}
	}
	return actual
}

func completeResearchSchemaIndexes() map[string]map[string]researchSchemaIndexState {
	actual := make(map[string]map[string]researchSchemaIndexState)
	for _, expected := range requiredResearchSchemaIndexes {
		if actual[expected.table] == nil {
			actual[expected.table] = make(map[string]researchSchemaIndexState)
		}
		actual[expected.table][expected.name] = researchSchemaIndexState{
			unique: expected.unique, columns: append([]string(nil), expected.columns...),
		}
	}
	return actual
}

func TestValidateResearchSchemaColumnsAcceptsCompleteBaseline(t *testing.T) {
	if err := validateResearchSchemaColumns(completeResearchSchemaColumns()); err != nil {
		t.Fatalf("complete Research schema rejected: %v", err)
	}
}

func TestValidateResearchSchemaColumnsRejectsMissingTableAndColumn(t *testing.T) {
	t.Run("missing table", func(t *testing.T) {
		actual := completeResearchSchemaColumns()
		delete(actual, "research_evidence_records")
		err := validateResearchSchemaColumns(actual)
		if err == nil || !strings.Contains(err.Error(), "research_evidence_records") {
			t.Fatalf("missing table error = %v", err)
		}
	})
	t.Run("missing PIT column", func(t *testing.T) {
		actual := completeResearchSchemaColumns()
		delete(actual["research_evidence_records"], "available_at")
		err := validateResearchSchemaColumns(actual)
		if err == nil || !strings.Contains(err.Error(), "available_at") {
			t.Fatalf("missing PIT column error = %v", err)
		}
	})
}

func TestValidateResearchSchemaIndexesRejectsMissingAndDriftedIndexes(t *testing.T) {
	t.Run("missing PIT index", func(t *testing.T) {
		actual := completeResearchSchemaIndexes()
		delete(actual["research_evidence_records"], "idx_research_evidence_pit")
		err := validateResearchSchemaIndexes(actual)
		if err == nil || !strings.Contains(err.Error(), "idx_research_evidence_pit") {
			t.Fatalf("missing PIT index error = %v", err)
		}
	})
	t.Run("wrong order and uniqueness", func(t *testing.T) {
		actual := completeResearchSchemaIndexes()
		actual["research_notification_outbox"]["idx_research_outbox_delivery"] = researchSchemaIndexState{
			unique: true, columns: []string{"delivery_status", "created_at", "available_at"},
		}
		err := validateResearchSchemaIndexes(actual)
		if err == nil || !strings.Contains(err.Error(), "idx_research_outbox_delivery") {
			t.Fatalf("drifted index error = %v", err)
		}
	})
}
