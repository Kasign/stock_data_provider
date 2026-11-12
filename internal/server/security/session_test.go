package security

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
)

type testSessionStore struct {
	record  platformsecurity.SessionRecord
	err     error
	digest  string
	idleTTL time.Duration
}

func (s *testSessionStore) Create(context.Context, string, platformsecurity.SessionRecord) error {
	return nil
}

func (s *testSessionStore) LookupAndTouch(_ context.Context, digest string, now time.Time, idleTTL time.Duration) (platformsecurity.SessionRecord, error) {
	s.digest, s.idleTTL = digest, idleTTL
	if s.err != nil {
		return platformsecurity.SessionRecord{}, s.err
	}
	record := s.record
	record.LastSeenAt = now
	record.IdleExpiresAt = now.Add(idleTTL)
	if record.AbsoluteExpiresAt.Before(record.IdleExpiresAt) {
		record.IdleExpiresAt = record.AbsoluteExpiresAt
	}
	return record, nil
}

func (s *testSessionStore) Rotate(context.Context, string, string, platformsecurity.SessionRecord, time.Time) error {
	return nil
}

func (s *testSessionStore) Revoke(context.Context, string) error {
	return nil
}

type testCredentialStore struct {
	credential platformsecurity.OwnerCredential
	err        error
}

func (s *testCredentialStore) GetOwnerCredential(context.Context) (platformsecurity.OwnerCredential, error) {
	return s.credential, s.err
}

func (s *testCredentialStore) InitializeOwnerCredential(context.Context, string) (platformsecurity.OwnerCredential, error) {
	return platformsecurity.OwnerCredential{}, errors.New("not implemented")
}

func (s *testCredentialStore) UpdateOwnerPassword(context.Context, uint64, string) (platformsecurity.OwnerCredential, error) {
	return platformsecurity.OwnerCredential{}, errors.New("not implemented")
}

