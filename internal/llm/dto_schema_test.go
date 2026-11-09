package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func readLLMContractSchema(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "doc", "llm", "schema", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read LLM contract schema %s: %v", name, err)
	}
	return data
}

func assertLLMContractSchema(t *testing.T, name string, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s contract value: %v", name, err)
	}
	if err := ValidateJSONAgainstSchema(readLLMContractSchema(t, name), encoded); err != nil {
		t.Fatalf("%s does not satisfy its published schema: %v\n%s", name, err, encoded)
	}
}

func TestLLMPublicDTOJSONSchemas(t *testing.T) {
	request := validResponsesRequest()
	assertLLMContractSchema(t, "model-request-1.0.0.schema.json", request)

	provider := ProviderConfig{
		ProviderID: "provider-test", Protocol: ProtocolOpenAIResponses,
		BaseURL: "https://gateway.example/v1", DefaultModel: "qwen-local",
		APIKeySecretRef: "secret://llm/provider-test", RequestTimeoutMS: 30000, CapabilityTTLMS: 24 * 60 * 60 * 1000,
		AllowedModels: []string{"qwen-local"}, MaxRequestBytes: 1 << 20,
		MaxResponseBytes: MaxPayloadBytes, StreamEnabled: true,
	}
	assertLLMContractSchema(t, "provider-config-1.0.0.schema.json", provider)

	verifiedAt := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	validUntil := verifiedAt.Add(time.Hour)
	capability := CapabilityState{
		ProviderID: provider.ProviderID, ModelID: "qwen-local",
		Protocol: ProtocolOpenAIResponses, GatewayVersion: "gateway-test",
		VerifiedAt: &verifiedAt, ValidUntil: &validUntil, VerificationMethod: "automated_e2e", EvidenceRef: "test:evidence/1",
		MaxOutputTokensLimit: 4096,
		Values: map[Capability]CapabilityStatus{
			CapabilityResponsesJSON:   CapabilitySupported,
			CapabilityResponsesStream: CapabilityUnknown,
		},
	}
	assertLLMContractSchema(t, "capability-state-1.0.0.schema.json", capability)

	modelError := ModelErrorInfo{
		Kind: ErrorUpstream, StableCode: "llm.provider.request_failed",
		Retryable: true, SafeMessage: "model provider request failed",
	}
	assertLLMContractSchema(t, "model-error-info-1.0.0.schema.json", modelError)
}

func TestLLMResponseAndEventGoldenDTOsSatisfyPublishedSchemas(t *testing.T) {
	jsonRequest := validResponsesRequest()
	jsonFixtures := []string{
		"response_completed_text.json",
		"response_completed_function_call.json",
		"response_incomplete_max_output_tokens.json",
		"response_usage_inconsistent_total.json",
	}
	for _, fixture := range jsonFixtures {
		t.Run("json/"+fixture, func(t *testing.T) {
			response, err := ParseResponsesResponse(readLLMFixture(t, "json/"+fixture), jsonRequest)
			if err != nil {
				t.Fatalf("parse JSON fixture: %v", err)
			}
			assertLLMContractSchema(t, "model-response-1.0.0.schema.json", response)
		})
	}

	sseRequest := validSSERequest()
	clock := sseTestClock{now: sseRequest.SystemAsOf.Add(time.Second)}
	sseFixtures := []string{
		"stream_completed_text.sse",
		"stream_function_call.sse",
		"stream_incomplete.sse",
	}
	for _, fixture := range sseFixtures {
		t.Run("sse/"+fixture, func(t *testing.T) {
			var firstEvent *ModelEvent
			response, err := ParseResponsesSSE(readLLMFixture(t, "sse/"+fixture), sseRequest, clock, func(event ModelEvent) error {
				if firstEvent == nil {
					copy := event
					firstEvent = &copy
				}
				return nil
			})
			if err != nil {
				t.Fatalf("parse SSE fixture: %v", err)
			}
			assertLLMContractSchema(t, "model-response-1.0.0.schema.json", response)
			if firstEvent == nil {
				t.Fatal("SSE fixture did not emit an event")
			}
			assertLLMContractSchema(t, "model-event-1.0.0.schema.json", *firstEvent)
		})
	}
}

func TestLLMPublicDTOJSONSchemasRejectInvalidValues(t *testing.T) {
	request := validResponsesRequest()
	request.RequestID = ""
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateJSONAgainstSchema(readLLMContractSchema(t, "model-request-1.0.0.schema.json"), encoded); err == nil {
		t.Fatal("schema must reject ModelRequest without request_id")
	}

	invalidError := ModelErrorInfo{
		Kind: ModelErrorKind("secret_internal_kind"), StableCode: "llm.test",
		Retryable: false, SafeMessage: "safe",
	}
	encoded, err = json.Marshal(invalidError)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateJSONAgainstSchema(readLLMContractSchema(t, "model-error-info-1.0.0.schema.json"), encoded); err == nil {
		t.Fatal("schema must reject unknown public ModelErrorKind")
	}
}
