package storage

import (
	"strings"
	"testing"
)

func TestMigrationsDefineFinalNewsBaseline(t *testing.T) {
	migrations := Migrations()
	if len(migrations) != 1 || migrations[0].Version != 1 || len(migrations[0].Statements) != 10 {
		t.Fatalf("migration baseline = %#v, want one V1 with ten tables", migrations)
	}
	schema := strings.Join(migrations[0].Statements, "\n")
	for _, table := range []string{
		"news_items", "news_identifiers", "news_observations", "news_item_versions",
		"news_authorization_snapshots", "news_evidence_snapshots", "news_policy_versions",
		"news_current_policy_versions", "news_version_availability_evidence",
		"news_version_availability_decisions",
	} {
		if !strings.Contains(schema, "CREATE TABLE "+table+" (") {
			t.Errorf("baseline is missing table %s", table)
		}
	}
	for _, fragment := range []string{
		"canonical_url MEDIUMTEXT",
		"canonical_url_hash CHAR(64)",
		"uk_news_items_canonical_url_hash",
		"content_fingerprint CHAR(64)",
		"PRIMARY KEY (source_id, identifier_hash)",
		"uk_news_observations_idempotency",
		"uk_news_versions_number",
		"FOREIGN KEY (version_id, news_id) REFERENCES news_item_versions (version_id, news_id)",
		"FOREIGN KEY (version_id, selected_evidence_id)",
		"FOREIGN KEY (version_id, supersedes_decision_id)",
	} {
		if !strings.Contains(schema, fragment) {
			t.Errorf("baseline is missing final schema fragment %q", fragment)
		}
	}
	for _, obsolete := range []string{
		"news_relation_assertions", "news_relation_evidence",
		"news_item_relations", "news_identifier_status_history",
		"origin_source_id", "identifier_type", "identifier_status",
	} {
		if strings.Contains(schema, obsolete) {
			t.Errorf("final baseline must not contain obsolete schema element %q", obsolete)
		}
	}
	if strings.Contains(strings.ToUpper(schema), "ALTER TABLE") {
		t.Fatal("fresh baseline must not contain incremental compatibility DDL")
	}
}
