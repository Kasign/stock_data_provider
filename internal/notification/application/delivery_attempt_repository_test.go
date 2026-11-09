package application

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

func (m *memoryDeliveries) StartWithLease(ctx context.Context, expectedDeliveryVersion, fencingToken int64, attempt domain.DeliveryAttempt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := attempt.Validate(); err != nil || attempt.FinishedAt != nil {
		return domain.ErrInvalidDeliveryAttempt
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	current, found := m.records[attempt.DeliveryID]
	if !found {
		return ErrDeliveryNotFound
	}
	now := m.now
	if !isUTCApplicationTime(now) || attempt.StartedAt.After(now) {
		return domain.ErrInvalidDeliveryAttempt
	}
	lease, leased := m.leases[attempt.DeliveryID]
	if !leased || lease.FencingToken != fencingToken || attempt.FencingToken != fencingToken || lease.WorkerID != attempt.WorkerID ||
		!lease.ExpiresAt.Equal(attempt.LeaseExpiresAt) || !now.Before(lease.ExpiresAt) ||
		attempt.StartedAt.Before(current.UpdatedAt) || !attempt.StartedAt.Before(lease.ExpiresAt) {
		return ErrDeliveryLeaseConflict
	}
	if current.Version != expectedDeliveryVersion {
		return ErrDeliveryVersionConflict
	}
	if current.Status != domain.DeliverySending || current.AttemptCount >= current.MaxAttempts ||
		!attempt.StartedAt.Before(current.DeadlineAt) {
		return ErrDeliveryAttemptConflict
	}
	if _, exists := m.attempts[attempt.AttemptID]; exists {
		return ErrDeliveryAttemptAlreadyExists
	}
	if attempt.AttemptNo != current.AttemptCount+1 {
		return ErrDeliveryAttemptConflict
	}
	for _, existing := range m.attempts {
		if existing.DeliveryID == attempt.DeliveryID && existing.AttemptNo == attempt.AttemptNo {
			return ErrDeliveryAttemptAlreadyExists
		}
	}
	if m.failAttemptNext {
		m.failAttemptNext = false
		return errInjectedRepositoryFailure
	}

	m.attempts[attempt.AttemptID] = cloneApplicationAttempt(attempt)
	current.AttemptCount = attempt.AttemptNo
	m.records[attempt.DeliveryID] = cloneDeliveryRecord(current)
	return nil
}

func (m *memoryDeliveries) CompleteWithLease(ctx context.Context, expectedDeliveryVersion, fencingToken int64, completed domain.DeliveryAttempt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := completed.Validate(); err != nil || completed.FinishedAt == nil {
		return domain.ErrInvalidDeliveryAttempt
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	current, found := m.records[completed.DeliveryID]
	if !found {
		return ErrDeliveryNotFound
	}
	now := m.now
	if !isUTCApplicationTime(now) || completed.FinishedAt.After(now) {
		return domain.ErrInvalidDeliveryAttempt
	}
	lease, leased := m.leases[completed.DeliveryID]
	if !leased || lease.FencingToken != fencingToken || lease.WorkerID != completed.WorkerID ||
		!now.Before(lease.ExpiresAt) || !completed.FinishedAt.Before(lease.ExpiresAt) {
		return ErrDeliveryLeaseConflict
	}
	if current.Version != expectedDeliveryVersion {
		return ErrDeliveryVersionConflict
	}
	if current.Status != domain.DeliverySending {
		return ErrDeliveryAttemptConflict
	}
	existing, exists := m.attempts[completed.AttemptID]
	if !exists || existing.DeliveryID != completed.DeliveryID {
		return ErrDeliveryAttemptNotFound
	}
	if existing.FinishedAt != nil || existing.AttemptNo != completed.AttemptNo ||
		existing.WorkerID != completed.WorkerID || existing.FencingToken != completed.FencingToken ||
		!existing.LeaseExpiresAt.Equal(completed.LeaseExpiresAt) ||
		!existing.StartedAt.Equal(completed.StartedAt) || completed.FencingToken != fencingToken {
		return ErrDeliveryAttemptConflict
	}
	if m.failAttemptNext {
		m.failAttemptNext = false
		return errInjectedRepositoryFailure
	}
	m.attempts[completed.AttemptID] = cloneApplicationAttempt(completed)
	return nil
}

func (m *memoryDeliveries) ListByDelivery(ctx context.Context, deliveryID string) ([]domain.DeliveryAttempt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, found := m.records[deliveryID]; !found {
		return nil, ErrDeliveryNotFound
	}
	result := make([]domain.DeliveryAttempt, 0)
	for _, attempt := range m.attempts {
		if attempt.DeliveryID == deliveryID {
			result = append(result, cloneApplicationAttempt(attempt))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].AttemptNo < result[j].AttemptNo })
	return result, nil
}

func cloneApplicationAttempt(attempt domain.DeliveryAttempt) domain.DeliveryAttempt {
	if attempt.FinishedAt != nil {
		value := *attempt.FinishedAt
		attempt.FinishedAt = &value
	}
	if attempt.Outcome != nil {
		value := *attempt.Outcome
		attempt.Outcome = &value
	}
	attempt.ProviderMessageRef = cloneDeliveryString(attempt.ProviderMessageRef)
	attempt.ErrorCode = cloneDeliveryString(attempt.ErrorCode)
	attempt.EvidenceRef = cloneDeliveryString(attempt.EvidenceRef)
	if attempt.DurationMS != nil {
		value := *attempt.DurationMS
		attempt.DurationMS = &value
	}
	return attempt
}

func claimDeliveryForAttempt(t *testing.T, repository *memoryDeliveries) DeliveryClaim {
	t.Helper()
	delivery := validDeliveryForRepository()
	if err := repository.Create(context.Background(), delivery); err != nil {
		t.Fatal(err)
	}
	claim, ok, err := repository.ClaimDue(context.Background(), "worker-attempt", delivery.CreatedAt.Add(time.Second), delivery.CreatedAt.Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("claim delivery: claimed=%v err=%v", ok, err)
	}
	repository.now = claim.Lease.ExpiresAt.Add(-time.Second)
	return claim
}

func attemptForClaim(claim DeliveryClaim, attemptNo int) domain.DeliveryAttempt {
	started := claim.Delivery.UpdatedAt.Add(time.Second)
	return domain.DeliveryAttempt{
		AttemptID: "attempt-" + string(rune('0'+attemptNo)), DeliveryID: claim.Delivery.DeliveryID,
		AttemptNo: attemptNo, WorkerID: claim.Lease.WorkerID, FencingToken: claim.Lease.FencingToken,
		LeaseExpiresAt: claim.Lease.ExpiresAt, StartedAt: started,
	}
}

func TestDeliveryAttemptRepositoryStartAndCompleteWithActiveLease(t *testing.T) {
	repository := newMemoryDeliveries()
	claim := claimDeliveryForAttempt(t, repository)
	ctx := context.Background()
	attempt := attemptForClaim(claim, 1)

	if err := repository.StartWithLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, attempt); err != nil {
		t.Fatalf("start attempt: %v", err)
	}
	current, found, err := repository.Get(ctx, claim.Delivery.DeliveryID)
	if err != nil || !found || current.AttemptCount != 1 || current.Version != claim.Delivery.Version {
		t.Fatalf("attempt start must update count projection without changing state version: %+v found=%v err=%v", current, found, err)
	}
	listed, err := repository.ListByDelivery(ctx, claim.Delivery.DeliveryID)
	if err != nil || len(listed) != 1 || listed[0].FinishedAt != nil {
		t.Fatalf("expected one running attempt, attempts=%+v err=%v", listed, err)
	}

	finishedAt := attempt.StartedAt.Add(1500 * time.Millisecond)
	completed, err := attempt.Complete(finishedAt, domain.DeliveryAttemptAccepted, "provider-ref-1", "", false, "")
	if err != nil {
		t.Fatal(err)
	}
	repository.now = finishedAt.Add(time.Millisecond)
	if err := repository.CompleteWithLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, completed); err != nil {
		t.Fatalf("complete attempt: %v", err)
	}
	listed, err = repository.ListByDelivery(ctx, claim.Delivery.DeliveryID)
	if err != nil || len(listed) != 1 || listed[0].Outcome == nil || *listed[0].Outcome != domain.DeliveryAttemptAccepted {
		t.Fatalf("completed attempt not visible: attempts=%+v err=%v", listed, err)
	}
	if err := repository.CompleteWithLease(ctx, claim.Delivery.Version, claim.Lease.FencingToken, completed); !errors.Is(err, ErrDeliveryAttemptConflict) {
		t.Fatalf("completed attempt must not be overwritten: %v", err)
	}
}

