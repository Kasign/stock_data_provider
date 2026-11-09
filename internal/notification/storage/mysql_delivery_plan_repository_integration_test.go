package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"example.com/stock-analysis-flow/internal/notification/domain"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

func TestNotificationMariaDBDeliveryPlanRepositoryIntegration(t *testing.T) {
	dsn := os.Getenv("NOTIFICATION_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NOTIFICATION_MARIADB_TEST_DSN to run Notification MariaDB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("MariaDB ping failed: %v", err)
	}
	if err := platformstorage.ApplyMigrations(ctx, db, "notification", Migrations()); err != nil {
		t.Fatalf("apply Notification migrations: %v", err)
	}
	repository, err := NewMySQLDeliveryPlanRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	plan := integrationDeliveryPlan()
	created, duplicate, err := repository.CreateOrGet(ctx, plan)
	if err != nil || duplicate || created.DeliveryID != plan.DeliveryID {
		t.Fatalf("first create: duplicate=%v plan=%+v err=%v", duplicate, created, err)
	}
	replayed, duplicate, err := repository.CreateOrGet(ctx, plan)
	if err != nil || !duplicate || replayed.DeliveryID != plan.DeliveryID {
		t.Fatalf("same plan replay: duplicate=%v plan=%+v err=%v", duplicate, replayed, err)
	}

	conflict := plan
	conflict.AuthorizationPolicyVersions = []domain.AuthorizationPolicyVersion{
		{PolicyType: "provider_output", PolicyRef: "provider-it", PolicyVersion: "v2"},
	}
	conflict.PlanFingerprint, err = conflict.CalculateFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.CreateOrGet(ctx, conflict); !errors.Is(err, domain.ErrDeliveryPlanConflict) {
		t.Fatalf("changed immutable plan must conflict, got %v", err)
	}

	// A different business tuple with a reused DeliveryID must not be mistaken
	// for an idempotent replay of the original plan.
	idCollision := plan
	idCollision.EventID = plan.EventID + "-other"
	idCollision.PlanFingerprint, err = idCollision.CalculateFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.CreateOrGet(ctx, idCollision); err == nil || errors.Is(err, domain.ErrDeliveryPlanConflict) {
		t.Fatalf("delivery_id collision must remain a storage conflict, got %v", err)
	}
}

func integrationDeliveryPlan() domain.DeliveryPlan {
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	plan := domain.DeliveryPlan{
		SourceModule: "researchflow", EventID: "event-" + suffix,
		SubscriptionID: "subscription-" + suffix, SubscriptionVersion: 1,
		ChannelRef: "channel-" + suffix, ChannelConfigVersion: 1,
		TemplateID: "template-" + suffix, TemplateVersion: 1,
		RecipientRef: "recipient-" + suffix,
		PayloadFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		AuthorizationPolicyVersions: []domain.AuthorizationPolicyVersion{
			{PolicyType: "provider_output", PolicyRef: "provider-it", PolicyVersion: "v1"},
		},
		AuthorizationRefs:      []string{"authorization-" + suffix},
		DeliveryID:              "delivery-" + suffix,
		DeliveryIdempotencyKey: "idempotency-" + suffix,
	}
	plan.PlanFingerprint, _ = plan.CalculateFingerprint()
	return plan
}
