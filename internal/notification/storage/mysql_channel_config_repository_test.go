package storage

import (
	"testing"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

func TestChannelConfigDigestCanonicalizesRetryableErrorCodeSet(t *testing.T) {
	snapshot := domain.ChannelConfigSnapshot{
		ChannelRef: "channel-1", ConfigVersion: 1, ChannelKind: "provider-a", Enabled: true,
		SecretRef: "secret://notification/provider-a/v1",
		Capabilities: domain.ChannelCapabilities{
			SupportsIdempotency: domain.CapabilityUnknown,
			SupportsDeliveryLookup: domain.CapabilityUnsupported,
			SupportsRetryAfter: domain.CapabilitySupported,
			MaxPayloadBytes: 65536,
		},
		RetryableErrorCodes: []string{"temporary_unavailable", "rate_limited"},
	}
	firstDigest, _, err := channelConfigDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.RetryableErrorCodes = []string{"rate_limited", "temporary_unavailable"}
	secondDigest, _, err := channelConfigDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest != secondDigest {
		t.Fatalf("set order must not change immutable config digest: first=%s second=%s", firstDigest, secondDigest)
	}
	snapshot.Enabled = false
	changedDigest, _, err := channelConfigDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if changedDigest == firstDigest {
		t.Fatal("changing an immutable config field must change its digest")
	}
}

func TestNormalizeChannelConfigClonesAndSortsErrorCodes(t *testing.T) {
	source := domain.ChannelConfigSnapshot{RetryableErrorCodes: []string{"z_error", "a_error"}}
	normalized := normalizeChannelConfig(source)
	if normalized.RetryableErrorCodes[0] != "a_error" || normalized.RetryableErrorCodes[1] != "z_error" {
		t.Fatalf("error codes were not sorted: %v", normalized.RetryableErrorCodes)
	}
	normalized.RetryableErrorCodes[0] = "changed"
	if source.RetryableErrorCodes[0] != "z_error" {
		t.Fatal("normalization must not mutate caller-owned slices")
	}
}
