package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

type memoryDeliveries struct {
	mu       sync.Mutex
	records  map[string]domain.DeliveryRecord
	leases   map[string]DeliveryLease
	tokens   map[string]int64
	events          map[string][]domain.DeliveryStateEvent
	attempts        map[string]domain.DeliveryAttempt
	failNext        bool
	failAttemptNext bool
}

func newMemoryDeliveries() *memoryDeliveries {
	return &memoryDeliveries{
		records: make(map[string]domain.DeliveryRecord),
		leases:  make(map[string]DeliveryLease),
		tokens:  make(map[string]int64),
		events:   make(map[string][]domain.DeliveryStateEvent),
		attempts: make(map[string]domain.DeliveryAttempt),
	}
}

func cloneDeliveryRecord(record domain.DeliveryRecord) domain.DeliveryRecord {
	record.NextAttemptAt = cloneDeliveryTime(record.NextAttemptAt)
	record.ReasonCode = cloneDeliveryString(record.ReasonCode)
	record.LastErrorCode = cloneDeliveryString(record.LastErrorCode)
	record.ProviderMessageRef = cloneDeliveryString(record.ProviderMessageRef)
	return record
}

func cloneDeliveryTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneDeliveryString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func (m *memoryDeliveries) Create(ctx context.Context, delivery domain.DeliveryRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := delivery.Validate(); err != nil {
		return err
	}
	if delivery.Status != domain.DeliveryQueued || delivery.Version != 1 ||
		delivery.AttemptCount != 0 || delivery.NextAttemptAt != nil {
		return domain.ErrInvalidDelivery
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failNext {
		m.failNext = false
		return errInjectedRepositoryFailure
	}
	if _, exists := m.records[delivery.DeliveryID]; exists {
		return ErrDeliveryAlreadyExists
	}
	m.records[delivery.DeliveryID] = cloneDeliveryRecord(delivery)
	m.tokens[delivery.DeliveryID] = 0
	return nil
}

func (m *memoryDeliveries) Get(ctx context.Context, deliveryID string) (domain.DeliveryRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return domain.DeliveryRecord{}, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	record, found := m.records[deliveryID]
	if !found {
		return domain.DeliveryRecord{}, false, nil
	}
	return cloneDeliveryRecord(record), true, nil
}

func (m *memoryDeliveries) ClaimDue(ctx context.Context, workerID string, now, leaseExpiresAt time.Time) (DeliveryClaim, bool, error) {
	if err := ctx.Err(); err != nil {
		return DeliveryClaim{}, false, err
	}
	if !validOpaqueWorkerRef(workerID) || !isUTCApplicationTime(now) ||
		!isUTCApplicationTime(leaseExpiresAt) || !leaseExpiresAt.After(now) {
		return DeliveryClaim{}, false, domain.ErrInvalidDeliveryChange
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failNext {
		m.failNext = false
		return DeliveryClaim{}, false, errInjectedRepositoryFailure
	}

	ids := make([]string, 0, len(m.records))
	for id := range m.records {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		left, right := m.records[ids[i]], m.records[ids[j]]
		if left.CreatedAt.Equal(right.CreatedAt) {
			return ids[i] < ids[j]
		}
		return left.CreatedAt.Before(right.CreatedAt)
	})

	for _, id := range ids {
		record := m.records[id]
		if record.Status != domain.DeliveryQueued && record.Status != domain.DeliveryRetryWait {
			continue
		}
		if !now.Before(record.DeadlineAt) || record.AttemptCount >= record.MaxAttempts {
			continue
		}
		if record.Status == domain.DeliveryRetryWait && now.Before(*record.NextAttemptAt) {
			continue
		}

		next, event, err := record.Transition(domain.DeliveryTransition{
			To: domain.DeliverySending, At: now,
		})
		if err != nil {
			continue
		}
		token := m.tokens[id] + 1
		effectiveLeaseExpiry := leaseExpiresAt
		if effectiveLeaseExpiry.After(record.DeadlineAt) {
			effectiveLeaseExpiry = record.DeadlineAt
		}
		if !effectiveLeaseExpiry.After(now) {
			continue
		}
		lease := DeliveryLease{WorkerID: workerID, FencingToken: token, ExpiresAt: effectiveLeaseExpiry}
		m.records[id] = cloneDeliveryRecord(next)
		m.tokens[id] = token
		m.leases[id] = lease
		m.events[id] = append(m.events[id], event)
		return DeliveryClaim{Delivery: cloneDeliveryRecord(next), Lease: lease}, true, nil
	}
	return DeliveryClaim{}, false, nil
}

func (m *memoryDeliveries) UpdateWithLease(ctx context.Context, expectedVersion, fencingToken int64, next domain.DeliveryRecord, event domain.DeliveryStateEvent, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !isUTCApplicationTime(now) || expectedVersion < 1 || fencingToken < 1 || next.UpdatedAt.After(now) {
		return ErrDeliveryLeaseConflict
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	current, found := m.records[next.DeliveryID]
	if !found {
		return ErrDeliveryNotFound
	}
	lease, leased := m.leases[next.DeliveryID]
	if !leased || lease.FencingToken != fencingToken || !now.Before(lease.ExpiresAt) {
		return ErrDeliveryLeaseConflict
	}
	if current.Version != expectedVersion || next.Version != expectedVersion+1 {
		return ErrDeliveryVersionConflict
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if event.DeliveryID != next.DeliveryID || event.Version != next.Version ||
		event.From != current.Status || event.To != next.Status ||
		!event.ChangedAt.Equal(next.UpdatedAt) {
		return domain.ErrInvalidDeliveryChange
	}
	if m.failNext {
		m.failNext = false
		return errInjectedRepositoryFailure
	}

	m.records[next.DeliveryID] = cloneDeliveryRecord(next)
	m.events[next.DeliveryID] = append(m.events[next.DeliveryID], cloneDeliveryEvent(event))
	if next.Status != domain.DeliverySending {
		delete(m.leases, next.DeliveryID)
	}
	return nil
}

func (m *memoryDeliveries) StateEvents(deliveryID string) []domain.DeliveryStateEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	events := m.events[deliveryID]
	result := make([]domain.DeliveryStateEvent, len(events))
	for i, event := range events {
		result[i] = cloneDeliveryEvent(event)
	}
	return result
}

func cloneDeliveryEvent(event domain.DeliveryStateEvent) domain.DeliveryStateEvent {
	event.ReasonCode = cloneDeliveryString(event.ReasonCode)
	event.EvidenceRef = cloneDeliveryString(event.EvidenceRef)
	event.ActorRef = cloneDeliveryString(event.ActorRef)
	event.RequestID = cloneDeliveryString(event.RequestID)
	event.ActorReason = cloneDeliveryString(event.ActorReason)
	return event
}

func validOpaqueWorkerRef(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z') &&
			!(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') &&
			character != '-' && character != '_' && character != '.' && character != ':' {
			return false
		}
	}
	return true
}

func isUTCApplicationTime(value time.Time) bool {
	if value.IsZero() {
		return false
	}
	_, offset := value.Zone()
	return value.Location() == time.UTC && offset == 0
}

func validDeliveryForRepository() domain.DeliveryRecord {
	now := time.Date(2026, 11, 9, 2, 0, 0, 0, time.UTC)
	return domain.DeliveryRecord{
		DeliveryID: "delivery-1", SourceModule: "researchflow", EventID: "event-1",
		SubscriptionID: "subscription-1", SubscriptionVersion: 1,
		ChannelRef: "channel-1", ChannelConfigVersion: 1,
		TemplateID: "template-1", TemplateVersion: 1, RecipientRef: "recipient-1",
		PayloadFingerprint: strings.Repeat("a", 64), PlanFingerprint: strings.Repeat("b", 64),
		DeliveryIdempotencyKey: "delivery-key-1", Status: domain.DeliveryQueued, Version: 1,
		AttemptCount: 0, MaxAttempts: 3, DeadlineAt: now.Add(time.Hour),
		CreatedAt: now, UpdatedAt: now,
	}
}

func TestDeliveryRepositoryCreateIsUniqueAndFailureAtomic(t *testing.T) {
	repository := newMemoryDeliveries()
	ctx := context.Background()
	delivery := validDeliveryForRepository()

	repository.failNext = true
	if err := repository.Create(ctx, delivery); !errors.Is(err, errInjectedRepositoryFailure) {
		t.Fatalf("expected injected create failure, got %v", err)
	}
	if _, found, err := repository.Get(ctx, delivery.DeliveryID); err != nil || found {
		t.Fatalf("failed create must not persist a partial delivery, found=%v err=%v", found, err)
	}
	if err := repository.Create(ctx, delivery); err != nil {
		t.Fatalf("create after transient failure: %v", err)
	}
	if err := repository.Create(ctx, delivery); !errors.Is(err, ErrDeliveryAlreadyExists) {
		t.Fatalf("duplicate DeliveryID must conflict, got %v", err)
	}
}

func TestDeliveryRepositoryConcurrentClaimHasSingleWinner(t *testing.T) {
	repository := newMemoryDeliveries()
	ctx := context.Background()
	delivery := validDeliveryForRepository()
	if err := repository.Create(ctx, delivery); err != nil {
		t.Fatal(err)
	}

	now := delivery.CreatedAt.Add(time.Second)
	leaseUntil := now.Add(time.Minute)
	const workers = 32
	var claimed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			claim, ok, err := repository.ClaimDue(ctx, fmt.Sprintf("worker-%02d", worker), now, leaseUntil)
			if err != nil {
				t.Errorf("claim delivery: %v", err)
				return
			}
			if ok {
				claimed.Add(1)
				if claim.Delivery.Status != domain.DeliverySending || claim.Delivery.Version != 2 ||
					claim.Lease.FencingToken != 1 || !claim.Lease.ExpiresAt.Equal(leaseUntil) {
					t.Errorf("unexpected claim: %+v", claim)
				}
			}
		}(i)
	}
	wg.Wait()
	if claimed.Load() != 1 {
		t.Fatalf("expected exactly one worker claim, got %d", claimed.Load())
	}
	record, found, err := repository.Get(ctx, delivery.DeliveryID)
	if err != nil || !found || record.Status != domain.DeliverySending || record.Version != 2 {
		t.Fatalf("unexpected persisted delivery: record=%+v found=%v err=%v", record, found, err)
	}
	if got := len(repository.StateEvents(delivery.DeliveryID)); got != 1 {
		t.Fatalf("claim must append exactly one state event, got %d", got)
	}
}

