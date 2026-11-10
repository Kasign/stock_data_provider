package application

import (
	"context"
	"errors"
	"fmt"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var ErrChannelDeliveryLookupUnavailable = errors.New("notification: channel delivery lookup unavailable")

// BuildChannelDeliveryLookupRequest 从 Delivery 和最后一次模糊 Attempt 构造查询请求。
func BuildChannelDeliveryLookupRequest(
	delivery domain.DeliveryRecord,
	channel domain.ChannelConfigSnapshot,
	attempt domain.DeliveryAttempt,
) (domain.ChannelDeliveryLookupRequest, error) {
	if delivery.Validate() != nil || channel.Validate() != nil || attempt.Validate() != nil ||
		delivery.Status != domain.DeliveryUnknown ||
		delivery.ChannelRef != channel.ChannelRef ||
		delivery.ChannelConfigVersion != channel.ConfigVersion ||
		attempt.DeliveryID != delivery.DeliveryID ||
		attempt.AttemptNo != delivery.AttemptCount ||
		attempt.FinishedAt == nil || attempt.Outcome == nil ||
		*attempt.Outcome != domain.DeliveryAttemptAmbiguous ||
		channel.Capabilities.SupportsDeliveryLookup != domain.CapabilitySupported {
		return domain.ChannelDeliveryLookupRequest{}, domain.ErrInvalidChannelLookupRequest
	}

	request := domain.ChannelDeliveryLookupRequest{
		ChannelRef: channel.ChannelRef, ConfigVersion: channel.ConfigVersion,
		DeliveryID: delivery.DeliveryID, AttemptID: attempt.AttemptID,
	}
	if channel.Capabilities.SupportsIdempotency == domain.CapabilitySupported {
		request.IdempotencyKey = delivery.DeliveryIdempotencyKey
	}
	if attempt.ProviderMessageRef != nil {
		request.ProviderMessageRef = *attempt.ProviderMessageRef
	}
	if err := request.Validate(channel.Capabilities); err != nil {
		return domain.ChannelDeliveryLookupRequest{}, err
	}
	return request, nil
}

// LookupUnknownDelivery 只通过 Registry 中精确绑定的 Adapter 查询，
// 并将已校验结果转换为 Unknown 对账证据。查询错误原样返回，不能视为未接受。
func (r *ChannelRegistry) LookupUnknownDelivery(
	ctx context.Context,
	delivery domain.DeliveryRecord,
	channel domain.ChannelConfigSnapshot,
	attempt domain.DeliveryAttempt,
) (UnknownDeliveryEvidence, error) {
	request, err := BuildChannelDeliveryLookupRequest(delivery, channel, attempt)
	if err != nil {
		return UnknownDeliveryEvidence{}, err
	}
	lookup, err := r.ResolveDeliveryLookup(ctx, request.ChannelRef, request.ConfigVersion)
	if err != nil {
		return UnknownDeliveryEvidence{}, err
	}
	result, err := lookup.LookupDelivery(ctx, request)
	if err != nil {
		return UnknownDeliveryEvidence{}, err
	}

	evidence := UnknownDeliveryEvidence{
		DeliveryID: result.DeliveryID, AttemptID: result.AttemptID,
		EvidenceRef: result.EvidenceRef, ObservedAt: result.ObservedAt,
		ProviderMessageRef: result.ProviderMessageRef, ErrorCode: result.ErrorCode,
	}
	switch result.Outcome {
	case domain.ChannelLookupAccepted:
		evidence.Outcome = UnknownLookupAccepted
	case domain.ChannelLookupNotAccepted:
		evidence.Outcome = UnknownLookupNotAccepted
	case domain.ChannelLookupIndeterminate:
		evidence.Outcome = UnknownLookupIndeterminate
	default:
		return UnknownDeliveryEvidence{}, fmt.Errorf("%w: unsupported lookup outcome", domain.ErrInvalidChannelLookupResult)
	}
	return evidence, nil
}

// ResolveDeliveryLookup 只解析精确版本，并在能力声明与 Adapter 实现不一致时失败关闭。
func (r *ChannelRegistry) ResolveDeliveryLookup(ctx context.Context, channelRef string, configVersion int64) (ChannelDeliveryLookup, error) {
	if r == nil || ctx == nil {
		return nil, ErrChannelDeliveryLookupUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	registration, exists := r.registrations[channelKey{ref: channelRef, version: configVersion}]
	r.mu.RUnlock()
	if !exists {
		return nil, ErrChannelVersionNotFound
	}
	if registration.snapshot.Capabilities.SupportsDeliveryLookup != domain.CapabilitySupported {
		return nil, ErrChannelDeliveryLookupUnavailable
	}
	lookup, ok := registration.adapter.(ChannelDeliveryLookup)
	if !ok || lookup == nil {
		return nil, ErrChannelDeliveryLookupUnavailable
	}
	return &boundChannelDeliveryLookup{
		snapshot: cloneChannelSnapshot(registration.snapshot),
		lookup:   lookup,
	}, nil
}

type boundChannelDeliveryLookup struct {
	snapshot domain.ChannelConfigSnapshot
	lookup   ChannelDeliveryLookup
}

func (a *boundChannelDeliveryLookup) LookupDelivery(ctx context.Context, request domain.ChannelDeliveryLookupRequest) (domain.ChannelDeliveryLookupResult, error) {
	if a == nil || a.lookup == nil || ctx == nil {
		return domain.ChannelDeliveryLookupResult{}, ErrChannelDeliveryLookupUnavailable
	}
	if err := ctx.Err(); err != nil {
		return domain.ChannelDeliveryLookupResult{}, err
	}
	if a.snapshot.Capabilities.SupportsDeliveryLookup != domain.CapabilitySupported ||
		request.Validate(a.snapshot.Capabilities) != nil ||
		request.ChannelRef != a.snapshot.ChannelRef ||
		request.ConfigVersion != a.snapshot.ConfigVersion {
		return domain.ChannelDeliveryLookupResult{}, domain.ErrInvalidChannelLookupRequest
	}
	result, err := a.lookup.LookupDelivery(ctx, request)
	if err != nil {
		return domain.ChannelDeliveryLookupResult{}, err
	}
	if validationErr := result.Validate(request); validationErr != nil {
		return domain.ChannelDeliveryLookupResult{}, fmt.Errorf("%w: %v", domain.ErrInvalidChannelLookupResult, validationErr)
	}
	return result, nil
}
