package application

import (
	"context"
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

type fixedRecipientResolver struct {
	target domain.RecipientTarget
	err    error
	calls  int
}

func (r *fixedRecipientResolver) Resolve(_ context.Context, _ domain.Subscription, _ domain.ChannelConfigSnapshot) (domain.RecipientTarget, error) {
	r.calls++
	return r.target, r.err
}

func preflightFixture(t *testing.T) (*DeliveryPreflight, domain.DeliveryRecord, *fixedRecipientResolver) {
	t.Helper()
	delivery := validDeliveryForRepository()
	delivery.Status = domain.DeliverySending
	delivery.Version = 2

	subscription := validApplicationSubscription()
	subscription.SubscriptionID = delivery.SubscriptionID
	subscription.ChannelRef = delivery.ChannelRef
	subscription.TemplateID = delivery.TemplateID
	subscriptions := newMemorySubscriptions()
	if err := subscriptions.Create(context.Background(), subscription); err != nil {
		t.Fatal(err)
	}
	next := nextSubscriptionVersion(subscription)
	if err := subscriptions.AppendVersion(context.Background(), subscription.Version, next); err != nil {
		t.Fatal(err)
	}

	snapshot := channelRegistryTestSnapshot()
	snapshot.ConfigVersion = delivery.ChannelConfigVersion
	adapter := &fakeChannelAdapter{snapshot: snapshot}
	registry := NewChannelRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	recipients := &fixedRecipientResolver{
		target: domain.RecipientTarget{
			RecipientRef:     delivery.RecipientRef,
			ChannelRef:       delivery.ChannelRef,
			ConfigVersion:    delivery.ChannelConfigVersion,
			AdapterTargetRef: "opaque-target-1",
		},
	}
	preflight, err := NewDeliveryPreflight(subscriptions, registry, recipients)
	if err != nil {
		t.Fatal(err)
	}
	return preflight, delivery, recipients
}

func TestDeliveryPreflightUsesCurrentSubscriptionAndExactHistoricalChannelVersion(t *testing.T) {
	preflight, delivery, recipients := preflightFixture(t)
	result, err := preflight.Check(context.Background(), delivery)
	if err != nil || result.Action != DeliveryPreflightReady ||
		result.CurrentSubscription.Version != 2 ||
		result.ChannelSnapshot.ConfigVersion != delivery.ChannelConfigVersion ||
		result.Recipient.RecipientRef != delivery.RecipientRef {
		t.Fatalf("expected ready preflight with current authorization and historical channel: %+v err=%v", result, err)
	}
	if recipients.calls != 1 {
		t.Fatalf("recipient authorization must be checked exactly once, got %d", recipients.calls)
	}
}

func TestDeliveryPreflightSkipsInactiveSubscriptionAndChangedRecipient(t *testing.T) {
	preflight, delivery, recipients := preflightFixture(t)
	subscription := nextSubscriptionVersion(validApplicationSubscription())
	subscription.SubscriptionID = delivery.SubscriptionID
	subscription.Status = domain.SubscriptionPaused
	repository := newMemorySubscriptions()
	initial := validApplicationSubscription()
	initial.SubscriptionID = delivery.SubscriptionID
	if err := repository.Create(context.Background(), initial); err != nil {
		t.Fatal(err)
	}
	if err := repository.AppendVersion(context.Background(), initial.Version, subscription); err != nil {
		t.Fatal(err)
	}
	preflight.subscriptions = repository
	result, err := preflight.Check(context.Background(), delivery)
	if err != nil || result.Action != DeliveryPreflightSkip || result.ReasonCode != "subscription_inactive" {
		t.Fatalf("inactive subscription must skip: %+v err=%v", result, err)
	}
	if recipients.calls != 0 {
		t.Fatal("recipient resolver must not run for inactive subscriptions")
	}

	preflight, delivery, _ = preflightFixture(t)
	preflight.recipients = &fixedRecipientResolver{target: domain.RecipientTarget{
		RecipientRef: "new-recipient", ChannelRef: delivery.ChannelRef,
		ConfigVersion: delivery.ChannelConfigVersion, AdapterTargetRef: "opaque-target-2",
	}}
	result, err = preflight.Check(context.Background(), delivery)
	if err != nil || result.Action != DeliveryPreflightSkip || result.ReasonCode != "recipient_not_authorized" {
		t.Fatalf("changed recipient must not receive historical delivery: %+v err=%v", result, err)
	}
}

func TestDeliveryPreflightBlocksMissingChannelAndResolverDependency(t *testing.T) {
	preflight, delivery, _ := preflightFixture(t)
	preflight.channels = NewChannelRegistry()
	result, err := preflight.Check(context.Background(), delivery)
	if !errors.Is(err, ErrChannelVersionNotFound) ||
		result.Action != DeliveryPreflightBlock || result.ReasonCode != "channel_unavailable" {
		t.Fatalf("missing exact channel version must block: %+v err=%v", result, err)
	}

	preflight, delivery, _ = preflightFixture(t)
	preflight.recipients = &fixedRecipientResolver{err: errors.New("recipient directory unavailable")}
	result, err = preflight.Check(context.Background(), delivery)
	if err == nil || result.Action != DeliveryPreflightBlock || result.ReasonCode != "recipient_resolution_failed" {
		t.Fatalf("recipient dependency failure must block, not skip: %+v err=%v", result, err)
	}
}

func TestDeliveryPreflightBlocksDisabledChannelAndSkipsUnauthorizedRecipient(t *testing.T) {
	preflight, delivery, _ := preflightFixture(t)
	disabled := channelRegistryTestSnapshot()
	disabled.ConfigVersion = delivery.ChannelConfigVersion
	disabled.Enabled = false
	adapter := &fakeChannelAdapter{snapshot: disabled}
	registry := NewChannelRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	preflight.channels = registry
	result, err := preflight.Check(context.Background(), delivery)
	if err != nil || result.Action != DeliveryPreflightBlock || result.ReasonCode != "channel_disabled" {
		t.Fatalf("disabled historical channel must block: %+v err=%v", result, err)
	}

	preflight, delivery, _ = preflightFixture(t)
	preflight.recipients = &fixedRecipientResolver{err: ErrRecipientNotAuthorized}
	result, err = preflight.Check(context.Background(), delivery)
	if err != nil || result.Action != DeliveryPreflightSkip || result.ReasonCode != "recipient_not_authorized" {
		t.Fatalf("explicit authorization denial must skip: %+v err=%v", result, err)
	}
}

func TestDeliveryPreflightRejectsNonSendingDelivery(t *testing.T) {
	preflight, delivery, _ := preflightFixture(t)
	delivery.Status = domain.DeliveryQueued
	if _, err := preflight.Check(context.Background(), delivery); !errors.Is(err, ErrDeliveryPreflightNotApplicable) {
		t.Fatalf("preflight only applies after a sending claim: %v", err)
	}
}
