package security

import (
	"testing"
	"time"
)

func TestCSRFTokenDigestAndVerification(t *testing.T) {
	raw, digest, err := NewCSRFToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 43 || !validSessionDigest(digest) {
		t.Fatalf("unexpected CSRF token or digest: token length=%d digest=%q", len(raw), digest)
	}
	got, err := CSRFTokenDigest(raw)
	if err != nil || got != digest {
		t.Fatalf("CSRFTokenDigest() = %q, %v; want %q", got, err, digest)
	}
	if !VerifyCSRFToken(raw, digest) {
		t.Fatal("the token must verify against its own session digest")
	}
	otherToken, _, err := NewCSRFToken()
	if err != nil {
		t.Fatal(err)
	}
	if VerifyCSRFToken(otherToken, digest) {
		t.Fatal("a token from another session must be rejected")
	}
	if VerifyCSRFToken(raw+"=", digest) {
		t.Fatal("padded token must be rejected")
	}
	if VerifyCSRFToken(raw, "not-a-digest") {
		t.Fatal("malformed stored digest must be rejected")
	}
	if _, err := CSRFTokenDigest(raw + "="); err == nil {
		t.Fatal("CSRFTokenDigest must reject non-canonical tokens")
	}
}

func TestSessionRecordAcceptsCSRFTokenDigest(t *testing.T) {
	raw, digest, err := NewCSRFToken()
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyCSRFToken(raw, digest) {
		t.Fatal("generated CSRF token failed verification")
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	record := SessionRecord{
		Subject: "owner", CreatedAt: now, LastSeenAt: now,
		IdleExpiresAt: now.Add(DefaultSessionIdleTTL),
		AbsoluteExpiresAt: now.Add(DefaultSessionAbsoluteTTL),
		CredentialVersion: 1, CSRFTokenDigest: digest,
	}
	if err := record.validate(); err != nil {
		t.Fatalf("session record rejected valid CSRF digest: %v", err)
	}
}
