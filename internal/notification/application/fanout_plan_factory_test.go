package application

import (
	"context"
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

type fanoutChannelSelectorStub struct {
	snapshot domain.ChannelConfigSnapshot
	err error
}

func (s fanoutChannelSelectorStub) ResolveForFanout(context.Context, domain.Subscription) (domain.ChannelConfigSnapshot, error) {
	return s.snapshot, s.err
}

type fanoutTemplateSelectorStub struct {
	version int64
	err error
}

func (s fanoutTemplateSelectorStub) ResolveForFanout(context.Context, string, string, string) (int64, error) {
	return s.version, s.err
}

type fanoutRecipientDirectoryStub struct {
	result RecipientDirectoryResult
	err error
	request RecipientDirectoryRequest
}

func (s *fanoutRecipientDirectoryStub) ResolveAuthorized(_ context.Context, request RecipientDirectoryRequest) (RecipientDirectoryResult, error) {
	s.request = request
	return s.result, s.err
}

type fanoutFilterMatcherStub struct {
	matched bool
	err error
	calls int
}

func (s *fanoutFilterMatcherStub) Matches(context.Context, domain.EventEnvelope, domain.Subscription) (bool, error) {
	s.calls++
	return s.matched, s.err
}

func productionFanoutInputs() (domain.EventEnvelope, domain.Subscription, domain.ChannelConfigSnapshot, RecipientDirectoryResult) {
	event := validFanoutEvent()
	channel := domain.ChannelConfigSnapshot{
		ChannelRef: "channel-1", ConfigVersion: 2, ChannelKind: "email", Enabled: true,
		Capabilities: domain.ChannelCapabilities{
			SupportsIdempotency: domain.CapabilityUnknown,
			SupportsDeliveryLookup: domain.CapabilityUnknown,
			SupportsRetryAfter: domain.CapabilityUnknown,
			MaxPayloadBytes: 4096,
		},
	}
	subscription := domain.Subscription{
		SubscriptionID: "sub-1", OwnerRef: "owner-1", Version: 3,
		Status: domain.SubscriptionActive, EventType: event.EventType,
		RecipientScope: domain.RecipientScopeOwner, ChannelRef: channel.ChannelRef,
		TemplateID: "template-1",
	}
	recipient := RecipientDirectoryResult{
		Target: domain.RecipientTarget{
			RecipientRef: "recipient-opaque-1", ChannelRef: channel.ChannelRef,
			ConfigVersion: channel.ConfigVersion, AdapterTargetRef: "adapter-target-opaque-1",
		},
		AuthorizationPolicyVersions: []domain.AuthorizationPolicyVersion{{
			PolicyType: "notification", PolicyRef: "policy-owner", PolicyVersion: "v4",
		}},
		AuthorizationRefs: []string{"authz-decision-1"},
	}
	return event, subscription, channel, recipient
}

func TestProductionFanoutPlanFactoryBuildsDeterministicImmutablePlan(t *testing.T) {
	event, subscription, channel, recipient := productionFanoutInputs()
	directory := &fanoutRecipientDirectoryStub{result: recipient}
	factory, err := NewProductionFanoutPlanFactory(
		nil, fanoutChannelSelectorStub{snapshot: channel},
		fanoutTemplateSelectorStub{version: 7}, directory,
	)
	if err != nil {
		t.Fatal(err)
	}
	first, err := factory.Build(context.Background(), event, subscription)
	if err != nil {
		t.Fatal(err)
	}
	second, err := factory.Build(context.Background(), event, subscription)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Matched || !second.Matched || first.Plan != second.Plan {
		t.Fatalf("same immutable inputs must produce the same plan: first=%+v second=%+v", first, second)
	}
	if err := first.Plan.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := first.Plan.VerifyFingerprint(); err != nil {
		t.Fatal(err)
	}
	if first.Plan.ChannelConfigVersion != channel.ConfigVersion ||
		first.Plan.TemplateVersion != 7 ||
		first.Plan.RecipientRef != recipient.Target.RecipientRef ||
		len(first.Plan.AuthorizationPolicyVersions) != 1 ||
		len(first.Plan.AuthorizationRefs) != 1 {
		t.Fatalf("plan omitted resolved production evidence: %+v", first.Plan)
	}
}

func TestProductionFanoutPlanFactoryDoesNotRequireFilterMatcherForNullFilters(t *testing.T) {
	event, subscription, channel, recipient := productionFanoutInputs()
	subscription.Filters = []byte("null")
	factory, err := NewProductionFanoutPlanFactory(
		nil, fanoutChannelSelectorStub{snapshot: channel},
		fanoutTemplateSelectorStub{version: 1}, &fanoutRecipientDirectoryStub{result: recipient},
	)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := factory.Build(context.Background(), event, subscription)
	if err != nil || !decision.Matched {
		t.Fatalf("null filters should match without a matcher: decision=%+v err=%v", decision, err)
	}
}

func TestProductionFanoutPlanFactoryFailsClosedWhenFilterMatcherMissing(t *testing.T) {
	event, subscription, channel, recipient := productionFanoutInputs()
	subscription.Filters = []byte(`{"status":"reject"}`)
	factory, err := NewProductionFanoutPlanFactory(
		nil, fanoutChannelSelectorStub{snapshot: channel},
		fanoutTemplateSelectorStub{version: 1}, &fanoutRecipientDirectoryStub{result: recipient},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = factory.Build(context.Background(), event, subscription)
	if !errors.Is(err, ErrFanoutFilterMatcherUnavailable) {
		t.Fatalf("expected fail-closed filter error, got %v", err)
	}
}

func TestProductionFanoutPlanFactoryReturnsUnmatchedForFilterAndAuthorizationDenial(t *testing.T) {
	event, subscription, channel, recipient := productionFanoutInputs()
	matcher := &fanoutFilterMatcherStub{matched: false}
	factory, err := NewProductionFanoutPlanFactory(
		matcher, fanoutChannelSelectorStub{snapshot: channel},
		fanoutTemplateSelectorStub{version: 1}, &fanoutRecipientDirectoryStub{result: recipient},
	)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := factory.Build(context.Background(), event, subscription)
	if err != nil || decision.Matched || matcher.calls != 1 {
		t.Fatalf("filter mismatch should not resolve downstream dependencies: decision=%+v calls=%d err=%v", decision, matcher.calls, err)
	}

	subscription.Filters = nil
	directory := &fanoutRecipientDirectoryStub{err: ErrRecipientNotAuthorized}
	factory, _ = NewProductionFanoutPlanFactory(
		nil, fanoutChannelSelectorStub{snapshot: channel},
		fanoutTemplateSelectorStub{version: 1}, directory,
	)
	decision, err = factory.Build(context.Background(), event, subscription)
	if err != nil || decision.Matched {
		t.Fatalf("authorization denial should be a non-match: decision=%+v err=%v", decision, err)
	}
}

func TestProductionFanoutPlanFactoryDoesNotDowngradeDependencyFailures(t *testing.T) {
	event, subscription, channel, recipient := productionFanoutInputs()
	dependencyErr := errors.New("template repository unavailable")
	factory, _ := NewProductionFanoutPlanFactory(
		nil, fanoutChannelSelectorStub{snapshot: channel},
		fanoutTemplateSelectorStub{err: dependencyErr}, &fanoutRecipientDirectoryStub{result: recipient},
	)
	_, err := factory.Build(context.Background(), event, subscription)
	if !errors.Is(err, dependencyErr) {
		t.Fatalf("dependency failure must remain an error: %v", err)
	}
}
