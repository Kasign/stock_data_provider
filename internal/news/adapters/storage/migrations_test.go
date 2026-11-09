package storage

import (
	"strings"
	"testing"
)

func TestMigrationsDefineNewsOwnedPersistenceSchema(t *testing.T) {
	migrations := Migrations()
	if len(migrations) != 5 {
		t.Fatalf("Migrations() count = %d, want 5", len(migrations))
	}
	for index, migration := range migrations {
		if migration.Version != uint64(index+1) || strings.TrimSpace(migration.Description) == "" || len(migration.Statements) == 0 {
			t.Fatalf("invalid migration metadata at index %d: %#v", index, migration)
		}
	}

	var statements []string
	for _, migration := range migrations {
		statements = append(statements, migration.Statements...)
	}
	joined := strings.Join(statements, "\n")
	for _, table := range []string{
		"news_items",
		"news_identifiers",
		"news_identifier_status_history",
		"news_observations",
		"news_item_versions",
		"news_relation_assertions",
		"news_relation_evidence",
		"news_item_relations",
		"news_authorization_snapshots",
		"news_evidence_snapshots",
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
		"fk_news_identifier_history_credential",
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
	if len(first) != 5 || len(second) != 5 {
		t.Fatalf("migration counts = %d/%d, want 5", len(first), len(second))
	}
	for index := range first {
		if len(first[index].Statements) == 0 || len(second[index].Statements) == 0 {
			t.Fatalf("migration %d statements are empty", index+1)
		}
	}
	first[4].Statements[0] = "mutated"
	if second[4].Statements[0] == "mutated" {
		t.Fatal("migration statements are shared across calls")
	}
}

func TestMigrationsPreserveNewsPointInTimeAccessPaths(t *testing.T) {
	migrations := Migrations()
	if len(migrations) != 5 {
		t.Fatalf("Migrations() count = %d, want 5", len(migrations))
	}

	versionSchema := strings.Join(migrations[0].Statements, "\n")
	for _, fragment := range []string{
		"KEY idx_news_observations_news_time (news_id, observed_at)",
		"KEY idx_news_versions_observed (news_id, observed_at, version_no)",
		"CONSTRAINT fk_news_versions_observation FOREIGN KEY (observation_id) REFERENCES news_observations (observation_id)",
	} {
		if !strings.Contains(versionSchema, fragment) {
			t.Errorf("V1 schema is missing PIT access-path fragment %q", fragment)
		}
	}

	authorizationSchema := strings.Join(migrations[1].Statements, "\n")
	for _, fragment := range []string{
		"PRIMARY KEY (authorization_ref)",
		"decision_fingerprint CHAR(64)",
		"payload_hash CHAR(64)",
	} {
		if !strings.Contains(authorizationSchema, fragment) {
			t.Errorf("V2 schema is missing authorization integrity fragment %q", fragment)
		}
	}

	snapshotSchema := strings.Join(migrations[2].Statements, "\n")
	for _, fragment := range []string{
		"PRIMARY KEY (snapshot_ref)",
		"KEY idx_news_snapshot_caller_expiry (caller_ref, expires_at)",
		"KEY idx_news_snapshot_expiry (expires_at)",
		"payload_json JSON NOT NULL",
		"payload_hash CHAR(64)",
	} {
		if !strings.Contains(snapshotSchema, fragment) {
			t.Errorf("V3 schema is missing snapshot integrity/access fragment %q", fragment)
		}
	}

	identityHistorySchema := strings.Join(migrations[3].Statements, "\n")
	for _, fragment := range []string{
		"KEY idx_news_identifier_history_at",
		"FOREIGN KEY (source_id, identifier_type, identifier_hash)",
		"SELECT source_id, identifier_type, identifier_hash, identifier_status, created_at FROM news_identifiers",
	} {
		if !strings.Contains(identityHistorySchema, fragment) {
			t.Errorf("V4 schema is missing identity history fragment %q", fragment)
		}
	}

	providerTraceSchema := strings.Join(migrations[4].Statements, "\n")
	if !strings.Contains(providerTraceSchema, "ADD COLUMN provider_output_policy_version_id") {
		t.Fatal("V5 schema is missing provider output policy version trace")
	}
}
