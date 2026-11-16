package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	serversecurity "example.com/stock-analysis-flow/internal/server/security"
)

type ownerHTTPTestCredentials struct {
	credential platformsecurity.OwnerCredential
}

func (s *ownerHTTPTestCredentials) GetOwnerCredential(context.Context) (platformsecurity.OwnerCredential, error) {
	return s.credential, nil
}
func (s *ownerHTTPTestCredentials) InitializeOwnerCredential(context.Context, string) (platformsecurity.OwnerCredential, error) {
	return platformsecurity.OwnerCredential{}, errors.New("not implemented")
}
func (s *ownerHTTPTestCredentials) UpdateOwnerPassword(_ context.Context, version uint64, hash string) (platformsecurity.OwnerCredential, error) {
	if version != s.credential.CredentialVersion {
		return platformsecurity.OwnerCredential{}, errors.New("credential version conflict")
	}
	s.credential.PasswordHash = hash
	s.credential.CredentialVersion++
	return s.credential, nil
}

type ownerHTTPTestSessions struct {
	records     map[string]platformsecurity.SessionRecord
	touches     int
	csrfTouches int
}

func (s *ownerHTTPTestSessions) Create(_ context.Context, digest string, record platformsecurity.SessionRecord) error {
	if s.records == nil {
		s.records = make(map[string]platformsecurity.SessionRecord)
	}
	if _, exists := s.records[digest]; exists {
		return platformsecurity.ErrSessionConflict
	}
	s.records[digest] = record
	return nil
}
func (s *ownerHTTPTestSessions) LookupAndTouch(_ context.Context, digest string, now time.Time, idle time.Duration) (platformsecurity.SessionRecord, error) {
	s.touches++
	record, ok := s.records[digest]
	if !ok {
		return platformsecurity.SessionRecord{}, platformsecurity.ErrSessionNotFound
	}
	record.LastSeenAt = now
	record.IdleExpiresAt = minOwnerHTTPTime(now.Add(idle), record.AbsoluteExpiresAt)
	s.records[digest] = record
	return record, nil
}
func (s *ownerHTTPTestSessions) LookupAndTouchWithCSRF(_ context.Context, digest, csrfDigest string, now time.Time, idle time.Duration) (platformsecurity.SessionRecord, error) {
	s.csrfTouches++
	record, ok := s.records[digest]
	if !ok {
		return platformsecurity.SessionRecord{}, platformsecurity.ErrSessionNotFound
	}
	if record.CSRFTokenDigest != csrfDigest {
		return platformsecurity.SessionRecord{}, platformsecurity.ErrSessionCSRFRejected
	}
	record.LastSeenAt = now
	record.IdleExpiresAt = minOwnerHTTPTime(now.Add(idle), record.AbsoluteExpiresAt)
	s.records[digest] = record
	return record, nil
}
func (s *ownerHTTPTestSessions) Rotate(_ context.Context, oldDigest, newDigest string, record platformsecurity.SessionRecord, _ time.Time) error {
	if _, ok := s.records[oldDigest]; !ok {
		return platformsecurity.ErrSessionNotFound
	}
	delete(s.records, oldDigest)
	s.records[newDigest] = record
	return nil
}
func (s *ownerHTTPTestSessions) RotateCSRFToken(_ context.Context, digest, newCSRFDigest string, version uint64, now time.Time, idle time.Duration) (platformsecurity.SessionRecord, error) {
	record, ok := s.records[digest]
	if !ok || record.CredentialVersion != version {
		return platformsecurity.SessionRecord{}, platformsecurity.ErrSessionNotFound
	}
	record.CSRFTokenDigest = newCSRFDigest
	record.LastSeenAt = now
	record.IdleExpiresAt = minOwnerHTTPTime(now.Add(idle), record.AbsoluteExpiresAt)
	s.records[digest] = record
	return record, nil
}
func (s *ownerHTTPTestSessions) Revoke(_ context.Context, digest string) error {
	delete(s.records, digest)
	return nil
}

type ownerHTTPTestRateLimiter struct{}

