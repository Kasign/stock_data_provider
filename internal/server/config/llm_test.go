package config

import (
	"encoding/json"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/llm"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

func TestLoadLLMReturnsDisabledWithoutConfiguration(t *testing.T) {
	cfg, err := LoadLLM(platformconfig.NewEnvWithLookup(func(string) (string, bool) {
		return "", false
	}))
	if err != nil {
		t.Fatalf("LoadLLM() error = %v", err)
	}
	if cfg.Configured {
		t.Fatal("LLM must stay disabled when no provider/capability configuration exists")
	}
}

func TestLoadLLMValidatesConfiguredProviderAndCapabilityEvidence(t *testing.T) {
	now := time.Now().UTC()
	verifiedAt := now.Add(-time.Minute)
	validUntil := now.Add(time.Hour)
	provider := llm.ProviderConfig{
		ProviderID:       "provider-test",
		Protocol:         llm.ProtocolOpenAIResponses,
		BaseURL:          "https://gateway.example.test/v1",
		DefaultModel:     "model-test",
		APIKeySecretRef:  "TEST_LLM_API_KEY_SECRET_REF",
		RequestTimeoutMS: 15000,
		CapabilityTTLMS:  int64(2 * time.Hour / time.Millisecond),
		AllowedModels:    []string{"model-test"},
		MaxRequestBytes:  1 << 20,
		MaxResponseBytes: 1 << 20,
		StreamEnabled:    false,
	}
	capabilities := llm.CapabilityState{
		ProviderID:           provider.ProviderID,
		ModelID:              provider.DefaultModel,
		Protocol:             provider.Protocol,
		GatewayVersion:       "gateway-v1",
		VerifiedAt:           &verifiedAt,
		ValidUntil:           &validUntil,
		VerificationMethod:   "probe",
		EvidenceRef:          "evidence://llm/provider-test/model-test",
		MaxOutputTokensLimit: 2048,
		Values: map[llm.Capability]llm.CapabilityStatus{
			llm.CapabilityResponsesJSON: llm.CapabilitySupported,
		},
	}
	providerJSON, err := json.Marshal(provider)
	if err != nil {
		t.Fatal(err)
	}
	capabilityJSON, err := json.Marshal(capabilities)
	if err != nil {
		t.Fatal(err)
	}
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		switch key {
		case "SERVER_LLM_PROVIDER_JSON":
			return string(providerJSON), true
		case "SERVER_LLM_CAPABILITY_JSON":
			return string(capabilityJSON), true
		case provider.APIKeySecretRef:
			return "test-only-credential", true
		default:
			return "", false
		}
	})

	cfg, err := LoadLLM(env)
	if err != nil {
		t.Fatalf("LoadLLM() error = %v", err)
	}
	if !cfg.Configured {
		t.Fatal("LLM configuration must be reported as configured")
	}
	if cfg.Provider.ProviderID != provider.ProviderID || cfg.Capabilities.ModelID != provider.DefaultModel {
		t.Fatalf("unexpected LLM identity: provider=%s model=%s", cfg.Provider.ProviderID, cfg.Capabilities.ModelID)
	}
	if !cfg.APIKey.Configured() {
		t.Fatal("LLM API key secret reference was not resolved")
	}
	if cfg.Policy.Dependency != "llm.provider" || len(cfg.Policy.AllowedHosts) != 1 ||
		cfg.Policy.AllowedHosts[0] != "gateway.example.test" ||
		len(cfg.Policy.AllowedPorts) != 0 || cfg.Policy.AllowHTTP || cfg.Policy.AllowRedirects ||
		cfg.Policy.MaxRedirects != 0 || cfg.Policy.Idempotent {
		t.Fatalf("unexpected LLM destination policy: %+v", cfg.Policy)
	}
}

