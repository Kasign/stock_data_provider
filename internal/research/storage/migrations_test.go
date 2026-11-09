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
	if len(migrations) != 5 {
		t.Fatalf("migration count = %d, want 5", len(migrations))
	}
	if migrations[0].Version != 1 || migrations[1].Version != 2 || migrations[2].Version != 3 || migrations[3].Version != 4 || migrations[4].Version != 5 {
		t.Fatalf("migration versions = %d,%d,%d,%d,%d, want 1,2,3,4,5", migrations[0].Version, migrations[1].Version, migrations[2].Version, migrations[3].Version, migrations[4].Version)
	}
	if len(migrations[0].Statements) != 5 || len(migrations[1].Statements) != 12 || len(migrations[2].Statements) != 2 || len(migrations[3].Statements) != 1 || len(migrations[4].Statements) != 3 {
		t.Fatalf("migration statement counts = %d,%d,%d,%d,%d, want 5,12,2,1,3", len(migrations[0].Statements), len(migrations[1].Statements), len(migrations[2].Statements), len(migrations[3].Statements), len(migrations[4].Statements))
	}
	joined := strings.Join(migrations[0].Statements, "\n") + "\n" + strings.Join(migrations[1].Statements, "\n") + "\n" + strings.Join(migrations[2].Statements, "\n") + "\n" + strings.Join(migrations[3].Statements, "\n") + "\n" + strings.Join(migrations[4].Statements, "\n")
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
		"delivery_status VARCHAR(16)",
		"lease_until CHAR(30)",
		"lease_token CHAR(32)",
		"attempt_count INT UNSIGNED",
		"idx_research_outbox_delivery",
	} {
		if !strings.Contains(joined, fragment) {
			t.Errorf("Research lifecycle Outbox migration is missing %q", fragment)
		}
	}
}



func TestResearchMigrationPreservesExistingPITAndOutboxValuesInOrder(t *testing.T) {
	migrations := Migrations()
	if len(migrations) != 5 {
		t.Fatalf("migration count = %d, want 5", len(migrations))
	}

	assertOrdered := func(name string, statements []string, fragments ...string) {
		t.Helper()
		joined := strings.Join(statements, "\n")
		last := -1
		for _, fragment := range fragments {
			index := strings.Index(joined, fragment)
			if index < 0 {
				t.Errorf("%s is missing SQL fragment %q", name, fragment)
				return
			}
			if index <= last {
				t.Errorf("%s SQL fragment %q is out of order", name, fragment)
				return
			}
			last = index
		}
	}

	assertOrdered("Context PIT conversion", migrations[1].Statements,
		"ALTER TABLE research_context_versions ADD COLUMN system_as_of_exact",
		"UPDATE research_context_versions SET system_as_of_exact = CONCAT(DATE_FORMAT(system_as_of",
		"ALTER TABLE research_context_versions DROP INDEX idx_research_context_asof",
	)
	assertOrdered("Evidence PIT conversion", migrations[1].Statements,
		"ALTER TABLE research_evidence_records ADD COLUMN available_at_exact",
		"UPDATE research_evidence_records SET available_at_exact = CASE WHEN available_at IS NULL THEN NULL",
		"system_observed_at_exact = CONCAT(DATE_FORMAT(system_observed_at",
		"system_as_of_exact = CONCAT(DATE_FORMAT(system_as_of",
		"ALTER TABLE research_evidence_records DROP INDEX idx_research_evidence_pit",
	)
	assertOrdered("Event PIT conversion", migrations[1].Statements,
		"ALTER TABLE research_event_versions ADD COLUMN system_as_of_exact",
		"UPDATE research_event_versions SET system_as_of_exact = CONCAT(DATE_FORMAT(system_as_of",
		"ALTER TABLE research_event_versions DROP INDEX idx_research_event_status_asof",
	)
	assertOrdered("Claim PIT conversion", migrations[1].Statements,
		"ALTER TABLE research_claim_versions ADD COLUMN system_as_of_exact",
		"UPDATE research_claim_versions SET system_as_of_exact = CONCAT(DATE_FORMAT(system_as_of",
		"ALTER TABLE research_claim_versions DROP INDEX idx_research_claim_status_asof",
	)
	assertOrdered("Outbox delivery state backfill", migrations[4].Statements,
		"ALTER TABLE research_notification_outbox ADD COLUMN delivery_status",
		"available_at CHAR(30)",
		"attempt_count INT UNSIGNED NOT NULL DEFAULT 0",
		"UPDATE research_notification_outbox SET available_at = CONCAT(DATE_FORMAT(created_at",
		"ALTER TABLE research_notification_outbox MODIFY COLUMN available_at",
	)

	if !strings.Contains(migrations[1].Statements[4], "CASE WHEN available_at IS NULL THEN NULL") {
		t.Fatal("Evidence migration must preserve NULL available_at values")
	}
	if !strings.Contains(migrations[4].Statements[1], "updated_at = created_at") {
		t.Fatal("Outbox migration must backfill updated_at from the original created_at")
	}
}
