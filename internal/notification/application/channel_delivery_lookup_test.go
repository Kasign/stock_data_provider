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
	lookupErr error
	lookupCalls int
	lastRequest domain.ChannelDeliveryLookupRequest
}

func (f *fakeLookupChannelAdapter) LookupDelivery(_ context.Context, request domain.ChannelDeliveryLookupRequest) (domain.ChannelDeliveryLookupResult, error) {
	f.lookupCalls++
	f.lastRequest = request
	return f.lookupResult, f.lookupErr
}

func lookupSnapshot(state domain.CapabilityState) domain.ChannelConfigSnapshot {
	s := channelRegistryTestSnapshot()
	s.Capabilities.SupportsDeliveryLookup = state
	return s
}

func lookupRequest(s domain.ChannelConfigSnapshot) domain.ChannelDeliveryLookupRequest {
	return domain.ChannelDeliveryLookupRequest{
		ChannelRef:s.ChannelRef, ConfigVersion:s.ConfigVersion,
		DeliveryID:"delivery-lookup-1", AttemptID:"attempt-lookup-1",
		IdempotencyKey:"stable-key-1",
	}
}

func lookupResult(r domain.ChannelDeliveryLookupRequest) domain.ChannelDeliveryLookupResult {
	return domain.ChannelDeliveryLookupResult{
		ChannelRef:r.ChannelRef, ConfigVersion:r.ConfigVersion,
		DeliveryID:r.DeliveryID, AttemptID:r.AttemptID,
		Outcome:domain.ChannelLookupAccepted, EvidenceRef:"evidence-1",
		ObservedAt:time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		ProviderMessageRef:"provider-message-1",
	}
}

func TestChannelRegistryOptionalDeliveryLookup(t *testing.T) {
	s := lookupSnapshot(domain.CapabilitySupported)
	adapter := &fakeLookupChannelAdapter{
		fakeChannelAdapter:&fakeChannelAdapter{snapshot:s},
	}
	r := lookupRequest(s)
	adapter.lookupResult = lookupResult(r)
	registry := NewChannelRegistry()
	if err := registry.Register(adapter); err != nil { t.Fatal(err) }
	resolved, err := registry.ResolveDeliveryLookup(context.Background(), s.ChannelRef, s.ConfigVersion)
	if err != nil { t.Fatal(err) }
	got, err := resolved.LookupDelivery(context.Background(), r)
	if err != nil || got.Outcome != domain.ChannelLookupAccepted || adapter.lookupCalls != 1 {
		t.Fatalf("lookup failed: result=%+v calls=%d err=%v", got, adapter.lookupCalls, err)
	}
}

func TestChannelRegistryLookupFailsClosedWhenCapabilityAndAdapterDisagree(t *testing.T) {
	s := lookupSnapshot(domain.CapabilitySupported)
	registry := NewChannelRegistry()
	if err := registry.Register(&fakeChannelAdapter{snapshot:s}); err != nil { t.Fatal(err) }
	if _, err := registry.ResolveDeliveryLookup(context.Background(), s.ChannelRef, s.ConfigVersion); !errors.Is(err, ErrChannelDeliveryLookupUnavailable) {
		t.Fatalf("missing optional interface must fail closed: %v", err)
	}
	for _, state := range []domain.CapabilityState{domain.CapabilityUnknown, domain.CapabilityUnsupported} {
		s := lookupSnapshot(state)
		r := NewChannelRegistry()
		a := &fakeLookupChannelAdapter{fakeChannelAdapter:&fakeChannelAdapter{snapshot:s}}
		if err := r.Register(a); err != nil { t.Fatal(err) }
		if _, err := r.ResolveDeliveryLookup(context.Background(), s.ChannelRef, s.ConfigVersion); !errors.Is(err, ErrChannelDeliveryLookupUnavailable) || a.lookupCalls != 0 {
			t.Fatalf("state %s must prohibit lookup: %v", state, err)
		}
	}
}

