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

type subscriptionTestEventTypes struct{}

func (subscriptionTestEventTypes) HasEventType(eventType string) bool {
	return eventType == "quant.risk_decision.changed"
}

type memorySubscriptions struct {
	mu              sync.RWMutex
	versions        map[string][]domain.Subscription
	eventTypes      domain.EventTypeLookup
	filterValidator domain.FilterSchemaValidator
	failNextCreate  bool
	failNextAppend  bool
}

func newMemorySubscriptions() *memorySubscriptions {
	return &memorySubscriptions{
		versions:   make(map[string][]domain.Subscription),
		eventTypes: subscriptionTestEventTypes{},
	}
}

func cloneSubscription(subscription domain.Subscription) domain.Subscription {
	cloned := subscription
	cloned.Filters = append(json.RawMessage(nil), subscription.Filters...)
	return cloned
}

func (m *memorySubscriptions) Create(ctx context.Context, subscription domain.Subscription) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if subscription.Version != 1 {
		return domain.ErrSubscriptionVersionConflict
	}
	if err := subscription.Validate(m.eventTypes, m.filterValidator); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failNextCreate {
		m.failNextCreate = false
		return errInjectedRepositoryFailure
	}
	if _, exists := m.versions[subscription.SubscriptionID]; exists {
		return domain.ErrSubscriptionVersionConflict
	}
	m.versions[subscription.SubscriptionID] = []domain.Subscription{cloneSubscription(subscription)}
	return nil
}

func (m *memorySubscriptions) GetLatest(ctx context.Context, subscriptionID string) (domain.Subscription, bool, error) {
	if err := ctx.Err(); err != nil {
		return domain.Subscription{}, false, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	versions := m.versions[subscriptionID]
	if len(versions) == 0 {
		return domain.Subscription{}, false, nil
	}
	return cloneSubscription(versions[len(versions)-1]), true, nil
}

func (m *memorySubscriptions) AppendVersion(ctx context.Context, expectedVersion int64, next domain.Subscription) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	versions := m.versions[next.SubscriptionID]
	if len(versions) == 0 {
		return domain.ErrSubscriptionVersionConflict
	}
	previous := versions[len(versions)-1]
	if previous.Version != expectedVersion {
		return domain.ErrSubscriptionVersionConflict
	}
	if err := domain.ValidateSubscriptionVersion(previous, next); err != nil {
		return err
	}
	if err := next.Validate(m.eventTypes, m.filterValidator); err != nil {
		return err
	}
	if m.failNextAppend {
		m.failNextAppend = false
		return errInjectedRepositoryFailure
	}
	m.versions[next.SubscriptionID] = append(versions, cloneSubscription(next))
	return nil
}

func validApplicationSubscription() domain.Subscription {
	now := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	return domain.Subscription{
		SubscriptionID: "sub-1", OwnerRef: "owner-1", Version: 1,
		Status: domain.SubscriptionActive, EventType: "quant.risk_decision.changed",
		RecipientScope: domain.RecipientScopeOwner,
		ChannelRef:     "channel-1", TemplateID: "template-1",
		CreatedAt: now, UpdatedAt: now, EffectiveAt: now,
	}
}

func nextSubscriptionVersion(previous domain.Subscription) domain.Subscription {
	next := cloneSubscription(previous)
	next.Version++
	next.UpdatedAt = previous.UpdatedAt.Add(time.Second)
	next.EffectiveAt = previous.EffectiveAt.Add(time.Second)
	return next
}

func TestSubscriptionRepositoryCreatesAndAppendsImmutableVersions(t *testing.T) {
	repository := newMemorySubscriptions()
	ctx := context.Background()
	initial := validApplicationSubscription()
	if err := repository.Create(ctx, initial); err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(ctx, initial); !errors.Is(err, domain.ErrSubscriptionVersionConflict) {
		t.Fatalf("duplicate subscription create should conflict, got %v", err)
	}

	next := nextSubscriptionVersion(initial)
	next.Status = domain.SubscriptionPaused
	if err := repository.AppendVersion(ctx, 1, next); err != nil {
		t.Fatalf("append version 2: %v", err)
	}

	latest, found, err := repository.GetLatest(ctx, initial.SubscriptionID)
	if err != nil || !found {
		t.Fatalf("get latest subscription: found=%v err=%v", found, err)
	}
	if latest.Version != 2 || latest.Status != domain.SubscriptionPaused {
		t.Fatalf("unexpected latest subscription: %+v", latest)
	}
	if repository.versions[initial.SubscriptionID][0].Version != 1 ||
		repository.versions[initial.SubscriptionID][0].Status != domain.SubscriptionActive {
		t.Fatal("append must preserve the original subscription version")
	}

	latest.Filters = json.RawMessage(`{"mutated":true}`)
	reloaded, _, err := repository.GetLatest(ctx, initial.SubscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Filters) != 0 {
		t.Fatal("mutating a returned subscription must not mutate repository state")
	}
}

func TestSubscriptionRepositoryRejectsStaleAndInvalidVersions(t *testing.T) {
	repository := newMemorySubscriptions()
	ctx := context.Background()
	initial := validApplicationSubscription()
	if err := repository.Create(ctx, initial); err != nil {
		t.Fatal(err)
	}

	next := nextSubscriptionVersion(initial)
	if err := repository.AppendVersion(ctx, 0, next); !errors.Is(err, domain.ErrSubscriptionVersionConflict) {
		t.Fatalf("stale expected version should conflict, got %v", err)
	}

	invalid := next
	invalid.Version = 3
	if err := repository.AppendVersion(ctx, 1, invalid); !errors.Is(err, domain.ErrSubscriptionVersionConflict) {
		t.Fatalf("version gap should conflict, got %v", err)
	}

	invalid = next
	invalid.OwnerRef = "different-owner"
	if err := repository.AppendVersion(ctx, 1, invalid); !errors.Is(err, domain.ErrSubscriptionVersionConflict) {
		t.Fatalf("owner mutation should conflict, got %v", err)
	}
}

func TestSubscriptionRepositoryConcurrentAppendUsesCompareAndSwap(t *testing.T) {
	repository := newMemorySubscriptions()
	ctx := context.Background()
	initial := validApplicationSubscription()
	if err := repository.Create(ctx, initial); err != nil {
		t.Fatal(err)
	}
	next := nextSubscriptionVersion(initial)

	const workers = 32
	var succeeded atomic.Int64
	var conflicted atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
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
				t.Errorf("unexpected append error: %v", err)
			}
		}()
	}
	wg.Wait()

	if succeeded.Load() != 1 || conflicted.Load() != workers-1 {
		t.Fatalf("expected one append and %d conflicts, got appended=%d conflicts=%d",
			workers-1, succeeded.Load(), conflicted.Load())
	}
	latest, found, err := repository.GetLatest(ctx, initial.SubscriptionID)
	if err != nil || !found || latest.Version != 2 {
		t.Fatalf("expected one version-2 winner, latest=%+v found=%v err=%v", latest, found, err)
	}
	if got := len(repository.versions[initial.SubscriptionID]); got != 2 {
		t.Fatalf("expected exactly two immutable versions, got %d", got)
	}
}
