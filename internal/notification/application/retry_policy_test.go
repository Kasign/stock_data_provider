package application

import (
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

func retryPolicyFixture(t *testing.T, count, maxAttempts int) (domain.DeliveryRecord, []domain.DeliveryAttempt, time.Time) {
	t.Helper()
	delivery, lease, now := recoveryFixture(t)
	delivery.AttemptCount = count
	delivery.MaxAttempts = maxAttempts
	attempts := make([]domain.DeliveryAttempt, 0, count)
	for number := 1; number <= count; number++ {
		startedAt := delivery.UpdatedAt.Add(time.Duration(number) * time.Second)
		currentLease := DeliveryLease{
			WorkerID: lease.WorkerID, FencingToken: int64(number),
			ExpiresAt: startedAt.Add(time.Minute),
		}
		attempt := domain.DeliveryAttempt{
			AttemptID: fmt.Sprintf("attempt-%d", number), DeliveryID: delivery.DeliveryID,
			AttemptNo: number, WorkerID: currentLease.WorkerID,
			FencingToken: currentLease.FencingToken, LeaseExpiresAt: currentLease.ExpiresAt,
			StartedAt: startedAt,
		}
		completed, err := attempt.Complete(startedAt.Add(time.Second), domain.DeliveryAttemptRejected, "", "provider_busy", true, "")
		if err != nil {
			t.Fatal(err)
		}
		attempts = append(attempts, completed)
	}
	return delivery, attempts, now
}

func standardRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts:       3,
		MaxElapsed:        10 * time.Minute,
		PerAttemptTimeout: 5 * time.Second,
		BaseDelay:         time.Second,
		MaxDelay:          4 * time.Second,
	}
}

