package llm

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/platform/fault"
)

func testProviderConfig() ProviderConfig {
	return ProviderConfig{
		ProviderID: "local-gateway", Protocol: ProtocolOpenAIResponses,
		BaseURL: "https://gateway.example.com:8443/v1", DefaultModel: "qwen-local",
		APIKeySecretRef: "secret://llm/gateway", RequestTimeoutMS: 30000, CapabilityTTLMS: 24 * 60 * 60 * 1000, AllowedModels: []string{"qwen-local"},
		MaxRequestBytes: 1 << 20, MaxResponseBytes: 2 << 20,
	}
}


type fixedLLMTestClock struct {
	now time.Time
}

func (c fixedLLMTestClock) Now() time.Time {
	return c.now
}

func TestModelRequestAndCapabilityValidationUseInjectedClock(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	clock := fixedLLMTestClock{now: now}
	request := ModelRequest{
		ProviderID: "p1", RequestID: "req-1", PurposeCode: "research.summary",
		Model: "m1", InputText: "input", MaxOutputTokens: 128,
		DeadlineAt: now.Add(time.Minute), CorrelationID: "run-1", SystemAsOf: now,
	}
	if err := request.ValidateAt(clock.Now()); err != nil {
		t.Fatalf("request valid at injected clock was rejected: %v", err)
	}
	request.DeadlineAt = now
	if err := request.ValidateAt(clock.Now()); err != nil {
		t.Fatalf("deadline validity is enforced at dispatch, not DTO shape validation: %v", err)
	}

	verifiedAt := now.Add(-time.Minute)
	validUntil := now.Add(time.Minute)
	state := CapabilityState{
		ProviderID: "p1", ModelID: "m1", Protocol: ProtocolOpenAIResponses,
		GatewayVersion: "gateway-v1", VerifiedAt: &verifiedAt, ValidUntil: &validUntil,
		VerificationMethod: "automated_e2e", EvidenceRef: "fixture://capability", MaxOutputTokensLimit: 256,
		Values: map[Capability]CapabilityStatus{CapabilityResponsesJSON: CapabilitySupported},
	}
	request.DeadlineAt = now.Add(time.Minute)
	if err := request.ValidateCapabilitiesAt(state, clock.Now()); err != nil {
		t.Fatalf("capability valid at injected clock was rejected: %v", err)
	}
	expiredClock := fixedLLMTestClock{now: validUntil}
	if err := request.ValidateCapabilitiesAt(state, expiredClock.Now()); err == nil {
		t.Fatal("capability at valid_until must be expired")
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
	validUntil := now.Add(time.Hour)
	s.ProviderID, s.ModelID, s.Protocol, s.GatewayVersion, s.VerifiedAt = "p1", "m1", ProtocolOpenAIResponses, "gateway-v1", &now
	s.ValidUntil, s.VerificationMethod, s.EvidenceRef = &validUntil, "automated_e2e", "test-evidence"
	if !s.Supports(CapabilityResponsesJSON) { t.Fatal("complete evidence should enable capability") }

	futureVerifiedAt := time.Now().UTC().Add(time.Minute)
	futureValidUntil := futureVerifiedAt.Add(time.Hour)
	s.VerifiedAt, s.ValidUntil = &futureVerifiedAt, &futureValidUntil
	if s.Supports(CapabilityResponsesJSON) {
		t.Fatal("future verification evidence must not enable capability")
	}
}


func TestCapabilityEvidenceRequiresMethodAndProviderTTL(t *testing.T) {
	config := testProviderConfig()
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	verifiedAt := now.Add(-time.Minute)
	validUntil := verifiedAt.Add(time.Duration(config.CapabilityTTLMS)*time.Millisecond)
	state := CapabilityState{
		ProviderID: config.ProviderID, ModelID: config.DefaultModel,
		Protocol: config.Protocol, GatewayVersion: "gateway-v1",
		VerifiedAt: &verifiedAt, ValidUntil: &validUntil,
		VerificationMethod: "automated_e2e", EvidenceRef: "test:evidence",
		MaxOutputTokensLimit: 1024,
		Values: map[Capability]CapabilityStatus{CapabilityResponsesJSON: CapabilitySupported},
	}
	if err := config.ValidateCapabilityStateAt(state, now); err != nil {
		t.Fatalf("evidence within configured TTL rejected: %v", err)
	}
	tooLong := state
	tooLongValidUntil := validUntil.Add(time.Nanosecond)
	tooLong.ValidUntil = &tooLongValidUntil
	if err := config.ValidateCapabilityStateAt(tooLong, now); err == nil {
		t.Fatal("evidence validity beyond ProviderConfig capability_ttl_ms must be rejected")
	}
	missingMethod := state
	missingMethod.VerificationMethod = ""
	if missingMethod.SupportsAt(CapabilityResponsesJSON, now) {
		t.Fatal("capability without verification method must not be supported")
	}
	if err := config.ValidateCapabilityStateAt(missingMethod, now); err == nil {
		t.Fatal("provider validation must reject missing verification method")
	}
	config.CapabilityTTLMS = 0
	if err := config.Validate(); err == nil {
		t.Fatal("provider capability TTL must be explicitly positive")
	}
}

func TestProviderStreamSwitchRequiresCurrentModelEvidence(t *testing.T) {
	config := testProviderConfig()
	now := time.Now().UTC()
	validUntil := now.Add(time.Hour)
	state := CapabilityState{
		ProviderID: config.ProviderID, ModelID: config.DefaultModel, Protocol: config.Protocol,
		GatewayVersion: "gateway-v1", VerifiedAt: &now, ValidUntil: &validUntil,
		VerificationMethod: "automated_e2e", EvidenceRef: "test-evidence", MaxOutputTokensLimit: 1024,
		Values: map[Capability]CapabilityStatus{CapabilityResponsesJSON: CapabilitySupported},
	}
	if err := config.ValidateCapabilityState(state); err != nil {
		t.Fatalf("JSON-only provider evidence rejected: %v", err)
	}
	invalidLimitState := state
	invalidLimitState.MaxOutputTokensLimit = 0
	if err := config.ValidateCapabilityState(invalidLimitState); err == nil {
		t.Fatal("provider capability state must require a positive output token limit")
	}
	config.StreamEnabled = true
	if err := config.ValidateCapabilityState(state); err == nil {
		t.Fatal("stream_enabled must require current SSE evidence")
	}
	state.Values[CapabilityResponsesStream] = CapabilitySupported
	if err := config.ValidateCapabilityState(state); err != nil {
		t.Fatalf("verified stream evidence rejected: %v", err)
	}
}

func TestModelRequestValidationAndCapabilityGate(t *testing.T) {
	now := time.Now().UTC()
	temperature := 0.2
	r := ModelRequest{
		ProviderID: "gateway-1", RequestID: "req-1", PurposeCode: "research.summary", Model: "qwen-local",
		InputText: "Evidence", MaxOutputTokens: 128, Temperature: &temperature,
		DeadlineAt: now.Add(time.Minute), CorrelationID: "run-1", SystemAsOf: now,
	}
	if err := r.Validate(); err != nil { t.Fatalf("valid request rejected: %v", err) }
	if err := r.ValidateCapabilities(CapabilityState{}); err == nil || !strings.Contains(err.Error(), "capability evidence") {
		t.Fatalf("expected missing capability evidence rejection, got %v", err)
	}
	verifiedAt := now
	validUntil := now.Add(time.Hour)
	state := CapabilityState{
		ProviderID: r.ProviderID, ModelID: r.Model, Protocol: ProtocolOpenAIResponses,
		GatewayVersion: "gateway-v1", VerifiedAt: &verifiedAt, ValidUntil: &validUntil,
		VerificationMethod: "automated_e2e", EvidenceRef: "test-evidence", MaxOutputTokensLimit: 512,
		Values: map[Capability]CapabilityStatus{
			CapabilityResponsesJSON: CapabilitySupported,
			CapabilitySamplingParameters: CapabilityUnknown,
		},
	}
	if err := r.ValidateCapabilities(state); err == nil || !strings.Contains(err.Error(), "sampling_parameters") {
		t.Fatalf("expected sampling capability rejection, got %v", err)
	}
	state.Values[CapabilitySamplingParameters] = CapabilitySupported
	if err := r.ValidateCapabilities(state); err != nil { t.Fatalf("verified capabilities rejected: %v", err) }
	if err := r.ValidateStreamCapabilities(state); err == nil || !strings.Contains(err.Error(), "responses_stream") {
		t.Fatalf("expected unverified stream capability rejection, got %v", err)
	}
	state.Values[CapabilityResponsesStream] = CapabilitySupported
	if err := r.ValidateStreamCapabilities(state); err != nil { t.Fatalf("verified stream capability rejected: %v", err) }
	r.InputText = " "
	if err := r.Validate(); err == nil { t.Fatal("empty input must be rejected") }
}

func TestProviderModelAllowlistAndLocalSchemaMode(t *testing.T) {
	config := testProviderConfig()
	r := ModelRequest{ProviderID: config.ProviderID, Model: "qwen-local"}
	if err := r.ValidateAgainstProvider(config); err != nil { t.Fatalf("allowed provider/model rejected: %v", err) }
	r.Model = "other-model"
	if err := r.ValidateAgainstProvider(config); err == nil { t.Fatal("model outside allowlist must be rejected") }
}

func TestLocalValidationOnlyDoesNotRequireUpstreamStructuredOutput(t *testing.T) {
	now := time.Now().UTC()
	validUntil := now.Add(time.Hour)
	r := ModelRequest{
		ProviderID: "gateway-1", RequestID: "req-1", PurposeCode: "research.summary", Model: "qwen-local",
		InputText: "Evidence", MaxOutputTokens: 128, DeadlineAt: now.Add(time.Minute),
		CorrelationID: "run-1", SystemAsOf: now, LocalValidationOnly: true,
		ResponseSchema: &JSONSchemaSpec{Name: "summary", SchemaJSON: json.RawMessage("{\"type\":\"object\"}")},
	}
	state := CapabilityState{
		ProviderID: r.ProviderID, ModelID: r.Model, Protocol: ProtocolOpenAIResponses,
		GatewayVersion: "gateway-v1", VerifiedAt: &now, ValidUntil: &validUntil,
		VerificationMethod: "automated_e2e", EvidenceRef: "test-evidence", MaxOutputTokensLimit: 256,
		Values: map[Capability]CapabilityStatus{CapabilityResponsesJSON: CapabilitySupported},
	}
	if err := r.ValidateCapabilities(state); err != nil {
		t.Fatalf("local-only schema validation should not require upstream structured output: %v", err)
	}
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
		ProviderID: "gateway-1", RequestID: "req-1", PurposeCode: "research.summary", Model: "qwen-local",
		InputText: "Evidence", MaxOutputTokens: 128, DeadlineAt: now.Add(time.Minute),
		CorrelationID: "run-1", SystemAsOf: now,
		ResponseSchema: &JSONSchemaSpec{Name: "summary", SchemaJSON: json.RawMessage("{\"type\":\"object\"}")},
		Tools: []ToolDefinition{{Name: "lookup_quote", ParametersJSON: json.RawMessage("{\"type\":\"object\"}")}},
	}
	if err := r.Validate(); err != nil { t.Fatalf("valid schema/tool rejected: %v", err) }
	r.ResponseSchema.SchemaJSON = json.RawMessage("{\"type\":")
	if err := r.Validate(); err == nil { t.Fatal("invalid schema JSON must be rejected") }
}

