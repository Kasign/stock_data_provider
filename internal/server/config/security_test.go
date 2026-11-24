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
	if len(cfg.ServiceCredentials) != 0 {
		t.Fatalf("ServiceCredentials = %d, want none", len(cfg.ServiceCredentials))
	}
}

func TestLoadSecurityLoadsExplicitServiceCredentials(t *testing.T) {
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		switch key {
		case "PLATFORM_ADMIN_TOKEN":
			return strings.Repeat("a", 43), true
		case "PLATFORM_SERVICE_CREDENTIALS":
			return `[{"name":"quant","token_env":"QUANT_SERVICE_TOKEN","scopes":["data:read"]}]`, true
		case "QUANT_SERVICE_TOKEN":
			return strings.Repeat("b", 43), true
		default:
			return "", false
		}
	})
	cfg, err := LoadSecurity(env)
	if err != nil {
		t.Fatalf("LoadSecurity() error = %v", err)
	}
	if len(cfg.ServiceCredentials) != 1 {
		t.Fatalf("ServiceCredentials = %d, want 1", len(cfg.ServiceCredentials))
	}
	credential := cfg.ServiceCredentials[0]
	if credential.Name != "quant" || credential.Secret.Value() != strings.Repeat("b", 43) ||
		len(credential.Scopes) != 1 || credential.Scopes[0] != "data:read" {
		t.Fatalf("unexpected service credential: %#v", credential)
	}
	if got := credential.Secret.String(); got != "[REDACTED]" {
		t.Fatalf("service secret was not redacted: %q", got)
	}
}

func TestLoadSecurityRejectsMalformedServiceCredentials(t *testing.T) {
	for _, raw := range []string{
		`{}`,
		`[{"name":"quant","token_env":"QUANT_SERVICE_TOKEN","unknown":true}]`,
		`[{"name":"quant","token_env":"QUANT_SERVICE_TOKEN","scopes":[]}]`,
		`[]`,
		`[{"name":"quant","token_env":"QUANT_SERVICE_TOKEN","scopes":["data:read"]}] trailing`,
	} {
		t.Run(raw, func(t *testing.T) {
			env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
				switch key {
				case "PLATFORM_ADMIN_TOKEN":
					return strings.Repeat("a", 43), true
				case "PLATFORM_SERVICE_CREDENTIALS":
					return raw, true
				case "QUANT_SERVICE_TOKEN":
					return strings.Repeat("b", 43), true
				default:
					return "", false
				}
			})
			if _, err := LoadSecurity(env); err == nil {
				t.Fatal("LoadSecurity() error = nil, want invalid service credential configuration")
			}
		})
	}
}

func TestLoadSecurityRejectsMissingServiceSecret(t *testing.T) {
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		switch key {
		case "PLATFORM_ADMIN_TOKEN":
			return strings.Repeat("a", 43), true
		case "PLATFORM_SERVICE_CREDENTIALS":
			return `[{"name":"quant","token_env":"QUANT_SERVICE_TOKEN","scopes":["data:read"]}]`, true
		default:
			return "", false
		}
	})
	if _, err := LoadSecurity(env); err == nil || !strings.Contains(err.Error(), "QUANT_SERVICE_TOKEN") {
		t.Fatalf("LoadSecurity() error = %v, want missing service secret", err)
	}
}


func TestLoadServiceCredentialsDoesNotRequireOwnerToken(t *testing.T) {
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		switch key {
		case "PLATFORM_SERVICE_CREDENTIALS":
			return `[{"name":"researchflow-worker","token_env":"RESEARCHFLOW_WORKER_TOKEN","scopes":["data:read"]}]`, true
		case "RESEARCHFLOW_WORKER_TOKEN":
			return strings.Repeat("w", 43), true
		default:
			return "", false
		}
	})
	credentials, err := LoadServiceCredentials(env)
	if err != nil {
		t.Fatal(err)
	}
	if len(credentials) != 1 || credentials[0].Name != "researchflow-worker" ||
		!credentials[0].Secret.Configured() || len(credentials[0].Scopes) != 1 ||
		credentials[0].Scopes[0] != "data:read" {
		t.Fatalf("unexpected service credentials: %#v", credentials)
	}
}
