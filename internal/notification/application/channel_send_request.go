package application

import (
	"example.com/stock-analysis-flow/internal/notification/domain"
)

// BuildChannelSendRequest creates the only supported boundary between a frozen
// Delivery plan and a concrete channel call. It prevents callers from mixing
// content, recipients, or idempotency keys from another Delivery.
func BuildChannelSendRequest(
	delivery domain.DeliveryRecord,
	channel domain.ChannelConfigSnapshot,
	recipient domain.RecipientTarget,
	content domain.RenderedContent,
) (domain.ChannelSendRequest, error) {
	if err := delivery.Validate(); err != nil {
		return domain.ChannelSendRequest{}, err
	}
	if err := channel.Validate(); err != nil {
		return domain.ChannelSendRequest{}, err
	}
	if channel.ChannelRef != delivery.ChannelRef ||
		channel.ConfigVersion != delivery.ChannelConfigVersion ||
		!channel.Enabled ||
		recipient.RecipientRef != delivery.RecipientRef ||
		recipient.ChannelRef != delivery.ChannelRef ||
		recipient.ConfigVersion != delivery.ChannelConfigVersion ||
		content.TemplateID != delivery.TemplateID ||
		content.TemplateVersion != delivery.TemplateVersion ||
		content.EventPayloadFingerprint != delivery.PayloadFingerprint {
		return domain.ChannelSendRequest{}, domain.ErrInvalidChannelSendRequest
	}

	request := domain.ChannelSendRequest{
		DeliveryID: delivery.DeliveryID,
		Target: recipient,
		Content: content,
	}
	if channel.Capabilities.SupportsIdempotency == domain.CapabilitySupported {
		request.IdempotencyKey = delivery.DeliveryIdempotencyKey
	}
	if err := request.Validate(channel); err != nil {
		return domain.ChannelSendRequest{}, err
	}
	return request, nil
}
