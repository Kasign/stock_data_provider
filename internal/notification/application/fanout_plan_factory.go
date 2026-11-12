package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
	"github.com/gowebpki/jcs"
)

var (
	ErrFanoutFilterMatcherUnavailable    = errors.New("notification: fanout filter matcher is unavailable")
	ErrFanoutChannelSelectorUnavailable  = errors.New("notification: fanout channel selector is unavailable")
	ErrFanoutTemplateSelectorUnavailable = errors.New("notification: fanout template selector is unavailable")
)

// SubscriptionFilterMatcher evaluates only the registered, versioned filter schema.
// It must not interpret arbitrary JSON as a query language.
type SubscriptionFilterMatcher interface {
	Matches(ctx context.Context, event domain.EventEnvelope, subscription domain.Subscription) (bool, error)
}

// FanoutChannelConfigSelector resolves the exact immutable Channel version to freeze
// into this DeliveryPlan. It must not return a disabled or silently substituted version.
type FanoutChannelConfigSelector interface {
	ResolveForFanout(ctx context.Context, subscription domain.Subscription) (domain.ChannelConfigSnapshot, error)
}

// FanoutTemplateVersionSelector resolves an immutable template version compatible
// with the Event's exact event_type and schema_version.
type FanoutTemplateVersionSelector interface {
	ResolveForFanout(ctx context.Context, templateID, eventType, schemaVersion string) (int64, error)
}

// ProductionFanoutPlanFactory builds a deterministic immutable DeliveryPlan from
// the resolved Notification configuration and current identity authorization facts.
type ProductionFanoutPlanFactory struct {
	filters    SubscriptionFilterMatcher
	channels   FanoutChannelConfigSelector
	templates  FanoutTemplateVersionSelector
	recipients RecipientDirectory
}

func NewProductionFanoutPlanFactory(
	filters SubscriptionFilterMatcher,
	channels FanoutChannelConfigSelector,
	templates FanoutTemplateVersionSelector,
	recipients RecipientDirectory,
) (*ProductionFanoutPlanFactory, error) {
	if channels == nil {
		return nil, ErrFanoutChannelSelectorUnavailable
	}
	if templates == nil {
		return nil, ErrFanoutTemplateSelectorUnavailable
	}
	if recipients == nil {
		return nil, ErrRecipientDirectoryUnavailable
	}
	return &ProductionFanoutPlanFactory{
		filters: filters, channels: channels, templates: templates, recipients: recipients,
	}, nil
}

func (f *ProductionFanoutPlanFactory) Build(
	ctx context.Context,
	event domain.EventEnvelope,
	subscription domain.Subscription,
) (FanoutPlanDecision, error) {
	if f == nil || f.channels == nil || f.templates == nil || f.recipients == nil || ctx == nil {
		return FanoutPlanDecision{}, ErrFanoutConfigurationInvalid
	}
	if err := validateFanoutSubscription(event, subscription); err != nil {
		return FanoutPlanDecision{}, err
	}
	if err := ctx.Err(); err != nil {
		return FanoutPlanDecision{}, err
	}
	if err := event.VerifyPayloadFingerprint(); err != nil {
		return FanoutPlanDecision{}, err
	}

	if hasSubscriptionFilters(subscription.Filters) {
		if f.filters == nil {
			return FanoutPlanDecision{}, ErrFanoutFilterMatcherUnavailable
		}
		matched, err := f.filters.Matches(ctx, event, subscription)
		if err != nil {
			return FanoutPlanDecision{}, err
		}
		if !matched {
			return FanoutPlanDecision{Matched: false}, nil
		}
	}

	channel, err := f.channels.ResolveForFanout(ctx, subscription)
	if err != nil {
		return FanoutPlanDecision{}, err
	}
	if err := channel.Validate(); err != nil {
		return FanoutPlanDecision{}, err
	}
	if channel.ChannelRef != subscription.ChannelRef || !channel.Enabled {
		return FanoutPlanDecision{}, domain.ErrInvalidChannelConfig
	}

	templateVersion, err := f.templates.ResolveForFanout(
		ctx, subscription.TemplateID, event.EventType, event.SchemaVersion,
	)
	if err != nil {
		return FanoutPlanDecision{}, err
	}
	if templateVersion < 1 {
		return FanoutPlanDecision{}, domain.ErrInvalidDeliveryPlan
	}

	request := RecipientDirectoryRequest{
		Scope: subscription.RecipientScope, OwnerRef: subscription.OwnerRef,
		ChannelRef: channel.ChannelRef, ChannelKind: channel.ChannelKind,
		ConfigVersion: channel.ConfigVersion,
	}
	switch subscription.RecipientScope {
	case domain.RecipientScopeOwner:
		if subscription.RecipientRef != "" {
			return FanoutPlanDecision{}, domain.ErrInvalidRecipientTarget
		}
	case domain.RecipientScopePreconfiguredTarget:
		if !validRecipientDirectoryRef(subscription.RecipientRef) {
			return FanoutPlanDecision{}, domain.ErrInvalidRecipientTarget
		}
		request.RecipientRef = subscription.RecipientRef
	default:
		return FanoutPlanDecision{}, domain.ErrInvalidRecipientTarget
	}

	authorization, err := f.recipients.ResolveAuthorized(ctx, request)
	if errors.Is(err, ErrRecipientNotAuthorized) {
		return FanoutPlanDecision{Matched: false}, nil
	}
	if err != nil {
		return FanoutPlanDecision{}, err
	}
	if err := validateRecipientDirectoryResult(authorization, channel); err != nil {
		return FanoutPlanDecision{}, err
	}
	if subscription.RecipientScope == domain.RecipientScopePreconfiguredTarget &&
		authorization.Target.RecipientRef != subscription.RecipientRef {
		return FanoutPlanDecision{Matched: false}, nil
	}

	plan := domain.DeliveryPlan{
		SourceModule: event.SourceModule, EventID: event.EventID,
		SubscriptionID: subscription.SubscriptionID, SubscriptionVersion: subscription.Version,
		ChannelRef: channel.ChannelRef, ChannelConfigVersion: channel.ConfigVersion,
		TemplateID: subscription.TemplateID, TemplateVersion: templateVersion,
		RecipientRef:                authorization.Target.RecipientRef,
		PayloadFingerprint:          event.PayloadFingerprint,
		AuthorizationPolicyVersions: append([]domain.AuthorizationPolicyVersion(nil), authorization.AuthorizationPolicyVersions...),
		AuthorizationRefs:           append([]string(nil), authorization.AuthorizationRefs...),
	}
	if err := assignDeterministicDeliveryIdentity(&plan); err != nil {
		return FanoutPlanDecision{}, err
	}
	fingerprint, err := plan.CalculateFingerprint()
	if err != nil {
		return FanoutPlanDecision{}, err
	}
	plan.PlanFingerprint = fingerprint
	if err := plan.Validate(); err != nil {
		return FanoutPlanDecision{}, err
	}
	if err := plan.VerifyFingerprint(); err != nil {
		return FanoutPlanDecision{}, err
	}
	if err := ctx.Err(); err != nil {
		return FanoutPlanDecision{}, err
	}
	return FanoutPlanDecision{Matched: true, Plan: plan}, nil
}


