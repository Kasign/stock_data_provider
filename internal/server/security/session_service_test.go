package security

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
)

type lifecycleTestHasher struct{ expected string }

func (h lifecycleTestHasher) Hash(password []byte) (string, error) { return "encoded:" + string(password), nil }
func (h lifecycleTestHasher) Verify(_ string, password []byte) (bool, error) {
	return string(password) == h.expected, nil
}
func (h lifecycleTestHasher) NeedsRehash(string) (bool, error) { return false, nil }

type lifecycleTestCredentials struct {
	credential platformsecurity.OwnerCredential
	initErr    error
	updateErr  error
}

func (s *lifecycleTestCredentials) GetOwnerCredential(context.Context) (platformsecurity.OwnerCredential, error) {
	return s.credential, nil
}
func (s *lifecycleTestCredentials) InitializeOwnerCredential(_ context.Context, hash string) (platformsecurity.OwnerCredential, error) {
	if s.initErr != nil { return platformsecurity.OwnerCredential{}, s.initErr }
	s.credential = platformsecurity.OwnerCredential{Subject: "owner", PasswordHash: hash, CredentialVersion: 1}
	return s.credential, nil
}
func (s *lifecycleTestCredentials) UpdateOwnerPassword(_ context.Context, version uint64, hash string) (platformsecurity.OwnerCredential, error) {
	if s.updateErr != nil { return platformsecurity.OwnerCredential{}, s.updateErr }
	if version != s.credential.CredentialVersion { return platformsecurity.OwnerCredential{}, errors.New("version conflict") }
	s.credential.PasswordHash = hash
	s.credential.CredentialVersion++
	return s.credential, nil
}

type lifecycleTestSessions struct {
	createdDigest string
	record        platformsecurity.SessionRecord
	revoked       []string
	rotateErr     error
}

func (s *lifecycleTestSessions) Create(_ context.Context, digest string, record platformsecurity.SessionRecord) error {
	s.createdDigest, s.record = digest, record
	return nil
}
func (s *lifecycleTestSessions) LookupAndTouch(_ context.Context, _ string, now time.Time, idle time.Duration) (platformsecurity.SessionRecord, error) {
	record := s.record
	record.LastSeenAt = now
	record.IdleExpiresAt = minTime(now.Add(idle), record.AbsoluteExpiresAt)
	return record, nil
}
func (s *lifecycleTestSessions) Rotate(_ context.Context, _, newDigest string, record platformsecurity.SessionRecord, _ time.Time) error {
	if s.rotateErr != nil { return s.rotateErr }
	s.createdDigest, s.record = newDigest, record
	return nil
}
func (s *lifecycleTestSessions) Revoke(_ context.Context, digest string) error {
	s.revoked = append(s.revoked, digest)
	return nil
}

type lifecycleTestRateLimiter struct {
	checkResult platformsecurity.LoginRateLimitResult
	failures    int
	reset       int
	err         error
}

func (r *lifecycleTestRateLimiter) Check(context.Context, string, string) (platformsecurity.LoginRateLimitResult, error) {
	if r.err != nil { return platformsecurity.LoginRateLimitResult{}, r.err }
	return r.checkResult, nil
}
func (r *lifecycleTestRateLimiter) RecordFailure(context.Context, string, string, platformsecurity.LoginRateLimitPolicy) (platformsecurity.LoginRateLimitResult, error) {
	r.failures++
	return platformsecurity.LoginRateLimitResult{Allowed: true}, r.err
}
func (r *lifecycleTestRateLimiter) ResetAccount(context.Context, string) error {
	r.reset++
	return r.err
}

type lifecycleTestAudit struct {
	events []platformsecurity.SecurityAuditEvent
	err    error
}

func (a *lifecycleTestAudit) Write(_ context.Context, event platformsecurity.SecurityAuditEvent) error {
	a.events = append(a.events, event)
	return a.err
}

