package application

import (
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

func recoveryFixture(t *testing.T) (domain.DeliveryRecord, DeliveryLease, time.Time) {
	t.Helper()
	delivery := validDeliveryForRepository()
	claimedAt := delivery.CreatedAt.Add(time.Second)
	sending, _, err := delivery.Transition(domain.DeliveryTransition{To: domain.DeliverySending, At: claimedAt})
	if err != nil {
		t.Fatal(err)
	}
	lease := DeliveryLease{WorkerID: "worker-1", FencingToken: 1, ExpiresAt: claimedAt.Add(time.Minute)}
	now := lease.ExpiresAt.Add(time.Second)
	return sending, lease, now
}

func recoveryAttempt(t *testing.T, delivery domain.DeliveryRecord, lease DeliveryLease, outcome domain.DeliveryAttemptOutcome, finished bool) domain.DeliveryAttempt {
	t.Helper()
	startedAt := delivery.UpdatedAt.Add(time.Second)
	attempt := domain.DeliveryAttempt{
		AttemptID: "attempt-1", DeliveryID: delivery.DeliveryID, AttemptNo: 1,
		WorkerID: lease.WorkerID, FencingToken: lease.FencingToken,
		LeaseExpiresAt: lease.ExpiresAt, StartedAt: startedAt,
	}
	if !finished {
		return attempt
	}
	finishedAt := startedAt.Add(time.Second)
	providerRef, errorCode, evidenceRef := "", "", ""
	retryable := false
	switch outcome {
	case domain.DeliveryAttemptRejected:
		errorCode, retryable = "provider_busy", true
	case domain.DeliveryAttemptAmbiguous:
		errorCode = "request_timeout"
	case domain.DeliveryAttemptNotSent:
		errorCode, evidenceRef = "connection_not_opened", "evidence-not-sent-1"
	}
	completed, err := attempt.Complete(finishedAt, outcome, providerRef, errorCode, retryable, evidenceRef)
	if err != nil {
		t.Fatal(err)
	}
	return completed
}

func TestAssessExpiredDeliveryLeaseWithoutAttemptIsSafeOnlyWithinBudget(t *testing.T) {
	delivery, lease, now := recoveryFixture(t)
	got, err := AssessExpiredDeliveryLease(delivery, lease, nil, now)
	if err != nil || got.Action != LeaseRecoverySafeToRetry || got.ReasonCode != "lease_expired_before_attempt" {
		t.Fatalf("expected safe retry without an Attempt, got %+v err=%v", got, err)
	}

	previousLease := DeliveryLease{WorkerID: "worker-1", FencingToken: 1, ExpiresAt: delivery.UpdatedAt.Add(30 * time.Second)}
	previous := domain.DeliveryAttempt{AttemptID: "attempt-previous", DeliveryID: delivery.DeliveryID, AttemptNo: 1,
		WorkerID: previousLease.WorkerID, FencingToken: previousLease.FencingToken,
		LeaseExpiresAt: previousLease.ExpiresAt, StartedAt: delivery.UpdatedAt.Add(time.Second)}
	previousFinished := previous.StartedAt.Add(time.Second)
	previous, err = previous.Complete(previousFinished, domain.DeliveryAttemptNotSent, "", "connection_not_opened", false, "evidence-previous")
	if err != nil { t.Fatal(err) }
	delivery.AttemptCount = 1
	delivery.MaxAttempts = 1
	lease.FencingToken = 2
	got, err = AssessExpiredDeliveryLease(delivery, lease, []domain.DeliveryAttempt{previous}, now)
	if err != nil || got.Action != LeaseRecoveryFinalizeFailed || got.ReasonCode != "retry_budget_exhausted" {
		t.Fatalf("expected exhausted budget to fail, got %+v err=%v", got, err)
	}
}

func TestAssessExpiredDeliveryLeaseClassifiesAttemptOutcomes(t *testing.T) {
	cases := []struct {
		name string
		outcome domain.DeliveryAttemptOutcome
		finished bool
		retryable bool
		want LeaseRecoveryAction
	}{
		{name: "running attempt is unknown", finished: false, want: LeaseRecoveryMarkUnknown},
		{name: "accepted is finalized as sent", outcome: domain.DeliveryAttemptAccepted, finished: true, want: LeaseRecoveryFinalizeSent},
		{name: "retryable rejection requires retry policy", outcome: domain.DeliveryAttemptRejected, finished: true, retryable: true, want: LeaseRecoveryApplyRetryPolicy},
		{name: "permanent rejection is failed", outcome: domain.DeliveryAttemptRejected, finished: true, want: LeaseRecoveryFinalizeFailed},
		{name: "ambiguous is unknown", outcome: domain.DeliveryAttemptAmbiguous, finished: true, want: LeaseRecoveryMarkUnknown},
		{name: "proved not sent is safe to retry", outcome: domain.DeliveryAttemptNotSent, finished: true, want: LeaseRecoverySafeToRetry},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			delivery, lease, now := recoveryFixture(t)
			attempt := recoveryAttempt(t, delivery, lease, tc.outcome, tc.finished)
			if tc.retryable {
				attempt.Retryable = true
			}
			got, err := AssessExpiredDeliveryLease(delivery, lease, []domain.DeliveryAttempt{attempt}, now)
			if err != nil || got.Action != tc.want {
				t.Fatalf("want %s, got %+v err=%v", tc.want, got, err)
			}
			if tc.outcome == domain.DeliveryAttemptNotSent && got.EvidenceRef != "evidence-not-sent-1" {
				t.Fatalf("not_sent evidence must be retained, got %+v", got)
			}
		})
	}
}

