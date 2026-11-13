package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/template"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

func TestNotificationMariaDBTemplateRepositoryIntegration(t *testing.T) {
	dsn := os.Getenv("NOTIFICATION_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NOTIFICATION_MARIADB_TEST_DSN to run Notification MariaDB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := openNotificationIntegrationDB(t, ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB through Platform Storage: %v", err)
	}
	defer db.Close()
	if err := platformstorage.ApplyMigrations(ctx, db, "notification", Migrations()); err != nil {
		t.Fatalf("apply Notification migrations: %v", err)
	}
	limits := template.Limits{MaxTemplateBytes: 8192, MaxOutputBytes: 8192, MaxPayloadDepth: 16}
	repository, err := NewMySQLTemplateRepository(db, limits)
	if err != nil {
		t.Fatal(err)
	}
	definition := template.Definition{
		TemplateID: fmt.Sprintf("template-%d", time.Now().UnixNano()), Version: 1,
		EventType: "quant.risk_decision.changed", SchemaVersion: "1.0.0",
		Format:           template.FormatText,
		SubjectTemplate:  "Risk {{.status}}",
		BodyTemplate:     "Decision: {{.message}}",
		VariableBindings: map[string]string{"status": "/status", "message": "/message"},
	}
	digest, duplicate, err := repository.Create(ctx, definition)
	if err != nil || duplicate || digest == "" {
		t.Fatalf("create template: digest=%q duplicate=%v err=%v", digest, duplicate, err)
	}
	repeatedDigest, duplicate, err := repository.Create(ctx, definition)
	if err != nil || !duplicate || repeatedDigest != digest {
		t.Fatalf("identical create must be idempotent: digest=%q duplicate=%v err=%v", repeatedDigest, duplicate, err)
	}
	changed := definition
	changed.BodyTemplate = "Changed: {{.message}}"
	if _, _, err := repository.Create(ctx, changed); !errors.Is(err, template.ErrTemplateImmutable) {
		t.Fatalf("changed same version must be immutable: %v", err)
	}
	next := changed
	next.Version = 2
	if _, duplicate, err := repository.Create(ctx, next); err != nil || duplicate {
		t.Fatalf("new version should be accepted: duplicate=%v err=%v", duplicate, err)
	}
	loaded, loadedDigest, found, err := repository.Get(ctx, definition.TemplateID, 1)
	if err != nil || !found || loadedDigest != digest || loaded.BodyTemplate != definition.BodyTemplate {
		t.Fatalf("load immutable template: found=%v definition=%+v digest=%q err=%v", found, loaded, loadedDigest, err)
	}
	loaded.VariableBindings["message"] = "/tampered"
	reloaded, _, found, err := repository.Get(ctx, definition.TemplateID, 1)
	if err != nil || !found || reloaded.VariableBindings["message"] != "/message" {
		t.Fatalf("returned bindings must not mutate persisted snapshot: %+v err=%v", reloaded, err)
	}
	if _, _, found, err := repository.Get(ctx, definition.TemplateID+"-missing", 1); err != nil || found {
		t.Fatalf("missing template should return found=false: found=%v err=%v", found, err)
	}

	if _, err := db.ExecContext(ctx, `UPDATE notification_template_versions
		SET body_template = ? WHERE template_id = ? AND version = 1`,
		"Tampered: {{.message}}", definition.TemplateID); err != nil {
		t.Fatalf("tamper persisted template fixture: %v", err)
	}
	if _, _, found, err := repository.Get(ctx, definition.TemplateID, 1); err == nil || found {
		t.Fatalf("tampered template snapshot must fail digest validation: found=%v err=%v", found, err)
	}
	if _, _, err := repository.Create(ctx, definition); err == nil {
		t.Fatal("duplicate create must reject a tampered persisted template snapshot")
	}
}
