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
		DeadlineAt: now.Add(10 * time.Minute), CreatedAt: now.Add(-3 * time.Second), UpdatedAt: now.Add(-3 * time.Second),
	}
	repository, err := NewMySQLDeliveryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(ctx, delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	claimAt := now.Add(-2 * time.Second)
	leaseExpiry := now.Add(time.Minute)
	claim, claimed, err := repository.ClaimDue(ctx, "recovery-worker", claimAt, leaseExpiry)
	if err != nil || !claimed {
		t.Fatalf("claim delivery: claimed=%v err=%v", claimed, err)
	}
	attemptRepository, err := NewMySQLDeliveryAttemptRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	var startedClock []byte
	if err := db.QueryRowContext(ctx, `SELECT DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%d %H:%i:%s.%f')`).Scan(&startedClock); err != nil {
		t.Fatal(err)
	}
	startedAt, err := parseDatabaseTime(startedClock)
	if err != nil {
		t.Fatal(err)
	}
	attempt := domain.DeliveryAttempt{
		AttemptID: "attempt-" + fmt.Sprint(time.Now().UnixNano()), DeliveryID: delivery.DeliveryID,
		AttemptNo: 1, WorkerID: claim.Lease.WorkerID, FencingToken: claim.Lease.FencingToken,
		LeaseExpiresAt: claim.Lease.ExpiresAt, StartedAt: startedAt,
	}
	if err := attemptRepository.StartWithLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, attempt); err != nil {
		t.Fatalf("start attempt before lease expiry: %v", err)
	}
	// 测试夹具同时推进 Delivery 与 Attempt 的租约截止时间，模拟 Worker 在发送期间失联。
	expiredAt := now.Add(-time.Second)
	if _, err := db.ExecContext(ctx, "UPDATE notification_deliveries SET lease_expires_at = ? WHERE delivery_id = ?", expiredAt, delivery.DeliveryID); err != nil {
		t.Fatalf("expire delivery lease fixture: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE notification_delivery_attempts SET lease_expires_at = ? WHERE delivery_id = ?", expiredAt, delivery.DeliveryID); err != nil {
		t.Fatalf("expire attempt lease fixture: %v", err)
	}

	expiredClaims, err := repository.ListExpiredLeases(ctx, 10)
	if err != nil {
		t.Fatalf("scan expired leases: %v", err)
	}
	foundExpired := false
	for _, expired := range expiredClaims {
		if expired.Delivery.DeliveryID == delivery.DeliveryID &&
			expired.Lease.FencingToken == claim.Lease.FencingToken &&
			expired.Lease.WorkerID == claim.Lease.WorkerID {
			foundExpired = true
			break
		}
	}
	if !foundExpired {
		t.Fatalf("bounded scan must return the expired lease for %s", delivery.DeliveryID)
	}
	recoveryAt := now
	next, event, err := claim.Delivery.Transition(domain.DeliveryTransition{
		To: domain.DeliveryUnknown, At: recoveryAt,
		ReasonCode: "lease_expired_during_attempt", EvidenceRef: attempt.AttemptID,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := repository.RecoverExpiredLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, next, event, leaseExpiry.Add(-time.Microsecond)); !errors.Is(err, application.ErrDeliveryLeaseConflict) {
		t.Fatalf("active lease must not be recovered, got %v", err)
	}

	// 恢复只能改变状态与租约，不得改写 Delivery 的创建时间、截止时间或重试预算。
	for name, mutate := range map[string]func(*domain.DeliveryRecord){
		"attempt_count": func(value *domain.DeliveryRecord) { value.AttemptCount++ },
		"max_attempts":  func(value *domain.DeliveryRecord) { value.MaxAttempts++ },
		"deadline_at":   func(value *domain.DeliveryRecord) { value.DeadlineAt = value.DeadlineAt.Add(time.Minute) },
		"created_at":    func(value *domain.DeliveryRecord) { value.CreatedAt = value.CreatedAt.Add(-time.Second) },
	} {
		tampered := next
		mutate(&tampered)
		if err := repository.RecoverExpiredLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, tampered, event, recoveryAt); !errors.Is(err, application.ErrDeliveryVersionConflict) {
			t.Fatalf("recovery must reject tampered %s, got %v", name, err)
		}
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
	finishedAt := recoveryAt.Add(time.Second)
	completed, err := attempt.Complete(finishedAt, domain.DeliveryAttemptAccepted, "late-provider-acceptance", "", false, "")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var recoveryErr, completionErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		recoveryErr = repository.RecoverExpiredLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, next, event, recoveryAt)
	}()
	go func() {
		defer wg.Done()
		<-start
		completionErr = attemptRepository.CompleteWithLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, completed)
	}()
	close(start)
	wg.Wait()
	if recoveryErr != nil {
		t.Fatalf("concurrent lease recovery must win after expiry: %v", recoveryErr)
	}
	if !errors.Is(completionErr, application.ErrDeliveryLeaseConflict) {
		t.Fatalf("concurrent stale worker completion must lose to recovery: %v", completionErr)
	}
	recovered, found, err := repository.Get(ctx, delivery.DeliveryID)
	if err != nil || !found || recovered.Status != domain.DeliveryUnknown || recovered.Version != next.Version {
		t.Fatalf("load recovered delivery: found=%v delivery=%+v err=%v", found, recovered, err)
	}
	persistedAttempts, err := attemptRepository.ListByDelivery(ctx, delivery.DeliveryID)
	if err != nil || len(persistedAttempts) != 1 || persistedAttempts[0].FinishedAt != nil {
		t.Fatalf("recovered running Attempt must remain immutable: attempts=%+v err=%v", persistedAttempts, err)
	}

	var leaseCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notification_deliveries WHERE delivery_id = ? AND lease_worker_id IS NULL AND lease_expires_at IS NULL", delivery.DeliveryID).Scan(&leaseCount); err != nil || leaseCount != 1 {
		t.Fatalf("recovered lease must be released: count=%d err=%v", leaseCount, err)
	}
	if err := repository.RecoverExpiredLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, next, event, recoveryAt); !errors.Is(err, application.ErrDeliveryVersionConflict) && !errors.Is(err, application.ErrDeliveryLeaseConflict) {
		t.Fatalf("stale recovery must be rejected, got %v", err)
	}
}
