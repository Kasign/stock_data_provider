package storage

import (
	"strings"
	"testing"
)

func TestMigrationsDefineCompleteAnalysisBaseline(t *testing.T) {
	migrations := Migrations()
	if len(migrations) != 1 || migrations[0].Version != 1 || len(migrations[0].Statements) != 4 {
		t.Fatalf("migration baseline = %#v, want one V1 with four tables", migrations)
	}
	schema := strings.Join(migrations[0].Statements, "\n")
	for _, table := range []string{
		"analysis_runs", "analysis_stage_events", "analysis_snapshots", "analysis_tasks",
	} {
		if !strings.Contains(schema, "CREATE TABLE "+table+" (") {
			t.Errorf("baseline is missing table %s", table)
		}
	}
	for _, fragment := range []string{
		"FOREIGN KEY (run_id) REFERENCES analysis_runs(run_id) ON DELETE CASCADE",
		"UNIQUE KEY uq_analysis_tasks_run (run_id)",
		"UNIQUE KEY uq_analysis_tasks_idempotency (idempotency_key)",
		"idx_tasks_status_created",
	} {
		if !strings.Contains(schema, fragment) {
			t.Errorf("baseline is missing schema fragment %q", fragment)
		}
	}
	if strings.Contains(strings.ToUpper(schema), "IF NOT EXISTS") || strings.Contains(strings.ToUpper(schema), "ALTER TABLE") {
		t.Fatal("fresh baseline must use strict CREATE TABLE statements without compatibility DDL")
	}
}
