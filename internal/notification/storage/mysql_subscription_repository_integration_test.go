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

	mysql "github.com/go-sql-driver/mysql"

	"example.com/stock-analysis-flow/internal/notification/application"
	"example.com/stock-analysis-flow/internal/notification/domain"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

type subscriptionIntegrationEventTypes struct{}

func (subscriptionIntegrationEventTypes) HasEventType(eventType string) bool {
	return eventType == "quant.risk_decision.changed"
}

func TestNotificationMariaDBSubscriptionRepositoryIntegration(t *testing.T) {
	dsn := os.Getenv("NOTIFICATION_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NOTIFICATION_MARIADB_TEST_DSN to run Notification MariaDB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	driverConfig, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse MariaDB DSN: %v", err)
	}
	connector, err := mysql.NewConnector(driverConfig)
	if err != nil {
		t.Fatalf("create MariaDB connector: %v", err)
	}
	db, err := platformstorage.OpenSQL(ctx, connector, platformstorage.DefaultSQLConfig())
	if err != nil {
		t.Fatalf("open MariaDB through Platform Storage: %v", err)
	}
	defer db.Close()
	if err := platformstorage.ApplyMigrations(ctx, db, "notification", Migrations()); err != nil {
		t.Fatalf("apply Notification migrations: %v", err)
	}
	repository, err := NewMySQLSubscriptionRepository(db, subscriptionIntegrationEventTypes{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	initial := domain.Subscription{
		SubscriptionID: fmt.Sprintf("subscription-%d", time.Now().UnixNano()),
		OwnerRef: "owner-integration", Version: 1, Status: domain.SubscriptionActive,
		EventType: "quant.risk_decision.changed", RecipientScope: domain.RecipientScopeOwner,
		ChannelRef: "channel-integration", TemplateID: "template-integration",
		CreatedAt: now, UpdatedAt: now, EffectiveAt: now,
	}
	if err := repository.Create(ctx, initial); err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	if err := repository.Create(ctx, initial); !errors.Is(err, domain.ErrSubscriptionVersionConflict) {
		t.Fatalf("duplicate initial subscription must conflict, got %v", err)
	}

	next := initial
	next.Version = 2
	next.Status = domain.SubscriptionPaused
	next.UpdatedAt = now.Add(time.Second)
	next.EffectiveAt = now.Add(time.Second)

	const contenders = 12
	var succeeded atomic.Int64
	var conflicted atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := repository.AppendVersion(ctx, 1, next)
			switch {
			case err == nil:
				succeeded.Add(1)
			case errors.Is(err, domain.ErrSubscriptionVersionConflict):
				conflicted.Add(1)
			default:
				t.Errorf("unexpected concurrent append error: %v", err)
			}
		}()
	}
	wg.Wait()
	if succeeded.Load() != 1 || conflicted.Load() != contenders-1 {
		t.Fatalf("expected one version-2 winner, got success=%d conflict=%d", succeeded.Load(), conflicted.Load())
	}

	latest, found, err := repository.GetLatest(ctx, initial.SubscriptionID)
	if err != nil || !found || latest.Version != 2 || latest.Status != domain.SubscriptionPaused {
		t.Fatalf("load latest subscription: found=%v subscription=%+v err=%v", found, latest, err)
	}
	if latest.CreatedAt != initial.CreatedAt || latest.OwnerRef != initial.OwnerRef {
		t.Fatalf("immutable subscription identity changed: %+v", latest)
	}
	latest.Status = domain.SubscriptionDisabled
	reloaded, found, err := repository.GetLatest(ctx, initial.SubscriptionID)
	if err != nil || !found || reloaded.Status != domain.SubscriptionPaused {
		t.Fatalf("mutating returned value must not affect persisted subscription: found=%v value=%+v err=%v", found, reloaded, err)
	}
	if _, found, err := repository.GetLatest(ctx, initial.SubscriptionID+"-missing"); err != nil || found {
		t.Fatalf("missing subscription should return found=false: found=%v err=%v", found, err)
	}

	invalid := next
	invalid.Version = 3
	invalid.OwnerRef = "owner-changed"
	invalid.UpdatedAt = next.UpdatedAt.Add(time.Second)
	invalid.EffectiveAt = next.EffectiveAt.Add(time.Second)
	if err := repository.AppendVersion(ctx, 2, invalid); !errors.Is(err, domain.ErrSubscriptionVersionConflict) {
		t.Fatalf("owner mutation must conflict: %v", err)
	}

	_ = application.ErrDeliveryNotFound
}