func TestModelErrorFromPlatformFault(t *testing.T) {
	tests := []struct {
		name string
		class fault.Class
		want ModelErrorKind
		retryable bool
	}{
		{name: "validation", class: fault.ClassValidation, want: ErrorInvalidRequest},
		{name: "unauthenticated", class: fault.ClassUnauthenticated, want: ErrorUnauthorized},
		{name: "forbidden", class: fault.ClassForbidden, want: ErrorUnauthorized},
		{name: "unavailable", class: fault.ClassUnavailable, want: ErrorUpstreamUnavailable, retryable: true},
		{name: "timeout", class: fault.ClassTimeout, want: ErrorTimeout, retryable: true},
		{name: "not_found", class: fault.ClassNotFound, want: ErrorUpstream},
		{name: "conflict", class: fault.ClassConflict, want: ErrorUpstream},
		{name: "internal", class: fault.ClassInternal, want: ErrorUpstream},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &fault.Fault{Class: tt.class, Code: "platform.test." + string(tt.class), SafeMessage: "safe summary", Retryable: tt.retryable}
			got := ModelErrorFromPlatformFault(err)
			if got == nil {
				t.Fatal("expected mapped model error")
			}
			if got.Kind != tt.want || got.Retryable != tt.retryable {
				t.Fatalf("unexpected mapping: %+v", got)
			}
			if got.StableCode != "platform.test."+string(tt.class) || got.SafeMessage != "safe summary" {
				t.Fatalf("stable code or safe message lost: %+v", got)
			}
		})
	}
}