func newTestSessionAuthenticator(t *testing.T, now time.Time) (*SessionAuthenticator, *testSessionStore, string, string) {
	t.Helper()
	rawToken, _, err := platformsecurity.NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	csrfToken, csrfDigest, err := platformsecurity.NewCSRFToken()
	if err != nil {
		t.Fatal(err)
	}
	sessions := &testSessionStore{record: platformsecurity.SessionRecord{
		Subject: "owner", CreatedAt: now.Add(-time.Hour), LastSeenAt: now.Add(-time.Minute),
		IdleExpiresAt:     now.Add(platformsecurity.DefaultSessionIdleTTL),
		AbsoluteExpiresAt: now.Add(platformsecurity.DefaultSessionAbsoluteTTL),
		CredentialVersion: 3, CSRFTokenDigest: csrfDigest,
	}}
	credentials := &testCredentialStore{credential: platformsecurity.OwnerCredential{
		Subject: "owner", CredentialVersion: 3,
	}}
	auth, err := NewSessionAuthenticator(sessions, credentials, SessionAuthenticatorConfig{
		Cookie: platformsecurity.SessionCookieConfig{
			Name: "__Secure-owner_session", Path: "/api", Secure: true, SameSite: http.SameSiteLaxMode,
		},
		PublicOrigin: "https://stocks.example.com", CSRFHeader: DefaultCSRFAuthHeader,
		IdleTTL: 12 * time.Hour, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	return auth, sessions, rawToken, csrfToken
}

func sessionRequest(method, rawToken string) *http.Request {
	request := httptest.NewRequest(method, "https://stocks.example.com/api/private", nil)
	request.Host = "stocks.example.com"
	if rawToken != "" {
		request.AddCookie(&http.Cookie{Name: "__Secure-owner_session", Value: rawToken})
	}
	return request
}

func TestSessionAuthenticatorAddsOwnerSessionPrincipalAndRefreshCookie(t *testing.T) {
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	auth, sessions, rawToken, _ := newTestSessionAuthenticator(t, now)
	request := sessionRequest(http.MethodGet, rawToken)
	authenticated, cookie, err := auth.Authenticate(request)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	principal, ok := platformsecurity.PrincipalFromContext(authenticated)
	if !ok || principal.Kind != platformsecurity.IdentityOwner ||
		principal.AuthMethod != platformsecurity.AuthMethodOwnerSession ||
		principal.Subject != "owner" || len(principal.Scopes) != 0 {
		t.Fatalf("unexpected Session Principal: %+v, present=%v", principal, ok)
	}
	if cookie == nil || cookie.Value != rawToken || !cookie.HttpOnly || !cookie.Secure ||
		cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/api" {
		t.Fatalf("unexpected refreshed Session Cookie: %+v", cookie)
	}
	if sessions.idleTTL != 12*time.Hour || sessions.digest == "" {
		t.Fatalf("SessionStore lookup arguments were not passed: %+v", sessions)
	}
}

func TestSessionAuthenticatorRequiresCookieAndRejectsDuplicateCookies(t *testing.T) {
	now := time.Now().UTC()
	auth, _, rawToken, _ := newTestSessionAuthenticator(t, now)
	if _, _, err := auth.Authenticate(sessionRequest(http.MethodGet, "")); err == nil {
		t.Fatal("missing Session Cookie was accepted")
	}
	request := sessionRequest(http.MethodGet, rawToken)
	request.AddCookie(&http.Cookie{Name: "__Secure-owner_session", Value: rawToken})
	if _, _, err := auth.Authenticate(request); err == nil {
		t.Fatal("duplicate Session Cookies were accepted")
	}
}

func TestSessionAuthenticatorRequiresCSRFAndSameOriginForWrites(t *testing.T) {
	now := time.Now().UTC()
	auth, _, rawToken, csrfToken := newTestSessionAuthenticator(t, now)
	request := sessionRequest(http.MethodPost, rawToken)
	request.Header.Set("Origin", "https://stocks.example.com")
	if _, _, err := auth.Authenticate(request); err == nil {
		t.Fatal("unsafe request without CSRF token was accepted")
	}
	request.Header.Set(DefaultCSRFAuthHeader, csrfToken)
	request.Header.Add(DefaultCSRFAuthHeader, csrfToken)
	if _, _, err := auth.Authenticate(request); err == nil {
		t.Fatal("duplicate CSRF headers were accepted")
	}
	request.Header.Del(DefaultCSRFAuthHeader)
	request.Header.Set(DefaultCSRFAuthHeader, csrfToken)
	request.Header.Set("Origin", "https://attacker.example")
	if _, _, err := auth.Authenticate(request); err == nil {
		t.Fatal("cross-origin write was accepted")
	}
	request.Header.Set("Origin", "https://stocks.example.com")
	if _, _, err := auth.Authenticate(request); err != nil {
		t.Fatalf("same-origin CSRF-protected request failed: %v", err)
	}
}

func TestSessionAuthenticatorRejectsHostMismatch(t *testing.T) {
	now := time.Now().UTC()
	auth, _, rawToken, _ := newTestSessionAuthenticator(t, now)
	request := sessionRequest(http.MethodGet, rawToken)
	request.Host = "attacker.example"
	if _, _, err := auth.Authenticate(request); err == nil {
		t.Fatal("untrusted Host was accepted")
	}
}

func TestSessionAuthenticatorRejectsCredentialVersionMismatch(t *testing.T) {
	now := time.Now().UTC()
	auth, _, rawToken, _ := newTestSessionAuthenticator(t, now)
	auth.credentials = &testCredentialStore{credential: platformsecurity.OwnerCredential{
		Subject: "owner", CredentialVersion: 4,
	}}
	if _, _, err := auth.Authenticate(sessionRequest(http.MethodGet, rawToken)); err == nil {
		t.Fatal("stale Session credential version was accepted")
	}
}

func TestSessionAuthenticatorFailsClosedWhenSessionStoreUnavailable(t *testing.T) {
	now := time.Now().UTC()
	auth, sessions, rawToken, _ := newTestSessionAuthenticator(t, now)
	sessions.err = errors.New("redis unavailable")
	if _, _, err := auth.Authenticate(sessionRequest(http.MethodGet, rawToken)); err == nil ||
		!strings.Contains(err.Error(), "redis unavailable") {
		t.Fatalf("SessionStore failure = %v, want propagated failure", err)
	}
}
