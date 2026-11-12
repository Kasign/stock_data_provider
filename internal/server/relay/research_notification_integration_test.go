package relay

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	notificationapplication "example.com/stock-analysis-flow/internal/notification/application"
	notificationschema "example.com/stock-analysis-flow/internal/notification/schema"
	notificationstorage "example.com/stock-analysis-flow/internal/notification/storage"
	researchapp "example.com/stock-analysis-flow/internal/research/application"
	researchcontext "example.com/stock-analysis-flow/internal/research/context"
	researchevidence "example.com/stock-analysis-flow/internal/research/evidence"
	researchstorage "example.com/stock-analysis-flow/internal/research/storage"
	servermigration "example.com/stock-analysis-flow/internal/server/migration"
)

type relayIntegrationContextRepository struct{}

func (relayIntegrationContextRepository) Append(context.Context, researchcontext.Context) error {
	return nil
}

func (relayIntegrationContextRepository) GetVersion(context.Context, string, int64) (researchcontext.Context, error) {
	return researchcontext.Context{}, fmt.Errorf("context not found")
}

type relayIntegrationEvidenceRepository struct{}

func (relayIntegrationEvidenceRepository) Append(context.Context, researchevidence.Evidence) error {
	return nil
}

func (relayIntegrationEvidenceRepository) Get(context.Context, string) (researchevidence.Evidence, error) {
	return researchevidence.Evidence{}, fmt.Errorf("evidence not found")
}

type relayIntegrationEvidenceAuthorizer struct{}

func (relayIntegrationEvidenceAuthorizer) AuthorizeEvidence(context.Context, string, researchevidence.PurposeCode, researchevidence.Evidence) error {
	return nil
}

type relayIntegrationIDGenerator struct {
	sequence atomic.Uint64
	suffix   string
}

func (g *relayIntegrationIDGenerator) NewID(prefix string) (string, error) {
	return fmt.Sprintf("%s%s%d", prefix, g.suffix, g.sequence.Add(1)), nil
}

type relayIntegrationClock struct{}

func (relayIntegrationClock) Now() time.Time { return time.Now().UTC() }

