package application

import (
	"context"
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

type recipientDirectoryStub struct {
	request RecipientDirectoryRequest
	result  RecipientDirectoryResult
	err     error
}

func (s *recipientDirectoryStub) ResolveAuthorized(_ context.Context, request RecipientDirectoryRequest) (RecipientDirectoryResult, error) {
	s.request = request
	return s.result, s.err
}

func recipientDirectoryChannel() domain.ChannelConfigSnapshot {
	return domain.ChannelConfigSnapshot{
		ChannelRef: "channel-email", ConfigVersion: 3, ChannelKind: "email", Enabled: true,
		Capabilities: domain.ChannelCapabilities{
			SupportsIdempotency:    domain.CapabilityUnknown,
			SupportsDeliveryLookup: domain.CapabilityUnknown,
			SupportsRetryAfter:     domain.CapabilityUnknown,
			MaxPayloadBytes:        4096,
		},
	}
}

func TestDirectoryRecipientResolverResolvesSubscriptionOwnerWithPolicyEvidence(t *testing.T) {
	channel := recipientDirectoryChannel()
	directory := &recipientDirectoryStub{result: RecipientDirectoryResult{
		Target: domain.RecipientTarget{
			RecipientRef: "identity:user-opaque", ChannelRef: channel.ChannelRef,
			ConfigVersion: channel.ConfigVersion, AdapterTargetRef: "email-target:opaque",
		},
		AuthorizationPolicyVersions: []domain.AuthorizationPolicyVersion{{
			PolicyType: "notification_delivery", PolicyRef: "policy:owner", PolicyVersion: "v2",
		}},
		AuthorizationRefs: []string{"authorization:decision-123"},
	}}
	resolver, err := NewDirectoryRecipientResolver(directory)
	if err != nil {
		t.Fatal(err)
	}
	target, err := resolver.Resolve(context.Background(), domain.Subscription{
		SubscriptionID: "sub-1", OwnerRef: "identity:owner-1",
		RecipientScope: domain.RecipientScopeOwner, ChannelRef: channel.ChannelRef,
	}, channel)
	if err != nil {
		t.Fatal(err)
	}
	if target.RecipientRef != "identity:user-opaque" ||
		directory.request.OwnerRef != "identity:owner-1" ||
		directory.request.RecipientRef != "" ||
		directory.request.ChannelRef != channel.ChannelRef ||
		directory.request.ChannelKind != channel.ChannelKind ||
		directory.request.ConfigVersion != channel.ConfigVersion {
		t.Fatalf("unexpected identity lookup: target=%+v request=%+v", target, directory.request)
	}
}

func TestDirectoryRecipientResolverBindsPreconfiguredTarget(t *testing.T) {
	channel := recipientDirectoryChannel()
	directory := &recipientDirectoryStub{result: RecipientDirectoryResult{
		Target: domain.RecipientTarget{
			RecipientRef:   "recipient:configured-1", ChannelRef: channel.ChannelRef,
			ConfigVersion: channel.ConfigVersion, AdapterTargetRef: "email-target:opaque",
		},
	}}
	resolver, _ := NewDirectoryRecipientResolver(directory)
	_, err := resolver.Resolve(context.Background(), domain.Subscription{
		SubscriptionID: "sub-1", OwnerRef: "identity:owner-1",
		RecipientScope: domain.RecipientScopePreconfiguredTarget,
		RecipientRef: "recipient:configured-1", ChannelRef: channel.ChannelRef,
	}, channel)
	if err != nil {
		t.Fatal(err)
	}
	if directory.request.RecipientRef != "recipient:configured-1" {
		t.Fatalf("preconfigured recipient ref was not forwarded: %+v", directory.request)
	}
}

func TestDirectoryRecipientResolverFailsClosedOnDirectoryMismatch(t *testing.T) {
	channel := recipientDirectoryChannel()
	directory := &recipientDirectoryStub{result: RecipientDirectoryResult{
		Target: domain.RecipientTarget{
			RecipientRef: "recipient:other", ChannelRef: channel.ChannelRef,
			ConfigVersion: channel.ConfigVersion, AdapterTargetRef: "email-target:opaque",
		},
	}}
	resolver, _ := NewDirectoryRecipientResolver(directory)
	_, err := resolver.Resolve(context.Background(), domain.Subscription{
		SubscriptionID: "sub-1", OwnerRef: "identity:owner-1",
		RecipientScope: domain.RecipientScopePreconfiguredTarget,
		RecipientRef: "recipient:configured-1", ChannelRef: channel.ChannelRef,
	}, channel)
	if !errors.Is(err, ErrRecipientNotAuthorized) {
		t.Fatalf("expected authorization denial, got %v", err)
	}
}

func TestDirectoryRecipientResolverPreservesDependencyFailure(t *testing.T) {
	dependencyErr := errors.New("identity service unavailable")
	channel := recipientDirectoryChannel()
	directory := &recipientDirectoryStub{err: dependencyErr}
	resolver, _ := NewDirectoryRecipientResolver(directory)
	_, err := resolver.Resolve(context.Background(), domain.Subscription{
		SubscriptionID: "sub-1", OwnerRef: "identity:owner-1",
		RecipientScope: domain.RecipientScopeOwner, ChannelRef: channel.ChannelRef,
	}, channel)
	if !errors.Is(err, dependencyErr) {
		t.Fatalf("dependency failure must not become an authorization denial: %v", err)
	}
}

func TestDirectoryRecipientResolverRejectsPlaintextOrInvalidEvidence(t *testing.T) {
	channel := recipientDirectoryChannel()
	directory := &recipientDirectoryStub{result: RecipientDirectoryResult{
		Target: domain.RecipientTarget{
			RecipientRef: "user@example.com", ChannelRef: channel.ChannelRef,
			ConfigVersion: channel.ConfigVersion, AdapterTargetRef: "email-target:opaque",
		},
	}}
	resolver, _ := NewDirectoryRecipientResolver(directory)
	_, err := resolver.Resolve(context.Background(), domain.Subscription{
		SubscriptionID: "sub-1", OwnerRef: "identity:owner-1",
		RecipientScope: domain.RecipientScopeOwner, ChannelRef: channel.ChannelRef,
	}, channel)
	if !errors.Is(err, domain.ErrInvalidRecipientTarget) {
		t.Fatalf("plaintext recipient must be rejected: %v", err)
	}
}
