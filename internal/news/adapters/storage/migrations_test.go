package storage

import (
	"strings"
	"testing"
)

func TestMigrationsDefineNewsOwnedPersistenceSchema(t *testing.T) {
	migrations := Migrations()
	if len(migrations) != 1 {
		t.Fatalf("Migrations() count = %d, want 1", len(migrations))
	}
	migration := migrations[0]
	if migration.Version != 1 || strings.TrimSpace(migration.Description) == "" {
		t.Fatalf("invalid migration metadata: %#v", migration)
	}

	joined := strings.Join(migration.Statements, "\n")
	for _, table := range []string{
		"news_items",
		"news_identifiers",
		"news_observations",
		"news_item_versions",
		"news_relation_assertions",
		"news_relation_evidence",
		"news_item_relations",
	} {
		if !strings.Contains(joined, "CREATE TABLE "+table+" ") {
			t.Errorf("migration does not create %s", table)
		}
	}
	for _, constraint := range []string{
		"uk_news_observations_idempotency",
		"uk_news_versions_number",
		"PRIMARY KEY (source_id, identifier_type, identifier_hash)",
		"fk_news_relation_evidence_version",
		"fk_news_relation_evidence_observation",
	} {
		if !strings.Contains(joined, constraint) {
			t.Errorf("migration is missing constraint %q", constraint)
		}
	}
	if strings.Contains(joined, "news_schema_migrations") {
		t.Fatal("News must use the Platform namespace migration ledger")
	}
}

func TestMigrationsReturnIndependentStatementSlices(t *testing.T) {
	first := Migrations()
	second := Migrations()
	if len(first) == 0 || len(first[0].Statements) == 0 || len(second[0].Statements) == 0 {
		t.Fatal("migration statements are empty")
	}
	first[0].Statements[0] = "mutated"
	if second[0].Statements[0] == "mutated" {
		t.Fatal("migration statements are shared across calls")
	}
}