func TestModelErrorFromPlatformFaultUsesStableFallbacks(t *testing.T) {
	got := ModelErrorFromPlatformFault(&fault.Fault{Class: fault.ClassConflict})
	if got == nil || got.Kind != ErrorUpstream ||
		got.StableCode != "platform.fault.conflict" ||
		got.SafeMessage != "model provider request failed" {
		t.Fatalf("missing code/message must use stable safe fallbacks: %+v", got)
	}
}

func TestModelErrorFromPlatformFaultDoesNotExposeDependencyOrCause(t *testing.T) {
	secretCause := errors.New("authorization: Bearer secret-value")
	f := &fault.Fault{
		Class: fault.ClassUnavailable, Code: "platform.httpclient.request_failed",
		SafeMessage: "upstream request failed", Dependency: "llm.provider.secret-host",
		Retryable: true, Cause: secretCause,
	}
	got := ModelErrorFromPlatformFault(f)
	if got == nil {
		t.Fatal("expected mapped model error")
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret-host") || strings.Contains(string(encoded), "secret-value") {
		t.Fatalf("internal dependency or cause leaked: %s", encoded)
	}
}

func TestModelErrorFromPlatformFaultResponseLimit(t *testing.T) {
	f := &fault.Fault{
		Class: fault.ClassValidation, Code: "platform.httpclient.response_too_large",
		SafeMessage: "response body exceeds configured limit",
	}
	if got := ModelErrorFromPlatformFault(f); got == nil || got.Kind != ErrorResourceLimit {
		t.Fatalf("response size limit must map to resource_limit: %+v", got)
	}
}

