package storage

import (
	"strings"
	"testing"
)

func TestMigrationsDeclareImmutableVersionAndRequiredUniqueness(t *testing.T) {
	migrations := Migrations()
	if len(migrations) != 5 || migrations[0].Version != 1 || migrations[1].Version != 2 || migrations[2].Version != 3 || migrations[3].Version != 4 || migrations[4].Version != 5 ||
		migrations[0].Description == "" || len(migrations[0].Statements) != 5 ||
		migrations[1].Description == "" || len(migrations[1].Statements) != 1 {
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
	inboxDDL := strings.Join(migrations[1].Statements, "\n")
	for _, required := range []string{
		"CREATE TABLE notification_inbox",
		"PRIMARY KEY (source_module, event_id)",
		"uk_notification_inbox_idempotency",
		"payload_fingerprint",
		"received_at DATETIME(6)",
		"ENGINE=InnoDB",
	} {
		if !strings.Contains(inboxDDL, required) {
			t.Errorf("Inbox migration is missing required schema element %q", required)
		}
	}
	if strings.Contains(inboxDDL, "notification_fanout_work_items") {
		t.Fatal("Inbox migration must not define an unfrozen FanoutWorkItem schema")
	}
	templateDDL := strings.Join(migrations[2].Statements, "\n")
	for _, required := range []string{
		"CREATE TABLE notification_template_versions",
		"PRIMARY KEY (template_id, version)",
		"variable_bindings JSON NOT NULL",
		"digest CHAR(64)",
		"MEDIUMTEXT",
	} {
		if !strings.Contains(templateDDL, required) {
			t.Errorf("Template migration is missing required schema element %q", required)
		}
	}
	channelDDL := strings.Join(migrations[3].Statements, "\n")
	for _, required := range []string{
		"CREATE TABLE notification_channel_config_versions",
		"PRIMARY KEY (channel_ref, config_version)",
		"snapshot_json JSON NOT NULL",
		"digest CHAR(64)",
	} {
		if !strings.Contains(channelDDL, required) {
			t.Errorf("Channel config migration is missing required schema element %q", required)
		}
	}
	if len(migrations[4].Statements) != 1 || !strings.Contains(migrations[4].Statements[0], "DROP INDEX idx_notification_channel_config_kind") {
		t.Fatal("migration v5 must remove the redundant channel config index")
	}
}

func TestMigrationsReturnIndependentStatementSlices(t *testing.T) {
	first := Migrations()
	second := Migrations()
	if len(first) != 5 || len(second) != 5 {
		t.Fatal("migration set must be stable")
	}
	first[0].Statements[0] = "mutated"
	if second[0].Statements[0] == "mutated" {
		t.Fatal("each call must return independent migration statements")
	}
}
