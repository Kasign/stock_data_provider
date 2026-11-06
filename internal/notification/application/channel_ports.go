package application

import (
	"context"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

// ChannelAdapter is bound to one immutable channel configuration version.
// Implementations receive Platform config.Secret and a policy-bound HTTP
// doer through construction; plaintext secrets and destinations stay inside
// the concrete adapter. ConfigSnapshot must return the exact immutable version
// used by Send. If Send returns an error, Outcome must still be classified;
// use ambiguous unless the adapter can prove the request was not sent.
type ChannelAdapter interface {
	ConfigSnapshot() domain.ChannelConfigSnapshot
	Send(ctx context.Context, request domain.ChannelSendRequest) (domain.ChannelSendResult, error)
}

// ChannelResolver resolves an exact channel configuration version. It must
// never silently substitute the latest version.
type ChannelResolver interface {
	Resolve(ctx context.Context, channelRef string, configVersion int64) (ChannelAdapter, error)
}

// RecipientResolver converts a Subscription into an opaque adapter target.
// The returned target must not contain a plaintext address or arbitrary URL.
type RecipientResolver interface {
	Resolve(ctx context.Context, subscription domain.Subscription, channel domain.ChannelConfigSnapshot) (domain.RecipientTarget, error)
}