func TestChannelRegistryLookupRequiresExactVersionAndIdentity(t *testing.T) {
	s := lookupSnapshot(domain.CapabilitySupported)
	a := &fakeLookupChannelAdapter{fakeChannelAdapter:&fakeChannelAdapter{snapshot:s}}
	r := lookupRequest(s)
	a.lookupResult = lookupResult(r)
	registry := NewChannelRegistry()
	if err := registry.Register(a); err != nil { t.Fatal(err) }
	if _, err := registry.ResolveDeliveryLookup(context.Background(), s.ChannelRef, s.ConfigVersion+1); !errors.Is(err, ErrChannelVersionNotFound) {
		t.Fatalf("must not fall back to another version: %v", err)
	}
	resolved, err := registry.ResolveDeliveryLookup(context.Background(), s.ChannelRef, s.ConfigVersion)
	if err != nil { t.Fatal(err) }
	wrong := r
	wrong.ConfigVersion++
	if _, err := resolved.LookupDelivery(context.Background(), wrong); !errors.Is(err, domain.ErrInvalidChannelLookupRequest) || a.lookupCalls != 0 {
		t.Fatalf("wrong request identity reached adapter: %v calls=%d", err, a.lookupCalls)
	}
	a.lookupResult.DeliveryID = "other-delivery"
	if _, err := resolved.LookupDelivery(context.Background(), r); !errors.Is(err, domain.ErrInvalidChannelLookupResult) {
		t.Fatalf("mismatched result identity must be rejected: %v", err)
	}
}

func TestChannelDeliveryLookupValidatesOutcomeEvidence(t *testing.T) {
	s := lookupSnapshot(domain.CapabilitySupported)
	r := lookupRequest(s)
	for name, mutate := range map[string]func(*domain.ChannelDeliveryLookupResult){
		"accepted missing provider ref":func(v *domain.ChannelDeliveryLookupResult){v.ProviderMessageRef=""},
		"not accepted missing code":func(v *domain.ChannelDeliveryLookupResult){v.Outcome=domain.ChannelLookupNotAccepted;v.ProviderMessageRef="";v.ErrorCode=""},
		"not accepted missing evidence":func(v *domain.ChannelDeliveryLookupResult){v.Outcome=domain.ChannelLookupNotAccepted;v.ProviderMessageRef="";v.ErrorCode="provider_rejected";v.EvidenceRef=""},
		"invalid observation time":func(v *domain.ChannelDeliveryLookupResult){v.ObservedAt=time.Time{}},
	} {
		t.Run(name,func(t *testing.T){
			v:=lookupResult(r);mutate(&v)
			if !errors.Is(v.Validate(r),domain.ErrInvalidChannelLookupResult) {t.Fatalf("expected invalid result, got %+v",v)}
		})
	}
}

func TestChannelDeliveryLookupDoesNotClassifyQueryErrorsAsNonAcceptance(t *testing.T) {
	s:=lookupSnapshot(domain.CapabilitySupported)
	r:=lookupRequest(s)
	a:=&fakeLookupChannelAdapter{fakeChannelAdapter:&fakeChannelAdapter{snapshot:s},lookupErr:errors.New("provider timeout")}
	registry:=NewChannelRegistry()
	if err:=registry.Register(a);err!=nil{t.Fatal(err)}
	resolved,err:=registry.ResolveDeliveryLookup(context.Background(),s.ChannelRef,s.ConfigVersion)
	if err!=nil{t.Fatal(err)}
	if _,err=resolved.LookupDelivery(context.Background(),r);err==nil || a.lookupCalls!=1 {t.Fatalf("query error must propagate: %v",err)}
}

func TestChannelDeliveryLookupIdempotencyKeyRequiresCapability(t *testing.T) {
	s:=lookupSnapshot(domain.CapabilitySupported)
	s.Capabilities.SupportsIdempotency=domain.CapabilityUnknown
	r:=lookupRequest(s)
	if !errors.Is(r.Validate(s.Capabilities),domain.ErrInvalidChannelLookupRequest) {t.Fatal("unverified idempotency key must be rejected")}
	r.IdempotencyKey=""
	if err:=r.Validate(s.Capabilities);err!=nil{t.Fatalf("lookup without idempotency key should be valid: %v",err)}
}
