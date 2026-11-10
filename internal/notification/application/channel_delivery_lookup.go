package application

import (
	"context"
	"errors"
	"fmt"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var ErrChannelDeliveryLookupUnavailable = errors.New("notification: channel delivery lookup unavailable")

// ResolveDeliveryLookup 只解析精确版本，并在能力声明与 Adapter 实现不一致时失败关闭。
func (r *ChannelRegistry) ResolveDeliveryLookup(ctx context.Context, channelRef string, configVersion int64) (ChannelDeliveryLookup, error) {
	if r == nil || ctx == nil { return nil, ErrChannelDeliveryLookupUnavailable }
	if err := ctx.Err(); err != nil { return nil, err }
	r.mu.RLock()
	registration, exists := r.registrations[channelKey{ref: channelRef, version: configVersion}]
	r.mu.RUnlock()
	if !exists { return nil, ErrChannelVersionNotFound }
	if registration.snapshot.Capabilities.SupportsDeliveryLookup != domain.CapabilitySupported {
		return nil, ErrChannelDeliveryLookupUnavailable
	}
	lookup, ok := registration.adapter.(ChannelDeliveryLookup)
	if !ok || lookup == nil { return nil, ErrChannelDeliveryLookupUnavailable }
	return &boundChannelDeliveryLookup{snapshot:cloneChannelSnapshot(registration.snapshot), lookup:lookup}, nil
}

type boundChannelDeliveryLookup struct {
	snapshot domain.ChannelConfigSnapshot
	lookup ChannelDeliveryLookup
}

func (a *boundChannelDeliveryLookup) LookupDelivery(ctx context.Context, request domain.ChannelDeliveryLookupRequest) (domain.ChannelDeliveryLookupResult, error) {
	if a == nil || a.lookup == nil || ctx == nil { return domain.ChannelDeliveryLookupResult{}, ErrChannelDeliveryLookupUnavailable }
	if err := ctx.Err(); err != nil { return domain.ChannelDeliveryLookupResult{}, err }
	if a.snapshot.Capabilities.SupportsDeliveryLookup != domain.CapabilitySupported ||
		request.Validate(a.snapshot.Capabilities) != nil ||
		request.ChannelRef != a.snapshot.ChannelRef || request.ConfigVersion != a.snapshot.ConfigVersion {
		return domain.ChannelDeliveryLookupResult{}, domain.ErrInvalidChannelLookupRequest
	}
	result, err := a.lookup.LookupDelivery(ctx, request)
	if err != nil { return domain.ChannelDeliveryLookupResult{}, err }
	if validationErr := result.Validate(request); validationErr != nil {
		return domain.ChannelDeliveryLookupResult{}, fmt.Errorf("%w: %v", domain.ErrInvalidChannelLookupResult, validationErr)
	}
	return result, nil
}