func TestModelErrorFromPlatformFaultCancellationAndUnknown(t *testing.T) {
	cancelled := &fault.Fault{Class: fault.ClassUnavailable, Code: "platform.httpclient.canceled", Cause: context.Canceled}
	if got := ModelErrorFromPlatformFault(cancelled); got == nil || got.Kind != ErrorCancelled {
		t.Fatalf("context cancellation must map to cancelled: %+v", got)
	}
	deadline := &fault.Fault{Class: fault.ClassUnavailable, Code: "platform.httpclient.timeout", Cause: context.DeadlineExceeded}
	if got := ModelErrorFromPlatformFault(deadline); got == nil || got.Kind != ErrorTimeout {
		t.Fatalf("deadline must map to timeout: %+v", got)
	}
	if got := ModelErrorFromPlatformFault(errors.New("ordinary error")); got != nil {
		t.Fatalf("non-platform error must not be silently mapped: %+v", got)
	}
}

func TestDraft2020SchemaCompileAndFormatAssertion(t *testing.T) {
	schema := []byte(`{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"object",
		"required":["created_at"],
		"properties":{"created_at":{"type":"string","format":"date-time"}},
		"additionalProperties":false
	}`)
	if _, err := CompileJSONSchema(schema); err != nil {
		t.Fatalf("valid Draft 2020-12 schema rejected: %v", err)
	}
	if err := ValidateJSONAgainstSchema(schema, []byte(`{"created_at":"2026-11-06T10:00:00Z"}`)); err != nil {
		t.Fatalf("valid instance rejected: %v", err)
	}
	if err := ValidateJSONAgainstSchema(schema, []byte(`{"created_at":"not-a-date"}`)); !errors.Is(err, ErrJSONSchemaMismatch) {
		t.Fatalf("format assertion should reject invalid date-time, got %v", err)
	}
	if err := ValidateJSONAgainstSchema(schema, []byte(`{"created_at":"2026-11-06T10:00:00Z","extra":true}`)); !errors.Is(err, ErrJSONSchemaMismatch) {
		t.Fatalf("additionalProperties should be enforced, got %v", err)
	}
}

