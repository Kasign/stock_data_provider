package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

type fakeLookupChannelAdapter struct {
	*fakeChannelAdapter
	lookupResult domain.ChannelDeliveryLookupResult
	lookupErr    error
	lookupCalls  int
	lastRequest  domain.ChannelDeliveryLookupRequest
}

func (f *fakeLookupChannelAdapter) LookupDelivery(_ context.Context, request domain.ChannelDeliveryLookupRequest) (domain.ChannelDeliveryLookupResult, error) {
	f.lookupCalls++
	f.lastRequest = request
	return f.lookupResult, f.lookupErr
}

func lookupSnapshot(state domain.CapabilityState) domain.ChannelConfigSnapshot {
	snapshot := channelRegistryTestSnapshot()
	snapshot.Capabilities.SupportsDeliveryLookup = state
	return snapshot
}

func lookupRequest(snapshot domain.ChannelConfigSnapshot) domain.ChannelDeliveryLookupRequest {
	return domain.ChannelDeliveryLookupRequest{
		ChannelRef: snapshot.ChannelRef, ConfigVersion: snapshot.ConfigVersion,
		DeliveryID: "delivery-lookup-1", AttemptID: "attempt-lookup-1",
		IdempotencyKey: "stable-key-1",
	}
}

func lookupResult(request domain.ChannelDeliveryLookupRequest) domain.ChannelDeliveryLookupResult {
	return domain.ChannelDeliveryLookupResult{
		ChannelRef: request.ChannelRef, ConfigVersion: request.ConfigVersion,
		DeliveryID: request.DeliveryID, AttemptID: request.AttemptID,
		Outcome: domain.ChannelLookupAccepted, EvidenceRef: "evidence-1",
		ObservedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		ProviderMessageRef: "provider-message-1",
	}
}

func TestChannelRegistryOptionalDeliveryLookup(t *testing.T) {
	snapshot := lookupSnapshot(domain.CapabilitySupported)
	adapter := &fakeLookupChannelAdapter{
		fakeChannelAdapter: &fakeChannelAdapter{snapshot: snapshot},
	}
	request := lookupRequest(snapshot)
	adapter.lookupResult = lookupResult(request)
	registry := NewChannelRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	resolved, err := registry.ResolveDeliveryLookup(context.Background(), snapshot.ChannelRef, snapshot.ConfigVersion)
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolved.LookupDelivery(context.Background(), request)
	if err != nil || got.Outcome != domain.ChannelLookupAccepted || adapter.lookupCalls != 1 {
		t.Fatalf("lookup failed: result=%+v calls=%d err=%v", got, adapter.lookupCalls, err)
	}
}

func TestChannelRegistryLookupFailsClosedWhenCapabilityAndAdapterDisagree(t *testing.T) {
	snapshot := lookupSnapshot(domain.CapabilitySupported)
	registry := NewChannelRegistry()
	if err := registry.Register(&fakeChannelAdapter{snapshot: snapshot}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.ResolveDeliveryLookup(context.Background(), snapshot.ChannelRef, snapshot.ConfigVersion); !errors.Is(err, ErrChannelDeliveryLookupUnavailable) {
		t.Fatalf("missing optional interface must fail closed: %v", err)
	}
	for _, state := range []domain.CapabilityState{domain.CapabilityUnknown, domain.CapabilityUnsupported} {
		snapshot := lookupSnapshot(state)
		registry := NewChannelRegistry()
		adapter := &fakeLookupChannelAdapter{fakeChannelAdapter: &fakeChannelAdapter{snapshot: snapshot}}
		if err := registry.Register(adapter); err != nil {
			t.Fatal(err)
		}
		if _, err := registry.ResolveDeliveryLookup(context.Background(), snapshot.ChannelRef, snapshot.ConfigVersion); !errors.Is(err, ErrChannelDeliveryLookupUnavailable) || adapter.lookupCalls != 0 {
			t.Fatalf("state %s must prohibit lookup: %v", state, err)
		}
	}
}

func TestChannelRegistryLookupRequiresExactVersionAndIdentity(t *testing.T) {
	snapshot := lookupSnapshot(domain.CapabilitySupported)
	adapter := &fakeLookupChannelAdapter{fakeChannelAdapter: &fakeChannelAdapter{snapshot: snapshot}}
	request := lookupRequest(snapshot)
	adapter.lookupResult = lookupResult(request)
	registry := NewChannelRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.ResolveDeliveryLookup(context.Background(), snapshot.ChannelRef, snapshot.ConfigVersion+1); !errors.Is(err, ErrChannelVersionNotFound) {
		t.Fatalf("must not fall back to another version: %v", err)
	}
	resolved, err := registry.ResolveDeliveryLookup(context.Background(), snapshot.ChannelRef, snapshot.ConfigVersion)
	if err != nil {
		t.Fatal(err)
	}
	wrong := request
	wrong.ConfigVersion++
	if _, err := resolved.LookupDelivery(context.Background(), wrong); !errors.Is(err, domain.ErrInvalidChannelLookupRequest) || adapter.lookupCalls != 0 {
		t.Fatalf("wrong request identity reached adapter: %v calls=%d", err, adapter.lookupCalls)
	}
	adapter.lookupResult.DeliveryID = "other-delivery"
	if _, err := resolved.LookupDelivery(context.Background(), request); !errors.Is(err, domain.ErrInvalidChannelLookupResult) {
		t.Fatalf("mismatched result identity must be rejected: %v", err)
	}
}

func TestChannelDeliveryLookupValidatesOutcomeEvidence(t *testing.T) {
	snapshot := lookupSnapshot(domain.CapabilitySupported)
	request := lookupRequest(snapshot)
	tests := map[string]func(*domain.ChannelDeliveryLookupResult){
		"accepted missing provider ref": func(result *domain.ChannelDeliveryLookupResult) {
			result.ProviderMessageRef = ""
		},
		"not accepted missing code": func(result *domain.ChannelDeliveryLookupResult) {
			result.Outcome = domain.ChannelLookupNotAccepted
			result.ProviderMessageRef = ""
			result.ErrorCode = ""
		},
		"not accepted missing evidence": func(result *domain.ChannelDeliveryLookupResult) {
			result.Outcome = domain.ChannelLookupNotAccepted
			result.ProviderMessageRef = ""
			result.ErrorCode = "provider_rejected"
			result.EvidenceRef = ""
		},
		"invalid observation time": func(result *domain.ChannelDeliveryLookupResult) {
			result.ObservedAt = time.Time{}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			result := lookupResult(request)
			mutate(&result)
			if !errors.Is(result.Validate(request), domain.ErrInvalidChannelLookupResult) {
				t.Fatalf("expected invalid result, got %+v", result)
			}
		})
	}
}

