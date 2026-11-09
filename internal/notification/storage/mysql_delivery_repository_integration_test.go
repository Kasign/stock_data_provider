package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/application"
	"example.com/stock-analysis-flow/internal/notification/domain"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

func TestNotificationMariaDBDeliveryRepositoryIntegration(t *testing.T) {
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
	plan.PlanFingerprint, err = plan.CalculateFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	planRepository, err := NewMySQLDeliveryPlanRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, duplicate, err := planRepository.CreateOrGet(ctx, plan); err != nil || duplicate {
		t.Fatalf("create delivery plan: duplicate=%v err=%v", duplicate, err)
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
		DeadlineAt: now.Add(30 * time.Minute), CreatedAt: now, UpdatedAt: now,
	}
	repository, err := NewMySQLDeliveryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(ctx, delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}
	mismatchedPlan := delivery
	mismatchedPlan.RecipientRef = "recipient-mismatch"
	if err := repository.Create(ctx, mismatchedPlan); err == nil {
		t.Fatal("delivery snapshot must match its immutable persisted plan")
	}
	if err := repository.Create(ctx, delivery); !errors.Is(err, application.ErrDeliveryAlreadyExists) {
		t.Fatalf("duplicate delivery must conflict, got %v", err)
	}
	loaded, found, err := repository.Get(ctx, delivery.DeliveryID)
	if err != nil || !found || loaded.Status != domain.DeliveryQueued || loaded.Version != 1 {
		t.Fatalf("load created delivery: found=%v delivery=%+v err=%v", found, loaded, err)
	}

	const workers = 16
	var claimed atomic.Int64
	var winner application.DeliveryClaim
	var winnerMu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			claim, ok, claimErr := repository.ClaimDue(ctx, fmt.Sprintf("mariadb-worker-%02d", worker), now.Add(time.Second), now.Add(2*time.Minute))
			if claimErr != nil {
				t.Errorf("claim delivery: %v", claimErr)
				return
			}
			if ok {
				claimed.Add(1)
				winnerMu.Lock()
				winner = claim
				winnerMu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if claimed.Load() != 1 {
		t.Fatalf("expected one database claim winner, got %d", claimed.Load())
	}
	if winner.Delivery.Status != domain.DeliverySending || winner.Delivery.Version != 2 || winner.Lease.FencingToken != 1 {
		t.Fatalf("unexpected winning claim: %+v", winner)
	}

	finishedAt := now.Add(3 * time.Second)
	next, event, err := winner.Delivery.Transition(domain.DeliveryTransition{
		To: domain.DeliverySent, At: finishedAt, ProviderMessageRef: "provider-message-it",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Force the append-only event insert to fail and prove the snapshot update rolls back.
	if _, err := db.ExecContext(ctx, `INSERT INTO notification_delivery_state_events (
		delivery_id, version, from_status, to_status, changed_at
	) VALUES (?, ?, ?, ?, ?)`, delivery.DeliveryID, next.Version, string(domain.DeliverySending),
		string(domain.DeliverySent), finishedAt); err != nil {
		t.Fatalf("insert conflicting event fixture: %v", err)
	}
	if err := repository.UpdateWithLease(ctx, winner.Delivery.Version, winner.Lease.FencingToken, next, event, finishedAt); err == nil {
		t.Fatal("duplicate state event must fail the transaction")
	}
	afterRollback, found, err := repository.Get(ctx, delivery.DeliveryID)
	if err != nil || !found || afterRollback.Status != domain.DeliverySending || afterRollback.Version != winner.Delivery.Version {
		t.Fatalf("failed event append must roll back delivery snapshot: found=%v delivery=%+v err=%v", found, afterRollback, err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM notification_delivery_state_events WHERE delivery_id = ? AND version = ?", delivery.DeliveryID, next.Version); err != nil {
		t.Fatalf("remove conflicting event fixture: %v", err)
	}
	if err := repository.UpdateWithLease(ctx, winner.Delivery.Version, winner.Lease.FencingToken, next, event, finishedAt); err != nil {
		t.Fatalf("complete delivery with active lease: %v", err)
	}
	loaded, found, err = repository.Get(ctx, delivery.DeliveryID)
	if err != nil || !found || loaded.Status != domain.DeliverySent || loaded.Version != 3 {
		t.Fatalf("load completed delivery: found=%v delivery=%+v err=%v", found, loaded, err)
	}
	var eventCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notification_delivery_state_events WHERE delivery_id = ?", delivery.DeliveryID).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 2 {
		t.Fatalf("claim and completion must append two state events, got %d", eventCount)
	}
	if err := repository.UpdateWithLease(ctx, winner.Delivery.Version, winner.Lease.FencingToken, next, event, finishedAt.Add(time.Second)); !errors.Is(err, application.ErrDeliveryLeaseConflict) {
		t.Fatalf("released lease must reject repeated update, got %v", err)
	}
}
