package application

import (
	"context"
	"errors"
	"strings"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var ErrRecipientDirectoryUnavailable = errors.New("notification: recipient directory is unavailable")

// RecipientDirectoryRequest is the complete, non-secret lookup identity.
// Implementations must perform current authorization checks and return only opaque target references.
type RecipientDirectoryRequest struct {
	Scope         domain.RecipientScope
	OwnerRef      string
	RecipientRef  string
	ChannelRef    string
	ChannelKind   string
	ConfigVersion int64
}

// RecipientDirectoryResult carries the authorized opaque target and the policy evidence
// that must be frozen into DeliveryPlan. It must never contain plaintext addresses or credentials.
type RecipientDirectoryResult struct {
	Target                      domain.RecipientTarget
	AuthorizationPolicyVersions []domain.AuthorizationPolicyVersion
	AuthorizationRefs           []string
}

// RecipientDirectory is the Notification-owned read Port for the project's identity directory.
// Server supplies the concrete adapter to the actual identity source during composition.
type RecipientDirectory interface {
	ResolveAuthorized(ctx context.Context, request RecipientDirectoryRequest) (RecipientDirectoryResult, error)
}

// DirectoryRecipientResolver adapts RecipientDirectory to the send-time RecipientResolver Port.
type DirectoryRecipientResolver struct {
	directory RecipientDirectory
}

func NewDirectoryRecipientResolver(directory RecipientDirectory) (*DirectoryRecipientResolver, error) {
	if directory == nil {
		return nil, ErrRecipientDirectoryUnavailable
	}
	return &DirectoryRecipientResolver{directory: directory}, nil
}

func (r *DirectoryRecipientResolver) Resolve(
	ctx context.Context,
	subscription domain.Subscription,
	channel domain.ChannelConfigSnapshot,
) (domain.RecipientTarget, error) {
	if r == nil || r.directory == nil || ctx == nil {
		return domain.RecipientTarget{}, ErrRecipientDirectoryUnavailable
	}
	if err := ctx.Err(); err != nil {
		return domain.RecipientTarget{}, err
	}
	if err := channel.Validate(); err != nil {
		return domain.RecipientTarget{}, err
	}
	if subscription.SubscriptionID == "" || subscription.OwnerRef == "" ||
		subscription.ChannelRef != channel.ChannelRef {
		return domain.RecipientTarget{}, domain.ErrInvalidRecipientTarget
	}

	request := RecipientDirectoryRequest{
		Scope:      subscription.RecipientScope,
		OwnerRef:   subscription.OwnerRef,
		ChannelRef: channel.ChannelRef, ChannelKind: channel.ChannelKind,
		ConfigVersion: channel.ConfigVersion,
	}
	switch subscription.RecipientScope {
	case domain.RecipientScopeOwner:
		if subscription.RecipientRef != "" {
			return domain.RecipientTarget{}, domain.ErrInvalidRecipientTarget
		}
	case domain.RecipientScopePreconfiguredTarget:
		if !validRecipientDirectoryRef(subscription.RecipientRef) {
			return domain.RecipientTarget{}, domain.ErrInvalidRecipientTarget
		}
		request.RecipientRef = subscription.RecipientRef
	default:
		return domain.RecipientTarget{}, domain.ErrInvalidRecipientTarget
	}

	resolved, err := r.directory.ResolveAuthorized(ctx, request)
	if err != nil {
		return domain.RecipientTarget{}, err
	}
	if err := validateRecipientDirectoryResult(resolved, channel); err != nil {
		return domain.RecipientTarget{}, err
	}
	if subscription.RecipientScope == domain.RecipientScopePreconfiguredTarget &&
		resolved.Target.RecipientRef != subscription.RecipientRef {
		return domain.RecipientTarget{}, ErrRecipientNotAuthorized
	}
	if err := ctx.Err(); err != nil {
		return domain.RecipientTarget{}, err
	}
	return resolved.Target, nil
}

func validateRecipientDirectoryResult(result RecipientDirectoryResult, channel domain.ChannelConfigSnapshot) error {
	if err := result.Target.Validate(channel); err != nil {
		return err
	}
	seenPolicies := make(map[domain.AuthorizationPolicyVersion]struct{}, len(result.AuthorizationPolicyVersions))
	for _, policy := range result.AuthorizationPolicyVersions {
		if !validRecipientDirectoryRef(policy.PolicyType) ||
			!validRecipientDirectoryRef(policy.PolicyRef) ||
			!validRecipientDirectoryRef(policy.PolicyVersion) {
			return domain.ErrInvalidDeliveryPlan
		}
		if _, exists := seenPolicies[policy]; exists {
			return domain.ErrInvalidDeliveryPlan
		}
		seenPolicies[policy] = struct{}{}
	}
	seenRefs := make(map[string]struct{}, len(result.AuthorizationRefs))
	for _, ref := range result.AuthorizationRefs {
		if !validRecipientDirectoryRef(ref) {
			return domain.ErrInvalidDeliveryPlan
		}
		if _, exists := seenRefs[ref]; exists {
			return domain.ErrInvalidDeliveryPlan
		}
		seenRefs[ref] = struct{}{}
	}
	if len(result.AuthorizationRefs) > 0 && len(result.AuthorizationPolicyVersions) == 0 {
		return domain.ErrInvalidDeliveryPlan
	}
	return nil
}

func validRecipientDirectoryRef(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= 256
}
