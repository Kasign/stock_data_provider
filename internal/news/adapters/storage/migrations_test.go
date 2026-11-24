package storage

import (
	"strings"
	"testing"
)

func TestMigrationsDefineCompleteNewsBaseline(t *testing.T) {
	migrations := Migrations()
	if len(migrations) != 1 || migrations[0].Version != 1 || len(migrations[0].Statements) != 14 {
		t.Fatalf("migration baseline = %#v, want one V1 with fourteen tables", migrations)
	}
	schema := strings.Join(migrations[0].Statements, "\n")
	for _, table := range []string{
		"news_items", "news_identifiers", "news_observations", "news_item_versions",
		"news_relation_assertions", "news_relation_evidence", "news_item_relations",
		"news_authorization_snapshots", "news_evidence_snapshots",
		"news_identifier_status_history", "news_policy_versions", "news_current_policy_versions",
		"news_version_availability_evidence", "news_version_availability_decisions",
	} {
		if !strings.Contains(schema, "CREATE TABLE "+table+" (") {
			t.Errorf("baseline is missing table %s", table)
		}
	}
	for _, fragment := range []string{
		"endpoint_id VARCHAR(128)",
		"provider_output_policy_ref VARCHAR(128)",
		"provider_output_policy_version_id VARCHAR(128)",
		"retention_until DATETIME(6)",
		"payload_purged_at DATETIME(6)",
		"uk_news_observations_id_news",
		"uk_news_versions_id_news",
		"uk_news_relation_assertions_supersedes",
		"uk_news_item_relations_supersedes",
		"FOREIGN KEY (version_id, news_id) REFERENCES news_item_versions (version_id, news_id)",
		"FOREIGN KEY (version_id, selected_evidence_id)",
		"FOREIGN KEY (version_id, supersedes_decision_id)",
	} {
		if !strings.Contains(schema, fragment) {
			t.Errorf("baseline is missing final schema fragment %q", fragment)
		}
	}
	if strings.Contains(schema, "idx_news_relations_supersedes") || strings.Contains(schema, "idx_news_item_relations_parent") {
		t.Fatal("redundant non-unique supersedes indexes must not be recreated")
	}
	if strings.Contains(strings.ToUpper(schema), "ALTER TABLE") || strings.Contains(strings.ToUpper(schema), "INSERT INTO NEWS_IDENTIFIER_STATUS_HISTORY") {
		t.Fatal("fresh baseline must not contain incremental ALTER or historical backfill")
	}
}
