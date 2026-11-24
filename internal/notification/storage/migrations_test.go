package storage

import (
	"strings"
	"testing"
)

func TestMigrationsDefineCompleteNotificationBaseline(t *testing.T) {
	if MigrationNamespace != "notification" {
		t.Fatalf("MigrationNamespace = %q", MigrationNamespace)
	}
	migrations := Migrations()
	if len(migrations) != 1 || migrations[0].Version != 1 || len(migrations[0].Statements) != 11 {
		t.Fatalf("migration baseline = %#v, want one V1 with ten tables and one seed", migrations)
	}
	schema := strings.Join(migrations[0].Statements, "\n")
	schema = strings.Join(strings.Fields(schema), " ")
	for _, table := range []string{
		"notification_subscriptions", "notification_delivery_plans", "notification_deliveries",
		"notification_delivery_attempts", "notification_delivery_state_events",
		"notification_inbox", "notification_template_versions", "notification_channel_config_versions",
		"notification_subscription_revision", "notification_fanout_work_items",
	} {
		if !strings.Contains(schema, "CREATE TABLE "+table+" (") {
			t.Errorf("baseline is missing table %s", table)
		}
	}
	for _, fragment := range []string{
		"revision_id BIGINT UNSIGNED NOT NULL DEFAULT 0",
		"idx_notification_subscriptions_revision",
		"UNIQUE KEY uk_notification_fanout_event (source_module, event_id)",
		"FOREIGN KEY (source_module, event_id) REFERENCES notification_inbox (source_module, event_id)",
		"INSERT INTO notification_subscription_revision (singleton_id, current_revision) VALUES (1, 0)",
	} {
		normalizedFragment := strings.Join(strings.Fields(fragment), " ")
		if !strings.Contains(schema, normalizedFragment) {
			t.Errorf("baseline is missing final schema fragment %q", fragment)
		}
	}
	if strings.Contains(schema, "idx_notification_channel_config_kind") {
		t.Fatal("redundant channel config index must not be recreated")
	}
	if strings.Contains(strings.ToUpper(schema), "ALTER TABLE") {
		t.Fatal("fresh baseline must not contain incremental ALTER statements")
	}
}
