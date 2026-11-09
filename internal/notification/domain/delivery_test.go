package domain

import (
	"errors"
	"testing"
	"time"
)

func validDeliveryRecord() DeliveryRecord {
	now := time.Date(2026, 11, 9, 2, 0, 0, 0, time.UTC)
	return DeliveryRecord{
		DeliveryID: "delivery-1", SourceModule: "researchflow", EventID: "event-1",
		SubscriptionID: "subscription-1", SubscriptionVersion: 1,
		ChannelRef: "channel-1", ChannelConfigVersion: 1,
		TemplateID: "template-1", TemplateVersion: 1, RecipientRef: "recipient-1",
		PayloadFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		PlanFingerprint: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		DeliveryIdempotencyKey: "delivery-key-1", Status: DeliveryQueued, Version: 1,
		AttemptCount: 0, MaxAttempts: 3, DeadlineAt: now.Add(time.Hour),
		CreatedAt: now, UpdatedAt: now,
	}
}

func TestDeliveryTransitionIncrementsVersionAndAppendsEvent(t *testing.T) {
	record := validDeliveryRecord()
	next, event, err := record.Transition(DeliveryTransition{
		To: DeliverySending, At: record.UpdatedAt.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.Status != DeliverySending || next.Version != 2 {
		t.Fatalf("unexpected next state: status=%s version=%d", next.Status, next.Version)
	}
	if event.From != DeliveryQueued || event.To != DeliverySending || event.Version != next.Version {
		t.Fatalf("unexpected state event: %+v", event)
	}
	if record.Status != DeliveryQueued || record.Version != 1 {
		t.Fatal("transition mutated the original delivery")
	}
}

func TestDeliveryTransitionRejectsUnsafeSkippedAndTerminalChanges(t *testing.T) {
	record := validDeliveryRecord()
	sending, _, err := record.Transition(DeliveryTransition{
		To: DeliverySending, At: record.UpdatedAt.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := sending.Transition(DeliveryTransition{
		To: DeliverySkipped, At: sending.UpdatedAt.Add(time.Second), ReasonCode: "subscription_revoked",
	}); !errors.Is(err, ErrInvalidDeliveryChange) {
		t.Fatalf("sending->skipped without proof must fail, got %v", err)
	}
	sent, _, err := sending.Transition(DeliveryTransition{
		To: DeliverySent, At: sending.UpdatedAt.Add(time.Second), ProviderMessageRef: "provider-message-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := sent.Transition(DeliveryTransition{
		To: DeliveryQueued, At: sent.UpdatedAt.Add(time.Second),
	}); !errors.Is(err, ErrInvalidDeliveryChange) {
		t.Fatalf("sent must be terminal, got %v", err)
	}
}

func TestDeliveryUnknownRequiresReconciliationEvidence(t *testing.T) {
	record := validDeliveryRecord()
	sending, _, err := record.Transition(DeliveryTransition{
		To: DeliverySending, At: record.UpdatedAt.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	unknown, _, err := sending.Transition(DeliveryTransition{
		To: DeliveryUnknown, At: sending.UpdatedAt.Add(time.Second), ReasonCode: "outcome_ambiguous",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := unknown.Transition(DeliveryTransition{
		To: DeliverySent, At: unknown.UpdatedAt.Add(time.Second),
	}); !errors.Is(err, ErrInvalidDeliveryChange) {
		t.Fatalf("unknown without evidence must not converge, got %v", err)
	}
	reconciled, event, err := unknown.Transition(DeliveryTransition{
		To: DeliverySent, At: unknown.UpdatedAt.Add(time.Second),
		EvidenceRef: "reconcile-1", ProviderMessageRef: "provider-message-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if reconciled.Status != DeliverySent || event.EvidenceRef == nil || *event.EvidenceRef != "reconcile-1" {
		t.Fatalf("reconciliation evidence not retained: %+v", event)
	}
}

func TestDeliveryRetryWaitRequiresBoundedFutureTime(t *testing.T) {
	record := validDeliveryRecord()
	sending, _, err := record.Transition(DeliveryTransition{
		To: DeliverySending, At: record.UpdatedAt.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	next := sending.UpdatedAt.Add(time.Minute)
	retry, _, err := sending.Transition(DeliveryTransition{
		To: DeliveryRetryWait, At: sending.UpdatedAt.Add(time.Second),
		ReasonCode: "provider_retryable", ErrorCode: "rate_limited", NextAttemptAt: &next,
	})
	if err != nil {
		t.Fatal(err)
	}
	if retry.NextAttemptAt == nil || !retry.NextAttemptAt.Equal(next) {
		t.Fatal("retry_wait must retain the scheduled next attempt")
	}
	if _, _, err := retry.Transition(DeliveryTransition{
		To: DeliverySending, At: retry.UpdatedAt.Add(time.Second),
	}); !errors.Is(err, ErrInvalidDeliveryChange) {
		t.Fatalf("retry must not be claimed before next_attempt_at, got %v", err)
	}
	outsideDeadline := record.DeadlineAt
	if _, _, err := sending.Transition(DeliveryTransition{
		To: DeliveryRetryWait, At: sending.UpdatedAt.Add(time.Second),
		ReasonCode: "provider_retryable", ErrorCode: "rate_limited", NextAttemptAt: &outsideDeadline,
	}); !errors.Is(err, ErrInvalidDeliveryChange) {
		t.Fatalf("retry at/after deadline must fail, got %v", err)
	}
}

func TestDeliveryManualRetryRequiresAuditedAuthorization(t *testing.T) {
	record := validDeliveryRecord()
	sending, _, err := record.Transition(DeliveryTransition{
		To: DeliverySending, At: record.UpdatedAt.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	failed, _, err := sending.Transition(DeliveryTransition{
		To: DeliveryFailed, At: sending.UpdatedAt.Add(time.Second), ReasonCode: "permanent_rejection",
	})
	if err != nil {
		t.Fatal(err)
	}
	change := DeliveryTransition{To: DeliveryQueued, At: failed.UpdatedAt.Add(time.Second)}
	if _, _, err := failed.Transition(change); !errors.Is(err, ErrInvalidDeliveryChange) {
		t.Fatalf("unauthorized manual retry must fail, got %v", err)
	}
	change.AuthorizedManualRetry = true
	change.ActorRef = "owner-1"
	change.RequestID = "request-1"
	change.ManualRetryReason = "修复渠道配置后人工重试"
	next, event, err := failed.Transition(change)
	if err != nil {
		t.Fatal(err)
	}
	if next.Status != DeliveryQueued || event.ActorRef == nil || event.RequestID == nil {
		t.Fatalf("manual retry audit data missing: %+v", event)
	}
}

func TestDeliveryValidationRejectsInconsistentNullableFields(t *testing.T) {
	record := validDeliveryRecord()
	next := record.UpdatedAt.Add(time.Minute)
	record.NextAttemptAt = &next
	if !errors.Is(record.Validate(), ErrInvalidDelivery) {
		t.Fatal("non-retry_wait delivery must reject next_attempt_at")
	}

	record = validDeliveryRecord()
	nonUTC := record.UpdatedAt.In(time.FixedZone("UTC+8", 8*60*60))
	record.UpdatedAt = nonUTC
	if !errors.Is(record.Validate(), ErrInvalidDelivery) {
		t.Fatal("non-UTC timestamps must be rejected")
	}
}
