package domain

import (
	"errors"
	"testing"
	"time"
)

func validDeliveryAttempt() DeliveryAttempt {
	started := time.Date(2026, 11, 9, 3, 0, 0, 0, time.UTC)
	return DeliveryAttempt{
		AttemptID: "attempt-1", DeliveryID: "delivery-1", AttemptNo: 1,
		WorkerID: "worker-1", FencingToken: 1,
		LeaseExpiresAt: started.Add(time.Minute), StartedAt: started,
	}
}

func TestDeliveryAttemptValidateRunningAndCompleteAccepted(t *testing.T) {
	attempt := validDeliveryAttempt()
	if err := attempt.Validate(); err != nil {
		t.Fatalf("running attempt should be valid: %v", err)
	}
	finished := attempt.StartedAt.Add(1500 * time.Millisecond)
	completed, err := attempt.Complete(finished, DeliveryAttemptAccepted, "provider-message-1", "", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Outcome == nil || *completed.Outcome != DeliveryAttemptAccepted ||
		completed.DurationMS == nil || *completed.DurationMS != 1500 ||
		completed.ProviderMessageRef == nil || *completed.ProviderMessageRef != "provider-message-1" {
		t.Fatalf("unexpected completed attempt: %+v", completed)
	}
	if attempt.FinishedAt != nil || attempt.Outcome != nil || attempt.DurationMS != nil {
		t.Fatal("Complete must not mutate the original attempt")
	}
	if err := completed.Validate(); err != nil {
		t.Fatalf("completed attempt should be valid: %v", err)
	}
	if _, err := completed.Complete(finished.Add(time.Second), DeliveryAttemptAccepted, "", "", false, ""); !errors.Is(err, ErrInvalidDeliveryAttempt) {
		t.Fatalf("completed attempt must be immutable: %v", err)
	}
}

func TestDeliveryAttemptCompletionOutcomesRequireConsistentEvidence(t *testing.T) {
	started := validDeliveryAttempt()
	at := started.StartedAt.Add(time.Second)

	if _, err := started.Complete(at, DeliveryAttemptRejected, "", "", true, ""); !errors.Is(err, ErrInvalidDeliveryAttempt) {
		t.Fatalf("rejected outcome requires error_code: %v", err)
	}
	if _, err := started.Complete(at, DeliveryAttemptAmbiguous, "", "timeout_after_write", true, ""); err != nil {
		t.Fatalf("ambiguous outcome with stable error should be valid: %v", err)
	}
	if _, err := started.Complete(at, DeliveryAttemptNotSent, "", "connection_not_started", false, ""); !errors.Is(err, ErrInvalidDeliveryAttempt) {
		t.Fatalf("not_sent without proof must fail: %v", err)
	}
	if _, err := started.Complete(at, DeliveryAttemptNotSent, "provider-message-1", "connection_not_started", false, "proof-1"); !errors.Is(err, ErrInvalidDeliveryAttempt) {
		t.Fatalf("not_sent cannot claim provider acceptance: %v", err)
	}
	if _, err := started.Complete(at, DeliveryAttemptNotSent, "", "connection_not_started", false, "proof-1"); err != nil {
		t.Fatalf("not_sent with proof should be valid: %v", err)
	}
	if _, err := started.Complete(at, DeliveryAttemptAccepted, "", "unexpected_error", false, ""); !errors.Is(err, ErrInvalidDeliveryAttempt) {
		t.Fatalf("accepted outcome cannot carry error: %v", err)
	}
}

func TestDeliveryAttemptPersistsRetryAfterOnlyForRejectedOutcome(t *testing.T) {
	attempt := validDeliveryAttempt()
	finished := attempt.StartedAt.Add(time.Second)
	retryAfter := finished.Add(30 * time.Second)
	completed, err := attempt.CompleteWithRetryAfter(finished, DeliveryAttemptRejected, "", "rate_limited", true, "", &retryAfter)
	if err != nil {
		t.Fatalf("retryable rejection with Retry-After should be valid: %v", err)
	}
	if completed.RetryAfterAt == nil || !completed.RetryAfterAt.Equal(retryAfter) {
		t.Fatalf("Retry-After timestamp was not retained: %+v", completed)
	}
	cloned := cloneDeliveryAttempt(completed)
	*cloned.RetryAfterAt = retryAfter.Add(time.Minute)
	if !completed.RetryAfterAt.Equal(retryAfter) {
		t.Fatal("Attempt clone must deep-copy Retry-After timestamp")
	}
	if _, err := attempt.CompleteWithRetryAfter(finished, DeliveryAttemptAccepted, "", "", false, "", &retryAfter); !errors.Is(err, ErrInvalidDeliveryAttempt) {
		t.Fatalf("accepted outcome cannot carry Retry-After: %v", err)
	}
}

func TestDeliveryAttemptRejectsInvalidLeaseAndTime(t *testing.T) {
	attempt := validDeliveryAttempt()
	attempt.LeaseExpiresAt = attempt.StartedAt
	if !errors.Is(attempt.Validate(), ErrInvalidDeliveryAttempt) {
		t.Fatal("lease must expire after attempt start")
	}
	attempt = validDeliveryAttempt()
	nonUTC := attempt.StartedAt.In(time.FixedZone("UTC+8", 8*60*60))
	if _, err := attempt.Complete(nonUTC.Add(time.Second), DeliveryAttemptAccepted, "", "", false, ""); !errors.Is(err, ErrInvalidDeliveryAttempt) {
		t.Fatalf("completion timestamp must be UTC: %v", err)
	}
	attempt = validDeliveryAttempt()
	if _, err := attempt.Complete(attempt.StartedAt.Add(-time.Second), DeliveryAttemptAccepted, "", "", false, ""); !errors.Is(err, ErrInvalidDeliveryAttempt) {
		t.Fatalf("completion cannot precede start: %v", err)
	}
}

func TestDeliveryAttemptCloneDoesNotShareMutableFields(t *testing.T) {
	attempt := validDeliveryAttempt()
	completed, err := attempt.Complete(attempt.StartedAt.Add(time.Second), DeliveryAttemptRejected, "", "rate_limited", true, "evidence-1")
	if err != nil {
		t.Fatal(err)
	}
	cloned := cloneDeliveryAttempt(completed)
	*cloned.ErrorCode = "changed"
	*cloned.DurationMS = 99
	if *completed.ErrorCode != "rate_limited" || *completed.DurationMS != 1000 {
		t.Fatal("clone must deep-copy nullable fields")
	}
}