func TestDeliveryRepositoryCASRejectsExpiredLeaseAndStaleVersion(t *testing.T) {
	repository := newMemoryDeliveries()
	ctx := context.Background()
	delivery := validDeliveryForRepository()
	if err := repository.Create(ctx, delivery); err != nil {
		t.Fatal(err)
	}

	now := delivery.CreatedAt.Add(time.Second)
	leaseUntil := now.Add(10 * time.Second)
	claim, ok, err := repository.ClaimDue(ctx, "worker-a", now, leaseUntil)
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	next, event, err := claim.Delivery.Transition(domain.DeliveryTransition{
		To: domain.DeliverySent, At: now.Add(2 * time.Second), ProviderMessageRef: "provider-message-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := repository.UpdateWithLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, next, event, leaseUntil); !errors.Is(err, ErrDeliveryLeaseConflict) {
		t.Fatalf("expired lease must be rejected, got %v", err)
	}
	if err := repository.UpdateWithLease(ctx, claim.Delivery.Version-1, claim.Lease.FencingToken, next, event, now.Add(3*time.Second)); !errors.Is(err, ErrDeliveryVersionConflict) {
		t.Fatalf("stale version must be rejected, got %v", err)
	}
	if err := repository.UpdateWithLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken+1, next, event, now.Add(3*time.Second)); !errors.Is(err, ErrDeliveryLeaseConflict) {
		t.Fatalf("stale fencing token must be rejected, got %v", err)
	}
	if err := repository.UpdateWithLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, next, event, now.Add(3*time.Second)); err != nil {
		t.Fatalf("valid CAS update: %v", err)
	}
	if got := len(repository.StateEvents(delivery.DeliveryID)); got != 2 {
		t.Fatalf("expected claim and completion events, got %d", got)
	}
	if err := repository.UpdateWithLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, next, event, now.Add(4*time.Second)); !errors.Is(err, ErrDeliveryLeaseConflict) {
		t.Fatalf("completed lease must no longer authorize writes, got %v", err)
	}
}

