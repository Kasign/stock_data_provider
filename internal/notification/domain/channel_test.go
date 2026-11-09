package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func validChannelSnapshot() ChannelConfigSnapshot {
	return ChannelConfigSnapshot{
		ChannelRef: "channel-ref-1", ConfigVersion: 3, ChannelKind: "provider-a",
		Enabled: true, SecretRef: "secret://notification/email/v3",
		Capabilities: ChannelCapabilities{
			SupportsIdempotency: CapabilityUnknown,
			SupportsDeliveryLookup: CapabilityUnsupported,
			SupportsRetryAfter: CapabilitySupported,
			MaxPayloadBytes: 64 * 1024,
		},
		RetryableErrorCodes: []string{"rate_limited", "temporary_unavailable"},
	}
}

func validChannelSendRequest(channel ChannelConfigSnapshot) ChannelSendRequest {
	return ChannelSendRequest{
		DeliveryID: "delivery-1",
		Target: RecipientTarget{
			RecipientRef: "recipient-1", ChannelRef: channel.ChannelRef,
			ConfigVersion: channel.ConfigVersion, AdapterTargetRef: "target-opaque-1",
		},
		Content: RenderedContent{
			TemplateID: "risk-alert", TemplateVersion: 1,
			TemplateDigest: strings.Repeat("a", 64), EventPayloadFingerprint: strings.Repeat("b", 64),
			Format: TemplateFormatText, Subject: "Risk", Body: "Alert",
		},
	}
}

func TestChannelConfigRequiresExplicitCapabilitiesAndPayloadLimit(t *testing.T) {
	channel := validChannelSnapshot()
	if err := channel.Validate(); err != nil {
		t.Fatal(err)
	}
	channel.Capabilities.SupportsRetryAfter = ""
	if !errors.Is(channel.Validate(), ErrInvalidChannelConfig) {
		t.Fatal("empty capability state must not be treated as unsupported")
	}
	channel = validChannelSnapshot()
	channel.Capabilities.MaxPayloadBytes = 0
	if !errors.Is(channel.Validate(), ErrInvalidChannelConfig) {
		t.Fatal("missing payload limit must be rejected")
	}
	channel = validChannelSnapshot()
	channel.RetryableErrorCodes = []string{"rate_limited", "rate_limited"}
	if !errors.Is(channel.Validate(), ErrInvalidChannelConfig) {
		t.Fatal("duplicate retryable codes must be rejected")
	}
	channel = validChannelSnapshot()
	channel.RetryableErrorCodes = []string{"429 Too Many Requests"}
	if !errors.Is(channel.Validate(), ErrInvalidChannelConfig) {
		t.Fatal("retryable codes must be stable machine codes")
	}
}

func TestChannelSendRequestOnlyUsesVerifiedIdempotencyCapability(t *testing.T) {
	channel := validChannelSnapshot()
	request := validChannelSendRequest(channel)
	if err := request.Validate(channel); err != nil {
		t.Fatal(err)
	}
	request.Content.Body = string(make([]byte, channel.Capabilities.MaxPayloadBytes))
	if err := request.Validate(channel); !errors.Is(err, ErrInvalidChannelSendRequest) {
		t.Fatal("rendered content above the channel limit must be rejected")
	}
	request = validChannelSendRequest(channel)
	channel.Enabled = false
	if !errors.Is(request.Validate(channel), ErrChannelDisabled) {
		t.Fatal("disabled channel must be rejected before sending")
	}
	channel.Enabled = true
	request.IdempotencyKey = "delivery-key-1"
	if err := request.Validate(channel); !errors.Is(err, ErrInvalidChannelSendRequest) {
		t.Fatalf("unknown capability must not send idempotency key, got %v", err)
	}

	channel.Capabilities.SupportsIdempotency = CapabilitySupported
	if err := request.Validate(channel); err != nil {
		t.Fatalf("verified support should allow stable key: %v", err)
	}
	request.IdempotencyKey = ""
	if err := request.Validate(channel); !errors.Is(err, ErrInvalidChannelSendRequest) {
		t.Fatal("verified idempotency support requires the stable delivery key")
	}
}

