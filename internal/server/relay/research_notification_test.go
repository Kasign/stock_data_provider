package relay

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	notificationdomain "example.com/stock-analysis-flow/internal/notification/domain"
	notificationschema "example.com/stock-analysis-flow/internal/notification/schema"
	researchapp "example.com/stock-analysis-flow/internal/research/application"
)

type outboxFake struct {
	mu          sync.Mutex
	events      []researchapp.NotificationEventEnvelope
	claimed     int
	confirmed   []string
	retried     []string
	rejected    []string
	confirmErr  error
	retryErr    error
	rejectErr   error
	workerID    string
	batchSize   int
	leasePeriod time.Duration
}

func (f *outboxFake) Claim(_ context.Context, worker string, limit int, lease time.Duration) ([]researchapp.NotificationEventEnvelope, error) {
	f.claimed++
	f.workerID, f.batchSize, f.leasePeriod = worker, limit, lease
	return append([]researchapp.NotificationEventEnvelope(nil), f.events...), nil
}
func (f *outboxFake) Confirm(_ context.Context, eventID, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.confirmed = append(f.confirmed, eventID)
	return f.confirmErr
}
func (f *outboxFake) Retry(_ context.Context, eventID, _, _ string, _ time.Duration, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retried = append(f.retried, eventID)
	return f.retryErr
}
func (f *outboxFake) Reject(_ context.Context, eventID, _, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rejected = append(f.rejected, eventID)
	return f.rejectErr
}

type receiverFake struct {
	raw []byte
	err error
}

func (f *receiverFake) Receive(_ context.Context, raw []byte) error {
	f.raw = append([]byte(nil), raw...)
	return f.err
}

func relayConfig() Config {
	return Config{WorkerID: "relay-worker", BatchSize: 8, PollInterval: time.Second, LeaseDuration: 30 * time.Second, RetryDelay: 20 * time.Second}
}

func relayEvent() researchapp.NotificationEventEnvelope {
	return researchapp.NotificationEventEnvelope{
		EventID: "nte_1", EventType: "research.event.lifecycle_changed", SchemaVersion: "1.0.0",
		SourceModule: "research", OccurredAt: "2026-11-12T00:00:00Z", PublishedAt: "2026-11-12T00:00:01Z",
		SubjectRef: "research-event:evt_1", IdempotencyKey: "research-event-state:research-event:evt_1:1",
		CorrelationID: "corr_1", PayloadFingerprint: strings.Repeat("a", 64),
		Payload: []byte(`{"event_version":1}`), LeaseToken: strings.Repeat("b", 32),
	}
}

func TestProcessOnceForwardsEnvelopeAndConfirmsLease(t *testing.T) {
	outbox := &outboxFake{events: []researchapp.NotificationEventEnvelope{relayEvent()}}
	receiver := &receiverFake{}
	relay, err := NewResearchOutboxRelay(outbox, receiver, relayConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := relay.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if outbox.claimed != 1 || outbox.workerID != "relay-worker" || outbox.batchSize != 8 || outbox.leasePeriod != 30*time.Second {
		t.Fatalf("unexpected claim arguments: %+v", outbox)
	}
	if len(outbox.confirmed) != 1 || outbox.confirmed[0] != "nte_1" {
		t.Fatalf("confirmed events = %v", outbox.confirmed)
	}
	if strings.Contains(string(receiver.raw), "lease_token") || strings.Contains(string(receiver.raw), strings.Repeat("b", 32)) {
		t.Fatalf("Research lease token leaked to Notification Envelope: %s", receiver.raw)
	}
	if !strings.Contains(string(receiver.raw), "research.event.lifecycle_changed") {
		t.Fatalf("Notification did not receive the Research event envelope: %s", receiver.raw)
	}
}

func TestProcessOnceRejectsPermanentEventContractErrors(t *testing.T) {
	outbox := &outboxFake{events: []researchapp.NotificationEventEnvelope{relayEvent()}}
	receiver := &receiverFake{err: notificationschema.ErrUnknownSchema}
	relay, err := NewResearchOutboxRelay(outbox, receiver, relayConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := relay.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if len(outbox.rejected) != 1 || outbox.rejected[0] != "nte_1" || len(outbox.retried) != 0 {
		t.Fatalf("permanent contract failure was not rejected: rejected=%v retried=%v", outbox.rejected, outbox.retried)
	}
}

func TestProcessOnceRetriesTransientInboxErrors(t *testing.T) {
	outbox := &outboxFake{events: []researchapp.NotificationEventEnvelope{relayEvent()}}
	receiver := &receiverFake{err: errors.New("database temporarily unavailable")}
	relay, err := NewResearchOutboxRelay(outbox, receiver, relayConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := relay.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if len(outbox.retried) != 1 || outbox.retried[0] != "nte_1" || len(outbox.rejected) != 0 {
		t.Fatalf("transient Inbox error was not retried: retried=%v rejected=%v", outbox.retried, outbox.rejected)
	}
}

func TestProcessOnceReturnsConfirmationFailureForLeaseRecovery(t *testing.T) {
	confirmErr := errors.New("outbox confirmation failed")
	outbox := &outboxFake{events: []researchapp.NotificationEventEnvelope{relayEvent()}, confirmErr: confirmErr}
	receiver := &receiverFake{}
	relay, err := NewResearchOutboxRelay(outbox, receiver, relayConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := relay.ProcessOnce(context.Background()); !errors.Is(err, confirmErr) {
		t.Fatalf("ProcessOnce() error = %v, want confirmation failure", err)
	}
	if receiver.raw == nil {
		t.Fatal("Notification Inbox must be called before Outbox confirmation")
	}
}

func TestPermanentInboxIdempotencyConflictIsRejected(t *testing.T) {
	if !isPermanentEventError(notificationdomain.ErrInboxIdempotencyConflict) {
		t.Fatal("Inbox idempotency conflict must be treated as a permanent event conflict")
	}
}

func TestConfigRejectsInvalidValues(t *testing.T) {
	config := relayConfig()
	config.LeaseDuration = 0
	if err := config.Validate(); err == nil {
		t.Fatal("zero lease duration must be rejected")
	}
}


func TestProcessOnceHandlesEveryClaimedEvent(t *testing.T) {
	events := []researchapp.NotificationEventEnvelope{relayEvent(), relayEvent()}
	events[1].EventID = "nte_2"
	outbox := &outboxFake{events: events}
	var received atomic.Int64
	receiver := receiverFunc(func(context.Context, []byte) error {
		received.Add(1)
		return nil
	})
	relay, err := NewResearchOutboxRelay(outbox, receiver, relayConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := relay.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if received.Load() != 2 || len(outbox.confirmed) != 2 {
		t.Fatalf("batch was not fully handled: received=%d confirmed=%v", received.Load(), outbox.confirmed)
	}
}

type receiverFunc func(context.Context, []byte) error

func (f receiverFunc) Receive(ctx context.Context, raw []byte) error {
	return f(ctx, raw)
}
