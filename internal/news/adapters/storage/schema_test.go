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

func completeNewsSchemaTables() []string {
	tables := make([]string, 0, len(requiredNewsSchemaColumns))
	for table := range requiredNewsSchemaColumns {
		tables = append(tables, table)
	}
	return tables
}

func TestValidateNewsSchemaTableSetAcceptsFinalBaseline(t *testing.T) {
	if err := validateNewsSchemaTableSet(completeNewsSchemaTables()); err != nil {
		t.Fatalf("complete News table set rejected: %v", err)
	}
}

func TestValidateNewsSchemaTableSetRejectsObsoleteTables(t *testing.T) {
	actual := append(completeNewsSchemaTables(), "news_item_relations")
	err := validateNewsSchemaTableSet(actual)
	if err == nil || !strings.Contains(err.Error(), "table set mismatch") {
		t.Fatalf("obsolete table error = %v", err)
	}
}

func TestValidateNewsSchemaColumnsAcceptsMigrationBaseline(t *testing.T) {
	if len(requiredNewsSchemaColumns) != 10 {
		t.Fatalf("News schema table count = %d, want 10", len(requiredNewsSchemaColumns))
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
	t.Run("canonical URL is globally unique", func(t *testing.T) {
		actual := completeNewsSchemaIndexes()
		actual["news_items"]["uk_news_items_canonical_url_hash"] = newsSchemaIndexState{
			unique: false, columns: []string{"canonical_url_hash"},
		}
		err := validateNewsSchemaIndexes(actual)
		if err == nil || !strings.Contains(err.Error(), "uk_news_items_canonical_url_hash") {
			t.Fatalf("non-unique canonical URL index error = %v", err)
		}
	})
}

func TestValidateNewsSchemaIndexesAcceptsMigrationBaseline(t *testing.T) {
	if err := validateNewsSchemaIndexes(completeNewsSchemaIndexes()); err != nil {
		t.Fatalf("complete News indexes rejected: %v", err)
	}
}


func completeNewsSchemaForeignKeys() map[string]newsSchemaForeignKeyState {
	actual := make(map[string]newsSchemaForeignKeyState, len(requiredNewsSchemaForeignKeys))
	for _, expected := range requiredNewsSchemaForeignKeys {
		actual[expected.name] = newsSchemaForeignKeyState{
			table: expected.table, columns: append([]string(nil), expected.columns...),
			referencedTable: expected.referencedTable,
			referencedColumns: append([]string(nil), expected.referencedColumns...),
		}
	}
	return actual
}

func TestValidateNewsSchemaForeignKeysRejectsMissingAndDriftedConstraints(t *testing.T) {
	t.Run("missing constraint", func(t *testing.T) {
		actual := completeNewsSchemaForeignKeys()
		delete(actual, "fk_news_availability_evidence_version")
		err := validateNewsSchemaForeignKeys(actual)
		if err == nil || !strings.Contains(err.Error(), "fk_news_availability_evidence_version") {
			t.Fatalf("missing foreign key error = %v", err)
		}
	})
	t.Run("wrong referenced table", func(t *testing.T) {
		actual := completeNewsSchemaForeignKeys()
		state := actual["fk_news_identifiers_item"]
		state.referencedTable = "news_item_versions"
		actual["fk_news_identifiers_item"] = state
		err := validateNewsSchemaForeignKeys(actual)
		if err == nil || !strings.Contains(err.Error(), "fk_news_identifiers_item") {
			t.Fatalf("drifted foreign key error = %v", err)
		}
	})
	t.Run("wrong column order", func(t *testing.T) {
		actual := completeNewsSchemaForeignKeys()
		state := actual["fk_news_availability_decision_evidence"]
		state.columns = []string{"selected_evidence_id", "version_id"}
		actual["fk_news_availability_decision_evidence"] = state
		err := validateNewsSchemaForeignKeys(actual)
		if err == nil || !strings.Contains(err.Error(), "fk_news_availability_decision_evidence") {
			t.Fatalf("wrong foreign key column order error = %v", err)
		}
	})
}

func TestValidateNewsSchemaForeignKeysAcceptsMigrationBaseline(t *testing.T) {
	if err := validateNewsSchemaForeignKeys(completeNewsSchemaForeignKeys()); err != nil {
		t.Fatalf("complete News foreign keys rejected: %v", err)
	}
}
