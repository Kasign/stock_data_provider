package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/application"
	"example.com/stock-analysis-flow/internal/notification/domain"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

func TestNotificationMariaDBDeliveryAttemptRepositoryIntegration(t *testing.T) {
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
		DeadlineAt: now.Add(20 * time.Minute), CreatedAt: now.Add(-2 * time.Minute), UpdatedAt: now.Add(-2 * time.Minute),
	}
	deliveryRepository, err := NewMySQLDeliveryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := deliveryRepository.Create(ctx, delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}
	claim, claimed, err := deliveryRepository.ClaimDue(ctx, "attempt-integration-worker", now.Add(-time.Minute), now.Add(10*time.Minute))
	if err != nil || !claimed {
		t.Fatalf("claim delivery: claimed=%v err=%v", claimed, err)
	}

	repository, err := NewMySQLDeliveryAttemptRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	var databaseNow []byte
	if err := db.QueryRowContext(ctx, `SELECT DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%d %H:%i:%s.%f')`).Scan(&databaseNow); err != nil {
		t.Fatal(err)
	}
	startedAt, err := parseDatabaseTime(databaseNow)
	if err != nil {
		t.Fatal(err)
	}
	attempt := domain.DeliveryAttempt{
		AttemptID: "attempt-" + fmt.Sprint(time.Now().UnixNano()), DeliveryID: delivery.DeliveryID,
		AttemptNo: 1, WorkerID: claim.Lease.WorkerID, FencingToken: claim.Lease.FencingToken,
		LeaseExpiresAt: claim.Lease.ExpiresAt, StartedAt: startedAt,
	}

	const contenders = 8
	var successes int
	var successMu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			candidate := attempt
			candidate.AttemptID = fmt.Sprintf("%s-%d", attempt.AttemptID, index)
			err := repository.StartWithLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, candidate)
			if err == nil {
				successMu.Lock()
				successes++
				successMu.Unlock()
				return
			}
			if !errors.Is(err, application.ErrDeliveryAttemptConflict) &&
				!errors.Is(err, application.ErrDeliveryAttemptAlreadyExists) {
				t.Errorf("unexpected concurrent start error: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if successes != 1 {
		t.Fatalf("one lease must start exactly one attempt, got %d", successes)
	}

	loadedDelivery, found, err := deliveryRepository.Get(ctx, delivery.DeliveryID)
	if err != nil || !found || loadedDelivery.AttemptCount != 1 || loadedDelivery.Version != claim.Delivery.Version {
		t.Fatalf("attempt count projection must update without changing delivery version: found=%v delivery=%+v err=%v", found, loadedDelivery, err)
	}
	attempts, err := repository.ListByDelivery(ctx, delivery.DeliveryID)
	if err != nil || len(attempts) != 1 || attempts[0].FinishedAt != nil {
		t.Fatalf("expected one running attempt: attempts=%+v err=%v", attempts, err)
	}
	if _, err := repository.ListByDelivery(ctx, "missing-delivery"); !errors.Is(err, application.ErrDeliveryNotFound) {
		t.Fatalf("missing delivery must be distinguished from an empty attempt history: %v", err)
	}

	var finishClock []byte
	if err := db.QueryRowContext(ctx, `SELECT DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%d %H:%i:%s.%f')`).Scan(&finishClock); err != nil {
		t.Fatal(err)
	}
	finishedAt, err := parseDatabaseTime(finishClock)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := attempts[0].Complete(finishedAt, domain.DeliveryAttemptAccepted, "provider-message-attempt-it", "", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteWithLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, completed); err != nil {
		t.Fatalf("complete attempt with active lease: %v", err)
	}
	if err := repository.CompleteWithLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, completed); !errors.Is(err, application.ErrDeliveryAttemptConflict) {
		t.Fatalf("completed attempt must be immutable: %v", err)
	}
	attempts, err = repository.ListByDelivery(ctx, delivery.DeliveryID)
	if err != nil || len(attempts) != 1 || attempts[0].Outcome == nil ||
		*attempts[0].Outcome != domain.DeliveryAttemptAccepted ||
		attempts[0].ProviderMessageRef == nil || *attempts[0].ProviderMessageRef != "provider-message-attempt-it" {
		t.Fatalf("completed attempt snapshot was not persisted: attempts=%+v err=%v", attempts, err)
	}

	second := attempt
	second.AttemptID += "-second"
	second.AttemptNo = 2
	if err := repository.StartWithLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, second); !errors.Is(err, application.ErrDeliveryAttemptConflict) {
		t.Fatalf("same fencing token cannot append a second attempt: %v", err)
	}
}
