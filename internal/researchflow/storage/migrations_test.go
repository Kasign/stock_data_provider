package storage

import (
	"strings"
	"testing"
)

func TestMigrationsAreExplicitAndVersioned(t *testing.T) {
	if MigrationNamespace != "researchflow" {
		t.Fatalf("MigrationNamespace = %q, want researchflow", MigrationNamespace)
	}
	migrations := Migrations()
	if len(migrations) != 2 || migrations[0].Version != 1 || migrations[1].Version != 2 {
		t.Fatalf("expected ordered migrations v1 and v2, got %+v", migrations)
	}
	migration := migrations[0]
	if migration.Description == "" || len(migration.Statements) != 6 {
		t.Fatalf("initial migration metadata/statements are incomplete: %+v", migration)
	}
	outboxMigration := migrations[1]
	if outboxMigration.Description == "" || len(outboxMigration.Statements) != 2 {
		t.Fatalf("Outbox migration metadata/statements are incomplete: %+v", outboxMigration)
	}
	outboxDDL := strings.Join(outboxMigration.Statements, "\n")
	for _, fragment := range []string{
		"lease_owner VARCHAR(128)",
		"lease_expires_at CHAR(30)",
		"lease_generation BIGINT UNSIGNED NOT NULL DEFAULT 0",
		"next_attempt_at CHAR(30)",
		"idx_researchflow_outbox_claim",
	} {
		if !strings.Contains(outboxDDL, fragment) {
			t.Errorf("Outbox migration is missing %q", fragment)
		}
	}

	joined := strings.Join(migration.Statements, "\n")
	for _, table := range []string{
		"researchflow_runs",
		"researchflow_stages",
		"researchflow_attempts",
		"researchflow_run_idempotency",
		"researchflow_result_summaries",
		"researchflow_outbox",
	} {
		if !strings.Contains(joined, "CREATE TABLE "+table) {
			t.Errorf("migration does not create %s", table)
		}
	}
	for _, fragment := range []string{
		"UNIQUE KEY uq_researchflow_stage_run_code (run_ref, stage_code)",
		"UNIQUE KEY uq_researchflow_attempt_stage_no (stage_ref, attempt_no)",
		"UNIQUE KEY uq_researchflow_attempt_stage_fencing (stage_ref, fencing_token)",
		"FOREIGN KEY (stage_ref, run_ref)",
		"PRIMARY KEY (caller_ref, operation, idempotency_key)",
		"request_fingerprint CHAR(64)",
		"UNIQUE KEY uq_researchflow_result_summary_run (run_ref)",
		"UNIQUE KEY uq_researchflow_outbox_run_event (run_ref, event_type)",
		"UNIQUE KEY uq_researchflow_outbox_idempotency (idempotency_key)",
		"fencing_token BIGINT UNSIGNED NOT NULL",
		"version BIGINT UNSIGNED NOT NULL",
		"system_as_of CHAR(30)",
		"deadline_at CHAR(30)",
	} {
		if !strings.Contains(joined, fragment) {
			t.Errorf("migration is missing required persistence invariant %q", fragment)
		}
	}
	if strings.Contains(joined, "CREATE DATABASE") || strings.Contains(joined, "DROP TABLE") {
		t.Fatal("module migration must not create databases or drop existing tables")
	}
}
