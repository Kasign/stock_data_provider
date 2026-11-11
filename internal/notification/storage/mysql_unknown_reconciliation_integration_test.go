package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/application"
	"example.com/stock-analysis-flow/internal/notification/domain"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

func TestNotificationMariaDBUnknownReconciliationIntegration(t *testing.T) {
	dsn := os.Getenv("NOTIFICATION_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NOTIFICATION_MARIADB_TEST_DSN to run Notification MariaDB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	db, err := openNotificationIntegrationDB(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB through Platform Storage: %v", err)
	}
	defer db.Close()
	if err := platformstorage.ApplyMigrations(ctx, db, "notification", Migrations()); err != nil {
		t.Fatalf("apply Notification migrations: %v", err)
	}
	repository, err := NewMySQLDeliveryRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("并发对账只有一个CAS成功", func(t *testing.T) {
		unknown := createMariaDBUnknownDelivery(t, ctx, db, repository, "concurrent")
		next, event := unknownSentTransition(t, unknown, "provider-concurrent")
		var successes atomic.Int64
		var conflicts atomic.Int64
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				err := repository.ReconcileUnknown(ctx, unknown.Version, next, event)
				switch {
				case err == nil:
					successes.Add(1)
				case errors.Is(err, application.ErrUnknownReconciliationConflict):
					conflicts.Add(1)
				default:
					t.Errorf("unexpected reconciliation result: %v", err)
				}
			}()
		}
		close(start)
		wg.Wait()
		if successes.Load() != 1 || conflicts.Load() != 1 {
			t.Fatalf("expected one CAS winner and one conflict, success=%d conflict=%d",
				successes.Load(), conflicts.Load())
		}
		loaded, found, err := repository.Get(ctx, unknown.DeliveryID)
		if err != nil || !found || loaded.Status != domain.DeliverySent || loaded.Version != next.Version {
			t.Fatalf("load concurrent result: found=%v delivery=%+v err=%v", found, loaded, err)
		}
	})

	t.Run("状态事件失败回滚快照并保留租约", func(t *testing.T) {
		unknown := createMariaDBUnknownDelivery(t, ctx, db, repository, "rollback")
		setStaleLeaseForUnknownFixture(t, ctx, db, unknown.DeliveryID)
		next, event := unknownSentTransition(t, unknown, "provider-rollback")

		if _, err := db.ExecContext(ctx, `INSERT INTO notification_delivery_state_events (
			delivery_id, version, from_status, to_status, changed_at
		) VALUES (?, ?, ?, ?, ?)`, unknown.DeliveryID, next.Version, string(event.From),
			string(event.To), event.ChangedAt); err != nil {
			t.Fatalf("insert conflicting event fixture: %v", err)
		}
		if err := repository.ReconcileUnknown(ctx, unknown.Version, next, event); err == nil {
			t.Fatal("duplicate state event must fail reconciliation transaction")
		}
		stillUnknown, found, err := repository.Get(ctx, unknown.DeliveryID)
		if err != nil || !found || stillUnknown.Status != domain.DeliveryUnknown ||
			stillUnknown.Version != unknown.Version {
			t.Fatalf("failed event append must roll back delivery snapshot: found=%v delivery=%+v err=%v",
				found, stillUnknown, err)
		}
		assertMariaDBLeaseFields(t, ctx, db, unknown.DeliveryID, true)
		if _, err := db.ExecContext(ctx, "DELETE FROM notification_delivery_state_events WHERE delivery_id = ? AND version = ?",
			unknown.DeliveryID, next.Version); err != nil {
			t.Fatalf("remove conflicting event fixture: %v", err)
		}
		if err := repository.ReconcileUnknown(ctx, unknown.Version, next, event); err != nil {
			t.Fatalf("retry reconciliation after removing fixture: %v", err)
		}
		assertMariaDBLeaseFields(t, ctx, db, unknown.DeliveryID, false)
	})

	t.Run("旧版本拒绝覆盖", func(t *testing.T) {
		unknown := createMariaDBUnknownDelivery(t, ctx, db, repository, "stale")
		next, event := unknownSentTransition(t, unknown, "provider-stale")

		staleBase := unknown
		staleBase.Version--
		staleBase.UpdatedAt = unknown.UpdatedAt
		staleNext, staleEvent, err := staleBase.Transition(domain.DeliveryTransition{
			To: domain.DeliveryFailed, At: unknown.UpdatedAt.Add(time.Second),
			ReasonCode: "provider_rejected", EvidenceRef: "lookup-stale",
		})
		if err != nil {
			t.Fatalf("build stale transition: %v", err)
		}
		if err := repository.ReconcileUnknown(ctx, staleBase.Version, staleNext, staleEvent); !errors.Is(err, application.ErrUnknownReconciliationConflict) {
			t.Fatalf("stale expected version must conflict, got %v", err)
		}
		if err := repository.ReconcileUnknown(ctx, unknown.Version, next, event); err != nil {
			t.Fatalf("current version should still reconcile after stale request: %v", err)
		}
	})

	t.Run("不可变重试预算拒绝篡改", func(t *testing.T) {
		unknown := createMariaDBUnknownDelivery(t, ctx, db, repository, "immutable-budget")
		next, event := unknownSentTransition(t, unknown, "provider-immutable-budget")

		tampered := next
		tampered.MaxAttempts++
		if err := repository.ReconcileUnknown(ctx, unknown.Version, tampered, event); !errors.Is(err, application.ErrUnknownReconciliationConflict) {
			t.Fatalf("changed max_attempts must conflict, got %v", err)
		}

		tampered = next
		tampered.AttemptCount++
		if err := repository.ReconcileUnknown(ctx, unknown.Version, tampered, event); !errors.Is(err, application.ErrUnknownReconciliationConflict) {
			t.Fatalf("changed attempt_count must conflict, got %v", err)
		}

		tampered = next
		tampered.DeadlineAt = unknown.DeadlineAt.Add(time.Minute)
		if err := repository.ReconcileUnknown(ctx, unknown.Version, tampered, event); !errors.Is(err, application.ErrUnknownReconciliationConflict) {
			t.Fatalf("changed deadline_at must conflict, got %v", err)
		}

		tampered = next
		tampered.CreatedAt = unknown.CreatedAt.Add(time.Second)
		if err := repository.ReconcileUnknown(ctx, unknown.Version, tampered, event); !errors.Is(err, application.ErrUnknownReconciliationConflict) {
			t.Fatalf("changed created_at must conflict, got %v", err)
		}

		if err := repository.ReconcileUnknown(ctx, unknown.Version, next, event); err != nil {
			t.Fatalf("valid immutable snapshot should still reconcile: %v", err)
		}
	})

	t.Run("StateEvent 必须与状态快照一致", func(t *testing.T) {
		unknown := createMariaDBUnknownDelivery(t, ctx, db, repository, "event-mismatch")
		next, event := unknownSentTransition(t, unknown, "provider-event-mismatch")

		mismatched := event
		reason := "different_reason"
		mismatched.ReasonCode = &reason
		if err := repository.ReconcileUnknown(ctx, unknown.Version, next, mismatched); !errors.Is(err, domain.ErrInvalidDeliveryChange) {
			t.Fatalf("mismatched event reason must be rejected before transaction, got %v", err)
		}

		mismatched = event
		invalidEvidence := "lookup with spaces"
		mismatched.EvidenceRef = &invalidEvidence
		if err := repository.ReconcileUnknown(ctx, unknown.Version, next, mismatched); !errors.Is(err, domain.ErrInvalidDeliveryChange) {
			t.Fatalf("invalid evidence reference must be rejected, got %v", err)
		}

		mismatched = event
		local := time.FixedZone("UTC+8", 8*60*60)
		mismatched.ChangedAt = event.ChangedAt.In(local)
		if err := repository.ReconcileUnknown(ctx, unknown.Version, next, mismatched); !errors.Is(err, domain.ErrInvalidDeliveryChange) {
			t.Fatalf("non-UTC event timestamp must be rejected, got %v", err)
		}

		mismatched = event
		actor := "unexpected-actor"
		mismatched.ActorRef = &actor
		if err := repository.ReconcileUnknown(ctx, unknown.Version, next, mismatched); !errors.Is(err, domain.ErrInvalidDeliveryChange) {
			t.Fatalf("unknown reconciliation must not accept manual actor metadata, got %v", err)
		}

		if err := repository.ReconcileUnknown(ctx, unknown.Version, next, event); err != nil {
			t.Fatalf("valid state event should reconcile: %v", err)
		}
	})

	t.Run("永久拒绝收敛failed", func(t *testing.T) {
		unknown := createMariaDBUnknownDelivery(t, ctx, db, repository, "rejected")
		next, event, err := unknown.Transition(domain.DeliveryTransition{
			To: domain.DeliveryFailed, At: unknown.UpdatedAt.Add(time.Second),
			ReasonCode: "provider_rejected", ErrorCode: "provider_rejected",
			EvidenceRef: "lookup-rejected",
		})
		if err != nil {
			t.Fatalf("build failed transition: %v", err)
		}
		if err := repository.ReconcileUnknown(ctx, unknown.Version, next, event); err != nil {
			t.Fatalf("reconcile permanent rejection: %v", err)
		}
		loaded, found, err := repository.Get(ctx, unknown.DeliveryID)
		if err != nil || !found || loaded.Status != domain.DeliveryFailed || loaded.Version != next.Version {
			t.Fatalf("load failed reconciliation: found=%v delivery=%+v err=%v", found, loaded, err)
		}
		var evidence string
		if err := db.QueryRowContext(ctx, `SELECT evidence_ref FROM notification_delivery_state_events
			WHERE delivery_id = ? AND version = ?`, unknown.DeliveryID, next.Version).Scan(&evidence); err != nil || evidence != "lookup-rejected" {
			t.Fatalf("failed reconciliation must persist evidence_ref: evidence=%q err=%v", evidence, err)
		}
		assertMariaDBLeaseFields(t, ctx, db, unknown.DeliveryID, false)
	})
}

