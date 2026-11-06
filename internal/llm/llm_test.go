package llm

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testProviderConfig() ProviderConfig {
	return ProviderConfig{
		ProviderID: "local-gateway", Protocol: ProtocolOpenAIResponses,
		BaseURL: "https://gateway.example.com:8443/v1", DefaultModel: "qwen-local",
		APIKeySecretRef: "secret://llm/gateway", RequestTimeout: 30 * time.Second,
		MaxRequestBytes: 1 << 20, MaxResponseBytes: 2 << 20,
	}
}

func TestProviderConfigValidate(t *testing.T) {
	c := testProviderConfig()
	if err := c.Validate(); err != nil { t.Fatalf("valid config rejected: %v", err) }
	c.BaseURL = "http://gateway.example.com/v1"
	if err := c.Validate(); err == nil { t.Fatal("expected HTTP URL rejection") }
	c = testProviderConfig()
	c.BaseURL = "https://gateway.example.com/v1?key=secret"
	if err := c.Validate(); err == nil { t.Fatal("expected query string rejection") }
	c = testProviderConfig()
	c.APIKeySecretRef = ""
	if err := c.Validate(); err == nil { t.Fatal("expected missing SecretRef rejection") }
}

func TestCapabilityUnknownByDefaultAndRequiresVersionedEvidence(t *testing.T) {
	s := CapabilityState{Values: map[Capability]CapabilityStatus{CapabilityResponsesJSON: CapabilitySupported}}
	if s.Status(CapabilityResponsesStream) != CapabilityUnknown { t.Fatal("missing capability must be unknown") }
	if s.Supports(CapabilityResponsesJSON) { t.Fatal("unversioned capability evidence must not be trusted") }
	now := time.Now().UTC()
	s.ProviderID, s.ModelID, s.GatewayVersion, s.VerifiedAt = "p1", "m1", "gateway-v1", &now
	if !s.Supports(CapabilityResponsesJSON) { t.Fatal("complete evidence should enable capability") }
}

func TestModelRequestValidationAndCapabilityGate(t *testing.T) {
	now := time.Now().UTC()
	temperature := 0.2
	r := ModelRequest{
		RequestID: "req-1", PurposeCode: "research.summary", Model: "qwen-local",
		InputText: "Evidence", MaxOutputTokens: 128, Temperature: &temperature,
		DeadlineAt: now.Add(time.Minute), CorrelationID: "run-1", SystemAsOf: now,
	}
	if err := r.Validate(); err != nil { t.Fatalf("valid request rejected: %v", err) }
	if err := r.ValidateCapabilities(CapabilityState{}); err == nil || !strings.Contains(err.Error(), "unsupported_capability") {
		t.Fatalf("expected unknown capability rejection, got %v", err)
	}
	r.InputText = " "
	if err := r.Validate(); err == nil { t.Fatal("empty input must be rejected") }
}

func TestUsageMissingValuesRemainNull(t *testing.T) {
	b, err := json.Marshal(Usage{Source: UsageSourceUnknown})
	if err != nil { t.Fatal(err) }
	if strings.Contains(string(b), "input_tokens") || strings.Contains(string(b), "total_tokens") {
		t.Fatalf("missing usage must not serialize as zero: %s", b)
	}
}

func TestSchemaAndToolJSONValidation(t *testing.T) {
	now := time.Now().UTC()
	r := ModelRequest{
		RequestID: "req-1", PurposeCode: "research.summary", Model: "qwen-local",
		InputText: "Evidence", MaxOutputTokens: 128, DeadlineAt: now.Add(time.Minute),
		CorrelationID: "run-1", SystemAsOf: now,
		ResponseSchema: &JSONSchemaSpec{Name: "summary", SchemaJSON: json.RawMessage("{\"type\":\"object\"}")},
		Tools: []ToolDefinition{{Name: "lookup_quote", ParametersJSON: json.RawMessage("{\"type\":\"object\"}")}},
	}
	if err := r.Validate(); err != nil { t.Fatalf("valid schema/tool rejected: %v", err) }
	r.ResponseSchema.SchemaJSON = json.RawMessage("{\"type\":")
	if err := r.Validate(); err == nil { t.Fatal("invalid schema JSON must be rejected") }
}
