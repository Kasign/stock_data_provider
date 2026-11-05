// Package config provides small, typed configuration helpers.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Env reads process configuration without exposing environment access to
// business code. Each service should decode its own Config struct.
type Env struct {
	lookup func(string) (string, bool)
}

// NewEnv creates an environment reader backed by os.LookupEnv.
func NewEnv() Env {
	return Env{lookup: os.LookupEnv}
}

// NewEnvWithLookup creates an environment reader with an explicit source.
// It is useful for tests and controlled bootstrap configuration.
func NewEnvWithLookup(lookup func(string) (string, bool)) Env {
	if lookup == nil {
		return Env{lookup: func(string) (string, bool) { return "", false }}
	}
	return Env{lookup: lookup}
}

// lookupValue returns a raw value only inside this package so callers use Secret for sensitive keys.
func (e Env) lookupValue(key string) (string, bool) {
	if !validKey(key) || e.lookup == nil {
		return "", false
	}
	return e.lookup(key)
}

// String returns the configured value or fallback. An explicitly configured
// empty value is treated as empty rather than silently replaced.
func (e Env) String(key, fallback string) string {
	value, ok := e.lookupValue(key)
	if !ok {
		return fallback
	}
	return value
}

// RequiredString returns a non-empty value. Errors contain the key, never the
// configured value.
func (e Env) RequiredString(key string) (string, error) {
	value, ok := e.lookupValue(key)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("configuration %s is required", key)
	}
	return value, nil
}

// Bool parses a boolean value using strconv.ParseBool.
func (e Env) Bool(key string, fallback bool) (bool, error) {
	value, ok := e.lookupValue(key)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fallback, fmt.Errorf("configuration %s must be a boolean", key)
	}
	return parsed, nil
}

// Int parses a base-10 integer. Range validation remains the responsibility
// of the service-specific Config.Validate method.
func (e Env) Int(key string, fallback int) (int, error) {
	value, ok := e.lookupValue(key)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback, fmt.Errorf("configuration %s must be an integer", key)
	}
	return parsed, nil
}

// Duration parses a Go duration such as 5s or 2m.
func (e Env) Duration(key string, fallback time.Duration) (time.Duration, error) {
	value, ok := e.lookupValue(key)
	if !ok {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return fallback, fmt.Errorf("configuration %s must be a duration", key)
	}
	return parsed, nil
}

// Secret reads an optional secret. The zero value means not configured.
func (e Env) Secret(key string) Secret {
	value, ok := e.lookupValue(key)
	if !ok {
		return Secret{}
	}
	return Secret{value: value}
}

// RequiredSecret reads a mandatory secret without including its value in
// errors.
func (e Env) RequiredSecret(key string) (Secret, error) {
	value, ok := e.lookupValue(key)
	if !ok || strings.TrimSpace(value) == "" {
		return Secret{}, fmt.Errorf("configuration %s is required", key)
	}
	return Secret{value: value}, nil
}

func validKey(key string) bool {
	if strings.TrimSpace(key) == "" {
		return false
	}
	for _, r := range key {
		if !(r == '_' || r == '.' || r == '-' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// Secret stores sensitive configuration. Its value is only available through
// the explicitly named Value method; all formatting and JSON output redact it.
type Secret struct {
	value string
}

// Value returns the secret for an intentional use such as constructing an
// authenticated client. Callers must never log or include it in errors.
func (s Secret) Value() string {
	return s.value
}

// Configured reports whether a non-empty secret was provided.
func (s Secret) Configured() bool {
	return strings.TrimSpace(s.value) != ""
}

// String always redacts the secret.
func (Secret) String() string {
	return "[REDACTED]"
}

// GoString always redacts the secret, including %#v formatting.
func (s Secret) GoString() string {
	if !s.Configured() {
		return "[REDACTED]"
	}
	return "[REDACTED]"
}

// Format prevents fmt verbs from exposing the underlying secret.
func (Secret) Format(state fmt.State, _ rune) {
	_, _ = state.Write([]byte("[REDACTED]"))
}

// MarshalJSON serializes only a redaction marker, never the secret.
func (Secret) MarshalJSON() ([]byte, error) {
	return json.Marshal("[REDACTED]")
}

// MarshalText redacts the secret for text-based serializers.
func (Secret) MarshalText() ([]byte, error) {
	return []byte("[REDACTED]"), nil
}

// IsZero supports configuration validation without exposing the value.
func (s Secret) IsZero() bool {
	return !s.Configured()
}

// ValidateRequired reports a missing required configuration value.
func ValidateRequired(key, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("configuration %s is required", key)
	}
	return nil
}

// ValidatePositiveDuration reports a non-positive duration without echoing
// the supplied value.
func ValidatePositiveDuration(key string, value time.Duration) error {
	if value <= 0 {
		return fmt.Errorf("configuration %s must be greater than zero", key)
	}
	return nil
}

// ValidatePositiveInt reports a non-positive integer without echoing
// the supplied value.
func ValidatePositiveInt(key string, value int) error {
	if value <= 0 {
		return fmt.Errorf("configuration %s must be greater than zero", key)
	}
	return nil
}

// JoinValidationErrors combines independent service-specific validation
// errors while preserving errors.Is/errors.As behavior.
func JoinValidationErrors(errs ...error) error {
	return errors.Join(errs...)
}
