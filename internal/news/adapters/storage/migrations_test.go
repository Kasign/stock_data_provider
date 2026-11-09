package storage

import (
	"strings"
	"testing"
)

func TestMigrationsDefineNewsOwnedPersistenceSchema(t *testing.T) {
	migrations := Migrations()
	if len(migrations) != 4 {
		t.Fatalf("Migrations() count = %d, want 4", len(migrations))
	}
	if migrations[0].Version != 1 || migrations[1].Version != 2 || migrations[2].Version != 3 ||
		strings.TrimSpace(migrations[0].Description) == "" ||
		strings.TrimSpace(migrations[1].Description) == "" ||
		strings.TrimSpace(migrations[2].Description) == "" {
		t.Fatalf("invalid migration metadata: %#v", migrations)
	}

	joinedStatements := append([]string{}, migrations[0].Statements...)
	for _, migration := range migrations[1:] {
		joinedStatements = append(joinedStatements, migration.Statements...)
	}
	joined := strings.Join(joinedStatements, "\n")
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
	} {
		if !strings.Contains(joined, constraint) {
			t.Errorf("migration is missing constraint %q", constraint)
		}
	}
	identityHistorySchema := strings.Join(migrations[3].Statements, "\n")
	for _, fragment := range []string{"KEY idx_news_identifier_history_at", "SELECT source_id, identifier_type, identifier_hash, identifier_status, created_at FROM news_identifiers"} {
		if !strings.Contains(identityHistorySchema, fragment) {
			t.Errorf("V4 schema is missing identity history fragment %q", fragment)
		}
	}
	if strings.Contains(joined, "news_schema_migrations") {
		t.Fatal("News must use the Platform namespace migration ledger")
	}
}

func TestMigrationsReturnIndependentStatementSlices(t *testing.T) {
	first := Migrations()
	second := Migrations()
	if len(first) == 0 || len(first[0].Statements) == 0 || len(second[0].Statements) == 0 || len(first[2].Statements) == 0 || len(second[2].Statements) == 0 || len(first[3].Statements) == 0 || len(second[3].Statements) == 0 {
		t.Fatal("migration statements are empty")
	}
	first[3].Statements[0] = "mutated"
	if second[3].Statements[0] == "mutated" {
		t.Fatal("migration statements are shared across calls")
	}
}


func TestMigrationsPreserveEvidencePITAndSnapshotAccessPaths(t *testing.T) {
	migrations := Migrations()
	if len(migrations) != 3 {
		t.Fatalf("Migrations() count = %d, want 3", len(migrations))
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
		"FROM news_identifiers",
	} {
		if !strings.Contains(identityHistorySchema, fragment) {
			t.Errorf("V4 schema is missing identity history fragment %q", fragment)
		}
	}
}
