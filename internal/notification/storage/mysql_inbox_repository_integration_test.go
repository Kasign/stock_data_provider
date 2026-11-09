package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

type inboxFanoutWriterFunc func(context.Context, *sql.Tx, domain.InboxRecord) error

func (f inboxFanoutWriterFunc) InsertForAcceptedEvent(ctx context.Context, tx *sql.Tx, record domain.InboxRecord) error {
	return f(ctx, tx, record)
}

func TestNotificationMariaDBInboxRepositoryIntegration(t *testing.T) {
	dsn := os.Getenv("NOTIFICATION_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NOTIFICATION_MARIADB_TEST_DSN to run Notification MariaDB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	db, err := openNotificationIntegrationDB(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB through Platform Storage: %v", err)
	}
	defer db.Close()
	if err := platformstorage.ApplyMigrations(ctx, db, "notification", Migrations()); err != nil {
		t.Fatalf("apply Notification migrations: %v", err)
	}

	table := fmt.Sprintf("notification_inbox_fanout_it_%d", time.Now().UnixNano())
	if _, err := db.ExecContext(ctx, "CREATE TABLE "+table+" (event_id VARCHAR(256) PRIMARY KEY) ENGINE=InnoDB"); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(context.Background(), "DROP TABLE "+table)

	writer := inboxFanoutWriterFunc(func(ctx context.Context, tx *sql.Tx, record domain.InboxRecord) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO "+table+" (event_id) VALUES (?)", record.Event.EventID)
		return err
	})
	repository, err := NewMySQLInboxRepository(db, writer)
	if err != nil {
		t.Fatal(err)
	}

	event := integrationInboxEvent("event-" + fmt.Sprintf("%d", time.Now().UnixNano()))
	receivedAt := time.Now().UTC().Truncate(time.Microsecond)
	first, duplicate, err := repository.Accept(ctx, event, receivedAt)
	if err != nil || duplicate || first.Event.EventID != event.EventID {
		t.Fatalf("first accept: record=%+v duplicate=%v err=%v", first, duplicate, err)
	}
	replayed, duplicate, err := repository.Accept(ctx, event, receivedAt.Add(time.Second))
	if err != nil || !duplicate || !replayed.ReceivedAt.Equal(receivedAt) {
		t.Fatalf("same event replay must return original record: record=%+v duplicate=%v err=%v", replayed, duplicate, err)
	}
	var fanoutCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE event_id = ?", event.EventID).Scan(&fanoutCount); err != nil {
		t.Fatal(err)
	}
	if fanoutCount != 1 {
		t.Fatalf("replay must not duplicate fanout work, got %d", fanoutCount)
	}

	eventConflict := event
	eventConflict.EventID = event.EventID + "-other"
	if _, _, err := repository.Accept(ctx, eventConflict, receivedAt); !errors.Is(err, domain.ErrInboxIdempotencyConflict) {
		t.Fatalf("reused idempotency key must conflict, got %v", err)
	}
	eventConflict = event
	eventConflict.IdempotencyKey = event.IdempotencyKey + "-other"
	eventConflict.EventType = "quant.other.event"
	eventConflict.PayloadFingerprint, err = eventConflict.CalculatePayloadFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.Accept(ctx, eventConflict, receivedAt); !errors.Is(err, domain.ErrInboxIdempotencyConflict) {
		t.Fatalf("reused event id with changed immutable envelope must conflict, got %v", err)
	}

	failingRepository, err := NewMySQLInboxRepository(db, inboxFanoutWriterFunc(
		func(context.Context, *sql.Tx, domain.InboxRecord) error { return errors.New("injected fanout failure") },
	))
	if err != nil {
		t.Fatal(err)
	}
	rollbackEvent := integrationInboxEvent(event.EventID + "-rollback")
	if _, _, err := failingRepository.Accept(ctx, rollbackEvent, receivedAt); err == nil {
		t.Fatal("fanout failure must fail Inbox acceptance")
	}
	var inboxCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notification_inbox WHERE source_module = ? AND event_id = ?",
		rollbackEvent.SourceModule, rollbackEvent.EventID).Scan(&inboxCount); err != nil {
		t.Fatal(err)
	}
	if inboxCount != 0 {
		t.Fatal("Inbox row must roll back when transactional fanout insertion fails")
	}
}

func integrationInboxEvent(eventID string) domain.EventEnvelope {
	event := domain.EventEnvelope{
		EventID: eventID, EventType: "quant.risk_decision.changed", SchemaVersion: "1.0.0",
		SourceModule: "quant", OccurredAt: "2026-11-09T09:00:00Z",
		PublishedAt: "2026-11-09T09:00:01Z", SubjectRef: "stock:600000",
		IdempotencyKey: "publish-" + eventID, CorrelationID: "correlation-" + eventID,
		Payload: []byte(`{"decision":"watch"}`),
	}
	fingerprint, err := event.CalculatePayloadFingerprint()
	if err != nil {
		panic(err)
	}
	event.PayloadFingerprint = fingerprint
	return event
}
