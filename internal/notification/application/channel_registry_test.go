package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

type fakeChannelAdapter struct {
	snapshot domain.ChannelConfigSnapshot
	result   domain.ChannelSendResult
	err      error
	calls    int
}

func (f *fakeChannelAdapter) ConfigSnapshot() domain.ChannelConfigSnapshot {
	return cloneChannelSnapshot(f.snapshot)
}

func (f *fakeChannelAdapter) Send(_ context.Context, _ domain.ChannelSendRequest) (domain.ChannelSendResult, error) {
	f.calls++
	return f.result, f.err
}

func channelRegistryTestSnapshot() domain.ChannelConfigSnapshot {
	return domain.ChannelConfigSnapshot{
		ChannelRef: "channel-1", ConfigVersion: 2, ChannelKind: "test",
		Enabled: true, SecretRef: "secret://channel-1/v2",
		Capabilities: domain.ChannelCapabilities{
			SupportsIdempotency:    domain.CapabilitySupported,
			SupportsDeliveryLookup: domain.CapabilityUnknown,
			SupportsRetryAfter:     domain.CapabilityUnsupported,
			MaxPayloadBytes:        4096,
		},
		RetryableErrorCodes: []string{"rate_limited"},
	}
}

func channelRegistryTestRequest(snapshot domain.ChannelConfigSnapshot) domain.ChannelSendRequest {
	return domain.ChannelSendRequest{
		DeliveryID: "delivery-1", IdempotencyKey: "delivery-key-1",
		Target: domain.RecipientTarget{
			RecipientRef: "recipient-1", ChannelRef: snapshot.ChannelRef,
			ConfigVersion: snapshot.ConfigVersion, AdapterTargetRef: "target-opaque-1",
		},
		Content: domain.RenderedContent{
			TemplateID: "template-1", TemplateVersion: 1,
			TemplateDigest: strings.Repeat("a", 64), EventPayloadFingerprint: strings.Repeat("b", 64),
			Format: domain.TemplateFormatText, Subject: "subject", Body: "body",
		},
	}
}

func TestChannelRegistryResolvesExactImmutableVersion(t *testing.T) {
	registry := NewChannelRegistry()
	adapter := &fakeChannelAdapter{snapshot: channelRegistryTestSnapshot()}
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(adapter); !errors.Is(err, ErrChannelAlreadyRegistered) {
		t.Fatalf("duplicate registration must fail, got %v", err)
	}
	if _, err := registry.Resolve(context.Background(), "channel-1", 1); !errors.Is(err, ErrChannelVersionNotFound) {
		t.Fatalf("registry must not fall back to another version, got %v", err)
	}

	resolved, err := registry.Resolve(context.Background(), "channel-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := resolved.ConfigSnapshot()
	if !equalChannelSnapshot(snapshot, adapter.snapshot) {
		t.Fatalf("resolved snapshot differs from registered version: %+v", snapshot)
	}
	snapshot.RetryableErrorCodes[0] = "mutated"
	if !equalChannelSnapshot(resolved.ConfigSnapshot(), adapter.snapshot) {
		t.Fatal("mutating returned snapshot must not mutate registered version")
	}
}

func TestBoundChannelAdapterValidatesRequestBeforeCallingAdapter(t *testing.T) {
	registry := NewChannelRegistry()
	adapter := &fakeChannelAdapter{
		snapshot: channelRegistryTestSnapshot(),
		result:   domain.ChannelSendResult{Outcome: domain.ChannelOutcomeAccepted},
	}
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	resolved, err := registry.Resolve(context.Background(), "channel-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	request := channelRegistryTestRequest(adapter.snapshot)
	request.Target.ConfigVersion++
	result, err := resolved.Send(context.Background(), request)
	if !errors.Is(err, domain.ErrInvalidChannelSendRequest) ||
		result.Outcome != domain.ChannelOutcomeNotSent || adapter.calls != 0 {
		t.Fatalf("invalid request must not reach adapter: result=%+v calls=%d err=%v", result, adapter.calls, err)
	}
}

func TestBoundChannelAdapterFailsClosedOnUnclassifiedAdapterResult(t *testing.T) {
	registry := NewChannelRegistry()
	adapter := &fakeChannelAdapter{
		snapshot: channelRegistryTestSnapshot(),
		err:      errors.New("transport failed after request write"),
	}
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	resolved, err := registry.Resolve(context.Background(), "channel-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	result, sendErr := resolved.Send(context.Background(), channelRegistryTestRequest(adapter.snapshot))
	if sendErr == nil || result.Outcome != domain.ChannelOutcomeAmbiguous ||
		result.ErrorCode != "adapter_invalid_result" {
		t.Fatalf("unclassified error must become ambiguous: result=%+v err=%v", result, sendErr)
	}
	if adapter.calls != 1 {
		t.Fatalf("expected one adapter call, got %d", adapter.calls)
	}
}

func TestBoundChannelAdapterPreservesExplicitOutcome(t *testing.T) {
	registry := NewChannelRegistry()
	adapter := &fakeChannelAdapter{
		snapshot: channelRegistryTestSnapshot(),
		result:   domain.ChannelSendResult{Outcome: domain.ChannelOutcomeRejected, ErrorCode: "rate_limited"},
	}
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	resolved, err := registry.Resolve(context.Background(), "channel-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	result, sendErr := resolved.Send(context.Background(), channelRegistryTestRequest(adapter.snapshot))
	if sendErr != nil || result.Outcome != domain.ChannelOutcomeRejected ||
		!result.IsRetryable(adapter.snapshot) {
		t.Fatalf("explicit rejected result should be preserved: result=%+v err=%v", result, sendErr)
	}
}

func TestChannelRegistryRegisterSnapshotRequiresExactPersistedConfiguration(t *testing.T) {
	expected := channelRegistryTestSnapshot()
	adapter := &fakeChannelAdapter{snapshot: expected}
	registry := NewChannelRegistry()

	if err := registry.RegisterSnapshot(expected, adapter); err != nil {
		t.Fatalf("matching persisted snapshot should register: %v", err)
	}
	if _, err := registry.Resolve(context.Background(), expected.ChannelRef, expected.ConfigVersion); err != nil {
		t.Fatalf("registered persisted version should resolve: %v", err)
	}

	mismatchRegistry := NewChannelRegistry()
	mismatched := expected
	mismatched.SecretRef = "secret://wrong/channel"
	if err := mismatchRegistry.RegisterSnapshot(expected, &fakeChannelAdapter{snapshot: mismatched}); !errors.Is(err, ErrChannelConfigVersionConflict) {
		t.Fatalf("adapter configuration mismatch must fail closed: %v", err)
	}
	if _, err := mismatchRegistry.Resolve(context.Background(), expected.ChannelRef, expected.ConfigVersion); !errors.Is(err, ErrChannelVersionNotFound) {
		t.Fatal("mismatched adapter must not be registered")
	}
}

func TestChannelRegistryRegisterSnapshotNormalizesRetryableCodes(t *testing.T) {
	expected := channelRegistryTestSnapshot()
	expected.RetryableErrorCodes = []string{"rate_limited", "temporary"}
	actual := cloneChannelSnapshot(expected)
	actual.RetryableErrorCodes = []string{"temporary", "rate_limited"}

	registry := NewChannelRegistry()
	if err := registry.RegisterSnapshot(expected, &fakeChannelAdapter{snapshot: actual}); err != nil {
		t.Fatalf("retryable error code order must not affect immutable snapshot equality: %v", err)
	}
}
