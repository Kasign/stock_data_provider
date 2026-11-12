package storage

import (
	"strings"
	"testing"
)

func TestResearchMigrationCreatesOnlyFinalInitialSchema(t *testing.T) {
	if MigrationNamespace != "research" {
		t.Fatalf("MigrationNamespace = %q", MigrationNamespace)
	}
	migrations := Migrations()
	if len(migrations) != 1 || migrations[0].Version != 1 {
		t.Fatalf("initial migration set = %#v, want one version-1 migration", migrations)
	}
	if len(migrations[0].Statements) != 6 {
		t.Fatalf("initial migration statement count = %d, want six final tables", len(migrations[0].Statements))
	}
	joined := strings.Join(migrations[0].Statements, "\n")
	for _, table := range []string{"research_aggregate_heads", "research_context_versions", "research_evidence_records", "research_event_versions", "research_claim_versions", "research_notification_outbox"} {
		if !strings.Contains(joined, "CREATE TABLE "+table) {
			t.Errorf("initial schema does not create %s", table)
		}
	}
	for _, fragment := range []string{
		"system_as_of CHAR(30)", "system_observed_at CHAR(30)", "available_at CHAR(30)",
		"event_version BIGINT UNSIGNED NOT NULL", "claim_version BIGINT UNSIGNED NOT NULL",
		"UNIQUE KEY uq_research_outbox_idempotency", "delivery_status VARCHAR(16)",
		"lease_until CHAR(30)", "lease_token CHAR(32)", "attempt_count INT UNSIGNED",
		"idx_research_outbox_delivery",
	} {
		if !strings.Contains(joined, fragment) {
			t.Errorf("final initial schema missing %q", fragment)
		}
	}
	for _, fragment := range []string{"ALTER TABLE", "UPDATE research_", "_exact CHAR(30)", "research_event_versions_v2", "research_claim_versions_v2"} {
		if strings.Contains(joined, fragment) {
			t.Errorf("initial schema contains unnecessary compatibility SQL %q", fragment)
		}
	}
}
