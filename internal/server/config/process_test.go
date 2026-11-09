package config

import (
	"testing"
	"time"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

func TestLoadProcessDefaults(t *testing.T) {
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) { if key == "PLATFORM_ADMIN_TOKEN" { return "0123456789abcdef0123456789abcdef", true }; return "", false })
	cfg, err := LoadProcess(env)
	if err != nil {
		t.Fatalf("LoadProcess() error = %v", err)
	}
	if cfg.LogLevel != "info" || cfg.AnalysisTimeout != 5*time.Minute ||
		cfg.AnalysisWorkers != 2 || cfg.AnalysisMaxQueued != 100 ||
		cfg.Role != RoleAll || cfg.HTTP.Address != ":8092" {
		t.Fatalf("unexpected process defaults: %+v", cfg)
	}
}

func TestLoadProcessWorkerSkipsHTTPAndSecurity(t *testing.T) {
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		if key == "SERVER_ROLE" {
			return "worker", true
		}
		return "", false
	})
	cfg, err := LoadProcess(env)
	if err != nil {
		t.Fatalf("LoadProcess() error = %v", err)
	}
	if cfg.Role != RoleWorker || cfg.HTTP.Address != "" || cfg.Security.OwnerToken.Configured() {
		t.Fatalf("worker process unexpectedly loaded HTTP/security config: %+v", cfg)
	}
}

func TestLoadProcessRejectsInvalidProcessSettings(t *testing.T) {
	tests := []struct {
		name string
		key  string
		raw  string
	}{
		{name: "log level", key: "LOG_LEVEL", raw: "trace"},
		{name: "analysis timeout", key: "ANALYSIS_TIMEOUT", raw: "0s"},
		{name: "workers", key: "ANALYSIS_WORKERS", raw: "0"},
		{name: "queue smaller than workers", key: "ANALYSIS_MAX_QUEUED", raw: "1"},
		{name: "migration timeout", key: "MIGRATION_TIMEOUT", raw: "0s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
				if key == tt.key {
					return tt.raw, true
				}
				if key == "SERVER_ROLE" {
					return "worker", true
				}
				return "", false
			})
			if _, err := LoadProcess(env); err == nil {
				t.Fatal("LoadProcess() error = nil, want validation error")
			}
		})
	}
}

func TestLoadProcessReturnsStableSnapshot(t *testing.T) {
	values := map[string]string{
		"LOG_LEVEL": "debug",
		"ANALYSIS_WORKERS": "4",
		"ANALYSIS_MAX_QUEUED": "200",
		"SERVER_ROLE": "worker",
	}
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	cfg, err := LoadProcess(env)
	if err != nil {
		t.Fatalf("LoadProcess() error = %v", err)
	}
	values["LOG_LEVEL"] = "error"
	values["ANALYSIS_WORKERS"] = "8"
	if cfg.LogLevel != "debug" || cfg.AnalysisWorkers != 4 {
		t.Fatalf("process snapshot changed after source mutation: %+v", cfg)
	}
}
