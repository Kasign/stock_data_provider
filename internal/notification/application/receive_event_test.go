package application

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

type staticValidator struct {
	event domain.EventEnvelope
	err   error
}

func (v staticValidator) ValidateEvent(raw []byte) (domain.EventEnvelope, error) {
	if v.err != nil {
		return domain.EventEnvelope{}, v.err
	}
	return v.event, nil
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

type inboxKey struct {
	source string
	value  string
}

type memoryInbox struct {
	mu              sync.Mutex
	byEvent         map[inboxKey]domain.InboxRecord
	byIdempotency   map[inboxKey]domain.InboxRecord
	fanoutWorkItems int
	failNextAccept bool
}

func newMemoryInbox() *memoryInbox {
	return &memoryInbox{
		byEvent:       make(map[inboxKey]domain.InboxRecord),
		byIdempotency: make(map[inboxKey]domain.InboxRecord),
	}
}

func (m *memoryInbox) Accept(_ context.Context, event domain.EventEnvelope, receivedAt time.Time) (domain.InboxRecord, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failNextAccept {
		m.failNextAccept = false
		return domain.InboxRecord{}, false, errInjectedRepositoryFailure
	}

	eventKey := inboxKey{source: event.SourceModule, value: event.EventID}
	idempotencyKey := inboxKey{source: event.SourceModule, value: event.IdempotencyKey}
	byEvent, eventExists := m.byEvent[eventKey]
	byIdempotency, idempotencyExists := m.byIdempotency[idempotencyKey]
	if eventExists || idempotencyExists {
		if eventExists && idempotencyExists && byEvent.Event.EventID != byIdempotency.Event.EventID {
			return domain.InboxRecord{}, false, domain.ErrInboxIdempotencyConflict
		}
		existing := byEvent
		if !eventExists {
			existing = byIdempotency
		}
		if err := domain.ValidateInboxReplay(existing.Event, event); err != nil {
			return domain.InboxRecord{}, false, err
		}
		return existing, true, nil
	}

	record := domain.InboxRecord{Event: event, ReceivedAt: receivedAt}
	m.byEvent[eventKey] = record
	m.byIdempotency[idempotencyKey] = record
	m.fanoutWorkItems++
	return record, false, nil
}

func TestEventReceiverAcceptsAndDeduplicates(t *testing.T) {
	now := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	event := domain.EventEnvelope{
		SourceModule: "quant", EventID: "event-1",
		IdempotencyKey: "publish-1", PayloadFingerprint: "fingerprint",
	}
	repo := newMemoryInbox()
	receiver, err := NewEventReceiver(staticValidator{event: event}, repo, fixedClock{now: now})
	if err != nil {
		t.Fatal(err)
	}

	first, err := receiver.Receive(context.Background(), []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Duplicate || first.EventID != event.EventID || !first.AcceptedAt.Equal(now) {
		t.Fatalf("unexpected first result: %+v", first)
	}
	second, err := receiver.Receive(context.Background(), []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if !second.Duplicate || second.EventID != event.EventID {
		t.Fatalf("expected duplicate result: %+v", second)
	}
	if len(repo.byEvent) != 1 || len(repo.byIdempotency) != 1 || repo.fanoutWorkItems != 1 {
		t.Fatalf("expected one Inbox record and one fan-out item, got event=%d idempotency=%d fanout=%d", len(repo.byEvent), len(repo.byIdempotency), repo.fanoutWorkItems)
	}
}

func TestEventReceiverRejectsBothUniqueKeyConflicts(t *testing.T) {
	now := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	base := domain.EventEnvelope{
		SourceModule: "quant", EventID: "event-1",
		IdempotencyKey: "publish-1", PayloadFingerprint: "fingerprint",
	}
	repo := newMemoryInbox()
	if _, _, err := repo.Accept(context.Background(), base, now); err != nil {
		t.Fatal(err)
	}

	incomingEvents := []domain.EventEnvelope{
		{SourceModule: "quant", EventID: "event-1", IdempotencyKey: "publish-2", PayloadFingerprint: "fingerprint"},
		{SourceModule: "quant", EventID: "event-2", IdempotencyKey: "publish-1", PayloadFingerprint: "fingerprint"},
		{SourceModule: "quant", EventID: "event-1", IdempotencyKey: "publish-1", PayloadFingerprint: "different"},
	}
	for _, incoming := range incomingEvents {
		if _, _, err := repo.Accept(context.Background(), incoming, now); !errors.Is(err, domain.ErrInboxIdempotencyConflict) {
			t.Fatalf("expected idempotency conflict for %+v, got %v", incoming, err)
		}
	}
}

func TestEventReceiverConcurrentReplayCreatesOneRecord(t *testing.T) {
	now := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	event := domain.EventEnvelope{
		SourceModule: "quant", EventID: "event-1",
		IdempotencyKey: "publish-1", PayloadFingerprint: "fingerprint",
	}
	repo := newMemoryInbox()
	receiver, err := NewEventReceiver(staticValidator{event: event}, repo, fixedClock{now: now})
	if err != nil {
		t.Fatal(err)
	}

	const workers = 32
	var accepted atomic.Int64
	var duplicates atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := receiver.Receive(context.Background(), json.RawMessage("{}"))
			if err != nil {
				t.Errorf("receive: %v", err)
				return
			}
			if result.Duplicate {
				duplicates.Add(1)
			} else {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 1 || duplicates.Load() != workers-1 {
		t.Fatalf("expected one accepted and %d duplicates, got accepted=%d duplicates=%d", workers-1, accepted.Load(), duplicates.Load())
	}
	if len(repo.byEvent) != 1 || len(repo.byIdempotency) != 1 {
		t.Fatalf("expected one Inbox record, got event=%d idempotency=%d", len(repo.byEvent), len(repo.byIdempotency))
	}
}

func TestEventReceiverValidatesBeforeRepository(t *testing.T) {
	repo := newMemoryInbox()
	receiver, err := NewEventReceiver(
		staticValidator{err: errors.New("invalid envelope")},
		repo,
		fixedClock{now: time.Now().UTC()},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := receiver.Receive(context.Background(), []byte("{}")); err == nil {
		t.Fatal("expected validation failure")
	}
	if len(repo.byEvent) != 0 {
		t.Fatal("invalid event reached Inbox repository")
	}
}
