package config

import (
	"testing"
	"time"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

func TestLoadHTTPDefaults(t *testing.T) {
	cfg, err := LoadHTTP(platformconfig.NewEnvWithLookup(func(string) (string, bool) {
		return "", false
	}), 5*time.Minute)
	if err != nil {
		t.Fatalf("LoadHTTP() error = %v", err)
	}
	if cfg.Address != ":8092" || cfg.ReadTimeout != 15*time.Second ||
		cfg.WriteTimeout != 5*time.Minute+10*time.Second ||
		cfg.IdleTimeout != 60*time.Second || cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("unexpected HTTP defaults: %+v", cfg)
	}
}

func TestLoadHTTPUsesDefaultForBlankAddress(t *testing.T) {
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		if key == "HTTP_ADDR" {
			return " ", true
		}
		return "", false
	})
	cfg, err := LoadHTTP(env, 5*time.Minute)
	if err != nil {
		t.Fatalf("LoadHTTP() error = %v", err)
	}
	if cfg.Address != ":8092" {
		t.Fatalf("Address = %q, want :8092", cfg.Address)
	}
}

func TestLoadHTTPRejectsInvalidTimeout(t *testing.T) {
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		if key == "HTTP_READ_TIMEOUT" {
			return "0s", true
		}
		return "", false
	})
	if _, err := LoadHTTP(env, 5*time.Minute); err == nil {
		t.Fatal("LoadHTTP() error = nil, want invalid timeout error")
	}
}

func TestLoadRoleDefaultsToAll(t *testing.T) {
	role, err := LoadRole(platformconfig.NewEnvWithLookup(func(string) (string, bool) { return "", false }))
	if err != nil || role != RoleAll {
		t.Fatalf("LoadRole() = %q, %v; want all", role, err)
	}
}

func TestLoadRoleAcceptsExplicitRoles(t *testing.T) {
	for _, want := range []Role{RoleAPI, RoleWorker, RoleAll} {
		env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
			if key == "SERVER_ROLE" {
				return string(want), true
			}
			return "", false
		})
		got, err := LoadRole(env)
		if err != nil || got != want {
			t.Fatalf("LoadRole(%q) = %q, %v", want, got, err)
		}
	}
}

func TestLoadRoleRejectsUnknownRole(t *testing.T) {
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		if key == "SERVER_ROLE" {
			return "scheduler", true
		}
		return "", false
	})
	if _, err := LoadRole(env); err == nil {
		t.Fatal("LoadRole() error = nil, want invalid role")
	}
}