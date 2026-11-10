package config

import (
	"testing"
	"time"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

func TestLoadMigrationDefaultsToDisabled(t *testing.T) {
	cfg, err := LoadMigration(platformconfig.NewEnvWithLookup(func(string) (string, bool) {
		return "", false
	}))
	if err != nil {
		t.Fatalf("LoadMigration() error = %v", err)
	}
	if cfg.Enabled {
		t.Fatal("Enabled = true, want false by default")
	}
	if cfg.Timeout != 2*time.Minute {
		t.Fatalf("Timeout = %s, want 2m", cfg.Timeout)
	}
}

func TestLoadMigrationReadsExplicitSettings(t *testing.T) {
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		switch key {
		case "MIGRATION_ENABLED":
			return "true", true
		case "MIGRATION_TIMEOUT":
			return "45s", true
		default:
			return "", false
		}
	})
	cfg, err := LoadMigration(env)
	if err != nil {
		t.Fatalf("LoadMigration() error = %v", err)
	}
	if !cfg.Enabled || cfg.Timeout != 45*time.Second {
		t.Fatalf("unexpected migration config: %+v", cfg)
	}
}

func TestLoadMigrationRejectsInvalidSettings(t *testing.T) {
	tests := []struct {
		name string
		env  func(string) (string, bool)
	}{
		{
			name: "invalid enabled",
			env: func(key string) (string, bool) {
				if key == "MIGRATION_ENABLED" {
					return "sometimes", true
				}
				return "", false
			},
		},
		{
			name: "zero timeout",
			env: func(key string) (string, bool) {
				if key == "MIGRATION_TIMEOUT" {
					return "0s", true
				}
				return "", false
			},
		},
		{
			name: "timeout over maximum",
			env: func(key string) (string, bool) {
				if key == "MIGRATION_TIMEOUT" {
					return "31m", true
				}
				return "", false
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := LoadMigration(platformconfig.NewEnvWithLookup(tt.env)); err == nil {
				t.Fatal("LoadMigration() error = nil, want validation error")
			}
		})
	}
}