func TestAssessExpiredDeliveryLeaseRejectsIncompleteOrMismatchedHistory(t *testing.T) {
	delivery, lease, now := recoveryFixture(t)
	attempt := recoveryAttempt(t, delivery, lease, domain.DeliveryAttemptAccepted, true)
	delivery.AttemptCount = 1
	if _, err := AssessExpiredDeliveryLease(delivery, lease, nil, now); !errors.Is(err, ErrLeaseRecoveryHistory) {
		t.Fatalf("missing history must fail closed, got %v", err)
	}
	lease.WorkerID = "another-worker"
	if _, err := AssessExpiredDeliveryLease(delivery, lease, []domain.DeliveryAttempt{attempt}, now); !errors.Is(err, ErrLeaseRecoveryHistory) {
		t.Fatalf("lease/attempt mismatch must fail closed, got %v", err)
	}
}

func TestAssessExpiredDeliveryLeaseRequiresExpiredUTCLease(t *testing.T) {
	delivery, lease, now := recoveryFixture(t)
	if _, err := AssessExpiredDeliveryLease(delivery, lease, nil, lease.ExpiresAt.Add(-time.Second)); !errors.Is(err, ErrLeaseRecoveryNotApplicable) {
		t.Fatalf("active lease must not be recovered, got %v", err)
	}
	local := now.In(time.FixedZone("local", 8*60*60))
	if _, err := AssessExpiredDeliveryLease(delivery, lease, nil, local); !errors.Is(err, ErrLeaseRecoveryNotApplicable) {
		t.Fatalf("non-UTC clock must be rejected, got %v", err)
	}
}

func TestAssessExpiredDeliveryLeaseDoesNotExposeProviderDetails(t *testing.T) {
	delivery, lease, now := recoveryFixture(t)
	attempt := recoveryAttempt(t, delivery, lease, domain.DeliveryAttemptRejected, true)
	attempt.ErrorCode = stringPointerForRecovery("provider returned private response: " + strings.Repeat("x", 4))
	if _, err := AssessExpiredDeliveryLease(delivery, lease, []domain.DeliveryAttempt{attempt}, now); !errors.Is(err, ErrLeaseRecoveryHistory) {
		t.Fatalf("invalid attempt error detail must fail closed, got %v", err)
	}
}

func stringPointerForRecovery(value string) *string { return &value }
