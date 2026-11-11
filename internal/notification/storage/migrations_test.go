package storage

import (
	"strings"
	"testing"
)

func TestMigrationsDeclareImmutableVersionAndRequiredUniqueness(t *testing.T) {
	migrations := Migrations()
	if len(migrations) != 1 || migrations[0].Version != 1 ||
		migrations[0].Description == "" || len(migrations[0].Statements) != 5 {
		t.Fatalf("unexpected notification migration set: %+v", migrations)
	}

	joined := strings.Join(migrations[0].Statements, "\n")
	for _, required := range []string{
		"CREATE TABLE notification_subscriptions",
		"PRIMARY KEY (subscription_id, version)",
		"CREATE TABLE notification_delivery_plans",
		"uk_notification_delivery_plan_identity",
		"CREATE TABLE notification_deliveries",
		"uk_notification_delivery_identity",
		"uk_notification_delivery_idempotency",
		"lease_worker_id",
		"lease_expires_at",
		"fencing_token BIGINT UNSIGNED NOT NULL DEFAULT 0",
		"idx_notification_deliveries_lease",
		"CREATE TABLE notification_delivery_attempts",
		"uk_notification_attempt_number",
		"uk_notification_attempt_fencing_token",
		"CREATE TABLE notification_delivery_state_events",
		"PRIMARY KEY (delivery_id, version)",
		"DATETIME(6)",
		"ENGINE=InnoDB",
	} {
		if !strings.Contains(joined, required) {
			t.Errorf("migration is missing required schema element %q", required)
		}
	}
	for _, forbidden := range []string{
		"notification_inbox",
		"notification_fanout_work_items",
	} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("migration must not invent unfrozen fan-out contract table %q", forbidden)
		}
	}
}

func TestMigrationsReturnIndependentStatementSlices(t *testing.T) {
	first := Migrations()
	second := Migrations()
	if len(first) != 1 || len(second) != 1 {
		t.Fatal("migration set must be stable")
	}
	first[0].Statements[0] = "mutated"
	if second[0].Statements[0] == "mutated" {
		t.Fatal("each call must return independent migration statements")
	}
}
