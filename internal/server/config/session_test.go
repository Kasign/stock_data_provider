package config

import (
	"net/http"
	"strings"
	"testing"
	"time"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

func TestLoadOwnerSessionDisabledByDefault(t *testing.T) {
	cfg, err := LoadOwnerSession(platformconfig.NewEnvWithLookup(func(string) (string, bool) { return "", false }))
	if err != nil || cfg.Enabled {
		t.Fatalf("LoadOwnerSession() = %+v, %v; want disabled", cfg, err)
	}
}

func TestLoadOwnerSessionRequiresExplicitArgonCosts(t *testing.T) {
	values := map[string]string{
		"OWNER_SESSION_ENABLED": "true",
		"OWNER_SESSION_PUBLIC_ORIGIN": "https://stocks.example.com",
	}
	_, err := LoadOwnerSession(platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}))
	if err == nil || !strings.Contains(err.Error(), "OWNER_SESSION_ARGON2_MEMORY_KIB") {
		t.Fatalf("LoadOwnerSession() error = %v, want explicit Argon2id cost", err)
	}
}

func TestLoadOwnerSessionLoadsExplicitConfiguration(t *testing.T) {
	values := map[string]string{
		"OWNER_SESSION_ENABLED": "true",
		"OWNER_SESSION_PUBLIC_ORIGIN": "https://stocks.example.com",
		"OWNER_SESSION_ARGON2_MEMORY_KIB": "65536",
		"OWNER_SESSION_ARGON2_ITERATIONS": "3",
		"OWNER_SESSION_ARGON2_PARALLELISM": "2",
	}
	cfg, err := LoadOwnerSession(platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}))
	if err != nil {
		t.Fatalf("LoadOwnerSession() error = %v", err)
	}
	if !cfg.Enabled || !cfg.Cookie.Secure || cfg.Cookie.SameSite != http.SameSiteLaxMode ||
		cfg.IdleTTL != 12*time.Hour || cfg.AbsoluteTTL != 7*24*time.Hour ||
		cfg.Argon2id.Memory != 65536 || cfg.Argon2id.Iterations != 3 ||
		cfg.Argon2id.Parallelism != 2 {
		t.Fatalf("unexpected Owner Session config: %+v", cfg)
	}
}

func TestLoadOwnerSessionRejectsInsecureHTTPSCookie(t *testing.T) {
	values := map[string]string{
		"OWNER_SESSION_ENABLED": "true",
		"OWNER_SESSION_PUBLIC_ORIGIN": "https://stocks.example.com",
		"OWNER_SESSION_COOKIE_SECURE": "false",
		"OWNER_SESSION_ARGON2_MEMORY_KIB": "65536",
		"OWNER_SESSION_ARGON2_ITERATIONS": "3",
		"OWNER_SESSION_ARGON2_PARALLELISM": "2",
	}
	_, err := LoadOwnerSession(platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}))
	if err == nil || !strings.Contains(err.Error(), "HTTPS Origin") {
		t.Fatalf("LoadOwnerSession() error = %v, want HTTPS Secure rejection", err)
	}
}

func TestLoadOwnerSessionRejectsOutOfRangeArgonCosts(t *testing.T) {
	values := map[string]string{
		"OWNER_SESSION_ENABLED": "true",
		"OWNER_SESSION_PUBLIC_ORIGIN": "https://stocks.example.com",
		"OWNER_SESSION_ARGON2_MEMORY_KIB": "4294967296",
		"OWNER_SESSION_ARGON2_ITERATIONS": "3",
		"OWNER_SESSION_ARGON2_PARALLELISM": "2",
	}
	_, err := LoadOwnerSession(platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}))
	if err == nil || !strings.Contains(err.Error(), "安全上限") {
		t.Fatalf("LoadOwnerSession() error = %v, want bounded Argon2id costs", err)
	}
}
