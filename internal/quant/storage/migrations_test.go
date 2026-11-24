package storage

import (
	"strings"
	"testing"
)

func TestMigrationsDefineCompleteQuantBaseline(t *testing.T) {
	if MigrationNamespace != "quant" {
		t.Fatalf("MigrationNamespace = %q", MigrationNamespace)
	}
	migrations := Migrations()
	if len(migrations) != 1 || migrations[0].Version != 1 || len(migrations[0].Statements) != 6 {
		t.Fatalf("migration baseline = %#v, want one V1 with five tables and one seed", migrations)
	}
	schema := strings.Join(migrations[0].Statements, "\n")
	for _, table := range []string{
		"quant_results", "quant_access_audit", "quant_result_access_policy_versions",
		"quant_result_access_policy_head", "quant_policy_admin_audit",
	} {
		if !strings.Contains(schema, "CREATE TABLE "+table+" (") {
			t.Errorf("baseline is missing table %s", table)
		}
	}
	for _, fragment := range []string{
		"run_ref VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL",
		"idx_quant_access_audit_run_id",
		"content_level VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'full'",
		"authorization_snapshot_json LONGTEXT",
		"policy_snapshot_ref VARCHAR(255)",
		"policy_snapshot_json LONGTEXT",
		"decision_fingerprint VARCHAR(128)",
		"CHECK (enabled IN (0, 1))",
		"CHECK (action IN ('publish', 'revoke'))",
		"VALUES ('quant_result_access', 'quant-result-access', 0, '1970-01-01T00:00:00.000000000Z')",
	} {
		if !strings.Contains(schema, fragment) {
			t.Errorf("baseline is missing final schema fragment %q", fragment)
		}
	}
	if strings.Contains(strings.ToUpper(schema), "ALTER TABLE") {
		t.Fatal("fresh baseline must not contain incremental ALTER statements")
	}
}