func newLifecycleTestService(t *testing.T, auditErr error) (*SessionLifecycleService, *lifecycleTestCredentials, *lifecycleTestSessions, *lifecycleTestRateLimiter, *lifecycleTestAudit) {
	t.Helper()
	credentials := &lifecycleTestCredentials{credential: platformsecurity.OwnerCredential{
		Subject: "owner", PasswordHash: "encoded:correct", CredentialVersion: 7,
	}}
	sessions := &lifecycleTestSessions{}
	limiter := &lifecycleTestRateLimiter{checkResult: platformsecurity.LoginRateLimitResult{Allowed: true}}
	audit := &lifecycleTestAudit{err: auditErr}
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	service, err := NewSessionLifecycleService(SessionLifecycleDependencies{
		Credentials: credentials,
		Sessions:    sessions,
		RateLimiter: limiter,
		Hasher:      lifecycleTestHasher{expected: "correct"},
		Audit:       audit,
	}, SessionLifecycleConfig{
		RateLimitHMACKey: []byte("0123456789abcdef0123456789abcdef"),
		Cookie: platformsecurity.SessionCookieConfig{
			Name:     "__Secure-owner_session",
			Path:     "/api",
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		},
		IdleTTL:     12 * time.Hour,
		AbsoluteTTL: 7 * 24 * time.Hour,
		RateLimitPolicy: platformsecurity.LoginRateLimitPolicy{
			AccountMaxFailures: 5,
			IPMaxFailures:      10,
			Window:             15 * time.Minute,
			BaseBackoff:        time.Second,
			MaxBackoff:         15 * time.Minute,
		},
		Now: func() time.Time { return now },
	})
	if err != nil { t.Fatal(err) }
	return service, credentials, sessions, limiter, audit
}

func TestSessionLifecycleLoginCreatesFreshOwnerSession(t *testing.T) {
	service, _, sessions, limiter, audit := newLifecycleTestService(t, nil)
	password := []byte("correct")
	issue, err := service.Login(context.Background(), LoginInput{
		Password: password, SourceIP: net.ParseIP("192.0.2.10"), RequestID: "req-123",
	})
	if err != nil { t.Fatalf("Login() error = %v", err) }
	if issue.Cookie == nil || !issue.Cookie.HttpOnly || !issue.Cookie.Secure || issue.CSRFToken == "" {
		t.Fatalf("invalid Session issue: %+v", issue)
	}
	if sessions.createdDigest == "" || sessions.record.CredentialVersion != 7 ||
		sessions.record.CSRFTokenDigest == "" || limiter.reset != 1 || len(audit.events) != 2 {
		t.Fatalf("Session lifecycle side effects are incomplete: record=%+v limiter=%+v audit=%+v", sessions.record, limiter, audit.events)
	}
	for _, value := range password {
		if value != 0 { t.Fatal("Login did not clear the supplied password buffer") }
	}
}

func TestSessionLifecycleLoginRevokesSessionWhenAuditFails(t *testing.T) {
	service, _, sessions, _, _ := newLifecycleTestService(t, errors.New("audit unavailable"))
	_, err := service.Login(context.Background(), LoginInput{
		Password: []byte("correct"), SourceIP: net.ParseIP("192.0.2.10"), RequestID: "req-123",
	})
	if err == nil { t.Fatal("Login succeeded without required audit persistence") }
	if sessions.createdDigest == "" || len(sessions.revoked) != 1 || sessions.revoked[0] != sessions.createdDigest {
		t.Fatalf("new Session was not revoked after audit failure: %+v", sessions)
	}
}

func TestSessionLifecycleLoginRecordsWrongPassword(t *testing.T) {
	service, _, sessions, limiter, audit := newLifecycleTestService(t, nil)
	_, err := service.Login(context.Background(), LoginInput{
		Password: []byte("wrong"), SourceIP: net.ParseIP("192.0.2.10"), RequestID: "req-123",
	})
	if err == nil { t.Fatal("wrong password was accepted") }
	if limiter.failures != 1 || sessions.createdDigest != "" || len(audit.events) != 1 ||
		audit.events[0].EventType != platformsecurity.AuditLoginFailed {
		t.Fatalf("wrong-password handling is incorrect: limiter=%+v sessions=%+v audit=%+v", limiter, sessions, audit.events)
	}
}

func TestSessionLifecycleInitializeOwnerClearsBootstrapSecret(t *testing.T) {
	service, credentials, _, _, audit := newLifecycleTestService(t, nil)
	secret := []byte("first-owner-secret")
	if err := service.InitializeOwner(context.Background(), secret, "req-init"); err != nil {
		t.Fatalf("InitializeOwner() error = %v", err)
	}
	if credentials.credential.PasswordHash != "encoded:first-owner-secret" ||
		credentials.credential.CredentialVersion != 1 || len(audit.events) != 1 {
		t.Fatalf("Owner credential was not initialized: %+v", credentials.credential)
	}
	for _, value := range secret {
		if value != 0 { t.Fatal("InitializeOwner did not clear the bootstrap secret buffer") }
	}
}
