package application

import (
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

func unknownReconciliationFixture(t *testing.T) (domain.DeliveryRecord, domain.ChannelConfigSnapshot, []domain.DeliveryAttempt, UnknownDeliveryEvidence, time.Time) {
	t.Helper()
	delivery, lease, now := recoveryFixture(t)
	attempt := recoveryAttempt(t, delivery, lease, domain.DeliveryAttemptAmbiguous, true)
	delivery.AttemptCount = 1
	unknown, _, err := delivery.Transition(domain.DeliveryTransition{
		To: domain.DeliveryUnknown, At: now, ReasonCode: "channel_outcome_ambiguous",
		EvidenceRef: attempt.AttemptID,
	})
	if err != nil {
		t.Fatal(err)
	}
	channel := domain.ChannelConfigSnapshot{
		ChannelRef: unknown.ChannelRef, ConfigVersion: unknown.ChannelConfigVersion,
		ChannelKind: "test", Enabled: true,
		Capabilities: domain.ChannelCapabilities{
			SupportsIdempotency:    domain.CapabilityUnknown,
			SupportsDeliveryLookup: domain.CapabilitySupported,
			SupportsRetryAfter:     domain.CapabilityUnknown,
			MaxPayloadBytes:        4096,
		},
		RetryableErrorCodes: []string{"provider_busy"},
	}
	evidence := UnknownDeliveryEvidence{
		DeliveryID: unknown.DeliveryID, AttemptID: attempt.AttemptID,
		Outcome: UnknownLookupIndeterminate, EvidenceRef: "lookup-evidence-1",
		ObservedAt: now,
	}
	return unknown, channel, []domain.DeliveryAttempt{attempt}, evidence, now
}

func TestAssessUnknownDeliveryRequiresVerifiedLookupCapability(t *testing.T) {
	delivery, channel, attempts, evidence, now := unknownReconciliationFixture(t)
	channel.Capabilities.SupportsDeliveryLookup = domain.CapabilityUnknown
	if _, err := AssessUnknownDelivery(delivery, channel, attempts, evidence, now); !errors.Is(err, ErrUnknownReconciliationNotApplicable) {
		t.Fatalf("unverified lookup capability must be rejected, got %v", err)
	}
}

func TestAssessUnknownDeliveryAcceptedRequiresProviderReference(t *testing.T) {
	delivery, channel, attempts, evidence, now := unknownReconciliationFixture(t)
	evidence.Outcome = UnknownLookupAccepted
	if _, err := AssessUnknownDelivery(delivery, channel, attempts, evidence, now); !errors.Is(err, ErrUnknownReconciliationNotApplicable) {
		t.Fatalf("accepted result without provider reference must be rejected, got %v", err)
	}
	evidence.ProviderMessageRef = "provider-message-1"
	got, err := AssessUnknownDelivery(delivery, channel, attempts, evidence, now)
	if err != nil || got.Action != UnknownReconciliationFinalizeSent || got.EvidenceRef != evidence.EvidenceRef {
		t.Fatalf("expected evidence-bound sent assessment, got %+v err=%v", got, err)
	}
}

func TestAssessUnknownDeliveryNotAcceptedUsesRetryAllowlist(t *testing.T) {
	delivery, channel, attempts, evidence, now := unknownReconciliationFixture(t)
	evidence.Outcome = UnknownLookupNotAccepted
	evidence.ErrorCode = "provider_busy"
	got, err := AssessUnknownDelivery(delivery, channel, attempts, evidence, now)
	if err != nil || got.Action != UnknownReconciliationApplyRetryPolicy {
		t.Fatalf("retryable lookup result must go through RetryPolicy, got %+v err=%v", got, err)
	}
	evidence.ErrorCode = "invalid_recipient"
	got, err = AssessUnknownDelivery(delivery, channel, attempts, evidence, now)
	if err != nil || got.Action != UnknownReconciliationFinalizeFailed {
		t.Fatalf("permanent lookup result must finalize failed, got %+v err=%v", got, err)
	}
}

func TestAssessUnknownDeliveryIndeterminateNeverRetries(t *testing.T) {
	delivery, channel, attempts, evidence, now := unknownReconciliationFixture(t)
	got, err := AssessUnknownDelivery(delivery, channel, attempts, evidence, now)
	if err != nil || got.Action != UnknownReconciliationRemainUnknown {
		t.Fatalf("indeterminate lookup must remain unknown, got %+v err=%v", got, err)
	}
}

func TestAssessUnknownDeliveryRejectsMismatchedAttemptAndFutureEvidence(t *testing.T) {
	delivery, channel, attempts, evidence, now := unknownReconciliationFixture(t)
	evidence.AttemptID = "another-attempt"
	if _, err := AssessUnknownDelivery(delivery, channel, attempts, evidence, now); !errors.Is(err, ErrUnknownReconciliationNotApplicable) {
		t.Fatalf("mismatched attempt must be rejected, got %v", err)
	}
	evidence.AttemptID = attempts[0].AttemptID
	evidence.ObservedAt = now.Add(time.Second)
	if _, err := AssessUnknownDelivery(delivery, channel, attempts, evidence, now); !errors.Is(err, ErrUnknownReconciliationNotApplicable) {
		t.Fatalf("future evidence must be rejected, got %v", err)
	}
}

func TestAssessUnknownDeliveryRequiresContiguousUniqueAttemptNumbers(t *testing.T) {
	delivery, channel, attempts, evidence, now := unknownReconciliationFixture(t)
	delivery.AttemptCount = 2

	first := attempts[0]
	first.AttemptNo = 2
	second := first
	second.AttemptID = first.AttemptID + "-duplicate-number"
	evidence.AttemptID = second.AttemptID
	if _, err := AssessUnknownDelivery(delivery, channel, []domain.DeliveryAttempt{first, second}, evidence, now); !errors.Is(err, ErrLeaseRecoveryHistory) {
		t.Fatalf("duplicate attempt numbers must be rejected, got %v", err)
	}

	second.AttemptNo = 3
	if _, err := AssessUnknownDelivery(delivery, channel, []domain.DeliveryAttempt{first, second}, evidence, now); !errors.Is(err, ErrLeaseRecoveryHistory) {
		t.Fatalf("attempt numbers outside the delivery history must be rejected, got %v", err)
	}
}

func TestAssessUnknownDeliveryRejectsEvidenceOlderThanAttempt(t *testing.T) {
	delivery, channel, attempts, evidence, now := unknownReconciliationFixture(t)
	evidence.Outcome = UnknownLookupAccepted
	evidence.ProviderMessageRef = "provider-message-1"
	evidence.ObservedAt = attempts[0].StartedAt.Add(-time.Nanosecond)
	if _, err := AssessUnknownDelivery(delivery, channel, attempts, evidence, now); !errors.Is(err, ErrUnknownReconciliationNotApplicable) {
		t.Fatalf("lookup evidence before the channel attempt must be rejected, got %v", err)
	}
	evidence.ObservedAt = attempts[0].FinishedAt.Add(-time.Nanosecond)
	if _, err := AssessUnknownDelivery(delivery, channel, attempts, evidence, now); !errors.Is(err, ErrUnknownReconciliationNotApplicable) {
		t.Fatalf("lookup evidence before ambiguous completion must be rejected, got %v", err)
	}
}

func TestAssessUnknownDeliveryRejectsImpossibleEarlierAttemptHistory(t *testing.T) {
	delivery, channel, attempts, evidence, now := unknownReconciliationFixture(t)
	latest := attempts[0]
	first := latest
	first.AttemptID = latest.AttemptID + "-first"
	first.AttemptNo = 1
	first.FinishedAt = nil
	first.Outcome = nil
	first.ProviderMessageRef = nil
	first.ErrorCode = nil
	first.RetryAfterAt = nil
	first.DurationMS = nil
	first.EvidenceRef = nil
	first.Retryable = false
	firstFinishedAt := first.StartedAt.Add(time.Second)
	firstCompleted, err := first.Complete(firstFinishedAt, domain.DeliveryAttemptAccepted, "provider-accepted", "", false, "")
	if err != nil {
		t.Fatal(err)
	}
	latest.AttemptNo = 2
	latest.AttemptID = latest.AttemptID + "-second"
	latest.FencingToken = first.FencingToken + 1
	latest.StartedAt = firstFinishedAt.Add(time.Second)
	latest.LeaseExpiresAt = latest.StartedAt.Add(time.Minute)
	latest.FinishedAt = nil
	latest.Outcome = nil
	latest.ProviderMessageRef = nil
	latest.ErrorCode = nil
	latest.RetryAfterAt = nil
	latest.DurationMS = nil
	latest.EvidenceRef = nil
	latest.Retryable = false
	latest, err = latest.Complete(latest.StartedAt.Add(time.Second), domain.DeliveryAttemptAmbiguous, "", "timeout", false, "")
	if err != nil {
		t.Fatal(err)
	}
	delivery.AttemptCount = 2
	evidence.AttemptID = latest.AttemptID
	evidence.ObservedAt = latest.FinishedAt.Add(time.Second)
	if _, err := AssessUnknownDelivery(delivery, channel, []domain.DeliveryAttempt{firstCompleted, latest}, evidence, evidence.ObservedAt); !errors.Is(err, ErrLeaseRecoveryHistory) {
		t.Fatalf("an accepted earlier attempt must invalidate reconciliation history, got %v", err)
	}
}
