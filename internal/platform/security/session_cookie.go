package security

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/platform/fault"
)

// SessionCookieConfig is supplied by Server because cookie names and public
// paths belong to the deployed Web/API contract.
type SessionCookieConfig struct {
	Name     string
	Path     string
	Domain   string
	Secure   bool
	SameSite http.SameSite
}

// Validate rejects cookie configurations that broaden scope or weaken the
// browser-side protections required for an authenticated session.
func (c SessionCookieConfig) Validate() error {
	if c.Name == "" || !validCookieName(c.Name) ||
		c.Path == "" || !strings.HasPrefix(c.Path, "/") ||
		strings.ContainsAny(c.Path, ";\r\n") ||
		strings.Contains(c.Path, "..") || c.Path == "/" ||
		strings.TrimSpace(c.Domain) != "" {
		return errors.New("session cookie name/path/domain is invalid")
	}
	if c.SameSite != http.SameSiteLaxMode {
		return errors.New("session cookie SameSite must be Lax")
	}
	return nil
}

// NewSessionCookie creates an HttpOnly, SameSite=Lax cookie. Secure must be
// enabled by Server when the public deployment uses HTTPS.
func NewSessionCookie(config SessionCookieConfig, rawToken string, expiresAt, now time.Time) (*http.Cookie, error) {
	if err := config.Validate(); err != nil {
		return nil, fault.New(fault.ClassValidation, "platform.security.invalid_session_cookie", "session cookie configuration is invalid", err)
	}
	if _, err := SessionTokenDigest(rawToken); err != nil {
		return nil, err
	}
	if now.IsZero() || expiresAt.IsZero() || !expiresAt.After(now) {
		return nil, fault.New(fault.ClassValidation, "platform.security.invalid_session_cookie_expiry", "session cookie expiry is invalid", nil)
	}
	maxAge := int(expiresAt.Sub(now).Seconds())
	if maxAge < 1 {
		return nil, fault.New(fault.ClassValidation, "platform.security.invalid_session_cookie_expiry", "session cookie expiry is invalid", nil)
	}
	return &http.Cookie{
		Name: config.Name, Value: rawToken, Path: config.Path,
		HttpOnly: true, Secure: config.Secure, SameSite: http.SameSiteLaxMode,
		Expires: expiresAt.UTC(), MaxAge: maxAge,
	}, nil
}

// ClearSessionCookie returns an expired cookie with the same scope attributes.
func ClearSessionCookie(config SessionCookieConfig) (*http.Cookie, error) {
	if err := config.Validate(); err != nil {
		return nil, fault.New(fault.ClassValidation, "platform.security.invalid_session_cookie", "session cookie configuration is invalid", err)
	}
	return &http.Cookie{
		Name: config.Name, Value: "", Path: config.Path, HttpOnly: true,
		Secure: config.Secure, SameSite: http.SameSiteLaxMode,
		Expires: time.Unix(1, 0).UTC(), MaxAge: -1,
	}, nil
}

func validCookieName(name string) bool {
	for _, char := range []byte(name) {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}
