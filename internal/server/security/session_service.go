package security

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	platformfault "example.com/stock-analysis-flow/internal/platform/fault"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
)

type PasswordHasher interface {
	Hash(password []byte) (string, error)
	Verify(encoded string, password []byte) (bool, error)
}

type SessionLifecycleConfig struct {
	RateLimitHMACKey []byte
	Cookie          platformsecurity.SessionCookieConfig
	IdleTTL         time.Duration
	AbsoluteTTL     time.Duration
	RateLimitPolicy platformsecurity.LoginRateLimitPolicy
	Now             func() time.Time
}

type SessionLifecycleDependencies struct {
	Credentials platformsecurity.CredentialStore
	Sessions    platformsecurity.SessionStore
	RateLimiter platformsecurity.LoginRateLimiter
	Hasher      PasswordHasher
	Audit       platformsecurity.SecurityAuditWriter
}

type SessionLifecycleService struct {
	credentials platformsecurity.CredentialStore
	sessions    platformsecurity.SessionStore
	rateLimiter platformsecurity.LoginRateLimiter
	hasher      PasswordHasher
	audit       platformsecurity.SecurityAuditWriter
	cookie      platformsecurity.SessionCookieConfig
	idleTTL     time.Duration
	absoluteTTL time.Duration
	policy      platformsecurity.LoginRateLimitPolicy
	rateKey     []byte
	now         func() time.Time
}

type LoginInput struct {
	Password  []byte
	SourceIP  net.IP
	RequestID string
}

type SessionIssue struct {
	Cookie    *http.Cookie
	CSRFToken string
}