func TestDeliveryAttemptRepositoryRejectsStaleLeaseAndVersion(t *testing.T) {
	repository := newMemoryDeliveries()
	claim := claimDeliveryForAttempt(t, repository)
	attempt := attemptForClaim(claim, 1)

	if err := repository.StartWithLease(context.Background(), claim.Delivery.Version, claim.Lease.FencingToken+1, attempt); !errors.Is(err, ErrDeliveryLeaseConflict) {
		t.Fatalf("stale fencing token must fail: %v", err)
	}
	if err := repository.StartWithLease(context.Background(), claim.Delivery.Version-1, claim.Lease.FencingToken, attempt); !errors.Is(err, ErrDeliveryVersionConflict) {
		t.Fatalf("stale delivery version must fail: %v", err)
	}
	attempt.WorkerID = "other-worker"
	if err := repository.StartWithLease(context.Background(), claim.Delivery.Version, claim.Lease.FencingToken, attempt); !errors.Is(err, ErrDeliveryLeaseConflict) {
		t.Fatalf("attempt worker must match lease owner: %v", err)
	}
	attempt = attemptForClaim(claim, 1)
	if err := repository.StartWithLease(context.Background(), claim.Delivery.Version, claim.Lease.FencingToken, attempt); !errors.Is(err, ErrDeliveryLeaseConflict) {
		t.Fatalf("expired lease must reject late start even if StartedAt is earlier: %v", err)
	}
	if err := repository.StartWithLease(context.Background(), claim.Delivery.Version, claim.Lease.FencingToken, attempt); !errors.Is(err, domain.ErrInvalidDeliveryAttempt) {
		t.Fatalf("future StartedAt must be rejected: %v", err)
	}
	if err := repository.StartWithLease(context.Background(), claim.Delivery.Version, claim.Lease.FencingToken, attempt); err != nil {
		t.Fatal(err)
	}
	completed, err := attempt.Complete(claim.Lease.ExpiresAt, domain.DeliveryAttemptAccepted, "", "", false, "")
	if err != nil {
		t.Fatal(err)
	}
	repository.now = claim.Lease.ExpiresAt
	if err := repository.CompleteWithLease(context.Background(), claim.Delivery.Version, claim.Lease.FencingToken, completed); !errors.Is(err, ErrDeliveryLeaseConflict) {
		t.Fatalf("expired lease must reject late completion: %v", err)
	}
}

