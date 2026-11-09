package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

type unknownReconciliationRepositoryFake struct {
	calls   int
	version int64
	next    domain.DeliveryRecord
	event   domain.DeliveryStateEvent
	err     error
}

func (f *unknownReconciliationRepositoryFake) ReconcileUnknown(
	_ context.Context, version int64, next domain.DeliveryRecord, event domain.DeliveryStateEvent,
) error {
	f.calls++
	f.version, f.next, f.event = version, next, event
	return f.err
}

func TestPersistUnknownReconciliationAcceptedAndFailed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		action UnknownReconciliationAction
		status domain.DeliveryStatus
	}{
		{"accepted", UnknownReconciliationFinalizeSent, domain.DeliverySent},
		{"permanent rejection", UnknownReconciliationFinalizeFailed, domain.DeliveryFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			delivery, _, _, evidence, now := unknownReconciliationFixture(t)
			assessment, err := AssessUnknownDelivery(delivery, func() domain.ChannelConfigSnapshot {
				_, channel, _, _, _ := unknownReconciliationFixture(t)
				return channel
			}(), func() []domain.DeliveryAttempt {
				_, _, attempts, _, _ := unknownReconciliationFixture(t)
				return attempts
			}(), evidence, now)
			if err != nil { t.Fatal(err) }
			assessment.Action = tc.action
			assessment.ReasonCode = "channel_lookup_confirmed_permanent_rejection"
			if tc.status == domain.DeliverySent {
				assessment.ReasonCode = "channel_lookup_confirmed_accepted"
				assessment.ProviderMessageRef = "provider-message-1"
			} else {
				assessment.ErrorCode = "invalid_recipient"
			}
			repository := &unknownReconciliationRepositoryFake{}
			if err := PersistUnknownReconciliation(context.Background(), repository, delivery, assessment, now, nil); err != nil {
				t.Fatal(err)
			}
			if repository.calls != 1 || repository.version != delivery.Version ||
				repository.next.Status != tc.status || repository.event.EvidenceRef == nil ||
				*repository.event.EvidenceRef != evidence.EvidenceRef {
				t.Fatalf("unexpected persisted transition: %+v", repository)
			}
		})
	}
}

func TestPersistUnknownReconciliationIndeterminateDoesNotWrite(t *testing.T) {
	delivery, channel, attempts, evidence, now := unknownReconciliationFixture(t)
	assessment, err := AssessUnknownDelivery(delivery, channel, attempts, evidence, now)
	if err != nil { t.Fatal(err) }
	repository := &unknownReconciliationRepositoryFake{}
	if err := PersistUnknownReconciliation(context.Background(), repository, delivery, assessment, now, nil); err != nil {
		t.Fatal(err)
	}
	if repository.calls != 0 { t.Fatalf("indeterminate evidence must not write, calls=%d", repository.calls) }
}

func TestPersistUnknownReconciliationRetryRequiresBoundDecision(t *testing.T) {
	delivery, channel, attempts, evidence, now := unknownReconciliationFixture(t)
	evidence.Outcome = UnknownLookupNotAccepted
	evidence.ErrorCode = "provider_busy"
	assessment, err := AssessUnknownDelivery(delivery, channel, attempts, evidence, now)
	if err != nil { t.Fatal(err) }
	nextAt := now.Add(time.Second)
	decision := &RetryDecision{
		DeliveryID: delivery.DeliveryID, DeliveryVersion: delivery.Version,
		AttemptID: assessment.AttemptID, FencingToken: assessment.FencingToken,
		EvaluatedAt: now, Retry: true, NextAttemptAt: &nextAt,
	}
	repository := &unknownReconciliationRepositoryFake{}
	if err := PersistUnknownReconciliation(context.Background(), repository, delivery, assessment, now, decision); err != nil {
		t.Fatal(err)
	}
	if repository.calls != 1 || repository.next.Status != domain.DeliveryRetryWait ||
		repository.event.EvidenceRef == nil || *repository.event.EvidenceRef != evidence.EvidenceRef {
		t.Fatalf("retry transition did not preserve evidence: %+v", repository)
	}
	decision.AttemptID = "different-attempt"
	repository = &unknownReconciliationRepositoryFake{}
	if err := PersistUnknownReconciliation(context.Background(), repository, delivery, assessment, now, decision); !errors.Is(err, ErrRetryDecisionConflict) {
		t.Fatalf("mismatched retry decision must fail, got %v", err)
	}
	if repository.calls != 0 { t.Fatal("mismatched retry decision reached repository") }
}

func TestPersistUnknownReconciliationPropagatesTransactionFailure(t *testing.T) {
	delivery, channel, attempts, evidence, now := unknownReconciliationFixture(t)
	evidence.Outcome = UnknownLookupAccepted
	evidence.ProviderMessageRef = "provider-message-1"
	assessment, err := AssessUnknownDelivery(delivery, channel, attempts, evidence, now)
	if err != nil { t.Fatal(err) }
	repository := &unknownReconciliationRepositoryFake{err: errors.New("transaction failed")}
	if err := PersistUnknownReconciliation(context.Background(), repository, delivery, assessment, now, nil); err == nil {
		t.Fatal("repository transaction failure must be propagated")
	}
}
