package application

import (
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

func TestBuildChannelSendRequestBindsFrozenDeliverySnapshot(t *testing.T) {
	delivery := validDeliveryForRepository()
	channel := completionChannel()
	channel.Capabilities.SupportsIdempotency = domain.CapabilitySupported
	recipient := domain.RecipientTarget{
		RecipientRef: delivery.RecipientRef,
		ChannelRef: delivery.ChannelRef,
		ConfigVersion: delivery.ChannelConfigVersion,
		AdapterTargetRef: "opaque-target-1",
	}
	content := domain.RenderedContent{
		TemplateID: delivery.TemplateID,
		TemplateVersion: delivery.TemplateVersion,
		TemplateDigest: strings.Repeat("a", 64),
		EventPayloadFingerprint: delivery.PayloadFingerprint,
		Format: domain.TemplateFormatText,
		Subject: "subject",
		Body: "body",
	}
	request, err := BuildChannelSendRequest(delivery, channel, recipient, content)
	if err != nil {
		t.Fatalf("build valid channel request: %v", err)
	}
	if request.DeliveryID != delivery.DeliveryID ||
		request.IdempotencyKey != delivery.DeliveryIdempotencyKey ||
		request.Target.RecipientRef != delivery.RecipientRef {
		t.Fatalf("request does not preserve delivery snapshot: %+v", request)
	}
}

func TestBuildChannelSendRequestRejectsCrossDeliverySnapshots(t *testing.T) {
	delivery := validDeliveryForRepository()
	channel := completionChannel()
	recipient := domain.RecipientTarget{
		RecipientRef: delivery.RecipientRef,
		ChannelRef: delivery.ChannelRef,
		ConfigVersion: delivery.ChannelConfigVersion,
		AdapterTargetRef: "opaque-target-1",
	}
	content := domain.RenderedContent{
		TemplateID: delivery.TemplateID,
		TemplateVersion: delivery.TemplateVersion,
		TemplateDigest: strings.Repeat("a", 64),
		EventPayloadFingerprint: delivery.PayloadFingerprint,
		Format: domain.TemplateFormatText,
		Subject: "subject",
		Body: "body",
	}
	tests := []struct {
		name string
		mutate func(*domain.DeliveryRecord, *domain.ChannelConfigSnapshot, *domain.RecipientTarget, *domain.RenderedContent)
	}{
		{name: "wrong channel version", mutate: func(_ *domain.DeliveryRecord, c *domain.ChannelConfigSnapshot, _ *domain.RecipientTarget, _ *domain.RenderedContent) { c.ConfigVersion++ }},
		{name: "wrong recipient", mutate: func(_ *domain.DeliveryRecord, _ *domain.ChannelConfigSnapshot, r *domain.RecipientTarget, _ *domain.RenderedContent) { r.RecipientRef = "other-recipient" }},
		{name: "wrong template version", mutate: func(_ *domain.DeliveryRecord, _ *domain.ChannelConfigSnapshot, _ *domain.RecipientTarget, c *domain.RenderedContent) { c.TemplateVersion++ }},
		{name: "wrong payload fingerprint", mutate: func(_ *domain.DeliveryRecord, _ *domain.ChannelConfigSnapshot, _ *domain.RecipientTarget, c *domain.RenderedContent) { c.EventPayloadFingerprint = strings.Repeat("b", 64) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d, ch, r, c := delivery, channel, recipient, content
			test.mutate(&d, &ch, &r, &c)
			if _, err := BuildChannelSendRequest(d, ch, r, c); err == nil {
				t.Fatal("cross-delivery data must be rejected")
			}
		})
	}
}

func TestBuildChannelSendRequestDoesNotSendUnverifiedIdempotencyKey(t *testing.T) {
	delivery := validDeliveryForRepository()
	channel := completionChannel()
	channel.Capabilities.SupportsIdempotency = domain.CapabilityUnknown
	recipient := domain.RecipientTarget{
		RecipientRef: delivery.RecipientRef,
		ChannelRef: delivery.ChannelRef,
		ConfigVersion: delivery.ChannelConfigVersion,
		AdapterTargetRef: "opaque-target-1",
	}
	content := domain.RenderedContent{
		TemplateID: delivery.TemplateID,
		TemplateVersion: delivery.TemplateVersion,
		TemplateDigest: strings.Repeat("a", 64),
		EventPayloadFingerprint: delivery.PayloadFingerprint,
		Format: domain.TemplateFormatText,
		Subject: "subject",
		Body: "body",
	}
	request, err := BuildChannelSendRequest(delivery, channel, recipient, content)
	if err != nil {
		t.Fatal(err)
	}
	if request.IdempotencyKey != "" {
		t.Fatal("unverified channel idempotency capability must not receive an idempotency key")
	}
}