func TestResearchOutboxRelayMariaDB(t *testing.T) {
	dsn := os.Getenv("SERVER_RELAY_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SERVER_RELAY_TEST_MYSQL_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(8)
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping MariaDB: %v", err)
	}

	migrations := servermigration.NewRegistry()
	if err := migrations.Register(researchstorage.MigrationNamespace, researchstorage.Migrations()); err != nil {
		t.Fatalf("register Research migrations: %v", err)
	}
	if err := migrations.Register("notification", notificationstorage.Migrations()); err != nil {
		t.Fatalf("register Notification migrations: %v", err)
	}
	if _, err := migrations.RunWithReport(ctx, db); err != nil {
		t.Fatalf("apply Research and Notification migrations: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	clock := func() time.Time { return now }
	eventRepository, err := researchstorage.NewEventRepository(db, clock)
	if err != nil {
		t.Fatalf("create Research Event Repository: %v", err)
	}
	ids := &relayIntegrationIDGenerator{suffix: fmt.Sprintf("it%d", time.Now().UnixNano())}
	researchService, err := researchapp.NewServiceWithEvents(
		relayIntegrationContextRepository{},
		relayIntegrationEvidenceRepository{},
		eventRepository,
		relayIntegrationEvidenceAuthorizer{},
		clock,
		ids,
	)
	if err != nil {
		t.Fatalf("create Research Event Service: %v", err)
	}
	created, err := researchService.CreateEvent(ctx, researchapp.CreateEventCommand{
		EventType: "integration.test_event",
		Title:     "Research Notification relay integration event",
		Status:    "detected",
		SubjectRefs: []string{"stock:600000"},
		EvidenceRefs: []string{},
		SystemObservedAt: now,
		SystemAsOf: now,
		ImpactHypothesisRefs: []string{},
		InvalidationConditions: []string{},
		Uncertainty: []string{},
		TimeNormalizationRuleVersion: "1.0.0",
		SourceTimes: []researchapp.EventSourceTime{},
		DerivedFromRefs: []researchapp.EventRef{},
	})
	if err != nil {
		t.Fatalf("create Research Event and transactional Outbox: %v", err)
	}

	outboxRepository, err := researchstorage.NewOutboxRepository(db, clock)
	if err != nil {
		t.Fatalf("create Research Outbox Repository: %v", err)
	}
	outbox, err := researchapp.NewOutboxService(outboxRepository, clock)
	if err != nil {
		t.Fatalf("create Research Outbox Service: %v", err)
	}
	fanoutWriter, err := notificationstorage.NewMySQLFanoutWorkItemRepository(db)
	if err != nil {
		t.Fatalf("create Notification Fanout WorkItem Repository: %v", err)
	}
	inbox, err := notificationstorage.NewMySQLInboxRepository(db, fanoutWriter)
	if err != nil {
		t.Fatalf("create Notification Inbox Repository: %v", err)
	}
	registry, err := notificationschema.NewRegistry(os.DirFS("../../../doc/notification/schema"), notificationschema.ResourceLimits{
		MaxEnvelopeBytes: 8192, MaxPayloadBytes: 4096, MaxJSONDepth: 16,
	})
	if err != nil {
		t.Fatalf("create Notification Schema Registry: %v", err)
	}
	receiver, err := notificationapplication.NewEventReceiver(registry, inbox, relayIntegrationClock{})
	if err != nil {
		t.Fatalf("create Notification EventReceiver: %v", err)
	}
	relay, err := NewResearchOutboxRelay(outbox, eventReceiverAdapter{receiver: receiver}, Config{
		WorkerID: "relay-integration-worker", BatchSize: 4,
		PollInterval: time.Second, LeaseDuration: 30 * time.Second, RetryDelay: 10 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("create Research Notification Relay: %v", err)
	}

	if err := relay.ProcessOnce(ctx); err != nil {
		t.Fatalf("process Research Outbox through Notification Inbox: %v", err)
	}

	idempotencyKey := "research-event-state:research-event:" + created.EventID + ":1"
	var outboxEventID, deliveryStatus string
	var deliveredAt sql.NullString
	if err := db.QueryRowContext(ctx,
		"SELECT event_id, delivery_status, delivered_at FROM research_notification_outbox WHERE idempotency_key=?",
		idempotencyKey,
	).Scan(&outboxEventID, &deliveryStatus, &deliveredAt); err != nil {
		t.Fatalf("read Research Outbox delivery state: %v", err)
	}
	if deliveryStatus != "published" || !deliveredAt.Valid || deliveredAt.String == "" {
		t.Fatalf("Research Outbox state = (%s, delivered_at=%v), want published", deliveryStatus, deliveredAt)
	}

	var inboxCount, fanoutCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM notification_inbox WHERE source_module='research' AND event_id=?",
		outboxEventID,
	).Scan(&inboxCount); err != nil {
		t.Fatalf("count Notification Inbox records: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM notification_fanout_work_items WHERE source_module='research' AND event_id=?",
		outboxEventID,
	).Scan(&fanoutCount); err != nil {
		t.Fatalf("count Notification Fanout WorkItems: %v", err)
	}
	if inboxCount != 1 || fanoutCount != 1 {
		t.Fatalf("cross-module delivery counts = inbox:%d fanout:%d, want exactly one each", inboxCount, fanoutCount)
	}

	if err := relay.ProcessOnce(ctx); err != nil {
		t.Fatalf("repeat Research Outbox relay pass: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM notification_inbox WHERE source_module='research' AND event_id=?",
		outboxEventID,
	).Scan(&inboxCount); err != nil {
		t.Fatalf("recount Notification Inbox records: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM notification_fanout_work_items WHERE source_module='research' AND event_id=?",
		outboxEventID,
	).Scan(&fanoutCount); err != nil {
		t.Fatalf("recount Notification Fanout WorkItems: %v", err)
	}
	if inboxCount != 1 || fanoutCount != 1 {
		t.Fatalf("replayed delivery duplicated records: inbox:%d fanout:%d", inboxCount, fanoutCount)
	}
}

type eventReceiverAdapter struct {
	receiver *notificationapplication.EventReceiver
}

func (a eventReceiverAdapter) Receive(ctx context.Context, raw []byte) error {
	_, err := a.receiver.Receive(ctx, raw)
	return err
}
