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
			SupportsIdempotency: domain.CapabilityUnknown,
			SupportsDeliveryLookup: domain.CapabilitySupported,
			SupportsRetryAfter: domain.CapabilityUnknown,
			MaxPayloadBytes: 4096,
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