func TestLoadLLMRejectsPartialConfiguration(t *testing.T) {
	now := time.Now().UTC()
	capabilities := llm.CapabilityState{
		ProviderID:           "provider-test",
		ModelID:              "model-test",
		Protocol:             llm.ProtocolOpenAIResponses,
		GatewayVersion:       "gateway-v1",
		VerifiedAt:           &now,
		ValidUntil:           func() *time.Time { value := now.Add(time.Hour); return &value }(),
		VerificationMethod:   "probe",
		EvidenceRef:          "evidence://llm/provider-test/model-test",
		MaxOutputTokensLimit: 2048,
		Values: map[llm.Capability]llm.CapabilityStatus{
			llm.CapabilityResponsesJSON: llm.CapabilitySupported,
		},
	}
	capabilityJSON, err := json.Marshal(capabilities)
	if err != nil {
		t.Fatal(err)
	}
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		if key == "SERVER_LLM_CAPABILITY_JSON" {
			return string(capabilityJSON), true
		}
		return "", false
	})
	if _, err := LoadLLM(env); err == nil {
		t.Fatal("capability configuration without provider configuration must fail closed")
	}
}

func TestLoadLLMRejectsMissingSecretReference(t *testing.T) {
	provider := llm.ProviderConfig{
		ProviderID:       "provider-test",
		Protocol:         llm.ProtocolOpenAIResponses,
		BaseURL:          "https://gateway.example.test/v1",
		DefaultModel:     "model-test",
		APIKeySecretRef:  "MISSING_TEST_LLM_SECRET_REF",
		RequestTimeoutMS: 15000,
		CapabilityTTLMS:  int64(2 * time.Hour / time.Millisecond),
		AllowedModels:    []string{"model-test"},
		MaxRequestBytes:  1 << 20,
		MaxResponseBytes: 1 << 20,
	}
	providerJSON, err := json.Marshal(provider)
	if err != nil {
		t.Fatal(err)
	}
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		if key == "SERVER_LLM_PROVIDER_JSON" {
			return string(providerJSON), true
		}
		return "", false
	})
	if _, err := LoadLLM(env); err == nil {
		t.Fatal("missing capability evidence must fail closed")
	}
}

func TestLoadLLMRejectsCapabilityProviderMismatch(t *testing.T) {
	now := time.Now().UTC()
	validUntil := now.Add(time.Hour)
	provider := llm.ProviderConfig{
		ProviderID:       "provider-test",
		Protocol:         llm.ProtocolOpenAIResponses,
		BaseURL:          "https://gateway.example.test/v1",
		DefaultModel:     "model-test",
		APIKeySecretRef:  "TEST_LLM_API_KEY_SECRET_REF",
		RequestTimeoutMS: 15000,
		CapabilityTTLMS:  int64(2 * time.Hour / time.Millisecond),
		AllowedModels:    []string{"model-test"},
		MaxRequestBytes:  1 << 20,
		MaxResponseBytes: 1 << 20,
	}
	capabilities := llm.CapabilityState{
		ProviderID:           "provider-other",
		ModelID:              provider.DefaultModel,
		Protocol:             provider.Protocol,
		GatewayVersion:       "gateway-v1",
		VerifiedAt:           &now,
		ValidUntil:           &validUntil,
		VerificationMethod:   "probe",
		EvidenceRef:          "evidence://llm/provider-other/model-test",
		MaxOutputTokensLimit: 2048,
		Values: map[llm.Capability]llm.CapabilityStatus{
			llm.CapabilityResponsesJSON: llm.CapabilitySupported,
		},
	}
	providerJSON, err := json.Marshal(provider)
	if err != nil {
		t.Fatal(err)
	}
	capabilityJSON, err := json.Marshal(capabilities)
	if err != nil {
		t.Fatal(err)
	}
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		switch key {
		case "SERVER_LLM_PROVIDER_JSON":
			return string(providerJSON), true
		case "SERVER_LLM_CAPABILITY_JSON":
			return string(capabilityJSON), true
		case provider.APIKeySecretRef:
			return "test-only-credential", true
		default:
			return "", false
		}
	})
	if _, err := LoadLLM(env); err == nil {
		t.Fatal("capability evidence bound to another provider must fail closed")
	}
}
