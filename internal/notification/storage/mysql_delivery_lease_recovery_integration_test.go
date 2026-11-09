package storage

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/application"
	"example.com/stock-analysis-flow/internal/notification/domain"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

func TestNotificationMariaDBExpiredDeliveryLeaseRecoveryIntegration(t *testing.T) {
	dsn := os.Getenv("NOTIFICATION_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NOTIFICATION_MARIADB_TEST_DSN to run Notification MariaDB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, err := openNotificationIntegrationDB(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB through Platform Storage: %v", err)
	}
	defer db.Close()
	if err := platformstorage.ApplyMigrations(ctx, db, "notification", Migrations()); err != nil {
		t.Fatalf("apply Notification migrations: %v", err)
	}

	plan := integrationDeliveryPlan()
	plan.DeliveryID += "-lease-recovery"
	plan.EventID += "-lease-recovery"
	plan.SubscriptionID += "-lease-recovery"
	plan.PlanFingerprint, err = plan.CalculateFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	plans, err := NewMySQLDeliveryPlanRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, duplicate, err := plans.CreateOrGet(ctx, plan); err != nil || duplicate {
		t.Fatalf("create recovery plan: duplicate=%v err=%v", duplicate, err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	delivery := domain.DeliveryRecord{
		DeliveryID: plan.DeliveryID, SourceModule: plan.SourceModule, EventID: plan.EventID,
		SubscriptionID: plan.SubscriptionID, SubscriptionVersion: plan.SubscriptionVersion,
		ChannelRef: plan.ChannelRef, ChannelConfigVersion: plan.ChannelConfigVersion,
		TemplateID: plan.TemplateID, TemplateVersion: plan.TemplateVersion,
		RecipientRef: plan.RecipientRef, PayloadFingerprint: plan.PayloadFingerprint,
		PlanFingerprint: plan.PlanFingerprint, DeliveryIdempotencyKey: plan.DeliveryIdempotencyKey,
		Status: domain.DeliveryQueued, Version: 1, AttemptCount: 0, MaxAttempts: 3,
		DeadlineAt: now.Add(10 * time.Minute), CreatedAt: now, UpdatedAt: now,
	}
	repository, err := NewMySQLDeliveryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(ctx, delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	claimAt := now.Add(time.Second)
	leaseExpiry := claimAt.Add(time.Second)
	claim, claimed, err := repository.ClaimDue(ctx, "recovery-worker", claimAt, leaseExpiry)
	if err != nil || !claimed {
		t.Fatalf("claim delivery: claimed=%v err=%v", claimed, err)
	}
	recoveryAt := leaseExpiry.Add(time.Second)
	nextAttemptAt := recoveryAt.Add(time.Second)
	next, event, err := claim.Delivery.Transition(domain.DeliveryTransition{
		To: domain.DeliveryRetryWait, At: recoveryAt,
		NextAttemptAt: &nextAttemptAt, ReasonCode: "lease_expired_before_attempt",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := repository.RecoverExpiredLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, next, event, leaseExpiry.Add(-time.Microsecond)); !errors.Is(err, application.ErrDeliveryLeaseConflict) {
		t.Fatalf("active lease must not be recovered, got %v", err)
	}
	// A conflicting append-only event must roll back the Delivery snapshot and lease release.
	if _, err := db.ExecContext(ctx, `INSERT INTO notification_delivery_state_events (
		delivery_id, version, from_status, to_status, changed_at
	) VALUES (?, ?, ?, ?, ?)`, delivery.DeliveryID, next.Version, string(event.From),
		string(event.To), event.ChangedAt); err != nil {
		t.Fatalf("insert conflicting state event fixture: %v", err)
	}
	if err := repository.RecoverExpiredLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, next, event, recoveryAt); err == nil {
		t.Fatal("duplicate state event must fail recovery transaction")
	}
	stillSending, found, err := repository.Get(ctx, delivery.DeliveryID)
	if err != nil || !found || stillSending.Status != domain.DeliverySending || stillSending.Version != claim.Delivery.Version {
		t.Fatalf("failed recovery must roll back snapshot: found=%v delivery=%+v err=%v", found, stillSending, err)
	}
	var leaseWorker string
	if err := db.QueryRowContext(ctx, "SELECT lease_worker_id FROM notification_deliveries WHERE delivery_id = ?", delivery.DeliveryID).Scan(&leaseWorker); err != nil || leaseWorker != claim.Lease.WorkerID {
		t.Fatalf("failed recovery must retain lease: worker=%q err=%v", leaseWorker, err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM notification_delivery_state_events WHERE delivery_id = ? AND version = ?", delivery.DeliveryID, next.Version); err != nil {
		t.Fatalf("remove conflicting event fixture: %v", err)
	}
	if err := repository.RecoverExpiredLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, next, event, recoveryAt); err != nil {
		t.Fatalf("recover expired lease: %v", err)
	}
	recovered, found, err := repository.Get(ctx, delivery.DeliveryID)
	if err != nil || !found || recovered.Status != domain.DeliveryRetryWait || recovered.Version != next.Version {
		t.Fatalf("load recovered delivery: found=%v delivery=%+v err=%v", found, recovered, err)
	}
	var leaseCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notification_deliveries WHERE delivery_id = ? AND lease_worker_id IS NULL AND lease_expires_at IS NULL", delivery.DeliveryID).Scan(&leaseCount); err != nil || leaseCount != 1 {
		t.Fatalf("recovered lease must be released: count=%d err=%v", leaseCount, err)
	}
	if err := repository.RecoverExpiredLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, next, event, recoveryAt); !errors.Is(err, application.ErrDeliveryVersionConflict) && !errors.Is(err, application.ErrDeliveryLeaseConflict) {
		t.Fatalf("stale recovery must be rejected, got %v", err)
	}
}
