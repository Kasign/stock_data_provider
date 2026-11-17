package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"

	"example.com/stock-analysis-flow/internal/platform/fault"
)

const CSRFTokenBytes = 32

// NewCSRFToken creates a high-entropy token for one authenticated session.
// The caller returns the raw token to the same-origin Web client and stores
// only the digest in SessionRecord.CSRFTokenDigest.
func NewCSRFToken() (rawToken, tokenDigest string, err error) {
	secret := make([]byte, CSRFTokenBytes)
	if _, err := rand.Read(secret); err != nil {
		return "", "", fault.New(fault.ClassInternal, "platform.security.csrf_token_generation_failed", "CSRF token could not be generated", err)
	}
	rawToken = base64.RawURLEncoding.EncodeToString(secret)
	digest := sha256.Sum256([]byte(rawToken))
	return rawToken, hex.EncodeToString(digest[:]), nil
}

// CSRFTokenDigest validates the canonical unpadded URL-safe token format and
// returns its SHA-256 digest for storage in the corresponding SessionRecord.
func CSRFTokenDigest(rawToken string) (string, error) {
	if len(rawToken) != base64.RawURLEncoding.EncodedLen(CSRFTokenBytes) {
		return "", fault.New(fault.ClassUnauthenticated, "platform.security.invalid_csrf_token", "CSRF token is invalid", nil)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(rawToken)
	if err != nil || len(decoded) != CSRFTokenBytes || base64.RawURLEncoding.EncodeToString(decoded) != rawToken {
		return "", fault.New(fault.ClassUnauthenticated, "platform.security.invalid_csrf_token", "CSRF token is invalid", nil)
	}
	digest := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(digest[:]), nil
}

// VerifyCSRFToken compares the presented token digest with the digest stored
// for the authenticated session using constant-time comparison.
func VerifyCSRFToken(rawToken, expectedDigest string) bool {
	if !validSessionDigest(expectedDigest) {
		return false
	}
	actualDigest, err := CSRFTokenDigest(rawToken)
	if err != nil {
		return false
	}
	actual, err := hex.DecodeString(actualDigest)
	if err != nil {
		return false
	}
	expected, err := hex.DecodeString(expectedDigest)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(actual, expected) == 1
}
