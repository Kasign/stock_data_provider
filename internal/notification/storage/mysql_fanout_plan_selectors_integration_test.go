package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/application"
	"example.com/stock-analysis-flow/internal/notification/domain"
	"example.com/stock-analysis-flow/internal/notification/template"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

func TestNotificationMariaDBFanoutPlanSelectorsIntegration(t *testing.T) {
	dsn := os.Getenv("NOTIFICATION_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NOTIFICATION_MARIADB_TEST_DSN to run Notification MariaDB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := openNotificationIntegrationDB(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := platformstorage.ApplyMigrations(ctx, db, "notification", Migrations()); err != nil {
		t.Fatalf("apply Notification migrations: %v", err)
	}
	channelRepository, err := NewMySQLChannelConfigRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	templateRepository, err := NewMySQLTemplateRepository(db, template.Limits{
		MaxTemplateBytes: 4096, MaxOutputBytes: 4096, MaxPayloadDepth: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	channelSelector, err := NewMySQLFanoutChannelConfigSelector(db, channelRepository)
	if err != nil {
		t.Fatal(err)
	}
	templateSelector, err := NewMySQLFanoutTemplateVersionSelector(db, templateRepository)
	if err != nil {
		t.Fatal(err)
	}

	channelV1 := domain.ChannelConfigSnapshot{
		ChannelRef: "fanout-selector-channel", ConfigVersion: 1, ChannelKind: "email", Enabled: true,
		Capabilities: domain.ChannelCapabilities{
			SupportsIdempotency:    domain.CapabilityUnknown,
			SupportsDeliveryLookup: domain.CapabilityUnknown,
			SupportsRetryAfter:     domain.CapabilityUnknown,
			MaxPayloadBytes:        4096,
		},
	}
	channelV2 := channelV1
	channelV2.ConfigVersion = 2
	if _, _, err := channelRepository.Create(ctx, channelV1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := channelRepository.Create(ctx, channelV2); err != nil {
		t.Fatal(err)
	}
	selectedChannel, err := channelSelector.ResolveForFanout(ctx, domain.Subscription{ChannelRef: channelV1.ChannelRef})
	if err != nil {
		t.Fatal(err)
	}
	if selectedChannel.ConfigVersion != 2 {
		t.Fatalf("expected latest channel config v2, got %+v", selectedChannel)
	}

	definition := template.Definition{
		TemplateID: "fanout-selector-template", Version: 1,
		EventType: "quant.risk_decision.changed", SchemaVersion: "1.0.0",
		Format: template.FormatText, BodyTemplate: "Risk: {{.status}}",
		VariableBindings: map[string]string{"status": "/status"},
	}
	if _, _, err := templateRepository.Create(ctx, definition); err != nil {
		t.Fatal(err)
	}
	definition.Version = 2
	if _, _, err := templateRepository.Create(ctx, definition); err != nil {
		t.Fatal(err)
	}
	selectedTemplate, err := templateSelector.ResolveForFanout(
		ctx, definition.TemplateID, definition.EventType, definition.SchemaVersion,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selectedTemplate != 2 {
		t.Fatalf("expected latest compatible template v2, got %d", selectedTemplate)
	}
	if _, err := templateSelector.ResolveForFanout(
		ctx, definition.TemplateID, definition.EventType, "2.0.0",
	); err == nil {
		t.Fatal("incompatible schema version must not fall back to another template")
	}
}

var _ application.FanoutChannelConfigSelector = (*MySQLFanoutChannelConfigSelector)(nil)
var _ application.FanoutTemplateVersionSelector = (*MySQLFanoutTemplateVersionSelector)(nil)
