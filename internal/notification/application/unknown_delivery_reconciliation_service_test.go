package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

func TestVerifiedUnknownDeliveryEvidenceBindsSnapshotIdentity(t *testing.T) {
	delivery, channel, attempts, evidence, _ := unknownReconciliationFixture(t)
	verified := verifiedUnknownDeliveryEvidence{
		evidence: evidence, deliveryVersion: delivery.Version,
		attemptID: attempts[0].AttemptID, channel: cloneChannelSnapshot(channel),
	}
	if !verified.matches(delivery, channel, attempts) {
		t.Fatal("evidence from the exact registry lookup snapshot should match")
	}

	changedDelivery := delivery
	changedDelivery.Version++
	if verified.matches(changedDelivery, channel, attempts) {
		t.Fatal("evidence must not be reusable against a different delivery version")
	}
	changedChannel := cloneChannelSnapshot(channel)
	changedChannel.Enabled = !changedChannel.Enabled
	if verified.matches(delivery, changedChannel, attempts) {
		t.Fatal("evidence must not be reusable against a different channel snapshot")
	}
	changedAttempts := append([]domain.DeliveryAttempt(nil), attempts...)
	changedAttempts[0].AttemptID += "-other"
	if verified.matches(delivery, channel, changedAttempts) {
		t.Fatal("evidence must not be reusable against a different attempt")
	}
}

type unknownReconciliationLookupAdapter struct {
	*fakeChannelAdapter
	outcome    domain.ChannelLookupOutcome
	lookupErr  error
	lookupCalls int
	cancel     context.CancelFunc
}

func (a *unknownReconciliationLookupAdapter) LookupDelivery(
	_ context.Context, request domain.ChannelDeliveryLookupRequest,
) (domain.ChannelDeliveryLookupResult, error) {
	a.lookupCalls++
	if a.cancel != nil {
		a.cancel()
	}
	if a.lookupErr != nil {
		return domain.ChannelDeliveryLookupResult{}, a.lookupErr
	}
	result := domain.ChannelDeliveryLookupResult{
		ChannelRef: request.ChannelRef, ConfigVersion: request.ConfigVersion,
		DeliveryID: request.DeliveryID, AttemptID: request.AttemptID,
		Outcome: a.outcome, EvidenceRef: "service-lookup-evidence",
		ObservedAt: time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC),
	}
	switch a.outcome {
	case domain.ChannelLookupAccepted:
		result.ProviderMessageRef = "provider-message-service"
	case domain.ChannelLookupNotAccepted:
		result.ErrorCode = "invalid_recipient"
	}
	return result, nil
}

