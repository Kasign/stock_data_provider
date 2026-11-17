package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeOutboxStore struct {
	mu             sync.Mutex
	messages       []OutboxMessage
	acknowledged   []string
	retried        []string
	staleAck       map[string]bool
	staleRetry     map[string]bool
	retryAt        map[string]time.Time
	retryErrorCode map[string]string
	claimErr       error
}

func (s *fakeOutboxStore) ClaimOutbox(_ context.Context, _, _ string, _ int, _ time.Duration) ([]OutboxMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]OutboxMessage(nil), s.messages...), s.claimErr
}

func (s *fakeOutboxStore) AcknowledgeOutbox(_ context.Context, ref, _ string, _ uint64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.staleAck[ref] {
		return false, nil
	}
	s.acknowledged = append(s.acknowledged, ref)
	return true, nil
}

func (s *fakeOutboxStore) RetryOutbox(_ context.Context, ref, _ string, _ uint64, next time.Time, code string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.staleRetry[ref] {
		return false, nil
	}
	s.retried = append(s.retried, ref)
	s.retryAt[ref] = next
	s.retryErrorCode[ref] = code
	return true, nil
}

type fakeOutboxPublisher struct {
	mu     sync.Mutex
	errors map[string]error
	seen   []OutboxMessage
}

func (p *fakeOutboxPublisher) Publish(_ context.Context, message OutboxMessage) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seen = append(p.seen, message)
	return p.errors[message.Ref]
}

type codedOutboxError struct{ code string }

func (e codedOutboxError) Error() string { return "publish failed" }
func (e codedOutboxError) Code() string  { return e.code }

func TestOutboxDispatcherPublishesAndSchedulesRetry(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	store := &fakeOutboxStore{
		messages: []OutboxMessage{
			{Ref: "event-ok", IdempotencyKey: "idem-ok", AttemptCount: 1, LeaseGeneration: 1},
			{Ref: "event-fail", IdempotencyKey: "idem-fail", AttemptCount: 2, LeaseGeneration: 3},
		},
		retryAt: map[string]time.Time{}, retryErrorCode: map[string]string{},
		staleAck: map[string]bool{}, staleRetry: map[string]bool{},
	}
	publisher := &fakeOutboxPublisher{errors: map[string]error{
		"event-fail": codedOutboxError{code: "broker_unavailable"},
	}}
	dispatcher, err := NewOutboxDispatcher(store, publisher, OutboxDispatcherConfig{
		WorkerID: "worker-1", BatchSize: 10, MaxConcurrency: 2, LeaseDuration: 30 * time.Second,
		PublishTimeout: 10 * time.Second,
		RetryPolicy:    ExponentialOutboxRetryPolicy{BaseDelay: time.Second, MaxDelay: time.Minute},
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.now = func() time.Time { return now }

	result, err := dispatcher.DispatchBatch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Claimed != 2 || result.Published != 1 || result.Retried != 1 || result.Stale != 0 {
		t.Fatalf("unexpected dispatch result: %+v", result)
	}
	if len(store.acknowledged) != 1 || store.acknowledged[0] != "event-ok" {
		t.Fatalf("acknowledged = %v", store.acknowledged)
	}
	if len(store.retried) != 1 || store.retried[0] != "event-fail" {
		t.Fatalf("retried = %v", store.retried)
	}
	if got, want := store.retryErrorCode["event-fail"], "broker_unavailable"; got != want {
		t.Fatalf("retry error code = %q, want %q", got, want)
	}
	if got, want := store.retryAt["event-fail"], now.Add(2*time.Second); !got.Equal(want) {
		t.Fatalf("retry time = %s, want %s", got, want)
	}
	if len(publisher.seen) != 2 {
		t.Fatalf("publisher received %d messages, want 2", len(publisher.seen))
	}
	foundIdempotencyKey := false
	for _, message := range publisher.seen {
		if message.Ref == "event-fail" && message.IdempotencyKey == "idem-fail" {
			foundIdempotencyKey = true
		}
	}
	if !foundIdempotencyKey {
		t.Fatalf("publisher did not receive stable idempotency metadata: %+v", publisher.seen)
	}
}

func TestOutboxDispatcherCountsStaleLeaseAndClaimFailure(t *testing.T) {
	store := &fakeOutboxStore{
		messages:   []OutboxMessage{{Ref: "stale", AttemptCount: 1, LeaseGeneration: 2}},
		staleAck:   map[string]bool{"stale": true},
		staleRetry: map[string]bool{},
		retryAt:    map[string]time.Time{}, retryErrorCode: map[string]string{},
	}
	dispatcher, err := NewOutboxDispatcher(store, &fakeOutboxPublisher{errors: map[string]error{}},
		OutboxDispatcherConfig{WorkerID: "worker-1", BatchSize: 1, MaxConcurrency: 1, LeaseDuration: time.Minute, PublishTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	result, err := dispatcher.DispatchBatch(context.Background())
	if err != nil || result.Stale != 1 || result.Published != 0 {
		t.Fatalf("stale acknowledgement result=%+v err=%v", result, err)
	}
	store.claimErr = errors.New("database unavailable")
	if _, err := dispatcher.DispatchBatch(context.Background()); err == nil {
		t.Fatal("expected claim failure")
	}
}

func TestOutboxDispatcherRejectsUnsafeLeaseConfiguration(t *testing.T) {
	_, err := NewOutboxDispatcher(&fakeOutboxStore{}, &fakeOutboxPublisher{},
		OutboxDispatcherConfig{WorkerID: "worker", BatchSize: 1, MaxConcurrency: 1, LeaseDuration: time.Second, PublishTimeout: time.Second})
	if err == nil {
		t.Fatal("publish timeout must be shorter than the lease")
	}
}

func TestExponentialOutboxRetryPolicyCapsDelay(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	policy := ExponentialOutboxRetryPolicy{BaseDelay: time.Second, MaxDelay: 4 * time.Second}
	for attempt, want := range map[uint32]time.Duration{1: time.Second, 2: 2 * time.Second, 3: 4 * time.Second, 8: 4 * time.Second} {
		if got := policy.NextAttemptAt(attempt, now); !got.Equal(now.Add(want)) {
			t.Errorf("attempt %d retry at %s, want %s", attempt, got, now.Add(want))
		}
	}
}
