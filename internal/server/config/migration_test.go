package config

import (
	"testing"
	"time"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

func TestLoadMigrationDefaultsTimeout(t *testing.T) {
	cfg, err := LoadMigration(platformconfig.NewEnvWithLookup(func(string) (string, bool) {
		return "", false
	}))
	if err != nil {
		t.Fatalf("LoadMigration() error = %v", err)
	}
	if cfg.Timeout != 2*time.Minute {
		t.Fatalf("Timeout = %s, want 2m", cfg.Timeout)
	}
}

func TestLoadMigrationReadsTimeout(t *testing.T) {
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		if key == "MIGRATION_TIMEOUT" {
			return "45s", true
		}
		return "", false
	})
	cfg, err := LoadMigration(env)
	if err != nil {
		t.Fatalf("LoadMigration() error = %v", err)
	}
	if cfg.Timeout != 45*time.Second {
		t.Fatalf("Timeout = %s, want 45s", cfg.Timeout)
	}
}

func TestLoadMigrationRejectsInvalidTimeout(t *testing.T) {
	for _, value := range []string{"invalid", "0s", "-1s", "31m"} {
		t.Run(value, func(t *testing.T) {
			env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
				if key == "MIGRATION_TIMEOUT" {
					return value, true
				}
				return "", false
			})
			if _, err := LoadMigration(env); err == nil {
				t.Fatal("LoadMigration() error = nil, want validation error")
			}
		})
	}
}
