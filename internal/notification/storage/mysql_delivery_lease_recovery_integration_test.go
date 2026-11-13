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

	db, err := openNotificationIntegrationDB(t, ctx, dsn)
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
	leaseExpiry := now.Add(500 * time.Millisecond)
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
	recoverySnapshot, found, err := repository.Get(ctx, delivery.DeliveryID)
	if err != nil || !found || recoverySnapshot.AttemptCount != 1 {
		t.Fatalf("load Delivery after Attempt start: found=%v delivery=%+v err=%v", found, recoverySnapshot, err)
	}
	activeNext, activeEvent, err := recoverySnapshot.Transition(domain.DeliveryTransition{
		To: domain.DeliveryUnknown, At: now,
		ReasonCode: "lease_expired_during_attempt", EvidenceRef: attempt.AttemptID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.RecoverExpiredLease(ctx, recoverySnapshot.Version, claim.Lease.FencingToken, activeNext, activeEvent, now); !errors.Is(err, application.ErrDeliveryLeaseConflict) {
		t.Fatalf("active lease must not be recovered, got %v", err)
	}

	// Attempt 的租约必须晚于 StartedAt；将其推进到 StartedAt 之后再等待自然过期。
	expiredAt := startedAt.Add(100 * time.Millisecond)
	if _, err := db.ExecContext(ctx, "UPDATE notification_deliveries SET lease_expires_at = ? WHERE delivery_id = ?", expiredAt, delivery.DeliveryID); err != nil {
		t.Fatalf("expire delivery lease fixture: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE notification_delivery_attempts SET lease_expires_at = ? WHERE delivery_id = ?", expiredAt, delivery.DeliveryID); err != nil {
		t.Fatalf("expire attempt lease fixture: %v", err)
	}
	wait := time.Until(expiredAt.Add(20 * time.Millisecond))
	if wait > 0 {
		time.Sleep(wait)
	}
	var recoveryClock []byte
	if err := db.QueryRowContext(ctx, "SELECT DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%d %H:%i:%s.%f')").Scan(&recoveryClock); err != nil {
		t.Fatal(err)
	}
	recoveryAt, err := parseDatabaseTime(recoveryClock)
	if err != nil {
		t.Fatal(err)
	}

	expiredClaims, err := repository.ListExpiredLeases(ctx, 10)
	if err != nil {
		t.Fatalf("scan expired leases: %v", err)
	}
	foundExpired := false
	var recoveryBase domain.DeliveryRecord
	for _, expired := range expiredClaims {
		if expired.Delivery.DeliveryID == delivery.DeliveryID &&
			expired.Lease.FencingToken == claim.Lease.FencingToken &&
			expired.Lease.WorkerID == claim.Lease.WorkerID {
			foundExpired = true
			recoveryBase = expired.Delivery
			break
		}
	}
	if !foundExpired {
		t.Fatalf("bounded scan must return the expired lease for %s", delivery.DeliveryID)
	}
	next, event, err := recoveryBase.Transition(domain.DeliveryTransition{
		To: domain.DeliveryUnknown, At: recoveryAt,
		ReasonCode: "lease_expired_during_attempt", EvidenceRef: attempt.AttemptID,
	})
	if err != nil {
		t.Fatal(err)
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
		if err := repository.RecoverExpiredLease(ctx, recoveryBase.Version, claim.Lease.FencingToken, tampered, event, recoveryAt); !errors.Is(err, application.ErrDeliveryVersionConflict) {
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
	if err := repository.RecoverExpiredLease(ctx, recoveryBase.Version, claim.Lease.FencingToken, next, event, recoveryAt); err == nil {
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
	finishedAt := leaseExpiry.Add(-time.Microsecond)
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
		recoveryErr = repository.RecoverExpiredLease(ctx, recoveryBase.Version, claim.Lease.FencingToken, next, event, recoveryAt)
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
	if err := repository.RecoverExpiredLease(ctx, recoveryBase.Version, claim.Lease.FencingToken, next, event, recoveryAt); !errors.Is(err, application.ErrDeliveryVersionConflict) && !errors.Is(err, application.ErrDeliveryLeaseConflict) {
		t.Fatalf("stale recovery must be rejected, got %v", err)
	}
}

func TestNotificationMariaDBRecoveryRetryReclaimsWithNewFencingTokenIntegration(t *testing.T) {
	dsn := os.Getenv("NOTIFICATION_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NOTIFICATION_MARIADB_TEST_DSN to run Notification MariaDB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := openNotificationIntegrationDB(t, ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer db.Close()
	if err := platformstorage.ApplyMigrations(ctx, db, "notification", Migrations()); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	plan := integrationDeliveryPlan()
	plan.DeliveryID += "-recovery-reclaim"
	plan.EventID += "-recovery-reclaim"
	plan.SubscriptionID += "-recovery-reclaim"
	plan.PlanFingerprint, err = plan.CalculateFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	plans, err := NewMySQLDeliveryPlanRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, duplicate, err := plans.CreateOrGet(ctx, plan); err != nil || duplicate {
		t.Fatalf("create plan: duplicate=%v err=%v", duplicate, err)
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
	deliveries, err := NewMySQLDeliveryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := deliveries.Create(ctx, delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}
	oldClaim, claimed, err := deliveries.ClaimDue(ctx, "old-worker", now, now.Add(time.Minute))
	if err != nil || !claimed {
		t.Fatalf("first claim: claimed=%v err=%v", claimed, err)
	}
	attempts, err := NewMySQLDeliveryAttemptRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	var clock []byte
	if err := db.QueryRowContext(ctx, "SELECT CAST(UTC_TIMESTAMP(6) AS CHAR)").Scan(&clock); err != nil {
		t.Fatal(err)
	}
	startedAt, err := parseDatabaseTime(clock)
	if err != nil {
		t.Fatal(err)
	}
	oldAttempt := domain.DeliveryAttempt{
		AttemptID: "attempt-" + fmt.Sprint(time.Now().UnixNano()), DeliveryID: delivery.DeliveryID,
		AttemptNo: 1, WorkerID: oldClaim.Lease.WorkerID, FencingToken: oldClaim.Lease.FencingToken,
		LeaseExpiresAt: oldClaim.Lease.ExpiresAt, StartedAt: startedAt,
	}
	if err := attempts.StartWithLease(ctx, oldClaim.Delivery.Version, oldClaim.Lease.FencingToken, oldAttempt); err != nil {
		t.Fatalf("start old attempt: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT CAST(UTC_TIMESTAMP(6) AS CHAR)").Scan(&clock); err != nil {
		t.Fatal(err)
	}
	finishedAt, err := parseDatabaseTime(clock)
	if err != nil {
		t.Fatal(err)
	}
	oldCompleted, err := oldAttempt.Complete(finishedAt, domain.DeliveryAttemptRejected, "", "temporary_provider_error", true, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := attempts.CompleteWithLease(ctx, oldClaim.Delivery.Version, oldClaim.Lease.FencingToken, oldCompleted); err != nil {
		t.Fatalf("complete retryable rejection: %v", err)
	}
	// 缩短租约，但仍保证租约截止晚于 Attempt 完成时间。
	expiredAt := finishedAt.Add(700 * time.Millisecond)
	if _, err := db.ExecContext(ctx, "UPDATE notification_deliveries SET lease_expires_at = ? WHERE delivery_id = ?", expiredAt, delivery.DeliveryID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE notification_delivery_attempts SET lease_expires_at = ? WHERE delivery_id = ?", expiredAt, delivery.DeliveryID); err != nil {
		t.Fatal(err)
	}
	wait := time.Until(expiredAt.Add(20 * time.Millisecond))
	if wait > 0 {
		time.Sleep(wait)
	}
	recoveryNow := time.Now().UTC().Truncate(time.Microsecond)
	recoverySnapshot, found, err := deliveries.Get(ctx, delivery.DeliveryID)
	if err != nil || !found || recoverySnapshot.AttemptCount != 1 {
		t.Fatalf("load current Delivery projection before recovery: found=%v delivery=%+v err=%v", found, recoverySnapshot, err)
	}
	retryAt := recoveryNow.Add(100 * time.Millisecond)
	next, event, err := recoverySnapshot.Transition(domain.DeliveryTransition{
		To: domain.DeliveryRetryWait, At: recoveryNow, NextAttemptAt: &retryAt,
		ReasonCode: "retryable_rejection_recovered", ErrorCode: "temporary_provider_error",
		EvidenceRef: oldAttempt.AttemptID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := deliveries.RecoverExpiredLease(ctx, oldClaim.Delivery.Version, oldClaim.Lease.FencingToken, next, event, recoveryNow); err != nil {
		t.Fatalf("recover retryable lease: %v", err)
	}
	retryDelay := time.Until(retryAt.Add(10 * time.Millisecond))
	if retryDelay > 0 {
		time.Sleep(retryDelay)
	}
	if err := db.QueryRowContext(ctx, "SELECT CAST(UTC_TIMESTAMP(6) AS CHAR)").Scan(&clock); err != nil {
		t.Fatal(err)
	}
	newNow, err := parseDatabaseTime(clock)
	if err != nil {
		t.Fatal(err)
	}
	newClaim, claimed, err := deliveries.ClaimDue(ctx, "new-worker", newNow, newNow.Add(time.Minute))
	if err != nil || !claimed {
		t.Fatalf("reclaim after recovery: claimed=%v err=%v", claimed, err)
	}
	if newClaim.Delivery.Status != domain.DeliverySending || newClaim.Delivery.AttemptCount != 1 ||
		newClaim.Lease.FencingToken <= oldClaim.Lease.FencingToken {
		t.Fatalf("reclaim must preserve attempt count and advance fencing token: old=%+v new=%+v", oldClaim, newClaim)
	}
	if err := db.QueryRowContext(ctx, "SELECT CAST(UTC_TIMESTAMP(6) AS CHAR)").Scan(&clock); err != nil {
		t.Fatal(err)
	}
	newStartedAt, err := parseDatabaseTime(clock)
	if err != nil {
		t.Fatal(err)
	}
	newAttempt := domain.DeliveryAttempt{
		AttemptID: oldAttempt.AttemptID + "-retry", DeliveryID: delivery.DeliveryID,
		AttemptNo: 2, WorkerID: newClaim.Lease.WorkerID, FencingToken: newClaim.Lease.FencingToken,
		LeaseExpiresAt: newClaim.Lease.ExpiresAt, StartedAt: newStartedAt,
	}
	if err := attempts.StartWithLease(ctx, newClaim.Delivery.Version, newClaim.Lease.FencingToken, newAttempt); err != nil {
		t.Fatalf("new worker starts second attempt: %v", err)
	}
	if err := attempts.CompleteWithLease(ctx, newClaim.Delivery.Version, oldClaim.Lease.FencingToken, oldCompleted); !errors.Is(err, application.ErrDeliveryLeaseConflict) {
		t.Fatalf("old worker completion must be fenced out: %v", err)
	}
	staleStart := newAttempt
	staleStart.AttemptID += "-stale-old-worker"
	staleStart.WorkerID = oldClaim.Lease.WorkerID
	staleStart.FencingToken = oldClaim.Lease.FencingToken
	staleStart.LeaseExpiresAt = oldClaim.Lease.ExpiresAt
	if err := attempts.StartWithLease(ctx, newClaim.Delivery.Version, oldClaim.Lease.FencingToken, staleStart); !errors.Is(err, application.ErrDeliveryLeaseConflict) {
		t.Fatalf("old worker must not start another attempt: %v", err)
	}
	loaded, found, err := deliveries.Get(ctx, delivery.DeliveryID)
	if err != nil || !found || loaded.AttemptCount != 2 || loaded.Status != domain.DeliverySending {
		t.Fatalf("new attempt projection must remain intact: found=%v delivery=%+v err=%v", found, loaded, err)
	}
	history, err := attempts.ListByDelivery(ctx, delivery.DeliveryID)
	if err != nil || len(history) != 2 || history[0].FencingToken != oldClaim.Lease.FencingToken ||
		history[1].FencingToken != newClaim.Lease.FencingToken || history[1].FinishedAt != nil {
		t.Fatalf("attempt history must contain old rejection and new running attempt only: %+v err=%v", history, err)
	}
}

func TestNotificationMariaDBConcurrentRetryWaitClaimsSingleWinnerIntegration(t *testing.T) {
	dsn := os.Getenv("NOTIFICATION_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NOTIFICATION_MARIADB_TEST_DSN to run Notification MariaDB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := openNotificationIntegrationDB(t, ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer db.Close()
	if err := platformstorage.ApplyMigrations(ctx, db, "notification", Migrations()); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	plan := integrationDeliveryPlan()
	plan.DeliveryID += "-claim-race"
	plan.EventID += "-claim-race"
	plan.SubscriptionID += "-claim-race"
	plan.PlanFingerprint, err = plan.CalculateFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	plans, err := NewMySQLDeliveryPlanRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, duplicate, err := plans.CreateOrGet(ctx, plan); err != nil || duplicate {
		t.Fatalf("create plan: duplicate=%v err=%v", duplicate, err)
	}
	var clock []byte
	readDBNow := func() time.Time {
		t.Helper()
		if err := db.QueryRowContext(ctx, "SELECT CAST(UTC_TIMESTAMP(6) AS CHAR)").Scan(&clock); err != nil {
			t.Fatal(err)
		}
		value, err := parseDatabaseTime(clock)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	now := readDBNow()
	delivery := domain.DeliveryRecord{
		DeliveryID: plan.DeliveryID, SourceModule: plan.SourceModule, EventID: plan.EventID,
		SubscriptionID: plan.SubscriptionID, SubscriptionVersion: plan.SubscriptionVersion,
		ChannelRef: plan.ChannelRef, ChannelConfigVersion: plan.ChannelConfigVersion,
		TemplateID: plan.TemplateID, TemplateVersion: plan.TemplateVersion,
		RecipientRef: plan.RecipientRef, PayloadFingerprint: plan.PayloadFingerprint,
		PlanFingerprint: plan.PlanFingerprint, DeliveryIdempotencyKey: plan.DeliveryIdempotencyKey,
		Status: domain.DeliveryQueued, Version: 1, AttemptCount: 0, MaxAttempts: 3,
		DeadlineAt: now.Add(10 * time.Minute), CreatedAt: now.Add(-time.Second), UpdatedAt: now.Add(-time.Second),
	}
	repository, err := NewMySQLDeliveryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(ctx, delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}
	oldClaim, claimed, err := repository.ClaimDue(ctx, "recovery-worker", now, now.Add(time.Minute))
	if err != nil || !claimed {
		t.Fatalf("initial claim: claimed=%v err=%v", claimed, err)
	}
	expiredAt := readDBNow().Add(-time.Second)
	if _, err := db.ExecContext(ctx, "UPDATE notification_deliveries SET lease_expires_at = ? WHERE delivery_id = ?", expiredAt, delivery.DeliveryID); err != nil {
		t.Fatal(err)
	}
	recoveryNow := readDBNow()
	recoverySnapshot, found, err := repository.Get(ctx, delivery.DeliveryID)
	if err != nil || !found {
		t.Fatalf("load recovery snapshot: found=%v err=%v", found, err)
	}
	retryAt := recoveryNow.Add(100 * time.Millisecond)
	retryWait, event, err := recoverySnapshot.Transition(domain.DeliveryTransition{
		To: domain.DeliveryRetryWait, At: recoveryNow, NextAttemptAt: &retryAt,
		ReasonCode: "lease_expired_before_attempt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.RecoverExpiredLease(ctx, recoverySnapshot.Version, oldClaim.Lease.FencingToken, retryWait, event, recoveryNow); err != nil {
		t.Fatalf("recover expired lease into retry_wait: %v", err)
	}
	retryDelay := time.Until(retryAt.Add(10 * time.Millisecond))
	if retryDelay > 0 {
		time.Sleep(retryDelay)
	}
	claimAt := readDBNow()
	type claimResult struct {
		worker string
		claim  application.DeliveryClaim
		ok     bool
		err    error
	}
	start := make(chan struct{})
	results := make(chan claimResult, 2)
	var wg sync.WaitGroup
	for _, worker := range []string{"worker-a", "worker-b"} {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			claim, ok, err := repository.ClaimDue(ctx, worker, claimAt, claimAt.Add(time.Minute))
			results <- claimResult{worker: worker, claim: claim, ok: ok, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	var winners []claimResult
	for result := range results {
		if result.err != nil {
			t.Fatalf("%s claim failed: %v", result.worker, result.err)
		}
		if result.ok {
			winners = append(winners, result)
		}
	}
	if len(winners) != 1 {
		t.Fatalf("exactly one concurrent worker may claim retry_wait Delivery, winners=%+v", winners)
	}
	winner := winners[0]
	if winner.claim.Delivery.Status != domain.DeliverySending ||
		winner.claim.Delivery.Version != retryWait.Version+1 ||
		winner.claim.Lease.FencingToken != oldClaim.Lease.FencingToken+1 ||
		winner.claim.Lease.WorkerID != winner.worker {
		t.Fatalf("winning claim must advance state/version/token exactly once: %+v", winner)
	}
	current, found, err := repository.Get(ctx, delivery.DeliveryID)
	if err != nil || !found || current.Status != domain.DeliverySending ||
		current.Version != winner.claim.Delivery.Version || current.AttemptCount != 0 {
		t.Fatalf("persisted Delivery must reflect one claim only: found=%v delivery=%+v err=%v", found, current, err)
	}
	events, err := repository.ListStateEvents(ctx, delivery.DeliveryID)
	if err != nil || len(events) != 3 || events[2].To != domain.DeliverySending ||
		events[2].Version != winner.claim.Delivery.Version {
		t.Fatalf("exactly one post-recovery claim event must exist: events=%+v err=%v", events, err)
	}
}
