// Package security contains Root Server authentication adapters built on
// Platform security primitives.
package security

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	platformfault "example.com/stock-analysis-flow/internal/platform/fault"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
)

const DefaultCSRFAuthHeader = "X-CSRF-Token"

type SessionAuthenticatorConfig struct {
	Cookie       platformsecurity.SessionCookieConfig
	PublicOrigin string
	CSRFHeader   string
	IdleTTL      time.Duration
	Now          func() time.Time
}

// SessionAuthenticator verifies browser Owner Sessions independently from
// Platform Bearer authentication. It never upgrades a Session to Bearer scope.
type SessionAuthenticator struct {
	sessions    platformsecurity.SessionStore
	credentials platformsecurity.CredentialStore
	cookie      platformsecurity.SessionCookieConfig
	origin      *url.URL
	csrfHeader  string
	idleTTL     time.Duration
	now         func() time.Time
}

func NewSessionAuthenticator(
	sessions platformsecurity.SessionStore,
	credentials platformsecurity.CredentialStore,
	cfg SessionAuthenticatorConfig,
) (*SessionAuthenticator, error) {
	if sessions == nil || credentials == nil {
		return nil, errors.New("SessionStore and CredentialStore are required")
	}
	if err := cfg.Cookie.Validate(); err != nil {
		return nil, fmt.Errorf("Session Cookie 配置无效: %w", err)
	}
	originText := strings.TrimSpace(cfg.PublicOrigin)
	origin, err := url.Parse(originText)
	if err != nil || originText == "" || origin.Scheme == "" || origin.Host == "" ||
		origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return nil, errors.New("公开 Origin 必须是无路径的绝对 HTTP(S) Origin")
	}
	if origin.Scheme != "http" && origin.Scheme != "https" {
		return nil, errors.New("公开 Origin 只允许 HTTP 或 HTTPS")
	}
	if origin.Scheme == "https" && !cfg.Cookie.Secure {
		return nil, errors.New("HTTPS Origin 必须启用 Secure Session Cookie")
	}
	header := http.CanonicalHeaderKey(strings.TrimSpace(cfg.CSRFHeader))
	if header == "" || strings.ContainsAny(header, " \t\r\n:") {
		return nil, errors.New("CSRF Header 名称无效")
	}
	if cfg.IdleTTL < time.Second || cfg.IdleTTL > platformsecurity.DefaultSessionIdleTTL {
		return nil, fmt.Errorf("Session 空闲期限必须在 0 到 %s 之间", platformsecurity.DefaultSessionIdleTTL)
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &SessionAuthenticator{
		sessions: sessions, credentials: credentials, cookie: cfg.Cookie,
		origin: origin, csrfHeader: header, idleTTL: cfg.IdleTTL, now: now,
	}, nil
}

// Authenticate validates the Session Cookie, Redis record, authoritative
// MariaDB credential version, Host/Origin and CSRF token. The returned request
// carries an owner_session Principal; the returned cookie refreshes idle expiry.
// Callers must use the returned request and emit the cookie only after success.
func (a *SessionAuthenticator) Authenticate(request *http.Request) (*http.Request, *http.Cookie, error) {
	if a == nil || request == nil || request.URL == nil {
		return nil, nil, platformfault.New(platformfault.ClassValidation, "server.invalid_session_authentication", "Session authentication input is invalid", nil)
	}
	if err := a.validateRequestOrigin(request); err != nil {
		return nil, nil, err
	}
	rawToken, err := a.sessionCookie(request)
	if err != nil {
		return nil, nil, err
	}
	digest, err := platformsecurity.SessionTokenDigest(rawToken)
	if err != nil {
		return nil, nil, invalidSessionFault()
	}
	now := a.now().UTC()
	record, err := a.sessions.LookupAndTouch(request.Context(), digest, now, a.idleTTL)
	if errors.Is(err, platformsecurity.ErrSessionNotFound) {
		return nil, nil, invalidSessionFault()
	}
	if err != nil {
		return nil, nil, err
	}
	if record.Subject != "owner" || record.CredentialVersion == 0 || !validDigest(record.CSRFTokenDigest) {
		return nil, nil, invalidSessionFault()
	}
	if !isSafeMethod(request.Method) {
		csrfValues := request.Header.Values(a.csrfHeader)
		if len(csrfValues) != 1 || !platformsecurity.VerifyCSRFToken(csrfValues[0], record.CSRFTokenDigest) {
			return nil, nil, platformfault.New(platformfault.ClassForbidden, "server.csrf_rejected", "request origin or CSRF token is invalid", nil)
		}
	}
	credential, err := a.credentials.GetOwnerCredential(request.Context())
	if err != nil {
		var credentialFault *platformfault.Fault
		if errors.As(err, &credentialFault) && credentialFault.Class == platformfault.ClassNotFound {
			return nil, nil, invalidSessionFault()
		}
		return nil, nil, err
	}
	if credential.Subject != "owner" || credential.CredentialVersion == 0 ||
		credential.CredentialVersion != record.CredentialVersion {
		return nil, nil, invalidSessionFault()
	}
	expiresAt := record.IdleExpiresAt
	if record.AbsoluteExpiresAt.Before(expiresAt) {
		expiresAt = record.AbsoluteExpiresAt
	}
	cookie, err := platformsecurity.NewSessionCookie(a.cookie, rawToken, expiresAt, now)
	if err != nil {
		return nil, nil, invalidSessionFault()
	}
	ctx, err := platformsecurity.WithPrincipal(request.Context(), platformsecurity.Principal{
		Kind: platformsecurity.IdentityOwner, AuthMethod: platformsecurity.AuthMethodOwnerSession,
		Subject: "owner",
	})
	if err != nil {
		return nil, nil, fmt.Errorf("建立 Owner Session Principal 失败: %w", err)
	}
	return request.WithContext(ctx), cookie, nil
}

func (a *SessionAuthenticator) validateRequestOrigin(request *http.Request) error {
	if !strings.EqualFold(request.Host, a.origin.Host) {
		return platformfault.New(platformfault.ClassForbidden, "server.host_rejected", "request Host is not allowed", nil)
	}
	origins := request.Header.Values("Origin")
	if len(origins) > 1 {
		return platformfault.New(platformfault.ClassForbidden, "server.origin_rejected", "request Origin is not allowed", nil)
	}
	if isSafeMethod(request.Method) && len(origins) == 0 {
		return nil
	}
	if len(origins) != 1 {
		return platformfault.New(platformfault.ClassForbidden, "server.origin_rejected", "request Origin is required", nil)
	}
	origin, err := url.Parse(strings.TrimSpace(origins[0]))
	if err != nil || origin.User != nil || origin.Path != "" || origin.RawQuery != "" ||
		origin.Fragment != "" || !strings.EqualFold(origin.Scheme, a.origin.Scheme) ||
		!strings.EqualFold(origin.Host, a.origin.Host) {
		return platformfault.New(platformfault.ClassForbidden, "server.origin_rejected", "request Origin is not allowed", nil)
	}
	return nil
}

func (a *SessionAuthenticator) sessionCookie(request *http.Request) (string, error) {
	var value string
	count := 0
	for _, cookie := range request.Cookies() {
		if cookie.Name != a.cookie.Name {
			continue
		}
		count++
		value = cookie.Value
	}
	if count != 1 || value == "" {
		return "", invalidSessionFault()
	}
	return value, nil
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func validDigest(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func invalidSessionFault() *platformfault.Fault {
	return platformfault.New(platformfault.ClassUnauthenticated, "server.session_unauthenticated", "authentication required", nil)
}