func createMariaDBUnknownDelivery(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	repository *MySQLDeliveryRepository,
	suffix string,
) domain.DeliveryRecord {
	t.Helper()
	plan := integrationDeliveryPlan()
	plan.DeliveryID += "-unknown-" + suffix
	plan.EventID += "-unknown-" + suffix
	plan.SubscriptionID += "-unknown-" + suffix
	fingerprint, err := plan.CalculateFingerprint()
	if err != nil {
		t.Fatalf("calculate plan fingerprint: %v", err)
	}
	plan.PlanFingerprint = fingerprint
	plans, err := NewMySQLDeliveryPlanRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, duplicate, err := plans.CreateOrGet(ctx, plan); err != nil || duplicate {
		t.Fatalf("create unknown delivery plan: duplicate=%v err=%v", duplicate, err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	delivery := domain.DeliveryRecord{
		DeliveryID: plan.DeliveryID, SourceModule: plan.SourceModule, EventID: plan.EventID,
		SubscriptionID: plan.SubscriptionID, SubscriptionVersion: plan.SubscriptionVersion,
		ChannelRef: plan.ChannelRef, ChannelConfigVersion: plan.ChannelConfigVersion,
		TemplateID: plan.TemplateID, TemplateVersion: plan.TemplateVersion,
		RecipientRef: plan.RecipientRef, PayloadFingerprint: plan.PayloadFingerprint,
		PlanFingerprint: plan.PlanFingerprint, DeliveryIdempotencyKey: plan.DeliveryIdempotencyKey,
		Status: domain.DeliveryQueued, Version: 1, MaxAttempts: 3,
		DeadlineAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.Create(ctx, delivery); err != nil {
		t.Fatalf("create queued delivery: %v", err)
	}
	claim, claimed, err := repository.ClaimDue(ctx, "unknown-fixture-"+suffix, now.Add(time.Second), now.Add(30*time.Second))
	if err != nil || !claimed || claim.Delivery.DeliveryID != delivery.DeliveryID {
		t.Fatalf("claim fixture delivery: claimed=%v claim=%+v err=%v", claimed, claim, err)
	}
	unknownAt := now.Add(2 * time.Second)
	unknown, event, err := claim.Delivery.Transition(domain.DeliveryTransition{
		To: domain.DeliveryUnknown, At: unknownAt, ReasonCode: "channel_outcome_unknown",
		EvidenceRef: "attempt-ambiguous-" + suffix,
	})
	if err != nil {
		t.Fatalf("build unknown transition: %v", err)
	}
	if err := repository.UpdateWithLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, unknown, event, unknownAt); err != nil {
		t.Fatalf("persist unknown fixture: %v", err)
	}
	return unknown
}

func unknownSentTransition(
	t *testing.T,
	unknown domain.DeliveryRecord,
	providerMessageRef string,
) (domain.DeliveryRecord, domain.DeliveryStateEvent) {
	t.Helper()
	next, event, err := unknown.Transition(domain.DeliveryTransition{
		To: domain.DeliverySent, At: unknown.UpdatedAt.Add(time.Second),
		EvidenceRef: "lookup-" + providerMessageRef, ProviderMessageRef: providerMessageRef,
	})
	if err != nil {
		t.Fatalf("build sent transition: %v", err)
	}
	return next, event
}

func setStaleLeaseForUnknownFixture(t *testing.T, ctx context.Context, db *sql.DB, deliveryID string) {
	t.Helper()
	_, err := db.ExecContext(ctx, `UPDATE notification_deliveries
		SET lease_worker_id = ?, lease_expires_at = UTC_TIMESTAMP(6)
		WHERE delivery_id = ?`, "stale-unknown-worker", deliveryID)
	if err != nil {
		t.Fatalf("set stale lease fixture: %v", err)
	}
}

func assertMariaDBLeaseFields(t *testing.T, ctx context.Context, db *sql.DB, deliveryID string, wantPresent bool) {
	t.Helper()
	var worker sql.NullString
	var expiresAt sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT lease_worker_id, lease_expires_at
		FROM notification_deliveries WHERE delivery_id = ?`, deliveryID).Scan(&worker, &expiresAt); err != nil {
		t.Fatalf("read lease fields: %v", err)
	}
	present := worker.Valid || expiresAt.Valid
	if present != wantPresent {
		t.Fatalf("unexpected lease fields: worker=%v expires_at=%v wantPresent=%v", worker, expiresAt, wantPresent)
	}
}
