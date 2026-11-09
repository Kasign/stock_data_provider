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
	if len(migrations) != 4 {
		t.Fatalf("migration count = %d, want 5", len(migrations))
	}
	if migrations[0].Version != 1 || migrations[1].Version != 2 || migrations[2].Version != 3 || migrations[3].Version != 4 {
		t.Fatalf("migration versions = %d,%d,%d,%d, want 1,2,3,4", migrations[0].Version, migrations[1].Version, migrations[2].Version, migrations[3].Version)
	}
	if len(migrations[0].Statements) != 5 || len(migrations[1].Statements) != 12 || len(migrations[2].Statements) != 2 || len(migrations[3].Statements) != 1 {
		t.Fatalf("migration statement counts = %d,%d,%d,%d, want 5,12,2,1", len(migrations[0].Statements), len(migrations[1].Statements), len(migrations[2].Statements), len(migrations[3].Statements))
	}
	joined := strings.Join(migrations[0].Statements, "\n") + "\n" + strings.Join(migrations[1].Statements, "\n") + "\n" + strings.Join(migrations[2].Statements, "\n") + "\n" + strings.Join(migrations[3].Statements, "\n")
	for _, table := range []string{
		"research_aggregate_heads",
		"research_context_versions",
		"research_evidence_records",
		"research_event_versions",
		"research_claim_versions",
		"research_notification_outbox",
	} {
		if !strings.Contains(joined, "CREATE TABLE "+table) {
			t.Errorf("migration does not create %s", table)
		}
	}
	for _, fragment := range []string{
		"system_as_of_exact CHAR(30)",
		"system_observed_at_exact CHAR(30)",
		"available_at_exact CHAR(30)",
		"DATE_FORMAT(system_as_of, '%Y-%m-%dT%H:%i:%s.%f')",
	} {
		if !strings.Contains(joined, fragment) {
			t.Errorf("precision-preserving migration is missing %q", fragment)
		}
	}
	for _, fragment := range []string{
		"MODIFY COLUMN event_version BIGINT UNSIGNED NOT NULL",
		"MODIFY COLUMN claim_version BIGINT UNSIGNED NOT NULL",
	} {
		if !strings.Contains(joined, fragment) {
			t.Errorf("version range migration is missing %q", fragment)
		}
	}
	for _, fragment := range []string{
		"UNIQUE KEY uq_research_outbox_idempotency (source_module, idempotency_key)",
		"payload_fingerprint CHAR(64)",
		"correlation_id VARCHAR(128)",
	} {
		if !strings.Contains(joined, fragment) {
			t.Errorf("Research lifecycle Outbox migration is missing %q", fragment)
		}
	}
}