func NewSessionLifecycleService(deps SessionLifecycleDependencies, cfg SessionLifecycleConfig) (*SessionLifecycleService, error) {
	if deps.Credentials == nil || deps.Sessions == nil || deps.RateLimiter == nil || deps.Hasher == nil || deps.Audit == nil {
		return nil, errors.New("Owner Session lifecycle dependencies are incomplete")
	}
	if err := cfg.Cookie.Validate(); err != nil {
		return nil, fmt.Errorf("Session Cookie 配置无效: %w", err)
	}
	if len(cfg.RateLimitHMACKey) < 32 {
		return nil, errors.New("login rate limit HMAC key must contain at least 32 bytes")
	}
	if cfg.IdleTTL < time.Second || cfg.IdleTTL > platformsecurity.DefaultSessionIdleTTL {
		return nil, errors.New("Session idle TTL is outside Platform bounds")
	}
	if cfg.AbsoluteTTL < cfg.IdleTTL || cfg.AbsoluteTTL > platformsecurity.DefaultSessionAbsoluteTTL {
		return nil, errors.New("Session absolute TTL is outside Platform bounds")
	}
	if cfg.RateLimitPolicy.AccountMaxFailures < 1 || cfg.RateLimitPolicy.AccountMaxFailures > 100000 ||
		cfg.RateLimitPolicy.IPMaxFailures < 1 || cfg.RateLimitPolicy.IPMaxFailures > 100000 ||
		cfg.RateLimitPolicy.Window < time.Millisecond || cfg.RateLimitPolicy.Window > 24*time.Hour ||
		cfg.RateLimitPolicy.BaseBackoff < time.Millisecond || cfg.RateLimitPolicy.BaseBackoff > 24*time.Hour ||
		cfg.RateLimitPolicy.MaxBackoff < cfg.RateLimitPolicy.BaseBackoff || cfg.RateLimitPolicy.MaxBackoff > 24*time.Hour {
		return nil, errors.New("login rate limit policy is incomplete")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &SessionLifecycleService{
		credentials: deps.Credentials,
		sessions:    deps.Sessions,
		rateLimiter: deps.RateLimiter,
		hasher:      deps.Hasher,
		audit:       deps.Audit,
		cookie:      cfg.Cookie,
		idleTTL:     cfg.IdleTTL,
		absoluteTTL: cfg.AbsoluteTTL,
		policy:      cfg.RateLimitPolicy,
		rateKey:     append([]byte(nil), cfg.RateLimitHMACKey...),
		now:         now,
	}, nil
}

// InitializeOwner consumes the one-time bootstrap secret. The caller must remove
// the secret from its source after this call; this method also clears the supplied bytes.
func (s *SessionLifecycleService) InitializeOwner(ctx context.Context, initialSecret []byte, requestID string) error {
	defer clear(initialSecret)
	if err := s.validateOperation(ctx, requestID); err != nil {
		return err
	}
	if len(initialSecret) == 0 {
		return platformfault.New(platformfault.ClassValidation, "server.invalid_initial_owner_secret", "initial Owner secret is invalid", nil)
	}
	hash, err := s.hasher.Hash(initialSecret)
	if err != nil {
		return platformfault.New(platformfault.ClassValidation, "server.owner_password_hash_failed", "Owner credential could not be initialized", err)
	}
	credential, err := s.credentials.InitializeOwnerCredential(ctx, hash)
	if err != nil {
		return err
	}
	if credential.Subject != "owner" || credential.CredentialVersion == 0 {
		return platformfault.New(platformfault.ClassInternal, "server.invalid_initialized_owner_credential", "Owner credential initialization returned invalid state", nil)
	}
	return s.writeAudit(ctx, requestID, platformsecurity.AuditSensitiveAction,
		"owner", platformsecurity.AuditResultSuccess, platformsecurity.AuditSourceInternal,
		"owner_credential_initialized")
}

// Login verifies the fixed single Owner account and creates a fresh independent Session.
// SourceIP must already be resolved by the trusted ingress policy; this service never reads X-Forwarded-For.
func (s *SessionLifecycleService) Login(ctx context.Context, input LoginInput) (SessionIssue, error) {
	defer clear(input.Password)
	if err := s.validateOperation(ctx, input.RequestID); err != nil {
		return SessionIssue{}, err
	}
	if len(input.Password) == 0 || len(input.Password) > 1024 ||
		(input.SourceIP.To4() == nil && input.SourceIP.To16() == nil) {
		return SessionIssue{}, platformfault.New(platformfault.ClassValidation, "server.invalid_login_input", "login input is invalid", nil)
	}
	accountDigest := securityDigest(s.rateKey, "account:owner")
	ipDigest := securityDigest(s.rateKey, "source-ip:" + canonicalIP(input.SourceIP))
	limit, err := s.rateLimiter.Check(ctx, accountDigest, ipDigest)
	if err != nil {
		return SessionIssue{}, err
	}
	if !limit.Allowed {
		if auditErr := s.writeAudit(ctx, input.RequestID, platformsecurity.AuditLoginRateLimited,
			"anonymous", platformsecurity.AuditResultBlocked, platformsecurity.AuditSourceAnonymous, "login_rate_limited"); auditErr != nil {
			return SessionIssue{}, auditErr
		}
		return SessionIssue{}, unauthenticatedLoginFault()
	}
	credential, err := s.credentials.GetOwnerCredential(ctx)
	if err != nil {
		if isCredentialNotFound(err) {
			if _, limitErr := s.rateLimiter.RecordFailure(ctx, accountDigest, ipDigest, s.policy); limitErr != nil {
				return SessionIssue{}, limitErr
			}
			if auditErr := s.writeAudit(ctx, input.RequestID, platformsecurity.AuditLoginFailed,
				"anonymous", platformsecurity.AuditResultFailure, platformsecurity.AuditSourceAnonymous, "invalid_credentials"); auditErr != nil {
				return SessionIssue{}, auditErr
			}
			return SessionIssue{}, unauthenticatedLoginFault()
		}
		return SessionIssue{}, err
	}
	if credential.Subject != "owner" || credential.CredentialVersion == 0 || credential.PasswordHash == "" {
		return SessionIssue{}, platformfault.New(platformfault.ClassInternal, "server.invalid_owner_credential", "Owner credential is invalid", nil)
	}
	valid, err := s.hasher.Verify(credential.PasswordHash, input.Password)
	if err != nil {
		return SessionIssue{}, platformfault.New(platformfault.ClassInternal, "server.owner_password_verification_failed", "Owner authentication is unavailable", err)
	}
	if !valid {
		result, limitErr := s.rateLimiter.RecordFailure(ctx, accountDigest, ipDigest, s.policy)
		if limitErr != nil {
			return SessionIssue{}, limitErr
		}
		eventType, resultType, reason := platformsecurity.AuditLoginFailed, platformsecurity.AuditResultFailure, "invalid_credentials"
		if !result.Allowed {
			eventType, resultType, reason = platformsecurity.AuditLoginRateLimited, platformsecurity.AuditResultBlocked, "login_rate_limited"
		}
		if auditErr := s.writeAudit(ctx, input.RequestID, eventType, "anonymous", resultType,
			platformsecurity.AuditSourceAnonymous, reason); auditErr != nil {
			return SessionIssue{}, auditErr
		}
		return SessionIssue{}, unauthenticatedLoginFault()
	}
	if err := s.rateLimiter.ResetAccount(ctx, accountDigest); err != nil {
		return SessionIssue{}, err
	}
	issue, digest, err := s.createSession(ctx, credential.CredentialVersion)
	if err != nil {
		return SessionIssue{}, err
	}
	if err := s.writeAudit(ctx, input.RequestID, platformsecurity.AuditSessionCreated,
		"owner", platformsecurity.AuditResultSuccess, platformsecurity.AuditSourceAnonymous, "login_session_created"); err != nil {
		_ = s.sessions.Revoke(ctx, digest)
		return SessionIssue{}, err
	}
	if err := s.writeAudit(ctx, input.RequestID, platformsecurity.AuditLoginSucceeded,
		"owner", platformsecurity.AuditResultSuccess, platformsecurity.AuditSourceAnonymous, "credentials_verified"); err != nil {
		_ = s.sessions.Revoke(ctx, digest)
		return SessionIssue{}, err
	}
	return issue, nil
}

func (s *SessionLifecycleService) Logout(ctx context.Context, rawToken, requestID string) (*http.Cookie, error) {
	if err := s.validateOperation(ctx, requestID); err != nil {
		return nil, err
	}
	digest, err := platformsecurity.SessionTokenDigest(rawToken)
	if err != nil {
		return nil, unauthenticatedLoginFault()
	}
	if err := s.sessions.Revoke(ctx, digest); err != nil {
		return nil, err
	}
	cookie, err := platformsecurity.ClearSessionCookie(s.cookie)
	if err != nil {
		return nil, err
	}
	auditErr := s.writeAudit(ctx, requestID, platformsecurity.AuditLogout,
		"owner", platformsecurity.AuditResultSuccess, platformsecurity.AuditSourceOwnerSession, "session_revoked")
	return cookie, auditErr
}

func (s *SessionLifecycleService) RotateSession(ctx context.Context, rawToken, requestID string) (SessionIssue, error) {
	if err := s.validateOperation(ctx, requestID); err != nil {
		return SessionIssue{}, err
	}
	oldDigest, err := platformsecurity.SessionTokenDigest(rawToken)
	if err != nil {
		return SessionIssue{}, unauthenticatedLoginFault()
	}
	now := s.now().UTC()
	record, err := s.sessions.LookupAndTouch(ctx, oldDigest, now, s.idleTTL)
	if errors.Is(err, platformsecurity.ErrSessionNotFound) {
		return SessionIssue{}, unauthenticatedLoginFault()
	}
	if err != nil {
		return SessionIssue{}, err
	}
	credential, err := s.credentials.GetOwnerCredential(ctx)
	if err != nil {
		return SessionIssue{}, err
	}
	if credential.Subject != "owner" || credential.CredentialVersion != record.CredentialVersion {
		return SessionIssue{}, unauthenticatedLoginFault()
	}
	newRaw, newDigest, err := platformsecurity.NewSessionToken()
	if err != nil {
		return SessionIssue{}, err
	}
	csrf, csrfDigest, err := platformsecurity.NewCSRFToken()
	if err != nil {
		return SessionIssue{}, err
	}
	record.LastSeenAt = now
	record.IdleExpiresAt = minTime(now.Add(s.idleTTL), record.AbsoluteExpiresAt)
	record.CSRFTokenDigest = csrfDigest
	if err := s.sessions.Rotate(ctx, oldDigest, newDigest, record, now); err != nil {
		return SessionIssue{}, err
	}
	cookie, err := platformsecurity.NewSessionCookie(s.cookie, newRaw, record.IdleExpiresAt, now)
	if err != nil {
		_ = s.sessions.Revoke(ctx, newDigest)
		return SessionIssue{}, err
	}
	if err := s.writeAudit(ctx, requestID, platformsecurity.AuditSessionRotated,
		"owner", platformsecurity.AuditResultSuccess, platformsecurity.AuditSourceOwnerSession, "session_rotated"); err != nil {
		_ = s.sessions.Revoke(ctx, newDigest)
		return SessionIssue{}, err
	}
	return SessionIssue{Cookie:\tcookie, CSRFToken: csrf}, nil
}

func (s *SessionLifecycleService) ChangePassword(ctx context.Context, currentPassword, newPassword []byte, requestID string) error {
	defer clear(currentPassword)
	defer clear(newPassword)
	if err := s.validateOperation(ctx, requestID); err != nil {
		return err
	}
	if len(currentPassword) == 0 || len(newPassword) == 0 || len(newPassword) > 1024 {
		return platformfault.New(platformfault.ClassValidation, "server.invalid_password_change", "password change input is invalid", nil)
	}
	credential, err := s.credentials.GetOwnerCredential(ctx)
	if err != nil {
		return err
	}
	valid, err := s.hasher.Verify(credential.PasswordHash, currentPassword)
	if err != nil {
		return platformfault.New(platformfault.ClassInternal, "server.owner_password_verification_failed", "Owner authentication is unavailable", err)
	}
	if !valid {
		if auditErr := s.writeAudit(ctx, requestID, platformsecurity.AuditPasswordChanged,
			"owner", platformsecurity.AuditResultFailure, platformsecurity.AuditSourceOwnerSession, "current_password_rejected"); auditErr != nil {
			return auditErr
		}
		return platformfault.New(platformfault.ClassUnauthenticated, "server.current_password_rejected", "current password is invalid", nil)
	}
	hash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return platformfault.New(platformfault.ClassValidation, "server.owner_password_hash_failed", "password could not be updated", err)
	}
	if _, err := s.credentials.UpdateOwnerPassword(ctx, credential.CredentialVersion, hash); err != nil {
		return err
	}
	return s.writeAudit(ctx, requestID, platformsecurity.AuditPasswordChanged,
		"owner", platformsecurity.AuditResultSuccess, platformsecurity.AuditSourceOwnerSession, "credential_version_advanced")
}