func TestDeliveryRepositoryUpdateFailureDoesNotPartiallyAppend(t *testing.T) {
	repository := newMemoryDeliveries()
	ctx := context.Background()
	delivery := validDeliveryForRepository()
	if err := repository.Create(ctx, delivery); err != nil {
		t.Fatal(err)
	}

	now := delivery.CreatedAt.Add(time.Second)
	claim, ok, err := repository.ClaimDue(ctx, "worker-a", now, now.Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	next, event, err := claim.Delivery.Transition(domain.DeliveryTransition{
		To: domain.DeliverySent, At: now.Add(2 * time.Second), ProviderMessageRef: "provider-message-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	repository.failNext = true
	if err := repository.UpdateWithLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, next, event, now.Add(3*time.Second)); !errors.Is(err, errInjectedRepositoryFailure) {
		t.Fatalf("expected injected update failure, got %v", err)
	}
	current, found, err := repository.Get(ctx, delivery.DeliveryID)
	if err != nil || !found || current.Status != domain.DeliverySending || current.Version != claim.Delivery.Version {
		t.Fatalf("failed update must preserve current snapshot, current=%+v found=%v err=%v", current, found, err)
	}
	if got := len(repository.StateEvents(delivery.DeliveryID)); got != 1 {
		t.Fatalf("failed update must not append a partial state event, got %d", got)
	}
	if err := repository.UpdateWithLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, next, event, now.Add(4*time.Second)); err != nil {
		t.Fatalf("retry after transient failure: %v", err)
	}
	if got := len(repository.StateEvents(delivery.DeliveryID)); got != 2 {
		t.Fatalf("successful retry must append exactly one state event, got %d", got)
	}
}

func TestDeliveryRepositoryRetryWaitIsNotClaimedBeforeDueTime(t *testing.T) {
	repository := newMemoryDeliveries()
	ctx := context.Background()
	delivery := validDeliveryForRepository()
	if err := repository.Create(ctx, delivery); err != nil {
		t.Fatal(err)
	}

	now := delivery.CreatedAt.Add(time.Second)
	leaseUntil := now.Add(time.Minute)
	first, ok, err := repository.ClaimDue(ctx, "worker-a", now, leaseUntil)
	if err != nil || !ok {
		t.Fatalf("first claim: ok=%v err=%v", ok, err)
	}
	retryAt := now.Add(20 * time.Second)
	retry, retryEvent, err := first.Delivery.Transition(domain.DeliveryTransition{
		To: domain.DeliveryRetryWait, At: now.Add(2 * time.Second),
		ReasonCode: "provider_retryable", ErrorCode: "rate_limited", NextAttemptAt: &retryAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdateWithLease(ctx, first.Delivery.Version, first.Lease.FencingToken, retry, retryEvent, now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}

	if _, claimed, err := repository.ClaimDue(ctx, "worker-b", retryAt.Add(-time.Second), retryAt.Add(time.Minute)); err != nil || claimed {
		t.Fatalf("retry must not be claimed early, claimed=%v err=%v", claimed, err)
	}
	second, claimed, err := repository.ClaimDue(ctx, "worker-b", retryAt, retryAt.Add(time.Minute))
	if err != nil || !claimed {
		t.Fatalf("retry should be claimable at next_attempt_at, claimed=%v err=%v", claimed, err)
	}
	if second.Lease.FencingToken != first.Lease.FencingToken+1 {
		t.Fatalf("fencing token must increase monotonically, first=%d second=%d", first.Lease.FencingToken, second.Lease.FencingToken)
	}

	staleNext, staleEvent, err := first.Delivery.Transition(domain.DeliveryTransition{
		To: domain.DeliverySent, At: retryAt.Add(time.Second), ProviderMessageRef: "late-provider-message",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdateWithLease(ctx, first.Delivery.Version, first.Lease.FencingToken, staleNext, staleEvent, retryAt.Add(time.Second)); !errors.Is(err, ErrDeliveryVersionConflict) {
		t.Fatalf("old worker must not overwrite the newer claim, got %v", err)
	}
}