func TestChannelSendResultSeparatesAcceptedRejectedAmbiguousAndNotSent(t *testing.T) {
	channel := validChannelSnapshot()
	cases := []struct {
		name   string
		result ChannelSendResult
		valid  bool
	}{
		{name: "accepted", result: ChannelSendResult{Outcome: ChannelOutcomeAccepted}, valid: true},
		{name: "rejected", result: ChannelSendResult{Outcome: ChannelOutcomeRejected, ErrorCode: "rate_limited"}, valid: true},
		{name: "ambiguous", result: ChannelSendResult{Outcome: ChannelOutcomeAmbiguous, ErrorCode: "timeout_after_write"}, valid: true},
		{name: "not sent", result: ChannelSendResult{Outcome: ChannelOutcomeNotSent, ErrorCode: "cancelled_before_write"}, valid: true},
		{name: "missing error code", result: ChannelSendResult{Outcome: ChannelOutcomeRejected}, valid: false},
		{name: "ambiguous retry-after", result: ChannelSendResult{Outcome: ChannelOutcomeAmbiguous, ErrorCode: "timeout_after_write", RetryAfter: func() *time.Time { value := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC); return &value }()}, valid: false},
		{name: "invalid error code", result: ChannelSendResult{Outcome: ChannelOutcomeRejected, ErrorCode: "429 Too Many Requests"}, valid: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.result.Validate(channel.Capabilities)
			if (err == nil) != testCase.valid {
				t.Fatalf("valid=%v, got error %v", testCase.valid, err)
			}
		})
	}
}

func TestChannelSendResultRetryRequiresExplicitRejectedAllowlist(t *testing.T) {
	channel := validChannelSnapshot()
	rejected := ChannelSendResult{Outcome: ChannelOutcomeRejected, ErrorCode: "rate_limited"}
	if !rejected.IsRetryable(channel) {
		t.Fatal("allowlisted explicit rejection should be retryable")
	}
	for _, result := range []ChannelSendResult{
		{Outcome: ChannelOutcomeRejected, ErrorCode: "unknown_error"},
		{Outcome: ChannelOutcomeAmbiguous, ErrorCode: "timeout_after_write"},
		{Outcome: ChannelOutcomeNotSent, ErrorCode: "cancelled_before_write"},
	} {
		if result.IsRetryable(channel) {
			t.Fatalf("outcome %q must not be automatically retried", result.Outcome)
		}
	}
}

func TestChannelSendResultRetryAfterRequiresVerifiedCapabilityAndUTC(t *testing.T) {
	channel := validChannelSnapshot()
	result := ChannelSendResult{
		Outcome: ChannelOutcomeRejected, ErrorCode: "rate_limited",
		RetryAfter: func() *time.Time {
			value := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
			return &value
		}(),
	}
	if err := result.Validate(channel.Capabilities); err != nil {
		t.Fatal(err)
	}
	channel.Capabilities.SupportsRetryAfter = CapabilityUnknown
	if !errors.Is(result.Validate(channel.Capabilities), ErrInvalidChannelSendResult) {
		t.Fatal("unverified Retry-After must be rejected")
	}
}

func TestRecipientTargetIsBoundToExactChannelVersion(t *testing.T) {
	channel := validChannelSnapshot()
	request := validChannelSendRequest(channel)
	if err := request.Target.Validate(channel); err != nil {
		t.Fatal(err)
	}
	request.Target.ConfigVersion++
	if !errors.Is(request.Target.Validate(channel), ErrInvalidRecipientTarget) {
		t.Fatal("recipient target from another config version must be rejected")
	}
	request = validChannelSendRequest(channel)
	request.Target.AdapterTargetRef = "https://example.com"
	if !errors.Is(request.Target.Validate(channel), ErrInvalidRecipientTarget) {
		t.Fatal("recipient target must not carry a URL or plaintext destination")
	}
}