func (s *SessionLifecycleService) createSession(ctx context.Context, credentialVersion uint64) (SessionIssue, string, error) {
	now := s.now().UTC()
	rawToken, digest, err := platformsecurity.NewSessionToken()
	if err != nil {
		return SessionIssue{}, "", err
	}
	csrf, csrfDigest, err := platformsecurity.NewCSRFToken()
	if err != nil {
		return SessionIssue{}, "", err
	}
	record := platformsecurity.SessionRecord{
		Subject:\t"owner",
		CreatedAt:\tnow,
		LastSeenAt:\tnow,
		IdleExpiresAt:\tminTime(now.Add(s.idleTTL), now.Add(s.absoluteTTL)),
		AbsoluteExpiresAt:\tnow.Add(s.absoluteTTL),
		CredentialVersion:\tcredentialVersion,
		CSRFTokenDigest:\tcsrfDigest,
	}
	if err := s.sessions.Create(ctx, digest, record); err != nil {
		return SessionIssue{}, "", err
	}
	cookie, err := platformsecurity.NewSessionCookie(s.cookie, rawToken, record.IdleExpiresAt, now)
	if err != nil {
		_ = s.sessions.Revoke(ctx, digest)
		return SessionIssue{}, "", err
	}
	return SessionIssue{Cookie:\tcookie, CSRFToken: csrf}, digest, nil
}

