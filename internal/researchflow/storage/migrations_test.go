package storage

import (
	"strings"
	"testing"
)

func TestMigrationsDefineCompleteResearchflowBaseline(t *testing.T) {
	if MigrationNamespace != "researchflow" {
		t.Fatalf("MigrationNamespace = %q", MigrationNamespace)
	}
	migrations := Migrations()
	if len(migrations) != 2 || migrations[0].Version != 1 || migrations[1].Version != 2 {
		t.Fatalf("migration versions = %v, want V1 baseline followed by V2 RunResult increment", migrations)
	}
	statements := migrations[0].Statements
	if len(statements) != 10 {
		t.Fatalf("V1 statement count = %d, want 9 tables plus capacity seed", len(statements))
	}
	schema := strings.Join(statements, "\n")
	for _, table := range []string{
		"researchflow_runs", "researchflow_stages", "researchflow_attempts",
		"researchflow_run_idempotency", "researchflow_result_summaries",
		"researchflow_outbox", "researchflow_workflow_templates",
		"researchflow_capacity_guard", "researchflow_stage_output_resources",
	} {
		if !strings.Contains(schema, "CREATE TABLE "+table+" (") {
			t.Errorf("baseline is missing table %s", table)
		}
	}
	for _, fragment := range []string{
		"lease_owner VARCHAR(128) NULL",
		"lease_expires_at CHAR(30)",
		"lease_generation BIGINT UNSIGNED NOT NULL DEFAULT 0",
		"next_attempt_at CHAR(30)",
		"idx_researchflow_outbox_claim",
		"UNIQUE KEY uq_researchflow_idempotency_run (run_ref)",
		"CHECK (resource_kind IN ('payload', 'quality_summary'))",
		"FOREIGN KEY (stage_ref, run_ref)",
		"INSERT INTO researchflow_capacity_guard (guard_id) VALUES (1)",
	} {
		if !strings.Contains(schema, fragment) {
			t.Errorf("baseline is missing final schema fragment %q", fragment)
		}
	}
	if strings.Contains(schema, "idx_researchflow_idempotency_run") {
		t.Fatal("redundant non-unique Run index must not be recreated")
	}
	if strings.Contains(strings.ToUpper(schema), "ALTER TABLE") {
		t.Fatal("fresh baseline must not contain incremental ALTER statements")
	}
	v2 := strings.Join(migrations[1].Statements, "\n")
	if len(migrations[1].Statements) != 2 {
		t.Fatalf("V2 statement count = %d, want result_ref column and RunResult table", len(migrations[1].Statements))
	}
	for _, fragment := range []string{
		"ALTER TABLE researchflow_runs ADD COLUMN result_ref VARCHAR(255) NULL",
		"CREATE TABLE researchflow_run_results (",
		"UNIQUE KEY uq_researchflow_run_result_run (run_ref)",
		"CONSTRAINT fk_researchflow_run_results_run FOREIGN KEY (run_ref)",
	} {
		if !strings.Contains(v2, fragment) {
			t.Errorf("V2 is missing fragment %q", fragment)
		}
	}
}

func TestMigrationsReturnIndependentStatementSlices(t *testing.T) {
	first, second := Migrations(), Migrations()
	if len(first) != 2 || len(second) != 2 ||
		len(first[0].Statements) != len(second[0].Statements) ||
		len(first[1].Statements) != len(second[1].Statements) {
		t.Fatal("migration shape changed between calls")
	}
	first[0].Statements[0] = "mutated"
	first[1].Statements[0] = "mutated"
	if second[0].Statements[0] == "mutated" || second[1].Statements[0] == "mutated" {
		t.Fatal("migration statements must be independently allocated")
	}
}
