package config

import (
	"strings"
	"testing"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

func TestLoadSecurityRequiresOwnerToken(t *testing.T) {
	_, err := LoadSecurity(platformconfig.NewEnvWithLookup(func(string) (string, bool) {
		return "", false
	}))
	if err == nil || !strings.Contains(err.Error(), "PLATFORM_ADMIN_TOKEN") {
		t.Fatalf("LoadSecurity() error = %v, want missing owner token", err)
	}
}

func TestLoadSecurityLoadsRedactedOwnerToken(t *testing.T) {
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		if key == "PLATFORM_ADMIN_TOKEN" {
			return strings.Repeat("a", 43), true
		}
		return "", false
	})
	cfg, err := LoadSecurity(env)
	if err != nil {
		t.Fatalf("LoadSecurity() error = %v", err)
	}
	if !cfg.OwnerToken.Configured() {
		t.Fatal("OwnerToken is not configured")
	}
	if got := cfg.OwnerToken.String(); got != "[REDACTED]" {
		t.Fatalf("OwnerToken.String() = %q, want redacted", got)
	}
}