func (s *SessionLifecycleService) writeAudit(ctx context.Context, requestID string, eventType platformsecurity.SecurityAuditEventType,
	subject string, result platformsecurity.SecurityAuditResult, source platformsecurity.SecurityAuditSource, reason string) error {
	return platformsecurity.WriteSecurityAudit(ctx, s.audit, platformsecurity.SecurityAuditEvent{
		EventType:\teventType, Subject:\tsubject, Result:\tresult, OccurredAt:\ts.now().UTC(),
		RequestID:\trequestID, Source:\tsource, ReasonCode:\treason,
	})
}

func (s *SessionLifecycleService) validateOperation(ctx context.Context, requestID string) error {
	if s == nil || ctx == nil || strings.TrimSpace(requestID) == "" {
		return platformfault.New(platformfault.ClassValidation, "server.invalid_security_operation", "security operation input is invalid", nil)
	}
	return nil
}

func isCredentialNotFound(err error) bool {
	var value *platformfault.Fault
	return errors.As(err, &value) && value.Class == platformfault.ClassNotFound
}

func unauthenticatedLoginFault() error {
	return platformfault.New(platformfault.ClassUnauthenticated, "server.owner_login_rejected", "account or password is invalid", nil)
}

func canonicalIP(ip net.IP) string {
	if value := ip.To4(); value != nil {
		return value.String()
	}
	return ip.To16().String()
}

func securityDigest(key []byte, value string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