func unknownReconciliationServiceFixture(t *testing.T, outcome domain.ChannelLookupOutcome) (
	*UnknownDeliveryReconciliationService, *unknownReconciliationLookupAdapter,
	*unknownReconciliationRepositoryFake, time.Time,
) {
	t.Helper()
	base := validDeliveryForRepository()
	sendingAt := base.CreatedAt.Add(time.Second)
	sending, sendingEvent, err := base.Transition(domain.DeliveryTransition{
		To: domain.DeliverySending, At: sendingAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	lease := DeliveryLease{
		WorkerID: "worker-service", FencingToken: 1,
		ExpiresAt: sendingAt.Add(time.Minute),
	}
	attempt := domain.DeliveryAttempt{
		AttemptID: "attempt-service", DeliveryID: sending.DeliveryID,
		AttemptNo: 1, WorkerID: lease.WorkerID, FencingToken: lease.FencingToken,
		LeaseExpiresAt: lease.ExpiresAt, StartedAt: sendingAt.Add(time.Second),
	}
	finishedAt := attempt.StartedAt.Add(time.Second)
	attempt, err = attempt.Complete(finishedAt, domain.DeliveryAttemptAmbiguous, "", "request_timeout", false, "")
	if err != nil {
		t.Fatal(err)
	}
	sending.AttemptCount = 1
	unknownAt := finishedAt.Add(time.Second)
	unknown, unknownEvent, err := sending.Transition(domain.DeliveryTransition{
		To: domain.DeliveryUnknown, At: unknownAt,
		ReasonCode: "channel_outcome_ambiguous", EvidenceRef: attempt.AttemptID,
	})
	if err != nil {
		t.Fatal(err)
	}
	channel := domain.ChannelConfigSnapshot{
		ChannelRef: unknown.ChannelRef, ConfigVersion: unknown.ChannelConfigVersion,
		ChannelKind: "test", Enabled: true,
		Capabilities: domain.ChannelCapabilities{
			SupportsIdempotency: domain.CapabilitySupported,
			SupportsDeliveryLookup: domain.CapabilitySupported,
			SupportsRetryAfter: domain.CapabilityUnsupported,
			MaxPayloadBytes: 4096,
		},
	}
	adapter := &unknownReconciliationLookupAdapter{
		fakeChannelAdapter: &fakeChannelAdapter{snapshot: channel},
		outcome: outcome,
	}
	registry := NewChannelRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	reader := consistencySnapshotReaderFake{
		snapshot: DeliveryConsistencySnapshot{
			Delivery: unknown, Attempts: []domain.DeliveryAttempt{attempt},
			StateEvents: []domain.DeliveryStateEvent{sendingEvent, unknownEvent},
		},
		found: true,
	}
	repository := &unknownReconciliationRepositoryFake{}
	service, err := NewUnknownDeliveryReconciliationService(
		reader, registry, repository, standardRetryPolicy(),
	)
	if err != nil {
		t.Fatal(err)
	}
	now := unknownAt.Add(2 * time.Second)
	return service, adapter, repository, now
}

func TestUnknownDeliveryReconciliationServiceFailsClosedBeforeLookup(t *testing.T) {
	readFailure := errors.New("snapshot read failed")
	for _, tc := range []struct {
		name string
		reader consistencySnapshotReaderFake
		want error
	}{
		{
			name: "reader error",
			reader: consistencySnapshotReaderFake{err: readFailure},
			want: readFailure,
		},
		{
			name: "delivery missing",
			reader: consistencySnapshotReaderFake{},
			want: ErrUnknownDeliverySnapshotUnavailable,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, adapter, repository, now := unknownReconciliationServiceFixture(t, domain.ChannelLookupIndeterminate)
			service, err := NewUnknownDeliveryReconciliationService(
				tc.reader, serviceRegistryForTest(t, adapter), repository, standardRetryPolicy(),
			)
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = service.Reconcile(context.Background(), "delivery-service", now, 0.5)
			if !errors.Is(err, tc.want) || adapter.lookupCalls != 0 || repository.calls != 0 {
				t.Fatalf("must fail before provider lookup or write: calls=%d writes=%d err=%v", adapter.lookupCalls, repository.calls, err)
			}
		})
	}
}

func serviceRegistryForTest(t *testing.T, adapter *unknownReconciliationLookupAdapter) *ChannelRegistry {
	t.Helper()
	registry := NewChannelRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	return registry
}

func TestUnknownDeliveryReconciliationServiceRejectsIncompleteHistory(t *testing.T) {
	service, adapter, repository, now := unknownReconciliationServiceFixture(t, domain.ChannelLookupIndeterminate)
	reader := service.reader.(consistencySnapshotReaderFake)
	reader.snapshot.StateEvents = reader.snapshot.StateEvents[:1]
	service.reader = reader

	_, _, err := service.Reconcile(context.Background(), reader.snapshot.Delivery.DeliveryID, now, 0.5)
	if !errors.Is(err, ErrUnknownDeliverySnapshotNotConsistent) ||
		adapter.lookupCalls != 0 || repository.calls != 0 {
		t.Fatalf("incomplete state history must fail closed: lookups=%d writes=%d err=%v",
			adapter.lookupCalls, repository.calls, err)
	}
}

func TestUnknownDeliveryReconciliationServiceDoesNotWriteOnLookupError(t *testing.T) {
	service, adapter, repository, now := unknownReconciliationServiceFixture(t, domain.ChannelLookupIndeterminate)
	adapter.lookupErr = errors.New("provider lookup timeout")

	_, _, err := service.Reconcile(context.Background(), "delivery-service", now, 0.5)
	if err == nil || adapter.lookupCalls != 1 || repository.calls != 0 {
		t.Fatalf("lookup failure must not reach persistence: lookups=%d writes=%d err=%v",
			adapter.lookupCalls, repository.calls, err)
	}
}

func TestUnknownDeliveryReconciliationServiceKeepsIndeterminateUnknown(t *testing.T) {
	service, adapter, repository, now := unknownReconciliationServiceFixture(t, domain.ChannelLookupIndeterminate)

	assessment, decision, err := service.Reconcile(context.Background(), "delivery-service", now, 0.5)
	if err != nil || assessment.Action != UnknownReconciliationRemainUnknown ||
		decision != nil || adapter.lookupCalls != 1 || repository.calls != 0 {
		t.Fatalf("indeterminate must remain unknown without CAS: assessment=%+v decision=%+v lookups=%d writes=%d err=%v",
			assessment, decision, adapter.lookupCalls, repository.calls, err)
	}
}

func TestUnknownDeliveryReconciliationServiceStopsAfterLookupCancellation(t *testing.T) {
	service, adapter, repository, now := unknownReconciliationServiceFixture(t, domain.ChannelLookupIndeterminate)
	ctx, cancel := context.WithCancel(context.Background())
	adapter.cancel = cancel

	_, _, err := service.Reconcile(ctx, "delivery-service", now, 0.5)
	if !errors.Is(err, context.Canceled) || adapter.lookupCalls != 1 || repository.calls != 0 {
		t.Fatalf("cancellation after lookup must prevent CAS: lookups=%d writes=%d err=%v",
			adapter.lookupCalls, repository.calls, err)
	}
}

func TestUnknownDeliveryReconciliationServiceDoesNotReplayEvidenceAfterCASConflict(t *testing.T) {
	service, adapter, repository, now := unknownReconciliationServiceFixture(t, domain.ChannelLookupAccepted)
	repository.err = ErrUnknownReconciliationConflict

	assessment, decision, err := service.Reconcile(context.Background(), "delivery-service", now, 0.5)
	if !errors.Is(err, ErrUnknownReconciliationConflict) ||
		assessment.Action != UnknownReconciliationFinalizeSent || decision != nil ||
		adapter.lookupCalls != 1 || repository.calls != 1 {
		t.Fatalf("CAS conflict must propagate without replay: assessment=%+v decision=%+v lookups=%d writes=%d err=%v",
			assessment, decision, adapter.lookupCalls, repository.calls, err)
	}
}
