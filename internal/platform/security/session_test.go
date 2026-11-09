package security

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestSessionTokenAndDigest(t *testing.T) {
	raw, digest, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 43 || !validSessionDigest(digest) {
		t.Fatalf("unexpected token/digest: %d %q", len(raw), digest)
	}
	got, err := SessionTokenDigest(raw)
	if err != nil || got != digest {
		t.Fatalf("digest = %q, %v; want %q", got, err, digest)
	}
	if _, err := SessionTokenDigest(raw + "="); err == nil {
		t.Fatal("padded token must be rejected")
	}
	if _, err := SessionTokenDigest("not-a-session-token"); err == nil {
		t.Fatal("malformed token must be rejected")
	}
}

func TestSessionRecordValidation(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	record := SessionRecord{
		Subject: "owner", CreatedAt: now, LastSeenAt: now,
		IdleExpiresAt:     now.Add(DefaultSessionIdleTTL),
		AbsoluteExpiresAt: now.Add(DefaultSessionAbsoluteTTL), CredentialVersion: 1,
	}
	if err := record.validate(); err != nil {
		t.Fatalf("valid record rejected: %v", err)
	}
	record.AbsoluteExpiresAt = now.Add(DefaultSessionAbsoluteTTL + time.Second)
	if err := record.validate(); err == nil {
		t.Fatal("record exceeding absolute lifetime must be rejected")
	}
}

func TestSessionCookieHelpers(t *testing.T) {
	config := SessionCookieConfig{
		Name: "kasign_session", Path: "/market-api", Secure: true,
		SameSite: http.SameSiteLaxMode,
	}
	raw, _, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	cookie, err := NewSessionCookie(config, raw, now.Add(time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode ||
		cookie.Path != config.Path || cookie.Domain != "" {
		t.Fatalf("cookie security attributes are incorrect: %+v", cookie)
	}
	cleared, err := ClearSessionCookie(config)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Name != cookie.Name || cleared.Path != cookie.Path || cleared.MaxAge >= 0 {
		t.Fatalf("clear cookie scope does not match: %+v", cleared)
	}
	config.Path = "/"
	if _, err := ClearSessionCookie(config); err == nil {
		t.Fatal("root cookie path must be rejected")
	}
}

func TestRedisSessionStoreIntegration(t *testing.T) {
	if os.Getenv("PLATFORM_STORAGE_INTEGRATION") != "1" {
		t.Skip("set PLATFORM_STORAGE_INTEGRATION=1 to run real Redis integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := redis.NewClient(&redis.Options{Addr: os.Getenv("PLATFORM_REDIS_ADDR")})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		t.Fatalf("connect to Redis: %v", err)
	}
	defer client.Close()
	prefix := "platform:session-it:" + time.Now().Format("20060102150405.000000000")
	store, err := NewRedisSessionStore(client, prefix)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	_, digest, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	record := SessionRecord{
		Subject: "owner", CreatedAt: now, LastSeenAt: now,
		IdleExpiresAt:     now.Add(DefaultSessionIdleTTL),
		AbsoluteExpiresAt: now.Add(DefaultSessionAbsoluteTTL), CredentialVersion: 1,
	}
	if err := store.Create(ctx, digest, record); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.Create(ctx, digest, record); !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("duplicate create = %v", err)
	}
	touchedAt := now.Add(2 * time.Minute)
	touched, err := store.LookupAndTouch(ctx, digest, touchedAt, DefaultSessionIdleTTL)
	if err != nil {
		t.Fatalf("touch: %v", err)
	}
	if !touched.LastSeenAt.Equal(touchedAt) ||
		!touched.IdleExpiresAt.Equal(touchedAt.Add(DefaultSessionIdleTTL)) ||
		!touched.AbsoluteExpiresAt.Equal(record.AbsoluteExpiresAt) {
		t.Fatalf("touch expiry mismatch: %+v", touched)
	}
	_, digestA, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	_, digestB, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	rotated := SessionRecord{
		Subject: "owner", CreatedAt: touchedAt, LastSeenAt: touchedAt,
		IdleExpiresAt:     touchedAt.Add(DefaultSessionIdleTTL),
		AbsoluteExpiresAt: record.AbsoluteExpiresAt, CredentialVersion: 1,
	}
	var winners atomic.Int32
	var wait sync.WaitGroup
	for _, candidate := range []string{digestA, digestB} {
		candidate := candidate
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := store.Rotate(ctx, digest, candidate, rotated, touchedAt)
			if err == nil {
				winners.Add(1)
			} else if !errors.Is(err, ErrSessionNotFound) && !errors.Is(err, ErrSessionConflict) {
				t.Errorf("unexpected rotate error: %v", err)
			}
		}()
	}
	wait.Wait()
	if winners.Load() != 1 {
		t.Fatalf("concurrent rotations succeeded %d times, want one", winners.Load())
	}
	if _, err := store.LookupAndTouch(ctx, digest, touchedAt, DefaultSessionIdleTTL); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("old token still valid: %v", err)
	}
	winner := digestA
	if _, err := store.LookupAndTouch(ctx, winner, touchedAt, DefaultSessionIdleTTL); err != nil {
		winner = digestB
		if _, retryErr := store.LookupAndTouch(ctx, winner, touchedAt, DefaultSessionIdleTTL); retryErr != nil {
			t.Fatalf("neither rotated token valid: %v", retryErr)
		}
	}
	if err := store.Revoke(ctx, winner); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := store.LookupAndTouch(ctx, winner, touchedAt, DefaultSessionIdleTTL); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("revoked token still valid: %v", err)
	}
}