func (ownerHTTPTestRateLimiter) Check(context.Context, string, string) (platformsecurity.LoginRateLimitResult, error) {
	return platformsecurity.LoginRateLimitResult{Allowed: true}, nil
}
func (ownerHTTPTestRateLimiter) RecordFailure(context.Context, string, string, platformsecurity.LoginRateLimitPolicy) (platformsecurity.LoginRateLimitResult, error) {
	return platformsecurity.LoginRateLimitResult{Allowed: true}, nil
}
func (ownerHTTPTestRateLimiter) ResetAccount(context.Context, string) error { return nil }

type ownerHTTPTestHasher struct{}

func (ownerHTTPTestHasher) Hash(password []byte) (string, error) {
	return "hash:" + string(password), nil
}
func (ownerHTTPTestHasher) Verify(encoded string, password []byte) (bool, error) {
	return encoded == "hash:"+string(password), nil
}

type ownerHTTPTestAudit struct{}

func (ownerHTTPTestAudit) Write(context.Context, platformsecurity.SecurityAuditEvent) error {
	return nil
}

func minOwnerHTTPTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

func newOwnerHTTPTestHandler(t *testing.T, sessions *ownerHTTPTestSessions, credentials *ownerHTTPTestCredentials) http.Handler {
	t.Helper()
	cookie := platformsecurity.SessionCookieConfig{
		Name: "__Secure-owner_session", Path: "/api", Secure: true, SameSite: http.SameSiteLaxMode,
	}
	policy := platformsecurity.LoginRateLimitPolicy{
		AccountMaxFailures: 5, IPMaxFailures: 10, Window: time.Minute,
		BaseBackoff: time.Second, MaxBackoff: time.Minute,
	}
	lifecycle, err := serversecurity.NewSessionLifecycleService(serversecurity.SessionLifecycleDependencies{
		Credentials: credentials, Sessions: sessions, RateLimiter: ownerHTTPTestRateLimiter{},
		Hasher: ownerHTTPTestHasher{}, Audit: ownerHTTPTestAudit{},
	}, serversecurity.SessionLifecycleConfig{
		Cookie: cookie, IdleTTL: time.Hour, AbsoluteTTL: 24 * time.Hour,
		RateLimitPolicy: policy,
	})
	if err != nil {
		t.Fatal(err)
	}
	authenticator, err := serversecurity.NewSessionAuthenticator(sessions, credentials, serversecurity.SessionAuthenticatorConfig{
		Cookie: cookie, PublicOrigin: "https://stocks.example.com", CSRFHeader: "X-CSRF-Token", IdleTTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	composition := &ownerSessionComposition{lifecycle: lifecycle, authenticator: authenticator, cookie: cookie}
	ownerToken := strings.Repeat("a", 43)
	env := platformconfigForOwnerHTTP(ownerToken)
	bearer, err := platformsecurity.NewAuthenticator(env, nil)
	if err != nil {
		t.Fatal(err)
	}
	legacy := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	health := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	handler, err := newHTTPHandler(bearer, health, legacy, composition)
	if err != nil {
		t.Fatal(err)
	}
	handler, err = newHTTPMiddleware(handler, httpMiddlewareConfig{maxRequestBodyBytes: 64 * 1024, maxConcurrentRequests: 8})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func platformconfigForOwnerHTTP(token string) platformconfig.Secret {
	return platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		if key == "PLATFORM_ADMIN_TOKEN" {
			return token, true
		}
		return "", false
	}).Secret("PLATFORM_ADMIN_TOKEN")
}

func TestOwnerSessionHTTPLoginRefreshCSRFAndLogout(t *testing.T) {
	credentials := &ownerHTTPTestCredentials{credential: platformsecurity.OwnerCredential{
		Subject: "owner", PasswordHash: "hash:correct", CredentialVersion: 1,
	}}
	sessions := &ownerHTTPTestSessions{records: make(map[string]platformsecurity.SessionRecord)}
	handler := newOwnerHTTPTestHandler(t, sessions, credentials)

	login := httptest.NewRequest(http.MethodPost, "https://stocks.example.com/api/v1/owner/session", strings.NewReader(`{"password":"correct"}`))
	login.Header.Set("Content-Type", "application/json; charset=utf-8")
	login.Header.Set("Origin", "https://stocks.example.com")
	login.RemoteAddr = "192.0.2.10:4321"
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginResponse.Code, loginResponse.Body.String())
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Value == "" || !cookies[0].HttpOnly || !cookies[0].Secure {
		t.Fatalf("login did not issue a secure HttpOnly cookie: %+v", cookies)
	}
	csrfToken := struct {
		CSRFToken string `json:"csrf_token"`
	}{}
	if err := json.Unmarshal(loginResponse.Body.Bytes(), &csrfToken); err != nil || csrfToken.CSRFToken == "" {
		t.Fatalf("login did not return CSRF token: %v body=%s", err, loginResponse.Body.String())
	}

	current := httptest.NewRequest(http.MethodGet, "https://stocks.example.com/api/v1/owner/session", nil)
	current.AddCookie(cookies[0])
	currentResponse := httptest.NewRecorder()
	handler.ServeHTTP(currentResponse, current)
	if currentResponse.Code != http.StatusOK {
		t.Fatalf("current session status=%d body=%s", currentResponse.Code, currentResponse.Body.String())
	}

	refresh := httptest.NewRequest(http.MethodPost, "https://stocks.example.com/api/v1/owner/session/csrf", nil)
	refresh.AddCookie(cookies[0])
	refresh.Header.Set("Origin", "https://stocks.example.com")
	refreshResponse := httptest.NewRecorder()
	handler.ServeHTTP(refreshResponse, refresh)
	if refreshResponse.Code != http.StatusOK {
		t.Fatalf("CSRF refresh status=%d body=%s", refreshResponse.Code, refreshResponse.Body.String())
	}
	var refreshed struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(refreshResponse.Body.Bytes(), &refreshed); err != nil || refreshed.CSRFToken == "" || refreshed.CSRFToken == csrfToken.CSRFToken {
		t.Fatalf("CSRF refresh did not rotate token: %v body=%s", err, refreshResponse.Body.String())
	}

	digest, err := platformsecurity.SessionTokenDigest(cookies[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	recordBefore := sessions.records[digest]
	badWrite := httptest.NewRequest(http.MethodDelete, "https://stocks.example.com/api/v1/owner/session", nil)
	badWrite.AddCookie(cookies[0])
	badWrite.Header.Set("Origin", "https://stocks.example.com")
	badWrite.Header.Set("X-CSRF-Token", csrfToken.CSRFToken)
	badWriteResponse := httptest.NewRecorder()
	handler.ServeHTTP(badWriteResponse, badWrite)
	if badWriteResponse.Code != http.StatusForbidden {
		t.Fatalf("old CSRF status=%d body=%s", badWriteResponse.Code, badWriteResponse.Body.String())
	}
	if sessions.records[digest].LastSeenAt != recordBefore.LastSeenAt {
		t.Fatal("rejected CSRF changed session activity")
	}

	logout := httptest.NewRequest(http.MethodDelete, "https://stocks.example.com/api/v1/owner/session", nil)
	logout.AddCookie(cookies[0])
	logout.Header.Set("Origin", "https://stocks.example.com")
	logout.Header.Set("X-CSRF-Token", refreshed.CSRFToken)
	logoutResponse := httptest.NewRecorder()
	handler.ServeHTTP(logoutResponse, logout)
	if logoutResponse.Code != http.StatusNoContent {
		t.Fatalf("logout status=%d body=%s", logoutResponse.Code, logoutResponse.Body.String())
	}
	if len(logoutResponse.Result().Cookies()) != 1 || logoutResponse.Result().Cookies()[0].MaxAge >= 0 {
		t.Fatal("logout did not clear the Session Cookie")
	}
	if _, ok := sessions.records[digest]; ok {
		t.Fatal("logout did not revoke the Session")
	}
}

func TestOwnerSessionHTTPRejectsLoginOriginMismatch(t *testing.T) {
	credentials := &ownerHTTPTestCredentials{credential: platformsecurity.OwnerCredential{
		Subject: "owner", PasswordHash: "hash:correct", CredentialVersion: 1,
	}}
	handler := newOwnerHTTPTestHandler(t, &ownerHTTPTestSessions{records: make(map[string]platformsecurity.SessionRecord)}, credentials)
	request := httptest.NewRequest(http.MethodPost, "https://stocks.example.com/api/v1/owner/session", strings.NewReader(`{"password":"correct"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://attacker.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin login status=%d body=%s", response.Code, response.Body.String())
	}
}