func TestRetryPolicyUsesCappedExponentialFullJitter(t *testing.T) {
	delivery, attempts, now := retryPolicyFixture(t, 1, 3)
	policy := standardRetryPolicy()
	got, err := policy.ScheduleRetry(delivery, attempts, now, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Retry || got.ReasonCode != "" || got.DelayCap != time.Second ||
		got.ChosenDelay != 500*time.Millisecond || got.NextAttemptAt == nil ||
		!got.NextAttemptAt.Equal(now.Add(500*time.Millisecond)) {
		t.Fatalf("unexpected first retry schedule: %+v", got)
	}
	zeroSample, err := policy.ScheduleRetry(delivery, attempts, now, 0)
	if err != nil || zeroSample.ChosenDelay != time.Microsecond {
		t.Fatalf("zero jitter must use the persisted timestamp precision floor: %+v err=%v", zeroSample, err)
	}

	delivery, attempts, now = retryPolicyFixture(t, 2, 3)
	got, err = policy.ScheduleRetry(delivery, attempts, now, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if got.DelayCap != 2*time.Second || got.ChosenDelay != time.Second {
		t.Fatalf("second attempt must double the cap: %+v", got)
	}

	delivery, attempts, now = retryPolicyFixture(t, 3, 3)
	got, err = policy.ScheduleRetry(delivery, attempts, now, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if got.Retry || got.ReasonCode != RetryReasonBudgetExhausted {
		t.Fatalf("max_attempts includes the first attempt: %+v", got)
	}
}

func TestRetryPolicyQuantizesSubMicrosecondJitter(t *testing.T) {
	delivery, attempts, now := retryPolicyFixture(t, 1, 3)
	policy := standardRetryPolicy()
	policy.BaseDelay = time.Millisecond
	policy.MaxDelay = time.Millisecond
	got, err := policy.ScheduleRetry(delivery, attempts, now, 0.0001)
	if err != nil || got.ChosenDelay != time.Microsecond {
		t.Fatalf("sub-microsecond jitter must be clamped to database precision: %+v err=%v", got, err)
	}
}

func TestRetryPolicyCapsExponentialDelayAndHonorsRetryAfter(t *testing.T) {
	delivery, attempts, now := retryPolicyFixture(t, 4, 5)
	policy := standardRetryPolicy()
	policy.MaxAttempts = 5
	retryAfter := now.Add(5 * time.Second)
	attempts[len(attempts)-1].RetryAfterAt = &retryAfter

	got, err := policy.ScheduleRetry(delivery, attempts, now, 0.9)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Retry || got.DelayCap != 4*time.Second ||
		!got.RetryAfterUsed || !got.NextAttemptAt.Equal(retryAfter) {
		t.Fatalf("Retry-After must not shorten local backoff: %+v", got)
	}

	past := now.Add(-time.Second)
	attempts[len(attempts)-1].RetryAfterAt = &past
	got, err = policy.ScheduleRetry(delivery, attempts, now, 0.25)
	if err != nil {
		t.Fatal(err)
	}
	if got.RetryAfterUsed || !got.NextAttemptAt.Equal(now.Add(time.Second)) {
		t.Fatalf("past Retry-After must not shorten local backoff: %+v", got)
	}
}

func TestRetryPolicyStopsAtDeadlineAndMaxElapsed(t *testing.T) {
	delivery, attempts, now := retryPolicyFixture(t, 1, 3)
	policy := standardRetryPolicy()
	policy.MaxElapsed = 10 * time.Minute
	delivery.DeadlineAt = now.Add(400 * time.Millisecond)
	got, err := policy.ScheduleRetry(delivery, attempts, now, 0.9)
	if err != nil {
		t.Fatal(err)
	}
	if got.Retry || got.ReasonCode != RetryReasonDeadlineExceeded {
		t.Fatalf("retry at or after Delivery deadline must fail: %+v", got)
	}

	delivery, attempts, now = retryPolicyFixture(t, 1, 3)
	policy.MaxElapsed = time.Second
	got, err = policy.ScheduleRetry(delivery, attempts, now, 0.9)
	if err != nil {
		t.Fatal(err)
	}
	if got.Retry || got.ReasonCode != RetryReasonDeadlineExceeded {
		t.Fatalf("retry at or after max elapsed cutoff must fail: %+v", got)
	}
}

func TestRetryPolicyRequiresExplicitRetryableRejectionAndCompleteHistory(t *testing.T) {
	delivery, attempts, now := retryPolicyFixture(t, 1, 3)
	policy := standardRetryPolicy()
	attempts[0].Retryable = false
	if _, err := policy.ScheduleRetry(delivery, attempts, now, 0.5); !errors.Is(err, ErrRetryNotApplicable) {
		t.Fatalf("non-retryable rejection must not be scheduled: %v", err)
	}
	attempts[0].Retryable = true
	outcome := domain.DeliveryAttemptAmbiguous
	attempts[0].Outcome = &outcome
	if _, err := policy.ScheduleRetry(delivery, attempts, now, 0.5); !errors.Is(err, ErrRetryNotApplicable) {
		t.Fatalf("ambiguous result must never be retried: %v", err)
	}
	if _, err := policy.ScheduleRetry(delivery, nil, now, 0.5); !errors.Is(err, ErrRetryNotApplicable) {
		t.Fatalf("incomplete Attempt history must fail closed: %v", err)
	}
	duplicateDelivery, duplicateAttempts, duplicateNow := retryPolicyFixture(t, 2, 3)
	duplicateAttempts[1].FencingToken = duplicateAttempts[0].FencingToken
	if _, err := policy.ScheduleRetry(duplicateDelivery, duplicateAttempts, duplicateNow, 0.5); !errors.Is(err, ErrRetryNotApplicable) {
		t.Fatalf("duplicate fencing token history must fail closed: %v", err)
	}
	descendingDelivery, descendingAttempts, descendingNow := retryPolicyFixture(t, 2, 3)
	descendingAttempts[0].FencingToken = 2
	descendingAttempts[1].FencingToken = 1
	if _, err := policy.ScheduleRetry(descendingDelivery, descendingAttempts, descendingNow, 0.5); !errors.Is(err, ErrRetryNotApplicable) {
		t.Fatalf("non-monotonic fencing token history must fail closed: %v", err)
	}
	acceptedDelivery, acceptedAttempts, acceptedNow := retryPolicyFixture(t, 2, 3)
	first := acceptedAttempts[0]
	started := first.StartedAt
	accepted := domain.DeliveryAttempt{
		AttemptID: first.AttemptID, DeliveryID: first.DeliveryID, AttemptNo: first.AttemptNo,
		WorkerID: first.WorkerID, FencingToken: first.FencingToken,
		LeaseExpiresAt: first.LeaseExpiresAt, StartedAt: started,
	}
	accepted, err := accepted.Complete(started.Add(time.Second), domain.DeliveryAttemptAccepted, "provider-accepted", "", false, "")
	if err != nil {
		t.Fatal(err)
	}
	acceptedAttempts[0] = accepted
	if _, err := policy.ScheduleRetry(acceptedDelivery, acceptedAttempts, acceptedNow, 0.5); !errors.Is(err, ErrRetryNotApplicable) {
		t.Fatalf("history containing an earlier accepted Attempt must fail closed: %v", err)
	}
}

func TestRetryPolicyValidatesDurationsAndRandomSample(t *testing.T) {
	policy := standardRetryPolicy()
	policy.BaseDelay = time.Nanosecond
	if !errors.Is(policy.Validate(), ErrInvalidRetryPolicy) {
		t.Fatal("policy durations must be positive integer milliseconds")
	}
	policy = standardRetryPolicy()
	delivery, attempts, now := retryPolicyFixture(t, 1, 3)
	for _, sample := range []float64{-0.1, 1, math.NaN(), math.Inf(1)} {
		if _, err := policy.ScheduleRetry(delivery, attempts, now, sample); !errors.Is(err, ErrRetryNotApplicable) {
			t.Fatalf("invalid random sample %v must be rejected: %v", sample, err)
		}
	}
}

func TestRetryPolicyComputesAttemptDeadline(t *testing.T) {
	policy := standardRetryPolicy()
	first := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	now := first.Add(2 * time.Second)
	deliveryDeadline := first.Add(4 * time.Second)
	got, err := policy.AttemptDeadline(now, first, deliveryDeadline)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(deliveryDeadline) {
		t.Fatalf("attempt deadline must be capped by Delivery deadline: %s", got)
	}
}
