package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

func env(values map[string]string) platformconfig.Env {
	return platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
}

func TestLoadLeavesOptionalProvidersUnconfigured(t *testing.T) {
	got, err := Load(env(nil))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	for name, provider := range map[string]ProviderConfig{
		"bocha":   got.Bocha(),
		"tavily":  got.Tavily(),
		"brave":   got.Brave(),
		"searxng": got.SearXNG(),
	} {
		if provider.Configured() {
			t.Errorf("%s unexpectedly configured", name)
		}
	}
}

func TestLoadCredentialProvidersAsPairedSettings(t *testing.T) {
	const apiKey = "secret-provider-key"

	got, err := Load(env(map[string]string{
		BochaEndpointEnv:   "https://bocha.example.com/v1/search",
		BochaAPIKeyEnv:     apiKey,
		TavilyEndpointEnv:  "https://api.tavily.example/search",
		TavilyAPIKeyEnv:    "tavily-secret",
		BraveEndpointEnv:   "https://api.brave.example/news",
		BraveAPIKeyEnv:     "brave-secret",
		SearXNGEndpointEnv: "http://search.internal.example",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !got.Bocha().Configured() || got.Bocha().Endpoint() != "https://bocha.example.com/v1/search" {
		t.Fatalf(
			"unexpected Bocha config: configured=%v endpoint=%q",
			got.Bocha().Configured(),
			got.Bocha().Endpoint(),
		)
	}
	if got.Bocha().APIKey().Value() != apiKey {
		t.Fatal("Bocha API key was not preserved in Secret")
	}
	if !got.Tavily().Configured() || !got.Brave().Configured() || !got.SearXNG().Configured() {
		t.Fatal("configured providers were not loaded")
	}
	if got.SearXNG().APIKey().Configured() {
		t.Fatal("SearXNG must not require an API key")
	}
}

func TestLoadRejectsIncompleteCredentialPairsWithoutLeakingValues(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
	}{
		{
			name:   "endpoint without key",
			values: map[string]string{BochaEndpointEnv: "https://provider.example/path"},
		},
		{
			name:   "key without endpoint",
			values: map[string]string{BochaAPIKeyEnv: "do-not-leak-this-secret"},
		},
		{
			name: "whitespace key",
			values: map[string]string{
				BochaEndpointEnv: "https://provider.example/path",
				BochaAPIKeyEnv:   "   ",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load(env(test.values))
			if err == nil {
				t.Fatal("Load() error = nil, want incomplete-pair error")
			}
			if !strings.Contains(err.Error(), BochaEndpointEnv) ||
				!strings.Contains(err.Error(), BochaAPIKeyEnv) {
				t.Fatalf("error does not identify configuration keys: %v", err)
			}
			if strings.Contains(err.Error(), "do-not-leak-this-secret") ||
				strings.Contains(err.Error(), "provider.example") {
				t.Fatalf("error leaked configuration value: %v", err)
			}
		})
	}
}

func TestLoadRejectsUnsafeOrMalformedEndpoints(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
	}{
		{name: "non HTTP scheme", endpoint: "file:///etc/passwd"},
		{name: "userinfo", endpoint: "https://user:password@provider.example/path"},
		{name: "query", endpoint: "https://provider.example/path?token=secret"},
		{name: "fragment", endpoint: "https://provider.example/path#fragment"},
		{name: "invalid port", endpoint: "https://provider.example:70000/path"},
		{name: "empty port", endpoint: "https://provider.example:/path"},
		{name: "relative URL", endpoint: "/provider/search"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load(env(map[string]string{SearXNGEndpointEnv: test.endpoint}))
			if err == nil {
				t.Fatal("Load() error = nil, want endpoint validation error")
			}
			if strings.Contains(err.Error(), test.endpoint) {
				t.Fatalf("error leaked endpoint value: %v", err)
			}
		})
	}
}

