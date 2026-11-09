package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

func completeRejectedAttemptForRetry(t *testing.T, repository *memoryDeliveries, claim DeliveryClaim) domain.DeliveryAttempt {
	t.Helper()
	attempt := attemptForClaim(claim, 1)
	if err := repository.StartWithLease(context.Background(), claim.Delivery.Version, claim.Lease.FencingToken, attempt); err != nil {
		t.Fatal(err)
	}
	finishedAt := attempt.StartedAt.Add(1500 * time.Millisecond)
	completed, err := attempt.Complete(finishedAt, domain.DeliveryAttemptRejected, "", "provider_busy", true, "")
	if err != nil {
		t.Fatal(err)
	}
	repository.now = finishedAt.Add(time.Millisecond)
	if err := repository.CompleteWithLease(context.Background(), claim.Delivery.Version, claim.Lease.FencingToken, completed); err != nil {
		t.Fatal(err)
	}
	return completed
}

func TestApplyRetryDecisionAtomicallySchedulesRetryAndReleasesLease(t *testing.T) {
	repository := newMemoryDeliveries()
	claim := claimDeliveryForAttempt(t, repository)
	attempt := completeRejectedAttemptForRetry(t, repository, claim)
	delivery, found, err := repository.Get(context.Background(), claim.Delivery.DeliveryID)
	if err != nil || !found {
		t.Fatalf("read delivery after attempt completion: found=%v err=%v", found, err)
	}
	history, err := repository.ListByDelivery(context.Background(), delivery.DeliveryID)
	if err != nil {
		t.Fatal(err)
	}
	now := repository.now
	policy := standardRetryPolicy()
	decision, err := policy.ScheduleRetry(delivery, history, now, 0.5)
	if err != nil || !decision.Retry {
		t.Fatalf("expected retry decision, got %+v err=%v", decision, err)
	}

	claim.Delivery = delivery
	if err := ApplyRetryDecision(context.Background(), repository, claim, attempt, decision, now); err != nil {
		t.Fatalf("persist retry decision: %v", err)
	}
	updated, found, err := repository.Get(context.Background(), delivery.DeliveryID)
	if err != nil || !found || updated.Status != domain.DeliveryRetryWait ||
		updated.NextAttemptAt == nil || !updated.NextAttemptAt.Equal(*decision.NextAttemptAt) ||
		updated.Version != delivery.Version+1 || updated.AttemptCount != 1 {
		t.Fatalf("retry state was not persisted atomically: %+v found=%v err=%v", updated, found, err)
	}
	if _, exists := repository.leases[delivery.DeliveryID]; exists {
		t.Fatal("retry_wait transition must release the old sending lease")
	}
	events := repository.StateEvents(delivery.DeliveryID)
	if len(events) != 2 || events[1].To != domain.DeliveryRetryWait || events[1].ReasonCode == nil ||
		*events[1].ReasonCode != "retry_scheduled" {
		t.Fatalf("retry transition event missing: %+v", events)
	}
}

func TestApplyRetryDecisionFinalizesExhaustedBudgetAndReleasesLease(t *testing.T) {
	repository := newMemoryDeliveries()
	delivery := validDeliveryForRepository()
	delivery.MaxAttempts = 1
	if err := repository.Create(context.Background(), delivery); err != nil {
		t.Fatal(err)
	}
	claim, claimed, err := repository.ClaimDue(context.Background(), "worker-final", delivery.CreatedAt.Add(time.Second), delivery.CreatedAt.Add(time.Minute))
	if err != nil || !claimed {
		t.Fatalf("claim delivery: claimed=%v err=%v", claimed, err)
	}
	repository.now = claim.Lease.ExpiresAt.Add(-time.Second)
	attempt := completeRejectedAttemptForRetry(t, repository, claim)
	current, found, err := repository.Get(context.Background(), delivery.DeliveryID)
	if err != nil || !found {
		t.Fatalf("read delivery: found=%v err=%v", found, err)
	}
	history, err := repository.ListByDelivery(context.Background(), delivery.DeliveryID)
	if err != nil {
		t.Fatal(err)
	}
	policy := standardRetryPolicy()
	policy.MaxAttempts = 1
	decision, err := policy.ScheduleRetry(current, history, repository.now, 0.5)
	if err != nil || decision.Retry || decision.ReasonCode != RetryReasonBudgetExhausted {
		t.Fatalf("expected exhausted retry budget: %+v err=%v", decision, err)
	}
	claim.Delivery = current
	if err := ApplyRetryDecision(context.Background(), repository, claim, attempt, decision, repository.now); err != nil {
		t.Fatalf("persist exhausted retry decision: %v", err)
	}
	updated, found, err := repository.Get(context.Background(), delivery.DeliveryID)
	if err != nil || !found || updated.Status != domain.DeliveryFailed ||
		updated.ReasonCode == nil || *updated.ReasonCode != RetryReasonBudgetExhausted {
		t.Fatalf("exhausted delivery must become failed: %+v found=%v err=%v", updated, found, err)
	}
	if _, exists := repository.leases[delivery.DeliveryID]; exists {
		t.Fatal("failed transition must release the old sending lease")
	}
}

func TestApplyRetryDecisionRejectsStaleOrAmbiguousInputs(t *testing.T) {
	repository := newMemoryDeliveries()
	claim := claimDeliveryForAttempt(t, repository)
	attempt := completeRejectedAttemptForRetry(t, repository, claim)
	delivery, _, _ := repository.Get(context.Background(), claim.Delivery.DeliveryID)
	claim.Delivery = delivery
	now := repository.now
	validDecision := RetryDecision{Retry: true, NextAttemptAt: timePointerForRetry(now.Add(time.Second))}
	claim.Lease.FencingToken++
	if err := ApplyRetryDecision(context.Background(), repository, claim, attempt, validDecision, now); !errors.Is(err, ErrRetryDecisionConflict) {
		t.Fatalf("stale fencing token must fail: %v", err)
	}
	claim.Lease.FencingToken--
	invalidDecision := RetryDecision{Retry: false, ReasonCode: "unknown"}
	if err := ApplyRetryDecision(context.Background(), repository, claim, attempt, invalidDecision, now); !errors.Is(err, ErrRetryDecisionConflict) {
		t.Fatalf("unknown must never be scheduled as retry failure: %v", err)
	}
}

func timePointerForRetry(value time.Time) *time.Time { return &value }
