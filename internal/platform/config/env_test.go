package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func testEnv(values map[string]string) Env {
	return NewEnvWithLookup(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
}

func TestEnvTypedValues(t *testing.T) {
	env := testEnv(map[string]string{
		"APP_NAME":    "stock-api",
		"APP_PORT":    "8092",
		"APP_DEBUG":   "true",
		"APP_TIMEOUT": "5m",
	})

	if got := env.String("APP_NAME", "fallback"); got != "stock-api" {
		t.Fatalf("String() = %q", got)
	}
	if got, err := env.Int("APP_PORT", 80); err != nil || got != 8092 {
		t.Fatalf("Int() = %d, %v", got, err)
	}
	if got, err := env.Bool("APP_DEBUG", false); err != nil || !got {
		t.Fatalf("Bool() = %v, %v", got, err)
	}
	if got, err := env.Duration("APP_TIMEOUT", time.Second); err != nil || got != 5*time.Minute {
		t.Fatalf("Duration() = %v, %v", got, err)
	}
	if got := env.String("MISSING", "fallback"); got != "fallback" {
		t.Fatalf("fallback = %q", got)
	}
}

func TestEnvInvalidValuesDoNotLeak(t *testing.T) {
	const secretValue = "super-secret-value"
	env := testEnv(map[string]string{
		"APP_PORT":    secretValue,
		"APP_DEBUG":   secretValue,
		"APP_TIMEOUT": secretValue,
	})

	for _, err := range []error{
		func() error { _, err := env.Int("APP_PORT", 1); return err }(),
		func() error { _, err := env.Bool("APP_DEBUG", false); return err }(),
		func() error { _, err := env.Duration("APP_TIMEOUT", time.Second); return err }(),
	} {
		if err == nil || strings.Contains(err.Error(), secretValue) {
			t.Fatalf("expected sanitized parse error, got %v", err)
		}
	}
}

func TestRequiredValues(t *testing.T) {
	env := testEnv(map[string]string{"EMPTY": "  ", "VALUE": "configured"})
	if _, err := env.RequiredString("EMPTY"); err == nil {
		t.Fatal("expected empty required string to fail")
	}
	if got, err := env.RequiredString("VALUE"); err != nil || got != "configured" {
		t.Fatalf("RequiredString() = %q, %v", got, err)
	}
	if _, err := env.RequiredSecret("MISSING_SECRET"); err == nil {
		t.Fatal("expected missing secret to fail")
	}
}

func TestSecretAlwaysRedacts(t *testing.T) {
	const raw = "token-do-not-leak"
	secret := testEnv(map[string]string{"API_TOKEN": raw}).Secret("API_TOKEN")
	if !secret.Configured() || secret.Value() != raw {
		t.Fatal("secret value was not retained")
	}

	formatted := fmt.Sprintf("%v %#v %+v %q", secret, secret, secret, secret)
	encoded, err := json.Marshal(secret)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(formatted, raw) || strings.Contains(string(encoded), raw) {
		t.Fatalf("secret leaked: formatted=%q json=%s", formatted, encoded)
	}
	if string(encoded) != "\"[REDACTED]\"" {
		t.Fatalf("unexpected JSON redaction: %s", encoded)
	}
}

func TestValidationHelpers(t *testing.T) {
	if err := ValidatePositiveInt("APP_WORKERS", 0); err == nil {
		t.Fatal("expected positive integer validation to fail")
	}
	if err := ValidatePositiveDuration("APP_TIMEOUT", -time.Second); err == nil {
		t.Fatal("expected positive duration validation to fail")
	}
	if err := ValidateRequired("APP_NAME", " "); err == nil {
		t.Fatal("expected required value validation to fail")
	}
	if err := JoinValidationErrors(nil, ValidateRequired("APP_NAME", "")); err == nil {
		t.Fatal("expected joined validation error")
	}
}

func TestInvalidConfigurationKeyIsIgnored(t *testing.T) {
	env := testEnv(map[string]string{"APP TOKEN": "unexpected"})
	if got := env.String("APP TOKEN", "fallback"); got != "fallback" {
		t.Fatalf("invalid configuration key returned %q", got)
	}
}