func TestChannelDeliveryLookupDoesNotClassifyQueryErrorsAsNonAcceptance(t *testing.T) {
	snapshot := lookupSnapshot(domain.CapabilitySupported)
	request := lookupRequest(snapshot)
	adapter := &fakeLookupChannelAdapter{
		fakeChannelAdapter: &fakeChannelAdapter{snapshot: snapshot},
		lookupErr:          errors.New("provider timeout"),
	}
	registry := NewChannelRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	resolved, err := registry.ResolveDeliveryLookup(context.Background(), snapshot.ChannelRef, snapshot.ConfigVersion)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = resolved.LookupDelivery(context.Background(), request); err == nil || adapter.lookupCalls != 1 {
		t.Fatalf("query error must propagate: %v", err)
	}
}

func TestChannelDeliveryLookupIdempotencyKeyRequiresCapability(t *testing.T) {
	snapshot := lookupSnapshot(domain.CapabilitySupported)
	snapshot.Capabilities.SupportsIdempotency = domain.CapabilityUnknown
	request := lookupRequest(snapshot)
	if !errors.Is(request.Validate(snapshot.Capabilities), domain.ErrInvalidChannelLookupRequest) {
		t.Fatal("unverified idempotency key must be rejected")
	}
	request.IdempotencyKey = ""
	if err := request.Validate(snapshot.Capabilities); err != nil {
		t.Fatalf("lookup without idempotency key should be valid: %v", err)
	}
}

func TestBuildChannelDeliveryLookupRequestBindsUnknownDeliveryAttempt(t *testing.T) {
	delivery, channel, attempts, _, _ := unknownReconciliationFixture(t)
	request, err := BuildChannelDeliveryLookupRequest(delivery, channel, attempts[0])
	if err != nil {
		t.Fatal(err)
	}
	if request.DeliveryID != delivery.DeliveryID || request.AttemptID != attempts[0].AttemptID ||
		request.ChannelRef != delivery.ChannelRef || request.ConfigVersion != delivery.ChannelConfigVersion ||
		request.IdempotencyKey != "" {
		t.Fatalf("lookup request contains mismatched identity or unsupported idempotency key: %+v", request)
	}

	channel.Capabilities.SupportsIdempotency = domain.CapabilitySupported
	request, err = BuildChannelDeliveryLookupRequest(delivery, channel, attempts[0])
	if err != nil || request.IdempotencyKey != delivery.DeliveryIdempotencyKey {
		t.Fatalf("supported idempotency key was not bound to delivery: %+v err=%v", request, err)
	}
}

func TestRegistryLookupUnknownDeliveryBuildsEvidenceOnlyFromAdapterResult(t *testing.T) {
	delivery, channel, attempts, _, now := unknownReconciliationFixture(t)
	channel.Capabilities.SupportsIdempotency = domain.CapabilitySupported
	adapter := &fakeLookupChannelAdapter{
		fakeChannelAdapter: &fakeChannelAdapter{snapshot: channel},
	}
	registry := NewChannelRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	request := domain.ChannelDeliveryLookupRequest{
		ChannelRef: channel.ChannelRef, ConfigVersion: channel.ConfigVersion,
		DeliveryID: delivery.DeliveryID, AttemptID: attempts[0].AttemptID,
		IdempotencyKey: delivery.DeliveryIdempotencyKey,
	}
	adapter.lookupResult = domain.ChannelDeliveryLookupResult{
		ChannelRef: request.ChannelRef, ConfigVersion: request.ConfigVersion,
		DeliveryID: request.DeliveryID, AttemptID: request.AttemptID,
		Outcome: domain.ChannelLookupIndeterminate, EvidenceRef: "lookup-evidence-1",
		ObservedAt: now, 
	}
	evidence, err := registry.LookupUnknownDelivery(context.Background(), delivery, channel, attempts[0])
	if err != nil {
		t.Fatal(err)
	}
	if evidence.DeliveryID != delivery.DeliveryID || evidence.AttemptID != attempts[0].AttemptID ||
		evidence.Outcome != UnknownLookupIndeterminate || evidence.EvidenceRef != "lookup-evidence-1" {
		t.Fatalf("unexpected evidence mapping: %+v", evidence)
	}
}
