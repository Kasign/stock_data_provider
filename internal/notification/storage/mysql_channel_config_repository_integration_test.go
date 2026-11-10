package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/application"
	"example.com/stock-analysis-flow/internal/notification/domain"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

func TestNotificationMariaDBChannelConfigRepositoryIntegration(t *testing.T) {
	dsn := os.Getenv("NOTIFICATION_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NOTIFICATION_MARIADB_TEST_DSN to run Notification MariaDB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := openNotificationIntegrationDB(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB through Platform Storage: %v", err)
	}
	defer db.Close()
	if err := platformstorage.ApplyMigrations(ctx, db, "notification", Migrations()); err != nil {
		t.Fatalf("apply Notification migrations: %v", err)
	}
	repository, err := NewMySQLChannelConfigRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := domain.ChannelConfigSnapshot{
		ChannelRef: fmt.Sprintf("channel-%d", time.Now().UnixNano()), ConfigVersion: 1,
		ChannelKind: "provider-a", Enabled: true, SecretRef: "secret://notification/provider-a/v1",
		Capabilities: domain.ChannelCapabilities{
			SupportsIdempotency:    domain.CapabilityUnknown,
			SupportsDeliveryLookup: domain.CapabilityUnsupported,
			SupportsRetryAfter:     domain.CapabilitySupported,
			MaxPayloadBytes:        65536,
		},
		RetryableErrorCodes: []string{"temporary_unavailable", "rate_limited"},
	}
	digest, duplicate, err := repository.Create(ctx, snapshot)
	if err != nil || duplicate || digest == "" {
		t.Fatalf("create channel config: digest=%q duplicate=%v err=%v", digest, duplicate, err)
	}
	reordered := snapshot
	reordered.RetryableErrorCodes = []string{"rate_limited", "temporary_unavailable"}
	repeatedDigest, duplicate, err := repository.Create(ctx, reordered)
	if err != nil || !duplicate || repeatedDigest != digest {
		t.Fatalf("retryable code order must be canonical: digest=%q duplicate=%v err=%v", repeatedDigest, duplicate, err)
	}
	changed := snapshot
	changed.Enabled = false
	if _, _, err := repository.Create(ctx, changed); !errors.Is(err, application.ErrChannelConfigVersionConflict) {
		t.Fatalf("changed same version must conflict: %v", err)
	}
	next := changed
	next.ConfigVersion = 2
	if _, duplicate, err := repository.Create(ctx, next); err != nil || duplicate {
		t.Fatalf("new config version should be accepted: duplicate=%v err=%v", duplicate, err)
	}
	loaded, loadedDigest, found, err := repository.Get(ctx, snapshot.ChannelRef, 1)
	if err != nil || !found || loadedDigest != digest || loaded.Enabled != snapshot.Enabled {
		t.Fatalf("load channel config: found=%v snapshot=%+v digest=%q err=%v", found, loaded, loadedDigest, err)
	}
	loaded.RetryableErrorCodes[0] = "mutated"
	reloaded, _, found, err := repository.Get(ctx, snapshot.ChannelRef, 1)
	if err != nil || !found || reloaded.RetryableErrorCodes[0] == "mutated" {
		t.Fatalf("returned config must be isolated from persisted snapshot: %+v err=%v", reloaded, err)
	}
}
