package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

type channelConfigRepositoryStub struct {
	snapshot domain.ChannelConfigSnapshot
	digest string
	found bool
	err error
}

func (s channelConfigRepositoryStub) Create(context.Context, domain.ChannelConfigSnapshot) (string, bool, error) {
	return s.digest, false, nil
}

func (s channelConfigRepositoryStub) Get(context.Context, string, int64) (domain.ChannelConfigSnapshot, string, bool, error) {
	return s.snapshot, s.digest, s.found, s.err
}

func validConfigRepositorySnapshot() domain.ChannelConfigSnapshot {
	return domain.ChannelConfigSnapshot{
		ChannelRef: "channel-1", ConfigVersion: 3, ChannelKind: "test", Enabled: true,
		SecretRef: "secret://notification/channel-1/v3",
		Capabilities: domain.ChannelCapabilities{
			SupportsIdempotency: domain.CapabilityUnknown,
			SupportsDeliveryLookup: domain.CapabilityUnknown,
			SupportsRetryAfter: domain.CapabilityUnsupported,
			MaxPayloadBytes: 4096,
		},
		RetryableErrorCodes: []string{"rate_limited"},
	}
}

func TestLoadChannelConfigSnapshotValidatesExactPersistedVersion(t *testing.T) {
	snapshot := validConfigRepositorySnapshot()
	repository := channelConfigRepositoryStub{snapshot: snapshot, digest: strings.Repeat("a", 64), found: true}
	loaded, err := LoadChannelConfigSnapshot(context.Background(), repository, snapshot.ChannelRef, snapshot.ConfigVersion)
	if err != nil {
		t.Fatal(err)
	}
	loaded.RetryableErrorCodes[0] = "mutated"
	if repository.snapshot.RetryableErrorCodes[0] != "rate_limited" {
		t.Fatal("loaded snapshot must not share mutable slices with repository result")
	}
}

func TestLoadChannelConfigSnapshotClassifiesUnavailableMissingAndCorrupt(t *testing.T) {
	snapshot := validConfigRepositorySnapshot()
	if _, err := LoadChannelConfigSnapshot(context.Background(), nil, snapshot.ChannelRef, snapshot.ConfigVersion); !errors.Is(err, ErrChannelConfigRepositoryUnavailable) {
		t.Fatalf("nil repository must be classified: %v", err)
	}
	repository := channelConfigRepositoryStub{snapshot: snapshot, digest: strings.Repeat("a", 64)}
	if _, err := LoadChannelConfigSnapshot(context.Background(), repository, snapshot.ChannelRef, snapshot.ConfigVersion); !errors.Is(err, ErrChannelConfigNotFound) {
		t.Fatalf("missing exact version must be classified: %v", err)
	}
	repository.found = true
	repository.digest = "bad-digest"
	if _, err := LoadChannelConfigSnapshot(context.Background(), repository, snapshot.ChannelRef, snapshot.ConfigVersion); !errors.Is(err, ErrChannelConfigCorrupt) {
		t.Fatalf("invalid digest must be classified as corrupt: %v", err)
	}
	repository.digest = strings.Repeat("a", 64)
	repository.snapshot.ChannelRef = "other-channel"
	if _, err := LoadChannelConfigSnapshot(context.Background(), repository, snapshot.ChannelRef, snapshot.ConfigVersion); !errors.Is(err, ErrChannelConfigCorrupt) {
		t.Fatalf("identity mismatch must be classified as corrupt: %v", err)
	}
}

func TestLoadChannelConfigSnapshotPropagatesRepositoryAndContextErrors(t *testing.T) {
	snapshot := validConfigRepositorySnapshot()
	repositoryErr := errors.New("database unavailable")
	repository := channelConfigRepositoryStub{snapshot: snapshot, digest: strings.Repeat("a", 64), found: true, err: repositoryErr}
	if _, err := LoadChannelConfigSnapshot(context.Background(), repository, snapshot.ChannelRef, snapshot.ConfigVersion); !errors.Is(err, repositoryErr) {
		t.Fatalf("repository error must be preserved: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := LoadChannelConfigSnapshot(ctx, repository, snapshot.ChannelRef, snapshot.ConfigVersion); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context must be returned before repository access: %v", err)
	}
}