func TestDeliveryAttemptRepositoryAppendIsUniqueAndFailureAtomic(t *testing.T) {
	repository := newMemoryDeliveries()
	claim := claimDeliveryForAttempt(t, repository)
	attempt := attemptForClaim(claim, 1)
	repository.failAttemptNext = true
	if err := repository.StartWithLease(context.Background(), claim.Delivery.Version, claim.Lease.FencingToken, attempt); !errors.Is(err, errInjectedRepositoryFailure) {
		t.Fatalf("expected injected start failure, got %v", err)
	}
	current, _, _ := repository.Get(context.Background(), claim.Delivery.DeliveryID)
	listed, err := repository.ListByDelivery(context.Background(), claim.Delivery.DeliveryID)
	if err != nil || current.AttemptCount != 0 || len(listed) != 0 {
		t.Fatalf("failed append must not partially update count/history: count=%d attempts=%d err=%v", current.AttemptCount, len(listed), err)
	}
	if err := repository.StartWithLease(context.Background(), claim.Delivery.Version, claim.Lease.FencingToken, attempt); err != nil {
		t.Fatal(err)
	}
	duplicate := attempt
	duplicate.AttemptID = "attempt-2"
	if err := repository.StartWithLease(context.Background(), claim.Delivery.Version, claim.Lease.FencingToken, duplicate); !errors.Is(err, ErrDeliveryAttemptConflict) {
		t.Fatalf("attempt number must be sequential and unique: %v", err)
	}
	if err := repository.StartWithLease(context.Background(), claim.Delivery.Version, claim.Lease.FencingToken, attempt); !errors.Is(err, ErrDeliveryAttemptAlreadyExists) {
		t.Fatalf("attempt ID must be unique: %v", err)
	}
}
