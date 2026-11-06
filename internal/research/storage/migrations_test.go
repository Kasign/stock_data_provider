package storage

import (
	"strings"
	"testing"
)

func TestResearchMigrationsAreExplicitAndVersioned(t *testing.T) {
	if MigrationNamespace != "research" {
		t.Fatalf("MigrationNamespace = %q, want research", MigrationNamespace)
	}
	migrations := Migrations()
	if len(migrations) != 1 {
		t.Fatalf("migration count = %d, want 1", len(migrations))
	}
	if migrations[0].Version != 1 {
		t.Fatalf("migration version = %d, want 1", migrations[0].Version)
	}
	if len(migrations[0].Statements) != 5 {
		t.Fatalf("statement count = %d, want 5", len(migrations[0].Statements))
	}
	joined := strings.Join(migrations[0].Statements, "\n")
	for _, table := range []string{
		"research_aggregate_heads",
		"research_context_versions",
		"research_evidence_records",
		"research_event_versions",
		"research_claim_versions",
	} {
		if !strings.Contains(joined, "CREATE TABLE "+table) {
			t.Errorf("migration does not create %s", table)
		}
	}
	if strings.Contains(joined, "research_outbox") {
		t.Fatal("Outbox schema must remain gated by the unresolved INT-CTR-001 mapping")
	}
}
