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
		cfg.IdleTimeout != 60*time.Second || cfg.ShutdownTimeout != 10*time.Second ||
		cfg.MaxRequestBodyBytes != 4<<20 || cfg.MaxConcurrentRequests != 64 {
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

func TestLoadHTTPRejectsInvalidAdmissionLimits(t *testing.T) {
	tests := []struct {
		name string
		key  string
		raw  string
	}{
		{name: "request body too small", key: "HTTP_MAX_REQUEST_BYTES", raw: "512"},
		{name: "request body too large", key: "HTTP_MAX_REQUEST_BYTES", raw: "67108865"},
		{name: "concurrency zero", key: "HTTP_MAX_CONCURRENT_REQUESTS", raw: "0"},
		{name: "concurrency too large", key: "HTTP_MAX_CONCURRENT_REQUESTS", raw: "1025"},
		{name: "non integer", key: "HTTP_MAX_CONCURRENT_REQUESTS", raw: "many"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
				if key == test.key {
					return test.raw, true
				}
				return "", false
			})
			if _, err := LoadHTTP(env, 5*time.Minute); err == nil {
				t.Fatal("LoadHTTP() error = nil, want validation error")
			}
		})
	}
}

func TestLoadHTTPAcceptsConfiguredAdmissionLimits(t *testing.T) {
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		switch key {
		case "HTTP_MAX_REQUEST_BYTES":
			return "8388608", true
		case "HTTP_MAX_CONCURRENT_REQUESTS":
			return "128", true
		default:
			return "", false
		}
	})
	cfg, err := LoadHTTP(env, 5*time.Minute)
	if err != nil {
		t.Fatalf("LoadHTTP() error = %v", err)
	}
	if cfg.MaxRequestBodyBytes != 8<<20 || cfg.MaxConcurrentRequests != 128 {
		t.Fatalf("unexpected admission limits: %+v", cfg)
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

func TestLoadResearchflowCapacityRequiresBothExplicitPositiveLimits(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
		want   bool
	}{
		{name: "not configured", values: map[string]string{}, want: false},
		{name: "both configured", values: map[string]string{
			"RESEARCHFLOW_MAX_QUEUED_RUNS": "120",
			"RESEARCHFLOW_MAX_WORKERS":     "6",
		}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
				value, ok := test.values[key]
				return value, ok
			})
			got, err := LoadResearchflowCapacity(env)
			if err != nil {
				t.Fatalf("LoadResearchflowCapacity() error = %v", err)
			}
			if got.Configured != test.want {
				t.Fatalf("Configured = %v, want %v", got.Configured, test.want)
			}
			if test.want && (got.MaxQueuedRuns != 120 || got.MaxWorkers != 6) {
				t.Fatalf("unexpected capacity: %+v", got)
			}
		})
	}
}

func TestLoadResearchflowCapacityRejectsPartialOrInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
	}{
		{name: "missing workers", values: map[string]string{"RESEARCHFLOW_MAX_QUEUED_RUNS": "10"}},
		{name: "missing queue", values: map[string]string{"RESEARCHFLOW_MAX_WORKERS": "2"}},
		{name: "zero queue", values: map[string]string{"RESEARCHFLOW_MAX_QUEUED_RUNS": "0", "RESEARCHFLOW_MAX_WORKERS": "2"}},
		{name: "negative workers", values: map[string]string{"RESEARCHFLOW_MAX_QUEUED_RUNS": "10", "RESEARCHFLOW_MAX_WORKERS": "-1"}},
		{name: "non integer", values: map[string]string{"RESEARCHFLOW_MAX_QUEUED_RUNS": "many", "RESEARCHFLOW_MAX_WORKERS": "2"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
				value, ok := test.values[key]
				return value, ok
			})
			if _, err := LoadResearchflowCapacity(env); err == nil {
				t.Fatal("LoadResearchflowCapacity() error = nil, want validation error")
			}
		})
	}
}
