package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/application"
	"example.com/stock-analysis-flow/internal/notification/domain"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

func TestNotificationMariaDBFanoutWorkItemIntegration(t *testing.T) {
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
	fanout, err := NewMySQLFanoutWorkItemRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	inbox, err := NewMySQLInboxRepository(db, fanout)
	if err != nil {
		t.Fatal(err)
	}
	event := integrationInboxEvent(fmt.Sprintf("fanout-%d", time.Now().UnixNano()))
	receivedAt := time.Now().UTC().Truncate(time.Microsecond)
	if _, duplicate, err := inbox.Accept(ctx, event, receivedAt); err != nil || duplicate {
		t.Fatalf("first accept: duplicate=%v err=%v", duplicate, err)
	}
	if _, duplicate, err := inbox.Accept(ctx, event, receivedAt.Add(time.Second)); err != nil || !duplicate {
		t.Fatalf("replay: duplicate=%v err=%v", duplicate, err)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notification_fanout_work_items WHERE source_module = ? AND event_id = ?",
		event.SourceModule, event.EventID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("Inbox replay must keep exactly one FanoutWorkItem, got %d", count)
	}

	first, claimed, err := fanout.ClaimDue(ctx, "fanout-worker-1", time.Minute)
	if err != nil || !claimed {
		t.Fatalf("claim first page: claimed=%v err=%v", claimed, err)
	}
	if first.WorkItem.Status != domain.FanoutProcessing || first.WorkItem.AttemptCount != 1 ||
		first.WorkItem.FencingToken != 1 || !first.WorkItem.HasSnapshot ||
		first.Event.EventID != event.EventID {
		t.Fatalf("unexpected first claim: %+v", first)
	}
	if err := fanout.AdvanceWithLease(ctx, first, "sub-001", false); err != nil {
		t.Fatalf("advance first page: %v", err)
	}
	second, claimed, err := fanout.ClaimDue(ctx, "fanout-worker-2", time.Minute)
	if err != nil || !claimed {
		t.Fatalf("claim second page: claimed=%v err=%v", claimed, err)
	}
	if second.WorkItem.FencingToken != first.WorkItem.FencingToken+1 ||
		second.WorkItem.CursorSubscriptionID != "sub-001" ||
		second.WorkItem.SnapshotRevision != first.WorkItem.SnapshotRevision {
		t.Fatalf("reclaim must preserve snapshot/cursor and increase fencing: first=%+v second=%+v", first.WorkItem, second.WorkItem)
	}
	if err := fanout.AdvanceWithLease(ctx, first, "sub-002", false); !errors.Is(err, application.ErrFanoutWorkItemLeaseConflict) {
		t.Fatalf("stale fencing token must be rejected, got %v", err)
	}
	if err := fanout.AdvanceWithLease(ctx, second, "sub-002", true); err != nil {
		t.Fatalf("complete fanout: %v", err)
	}
	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM notification_fanout_work_items WHERE work_item_id = ?",
		second.WorkItem.WorkItemID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(domain.FanoutCompleted) {
		t.Fatalf("expected completed work item, got %s", status)
	}
}

func TestNotificationMariaDBSubscriptionRevisionSnapshotIntegration(t *testing.T) {
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
	repository, err := NewMySQLSubscriptionRepository(db, subscriptionIntegrationEventTypes{}, subscriptionIntegrationFilterValidator{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	first := domain.Subscription{
		SubscriptionID:      fmt.Sprintf("snapshot-sub-%d", time.Now().UnixNano()),
		OwnerRef:            "owner-snapshot",
		Version:             1,
		Status:              domain.SubscriptionActive,
		EventType:           "quant.risk_decision.changed",
		FilterSchemaVersion: "1.0.0",
		Filters:             []byte(`{"min_severity":"high"}`),
		RecipientScope:      domain.RecipientScopeOwner,
		ChannelRef:          "channel-snapshot",
		TemplateID:          "template-snapshot",
		CreatedAt:           now,
		UpdatedAt:           now,
		EffectiveAt:         now,
	}
	if err := repository.Create(ctx, first); err != nil {
		t.Fatal(err)
	}
	var firstRevision int64
	if err := db.QueryRowContext(ctx, "SELECT current_revision FROM notification_subscription_revision WHERE singleton_id = 1").Scan(&firstRevision); err != nil {
		t.Fatal(err)
	}
	paused := first
	paused.Version = 2
	paused.Status = domain.SubscriptionPaused
	paused.UpdatedAt = now.Add(time.Second)
	paused.EffectiveAt = now.Add(time.Second)
	if err := repository.AppendVersion(ctx, 1, paused); err != nil {
		t.Fatal(err)
	}
	second := first
	second.SubscriptionID += "-later"
	second.CreatedAt = now.Add(2 * time.Second)
	second.UpdatedAt = second.CreatedAt
	second.EffectiveAt = second.CreatedAt
	if err := repository.Create(ctx, second); err != nil {
		t.Fatal(err)
	}
	var currentRevision int64
	if err := db.QueryRowContext(ctx, "SELECT current_revision FROM notification_subscription_revision WHERE singleton_id = 1").Scan(&currentRevision); err != nil {
		t.Fatal(err)
	}
	oldPage, err := repository.ListActiveAtRevision(ctx, first.EventType, firstRevision, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(oldPage) != 1 || oldPage[0].SubscriptionID != first.SubscriptionID || oldPage[0].Version != 1 ||
		oldPage[0].Status != domain.SubscriptionActive {
		t.Fatalf("old snapshot must retain the previously active version: %+v", oldPage)
	}
	currentPage, err := repository.ListActiveAtRevision(ctx, first.EventType, currentRevision, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(currentPage) != 1 || currentPage[0].SubscriptionID != second.SubscriptionID {
		t.Fatalf("current snapshot must exclude paused latest version and include later subscription: %+v", currentPage)
	}
}