func TestProviderSecretFormattingAndJSONAreRedacted(t *testing.T) {
	got, err := Load(env(map[string]string{
		BochaEndpointEnv: "https://bocha.example.com/search",
		BochaAPIKeyEnv:   "sensitive-value",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	secret := got.Bocha().APIKey()
	for _, rendered := range []string{
		fmt.Sprintf("%v", secret),
		fmt.Sprintf("%#v", secret),
	} {
		if strings.Contains(rendered, "sensitive-value") || !strings.Contains(rendered, "[REDACTED]") {
			t.Fatalf("secret formatting was not redacted: %q", rendered)
		}
	}

	encoded, err := json.Marshal(secret)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(encoded), "sensitive-value") || string(encoded) != "\"[REDACTED]\"" {
		t.Fatalf("secret JSON was not redacted: %s", encoded)
	}
}

func TestLoadEvidenceCursorKeyRingIsOptionalAndDefensivelyCopied(t *testing.T) {
	if _, _, configured := mustLoad(t, nil).EvidenceCursorKeyRing(); configured {
		t.Fatal("cursor key ring should remain optional when not configured")
	}
	active := "current-2026-11"
	values := map[string]string{
		EvidenceCursorActiveKeyIDEnv: active,
		EvidenceCursorKeyRingEnv:     `{"current-2026-11":"01234567890123456789012345678901","previous-2026-10":"abcdefghijklmnopqrstuvwxyzABCDEF"}`,
	}
	got := mustLoad(t, values)
	activeID, keys, configured := got.EvidenceCursorKeyRing()
	if !configured || activeID != active || len(keys) != 2 {
		t.Fatalf("unexpected cursor key ring: active=%q keys=%d configured=%v", activeID, len(keys), configured)
	}
	if keys[active].Value() != "01234567890123456789012345678901" {
		t.Fatal("active cursor key was not preserved")
	}
	delete(keys, active)
	_, copiedAgain, configured := got.EvidenceCursorKeyRing()
	if !configured || len(copiedAgain) != 2 {
		t.Fatal("caller mutation changed stored cursor key ring")
	}
}

func TestLoadEvidenceCursorKeyRingRejectsInvalidConfigurationWithoutLeakingSecrets(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
	}{
		{name: "active key id without key ring", values: map[string]string{EvidenceCursorActiveKeyIDEnv: "current"}},
		{name: "key ring without active key id", values: map[string]string{EvidenceCursorKeyRingEnv: `{"current":"01234567890123456789012345678901"}`}},
		{name: "invalid JSON", values: map[string]string{EvidenceCursorActiveKeyIDEnv: "current", EvidenceCursorKeyRingEnv: "not-json"}},
		{name: "active key missing", values: map[string]string{EvidenceCursorActiveKeyIDEnv: "missing", EvidenceCursorKeyRingEnv: `{"current":"01234567890123456789012345678901"}`}},
		{name: "short secret", values: map[string]string{EvidenceCursorActiveKeyIDEnv: "current", EvidenceCursorKeyRingEnv: `{"current":"short"}`}},
		{name: "invalid key id", values: map[string]string{EvidenceCursorActiveKeyIDEnv: "bad/key", EvidenceCursorKeyRingEnv: `{"bad/key":"01234567890123456789012345678901"}`}},
		{name: "too many keys", values: map[string]string{EvidenceCursorActiveKeyIDEnv: "k1", EvidenceCursorKeyRingEnv: `{"k1":"01234567890123456789012345678901","k2":"01234567890123456789012345678901","k3":"01234567890123456789012345678901","k4":"01234567890123456789012345678901","k5":"01234567890123456789012345678901","k6":"01234567890123456789012345678901","k7":"01234567890123456789012345678901","k8":"01234567890123456789012345678901","k9":"01234567890123456789012345678901"}`}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load(env(test.values))
			if err == nil {
				t.Fatal("Load() error = nil, want invalid cursor key ring error")
			}
			if strings.Contains(err.Error(), "01234567890123456789012345678901") ||
				strings.Contains(err.Error(), "not-json") {
				t.Fatalf("error leaked cursor key material: %v", err)
			}
		})
	}
}

func mustLoad(t *testing.T, values map[string]string) Config {
	t.Helper()
	got, err := Load(env(values))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return got
}