// validateFanoutSubscription enforces the frozen Subscription field contract
// before resolving any downstream dependency. Event-specific filter schema lookup
// and matching are still performed by SubscriptionFilterMatcher.
func validateFanoutSubscription(event domain.EventEnvelope, subscription domain.Subscription) error {
	if subscription.Status != domain.SubscriptionActive ||
		subscription.SubscriptionID == "" || subscription.Version < 1 ||
		subscription.OwnerRef == "" || subscription.EventType == "" ||
		subscription.EventType != event.EventType || subscription.ChannelRef == "" ||
		subscription.TemplateID == "" ||
		subscription.CreatedAt.IsZero() || subscription.UpdatedAt.IsZero() || subscription.EffectiveAt.IsZero() ||
		subscription.CreatedAt.Location() != time.UTC || subscription.UpdatedAt.Location() != time.UTC ||
		subscription.EffectiveAt.Location() != time.UTC ||
		subscription.UpdatedAt.Before(subscription.CreatedAt) || subscription.EffectiveAt.Before(subscription.CreatedAt) {
		return domain.ErrInvalidSubscription
	}
	filters := bytes.TrimSpace(subscription.Filters)
	if len(filters) == 0 || bytes.Equal(filters, []byte("null")) {
		if subscription.FilterSchemaVersion != "" {
			return domain.ErrInvalidSubscription
		}
		return nil
	}
	if subscription.FilterSchemaVersion == "" || subscription.FilterSchemaVersion != strings.TrimSpace(subscription.FilterSchemaVersion) {
		return domain.ErrInvalidSubscription
	}
	if err := domain.ValidateSubscriptionFilterDocument(subscription.Filters); err != nil {
		return fmt.Errorf("%w: invalid filter document: %v", domain.ErrInvalidSubscription, err)
	}
	return nil
}

func hasSubscriptionFilters(filters json.RawMessage) bool {
	trimmed := bytes.TrimSpace(filters)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null"))
}

type deliveryPlanIdentityInput struct {
	SourceModule        string `json:"source_module"`
	EventID             string `json:"event_id"`
	SubscriptionID      string `json:"subscription_id"`
	SubscriptionVersion int64  `json:"subscription_version"`
	ChannelRef          string `json:"channel_ref"`
	ChannelVersion      int64  `json:"channel_config_version"`
	TemplateID          string `json:"template_id"`
	TemplateVersion     int64  `json:"template_version"`
	RecipientRef        string `json:"recipient_ref"`
}

func assignDeterministicDeliveryIdentity(plan *domain.DeliveryPlan) error {
	if plan == nil {
		return domain.ErrInvalidDeliveryPlan
	}
	input := deliveryPlanIdentityInput{
		SourceModule: plan.SourceModule, EventID: plan.EventID,
		SubscriptionID: plan.SubscriptionID, SubscriptionVersion: plan.SubscriptionVersion,
		ChannelRef: plan.ChannelRef, ChannelVersion: plan.ChannelConfigVersion,
		TemplateID: plan.TemplateID, TemplateVersion: plan.TemplateVersion,
		RecipientRef: plan.RecipientRef,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("notification: encode delivery identity: %w", err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return fmt.Errorf("notification: canonicalize delivery identity: %w", err)
	}
	sum := sha256.Sum256(canonical)
	digest := hex.EncodeToString(sum[:])
	plan.DeliveryID = "ntf_" + digest[:32]
	plan.DeliveryIdempotencyKey = "notification-delivery-v1-" + digest
	return nil
}

// ValidateFanoutPlanInput is shared by production factory tests and future adapters
// to ensure no channel/template/recipient version is silently substituted.
func ValidateFanoutPlanInput(event domain.EventEnvelope, subscription domain.Subscription, channel domain.ChannelConfigSnapshot, templateVersion int64, recipient RecipientDirectoryResult) error {
	if subscription.EventType != event.EventType || subscription.ChannelRef != channel.ChannelRef ||
		subscription.TemplateID == "" || templateVersion < 1 {
		return domain.ErrInvalidDeliveryPlan
	}
	if err := channel.Validate(); err != nil {
		return err
	}
	if !channel.Enabled {
		return domain.ErrChannelDisabled
	}
	return validateRecipientDirectoryResult(recipient, channel)
}