func TestDraft2020SchemaRejectsOtherDialectAndRemoteRefs(t *testing.T) {
	otherDialect := []byte(`{"$schema":"http://json-schema.org/draft-07/schema#","type":"object"}`)
	if _, err := CompileJSONSchema(otherDialect); !errors.Is(err, ErrInvalidJSONSchema) {
		t.Fatalf("non-2020 dialect must be rejected, got %v", err)
	}
	remoteRef := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$ref":"https://attacker.invalid/schema.json"}`)
	if _, err := CompileJSONSchema(remoteRef); !errors.Is(err, ErrInvalidJSONSchema) {
		t.Fatalf("remote $ref must be rejected without fetching, got %v", err)
	}
}

func TestDraft2020SchemaRejectsNestedOtherDialect(t *testing.T) {
	mixedDialect := []byte(`{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"$defs":{"legacy":{
			"$schema":"http://json-schema.org/draft-07/schema#",
			"type":"string"
		}}
	}`)
	if _, err := CompileJSONSchema(mixedDialect); !errors.Is(err, ErrInvalidJSONSchema) {
		t.Fatalf("nested non-2020 dialect must be rejected, got %v", err)
	}
}

func TestDraft2020SchemaDepthLimit(t *testing.T) {
	nested := `{"type":"integer"}`
	for i := 0; i < MaxJSONDepth+1; i++ {
		nested = `{"type":"array","items":` + nested + `}`
	}
	if _, err := CompileJSONSchema([]byte(nested)); !errors.Is(err, ErrInvalidJSONSchema) {
		t.Fatalf("schema nesting above limit must be rejected, got %v", err)
	}
	deepInstance := strings.Repeat("[", MaxJSONDepth+1) + "0" + strings.Repeat("]", MaxJSONDepth+1)
	if err := ValidateJSONAgainstSchema([]byte(`{"type":"array"}`), []byte(deepInstance)); !errors.Is(err, ErrJSONSchemaMismatch) {
		t.Fatalf("instance nesting above limit must be rejected, got %v", err)
	}
}

func TestToolParametersMustCompileAsDraft2020Schema(t *testing.T) {
	now := time.Now().UTC()
	r := ModelRequest{
		ProviderID: "gateway-1", RequestID: "req-1", PurposeCode: "research.summary", Model: "qwen-local",
		InputText: "Evidence", MaxOutputTokens: 128, DeadlineAt: now.Add(time.Minute),
		CorrelationID: "run-1", SystemAsOf: now,
		Tools: []ToolDefinition{{Name: "lookup_quote", ParametersJSON: json.RawMessage(`{"type":"object"}`)}},
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("valid tool schema rejected: %v", err)
	}
	r.Tools[0].ParametersJSON = json.RawMessage(`{"type":"object","properties":{"x":{"type":"unknown-type"}}}`)
	if err := r.Validate(); err == nil {
		t.Fatal("invalid JSON Schema keyword value must be rejected")
	}
}
