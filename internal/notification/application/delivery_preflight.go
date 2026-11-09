package application

import (
	"context"
	"errors"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var (
	ErrDeliveryPreflightUnavailable   = errors.New("notification: delivery preflight unavailable")
	ErrDeliveryPreflightNotApplicable = errors.New("notification: delivery preflight is not applicable")
	ErrRecipientNotAuthorized         = errors.New("notification: recipient is not authorized")
)

type DeliveryPreflightAction string

const (
	DeliveryPreflightReady DeliveryPreflightAction = "ready"
	DeliveryPreflightSkip  DeliveryPreflightAction = "skip"
	DeliveryPreflightBlock DeliveryPreflightAction = "block"
)

type DeliveryPreflightResult struct {
	Action              DeliveryPreflightAction
	ReasonCode          string
	CurrentSubscription domain.Subscription
	Channel             ChannelAdapter
	ChannelSnapshot     domain.ChannelConfigSnapshot
	Recipient           domain.RecipientTarget
}

type DeliveryPreflight struct {
	subscriptions SubscriptionRepository
	channels      ChannelResolver
	recipients    RecipientResolver
}

func NewDeliveryPreflight(
	subscriptions SubscriptionRepository,
	channels ChannelResolver,
	recipients RecipientResolver,
) (*DeliveryPreflight, error) {
	if subscriptions == nil || channels == nil || recipients == nil {
		return nil, ErrDeliveryPreflightUnavailable
	}
	return &DeliveryPreflight{
		subscriptions: subscriptions,
		channels:      channels,
		recipients:    recipients,
	}, nil
}

// Check revalidates current subscription authorization while preserving the
// immutable channel version and recipient reference captured by the Delivery.
func (p *DeliveryPreflight) Check(ctx context.Context, delivery domain.DeliveryRecord) (DeliveryPreflightResult, error) {
	if p == nil || p.subscriptions == nil || p.channels == nil || p.recipients == nil {
		return DeliveryPreflightResult{}, ErrDeliveryPreflightUnavailable
	}
	if ctx == nil {
		return DeliveryPreflightResult{}, ErrDeliveryPreflightNotApplicable
	}
	if err := ctx.Err(); err != nil {
		return DeliveryPreflightResult{}, err
	}
	if err := delivery.Validate(); err != nil {
		return DeliveryPreflightResult{}, err
	}
	if delivery.Status != domain.DeliverySending {
		return DeliveryPreflightResult{}, ErrDeliveryPreflightNotApplicable
	}

	subscription, found, err := p.subscriptions.GetLatest(ctx, delivery.SubscriptionID)
	if err != nil {
		return DeliveryPreflightResult{
			Action: DeliveryPreflightBlock, ReasonCode: "subscription_lookup_failed",
		}, err
	}
	if !found {
		return DeliveryPreflightResult{
			Action: DeliveryPreflightSkip, ReasonCode: "subscription_missing",
		}, nil
	}
	if subscription.SubscriptionID != delivery.SubscriptionID || subscription.Version < delivery.SubscriptionVersion {
		return DeliveryPreflightResult{
			Action: DeliveryPreflightBlock, ReasonCode: "subscription_snapshot_invalid",
		}, nil
	}
	result := DeliveryPreflightResult{CurrentSubscription: subscription}
	switch subscription.Status {
	case domain.SubscriptionActive:
	case domain.SubscriptionPaused, domain.SubscriptionDisabled:
		result.Action = DeliveryPreflightSkip
		result.ReasonCode = "subscription_inactive"
		return result, nil
	default:
		result.Action = DeliveryPreflightBlock
		result.ReasonCode = "subscription_status_invalid"
		return result, nil
	}

	adapter, err := p.channels.Resolve(ctx, delivery.ChannelRef, delivery.ChannelConfigVersion)
	if err != nil {
		result.Action = DeliveryPreflightBlock
		result.ReasonCode = "channel_unavailable"
		return result, err
	}
	if adapter == nil {
		result.Action = DeliveryPreflightBlock
		result.ReasonCode = "channel_unavailable"
		return result, ErrChannelAdapterUnavailable
	}
	snapshot := adapter.ConfigSnapshot()
	if err := snapshot.Validate(); err != nil ||
		snapshot.ChannelRef != delivery.ChannelRef ||
		snapshot.ConfigVersion != delivery.ChannelConfigVersion {
		result.Action = DeliveryPreflightBlock
		result.ReasonCode = "channel_snapshot_mismatch"
		return result, nil
	}
	result.Channel = adapter
	result.ChannelSnapshot = snapshot
	if !snapshot.Enabled {
		result.Action = DeliveryPreflightBlock
		result.ReasonCode = "channel_disabled"
		return result, nil
	}

	target, err := p.recipients.Resolve(ctx, subscription, snapshot)
	if errors.Is(err, ErrRecipientNotAuthorized) {
		result.Action = DeliveryPreflightSkip
		result.ReasonCode = "recipient_not_authorized"
		return result, nil
	}
	if err != nil {
		result.Action = DeliveryPreflightBlock
		result.ReasonCode = "recipient_resolution_failed"
		return result, err
	}
	if err := target.Validate(snapshot); err != nil {
		result.Action = DeliveryPreflightBlock
		result.ReasonCode = "recipient_target_invalid"
		return result, err
	}
	if target.RecipientRef != delivery.RecipientRef {
		result.Action = DeliveryPreflightSkip
		result.ReasonCode = "recipient_not_authorized"
		return result, nil
	}
	result.Action = DeliveryPreflightReady
	result.ReasonCode = "preflight_passed"
	result.Recipient = target
	return result, nil
}
