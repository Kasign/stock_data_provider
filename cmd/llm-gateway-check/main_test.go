package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/llm"
)

func probeConfig() (llm.ProviderConfig, llm.CapabilityState) {
	verifiedAt := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
	validUntil := verifiedAt.Add(2 * time.Hour)
	provider := llm.ProviderConfig{
		ProviderID: "openresty-gateway", Protocol: llm.ProtocolOpenAIResponses,
		BaseURL: "https://llm.internal.example/v1", DefaultModel: "qwen-local",
		APIKeySecretRef: "SERVER_LLM_API_KEY", RequestTimeoutMS: 30000,
		CapabilityTTLMS: int64(2 * time.Hour / time.Millisecond),
		AllowedModels:   []string{"qwen-local"},
		MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20,
	}
	capabilities := llm.CapabilityState{
		ProviderID: provider.ProviderID, ModelID: provider.DefaultModel,
		Protocol: llm.ProtocolOpenAIResponses, GatewayVersion: "openresty-v1",
		VerifiedAt: &verifiedAt, ValidUntil: &validUntil,
		VerificationMethod: "probe", EvidenceRef: "evidence://llm/gateway-probe",
		MaxOutputTokensLimit: 4096,
		Values: map[llm.Capability]llm.CapabilityStatus{
			llm.CapabilityResponsesJSON: llm.CapabilitySupported,
		},
	}
	return provider, capabilities
}

func completeProbeResponse() llm.ModelResponse {
	return llm.ModelResponse{
		Status: llm.ResponseCompleted, ProviderID: "openresty-gateway",
		ModelRequested: "qwen-local", ModelActual: "qwen-local-v2",
		ModelVersion: "2026-10", ResponseID: "resp-verify-1",
		GatewayRequestID: "gw-req-1", OutputText: "OK",
		SystemAsOf: time.Now().UTC(),
	}
}

func TestBuildProbeRequestIsValidAgainstConfiguredProviderAndCapabilities(t *testing.T) {
	provider, capabilities := probeConfig()
	now := time.Date(2026, 11, 27, 1, 0, 0, 0, time.UTC)

	request, err := buildProbeRequest(provider, capabilities, now)
	if err != nil {
		t.Fatalf("buildProbeRequest() error = %v", err)
	}
	if request.ProviderID != provider.ProviderID || request.Model != provider.DefaultModel {
		t.Fatalf("probe request identity = %s/%s", request.ProviderID, request.Model)
	}
	if err := request.ValidateAgainstProvider(provider); err != nil {
		t.Fatalf("probe request rejected by provider: %v", err)
	}
	if err := request.ValidateCapabilitiesAt(capabilities, now); err != nil {
		t.Fatalf("probe request rejected by capabilities: %v", err)
	}
	if len(request.Tools) != 0 || request.ResponseSchema != nil {
		t.Fatal("probe request must not request tools or structured output")
	}
	if request.MaxOutputTokens > capabilities.MaxOutputTokensLimit {
		t.Fatal("probe request exceeds verified capability limit")
	}
	if request.SystemAsOf.Location() != time.UTC || request.DeadlineAt.Location() != time.UTC {
		t.Fatal("probe request times must be UTC")
	}
}

func TestEvaluateProbePassesCompleteResponse(t *testing.T) {
	report := evaluateProbe(completeProbeResponse(), nil)
	if !report.Pass {
		t.Fatalf("complete probe response must pass: %+v", report)
	}
	if report.ModelActual != "qwen-local-v2" || report.ModelVersion != "2026-10" ||
		report.ResponseID != "resp-verify-1" || report.GatewayRequestID != "gw-req-1" {
		t.Fatalf("probe report lost identity fields: %+v", report)
	}
	if report.OutputChars != 2 {
		t.Fatalf("OutputChars = %d, want 2", report.OutputChars)
	}
}

func TestEvaluateProbeRejectsIncompleteIdentity(t *testing.T) {
	for _, field := range []string{"model_actual", "model_version", "response_id"} {
		response := completeProbeResponse()
		switch field {
		case "model_actual":
			response.ModelActual = ""
		case "model_version":
			response.ModelVersion = ""
		case "response_id":
			response.ResponseID = ""
		}
		report := evaluateProbe(response, nil)
		if report.Pass {
			t.Fatalf("missing %s must fail closed: %+v", field, report)
		}
		if report.Reason != probeReasonIdentityIncomplete {
			t.Fatalf("missing %s reason = %q, want %q", field, report.Reason, probeReasonIdentityIncomplete)
		}
	}
}

func TestEvaluateProbeRejectsNonCompletedStatus(t *testing.T) {
	response := completeProbeResponse()
	response.Status = llm.ResponseIncomplete
	report := evaluateProbe(response, nil)
	if report.Pass || report.Reason != probeReasonStatusNotCompleted {
		t.Fatalf("incomplete status must fail: %+v", report)
	}
}

func TestEvaluateProbeRejectsEmptyOutput(t *testing.T) {
	response := completeProbeResponse()
	response.OutputText = ""
	report := evaluateProbe(response, nil)
	if report.Pass || report.Reason != probeReasonEmptyOutput {
		t.Fatalf("empty output must fail: %+v", report)
	}
}

func TestEvaluateProbeCallErrorNeverLeaksSecret(t *testing.T) {
	const secret = "sk-super-secret-credential-must-never-print"
	callErr := errors.New("call failed with Bearer " + secret + " to https://llm.internal.example/v1/responses")
	report := evaluateProbe(llm.ModelResponse{}, callErr)
	if report.Pass {
		t.Fatal("call error must fail")
	}
	if strings.Contains(report.Reason, secret) || strings.Contains(report.Reason, "Bearer") ||
		strings.Contains(report.Reason, "llm.internal.example") {
		t.Fatalf("call error leaked credential or endpoint into report: %q", report.Reason)
	}
	if report.Reason != probeReasonCallFailed {
		t.Fatalf("call error reason = %q, want %q", report.Reason, probeReasonCallFailed)
	}
}

func TestEvaluateProbeUsesSanitizedAdapterErrorMessage(t *testing.T) {
	callErr := &llm.AdapterError{Info: &llm.ModelErrorInfo{
		Kind: llm.ErrorUnauthorized, StableCode: "llm.gateway.unauthorized",
		SafeMessage: "gateway rejected the caller credential",
	}}
	report := evaluateProbe(llm.ModelResponse{}, callErr)
	if report.Pass || report.Reason != "gateway rejected the caller credential" {
		t.Fatalf("adapter safe message not used: %+v", report)
	}
}
