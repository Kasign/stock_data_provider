package security

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
)

type lifecycleTestHasher struct{ expected string }

func (h lifecycleTestHasher) Hash(password []byte) (string, error) {
	return "encoded:" + string(password), nil
}
func (h lifecycleTestHasher) Verify(_ string, password []byte) (bool, error) {
	return string(password) == h.expected, nil
}
func (h lifecycleTestHasher) NeedsRehash(string) (bool, error) {
	return false, nil
}

type lifecycleTestCredentials struct {
	credential platformsecurity.OwnerCredential
	initErr    error
	updateErr  error
}

func (s *lifecycleTestCredentials) GetOwnerCredential(context.Context) (platformsecurity.OwnerCredential, error) {
	return s.credential, nil
}
func (s *lifecycleTestCredentials) InitializeOwnerCredential(_ context.Context, hash string) (platformsecurity.OwnerCredential, error) {
	if s.initErr != nil {
		return platformsecurity.OwnerCredential{}, s.initErr
	}
	s.credential = platformsecurity.OwnerCredential{Subject: "owner", PasswordHash: hash, CredentialVersion: 1}
	return s.credential, nil
}
func (s *lifecycleTestCredentials) UpdateOwnerPassword(_ context.Context, version uint64, hash string) (platformsecurity.OwnerCredential, error) {
	if s.updateErr != nil {
		return platformsecurity.OwnerCredential{}, s.updateErr
	}
	if version != s.credential.CredentialVersion {
		return platformsecurity.OwnerCredential{}, errors.New("version conflict")
	}
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
func (s *lifecycleTestSessions) Rotate(_ context.Context, oldDigest, newDigest string, record platformsecurity.SessionRecord, _ time.Time) error {
	if s.rotateErr != nil {
		return s.rotateErr
	}
	s.revoked = append(s.revoked, oldDigest)
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
	accountDigest string
	ipDigest      string
	err         error
}

func (r *lifecycleTestRateLimiter) Check(_ context.Context, accountDigest, ipDigest string) (platformsecurity.LoginRateLimitResult, error) {
	r.accountDigest, r.ipDigest = accountDigest, ipDigest
	if r.err != nil {
		return platformsecurity.LoginRateLimitResult{}, r.err
	}
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
		Now: func() time.Time {
			return now
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return service, credentials, sessions, limiter, audit
}


func TestSessionLifecycleLoginUsesContractSHA256RateLimitDigests(t *testing.T) {
	service, _, _, limiter, _ := newLifecycleTestService(t, nil)
	_, err := service.Login(context.Background(), LoginInput{
		Password: []byte("correct"), SourceIP: net.ParseIP("192.0.2.10"), RequestID: "req-digest",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	wantDigest := func(value string) string {
		digest := sha256.Sum256([]byte(value))
		return hex.EncodeToString(digest[:])
	}
	if limiter.accountDigest != wantDigest("owner") || limiter.ipDigest != wantDigest("192.0.2.10") {
		t.Fatalf("rate-limit digests = (%q, %q), want SHA-256 of normalized account and IP", limiter.accountDigest, limiter.ipDigest)
	}
	if len(limiter.accountDigest) != 64 || len(limiter.ipDigest) != 64 {
		t.Fatalf("rate-limit digest lengths = (%d, %d), want 64 lowercase hex characters", len(limiter.accountDigest), len(limiter.ipDigest))
	}
	if limiter.accountDigest != strings.ToLower(limiter.accountDigest) || limiter.ipDigest != strings.ToLower(limiter.ipDigest) {
		t.Fatal("rate-limit digests must use lowercase hexadecimal")
	}
}

func TestSessionLifecycleLoginCreatesFreshOwnerSession(t *testing.T) {
	service, _, sessions, limiter, audit := newLifecycleTestService(t, nil)
	password := []byte("correct")
	issue, err := service.Login(context.Background(), LoginInput{
		Password: password, SourceIP: net.ParseIP("192.0.2.10"), RequestID: "req-123",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if issue.Cookie == nil || !issue.Cookie.HttpOnly || !issue.Cookie.Secure || issue.CSRFToken == "" {
		t.Fatalf("invalid Session issue: %+v", issue)
	}
	if sessions.createdDigest == "" || sessions.record.CredentialVersion != 7 ||
		sessions.record.CSRFTokenDigest == "" || limiter.reset != 1 || len(audit.events) != 2 {
		t.Fatalf("Session lifecycle side effects are incomplete: record=%+v limiter=%+v audit=%+v", sessions.record, limiter, audit.events)
	}
	for _, value := range password {
		if value != 0 {
			t.Fatal("Login did not clear the supplied password buffer")
		}
	}
}

func TestSessionLifecycleLoginRevokesSessionWhenAuditFails(t *testing.T) {
	service, _, sessions, _, _ := newLifecycleTestService(t, errors.New("audit unavailable"))
	_, err := service.Login(context.Background(), LoginInput{
		Password: []byte("correct"), SourceIP: net.ParseIP("192.0.2.10"), RequestID: "req-123",
	})
	if err == nil {
		t.Fatal("Login succeeded without required audit persistence")
	}
	if sessions.createdDigest == "" || len(sessions.revoked) != 1 || sessions.revoked[0] != sessions.createdDigest {
		t.Fatalf("new Session was not revoked after audit failure: %+v", sessions)
	}
}

func TestSessionLifecycleLoginRecordsWrongPassword(t *testing.T) {
	service, _, sessions, limiter, audit := newLifecycleTestService(t, nil)
	_, err := service.Login(context.Background(), LoginInput{
		Password: []byte("wrong"), SourceIP: net.ParseIP("192.0.2.10"), RequestID: "req-123",
	})
	if err == nil {
		t.Fatal("wrong password was accepted")
	}
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
		if value != 0 {
			t.Fatal("InitializeOwner did not clear the bootstrap secret buffer")
		}
	}
}

func TestSessionLifecycleLogoutRevokesSessionAndClearsCookie(t *testing.T) {
	service, _, sessions, _, audit := newLifecycleTestService(t, nil)
	issue, err := service.Login(context.Background(), LoginInput{
		Password: []byte("correct"), SourceIP: net.ParseIP("192.0.2.10"), RequestID: "req-login",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	oldDigest := sessions.createdDigest
	cookie, err := service.Logout(context.Background(), issue.Cookie.Value, "req-logout")
	if err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if cookie == nil || cookie.Name != issue.Cookie.Name || cookie.Path != issue.Cookie.Path ||
		cookie.Value != "" || cookie.MaxAge >= 0 {
		t.Fatalf("Logout() returned an invalid clearing cookie: %+v", cookie)
	}
	if len(sessions.revoked) != 1 || sessions.revoked[0] != oldDigest {
		t.Fatalf("Logout() did not revoke the active Session: %+v", sessions.revoked)
	}
	if len(audit.events) != 4 ||
		audit.events[2].EventType != platformsecurity.AuditLogout ||
		audit.events[3].EventType != platformsecurity.AuditSessionRevoked {
		t.Fatalf("logout audit events must record action and revocation outcome: %+v", audit.events)
	}
}

func TestSessionLifecycleLogoutReturnsClearingCookieWhenAuditFails(t *testing.T) {
	service, _, sessions, _, audit := newLifecycleTestService(t, nil)
	issue, err := service.Login(context.Background(), LoginInput{
		Password: []byte("correct"), SourceIP: net.ParseIP("192.0.2.10"), RequestID: "req-login",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	audit.err = errors.New("audit unavailable")
	cookie, err := service.Logout(context.Background(), issue.Cookie.Value, "req-logout")
	if err == nil || cookie == nil || cookie.Value != "" {
		t.Fatalf("Logout() must return clearing cookie and audit error, cookie=%+v err=%v", cookie, err)
	}
	if len(sessions.revoked) != 1 {
		t.Fatalf("Session was not revoked before audit: %+v", sessions.revoked)
	}
	if len(audit.events) != 4 ||
		audit.events[2].EventType != platformsecurity.AuditLogout ||
		audit.events[3].EventType != platformsecurity.AuditSessionRevoked {
		t.Fatalf("both logout audit events must be attempted even when persistence fails: %+v", audit.events)
	}
}

func TestSessionLifecycleRotateReplacesTokenAndCSRF(t *testing.T) {
	service, _, sessions, _, audit := newLifecycleTestService(t, nil)
	issue, err := service.Login(context.Background(), LoginInput{
		Password: []byte("correct"), SourceIP: net.ParseIP("192.0.2.10"), RequestID: "req-login",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	oldDigest := sessions.createdDigest
	createdAt := sessions.record.CreatedAt
	absoluteExpiry := sessions.record.AbsoluteExpiresAt
	oldCSRF := sessions.record.CSRFTokenDigest
	rotated, err := service.RotateSession(context.Background(), issue.Cookie.Value, "req-rotate")
	if err != nil {
		t.Fatalf("RotateSession() error = %v", err)
	}
	if rotated.Cookie == nil || rotated.Cookie.Value == "" || rotated.Cookie.Value == issue.Cookie.Value ||
		rotated.CSRFToken == "" || rotated.CSRFToken == issue.CSRFToken {
		t.Fatalf("rotation did not issue fresh credentials: %+v", rotated)
	}
	if sessions.createdDigest == oldDigest || sessions.record.CSRFTokenDigest == oldCSRF ||
		!sessions.record.CreatedAt.Equal(createdAt) || !sessions.record.AbsoluteExpiresAt.Equal(absoluteExpiry) {
		t.Fatalf("rotation did not preserve Session invariants: digest=%q record=%+v", sessions.createdDigest, sessions.record)
	}
	if len(sessions.revoked) != 1 || sessions.revoked[0] != oldDigest {
		t.Fatalf("old Session was not invalidated: %+v", sessions.revoked)
	}
	if got := audit.events[len(audit.events)-1].EventType; got != platformsecurity.AuditSessionRotated {
		t.Fatalf("last audit event = %q, want session_rotated", got)
	}
}

func TestSessionLifecycleRotateRevokesNewTokenWhenAuditFails(t *testing.T) {
	service, _, sessions, _, audit := newLifecycleTestService(t, nil)
	issue, err := service.Login(context.Background(), LoginInput{
		Password: []byte("correct"), SourceIP: net.ParseIP("192.0.2.10"), RequestID: "req-login",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	oldDigest := sessions.createdDigest
	audit.err = errors.New("audit unavailable")
	_, err = service.RotateSession(context.Background(), issue.Cookie.Value, "req-rotate")
	if err == nil {
		t.Fatal("RotateSession() succeeded without required audit persistence")
	}
	if sessions.createdDigest == oldDigest || len(sessions.revoked) != 2 ||
		sessions.revoked[0] != oldDigest || sessions.revoked[1] != sessions.createdDigest {
		t.Fatalf("rotation audit failure did not fail closed: digest=%q revoked=%+v", sessions.createdDigest, sessions.revoked)
	}
}

func TestSessionLifecycleChangePasswordAdvancesCredentialVersion(t *testing.T) {
	service, credentials, _, _, audit := newLifecycleTestService(t, nil)
	currentPassword := []byte("correct")
	newPassword := []byte("new-owner-password")
	if err := service.ChangePassword(context.Background(), currentPassword, newPassword, "req-password"); err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}
	if credentials.credential.PasswordHash != "encoded:new-owner-password" ||
		credentials.credential.CredentialVersion != 8 {
		t.Fatalf("credential was not atomically advanced: %+v", credentials.credential)
	}
	if got := audit.events[len(audit.events)-1].EventType; got != platformsecurity.AuditPasswordChanged {
		t.Fatalf("last audit event = %q, want password_changed", got)
	}
	for _, buffer := range [][]byte{currentPassword, newPassword} {
		for _, value := range buffer {
			if value != 0 {
				t.Fatal("ChangePassword did not clear supplied password buffers")
			}
		}
	}
}

func TestSessionLifecycleChangePasswordAuditFailureDoesNotUndoCredentialUpdate(t *testing.T) {
	service, credentials, _, _, audit := newLifecycleTestService(t, nil)
	audit.err = errors.New("audit unavailable")
	err := service.ChangePassword(context.Background(), []byte("correct"), []byte("new-owner-password"), "req-password")
	if err == nil {
		t.Fatal("ChangePassword() must report required audit failure")
	}
	if credentials.credential.PasswordHash != "encoded:new-owner-password" ||
		credentials.credential.CredentialVersion != 8 {
		t.Fatalf("audit failure unexpectedly changed committed credential state: %+v", credentials.credential)
	}
}

func TestSessionLifecycleRotateRejectsInvalidSessionRecord(t *testing.T) {
	service, _, sessions, _, _ := newLifecycleTestService(t, nil)
	issue, err := service.Login(context.Background(), LoginInput{
		Password: []byte("correct"), SourceIP: net.ParseIP("192.0.2.10"), RequestID: "req-login",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	oldDigest := sessions.createdDigest
	sessions.record.Subject = "service"
	if _, err := service.RotateSession(context.Background(), issue.Cookie.Value, "req-rotate"); err == nil {
		t.Fatal("RotateSession() accepted a non-owner Session record")
	}
	if sessions.createdDigest != oldDigest || len(sessions.revoked) != 0 {
		t.Fatalf("invalid Session record was rotated: digest=%q revoked=%+v", sessions.createdDigest, sessions.revoked)
	}
}

func TestSessionLifecycleRotateRejectsStaleCredentialVersion(t *testing.T) {
	service, credentials, sessions, _, _ := newLifecycleTestService(t, nil)
	issue, err := service.Login(context.Background(), LoginInput{
		Password: []byte("correct"), SourceIP: net.ParseIP("192.0.2.10"), RequestID: "req-login",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	oldDigest := sessions.createdDigest
	credentials.credential.CredentialVersion++
	if _, err := service.RotateSession(context.Background(), issue.Cookie.Value, "req-rotate"); err == nil {
		t.Fatal("RotateSession() accepted a stale credential version")
	}
	if sessions.createdDigest != oldDigest || len(sessions.revoked) != 0 {
		t.Fatalf("stale Session was rotated: digest=%q revoked=%+v", sessions.createdDigest, sessions.revoked)
	}
}

func TestSessionLifecycleChangePasswordRejectsInvalidCredential(t *testing.T) {
	service, credentials, _, _, _ := newLifecycleTestService(t, nil)
	credentials.credential.Subject = "service"
	if err := service.ChangePassword(context.Background(), []byte("correct"), []byte("new-owner-password"), "req-password"); err == nil {
		t.Fatal("ChangePassword() accepted an invalid Owner credential")
	}
	if credentials.credential.CredentialVersion != 7 || credentials.credential.PasswordHash != "encoded:correct" {
		t.Fatalf("invalid credential was modified: %+v", credentials.credential)
	}
}

func TestSessionLifecycleChangePasswordRejectsWrongCurrentPassword(t *testing.T) {
	service, credentials, _, _, audit := newLifecycleTestService(t, nil)
	if err := service.ChangePassword(context.Background(), []byte("wrong"), []byte("new-owner-password"), "req-password"); err == nil {
		t.Fatal("ChangePassword() accepted an incorrect current password")
	}
	if credentials.credential.PasswordHash != "encoded:correct" || credentials.credential.CredentialVersion != 7 {
		t.Fatalf("wrong current password changed credential: %+v", credentials.credential)
	}
	if len(audit.events) != 1 || audit.events[0].Result != platformsecurity.AuditResultFailure {
		t.Fatalf("wrong current password was not audited: %+v", audit.events)
	}
}
