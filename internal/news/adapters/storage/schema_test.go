package storage

import (
	"strings"
	"testing"
)

func completeNewsSchemaColumns() map[string]map[string]struct{} {
	actual := make(map[string]map[string]struct{}, len(requiredNewsSchemaColumns))
	for table, columns := range requiredNewsSchemaColumns {
		actual[table] = make(map[string]struct{}, len(columns))
		for _, column := range columns {
			actual[table][column] = struct{}{}
		}
	}
	return actual
}

func completeNewsSchemaIndexes() map[string]map[string]newsSchemaIndexState {
	actual := make(map[string]map[string]newsSchemaIndexState)
	for _, expected := range requiredNewsSchemaIndexes {
		if actual[expected.table] == nil {
			actual[expected.table] = make(map[string]newsSchemaIndexState)
		}
		actual[expected.table][expected.name] = newsSchemaIndexState{
			unique: expected.unique, columns: append([]string(nil), expected.columns...),
		}
	}
	return actual
}

func TestValidateNewsSchemaColumnsAcceptsMigrationBaseline(t *testing.T) {
	if len(requiredNewsSchemaColumns) != 14 {
		t.Fatalf("News schema table count = %d, want 14", len(requiredNewsSchemaColumns))
	}
	if err := validateNewsSchemaColumns(completeNewsSchemaColumns()); err != nil {
		t.Fatalf("complete News schema rejected: %v", err)
	}
}

func TestValidateNewsSchemaColumnsRejectsMissingTableAndPITColumn(t *testing.T) {
	t.Run("missing table", func(t *testing.T) {
		actual := completeNewsSchemaColumns()
		delete(actual, "news_version_availability_evidence")
		err := validateNewsSchemaColumns(actual)
		if err == nil || !strings.Contains(err.Error(), "news_version_availability_evidence") {
			t.Fatalf("missing table error = %v", err)
		}
	})
	t.Run("missing PIT column", func(t *testing.T) {
		actual := completeNewsSchemaColumns()
		delete(actual["news_version_availability_evidence"], "candidate_available_at_utc")
		err := validateNewsSchemaColumns(actual)
		if err == nil || !strings.Contains(err.Error(), "candidate_available_at_utc") {
			t.Fatalf("missing PIT column error = %v", err)
		}
	})
}

func TestValidateNewsSchemaIndexesRejectsMissingAndDriftedIndexes(t *testing.T) {
	t.Run("missing idempotency index", func(t *testing.T) {
		actual := completeNewsSchemaIndexes()
		delete(actual["news_observations"], "uk_news_observations_idempotency")
		err := validateNewsSchemaIndexes(actual)
		if err == nil || !strings.Contains(err.Error(), "uk_news_observations_idempotency") {
			t.Fatalf("missing index error = %v", err)
		}
	})
	t.Run("wrong order and uniqueness", func(t *testing.T) {
		actual := completeNewsSchemaIndexes()
		actual["news_version_availability_decisions"]["idx_news_availability_decision_pit"] = newsSchemaIndexState{
			unique: true, columns: []string{"version_id", "decision_seq", "recorded_at"},
		}
		err := validateNewsSchemaIndexes(actual)
		if err == nil || !strings.Contains(err.Error(), "idx_news_availability_decision_pit") {
			t.Fatalf("drifted PIT index error = %v", err)
		}
	})
}

func TestValidateNewsSchemaIndexesAcceptsMigrationBaseline(t *testing.T) {
	if err := validateNewsSchemaIndexes(completeNewsSchemaIndexes()); err != nil {
		t.Fatalf("complete News indexes rejected: %v", err)
	}
}
